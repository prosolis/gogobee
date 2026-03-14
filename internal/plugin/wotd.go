package plugin

import (
	"bytes"
	"context"
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

// wordnikWOTDResponse is the top-level Wordnik Word of the Day response.
type wordnikWOTDResponse struct {
	Word        string            `json:"word"`
	Definitions []wordnikWOTDDef  `json:"definitions"`
	Examples    []wordnikWOTDEx   `json:"examples"`
}

type wordnikWOTDDef struct {
	Text         string `json:"text"`
	PartOfSpeech string `json:"partOfSpeech"`
}

type wordnikWOTDEx struct {
	Text string `json:"text"`
}

// WOTDPlugin provides a Word of the Day feature using the Wordnik API.
type WOTDPlugin struct {
	Base
	apiKey     string
	httpClient *http.Client
}

// NewWOTDPlugin creates a new WOTDPlugin.
func NewWOTDPlugin(client *mautrix.Client) *WOTDPlugin {
	return &WOTDPlugin{
		Base:   NewBase(client),
		apiKey: os.Getenv("WORDNIK_API_KEY"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *WOTDPlugin) Name() string { return "wotd" }

func (p *WOTDPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "wotd", Description: "Show today's Word of the Day", Usage: "!wotd", Category: "Lookup & Reference"},
	}
}

func (p *WOTDPlugin) Init() error { return nil }

func (p *WOTDPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *WOTDPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "wotd") {
		return p.handleWOTD(ctx)
	}

	// Passive: track WOTD usage in messages
	if !ctx.IsCommand {
		p.trackUsage(ctx)
	}

	return nil
}

// Prefetch fetches today's Word of the Day from Wordnik and stores it in the database.
func (p *WOTDPlugin) Prefetch() error {
	if p.apiKey == "" {
		slog.Warn("wotd: WORDNIK_API_KEY not set, skipping prefetch")
		return nil
	}

	today := time.Now().UTC().Format("2006-01-02")

	// Check if already fetched
	d := db.Get()
	var exists int
	err := d.QueryRow(`SELECT 1 FROM wotd_log WHERE date = ?`, today).Scan(&exists)
	if err == nil {
		slog.Info("wotd: already fetched for today", "date", today)
		return nil
	}

	url := fmt.Sprintf("https://api.wordnik.com/v4/words.json/wordOfTheDay?api_key=%s", p.apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("wotd: create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wotd: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wotd: API returned status %d", resp.StatusCode)
	}

	var wotd wordnikWOTDResponse
	if err := json.NewDecoder(resp.Body).Decode(&wotd); err != nil {
		return fmt.Errorf("wotd: decode response: %w", err)
	}

	definition := ""
	partOfSpeech := ""
	if len(wotd.Definitions) > 0 {
		definition = wotd.Definitions[0].Text
		partOfSpeech = wotd.Definitions[0].PartOfSpeech
	}

	example := ""
	if len(wotd.Examples) > 0 {
		example = wotd.Examples[0].Text
	}

	_, err = d.Exec(
		`INSERT INTO wotd_log (date, word, definition, part_of_speech, example, posted)
		 VALUES (?, ?, ?, ?, ?, 0)
		 ON CONFLICT(date) DO NOTHING`,
		today, wotd.Word, definition, partOfSpeech, example,
	)
	if err != nil {
		return fmt.Errorf("wotd: store: %w", err)
	}

	slog.Info("wotd: prefetched", "date", today, "word", wotd.Word)
	return nil
}

