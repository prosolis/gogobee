package plugin

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for DecodeConfig
	_ "image/jpeg" // register JPEG decoder for DecodeConfig
	_ "image/png"  // register PNG decoder for DecodeConfig
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/safehttp"

	_ "golang.org/x/image/webp" // register WebP decoder for DecodeConfig

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// thumbnailUserAgent identifies the bot when probing and downloading the
// preview image for a posted link.
const thumbnailUserAgent = "GogoBee/1.0 (+link thumbnail fetch)"

// thumbnailClient validates and downloads link-supplied image URLs. It routes
// through safehttp so a posted link can't steer fetches at loopback / RFC1918 /
// cloud-metadata IPs — the dial-time guard re-checks every redirect target too.
// The 10 MiB download ceiling below bounds memory regardless.
var thumbnailClient = safehttp.NewClient(15 * time.Second)

// resolveURL turns a possibly-relative ref into an absolute URL against base.
func resolveURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// normalizeImageURL rewrites known CDN thumbnail URLs to a higher-resolution
// variant. Currently handles The Guardian's i.guim.co.uk, whose pages hand out
// narrow thumbnails. Signed URLs are left alone (re-signing isn't possible),
// as are unrecognized hosts.
func normalizeImageURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "i.guim.co.uk" {
		return raw
	}
	q := u.Query()
	if q.Get("width") == "" || q.Get("s") != "" {
		return raw
	}
	q.Set("width", "1200")
	u.RawQuery = q.Encode()
	return u.String()
}

// trackingPixelBytes is the size at or below which a declared image is assumed
// to be a tracking pixel rather than a real preview thumbnail.
const trackingPixelBytes = 5120

// declaredSize reports the full size of the resource a probe response describes,
// or -1 when the origin declares none. A ranged probe answers 206 with a
// Content-Length covering only the requested slice, so the total comes from
// Content-Range's "bytes 0-1023/119070" suffix instead.
func declaredSize(resp *http.Response) int64 {
	if resp.StatusCode == http.StatusPartialContent {
		cr := resp.Header.Get("Content-Range")
		i := strings.LastIndexByte(cr, '/')
		if i < 0 {
			return -1
		}
		total, err := strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64)
		if err != nil {
			return -1 // "*" or malformed: unknown, not empty
		}
		return total
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			return size
		}
	}
	return -1
}

// probeImage asks the origin about rawURL using method and reports its content
// type and full size. GET probes request only the first KiB, so falling back to
// one stays about as cheap as the HEAD it replaces.
func probeImage(method, rawURL string) (contentType string, size int64, ok bool) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return "", -1, false
	}
	req.Header.Set("User-Agent", thumbnailUserAgent)
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-1023")
	}

	resp, err := thumbnailClient.Do(req)
	if err != nil {
		slog.Debug("thumbnail: image probe failed", "method", method, "url", rawURL, "err", err)
		return "", -1, false
	}
	defer resp.Body.Close()
	// Drain the sipped bytes so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		slog.Debug("thumbnail: image probe rejected", "method", method, "url", rawURL, "status", resp.StatusCode)
		return "", -1, false
	}
	return resp.Header.Get("Content-Type"), declaredSize(resp), true
}

// validateImageURL probes an image URL: it must be http(s), answer a probe, have
// an image/* content type, and (if a size is declared) exceed trackingPixelBytes.
// Returns false with no error on any failure.
//
// The probe is a HEAD first, falling back to a ranged GET: CDNs in front of some
// publishers (apnews's dims.apnews.com among them) answer HEAD with 403 while
// serving the very same URL over GET, and a HEAD-only check silently drops those
// thumbnails.
func validateImageURL(rawURL string) bool {
	if rawURL == "" || safehttp.ValidateURL(rawURL) != nil {
		return false
	}

	contentType, size, ok := probeImage(http.MethodHead, rawURL)
	if !ok {
		contentType, size, ok = probeImage(http.MethodGet, rawURL)
	}
	if !ok {
		return false
	}

	if !strings.HasPrefix(contentType, "image/") {
		slog.Debug("thumbnail: not an image", "url", rawURL, "content_type", contentType)
		return false
	}
	if size >= 0 && size <= trackingPixelBytes {
		slog.Debug("thumbnail: image too small, treating as tracking pixel", "url", rawURL, "size", size)
		return false
	}
	return true
}

// SendImageFromURL downloads imageURL, uploads it to Matrix, and posts it as an
// m.image event in roomID. Returns an error on any failure so callers can fall
// back to a text-only preview. The fetch is SSRF-guarded and size-capped.
func (b *Base) SendImageFromURL(roomID id.RoomID, imageURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return fmt.Errorf("create image request: %w", err)
	}
	req.Header.Set("User-Agent", thumbnailUserAgent)

	resp, err := thumbnailClient.Do(req)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image download status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("not an image: %s", contentType)
	}

	// Decode dimensions so clients render the image inline rather than as a
	// downloadable file attachment.
	var width, height int
	if cfg, _, decErr := image.DecodeConfig(bytes.NewReader(data)); decErr == nil {
		width, height = cfg.Width, cfg.Height
	}

	ext := ".jpg"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	filename := "thumbnail" + ext

	uri, err := b.UploadContent(data, contentType, filename)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}

	content := &event.MessageEventContent{
		MsgType:  event.MsgImage,
		Body:     filename,
		FileName: filename,
		URL:      uri.CUString(),
		Info: &event.FileInfo{
			MimeType: contentType,
			Size:     len(data),
			Width:    width,
			Height:   height,
		},
	}
	_, err = b.Client.SendMessageEvent(context.Background(), roomID, event.EventMessage, content)
	return err
}
