package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// !expedition — Phase 12 E1c. The command surface for the multi-day
// expedition system landed in E1a/E1b. This file is presentation +
// dispatch only; data plumbing lives in dnd_expedition.go and supply
// math in dnd_expedition_supplies.go.
//
// Subcommands:
//
//	!expedition                          → help (or status if active)
//	!expedition list                     → zones available at player level
//	!expedition start <zone> [Ns] [Md]   → outfit & start; default 1 standard pack
//	!expedition status                   → daily briefing-style block (§12.1)
//	!expedition log                      → last 5 expedition log entries
//	!expedition abandon                  → end the expedition (no rewards)
//	!expedition help                     → help text
//
// Day cycle (06:00/21:00 cron) lands in E1d. Camp commands (!camp) land
// in E1e. !advance / !search / !rest / !extract are out-of-scope for E1.

func (p *AdventurePlugin) handleDnDExpeditionCmd(ctx MessageContext, args string) error {
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
	case "":
		// If active, show status; otherwise help. A party member is on an
		// expedition they do not own, so ask the leader-or-member resolver.
		if exp, _, _ := activeExpeditionFor(ctx.Sender); exp != nil {
			return p.expeditionCmdStatus(ctx, "")
		}
		return p.SendDM(ctx.Sender, expeditionHelpText())
	case "help", "?":
		return p.SendDM(ctx.Sender, expeditionHelpText())
	case "list", "ls":
		return p.expeditionCmdList(ctx, c)
	case "start", "begin":
		return p.expeditionCmdStart(ctx, c, rest)
	case "go", "choose":
		// Context-sensitive: with an active expedition + numeric rest,
		// treat as "take fork path N" (forwards to zoneCmdGo so the
		// player doesn't have to remember the !zone seam). Otherwise
		// fall back to the historical alias for `!expedition start`.
		if rest != "" && isAllDigits(strings.TrimSpace(rest)) {
			exp, isLeader, _ := activeExpeditionFor(ctx.Sender)
			switch {
			case exp != nil && isLeader:
				return p.zoneCmdGo(ctx, rest)
			case exp != nil:
				// A member reaching a fork must not silently fall through to
				// `!expedition start` and be told their path number is an
				// unknown zone. The leader picks the way (C1); P6c enforces
				// that at the !zone seam too.
				return p.SendDM(ctx.Sender, "The party leader picks the path.")
			}
		}
		return p.expeditionCmdStart(ctx, c, rest)
	case "status", "info":
		return p.expeditionCmdStatus(ctx, rest)
	case "log", "history":
		return p.expeditionCmdLog(ctx)
	case "abandon", "quit":
		return p.expeditionCmdAbandon(ctx)
	case "invite":
		return p.expeditionCmdInvite(ctx, rest)
	case "accept", "join":
		return p.expeditionCmdAccept(ctx, c, rest)
	case "decline":
		return p.expeditionCmdDecline(ctx)
	case "party", "roster":
		return p.expeditionCmdParty(ctx)
	case "leave":
		return p.expeditionCmdLeave(ctx)
	case "extract":
		return p.handleExtractCmd(ctx, "")
	case "resume":
		return p.handleResumeCmd(ctx, rest)
	case "map", "m":
		return p.handleExpeditionMapCmd(ctx, "")
	case "run", "explore", "advance":
		return p.expeditionCmdRun(ctx)
	default:
		return p.SendDM(ctx.Sender, expeditionHelpText())
	}
}

func expeditionHelpText() string {
	var b strings.Builder
	b.WriteString("**!expedition** — multi-day dungeon expeditions.\n\n")
	b.WriteString("**The shape:** pick a zone, pick a supply pack, watch it play out. ")
	b.WriteString("Autopilot walks rooms, pitches camp at night, and DMs you when something needs a decision (a fork, a boss, low HP).\n\n")
	b.WriteString("**Run an expedition:**\n")
	b.WriteString("`!expedition list` — zones available at your level\n")
	b.WriteString("`!expedition start <zone>` — prompts a loadout: `lean` / `balanced` / `heavy`\n")
	b.WriteString("`!expedition run` — start (or resume) the autopilot walk (alias `!explore`)\n")
	b.WriteString("`!expedition status` — day, rooms, supplies, recent events\n\n")
	b.WriteString("**Bring someone** _(Day 1, up to three of you)_:\n")
	b.WriteString("`!expedition invite @user` — they buy their own supplies into the party pool\n")
	b.WriteString("`!expedition accept` / `!expedition decline` — answer an invite\n")
	b.WriteString("`!expedition party` — who's with you\n")
	b.WriteString("`!expedition leave` — turn back for town (members only)\n\n")
	b.WriteString("**Mid-expedition:**\n")
	b.WriteString("`!extract` — bail safely (resumable for 7 days)\n")
	b.WriteString("`!resume [loadout]` — resume an extracted expedition\n")
	b.WriteString("`!expedition log` — last 5 log entries\n")
	b.WriteString("`!expedition abandon` — end without rewards\n")
	b.WriteString("`!map` — region/room ASCII map\n\n")
	b.WriteString("**Overrides** _(autopilot covers these — only reach for them if you want manual control)_:\n")
	b.WriteString("`!fight` — engage an Elite/Boss the autopilot paused at\n")
	b.WriteString("`!camp` — force a rest right now (see `!camp` for types)\n")
	b.WriteString("`!expedition start <zone> Ns Md` — raw pack counts instead of a preset")
	return b.String()
}

