package plugin

import (
	"fmt"
	"log/slog"
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

// ── !fight ──────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleFightCmd(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>` first.")
	}
	roomType := run.CurrentRoomType()
	if roomType != RoomElite && roomType != RoomBoss {
		return p.SendDM(ctx.Sender, "Nothing to fight here — `!fight` is for Elite and Boss rooms. Use `!zone advance`.")
	}

	encID := encounterIDForRoom(run.CurrentRoom)
	if existing, _ := getCombatSessionForEncounter(run.RunID, encID); existing != nil {
		switch existing.Status {
		case CombatStatusActive:
			return p.SendDM(ctx.Sender, "You're already in this fight — `!attack` or `!flee`.")
		case CombatStatusWon:
			return p.SendDM(ctx.Sender, "You've already cleared this room. `!zone advance` to move on.")
		default:
			return p.SendDM(ctx.Sender, "This fight is already over. `!zone status` for where you stand.")
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
	if !ok {
		return p.SendDM(ctx.Sender, "_(No bestiary entry for this encounter — file a bug. `!zone abandon` to bail.)_")
	}

	_, enemy, _, err := p.buildZoneCombatants(ctx.Sender, monster, int(zone.Tier), run.DMMood)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't set up the fight: "+err.Error())
	}

	playerHP, playerMax := dndHPSnapshot(ctx.Sender)
	if playerHP <= 0 {
		return p.SendDM(ctx.Sender, "You're in no shape to fight. `!rest` first.")
	}
	enemyHP := enemy.Stats.MaxHP

	sess, err := startCombatSession(ctx.Sender, run.RunID, encID, monster.ID, playerHP, playerMax, enemyHP, enemyHP)
	if err != nil {
		if err == ErrCombatSessionAlreadyActive {
			return p.SendDM(ctx.Sender, "You're already in a fight. Finish it with `!attack` / `!flee`.")
		}
		return p.SendDM(ctx.Sender, "Couldn't start the fight: "+err.Error())
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
	b.WriteString(fmt.Sprintf("You: **%d/%d HP**.\n\n", playerHP, playerMax))
	b.WriteString(combatTurnPrompt(sess))
	return p.SendDM(ctx.Sender, b.String())
}

// ── !attack / !flee ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleAttackCmd(ctx MessageContext) error {
	return p.handleCombatActionCmd(ctx, PlayerAction{Kind: ActionAttack})
}

func (p *AdventurePlugin) handleFleeCmd(ctx MessageContext) error {
	return p.handleCombatActionCmd(ctx, PlayerAction{Kind: ActionFlee})
}

func (p *AdventurePlugin) handleCombatActionCmd(ctx MessageContext, action PlayerAction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	sess, err := getActiveCombatSession(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read combat state: "+err.Error())
	}
	if sess == nil {
		return p.SendDM(ctx.Sender, "You're not in a fight. `!fight` at an Elite or Boss room to start one.")
	}

	player, enemy, err := p.combatantsForSession(sess)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't rebuild the fight: "+err.Error())
	}

	events, err := runCombatRound(sess, &player, &enemy, action)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't resolve the round: "+err.Error())
	}

	var b strings.Builder
	b.WriteString(renderCombatRound(events, player.Name, enemy.Name))

	if sess.IsActive() {
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("You: **%d/%d**  ·  %s: **%d/%d**\n",
			sess.PlayerHP, sess.PlayerHPMax, enemy.Name, sess.EnemyHP, sess.EnemyHPMax))
		b.WriteString(combatTurnPrompt(sess))
		return p.SendDM(ctx.Sender, b.String())
	}

	b.WriteString("\n\n")
	b.WriteString(p.finishCombatSession(ctx.Sender, sess, enemy))
	return p.SendDM(ctx.Sender, b.String())
}

// runCombatRound resolves one full round: the player's chosen action, then the
// enemy turn and the round-end status tick, advancing the session until it is
// back at a player_turn or has reached a terminal status. Returns every event
// the round produced. Each advanceCombatSession call persists the session, so
// a crash mid-round resumes cleanly from the last phase.
func runCombatRound(sess *CombatSession, player, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	events, err := advanceCombatSession(sess, player, enemy, action)
	if err != nil {
		return events, err
	}
	for sess.IsActive() && sess.Phase != CombatPhasePlayerTurn {
		more, merr := advanceCombatSession(sess, player, enemy, PlayerAction{})
		if merr != nil {
			return events, merr
		}
		events = append(events, more...)
	}
	return events, nil
}

// combatTurnPrompt is the "your move" footer shown after every non-terminal
// round and on the opening !fight message.
func combatTurnPrompt(sess *CombatSession) string {
	return fmt.Sprintf("**Round %d.** Your move — `!attack` or `!flee`.", sess.Round)
}

// ── close-out ───────────────────────────────────────────────────────────────

