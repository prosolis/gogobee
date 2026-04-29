package plugin

import (
	"os"
	"strings"
	"testing"

	"github.com/chehsunliu/poker"
)

var testPolicy PolicyTable

func init() {
	path := os.Getenv("HOLDEM_CFR_POLICY")
	if path == "" {
		path = "../../data/policy.gob"
	}
	p, err := LoadPolicy(path)
	if err == nil {
		testPolicy = p
	}
}

// policyStrategy looks up the trained policy for the given game state,
// falling back to the pot-odds heuristic if no entry exists.
func policyStrategy(game *HoldemGame, heroIdx int) (probs [cfrNumActions]float64, source string) {
	p := game.Players[heroIdx]
	numOpp := game.activeCount() - 1
	if numOpp < 1 {
		numOpp = 1
	}
	eq := Equity(p.Hole, game.Community, numOpp, 2000)

	if testPolicy != nil {
		eqBkt := equityBucket(eq.Win + eq.Tie*0.5)
		totalPot := game.Pot
		for _, pp := range game.Players {
			totalPot += pp.Bet
		}
		spr := 0.0
		if totalPot > 0 {
			spr = float64(p.Stack) / float64(totalPot)
		}
		sprBkt := sprBucket(spr)
		pos := game.positionLabel(heroIdx)
		boardTex := boardTexture(game.Community)
		key := buildInfoSetKey(game.Street, pos, eqBkt, sprBkt, boardTex, truncateHistory(game.StreetHistory))

		if entry, ok := testPolicy[key]; ok {
			return filterLegalActions(entry, game, heroIdx), "trained"
		}
	}

	eq2 := Equity(p.Hole, game.Community, numOpp, 2000)
	return filterLegalActions(fallbackStrategy(eq2, game, heroIdx), game, heroIdx), "fallback"
}

// TestTipScenarios_CFRAlignment builds a minimal HoldemGame from each scenario,
// looks up the trained CFR policy (falling back to pot-odds heuristic), and
// verifies the rules-engine tip doesn't recommend an action that the CFR gives
// zero weight to.
func TestTipScenarios_CFRAlignment(t *testing.T) {
	if testPolicy != nil {
		t.Logf("loaded trained policy with %d info sets", len(testPolicy))
	} else {
		t.Log("no trained policy found, using fallback heuristic only")
	}

	for _, s := range tipScenarios {
		t.Run(s.Name, func(t *testing.T) {
			ctx := scenarioContext(t, s)
			tip := strings.ToLower(generateRulesTip(ctx))

			game := buildGameFromScenario(s)
			heroIdx := 0

			probs, source := policyStrategy(game, heroIdx)

			tipAction := extractTipAction(tip)
			cfrActions := mapTipActionToCFR(tipAction)
			if len(cfrActions) == 0 {
				return
			}

			totalWeight := 0.0
			for _, cfrIdx := range cfrActions {
				totalWeight += probs[cfrIdx]
			}

			if totalWeight < 0.01 {
				t.Errorf("rules tip action %q has zero CFR weight (source: %s).\n  tip: %s\n  CFR probs: fold=%.2f call/chk=%.2f raise½=%.2f raisePot=%.2f allIn=%.2f",
					tipAction, source, tip, probs[0], probs[1], probs[2], probs[3], probs[4])
			}
		})
	}
}

// buildGameFromScenario constructs a minimal HoldemGame for CFR strategy lookup.
func buildGameFromScenario(s TipScenario) *HoldemGame {
	hole := [2]poker.Card{
		poker.NewCard(s.HoleStr[0]),
		poker.NewCard(s.HoleStr[1]),
	}

	community := make([]poker.Card, len(s.BoardStr))
	for i, cs := range s.BoardStr {
		community[i] = poker.NewCard(cs)
	}

	numActive := s.NumActive
	if numActive < 2 {
		numActive = 2
	}

	players := make([]*HoldemPlayer, numActive)
	players[0] = &HoldemPlayer{
		DisplayName: "Hero",
		Hole:        hole,
		Stack:       s.Stack,
		State:       PlayerActive,
	}
	for i := 1; i < numActive; i++ {
		players[i] = &HoldemPlayer{
			DisplayName: "Villain",
			Hole:        [2]poker.Card{poker.NewCard("2c"), poker.NewCard("3d")},
			Stack:       s.Stack,
			State:       PlayerActive,
		}
	}

	currentBet := int64(0)
	if s.ToCall > 0 {
		currentBet = s.ToCall
		players[0].Bet = 0
	}

	pot := s.Pot
	if s.ToCall > 0 {
		pot -= s.ToCall
		if pot < 0 {
			pot = 0
		}
	}

	dealerIdx := 0
	if s.Position != "BTN" && s.Position != "SB" {
		dealerIdx = numActive - 1
	}

	return &HoldemGame{
		Players:        players,
		Community:      community,
		Street:         s.Street,
		Pot:            pot,
		CurrentBet:     currentBet,
		MinRaise:       currentBet,
		DealerIdx:      dealerIdx,
		ActionIdx:      0,
		HandInProgress: true,
	}
}

// mapTipActionToCFR maps a rules-engine action verb to corresponding CFR action indices.
func mapTipActionToCFR(action string) []int {
	switch action {
	case "fold":
		return []int{cfrFold}
	case "call", "check":
		return []int{cfrCallCheck}
	case "bet", "raise":
		return []int{cfrRaiseHalf, cfrRaisePot, cfrAllIn}
	case "shove":
		return []int{cfrAllIn}
	default:
		return nil
	}
}
