package plugin

import (
	"encoding/gob"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"sync/atomic"
	"time"

	"github.com/chehsunliu/poker"
)

// TrainProgress tracks iteration progress across workers for logging.
type TrainProgress struct {
	Total     int
	Completed atomic.Int64
	StartTime time.Time
}

// CFR action indices.
const (
	cfrFold       = 0
	cfrCallCheck  = 1
	cfrRaiseHalf  = 2
	cfrRaisePot   = 3
	cfrAllIn      = 4
	cfrNumActions = 5
)

// Maximum raises per street in the simplified game tree.
const cfrMaxRaisesPerStreet = 2

// Regret pruning threshold — actions below this are skipped after warmup.
const cfrPruneThreshold = -300.0

// Equity bucket thresholds (12 buckets for finer granularity).
func equityBucket(eq float64) int {
	switch {
	case eq < 0.08:
		return 0 // Trash
	case eq < 0.17:
		return 1 // Air
	case eq < 0.25:
		return 2 // Weak
	case eq < 0.33:
		return 3 // Below avg
	case eq < 0.42:
		return 4 // Marginal low
	case eq < 0.50:
		return 5 // Marginal high
	case eq < 0.58:
		return 6 // Above avg
	case eq < 0.67:
		return 7 // Good
	case eq < 0.75:
		return 8 // Strong
	case eq < 0.83:
		return 9 // Very strong
	case eq < 0.92:
		return 10 // Premium
	default:
		return 11 // Monster
	}
}

// SPR bucket thresholds (5 buckets).
func sprBucket(spr float64) int {
	switch {
	case spr < 1:
		return 0 // micro
	case spr < 3:
		return 1 // low
	case spr < 6:
		return 2 // medium
	case spr < 12:
		return 3 // high
	default:
		return 4 // deep
	}
}

// Board texture categories (post-flop only).
const (
	boardDry    = 0 // no flush/straight draws, no pairs
	boardWet    = 1 // flush draw or straight draw potential
	boardPaired = 2 // board has a pair or trips
)

// boardTexture classifies the community cards into dry/wet/paired.
func boardTexture(community []poker.Card) int {
	if len(community) < 3 {
		return boardDry
	}

	// Check for paired board.
	rankCounts := make(map[byte]int)
	suitCounts := make(map[byte]int)
	var rankValues []int

	for _, c := range community {
		s := c.String()
		if len(s) >= 2 {
			rankCounts[s[0]]++
			suitCounts[s[1]]++
			rankValues = append(rankValues, cardRankIndex(c))
		}
	}

	for _, count := range rankCounts {
		if count >= 2 {
			return boardPaired
		}
	}

	// Check for flush draw (3+ of same suit).
	for _, count := range suitCounts {
		if count >= 3 {
			return boardWet
		}
	}

	// Check for straight draw potential (3+ cards within a 5-card span).
	if len(rankValues) >= 3 {
		// Sort ranks.
		for i := 0; i < len(rankValues); i++ {
			for j := i + 1; j < len(rankValues); j++ {
				if rankValues[j] < rankValues[i] {
					rankValues[i], rankValues[j] = rankValues[j], rankValues[i]
				}
			}
		}
		// Check if any 3 consecutive sorted ranks fit in a 5-card window.
		for i := 0; i <= len(rankValues)-3; i++ {
			if rankValues[i+2]-rankValues[i] <= 4 {
				return boardWet
			}
		}
	}

	return boardDry
}

// ── Preflop Hand Strength Lookup ────────────────────────────────────────────

// preflopEquityTable maps the 169 strategically distinct starting hands to
// precomputed equity buckets. Built once at init via Monte Carlo (10K iterations
// per hand class). This eliminates all MC work during preflop training nodes.
var preflopEquityTable [13][13]int // [rank1][rank2], suited when rank1 < rank2

func init() {
	// We compute these once at startup. ~169 * 10K MC = ~1.7M evaluations,
	// takes about 1-2 seconds but saves billions of MC calls during training.
	ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}

	for i := 0; i < 13; i++ {
		for j := i; j < 13; j++ {
			// Pick representative cards for this hand class.
			var hole [2]poker.Card
			if i == j {
				// Pair: use two different suits.
				hole = [2]poker.Card{
					poker.NewCard(ranks[i] + "s"),
					poker.NewCard(ranks[j] + "h"),
				}
			} else {
				// Suited (stored in upper triangle).
				hole = [2]poker.Card{
					poker.NewCard(ranks[i] + "s"),
					poker.NewCard(ranks[j] + "s"),
				}
			}
			eq := Equity(hole, nil, 1, 10000)
			preflopEquityTable[i][j] = equityBucket(eq.Win + eq.Tie*0.5)

			if i != j {
				// Offsuit (lower triangle).
				hole = [2]poker.Card{
					poker.NewCard(ranks[i] + "s"),
					poker.NewCard(ranks[j] + "h"),
				}
				eq = Equity(hole, nil, 1, 10000)
				preflopEquityTable[j][i] = equityBucket(eq.Win + eq.Tie*0.5)
			}
		}
	}
}

