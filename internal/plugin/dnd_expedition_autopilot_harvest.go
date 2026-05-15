package plugin

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Phase 2 of expedition autopilot — auto-harvest on room entry.
//
// Design (settled 2026-05-14):
//   - All applicable actions auto-run on every walked room. Class- and
//     kill-gated nodes filter themselves out via the existing per-resource
//     restrictions.
//   - Rare+ nodes are NOT auto-attempted. If any sit available in the
//     room, autopilot stops with stopRareNode so the player can spend the
//     attempt deliberately.
//   - One attempt per eligible node per visit. A failed roll doesn't
//     retry; it logs the fail and moves on. Charges only decrement on
//     non-Nothing outcomes (mirrors handleHarvestCmd).
//   - Noise interrupts apply threat++ and continue (parity with manual).
//   - Standard/Elite/Patrol combat interrupts hard-stop the walk:
//     stopEnded on death, stopHarvestCombat otherwise.
//   - No SU surcharge — manual !harvest is free; autopilot matches.
//   - Per-room footer ("+2 Iron, +1 Sage; 1 fail") + a walk-end tally
//     across all rooms.

// autoHarvestSummary holds the aggregated outcome for one room's
// auto-harvest pass. Per-room footer renders from this; expeditionCmdRun
// also folds it into a walk-wide tally for the closing block.
type autoHarvestSummary struct {
	Yields    map[string]int    // resourceID -> total qty granted
	Names     map[string]string // resourceID -> display name (capture once)
	Fails     int
	NoiseInts int
}

// autoHarvestResult is what autoHarvestRoom returns to the autopilot loop.
type autoHarvestResult struct {
	Summary     autoHarvestSummary
	RarePending []ZoneResource // non-empty = at least one Rare+ node sits in the room
	CombatNarr  string         // non-empty = a combat interrupt fired during the pass
	PlayerEnded bool           // true = combat killed the player
}

// allHarvestActions lists every action autopilot will try per room. Order
// matters only for footer readability — class- and gate-restricted nodes
// drop out via the existing filters.
var allHarvestActions = []HarvestAction{
	HarvestForage,
	HarvestScavenge,
	HarvestMine,
	HarvestFish,
	HarvestEssence,
	HarvestCommune,
}

// isRarePlus reports whether a resource's rarity should pause autopilot.
// Common/Uncommon are auto-attempted; Rare and above stop the walk.
func isRarePlus(r DnDRarity) bool {
	switch r {
	case RarityRare, RarityEpic, RarityVeryRare, RarityLegendary:
		return true
	}
	return false
}

