// cmd/gensolver drives TexasSolver offline to populate
// internal/plugin/testdata/solver_freqs.json for Layer 2 tip scenario tests.
//
// Usage:
//
//	GOGOBEE_SOLVER=/path/to/console_solver \
//	GOGOBEE_SOLVER_RESOURCES=/path/to/TexasSolver-v0.2.0-Linux/resources \
//	go run ./cmd/gensolver [scenario-name-substring]
//
// If no positional arg is given, every postflop scenario is solved. Results
// are *merged* into the existing fixture file — re-running one scenario does
// not wipe the others.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gogobee/internal/plugin"
)

const fixturePath = "internal/plugin/testdata/solver_freqs.json"

// Heads-up default ranges. Postflop only — preflop solving needs a full range
// tree and is out of scope for our Layer 2 validation, so we skip preflop
// scenarios entirely.
//
// These are deliberately coarse: HU BTN opens wide (~70%), BB defends wide
// (~55% vs min-raise). Refine per scenario if solver output looks nonsensical.
// TexasSolver range syntax does NOT support the `22+` / `A2s+` shorthand — it
// requires explicit enumeration. These two ranges are lifted verbatim from the
// solver's own sample input file so we know they parse and produce sensible
// equilibria. Not tuned for heads-up specifically; refine later if needed.
const (
	rangeBTNOpen  = "AA,KK,QQ,JJ,TT,99:0.75,88:0.75,77:0.5,66:0.25,55:0.25,AK,AQs,AQo:0.75,AJs,AJo:0.5,ATs:0.75,A6s:0.25,A5s:0.75,A4s:0.75,A3s:0.5,A2s:0.5,KQs,KQo:0.5,KJs,KTs:0.75,K5s:0.25,K4s:0.25,QJs:0.75,QTs:0.75,Q9s:0.5,JTs:0.75,J9s:0.75,J8s:0.75,T9s:0.75,T8s:0.75,T7s:0.75,98s:0.75,97s:0.75,96s:0.5,87s:0.75,86s:0.5,85s:0.5,76s:0.75,75s:0.5,65s:0.75,64s:0.5,54s:0.75,53s:0.5,43s:0.5"
	rangeBBDefend = "QQ:0.5,JJ:0.75,TT,99,88,77,66,55,44,33,22,AKo:0.25,AQs,AQo:0.75,AJs,AJo:0.75,ATs,ATo:0.75,A9s,A8s,A7s,A6s,A5s,A4s,A3s,A2s,KQ,KJ,KTs,KTo:0.5,K9s,K8s,K7s,K6s,K5s,K4s:0.5,K3s:0.5,K2s:0.5,QJ,QTs,Q9s,Q8s,Q7s,JTs,JTo:0.5,J9s,J8s,T9s,T8s,T7s,98s,97s,96s,87s,86s,76s,75s,65s,64s,54s,53s,43s"
)

// SolverNode mirrors the recursive shape of TexasSolver's dump_result JSON.
// Every action node has: `actions` (the player-to-act's options), `strategy`
// (hand→freq map for those actions), and `childrens` (subtree per action).
type SolverNode struct {
	Actions   []string               `json:"actions"`
	Strategy  *StrategyBlock         `json:"strategy,omitempty"`
	Childrens map[string]*SolverNode `json:"childrens,omitempty"`
	NodeType  string                 `json:"node_type,omitempty"`
	Player    int                    `json:"player,omitempty"`
}

type StrategyBlock struct {
	Actions  []string             `json:"actions"`
	Strategy map[string][]float64 `json:"strategy"`
}

func main() {
	flag.Parse()
	filter := strings.ToLower(flag.Arg(0))

	solverBin := os.Getenv("GOGOBEE_SOLVER")
	resourceDir := os.Getenv("GOGOBEE_SOLVER_RESOURCES")
	if solverBin == "" || resourceDir == "" {
		fmt.Fprintln(os.Stderr, "set GOGOBEE_SOLVER and GOGOBEE_SOLVER_RESOURCES")
		os.Exit(2)
	}

	existing := loadFixture()
	workDir, err := os.MkdirTemp("", "gensolver-*")
	must(err)
	defer os.RemoveAll(workDir)

	solved := 0
	for _, s := range plugin.TipScenarios() {
		if s.Street == plugin.StreetPreFlop || len(s.BoardStr) == 0 {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(s.Name), filter) {
			continue
		}
		fmt.Printf("solving: %s\n", s.Name)
		freqs, err := solveScenario(s, solverBin, resourceDir, workDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  FAILED: %v\n", err)
			continue
		}
		existing[s.Name] = freqs
		fmt.Printf("  → %v\n", freqs)
		solved++
	}

	writeFixture(existing)
	fmt.Printf("\ndone. %d scenarios solved, fixture written to %s\n", solved, fixturePath)
}

