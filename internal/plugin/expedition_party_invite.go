package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// N3/P6b — the invite handshake.
//
// A party forms in the gap between "I started an expedition" and "we set off",
// and that gap is short: the autopilot leaves a fresh expedition alone for
// autoRunMinExpeditionAge (30 minutes) and then starts walking it. Thirty
// minutes is not long enough to ask a friend who is asleep.
//
// So an outstanding invite pins the autopilot in place — loadExpeditionsForAutoRun
// skips any expedition somebody has been asked to join. The pin is bounded by
// expeditionInviteTTL, because a leader who invites someone on holiday must not
// have their expedition frozen forever.
//
// The window the *leader* sees is wider than the plan's "before the first walk":
// an invite is legal for the whole of Day 1. Tying it to the first step would
// have made it a race against a ticker the player cannot see, and lost the
// leader's own `!expedition run` as well. Supplies barely move on day one — the
// pool burns at the night rollover — so a companion who arrives three rooms in
// pays and receives the same as one who arrives at the gate.

const (
	// expeditionInviteTTL bounds how long an unanswered invite holds the
	// autopilot. Long enough to catch someone between sessions of an async chat
	// bot; short enough that a forgotten invite costs the leader one afternoon,
	// not their expedition.
	expeditionInviteTTL = 2 * time.Hour
)

// Errors returned by the invite layer.
var (
	ErrInviteNotFound   = errors.New("no pending invite")
	ErrInviteWindowShut = errors.New("this expedition has already set off")
	ErrAlreadyInvited   = errors.New("player already has a pending invite here")
)

// ExpeditionInvite is one unanswered ask.
type ExpeditionInvite struct {
	ExpeditionID string
	UserID       string
	InvitedBy    string
	InvitedAt    time.Time
}

// inviteWindowOpen reports whether a party may still take on a member. Day 1
// only, and never after the boss is down — joining a finished expedition would
// seat someone into a payout they did not walk to.
func inviteWindowOpen(e *Expedition) bool {
	return e != nil && e.Status == ExpeditionStatusActive &&
		e.CurrentDay <= 1 && !e.BossDefeated
}

// inviteToParty records a pending invite. It refuses a full roster — counting
// outstanding invites against the ceiling, so a leader cannot ask four people
// and let them race for two seats — and refuses anyone already committed to an
// expedition of their own.
//
// The count, the guard, and the insert share a transaction for the same reason
// joinParty's do: two invites sent at once must not both find the last seat free.
func inviteToParty(expeditionID string, invitee, leader id.UserID) error {
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := seatLeader(tx, expeditionID); err != nil {
		return err
	}

	var seated, pending int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM expedition_party WHERE expedition_id = ?`,
		expeditionID).Scan(&seated); err != nil {
		return err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM expedition_invite WHERE expedition_id = ? AND invited_at > ?`,
		expeditionID, inviteCutoff()).Scan(&pending); err != nil {
		return err
	}
	if seated+pending >= expeditionPartyMax {
		return ErrPartyFull
	}

	if err := assertNotAdventuring(tx, expeditionID, invitee); err != nil {
		return err
	}

	res, err := tx.Exec(`
		INSERT INTO expedition_invite (expedition_id, user_id, invited_by, invited_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (expedition_id, user_id) DO NOTHING`,
		expeditionID, string(invitee), string(leader), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("invite %s: %w", invitee, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAlreadyInvited
	}
	return tx.Commit()
}

// inviteCutoff is the age past which an invite no longer counts as pending.
func inviteCutoff() time.Time { return time.Now().UTC().Add(-expeditionInviteTTL) }

// latestInviteFor returns the freshest unanswered invite addressed to a player,
// or ErrInviteNotFound.
//
// A player may hold invites from several leaders at once; `!expedition accept`
// takes the most recent, which is the one whose DM is still on their screen. The
// first accept wins the player — joinParty's assertNotAdventuring refuses the
// rest — so there is nothing to reconcile.
func latestInviteFor(userID id.UserID) (*ExpeditionInvite, error) {
	var inv ExpeditionInvite
	err := db.Get().QueryRow(`
		SELECT expedition_id, user_id, invited_by, invited_at
		  FROM expedition_invite
		 WHERE user_id = ? AND invited_at > ?
		 ORDER BY invited_at DESC
		 LIMIT 1`, string(userID), inviteCutoff()).Scan(
		&inv.ExpeditionID, &inv.UserID, &inv.InvitedBy, &inv.InvitedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// pendingInvites lists an expedition's unanswered invites, for the roster view.
func pendingInvites(expeditionID string) ([]ExpeditionInvite, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id, user_id, invited_by, invited_at
		  FROM expedition_invite
		 WHERE expedition_id = ? AND invited_at > ?
		 ORDER BY invited_at ASC`, expeditionID, inviteCutoff())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExpeditionInvite
	for rows.Next() {
		var inv ExpeditionInvite
		if err := rows.Scan(&inv.ExpeditionID, &inv.UserID, &inv.InvitedBy, &inv.InvitedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// clearInvite removes one answered invite.
func clearInvite(expeditionID string, userID id.UserID) error {
	_, err := db.Get().Exec(
		`DELETE FROM expedition_invite WHERE expedition_id = ? AND user_id = ?`,
		expeditionID, string(userID))
	return err
}

// clearExpeditionInvites drops every invite an expedition ever sent. Called
// beside disbandParty when the run reaches a terminal status.
func clearExpeditionInvites(expeditionID string) error {
	_, err := db.Get().Exec(
		`DELETE FROM expedition_invite WHERE expedition_id = ?`, expeditionID)
	return err
}

// purgeExpiredInvites deletes invites nobody answered. Every read already
// filters on the TTL, so this reclaims rows rather than enforcing a rule; it
// rides the existing one-minute eventTicker (D4: no net-new ticker).
func purgeExpiredInvites() {
	if _, err := db.Get().Exec(
		`DELETE FROM expedition_invite WHERE invited_at <= ?`, inviteCutoff()); err != nil {
		slog.Warn("expedition: purge expired invites", "err", err)
	}
}
