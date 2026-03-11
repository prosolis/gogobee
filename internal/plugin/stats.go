package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/util"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

var milestoneThresholds = []int{
	1_000, 5_000, 10_000, 25_000, 50_000, 100_000, 250_000, 500_000, 1_000_000,
}

// StatsPlugin passively tracks message statistics and provides query commands.
type StatsPlugin struct {
	Base
}

// NewStatsPlugin creates a new stats plugin.
func NewStatsPlugin(client *mautrix.Client) *StatsPlugin {
	return &StatsPlugin{
		Base: NewBase(client),
	}
}

func (p *StatsPlugin) Name() string { return "stats" }

func (p *StatsPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "stats", Description: "Show message statistics for a user", Usage: "!stats [@user]", Category: "Leveling & Stats"},
		{Name: "rankings", Description: "Show rankings for a stat category", Usage: "!rankings [words|links|questions|emojis]", Category: "Leveling & Stats"},
		{Name: "personality", Description: "Show your chat personality archetype", Usage: "!personality", Category: "Leveling & Stats"},
	}
}

func (p *StatsPlugin) Init() error { return nil }

func (p *StatsPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *StatsPlugin) OnMessage(ctx MessageContext) error {
	// Handle commands first
	if p.IsCommand(ctx.Body, "stats") {
		return p.handleStats(ctx)
	}
	if p.IsCommand(ctx.Body, "rankings") {
		return p.handleRankings(ctx)
	}
	if p.IsCommand(ctx.Body, "personality") {
		return p.handlePersonality(ctx)
	}

	// Skip tracking for bot commands
	if ctx.IsCommand {
		return nil
	}

	// Passive: track message stats
	return p.trackMessage(ctx)
}

func (p *StatsPlugin) trackMessage(ctx MessageContext) error {
	stats := util.ParseMessage(ctx.Body)
	d := db.Get()
	now := time.Now().UTC()

	// Determine time-of-day buckets
	hour := now.Hour()
	nightIncr := 0
	morningIncr := 0
	if hour >= 0 && hour < 6 {
		nightIncr = 1
	} else if hour >= 6 && hour < 12 {
		morningIncr = 1
	}

	_, err := d.Exec(
		`INSERT INTO user_stats (user_id, total_messages, total_words, total_chars, total_links,
		 total_images, total_questions, total_exclamations, total_emojis, night_messages, morning_messages, updated_at)
		 VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   total_messages = total_messages + 1,
		   total_words = total_words + ?,
		   total_chars = total_chars + ?,
		   total_links = total_links + ?,
		   total_images = total_images + ?,
		   total_questions = total_questions + ?,
		   total_exclamations = total_exclamations + ?,
		   total_emojis = total_emojis + ?,
		   night_messages = night_messages + ?,
		   morning_messages = morning_messages + ?,
		   updated_at = ?`,
		string(ctx.Sender),
		stats.Words, stats.Chars, stats.Links, stats.Images,
		stats.Questions, stats.Exclamations, stats.Emojis,
		nightIncr, morningIncr, now.Unix(),
		// ON CONFLICT values
		stats.Words, stats.Chars, stats.Links, stats.Images,
		stats.Questions, stats.Exclamations, stats.Emojis,
		nightIncr, morningIncr, now.Unix(),
	)
	if err != nil {
		slog.Error("stats: track message", "err", err)
		return nil // Don't fail the message pipeline
	}

	// Room milestone tracking
	p.checkRoomMilestone(ctx)

	return nil
}

func (p *StatsPlugin) checkRoomMilestone(ctx MessageContext) {
	d := db.Get()

	// Upsert room message count
	_, err := d.Exec(
		`INSERT INTO room_milestones (room_id, total_messages, last_milestone)
		 VALUES (?, 1, 0)
		 ON CONFLICT(room_id) DO UPDATE SET total_messages = total_messages + 1`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("stats: room milestone upsert", "err", err)
		return
	}

	var totalMessages, lastMilestone int
	err = d.QueryRow(
		`SELECT total_messages, last_milestone FROM room_milestones WHERE room_id = ?`,
		string(ctx.RoomID),
	).Scan(&totalMessages, &lastMilestone)
	if err != nil {
		slog.Error("stats: room milestone read", "err", err)
		return
	}

	// Check if we crossed a new milestone
	for _, threshold := range milestoneThresholds {
		if totalMessages >= threshold && lastMilestone < threshold {
			_, err = d.Exec(
				`UPDATE room_milestones SET last_milestone = ? WHERE room_id = ?`,
				threshold, string(ctx.RoomID),
			)
			if err != nil {
				slog.Error("stats: update milestone", "err", err)
				return
			}

			msg := fmt.Sprintf("🎉 This room just hit %s messages! Keep the conversation going!", formatNumber(threshold))
			if err := p.SendMessage(ctx.RoomID, msg); err != nil {
				slog.Error("stats: milestone announcement", "err", err)
			}
			return // Only announce one milestone at a time
		}
	}
}

