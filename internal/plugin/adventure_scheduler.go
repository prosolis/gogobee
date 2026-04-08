package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Morning DM Ticker ────────────────────────────────────────────────────────

func (p *AdventurePlugin) morningTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		if now.Hour() != p.morningHour || now.Minute() != 0 {
			continue
		}

		dateKey := now.Format("2006-01-02")
		jobName := "adventure_morning"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}

		slog.Info("adventure: sending morning DMs")
		p.sendMorningDMs()
		db.MarkJobCompleted(jobName, dateKey)
	}
}

func (p *AdventurePlugin) sendMorningDMs() {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("adventure: failed to load characters for morning DMs", "err", err)
		return
	}

	now := time.Now().UTC()
	isHol, holName := isHolidayToday()
	if isHol {
		slog.Info("adventure: holiday detected for morning DMs", "holiday", holName)
	}

	// Shuffle to avoid always hitting the same users first if early sends
	// get rate-limited and later ones don't go out.
	rand.Shuffle(len(chars), func(i, j int) { chars[i], chars[j] = chars[j], chars[i] })

	for i, char := range chars {
		char := char

		// Jitter between DMs to avoid Matrix rate limits.
		// Skip delay before the first one.
		if i > 0 {
			time.Sleep(time.Duration(1000+rand.IntN(2000)) * time.Millisecond)
		}

		// Check if dead and ready to respawn (before babysit check so
		// dead+babysitting characters don't stay stuck dead forever).
		if !char.Alive && char.DeadUntil != nil && now.After(*char.DeadUntil) {
			char.Alive = true
			char.DeadUntil = nil
			if err := saveAdvCharacter(&char); err != nil {
				slog.Error("adventure: failed to revive character", "user", char.UserID, "err", err)
				continue
			}

			// Send respawn DM
			text := renderAdvRespawnDM(&char)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send respawn DM", "user", char.UserID, "err", err)
			}
		}

		// Babysitting: auto-resolve daily action, skip DM
		if char.BabysitActive {
			if !char.Alive {
				// Dead and not yet ready to respawn — skip babysit action
				continue
			}
			p.runBabysitDaily(&char)
			continue
		}

		// If still dead, send death status
		if !char.Alive {
			text := renderAdvDeathStatusDM(&char)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send death status DM", "user", char.UserID, "err", err)
			}
			continue
		}

		// If already acted today, skip
		if char.ActionTakenToday {
			continue
		}

		// Send morning DM with choices
		equip, err := loadAdvEquipment(char.UserID)
		if err != nil {
			slog.Error("adventure: failed to load equipment for morning DM", "user", char.UserID, "err", err)
			continue
		}

		treasures, _ := loadAdvTreasureBonuses(char.UserID)
		buffs, _ := loadAdvActiveBuffs(char.UserID)
		bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
		balance := p.euro.GetBalance(char.UserID)

		holidayLabel := ""
		if isHol {
			holidayLabel = holName
		}
		text := renderAdvMorningDM(&char, equip, balance, bonuses, holidayLabel)
		p.advMarkMenuSent(char.UserID)
		if err := p.SendDM(char.UserID, text); err != nil {
			slog.Error("adventure: failed to send morning DM", "user", char.UserID, "err", err)
			continue
		}

		// Register DM room for reply routing
		p.registerDMRoom(char.UserID)
	}
}

// ── Evening Summary Ticker ───────────────────────────────────────────────────

func (p *AdventurePlugin) summaryTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		if now.Hour() != p.summaryHour || now.Minute() != 0 {
			continue
		}

		dateKey := now.Format("2006-01-02")
		jobName := "adventure_summary"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}

		slog.Info("adventure: posting daily summary")
		p.postDailySummary()
		db.MarkJobCompleted(jobName, dateKey)
	}
}

