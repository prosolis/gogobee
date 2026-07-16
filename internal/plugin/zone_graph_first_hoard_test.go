package plugin

import "testing"

func TestFirstHoardGraph_Invariants(t *testing.T) {
	assertPostgameGraph(t, ZoneFirstHoard, zoneFirstHoardGraph)
}

// TestFirstHoardGraph_GameMaxLootBias verifies the plan's requirement that
// the First Hoard carries the game-max LootBias route (2.5–3.0), which the
// boss's Greed Tax self-balances. The deepest gilded vein hits the 3.0
// ceiling and at least three nodes sit at or above 2.5.
func TestFirstHoardGraph_GameMaxLootBias(t *testing.T) {
	g := zoneFirstHoardGraph()

	var max float64
	rich := 0
	for _, n := range g.Nodes {
		if n.Content.LootBias > max {
			max = n.Content.LootBias
		}
		if n.Content.LootBias >= 2.5 {
			rich++
		}
	}
	if max < 3.0 {
		t.Errorf("max LootBias = %v, want >= 3.0 (game-max gilded route)", max)
	}
	if rich < 3 {
		t.Errorf("nodes with LootBias >= 2.5 = %d, want >= 3", rich)
	}
}
