package plugin

import "testing"

// backtrackGraph — a fork at z.fork with two branches, plus a dead-end spur
// hanging off z.b whose only exit is back into z.b.
//
//	z.entry → z.fork → z.a
//	                 → z.b → z.spur
func backtrackGraph() ZoneGraph {
	return ZoneGraph{
		Nodes: map[string]ZoneNode{},
		Edges: map[string][]ZoneEdge{
			"z.entry": {{From: "z.entry", To: "z.fork"}},
			"z.fork":  {{From: "z.fork", To: "z.a"}, {From: "z.fork", To: "z.b"}},
			"z.b":     {{From: "z.b", To: "z.spur"}},
		},
	}
}

// TestBacktrackTargetRequiresAdjacency is the regression guard for the bug this
// fixes: VisitedNodes is a first-entry ordered set, not a path stack, so the
// entry before CurrentNode can sit on a branch the party never walked from
// here. Stepping to it blind teleports them across the map.
func TestBacktrackTargetRequiresAdjacency(t *testing.T) {
	// Walked entry → fork → a, doubled back, then took fork → b. The visited
	// set is [entry, fork, a, b]; z.a is *not* joined to z.b.
	run := &DungeonRun{
		CurrentNode:  "z.b",
		VisitedNodes: []string{"z.entry", "z.fork", "z.a", "z.b"},
	}
	got, ok := backtrackTarget(backtrackGraph(), run)
	if !ok {
		t.Fatal("should fall back to the fork")
	}
	if got != "z.fork" {
		t.Errorf("backtrack target = %s, want z.fork (z.a shares no edge with z.b)", got)
	}
}

// TestBacktrackTargetSkipsOneWayCorridor — backing into a room whose only exit
// is the sealed fork walks straight back in, and the lock rolls are seeded per
// (run, edge), so it would do it forever. Refuse instead.
func TestBacktrackTargetSkipsOneWayCorridor(t *testing.T) {
	run := &DungeonRun{
		CurrentNode:  "z.spur",
		VisitedNodes: []string{"z.entry", "z.fork", "z.b", "z.spur"},
	}
	if got, ok := backtrackTarget(backtrackGraph(), run); ok {
		t.Errorf("backtracked to %s; z.b only leads back to z.spur, so this loops", got)
	}
}

// TestBacktrackTargetAtEntry — nowhere behind the entry node.
func TestBacktrackTargetAtEntry(t *testing.T) {
	run := &DungeonRun{CurrentNode: "z.entry", VisitedNodes: []string{"z.entry"}}
	if _, ok := backtrackTarget(backtrackGraph(), run); ok {
		t.Error("entry node has nowhere to back out to")
	}
}
