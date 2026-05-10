package plugin

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// !zone — Phase 11 D1c. The command surface for the dungeon-zone state
// machine landed in D1b. This file is presentation + dispatch only; the
// actual run/state plumbing lives in dnd_zone_run.go and the registry in
// dnd_zone.go.
//
// Subcommands:
//
//	!zone                         → list zones available at player level
//	!zone list                    → same
//	!zone enter <id|number|name>  → start a run
//	!zone status                  → current room / cleared count / mood / loot
//	!zone map                     → ASCII layout of the run with current marker
//	!zone advance                 → resolve the current room and move on
//	!zone abandon                 → end the active run (no rewards)
//	!zone taunt                   → poke TwinBee (mood −10, snarky reply)
//	!zone compliment              → flatter TwinBee (mood +5, fond reply)
//
// TwinBee narration / mood triggers arrive in D1d. Combat / trap / boss
// resolution per room is wired in D1e via dnd_zone_combat.go.

func (p *AdventurePlugin) handleDnDZoneCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender,
			"No Adv 2.0 character yet — run `!setup` (or just enter combat and we'll auto-build one).")
	}

	args = strings.TrimSpace(args)
	sub, rest := splitFirstWord(args)
	switch strings.ToLower(sub) {
	case "", "list", "ls":
		return p.zoneCmdList(ctx, c)
	case "enter", "start":
		return p.zoneCmdEnter(ctx, c, rest)
	case "go", "choose":
		// Graph-mode fork commit. Outside the POC gate (or with no
		// fork pending) the handler short-circuits with a friendly
		// message — see zoneCmdGo for the full surface.
		return p.zoneCmdGo(ctx, rest)
	case "status", "info":
		return p.zoneCmdStatus(ctx)
	case "map", "m":
		return p.zoneCmdMap(ctx)
	case "advance", "next", "a":
		return p.zoneCmdAdvance(ctx)
	case "abandon", "leave", "quit":
		return p.zoneCmdAbandon(ctx)
	case "taunt":
		return p.zoneCmdTaunt(ctx)
	case "compliment":
		return p.zoneCmdCompliment(ctx)
	case "lore", "history":
		return p.zoneCmdLore(ctx)
	default:
		return p.SendDM(ctx.Sender, zoneHelpText())
	}
}

func zoneHelpText() string {
	var b strings.Builder
	b.WriteString("**!zone** — dungeon zone runs.\n\n")
	b.WriteString("`!zone` or `!zone list` — show zones available at your level\n")
	b.WriteString("`!zone enter <id|#>` — start a new run\n")
	b.WriteString("`!zone status` — show your current run\n")
	b.WriteString("`!zone map` — show the room layout\n")
	b.WriteString("`!zone advance` — resolve the current room and move on\n")
	b.WriteString("`!zone go <n>` — at a fork, take path #n\n")
	b.WriteString("`!zone abandon` — end the active run (no rewards)\n")
	b.WriteString("`!zone taunt` — poke TwinBee (mood −10)\n")
	b.WriteString("`!zone compliment` — flatter TwinBee (mood +5)\n")
	b.WriteString("`!zone lore` — TwinBee shares zone history")
	return b.String()
}

// splitFirstWord returns (firstWord, rest) with rest trimmed.
func splitFirstWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}

// ── list ────────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdList(ctx MessageContext, c *DnDCharacter) error {
	zones := zonesForLevel(c.Level)
	if len(zones) == 0 {
		return p.SendDM(ctx.Sender, "No zones available at your level. (This shouldn't happen — file a bug.)")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Zones available at L%d** (you can enter zones up to 2 tiers above your current tier):\n\n", c.Level))
	for i, z := range zones {
		b.WriteString(fmt.Sprintf("**%d.** %s — _T%d, L%d–%d_  `!zone enter %s`\n",
			i+1, z.Display, int(z.Tier), z.LevelMin, z.LevelMax, z.ID))
		b.WriteString(fmt.Sprintf("    %s\n", z.Atmosphere))
	}
	if active, _ := getActiveZoneRun(ctx.Sender); active != nil {
		zone := zoneOrFallback(active.ZoneID)
		b.WriteString(fmt.Sprintf("\n_⚠ Active run: **%s**, room %d/%d. Use `!zone status` or `!zone abandon`._",
			zone.Display, active.CurrentRoom+1, active.TotalRooms))
	}
	return p.SendDM(ctx.Sender, b.String())
}

