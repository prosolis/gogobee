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
	case "revisit", "back":
		return p.handleRevisitCmd(ctx, rest)
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
	b.WriteString("`!revisit <n>` — walk back to a room you've already cleared\n")
	b.WriteString("`!zone abandon` — end the active run (no rewards)\n")
	b.WriteString("`!zone taunt` — poke TwinBee (they'll remember)\n")
	b.WriteString("`!zone compliment` — flatter TwinBee (they'll like that)\n")
	b.WriteString("`!zone lore` — I share zone history")
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
	if active, isLeader, _ := activeZoneRunFor(ctx.Sender); active != nil {
		zone := zoneOrFallback(active.ZoneID)
		tail := "Use `!zone status` or `!zone abandon`."
		if !isLeader {
			tail = "Use `!zone status`; your leader ends it."
		}
		b.WriteString(fmt.Sprintf("\n_⚠ Active run: **%s**, room %d/%d. %s_",
			zone.Display, active.CurrentRoom+1, active.TotalRooms, tail))
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
	if cs, _ := activeCombatSessionFor(ctx.Sender); cs != nil {
		return p.SendDM(ctx.Sender, "⚔️ Finish your fight first — `!attack` or `!flee`.")
	}
	// startZoneRun only refuses a player who owns an active run. A seated party
	// member owns neither run nor expedition row, so without this they could
	// open a private dungeon while riding the leader's — and activeZoneRunFor
	// would then hand every party read their solo run instead of the party's.
	// seatedExpeditionFor, not activeExpeditionFor: the seat outlives an
	// `extracting` expedition, and so must the refusal.
	if seated, _ := seatedExpeditionFor(ctx.Sender); seated != nil {
		return p.SendDM(ctx.Sender,
			"You're already in your party's dungeon. `!expedition leave` to strike out on your own.")
	}
	if rest == "" {
		return p.SendDM(ctx.Sender,
			"`!zone enter <id|#>` — pick from `!zone list`. Example: `!zone enter goblin_warrens` or `!zone enter 1`.")
	}
	if remaining := restingLockoutRemaining(c); remaining > 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"🛌 You're still resting — %s remaining. The dungeon won't go anywhere.",
			formatRespecDuration(remaining)))
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
	// Wounded-entry warning: if HP < 50% MaxHP at zone-enter time, flag it
	// before the player advances into combat. Wounds carry across runs,
	// monster damage tuning assumes near-full HP; entering at half or less
	// is a death-spiral invitation.
	if c.HPMax > 0 && c.HPCurrent*2 < c.HPMax {
		b.WriteString(fmt.Sprintf("⚠️ _You're entering at **%d/%d HP**. Consider `!rest` first._\n\n",
			c.HPCurrent, c.HPMax))
	}
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
	run, isLeader, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone list` then `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run, isLeader)
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
	run, isLeader, err := activeZoneRunFor(ctx.Sender)
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
		if path := renderVisitedPath(g, run); path != "" {
			b.WriteString("\n**Path:** " + path)
		}
		// A member reads the same map but can't walk it — `!revisit` is the
		// leader's call, like the fork. Offering them the numbers would only
		// earn a refusal.
		if targets := revisitTargets(g, run); len(targets) > 0 && isLeader {
			nums := make([]string, len(targets))
			for i, n := range targets {
				nums[i] = fmt.Sprintf("`!revisit %d`", n)
			}
			b.WriteString("\n**Back to:** " + strings.Join(nums, " · "))
		}
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

// stopReason classifies why a single advanceOnce step stopped. StopOK means
// the step walked into the next room cleanly and a loop caller may keep
// going. Everything else is an interrupt — the autopilot loop in
// expeditionCmdRun renders the result and stops; the one-shot
// zoneCmdAdvance treats them all the same (single dispatch).
type stopReason int

