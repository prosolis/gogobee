package plugin

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Phase 13 — turn-based combat command surface.
//
// !fight  — engage the Elite/Boss room the player is standing at, opening a
//           persisted CombatSession.
// !attack — resolve one full round (player turn → enemy turn → round end).
// !flee   — break off; the run ends with a light penalty.
//
// !zone advance stops at an Elite/Boss doorway (see zoneCmdAdvance); the
// player explicitly opts into the fight here. While a session is active,
// !zone advance / enter / go are blocked — one fight locks the run.

// encounterIDForRoom is the stable per-room key tying a CombatSession to the
// room it was opened in, so a won session can be recognised by the room
// resolver. Unique within a run; combined with run_id it's globally unique.
func encounterIDForRoom(roomIdx int) string {
	return fmt.Sprintf("room%d", roomIdx)
}

// replyDM sends a player-facing combat reply unless ctx.Silent is set. The
// turn-engine combat commands route all their DMs through here so the
// background autopilot can drive a boss/elite fight on the real engine
// (long-expedition D8-f) without spamming the player a DM per round — the
// state mutations (HP, XP, threat, run-clear) still happen; only the
// narration is dropped. Non-silent callers (manual !fight) are unchanged.
func (p *AdventurePlugin) replyDM(ctx MessageContext, text string) error {
	// An empty body means the caller already answered the player another way —
	// a party fan-out, say. Sending it would post a blank DM.
	if ctx.Silent || text == "" {
		return nil
	}
	return p.SendDM(ctx.Sender, text)
}

