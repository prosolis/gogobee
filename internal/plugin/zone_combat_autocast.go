package plugin

import (
	"maunium.net/go/mautrix/id"
)

// §6 — a caster casts in an auto-resolved fight.
//
// An ordinary room fight is runZoneCombatRoster → SimulateCombat on an 8-round
// clock: one breath, no turn engine, no action picker. The only spell that has
// ever landed there is a PendingCast the player queued BY HAND with `!cast`
// before walking in. Under autopilot nobody queues one — so for every room of
// every expedition, a caster fought with a weapon and nothing else.
//
// A cleric is the extreme case: no Extra Attack, a mace, and its whole kit
// unusable. Measured on the room path at L10, a wounded cleric loses 8% of
// ORDINARY room fights (a fighter loses 0.0%), and a room loss ends the entire
// expedition — "the fight drags on, X outlasts you, you retreat". That is why
// cleric fled 167 of 500 runs in the corpus while fighter, ranger and paladin
// fled none, and it is the whole of the "cleric is a weak class" gap.
//
// The tell: dnd_class_balance.go — the harness the class corpus was tuned on —
// ALWAYS hands a caster their best damage spell (pickBestDamageSpell +
// applyHarnessSpellCast) before it simulates. So the numbers we balanced against
// modelled a caster who casts, and the live room modelled one who does not. This
// closes that gap: the character fights with the kit it actually owns.
//
// It is NOT a new mechanic. A folded-in spell is additive pre-damage, exactly as
// a hand-queued PendingCast has always been, which is what keeps this comparable
// to the corpus rather than a fresh invention.

// autoCastForAutoResolve picks the caster's best damage spell, spends the slot,
// and folds the damage into the Combatant pair. Reports whether it cast.
//
// It reads the seat's ACTUAL remaining slots, not the class slot table.
// pickBestDamageSpell (the harness one) reads slotsForClassLevel — the
// theoretical pool — which is right for a one-fight harness and would be an
// infinite spell here: the same "no row to persist onto, so it arrives fresh"
// bug that gave the companion an unlimited body and an unlimited slot pool.
// A leveled spell cast here costs a real slot and stays spent until a rest.
func (p *AdventurePlugin) autoCastForAutoResolve(
	uid id.UserID,
	c *DnDCharacter,
	playerStats *CombatStats,
	playerMods *CombatModifiers,
	enemyStats *CombatStats,
) bool {
	// A hand-queued spell wins: the player already chose, applyPendingCast owns
	// it, and casting a second one would be two spells in one action.
	if c == nil || c.PendingCast != "" || !isSpellcaster(c) {
		return false
	}
	spell, slot, ok := pickAutoResolveSpell(uid, c)
	if !ok {
		return false
	}
	// Leveled spells cost a slot. Spend it BEFORE the damage lands, and bail if
	// the debit fails — a spell that could not be paid for must not be cast.
	if slot > 0 {
		spent, err := consumeSpellSlot(uid, slot)
		if err != nil || !spent {
			return false
		}
	}
	applyHarnessSpellCast(c, spell, slot, playerStats, playerMods, enemyStats)
	return true
}

// pickAutoResolveSpell is the best damage spell this caster can actually cast
// right now: known, prepared, on their list, and backed by a slot they still
// hold.
//
// It casts at the spell's NATIVE level and never upcasts. simPickSpell upcasts
// aggressively — correct there, because it runs in the turn engine at an elite
// or a boss, which is what the big slots are for. A trash room is not, and a
// picker that nukes a goblin with a 5th-level slot would leave the caster
// swinging a stick at the thing that actually matters.
func pickAutoResolveSpell(uid id.UserID, c *DnDCharacter) (SpellDefinition, int, bool) {
	known, err := listKnownSpells(uid)
	if err != nil || len(known) == 0 {
		return SpellDefinition{}, 0, false
	}
	slots, err := getSpellSlots(uid)
	if err != nil {
		return SpellDefinition{}, 0, false
	}

	var best SpellDefinition
	bestSlot := 0
	bestScore := -1.0
	for _, k := range known {
		if !k.Prepared {
			continue
		}
		sp, ok := lookupSpell(k.SpellID)
		if !ok {
			continue
		}
		switch sp.Effect {
		case EffectDamageAttack, EffectDamageSave, EffectDamageAuto:
		default:
			continue
		}
		// The auto-resolve engine has no reaction window, same as everywhere else.
		if sp.CastTime == CastReaction {
			continue
		}
		onList := false
		for _, cl := range sp.Classes {
			if cl == c.Class {
				onList = true
				break
			}
		}
		if !onList {
			continue
		}
		// A cantrip is free; a leveled spell needs a slot still in hand.
		if sp.Level > 0 {
			if pair, ok := slots[sp.Level]; !ok || pair[0]-pair[1] <= 0 {
				continue
			}
		}
		if score := spellExpectedDamage(sp, sp.Level, c.Level); score > bestScore {
			best, bestSlot, bestScore = sp, sp.Level, score
		}
	}
	return best, bestSlot, bestScore >= 0
}
