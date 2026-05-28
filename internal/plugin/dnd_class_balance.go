package plugin

import (
	"math/rand/v2"
	"sort"
)

// Measurement harness for the class-balance pass (gogobee_class_balance.md).
// Phase 0 introduced this for a Fighter-vs-Mage spike; Phase 1 extended it
// to drive the full 10-class × 30-subclass matrix (subclass==""  at L1–L4,
// each of a class's three subclasses at the L5/L7/L10/L15/L20 checkpoints).
//
// Sibling to dnd_race_balance.go — same spirit, different method. Races
// don't fight, so race balance had to use a hand-weighted scoring proxy.
// Classes do fight: combat collapses to a single seedable call into the
// engine, so class balance is *measured*, not modeled. This file is the
// measurement harness.
//
// Scope here is Phase 0 only: build a synthetic Fighter and Mage at a
// handful of levels, layer equipment and a queued spell the same way live
// combat does, run N fights per dungeon tier, read the win rates. The
// goal is to sanity-check the two policies the doc flags in §3 — the
// equipment loadout and the spell-selection heuristic — before Phase 1
// generalizes to all 10 classes × 30 subclasses.
//
// Bypassed deliberately (Phase 0 simplifying constraints, doc §2):
//
//   - DB-touching layers: applyMagicItemEffects, applyArmedAbility, and
//     the SaveDnDCharacter inside applyPendingCast. The harness is pure
//     Go; tests run without a sqlite instance.
//   - Race passives beyond Human (+1 all): neutral baseline, again per §2.
//   - Inventory consumables: empty.
//
// Everything else flows through the production code paths
// (applyDnDPlayerLayer, applyClassPassives, applyRacePassives, the
// equipment-derived AC/weapon-dice resolution inside SimulateCombat) so
// numbers from this harness are directly comparable to live fights at
// the same character level.

// ── Build profile ────────────────────────────────────────────────────────────

// classBalanceProfile is one row of the matrix: a single class build at a
// single level. Race is fixed to Human (the +1-to-all neutral baseline
// shipped in the race-balance pass) so class numbers aren't skewed by
// racial mods.
type classBalanceProfile struct {
	Class    DnDClass
	Subclass DnDSubclass // empty below L5, per doc §2
	Level    int
}

// classBalanceResult is the empirical performance of one profile against
// one dungeon tier. WinRate is the headline number; the rest are
// diagnostics logged but not asserted on (per doc §4).
type classBalanceResult struct {
	Profile           classBalanceProfile
	Tier              int
	Trials            int
	Wins              int
	AvgHPRemainingPct float64 // mean of endHP/MaxHP across won trials; 0 if no wins
	NearDeathRate     float64 // fraction of trials flagged NearDeath
}

// WinRate is the cell value the doc's parity rule asserts on once we get
// to Phase 2 tuning.
func (r classBalanceResult) WinRate() float64 {
	if r.Trials == 0 {
		return 0
	}
	return float64(r.Wins) / float64(r.Trials)
}

// ── Equipment loadout policy (doc §3.1) ──────────────────────────────────────
//
// One of the two policies Phase 0 exists to de-risk. The kit must be
// standardized fairly across classes — otherwise downstream win rates
// reflect the kit, not the class. The mapping below treats character
// level as a proxy for "what tier of gear would a player at this level
// realistically be holding," using the dungeon tier MinLevel ladder
// (advDungeons in adventure_activities.go) as the reference.
//
//	L1–L4   → T1 mundane kit (no magic bonus)
//	L5–L8   → T2, +1 weapon and armor
//	L9–L12  → T3, +2
//	L13–L16 → T4, +3
//	L17–L20 → T5, +3 (cap — the appendix doesn't go higher)
//
// Per-class kit choice tracks the character's primary attack stat: STR
// martials wear heavy armor and swing martial-melee; DEX skirmishers
// take light armor and a finesse weapon; casters keep a quarterstaff
// and rely on Mage Armor / class AC floors instead of armor proficiency.

// gearTier maps a character level to a 1..5 magic/quality tier for the
// loadout policy. Kept private and tunable in one place.
func gearTier(level int) int {
	switch {
	case level >= 17:
		return 5
	case level >= 13:
		return 4
	case level >= 9:
		return 3
	case level >= 5:
		return 2
	}
	return 1
}

