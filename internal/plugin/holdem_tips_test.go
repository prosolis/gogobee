package plugin

import (
	"testing"

	"github.com/chehsunliu/poker"
)

// ---------------------------------------------------------------------------
// Position label — heads-up street awareness
// ---------------------------------------------------------------------------

func TestTipPositionLabel_HeadsUp_PreFlop(t *testing.T) {
	// Pre-flop: dealer is SB (acts first), other is BB
	if pos := tipPositionLabel(true, StreetPreFlop); pos != "SB" {
		t.Errorf("dealer preflop HU: got %q, want SB", pos)
	}
	if pos := tipPositionLabel(false, StreetPreFlop); pos != "BB" {
		t.Errorf("non-dealer preflop HU: got %q, want BB", pos)
	}
}

func TestTipPositionLabel_HeadsUp_PostFlop(t *testing.T) {
	// Post-flop: dealer is BTN (acts last = positional advantage)
	for _, street := range []Street{StreetFlop, StreetTurn, StreetRiver} {
		if pos := tipPositionLabel(true, street); pos != "BTN" {
			t.Errorf("dealer %s HU: got %q, want BTN", street, pos)
		}
		if pos := tipPositionLabel(false, street); pos != "BB" {
			t.Errorf("non-dealer %s HU: got %q, want BB", street, pos)
		}
	}
}

// ---------------------------------------------------------------------------
// Draw detection
// ---------------------------------------------------------------------------

func TestComputeDraws_GutShotStraight(t *testing.T) {
	// 9♠ 8♠ on 7♣ 5♦ 2♥ — gutshot (need 6 for 5-6-7-8-9)
	hole := [2]poker.Card{poker.NewCard("9s"), poker.NewCard("8s")}
	community := []poker.Card{poker.NewCard("7c"), poker.NewCard("5d"), poker.NewCard("2h")}

	draw := computeDraws(hole, community)
	if !draw.IsDraw {
		t.Fatal("should detect a draw")
	}
	if draw.StraightDrawOuts != 4 {
		t.Errorf("straight outs: got %d, want 4 (gutshot)", draw.StraightDrawOuts)
	}
}

func TestComputeDraws_SpecScenario(t *testing.T) {
	// 8♥ 7♥ on Q♥ K♠ 10♦ — the spec scenario
	// This is a backdoor flush (3 hearts) + backdoor straight, NOT a standard gutshot.
	// A straight here needs 9+J (two cards), so it's runner-runner.
	hole := [2]poker.Card{poker.NewCard("8h"), poker.NewCard("7h")}
	community := []poker.Card{poker.NewCard("Qh"), poker.NewCard("Ks"), poker.NewCard("Td")}

	draw := computeDraws(hole, community)
	if !draw.IsDraw {
		t.Fatal("should detect backdoor draws")
	}
	if draw.TotalOuts == 0 {
		t.Error("should have some backdoor outs")
	}
}

func TestComputeDraws_FlushDraw(t *testing.T) {
	// 5♥ 6♥ on 7♥ 8♣ 2♥ — flush draw (4 hearts, need 1 more)
	hole := [2]poker.Card{poker.NewCard("5h"), poker.NewCard("6h")}
	community := []poker.Card{poker.NewCard("7h"), poker.NewCard("8c"), poker.NewCard("2h")}

	draw := computeDraws(hole, community)
	if !draw.IsDraw {
		t.Fatal("should detect a draw")
	}
	if draw.FlushDrawOuts != 9 {
		t.Errorf("flush outs: got %d, want 9", draw.FlushDrawOuts)
	}
}

func TestComputeDraws_OESDPlusFlush(t *testing.T) {
	// 5♥ 6♥ on 7♥ 8♣ 2♥ — OESD (4-5-6-7-8) + flush draw
	hole := [2]poker.Card{poker.NewCard("5h"), poker.NewCard("6h")}
	community := []poker.Card{poker.NewCard("7h"), poker.NewCard("8c"), poker.NewCard("2h")}

	draw := computeDraws(hole, community)
	if draw.TotalOuts > 15 {
		t.Errorf("total outs should be capped at 15, got %d", draw.TotalOuts)
	}
	if draw.FlushDrawOuts == 0 && draw.StraightDrawOuts == 0 {
		t.Error("should detect flush and/or straight draw")
	}
}

