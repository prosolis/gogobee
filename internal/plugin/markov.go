package plugin

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"gogobee/internal/crypto"
	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// MarkovPlugin collects messages and generates trigram-based text.
type MarkovPlugin struct {
	Base
	encKey  []byte // AES-256 key; nil means disabled
	enabled bool
}

// NewMarkovPlugin creates a new Markov chain plugin.
func NewMarkovPlugin(client *mautrix.Client) *MarkovPlugin {
	p := &MarkovPlugin{
		Base: NewBase(client),
	}

	raw := os.Getenv("QUOTE_ENCRYPTION_KEY")
	if raw == "" {
		slog.Error("markov: QUOTE_ENCRYPTION_KEY not set — markov collection and commands disabled")
		return p
	}

	key, err := crypto.ParseKey(raw)
	if err != nil {
		slog.Error("markov: invalid QUOTE_ENCRYPTION_KEY — markov disabled", "err", err)
		return p
	}

	p.encKey = key
	p.enabled = true

	// Migration: purge any unencrypted rows
	p.purgeUnencryptedRows()

	return p
}

func (p *MarkovPlugin) Name() string { return "markov" }

func (p *MarkovPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "markov", Description: "Generate Markov chain text from a user's messages", Usage: "!markov [@user|me|stats|forget|leaderboard]", Category: "Fun & Games"},
		{Name: "impersonate", Description: "Alias for !markov @user", Usage: "!impersonate @user", Category: "Fun & Games"},
		{Name: "ghostwrite", Description: "Seed Markov chain with a starting phrase", Usage: "!ghostwrite @user <prompt>", Category: "Fun & Games"},
	}
}

func (p *MarkovPlugin) Init() error { return nil }

func (p *MarkovPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *MarkovPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		if p.IsCommand(ctx.Body, "markov") || p.IsCommand(ctx.Body, "impersonate") || p.IsCommand(ctx.Body, "ghostwrite") {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Markov is disabled (encryption key not configured).")
		}
		return nil
	}

	if p.IsCommand(ctx.Body, "markov") {
		return p.handleMarkov(ctx)
	}
	if p.IsCommand(ctx.Body, "impersonate") {
		return p.handleImpersonate(ctx)
	}
	if p.IsCommand(ctx.Body, "ghostwrite") {
		return p.handleGhostwrite(ctx)
	}

	// Passive: collect non-command messages
	if !ctx.IsCommand {
		p.collectMessage(ctx.Sender, ctx.Body)
	}

	return nil
}

// ── Regex for stripping noise from corpus entries ───────────────────────────

var (
	matrixEventIDRe = regexp.MustCompile(`\$[A-Za-z0-9_/+=.-]{20,}`)
	matrixRoomIDRe  = regexp.MustCompile(`![A-Za-z0-9_/+=.-]+:[A-Za-z0-9._-]+`)
	commandPrefixRe = regexp.MustCompile(`^!\S+\s*`)
)