// ── !fight ──────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleFightCmd(ctx MessageContext) error {
	// Resolve the roster before locking — a member's `!fight` opens the leader's
	// fight, under the leader's lock, and seat 0 names that leader. fightRoster
	// deliberately does not touch getActiveZoneRun: that lookup carries the §4.3
	// idle reap, and it must only ever fire under the lock, on its owner's behalf.
	roster := fightRoster(ctx.Sender)
	release := p.lockCombatFight(roster[0], ctx.Sender)
	defer release()

	run, _, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.replyDM(ctx, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.replyDM(ctx, "No active zone run. Use `!zone enter <id>` first.")
	}
	roomType := run.CurrentRoomType()
	if roomType != RoomElite && roomType != RoomBoss {
		return p.replyDM(ctx, "Nothing to fight here — `!fight` is for Elite and Boss rooms. Use `!zone advance`.")
	}

	encID := encounterIDForRoom(run.CurrentRoom)
	if existing, _ := getCombatSessionForEncounter(run.RunID, encID); existing != nil {
		switch existing.Status {
		case CombatStatusActive:
			return p.replyDM(ctx, "You're already in this fight — `!attack` or `!flee`.")
		case CombatStatusWon:
			return p.replyDM(ctx, "You've already cleared this room. "+continueHint(ctx.Sender))
		default:
			return p.replyDM(ctx, "This fight is already over. `!zone status` for where you stand.")
		}
	}

	zone := zoneOrFallback(run.ZoneID)
	isBoss := roomType == RoomBoss
	var monster DnDMonsterTemplate
	var ok bool
	if isBoss {
		monster, ok = dndBestiary[zone.Boss.BestiaryID]
	} else {
		monster, ok = pickZoneEnemy(zone, run.RunID, run.CurrentRoom, true)
	}
	if !ok || monster.ID == "" {
		// monster.ID == "" guards a malformed bestiary entry (e.g. one whose
		// ID field was dropped): startCombatSession would otherwise persist a
		// session with an empty EnemyID, and the turn engine — having no enemy
		// to resolve — spins inertly until autoDriveCombat's round cap. Fail
		// loudly instead of stalling.
		return p.replyDM(ctx, "_(No bestiary entry for this encounter — file a bug. `!zone abandon` to bail.)_")
	}

	// Seat the whole party, leader first. A solo player is a one-seat roster and
	// takes the path they always took: one build, one INSERT, no participant rows.
	seats, enemy, senderSkip, refusal := p.buildFightSeats(ctx.Sender, roster, monster, int(zone.Tier), run.DMMood, run)
	if refusal != "" {
		return p.replyDM(ctx, refusal)
	}
	// The persisted session scales enemy HP for a party (solo scales by 1.0, so
	// this is a no-op there); mirror it here so the entry banner and the opening
	// round resolve against the same ceiling startPartyCombatSession persisted and
	// the rebuilt rounds use.
	enemyHP := scaledEnemyMaxHP(enemy.Stats.MaxHP, seatSetupWeight(seats))

	// Fight-start one-shot resources (Abjuration Arcane Ward, etc.) are seeded
	// per seat onto the session and its participant rows, so they survive the
	// turn engine's resume/commit cycle. The pet rolls per-turn inside the
	// engine, so there's no fight-start roll.
	sess, err := p.startPartyCombatSession(run.RunID, encID, monster.ID, enemy, seats)
	if err != nil {
		if err == ErrCombatSessionAlreadyActive {
			return p.replyDM(ctx, "You're already in a fight. Finish it with `!attack` / `!flee`.")
		}
		return p.replyDM(ctx, "Couldn't start the fight: "+err.Error())
	}

	// Layer-2 boss state that is spent mid-fight (Valdris's Phylactery Verses)
	// is seeded onto the fresh session ONCE, from the route the player walked to
	// get here — not on the per-round rebuild, which would resurrect spent
	// rebirths. enemyHP is the party-scaled pool persisted above. No-op for every
	// non-hooked enemy.
	if seedBossRunStatuses(sess, monster.ID, enemyHP, run) {
		if err := saveCombatSession(sess); err != nil {
			return p.replyDM(ctx, "Couldn't start the fight: "+err.Error())
		}
	}

	var b strings.Builder
	if isBoss {
		if line := composeBossEntry(zone.ID, run.RunID, run.CurrentRoom); line != "" {
			b.WriteString(line + "\n\n")
		}
		b.WriteString(fmt.Sprintf("👑 **Boss — %s** (HP %d, AC %d)\n", monster.Name, enemyHP, enemy.Stats.AC))
	} else {
		if line := eliteRoomEntryLine(zone.ID, run.RunID, run.CurrentRoom); line != "" {
			b.WriteString(line + "\n\n")
		}
		b.WriteString(fmt.Sprintf("⚔️ **Elite — %s** (HP %d, AC %d)\n", monster.Name, enemyHP, enemy.Stats.AC))
	}

	if sess.IsParty() {
		players := seatCombatants(seats)
		// Align the in-memory template with the scaled HP persisted above, so the
		// opening-round settle resolves the enemy against the same MaxHP the
		// rebuilt rounds do (regen clamp, bloodied-ability threshold). The persist
		// already happened off the unscaled value, so this does not double-scale.
		enemy.Stats.MaxHP = enemyHP
		// The enemy may have won initiative. Resolve everything the round owes
		// before anyone is asked to act, so the opening block narrates the hit
		// rather than showing its damage with no explanation.
		opening, serr := settleCombatSession(sess, players, enemy)
		if serr != nil {
			return p.replyDM(ctx, "Couldn't resolve the round: "+serr.Error())
		}
		ct := &combatTurn{sess: sess, players: players, enemy: enemy, seat: 0, uid: roster[0]}
		outcomes := p.closePartyRound(ct)
		if !ctx.Silent {
			// The opening block is per-reader for the same reason a round's
			// narration is: "You: 40/40 HP" has to be the reader's own pool.
			p.announcePartyFightStart(sess, players, enemy, b.String(), opening, outcomes)
		}
		// A member the roster left behind is owed an answer of their own: nothing
		// above was addressed to them, because they are not seated.
		if senderSkip != "" {
			return p.replyDM(ctx, senderSkip)
		}
		return nil
	}

	// One seat. Usually that is a solo player fighting their own fight, and this
	// is the block they have always been sent. It can also be a leader whose only
	// companion was left behind — in which case the block is the leader's and the
	// sender is owed the reason they are not in it.
	owner := id.UserID(sess.UserID)
	b.WriteString(fmt.Sprintf("You: **%d/%d HP**.\n", sess.PlayerHP, sess.PlayerHPMax))
	if curios := activeMagicItemsLine(owner); curios != "" {
		b.WriteString(curios)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(combatTurnPrompt(sess))
	if senderSkip != "" {
		if !ctx.Silent {
			if err := p.SendDM(owner, b.String()); err != nil {
				slog.Error("combat: fight-start DM to leader failed", "user", owner, "err", err)
			}
		}
		return p.replyDM(ctx, senderSkip)
	}
	return p.replyDM(ctx, b.String())
}

// ── !attack / !flee ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleAttackCmd(ctx MessageContext) error {
	return p.handleCombatActionCmd(ctx, PlayerAction{Kind: ActionAttack})
}

func (p *AdventurePlugin) handleFleeCmd(ctx MessageContext) error {
	return p.handleCombatActionCmd(ctx, PlayerAction{Kind: ActionFlee})
}

const noFightMsg = "You're not in a fight. `!fight` at an Elite or Boss room to start one."

func (p *AdventurePlugin) handleCombatActionCmd(ctx MessageContext, action PlayerAction) error {
	ct, release, msg := p.beginCombatTurn(ctx.Sender, noFightMsg)
	if ct == nil {
		return p.replyDM(ctx, msg)
	}
	defer release()

	// Fleeing ends the run for the whole party, so it is the leader's call —
	// the same reasoning that makes `!extract` leader-only.
	if action.Kind == ActionFlee && ct.isParty() && ct.seat != 0 {
		return p.replyDM(ctx, "Only your party leader can break off a fight.")
	}

	events, err := p.driveCombatRound(ct, action)
	if err != nil {
		return p.replyDM(ctx, "Couldn't resolve the round: "+err.Error())
	}
	return p.replyCombatRound(ctx, ct, events)
}