// ── enter ───────────────────────────────────────────────────────────────────

// resolveZoneInput maps a user-typed token (id, display name, or 1-based
// list index from `!zone list`) to a ZoneID at-or-below the player's
// allowed tier ceiling. Matching is case-insensitive and forgiving on
// spaces/underscores.
func resolveZoneInput(input string, available []ZoneDefinition) (ZoneID, bool) {
	in := strings.ToLower(strings.TrimSpace(input))
	if in == "" {
		return "", false
	}
	// Numeric index into the available list (1-based).
	if n := atoiSafe(in); n >= 1 && n <= len(available) {
		return available[n-1].ID, true
	}
	normIn := strings.ReplaceAll(in, " ", "_")
	for _, z := range available {
		if strings.EqualFold(string(z.ID), in) || strings.EqualFold(string(z.ID), normIn) {
			return z.ID, true
		}
		if strings.EqualFold(z.Display, input) {
			return z.ID, true
		}
		// Loose match: ignore "the " prefix on display name.
		disp := strings.TrimPrefix(strings.ToLower(z.Display), "the ")
		if disp == in || disp == normIn {
			return z.ID, true
		}
	}
	return "", false
}

// atoiSafe — returns -1 on parse failure.
func atoiSafe(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (p *AdventurePlugin) zoneCmdEnter(ctx MessageContext, c *DnDCharacter, rest string) error {
	if rest == "" {
		return p.SendDM(ctx.Sender,
			"`!zone enter <id|#>` — pick from `!zone list`. Example: `!zone enter goblin_warrens` or `!zone enter 1`.")
	}
	available := zonesForLevel(c.Level)
	id, ok := resolveZoneInput(rest, available)
	if !ok {
		return p.SendDM(ctx.Sender,
			"Unknown zone for your level. Try `!zone list` to see what's available.")
	}
	run, err := startZoneRun(ctx.Sender, id, c.Level, nil)
	if err != nil {
		switch err {
		case ErrRunAlreadyActive:
			active, _ := getActiveZoneRun(ctx.Sender)
			zone := zoneOrFallback(active.ZoneID)
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"You're already in **%s** (room %d/%d). Finish it or `!zone abandon` first.",
				zone.Display, active.CurrentRoom+1, active.TotalRooms))
		case ErrZoneTierLocked:
			return p.SendDM(ctx.Sender,
				"That zone is too far above your level. (Cap: 2 tiers above your current.)")
		case ErrUnknownZone:
			return p.SendDM(ctx.Sender, "Unknown zone.")
		default:
			return p.SendDM(ctx.Sender, "Couldn't start zone run: "+err.Error())
		}
	}
	zone := zoneOrFallback(run.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗝 **Entering %s** _(T%d, %d rooms)_\n\n", zone.Display, int(zone.Tier), run.TotalRooms))
	b.WriteString("_" + zone.Hook + "_\n\n")
	if line := twinBeeLine(zone.ID, DMRoomEntry, run.RunID, narrationCadence(run)); line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	if aside := moodAsideLine(run.DMMood, run.RunID, narrationCadence(run)); aside != "" {
		b.WriteString(aside)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("**Room 1/%d — %s.** Use `!zone advance` to proceed, `!zone map` for layout.",
		run.TotalRooms, prettyRoomType(run.CurrentRoomType())))
	return p.SendDM(ctx.Sender, b.String())
}

// ── status ──────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdStatus(ctx MessageContext) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone list` then `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run)
	zone := zoneOrFallback(run.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s** — room %d/%d (%s)\n",
		zone.Display, run.CurrentRoom+1, run.TotalRooms, prettyRoomType(run.CurrentRoomType())))
	b.WriteString(fmt.Sprintf("Cleared: %d   Loot: %d   DM mood: %d/100 (%s)\n",
		len(run.RoomsCleared), len(run.LootCollected), run.DMMood, dmMoodLabel(run.DMMood)))
	b.WriteString(fmt.Sprintf("Started: %s   Last action: %s",
		run.StartedAt.Format("2006-01-02 15:04"),
		run.LastActionAt.Format("2006-01-02 15:04")))
	return p.SendDM(ctx.Sender, b.String())
}

