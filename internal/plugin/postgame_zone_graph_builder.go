package plugin

// Shared construction for the five Tier 6 "Mythic" post-game zone graphs
// (Phase P4). All five share one topology so the entry→boss longest walk
// lands deterministically in the T6 [44,52] band (46 by construction) and
// so the four regions are evenly weighted sub-dungeons. Per-zone identity
// lives entirely in the pgZoneSpec labels; structural set-pieces (Ossuary's
// SECRET Verses, the First Hoard's game-max LootBias gilded route) ride the
// Overrides map so the skeleton stays flavor-agnostic.
//
// Topology (counts give the longest walk = 46 nodes; every fork's first
// spur is a LockNone edge so no run can be stranded — the graph-wide
// TestZoneGraphs_NoSoftLockedFork invariant):
//
//   R1 (region[0]):  entry ─ p1..p12 ─ fork1                          (14)
//   R2 (region[1]):  fork1 ⇒ {free | locked} 3-spur ─ r2gate
//                          ─ r2b1..6 ─ fork2                          (+11)
//   R3 (region[2]):  fork2 ⇒ {free | locked} 3-spur ─ r3gate
//                          ─ r3b1..6 ─ fork3                          (+11)
//   R4 (region[3]):  fork3 ⇒ 3× capstone 3-spur ─ merge
//                          ─ ap1..5 ─ boss                            (+10)
//
// fork1 / fork2 are binary (free + one locked spur); fork3 is the ternary
// capstone (free + two locked spurs), each capstone leaf reaching the boss
// through the merge. Node-id suffixes are stable and referenced by the
// per-zone tests: entry, p1..p12, fork1, f1a1..3 (free) / f1b1..3 (locked),
// r2gate, r2b1..6, fork2, f2a1..3 / f2b1..3, r3gate, r3b1..6, fork3,
// cap11..13 (free) / cap21..23 (lockA) / cap31..33 (lockB), merge, ap1..5,
// boss. Baseline anchors (a mid-run trap + two region-guardian elites) are
// stamped by the builder; a spec Override can retype any node on top.

import "fmt"

// pgLock describes the gate on a fork's spur-entry edge. The zero value is
// an unlocked (LockNone) edge.
type pgLock struct {
	kind     ZoneEdgeLockKind
	lockData map[string]any
	hint     string
}

// pgNodeOverride retypes and/or biases a single node by suffix. An empty
// kind leaves the builder's default; a zero bias leaves LootBias unset.
type pgNodeOverride struct {
	kind ZoneNodeKind
	bias float64
}

// pgFork is a binary fork: a free (LockNone) 3-spur and a locked 3-spur,
// both converging on the region's gate node.
type pgFork struct {
	label     string
	freeLabel [3]string
	lockLabel [3]string
	lock      pgLock
}

// pgCapstone is the ternary fork3: one free spur and two independently
// locked spurs, all converging on the merge.
type pgCapstone struct {
	label      string
	freeLabel  [3]string
	lockALabel [3]string
	lockA      pgLock
	lockBLabel [3]string
	lockB      pgLock
}

type pgZoneSpec struct {
	zoneID    ZoneID
	regions   [4]string
	entry     string
	preamble  [12]string
	fork1     pgFork
	r2gate    string
	r2build   [6]string
	fork2     pgFork
	r3gate    string
	r3build   [6]string
	capstone  pgCapstone // fork3
	merge     string
	approach  [5]string
	boss      string
	overrides map[string]pgNodeOverride // keyed by node-id suffix (no zone prefix)
}