// replyCombatRound narrates a resolved round. A solo fight answers the one
// player who typed, exactly as it always has. A party fight fans out: every
// member gets the play-by-play with the right names on it, and their own footer
// or close-out. Terminal side effects run either way — a silent autopilot round
// still owes the party its XP and loot.
func (p *AdventurePlugin) replyCombatRound(ctx MessageContext, ct *combatTurn, events []CombatEvent) error {
	if !ct.isParty() {
		return p.replyDM(ctx, p.renderRoundResult(ctx.Sender, ct.sess, events, ct.players[0].Name, *ct.enemy))
	}
	outcomes := p.closePartyRound(ct)
	if ctx.Silent {
		return nil
	}
	p.announcePartyRound(ct, events, "", outcomes)
	return nil
}

// renderRoundResult turns a resolved round into the player-facing block: the
// play-by-play, then either the HP/turn-prompt footer (fight continues) or the
// close-out block (terminal status). Shared by !attack/!flee, !cast, !consume.
func (p *AdventurePlugin) renderRoundResult(userID id.UserID, sess *CombatSession, events []CombatEvent, playerName string, enemy Combatant) string {
	var b strings.Builder
	b.WriteString(RenderTurnRound(events, playerName, enemy.Name))
	if sess.IsActive() {
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("You: **%d/%d**  ·  %s: **%d/%d**\n",
			sess.PlayerHP, sess.PlayerHPMax, enemy.Name, sess.EnemyHP, sess.EnemyHPMax))
		b.WriteString(combatTurnPrompt(sess))
		return b.String()
	}
	b.WriteString("\n\n")
	b.WriteString(p.finishCombatSession(userID, sess, enemy))
	return b.String()
}

// runCombatRound resolves one full round of a solo fight: the player's chosen
// action, then the enemy turn and the round-end status tick, advancing the
// session until it is back at a player_turn or has reached a terminal status.
// Returns every event the round produced. Each advance persists the session, so
// a crash mid-round resumes cleanly from the last phase.
func runCombatRound(sess *CombatSession, player, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	return runPartyCombatRound(sess, []*Combatant{player}, enemy, action)
}

// partyRoundStepCap bounds the drain loop below. A round is at most one step per
// seat plus the enemy turn and the round-end tick, so a 3-player party settles
// in 5; the cap only turns a hypothetical non-advancing phase into a loud error
// instead of a hung goroutine holding the fight's lock.
const partyRoundStepCap = 64

// runPartyCombatRound resolves the acting seat's action and then drains every
// phase after it that needs no human: the enemy turn, the round-end status tick,
// and any seat that is down (which forfeits its turn silently). It comes to rest
// on a standing player's turn, or on a terminal status.
//
// A latched-onto-autopilot seat is *not* drained here — resolving its turn means
// running the picker, which needs the plugin to reach the character's spells and
// inventory. driveCombatRound layers that on top.
//
// For a solo roster this is exactly the old loop: a solo player_turn always
// belongs to a standing player, since a downed one has already ended the fight.
func runPartyCombatRound(sess *CombatSession, players []*Combatant, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	events, err := advancePartyCombatSession(sess, players, enemy, action)
	if err != nil {
		return events, err
	}
	more, err := settleCombatSession(sess, players, enemy)
	return append(events, more...), err
}

// settleCombatSession drains every phase the engine can resolve without a human:
// the enemy turn, the round-end status tick, and any seat that is down. It comes
// to rest on a standing player's turn, or on a terminal status.
//
// It is a no-op on a session already parked on a standing player's turn, which
// is where every solo fight sits between commands.
func settleCombatSession(sess *CombatSession, players []*Combatant, enemy *Combatant) ([]CombatEvent, error) {
	var events []CombatEvent
	for i := 0; i < partyRoundStepCap; i++ {
		if !sess.IsActive() {
			return events, nil
		}
		if seat, waiting := actingSeat(sess, players, enemy); waiting && sess.seatAlive(seat) {
			return events, nil
		}
		more, merr := advancePartyCombatSession(sess, players, enemy, PlayerAction{})
		if merr != nil {
			return events, merr
		}
		events = append(events, more...)
	}
	return events, fmt.Errorf("combat session %s: round did not settle within %d steps",
		sess.SessionID, partyRoundStepCap)
}

// combatTurnPrompt is the "your move" footer shown after every non-terminal
// round and on the opening !fight message.
func combatTurnPrompt(sess *CombatSession) string {
	return fmt.Sprintf("**Round %d.** Your move — `!attack`, `!cast <spell>`, `!consume <item>`, or `!flee`.", sess.Round)
}

// ── close-out ───────────────────────────────────────────────────────────────

// continueHint returns the verb the player uses to keep moving after a
// manual Elite/Boss fight, phrased for their current mode. On an
// expedition the autopilot drives the walk, so `!zone advance` is the
// wrong surface — point them at `!expedition run` instead. Standalone
// zone runs still advance with `!zone advance`.
func continueHint(userID id.UserID) string {
	exp, isLeader, err := activeExpeditionFor(userID)
	switch {
	case err != nil || exp == nil:
		return "`!zone advance` to move on."
	case !isLeader:
		return "Your leader marches the party on."
	}
	return "`!expedition run` to keep going."
}