// ── list ────────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionCmdList(ctx MessageContext, c *DnDCharacter) error {
	zones := zonesForLevel(c.Level)
	if len(zones) == 0 {
		return p.SendDM(ctx.Sender, "No zones available at your level. (This shouldn't happen — file a bug.)")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Expeditions available at L%d** (you can enter zones up to 2 tiers above your current tier):\n\n", c.Level))
	for i, z := range zones {
		suffix := ""
		if raidContentWarning(z.ID) != "" {
			suffix = "  _⚠ raid-shaped — solo runs not yet survivable_"
		}
		b.WriteString(fmt.Sprintf("**%d.** %s — _T%d, L%d–%d_  `!expedition start %s`%s\n",
			i+1, z.Display, int(z.Tier), z.LevelMin, z.LevelMax, z.ID, suffix))
		b.WriteString(fmt.Sprintf("    %s\n", z.Atmosphere))
	}
	if exp, _ := getActiveExpedition(ctx.Sender); exp != nil {
		zone := zoneOrFallback(exp.ZoneID)
		b.WriteString(fmt.Sprintf("\n_⚠ Active expedition: **%s**, day %d. Use `!expedition status` or `!expedition abandon`._",
			zone.Display, exp.CurrentDay))
	}
	return p.SendDM(ctx.Sender, b.String())
}

// ── start ───────────────────────────────────────────────────────────────────

// parseSupplyArgs reads a token stream like "2s 1d" into a SupplyPurchase.
// Tokens are case-insensitive; suffix s|standard or d|deluxe. Bare integer
// is interpreted as standard packs. Default (empty) is one standard pack.
func parseSupplyArgs(rest string) (SupplyPurchase, error) {
	p := SupplyPurchase{}
	if strings.TrimSpace(rest) == "" {
		p.StandardPacks = 1
		return p, nil
	}
	for _, tok := range strings.Fields(rest) {
		t := strings.ToLower(strings.TrimSpace(tok))
		if t == "" {
			continue
		}
		var (
			numPart string
			suf     string
		)
		// Find first non-digit position.
		i := 0
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		numPart = t[:i]
		suf = t[i:]
		if numPart == "" {
			return p, fmt.Errorf("can't parse pack token %q", tok)
		}
		n, err := strconv.Atoi(numPart)
		if err != nil {
			return p, fmt.Errorf("can't parse pack count in %q", tok)
		}
		switch suf {
		case "", "s", "std", "standard":
			p.StandardPacks += n
		case "d", "dlx", "deluxe":
			p.DeluxePacks += n
		default:
			return p, fmt.Errorf("unknown pack suffix in %q (use s or d)", tok)
		}
	}
	return p, nil
}

// resolveLoadoutOrParse first tries a single-token preset (lean/balanced/
// heavy and short forms); on miss it falls back to raw `Ns Md` parsing.
// Tier is needed because preset purchase counts scale by zone tier.
func resolveLoadoutOrParse(tok string, tier ZoneTier) (SupplyPurchase, error) {
	trimmed := strings.TrimSpace(tok)
	if !strings.ContainsAny(trimmed, " \t") {
		if l, ok := parseLoadoutToken(trimmed); ok {
			return loadoutPurchase(tier, l), nil
		}
	}
	return parseSupplyArgs(tok)
}

// renderLoadoutPrompt formats the "pick your loadout" DM. The resume
// command and start command share it; cmdHint tells the player which
// command to type back with the chosen preset.
func renderLoadoutPrompt(zone ZoneDefinition, cmdHint string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎒 **Pick a loadout — %s** _(T%d)_\n\n", zone.Display, int(zone.Tier)))
	for _, l := range []SupplyLoadout{LoadoutLean, LoadoutBalanced, LoadoutHeavy} {
		pp := loadoutPurchase(zone.Tier, l)
		b.WriteString(fmt.Sprintf("  `%s` — %s — %.0f SU, %d coins — %s\n",
			loadoutName(l), packBreakdown(pp), pp.Total(), pp.Cost(), loadoutBlurb(l)))
	}
	b.WriteString(fmt.Sprintf("\nType `!%s %s` (or `lean` / `heavy`).\n", cmdHint, loadoutName(LoadoutBalanced)))
	b.WriteString("Advanced: raw pack counts like `2s 1d`.")
	return b.String()
}

func loadoutName(l SupplyLoadout) string {
	switch l {
	case LoadoutLean:
		return "lean"
	case LoadoutHeavy:
		return "heavy"
	}
	return "balanced"
}

func loadoutBlurb(l SupplyLoadout) string {
	switch l {
	case LoadoutLean:
		return "covers the intended run at calm burn; thin if things go sideways"
	case LoadoutHeavy:
		return "max cap; rides out harsh stretches and overruns"
	}
	return "recommended; absorbs a harsh patch or two"
}

