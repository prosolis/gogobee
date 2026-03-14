package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// profanityKeywords is a basic list used for pre-filtering.
var profanityKeywords = []string{
	"fuck", "shit", "damn", "hell", "ass", "bitch", "crap", "dick",
	"bastard", "piss", "cock", "cunt", "douche", "dumbass", "idiot",
	"moron", "stupid", "stfu", "wtf", "lmao", "lmfao",
}

var thanksPatternRe = regexp.MustCompile(`(?i)\b(thanks|thank\s+you|thankyou|thx|ty|tysm|tyvm|appreciate)\b`)
var mentionRe = regexp.MustCompile(`@[a-zA-Z0-9._=-]+:[a-zA-Z0-9.-]+`)

// classificationResult holds the parsed LLM JSON response.
type classificationResult struct {
	Sentiment         string   `json:"sentiment"`
	SentimentScore    float64  `json:"sentiment_score"`
	Topics            []string `json:"topics"`
	Profanity         bool     `json:"profanity"`
	ProfanitySeverity int      `json:"profanity_severity"`
	InsultTarget      string   `json:"insult_target"`
	WOTDUsed          bool     `json:"wotd_used"`
	GratitudeTarget   string   `json:"gratitude_target"`
}

// queueItem holds a message pending classification.
type queueItem struct {
	UserID        id.UserID
	RoomID        id.RoomID
	EventID       id.EventID
	Body          string
	FormattedBody string
}

// LLMPassivePlugin classifies messages using Ollama and reacts accordingly.
type LLMPassivePlugin struct {
	Base
	xp         *XPPlugin
	ollamaHost  string
	ollamaModel string
	sampleRate  float64
	enabled     bool

	mu      sync.Mutex
	queue   []queueItem
	backoff time.Duration

	httpClient *http.Client
	stopCh     chan struct{}
}

// NewLLMPassivePlugin creates a new LLM passive classification plugin.
func NewLLMPassivePlugin(client *mautrix.Client, xp *XPPlugin) *LLMPassivePlugin {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	enabled := host != "" && model != ""

	sampleRate := 0.15
	if v := os.Getenv("LLM_SAMPLE_RATE"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 && parsed <= 1 {
			sampleRate = parsed
		}
	}

	p := &LLMPassivePlugin{
		Base:        NewBase(client),
		xp:          xp,
		ollamaHost:  host,
		ollamaModel: model,
		sampleRate:  sampleRate,
		enabled:     enabled,
		backoff:     5 * time.Second,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		stopCh:      make(chan struct{}),
	}

	return p
}

func (p *LLMPassivePlugin) Name() string { return "llm_passive" }

func (p *LLMPassivePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "potty", Description: "Show profanity count for a user", Usage: "!potty [@user]", Category: "LLM & Sentiment"},
		{Name: "pottyboard", Description: "Top 10 most profane users", Usage: "!pottyboard", Category: "LLM & Sentiment"},
		{Name: "insults", Description: "Show insult stats for a user", Usage: "!insults [@user]", Category: "LLM & Sentiment"},
		{Name: "insultboard", Description: "Top 10 most insulted users", Usage: "!insultboard", Category: "LLM & Sentiment"},
		{Name: "sentiment", Description: "Show sentiment stats for a user", Usage: "!sentiment [@user]", Category: "LLM & Sentiment"},
		{Name: "roomsentiment", Description: "Show sentiment breakdown for this room", Usage: "!roomsentiment", Category: "LLM & Sentiment"},
	}
}

func (p *LLMPassivePlugin) Init() error {
	if p.enabled {
		slog.Info("llm_passive: enabled", "host", p.ollamaHost, "model", p.ollamaModel, "sample_rate", p.sampleRate)
		go p.processQueue()
	} else {
		slog.Warn("llm_passive: disabled (OLLAMA_HOST or OLLAMA_MODEL not set)",
			"host", p.ollamaHost, "model", p.ollamaModel)
	}
	return nil
}