// stripNoise removes Matrix event IDs, room IDs, and !command prefixes from text.
func stripNoise(text string) string {
	text = matrixEventIDRe.ReplaceAllString(text, "")
	text = matrixRoomIDRe.ReplaceAllString(text, "")
	text = commandPrefixRe.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// ── Encryption helpers ──────────────────────────────────────────────────────

func (p *MarkovPlugin) encryptText(plaintext string) (string, error) {
	ct, err := crypto.Encrypt(p.encKey, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (p *MarkovPlugin) decryptText(encoded string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	pt, err := crypto.Decrypt(p.encKey, ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// ── Corpus collection ───────────────────────────────────────────────────────

// collectMessage stores an encrypted message in the markov_corpus, capping at 10,000 per user.
func (p *MarkovPlugin) collectMessage(userID id.UserID, text string) {
	cleaned := stripNoise(text)
	if len(strings.Fields(cleaned)) < 3 {
		return
	}

	enc, err := p.encryptText(cleaned)
	if err != nil {
		slog.Error("markov: encrypt message", "err", err)
		return
	}

	d := db.Get()

	_, err = d.Exec(
		`INSERT INTO markov_corpus (user_id, text) VALUES (?, ?)`,
		string(userID), enc,
	)
	if err != nil {
		slog.Error("markov: insert message", "err", err)
		return
	}

	// Cap at 10,000 messages per user — delete oldest excess
	db.Exec("markov: prune corpus",
		`DELETE FROM markov_corpus WHERE user_id = ? AND id NOT IN (
			SELECT id FROM markov_corpus WHERE user_id = ? ORDER BY id DESC LIMIT 10000
		)`,
		string(userID), string(userID),
	)
}

// loadCorpus loads and decrypts all corpus entries for a user. Unencrypted/corrupt rows are skipped.
func (p *MarkovPlugin) loadCorpus(userID id.UserID) []string {
	d := db.Get()
	rows, err := d.Query(
		`SELECT text FROM markov_corpus WHERE user_id = ?`,
		string(userID),
	)
	if err != nil {
		slog.Error("markov: query corpus", "err", err)
		return nil
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var enc string
		if err := rows.Scan(&enc); err != nil {
			continue
		}
		pt, err := p.decryptText(enc)
		if err != nil {
			// Unencrypted or corrupt row — skip
			continue
		}
		texts = append(texts, pt)
	}
	return texts
}

// corpusCount returns the number of corpus entries for a user.
func corpusCount(userID id.UserID) int {
	d := db.Get()
	var count int
	_ = d.QueryRow(`SELECT COUNT(*) FROM markov_corpus WHERE user_id = ?`, string(userID)).Scan(&count)
	return count
}

// ── Migration ───────────────────────────────────────────────────────────────

// purgeUnencryptedRows detects unencrypted rows (decrypt fails on non-base64) and purges them.
func (p *MarkovPlugin) purgeUnencryptedRows() {
	d := db.Get()
	rows, err := d.Query(`SELECT id, text FROM markov_corpus`)
	if err != nil {
		slog.Error("markov: migration scan", "err", err)
		return
	}
	defer rows.Close()

	var badIDs []interface{}
	for rows.Next() {
		var rowID int64
		var enc string
		if err := rows.Scan(&rowID, &enc); err != nil {
			continue
		}
		_, err := p.decryptText(enc)
		if err != nil {
			badIDs = append(badIDs, rowID)
		}
	}

	if len(badIDs) == 0 {
		return
	}

	slog.Warn("markov: purging unencrypted/corrupt rows", "count", len(badIDs))

	// Delete in batches of 100
	for i := 0; i < len(badIDs); i += 100 {
		end := i + 100
		if end > len(badIDs) {
			end = len(badIDs)
		}
		batch := badIDs[i:end]
		placeholders := strings.Repeat("?,", len(batch))
		placeholders = placeholders[:len(placeholders)-1]
		_, err := d.Exec(
			fmt.Sprintf(`DELETE FROM markov_corpus WHERE id IN (%s)`, placeholders),
			batch...,
		)
		if err != nil {
			slog.Error("markov: migration delete batch", "err", err)
		}
	}
}

// ── TTL purge ───────────────────────────────────────────────────────────────

// MarkovPurgeExpired deletes corpus entries older than 90 days. Intended to be
// called from the nightly scheduled jobs in main.go.
func MarkovPurgeExpired() {
	d := db.Get()
	cutoff := time.Now().UTC().AddDate(0, 0, -90).Unix()
	res, err := d.Exec(`DELETE FROM markov_corpus WHERE created_at < ?`, cutoff)
	if err != nil {
		slog.Error("markov: purge expired", "err", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		slog.Info("markov: purged expired entries", "rows", n)
	}
}

// ── Command handlers ────────────────────────────────────────────────────────

func (p *MarkovPlugin) handleMarkov(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "markov")

	switch {
	case args == "":
		return p.generateForUser(ctx, ctx.Sender, "")
	case args == "me":
		return p.generateForUser(ctx, ctx.Sender, "")
	case strings.HasPrefix(args, "stats"):
		return p.handleStats(ctx, strings.TrimSpace(strings.TrimPrefix(args, "stats")))
	case args == "forget":
		return p.handleForgetSelf(ctx)
	case strings.HasPrefix(args, "forget "):
		return p.handleForgetAdmin(ctx, strings.TrimSpace(strings.TrimPrefix(args, "forget ")))
	case args == "leaderboard":
		return p.handleLeaderboard(ctx)
	default:
		// Could be one user or two users (mashup)
		return p.handleGenerate(ctx, args)
	}
}

func (p *MarkovPlugin) handleImpersonate(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "impersonate")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !impersonate @user")
	}
	resolved, ok := p.ResolveUser(args, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", args))
	}
	return p.generateForUser(ctx, resolved, "")
}

func (p *MarkovPlugin) handleGhostwrite(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "ghostwrite")
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !ghostwrite @user <prompt>")
	}

	resolved, ok := p.ResolveUser(parts[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", parts[0]))
	}

	seed := strings.TrimSpace(parts[1])
	return p.generateForUser(ctx, resolved, seed)
}

// handleGenerate parses one or two user mentions and generates accordingly.
func (p *MarkovPlugin) handleGenerate(ctx MessageContext, args string) error {
	fields := strings.Fields(args)

	if len(fields) == 1 {
		resolved, ok := p.ResolveUser(fields[0], ctx.RoomID)
		if !ok {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", fields[0]))
		}
		return p.generateForUser(ctx, resolved, "")
	}

	if len(fields) == 2 {
		user1, ok1 := p.ResolveUser(fields[0], ctx.RoomID)
		user2, ok2 := p.ResolveUser(fields[1], ctx.RoomID)
		if !ok1 || !ok2 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve one or both users.")
		}
		return p.generateMashup(ctx, user1, user2)
	}

	// More than 2 — cap at 2
	return p.SendReply(ctx.RoomID, ctx.EventID, "Mashup supports at most 2 users.")
}

// generateForUser generates markov text for a single user, optionally seeded.
func (p *MarkovPlugin) generateForUser(ctx MessageContext, userID id.UserID, seed string) error {
	texts := p.loadCorpus(userID)

	if len(texts) < 50 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Not enough data for %s (%d messages, need at least 50).",
				p.DisplayName(userID), len(texts)))
	}

	result := p.generateOutput(texts, seed)
	if result == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to generate Markov text.")
	}

	// Re-roll once if exact corpus match
	for _, t := range texts {
		if result == t {
			result = p.generateOutput(texts, seed)
			break
		}
	}

	if result == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to generate Markov text.")
	}

	name := p.DisplayName(userID)
	msg := fmt.Sprintf("[%s]: %s", name, result)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// generateMashup interleaves corpora of two users and generates.