// finishCombatSession runs the post-fight side effects once a CombatSession
// has reached a terminal status, and returns the player-facing outcome block.
// The graph is NOT advanced here: the terminal session row is the record that
// the room's manual combat is done, and a fresh !zone advance clears the room.
func (p *AdventurePlugin) finishCombatSession(userID id.UserID, sess *CombatSession, enemy Combatant) string {
	persistDnDHPAfterCombat(userID, sess.PlayerHP)
	// Achievements and post-combat subclass state are owed on every terminal
	// status, not just a win — a Berserker who rages and loses still comes out
	// of it exhausted. The auto-resolve close-outs have always done this.
	p.postCombatBookkeepingForSeat(sess, 0)

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

	// nat20/nat1 mood deltas from the whole fight's event log.
	scanMoodEventsFromEvents(sess.RunID, sess.TurnLog)

	var b strings.Builder
	switch sess.Status {
	case CombatStatusWon:
		tier := 1
		if run != nil {
			tier = int(zone.Tier)
		}
		bossOnExpedition := p.applyOwnerWinEffects(userID, sess.EnemyID, elite)
		p.grantSeatWinXP(userID, sess.PlayerHP, sess.PlayerHPMax, monster, tier)
		if line := twinBeeLine(zone.ID, DMCombatEnd, sess.RunID, cadence); line != "" {
			b.WriteString(line + "\n")
		}
		emoji := "✅"
		if !elite {
			emoji = "🏆"
		}
		b.WriteString(fmt.Sprintf("%s **%s** down. You finished at **%d/%d HP**.\n",
			emoji, enemy.Name, sess.PlayerHP, sess.PlayerHPMax))
		if drop := p.dropZoneLoot(userID, zone.ID, monster, !elite, elite); drop != "" {
			b.WriteString(drop + "\n")
		}
		if !elite {
			writeBossEpilogue(&b, zone.ID)
		}
		if bossOnExpedition {
			// The boss is the expedition's climax. Frame the close-out as
			// the win rather than a "keep walking" nudge. One more
			// `!expedition run` walks out the cleared room and triggers
			// finalizeExpeditionOnZoneClear (rewards + status flip).
			b.WriteString("🎉 **Zone cleared — the expedition is won.** `!expedition run` to march out and claim your spoils.")
		} else {
			b.WriteString(continueHint(userID))
		}

	case CombatStatusLost:
		endRunOnLoss(userID, sess.RunID, true)
		markAdventureDead(userID, "zone", zone.Display)
		if line := twinBeeLine(zone.ID, DMPlayerDeath, sess.RunID, cadence); line != "" {
			b.WriteString(line + "\n")
		}
		b.WriteString(fmt.Sprintf("💀 You fell to **%s**. Run ended.", enemy.Name))

	case CombatStatusFled:
		// Flee = run ends, light penalty: wounds persist (HP already saved),
		// but no death timer. Chosen candidate from the migration plan's
		// open question on flee outcome.
		endRunOnLoss(userID, sess.RunID, false)
		b.WriteString(fmt.Sprintf("🏃 You broke off from **%s** and slipped away. Run ended — you keep your wounds, but you live.", enemy.Name))

	default:
		b.WriteString("The fight is over.")
	}
	return b.String()
}

// ── !cast (in-combat) ───────────────────────────────────────────────────────
//
// handleDnDCastCmd routes here when the player has an active CombatSession:
// !cast resolves as the player's turn for the round instead of queuing for
// "next combat". Out-of-combat !cast is unchanged.

// parseCombatCast validates a !cast invocation for a caster mid-fight and
// returns the spell plus the resolved slot level. errMsg is non-empty and
// player-facing on any validation failure. It performs NO resource spend —
// the caller debits the slot only once the round is about to resolve.
//
// It takes the seat, not just the user, because "do you know this spell" is a
// question about a combatant and only *usually* a question about a database row:
// the hired companion has no rows and answers it from his synthetic sheet.
func parseCombatCast(sess *CombatSession, seat int, userID id.UserID, c *DnDCharacter, args string) (SpellDefinition, int, string) {
	tokens := strings.Fields(args)
	upcast := 0
	var spellTokens []string
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "--upcast":
			if i+1 < len(tokens) {
				if n, err := strconv.Atoi(tokens[i+1]); err == nil {
					upcast = n
				}
				i++
			}
		case "--target":
			if i+1 < len(tokens) {
				i++
			}
		default:
			spellTokens = append(spellTokens, tokens[i])
		}
	}
	if len(spellTokens) == 0 {
		return SpellDefinition{}, 0, "Usage: `!cast <spell> [--upcast N]` — casts as your turn this round. `!spells` lists options."
	}
	spell, ok := parseSpell(strings.Join(spellTokens, " "))
	if !ok {
		return SpellDefinition{}, 0, fmt.Sprintf("Unknown spell %q. Run `!spells` to list what you know.", strings.Join(spellTokens, " "))
	}
	// Class gate — Arcane Trickster Rogues use the Mage list.
	effectiveClass := c.Class
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
		effectiveClass = ClassMage
	}
	classOK := false
	for _, cl := range spell.Classes {
		if cl == effectiveClass {
			classOK = true
			break
		}
	}
	if !classOK {
		return SpellDefinition{}, 0, fmt.Sprintf("%s is not on the %s spell list.", spell.Name, titleClass(c.Class))
	}
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
		if mx := highestAvailableSlotForChar(c); spell.Level > mx {
			return SpellDefinition{}, 0, fmt.Sprintf("At level %d, your Arcane Trickster magic only reaches level-%d spells.", c.Level, mx)
		}
	}
	known, prepared, err := seatKnowsSpell(sess, seat, userID, spell.ID)
	if err != nil {
		return SpellDefinition{}, 0, "Couldn't check your spell list."
	}
	if !known {
		return SpellDefinition{}, 0, fmt.Sprintf("You don't know %s yet.", spell.Name)
	}
	if !prepared {
		return SpellDefinition{}, 0, fmt.Sprintf("%s isn't prepared today. Run `!prepare %s` first.", spell.Name, spell.ID)
	}
	slotLevel := spell.Level
	if upcast > slotLevel && spell.Level > 0 {
		slotLevel = upcast
	}
	if slotLevel < 0 || slotLevel > 5 {
		return SpellDefinition{}, 0, "Slot level out of range (1–5)."
	}
	return spell, slotLevel, ""
}

