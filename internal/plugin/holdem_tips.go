package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gogobee/internal/db"

	"github.com/chehsunliu/poker"
	"maunium.net/go/mautrix/id"
)

var holdemTipsClient = &http.Client{Timeout: 60 * time.Second}

// loadTipsPref loads a user's tip preference from the database.
func loadTipsPref(userID id.UserID) bool {
	d := db.Get()
	var enabled int
	err := d.QueryRow(`SELECT enabled FROM holdem_tips_prefs WHERE user_id = ?`, string(userID)).Scan(&enabled)
	if err != nil {
		return true // default: tips on
	}
	return enabled == 1
}

// saveTipsPref saves a user's tip preference.
func saveTipsPref(userID id.UserID, enabled bool) {
	val := 0
	if enabled {
		val = 1
	}
	db.Exec("holdem: save tips preference",
		`INSERT INTO holdem_tips_prefs (user_id, enabled) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET enabled = ?, updated_at = CURRENT_TIMESTAMP`,
		string(userID), val, val,
	)
}

// holdemTipContext holds the data needed to generate a tip.
type holdemTipContext struct {
	PlayerName   string
	Hole         [2]string // rendered card strings
	Community    string    // rendered board string
	Equity       EquityResult
	SPR          float64
	PotOddsPct   float64
	ToCall       int64
	Pot          int64
	Stack        int64
	Street       Street
	Position     string
	NumActive    int
	HandCategory string   // from poker.RankString(), e.g. "Pair", "Flush"
	Draw         DrawInfo // flush/straight draw analysis
	HeadsUp      bool

	// IsAllIn is true when at least one opponent is already all-in, meaning
	// this is a terminal decision — no future streets, no position leverage.
	IsAllIn bool
	// EquityVsShove is hero's equity vs a realistic facing-all-in shove range.
	// Only computed when IsAllIn; zero-valued otherwise. Use this instead of
	// Equity for terminal decisions, since "vs random" systematically inflates
	// high-card equity against narrow shove ranges.
	EquityVsShove EquityResult

	// Derived fields for the rules engine.
	HoleRanks          [2]int // 0-12, for preflop classification
	HoleSuited         bool
	PreflopTier        string // "premium", "strong", "speculative", "playable", "trash"
	BoardPaired        bool
	BoardMaxSuit       int // largest same-suit count on the board
	BoardStraightRisk  int // 0-4, rough straight-possibility score
	BoardHighRank      int // highest rank on board, -1 preflop
}

// tipSnapshot holds game data captured under lock for async tip generation.
type tipSnapshot struct {
	playerName string
	hole       [2]poker.Card
	holeStr    [2]string
	community  []poker.Card
	communityS string
	numActive  int
	numOpp     int
	toCall     int64
	totalPot   int64
	stack      int64
	street     Street
	position   string
	headsUp    bool
	isDealer   bool
	isAllIn    bool // at least one opponent is already all-in
}

// snapshotForTip captures game state under lock. Cheap — no MC here.
func snapshotForTip(g *HoldemGame, playerIdx int) tipSnapshot {
	p := g.Players[playerIdx]

	totalPot := g.Pot
	for _, pp := range g.Players {
		totalPot += pp.Bet
	}

	toCall := g.CurrentBet - p.Bet
	if toCall < 0 {
		toCall = 0
	}

	numActive := g.activeCount()
	numOpp := numActive - 1
	if numOpp < 1 {
		numOpp = 1
	}

	communityS := "—"
	if len(g.Community) > 0 {
		communityS = renderCards(g.Community)
	}

	// Copy community cards so the slice is safe outside the lock.
	comm := make([]poker.Card, len(g.Community))
	copy(comm, g.Community)

	headsUp := numActive == 2
	isDealer := playerIdx == g.DealerIdx

	// Facing an all-in? Any opponent (not hero) who is still in the hand
	// and marked AllIn turns this into a terminal decision.
	isAllIn := false
	for i, pp := range g.Players {
		if i == playerIdx {
			continue
		}
		if pp.State == PlayerAllIn {
			isAllIn = true
			break
		}
	}

	// Compute position with heads-up street awareness.
	position := g.positionLabel(playerIdx)
	if headsUp {
		position = tipPositionLabel(isDealer, g.Street)
	}

	return tipSnapshot{
		playerName: p.DisplayName,
		hole:       p.Hole,
		holeStr:    [2]string{renderCard(p.Hole[0]), renderCard(p.Hole[1])},
		community:  comm,
		communityS: communityS,
		numActive:  numActive,
		numOpp:     numOpp,
		toCall:     toCall,
		totalPot:   totalPot,
		stack:      p.Stack,
		street:     g.Street,
		position:   position,
		headsUp:    headsUp,
		isDealer:   isDealer,
		isAllIn:    isAllIn,
	}
}

// tipPositionLabel returns the correct position label for heads-up play,
// accounting for the fact that position semantics change between preflop and postflop.
// Pre-flop: dealer/SB acts first (out of position for tips), BB acts last.
// Post-flop: dealer/BTN acts last (positional advantage), BB acts first (out of position).
func tipPositionLabel(isDealer bool, street Street) string {
	if street == StreetPreFlop {
		if isDealer {
			return "SB" // dealer is SB in heads-up, acts first preflop
		}
		return "BB"
	}
	// Post-flop: dealer has position
	if isDealer {
		return "BTN" // acts last = positional advantage
	}
	return "BB" // acts first = out of position
}

