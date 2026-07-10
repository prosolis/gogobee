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
	// N5/D4 added the Sealed Reliquary cross-zone vault → 36.
	if len(g.Nodes) != 36 {
		t.Errorf("nodes = %d, want 36", len(g.Nodes))
	}
}

// TestManorBlackspireGraph_StackedForks captures the design shape: great_hall
// exposes three options; upper_hall exposes four since N5/D4 added the Sealed
// Reliquary key spoke (the other three remain the elite/secret/tower gates).
func TestManorBlackspireGraph_StackedForks(t *testing.T) {
	g := zoneManorBlackspireGraph()
	want := map[string]int{
		"manor_blackspire.great_hall": 3,
		"manor_blackspire.upper_hall": 4,
	}
	for hub, n := range want {
		if outs := g.outgoingEdges(hub); len(outs) != n {
			t.Errorf("%s outgoing = %d, want %d", hub, len(outs), n)
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
		"manor_blackspire.sealed_reliquary",
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