// cardRankIndex returns 0-12 for card rank (2=0, 3=1, ..., A=12).
func cardRankIndex(c poker.Card) int {
	// poker.Card ranks: the library uses specific encoding.
	// We extract the rank string and map it.
	s := c.String()
	if len(s) < 1 {
		return 0
	}
	switch s[0] {
	case '2':
		return 0
	case '3':
		return 1
	case '4':
		return 2
	case '5':
		return 3
	case '6':
		return 4
	case '7':
		return 5
	case '8':
		return 6
	case '9':
		return 7
	case 'T':
		return 8
	case 'J':
		return 9
	case 'Q':
		return 10
	case 'K':
		return 11
	case 'A':
		return 12
	}
	return 0
}

// cardSuitChar returns the suit character for a card.
func cardSuitChar(c poker.Card) byte {
	s := c.String()
	if len(s) >= 2 {
		return s[1]
	}
	return '?'
}

// preflopBucket returns the precomputed equity bucket for a hole hand.
func preflopBucket(hole [2]poker.Card) int {
	r0 := cardRankIndex(hole[0])
	r1 := cardRankIndex(hole[1])
	suited := cardSuitChar(hole[0]) == cardSuitChar(hole[1])

	lo, hi := r0, r1
	if lo > hi {
		lo, hi = hi, lo
	}

	if suited {
		return preflopEquityTable[lo][hi] // upper triangle = suited
	}
	if lo == hi {
		return preflopEquityTable[lo][hi] // diagonal = pair
	}
	return preflopEquityTable[hi][lo] // lower triangle = offsuit
}

// ── Fast Training Equity ────────────────────────────────────────────────────

// trainingEquityFast computes equity for post-flop using the already-dealt
// remaining deck, avoiding allCards()/map rebuilds. Uses partial Fisher-Yates.
func trainingEquityFast(hero [2]poker.Card, community []poker.Card, remaining []poker.Card, iterations int) float64 {
	// Build a deck of unknowns (remaining minus hero cards and community).
	// remaining already excludes the 4 hole cards from the deal.
	// community is a sub-slice of remaining, so we need to skip those indices.
	boardLen := len(community)

	// The unknowns start after the community cards in remaining.
	unknowns := remaining[boardLen:]
	boardNeeded := 5 - boardLen
	cardsNeeded := 2 + boardNeeded // 1 opponent + board completion

	if cardsNeeded > len(unknowns) {
		cardsNeeded = len(unknowns)
	}

	var wins, ties int
	var heroCards, oppCards [7]poker.Card
	heroCards[0] = hero[0]
	heroCards[1] = hero[1]

	var fullBoard [5]poker.Card
	copy(fullBoard[:boardLen], community)

	for i := 0; i < iterations; i++ {
		// Partial Fisher-Yates on unknowns.
		for j := 0; j < cardsNeeded; j++ {
			k := j + rand.IntN(len(unknowns)-j)
			unknowns[j], unknowns[k] = unknowns[k], unknowns[j]
		}

		// Opponent hole cards.
		oppCards[0] = unknowns[0]
		oppCards[1] = unknowns[1]

		// Complete board.
		for b := 0; b < boardNeeded; b++ {
			fullBoard[boardLen+b] = unknowns[2+b]
		}

		copy(heroCards[2:], fullBoard[:])
		copy(oppCards[2:], fullBoard[:])

		heroRank := poker.Evaluate(heroCards[:])
		oppRank := poker.Evaluate(oppCards[:])

		if heroRank < oppRank {
			wins++
		} else if heroRank == oppRank {
			ties++
		}
	}

	return float64(wins)/float64(iterations) + float64(ties)/float64(iterations)*0.5
}

// trainingEquity computes equity for runtime/validation using the standard Equity function.
func trainingEquity(hero [2]poker.Card, community []poker.Card, iterations int) float64 {
	eq := Equity(hero, community, 1, iterations)
	return eq.Win + eq.Tie*0.5
}

// ── Integer Info Set Keys ───────────────────────────────────────────────────

// InfoSetKey packs all info set dimensions into a uint64 for fast map access.
// Layout: [street:3][position:1][eqBucket:4][sprBucket:3][boardTex:2][history:24][histLen:3]
//
// History is encoded as up to 6 action chars, 4 bits each.
type InfoSetKey = uint64

