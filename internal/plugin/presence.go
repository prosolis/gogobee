package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

// PresencePlugin handles away status and user profile lookups.
type PresencePlugin struct {
	Base
}

// NewPresencePlugin creates a new PresencePlugin.
func NewPresencePlugin(client *mautrix.Client) *PresencePlugin {
	return &PresencePlugin{
		Base: NewBase(client),
	}
}

func (p *PresencePlugin) Name() string { return "presence" }

func (p *PresencePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "away", Description: "Set away status", Usage: "!away [message]", Category: "Personal"},
		{Name: "afk", Description: "Set away status (alias)", Usage: "!afk [message]", Category: "Personal"},
		{Name: "back", Description: "Clear away status", Usage: "!back", Category: "Personal"},
		{Name: "whois", Description: "Show user profile card", Usage: "!whois @user", Category: "Personal"},
	}
}

func (p *PresencePlugin) Init() error { return nil }

func (p *PresencePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *PresencePlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "away"):
		return p.handleAway(ctx, p.GetArgs(ctx.Body, "away"))
	case p.IsCommand(ctx.Body, "afk"):
		return p.handleAway(ctx, p.GetArgs(ctx.Body, "afk"))
	case p.IsCommand(ctx.Body, "back"):
		return p.handleBack(ctx)
	case p.IsCommand(ctx.Body, "whois"):
		return p.handleWhois(ctx)
	default:
		// Auto-clear away status on non-command messages
		return p.autoClearAway(ctx)
	}
}

func (p *PresencePlugin) handleAway(ctx MessageContext, message string) error {
	if message == "" {
		message = "Away"
	}

	now := time.Now().Unix()
	_, err := db.Get().Exec(
		`INSERT INTO presence (user_id, status, message, updated_at)
		 VALUES (?, 'away', ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   status = 'away',
		   message = ?,
		   updated_at = ?`,
		string(ctx.Sender), message, now, message, now,
	)
	if err != nil {
		slog.Error("presence: set away", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to set away status.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("💤 %s is now away: %s", ctx.Sender, message))
}

func (p *PresencePlugin) handleBack(ctx MessageContext) error {
	result, err := db.Get().Exec(
		`UPDATE presence SET status = 'online', message = '', updated_at = ?
		 WHERE user_id = ? AND status = 'away'`,
		time.Now().Unix(), string(ctx.Sender),
	)
	if err != nil {
		slog.Error("presence: set back", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to clear away status.")
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return p.SendMessage(ctx.RoomID, "You weren't marked as away.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("👋 Welcome back, %s!", ctx.Sender))
}

func (p *PresencePlugin) autoClearAway(ctx MessageContext) error {
	// Check if user is away
	var status string
	err := db.Get().QueryRow(
		`SELECT status FROM presence WHERE user_id = ?`,
		string(ctx.Sender),
	).Scan(&status)

	if err != nil || status != "away" {
		return nil
	}

	// Clear away status
	_, err = db.Get().Exec(
		`UPDATE presence SET status = 'online', message = '', updated_at = ?
		 WHERE user_id = ?`,
		time.Now().Unix(), string(ctx.Sender),
	)
	if err != nil {
		slog.Error("presence: auto-clear away", "err", err)
		return nil
	}

	return p.SendMessage(ctx.RoomID, fmt.Sprintf("👋 Welcome back, %s! (auto-cleared away status)", ctx.Sender))
}

func (p *PresencePlugin) handleWhois(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "whois"))
	if args == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !whois <user>")
	}

	targetUser, ok := p.ResolveUser(args)
	if !ok {
		return p.SendMessage(ctx.RoomID, "Could not find a user matching that name.")
	}

	// Gather profile data
	var displayName string
	var xp, level int
	err := db.Get().QueryRow(
		`SELECT display_name, xp, level FROM users WHERE user_id = ?`,
		string(targetUser),
	).Scan(&displayName, &xp, &level)
	if err != nil {
		displayName = string(targetUser)
	}

	// Get stats
	var totalMessages, totalWords int
	_ = db.Get().QueryRow(
		`SELECT total_messages, total_words FROM user_stats WHERE user_id = ?`,
		string(targetUser),
	).Scan(&totalMessages, &totalWords)

	// Get streak
	var currentStreak int
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	var countToday, countYesterday int
	_ = db.Get().QueryRow(
		`SELECT COUNT(*) FROM daily_activity WHERE user_id = ? AND date = ?`,
		string(targetUser), today,
	).Scan(&countToday)
	_ = db.Get().QueryRow(
		`SELECT COUNT(*) FROM daily_activity WHERE user_id = ? AND date = ?`,
		string(targetUser), yesterday,
	).Scan(&countYesterday)
	if countToday > 0 || countYesterday > 0 {
		// Count consecutive days backward
		currentStreak = 0
		checkDate := time.Now()
		for {
			dateStr := checkDate.Format("2006-01-02")
			var cnt int
			err := db.Get().QueryRow(
				`SELECT COUNT(*) FROM daily_activity WHERE user_id = ? AND date = ?`,
				string(targetUser), dateStr,
			).Scan(&cnt)
			if err != nil || cnt == 0 {
				break
			}
			currentStreak++
			checkDate = checkDate.AddDate(0, 0, -1)
		}
	}

	// Get reputation (from XP log with reason = 'reputation')
	var repXP int
	_ = db.Get().QueryRow(
		`SELECT COALESCE(SUM(amount), 0) FROM xp_log WHERE user_id = ? AND reason = 'reputation'`,
		string(targetUser),
	).Scan(&repXP)
	repCount := repXP / 5

	// Get presence status
	var status, statusMsg string
	_ = db.Get().QueryRow(
		`SELECT status, message FROM presence WHERE user_id = ?`,
		string(targetUser),
	).Scan(&status, &statusMsg)

	// Get birthday
	var bdayMonth, bdayDay int
	var timezone string
	hasBirthday := false
	err = db.Get().QueryRow(
		`SELECT month, day, timezone FROM birthdays WHERE user_id = ?`,
		string(targetUser),
	).Scan(&bdayMonth, &bdayDay, &timezone)
	if err == nil {
		hasBirthday = true
	}
	if timezone == "" {
		timezone = "Not set"
	}

	// Build profile card
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 **Profile: %s**\n", displayName))
	sb.WriteString(fmt.Sprintf("  ID: %s\n", targetUser))
	sb.WriteString(fmt.Sprintf("  Level: %d (XP: %d)\n", level, xp))
	sb.WriteString(fmt.Sprintf("  Messages: %d | Words: %d\n", totalMessages, totalWords))
	sb.WriteString(fmt.Sprintf("  Rep: +%d\n", repCount))
	sb.WriteString(fmt.Sprintf("  Streak: %d days\n", currentStreak))
	sb.WriteString(fmt.Sprintf("  Timezone: %s\n", timezone))

	if hasBirthday {
		sb.WriteString(fmt.Sprintf("  Birthday: %s %d\n", time.Month(bdayMonth).String(), bdayDay))
	}

	if status == "away" {
		sb.WriteString(fmt.Sprintf("  Status: 💤 Away — %s\n", statusMsg))
	} else {
		sb.WriteString("  Status: 🟢 Online\n")
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}
