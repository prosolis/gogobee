package plugin

import "testing"

func TestUnderforgeGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneUnderforge]
	if !ok {
		t.Fatal("zoneUnderforgeGraph not registered")
	}
	// Long-expedition D1-c widened this zone from 10 → 31 nodes so the
	// longest entry→boss walk lands in the T3 [22,26] traversal band.
	if len(g.Nodes) != 31 {
		t.Errorf("nodes = %d, want 31", len(g.Nodes))
	}
}

// TestUnderforgeGraph_LinearPreamble locks in the gauntlet identity:
// every node from entry through resonance_passage has exactly one
// outgoing edge — the only decision in the zone is the antechamber
// 3-way at the very end. D1-c extends the chain but preserves the
// "one-way descent" intent ("Kharak Dûn is a one-way descent — there
// is no scenic route").
func TestUnderforgeGraph_LinearPreamble(t *testing.T) {
	g := zoneUnderforgeGraph()
	for _, id := range []string{
		"underforge.entry",
		"underforge.sealed_threshold",
		"underforge.wardstone_arch",
		"underforge.forge_descent",
		"underforge.vapor_steps",
		"underforge.slag_warrens",
		"underforge.cooling_river",
		"underforge.bellows_chamber",
		"underforge.cinder_walk",
		"underforge.magma_chamber",
		"underforge.emberforge_hall",
		"underforge.ashfall_corridor",
		"underforge.smelting_vault",
		"underforge.foreman_landing",
		"underforge.broken_crucible",
		"underforge.great_anvil",
		"underforge.resonance_passage",
	} {
		outs := g.outgoingEdges(id)
		if len(outs) != 1 {
			t.Errorf("preamble node %s outgoing = %d, want 1 (gauntlet)", id, len(outs))
		}
	}
}

func TestUnderforgeGraph_AntechamberThreeWay(t *testing.T) {
	g := zoneUnderforgeGraph()
	outs := g.outgoingEdges("underforge.antechamber")
	if len(outs) != 3 {
		t.Fatalf("antechamber outgoing = %d, want 3", len(outs))
	}
	for _, leaf := range []string{
		"underforge.direct_assault",
		"underforge.catwalks",
		"underforge.forge_vault",
	} {
		if !reachable(g, leaf, "underforge.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}
