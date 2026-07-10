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

func renderAdvCharacterSheet(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment, items []AdvItem, treasures []AdvTreasureBonus, balance float64) string {
	var sb strings.Builder

	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return "No adventurer found. Type `!adventure` to begin."
	}

	displayName, _ := loadDisplayName(userID)
	sb.WriteString(fmt.Sprintf("⚔️ **%s's Adventurer**\n\n", displayName))

	// Stats — Combat line shows D&D Level (CombatXP display retired in L4f).
	dndLevel := dndLevelForUser(userID)
	sb.WriteString("📊 Stats:\n")
	sb.WriteString(fmt.Sprintf("  Combat:  Lv.%d\n", dndLevel))
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

	// Rival status — read from player_meta (Adv 2.0 Phase L4b).
	rivalPoolFlag, _, _ := loadRivalState(userID)
	if rivalPoolFlag == 1 {
		records, _ := loadAllRivalRecords(userID)
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

	return sb.String()
}

// ── Morning DM ───────────────────────────────────────────────────────────────

// renderCraftingTeaser surfaces the crafting system before and just after
// the Foraging-10 auto-unlock, so it doesn't stay invisible to players who
// never type !adventure recipes. Returns "" when the teaser shouldn't fire
// today (out of pre-unlock window, or already deep into post-unlock).
func renderCraftingTeaser(userID id.UserID) string {
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return ""
	}
	return craftingTeaserText(char.ForagingSkill, char.CraftsSucceeded, userID)
}

