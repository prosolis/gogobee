package plugin

import (
	"encoding/json"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Revisit R1 (gogobee_revisit_plan.md §R1) — the structural split between
// "where am I on the path" (CurrentRoom) and "how far have I walked"
// (RoomsTraversed). No player-visible behavior changes in R1; these tests
// pin the invariants R2's `!revisit` will lean on.

// setupRevisitTestDB builds a schema-only database in a temp dir. Unlike
// setupZoneRunTestDB it does not copy the operator's data/gogobee.db, so it
// runs everywhere instead of skipping on any machine that lacks one — these
// tests assert the R1 invariants, which no prod row is needed to exercise.
func setupRevisitTestDB(t *testing.T) {
	t.Helper()
	db.Close()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)
}

// ── pure helpers ────────────────────────────────────────────────────────────

func TestPathIndexOf_ReturnsFirstEntryIndex(t *testing.T) {
	// A backtracked path: the player walked r1→r2→r3, then back to r2.
	// r2's room number must stay 1 — the number they were shown on the way
	// in, and the salt every enemy/trap/harvest key downstream hashes.
	visited := []string{"z.r1", "z.r2", "z.r3"}
	if got := pathIndexOf(visited, "z.r2"); got != 1 {
		t.Errorf("pathIndexOf(r2) = %d, want 1", got)
	}
	if got := pathIndexOf(visited, "z.r3"); got != 2 {
		t.Errorf("pathIndexOf(r3) = %d, want 2", got)
	}
}

func TestPathIndexOf_FallsBackToTailWhenAbsent(t *testing.T) {
	// Pre-G4 rows carry a current_node that was never recorded as visited.
	// The fallback must reproduce the pre-R1 derivation (len-1), not 0 —
	// pinning such a row to the entry room would re-fire the entry encounter.
	visited := []string{"z.r1", "z.r2", "z.r3"}
	if got := pathIndexOf(visited, "z.nowhere"); got != 2 {
		t.Errorf("pathIndexOf(absent) = %d, want tail index 2", got)
	}
	if got := pathIndexOf(nil, "z.r1"); got != 0 {
		t.Errorf("pathIndexOf(empty) = %d, want 0", got)
	}
}

func TestAppendVisited_DoesNotRenumberOnRevisit(t *testing.T) {
	visited := []string{"z.r1", "z.r2"}
	again := appendVisited(visited, "z.r1")
	if len(again) != 2 {
		t.Fatalf("re-entering a visited node appended: %v", again)
	}
	fresh := appendVisited(again, "z.r3")
	if len(fresh) != 3 || fresh[2] != "z.r3" {
		t.Errorf("new node not appended in order: %v", fresh)
	}
}

func TestAppendClearedRoom_IsIdempotent(t *testing.T) {
	// Walking out of room 1 twice must not inflate RoomsCleared — its
	// length feeds "N/M rooms" renders and would eventually exceed TotalRooms.
	cleared := appendClearedRoom([]int{0, 1}, 1)
	if len(cleared) != 2 {
		t.Fatalf("re-clearing room 1 appended: %v", cleared)
	}
	if got := appendClearedRoom(cleared, 2); len(got) != 3 {
		t.Errorf("fresh room not appended: %v", got)
	}
}

// ── the load-bearing property: CurrentRoom tracks the node, not the tail ────