const (
	stopOK            stopReason = iota // walked to next room; loop may continue
	stopFork                            // advanceTransitionGraph returned a forkMsg
	stopElite                           // standing at an Elite doorway; needs !fight
	stopBoss                            // standing at a Boss doorway; needs !fight
	stopEnded                           // patrol or room resolution killed the player
	stopComplete                        // run cleared (boss down, no outgoing edges)
	stopBlocked                         // an active CombatSession blocks the advance
	stopHarvestCombat                   // auto-harvest pulled into combat that resolved short of death
	stopPreflight                       // pre-iteration preflight tripped (low HP / low SU)
	stopBossSafety                      // compact autopilot bailed before boss (HP/SU/exhaustion gate) — caller pitches a rest camp
)

// bossSafetyHPPct — compact-autopilot won't engage a boss while current HP
// is at or below this fraction of max. 0.80 ≫ autopilotLowHPPct (0.30) so
// the gate fires well before the player is in real danger; the boss is
// the run's climax beat and we'd rather rest first than chip-trade into it.
const bossSafetyHPPct = 0.80

// bossSafetyExhaustion — gate trips at this level or above. 3 is the 5e
// "disadvantage on attack rolls and saving throws" tier; engaging a boss
// past that is a coin-flip TPK. A standard rest decrements exhaustion by 1,
// so two rest cycles clears a stack of 3 even without a long rest.
const bossSafetyExhaustion = 3

// bossSafetyGate reports whether the compact autopilot should pause before
// engaging the boss. Returns (player-facing reason, true) when blocked.
// Plumbed through the boss/elite branch of advanceOnceWithOpts so the
// scheduler can pitch a rest camp in response (see tryAutoRun).
func bossSafetyGate(userID id.UserID, exp *Expedition) (string, bool) {
	cur, max := dndHPSnapshot(userID)
	if max > 0 && float64(cur) <= float64(max)*bossSafetyHPPct {
		return fmt.Sprintf("HP %d/%d — below %.0f%% boss-engage threshold", cur, max, bossSafetyHPPct*100), true
	}
	if exp != nil && exp.Supplies.DailyBurn > 0 && exp.Supplies.Current < exp.Supplies.DailyBurn {
		return fmt.Sprintf("supplies %.1f/%.1f SU — under a day's burn", exp.Supplies.Current, exp.Supplies.DailyBurn), true
	}
	if c, _ := LoadDnDCharacter(userID); c != nil && c.Exhaustion >= bossSafetyExhaustion {
		return fmt.Sprintf("exhaustion %d — too worn to fight clean", c.Exhaustion), true
	}
	return "", false
}

// advanceResult bundles the staged narration + dispatch shape of one
// advanceOnce step. preStream/intro/phases/final mirror the streamOrSend
// contract — phases nil means "no per-step pacing required". reason tells
// loop callers whether they may iterate again.
type advanceResult struct {
	preStream []string
	intro     string
	phases    []string
	final     string
	reason    stopReason
	// nextRoomType — when reason is stopOK, the type of the room the
	// player just stepped into. The autopilot loop reads this to break
	// before running a redundant iteration whose only job would be to
	// re-emit the Boss/Elite doorway gate (and produce a duplicate
	// "Room X/Y — Boss" line in the same DM).
	nextRoomType RoomType
	// harvest carries the auto-harvest pass result for the room the
	// player just walked into (Josie H2 — auto-harvest parity between
	// `!zone advance` and `!expedition run`). Zero value when no
	// harvest ran (Entry/Trap/Elite/Boss rooms, forks, stops).
	harvest       autoHarvestResult
	harvestFooter string
}

// zoneCmdAdvance resolves the room the player is currently standing in,
// then moves them to the next. Thin wrapper over advanceOnce + dispatch;
// the expedition autopilot (expeditionCmdRun) reuses advanceOnce in a
// loop. Resolution branches on RoomType — combat for Exploration/Elite/
// Boss, a trap nick for Trap, narration-only for Entry. Player loss
// aborts the run with a mood penalty and player-death flavor; boss win
// drops the zone Loot table.
func (p *AdventurePlugin) zoneCmdAdvance(ctx MessageContext) error {
	// The party walks on the leader's row. advanceOnce would tell a member they
	// have no run at all — right outcome, wrong story. Ask membership directly
	// rather than `run != nil && !isLeader`: that spelling misses the window
	// where the leader has an expedition but has not taken the first step (no
	// run row yet), and it would re-resolve the run advanceOnce is about to
	// resolve anyway, firing the §4.3 idle reap twice per keystroke.
	if isPartyMember(ctx.Sender) {
		return p.SendDM(ctx.Sender,
			"Your party leader sets the pace. `!map` to see where you're standing.")
	}
	res, err := p.advanceOnce(ctx)
	if err != nil {
		return p.SendDM(ctx.Sender, err.Error())
	}
	return p.streamOrSend(ctx.Sender, res.preStream, res.intro, res.phases, res.final)
}

