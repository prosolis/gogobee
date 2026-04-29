package plugin

// TipScenario describes one canonical spot for validating poker tips.
//
// Scenarios are consumed by two layers of automated testing:
//
//  1. Layer 1 — hand-authored: ExpectedAction and ExpectedThemes are filled
//     in by a reviewer who picked the "right" answer. The test asserts that
//     the rules engine's tip contains the expected action verb and at least
//     one theme keyword. Fast, cheap, catches obvious regressions.
//
//  2. Layer 2 — solver-derived: SolverFreqs is populated from a GTO solver
//     (e.g. TexasSolver) offline and committed as a fixture. The test
//     asserts that the rules engine's action is in the solver's
//     significant-frequency set. Slower to generate once, free at test time.
//
// A single scenario can carry either or both layers — the shared test harness
// applies whichever fields are populated. The same struct is also designed to
// eventually feed the runtime dual-perspective tip display, so the schema is
// intentionally broader than just testing.
type TipScenario struct {
	Name string

	// Game state — minimum needed to reconstruct a holdemTipContext.
	HoleStr   [2]string // card strings parseable by poker.NewCard (e.g. "As", "9h")
	BoardStr  []string  // community cards; empty for preflop
	Street    Street
	Position  string // "BTN", "CO", "MP", "UTG", "SB", "BB"
	HeadsUp   bool
	NumActive int
	Stack     int64
	Pot       int64 // total pot before the facing bet
	ToCall    int64

	// Layer 1 — hand-authored expectations.
	ExpectedAction string   // one of "fold", "check", "call", "bet", "raise"
	ExpectedThemes []string // substrings the reasoning should contain
	MustNotContain []string // substrings that would indicate a wrong-action bug

	// Layer 2 — solver-derived expectations (populated by offline fixture gen).
	// Maps action to frequency, e.g. {"call": 0.55, "raise": 0.38, "fold": 0.07}.
	// The shared test treats any action with frequency ≥ 0.15 as "valid".
	SolverFreqs map[string]float64
}

// TipScenarios returns the canonical library of poker spots used by the
// scenario test harness and the cmd/gensolver offline solver pipeline.
// Exported so tools under cmd/ can iterate the list without duplicating it.
func TipScenarios() []TipScenario { return tipScenarios }

