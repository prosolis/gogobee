package plugin

import "testing"

func TestFeywildCrossingGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneFeywildCrossing]
	if !ok {
		t.Fatal("zoneFeywildCrossingGraph not registered")
	}
	// Long-expedition D1-d widened this zone from 9 → 53 nodes so the
	// longest entry→boss walk lands in the T4 [28,34] traversal band.
	if len(g.Nodes) != 53 {
		t.Errorf("nodes = %d, want 53", len(g.Nodes))
	}
}

func TestFeywildCrossingGraph_LongestPathInBand(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	got := graphLongestPath(g)
	if got < 28 || got > 34 {
		t.Errorf("longest path = %d, want in T4 band [28,34]", got)
	}
}

// TestFeywildCrossingGraph_HagCircleSharedReach verifies hag_circle
// has TWO incoming edges — one from each first-stage path. This is the
// woven-fork shape's signature; if a future edit splits hag_circle so
// each path has its own copy, this fails (and the shape comment needs
// to be re-authored).
func TestFeywildCrossingGraph_HagCircleSharedReach(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	incoming := map[string][]string{}
	for from, outs := range g.Edges {
		for _, e := range outs {
			incoming[e.To] = append(incoming[e.To], from)
		}
	}
	hagSources := incoming["feywild_crossing.hag_circle"]
	if len(hagSources) != 2 {
		t.Errorf("hag_circle incoming = %d, want 2 (woven fork)", len(hagSources))
	}
}

// TestFeywildCrossingGraph_PartialOverlap verifies time_eddy is
// reachable only via the grove and illusion_garden only via the marsh.
// This is what makes first-stage choice meaningful even though
// hag_circle is shared.
func TestFeywildCrossingGraph_PartialOverlap(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	if reachable(g, "feywild_crossing.wisp_marsh", "feywild_crossing.time_eddy") {
		t.Error("time_eddy should NOT be reachable from wisp_marsh — grove-exclusive")
	}
	if reachable(g, "feywild_crossing.glamoured_grove", "feywild_crossing.illusion_garden") {
		t.Error("illusion_garden should NOT be reachable from glamoured_grove — marsh-exclusive")
	}
}

// TestFeywildCrossingGraph_NoFreeChoiceAtFork1 captures the design
// intent: both fork1 outgoing edges are locked. The player must succeed
// at CHA or Perception to enter; no LockNone fallback.
func TestFeywildCrossingGraph_NoFreeChoiceAtFork1(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	for _, e := range g.outgoingEdges("feywild_crossing.fork1") {
		if e.Lock == LockNone {
			t.Errorf("fork1 has unlocked edge to %s — expected all locked", e.To)
		}
	}
}

// TestFeywildCrossingGraph_FirstCHALock confirms this zone uses
// LockStatCheck CHA — completing CHA in lock-kind coverage by G8f.
func TestFeywildCrossingGraph_FirstCHALock(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	found := false
	for _, outs := range g.Edges {
		for _, e := range outs {
			if e.Lock == LockStatCheck && lockDataString(e.LockData, "stat") == "CHA" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected at least one LockStatCheck CHA edge — Feywild theme")
	}
}

// TestFeywildCrossingGraph_TrapAnchor verifies D1-d added the missing
// Trap node. Original G8f graph had elite/secret but no trap.
func TestFeywildCrossingGraph_TrapAnchor(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	var trapCount int
	for _, n := range g.Nodes {
		if n.Kind == NodeKindTrap {
			trapCount++
		}
	}
	if trapCount != 1 {
		t.Errorf("trap nodes = %d, want 1", trapCount)
	}
}

// TestFeywildCrossingGraph_FaeCourtMerge verifies all three second-
// stage endings (hag_circle, time_eddy, illusion_garden) converge at
// the fae_court merge before the final boss approach. D1-d added this
// merge to avoid triplicating the long pre-boss walk.
func TestFeywildCrossingGraph_FaeCourtMerge(t *testing.T) {
	g := zoneFeywildCrossingGraph()
	for _, ending := range []string{
		"feywild_crossing.hag_circle",
		"feywild_crossing.time_eddy",
		"feywild_crossing.illusion_garden",
	} {
		if !reachable(g, ending, "feywild_crossing.fae_court") {
			t.Errorf("%s does not reach fae_court merge", ending)
		}
	}
	if !reachable(g, "feywild_crossing.fae_court", "feywild_crossing.boss") {
		t.Error("fae_court → boss path missing")
	}
}
