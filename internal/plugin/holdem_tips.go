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

	spr := 0.0
	if snap.totalPot > 0 {
		spr = float64(snap.stack) / float64(snap.totalPot)
	}

	potOdds := 0.0
	if snap.toCall > 0 {
		potOdds = float64(snap.toCall) / float64(snap.totalPot+snap.toCall) * 100
	}

	// Compute hand category from current best 5-card hand.
	handCategory := ""
	if len(snap.community) >= 3 {
		_, handCategory = handRank(snap.hole, snap.community)
	}

	// Compute draw info (meaningful on flop/turn only).
	draw := computeDraws(snap.hole, snap.community)

	return holdemTipContext{
		PlayerName:   snap.playerName,
		Hole:         snap.holeStr,
		Community:    snap.communityS,
		Equity:       eq,
		SPR:          spr,
		PotOddsPct:   potOdds,
		ToCall:       snap.toCall,
		Pot:          snap.totalPot,
		Stack:        snap.stack,
		Street:       snap.street,
		Position:     snap.position,
		NumActive:    snap.numActive,
		HandCategory: handCategory,
		Draw:         draw,
		HeadsUp:      snap.headsUp,
	}
}

// generateTip generates a coaching tip, trying LLM first then falling back to rules.
func generateTip(ctx holdemTipContext) string {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")

	if host != "" && model != "" {
		tip, err := generateLLMTip(host, model, ctx)
		if err != nil {
			slog.Warn("holdem: LLM tip failed, using fallback", "err", err)
		} else if tip != "" {
			return "💡 " + tip
		}
	}

	return "💡 " + generateRulesTip(ctx)
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

// buildTipSystemPrompt returns the system prompt for poker tip generation.
func buildTipSystemPrompt() string {
	return `You are a Texas Hold'em coach giving advice to a single player via private message.
You will receive structured game context. Reason through it in this order:

1. What type of hand do I have — made hand, drawing hand, or air?
2. If drawing: how many outs, and do pot odds justify continuing?
3. If made hand: is it strong enough to bet for value, or weak enough to just pot control?
4. Does position affect what I should do here?
5. Is a free card available, and if so, is taking it correct?

Then write ONE piece of advice — 2 to 3 sentences maximum — that tells the player
what to do and why, using the specific cards and numbers provided.
Do not list concepts. Do not use generic poker vocabulary without connecting it to
this specific hand. If the correct play is obvious (e.g. free card with a draw),
say so plainly and briefly.`
}

// buildTipUserPrompt builds the structured user prompt with hand context.
func buildTipUserPrompt(ctx holdemTipContext) string {
	var b strings.Builder
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
	} else {
		b.WriteString("Free card available — no bet to call\n")
	}

	headsUp := "no"
	if ctx.HeadsUp {
		headsUp = "yes"
	}
	b.WriteString(fmt.Sprintf("SPR: %.1f | Position: %s | Heads-up: %s | Active players: %d\n",
		ctx.SPR, ctx.Position, headsUp, ctx.NumActive))

	return b.String()
}

func generateLLMTip(host, model string, ctx holdemTipContext) (string, error) {
	req := ollamaChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: buildTipSystemPrompt()},
			{Role: "user", Content: buildTipUserPrompt(ctx)},
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

	return tip, nil
}

func generateRulesTip(ctx holdemTipContext) string {
	equity := ctx.Equity.Win + ctx.Equity.Tie*0.5

	var tip string

	switch {
	case equity > 0.80:
		tip = fmt.Sprintf("Strong hand (%.0f%% equity). Size your bet for value — you want to get paid off.", equity*100)

	case ctx.Draw.IsDraw && ctx.ToCall == 0:
		tip = fmt.Sprintf("You have a %s. With a free card available, check and see the next card without risk.", ctx.Draw.Description)

	case ctx.Draw.IsDraw && ctx.ToCall > 0 && equity*100 > ctx.PotOddsPct:
		tip = fmt.Sprintf("Drawing hand (%s) with %.0f%% equity vs %.0f%% pot odds — the price is right to call.", ctx.Draw.Description, equity*100, ctx.PotOddsPct)

	case ctx.Draw.IsDraw && ctx.ToCall > 0 && equity*100 <= ctx.PotOddsPct:
		tip = fmt.Sprintf("Drawing hand (%s) but equity %.0f%% falls short of pot odds %.0f%% — consider folding unless implied odds justify calling.", ctx.Draw.Description, equity*100, ctx.PotOddsPct)

	case ctx.ToCall > 0 && equity*100 > ctx.PotOddsPct:
		tip = fmt.Sprintf("Equity %.0f%% exceeds pot odds %.0f%% — calling is +EV here.", equity*100, ctx.PotOddsPct)

	case ctx.ToCall > 0 && equity*100 <= ctx.PotOddsPct:
		tip = fmt.Sprintf("Equity %.0f%% falls short of pot odds %.0f%% — consider folding.", equity*100, ctx.PotOddsPct)

	case ctx.ToCall == 0 && equity > 0.65:
		tip = fmt.Sprintf("%.0f%% equity with check available — bet for value and deny free cards to draws.", equity*100)

	case ctx.ToCall == 0 && equity < 0.40:
		tip = fmt.Sprintf("%.0f%% equity — check to control pot size.", equity*100)

	case ctx.SPR < 1:
		tip = "Shallow stack (SPR < 1) — commit or fold. No room to maneuver."

	case ctx.SPR > 10 && ctx.Street == StreetPreFlop:
		tip = "Deep stacked preflop — implied odds outweigh raw equity. Speculative hands gain value."

	default:
		tip = fmt.Sprintf("%.0f%% equity. Evaluate your position and pot odds before acting.", equity*100)
	}

	// Position note.
	switch ctx.Position {
	case "BTN", "CO":
		tip += " You have positional advantage — use it."
	case "SB", "BB":
		tip += " Out of position — play tighter."
	case "UTG":
		tip += " Early position — you need a strong range here."
	}

	if ctx.NumActive >= 4 {
		tip += " Multiway pot — hand values shift; drawing hands improve, bluffs lose value."
	}

	return tip
}
