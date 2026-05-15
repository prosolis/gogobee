package plugin

// Phase R3 — Combat-link integration for expeditions.
// Spec: gogobee_resource_combat_integration.md §4.
//
// Surface:
//   • resolveCombatInterrupt — §4.2 d20+tier brackets for harvest interrupts.
//   • runHarvestInterrupt    — picks an enemy, runs combat, returns a
//     narration block + endedRun flag for the caller to fold in.
//   • monsterKillTags        — bestiary→tag map used by the kill-log writer
//     so RequiresKill resources unlock once their gating monster falls.
//   • recordZoneKill         — appends tags into RegionState["kills"][region].
//   • tryPatrolEncounter     — Threat-Clock Alert+ pre-room patrol roll.
//
// Notes / scope discipline:
//   • The §4.1 "surprise round" mechanic is approximated as one free enemy
//     swing before normal combat. The dnd combat engine doesn't expose a
//     real surprise primitive yet, and a one-shot HP nick captures the
//     intent without forking SimulateCombat. Polish lives in R6.
//   • §4.1 Night Ambush integration is left to the existing wandering-
//     monster system — that path is independent of !advance.

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// retreatThreatBump is the threat penalty applied when the player times
// out of a combat (PlayerEndHP > 0 but PlayerWon == false). Retreats
// represent the player breaking off wounded — the run continues, but the
// zone's awareness ratchets up. Tuned alongside the expedition-difficulty
// pass (see gogobee_expedition_difficulty.md): low enough not to compound
// brutally with chained retreats, high enough that 3–4 retreats walks the
// threat clock toward Stirring.
//
// History: pre-Phase-2 the engine's timeout-loss path called
// abandonZoneRun + retireAllRegionRuns, ending the expedition outright
// despite the engine's contract ("Timeout = retreat, not lethal blow").
// That made any single fight loss across a 14-day expedition an
// auto-fail, which the sim harness exposed as uniform-0% completion
// across every tier. Splitting the retreat path here was Phase 2's
// first lever.
const retreatThreatBump = 5

// ── Combat Interrupt (§4.2) ─────────────────────────────────────────────────

// CombatInterruptKind is the bucket the d20+tier roll lands in.
type CombatInterruptKind int

const (
	InterruptNone     CombatInterruptKind = iota // 1–8
	InterruptNoise                               // 9–14, threat +2
	InterruptStandard                            // 15–18, 1 enemy, surprise
	InterruptElite                               // 19–21, elite enemy, node not depleted
	InterruptPatrol                              // 22+, 1d3 enemies, harvest fails
)

// resolveCombatInterrupt rolls 1d20 + zone tier + threat-mod, applies
// class adjustments, and returns the bracket. rollFn is injectable for
// tests; pass nil for the live d20.
//
//	threatLevel: 0–100 expedition threat
//	tier:        zone tier 1..5
//	class:       active character class
//	zoneID:      for Ranger wilderness check (§4.1)
func resolveCombatInterrupt(
	threatLevel, tier int,
	class DnDClass,
	zoneID ZoneID,
	rollFn func() int,
) (CombatInterruptKind, int) {
	if rollFn == nil {
		rollFn = func() int { return rand.IntN(20) + 1 }
	}
	r := rollFn()
	mod := tier
	if threatLevel > 40 {
		// §4.2: +1 per 20 threat above 40.
		mod += (threatLevel - 40) / 20
	}
	// Ranger: -3 to interrupt roll in wilderness zones (§4.1).
	if class == ClassRanger && wildernessZones[zoneID] {
		mod -= 3
	}
	total := r + mod
	switch {
	case total >= 22:
		return InterruptPatrol, total
	case total >= 19:
		return InterruptElite, total
	case total >= 15:
		return InterruptStandard, total
	case total >= 9:
		return InterruptNoise, total
	default:
		return InterruptNone, total
	}
}

