package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Flavor Text Selection ────────────────────────────────────────────────────

// flavorHistory tracks last N used indices per player per category to avoid repetition.
var flavorHistory sync.Map // key: "userID:category" -> []int

const flavorHistorySize = 5

func advPickFlavor(pool []string, userID id.UserID, category string) (string, int) {
	if len(pool) == 0 {
		return "", 0
	}
	if len(pool) == 1 {
		return pool[0], 0
	}

	key := string(userID) + ":" + category

	// Load history
	var history []int
	if val, ok := flavorHistory.Load(key); ok {
		history = val.([]int)
	}

	// Try to pick an index not in recent history
	var idx int
	for attempts := 0; attempts < 20; attempts++ {
		idx = rand.IntN(len(pool))
		if !intSliceContains(history, idx) {
			break
		}
	}

	// Update history
	history = append(history, idx)
	if len(history) > flavorHistorySize {
		history = history[len(history)-flavorHistorySize:]
	}
	flavorHistory.Store(key, history)

	return pool[idx], idx
}

func intSliceContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// advClearFlavorHistory resets the dedup tracker. Called at midnight reset.
func advClearFlavorHistory() {
	flavorHistory.Range(func(key, _ any) bool {
		flavorHistory.Delete(key)
		return true
	})
}

// advSubstituteFlavor replaces {var} placeholders in a flavor text string.
func advSubstituteFlavor(template string, vars map[string]string) string {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		// Prevent "The The Soggy Cellar" — if the value starts with "The "
		// and the template says "The {location}", replace that as a unit first.
		if strings.HasPrefix(v, "The ") {
			theKey := "The " + k
			if strings.Contains(template, theKey) {
				pairs = append(pairs, theKey, v)
			}
		}
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// ── Character Sheet ──────────────────────────────────────────────────────────

func renderAdvCharacterSheet(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, items []AdvItem, treasures []AdvTreasureBonus, balance float64) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("⚔️ **%s's Adventurer**\n\n", char.DisplayName))

	// Stats
	sb.WriteString("📊 Stats:\n")
	sb.WriteString(fmt.Sprintf("  Combat:  Lv.%d  (%d/%d XP)\n", char.CombatLevel, char.CombatXP, xpToNextLevel(char.CombatLevel)))
	sb.WriteString(fmt.Sprintf("  Mining:  Lv.%d  (%d/%d XP)\n", char.MiningSkill, char.MiningXP, xpToNextLevel(char.MiningSkill)))
	sb.WriteString(fmt.Sprintf("  Forage:  Lv.%d  (%d/%d XP)\n", char.ForagingSkill, char.ForagingXP, xpToNextLevel(char.ForagingSkill)))
	sb.WriteString(fmt.Sprintf("  Fishing: Lv.%d  (%d/%d XP)\n", char.FishingSkill, char.FishingXP, xpToNextLevel(char.FishingSkill)))

	// Status
	if char.Alive {
		sb.WriteString("  Status: Alive\n")
	} else if char.DeadUntil != nil {
		sb.WriteString(fmt.Sprintf("  Status: 💀 Dead — revives %s UTC\n", char.DeadUntil.Format("15:04")))
	}

	// Streak
	if char.CurrentStreak > 0 {
		sb.WriteString(fmt.Sprintf("  🔥 Streak: %d days", char.CurrentStreak))
		if char.CurrentStreak >= 30 {
			sb.WriteString(" 🏆")
		} else if char.CurrentStreak >= 14 {
			sb.WriteString(" ⭐")
		} else if char.CurrentStreak >= 7 {
			sb.WriteString(" 🔥")
		}
		sb.WriteString(fmt.Sprintf(" (best: %d)\n", char.BestStreak))
	}

	// Equipment
	sb.WriteString("\n🛡️ Equipment:\n")
	eqScore := advEquipmentScore(equip)
	for _, slot := range allSlots {
		eq := equip[slot]
		mastery := ""
		if eq != nil && eq.ActionsUsed >= 20 {
			mastery = " ✦"
		}
		if eq != nil {
			marker := ""
			if eq.Masterwork {
				marker = " ⭐"
			} else if eq.ArenaTier > 0 {
				marker = " ⚔️"
			}
			sb.WriteString(fmt.Sprintf("  %s %s: %s%s (Tier %d | %d%% condition%s)\n",
				slotEmoji(slot), slotTitle(slot), eq.Name, marker, eq.Tier, eq.Condition, mastery))
		}
	}
	sb.WriteString(fmt.Sprintf("  Equipment Score: %d\n", eqScore))

	// Treasures
	if len(treasures) > 0 {
		seen := make(map[string]bool)
		sb.WriteString("\n💎 Treasures:\n")
		for _, t := range treasures {
			if seen[t.TreasureKey] {
				continue
			}
			seen[t.TreasureKey] = true
			// Find def for inventory desc
			for tier, defs := range advAllTreasures {
				_ = tier
				for _, def := range defs {
					if def.Key == t.TreasureKey {
						sb.WriteString(fmt.Sprintf("  %s\n", def.InventoryDesc))
						break
					}
				}
			}
		}
	}

	// Inventory summary
	var invValue int64
	for _, item := range items {
		invValue += item.Value
	}
	sb.WriteString(fmt.Sprintf("\n🎒 Inventory: %d items (total value ~€%d)\n", len(items), invValue))
	sb.WriteString(fmt.Sprintf("💰 Balance: €%.0f\n", balance))

	// Babysit status
	if char.BabysitActive {
		remaining := "active"
		if char.BabysitExpiresAt != nil {
			days := int(time.Until(*char.BabysitExpiresAt).Hours() / 24)
			if days < 1 {
				remaining = "less than a day left"
			} else {
				remaining = fmt.Sprintf("%d days left", days)
			}
		}
		sb.WriteString(fmt.Sprintf("\n🍼 Babysitting: %s (focus: %s)\n", remaining, char.BabysitSkillFocus))
	}

	// Rival status
	if char.RivalPool == 1 {
		records, _ := loadAllRivalRecords(char.UserID)
		sb.WriteString("\n⚔️ Rivals: Unlocked")
		if len(records) > 0 {
			totalW, totalL := 0, 0
			for _, r := range records {
				totalW += r.Wins
				totalL += r.Losses
			}
			sb.WriteString(fmt.Sprintf(" (%dW / %dL)", totalW, totalL))
		}
		sb.WriteString(" — `!adventure rivals` for details\n")
	}

	// Today's action
	if char.ActionTakenToday {
		sb.WriteString("\n📅 Today: Action taken")
	} else {
		sb.WriteString("\n📅 Today: No action yet — reply to morning DM or type `!adventure`")
	}

	return sb.String()
}

