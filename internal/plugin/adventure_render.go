package plugin

import (
	"fmt"
	"hash/fnv"
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

	// Crafting (only show once they've actually crafted something)
	if char.CraftsSucceeded > 0 {
		sb.WriteString(fmt.Sprintf("🧪 Crafts: %d successful (Foraging Lv.%d — `!adventure recipes`)\n",
			char.CraftsSucceeded, char.ForagingSkill))
	}

	// Equipment
	sb.WriteString("\n🛡️ Equipment:\n")
	eqScore := advEquipmentScore(equip)
	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil {
			continue
		}
		marker := ""
		if eq.Masterwork {
			marker = " ⭐"
		} else if eq.ArenaTier > 0 {
			marker = " ⚔️"
		}
		// Mastery segment is appended as a third inline field after
		// condition only when there's progress — keeps rows tight for
		// freshly-equipped gear and grows with use.
		mastery := ""
		if seg := advMasteryRowSegment(eq.ActionsUsed); seg != "" {
			mastery = " | " + seg
		}
		sb.WriteString(fmt.Sprintf("  %s %s: %s%s (Tier %d | %d%% cond%s)\n",
			slotEmoji(slot), slotTitle(slot), eq.Name, marker, eq.Tier, eq.Condition, mastery))
	}
	sb.WriteString(fmt.Sprintf("  Equipment Score: %.1f\n", eqScore))
	if line := renderAdvMasteryAggregate(equip); line != "" {
		sb.WriteString("  " + line + "\n")
	}

	// Treasures
	if len(treasures) > 0 {
		seen := make(map[string]bool)
		sb.WriteString("\n💎 Treasures:\n")
		for _, t := range treasures {
			if seen[t.TreasureKey] {
				continue
			}
			seen[t.TreasureKey] = true
			if def := lookupAdvTreasureDef(t.TreasureKey); def != nil {
				sb.WriteString(fmt.Sprintf("  %s\n", def.InventoryDesc))
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

	// Today's actions
	isHolSheet, _ := isHolidayToday()
	combatMax := maxCombatActions
	harvestMax := maxHarvestActions
	if isHolSheet {
		combatMax++
		harvestMax++
	}
	combatRemaining := combatMax - char.CombatActionsUsed
	if combatRemaining < 0 {
		combatRemaining = 0
	}
	harvestRemaining := harvestMax - char.HarvestActionsUsed
	if harvestRemaining < 0 {
		harvestRemaining = 0
	}
	if char.HasActedToday() {
		sb.WriteString(fmt.Sprintf("\n📅 Today: %d combat, %d harvest actions remaining", combatRemaining, harvestRemaining))
	} else {
		sb.WriteString(fmt.Sprintf("\n📅 Today: %d combat, %d harvest actions available", combatRemaining, harvestRemaining))
	}

	return sb.String()
}

// ── Morning DM ───────────────────────────────────────────────────────────────

// renderCoopTeaser surfaces an open co-op run the player could join, so the
// system isn't a footnote in help text. Returns "" when there's nothing to
// surface (no open runs, or the player is already locked into one). When
// runs exist that the player is under-levelled for, we still hint at them
// as a stretch goal — joining as a liability is a real choice.
func renderCoopTeaser(char *AdventureCharacter) string {
	runs, err := loadOpenCoopRuns()
	if err != nil || len(runs) == 0 {
		return ""
	}

	pick, stretch := pickCoopTeaserCandidate(runs, char)
	tag := ""
	if pick == nil {
		pick = stretch
		tag = " *(below recommended level — would join as liability)*"
	}
	if pick == nil {
		return ""
	}

	leaderName := string(pick.LeaderID)
	if c, err := loadAdvCharacter(pick.LeaderID); err == nil && c != nil && c.DisplayName != "" {
		leaderName = c.DisplayName
	}
	def := coopTierTable[pick.Tier]
	return fmt.Sprintf("🛡️ **Open Co-op #%d** — Tier %d (%s) led by %s · `!coop join %d`%s",
		pick.ID, pick.Tier, def.difficulty, leaderName, pick.ID, tag)
}

// pickCoopTeaserCandidate scans the open coop runs and returns up to two
// picks: a primary (where the player meets the level gate) and a stretch
// (any other run, joined as a liability). Skips runs the player already
// leads. Pure function — testable without DB.
func pickCoopTeaserCandidate(runs []*CoopRun, char *AdventureCharacter) (match, stretch *CoopRun) {
	for _, r := range runs {
		if r.LeaderID == char.UserID {
			continue
		}
		def := coopTierTable[r.Tier]
		if char.CombatLevel >= def.minLevel && match == nil {
			match = r
		} else if stretch == nil {
			stretch = r
		}
	}
	return match, stretch
}

// renderCraftingTeaser surfaces the crafting system before and just after
// the Foraging-10 auto-unlock, so it doesn't stay invisible to players who
// never type !adventure recipes. Returns "" when the teaser shouldn't fire
// today (out of pre-unlock window, or already deep into post-unlock).
func renderCraftingTeaser(char *AdventureCharacter) string {
	const unlock = 10
	if char.ForagingSkill < unlock {
		// Pre-unlock teaser: only when within 3 levels.
		if char.ForagingSkill < unlock-3 {
			return ""
		}
		levelsToGo := unlock - char.ForagingSkill
		return fmt.Sprintf("🧪 **Crafting unlocks in %d Foraging level%s** — auto-craft consumables from gathered ingredients (Berry Poultice, Herb Salve, etc.).",
			levelsToGo, plural(levelsToGo))
	}

	// Post-unlock: nudge when no successful crafts yet (gentle), or once a
	// week (loose periodicity via day-of-year %).
	if char.CraftsSucceeded == 0 {
		return "🧪 **Crafting is unlocked.** Gather a couple of matching ingredients and TwinBee will auto-craft consumables — try `!adventure recipes` to see what your level supports."
	}
	// Per-player weekly reminder: hash UserID + ISO week to pick a stable
	// weekday for this player. Spreads the cohort across the week instead
	// of all firing on the same global day.
	if int(time.Now().UTC().Weekday()) == craftingReminderWeekday(char.UserID, time.Now().UTC()) {
		return "🧪 *Crafting reminder* — `!adventure recipes` shows what's available at Foraging Lv." + fmt.Sprintf("%d.", char.ForagingSkill)
	}
	return ""
}

// craftingReminderWeekday picks the day-of-week (0=Sunday..6=Saturday) on
// which a given player gets the weekly crafting nudge during the given
// week. Stable within an ISO week (predictable, not annoying), rotates
// across weeks (not always the same day long-term).
func craftingReminderWeekday(userID id.UserID, now time.Time) int {
	year, week := now.ISOWeek()
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%d|%d", string(userID), year, week)
	return int(h.Sum32() % 7)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// renderRivalNudge surfaces a pending rival challenge in the morning DM so
// players who missed or ignored the dramatic challenge DM see a reminder
// before it expires. Returns "" if there's no pending challenge against
// this player.
func renderRivalNudge(char *AdventureCharacter) string {
	c := pendingRivalChallengeForChallenged(char.UserID)
	if c == nil {
		return ""
	}
	hours := int(time.Until(c.ExpiresAt).Hours())
	if hours < 1 {
		hours = 1 // floor — "<1h" feels worse than "1h"
	}
	rivalName := string(c.ChallengerID)
	if rc, err := loadAdvCharacter(c.ChallengerID); err == nil && rc != nil && rc.DisplayName != "" {
		rivalName = rc.DisplayName
	}
	return fmt.Sprintf("⚔️ **Rival challenge open** — %s, round %d/3, €%d on the line · expires in %dh · reply **rock**, **paper**, or **scissors**",
		rivalName, c.Round, c.Stake, hours)
}

// writeAdvLocationLines renders the eligible-location bullets shown in the
// morning DM. Each line surfaces both the downside (death %) and upside
// (exceptional %) so the menu teaches risk/reward, not just risk.
func writeAdvLocationLines(sb *strings.Builder, locs []AdvEligibleLocation) {
	for _, el := range locs {
		warn := ""
		if el.InPenaltyZone {
			warn = " ⚠️"
		}
		sb.WriteString(fmt.Sprintf("  • %s (Tier %d, ~%.0f%% death · ~%.0f%% exceptional%s)\n",
			el.Location.Name, el.Location.Tier, el.DeathPct, el.ExceptionalPct, warn))
	}
}

func renderAdvMorningDM(char *AdventureCharacter, equip map[EquipmentSlot]*AdvEquipment, balance float64, bonuses *AdvBonusSummary, holidayName string) string {
	var sb strings.Builder

	// Holiday notice (before greeting)
	if holidayName != "" {
		sb.WriteString(fmt.Sprintf("🎉 Happy %s! In recognition of %s, you get an extra combat and harvest action today.\n\n", holidayName, holidayName))
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

	// Action budget
	isHol := holidayName != ""
	combatMax := maxCombatActions
	harvestMax := maxHarvestActions
	if isHol {
		combatMax++
		harvestMax++
	}
	combatLeft := combatMax - char.CombatActionsUsed
	if combatLeft < 0 {
		combatLeft = 0
	}
	harvestLeft := harvestMax - char.HarvestActionsUsed

	// Co-op participants have combat locked for the duration of the run.
	coopRun, _ := loadCoopRunForUser(char.UserID)
	inCoop := coopRun != nil && coopRun.Status == "active"

	if inCoop {
		sb.WriteString(fmt.Sprintf("📋 **Actions:** combat locked (in Co-op #%d, day %d/%d) · %d/%d harvest\n\n",
			coopRun.ID, coopRun.CurrentDay, coopRun.TotalDays, harvestLeft, harvestMax))
	} else {
		sb.WriteString(fmt.Sprintf("📋 **Actions:** %d/%d combat · %d/%d harvest\n\n", combatLeft, combatMax, harvestLeft, harvestMax))

		// Co-op teaser — show open runs the player could join.
		if line := renderCoopTeaser(char); line != "" {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	// Rival nudge — a pending challenge waits for action.
	if line := renderRivalNudge(char); line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Crafting teaser — surface the system when it's near unlock or post-unlock.
	if line := renderCraftingTeaser(char); line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Location choices
	if inCoop {
		sb.WriteString("**1️⃣ Dungeon:** _(off in the Co-op, no solo combat — `!coop status` for run state)_\n")
	} else if char.CanDoCombat(isHol) {
		sb.WriteString("**1️⃣ Dungeon:**\n")
		writeAdvLocationLines(&sb, advEligibleLocations(char, equip, AdvActivityDungeon, bonuses))
	} else {
		sb.WriteString("**1️⃣ Dungeon:** _(no combat actions remaining)_\n")
	}

	if char.CanDoHarvest(isHol) {
		sb.WriteString("**2️⃣ Mine:**\n")
		writeAdvLocationLines(&sb, advEligibleLocations(char, equip, AdvActivityMining, bonuses))

		sb.WriteString("**3️⃣ Forage:**\n")
		writeAdvLocationLines(&sb, advEligibleLocations(char, equip, AdvActivityForaging, bonuses))

		sb.WriteString("**4️⃣ Fish:**\n")
		writeAdvLocationLines(&sb, advEligibleLocations(char, equip, AdvActivityFishing, bonuses))
	} else {
		sb.WriteString("**2️⃣ Mine:** _(no harvest actions remaining)_\n")
		sb.WriteString("**3️⃣ Forage:** _(no harvest actions remaining)_\n")
		sb.WriteString("**4️⃣ Fish:** _(no harvest actions remaining)_\n")
	}

	sb.WriteString("**5️⃣ Shop** — buy/sell gear and loot\n")
	sb.WriteString("**6️⃣ Blacksmith** — repair damaged equipment\n")
	sb.WriteString("**7️⃣ Rest** — skip today, bank your luck\n")
	sb.WriteString("**8️⃣ Thom** — `!thom` visit Krooke Realty 🏠\n\n")

	sb.WriteString("Reply with the number and location, e.g: `1 Soggy Cellar`\n")
	sb.WriteString("You have until midnight UTC to choose.")

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
		sb.WriteString(fmt.Sprintf("💰 Loot: %s total\n", fmtEuro(result.TotalLootValue)))
		for _, item := range result.LootItems {
			sb.WriteString(fmt.Sprintf("  • %s — %s\n", item.Name, fmtEuro(item.Value)))
		}
	}

	sb.WriteString(fmt.Sprintf("✨ +%d %s XP", result.XPGained, result.XPSkill))
	if result.XPBreakdown != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", result.XPBreakdown))
	}
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
	cost := int64(char.CombatLevel) * 25_000
	remaining := "unknown"
	if char.DeadUntil != nil {
		remaining = char.DeadUntil.Format("15:04")
	}
	return fmt.Sprintf("💀 You're still dead.\n\n"+
		"🏥 Type `!hospital` for same-day revival (€%d after insurance)\n"+
		"⏳ Or wait — natural respawn at %s UTC",
		cost, remaining)
}

// ── Respawn DM ───────────────────────────────────────────────────────────────

func renderAdvRespawnDM(char *AdventureCharacter) string {
	text, _ := advPickFlavor(RespawnDM, char.UserID, "respawn_dm")
	return advSubstituteFlavor(text, map[string]string{
		"{name}": char.DisplayName,
	})
}

// ── Treasure List ───────────────────────────────────────────────────────────

// lookupAdvTreasureDef returns the canonical definition for a treasure key,
// or nil if unknown. Walks the tiered table; cheap because the total count
// is small.
func lookupAdvTreasureDef(key string) *AdvTreasureDef {
	for _, defs := range advAllTreasures {
		for i := range defs {
			if defs[i].Key == key {
				d := defs[i]
				return &d
			}
		}
	}
	return nil
}

// renderAdvTreasureList renders the standalone treasure listing for
// `!adventure treasures`. Shows each treasure, marks irreplaceable ones,
// and surfaces the lock state so players know whether new drops will
// auto-swap or be refused.
func renderAdvTreasureList(treasures []AdvTreasureDef, locked bool) string {
	var sb strings.Builder
	sb.WriteString("💎 **Your Treasures**")
	if locked {
		sb.WriteString(" 🔒 _(locked — drops at cap refused)_")
	}
	sb.WriteString("\n\n")

	if len(treasures) == 0 {
		sb.WriteString("_No treasures yet. Higher-tier locations have higher drop rates._")
		return sb.String()
	}

	for _, t := range treasures {
		t := t
		marker := ""
		if advTreasureIrreplaceable(&t) {
			marker = " 🛡️"
		}
		sb.WriteString(fmt.Sprintf("  • Tier %d · %s%s\n    _%s_\n", t.Tier, t.Name, marker, t.InventoryDesc))
	}

	if locked {
		sb.WriteString("\n_Unlock with `!adventure treasures unlock` to allow higher-tier auto-swaps._")
	} else {
		sb.WriteString("\n_Higher-tier drops auto-swap your lowest replaceable treasure (10-min undo). 🛡️ = irreplaceable, never auto-discarded._\n_Lock with `!adventure treasures lock` to refuse drops at cap._")
	}
	return sb.String()
}

// renderAdvMasteryAggregate produces the one-line summary shown under the
// equipment block on the character sheet. Three states:
//   - Bonus active: shows percent, threshold count, cap-reached when at +10%.
//   - No bonus but some progress: hints that a threshold is approaching.
//   - No progress: empty (don't bloat sheets for new players).
func renderAdvMasteryAggregate(equip map[EquipmentSlot]*AdvEquipment) string {
	r := advMasteryRollup(equip)
	if !r.AnyProgress {
		return ""
	}
	if r.Bonus > 0 {
		base := fmt.Sprintf("🎯 Mastery: +%.0f%% loot quality · %d threshold%s crossed",
			r.Bonus, r.ThresholdsCrossed, plural(r.ThresholdsCrossed))
		if r.MaxedSlots > 0 {
			base += fmt.Sprintf(" · %d slot%s maxed", r.MaxedSlots, plural(r.MaxedSlots))
		}
		// Cap is +10%; the raw count keeps rising past it. Flag when the
		// player is paying threshold tax for nothing.
		if r.Bonus >= 10 {
			base += " (cap reached)"
		}
		base += " · `!adventure mastery`"
		return base
	}
	// Some progress, no thresholds crossed yet — surface the next gate.
	return "🎯 Mastery: building up — first threshold at 50 actions/slot · `!adventure mastery`"
}

// ── Equipment Mastery View ──────────────────────────────────────────────────

// renderAdvMasteryView renders a per-slot mastery readout: progress bar,
// tier marker, and next threshold so players can see "where am I" between
// the discrete celebration DMs that fire at threshold crossings.
func renderAdvMasteryView(equip map[EquipmentSlot]*AdvEquipment) string {
	var sb strings.Builder
	sb.WriteString("🎯 **Equipment Mastery**\n\n")

	any := false
	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil {
			sb.WriteString(fmt.Sprintf("  %s %s — _empty_\n", slotEmoji(slot), slotTitle(slot)))
			continue
		}
		any = true
		tier, next := advMasteryTier(eq.ActionsUsed)
		marker := advMasteryMarker(eq.ActionsUsed)
		if marker == "" {
			marker = "·"
		}

		// Bar against the next threshold (or against 250 when at max so the
		// bar visually maxes out rather than disappearing).
		var barTotal int
		if next > 0 {
			barTotal = next
		} else {
			barTotal = advMasteryThresholds[len(advMasteryThresholds)-1]
		}
		bar := masteryBar(eq.ActionsUsed, barTotal)
		_ = tier

		if next > 0 {
			sb.WriteString(fmt.Sprintf("  %s %s · %s · %d/%d %s %s\n",
				slotEmoji(slot), slotTitle(slot), eq.Name, eq.ActionsUsed, next, bar, marker))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %s · %s · %d (max) %s %s\n",
				slotEmoji(slot), slotTitle(slot), eq.Name, eq.ActionsUsed, bar, marker))
		}
	}

	bonus := advEquipmentMasteryBonus(equip)
	sb.WriteString("\n")
	if !any {
		sb.WriteString("_No equipment yet — gear up first._")
		return sb.String()
	}
	if bonus > 0 {
		sb.WriteString(fmt.Sprintf("**Active mastery bonus: +%.0f%% loot quality** (cap +10%%)\n", bonus))
	} else {
		sb.WriteString("_No mastery bonus yet — first threshold at 50 actions per slot._\n")
	}
	sb.WriteString("Thresholds: 50 ✦ · 100 ✦✦ · 250 ✦✦✦. Each crossed threshold per slot adds +1% loot quality.")
	return sb.String()
}

// masteryBar builds a 10-cell progress bar for the mastery view.
func masteryBar(value, total int) string {
	if total <= 0 {
		return "▱▱▱▱▱▱▱▱▱▱"
	}
	filled := value * 10 / total
	if filled > 10 {
		filled = 10
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", 10-filled)
}

// ── Auto-Babysit DM ─────────────────────────────────────────────────────────

// renderAutoBabysitDM builds the morning notification when auto-babysit
// covered an idle day. Surfaces what the babysitter actually accomplished
// (skill focus, gold/XP, items) plus a flavor highlight on lucky days, so
// the system feels like an active companion instead of a silent debit.
func renderAutoBabysitDM(daily int, streak int, res AutoBabysitDayResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🍼 **Auto-babysit activated** — €%d deducted. Your streak is safe at %d days.\n", daily, streak))
	if res.Skill != "" {
		sb.WriteString(fmt.Sprintf("Focus: %s · €%d earned · %d XP", res.Skill, res.Gold, res.XP))
		if len(res.Items) > 0 {
			sb.WriteString(fmt.Sprintf(" · items: %s", strings.Join(res.Items, ", ")))
		}
		sb.WriteString("\n")
	}
	if res.Highlight && res.Skill != "" {
		line := pickBabysitFlavor(babysitHighlightLines)
		if line != "" {
			sb.WriteString("\n_" + fmt.Sprintf(line, res.Skill) + "_")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
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
	DeathSource    string
	DeathLocation  string
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
			// Dead players who acted today still show in the main section.
			// If they died OUTSIDE the adventure (e.g. Arena), the adventure
			// itself was successful — show the alive icon for that block and
			// append a "later fell" note. Empty DeathSource is legacy and
			// treated as adventure-death (current behavior).
			if p.Location != "" {
				diedOnAdventure := p.DeathSource == "" || p.DeathSource == "adventure"
				icon := "💀"
				if !diedOnAdventure {
					icon = "⚔️"
				}
				sb.WriteString(fmt.Sprintf("%s **%s** — Combat Lv.%d | Mining Lv.%d | Forage Lv.%d | Fishing Lv.%d\n",
					icon, p.DisplayName, p.CombatLevel, p.MiningSkill, p.ForagingSkill, p.FishingSkill))
				sb.WriteString(fmt.Sprintf("   Went to: %s\n", p.Location))
				sb.WriteString(fmt.Sprintf("   Outcome: %s\n", p.SummaryLine))
				if !diedOnAdventure {
					where := p.DeathLocation
					if where == "" {
						where = "elsewhere"
					}
					sb.WriteString(fmt.Sprintf("   💀 Later fell in %s.\n", where))
				}
				sb.WriteString("\n")
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
		if p.Location != "" {
			sb.WriteString(fmt.Sprintf("   Went to: %s\n", p.Location))
			sb.WriteString(fmt.Sprintf("   Outcome: %s\n\n", p.SummaryLine))
		} else {
			sb.WriteString("   Acted today — no log recorded.\n\n")
		}

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
			lossLoc := worstPlayer.DeathLocation
			if lossLoc == "" {
				lossLoc = worstPlayer.Location
			}
			line = advSubstituteFlavor(line, map[string]string{
				"{name}":     worstPlayer.DisplayName,
				"{location}": lossLoc,
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
		score := (c.CombatLevel + c.MiningSkill + c.ForagingSkill + c.FishingSkill) * 10
		entries = append(entries, entry{
			Name:   c.DisplayName,
			Score:  score,
			Levels: fmt.Sprintf("⚔️%d ⛏️%d 🌿%d 🎣%d", c.CombatLevel, c.MiningSkill, c.ForagingSkill, c.FishingSkill),
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