func packBreakdown(p SupplyPurchase) string {
	switch {
	case p.StandardPacks > 0 && p.DeluxePacks > 0:
		return fmt.Sprintf("%d standard + %d deluxe", p.StandardPacks, p.DeluxePacks)
	case p.DeluxePacks > 0:
		return fmt.Sprintf("%d deluxe", p.DeluxePacks)
	default:
		return fmt.Sprintf("%d standard", p.StandardPacks)
	}
}

func (p *AdventurePlugin) expeditionCmdStart(ctx MessageContext, c *DnDCharacter, rest string) error {
	if rest == "" {
		return p.SendDM(ctx.Sender,
			"`!expedition start <zone> [Ns] [Md]` — pick from `!expedition list`. Example: `!expedition start goblin_warrens 2s` (2 standard packs).")
	}
	if remaining := restingLockoutRemaining(c); remaining > 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"🛌 You're still resting — %s remaining. Pack up after.",
			formatRespecDuration(remaining)))
	}
	zoneTok, packTok := splitFirstWord(rest)
	available := zonesForLevel(c.Level)
	zoneID, ok := resolveZoneInput(zoneTok, available)
	if !ok {
		return p.SendDM(ctx.Sender,
			"Unknown zone for your level. Try `!expedition list`.")
	}
	zoneForCaps, _ := getZone(zoneID)
	// D5-b: prompt for a preset loadout on empty args. Raw `Ns Md` syntax
	// still works as the advanced override.
	if strings.TrimSpace(packTok) == "" {
		return p.SendDM(ctx.Sender, renderLoadoutPrompt(zoneForCaps, "expedition start "+string(zoneID)))
	}
	purchase, err := resolveLoadoutOrParse(packTok, zoneForCaps.Tier)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't parse supply packs: "+err.Error())
	}
	if err := purchase.Validate(zoneForCaps.Tier); err != nil {
		return p.SendDM(ctx.Sender, "Invalid pack selection: "+err.Error())
	}
	cost := float64(purchase.Cost())
	if p.euro == nil {
		return p.SendDM(ctx.Sender, "Coin system unavailable — try again later.")
	}
	if balance := p.euro.GetBalance(ctx.Sender); balance < cost {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Not enough coins. Outfitting costs **%d** but you have **%.0f**.",
			int(cost), balance))
	}

	// Reject if any expedition or zone run already active.
	if existing, _ := getActiveExpedition(ctx.Sender); existing != nil {
		zone, _ := getZone(existing.ZoneID)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You're already on expedition in **%s** (Day %d). Finish it or `!expedition abandon` first.",
			zone.Display, existing.CurrentDay))
	}
	if existing, _ := getActiveZoneRun(ctx.Sender); existing != nil {
		zone, _ := getZone(existing.ZoneID)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You have an active single-session zone run in **%s**. Finish or `!zone abandon` before starting an expedition.",
			zone.Display))
	}

	zone := zoneForCaps
	// Holiday perk: a complimentary standard pack is added to the supplies
	// snapshot without inflating the coin cost. Bypasses the per-tier cap
	// on purpose — it's a freebie on top of whatever the player bought.
	suppliesPurchase := purchase
	if isHol, _ := isHolidayToday(); isHol {
		suppliesPurchase.StandardPacks++
	}
	supplies := makeSupplies(zone.Tier, suppliesPurchase)

	// Debit coins; bail on debit failure (race / cap).
	if !p.euro.Debit(ctx.Sender, cost, "expedition outfitting: "+string(zoneID)) {
		return p.SendDM(ctx.Sender, "Couldn't debit outfitting cost (try again).")
	}

	exp, err := startExpedition(ctx.Sender, zoneID, "", supplies)
	if err != nil {
		// Refund on persistence failure.
		p.euro.Credit(ctx.Sender, cost, "expedition outfitting refund")
		return p.SendDM(ctx.Sender, "Couldn't start expedition: "+err.Error())
	}

	// R2 — auto-spawn the DungeonRun for the starting region. Per-room
	// harvest depends on this; the existing zone-run !advance/!retreat
	// commands now operate on this run while the expedition is active.
	if _, err := ensureRegionRun(exp, c.Level); err != nil {
		// Refund and tear the expedition row back down — without a
		// linked run, harvest and rooms can't function.
		_ = abandonExpedition(ctx.Sender)
		p.euro.Credit(ctx.Sender, cost, "expedition outfitting refund (run-spawn failed)")
		return p.SendDM(ctx.Sender, "Couldn't outfit the first region: "+err.Error())
	}

	// Log the start with prewritten flavor.
	startLine := flavor.Pick(flavor.ExpeditionStart)
	_ = appendExpeditionLog(exp.ID, 1, "narrative", "expedition started", startLine)
	markActedToday(ctx.Sender)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗺 **Expedition begins — %s** _(T%d)_\n\n", zone.Display, int(zone.Tier)))
	b.WriteString("_" + zone.Hook + "_\n\n")
	b.WriteString(fmt.Sprintf("**Outfitted:** %.0f SU (%d standard, %d deluxe) — %d coins\n",
		supplies.Max, purchase.StandardPacks, purchase.DeluxePacks, purchase.Cost()))
	b.WriteString(fmt.Sprintf("**Daily burn:** %.1f SU/day — that's roughly **%d days** of provisions.\n\n",
		supplies.DailyBurn, estimateDays(supplies.Max, supplies.DailyBurn)))
	if startLine != "" {
		b.WriteString(startLine)
		b.WriteString("\n\n")
	}
	if w := raidContentWarning(zoneID); w != "" {
		b.WriteString(w)
		b.WriteString("\n\n")
	}
	b.WriteString("Use `!expedition status` for the daily briefing format. Day 1 begins now.")
	return p.SendDM(ctx.Sender, b.String())
}