// buildPostgameZoneGraph assembles and validates the canonical T6 graph.
func buildPostgameZoneGraph(s pgZoneSpec) ZoneGraph {
	zid := string(s.zoneID)
	// Baseline anchors so every T6 zone carries a mid-run hazard and two
	// region-guardian elites without repeating them in each spec. Spec
	// overrides win (applied after).
	defaults := map[string]ZoneNodeKind{
		"p7":   NodeKindTrap,
		"r2b6": NodeKindElite,
		"r3b6": NodeKindElite,
	}

	var nodes []ZoneNode
	var edges []ZoneEdge
	id := func(suffix string) string { return zid + "." + suffix }

	addNode := func(suffix, label, region string, kind ZoneNodeKind, x, y int) {
		if d, ok := defaults[suffix]; ok {
			kind = d
		}
		n := ZoneNode{
			NodeID: id(suffix), Kind: kind, RegionID: region,
			Label: label, PosX: x, PosY: y,
		}
		if ov, ok := s.overrides[suffix]; ok {
			if ov.kind != "" {
				n.Kind = ov.kind
			}
			if ov.bias != 0 {
				n.Content.LootBias = ov.bias
			}
		}
		if n.Kind == NodeKindEntry {
			n.IsEntry = true
		}
		if n.Kind == NodeKindBoss {
			n.IsBoss = true
		}
		nodes = append(nodes, n)
	}
	addEdge := func(from, to string, lock pgLock, weight int) {
		e := ZoneEdge{From: id(from), To: id(to), Weight: weight, Lock: LockNone}
		if lock.kind != "" {
			e.Lock = lock.kind
			e.LockData = lock.lockData
			e.Hint = lock.hint
		}
		edges = append(edges, e)
	}
	// spur wires forkSuf → three spur nodes → gateSuf, locking only the
	// fork→spur[0] entry edge. The SECRET/LootBias node in a locked spur is
	// its middle node (index 1), reached by a LockNone edge — matching the
	// established "gated threshold, then unlocked entry into the secret"
	// pattern (see zone_graph_abyss_portal.go reality_seam).
	spur := func(forkSuf string, labels [3]string, lock pgLock, gateSuf, region, sufPrefix string, xStart, y, weight int) {
		prev := forkSuf
		for i := 0; i < 3; i++ {
			suf := fmt.Sprintf("%s%d", sufPrefix, i+1)
			addNode(suf, labels[i], region, NodeKindExploration, xStart+i, y)
			lk := pgLock{}
			if i == 0 {
				lk = lock
			}
			addEdge(prev, suf, lk, weight)
			prev = suf
		}
		addEdge(prev, gateSuf, pgLock{}, 1)
	}

	r1, r2, r3, r4 := s.regions[0], s.regions[1], s.regions[2], s.regions[3]

	// R1 preamble.
	addNode("entry", s.entry, r1, NodeKindEntry, 0, 2)
	prev := "entry"
	for i, lab := range s.preamble {
		suf := fmt.Sprintf("p%d", i+1)
		addNode(suf, lab, r1, NodeKindExploration, i+1, 2)
		addEdge(prev, suf, pgLock{}, 1)
		prev = suf
	}
	addNode("fork1", s.fork1.label, r1, NodeKindFork, 13, 2)
	addEdge(prev, "fork1", pgLock{}, 1)

	// R2: fork1 spurs → r2gate → buildup → fork2.
	addNode("r2gate", s.r2gate, r2, NodeKindExploration, 17, 2)
	spur("fork1", s.fork1.freeLabel, pgLock{}, "r2gate", r2, "f1a", 14, 1, 1)
	spur("fork1", s.fork1.lockLabel, s.fork1.lock, "r2gate", r2, "f1b", 14, 3, 2)
	prev = "r2gate"
	for i, lab := range s.r2build {
		suf := fmt.Sprintf("r2b%d", i+1)
		addNode(suf, lab, r2, NodeKindExploration, 18+i, 2)
		addEdge(prev, suf, pgLock{}, 1)
		prev = suf
	}
	addNode("fork2", s.fork2.label, r2, NodeKindFork, 24, 2)
	addEdge(prev, "fork2", pgLock{}, 1)

	// R3: fork2 spurs → r3gate → buildup → fork3.
	addNode("r3gate", s.r3gate, r3, NodeKindExploration, 28, 2)
	spur("fork2", s.fork2.freeLabel, pgLock{}, "r3gate", r3, "f2a", 25, 1, 1)
	spur("fork2", s.fork2.lockLabel, s.fork2.lock, "r3gate", r3, "f2b", 25, 3, 2)
	prev = "r3gate"
	for i, lab := range s.r3build {
		suf := fmt.Sprintf("r3b%d", i+1)
		addNode(suf, lab, r3, NodeKindExploration, 29+i, 2)
		addEdge(prev, suf, pgLock{}, 1)
		prev = suf
	}
	addNode("fork3", s.capstone.label, r3, NodeKindFork, 35, 2)
	addEdge(prev, "fork3", pgLock{}, 1)

	// R4: fork3 ternary capstone → merge → approach → boss.
	addNode("merge", s.merge, r4, NodeKindMerge, 39, 2)
	spur("fork3", s.capstone.freeLabel, pgLock{}, "merge", r4, "cap1", 36, 1, 1)
	spur("fork3", s.capstone.lockALabel, s.capstone.lockA, "merge", r4, "cap2", 36, 2, 2)
	spur("fork3", s.capstone.lockBLabel, s.capstone.lockB, "merge", r4, "cap3", 36, 3, 3)
	prev = "merge"
	for i, lab := range s.approach {
		suf := fmt.Sprintf("ap%d", i+1)
		addNode(suf, lab, r4, NodeKindExploration, 40+i, 2)
		addEdge(prev, suf, pgLock{}, 1)
		prev = suf
	}
	addNode("boss", s.boss, r4, NodeKindBoss, 45, 2)
	addEdge(prev, "boss", pgLock{}, 1)

	return buildPostgameGraphFrom(s.zoneID, nodes, edges)
}

// buildPostgameGraphFrom is a thin seam over BuildGraph so tests can build
// a spec's graph without re-registering it.
func buildPostgameGraphFrom(zoneID ZoneID, nodes []ZoneNode, edges []ZoneEdge) ZoneGraph {
	return BuildGraph(zoneID, nodes, edges)
}