// advanceOnce runs the single-room advance pipeline and returns a
// structured result the caller can stream as-is or aggregate. Errors
// returned here are user-facing (already prefixed with the appropriate
// "Couldn't ..." context); callers should SendDM them verbatim. State
// mutations on run / character / combat sessions persist regardless of
// how the caller dispatches the narration.
// advanceOnce — single-room advance. compact==true switches to the
// terse autopilot-friendly combat narration and auto-resolves elite
// doorways (boss still stops; boss is the climax beat). Foreground
// `!expedition run` / `!zone advance` always pass false.
func (p *AdventurePlugin) advanceOnce(ctx MessageContext) (advanceResult, error) {
	return p.advanceOnceWithOpts(ctx, false, false)
}

// inlineBossCombat (only consulted when compact==true) selects between the
// two background combat paths at a boss/elite doorway. true keeps the
// legacy inline auto-resolve (SimulateCombat — fast, but ignores enemy
// multiattack). false returns stopBoss/stopElite after the safety gate so
// a turn-based driver — autoDriveCombat / pickAutoCombatAction — handles
// the fight via the regular !fight / !attack engine. Both the headless sim
// and the production autorun (long-expedition D8-f) now pass false so the
// real engine (with multiattack) resolves the encounter and simPickSpell
// actually fires; D8-prereq re-wired this seam, D8-f flipped prod onto it.
func (p *AdventurePlugin) advanceOnceWithOpts(ctx MessageContext, compact, inlineBossCombat bool) (advanceResult, error) {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return advanceResult{}, fmt.Errorf("Couldn't read run state: %s", err.Error())
	}
	if run == nil {
		return advanceResult{}, fmt.Errorf("No active zone run. Use `!zone enter <id>`.")
	}
	if cs, _ := getActiveCombatSession(ctx.Sender); cs != nil {
		return advanceResult{
			final:  "⚔️ Finish your fight first — `!attack` or `!flee`.",
			reason: stopBlocked,
		}, nil
	}
	// getActiveZoneRun above resolves the sender's own row, so the walker is
	// always this run's owner.
	_ = applyMoodDecayIfStale(run, true)
	zone := zoneOrFallback(run.ZoneID)
	// A pending fork means advanceTransitionGraph already cleared the
	// current room and stopped — re-running resolveRoom would re-fire
	// combat and re-drop loot on the same room. Re-emit the fork prompt
	// and let the caller surface it; the player commits via !zone go <n>.
	// This returns *before* crediting the daily streak: spamming `!zone
	// advance` at a fork resolves no room, so it must not keep the streak alive.
	if pf, derr := decodePendingFork(run.NodeChoices); derr == nil && pf != nil {
		return advanceResult{
			final:  renderForkPrompt(zone, *pf),
			reason: stopFork,
		}, nil
	}
	// compact==true is the background auto-walk path; only credit
	// player-initiated advances toward the daily streak.
	if !compact {
		markActedToday(ctx.Sender)
	}
	prev := run.CurrentRoomType()
	prevIdx := run.CurrentRoom

	// Elite/Boss rooms are manual: !zone advance stops at the doorway and the
	// player engages with !fight. Patrols don't roll while standing at the
	// door — only on the advance that walked the player here, which already
	// fired below on the previous room. A won CombatSession for the encounter
	// means the fight's done; fall through so the graph clears the room.
	//
	// compact==true (background autopilot) auto-resolves elite rooms via
	// the same forward-sim engine used for exploration combat. Long-
	// expedition D3 extends the same path to boss rooms — gated by a
	// safety check (HP/SU/exhaustion). When the gate trips the walk
	// returns stopBossSafety; the autorun layer pitches a rest camp in
	// response (see tryAutoRun). Foreground (!compact) still parks the
	// player at the doorway for a manual !fight.
	var eliteAutoIntro string
	var eliteAutoPhases []string
	var eliteAutoOutcome string
	eliteAutoResolved := false
	if prev == RoomElite || prev == RoomBoss {
		encID := encounterIDForRoom(prevIdx)
		sess, serr := getCombatSessionForEncounter(run.RunID, encID)
		if serr != nil {
			return advanceResult{}, fmt.Errorf("Couldn't read combat state: %s", serr.Error())
		}
		if sess == nil || sess.Status != CombatStatusWon {
			if !compact {
				kind := "Elite"
				r := stopElite
				if prev == RoomBoss {
					kind = "Boss"
					r = stopBoss
				}
				return advanceResult{
					final: fmt.Sprintf("**Room %d/%d — %s.** Type `!fight` to engage.",
						prevIdx+1, run.TotalRooms, kind),
					reason: r,
				}, nil
			}
			if prev == RoomBoss {
				// Cheap extra read — only fires on the boss doorway tick.
				exp, _ := getActiveExpedition(ctx.Sender)
				if reason, blocked := bossSafetyGate(ctx.Sender, exp); blocked {
					return advanceResult{
						final: fmt.Sprintf(
							"⏸ **Autopilot held back from the boss** — %s. Pitching a rest camp; will re-engage after recovery.",
							reason),
						reason: stopBossSafety,
					}, nil
				}
			}
			if !inlineBossCombat {
				// Background caller wants to drive the fight itself via the
				// turn engine (autoDriveCombat / pickAutoCombatAction).
				// Surface the doorway like the foreground path does, after
				// the safety gate has had a chance to defer the engagement.
				kind := "Elite"
				r := stopElite
				if prev == RoomBoss {
					kind = "Boss"
					r = stopBoss
				}
				return advanceResult{
					final: fmt.Sprintf("**Room %d/%d — %s.** Type `!fight` to engage.",
						prevIdx+1, run.TotalRooms, kind),
					reason: r,
				}, nil
			}
			// Compact-mode elite/boss auto-resolve. resolveCombatRoom
			// selects monster + label by run.CurrentRoomType().
			ai, ap, ao, aended, aerr := p.resolveCombatRoom(ctx.Sender, run, zone, prev == RoomElite, true)
			if aerr != nil {
				return advanceResult{}, fmt.Errorf("Couldn't auto-resolve %s: %s", strings.ToLower(string(prev)), aerr.Error())
			}
			if aended {
				return advanceResult{
					intro:  ai,
					phases: ap,
					final:  ao,
					reason: stopEnded,
				}, nil
			}
			eliteAutoIntro = ai
			eliteAutoPhases = ap
			eliteAutoOutcome = ao
			eliteAutoResolved = true
		}
	}

	// §4.1 Patrol Encounter: at Threat-Clock Alert+, patrols may move
	// through cleared rooms. Roll on `!advance` *before* the next room's
	// own resolution. Player KO ends the run.
	//
	// Patrol-then-room is chained into a single 2–3s-paced stream so the
	// player reads patrol → patrol play-by-play → patrol resolution →
	// room intro → room play-by-play → final, in one continuous beat.
	patrolIntro, patrolPhases, patrolOutcome, patrolEnded, perr := p.tryPatrolEncounter(ctx.Sender, run, zone)
	if perr != nil {
		return advanceResult{}, fmt.Errorf("Couldn't resolve patrol: %s", perr.Error())
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
		return advanceResult{
			preStream: preStream,
			final:     patrolOutcome,
			reason:    stopEnded,
		}, nil
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
	intro, phases, outcome, ended, err := p.resolveRoom(ctx.Sender, run, zone, compact)
	if eliteAutoResolved {
		// Fold the auto-resolved elite combat in. resolveRoom returned
		// empty for the elite room type (turn-based system never ran),
		// so we just use the forward-sim narration we captured above.
		intro = eliteAutoIntro
		phases = eliteAutoPhases
		outcome = eliteAutoOutcome
	}
	if err != nil {
		return advanceResult{}, fmt.Errorf("Couldn't resolve room: %s", err.Error())
	}
	if ended {
		// Death (combat or otherwise). Stream phases if present, then the
		// death narration as the final message.
		return advanceResult{
			preStream: preStream,
			intro:     intro,
			phases:    phases,
			final:     outcome,
			reason:    stopEnded,
		}, nil
	}

	// Graph-mode advance (G9a — only mode now): clear the current room,
	// then either auto-advance down a single edge, surface a fork prompt
	// for 2+ edges, or complete the run when there are 0 outgoing edges.
	c, cerr := LoadDnDCharacter(ctx.Sender)
	if cerr != nil {
		return advanceResult{}, fmt.Errorf("Couldn't load character: %s", cerr.Error())
	}
	next, forkMsg, complete, gerr := p.advanceTransitionGraph(run, zone, c)
	if gerr != nil {
		return advanceResult{}, fmt.Errorf("Couldn't advance: %s", gerr.Error())
	}
	var campStruck string
	if kind := autoBreakCampOnMove(ctx.Sender); kind != "" {
		campStruck = fmt.Sprintf("⛺ Camp struck (**%s**) — the party moved on.\n\n", kind)
	}
	if complete {
		_, _ = applyMoodEvent(run.RunID, MoodEventZoneComplete)
		var b strings.Builder
		if outcome != "" {
			b.WriteString(outcome)
			b.WriteString("\n\n")
		}
		if campStruck != "" {
			b.WriteString(campStruck)
		}
		// A "complete" run is only a full zone clear when it isn't a mid-zone
		// region clear of a multi-region zone. For the latter, name the region
		// and point at the next one — "Cleared {zone}. Run complete." reads
		// wrong right before the auto-advance transit block (and is shared with
		// manual `!region travel`, which advances next).
		if region, next, midZone := midZoneRegionClear(ctx.Sender, run.RunID); midZone {
			b.WriteString(fmt.Sprintf("🏁 **Cleared %s.** The way to %s opens ahead.\n\n", region.Name, next.Name))
		} else {
			b.WriteString(fmt.Sprintf("🏆 **Cleared %s.** Run complete.\n\n", zone.Display))
		}
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
		// Success-path expedition close-out: flip the wrapping expedition to
		// 'complete' (when this clear finishes the whole zone) and surface any
		// completion milestones. No-op for standalone runs / mid-zone region
		// clears.
		if lines := p.finalizeExpeditionOnZoneClear(ctx.Sender, run.RunID); len(lines) > 0 {
			b.WriteString("\n")
			for _, line := range lines {
				b.WriteString(line)
			}
		}
		return advanceResult{
			preStream: preStream,
			intro:     intro,
			phases:    phases,
			final:     b.String(),
			reason:    stopComplete,
		}, nil
	}
	if forkMsg != "" {
		var b strings.Builder
		if outcome != "" {
			b.WriteString(outcome)
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("✓ Cleared room %d (%s).\n\n", prevIdx+1, prettyRoomType(prev)))
		if campStruck != "" {
			b.WriteString(campStruck)
		}
		b.WriteString(forkMsg)
		return advanceResult{
			preStream: preStream,
			intro:     intro,
			phases:    phases,
			final:     b.String(),
			reason:    stopFork,
		}, nil
	}
	finalMsg := p.formatNextRoomMessage(run, zone, prev, prevIdx, outcome, next)
	if campStruck != "" {
		finalMsg = campStruck + finalMsg
	}

	// H2 — auto-harvest the room the player just walked into. Only fires
	// for Exploration rooms (Entry/Trap/Elite/Boss self-skip via
	// currentRoomCleared / autoHarvestRoom's no-op when there's nothing
	// to harvest). Uses standalone storage when there's no active
	// expedition, expedition region_state when there is.
	hr, hfooter := p.runHarvestForAdvance(ctx.Sender, run.RunID, c, next)
	if hfooter != "" {
		finalMsg += "\n\n" + hfooter
	}
	if hr.CombatNarr != "" {
		// Harvest interrupt fired combat. Append the narration to the
		// final block; reclassify reason for the autopilot loop so it
		// can break with the right footer/tally treatment.
		finalMsg += "\n\n" + hr.CombatNarr
	}
	res := advanceResult{
		preStream:     preStream,
		intro:         intro,
		phases:        phases,
		final:         finalMsg,
		reason:        stopOK,
		nextRoomType:  next,
		harvest:       hr,
		harvestFooter: hfooter,
	}
	if hr.CombatNarr != "" {
		res.reason = stopHarvestCombat
		if hr.PlayerEnded {
			res.reason = stopEnded
		}
	}
	return res, nil
}

