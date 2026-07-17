package plugin

import (
	"testing"
)

// mapFixtureGraph registers a small branching graph and returns a cleanup.
//
//	n1(entry) --none--------> n2(exploration) --key_required--> n4(boss)
//	   \--perception_check--> n3(trap) ------------------------/
//
// n4 is reachable two ways; the validator wants a single entry and a reachable
// boss, which this satisfies.
func mapFixtureGraph(t *testing.T) ZoneID {
	t.Helper()
	const zid ZoneID = "map_test_zone"
	g := ZoneGraph{
		ZoneID: zid,
		Entry:  "n1",
		Boss:   "n4",
		Nodes: map[string]ZoneNode{
			"n1": {NodeID: "n1", ZoneID: zid, Kind: NodeKindEntry, IsEntry: true, Label: "The Gate"},
			"n2": {NodeID: "n2", ZoneID: zid, Kind: NodeKindExploration, Label: "Dusty Hall"},
			"n3": {NodeID: "n3", ZoneID: zid, Kind: NodeKindTrap, Label: "Spiked Pit"},
			"n4": {NodeID: "n4", ZoneID: zid, Kind: NodeKindBoss, IsBoss: true, Label: "Throne of Bone"},
		},
		Edges: map[string][]ZoneEdge{
			"n1": {
				{From: "n1", To: "n2", Lock: LockNone, Weight: 1},
				{From: "n1", To: "n3", Lock: LockPerception, Weight: 1},
			},
			"n2": {{From: "n2", To: "n4", Lock: LockKey, Weight: 1}},
			"n3": {{From: "n3", To: "n4", Lock: LockNone, Weight: 1}},
		},
	}
	registerZoneGraph(g)
	t.Cleanup(func() { delete(zoneGraphRegistry, zid) })
	return zid
}

func TestBuildRosterMap_FogOfWar(t *testing.T) {
	zid := mapFixtureGraph(t)
	run := &DungeonRun{
		ZoneID:       zid,
		CurrentNode:  "n2",
		VisitedNodes: []string{"n1", "n2"},
		TotalRooms:   4,
	}
	m := buildRosterMap(run)
	if m == nil {
		t.Fatal("buildRosterMap returned nil for a visited run")
	}
	if m.ZoneID != string(zid) || m.CurrentNode != "n2" {
		t.Fatalf("header wrong: %+v", m)
	}

	kinds := map[string]string{}
	for _, n := range m.Nodes {
		if _, dup := kinds[n.ID]; dup {
			t.Errorf("node %q emitted twice", n.ID)
		}
		kinds[n.ID] = n.Kind
	}

	// Visited nodes carry their true kind.
	if kinds["n1"] != "entry" || kinds["n2"] != "exploration" {
		t.Errorf("visited kinds wrong: %v", kinds)
	}
	// Frontier nodes are present but their kind is withheld.
	if kinds["n3"] != "unknown" {
		t.Errorf("n3 is one hop out of n1 and must be unknown, got %q", kinds["n3"])
	}
	if kinds["n4"] != "unknown" {
		t.Errorf("n4 (the boss) is one hop out of n2 and must be unknown, got %q", kinds["n4"])
	}

	// The map must never leak a room the player has not reached a door to.
	// n4 is a real boss node, but reachable only as frontier — its kind stays
	// hidden. A node past a frontier door (there is none deeper here) must not
	// appear at all; assert exactly four nodes.
	if len(m.Nodes) != 4 {
		t.Fatalf("want 4 nodes (2 visited + 2 frontier), got %d: %+v", len(m.Nodes), m.Nodes)
	}

	// Edges out of visited nodes only. n3->n4 must NOT appear: n3 is frontier,
	// not visited, so walking its doors would leak structure past the fog.
	var edgeKeys []string
	for _, e := range m.Edges {
		edgeKeys = append(edgeKeys, e.From+"->"+e.To+":"+e.Lock)
	}
	want := map[string]bool{
		"n1->n2:":                 true, // LockNone dropped to ""
		"n1->n3:perception_check": true,
		"n2->n4:key_required":     true,
	}
	if len(edgeKeys) != len(want) {
		t.Fatalf("want %d edges, got %v", len(want), edgeKeys)
	}
	for _, k := range edgeKeys {
		if !want[k] {
			t.Errorf("unexpected edge %q (n3->n4 would be a fog leak)", k)
		}
	}
}

func TestBuildRosterMap_EmptyRunIsNil(t *testing.T) {
	zid := mapFixtureGraph(t)
	// A run that has visited nothing yet has no map to show.
	if m := buildRosterMap(&DungeonRun{ZoneID: zid}); m != nil {
		t.Errorf("empty VisitedNodes should yield nil, got %+v", m)
	}
	// An unknown zone (no graph, no legacy fallback row) yields nil, not a panic.
	if m := buildRosterMap(&DungeonRun{ZoneID: "no_such_zone", VisitedNodes: []string{"x"}}); m != nil {
		t.Errorf("unknown zone should yield nil, got %+v", m)
	}
}

func TestBuildRosterMap_BacktrackDedup(t *testing.T) {
	zid := mapFixtureGraph(t)
	// VisitedNodes is an ordered set that repeats on backtrack: n1,n2,n1.
	run := &DungeonRun{
		ZoneID:       zid,
		CurrentNode:  "n1",
		VisitedNodes: []string{"n1", "n2", "n1"},
	}
	m := buildRosterMap(run)
	seen := map[string]int{}
	for _, n := range m.Nodes {
		seen[n.ID]++
	}
	if seen["n1"] != 1 {
		t.Errorf("backtracked node n1 emitted %d times, want 1", seen["n1"])
	}
	if len(m.Visited) != 2 {
		t.Errorf("Visited should dedup to [n1 n2], got %v", m.Visited)
	}
}
