package plugin

import "testing"

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
