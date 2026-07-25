package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

// W9: the three web verbs that undo something — abandon an expedition, walk out
// of somebody else's party, send the sitter home.
//
// The thing worth pinning here is not the verdict text, it is the LOCK. Each of
// these three now has a headless twin shared between a Matrix command and the web
// order path, and the two halves take advUserLock on opposite sides: the two
// expedition twins cannot take it (their Matrix caller holds it across the whole
// `!expedition` switch) and their web wrappers must, while the babysit twin takes
// it itself and its web wrapper must not.
//
// Getting either of those backwards does not fail loudly. advUserLock is a plain
// sync.Mutex, so a second acquire parks the goroutine forever with the deferred
// Unlock never running — which wedges every later !adventure / !expedition /
// !zone command from that player, not just the one that deadlocked. That is a
// bug that shipped once already (see TestExpeditionAliasesDoNotWedgeTheUserLock),
// so both directions are tested here.
//
// NOTE: these two lock tests HANG rather than fail on regression. The timeout is
// the assertion.

// TestUndoCommandsDoNotWedgeTheUserLock is the Matrix half: `!expedition abandon`
// and `!expedition leave` reach their twins with the lock already held, so a twin
// that took it itself would park here.
func TestUndoCommandsDoNotWedgeTheUserLock(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@w9-cmd-lock:example")
	t.Cleanup(func() { cleanupExpeditions(uid) })

	for _, sub := range []string{"abandon", "leave"} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			p := &AdventurePlugin{euro: &EuroPlugin{}}
			_ = p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, sub)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("!expedition %s never returned: its twin re-took advUserLock", sub)
		}
	}
}

// TestUndoOrdersDoNotWedgeTheUserLock is the web half, and it checks the harder
// half of the same property: not just that the order returns, but that the lock
// is FREE afterwards. A wrapper that took the lock around a twin that also takes
// it would park inside applyAdvOrder; a wrapper that forgot to release would let
// the order finish and wedge the next command instead, which is the failure that
// would have been missed by only timing the call.
func TestUndoOrdersDoNotWedgeTheUserLock(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@w9-order-lock:example")
	t.Cleanup(func() { cleanupExpeditions(uid) })

	for _, action := range []string{
		peteclient.AdvOrderAbandon,
		peteclient.AdvOrderLeave,
		peteclient.AdvOrderBabysitCancel,
	} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			p := &AdventurePlugin{euro: &EuroPlugin{}}
			p.applyAdvOrder(uid, peteclient.AdvOrder{GUID: "g-" + action, Action: action})
			// The lock must be back. Taking it here is what catches a wrapper that
			// returned without unlocking.
			mu := p.advUserLock(uid)
			mu.Lock()
			mu.Unlock()
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("order %q never returned or never released advUserLock", action)
		}
	}
}

// TestUndoOrderRefusalsAreTerminal: none of the three may come back as a retry.
// A retried refusal never reaches a verdict, so the order sits pending forever
// and the panel never stops saying "asked for…". Each also has to carry prose —
// the strip prefers gogobee's own sentence over its canned fallback, and an empty
// detail on a refusal is the one case where the page has nothing to show.
func TestUndoOrderRefusalsAreTerminal(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@w9-refusals:example")
	t.Cleanup(func() { cleanupExpeditions(uid) })
	// A real character with nothing going on. Without one, babysit_cancel refuses
	// with "no adventurer" and the sitter branch this is meant to cover is never
	// reached — the first cut of this test passed the wrong assertion for that
	// reason.
	if err := createAdvCharacter(uid, "w9refusals"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	p := &AdventurePlugin{euro: &EuroPlugin{}}

	cases := []struct {
		action string
		want   string
	}{
		// Nothing to abandon and nothing to leave are the same fact from two
		// doors, and both are the plain "you aren't on one" answer.
		{peteclient.AdvOrderAbandon, "rejected_not_running"},
		{peteclient.AdvOrderLeave, "rejected_not_running"},
		// No sitter is its own verdict rather than rejected_unavailable: the page
		// says something specific about it, and "unavailable" reads as a fault.
		{peteclient.AdvOrderBabysitCancel, "rejected_nothing_to_cancel"},
	}
	for _, tc := range cases {
		status, detail, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
			GUID: "g-" + tc.action, Action: tc.action,
		})
		if retry {
			t.Fatalf("%s asked for a retry on a refusal; it must be terminal", tc.action)
		}
		if status != tc.want {
			t.Fatalf("%s status = %q, want %q", tc.action, status, tc.want)
		}
		if strings.TrimSpace(detail) == "" {
			t.Fatalf("%s refused with no prose; the panel would have nothing to say", tc.action)
		}
	}
}