// magicBonusForTier is the +X enchantment we hand the player at this tier.
// T1 is mundane; the +1/+2/+3 ladder mirrors gogobee_equipment_appendix.md
// §7 magic-weapon tiers.
func magicBonusForTier(tier int) int {
	switch tier {
	case 1:
		return 0
	case 2:
		return 1
	case 3:
		return 2
	default:
		return 3
	}
}

// classLoadout is the standardized weapon + armor + shield kit a class
// fights with at this level. Returning copies so the caller can mutate
// MagicBonus without poisoning the registry. armor or shield may be nil.
func classLoadout(class DnDClass, level int) (weapon *WeaponProfile, armor *ArmorProfile, shield *ArmorProfile) {
	tier := gearTier(level)
	mb := magicBonusForTier(tier)

	weaponID, armorID, useShield := classLoadoutIDs(class)
	if w := weaponByID(weaponID); w != nil {
		copy := *w
		copy.MagicBonus = mb
		weapon = &copy
	}
	if armorID != "" {
		if a := armorByID(armorID); a != nil {
			copy := *a
			copy.MagicBonus = mb
			armor = &copy
		}
	}
	if useShield {
		if s := armorByID("arm_shield"); s != nil {
			copy := *s
			// Shields don't get the weapon-tier enchantment in this kit;
			// keeping them mundane avoids double-counting the +X.
			shield = &copy
		}
	}
	return
}

// classLoadoutIDs picks the canonical weapon / armor / shield set per
// class. Choices follow the class's PrimaryA stat and 5e proficiency
// expectations — Fighter swings martial melee in heavy armor; Mage stays
// behind a quarterstaff and lets Mage Armor / DEX carry AC.
func classLoadoutIDs(class DnDClass) (weapon, armor string, shield bool) {
	switch class {
	case ClassFighter, ClassPaladin:
		return "wpn_longsword", "arm_chain_mail", true
	case ClassRanger:
		// DEX skirmisher with a finesse-friendly bow; light armor, no
		// shield (two-handed bow occupies the off-hand anyway).
		return "wpn_longbow", "arm_studded", false
	case ClassRogue:
		return "wpn_shortsword", "arm_studded", false
	case ClassCleric:
		return "wpn_mace", "arm_chain_shirt", true
	case ClassDruid:
		// Druids canonically eschew metal armor; hide is the SRD default.
		return "wpn_scimitar", "arm_hide", false
	case ClassBard:
		return "wpn_rapier", "arm_leather", false
	case ClassMage, ClassSorcerer, ClassWarlock:
		// No armor proficiency. The Mage's class AC floor + DEX + a future
		// queued Mage Armor cast carry survival. Quarterstaff is the
		// canonical caster sidearm.
		return "wpn_quarterstaff", "", false
	}
	return "wpn_club", "", false
}

// ── Spell-selection policy (doc §3.2) ────────────────────────────────────────
//
// The other Phase 0 policy. Without a "what would a caster cast here"
// heuristic, the 8 caster classes fight as naked weapon-users — a
// measurement artifact, not real imbalance.
//
// Phase 0 simplification: pick the single best damage spell from the
// class's available spells (level ≤ highest slot the build owns). "Best"
// is the expected damage of one cast under generous assumptions — avg
// dice × cantrip/upcast scaling — ignoring hit chance and save-half
// (those would require knowing the target's AC/save, which we don't have
// at selection time). This gets the Mage casting Magic Missile / Fireball
// instead of Fire Bolt's weaker auto-damage, which is the whole point.
//
// Phase 1 will refine this: per-fight slot bookkeeping for multi-round
// fights, fight-context selection (control vs. damage), buff pre-casts.

