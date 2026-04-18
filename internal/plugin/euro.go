package plugin

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// fmtEuro formats a currency value with thousand separators, e.g. "€12,500".
func fmtEuro[T int | int64 | float64](v T) string {
	n := int64(v)
	if n < 0 {
		return "-" + fmtEuro(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return "€" + s
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return "€" + string(out)
}

// gamesRoom returns the configured GAMES_ROOM, or empty if unset.
func gamesRoom() id.RoomID {
	return id.RoomID(os.Getenv("GAMES_ROOM"))
}

// isGamesRoom checks whether the given room is the games room.
// Returns true if no games room is configured (unrestricted).
func isGamesRoom(roomID id.RoomID) bool {
	gr := gamesRoom()
	return gr == "" || roomID == gr
}

// ---------------------------------------------------------------------------
// Euro config
// ---------------------------------------------------------------------------

type euroConfig struct {
	CooldownSeconds int
	DebtReminder    bool
	StartingCap     float64
}

func loadEuroConfig() euroConfig {
	return euroConfig{
		CooldownSeconds: envInt("EURO_COOLDOWN_SECONDS", 30),
		DebtReminder:    envOrDefault("EURO_DEBT_REMINDER", "true") == "true",
		StartingCap:     envFloat("EURO_STARTING_CAP", 2500),
	}
}

// ---------------------------------------------------------------------------
// Combo system — activity streaks that multiply passive euro earning.
//
// Three independent combo categories: messages, links, images.
// Each combo increments with qualifying activity and resets after 10 min idle.
// Multiplier: 1 + (combo_step * 2.0) — so step 1 = 3x, step 5 = 11x, etc.
// Daily cap per category prevents unbounded farming.
// ---------------------------------------------------------------------------

const (
	comboTimeout     = 10 * time.Minute
	comboMsgCap      = 50
	comboLinkCap     = 10
	comboImageCap    = 15
	comboMultPerStep = 2.0 // +200% per step
	comboLinkBase    = 5.0
	comboImageBase   = 3.0
)

type comboState struct {
	Step    int
	LastAt  time.Time
	Today   string
	DayHits int
}

var urlPattern = regexp.MustCompile(`https?://\S+`)

// ---------------------------------------------------------------------------
// Euro Plugin
// ---------------------------------------------------------------------------

type EuroPlugin struct {
	Base
	cfg       euroConfig
	cooldowns map[id.UserID]time.Time
	combos    map[id.UserID]map[string]*comboState // category → state
	mu        sync.Mutex
}

func NewEuroPlugin(client *mautrix.Client) *EuroPlugin {
	return &EuroPlugin{
		Base:      NewBase(client),
		cfg:       loadEuroConfig(),
		cooldowns: make(map[id.UserID]time.Time),
		combos:    make(map[id.UserID]map[string]*comboState),
	}
}

func (p *EuroPlugin) Name() string    { return "euro" }
func (p *EuroPlugin) Version() string { return "1.1.0" }

func (p *EuroPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "balance", Description: "Check your euro balance", Usage: "!balance", Category: "Economy"},
		{Name: "baltop", Description: "Euro leaderboard", Usage: "!baltop", Category: "Economy"},
		{Name: "baltransfer", Description: "Send euros to another player", Usage: "!baltransfer @user €amount", Category: "Economy"},
		{Name: "combo", Description: "View your current activity combo streaks", Usage: "!combo", Category: "Economy"},
	}
}

func (p *EuroPlugin) Init() error { return nil }

