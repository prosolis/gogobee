package plugin

import "testing"

func TestGoblinWarrensGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneGoblinWarrens]
	if !ok {
		t.Fatal("zoneGoblinWarrensGraph not registered")
	}
	if g.Entry != "goblin_warrens.entry" {
		t.Errorf("entry node = %q, want goblin_warrens.entry", g.Entry)
	}
	if g.Boss != "goblin_warrens.boss" {
		t.Errorf("boss node = %q, want goblin_warrens.boss", g.Boss)
	}
	if len(g.Nodes) != 7 {
		t.Errorf("nodes = %d, want 7", len(g.Nodes))
	}
}

func TestGoblinWarrensGraph_BothPathsReachBoss(t *testing.T) {
	g := zoneGoblinWarrensGraph()
	if !reachable(g, "goblin_warrens.warband_pit", "goblin_warrens.boss") {
		t.Error("warband_pit path unreachable to boss")
	}
	if !reachable(g, "goblin_warrens.collapsed_shaft", "goblin_warrens.boss") {
		t.Error("collapsed_shaft path unreachable to boss")
	}
}

func TestGoblinWarrensGraph_ForkLayout(t *testing.T) {
	g := zoneGoblinWarrensGraph()
	outs := g.outgoingEdges("goblin_warrens.fork")
	if len(outs) != 2 {
		t.Fatalf("fork outgoing edges = %d, want 2", len(outs))
	}
	// outgoingEdges sorts by weight asc — warband_pit (weight 1) first.
	if outs[0].To != "goblin_warrens.warband_pit" || outs[0].Lock != LockNone {
		t.Errorf("first edge = %+v, want warband_pit/LockNone", outs[0])
	}
	if outs[1].To != "goblin_warrens.collapsed_shaft" || outs[1].Lock != LockPerception {
		t.Errorf("second edge = %+v, want collapsed_shaft/LockPerception", outs[1])
	}
	if outs[1].Hint == "" {
		t.Error("collapsed_shaft edge missing player-facing hint (plan §G8: cruel design)")
	}
	if dc := lockDataInt(outs[1].LockData, "dc", 0); dc != 12 {
		t.Errorf("perception DC = %d, want 12", dc)
	}
}

// TestGoblinWarrensGraph_NoSecretByDesign locks in the T1 "single fork,
// no secret" decision recorded in project_phase_G_progress memo. If a
// later session adds a secret to this zone, this test should be removed
// or updated deliberately — not silently.
func TestGoblinWarrensGraph_NoSecretByDesign(t *testing.T) {
	g := zoneGoblinWarrensGraph()
	for _, n := range g.Nodes {
		if n.Kind == NodeKindSecret {
			t.Errorf("unexpected secret node %q — Goblin Warrens is T1 single-fork by design", n.NodeID)
		}
	}
}
