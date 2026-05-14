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
	return p.SendDM(ctx.Sender, p.renderRoundResult(ctx.Sender, sess, events, player.Name, enemy))
}

// renderRoundResult turns a resolved round into the player-facing block: the
// play-by-play, then either the HP/turn-prompt footer (fight continues) or the
// close-out block (terminal status). Shared by !attack/!flee, !cast, !consume.
func (p *AdventurePlugin) renderRoundResult(userID id.UserID, sess *CombatSession, events []CombatEvent, playerName string, enemy Combatant) string {
	var b strings.Builder
	b.WriteString(renderCombatRound(events, playerName, enemy.Name))
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
	return fmt.Sprintf("**Round %d.** Your move — `!attack`, `!cast <spell>`, `!consume <item>`, or `!flee`.", sess.Round)
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
		case "spell_cast":
			if ev.Desc != "" {
				return "✨ " + ev.Desc
			}
			return fmt.Sprintf("✨ %s casts a spell.", playerName)
		case "use_consumable":
			if ev.Desc != "" {
				return "🧪 " + ev.Desc
			}
			return fmt.Sprintf("🧪 %s uses an item.", playerName)
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
		case "spell_held":
			return fmt.Sprintf("🌀 %s is held fast — no attack this round.", enemyName)
		}
	}
	if ev.Damage > 0 {
		return fmt.Sprintf("• %s — %s (%d)", ev.Actor, ev.Action, ev.Damage)
	}
	return ""
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
func parseCombatCast(userID id.UserID, c *DnDCharacter, args string) (SpellDefinition, int, string) {
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
			return SpellDefinition{}, 0, fmt.Sprintf("Arcane Trickster L%d only has up to L%d slots.", c.Level, mx)
		}
	}
	known, prepared, err := playerKnowsSpell(userID, spell.ID)
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
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	sess, err := getActiveCombatSession(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read combat state: "+err.Error())
	}
	if sess == nil {
		// Race: the fight closed between the route check and the lock.
		return p.SendDM(ctx.Sender, "You're not in a fight anymore.")
	}

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 sheet.")
	}
	if !isSpellcaster(c) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s isn't a caster class. `!attack` or `!consume <item>` instead.", titleClass(c.Class)))
	}

	spell, slotLevel, errMsg := parseCombatCast(ctx.Sender, c, strings.TrimSpace(args))
	if errMsg != "" {
		return p.SendDM(ctx.Sender, errMsg)
	}
	if spell.Effect == EffectReaction {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is a reaction spell — those still aren't usable. Pick a damage, healing, or control spell.", spell.Name))
	}

	player, enemy, err := p.combatantsForSession(sess)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't rebuild the fight: "+err.Error())
	}

	out, supported := resolveTurnSpell(c, spell, slotLevel, &enemy.Stats)
	if !supported {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is a buff/utility spell — those aren't usable in turn-based fights yet (they need cross-round tracking). Try a damage, healing, or control spell.", spell.Name))
	}

	// Material component cost (Revivify, Raise Dead — rare in a fight).
	if spell.MaterialCost > 0 {
		if p.euro == nil || !p.euro.Debit(ctx.Sender, float64(spell.MaterialCost), "dnd_spell_component") {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"%s needs a %d-coin diamond component you can't afford.", spell.Name, spell.MaterialCost))
		}
	}
	// Slot spend — audit pattern: debit, run the round, refund on failure.
	if spell.Level > 0 {
		ok, serr := consumeSpellSlot(ctx.Sender, slotLevel)
		if serr != nil {
			return p.SendDM(ctx.Sender, "Couldn't consume slot: "+serr.Error())
		}
		if !ok {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"No L%d slot available. %s", slotLevel, renderSlotsBrief(ctx.Sender)))
		}
	}

	eff := &turnActionEffect{
		Label:       out.Desc,
		Action:      "spell_cast",
		EnemyDamage: out.EnemyDamage,
		PlayerHeal:  out.PlayerHeal,
		EnemySkip:   out.EnemySkip,
	}
	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionCast, Effect: eff})
	if err != nil {
		if spell.Level > 0 {
			_ = refundSpellSlot(ctx.Sender, slotLevel)
		}
		return p.SendDM(ctx.Sender, "Couldn't resolve the round: "+err.Error())
	}
	return p.SendDM(ctx.Sender, p.renderRoundResult(ctx.Sender, sess, events, player.Name, enemy))
}

// ── !consume (in-combat) ────────────────────────────────────────────────────
//
// !consume <item> spends one combat consumable as the player's turn. Heal and
// flat-damage items resolve fully within the round; buff-type items (ward,
// atk/def boost, spore, reflect, auto-crit) need cross-round stat tracking and
// are refused for now.

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
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	sess, err := getActiveCombatSession(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read combat state: "+err.Error())
	}
	if sess == nil {
		return p.SendDM(ctx.Sender, "`!consume <item>` spends an item on your turn in an Elite or Boss fight. You're not in one — outside combat, consumables fire automatically.")
	}

	inv := p.loadConsumableInventory(ctx.Sender)
	args = strings.TrimSpace(args)
	if args == "" {
		if len(inv) == 0 {
			return p.SendDM(ctx.Sender, "You have no combat consumables. `!attack` / `!cast` / `!flee`.")
		}
		names := make([]string, len(inv))
		for i, c := range inv {
			names[i] = c.Def.Name
		}
		return p.SendDM(ctx.Sender, "Usage: `!consume <item>`. You're carrying: "+strings.Join(names, ", ")+".")
	}

	item, ambig := matchConsumable(inv, args)
	if ambig != "" {
		return p.SendDM(ctx.Sender, ambig)
	}
	if item == nil {
		return p.SendDM(ctx.Sender, fmt.Sprintf("No consumable matching %q in your inventory.", args))
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
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"**%s** is a buff-type consumable — those aren't usable in turn-based fights yet. Healing and damage items (Berry Poultice, Coal Bomb, …) work.", def.Name))
	}

	player, enemy, err := p.combatantsForSession(sess)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't rebuild the fight: "+err.Error())
	}

	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionConsume, Effect: eff})
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't resolve the round: "+err.Error())
	}
	// Round resolved and persisted — now burn the item. A removal failure here
	// leaves the player a free use (logged, rare); better than charging them
	// for a round that errored out.
	if rerr := removeAdvInventoryItem(item.InventoryID); rerr != nil {
		slog.Error("combat: consume remove inventory item failed", "user", ctx.Sender, "item", def.Name, "err", rerr)
	}
	return p.SendDM(ctx.Sender, p.renderRoundResult(ctx.Sender, sess, events, player.Name, enemy))
}
