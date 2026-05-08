package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Phase 3 — XP, level-up, and progression.
//
// XP curve anchors come from the design doc (v1.0 §4.1). Intermediate levels
// are interpolated to give a smooth ramp. Cumulative XP — the value stored
// in dnd_character.dnd_xp resets to 0 on level-up (carrying over surplus).
//
// Two design choices worth flagging:
//  1. XP is *segment-based*. dnd_xp tracks XP toward the next level, not
//     total XP earned across all levels. This keeps the math simple and
//     means a player who migrated in at L8 starts with 0/[L9-cost], no
//     adjustment needed for the levels they "didn't earn."
//  2. Combat XP only — no XP from gathering activities. Mining/foraging/
//     fishing keep their own legacy skill XP tracks.

// dndXPTable[L] = cumulative XP needed to reach level L from level 1.
// L=0 is unused (D&D characters start at L1).
var dndXPTable = [...]int{
	0,      // L1 (start)
	300,    // L2
	900,    // L3
	1700,   // L4
	2700,   // L5  (anchor: doc)
	4300,   // L6
	6500,   // L7  (anchor: doc)
	9000,   // L8
	11500,  // L9
	14000,  // L10 (anchor: doc)
	18000,  // L11
	22000,  // L12
	26000,  // L13
	30000,  // L14
	34000,  // L15 (anchor: doc)
	44000,  // L16
	54000,  // L17
	64000,  // L18
	74000,  // L19
	85000,  // L20 (anchor: doc)
}

const dndMaxLevel = 20

// dndXPToNextLevel returns the segment cost to advance from currentLevel to
// currentLevel+1. Returns 0 at L20 (already capped).
func dndXPToNextLevel(currentLevel int) int {
	if currentLevel < 1 {
		currentLevel = 1
	}
	if currentLevel >= dndMaxLevel {
		return 0
	}
	return dndXPTable[currentLevel] - dndXPTable[currentLevel-1]
}

// LevelUpEvent describes one level threshold crossed. A single grantDnDXP
// call can produce multiple events (cascading level-ups).
type LevelUpEvent struct {
	NewLevel int
	HPGain   int // hp_max delta
}

// grantDnDXP adds XP to the player's D&D character and processes any
// level-ups that result. Returns the level-up events (empty if none).
//
// Side effects:
//   - dnd_xp incremented (with carry-over on level-up)
//   - dnd_level incremented for each level threshold crossed
//   - hp_max recomputed and hp_current bumped by the gain
//   - DM sent for each level-up
//   - dnd_character row persisted
//
// No-op if the player has no D&D character (auto-migration normally
// guarantees one before any combat, so this is paranoia).
func (p *AdventurePlugin) grantDnDXP(userID id.UserID, amount int) ([]LevelUpEvent, error) {
	if amount <= 0 {
		return nil, nil
	}
	c, err := LoadDnDCharacter(userID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.PendingSetup {
		return nil, nil
	}

	c.XP += amount

	var events []LevelUpEvent
	for c.Level < dndMaxLevel {
		cost := dndXPToNextLevel(c.Level)
		if c.XP < cost {
			break
		}
		c.XP -= cost

		// Recompute HP max for the new level. HPCurrent gains the same delta.
		conMod := abilityModifier(c.CON)
		oldMax := c.HPMax
		c.Level++
		c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
		gain := c.HPMax - oldMax
		if gain < 1 {
			gain = 1 // floor — even with very negative CON, give at least 1
			c.HPMax = oldMax + 1
		}
		c.HPCurrent += gain
		if c.HPCurrent > c.HPMax {
			c.HPCurrent = c.HPMax
		}
		// AC may change too if DEX-derived (unlikely without item changes).
		c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))

		events = append(events, LevelUpEvent{NewLevel: c.Level, HPGain: gain})
	}

	// Cap at L20 — overflow XP is silently dropped.
	if c.Level >= dndMaxLevel {
		c.XP = 0
	}

	if err := SaveDnDCharacter(c); err != nil {
		return events, err
	}

	if len(events) > 0 {
		// Phase 10 SUB3a-ii — Battle Master Relentless (L15) raises the
		// superiority cap from 4 → 5; reconcile any level-gated subclass pool
		// growth here. Idempotent and non-fatal: a stale pool just means the
		// player reaches their full cap on the next long rest at the latest.
		if c.Subclass != "" {
			_ = initSubclassResources(userID, c.Subclass, c.Level)
		}
		p.sendLevelUpDM(userID, c, events, amount)
	}

	return events, nil
}

