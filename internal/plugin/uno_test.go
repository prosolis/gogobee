package plugin

import "testing"

// ---------------------------------------------------------------------------
// Card properties
// ---------------------------------------------------------------------------

func TestUnoCard_IsWild(t *testing.T) {
	tests := []struct {
		name string
		card unoCard
		want bool
	}{
		{"wild", unoCard{unoWild, unoWildCard}, true},
		{"wild draw four", unoCard{unoWild, unoWildDrawFour}, true},
		{"wild draw six", unoCard{unoWild, unoWildDrawSix}, true},
		{"wild draw ten", unoCard{unoWild, unoWildDrawTen}, true},
		{"wild reverse draw 4", unoCard{unoWild, unoWildReverseDraw4}, true},
		{"wild color roulette", unoCard{unoWild, unoWildColorRoulette}, true},
		{"red skip", unoCard{unoRed, unoSkip}, false},
		{"blue 5", unoCard{unoBlue, unoFive}, false},
		{"green draw two", unoCard{unoGreen, unoDrawTwo}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.card.isWild(); got != tt.want {
				t.Errorf("isWild() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnoValue_IsAction(t *testing.T) {
	actions := []unoValue{unoSkip, unoReverse, unoDrawTwo, unoWildCard, unoWildDrawFour,
		unoSkipEveryone, unoDrawFour, unoDiscardAll, unoWildReverseDraw4,
		unoWildDrawSix, unoWildDrawTen, unoWildColorRoulette}
	for _, v := range actions {
		if !v.isAction() {
			t.Errorf("%s should be an action card", v)
		}
	}

	numbers := []unoValue{unoZero, unoOne, unoTwo, unoThree, unoFour, unoFive, unoSix, unoSeven, unoEight, unoNine}
	for _, v := range numbers {
		if v.isAction() {
			t.Errorf("%s should NOT be an action card", v)
		}
	}
}

// ---------------------------------------------------------------------------
// canPlayOn
// ---------------------------------------------------------------------------

func TestCanPlayOn(t *testing.T) {
	tests := []struct {
		name     string
		card     unoCard
		top      unoCard
		topColor unoColor
		want     bool
	}{
		{"wild always playable", unoCard{unoWild, unoWildCard}, unoCard{unoRed, unoFive}, unoRed, true},
		{"wd4 always playable", unoCard{unoWild, unoWildDrawFour}, unoCard{unoBlue, unoThree}, unoBlue, true},
		{"same color", unoCard{unoRed, unoThree}, unoCard{unoRed, unoSeven}, unoRed, true},
		{"same value diff color", unoCard{unoBlue, unoFive}, unoCard{unoRed, unoFive}, unoRed, true},
		{"diff color diff value", unoCard{unoBlue, unoThree}, unoCard{unoRed, unoSeven}, unoRed, false},
		{"matches chosen color not card color", unoCard{unoGreen, unoTwo}, unoCard{unoWild, unoWildCard}, unoGreen, true},
		{"doesnt match chosen color", unoCard{unoRed, unoTwo}, unoCard{unoWild, unoWildCard}, unoGreen, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.card.canPlayOn(tt.top, tt.topColor); got != tt.want {
				t.Errorf("canPlayOn() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Deck sizes
// ---------------------------------------------------------------------------

func TestNewUnoDeck_Size(t *testing.T) {
	deck := newUnoDeck()
	// Standard UNO: 4 colors × (1 zero + 2×12 other) + 8 wilds = 4×25 + 8 = 108
	if len(deck) != 108 {
		t.Errorf("standard deck size = %d, want 108", len(deck))
	}
}

func TestNewNoMercyDeck_Size(t *testing.T) {
	deck := newNoMercyDeck()
	if len(deck) < 108 {
		t.Errorf("no mercy deck should be larger than standard, got %d", len(deck))
	}
}

// ---------------------------------------------------------------------------
// Bot AI
// ---------------------------------------------------------------------------

func TestBotPickCard_NoPlayable(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoOne},
		{unoRed, unoTwo},
	}
	top := unoCard{unoBlue, unoSeven}
	_, idx := botPickCard(hand, top, unoBlue, false, 5)
	if idx != -1 {
		t.Errorf("should return -1 when nothing playable, got %d", idx)
	}
}

func TestBotPickCard_PlaysPlayable(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoOne},
		{unoBlue, unoSeven}, // matches top
	}
	top := unoCard{unoBlue, unoFive}
	card, idx := botPickCard(hand, top, unoBlue, false, 5)
	if idx == -1 {
		t.Fatal("should find a playable card")
	}
	if !card.canPlayOn(top, unoBlue) {
		t.Error("picked card should be playable on top")
	}
}

func TestBotPickCard_AggressiveUsesWD4(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoOne},
		{unoWild, unoWildDrawFour},
		{unoBlue, unoSeven},
	}
	top := unoCard{unoBlue, unoFive}
	// bookDown=true triggers aggressive mode
	card, idx := botPickCard(hand, top, unoBlue, true, 5)
	if idx == -1 {
		t.Fatal("should find a playable card")
	}
	if card.Value != unoWildDrawFour {
		t.Errorf("aggressive mode should prefer WD4, got %s", card.Value)
	}
}

func TestBotPickColor_MostCommon(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoOne},
		{unoRed, unoTwo},
		{unoRed, unoThree},
		{unoBlue, unoFive},
		{unoWild, unoWildCard}, // wilds don't count
	}
	color := botPickColor(hand)
	if color != unoRed {
		t.Errorf("should pick red (3 cards), got %s", color)
	}
}

func TestBotPickColor_EmptyHand(t *testing.T) {
	// Should default to red with no cards
	color := botPickColor([]unoCard{})
	if color != unoRed {
		t.Errorf("empty hand should default to red, got %s", color)
	}
}

// ---------------------------------------------------------------------------
// No Mercy helpers
// ---------------------------------------------------------------------------

func TestCardDrawValue(t *testing.T) {
	tests := []struct {
		value unoValue
		want  int
	}{
		{unoDrawTwo, 2},
		{unoDrawFour, 4},
		{unoWildReverseDraw4, 4},
		{unoWildDrawSix, 6},
		{unoWildDrawTen, 10},
		{unoOne, 0},
		{unoSkip, 0},
		{unoWildCard, 0},
	}
	for _, tt := range tests {
		if got := cardDrawValue(tt.value); got != tt.want {
			t.Errorf("cardDrawValue(%s) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestIsDrawCard(t *testing.T) {
	if !isDrawCard(unoDrawTwo) {
		t.Error("Draw Two should be a draw card")
	}
	if !isDrawCard(unoWildDrawTen) {
		t.Error("Wild Draw Ten should be a draw card")
	}
	if isDrawCard(unoSkip) {
		t.Error("Skip should not be a draw card")
	}
}

func TestCanPlayOnStacking(t *testing.T) {
	tests := []struct {
		name          string
		card          unoCard
		topColor      unoColor
		stackMinValue int
		want          bool
	}{
		{"wild draw six stacks on 2", unoCard{unoWild, unoWildDrawSix}, unoRed, 2, true},
		{"draw two on draw two same color", unoCard{unoRed, unoDrawTwo}, unoRed, 2, true},
		{"draw two wrong color", unoCard{unoBlue, unoDrawTwo}, unoRed, 2, false},
		{"number cant stack", unoCard{unoRed, unoFive}, unoRed, 2, false},
		{"draw two stacks on 4 same color", unoCard{unoRed, unoDrawTwo}, unoRed, 4, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.card.canPlayOnStacking(tt.topColor, tt.stackMinValue); got != tt.want {
				t.Errorf("canPlayOnStacking() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasStackableCard(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoFive},
		{unoBlue, unoDrawTwo},
		{unoGreen, unoSkip},
	}
	if hasStackableCard(hand, unoBlue, 2) != true {
		t.Error("should find blue draw two as stackable")
	}
	if hasStackableCard(hand, unoRed, 2) != false {
		t.Error("no red draw cards in hand")
	}
}

func TestParseNoMercyFlags(t *testing.T) {
	tests := []struct {
		input     string
		noMercy   bool
		sevenZero bool
		amount    string
	}{
		{"nomercy", true, false, ""},
		{"nomercy 50", true, false, "50"},
		{"nomercy7-0", true, true, ""},
		{"nomercy7-0 100", true, true, "100"},
		{"50", false, false, "50"},
		{"", false, false, ""},
		{"NOMERCY", true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			nm, sz, amt := parseNoMercyFlags(tt.input)
			if nm != tt.noMercy || sz != tt.sevenZero || amt != tt.amount {
				t.Errorf("parseNoMercyFlags(%q) = (%v, %v, %q), want (%v, %v, %q)",
					tt.input, nm, sz, amt, tt.noMercy, tt.sevenZero, tt.amount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Color/Value string representations
// ---------------------------------------------------------------------------

func TestUnoColor_String(t *testing.T) {
	if unoRed.String() != "Red" {
		t.Errorf("red: got %q", unoRed.String())
	}
	if unoWild.String() != "Wild" {
		t.Errorf("wild: got %q", unoWild.String())
	}
}

func TestUnoColor_Emoji(t *testing.T) {
	if unoRed.Emoji() != "🟥" {
		t.Errorf("red emoji: got %q", unoRed.Emoji())
	}
	if unoBlue.Emoji() != "🟦" {
		t.Errorf("blue emoji: got %q", unoBlue.Emoji())
	}
}

// ---------------------------------------------------------------------------
// Card point values (sudden death scoring)
// ---------------------------------------------------------------------------

func TestCardPointValue(t *testing.T) {
	tests := []struct {
		name string
		card unoCard
		want int
	}{
		{"zero", unoCard{unoRed, unoZero}, 0},
		{"five", unoCard{unoBlue, unoFive}, 5},
		{"nine", unoCard{unoGreen, unoNine}, 9},
		{"skip", unoCard{unoRed, unoSkip}, 20},
		{"reverse", unoCard{unoBlue, unoReverse}, 20},
		{"draw two", unoCard{unoYellow, unoDrawTwo}, 20},
		{"skip everyone", unoCard{unoRed, unoSkipEveryone}, 30},
		{"draw four colored", unoCard{unoGreen, unoDrawFour}, 30},
		{"discard all", unoCard{unoBlue, unoDiscardAll}, 30},
		{"wild", unoCard{unoWild, unoWildCard}, 50},
		{"wild draw four", unoCard{unoWild, unoWildDrawFour}, 50},
		{"wild reverse draw 4", unoCard{unoWild, unoWildReverseDraw4}, 60},
		{"wild draw six", unoCard{unoWild, unoWildDrawSix}, 60},
		{"wild color roulette", unoCard{unoWild, unoWildColorRoulette}, 60},
		{"wild draw ten", unoCard{unoWild, unoWildDrawTen}, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardPointValue(tt.card); got != tt.want {
				t.Errorf("cardPointValue(%v) = %d, want %d", tt.card, got, tt.want)
			}
		})
	}
}

func TestScoreHand(t *testing.T) {
	hand := []unoCard{
		{unoRed, unoFive},       // 5
		{unoBlue, unoNine},      // 9
		{unoGreen, unoSkip},     // 20
		{unoWild, unoWildCard},  // 50
	}
	got := scoreHand(hand)
	want := 84
	if got != want {
		t.Errorf("scoreHand() = %d, want %d", got, want)
	}
}

func TestScoreHandEmpty(t *testing.T) {
	if got := scoreHand(nil); got != 0 {
		t.Errorf("scoreHand(nil) = %d, want 0", got)
	}
}