// RegretTableInt maps integer info set keys to cumulative regrets per action.
type RegretTableInt map[InfoSetKey][cfrNumActions]float64

// PolicyTable maps info set keys to action probability distributions.
type PolicyTable map[string][cfrNumActions]float64

// RegretTable maps info set keys to cumulative regrets per action.
// Used for gob serialization (string keys for compatibility).
type RegretTable map[string][cfrNumActions]float64

// CFRTrainingMeta stores metadata about a trained policy.
type CFRTrainingMeta struct {
	Iterations int
	Seed       int64
	Date       string
}

// CFRData holds both regret and strategy tables for training persistence.
type CFRData struct {
	Regrets  RegretTable
	Strategy RegretTable // cumulative strategy (sum of all iteration strategies)
	Meta     CFRTrainingMeta
}

func packInfoSetKey(street Street, posIP bool, eqBucket, sprBkt, boardTex int, history string) InfoSetKey {
	var key uint64
	key |= uint64(street) & 0x7         // bits 0-2
	if posIP {
		key |= 1 << 3                   // bit 3
	}
	key |= (uint64(eqBucket) & 0xF) << 4  // bits 4-7 (4 bits for 12 buckets)
	key |= (uint64(sprBkt) & 0x7) << 8    // bits 8-10
	key |= (uint64(boardTex) & 0x3) << 11 // bits 11-12

	// Pack up to 6 history chars, 4 bits each (bits 13-36).
	h := history
	if len(h) > 6 {
		h = h[len(h)-6:]
	}
	for i := 0; i < len(h); i++ {
		var v uint64
		switch h[i] {
		case 'f':
			v = 1
		case 'c':
			v = 2
		case 'r':
			v = 3
		case 'R':
			v = 4
		case 'a':
			v = 5
		}
		key |= v << (13 + uint(i)*4)
	}
	// Encode history length (bits 37-39).
	key |= uint64(len(h)) << 37

	return key
}

func infoSetKeyToString(key InfoSetKey) string {
	street := key & 0x7
	pos := "OOP"
	if (key>>3)&1 == 1 {
		pos = "IP"
	}
	eqBkt := (key >> 4) & 0xF
	sprBkt := (key >> 8) & 0x7
	boardTex := (key >> 11) & 0x3
	hLen := (key >> 37) & 0x7
	var history string
	charMap := [6]byte{0, 'f', 'c', 'r', 'R', 'a'}
	for i := uint64(0); i < hLen; i++ {
		v := (key >> (13 + i*4)) & 0xF
		if v < 6 {
			history += string(charMap[v])
		}
	}
	return fmt.Sprintf("%d|%s|%d|%d|%d|%s", street, pos, eqBkt, sprBkt, boardTex, history)
}

// buildInfoSetKey constructs a string info set key (used for runtime policy lookup).
func buildInfoSetKey(street Street, position string, eqBucket, sprBkt, boardTex int, actionHistory string) string {
	return fmt.Sprintf("%d|%s|%d|%d|%d|%s", street, position, eqBucket, sprBkt, boardTex, actionHistory)
}

// truncateHistory keeps only the last 6 action characters.
func truncateHistory(h string) string {
	if len(h) > 6 {
		return h[len(h)-6:]
	}
	return h
}

// actionChar maps CFR action index to a history character.
func actionChar(a int) byte {
	switch a {
	case cfrFold:
		return 'f'
	case cfrCallCheck:
		return 'c'
	case cfrRaiseHalf:
		return 'r'
	case cfrRaisePot:
		return 'R'
	case cfrAllIn:
		return 'a'
	default:
		return '?'
	}
}

// getStrategy computes the current strategy from regrets via regret matching.
func getStrategy(regrets [cfrNumActions]float64) [cfrNumActions]float64 {
	var strategy [cfrNumActions]float64
	positiveSum := 0.0
	for _, r := range regrets {
		if r > 0 {
			positiveSum += r
		}
	}
	if positiveSum > 0 {
		for i, r := range regrets {
			if r > 0 {
				strategy[i] = r / positiveSum
			}
		}
	} else {
		// Uniform strategy.
		for i := range strategy {
			strategy[i] = 1.0 / float64(cfrNumActions)
		}
	}
	return strategy
}

// sampleAction samples an action index from a probability distribution.
func sampleAction(probs [cfrNumActions]float64) int {
	r := rand.Float64()
	cumulative := 0.0
	for i, p := range probs {
		cumulative += p
		if r < cumulative {
			return i
		}
	}
	return cfrNumActions - 1
}