// dmMoodLabel returns a human-friendly label for the mood gauge per
// design doc §3.2 mood bands (≥80 effusive, 60–79 friendly, 40–59 neutral,
// 20–39 grumpy, <20 hostile).
func dmMoodLabel(mood int) string {
	switch {
	case mood >= 80:
		return "effusive"
	case mood >= 60:
		return "friendly"
	case mood >= 40:
		return "neutral"
	case mood >= 20:
		return "grumpy"
	default:
		return "hostile"
	}
}

// prettyRoomType formats a RoomType for display strings.
func prettyRoomType(rt RoomType) string {
	switch rt {
	case RoomEntry:
		return "Entry"
	case RoomExploration:
		return "Exploration"
	case RoomTrap:
		return "Trap"
	case RoomElite:
		return "Elite"
	case RoomBoss:
		return "Boss"
	default:
		return "?"
	}
}

// ── map ─────────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdMap(ctx MessageContext) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	zone := zoneOrFallback(run.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s — Map**\n```\n", zone.Display))
	if g, ok := loadZoneGraph(run.ZoneID); ok {
		b.WriteString(renderZoneGraphMap(g, run))
		b.WriteString("\n```\n")
		b.WriteString("_E=Entry  ?=Exploration  T=Trap  ★=Elite  ☠=Boss  ⚿=Secret · ✓ cleared  ▶ here  · pending  ╳ locked_")
		return p.SendDM(ctx.Sender, b.String())
	}
	// No registered graph (defensive — every zone has one post-G8).
	cleared := map[int]bool{}
	for _, r := range run.RoomsCleared {
		cleared[r] = true
	}
	b.WriteString(renderZoneMap(run.RoomSeq, run.CurrentRoom, cleared))
	b.WriteString("\n```\n")
	b.WriteString("_E=Entry  ?=Exploration  T=Trap  ★=Elite  ☠=Boss · ✓ cleared  ▶ here  · pending_")
	return p.SendDM(ctx.Sender, b.String())
}

// renderZoneMap produces a horizontal ASCII layout: a row of room glyphs
// connected with `──`, with a status row underneath showing cleared/here/
// pending markers per room. Glyphs match the legend printed alongside the
// map. roomSeq is the run's room order; current is the 0-based index of
// the player's location; cleared maps room index → true for finished rooms.
func renderZoneMap(roomSeq []RoomType, current int, cleared map[int]bool) string {
	if len(roomSeq) == 0 {
		return "(no rooms)"
	}
	var top, bot strings.Builder
	for i, rt := range roomSeq {
		if i > 0 {
			top.WriteString("──")
			bot.WriteString("  ")
		}
		top.WriteString(roomGlyph(rt))
		switch {
		case cleared[i]:
			bot.WriteString("✓")
		case i == current:
			bot.WriteString("▶")
		default:
			bot.WriteString("·")
		}
	}
	return top.String() + "\n" + bot.String()
}

// roomGlyph returns the single-character ASCII map glyph for a RoomType.
func roomGlyph(rt RoomType) string {
	switch rt {
	case RoomEntry:
		return "E"
	case RoomExploration:
		return "?"
	case RoomTrap:
		return "T"
	case RoomElite:
		return "★"
	case RoomBoss:
		return "☠"
	default:
		return "·"
	}
}

// ── advance ─────────────────────────────────────────────────────────────────

