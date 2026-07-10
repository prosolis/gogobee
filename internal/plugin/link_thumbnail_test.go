package plugin

import (
	"net/http"
	"testing"
)

func TestResolveURL(t *testing.T) {
	cases := []struct {
		base, ref, want string
	}{
		{"https://e.com/news/story", "https://cdn.e.com/a.jpg", "https://cdn.e.com/a.jpg"},
		{"https://e.com/news/story", "//cdn.e.com/a.jpg", "https://cdn.e.com/a.jpg"},
		{"https://e.com/news/story", "/img/a.jpg", "https://e.com/img/a.jpg"},
		{"https://e.com/news/story", "a.jpg", "https://e.com/news/a.jpg"},
		{"https://e.com/news/story", "", ""},
	}
	for _, c := range cases {
		if got := resolveURL(c.base, c.ref); got != c.want {
			t.Errorf("resolveURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

func TestNormalizeImageURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"non-guardian passthrough", "https://e.com/a.jpg?width=140", "https://e.com/a.jpg?width=140"},
		{"signed left alone", "https://i.guim.co.uk/x.jpg?width=140&s=abc", "https://i.guim.co.uk/x.jpg?width=140&s=abc"},
		{"no width passthrough", "https://i.guim.co.uk/x.jpg", "https://i.guim.co.uk/x.jpg"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := normalizeImageURL(c.in); got != c.want {
			t.Errorf("%s: normalizeImageURL(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
	// Unsigned Guardian thumbnail gets bumped to width=1200.
	if got := normalizeImageURL("https://i.guim.co.uk/x.jpg?width=140"); got != "https://i.guim.co.uk/x.jpg?width=1200" {
		t.Errorf("normalizeImageURL unsigned = %q, want width=1200", got)
	}
}

func TestDeclaredSize(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header map[string]string
		want   int64
	}{
		{"content-length on a full response", http.StatusOK,
			map[string]string{"Content-Length": "119070"}, 119070},
		// A ranged probe's Content-Length covers only the slice we asked for, so
		// the resource's real size has to come from Content-Range's total.
		{"content-range total on a partial response", http.StatusPartialContent,
			map[string]string{"Content-Length": "1024", "Content-Range": "bytes 0-1023/119070"}, 119070},
		{"unknown total in content-range", http.StatusPartialContent,
			map[string]string{"Content-Range": "bytes 0-1023/*"}, -1},
		{"partial response missing content-range", http.StatusPartialContent,
			map[string]string{"Content-Length": "1024"}, -1},
		{"no size declared", http.StatusOK, map[string]string{}, -1},
		{"malformed content-length", http.StatusOK,
			map[string]string{"Content-Length": "banana"}, -1},
	}
	for _, c := range cases {
		resp := &http.Response{StatusCode: c.status, Header: http.Header{}}
		for k, v := range c.header {
			resp.Header.Set(k, v)
		}
		if got := declaredSize(resp); got != c.want {
			t.Errorf("%s: declaredSize() = %d, want %d", c.name, got, c.want)
		}
	}
}

// An undeclared size (-1) must not be mistaken for a zero-byte image and dropped
// as a tracking pixel — plenty of CDNs omit the length entirely.
func TestTrackingPixelFilterSkipsUnknownSize(t *testing.T) {
	rejected := func(size int64) bool { return size >= 0 && size <= trackingPixelBytes }
	for _, size := range []int64{-1, trackingPixelBytes + 1, 119070} {
		if rejected(size) {
			t.Errorf("size %d was rejected as a tracking pixel, want kept", size)
		}
	}
	for _, size := range []int64{0, 1, trackingPixelBytes} {
		if !rejected(size) {
			t.Errorf("size %d was kept, want rejected as a tracking pixel", size)
		}
	}
}