func (p *AdventurePlugin) handleCombatCastCmd(ctx MessageContext, args string) error {
	// "not in a fight anymore" — this handler is only routed to when the caller
	// already saw an active session, so a miss here means it closed under them.
	ct, release, msg := p.beginCombatTurn(ctx.Sender, "You're not in a fight anymore.")
	if ct == nil {
		return p.replyDM(ctx, msg)
	}
	defer release()

	action, settle, errMsg := p.castActionForSeat(ct, ct.seat, args)
	if errMsg != "" {
		return p.replyDM(ctx, errMsg)
	}
	events, err := p.driveCombatRound(ct, action)
	settle(err == nil)
	if err != nil {
		return p.replyDM(ctx, "Couldn't resolve the round: "+err.Error())
	}
	return p.replyCombatRound(ctx, ct, events)
}

// castActionForSeat resolves a `!cast` for one seat into a PlayerAction, having
// already spent the slot and any material component. It is shared by the command
// handler and by the autopilot that plays an absent member's turn, so both spend
// resources through exactly one code path.
//
// The returned settle(ok) must be called once the round has been attempted: on
// failure it refunds the slot. On refusal (non-empty msg) nothing was spent.
//
// A buff spell folds its delta into *this seat's* persisted statuses and rebuilds
// the roster so the buff is live for the round's enemy turn. Before P5 that
// delta went to the session's embedded copy — seat 0 — so a party member
// buffing themselves would have buffed the leader.
// splitCastTarget peels an optional ally target off the end of a `!cast` argument
// — `cure wounds @alex`, or just `cure wounds alex` — and resolves it to a seat.
//
// It resolves against the people *in this fight* rather than the room, which is
// both cheaper (no ResolveUser round-trip, no RoomID to thread down here) and
// more correct: the only legal target of a combat heal is somebody sitting in the
// combat. Anything it does not recognise is left on the string for the spell
// parser, so `!cast cure wounds` and `!cast fireball 3` behave exactly as before.
//
// Both spellings work: the `--target @alex` flag that `!help` has advertised all
// along (and that parseCombatCast has been silently swallowing since SP2 —
// "reserved for SP3, accept and ignore"), and a plain trailing `@alex`.
//
// Returns (remainingArgs, seat, errMsg). seat is -1 when no target was named.
func splitCastTarget(ct *combatTurn, caster int, args string) (string, int, string) {
	args = strings.TrimSpace(args)
	if args == "" || !ct.isParty() {
		return args, -1, ""
	}
	fields := strings.Fields(args)

	// `--target <who>` anywhere in the string.
	explicit, name := false, ""
	for i := 0; i < len(fields); i++ {
		if !strings.EqualFold(fields[i], "--target") {
			continue
		}
		if i+1 >= len(fields) {
			return args, -1, "`--target` needs a name: `!cast cure wounds --target @alex`."
		}
		explicit, name = true, strings.TrimPrefix(fields[i+1], "@")
		fields = append(fields[:i], fields[i+2:]...)
		break
	}

	if name == "" {
		last := fields[len(fields)-1]
		// A bare number is a slot level (`!cast fireball 3`), never a target.
		if _, err := strconv.Atoi(last); err == nil {
			return args, -1, ""
		}
		explicit = strings.HasPrefix(last, "@")
		name = strings.TrimPrefix(last, "@")
		if name == "" {
			return args, -1, ""
		}
		fields = fields[:len(fields)-1]
	}
	// From here `fields` is the spell tokens with the target removed.
	rest := strings.Join(fields, " ")

	for i, c := range ct.players {
		uid := ct.sess.seatUserID(i)
		if !strings.EqualFold(c.Name, name) &&
			!strings.EqualFold(uid, name) &&
			!strings.EqualFold(id.UserID(uid).Localpart(), name) {
			continue
		}
		if i == caster {
			// Targeting yourself is just casting it on yourself, which is what the
			// engine does by default. Drop the target and carry on.
			return rest, -1, ""
		}
		return rest, i, ""
	}
	// An explicit @mention that matches nobody in the fight is a mistake worth
	// naming — silently casting it on yourself would waste the slot.
	if explicit {
		return args, -1, fmt.Sprintf("**%s** isn't in this fight. Cast it on someone who is.", name)
	}
	return args, -1, ""
}

