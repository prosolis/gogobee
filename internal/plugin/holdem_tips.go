package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

var holdemTipsClient = &http.Client{Timeout: 15 * time.Second}

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
	d := db.Get()
	val := 0
	if enabled {
		val = 1
	}
	_, _ = d.Exec(
		`INSERT INTO holdem_tips_prefs (user_id, enabled) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET enabled = ?, updated_at = CURRENT_TIMESTAMP`,
		string(userID), val, val,
	)
}

// holdemTipContext holds the data needed to generate a tip.
type holdemTipContext struct {
	PlayerName  string
	Hole        [2]string // rendered card strings
	Community   string    // rendered board string
	Equity      EquityResult
	SPR         float64
	PotOddsPct  float64
	ToCall      int64
	Pot         int64
	Stack       int64
	Street      Street
	Position    string
	NumActive   int
}

// buildTipContext creates a tip context from the current game state.
func buildTipContext(g *HoldemGame, playerIdx int) holdemTipContext {
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

	iterations := 5000
	if len(g.Community) > 0 {
		iterations = 10000
	}
	eq := Equity(p.Hole, g.Community, numOpp, iterations)

	spr := 0.0
	if totalPot > 0 {
		spr = float64(p.Stack) / float64(totalPot)
	}

	potOdds := 0.0
	if toCall > 0 {
		potOdds = float64(toCall) / float64(totalPot+toCall) * 100
	}

	community := "—"
	if len(g.Community) > 0 {
		community = renderCards(g.Community)
	}

	return holdemTipContext{
		PlayerName: p.DisplayName,
		Hole:       [2]string{renderCard(p.Hole[0]), renderCard(p.Hole[1])},
		Community:  community,
		Equity:     eq,
		SPR:        spr,
		PotOddsPct: potOdds,
		ToCall:     toCall,
		Pot:        totalPot,
		Stack:      p.Stack,
		Street:     g.Street,
		Position:   g.positionLabel(playerIdx),
		NumActive:  numActive,
	}
}

// generateTip generates a coaching tip, trying LLM first then falling back to rules.
func generateTip(ctx holdemTipContext) string {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")

	if host != "" && model != "" {
		tip, err := generateLLMTip(host, model, ctx)
		if err != nil {
			slog.Debug("holdem: LLM tip failed, using fallback", "err", err)
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

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func generateLLMTip(host, model string, ctx holdemTipContext) (string, error) {
	systemPrompt := `You are a concise Texas Hold'em coach embedded in a Matrix chat bot.
You will be given structured game context including pre-computed equity.
Give exactly 2-4 sentences of actionable advice for the player's current decision.
Lead with the equity vs pot odds relationship when a bet is to call.
Be direct. No preamble. No praise. No "great hand" filler.`

	var userPrompt strings.Builder
	userPrompt.WriteString(fmt.Sprintf("Street: %s\n", ctx.Street.String()))
	userPrompt.WriteString(fmt.Sprintf("Your hand: %s  %s\n", ctx.Hole[0], ctx.Hole[1]))
	userPrompt.WriteString(fmt.Sprintf("Board: %s\n", ctx.Community))
	userPrompt.WriteString(fmt.Sprintf("Equity vs %d opponents: Win %.0f%% | Tie %.0f%% | Loss %.0f%%\n",
		ctx.NumActive-1, ctx.Equity.Win*100, ctx.Equity.Tie*100, ctx.Equity.Loss*100))

	if ctx.ToCall > 0 {
		exceeds := "exceeds"
		if ctx.Equity.Win*100 < ctx.PotOddsPct {
			exceeds = "falls short of"
		}
		userPrompt.WriteString(fmt.Sprintf("Pot odds to call: %.0f%% — equity %s price\n", ctx.PotOddsPct, exceeds))
	} else {
		userPrompt.WriteString("Check available — no bet to call\n")
	}

	userPrompt.WriteString(fmt.Sprintf("SPR: %.1f | Position: %s | Active players: %d\n",
		ctx.SPR, ctx.Position, ctx.NumActive))
	userPrompt.WriteString("\nWhat should I consider for my decision?")

	req := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt.String()},
		},
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(host, "/") + "/v1/chat/completions"
	resp, err := holdemTipsClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response")
	}

	tip := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip thinking tags if present.
	if i := strings.Index(tip, "<think>"); i != -1 {
		if j := strings.Index(tip, "</think>"); j != -1 {
			tip = strings.TrimSpace(tip[:i] + tip[j+len("</think>"):])
		}
	}

	return tip, nil
}

func generateRulesTip(ctx holdemTipContext) string {
	equity := ctx.Equity.Win + ctx.Equity.Tie*0.5

	var tip string

	switch {
	case equity > 0.80:
		tip = fmt.Sprintf("Strong hand (%.0f%% equity). Size your bet for value — you want to get paid off.", equity*100)
	case ctx.ToCall > 0 && equity*100 > ctx.PotOddsPct:
		tip = fmt.Sprintf("Equity %.0f%% exceeds pot odds %.0f%% — calling is +EV here.", equity*100, ctx.PotOddsPct)
	case ctx.ToCall > 0 && equity*100 <= ctx.PotOddsPct:
		tip = fmt.Sprintf("Equity %.0f%% falls short of pot odds %.0f%% — consider folding unless you have a strong draw.", equity*100, ctx.PotOddsPct)
	case ctx.ToCall == 0 && equity > 0.65:
		tip = fmt.Sprintf("%.0f%% equity with check available — bet for value and deny free cards to draws.", equity*100)
	case ctx.ToCall == 0 && equity < 0.40:
		tip = fmt.Sprintf("%.0f%% equity — check to control pot size. Not enough equity to bet.", equity*100)
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
