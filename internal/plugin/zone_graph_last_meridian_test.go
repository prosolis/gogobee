package plugin

import "testing"

func TestLastMeridianGraph_Invariants(t *testing.T) {
	assertPostgameGraph(t, ZoneLastMeridian, zoneLastMeridianGraph)
}

// TestLastMeridianGraph_RhythmLocks verifies the escapement's rhythm gate
// (INT on fork2) and the unspent-time secret pockets are authored.
func TestLastMeridianGraph_RhythmLocks(t *testing.T) {
	g := zoneLastMeridianGraph()
	if got := countSecretNodes(g); got != 3 {
		t.Errorf("secret nodes = %d, want 3", got)
	}
	intFork := false
	for _, e := range g.outgoingEdges("last_meridian.fork2") {
		if e.Lock == LockStatCheck && lockDataString(e.LockData, "stat") == "INT" {
			intFork = true
		}
	}
	if !intFork {
		t.Error("Last Meridian fork2 should gate on INT (escapement rhythm)")
	}
}
