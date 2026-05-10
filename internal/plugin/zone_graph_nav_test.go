package plugin

import (
	"strings"
	"testing"
)

func TestEvaluateEdgeLock_None(t *testing.T) {
	e := ZoneEdge{From: "a", To: "b", Lock: LockNone}
	ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{})
	if !ok {
		t.Fatal("LockNone should always unlock")
	}
}

func TestEvaluateEdgeLock_Perception(t *testing.T) {
	e := ZoneEdge{
		From:     "a",
		To:       "b",
		Lock:     LockPerception,
		LockData: map[string]any{"dc": 1}, // trivially passes
	}
	ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{
		RunID: "run", FromNode: "a",
		AbilityMods: [6]int{0, 0, 0, 0, 5, 0},
	})
	if !ok {
		t.Fatal("Perception roll+5 vs DC 1 should unlock")
	}

	// DC 99 + bad WIS → never unlock
	e.LockData = map[string]any{"dc": 99}
	ok, reason := evaluateEdgeLock(e, edgeUnlockCtx{
		RunID: "run", FromNode: "a",
		AbilityMods: [6]int{0, 0, 0, 0, -2, 0},
	})
	if ok {
		t.Fatal("Perception vs DC 99 should fail")
	}
	if !strings.Contains(reason, "Perception") {
		t.Errorf("reason = %q, want Perception phrasing", reason)
	}
}

func TestPerceptionRollForEdge_Deterministic(t *testing.T) {
	r1 := perceptionRollForEdge("run-x", "a", "b")
	r2 := perceptionRollForEdge("run-x", "a", "b")
	if r1 != r2 {
		t.Fatalf("perception roll non-deterministic: %d vs %d", r1, r2)
	}
	if r1 < 1 || r1 > 20 {
		t.Errorf("roll %d out of 1..20", r1)
	}
	r3 := perceptionRollForEdge("run-y", "a", "b")
	r4 := perceptionRollForEdge("run-x", "a", "c")
	// Just sanity-check the seed actually mixes inputs.
	if r1 == r3 && r1 == r4 {
		t.Error("perception roll insensitive to seed inputs")
	}
}

func TestEvaluateEdgeLock_Key(t *testing.T) {
	e := ZoneEdge{
		From:     "a",
		To:       "b",
		Lock:     LockKey,
		LockData: map[string]any{"key_id": "Brass Key"},
	}
	ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{
		InventoryNames: map[string]bool{"brass key": true},
	})
	if !ok {
		t.Fatal("matching key should unlock")
	}
	ok, reason := evaluateEdgeLock(e, edgeUnlockCtx{
		InventoryNames: map[string]bool{},
	})
	if ok {
		t.Fatal("missing key should not unlock")
	}
	if !strings.Contains(reason, "key") {
		t.Errorf("reason = %q", reason)
	}
}

func TestEvaluateEdgeLock_LevelMin(t *testing.T) {
	e := ZoneEdge{From: "a", To: "b", Lock: LockLevelMin, LockData: map[string]any{"min_level": 5}}
	if ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{CharLevel: 5}); !ok {
		t.Fatal("level 5 should pass min 5")
	}
	if ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{CharLevel: 4}); ok {
		t.Fatal("level 4 should fail min 5")
	}
}

func TestEvaluateEdgeLock_StatCheck(t *testing.T) {
	e := ZoneEdge{
		From: "a", To: "b", Lock: LockStatCheck,
		LockData: map[string]any{"stat": "STR", "dc": 1},
	}
	ok, _ := evaluateEdgeLock(e, edgeUnlockCtx{
		RunID: "x", FromNode: "a",
		AbilityMods: [6]int{5, 0, 0, 0, 0, 0},
	})
	if !ok {
		t.Fatal("STR+5 vs DC 1 should unlock")
	}
}

func TestPendingFork_Roundtrip(t *testing.T) {
	pf := pendingFork{
		PendingAt: "fork",
		Options: []pendingChoice{
			{Index: 1, To: "left", Label: "Left", Unlocked: true},
			{Index: 2, To: "right", Label: "Right", Unlocked: false, Hint: "you sense a draft", Reason: "Perception 4 vs DC 12"},
		},
	}
	m, err := encodePendingFork(pf)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodePendingFork(m)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got == nil || got.PendingAt != "fork" || len(got.Options) != 2 {
		t.Fatalf("decoded pendingFork mismatch: %+v", got)
	}
	if got.Options[0].To != "left" || got.Options[1].Hint != "you sense a draft" {
		t.Errorf("decoded options bad: %+v", got.Options)
	}
}

