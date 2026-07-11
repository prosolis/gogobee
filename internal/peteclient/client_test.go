package peteclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gogobee/internal/db"
)

// TestEmitDrainRoundTrip proves the durable path: Emit queues a fact, the sender
// POSTs it to Pete with bearer auth, and the row is marked sent (so it won't
// re-send), while a duplicate GUID collapses to one delivery.
func TestEmitDrainRoundTrip(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var got []Fact
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ingest/adventure" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var f Fact
		_ = json.Unmarshal(body, &f)
		mu.Lock()
		got = append(got, f)
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	std = &Client{cfg: Config{IngestURL: srv.URL, Token: "tok", Enabled: true}, http: srv.Client()}

	Emit(Fact{GUID: "death:a:1", EventType: "death", Tier: "priority", Subject: "Brannigan", OccurredAt: 1})
	Emit(Fact{GUID: "death:a:1", EventType: "death", Tier: "priority", Subject: "Brannigan", OccurredAt: 1}) // dup guid

	std.drain(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("delivered %d facts, want 1 (dup should collapse)", len(got))
	}
	if got[0].Subject != "Brannigan" {
		t.Errorf("subject = %q", got[0].Subject)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q", gotAuth)
	}

	var pending int
	if err := db.Get().QueryRow(`SELECT count(*) FROM pete_emit_queue WHERE sent_at IS NULL`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("%d rows still pending after successful drain", pending)
	}

	// A second drain re-sends nothing.
	std.drain(context.Background())
	if len(got) != 1 {
		t.Errorf("re-drained a sent row: %d deliveries", len(got))
	}
}

// TestEmitDisabledNoQueue: when disabled, Emit is a durable no-op.
func TestEmitDisabledNoQueue(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	std = &Client{cfg: Config{Enabled: false}, http: http.DefaultClient}
	Emit(Fact{GUID: "disabled-guid", EventType: "death", OccurredAt: 1})
	var n int
	// db.Init is a process singleton, so this may share state with other tests;
	// scope the check to this fact's guid.
	if err := db.Get().QueryRow(`SELECT count(*) FROM pete_emit_queue WHERE guid = 'disabled-guid'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("disabled Emit queued %d rows, want 0", n)
	}
}
