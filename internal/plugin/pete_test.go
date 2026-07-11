package plugin

import (
	"os"
	"testing"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// enablePeteSeam turns on the emit seam for a test and restores it to disabled
// afterwards, so a leaked-enabled singleton can't affect other tests.
func enablePeteSeam(t *testing.T) {
	t.Helper()
	os.Setenv("FEATURE_PETE_NEWS", "true")
	os.Setenv("PETE_INGEST_URL", "http://127.0.0.1:0")
	os.Setenv("PETE_INGEST_TOKEN", "tok")
	peteclient.Init()
	t.Cleanup(func() {
		os.Unsetenv("FEATURE_PETE_NEWS")
		os.Unsetenv("PETE_INGEST_URL")
		os.Unsetenv("PETE_INGEST_TOKEN")
		peteclient.Init()
	})
}

func queuedCount(t *testing.T, likeGUID string) int {
	t.Helper()
	var n int
	if err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM pete_emit_queue WHERE guid LIKE ?`, likeGUID).Scan(&n); err != nil {
		t.Fatalf("queue count: %v", err)
	}
	return n
}

// TestNewsOptoutRoundTrip: opting out records the player, opting in clears them,
// both idempotently.
func TestNewsOptoutRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	u := id.UserID("@zapp:example.org")
	if isNewsOptedOut(u) {
		t.Fatal("fresh player should not be opted out")
	}

	setNewsOptout(u, true)
	if !isNewsOptedOut(u) {
		t.Error("opt-out not recorded")
	}
	setNewsOptout(u, true) // idempotent
	if !isNewsOptedOut(u) {
		t.Error("second opt-out flipped state")
	}

	setNewsOptout(u, false)
	if isNewsOptedOut(u) {
		t.Error("opt-in did not clear")
	}
	setNewsOptout(u, false) // idempotent
	if isNewsOptedOut(u) {
		t.Error("second opt-in flipped state")
	}
}

// TestPeteNewsBackfill: the one-shot replays deaths + solo achievements + zone
// realm-firsts through the emit queue, seeds the realm-first ledger, and is
// idempotent on a second boot.
func TestPeteNewsBackfill(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)
	enablePeteSeam(t)

	// A named, dead adventurer.
	db.Exec("seed death", `INSERT INTO player_meta (user_id, display_name, last_death_date, death_location)
		VALUES (?, ?, ?, ?)`, "@zapp:x", "Zapp", "2026-06-01", "the Underforge")
	// A single-holder achievement.
	db.Exec("seed achv", `INSERT INTO achievements (user_id, achievement_id, unlocked_at)
		VALUES (?, ?, ?)`, "@zapp:x", "adv_first_blood", 1717200000)
	// A completed, boss-defeated zone run → realm-first.
	db.Exec("seed run", `INSERT INTO dnd_zone_run (run_id, user_id, zone_id, total_rooms, boss_defeated, completed_at)
		VALUES (?, ?, ?, ?, 1, ?)`, "r1", "@zapp:x", "goblin_warren", 5, "2026-05-20 12:00:00")

	p := &AdventurePlugin{}
	p.bootstrapPeteNewsBackfill()

	if got := queuedCount(t, "death:%"); got != 1 {
		t.Errorf("death dispatches queued = %d, want 1", got)
	}
	if got := queuedCount(t, "achv:%"); got != 1 {
		t.Errorf("achievement dispatches queued = %d, want 1", got)
	}
	if got := queuedCount(t, "zone:%"); got != 1 {
		t.Errorf("zone-first dispatches queued = %d, want 1", got)
	}
	// Realm-first ledger seeded so a later live clear tiers as a repeat.
	if claimRealmFirst("zone", "goblin_warren") {
		t.Error("realm-first ledger not seeded — a live clear would mis-announce as first-ever")
	}

	// Second boot: job gate holds, nothing re-queued.
	p.bootstrapPeteNewsBackfill()
	if got := queuedCount(t, "%"); got != 3 {
		t.Errorf("total queued after re-run = %d, want 3 (idempotent)", got)
	}
}

// TestNewsEmissionKillSwitch: the runtime flag defaults on and persists a flip.
func TestNewsEmissionKillSwitch(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	if !newsEmissionOn() {
		t.Error("emission should default ON")
	}
	setNewsEmission(false)
	if newsEmissionOn() {
		t.Error("emission still ON after disable")
	}
	setNewsEmission(true)
	if !newsEmissionOn() {
		t.Error("emission still OFF after enable")
	}
}