// raidContentWarning returns a TwinBee-voiced heads-up for zones whose
// boss is tuned for a party rather than a solo adventurer. T5 zones
// (Dragon's Lair / Abyss Portal) have boss HP / damage curves that the
// solo combat path can't realistically clear — the J3 trace sweep at
// L12 showed 0% solo clears across all 10 classes. Until multiplayer
// expeditions ship, this is the surface that tells a player what
// they're walking into without nerfing the encounter for parties later.
func raidContentWarning(zoneID ZoneID) string {
	switch zoneID {
	case ZoneDragonsLair:
		return "⚠ A note before we commit. Infernax doesn't go down to one sword. I've watched better-prepared adventurers walk in here and not walk back out, and I haven't yet seen the lone exception. Bring friends when you can. For tonight — I'm with you anyway."
	case ZoneAbyssPortal:
		return "⚠ A note before we commit. Belaxath is the kind of enemy you bring a band to. Not one solo hero has put him down yet, and I'd rather you weren't the first to try. We can still go. I just want the record to show I said this."
	}
	return ""
}

func estimateDays(maxSU, dailyBurn float32) int {
	if dailyBurn <= 0 {
		return 0
	}
	return int(math.Floor(float64(maxSU / dailyBurn)))
}

// ── status ──────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionCmdStatus(ctx MessageContext, args string) error {
	debug := false
	for _, tok := range strings.Fields(args) {
		switch strings.ToLower(tok) {
		case "--debug", "-d", "debug", "raw":
			debug = true
		}
	}
	return p.expeditionCmdStatusImpl(ctx, debug)
}

func (p *AdventurePlugin) expeditionCmdStatusImpl(ctx MessageContext, debug bool) error {
	exp, _, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender,
			"No active expedition. Use `!expedition list` then `!expedition start <zone>`.")
	}
	zone, _ := getZone(exp.ZoneID)
	c, _ := LoadDnDCharacter(ctx.Sender)

	target := expeditionTargetDays(zone.Tier)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎭 **TwinBee — Status, Day %d / ~%d expected**\n\n", exp.CurrentDay, target))
	b.WriteString(fmt.Sprintf("📍 **Zone:** %s _(T%d)_\n", zone.Display, int(zone.Tier)))
	if r, ok := CurrentRegion(exp); ok {
		cleared := IsRegionCleared(exp, r.ID)
		marker := ""
		if cleared {
			marker = " ✓"
		}
		b.WriteString(fmt.Sprintf("🗺 **Region:** %s (%d/%d)%s\n",
			r.Name, r.Order, len(regionsForZone(exp.ZoneID)), marker))
	}
	if run, _, rerr := activeZoneRunFor(ctx.Sender); rerr == nil && run != nil && run.TotalRooms > 0 {
		b.WriteString(fmt.Sprintf("🚪 **Rooms:** %d / %d in this region\n",
			run.CurrentRoom+1, run.TotalRooms))
	}
	if c != nil {
		b.WriteString(fmt.Sprintf("❤️  **HP:** %d / %d\n", c.HPCurrent, c.HPMax))
	}
	// Default view is verb/outcome-led: days-left summary + threat label,
	// no raw SU / threat-out-of-100 / zone-stack / roll-modifier numbers.
	// Pass `--debug` to !expedition status for the engineering view.
	days := estimateDays(exp.Supplies.Current, currentBurn(exp))
	if debug {
		b.WriteString(fmt.Sprintf("🎒 **Supplies:** %.1f / %.1f SU  _(burn %.1f/day → ~%d days left)_\n",
			exp.Supplies.Current, exp.Supplies.Max, currentBurn(exp), days))
		b.WriteString(fmt.Sprintf("⏰ **Threat:** %d / 100 — %s\n",
			exp.ThreatLevel, threatThresholdLabel(exp.ThreatLevel, exp.SiegeMode)))
		if exp.TemporalStack != 0 {
			b.WriteString(fmt.Sprintf("🌡 **Zone stack:** %d\n", exp.TemporalStack))
		}
	} else {
		b.WriteString(fmt.Sprintf("🎒 **Supplies:** ~%d day%s left\n",
			days, plural(days)))
		b.WriteString(fmt.Sprintf("⏰ **Threat:** %s\n",
			threatThresholdLabel(exp.ThreatLevel, exp.SiegeMode)))
	}
	if exp.Camp != nil && exp.Camp.Active {
		b.WriteString(fmt.Sprintf("⛺ **Camp:** %s (room %d)\n", exp.Camp.Type, exp.Camp.RoomIndex+1))
	}
	state := supplyDepletion(exp.Supplies)
	if state != SupplyNormal {
		if debug {
			b.WriteString(fmt.Sprintf("⚠ **%s** — roll modifier %d\n",
				depletionLabel(state), supplyRollModifier(state)))
		} else {
			b.WriteString(fmt.Sprintf("⚠ **%s** — you're slower and clumsier than usual.\n",
				depletionLabel(state)))
		}
	}
	if entries, lerr := recentExpeditionLog(exp.ID, 3); lerr == nil && len(entries) > 0 {
		b.WriteString("\n**Recent:**\n")
		for _, e := range entries {
			line := e.Summary
			if line == "" {
				line = e.Flavor
			}
			if line == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("  · _Day %d_ — %s\n", e.Day, line))
		}
	}
	b.WriteString(fmt.Sprintf("\nStarted: %s   Last activity: %s",
		exp.StartDate.Format("2006-01-02 15:04"),
		exp.LastActivity.Format("2006-01-02 15:04")))
	return p.SendDM(ctx.Sender, b.String())
}

