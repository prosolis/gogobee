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
		{Name: "superstatsexplusalpha", Description: "Everything we know about you", Usage: "!superstatsexplusalpha [@user]", Category: "Leveling & Stats"},
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
	if p.IsCommand(ctx.Body, "superstatsexplusalpha") {
		return p.handleSuperStats(ctx)
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

func (p *StatsPlugin) handleSuperStats(ctx MessageContext) error {
	target := ctx.Sender
	args := p.GetArgs(ctx.Body, "superstatsexplusalpha")
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = resolved
		}
	}

	d := db.Get()
	uid := string(target)
	displayName := p.DisplayName(target)
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 **SUPER STATS EX+α — %s**\n", displayName))
	sb.WriteString("═══════════════════════════════\n\n")

	// ── Level & Economy ──
	var xp, level int
	var balance float64
	_ = d.QueryRow(`SELECT xp, level FROM users WHERE user_id = ?`, uid).Scan(&xp, &level)
	_ = d.QueryRow(`SELECT balance FROM euro_balances WHERE user_id = ?`, uid).Scan(&balance)

	var achievementCount int
	_ = d.QueryRow(`SELECT COUNT(*) FROM achievements WHERE user_id = ?`, uid).Scan(&achievementCount)

	sb.WriteString(fmt.Sprintf("⭐ **Level %d** · %s XP · 💰 €%.0f · 🏅 %d achievements\n\n",
		level, formatNumber(xp), balance, achievementCount))

	// ── Chat Stats ──
	var totalMsg, totalWords, totalLinks, totalEmojis, totalQuestions int
	err := d.QueryRow(
		`SELECT total_messages, total_words, total_links, total_emojis, total_questions
		 FROM user_stats WHERE user_id = ?`, uid,
	).Scan(&totalMsg, &totalWords, &totalLinks, &totalEmojis, &totalQuestions)
	if err == nil && totalMsg > 0 {
		sb.WriteString(fmt.Sprintf("💬 **Chat:** %s msgs · %s words · %s links · %s emoji · %s questions\n",
			formatNumber(totalMsg), formatNumber(totalWords), formatNumber(totalLinks),
			formatNumber(totalEmojis), formatNumber(totalQuestions)))

		// Personality
		msgStats := util.MessageStats{Words: totalWords, Links: totalLinks, Emojis: totalEmojis, Questions: totalQuestions}
		archetype := util.DeriveArchetype(msgStats, totalMsg)
		sb.WriteString(fmt.Sprintf("   Personality: **%s**\n", archetype.Name))
	}

	// ── Sentiment ──
	var positive, negative, neutral int
	err = d.QueryRow(
		`SELECT COALESCE(positive,0), COALESCE(negative,0), COALESCE(neutral,0)
		 FROM sentiment_stats WHERE user_id = ?`, uid,
	).Scan(&positive, &negative, &neutral)
	if err == nil {
		total := positive + negative + neutral
		if total > 0 {
			sb.WriteString(fmt.Sprintf("   Sentiment: +%d / -%d / ~%d", positive, negative, neutral))
			pct := float64(positive) / float64(total) * 100
			if pct >= 60 {
				sb.WriteString(" 😊")
			} else if float64(negative)/float64(total)*100 >= 40 {
				sb.WriteString(" 😤")
			}
			sb.WriteString("\n")
		}
	}

	// ── Potty Mouth ──
	var mild, moderate, scorching int
	err = d.QueryRow(
		`SELECT COALESCE(mild,0), COALESCE(moderate,0), COALESCE(scorching,0)
		 FROM potty_mouth WHERE user_id = ?`, uid,
	).Scan(&mild, &moderate, &scorching)
	if err == nil && (mild+moderate+scorching) > 0 {
		sb.WriteString(fmt.Sprintf("   Potty mouth: 🤬 %d mild · %d moderate · %d scorching\n", mild, moderate, scorching))
	}
	sb.WriteString("\n")

	// ── Adventure ──
	var combatLv, miningLv, fishingLv, forageLv, combatXP, miningXP, fishingXP, forageXP int
	var alive bool
	var streak, bestStreak int
	err = d.QueryRow(
		`SELECT combat_level, mining_skill, fishing_skill, foraging_skill,
		        combat_xp, mining_xp, fishing_xp, foraging_xp,
		        alive, current_streak, best_streak
		 FROM adventure_characters WHERE user_id = ?`, uid,
	).Scan(&combatLv, &miningLv, &fishingLv, &forageLv, &combatXP, &miningXP, &fishingXP, &forageXP,
		&alive, &streak, &bestStreak)
	if err == nil {
		status := "Alive"
		if !alive {
			status = "💀 Dead"
		}
		sb.WriteString(fmt.Sprintf("⚔️ **Adventure:** %s\n", status))
		sb.WriteString(fmt.Sprintf("   Combat Lv.%d (%d XP) · Mining Lv.%d (%d XP) · Fishing Lv.%d (%d XP) · Forage Lv.%d (%d XP)\n",
			combatLv, combatXP, miningLv, miningXP, fishingLv, fishingXP, forageLv, forageXP))
		if streak > 0 || bestStreak > 0 {
			sb.WriteString(fmt.Sprintf("   🔥 Streak: %d days (best: %d)\n", streak, bestStreak))
		}

		// Equipment score (use canonical scoring function)
		equip, eqErr := loadAdvEquipment(id.UserID(uid))
		if eqErr == nil && len(equip) > 0 {
			sb.WriteString(fmt.Sprintf("   Equipment score: %d\n", advEquipmentScore(equip)))
		}
		sb.WriteString("\n")
	}

	// ── Game Records ──
	sb.WriteString("🎮 **Games:**\n")
	hasGames := false

	// Holdem
	var hPlayed int
	var hWon, hLost, hBiggest int64
	err = d.QueryRow(
		`SELECT hands_played, total_won, total_lost, biggest_pot
		 FROM holdem_scores WHERE user_id = ?`, uid,
	).Scan(&hPlayed, &hWon, &hLost, &hBiggest)
	if err == nil && hPlayed > 0 {
		net := hWon - hLost
		sign := "+"
		if net < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("   🃏 Holdem: %d hands · %s€%d net · biggest pot €%d\n",
			hPlayed, sign, net, hBiggest))
		hasGames = true
	}

	// Blackjack
	var bjPlayed, bjWon int
	var bjEarned float64
	err = d.QueryRow(
		`SELECT games_played, games_won, total_earned
		 FROM blackjack_scores WHERE user_id = ?`, uid,
	).Scan(&bjPlayed, &bjWon, &bjEarned)
	if err == nil && bjPlayed > 0 {
		sb.WriteString(fmt.Sprintf("   🂡 Blackjack: %d/%d W/L · €%.0f earned\n",
			bjWon, bjPlayed-bjWon, bjEarned))
		hasGames = true
	}

	// Hangman
	var hmPlayed, hmWon int
	var hmEarned float64
	err = d.QueryRow(
		`SELECT games_played, games_won, total_earned
		 FROM hangman_scores WHERE user_id = ?`, uid,
	).Scan(&hmPlayed, &hmWon, &hmEarned)
	if err == nil && hmPlayed > 0 {
		sb.WriteString(fmt.Sprintf("   📝 Hangman: %d/%d W/L · €%.0f earned\n",
			hmWon, hmPlayed-hmWon, hmEarned))
		hasGames = true
	}

	// Wordle
	var wPlayed, wSolved, wGuesses int
	err = d.QueryRow(
		`SELECT puzzles_played, puzzles_solved, total_guesses
		 FROM wordle_stats WHERE user_id = ?`, uid,
	).Scan(&wPlayed, &wSolved, &wGuesses)
	if err == nil && wPlayed > 0 {
		avg := float64(wGuesses) / float64(wPlayed)
		sb.WriteString(fmt.Sprintf("   🟩 Wordle: %d/%d solved · %.1f avg guesses\n",
			wSolved, wPlayed, avg))
		hasGames = true
	}

	// Trivia
	var tCorrect, tWrong int
	var tFastest int64
	err = d.QueryRow(
		`SELECT COALESCE(SUM(correct),0), COALESCE(SUM(wrong),0), COALESCE(MIN(fastest_ms),0)
		 FROM trivia_scores WHERE user_id = ?`, uid,
	).Scan(&tCorrect, &tWrong, &tFastest)
	if (tCorrect + tWrong) > 0 {
		fastStr := ""
		if tFastest > 0 {
			fastStr = fmt.Sprintf(" · fastest: %.1fs", float64(tFastest)/1000)
		}
		sb.WriteString(fmt.Sprintf("   🧠 Trivia: %d/%d correct%s\n",
			tCorrect, tCorrect+tWrong, fastStr))
		hasGames = true
	}

	// UNO (single + multi combined)
	var unoSingleWins, unoSingleTotal int
	_ = d.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN result = 'win' THEN 1 ELSE 0 END), 0)
		 FROM uno_games WHERE player_id = ?`, uid,
	).Scan(&unoSingleTotal, &unoSingleWins)
	var unoMultiWins, unoMultiTotal int
	_ = d.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN winner_id = ? THEN 1 ELSE 0 END), 0)
		 FROM uno_multi_games WHERE player_ids LIKE ?`, uid, "%"+uid+"%",
	).Scan(&unoMultiTotal, &unoMultiWins)
	unoTotal := unoSingleTotal + unoMultiTotal
	unoWins := unoSingleWins + unoMultiWins
	if unoTotal > 0 {
		sb.WriteString(fmt.Sprintf("   🎴 UNO: %d/%d W/L\n",
			unoWins, unoTotal-unoWins))
		hasGames = true
	}

	// Bot defeats
	var botDefeats int
	_ = d.QueryRow(`SELECT COALESCE(SUM(losses), 0) FROM bot_defeats WHERE user_id = ?`, uid).Scan(&botDefeats)
	if botDefeats > 0 {
		sb.WriteString(fmt.Sprintf("   🤖 Lost to TwinBee: %d times\n", botDefeats))
		hasGames = true
	}

	if !hasGames {
		sb.WriteString("   No game records yet.\n")
	}

	// ── Commands ──
	var cmdCount int
	_ = d.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM command_usage WHERE user_id = ?`, uid).Scan(&cmdCount)
	if cmdCount > 0 {
		sb.WriteString(fmt.Sprintf("\n⌨️ %s commands used\n", formatNumber(cmdCount)))
	}

	// ── Account Age ──
	var createdAt time.Time
	err = d.QueryRow(`SELECT created_at FROM users WHERE user_id = ?`, uid).Scan(&createdAt)
	if err == nil {
		days := int(time.Since(createdAt).Hours() / 24)
		sb.WriteString(fmt.Sprintf("📅 Member for %d days\n", days))
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}
