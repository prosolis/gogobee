package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// Out-of-combat ally targeting for `!cast` — the other half of §1.
//
// splitCastTarget (combat_cmd.go) resolves a target against the people *in the
// fight*. Out of combat there is no fight, so the equivalent set is the people
// on your expedition: the party is who you are standing next to. Healing a
// stranger across the world is not a thing a cleric at camp can do, and scoping
// it to the party keeps the two `!cast` paths answering the same question.
//
// Only heals may name somebody else. Everything else `!cast` can do out of
// combat either resolves on the caster (UTILITY) or queues as a PendingCast for
// **the caster's** next fight — an ally target on those would be silently
// dropped, which is how `--target` came to be swallowed in the first place.

// splitOutOfCombatTarget peels an ally target off a `!cast` argument, using the
// same two spellings the combat path accepts: the `--target @alex` flag, and a
// plain trailing `@alex`. The bare form requires the `@` here (unlike in combat,
// where the roster disambiguates) so a trailing spell word is never mistaken for
// a name.
//
// Returns (remainingArgs, name). name is empty when no target was named.
func splitOutOfCombatTarget(args string) (string, string) {
	fields := strings.Fields(args)
	for i := 0; i < len(fields); i++ {
		if !strings.EqualFold(fields[i], "--target") {
			continue
		}
		if i+1 >= len(fields) {
			return args, ""
		}
		name := strings.TrimPrefix(fields[i+1], "@")
		fields = append(fields[:i], fields[i+2:]...)
		return strings.Join(fields, " "), name
	}
	if len(fields) == 0 {
		return args, ""
	}
	if last := fields[len(fields)-1]; strings.HasPrefix(last, "@") {
		if name := strings.TrimPrefix(last, "@"); name != "" {
			return strings.Join(fields[:len(fields)-1], " "), name
		}
	}
	return args, ""
}

// resolveCastTargetOnExpedition maps a named target to a human on the caster's
// active expedition. errMsg is non-empty and player-facing on any failure; a
// caster who names somebody must never fall through to healing themselves,
// because that quietly burns the slot on the wrong body.
func resolveCastTargetOnExpedition(caster id.UserID, name string) (id.UserID, string) {
	exp, _, err := activeExpeditionFor(caster)
	if err != nil {
		return "", "Couldn't look up your expedition."
	}
	if exp == nil {
		return "", "You're not travelling with anyone. `!cast` on yourself, or join a party first."
	}
	seats, err := partyHumans(exp.ID, exp.UserID)
	if err != nil {
		return "", "Couldn't look up your party."
	}
	for _, s := range seats {
		uid := string(s.UserID)
		if strings.EqualFold(uid, name) || strings.EqualFold(id.UserID(uid).Localpart(), name) {
			if s.UserID == caster {
				// Naming yourself is just casting it on yourself.
				return "", ""
			}
			return s.UserID, ""
		}
		// Character name, which is what players actually call each other.
		if n := charName(s.UserID); n != "" && strings.EqualFold(n, name) {
			if s.UserID == caster {
				return "", ""
			}
			return s.UserID, ""
		}
	}
	if strings.EqualFold(name, companionDisplayName) {
		return "", companionDisplayName + " patches himself up at camp. Save the slot."
	}
	return "", fmt.Sprintf("**%s** isn't on your expedition. You can only heal someone you're travelling with.", name)
}

// errHealTargetFull / errHealTargetDown are the two ways an ally heal declines
// to spend the slot. Both are recoverable: the caller refunds.
var (
	errHealTargetFull = errors.New("target already at full HP")
	errHealTargetDown = errors.New("target is down")
)

// healPartyMember applies a heal to somebody else's sheet.
//
// It does NOT take the target's advUserLock. Gifting sets the precedent
// (adventure_gifting.go): mutate the other player's row with one guarded
// statement rather than a read-modify-write under two locks. Two clerics healing
// each other at the same instant would otherwise take those locks in opposite
// orders and deadlock the pair of them.
//
// The clamp lives in SQL for the same reason — read, add, cap and write are one
// statement, so a heal landing between a concurrent heal's read and write cannot
// push anybody past their maximum.
func healPartyMember(target id.UserID, amount int) (before, after, maxHP int, err error) {
	tx, err := db.Get().Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRow(
		`SELECT hp_current, hp_max FROM dnd_character WHERE user_id = ?`,
		string(target)).Scan(&before, &maxHP)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, 0, 0, err
	}
	switch {
	case before <= 0:
		// A heal is not a resurrection — the same rule the combat path holds.
		return before, before, maxHP, errHealTargetDown
	case before >= maxHP:
		return before, before, maxHP, errHealTargetFull
	}

	if _, err = tx.Exec(
		`UPDATE dnd_character
		    SET hp_current = MIN(hp_max, hp_current + ?),
		        updated_at = CURRENT_TIMESTAMP
		  WHERE user_id = ? AND hp_current > 0`,
		amount, string(target)); err != nil {
		return before, before, maxHP, err
	}
	if err = tx.QueryRow(
		`SELECT hp_current FROM dnd_character WHERE user_id = ?`,
		string(target)).Scan(&after); err != nil {
		return before, before, maxHP, err
	}
	if err = tx.Commit(); err != nil {
		return before, before, maxHP, err
	}
	return before, after, maxHP, nil
}
