package plugin

import (
	"database/sql"
	"encoding/json"
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

func (p *AchievementsPlugin) Name() string    { return "achievements" }
func (p *AchievementsPlugin) Version() string { return "1.2.0" }

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
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
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

		// ── Economy ─────────────────────────────────────────────────────────
		{
			ID: "euro_first", Name: "First Paycheck", Description: "The economy has claimed another victim.",
			Emoji: "💶",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(*) FROM euro_transactions WHERE user_id = ? AND amount > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 1
			},
		},
		{
			ID: "euro_1k", Name: "Four Figures", Description: "You are now, technically, a thousandaire.",
			Emoji: "💰",
			Check: func(d *sql.DB, u id.UserID) bool {
				var balance float64
				err := d.QueryRow(
					`SELECT balance FROM euro_balances WHERE user_id = ?`,
					string(u),
				).Scan(&balance)
				return err == nil && balance >= 1000
			},
		},
		{
			ID: "euro_10k", Name: "High Roller", Description: "Someone stop this person.",
			Emoji: "🤑",
			Check: func(d *sql.DB, u id.UserID) bool {
				var balance float64
				err := d.QueryRow(
					`SELECT balance FROM euro_balances WHERE user_id = ?`,
					string(u),
				).Scan(&balance)
				return err == nil && balance >= 10000
			},
		},
		{
			ID: "euro_broke", Name: "Overdrafted", Description: "Character-building experience.",
			Emoji: "📉",
			Check: func(d *sql.DB, u id.UserID) bool {
				var balance float64
				err := d.QueryRow(
					`SELECT balance FROM euro_balances WHERE user_id = ?`,
					string(u),
				).Scan(&balance)
				return err == nil && balance < 0
			},
		},
		{
			ID: "euro_comeback", Name: "Comeback Kid", Description: "Against all odds.",
			Emoji: "🔄",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by euro plugin when recovering from debt
				return false
			},
		},
		{
			ID: "euro_generous", Name: "Philanthropist", Description: "Money is just a social construct anyway.",
			Emoji: "🎁",
			Check: func(d *sql.DB, u id.UserID) bool {
				var total float64
				err := d.QueryRow(
					`SELECT COALESCE(SUM(ABS(amount)), 0) FROM euro_transactions WHERE user_id = ? AND reason = 'transfer' AND amount < 0`,
					string(u),
				).Scan(&total)
				return err == nil && total >= 1000
			},
		},

		// ── Blackjack ───────────────────────────────────────────────────────
		{
			ID: "bj_first_win", Name: "Natural", Description: "The table fears you. Slightly.",
			Emoji: "🃏",
			Check: func(d *sql.DB, u id.UserID) bool {
				var won int
				err := d.QueryRow(
					`SELECT games_won FROM blackjack_scores WHERE user_id = ?`,
					string(u),
				).Scan(&won)
				return err == nil && won >= 1
			},
		},
		{
			ID: "bj_blackjack", Name: "Blackjack", Description: "21. As the prophecy foretold.",
			Emoji: "🎰",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by blackjack plugin on natural 21
				return false
			},
		},
		{
			ID: "bj_bust", Name: "Going for It", Description: "Hope is a beautiful thing.",
			Emoji: "💥",
			Check: func(d *sql.DB, u id.UserID) bool {
				var played, won int
				err := d.QueryRow(
					`SELECT games_played, games_won FROM blackjack_scores WHERE user_id = ?`,
					string(u),
				).Scan(&played, &won)
				// Busts are approximated as losses (played - won)
				return err == nil && (played-won) >= 10
			},
		},
		{
			ID: "bj_beat_twinbee", Name: "Gotcha 🐝", Description: "They are furious and will not forget this.",
			Emoji: "🐝",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by blackjack plugin when beating the bot
				return false
			},
		},
		{
			ID: "bj_100_hands", Name: "Card Shark", Description: "Statistically, you should have stopped.",
			Emoji: "🦈",
			Check: func(d *sql.DB, u id.UserID) bool {
				var played int
				err := d.QueryRow(
					`SELECT games_played FROM blackjack_scores WHERE user_id = ?`,
					string(u),
				).Scan(&played)
				return err == nil && played >= 100
			},
		},

		// ── UNO ─────────────────────────────────────────────────────────────
		{
			ID: "uno_first_win", Name: "UNO!", Description: "One card. Victory. Glory.",
			Emoji: "🟥",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by UNO plugin on first win
				return false
			},
		},
		{
			ID: "uno_nomercy_win", Name: "No Mercy", Description: "You are a monster and we respect it.",
			Emoji: "😈",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by UNO plugin on No Mercy mode win
				return false
			},
		},
		{
			ID: "uno_draw_stack", Name: "Stack Overflow", Description: "The pile grows. Your opponents weep.",
			Emoji: "📚",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by UNO plugin on 3+ draw stacks in one game
				return false
			},
		},
		{
			ID: "uno_comeback", Name: "Down to Zero", Description: "The math said no. You said yes.",
			Emoji: "🔥",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by UNO plugin on winning from 10+ cards
				return false
			},
		},

		// ── Texas Hold'em ───────────────────────────────────────────────────
		{
			ID: "holdem_first_win", Name: "Ante Up", Description: "A pot has been claimed.",
			Emoji: "♠️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var won int
				err := d.QueryRow(
					`SELECT total_won FROM holdem_scores WHERE user_id = ?`,
					string(u),
				).Scan(&won)
				return err == nil && won > 0
			},
		},
		{
			ID: "holdem_royal_flush", Name: "Royal Flush", Description: "This has never happened before in recorded history.",
			Emoji: "👑",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by holdem plugin on royal flush
				return false
			},
		},
		{
			ID: "holdem_bluff", Name: "Poker Face", Description: "The audacity. Incredible.",
			Emoji: "😶",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by holdem plugin on high-card-only win
				return false
			},
		},
		{
			ID: "holdem_beat_npc", Name: "Outsmarted the Bot", Description: "CFR trained on millions of hands. You trained on vibes.",
			Emoji: "🤖",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by holdem plugin on beating CFR NPC
				return false
			},
		},
		{
			ID: "holdem_allin_win", Name: "All In", Description: "Everything on the line. Everything gained.",
			Emoji: "💎",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by holdem plugin on all-in win
				return false
			},
		},
		{
			ID: "holdem_bounty", Name: "Bounty Hunter", Description: "They had families.",
			Emoji: "🎯",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by holdem plugin on knocking out a player
				return false
			},
		},

		// ── Hangman ─────────────────────────────────────────────────────────
		{
			ID: "hangman_solve", Name: "Lexicographer", Description: "Language. You understand it.",
			Emoji: "📖",
			Check: func(d *sql.DB, u id.UserID) bool {
				var won int
				err := d.QueryRow(
					`SELECT games_won FROM hangman_scores WHERE user_id = ?`,
					string(u),
				).Scan(&won)
				return err == nil && won >= 1
			},
		},
		{
			ID: "hangman_first_guess", Name: "Mind Reader", Description: "How.",
			Emoji: "🔮",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by hangman plugin on first-guess solve
				return false
			},
		},
		{
			ID: "hangman_submitted", Name: "Contributor", Description: "Your words. Their suffering.",
			Emoji: "✏️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by hangman plugin when a submitted phrase is used
				return false
			},
		},
		{
			ID: "hangman_executioner", Name: "Executioner", Description: "You did this.",
			Emoji: "⚰️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by hangman plugin on final wrong letter
				return false
			},
		},

		// ── Wordle ──────────────────────────────────────────────────────────
		{
			ID: "wordle_solve", Name: "Wordled", Description: "The letters obeyed you.",
			Emoji: "🟩",
			Check: func(d *sql.DB, u id.UserID) bool {
				var solved int
				err := d.QueryRow(
					`SELECT puzzles_solved FROM wordle_stats WHERE user_id = ?`,
					string(u),
				).Scan(&solved)
				return err == nil && solved >= 1
			},
		},
		{
			ID: "wordle_first_guess", Name: "Omniscient", Description: "That was statistically impossible.",
			Emoji: "🧿",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by wordle plugin on 1-guess community solve
				return false
			},
		},
		{
			ID: "wordle_streak_3", Name: "Hot Streak", Description: "Momentum.",
			Emoji: "🔥",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by wordle plugin on 3 consecutive community wins
				return false
			},
		},
		{
			ID: "wordle_bonus", Name: "Nerd", Description: "One of us.",
			Emoji: "🎮",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by wordle plugin on bonus word solve
				return false
			},
		},
		{
			ID: "wordle_closer", Name: "Closer", Description: "The community thanks you. Sort of.",
			Emoji: "🏁",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by wordle plugin on submitting the winning guess
				return false
			},
		},

		// ── Adventure ───────────────────────────────────────────────────────
		{
			ID: "adv_first", Name: "Adventurer", Description: "The dungeon awaits. Probably.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(*) FROM adventure_activity_log WHERE user_id = ?`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 1
			},
		},
		{
			ID: "adv_died", Name: "Respawning", Description: "This is fine.",
			Emoji: "💀",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on death
				return false
			},
		},
		{
			ID: "adv_revived", Name: "Second Life", Description: "Someone cared enough.",
			Emoji: "✨",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on admin revive
				return false
			},
		},
		{
			ID: "adv_streak_7", Name: "Daily Grind", Description: "Committed. Concerningly so.",
			Emoji: "📅",
			Check: func(d *sql.DB, u id.UserID) bool {
				var streak int
				err := d.QueryRow(
					`SELECT best_streak FROM player_meta WHERE user_id = ?`,
					string(u),
				).Scan(&streak)
				return err == nil && streak >= 7
			},
		},
		{
			ID: "adv_streak_30", Name: "Devoted", Description: "Please go outside.",
			Emoji: "🗓️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var streak int
				err := d.QueryRow(
					`SELECT best_streak FROM player_meta WHERE user_id = ?`,
					string(u),
				).Scan(&streak)
				return err == nil && streak >= 30
			},
		},
		{
			ID: "adv_max_level", Name: "Legendary", Description: "What's next? Nothing. You've done it.",
			Emoji: "🏆",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on reaching max level
				return false
			},
		},
		{
			ID: "adv_treasure_cap", Name: "Hoarder", Description: "No more pockets.",
			Emoji: "💎",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(*) FROM adventure_treasures WHERE user_id = ?`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 3
			},
		},
		{
			ID: "adv_grudge_win", Name: "Revenge", Description: "Redemption arc complete.",
			Emoji: "⚡",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on grudge location success
				return false
			},
		},
		{
			ID: "adv_party", Name: "Party of Two", Description: "Accidental cooperation.",
			Emoji: "🤝",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on party bonus
				return false
			},
		},
		{
			ID: "adv_twinbee_gift", Name: "Blessed by TwinBee", Description: "The bee has chosen you. Unknown why.",
			Emoji: "🐝",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by adventure plugin on TwinBee NPC buff
				return false
			},
		},

		// ── Word of the Day ─────────────────────────────────────────────────
		{
			ID: "wotd_first", Name: "Logophile Jr.", Description: "25 XP well earned.",
			Emoji: "📝",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(DISTINCT date) FROM wotd_usage WHERE user_id = ? AND count > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 1
			},
		},
		{
			ID: "wotd_week", Name: "Vocabulary Builder", Description: "The dictionary approves.",
			Emoji: "📚",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(DISTINCT date) FROM wotd_usage WHERE user_id = ? AND count > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 7
			},
		},
		{
			ID: "wotd_30", Name: "Wordsmith Elite", Description: "You have transcended mere communication.",
			Emoji: "🎓",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				err := d.QueryRow(
					`SELECT COUNT(DISTINCT date) FROM wotd_usage WHERE user_id = ? AND count > 0`,
					string(u),
				).Scan(&count)
				return err == nil && count >= 30
			},
		},

		// ── Community & Social ──────────────────────────────────────────────
		{
			ID: "quote_saved", Name: "Quotable", Description: "Posterity has noted your words.",
			Emoji: "💬",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by quote plugin when a quote is saved for the user
				return false
			},
		},
		{
			ID: "quote_saved_10", Name: "Memorable", Description: "You say things worth remembering. Allegedly.",
			Emoji: "🏅",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by quote plugin when 10 quotes are saved for the user
				return false
			},
		},
		{
			ID: "missing_poster", Name: "Milk Carton", Description: "Someone noticed you were gone. Eventually.",
			Emoji: "🥛",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by missing poster plugin
				return false
			},
		},
		{
			ID: "tarot_spread", Name: "The Fool's Journey", Description: "Past, present, future. All vibes.",
			Emoji: "🔮",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by tarot plugin on three-card spread
				return false
			},
		},

		// ── Arena ──────────────────────────────────────────────────────────
		{
			ID: "arena_first_blood", Name: "First Blood", Description: "The Arena has noted your existence.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by arena plugin on first round survived
				return false
			},
		},
		{
			ID: "arena_tier1", Name: "Scrub Slayer", Description: "Gelatinous Homunculus has been avenged by no one.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 1 AND outcome IN ('completed','cashed_out')`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_tier2", Name: "Promoted to Violence", Description: "The Impersonator was not, in fact, a chest.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 2 AND outcome IN ('completed','cashed_out')`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_tier3", Name: "Brute Force", Description: "The Inevitable was, on this occasion, avoidable.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 3 AND outcome IN ('completed','cashed_out')`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_tier4", Name: "Horror Show", Description: "The Collector of Faces does not have yours. Yet.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 4 AND outcome IN ('completed','cashed_out')`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_tier5", Name: "World Eater", Description: "That Which Has Always Been has now lost once.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 5 AND outcome = 'completed'`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_descend", Name: "Going Deeper", Description: "Greed is a valid strategy.",
			Emoji: "⚔️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by arena plugin on descend
				return false
			},
		},
		{
			ID: "arena_full_run", Name: "All the Way Down", Description: "Statistically impossible. Empirically: you.",
			Emoji: "🏆",
			Check: func(d *sql.DB, u id.UserID) bool {
				// A full run = cleared all 5 tiers without cashing out
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 5 AND outcome = 'completed' AND rounds_survived = 20`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_death_t5", Name: "Acceptable Losses", Description: "You made it to Tier 5. That's something.",
			Emoji: "💀",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND tier = 5 AND outcome = 'dead'`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "arena_omega", Name: "The Test", Description: "The machine has logged a loss for the first time.",
			Emoji: "🤖",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by arena plugin when defeating Omega Mk. Zero
				return false
			},
		},
		{
			ID: "arena_cashout_big", Name: "Take the Money", Description: "Discretion. Valor. Etc.",
			Emoji: "💰",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM arena_history WHERE user_id = ? AND outcome = 'cashed_out' AND earnings >= 10000`, string(u)).Scan(&count)
				return count > 0
			},
		},

		// ── Streak & Babysit ────────────────────────────────────────────────
		{
			ID: "adv_streaker", Name: "Streaker", Description: "Maintained a 7-day streak. Running wild.",
			Emoji: "🏃",
			Check: func(d *sql.DB, u id.UserID) bool {
				var streak int
				_ = d.QueryRow(`SELECT best_streak FROM player_meta WHERE user_id = ?`, string(u)).Scan(&streak)
				return streak >= 7
			},
		},
		{
			ID: "adv_streak_60", Name: "Obsessed", Description: "60 days. The game plays you now.",
			Emoji: "🔥",
			Check: func(d *sql.DB, u id.UserID) bool {
				var streak int
				_ = d.QueryRow(`SELECT best_streak FROM player_meta WHERE user_id = ?`, string(u)).Scan(&streak)
				return streak >= 60
			},
		},
		{
			ID: "adv_streak_100", Name: "Centurion", Description: "100 days. This is your life now.",
			Emoji: "💯",
			Check: func(d *sql.DB, u id.UserID) bool {
				var streak int
				_ = d.QueryRow(`SELECT best_streak FROM player_meta WHERE user_id = ?`, string(u)).Scan(&streak)
				return streak >= 100
			},
		},
		{
			ID: "adv_babysit_hired", Name: "Delegation", Description: "Hired someone to do the hard part.",
			Emoji: "🍼",
			Check: func(d *sql.DB, u id.UserID) bool {
				var count int
				_ = d.QueryRow(`SELECT COUNT(*) FROM adventure_babysit_log WHERE user_id = ?`, string(u)).Scan(&count)
				return count > 0
			},
		},
		{
			ID: "adv_auto_babysit", Name: "Safety Net", Description: "Auto-babysit saved your streak. Worth every euro.",
			Emoji: "🛡️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by scheduler when auto-babysit fires
				return false
			},
		},
		{
			ID: "adv_streak_survivor", Name: "Streak Survivor", Description: "Rebuilt your streak to 14 after losing it.",
			Emoji: "🔄",
			Check: func(d *sql.DB, u id.UserID) bool {
				var current, best, decayed int
				err := d.QueryRow(`SELECT current_streak, best_streak, streak_decayed FROM player_meta WHERE user_id = ?`, string(u)).Scan(&current, &best, &decayed)
				return err == nil && current >= 14 && decayed > 0
			},
		},

		// ── Combat Moments ──────────────────────────────────────────────────
		{
			ID: "combat_near_death", Name: "Clutch", Description: "Won with less than 15% HP. Barely.",
			Emoji: "💔",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on near-death victory
				return false
			},
		},
		{
			ID: "combat_pet_save", Name: "Good Boy", Description: "Your pet distracted the enemy at exactly the right moment.",
			Emoji: "🐾",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on pet whiff event
				return false
			},
		},
		{
			ID: "combat_sniper_kill", Name: "One Shot", Description: "Arina ended the fight before it started.",
			Emoji: "🎯",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on sniper kill
				return false
			},
		},
		{
			ID: "combat_death_save", Name: "Sovereign's Reprieve", Description: "The crown said not today.",
			Emoji: "👑",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on death save event
				return false
			},
		},
		{
			ID: "combat_misty_clutch", Name: "Misty's Touch", Description: "Healed mid-combat. She was watching.",
			Emoji: "💚",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on Misty heal event
				return false
			},
		},
		{
			ID: "combat_consumable_used", Name: "Prepared", Description: "Used a consumable in combat. Preparation pays off.",
			Emoji: "🧪",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on consumable use event
				return false
			},
		},
		{
			ID: "adv_first_craft", Name: "Apprentice Herbalist", Description: "Crafted your first consumable. The wilds provide.",
			Emoji: "🌿",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on first successful craft
				return false
			},
		},
		{
			ID: "adv_craft_t5", Name: "Master Alchemist", Description: "Crafted a Tier 5 consumable. Nature bends to your will.",
			Emoji: "⚗️",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by combat bridge on T5 craft
				return false
			},
		},

		// ── Expeditions ──
		// All passive: they read the expedition rows the extract path already
		// writes. Tier comes from the zone registry, not a column, so the
		// checks map zone_id → Tier in Go.
		{
			ID: "expedition_clear_t1", Name: "Off the Porch", Description: "Cleared your first Tier 1 zone. It only gets worse from here.",
			Emoji: "🗺️",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedAnyZoneOfTier(d, u, 1) },
		},
		{
			ID: "expedition_clear_t2", Name: "Reasonably Lost", Description: "Cleared a Tier 2 zone. The map stops being a suggestion.",
			Emoji: "🗺️",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedAnyZoneOfTier(d, u, 2) },
		},
		{
			ID: "expedition_clear_t3", Name: "Deep Enough", Description: "Cleared a Tier 3 zone. Someone should have stopped you.",
			Emoji: "🗺️",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedAnyZoneOfTier(d, u, 3) },
		},
		{
			ID: "expedition_clear_t4", Name: "Past the Warnings", Description: "Cleared a Tier 4 zone. The signs were quite clear.",
			Emoji: "🗺️",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedAnyZoneOfTier(d, u, 4) },
		},
		{
			ID: "expedition_clear_t5", Name: "Where the Map Ends", Description: "Cleared a Tier 5 zone. There was nothing left to be brave about.",
			Emoji: "🗺️",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedAnyZoneOfTier(d, u, 5) },
		},
		{
			ID: "expedition_master_t1", Name: "Warren Cartography", Description: "Cleared every Tier 1 zone. Thorough.",
			Emoji: "🧭",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedEveryZoneOfTier(d, u, 1) },
		},
		{
			ID: "expedition_master_t2", Name: "No Stone Unturned", Description: "Cleared every Tier 2 zone.",
			Emoji: "🧭",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedEveryZoneOfTier(d, u, 2) },
		},
		{
			ID: "expedition_master_t3", Name: "Completionist's Limp", Description: "Cleared every Tier 3 zone. Walk it off.",
			Emoji: "🧭",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedEveryZoneOfTier(d, u, 3) },
		},
		{
			ID: "expedition_master_t4", Name: "Nowhere Left to Go Wrong", Description: "Cleared every Tier 4 zone.",
			Emoji: "🧭",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedEveryZoneOfTier(d, u, 4) },
		},
		{
			ID: "expedition_master_t5", Name: "The Long Way Down", Description: "Cleared every Tier 5 zone. Both of them. All the way.",
			Emoji: "🧭",
			Check: func(d *sql.DB, u id.UserID) bool { return clearedEveryZoneOfTier(d, u, 5) },
		},
		{
			ID: "expedition_quiet_clear", Name: "Nobody Saw Anything", Description: "Cleared a zone without threat ever passing 50. You were never here.",
			Emoji: "🌑",
			Check: clearedUnderThreat50,
		},
		{
			ID: "temper_legendary", Name: "Hot Enough", Description: "Tempered an item all the way to Legendary. The blacksmith needed a moment.",
			Emoji: "🔨",
			Check: func(d *sql.DB, u id.UserID) bool {
				// Granted by the temper path on reaching Legendary
				return false
			},
		},
		// N7/B4 — the Renown wing (gogobee_engagement_plan.md §B4). Passive checks
		// against the derived Renown level (N7/B2); no event hook needed.
		{
			ID: "renown_1", Name: "Beyond the Cap", Description: "Earned your first level of Renown. The story was supposed to be over.",
			Emoji: "✦",
			Check: func(d *sql.DB, u id.UserID) bool { return renownAtLeast(d, u, 1) },
		},
		{
			ID: "renown_5", Name: "Storied", Description: "Reached Renown 5. They tell versions of your runs that never happened.",
			Emoji: "✦",
			Check: func(d *sql.DB, u id.UserID) bool { return renownAtLeast(d, u, 5) },
		},
		{
			ID: "renown_10", Name: "Fabled", Description: "Reached Renown 10. Somewhere a bard is getting rich off your name.",
			Emoji: "✦",
			Check: func(d *sql.DB, u id.UserID) bool { return renownAtLeast(d, u, 10) },
		},
	}
}

// renownAtLeast reports whether the player's derived Renown level (N7/B2) has
// reached level, reading player_meta.renown_xp through the passed handle.
func renownAtLeast(d *sql.DB, userID id.UserID, level int) bool {
	var xp int
	if err := d.QueryRow(
		`SELECT renown_xp FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&xp); err != nil {
		return false
	}
	return renownLevelFor(xp) >= level
}