func TestComputeDraws_MadeHand_NoDraw(t *testing.T) {
	// Q♣ Q♦ on Q♥ 2♠ 7♣ — trips, no draw
	hole := [2]poker.Card{poker.NewCard("Qc"), poker.NewCard("Qd")}
	community := []poker.Card{poker.NewCard("Qh"), poker.NewCard("2s"), poker.NewCard("7c")}

	draw := computeDraws(hole, community)
	if draw.IsDraw && draw.TotalOuts > 2 {
		t.Errorf("set of queens should not report significant draw, got %d outs", draw.TotalOuts)
	}
}

func TestComputeDraws_PreFlop_NoDraw(t *testing.T) {
	// No community cards — draws not applicable
	hole := [2]poker.Card{poker.NewCard("As"), poker.NewCard("Ks")}
	draw := computeDraws(hole, nil)
	if draw.IsDraw {
		t.Error("preflop should not report draws")
	}
}

func TestComputeDraws_River_NoDraw(t *testing.T) {
	// 5 community cards — draws not applicable (no more cards to come)
	hole := [2]poker.Card{poker.NewCard("5h"), poker.NewCard("6h")}
	community := []poker.Card{
		poker.NewCard("7h"), poker.NewCard("8c"), poker.NewCard("2h"),
		poker.NewCard("Ks"), poker.NewCard("3d"),
	}
	draw := computeDraws(hole, community)
	if draw.IsDraw {
		t.Error("river should not report draws")
	}
}

// ---------------------------------------------------------------------------
// Straight detection helpers
// ---------------------------------------------------------------------------

