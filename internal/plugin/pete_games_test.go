package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// fakePete is Pete's half of the escrow wire: it offers rows, lets us claim
// them, and records the verdicts we push back. Enough to drive the loop end to
// end without either real box.
type fakePete struct {
	mu       sync.Mutex
	pending  []peteclient.Escrow
	claimed  map[string]bool
	verdicts []peteclient.EscrowVerdict
	srv      *httptest.Server
}

func newFakePete(t *testing.T, token string, rows ...peteclient.Escrow) *fakePete {
	t.Helper()
	f := &fakePete{pending: rows, claimed: map[string]bool{}}

	mux := http.NewServeMux()
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+token }

	mux.HandleFunc("GET /api/games/escrow/pending", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.pending)
	})
	mux.HandleFunc("POST /api/games/escrow/claim", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		var req struct{ GUID string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, e := range f.pending {
			if e.GUID != req.GUID {
				continue
			}
			f.claimed[e.GUID] = true
			e.State = "claimed"
			_ = json.NewEncoder(w).Encode(e)
			return
		}
		w.WriteHeader(404)
	})
	mux.HandleFunc("POST /api/games/escrow/settled", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		var v peteclient.EscrowVerdict
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.verdicts = append(f.verdicts, v)
		w.WriteHeader(200)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// wire points the peteclient singleton at the fake and turns the seam on.
func (f *fakePete) wire(t *testing.T, token string) {
	t.Helper()
	t.Setenv("PETE_INGEST_URL", f.srv.URL)
	t.Setenv("PETE_INGEST_TOKEN", token)
	t.Setenv("FEATURE_PETE_NEWS", "true")
	peteclient.Init()
}

func (f *fakePete) got() []peteclient.EscrowVerdict {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]peteclient.EscrowVerdict(nil), f.verdicts...)
}

// TestEscrowLoopBuyInDebitsOnceAndReportsIt is the happy path across the border:
// Pete offers a buy-in, we take the euros, and the verdict lands back on Pete
// carrying the balance the player now has.
func TestEscrowLoopBuyInDebitsOnceAndReportsIt(t *testing.T) {
	euro := newEuroTestDB(t)
	user := id.UserID("@reala:parodia.dev")
	seedBalance(t, euro, user, 1000)

	pete := newFakePete(t, "tok", peteclient.Escrow{
		GUID: "esc-1", MatrixUser: string(user), Kind: "buyin", Amount: 400, State: "requested",
	})
	pete.wire(t, "tok")

	pollEscrowOnce(context.Background(), euro)

	if got := euro.GetBalance(user); got != 600 {
		t.Fatalf("balance = %v, want 600 (1000 less a 400 buy-in)", got)
	}
	v := pete.got()
	if len(v) != 1 || !v[0].OK || v[0].GUID != "esc-1" {
		t.Fatalf("verdicts = %+v, want one ok verdict for esc-1", v)
	}
	if v[0].BalanceAfter != 600 {
		t.Fatalf("balance_after = %v, want 600", v[0].BalanceAfter)
	}
}

// TestEscrowLoopReofferedRowDebitsOnce is the failure this whole design is built
// around. We move the euros, then die before the verdict is delivered. Pete
// re-offers the row. The player must not pay twice.
func TestEscrowLoopReofferedRowDebitsOnce(t *testing.T) {
	euro := newEuroTestDB(t)
	user := id.UserID("@twice:parodia.dev")
	seedBalance(t, euro, user, 1000)

	pete := newFakePete(t, "tok", peteclient.Escrow{
		GUID: "esc-dup", MatrixUser: string(user), Kind: "buyin", Amount: 250, State: "requested",
	})
	pete.wire(t, "tok")

	// Three passes over a row Pete keeps offering, as it would after a crash.
	for i := 0; i < 3; i++ {
		pollEscrowOnce(context.Background(), euro)
	}

	if got := euro.GetBalance(user); got != 750 {
		t.Fatalf("balance = %v, want 750 — the re-offered row debited more than once", got)
	}
	// The verdict is queued under the escrow guid, so it is enqueued once no
	// matter how many times we decide it. Pete's settle is idempotent anyway,
	// but the queue is the first line of that defence.
	if v := pete.got(); len(v) != 1 {
		t.Fatalf("delivered %d verdicts for one row, want 1: %+v", len(v), v)
	}
}

// TestEscrowLoopBrokePlayerIsRejectedCleanly — the debit is refused. Nothing may
// move, and Pete has to be told why, because the player is staring at a spinner
// that needs to become a sentence.
func TestEscrowLoopBrokePlayerIsRejectedCleanly(t *testing.T) {
	euro := newEuroTestDB(t)
	user := id.UserID("@broke:parodia.dev")
	seedBalance(t, euro, user, 10)

	pete := newFakePete(t, "tok", peteclient.Escrow{
		GUID: "esc-broke", MatrixUser: string(user), Kind: "buyin", Amount: 5000, State: "requested",
	})
	pete.wire(t, "tok")

	pollEscrowOnce(context.Background(), euro)

	if got := euro.GetBalance(user); got != 10 {
		t.Fatalf("balance = %v, want 10 untouched", got)
	}
	v := pete.got()
	if len(v) != 1 || v[0].OK || v[0].Reason != reasonInsufficientFunds {
		t.Fatalf("verdicts = %+v, want one rejection carrying insufficient_funds", v)
	}
}

// TestEscrowLoopCashOutCredits — the way out of the casino.
func TestEscrowLoopCashOutCredits(t *testing.T) {
	euro := newEuroTestDB(t)
	user := id.UserID("@winner:parodia.dev")
	seedBalance(t, euro, user, 100)

	pete := newFakePete(t, "tok", peteclient.Escrow{
		GUID: "esc-out", MatrixUser: string(user), Kind: "cashout", Amount: 900, State: "requested",
	})
	pete.wire(t, "tok")

	pollEscrowOnce(context.Background(), euro)

	if got := euro.GetBalance(user); got != 1000 {
		t.Fatalf("balance = %v, want 1000", got)
	}
	if v := pete.got(); len(v) != 1 || !v[0].OK || v[0].BalanceAfter != 1000 {
		t.Fatalf("verdicts = %+v, want one ok cash-out at 1000", v)
	}
}

// TestEscrowLoopUnknownKindIsRefusedNotRetried — a row we can't interpret. A
// silent skip would leave it re-offered forever, so say no and let it reach a
// terminal state on Pete.
func TestEscrowLoopUnknownKindIsRefusedNotRetried(t *testing.T) {
	euro := newEuroTestDB(t)
	user := id.UserID("@weird:parodia.dev")
	seedBalance(t, euro, user, 500)

	pete := newFakePete(t, "tok", peteclient.Escrow{
		GUID: "esc-weird", MatrixUser: string(user), Kind: "tribute", Amount: 100, State: "requested",
	})
	pete.wire(t, "tok")

	pollEscrowOnce(context.Background(), euro)

	if got := euro.GetBalance(user); got != 500 {
		t.Fatalf("balance = %v, want 500 untouched", got)
	}
	v := pete.got()
	if len(v) != 1 || v[0].OK || v[0].Reason != "unknown_kind" {
		t.Fatalf("verdicts = %+v, want one refusal", v)
	}
}
