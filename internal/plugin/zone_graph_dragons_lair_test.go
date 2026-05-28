package plugin

import "testing"

func TestDragonsLairGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneDragonsLair]
	if !ok {
		t.Fatal("zoneDragonsLairGraph not registered")
	}
	// Long-expedition D1-e widened this zone from 12 → 47 nodes so the
	// longest entry→boss walk lands in the T5 [36,44] traversal band.
	if len(g.Nodes) != 47 {
		t.Errorf("nodes = %d, want 47", len(g.Nodes))
	}
}

func TestDragonsLairGraph_LongestPathInBand(t *testing.T) {
	g := zoneDragonsLairGraph()
	got := graphLongestPath(g)
	if got < 36 || got > 44 {
		t.Errorf("longest path = %d, want in T5 band [36,44]", got)
	}
}

// TestDragonsLairGraph_AllNodesHaveRegion confirms D1-e backfilled the
// missing RegionID authoring per dnd_expedition_region.go: every node
// carries a non-empty RegionID matching the registry.
func TestDragonsLairGraph_AllNodesHaveRegion(t *testing.T) {
	g := zoneDragonsLairGraph()
	validRegions := map[string]bool{
		"dragons_lair_kobold_warrens":   true,
		"dragons_lair_drake_pens":       true,
		"dragons_lair_the_vault":        true,
		"dragons_lair_infernax_chamber": true,
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

// TestDragonsLairGraph_AllFourRegionsRepresented confirms each authored
// region has at least one node.
func TestDragonsLairGraph_AllFourRegionsRepresented(t *testing.T) {
	g := zoneDragonsLairGraph()
	regions := map[string]int{}
	for _, n := range g.Nodes {
		regions[n.RegionID]++
	}
	for _, r := range []string{
		"dragons_lair_kobold_warrens",
		"dragons_lair_drake_pens",
		"dragons_lair_the_vault",
		"dragons_lair_infernax_chamber",
	} {
		if regions[r] == 0 {
			t.Errorf("region %q has no nodes — multi-region invariant broken", r)
		}
	}
}

// TestDragonsLairGraph_Fork1Converges verifies the binary mid-fork's
// two spurs both converge at wyrmlings_nest (R3 boundary).
func TestDragonsLairGraph_Fork1Converges(t *testing.T) {
	g := zoneDragonsLairGraph()
	for _, spurTail := range []string{"dragons_lair.cinder_walk", "dragons_lair.vault_passage"} {
		outs := g.outgoingEdges(spurTail)
		if len(outs) != 1 || outs[0].To != "dragons_lair.wyrmlings_nest" {
			t.Errorf("%s outs = %+v, want single edge to wyrmlings_nest", spurTail, outs)
		}
	}
}

// TestDragonsLairGraph_Fork2Capstone verifies the late fork has 3
// distinct paths to the boss, each through its own capstone node.
func TestDragonsLairGraph_Fork2Capstone(t *testing.T) {
	g := zoneDragonsLairGraph()
	outs := g.outgoingEdges("dragons_lair.fork2")
	if len(outs) != 3 {
		t.Fatalf("fork2 outgoing = %d, want 3", len(outs))
	}
	for _, leaf := range []string{
		"dragons_lair.direct_confrontation",
		"dragons_lair.dragon_bargain",
		"dragons_lair.hoard_pillar",
	} {
		if !reachable(g, leaf, "dragons_lair.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
	// Locks escalate: open / CHA 16 / Perception 17 (T5 secret bias).
	for _, e := range outs {
		if e.To == "dragons_lair.hidden_passage" {
			if dc := lockDataInt(e.LockData, "dc", 0); dc < 17 {
				t.Errorf("hoard_pillar spur DC = %d, want >= 17 (T5)", dc)
			}
		}
	}
}

// TestDragonsLairGraph_LootBiasEscalation verifies the T5 secret has
// the highest loot bias of any authored zone (≥ 2.5).
func TestDragonsLairGraph_LootBiasEscalation(t *testing.T) {
	g := zoneDragonsLairGraph()
	hoard := g.Nodes["dragons_lair.hoard_pillar"]
	if hoard.Content.LootBias < 2.5 {
		t.Errorf("hoard_pillar LootBias = %v, want >= 2.5 (T5 secret)", hoard.Content.LootBias)
	}
}
