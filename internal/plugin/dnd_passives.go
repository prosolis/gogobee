package plugin

// Phase 3 — class passives applied via CombatModifiers.
//
// Phase 3 keeps abilities passive-only: they auto-trigger via the existing
// CombatModifiers machinery rather than being player-activated mid-fight
// (the combat engine is one-shot, no turn-based UI). Active/reactive
// abilities arrive once we have a model for them — likely tied to !arena
// pre-arming or to a turn-based PvP variant.

// ── Class passive definitions ────────────────────────────────────────────────

// DnDClassAbility is the lore/UI representation of a class's signature
// passive. Used by !abilities and !sheet for display; mechanics live in
// applyClassPassives below.
type DnDClassAbility struct {
	Name        string
	Description string
}

var dndClassAbilities = map[DnDClass]DnDClassAbility{
	ClassFighter: {
		Name:        "Battle Trained",
		Description: "Years of weapon drill add +5% to all damage you deal.",
	},
	ClassRogue: {
		Name:        "Sneak Attack",
		Description: "Your first strike each combat lands as a critical hit, doubling its damage; precision drills add +5% to all damage you deal.",
	},
	ClassMage: {
		Name:        "Arcane Focus",
		Description: "Practiced channeling adds +1 to your attack rolls and a small bump (+5%) to all damage you deal.",
	},
	ClassCleric: {
		Name:        "Divine Favor",
		Description: "When you fall below half HP, divine intervention restores 5 HP. Once per combat.",
	},
	ClassRanger: {
		Name:        "Hunter's Mark",
		Description: "You read your prey's weak points: +5% damage and +1 to attack rolls.",
	},
	// Open5e caster scaffold — one signature passive each, riding existing
	// CombatModifiers channels the same way the original five do.
	ClassDruid: {
		Name:        "Wild Resilience",
		Description: "The wild lends you its toughness — incoming damage is reduced 5% — and a thorn surge on engage scaled by your Wisdom.",
	},
	ClassBard: {
		Name:        "Bardic Inspiration",
		Description: "Quick wit keeps you a step ahead: +1 initiative, +1 attack, and +5% to all damage you deal.",
	},
	ClassSorcerer: {
		Name:        "Innate Sorcery",
		Description: "Raw magic spills out as the fight begins, dealing immediate damage scaled by your Charisma, and +5% to all damage you deal.",
	},
	ClassWarlock: {
		Name:        "Agonizing Blast",
		Description: "Your pact-fueled eldritch power adds +12% to all damage you deal and +1 to attack rolls.",
	},
	ClassPaladin: {
		Name:        "Divine Smite",
		Description: "You channel a burst of radiant power on engagement, dealing flat damage that grows with your level.",
	},
}

// clampNonNeg returns max(0, x). Used by Phase-2 class passives to keep
// ability-mod-scaled FlatDmgStart additions from going negative when a
// caster's casting stat happens to be below 10 (or when tests construct
// a DnDCharacter with zero-value stats).
func clampNonNeg(x int) int {
	if x < 0 {
		return 0
	}
	return x
}

// applyRacePassives sets the combat-impacting flags from the player's race.
// Elf "trance" (resilience flavor), Half-Elf "adaptable" (cross-cultural know-
// how), and the Human floating +1 are non-combat or stat-only and have no
// engine hook here.
func applyRacePassives(stats *CombatStats, mods *CombatModifiers, c *DnDCharacter) {
	switch c.Race {
	case RaceHalfling:
		mods.LuckyReroll = true
	case RaceOrc:
		mods.RageReady = true
	case RaceDwarf:
		mods.PoisonResist = true
	case RaceTiefling:
		// Fiendish heritage: incoming fire damage is halved by the combat
		// engine (enemy attack path + aoe_fire ability) and by the trap
		// resolver (fire-tagged traps).
		mods.FireResist = true
	}
}