// ── Morning DM ───────────────────────────────────────────────────────────────

func renderAdvMorningDM(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, balance float64, bonuses *AdvBonusSummary, holidayName string) string {
	var sb strings.Builder

	// Holiday notice (before greeting)
	if holidayName != "" {
		sb.WriteString(fmt.Sprintf("🎉 Happy %s! In recognition of %s, you are able to take **two actions** today.\n\n", holidayName, holidayName))
	}

	// Pick a morning greeting
	greeting, _ := advPickFlavor(MorningDM, char.UserID, "morning_dm")
	vars := map[string]string{
		"{name}": char.DisplayName,
		"{character_sheet}": fmt.Sprintf(
			"  ⚔️ Combat Lv.%d  ⛏️ Mining Lv.%d  🌿 Foraging Lv.%d  🎣 Fishing Lv.%d\n  💰 €%.0f",
			char.CombatLevel, char.MiningSkill, char.ForagingSkill, char.FishingSkill, balance),
	}
	sb.WriteString(advSubstituteFlavor(greeting, vars))
	sb.WriteString("\n\n")

	// Active buffs
	buffs, _ := loadAdvActiveBuffs(char.UserID)
	if len(buffs) > 0 {
		sb.WriteString("✨ **Active buffs:**\n")
		for _, b := range buffs {
			remaining := time.Until(b.ExpiresAt).Truncate(time.Hour)
			sb.WriteString(fmt.Sprintf("  %s (%.0fh remaining)\n", b.BuffName, remaining.Hours()))
		}
		sb.WriteString("\n")
	}

	// Streak info
	if char.CurrentStreak >= 3 {
		sb.WriteString(fmt.Sprintf("🔥 **Streak: %d days** — ", char.CurrentStreak))
		switch {
		case char.CurrentStreak >= 30:
			sb.WriteString("+20% XP, +15% loot, -5% death")
		case char.CurrentStreak >= 14:
			sb.WriteString("+15% XP, +10% loot, -3% death")
		case char.CurrentStreak >= 7:
			sb.WriteString("+10% XP, +5% loot")
		case char.CurrentStreak >= 3:
			sb.WriteString("+5% XP")
		}
		sb.WriteString("\n\n")
	}

	// Location choices
	sb.WriteString("**1️⃣ Dungeon:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityDungeon, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**2️⃣ Mine:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityMining, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**3️⃣ Forage:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityForaging, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**4️⃣ Fish:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityFishing, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**5️⃣ Shop** — buy/sell gear and loot\n")
	sb.WriteString("**6️⃣ Rest** — skip today, bank your luck\n\n")

	sb.WriteString("Reply with the number and location, e.g: `1 Soggy Cellar`\n")
	sb.WriteString("You have until midnight UTC to choose.")

	return sb.String()
}

