package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Arena Run State ─────────────────────────────────────────────────────────

type ArenaRun struct {
	ID              int64
	UserID          id.UserID
	RoomID          id.RoomID
	StartTier       int
	Tier            int
	Round           int
	Status          string // "active", "awaiting", "completed", "dead", "cashed_out"
	Earnings        int64
	RoundsSurvived  int
	LastMonster     string
	StartedAt       time.Time
	EndedAt         *time.Time
}

// ── Command Dispatch ────────────────────────────────────────────────────────

func (p *AdventurePlugin) dispatchArenaCommand(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "arena"))
	lower := strings.ToLower(args)

	switch {
	case args == "" || lower == "menu":
		return p.handleArenaMenu(ctx)
	case strings.HasPrefix(lower, "tier "):
		return p.handleArenaTier(ctx, strings.TrimSpace(args[5:]))
	case lower == "fight" || lower == "f":
		return p.handleArenaFight(ctx)
	case lower == "descend":
		return p.handleArenaDescend(ctx)
	case lower == "cashout":
		return p.handleArenaCashout(ctx)
	case lower == "cancel":
		return p.handleArenaCancel(ctx)
	case lower == "status":
		return p.handleArenaStatus(ctx)
	case lower == "stats":
		return p.handleArenaStats(ctx)
	case lower == "leaderboard" || lower == "lb":
		return p.handleArenaLeaderboard(ctx)
	case lower == "help":
		return p.SendDM(ctx.Sender, arenaHelpText)
	}

	return p.SendDM(ctx.Sender, "Unknown arena command. Type `!arena help` for available commands.")
}

const arenaHelpText = `**Arena Commands**

` + "`!arena`" + ` — Show tier menu and eligible tiers
` + "`!arena tier <1-5>`" + ` — Preview a tier (requires confirmation)
` + "`!arena fight`" + ` — Confirm entry or fight current round
` + "`!arena cancel`" + ` — Cancel pending tier entry
` + "`!arena descend`" + ` — Descend to next tier after clearing (earnings stay at risk)
` + "`!arena cashout`" + ` — Take your earnings and leave
` + "`!arena status`" + ` — Current run state
` + "`!arena stats`" + ` — Your personal arena stats
` + "`!arena leaderboard`" + ` — Top arena players
` + "`!arena help`" + ` — This message

The Arena is independent of your daily adventure action. You can run both on the same day. Death in the arena locks you out of both arena and adventure until midnight UTC.`

// ── Handlers ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleArenaMenu(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Type `!adventure` to create one first — the Arena is no place for the unregistered.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	// Clear any pending tier selection when viewing menu
	p.arenaPending.Delete(string(ctx.Sender))

	// Check for active run
	run, err := loadActiveArenaRun(ctx.Sender)
	if err == nil && run != nil {
		return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(run))
	}

	stats := loadArenaPersonalStats(ctx.Sender)
	return p.SendDM(ctx.Sender, renderArenaTierMenu(char, stats))
}

func (p *AdventurePlugin) handleArenaTier(ctx MessageContext, arg string) error {
	tierNum, err := strconv.Atoi(arg)
	if err != nil || tierNum < 1 || tierNum > 5 {
		return p.SendDM(ctx.Sender, "Invalid tier. Use `!arena tier <1-5>`.")
	}

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Type `!adventure` to create one first.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	// Check active run
	existing, _ := loadActiveArenaRun(ctx.Sender)
	if existing != nil {
		return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(existing))
	}

	tier := arenaGetTier(tierNum)
	if tier == nil {
		return p.SendDM(ctx.Sender, "Invalid tier.")
	}

	// Level gate
	if char.CombatLevel < tier.MinLevel {
		return p.SendDM(ctx.Sender, renderArenaLevelGate(tier, char.CombatLevel))
	}

	// Store pending tier selection — run is NOT created yet
	p.arenaPending.Store(string(ctx.Sender), tierNum)

	monster := arenaGetMonster(tierNum, 1)
	text := renderArenaTierConfirm(tier, monster)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleArenaFight(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	// Check for pending tier confirmation first
	if val, ok := p.arenaPending.LoadAndDelete(string(ctx.Sender)); ok {
		tierNum := val.(int)
		return p.confirmAndStartArenaRun(ctx, tierNum)
	}

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You don't have an active arena run. Start one with `!arena tier <1-5>`.")
	}

	if run.Status != "active" {
		if run.Status == "awaiting" {
			return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(run))
		}
		return p.SendDM(ctx.Sender, "Your arena run is not in a fightable state.")
	}

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load character.")
	}

	equip, _ := loadAdvEquipment(ctx.Sender)

	tier := arenaGetTier(run.Tier)
	monster := arenaGetMonster(run.Tier, run.Round)
	if tier == nil || monster == nil {
		return p.SendDM(ctx.Sender, "Arena data error. This shouldn't happen.")
	}

	// Resolve combat
	deathChance := arenaDeathChance(monster, char, equip)
	roll := rand.Float64()
	died := roll < deathChance

	// Generate combat log (cosmetic — outcome already determined)
	closeness := 1.0 - math.Abs(roll-deathChance)/math.Max(deathChance, 1-deathChance)
	combatLog := generateArenaCombatLog(!died, closeness)

	if died {
		return p.resolveArenaDeath(ctx, run, char, tier, monster, combatLog)
	}

	return p.resolveArenaSurvival(ctx, run, char, tier, monster, combatLog)
}

