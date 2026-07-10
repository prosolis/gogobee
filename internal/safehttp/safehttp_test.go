package safehttp

import (
	"io"
	"strings"
	"testing"
)

// A body under the cap is passed through untouched.
func TestLimitedBodyUnderCap(t *testing.T) {
	got, err := io.ReadAll(LimitedBody(strings.NewReader("hello"), 1024))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

// A body over the cap truncates cleanly at EOF rather than failing the read.
// Callers parse whatever fits (an HTML <head>, an image header) instead of
// losing the whole document to an oversized tail.
func TestLimitedBodyTruncatesAtEOF(t *testing.T) {
	body := strings.Repeat("x", 5000)
	got, err := io.ReadAll(LimitedBody(strings.NewReader(body), 100))
	if err != nil {
		t.Fatalf("ReadAll returned an error instead of truncating: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("read %d bytes, want the 100-byte cap", len(got))
	}
}

// The cap bounds total bytes across many small reads, not just a single one.
func TestLimitedBodyCapsAcrossReads(t *testing.T) {
	r := LimitedBody(strings.NewReader(strings.Repeat("x", 5000)), 10)
	total := 0
	buf := make([]byte, 3)
	for {
		n, err := r.Read(buf)
		total += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if total > 10 {
			t.Fatalf("read %d bytes past the 10-byte cap", total)
		}
	}
	if total != 10 {
		t.Fatalf("read %d bytes, want 10", total)
	}
}