func (p *AdventurePlugin) sendLevelUpDM(userID id.UserID, c *DnDCharacter, events []LevelUpEvent, xpThisGrant int) {
	if p == nil || p.Client == nil {
		return // tests construct AdventurePlugin{} without a Matrix client
	}
	if err := p.SendDM(userID, buildLevelUpMessage(c, events, dndLevelUpFlavorLine())); err != nil {
		slog.Error("dnd: level-up DM failed", "user", userID, "err", err)
	}
}

// buildLevelUpMessage formats the level-up DM. flavor is the optional flavor
// line (pass "" to skip). Pure function — exposed for tests.
func buildLevelUpMessage(c *DnDCharacter, events []LevelUpEvent, flavor string) string {
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	var b strings.Builder
	b.WriteString("✨ **LEVEL UP** ✨\n\n")
	if flavor != "" {
		b.WriteString("_" + flavor + "_\n\n")
	}

	if len(events) == 1 {
		ev := events[0]
		b.WriteString(fmt.Sprintf("You are now a **Level %d %s %s**.\n", ev.NewLevel, ri.Display, ci.Display))
		b.WriteString(fmt.Sprintf("  HP: %d → %d (+%d)\n", c.HPMax-ev.HPGain, c.HPMax, ev.HPGain))
	} else {
		b.WriteString(fmt.Sprintf("You crossed **%d** levels in one go!\n", len(events)))
		for _, ev := range events {
			b.WriteString(fmt.Sprintf("  → Level %d  (+%d HP)\n", ev.NewLevel, ev.HPGain))
		}
		b.WriteString(fmt.Sprintf("\nFinal: **Level %d %s %s** with %d HP.\n",
			c.Level, ri.Display, ci.Display, c.HPMax))
	}

	if c.Level >= dndMaxLevel {
		b.WriteString("\n_You've reached the level cap._")
	} else {
		next := dndXPToNextLevel(c.Level)
		b.WriteString(fmt.Sprintf("\nNext level: %d / %d XP.\n", c.XP, next))
	}

	// Mage spellbook budget: every level-up grants +2 leveled spells. Surface
	// the headroom so the player knows to run `!spells learn`.
	if c.Class == ClassMage {
		count, _ := mageLeveledKnownCount(c.UserID)
		cap := mageKnownSpellsCap(c.Level)
		if avail := cap - count; avail > 0 {
			b.WriteString(fmt.Sprintf("\n📖 _Spells available to learn: **%d** (run `!spells learn <name>`)._\n",
				avail))
		}
	}

	// Phase 10 — subclass selection cue. Fires when the player crosses L5
	// without already having a subclass (rare path: a multi-level grant
	// that crosses L5 keeps prompting until they pick). Existing-subclass
	// cases stay quiet on later level-ups.
	if c.Subclass == "" && c.Level >= dndSubclassMinLevel {
		for _, ev := range events {
			if ev.NewLevel >= dndSubclassMinLevel {
				b.WriteString("\n🎯 ")
				b.WriteString(renderSubclassPrompt(c.Class))
				b.WriteString("\n")
				break
			}
		}
	}
	return b.String()
}

// ── XP grant amounts ─────────────────────────────────────────────────────────

// Combat XP formulas. Tuned so a L1 player needs ~3 wins per level early
// and many more at high levels.
const (
	dndArenaXPPerThreat   = 12 // arena win XP = threat * this
	dndArenaXPMin         = 30 // arena win XP floor (low-threat fights still pay)
	dndDungeonXPPerTier   = 60 // dungeon win XP = tier^1.4 * this, roughly
	dndLossXPFraction     = 0.25
	dndNearDeathXPBonus   = 1.25 // multiplier for clutch wins
)

// arenaCombatXP returns the D&D XP to grant for an arena combat outcome.
func arenaCombatXP(result CombatResult, threatLevel int) int {
	base := dndArenaXPPerThreat * threatLevel
	if base < dndArenaXPMin {
		base = dndArenaXPMin
	}
	if result.PlayerWon {
		if result.NearDeath {
			return int(float64(base) * dndNearDeathXPBonus)
		}
		return base
	}
	return int(float64(base) * dndLossXPFraction)
}

// dungeonCombatXP returns the D&D XP to grant for a dungeon combat outcome.
// Quadratic-ish scaling so high-tier dungeons feel meaningfully more rewarding.
func dungeonCombatXP(result CombatResult, tier int) int {
	if tier < 1 {
		tier = 1
	}
	base := dndDungeonXPPerTier * tier * tier / 2 // T1=30, T3=270, T5=750
	if base < dndArenaXPMin {
		base = dndArenaXPMin
	}
	if result.PlayerWon {
		if result.NearDeath {
			return int(float64(base) * dndNearDeathXPBonus)
		}
		return base
	}
	return int(float64(base) * dndLossXPFraction)
}
