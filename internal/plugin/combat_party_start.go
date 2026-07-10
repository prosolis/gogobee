package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"maunium.net/go/mautrix/id"
)

// N3/P6c — seating the party at the doorway.
//
// P5 built startPartyCombatSession and left it with no production caller: there
// was no roster to seat it with. P6b filled the roster. This is the join.
//
// The rule the whole file turns on is that **seat 0 is the expedition leader**.
// Every seat-0 invariant P4 and P5 laid down rests on it: combat_session.user_id
// is the fight's lock key, the run- and expedition-scoped close-out effects fire
// once through it, and `!flee` is refused to any seat but zero. A party whose
// seat 0 were merely "whoever typed !fight" would flee the leader's run on a
// member's say-so.

// fightRoster is the seating order for a fight opened on this run: the
// expedition's leader first, then their party in join order. A bare zone run —
// and a solo expedition, whose roster table is empty — resolves to the one
// player who owns it.
//
// It doubles as the answer to "under whose lock is this fight taken", since
// roster[0] is the session's owner. It is resolved *before* the lock, so it
// must not touch getActiveZoneRun — that lookup carries the §4.3 idle reap.
func fightRoster(sender id.UserID) []id.UserID {
	e, _, err := activeExpeditionFor(sender)
	if err != nil || e == nil {
		return []id.UserID{sender}
	}
	return expeditionAudience(e)
}

// buildFightSeats turns a roster into the seats that will actually sit down, and
// the enemy they face. A member who is down, or somehow already fighting, is left
// out: they sit this one out rather than blocking the party. The leader is not
// optional — if seat 0 cannot fight, nobody does, and `refusal` says why in terms
// of whoever typed `!fight`.
//
// senderSkip is the sender's own reason for being left out, empty when they are
// seated. Without it a downed member's `!fight` opens the party's fight and then
// answers them with silence.
//
// The enemy is built once. Every seat's build derives the identical stat block
// from (monster, tier, dmMood); only the player half varies.
func (p *AdventurePlugin) buildFightSeats(
	sender id.UserID, roster []id.UserID, monster DnDMonsterTemplate, tier, dmMood int,
) (seats []CombatSeatSetup, enemy *Combatant, senderSkip, refusal string) {
	skip := func(uid id.UserID, why string) {
		if uid == sender {
			senderSkip = why
		}
	}
	for i, uid := range roster {
		leader := i == 0

		// Both refusals below are cheap and neither needs the build, so they run
		// before it: consuming a seat's armed ability and *then* sitting them out
		// would spend their rage on a fight they never joined.
		hp, hpMax := dndHPSnapshot(uid)
		if hp <= 0 {
			if leader {
				return nil, nil, "", seatZeroRefusal(sender, uid,
					"You're in no shape to fight. `!rest` first.",
					"Your party leader is in no shape to fight. The fight waits for them.")
			}
			skip(uid, "You're in no shape to fight — the party goes in without you. `!rest` when you can.")
			continue
		}
		// A member's live fight lives on a combat_participant row, so
		// hasActiveCombatSession — which keys on combat_session.user_id — would
		// answer "no" for them mid-fight. The caller has already ruled out this
		// room's own encounter, so anything found here is a different fight.
		if s, serr := activeCombatSessionFor(uid); serr == nil && s != nil {
			if leader {
				return nil, nil, "", seatZeroRefusal(sender, uid,
					"You're already in a fight. Finish it with `!attack` / `!flee`.",
					"Your party leader is already in a fight somewhere else.")
			}
			slog.Info("combat: party member busy in another fight", "user", uid, "session", s.SessionID)
			skip(uid, "You're already in a fight elsewhere — the party goes in without you.")
			continue
		}

		// Consumed exactly once for the fight, here. Every later rebuild
		// re-applies this id off the seat's statuses rather than re-arming.
		armed := ""
		if c, cerr := LoadDnDCharacter(uid); cerr == nil && c != nil {
			trySimAutoArm(c)
			armed = consumeArmedAbility(c)
		}
		player, e, _, err := p.buildZoneCombatants(uid, monster, tier, dmMood, armed)
		if err != nil {
			if leader {
				return nil, nil, "", "Couldn't set up the fight: " + err.Error()
			}
			slog.Warn("combat: party member left out of fight", "user", uid, "err", err)
			skip(uid, "Couldn't bring you into the fight: "+err.Error())
			continue
		}
		if leader {
			enemy = &e
		}
		if armed != "" {
			slog.Info("combat: armed ability fired (turn-based start)", "user", uid, "ability", armed)
		}

		seats = append(seats, CombatSeatSetup{
			UserID: uid, HP: hp, HPMax: hpMax, Mods: player.Mods, C: &player, ArmedAbility: armed,
		})
	}
	return seats, enemy, senderSkip, ""
}