func (p *StatsPlugin) handleStats(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "stats")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	var totalMsg, totalWords, totalChars, totalLinks, totalImages int
	var totalQuestions, totalExclamations, totalEmojis, nightMsg, morningMsg int

	err := d.QueryRow(
		`SELECT total_messages, total_words, total_chars, total_links, total_images,
		 total_questions, total_exclamations, total_emojis, night_messages, morning_messages
		 FROM user_stats WHERE user_id = ?`, string(target),
	).Scan(&totalMsg, &totalWords, &totalChars, &totalLinks, &totalImages,
		&totalQuestions, &totalExclamations, &totalEmojis, &nightMsg, &morningMsg)

	if err == sql.ErrNoRows {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No stats found for %s.", string(target)))
	}
	if err != nil {
		slog.Error("stats: query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to look up stats.")
	}

	avgWords := 0
	if totalMsg > 0 {
		avgWords = totalWords / totalMsg
	}

	msg := fmt.Sprintf(
		"📊 Stats for %s\n"+
			"Messages: %s | Words: %s | Chars: %s\n"+
			"Links: %s | Images: %s | Emojis: %s\n"+
			"Questions: %s | Exclamations: %s\n"+
			"Night msgs (00-06): %s | Morning msgs (06-12): %s\n"+
			"Avg words/msg: %d",
		string(target),
		formatNumber(totalMsg), formatNumber(totalWords), formatNumber(totalChars),
		formatNumber(totalLinks), formatNumber(totalImages), formatNumber(totalEmojis),
		formatNumber(totalQuestions), formatNumber(totalExclamations),
		formatNumber(nightMsg), formatNumber(morningMsg),
		avgWords,
	)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *StatsPlugin) handleRankings(ctx MessageContext) error {
	args := strings.ToLower(strings.TrimSpace(p.GetArgs(ctx.Body, "rankings")))

	column := "total_words"
	label := "Words"
	switch args {
	case "links":
		column = "total_links"
		label = "Links"
	case "questions":
		column = "total_questions"
		label = "Questions"
	case "emojis":
		column = "total_emojis"
		label = "Emojis"
	case "words", "":
		column = "total_words"
		label = "Words"
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Valid categories: words, links, questions, emojis")
	}

	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	// Using fmt.Sprintf for column name is safe here since we control the value above
	query := fmt.Sprintf(
		`SELECT user_id, %s FROM user_stats WHERE %s > 0 ORDER BY %s DESC`,
		column, column, column,
	)
	rows, err := d.Query(query)
	if err != nil {
		slog.Error("stats: rankings query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load rankings.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📈 %s Rankings — Top 10\n\n", label))

	medals := []string{"🥇", "🥈", "🥉"}
	i := 0
	for rows.Next() && i < 10 {
		var userID string
		var val int
		if err := rows.Scan(&userID, &val); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(userID)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s %s\n", prefix, userID, formatNumber(val), strings.ToLower(label)))
		i++
	}

	if i == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No %s data yet.", strings.ToLower(label)))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *StatsPlugin) handlePersonality(ctx MessageContext) error {
	d := db.Get()
	var totalMsg, totalWords, totalChars, totalLinks, totalImages int
	var totalQuestions, totalExclamations, totalEmojis int

	err := d.QueryRow(
		`SELECT total_messages, total_words, total_chars, total_links, total_images,
		 total_questions, total_exclamations, total_emojis
		 FROM user_stats WHERE user_id = ?`, string(ctx.Sender),
	).Scan(&totalMsg, &totalWords, &totalChars, &totalLinks, &totalImages,
		&totalQuestions, &totalExclamations, &totalEmojis)

	if err == sql.ErrNoRows {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Not enough data to determine your personality yet. Keep chatting!")
	}
	if err != nil {
		slog.Error("stats: personality query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to analyze personality.")
	}

	msgStats := util.MessageStats{
		Words:        totalWords,
		Chars:        totalChars,
		Links:        totalLinks,
		Images:       totalImages,
		Questions:    totalQuestions,
		Exclamations: totalExclamations,
		Emojis:       totalEmojis,
	}

	archetype := util.DeriveArchetype(msgStats, totalMsg)

	msg := fmt.Sprintf(
		"🧠 %s, your chat personality is: **%s**\n_%s_\n\nBased on %s messages analyzed.",
		string(ctx.Sender), archetype.Name, archetype.Description, formatNumber(totalMsg),
	)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}
