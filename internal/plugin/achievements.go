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

// AchievementRegistry provides cross-plugin query access for achievements.
type AchievementRegistry interface {
	GetCommands() []CommandDef
}

// achievementDef describes a single achievement with its check function.
type achievementDef struct {
	ID          string
	Name        string
	Description string
	Emoji       string
	Check       func(d *sql.DB, userID id.UserID) bool
}

// AchievementsPlugin checks and grants achievements silently on every message.
type AchievementsPlugin struct {
	Base
	registry     AchievementRegistry
	achievements []achievementDef
}

// NewAchievementsPlugin creates a new achievements plugin.
func NewAchievementsPlugin(client *mautrix.Client, registry AchievementRegistry) *AchievementsPlugin {
	p := &AchievementsPlugin{
		Base:     NewBase(client),
		registry: registry,
	}
	p.achievements = p.buildAchievements()
	return p
}

func (p *AchievementsPlugin) Name() string { return "achievements" }

func (p *AchievementsPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "achievements", Description: "List unlocked achievements", Usage: "!achievements [@user]", Category: "Leveling & Stats"},
	}
}

func (p *AchievementsPlugin) Init() error { return nil }

func (p *AchievementsPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *AchievementsPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "achievements") {
		return p.handleAchievements(ctx)
	}

	// Passive: check achievements for the sender
	p.checkAndGrant(ctx.Sender)

	return nil
}