// zoneCmdAdvance resolves the room the player is currently standing in,
// then moves them to the next. Resolution branches on RoomType — combat
// for Exploration/Elite/Boss, a trap nick for Trap, narration-only for
// Entry. Player loss aborts the run with a mood penalty and player-death
// flavor; boss win drops the zone Loot table.
func (p *AdventurePlugin) zoneCmdAdvance(ctx MessageContext) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run)
	zone := zoneOrFallback(run.ZoneID)
	prev := run.CurrentRoomType()
	prevIdx := run.CurrentRoom

	// §4.1 Patrol Encounter: at Threat-Clock Alert+, patrols may move
	// through cleared rooms. Roll on `!advance` *before* the next room's
	// own resolution. Player KO ends the run.
	//
	// Patrol-then-room is chained into a single 2–3s-paced stream so the
	// player reads patrol → patrol play-by-play → patrol resolution →
	// room intro → room play-by-play → final, in one continuous beat.
	patrolIntro, patrolPhases, patrolOutcome, patrolEnded, perr := p.tryPatrolEncounter(ctx.Sender, run, zone)
	if perr != nil {
		return p.SendDM(ctx.Sender, "Couldn't resolve patrol: "+perr.Error())
	}
	var preStream []string
	if patrolPhases != nil {
		if patrolIntro != "" {
			preStream = append(preStream, patrolIntro)
		}
		preStream = append(preStream, patrolPhases...)
	}
	if patrolEnded {
		// Patrol dropped the player; run is over.
		return p.streamFlow(ctx.Sender, preStream, patrolOutcome)
	}
	// Patrol survived (or didn't fire). If it fired, patrolOutcome becomes
	// a streamed midpoint between the patrol fight and the room resolver.
	if patrolPhases != nil && patrolOutcome != "" {
		preStream = append(preStream, patrolOutcome)
	}

	// Resolve the current room *before* clearing it, so combat results
	// can decide whether the player advances or the run ends. Combat
	// rooms return a non-nil phases slice — those get streamed with
	// 2–3s delays for suspense; non-combat rooms collapse into a single
	// SendDM as before.
	intro, phases, outcome, ended, err := p.resolveRoom(ctx.Sender, run, zone)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't resolve room: "+err.Error())
	}
	if ended {
		// Death (combat or otherwise). Stream phases if present, then the
		// death narration as the final message.
		return p.streamOrSend(ctx.Sender, preStream, intro, phases, outcome)
	}

	// Graph-mode advance (G9a — only mode now): clear the current room,
	// then either auto-advance down a single edge, surface a fork prompt
	// for 2+ edges, or complete the run when there are 0 outgoing edges.
	c, cerr := LoadDnDCharacter(ctx.Sender)
	if cerr != nil {
		return p.SendDM(ctx.Sender, "Couldn't load character: "+cerr.Error())
	}
	next, forkMsg, complete, gerr := p.advanceTransitionGraph(run, zone, c)
	if gerr != nil {
		return p.SendDM(ctx.Sender, "Couldn't advance: "+gerr.Error())
	}
	if complete {
		_, _ = applyMoodEvent(run.RunID, MoodEventZoneComplete)
		var b strings.Builder
		if outcome != "" {
			b.WriteString(outcome)
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("🏆 **Cleared %s.** Run complete.\n\n", zone.Display))
		if line := twinBeeLine(zone.ID, DMZoneComplete, run.RunID, prevIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
		if granted := p.rollZoneLoot(ctx.Sender, run, zone); len(granted) > 0 {
			b.WriteString("**Loot:**\n")
			for _, id := range granted {
				b.WriteString("• " + id + "\n")
			}
		}
		return p.streamOrSend(ctx.Sender, preStream, intro, phases, b.String())
	}
	if forkMsg != "" {
		var b strings.Builder
		if outcome != "" {
			b.WriteString(outcome)
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("✓ Cleared room %d (%s).\n\n", prevIdx+1, prettyRoomType(prev)))
		b.WriteString(forkMsg)
		return p.streamOrSend(ctx.Sender, preStream, intro, phases, b.String())
	}
	return p.streamOrSend(ctx.Sender, preStream, intro, phases,
		p.formatNextRoomMessage(run, zone, prev, prevIdx, outcome, next))
}

