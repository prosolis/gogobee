package plugin

import "testing"

func TestUnderdarkGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneUnderdark]
	if !ok {
		t.Fatal("zoneUnderdarkGraph not registered")
	}
	if len(g.Nodes) != 10 {
		t.Errorf("nodes = %d, want 10", len(g.Nodes))
	}
}

// TestUnderdarkGraph_AllNodesHaveRegion confirms Underdark is the
// canonical multi-region zone — every node carries a non-empty RegionID
// matching the dnd_expedition_region.go registry.
func TestUnderdarkGraph_AllNodesHaveRegion(t *testing.T) {
	g := zoneUnderdarkGraph()
	validRegions := map[string]bool{
		"underdark_surface_tunnels":  true,
		"underdark_drow_outpost":     true,
		"underdark_illithid_warren":  true,
		"underdark_deep_throne":      true,
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
