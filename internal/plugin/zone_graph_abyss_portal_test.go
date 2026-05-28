package plugin

import "testing"

func TestAbyssPortalGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneAbyssPortal]
	if !ok {
		t.Fatal("zoneAbyssPortalGraph not registered")
	}
	// Long-expedition D1-e widened this zone from 13 → 51 nodes so the
	// longest entry→boss walk lands in the T5 [36,44] traversal band.
	if len(g.Nodes) != 51 {
		t.Errorf("nodes = %d, want 51", len(g.Nodes))
	}
}

func TestAbyssPortalGraph_LongestPathInBand(t *testing.T) {
	g := zoneAbyssPortalGraph()
	got := graphLongestPath(g)
	if got < 36 || got > 44 {
		t.Errorf("longest path = %d, want in T5 band [36,44]", got)
	}
}

// TestAbyssPortalGraph_AllNodesHaveRegion confirms D1-e backfilled the
// missing RegionID authoring per dnd_expedition_region.go: every node
// carries a non-empty RegionID matching the registry.
func TestAbyssPortalGraph_AllNodesHaveRegion(t *testing.T) {
	g := zoneAbyssPortalGraph()
	validRegions := map[string]bool{
		"abyss_outer_rift":     true,
		"abyss_demon_assembly": true,
		"abyss_wardens_post":   true,
		"abyss_the_tear":       true,
	}
	for id, n := range g.Nodes {
		if n.RegionID == "" {
			t.Errorf("node %s has empty RegionID — D1-e requires region authoring on every node", id)
		}
		if !validRegions[n.RegionID] {
			t.Errorf("node %s RegionID = %q, not in dnd_expedition_region.go registry", id, n.RegionID)
		}
	}
}

// TestAbyssPortalGraph_AllFourRegionsRepresented confirms each authored
// region has at least one node.
func TestAbyssPortalGraph_AllFourRegionsRepresented(t *testing.T) {
	g := zoneAbyssPortalGraph()
	regions := map[string]int{}
	for _, n := range g.Nodes {
		regions[n.RegionID]++
	}
	for _, r := range []string{
		"abyss_outer_rift",
		"abyss_demon_assembly",
		"abyss_wardens_post",
		"abyss_the_tear",
	} {
		if regions[r] == 0 {
			t.Errorf("region %q has no nodes — multi-region invariant broken", r)
		}
	}
}

// TestAbyssPortalGraph_ThreeSequentialForks confirms the design shape:
// three fork nodes in series, each binary or ternary.
func TestAbyssPortalGraph_ThreeSequentialForks(t *testing.T) {
	g := zoneAbyssPortalGraph()
	wants := map[string]int{
		"abyss_portal.fork1": 2,
		"abyss_portal.fork2": 2,
		"abyss_portal.fork3": 3,
	}
	for id, want := range wants {
		if got := len(g.outgoingEdges(id)); got != want {
			t.Errorf("%s outgoing = %d, want %d", id, got, want)
		}
	}
}

func TestAbyssPortalGraph_AllCapstoneLeavesReachBoss(t *testing.T) {
	g := zoneAbyssPortalGraph()
	for _, leaf := range []string{
		"abyss_portal.direct_assault",
		"abyss_portal.usurper_throne",
		"abyss_portal.reality_seam",
	} {
		if !reachable(g, leaf, "abyss_portal.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}

// TestAbyssPortalGraph_FullStatRosterCoverage confirms the
// project-wide claim: by G8h, all six abilities (STR/DEX/CON/INT/WIS/
// CHA) appear as authored stat-check locks across shipping zones.
// CON is the missing one prior to this zone — locked here on
// mind_corridor.
func TestAbyssPortalGraph_FullStatRosterCoverage(t *testing.T) {
	g := zoneAbyssPortalGraph()
	conSeen := false
	for _, outs := range g.Edges {
		for _, e := range outs {
			if e.Lock == LockStatCheck && lockDataString(e.LockData, "stat") == "CON" {
				conSeen = true
			}
		}
	}
	if !conSeen {
		t.Error("expected at least one CON stat-check edge — completes ability roster by G8h")
	}
}

func TestAbyssPortalGraph_RealitySeamHighestBias(t *testing.T) {
	g := zoneAbyssPortalGraph()
	seam := g.Nodes["abyss_portal.reality_seam"]
	if seam.Content.LootBias < 3.0 {
		t.Errorf("reality_seam LootBias = %v, want >= 3.0 (Abyss capstone)", seam.Content.LootBias)
	}
}