// ── Expedition achievement helpers ──────────────────────────────────────────

// clearedZoneIDs returns the zones this player has beaten outright. Boss-
// defeated gates out extractions that merely ended in 'complete'.
func clearedZoneIDs(d *sql.DB, userID id.UserID) map[ZoneID]bool {
	out := map[ZoneID]bool{}
	rows, err := d.Query(
		`SELECT DISTINCT zone_id FROM dnd_expedition
		 WHERE user_id = ? AND status = ? AND boss_defeated = 1`,
		string(userID), ExpeditionStatusComplete)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var z string
		if err := rows.Scan(&z); err != nil {
			return out
		}
		out[ZoneID(z)] = true
	}
	return out
}

func clearedAnyZoneOfTier(d *sql.DB, userID id.UserID, tier ZoneTier) bool {
	cleared := clearedZoneIDs(d, userID)
	for zid := range cleared {
		if z, ok := getZone(zid); ok && z.Tier == tier {
			return true
		}
	}
	return false
}

func clearedEveryZoneOfTier(d *sql.DB, userID id.UserID, tier ZoneTier) bool {
	cleared := clearedZoneIDs(d, userID)
	seen := 0
	for _, z := range allZones() {
		if z.Tier != tier {
			continue
		}
		if !cleared[z.ID] {
			return false
		}
		seen++
	}
	return seen > 0
}