// pickBestDamageSpell returns the spell a caster of this class+level
// would queue for one fight, plus the slot level to upcast at. Returns
// (zero, 0, false) for non-casters and classes with no damage spells.
func pickBestDamageSpell(c *DnDCharacter) (SpellDefinition, int, bool) {
	if !classIsCaster(c.Class) {
		return SpellDefinition{}, 0, false
	}
	slots := slotsForClassLevel(c.Class, c.Level)
	maxSlot := 0
	for lvl := range slots {
		if slots[lvl] > 0 && lvl > maxSlot {
			maxSlot = lvl
		}
	}
	candidates := spellsForClass(c.Class, maxSlot)
	var best SpellDefinition
	var bestSlot int
	bestScore := -1.0
	for _, s := range candidates {
		switch s.Effect {
		case EffectDamageAttack, EffectDamageSave, EffectDamageAuto:
		default:
			continue
		}
		// Cantrip → always castable, no slot cost.
		// Leveled → upcast to maxSlot when we own a slot ≥ spell level.
		slot := s.Level
		if s.Level == 0 {
			slot = 0
		} else if slots[s.Level] == 0 && s.Level > 0 {
			continue
		} else if maxSlot > s.Level {
			slot = maxSlot
		}
		score := spellExpectedDamage(s, slot, c.Level)
		if score > bestScore {
			bestScore = score
			best = s
			bestSlot = slot
		}
	}
	if bestScore < 0 {
		return SpellDefinition{}, 0, false
	}
	return best, bestSlot, true
}

// spellExpectedDamage estimates the average raw damage of one cast — dice
// count × avg-face + flat, with cantrip/upcast scaling identical to
// rollSpellDamageDice. No hit-chance or save-half weighting (see policy
// note above). Magic Missile's auto-damage path gets a small explicit
// bonus to reflect that it never misses; the auto-damage flag alone
// already steers picks correctly in practice.
func spellExpectedDamage(s SpellDefinition, slot, charLevel int) float64 {
	dice, faces, flat := parseDamageDice(s.DamageDice)
	if dice == 0 || faces == 0 {
		return 0
	}
	if s.Level == 0 {
		switch {
		case charLevel >= 17:
			dice *= 4
		case charLevel >= 11:
			dice *= 3
		case charLevel >= 5:
			dice *= 2
		}
	} else if extra := slot - s.Level; extra > 0 {
		dice += extra
	}
	avgFace := (float64(faces) + 1) / 2
	avg := float64(dice)*avgFace + float64(flat)
	// Concentration damage spells (heat_metal, spirit_guardians,
	// flaming_sphere, call_lightning, spike_growth, cloud_of_daggers, …)
	// re-tick each round while concentration holds. Without this factor
	// the picker scores them as one-shots and they lose to higher-tier
	// blasts on every comparison. Conservative ×3 = roughly the median
	// fight length the picker can hope to keep concentration up; tier-
	// scaled would be more correct but adds noise here.
	// EffectDamageAttack is excluded — single-target attack-roll spells
	// aren't generally concentration; the rare ones (hex-style) get
	// their lift from mods, not this score.
	if s.Concentration && (s.Effect == EffectDamageSave || s.Effect == EffectDamageAuto) {
		avg *= 3
	}
	return avg
}

// applyHarnessSpellCast is the DB-free version of applyPendingCast: same
// damage resolution, no SaveDnDCharacter. Mirrors the live path's choice
// of attack/save/auto handlers so the Mage's contribution to a fight is
// the same shape as it would be in production.
func applyHarnessSpellCast(
	c *DnDCharacter,
	spell SpellDefinition,
	slot int,
	playerStats *CombatStats,
	playerMods *CombatModifiers,
	enemyStats *CombatStats,
) {
	dc := spellSaveDC(c)
	atk := spellAttackBonus(c)
	preDmgBefore := playerMods.SpellPreDamage
	switch spell.Effect {
	case EffectDamageAttack:
		applySpellDamageAttack(spell, atk, playerMods, enemyStats, slot, c.Level)
	case EffectDamageSave:
		applySpellDamageSave(spell, dc, c, playerMods, enemyStats, slot)
	case EffectDamageAuto:
		applySpellDamageAuto(spell, playerMods, slot, c.Level)
	}
	if playerMods.SpellPreDamage > preDmgBefore && c.Class == ClassMage {
		// Mage evocation/necromancy hooks live in the live spell-combat
		// path; we never set a subclass in Phase 0, so the call is a
		// no-op today but keeps shape parity for Phase 1.
		applyMageSubclassSpellHooks(c, spell, slot, playerMods)
	}
}

// ── Synthesizing the build ───────────────────────────────────────────────────

