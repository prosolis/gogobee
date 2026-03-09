package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// LookupPlugin provides Wikipedia, dictionary, Urban Dictionary, and translation lookups.
type LookupPlugin struct {
	Base
	rateLimiter *RateLimitsPlugin
}

// NewLookupPlugin creates a new lookup plugin.
func NewLookupPlugin(client *mautrix.Client, rateLimiter *RateLimitsPlugin) *LookupPlugin {
	return &LookupPlugin{
		Base:        NewBase(client),
		rateLimiter: rateLimiter,
	}
}

func (p *LookupPlugin) Name() string { return "lookup" }

func (p *LookupPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "wiki", Description: "Look up a Wikipedia summary", Usage: "!wiki <topic>", Category: "Lookup & Reference"},
		{Name: "define", Description: "Look up a word definition", Usage: "!define <word>", Category: "Lookup & Reference"},
		{Name: "urban", Description: "Look up Urban Dictionary definition", Usage: "!urban <term>", Category: "Lookup & Reference"},
		{Name: "translate", Description: "Translate text (pt/es/fr/de/ja/ko/zh/ar/ru/it)", Usage: "!translate [lang] <text>", Category: "Lookup & Reference"},
	}
}

func (p *LookupPlugin) Init() error { return nil }

func (p *LookupPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *LookupPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "wiki"):
		return p.handleWiki(ctx)
	case p.IsCommand(ctx.Body, "define"):
		return p.handleDefine(ctx)
	case p.IsCommand(ctx.Body, "urban"):
		return p.handleUrban(ctx)
	case p.IsCommand(ctx.Body, "translate"):
		return p.handleTranslate(ctx)
	}
	return nil
}

