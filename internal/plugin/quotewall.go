package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/crypto"
	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// QuoteWallPlugin saves and retrieves encrypted quotes from chat messages.
type QuoteWallPlugin struct {
	Base
	rate    *RateLimitsPlugin
	encKey  []byte
	enabled bool
}

// NewQuoteWallPlugin creates a new quote wall plugin.
func NewQuoteWallPlugin(client *mautrix.Client, rate *RateLimitsPlugin) *QuoteWallPlugin {
	p := &QuoteWallPlugin{
		Base: NewBase(client),
		rate: rate,
	}

	raw := os.Getenv("QUOTE_ENCRYPTION_KEY")
	if raw == "" {
		slog.Warn("quotewall: disabled (QUOTE_ENCRYPTION_KEY not set)")
		p.enabled = false
		return p
	}

	key, err := crypto.ParseKey(raw)
	if err != nil {
		slog.Error("quotewall: invalid encryption key", "err", err)
		p.enabled = false
		return p
	}

	p.encKey = key
	p.enabled = true
	return p
}

func (p *QuoteWallPlugin) Name() string { return "quotewall" }

func (p *QuoteWallPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "quote", Description: "Save or retrieve quotes", Usage: "!quote [random|search|@user|delete]", Category: "Community"},
		{Name: "quoteboard", Description: "Top 5 most-quoted members", Usage: "!quoteboard", Category: "Community"},
	}
}

func (p *QuoteWallPlugin) Init() error { return nil }

func (p *QuoteWallPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *QuoteWallPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "quoteboard") {
		return p.handleQuoteboard(ctx)
	}
	if p.IsCommand(ctx.Body, "quote") {
		return p.handleQuote(ctx)
	}
	return nil
}

func (p *QuoteWallPlugin) handleQuote(ctx MessageContext) error {
	if !p.enabled {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Quote wall is not configured.")
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "quote"))

	// Check if this is a reply to another message
	content := ctx.Event.Content.AsMessage()
	if content != nil && content.RelatesTo != nil && content.RelatesTo.InReplyTo != nil && args == "" {
		return p.handleQuoteSaveReply(ctx, content.RelatesTo.InReplyTo.EventID)
	}

	// !quote delete [id]
	argsLower := strings.ToLower(args)
	if argsLower == "delete" || strings.HasPrefix(argsLower, "delete ") {
		return p.handleQuoteDelete(ctx, strings.TrimSpace(args[6:]))
	}

	// !quote search [keyword]
	if argsLower == "search" || strings.HasPrefix(argsLower, "search ") {
		keyword := strings.TrimSpace(args[6:])
		return p.handleQuoteSearch(ctx, keyword)
	}

	// !quote random or !quote (no args, no reply)
	if args == "" || strings.EqualFold(args, "random") {
		return p.handleQuoteRandom(ctx)
	}

	// !quote "text" -- @user (manual save)
	if strings.Contains(args, " -- ") {
		return p.handleQuoteManualSave(ctx, args)
	}

	// !quote @user — random quote from a specific user
	return p.handleQuoteByUser(ctx, args)
}