// buildHarnessCharacter constructs the DnDCharacter for one profile. Uses
// the class's stat priority + Human's +1-to-all racial mods, then derives
// HP and the baseline AC from the class.
func buildHarnessCharacter(p classBalanceProfile) *DnDCharacter {
	scores := classStatPriority(p.Class)
	scores = applyRaceMods(RaceHuman, scores)
	c := &DnDCharacter{
		Race:     RaceHuman,
		Class:    p.Class,
		Subclass: p.Subclass,
		Level:    p.Level,
		STR:      scores[0], DEX: scores[1], CON: scores[2],
		INT: scores[3], WIS: scores[4], CHA: scores[5],
	}
	conMod := abilityModifier(c.CON)
	dexMod := abilityModifier(c.DEX)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, dexMod)
	return c
}

// buildHarnessPlayer assembles the Combatant the engine will fight with.
// Layers the same calls runDungeonCombat makes, in order, minus the
// DB-touching ones (per the file header). Returns the Combatant; the
// caller decides whether to queue a spell on top.
func buildHarnessPlayer(c *DnDCharacter) Combatant {
	stats := CombatStats{}
	mods := CombatModifiers{DamageReduct: 1.0}

	// 1. Player layer (HP/AC/AttackBonus from the sheet).
	applyDnDPlayerLayer(&stats, c)

	// 2. Equipment layer — inlined from applyDnDEquipmentLayer to avoid
	// the AdvEquipment synthesis chain. Same net effect on stats.
	weapon, armor, shield := classLoadout(c.Class, c.Level)
	if weapon != nil {
		stats.Weapon = weapon
		stats.AbilityModForDamage = pickWeaponAbilityMod(weapon, c)
		stats.WeaponProficient = dndClassWeaponProficiency(c.Class, weapon)
		stats.AttackBonus += weapon.MagicBonus
		if weapon.HasProperty(PropTwoHanded) || (weapon.HasProperty(PropVersatile) && shield == nil) {
			stats.TwoHandedMode = true
		}
	}
	// Two-handed weapons forbid shields (appendix §5.4).
	if weapon != nil && weapon.HasProperty(PropTwoHanded) {
		shield = nil
	}
	if armor != nil || shield != nil {
		stats.AC = computeArmorAC(armor, shield, abilityModifier(c.DEX))
	}

	// Phase 5-B player power floor. applyDnDEquipmentLayer applies this
	// at the same point in the live combat path — keep the harness's
	// measurement aligned with what live players experience by calling
	// the same helper here, before passives stack on top.
	applyPhase5BPlayerFloor(&stats)

	// 3. Passives. Live order is class → race → subclass (see
	// combat_bridge.go and combat_session_build.go). Subclass passives are
	// a no-op when c.Subclass == "" — the harness uses that for the L1–L4
	// pre-unlock rows.
	applyClassPassives(&stats, &mods, c)
	applyRacePassives(&stats, &mods, c)
	applySubclassPassives(&stats, &mods, c)

	return Combatant{
		Name:     string(c.Class),
		Stats:    stats,
		Mods:     mods,
		IsPlayer: true,
	}
}

// buildHarnessEnemy mirrors runDungeonCombat's enemy assembly: the
// per-tier stat curve from DeriveDungeonMonsterStats, then the d20
// AC/AttackBonus overlay from applyDnDDungeonMonsterLayer. No
// MonsterAbility — Phase 0 measures the base case.
func buildHarnessEnemy(tier int) Combatant {
	loc := dungeonLocForTier(tier)
	stats, mods := DeriveDungeonMonsterStats(loc)
	applyDnDDungeonMonsterLayer(&stats, tier)
	return Combatant{Name: loc.Denizens, Stats: stats, Mods: mods}
}

// dungeonLocForTier returns the canonical advDungeons row for a tier.
// Falls back to T1 for out-of-range input.
func dungeonLocForTier(tier int) *AdvLocation {
	for i := range advDungeons {
		if advDungeons[i].Tier == tier {
			return &advDungeons[i]
		}
	}
	return &advDungeons[0]
}

// ── Monte Carlo runner ───────────────────────────────────────────────────────

