package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// N3/P5 — the party turn layer.
//
// A solo fight is a conversation between one player and the engine: they type,
// it answers, and nothing happens in between. A party fight is a queue. Three
// things follow from that, and this file is all three:
//
//  1. Turn ownership. Only the seat on the clock may act, so every combat
//     command has to ask "is it mine?" before it spends a slot or burns an item.
//  2. A fight-scoped lock. Three members typing `!attack` at once take three
//     *different* user locks, and the check above would pass for all three.
//  3. A turn deadline. One member who wanders off must not freeze the other two
//     until the 1h session reaper wakes up.
//
// Solo pays for none of it: the fight lock collapses to the user lock it always
// took, the turn check is trivially true, and nothing latches a solo seat onto
// autopilot — a lone player who walks away is the session reaper's problem, as
// they have always been.

// partyTurnDeadline is how long a party fight waits on one member before
// resolving their turn for them. Swept by the existing one-minute eventTicker
// (D4: no net-new tickers), so a lapse actually fires somewhere in
// [deadline, deadline+1m) — 3–4 minutes here.
//
// The number is a compromise between two failure modes. Too short and a player
// reading the room on their phone loses their boss turn; too long and two people
// sit staring at a prompt. Expeditions run for days, so the asymmetry favours
// patience.
const partyTurnDeadline = 3 * time.Minute

// combatTurn is a fight, opened for one member, with that member verified to be
// the seat on the clock. Held only for the duration of one command, under the
// locks release() drops.
type combatTurn struct {
	sess    *CombatSession
	players []*Combatant
	enemy   *Combatant
	seat    int
	// uid is the member acting, i.e. the player at seat.
	uid id.UserID
}

// isParty reports whether more than one character is seated.
func (ct *combatTurn) isParty() bool { return ct.sess.IsParty() }

// seatNames is the roster's display names in seating order, for narration.
func (ct *combatTurn) seatNames() []string {
	names := make([]string, len(ct.players))
	for i, c := range ct.players {
		names[i] = c.Name
	}
	return names
}

// activeCombatSessionFor finds the in-flight fight a player is in, whether they
// are its owner (seat 0) or a seated member. getActiveCombatSession alone keys
// on combat_session.user_id and so tells a party member they are not fighting.
func activeCombatSessionFor(userID id.UserID) (*CombatSession, error) {
	if s, err := getActiveCombatSession(userID); err != nil || s != nil {
		return s, err
	}
	return getActiveCombatSessionForMember(userID)
}

// lockCombatFight takes the locks a combat command needs, in an order that
// cannot deadlock: the fight's lock first — keyed on seat 0, the session's owner
// — and then the acting member's own lock. Every other command in the codebase
// takes at most one user lock, so there is no cycle to close.
//
// A solo fight's owner *is* the sender, and sync.Mutex is not reentrant, so that
// case takes exactly one lock: the same one handleAttackCmd has always taken.
func (p *AdventurePlugin) lockCombatFight(owner, sender id.UserID) func() {
	fight := p.advUserLock(owner)
	fight.Lock()
	if owner == sender {
		return fight.Unlock
	}
	self := p.advUserLock(sender)
	self.Lock()
	return func() {
		self.Unlock()
		fight.Unlock()
	}
}

