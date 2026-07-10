package plugin

import (
	"fmt"
	"log/slog"
	"strings"

	"maunium.net/go/mautrix/id"
)

// N3/P5 — closing out a party fight.
//
// finishCombatSession (combat_cmd.go) is the solo close-out and stays exactly
// what it was. This is its sibling for a seated roster, and the split it makes
// is the one the data model already forced:
//
//   - Run- and expedition-scoped effects fire ONCE, for the owner. Threat, the
//     zone-kill record, the boss-defeat threat drop, the run teardown on a loss
//     — every one of them resolves through getActiveExpedition(userID) or
//     getActiveZoneRun(userID), and a party member owns neither row. Fanning
//     them out would be a no-op for members and would triple the threat the
//     owner pays for a single kill.
//   - Character-scoped effects fan out. HP, XP, loot, and death belong to the
//     person, not the expedition, and every seat gets their own.
//
// A member can be dead in a fight the party won: they dropped, the others
// finished the job. So death is decided per seat off HP, not off the session's
// terminal status.

// finishPartyCombatSession runs the terminal side effects for a seated roster
// and returns each seat's own player-facing close-out, indexed by seat. A solo
// session delegates to finishCombatSession untouched.
func (p *AdventurePlugin) finishPartyCombatSession(ct *combatTurn) []string {
	sess, enemy := ct.sess, *ct.enemy
	owner := id.UserID(sess.UserID)
	if !ct.isParty() {
		return []string{p.finishCombatSession(owner, sess, enemy)}
	}

	run, _ := getZoneRun(sess.RunID)
	var zone ZoneDefinition
	elite := true
	cadence := sess.Round
	if run != nil {
		zone = zoneOrFallback(run.ZoneID)
		elite = run.CurrentRoomType() != RoomBoss
		cadence = narrationCadence(run)
	}
	monster := dndBestiary[sess.EnemyID]

	// nat20/nat1 mood deltas from the whole fight's event log — the run's, so once.
	scanMoodEventsFromEvents(sess.RunID, sess.TurnLog)

	// Every seat's HP lands on their sheet regardless of how the fight ended, and
	// so does the bookkeeping that outlives the fight — a Berserker who raged and
	// lost is still exhausted. Both fan out; neither is the owner's alone.
	for seat := range sess.RosterSize() {
		persistDnDHPAfterCombat(id.UserID(sess.seatUserID(seat)), sess.seatHP(seat))
		p.postCombatBookkeepingForSeat(sess, seat)
	}

	switch sess.Status {
	case CombatStatusWon:
		return p.finishPartyWin(ct, run, zone, monster, elite, cadence)
	case CombatStatusLost:
		return p.finishPartyLoss(ct, zone, cadence)
	case CombatStatusFled:
		endRunOnLoss(owner, sess.RunID, false)
		return p.eachSeat(ct, fmt.Sprintf(
			"🏃 The party broke off from **%s** and slipped away. Run ended — you keep your wounds, but you live.", enemy.Name))
	default:
		return p.eachSeat(ct, "The fight is over.")
	}
}

// finishPartyWin pays the party out. The kill is the expedition's, so threat and
// the zone-kill record resolve once through the owner; XP and loot are the
// character's, so every member rolls their own — loot independently, per the C1
// no-split rule, and XP in full, per accessibility over crunch.
func (p *AdventurePlugin) finishPartyWin(
	ct *combatTurn, run *DungeonRun, zone ZoneDefinition, monster DnDMonsterTemplate, elite bool, cadence int,
) []string {
	sess := ct.sess
	owner := id.UserID(sess.UserID)

	bossOnExpedition := p.applyOwnerWinEffects(owner, sess.EnemyID, elite)

	tier := 1
	if run != nil {
		tier = int(zone.Tier)
	}
	shared := twinBeeLine(zone.ID, DMCombatEnd, sess.RunID, cadence)
	emoji := "✅"
	if !elite {
		emoji = "🏆"
	}

	out := make([]string, sess.RosterSize())
	for seat := range sess.RosterSize() {
		uid := id.UserID(sess.seatUserID(seat))
		hp, hpMax := sess.seatHP(seat), sess.seatHPMax(seat)

		// A member who went down before the killing blow still earns the kill.
		p.grantSeatWinXP(uid, hp, hpMax, monster, tier)

		var b strings.Builder
		if shared != "" && seat == 0 {
			b.WriteString(shared + "\n")
		}
		b.WriteString(fmt.Sprintf("%s **%s** down. The party stands.\n", emoji, ct.enemy.Name))
		if hp <= 0 {
			// They won it from the floor. Down is down: the hospital takes them.
			markAdventureDead(uid, "zone", zone.Display)
			b.WriteString("💀 You didn't see it fall — you were already down.\n")
		} else {
			b.WriteString(fmt.Sprintf("You finished at **%d/%d HP**.\n", hp, hpMax))
			if drop := p.dropZoneLoot(uid, zone.ID, monster, !elite, elite); drop != "" {
				b.WriteString(drop + "\n")
			}
		}
		switch {
		case bossOnExpedition && seat == 0:
			b.WriteString("🎉 **Zone cleared — the expedition is won.** `!expedition run` to march out and claim your spoils.")
		case bossOnExpedition:
			b.WriteString("🎉 **Zone cleared — the expedition is won.** Your leader marches the party out.")
		case seat == 0:
			b.WriteString(continueHint(owner))
		default:
			b.WriteString("Waiting on your leader to move the party on.")
		}
		out[seat] = b.String()
	}
	return out
}

