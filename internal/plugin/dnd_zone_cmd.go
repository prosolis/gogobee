package plugin

import (
	"fmt"
	"strings"
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
//
// Combat resolution per room arrives in D1e; advance currently just
// records the room as cleared and reports the next room type.
// TwinBee narration / mood triggers arrive in D1d.

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
	case "enter", "go", "start":
		return p.zoneCmdEnter(ctx, c, rest)
	case "status", "info":
		return p.zoneCmdStatus(ctx)
	case "map", "m":
		return p.zoneCmdMap(ctx)
	case "advance", "next", "a":
		return p.zoneCmdAdvance(ctx)
	case "abandon", "leave", "quit":
		return p.zoneCmdAbandon(ctx)
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
	b.WriteString("`!zone abandon` — end the active run (no rewards)")
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
		zone, _ := getZone(active.ZoneID)
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
			zone, _ := getZone(active.ZoneID)
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
	zone, _ := getZone(run.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗝 **Entering %s** _(T%d, %d rooms)_\n\n", zone.Display, int(zone.Tier), run.TotalRooms))
	b.WriteString("_" + zone.Hook + "_\n\n")
	if line := twinBeeLine(zone.ID, GMRoomEntry, run.RunID, run.CurrentRoom); line != "" {
		b.WriteString(line)
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
	zone, _ := getZone(run.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s** — room %d/%d (%s)\n",
		zone.Display, run.CurrentRoom+1, run.TotalRooms, prettyRoomType(run.CurrentRoomType())))
	b.WriteString(fmt.Sprintf("Cleared: %d   Loot: %d   GM mood: %d/100 (%s)\n",
		len(run.RoomsCleared), len(run.LootCollected), run.GMMood, gmMoodLabel(run.GMMood)))
	b.WriteString(fmt.Sprintf("Started: %s   Last action: %s",
		run.StartedAt.Format("2006-01-02 15:04"),
		run.LastActionAt.Format("2006-01-02 15:04")))
	return p.SendDM(ctx.Sender, b.String())
}

// gmMoodLabel returns a human-friendly label for the mood gauge per
// design doc §3.2 mood bands (≥80 effusive, 60–79 friendly, 40–59 neutral,
// 20–39 grumpy, <20 hostile).
func gmMoodLabel(mood int) string {
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
	zone, _ := getZone(run.ZoneID)
	cleared := map[int]bool{}
	for _, r := range run.RoomsCleared {
		cleared[r] = true
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s — Map**\n```\n", zone.Display))
	for i, rt := range run.RoomSeq {
		marker := "  "
		switch {
		case cleared[i]:
			marker = "✓ "
		case i == run.CurrentRoom:
			marker = "▶ "
		}
		b.WriteString(fmt.Sprintf("%s[%d] %s\n", marker, i+1, prettyRoomType(rt)))
	}
	b.WriteString("```")
	return p.SendDM(ctx.Sender, b.String())
}

// ── advance ─────────────────────────────────────────────────────────────────

// zoneCmdAdvance is the D1c stub: it records the current room cleared and
// reports the next room. Real combat / trap / boss resolution wires in
// D1e+. This is intentional — D1c ships the *surface*.
func (p *AdventurePlugin) zoneCmdAdvance(ctx MessageContext) error {
	run, err := getActiveZoneRun(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read run state: "+err.Error())
	}
	if run == nil {
		return p.SendDM(ctx.Sender, "No active zone run. Use `!zone enter <id>`.")
	}
	_ = applyMoodDecayIfStale(run)
	zone, _ := getZone(run.ZoneID)
	prev := run.CurrentRoomType()
	prevIdx := run.CurrentRoom
	next, err := markRoomCleared(run.RunID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't advance: "+err.Error())
	}
	if next == "" {
		// Boss room cleared → zone complete. Bump mood per design §3.2.
		_, _ = applyMoodEvent(run.RunID, MoodEventZoneComplete)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("🏆 **Cleared %s.** Boss defeated. Run complete.\n\n", zone.Display))
		if line := twinBeeLine(zone.ID, GMZoneComplete, run.RunID, prevIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
		b.WriteString("_(Combat resolution + loot rolls land in D1e — for now this is a clean state-machine win.)_")
		return p.SendDM(ctx.Sender, b.String())
	}
	nextIdx := run.CurrentRoom + 1 // markRoomCleared already advanced; reflect for narration salt
	var b strings.Builder
	b.WriteString(fmt.Sprintf("✓ Cleared room %d (%s).\n\n", prevIdx+1, prettyRoomType(prev)))
	if next == RoomBoss {
		if line := twinBeeLine(zone.ID, GMBossEntry, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	} else {
		if line := twinBeeLine(zone.ID, GMRoomEntry, run.RunID, nextIdx); line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	}
	b.WriteString(fmt.Sprintf("**Room %d/%d — %s.** ", nextIdx+1, run.TotalRooms, prettyRoomType(next)))
	b.WriteString("`!zone advance` to continue.")
	return p.SendDM(ctx.Sender, b.String())
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
	zone, _ := getZone(run.ZoneID)
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"🚪 Abandoned **%s** at room %d/%d. No rewards.",
		zone.Display, run.CurrentRoom+1, run.TotalRooms))
}