// formatNextRoomMessage builds the "✓ Cleared room X. ... Room Y/N —
// TYPE." block used by both the legacy linear advance and graph-mode
// single-edge auto-advance. nextIdx is derived from the live run row
// — markRoomCleared / advanceZoneRunNode have already bumped it by the
// time this is called.
func (p *AdventurePlugin) formatNextRoomMessage(run *DungeonRun, zone ZoneDefinition, prev RoomType, prevIdx int, outcome string, next RoomType) string {
	nextIdx := prevIdx + 1
	var b strings.Builder
	if outcome != "" {
		b.WriteString(outcome)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("✓ Cleared room %d (%s).\n\n", prevIdx+1, prettyRoomType(prev)))
	if next == RoomBoss {
		if line := composeBossEntry(zone.ID, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	} else {
		if line := twinBeeLine(zone.ID, DMRoomEntry, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	}
	// Reload mood — combat-event nat20/nat1 deltas have been persisted via
	// scanMoodEventsFromCombat but not reflected on the in-memory run.
	freshMood := run.DMMood
	if fresh, _ := getZoneRun(run.RunID); fresh != nil {
		freshMood = fresh.DMMood
	}
	if aside := moodAsideLine(freshMood, run.RunID, nextIdx); aside != "" {
		b.WriteString(aside)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("**Room %d/%d — %s.** ", nextIdx+1, run.TotalRooms, prettyRoomType(next)))
	b.WriteString("`!zone advance` to continue.")
	return b.String()
}

// streamOrSend dispatches a staged room resolution. When the room produced
// combat phases (or there was a paced patrol pre-stream), the messages get
// streamed with 2–3s delays. Otherwise everything collapses into a single
// SendDM. preStream is non-empty when a patrol fight ran ahead of the
// current room and its play-by-play needs to lead the dispatch.
func (p *AdventurePlugin) streamOrSend(userID id.UserID, preStream []string, intro string, phases []string, final string) error {
	if phases == nil && len(preStream) == 0 {
		var b strings.Builder
		if intro != "" {
			b.WriteString(intro)
			b.WriteString("\n\n")
		}
		b.WriteString(final)
		return p.SendDM(userID, b.String())
	}
	msgs := append([]string{}, preStream...)
	if intro != "" {
		msgs = append(msgs, intro)
	}
	if phases != nil {
		msgs = append(msgs, phases...)
	}
	return p.streamFlow(userID, msgs, final)
}

// streamFlow ships a list of phase messages followed by a final message,
// with the zone-combat 2–3s delay pacing. Single entry point for both the
// patrol-and-die path and the patrol-leading-into-room path.
func (p *AdventurePlugin) streamFlow(userID id.UserID, phaseMessages []string, finalMessage string) error {
	if len(phaseMessages) == 0 {
		return p.SendDM(userID, finalMessage)
	}
	p.sendZoneCombatMessages(userID, phaseMessages, finalMessage)
	return nil
}

// resolveRoom dispatches to the per-room-type resolver. Returns staged
// messages (intro, phases, outcome) so combat rooms can be paced with
// inter-phase delays — see resolveCombatRoom for the contract. For
// non-combat rooms (entry, trap), phases is nil and outcome carries the
// resolution narration.
func (p *AdventurePlugin) resolveRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition) (intro string, phases []string, outcome string, ended bool, err error) {
	switch run.CurrentRoomType() {
	case RoomEntry:
		return
	case RoomTrap:
		_, narration := p.resolveTrapRoom(userID, run, zone)
		outcome = narration
		return
	case RoomExploration:
		return p.resolveCombatRoom(userID, run, zone, false)
	case RoomElite:
		return p.resolveCombatRoom(userID, run, zone, true)
	case RoomBoss:
		return p.resolveBossRoom(userID, run, zone)
	}
	return
}

// resolveCombatRoom spawns one roster enemy (elite filter optional),
// runs combat, persists side effects, fires nat-1/nat-20 mood deltas,
// and renders the staged narration. Returns:
//   intro   — pre-combat block (TwinBee combat-start + monster stat block)
//   phases  — RenderCombatLog output, streamed with delays by the caller
//   outcome — post-combat block: nat20/nat1 flavor, kill line, loot, d20 summary
//   ended   — true when the player went down (caller skips next-room teaser)
//
// Phases will be nil only on a "no roster" skip — caller treats that as a
// non-paced fallthrough.
func (p *AdventurePlugin) resolveCombatRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition, elite bool) (intro string, phases []string, outcome string, ended bool, err error) {
	monster, ok := pickZoneEnemy(zone, run.RunID, run.CurrentRoom, elite)
	if !ok {
		outcome = fmt.Sprintf("_(No %s roster entry — skipping.)_", map[bool]string{true: "elite", false: "exploration"}[elite])
		return
	}
	// Capture D&D-scale HP before combat so the outcome line can show
	// sheet HP rather than the engine's legacy-scale numbers.
	preHP, _ := dndHPSnapshot(userID)
	result, rerr := p.runZoneCombat(userID, monster, int(zone.Tier))
	if rerr != nil {
		err = rerr
		return
	}
	postHP, maxHP := dndHPSnapshot(userID)
	nat20s, nat1s := scanMoodEventsFromCombat(run.RunID, result)

	// Intro: pre-combat narration + creature stat block. This lands first,
	// before the 5–8s phase pacing kicks in.
	var ib strings.Builder
	if elite {
		if line := eliteRoomEntryLine(zone.ID, run.RunID, narrationCadence(run)); line != "" {
			ib.WriteString(line)
			ib.WriteString("\n")
		}
	}
	if line := twinBeeLine(zone.ID, DMCombatStart, run.RunID, narrationCadence(run)); line != "" {
		ib.WriteString(line)
		ib.WriteString("\n")
	}
	if elite {
		ib.WriteString(fmt.Sprintf("⚔️ **Elite encounter — %s** (HP %d, AC %d)", monster.Name, monster.HP, monster.AC))
	} else {
		ib.WriteString(fmt.Sprintf("⚔️ **%s** (HP %d, AC %d)", monster.Name, monster.HP, monster.AC))
	}
	intro = ib.String()

	// Phases: forward-simulating engine play-by-play. Use the player's
	// display name when available so narrative lines read naturally.
	playerName := "You"
	if name, _ := loadDisplayName(userID); name != "" {
		playerName = name
	}
	phases = RenderCombatLog(result, playerName, monster.Name)

	// Outcome: post-combat block. Nat20/nat1 narration goes here so it
	// lands *after* the play-by-play, where it has dramatic context.
	var ob strings.Builder
	if nat20s > 0 {
		if line := twinBeeLine(zone.ID, DMNat20, run.RunID, narrationCadence(run)); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
	} else if nat1s > 0 {
		if line := twinBeeLine(zone.ID, DMNat1, run.RunID, narrationCadence(run)); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
	}
	if !result.PlayerWon {
		_, _ = applyMoodEvent(run.RunID, MoodEventPlayerDeath)
		_ = abandonZoneRun(userID)
		markAdventureDead(userID, "zone", zone.Display)
		if line := twinBeeLine(zone.ID, DMPlayerDeath, run.RunID, narrationCadence(run)); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
		ob.WriteString(fmt.Sprintf("💀 You fell to **%s**. Run ended.", monster.Name))
		if rollLine := dndRollSummaryLine(result); rollLine != "" {
			ob.WriteString("\n")
			ob.WriteString(rollLine)
		}
		outcome = ob.String()
		ended = true
		return
	}
	if line := twinBeeLine(zone.ID, DMCombatEnd, run.RunID, narrationCadence(run)); line != "" {
		ob.WriteString(line)
		ob.WriteString("\n")
	}
	ob.WriteString(fmt.Sprintf("✅ **%s** down (HP %d→%d / %d).", monster.Name, preHP, postHP, maxHP))
	if rollLine := dndRollSummaryLine(result); rollLine != "" {
		ob.WriteString("\n")
		ob.WriteString(rollLine)
	}
	recordZoneKillForUser(userID, monster.ID)
	applyRoomCombatThreatForUser(userID, elite)
	if drop := p.dropZoneLoot(userID, zone.ID, monster, false); drop != "" {
		ob.WriteString("\n")
		ob.WriteString(drop)
	}
	outcome = ob.String()
	return
}