func (p *AdventurePlugin) confirmAndStartArenaRun(ctx MessageContext, tierNum int) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load character.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	// Re-check active run (could have changed since tier selection)
	existing, _ := loadActiveArenaRun(ctx.Sender)
	if existing != nil {
		return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(existing))
	}

	tier := arenaGetTier(tierNum)
	if tier == nil || char.CombatLevel < tier.MinLevel {
		return p.SendDM(ctx.Sender, "You are no longer eligible for this tier.")
	}

	// NOW create the run
	run := &ArenaRun{
		UserID:    ctx.Sender,
		RoomID:    ctx.RoomID,
		StartTier: tierNum,
		Tier:      tierNum,
		Round:     1,
		Status:    "active",
		Earnings:  0,
		StartedAt: time.Now().UTC(),
	}

	if err := createArenaRun(run); err != nil {
		slog.Error("arena: failed to create run", "user", ctx.Sender, "err", err)
		return p.SendDM(ctx.Sender, "Failed to start arena run.")
	}

	// Resolve round 1 immediately
	equip, _ := loadAdvEquipment(ctx.Sender)
	monster := arenaGetMonster(tierNum, 1)

	deathChance := arenaDeathChance(monster, char, equip)
	roll := rand.Float64()
	died := roll < deathChance

	closeness := 1.0 - math.Abs(roll-deathChance)/math.Max(deathChance, 1-deathChance)
	combatLog := generateArenaCombatLog(!died, closeness)

	if died {
		return p.resolveArenaDeath(ctx, run, char, tier, monster, combatLog)
	}

	return p.resolveArenaSurvival(ctx, run, char, tier, monster, combatLog)
}

func (p *AdventurePlugin) handleArenaCancel(ctx MessageContext) error {
	if _, ok := p.arenaPending.LoadAndDelete(string(ctx.Sender)); ok {
		return p.SendDM(ctx.Sender, "Tier entry cancelled. The Arena will wait.")
	}
	return p.SendDM(ctx.Sender, "Nothing to cancel.")
}

func (p *AdventurePlugin) handleArenaDescend(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil || run.Status != "awaiting" {
		return p.SendDM(ctx.Sender, "You don't have a pending tier decision.")
	}

	if run.Tier >= 5 {
		return p.SendDM(ctx.Sender, "There is nothing deeper. You've reached the bottom.")
	}

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load character.")
	}

	nextTier := arenaGetTier(run.Tier + 1)
	if nextTier == nil {
		return p.SendDM(ctx.Sender, "Invalid next tier.")
	}

	// Level gate for next tier
	if char.CombatLevel < nextTier.MinLevel {
		return p.SendDM(ctx.Sender, renderArenaLevelGate(nextTier, char.CombatLevel))
	}

	// Clear auto-cashout deadline
	p.arenaDeadlines.Delete(string(ctx.Sender))

	// Update run: advance to next tier
	run.Tier = nextTier.Number
	run.Round = 1
	run.Status = "active"
	if err := saveArenaRun(run); err != nil {
		return p.SendDM(ctx.Sender, "Failed to update arena run.")
	}

	// Grant descend achievement
	if p.achievements != nil {
		p.achievements.GrantAchievement(ctx.Sender, "arena_descend")
	}

	monster := arenaGetMonster(nextTier.Number, 1)
	text := renderArenaRoundStart(nextTier, 1, monster, run)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleArenaCashout(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil || run.Status != "awaiting" {
		return p.SendDM(ctx.Sender, "You don't have a pending tier decision.")
	}

	return p.arenaCompleteCashout(ctx.Sender, run, false)
}