// ── Holiday Second Action Prompt ──────────────────────────────────────────────

func renderAdvHolidaySecondPrompt(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, bonuses *AdvBonusSummary) string {
	var sb strings.Builder

	sb.WriteString("✅ Action 1 complete.\n\nNow choose your **second action**:\n\n")

	sb.WriteString("**1️⃣ Dungeon:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityDungeon, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**2️⃣ Mine:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityMining, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**3️⃣ Forage:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityForaging, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**4️⃣ Fish:**\n")
	for _, el := range advEligibleLocations(char, equip, AdvActivityFishing, bonuses) {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death%s)\n", el.Location.Name, el.Location.Tier, el.DeathPct, warn))
	}

	sb.WriteString("**5️⃣ Shop** — buy/sell gear and loot\n")
	sb.WriteString("**6️⃣ Rest** — skip the second action\n\n")
	sb.WriteString("Reply with the number and location, e.g: `1 Soggy Cellar`")

	return sb.String()
}

// ── Resolution DM ────────────────────────────────────────────────────────────

func renderAdvResolutionDM(result *AdvActionResult, char *AdventureCharacter) string {
	var sb strings.Builder

	sb.WriteString(result.FlavorText)
	sb.WriteString("\n\n")

	// Summary line
	sb.WriteString("───────────────────\n")

	if result.Outcome == AdvOutcomeDeath {
		sb.WriteString("💀 **You died.**\n")
		if char.DeadUntil != nil {
			sb.WriteString(fmt.Sprintf("Expected return: %s UTC\n", char.DeadUntil.Format("2006-01-02 15:04")))
		}
	}

	if result.NearDeath && result.Outcome != AdvOutcomeDeath {
		sb.WriteString("😰 **Near death!** You survived by the thinnest margin. +15% XP bonus.\n")
	}

	if len(result.LootItems) > 0 {
		sb.WriteString(fmt.Sprintf("💰 Loot: €%d total\n", result.TotalLootValue))
		for _, item := range result.LootItems {
			sb.WriteString(fmt.Sprintf("  • %s — €%d\n", item.Name, item.Value))
		}
	}

	sb.WriteString(fmt.Sprintf("✨ +%d %s XP", result.XPGained, result.XPSkill))
	if result.LeveledUp {
		sb.WriteString(fmt.Sprintf(" — **LEVEL UP! %s Lv.%d!** 🎉", titleCase(result.XPSkill), result.NewLevel))
	}
	sb.WriteString("\n")

	// Equipment damage
	if len(result.EquipDamage) > 0 {
		sb.WriteString("🔧 Equipment damage:\n")
		for slot, dmg := range result.EquipDamage {
			if dmg > 0 {
				sb.WriteString(fmt.Sprintf("  • %s: -%d condition\n", slotTitle(slot), dmg))
			}
		}
	}

	// Equipment broken
	if len(result.EquipBroken) > 0 {
		for _, slot := range result.EquipBroken {
			breakPool, ok := EquipmentBreaking[string(slot)]
			if ok && len(breakPool) > 0 {
				text := breakPool[rand.IntN(len(breakPool))]
				replacement := tier0Equipment(slot)
				text = advSubstituteFlavor(text, map[string]string{
					"{item}":        string(slot),
					"{replacement}": replacement,
				})
				sb.WriteString("\n" + text + "\n")
			}
		}
	}

	// Streak
	if result.StreakBonus > 0 {
		sb.WriteString(fmt.Sprintf("\n🔥 Streak: %d days\n", result.StreakBonus))
	}

	return sb.String()
}