// LoadPolicy loads a pre-trained policy table from a gob file.
func LoadPolicy(path string) (PolicyTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open policy: %w", err)
	}
	defer f.Close()

	var data CFRData
	if err := gob.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}

	slog.Info("holdem: loaded CFR policy",
		"entries", len(data.Strategy),
		"iterations", data.Meta.Iterations,
		"date", data.Meta.Date)

	// Normalize strategy table to produce probabilities.
	policy := make(PolicyTable, len(data.Strategy))
	for key, strat := range data.Strategy {
		var total float64
		for _, v := range strat {
			total += v
		}
		var probs [cfrNumActions]float64
		if total > 0 {
			for i, v := range strat {
				probs[i] = v / total
			}
		} else {
			for i := range probs {
				probs[i] = 1.0 / float64(cfrNumActions)
			}
		}
		policy[key] = probs
	}

	return policy, nil
}

// SaveCFRData saves training data (regrets + strategy) to a gob file.
func SaveCFRData(path string, data *CFRData) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(data); err != nil {
		return fmt.Errorf("encode data: %w", err)
	}
	return nil
}

// LoadCFRData loads training checkpoint data.
func LoadCFRData(path string) (*CFRData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open checkpoint: %w", err)
	}
	defer f.Close()

	var data CFRData
	if err := gob.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	return &data, nil
}

// NPCChooseAction selects an action for the NPC using the policy table.
func NPCChooseAction(policy PolicyTable, g *HoldemGame, npcIdx int) (action int, delay time.Duration) {
	p := g.Players[npcIdx]

	// Compute equity.
	numOpp := g.activeCount() - 1
	if numOpp < 1 {
		numOpp = 1
	}
	eq := Equity(p.Hole, g.Community, numOpp, 1000)

	// Build info set key.
	eqBkt := equityBucket(eq.Win + eq.Tie*0.5)

	totalPot := g.Pot
	for _, pp := range g.Players {
		totalPot += pp.Bet
	}
	spr := 0.0
	if totalPot > 0 {
		spr = float64(p.Stack) / float64(totalPot)
	}
	sprBkt := sprBucket(spr)

	pos := g.positionLabel(npcIdx)
	history := buildActionHistory(g)
	boardTex := boardTexture(g.Community)

	key := buildInfoSetKey(g.Street, pos, eqBkt, sprBkt, boardTex, truncateHistory(history))

	probs, ok := policy[key]
	if !ok {
		// Fallback: pot-odds rule.
		probs = fallbackStrategy(eq, g, npcIdx)
	}

	// Filter out illegal actions.
	probs = filterLegalActions(probs, g, npcIdx)

	action = sampleAction(probs)

	// Random delay for natural feel.
	delayMs := 500 + rand.IntN(1500)
	delay = time.Duration(delayMs) * time.Millisecond

	return action, delay
}

// fallbackStrategy produces a simple strategy when no policy entry exists.
func fallbackStrategy(eq EquityResult, g *HoldemGame, npcIdx int) [cfrNumActions]float64 {
	p := g.Players[npcIdx]
	equity := eq.Win + eq.Tie*0.5

	toCall := g.CurrentBet - p.Bet
	totalPot := g.Pot
	for _, pp := range g.Players {
		totalPot += pp.Bet
	}

	potOdds := 0.0
	if toCall > 0 && totalPot+toCall > 0 {
		potOdds = float64(toCall) / float64(totalPot+toCall)
	}

	var probs [cfrNumActions]float64

	switch {
	case equity > 0.8:
		probs[cfrRaisePot] = 0.6
		probs[cfrAllIn] = 0.2
		probs[cfrCallCheck] = 0.2
	case equity > 0.6:
		probs[cfrRaiseHalf] = 0.4
		probs[cfrCallCheck] = 0.5
		probs[cfrFold] = 0.1
	case toCall > 0 && equity > potOdds:
		probs[cfrCallCheck] = 0.7
		probs[cfrRaiseHalf] = 0.2
		probs[cfrFold] = 0.1
	case toCall > 0:
		probs[cfrFold] = 0.7
		probs[cfrCallCheck] = 0.3
	default:
		probs[cfrCallCheck] = 0.6 // check
		probs[cfrRaiseHalf] = 0.3
		probs[cfrFold] = 0.1
	}

	return probs
}