// autoHarvestRoom runs a single auto-harvest pass on the player's current
// room. Called by expeditionCmdRun between walked rooms. Does not advance
// the room; only resolves nodes already at the player's feet.
func (p *AdventurePlugin) autoHarvestRoom(
	userID id.UserID, exp *Expedition, char *DnDCharacter,
) (autoHarvestResult, error) {
	res := autoHarvestResult{
		Summary: autoHarvestSummary{
			Yields: map[string]int{},
			Names:  map[string]string{},
		},
	}
	if exp == nil || exp.RunID == "" {
		return res, nil
	}
	run, err := getZoneRun(exp.RunID)
	if err != nil || run == nil {
		return res, nil
	}
	if !currentRoomCleared(run) {
		return res, nil
	}

	nodeID := harvestNodeIDFor(run)
	nodes := loadHarvestNodes(exp, nodeID)
	if len(nodes) == 0 {
		return res, nil
	}

	zone, _ := getZone(exp.ZoneID)
	persistNodes := false

	for _, action := range allHarvestActions {
		if action == HarvestFish && !isFishingZone(exp.ZoneID) {
			continue
		}
		if blockReason := zoneConditionHarvestBlock(exp, action); blockReason != "" {
			continue
		}

		// One pass per action. Iterate every eligible node (not just the
		// lowest-DC one) so a room with two foragables gets two attempts.
		for i := range nodes {
			n := &nodes[i]
			if n.CurrentCharges <= 0 {
				continue
			}
			rdef, ok := FindZoneResource(exp.ZoneID, n.ResourceID)
			if !ok || rdef.Action != action {
				continue
			}
			if rdef.ClassRestrict != "" && rdef.ClassRestrict != char.Class {
				continue
			}
			if rdef.RequiresKill != "" && !regionKillsContains(exp, rdef.RequiresKill) {
				continue
			}
			if rdef.ConditionalEvent != "" && !regionEventActive(exp, rdef.ConditionalEvent) {
				continue
			}

			if isRarePlus(rdef.Rarity) {
				// Defer the decision to the player; don't consume the attempt.
				res.RarePending = append(res.RarePending, rdef)
				continue
			}

			// Interrupt roll, same model as handleHarvestCmd.
			interrupt, intTotal := resolveCombatInterrupt(
				exp.ThreatLevel, int(zone.Tier), char.Class, exp.ZoneID, nil)
			switch interrupt {
			case InterruptNoise:
				_ = applyThreatDelta(exp.ID, 2, "harvest noise (§4.2, autopilot)")
				res.Summary.NoiseInts++
				_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "interrupt",
					fmt.Sprintf("autopilot noise %s total=%d", string(action), intTotal), "")
				// fall through to the d20 roll, same as manual harvest.
			case InterruptStandard, InterruptElite, InterruptPatrol:
				body, ended := p.runHarvestInterrupt(userID, exp, run, zone, interrupt)
				res.CombatNarr = body
				res.PlayerEnded = ended
				_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "interrupt",
					fmt.Sprintf("autopilot %s kind=%d total=%d", string(action), int(interrupt), intTotal), "")
				if persistNodes {
					_ = saveHarvestNodes(exp, nodeID, nodes)
				}
				return res, nil
			}

			// Resolve the harvest attempt.
			roll := rand.IntN(20) + 1
			mod := harvestActionAbility(char, action) + classHarvestBonus(char.Class, action)
			if action == HarvestFish {
				if adv, _ := loadAdvCharacter(userID); adv != nil {
					mod += fishingSkillBonus(adv.FishingSkill)
				}
			}
			total := roll + mod
			dcDelta, _ := zoneConditionHarvestDCMod(exp, action)
			effectiveDC := rdef.DC + dcDelta
			outcome := resolveHarvestOutcome(total, effectiveDC)
			outcome = rangerRareCatchUpgrade(char.Class, action, outcome, nil)

			var grantedQty int
			switch outcome {
			case OutcomeNothing:
				res.Summary.Fails++
			case OutcomeCommon:
				grantedQty = 1
			case OutcomeStandard:
				grantedQty = rand.IntN(3) + 1
			case OutcomeRich:
				grantedQty = rand.IntN(3) + 1
				if bonus := pickRichBonus(exp.ZoneID, rdef.ID); bonus != nil {
					_ = grantHarvestYield(userID, *bonus, 1)
					res.Summary.Yields[bonus.ID] += 1
					res.Summary.Names[bonus.ID] = bonus.Name
				}
			}
			if grantedQty > 0 {
				_ = grantHarvestYield(userID, rdef, grantedQty)
				res.Summary.Yields[rdef.ID] += grantedQty
				res.Summary.Names[rdef.ID] = rdef.Name
				n.CurrentCharges--
				persistNodes = true
			}

			_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "harvest",
				fmt.Sprintf("autopilot %s %s d20=%d total=%d dc=%d → %s",
					string(action), rdef.ID, roll, total, rdef.DC, string(outcome)), "")
		}
	}

	if persistNodes {
		if err := saveHarvestNodes(exp, nodeID, nodes); err != nil {
			_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "harvest",
				fmt.Sprintf("autopilot persistence error: %v", err), "")
		}
	}
	return res, nil
}

// renderAutoHarvestFooter formats one room's pass as a single compact
// line: "_+2 Scrap Iron, +1 Shadow Herb · 1 fail · 1 close call._".
// Empty string means there's nothing worth showing.
func renderAutoHarvestFooter(s autoHarvestSummary) string {
	if len(s.Yields) == 0 && s.Fails == 0 && s.NoiseInts == 0 {
		return ""
	}
	parts := []string{}
	// Stable ordering — sort yield keys so test output is deterministic
	// and the footer reads consistently across rooms.
	keys := make([]string, 0, len(s.Yields))
	for k := range s.Yields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("+%d %s", s.Yields[k], s.Names[k]))
	}
	if s.Fails > 0 {
		parts = append(parts, fmt.Sprintf("%d fail", s.Fails))
	}
	if s.NoiseInts > 0 {
		noun := "close call"
		if s.NoiseInts > 1 {
			noun = "close calls"
		}
		parts = append(parts, fmt.Sprintf("%d %s", s.NoiseInts, noun))
	}
	return "_🎒 " + strings.Join(parts, " · ") + "._"
}

// renderWalkTally formats the cross-room cumulative haul for the walk's
// final block. Empty string when nothing was gathered.
func renderWalkTally(yields map[string]int, names map[string]string) string {
	if len(yields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(yields))
	for k := range yields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("**Walk haul:** ")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%d %s", yields[k], names[k]))
	}
	return b.String()
}

// renderRarePendingFooter announces which Rare+ nodes triggered the
// autopilot pause. Used by the stop reason footer in expeditionCmdRun.
func renderRarePendingFooter(pending []ZoneResource) string {
	if len(pending) == 0 {
		return ""
	}
	// De-dup by ID (a single resource can have multiple charges across
	// nodes; we want one mention).
	seen := map[string]bool{}
	var names []string
	for _, r := range pending {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		names = append(names, fmt.Sprintf("**%s** (%s)", r.Name, string(r.Rarity)))
	}
	sort.Strings(names)
	verb := "is"
	if len(names) > 1 {
		verb = "are"
	}
	return fmt.Sprintf("⏸ **Autopilot paused — rare yield nearby.** %s %s sitting in this room. `!%s` to take the shot, or `!expedition run` to push past.",
		strings.Join(names, ", "), verb,
		// suggest the action of the first pending resource as a hint.
		string(pending[0].Action))
}