// finishPartyLoss ends the run for everyone. The whole roster is down — the turn
// engine only reports Lost once anyAlive() goes false — so every member takes
// the death, and the owner's run and expedition are torn down once.
func (p *AdventurePlugin) finishPartyLoss(ct *combatTurn, zone ZoneDefinition, cadence int) []string {
	sess := ct.sess
	owner := id.UserID(sess.UserID)

	endRunOnLoss(owner, sess.RunID, true)

	line := twinBeeLine(zone.ID, DMPlayerDeath, sess.RunID, cadence)
	out := make([]string, sess.RosterSize())
	for seat := range sess.RosterSize() {
		markAdventureDead(id.UserID(sess.seatUserID(seat)), "zone", zone.Display)
		var b strings.Builder
		if line != "" && seat == 0 {
			b.WriteString(line + "\n")
		}
		b.WriteString(fmt.Sprintf("💀 The party fell to **%s**. Run ended.", ct.enemy.Name))
		out[seat] = b.String()
	}
	return out
}

// eachSeat gives every seat the same close-out block.
func (p *AdventurePlugin) eachSeat(ct *combatTurn, block string) []string {
	out := make([]string, ct.sess.RosterSize())
	for i := range out {
		out[i] = block
	}
	return out
}

// ── Shared terminal effects (item D) ─────────────────────────────────────────
//
// finishCombatSession (solo, combat_cmd.go) and finishPartyWin/finishPartyLoss
// (party, above) still write their own player-facing text — the solo line says
// "You finished", the party line says "The party stands", and the death-on-win
// rule is deliberately roster-size-dependent (see item E). But the *effects*
// they fire were hand-copied lists, and item A drifted exactly there. These
// helpers are the single copy of each effect; both close-outs route through
// them so the effect lists cannot diverge again.

// applyOwnerWinEffects fires the terminal effects a win owes the run and
// expedition — the zone-kill record, room threat, and the boss-defeat threat
// drop — once, through the row's owner (a party member owns neither row).
// Returns whether the kill was a zone boss on an active expedition; both
// close-outs use it to reframe the final line as the expedition's climax.
func (p *AdventurePlugin) applyOwnerWinEffects(owner id.UserID, enemyID string, elite bool) (bossOnExpedition bool) {
	recordZoneKillForUser(owner, enemyID)
	applyRoomCombatThreatForUser(owner, elite)
	if !elite {
		// §8.1 — a zone boss defeat drops expedition threat. Silent no-op for a
		// standalone zone run with no active expedition.
		if exp, eerr := getActiveExpedition(owner); eerr == nil && exp != nil {
			_ = applyBossDefeatThreat(exp.ID)
			bossOnExpedition = true
		}
	}
	return bossOnExpedition
}

// grantSeatWinXP pays one combatant their kill XP. A seat that finished at a
// fifth of its pool or less takes the near-death path. Full XP per person,
// solo or party, per the accessibility-over-crunch stance.
func (p *AdventurePlugin) grantSeatWinXP(uid id.UserID, hp, hpMax int, monster DnDMonsterTemplate, tier int) {
	nearDeath := hpMax > 0 && hp*5 < hpMax
	if xp := zoneCombatXP(CombatResult{PlayerWon: true, NearDeath: nearDeath}, monster.CR, tier); xp > 0 {
		if _, err := p.grantDnDXP(uid, xp); err != nil {
			slog.Error("combat: grantDnDXP", "user", uid, "err", err)
		}
	}
}

// endRunOnLoss tears down the owner's run and wrapping expedition when a fight
// ends in death or flight. On a death (not a flee) it also stamps the run's
// mood event. Owner-scoped, fires once; the per-seat death mark stays with the
// caller, since it is decided per HP and gated on roster size.
func endRunOnLoss(owner id.UserID, runID string, death bool) {
	if death {
		if run, _ := getZoneRun(runID); run != nil {
			_, _ = applyMoodEvent(runID, MoodEventPlayerDeath)
		}
	}
	_ = abandonZoneRun(owner)
	reason := "combat flee"
	if death {
		reason = "combat death"
	}
	forceExtractExpeditionForRunLoss(owner, reason)
}
