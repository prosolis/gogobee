package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Revisit R2 (gogobee_revisit_plan.md §R2) — `!revisit <N>` backtracking.

// ── adjacency ───────────────────────────────────────────────────────────────

func testGraph() ZoneGraph {
	// a → b → c, plus a side branch b → d. Directed, as all zone graphs are.
	return ZoneGraph{
		Entry: "a",
		Nodes: map[string]ZoneNode{
			"a": {NodeID: "a", Kind: NodeKindEntry},
			"b": {NodeID: "b", Kind: NodeKindExploration},
			"c": {NodeID: "c", Kind: NodeKindExploration},
			"d": {NodeID: "d", Kind: NodeKindHarvest},
		},
		Edges: map[string][]ZoneEdge{
			"a": {{From: "a", To: "b"}},
			"b": {{From: "b", To: "c"}, {From: "b", To: "d"}},
		},
	}
}

func TestAdjacentNodes_TraversesEdgesInBothDirections(t *testing.T) {
	g := testGraph()
	// From b: forward to c and d, backward to a. A corridor you walked down
	// is a corridor you can walk back up.
	adj := adjacentNodes(g, "b")
	for _, want := range []string{"a", "c", "d"} {
		if !adj[want] {
			t.Errorf("adjacentNodes(b) missing %q: %v", want, adj)
		}
	}
	if len(adj) != 3 {
		t.Errorf("adjacentNodes(b) = %v, want exactly {a,c,d}", adj)
	}
	// c is a leaf: only the reverse edge from b.
	if adj := adjacentNodes(g, "c"); len(adj) != 1 || !adj["b"] {
		t.Errorf("adjacentNodes(c) = %v, want {b}", adj)
	}
}

func TestAdjacentNodes_NonAdjacentIsNotReachable(t *testing.T) {
	// a and c are two edges apart — revisit is one step at a time.
	if adjacentNodes(testGraph(), "a")["c"] {
		t.Error("a is adjacent to c; want not adjacent")
	}
}

func TestRevisitTargets_OnlyVisitedNeighbours(t *testing.T) {
	g := testGraph()
	// Walked a → b, standing in b. d is adjacent but never visited; c same.
	run := &DungeonRun{VisitedNodes: []string{"a", "b"}, CurrentNode: "b"}
	got := revisitTargets(g, run)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("revisitTargets = %v, want [1] (room 1 = node a)", got)
	}
	// Standing in a with nothing behind: no targets.
	atEntry := &DungeonRun{VisitedNodes: []string{"a"}, CurrentNode: "a"}
	if got := revisitTargets(g, atEntry); len(got) != 0 {
		t.Errorf("revisitTargets at entry = %v, want none", got)
	}
}

// ── the cost, and the guard on it ───────────────────────────────────────────

func TestRevisitThreatCost_IsOneAndGuardsSiege(t *testing.T) {
	// The plan specced a discount on a cost that doesn't exist; +1 is the
	// net-new charge. If someone retunes it, the siege guard must follow.
	if revisitThreatCost != 1 {
		t.Errorf("revisitThreatCost = %d, want 1", revisitThreatCost)
	}
	if revisitSiegeGuard+revisitThreatCost != 100 {
		t.Errorf("siege guard %d + cost %d != 100 — a revisit could trip siege",
			revisitSiegeGuard, revisitThreatCost)
	}
}

// ── the state move ──────────────────────────────────────────────────────────

func TestRevisitZoneRun_MovesBackWithoutRenumbering(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r2-move:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := markRoomCleared(run.RunID); err != nil {
			t.Fatalf("markRoomCleared: %v", err)
		}
	}
	fwd, _ := getZoneRun(run.RunID)
	if fwd.CurrentRoom != 2 || fwd.RoomsTraversed != 3 {
		t.Fatalf("setup: room=%d traversed=%d, want 2/3", fwd.CurrentRoom, fwd.RoomsTraversed)
	}

	target := fwd.VisitedNodes[1] // room 2
	idx, err := revisitZoneRun(run.RunID, target, fwd.VisitedNodes)
	if err != nil {
		t.Fatalf("revisitZoneRun: %v", err)
	}
	if idx != 1 {
		t.Errorf("revisit returned index %d, want 1", idx)
	}

	back, _ := getZoneRun(run.RunID)
	if back.CurrentNode != target {
		t.Errorf("CurrentNode = %q, want %q", back.CurrentNode, target)
	}
	if back.CurrentRoom != 1 {
		t.Errorf("CurrentRoom = %d, want 1 — the room keeps its original number", back.CurrentRoom)
	}
	if len(back.VisitedNodes) != 3 {
		t.Errorf("VisitedNodes grew on revisit: %v", back.VisitedNodes)
	}
	// Effort climbs even though position fell back. This is the R1 split.
	if back.RoomsTraversed != 4 {
		t.Errorf("RoomsTraversed = %d, want 4 (the walk happened)", back.RoomsTraversed)
	}
}

