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
	Earnings        int64  // session total (multiplied euros accumulated across completed tiers)
	TierEarnings    int64  // current tier's raw earnings (reset each tier)
	XPAccumulated   int    // session XP accumulator (raw, multiplied at payout)
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
	case lower == "fight" || lower == "f":
		return p.handleArenaFight(ctx)
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

` + "`!arena`" + ` — Enter the arena or view your current run
` + "`!arena fight`" + ` — Confirm entry or fight current round
` + "`!arena cancel`" + ` — Cancel pending entry
` + "`!bail`" + ` — Opt out between tiers and collect your rewards
` + "`!arena status`" + ` — Current run state
` + "`!arena stats`" + ` — Your personal arena stats
` + "`!arena leaderboard`" + ` — Top arena players
` + "`!arena help`" + ` — This message

The Arena is a streak. You start at Tier 1 and fight your way down. After each tier, you have 30 seconds to bail or you auto-advance. Death forfeits all accumulated rewards. The Arena is independent of your daily adventure actions.`

// ── Handlers ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleArenaMenu(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Type `!adventure` to create one first — the Arena is no place for the unregistered.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	// Clear any pending entry when viewing menu
	p.arenaPending.Delete(string(ctx.Sender))

	// Check for active run
	run, err := loadActiveArenaRun(ctx.Sender)
	if err == nil && run != nil {
		return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(run))
	}

	// Always start at Tier 1
	tier := arenaGetTier(1)

	// Store pending T1 entry — run is NOT created yet
	p.arenaPending.Store(string(ctx.Sender), 1)

	stats := loadArenaPersonalStats(ctx.Sender)
	monster := arenaGetMonster(1, 1)
	text := renderArenaStreakEntry(char, stats, tier, monster)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleArenaFight(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	// Check for pending entry confirmation first
	if _, ok := p.arenaPending.LoadAndDelete(string(ctx.Sender)); ok {
		return p.confirmAndStartArenaRun(ctx)
	}

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You don't have an active arena run. Type `!arena` to enter.")
	}

	if run.Status != "active" {
		if run.Status == "awaiting" {
			return p.SendDM(ctx.Sender, "Your next tier is loading. Type `!bail` to cash out instead.")
		}
		return p.SendDM(ctx.Sender, "Your arena run is not in a fightable state.")
	}

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. The arena requires living participants.")
	}

	equip, _ := loadAdvEquipment(ctx.Sender)

	tier := arenaGetTier(run.Tier)
	monster := arenaGetMonster(run.Tier, run.Round)
	if tier == nil || monster == nil {
		return p.SendDM(ctx.Sender, "Arena data error. This shouldn't happen.")
	}

	return p.resolveArenaRound(ctx, run, char, equip, tier, monster)
}

func (p *AdventurePlugin) confirmAndStartArenaRun(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load character.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	// Re-check active run (could have changed since entry prompt)
	existing, _ := loadActiveArenaRun(ctx.Sender)
	if existing != nil {
		return p.SendDM(ctx.Sender, renderArenaAlreadyInRun(existing))
	}

	// Always start at Tier 1
	tier := arenaGetTier(1)

	// NOW create the run
	run := &ArenaRun{
		UserID:    ctx.Sender,
		RoomID:    ctx.RoomID,
		StartTier: 1,
		Tier:      1,
		Round:     1,
		Status:    "active",
		StartedAt: time.Now().UTC(),
	}

	if err := createArenaRun(run); err != nil {
		slog.Error("arena: failed to create run", "user", ctx.Sender, "err", err)
		return p.SendDM(ctx.Sender, "Failed to start arena run.")
	}

	// Resolve round 1 immediately
	equip, _ := loadAdvEquipment(ctx.Sender)
	monster := arenaGetMonster(1, 1)

	return p.resolveArenaRound(ctx, run, char, equip, tier, monster)
}

// resolveArenaRound runs a single arena round through the combat engine and
// dispatches to survival or death handlers.
func (p *AdventurePlugin) resolveArenaRound(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, tier *ArenaTier, monster *ArenaMonster) error {
	result, condRepair := p.runArenaCombat(ctx.Sender, char, equip, monster, run.Round, run.Tier)

	// Apply event-based equipment degradation
	degradation := combatDegradation(result, equip)
	for slot, eq := range equip {
		if d, ok := degradation[slot]; ok && d > 0 {
			_ = saveAdvEquipment(ctx.Sender, eq)
		}
	}

	deathSaved := checkDeathSaveEvent(result.Events)

	if deathSaved {
		now := time.Now().UTC()
		char.DeathReprieveLast = &now
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("arena: failed to save character after reprieve", "user", ctx.Sender, "err", err)
		}
	}

	if !result.PlayerWon {
		return p.resolveArenaDeath(ctx, run, char, tier, monster, result)
	}

	// Misty condition repair (post-combat)
	if condRepair > 0 {
		npcRepairMostDamaged(ctx.Sender, equip, condRepair)
	}

	return p.resolveArenaSurvival(ctx, run, char, tier, monster, result)
}

func (p *AdventurePlugin) handleArenaCancel(ctx MessageContext) error {
	if _, ok := p.arenaPending.LoadAndDelete(string(ctx.Sender)); ok {
		return p.SendDM(ctx.Sender, "Tier entry cancelled. The Arena will wait.")
	}
	return p.SendDM(ctx.Sender, "Nothing to cancel.")
}

func (p *AdventurePlugin) handleArenaBail(ctx MessageContext) error {
	// Signal the countdown goroutine if one is running
	if ch, ok := p.arenaBailCh.LoadAndDelete(string(ctx.Sender)); ok {
		close(ch.(chan struct{}))
		// The countdown goroutine handles the actual payout
		return nil
	}

	// No countdown running — check if there's an awaiting run (stale state)
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You don't have an active arena session to bail from.")
	}

	if run.Status == "awaiting" {
		return p.arenaProcessBail(ctx.Sender, run)
	}

	if run.Status == "active" {
		return p.SendDM(ctx.Sender, "You're mid-fight. Finish the current tier first — you can only bail between tiers.")
	}

	return p.SendDM(ctx.Sender, "You don't have an active arena session to bail from.")
}

func (p *AdventurePlugin) handleArenaStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found.")
	}

	run, err := loadActiveArenaRun(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "No active arena run. Start one with `!arena`.")
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

// ── Streak Multipliers ──────────────────────────────────────────────────────

var arenaStreakEuroMultiplier = [6]float64{0, 1.0, 1.5, 2.0, 2.75, 4.0} // index = tier
var arenaStreakXPMultiplier = [6]float64{0, 1.0, 1.2, 1.5, 1.85, 2.5}   // index = tiers won

func (p *AdventurePlugin) resolveArenaSurvival(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, tier *ArenaTier, monster *ArenaMonster, result CombatResult) error {
	// Calculate reward — accumulate in tier earnings, not credited yet
	reward := arenaRoundReward(tier, run.Round, char.CombatLevel)
	run.TierEarnings += reward
	run.RoundsSurvived++
	run.LastMonster = monster.Name

	// Accumulate battle XP (Ironclad set: Battle-Hardened — +5% XP, chat level bonus)
	battleXP := tier.BattleXP
	equip, _ := loadAdvEquipment(ctx.Sender)
	if advEquippedArenaSets(equip)["ironclad"] {
		battleXP = int(float64(battleXP) * 1.05)
	}
	if bonus := chatLevelXPBonus(p.chatLevel(ctx.Sender)); bonus > 0 {
		battleXP = int(float64(battleXP) * (1.0 + bonus))
	}
	run.XPAccumulated += battleXP

	// Render combat log as phased messages + final outcome
	phaseMessages := RenderCombatLogArena(result, char.DisplayName, monster.Name)
	phaseMessages = p.prependCraftNarrative(ctx.Sender, phaseMessages)
	finalMessage := renderArenaCombatFinalMessage(result, monster, reward, battleXP, run.Round)

	// Suppress the "(at risk)" line on the tier-completing round — those earnings
	// are about to be locked in by the tier-complete branch below, so labelling
	// them as at-risk is stale the moment it's written.
	if run.Round < 4 {
		finalMessage += fmt.Sprintf("\nTier earnings: %s | Session total: %s (at risk)\n",
			fmtEuro(run.TierEarnings), fmtEuro(run.Earnings+run.TierEarnings))
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
		run.Round = 4

		tierRaw := run.TierEarnings
		run.TierEarnings += tier.CompletionBonus

		multiplier := arenaStreakEuroMultiplier[run.Tier]
		run.Earnings += int64(float64(run.TierEarnings) * multiplier)
		run.TierEarnings = 0

		p.grantArenaTierAchievement(ctx.Sender, run.Tier)

		if run.Tier >= 5 {
			run.Status = "completed"
			now := time.Now().UTC()
			run.EndedAt = &now
			if err := saveArenaRun(run); err != nil {
				slog.Error("arena: failed to save run after T5", "user", ctx.Sender, "err", err)
			}

			finalMessage += fmt.Sprintf("\n🏆 **Tier %d cleared!** Round earnings: €%d + completion bonus: €%d (×%.1f streak)\n",
				tier.Number, tierRaw, tier.CompletionBonus, multiplier)

			done := p.sendCombatMessages(ctx.Sender, phaseMessages, finalMessage)
			<-done
			return p.arenaCompleteSession(ctx.Sender, run, char, "")
		}

		run.Status = "awaiting"
		if err := saveArenaRun(run); err != nil {
			slog.Error("arena: failed to save run after tier complete", "user", ctx.Sender, "err", err)
		}

		finalMessage += fmt.Sprintf("\n🏆 **Tier %d cleared!** Round earnings: %s + completion bonus: %s (×%.1f streak)\n"+
			"Session total: %s (at risk)\n",
			tier.Number, fmtEuro(tierRaw), fmtEuro(tier.CompletionBonus), multiplier, fmtEuro(run.Earnings))

		if dropped := p.arenaRollHelmetDrop(ctx.Sender, run.Tier); dropped != nil {
			finalMessage += "\n" + renderArenaHelmetDrop(dropped)
			p.postArenaDropAnnouncement(char.DisplayName, dropped)
		}

		done := p.sendCombatMessages(ctx.Sender, phaseMessages, finalMessage)
		go func() {
			<-done
			p.arenaCountdown(ctx.Sender, run)
		}()
		return nil
	}

	// Advance to next round
	run.Round++
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to save run after round", "user", ctx.Sender, "err", err)
	}

	nextMonster := arenaGetMonster(run.Tier, run.Round)
	if nextMonster != nil {
		finalMessage += fmt.Sprintf("\n\n─────────────────────────────\n\n")
		finalMessage += fmt.Sprintf("**Round %d/4 — %s**\n", run.Round, nextMonster.Name)
		finalMessage += fmt.Sprintf("_%s_\n\n", nextMonster.Flavor)
		finalMessage += "`!arena fight` — Face this opponent"
	}

	p.sendCombatMessages(ctx.Sender, phaseMessages, finalMessage)
	return nil
}

func (p *AdventurePlugin) resolveArenaDeath(ctx MessageContext, run *ArenaRun, char *AdventureCharacter, tier *ArenaTier, monster *ArenaMonster, result CombatResult) error {
	run.LastMonster = monster.Name
	lostEarnings := run.Earnings + run.TierEarnings

	phaseMessages := RenderCombatLogArena(result, char.DisplayName, monster.Name)

	dt := transitionDeath(DeathTransitionParams{
		Char:          char,
		Source:        "arena",
		DeathLocation: "the Arena",
	})

	char.ArenaLosses++
	char.CombatXP += arenaParticipationXP
	if leveled, _ := checkAdvLevelUp(char, "combat"); leveled {
		p.checkRivalPoolUnlock(char)
	}
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("arena: failed to save character after death", "user", ctx.Sender, "err", err)
	}

	now := time.Now().UTC()
	run.Status = "dead"
	run.Earnings = 0
	run.TierEarnings = 0
	run.XPAccumulated = 0
	run.EndedAt = &now
	if err := saveArenaRun(run); err != nil {
		slog.Error("arena: failed to end arena run", "user", ctx.Sender, "err", err)
	}
	insertArenaHistory(run.UserID, run.StartTier, run.Tier, run.RoundsSurvived, 0, "dead", monster.Name)
	upsertArenaStats(run.UserID, 0, true, run.Tier)

	finalMsg := renderArenaCombatFinalMessage(result, monster, 0, arenaParticipationXP, run.Round)
	if dt.PetRecovered {
		finalMsg += fmt.Sprintf("\n\nYour pet dragged you out of the arena. Death timer reduced. All session earnings forfeited.")
	} else {
		if run.Tier == 5 && p.achievements != nil {
			p.achievements.GrantAchievement(ctx.Sender, "arena_death_t5")
		}
		finalMsg += fmt.Sprintf("\nYou died in Tier %d. Everything you were carrying goes with you. The arena keeps its own ledger.\n", run.Tier)
		if lostEarnings > 0 {
			finalMsg += fmt.Sprintf("Forfeited: %s\n", fmtEuro(lostEarnings))
		}
	}

	done := p.sendCombatMessages(ctx.Sender, phaseMessages, finalMsg)

	if dt.PetRecovered {
		gr := gamesRoom()
		if gr != "" {
			_ = p.SendMessage(gr, petDitchRecoveryGameRoom(char.DisplayName, char.PetName, true))
		}
	}

	go func() {
		<-done
		p.sendHospitalAd(ctx.Sender, char)
	}()

	return nil
}

// arenaCompleteSession handles session payout for both T5 completion and bail.
// prefixText is the combat log / tier-complete text to prepend to the summary.
func (p *AdventurePlugin) arenaCompleteSession(userID id.UserID, run *ArenaRun, char *AdventureCharacter, prefixText string) error {
	tiersWon := run.Tier // tiers cleared in this session (always starts at 1)

	// Apply XP streak multiplier
	xpMult := 1.0
	if tiersWon >= 1 && tiersWon <= 5 {
		xpMult = arenaStreakXPMultiplier[tiersWon]
	}
	totalXP := int(float64(run.XPAccumulated) * xpMult)

	// Arena tax: 10% of earnings to community pot.
	arenaTax := int64(math.Round(float64(run.Earnings) * 0.1))
	arenaNet := run.Earnings - arenaTax
	if arenaTax > 0 {
		communityPotAdd(int(arenaTax))
		trackTaxPaid(userID, int(arenaTax))
	}
	p.euro.Credit(userID, float64(arenaNet), "arena_streak_payout")

	// Credit XP
	char.CombatXP += totalXP
	char.ArenaWins++
	leveled, newLevel := checkAdvLevelUp(char, "combat")
	if leveled {
		p.checkRivalPoolUnlock(char)
	}
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("arena: failed to save character after session complete", "user", userID, "err", err)
	}

	// End run if not already ended
	if run.Status != "completed" && run.Status != "cashed_out" {
		now := time.Now().UTC()
		run.Status = "cashed_out"
		run.EndedAt = &now
		if err := saveArenaRun(run); err != nil {
			slog.Error("arena: failed to end arena run on session complete", "user", userID, "err", err)
		}
	} else {
		if err := saveArenaRun(run); err != nil {
			slog.Error("arena: failed to save arena run on session complete", "user", userID, "err", err)
		}
	}

	// History and stats
	outcome := "cashed_out"
	lastMonster := run.LastMonster
	if run.Tier >= 5 && run.Status == "completed" {
		outcome = "completed"
		lastMonster = "That Which Has Always Been"
	}
	insertArenaHistory(userID, run.StartTier, run.Tier, run.RoundsSurvived, run.Earnings, outcome, lastMonster)
	upsertArenaStats(userID, run.Earnings, false, run.Tier)

	// Achievements
	if run.Earnings >= 10000 && p.achievements != nil {
		p.achievements.GrantAchievement(userID, "arena_cashout_big")
	}
	if run.Tier >= 5 {
		if p.achievements != nil {
			p.achievements.GrantAchievement(userID, "arena_full_run")
		}
		// Room announcement for T5
		gr := gamesRoom()
		if gr != "" {
			announce := fmt.Sprintf("🏆 **%s has conquered the Arena.** Tier 5 streak. €%d earned. That Which Has Always Been has fallen.",
				char.DisplayName, run.Earnings)
			p.SendMessage(id.RoomID(gr), announce)
		}
	}

	// Gladiator's Helm roll (Step 9)
	helmText := ""
	if tiersWon >= 4 {
		helmDrop := p.arenaRollGladiatorHelm(userID, tiersWon)
		if helmDrop != "" {
			helmText = helmDrop
			// Game room announcement
			gr := gamesRoom()
			if gr != "" {
				p.SendMessage(id.RoomID(gr), fmt.Sprintf(
					"🏆 %s walked out of the arena with the Gladiator's Helm. Tier %d streak. They had the option to stop. They did not stop.",
					char.DisplayName, tiersWon))
			}
		}
	}

	// Build payout summary DM
	text := ""
	if prefixText != "" {
		text = prefixText + "\n\n"
	}
	text += fmt.Sprintf("⚔️ **Arena Session Complete — %d tiers cleared**\n\n", tiersWon)
	text += fmt.Sprintf("Euros:     +%s (%s after 10%% arena tax → community pot)\n", fmtEuro(run.Earnings), fmtEuro(arenaNet))
	text += fmt.Sprintf("XP:        +%d (%.1f× streak bonus)\n", totalXP, xpMult)
	if helmText != "" {
		text += fmt.Sprintf("Helm drop: %s\n", helmText)
	} else {
		text += "Helm drop: —\n"
	}
	text += "\nYour rewards have been applied."
	if leveled {
		text += fmt.Sprintf("\n\n🎉 **Combat Level %d!**", newLevel)
	}

	return p.SendDM(userID, text)
}

// arenaProcessBail handles bail payout (called from handleArenaBail or countdown).
func (p *AdventurePlugin) arenaProcessBail(userID id.UserID, run *ArenaRun) error {
	char, err := loadAdvCharacter(userID)
	if err != nil {
		slog.Error("arena: failed to load character for bail", "user", userID, "err", err)
		return p.SendDM(userID, "Failed to process bail. Your session state is preserved.")
	}

	now := time.Now().UTC()
	run.Status = "cashed_out"
	run.EndedAt = &now

	return p.arenaCompleteSession(userID, run, char, "")
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

	// Housing HP bonus reduces death chance
	houseMod := char.HouseHPBonus() // 0-20% based on house tier

	// Pet morning defense buff (cat offering / dog smothering)
	petDefMod := 0.0
	if char.PetMorningDefense {
		petDefMod = 0.05
	}

	deathChance := baseDeath + levelMod - equipMod + skillMod - houseMod - petDefMod
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

// ── Arena Countdown (30-second opt-out timer) ──────────────────────────────

func (p *AdventurePlugin) arenaCountdown(userID id.UserID, run *ArenaRun) {
	// Create bail channel — ownership is transferred to either the bail handler
	// (via LoadAndDelete) or the timer expiry path. The defer is removed to
	// avoid racing with those paths.
	bailCh := make(chan struct{})
	p.arenaBailCh.Store(string(userID), bailCh)

	nextTier := arenaGetTier(run.Tier + 1)
	nextTierName := "unknown"
	if nextTier != nil {
		nextTierName = nextTier.Name
	}

	// Send countdown DM
	countdownText := fmt.Sprintf("⚔️ Tier %d cleared.\n\nEntering Tier %d — %s in 30 seconds. Type `!bail` to stop here and collect your rewards.\n\n▸ 30 seconds",
		run.Tier, run.Tier+1, nextTierName)
	msgID, err := p.SendDMID(userID, countdownText)
	if err != nil {
		slog.Error("arena: failed to send countdown DM", "user", userID, "err", err)
		// Fall through to auto-advance anyway
	}

	// 3 intervals of 10 seconds each
	for _, remaining := range []int{20, 10, 0} {
		select {
		case <-bailCh:
			// Player bailed — process payout
			userMu := p.advUserLock(userID)
			userMu.Lock()
			// Re-load run in case state changed
			freshRun, err := loadActiveArenaRun(userID)
			if err != nil || freshRun == nil || freshRun.Status != "awaiting" {
				userMu.Unlock()
				return
			}
			if err := p.arenaProcessBail(userID, freshRun); err != nil {
				slog.Error("arena: bail payout failed", "user", userID, "err", err)
			}
			userMu.Unlock()
			return
		case <-time.After(10 * time.Second):
			if remaining > 0 && msgID != "" {
				editText := fmt.Sprintf("⚔️ Tier %d cleared.\n\nEntering Tier %d — %s in %d seconds. Type `!bail` to stop here and collect your rewards.\n\n▸ %d seconds",
					run.Tier, run.Tier+1, nextTierName, remaining, remaining)
				if err := p.EditDM(userID, msgID, editText); err != nil {
					slog.Warn("arena: failed to edit countdown DM", "user", userID, "err", err)
				}
			}
		}
	}

	// Timer expired — remove bail channel to prevent late bail from closing it.
	// If LoadAndDelete fails, bail already claimed it — exit silently.
	if _, ok := p.arenaBailCh.LoadAndDelete(string(userID)); !ok {
		return // bail handler already took ownership
	}

	userMu := p.advUserLock(userID)
	userMu.Lock()
	defer userMu.Unlock()

	freshRun, err := loadActiveArenaRun(userID)
	if err != nil || freshRun == nil || freshRun.Status != "awaiting" {
		return
	}

	if nextTier == nil {
		// Shouldn't happen — T5 is handled before countdown starts
		p.arenaProcessBail(userID, freshRun)
		return
	}

	char, err := loadAdvCharacter(userID)
	if err != nil {
		slog.Error("arena: failed to load character for auto-advance", "user", userID, "err", err)
		p.arenaProcessBail(userID, freshRun)
		return
	}

	// Level gate for next tier
	if char.CombatLevel < nextTier.MinLevel {
		p.SendDM(userID, renderArenaLevelGate(nextTier, char.CombatLevel)+"\n\nYour accumulated rewards have been paid out.")
		p.arenaProcessBail(userID, freshRun)
		return
	}

	// Advance to next tier
	freshRun.Tier = nextTier.Number
	freshRun.Round = 1
	freshRun.Status = "active"
	if err := saveArenaRun(freshRun); err != nil {
		slog.Error("arena: failed to advance to next tier", "user", userID, "err", err)
		return
	}

	// Grant descend achievement
	if p.achievements != nil {
		p.achievements.GrantAchievement(userID, "arena_descend")
	}

	monster := arenaGetMonster(nextTier.Number, 1)
	text := renderArenaRoundStart(nextTier, 1, monster, freshRun)
	p.SendDM(userID, text)
}

// ── Restart Recovery ────────────────────────────────────────────────────────

// arenaCleanupStaleRuns auto-bails any active/awaiting runs on restart.
// Per spec: do not forfeit due to infrastructure reasons.
func (p *AdventurePlugin) arenaCleanupStaleRuns() {
	runs, err := loadStaleArenaRuns()
	if err != nil {
		slog.Error("arena: failed to load stale runs", "err", err)
		return
	}
	for _, run := range runs {
		run := run
		slog.Info("arena: auto-bailing stale run on restart", "user", run.UserID, "tier", run.Tier, "earnings", run.Earnings)
		if err := p.arenaProcessBail(run.UserID, &run); err != nil {
			slog.Error("arena: stale run bail failed", "user", run.UserID, "err", err)
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
		       tier_earnings, xp_accumulated,
		       rounds_survived, last_monster, started_at, ended_at
		FROM arena_runs
		WHERE user_id = ? AND status IN ('active', 'awaiting')
		ORDER BY id DESC LIMIT 1`, string(userID)).Scan(
		&run.ID, &run.UserID, &run.RoomID, &run.StartTier, &run.Tier, &run.Round,
		&run.Status, &run.Earnings, &run.TierEarnings, &run.XPAccumulated,
		&run.RoundsSurvived, &run.LastMonster,
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

func loadStaleArenaRuns() ([]ArenaRun, error) {
	d := db.Get()
	rows, err := d.Query(`
		SELECT id, user_id, room_id, start_tier, tier, round, status, earnings,
		       tier_earnings, xp_accumulated,
		       rounds_survived, last_monster, started_at, ended_at
		FROM arena_runs WHERE status IN ('awaiting', 'active')`)
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
			&r.Status, &r.Earnings, &r.TierEarnings, &r.XPAccumulated,
			&r.RoundsSurvived, &r.LastMonster,
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
		                        tier_earnings, xp_accumulated,
		                        rounds_survived, last_monster, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(run.UserID), string(run.RoomID), run.StartTier, run.Tier, run.Round,
		run.Status, run.Earnings, run.TierEarnings, run.XPAccumulated,
		run.RoundsSurvived, run.LastMonster,
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
			tier_earnings = ?, xp_accumulated = ?,
			rounds_survived = ?, last_monster = ?, ended_at = ?
		WHERE id = ?`,
		run.Tier, run.Round, run.Status, run.Earnings,
		run.TierEarnings, run.XPAccumulated,
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

func arenaGearByName(name string) *ArenaGearSet {
	for i := range arenaGearSets {
		if arenaGearSets[i].HelmetName == name {
			return &arenaGearSets[i]
		}
	}
	return nil
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
	hasRealHelmet := hasHelmet && helmet.Tier > 0

	// Discard only if existing is a same-or-better arena helmet (duplicate suppression).
	if hasRealHelmet && helmet.ArenaTier >= tier {
		return nil
	}

	// If existing is a strictly better non-arena helmet, stash the drop in inventory
	// so the player can equip it later via !adventure equip rather than overwriting.
	if hasRealHelmet && helmet.ArenaTier == 0 && helmet.Tier > tier {
		if err := addAdvInventoryItem(userID, AdvItem{
			Name: gear.HelmetName,
			Type: "ArenaGear",
			Tier: tier,
			Slot: SlotHelmet,
		}); err != nil {
			slog.Error("arena: failed to stash arena helmet drop", "user", userID, "err", err)
			return nil
		}
		return gear
	}

	// Auto-equip: move the old helmet to inventory first (preserving its flavor)
	// so the player doesn't silently lose it.
	if hasRealHelmet {
		oldType := "ShopGear"
		if helmet.Masterwork {
			oldType = "MasterworkGear"
		} else if helmet.ArenaTier > 0 {
			oldType = "ArenaGear"
		}
		if err := addAdvInventoryItem(userID, AdvItem{
			Name:        helmet.Name,
			Type:        oldType,
			Tier:        helmet.Tier,
			Slot:        SlotHelmet,
			SkillSource: helmet.SkillSource,
		}); err != nil {
			slog.Error("arena: failed to move old helmet to inventory", "user", userID, "err", err)
		}
	}

	if !hasHelmet {
		helmet = &AdvEquipment{Slot: SlotHelmet}
	}
	helmet.Tier = tier
	helmet.Condition = 100
	helmet.Name = gear.HelmetName
	helmet.ActionsUsed = 0
	helmet.ArenaTier = tier
	helmet.ArenaSet = gear.SetKey
	helmet.Masterwork = false
	helmet.SkillSource = ""

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

// ── Gladiator's Helm (Streak-Exclusive Drop) ──────────────────────────────

const (
	gladiatorHelmDropT4 = 0.08
	gladiatorHelmDropT5 = 0.18
	gladiatorHelmTier   = 5 // equipment tier level
)

// arenaRollGladiatorHelm checks for the streak-exclusive Gladiator's Helm drop.
// Returns the DM text for the drop, or empty string if no drop.
func (p *AdventurePlugin) arenaRollGladiatorHelm(userID id.UserID, maxTierCleared int) string {
	if maxTierCleared < 4 {
		return ""
	}

	dropRate := gladiatorHelmDropT4
	if maxTierCleared >= 5 {
		dropRate = gladiatorHelmDropT5
	}

	if rand.Float64() >= dropRate {
		return ""
	}

	// Check if player already owns a Gladiator's Helm at full condition
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		slog.Error("arena: failed to load equipment for gladiator helm check", "user", userID, "err", err)
		return ""
	}

	helmet, hasHelmet := equip[SlotHelmet]
	if hasHelmet && helmet.Name == "Gladiator's Helm" && helmet.Condition == 100 {
		// Already owns at full condition — suppress silently
		return ""
	}

	// Equip the Gladiator's Helm (1.75x tier = effective tier 5 with bonus)
	if !hasHelmet {
		helmet = &AdvEquipment{Slot: SlotHelmet}
	}
	helmet.Tier = gladiatorHelmTier
	helmet.Condition = 100
	helmet.Name = "Gladiator's Helm"
	helmet.ActionsUsed = 0
	helmet.ArenaTier = gladiatorHelmTier
	helmet.ArenaSet = "gladiator"
	helmet.Masterwork = true // Masterwork-tier item

	if err := saveAdvEquipment(userID, helmet); err != nil {
		slog.Error("arena: failed to save gladiator helm", "user", userID, "err", err)
		return ""
	}

	return "The Gladiator's Helm"
}
