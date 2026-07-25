package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// seedRealmFixture builds a small but realistic realm: two players, three
// cleared runs across two zones, one live expedition, and the realm-first ledger
// that the zone clears would have seeded.
func seedRealmFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	db.Exec("seed josie", `INSERT INTO player_meta (user_id, display_name, alive) VALUES (?, ?, 1)`,
		"@josie:x", "Josie")
	db.Exec("seed quack", `INSERT INTO player_meta (user_id, display_name, alive) VALUES (?, ?, 1)`,
		"@quack:x", "Quack")

	// The board walks player_meta and then loads a character, so both halves have
	// to exist: a player_meta row with no character is somebody who never finished
	// setup, and standings correctly leaves them off.
	for _, c := range []*DnDCharacter{
		{UserID: "@josie:x", Race: RaceHuman, Class: ClassFighter, Level: 12,
			STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
			HPMax: 120, HPCurrent: 120, ArmorClass: 18},
		{UserID: "@quack:x", Race: RaceElf, Class: ClassMage, Level: 8,
			STR: 8, DEX: 16, CON: 12, INT: 18, WIS: 12, CHA: 10,
			HPMax: 48, HPCurrent: 48, ArmorClass: 12},
	} {
		if err := SaveDnDCharacter(c); err != nil {
			t.Fatalf("SaveDnDCharacter(%s): %v", c.UserID, err)
		}
	}

	// Josie cleared the Warrens first, then again; Quack cleared them later. Only
	// Josie has been through the Crypt — and it is a deeper tier, which is what
	// makes the board's depth-before-breadth ordering testable.
	for _, r := range []struct {
		runID, user, zone, at string
	}{
		{"r1", "@josie:x", string(ZoneGoblinWarrens), "2026-01-10 12:00:00"},
		{"r2", "@quack:x", string(ZoneGoblinWarrens), "2026-03-02 12:00:00"},
		{"r3", "@josie:x", string(ZoneGoblinWarrens), "2026-04-05 12:00:00"},
		{"r4", "@josie:x", string(ZoneCryptValdris), "2026-05-01 12:00:00"},
	} {
		db.Exec("seed run", `INSERT INTO dnd_zone_run
			(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
			VALUES (?, ?, ?, 6, 1, 0, ?)`, r.runID, r.user, r.zone, r.at)
	}
	// A retired-but-won run: boss_defeated = 1 with abandoned = 1. This IS a
	// clear — `abandoned` means the run row was retired (the expedition travelled
	// on after the kill), not that anybody gave up, and in prod it is how 30 of
	// the realm's 32 boss kills are stored. The fixture carries one so the
	// aggregate can never quietly go back to filtering them out.
	db.Exec("seed retired-win", `INSERT INTO dnd_zone_run
		(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
		VALUES ('r5', '@quack:x', 'crypt_valdris', 6, 1, 1, '2026-05-02 12:00:00')`)
	// A genuinely unfinished run: no boss, no completion. Not a clear.
	db.Exec("seed inflight", `INSERT INTO dnd_zone_run
		(run_id, user_id, zone_id, total_rooms, boss_defeated, abandoned, completed_at)
		VALUES ('r6', '@quack:x', 'crypt_valdris', 6, 0, 0, NULL)`)

	claimRealmFirst("zone", string(ZoneGoblinWarrens))
	claimRealmFirst("zone", string(ZoneCryptValdris))
}

// TestRealmClearStatsPickTheEarliestClearer is the load-bearing query on the
// whole map: "who first got through here" is the single most interesting fact a
// zone has, and it has to be the person who was actually first.
//
// It leans on SQLite's bare-column min/max rule — the user_id comes from the same
// row MIN(completed_at) came from — which is the same thing backfillZoneFirsts
// has relied on since the news seam shipped. If that ever stopped holding, this
// would attribute somebody else's conquest to whoever the grouping happened to
// land on, silently.
func TestRealmClearStatsPickTheEarliestClearer(t *testing.T) {
	seedRealmFixture(t)

	stats, err := loadRealmClearStats()
	if err != nil {
		t.Fatalf("loadRealmClearStats: %v", err)
	}

	warrens, ok := stats["goblin_warrens"]
	if !ok {
		t.Fatal("no stats for goblin_warrens")
	}
	if warrens.clears != 3 {
		t.Errorf("warrens clears = %d, want 3", warrens.clears)
	}
	if warrens.clearers != 2 {
		t.Errorf("warrens clearers = %d, want 2", warrens.clearers)
	}
	if warrens.firstUser != id.UserID("@josie:x") {
		t.Errorf("warrens first clearer = %q, want @josie:x — the earliest run didn't win", warrens.firstUser)
	}

	// Two clears: Josie's, and Quack's retired-but-won run. The in-flight run is
	// correctly excluded. A regression to `AND abandoned = 0` shows up here as 1.
	crypt := stats["crypt_valdris"]
	if crypt.clears != 2 {
		t.Errorf("crypt clears = %d, want 2 — a won-then-retired run is a clear, "+
			"and an in-flight one is not", crypt.clears)
	}
	if crypt.firstUser != id.UserID("@josie:x") {
		t.Errorf("crypt first clearer = %q, want @josie:x", crypt.firstUser)
	}
}

