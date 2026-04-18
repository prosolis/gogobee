package plugin

import (
	"math/rand/v2"

	"github.com/chehsunliu/poker"
)

// HandRange is a flat list of concrete 2-card combos. Expand hand classes
// like "AA", "AKs", "AKo" via expandRange, then optionally drop combos that
// conflict with the hero hole cards and the board.
type HandRange [][2]poker.Card

// expandHandClass converts a canonical hand class into concrete combos.
// "AA"  → 6 combos, "AKs" → 4 combos, "AKo" → 12 combos.
// Returns nil for malformed input.
func expandHandClass(class string) HandRange {
	if len(class) < 2 || len(class) > 3 {
		return nil
	}
	r1 := string(class[0])
	r2 := string(class[1])
	isPair := class[0] == class[1]
	mode := byte(0)
	if len(class) == 3 {
		mode = class[2]
	}
	suits := []string{"s", "h", "d", "c"}
	var out HandRange
	switch {
	case isPair:
		for i := 0; i < 4; i++ {
			for j := i + 1; j < 4; j++ {
				out = append(out, [2]poker.Card{
					poker.NewCard(r1 + suits[i]),
					poker.NewCard(r2 + suits[j]),
				})
			}
		}
	case mode == 's':
		for _, s := range suits {
			out = append(out, [2]poker.Card{
				poker.NewCard(r1 + s),
				poker.NewCard(r2 + s),
			})
		}
	case mode == 'o':
		for _, s1 := range suits {
			for _, s2 := range suits {
				if s1 == s2 {
					continue
				}
				out = append(out, [2]poker.Card{
					poker.NewCard(r1 + s1),
					poker.NewCard(r2 + s2),
				})
			}
		}
	}
	return out
}

// expandRange concatenates the expansions of each class in the input list.
func expandRange(classes []string) HandRange {
	var out HandRange
	for _, c := range classes {
		out = append(out, expandHandClass(c)...)
	}
	return out
}

// compatCombos returns combos from r that don't conflict with any card in known.
func compatCombos(r HandRange, known map[poker.Card]bool) HandRange {
	out := make(HandRange, 0, len(r))
	for _, combo := range r {
		if known[combo[0]] || known[combo[1]] || combo[0] == combo[1] {
			continue
		}
		out = append(out, combo)
	}
	return out
}

// EquityVsRange computes hero's equity when villain's hand is drawn uniformly
// from the given range (typically an all-in stackoff range, not the full
// deck). This is the right number for facing-all-in spots: "vs random"
// systematically overstates high-card hands because nobody shoves random.
func EquityVsRange(hole [2]poker.Card, community []poker.Card, villainRange HandRange, iterations int) EquityResult {
	known := make(map[poker.Card]bool, 2+len(community))
	known[hole[0]] = true
	known[hole[1]] = true
	for _, c := range community {
		known[c] = true
	}
	compat := compatCombos(villainRange, known)
	if len(compat) == 0 {
		return EquityResult{}
	}

	baseDeck := make([]poker.Card, 0, 52)
	for _, c := range allCards() {
		if !known[c] {
			baseDeck = append(baseDeck, c)
		}
	}
	boardNeeded := 5 - len(community)

	var wins, ties, losses int
	deck := make([]poker.Card, 0, len(baseDeck))
	heroCards := make([]poker.Card, 7)
	oppCards := make([]poker.Card, 7)

	for it := 0; it < iterations; it++ {
		combo := compat[rand.IntN(len(compat))]

		// Build this iteration's deck: baseDeck minus villain's two cards.
		deck = deck[:0]
		for _, c := range baseDeck {
			if c == combo[0] || c == combo[1] {
				continue
			}
			deck = append(deck, c)
		}

		// Partial Fisher-Yates for the first boardNeeded slots.
		for j := 0; j < boardNeeded && j < len(deck); j++ {
			k := j + rand.IntN(len(deck)-j)
			deck[j], deck[k] = deck[k], deck[j]
		}

		fullBoard := make([]poker.Card, 5)
		copy(fullBoard, community)
		for b := len(community); b < 5; b++ {
			fullBoard[b] = deck[b-len(community)]
		}

		heroCards[0] = hole[0]
		heroCards[1] = hole[1]
		copy(heroCards[2:], fullBoard)
		heroRank := poker.Evaluate(heroCards)

		oppCards[0] = combo[0]
		oppCards[1] = combo[1]
		copy(oppCards[2:], fullBoard)
		oppRank := poker.Evaluate(oppCards)

		switch {
		case heroRank < oppRank:
			wins++
		case heroRank == oppRank:
			ties++
		default:
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

// facingAllInPostflopClasses approximates an opponent's postflop shove range:
// value hands (sets, overpairs, top pair / good kicker) plus strong draws and
// suited broadways — roughly top 13% of hands. This is deliberately tighter
// than a "stackoff range" because what matters when facing a committed shove
// is the range they will actually put in, not the range they'd be willing to
// call off with. Directionally right, not solver-exact.
var facingAllInPostflopClasses = []string{
	"AA", "KK", "QQ", "JJ", "TT", "99", "88", "77", "66", "55",
	"AKs", "AQs", "AJs", "ATs", "A9s", "A5s", "A4s", "A3s", "A2s",
	"AKo", "AQo", "AJo",
	"KQs", "KJs", "KTs",
	"KQo",
	"QJs", "QTs",
	"JTs",
	"T9s", "98s", "87s", "76s", "65s", "54s",
}

// facingAllInPreflopClasses is a tighter shove range (~13%) for preflop
// all-ins, where ranges are narrower than postflop stackoffs.
var facingAllInPreflopClasses = []string{
	"AA", "KK", "QQ", "JJ", "TT", "99", "88", "77",
	"AKs", "AQs", "AJs", "ATs",
	"AKo", "AQo", "AJo",
	"KQs", "KJs", "KTs",
	"KQo",
	"QJs", "QTs",
	"JTs",
}

// Cached expanded ranges.
var (
	facingAllInPostflopRange HandRange
	facingAllInPreflopRange  HandRange
)

func init() {
	facingAllInPostflopRange = expandRange(facingAllInPostflopClasses)
	facingAllInPreflopRange = expandRange(facingAllInPreflopClasses)
}

// facingAllInRangeFor returns the appropriate shove range for a given street.
func facingAllInRangeFor(street Street) HandRange {
	if street == StreetPreFlop {
		return facingAllInPreflopRange
	}
	return facingAllInPostflopRange
}
