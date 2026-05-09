package plugin

import "testing"

func TestSunkenTempleGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneSunkenTemple]
	if !ok {
		t.Fatal("zoneSunkenTempleGraph not registered")
	}
	if g.Entry != "sunken_temple.entry" {
		t.Errorf("entry node = %q", g.Entry)
	}
	if g.Boss != "sunken_temple.boss" {
		t.Errorf("boss node = %q", g.Boss)
	}
	if len(g.Nodes) != 10 {
		t.Errorf("nodes = %d, want 10", len(g.Nodes))
	}
}

// TestSunkenTempleGraph_FourDistinctPaths verifies the four leaves
// (kuo_toa_pen, glyph_chamber, aboleth_thralls, coral_reliquary) all
// reach the boss. Together with the no-merge invariant below, this
// confirms the design intent: four committed paths, not a diamond.
func TestSunkenTempleGraph_FourDistinctPaths(t *testing.T) {
	g := zoneSunkenTempleGraph()
	for _, leaf := range []string{
		"sunken_temple.kuo_toa_pen",
		"sunken_temple.glyph_chamber",
		"sunken_temple.aboleth_thralls",
		"sunken_temple.coral_reliquary",
	} {
		if !reachable(g, leaf, "sunken_temple.boss") {
			t.Errorf("%s unreachable to boss", leaf)
		}
	}
}

// TestSunkenTempleGraph_NoMidPathMerge ensures no node other than the
// boss has more than one incoming edge. If a future edit collapses the
// two subtrees into a converging diamond, this test fails — that's the
// signal to either rename the zone's shape comment or re-author.
func TestSunkenTempleGraph_NoMidPathMerge(t *testing.T) {
	g := zoneSunkenTempleGraph()
	incoming := map[string]int{}
	for _, outs := range g.Edges {
		for _, e := range outs {
			incoming[e.To]++
		}
	}
	for id, n := range g.Nodes {
		if n.IsBoss {
			continue
		}
		if incoming[id] > 1 {
			t.Errorf("node %q has %d incoming edges, want ≤1 (no mid-path merge)", id, incoming[id])
		}
	}
	// Boss intentionally has 4 incoming edges (one per leaf).
	if incoming["sunken_temple.boss"] != 4 {
		t.Errorf("boss incoming = %d, want 4", incoming["sunken_temple.boss"])
	}
}

func TestSunkenTempleGraph_LockKindCoverage(t *testing.T) {
	g := zoneSunkenTempleGraph()
	kinds := map[ZoneEdgeLockKind]bool{}
	for _, outs := range g.Edges {
		for _, e := range outs {
			kinds[e.Lock] = true
		}
	}
	// Sunken Temple deliberately exercises Perception + StatCheck +
	// LockNone in one zone.
	for _, k := range []ZoneEdgeLockKind{LockNone, LockPerception, LockStatCheck} {
		if !kinds[k] {
			t.Errorf("missing lock kind %s — variety check", k)
		}
	}
}
