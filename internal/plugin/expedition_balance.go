package plugin

import (
	"fmt"
	"math/rand/v2"
	"sort"
)

// Measurement harness for the expedition-difficulty pass
// (gogobee_expedition_difficulty.md). Sibling to dnd_class_balance.go —
// same Monte-Carlo philosophy, different target metric. Class balance
// answers "is my class viable?"; this harness answers "is the dungeon
// fair?". Same combat engine, orthogonal knobs.
//
// Naming: the file lives under expedition_balance.go (no dnd_ prefix)
// per feedback_avoid_dnd_naming — new infrastructure files shouldn't
// pick up the trademark-adjacent prefix.
//
// ── Clock-injection seam (the Phase 0 critical de-risk) ────────────────────
//
// The live expedition runs on two wall-clock-driven goroutines:
//
//	expeditionBriefingTicker — 06:00 UTC, calls deliverBriefing
//	expeditionRecapTicker    — 21:00 UTC, calls deliverRecap
//	expedition_ambient.go    — every 3h, calls applyAmbientTick
//
// All three are unsuitable for a measurement harness: they're tied to
// wall time, they're 1-per-process singletons, and crucially their
// "advance" routines are heavily DB-coupled (deliverBriefing alone
// touches a half-dozen update statements).
//
// The decision Phase 0 makes here: **the harness does not run the
// tickers and does not inject a virtual time.Now**. Instead, advance
// is reimplemented as `advanceExpeditionOneDay`, a pure function that
// calls the *math-pure helpers* directly (applyDailyBurn,
// dailyThreatDrift, resolveCombatInterrupt, resolveWanderingCheck,
// SimulateCombat) on an in-memory *Expedition. No DB, no goroutines,
// no clock.
//
// What this gives up: any logic that lives only inside deliverBriefing
// (e.g. the temporal pre-burn override for Sunken Temple tidal, the
// approaching-siege RegionState["siege_warning_fired"] gate, the
// milestone narration). Those are flavor + bookkeeping, not difficulty
// math, so the cost is acceptable for measurement. Phase 1+ revisits
// any that turn out to affect outcomes.
//
// What this gives back: a self-contained replay that runs in <1ms per
// simulated day and produces reproducible numbers under a seedable
// RNG. The two combat helpers it leans on (SimulateCombat,
// surpriseRoundNick) already participate in the class-balance
// harness, so the contract is well-trodden.
//
// ── Phase 0 simplifying constraints ───────────────────────────────────────
//
// Per the plan doc, Phase 0's bar is "one cell runs to a sensible
// number." Specifically held off until Phase 1+:
//
//   - Boss completion path. Phase 0 uses survive-N-days as the
//     completion proxy. Boss assembly + per-zone room walks land in
//     Phase 1 alongside the full matrix.
//   - Per-region zones (multi-region E4 zones). Phase 0 treats every
//     zone as single-region; the registry's first region is implicit.
//   - Loot drops, XP accrual, kill-tag persistence. Pure economy; not
//     a difficulty knob in the regression sense.
//   - Pardon proc / Sovereign reprieve. Default OFF in Phase 0 so the
//     baseline numbers aren't blurred by end-game safety nets.
//   - Babysit safe-rest. OFF per [[feedback_npc_buffs_are_secret]] —
//     discovery buff, not baseline difficulty.
//   - Temporal stack effects (Heat / Time Dilation). Zone-specific
//     gimmicks; Phase 3 per-zone outlier pass handles them.
//
// Encounter cadence is a Phase 0 lever: `harnessHarvestRollsPerDay`
// controls how many d20+tier interrupts a player faces per day. The
// number is a placeholder calibrated against the autopilot-harvest
// path — Phase 1's matrix will compare cadences against real player
// data and fix it down to a single value.

// ── Profile + result ──────────────────────────────────────────────────────