func (p *AdventurePlugin) handleArenaStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found.")
	}

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "No active arena run. Start one with `!arena tier <1-5>`.")
	}

	return p.SendDM(ctx.Sender, renderArenaStatus(run, char))
}

func (p *AdventurePlugin) handleArenaStats(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Type `!adventure` to create one first.")
	}

	stats := loadArenaPersonalStats(ctx.Sender)
	return p.SendDM(ctx.Sender, renderArenaPersonalStats(char, stats))
}

func (p *AdventurePlugin) handleArenaLeaderboard(ctx MessageContext) error {
	entries, err := loadArenaLeaderboard()
	if err != nil {
		slog.Error("arena: failed to load leaderboard", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load arena leaderboard.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, renderArenaLeaderboard(entries))
}

// ── Combat Resolution ───────────────────────────────────────────────────────

func (p *AdventurePlugin) resolveArenaSurvival(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, tier *ArenaTier, monster *ArenaMonster, combatLog *ArenaCombatLog) error {
	// Calculate reward
	reward := arenaRoundReward(tier, run.Round, char.CombatLevel)
	run.Earnings += reward
	run.RoundsSurvived++
	run.LastMonster = monster.Name

	// Award battle XP (Ironclad set: Battle-Hardened — +5% XP)
	battleXP := tier.BattleXP
	equip, _ := loadAdvEquipment(ctx.Sender)
	if advEquippedArenaSets(equip)["ironclad"] {
		battleXP = int(float64(battleXP) * 1.05)
	}
	char.CombatXP += battleXP
	leveled, newLevel := checkAdvLevelUp(char, "combat")
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("arena: failed to save character after survival", "user", ctx.Sender, "err", err)
	}

	// Build survival message with combat log
	closer := arenaWinCloser(monster.Name, len(combatLog.Rounds))
	text := renderArenaCombatLog(combatLog, monster, true, reward, tier.BattleXP, closer)
	text += fmt.Sprintf("\nRun total: €%d\n", run.Earnings)
	if leveled {
		text += fmt.Sprintf("\n🎉 **Combat Level %d!**", newLevel)
	}

	// Achievement: first blood
	if run.RoundsSurvived == 1 && p.achievements != nil {
		p.achievements.GrantAchievement(ctx.Sender, "arena_first_blood")
	}

	// Achievement: Omega Mk. Zero defeated (T5R1 survival)
	if run.Tier == 5 && run.Round == 1 && p.achievements != nil {
		p.achievements.GrantAchievement(ctx.Sender, "arena_omega")
	}

	// Check if tier is complete (4 rounds)
	if run.Round >= 4 {
		// Add completion bonus
		run.Earnings += tier.CompletionBonus
		run.Round = 4 // keep at 4 for clarity

		if run.Tier >= 5 {
			// Tier 5 complete — full payout, run ends
			return p.arenaCompleteTier5(ctx, run, char, tier, text)
		}

		// Tier complete, awaiting decision
		run.Status = "awaiting"
		if err := saveArenaRun(run); err != nil {
			slog.Error("arena: failed to save run after tier complete", "user", ctx.Sender, "err", err)
		}

		// Set auto-cashout deadline
		deadline := time.Now().UTC().Add(10 * time.Minute)
		p.arenaDeadlines.Store(string(ctx.Sender), deadline)

		text += "\n\n" + renderArenaTierComplete(tier, tier.CompletionBonus, run.Earnings)

		// Check for arena helmet drop
		if dropped := p.arenaRollHelmetDrop(ctx.Sender, run.Tier); dropped != nil {
			text += "\n\n" + renderArenaHelmetDrop(dropped)
			p.postArenaDropAnnouncement(char.DisplayName, dropped)
		}

		// Grant tier achievement
		p.grantArenaTierAchievement(ctx.Sender, run.Tier)

		return p.SendDM(ctx.Sender, text)
	}

	// Advance to next round
	run.Round++
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to save run after round", "user", ctx.Sender, "err", err)
	}

	// Reveal next monster
	nextMonster := arenaGetMonster(run.Tier, run.Round)
	if nextMonster != nil {
		text += fmt.Sprintf("\n\n─────────────────────────────\n\n")
		text += fmt.Sprintf("**Round %d/4 — %s**\n", run.Round, nextMonster.Name)
		text += fmt.Sprintf("_%s_\n\n", nextMonster.Flavor)
		text += "`!arena fight` — Face this opponent"
	}

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) resolveArenaDeath(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, tier *ArenaTier, monster *ArenaMonster, combatLog *ArenaCombatLog) error {
	run.LastMonster = monster.Name

	// Sovereign set: Death's Reprieve — survive lethal arena outcome
	equip, _ := loadAdvEquipment(ctx.Sender)
	if advEquippedArenaSets(equip)["sovereign"] && char.DeathReprieveAvailable() {
		now := time.Now().UTC()
		char.DeathReprieveLast = &now
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("arena: failed to save character after reprieve", "user", ctx.Sender, "err", err)
		}

		// Gear absorbs the blow — all equipment set to 1 condition
		for _, slot := range allSlots {
			if eq, ok := equip[slot]; ok {
				eq.Condition = 1
				saveAdvEquipment(ctx.Sender, eq)
			}
		}

		// Run continues — not dead, earnings preserved
		nextWindow := now.Add(168 * time.Hour)
		gr := gamesRoom()
		if gr != "" {
			p.SendMessage(gr, renderArenaDeathReprieve(char.DisplayName, monster.Name, nextWindow))
		}

		// Show combat log (player "lost" but was saved by reprieve)
		closer := arenaLoseCloser(monster.Name, len(combatLog.Rounds))
		text := renderArenaCombatLog(combatLog, monster, false, 0, 0, closer)
		text += fmt.Sprintf("\n💀→⚔️ **%s nearly killed you.**\n\n"+
			"Your Sovereign gear activated **Death's Reprieve**. You survived — barely.\n"+
			"All equipment set to 1 condition.\n\n"+
			"Next reprieve window: %s\n",
			monster.Name, nextWindow.Format("2006-01-02 15:04 UTC"))

		run.RoundsSurvived++

		// Check if this was round 4 — tier completion via reprieve
		if run.Round >= 4 {
			run.Earnings += tier.CompletionBonus
			run.Round = 4

			if run.Tier >= 5 {
				// Tier 5 complete — hand off to the normal T5 completion path
				return p.arenaCompleteTier5(ctx, run, char, tier, text)
			}

			// Tier complete, awaiting descend/cashout
			run.Status = "awaiting"
			if err := saveArenaRun(run); err != nil {
				slog.Error("arena: failed to save run after reprieve tier complete", "user", ctx.Sender, "err", err)
			}

			deadline := time.Now().UTC().Add(10 * time.Minute)
			p.arenaDeadlines.Store(string(ctx.Sender), deadline)

			text += "\n" + renderArenaTierComplete(tier, tier.CompletionBonus, run.Earnings)

			// Check for arena helmet drop
			if dropped := p.arenaRollHelmetDrop(ctx.Sender, run.Tier); dropped != nil {
				text += "\n\n" + renderArenaHelmetDrop(dropped)
				p.postArenaDropAnnouncement(char.DisplayName, dropped)
			}

			p.grantArenaTierAchievement(ctx.Sender, run.Tier)
			return p.SendDM(ctx.Sender, text)
		}

		// Not round 4 — advance to next round
		run.Round++
		if err := saveArenaRun(run); err != nil {
			slog.Error("arena: failed to save run after reprieve", "user", ctx.Sender, "err", err)
		}

		text += fmt.Sprintf("\nRun earnings: €%d (still at risk)\n", run.Earnings)

		nextMonster := arenaGetMonster(run.Tier, run.Round)
		if nextMonster != nil {
			text += fmt.Sprintf("\n─────────────────────────────\n\n")
			text += fmt.Sprintf("**Round %d/4 — %s**\n", run.Round, nextMonster.Name)
			text += fmt.Sprintf("_%s_\n\n", nextMonster.Flavor)
			text += "`!arena fight` — Face this opponent"
		}

		return p.SendDM(ctx.Sender, text)
	}

	lostEarnings := run.Earnings

	// Kill the character (locked out until next midnight UTC)
	char.Alive = false
	now := time.Now().UTC()
	deadUntil := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	char.DeadUntil = &deadUntil
	char.ArenaLosses++
	char.CombatXP += arenaParticipationXP // +60 flat participation XP
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("arena: failed to save character after death", "user", ctx.Sender, "err", err)
	}

	// End the run
	run.Status = "dead"
	run.Earnings = 0
	run.EndedAt = &now
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to end arena run", "user", ctx.Sender, "err", err)
	}

	// Insert history
	insertArenaHistory(run.UserID, run.StartTier, run.Tier, run.RoundsSurvived, 0, "dead", monster.Name)

	// Update stats
	upsertArenaStats(run.UserID, 0, true, run.Tier)

	// Achievement: death in T5
	if run.Tier == 5 && p.achievements != nil {
		p.achievements.GrantAchievement(ctx.Sender, "arena_death_t5")
	}

	// Build death message with combat log
	closer := arenaLoseCloser(monster.Name, len(combatLog.Rounds))
	text := renderArenaCombatLog(combatLog, monster, false, 0, arenaParticipationXP, closer)
	text += fmt.Sprintf("\nLost earnings: €%d\n", lostEarnings)

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) arenaCompleteTier5(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, tier *ArenaTier, prefixText string) error {
	// Credit earnings
	p.euro.Credit(run.UserID, float64(run.Earnings), "arena_tier5_complete")
	char.ArenaWins++
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("arena: failed to save character after T5 complete", "user", ctx.Sender, "err", err)
	}

	// End run
	now := time.Now().UTC()
	run.Status = "completed"
	run.EndedAt = &now
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to end arena run after T5", "user", ctx.Sender, "err", err)
	}

	// History and stats
	insertArenaHistory(run.UserID, run.StartTier, 5, run.RoundsSurvived, run.Earnings, "completed", "That Which Has Always Been")
	upsertArenaStats(run.UserID, run.Earnings, false, 5)

	// Grant achievements
	p.grantArenaTierAchievement(ctx.Sender, 5)
	if run.StartTier == 1 && p.achievements != nil {
		p.achievements.GrantAchievement(ctx.Sender, "arena_full_run")
	}

	// Room announcement
	gr := gamesRoom()
	if gr != "" {
		announce := fmt.Sprintf("🏆 **%s has conquered the Arena.** Tier 5 cleared. €%d earned. That Which Has Always Been has fallen.",
			char.DisplayName, run.Earnings)
		p.SendMessage(id.RoomID(gr), announce)
	}

	text := prefixText + "\n\n" + renderArenaTier5Complete(run.Earnings, run.StartTier)

	// Check for arena helmet drop
	if dropped := p.arenaRollHelmetDrop(ctx.Sender, 5); dropped != nil {
		text += "\n\n" + renderArenaHelmetDrop(dropped)
		p.postArenaDropAnnouncement(char.DisplayName, dropped)
	}

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) arenaCompleteCashout(userID id.UserID, run *ArenaRun, isAuto bool) error {
	// Clear deadline
	p.arenaDeadlines.Delete(string(userID))

	// Credit earnings
	p.euro.Credit(userID, float64(run.Earnings), "arena_cashout")

	// Load char to update wins
	char, err := loadAdvCharacter(userID)
	if err == nil {
		char.ArenaWins++
		saveAdvCharacter(char)
	}

	// End run
	now := time.Now().UTC()
	run.Status = "cashed_out"
	run.EndedAt = &now
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to end arena run on cashout", "user", userID, "err", err)
	}

	// History and stats
	insertArenaHistory(userID, run.StartTier, run.Tier, run.RoundsSurvived, run.Earnings, "cashed_out", run.LastMonster)
	upsertArenaStats(userID, run.Earnings, false, run.Tier)

	// Achievement: big cashout
	if run.Earnings >= 10000 && p.achievements != nil {
		p.achievements.GrantAchievement(userID, "arena_cashout_big")
	}

	if isAuto {
		return p.SendDM(userID, renderArenaAutoCashout(run.Earnings))
	}
	return p.SendDM(userID, renderArenaCashout(run.Earnings, run.Tier))
}