func (p *EuroPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *EuroPlugin) OnMessage(ctx MessageContext) error {
	// Passive euro earning (all rooms, not just games room)
	if !ctx.IsCommand {
		p.awardPassiveEuros(ctx)
	}

	switch {
	case p.IsCommand(ctx.Body, "balance"):
		return p.handleBalance(ctx)
	case p.IsCommand(ctx.Body, "baltop"):
		return p.handleBaltop(ctx)
	case p.IsCommand(ctx.Body, "baltransfer"):
		return p.handleTransfer(ctx)
	case p.IsCommand(ctx.Body, "combo"):
		return p.handleCombo(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Passive earning
// ---------------------------------------------------------------------------

func (p *EuroPlugin) awardPassiveEuros(ctx MessageContext) {
	p.mu.Lock()
	last, ok := p.cooldowns[ctx.Sender]
	now := time.Now()
	if ok && now.Sub(last) < time.Duration(p.cfg.CooldownSeconds)*time.Second {
		p.mu.Unlock()
		return
	}
	p.cooldowns[ctx.Sender] = now
	if len(p.cooldowns) > 1000 {
		for uid, t := range p.cooldowns {
			if now.Sub(t) > time.Duration(p.cfg.CooldownSeconds)*time.Second {
				delete(p.cooldowns, uid)
			}
		}
	}
	p.mu.Unlock()

	p.ensureBalance(ctx.Sender)

	// Message combo — base amount from word count, multiplied by combo.
	words := len(strings.Fields(ctx.Body))
	var baseMsg float64
	switch {
	case words >= 51:
		baseMsg = 20.00
	case words >= 26:
		baseMsg = 10.00
	case words >= 11:
		baseMsg = 5.00
	case words >= 4:
		baseMsg = 2.50
	default:
		baseMsg = 1.00
	}
	msgMult := p.advanceCombo(ctx.Sender, "message", comboMsgCap)
	p.credit(ctx.Sender, baseMsg*msgMult, "message")

	// Link combo — bonus per URL in the message.
	if urls := urlPattern.FindAllString(ctx.Body, -1); len(urls) > 0 {
		linkMult := p.advanceCombo(ctx.Sender, "link", comboLinkCap)
		p.credit(ctx.Sender, comboLinkBase*linkMult*float64(len(urls)), "link_combo")
	}

	// Image combo — bonus for image/video/file attachments.
	if mc := ctx.Event.Content.AsMessage(); mc != nil {
		if mc.MsgType == event.MsgImage || mc.MsgType == event.MsgVideo || mc.MsgType == event.MsgFile {
			imgMult := p.advanceCombo(ctx.Sender, "image", comboImageCap)
			p.credit(ctx.Sender, comboImageBase*imgMult, "image_combo")
		}
	}
}

// advanceCombo increments a user's combo for a category, resets if timed out
// or daily cap reached, and returns the current multiplier.
func (p *EuroPlugin) advanceCombo(userID id.UserID, category string, dailyCap int) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	today := now.UTC().Format("2006-01-02")

	userCombos, ok := p.combos[userID]
	if !ok {
		userCombos = make(map[string]*comboState)
		p.combos[userID] = userCombos
	}

	cs, ok := userCombos[category]
	if !ok {
		cs = &comboState{}
		userCombos[category] = cs
	}

	// Reset on new day.
	if cs.Today != today {
		cs.Step = 0
		cs.DayHits = 0
		cs.Today = today
	}

	// Reset if combo timed out.
	if !cs.LastAt.IsZero() && now.Sub(cs.LastAt) > comboTimeout {
		cs.Step = 0
	}

	// At daily cap, return base multiplier (no combo bonus).
	if cs.DayHits >= dailyCap {
		return 1.0
	}

	cs.DayHits++
	cs.Step++
	cs.LastAt = now

	return 1.0 + float64(cs.Step)*comboMultPerStep
}

// ---------------------------------------------------------------------------
// !combo command
// ---------------------------------------------------------------------------

func (p *EuroPlugin) handleCombo(ctx MessageContext) error {
	p.mu.Lock()
	userCombos := p.combos[ctx.Sender]
	now := time.Now()
	today := now.UTC().Format("2006-01-02")

	type info struct {
		name    string
		step    int
		dayHits int
		cap     int
		active  bool
	}
	cats := []info{
		{"Messages", 0, 0, comboMsgCap, false},
		{"Links", 0, 0, comboLinkCap, false},
		{"Images", 0, 0, comboImageCap, false},
	}
	keys := []string{"message", "link", "image"}

	if userCombos != nil {
		for i, k := range keys {
			if cs, ok := userCombos[k]; ok && cs.Today == today {
				cats[i].step = cs.Step
				cats[i].dayHits = cs.DayHits
				cats[i].active = !cs.LastAt.IsZero() && now.Sub(cs.LastAt) < comboTimeout
			}
		}
	}
	p.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("🔥 **Activity Combo**\n\n")

	for _, c := range cats {
		mult := 1.0 + float64(c.step)*comboMultPerStep
		status := "💤 inactive"
		if c.active {
			status = fmt.Sprintf("⚡ **%.0fx** multiplier", mult)
		} else if c.step > 0 {
			status = "⏳ expired (resets on next activity)"
		}
		sb.WriteString(fmt.Sprintf("**%s**: step %d/%d — %s\n", c.name, c.dayHits, c.cap, status))
	}

	sb.WriteString(fmt.Sprintf("\nCombo grows +%.0f%% per step. Resets after %d min idle.", comboMultPerStep*100, int(comboTimeout.Minutes())))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// ---------------------------------------------------------------------------
// Balance management
// ---------------------------------------------------------------------------

// ensureBalance creates a balance row if none exists, seeding from corpus.
// Uses INSERT OR IGNORE + RowsAffected to avoid duplicate starting_balance logs.
func (p *EuroPlugin) ensureBalance(userID id.UserID) {
	d := db.Get()

	// Calculate starting balance from corpus character count
	var totalChars float64
	if err := d.QueryRow("SELECT COALESCE(SUM(total_chars), 0) FROM user_stats WHERE user_id = ?",
		string(userID)).Scan(&totalChars); err != nil {
		totalChars = 0
	}

	starting := totalChars / 1000.0
	if starting > p.cfg.StartingCap {
		starting = p.cfg.StartingCap
	}

	result, err := d.Exec(
		"INSERT OR IGNORE INTO euro_balances (user_id, balance) VALUES (?, ?)",
		string(userID), starting,
	)
	if err != nil {
		slog.Error("euro: failed to create balance", "user", userID, "err", err)
		return
	}

	// Only log transaction if a row was actually inserted (not ignored)
	affected, _ := result.RowsAffected()
	if affected > 0 && starting > 0 {
		p.logTransaction(userID, starting, "starting_balance")
	}
}

func (p *EuroPlugin) credit(userID id.UserID, amount float64, reason string) {
	d := db.Get()
	_, err := d.Exec(
		"UPDATE euro_balances SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?",
		amount, string(userID),
	)
	if err != nil {
		slog.Error("euro: credit failed", "user", userID, "amount", amount, "err", err)
		return
	}
	p.logTransaction(userID, amount, reason)
}

// Debit subtracts euros atomically. Returns false if this would exceed debt limit.
// Uses a conditional UPDATE to prevent race conditions (check-and-act in one statement).
func (p *EuroPlugin) Debit(userID id.UserID, amount float64, reason string) bool {
	if amount <= 0 {
		slog.Warn("euro: Debit called with non-positive amount", "user", userID, "amount", amount)
		return false
	}
	p.ensureBalance(userID)
	d := db.Get()

	debtLimit := envFloat("BLACKJACK_DEBT_LIMIT", 1000)

	// Atomic: only debit if the result stays within debt limit
	result, err := d.Exec(
		`UPDATE euro_balances SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND (balance - ?) >= ?`,
		amount, string(userID), amount, -debtLimit,
	)
	if err != nil {
		slog.Error("euro: debit failed", "user", userID, "amount", amount, "err", err)
		return false
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false // balance check failed or user doesn't exist
	}

	p.logTransaction(userID, -amount, reason)
	return true
}

// Credit adds euros (exported for other plugins).
func (p *EuroPlugin) Credit(userID id.UserID, amount float64, reason string) {
	if amount <= 0 {
		slog.Warn("euro: Credit called with non-positive amount", "user", userID, "amount", amount)
		return
	}
	p.ensureBalance(userID)
	p.credit(userID, amount, reason)
}

// GetBalance returns current balance for a user.
func (p *EuroPlugin) GetBalance(userID id.UserID) float64 {
	p.ensureBalance(userID)
	d := db.Get()
	var balance float64
	if err := d.QueryRow("SELECT balance FROM euro_balances WHERE user_id = ?",
		string(userID)).Scan(&balance); err != nil {
		slog.Error("euro: failed to get balance", "user", userID, "err", err)
	}
	return balance
}

func (p *EuroPlugin) logTransaction(userID id.UserID, amount float64, reason string) {
	db.Exec("euro: log transaction",
		"INSERT INTO euro_transactions (user_id, amount, reason) VALUES (?, ?, ?)",
		string(userID), amount, reason,
	)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (p *EuroPlugin) handleBalance(ctx MessageContext) error {
	p.ensureBalance(ctx.Sender)
	d := db.Get()

	var balance float64
	if err := d.QueryRow("SELECT balance FROM euro_balances WHERE user_id = ?",
		string(ctx.Sender)).Scan(&balance); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch balance.")
	}

	debtLimit := envFloat("BLACKJACK_DEBT_LIMIT", 1000)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💰 **Your Balance:** €%d\n", int(balance)))

	if balance < 0 {
		sb.WriteString(fmt.Sprintf("⚠️ You are in debt! (limit: €%d)\n", int(debtLimit)))
		if balance <= -debtLimit {
			sb.WriteString("🚫 Betting disabled until you earn your way out of debt.\n")
		}
	}

	// Recent transactions
	rows, err := d.Query(
		`SELECT amount, reason, created_at FROM euro_transactions
		 WHERE user_id = ? ORDER BY created_at DESC LIMIT 5`,
		string(ctx.Sender),
	)
	if err == nil {
		defer rows.Close()
		sb.WriteString("\n**Recent transactions:**\n")
		for rows.Next() {
			var amount float64
			var reason, createdAt string
			rows.Scan(&amount, &reason, &createdAt)
			sign := "+"
			if amount < 0 {
				sign = ""
			}
			sb.WriteString(fmt.Sprintf("  %s€%.0f — %s\n", sign, amount, reason))
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *EuroPlugin) handleBaltop(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, balance FROM euro_balances ORDER BY balance DESC LIMIT 10`,
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch leaderboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("💰 **Euro Leaderboard**\n\n")
	rank := 0
	for rows.Next() {
		var userID string
		var balance float64
		rows.Scan(&userID, &balance)
		rank++
		name := p.DisplayName(id.UserID(userID))
		medal := ""
		switch rank {
		case 1:
			medal = "🥇"
		case 2:
			medal = "🥈"
		case 3:
			medal = "🥉"
		}
		sb.WriteString(fmt.Sprintf("%s %d. **%s** — €%d\n", medal, rank, name, int(balance)))
	}

	if rank == 0 {
		sb.WriteString("No balances yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *EuroPlugin) handleTransfer(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "baltransfer")
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!baltransfer @user €amount`")
	}

	targetID, ok := p.ResolveUser(parts[0], ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not resolve user.")
	}
	if targetID == ctx.Sender {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You can't transfer to yourself.")
	}

	amountStr := strings.TrimPrefix(parts[1], "€")
	amount := 0.0
	fmt.Sscanf(amountStr, "%f", &amount)
	if amount < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Minimum transfer is €1.")
	}

	balance := p.GetBalance(ctx.Sender)
	if balance < amount {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Insufficient balance. You have €%d.", int(balance)))
	}

	if !p.Debit(ctx.Sender, amount, fmt.Sprintf("transfer to %s", targetID)) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Transfer failed.")
	}
	p.Credit(targetID, amount, fmt.Sprintf("transfer from %s", ctx.Sender))

	senderName := p.DisplayName(ctx.Sender)
	targetName := p.DisplayName(targetID)
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("💸 **%s** sent €%d to **%s**.", senderName, int(amount), targetName))
}

