package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// seedZoneRun inserts an active zone run owned by the given user. The columns
// activeZoneRunFor reads past the CREATE TABLE block (current_node, …) all carry
// defaults, so the five below are enough.
func seedZoneRun(t *testing.T, runID string, owner id.UserID) {
	t.Helper()
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_zone_run (run_id, user_id, zone_id, total_rooms, last_action_at)
		VALUES (?, ?, 'goblin_warrens', 8, ?)`,
		runID, string(owner), time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
}

// A solo expedition must resolve exactly as it did before N3: the owner's own
// run, flagged as theirs. This is the path every existing player is on.
func TestActiveZoneRunFor_SoloOwnsItsRun(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", owner, "active")
	seedZoneRun(t, "run-exp-solo", owner)

	run, isLeader, err := activeZoneRunFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.RunID != "run-exp-solo" {
		t.Fatalf("run = %v, want run-exp-solo", run)
	}
	if !isLeader {
		t.Error("solo player is not reported as owning their run")
	}
}

// The whole point of the seam: a member owns no dnd_zone_run row, and
// getActiveZoneRun therefore tells them they are standing nowhere.
func TestActiveZoneRunFor_MemberRidesTheLeadersRun(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	seedZoneRun(t, "run-exp-1", leader)
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}

	// The old lookup is blind to them — this is the bug being fixed.
	if r, err := getActiveZoneRun(member); err != nil || r != nil {
		t.Fatalf("getActiveZoneRun(member) = %v, %v; want nil, nil", r, err)
	}

	run, isLeader, err := activeZoneRunFor(member)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.RunID != "run-exp-1" {
		t.Fatalf("member resolved run = %v, want run-exp-1", run)
	}
	if isLeader {
		t.Error("member reported as leader")
	}

	// And the leader still resolves the same run, still as its owner.
	run, isLeader, err = activeZoneRunFor(leader)
	if err != nil || run == nil || run.RunID != "run-exp-1" || !isLeader {
		t.Fatalf("leader resolved (%v, %v, %v), want run-exp-1 as leader", run, isLeader, err)
	}
}

// A finished or abandoned run must not resolve for a member either. Without the
// IsActive filter getZoneRun happily hands back a corpse — it fetches by id
// "regardless of completion state".
func TestActiveZoneRunFor_MemberGetsNilForDeadRun(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	seedZoneRun(t, "run-exp-1", leader)
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET abandoned = 1 WHERE run_id = 'run-exp-1'`); err != nil {
		t.Fatal(err)
	}

	run, _, err := activeZoneRunFor(member)
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Errorf("member resolved an abandoned run: %v", run)
	}
}

// A member must never trigger the §4.3 idle reap on the leader's behalf. The
// reap lives inside getActiveZoneRun and force-extracts the wrapping expedition;
// a member glancing at the map would otherwise end everyone's run.
func TestActiveZoneRunFor_MemberDoesNotReapAStaleRun(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	seedZoneRun(t, "run-exp-1", leader)
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-2 * zoneRunInactivityTimeout)
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET last_action_at = ? WHERE run_id = 'run-exp-1'`, stale); err != nil {
		t.Fatal(err)
	}

	if _, _, err := activeZoneRunFor(member); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.Get().QueryRow(
		`SELECT status FROM dnd_expedition WHERE expedition_id = 'exp-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Errorf("member's lookup reaped the leader's expedition: status = %q", status)
	}
}

// The run is created lazily on the first walk, so a member who joins before the
// leader steps off gets a clean nil rather than an error.
func TestActiveZoneRunFor_MemberBeforeFirstWalk(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, run_id, status, start_date)
		VALUES ('exp-1', ?, 'goblin_warrens', NULL, 'active', ?)`,
		string(leader), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}

	run, isLeader, err := activeZoneRunFor(member)
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Errorf("run = %v, want nil before the first walk", run)
	}
	if isLeader {
		t.Error("member reported as leader")
	}
}

// ── the DM audience ──────────────────────────────────────────────────────────

// A solo expedition must resolve to exactly one recipient: the owner. Every
// briefing, recap and digest loops this, and a solo run is every run that has
// ever happened.
func TestExpeditionAudience_SoloIsTheOwnerAlone(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", owner, "active")
	e, err := getExpedition("exp-solo")
	if err != nil || e == nil {
		t.Fatal(err)
	}

	got := expeditionAudience(e)
	if len(got) != 1 || got[0] != owner {
		t.Errorf("audience = %v, want [%s]", got, owner)
	}
}

func TestExpeditionAudience_PartyIsLeaderFirst(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	a := id.UserID("@a:example.org")
	b := id.UserID("@b:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	for _, m := range []id.UserID{a, b} {
		if err := joinParty("exp-1", m); err != nil {
			t.Fatal(err)
		}
	}
	e, err := getExpedition("exp-1")
	if err != nil || e == nil {
		t.Fatal(err)
	}

	got := expeditionAudience(e)
	want := []id.UserID{leader, a, b}
	if len(got) != len(want) {
		t.Fatalf("audience = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audience = %v, want %v", got, want)
		}
	}
}

// ── the roster's lifetime ────────────────────────────────────────────────────

// Every terminal transition must free the members: assertNotAdventuring bars a
// seated player from starting anything of their own, so a roster that outlives
// its expedition strands the whole party.
func TestReleaseParty_TerminalTransitionsFreeTheMembers(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(t *testing.T, expID string, leader id.UserID)
	}{
		{"complete", func(t *testing.T, expID string, _ id.UserID) {
			if err := completeExpedition(expID, ExpeditionStatusComplete); err != nil {
				t.Fatal(err)
			}
		}},
		{"failed", func(t *testing.T, expID string, _ id.UserID) {
			if err := completeExpedition(expID, ExpeditionStatusFailed); err != nil {
				t.Fatal(err)
			}
		}},
		{"abandon", func(t *testing.T, _ string, leader id.UserID) {
			if err := abandonExpedition(leader); err != nil {
				t.Fatal(err)
			}
		}},
		{"forced-extract", func(t *testing.T, expID string, _ id.UserID) {
			if _, _, err := forcedExtractExpedition(expID, "starvation"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupEmptyTestDB(t)
			leader := id.UserID("@lead:example.org")
			member := id.UserID("@member:example.org")
			seedExpedition(t, "exp-1", leader, "active")
			if err := joinParty("exp-1", member); err != nil {
				t.Fatal(err)
			}

			tc.end(t, "exp-1", leader)

			members, err := partyMembers("exp-1")
			if err != nil {
				t.Fatal(err)
			}
			if len(members) != 0 {
				t.Fatalf("roster survived %s: %v", tc.name, members)
			}
			// The freed member can now be seated somewhere else.
			seedExpedition(t, "exp-2", id.UserID("@other:example.org"), "active")
			if err := joinParty("exp-2", member); err != nil {
				t.Errorf("member still stranded after %s: %v", tc.name, err)
			}
		})
	}
}

// Extraction is a seven-day resumable limbo, not a terminal status. The party
// must survive it so `!resume` brings everyone back.
func TestReleaseParty_ExtractingKeepsTheRoster(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}

	if _, err := voluntaryExtractExpedition(leader); err != nil {
		t.Fatal(err)
	}

	members, err := partyMembers("exp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("roster = %d after extraction, want 2 (resumable)", len(members))
	}
}