// filterLegalActions zeroes out illegal actions and renormalizes.
func filterLegalActions(probs [cfrNumActions]float64, g *HoldemGame, npcIdx int) [cfrNumActions]float64 {
	p := g.Players[npcIdx]
	toCall := g.CurrentBet - p.Bet

	// Can't check if there's a bet.
	if toCall > 0 {
		// cfrCallCheck is "call" here, which is legal.
	} else {
		// Can't fold if no bet (well, technically can but shouldn't).
		probs[cfrFold] = 0
	}

	// Can't raise if stack is 0 or would be under min raise.
	if p.Stack <= toCall {
		probs[cfrRaiseHalf] = 0
		probs[cfrRaisePot] = 0
		probs[cfrAllIn] = 0
		if toCall > 0 {
			// Can only call or fold.
		} else {
			probs[cfrCallCheck] = 1.0
		}
	}

	// Renormalize.
	total := 0.0
	for _, v := range probs {
		total += v
	}
	if total > 0 {
		for i := range probs {
			probs[i] /= total
		}
	} else {
		// Default to call/check.
		probs[cfrCallCheck] = 1.0
	}

	return probs
}

// buildActionHistory returns the action history for the current street,
// matching the format used during CFR training (f/c/r/R/a chars).
func buildActionHistory(g *HoldemGame) string {
	return truncateHistory(g.StreetHistory)
}

// cfrActionToGameAction converts a CFR action index to concrete game parameters.
func cfrActionToGameAction(action int, g *HoldemGame, npcIdx int) (string, int64) {
	p := g.Players[npcIdx]
	toCall := g.CurrentBet - p.Bet

	totalPot := g.Pot
	for _, pp := range g.Players {
		totalPot += pp.Bet
	}

	switch action {
	case cfrFold:
		if toCall <= 0 {
			return "check", 0 // don't fold when checking is free
		}
		return "fold", 0
	case cfrCallCheck:
		if toCall > 0 {
			return "call", 0
		}
		return "check", 0
	case cfrRaiseHalf:
		raiseSize := totalPot / 2
		if raiseSize < g.MinRaise {
			raiseSize = g.MinRaise
		}
		raiseTo := g.CurrentBet + raiseSize
		maxRaise := p.Bet + p.Stack
		if raiseTo > maxRaise {
			return "allin", 0
		}
		return "raise", raiseTo
	case cfrRaisePot:
		raiseSize := totalPot
		if raiseSize < g.MinRaise {
			raiseSize = g.MinRaise
		}
		raiseTo := g.CurrentBet + raiseSize
		maxRaise := p.Bet + p.Stack
		if raiseTo > maxRaise {
			return "allin", 0
		}
		return "raise", raiseTo
	case cfrAllIn:
		return "allin", 0
	default:
		return "check", 0
	}
}

// ── Training Engine ─────────────────────────────────────────────────────────

// TrainCFR runs External Sampling MCCFR for the given number of iterations.
func TrainCFR(data *CFRData, iterations int, progressEvery int, workerLabel string, progress *TrainProgress) {
	// Use integer-keyed tables internally for speed, convert at the end.
	regrets := make(RegretTableInt, len(data.Regrets))
	strategy := make(RegretTableInt, len(data.Strategy))

	// Import existing string-keyed data.
	for k, v := range data.Regrets {
		// Parse the string key back... or just start fresh for training.
		// Since we need a fresh start anyway (broken policy), we skip import.
		_ = k
		_ = v
	}

	lastLog := time.Now()
	logInterval := 30 * time.Second

	for i := 0; i < iterations; i++ {
		// Create a random game state.
		deck := newShuffledDeck()
		holes := [2][2]poker.Card{
			{deck[0], deck[1]},
			{deck[2], deck[3]},
		}

		prune := i >= 500_000 // enable regret pruning after warmup

		// Traverse for each player.
		for player := 0; player < 2; player++ {
			cfrTraverseFast(regrets, strategy, holes, deck[4:], player, StreetPreFlop, "", 20, 20, 1, 2, 0, 0, prune)
		}

		if progress != nil {
			progress.Completed.Add(1)
		}

		hitInterval := progressEvery > 0 && (i+1)%progressEvery == 0
		hitTimer := time.Since(lastLog) >= logInterval
		if hitInterval || hitTimer {
			lastLog = time.Now()
			attrs := []any{
				"iteration", i + 1,
				"worker_total", iterations,
				"nodes", len(regrets),
			}
			if workerLabel != "" {
				attrs = append(attrs, "worker", workerLabel)
			}
			if progress != nil {
				completed := int(progress.Completed.Load())
				pct := float64(completed) / float64(progress.Total) * 100
				elapsed := time.Since(progress.StartTime)
				eta := time.Duration(0)
				if completed > 0 {
					eta = time.Duration(float64(elapsed) / float64(completed) * float64(progress.Total-completed))
				}
				attrs = append(attrs,
					"overall", fmt.Sprintf("%d/%d (%.1f%%)", completed, progress.Total, pct),
					"eta", eta.Round(time.Second),
				)
			}
			slog.Info("CFR training progress", attrs...)
		}
	}

	// Convert integer-keyed tables back to string-keyed for serialization.
	data.Regrets = make(RegretTable, len(regrets))
	data.Strategy = make(RegretTable, len(strategy))
	for k, v := range regrets {
		data.Regrets[infoSetKeyToString(k)] = v
	}
	for k, v := range strategy {
		data.Strategy[infoSetKeyToString(k)] = v
	}
}

