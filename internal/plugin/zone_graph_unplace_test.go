package plugin

import "testing"

func TestUnplaceGraph_Invariants(t *testing.T) {
	assertPostgameGraph(t, ZoneUnplace, zoneUnplaceGraph)
}

// TestUnplaceGraph_ImpossibleShortcuts verifies the three high-bias secret
// pockets and that its capstone offers the "not here" Perception shortcut.
func TestUnplaceGraph_ImpossibleShortcuts(t *testing.T) {
	g := zoneUnplaceGraph()
	if got := countSecretNodes(g); got != 3 {
		t.Errorf("secret nodes = %d, want 3", got)
	}
	percCapstone := false
	for _, e := range g.outgoingEdges("unplace.fork3") {
		if e.Lock == LockPerception {
			percCapstone = true
		}
	}
	if !percCapstone {
		t.Error("Unplace capstone should offer a LockPerception shortcut spur")
	}
}
