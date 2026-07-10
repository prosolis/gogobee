package plugin

import (
	"fmt"
	"strings"
)

// Well-rested — a long rest taken at a home of tier 2+ grants a temporary buff
// that lasts until the next long rest. Renting a room at Thom Krooke's inn does
// not: the perk exists to give owning (and upgrading) a house a payoff the inn
// can't match, and it turns the dead T3/T4 prestige tiers into something worth
// buying.
//
// Two halves, both scaling with house tier (2 < 3 < 4):
//   - wellRestedTempHP: a temporary hit-point cushion, layered above MaxHP for
//     the day's fights via applyDnDHPScaling; and
//   - wellRestedSlotBonus: bonus spell slots folded into the caster's pool at
//     their highest available slot level.
//
// The tuning below is deliberately generous toward casters, who trail on
// spell-pool richness — the bonus slots are the substantive reward and the
// temp HP is a lighter feel-good cushion. All values are tunable.
//
// Nothing here is a secret discovery mechanic (cf. Misty/Arina), so the rest
// message surfaces exactly what was granted.

// homeRestTempHPPct maps house tier → temp-HP fraction of MaxHP.
func homeRestTempHPPct(houseTier int) float64 {
	switch {
	case houseTier >= 4:
		return 0.16
	case houseTier >= 3:
		return 0.12
	case houseTier >= 2:
		return 0.08
	default:
		return 0
	}
}

// homeRestBonusSlots maps house tier → number of bonus spell slots.
func homeRestBonusSlots(houseTier int) int {
	switch {
	case houseTier >= 4:
		return 3
	case houseTier >= 3:
		return 2
	case houseTier >= 2:
		return 1
	default:
		return 0
	}
}

// wellRestedTempHP returns the temporary HP granted by a home long rest at the
// given house tier. Tier 1 and the inn grant none. Always at least 1 when the
// tier qualifies, so a low-HP character still sees the buff.
func wellRestedTempHP(hpMax, houseTier int) int {
	pct := homeRestTempHPPct(houseTier)
	if pct <= 0 {
		return 0
	}
	hp := int(float64(hpMax) * pct)
	if hp < 1 {
		hp = 1
	}
	return hp
}

// wellRestedSlotBonus returns the temporary bonus spell slots granted by a home
// long rest, placed entirely at the caster's highest available slot level (the
// most valuable, and one a caster is guaranteed to have). Tier 1, the inn, and
// non-casters (empty pool) grant none.
func wellRestedSlotBonus(houseTier int, pool map[int]int) map[int]int {
	n := homeRestBonusSlots(houseTier)
	if n == 0 || len(pool) == 0 {
		return nil
	}
	highest := 0
	for lvl := range pool {
		if lvl > highest {
			highest = lvl
		}
	}
	if highest == 0 {
		return nil
	}
	return map[int]int{highest: n}
}

// wellRestedSummary renders the player-facing description of a granted buff,
// e.g. "+18 temp HP, +2 level-5 spell slots". Returns "" when nothing was
// granted (tier 1, the inn, or a non-caster with no temp HP).
func wellRestedSummary(tempHP int, slotBonus map[int]int) string {
	var bits []string
	if tempHP > 0 {
		bits = append(bits, fmt.Sprintf("+%d temp HP", tempHP))
	}
	for lvl, n := range slotBonus {
		if n <= 0 {
			continue
		}
		plural := ""
		if n != 1 {
			plural = "s"
		}
		bits = append(bits, fmt.Sprintf("+%d level-%d spell slot%s", n, lvl, plural))
	}
	return strings.Join(bits, ", ")
}