// ── Combat Math ─────────────────────────────────────────────────────────────

func arenaDeathChance(monster *ArenaMonster, char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment) float64 {
	baseDeath := monster.BaseLethality
	levelMod := float64(monster.ThreatLevel-char.CombatLevel) * 0.015
	skillMod := math.Max(0, 0.25-float64(char.CombatLevel)*0.008)

	// Average equipment tier → up to 15% reduction at max gear (tier 5 avg = 0.15)
	var totalTier float64
	count := 0
	for _, slot := range allSlots {
		if eq, ok := equip[slot]; ok {
			totalTier += float64(eq.Tier)
			count++
		}
	}
	avgTier := 0.0
	if count > 0 {
		avgTier = totalTier / float64(count)
	}
	equipMod := avgTier * 0.03 // 0 at tier 0, 0.15 at tier 5

	deathChance := baseDeath + levelMod - equipMod + skillMod
	return math.Max(0.01, math.Min(0.98, deathChance))
}

func arenaRoundReward(tier *ArenaTier, round int, battleSkill int) int64 {
	base := tier.BasePayout * int64(round)
	skillBonus := int64(float64(battleSkill) * tier.SkillMultiplier)
	return base + skillBonus
}

// ── Achievement Helpers ─────────────────────────────────────────────────────