// TestUndoVerdictsCarryNoMarkdown: every detail line these file is reused from a
// Matrix sentence, and Pete renders a verdict as text. An asterisk or a backtick
// left in it shows up literally under the button.
//
// The backticks matter more than they look: the leave refusal names `!extract`
// and `!expedition abandon`, which is the correct thing to say (the web now has a
// button for one of them and not the other), but it must not say it in markup.
func TestUndoVerdictsCarryNoMarkdown(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@w9-markdown:example")
	t.Cleanup(func() { cleanupExpeditions(uid) })
	p := &AdventurePlugin{euro: &EuroPlugin{}}

	for _, action := range []string{
		peteclient.AdvOrderAbandon,
		peteclient.AdvOrderLeave,
		peteclient.AdvOrderBabysitCancel,
	} {
		_, detail, _ := p.applyAdvOrder(uid, peteclient.AdvOrder{GUID: "g-md-" + action, Action: action})
		if strings.ContainsAny(detail, "*`\n") {
			t.Fatalf("%s verdict carries markdown or a newline: %q", action, detail)
		}
	}
}

// TestWebAbandonIsTheGamesOwnAbandon: the web verb must run the real path, not a
// lookalike. The proof is the state the row lands in — no expedition left at all,
// as opposed to the 'extracting' limbo an extraction leaves behind — and that the
// verdict says what became of the supplies, which is the one thing a player who
// clicked the wrong button needs to be told.
func TestWebAbandonIsTheGamesOwnAbandon(t *testing.T) {
	// setupEmptyTestDB, NOT setupZoneRunTestDB: the latter copies data/gogobee.db
	// and t.Skip()s when it is missing, and that file is deleted after every local
	// run — so a test written on it is green-by-skipping on any clean checkout.
	// See the Decisions note in the plan's progress file; W5a's order tests were
	// silently skipping for exactly this reason.
	setupEmptyTestDB(t)
	uid := id.UserID("@w9-abandon-live:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid) })
	if err := createAdvCharacter(uid, "w9abandon"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	p := &AdventurePlugin{euro: &EuroPlugin{}}

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{
		Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1, PacksStandard: 1,
	}); err != nil {
		t.Fatalf("startExpedition: %v", err)
	}

	status, detail, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g-abandon", Action: peteclient.AdvOrderAbandon,
	})
	if retry || status != "applied" {
		t.Fatalf("abandon = %q retry=%v detail=%q, want applied", status, retry, detail)
	}
	if !strings.Contains(strings.ToLower(detail), "supplies") {
		t.Fatalf("verdict %q never says the supplies are gone, which is the difference from pulling out", detail)
	}
	if exp, _, err := activeExpeditionFor(uid); err != nil {
		t.Fatalf("read expedition: %v", err)
	} else if exp != nil {
		t.Fatalf("expedition survived a web abandon with status %q", exp.Status)
	}

	// And the second click, which is what a stale page produces: a terminal
	// refusal, never a retry and never a second abandon.
	status, _, retry = p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g-abandon-2", Action: peteclient.AdvOrderAbandon,
	})
	if retry || status != "rejected_not_running" {
		t.Fatalf("re-abandon = %q retry=%v, want a terminal rejected_not_running", status, retry)
	}
}
