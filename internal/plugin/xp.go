package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/util"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// levelUpMessages are Twinbee/Parodius themed congratulations.
var levelUpMessages = []string{
	"Power-up collected! %s just reached Level %d! 🔔",
	"Twinbee's bell is ringing! %s ascended to Level %d! 🛎️",
	"Parodius would be proud — %s hit Level %d! 🐙",
	"Stage clear! %s leveled up to Level %d! ⭐",
	"Winbee sends sparkles — %s is now Level %d! ✨",
	"Bonus round complete! %s reached Level %d! 🎰",
	"GwinBee detected a power surge — %s is Level %d! ⚡",
	"The Vic Viper salutes %s for reaching Level %d! 🚀",
	"Takosuke is dancing! %s just became Level %d! 🎶",
	"A shower of bells for %s — Level %d unlocked! 🔔🔔🔔",
	"Pastel confirms: %s has broken through to Level %d! 🌈",
	"The Shooting Star squadron welcomes %s to Level %d! 💫",
}

// XPPlugin awards XP for messages and tracks levels.
type XPPlugin struct {
	Base
	mu        sync.Mutex
	cooldowns map[id.UserID]time.Time
}

// NewXPPlugin creates a new XP plugin.
func NewXPPlugin(client *mautrix.Client) *XPPlugin {
	return &XPPlugin{
		Base:      NewBase(client),
		cooldowns: make(map[id.UserID]time.Time),
	}
}

func (p *XPPlugin) Name() string { return "xp" }

func (p *XPPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "rank", Description: "Show your level, XP, and progress", Usage: "!rank [@user]", Category: "Leveling & Stats"},
		{Name: "leaderboard", Description: "Show top 10 users by XP", Usage: "!leaderboard", Category: "Leveling & Stats"},
	}
}

func (p *XPPlugin) Init() error { return nil }

func (p *XPPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *XPPlugin) OnMessage(ctx MessageContext) error {
	// Handle commands
	if p.IsCommand(ctx.Body, "rank") {
		return p.handleRank(ctx)
	}
	if p.IsCommand(ctx.Body, "leaderboard") {
		return p.handleLeaderboard(ctx)
	}

	// Skip XP for commands
	if ctx.IsCommand {
		return nil
	}

	// Passive XP with cooldown
	p.mu.Lock()
	last, ok := p.cooldowns[ctx.Sender]
	now := time.Now().UTC()
	if ok && now.Sub(last) < 30*time.Second {
		p.mu.Unlock()
		return nil
	}
	p.cooldowns[ctx.Sender] = now
	// Periodically clean up expired cooldowns to prevent unbounded growth
	if len(p.cooldowns) > 1000 {
		for uid, t := range p.cooldowns {
			if now.Sub(t) > time.Minute {
				delete(p.cooldowns, uid)
			}
		}
	}
	p.mu.Unlock()

	_, leveledUp, newLevel := p.grantXPInternal(ctx.Sender, 10, "message")
	if leveledUp {
		msg := levelUpMessages[rand.Intn(len(levelUpMessages))]
		announcement := fmt.Sprintf(msg, string(ctx.Sender), newLevel)
		if err := p.SendMessage(ctx.RoomID, announcement); err != nil {
			slog.Error("failed to send level-up announcement", "err", err)
		}
	}

	return nil
}

// GrantXP awards XP to a user from an external plugin and returns (newXP, leveledUp, newLevel).
func (p *XPPlugin) GrantXP(userID id.UserID, amount int, reason string) (int, bool, int) {
	return p.grantXPInternal(userID, amount, reason)
}

func (p *XPPlugin) grantXPInternal(userID id.UserID, amount int, reason string) (int, bool, int) {
	d := db.Get()
	now := time.Now().UTC().Unix()

	// Ensure user row exists
	_, err := d.Exec(
		`INSERT INTO users (user_id, xp, level, last_xp_at) VALUES (?, 0, 0, 0)
		 ON CONFLICT(user_id) DO NOTHING`, string(userID))
	if err != nil {
		slog.Error("xp: ensure user", "err", err)
		return 0, false, 0
	}

	// Get current state
	var oldXP, oldLevel int
	err = d.QueryRow(`SELECT xp, level FROM users WHERE user_id = ?`, string(userID)).Scan(&oldXP, &oldLevel)
	if err != nil {
		slog.Error("xp: read user", "err", err)
		return 0, false, 0
	}

	newXP := oldXP + amount
	newLevel := util.LevelFromXP(newXP)
	leveledUp := newLevel > oldLevel

	_, err = d.Exec(
		`UPDATE users SET xp = ?, level = ?, last_xp_at = ? WHERE user_id = ?`,
		newXP, newLevel, now, string(userID))
	if err != nil {
		slog.Error("xp: update user", "err", err)
		return oldXP, false, oldLevel
	}

	// Log the XP grant
	_, err = d.Exec(
		`INSERT INTO xp_log (user_id, amount, reason) VALUES (?, ?, ?)`,
		string(userID), amount, reason)
	if err != nil {
		slog.Error("xp: log grant", "err", err)
	}

	return newXP, leveledUp, newLevel
}

func (p *XPPlugin) handleRank(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "rank")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var xp, level int
	err := d.QueryRow(`SELECT xp, level FROM users WHERE user_id = ?`, string(target)).Scan(&xp, &level)
	if err == sql.ErrNoRows {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s hasn't earned any XP yet.", string(target)))
	}
	if err != nil {
		slog.Error("xp: rank query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to look up rank.")
	}

	nextLevel := level + 1
	xpForNext := util.XPForLevel(nextLevel)
	xpForCurrent := util.XPForLevel(level)
	progress := xp - xpForCurrent
	needed := xpForNext - xpForCurrent
	bar := util.ProgressBar(progress, needed, 20)

	// Get rank position (users with same XP share the same rank)
	var rank int
	err = d.QueryRow(`SELECT COUNT(DISTINCT xp) + 1 FROM users WHERE xp > ?`, xp).Scan(&rank)
	if err != nil {
		rank = 0
	}

	msg := fmt.Sprintf(
		"📊 %s — Level %d (Rank #%s)\nXP: %s / %s\n%s",
		string(target), level, formatNumber(rank),
		formatNumber(xp), formatNumber(xpForNext), bar,
	)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *XPPlugin) handleLeaderboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	rows, err := d.Query(`SELECT user_id, xp, level FROM users ORDER BY xp DESC`)
	if err != nil {
		slog.Error("xp: leaderboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load leaderboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🏆 XP Leaderboard — Top 10\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var xp, level int
		if err := rows.Scan(&userID, &xp, &level); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — Level %d (%s XP)\n", prefix, userID, level, formatNumber(xp)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No XP data yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// formatNumber adds commas to an integer for display.
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
