package plugin

import (
	"strings"
	"testing"
)

// TestRenderZoneGraphMap_LinearLooksFamiliar exercises the linear-graph
// case so the new renderer doesn't regress the legacy single-row look.
func TestRenderZoneGraphMap_LinearLooksFamiliar(t *testing.T) {
	g := BuildLinearGraph(ZoneGoblinWarrens, []ZoneNodeKind{
		NodeKindEntry, NodeKindExploration, NodeKindTrap, NodeKindBoss,
	})
	run := &DungeonRun{
		ZoneID:       ZoneGoblinWarrens,
		CurrentNode:  "goblin_warrens.r2",
		VisitedNodes: []string{"goblin_warrens.r1", "goblin_warrens.r2"},
	}
	got := renderZoneGraphMap(g, run)
	// Top row should contain glyphs in order.
	if !strings.Contains(got, "E──?──T──☠") {
		t.Errorf("missing linear glyph row in:\n%s", got)
	}
	// Status row: first cleared, second current, rest pending.
	if !strings.Contains(got, "✓") || !strings.Contains(got, "▶") || !strings.Contains(got, "·") {
		t.Errorf("missing status markers in:\n%s", got)
	}
}

// TestRenderZoneGraphMap_HidesUnvisitedSecrets confirms the spoiler
// guard: secret nodes don't render until the player has reached them.
func TestRenderZoneGraphMap_HidesUnvisitedSecrets(t *testing.T) {
	nodes := []ZoneNode{
		{NodeID: "z.entry", Kind: NodeKindEntry, IsEntry: true, PosX: 0},
		{NodeID: "z.boss", Kind: NodeKindBoss, IsBoss: true, PosX: 1},
		{NodeID: "z.secret", Kind: NodeKindSecret, PosX: 1, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "z.entry", To: "z.boss"},
		{From: "z.entry", To: "z.secret", Lock: LockPerception, LockData: map[string]any{"dc": 12}},
	}
	g := BuildGraph("test_zone", nodes, edges)

	hidden := renderZoneGraphMap(g, &DungeonRun{
		CurrentNode:  "z.entry",
		VisitedNodes: []string{"z.entry"},
	})
	if strings.Contains(hidden, "⚿") {
		t.Errorf("unvisited secret should not render, got:\n%s", hidden)
	}
	// Once visited, the glyph appears.
	revealed := renderZoneGraphMap(g, &DungeonRun{
		CurrentNode:  "z.secret",
		VisitedNodes: []string{"z.entry", "z.secret"},
	})
	if !strings.Contains(revealed, "⚿") {
		t.Errorf("visited secret should render, got:\n%s", revealed)
	}
}

// TestRenderZoneGraphMap_LockedEdgeRenderedAsCross verifies locked-only
// approaches show ╳ instead of ──.
func TestRenderZoneGraphMap_LockedEdgeRenderedAsCross(t *testing.T) {
	nodes := []ZoneNode{
		{NodeID: "z.entry", Kind: NodeKindEntry, IsEntry: true, PosX: 0},
		{NodeID: "z.gate", Kind: NodeKindExploration, PosX: 1},
		{NodeID: "z.boss", Kind: NodeKindBoss, IsBoss: true, PosX: 2},
	}
	edges := []ZoneEdge{
		{From: "z.entry", To: "z.gate", Lock: LockKey, LockData: map[string]any{"key_id": "rune"}},
		{From: "z.gate", To: "z.boss"},
	}
	g := BuildGraph("test_zone_locked", nodes, edges)
	got := renderZoneGraphMap(g, &DungeonRun{
		CurrentNode:  "z.entry",
		VisitedNodes: []string{"z.entry"},
	})
	if !strings.Contains(got, "╳") {
		t.Errorf("locked edge should render as ╳, got:\n%s", got)
	}
}