func (p *AdventurePlugin) castActionForSeat(ct *combatTurn, seat int, args string) (PlayerAction, func(bool), string) {
	noop := func(bool) {}
	uid := id.UserID(ct.sess.seatUserID(seat))

	// §1 — a heal may name somebody else in the fight: `!cast cure wounds @alex`.
	// Split the target off before the spell is parsed, so the spell parser sees
	// the same string it always has.
	args, targetSeat, targetErr := splitCastTarget(ct, seat, args)
	if targetErr != "" {
		return PlayerAction{}, noop, targetErr
	}

	c, err := p.seatCastSheet(ct.sess, uid)
	if err != nil || c == nil {
		return PlayerAction{}, noop, "Couldn't load your Adv 2.0 sheet."
	}
	if !isSpellcaster(c) {
		return PlayerAction{}, noop, fmt.Sprintf(
			"%s isn't a caster class. `!attack` or `!consume <item>` instead.", titleClass(c.Class))
	}

	spell, slotLevel, errMsg := parseCombatCast(ct.sess, seat, uid, c, strings.TrimSpace(args))
	if errMsg != "" {
		return PlayerAction{}, noop, errMsg
	}
	if spell.Effect == EffectReaction {
		return PlayerAction{}, noop, fmt.Sprintf(
			"%s is a reaction spell — those still aren't usable. Pick a damage, healing, or control spell.", spell.Name)
	}

	refund := func(ok bool) {
		if !ok && spell.Level > 0 {
			_ = refundSeatSlot(ct.sess, seat, uid, slotLevel)
		}
	}

	var eff *turnActionEffect
	if spell.Effect == EffectBuffSelf || spell.Effect == EffectBuffAlly {
		// Buff path — resolve the buff against a throwaway combatant, fold the
		// marginal effect into that seat's persisted state, then rebuild the
		// roster so the buff is live for this round's enemy turn.
		player := ct.players[seat]
		as, am := player.Stats, player.Mods
		applySpellBuff(spell, c, &as, &am, slotLevel)
		d := diffTurnBuff(player.Stats, as, player.Mods, am)
		if !d.any() {
			return PlayerAction{}, noop, fmt.Sprintf(
				"%s has no effect the turn-based engine can apply yet.", spell.Name)
		}
		if msg := p.chargeSpellCost(ct.sess, seat, uid, spell, slotLevel); msg != "" {
			return PlayerAction{}, noop, msg
		}
		ct.sess.actorStatusesPtr(seat).applyBuffDelta(d)
		if rerr := p.rebuildRoster(ct); rerr != nil {
			refund(false)
			return PlayerAction{}, noop, "Couldn't rebuild the fight: " + rerr.Error()
		}
		label := spell.Name + " — active"
		if d.heal > 0 {
			label = fmt.Sprintf("%s — +%d HP", spell.Name, d.heal)
		}
		eff = &turnActionEffect{
			Action: "spell_cast", Label: label,
			PlayerHeal: d.heal, EnemySkip: d.enemySkip,
		}
	} else {
		out, supported := resolveTurnSpell(c, spell, slotLevel, &ct.enemy.Stats)
		if !supported {
			return PlayerAction{}, noop, fmt.Sprintf(
				"%s is a utility spell — those aren't usable in turn-based fights yet. Try a damage, healing, control, or buff spell.", spell.Name)
		}
		if msg := p.chargeSpellCost(ct.sess, seat, uid, spell, slotLevel); msg != "" {
			return PlayerAction{}, noop, msg
		}
		// Park the Necromancy kill-heal stash on the casting seat. The
		// auto-resolve path keeps it on the fight-start CombatModifiers, which
		// a turn-based fight has nowhere to hold — it rebuilds its combatants
		// every round. Only a damaging cast stashes (a miss leaves the slot at
		// 0), and each one overwrites the last, so the stash always describes
		// the seat's most recent landed spell — which is the only one that can
		// have been lethal by the time the close-out reads it.
		if out.GrimHarvestSlot > 0 {
			as := ct.sess.actorStatusesPtr(seat)
			as.GrimHarvestSlot = out.GrimHarvestSlot
			as.GrimHarvestNecrotic = out.GrimHarvestNecrotic
		}
		eff = &turnActionEffect{
			Label:       out.Desc,
			Action:      "spell_cast",
			EnemyDamage: out.EnemyDamage,
			PlayerHeal:  out.PlayerHeal,
			EnemySkip:   out.EnemySkip,
		}
		// Revive the arcane-blaster sustained cantrip floor in the turn engine.
		// combat_engine.go:590 deals CantripPerRound flat every round, but that
		// path is the swing-based engine (SimulateCombat). Auto-resolve — the
		// live path for every expedition — runs the turn engine, where a caster
		// autocasts and never weapon-swings, so the floor was dead code: casters
		// fought at bare cantrip dice (~4d10≈22 at L20). Lift LANDED cantrip
		// damage to the floor, but only when the cast already connected
		// (eff.EnemyDamage > 0) — a whiffed Fire Bolt still whiffs, so the ~35%
		// miss variance survives and the floor doesn't become a guaranteed flat
		// hammer (that overshot to ~99%). Damage cantrips only; slot spells keep
		// their rolled damage. Only Mage/Sorcerer/Warlock carry a nonzero
		// CantripPerRound, so this is self-targeting.
		if spell.Level == 0 && eff.EnemyDamage > 0 &&
			(spell.Effect == EffectDamageAttack || spell.Effect == EffectDamageSave || spell.Effect == EffectDamageAuto) {
			if floor := ct.players[seat].Mods.CantripPerRound; floor > eff.EnemyDamage {
				eff.EnemyDamage = floor
			}
		}
		// §1 — redirect the heal onto the named ally. The roll is the same; only
		// the body it lands on changes. This is the line that makes a cleric a
		// cleric: until it existed, every heal in the engine was a self-heal, and
		// the class whose whole job is keeping other people upright could not put
		// a single hit point on a friend.
		if targetSeat >= 0 && targetSeat != seat {
			if out.PlayerHeal <= 0 {
				return PlayerAction{}, refund, fmt.Sprintf(
					"%s isn't something you can cast on someone else. Drop the target to cast it yourself.", spell.Name)
			}
			eff.AllyHeal, eff.AllySeat = out.PlayerHeal, targetSeat
			eff.PlayerHeal = 0
			eff.Label = fmt.Sprintf("%s → %s (+%d HP)", spell.Name, ct.players[targetSeat].Name, out.PlayerHeal)
		}
		// Concentration AOE damage spells linger: the burst lands this round
		// (EnemyDamage) and the same value re-ticks every round_end after, via
		// the engine's concentration aura. spiritual_weapon already covers the
		// cleric's bonus-action half of the combo; this restores the action half.
		if spell.Concentration &&
			(spell.Effect == EffectDamageAuto || spell.Effect == EffectDamageSave) {
			eff.ConcentrationDmg = out.EnemyDamage
		}
	}
	return PlayerAction{Kind: ActionCast, Effect: eff}, refund, ""
}