func solveScenario(s plugin.TipScenario, bin, resources, workDir string) (map[string]float64, error) {
	input := buildInputFile(s)
	inputPath := filepath.Join(workDir, "input.txt")
	outputPath := filepath.Join(workDir, "output.json")
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		return nil, err
	}
	// TexasSolver writes output_result.json to its CWD, so cd into workDir.
	cmd := exec.Command(bin, "-i", inputPath, "-r", resources)
	cmd.Dir = workDir
	cmd.Stdout = os.Stderr // surface solver logs on stderr so JSON doesn't mix in
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("solver failed: %w", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	return extractHeroFrequencies(data, s)
}

func buildInputFile(s plugin.TipScenario) string {
	board := strings.Join(s.BoardStr, ",")
	ipRange, oopRange := rangesFor(s)

	// Normalize to solver-friendly scale: pot=50, stack=8×pot, preserving
	// SPR. TexasSolver segfaults on large chip counts for certain textures
	// (suspected internal precision/overflow on some flop trees). Strategic
	// equivalence holds because GTO frequencies are scale-invariant; only
	// the raw chip values in action labels change. Caps SPR at 8 to keep
	// tree build time sane regardless of scenario stack depth.
	const normalizedPot = 50
	spr := float64(s.Stack) / float64(s.Pot)
	if spr > 8 {
		spr = 8
	}
	normalizedStack := int(float64(normalizedPot) * spr)
	if normalizedStack < 100 {
		normalizedStack = 100
	}

	var b strings.Builder
	fmt.Fprintf(&b, "set_pot %d\n", normalizedPot)
	fmt.Fprintf(&b, "set_effective_stack %d\n", normalizedStack)
	fmt.Fprintf(&b, "set_board %s\n", board)
	fmt.Fprintf(&b, "set_range_ip %s\n", ipRange)
	fmt.Fprintf(&b, "set_range_oop %s\n", oopRange)
	// Simple bet tree: half-pot + allin each street.
	for _, side := range []string{"ip", "oop"} {
		for _, street := range []string{"flop", "turn", "river"} {
			fmt.Fprintf(&b, "set_bet_sizes %s,%s,bet,50,100\n", side, street)
			fmt.Fprintf(&b, "set_bet_sizes %s,%s,raise,60\n", side, street)
			fmt.Fprintf(&b, "set_bet_sizes %s,%s,allin\n", side, street)
		}
	}
	b.WriteString("set_allin_threshold 0.67\n")
	b.WriteString("build_tree\n")
	b.WriteString("set_thread_num 8\n")
	// accuracy 1.0 = stop when total exploitability < 1% of pot. Empirically
	// this is reached in ~80 iterations on our scenarios vs 120+ for 0.5%,
	// cutting per-scenario time roughly in half with no practical loss for
	// validation (we only check if rules engine matches a significant-freq
	// action, not exact frequencies).
	b.WriteString("set_accuracy 1.0\n")
	b.WriteString("set_max_iteration 100\n")
	b.WriteString("set_print_interval 20\n")
	// Isomorphism optimization segfaults on certain flop textures (notably
	// paired boards and some two-tone). Disabling costs ~20% extra solve
	// time but makes the pipeline reliable.
	b.WriteString("set_use_isomorphism 0\n")
	b.WriteString("start_solve\n")
	b.WriteString("set_dump_rounds 1\n")
	b.WriteString("dump_result output.json\n")
	return b.String()
}

func rangesFor(s plugin.TipScenario) (ip, oop string) {
	// HU: BTN=IP, BB=OOP.
	return rangeBTNOpen, rangeBBDefend
}

// extractHeroFrequencies walks the dumped tree to find the node where hero
// is actually to act, then returns hero's action frequencies for their exact
// hole combo, normalized to {check,bet,call,fold,raise}.
//
// HU postflop ordering: OOP acts first on every street. So the root node is
// always OOP's decision.
//
//   - hero=OOP, ToCall=0  → root (OOP first to act, no action yet)
//   - hero=IP,  ToCall=0  → root.childrens["CHECK"] (OOP checked, IP facing check)
//   - hero=IP,  ToCall>0  → root.childrens["BET <x>"] matching ToCall size
//   - hero=OOP, ToCall>0  → not supported yet (check-bet line, 2 levels deep)
func extractHeroFrequencies(data []byte, s plugin.TipScenario) (map[string]float64, error) {
	var root SolverNode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	heroIP := s.Position == "BTN" || s.Position == "SB"
	// Normalize ToCall to the solver's chip scale (pot=50) so bet-child
	// matching works after the pot/stack normalization in buildInputFile.
	normalizedToCall := float64(s.ToCall) * 50.0 / float64(s.Pot)
	node, err := navigateToHero(&root, heroIP, normalizedToCall)
	if err != nil {
		return nil, err
	}
	if node.Strategy == nil {
		return nil, fmt.Errorf("hero node has no strategy (node_type=%s)", node.NodeType)
	}

	key := holeKey(s.HoleStr)
	raw, ok := node.Strategy.Strategy[key]
	if !ok {
		raw, ok = node.Strategy.Strategy[flipHole(key)]
	}
	if !ok {
		return nil, fmt.Errorf("hole %q not found in strategy (tried %q)", key, flipHole(key))
	}
	if len(raw) != len(node.Strategy.Actions) {
		return nil, fmt.Errorf("action/freq length mismatch: %d vs %d", len(raw), len(node.Strategy.Actions))
	}

	out := map[string]float64{}
	for i, act := range node.Strategy.Actions {
		out[normalizeAction(act)] += raw[i]
	}
	return out, nil
}