func (p *LookupPlugin) handleWiki(ctx MessageContext) error {
	topic := strings.TrimSpace(p.GetArgs(ctx.Body, "wiki"))
	if topic == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !wiki <topic>")
	}

	encoded := url.PathEscape(strings.ReplaceAll(topic, " ", "_"))
	apiURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", encoded)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		slog.Error("lookup: wiki request", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to reach Wikipedia.")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No Wikipedia article found for \"%s\".", topic))
	}
	if resp.StatusCode != 200 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Wikipedia returned an error.")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to read Wikipedia response.")
	}

	var result struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to parse Wikipedia response.")
	}

	extract := result.Extract
	if len(extract) > 500 {
		extract = extract[:500] + "..."
	}

	msg := fmt.Sprintf("%s\n\n%s\n\n%s", result.Title, extract, result.ContentURLs.Desktop.Page)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *LookupPlugin) handleDefine(ctx MessageContext) error {
	word := strings.TrimSpace(p.GetArgs(ctx.Body, "define"))
	if word == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !define <word>")
	}

	apiURL := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", url.PathEscape(word))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		slog.Error("lookup: define request", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to reach dictionary API.")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No definition found for \"%s\".", word))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to read dictionary response.")
	}

	var entries []struct {
		Word     string `json:"word"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string `json:"definition"`
				Example    string `json:"example"`
			} `json:"definitions"`
		} `json:"meanings"`
	}

	if err := json.Unmarshal(body, &entries); err != nil || len(entries) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No definition found for \"%s\".", word))
	}

	entry := entries[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Definition of \"%s\":\n\n", entry.Word))

	for i, meaning := range entry.Meanings {
		if i >= 3 { // Limit to 3 meanings
			break
		}
		sb.WriteString(fmt.Sprintf("(%s)\n", meaning.PartOfSpeech))
		for j, def := range meaning.Definitions {
			if j >= 2 { // Limit to 2 definitions per meaning
				break
			}
			sb.WriteString(fmt.Sprintf("  %d. %s\n", j+1, def.Definition))
			if def.Example != "" {
				sb.WriteString(fmt.Sprintf("     Example: \"%s\"\n", def.Example))
			}
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LookupPlugin) handleUrban(ctx MessageContext) error {
	term := strings.TrimSpace(p.GetArgs(ctx.Body, "urban"))
	if term == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !urban <term>")
	}

	d := db.Get()
	termLower := strings.ToLower(term)

	// Check cache (24h)
	var cachedData string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM urban_cache WHERE term = ?`,
		termLower,
	).Scan(&cachedData, &cachedAt)

	now := time.Now().UTC().Unix()
	if err == nil && now-cachedAt < 86400 {
		return p.SendReply(ctx.RoomID, ctx.EventID, cachedData)
	}

	// Fetch from API
	apiURL := fmt.Sprintf("https://api.urbandictionary.com/v0/define?term=%s", url.QueryEscape(term))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		slog.Error("lookup: urban request", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to reach Urban Dictionary.")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to read Urban Dictionary response.")
	}

	var result struct {
		List []struct {
			Word       string `json:"word"`
			Definition string `json:"definition"`
			Example    string `json:"example"`
			ThumbsUp   int    `json:"thumbs_up"`
			ThumbsDown int    `json:"thumbs_down"`
		} `json:"list"`
	}

	if err := json.Unmarshal(body, &result); err != nil || len(result.List) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No Urban Dictionary definition found for \"%s\".", term))
	}

	entry := result.List[0]
	def := entry.Definition
	// Remove bracket notation used by Urban Dictionary
	def = strings.ReplaceAll(def, "[", "")
	def = strings.ReplaceAll(def, "]", "")
	if len(def) > 400 {
		def = def[:400] + "..."
	}

	example := entry.Example
	example = strings.ReplaceAll(example, "[", "")
	example = strings.ReplaceAll(example, "]", "")
	if len(example) > 200 {
		example = example[:200] + "..."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Urban Dictionary: %s\n\n", entry.Word))
	sb.WriteString(fmt.Sprintf("%s\n", def))
	if example != "" {
		sb.WriteString(fmt.Sprintf("\nExample: %s\n", example))
	}
	sb.WriteString(fmt.Sprintf("\n+%d / -%d", entry.ThumbsUp, entry.ThumbsDown))

	msg := sb.String()

	// Cache the result
	_, err = d.Exec(
		`INSERT INTO urban_cache (term, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(term) DO UPDATE SET data = ?, cached_at = ?`,
		termLower, msg, now, msg, now,
	)
	if err != nil {
		slog.Error("lookup: urban cache", "err", err)
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

var supportedLangs = map[string]bool{
	"pt": true, "es": true, "fr": true, "de": true, "ja": true,
	"ko": true, "zh": true, "ar": true, "ru": true, "it": true,
}

func (p *LookupPlugin) handleTranslate(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "translate"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: !translate [lang] <text>\nSupported: pt, es, fr, de, ja, ko, zh, ar, ru, it")
	}

	ltURL := os.Getenv("LIBRETRANSLATE_URL")
	if ltURL == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Translation service is not configured.")
	}

	// Rate limit: 10 translations per day
	if p.rateLimiter != nil && !p.rateLimiter.CheckLimit(ctx.Sender, "translate", 10) {
		remaining := p.rateLimiter.Remaining(ctx.Sender, "translate", 10)
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Translation rate limit reached. %d remaining today.", remaining))
	}

	// Parse lang code and text
	parts := strings.SplitN(args, " ", 2)
	targetLang := "es" // default
	text := args

	if len(parts) >= 2 && supportedLangs[strings.ToLower(parts[0])] {
		targetLang = strings.ToLower(parts[0])
		text = parts[1]
	}

	// Call LibreTranslate
	payload := fmt.Sprintf(`{"q":%q,"source":"auto","target":%q}`, text, targetLang)
	req, err := http.NewRequest("POST", strings.TrimRight(ltURL, "/")+"/translate", strings.NewReader(payload))
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to create translation request.")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("lookup: translate request", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to reach translation service.")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to read translation response.")
	}

	var result struct {
		TranslatedText string `json:"translatedText"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to parse translation response.")
	}

	if result.Error != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Translation error: %s", result.Error))
	}

	msg := fmt.Sprintf("Translation (-> %s):\n%s", targetLang, result.TranslatedText)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}