// rebuildRoster re-derives the seated combatants after a mid-fight buff changed
// a seat's persisted statuses, so the buff is live for the rest of the round.
func (p *AdventurePlugin) rebuildRoster(ct *combatTurn) error {
	players, enemy, err := p.partyCombatantsForSession(ct.sess)
	if err != nil {
		return err
	}
	ct.players, ct.enemy = players, enemy
	return nil
}

// chargeSpellCost debits a spell's material component and leveled slot for a
// turn-based cast. It returns a non-empty player-facing message on failure; on
// success the caller owns the slot and must refundSpellSlot if the round itself
// errors. Material components (rare in a fight) are not refunded if the slot
// debit then fails — matching the auto-resolve cast path.
func (p *AdventurePlugin) chargeSpellCost(sess *CombatSession, seat int, userID id.UserID, spell SpellDefinition, slotLevel int) string {
	_, _, companion := seatCompanionLoadout(sess, userID)

	// The companion carries no purse — he has no wallet to debit and no inventory
	// to stock one from, so a component cost is not a price he can pay but a spell
	// he cannot cast. Refusing here (rather than letting the debit fail on an empty
	// account) keeps that an explicit rule instead of an accident of his balance.
	if spell.MaterialCost > 0 {
		if companion {
			return fmt.Sprintf("%s needs a component %s doesn't carry.", spell.Name, companionDisplayName)
		}
		if p.euro == nil || !p.euro.Debit(userID, float64(spell.MaterialCost), "dnd_spell_component") {
			return fmt.Sprintf("%s needs a %d-coin diamond component you can't afford.", spell.Name, spell.MaterialCost)
		}
	}
	if spell.Level > 0 {
		ok, serr := consumeSeatSlot(sess, seat, userID, slotLevel)
		if serr != nil {
			return "Couldn't consume slot: " + serr.Error()
		}
		if !ok {
			if companion {
				return fmt.Sprintf("%s is out of level-%d energy.", companionDisplayName, slotLevel)
			}
			return fmt.Sprintf("You're out of level-%d energy. %s", slotLevel, renderSlotsBrief(userID))
		}
	}
	return ""
}

// ── !consume (in-combat) ────────────────────────────────────────────────────
//
// !consume <item> spends one combat consumable as the player's turn. Heal and
// flat-damage items resolve fully within the round; buff-type items (ward,
// atk/def boost, spore, reflect, auto-crit) fold into the session's persisted
// fight-scoped state and carry for the rest of the fight.

