package plugin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chehsunliu/poker"
)

// loadSolverFixture reads testdata/solver_freqs.json (if present) and merges
// SolverFreqs into matching scenarios. Called once at test start so Layer 2
// activates automatically whenever the fixture has been regenerated.
func loadSolverFixture() {
	data, err := os.ReadFile("testdata/solver_freqs.json")
	if err != nil {
		return
	}
	m := map[string]map[string]float64{}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	for i := range tipScenarios {
		if freqs, ok := m[tipScenarios[i].Name]; ok && len(freqs) > 0 {
			tipScenarios[i].SolverFreqs = freqs
		}
	}
}

func TestMain(m *testing.M) {
	loadSolverFixture()
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Scenario harness — shared between Layer 1 (hand-authored) and Layer 2
// (solver-derived) tip validation.
// ---------------------------------------------------------------------------

// scenarioContext reconstructs a holdemTipContext the same way the live code
// does, so the test exercises the full pipeline (equity MC, draw detection,
// hand category, board texture, preflop classification).
func scenarioContext(t *testing.T, s TipScenario) holdemTipContext {
	t.Helper()

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
	numOpp := numActive - 1
	if numOpp < 1 {
		numOpp = 1
	}

	totalPot := s.Pot

	communityS := "—"
	if len(community) > 0 {
		communityS = renderCards(community)
	}

	snap := tipSnapshot{
		hole:       hole,
		holeStr:    [2]string{renderCard(hole[0]), renderCard(hole[1])},
		community:  community,
		communityS: communityS,
		numActive:  numActive,
		numOpp:     numOpp,
		toCall:     s.ToCall,
		totalPot:   totalPot,
		stack:      s.Stack,
		street:     s.Street,
		position:   s.Position,
		headsUp:    s.HeadsUp,
		isDealer:   s.Position == "BTN" || s.Position == "SB",
	}

	return buildTipContext(snap)
}

// ---------------------------------------------------------------------------
// Layer 1 — hand-authored scenarios
// ---------------------------------------------------------------------------

func TestTipScenarios_Layer1(t *testing.T) {
	for _, s := range tipScenarios {
		t.Run(s.Name, func(t *testing.T) {
			ctx := scenarioContext(t, s)
			tip := generateRulesTip(ctx)
			lower := strings.ToLower(tip)

			if s.ExpectedAction != "" {
				if !strings.Contains(lower, s.ExpectedAction) {
					t.Errorf("expected action %q in tip, got: %s", s.ExpectedAction, tip)
				}
			}
			for _, theme := range s.ExpectedThemes {
				if !strings.Contains(lower, strings.ToLower(theme)) {
					t.Errorf("expected theme %q in tip, got: %s", theme, tip)
				}
			}
			for _, bad := range s.MustNotContain {
				if strings.Contains(lower, strings.ToLower(bad)) {
					t.Errorf("tip should not contain %q, got: %s", bad, tip)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer 2 — solver-derived scenarios
//
// When SolverFreqs is populated (from an offline fixture-generation step
// that calls TexasSolver or an equivalent), we assert that the rules
// engine's recommended action is in the solver's "significant" set —
// i.e. any action the solver picks at least 15% of the time in the
// equilibrium strategy. GTO is mixed: demanding exact top-action match
// would fail legitimately mixed spots.
//
// Until the fixture generator is wired up, this test skips per-scenario
// when SolverFreqs is nil and serves as a scaffolding hook only.
// ---------------------------------------------------------------------------

const solverSignificantFreq = 0.15

func TestTipScenarios_Layer2(t *testing.T) {
	for _, s := range tipScenarios {
		if s.SolverFreqs == nil {
			continue
		}
		t.Run(s.Name, func(t *testing.T) {
			ctx := scenarioContext(t, s)
			tip := strings.ToLower(generateRulesTip(ctx))

			matched := false
			var significant []string
			for action, freq := range s.SolverFreqs {
				if freq < solverSignificantFreq {
					continue
				}
				significant = append(significant, action)
				if strings.Contains(tip, strings.ToLower(action)) {
					matched = true
					break
				}
			}

			if !matched {
				t.Errorf("rules tip did not match any solver-significant action %v: %s", significant, tip)
			}
		})
	}
}