// expeditionBalanceProfile is one cell of the matrix: a single
// (zone, character build) pair to measure. Race fixed to Human like
// the class harness so the racial floor is neutral across cells.
type expeditionBalanceProfile struct {
	ZoneID   ZoneID
	Class    DnDClass
	Subclass DnDSubclass
	Level    int
	// Supplies the player departed with. Use makeSupplies for the
	// canonical T-appropriate 3×standard kit.
	Supplies ExpeditionSupplies
	// CampType is the camp the player establishes each night.
	// Standard is the spike default; Phase 3 may sweep this per cell.
	CampType string
	// HarvestRollsPerDay overrides the harness's default combat-interrupt
	// roll count for the daytime phase. Zero means "use the package
	// default" (harnessHarvestRollsPerDay). Phase 2's cadence
	// calibration sweep sets this per cell; everyday callers leave it
	// zero.
	HarvestRollsPerDay int
	// Phase 2 lever overrides (sweep-only). Zero means "use the live
	// shipped value." Wired into the harness day loop / runHarnessFight
	// path; the live runHarvestInterrupt is untouched. See
	// TestExpeditionBalance_Phase2_LeverSweep.
	RetreatThreatBumpOverride   int
	SurpriseNickDivisorOverride int

	// Phase 3 global-lever overrides. Zero means "use live":
	//
	//   EliteInterruptThresholdOverride — total (raw + tier + threat-mod)
	//   at which the harvest-interrupt bracket promotes from Standard
	//   to Elite. Live value is 19 (dnd_expedition_combat.go: bracket
	//   table). Higher = fewer elites; lower = more.
	//
	//   ThreatDriftBaseOverride — daily threat-clock drift base before
	//   DM-mood mod. Live value is 3 (dnd_expedition_threat.go:
	//   dailyThreatDrift). Lower = slower AC/init creep + slower
	//   bracket promotion via the +1/20-over-40 threat-mod path.
	//
	// Both wired into the harness day loop only; live callers go
	// through the shipped constants. See
	// TestExpeditionBalance_Phase3_GlobalLeverSweep.
	EliteInterruptThresholdOverride int
	ThreatDriftBaseOverride         int

	// Phase 3-B global-lever overrides. Zero means "use live":
	//
	//   SurpriseNickFloorOverride — absolute floor for the raw
	//   surprise-round nick. Live floor is the zone tier (1..5).
	//   Convention: 0 (the zero value) = "use live tier floor"; a
	//   positive value sets the floor directly; -1 disables the floor
	//   entirely (floor = 0). Lower floor = less per-fight chip damage
	//   on fresh entries.
	//
	//   SupplyBurnRatePctOverride — percent multiplier on the per-day
	//   supply burn. Zero means "use live" (100%); 50 = half burn.
	//   Lower = more supply margin for T4/T5 where starvation is the
	//   dominant failure mode post-Phase-3-A.
	//
	// Both wired into the harness day loop only; live callers go
	// through the shipped helpers. See
	// TestExpeditionBalance_Phase3B_NickSupplySweep.
	SurpriseNickFloorOverride int
	SupplyBurnRatePctOverride int

	// Phase 5-B lever — gear magic-bonus delta applied on top of the
	// live magicBonusForTier ladder. Lifts weapon.MagicBonus,
	// armor.MagicBonus, and AttackBonus by this many points at fight
	// time. Zero = live ladder.
	//
	// Phase 5-A's sensitivity sweep named player level as the dominant
	// lever at T2/T3, but the within-bracket slope showed even
	// max-of-range still misses the band — closing T2/T3 to the
	// 62-82%/55-75% targets requires a player-power lift, not just a
	// centerline shift. This knob measures the cross-zone effect of a
	// flat gear-bonus bump (the simplest player-power lever that
	// already participates in the combat math via weapon/armor
	// MagicBonus) so the shipped change can pick the smallest delta
	// that lands T1-T5 in band.
	//
	// Wired into the harness's runHarnessFight player-build site only;
	// live combat untouched. Shield MagicBonus is not bumped, mirroring
	// magicBonusForTier's "shields stay mundane" rule (classLoadout
	// comment).
	GearMagicBonusOverride int

	// Phase 5-B HP multiplier — scales the player's HPMax (and resets
	// HPCurrent to the new max at the start of each trial). Zero or
	// 1.0 = live HP curve. Lever exists because the Phase 5-B gear-
	// only sweep showed T3 and T5 have low slopes on gear delta (~5pp
	// per +1) — those tiers are HP-bound rather than to-hit/AC-bound,
	// so closing them needs a separate handle on durability.
	//
	// Wired into the harness's character-build site only; live HP
	// curve in computeMaxHP untouched.
	PlayerHPMultOverride float64
}