func (p *LLMPassivePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *LLMPassivePlugin) OnMessage(ctx MessageContext) error {
	// Handle commands
	if p.IsCommand(ctx.Body, "potty") {
		return p.handlePotty(ctx)
	}
	if p.IsCommand(ctx.Body, "pottyboard") {
		return p.handlePottyboard(ctx)
	}
	if p.IsCommand(ctx.Body, "insults") {
		return p.handleInsults(ctx)
	}
	if p.IsCommand(ctx.Body, "insultboard") {
		return p.handleInsultboard(ctx)
	}
	if p.IsCommand(ctx.Body, "roomsentiment") {
		return p.handleRoomSentiment(ctx)
	}
	if p.IsCommand(ctx.Body, "sentiment") {
		return p.handleSentiment(ctx)
	}

	if !p.enabled {
		return nil
	}

	// Skip command messages
	if ctx.IsCommand {
		return nil
	}

	// Pre-filter: only classify messages that match certain criteria
	var fmtBody string
	if ctx.Event != nil {
		if mc := ctx.Event.Content.AsMessage(); mc != nil {
			fmtBody = mc.FormattedBody
		}
	}
	if !p.shouldClassify(ctx.Body, fmtBody) {
		slog.Debug("llm_passive: message did not pass pre-filter", "body_len", len(ctx.Body))
		return nil
	}

	// Enqueue for async classification
	slog.Debug("llm_passive: enqueuing message for classification", "user", ctx.Sender, "body_len", len(ctx.Body))
	var formattedBody string
	if ctx.Event != nil {
		if mc := ctx.Event.Content.AsMessage(); mc != nil {
			formattedBody = mc.FormattedBody
		}
	}
	p.mu.Lock()
	p.queue = append(p.queue, queueItem{
		UserID:        ctx.Sender,
		RoomID:        ctx.RoomID,
		EventID:       ctx.EventID,
		Body:          ctx.Body,
		FormattedBody: formattedBody,
	})
	p.mu.Unlock()

	return nil
}

// shouldClassify applies pre-filtering heuristics.
func (p *LLMPassivePlugin) shouldClassify(body, formattedBody string) bool {
	// Skip single-character messages (trivia answers, etc.)
	if len(strings.TrimSpace(body)) <= 1 {
		return false
	}

	lower := strings.ToLower(body)

	// Check profanity keywords
	for _, kw := range profanityKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Check mentions (plain text or HTML formatted body)
	if mentionRe.MatchString(body) {
		return true
	}
	if strings.Contains(formattedBody, "matrix.to/#/@") {
		return true
	}

	// Check WOTD pattern (simple heuristic)
	if strings.Contains(lower, "wotd") {
		return true
	}

	// Check non-ASCII
	for _, r := range body {
		if r > unicode.MaxASCII {
			return true
		}
	}

	// Check thanks patterns
	if thanksPatternRe.MatchString(body) {
		return true
	}

	// Random sample of remaining messages (controlled by LLM_SAMPLE_RATE)
	if p.sampleRate >= 1.0 || rand.Float64() < p.sampleRate {
		return true
	}

	return false
}

// processQueue processes the classification queue with backoff.
func (p *LLMPassivePlugin) processQueue() {
	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		p.mu.Lock()
		if len(p.queue) == 0 {
			p.mu.Unlock()
			time.Sleep(1 * time.Second)
			continue
		}
		item := p.queue[0]
		p.queue = p.queue[1:]
		p.mu.Unlock()

		slog.Debug("llm_passive: processing queued item", "user", item.UserID, "body_preview", truncate(item.Body, 50))
		err := p.classifyAndProcess(item)
		if err != nil {
			slog.Error("llm: classification failed", "err", err, "user", item.UserID)
			// Backoff on error
			p.mu.Lock()
			p.backoff *= 2
			if p.backoff > 5*time.Minute {
				p.backoff = 5 * time.Minute
			}
			p.mu.Unlock()
			time.Sleep(p.backoff)
		} else {
			// Reset backoff on success
			p.mu.Lock()
			p.backoff = 5 * time.Second
			p.mu.Unlock()
		}
	}
}