// runHarvestForAdvance runs an auto-harvest pass on the room the player
// just walked into and returns the result plus a rendered per-room
// footer. Reloads the run so the harvest sees the post-graph-advance
// current node; binds to the active expedition when one exists, falls
// through to standalone (per-run) storage otherwise. nextRoomType is a
// fast-path filter so we don't even touch the DB for Entry/Trap/Elite/
// Boss rooms — autoHarvestRoom would no-op on them anyway.
func (p *AdventurePlugin) runHarvestForAdvance(
	userID id.UserID, runID string, c *DnDCharacter, nextRoomType RoomType,
) (autoHarvestResult, string) {
	if nextRoomType != RoomExploration {
		return autoHarvestResult{}, ""
	}
	fresh, ferr := getZoneRun(runID)
	if ferr != nil || fresh == nil {
		return autoHarvestResult{}, ""
	}
	exp, _ := getActiveExpedition(userID)
	hr, herr := p.autoHarvestRoom(userID, fresh, c, exp)
	if herr != nil {
		return autoHarvestResult{}, ""
	}
	return hr, renderAutoHarvestFooter(hr.Summary)
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
	// Route the action hint by what's actually ahead. The next advance
	// against an Elite/Boss room hits the doorway gate and demands
	// `!fight` — telling the player to `!zone advance` here would
	// directly contradict that gate's message.
	switch next {
	case RoomBoss:
		b.WriteString("`!fight` when you're ready for the boss.")
	case RoomElite:
		b.WriteString("`!fight` when ready.")
	default:
		b.WriteString(continueHint(id.UserID(run.UserID)))
	}
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
	// Fire-and-forget: streamFlow's whole job is to hand off to the
	// streamer and return — no post-flush chaining. Explicit discard
	// honors the sendZoneCombatMessages contract.
	_ = p.sendZoneCombatMessages(userID, phaseMessages, finalMessage)
	return nil
}

