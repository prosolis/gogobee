package plugin

import (
	"math/rand/v2"

	"github.com/chehsunliu/poker"
)

// EquityResult holds Monte Carlo simulation results.
type EquityResult struct {
	Win  float64
	Tie  float64
	Loss float64
}

// allCards returns a fresh 52-card slice.
func allCards() []poker.Card {
	suits := []string{"s", "h", "d", "c"}
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}
	cards := make([]poker.Card, 0, 52)
	for _, r := range ranks {
		for _, s := range suits {
			cards = append(cards, poker.NewCard(r+s))
		}
	}
	return cards
}

// Equity computes win/tie/loss fractions via Monte Carlo simulation.
func Equity(hole [2]poker.Card, community []poker.Card, numOpponents, iterations int) EquityResult {
	if numOpponents < 1 {
		numOpponents = 1
	}

	// Build set of known cards to exclude.
	known := make(map[poker.Card]bool, 2+len(community))
	known[hole[0]] = true
	known[hole[1]] = true
	for _, c := range community {
		known[c] = true
	}

	// Remaining deck.
	remaining := make([]poker.Card, 0, 52-len(known))
	for _, c := range allCards() {
		if !known[c] {
			remaining = append(remaining, c)
		}
	}

	boardNeeded := 5 - len(community)
	cardsNeeded := numOpponents*2 + boardNeeded

	var wins, ties, losses int

	for i := 0; i < iterations; i++ {
		// Fisher-Yates shuffle of first cardsNeeded elements.
		for j := 0; j < cardsNeeded && j < len(remaining); j++ {
			k := j + rand.IntN(len(remaining)-j)
			remaining[j], remaining[k] = remaining[k], remaining[j]
		}

		// Deal opponent holes.
		idx := 0
		opponentHoles := make([][2]poker.Card, numOpponents)
		for o := 0; o < numOpponents; o++ {
			opponentHoles[o] = [2]poker.Card{remaining[idx], remaining[idx+1]}
			idx += 2
		}

		// Complete board.
		fullBoard := make([]poker.Card, 5)
		copy(fullBoard, community)
		for b := len(community); b < 5; b++ {
			fullBoard[b] = remaining[idx]
			idx++
		}

		// Evaluate hero.
		heroCards := make([]poker.Card, 7)
		heroCards[0] = hole[0]
		heroCards[1] = hole[1]
		copy(heroCards[2:], fullBoard)
		heroRank := poker.Evaluate(heroCards)

		// Evaluate opponents.
		bestOpp := int32(7463) // worst possible rank
		for _, oh := range opponentHoles {
			oppCards := make([]poker.Card, 7)
			oppCards[0] = oh[0]
			oppCards[1] = oh[1]
			copy(oppCards[2:], fullBoard)
			oppRank := poker.Evaluate(oppCards)
			if oppRank < bestOpp {
				bestOpp = oppRank
			}
		}

		if heroRank < bestOpp {
			wins++
		} else if heroRank == bestOpp {
			ties++
		} else {
			losses++
		}
	}

	total := float64(iterations)
	return EquityResult{
		Win:  float64(wins) / total,
		Tie:  float64(ties) / total,
		Loss: float64(losses) / total,
	}
}