func (p *AdventurePlugin) postDailySummary() {
	gr := gamesRoom()
	if gr == "" {
		return
	}

	// Run TwinBee daily action
	tbResult := p.runTwinBeeDaily()
	tbRewards := p.distributeTwinBeeRewards(tbResult)

	// Load all characters and today's logs
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("adventure: failed to load characters for summary", "err", err)
		return
	}

	todayLogs, _ := loadAdvTodayLogs()
	// Group logs per user — holiday days may produce 2 entries per user
	logsPerUser := make(map[id.UserID][]*AdvDayLog)
	for i := range todayLogs {
		uid := todayLogs[i].UserID
		logsPerUser[uid] = append(logsPerUser[uid], &todayLogs[i])
	}
	// logMap picks the last (most recent) log entry for summary display
	// lootSums aggregates loot across all actions (relevant on holiday double-action days)
	logMap := make(map[id.UserID]*AdvDayLog)
	lootSums := make(map[id.UserID]int64)
	for uid, logs := range logsPerUser {
		logMap[uid] = logs[len(logs)-1]
		var total int64
		for _, l := range logs {
			total += l.LootValue
		}
		lootSums[uid] = total
	}

	isHol, holName := isHolidayToday()

	// Build player summaries
	var players []AdvPlayerDaySummary
	for _, c := range chars {
		ps := AdvPlayerDaySummary{
			DisplayName:   c.DisplayName,
			CombatLevel:   c.CombatLevel,
			MiningSkill:   c.MiningSkill,
			ForagingSkill: c.ForagingSkill,
			FishingSkill:  c.FishingSkill,
		}

		// Holiday action count from log entries
		if isHol {
			ps.HolidayActions = len(logsPerUser[c.UserID])
		}

		if !c.Alive {
			ps.IsDead = true
			if c.DeadUntil != nil {
				ps.DeadUntil = c.DeadUntil.Format("15:04") + " UTC"
			}
			// Check if they died today
			if log, ok := logMap[c.UserID]; ok {
				ps.Activity = log.ActivityType
				ps.Location = log.Location
				ps.Outcome = log.Outcome
				ps.LootValue = lootSums[c.UserID] // aggregate across all actions
				ps.SummaryLine = advSummaryOneLiner(c.UserID, AdvActivityType(log.ActivityType), AdvOutcomeType(log.Outcome), lootSums[c.UserID], log.Location)
			}
			players = append(players, ps)
			continue
		}

		if !c.ActionTakenToday {
			ps.IsResting = true
			if len(SummaryResting) > 0 {
				ps.SummaryLine = SummaryResting[time.Now().Nanosecond()%len(SummaryResting)]
			}
			players = append(players, ps)
			continue
		}

		// Active player with today's log
		if log, ok := logMap[c.UserID]; ok {
			ps.Activity = log.ActivityType
			ps.Location = log.Location
			ps.Outcome = log.Outcome
			ps.LootValue = lootSums[c.UserID] // aggregate across all actions
			ps.SummaryLine = advSummaryOneLiner(c.UserID, AdvActivityType(log.ActivityType), AdvOutcomeType(log.Outcome), lootSums[c.UserID], log.Location)
		}

		players = append(players, ps)
	}

	// Check party bonuses and add to summary
	for i := range players {
		if players[i].Location != "" && !players[i].IsResting {
			for j := i + 1; j < len(players); j++ {
				if players[j].Location == players[i].Location && !players[j].IsResting {
					players[i].SummaryLine += fmt.Sprintf(" (Party bonus with %s!)", players[j].DisplayName)
					players[j].SummaryLine += fmt.Sprintf(" (Party bonus with %s!)", players[i].DisplayName)
				}
			}
		}
	}

	date := time.Now().UTC().Format("2006-01-02")
	summaryHolName := ""
	if isHol {
		summaryHolName = holName
	}
	summary := renderAdvDailySummary(date, tbResult, tbRewards, players, summaryHolName)

	if err := p.SendMessage(id.RoomID(gr), summary); err != nil {
		slog.Error("adventure: failed to post daily summary", "err", err)
	}
}

// ── Midnight Reset Ticker ────────────────────────────────────────────────────

func (p *AdventurePlugin) midnightTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		if now.Hour() != 0 || now.Minute() != 0 {
			continue
		}

		dateKey := now.Format("2006-01-02")
		jobName := "adventure_midnight"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}

		slog.Info("adventure: midnight reset")
		if err := p.midnightReset(); err != nil {
			slog.Error("adventure: midnight reset failed, will retry next tick", "err", err)
			continue
		}
		db.MarkJobCompleted(jobName, dateKey)
	}
}

func (p *AdventurePlugin) midnightReset() error {
	// Send idle shame DMs to players who didn't act
	chars, err := loadAllAdvCharacters()
	if err != nil {
		return fmt.Errorf("load chars: %w", err)
	}

	today := time.Now().UTC().Format("2006-01-02")

	dmsSent := 0
	for _, char := range chars {
		// Dead players freeze their streak — death is involuntary, don't punish it.
		if !char.Alive {
			continue
		}

		if !char.ActionTakenToday {
			// If the player died today (or yesterday — covering late-night deaths
			// that span midnight), grant a grace period: no shame, no streak reset.
			if char.LastDeathDate == today ||
				char.LastDeathDate == time.Now().UTC().Add(-24*time.Hour).Format("2006-01-02") {
				continue
			}

			// Jitter between DMs to avoid Matrix rate limits
			if dmsSent > 0 {
				time.Sleep(time.Duration(1000+rand.IntN(2000)) * time.Millisecond)
			}
			dmsSent++

			// Idle shame DM
			text := renderAdvIdleShameDM(&char)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send idle shame DM", "user", char.UserID, "err", err)
			}

			// Reset streak
			if char.CurrentStreak > 0 {
				char.CurrentStreak = 0
				_ = saveAdvCharacter(&char)
			}
		} else {
			// Update streak — LastActionDate was set at action time
			yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
			if char.LastActionDate == yesterday || char.LastActionDate == today {
				char.CurrentStreak++
			} else {
				// Gap in activity — start fresh
				char.CurrentStreak = 1
			}
			if char.CurrentStreak > char.BestStreak {
				char.BestStreak = char.CurrentStreak
			}
			_ = saveAdvCharacter(&char)
		}
	}

	// Reset all daily actions — retry up to 3 times to handle SQLite busy errors
	// from concurrent writers (e.g. reminder fire loop).
	var resetErr error
	for attempt := 0; attempt < 3; attempt++ {
		if resetErr = resetAllAdvDailyActions(); resetErr == nil {
			break
		}
		slog.Warn("adventure: daily action reset failed, retrying", "attempt", attempt+1, "err", resetErr)
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	if resetErr != nil {
		return fmt.Errorf("reset daily actions after 3 attempts: %w", resetErr)
	}

	// Prune expired buffs
	if err := pruneAdvExpiredBuffs(); err != nil {
		slog.Error("adventure: failed to prune expired buffs", "err", err)
	}

	// Clear flavor history to prevent unbounded memory growth.
	// Entries are only used for dedup within a day, so clearing at midnight is fine.
	advClearFlavorHistory()

	// Clear DM reminder dedup — entries are date-keyed so stale after midnight.
	p.dmRemindedDate.Range(func(key, _ any) bool {
		p.dmRemindedDate.Delete(key)
		return true
	})

	// Expire any rival challenges that went unanswered
	p.expireRivalChallenges()

	// Check babysitting service expirations
	p.checkBabysitExpiry(chars)

	return nil
}

// ── Helper ───────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) registerDMRoom(userID id.UserID) {
	room, err := p.GetDMRoom(userID)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.dmToPlayer[room] = userID
	p.mu.Unlock()
}
