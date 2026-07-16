package plugin

import "testing"

func TestDrownedStarGraph_Invariants(t *testing.T) {
	assertPostgameGraph(t, ZoneDrownedStar, zoneDrownedStarGraph)
}

// TestDrownedStarGraph_PilgrimGrace verifies the CHA pilgrim-route capstone
// spur and the radiant secret pockets are authored.
func TestDrownedStarGraph_PilgrimGrace(t *testing.T) {
	g := zoneDrownedStarGraph()
	if got := countSecretNodes(g); got != 3 {
		t.Errorf("secret nodes = %d, want 3", got)
	}
	chaSpur := false
	for _, e := range g.outgoingEdges("drowned_star.fork3") {
		if e.Lock == LockStatCheck && lockDataString(e.LockData, "stat") == "CHA" {
			chaSpur = true
		}
	}
	if !chaSpur {
		t.Error("Drowned Star capstone should offer a CHA pilgrim-route spur")
	}
}
