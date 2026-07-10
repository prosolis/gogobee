// Package safehttp provides an http.Client hardened against SSRF and
// memory-DoS via hostile upstreams. Every outbound fetch the bot makes
// against feed-supplied URLs (RSS articles, image hosts) should go through
// one of these clients so a malicious feed can't steer the bot at loopback,
// link-local, RFC1918, or cloud metadata IPs, and can't OOM the process by
// streaming an unbounded body.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrBlockedHost is returned when a URL resolves to a non-public IP.
var ErrBlockedHost = errors.New("safehttp: blocked non-public host")

// AllowPrivate, when true, disables the loopback/RFC1918 dial guard. It
// exists for tests that spin up httptest.NewServer on 127.0.0.1 — never
// set this in production.
var AllowPrivate bool

// safeDialContext refuses connections to non-public IPs. It runs after
// DNS resolution, so a hostile DNS rebinding that returns 127.0.0.1
// still gets blocked at dial time.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := (&net.Resolver{}).LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var allowed net.IP
	for _, ip := range ips {
		if AllowPrivate || isPublicIP(ip) {
			allowed = ip
			break
		}
	}
	if allowed == nil {
		return nil, fmt.Errorf("%w: %s", ErrBlockedHost, host)
	}
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(allowed.String(), port))
}

// isPublicIP reports whether ip is a globally routable unicast address.
// Rejects loopback, link-local, multicast, RFC1918, CGNAT, and the
// AWS/GCP/Azure metadata IPs 169.254.169.254 / fd00:ec2::254 (these
// already fall under link-local but spell it out for clarity).
func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsPrivate() {
		return false
	}
	// 100.64.0.0/10 (CGNAT) is not covered by IsPrivate on older Go.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
		// 0.0.0.0/8 and friends.
		if v4[0] == 0 {
			return false
		}
	}
	return true
}

// ValidateURL returns nil if the URL is http(s) and parseable. It does
// not resolve DNS — the dial step does that — but it does reject bare
// schemes (file://, gopher://, etc.) before we even open a connection.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("safehttp: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("safehttp: empty host")
	}
	return nil
}

// NewClient returns an http.Client whose transport blocks non-public
// destinations at dial time, caps redirects at 5, and re-validates each
// redirect target's scheme. timeout is the per-request overall budget.
func NewClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("safehttp: stopped after 5 redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("safehttp: unsupported redirect scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// LimitedBody wraps r in a reader that reports EOF once max bytes have been
// read. Use to cap how much of a response body downstream parsers (goquery,
// image.Decode) will ever see — a hostile origin streaming an endless body
// otherwise OOMs the process.
//
// Hitting the cap is a truncation, not an error: parsers get a short-but-valid
// body and decide for themselves whether they found what they needed. Returning
// an error here instead would fail the whole parse on any oversized page, even
// when the interesting bytes (an HTML <head>, an image header) sit well inside
// the cap.
func LimitedBody(r io.Reader, max int64) io.Reader {
	return &limitedReader{R: r, N: max}
}

type limitedReader struct {
	R io.Reader
	N int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.N <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.N {
		p = p[:l.N]
	}
	n, err := l.R.Read(p)
	l.N -= int64(n)
	return n, err
}