// cfrTraverseFast is the optimized training traversal using integer keys,
// preflop lookup, fast equity, raise caps, and regret pruning.
func cfrTraverseFast(
	regrets, strategy RegretTableInt,
	holes [2][2]poker.Card,
	remaining []poker.Card,
	traversingPlayer int,
	street Street,
	history string,
	stack0, stack1 int64,
	pot int64,
	currentBet int64,
	depth int,
	raisesThisStreet int,
	prune bool,
) float64 {
	if depth > 20 || street == StreetShowdown {
		return cfrTerminalValue(holes, remaining, street, traversingPlayer, pot, stack0, stack1)
	}

	// Determine whose turn it is based on history length (alternating).
	actingPlayer := len(history) % 2

	// Compute equity bucket and board texture.
	var eqBkt int
	var boardTex int
	if street == StreetPreFlop {
		// Use precomputed lookup — zero MC cost.
		eqBkt = preflopBucket(holes[actingPlayer])
		boardTex = boardDry // no board yet
	} else {
		// Post-flop: fast equity using pre-dealt remaining cards.
		var community []poker.Card
		switch street {
		case StreetFlop:
			community = remaining[:3]
		case StreetTurn:
			community = remaining[:4]
		case StreetRiver:
			community = remaining[:5]
		}
		eqVal := trainingEquityFast(holes[actingPlayer], community, remaining, 30)
		eqBkt = equityBucket(eqVal)
		boardTex = boardTexture(community)
	}

	spr := 0.0
	if pot > 0 {
		stack := stack0
		if actingPlayer == 1 {
			stack = stack1
		}
		spr = float64(stack) / float64(pot)
	}
	sprBkt := sprBucket(spr)

	posIP := actingPlayer == 1

	key := packInfoSetKey(street, posIP, eqBkt, sprBkt, boardTex, history)

	regretArr := regrets[key]
	strat := getStrategy(regretArr)

	// Accumulate strategy for average policy.
	stratArr := strategy[key]
	for a := 0; a < cfrNumActions; a++ {
		stratArr[a] += strat[a]
	}
	strategy[key] = stratArr

	// Determine which actions are available (raise cap).
	raiseAllowed := raisesThisStreet < cfrMaxRaisesPerStreet

	if actingPlayer != traversingPlayer {
		// External sampling: sample one action for the opponent.
		// If raises not allowed, redistribute raise probability to call.
		samplingStrat := strat
		if !raiseAllowed {
			samplingStrat = clampRaises(strat)
		}
		action := sampleAction(samplingStrat)

		// Fold: opponent forfeits — traversing player wins the pot.
		if action == cfrFold {
			return float64(pot) / 2.0
		}

		newHistory := history + string(actionChar(action))
		newRaises := raisesThisStreet
		if action == cfrRaiseHalf || action == cfrRaisePot {
			newRaises++
		}

		ns0, ns1, np, nb, ns, nd := applyTrainingAction(action, actingPlayer, stack0, stack1, pot, currentBet, street, depth)

		// Reset raise counter on street change.
		if ns != street {
			newRaises = 0
		}

		return cfrTraverseFast(regrets, strategy, holes, remaining, traversingPlayer, ns, newHistory, ns0, ns1, np, nb, nd, newRaises, prune)
	}

	// Traversing player: enumerate all actions.
	var actionValues [cfrNumActions]float64
	nodeValue := 0.0

	for a := 0; a < cfrNumActions; a++ {
		// Skip raise actions if raise cap reached.
		if !raiseAllowed && (a == cfrRaiseHalf || a == cfrRaisePot) {
			actionValues[a] = actionValues[cfrCallCheck] // treat as call
			nodeValue += strat[a] * actionValues[a]
			continue
		}

		// Regret pruning: skip deeply negative regret actions after warmup.
		if prune && regretArr[a] < cfrPruneThreshold {
			actionValues[a] = 0
			continue
		}

		// Fold: traversing player forfeits — they lose their share of the pot.
		if a == cfrFold {
			actionValues[a] = -float64(pot) / 2.0
			nodeValue += strat[a] * actionValues[a]
			continue
		}

		newHistory := history + string(actionChar(a))
		newRaises := raisesThisStreet
		if a == cfrRaiseHalf || a == cfrRaisePot {
			newRaises++
		}

		_, ns0, ns1, np, nb, ns, nd := applyTrainingActionFull(a, actingPlayer, stack0, stack1, pot, currentBet, street, depth)

		if ns != street {
			newRaises = 0
		}

		actionValues[a] = cfrTraverseFast(regrets, strategy, holes, remaining, traversingPlayer, ns, newHistory, ns0, ns1, np, nb, nd, newRaises, prune)
		nodeValue += strat[a] * actionValues[a]
	}

	// Update regrets.
	for a := 0; a < cfrNumActions; a++ {
		regretArr[a] += actionValues[a] - nodeValue
	}
	regrets[key] = regretArr

	return nodeValue
}

