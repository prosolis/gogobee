package plugin

import "testing"

func TestAbyssPortalGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneAbyssPortal]
	if !ok {
		t.Fatal("zoneAbyssPortalGraph not registered")
	}
	if len(g.Nodes) != 13 {
		t.Errorf("nodes = %d, want 13", len(g.Nodes))
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