func currentBurn(exp *Expedition) float32 {
	burn := exp.Supplies.DailyBurn
	if exp.SiegeMode {
		// §8.3: siege doubles supply burn outright, overriding HarshMod
		// (which can be < 2 at low tiers).
		mult := exp.Supplies.HarshMod
		if mult < 2 {
			mult = 2
		}
		return burn * mult
	}
	if exp.ThreatLevel > 60 {
		// Harsh conditions: §4.1.
		mult := exp.Supplies.HarshMod
		if mult <= 0 {
			mult = 1
		}
		burn *= mult
	}
	return burn
}

func threatThresholdLabel(level int, siege bool) string {
	if siege {
		return "Siege"
	}
	switch {
	case level >= 81:
		return "Siege"
	case level >= 61:
		return "Hostile"
	case level >= 41:
		return "Alert"
	case level >= 21:
		return "Stirring"
	default:
		return "Quiet"
	}
}

func depletionLabel(s SupplyDepletionState) string {
	switch s {
	case SupplyRationing:
		return "Rationing"
	case SupplySevereRationing:
		return "Severe rationing"
	case SupplyStarvation:
		return "Starvation"
	}
	return "Normal"
}

// ── log ─────────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionCmdLog(ctx MessageContext) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition.")
	}
	entries, err := recentExpeditionLog(exp.ID, 5)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read log: "+err.Error())
	}
	if len(entries) == 0 {
		return p.SendDM(ctx.Sender, "No log entries yet.")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📜 **Expedition log — last %d entries**\n\n", len(entries)))
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("**Day %d** — _%s_ (%s)\n",
			e.Day, e.Type, formatLogTimestamp(e.Timestamp)))
		if e.Summary != "" {
			b.WriteString("  " + e.Summary + "\n")
		}
		if e.Flavor != "" {
			b.WriteString("  _" + e.Flavor + "_\n")
		}
	}
	return p.SendDM(ctx.Sender, b.String())
}

func formatLogTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// ── abandon ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionCmdAbandon(ctx MessageContext) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition to abandon.")
	}
	zone, _ := getZone(exp.ZoneID)
	if err := abandonExpedition(ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't abandon: "+err.Error())
	}
	markActedToday(ctx.Sender)
	_ = retireAllRegionRuns(exp)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative", "expedition abandoned", "")
	if err := p.SendDM(ctx.Sender, fmt.Sprintf(
		"Expedition in **%s** abandoned on Day %d. Supplies are forfeit. The dungeon remembers.",
		zone.Display, exp.CurrentDay)); err != nil {
		return err
	}
	// Emergence seam: see maybeRollPetArrivalOnEmerge.
	p.maybeRollPetArrivalOnEmerge(ctx.Sender)
	return nil
}

// helper: ensure we don't shadow id.UserID import in test harness.
var _ id.UserID

// isAllDigits reports whether s is a non-empty string of ASCII digits.
// Used to route `!expedition go N` to the fork-choice handler when N is
// numeric; non-numeric rest (`!expedition go ironforge`) still falls
// through to expeditionCmdStart.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ── run (autopilot) ─────────────────────────────────────────────────────────

// autopilotRoomCap bounds a single `!expedition run` invocation. Real-time
// background ticking is the planned long-game (see chat 2026-05-14); this
// cap keeps the foreground walk from monopolising the bot for an
// unbounded stretch and gives the player a natural "press to continue"
// breath between long runs.
const autopilotRoomCap = 6

// autopilotLowHPPct stops the walk when current HP drops at or below this
// fraction of max. 0.30 = "you're hurt enough that the next bad room
// could end the run; pause, heal, or commit."
const autopilotLowHPPct = 0.30