// expeditionTrialResult is the outcome of one simulated expedition.
// Completed and Died are mutually exclusive only when the run ended
// for a known reason — a SupplyStarvation extraction sets neither.
type expeditionTrialResult struct {
	Completed        bool
	Died             bool
	StarvedOut       bool
	DaysElapsed      int
	ThreatAtEnd      int
	CombatEncounters int
	HPRemainingPct   float64 // endHP / maxHP at termination; survivors only
	EndedReason      string  // "survived", "died_combat", "starved", "abandoned"
}

// expeditionBalanceResult is the aggregated win-rate per cell. Mirrors
// classBalanceResult — same shape, different lens.
type expeditionBalanceResult struct {
	Profile           expeditionBalanceProfile
	Trials            int
	Completions       int
	Deaths            int
	StarvedOuts       int
	MedianDays        int
	MedianThreatEnd   int
	AvgEncounters     float64
	AvgHPRemainingPct float64 // mean across survivors
}

// CompletionRate is the band-asserted headline metric for the
// per-tier target band (T1 80%, …, T5 45%, ±10pp).
func (r expeditionBalanceResult) CompletionRate() float64 {
	if r.Trials == 0 {
		return 0
	}
	return float64(r.Completions) / float64(r.Trials)
}

// DeathRate is the secondary diagnostic; useful for distinguishing
// "expedition is hard because you die" from "expedition is hard
// because you starve and forced-extract."
func (r expeditionBalanceResult) DeathRate() float64 {
	if r.Trials == 0 {
		return 0
	}
	return float64(r.Deaths) / float64(r.Trials)
}

// ── Phase 0 tunables ─────────────────────────────────────────────────────

// harnessHarvestRollsPerDay is the number of combat-interrupt rolls a
// simulated player triggers per day. Placeholder: live autopilot
// harvests are interrupt-rolled per resource action, and a typical
// active day touches 3–5 nodes. Phase 1 will compare cell numbers
// against real player traces and pick a final cadence.
const harnessHarvestRollsPerDay = 4

// harnessMaxDays caps a Phase 0 expedition at this many days. The
// spike's completion proxy is "survived to this cap"; Phase 1
// replaces the cap with a boss-kill completion path.
const harnessMaxDays = 14

// ── Expedition seed ──────────────────────────────────────────────────────

// newHarnessExpedition builds an in-memory *Expedition for the
// profile. Skips the DB insert that startExpedition does in
// production. Mirrors the field defaults of startExpedition closely
// enough that the math-pure helpers (applyDailyBurn,
// dailyThreatDrift, resolveCombatInterrupt) see the same shape.
func newHarnessExpedition(p expeditionBalanceProfile) *Expedition {
	e := &Expedition{
		ID:           "harness",
		UserID:       "harness",
		ZoneID:       p.ZoneID,
		Status:       ExpeditionStatusActive,
		CurrentDay:   1,
		Supplies:     p.Supplies,
		ThreatEvents: []ThreatEvent{},
		RegionState:  map[string]any{},
		DMMood:       50,
	}
	if p.CampType != "" {
		e.Camp = &CampState{
			Active:    true,
			Type:      p.CampType,
			RoomIndex: 0,
		}
	}
	return e
}

// ── Day advance (the seam) ───────────────────────────────────────────────