// advClosingBlock selects and formats a closing block based on outcome.
func advClosingBlock(outcome AdvOutcomeType, userID id.UserID, location string, morningHour, summaryHour int) string {
	var pool []string
	var category string

	switch outcome {
	case AdvOutcomeExceptional:
		pool = ClosingExceptional
		category = "closing_exceptional"
	case AdvOutcomeSuccess:
		pool = ClosingSuccess
		category = "closing_success"
	case AdvOutcomeDeath:
		pool = ClosingDeath
		category = "closing_death"
	default:
		// Empty, cave-in, hornets, bear, river — all failure closings
		pool = ClosingFailure
		category = "closing_failure"
	}

	if len(pool) == 0 {
		return ""
	}

	text, _ := advPickFlavor(pool, userID, category)

	// Compute reset time (next midnight UTC)
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	remaining := midnight.Sub(now)
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60

	timeUntil := fmt.Sprintf("%dh %dm", hours, minutes)

	return advSubstituteFlavor(text, map[string]string{
		"{location}":     location,
		"{reset_time}":   "00:00 UTC",
		"{time_until}":   timeUntil,
		"{morning_time}": fmt.Sprintf("%02d:00", morningHour),
		"{summary_time}": fmt.Sprintf("%02d:00", summaryHour),
	})
}

// ── Death Status DM ──────────────────────────────────────────────────────────

func renderAdvDeathStatusDM(char *AdventureCharacter) string {
	text, _ := advPickFlavor(DeathDM, char.UserID, "death_dm")
	remaining := ""
	if char.DeadUntil != nil {
		remaining = char.DeadUntil.Format("15:04")
	}
	location := char.GrudgeLocation
	if location == "" {
		location = "an unknown location"
	}
	return advSubstituteFlavor(text, map[string]string{
		"{name}":     char.DisplayName,
		"{time}":     remaining,
		"{location}": location,
	})
}

// ── Respawn DM ───────────────────────────────────────────────────────────────

func renderAdvRespawnDM(char *AdventureCharacter) string {
	text, _ := advPickFlavor(RespawnDM, char.UserID, "respawn_dm")
	return advSubstituteFlavor(text, map[string]string{
		"{name}": char.DisplayName,
	})
}

// ── Idle Shame DM ────────────────────────────────────────────────────────────

func renderAdvIdleShameDM(char *AdventureCharacter) string {
	text, _ := advPickFlavor(IdleShameDM, char.UserID, "idle_shame")
	return advSubstituteFlavor(text, map[string]string{
		"{name}": char.DisplayName,
	})
}

// ── Onboarding DM ────────────────────────────────────────────────────────────

func renderAdvOnboardingDM(char *AdventureCharacter) string {
	text, _ := advPickFlavor(OnboardingDM, char.UserID, "onboarding")
	return advSubstituteFlavor(text, map[string]string{
		"{name}": char.DisplayName,
	})
}

// ── Daily Summary ────────────────────────────────────────────────────────────

type AdvPlayerDaySummary struct {
	DisplayName    string
	CombatLevel    int
	MiningSkill    int
	ForagingSkill  int
	FishingSkill   int
	Activity       string
	Location       string
	Outcome        string
	LootValue      int64
	IsDead         bool
	DeadUntil      string
	IsResting      bool
	SummaryLine    string
	HolidayActions int // 0 = not holiday or no action; 1 = took one; 2 = took both
}

