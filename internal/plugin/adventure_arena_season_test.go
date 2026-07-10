package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func TestArenaSeasonKeyAndBounds(t *testing.T) {
	tests := []struct {
		when      time.Time
		key       string
		wantStart time.Time
	}{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "2026-Q1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC), "2026-Q1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "2026-Q2", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC), "2026-Q3", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), "2026-Q4", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		if got := arenaSeasonKey(tc.when); got != tc.key {
			t.Errorf("arenaSeasonKey(%s) = %s, want %s", tc.when, got, tc.key)
		}
		start, end := arenaSeasonBounds(tc.when)
		if !start.Equal(tc.wantStart) {
			t.Errorf("season start for %s = %s, want %s", tc.when, start, tc.wantStart)
		}
		if !end.Equal(start.AddDate(0, 3, 0)) {
			t.Errorf("season end for %s is not start+3mo", tc.when)
		}
		if !tc.when.Before(end) || tc.when.Before(start) {
			t.Errorf("%s does not fall inside its own season bounds", tc.when)
		}
	}
}

// TestPreviousArenaSeasonWrapsYear pins the Q1 → prior-year-Q4 edge.
func TestPreviousArenaSeasonWrapsYear(t *testing.T) {
	key, start, end := previousArenaSeason(time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC))
	if key != "2025-Q4" {
		t.Errorf("previous season = %s, want 2025-Q4", key)
	}
	if !start.Equal(time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("start = %s, want 2025-10-01", start)
	}
	if !end.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("end = %s, want 2026-01-01", end)
	}
}

func insertArenaRun(t *testing.T, user id.UserID, tier, rounds int, earnings int64, outcome string, at time.Time) {
	t.Helper()
	_, err := db.Get().Exec(
		`INSERT INTO arena_history (user_id, start_tier, tier, rounds_survived, earnings, outcome, monster_name, created_at)
		 VALUES (?, 1, ?, ?, ?, ?, 'Test Monster', ?)`,
		string(user), tier, rounds, earnings, outcome, at.Unix())
	if err != nil {
		t.Fatal(err)
	}
}

// TestArenaSeasonLeaderboardWindowing: only runs inside the season window count.
func TestArenaSeasonLeaderboardWindowing(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	inSeason := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	start, end := arenaSeasonBounds(inSeason)

	alice := id.UserID("@alice:test.invalid")
	bob := id.UserID("@bob:test.invalid")

	insertArenaRun(t, alice, 3, 4, 5000, "completed", inSeason)
	insertArenaRun(t, alice, 5, 2, 1000, "dead", inSeason.AddDate(0, 0, 1))
	insertArenaRun(t, bob, 2, 3, 9000, "cashed_out", inSeason)
	// Last season — must not appear.
	insertArenaRun(t, alice, 5, 4, 999999, "completed", start.AddDate(0, 0, -1))

	entries, err := loadArenaSeasonLeaderboard(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Bob out-earned Alice this season (9000 vs 6000).
	if entries[0].DisplayName != string(bob) {
		t.Errorf("top entry = %s, want bob", entries[0].DisplayName)
	}
	if entries[0].TotalEarnings != 9000 {
		t.Errorf("bob earnings = %d, want 9000", entries[0].TotalEarnings)
	}
	if entries[1].TotalEarnings != 6000 {
		t.Errorf("alice earnings = %d, want 6000 (last season's 999999 must not count)", entries[1].TotalEarnings)
	}
	if entries[1].TotalDeaths != 1 {
		t.Errorf("alice deaths = %d, want 1", entries[1].TotalDeaths)
	}
	if entries[1].HighestTier != 5 {
		t.Errorf("alice highest tier = %d, want 5", entries[1].HighestTier)
	}
}

// TestArenaSeasonChampions: earnings crown goes by total, streak crown by the
// single longest run — they can be different players.
func TestArenaSeasonChampions(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	when := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	start, end := arenaSeasonBounds(when)

	rich := id.UserID("@rich:test.invalid")
	tough := id.UserID("@tough:test.invalid")

	insertArenaRun(t, rich, 5, 2, 50000, "cashed_out", when)
	insertArenaRun(t, tough, 1, 12, 500, "completed", when)

	uid, val, ok := arenaSeasonChampion(arenaTitleEarnings, start, end)
	if !ok || uid != rich || val != 50000 {
		t.Errorf("earnings champion = (%s, %d, %v), want rich/50000", uid, val, ok)
	}
	uid, val, ok = arenaSeasonChampion(arenaTitleStreak, start, end)
	if !ok || uid != tough || val != 12 {
		t.Errorf("streak champion = (%s, %d, %v), want tough/12", uid, val, ok)
	}

	// An empty season crowns nobody.
	emptyStart, emptyEnd := arenaSeasonBounds(when.AddDate(1, 0, 0))
	if _, _, ok := arenaSeasonChampion(arenaTitleEarnings, emptyStart, emptyEnd); ok {
		t.Error("an empty season produced an earnings champion")
	}

	// A season of nothing but round-zero deaths crowns no streak.
	deathsOnly := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	dStart, dEnd := arenaSeasonBounds(deathsOnly)
	insertArenaRun(t, tough, 1, 0, 0, "dead", deathsOnly)
	if _, _, ok := arenaSeasonChampion(arenaTitleStreak, dStart, dEnd); ok {
		t.Error("a season of round-zero deaths produced a streak champion")
	}
}

// TestRecordArenaSeasonTitleIdempotent: the rollover must be safe to re-run.
func TestRecordArenaSeasonTitleIdempotent(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	winner := id.UserID("@winner:test.invalid")
	usurper := id.UserID("@usurper:test.invalid")

	if err := recordArenaSeasonTitle("2026-Q2", arenaTitleEarnings, winner, 100, now); err != nil {
		t.Fatal(err)
	}
	// A second write for the same (season, kind) must not overwrite the crown.
	if err := recordArenaSeasonTitle("2026-Q2", arenaTitleEarnings, usurper, 999, now); err != nil {
		t.Fatal(err)
	}

	titles, err := loadArenaSeasonTitles(winner)
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 {
		t.Fatalf("winner has %d titles, want 1", len(titles))
	}
	stolen, _ := loadArenaSeasonTitles(usurper)
	if len(stolen) != 0 {
		t.Errorf("usurper took the crown: %v", stolen)
	}
}