// buildTipContext computes the full tip context including equity MC.
// Call this OUTSIDE the lock — the expensive equity computation runs here.
func buildTipContext(snap tipSnapshot) holdemTipContext {
	iterations := 5000
	if len(snap.community) > 0 {
		iterations = 10000
	}
	eq := Equity(snap.hole, snap.community, snap.numOpp, iterations)

	var eqShove EquityResult
	if snap.isAllIn {
		eqShove = EquityVsRange(snap.hole, snap.community, facingAllInRangeFor(snap.street), iterations)
	}

	spr := 0.0
	if snap.totalPot > 0 {
		spr = float64(snap.stack) / float64(snap.totalPot)
	}

	potOdds := 0.0
	if snap.toCall > 0 {
		potOdds = float64(snap.toCall) / float64(snap.totalPot+snap.toCall) * 100
	}

	// Compute hand category from current best 5-card hand, enriched with
	// which ranks actually make the hand so the LLM can't hallucinate a
	// different pair / set / kicker.
	handCategory := ""
	if len(snap.community) >= 3 {
		_, raw := handRank(snap.hole, snap.community)
		handCategory = describeMadeHand(raw, snap.hole, snap.community)
	}

	// Compute draw info (meaningful on flop/turn only).
	draw := computeDraws(snap.hole, snap.community)

	holeRanks := [2]int{cardRankIndex(snap.hole[0]), cardRankIndex(snap.hole[1])}
	holeSuited := snap.hole[0].Suit() == snap.hole[1].Suit()
	preflopTier := classifyPreflopHand(holeRanks, holeSuited, snap.headsUp)
	paired, maxSuit, straightRisk, boardHigh := analyzeBoardTexture(snap.community)

	return holdemTipContext{
		PlayerName:        snap.playerName,
		Hole:              snap.holeStr,
		Community:         snap.communityS,
		Equity:            eq,
		SPR:               spr,
		PotOddsPct:        potOdds,
		ToCall:            snap.toCall,
		Pot:               snap.totalPot,
		Stack:             snap.stack,
		Street:            snap.street,
		Position:          snap.position,
		NumActive:         snap.numActive,
		HandCategory:      handCategory,
		Draw:              draw,
		HeadsUp:           snap.headsUp,
		HoleRanks:         holeRanks,
		HoleSuited:        holeSuited,
		PreflopTier:       preflopTier,
		BoardPaired:       paired,
		BoardMaxSuit:      maxSuit,
		BoardStraightRisk: straightRisk,
		BoardHighRank:     boardHigh,
		IsAllIn:           snap.isAllIn,
		EquityVsShove:     eqShove,
	}
}

// classifyPreflopHand buckets a starting hand into one of five tiers.
// Sklansky-lite for full ring; significantly wider for heads-up where
// ranges are ~70% open from BTN and ~55% defend from BB.
func classifyPreflopHand(ranks [2]int, suited, headsUp bool) string {
	hi, lo := ranks[0], ranks[1]
	if lo > hi {
		hi, lo = lo, hi
	}
	pair := hi == lo
	gap := hi - lo

	// Rank indices: 2=0, 3=1, ..., 9=7, T=8, J=9, Q=10, K=11, A=12.

	if headsUp {
		return classifyPreflopHU(hi, lo, pair, gap, suited)
	}

	// --- Full-ring / 6-max tiers ---

	// Pocket pairs
	if pair {
		switch {
		case hi >= 9: // JJ+
			return "premium"
		case hi == 8: // TT
			return "strong"
		default: // 22-99
			return "speculative"
		}
	}

	// AK
	if hi == 12 && lo == 11 {
		return "premium"
	}
	// AQ, AJs, KQs
	if hi == 12 && lo == 10 {
		return "strong"
	}
	if suited && hi == 12 && lo == 9 {
		return "strong"
	}
	if suited && hi == 11 && lo == 10 {
		return "strong"
	}
	// AJo, KQo, KJs, QJs, JTs, ATs
	if hi == 12 && lo == 9 {
		return "playable"
	}
	if suited && hi == 12 && lo == 8 {
		return "playable"
	}
	if hi == 11 && lo == 10 {
		return "playable"
	}
	if suited && hi == 11 && lo >= 8 {
		return "playable"
	}
	if suited && hi == 10 && lo >= 7 {
		return "playable"
	}
	if suited && hi == 9 && lo == 8 {
		return "playable"
	}
	// Suited connectors & one-gappers down to 43s
	if suited && gap <= 2 && lo >= 2 {
		return "speculative"
	}
	// Suited aces (any)
	if suited && hi == 12 {
		return "speculative"
	}
	return "trash"
}

// classifyPreflopHU uses much wider tiers appropriate for heads-up play.
// BTN opens ~70%, BB defends ~55%. Any King, any Ace, any pair, most suited
// hands, and connected hands are all playable.
func classifyPreflopHU(hi, lo int, pair bool, gap int, suited bool) string {
	// Premium: AA-QQ, AKs, AKo
	if pair && hi >= 10 {
		return "premium"
	}
	if hi == 12 && lo == 11 {
		return "premium"
	}

	// Strong: JJ-88, AQ-ATs, KQs, AQo-AJo, KQo
	if pair && hi >= 6 { // 88+
		return "strong"
	}
	if hi == 12 && lo >= 8 { // AT+
		return "strong"
	}
	if hi == 11 && lo == 10 { // KQ
		return "strong"
	}
	if suited && hi == 11 && lo >= 9 { // KJs
		return "strong"
	}

	// Playable: 77-22, any Ax, Kx (x>=5), Q9+, J9+, T8+, suited connectors
	if pair {
		return "playable"
	}
	if hi == 12 { // any Ace
		return "playable"
	}
	if hi == 11 && lo >= 3 { // K5+
		return "playable"
	}
	if hi == 10 && lo >= 7 { // Q9+
		return "playable"
	}
	if hi == 9 && lo >= 7 { // J9+
		return "playable"
	}
	if suited && gap <= 3 { // suited connectors/gappers
		return "playable"
	}

	// Speculative: Kx (any), suited broadway/connectors, T7+, 97+
	if hi == 11 { // any remaining King
		return "speculative"
	}
	if suited && hi >= 8 { // any suited T+ hand
		return "speculative"
	}
	if hi == 8 && lo >= 5 { // T7+
		return "speculative"
	}
	if hi == 7 && lo >= 5 { // 97+
		return "speculative"
	}
	if gap <= 2 { // offsuit connectors/gappers
		return "speculative"
	}

	return "trash"
}