// tipScenarios is the canonical library of poker spots used by the scenario
// test harness. Seed with ~20 cases covering the main decision regions.
// Add more scenarios here as bugs are found or as the rules engine grows.
var tipScenarios = []TipScenario{
	// ---- Preflop ---------------------------------------------------------

	{
		Name:           "preflop/AA unopened BTN HU",
		HoleStr:        [2]string{"As", "Ah"},
		Street:         StreetPreFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          10000,
		Pot:            150,
		ToCall:         0,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"premium"},
	},
	{
		Name:           "preflop/AKs BTN facing 3bet",
		HoleStr:        [2]string{"As", "Ks"},
		Street:         StreetPreFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          10000,
		Pot:            900,
		ToCall:         600,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"premium"},
	},
	{
		Name:           "preflop/TT BTN unopened HU",
		HoleStr:        [2]string{"Tc", "Td"},
		Street:         StreetPreFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          10000,
		Pot:            150,
		ToCall:         0,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"strong"},
	},
	{
		Name:           "preflop/54s BB facing min-raise deep",
		HoleStr:        [2]string{"5h", "4h"},
		Street:         StreetPreFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          20000, // deep stacks → implied odds
		Pot:            300,
		ToCall:         150,
		ExpectedAction: "call",
		ExpectedThemes: []string{"heads-up"},
	},
	{
		Name:           "preflop/54s BB facing big raise shallow",
		HoleStr:        [2]string{"5h", "4h"},
		Street:         StreetPreFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          2000, // shallow → no implied odds
		Pot:            600,
		ToCall:         500,
		ExpectedAction: "fold",
		ExpectedThemes: []string{"stack"},
	},
	{
		Name:           "preflop/72o BB facing raise",
		HoleStr:        [2]string{"7c", "2d"},
		Street:         StreetPreFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          10000,
		Pot:            300,
		ToCall:         200,
		ExpectedAction: "fold",
		ExpectedThemes: []string{"weak"},
	},

	// ---- Flop ------------------------------------------------------------

	{
		Name:           "flop/top set on dry board checked to",
		HoleStr:        [2]string{"Tc", "Td"},
		BoardStr:       []string{"Th", "2c", "7d"},
		Street:         StreetFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          10000,
		Pot:            600,
		ToCall:         0,
		ExpectedAction: "bet",
		ExpectedThemes: []string{"value"},
		MustNotContain: []string{"fold", "call"},
	},
	{
		Name:           "flop/overpair on wet board facing bet",
		HoleStr:        [2]string{"Ac", "Ah"},
		BoardStr:       []string{"9s", "8s", "7s"},
		Street:         StreetFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            1200,
		ToCall:         600,
		ExpectedAction: "call",
		ExpectedThemes: []string{"wet"},
		MustNotContain: []string{"check"},
	},
	{
		Name:           "flop/flush draw with free card",
		HoleStr:        [2]string{"7h", "6h"},
		BoardStr:       []string{"Kh", "9h", "2c"},
		Street:         StreetFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            400,
		ToCall:         0,
		ExpectedAction: "check",
		ExpectedThemes: []string{"free card"},
		MustNotContain: []string{"fold", "call"},
	},
	{
		Name:           "flop/OESD facing small bet priced in",
		HoleStr:        [2]string{"5s", "4s"},
		BoardStr:       []string{"7c", "6d", "2h"},
		Street:         StreetFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            600,
		ToCall:         120, // 20% pot odds, OESD has ~32% equity
		ExpectedAction: "call",
		ExpectedThemes: []string{"price"},
		MustNotContain: []string{"check"},
	},
	{
		Name:           "flop/bottom pair facing big bet",
		HoleStr:        [2]string{"2c", "2d"},
		BoardStr:       []string{"Ks", "Td", "7h"},
		Street:         StreetFlop,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            500,
		ToCall:         500, // overbet — bad price for a weak hand
		ExpectedAction: "fold",
		MustNotContain: []string{"check"},
	},

	// ---- Turn ------------------------------------------------------------

	{
		Name:           "turn/top set facing bet",
		HoleStr:        [2]string{"Kc", "Kd"},
		BoardStr:       []string{"Kh", "5c", "2d", "8s"},
		Street:         StreetTurn,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          8000,
		Pot:            1500,
		ToCall:         750,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"value"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "turn/combo draw facing half-pot bet",
		HoleStr:        [2]string{"Jh", "Th"},
		BoardStr:       []string{"Qh", "9h", "2c", "4d"},
		Street:         StreetTurn,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            1000,
		ToCall:         500, // 33% pot odds vs 15+ outs
		ExpectedAction: "call",
		MustNotContain: []string{"check"},
	},
	{
		Name:           "turn/weak top pair facing overbet",
		HoleStr:        [2]string{"6h", "3d"},
		BoardStr:       []string{"Jd", "9s", "4c", "8h"},
		Street:         StreetTurn,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          4000,
		Pot:            800,
		ToCall:         1200, // overbet — weak kicker on coordinated board
		ExpectedAction: "fold",
		MustNotContain: []string{"check"},
	},

	// ---- River -----------------------------------------------------------

	{
		Name:           "river/nut flush checked to us",
		HoleStr:        [2]string{"Ah", "Qh"},
		BoardStr:       []string{"Kh", "9h", "2c", "5h", "3d"},
		Street:         StreetRiver,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            1500,
		ToCall:         0,
		ExpectedAction: "bet",
		ExpectedThemes: []string{"charge"},
		MustNotContain: []string{"fold", "call"},
	},
	{
		Name:           "river/bluffcatcher facing overbet",
		HoleStr:        [2]string{"5h", "4h"},
		BoardStr:       []string{"Kh", "7s", "2d", "3c", "9s"},
		Street:         StreetRiver,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          2500,
		Pot:            800,
		ToCall:         1200, // overbet — polarized, top pair is a bluffcatcher
		ExpectedAction: "fold",
		MustNotContain: []string{"check"},
	},
	{
		Name:           "river/weak hand check available",
		HoleStr:        [2]string{"7c", "6c"},
		BoardStr:       []string{"Ah", "Kd", "2s", "8c", "9d"},
		Street:         StreetRiver,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          5000,
		Pot:            600,
		ToCall:         0,
		ExpectedAction: "check",
		MustNotContain: []string{"call", "raise"},
	},
	{
		Name:           "river/second pair facing bet",
		HoleStr:        [2]string{"6d", "5d"},
		BoardStr:       []string{"Ac", "Qh", "7d", "3s", "Kc"},
		Street:         StreetRiver,
		Position:       "BB",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          4000,
		Pot:            800,
		ToCall:         1200,
		ExpectedAction: "fold",
		MustNotContain: []string{"check"},
	},
	{
		Name:           "flop/monster set on paired board facing bet",
		HoleStr:        [2]string{"9c", "9d"},
		BoardStr:       []string{"9h", "9s", "4c"},
		Street:         StreetFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          8000,
		Pot:            600,
		ToCall:         300,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"monster"},
		MustNotContain: []string{"fold"},
	},

	// ---- Multiway scenarios -----------------------------------------------

	{
		Name:           "flop/top pair multiway checked to",
		HoleStr:        [2]string{"As", "Jh"},
		BoardStr:       []string{"Jc", "7d", "3s"},
		Street:         StreetFlop,
		Position:       "BTN",
		HeadsUp:        false,
		NumActive:      4,
		Stack:          8000,
		Pot:            800,
		ToCall:         0,
		ExpectedAction: "bet",
		ExpectedThemes: []string{"value"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "flop/middle pair multiway facing bet",
		HoleStr:        [2]string{"8c", "8d"},
		BoardStr:       []string{"Jh", "5s", "2d"},
		Street:         StreetFlop,
		Position:       "CO",
		HeadsUp:        false,
		NumActive:      4,
		Stack:          5000,
		Pot:            1200,
		ToCall:         400,
		ExpectedAction: "fold",
		ExpectedThemes: []string{"multiway"},
		MustNotContain: []string{"raise"},
	},
	{
		Name:           "turn/flush draw multiway with implied odds",
		HoleStr:        [2]string{"Th", "9h"},
		BoardStr:       []string{"Kh", "5h", "2c", "Qd"},
		Street:         StreetTurn,
		Position:       "BB",
		HeadsUp:        false,
		NumActive:      3,
		Stack:          10000,
		Pot:            2000,
		ToCall:         600,
		ExpectedAction: "call",
		ExpectedThemes: []string{"draw"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "river/weak hand multiway check available",
		HoleStr:        [2]string{"6s", "5s"},
		BoardStr:       []string{"Kd", "Jh", "8c", "3d", "2h"},
		Street:         StreetRiver,
		Position:       "BB",
		HeadsUp:        false,
		NumActive:      3,
		Stack:          5000,
		Pot:            1200,
		ToCall:         0,
		ExpectedAction: "check",
		ExpectedThemes: []string{"bluff"},
		MustNotContain: []string{"call", "raise"},
	},
	{
		Name:           "preflop/K8o BTN HU",
		HoleStr:        [2]string{"Kc", "8d"},
		Street:         StreetPreFlop,
		Position:       "BTN",
		HeadsUp:        true,
		NumActive:      2,
		Stack:          19900,
		Pot:            300,
		ToCall:         100,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"heads-up"},
		MustNotContain: []string{"fold"},
	},

	// ---- Additional multiway scenarios -----------------------------------

	{
		Name:           "flop/overpair multiway facing bet",
		HoleStr:        [2]string{"Qc", "Qd"},
		BoardStr:       []string{"Jh", "7s", "3c"},
		Street:         StreetFlop,
		Position:       "CO",
		HeadsUp:        false,
		NumActive:      4,
		Stack:          8000,
		Pot:            1200,
		ToCall:         400,
		ExpectedAction: "call",
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "flop/nut flush draw multiway free card",
		HoleStr:        [2]string{"As", "5s"},
		BoardStr:       []string{"Ks", "9s", "4d"},
		Street:         StreetFlop,
		Position:       "BB",
		HeadsUp:        false,
		NumActive:      4,
		Stack:          6000,
		Pot:            800,
		ToCall:         0,
		ExpectedAction: "check",
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "turn/top pair top kicker multiway checked to",
		HoleStr:        [2]string{"Ac", "Kd"},
		BoardStr:       []string{"Ad", "8s", "3h", "5c"},
		Street:         StreetTurn,
		Position:       "BTN",
		HeadsUp:        false,
		NumActive:      3,
		Stack:          7000,
		Pot:            1500,
		ToCall:         0,
		ExpectedAction: "bet",
		ExpectedThemes: []string{"value"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "turn/gutshot multiway facing bet",
		HoleStr:        [2]string{"Jd", "Tc"},
		BoardStr:       []string{"Qs", "8h", "3c", "4d"},
		Street:         StreetTurn,
		Position:       "BB",
		HeadsUp:        false,
		NumActive:      3,
		Stack:          5000,
		Pot:            2000,
		ToCall:         1000,
		ExpectedAction: "fold",
		MustNotContain: []string{"raise"},
	},
	{
		Name:           "river/trips multiway checked to",
		HoleStr:        [2]string{"9c", "9d"},
		BoardStr:       []string{"9h", "6s", "2d", "Kc", "4h"},
		Street:         StreetRiver,
		Position:       "BTN",
		HeadsUp:        false,
		NumActive:      3,
		Stack:          6000,
		Pot:            1800,
		ToCall:         0,
		ExpectedAction: "bet",
		ExpectedThemes: []string{"value"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "flop/air multiway facing bet",
		HoleStr:        [2]string{"4c", "3d"},
		BoardStr:       []string{"Ah", "Kd", "Js"},
		Street:         StreetFlop,
		Position:       "BB",
		HeadsUp:        false,
		NumActive:      4,
		Stack:          5000,
		Pot:            1000,
		ToCall:         500,
		ExpectedAction: "fold",
		MustNotContain: []string{"raise", "call"},
	},
	{
		Name:           "preflop/TT UTG multiway",
		HoleStr:        [2]string{"Ts", "Th"},
		Street:         StreetPreFlop,
		Position:       "UTG",
		HeadsUp:        false,
		NumActive:      6,
		Stack:          10000,
		Pot:            150,
		ToCall:         0,
		ExpectedAction: "raise",
		ExpectedThemes: []string{"strong"},
		MustNotContain: []string{"fold"},
	},
	{
		Name:           "preflop/72o UTG multiway facing raise",
		HoleStr:        [2]string{"7d", "2c"},
		Street:         StreetPreFlop,
		Position:       "UTG",
		HeadsUp:        false,
		NumActive:      6,
		Stack:          10000,
		Pot:            450,
		ToCall:         300,
		ExpectedAction: "fold",
		ExpectedThemes: []string{"weak"},
	},
}