// ── the exploit the cleared-room gate closes ────────────────────────────────

func TestRoomIsCleared_GatesReResolution(t *testing.T) {
	run := &DungeonRun{RoomsCleared: []int{0, 1, 2}}
	for _, idx := range []int{0, 1, 2} {
		if !run.RoomIsCleared(idx) {
			t.Errorf("room %d should read as cleared", idx)
		}
	}
	if run.RoomIsCleared(3) {
		t.Error("uncleared room 3 read as cleared")
	}
}

// Walking back into a cleared room and advancing out of it must not re-roll
// combat or re-arm the trap. Pre-R2 resolveRoom dispatched unconditionally
// for Exploration/Trap — only Elite/Boss were gated (by their combat
// session), so this was a farm: step back one room, advance, repeat.
func TestRevisitedRoom_DoesNotReResolve(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r2-noexploit:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	// Clear two rooms, then stand back in room 1 as a revisit would leave us.
	for i := 0; i < 2; i++ {
		if _, err := markRoomCleared(run.RunID); err != nil {
			t.Fatalf("markRoomCleared: %v", err)
		}
	}
	fwd, _ := getZoneRun(run.RunID)
	if _, err := revisitZoneRun(run.RunID, fwd.VisitedNodes[1], fwd.VisitedNodes); err != nil {
		t.Fatalf("revisitZoneRun: %v", err)
	}
	back, _ := getZoneRun(run.RunID)

	if !back.RoomIsCleared(back.CurrentRoom) {
		t.Fatalf("revisited room %d is not marked cleared — the gate won't fire", back.CurrentRoom)
	}
	p := &AdventurePlugin{}
	zone := zoneOrFallback(back.ZoneID)
	intro, phases, outcome, ended, err := p.resolveRoom(uid, back, zone, true)
	if err != nil {
		t.Fatalf("resolveRoom: %v", err)
	}
	if intro != "" || phases != nil || outcome != "" || ended {
		t.Errorf("cleared room re-resolved: intro=%q phases=%v outcome=%q ended=%v",
			intro, phases, outcome, ended)
	}
}

// Forward-only runs must still resolve their rooms — the gate is a no-op
// until someone backtracks. A regression here silently empties every zone.
func TestFreshRoom_StillResolves(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r2-fresh:example")

	if err := createAdvCharacter(uid, "r2fresh"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
		HPMax: 60, HPCurrent: 60, ArmorClass: 18,
	}); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	// Advance off the entry room (entry resolves to nothing by design) so we
	// land on a fresh, uncleared exploration room.
	if _, err := markRoomCleared(run.RunID); err != nil {
		t.Fatalf("markRoomCleared: %v", err)
	}
	cur, _ := getZoneRun(run.RunID)
	if cur.RoomIsCleared(cur.CurrentRoom) {
		t.Fatalf("room %d cleared straight after arriving", cur.CurrentRoom)
	}
	if cur.CurrentRoomType() != RoomExploration {
		t.Skipf("room 2 is %s, not exploration — nothing to assert", cur.CurrentRoomType())
	}
	p := &AdventurePlugin{}
	zone := zoneOrFallback(cur.ZoneID)
	// compact mode collapses a won fight into a single outcome line and leaves
	// phases nil, so outcome is the signal that combat actually ran.
	intro, phases, outcome, _, err := p.resolveRoom(uid, cur, zone, true)
	if err != nil {
		t.Fatalf("resolveRoom: %v", err)
	}
	if intro == "" && phases == nil && outcome == "" {
		t.Error("fresh exploration room resolved to nothing — the gate is over-firing")
	}
}

// ── autopilot grace ─────────────────────────────────────────────────────────

// Autopilot is ticker-driven with no on/off switch, so without a grace stamp
// the background walker marches the player straight back out of the room they
// revisited and the feature reads as broken.
func TestGrantAutorunGrace_StampsCooldown(t *testing.T) {
	setupRevisitTestDB(t)
	uid := id.UserID("@r2-grace:example")

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, run.RunID, makeSupplies(ZoneTier(1), SupplyPurchase{StandardPacks: 3}))
	if err != nil {
		t.Skipf("startExpedition unavailable in this harness: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET last_autorun_at = NULL WHERE expedition_id = ?`, exp.ID); err != nil {
		t.Fatalf("clear stamp: %v", err)
	}
	grantAutorunGrace(exp.ID)

	var stamped int
	if err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM dnd_expedition WHERE expedition_id = ? AND last_autorun_at IS NOT NULL`,
		exp.ID).Scan(&stamped); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if stamped != 1 {
		t.Error("grantAutorunGrace did not stamp last_autorun_at")
	}
}
