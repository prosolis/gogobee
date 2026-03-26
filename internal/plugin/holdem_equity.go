package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"

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

// DrawInfo holds computed draw information for tip generation.
type DrawInfo struct {
	IsDraw           bool
	FlushDrawOuts    int
	StraightDrawOuts int
	TotalOuts        int
	Description      string // e.g. "flush draw + gutshot (13 outs)"
}

// computeDraws analyzes hole cards and community for flush and straight draws.
// Only meaningful on flop and turn (not preflop, not river).
func computeDraws(hole [2]poker.Card, community []poker.Card) DrawInfo {
	if len(community) < 3 || len(community) > 4 {
		return DrawInfo{}
	}

	all := make([]poker.Card, 0, 7)
	all = append(all, hole[0], hole[1])
	all = append(all, community...)

	flushOuts := countFlushOuts(hole, community)
	straightOuts := countStraightOuts(all, hole, community)

	total := flushOuts + straightOuts
	if total > 15 {
		total = 15 // cap to avoid double-counting
	}

	if total == 0 {
		// Check backdoor draws (only on flop)
		if len(community) == 3 {
			return computeBackdoorDraws(hole, community)
		}
		return DrawInfo{}
	}

	var parts []string
	if flushOuts >= 8 {
		parts = append(parts, "flush draw")
	}
	if straightOuts == 8 {
		parts = append(parts, "open-ended straight draw")
	} else if straightOuts == 4 {
		parts = append(parts, "gutshot straight draw")
	}

	desc := fmt.Sprintf("%s (%d outs)", strings.Join(parts, " + "), total)

	return DrawInfo{
		IsDraw:           true,
		FlushDrawOuts:    flushOuts,
		StraightDrawOuts: straightOuts,
		TotalOuts:        total,
		Description:      desc,
	}
}

// countFlushOuts returns 9 if we have a flush draw (4 to a flush), 0 otherwise.
func countFlushOuts(hole [2]poker.Card, community []poker.Card) int {
	suitCounts := map[int32]int{}
	holeSuits := map[int32]bool{}

	for _, c := range []poker.Card{hole[0], hole[1]} {
		s := c.Suit()
		suitCounts[s]++
		holeSuits[s] = true
	}
	for _, c := range community {
		suitCounts[c.Suit()]++
	}

	for s, count := range suitCounts {
		if count == 4 && holeSuits[s] {
			return 9 // 13 cards of suit minus 4 seen = 9 outs
		}
	}
	return 0
}

// countStraightOuts returns the number of straight outs (8 for OESD, 4 for gutshot).
func countStraightOuts(allCards []poker.Card, hole [2]poker.Card, community []poker.Card) int {
	// Get unique ranks present (0-12 where 0=2, 12=A)
	rankSet := uint16(0)
	for _, c := range allCards {
		rankSet |= 1 << uint(c.Rank())
	}

	// Already have a straight? (5+ consecutive bits)
	if hasStraight(rankSet) {
		return 0
	}

	// Try adding each rank not already present; if it completes a straight, it's an out.
	// But only count if at least one hole card is part of the straight.
	outs := 0
	for r := int32(0); r < 13; r++ {
		if rankSet&(1<<uint(r)) != 0 {
			continue // already have this rank
		}
		test := rankSet | (1 << uint(r))
		if hasStraight(test) {
			// Verify at least one hole card participates in the completed straight.
			if holeParticipatesInStraight(test, hole) {
				// Count available cards of this rank (4 minus those on board)
				available := 4
				for _, c := range community {
					if c.Rank() == r {
						available--
					}
				}
				outs += available
			}
		}
	}

	// Normalize: OESD = 8, gutshot = 4, double gutshot = 8
	if outs > 8 {
		outs = 8
	}
	return outs
}

// hasStraight checks if a rank bitset contains 5+ consecutive ranks.
// Handles A-low straight (A-2-3-4-5) by duplicating ace as rank -1.
func hasStraight(ranks uint16) bool {
	// Check A-low straight: A(12), 2(0), 3(1), 4(2), 5(3)
	if ranks&0x100F == 0x100F { // bits 0,1,2,3,12
		return true
	}

	consecutive := 0
	for i := uint(0); i < 13; i++ {
		if ranks&(1<<i) != 0 {
			consecutive++
			if consecutive >= 5 {
				return true
			}
		} else {
			consecutive = 0
		}
	}
	return false
}

// holeParticipatesInStraight checks if at least one hole card rank is part of
// any 5-consecutive-rank window in the given rank set.
func holeParticipatesInStraight(ranks uint16, hole [2]poker.Card) bool {
	hr0 := uint(hole[0].Rank())
	hr1 := uint(hole[1].Rank())

	// Check each possible 5-card window
	for start := uint(0); start <= 8; start++ {
		window := uint16(0x1F) << start // 5 consecutive bits
		if ranks&window == window {
			if hr0 >= start && hr0 < start+5 {
				return true
			}
			if hr1 >= start && hr1 < start+5 {
				return true
			}
		}
	}

	// Check A-low straight (A=12, 2=0, 3=1, 4=2, 5=3)
	if ranks&0x100F == 0x100F && ranks&0x6 == 0x6 { // A,2,3,4,5
		if hr0 == 12 || hr0 <= 3 || hr1 == 12 || hr1 <= 3 {
			return true
		}
	}

	return false
}

// computeBackdoorDraws detects backdoor flush/straight draws (flop only).
func computeBackdoorDraws(hole [2]poker.Card, community []poker.Card) DrawInfo {
	var parts []string
	totalOuts := 0

	// Backdoor flush: 3 to a flush with at least one hole card
	suitCounts := map[int32]int{}
	holeSuits := map[int32]bool{}
	for _, c := range []poker.Card{hole[0], hole[1]} {
		s := c.Suit()
		suitCounts[s]++
		holeSuits[s] = true
	}
	for _, c := range community {
		suitCounts[c.Suit()]++
	}
	for s, count := range suitCounts {
		if count == 3 && holeSuits[s] {
			parts = append(parts, "backdoor flush")
			totalOuts += 1
			break
		}
	}

	// Backdoor straight: 3 to a straight with connected hole cards
	// Simplified: if hole cards are within 4 ranks of each other, count it
	r0 := hole[0].Rank()
	r1 := hole[1].Rank()
	gap := r0 - r1
	if gap < 0 {
		gap = -gap
	}
	if gap >= 1 && gap <= 4 {
		parts = append(parts, "backdoor straight")
		totalOuts += 1
	}

	if len(parts) == 0 {
		return DrawInfo{}
	}

	return DrawInfo{
		IsDraw:      true,
		TotalOuts:   totalOuts,
		Description: fmt.Sprintf("%s (%d outs)", strings.Join(parts, " + "), totalOuts),
	}
}
