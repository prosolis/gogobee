package plugin

import (
	"maunium.net/go/mautrix/id"
)

// Seat-scoped spellbook.
//
// Every spell lookup in the engine — known list, prepared flag, slot pool, slot
// spend — is keyed on a Matrix user id and answered by a `dnd_*` table. That was
// fine while every combatant was a player. The hired companion is not: he has no
// row in dnd_character, dnd_known_spells or dnd_spell_slots, deliberately, because
// a sheet on disk for him is the thing that would turn him into a real character
// everywhere (player_meta, the leaderboard, !stats). His sheet is synthesized per
// fight and thrown away.
//
// So every one of those lookups returned "nothing" for him, and the picker's very
// first line — `c, _ := LoadDnDCharacter(uid); if c == nil { return "attack" }` —
// sent him to swing a mace, every turn, forever. A hired Cleric could not heal.
// Role-fill hands a lone fighter a Cleric, so that was the *common* case.
//
// These are the seat-scoped forms of those lookups. A human seat delegates to the
// DB functions verbatim — same queries, same order, so solo combat and the balance
// corpus are untouched. A companion seat is answered from his in-memory sheet and
// a slot ledger on his seat's persisted statuses.
//
// Nothing here may write a row for the companion. That invariant is what
// TestCompanion_SheetIsBelowMedianAndNeverPersisted pins, and the auto-migration
// inside ensureCharForDnDCmd would violate it silently: handed a user with no
// sheet, it *builds one at level 1 and saves it*. Hence seatCastSheet.

// seatCompanionLoadout returns the class and level a companion seat fights as,
// and whether the seat is the companion at all.
func seatCompanionLoadout(sess *CombatSession, uid id.UserID) (DnDClass, int, bool) {
	if sess == nil || !isCompanionSeat(uid) {
		return "", 0, false
	}
	class, level := companionLoadoutForRun(sess.RunID)
	return class, level, true
}

// seatCastSheet resolves the character behind a seat for the !cast path.
//
// A human goes through ensureCharForDnDCmd, exactly as before — including its
// auto-migration for a player who predates Adv 2.0. The companion must NOT: that
// migration would mint and persist a level-1 dnd_character row for him, quietly
// making him a player and throwing away the class and level he was hired at. He
// gets his synthetic sheet instead, built from the same class-priority pipeline a
// real character of his level uses.
func (p *AdventurePlugin) seatCastSheet(sess *CombatSession, uid id.UserID) (*DnDCharacter, error) {
	if class, level, ok := seatCompanionLoadout(sess, uid); ok {
		return companionSheet(class, level), nil
	}
	advChar, _ := loadAdvCharacter(uid)
	return p.ensureCharForDnDCmd(uid, advChar)
}

// seatPickSheet is seatCastSheet for the auto-picker, which reads a sheet to
// decide a turn and must never create one. Humans get LoadDnDCharacter, which is
// what the picker has always called; a miss returns nil and the caller swings.
func seatPickSheet(sess *CombatSession, uid id.UserID) *DnDCharacter {
	if class, level, ok := seatCompanionLoadout(sess, uid); ok {
		return companionSheet(class, level)
	}
	c, _ := LoadDnDCharacter(uid)
	return c
}

// companionKnownSpells is the companion's spell list: the same default kit
// ensureSpellsForCharacter grants a real character of that class and level, every
// entry prepared (which is also what that function does — preparation is SP4 and
// until it lands every granted spell is auto-prepared so !cast works).
//
// He gets the player kit on purpose. His below-median comes from the level penalty,
// the never-Masterwork gear and the absent subclass/magic items — the layers a
// player accumulates. Handing him a bespoke, weaker spell list would be a second
// nerf hidden in a different file, and it would drift away from the tuned list the
// moment anyone touched one and not the other.
func companionKnownSpells(class DnDClass, level int) []knownSpellRow {
	ids := defaultKnownSpells(class, level)
	out := make([]knownSpellRow, 0, len(ids))
	for _, sid := range ids {
		out = append(out, knownSpellRow{SpellID: sid, Source: "companion", Prepared: true})
	}
	return out
}

// companionSlotPool is his slot table (total, used) — the class/level progression
// every caster shares, less what he has already spent. `used` is the expedition's
// ledger, NOT a per-fight one: he rations one pool across the run and gets it back
// at camp, exactly as a human caster does.
func companionSlotPool(class DnDClass, level int, used [6]int) map[int][2]int {
	out := map[int][2]int{}
	for lvl, total := range slotsForClassLevel(class, level) {
		spent := 0
		if lvl >= 0 && lvl < len(used) {
			spent = min(used[lvl], total)
		}
		out[lvl] = [2]int{total, spent}
	}
	return out
}

// seatKnownSpells is listKnownSpells for a seat.
func seatKnownSpells(sess *CombatSession, seat int, uid id.UserID) ([]knownSpellRow, error) {
	if class, level, ok := seatCompanionLoadout(sess, uid); ok {
		return companionKnownSpells(class, level), nil
	}
	return listKnownSpells(uid)
}

// seatSpellSlots is getSpellSlots for a seat.
func seatSpellSlots(sess *CombatSession, seat int, uid id.UserID) (map[int][2]int, error) {
	if class, level, ok := seatCompanionLoadout(sess, uid); ok {
		return companionSlotPool(class, level, companionSlotsForRun(sess.RunID)), nil
	}
	return getSpellSlots(uid)
}

// seatKnowsSpell is playerKnowsSpell for a seat: (known, prepared, err).
func seatKnowsSpell(sess *CombatSession, seat int, uid id.UserID, spellID string) (bool, bool, error) {
	if _, _, ok := seatCompanionLoadout(sess, uid); ok {
		known, err := seatKnownSpells(sess, seat, uid)
		if err != nil {
			return false, false, err
		}
		for _, k := range known {
			if k.SpellID == spellID {
				return true, k.Prepared, nil
			}
		}
		return false, false, nil
	}
	return playerKnowsSpell(uid, spellID)
}

// consumeSeatSlot is consumeSpellSlot for a seat. The companion's spend lands on
// the expedition's ledger — one pool for the whole run, refilled at camp — so he
// rations his slots the way a human caster has to. Keying it by expedition also
// keeps two parties who have each hired him from sharing a pool, which anything
// keyed on his (single, shared) user id would have done.
func consumeSeatSlot(sess *CombatSession, seat int, uid id.UserID, slotLevel int) (bool, error) {
	class, level, ok := seatCompanionLoadout(sess, uid)
	if !ok {
		return consumeSpellSlot(uid, slotLevel)
	}
	used := companionSlotsForRun(sess.RunID)
	if slotLevel < 1 || slotLevel >= len(used) {
		return false, nil
	}
	pair, exists := companionSlotPool(class, level, used)[slotLevel]
	if !exists || pair[0]-pair[1] <= 0 {
		return false, nil
	}
	used[slotLevel]++
	if err := setCompanionSlotsForRun(sess.RunID, used); err != nil {
		return false, err
	}
	return true, nil
}

// refundSeatSlot is refundSpellSlot for a seat: the rollback half of the above,
// called when the round the slot was spent on failed to resolve.
func refundSeatSlot(sess *CombatSession, seat int, uid id.UserID, slotLevel int) error {
	if _, _, ok := seatCompanionLoadout(sess, uid); !ok {
		return refundSpellSlot(uid, slotLevel)
	}
	used := companionSlotsForRun(sess.RunID)
	if slotLevel < 1 || slotLevel >= len(used) || used[slotLevel] <= 0 {
		return nil
	}
	used[slotLevel]--
	return setCompanionSlotsForRun(sess.RunID, used)
}
