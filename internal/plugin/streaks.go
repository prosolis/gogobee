package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// StreaksPlugin tracks daily activity streaks and first-poster records.
type StreaksPlugin struct {
	Base
}

// NewStreaksPlugin creates a new streaks plugin.
func NewStreaksPlugin(client *mautrix.Client) *StreaksPlugin {
	return &StreaksPlugin{
		Base: NewBase(client),
	}
}

func (p *StreaksPlugin) Name() string { return "streaks" }

func (p *StreaksPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "streak", Description: "Show your current and longest activity streak", Usage: "!streak", Category: "Leveling & Stats"},
		{Name: "firstboard", Description: "Show top first-posters of the day", Usage: "!firstboard", Category: "Leveling & Stats"},
	}
}

func (p *StreaksPlugin) Init() error { return nil }

func (p *StreaksPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *StreaksPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "streak") {
		return p.handleStreak(ctx)
	}
	if p.IsCommand(ctx.Body, "firstboard") {
		return p.handleFirstboard(ctx)
	}

	// Skip tracking for bot commands
	if ctx.IsCommand {
		return nil
	}

	// Passive: track daily activity and first poster
	p.trackDailyActivity(ctx)
	p.trackFirstPoster(ctx)

	return nil
}

func (p *StreaksPlugin) trackDailyActivity(ctx MessageContext) {
	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")

	_, err := d.Exec(
		`INSERT INTO daily_activity (user_id, date, message_count) VALUES (?, ?, 1)
		 ON CONFLICT(user_id, date) DO UPDATE SET message_count = message_count + 1`,
		string(ctx.Sender), today,
	)
	if err != nil {
		slog.Error("streaks: track daily activity", "err", err)
	}
}

func (p *StreaksPlugin) trackFirstPoster(ctx MessageContext) {
	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")
	now := time.Now().UTC().Unix()

	// INSERT OR IGNORE — only the first poster for this room+date wins
	_, err := d.Exec(
		`INSERT OR IGNORE INTO daily_first (room_id, date, user_id, timestamp) VALUES (?, ?, ?, ?)`,
		string(ctx.RoomID), today, string(ctx.Sender), now,
	)
	if err != nil {
		slog.Error("streaks: track first poster", "err", err)
	}
}

func (p *StreaksPlugin) handleStreak(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "streak")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()

	// Get all active dates for this user, sorted descending
	rows, err := d.Query(
		`SELECT date FROM daily_activity WHERE user_id = ? ORDER BY date DESC`,
		string(target),
	)
	if err != nil {
		slog.Error("streaks: query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to look up streak.")
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			continue
		}
		dates = append(dates, date)
	}

	if len(dates) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s has no activity recorded yet.", string(target)))
	}

	currentStreak, longestStreak := calculateStreaks(dates)

	msg := fmt.Sprintf(
		"🔥 %s — Streak Report\nCurrent streak: %s days\nLongest streak: %s days\nTotal active days: %s",
		string(target),
		formatNumber(currentStreak),
		formatNumber(longestStreak),
		formatNumber(len(dates)),
	)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// calculateStreaks computes current and longest streaks from descending-sorted date strings.
func calculateStreaks(dates []string) (current int, longest int) {
	if len(dates) == 0 {
		return 0, 0
	}

	today := time.Now().UTC().Format("2006-01-02")

	// Parse first date — current streak only counts if it includes today or yesterday
	firstDate, err := time.Parse("2006-01-02", dates[0])
	if err != nil {
		return 0, 0
	}

	todayDate, _ := time.Parse("2006-01-02", today)
	daysSinceFirst := int(todayDate.Sub(firstDate).Hours() / 24)
	if daysSinceFirst > 1 {
		// Streak is broken — current streak is 0
		// Still calculate longest
		current = 0
	} else {
		// Count current streak
		current = 1
		for i := 1; i < len(dates); i++ {
			prev, err1 := time.Parse("2006-01-02", dates[i-1])
			curr, err2 := time.Parse("2006-01-02", dates[i])
			if err1 != nil || err2 != nil {
				break
			}
			diff := int(prev.Sub(curr).Hours() / 24)
			if diff == 1 {
				current++
			} else {
				break
			}
		}
	}

	// Calculate longest streak (walk all dates)
	streak := 1
	longest = 1
	for i := 1; i < len(dates); i++ {
		prev, err1 := time.Parse("2006-01-02", dates[i-1])
		curr, err2 := time.Parse("2006-01-02", dates[i])
		if err1 != nil || err2 != nil {
			streak = 1
			continue
		}
		diff := int(prev.Sub(curr).Hours() / 24)
		if diff == 1 {
			streak++
		} else {
			streak = 1
		}
		if streak > longest {
			longest = streak
		}
	}

	if current > longest {
		longest = current
	}

	return current, longest
}

func (p *StreaksPlugin) handleFirstboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()

	rows, err := d.Query(
		`SELECT user_id, COUNT(*) as wins
		 FROM daily_first
		 GROUP BY user_id
		 ORDER BY wins DESC`,
	)
	if err != nil {
		slog.Error("streaks: firstboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load first-poster board.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🌅 First Poster Board — Top 10\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var wins int
		if err := rows.Scan(&userID, &wins); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s first posts\n", prefix, userID, formatNumber(wins)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No first-poster data yet.")
	}

	// Also show today's first poster
	today := time.Now().UTC().Format("2006-01-02")
	var todayFirst sql.NullString
	_ = d.QueryRow(
		`SELECT user_id FROM daily_first WHERE room_id = ? AND date = ?`,
		string(ctx.RoomID), today,
	).Scan(&todayFirst)

	if todayFirst.Valid {
		sb.WriteString(fmt.Sprintf("\nToday's first poster in this room: %s", todayFirst.String))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