// streamFlowThen behaves like streamFlow but runs after() once the final
// message has actually been delivered. Emergence follow-ups (e.g. the pet
// arrival DM) must land strictly *after* the paced run narration — rolling
// them before streamFlow returns races the streamer's goroutine and surfaces
// "there's an animal in your house" ahead of the "Run complete" beat the
// player is still waiting on. after may be nil.
func (p *AdventurePlugin) streamFlowThen(userID id.UserID, phaseMessages []string, finalMessage string, after func()) error {
	if len(phaseMessages) == 0 {
		err := p.SendDM(userID, finalMessage)
		if after != nil {
			after()
		}
		return err
	}
	done := p.sendZoneCombatMessages(userID, phaseMessages, finalMessage)
	if after != nil {
		go func() {
			<-done
			after()
		}()
	}
	return nil
}

// resolveRoom dispatches to the per-room-type resolver. Returns staged
// messages (intro, phases, outcome) so combat rooms can be paced with
// inter-phase delays — see resolveCombatRoom for the contract. For
// non-combat rooms (entry, trap), phases is nil and outcome carries the
// resolution narration.
func (p *AdventurePlugin) resolveRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition, compact bool) (intro string, phases []string, outcome string, ended bool, err error) {
	// Revisit R2 — a cleared room stays cleared. Advancing out of a room the
	// player backtracked into must not re-roll its combat or re-arm its trap.
	//
	// The revisit plan assumed terminal CombatSession rows already gated this.
	// They only gate Elite/Boss (checked at the doorway in advanceOnceWithOpts);
	// an Exploration room re-enters resolveCombatRoom unconditionally, and a
	// Trap room re-arms. Without this, `!revisit` would be a loot exploit:
	// walk back one room, advance, farm the same enemy forever.
	//
	// A no-op under forward-only navigation — advance clears the current room
	// only after resolving it, so the room being resolved is never already in
	// RoomsCleared.
	if run.RoomIsCleared(run.CurrentRoom) {
		return
	}
	switch run.CurrentRoomType() {
	case RoomEntry:
		return
	case RoomTrap:
		_, narration := p.resolveTrapRoom(userID, run, zone)
		outcome = narration
		return
	case RoomExploration:
		return p.resolveCombatRoom(userID, run, zone, false, compact)
	case RoomElite, RoomBoss:
		// Manual turn-based combat (!fight / !attack). zoneCmdAdvance only
		// reaches resolveRoom for an Elite/Boss room once a won CombatSession
		// exists for it — so there's nothing to resolve here. Return empty
		// and let the caller advance the graph past the now-cleared room.
		return
	}
	return
}

