package plugin

import (
	"strings"
	"testing"
)

func TestBuildLinearGraph_BasicShape(t *testing.T) {
	seq := []ZoneNodeKind{
		NodeKindEntry,
		NodeKindExploration,
		NodeKindElite,
		NodeKindBoss,
	}
	g := BuildLinearGraph("test_zone", seq)
	if err := validateZoneGraph(g); err != nil {
		t.Fatalf("linear graph should validate: %v", err)
	}
	if len(g.Nodes) != 4 {
		t.Fatalf("want 4 nodes, got %d", len(g.Nodes))
	}
	if g.Entry == "" || g.Boss == "" {
		t.Fatalf("entry/boss not set: entry=%q boss=%q", g.Entry, g.Boss)
	}
	if !reachable(g, g.Entry, g.Boss) {
		t.Fatalf("boss unreachable from entry")
	}
}

func TestBuildLinearGraph_Empty(t *testing.T) {
	g := BuildLinearGraph("empty", nil)
	if err := validateZoneGraph(g); err == nil {
		t.Fatalf("empty graph should fail validation")
	}
}

func TestValidateZoneGraph_OrphanNode(t *testing.T) {
	nodes := []ZoneNode{
		{NodeID: "e", Kind: NodeKindEntry, IsEntry: true},
		{NodeID: "b", Kind: NodeKindBoss, IsBoss: true},
		{NodeID: "orphan", Kind: NodeKindExploration},
	}
	edges := []ZoneEdge{{From: "e", To: "b"}}
	g := ZoneGraph{
		ZoneID: "z",
		Nodes:  map[string]ZoneNode{},
		Edges:  map[string][]ZoneEdge{},
	}
	for _, n := range nodes {
		g.Nodes[n.NodeID] = n
		if n.IsEntry {
			g.Entry = n.NodeID
		}
		if n.IsBoss {
			g.Boss = n.NodeID
		}
	}
	for _, e := range edges {
		g.Edges[e.From] = append(g.Edges[e.From], e)
	}
	err := validateZoneGraph(g)
	if err == nil || !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("expected orphan error, got %v", err)
	}
}

func TestValidateZoneGraph_BossUnreachable(t *testing.T) {
	// b has an incoming self-loop (so it's not orphaned) but is not
	// reachable from entry — exercises the BFS reachability check.
	g := ZoneGraph{
		ZoneID: "z",
		Nodes: map[string]ZoneNode{
			"e": {NodeID: "e", Kind: NodeKindEntry, IsEntry: true},
			"b": {NodeID: "b", Kind: NodeKindBoss, IsBoss: true,
				Content: ZoneNodeContent{AllowSelfLoop: true}},
		},
		Edges: map[string][]ZoneEdge{
			"b": {{From: "b", To: "b"}},
		},
		Entry: "e",
		Boss:  "b",
	}
	err := validateZoneGraph(g)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("expected unreachable error, got %v", err)
	}
}

func TestValidateZoneGraph_TwoEntries(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on two entries")
		}
	}()
	BuildGraph("z",
		[]ZoneNode{
			{NodeID: "e1", Kind: NodeKindEntry, IsEntry: true},
			{NodeID: "e2", Kind: NodeKindEntry, IsEntry: true},
			{NodeID: "b", Kind: NodeKindBoss, IsBoss: true},
		},
		[]ZoneEdge{
			{From: "e1", To: "b"},
			{From: "e2", To: "b"},
		},
	)
}

func TestValidateZoneGraph_SelfLoopRejected(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on self-loop without opt-in")
		}
	}()
	BuildGraph("z",
		[]ZoneNode{
			{NodeID: "e", Kind: NodeKindEntry, IsEntry: true},
			{NodeID: "b", Kind: NodeKindBoss, IsBoss: true},
		},
		[]ZoneEdge{
			{From: "e", To: "e"},
			{From: "e", To: "b"},
		},
	)
}

func TestValidateZoneGraph_SelfLoopAllowed(t *testing.T) {
	g := BuildGraph("z",
		[]ZoneNode{
			{NodeID: "e", Kind: NodeKindEntry, IsEntry: true,
				Content: ZoneNodeContent{AllowSelfLoop: true}},
			{NodeID: "b", Kind: NodeKindBoss, IsBoss: true},
		},
		[]ZoneEdge{
			{From: "e", To: "e"},
			{From: "e", To: "b"},
		},
	)
	if err := validateZoneGraph(g); err != nil {
		t.Fatalf("self-loop with opt-in should validate: %v", err)
	}
}

func TestBuildGraph_BranchedShape(t *testing.T) {
	g := BuildGraph("crypt",
		[]ZoneNode{
			{NodeID: "entry", Kind: NodeKindEntry, IsEntry: true},
			{NodeID: "fork", Kind: NodeKindFork},
			{NodeID: "left", Kind: NodeKindElite},
			{NodeID: "right", Kind: NodeKindExploration},
			{NodeID: "merge", Kind: NodeKindMerge},
			{NodeID: "boss", Kind: NodeKindBoss, IsBoss: true},
		},
		[]ZoneEdge{
			{From: "entry", To: "fork"},
			{From: "fork", To: "left", Weight: 1},
			{From: "fork", To: "right", Weight: 2,
				Lock: LockPerception, LockData: map[string]any{"dc": 12}},
			{From: "left", To: "merge"},
			{From: "right", To: "merge"},
			{From: "merge", To: "boss"},
		},
	)
	outs := g.outgoingEdges("fork")
	if len(outs) != 2 {
		t.Fatalf("want 2 fork edges, got %d", len(outs))
	}
	if outs[0].To != "left" {
		t.Fatalf("weight ordering broken: %v", outs)
	}
}

func TestCompileLegacyZoneGraph_AllRegistered(t *testing.T) {
	for _, z := range allZones() {
		g, ok := loadZoneGraph(z.ID)
		if !ok {
			t.Errorf("loadZoneGraph(%q): not ok", z.ID)
			continue
		}
		if err := validateZoneGraph(g); err != nil {
			t.Errorf("zone %q: legacy graph invalid: %v", z.ID, err)
		}
		if g.Entry == "" || g.Boss == "" {
			t.Errorf("zone %q: missing entry/boss", z.ID)
		}
		// Boss reachability is enforced by validate, but assert again for
		// clarity.
		if !reachable(g, g.Entry, g.Boss) {
			t.Errorf("zone %q: boss unreachable from entry", z.ID)
		}
	}
}

func TestDeriveLegacyNodeID_StableShape(t *testing.T) {
	got := deriveLegacyNodeID("crypt_valdris", 0)
	want := "crypt_valdris.r1"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	got = deriveLegacyNodeID("crypt_valdris", 4)
	want = "crypt_valdris.r5"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestRegisterZoneGraph_DuplicateRejected(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate registration")
		}
		// scrub registry side-effect from this test
		delete(zoneGraphRegistry, "dup_test")
	}()
	g := BuildGraph("dup_test",
		[]ZoneNode{
			{NodeID: "e", Kind: NodeKindEntry, IsEntry: true},
			{NodeID: "b", Kind: NodeKindBoss, IsBoss: true},
		},
		[]ZoneEdge{{From: "e", To: "b"}},
	)
	registerZoneGraph(g)
	registerZoneGraph(g)
}
