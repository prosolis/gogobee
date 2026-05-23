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

	preDmgBefore := playerMods.SpellPreDamage

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

	if playerMods.SpellPreDamage > preDmgBefore {
		applyMageSubclassSpellHooks(c, spell, pc.SlotLevel, playerMods)
	}

	c.PendingCast = ""
	if err := SaveDnDCharacter(c); err != nil {
		slog.Error("dnd: clear pending_cast failed", "user", userID, "err", err)
	}
}

// applyMageSubclassSpellHooks layers the Mage subclass damage/healing
// adjustments on top of a freshly-resolved spell cast. Caller passes mods
// only after the spell actually dealt damage so misses don't trigger
// Empowered Evocation or Grim Harvest stash.
func applyMageSubclassSpellHooks(c *DnDCharacter, spell SpellDefinition, slotLevel int, mods *CombatModifiers) {
	if c == nil || c.Class != ClassMage {
		return
	}
	switch c.Subclass {
	case SubclassEvocation:
		// L7 Empowered Evocation: +INT mod (min 1) to one Mage evocation
		// spell's damage per turn. We're one-shot, so "per turn" collapses
		// to "this cast".
		if c.Level >= 7 && spell.School == "evocation" {
			bonus := abilityModifier(c.INT)
			if bonus < 1 {
				bonus = 1
			}
			mods.SpellPreDamage += bonus
		}
		// Phase 10 SUB3b-i — L10 Overchannel: maximize damage dice on a
		// 1st–5th-level spell. We don't track per-die rolls here (damage was
		// rolled in the apply* path above), so we proxy max-vs-avg as +50%
		// to SpellPreDamage on a leveled cast. 5e gates this to "1st–5th
		// level" — slot 6+ wouldn't get max dice — and includes a 2d12
		// necrotic self-damage drawback per cast above the first per long
		// rest. Self-damage is omitted here: in our model the Mage queues
		// at most one cast per fight, so the "first per rest" exemption
		// almost always applies. L15 Sculptural Mastery is AoE-friendly-fire
		// only — skipped (same reason SUB2b skipped Sculpt Spells).
		if c.Level >= 10 && slotLevel >= 1 && slotLevel <= 5 {
			mods.SpellPreDamage = (mods.SpellPreDamage * 3) / 2
		}
	case SubclassNecromancy:
		// L5 Grim Harvest stash — heal applied post-combat iff this spell
		// landed the killing blow. Cantrip kills count as L1 for the heal
		// formula; necrotic damage triples the multiplier (per design doc).
		if c.Level >= 5 {
			slot := slotLevel
			if spell.Level == 0 {
				slot = 1
			}
			mods.GrimHarvestSlot = slot
			mods.GrimHarvestNecrotic = spell.DamageType == "necrotic"
		}
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
// Half damage on success, full on fail.
//
// Single-target limitation: Phase 9 combat features one enemy, so AoE-flagged
// spells (Fireball, Burning Hands, Cone of Cold, Flame Strike, etc.) collapse
// to a single-target damage roll. The damage formula is the same as the
// single-target case — the AoE flag is preserved on SpellDefinition for
// future multi-enemy combat (Phase 11+) but is not consulted here.
func applySpellDamageSave(spell SpellDefinition, dc int, c *DnDCharacter, mods *CombatModifiers, enemy *CombatStats, slot int) {
	saveMod := enemySpellSaveMod(enemy)
	saveRoll := 1 + rand.IntN(20)
	saved := saveRoll+saveMod >= dc
	dmg := rollSpellDamageDice(spell, slot, c.Level)
	if saved {
		dmg = halveSavedDamage(dmg)
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

// halveSavedDamage applies the save-half rule. Integer division floors,
// then a 1-damage floor ensures even a successful save against any spell
// chips at least 1 HP. This matches the original applySpellDamageSave
// inline branch behavior; pulled out so rounding is unit-testable.
func halveSavedDamage(dmg int) int {
	halved := dmg / 2
	if halved < 1 {
		return 1
	}
	return halved
}

// enemySpellSaveMod is the heuristic save modifier for an arena/dungeon
// enemy. We don't track per-stat saves on monsters, so we approximate with
// half their attack bonus — that scales linearly with threat tier (T1 ≈ +2,
// T5 ≈ +5..7) and keeps the save vs. caster DC math meaningful at both ends.
func enemySpellSaveMod(enemy *CombatStats) int {
	if enemy == nil {
		return 0
	}
	return enemy.AttackBonus / 2
}

// applySpellControl — Hold Person, Sleep, Command, Hypnotic Pattern.
// Enemy save vs DC; failure → skip first attack. Hold-family also primes
// the engine's auto-crit-on-first-hit path so melee strikes connect for
// double damage (5e: paralyzed creatures auto-crit on melee hits).
func applySpellControl(spell SpellDefinition, dc int, mods *CombatModifiers, enemy *CombatStats, slot int) {
	saveMod := enemySpellSaveMod(enemy)
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
		// Spectral bonus-action attack each round on its own channel so the
		// narration doesn't borrow pet flavor (the cleric may have no pet).
		// 1d8 + spell mod base, +1d8 per 2 slot levels above 2nd; the engine
		// rolls the d5 variance, so Dmg carries the average + upcast bump.
		base := 4 + spellAttackBonus(c)
		if slot > 2 {
			base += 4 * ((slot - 2) / 2)
		}
		if base < 4 {
			base = 4
		}
		if mods.SpiritWeaponProc < 0.5 {
			mods.SpiritWeaponProc = 0.5
		}
		if mods.SpiritWeaponDmg < base {
			mods.SpiritWeaponDmg = base
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

// ── Phase 13 — per-round turn-based casting ──────────────────────────────────

// turnSpellOutcome is the resolved effect of a spell cast as a player turn in a
// turn-based fight. Damage, healing, and the one-round enemy skip all land
// within the casting round, so no cross-round persistence is needed.
type turnSpellOutcome struct {
	EnemyDamage int
	PlayerHeal  int
	EnemySkip   bool
	Desc        string
}

// resolveTurnSpell resolves a spell as a turn-based player action, reusing the
// auto-resolve spell math against a throwaway modifier set. supported is false
// for buff and utility spells: those mutate stats for the rest of the fight,
// which the turn engine can't carry yet (it rebuilds combatants each round) —
// the caller should refuse them without spending the slot. Reaction spells are
// likewise out of scope here and should be filtered by the caller.
func resolveTurnSpell(c *DnDCharacter, spell SpellDefinition, slotLevel int, enemy *CombatStats) (out turnSpellOutcome, supported bool) {
	switch spell.Effect {
	case EffectSpellHeal:
		out.PlayerHeal = rollTurnSpellHeal(c, spell, slotLevel)
		out.Desc = fmt.Sprintf("%s — +%d HP", spell.Name, out.PlayerHeal)
		return out, true
	case EffectDamageAttack, EffectDamageSave, EffectDamageAuto, EffectControl:
		var mods CombatModifiers
		dc := spellSaveDC(c)
		atk := spellAttackBonus(c)
		switch spell.Effect {
		case EffectDamageAttack:
			applySpellDamageAttack(spell, atk, &mods, enemy, slotLevel, c.Level)
		case EffectDamageSave:
			applySpellDamageSave(spell, dc, c, &mods, enemy, slotLevel)
		case EffectDamageAuto:
			applySpellDamageAuto(spell, &mods, slotLevel, c.Level)
		case EffectControl:
			applySpellControl(spell, dc, &mods, enemy, slotLevel)
		}
		if mods.SpellPreDamage > 0 {
			applyMageSubclassSpellHooks(c, spell, slotLevel, &mods)
		}
		out.EnemyDamage = mods.SpellPreDamage
		out.EnemySkip = mods.SpellEnemySkipFirst
		out.Desc = mods.SpellPreDamageDesc
		if out.Desc == "" {
			out.Desc = spell.Name
		}
		return out, true
	default:
		// EffectBuffSelf / EffectBuffAlly / EffectUtility — deferred.
		return out, false
	}
}

// rollTurnSpellHeal rolls an in-combat healing cast. Mirrors
// resolveHealOutOfCombat's formula but returns the amount instead of writing
// to the character sheet — in a turn-based fight, HP lives on the session row.
func rollTurnSpellHeal(c *DnDCharacter, spell SpellDefinition, slotLevel int) int {
	dice, faces, _ := parseDamageDice(spell.DamageDice)
	if dice == 0 {
		dice, faces = 1, 8
	}
	extra := slotLevel - spell.Level
	if extra < 0 {
		extra = 0
	}
	totalDice := dice + extra
	supreme := lifeDomainSupremeHealing(c)
	heal := 0
	for i := 0; i < totalDice; i++ {
		if supreme {
			heal += faces
		} else {
			heal += 1 + rand.IntN(faces)
		}
	}
	heal += abilityModifier(c.WIS)
	heal += lifeDomainHealBonus(c, spell, slotLevel)
	if heal < 1 {
		heal = 1
	}
	return heal
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

// ── Phase 13 — turn-based buff resolution ────────────────────────────────────

// turnBuffDelta is the marginal effect of one buff spell or consumable applied
// as a turn-based player action, diffed against the player's already-buffed
// combatant. Stat components (dAC, dDmgBonus, …) accumulate as session deltas;
// depleting resources (ward, spore, …) add to their running counters; heal and
// enemySkip land within the casting round only.
type turnBuffDelta struct {
	dAC, dAtk, dSpeed, dPetDmg int
	dCrit, dDmgBonus, dPetProc float64
	dReductMul                 float64 // multiplicative; 1 = no change
	ward, spore, arcaneWard    int
	reflect                    float64
	autoCrit, enemySkip        bool
	heal                       int // a MaxHP-raise (Aid) collapses to an immediate heal
	dSpiritProc                float64
	dSpiritDmg                 int
}

// statComponent reports whether the buff has a re-applicable persistent stat
// effect — the part applySessionBuffs folds back onto the player every round.
func (d turnBuffDelta) statComponent() bool {
	return d.dAC != 0 || d.dAtk != 0 || d.dSpeed != 0 || d.dPetDmg != 0 ||
		d.dCrit != 0 || d.dDmgBonus != 0 || d.dPetProc != 0 ||
		d.dSpiritProc != 0 || d.dSpiritDmg != 0 ||
		(d.dReductMul > 0 && d.dReductMul != 1)
}

// any reports whether the buff produced any applicable effect at all. A buff
// the turn engine can't represent yet (pure utility) diffs to nothing, and the
// caller refuses it before a slot or item is spent.
func (d turnBuffDelta) any() bool {
	return d.statComponent() || d.ward != 0 || d.spore != 0 || d.reflect != 0 ||
		d.autoCrit || d.arcaneWard != 0 || d.enemySkip || d.heal != 0
}

// diffTurnBuff computes the marginal effect of a buff: (bs,bm) is the player's
// state before the buff, (as,am) the throwaway result of applying it. Reusing
// applySpellBuff / ApplyConsumableMods against a copy keeps turn-based buffs
// numerically identical to the auto-resolve engine.
func diffTurnBuff(bs, as CombatStats, bm, am CombatModifiers) turnBuffDelta {
	d := turnBuffDelta{
		dAC:        as.AC - bs.AC,
		dAtk:       as.AttackBonus - bs.AttackBonus,
		dSpeed:     as.Speed - bs.Speed,
		dPetDmg:    am.PetAttackDmg - bm.PetAttackDmg,
		dCrit:      as.CritRate - bs.CritRate,
		dDmgBonus:  am.DamageBonus - bm.DamageBonus,
		dPetProc:   am.PetAttackProc - bm.PetAttackProc,
		ward:       am.WardCharges - bm.WardCharges,
		spore:      am.SporeCloud - bm.SporeCloud,
		reflect:    am.ReflectNext - bm.ReflectNext,
		arcaneWard: am.ArcaneWardHP - bm.ArcaneWardHP,
		autoCrit:   am.AutoCritFirst && !bm.AutoCritFirst,
		enemySkip:  am.SpellEnemySkipFirst && !bm.SpellEnemySkipFirst,
		heal:       as.MaxHP - bs.MaxHP,
		dSpiritProc: am.SpiritWeaponProc - bm.SpiritWeaponProc,
		dSpiritDmg:  am.SpiritWeaponDmg - bm.SpiritWeaponDmg,
	}
	if bm.DamageReduct > 0 {
		d.dReductMul = am.DamageReduct / bm.DamageReduct
	} else {
		d.dReductMul = 1
	}
	return d
}