// autopilotWalkResult bundles the staged narration plus structured
// metadata so foreground (streamFlow) and background (single DM, with
// progress-based DM suppression) callers can share the same walk loop.
type autopilotWalkResult struct {
	stream   []string
	finalMsg string
	rooms    int
	reason   stopReason
	// initErr — populated when the walk couldn't start (no expedition,
	// no run). Foreground surfaces it as a DM; background swallows it.
	initErr string
}

// expeditionCmdRun is the autopilot surface. It loops advanceOnce until a
// natural interrupt fires (fork, elite/boss doorway, death, complete) or
// an injected interrupt fires (low HP, low SU, room cap). Each iteration's
// staged narration is concatenated into a single 2–3s-paced stream so the
// player reads the full multi-room walk as one continuous beat. Trash
// combat already auto-resolves inside resolveCombatRoom; elite/boss
// doorways stop here so the player can choose !fight on their own terms.
func (p *AdventurePlugin) expeditionCmdRun(ctx MessageContext) error {
	r := p.runAutopilotWalk(ctx, autopilotRoomCap, false, false)
	if r.initErr != "" {
		return p.SendDM(ctx.Sender, r.initErr)
	}
	// Emergence seam: a natural run-complete (boss down / dead-end node)
	// surfaces the player alive just like an extract or abandon — roll pet
	// arrival here too. The roll lives in the real callers, not in
	// runAutopilotWalk, so the sim path (which calls the walk directly)
	// never fires arrival DMs. See maybeRollPetArrivalOnEmerge. Defer it
	// behind the paced stream so the "animal in your house" DM lands after
	// the "Run complete" beat, not before it.
	var after func()
	if r.reason == stopComplete {
		uid := ctx.Sender
		after = func() { p.maybeRollPetArrivalOnEmerge(uid) }
	}
	return p.streamFlowThen(ctx.Sender, r.stream, r.finalMsg, after)
}

// runAutopilotWalk runs the autopilot loop up to maxRooms times and
// returns a bundle the caller can either streamFlow (foreground) or
// post as a single DM (background ticker). Pure side effects on the
// run graph / harvest tally / supplies / threat — same as before, just
// no streamFlow here. compact==true switches the underlying combat
// narration into terse mode and auto-resolves elite (not boss) rooms.
// forkAutoPickTimeout — how long a background fork may sit unanswered
// before the autopilot picks an available route itself. Short enough that
// the expedition keeps moving rather than idling out to the 24h stale-run
// reaper; long enough that a player away for the evening still gets first
// say on a genuine fork.
const forkAutoPickTimeout = 8 * time.Hour

// autoPickStaleFork commits the first unlocked option of a stale background
// fork, advancing the run to that node exactly as `!zone go <n>` would
// (advanceZoneRunNode + region-transition hook). Returns false — no pick —
// when every option is locked, so the caller re-emits the prompt and the
// run idles on toward the reaper. The choice is logged as a narrative entry
// so the end-of-day digest can surface the decision the player missed.
func (p *AdventurePlugin) autoPickStaleFork(exp *Expedition, run *DungeonRun, pf *pendingFork) bool {
	var chosen *pendingChoice
	for i := range pf.Options {
		if pf.Options[i].Unlocked {
			chosen = &pf.Options[i]
			break
		}
	}
	if chosen == nil {
		return false // nothing unlocked — leave it for the player / reaper
	}
	if _, err := advanceZoneRunNode(run.RunID, chosen.To); err != nil {
		slog.Warn("expedition: auto-pick stale fork",
			"user", run.UserID, "run", run.RunID, "err", err)
		return false
	}
	g, _ := loadZoneGraph(run.ZoneID)
	fireGraphRegionTransition(run.UserID, g.Nodes[run.CurrentNode], g.Nodes[chosen.To])
	if exp != nil {
		_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
			fmt.Sprintf("autopilot took an available path after %dh idle at the fork: %s",
				int(forkAutoPickTimeout.Hours()), chosen.Label), "")
	}
	return true
}