// runHarvestInterrupt resolves a Standard/Elite/Patrol interrupt: picks
// an enemy from the zone roster, applies the surprise-round HP nick, runs
// SimulateCombat, persists the kill (R3 kill-log writer), and returns
// the narration block + endedRun (true if the player went down).
func (p *AdventurePlugin) runHarvestInterrupt(
	userID id.UserID,
	exp *Expedition,
	run *DungeonRun,
	zone ZoneDefinition,
	kind CombatInterruptKind,
) (string, bool) {
	elite := kind == InterruptElite
	monster, ok := pickZoneEnemy(zone, run.RunID, run.CurrentRoom, elite)
	if !ok {
		return fmt.Sprintf("_(No %s roster entry — interrupt skipped.)_",
			map[bool]string{true: "elite", false: "standard"}[elite]), false
	}

	// Surprise round (§4.1): a single free enemy swing nicks HP. We cap at
	// HP-1 so the nick alone can't KO; real combat resolves below.
	preDmg := surpriseRoundNick(monster, int(zone.Tier))
	dndChar, _ := LoadDnDCharacter(userID)
	if dndChar != nil && preDmg > 0 {
		if preDmg >= dndChar.HPCurrent {
			preDmg = dndChar.HPCurrent - 1
			if preDmg < 0 {
				preDmg = 0
			}
		}
		dndChar.HPCurrent -= preDmg
		_ = SaveDnDCharacter(dndChar)
	}

	preCombatHP, _ := dndHPSnapshot(userID)
	result, err := p.runZoneCombat(userID, monster, int(zone.Tier), nil, run.DMMood)
	if err != nil {
		return fmt.Sprintf("_(Interrupt combat error: %v.)_", err), false
	}
	postHP, maxHP := dndHPSnapshot(userID)
	scanMoodEventsFromCombat(run.RunID, result)

	var b strings.Builder
	if kind == InterruptPatrol {
		b.WriteString(flavor.Pick(flavor.PatrolEncounter))
	} else {
		b.WriteString(flavor.Pick(flavor.HarvestInterrupt))
	}
	b.WriteString("\n")
	if elite {
		b.WriteString(fmt.Sprintf("⚔️ **Elite interrupt — %s** (HP %d, AC %d)\n",
			monster.Name, monster.HP, monster.AC))
	} else {
		b.WriteString(fmt.Sprintf("⚔️ **%s** (HP %d, AC %d)\n",
			monster.Name, monster.HP, monster.AC))
	}
	if preDmg > 0 {
		b.WriteString(fmt.Sprintf("_Surprise round — %s strikes first for %d HP._\n",
			monster.Name, preDmg))
	}

	if !result.PlayerWon {
		if result.TimedOut {
			// Retreat: fighter broke off wounded but alive. The engine's
			// contract (combat_engine.go: "Timeout = retreat, not lethal
			// blow") means HP stays where the engine left it and we keep
			// the run going. The harvest slot is forfeit (no kill, no
			// loot) and threat ticks up.
			_ = applyThreatDelta(exp.ID, retreatThreatBump, "combat retreat")
			b.WriteString(fmt.Sprintf("⏳ **%s** outlasts you. You break off, wounded but alive. (Threat +%d.)",
				monster.Name, retreatThreatBump))
			return b.String(), false
		}
		// True death.
		_, _ = applyMoodEvent(run.RunID, MoodEventPlayerDeath)
		_ = abandonZoneRun(userID)
		_ = retireAllRegionRuns(exp)
		markAdventureDead(userID, "expedition", zone.Display)
		if line := flavor.Pick(flavor.PlayerDeath); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("💀 You fell to **%s**. Run ended.", monster.Name))
		return b.String(), true
	}

	// Win: kill-log writer.
	_ = recordZoneKill(exp, monster.ID)
	if line := flavor.Pick(flavor.CombatVictory); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("✅ **%s** down (HP %d→%d / %d).",
		monster.Name, preCombatHP, postHP, maxHP))
	if drop := p.dropZoneLoot(userID, zone.ID, monster, false); drop != "" {
		b.WriteString("\n")
		b.WriteString(drop)
	}
	return b.String(), false
}

// surpriseRoundNick computes a small HP nick representing the enemy's
// free first swing. Roughly attack-bonus + 1d4, with a tier-based floor.
func surpriseRoundNick(m DnDMonsterTemplate, tier int) int {
	if tier < 1 {
		tier = 1
	}
	dmg := 1 + rand.IntN(4) + m.AttackBonus/2
	floor := tier
	if dmg < floor {
		dmg = floor
	}
	return dmg
}