// applyClassPassives mutates a player's CombatModifiers to apply their
// class passive. Called after applyDnDPlayerLayer + DerivePlayerStats but
// BEFORE consumable application (so consumables can stack on top).
//
// Some passives ride on existing CombatModifiers fields:
//   Rogue's Sneak Attack reuses AutoCritFirst (the consumable Crystal Berry
//   field) — the engine already implements first-hit-auto-crit semantics.
//   Cleric's Divine Favor reuses HealItem, the under-50%-HP heal trigger.
//
// This means:
//   - A Rogue carrying a Crystal Berry doesn't get *two* auto-crits;
//     AutoCritFirst is already a one-shot bool.
//   - A Cleric carrying a healing potion stacks: passive 5 + potion 8 = 13.
//     The passive heal triggers first since both use the same threshold.
func applyClassPassives(stats *CombatStats, mods *CombatModifiers, c *DnDCharacter) {
	switch c.Class {
	case ClassFighter:
		mods.DamageBonus += 0.05
	case ClassRogue:
		mods.AutoCritFirst = true
		// Phase 2 class-balance: rogue's once-per-fight auto-crit goes stale
		// at high tiers (T5 mean trails leaders by ~10pp pre-tune). Add a
		// modest steady-DPS rider so post-opener rounds aren't pure attrition.
		mods.DamageBonus += 0.05
	case ClassMage:
		stats.AttackBonus++
		// Phase 2 class-balance: +1 attack alone left Mage mid-pack on damage
		// per round. A modest damage rider lifts weapon hits (DamageBonus does
		// not multiply queued SpellPreDamage — that path is its own field).
		mods.DamageBonus += 0.05
		// Phase 2 class-balance: Arcane focus also produces a small pre-combat
		// arcane burst, scaling with level and INT. Helps the L1-4 chassis,
		// which would otherwise rely on a single weak L1 spell + quarterstaff
		// against T2-T3 monsters. Saturates harmlessly at L10+ where every
		// class wins anyway.
		mods.FlatDmgStart += c.Level + clampNonNeg(abilityModifier(c.INT))
	case ClassCleric:
		// Passive heal at <50% HP. Stacks additively with consumable HealItem.
		mods.HealItem += 5
	case ClassRanger:
		mods.DamageBonus += 0.05
		stats.AttackBonus++
	case ClassDruid:
		// Wild Resilience — multiplicative, so it stacks cleanly with the
		// subclass DamageReduct riders. DamageReduct is initialized to 1.0
		// by DerivePlayerStats before passives run.
		mods.DamageReduct *= 0.95
		// Phase 3 class-balance: druid was the only caster chassis with a
		// purely defensive passive, and the off-tier numbers showed it —
		// L1/T2 mean 0.04 vs Mage 0.27. Mirror the other caster bursts so
		// the L1-4 druid can chip a T2 monster: WIS-scaled FlatDmgStart on
		// engage. No DamageBonus rider; the chassis keeps its defensive
		// identity, and the burst alone (plus the existing DR) lifts the
		// off-tier cells without pushing in-tier wins past 1.0.
		mods.FlatDmgStart += c.Level + clampNonNeg(abilityModifier(c.WIS))
	case ClassBard:
		// Phase 2 class-balance: bare +1 initiative left Bard the weakest
		// class chassis (T5 mean 0.48 pre-tune). Add a Ranger-tier rider
		// (+1 attack, +5% damage) so the chassis pulls weight before subclass
		// kicks in at L5, plus a CHA-scaled opening flourish (FlatDmgStart) so
		// the L1-4 chassis isn't dead at T3.
		mods.InitiativeBias += 1
		stats.AttackBonus++
		mods.DamageBonus += 0.05
		mods.FlatDmgStart += c.Level + clampNonNeg(abilityModifier(c.CHA))
	case ClassSorcerer:
		// Innate Sorcery — pre-combat burst, CHA-scaled like the Sorcerer's
		// spellcasting stat. Floors at the flat base for low-CHA builds.
		// Phase 2 class-balance: pure FlatDmgStart faded at higher tiers as
		// monster HP grew. Adding a 5% damage rider plus level scaling on the
		// burst keeps the chassis relevant past L1.
		// Phase 3 class-balance: sorcerer was the second-worst off-tier caster
		// (L1/T2 0.10 pre-tune), trailing mage by ~17pp despite a comparable
		// stat line. Bump the burst base 3→5 to lift the L1-4 chassis without
		// touching the +0.05 rider that already saturates at high tier.
		mods.FlatDmgStart += 5 + c.Level + clampNonNeg(abilityModifier(c.CHA))
		mods.DamageBonus += 0.05
	case ClassWarlock:
		// Phase 2 class-balance: bumped from 10% to 12% damage + 1 attack —
		// the Warlock chassis read mid-pack at T5 (0.52) pre-tune. Eldritch
		// blast as an opener (FlatDmgStart, level + CHA-scaled) covers the
		// caster's L1-4 quarterstaff weakness, same shape as the other three
		// arcane chassis.
		mods.DamageBonus += 0.12
		stats.AttackBonus++
		mods.FlatDmgStart += c.Level + clampNonNeg(abilityModifier(c.CHA))
	case ClassPaladin:
		// Divine Smite — radiant burst on engage, scaling with level so it
		// stays relevant against tougher foes.
		mods.FlatDmgStart += 4 + c.Level/2
	}
}