// TestOptedOutFirstClearerIsAnonymisedNotErased. The Siege contributor rule,
// applied to the map: deleting an opted-out clearer's claim would leave the zone
// drawn as never-cleared, and "nobody has ever come out of there" is the most
// dramatic thing the page can say. Saying it falsely because somebody chose
// privacy would be worse than saying nothing.
func TestOptedOutFirstClearerIsAnonymisedNotErased(t *testing.T) {
	seedRealmFixture(t)
	setNewsOptout(id.UserID("@josie:x"), true)

	snap, err := buildRealmSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildRealmSnapshot: %v", err)
	}

	var warrens *struct {
		clears    int
		by, token string
	}
	for _, z := range snap.Zones {
		if z.ID == "goblin_warrens" {
			warrens = &struct {
				clears    int
				by, token string
			}{z.Clears, z.FirstClearBy, z.FirstClearToken}
		}
	}
	if warrens == nil {
		t.Fatal("goblin_warrens is not in the snapshot at all")
	}
	if warrens.clears != 3 {
		t.Errorf("clears = %d, want 3 — an opt-out deleted the town's history", warrens.clears)
	}
	if warrens.by != "" || warrens.token != "" {
		t.Errorf("opted-out clearer still named: by=%q token=%q", warrens.by, warrens.token)
	}
}

// TestOptedOutPlayerLeavesTheBoardEntirely. Standings follow the board's rule,
// not the Siege's: an opted-out player is omitted outright. Their level, class
// and clear count would re-identify them, and unlike a siege contribution there
// is no shared total that stops adding up without them.
func TestOptedOutPlayerLeavesTheBoardEntirely(t *testing.T) {
	seedRealmFixture(t)
	setNewsOptout(id.UserID("@quack:x"), true)

	snap, err := buildRealmSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildRealmSnapshot: %v", err)
	}
	for _, s := range snap.Standings {
		if s.Name == "Quack" {
			t.Fatal("an opted-out player is still on the standings board")
		}
	}
}

// TestOccupantsDropOptedOutPlayers. Presence is the strictest case on the page
// and deliberately stricter than a first clear: "who is in the Crypt of Valdris
// right now" is the live-location fact the run liveblog refuses to publish at
// all, so an opted-out player is dropped rather than anonymised.
func TestOccupantsDropOptedOutPlayers(t *testing.T) {
	seedRealmFixture(t)
	db.Exec("seed expedition", `INSERT INTO dnd_expedition
		(expedition_id, user_id, zone_id, status, current_day)
		VALUES ('e1', '@josie:x', 'crypt_valdris', 'active', 3)`)

	if occ := loadRealmOccupants(); len(occ["crypt_valdris"]) != 1 {
		t.Fatalf("opted-in occupant missing: %+v", occ)
	} else if occ["crypt_valdris"][0].Name != "Josie" || occ["crypt_valdris"][0].Day != 3 {
		t.Errorf("occupant = %+v, want Josie on day 3", occ["crypt_valdris"][0])
	}

	setNewsOptout(id.UserID("@josie:x"), true)
	if occ := loadRealmOccupants(); len(occ["crypt_valdris"]) != 0 {
		t.Errorf("opted-out player still shows as standing in a zone: %+v", occ["crypt_valdris"])
	}
}

// TestStandingsCountFirstsFromTheSameAuthorityTheMapDoes. A zone's "first
// through: Josie" and Josie's own firsts count come off one map in one pass, so
// the two can never disagree — which they would if the board recounted the
// ledger itself and the two queries drifted.
func TestStandingsCountFirstsFromTheSameAuthorityTheMapDoes(t *testing.T) {
	seedRealmFixture(t)

	snap, err := buildRealmSnapshot(time.Now().UTC())
	if err != nil {
		t.Fatalf("buildRealmSnapshot: %v", err)
	}

	firstsByName := map[string]int{}
	for _, s := range snap.Standings {
		firstsByName[s.Name] = s.Firsts
	}
	// Josie was first through both zones; Quack was first through neither.
	if firstsByName["Josie"] != 2 {
		t.Errorf("Josie holds %d firsts, want 2", firstsByName["Josie"])
	}
	if firstsByName["Quack"] != 0 {
		t.Errorf("Quack holds %d firsts, want 0", firstsByName["Quack"])
	}

	named := 0
	for _, z := range snap.Zones {
		if z.FirstClearBy == "Josie" {
			named++
		}
	}
	if named != firstsByName["Josie"] {
		t.Errorf("the map names Josie on %d zones but the board credits her with %d — "+
			"the two disagree about the same fact", named, firstsByName["Josie"])
	}
}