func (p *AdventurePlugin) runAutopilotWalk(ctx MessageContext, maxRooms int, compact, inlineBossCombat bool) autopilotWalkResult {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return autopilotWalkResult{initErr: "Couldn't read expedition state: " + err.Error()}
	}
	if exp == nil {
		return autopilotWalkResult{initErr: "You're not on an expedition. `!expedition list` to pick one."}
	}
	if exp.RunID == "" {
		return autopilotWalkResult{initErr: "No active region run. Try `!region` to refresh."}
	}

	// Already standing at a pending fork. The autopilot can't pick for the
	// player, so a fresh fork re-emits the prompt with rooms=0 (background
	// DM suppression keeps quiet; the rooms counter doesn't tick on a no-op
	// walk). But a background fork left unanswered past forkAutoPickTimeout
	// would otherwise idle all the way to the 24h stale-run reaper and end
	// the expedition — so once it's stale, auto-pick the first available
	// (unlocked) route and keep walking instead of stalling out.
	if run, rerr := getActiveZoneRun(ctx.Sender); rerr == nil && run != nil {
		if pf, derr := decodePendingFork(run.NodeChoices); derr == nil && pf != nil {
			picked := compact &&
				time.Since(run.LastActionAt) > forkAutoPickTimeout &&
				p.autoPickStaleFork(exp, run, pf)
			if !picked {
				zone := zoneOrFallback(run.ZoneID)
				return autopilotWalkResult{
					finalMsg: renderForkPrompt(zone, *pf),
					rooms:    0,
					reason:   stopFork,
				}
			}
			// Auto-picked: the run now points at the chosen node. Fall
			// through into the walk loop so this same tick advances it.
		}
	}

	var stream []string
	var finalMsg string
	rooms := 0
	reason := stopOK // sticky: last loop-exit reason

	// Phase 2 — walk-wide auto-harvest tally. Per-room footers go into the
	// stream as we go; the cumulative haul renders on the final block.
	walkYields := map[string]int{}
	walkNames := map[string]string{}

	for i := 0; i < maxRooms; i++ {
		// Pre-iteration interrupts: low HP / low SU. Skip on the first
		// iteration so the player always gets at least one room out of
		// `!expedition run` even if they limped in — autopilot stops on
		// thresholds *between* rooms, not before the first one.
		if i > 0 {
			if msg, stop := autopilotPreflight(ctx.Sender, exp); stop {
				finalMsg = msg
				reason = stopPreflight
				break
			}
		}

		res, aerr := p.advanceOnceWithOpts(ctx, compact, inlineBossCombat)
		if aerr != nil {
			return autopilotWalkResult{initErr: aerr.Error()}
		}

		// Roll this step's beats into the running stream.
		stream = append(stream, res.preStream...)
		if res.intro != "" {
			stream = append(stream, res.intro)
		}
		stream = append(stream, res.phases...)

		// Doorway/blocked stops fire *before* the current room actually
		// resolved — those don't count as a walked room. Everything else
		// (OK, fork after a clear, ended after combat, complete) does.
		// stopBossSafety also fires at the doorway (compact autopilot
		// bailed before engaging), so it doesn't count either.
		if res.reason != stopBlocked && res.reason != stopElite && res.reason != stopBoss && res.reason != stopBossSafety {
			rooms++
		}

		// Multi-region auto-advance: a mid-zone region clear completes the
		// region's run (stopComplete) but leaves the wrapping expedition
		// active. Rather than dead-stopping the walk at every region
		// boundary, cross into the next region — burning the transit day +
		// supplies exactly like manual `!region travel` — and keep walking
		// within the remaining room budget. A full zone clear instead flips
		// the expedition to 'complete' (getActiveExpedition → nil) and falls
		// through to the normal stop below.
		if res.reason == stopComplete {
			if fresh, ferr := getActiveExpedition(ctx.Sender); ferr == nil && fresh != nil &&
				IsMultiRegionZone(fresh.ZoneID) {
				if cur, ok := CurrentRegion(fresh); ok {
					if next, ok := nextRegion(fresh.ZoneID, cur.ID); ok {
						// A region crossing burns a transit day + supplies and
						// draws unprotected wandering damage. On the background
						// walk, don't cross while the player is weak — preflight
						// HP/SU and hand the crossing back to a foreground
						// `!region travel` / `!expedition run` if either is low.
						if compact {
							if msg, stop := autopilotPreflight(ctx.Sender, fresh); stop {
								finalMsg = res.final + "\n\n" + msg
								reason = stopPreflight
								break
							}
						}
						stream = append(stream, res.final)
						transit, terr := p.advanceToNextRegion(ctx.Sender, fresh, cur, next)
						if terr != nil {
							finalMsg = res.final + "\n\n⏸ **Autopilot paused — region transit failed.** `!region travel` to cross over manually."
							reason = stopComplete
							break
						}
						stream = append(stream, transit)
						exp = fresh
						continue
					}
				}
			}
		}

		if res.reason != stopOK {
			footer := autopilotFooter(res.reason, rooms)
			if footer != "" {
				finalMsg = res.final + "\n\n" + footer
			} else {
				finalMsg = res.final
			}
			reason = res.reason
			break
		}

		// Walked into the next room. Push this step's "✓ cleared / next
		// room" block as a phase so the next iteration's intro lands
		// after it, then loop. Refresh exp for the next preflight read
		// (HP/SU may have moved during combat).
		stream = append(stream, res.final)
		if fresh, ferr := getActiveExpedition(ctx.Sender); ferr == nil && fresh != nil {
			exp = fresh
		}

		// Arrived at an Elite/Boss doorway. Foreground stops here so the
		// player can decide; the "Room X/Y — Boss. !fight when ready."
		// line in res.final already tells them what to do.
		//
		// In compact mode (background autopilot, long-expedition D2/D3)
		// we let the next iteration run because the gate will auto-
		// resolve the encounter inline — elite always, boss when the
		// safety check passes (otherwise the gate returns stopBossSafety
		// and the autorun ticker pitches a rest camp).
		if !compact && (res.nextRoomType == RoomBoss || res.nextRoomType == RoomElite) {
			r := stopBoss
			if res.nextRoomType == RoomElite {
				r = stopElite
			}
			finalMsg = autopilotFooter(r, rooms)
			reason = r
			break
		}

		// Phase 2 — auto-harvest the room we just walked into.
		// advanceOnceWithOpts now runs the pass inline and surfaces the
		// result via res.harvest + res.harvestFooter. The "✓ cleared /
		// next room" line in res.final already has the per-room footer
		// appended (and the combat-interrupt narration if one fired);
		// we still need to aggregate into walkYields/walkNames and to
		// split out the harvest-combat path so the walk-end footer/tally
		// rendering matches the pre-H2 behavior.
		hr := res.harvest
		for k, v := range hr.Summary.Yields {
			walkYields[k] += v
			walkNames[k] = hr.Summary.Names[k]
		}
		if hr.CombatNarr != "" {
			// A harvest interrupt fired combat. res.final already
			// includes the ✓-cleared block, the harvest footer, and the
			// combat narration; just attach the walk footer/tally and
			// stop.
			r := stopHarvestCombat
			if hr.PlayerEnded {
				r = stopEnded
			}
			footer := autopilotFooter(r, rooms)
			finalMsg = res.final
			if footer != "" {
				finalMsg += "\n\n" + footer
			}
			if tally := renderWalkTally(walkYields, walkNames); tally != "" && !hr.PlayerEnded {
				finalMsg += "\n\n" + tally
			}
			// Drop the just-pushed res.final from the stream so the
			// combined finalMsg lands as the closer, not as a duplicate
			// phase entry.
			if len(stream) > 0 && stream[len(stream)-1] == res.final {
				stream = stream[:len(stream)-1]
			}
			reason = r
			break
		}
		// Refresh exp once more — auto-harvest may have bumped threat
		// via noise interrupts.
		if fresh, ferr := getActiveExpedition(ctx.Sender); ferr == nil && fresh != nil {
			exp = fresh
		}
	}

	if finalMsg == "" {
		// Hit the room cap without a hard interrupt — synthesize a
		// "press to continue" footer so the player knows autopilot
		// stopped on its own clock, not because something needs them.
		finalMsg = autopilotFooter(stopOK, rooms)
	}
	// Walk-wide haul tally — appended to whatever the final block is,
	// unless a combat-interrupt branch already added it (death suppresses
	// it; harvest-combat survival adds it inline).
	if tally := renderWalkTally(walkYields, walkNames); tally != "" &&
		!strings.Contains(finalMsg, "Walk haul:") {
		finalMsg += "\n\n" + tally
	}
	return autopilotWalkResult{
		stream:   stream,
		finalMsg: finalMsg,
		rooms:    rooms,
		reason:   reason,
	}
}