// matchConsumable resolves a player-typed name against the player's consumable
// inventory: case-insensitive exact match first, then a unique prefix match.
func matchConsumable(inv []ConsumableItem, query string) (*ConsumableItem, string) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, ""
	}
	var prefix []*ConsumableItem
	for i := range inv {
		name := strings.ToLower(inv[i].Def.Name)
		if name == q {
			return &inv[i], ""
		}
		if strings.HasPrefix(name, q) {
			prefix = append(prefix, &inv[i])
		}
	}
	switch len(prefix) {
	case 0:
		return nil, ""
	case 1:
		return prefix[0], ""
	default:
		names := make([]string, len(prefix))
		for i, c := range prefix {
			names[i] = c.Def.Name
		}
		return nil, "Ambiguous — matches: " + strings.Join(names, ", ") + "."
	}
}

func (p *AdventurePlugin) handleConsumeCmd(ctx MessageContext, args string) error {
	const notFighting = "`!consume <item>` spends an item on your turn in an Elite or Boss fight. You're not in one — outside combat, consumables fire automatically."

	args = strings.TrimSpace(args)
	if args == "" {
		// Listing the pack reads no turn state, so it neither takes the fight's
		// lock nor settles a phase — a player peeking at their options between
		// rounds must not advance the fight.
		probe, err := activeCombatSessionFor(ctx.Sender)
		if err != nil {
			return p.replyDM(ctx, "Couldn't read combat state: "+err.Error())
		}
		if probe == nil {
			return p.replyDM(ctx, notFighting)
		}
		inv := p.loadConsumableInventory(ctx.Sender)
		if len(inv) == 0 {
			return p.replyDM(ctx, "You have no combat consumables. `!attack` / `!cast` / `!flee`.")
		}
		names := make([]string, len(inv))
		for i, c := range inv {
			names[i] = c.Def.Name
		}
		return p.replyDM(ctx, "Usage: `!consume <item>`. You're carrying: "+strings.Join(names, ", ")+".")
	}

	ct, release, msg := p.beginCombatTurn(ctx.Sender, notFighting)
	if ct == nil {
		return p.replyDM(ctx, msg)
	}
	defer release()

	action, settle, errMsg := p.consumeActionForSeat(ct, ct.seat, args)
	if errMsg != "" {
		return p.replyDM(ctx, errMsg)
	}
	events, err := p.driveCombatRound(ct, action)
	settle(err == nil)
	if err != nil {
		return p.replyDM(ctx, "Couldn't resolve the round: "+err.Error())
	}
	return p.replyCombatRound(ctx, ct, events)
}

// consumeActionForSeat resolves a `!consume` for one seat into a PlayerAction.
// Shared by the command handler and by the autopilot that plays an absent
// member's turn.
//
// The returned settle(ok) burns the item only once the round has resolved: a
// removal failure leaves the player a free use (logged, rare), which beats
// charging them for a round that errored out.
func (p *AdventurePlugin) consumeActionForSeat(ct *combatTurn, seat int, args string) (PlayerAction, func(bool), string) {
	noop := func(bool) {}
	uid := id.UserID(ct.sess.seatUserID(seat))

	inv := p.loadConsumableInventory(uid)
	item, ambig := matchConsumable(inv, args)
	if ambig != "" {
		return PlayerAction{}, noop, ambig
	}
	if item == nil {
		return PlayerAction{}, noop, fmt.Sprintf("No consumable matching %q in your inventory.", args)
	}

	def := item.Def
	eff := &turnActionEffect{Action: "use_consumable"}
	switch def.Effect {
	case EffectHeal:
		eff.PlayerHeal = int(def.Value)
		eff.Label = fmt.Sprintf("%s — +%d HP", def.Name, eff.PlayerHeal)
	case EffectFlatDmg:
		eff.EnemyDamage = int(def.Value)
		eff.Label = fmt.Sprintf("%s — %d damage", def.Name, eff.EnemyDamage)
	default:
		// Buff-type consumable — resolve the marginal effect against a
		// throwaway combatant, fold it into that seat's persisted state, and
		// rebuild the roster so the buff is live for this round's enemy turn.
		player := ct.players[seat]
		as, am := player.Stats, player.Mods
		ApplyConsumableMods(&as, &am, []ConsumableItem{{Def: def}})
		d := diffTurnBuff(player.Stats, as, player.Mods, am)
		if !d.any() {
			return PlayerAction{}, noop, fmt.Sprintf(
				"**%s** has no effect the turn-based engine can apply yet.", def.Name)
		}
		ct.sess.actorStatusesPtr(seat).applyBuffDelta(d)
		if rerr := p.rebuildRoster(ct); rerr != nil {
			return PlayerAction{}, noop, "Couldn't rebuild the fight: " + rerr.Error()
		}
		eff.Label = def.Name + " — active"
		eff.PlayerHeal = d.heal
		eff.EnemySkip = d.enemySkip
	}

	burn := func(ok bool) {
		if !ok {
			return
		}
		if rerr := removeAdvInventoryItem(item.InventoryID); rerr != nil {
			slog.Error("combat: consume remove inventory item failed", "user", uid, "item", def.Name, "err", rerr)
		}
	}
	return PlayerAction{Kind: ActionConsume, Effect: eff}, burn, ""
}