// resolveBossRoom runs the zone-boss bestiary entry through the same
// combat path as room combat. Win → caller drops zone loot.
//
// Returns staged messages — see resolveCombatRoom for the contract.
func (p *AdventurePlugin) resolveBossRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition) (intro string, phases []string, outcome string, ended bool, err error) {
	monster, ok := dndBestiary[zone.Boss.BestiaryID]
	if !ok {
		outcome = fmt.Sprintf("_(Boss %s not in bestiary — skipping.)_", zone.Boss.Name)
		return
	}
	preHP, _ := dndHPSnapshot(userID)
	result, rerr := p.runZoneCombat(userID, monster, int(zone.Tier))
	if rerr != nil {
		err = rerr
		return
	}
	postHP, maxHP := dndHPSnapshot(userID)
	nat20s, nat1s := scanMoodEventsFromCombat(run.RunID, result)

	intro = fmt.Sprintf("👑 **Boss — %s** (HP %d, AC %d)", monster.Name, monster.HP, monster.AC)

	playerName := "You"
	if name, _ := loadDisplayName(userID); name != "" {
		playerName = name
	}
	phases = RenderCombatLog(result, playerName, monster.Name)

	outcome = renderBossOutcome(BossOutcomeInputs{
		ZoneID:          zone.ID,
		RunID:           run.RunID,
		RoomIdx:         run.CurrentRoom,
		Monster:         monster,
		Result:          result,
		PreHP:           preHP,
		PostHP:          postHP,
		MaxHP:           maxHP,
		PhaseTwoAt:      zone.Boss.PhaseTwoAt,
		Nat20s:          nat20s,
		Nat1s:           nat1s,
		DefeatHeadline:  fmt.Sprintf("💀 **%s** stands over your body. Run ended.", monster.Name),
		VictoryHeadline: fmt.Sprintf("🏆 **%s** falls (HP %d→%d / %d).", monster.Name, preHP, postHP, maxHP),
	})
	if !result.PlayerWon {
		_, _ = applyMoodEvent(run.RunID, MoodEventPlayerDeath)
		_ = abandonZoneRun(userID)
		markAdventureDead(userID, "zone", zone.Display)
		ended = true
		return
	}
	recordZoneKillForUser(userID, monster.ID)
	// §8.1: zone boss defeat drops expedition threat by 20. Silent
	// no-op for standalone zone runs (no active expedition).
	if exp, eerr := getActiveExpedition(userID); eerr == nil && exp != nil {
		_ = applyBossDefeatThreat(exp.ID)
	}
	if drop := p.dropZoneLoot(userID, zone.ID, monster, true); drop != "" {
		outcome = outcome + "\n" + drop
	}
	return
}

