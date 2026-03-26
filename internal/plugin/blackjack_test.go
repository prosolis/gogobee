package plugin

import "testing"

func TestHandValue_SimpleHand(t *testing.T) {
	cards := []card{{Rank: 10, Suit: spades}, {Rank: 7, Suit: hearts}}
	val, soft := handValue(cards)
	if val != 17 || soft {
		t.Errorf("got %d soft=%v, want 17 soft=false", val, soft)
	}
}

func TestHandValue_AceAsSoft(t *testing.T) {
	cards := []card{{Rank: 1, Suit: spades}, {Rank: 6, Suit: hearts}}
	val, soft := handValue(cards)
	if val != 17 || !soft {
		t.Errorf("got %d soft=%v, want 17 soft=true", val, soft)
	}
}

func TestHandValue_AceDowngradesToOne(t *testing.T) {
	cards := []card{
		{Rank: 1, Suit: spades},
		{Rank: 9, Suit: hearts},
		{Rank: 5, Suit: diamonds},
	}
	val, soft := handValue(cards)
	// A(11)+9+5 = 25, bust, so A=1: 1+9+5 = 15
	if val != 15 || soft {
		t.Errorf("got %d soft=%v, want 15 soft=false", val, soft)
	}
}

func TestHandValue_TwoAces(t *testing.T) {
	cards := []card{
		{Rank: 1, Suit: spades},
		{Rank: 1, Suit: hearts},
	}
	val, soft := handValue(cards)
	// A(11)+A(11) = 22, one A->1: 12, still soft
	if val != 12 || !soft {
		t.Errorf("got %d soft=%v, want 12 soft=true", val, soft)
	}
}

func TestHandValue_FaceCards(t *testing.T) {
	cards := []card{{Rank: 11, Suit: spades}, {Rank: 12, Suit: hearts}} // J + Q
	val, soft := handValue(cards)
	if val != 20 || soft {
		t.Errorf("got %d soft=%v, want 20 soft=false", val, soft)
	}
}

func TestHandValue_Bust(t *testing.T) {
	cards := []card{
		{Rank: 10, Suit: spades},
		{Rank: 8, Suit: hearts},
		{Rank: 5, Suit: diamonds},
	}
	val, _ := handValue(cards)
	if val != 23 {
		t.Errorf("got %d, want 23 (bust)", val)
	}
}

func TestIsBlackjack(t *testing.T) {
	tests := []struct {
		name  string
		cards []card
		want  bool
	}{
		{"ace+king", []card{{1, spades}, {13, hearts}}, true},
		{"ace+ten", []card{{1, spades}, {10, hearts}}, true},
		{"ace+queen", []card{{1, spades}, {12, hearts}}, true},
		{"ten+jack", []card{{10, spades}, {11, hearts}}, false}, // 20, not 21
		{"three cards summing 21", []card{{7, spades}, {7, hearts}, {7, diamonds}}, false},
		{"ace+five", []card{{1, spades}, {5, hearts}}, false}, // 16
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBlackjack(tt.cards)
			if got != tt.want {
				t.Errorf("isBlackjack(%v) = %v, want %v", tt.cards, got, tt.want)
			}
		})
	}
}