// analyzeBoardTexture returns (paired, maxSuitCount, straightRisk, highRank).
// straightRisk is a 0-4 heuristic: higher means more straight possibilities.
// highRank is -1 when there are no community cards.
func analyzeBoardTexture(community []poker.Card) (bool, int, int, int) {
	if len(community) == 0 {
		return false, 0, 0, -1
	}
	var rankCounts [13]int
	var suitCounts [4]int
	high := -1
	for _, c := range community {
		r := cardRankIndex(c)
		rankCounts[r]++
		if r > high {
			high = r
		}
		suitCounts[cardSuitIndex(c)]++
	}

	paired := false
	for _, n := range rankCounts {
		if n >= 2 {
			paired = true
			break
		}
	}

	maxSuit := 0
	for _, n := range suitCounts {
		if n > maxSuit {
			maxSuit = n
		}
	}

	// Straight risk: count cards within a 5-rank window, take the max.
	// (Ace low also considered via wrap.)
	bestWindow := 0
	for start := 0; start <= 12; start++ {
		count := 0
		for r := start; r < start+5 && r <= 12; r++ {
			if rankCounts[r] > 0 {
				count++
			}
		}
		if count > bestWindow {
			bestWindow = count
		}
	}
	// Ace-low wheel window (A-2-3-4-5)
	wheel := 0
	if rankCounts[12] > 0 {
		wheel++
	}
	for r := 0; r < 4; r++ {
		if rankCounts[r] > 0 {
			wheel++
		}
	}
	if wheel > bestWindow {
		bestWindow = wheel
	}

	return paired, maxSuit, bestWindow, high
}

// cardSuitIndex maps a card suit to 0-3. Uses the string form rather than the
// library's bitfield encoding to stay consistent with cardRankIndex.
func cardSuitIndex(c poker.Card) int {
	s := c.String()
	if len(s) < 2 {
		return 0
	}
	switch s[1] {
	case 'c':
		return 0
	case 'd':
		return 1
	case 'h':
		return 2
	case 's':
		return 3
	}
	return 0
}

// generateTip generates a coaching tip. The rules engine is the source of truth
// for the recommended action and reasoning; an optional LLM step rewrites that
// tip for prose variety without introducing new facts or changing the action.
// If the LLM is unavailable or its rewrite looks wrong, we fall back to the
// rules tip verbatim.
func generateTip(ctx holdemTipContext) string {
	base := generateRulesTip(ctx)

	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")

	if host != "" && model != "" {
		rewritten, err := rewriteTipWithLLM(host, model, ctx, base)
		if err != nil {
			slog.Warn("holdem: LLM tip rewrite failed, using rules tip", "err", err)
		} else if rewritten != "" {
			return "💡 " + rewritten
		}
	}

	return "💡 " + base
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// extractTipFromResponse strips thinking tags from LLM output.
func extractTipFromResponse(raw string) string {
	cleaned := thinkTagRe.ReplaceAllString(raw, "")
	return strings.TrimSpace(cleaned)
}

// buildTipSystemPrompt returns the system prompt for poker tip rewriting.
// The LLM is a stylist, not a coach: it rewrites a tip produced by the rules
// engine and must not change the recommended action or invent facts.
func buildTipSystemPrompt() string {
	return `You are rewriting a Texas Hold'em tip so it reads naturally in a private
message to a single player. You are NOT a coach — the recommended action and
the reasoning are already decided by a rules engine. Your only job is to make
the existing tip read well.

You will receive:
- CONTEXT: the game state (cards, board, equity, pot odds, position, etc.)
- TIP: the rules-engine tip to rewrite

STRICT RULES — do not violate these:
- Keep the same recommended action (fold / check / call / bet / raise). If TIP
  says "call", your rewrite must also say "call", not "raise" or "bet".
- Do NOT add a second action on top of what TIP recommends. If TIP says
  "call", do NOT say "but raising is even better" or "you could also raise".
  The set of actions mentioned in your rewrite must be a subset of the actions
  TIP mentions.
- Do not invent bet sizing. Phrases like "3-4× the pot", "pot-sized bet",
  "half pot", "raise to X" are forbidden unless TIP uses that exact sizing.
- Do not introduce new facts, new numbers, new equity claims, or new board
  reads. Every fact in your rewrite must appear verbatim in TIP or CONTEXT.
- Do not restate or rename the hole cards. If you mention them, copy them
  exactly from CONTEXT. "Pair" does NOT mean pocket pair — the pair can
  come from a hole card matching the board. Do not infer pocket pairs.
- Do not reference actions that are not currently legal. If CONTEXT says
  "Available actions: check / bet", do NOT mention "call", "raise", "fold",
  or "facing a bet" — there is no bet to face. If CONTEXT says "Available
  actions: call / raise / fold", do NOT mention "check" or "free card".
- On the river there are no future streets. Do NOT use language like "later
  streets", "next card", "turn", "see another card", "stay in the pot to see",
  "if they commit later", "later in the hand", or "on future streets" when
  Street is River. The hand ends after this action.
- Output 1 to 3 sentences, no lists, no headings, no meta-commentary, no
  "here is the rewrite", just the rewritten tip text.
- If you are unsure how to rewrite it, output TIP unchanged.`
}

// buildTipUserPrompt builds the structured context block that accompanies the
// rules tip sent to the LLM rewriter. The text here is informational only —
// the rules engine has already decided the action.
func buildTipUserPrompt(ctx holdemTipContext) string {
	var b strings.Builder
	b.WriteString("CONTEXT:\n")
	b.WriteString(fmt.Sprintf("Street: %s\n", ctx.Street.String()))

	if ctx.HandCategory != "" {
		b.WriteString(fmt.Sprintf("Your hand: %s  %s  [%s]\n", ctx.Hole[0], ctx.Hole[1], ctx.HandCategory))
	} else {
		b.WriteString(fmt.Sprintf("Your hand: %s  %s\n", ctx.Hole[0], ctx.Hole[1]))
	}

	b.WriteString(fmt.Sprintf("Board: %s\n", ctx.Community))

	if ctx.Draw.IsDraw {
		b.WriteString(fmt.Sprintf("Draw outs: %d (%s)\n", ctx.Draw.TotalOuts, ctx.Draw.Description))
	}

	b.WriteString(fmt.Sprintf("Equity vs %d opponent(s): Win %.0f%% | Tie %.0f%% | Loss %.0f%%\n",
		ctx.NumActive-1, ctx.Equity.Win*100, ctx.Equity.Tie*100, ctx.Equity.Loss*100))

	if ctx.ToCall > 0 {
		exceeds := "exceeds"
		if ctx.Equity.Win*100 < ctx.PotOddsPct {
			exceeds = "falls short of"
		}
		b.WriteString(fmt.Sprintf("Pot odds to call: %.0f%% — equity %s price\n", ctx.PotOddsPct, exceeds))
		b.WriteString("Available actions: call / raise / fold (facing a bet — do NOT say 'bet' or 'check')\n")
	} else {
		b.WriteString("Free card available — no bet to call\n")
		b.WriteString("Available actions: check / bet (no bet to face — do NOT say 'call' or 'raise')\n")
	}

	headsUp := "no"
	if ctx.HeadsUp {
		headsUp = "yes"
	}
	b.WriteString(fmt.Sprintf("SPR: %.1f | Position: %s | Heads-up: %s | Active players: %d\n",
		ctx.SPR, ctx.Position, headsUp, ctx.NumActive))

	if ctx.IsAllIn {
		b.WriteString("FACING ALL-IN: this is a terminal decision. There are no future streets, no position leverage, and no \"fold later\" option. Do not mention \"the turn\", \"the river\", \"later streets\", \"see another card\", \"position advantage\", or \"act after\".\n")
		shoveEq := (ctx.EquityVsShove.Win + ctx.EquityVsShove.Tie*0.5) * 100
		b.WriteString(fmt.Sprintf("Equity vs a realistic shove range: %.0f%% (use this number, not the vs-random equity above)\n", shoveEq))
	}

	return b.String()
}

// rewriteTipWithLLM asks the LLM to rephrase the rules-engine tip for prose
// variety. The rules tip is the source of truth — if the rewrite diverges
// (empty, action vocabulary changed, etc.) we reject it and the caller falls
// back to the original.
func rewriteTipWithLLM(host, model string, ctx holdemTipContext, base string) (string, error) {
	userMsg := buildTipUserPrompt(ctx) + "\nTIP:\n" + base + "\n"
	req := ollamaChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: buildTipSystemPrompt()},
			{Role: "user", Content: userMsg},
		},
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(host, "/") + "/api/chat"
	resp, err := holdemTipsClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	tip := extractTipFromResponse(ollamaResp.Message.Content)
	if tip == "" {
		return "", fmt.Errorf("empty response")
	}
	if !rewriteKeepsAction(base, tip) {
		return "", fmt.Errorf("rewrite changed action vocabulary")
	}
	if reason := rewriteSemanticProblem(ctx, tip); reason != "" {
		return "", fmt.Errorf("rewrite semantic drift: %s", reason)
	}
	return tip, nil
}