// BossOutcomeInputs is the carrier struct for renderBossOutcome — the
// shared staged-narration body used by zone bosses (resolveBossRoom)
// and arena bosses (resolveArenaBoss, post-L2). Pure inputs: every
// field is read-only and the helper performs no DB writes.
type BossOutcomeInputs struct {
	ZoneID  ZoneID // routes twinBeeLine (use ZoneArena for arena fights)
	RunID   string // deterministic seed for twinBeeLine pickers
	RoomIdx int    // ditto

	Monster              DnDMonsterTemplate
	Result               CombatResult
	PreHP, PostHP, MaxHP int
	PhaseTwoAt           float64 // fraction of MaxHP; 0 disables phase-two narration
	Nat20s, Nat1s        int // pre-counted by scanMoodEventsFromCombat

	// Caller-supplied headlines so arena and zone read in their own voice.
	// DefeatHeadline is the full sentence shown after the PlayerDeath
	// TwinBee line; VictoryHeadline is the full sentence shown after
	// BossDeath. The roll-summary line is appended automatically.
	DefeatHeadline  string
	VictoryHeadline string
}

// renderBossOutcome composes the outcome block for a boss-shaped
// encounter: optional Nat20/Nat1 mood line, optional phase-two barb if
// the enemy crossed PhaseTwoAt mid-fight, BossDeath/PlayerDeath flavor,
// the caller-supplied victory/defeat headline, and a trailing dice-roll
// summary. Side-effect free — callers handle abandonZoneRun, loot,
// kill records, payout, etc.
func renderBossOutcome(in BossOutcomeInputs) string {
	var ob strings.Builder
	if in.Nat20s > 0 {
		if line := twinBeeLine(in.ZoneID, DMNat20, in.RunID, in.RoomIdx); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
	} else if in.Nat1s > 0 {
		if line := twinBeeLine(in.ZoneID, DMNat1, in.RunID, in.RoomIdx); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
	}
	if phaseTwoCrossedInEvents(in.Result.Events, in.Result.EnemyStartHP, in.PhaseTwoAt) {
		if line := bossPhaseTwoLine(in.ZoneID, in.RunID, in.RoomIdx); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
	}
	if !in.Result.PlayerWon {
		if line := twinBeeLine(in.ZoneID, DMPlayerDeath, in.RunID, in.RoomIdx); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
		ob.WriteString(in.DefeatHeadline)
		if rollLine := dndRollSummaryLine(in.Result); rollLine != "" {
			ob.WriteString("\n")
			ob.WriteString(rollLine)
		}
		return ob.String()
	}
	if line := twinBeeLine(in.ZoneID, DMBossDeath, in.RunID, in.RoomIdx); line != "" {
		ob.WriteString(line)
		ob.WriteString("\n")
	}
	ob.WriteString(in.VictoryHeadline)
	if rollLine := dndRollSummaryLine(in.Result); rollLine != "" {
		ob.WriteString("\n")
		ob.WriteString(rollLine)
	}
	return ob.String()
}