// handleQuoteSaveReply saves a quote from a replied-to message.
func (p *QuoteWallPlugin) handleQuoteSaveReply(ctx MessageContext, replyEventID id.EventID) error {
	if p.rate != nil && !p.rate.CheckLimit(ctx.Sender, "quote_save", 50) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the daily quote save limit.")
	}

	evt, err := p.Client.GetEvent(context.Background(), ctx.RoomID, replyEventID)
	if err != nil {
		slog.Error("quotewall: fetch reply event", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch the original message.")
	}

	// Decrypt if the event is encrypted
	if evt.Type == event.EventEncrypted {
		if p.Client.Crypto != nil {
			if parseErr := evt.Content.ParseRaw(evt.Type); parseErr != nil {
				slog.Error("quotewall: parse encrypted event", "err", parseErr)
				return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to parse the encrypted message.")
			}
			decrypted, decErr := p.Client.Crypto.Decrypt(context.Background(), evt)
			if decErr != nil {
				slog.Error("quotewall: decrypt reply event", "err", decErr)
				return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to decrypt the original message.")
			}
			evt = decrypted
		} else {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Cannot read encrypted messages without crypto support.")
		}
	} else {
		if parseErr := evt.Content.ParseRaw(evt.Type); parseErr != nil {
			slog.Error("quotewall: parse reply event content", "err", parseErr)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to parse the original message.")
		}
	}

	msgContent := evt.Content.AsMessage()
	if msgContent == nil || msgContent.Body == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "That message has been redacted and can't be saved.")
	}

	quoteText := msgContent.Body
	attributedTo := string(evt.Sender)

	return p.saveQuote(ctx, quoteText, attributedTo, "")
}

// handleQuoteManualSave parses `"text" -- @user` and saves.
func (p *QuoteWallPlugin) handleQuoteManualSave(ctx MessageContext, args string) error {
	if p.rate != nil && !p.rate.CheckLimit(ctx.Sender, "quote_save", 50) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the daily quote save limit.")
	}

	parts := strings.SplitN(args, " -- ", 2)
	if len(parts) != 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !quote \"text\" -- @user")
	}

	quoteText := strings.TrimSpace(parts[0])
	quoteText = strings.TrimPrefix(quoteText, "\"")
	quoteText = strings.TrimSuffix(quoteText, "\"")
	if quoteText == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Quote text cannot be empty.")
	}

	userArg := strings.TrimSpace(parts[1])
	resolvedUser, ok := p.ResolveUser(userArg, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve the attributed user.")
	}

	return p.saveQuote(ctx, quoteText, string(resolvedUser), "")
}