// rewriteSemanticProblem returns a non-empty reason when the rewrite contains
// language that is wrong for the current game state — e.g. future-street talk
// on the river, or referencing a bet-to-face when checking is free. This is a
// cheap, high-signal filter layered on top of rewriteKeepsAction, which only
// validates action verbs.
func rewriteSemanticProblem(ctx holdemTipContext, rewrite string) string {
	r := " " + strings.ToLower(rewrite) + " "

	// Facing an all-in is terminal: no future streets, no position leverage.
	// Reject forward-looking language and position advice that presumes more
	// actions to come.
	if ctx.IsAllIn {
		allInBadPhrases := []string{
			"see the turn", "see the river", "see another card", "see the next",
			"next card", "next street", "later street", "later streets",
			"future street", "future streets", "on later", "on future",
			"fold later", "raise later", "fold on the turn", "fold on the river",
			"position advantage", "act after", "acts after",
			"you can fold", "you can raise", "give you the advantage",
		}
		for _, p := range allInBadPhrases {
			if strings.Contains(r, p) {
				return "rewrite presumes future action but facing all-in: " + p
			}
		}
	}

	// River: no future streets exist. Reject any forward-looking language.
	if ctx.Street == StreetRiver {
		riverBadPhrases := []string{
			"later street", "later streets", "next card", "next street",
			"future street", "future streets", "on later", "on future",
			"see another card", "see the next", "see what comes",
			"stay in the pot to see", "stay in to see", "see if they commit",
			"if they commit later", "later in the hand", "later in the pot",
			"turn card", "river card", // referring to "the turn" / "the river" as upcoming
			"wait for the", "coming street",
		}
		for _, p := range riverBadPhrases {
			if strings.Contains(r, p) {
				return "river rewrite mentions future streets: " + p
			}
		}
	}

	// No bet to face: reject "facing a bet" / "folding after a bet" language,
	// which implies the player is currently under pressure they aren't.
	if ctx.ToCall == 0 {
		noBetBadPhrases := []string{
			"facing a bet", "facing the bet", "facing their bet",
			"folding after a bet", "fold after a bet",
			"call this bet", "call their bet",
			"under pressure to call",
		}
		for _, p := range noBetBadPhrases {
			if strings.Contains(r, p) {
				return "rewrite implies a bet to face but ToCall=0: " + p
			}
		}
	}

	return ""
}

// rewriteKeepsAction checks that the LLM rewrite uses the same action verbs as
// the rules tip. If the rules tip recommends "call" but the rewrite says
// "raise" or "bet" without mentioning "call", we reject the rewrite rather
// than misleading the player.
// actionFamilies groups interchangeable action words. The rewrite must mention
// at least one word from every family the base tip used. "bet" and "raise" are
// separate because recommending a raise when the base said bet (or vice-versa)
// is a real action change.
var actionFamilies = [][]string{
	{"fold"},
	{"check"},
	{"call"},
	{"raise", "3-bet", "three-bet", "re-raise", "reraise"},
	{"bet"},
	{"shove", "jam", "all-in", "allin", "all in"},
}

