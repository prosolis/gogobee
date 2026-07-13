package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/id"
)

// fakeHS serves the two endpoints the lazy store resolves against, counting hits
// so we can assert it caches instead of re-asking on every send.
type fakeHS struct {
	encryptionStatus int
	encryptionBody   string
	membersBody      string

	encryptionHits atomic.Int32
	memberHits     atomic.Int32
}

func (f *fakeHS) start(t *testing.T) *mautrix.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			f.encryptionHits.Add(1)
			w.WriteHeader(f.encryptionStatus)
			_, _ = w.Write([]byte(f.encryptionBody))
		case strings.HasSuffix(r.URL.Path, "/members"):
			f.memberHits.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(f.membersBody))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := mautrix.NewClient(srv.URL, "@bot:example.org", "token")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func newTestStore(t *testing.T, hs *fakeHS) *lazyStateStore {
	t.Helper()
	store := newLazyStateStore(mautrix.NewMemoryStateStore().(appservice.StateStore))
	store.client = hs.start(t)
	return store
}

const roomID = id.RoomID("!room:example.org")

// The regression: a room configured as encrypted before the process started. No
// transaction ever re-announces it, so the store must go ask.
func TestIsEncryptedResolvesFromServer(t *testing.T) {
	hs := &fakeHS{
		encryptionStatus: http.StatusOK,
		encryptionBody:   `{"algorithm":"m.megolm.v1.aes-sha2"}`,
	}
	store := newTestStore(t, hs)

	enc, err := store.IsEncrypted(context.Background(), roomID)
	if err != nil {
		t.Fatalf("IsEncrypted: %v", err)
	}
	if !enc {
		t.Fatal("room is encrypted on the server but IsEncrypted said false — this is the plaintext leak")
	}
}

// A room with no m.room.encryption really is plaintext, and must not re-fetch.
func TestIsEncryptedCachesUnencrypted(t *testing.T) {
	hs := &fakeHS{
		encryptionStatus: http.StatusNotFound,
		encryptionBody:   `{"errcode":"M_NOT_FOUND","error":"Event not found."}`,
	}
	store := newTestStore(t, hs)

	for i := 0; i < 3; i++ {
		enc, err := store.IsEncrypted(context.Background(), roomID)
		if err != nil {
			t.Fatalf("IsEncrypted: %v", err)
		}
		if enc {
			t.Fatal("unencrypted room reported as encrypted")
		}
	}
	if got := hs.encryptionHits.Load(); got != 1 {
		t.Errorf("expected the M_NOT_FOUND to be cached, got %d fetches", got)
	}
}

// The important half: an unresolvable room must fail the send, not quietly
// answer "not encrypted" and let the message out in the clear.
func TestIsEncryptedFailsClosed(t *testing.T) {
	hs := &fakeHS{
		encryptionStatus: http.StatusInternalServerError,
		encryptionBody:   `{"errcode":"M_UNKNOWN","error":"oops"}`,
	}
	store := newTestStore(t, hs)

	if _, err := store.IsEncrypted(context.Background(), roomID); err == nil {
		t.Fatal("a failed lookup returned no error; the send would proceed in plaintext")
	}
	// The failure must not poison the cache: a later send has to retry.
	hs.encryptionStatus = http.StatusOK
	hs.encryptionBody = `{"algorithm":"m.megolm.v1.aes-sha2"}`
	enc, err := store.IsEncrypted(context.Background(), roomID)
	if err != nil {
		t.Fatalf("IsEncrypted after recovery: %v", err)
	}
	if !enc {
		t.Fatal("store did not retry after a transient failure")
	}
}

// A burst of proactive sends into a cold room should resolve it once.
func TestIsEncryptedResolvesOncePerRoom(t *testing.T) {
	hs := &fakeHS{
		encryptionStatus: http.StatusOK,
		encryptionBody:   `{"algorithm":"m.megolm.v1.aes-sha2"}`,
	}
	store := newTestStore(t, hs)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.IsEncrypted(context.Background(), roomID); err != nil {
				t.Errorf("IsEncrypted: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := hs.encryptionHits.Load(); got != 1 {
		t.Errorf("expected 1 state fetch for 20 concurrent sends, got %d", got)
	}
}

// Encrypting to an empty member list would share the group session with nobody,
// so members must be resolved too.
func TestMembersResolveFromServer(t *testing.T) {
	hs := &fakeHS{
		encryptionStatus: http.StatusOK,
		encryptionBody:   `{"algorithm":"m.megolm.v1.aes-sha2"}`,
		membersBody: `{"chunk":[
			{"type":"m.room.member","room_id":"!room:example.org","state_key":"@alice:example.org","sender":"@alice:example.org","content":{"membership":"join"}},
			{"type":"m.room.member","room_id":"!room:example.org","state_key":"@bot:example.org","sender":"@bot:example.org","content":{"membership":"join"}}
		]}`,
	}
	store := newTestStore(t, hs)

	members, err := store.GetRoomJoinedOrInvitedMembers(context.Background(), roomID)
	if err != nil {
		t.Fatalf("GetRoomJoinedOrInvitedMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d (%v) — the group session would be shared to nobody", len(members), members)
	}

	if _, err := store.GetRoomJoinedOrInvitedMembers(context.Background(), roomID); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := hs.memberHits.Load(); got != 1 {
		t.Errorf("expected members to be cached after one fetch, got %d", got)
	}
}
