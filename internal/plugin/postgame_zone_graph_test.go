package plugin

import "testing"

// assertPostgameGraph runs the invariants every T6 zone graph must satisfy:
// registered, longest walk in the [44,52] band, all four authored regions
// present and matching the ExpeditionRegion registry, every fork keeps a
// free (LockNone) exit, and all three capstone leaves reach the boss.
func assertPostgameGraph(t *testing.T, zoneID ZoneID, build func() ZoneGraph) {
	t.Helper()

	if _, ok := zoneGraphRegistry[zoneID]; !ok {
		t.Fatalf("%s graph not registered", zoneID)
	}
	g := build()

	if got := graphLongestPath(g); got < 44 || got > 52 {
		t.Errorf("%s longest path = %d, want T6 band [44,52]", zoneID, got)
	}

	// Regions: every node has a RegionID drawn from the zone's registry,
	// and every registered region has at least one node.
	valid := map[string]bool{}
	for _, r := range regionsByZone[zoneID] {
		valid[r.ID] = true
	}
	if len(valid) != 4 {
		t.Fatalf("%s: expected 4 registered regions, got %d", zoneID, len(valid))
	}
	seen := map[string]int{}
	for id, n := range g.Nodes {
		if n.RegionID == "" {
			t.Errorf("%s node %s has empty RegionID", zoneID, id)
			continue
		}
		if !valid[n.RegionID] {
			t.Errorf("%s node %s RegionID %q not in region registry", zoneID, id, n.RegionID)
		}
		seen[n.RegionID]++
	}
	for r := range valid {
		if seen[r] == 0 {
			t.Errorf("%s region %q has no nodes", zoneID, r)
		}
	}

	// Every fork offers at least one LockNone exit (local mirror of the
	// global TestZoneGraphs_NoSoftLockedFork, kept here so a broken spec
	// fails in its own zone's test too).
	for id, n := range g.Nodes {
		if n.Kind != NodeKindFork {
			continue
		}
		free := false
		for _, e := range g.outgoingEdges(id) {
			if e.Lock == LockNone || e.Lock == "" {
				free = true
			}
		}
		if !free {
			t.Errorf("%s fork %s: no free exit (soft-lock)", zoneID, id)
		}
	}

	// All three capstone leaves reach the boss.
	for _, leaf := range []string{
		string(zoneID) + ".cap13",
		string(zoneID) + ".cap23",
		string(zoneID) + ".cap33",
	} {
		if _, ok := g.Nodes[leaf]; !ok {
			t.Errorf("%s missing capstone leaf %s", zoneID, leaf)
			continue
		}
		if !reachable(g, leaf, g.Boss) {
			t.Errorf("%s capstone leaf %s unreachable to boss", zoneID, leaf)
		}
	}
}