// clampRaises redistributes raise probability to call when raises are capped.
func clampRaises(strat [cfrNumActions]float64) [cfrNumActions]float64 {
	clamped := strat
	clamped[cfrCallCheck] += clamped[cfrRaiseHalf] + clamped[cfrRaisePot]
	clamped[cfrRaiseHalf] = 0
	clamped[cfrRaisePot] = 0
	return clamped
}

// cfrTerminalValue computes the payoff at a terminal node.
func cfrTerminalValue(
	holes [2][2]poker.Card,
	remaining []poker.Card,
	street Street,
	traversingPlayer int,
	pot, stack0, stack1 int64,
) float64 {
	// Deal out remaining community cards.
	var community []poker.Card
	if len(remaining) >= 5 {
		community = remaining[:5]
	} else {
		community = remaining
	}

	rank0, _ := handRank(holes[0], community)
	rank1, _ := handRank(holes[1], community)

	halfPot := float64(pot) / 2.0

	if rank0 < rank1 {
		// Player 0 wins.
		if traversingPlayer == 0 {
			return halfPot
		}
		return -halfPot
	} else if rank1 < rank0 {
		// Player 1 wins.
		if traversingPlayer == 1 {
			return halfPot
		}
		return -halfPot
	}
	return 0 // tie
}

// applyTrainingAction applies a CFR action and returns new game state.
func applyTrainingAction(
	action, actor int,
	s0, s1, pot, currentBet int64,
	street Street,
	depth int,
) (ns0, ns1, newPot, newBet int64, newStreet Street, newDepth int) {
	_, ns0, ns1, newPot, newBet, newStreet, newDepth = applyTrainingActionFull(action, actor, s0, s1, pot, currentBet, street, depth)
	return
}

// applyTrainingActionFull applies a CFR action with full return values.
func applyTrainingActionFull(
	action, actor int,
	s0, s1, pot, currentBet int64,
	street Street,
	depth int,
) (folded bool, ns0, ns1, newPot, newBet int64, newStreet Street, newDepth int) {
	ns0, ns1, newPot, newBet = s0, s1, pot, currentBet
	newStreet = street
	newDepth = depth + 1

	betSize := func(frac float64) int64 {
		return int64(math.Max(float64(pot)*frac, 2))
	}

	stack := &ns0
	if actor == 1 {
		stack = &ns1
	}

	switch action {
	case cfrFold:
		folded = true
		newStreet = StreetShowdown
	case cfrCallCheck:
		callAmt := currentBet
		if callAmt > *stack {
			callAmt = *stack
		}
		*stack -= callAmt
		newPot += callAmt
		newBet = 0
		// Advance street after call (simplified: assume 2-player, 1 raise per street).
		if newStreet < StreetRiver {
			newStreet++
		} else {
			newStreet = StreetShowdown
		}
	case cfrRaiseHalf:
		amt := betSize(0.5)
		if amt > *stack {
			amt = *stack
		}
		*stack -= amt
		newPot += amt
		newBet = amt
	case cfrRaisePot:
		amt := betSize(1.0)
		if amt > *stack {
			amt = *stack
		}
		*stack -= amt
		newPot += amt
		newBet = amt
	case cfrAllIn:
		newPot += *stack
		*stack = 0
		newStreet = StreetShowdown
	}

	return
}

// ── Validation ──────────────────────────────────────────────────────────────

// ValidatePolicy plays test hands between a trained policy and a random opponent.
// The policy plays both positions (alternating), and we simulate multi-street play
// using the same CFR action format as training.
func ValidatePolicy(policy PolicyTable, numHands int) (winRate, vpip, aggFactor float64) {
	wins := 0
	vpipHands := 0
	raises := 0
	calls := 0
	totalChips := int64(0)

	startStack := int64(20)
	bigBlind := int64(2)

	for i := 0; i < numHands; i++ {
		deck := newShuffledDeck()
		holes := [2][2]poker.Card{
			{deck[0], deck[1]},
			{deck[2], deck[3]},
		}
		remaining := deck[4:]

		// Alternate positions so policy plays both IP and OOP.
		policyPlayer := i % 2

		result, policyVPIP, policyRaises, policyCalls := simulateValidationHand(
			policy, holes, remaining, policyPlayer, startStack, startStack, bigBlind,
		)

		if result > 0 {
			wins++
		}
		totalChips += result
		if policyVPIP {
			vpipHands++
		}
		raises += policyRaises
		calls += policyCalls
	}

	winRate = float64(wins) / float64(numHands)
	vpip = float64(vpipHands) / float64(numHands)
	if calls > 0 {
		aggFactor = float64(raises) / float64(calls)
	}
	return
}