// advanceExpeditionOneDay simulates one (day, night) pair on the
// in-memory expedition + character. Returns the trial result if the
// expedition terminated this day (death, starvation, day cap); zero
// value otherwise. Caller continues advancing while the trial is
// still in flight.
//
// The pipeline mirrors deliverBriefing → autopilot harvest →
// deliverRecap order so any path-order dependencies stay observable:
//
//  1. Morning rollover: supply burn (pure), day++
//  2. Starvation check (forced extract if Current == 0)
//  3. Daily threat drift (pure)
//  4. Daytime: N harvest interrupt rolls; combat-rated brackets run
//     SimulateCombat against a zone-roster pick
//  5. Night phase: resolveWanderingCheck; ambush/elite outcome runs
//     a single fight
//  6. Day end: peg HP into the running char, check death
//
// All four "advance" functions (applyDailyBurn, dailyThreatDrift,
// resolveCombatInterrupt, resolveWanderingCheck) accept their RNG via
// the *Expedition state or an injectable rollFn; we use the latter
// where available so the seed flows through.
func (h *expeditionHarness) advanceExpeditionOneDay() expeditionTrialResult {
	zone, ok := getZone(h.exp.ZoneID)
	if !ok {
		// Unknown zone — abandon the trial with a flagged reason so
		// the matrix log surfaces the wiring bug instead of NaN-ing.
		return expeditionTrialResult{
			DaysElapsed: h.exp.CurrentDay,
			EndedReason: "unknown_zone",
		}
	}

	// 1. Morning rollover — supply burn + day++. Phase 3-B sweep can
	// scale the per-day burn via the harness profile; with no override
	// set the harness mirrors live (phase5BDailyBurnRatePct, shipped
	// as the new default in applyDailyBurn).
	harsh := h.exp.ThreatLevel > 60
	burnRate := h.supplyBurnRatePctOverride
	if burnRate == 0 {
		burnRate = phase5BDailyBurnRatePct
	}
	newSupplies, _ := applyDailyBurnP(h.exp.Supplies, harsh, h.exp.SiegeMode, burnRate)
	h.exp.Supplies = newSupplies
	h.exp.CurrentDay++

	// 2. Starvation forced-extract — matches deliverBriefing's §4.3.
	if supplyDepletion(h.exp.Supplies) == SupplyStarvation {
		return h.terminate("starved", false, false, true)
	}

	// 3. Daily threat drift — math is in dailyThreatDrift (pure);
	// applyDailyThreatDrift's DB write is bypassed but the in-memory
	// mutation is mirrored here. Phase 3 sweep can override the base
	// drift constant via the harness profile.
	if !h.exp.BossDefeated {
		delta, _ := dailyThreatDrift(h.exp.DMMood)
		if h.threatDriftBaseOverride > 0 {
			// dailyThreatDrift returns base(3) + mood-mod. Swap the
			// base while preserving the mood-mod component.
			delta = h.threatDriftBaseOverride + (delta - 3)
		}
		h.exp.ThreatLevel += delta
		if h.exp.ThreatLevel < 0 {
			h.exp.ThreatLevel = 0
		}
		if h.exp.ThreatLevel >= 100 {
			h.exp.ThreatLevel = 100
			h.exp.SiegeMode = true
		}
	}

	// 4. Daytime combat-interrupt rolls.
	for i := 0; i < h.rollsPerDay; i++ {
		kind, total := resolveCombatInterrupt(
			h.exp.ThreatLevel, int(zone.Tier), h.char.Class, h.exp.ZoneID, h.rng.d20,
		)
		// Phase 3 lever: re-bucket Standard ↔ Elite using the override
		// threshold instead of the live 19+ cutoff. Patrol (≥22) and
		// the Noise/None floor are unchanged.
		if h.eliteInterruptThresholdOverride > 0 &&
			(kind == InterruptStandard || kind == InterruptElite) &&
			total < 22 {
			if total >= h.eliteInterruptThresholdOverride {
				kind = InterruptElite
			} else if total >= 15 {
				kind = InterruptStandard
			}
		}
		switch kind {
		case InterruptNone:
			continue
		case InterruptNoise:
			// §4.2: noise bumps threat +2, no fight.
			h.exp.ThreatLevel += 2
			if h.exp.ThreatLevel > 100 {
				h.exp.ThreatLevel = 100
			}
			continue
		case InterruptStandard, InterruptElite, InterruptPatrol:
			res := h.runHarnessFight(zone, kind == InterruptElite)
			h.encounters++
			if !res.PlayerWon {
				if res.TimedOut {
					// Retreat — mirrors the live retreat semantics in
					// dnd_expedition_combat.go (Phase 2). Run continues
					// with carryover HP and a threat bump; the harvest
					// slot's loot is just forfeit.
					h.exp.ThreatLevel += h.resolvedRetreatBump()
					if h.exp.ThreatLevel > 100 {
						h.exp.ThreatLevel = 100
					}
					h.char.HPCurrent = res.PlayerEndHP
					if h.traceFight != nil {
						h.traceFight(fmt.Sprintf("retreat day=%d hp=%d threat=%d",
							h.exp.CurrentDay, h.char.HPCurrent, h.exp.ThreatLevel))
					}
					continue
				}
				return h.terminate("died_combat", false, true, false)
			}
			h.char.HPCurrent = res.PlayerEndHP
		}
	}

	// 5. Night phase — wandering check. resolveWanderingCheck reads
	// e.Camp/e.ThreatLevel and uses an injectable rollFn.
	nc := resolveWanderingCheck(h.exp, h.char.Class, h.rng.d20)
	if nc.ThreatBumped {
		h.exp.ThreatLevel += 2
		if h.exp.ThreatLevel > 100 {
			h.exp.ThreatLevel = 100
		}
	}
	switch nc.Outcome {
	case NightOutcomeStandard, NightOutcomeElite, NightOutcomeAmbush:
		// Live path defers these to !advance, where they resolve through
		// resolveCombatRoom — which ends the run on any loss because
		// the next-room path is gated on a win. Harness mirrors: a
		// failed night encounter terminates the trial.
		res := h.runHarnessFight(zone,
			nc.Outcome == NightOutcomeElite || nc.Outcome == NightOutcomeAmbush)
		h.encounters++
		if !res.PlayerWon {
			return h.terminate("died_combat", false, true, false)
		}
		h.char.HPCurrent = res.PlayerEndHP
	}

	// 6. Long rest: camped + survived → HP back to full. Production
	// puts this in processOvernightCamp, called by the next morning's
	// deliverBriefing; the harness folds it into the end-of-day
	// because we don't pend night-check encounters the way live
	// !advance does. Severe-rationing blocks the rest (§4.3).
	if h.exp.Camp != nil && h.exp.Camp.Active &&
		supplyAllowsLongRest(supplyDepletion(h.exp.Supplies)) {
		h.char.HPCurrent = h.char.HPMax
	}

	// 7. Day-cap completion proxy. Phase 1 replaces this with a real
	// boss-kill or extract decision.
	if h.exp.CurrentDay >= harnessMaxDays {
		return h.terminate("survived", true, false, false)
	}
	return expeditionTrialResult{}
}

