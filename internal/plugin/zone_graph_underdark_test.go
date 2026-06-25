package plugin

import "testing"

func TestUnderdarkGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneUnderdark]
	if !ok {
		t.Fatal("zoneUnderdarkGraph not registered")
	}
	// Long-expedition D1-d widened this zone from 10 → 46 nodes so the
	// longest entry→boss walk lands in the T4 [28,34] traversal band.
	if len(g.Nodes) != 46 {
		t.Errorf("nodes = %d, want 46", len(g.Nodes))
	}
}

func TestUnderdarkGraph_LongestPathInBand(t *testing.T) {
	g := zoneUnderdarkGraph()
	got := graphLongestPath(g)
	if got < 28 || got > 34 {
		t.Errorf("longest path = %d, want in T4 band [28,34]", got)
	}
}

// TestUnderdarkGraph_AllNodesHaveRegion confirms Underdark is the
// canonical multi-region zone — every node carries a non-empty RegionID
// matching the dnd_expedition_region.go registry.
func TestUnderdarkGraph_AllNodesHaveRegion(t *testing.T) {
	g := zoneUnderdarkGraph()
	validRegions := map[string]bool{
		"underdark_surface_tunnels": true,
		"underdark_drow_outpost":    true,
		"underdark_illithid_warren": true,
		"underdark_deep_throne":     true,
	}
	for id, n := range g.Nodes {
		if n.RegionID == "" {
			t.Errorf("node %s has empty RegionID — Underdark requires region authoring on every node", id)
		}
		if !validRegions[n.RegionID] {
			t.Errorf("node %s RegionID = %q, not in dnd_expedition_region.go registry", id, n.RegionID)
		}
	}
}

// TestUnderdarkGraph_AllFourRegionsRepresented confirms each authored
// region has at least one node on the canonical entry→boss path.
func TestUnderdarkGraph_AllFourRegionsRepresented(t *testing.T) {
	g := zoneUnderdarkGraph()
	regions := map[string]int{}
	for _, n := range g.Nodes {
		regions[n.RegionID]++
	}
	for _, r := range []string{
		"underdark_surface_tunnels",
		"underdark_drow_outpost",
		"underdark_illithid_warren",
		"underdark_deep_throne",
	} {
		if regions[r] == 0 {
			t.Errorf("region %q has no nodes — multi-region invariant broken", r)
		}
	}
}

// TestUnderdarkGraph_ForkCrossesRegionBoundaries verifies the fork
// edges actually traverse region boundaries (this is what makes the
// G6 fireGraphRegionTransition hook fire end-to-end on this zone).
func TestUnderdarkGraph_ForkCrossesRegionBoundaries(t *testing.T) {
	g := zoneUnderdarkGraph()
	fork := g.Nodes["underdark.fork1"]
	if fork.RegionID != "underdark_surface_tunnels" {
		t.Fatalf("fork1 region = %q, want surface_tunnels", fork.RegionID)
	}
	for _, e := range g.outgoingEdges("underdark.fork1") {
		toNode := g.Nodes[e.To]
		switch e.To {
		case "underdark.drow_patrol":
			if toNode.RegionID != "underdark_drow_outpost" {
				t.Errorf("drow_patrol region = %q", toNode.RegionID)
			}
		case "underdark.psionic_corridor":
			if toNode.RegionID != "underdark_illithid_warren" {
				t.Errorf("psionic_corridor region = %q", toNode.RegionID)
			}
		case "underdark.deep_chasm":
			// Deep_chasm intentionally stays in R1 (surface_tunnels) — the
			// vertical-shaft path explores R1 deeper before joining R4.
			if toNode.RegionID != "underdark_surface_tunnels" {
				t.Errorf("deep_chasm region = %q, want surface_tunnels (intentional R1 stay)", toNode.RegionID)
			}
		}
	}
}

func TestUnderdarkGraph_AllArmsReachBoss(t *testing.T) {
	g := zoneUnderdarkGraph()
	for _, leaf := range []string{
		"underdark.drow_captain",
		"underdark.mind_flayer",
		"underdark.deep_chasm",
	} {
		if !reachable(g, leaf, "underdark.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}

// TestUnderdarkGraph_TrapAnchor verifies D1-d added the missing R1 Trap
// (collapsed_arch) and D10 added the illithid-arm Trap (silenced_chamber).
func TestUnderdarkGraph_TrapAnchor(t *testing.T) {
	g := zoneUnderdarkGraph()
	var trapCount int
	for _, n := range g.Nodes {
		if n.Kind == NodeKindTrap {
			trapCount++
		}
	}
	if trapCount != 2 {
		t.Errorf("trap nodes = %d, want 2 (collapsed_arch + D10 silenced_chamber)", trapCount)
	}
	if g.Nodes["underdark.silenced_chamber"].Kind != NodeKindTrap {
		t.Error("D10: silenced_chamber (illithid arm) should be a Trap")
	}
}

// TestUnderdarkGraph_EliteAnchors verifies the per-arm elites (drow
// Captain, illithid Mind Flayer) plus the D10 region-guardian elite at
// the drow→throne boundary (drow_gate), so the drow arm carries two
// elites while the chasm spur stays anchor-light.
func TestUnderdarkGraph_EliteAnchors(t *testing.T) {
	g := zoneUnderdarkGraph()
	var eliteCount int
	for _, n := range g.Nodes {
		if n.Kind == NodeKindElite {
			eliteCount++
		}
	}
	if eliteCount != 3 {
		t.Errorf("elite nodes = %d, want 3 (drow_captain + mind_flayer + D10 drow_gate)", eliteCount)
	}
	if g.Nodes["underdark.drow_gate"].Kind != NodeKindElite {
		t.Error("D10: drow_gate (R2→R4 boundary) should be a region-guardian Elite")
	}
}
