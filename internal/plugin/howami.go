package plugin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// HowAmIPlugin generates LLM-powered "roast" profiles of users.
type HowAmIPlugin struct {
	Base
}

// NewHowAmIPlugin creates a new howami plugin.
func NewHowAmIPlugin(client *mautrix.Client) *HowAmIPlugin {
	return &HowAmIPlugin{
		Base: NewBase(client),
	}
}

func (p *HowAmIPlugin) Name() string { return "howami" }

func (p *HowAmIPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "howami", Description: "Get an LLM-generated roast profile", Usage: "!howami [@user]", Category: "LLM & Sentiment"},
	}
}

func (p *HowAmIPlugin) Init() error { return nil }

func (p *HowAmIPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *HowAmIPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "howami") {
		return nil
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "LLM is not configured.")
	}

	target := ctx.Sender
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "howami"))
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	if err := p.SendReply(ctx.RoomID, ctx.EventID, "Gathering your profile data..."); err != nil {
		slog.Error("howami: send thinking", "err", err)
	}

	profile := p.gatherProfile(target)

	botName := os.Getenv("BOT_DISPLAY_NAME")
	if botName == "" {
		botName = "GogoBee"
	}
	prompt := fmt.Sprintf(
		`You are a witty, playful community bot called %s. Based on the following user profile data, write a fun, lighthearted "roast" of this user in 3-5 sentences. Be creative, funny, and reference specific stats. Keep it friendly — no truly mean insults.

User: %s

Profile data:
%s

Write the roast now. Do not include any preamble or explanation, just the roast text.`,
		botName, string(target), profile,
	)

	response, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("howami: ollama call", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to generate profile. LLM might be offline.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, response)
}

func (p *HowAmIPlugin) gatherProfile(userID id.UserID) string {
	d := db.Get()
	uid := string(userID)
	var sb strings.Builder

	// XP and level
	var xp, level int
	if err := d.QueryRow(`SELECT xp, level FROM users WHERE user_id = ?`, uid).Scan(&xp, &level); err == nil {
		sb.WriteString(fmt.Sprintf("XP: %d, Level: %d\n", xp, level))
	}

	// Message stats
	var totalMsgs, totalWords, totalEmojis, totalQuestions, totalLinks int
	if err := d.QueryRow(
		`SELECT total_messages, total_words, total_emojis, total_questions, total_links
		 FROM user_stats WHERE user_id = ?`, uid,
	).Scan(&totalMsgs, &totalWords, &totalEmojis, &totalQuestions, &totalLinks); err == nil {
		sb.WriteString(fmt.Sprintf("Messages: %d, Words: %d, Emojis: %d, Questions: %d, Links: %d\n",
			totalMsgs, totalWords, totalEmojis, totalQuestions, totalLinks))
	}

	// Achievements
	var achievementCount int
	_ = d.QueryRow(`SELECT COUNT(*) FROM achievements WHERE user_id = ?`, uid).Scan(&achievementCount)
	sb.WriteString(fmt.Sprintf("Achievements unlocked: %d\n", achievementCount))

	// Sentiment stats
	var positive, negative, neutral, excited, sarcastic, frustrated, curious, grateful, humorous, supportive int
	if err := d.QueryRow(
		`SELECT positive, negative, neutral, excited, sarcastic, frustrated, curious, grateful, humorous, supportive
		 FROM sentiment_stats WHERE user_id = ?`, uid,
	).Scan(&positive, &negative, &neutral, &excited, &sarcastic, &frustrated, &curious, &grateful, &humorous, &supportive); err == nil {
		sb.WriteString(fmt.Sprintf("Sentiment: %d positive, %d excited, %d supportive, %d grateful, %d humorous, %d curious, %d neutral, %d sarcastic, %d frustrated, %d negative\n",
			positive, excited, supportive, grateful, humorous, curious, neutral, sarcastic, frustrated, negative))
	}

	// Profanity count
	var profanityCount int
	if err := d.QueryRow(`SELECT count FROM potty_mouth WHERE user_id = ?`, uid).Scan(&profanityCount); err == nil {
		sb.WriteString(fmt.Sprintf("Profanity count: %d\n", profanityCount))
	}

	// Insult stats
	var timesInsulted, timesInsulting int
	if err := d.QueryRow(
		`SELECT times_insulted, times_insulting FROM insult_log WHERE user_id = ?`, uid,
	).Scan(&timesInsulted, &timesInsulting); err == nil {
		sb.WriteString(fmt.Sprintf("Times insulted: %d, Times insulting: %d\n", timesInsulted, timesInsulting))
	}

	// Trivia scores
	var correct, wrong, totalScore int
	var fastestMs sql.NullInt64
	if err := d.QueryRow(
		`SELECT COALESCE(SUM(correct),0), COALESCE(SUM(wrong),0), COALESCE(SUM(total_score),0), MIN(fastest_ms)
		 FROM trivia_scores WHERE user_id = ?`, uid,
	).Scan(&correct, &wrong, &totalScore, &fastestMs); err == nil {
		sb.WriteString(fmt.Sprintf("Trivia: %d correct, %d wrong, score %d", correct, wrong, totalScore))
		if fastestMs.Valid && fastestMs.Int64 > 0 {
			sb.WriteString(fmt.Sprintf(", fastest: %dms", fastestMs.Int64))
		}
		sb.WriteString("\n")
	}

	// WOTD usage
	var wotdTotal int
	_ = d.QueryRow(`SELECT COALESCE(SUM(count),0) FROM wotd_usage WHERE user_id = ?`, uid).Scan(&wotdTotal)
	sb.WriteString(fmt.Sprintf("Word of the Day uses: %d\n", wotdTotal))

	// Reputation
	var repXP int
	_ = d.QueryRow(
		`SELECT COALESCE(SUM(amount),0) FROM xp_log WHERE user_id = ? AND reason = 'reputation'`, uid,
	).Scan(&repXP)
	sb.WriteString(fmt.Sprintf("Reputation XP: %d (rep points: %d)\n", repXP, repXP/5))

	// Streaks
	var activeDays int
	_ = d.QueryRow(`SELECT COUNT(*) FROM daily_activity WHERE user_id = ?`, uid).Scan(&activeDays)
	sb.WriteString(fmt.Sprintf("Active days: %d\n", activeDays))

	return sb.String()
}

// callOllama sends a prompt to the Ollama generate endpoint and returns the response.
func callOllama(host, model, prompt string) (string, error) {
	apiURL := strings.TrimRight(host, "/") + "/api/generate"

	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return strings.TrimSpace(result.Response), nil
}