// ── Kill-log writer ─────────────────────────────────────────────────────────

// monsterKillTags maps a bestiary ID to the kill-log tags it satisfies.
// Tags align with ZoneResource.RequiresKill values in dnd_resource_registry.go;
// some monsters cover multiple gates (an Owlbear is both "owlbear" and
// generic "beast" for Forest of Shadows pelts).
func monsterKillTags(bestiaryID string) []string {
	switch bestiaryID {
	case "worg":
		return []string{"worg", "beast"}
	case "owlbear":
		return []string{"owlbear", "beast"}
	case "dire_wolf", "displacer_beast", "dryad_corrupted":
		return []string{"beast"}
	case "kuo_toa", "kuo_toa_whip":
		return []string{"kuo-toa"}
	case "vampire_spawn":
		return []string{"vampire_spawn"}
	case "wraith":
		return []string{"wraith"}
	case "azer":
		return []string{"azer"}
	case "salamander":
		return []string{"salamander"}
	case "helmed_horror", "fire_elemental", "water_elemental":
		return []string{"construct"}
	case "drow", "drow_elite_warrior", "drow_mage":
		return []string{"drow"}
	case "mind_flayer":
		return []string{"illithid"}
	case "will_o_wisp":
		return []string{"will_o_wisp"}
	case "green_hag", "night_hag":
		return []string{"hag"}
	case "boss_thornmother":
		return []string{"thornmother"}
	case "guard_drake":
		return []string{"drake"}
	case "young_red_dragon":
		return []string{"drake"}
	case "boss_infernax":
		return []string{"infernax"}
	case "quasit", "vrock", "hezrou", "nalfeshnee", "marilith":
		return []string{"demon"}
	case "boss_belaxath":
		return []string{"balor", "demon", "portal_boss"}
	}
	return nil
}

// recordZoneKill appends every kill-tag for `bestiaryID` to
// RegionState["kills"][regionKey] and persists. No-op for monsters that
// gate no resources.
func recordZoneKill(exp *Expedition, bestiaryID string) error {
	if exp == nil {
		return nil
	}
	tags := monsterKillTags(bestiaryID)
	if len(tags) == 0 {
		return nil
	}
	if exp.RegionState == nil {
		exp.RegionState = map[string]any{}
	}
	table := loadKillsTable(exp)
	regionKey := regionHarvestKey(exp)
	existing := table[regionKey]
	for _, t := range tags {
		if !stringSliceContains(existing, t) {
			existing = append(existing, t)
		}
	}
	table[regionKey] = existing
	exp.RegionState["kills"] = table
	return persistRegionState(exp)
}

// loadKillsTable normalises RegionState["kills"] to map[region][]string
// regardless of whether it round-tripped through JSON.
func loadKillsTable(e *Expedition) map[string][]string {
	out := map[string][]string{}
	if e == nil || e.RegionState == nil {
		return out
	}
	raw, ok := e.RegionState["kills"]
	if !ok {
		return out
	}
	switch v := raw.(type) {
	case map[string][]string:
		return v
	case map[string]any:
		// JSON path: re-marshal then re-parse.
		if b, err := json.Marshal(v); err == nil {
			_ = json.Unmarshal(b, &out)
		}
	}
	return out
}

func stringSliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// recordZoneKillForUser is the zone-combat-side entry point: looks up the
// player's active expedition (if any) and records the kill against it.
// Safe when the user is doing a standalone (non-expedition) zone run —
// recordZoneKill returns nil with no exp.
func recordZoneKillForUser(userID id.UserID, bestiaryID string) {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return
	}
	_ = recordZoneKill(exp, bestiaryID)
}

// ── Patrol Encounters (§4.1) ────────────────────────────────────────────────

// patrolThreshold is the threat-band floor at which patrols start moving
// between cleared rooms. Below Alert (level < 41), no patrols.
const patrolThreatFloor = 41

// patrolBaseChance is the per-advance roll for a patrol when the threat
// clock is at the floor; scales linearly toward Siege.
const patrolBaseChance = 0.10

