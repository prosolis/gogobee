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
		Description: "Your first strike each combat lands as a critical hit, doubling its damage.",
	},
	ClassMage: {
		Name:        "Arcane Focus",
		Description: "Practiced channeling adds +1 to your attack rolls.",
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
		Description: "The wild lends you its toughness: incoming damage is reduced 5%.",
	},
	ClassBard: {
		Name:        "Bardic Inspiration",
		Description: "Quick wit keeps you a step ahead: +1 to your initiative each round.",
	},
	ClassSorcerer: {
		Name:        "Innate Sorcery",
		Description: "Raw magic spills out as the fight begins, dealing immediate damage scaled by your Charisma.",
	},
	ClassWarlock: {
		Name:        "Agonizing Blast",
		Description: "Your pact-fueled eldritch power adds +10% to all damage you deal.",
	},
	ClassPaladin: {
		Name:        "Divine Smite",
		Description: "You channel a burst of radiant power on engagement, dealing flat damage that grows with your level.",
	},
}

// applyRacePassives sets the combat-impacting flags from the player's race.
// Races whose passives apply to skill checks or non-combat scenarios
// (Tiefling fire resist, Elf sleep immunity, Half-Elf bonus profs, Human
// floating +1) are handled in their respective phases and have no combat
// hook in Phase 3.
func applyRacePassives(stats *CombatStats, mods *CombatModifiers, c *DnDCharacter) {
	switch c.Race {
	case RaceHalfling:
		mods.LuckyReroll = true
	case RaceOrc:
		mods.RageReady = true
	case RaceDwarf:
		mods.PoisonResist = true
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
	case ClassMage:
		stats.AttackBonus++
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
	case ClassBard:
		mods.InitiativeBias += 1
	case ClassSorcerer:
		// Innate Sorcery — pre-combat burst, CHA-scaled like the Sorcerer's
		// spellcasting stat. Floors at the flat 3 for low-CHA builds.
		mods.FlatDmgStart += 3 + abilityModifier(c.CHA)
	case ClassWarlock:
		mods.DamageBonus += 0.10
	case ClassPaladin:
		// Divine Smite — radiant burst on engage, scaling with level so it
		// stays relevant against tougher foes.
		mods.FlatDmgStart += 4 + c.Level/2
	}
}