// beginCombatTurn opens the sender's fight for an action: it locates the
// session, takes its locks, settles any phase the engine still owes (an enemy
// turn left half-resolved by a crash), rebuilds the roster, and verifies the
// sender is the seat on the clock.
//
// On any refusal it releases what it took and returns a player-facing message
// with a nil turn; noFightMsg is the caller's own copy for "you are not in a
// fight", which differs per command. On success the caller must call release().
//
// Acting also unlatches the member from autopilot: typing anything is proof they
// are back at the keyboard.
func (p *AdventurePlugin) beginCombatTurn(sender id.UserID, noFightMsg string) (*combatTurn, func(), string) {
	probe, err := activeCombatSessionFor(sender)
	if err != nil {
		return nil, nil, "Couldn't read combat state: " + err.Error()
	}
	if probe == nil {
		return nil, nil, noFightMsg
	}

	release := p.lockCombatFight(id.UserID(probe.UserID), sender)
	fail := func(msg string) (*combatTurn, func(), string) {
		release()
		return nil, nil, msg
	}

	// Re-read under the lock. The probe was unlocked, so the fight may have
	// ended, or been replaced by a fresh one, in the window since.
	sess, err := getCombatSession(probe.SessionID)
	if err != nil {
		return fail("Couldn't read combat state: " + err.Error())
	}
	if sess == nil || !sess.IsActive() {
		return fail(noFightMsg)
	}
	seat, seated := sess.seatOf(sender)
	if !seated {
		return fail(noFightMsg)
	}

	players, enemy, err := p.partyCombatantsForSession(sess)
	if err != nil {
		return fail("Couldn't rebuild the fight: " + err.Error())
	}

	// Settle any phase the engine still owes before reading whose turn it is. A
	// fight interrupted mid enemy-turn resumes parked there; without this the
	// player's !attack would be refused as "not your turn" and the fight would
	// never advance past it again.
	if _, serr := settleCombatSession(sess, players, enemy); serr != nil {
		return fail("Couldn't resolve the round: " + serr.Error())
	}
	if !sess.IsActive() {
		// The owed phase was lethal: the settle above ended the fight. The
		// terminal status is already persisted, and the reaper only scans for
		// active sessions, so this is the last chance anyone has to pay the
		// party out. Close it here rather than answering "you're not in a
		// fight" over a win nobody was credited for.
		ct := &combatTurn{sess: sess, players: players, enemy: enemy, seat: seat, uid: sender}
		outcomes := p.closePartyRound(ct)
		if !ct.isParty() {
			return fail(outcomes[0])
		}
		p.announcePartyRound(ct, nil, "", outcomes)
		return fail("")
	}

	acting, waiting := actingSeat(sess, players, enemy)
	if !waiting || acting != seat {
		return fail(notYourTurnMsg(players, acting, waiting))
	}

	// They typed, so they are here. Hand back the wheel.
	sess.actorStatusesPtr(seat).Autopilot = false

	return &combatTurn{sess: sess, players: players, enemy: enemy, seat: seat, uid: sender}, release, ""
}

// notYourTurnMsg tells a member who is holding the round up.
func notYourTurnMsg(players []*Combatant, acting int, waiting bool) string {
	if !waiting || acting < 0 || acting >= len(players) {
		return "The round is still resolving — try again in a moment."
	}
	return fmt.Sprintf("It's **%s**'s turn. Hang tight — I'll act for them if they're away.", players[acting].Name)
}

// ── driving a round ──────────────────────────────────────────────────────────

// driveCombatRound resolves the acting member's action, then keeps the fight
// moving until it comes to rest on a player who can actually answer: the enemy
// turn and round-end tick resolve, downed seats forfeit, and seats latched onto
// autopilot are played by the picker.
//
// Solo never enters the autopilot loop — nothing latches a solo seat.
func (p *AdventurePlugin) driveCombatRound(ct *combatTurn, action PlayerAction) ([]CombatEvent, error) {
	events, err := runPartyCombatRound(ct.sess, ct.players, ct.enemy, action)
	if err != nil {
		return events, err
	}
	more, err := p.driveAutopilotedSeats(ct)
	return append(events, more...), err
}

// driveAutopilotedSeats plays out every latched seat standing between the fight
// and its next live human turn.
func (p *AdventurePlugin) driveAutopilotedSeats(ct *combatTurn) ([]CombatEvent, error) {
	var events []CombatEvent
	for i := 0; i < partyRoundStepCap; i++ {
		seat, waiting := actingSeat(ct.sess, ct.players, ct.enemy)
		if !waiting || !ct.sess.seatIsAutopiloted(seat) {
			return events, nil
		}
		more, err := p.runAutoSeatTurn(ct, seat)
		if err != nil {
			return events, err
		}
		events = append(events, more...)
	}
	return events, fmt.Errorf("combat session %s: autopilot did not settle within %d turns",
		ct.sess.SessionID, partyRoundStepCap)
}

