package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Morning DM Ticker ────────────────────────────────────────────────────────

func (p *AdventurePlugin) morningTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
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
			text := renderAdvRespawnDM(char.UserID)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send respawn DM", "user", char.UserID, "err", err)
			}
		}

		// Babysitting: pet-care trickle (no harvest actions; safe-rest perk
		// is consumed inside the expedition camp/rest path). Still skips the
		// morning DM — the babysitter is handling things in the background.
		if char.BabysitActive {
			if !char.Alive {
				continue
			}
			p.runBabysitDailyTrickle(&char)
			if err := saveAdvCharacter(&char); err != nil {
				slog.Error("babysit: failed to save after daily trickle", "user", char.UserID, "err", err)
			}
			continue
		}

		// Mid-fight: a turn-based elite/boss session locks the run. The
		// per-round combat DMs are the player's feed right now — don't talk
		// over them with the overworld morning menu. (A combat session always
		// sits inside an active expedition, so the expedition skip below would
		// usually catch this too; the explicit guard is cheap insurance.)
		if char.Alive && hasActiveCombatSession(char.UserID) {
			continue
		}

		// Active expedition: the expedition cycle delivers its own morning
		// briefing at 06:00 UTC (deliverBriefing). The legacy overworld
		// morning DM is irrelevant — and confusing — while underground.
		if char.Alive {
			if exp, err := getActiveExpedition(char.UserID); err != nil {
				slog.Warn("adventure: failed to check active expedition for morning DM", "user", char.UserID, "err", err)
			} else if exp != nil {
				continue
			}
		}

		// If still dead, send death status
		if !char.Alive {
			text := renderAdvDeathStatusDM(char.UserID)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send death status DM", "user", char.UserID, "err", err)
			}
			continue
		}

		// If all actions used today, skip
		isHol, _ := isHolidayToday()
		if char.AllActionsUsed(isHol) {
			continue
		}

		// Pet arrival check (fires before normal morning DM)
		house, _ := loadHouseState(char.UserID)
		pet, _ := loadPetState(char.UserID)
		if petShouldArrive(pet, house) {
			p.petArrivalDM(char.UserID)
			continue
		}

		// Morning pet event
		petEvent := petMorningEvent(pet)
		if petEvent != "" {
			char.PetMorningDefense = true
			_ = saveAdvCharacter(&char)
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
		text := renderAdvMorningDM(char.UserID, equip, balance, bonuses, holidayLabel)
		if petEvent != "" {
			text = fmt.Sprintf("🐾 *%s*\n\n%s", petEvent, text)
		}
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

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
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

	// Load activity for today and yesterday — players may act across the UTC
	// boundary. Activity unifies legacy adventure_activity_log + dnd_zone_run
	// + dnd_expedition_log so the report sees every action regardless of
	// which subsystem produced it.
	now := time.Now().UTC()
	todayActs, _ := loadAdvDailyActivity(now.Format("2006-01-02"))
	yesterdayActs, _ := loadAdvDailyActivity(now.AddDate(0, 0, -1).Format("2006-01-02"))
	activityMap := make(map[id.UserID][]AdvDailyActivity)
	for uid, e := range yesterdayActs {
		activityMap[uid] = append(activityMap[uid], e...)
	}
	for uid, e := range todayActs {
		activityMap[uid] = append(activityMap[uid], e...)
	}
	lootSums := make(map[id.UserID]int64)
	for uid, acts := range activityMap {
		for _, a := range acts {
			lootSums[uid] += a.LootValue
		}
	}

	isHol, holName := isHolidayToday()

	// Build player summaries
	var players []AdvPlayerDaySummary
	for _, c := range chars {
		dispName, _ := loadDisplayName(c.UserID)
		ps := AdvPlayerDaySummary{
			DisplayName:   dispName,
			Level:         c.CombatLevel,
			MiningSkill:   c.MiningSkill,
			ForagingSkill: c.ForagingSkill,
			FishingSkill:  c.FishingSkill,
		}
		if dnd, _ := LoadDnDCharacter(c.UserID); dnd != nil {
			ps.HasDnDChar = true
			ps.DnDLevel = dnd.Level
			ps.DnDRace = string(dnd.Race)
			ps.DnDClass = string(dnd.Class)
			ps.HPCurrent = dnd.HPCurrent
			ps.HPMax = dnd.HPMax
		}

		acts := activityMap[c.UserID]

		// Holiday action count from activity entries
		if isHol {
			ps.HolidayActions = len(acts)
		}

		if !c.Alive {
			ps.IsDead = true
			ps.DeathSource = c.DeathSource
			ps.DeathLocation = c.DeathLocation
			if c.DeadUntil != nil {
				ps.DeadUntil = c.DeadUntil.Format("15:04") + " UTC"
				remaining := time.Until(*c.DeadUntil)
				if remaining > 0 {
					hrs := int(remaining.Hours())
					mins := int(remaining.Minutes()) - hrs*60
					// {hours}: round half-up integer hours.
					ps.DeadUntilHours = int(remaining.Hours() + 0.5)
					// {duration}: precise — "5h 27m", "47m", "2h".
					switch {
					case hrs > 0 && mins > 0:
						ps.DeadUntilDuration = fmt.Sprintf("%dh %dm", hrs, mins)
					case hrs > 0:
						ps.DeadUntilDuration = fmt.Sprintf("%dh", hrs)
					default:
						ps.DeadUntilDuration = fmt.Sprintf("%dm", mins)
					}
				}
			}
			if len(acts) > 0 {
				last := acts[len(acts)-1]
				ps.Activity = last.Activity
				ps.Location = last.Location
				ps.Outcome = last.Outcome
				ps.LootValue = lootSums[c.UserID]
				ps.SummaryLine = last.Summary
			}
			players = append(players, ps)
			continue
		}

		if len(acts) == 0 {
			ps.IsResting = true
			if len(SummaryResting) > 0 {
				ps.SummaryLine = SummaryResting[time.Now().Nanosecond()%len(SummaryResting)]
			}
			players = append(players, ps)
			continue
		}

		// Active player — represent with most-recent activity row.
		last := acts[len(acts)-1]
		ps.Activity = last.Activity
		ps.Location = last.Location
		ps.Outcome = last.Outcome
		ps.LootValue = lootSums[c.UserID]
		ps.SummaryLine = last.Summary

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

	lastRanDate := ""

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			dateKey := time.Now().UTC().Format("2006-01-02")
			if dateKey == lastRanDate {
				continue
			}

			// New UTC day — check DB in case we already ran (e.g. bot restart).
			jobName := "adventure_midnight"
			if db.JobCompleted(jobName, dateKey) {
				lastRanDate = dateKey
				continue
			}

			slog.Info("adventure: midnight reset")
			if err := p.midnightReset(); err != nil {
				slog.Error("adventure: midnight reset failed, will retry next tick", "err", err)
				continue
			}
			db.MarkJobCompleted(jobName, dateKey)
			lastRanDate = dateKey
		}
	}
}