func navigateToHero(root *SolverNode, heroIP bool, toCall float64) (*SolverNode, error) {
	// HU postflop: OOP always acts first on each street, so root is OOP's node.
	switch {
	case !heroIP && toCall == 0:
		// OOP first to act, no prior action.
		return root, nil

	case heroIP && toCall == 0:
		// OOP checked → IP facing check.
		return childByLabel(root, "CHECK")

	case heroIP && toCall > 0:
		// OOP donk-bet (or we're mid-street with OOP having bet first). Find
		// the BET child whose chip amount is closest to the scenario's ToCall.
		return childByBetSize(root, toCall)

	case !heroIP && toCall > 0:
		// Check-bet line: OOP checks → IP bets → OOP facing bet.
		checkNode, err := childByLabel(root, "CHECK")
		if err != nil {
			return nil, fmt.Errorf("check-bet line: %w", err)
		}
		return childByBetSize(checkNode, toCall)
	}
	return nil, fmt.Errorf("unreachable")
}

func childByLabel(node *SolverNode, label string) (*SolverNode, error) {
	child, ok := node.Childrens[label]
	if !ok {
		return nil, fmt.Errorf("no %q child; available: %v", label, keysOf(node.Childrens))
	}
	return child, nil
}

// childByBetSize picks the BET child whose chip amount is closest to toCall.
// Rejects the match if the nearest bet size differs by more than 25% — that
// usually means the solver wasn't configured with a comparable sizing and the
// returned frequencies would describe a different decision.
func childByBetSize(node *SolverNode, toCall float64) (*SolverNode, error) {
	var best *SolverNode
	var bestAmt float64
	bestDelta := 1e18
	for label, child := range node.Childrens {
		if !strings.HasPrefix(label, "BET") {
			continue
		}
		amt := parseBetAmount(label)
		if amt < 0 {
			continue
		}
		d := amt - toCall
		if d < 0 {
			d = -d
		}
		if d < bestDelta {
			bestDelta = d
			bestAmt = amt
			best = child
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no BET child matching toCall=%v; available: %v", toCall, keysOf(node.Childrens))
	}
	if toCall > 0 && bestDelta/toCall > 0.25 {
		return nil, fmt.Errorf("nearest BET child %.2f is >25%% off toCall=%.2f; solver sizings don't cover this spot; available: %v", bestAmt, toCall, keysOf(node.Childrens))
	}
	return best, nil
}

func parseBetAmount(label string) float64 {
	// "BET 25.000000" → 25
	parts := strings.Fields(label)
	if len(parts) != 2 {
		return -1
	}
	var v float64
	if _, err := fmt.Sscanf(parts[1], "%f", &v); err != nil {
		return -1
	}
	return v
}

func keysOf(m map[string]*SolverNode) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// holeKey builds TexasSolver's hand key: two cards with higher rank first.
// Ranks: 2..9,T,J,Q,K,A. Suit order for same-rank pairs: s > h > d > c.
func holeKey(hole [2]string) string {
	a, b := hole[0], hole[1]
	if cardLess(b, a) {
		return a + b
	}
	return b + a
}

func flipHole(k string) string {
	if len(k) != 4 {
		return k
	}
	return k[2:] + k[:2]
}

func cardLess(a, b string) bool {
	ra := rankIdx(a[0])
	rb := rankIdx(b[0])
	if ra != rb {
		return ra < rb
	}
	return suitIdx(a[1]) < suitIdx(b[1])
}

func rankIdx(r byte) int {
	return strings.IndexByte("23456789TJQKA", r)
}

func suitIdx(s byte) int {
	return strings.IndexByte("cdhs", s)
}

func normalizeAction(a string) string {
	a = strings.ToLower(a)
	switch {
	case strings.HasPrefix(a, "check"):
		return "check"
	case strings.HasPrefix(a, "fold"):
		return "fold"
	case strings.HasPrefix(a, "call"):
		return "call"
	case strings.HasPrefix(a, "bet"):
		return "bet"
	case strings.HasPrefix(a, "raise"):
		return "raise"
	case strings.Contains(a, "allin"):
		return "raise"
	default:
		return a
	}
}

func loadFixture() map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func writeFixture(m map[string]map[string]float64) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]map[string]float64, len(m))
	for _, k := range keys {
		ordered[k] = m[k]
	}
	data, err := json.MarshalIndent(ordered, "", "  ")
	must(err)
	must(os.MkdirAll(filepath.Dir(fixturePath), 0o755))
	must(os.WriteFile(fixturePath, append(data, '\n'), 0o644))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
