package plugin

import "testing"

func TestOssuaryAscendantGraph_Invariants(t *testing.T) {
	assertPostgameGraph(t, ZoneOssuaryAscendant, zoneOssuaryAscendantGraph)
}

// TestOssuaryAscendantGraph_ThreeVerses verifies the signature set-piece:
// three NodeKindSecret Verses, each reached through a LockPerception spur
// edge (the gate rides the fork→threshold edge; the edge into the Verse
// itself is unlocked).
func TestOssuaryAscendantGraph_ThreeVerses(t *testing.T) {
	g := zoneOssuaryAscendantGraph()

	verses := []string{"ossuary_ascendant.f1b2", "ossuary_ascendant.f2b2", "ossuary_ascendant.cap32"}
	for _, v := range verses {
		n, ok := g.Nodes[v]
		if !ok {
			t.Errorf("missing Verse node %s", v)
			continue
		}
		if n.Kind != NodeKindSecret {
			t.Errorf("Verse %s kind = %q, want secret", v, n.Kind)
		}
	}
	if got := countSecretNodes(g); got != 3 {
		t.Errorf("secret nodes = %d, want 3 (the Phylactery Verses)", got)
	}

	// Each Verse's spur is entered through a Perception gate.
	percThresholds := map[string]bool{
		"ossuary_ascendant.f1b1": false,
		"ossuary_ascendant.f2b1": false,
		"ossuary_ascendant.cap31": false,
	}
	for _, outs := range g.Edges {
		for _, e := range outs {
			if e.Lock == LockPerception {
				if _, ok := percThresholds[e.To]; ok {
					percThresholds[e.To] = true
				}
			}
		}
	}
	for th, gated := range percThresholds {
		if !gated {
			t.Errorf("Verse threshold %s is not behind a LockPerception edge", th)
		}
	}
}

func countSecretNodes(g ZoneGraph) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Kind == NodeKindSecret {
			n++
		}
	}
	return n
}