// rewriteKeepsAction checks that the LLM rewrite is a faithful restatement of
// the base tip's recommendation. Two rules apply:
//
//  1. Primary action preserved: the first action family that appears in the
//     base must also appear in the rewrite. This catches rewrites that
//     silently flip "call" → "fold".
//  2. No new action families: any action family the rewrite mentions must
//     have been present in the base too. This catches rewrites that add a
//     raise/bet recommendation on top of a call, which is the most common
//     way the LLM smuggles in unwanted aggression.
//
// The contains check uses word boundaries so "call this bet" (base) followed
// by a rewrite that drops the noun "bet" is not penalised.
func rewriteKeepsAction(base, rewrite string) bool {
	b := " " + strings.ToLower(base) + " "
	r := " " + strings.ToLower(rewrite) + " "

	containsWord := func(s, word string) bool {
		idx := 0
		for {
			i := strings.Index(s[idx:], word)
			if i < 0 {
				return false
			}
			i += idx
			if !isWordChar(s[i-1]) && !isWordChar(s[i+len(word)]) {
				return true
			}
			idx = i + 1
		}
	}
	familyInText := func(s string, family []string) bool {
		for _, w := range family {
			if containsWord(s, w) {
				return true
			}
		}
		return false
	}

	primaryFamily := -1
	earliest := len(b) + 1
	for i, family := range actionFamilies {
		for _, w := range family {
			idx := strings.Index(b, " "+w+" ")
			if idx >= 0 && idx < earliest {
				earliest = idx
				primaryFamily = i
			}
		}
	}
	if primaryFamily >= 0 {
		if !familyInText(r, actionFamilies[primaryFamily]) {
			return false
		}
	}

	// Subset rule (narrowed to aggressive families): if the rewrite mentions
	// raise/bet/shove and the base does not, reject. This catches the common
	// failure mode where the LLM injects "but raising is even better" on top
	// of a call. We don't apply this to passive families (fold/check/call)
	// because rewrites legitimately say things like "this is a fold, don't
	// call" or "check — don't bet into a wet board".
	aggressiveNames := map[string]bool{"raise": true, "bet": true, "shove": true}
	for _, family := range actionFamilies {
		if !aggressiveNames[family[0]] {
			continue
		}
		if familyInText(r, family) && !familyInText(b, family) {
			return false
		}
	}
	return true
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

var rankNames = [13]string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "Jack", "Queen", "King", "Ace"}
var rankNamesPlural = [13]string{"2s", "3s", "4s", "5s", "6s", "7s", "8s", "9s", "10s", "Jacks", "Queens", "Kings", "Aces"}

// describeMadeHand enriches a poker.RankString label ("Pair", "Two Pair",
// "Three of a Kind", etc.) with the specific ranks that form the hand and
// where those ranks come from (hole vs board). This removes the ambiguity
// that lets the LLM hallucinate "pocket 9s" when the pair is a 9 in hand
// matching a 9 on the board.
func describeMadeHand(category string, hole [2]poker.Card, community []poker.Card) string {
	var holeRanks [2]int
	holeRanks[0] = cardRankIndex(hole[0])
	holeRanks[1] = cardRankIndex(hole[1])

	var boardCounts [13]int
	for _, c := range community {
		boardCounts[cardRankIndex(c)]++
	}
	totalCounts := boardCounts
	totalCounts[holeRanks[0]]++
	totalCounts[holeRanks[1]]++

	holeHas := func(r int) bool { return holeRanks[0] == r || holeRanks[1] == r }
	pocketPair := holeRanks[0] == holeRanks[1]

	// Helper: where the group of a given rank "comes from".
	source := func(r int) string {
		inHole := 0
		if holeRanks[0] == r {
			inHole++
		}
		if holeRanks[1] == r {
			inHole++
		}
		onBoard := boardCounts[r]
		switch {
		case inHole == 2 && onBoard == 0:
			return "pocket pair"
		case inHole == 2 && onBoard >= 1:
			return "set (pocket pair + board)"
		case inHole == 1 && onBoard >= 2:
			return "trips (one in hand)"
		case inHole == 0 && onBoard >= 2:
			return "board"
		case inHole == 1 && onBoard >= 1:
			return "board-paired"
		}
		return ""
	}

	// Highest rank ≥ threshold count, scanning from Ace down.
	highestWithCount := func(counts [13]int, n int) int {
		for r := 12; r >= 0; r-- {
			if counts[r] >= n {
				return r
			}
		}
		return -1
	}

	switch category {
	case "High Card":
		best := holeRanks[0]
		if holeRanks[1] > best {
			best = holeRanks[1]
		}
		return fmt.Sprintf("High Card — best in hand: %s", rankNames[best])

	case "Pair":
		r := highestWithCount(totalCounts, 2)
		if r < 0 {
			return category
		}
		src := source(r)
		// Kicker: best of the top-3 remaining cards across hole+board,
		// excluding the paired rank. Report only if it comes from the hole.
		remaining := make([]int, 0, 5)
		for _, hr := range holeRanks {
			if hr != r {
				remaining = append(remaining, hr)
			}
		}
		for _, c := range community {
			br := cardRankIndex(c)
			if br != r {
				remaining = append(remaining, br)
			}
		}
		// Pick the single top kicker (first of the best 3 is the only one we name).
		best := -1
		for _, v := range remaining {
			if v > best {
				best = v
			}
		}
		holeKicker := best >= 0 && (holeRanks[0] == best || holeRanks[1] == best) && best != r
		if holeKicker && !pocketPair {
			return fmt.Sprintf("Pair of %s (%s, kicker %s)", rankNamesPlural[r], src, rankNames[best])
		}
		return fmt.Sprintf("Pair of %s (%s)", rankNamesPlural[r], src)

	case "Two Pair":
		high := highestWithCount(totalCounts, 2)
		low := -1
		for r := high - 1; r >= 0; r-- {
			if totalCounts[r] >= 2 {
				low = r
				break
			}
		}
		if high < 0 || low < 0 {
			return category
		}
		note := ""
		if !holeHas(high) && !holeHas(low) {
			note = " (both on board — playing the board)"
		} else if holeHas(high) && holeHas(low) {
			note = " (both pairs use hole cards)"
		}
		return fmt.Sprintf("Two Pair — %s and %s%s", rankNamesPlural[high], rankNamesPlural[low], note)

	case "Three of a Kind":
		r := highestWithCount(totalCounts, 3)
		if r < 0 {
			return category
		}
		return fmt.Sprintf("Three of a Kind — %s (%s)", rankNamesPlural[r], source(r))

	case "Straight":
		// Top of the highest run of 5 consecutive ranks. Ace plays high (A-high
		// straight) or low (wheel 5-4-3-2-A, reported as 5-high).
		present := func(r int) bool { return totalCounts[r] >= 1 }
		top := -1
		for r := 12; r >= 4; r-- {
			if present(r) && present(r-1) && present(r-2) && present(r-3) && present(r-4) {
				top = r
				break
			}
		}
		if top < 0 && present(3) && present(2) && present(1) && present(0) && present(12) {
			top = 3 // 5-high wheel
		}
		if top < 0 {
			return category
		}
		return fmt.Sprintf("Straight — high card %s", rankNames[top])

	case "Flush":
		// Find the flush suit (5+ of same suit across hole+board), then report
		// the actual high card of that suit.
		var suitRanks [4][]int
		for i := 0; i < 2; i++ {
			suitRanks[cardSuitIndex(hole[i])] = append(suitRanks[cardSuitIndex(hole[i])], holeRanks[i])
		}
		boardSuit := [4][]int{}
		for _, c := range community {
			s := cardSuitIndex(c)
			suitRanks[s] = append(suitRanks[s], cardRankIndex(c))
			boardSuit[s] = append(boardSuit[s], cardRankIndex(c))
		}
		flushSuit := -1
		for s := 0; s < 4; s++ {
			if len(suitRanks[s]) >= 5 {
				flushSuit = s
				break
			}
		}
		if flushSuit < 0 {
			return category
		}
		top := -1
		for _, r := range suitRanks[flushSuit] {
			if r > top {
				top = r
			}
		}
		note := ""
		holeInFlush := cardSuitIndex(hole[0]) == flushSuit || cardSuitIndex(hole[1]) == flushSuit
		if !holeInFlush {
			note = " (playing the board)"
		}
		return fmt.Sprintf("Flush — %s-high%s", rankNames[top], note)

	case "Full House":
		trips := highestWithCount(totalCounts, 3)
		pair := -1
		for r := 12; r >= 0; r-- {
			if r != trips && totalCounts[r] >= 2 {
				pair = r
				break
			}
		}
		if trips < 0 || pair < 0 {
			return category
		}
		return fmt.Sprintf("Full House — %s full of %s", rankNamesPlural[trips], rankNamesPlural[pair])

	case "Four of a Kind":
		r := highestWithCount(totalCounts, 4)
		if r < 0 {
			return category
		}
		return fmt.Sprintf("Four of a Kind — %s", rankNamesPlural[r])

	case "Straight Flush":
		return "Straight Flush"
	}
	return category
}