func (p *AdventurePlugin) grantArenaTierAchievement(userID id.UserID, tier int) {
	if p.achievements == nil {
		return
	}
	ids := map[int]string{
		1: "arena_tier1",
		2: "arena_tier2",
		3: "arena_tier3",
		4: "arena_tier4",
		5: "arena_tier5",
	}
	if achID, ok := ids[tier]; ok {
		p.achievements.GrantAchievement(userID, achID)
	}
}

// ── Death Message Selection ─────────────────────────────────────────────────

func arenaPickDeathMessage(monster *ArenaMonster, tier, round int) string {
	// 50% chance of generic, 50% monster-specific
	if rand.IntN(2) == 0 {
		return arenaDeathMessages[rand.IntN(len(arenaDeathMessages))]
	}

	template := arenaMonsterDeathMessages[rand.IntN(len(arenaMonsterDeathMessages))]
	r := strings.NewReplacer(
		"{monster}", monster.Name,
		"{tier}", strconv.Itoa(tier),
		"{round}", strconv.Itoa(round),
	)
	return r.Replace(template)
}

// ── Auto-Cashout Ticker ─────────────────────────────────────────────────────

func (p *AdventurePlugin) arenaAutoCashoutTicker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		p.arenaDeadlines.Range(func(key, value any) bool {
			userIDStr := key.(string)
			deadline := value.(time.Time)
			if now.After(deadline) {
				p.arenaDeadlines.Delete(userIDStr)
				go p.autoCollectArena(id.UserID(userIDStr))
			}
			return true
		})
	}
}