// extractMentionMap builds a display-name-to-MXID mapping from the HTML formatted body.
func extractMentionMap(formattedBody string) map[string]string {
	if formattedBody == "" {
		return nil
	}
	// Match: <a href="https://matrix.to/#/@user:server">Display Name</a>
	re := regexp.MustCompile(`<a\s+href="https://matrix\.to/#/(@[^"]+)">([^<]+)</a>`)
	matches := re.FindAllStringSubmatch(formattedBody, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make(map[string]string)
	for _, m := range matches {
		if len(m) >= 3 {
			result[m[2]] = m[1] // display name -> MXID
		}
	}
	return result
}

// classifyAndProcess sends a message to Ollama and processes the result.
func (p *LLMPassivePlugin) classifyAndProcess(item queueItem) error {
	// Build mention context so the LLM knows actual MXIDs
	mentionMap := extractMentionMap(item.FormattedBody)
	var mentionHint string
	if len(mentionMap) > 0 {
		var parts []string
		for displayName, mxid := range mentionMap {
			parts = append(parts, fmt.Sprintf("%s = %s", displayName, mxid))
		}
		mentionHint = "\nMentioned users: " + strings.Join(parts, ", ")
	}

	result, err := p.callOllama(item.Body + mentionHint)
	if err != nil {
		return fmt.Errorf("ollama call: %w", err)
	}

	// Resolve any display names in LLM targets back to MXIDs
	if result.InsultTarget != "" && !strings.Contains(result.InsultTarget, ":") {
		if mxid, ok := mentionMap[result.InsultTarget]; ok {
			result.InsultTarget = mxid
		}
	}
	if result.GratitudeTarget != "" && !strings.Contains(result.GratitudeTarget, ":") {
		if mxid, ok := mentionMap[result.GratitudeTarget]; ok {
			result.GratitudeTarget = mxid
		}
	}

	d := db.Get()

	// Store classification
	topicsJSON, _ := json.Marshal(result.Topics)
	profanityInt := 0
	if result.Profanity {
		profanityInt = 1
	}
	wotdInt := 0
	if result.WOTDUsed {
		wotdInt = 1
	}

	_, err = d.Exec(
		`INSERT INTO llm_classifications (user_id, room_id, message_text, sentiment, sentiment_score, topics, profanity, profanity_severity, insult_target, wotd_used, gratitude_target)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(item.UserID), string(item.RoomID), item.Body,
		result.Sentiment, result.SentimentScore, string(topicsJSON),
		profanityInt, result.ProfanitySeverity, result.InsultTarget, wotdInt, result.GratitudeTarget,
	)
	if err != nil {
		slog.Error("llm: store classification", "err", err)
	}

	// Aggregate sentiment stats
	sentimentCol := "neutral"
	validSentiments := map[string]bool{
		"positive": true, "negative": true, "neutral": true,
		"excited": true, "sarcastic": true, "frustrated": true,
		"curious": true, "grateful": true, "humorous": true, "supportive": true,
	}
	if validSentiments[result.Sentiment] {
		sentimentCol = result.Sentiment
	}
	_, _ = d.Exec(
		fmt.Sprintf(
			`INSERT INTO sentiment_stats (user_id, %s, total_score) VALUES (?, 1, ?)
			 ON CONFLICT(user_id) DO UPDATE SET %s = %s + 1, total_score = total_score + ?`,
			sentimentCol, sentimentCol, sentimentCol),
		string(item.UserID), result.SentimentScore, result.SentimentScore,
	)

	// Aggregate room sentiment stats
	_, _ = d.Exec(
		fmt.Sprintf(
			`INSERT INTO room_sentiment_stats (room_id, %s, total_score) VALUES (?, 1, ?)
			 ON CONFLICT(room_id) DO UPDATE SET %s = %s + 1, total_score = total_score + ?`,
			sentimentCol, sentimentCol, sentimentCol),
		string(item.RoomID), result.SentimentScore, result.SentimentScore,
	)

	// Track profanity with severity
	if result.Profanity {
		severity := result.ProfanitySeverity
		if severity < 1 {
			severity = 1
		}
		if severity > 3 {
			severity = 3
		}
		mildInc, modInc, scorchInc := 0, 0, 0
		switch severity {
		case 1:
			mildInc = 1
		case 2:
			modInc = 1
		case 3:
			scorchInc = 1
		}
		_, _ = d.Exec(
			`INSERT INTO potty_mouth (user_id, count, mild, moderate, scorching) VALUES (?, 1, ?, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET count = count + 1, mild = mild + ?, moderate = moderate + ?, scorching = scorching + ?`,
			string(item.UserID), mildInc, modInc, scorchInc, mildInc, modInc, scorchInc,
		)
	}

	// React if someone insults the bot
	if result.InsultTarget == string(p.Client.UserID) {
		botReactions := []string{
			"\U0001f595", // 🖕
			"\U0001f52a", // 🔪
			"\U0001f5e1", // 🗡️
			"\U0001fa78", // 🩸
			"\U0001f4a3", // 💣
		}
		emoji := botReactions[rand.Intn(len(botReactions))]
		_ = p.SendReact(item.RoomID, item.EventID, emoji)
	}

	// Track insults
	if result.InsultTarget != "" {
		_, _ = d.Exec(
			`INSERT INTO insult_log (user_id, times_insulting) VALUES (?, 1)
			 ON CONFLICT(user_id) DO UPDATE SET times_insulting = times_insulting + 1`,
			string(item.UserID),
		)
		_, _ = d.Exec(
			`INSERT INTO insult_log (user_id, times_insulted) VALUES (?, 1)
			 ON CONFLICT(user_id) DO UPDATE SET times_insulted = times_insulted + 1`,
			result.InsultTarget,
		)
	}

	// React with emojis based on sentiment
	sentimentEmojis := map[string]string{
		"positive":    "\U0001f44d", // 👍
		"negative":    "\U0001f44e", // 👎
		"excited":     "\U0001f525", // 🔥
		"sarcastic":   "\U0001f928", // 🤨
		"frustrated":  "\U0001f62e\u200d\U0001f4a8", // 😮‍💨
		"curious":     "\U0001f9d0", // 🧐
		"grateful":    "\U0001f49c", // 💜
		"humorous":    "\U0001f602", // 😂
		"supportive":  "\U0001f917", // 🤗
	}
	if emoji, ok := sentimentEmojis[result.Sentiment]; ok && result.Sentiment != "neutral" {
		// Only react to strong sentiments (|score| > 0.5)
		score := result.SentimentScore
		if score > 0.5 || score < -0.5 {
			_ = p.SendReact(item.RoomID, item.EventID, emoji)
		}
	}
	if result.Profanity {
		switch result.ProfanitySeverity {
		case 3:
			_ = p.SendReact(item.RoomID, item.EventID, "\U0001f92c") // 🤬
		case 2:
			_ = p.SendReact(item.RoomID, item.EventID, "\U0001f632") // 😲
		default:
			_ = p.SendReact(item.RoomID, item.EventID, "\U0001fae3") // 🫣
		}
	}
	if result.WOTDUsed {
		_ = p.SendReact(item.RoomID, item.EventID, "\U0001f4d6") // open book
	}
	if result.GratitudeTarget != "" {
		_ = p.SendReact(item.RoomID, item.EventID, "\U0001f49c") // purple heart
	}

	// Grant XP for gratitude
	if result.GratitudeTarget != "" && p.xp != nil {
		targetID := id.UserID(result.GratitudeTarget)
		if targetID != item.UserID {
			p.xp.GrantXP(targetID, 5, "gratitude")
		}
	}

	return nil
}

// ollamaRequest is the request body for the Ollama API.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Think  bool   `json:"think"`
}

// ollamaResponse is the response from the Ollama API.
type ollamaResponse struct {
	Response string `json:"response"`
}

// callOllama sends a classification prompt to Ollama and parses the JSON result.
func (p *LLMPassivePlugin) callOllama(messageText string) (*classificationResult, error) {
	prompt := fmt.Sprintf(`Classify the following chat message. Respond ONLY with valid JSON (no markdown, no explanation).

JSON schema:
{
  "sentiment": "positive" | "negative" | "neutral" | "excited" | "sarcastic" | "frustrated" | "curious" | "grateful" | "humorous" | "supportive",
  "sentiment_score": number between -1.0 and 1.0,
  "topics": ["topic1", "topic2"],
  "profanity": true | false,
  "profanity_severity": 0 | 1 | 2 | 3 (0=none, 1=mild e.g. damn/hell/crap, 2=moderate e.g. shit/ass/bitch, 3=scorching e.g. fuck/cunt and slurs),
  "insult_target": "" or "@user:server" if someone is being insulted,
  "wotd_used": true | false (if the message uses an unusual/sophisticated word),
  "gratitude_target": "" or "@user:server" if thanking someone
}

Message: %s`, messageText)

	reqBody := ollamaRequest{
		Model:  p.ollamaModel,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.ollamaHost, "/") + "/api/generate"
	slog.Debug("llm_passive: calling ollama", "url", url, "model", p.ollamaModel)
	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	result, err := parseClassification(ollamaResp.Response)
	if err != nil {
		return nil, fmt.Errorf("parse classification: %w", err)
	}

	return result, nil
}

// parseClassification parses a JSON classification response with repair logic.
func parseClassification(raw string) (*classificationResult, error) {
	cleaned := raw

	// Remove <think>...</think> blocks (Qwen 3.5 reasoning)
	if i := strings.Index(cleaned, "<think>"); i != -1 {
		if j := strings.Index(cleaned, "</think>"); j != -1 {
			cleaned = cleaned[:i] + cleaned[j+len("</think>"):]
			cleaned = strings.TrimSpace(cleaned)
		}
	}

	// Remove markdown code fences
	cleaned = strings.TrimSpace(cleaned)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
	}
	if strings.HasSuffix(cleaned, "```") {
		cleaned = strings.TrimSuffix(cleaned, "```")
	}
	cleaned = strings.TrimSpace(cleaned)

	// Try to parse directly first
	var result classificationResult
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil {
		return &result, nil
	}

	// Repair: replace single quotes with double quotes
	repaired := strings.ReplaceAll(cleaned, "'", "\"")

	// Repair: remove trailing commas before } or ]
	trailingCommaRe := regexp.MustCompile(`,\s*([}\]])`)
	repaired = trailingCommaRe.ReplaceAllString(repaired, "$1")

	if err := json.Unmarshal([]byte(repaired), &result); err != nil {
		return nil, fmt.Errorf("JSON parse failed after repair: %w (raw: %s)", err, raw)
	}

	return &result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Command handlers

func (p *LLMPassivePlugin) handlePotty(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "potty")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var count, mild, moderate, scorching int
	err := d.QueryRow(
		`SELECT COALESCE(count, 0), COALESCE(mild, 0), COALESCE(moderate, 0), COALESCE(scorching, 0)
		 FROM potty_mouth WHERE user_id = ?`,
		string(target),
	).Scan(&count, &mild, &moderate, &scorching)
	if err != nil {
		count = 0
	}

	msg := fmt.Sprintf("Profanity for %s: %s total (🫣 mild: %s, 😲 moderate: %s, 🤬 scorching: %s)",
		string(target), formatNumber(count), formatNumber(mild), formatNumber(moderate), formatNumber(scorching))
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *LLMPassivePlugin) handlePottyboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, count FROM potty_mouth ORDER BY count DESC`,
	)
	if err != nil {
		slog.Error("llm: pottyboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load potty board.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Potty Mouth Board — Top 10\n\n")

	medals := []string{"\U0001f947", "\U0001f948", "\U0001f949"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s incidents\n", prefix, userID, formatNumber(count)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No profanity data yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LLMPassivePlugin) handleInsults(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "insults")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var insulted, insulting int
	err := d.QueryRow(
		`SELECT COALESCE(times_insulted, 0), COALESCE(times_insulting, 0) FROM insult_log WHERE user_id = ?`,
		string(target),
	).Scan(&insulted, &insulting)
	if err != nil {
		insulted = 0
		insulting = 0
	}

	msg := fmt.Sprintf("Insult stats for %s:\nTimes insulted: %s\nTimes insulting others: %s",
		string(target), formatNumber(insulted), formatNumber(insulting))
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *LLMPassivePlugin) handleInsultboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, times_insulted FROM insult_log ORDER BY times_insulted DESC`,
	)
	if err != nil {
		slog.Error("llm: insultboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load insult board.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Most Insulted — Top 10\n\n")

	medals := []string{"\U0001f947", "\U0001f948", "\U0001f949"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s times\n", prefix, userID, formatNumber(count)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No insult data yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LLMPassivePlugin) handleSentiment(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "sentiment")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var positive, negative, neutral, excited, sarcastic, frustrated, curious, grateful, humorous, supportive int
	var totalScore float64
	err := d.QueryRow(
		`SELECT COALESCE(positive, 0), COALESCE(negative, 0), COALESCE(neutral, 0),
		        COALESCE(excited, 0), COALESCE(sarcastic, 0), COALESCE(frustrated, 0),
		        COALESCE(curious, 0), COALESCE(grateful, 0), COALESCE(humorous, 0),
		        COALESCE(supportive, 0), COALESCE(total_score, 0)
		 FROM sentiment_stats WHERE user_id = ?`,
		string(target),
	).Scan(&positive, &negative, &neutral, &excited, &sarcastic, &frustrated,
		&curious, &grateful, &humorous, &supportive, &totalScore)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No sentiment data for %s yet.", string(target)))
	}

	total := positive + negative + neutral + excited + sarcastic + frustrated + curious + grateful + humorous + supportive
	avgScore := 0.0
	if total > 0 {
		avgScore = totalScore / float64(total)
	}

	mood := "neutral"
	if avgScore > 0.3 {
		mood = "mostly positive"
	} else if avgScore > 0.1 {
		mood = "leaning positive"
	} else if avgScore < -0.3 {
		mood = "mostly negative"
	} else if avgScore < -0.1 {
		mood = "leaning negative"
	}

	// Build sentiment breakdown, only showing non-zero counts
	type sentEntry struct {
		emoji string
		label string
		count int
	}
	entries := []sentEntry{
		{"👍", "Positive", positive},
		{"🔥", "Excited", excited},
		{"🤗", "Supportive", supportive},
		{"💜", "Grateful", grateful},
		{"😂", "Humorous", humorous},
		{"🧐", "Curious", curious},
		{"😐", "Neutral", neutral},
		{"🤨", "Sarcastic", sarcastic},
		{"😮\u200d💨", "Frustrated", frustrated},
		{"👎", "Negative", negative},
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Sentiment for %s:\n", string(target)))
	for _, e := range entries {
		if e.count > 0 {
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", e.emoji, e.label, formatNumber(e.count)))
		}
	}
	sb.WriteString(fmt.Sprintf("Average mood: %.2f (%s)", avgScore, mood))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LLMPassivePlugin) handleRoomSentiment(ctx MessageContext) error {
	d := db.Get()
	var positive, negative, neutral, excited, sarcastic, frustrated, curious, grateful, humorous, supportive int
	var totalScore float64
	err := d.QueryRow(
		`SELECT COALESCE(positive, 0), COALESCE(negative, 0), COALESCE(neutral, 0),
		        COALESCE(excited, 0), COALESCE(sarcastic, 0), COALESCE(frustrated, 0),
		        COALESCE(curious, 0), COALESCE(grateful, 0), COALESCE(humorous, 0),
		        COALESCE(supportive, 0), COALESCE(total_score, 0)
		 FROM room_sentiment_stats WHERE room_id = ?`,
		string(ctx.RoomID),
	).Scan(&positive, &negative, &neutral, &excited, &sarcastic, &frustrated,
		&curious, &grateful, &humorous, &supportive, &totalScore)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No sentiment data for this room yet.")
	}

	total := positive + negative + neutral + excited + sarcastic + frustrated + curious + grateful + humorous + supportive
	avgScore := 0.0
	if total > 0 {
		avgScore = totalScore / float64(total)
	}

	mood := "neutral"
	if avgScore > 0.3 {
		mood = "mostly positive"
	} else if avgScore > 0.1 {
		mood = "leaning positive"
	} else if avgScore < -0.3 {
		mood = "mostly negative"
	} else if avgScore < -0.1 {
		mood = "leaning negative"
	}

	type sentEntry struct {
		emoji string
		label string
		count int
	}
	entries := []sentEntry{
		{"👍", "Positive", positive},
		{"🔥", "Excited", excited},
		{"🤗", "Supportive", supportive},
		{"💜", "Grateful", grateful},
		{"😂", "Humorous", humorous},
		{"🧐", "Curious", curious},
		{"😐", "Neutral", neutral},
		{"🤨", "Sarcastic", sarcastic},
		{"😮\u200d💨", "Frustrated", frustrated},
		{"👎", "Negative", negative},
	}

	var sb strings.Builder
	sb.WriteString("**Room Sentiment:**\n")
	for _, e := range entries {
		if e.count > 0 {
			pct := float64(e.count) / float64(total) * 100
			sb.WriteString(fmt.Sprintf("  %s %s: %s (%.0f%%)\n", e.emoji, e.label, formatNumber(e.count), pct))
		}
	}
	sb.WriteString(fmt.Sprintf("\n%s messages classified | Average mood: %.2f (%s)", formatNumber(total), avgScore, mood))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
