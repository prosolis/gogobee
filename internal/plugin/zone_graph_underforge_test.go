package plugin

import "testing"

func TestUnderforgeGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneUnderforge]
	if !ok {
		t.Fatal("zoneUnderforgeGraph not registered")
	}
	if len(g.Nodes) != 10 {
		t.Errorf("nodes = %d, want 10", len(g.Nodes))
	}
}

// TestUnderforgeGraph_LinearPreamble locks in the gauntlet shape:
// the first five nodes after entry must each have exactly one outgoing
// edge (linear chain). If a future edit splits the preamble, this test
// catches it — that change should re-author the shape comment too.
func TestUnderforgeGraph_LinearPreamble(t *testing.T) {
	g := zoneUnderforgeGraph()
	for _, id := range []string{
		"underforge.entry",
		"underforge.sealed_gate",
		"underforge.forge_descent",
		"underforge.cooling_river",
		"underforge.magma_chamber",
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
