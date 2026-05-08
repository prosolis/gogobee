package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"

	"maunium.net/go/mautrix/id"
)

// Phase 9 SP3 — combat-time resolution of pending spell casts.
//
// applyPendingCast consumes c.PendingCast (queued by !cast in dnd_cast.go)
// and folds the spell's effect into playerStats / playerMods / enemyStats
// before SimulateCombat begins. Damage spells emit a "spell_cast" pre-combat
// event via Mods.SpellPreDamage{,Desc}; control spells set
// Mods.SpellEnemySkipFirst (and AutoCritFirst on Hold Person family); buffs
// mutate stats directly.
//
// Concentration is left untouched — it persists across fights until the
// player drops it manually or casts a replacing concentration spell. The
// concentration-on-damage-break check is deferred to Phase 11 (turn-based
// boss combat).
//
// Pending cast is consumed regardless of outcome (a wasted cantrip miss is
// still a cantrip; a wasted slot-spell miss already debited the slot at
// !cast time).

func applyPendingCast(
	userID id.UserID,
	c *DnDCharacter,
	playerStats *CombatStats,
	playerMods *CombatModifiers,
	enemyStats *CombatStats,
) {
	if c == nil || c.PendingCast == "" {
		return
	}
	pc, ok := decodePendingCast(c.PendingCast)
	if !ok {
		c.PendingCast = ""
		_ = SaveDnDCharacter(c)
		return
	}
	spell, ok := lookupSpell(pc.SpellID)
	if !ok {
		c.PendingCast = ""
		_ = SaveDnDCharacter(c)
		return
	}

	dc := spellSaveDC(c)
	atk := spellAttackBonus(c)

	switch spell.Effect {
	case EffectDamageAttack:
		applySpellDamageAttack(spell, atk, playerMods, enemyStats, pc.SlotLevel, c.Level)
	case EffectDamageSave:
		applySpellDamageSave(spell, dc, c, playerMods, enemyStats, pc.SlotLevel)
	case EffectDamageAuto:
		applySpellDamageAuto(spell, playerMods, pc.SlotLevel, c.Level)
	case EffectControl:
		applySpellControl(spell, dc, playerMods, enemyStats, pc.SlotLevel)
	case EffectBuffSelf, EffectBuffAlly:
		applySpellBuff(spell, c, playerStats, playerMods, pc.SlotLevel)
	}

	c.PendingCast = ""
	if err := SaveDnDCharacter(c); err != nil {
		slog.Error("dnd: clear pending_cast failed", "user", userID, "err", err)
	}
}

// applySpellDamageAttack — Fire Bolt, Inflict Wounds, Chill Touch, etc.
// Roll d20 + spell attack vs enemy AC; nat 20 doubles dice damage.
func applySpellDamageAttack(spell SpellDefinition, atk int, mods *CombatModifiers, enemy *CombatStats, slot, charLevel int) {
	roll := 1 + rand.IntN(20)
	isCrit := roll == 20
	isFumble := roll == 1
	if isFumble || (!isCrit && roll+atk < enemy.AC) {
		mods.SpellPreDamageDesc = spell.Name + " — missed"
		return
	}
	dmg := rollSpellDamageDice(spell, slot, charLevel)
	if isCrit {
		dmg *= 2
	}
	mods.SpellPreDamage += dmg
	if isCrit {
		mods.SpellPreDamageDesc = spell.Name + " — crit!"
	} else {
		mods.SpellPreDamageDesc = spell.Name
	}
}

// applySpellDamageSave — Burning Hands, Fireball, Sacred Flame, etc.
// Enemy rolls a save (heuristic mod = enemy.AttackBonus / 2) vs spell DC.
// Half damage on success, full on fail. AoE flag is moot — we have one enemy.
func applySpellDamageSave(spell SpellDefinition, dc int, c *DnDCharacter, mods *CombatModifiers, enemy *CombatStats, slot int) {
	saveMod := enemy.AttackBonus / 2
	saveRoll := 1 + rand.IntN(20)
	saved := saveRoll+saveMod >= dc
	dmg := rollSpellDamageDice(spell, slot, c.Level)
	if saved {
		dmg /= 2
		if dmg < 1 {
			dmg = 1
		}
	}
	mods.SpellPreDamage += dmg
	if saved {
		mods.SpellPreDamageDesc = fmt.Sprintf("%s — saved (half, %d dmg)", spell.Name, dmg)
	} else {
		mods.SpellPreDamageDesc = fmt.Sprintf("%s — %d dmg", spell.Name, dmg)
	}
}

