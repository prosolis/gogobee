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
		{Name: "wotd", Description: "Show today's Word of the Day", Usage: "!wotd [force]", Category: "Lookup & Reference"},
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

// pickWord attempts to find a suitable word in the given language.
// Returns word, definition, partOfSpeech, translationsJSON.
// For non-English languages, requires English translations and filters cognates.
// For all languages, fetches synonyms and cross-translations.
func (p *WOTDPlugin) pickWord(lang string) (string, string, string, string) {
	// Define which other languages to optionally translate into.
	// English translation is required for non-English words; others are best-effort.
	type transTarget struct {
		lang     string
		key      string
		required bool
	}
	var targets []transTarget
	switch lang {
	case "pt-PT":
		targets = []transTarget{{"en", "en", false}, {"fr", "fr", false}}
	case "fr":
		targets = []transTarget{{"en", "en", false}, {"pt-PT", "pt-PT", false}}
	case "en":
		targets = []transTarget{{"fr", "fr", false}, {"pt-PT", "pt-PT", false}}
	}

	for attempt := 0; attempt < 100; attempt++ {
		candidate, err := p.dict.RandomWord(lang, "", 4, 14, 0)
		if err != nil {
			continue
		}

		defs, err := p.dict.Define(candidate, lang)
		if err != nil || len(defs) == 0 {
			continue
		}

		transMap := map[string][]string{}
		valid := true

		for _, t := range targets {
			trans, _ := p.dict.Translate(candidate, lang, t.lang)
			if len(trans) == 0 && t.required {
				valid = false
				break
			}
			if len(trans) > 0 {
				transMap[t.key] = trans
			}
		}
		if !valid {
			continue
		}

		// For non-English words, reject English words that leaked into the
		// foreign language database.
		if lang != "en" {
			// Skip if the word is a valid English word — it's almost certainly
			// not a real Portuguese/French word.
			if engValid, _ := p.dict.IsValidWord(candidate, "en"); engValid {
				continue
			}

			// If no English translation from DreamDict, ask the LLM.
			if _, ok := transMap["en"]; !ok {
				if llmTrans := p.llmTranslate(candidate, lang); llmTrans != "" {
					transMap["en"] = []string{llmTrans}
				} else {
					continue // can't provide any English meaning, skip
				}
			}

			// Skip if the English translation is just the word itself
			// (cognate with no meaningful translation).
			if enTrans, ok := transMap["en"]; ok {
				meaningfulEn := false
				for _, t := range enTrans {
					if !strings.EqualFold(t, candidate) {
						meaningfulEn = true
						break
					}
				}
				if !meaningfulEn {
					continue
				}
			}
		}

		// Fetch synonyms in the word's own language
		synonyms, _ := p.dict.Synonyms(candidate, lang)
		if len(synonyms) > 0 {
			transMap["syn"] = synonyms
		}

		// Fetch etymology
		if etym, err := p.dict.Etymology(candidate, lang); err == nil && etym != "" {
			// Truncate for WOTD display (max 200 chars).
			if len(etym) > 200 {
				if idx := strings.LastIndex(etym[:200], "."); idx > 100 {
					etym = etym[:idx+1] + "..."
				} else {
					etym = etym[:200] + "..."
				}
			}
			transMap["_etym"] = []string{etym}
		}

		var translationsJSON string
		if data, err := json.Marshal(transMap); err == nil {
			translationsJSON = string(data)
		}
		return candidate, defs[0].Gloss, defs[0].POS, translationsJSON
	}
	return "", "", "", ""
}