// finishCombatSession runs the post-fight side effects once a CombatSession
// has reached a terminal status, and returns the player-facing outcome block.
// The graph is NOT advanced here: the terminal session row is the record that
// the room's manual combat is done, and a fresh !zone advance clears the room.
func (p *AdventurePlugin) finishCombatSession(userID id.UserID, sess *CombatSession, enemy Combatant) string {
	persistDnDHPAfterCombat(userID, sess.PlayerHP)

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
		recordZoneKillForUser(userID, sess.EnemyID)
		applyRoomCombatThreatForUser(userID, elite)
		// zoneCombatXP only reads PlayerWon + NearDeath off the result.
		nearDeath := sess.PlayerHPMax > 0 && sess.PlayerHP*5 < sess.PlayerHPMax
		tier := 1
		if run != nil {
			tier = int(zone.Tier)
		}
		if xp := zoneCombatXP(CombatResult{PlayerWon: true, NearDeath: nearDeath}, monster.CR, tier); xp > 0 {
			if _, err := p.grantDnDXP(userID, xp); err != nil {
				slog.Error("combat: grantDnDXP turn-based", "user", userID, "err", err)
			}
		}
		if !elite {
			// §8.1 — zone boss defeat drops expedition threat. Silent no-op
			// for standalone zone runs (no active expedition).
			if exp, eerr := getActiveExpedition(userID); eerr == nil && exp != nil {
				_ = applyBossDefeatThreat(exp.ID)
			}
		}
		if line := twinBeeLine(zone.ID, DMCombatEnd, sess.RunID, cadence); line != "" {
			b.WriteString(line + "\n")
		}
		emoji := "✅"
		if !elite {
			emoji = "🏆"
		}
		b.WriteString(fmt.Sprintf("%s **%s** down. You finished at **%d/%d HP**.\n",
			emoji, enemy.Name, sess.PlayerHP, sess.PlayerHPMax))
		if drop := p.dropZoneLoot(userID, zone.ID, monster, !elite); drop != "" {
			b.WriteString(drop + "\n")
		}
		b.WriteString("`!zone advance` to move on.")

	case CombatStatusLost:
		if run != nil {
			_, _ = applyMoodEvent(sess.RunID, MoodEventPlayerDeath)
		}
		_ = abandonZoneRun(userID)
		markAdventureDead(userID, "zone", zone.Display)
		if line := twinBeeLine(zone.ID, DMPlayerDeath, sess.RunID, cadence); line != "" {
			b.WriteString(line + "\n")
		}
		b.WriteString(fmt.Sprintf("💀 You fell to **%s**. Run ended.", enemy.Name))

	case CombatStatusFled:
		// Flee = run ends, light penalty: wounds persist (HP already saved),
		// but no death timer. Chosen candidate from the migration plan's
		// open question on flee outcome.
		_ = abandonZoneRun(userID)
		b.WriteString(fmt.Sprintf("🏃 You broke off from **%s** and slipped away. Run ended — you keep your wounds, but you live.", enemy.Name))

	default:
		b.WriteString("The fight is over.")
	}
	return b.String()
}

// ── rendering ───────────────────────────────────────────────────────────────

// renderCombatRound turns one round's events into a compact play-by-play
// block. Full TwinBee per-round narration is a later sub-phase; this is the
// functional MVP renderer.
func renderCombatRound(events []CombatEvent, playerName, enemyName string) string {
	var lines []string
	for _, ev := range events {
		if line := renderCombatRoundEvent(ev, playerName, enemyName); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "_The round passes without a clean blow landed._"
	}
	return strings.Join(lines, "\n")
}

func renderCombatRoundEvent(ev CombatEvent, playerName, enemyName string) string {
	switch ev.Actor {
	case "player":
		switch ev.Action {
		case "hit":
			return fmt.Sprintf("⚔️ %s lands a hit for **%d**.", playerName, ev.Damage)
		case "crit":
			return fmt.Sprintf("💥 %s **crits** for **%d**!", playerName, ev.Damage)
		case "block":
			return fmt.Sprintf("⚔️ %s pushes through the guard for **%d**.", playerName, ev.Damage)
		case "miss":
			return fmt.Sprintf("… %s swings and misses.", playerName)
		case "stunned":
			return fmt.Sprintf("😵 %s is stunned and loses the turn.", playerName)
		case "rage":
			return fmt.Sprintf("🔥 %s's blood is up — rage takes hold.", playerName)
		case "death_save":
			return fmt.Sprintf("✨ %s should be down — and isn't.", playerName)
		case "flee":
			return fmt.Sprintf("🏃 %s breaks off and runs.", playerName)
		}
	case "enemy":
		switch ev.Action {
		case "hit":
			return fmt.Sprintf("🩸 %s hits %s for **%d**.", enemyName, playerName, ev.Damage)
		case "crit":
			return fmt.Sprintf("🩸 %s **crits** %s for **%d**!", enemyName, playerName, ev.Damage)
		case "miss":
			return fmt.Sprintf("… %s attacks, but misses.", enemyName)
		case "poison_tick":
			return fmt.Sprintf("☠️ Poison saps **%d** from %s.", ev.Damage, playerName)
		case "poison":
			return fmt.Sprintf("☠️ %s's strike leaves a poison festering.", enemyName)
		case "enrage":
			return fmt.Sprintf("😡 %s flies into a rage.", enemyName)
		case "armor_break":
			return fmt.Sprintf("🛡️ %s cracks %s's armor.", enemyName, playerName)
		case "stun":
			return fmt.Sprintf("💫 %s lands a stunning blow.", enemyName)
		case "lifesteal":
			return fmt.Sprintf("🧛 %s drains life from the wound.", enemyName)
		case "cleave":
			return fmt.Sprintf("🪓 %s cleaves into %s for **%d**.", enemyName, playerName, ev.Damage)
		}
	}
	if ev.Damage > 0 {
		return fmt.Sprintf("• %s — %s (%d)", ev.Actor, ev.Action, ev.Damage)
	}
	return ""
}