func TestCurrentRoom_DerivesFromCurrentNodeNotVisitedTail(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r1-backtrack:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	// Walk forward two rooms so VisitedNodes has a tail distinct from
	// the node we're about to stand on.
	for i := 0; i < 2; i++ {
		if _, err := markRoomCleared(run.RunID); err != nil {
			t.Fatalf("markRoomCleared %d: %v", i, err)
		}
	}
	fwd, err := getZoneRun(run.RunID)
	if err != nil || fwd == nil {
		t.Fatalf("reload: %v", err)
	}
	if len(fwd.VisitedNodes) != 3 {
		t.Fatalf("expected 3 visited nodes, got %v", fwd.VisitedNodes)
	}
	if fwd.CurrentRoom != 2 {
		t.Errorf("forward CurrentRoom = %d, want 2", fwd.CurrentRoom)
	}

	// Simulate the backtrack R2 will perform: current_node moves back to
	// the first room, visited_nodes is untouched. Pre-R1 this read as
	// room 2 (the tail); it must now read as room 0.
	backTo := fwd.VisitedNodes[0]
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET current_node = ? WHERE run_id = ?`,
		backTo, run.RunID); err != nil {
		t.Fatalf("backtrack write: %v", err)
	}
	back, err := getZoneRun(run.RunID)
	if err != nil || back == nil {
		t.Fatalf("reload after backtrack: %v", err)
	}
	if back.CurrentRoom != 0 {
		t.Errorf("after backtrack CurrentRoom = %d, want 0 (the node's first-entry index)", back.CurrentRoom)
	}
	if len(back.VisitedNodes) != 3 {
		t.Errorf("backtrack must not shrink VisitedNodes: %v", back.VisitedNodes)
	}
	// The step counter does not rewind with the player.
	if back.RoomsTraversed != 3 {
		t.Errorf("RoomsTraversed = %d, want 3 (monotonic)", back.RoomsTraversed)
	}
}

// ── R1's stated acceptance: forward-only trajectories are unchanged ─────────

func TestForwardOnlyRun_KeepsTraversalLockedToPathIndex(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r1-forward:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	if run.RoomsTraversed != 1 {
		t.Fatalf("fresh run RoomsTraversed = %d, want 1 (entry room walked into)", run.RoomsTraversed)
	}

	for step := 0; step < 3; step++ {
		if _, err := markRoomCleared(run.RunID); err != nil {
			t.Fatalf("markRoomCleared %d: %v", step, err)
		}
		cur, err := getZoneRun(run.RunID)
		if err != nil || cur == nil {
			t.Fatalf("reload %d: %v", step, err)
		}
		// This is the identity every pre-R1 caller silently assumed. As long
		// as navigation is forward-only it must keep holding, or R1 has
		// changed behavior it promised not to.
		if cur.RoomsTraversed != cur.CurrentRoom+1 {
			t.Errorf("step %d: RoomsTraversed=%d, CurrentRoom=%d — want traversed == room+1",
				step, cur.RoomsTraversed, cur.CurrentRoom)
		}
		if cur.RoomsTraversed != len(cur.VisitedNodes) {
			t.Errorf("step %d: RoomsTraversed=%d, len(VisitedNodes)=%d — want equal on a forward-only run",
				step, cur.RoomsTraversed, len(cur.VisitedNodes))
		}
		// narrationCadence is the pre-R1 salt: len(VisitedNodes)-1. Any drift
		// here silently re-keys every flavor pool for in-flight runs.
		if got, want := narrationCadence(cur), len(cur.VisitedNodes)-1; got != want {
			t.Errorf("step %d: narrationCadence = %d, want %d (pre-R1 salt)", step, got, want)
		}
	}
}

func TestNarrationCadence_TracksTraversalNotPosition(t *testing.T) {
	// A backtracked run: standing in room 0, but three rooms walked. Flavor
	// should keep advancing, so a player who doubles back reads new lines
	// instead of the entry-room narration a second time.
	run := &DungeonRun{
		VisitedNodes:   []string{"z.r1", "z.r2", "z.r3"},
		CurrentNode:    "z.r1",
		CurrentRoom:    0,
		RoomsTraversed: 4,
	}
	if got := narrationCadence(run); got != 3 {
		t.Errorf("narrationCadence = %d, want 3 (traversals-1), not the room index", got)
	}
	// Rows not yet swept by the bootstrap fall back to the visited count.
	stale := &DungeonRun{VisitedNodes: []string{"z.r1", "z.r2"}}
	if got := narrationCadence(stale); got != 1 {
		t.Errorf("un-backfilled row cadence = %d, want 1", got)
	}
}

// ── bootstrap ───────────────────────────────────────────────────────────────

func TestBootstrapRoomsTraversed_BackfillsFromVisitedNodes(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r1-bootstrap:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	visited := []string{"gw.r1", "gw.r2", "gw.r3", "gw.r4"}
	visitedJSON, _ := json.Marshal(visited)
	// Reproduce a row as the ALTER leaves it: a real path, counter at 0.
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET visited_nodes = ?, rooms_traversed = 0 WHERE run_id = ?`,
		string(visitedJSON), run.RunID); err != nil {
		t.Fatalf("seed pre-R1 row: %v", err)
	}

	bootstrapRoomsTraversed()

	got, err := getZoneRun(run.RunID)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.RoomsTraversed != 4 {
		t.Errorf("RoomsTraversed = %d, want 4 (len(visited_nodes))", got.RoomsTraversed)
	}

	// Idempotent: a second sweep must not touch the row it already fixed,
	// nor the counter of a run that has since walked on.
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET rooms_traversed = 9 WHERE run_id = ?`, run.RunID); err != nil {
		t.Fatalf("bump: %v", err)
	}
	bootstrapRoomsTraversed()
	again, _ := getZoneRun(run.RunID)
	if again.RoomsTraversed != 9 {
		t.Errorf("re-run clobbered a live counter: got %d, want 9", again.RoomsTraversed)
	}
}

// An empty visited_nodes must not backfill to 0 and re-arm the sentinel —
// the row would be swept on every single boot.
func TestBootstrapRoomsTraversed_FloorsAtOne(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r1-empty-path:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET visited_nodes = '[]', rooms_traversed = 0 WHERE run_id = ?`,
		run.RunID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bootstrapRoomsTraversed()
	got, _ := getZoneRun(run.RunID)
	if got.RoomsTraversed != 1 {
		t.Errorf("RoomsTraversed = %d, want 1 (floor)", got.RoomsTraversed)
	}
}
