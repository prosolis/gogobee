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

// validateImageURL HEAD-probes an image URL: it must be http(s), return 200,
// have an image/* content type, and (if a length is declared) exceed 5 KiB so
// tracking pixels are filtered. Returns false with no error on any failure.
func validateImageURL(rawURL string) bool {
	if rawURL == "" || safehttp.ValidateURL(rawURL) != nil {
		return false
	}
	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", thumbnailUserAgent)

	resp, err := thumbnailClient.Do(req)
	if err != nil {
		slog.Debug("thumbnail: image HEAD failed", "url", rawURL, "err", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return false
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil && size <= 5120 {
			return false // tracking pixel
		}
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