// PostWOTD posts today's Word of the Day to the given room and marks it as posted.
func (p *WOTDPlugin) PostWOTD(roomID id.RoomID) error {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	// Per-room dedup
	roomKey := fmt.Sprintf("%s:%s", today, roomID)
	if db.JobCompleted("wotd", roomKey) {
		slog.Info("wotd: already posted today", "date", today, "room", roomID)
		return nil
	}

	var word, definition, partOfSpeech, example string
	err := d.QueryRow(
		`SELECT word, definition, part_of_speech, example FROM wotd_log WHERE date = ?`, today,
	).Scan(&word, &definition, &partOfSpeech, &example)
	if err == sql.ErrNoRows {
		slog.Warn("wotd: no entry for today, attempting prefetch", "date", today)
		if err := p.Prefetch(); err != nil {
			return fmt.Errorf("wotd: prefetch failed: %w", err)
		}
		err = d.QueryRow(
			`SELECT word, definition, part_of_speech, example FROM wotd_log WHERE date = ?`, today,
		).Scan(&word, &definition, &partOfSpeech, &example)
		if err != nil {
			return fmt.Errorf("wotd: still no entry after prefetch: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("wotd: query: %w", err)
	}

	msg := p.formatWOTD(word, definition, partOfSpeech, example)
	if err := p.SendMessage(roomID, msg); err != nil {
		return fmt.Errorf("wotd: send message: %w", err)
	}

	db.MarkJobCompleted("wotd", roomKey)

	return nil
}

func (p *WOTDPlugin) handleWOTD(ctx MessageContext) error {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	var word, definition, partOfSpeech, example string
	err := d.QueryRow(
		`SELECT word, definition, part_of_speech, example FROM wotd_log WHERE date = ?`, today,
	).Scan(&word, &definition, &partOfSpeech, &example)
	if err == sql.ErrNoRows {
		// Prefetch on demand
		if pfErr := p.Prefetch(); pfErr != nil {
			slog.Error("wotd: on-demand prefetch failed", "err", pfErr)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch Word of the Day. Try again later.")
		}
		err = d.QueryRow(
			`SELECT word, definition, part_of_speech, example FROM wotd_log WHERE date = ?`, today,
		).Scan(&word, &definition, &partOfSpeech, &example)
	}
	if err != nil {
		slog.Error("wotd: query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch Word of the Day.")
	}

	msg := p.formatWOTD(word, definition, partOfSpeech, example)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *WOTDPlugin) formatWOTD(word, definition, partOfSpeech, example string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Word of the Day: %s", word))

	if partOfSpeech != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", partOfSpeech))
	}

	if definition != "" {
		sb.WriteString(fmt.Sprintf("\n\nDefinition: %s", definition))
	}

	if example != "" {
		sb.WriteString(fmt.Sprintf("\n\nExample: \"%s\"", example))
	}

	sb.WriteString("\n\nUse this word in a message today to earn 25 XP!")
	return sb.String()
}

// trackUsage checks if the user used the WOTD in their message and rewards them.
func (p *WOTDPlugin) trackUsage(ctx MessageContext) {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	// Get today's word
	var word string
	err := d.QueryRow(`SELECT word FROM wotd_log WHERE date = ?`, today).Scan(&word)
	if err != nil {
		return // No word today or error; silently skip
	}

	// Check if the message contains the word (case-insensitive)
	if !strings.Contains(strings.ToLower(ctx.Body), strings.ToLower(word)) {
		return
	}

	// Atomically insert/increment usage and try to claim reward in one step
	// This avoids the TOCTOU race where two messages could both grant XP
	_, err = d.Exec(
		`INSERT INTO wotd_usage (user_id, date, count, rewarded) VALUES (?, ?, 1, 0)
		 ON CONFLICT(user_id, date) DO UPDATE SET count = count + 1`,
		string(ctx.Sender), today,
	)
	if err != nil {
		slog.Error("wotd: track usage", "err", err)
		return
	}

	// Check if already rewarded before doing LLM verification
	var rewarded int
	err = d.QueryRow(
		`SELECT rewarded FROM wotd_usage WHERE user_id = ? AND date = ?`,
		string(ctx.Sender), today,
	).Scan(&rewarded)
	if err != nil || rewarded == 1 {
		return // Already rewarded or error
	}

	// Verify correct usage with LLM
	if !p.verifyUsage(word, ctx.Body) {
		slog.Debug("wotd: LLM rejected usage", "user", ctx.Sender, "word", word)
		return
	}

	// Atomically claim the reward: only update if not yet rewarded
	res, err := d.Exec(
		`UPDATE wotd_usage SET rewarded = 1 WHERE user_id = ? AND date = ? AND rewarded = 0`,
		string(ctx.Sender), today,
	)
	if err != nil {
		slog.Error("wotd: claim reward", "err", err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return // Already rewarded
	}

	// Grant 25 XP via direct SQL
	p.grantWOTDXP(ctx.Sender, 25)

	if err := p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Nice! You used today's word \"%s\" correctly and earned 25 XP!", word)); err != nil {
		slog.Error("wotd: send reward message", "err", err)
	}
}

// verifyUsage asks the LLM whether the word was used correctly in context.
// Returns false if LLM is not configured or on any error.
func (p *WOTDPlugin) verifyUsage(word, message string) bool {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host == "" || model == "" {
		return false
	}

	prompt := fmt.Sprintf(`A user sent this chat message: "%s"

The Word of the Day is "%s". Was this word used correctly and meaningfully in the message? The user must demonstrate understanding of the word by using it naturally in a sentence — not just mentioning it, quoting it, or saying "the word is X".

Respond with ONLY "yes" or "no".`, message, word)

	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"think":  false,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	apiURL := strings.TrimRight(host, "/") + "/api/generate"
	slog.Debug("wotd: sending LLM verification request", "url", apiURL, "word", word)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		slog.Error("wotd: LLM verify request failed", "err", err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		Response string `json:"response"`
	}
	if json.Unmarshal(body, &result) != nil {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(result.Response))
	accepted := strings.HasPrefix(answer, "yes")
	slog.Debug("wotd: LLM verification", "word", word, "answer", answer, "accepted", accepted)
	return accepted
}

// grantWOTDXP inserts XP directly via SQL to avoid cross-plugin dependency.
func (p *WOTDPlugin) grantWOTDXP(userID id.UserID, amount int) {
	d := db.Get()

	// Ensure user exists
	_, err := d.Exec(
		`INSERT INTO users (user_id, xp, level, last_xp_at) VALUES (?, 0, 0, 0)
		 ON CONFLICT(user_id) DO NOTHING`, string(userID))
	if err != nil {
		slog.Error("wotd: ensure user", "err", err)
		return
	}

	// Update XP
	_, err = d.Exec(
		`UPDATE users SET xp = xp + ?, last_xp_at = ? WHERE user_id = ?`,
		amount, time.Now().UTC().Unix(), string(userID))
	if err != nil {
		slog.Error("wotd: grant xp", "err", err)
		return
	}

	// Log XP grant
	_, err = d.Exec(
		`INSERT INTO xp_log (user_id, amount, reason) VALUES (?, ?, ?)`,
		string(userID), amount, "wotd_usage")
	if err != nil {
		slog.Error("wotd: log xp", "err", err)
	}
}