// resolveCombatRoom spawns one roster enemy (elite filter optional),
// runs combat, persists side effects, fires nat-1/nat-20 mood deltas,
// and renders the staged narration. Returns:
//
//   intro   — pre-combat block (TwinBee combat-start + monster stat block)
//   phases  — RenderCombatLog output, streamed with delays by the caller
//   outcome — post-combat block: nat20/nat1 flavor, kill line, loot, d20 summary
//   ended   — true when the player went down (caller skips next-room teaser)
//
// Phases will be nil only on a "no roster" skip — caller treats that as a
// non-paced fallthrough.
func (p *AdventurePlugin) resolveCombatRoom(userID id.UserID, run *DungeonRun, zone ZoneDefinition, elite, compact bool) (intro string, phases []string, outcome string, ended bool, err error) {
	// Long-expedition D3 — compact autopilot now auto-resolves boss rooms
	// too. The room-type drives monster selection (boss room → zone.Boss
	// bestiary entry; exploration/elite → roster pick). Foreground boss
	// combat is still the manual !fight path; resolveRoom() doesn't
	// dispatch for RoomBoss outside compact.
	isBoss := run.CurrentRoomType() == RoomBoss
	var monster DnDMonsterTemplate
	var ok bool
	if isBoss {
		monster, ok = dndBestiary[zone.Boss.BestiaryID]
	} else {
		monster, ok = pickZoneEnemy(zone, run.RunID, run.CurrentRoom, elite)
	}
	if !ok {
		kind := "exploration"
		switch {
		case isBoss:
			kind = "boss"
		case elite:
			kind = "elite"
		}
		outcome = fmt.Sprintf("_(No %s roster entry — skipping.)_", kind)
		return
	}
	preHP, _ := dndHPSnapshot(userID)
	result, rerr := p.runZoneCombat(userID, monster, int(zone.Tier), nil, run.DMMood)
	if rerr != nil {
		err = rerr
		return
	}
	postHP, maxHP := dndHPSnapshot(userID)
	nat20s, nat1s := scanMoodEventsFromCombat(run.RunID, result)

	// Compact mode: skip TwinBee banter, skip the multi-beat play-by-play.
	// Render a single outcome line. Still records kills, threat, and drops.
	if compact && result.PlayerWon {
		hpDelta := preHP - postHP
		var ob strings.Builder
		label := monster.Name
		switch {
		case isBoss:
			label = "👑 Boss — " + monster.Name
		case elite:
			label = "Elite " + monster.Name
		}
		ob.WriteString(fmt.Sprintf("⚔️ **%s** down — HP %d→%d", label, preHP, postHP))
		if hpDelta > 0 {
			ob.WriteString(fmt.Sprintf(" (−%d)", hpDelta))
		}
		ob.WriteString(".")
		recordZoneKillForUser(userID, monster.ID)
		applyRoomCombatThreatForUser(userID, elite || isBoss)
		if drop := p.dropZoneLoot(userID, zone.ID, monster, isBoss, elite); drop != "" {
			ob.WriteString(" ")
			ob.WriteString(drop)
		}
		outcome = ob.String()
		return
	}

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
	if curios := activeMagicItemsLine(userID); curios != "" {
		ib.WriteString("\n")
		ib.WriteString(curios)
	}
	intro = ib.String()

	// Phases: forward-simulating engine play-by-play. Use the player's
	// display name when available so narrative lines read naturally.
	// In compact mode we already returned early on a win; the remaining
	// compact loss path still wants no multi-beat phases (the player
	// needs the outcome, not 10 lines of attrition).
	if !compact {
		playerName := "You"
		if name, _ := loadDisplayName(userID); name != "" {
			playerName = name
		}
		phases = RenderCombatLog(result, playerName, monster.Name)
	}

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
		// resolveCombatRoom gates room progression — a retreat here has
		// nowhere to go (the elite/room is still blocking the way), so
		// any loss ends the run. The retreat-continues semantic only
		// fits the non-gating expedition paths (runHarvestInterrupt,
		// tryPatrolEncounter); see retreatThreatBump in
		// dnd_expedition_combat.go.
		_, _ = applyMoodEvent(run.RunID, MoodEventPlayerDeath)
		_ = abandonZoneRun(userID)
		// Timeout loss = retreat; player took wounds but isn't actually
		// dead. Don't fire markAdventureDead — that would trigger the 6h
		// respawn timer for what is mechanically "ran out the clock".
		if !result.TimedOut {
			markAdventureDead(userID, "zone", zone.Display)
			forceExtractExpeditionForRunLoss(userID, "combat death")
		} else {
			forceExtractExpeditionForRunLoss(userID, "combat retreat")
		}
		if line := twinBeeLine(zone.ID, DMPlayerDeath, run.RunID, narrationCadence(run)); line != "" {
			ob.WriteString(line)
			ob.WriteString("\n")
		}
		if result.TimedOut {
			ob.WriteString(fmt.Sprintf("⏳ The fight drags on. **%s** outlasts you. You retreat, wounded but alive.", monster.Name))
		} else {
			ob.WriteString(fmt.Sprintf("💀 You fell to **%s**. Run ended.", monster.Name))
		}
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
	ob.WriteString(fmt.Sprintf("✅ **%s** down. You finished at **%d/%d HP**.", monster.Name, postHP, maxHP))
	if rollLine := dndRollSummaryLine(result); rollLine != "" {
		ob.WriteString("\n")
		ob.WriteString(rollLine)
	}
	recordZoneKillForUser(userID, monster.ID)
	applyRoomCombatThreatForUser(userID, elite)
	if drop := p.dropZoneLoot(userID, zone.ID, monster, false, elite); drop != "" {
		ob.WriteString("\n")
		ob.WriteString(drop)
	}
	outcome = ob.String()
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
	Nat20s, Nat1s        int     // pre-counted by scanMoodEventsFromCombat

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
	// Mood is a property of the run, not of the player who typed at TwinBee — so
	// a member's taunt or compliment moves the party's gauge. That is only safe
	// because applyMoodEvent lands an atomic `gm_mood + delta`; the decay write
	// it sits next to is absolute, and applyMoodDecayIfStale refuses it for a
	// non-owner for exactly that reason.
	run, isLeader, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run, isLeader)
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
	run, _, err := activeZoneRunFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	zone := zoneOrFallback(run.ZoneID)
	line := twinBeeLine(zone.ID, DMLore, run.RunID, narrationCadence(run))
	if line == "" {
		return p.SendDM(ctx.Sender, "I have nothing to say about this place.")
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("📖 **%s — Lore**\n\n%s", zone.Display, line))
}

// ── abandon ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) zoneCmdAbandon(ctx MessageContext) error {
	if isPartyMember(ctx.Sender) {
		return p.SendDM(ctx.Sender,
			"Only your party leader can call off the run. `!expedition leave` to walk out alone.")
	}
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
