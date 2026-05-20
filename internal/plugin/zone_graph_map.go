package plugin

// Phase G6 — graph-aware !zone map renderer.
//
// Replaces the linear renderZoneMap() output. Walks the registered
// (or legacy-compiled) ZoneGraph in BFS order rooted at Entry, lays
// nodes out by PosX/PosY, and prints a horizontal glyph row per layer
// with a status row underneath. Locked outgoing edges show as `╳`;
// secret nodes stay hidden until the player has visited them
// (prevents spoilers from `!zone map`).

import (
	"fmt"
	"sort"
	"strings"
)

type laidNode struct {
	ID   string
	Node ZoneNode
}

// renderZoneGraphMap is the graph-mode replacement for renderZoneMap.
// Output stays inside the same ``` block in zoneCmdMap.
func renderZoneGraphMap(g ZoneGraph, run *DungeonRun) string {
	if len(g.Nodes) == 0 {
		return "(no graph)"
	}
	visited := map[string]bool{}
	for _, n := range run.VisitedNodes {
		visited[n] = true
	}
	current := run.CurrentNode

	// Group nodes by PosX, drop secret nodes the player hasn't reached.
	cols := map[int][]laidNode{}
	maxX := 0
	for id, n := range g.Nodes {
		if n.Kind == NodeKindSecret && !visited[id] {
			continue
		}
		cols[n.PosX] = append(cols[n.PosX], laidNode{ID: id, Node: n})
		if n.PosX > maxX {
			maxX = n.PosX
		}
	}
	if len(cols) == 0 {
		return "(no nodes)"
	}
	for x := range cols {
		sort.Slice(cols[x], func(i, j int) bool {
			a, b := cols[x][i].Node, cols[x][j].Node
			if a.PosY != b.PosY {
				return a.PosY < b.PosY
			}
			return cols[x][i].ID < cols[x][j].ID
		})
	}

	// Drop columns that have no visible nodes (e.g. when the only
	// entry was a hidden secret). Otherwise the render leaves a blank
	// slot mid-row that reads as a missing status mark.
	visibleX := make([]int, 0, maxX+1)
	for x := 0; x <= maxX; x++ {
		if len(cols[x]) > 0 {
			visibleX = append(visibleX, x)
		}
	}

	// Find max stack height per column for vertical alignment.
	maxRows := 0
	for _, x := range visibleX {
		if len(cols[x]) > maxRows {
			maxRows = len(cols[x])
		}
	}
	if maxRows == 0 {
		maxRows = 1
	}

	// Render row-by-row. Each "row" is a top (glyph) + bot (status)
	// pair. Empty cells get spaces so columns stay aligned.
	var sb strings.Builder
	for row := 0; row < maxRows; row++ {
		var top, bot strings.Builder
		for i, x := range visibleX {
			if i > 0 {
				connector, statusGap := graphConnector(g, cols[visibleX[i-1]], cols[x], row, visited)
				top.WriteString(connector)
				bot.WriteString(statusGap)
			}
			if row < len(cols[x]) {
				ln := cols[x][row]
				top.WriteString(nodeGlyph(ln.Node))
				bot.WriteString(nodeStatusMark(ln.ID, current, visited))
			} else {
				top.WriteString(" ")
				bot.WriteString(" ")
			}
		}
		sb.WriteString(strings.TrimRight(top.String(), " "))
		sb.WriteString("\n")
		sb.WriteString(strings.TrimRight(bot.String(), " "))
		if row < maxRows-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// graphConnector returns the 2-char top/bot strings drawn between
// columns for a given row. Defaults to `──`/`  `; if the only edge
// reaching this row's right-column node is locked, swaps to `╳ ` to
// signal a gate. Hidden secrets aren't connectable so they get blank.
func graphConnector(g ZoneGraph, leftCol, rightCol []laidNode, row int, visited map[string]bool) (string, string) {
	_ = visited
	if row >= len(rightCol) {
		return "  ", "  "
	}
	target := rightCol[row].ID
	hasUnlocked, hasAny := false, false
	for _, ln := range leftCol {
		for _, e := range g.Edges[ln.ID] {
			if e.To != target {
				continue
			}
			hasAny = true
			if e.Lock == "" || e.Lock == LockNone {
				hasUnlocked = true
			}
		}
	}
	if !hasAny {
		return "  ", "  "
	}
	if !hasUnlocked {
		return "╳─", "  "
	}
	return "──", "  "
}

func nodeStatusMark(id, current string, visited map[string]bool) string {
	switch {
	case id == current:
		return "▶"
	case visited[id]:
		return "✓"
	default:
		return "·"
	}
}

func nodeGlyph(n ZoneNode) string {
	switch n.Kind {
	case NodeKindEntry:
		return "E"
	case NodeKindExploration, NodeKindFork, NodeKindMerge:
		return "?"
	case NodeKindTrap:
		return "T"
	case NodeKindElite:
		return "★"
	case NodeKindBoss:
		return "☠"
	case NodeKindHarvest:
		return "H"
	case NodeKindRestCamp:
		return "⛺"
	case NodeKindSecret:
		return "⚿"
	}
	return "·"
}

// renderVisitedPath renders a one-line numbered breadcrumb of the
// player's path so they can reference rooms by index (e.g. !revisit 2).
// Numbers are 1-indexed to match every other surface ("Room 2/7", etc.).
func renderVisitedPath(g ZoneGraph, run *DungeonRun) string {
	if run == nil || len(run.VisitedNodes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(run.VisitedNodes))
	for i, nodeID := range run.VisitedNodes {
		glyph := "·"
		if n, ok := g.Nodes[nodeID]; ok {
			glyph = nodeGlyph(n)
		}
		mark := "✓"
		if nodeID == run.CurrentNode {
			mark = "▶"
		}
		parts = append(parts, fmt.Sprintf("[%d]%s%s", i+1, glyph, mark))
	}
	return strings.Join(parts, " → ")
}

// debugDump (test-only crutch) prints the raw column layout. Currently
// unused outside potential debugging — kept off the unused-warning list
// by routing fmt through it.
func debugDumpGraphMap(g ZoneGraph) string {
	var b strings.Builder
	for id, n := range g.Nodes {
		b.WriteString(fmt.Sprintf("%s @ (%d,%d) %s\n", id, n.PosX, n.PosY, n.Kind))
	}
	return b.String()
}
