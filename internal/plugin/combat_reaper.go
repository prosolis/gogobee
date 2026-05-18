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
	userMu := p.advUserLock(userID)
	userMu.Lock()
	defer userMu.Unlock()

	sess, err := getActiveCombatSession(userID)
	if err != nil {
		slog.Error("combat: reaper failed to reload session", "user", userID, "err", err)
		return
	}
	// Gone, replaced, or no longer the stale one — the player acted first.
	if sess == nil || sess.SessionID != sessionID {
		return
	}

	player, enemy, err := p.combatantsForSession(sess)
	if err != nil {
		// Can't reconstruct the fight — park it terminal so it isn't retried
		// every minute forever.
		slog.Warn("combat: reaper cannot rebuild session, marking expired",
			"user", userID, "session", sess.SessionID, "err", err)
		if merr := markCombatSessionExpired(sess.SessionID); merr != nil {
			slog.Error("combat: reaper failed to mark session expired", "session", sess.SessionID, "err", merr)
		}
		return
	}

	rounds := 0
	for sess.IsActive() {
		if rounds >= reaperRoundCap {
			slog.Warn("combat: reaper hit round cap, marking expired",
				"user", userID, "session", sess.SessionID)
			if merr := markCombatSessionExpired(sess.SessionID); merr != nil {
				slog.Error("combat: reaper failed to mark session expired", "session", sess.SessionID, "err", merr)
			}
			return
		}
		if _, rerr := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionAttack}); rerr != nil {
			slog.Error("combat: reaper round failed", "user", userID, "session", sess.SessionID, "err", rerr)
			if merr := markCombatSessionExpired(sess.SessionID); merr != nil {
				slog.Error("combat: reaper failed to mark session expired", "session", sess.SessionID, "err", merr)
			}
			return
		}
		rounds++
	}

	outcome := p.finishCombatSession(userID, sess, enemy)
	preamble := fmt.Sprintf("⏳ Your fight with **%s** timed out — I finished it for you.\n\n", enemy.Name)
	if err := p.SendDM(userID, preamble+outcome); err != nil {
		slog.Error("combat: reaper failed to DM outcome", "user", userID, "err", err)
	}
}