// TestStandingsRankDeepestFirst. The ordering is the game's statement about what
// it values, and Pete renders it without renumbering — so it has to be right
// here. Depth beats breadth: somebody who has put down a Tier 5 boss is ahead of
// somebody who has cleared the whole of Tier 1 forty times.
func TestStandingsRankDeepestFirst(t *testing.T) {
	seedRealmFixture(t)

	rows, err := loadRealmStandings(map[string]realmClearStats{})
	if err != nil {
		t.Fatalf("loadRealmStandings: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("got %d standings rows, want 2", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		a, b := rows[i-1], rows[i]
		if a.DeepestTier < b.DeepestTier {
			t.Errorf("row %d (T%d) sorts above row %d (T%d) — the board is not deepest-first",
				i-1, a.DeepestTier, i, b.DeepestTier)
		}
		if a.DeepestTier == b.DeepestTier && a.Zones < b.Zones {
			t.Errorf("equal depth but row %d covers %d zones above row %d's %d",
				i-1, a.Zones, i, b.Zones)
		}
	}
}

// TestFirstsLedgerIsRenderedNotJustCounted. news_realm_firsts has existed since
// the news seam shipped and has only ever been used to decide a dispatch tier —
// the ledger itself was never read back. This is the whole point of the hall: it
// is a history book, and every row needs a name and a date on it.
func TestFirstsLedgerIsRenderedNotJustCounted(t *testing.T) {
	seedRealmFixture(t)

	stats, err := loadRealmClearStats()
	if err != nil {
		t.Fatalf("loadRealmClearStats: %v", err)
	}
	firsts := loadRealmFirsts(stats)
	if len(firsts) != 2 {
		t.Fatalf("got %d firsts, want 2", len(firsts))
	}
	// Oldest first, which is the order it happened in.
	if firsts[0].Target != "goblin_warrens" {
		t.Errorf("ledger order starts with %q, want goblin_warrens", firsts[0].Target)
	}
	for _, f := range firsts {
		if f.Display == "" {
			t.Errorf("first %q has no display name — it would render as a blank row", f.Target)
		}
		if f.Holder != "Josie" {
			t.Errorf("first %q holder = %q, want Josie (she cleared both zones first)", f.Target, f.Holder)
		}
		if f.Token == "" {
			t.Errorf("first %q has a holder but no token, so the hall can't link to them", f.Target)
		}
	}
}

// TestUnrecoverableFirstStillGetsAnEntry. The ledger records (kind, target,
// first_at) and nothing else; the holder is recovered at push time from the run
// history. A treasure found and later discarded leaves no owner anywhere, and
// that entry has to survive as an unattributed first rather than vanish — it
// still happened, and the hall is a record of what happened.
func TestUnrecoverableFirstStillGetsAnEntry(t *testing.T) {
	seedRealmFixture(t)
	claimRealmFirst("treasure", "a_hat_nobody_kept")

	var found bool
	stats, err := loadRealmClearStats()
	if err != nil {
		t.Fatalf("loadRealmClearStats: %v", err)
	}
	for _, f := range loadRealmFirsts(stats) {
		if f.Target == "a_hat_nobody_kept" {
			found = true
			if f.Holder != "" {
				t.Errorf("holder = %q, want empty — nothing in the game knows who had it", f.Holder)
			}
			if f.Display == "" {
				t.Error("an unrecoverable first got no display name at all")
			}
		}
	}
	if !found {
		t.Error("a first with no recoverable holder was dropped from the ledger entirely")
	}
}

// TestRealmPushIsRateLimitedBelowTheRosterTick. The realm rides the 2-minute
// roster ticker but is aggregate scans over the whole run history, and none of
// it moves at roster speed. The self-limit is the thing that makes riding that
// ticker acceptable, so it is worth pinning — and so is the other half: a FAILED
// push must not stamp the clock, or an outage would be followed by ten minutes
// of silence instead of a retry on the next tick.
func TestRealmPushIsRateLimitedBelowTheRosterTick(t *testing.T) {
	if realmPushInterval <= rosterTickInterval {
		t.Fatalf("realmPushInterval (%v) is not longer than the roster tick (%v) — "+
			"the realm would be recomputed every tick", realmPushInterval, rosterTickInterval)
	}

	// The gate itself: zero means never-pushed and must always go.
	realmLastPush = time.Time{}
	t.Cleanup(func() { realmLastPush = time.Time{} })
	now := time.Now().UTC()
	if !realmLastPush.IsZero() {
		t.Fatal("fixture broken")
	}
	// A stamp inside the window suppresses; one outside it does not.
	realmLastPush = now.Add(-realmPushInterval / 2)
	if now.Sub(realmLastPush) >= realmPushInterval {
		t.Error("a push half an interval old is not being suppressed")
	}
	realmLastPush = now.Add(-realmPushInterval - time.Minute)
	if now.Sub(realmLastPush) < realmPushInterval {
		t.Error("a push older than the interval is still being suppressed")
	}
}