// runClassBalanceTrial runs one fight: build player + enemy fresh, queue
// the caster's best spell if applicable, simulate, return the result.
// Each trial constructs fresh combatants so the per-fight RNG (rand.IntN
// in spell rolls + the engine's package-global rand) drives variance.
func runClassBalanceTrial(p classBalanceProfile, tier int) CombatResult {
	c := buildHarnessCharacter(p)
	player := buildHarnessPlayer(c)
	enemy := buildHarnessEnemy(tier)
	if spell, slot, ok := pickBestDamageSpell(c); ok {
		applyHarnessSpellCast(c, spell, slot, &player.Stats, &player.Mods, &enemy.Stats)
	}
	return SimulateCombat(player, enemy, dungeonCombatPhases)
}

// runClassBalanceCell is one cell of the matrix: N trials of (profile,
// tier). Returns aggregated win rate + diagnostics.
func runClassBalanceCell(p classBalanceProfile, tier, trials int) classBalanceResult {
	r := classBalanceResult{Profile: p, Tier: tier, Trials: trials}
	var hpSum float64
	var hpWonTrials int
	for i := 0; i < trials; i++ {
		res := runClassBalanceTrial(p, tier)
		if res.PlayerWon {
			r.Wins++
			if res.PlayerStartHP > 0 {
				hpSum += float64(res.PlayerEndHP) / float64(res.PlayerStartHP)
				hpWonTrials++
			}
		}
		if res.NearDeath {
			r.NearDeathRate++
		}
	}
	if hpWonTrials > 0 {
		r.AvgHPRemainingPct = hpSum / float64(hpWonTrials)
	}
	if trials > 0 {
		r.NearDeathRate /= float64(trials)
	}
	return r
}

// runClassBalanceMatrix sweeps a list of profiles across the full T1..T5
// dungeon ladder. Returns results sorted by (Tier asc, Class, Level) for
// deterministic test output.
func runClassBalanceMatrix(profiles []classBalanceProfile, trials int) []classBalanceResult {
	tiers := []int{1, 2, 3, 4, 5}
	out := make([]classBalanceResult, 0, len(profiles)*len(tiers))
	for _, p := range profiles {
		for _, t := range tiers {
			out = append(out, runClassBalanceCell(p, t, trials))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return out[i].Tier < out[j].Tier
		}
		if out[i].Profile.Class != out[j].Profile.Class {
			return out[i].Profile.Class < out[j].Profile.Class
		}
		return out[i].Profile.Level < out[j].Profile.Level
	})
	return out
}

// ── Phase 1 matrix builder ───────────────────────────────────────────────────

// phase1SubclassLevels is the post-unlock checkpoint ladder from doc §2.
// L5/L7/L10/L15/L20 line up with the subclass tier-unlock structure in
// dnd_subclass_combat.go — each row reads a class's behaviour at one more
// unlocked tier than the row above it.
var phase1SubclassLevels = []int{5, 7, 10, 15, 20}

// phase1PreSubclassLevels is the L1–L4 ladder run with Subclass=="". Doc §2
// notes that subclasses aren't selected until L5, so these rows measure the
// raw class chassis.
var phase1PreSubclassLevels = []int{1, 2, 3, 4}

// buildPhase1Profiles assembles the full Phase 1 build matrix: every class
// at L1–L4 (no subclass), then each of that class's three subclasses at
// each of the five tier-unlock checkpoints. 10 × 4 + 10 × 3 × 5 = 190 rows.
// Order is registry order (dndClasses, then subclassesForClass) so the
// matrix log reads the same way as the design doc and the !class help.
func buildPhase1Profiles() []classBalanceProfile {
	out := make([]classBalanceProfile, 0, 10*4+10*3*5)
	for _, ci := range dndClasses {
		for _, lvl := range phase1PreSubclassLevels {
			out = append(out, classBalanceProfile{Class: ci.Key, Level: lvl})
		}
		for _, si := range subclassesForClass(ci.Key) {
			for _, lvl := range phase1SubclassLevels {
				out = append(out, classBalanceProfile{
					Class:    ci.Key,
					Subclass: si.ID,
					Level:    lvl,
				})
			}
		}
	}
	return out
}

// _ keeps the math/rand/v2 import live in case future iterations of this
// file want to draw directly (e.g. for harness-level RNG control). Today
// every randomized step is inside production helpers.
var _ = rand.IntN