func renderAdvDailySummary(date string, tb *TwinBeeResult, tbRewards TwinBeeRewardSummary, players []AdvPlayerDaySummary, holidayName string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📜 **ADVENTURER DAILY REPORT**\n%s\n\n", date))

	if holidayName != "" {
		sb.WriteString(fmt.Sprintf("🎉 In recognition of **%s**, adventurers had two actions today.\n\n", holidayName))
	}

	// TwinBee section
	if tb != nil {
		sb.WriteString("🐝 **TwinBee's Daily Report**\n")
		sb.WriteString(fmt.Sprintf("Went to: %s (Tier %d)\n", tb.Location.Name, tb.Location.Tier))

		// One-liner for TwinBee
		var tbSummaryPool []string
		switch tb.Outcome {
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			tbSummaryPool = TwinBeeSummarySuccess
		case AdvOutcomeEmpty, AdvOutcomeHornets, AdvOutcomeCaveIn, AdvOutcomeBear, AdvOutcomeRiver:
			tbSummaryPool = TwinBeeSummaryWithdrawal
		default:
			tbSummaryPool = TwinBeeSummaryEmpty
		}
		if len(tbSummaryPool) > 0 {
			line := tbSummaryPool[rand.IntN(len(tbSummaryPool))]
			line = advSubstituteFlavor(line, map[string]string{
				"{location}": tb.Location.Name,
				"{loot}":     tb.LootDesc,
				"{value}":    fmt.Sprintf("%d", tb.LootValue),
			})
			sb.WriteString(fmt.Sprintf("Outcome: %s\n", line))
		}

		if tb.LootValue > 0 {
			sb.WriteString(fmt.Sprintf("Haul: €%d in %s\n", tb.LootValue, tb.LootDesc))
		} else {
			sb.WriteString("Haul: Strategic withdrawal. No haul.\n")
		}

		sb.WriteString("\n")
		if tbRewards.Eligible > 0 {
			hasRewards := tbRewards.GoldShare > 0 || tbRewards.GiftCount > 0
			if hasRewards {
				sb.WriteString(fmt.Sprintf("Rewards distributed to %d participating adventurers (scaled by level):\n", tbRewards.Eligible))
				if tbRewards.GoldShare > 0 {
					sb.WriteString(fmt.Sprintf("  💰 ~€%d avg\n", tbRewards.GoldShare))
				}
				if tbRewards.GiftCount > 0 {
					sb.WriteString(fmt.Sprintf("  ⭐ %d players received a gift item\n", tbRewards.GiftCount))
				}
			} else {
				sb.WriteString(fmt.Sprintf("Rewards distributed to %d participating adventurers: jackshit.\n", tbRewards.Eligible))
			}
		}
		sb.WriteString("\n(Players who rested today received nothing. Fallen adventurers still earn their share. TwinBee noticed.)\n\n")
		sb.WriteString("───────────────────\n\n")
	}

	// Player summaries
	var dead []AdvPlayerDaySummary
	var resting []AdvPlayerDaySummary
	var bestPlayer *AdvPlayerDaySummary
	var worstPlayer *AdvPlayerDaySummary

	for i := range players {
		p := &players[i]
		if p.IsDead {
			dead = append(dead, *p)
			// Dead players who acted today still show in the main section
			if p.Location != "" {
				sb.WriteString(fmt.Sprintf("💀 **%s** — Combat Lv.%d | Mining Lv.%d | Forage Lv.%d | Fishing Lv.%d\n",
					p.DisplayName, p.CombatLevel, p.MiningSkill, p.ForagingSkill, p.FishingSkill))
				sb.WriteString(fmt.Sprintf("   Went to: %s\n", p.Location))
				sb.WriteString(fmt.Sprintf("   Outcome: %s\n\n", p.SummaryLine))
				if worstPlayer == nil {
					worstPlayer = p
				}
			}
			continue
		}
		if p.IsResting {
			resting = append(resting, *p)
			continue
		}

		sb.WriteString(fmt.Sprintf("⚔️ **%s** — Combat Lv.%d | Mining Lv.%d | Forage Lv.%d | Fishing Lv.%d\n",
			p.DisplayName, p.CombatLevel, p.MiningSkill, p.ForagingSkill, p.FishingSkill))
		sb.WriteString(fmt.Sprintf("   Went to: %s\n", p.Location))
		sb.WriteString(fmt.Sprintf("   Outcome: %s\n\n", p.SummaryLine))

		if bestPlayer == nil || p.LootValue > bestPlayer.LootValue {
			bestPlayer = p
		}
	}

	// Dead list (players who didn't act today but are still dead from before)
	var deadNoAction []AdvPlayerDaySummary
	for _, d := range dead {
		if d.Location == "" {
			deadNoAction = append(deadNoAction, d)
		}
	}
	if len(deadNoAction) > 0 {
		sb.WriteString("💀 **Currently Dead:**\n")
		for _, d := range deadNoAction {
			sb.WriteString(fmt.Sprintf("   %s — returns %s\n", d.DisplayName, d.DeadUntil))
		}
		sb.WriteString("\n")
	}

	// Resting list
	if len(resting) > 0 {
		sb.WriteString("😴 **Resting/Idle:**\n")
		for _, r := range resting {
			sb.WriteString(fmt.Sprintf("   %s\n", r.DisplayName))
		}
		sb.WriteString("\n")
	}

	// Holiday stats
	if holidayName != "" {
		tookBoth := 0
		totalActive := 0
		for _, p := range players {
			if p.IsDead || p.IsResting {
				if p.HolidayActions > 0 {
					totalActive++
				}
				if p.HolidayActions >= 2 {
					tookBoth++
				}
				continue
			}
			if p.Activity != "" {
				totalActive++
			}
			if p.HolidayActions >= 2 {
				tookBoth++
			}
		}
		if totalActive > 0 {
			sb.WriteString(fmt.Sprintf("🎉 %s double-action day — %d of %d adventurers took both actions.\n\n", holidayName, tookBoth, totalActive))
		}

		// Note players who died before their second action
		for _, d := range dead {
			if d.HolidayActions == 1 {
				sb.WriteString(fmt.Sprintf("• %s — died in %s before their second action. Rough holiday.\n", d.DisplayName, d.Location))
			}
		}
		if len(dead) > 0 {
			// Check if any had HolidayActions == 1
			for _, d := range dead {
				if d.HolidayActions == 1 {
					sb.WriteString("\n")
					break
				}
			}
		}
	}

	// Standout
	if bestPlayer != nil && bestPlayer.LootValue > 0 {
		pool := SummaryStandoutGood
		if len(pool) > 0 {
			line := pool[rand.IntN(len(pool))]
			line = advSubstituteFlavor(line, map[string]string{
				"{name}":     bestPlayer.DisplayName,
				"{item}":     "",
				"{value}":    fmt.Sprintf("%d", bestPlayer.LootValue),
				"{location}": bestPlayer.Location,
			})
			sb.WriteString(fmt.Sprintf("🏆 **Today's standout:** %s\n", line))
		}
	} else if worstPlayer != nil {
		pool := SummaryStandoutDeath
		if len(pool) > 0 {
			line := pool[rand.IntN(len(pool))]
			line = advSubstituteFlavor(line, map[string]string{
				"{name}":     worstPlayer.DisplayName,
				"{location}": worstPlayer.Location,
			})
			sb.WriteString(fmt.Sprintf("🏆 **Today's standout:** %s\n", line))
		}
	}

	return sb.String()
}