// resolvedRetreatBump returns the per-harness lever override or the
// shipped retreatThreatBump if no override is set. Zero is the "use
// live" sentinel so the field's zero value remains safe.
func (h *expeditionHarness) resolvedRetreatBump() int {
	if h.retreatThreatBumpOverride > 0 {
		return h.retreatThreatBumpOverride
	}
	return retreatThreatBump
}

// resolvedNickDivisor returns the per-harness override or the shipped
// liveSurpriseNickDivisor. Same zero-sentinel contract as above.
func (h *expeditionHarness) resolvedNickDivisor() int {
	if h.surpriseNickDivisorOverride > 0 {
		return h.surpriseNickDivisorOverride
	}
	return liveSurpriseNickDivisor
}

// resolvedNickFloor translates the harness profile's surprise-nick
// floor override into the int contract surpriseRoundNickF expects
// (<0 = use live tier-floor, >=0 = absolute floor value). The profile
// uses 0 for "use live" so the struct's zero-value is the safe
// default; -1 in the profile means "disable floor" (floor = 0).
func (h *expeditionHarness) resolvedNickFloor() int {
	switch {
	case h.surpriseNickFloorOverride == 0:
		return -1
	case h.surpriseNickFloorOverride < 0:
		return 0
	default:
		return h.surpriseNickFloorOverride
	}
}

// terminate stamps the final trial result with shared bookkeeping
// (days elapsed, threat at end, encounter count, HP%).
func (h *expeditionHarness) terminate(reason string, completed, died, starved bool) expeditionTrialResult {
	pct := 0.0
	if h.char.HPMax > 0 {
		pct = float64(h.char.HPCurrent) / float64(h.char.HPMax)
		if pct < 0 {
			pct = 0
		}
	}
	return expeditionTrialResult{
		Completed:        completed,
		Died:             died,
		StarvedOut:       starved,
		DaysElapsed:      h.exp.CurrentDay,
		ThreatAtEnd:      h.exp.ThreatLevel,
		CombatEncounters: h.encounters,
		HPRemainingPct:   pct,
		EndedReason:      reason,
	}
}

// ── Combat fold ──────────────────────────────────────────────────────────