func TestDecodePendingFork_Empty(t *testing.T) {
	if pf, _ := decodePendingFork(nil); pf != nil {
		t.Error("nil map should decode to nil fork")
	}
	if pf, _ := decodePendingFork(map[string]any{}); pf != nil {
		t.Error("empty map should decode to nil fork")
	}
	// Non-fork content (some future schema in the same column) is also nil.
	if pf, _ := decodePendingFork(map[string]any{"unrelated": 1}); pf != nil {
		t.Error("non-fork content should decode to nil fork")
	}
}

func TestResolveForkChoice(t *testing.T) {
	pf := &pendingFork{
		PendingAt: "fork",
		Options: []pendingChoice{
			{Index: 1, To: "left", Unlocked: true},
			{Index: 2, To: "right", Unlocked: false, Reason: "DC 12"},
		},
	}
	if c, err := resolveForkChoice(pf, 1); err != nil || c.To != "left" {
		t.Errorf("choice 1 = %+v err=%v", c, err)
	}
	if _, err := resolveForkChoice(pf, 2); err == nil {
		t.Error("locked choice should error")
	}
	if _, err := resolveForkChoice(pf, 3); err == nil {
		t.Error("out-of-range should error")
	}
	if _, err := resolveForkChoice(nil, 1); err == nil {
		t.Error("nil fork should error")
	}
}

func TestRenderForkPrompt_LockedHint(t *testing.T) {
	zone := ZoneDefinition{ID: "z", Display: "Test"}
	pf := pendingFork{
		Options: []pendingChoice{
			{Index: 1, Label: "Left", Unlocked: true},
			{Index: 2, Label: "Right", Unlocked: false, Hint: "you sense a draft"},
			{Index: 3, Label: "Hidden", Unlocked: false},
		},
	}
	got := renderForkPrompt(zone, pf)
	if !strings.Contains(got, "**1.** Left") {
		t.Errorf("unlocked option not rendered: %q", got)
	}
	if !strings.Contains(got, "you sense a draft") {
		t.Errorf("hint not rendered: %q", got)
	}
	if !strings.Contains(got, "**3.** Hidden _(locked)_") {
		t.Errorf("plain locked option not rendered: %q", got)
	}
}

func TestEvaluateForkEdges_OrdersByGraph(t *testing.T) {
	g := BuildGraph("z",
		[]ZoneNode{
			{NodeID: "e", IsEntry: true, Kind: NodeKindEntry, Label: "Entry"},
			{NodeID: "fork", Kind: NodeKindFork, Label: "Fork"},
			{NodeID: "left", Kind: NodeKindExploration, Label: "Left Hall"},
			{NodeID: "right", Kind: NodeKindExploration, Label: "Right Hall"},
			{NodeID: "b", IsBoss: true, Kind: NodeKindBoss, Label: "Boss"},
		},
		[]ZoneEdge{
			{From: "e", To: "fork"},
			{From: "fork", To: "right", Weight: 2,
				Lock: LockLevelMin, LockData: map[string]any{"min_level": 99}},
			{From: "fork", To: "left", Weight: 1},
			{From: "left", To: "b"},
			{From: "right", To: "b"},
		},
	)
	ctx := edgeUnlockCtx{RunID: "run-1", FromNode: "fork", CharLevel: 5}
	choices := evaluateForkEdges(g, "fork", ctx)
	if len(choices) != 2 {
		t.Fatalf("want 2 choices, got %d", len(choices))
	}
	// Lowest weight first → left.
	if choices[0].To != "left" || !choices[0].Unlocked {
		t.Errorf("choice 0 = %+v", choices[0])
	}
	if choices[1].To != "right" || choices[1].Unlocked {
		t.Errorf("choice 1 = %+v", choices[1])
	}
	if choices[0].Label != "Left Hall" {
		t.Errorf("label not propagated: %q", choices[0].Label)
	}
}

func TestNodeKindToRoomType(t *testing.T) {
	cases := map[ZoneNodeKind]RoomType{
		NodeKindEntry:       RoomEntry,
		NodeKindExploration: RoomExploration,
		NodeKindSecret:      RoomExploration,
		NodeKindHarvest:     RoomExploration,
		NodeKindFork:        RoomExploration,
		NodeKindMerge:       RoomExploration,
		NodeKindTrap:        RoomTrap,
		NodeKindElite:       RoomElite,
		NodeKindBoss:        RoomBoss,
	}
	for k, want := range cases {
		if got := nodeKindToRoomType(k); got != want {
			t.Errorf("%s → %s, want %s", k, got, want)
		}
	}
}

