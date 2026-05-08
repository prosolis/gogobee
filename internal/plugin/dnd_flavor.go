package plugin

import (
	"strings"

	"gogobee/internal/flavor"
)

// D&D-layer flavor wire-in. Helpers here pull from the protected pools in
// internal/flavor and are called at the relevant integration points
// (combat outputs, level-up DM, rest commands, NPC encounters, loot drops).
//
// Each helper returns "" when the corresponding pool is empty so callers can
// safely embed the result without conditional-noise everywhere.

// ── Combat narrative ─────────────────────────────────────────────────────────

// dndCombatOpeningLine returns a single CombatStart pool line for use at
// the top of a fight's combat-log render. Caller decides where to inject it
// (typically prepended to the first phase message).
func dndCombatOpeningLine() string {
	return flavor.Pick(flavor.CombatStart)
}

// dndCombatClosingLine returns a single line appropriate for the fight's
// end state: CombatVictory on a non-boss win, BossDeath on a boss kill,
// PlayerDeath on a loss.
func dndCombatClosingLine(playerWon, isBoss bool) string {
	switch {
	case playerWon && isBoss:
		return flavor.Pick(flavor.BossDeath)
	case playerWon:
		return flavor.Pick(flavor.CombatVictory)
	default:
		return flavor.Pick(flavor.PlayerDeath)
	}
}

// dndZoneCompleteLine fires on dungeon completion (only on victorious clear).
func dndZoneCompleteLine() string {
	return flavor.Pick(flavor.ZoneComplete)
}

// dndNat20Line / dndNat1Line — appended to the fight's roll summary when
// at least one nat 20 / nat 1 was rolled by the player. A single line per
// fight (not per roll) to avoid spam.
func dndNat20Line() string {
	return flavor.Pick(flavor.Nat20)
}

func dndNat1Line() string {
	return flavor.Pick(flavor.Nat1)
}

// ── Level-up + items ────────────────────────────────────────────────────────

// dndLevelUpFlavorLine — random LevelUp pool entry for the level-up DM.
func dndLevelUpFlavorLine() string {
	return flavor.Pick(flavor.LevelUp)
}

// dndItemFoundLine — used when loot drops in dungeon resolution.
func dndItemFoundLine() string {
	return flavor.Pick(flavor.ItemFound)
}

// ── Rest ─────────────────────────────────────────────────────────────────────

// dndRestShortFlavorLine — for !rest short.
func dndRestShortFlavorLine() string {
	return flavor.Pick(flavor.RestShort)
}

// dndRestLongFlavorLine — for !rest long. The HomeLongRest pool is
// preferred when the player is at home; falls back to generic RestLong.
func dndRestLongFlavorLine(atHome bool) string {
	if atHome {
		if line := flavor.Pick(flavor.HomeLongRest); line != "" {
			return line
		}
	}
	return flavor.Pick(flavor.RestLong)
}

// ── NPC greetings ───────────────────────────────────────────────────────────
//
// The NPC encounter system already has legacy `mistyOpenings` and
// `arinaOpenings` slices used for prompt openings. We don't replace those —
// we union them with the new D&D-flavored greetings so players see ~2x
// variety in the openings. Helpers below return the combined pool.

// dndMistyGreetingPool returns the union of legacy and D&D pools for Misty.
// Used by the encounter-fire path to vary opening lines.
func dndMistyGreetingPool() []string {
	combined := make([]string, 0, len(flavor.MistyGreeting)+len(mistyOpenings))
	combined = append(combined, flavor.MistyGreeting...)
	combined = append(combined, mistyOpenings...)
	return combined
}

func dndArinaGreetingPool() []string {
	combined := make([]string, 0, len(flavor.ArinaGreeting)+len(arinaOpenings))
	combined = append(combined, flavor.ArinaGreeting...)
	combined = append(combined, arinaOpenings...)
	return combined
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// dndItalicize wraps a non-empty line in markdown italics with leading
// blank line so callers can append narrative cleanly.
func dndItalicize(line string) string {
	if strings.TrimSpace(line) == "" {
		return ""
	}
	return "\n\n_" + line + "_"
}