// rollPatrolChance returns the per-advance probability that a patrol
// shows up between rooms. 0 below Alert; ~0.10 at 41, ~0.30 at 100.
func rollPatrolChance(threatLevel int) float64 {
	if threatLevel < patrolThreatFloor {
		return 0
	}
	over := float64(threatLevel - patrolThreatFloor)
	span := float64(100 - patrolThreatFloor)
	if span <= 0 {
		return patrolBaseChance
	}
	return patrolBaseChance + (0.20 * over / span)
}

// tryPatrolEncounter is called from zoneCmdAdvance *before* the next room
// resolves. If the player's active expedition is at Alert+, we roll for a
// patrol; on hit, we run a single combat against a non-elite roster entry.
//
// Returns staged messages so the patrol fight gets the same 2–3s pacing
// as room/boss combat — see resolveCombatRoom for the contract. When no
// patrol fires, intro/phases/outcome are all empty and ended is false.
func (p *AdventurePlugin) tryPatrolEncounter(
	userID id.UserID,
	run *DungeonRun,
	zone ZoneDefinition,
) (intro string, phases []string, outcome string, ended bool, err error) {
	exp, gerr := getActiveExpedition(userID)
	if gerr != nil || exp == nil {
		return
	}
	if exp.RunID != run.RunID {
		return
	}
	chance := rollPatrolChance(exp.ThreatLevel)
	if chance <= 0 || rand.Float64() > chance {
		return
	}
	monster, ok := pickZoneEnemy(zone, run.RunID, run.CurrentRoom, false)
	if !ok {
		return
	}
	result, rerr := p.runZoneCombat(userID, monster, int(zone.Tier), nil, run.DMMood)
	if rerr != nil {
		err = rerr
		return
	}
	postHP, maxHP := dndHPSnapshot(userID)
	scanMoodEventsFromCombat(run.RunID, result)

	// Intro: patrol-encounter flavor + creature stat block.
	var ib strings.Builder
	ib.WriteString(flavor.Pick(flavor.PatrolEncounter))
	ib.WriteString("\n")
	ib.WriteString(fmt.Sprintf("🛡 **Patrol — %s** (HP %d, AC %d)",
		monster.Name, monster.HP, monster.AC))
	intro = ib.String()

	// Phases: forward-simulating engine play-by-play.
	playerName := "You"
	if name, _ := loadDisplayName(userID); name != "" {
		playerName = name
	}
	phases = RenderCombatLog(result, playerName, monster.Name)

	var ob strings.Builder
	if !result.PlayerWon {
		if result.TimedOut {
			// Retreat — see retreatThreatBump doc. Run continues; threat
			// ticks; the patrol's awareness lingers as a soft penalty
			// instead of an auto-fail.
			_ = applyThreatDelta(exp.ID, retreatThreatBump, "patrol retreat")
			ob.WriteString(fmt.Sprintf("⏳ The patrol drags on. You break off, wounded but alive. (Threat +%d.)",
				retreatThreatBump))
			if rollLine := dndRollSummaryLine(result); rollLine != "" {
				ob.WriteString("\n")
				ob.WriteString(rollLine)
			}
			outcome = ob.String()
			ended = false
			return
		}
		_, _ = applyMoodEvent(run.RunID, MoodEventPlayerDeath)
		_ = abandonZoneRun(userID)
		_ = retireAllRegionRuns(exp)
		markAdventureDead(userID, "patrol", zone.Display)
		if line := flavor.Pick(flavor.PlayerDeath); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
		ob.WriteString("💀 The patrol takes you down. Run ended.")
		if rollLine := dndRollSummaryLine(result); rollLine != "" {
			ob.WriteString("\n")
			ob.WriteString(rollLine)
		}
		outcome = ob.String()
		ended = true
		return
	}
	_ = recordZoneKill(exp, monster.ID)
	ob.WriteString(fmt.Sprintf("✅ Patrol dispatched. You finished at **%d/%d HP**.",
		postHP, maxHP))
	if rollLine := dndRollSummaryLine(result); rollLine != "" {
		ob.WriteString("\n")
		ob.WriteString(rollLine)
	}
	if drop := p.dropZoneLoot(userID, zone.ID, monster, false); drop != "" {
		ob.WriteString("\n")
		ob.WriteString(drop)
	}
	outcome = ob.String()
	return
}
