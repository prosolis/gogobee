package plugin

import (
	"sync"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newEuroTestDB(t *testing.T) *EuroPlugin {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return NewEuroPlugin(nil)
}

func seedBalance(t *testing.T, p *EuroPlugin, user id.UserID, amount float64) {
	t.Helper()
	p.ensureBalance(user)
	if _, err := db.Get().Exec(
		`UPDATE euro_balances SET balance = ? WHERE user_id = ?`, amount, string(user),
	); err != nil {
		t.Fatal(err)
	}
}

func TestDebitIdem_ReplaySameGUIDDebitsOnce(t *testing.T) {
	p := newEuroTestDB(t)
	user := id.UserID("@replay:test.invalid")
	seedBalance(t, p, user, 500)

	for i := 0; i < 3; i++ {
		ok, bal, err := p.DebitIdem(user, 100, "games_buyin", "guid-abc")
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("attempt %d: debit rejected", i)
		}
		if bal != 400 {
			t.Fatalf("attempt %d: balance after = %v, want 400", i, bal)
		}
	}

	if got := p.GetBalance(user); got != 400 {
		t.Fatalf("final balance = %v, want 400 (replay debited more than once)", got)
	}

	var n int
	if err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM euro_transactions WHERE external_id = 'guid-abc'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("logged %d transactions for one guid, want 1", n)
	}
}

func TestCreditIdem_ReplaySameGUIDCreditsOnce(t *testing.T) {
	p := newEuroTestDB(t)
	user := id.UserID("@cashout:test.invalid")
	seedBalance(t, p, user, 0)

	for i := 0; i < 3; i++ {
		ok, bal, err := p.CreditIdem(user, 250, "games_cashout", "guid-xyz")
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if !ok || bal != 250 {
			t.Fatalf("attempt %d: ok=%v balance=%v, want true/250", i, ok, bal)
		}
	}

	if got := p.GetBalance(user); got != 250 {
		t.Fatalf("final balance = %v, want 250", got)
	}
}

func TestDebitIdem_DistinctGUIDsBothApply(t *testing.T) {
	p := newEuroTestDB(t)
	user := id.UserID("@distinct:test.invalid")
	seedBalance(t, p, user, 500)

	for _, guid := range []string{"guid-1", "guid-2"} {
		if ok, _, err := p.DebitIdem(user, 100, "games_buyin", guid); err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", guid, ok, err)
		}
	}
	if got := p.GetBalance(user); got != 300 {
		t.Fatalf("balance = %v, want 300", got)
	}
}

func TestDebitIdem_RespectsDebtLimitAndLogsNothing(t *testing.T) {
	t.Setenv("BLACKJACK_DEBT_LIMIT", "1000")
	p := newEuroTestDB(t)
	user := id.UserID("@broke:test.invalid")
	seedBalance(t, p, user, 0)

	ok, _, err := p.DebitIdem(user, 1500, "games_buyin", "guid-broke")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("debit past the debt limit was accepted")
	}
	if got := p.GetBalance(user); got != 0 {
		t.Fatalf("balance = %v, want 0 — a rejected debit moved money", got)
	}

	// A rejection must leave no trace, so the same guid can be retried later
	// (e.g. once the player has earned their way back above the limit).
	var n int
	if err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM euro_transactions WHERE external_id = 'guid-broke'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rejected debit logged %d transactions, want 0", n)
	}
}

// The SELECT-then-UPDATE in applyIdem is not by itself a guard against two
// concurrent claims of the same guid; the unique index is. Hammer it.
func TestDebitIdem_ConcurrentSameGUIDDebitsOnce(t *testing.T) {
	p := newEuroTestDB(t)
	user := id.UserID("@race:test.invalid")
	seedBalance(t, p, user, 1000)

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = p.DebitIdem(user, 100, "games_buyin", "guid-race")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("racer %d: %v", i, err)
		}
	}
	if got := p.GetBalance(user); got != 900 {
		t.Fatalf("balance = %v, want 900 — %d concurrent claims of one guid double-debited", got, racers)
	}
}

func TestIdem_RejectsEmptyExternalID(t *testing.T) {
	p := newEuroTestDB(t)
	user := id.UserID("@noguid:test.invalid")
	seedBalance(t, p, user, 500)

	if _, _, err := p.DebitIdem(user, 10, "games_buyin", ""); err == nil {
		t.Fatal("DebitIdem accepted an empty external id")
	}
	if _, _, err := p.CreditIdem(user, 10, "games_cashout", ""); err == nil {
		t.Fatal("CreditIdem accepted an empty external id")
	}
	if got := p.GetBalance(user); got != 500 {
		t.Fatalf("balance = %v, want 500", got)
	}
}