// simulateValidationHand plays a full hand between the trained policy and a random opponent.
func simulateValidationHand(
	policy PolicyTable,
	holes [2][2]poker.Card,
	remaining []poker.Card,
	policyPlayer int,
	stack0, stack1, bigBlind int64,
) (chipResult int64, vpip bool, policyRaises, policyCalls int) {
	pot := bigBlind + bigBlind/2 // SB + BB
	stack0 -= bigBlind / 2       // SB (player 0 = OOP)
	stack1 -= bigBlind           // BB (player 1 = IP)
	currentBet := bigBlind
	street := StreetPreFlop
	history := ""

	for depth := 0; depth < 30 && street != StreetShowdown; depth++ {
		actingPlayer := len(history) % 2

		// Build community for this street.
		var community []poker.Card
		switch street {
		case StreetFlop:
			community = remaining[:3]
		case StreetTurn:
			community = remaining[:4]
		case StreetRiver:
			community = remaining[:5]
		}

		stack := stack0
		if actingPlayer == 1 {
			stack = stack1
		}

		var action int

		if actingPlayer == policyPlayer {
			// Policy player: use trained strategy.
			eqVal := trainingEquity(holes[actingPlayer], community, 100)
			eqBkt := equityBucket(eqVal)

			spr := 0.0
			if pot > 0 {
				spr = float64(stack) / float64(pot)
			}
			sprBkt := sprBucket(spr)

			pos := "IP"
			if actingPlayer == 0 {
				pos = "OOP"
			}

			boardTex := boardTexture(community)
			key := buildInfoSetKey(street, pos, eqBkt, sprBkt, boardTex, truncateHistory(history))
			probs, ok := policy[key]
			if !ok {
				probs = [cfrNumActions]float64{0.1, 0.4, 0.25, 0.15, 0.1}
			}

			// Don't fold when checking is free.
			if currentBet == 0 {
				probs[cfrFold] = 0
				total := 0.0
				for _, p := range probs {
					total += p
				}
				if total > 0 {
					for j := range probs {
						probs[j] /= total
					}
				}
			}

			action = sampleAction(probs)

			// Track stats.
			if street == StreetPreFlop && action != cfrFold {
				vpip = true
			}
			if action == cfrRaiseHalf || action == cfrRaisePot || action == cfrAllIn {
				policyRaises++
			} else if action == cfrCallCheck && currentBet > 0 {
				policyCalls++
			}
		} else {
			// Random opponent: simple equity-based strategy.
			eqVal := trainingEquity(holes[actingPlayer], community, 50)
			if currentBet > 0 {
				if eqVal > 0.6 {
					action = cfrRaiseHalf
				} else if eqVal > 0.35 {
					action = cfrCallCheck
				} else {
					action = cfrFold
				}
			} else {
				if eqVal > 0.65 {
					action = cfrRaiseHalf
				} else {
					action = cfrCallCheck
				}
			}
		}

		history += string(actionChar(action))

		// Apply action.
		if action == cfrFold {
			halfPot := pot / 2
			if actingPlayer == policyPlayer {
				return -halfPot, vpip, policyRaises, policyCalls
			}
			return halfPot, vpip, policyRaises, policyCalls
		}

		_, stack0, stack1, pot, currentBet, street, _ = applyTrainingActionFull(
			action, actingPlayer, stack0, stack1, pot, currentBet, street, 0,
		)
	}

	// Showdown.
	var community []poker.Card
	if len(remaining) >= 5 {
		community = remaining[:5]
	} else {
		community = remaining
	}

	rank0, _ := handRank(holes[0], community)
	rank1, _ := handRank(holes[1], community)

	halfPot := pot / 2
	if rank0 < rank1 {
		if policyPlayer == 0 {
			return halfPot, vpip, policyRaises, policyCalls
		}
		return -halfPot, vpip, policyRaises, policyCalls
	} else if rank1 < rank0 {
		if policyPlayer == 1 {
			return halfPot, vpip, policyRaises, policyCalls
		}
		return -halfPot, vpip, policyRaises, policyCalls
	}
	return 0, vpip, policyRaises, policyCalls
}
