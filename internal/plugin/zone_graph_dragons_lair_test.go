package plugin

import "testing"

func TestDragonsLairGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneDragonsLair]
	if !ok {
		t.Fatal("zoneDragonsLairGraph not registered")
	}
	if len(g.Nodes) != 12 {
		t.Errorf("nodes = %d, want 12", len(g.Nodes))
	}
}

// TestDragonsLairGraph_Fork1Converges verifies the binary mid-fork
// converges at wyrmlings_nest.
func TestDragonsLairGraph_Fork1Converges(t *testing.T) {
	g := zoneDragonsLairGraph()
	for _, mid := range []string{"dragons_lair.ash_bridge", "dragons_lair.treasure_vault"} {
		outs := g.outgoingEdges(mid)
		if len(outs) != 1 || outs[0].To != "dragons_lair.wyrmlings_nest" {
			t.Errorf("%s outs = %+v, want single edge to wyrmlings_nest", mid, outs)
		}
	}
}

// TestDragonsLairGraph_Fork2Capstone verifies the late fork has 3
// distinct paths to the boss, each through its own capstone node.
func TestDragonsLairGraph_Fork2Capstone(t *testing.T) {
	g := zoneDragonsLairGraph()
	outs := g.outgoingEdges("dragons_lair.fork2")
	if len(outs) != 3 {
		t.Fatalf("fork2 outgoing = %d, want 3", len(outs))
	}
	for _, leaf := range []string{
		"dragons_lair.direct_confrontation",
		"dragons_lair.dragon_bargain",
		"dragons_lair.hoard_pillar",
	} {
		if !reachable(g, leaf, "dragons_lair.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
	// Locks escalate: open / CHA 16 / Perception 17 (T5 secret bias).
	for _, e := range outs {
		if e.To == "dragons_lair.hoard_pillar" {
			if dc := lockDataInt(e.LockData, "dc", 0); dc < 17 {
				t.Errorf("hoard_pillar DC = %d, want >= 17 (T5)", dc)
			}
		}
	}
}

// TestDragonsLairGraph_LootBiasEscalation verifies the T5 secret has
// the highest loot bias of any authored zone (≥ 2.5).
func TestDragonsLairGraph_LootBiasEscalation(t *testing.T) {
	g := zoneDragonsLairGraph()
	hoard := g.Nodes["dragons_lair.hoard_pillar"]
	if hoard.Content.LootBias < 2.5 {
		t.Errorf("hoard_pillar LootBias = %v, want >= 2.5 (T5 secret)", hoard.Content.LootBias)
	}
}