func (p *AdventurePlugin) autoCollectArena(userID id.UserID) {
	userMu := p.advUserLock(userID)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadActiveArenaRun(userID)
	if err != nil || run == nil || run.Status != "awaiting" {
		return
	}

	if err := p.arenaCompleteCashout(userID, run, true); err != nil {
		slog.Error("arena: auto-cashout failed", "user", userID, "err", err)
	}
}

// arenaCleanupStaleRuns auto-cashes out any runs left in 'awaiting' status
// (e.g. from a bot restart). Called during Init().
func (p *AdventurePlugin) arenaCleanupStaleRuns() {
	runs, err := loadAwaitingArenaRuns()
	if err != nil {
		slog.Error("arena: failed to load stale runs", "err", err)
		return
	}
	for _, run := range runs {
		run := run
		slog.Info("arena: auto-cashing out stale awaiting run", "user", run.UserID, "earnings", run.Earnings)
		if err := p.arenaCompleteCashout(run.UserID, &run, true); err != nil {
			slog.Error("arena: stale run cashout failed", "user", run.UserID, "err", err)
		}
	}
}

// ── DB CRUD ─────────────────────────────────────────────────────────────────

func loadActiveArenaRun(userID id.UserID) (*ArenaRun, error) {
	d := db.Get()
	run := &ArenaRun{}
	var startedAt int64
	var endedAt *int64
	err := d.QueryRow(`
		SELECT id, user_id, room_id, start_tier, tier, round, status, earnings,
		       rounds_survived, last_monster, started_at, ended_at
		FROM arena_runs
		WHERE user_id = ? AND status IN ('active', 'awaiting')
		ORDER BY id DESC LIMIT 1`, string(userID)).Scan(
		&run.ID, &run.UserID, &run.RoomID, &run.StartTier, &run.Tier, &run.Round,
		&run.Status, &run.Earnings, &run.RoundsSurvived, &run.LastMonster,
		&startedAt, &endedAt,
	)
	if err != nil {
		return nil, err
	}
	run.StartedAt = time.Unix(startedAt, 0).UTC()
	if endedAt != nil {
		t := time.Unix(*endedAt, 0).UTC()
		run.EndedAt = &t
	}
	return run, nil
}

