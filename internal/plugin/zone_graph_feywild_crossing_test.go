package plugin

import "testing"

func TestFeywildCrossingGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneFeywildCrossing]
	if !ok {
		t.Fatal("zoneFeywildCrossingGraph not registered")
	}
	if len(g.Nodes) != 9 {
		t.Errorf("nodes = %d, want 9", len(g.Nodes))
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