// generateRulesTip is the deterministic source of truth for poker tips.
// It returns a complete tip string with a recommended action and reasoning
// tied to the specific equity, hand type, draw, board texture, position and
// stack depth of the current spot.
func generateRulesTip(ctx holdemTipContext) string {
	// Facing an all-in is a terminal decision: no future streets, no position
	// leverage, and "vs random" equity is systematically wrong. Hand off to a
	// dedicated branch and do not append the usual position/multiway notes,
	// which all assume there are more actions to come.
	if ctx.IsAllIn {
		return generateAllInFacingTip(ctx)
	}

	var body string
	if ctx.Street == StreetPreFlop {
		body = generatePreflopTip(ctx)
	} else {
		body = generatePostflopTip(ctx)
	}

	if !ctx.HeadsUp {
		switch ctx.Position {
		case "BTN", "CO":
			body += " You have positional advantage — use it."
		case "UTG":
			body += " Early position — you need a strong range here."
		case "SB", "BB":
			if ctx.Street != StreetPreFlop {
				body += " Out of position — play tighter on later streets."
			}
		}
		if ctx.NumActive >= 4 {
			body += " Multiway pot — hand values shift; drawing hands improve, bluffs lose value."
		}
	} else if ctx.Street != StreetPreFlop {
		if ctx.Position == "BB" || ctx.Position == "SB" {
			body += " Out of position heads-up — play cautiously."
		}
	}

	return body
}

// generateAllInFacingTip is the terminal-decision branch. We compare hero's
// equity against a realistic shove range (not random) to the pot odds and
// recommend call/fold with a generous margin of safety. No mention of future
// streets, position, or "later".
func generateAllInFacingTip(ctx holdemTipContext) string {
	heroEq := (ctx.EquityVsShove.Win + ctx.EquityVsShove.Tie*0.5) * 100
	potOdds := ctx.PotOddsPct
	hand := ctx.HandCategory
	if hand == "" {
		hand = "your hand"
	}

	switch {
	case heroEq >= potOdds+8:
		return fmt.Sprintf(
			"Facing an all-in — terminal decision. Vs a realistic shove range, %s has about %.0f%% equity against %.0f%% pot odds, so calling is clearly +EV. The hand ends on this call.",
			hand, heroEq, potOdds)

	case heroEq >= potOdds-2:
		return fmt.Sprintf(
			"Facing an all-in — terminal decision. Vs a realistic shove range, %s has about %.0f%% equity against %.0f%% pot odds. This is borderline; if you think their range is tighter than a standard stackoff range, fold.",
			hand, heroEq, potOdds)

	default:
		return fmt.Sprintf(
			"Facing an all-in — fold. Vs a realistic shove range, %s only has about %.0f%% equity while you need %.0f%%. The 'vs random' equity number is misleading here because nobody shoves random cards. The hand ends on this decision.",
			hand, heroEq, potOdds)
	}
}

func generatePreflopTip(ctx holdemTipContext) string {
	if ctx.HeadsUp {
		return generatePreflopTipHU(ctx)
	}
	return generatePreflopTipRing(ctx)
}

func generatePreflopTipHU(ctx holdemTipContext) string {
	facing := ctx.ToCall > 0
	isBTN := ctx.Position == "BTN" || ctx.Position == "SB"
	deep := ctx.SPR > 10
	bigRaise := facing && ctx.ToCall > 0 && float64(ctx.ToCall)/float64(ctx.Pot) > 0.6

	switch ctx.PreflopTier {
	case "premium":
		if facing {
			return "Premium hand heads-up — raise or re-raise. You want to build the pot with this hand against any opponent range."
		}
		return "Premium hand heads-up — raise for value. Build the pot now."

	case "strong":
		if facing {
			return "Strong hand heads-up — raise. This hand plays well in a bigger pot against one opponent."
		}
		return "Strong hand heads-up — raise to put pressure on the BB."

	case "playable":
		if facing && bigRaise && !deep {
			return "Playable heads-up hand, but the raise is too large relative to your stack — fold. You need deeper stacks to speculate with this."
		}
		if facing && isBTN {
			return "Solid heads-up hand — complete or raise. You have position postflop, which makes this profitable."
		}
		if facing {
			return "Decent heads-up hand — call to see a flop. You're out of position, so keep the pot manageable."
		}
		return "Decent heads-up hand — raise to take the initiative."

	case "speculative":
		if facing && !deep {
			return "Speculative hand without deep stacks to support it heads-up — fold. You need implied odds to play these."
		}
		if facing && isBTN {
			return "Marginal heads-up hand — completing is fine with position. You'll have the advantage of acting last on every street."
		}
		if facing {
			return "Marginal heads-up hand — calling is borderline. Being out of position makes it harder to realize your equity."
		}
		return "Marginal heads-up hand — a small raise can take it down, but don't overcommit."

	case "trash":
		if facing && isBTN {
			return "Weak hand, but heads-up from the button you can consider completing cheaply — position makes many hands playable."
		}
		if facing {
			return "Weak hand out of position heads-up — fold. Without position advantage, this hand won't be profitable."
		}
		return "Weak hand — check and see a free flop if possible."
	}

	return fmt.Sprintf("Preflop %.0f%% equity heads-up — evaluate before committing.", (ctx.Equity.Win+ctx.Equity.Tie*0.5)*100)
}