// checkAndGrant evaluates all achievement definitions and grants any newly unlocked ones.
func (p *AchievementsPlugin) checkAndGrant(userID id.UserID) {
	d := db.Get()

	// Batch-fetch already unlocked achievements to avoid redundant checks
	unlocked := make(map[string]bool)
	rows, err := d.Query(`SELECT achievement_id FROM achievements WHERE user_id = ?`, string(userID))
	if err != nil {
		slog.Error("achievements: query unlocked", "err", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var aid string
		if err := rows.Scan(&aid); err != nil {
			continue
		}
		unlocked[aid] = true
	}

	// Only check achievements not yet unlocked
	for _, ach := range p.achievements {
		if unlocked[ach.ID] {
			continue
		}
		if ach.Check(d, userID) {
			p.grant(d, userID, ach.ID)
		}
	}
}

func (p *AchievementsPlugin) grant(d *sql.DB, userID id.UserID, achievementID string) {
	_, err := d.Exec(
		`INSERT INTO achievements (user_id, achievement_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
		string(userID), achievementID,
	)
	if err != nil {
		slog.Error("achievements: grant", "user", userID, "achievement", achievementID, "err", err)
		return
	}
	slog.Info("achievements: granted", "user", userID, "achievement", achievementID)
}

func (p *AchievementsPlugin) handleAchievements(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "achievements")
	if args != "" {
		if resolved, ok := p.ResolveUser(args); ok {
			target = resolved
		}
	}

	d := db.Get()
	rows, err := d.Query(
		`SELECT achievement_id, unlocked_at FROM achievements WHERE user_id = ? ORDER BY unlocked_at ASC`,
		string(target),
	)
	if err != nil {
		slog.Error("achievements: list query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load achievements.")
	}
	defer rows.Close()

	// Build a lookup map from achievement defs
	defMap := make(map[string]achievementDef)
	for _, a := range p.achievements {
		defMap[a.ID] = a
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Achievements for %s:\n\n", string(target)))

	count := 0
	for rows.Next() {
		var achID string
		var unlockedAt int64
		if err := rows.Scan(&achID, &unlockedAt); err != nil {
			continue
		}
		def, ok := defMap[achID]
		if !ok {
			continue
		}
		t := time.Unix(unlockedAt, 0).UTC().Format("2006-01-02")
		sb.WriteString(fmt.Sprintf("%s %s — %s (unlocked %s)\n", def.Emoji, def.Name, def.Description, t))
		count++
	}

	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s hasn't unlocked any achievements yet.", string(target)))
	}

	sb.WriteString(fmt.Sprintf("\n%d / %d achievements unlocked", count, len(p.achievements)))
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// buildAchievements returns all achievement definitions.
func (p *AchievementsPlugin) buildAchievements() []achievementDef {
	return []achievementDef{
		// Message milestones
		{
			ID: "first_message", Name: "First Steps", Description: "Sent your first message",
			Emoji: "👶",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 1) },
		},
		{
			ID: "100_messages", Name: "Chatterbox", Description: "Sent 100 messages",
			Emoji: "💬",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 100) },
		},
		{
			ID: "1000_messages", Name: "Motor Mouth", Description: "Sent 1,000 messages",
			Emoji: "🗣️",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 1000) },
		},
		{
			ID: "10000_messages", Name: "Legend", Description: "Sent 10,000 messages",
			Emoji: "🏛️",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 10000) },
		},

		// Time-based
		{
			ID: "night_owl", Name: "Night Owl", Description: "Sent 100 messages between midnight and 6am",
			Emoji: "🦉",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "night_messages", 100) },
		},
		{
			ID: "early_bird", Name: "Early Bird", Description: "Sent 100 messages between 6am and noon",
			Emoji: "🐦",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "morning_messages", 100) },
		},

		// Content
		{
			ID: "wordsmith", Name: "Wordsmith", Description: "Average message length over 8 words",
			Emoji: "✍️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var totalMessages, totalWords int
				err := d.QueryRow(
					`SELECT total_messages, total_words FROM user_stats WHERE user_id = ?`,
					string(u),
				).Scan(&totalMessages, &totalWords)
				if err != nil || totalMessages < 50 {
					return false
				}
				return float64(totalWords)/float64(totalMessages) > 8.0
			},
		},
		{
			ID: "link_collector", Name: "Link Collector", Description: "Shared 50 links",
			Emoji: "🔗",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_links", 50) },
		},
		{
			ID: "shutterbug", Name: "Shutterbug", Description: "Shared 20 images",
			Emoji: "📸",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_images", 20) },
		},
		{
			ID: "question_master", Name: "Question Master", Description: "Asked 100 questions",
			Emoji: "❓",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_questions", 100) },
		},

		// Social
		{
			ID: "welcome_wagon", Name: "Welcome Wagon", Description: "Welcomed a new member to the community",
			Emoji: "👋",
			Check: func(d *sql.DB, u id.UserID) bool {
				// This is checked externally when a new user joins
				// Here we just check if it was already granted
				return false
			},
		},
		{
			ID: "rep_magnet", Name: "Rep Magnet", Description: "Received 10 reputation points",
			Emoji: "💜",
			Check: func(d *sql.DB, u id.UserID) bool {
				var total int
				err := d.QueryRow(
					`SELECT COALESCE(SUM(amount), 0) FROM xp_log WHERE user_id = ? AND reason = 'reputation'`,
					string(u),
				).Scan(&total)
				if err != nil {
					return false
				}
				return total/5 >= 10
			},
		},

		// Streaks
		{
			ID: "week_streak", Name: "Weekly Warrior", Description: "Active for 7 consecutive days",
			Emoji: "📅",
			Check: func(d *sql.DB, u id.UserID) bool { return checkStreak(d, u, 7) },
		},
		{
			ID: "month_streak", Name: "Monthly Marvel", Description: "Active for 30 consecutive days",
			Emoji: "🗓️",
			Check: func(d *sql.DB, u id.UserID) bool { return checkStreak(d, u, 30) },
		},

		// Trivia
		{
			ID: "trivia_novice", Name: "Trivia Novice", Description: "Answered 10 trivia questions correctly",
			Emoji: "🧠",
			Check: func(d *sql.DB, u id.UserID) bool {
				var correct int
				err := d.QueryRow(
					`SELECT COALESCE(SUM(correct), 0) FROM trivia_scores WHERE user_id = ?`,
					string(u),
				).Scan(&correct)
				return err == nil && correct >= 10
			},
		},
		{
			ID: "trivia_master", Name: "Trivia Master", Description: "Answered 100 trivia questions correctly",
			Emoji: "🎓",
			Check: func(d *sql.DB, u id.UserID) bool {
				var correct int
				err := d.QueryRow(
					`SELECT COALESCE(SUM(correct), 0) FROM trivia_scores WHERE user_id = ?`,
					string(u),
				).Scan(&correct)
				return err == nil && correct >= 100
			},
		},

		// Special
		{
			ID: "markov_victim", Name: "Markov Victim", Description: "Had your words remixed by the Markov chain",
			Emoji: "🤖",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted externally by the markov plugin
				return false
			},
		},
		{
			ID: "logophile", Name: "Logophile", Description: "Used 10 different Words of the Day",
			Emoji: "📖",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(DISTINCT date) FROM wotd_usage WHERE user_id = ? AND count > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 10
			},
		},

		// Additional milestones to reach 32 total
		{
			ID: "emoji_lover", Name: "Emoji Lover", Description: "Used 500 emojis in messages",
			Emoji: "😍",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_emojis", 500) },
		},
		{
			ID: "exclaimer", Name: "Exclaimer", Description: "Used 200 exclamation marks",
			Emoji: "❗",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_exclamations", 200) },
		},
		{
			ID: "two_week_streak", Name: "Fortnight Fighter", Description: "Active for 14 consecutive days",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool { return checkStreak(d, u, 14) },
		},
		{
			ID: "quarter_streak", Name: "Seasonal Sage", Description: "Active for 90 consecutive days",
			Emoji: "🌿",
			Check: func(d *sql.DB, u id.UserID) bool { return checkStreak(d, u, 90) },
		},
		{
			ID: "500_messages", Name: "Conversationalist", Description: "Sent 500 messages",
			Emoji: "🎙️",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 500) },
		},
		{
			ID: "5000_messages", Name: "Veteran", Description: "Sent 5,000 messages",
			Emoji: "🎖️",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_messages", 5000) },
		},
		{
			ID: "link_hoarder", Name: "Link Hoarder", Description: "Shared 200 links",
			Emoji: "🌐",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_links", 200) },
		},
		{
			ID: "photographer", Name: "Photographer", Description: "Shared 100 images",
			Emoji: "🖼️",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "total_images", 100) },
		},
		{
			ID: "trivia_legend", Name: "Trivia Legend", Description: "Answered 500 trivia questions correctly",
			Emoji: "👑",
			Check: func(d *sql.DB, u id.UserID) bool {
				var correct int
				err := d.QueryRow(
					`SELECT COALESCE(SUM(correct), 0) FROM trivia_scores WHERE user_id = ?`,
					string(u),
				).Scan(&correct)
				return err == nil && correct >= 500
			},
		},
		{
			ID: "rep_star", Name: "Reputation Star", Description: "Received 50 reputation points",
			Emoji: "⭐",
			Check: func(d *sql.DB, u id.UserID) bool {
				var total int
				err := d.QueryRow(
					`SELECT COALESCE(SUM(amount), 0) FROM xp_log WHERE user_id = ? AND reason = 'reputation'`,
					string(u),
				).Scan(&total)
				return err == nil && total/5 >= 50
			},
		},
		{
			ID: "night_dweller", Name: "Night Dweller", Description: "Sent 500 night messages",
			Emoji: "🌙",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "night_messages", 500) },
		},
		{
			ID: "dawn_patrol", Name: "Dawn Patrol", Description: "Sent 500 morning messages",
			Emoji: "🌅",
			Check: func(d *sql.DB, u id.UserID) bool { return statGTE(d, u, "morning_messages", 500) },
		},
		{
			ID: "wotd_apprentice", Name: "WOTD Apprentice", Description: "Used 5 different Words of the Day",
			Emoji: "📝",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(DISTINCT date) FROM wotd_usage WHERE user_id = ? AND count > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 5
			},
		},
		{
			ID: "year_streak", Name: "Year-Long Devotion", Description: "Active for 365 consecutive days",
			Emoji: "🏆",
			Check: func(d *sql.DB, u id.UserID) bool { return checkStreak(d, u, 365) },
		},

		// Profile completeness
		{
			ID: "birthday_set", Name: "Born to Party", Description: "Set your birthday",
			Emoji: "🎂",
			Check: func(d *sql.DB, u id.UserID) bool {
				var month int
				err := d.QueryRow(
					`SELECT month FROM birthdays WHERE user_id = ? AND month > 0`,
					string(u),
				).Scan(&month)
				return err == nil && month > 0
			},
		},
		{
			ID: "timezone_set", Name: "Time Traveler", Description: "Set your timezone",
			Emoji: "🌍",
			Check: func(d *sql.DB, u id.UserID) bool {
				var tz string
				err := d.QueryRow(
					`SELECT timezone FROM birthdays WHERE user_id = ? AND timezone != ''`,
					string(u),
				).Scan(&tz)
				return err == nil && tz != ""
			},
		},
		{
			ID: "profile_complete", Name: "Identity Established", Description: "Set both birthday and timezone",
			Emoji: "🪪",
			Check: func(d *sql.DB, u id.UserID) bool {
				var month int
				var tz string
				err := d.QueryRow(
					`SELECT month, timezone FROM birthdays WHERE user_id = ? AND month > 0 AND timezone != ''`,
					string(u),
				).Scan(&month, &tz)
				return err == nil && month > 0 && tz != ""
			},
		},
	}
}

// statGTE checks if a user_stats column is >= threshold.
func statGTE(d *sql.DB, userID id.UserID, column string, threshold int) bool {
	var val int
	query := fmt.Sprintf(`SELECT COALESCE(%s, 0) FROM user_stats WHERE user_id = ?`, column)
	err := d.QueryRow(query, string(userID)).Scan(&val)
	if err != nil {
		return false
	}
	return val >= threshold
}

// checkStreak checks if a user has been active for N consecutive days ending today (or recently).
func checkStreak(d *sql.DB, userID id.UserID, days int) bool {
	rows, err := d.Query(
		`SELECT date FROM daily_activity WHERE user_id = ? ORDER BY date DESC LIMIT ?`,
		string(userID), days+7, // fetch a few extra to find the streak
	)
	if err != nil {
		return false
	}
	defer rows.Close()

	dates := make(map[string]bool)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			continue
		}
		dates[date] = true
	}

	if len(dates) < days {
		return false
	}

	// Check for a consecutive run of `days` days ending at any recent date
	today := time.Now().UTC()
	for offset := 0; offset < 7; offset++ {
		streak := 0
		for i := 0; i < days; i++ {
			dateStr := today.AddDate(0, 0, -(offset + i)).Format("2006-01-02")
			if dates[dateStr] {
				streak++
			} else {
				break
			}
		}
		if streak >= days {
			return true
		}
	}

	return false
}

// GrantAchievement allows external plugins to grant specific achievements.
func (p *AchievementsPlugin) GrantAchievement(userID id.UserID, achievementID string) {
	d := db.Get()
	p.grant(d, userID, achievementID)
}