// clearedUnderThreat50 looks for a cleared expedition whose peak threat never
// crossed 50 — the same max_threat_seen sample the Patient Zero milestone
// reads, so the two reward the same play.
//
// The key's *presence* is required, not just a low value. recordMaxThreat only
// samples on day rollover and never stores a zero, so an absent key means the
// expedition has no threat history at all (a same-day clear) — which is not
// evidence that threat stayed low.
func clearedUnderThreat50(d *sql.DB, userID id.UserID) bool {
	rows, err := d.Query(
		`SELECT region_state FROM dnd_expedition
		 WHERE user_id = ? AND status = ? AND boss_defeated = 1`,
		string(userID), ExpeditionStatusComplete)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false
		}
		var state map[string]any
		if json.Unmarshal([]byte(raw), &state) != nil {
			continue
		}
		peak, ok := state[regionStateMaxThreatKey].(float64)
		if ok && int(peak) < 50 {
			return true
		}
	}
	return false
}

// statGTE checks if a user_stats column is >= threshold.
var allowedStatColumns = map[string]bool{
	"total_messages":     true,
	"night_messages":     true,
	"morning_messages":   true,
	"total_links":        true,
	"total_images":       true,
	"total_questions":    true,
	"total_emojis":       true,
	"total_exclamations": true,
}

func statGTE(d *sql.DB, userID id.UserID, column string, threshold int) bool {
	if !allowedStatColumns[column] {
		slog.Error("statGTE: unknown column", "column", column)
		return false
	}
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
