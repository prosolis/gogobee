package plugin

import (
	"encoding/json"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// The retreat bulletin: the news can finally say "it went badly and nobody
// died". Before this, every event type Pete could file was a win, a death, or an
// introduction — so a run that simply fell apart was reported as nothing at all,
// and a class that retreated a third of the time just looked absent from the
// feed.
//
// The reason gate is the whole safety property. A death must not ALSO be filed
// as a retreat (it already files its own priority dispatch), and an idle reap
// must not be filed at all — Pete telling the realm that a named player was
// driven from the field, when in truth they closed their laptop, is a lie about
// a person by name.

func retreatFactFor(t *testing.T, uid id.UserID) map[string]any {
	t.Helper()
	var payload string
	err := db.Get().QueryRow(
		`SELECT payload FROM pete_emit_queue WHERE guid LIKE 'retreat:%'`).Scan(&payload)
	if err != nil {
		t.Fatalf("no retreat fact queued: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal([]byte(payload), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRetreatNews_ReasonGate(t *testing.T) {
	cases := []struct {
		reason string
		want   int
		why    string
	}{
		{lossCombatRetreat, 1, "a solo walker who ran out the clock and withdrew is news"},
		{lossCombatFlee, 1, "a party that broke off is news"},
		{lossCombatDeath, 0, "a death already files its own priority dispatch — do not double-report it as a retreat"},
		{lossIdleTimeout, 0, "an idle reap is not a retreat; the player closed their laptop, they did not flee"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			setupEmptyTestDB(t)
			enablePeteSeam(t)
			uid := id.UserID("@retreat:example.org")
			zoneCmdTestCharacter(t, uid, 10)

			emitRetreatNews(uid, tc.reason, ZoneID("underforge"), 3)

			if got := queuedCount(t, "retreat:%"); got != tc.want {
				t.Fatalf("reason %q queued %d retreat facts, want %d — %s",
					tc.reason, got, tc.want, tc.why)
			}
		})
	}
}

// The bulletin has to carry enough for Pete to write a sentence: who, where, how
// far they got. A retreat with no day count is just "someone left".
func TestRetreatNews_CarriesTheStory(t *testing.T) {
	setupEmptyTestDB(t)
	enablePeteSeam(t)
	uid := id.UserID("@retreat-story:example.org")
	zoneCmdTestCharacter(t, uid, 10)

	emitRetreatNews(uid, lossCombatRetreat, ZoneID("underforge"), 3)

	f := retreatFactFor(t, uid)
	if f["event_type"] != "retreat" {
		t.Errorf("event_type = %v, want retreat", f["event_type"])
	}
	// Bulletin, not priority: a retreat is a bad day, not a funeral.
	if f["tier"] != "bulletin" {
		t.Errorf("tier = %v, want bulletin", f["tier"])
	}
	if f["outcome"] != "retreated" {
		t.Errorf("outcome = %v, want retreated", f["outcome"])
	}
	if f["count"] != float64(3) {
		t.Errorf("count = %v, want 3 (the day it fell apart)", f["count"])
	}
	if f["zone"] == nil || f["zone"] == "" {
		t.Error("no zone — Pete cannot say where it happened")
	}
	if f["subject"] == nil || f["subject"] == "" {
		t.Error("no subject — Pete cannot say who it happened to")
	}
	// GUID prefix must equal the event type; Pete's taxonomy keys off it.
	if guid, _ := f["guid"].(string); len(guid) < 8 || guid[:8] != "retreat:" {
		t.Errorf("guid %q must be prefixed with the event type", guid)
	}
}

// The wiring, not just the emitter. Everything above calls emitRetreatNews
// directly, which proves nothing about the game: the fact only ships if the real
// run-loss chokepoint actually calls it, on a real expedition, and reads the day
// count BEFORE forcedExtractExpedition stamps the row 'abandoned' and takes the
// live fields with it. Drive the chokepoint.
func TestRetreatNews_WiredToTheRunLossChokepoint(t *testing.T) {
	setupEmptyTestDB(t)
	enablePeteSeam(t)
	uid := id.UserID("@retreat-wired:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}

	forceExtractExpeditionForRunLoss(uid, lossCombatRetreat)

	if got := queuedCount(t, "retreat:%"); got != 1 {
		t.Fatalf("the run-loss chokepoint queued %d retreat facts, want 1 — "+
			"emitRetreatNews passes its own unit tests but is not actually wired to the game", got)
	}
	f := retreatFactFor(t, uid)
	// The day must survive the extract. Read it after forcedExtractExpedition and
	// it is gone — the row is 'abandoned' and the live fields are zeroed.
	if f["count"] != float64(exp.CurrentDay) {
		t.Errorf("count = %v, want %d — the day count was read after the extract wiped it",
			f["count"], exp.CurrentDay)
	}
	// And the expedition really did end; a bulletin about a run still in progress
	// would be worse than no bulletin.
	if still, _ := getActiveExpedition(uid); still != nil {
		t.Error("expedition still active after a run-loss extract")
	}
}

// The opt-out covers the new event type too. A player who asked not to be named
// must not be named when they lose — that is precisely when they'd mind most.
func TestRetreatNews_HonoursOptOut(t *testing.T) {
	setupEmptyTestDB(t)
	enablePeteSeam(t)
	uid := id.UserID("@retreat-shy:example.org")
	zoneCmdTestCharacter(t, uid, 10)
	setNewsOptout(uid, true)

	emitRetreatNews(uid, lossCombatRetreat, ZoneID("underforge"), 2)

	f := retreatFactFor(t, uid)
	if f["subject"] != anonName {
		t.Fatalf("subject = %v, want %q — the opt-out must cover retreats", f["subject"], anonName)
	}
	actors, _ := f["actors"].([]any)
	for _, a := range actors {
		if a != anonName {
			t.Fatalf("actors leaked %v past the opt-out", a)
		}
	}
}