func (p *AdventurePlugin) midnightReset() error {
	// Send idle shame DMs to players who didn't act
	chars, err := loadAllAdvCharacters()
	if err != nil {
		return fmt.Errorf("load chars: %w", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")

	dmsSent := 0
	for _, char := range chars {
		if !char.HasActedToday() {
			// If the player died today or yesterday, they couldn't act — no shame,
			// no streak reset. This covers both currently-dead players and players
			// who were just revived at midnight (Alive already flipped to true by
			// the reminder loop before midnightReset runs).
			if char.LastDeathDate == today || char.LastDeathDate == yesterday {
				continue
			}

			// An active expedition — or a turn-based fight locked open across
			// midnight — counts as activity. Both track their own action flow
			// (zone/harvest/combat/transit/extract) and never touch the legacy
			// CombatActionsUsed/HarvestActionsUsed counters, so HasActedToday()
			// reports false. Treat them like the acted-today branch below:
			// advance the streak and bail out (no idle-shame, no streak decay).
			busy := false
			if exp, err := getActiveExpedition(char.UserID); err != nil {
				slog.Warn("adventure: failed to check active expedition for idle reaper", "user", char.UserID, "err", err)
			} else if exp != nil {
				busy = true
			}
			if !busy && hasActiveCombatSession(char.UserID) {
				busy = true
			}
			if busy {
				if char.LastActionDate == yesterday || char.LastActionDate == today {
					char.CurrentStreak++
				} else {
					char.CurrentStreak = 1
				}
				if char.CurrentStreak > char.BestStreak {
					char.BestStreak = char.CurrentStreak
				}
				char.LastActionDate = today
				_ = saveAdvCharacter(&char)
				continue
			}

			// Jitter between DMs to avoid Matrix rate limits
			if dmsSent > 0 {
				time.Sleep(time.Duration(1000+rand.IntN(2000)) * time.Millisecond)
			}
			dmsSent++

			// Idle shame DM
			text := renderAdvIdleShameDM(char.UserID)
			if char.CurrentStreak > 0 {
				oldStreak := char.CurrentStreak
				char.CurrentStreak /= 2
				char.StreakDecayed = true
				text += fmt.Sprintf("\n\n🔥 Streak: %d → %d days", oldStreak, char.CurrentStreak)
				if char.CurrentStreak > 0 {
					text += " — not all is lost."
				}
				_ = saveAdvCharacter(&char)
			}
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: failed to send idle shame DM", "user", char.UserID, "err", err)
			}
		} else {
			// Update streak — LastActionDate was set at action time
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
		if resetErr = resetAllPlayerMetaDailyActions(); resetErr == nil {
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

	// Reset NPC message counts and regenerate roll targets
	npcMidnightReset()

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

	// Pet supply shop unlock check
	p.petMidnightCheck()

	// Reset holdem NPC house balance
	resetNPCHouseBalance()

	// Daily database backup (async to avoid blocking reset if DM sends are slow)
	go func() {
		if err := db.Backup(); err != nil {
			slog.Error("adventure: daily backup failed", "err", err)
			p.notifyAdmins(fmt.Sprintf("⚠️ **Daily backup failed:** %v", err))
		}
	}()

	return nil
}

func (p *AdventurePlugin) notifyAdmins(msg string) {
	admins := os.Getenv("ADMIN_USERS")
	if admins == "" {
		return
	}
	for _, a := range strings.Split(admins, ",") {
		uid := id.UserID(strings.TrimSpace(a))
		if uid == "" {
			continue
		}
		if err := p.SendDM(uid, msg); err != nil {
			slog.Error("adventure: failed to notify admin", "user", uid, "err", err)
		}
	}
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