// ── taunt / compliment ──────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdTaunt(ctx MessageContext) error {
	return p.zoneMoodInteraction(ctx, MoodEventTaunt, tauntResponseLine, "💢")
}

func (p *AdventurePlugin) zoneCmdCompliment(ctx MessageContext) error {
	return p.zoneMoodInteraction(ctx, MoodEventCompliment, complimentResponseLine, "💐")
}

// zoneMoodInteraction is the shared wiring for !zone taunt / !zone compliment:
// require an active run, apply the mood event, render a deterministic line
// from the prewritten pool, and report the new mood band.
func (p *AdventurePlugin) zoneMoodInteraction(
	ctx MessageContext,
	ev DMMoodEvent,
	render func(runID string, roomIdx int) string,
	icon string,
) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run)
	newMood, err := applyMoodEvent(run.RunID, ev)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't apply mood event: "+err.Error())
	}
	var b strings.Builder
	if line := render(run.RunID, narrationCadence(run)); line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	delta := MoodEventDelta(ev)
	sign := "+"
	if delta < 0 {
		sign = "−"
		delta = -delta
	}
	b.WriteString(fmt.Sprintf("%s DM mood: %d/100 (%s) _(%s%d)_",
		icon, newMood, dmMoodLabel(newMood), sign, delta))
	return p.SendDM(ctx.Sender, b.String())
}

// ── lore ────────────────────────────────────────────────────────────────────

// zoneCmdLore renders a deterministic TwinBee lore line for the active
// run's current room. Pulls from the zone-specific LoreLines* pool when
// one exists (Tier 1–5 ship dedicated lore), falls back to the generic
// LoreLines pool otherwise. roomIdx is folded into the selector so
// repeated `!zone lore` in the same room returns the same prose, but
// cross-room calls vary.
func (p *AdventurePlugin) zoneCmdLore(ctx MessageContext) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	zone := zoneOrFallback(run.ZoneID)
	line := twinBeeLine(zone.ID, DMLore, run.RunID, narrationCadence(run))
	if line == "" {
		return p.SendDM(ctx.Sender, "TwinBee has nothing to say about this place.")
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("📖 **%s — Lore**\n\n%s", zone.Display, line))
}

// ── abandon ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdAbandon(ctx MessageContext) error {
	run, _ := getActiveZoneRun(ctx.Sender)
	if err := abandonZoneRun(ctx.Sender); err != nil {
		if err == ErrNoActiveRun {
			return p.SendDM(ctx.Sender, "No active run to abandon.")
		}
		return p.SendDM(ctx.Sender, "Couldn't abandon run: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "Run abandoned.")
	}
	zone := zoneOrFallback(run.ZoneID)
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"🚪 Abandoned **%s** at room %d/%d. No rewards.",
		zone.Display, run.CurrentRoom+1, run.TotalRooms))
}