func loadAwaitingArenaRuns() ([]ArenaRun, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT id, user_id, room_id, start_tier, tier, round, status, earnings,
		       rounds_survived, last_monster, started_at, ended_at
		FROM arena_runs WHERE status = 'awaiting'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ArenaRun
	for rows.Next() {
		var r ArenaRun
		var startedAt int64
		var endedAt *int64
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.RoomID, &r.StartTier, &r.Tier, &r.Round,
			&r.Status, &r.Earnings, &r.RoundsSurvived, &r.LastMonster,
			&startedAt, &endedAt,
		); err != nil {
			return nil, err
		}
		r.StartedAt = time.Unix(startedAt, 0).UTC()
		if endedAt != nil {
			t := time.Unix(*endedAt, 0).UTC()
			r.EndedAt = &t
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func createArenaRun(run *ArenaRun) error {
	d := db.Get()
	result, err := d.Exec(`
		INSERT INTO arena_runs (user_id, room_id, start_tier, tier, round, status, earnings,
		                        rounds_survived, last_monster, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(run.UserID), string(run.RoomID), run.StartTier, run.Tier, run.Round,
		run.Status, run.Earnings, run.RoundsSurvived, run.LastMonster,
		run.StartedAt.Unix(),
	)
	if err != nil {
		return err
	}
	run.ID, _ = result.LastInsertId()
	return nil
}

func saveArenaRun(run *ArenaRun) error {
	d := db.Get()
	var endedAt interface{}
	if run.EndedAt != nil {
		endedAt = run.EndedAt.Unix()
	}
	_, err := d.Exec(`
		UPDATE arena_runs SET
			tier = ?, round = ?, status = ?, earnings = ?,
			rounds_survived = ?, last_monster = ?, ended_at = ?
		WHERE id = ?`,
		run.Tier, run.Round, run.Status, run.Earnings,
		run.RoundsSurvived, run.LastMonster, endedAt,
		run.ID,
	)
	return err
}

func insertArenaHistory(userID id.UserID, startTier, tier, roundsSurvived int, earnings int64, outcome, monsterName string) {
	db.Exec("arena: insert history",
		`INSERT INTO arena_history (user_id, start_tier, tier, rounds_survived, earnings, outcome, monster_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(userID), startTier, tier, roundsSurvived, earnings, outcome, monsterName, time.Now().Unix(),
	)
}

func upsertArenaStats(userID id.UserID, earnings int64, died bool, highestTier int) {
	d := db.Get()
	now := time.Now().Unix()

	deathInc := 0
	if died {
		deathInc = 1
	}

	t5Inc := 0
	if highestTier == 5 && !died {
		t5Inc = 1
	}

	_, err := d.Exec(`
		INSERT INTO arena_stats (user_id, total_runs, total_earnings, total_deaths, highest_tier, tier5_completions, updated_at)
		VALUES (?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			total_runs = total_runs + 1,
			total_earnings = total_earnings + ?,
			total_deaths = total_deaths + ?,
			highest_tier = MAX(highest_tier, ?),
			tier5_completions = tier5_completions + ?,
			updated_at = ?`,
		string(userID), earnings, deathInc, highestTier, t5Inc, now,
		earnings, deathInc, highestTier, t5Inc, now,
	)
	if err != nil {
		slog.Error("arena: failed to upsert stats", "user", userID, "err", err)
	}
}

func loadArenaLeaderboard() ([]ArenaLeaderboardEntry, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT s.user_id, COALESCE(c.display_name, s.user_id),
		       s.total_earnings, s.highest_tier, s.tier5_completions,
		       s.total_runs, s.total_deaths
		FROM arena_stats s
		LEFT JOIN adventure_characters c ON c.user_id = s.user_id
		ORDER BY s.total_earnings DESC
		LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ArenaLeaderboardEntry
	for rows.Next() {
		var e ArenaLeaderboardEntry
		var uid string
		if err := rows.Scan(&uid, &e.DisplayName, &e.TotalEarnings, &e.HighestTier,
			&e.Tier5Completions, &e.TotalRuns, &e.TotalDeaths); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func loadArenaPersonalStats(userID id.UserID) *ArenaPersonalStats {
	d := db.Get()
	stats := &ArenaPersonalStats{}
	err := d.QueryRow(`
		SELECT total_runs, total_earnings, total_deaths, highest_tier, tier5_completions
		FROM arena_stats WHERE user_id = ?`, string(userID)).Scan(
		&stats.TotalRuns, &stats.TotalEarnings, &stats.TotalDeaths,
		&stats.HighestTier, &stats.Tier5Completions,
	)
	if err != nil {
		return nil
	}
	return stats
}

// ── Arena Gear ──────────────────────────────────────────────────────────────

type ArenaGearSet struct {
	Tier        int
	SetKey      string  // DB key: "bloodied", "ironclad", etc.
	SetName     string  // Display: "Bloodied", "Ironclad", etc.
	HelmetName  string
	Description string
	DropRate    float64
}

var arenaGearSets = [5]ArenaGearSet{
	{
		Tier: 1, SetKey: "bloodied", SetName: "Bloodied", HelmetName: "Bloodied Helm",
		Description: "A Brim & Battle VeriFort Series 1. The foam padding smells like a sporting goods store. " +
			"The chin strap is the kind used on baseball helmets, which this technically isn't anymore. " +
			"It held up. Brim & Battle would like you to know it held up.",
		DropRate: 0.05,
	},
	{
		Tier: 2, SetKey: "ironclad", SetName: "Ironclad", HelmetName: "Ironclad Helm",
		Description: "VeriFort Series 2. Brim & Battle made some adjustments after Series 1 feedback, " +
			"which they received exclusively from observing what happened to Series 1. " +
			"The rivets are new. The rivets are good.",
		DropRate: 0.04,
	},
	{
		Tier: 3, SetKey: "tempered", SetName: "Tempered", HelmetName: "Tempered Helm",
		Description: "VeriFort Series 3. At this point Brim & Battle has stopped calling these prototypes in public. " +
			"The internal documentation still says prototype. " +
			"The helmet does not know this and performs accordingly.",
		DropRate: 0.03,
	},
	{
		Tier: 4, SetKey: "champions", SetName: "Champion's", HelmetName: "Champion's Crown",
		Description: "VeriFort Series 4. Brim & Battle's premium tier, priced for the \"serious enthusiast combatant market\" " +
			"according to a pitch deck that was definitely never meant to be seen publicly. " +
			"The branding is subtle. The performance is not. " +
			"The QR code goes to a waitlist page for a product that does not yet exist.",
		DropRate: 0.02,
	},
	{
		Tier: 5, SetKey: "sovereign", SetName: "Sovereign", HelmetName: "Sovereign Crown",
		Description: "VeriFort Series 5. Brim & Battle did not design this. " +
			"They are not certain where it came from. It appeared in their warehouse inventory in Q3 " +
			"with no purchase order attached. They have claimed it anyway. " +
			"The feedback survey in the lining links to a page that returns a 404. " +
			"Brim & Battle appreciates all feedback.",
		DropRate: 0.005,
	},
}

func arenaGearByTier(tier int) *ArenaGearSet {
	if tier < 1 || tier > 5 {
		return nil
	}
	return &arenaGearSets[tier-1]
}

// arenaRollHelmetDrop checks if a helmet should drop and equips it if the player
// doesn't already have an arena helmet at this tier or higher. If they do, the
// drop is silently discarded (no duplicate drops).
// Returns the gear set if a drop was equipped, nil otherwise.
func (p *AdventurePlugin) arenaRollHelmetDrop(userID id.UserID, tier int) *ArenaGearSet {
	gear := arenaGearByTier(tier)
	if gear == nil {
		return nil
	}

	// Roll for drop
	if rand.Float64() >= gear.DropRate {
		return nil
	}

	// Check current helmet
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		slog.Error("arena: failed to load equipment for drop check", "user", userID, "err", err)
		return nil
	}

	helmet, hasHelmet := equip[SlotHelmet]
	if hasHelmet && helmet.ArenaTier >= tier {
		// Already has same or better arena helmet — silent discard
		return nil
	}

	// Equip the arena helmet
	if !hasHelmet {
		// Shouldn't happen (all slots created at character creation), but be safe
		helmet = &AdvEquipment{Slot: SlotHelmet}
	}
	helmet.Tier = tier
	helmet.Condition = 100
	helmet.Name = gear.HelmetName
	helmet.ActionsUsed = 0
	helmet.ArenaTier = tier
	helmet.ArenaSet = gear.SetKey

	if err := saveAdvEquipment(userID, helmet); err != nil {
		slog.Error("arena: failed to save arena helmet drop", "user", userID, "err", err)
		return nil
	}

	return gear
}

func (p *AdventurePlugin) postArenaDropAnnouncement(playerName string, gear *ArenaGearSet) {
	gr := gamesRoom()
	if gr == "" {
		return
	}
	var announce string
	if gear.Tier == 5 {
		announce = fmt.Sprintf("⚔️ **%s** has claimed **%s** from Tier 5 of the Arena. This is Sovereign gear. There are very few of these.",
			playerName, gear.HelmetName)
	} else {
		announce = fmt.Sprintf("⚔️ **%s** cleared Tier %d of the Arena and walked away with **%s**. %s Helmet. The monsters were unavailable for comment.",
			playerName, gear.Tier, gear.HelmetName, gear.SetName)
	}
	p.SendMessage(id.RoomID(gr), announce)
}