func (p *MarkovPlugin) generateMashup(ctx MessageContext, user1, user2 id.UserID) error {
	texts1 := p.loadCorpus(user1)
	texts2 := p.loadCorpus(user2)

	if len(texts1) < 50 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Not enough data for %s (%d messages, need at least 50).",
				p.DisplayName(user1), len(texts1)))
	}
	if len(texts2) < 50 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Not enough data for %s (%d messages, need at least 50).",
				p.DisplayName(user2), len(texts2)))
	}

	// Interleave corpora
	combined := make([]string, 0, len(texts1)+len(texts2))
	i, j := 0, 0
	for i < len(texts1) || j < len(texts2) {
		if i < len(texts1) {
			combined = append(combined, texts1[i])
			i++
		}
		if j < len(texts2) {
			combined = append(combined, texts2[j])
			j++
		}
	}

	result := p.generateOutput(combined, "")
	if result == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to generate Markov text.")
	}

	name1 := p.DisplayName(user1)
	name2 := p.DisplayName(user2)
	msg := fmt.Sprintf("[%s × %s]: %s", name1, name2, result)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// generateOutput builds a trigram model and generates 1-3 sentences, max 280 chars.
// If seed is non-empty, tries to start the chain near the seed words.
func (p *MarkovPlugin) generateOutput(texts []string, seed string) string {
	chain, starters := buildTrigramModel(texts)
	if len(starters) == 0 {
		return ""
	}

	var start trigramKey

	if seed != "" {
		// Try to find a starter that begins with/near the seed
		seedWords := strings.Fields(strings.ToLower(seed))
		if len(seedWords) >= 2 {
			// Look for exact bigram match
			target := trigramKey{seedWords[0], seedWords[1]}
			if _, ok := chain[target]; ok {
				start = target
			}
		}
		if start.w1 == "" && len(seedWords) >= 1 {
			// Look for a starter beginning with the first seed word
			seedLower := seedWords[0]
			var candidates []trigramKey
			for _, s := range starters {
				if strings.ToLower(s.w1) == seedLower {
					candidates = append(candidates, s)
				}
			}
			if len(candidates) > 0 {
				start = candidates[rand.Intn(len(candidates))]
			}
		}
	}

	// Fallback to random starter
	if start.w1 == "" {
		start = starters[rand.Intn(len(starters))]
	}

	result := []string{start.w1, start.w2}
	sentences := countSentenceEnds(start.w1) + countSentenceEnds(start.w2)

	for len(result) < 60 {
		key := trigramKey{result[len(result)-2], result[len(result)-1]}
		nextWords, ok := chain[key]
		if !ok || len(nextWords) == 0 {
			break
		}
		next := nextWords[rand.Intn(len(nextWords))]
		result = append(result, next)

		if endsWithPunctuation(next) {
			sentences++
			if sentences >= 3 {
				break
			}
			// After at least 8 words and 1 sentence, chance to stop
			if len(result) > 8 && rand.Float64() < 0.3 {
				break
			}
		}
	}

	output := strings.Join(result, " ")

	// Truncate to 280 chars at sentence boundary
	if len(output) > 280 {
		output = truncateAtSentence(output, 280)
	}

	return output
}

