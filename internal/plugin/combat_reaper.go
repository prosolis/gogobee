package plugin

import (
	"fmt"
	"log/slog"

	"maunium.net/go/mautrix/id"
)

// Phase 13 — turn-based combat timeout reaper.
//
// A CombatSession expires 1h after its last action (combatSessionTTL). Per the
// 2026-05-13 timeout decision, an abandoned fight is not flatly marked as a
// retreat — the reaper resumes it from persisted mid-state and auto-plays the
// rest of the fight through the same shared resolver a live !attack uses,
// landing on a real win or loss. The player is DM'd the outcome.
//
// Sessions that cannot be reconstructed (zone run deleted, enemy no longer in
// the bestiary) or that fail to converge within reaperRoundCap fall back to the
// terminal 'expired' status with no fatal-blow side effects — see
// markCombatSessionExpired.

// reaperRoundCap bounds the auto-play loop. A turn-based round always lands at
// least one damage roll eventually, so a real fight converges well within this;
// the cap only guards against a pathological non-terminating session.
const reaperRoundCap = 300

// reapExpiredCombatSessions is the timeout reaper, driven off eventTicker (one
// pass per minute). It auto-plays every stale session to a terminal status.
func (p *AdventurePlugin) reapExpiredCombatSessions() {
	expired, err := listExpiredCombatSessions()
	if err != nil {
		slog.Error("combat: reaper failed to list expired sessions", "err", err)
		return
	}
	for _, sess := range expired {
		p.reapCombatSession(sess.UserID, sess.SessionID)
	}
}

// reapCombatSession auto-plays a single stale session under the player's lock.
// It re-fetches the session after locking: the player may have finished the
// fight in the window between listing and locking, in which case there is
// nothing to do.
func (p *AdventurePlugin) reapCombatSession(userIDStr, sessionID string) {
	userID := id.UserID(userIDStr)
	// The session's owner is seat 0, which is the user the sweep listed — so the
	// fight lock and the user lock are the same mutex here, taken once.
	release := p.lockCombatFight(userID, userID)
	defer release()

	sess, err := getActiveCombatSession(userID)
	if err != nil {
		slog.Error("combat: reaper failed to reload session", "user", userID, "err", err)
		return
	}
	// Gone, replaced, or no longer the stale one — the player acted first.
	if sess == nil || sess.SessionID != sessionID {
		return
	}

	expire := func(why string, args ...any) {
		slog.Warn("combat: reaper "+why, append([]any{"user", userID, "session", sess.SessionID}, args...)...)
		if merr := markCombatSessionExpired(sess.SessionID); merr != nil {
			slog.Error("combat: reaper failed to mark session expired", "session", sess.SessionID, "err", merr)
		}
	}

	players, enemy, err := p.partyCombatantsForSession(sess)
	if err != nil {
		// Can't reconstruct the fight — park it terminal so it isn't retried
		// every minute forever.
		expire("cannot rebuild session, marking expired", "err", err)
		return
	}

	// Every seat swings until the fight lands. Deliberately *not* the auto-picker
	// the turn deadline uses: this is an abandoned fight, and finishing it should
	// not quietly burn the player's spell slots and potions on their behalf. The
	// turn-deadline latch is different — that member is mid-fight and their party
	// is waiting, so playing their character properly is the whole point.
	//
	// Each pass resolves one seat's turn and drains the enemy turn, the round-end
	// tick, and any downed seat behind it, so a solo fight walks exactly the loop
	// it always did.
	ct := &combatTurn{sess: sess, players: players, enemy: enemy, seat: 0, uid: userID}
	rounds := 0
	for sess.IsActive() {
		if rounds >= reaperRoundCap {
			expire("hit round cap, marking expired")
			return
		}
		if _, rerr := runPartyCombatRound(sess, players, enemy, PlayerAction{Kind: ActionAttack}); rerr != nil {
			expire("round failed", "err", rerr)
			return
		}
		rounds++
	}

	outcomes := p.closePartyRound(ct)
	preamble := fmt.Sprintf("⏳ Your fight with **%s** timed out — I finished it for you.\n\n", enemy.Name)
	if !ct.isParty() {
		if err := p.SendDM(userID, preamble+outcomes[0]); err != nil {
			slog.Error("combat: reaper failed to DM outcome", "user", userID, "err", err)
		}
		return
	}
	p.announcePartyRound(ct, nil, preamble, outcomes)
}