// runAutoSeatTurn resolves one seat's turn with nobody at the keyboard, through
// the same decision tree the headless sim and the expedition autopilot use. A
// cast or consume the picker chose but the resolver then refuses (the slot went
// missing, the item was sold from another room) degrades to a weapon attack
// rather than stalling the round.
func (p *AdventurePlugin) runAutoSeatTurn(ct *combatTurn, seat int) ([]CombatEvent, error) {
	uid := id.UserID(ct.sess.seatUserID(seat))
	kind, arg := p.pickAutoCombatActionForSeat(uid, ct.sess, seat)

	action := PlayerAction{Kind: ActionAttack}
	settle := func(bool) {}
	switch kind {
	case "cast":
		if a, s, msg := p.castActionForSeat(ct, seat, arg); msg == "" {
			action, settle = a, s
		} else {
			slog.Debug("combat: autopilot cast refused, swinging instead",
				"session", ct.sess.SessionID, "seat", seat, "spell", arg, "why", msg)
		}
	case "consume":
		if a, s, msg := p.consumeActionForSeat(ct, seat, arg); msg == "" {
			action, settle = a, s
		} else {
			slog.Debug("combat: autopilot consume refused, swinging instead",
				"session", ct.sess.SessionID, "seat", seat, "item", arg, "why", msg)
		}
	}

	events, err := runPartyCombatRound(ct.sess, ct.players, ct.enemy, action)
	settle(err == nil)
	return events, err
}

// ── the turn deadline ────────────────────────────────────────────────────────

// nudgeStalledPartyTurns latches every party seat whose turn deadline has lapsed
// onto autopilot and plays the fight forward. Driven off eventTicker, beside the
// session reaper, so it costs no new ticker.
//
// Solo sessions are never listed: a lone player who walks away owns their own
// fight, and the 1h reaper already finishes it for them.
func (p *AdventurePlugin) nudgeStalledPartyTurns() {
	stalled, err := listStalledPartyCombatSessions()
	if err != nil {
		slog.Error("combat: failed to list stalled party turns", "err", err)
		return
	}
	for _, sess := range stalled {
		p.nudgeStalledPartyTurn(sess.SessionID)
	}
}

// nudgeStalledPartyTurn resolves one stalled fight under its lock. It re-reads
// the session after locking: the member may have acted in the window between the
// sweep's query and the lock, in which case there is nothing to do.
func (p *AdventurePlugin) nudgeStalledPartyTurn(sessionID string) {
	probe, err := getCombatSession(sessionID)
	if err != nil || probe == nil {
		return
	}
	owner := id.UserID(probe.UserID)
	release := p.lockCombatFight(owner, owner)
	defer release()

	sess, err := getCombatSession(sessionID)
	if err != nil {
		slog.Error("combat: stalled-turn reload failed", "session", sessionID, "err", err)
		return
	}
	if sess == nil || !sess.IsActive() || !sess.IsParty() || !turnDeadlineLapsed(sess) {
		return
	}

	players, enemy, err := p.partyCombatantsForSession(sess)
	if err != nil {
		slog.Warn("combat: cannot rebuild stalled party fight", "session", sessionID, "err", err)
		return
	}
	seat, waiting := actingSeat(sess, players, enemy)
	if !waiting {
		return
	}

	// Latch the absent member. From here their turns resolve the moment the
	// round reaches them, until they type something.
	sess.actorStatusesPtr(seat).Autopilot = true
	away := id.UserID(sess.seatUserID(seat))

	ct := &combatTurn{sess: sess, players: players, enemy: enemy, seat: seat, uid: away}
	events, err := p.driveAutopilotedSeats(ct)
	if err != nil {
		slog.Error("combat: stalled-turn autopilot failed", "session", sessionID, "err", err)
		return
	}

	preamble := fmt.Sprintf("⏳ **%s** was away — I'm taking their turns.\n\n", players[seat].Name)
	p.announcePartyRound(ct, events, preamble, p.closePartyRound(ct))
}