// ── Leaderboard ──────────────────────────────────────────────────────────────

func renderAdvLeaderboard(chars []AdventureCharacter) string {
	if len(chars) == 0 {
		return "No adventurers registered yet. Type `!adventure` to begin."
	}

	// Sort by score
	type entry struct {
		Name  string
		Score int
		Levels string
		Streak int
	}
	var entries []entry
	for _, c := range chars {
		score := (c.CombatLevel + c.MiningSkill + c.ForagingSkill) * 10
		entries = append(entries, entry{
			Name:   c.DisplayName,
			Score:  score,
			Levels: fmt.Sprintf("⚔️%d ⛏️%d 🌿%d", c.CombatLevel, c.MiningSkill, c.ForagingSkill),
			Streak: c.CurrentStreak,
		})
	}

	// Simple sort (small list)
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Score > entries[i].Score {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("🏆 **Adventure Leaderboard**\n\n")

	limit := 10
	if len(entries) < limit {
		limit = len(entries)
	}

	medals := []string{"🥇", "🥈", "🥉"}
	for i := 0; i < limit; i++ {
		e := entries[i]
		prefix := fmt.Sprintf("%2d.", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		streak := ""
		if e.Streak >= 7 {
			streak = fmt.Sprintf(" 🔥%d", e.Streak)
		}
		sb.WriteString(fmt.Sprintf("%s **%s** — %s (score: %d%s)\n", prefix, e.Name, e.Levels, e.Score, streak))
	}

	return sb.String()
}

// ── Treasure Discard Prompt ──────────────────────────────────────────────────

func renderAdvTreasureDiscardPrompt(newTreasure *AdvTreasureDef, existing []AdvTreasureDef) string {
	if len(TreasureInventoryCap) == 0 {
		return "You found a treasure but your inventory is full. Reply 1, 2, or 3 to discard, or `keep`."
	}

	// Build substitution map with existing treasure info
	subs := map[string]string{
		"{treasure_name}": newTreasure.Name,
		"{location}":      "",
	}
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("%d", i+1)
		if i < len(existing) {
			subs["{treasure_"+key+"}"] = existing[i].Name
			subs["{bonus_"+key+"}"] = existing[i].InventoryDesc
		} else {
			subs["{treasure_"+key+"}"] = "(empty)"
			subs["{bonus_"+key+"}"] = ""
		}
	}

	text := TreasureInventoryCap[rand.IntN(len(TreasureInventoryCap))]
	return advSubstituteFlavor(text, subs)
}

// ── Summary One-Liners ───────────────────────────────────────────────────────

func advSummaryOneLiner(userID id.UserID, activity AdvActivityType, outcome AdvOutcomeType, lootValue int64, location string) string {
	var pool []string

	switch activity {
	case AdvActivityDungeon:
		switch outcome {
		case AdvOutcomeDeath:
			pool = SummaryDungeonDeath
		case AdvOutcomeEmpty:
			pool = SummaryDungeonEmpty
		case AdvOutcomeSuccess:
			pool = SummaryDungeonSuccess
		case AdvOutcomeExceptional:
			pool = SummaryDungeonExceptional
		}
	case AdvActivityMining:
		switch outcome {
		case AdvOutcomeDeath:
			pool = SummaryMiningDeath
		case AdvOutcomeEmpty, AdvOutcomeCaveIn:
			pool = SummaryMiningEmpty
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			pool = SummaryMiningSuccess
		}
	case AdvActivityForaging:
		switch outcome {
		case AdvOutcomeDeath:
			pool = SummaryForagingDeath
		case AdvOutcomeHornets:
			pool = SummaryForagingHornets
		case AdvOutcomeBear:
			pool = SummaryForagingBear
		case AdvOutcomeRiver:
			pool = SummaryForagingRiver
		case AdvOutcomeEmpty:
			pool = SummaryForagingEmpty
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			pool = SummaryForagingSuccess
		}
	case AdvActivityFishing:
		switch outcome {
		case AdvOutcomeDeath:
			pool = SummaryFishingDeath
		case AdvOutcomeEmpty:
			pool = SummaryFishingEmpty
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			pool = SummaryFishingSuccess
		}
	}

	if len(pool) == 0 {
		return fmt.Sprintf("Went to %s. Things happened.", location)
	}

	text, _ := advPickFlavor(pool, userID, fmt.Sprintf("summary_%s_%s", activity, outcome))
	return advSubstituteFlavor(text, map[string]string{
		"{name}":     "",
		"{item}":     "",
		"{value}":    fmt.Sprintf("%d", lootValue),
		"{location}": location,
		"{hours}":    "24",
		"{ore}":      "",
		"{tool}":     "",
		"{xp}":       "",
	})
}
