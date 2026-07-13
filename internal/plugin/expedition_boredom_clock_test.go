package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// These exercise the idle clock against real rows.
//
// The hazard they exist for: modernc.org/sqlite rebuilds a time.Time from the
// column's *declared* type, and both COALESCE() and MAX() erase it. A Scan that
// silently fails here is not a small bug — playerIsIdle fails open (a player it
// can't read counts as idle), so a broken scan would declare the entire server
// idle and march everyone into a dungeon at once.

func seedBoredomPlayer(t *testing.T, uid id.UserID, created, lastAction, lastBoredom *time.Time) {
	t.Helper()
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, display_name, alive, created_at, last_player_action_at, last_boredom_at)
		 VALUES (?, ?, 1, ?, ?, ?)`,
		string(uid), "Test", created, lastAction, lastBoredom)
	if err != nil {
		t.Fatalf("seed player_meta: %v", err)
	}
}

func TestPlayerIsIdleReadsTheClock(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-72 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	seedBoredomPlayer(t, "@idle:test", &old, &old, nil)
	seedBoredomPlayer(t, "@active:test", &old, &recent, nil)
	// Never acted: the COALESCE falls through to created_at.
	seedBoredomPlayer(t, "@never:test", &old, nil, nil)
	seedBoredomPlayer(t, "@fresh:test", &recent, nil, nil)

	cases := []struct {
		uid  id.UserID
		want bool
		why  string
	}{
		{"@idle:test", true, "last acted 72h ago"},
		{"@active:test", false, "acted an hour ago"},
		{"@never:test", true, "never acted, created 72h ago — COALESCE to created_at"},
		{"@fresh:test", false, "never acted, but only just created — must not be yanked out on day one"},
	}
	for _, tc := range cases {
		if got := playerIsIdle(tc.uid, now); got != tc.want {
			t.Errorf("playerIsIdle(%s) = %v, want %v (%s)", tc.uid, got, tc.want, tc.why)
		}
	}
}

func TestLoadBoredomCandidatesRespectsClockAndCooldown(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-72 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	seedBoredomPlayer(t, "@idle:test", &old, &old, nil)
	seedBoredomPlayer(t, "@active:test", &old, &recent, nil)
	// Idle, but already sent out an hour ago — the cooldown holds them back.
	seedBoredomPlayer(t, "@cooling:test", &old, &old, &recent)
	// Idle, and their last outing was days ago — eligible again.
	seedBoredomPlayer(t, "@ready:test", &old, &old, &old)

	got, err := loadBoredomCandidates(now)
	if err != nil {
		t.Fatalf("loadBoredomCandidates: %v", err)
	}
	set := map[id.UserID]bool{}
	for _, u := range got {
		set[u] = true
	}
	for uid, want := range map[id.UserID]bool{
		"@idle:test":    true,
		"@ready:test":   true,
		"@active:test":  false,
		"@cooling:test": false,
	} {
		if set[uid] != want {
			t.Errorf("candidate %s: got %v, want %v", uid, set[uid], want)
		}
	}
}

func TestMarkPlayerActionResetsTheClock(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-72 * time.Hour)
	seedBoredomPlayer(t, "@back:test", &old, &old, nil)

	if !playerIsIdle("@back:test", now) {
		t.Fatal("precondition: player should start idle")
	}
	markPlayerAction("@back:test")
	if playerIsIdle("@back:test", now) {
		t.Error("player acted against Adventure but is still counted idle — the clock isn't being stamped")
	}
}

// TestIsBoredomDrivenNeedsBothHalves pins the rule the idle reaper depends on:
// only an expedition the player never asked for, from a player who still hasn't
// come back, loses its streak protection. A self-started expedition holds it.
func TestIsBoredomDrivenNeedsBothHalves(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-72 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	seedExp := func(t *testing.T, uid id.UserID, boredom bool) {
		t.Helper()
		_, err := db.Get().Exec(
			`INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, status, start_date, boredom)
			 VALUES (?, ?, 'goblin_warrens', 'active', ?, ?)`,
			string(uid)+"-exp", string(uid), old, boredom)
		if err != nil {
			t.Fatalf("seed expedition: %v", err)
		}
	}

	seedBoredomPlayer(t, "@bored:test", &old, &old, nil)
	seedExp(t, "@bored:test", true)

	seedBoredomPlayer(t, "@selfstart:test", &old, &old, nil)
	seedExp(t, "@selfstart:test", false)

	seedBoredomPlayer(t, "@returned:test", &old, &recent, nil)
	seedExp(t, "@returned:test", true)

	seedBoredomPlayer(t, "@noexp:test", &old, &old, nil)

	cases := []struct {
		uid  id.UserID
		want bool
		why  string
	}{
		{"@bored:test", true, "absent player, expedition it started itself — reap"},
		{"@selfstart:test", false, "absent, but they started this expedition themselves — the autopilot still holds their streak"},
		{"@returned:test", false, "bored expedition, but the player came back — don't reap someone who's playing"},
		{"@noexp:test", false, "no expedition history at all — don't reap on a guess"},
	}
	for _, tc := range cases {
		if got := isBoredomDriven(tc.uid, now); got != tc.want {
			t.Errorf("isBoredomDriven(%s) = %v, want %v (%s)", tc.uid, got, tc.want, tc.why)
		}
	}
}

// TestLastExpeditionByZoneScans covers the MAX(start_date) scan — the same
// type-affinity hazard as the COALESCE above, and the tie-breaker that stops a
// bored adventurer grinding one zone forever.
func TestLastExpeditionByZoneScans(t *testing.T) {
	newBoredomTestDB(t)
	uid := id.UserID("@hist:test")
	older := time.Now().UTC().Add(-96 * time.Hour)
	newer := time.Now().UTC().Add(-24 * time.Hour)

	for i, e := range []struct {
		zone string
		when time.Time
	}{
		{"goblin_warrens", older},
		{"goblin_warrens", newer},
		{"crypt_valdris", older},
	} {
		if _, err := db.Get().Exec(
			`INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, status, start_date)
			 VALUES (?, ?, ?, 'complete', ?)`,
			string(uid)+string(rune('a'+i)), string(uid), e.zone, e.when); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, err := lastExpeditionByZone(uid)
	if err != nil {
		t.Fatalf("lastExpeditionByZone: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d zones, want 2 — the MAX(start_date) scan is dropping rows: %v", len(got), got)
	}
	if !got[ZoneGoblinWarrens].After(got[ZoneCryptValdris]) {
		t.Errorf("goblin_warrens (%v) should be more recent than crypt_valdris (%v); "+
			"the picker relies on this to rotate zones", got[ZoneGoblinWarrens], got[ZoneCryptValdris])
	}
}
