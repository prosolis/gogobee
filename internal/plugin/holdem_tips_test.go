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