// saveQuote encrypts and inserts a quote, handling dedup and reactions.
func (p *QuoteWallPlugin) saveQuote(ctx MessageContext, quoteText, attributedTo, quoteContext string) error {
	hmacVal := crypto.HMAC(p.encKey, []byte(quoteText))

	d := db.Get()

	// Check for duplicate
	var existing int
	err := d.QueryRow(`SELECT 1 FROM quotes WHERE content_hmac = ?`, hmacVal).Scan(&existing)
	if err == nil {
		// Duplicate found
		_ = p.SendReact(ctx.RoomID, ctx.EventID, "\U0001f501") // 🔁
		return nil
	}

	encText, err := crypto.Encrypt(p.encKey, []byte(quoteText))
	if err != nil {
		slog.Error("quotewall: encrypt quote_text", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to encrypt quote.")
	}

	encAttr, err := crypto.Encrypt(p.encKey, []byte(attributedTo))
	if err != nil {
		slog.Error("quotewall: encrypt attributed_to", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to encrypt quote.")
	}

	var encCtx []byte
	if quoteContext != "" {
		encCtx, err = crypto.Encrypt(p.encKey, []byte(quoteContext))
		if err != nil {
			slog.Error("quotewall: encrypt context", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to encrypt quote.")
		}
	}

	_, err = d.Exec(
		`INSERT INTO quotes (room_id, submitted_by, content_hmac, quote_text, attributed_to, context)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(ctx.RoomID), string(ctx.Sender), hmacVal, encText, encAttr, encCtx,
	)
	if err != nil {
		slog.Error("quotewall: insert quote", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save quote.")
	}

	_ = p.SendReact(ctx.RoomID, ctx.EventID, "\U0001f4cc") // 📌

	// Invalidate quoteboard cache for this room
	db.CacheSet("quoteboard:"+string(ctx.RoomID), "")

	return nil
}

// handleQuoteRandom retrieves a random quote from the room.
func (p *QuoteWallPlugin) handleQuoteRandom(ctx MessageContext) error {
	if p.rate != nil && !p.rate.CheckLimit(ctx.Sender, "quote_read", 30) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the daily quote read limit.")
	}

	d := db.Get()
	var qID int
	var encText, encAttr []byte
	var encCtx []byte
	var submittedBy, savedAt string

	err := d.QueryRow(
		`SELECT id, quote_text, attributed_to, context, submitted_by, saved_at
		 FROM quotes WHERE room_id = ? ORDER BY RANDOM() LIMIT 1`,
		string(ctx.RoomID),
	).Scan(&qID, &encText, &encAttr, &encCtx, &submittedBy, &savedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return p.SendReply(ctx.RoomID, ctx.EventID, "No quotes saved in this room yet.")
		}
		slog.Error("quotewall: query random", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to retrieve quote.")
	}

	return p.displayQuote(ctx, qID, encText, encAttr, encCtx, submittedBy, savedAt, "")
}

// handleQuoteByUser retrieves a random quote attributed to a specific user.
func (p *QuoteWallPlugin) handleQuoteByUser(ctx MessageContext, userArg string) error {
	if p.rate != nil && !p.rate.CheckLimit(ctx.Sender, "quote_read", 30) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the daily quote read limit.")
	}

	resolvedUser, ok := p.ResolveUser(userArg, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve that user.")
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT id, quote_text, attributed_to, context, submitted_by, saved_at
		 FROM quotes WHERE room_id = ?`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("quotewall: query by user", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search quotes.")
	}
	defer rows.Close()

	type quoteRow struct {
		id          int
		encText     []byte
		encAttr     []byte
		encCtx      []byte
		submittedBy string
		savedAt     string
	}

	var matches []quoteRow
	targetStr := string(resolvedUser)

	for rows.Next() {
		var q quoteRow
		if err := rows.Scan(&q.id, &q.encText, &q.encAttr, &q.encCtx, &q.submittedBy, &q.savedAt); err != nil {
			continue
		}
		attr, err := crypto.Decrypt(p.encKey, q.encAttr)
		if err != nil {
			continue
		}
		if string(attr) == targetStr {
			matches = append(matches, q)
		}
	}

	if len(matches) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No quotes found for that user in this room.")
	}

	// Pick a random match
	q := matches[rand.Intn(len(matches))]
	return p.displayQuote(ctx, q.id, q.encText, q.encAttr, q.encCtx, q.submittedBy, q.savedAt, "")
}

// handleQuoteSearch searches quotes by keyword.
func (p *QuoteWallPlugin) handleQuoteSearch(ctx MessageContext, keyword string) error {
	if keyword == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !quote search [keyword]")
	}

	if p.rate != nil && !p.rate.CheckLimit(ctx.Sender, "quote_read", 30) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the daily quote read limit.")
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT id, quote_text, attributed_to, context, submitted_by, saved_at
		 FROM quotes WHERE room_id = ?`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("quotewall: query search", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search quotes.")
	}
	defer rows.Close()

	type quoteRow struct {
		id          int
		encText     []byte
		encAttr     []byte
		encCtx      []byte
		submittedBy string
		savedAt     string
	}

	keywordLower := strings.ToLower(keyword)
	var matches []quoteRow

	for rows.Next() {
		var q quoteRow
		if err := rows.Scan(&q.id, &q.encText, &q.encAttr, &q.encCtx, &q.submittedBy, &q.savedAt); err != nil {
			continue
		}
		plaintext, err := crypto.Decrypt(p.encKey, q.encText)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(plaintext)), keywordLower) {
			matches = append(matches, q)
		}
	}

	if len(matches) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No quotes matching '%s' found.", keyword))
	}

	q := matches[rand.Intn(len(matches))]
	prefix := fmt.Sprintf("%d matches found, showing one:\n\n", len(matches))
	return p.displayQuote(ctx, q.id, q.encText, q.encAttr, q.encCtx, q.submittedBy, q.savedAt, prefix)
}

// handleQuoteDelete deletes a quote by ID (admin only).
func (p *QuoteWallPlugin) handleQuoteDelete(ctx MessageContext, idStr string) error {
	if !p.enabled {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Quote wall is not configured.")
	}

	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can delete quotes.")
	}

	quoteID, err := strconv.Atoi(strings.TrimSpace(idStr))
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !quote delete [id]")
	}

	d := db.Get()
	res, err := d.Exec(`DELETE FROM quotes WHERE id = ? AND room_id = ?`, quoteID, string(ctx.RoomID))
	if err != nil {
		slog.Error("quotewall: delete", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to delete quote.")
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Quote not found.")
	}

	// Invalidate quoteboard cache
	db.CacheSet("quoteboard:"+string(ctx.RoomID), "")

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Quote #%d deleted.", quoteID))
}

// handleQuoteboard shows the top 5 most-quoted members.
func (p *QuoteWallPlugin) handleQuoteboard(ctx MessageContext) error {
	if !p.enabled {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Quote wall is not configured.")
	}

	// Check cache first
	cacheKey := "quoteboard:" + string(ctx.RoomID)
	if cached := db.CacheGet(cacheKey, 300); cached != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, cached)
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT attributed_to FROM quotes WHERE room_id = ?`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("quotewall: quoteboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load quote board.")
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var encAttr []byte
		if err := rows.Scan(&encAttr); err != nil {
			continue
		}
		attr, err := crypto.Decrypt(p.encKey, encAttr)
		if err != nil {
			continue
		}
		counts[string(attr)]++
	}

	if len(counts) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No quotes saved in this room yet.")
	}

	// Sort by count descending
	type entry struct {
		user  string
		count int
	}
	var sorted []entry
	for u, c := range counts {
		sorted = append(sorted, entry{u, c})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	medals := []string{"\U0001f947", "\U0001f948", "\U0001f949"} // 🥇🥈🥉
	var sb strings.Builder
	sb.WriteString("\U0001f4cc Quote Board — Top 5\n\n")

	limit := 5
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for i := 0; i < limit; i++ {
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %d quotes\n", prefix, sorted[i].user, sorted[i].count))
	}

	result := sb.String()
	db.CacheSet(cacheKey, result)

	return p.SendReply(ctx.RoomID, ctx.EventID, result)
}

// displayQuote decrypts and formats a quote for display.
func (p *QuoteWallPlugin) displayQuote(ctx MessageContext, qID int, encText, encAttr, encCtx []byte, submittedBy, savedAt, prefix string) error {
	plainText, err := crypto.Decrypt(p.encKey, encText)
	if err != nil {
		slog.Error("quotewall: decrypt quote_text", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to decrypt quote.")
	}

	plainAttr, err := crypto.Decrypt(p.encKey, encAttr)
	if err != nil {
		slog.Error("quotewall: decrypt attributed_to", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to decrypt quote.")
	}

	// Parse saved_at for display
	var dateStr string
	t, err := time.Parse("2006-01-02 15:04:05", savedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, savedAt)
	}
	if err == nil {
		dateStr = t.Format("January 2, 2006")
	} else {
		dateStr = savedAt
	}

	var sb strings.Builder
	if prefix != "" {
		sb.WriteString(prefix)
	}
	sb.WriteString(fmt.Sprintf("\U0001f4cc Quote #%d\n", qID))
	sb.WriteString(fmt.Sprintf("\"%s\"\n", string(plainText)))
	sb.WriteString(fmt.Sprintf("   -- %s \u2022 %s\n", string(plainAttr), dateStr))

	// Decrypt and show context if present
	if len(encCtx) > 0 {
		plainCtx, err := crypto.Decrypt(p.encKey, encCtx)
		if err == nil && len(plainCtx) > 0 {
			sb.WriteString(fmt.Sprintf("   Context: %s\n", string(plainCtx)))
		}
	}

	sb.WriteString(fmt.Sprintf("   (saved by %s)", submittedBy))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
