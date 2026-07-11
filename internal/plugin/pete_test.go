package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
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
	if got := queuedCount(t, "milestone:%"); got != 1 {
		t.Errorf("achievement dispatches queued = %d, want 1", got)
	}
	if got := queuedCount(t, "zone_first:%"); got != 1 {
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

// TestEventToken: the public GUID token is salted (not a bare sha256 an attacker
// can recompute from the handle), stable per logical event (idempotency), and
// per-event so a user's tokens don't link to each other.
func TestEventToken(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	// The salt is cached process-wide via sync.Once; reset it so it derives (and
	// persists) against this test's fresh DB rather than an earlier test's.
	guidSaltOnce = sync.Once{}
	guidSalt = nil

	u := id.UserID("@alice:example.org")

	// Not the old unsalted digest: an attacker with the handle can't recompute it.
	bare := sha256.Sum256([]byte(u))
	if got := eventToken(u, "arrival"); got == hex.EncodeToString(bare[:9]) {
		t.Error("token matches unsalted sha256(handle) — enumeration attack reopened")
	}

	// Stable for the same (user, discriminator): a re-emit dedups.
	if eventToken(u, "arrival") != eventToken(u, "arrival") {
		t.Error("token not stable for the same event — idempotency broken")
	}

	// Per-event: two different events for the same user yield unrelated tokens, so
	// a named event can't be walked back to the user's anonymized events.
	if eventToken(u, "arrival") == eventToken(u, "death:123") {
		t.Error("tokens collide across events — linkage attack reopened")
	}

	// Different users, same discriminator, still differ.
	if eventToken(u, "arrival") == eventToken(id.UserID("@bob:example.org"), "arrival") {
		t.Error("token collides across users")
	}

	// The salt persisted, so it survives a restart (a re-emit after reboot dedups).
	var salt string
	if err := db.Get().QueryRow(`SELECT value FROM news_config WHERE key = ?`, newsGUIDSaltKey).Scan(&salt); err != nil || salt == "" {
		t.Fatalf("guid salt not persisted: %v", err)
	}
}

// TestEmitZoneClearTaxonomy: the realm's first clear of a zone emits a PRIORITY
// zone_first; a later clear emits a BULLETIN zone_clear — distinct event_type and
// GUID prefix, so Pete never files a repeat as a first-ever.
func TestEmitZoneClearTaxonomy(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)
	enablePeteSeam(t)

	db.Exec("seed player", `INSERT INTO player_meta (user_id, display_name) VALUES (?, ?)`,
		"@zapp:x", "Zapp")
	exp := &Expedition{ZoneID: "goblin_warren"}

	emitZoneClearNews(id.UserID("@zapp:x"), exp) // realm-first
	emitZoneClearNews(id.UserID("@zapp:x"), exp) // repeat

	if got := queuedCount(t, "zone_first:%"); got != 1 {
		t.Errorf("zone_first queued = %d, want 1 (the realm-first)", got)
	}
	if got := queuedCount(t, "zone_clear:%"); got != 1 {
		t.Errorf("zone_clear queued = %d, want 1 (the repeat)", got)
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
