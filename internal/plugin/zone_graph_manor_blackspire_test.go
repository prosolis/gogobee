package plugin

import "testing"

func TestManorBlackspireGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneManorBlackspire]
	if !ok {
		t.Fatal("zoneManorBlackspireGraph not registered")
	}
	if g.Entry != "manor_blackspire.entry" {
		t.Errorf("entry node = %q", g.Entry)
	}
	if len(g.Nodes) != 11 {
		t.Errorf("nodes = %d, want 11", len(g.Nodes))
	}
}

// TestManorBlackspireGraph_TwoStackedThreeWayForks captures the design
// shape: both great_hall and upper_hall expose three options.
func TestManorBlackspireGraph_TwoStackedThreeWayForks(t *testing.T) {
	g := zoneManorBlackspireGraph()
	for _, hub := range []string{
		"manor_blackspire.great_hall",
		"manor_blackspire.upper_hall",
	} {
		outs := g.outgoingEdges(hub)
		if len(outs) != 3 {
			t.Errorf("%s outgoing = %d, want 3", hub, len(outs))
		}
	}
}

func TestManorBlackspireGraph_AllSpokesReachBoss(t *testing.T) {
	g := zoneManorBlackspireGraph()
	for _, leaf := range []string{
		"manor_blackspire.master_bedroom",
		"manor_blackspire.tower_observatory",
		"manor_blackspire.hidden_oratory",
		"manor_blackspire.portrait_gallery",
		"manor_blackspire.study",
		"manor_blackspire.library",
	} {
		if !reachable(g, leaf, "manor_blackspire.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}

// TestManorBlackspireGraph_LockLevelMinFirstUse verifies this zone is
// the first to author LockLevelMin, completing lock-kind coverage
// (Perception, StatCheck, LevelMin, LockNone) by G8d.
func TestManorBlackspireGraph_LockLevelMinFirstUse(t *testing.T) {
	g := zoneManorBlackspireGraph()
	var levelMinEdge *ZoneEdge
	for _, outs := range g.Edges {
		for i := range outs {
			if outs[i].Lock == LockLevelMin {
				levelMinEdge = &outs[i]
			}
		}
	}
	if levelMinEdge == nil {
		t.Fatal("expected at least one LockLevelMin edge")
	}
	if min := lockDataInt(levelMinEdge.LockData, "min_level", 0); min < 7 {
		t.Errorf("min_level = %d, want >= 7 (T3 zone)", min)
	}
	if levelMinEdge.Hint == "" {
		t.Error("level-gated edge missing hint")
	}
}