// runHarnessFight picks an enemy from the zone roster (using a tag-
// agnostic RNG-driven pick — the live picker is deterministic on
// (run, room), which the harness doesn't model), folds in the
// surprise-round HP nick, and runs SimulateCombat with the player
// rebuilt fresh from the class-balance loadout. HP carries over via
// h.char.HPCurrent.
func (h *expeditionHarness) runHarnessFight(zone ZoneDefinition, elite bool) CombatResult {
	monster, ok := pickHarnessZoneEnemy(zone, elite, h.rng)
	if !ok {
		// Empty roster — should never happen for live zones, but if
		// it does the trial keeps running with a no-op "fight."
		return CombatResult{PlayerWon: true, PlayerEndHP: h.char.HPCurrent, PlayerStartHP: h.char.HPMax}
	}

	// Surprise-round nick — same shape as runHarvestInterrupt's
	// pre-combat HP shave, including the wounded-entrant clamp
	// (clampSurpriseNick) that breaks the chained-interrupt death
	// spiral. Mirror live exactly so the harness measures the same
	// lever the live caller applies.
	nick := clampSurpriseNickD(
		surpriseRoundNickF(monster, int(zone.Tier), h.resolvedNickFloor()),
		h.char.HPCurrent, h.char.HPMax,
		h.resolvedNickDivisor(),
	)
	h.char.HPCurrent -= nick

	player := buildHarnessPlayer(h.char)
	// Phase 5-B: apply gear-bonus override on top of the live ladder.
	// Mirrors what bumping magicBonusForTier would do for shipped code:
	// +delta to weapon.MagicBonus (damage path reads this directly in
	// rollWeaponDamage), +delta to AttackBonus (since buildHarnessPlayer
	// already folded weapon.MagicBonus into AttackBonus at build time —
	// re-bumping the pointer alone wouldn't move to-hit), +delta to AC
	// (armor.MagicBonus contributed; shield kept mundane per the
	// classLoadout shield-stays-mundane rule).
	if h.gearMagicBonusOverride > 0 {
		delta := h.gearMagicBonusOverride
		player.Stats.AttackBonus += delta
		player.Stats.AC += delta
		if player.Stats.Weapon != nil {
			wcopy := *player.Stats.Weapon
			wcopy.MagicBonus += delta
			player.Stats.Weapon = &wcopy
		}
	}
	// Wounded entry: carry HP from prior fights via StartHP so MaxHP
	// stays the ceiling (heals respect it). combat_engine.go:348
	// reads StartHP iff 0 < StartHP < MaxHP, which is exactly our
	// case for any post-first-fight chain.
	if h.char.HPCurrent < h.char.HPMax {
		player.Stats.StartHP = h.char.HPCurrent
	}
	enemy := buildHarnessZoneEnemy(monster, int(zone.Tier))
	hpBeforeFight := h.char.HPCurrent
	result := simulateCombatWithRNG(player, enemy, dungeonCombatPhases, h.rng.r)
	if h.traceFight != nil {
		outcome := "WON"
		if !result.PlayerWon {
			outcome = "LOST"
		}
		h.traceFight(fmt.Sprintf(
			"fight day=%d zone=%s tier=%d elite=%v monster=%s hp_max=%d nick=%d hp_pre=%d hp_post=%d enemy_ac=%d enemy_atk=%d → %s",
			h.exp.CurrentDay, h.exp.ZoneID, int(zone.Tier), elite,
			monster.Name, h.char.HPMax, nick, hpBeforeFight, result.PlayerEndHP,
			enemy.Stats.AC, enemy.Stats.AttackBonus, outcome))
	}
	if h.traceFightStruct != nil {
		h.traceFightStruct(harnessFightTrace{
			Day:         h.exp.CurrentDay,
			Tier:        int(zone.Tier),
			Elite:       elite,
			MonsterName: monster.Name,
			HPMax:       h.char.HPMax,
			Nick:        nick,
			HPPre:       hpBeforeFight,
			HPPost:      result.PlayerEndHP,
			EnemyAC:     enemy.Stats.AC,
			EnemyAtk:    enemy.Stats.AttackBonus,
			Won:         result.PlayerWon,
		})
	}
	return result
}