func generatePreflopTipRing(ctx holdemTipContext) string {
	facing := ctx.ToCall > 0
	deep := ctx.SPR > 10

	switch ctx.PreflopTier {
	case "premium":
		if facing {
			return "Premium hand — raise for value. Build the pot now; you want to be all-in by the river against most opponents."
		}
		return "Premium hand — open for a raise. Don't limp these; build the pot and charge weaker hands to see a flop."

	case "strong":
		if facing {
			return "Strong hand — call or 3-bet depending on opener's range. Don't fold pre, but avoid getting it all-in against tight ranges."
		}
		return "Strong hand — raise to open. Good equity against most hands that call you."

	case "playable":
		if facing {
			return "Playable but not strong enough to 3-bet — call if the price is right and you're in position, fold if you're out of position against tight opens."
		}
		return "Playable from late position — raise or fold to take control; don't limp."

	case "speculative":
		if facing && !deep {
			return "Speculative hand without deep stacks to support it — fold. You need implied odds to play these."
		}
		if facing && deep {
			return "Speculative hand with deep stacks — call for set-mining or flopping a big draw. Fold the flop if you miss."
		}
		return "Speculative hand — limp or raise small from late position; aim to see a cheap flop."

	case "trash":
		if facing {
			return "Weak starting hand — fold. Don't defend against a raise with this."
		}
		return "Weak starting hand — check if you can, otherwise fold. Not worth playing from this position."
	}

	return fmt.Sprintf("Preflop %.0f%% equity — evaluate position and stack depth before committing.", (ctx.Equity.Win+ctx.Equity.Tie*0.5)*100)
}

