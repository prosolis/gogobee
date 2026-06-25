package plugin

import "testing"

func TestForestShadowsGraph_Registered(t *testing.T) {
	g, ok := zoneGraphRegistry[ZoneForestShadows]
	if !ok {
		t.Fatal("zoneForestShadowsGraph not registered")
	}
	if g.Entry != "forest_shadows.entry" {
		t.Errorf("entry node = %q, want forest_shadows.entry", g.Entry)
	}
	if g.Boss != "forest_shadows.boss" {
		t.Errorf("boss node = %q, want forest_shadows.boss", g.Boss)
	}
	// Long-expedition D1-b widened this zone from 9 → 19 nodes so the
	// longest entry→boss walk lands in the T2 [16,20] traversal band.
	if len(g.Nodes) != 19 {
		t.Errorf("nodes = %d, want 19", len(g.Nodes))
	}
}

func TestForestShadowsGraph_AllPathsReachBoss(t *testing.T) {
	g := zoneForestShadowsGraph()
	for _, mid := range []string{
		"forest_shadows.grove_descent",
		"forest_shadows.dryad_circle",
		"forest_shadows.thorn_tunnel",
		"forest_shadows.fork2",
		"forest_shadows.sapling_shrine",
	} {
		if !reachable(g, mid, "forest_shadows.boss") {
			t.Errorf("%s unreachable to boss", mid)
		}
	}
}

// TestForestShadowsGraph_AsymmetricMainFork captures the design intent:
// the long branch is grove_descent → dryad_circle (2 mid-nodes), the
// short branch is thorn_tunnel (1 mid-node). If a future edit
// accidentally makes the branches the same length, this test fails so
// the divergence from the diamond shape is preserved.
func TestForestShadowsGraph_AsymmetricMainFork(t *testing.T) {
	g := zoneForestShadowsGraph()
	longLen := bfsHops(g, "forest_shadows.fork1", "forest_shadows.fork2", "forest_shadows.grove_descent")
	shortLen := bfsHops(g, "forest_shadows.fork1", "forest_shadows.fork2", "forest_shadows.thorn_tunnel")
	// D1-b: long branch is now 3 mid-nodes (grove_descent → dryad_circle
	// → torn_meadow), short is 2 (thorn_tunnel → briar_warren).
	if longLen != 4 {
		t.Errorf("long branch hops = %d, want 4 (grove_descent → dryad_circle → torn_meadow → fork2)", longLen)
	}
	if shortLen != 3 {
		t.Errorf("short branch hops = %d, want 3 (thorn_tunnel → briar_warren → fork2)", shortLen)
	}
	if longLen <= shortLen {
		t.Errorf("expected asymmetric branches (long > short); got long=%d short=%d", longLen, shortLen)
	}
}

// TestForestShadowsGraph_SecretIsStatCheck verifies the secret uses
// LockStatCheck (WIS) rather than LockPerception — Forest of Shadows
// is the first authored zone exercising stat_check, so a regression
// here would silently degrade to "another perception zone".
func TestForestShadowsGraph_SecretIsStatCheck(t *testing.T) {
	g := zoneForestShadowsGraph()
	outs := g.outgoingEdges("forest_shadows.fork2")
	var secretEdge *ZoneEdge
	for i := range outs {
		if outs[i].To == "forest_shadows.sapling_shrine" {
			secretEdge = &outs[i]
			break
		}
	}
	if secretEdge == nil {
		t.Fatal("no edge from fork2 to sapling_shrine")
	}
	if secretEdge.Lock != LockStatCheck {
		t.Errorf("secret edge lock = %s, want stat_check", secretEdge.Lock)
	}
	if stat := lockDataString(secretEdge.LockData, "stat"); stat != "WIS" {
		t.Errorf("stat = %q, want WIS", stat)
	}
	if dc := lockDataInt(secretEdge.LockData, "dc", 0); dc < 14 {
		t.Errorf("DC = %d, want >= 14", dc)
	}
	if secretEdge.Hint == "" {
		t.Error("secret edge missing hint")
	}
	secret := g.Nodes["forest_shadows.sapling_shrine"]
	if secret.Content.LootBias < 1.5 {
		t.Errorf("secret LootBias = %v, want >= 1.5", secret.Content.LootBias)
	}
}

// bfsHops returns the BFS shortest-path length from `from` to `to`,
// constrained to start by going through `via`. Returns 0 if unreachable.
func bfsHops(g ZoneGraph, from, to, via string) int {
	type frame struct {
		node string
		hops int
	}
	if from == to {
		return 0
	}
	seen := map[string]bool{from: true}
	queue := []frame{}
	for _, e := range g.Edges[from] {
		if e.To == via {
			queue = append(queue, frame{e.To, 1})
			seen[e.To] = true
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.node == to {
			return cur.hops
		}
		for _, e := range g.Edges[cur.node] {
			if !seen[e.To] {
				seen[e.To] = true
				queue = append(queue, frame{e.To, cur.hops + 1})
			}
		}
	}
	return 0
}
