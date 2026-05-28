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
	// Long-expedition D1-c widened this zone from 11 → 35 nodes so the
	// longest entry→boss walk lands in the T3 [22,26] traversal band.
	if len(g.Nodes) != 35 {
		t.Errorf("nodes = %d, want 35", len(g.Nodes))
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
		// great_hall spokes (entry of each 3-node branch).
		"manor_blackspire.portrait_gallery",
		"manor_blackspire.locked_study",
		"manor_blackspire.forbidden_library",
		// upper_hall spokes.
		"manor_blackspire.master_bedroom",
		"manor_blackspire.hidden_oratory",
		"manor_blackspire.tower_observatory",
	} {
		if !reachable(g, leaf, "manor_blackspire.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}

// TestManorBlackspireGraph_LockLevelMinFirstUse verifies the tower spoke
// still carries the LockLevelMin gate so all four lock kinds (None,
// Perception, StatCheck, LevelMin) remain authored within this zone.
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

// TestManorBlackspireGraph_SymmetricBranches locks in the D1-c design
// intent: all six fork spokes are 3 mid-nodes long so any route walks
// the same 23-room length — the choice is loot/encounter, not shortcut.
func TestManorBlackspireGraph_SymmetricBranches(t *testing.T) {
	g := zoneManorBlackspireGraph()
	checks := []struct {
		from, to, via string
		want          int
	}{
		// great_hall → second_floor_landing, hops via each spoke.
		{"manor_blackspire.great_hall", "manor_blackspire.second_floor_landing", "manor_blackspire.portrait_gallery", 4},
		{"manor_blackspire.great_hall", "manor_blackspire.second_floor_landing", "manor_blackspire.locked_study", 4},
		{"manor_blackspire.great_hall", "manor_blackspire.second_floor_landing", "manor_blackspire.forbidden_library", 4},
		// upper_hall → spire_corridor, hops via each spoke.
		{"manor_blackspire.upper_hall", "manor_blackspire.spire_corridor", "manor_blackspire.master_bedroom", 4},
		{"manor_blackspire.upper_hall", "manor_blackspire.spire_corridor", "manor_blackspire.hidden_oratory", 4},
		{"manor_blackspire.upper_hall", "manor_blackspire.spire_corridor", "manor_blackspire.tower_observatory", 4},
	}
	for _, c := range checks {
		got := bfsHops(g, c.from, c.to, c.via)
		if got != c.want {
			t.Errorf("hops %s → %s via %s = %d, want %d", c.from, c.to, c.via, got, c.want)
		}
	}
}
