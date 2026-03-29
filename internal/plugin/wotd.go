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
	"gogobee/internal/dreamclient"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// WOTDPlugin provides a Word of the Day feature using DreamDict.
type WOTDPlugin struct {
	Base
	dict *dreamclient.Client
}

// NewWOTDPlugin creates a new WOTDPlugin.
func NewWOTDPlugin(client *mautrix.Client, dict *dreamclient.Client) *WOTDPlugin {
	return &WOTDPlugin{
		Base: NewBase(client),
		dict: dict,
	}
}

func (p *WOTDPlugin) Name() string { return "wotd" }

func (p *WOTDPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "wotd", Description: "Show today's Palavra do Dia", Usage: "!wotd [force]", Category: "Lookup & Reference"},
	}
}

func (p *WOTDPlugin) Init() error { return nil }

func (p *WOTDPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *WOTDPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "wotd") {
		go func() {
			args := strings.TrimSpace(p.GetArgs(ctx.Body, "wotd"))
			if strings.ToLower(args) == "force" {
				if err := p.handleWOTDForce(ctx); err != nil {
					slog.Error("wotd: force handler error", "err", err)
				}
			} else {
				if err := p.handleWOTD(ctx); err != nil {
					slog.Error("wotd: handler error", "err", err)
				}
			}
		}()
		return nil
	}

	// Passive: track WOTD usage in messages
	if !ctx.IsCommand {
		p.trackUsage(ctx)
	}

	return nil
}

// Prefetch picks today's Palavra do Dia from DreamDict and stores it in the database.
// Prefers pt-PT words with at least one definition and one English translation.
func (p *WOTDPlugin) Prefetch() error {
	return p.prefetchWord(false)
}

func (p *WOTDPlugin) prefetchWord(force bool) error {
	if p.dict == nil {
		slog.Warn("wotd: DreamDict not configured, skipping prefetch")
		return nil
	}

	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	if !force {
		var exists int
		err := d.QueryRow(`SELECT 1 FROM wotd_log WHERE date = ?`, today).Scan(&exists)
		if err == nil {
			slog.Info("wotd: already fetched for today", "date", today)
			return nil
		}
	}

	// Pick a random pt-PT word with definitions; prefer one with English translations.
	var word, definition, partOfSpeech, translationsJSON string

	for attempt := 0; attempt < 10; attempt++ {
		candidate, err := p.dict.RandomWord("pt-PT", "", 4, 14)
		if err != nil {
			slog.Warn("wotd: random word attempt failed", "attempt", attempt+1, "err", err)
			continue
		}

		defs, err := p.dict.Define(candidate, "pt-PT")
		if err != nil || len(defs) == 0 {
			continue
		}

		// Check for English translation (preferred but not required after 5 attempts)
		enTrans, _ := p.dict.Translate(candidate, "pt-PT", "en")
		if len(enTrans) == 0 && attempt < 5 {
			continue
		}

		frTrans, _ := p.dict.Translate(candidate, "pt-PT", "fr")

		word = candidate
		definition = defs[0].Gloss
		partOfSpeech = defs[0].POS

		// Store translations as JSON in the example column
		transMap := map[string][]string{}
		if len(enTrans) > 0 {
			transMap["en"] = enTrans
		}
		if len(frTrans) > 0 {
			transMap["fr"] = frTrans
		}
		if data, err := json.Marshal(transMap); err == nil {
			translationsJSON = string(data)
		}
		break
	}

	if word == "" {
		return fmt.Errorf("wotd: failed to find a pt-PT word with definitions after 10 attempts")
	}

	if force {
		// Delete existing entry for today so the INSERT below replaces it
		if _, delErr := d.Exec(`DELETE FROM wotd_log WHERE date = ?`, today); delErr != nil {
			slog.Error("wotd: force delete failed", "err", delErr)
		}
		// Also clear job-completed flags so PostWOTD will re-post
		d.Exec(`DELETE FROM job_completed WHERE job_name = 'wotd' AND job_key LIKE ?`, today+"%")
	}

	_, err := d.Exec(
		`INSERT INTO wotd_log (date, word, definition, part_of_speech, example, posted)
		 VALUES (?, ?, ?, ?, ?, 0)
		 ON CONFLICT(date) DO UPDATE SET word = ?, definition = ?, part_of_speech = ?, example = ?, posted = 0`,
		today, word, definition, partOfSpeech, translationsJSON,
		word, definition, partOfSpeech, translationsJSON,
	)
	if err != nil {
		return fmt.Errorf("wotd: store: %w", err)
	}

	slog.Info("wotd: prefetched", "date", today, "word", word)
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

func (p *WOTDPlugin) formatWOTD(word, definition, partOfSpeech, translationsJSON string) string {
	now := time.Now().UTC()
	dateStr := now.Format("Monday, 2 January 2006")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 **Palavra do Dia** — %s\n\n", dateStr))

	if partOfSpeech != "" {
		sb.WriteString(fmt.Sprintf("✨ **%s**  (%s)\n\n", word, partOfSpeech))
	} else {
		sb.WriteString(fmt.Sprintf("✨ **%s**\n\n", word))
	}

	// Portuguese definition
	if definition != "" {
		sb.WriteString(fmt.Sprintf("🇵🇹 pt-PT\n  %s\n\n", definition))
	}

	// Parse translations from JSON stored in example column
	if translationsJSON != "" {
		var transMap map[string][]string
		if json.Unmarshal([]byte(translationsJSON), &transMap) == nil {
			if enTrans, ok := transMap["en"]; ok && len(enTrans) > 0 {
				display := enTrans
				if len(display) > 5 {
					display = display[:5]
				}
				sb.WriteString(fmt.Sprintf("🇬🇧 en\n  %s\n\n", strings.Join(display, ", ")))
			}
			if frTrans, ok := transMap["fr"]; ok && len(frTrans) > 0 {
				display := frTrans
				if len(display) > 5 {
					display = display[:5]
				}
				sb.WriteString(fmt.Sprintf("🇫🇷 fr\n  %s\n\n", strings.Join(display, ", ")))
			}
		}
	}

	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Learn more: `!define %s pt-PT`\n", word))
	sb.WriteString("Use this word in a message today to earn 25 XP!")
	return sb.String()
}

func (p *WOTDPlugin) handleWOTDForce(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only moderators can force a new Word of the Day.")
	}

	if err := p.prefetchWord(true); err != nil {
		slog.Error("wotd: force prefetch failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch a new Word of the Day. Try again later.")
	}

	return p.handleWOTD(ctx)
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

	response := result.Response
	// Strip <think>...</think> blocks (Qwen 3.5 reasoning)
	if i := strings.Index(response, "<think>"); i != -1 {
		if j := strings.Index(response, "</think>"); j != -1 {
			response = response[:i] + response[j+len("</think>"):]
		}
	}
	answer := strings.ToLower(strings.TrimSpace(response))
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
	db.Exec("wotd: log xp",
		`INSERT INTO xp_log (user_id, amount, reason) VALUES (?, ?, ?)`,
		string(userID), amount, "wotd_usage")
}