// pickHarnessZoneEnemy is the harness's RNG-driven analogue of
// pickZoneEnemy. Live picker uses fnv-hashed (run, room) for
// determinism across re-reads; the harness doesn't model rooms and
// gets its determinism from the seeded *harnessRNG instead.
func pickHarnessZoneEnemy(zone ZoneDefinition, isElite bool, rng *harnessRNG) (DnDMonsterTemplate, bool) {
	pool := make([]ZoneEnemy, 0, len(zone.Enemies))
	if isElite {
		for _, e := range zone.Enemies {
			if e.IsElite {
				pool = append(pool, e)
			}
		}
	}
	if len(pool) == 0 {
		// Live picker's fallback: drop the elite filter rather than
		// fail. Keeps zone rosters viable when IsElite isn't set.
		for _, e := range zone.Enemies {
			if !isElite && e.IsElite {
				continue
			}
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		return DnDMonsterTemplate{}, false
	}
	total := 0
	for _, e := range pool {
		w := e.SpawnWeight
		if w <= 0 {
			w = 5
		}
		total += w
	}
	roll := rng.intN(total)
	cum := 0
	for _, e := range pool {
		w := e.SpawnWeight
		if w <= 0 {
			w = 5
		}
		cum += w
		if roll < cum {
			tmpl, ok := dndBestiary[e.BestiaryID]
			return tmpl, ok
		}
	}
	return DnDMonsterTemplate{}, false
}

// buildHarnessZoneEnemy is the harness analogue of the enemy half of
// buildZoneCombatants — applies the tier floor for AC/AttackBonus
// (only-raise-to-floor, never double-scale bosses). Skips the DM-mood
// tilt because Phase 0 keeps mood pinned at neutral 50, where the
// tilt is a no-op.
func buildHarnessZoneEnemy(monster DnDMonsterTemplate, tier int) Combatant {
	stats, mods := monster.toCombatStats()
	if tier > 1 {
		if floor := dndDungeonACBase + tier; stats.AC < floor {
			stats.AC = floor
		}
		if floor := dndDungeonAtkBase + tier; stats.AttackBonus < floor {
			stats.AttackBonus = floor
		}
	}
	return Combatant{
		Name:    monster.Name,
		Stats:   stats,
		Mods:    mods,
		Ability: monster.Ability,
	}
}

// ── Per-trial runner ─────────────────────────────────────────────────────

// expeditionHarness threads per-trial mutable state (RNG, expedition,
// character HP carryover, encounter count) through the day loop.
type expeditionHarness struct {
	exp         *Expedition
	char        *DnDCharacter
	rng         *harnessRNG
	encounters  int
	rollsPerDay int // resolved from profile + default; never zero
	// Lever overrides for the Phase 2 sweep. Zero on both fields ⇒
	// live behavior (retreatThreatBump=5, /5 surprise-nick divisor).
	retreatThreatBumpOverride   int
	surpriseNickDivisorOverride int
	// Lever overrides for the Phase 3 sweep. Zero ⇒ live behavior
	// (eliteInterruptThreshold=19, threatDriftBase=3).
	eliteInterruptThresholdOverride int
	threatDriftBaseOverride         int
	// Phase 3-B levers. See expeditionBalanceProfile field doc; both
	// zero-sentinel for "use live".
	surpriseNickFloorOverride int
	supplyBurnRatePctOverride int
	// Phase 5-B levers. See expeditionBalanceProfile field doc.
	gearMagicBonusOverride int
	playerHPMultOverride   float64
	// traceFight, if non-nil, is invoked once per fight inside
	// runHarnessFight with a human-readable summary. Used by the
	// Phase 2 lethality probe to spot whether the nick, the picked
	// monster, or the combat fold itself is driving deaths. Nil in
	// production runs — has zero cost when unused.
	traceFight func(line string)
	// traceFightStruct, if non-nil, fires alongside traceFight with the
	// same fight's data as a parsed struct. Lets aggregate diagnostics
	// (e.g. Phase 4-A's per-monster attribution) skip the fragile parse
	// of the formatted line. Retreat events are not surfaced here — the
	// retreat trace is one-line only via traceFight. Nil in production.
	traceFightStruct func(harnessFightTrace)
}

// harnessFightTrace is the structured per-fight record exposed via
// traceFightStruct. Mirrors the fields the formatted traceFight line
// already emits; if you add a field, update both paths so the human
// log and the structured aggregate stay in sync.
type harnessFightTrace struct {
	Day         int
	Tier        int
	Elite       bool
	MonsterName string
	HPMax       int
	Nick        int
	HPPre       int
	HPPost      int
	EnemyAC     int
	EnemyAtk    int
	Won         bool
}

// harnessRNG is a thin wrapper around math/rand/v2 so the d20 helper
// can be passed to resolveCombatInterrupt / resolveWanderingCheck via
// their rollFn parameters, while the same source also drives the
// roster pick. Seed flows from the trial loop.
type harnessRNG struct {
	r *rand.Rand
}

func newHarnessRNG(seed uint64) *harnessRNG {
	return &harnessRNG{r: rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))}
}