// seatCombatants is the roster's freshly-built characters in seating order — the
// same slice partyCombatantsForSession would rebuild from the persisted session,
// threaded through from the build that seated them instead.
func seatCombatants(seats []CombatSeatSetup) []*Combatant {
	players := make([]*Combatant, len(seats))
	for i, s := range seats {
		players[i] = s.C
	}
	return players
}

// seatZeroRefusal picks the copy for a blocked seat 0. The leader hears about
// themselves in the second person; a member hears who the party is waiting on.
func seatZeroRefusal(sender, leader id.UserID, own, aboutLeader string) string {
	if sender == leader {
		return own
	}
	return aboutLeader
}

// announcePartyFightStart DMs every seated member the opening block: the shared
// header, their own HP and curios, the roster's initiative order, whatever the
// enemy did before anyone could stop it, and either "your move" or who the round
// is waiting on.
//
// opening carries the events of a round-1 enemy turn — the monster can win
// initiative, and a party that reads "your move" over an unnarrated 12-point hit
// has been lied to. outcomes, when set, is the per-seat close-out of a fight that
// ended before it started.
//
// Solo does not come through here — handleFightCmd answers the one player who
// typed, with the bytes it always sent.
func (p *AdventurePlugin) announcePartyFightStart(
	sess *CombatSession, players []*Combatant, enemy *Combatant, header string,
	opening []CombatEvent, outcomes []string,
) {
	acting, waiting := actingSeat(sess, players, enemy)
	names := make([]string, len(players))
	for i, c := range players {
		names[i] = c.Name
	}
	for seat, uid := range sess.SeatUserIDs() {
		var b strings.Builder
		b.WriteString(header)
		b.WriteString(fmt.Sprintf("You: **%d/%d HP**.\n", sess.seatHP(seat), sess.seatHPMax(seat)))
		if curios := activeMagicItemsLine(id.UserID(uid)); curios != "" {
			b.WriteString(curios + "\n")
		}
		b.WriteString("\n**Marching order:** ")
		b.WriteString(strings.Join(initiativeNames(players, sess, enemy), " → "))
		b.WriteString("\n\n")
		if len(opening) > 0 {
			b.WriteString(RenderPartyTurnRound(opening, names, enemy.Name, seat))
			b.WriteString("\n\n")
		}
		switch {
		case seat < len(outcomes):
			b.WriteString(outcomes[seat])
		case !waiting:
			b.WriteString(fmt.Sprintf("**Round %d.** The round is resolving.", sess.Round))
		case seat == acting:
			b.WriteString(partyMovePrompt(sess.Round))
		default:
			b.WriteString(fmt.Sprintf("**Round %d.** Waiting on **%s**.", sess.Round, players[acting].Name))
		}
		if err := p.SendDM(id.UserID(uid), b.String()); err != nil {
			slog.Error("combat: party fight-start DM failed", "user", uid, "err", err)
		}
	}
}

// partyMovePrompt is the line a seat sees when the round is its own. `!flee` is
// missing on purpose: in a party it is the leader's call, and P5 refuses it to
// every other seat.
func partyMovePrompt(round int) string {
	return fmt.Sprintf("**Round %d.** Your move — `!attack`, `!cast <spell>`, `!consume <item>`.", round)
}

// initiativeNames renders this round's turn order for the opening block. It is
// the party's one look at the initiative P3 rolls for them — the number itself
// stays hidden (accessibility over crunch); the order is the useful part.
func initiativeNames(players []*Combatant, sess *CombatSession, enemy *Combatant) []string {
	order := turnOrder(sess, sess.Round, players, enemy)
	names := make([]string, 0, len(order))
	for _, seat := range order {
		if seat == enemySeat {
			names = append(names, enemy.Name)
			continue
		}
		names = append(names, players[seat].Name)
	}
	return names
}