// autopilotPreflight checks the threshold-based interrupts that fire
// between rooms. Returns (footer, true) when autopilot should stop.
func autopilotPreflight(userID id.UserID, exp *Expedition) (string, bool) {
	cur, max := dndHPSnapshot(userID)
	if max > 0 && float64(cur) <= float64(max)*autopilotLowHPPct {
		return fmt.Sprintf(
			"⏸ **Autopilot paused — HP low** (%d/%d). `!camp` to rest, `!cast` healing, or `!expedition run` to push on.",
			cur, max), true
	}
	if exp.Supplies.DailyBurn > 0 && exp.Supplies.Current < exp.Supplies.DailyBurn {
		return fmt.Sprintf(
			"⏸ **Autopilot paused — supplies low** (%.1f / %.1f SU, under one day). `!extract` to bail or `!expedition run` to push on.",
			exp.Supplies.Current, exp.Supplies.DailyBurn), true
	}
	return "", false
}

// autopilotFooter renders the closing line for an autopilot walk. The
// rooms-walked tally is informational; reason controls the verb and the
// suggested next step.
func autopilotFooter(reason stopReason, rooms int) string {
	roomsStr := "1 room"
	if rooms != 1 {
		roomsStr = fmt.Sprintf("%d rooms", rooms)
	}
	switch reason {
	case stopFork:
		return fmt.Sprintf("⏸ **Autopilot paused at a fork** (after %s). `!expedition go <n>` to choose; auto-walk resumes automatically.", roomsStr)
	case stopElite:
		return fmt.Sprintf("⏸ **Autopilot paused — elite ahead** (after %s). `!fight` when ready, then `!expedition run` to continue.", roomsStr)
	case stopBoss:
		return fmt.Sprintf("⏸ **Autopilot paused — boss ahead** (after %s). `!fight` when ready.", roomsStr)
	case stopBossSafety:
		return "" // res.final already carries the held-back-from-boss line
	case stopEnded:
		return "" // death narration is the final; no footer
	case stopComplete:
		return "" // run-complete block is the final; no footer
	case stopBlocked:
		return "" // "finish your fight first" is the final; no footer
	case stopHarvestCombat:
		return fmt.Sprintf("⏸ **Autopilot paused — interrupted while gathering** (after %s). Reassess and `!expedition run` to continue.", roomsStr)
	default: // stopOK — hit the room cap
		return fmt.Sprintf("⏸ **Autopilot stretch complete** (%s). `!expedition run` to keep walking, or `!resources` / `!camp` first.", roomsStr)
	}
}