func (h *harnessRNG) d20() int { return h.r.IntN(20) + 1 }

func (h *harnessRNG) intN(n int) int {
	if n <= 0 {
		return 0
	}
	return h.r.IntN(n)
}

// runExpeditionBalanceTrial runs one full expedition end-to-end and
// returns the outcome. Seedable so a Phase 1 matrix run is fully
// reproducible.
func runExpeditionBalanceTrial(p expeditionBalanceProfile, seed uint64) expeditionTrialResult {
	exp := newHarnessExpedition(p)
	char := buildHarnessCharacter(classBalanceProfile{
		Class:    p.Class,
		Subclass: p.Subclass,
		Level:    p.Level,
	})
	rolls := p.HarvestRollsPerDay
	if rolls <= 0 {
		rolls = harnessHarvestRollsPerDay
	}
	h := &expeditionHarness{
		exp:                             exp,
		char:                            char,
		rng:                             newHarnessRNG(seed),
		rollsPerDay:                     rolls,
		retreatThreatBumpOverride:       p.RetreatThreatBumpOverride,
		surpriseNickDivisorOverride:     p.SurpriseNickDivisorOverride,
		eliteInterruptThresholdOverride: p.EliteInterruptThresholdOverride,
		threatDriftBaseOverride:         p.ThreatDriftBaseOverride,
		surpriseNickFloorOverride:       p.SurpriseNickFloorOverride,
		supplyBurnRatePctOverride:       p.SupplyBurnRatePctOverride,
		gearMagicBonusOverride:          p.GearMagicBonusOverride,
		playerHPMultOverride:            p.PlayerHPMultOverride,
	}
	// Phase 5-B HP multiplier (post-buildHarnessCharacter so we scale the
	// finalized HPMax rather than re-deriving from class HPDie/CON; less
	// invasive and easier to back out).
	if p.PlayerHPMultOverride > 0 && p.PlayerHPMultOverride != 1.0 {
		char.HPMax = int(float64(char.HPMax)*p.PlayerHPMultOverride + 0.5)
		if char.HPMax < 1 {
			char.HPMax = 1
		}
		char.HPCurrent = char.HPMax
	}
	for {
		res := h.advanceExpeditionOneDay()
		if res.EndedReason != "" {
			return res
		}
	}
}

// runExpeditionBalanceCell runs N trials of one cell and aggregates.
// Mirrors runClassBalanceCell.
func runExpeditionBalanceCell(p expeditionBalanceProfile, trials int, baseSeed uint64) expeditionBalanceResult {
	r := expeditionBalanceResult{Profile: p, Trials: trials}
	days := make([]int, 0, trials)
	threats := make([]int, 0, trials)
	var hpSum float64
	var hpN int
	var encSum float64
	for i := 0; i < trials; i++ {
		tr := runExpeditionBalanceTrial(p, baseSeed+uint64(i))
		days = append(days, tr.DaysElapsed)
		threats = append(threats, tr.ThreatAtEnd)
		encSum += float64(tr.CombatEncounters)
		if tr.Completed {
			r.Completions++
		}
		if tr.Died {
			r.Deaths++
		}
		if tr.StarvedOut {
			r.StarvedOuts++
		}
		if tr.Completed {
			hpSum += tr.HPRemainingPct
			hpN++
		}
	}
	sort.Ints(days)
	sort.Ints(threats)
	if len(days) > 0 {
		r.MedianDays = days[len(days)/2]
		r.MedianThreatEnd = threats[len(threats)/2]
	}
	if trials > 0 {
		r.AvgEncounters = encSum / float64(trials)
	}
	if hpN > 0 {
		r.AvgHPRemainingPct = hpSum / float64(hpN)
	}
	return r
}
