package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
)

// ledgerFirstAt reads a zone's recorded claim time, or -1 if the ledger has no
// row for it at all.
func ledgerFirstAt(t *testing.T, zoneID string) int64 {
	t.Helper()
	var at int64
	err := db.Get().QueryRow(
		`SELECT first_at FROM news_realm_firsts WHERE kind = 'zone' AND target = ?`,
		zoneID).Scan(&at)
	if err != nil {
		return -1
	}
	return at
}

func unixOf(t *testing.T, sqliteTime string) int64 {
	t.Helper()
	ts, ok := parseSQLiteTime(sqliteTime)
	if !ok {
		t.Fatalf("parseSQLiteTime(%q) failed", sqliteTime)
	}
	return ts.Unix()
}

// TestReseedClaimsAZoneWhoseClearsWereAllRetired is the regression for the bug
// that made this job necessary: every clear of forest_shadows carries
// abandoned = 1, which is how the game stores a kill the expedition walked on
// from, and the original one-shot's `abandoned = 0` filter therefore never saw
// the zone at all. An unclaimed zone is a spurious PRIORITY "realm first"
// waiting to fire the next time somebody clears it, months after the fact — so
// the assertion that matters is the claimRealmFirst one at the end.
func TestReseedClaimsAZoneWhoseClearsWereAllRetired(t *testing.T) {
	seedRealmFixture(t)

	db.Exec("seed retired-only zone", `INSERT INTO dnd_zone_run
		(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
		VALUES ('r7', '@josie:x', 'forest_shadows', 6, 1, 1, '2026-02-14 09:00:00')`)

	if got := ledgerFirstAt(t, "forest_shadows"); got != -1 {
		t.Fatalf("forest_shadows already claimed before the reseed (first_at=%d) — fixture drift", got)
	}

	bootstrapRealmFirstsReseed()

	if got := ledgerFirstAt(t, "forest_shadows"); got != unixOf(t, "2026-02-14 09:00:00") {
		t.Errorf("forest_shadows first_at = %d, want %d (the real clear, not the minute the job ran)",
			got, unixOf(t, "2026-02-14 09:00:00"))
	}
	// The point of the whole job. Before the reseed this returns true and the
	// next clear of a zone beaten in February announces itself as a realm first.
	if claimRealmFirst("zone", "forest_shadows") {
		t.Error("forest_shadows was still unclaimed after the reseed — the next clear would fire a spurious realm-first")
	}
}

// TestReseedCorrectsTheBackfillsDates pins the second half. claimRealmFirst
// stamps unixepoch(), so every row the original one-shot wrote carries the one
// minute that job ran — in prod, all six share the identical timestamp. The
// reseed has to overwrite an existing row, not INSERT OR IGNORE past it.
func TestReseedCorrectsTheBackfillsDates(t *testing.T) {
	seedRealmFixture(t)

	// The fixture claims both zones the way the backfill did: at claim time.
	before := ledgerFirstAt(t, "goblin_warrens")
	if before < time.Now().Unix()-300 {
		t.Fatalf("fixture claim for goblin_warrens is not a now-stamp (%d) — fixture drift", before)
	}

	bootstrapRealmFirstsReseed()

	// Josie's r1, January, not r2 or r3 and not today.
	if got, want := ledgerFirstAt(t, "goblin_warrens"), unixOf(t, "2026-01-10 12:00:00"); got != want {
		t.Errorf("goblin_warrens first_at = %d, want %d (earliest real clear)", got, want)
	}
	// crypt_valdris has a clean clear (r4) and a retired-but-won one (r5). The
	// earliest is r4.
	if got, want := ledgerFirstAt(t, "crypt_valdris"), unixOf(t, "2026-05-01 12:00:00"); got != want {
		t.Errorf("crypt_valdris first_at = %d, want %d", got, want)
	}
}

// TestReseedEmitsNothing is the reason this is a re-seed and not a re-run of the
// backfill. The ledger has to be repaired without any historical realm-first
// dispatch reaching the room; a zone beaten in February is not news in July.
func TestReseedEmitsNothing(t *testing.T) {
	seedRealmFixture(t)
	db.Exec("seed retired-only zone", `INSERT INTO dnd_zone_run
		(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
		VALUES ('r7', '@josie:x', 'forest_shadows', 6, 1, 1, '2026-02-14 09:00:00')`)

	bootstrapRealmFirstsReseed()

	var queued int
	if err := db.Get().QueryRow(`SELECT COUNT(*) FROM pete_emit_queue`).Scan(&queued); err != nil {
		t.Fatalf("count pete_emit_queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("reseed queued %d dispatches, want 0 — the ledger repair must be silent", queued)
	}
}

// TestReseedIsAOneShot. It is a bootstrap kept in place for fresh deploys (per
// feedback_loader_rewire_needs_bootstrap), so it runs on every start and must
// cost nothing after the first — and, more importantly, must not undo a
// later live claim by rewriting the ledger from stale history on every boot.
func TestReseedIsAOneShot(t *testing.T) {
	seedRealmFixture(t)
	bootstrapRealmFirstsReseed()

	// A zone cleared after the reseed, claimed live.
	if !claimRealmFirst("zone", "sunken_temple") {
		t.Fatal("sunken_temple should have been an unclaimed realm-first")
	}
	live := ledgerFirstAt(t, "sunken_temple")

	bootstrapRealmFirstsReseed()

	if got := ledgerFirstAt(t, "sunken_temple"); got != live {
		t.Errorf("second reseed moved a live claim: %d -> %d", live, got)
	}
	if claimRealmFirst("zone", "goblin_warrens") {
		t.Error("second reseed dropped an existing claim")
	}
}

// TestZoneFirstClearsCountsRetiredKills guards the shared query itself, which
// the kept backfill also uses. `abandoned` means the run row was retired, not
// that anybody gave up.
func TestZoneFirstClearsCountsRetiredKills(t *testing.T) {
	seedRealmFixture(t)
	db.Exec("seed retired-only zone", `INSERT INTO dnd_zone_run
		(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
		VALUES ('r7', '@josie:x', 'forest_shadows', 6, 1, 1, '2026-02-14 09:00:00')`)

	byZone := map[string]zoneFirstClear{}
	for _, f := range zoneFirstClears() {
		byZone[f.zoneID] = f
	}
	if len(byZone) != 3 {
		t.Fatalf("zoneFirstClears returned %d zones, want 3 (a regression to `abandoned = 0` gives 2)", len(byZone))
	}
	if got := byZone["forest_shadows"].userID; got != "@josie:x" {
		t.Errorf("forest_shadows first clearer = %q, want @josie:x", got)
	}
	if _, ok := byZone["arena"]; ok {
		t.Error("an unfinished run counted as a clear")
	}
}