func generatePostflopTip(ctx holdemTipContext) string {
	equity := ctx.Equity.Win + ctx.Equity.Tie*0.5
	equityPct := equity * 100
	facing := ctx.ToCall > 0
	wetBoard := ctx.BoardMaxSuit >= 2 || ctx.BoardStraightRisk >= 3
	monotoneFlop := ctx.BoardMaxSuit >= 3
	pairedBoard := ctx.BoardPaired
	lowSPR := ctx.SPR < 1.5
	deepSPR := ctx.SPR > 6
	multiway := ctx.NumActive >= 3

	// Equity thresholds shift with opponent count. "vs random" equity of 55%
	// against 3 opponents is much stronger than 55% against 1 — surviving
	// against multiple random ranges means you likely have a real hand.
	// Lower the thresholds so we don't undervalue multiway equity.
	monsterThresh := 0.85
	strongThresh := 0.70
	decentThresh := 0.55
	marginalThresh := 0.40
	if multiway {
		monsterThresh = 0.70
		strongThresh = 0.55
		decentThresh = 0.40
		marginalThresh = 0.28
	}

	// --- Monster / nuts range ---------------------------------------------
	if equity > monsterThresh {
		switch {
		case lowSPR:
			return fmt.Sprintf("Monster hand (%.0f%% equity) with a shallow stack — get it in. Shove or call any bet; you're way ahead.", equityPct)
		case multiway && facing:
			return fmt.Sprintf("Monster hand (%.0f%% equity) multiway — raise for value. Multiple opponents means a bigger pot when you're this far ahead.", equityPct)
		case multiway:
			return fmt.Sprintf("Monster hand (%.0f%% equity) multiway — bet for value. Size up (3/4 pot or more) since someone is likely to call with this many players.", equityPct)
		case facing && wetBoard:
			return fmt.Sprintf("Monster hand (%.0f%% equity) but the board is wet — raise for value and to deny equity to draws.", equityPct)
		case facing:
			return fmt.Sprintf("Monster hand (%.0f%% equity) — raise for value, or slow-play only if you're confident villain will keep firing.", equityPct)
		case wetBoard:
			return fmt.Sprintf("Monster hand (%.0f%% equity) on a wet board — bet big (3/4 pot or more) to charge draws and protect your equity.", equityPct)
		default:
			return fmt.Sprintf("Monster hand (%.0f%% equity) on a dry board — bet for thin value. A smaller size (1/3 to 1/2 pot) keeps weaker hands in.", equityPct)
		}
	}

	// --- Strong made hand --------------------------------------------------
	if equity > strongThresh {
		switch {
		case pairedBoard && ctx.HandCategory != "" && strings.HasPrefix(ctx.HandCategory, "Full House"):
			return fmt.Sprintf("Strong hand (%.0f%% equity) — %s. Bet for value; only be cautious if a higher full house is possible.", equityPct, ctx.HandCategory)
		case pairedBoard:
			return fmt.Sprintf("Strong hand (%.0f%% equity) but the board is paired — bet for value with caution. Slow down if villain raises big.", equityPct)
		case multiway && facing:
			return fmt.Sprintf("Strong hand (%.0f%% equity) multiway — call. Raising bloats the pot against multiple opponents who could have you beat. Re-evaluate on the next street.", equityPct)
		case multiway:
			return fmt.Sprintf("Strong hand (%.0f%% equity) multiway — bet for value but size conservatively. Multiple opponents increase the chance someone has a better hand.", equityPct)
		case facing && monotoneFlop:
			return fmt.Sprintf("Strong hand (%.0f%% equity) on a monotone board — call and see how villain reacts. Don't bloat the pot without a card in the flush suit.", equityPct)
		case facing:
			return fmt.Sprintf("Strong hand (%.0f%% equity) — raise for value. You're ahead of most hands that will continue.", equityPct)
		default:
			return fmt.Sprintf("Strong hand (%.0f%% equity) — bet 2/3 pot for value. Get paid by weaker hands and deny equity to draws.", equityPct)
		}
	}

	// --- Decent made hand (ahead but vulnerable) ---------------------------
	if equity > decentThresh {
		switch {
		case multiway && facing:
			if equity*100 > ctx.PotOddsPct {
				return fmt.Sprintf("Decent hand (%.0f%% equity) multiway — call. You're priced in, but don't raise into multiple opponents with a vulnerable hand.", equityPct)
			}
			return fmt.Sprintf("Decent hand (%.0f%% equity) multiway vs %.0f%% pot odds — fold. Against multiple opponents who've shown interest, you're likely behind.", equityPct, ctx.PotOddsPct)
		case multiway:
			return fmt.Sprintf("Decent hand (%.0f%% equity) multiway — check to control the pot. Betting into multiple opponents with a medium-strength hand is risky.", equityPct)
		case facing && equity*100 > ctx.PotOddsPct && wetBoard:
			return fmt.Sprintf("Decent hand (%.0f%% equity) on a wet board — call and reassess. Don't raise and bloat the pot out of position.", equityPct)
		case facing && equity*100 > ctx.PotOddsPct:
			return fmt.Sprintf("Decent hand (%.0f%% equity) — call. You're priced in and ahead of villain's continuing range more often than not.", equityPct)
		case facing:
			return fmt.Sprintf("Decent hand but the price is wrong (%.0f%% equity vs %.0f%% pot odds) — fold unless you have a read that villain is bluffing.", equityPct, ctx.PotOddsPct)
		case wetBoard:
			return fmt.Sprintf("Decent hand (%.0f%% equity) on a wet board — bet small for protection, or check to pot-control if the turn could get ugly.", equityPct)
		default:
			return fmt.Sprintf("Decent hand (%.0f%% equity) — bet small for thin value. Be ready to slow down if villain raises.", equityPct)
		}
	}

	// --- Drawing hand (check before marginal — draws with low "vs random"
	// equity in multiway pots are still worth playing for implied odds) ------
	if ctx.Draw.IsDraw {
		switch {
		case !facing && multiway:
			return fmt.Sprintf("You have a %s. Check for a free card — if you hit, the multiway pot gives you great implied odds.", ctx.Draw.Description)
		case !facing:
			return fmt.Sprintf("You have a %s. With a free card available, check and see the next card without risk.", ctx.Draw.Description)
		case equity*100 > ctx.PotOddsPct:
			return fmt.Sprintf("Drawing hand (%s) with %.0f%% equity vs %.0f%% pot odds — the price is right to call.", ctx.Draw.Description, equityPct, ctx.PotOddsPct)
		case multiway && deepSPR && ctx.Draw.TotalOuts >= 8:
			return fmt.Sprintf("Drawing hand (%s) — pot odds are short but the multiway pot gives you excellent implied odds when you hit. Call.", ctx.Draw.Description)
		case deepSPR && ctx.Draw.TotalOuts >= 8:
			return fmt.Sprintf("Drawing hand (%s) — pot odds don't quite justify the call, but deep stacks give you implied odds. Call if you'll get paid when you hit.", ctx.Draw.Description)
		default:
			return fmt.Sprintf("Drawing hand (%s) with %.0f%% equity vs %.0f%% pot odds — fold. The price is wrong and the implied odds aren't there.", ctx.Draw.Description, equityPct, ctx.PotOddsPct)
		}
	}

	// --- Marginal / bluffcatcher -------------------------------------------
	if equity > marginalThresh {
		switch {
		case multiway && facing:
			return fmt.Sprintf("Marginal hand (%.0f%% equity) multiway — fold. With multiple opponents still in, someone likely has you beat.", equityPct)
		case multiway:
			return fmt.Sprintf("Marginal hand (%.0f%% equity) multiway — check. Don't bluff or bet for thin value into multiple opponents.", equityPct)
		case facing && equity*100 > ctx.PotOddsPct:
			return fmt.Sprintf("Marginal hand (%.0f%% equity) — you're barely priced in. Call this bet but don't build the pot further.", equityPct)
		case facing:
			return fmt.Sprintf("Marginal hand (%.0f%% equity) vs %.0f%% pot odds — fold. This is the kind of spot where hero-calling bleeds your stack.", equityPct, ctx.PotOddsPct)
		default:
			return fmt.Sprintf("Marginal hand (%.0f%% equity) — check to control the pot. Don't turn your hand into a bluff with weak showdown value.", equityPct)
		}
	}

	// --- Weak / air --------------------------------------------------------
	if facing {
		return fmt.Sprintf("Weak hand (%.0f%% equity) — fold. You don't have the equity or the draw to continue.", equityPct)
	}
	if ctx.HeadsUp {
		return fmt.Sprintf("Weak hand (%.0f%% equity) — check. You could bluff here, but only if the board favors your perceived range.", equityPct)
	}
	return fmt.Sprintf("Weak hand (%.0f%% equity) — check and give up. Don't bluff into multiple opponents or a board you don't represent.", equityPct)
}

// extractTipAction pulls the recommended action verb from a rules tip for audit logging.
func extractTipAction(tip string) string {
	lower := strings.ToLower(tip)
	for _, action := range []string{"fold", "raise", "bet", "call", "check", "shove"} {
		if strings.Contains(lower, action) {
			return action
		}
	}
	return "unknown"
}

// logTipAudit records a tip with full context to the audit table.
func logTipAudit(userID string, ctx holdemTipContext, tip string) {
	eqPct := (ctx.Equity.Win + ctx.Equity.Tie*0.5) * 100
	drawDesc := ""
	if ctx.Draw.IsDraw {
		drawDesc = ctx.Draw.Description
	}
	db.RecordTipAudit(
		userID,
		ctx.Hole[0]+" "+ctx.Hole[1],
		ctx.Community,
		ctx.Street.String(),
		ctx.Position,
		ctx.NumActive,
		eqPct,
		ctx.Pot,
		ctx.ToCall,
		ctx.Stack,
		ctx.SPR,
		ctx.HandCategory,
		drawDesc,
		ctx.PreflopTier,
		extractTipAction(tip),
		tip,
	)
}