// turnDeadlineLapsed reports whether the fight has sat on one member's turn past
// partyTurnDeadline. LastActionAt is stamped by every saveCombatSession, and the
// save that parked the fight on this seat's player_turn was the last one — so it
// is exactly when the member's clock started.
func turnDeadlineLapsed(sess *CombatSession) bool {
	return sess.Phase == CombatPhasePlayerTurn &&
		time.Since(sess.LastActionAt) >= partyTurnDeadline
}

// listStalledPartyCombatSessions returns every active *party* fight parked on a
// player's turn past the deadline. The roster_size filter keeps solo fights —
// which is every fight that has ever run — out of the sweep entirely.
func listStalledPartyCombatSessions() ([]*CombatSession, error) {
	cutoff := time.Now().UTC().Add(-partyTurnDeadline)
	rows, err := db.Get().Query(`SELECT `+combatSessionCols+`
		  FROM combat_session
		 WHERE status = 'active'
		   AND roster_size > 1
		   AND phase = 'player_turn'
		   AND last_action_at <= ?
		 ORDER BY last_action_at ASC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*CombatSession
	for rows.Next() {
		s, err := scanCombatSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Hydrate only after the cursor is closed: a nested query while iterating
	// can stall on a single-connection pool.
	rows.Close()
	for _, s := range out {
		if err := hydrateCombatParticipants(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ── telling the party what happened ──────────────────────────────────────────

// closePartyRound runs the terminal side effects exactly once and returns each
// seat's own close-out block. It returns nil while the fight is still in flight.
//
// Callers must invoke it whether or not they intend to narrate: a silent
// autopilot round still owes the party its XP, loot, and death bookkeeping.
func (p *AdventurePlugin) closePartyRound(ct *combatTurn) []string {
	if ct.sess.IsActive() {
		return nil
	}
	return p.finishPartyCombatSession(ct)
}

// announcePartyRound DMs every seated member the round that just resolved. It is
// the fan-out the single-recipient SendDM seam does not have: one call per seat,
// with each member's own footer — or their own close-out, if outcomes is set.
//
// Solo callers do not use it — handleCombatActionCmd replies to the one player
// directly, as it always has.
func (p *AdventurePlugin) announcePartyRound(ct *combatTurn, events []CombatEvent, preamble string, outcomes []string) {
	names := ct.seatNames()
	acting, waiting := actingSeat(ct.sess, ct.players, ct.enemy)
	for seat, uid := range ct.sess.SeatUserIDs() {
		// Rendered once per reader: the flavor pool speaks in the second person,
		// so each member's own events must be theirs and nobody else's.
		body := RenderPartyTurnRound(events, names, ct.enemy.Name, seat)
		tail := ""
		switch {
		case seat < len(outcomes):
			tail = outcomes[seat]
		case waiting:
			tail = partyRoundFooter(ct, seat, acting)
		}
		if err := p.SendDM(id.UserID(uid), preamble+body+"\n\n"+tail); err != nil {
			slog.Error("combat: party round DM failed", "user", uid, "err", err)
		}
	}
}

// partyRoundFooter is the per-member close of a round: the roster's HP, then
// either "your move" or who everyone is waiting on.
func partyRoundFooter(ct *combatTurn, seat, acting int) string {
	var b strings.Builder
	for i, c := range ct.players {
		down := ""
		if !ct.sess.seatAlive(i) {
			down = " _(down)_"
		}
		b.WriteString(fmt.Sprintf("%s: **%d/%d**%s\n", c.Name, ct.sess.seatHP(i), ct.sess.seatHPMax(i), down))
	}
	b.WriteString(fmt.Sprintf("%s: **%d/%d**\n\n", ct.enemy.Name, ct.sess.EnemyHP, ct.sess.EnemyHPMax))
	if seat == acting {
		b.WriteString(partyMovePrompt(ct.sess.Round))
	} else {
		b.WriteString(fmt.Sprintf("**Round %d.** Waiting on **%s**.", ct.sess.Round, ct.players[acting].Name))
	}
	return b.String()
}