// applySpellDamageAuto — Magic Missile and other no-roll damage.
// Magic Missile: 3 darts × 1d4+1, +1 dart per slot above 1st.
func applySpellDamageAuto(spell SpellDefinition, mods *CombatModifiers, slot, charLevel int) {
	if spell.ID == "magic_missile" {
		darts := 3
		if slot > 1 {
			darts += slot - 1
		}
		total := 0
		for i := 0; i < darts; i++ {
			total += 1 + rand.IntN(4) + 1
		}
		mods.SpellPreDamage += total
		mods.SpellPreDamageDesc = fmt.Sprintf("Magic Missile (%d darts, %d dmg)", darts, total)
		return
	}
	// Fallback: roll listed dice + ability mod isn't standard for auto spells.
	dmg := rollSpellDamageDice(spell, slot, charLevel)
	mods.SpellPreDamage += dmg
	mods.SpellPreDamageDesc = fmt.Sprintf("%s — %d dmg", spell.Name, dmg)
}

// applySpellControl — Hold Person, Sleep, Command, Hypnotic Pattern.
// Enemy save vs DC; failure → skip first attack. Hold-family also primes
// the engine's auto-crit-on-first-hit path so melee strikes connect for
// double damage (5e: paralyzed creatures auto-crit on melee hits).
func applySpellControl(spell SpellDefinition, dc int, mods *CombatModifiers, enemy *CombatStats, slot int) {
	saveMod := enemy.AttackBonus / 2
	saveRoll := 1 + rand.IntN(20)
	if saveRoll+saveMod >= dc {
		mods.SpellPreDamageDesc = spell.Name + " — resisted"
		return
	}
	mods.SpellEnemySkipFirst = true
	switch spell.ID {
	case "hold_person", "hold_monster":
		mods.AutoCritFirst = true
		mods.SpellPreDamageDesc = spell.Name + " — paralyzed!"
	case "sleep":
		mods.SpellPreDamageDesc = spell.Name + " — asleep"
	default:
		mods.SpellPreDamageDesc = spell.Name + " — controlled"
	}
}

// applySpellBuff — folds known buff spells into stats/mods. Unknown buff
// IDs still emit a "spell active" beat so the cast registers in narrative.
func applySpellBuff(spell SpellDefinition, c *DnDCharacter, stats *CombatStats, mods *CombatModifiers, slot int) {
	switch spell.ID {
	case "mage_armor":
		// AC = 13 + DEX, take whichever is higher.
		newAC := 13 + abilityModifier(c.DEX)
		if newAC > stats.AC {
			stats.AC = newAC
		}
	case "shield_of_faith":
		stats.AC += 2
	case "barkskin":
		if stats.AC < 16 {
			stats.AC = 16
		}
	case "bless":
		stats.AttackBonus += 2 // 1d4 average ≈ 2.5
	case "guiding_bolt":
		stats.AttackBonus += 2 // proxy for next-attack advantage
	case "hunters_mark":
		mods.DamageBonus += 0.15 // ~+1d6 per hit on a typical baseline
	case "aid":
		stats.MaxHP += 5
	case "shillelagh":
		stats.AttackBonus += 1
		mods.DamageBonus += 0.05
	case "spiritual_weapon":
		// Spectral bonus-action attack each round. Reuse pet-attack channel.
		if mods.PetAttackProc < 0.5 {
			mods.PetAttackProc = 0.5
		}
		if mods.PetAttackDmg < 6 {
			mods.PetAttackDmg = 6
		}
	case "mirror_image":
		mods.WardCharges += 2
	case "blur":
		mods.SporeCloud += 3 // proxy: enemy miss chance per round
	case "greater_invisibility":
		mods.SporeCloud += 5
		stats.AttackBonus += 2
	case "ensnaring_strike":
		mods.SpellEnemySkipFirst = true
	}
	if mods.SpellPreDamageDesc == "" {
		mods.SpellPreDamageDesc = spell.Name + " — active"
	}
}

// rollSpellDamageDice rolls a spell's damage dice with cantrip and upcast
// scaling. Returns at least 1 if the spell has any dice; 0 if no dice are
// listed (utility/buff spells).
func rollSpellDamageDice(spell SpellDefinition, slot, charLevel int) int {
	dice, faces, flat := parseDamageDice(spell.DamageDice)
	if dice == 0 || faces == 0 {
		return 0
	}
	if spell.Level == 0 {
		// Cantrip scaling: 5e tiered at L5, L11, L17.
		switch {
		case charLevel >= 17:
			dice *= 4
		case charLevel >= 11:
			dice *= 3
		case charLevel >= 5:
			dice *= 2
		}
	} else if extra := slot - spell.Level; extra > 0 {
		// Upcast: +1 die per slot above base. Approximation for the common
		// "+1d6 per slot above 3rd" pattern; close enough for engine balance.
		dice += extra
	}
	total := flat
	for i := 0; i < dice; i++ {
		total += 1 + rand.IntN(faces)
	}
	if total < 1 {
		total = 1
	}
	return total
}