func countSentenceEnds(s string) int {
	n := 0
	for _, c := range s {
		if c == '.' || c == '!' || c == '?' {
			n++
		}
	}
	return n
}

// truncateAtSentence truncates text to maxLen, preferring to cut at a sentence boundary.
func truncateAtSentence(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	sub := text[:maxLen]
	// Find last sentence-ending punctuation
	lastEnd := -1
	for i := len(sub) - 1; i >= 0; i-- {
		if sub[i] == '.' || sub[i] == '!' || sub[i] == '?' {
			lastEnd = i
			break
		}
	}
	if lastEnd > 0 {
		return sub[:lastEnd+1]
	}
	// No sentence boundary — hard truncate at word boundary
	lastSpace := strings.LastIndex(sub, " ")
	if lastSpace > 0 {
		return sub[:lastSpace] + "..."
	}
	return sub
}

// ── Stats ───────────────────────────────────────────────────────────────────

func (p *MarkovPlugin) handleStats(ctx MessageContext, userArg string) error {
	var targetUser id.UserID
	if userArg == "" || userArg == "me" {
		targetUser = ctx.Sender
	} else {
		resolved, ok := p.ResolveUser(userArg, ctx.RoomID)
		if !ok {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", userArg))
		}
		targetUser = resolved
	}

	d := db.Get()

	// Message count
	var msgCount int
	_ = d.QueryRow(`SELECT COUNT(*) FROM markov_corpus WHERE user_id = ?`, string(targetUser)).Scan(&msgCount)

	if msgCount == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s has no Markov data.", p.DisplayName(targetUser)))
	}

	// Earliest date
	var earliest int64
	_ = d.QueryRow(`SELECT MIN(created_at) FROM markov_corpus WHERE user_id = ?`, string(targetUser)).Scan(&earliest)
	earliestDate := time.Unix(earliest, 0).UTC().Format("2006-01-02")

	// Decrypt all to compute word stats
	texts := p.loadCorpus(targetUser)
	wordFreq := make(map[string]int)
	totalWords := 0
	for _, t := range texts {
		for _, w := range strings.Fields(t) {
			w = strings.ToLower(w)
			wordFreq[w]++
			totalWords++
		}
	}

	uniqueWords := len(wordFreq)

	// Top 5 words
	type wordCount struct {
		word  string
		count int
	}
	var top []wordCount
	for w, c := range wordFreq {
		if len(w) < 3 { // skip very short words
			continue
		}
		inserted := false
		for i, tc := range top {
			if c > tc.count {
				top = append(top, wordCount{})
				copy(top[i+1:], top[i:])
				top[i] = wordCount{w, c}
				inserted = true
				break
			}
		}
		if !inserted && len(top) < 5 {
			top = append(top, wordCount{w, c})
		}
		if len(top) > 5 {
			top = top[:5]
		}
	}

	name := p.DisplayName(targetUser)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Markov Stats — %s\n\n", name))
	sb.WriteString(fmt.Sprintf("Messages: %d\n", msgCount))
	sb.WriteString(fmt.Sprintf("Unique words: %d\n", uniqueWords))
	sb.WriteString(fmt.Sprintf("Total words: %d\n", totalWords))
	sb.WriteString(fmt.Sprintf("Since: %s\n", earliestDate))

	if len(top) > 0 {
		sb.WriteString("\nTop words: ")
		for i, tc := range top {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s (%d)", tc.word, tc.count))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nℹ️ Entries older than 90 days are purged automatically.")

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// ── Forget ──────────────────────────────────────────────────────────────────

func (p *MarkovPlugin) handleForgetSelf(ctx MessageContext) error {
	count := corpusCount(ctx.Sender)
	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Your Markov corpus is already empty.")
	}

	d := db.Get()
	_, err := d.Exec(`DELETE FROM markov_corpus WHERE user_id = ?`, string(ctx.Sender))
	if err != nil {
		slog.Error("markov: forget self", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to delete your Markov corpus.")
	}

	_ = p.SendDM(ctx.Sender,
		fmt.Sprintf("Deleted %d entries from your Markov corpus. This cannot be undone.", count))

	return p.SendReply(ctx.RoomID, ctx.EventID, "Your Markov corpus has been deleted.")
}

func (p *MarkovPlugin) handleForgetAdmin(ctx MessageContext, userArg string) error {
	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can delete another user's corpus.")
	}

	resolved, ok := p.ResolveUser(userArg, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", userArg))
	}

	count := corpusCount(resolved)
	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s has no Markov data.", p.DisplayName(resolved)))
	}

	d := db.Get()
	_, err := d.Exec(`DELETE FROM markov_corpus WHERE user_id = ?`, string(resolved))
	if err != nil {
		slog.Error("markov: forget admin", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to delete corpus.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Deleted %d Markov entries for %s.", count, p.DisplayName(resolved)))
}

// ── Leaderboard ─────────────────────────────────────────────────────────────

func (p *MarkovPlugin) handleLeaderboard(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, COUNT(*) as cnt FROM markov_corpus
		 GROUP BY user_id ORDER BY cnt DESC LIMIT 10`,
	)
	if err != nil {
		slog.Error("markov: leaderboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load leaderboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🏆 Markov Leaderboard — Top 10 by Corpus Size\n\n")

	rank := 0
	for rows.Next() {
		var uid string
		var cnt int
		if err := rows.Scan(&uid, &cnt); err != nil {
			continue
		}
		rank++
		name := p.DisplayName(id.UserID(uid))
		sb.WriteString(fmt.Sprintf("%d. %s — %d messages\n", rank, name, cnt))
	}

	if rank == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No Markov data available yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// ── Trigram model ───────────────────────────────────────────────────────────

// trigram key
type trigramKey struct {
	w1, w2 string
}

// buildTrigramModel builds a trigram chain and collects starters from texts.
func buildTrigramModel(texts []string) (map[trigramKey][]string, []trigramKey) {
	chain := make(map[trigramKey][]string)
	var starters []trigramKey

	for _, text := range texts {
		words := strings.Fields(text)
		if len(words) < 3 {
			continue
		}

		starters = append(starters, trigramKey{words[0], words[1]})

		for i := 0; i < len(words)-2; i++ {
			key := trigramKey{words[i], words[i+1]}
			chain[key] = append(chain[key], words[i+2])
		}
	}

	return chain, starters
}

// generateMarkov builds a trigram model from texts and generates output.
// Kept for backward compatibility but delegates to the new model.
func generateMarkov(texts []string, maxWords int) string {
	chain, starters := buildTrigramModel(texts)

	if len(starters) == 0 {
		return ""
	}

	// Pick a random starter
	start := starters[rand.Intn(len(starters))]
	result := []string{start.w1, start.w2}

	for len(result) < maxWords {
		key := trigramKey{result[len(result)-2], result[len(result)-1]}
		nextWords, ok := chain[key]
		if !ok || len(nextWords) == 0 {
			break
		}
		next := nextWords[rand.Intn(len(nextWords))]
		result = append(result, next)

		// Stop at sentence-ending punctuation sometimes
		if len(result) > 8 && endsWithPunctuation(next) && rand.Float64() < 0.3 {
			break
		}
	}

	return strings.Join(result, " ")
}

func endsWithPunctuation(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '.' || last == '!' || last == '?'
}