func TestHasStraight(t *testing.T) {
	tests := []struct {
		name  string
		ranks uint16
		want  bool
	}{
		{"A-high straight (T-J-Q-K-A)", 0x1F00, true},   // bits 8-12
		{"low straight (A-2-3-4-5)", 0x100F, true},       // bits 0,1,2,3,12
		{"5-6-7-8-9", 0x00F8, true},                      // bits 3-7
		{"four consecutive", 0x000F, false},               // bits 0-3
		{"scattered", 0x1111, false},                      // bits 0,4,8,12
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasStraight(tt.ranks); got != tt.want {
				t.Errorf("hasStraight(0x%04X) = %v, want %v", tt.ranks, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Think tag stripping
// ---------------------------------------------------------------------------

func TestExtractTipFromResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"no think tags",
			"Check here and take the free card.",
			"Check here and take the free card.",
		},
		{
			"with think tags",
			"<think>Let me analyze this hand...</think>Check here and take the free card.",
			"Check here and take the free card.",
		},
		{
			"multiline think block",
			"<think>\nStep 1: assess hand type\nStep 2: evaluate outs\n</think>\n\nYou have a gutshot. Take the free card.",
			"You have a gutshot. Take the free card.",
		},
		{
			"only think block",
			"<think>reasoning only</think>",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTipFromResponse(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// User prompt structure
// ---------------------------------------------------------------------------

func TestBuildTipUserPrompt_IncludesDrawInfo(t *testing.T) {
	ctx := holdemTipContext{
		Hole:      [2]string{"8♥", "7♥"},
		Community: "Q♥ K♠ 10♦",
		Street:    StreetFlop,
		Position:  "BB",
		NumActive: 2,
		HeadsUp:   true,
		Equity:    EquityResult{Win: 0.29, Tie: 0.01, Loss: 0.70},
		Draw: DrawInfo{
			IsDraw:           true,
			StraightDrawOuts: 4,
			TotalOuts:        4,
			Description:      "gutshot straight draw (4 outs)",
		},
		HandCategory: "High Card",
	}

	prompt := buildTipUserPrompt(ctx)

	if !contains(prompt, "gutshot straight draw") {
		t.Error("prompt should include draw description")
	}
	if !contains(prompt, "[High Card]") {
		t.Error("prompt should include hand category")
	}
	if !contains(prompt, "Heads-up: yes") {
		t.Error("prompt should indicate heads-up")
	}
	if !contains(prompt, "Free card available") {
		t.Error("prompt should indicate free card when ToCall=0")
	}
}

func TestBuildTipUserPrompt_NoDrawWhenMadeHand(t *testing.T) {
	ctx := holdemTipContext{
		Hole:         [2]string{"Q♣", "Q♦"},
		Community:    "Q♥ 2♠ 7♣",
		Street:       StreetFlop,
		Position:     "BTN",
		NumActive:    3,
		Equity:       EquityResult{Win: 0.90, Tie: 0.02, Loss: 0.08},
		HandCategory: "Three of a Kind",
	}

	prompt := buildTipUserPrompt(ctx)

	if contains(prompt, "Draw outs:") {
		t.Error("prompt should not include draw line for made hand without draws")
	}
	if !contains(prompt, "[Three of a Kind]") {
		t.Error("prompt should include hand category")
	}
}

func TestBuildTipUserPrompt_PotOddsWhenFacingBet(t *testing.T) {
	ctx := holdemTipContext{
		Hole:       [2]string{"A♠", "K♠"},
		Community:  "2♣ 7♦ J♠",
		Street:     StreetFlop,
		Position:   "CO",
		NumActive:  4,
		Equity:     EquityResult{Win: 0.45, Tie: 0.02, Loss: 0.53},
		ToCall:     50,
		PotOddsPct: 25.0,
		HandCategory: "High Card",
	}

	prompt := buildTipUserPrompt(ctx)

	if !contains(prompt, "Pot odds to call:") {
		t.Error("prompt should include pot odds when facing a bet")
	}
	if contains(prompt, "Free card available") {
		t.Error("prompt should not say free card when facing a bet")
	}
}

// ---------------------------------------------------------------------------
// Rules-based fallback — draw awareness
// ---------------------------------------------------------------------------

func TestRulesTip_DrawWithFreeCard(t *testing.T) {
	ctx := holdemTipContext{
		Equity:   EquityResult{Win: 0.29, Tie: 0.01, Loss: 0.70},
		ToCall:   0,
		Draw:     DrawInfo{IsDraw: true, TotalOuts: 4, Description: "gutshot straight draw (4 outs)"},
		Position: "BB",
		Street:   StreetFlop,
	}

	tip := generateRulesTip(ctx)
	if !contains(tip, "free card") {
		t.Errorf("draw + free card tip should mention free card, got: %s", tip)
	}
}

func TestRulesTip_DrawFacingBet_GoodOdds(t *testing.T) {
	ctx := holdemTipContext{
		Equity:     EquityResult{Win: 0.35, Tie: 0.01, Loss: 0.64},
		ToCall:     20,
		PotOddsPct: 20.0,
		Draw:       DrawInfo{IsDraw: true, TotalOuts: 9, Description: "flush draw (9 outs)"},
		Position:   "BB",
		Street:     StreetFlop,
	}

	tip := generateRulesTip(ctx)
	if !contains(tip, "price is right") {
		t.Errorf("draw with good odds should mention price, got: %s", tip)
	}
}

// ---------------------------------------------------------------------------
// Rules-based tip — preflop classification
// ---------------------------------------------------------------------------

func TestRulesTip_Preflop_Premium_Open(t *testing.T) {
	ctx := holdemTipContext{
		Street:      StreetPreFlop,
		PreflopTier: "premium",
		Position:    "BTN",
		Equity:      EquityResult{Win: 0.82, Tie: 0.01, Loss: 0.17},
	}
	tip := generateRulesTip(ctx)
	if !contains(tip, "raise") || contains(tip, "fold") {
		t.Errorf("premium open should recommend raise, got: %s", tip)
	}
}

func TestRulesTip_Preflop_Trash_FacingRaise(t *testing.T) {
	ctx := holdemTipContext{
		Street:      StreetPreFlop,
		PreflopTier: "trash",
		Position:    "BB",
		ToCall:      100,
		Equity:      EquityResult{Win: 0.15, Tie: 0.0, Loss: 0.85},
	}
	tip := generateRulesTip(ctx)
	if !contains(tip, "fold") {
		t.Errorf("trash facing raise should fold, got: %s", tip)
	}
}

// ---------------------------------------------------------------------------
// Rules-based tip — postflop action vocabulary
// ---------------------------------------------------------------------------

func TestRulesTip_Marginal_FacingBet_UsesCallOrFold(t *testing.T) {
	// Facing a bet must never recommend "bet" or "check".
	ctx := holdemTipContext{
		Street:       StreetRiver,
		Equity:       EquityResult{Win: 0.66, Tie: 0.0, Loss: 0.34},
		ToCall:       400,
		PotOddsPct:   25.0,
		Position:     "BTN",
		HandCategory: "One Pair",
		HeadsUp:      true,
		NumActive:    2,
	}
	tip := generateRulesTip(ctx)
	if contains(tip, " bet ") || contains(tip, "check") {
		t.Errorf("facing-a-bet tip should not say 'bet' or 'check', got: %s", tip)
	}
	if !contains(tip, "call") && !contains(tip, "raise") && !contains(tip, "fold") {
		t.Errorf("facing-a-bet tip should recommend call/raise/fold, got: %s", tip)
	}
}

func TestRulesTip_Monster_BetsForValue(t *testing.T) {
	ctx := holdemTipContext{
		Street:       StreetFlop,
		Equity:       EquityResult{Win: 0.92, Tie: 0.0, Loss: 0.08},
		HandCategory: "Three of a Kind — 9s (set)",
		Position:     "BTN",
		NumActive:    2,
		SPR:          5,
	}
	tip := generateRulesTip(ctx)
	if !contains(tip, "value") && !contains(tip, "bet") {
		t.Errorf("monster hand should bet for value, got: %s", tip)
	}
}

// ---------------------------------------------------------------------------
// Preflop classification
// ---------------------------------------------------------------------------

func TestClassifyPreflopHand(t *testing.T) {
	cases := []struct {
		name   string
		hole   [2]poker.Card
		suited bool
		want   string
	}{
		{"AA", [2]poker.Card{poker.NewCard("As"), poker.NewCard("Ah")}, false, "premium"},
		{"AKs", [2]poker.Card{poker.NewCard("As"), poker.NewCard("Ks")}, true, "premium"},
		{"TT", [2]poker.Card{poker.NewCard("Ts"), poker.NewCard("Th")}, false, "strong"},
		{"22", [2]poker.Card{poker.NewCard("2s"), poker.NewCard("2h")}, false, "speculative"},
		{"72o", [2]poker.Card{poker.NewCard("7s"), poker.NewCard("2h")}, false, "trash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ranks := [2]int{cardRankIndex(c.hole[0]), cardRankIndex(c.hole[1])}
			got := classifyPreflopHand(ranks, c.suited)
			if got != c.want {
				t.Errorf("classify(%s) = %s, want %s", c.name, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LLM rewrite guard — keeps action verbs
// ---------------------------------------------------------------------------

func TestRewriteKeepsAction(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		rewrite string
		want    bool
	}{
		{"same verb", "You should call here — the price is right.", "Call this one: the price is right.", true},
		{"dropped call", "You should call here.", "Raise and apply pressure.", false},
		{"fold preserved", "Fold this marginal hand.", "This is a fold — don't call.", true},
		{"fold lost", "Fold this hand.", "Call, you're priced in.", false},
		{"bet preserved", "Bet 2/3 pot for value.", "Bet two-thirds of the pot for value.", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewriteKeepsAction(c.base, c.rewrite)
			if got != c.want {
				t.Errorf("rewriteKeepsAction(base=%q, rewrite=%q) = %v, want %v", c.base, c.rewrite, got, c.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Semantic guardrails on LLM rewrites
// ---------------------------------------------------------------------------

func TestRewriteSemanticProblem_RiverFutureStreets(t *testing.T) {
	ctx := holdemTipContext{Street: StreetRiver, ToCall: 0}
	bad := "Check and let the opponent act first—your hand has only 31 % equity, so the safest move is to stay in the pot and see if they commit; folding after a bet is the correct response."
	if r := rewriteSemanticProblem(ctx, bad); r == "" {
		t.Errorf("expected river rewrite with future-street + no-bet phrasing to be rejected")
	}
}

func TestRewriteSemanticProblem_RiverCleanRewrite(t *testing.T) {
	ctx := holdemTipContext{Street: StreetRiver, ToCall: 0}
	good := "Check — Queen high is too weak to value bet and you have showdown value against missed draws."
	if r := rewriteSemanticProblem(ctx, good); r != "" {
		t.Errorf("clean river rewrite should pass, got: %s", r)
	}
}

func TestRewriteSemanticProblem_FlopFutureStreetsOK(t *testing.T) {
	// Future-street language is fine when we're not on the river.
	ctx := holdemTipContext{Street: StreetFlop, ToCall: 0}
	ok := "Check to control the pot and see the next card cheaply."
	if r := rewriteSemanticProblem(ctx, ok); r != "" {
		t.Errorf("flop future-street language should pass, got: %s", r)
	}
}

func TestRewriteSemanticProblem_NoBetToFace(t *testing.T) {
	ctx := holdemTipContext{Street: StreetTurn, ToCall: 0}
	bad := "Check back — folding after a bet would be the right response if they fire."
	if r := rewriteSemanticProblem(ctx, bad); r == "" {
		t.Errorf("rewrite with 'folding after a bet' when ToCall=0 should be rejected")
	}
}

func TestRewriteKeepsAction_BetAsNoun(t *testing.T) {
	// Regression: base uses "bet" as a noun ("call this bet"); rewrite uses
	// "call" but not "bet". Primary action in base is "call", so accept.
	base := "Call this bet — your flush draw has the right price at 33% equity."
	rewrite := "Call — the flush draw gets correct odds here."
	if !rewriteKeepsAction(base, rewrite) {
		t.Errorf("rewrite should be accepted: base primary action is 'call', rewrite preserves it")
	}
}

func TestRewriteKeepsAction_ChangesPrimaryAction(t *testing.T) {
	base := "Call — your flush draw has the right price."
	rewrite := "Fold — this price is too steep."
	if rewriteKeepsAction(base, rewrite) {
		t.Errorf("rewrite should be rejected: primary action changed from call to fold")
	}
}

func TestRewriteKeepsAction_AddsRaiseOnTopOfCall(t *testing.T) {
	// The A♠5♠ SB 3bet spot: base recommends call, LLM injects a raise on
	// top. Primary action (call) is preserved but rewrite mentions raise,
	// which was not in the base.
	base := "Speculative hand with deep stacks — call for set-mining or flopping a big draw. Fold the flop if you miss."
	rewrite := "Call — your equity is fine, but raise 3-4x the pot to apply maximum pressure."
	if rewriteKeepsAction(base, rewrite) {
		t.Errorf("rewrite should be rejected: injects raise on top of call/fold base")
	}
}

// ---------------------------------------------------------------------------
// Facing all-in — terminal decision branch
// ---------------------------------------------------------------------------

func TestAllInTip_NoFutureStreetLanguage(t *testing.T) {
	hole := [2]poker.Card{poker.NewCard("Ac"), poker.NewCard("Kh")}
	comm := []poker.Card{poker.NewCard("4c"), poker.NewCard("9c"), poker.NewCard("4h")}
	snap := tipSnapshot{
		hole: hole, holeStr: [2]string{"A♣", "K♥"},
		community: comm, communityS: renderCards(comm),
		numActive: 2, numOpp: 1,
		toCall: 16600, totalPot: 21200, stack: 18800,
		street: StreetFlop, position: "BTN", headsUp: true, isDealer: true,
		isAllIn: true,
	}
	ctx := buildTipContext(snap)
	tip := generateRulesTip(ctx)

	if !containsCI(tip, "facing an all-in") && !containsCI(tip, "terminal decision") {
		t.Errorf("all-in tip should mark the decision as terminal, got: %s", tip)
	}
	forbidden := []string{"next card", "later street", "position advantage", "see the turn", "fold later", "act after", "future street"}
	for _, p := range forbidden {
		if containsCI(tip, p) {
			t.Errorf("all-in tip must not mention %q, got: %s", p, tip)
		}
	}
}

func TestAllInTip_UsesVsShoveEquity_NotVsRandom(t *testing.T) {
	hole := [2]poker.Card{poker.NewCard("Qs"), poker.NewCard("Js")}
	comm := []poker.Card{poker.NewCard("Kh"), poker.NewCard("Th"), poker.NewCard("4d")}
	snap := tipSnapshot{
		hole: hole, holeStr: [2]string{"Q♠", "J♠"},
		community: comm, communityS: renderCards(comm),
		numActive: 2, numOpp: 1,
		toCall: 800, totalPot: 1000, stack: 3000,
		street: StreetFlop, position: "BTN", headsUp: true, isDealer: true,
		isAllIn: true,
	}
	ctx := buildTipContext(snap)
	vsRand := ctx.Equity.Win + ctx.Equity.Tie*0.5
	vsShove := ctx.EquityVsShove.Win + ctx.EquityVsShove.Tie*0.5
	if vsShove >= vsRand {
		t.Errorf("OESD should lose equity vs a tight shove range; vsRand=%.2f vsShove=%.2f", vsRand, vsShove)
	}
	tip := generateRulesTip(ctx)
	if !containsCI(tip, "fold") {
		t.Errorf("OESD vs flop all-in with 44%% pot odds should recommend fold, got: %s", tip)
	}
}

func TestRewriteSemanticProblem_AllInFutureStreets(t *testing.T) {
	ctx := holdemTipContext{Street: StreetFlop, ToCall: 16600, IsAllIn: true}
	bad := "Call the bet and see the turn. You have positional advantage giving you the ability to fold later if the board turns ugly."
	if r := rewriteSemanticProblem(ctx, bad); r == "" {
		t.Errorf("all-in rewrite with future-street / position language should be rejected")
	}
}

func TestRewriteSemanticProblem_AllInCleanRewrite(t *testing.T) {
	ctx := holdemTipContext{Street: StreetFlop, ToCall: 16600, IsAllIn: true}
	ok := "Facing an all-in — call. Your equity vs a realistic shove range is well above the pot odds."
	if r := rewriteSemanticProblem(ctx, ok); r != "" {
		t.Errorf("clean all-in rewrite should pass, got: %s", r)
	}
}

func TestEquityVsRange_CompatCombosFiltersConflicts(t *testing.T) {
	hole := [2]poker.Card{poker.NewCard("As"), poker.NewCard("Ah")}
	community := []poker.Card{poker.NewCard("Ad"), poker.NewCard("Th"), poker.NewCard("5c")}
	r := expandRange([]string{"AA"}) // 6 combos of AA
	known := map[poker.Card]bool{}
	for _, c := range [...]poker.Card{hole[0], hole[1]} {
		known[c] = true
	}
	for _, c := range community {
		known[c] = true
	}
	compat := compatCombos(r, known)
	// Hero and board block 3 aces (As, Ah, Ad); only Ac remains — 0 combos
	// possible since villain needs 2 aces.
	if len(compat) != 0 {
		t.Errorf("villain AA should be fully blocked by hero AA + Ad on board, got %d combos", len(compat))
	}
}

func TestExpandHandClass_Counts(t *testing.T) {
	if n := len(expandHandClass("AA")); n != 6 {
		t.Errorf("AA expands to 6 combos, got %d", n)
	}
	if n := len(expandHandClass("AKs")); n != 4 {
		t.Errorf("AKs expands to 4 combos, got %d", n)
	}
	if n := len(expandHandClass("AKo")); n != 12 {
		t.Errorf("AKo expands to 12 combos, got %d", n)
	}
	if n := len(expandHandClass("garbage")); n != 0 {
		t.Errorf("garbage class should return nil/0, got %d", n)
	}
}

// containsCI is a case-insensitive contains used by the all-in tests.
func containsCI(s, sub string) bool {
	return contains(toLower(s), toLower(sub))
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func TestRewriteKeepsAction_FoldFamilyPresent(t *testing.T) {
	// "fold the flop if you miss" in the base legitimises mentioning fold in
	// the rewrite.
	base := "Speculative hand with deep stacks — call for set-mining. Fold the flop if you miss."
	rewrite := "Call and plan to fold the flop unconnected."
	if !rewriteKeepsAction(base, rewrite) {
		t.Errorf("rewrite should pass: both call and fold are in base")
	}
}