// craftingTeaserText is the pure-logic core of renderCraftingTeaser, split
// out so unit tests can exercise the bracket boundaries without a DB.
func craftingTeaserText(foragingSkill, craftsSucceeded int, userID id.UserID) string {
	const unlock = 10
	if foragingSkill < unlock {
		// Pre-unlock teaser: only when within 3 levels.
		if foragingSkill < unlock-3 {
			return ""
		}
		levelsToGo := unlock - foragingSkill
		return fmt.Sprintf("🧪 **Crafting unlocks in %d Foraging level%s** — auto-craft consumables from gathered ingredients (Berry Poultice, Herb Salve, etc.).",
			levelsToGo, plural(levelsToGo))
	}

	// Post-unlock: nudge when no successful crafts yet (gentle), or once a
	// week (loose periodicity via day-of-year %).
	if craftsSucceeded == 0 {
		return "🧪 **Crafting is unlocked.** Gather a couple of matching ingredients and I will auto-craft consumables — try `!adventure recipes` to see what your level supports."
	}
	if int(time.Now().UTC().Weekday()) == craftingReminderWeekday(userID, time.Now().UTC()) {
		return "🧪 *Crafting reminder* — `!adventure recipes` shows what's available at Foraging Lv." + fmt.Sprintf("%d.", foragingSkill)
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
func renderRivalNudge(userID id.UserID) string {
	c := pendingRivalChallengeForChallenged(userID)
	if c == nil {
		return ""
	}
	hours := int(time.Until(c.ExpiresAt).Hours())
	if hours < 1 {
		hours = 1 // floor — "<1h" feels worse than "1h"
	}
	rivalName := string(c.ChallengerID)
	if name, err := loadDisplayName(c.ChallengerID); err == nil && name != "" {
		rivalName = name
	}
	return fmt.Sprintf("⚔️ **Rival challenge open** — %s, round %d/3, €%d on the line · expires in %dh · reply **rock**, **paper**, or **scissors**",
		rivalName, c.Round, c.Stake, hours)
}

func renderAdvMorningDM(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment, balance float64, bonuses *AdvBonusSummary, holidayName string) string {
	var sb strings.Builder

	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return ""
	}

	// Holiday notice (before greeting). Today's perks: TwinBee starts new
	// runs in a slightly better mood (+5), expedition outfitting includes a
	// complimentary standard pack, and every harvest yields one extra unit.
	if holidayName != "" {
		sb.WriteString(fmt.Sprintf("🎉 Happy %s! In recognition of %s, today's adventures come with TwinBee's blessing — new zone & expedition runs start at +5 mood, expedition outfitting includes a complimentary standard pack, and every harvest yields one extra unit.\n\n", holidayName, holidayName))
	}

	// Pick a morning greeting
	greeting, _ := advPickFlavor(MorningDM, userID, "morning_dm")
	displayName, _ := loadDisplayName(userID)
	dndLevel := dndLevelForUser(userID)
	vars := map[string]string{
		"{name}": displayName,
		"{character_sheet}": fmt.Sprintf(
			"  ⚔️ Combat Lv.%d  ⛏️ Mining Lv.%d  🌿 Foraging Lv.%d  🎣 Fishing Lv.%d\n  💰 €%.0f",
			dndLevel, char.MiningSkill, char.ForagingSkill, char.FishingSkill, balance),
	}
	sb.WriteString(advSubstituteFlavor(greeting, vars))
	sb.WriteString("\n\n")

	// Active buffs
	buffs, _ := loadAdvActiveBuffs(userID)
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

	// L3 closure announcement — surfaced for one-week post-deploy soak so
	// active co-op participants see why the system is gone. Remove after
	// 2026-05-16 (TODO: drop announcement block at that revisit).
	sb.WriteString("📋 **Co-op dungeons closed for now** — `!expedition` is the way forward; a better co-op design is on the way.\n\n")

	// Rival nudge — a pending challenge waits for action.
	if line := renderRivalNudge(userID); line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Crafting teaser — surface the system when it's near unlock or post-unlock.
	if line := renderCraftingTeaser(userID); line != "" {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Phase R1 — the daily activity loop (dungeon/mine/forage/fish) has been
	// retired in favor of the expedition system. The menu now shows the new
	// entry points for adventuring and keeps the still-active town services.
	sb.WriteString("**🗺️ Adventure** — head into a zone:\n")
	sb.WriteString("• `!expedition` — overview & open expeditions\n")
	sb.WriteString("• `!expedition start <zone>` — begin a new run\n")
	sb.WriteString("• Harvest is automatic — yields land as you walk through cleared rooms.\n")
	sb.WriteString("\n")

	sb.WriteString("**🏘️ In town:**\n")
	sb.WriteString("• `5` / `shop` — buy/sell gear and loot\n")
	sb.WriteString("• `6` / `blacksmith` — repair damaged equipment\n")
	sb.WriteString("• `7` / `rest` — skip today, bank your luck\n")
	sb.WriteString("• `!thom` — visit Krooke Realty 🏠\n\n")

	sb.WriteString("Reply with a number for in-town services, or run an expedition command directly.\n")
	sb.WriteString("You have until midnight UTC to rest.")

	return sb.String()
}

// ── Resolution DM ────────────────────────────────────────────────────────────

func renderAdvResolutionDM(result *AdvActionResult, userID id.UserID) string {
	var sb strings.Builder

	sb.WriteString(result.FlavorText)
	sb.WriteString("\n\n")

	// Summary line
	sb.WriteString("───────────────────\n")

	if result.Outcome == AdvOutcomeDeath {
		sb.WriteString("💀 **You died.**\n")
		if char, err := loadAdvCharacter(userID); err == nil && char != nil && char.DeadUntil != nil {
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

func renderAdvDeathStatusDM(userID id.UserID) string {
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return "💀 You're still dead."
	}
	// Cost mirrors hospitalCostsForUser (post-L4a): D&D Level × 50k after insurance.
	_, cost := hospitalCostsForUser(char)
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

func renderAdvRespawnDM(userID id.UserID) string {
	text, _ := advPickFlavor(RespawnDM, userID, "respawn_dm")
	displayName, _ := loadDisplayName(userID)
	return advSubstituteFlavor(text, map[string]string{
		"{name}": displayName,
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

// ── Idle Shame DM ────────────────────────────────────────────────────────────

func renderAdvIdleShameDM(userID id.UserID) string {
	text, _ := advPickFlavor(IdleShameDM, userID, "idle_shame")
	displayName, _ := loadDisplayName(userID)
	return advSubstituteFlavor(text, map[string]string{
		"{name}": displayName,
	})
}

// ── Onboarding DM ────────────────────────────────────────────────────────────

func renderAdvOnboardingDM(userID id.UserID) string {
	text, _ := advPickFlavor(OnboardingDM, userID, "onboarding")
	displayName, _ := loadDisplayName(userID)
	return advSubstituteFlavor(text, map[string]string{
		"{name}": displayName,
	})
}

// ── Daily Summary ────────────────────────────────────────────────────────────

type AdvPlayerDaySummary struct {
	DisplayName string

	// D&D layer (preferred render). HasDnDChar=false → fall back to legacy
	// skill-line render and "needs !setup" hint.
	HasDnDChar bool
	DnDLevel   int
	DnDRace    string
	DnDClass   string
	HPCurrent  int
	HPMax      int

	// Legacy AdventureCharacter skill levels (rendered as fallback when
	// HasDnDChar is false).
	Level         int // legacy combat level
	MiningSkill   int
	ForagingSkill int
	FishingSkill  int

	Activity  string
	Location  string
	Outcome   string
	LootValue int64
	IsDead    bool
	DeadUntil string
	// DeadUntilHours is the integer hours-from-now until revival, used by
	// the standout-death template's {hours} placeholder. Computed when
	// the summary row is built; 0 if not dead or already past revival.
	DeadUntilHours int
	// DeadUntilDuration is the human-readable hours+minutes form of the
	// remaining respawn time ("5h 27m", "47m", "2h"). Used by the
	// {duration} placeholder for templates that want precision over the
	// rounded {hours} count.
	DeadUntilDuration string
	IsResting         bool
	SummaryLine       string
	HolidayActions    int // 0 = not holiday or no action; 1 = took one; 2 = took both
	DeathSource       string
	DeathLocation     string
}

// advPlayerHeadline renders the per-player headline for the daily report.
// D&D shape ("Lv.4 Half-Elf Cleric — HP 27/27") is preferred; falls back to
// the legacy four-skill line for accounts that haven't run !setup yet.
func advPlayerHeadline(p *AdvPlayerDaySummary) string {
	if p.HasDnDChar {
		race := titleizeDnDToken(p.DnDRace)
		class := titleizeDnDToken(p.DnDClass)
		return fmt.Sprintf("Lv.%d %s %s — HP %d/%d",
			p.DnDLevel, race, class, p.HPCurrent, p.HPMax)
	}
	return fmt.Sprintf("Combat Lv.%d | Mining Lv.%d | Forage Lv.%d | Fishing Lv.%d _(no D&D sheet — run `!setup`)_",
		p.Level, p.MiningSkill, p.ForagingSkill, p.FishingSkill)
}

// titleizeDnDToken converts a snake_case D&D enum token like "half_elf" into
// a human-readable label like "Half-Elf". Single-word tokens just title-case.
func titleizeDnDToken(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "-")
}

func renderAdvDailySummary(date string, tb *TwinBeeResult, tbRewards TwinBeeRewardSummary, players []AdvPlayerDaySummary, holidayName string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📜 **ADVENTURER DAILY REPORT**\n%s\n\n", date))

	if holidayName != "" {
		sb.WriteString(fmt.Sprintf("🎉 In recognition of **%s**, adventurers ran with TwinBee's blessing today: +5 starting mood, a free standard pack at outfitting, and +1 to every harvest yield.\n\n", holidayName))
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
		sb.WriteString("\n(Players who rested today received nothing. Fallen adventurers still earn their share. I noticed.)\n\n")
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
				sb.WriteString(fmt.Sprintf("%s **%s** — %s\n", icon, p.DisplayName, advPlayerHeadline(p)))
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

		sb.WriteString(fmt.Sprintf("⚔️ **%s** — %s\n", p.DisplayName, advPlayerHeadline(p)))
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
			hours := worstPlayer.DeadUntilHours
			if hours <= 0 {
				hours = 6 // canonical respawn duration when revival time isn't computable
			}
			duration := worstPlayer.DeadUntilDuration
			if duration == "" {
				duration = fmt.Sprintf("%dh", hours)
			}
			line = advSubstituteFlavor(line, map[string]string{
				"{name}":     worstPlayer.DisplayName,
				"{location}": lossLoc,
				"{hours}":    fmt.Sprintf("%d", hours),
				"{duration}": duration,
			})
			sb.WriteString(fmt.Sprintf("🏆 **Today's standout:** %s\n", line))
		}
	}

	return sb.String()
}

// ── Leaderboard ──────────────────────────────────────────────────────────────

// AdvLeaderboardEntry is the view-model the leaderboard renderer consumes.
// Callers populate it from AdvCharacter rows + a D&D level lookup so the
// renderer doesn't depend on the legacy character type.
type AdvLeaderboardEntry struct {
	UserID        id.UserID
	Level         int
	MiningSkill   int
	ForagingSkill int
	FishingSkill  int
	CurrentStreak int
}

func renderAdvLeaderboard(chars []AdvLeaderboardEntry) string {
	if len(chars) == 0 {
		return "No adventurers registered yet. Type `!adventure` to begin."
	}

	// Sort by score
	type entry struct {
		Name   string
		Score  int
		Levels string
		Streak int
	}
	var entries []entry
	for _, c := range chars {
		score := (c.Level + c.MiningSkill + c.ForagingSkill + c.FishingSkill) * 10
		name, _ := loadDisplayName(c.UserID)
		entries = append(entries, entry{
			Name:   name,
			Score:  score,
			Levels: fmt.Sprintf("⚔️%d ⛏️%d 🌿%d 🎣%d", c.Level, c.MiningSkill, c.ForagingSkill, c.FishingSkill),
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
	// The flavor templates below are written for the base 3-slot set. With a
	// T3 trophy room the set can hold 4, so fall back to a plain dynamic list
	// whenever it isn't exactly 3 — every slot has to be visible to choose it.
	if len(TreasureInventoryCap) == 0 || len(existing) != 3 {
		var b strings.Builder
		fmt.Fprintf(&b, "You found **%s** but your treasure slots are full. Discard one to make room:\n", newTreasure.Name)
		for i, ex := range existing {
			fmt.Fprintf(&b, "%d. %s — %s\n", i+1, ex.Name, ex.InventoryDesc)
		}
		fmt.Fprintf(&b, "\nReply with the number to discard, or `keep` to leave the new one behind.")
		return b.String()
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
