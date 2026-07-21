package plugin

import "testing"

// TestPickableLock pins which locks thieves' tools may answer. Tools substitute
// for a failed die roll, never for progression: a key is a quest token, a level
// gate is progression, a region-clear gate is structure.
func TestPickableLock(t *testing.T) {
	pickable := []ZoneEdgeLockKind{LockPerception, LockStatCheck}
	for _, k := range pickable {
		if !pickableLock(string(k)) {
			t.Errorf("%s should be pickable", k)
		}
	}
	sealed := []ZoneEdgeLockKind{LockKey, LockLevelMin, LockRegionClear, LockNone, ""}
	for _, k := range sealed {
		if pickableLock(string(k)) {
			t.Errorf("%s must not be pickable", k)
		}
	}
}

// TestBestAbilityTakesPartyMax is the core of the party-check fix: a door reads
// the best eyes present, not the leader's.
func TestBestAbilityTakesPartyMax(t *testing.T) {
	ctx := edgeUnlockCtx{AbilityMods: [6]int{0, 1, 0, 0, 1, 0}}
	ctx.bestAbility([6]int{0, 5, 0, 0, -1, 0}, "Josie")
	ctx.bestAbility([6]int{0, 2, 0, 0, 6, 0}, "Pete")

	if ctx.AbilityMods[1] != 5 || ctx.AbilityWho[1] != "Josie" {
		t.Errorf("DEX = %d by %q, want 5 by Josie", ctx.AbilityMods[1], ctx.AbilityWho[1])
	}
	if ctx.AbilityMods[4] != 6 || ctx.AbilityWho[4] != "Pete" {
		t.Errorf("WIS = %d by %q, want 6 by Pete", ctx.AbilityMods[4], ctx.AbilityWho[4])
	}
	// Nobody beat the leader's STR, so nobody is credited for it.
	if ctx.AbilityWho[0] != "" {
		t.Errorf("STR credited to %q, want nobody", ctx.AbilityWho[0])
	}
}

// TestEvaluateEdgeLockUsesPartyBest — a check the leader fails and a party-mate
// passes must open, and must say who opened it.
func TestEvaluateEdgeLockUsesPartyBest(t *testing.T) {
	e := ZoneEdge{To: "z.secret", Lock: LockPerception, LockData: map[string]any{"dc": 30}}
	ctx := edgeUnlockCtx{RunID: "run1", FromNode: "z.fork"}

	if ok, _ := evaluateEdgeLock(e, ctx); ok {
		t.Fatal("DC 30 should be unreachable with no modifiers")
	}

	// perceptionRollForEdge is seeded, so a mod big enough to clear DC 30 from
	// any roll makes this deterministic without pinning the roll itself.
	ctx.bestAbility([6]int{0, 0, 0, 0, 29, 0}, "Pete")
	ok, reason := evaluateEdgeLock(e, ctx)
	if !ok {
		t.Fatalf("party best should open the door, got %q", reason)
	}
	if reason != "Pete got it open" {
		t.Errorf("reason = %q, want credit to Pete", reason)
	}
}

// TestRankForkOptionsPrefersUnvisitedThenWeight guards the autopilot's route
// choice. Menu order is edge-authoring order and means nothing to a player.
func TestRankForkOptionsPrefersUnvisitedThenWeight(t *testing.T) {
	g := ZoneGraph{
		Nodes: map[string]ZoneNode{},
		Edges: map[string][]ZoneEdge{
			"z.fork": {
				{From: "z.fork", To: "z.seen", Weight: 9},
				{From: "z.fork", To: "z.thin", Weight: 1},
				{From: "z.fork", To: "z.fat", Weight: 5},
			},
		},
	}
	run := &DungeonRun{CurrentNode: "z.fork", VisitedNodes: []string{"z.fork", "z.seen"}}
	pf := &pendingFork{Options: []pendingChoice{
		{Index: 1, To: "z.seen", Label: "Seen", Unlocked: true},
		{Index: 2, To: "z.thin", Label: "Thin", Unlocked: true},
		{Index: 3, To: "z.fat", Label: "Fat", Unlocked: true},
		{Index: 4, To: "z.shut", Label: "Shut", Unlocked: false},
	}}

	got := rankForkOptions(g, run, pf)
	want := []string{"z.fat", "z.thin", "z.seen"}
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d (locked routes must be dropped)", len(got), len(want))
	}
	for i, w := range want {
		if got[i].To != w {
			t.Errorf("rank[%d] = %s, want %s", i, got[i].To, w)
		}
	}
}

// TestRankForkOptionsAllLocked — nothing takeable means nothing returned, which
// is what sends the autopilot to the tools/backtrack path instead of the reaper.
func TestRankForkOptionsAllLocked(t *testing.T) {
	g := ZoneGraph{Nodes: map[string]ZoneNode{}, Edges: map[string][]ZoneEdge{}}
	run := &DungeonRun{CurrentNode: "z.fork", VisitedNodes: []string{"z.fork"}}
	pf := &pendingFork{Options: []pendingChoice{
		{Index: 1, To: "z.a", Unlocked: false},
		{Index: 2, To: "z.b", Unlocked: false},
	}}
	if got := rankForkOptions(g, run, pf); len(got) != 0 {
		t.Errorf("got %d takeable options, want 0", len(got))
	}
}