// Prefetch picks today's Word of the Day from DreamDict and stores it in the database.
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

	// Rotate language by day: pt-PT, fr, en
	langs := []string{"pt-PT", "fr", "en"}
	dayOfYear := time.Now().UTC().YearDay()
	lang := langs[dayOfYear%len(langs)]

	word, definition, partOfSpeech, translationsJSON := p.pickWord(lang)

	// Fallback through other languages if primary fails
	if word == "" {
		for _, fallback := range langs {
			if fallback == lang {
				continue
			}
			slog.Warn("wotd: failed to find word", "lang", lang, "fallback", fallback)
			word, definition, partOfSpeech, translationsJSON = p.pickWord(fallback)
			if word != "" {
				lang = fallback
				break
			}
		}
	}

	if word == "" {
		return fmt.Errorf("wotd: failed to find a word in any language")
	}

	// Store the language in the translations JSON so formatWOTD knows which language was picked
	var transMap map[string][]string
	json.Unmarshal([]byte(translationsJSON), &transMap)
	if transMap == nil {
		transMap = map[string][]string{}
	}
	transMap["_lang"] = []string{lang}
	if data, err := json.Marshal(transMap); err == nil {
		translationsJSON = string(data)
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

var wotdHeaders = map[string]string{
	"pt-PT": "📖 **Palavra do Dia** — %s\n\n",
	"fr":    "📖 **Mot du Jour** — %s\n\n",
	"en":    "📖 **Word of the Day** — %s\n\n",
}

var wotdFlags = map[string]string{
	"pt-PT": "🇵🇹",
	"fr":    "🇫🇷",
	"en":    "🇬🇧",
}

var wotdSynonymLabels = map[string]string{
	"pt-PT": "Sinónimos",
	"fr":    "Synonymes",
	"en":    "Synonyms",
}

func (p *WOTDPlugin) formatWOTD(word, definition, partOfSpeech, translationsJSON string) string {
	now := time.Now().UTC()
	dateStr := now.Format("Monday, 2 January 2006")

	// Determine the source language from stored metadata
	lang := "pt-PT" // default for backwards compat with old entries
	var transMap map[string][]string
	if translationsJSON != "" {
		json.Unmarshal([]byte(translationsJSON), &transMap)
	}
	if transMap != nil {
		if l, ok := transMap["_lang"]; ok && len(l) > 0 {
			lang = l[0]
		}
	}

	var sb strings.Builder

	header := wotdHeaders[lang]
	if header == "" {
		header = "📖 **Word of the Day** — %s\n\n"
	}
	sb.WriteString(fmt.Sprintf(header, dateStr))

	if partOfSpeech != "" {
		sb.WriteString(fmt.Sprintf("✨ **%s**  (%s)\n\n", word, partOfSpeech))
	} else {
		sb.WriteString(fmt.Sprintf("✨ **%s**\n\n", word))
	}

	// Definition in the word's own language
	flag := wotdFlags[lang]
	if flag == "" {
		flag = lang
	}
	if definition != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n  %s\n", flag, lang, definition))
	}

	// Synonyms
	if transMap != nil {
		if syns, ok := transMap["syn"]; ok && len(syns) > 0 {
			display := syns
			if len(display) > 5 {
				display = display[:5]
			}
			label := wotdSynonymLabels[lang]
			if label == "" {
				label = "Synonyms"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", label, strings.Join(display, ", ")))
		}
		sb.WriteString("\n")

		// Translations into other languages
		for _, tLang := range []string{"en", "fr", "pt-PT"} {
			if tLang == lang {
				continue
			}
			if trans, ok := transMap[tLang]; ok && len(trans) > 0 {
				display := trans
				if len(display) > 5 {
					display = display[:5]
				}
				tFlag := wotdFlags[tLang]
				if tFlag == "" {
					tFlag = tLang
				}
				sb.WriteString(fmt.Sprintf("%s %s\n  %s\n\n", tFlag, tLang, strings.Join(display, ", ")))
			}
		}
	} else {
		sb.WriteString("\n")
	}

	// Etymology section (if available).
	if transMap != nil {
		if etym, ok := transMap["_etym"]; ok && len(etym) > 0 && etym[0] != "" {
			sb.WriteString(fmt.Sprintf("Etymology\n  %s\n\n", etym[0]))
		}
	}

	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Learn more: `!define %s %s`\n", word, lang))
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

// llmTranslate asks the LLM for a brief English translation of a foreign word.
// Returns empty string on failure.
func (p *WOTDPlugin) llmTranslate(word, lang string) string {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host == "" || model == "" {
		return ""
	}

	langName := map[string]string{
		"pt-PT": "Portuguese",
		"fr":    "French",
	}[lang]
	if langName == "" {
		langName = lang
	}

	prompt := fmt.Sprintf(
		`Translate the %s word "%s" into English. Reply with ONLY the English translation — one or two words, no explanation, no punctuation.`,
		langName, word)

	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"think":  false,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	apiURL := strings.TrimRight(host, "/") + "/api/generate"
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		slog.Error("wotd: LLM translate request failed", "err", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var result struct {
		Response string `json:"response"`
	}
	if json.Unmarshal(body, &result) != nil {
		return ""
	}

	response := result.Response
	if i := strings.Index(response, "<think>"); i != -1 {
		if j := strings.Index(response, "</think>"); j != -1 {
			response = response[:i] + response[j+len("</think>"):]
		}
	}
	translation := strings.TrimSpace(response)
	if translation == "" || len(translation) > 50 {
		return ""
	}

	slog.Debug("wotd: LLM translation", "word", word, "lang", lang, "translation", translation)
	return translation
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
