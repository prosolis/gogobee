package plugin

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
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
	args = strings.TrimSpace(args)
	sub, rest := splitFirstWord(args)

	// Two subcommands are aliases for top-level commands that take the per-user
	// lock themselves. Dispatch them BEFORE we take it: advUserLock is a plain
	// sync.Mutex, so grabbing it here and again in there does not merely block —
	// it wedges the lock forever, because the deferred Unlock below never runs.
	// Every later `!adventure` / `!expedition` / `!zone` command from that player
	// then hangs too. Both aliases load their own character, so nothing below is
	// being skipped.
	switch strings.ToLower(sub) {
	case "extract":
		return p.handleExtractCmd(ctx, "")
	case "resume":
		return p.handleResumeCmd(ctx, rest)
	}

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
	case "hire":
		return p.expeditionCmdHire(ctx, rest)
	case "dismiss":
		return p.expeditionCmdDismiss(ctx)
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
	b.WriteString("`!expedition leave` — turn back for town (members only)\n")
	b.WriteString("`!expedition hire [class]` — pay Pete to fill the role you're missing\n")
	b.WriteString("`!expedition dismiss` — send Pete home\n\n")
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
	// Post-game zones under the "Beyond the Map" divider (see zoneCmdList).
	if pg := postgameZones(); len(pg) > 0 {
		unlocked, reason := postgameUnlocked(ctx.Sender, c.Level)
		b.WriteString("\n— Beyond the Map —\n")
		if !unlocked {
			b.WriteString(fmt.Sprintf("_%s_\n", reason))
		}
		for j, z := range pg {
			if unlocked {
				b.WriteString(fmt.Sprintf("**%d.** %s — _T%d, L%d+_  `!expedition start %s`\n",
					len(zones)+j+1, z.Display, int(z.Tier), z.LevelMin, z.ID))
			} else {
				b.WriteString(fmt.Sprintf("🔒 %s — _T%d, L%d+_\n", z.Display, int(z.Tier), z.LevelMin))
			}
			b.WriteString(fmt.Sprintf("    %s\n", z.Atmosphere))
		}
	}
	if exp, isLeader, _ := activeExpeditionFor(ctx.Sender); exp != nil {
		zone := zoneOrFallback(exp.ZoneID)
		tail := "Use `!expedition status` or `!expedition abandon`."
		if !isLeader {
			tail = "Use `!expedition status` or `!expedition leave`."
		}
		b.WriteString(fmt.Sprintf("\n_⚠ Active expedition: **%s**, day %d. %s_",
			zone.Display, exp.CurrentDay, tail))
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
	// N5/D1c: the campaign finale is reached here but is not a normal zone — it
	// runs a single auto-resolved boss fight with its own unlock gate, no
	// supplies, no walk. Intercept before the zone/loadout machinery.
	if strings.EqualFold(zoneTok, "epilogue") {
		return p.handleEpilogueEncounter(ctx)
	}
	available := availableZonesFor(ctx.Sender, c.Level)
	zoneID, ok := resolveZoneInput(zoneTok, available)
	if !ok {
		if reason := postgameLockReason(zoneTok, ctx.Sender, c.Level); reason != "" {
			return p.SendDM(ctx.Sender, reason)
		}
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

	out, err := p.performExpeditionStart(ctx.Sender, c, zoneForCaps, purchase, "")
	if err != nil {
		// Every refusal below carries its own finished sentence — see the sentinel
		// block on performExpeditionStart — so the command only has to say it.
		return p.SendDM(ctx.Sender, err.Error())
	}
	zone, supplies, startLine := out.Zone, out.Supplies, out.StartLine

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

// ── the headless twin of `!expedition start` ────────────────────────────────

// Sentinels for the ways outfitting can be refused, so the web action queue
// (pete_orders.go) can pick a verdict without parsing prose. Every refusal is
// returned as an advRefusal, which wraps one of these AND carries the
// finished player-facing sentence — that is how `!expedition start` keeps the
// exact copy it always sent while the web gets a machine-readable answer.
var (
	errExpStartResting    = errors.New("expedition start: still resting")
	errExpStartZoneLocked = errors.New("expedition start: zone not available at this level")
	errExpStartBadPacks   = errors.New("expedition start: invalid pack selection")
	errExpStartBusy       = errors.New("expedition start: already adventuring")
	errExpStartBroke      = errors.New("expedition start: cannot cover outfitting")
	errExpStartFailed     = errors.New("expedition start: could not outfit")
)

// advRefusal is a refusal that is both classifiable and quotable: errors.Is
// picks the verdict, Error() is the sentence the command has always sent. Shared
// by every headless twin in the web action family (start, resume, babysit).
type advRefusal struct {
	kind error
	msg  string
}

func (e advRefusal) Error() string { return e.msg }
func (e advRefusal) Unwrap() error { return e.kind }

func refuseAdv(kind error, format string, args ...any) error {
	return advRefusal{kind: kind, msg: fmt.Sprintf(format, args...)}
}

// expStartOutcome is what outfitting did, for a caller describing it somewhere
// other than a DM.
type expStartOutcome struct {
	Zone      ZoneDefinition
	Supplies  ExpeditionSupplies
	Cost      int
	Days      int
	StartLine string
}

// performExpeditionStart is `!expedition start` minus the command framing: the
// eligibility guards, the price gate, the debit, and the expedition row. It is
// shared with the web action queue so that leaving town from a phone is the
// *same* departure — same guards, same supplies, same opening log line.
//
// idemKey, when set, is the web order's guid: the debit then goes through
// DebitIdem so a re-offered order that already paid cannot pay twice. The Matrix
// command passes "" and keeps the plain debit, which is right — a Matrix message
// arrives exactly once.
//
// LOCKING, and this is the one asymmetry in the headless-twin family: this
// function does NOT take the per-user lock. performExtraction, takeSiegeBout and
// performBabysitPurchase all take it themselves, because their command framings
// do not hold it — but handleDnDExpeditionCmd holds it across its whole switch,
// so taking it here would wedge advUserLock permanently (it is a plain
// sync.Mutex, and the deferred unlock up there would never run). The web caller
// takes it explicitly instead; see applyAdvOrder.
func (p *AdventurePlugin) performExpeditionStart(uid id.UserID, c *DnDCharacter, zone ZoneDefinition, purchase SupplyPurchase, idemKey string) (expStartOutcome, error) {
	if remaining := restingLockoutRemaining(c); remaining > 0 {
		return expStartOutcome{}, refuseAdv(errExpStartResting,
			"🛌 You're still resting — %s remaining. Pack up after.",
			formatRespecDuration(remaining))
	}
	// Re-resolve availability against the game's own tables rather than trusting
	// the caller. The web resolves a zone from an offer list gogobee itself
	// pushed, but that snapshot can be minutes old and is not a permission.
	if _, ok := resolveZoneInput(string(zone.ID), availableZonesFor(uid, c.Level)); !ok {
		if reason := postgameLockReason(string(zone.ID), uid, c.Level); reason != "" {
			return expStartOutcome{}, refuseAdv(errExpStartZoneLocked, "%s", reason)
		}
		return expStartOutcome{}, refuseAdv(errExpStartZoneLocked,
			"Unknown zone for your level. Try `!expedition list`.")
	}
	if err := purchase.Validate(zone.Tier); err != nil {
		return expStartOutcome{}, refuseAdv(errExpStartBadPacks,
			"Invalid pack selection: %s", err.Error())
	}
	// Reject if any expedition or zone run already active. This runs before the
	// price quote: a player who cannot leave doesn't need to hear what leaving
	// would have cost.
	//
	// The seat check spans `extracting` as well as `active` — a member of an
	// extracting party is still seated for the seven-day resume window, and
	// letting them outfit a rival expedition double-books them the moment their
	// leader types `!resume`.
	if seated, _ := seatedExpeditionFor(uid); seated != nil {
		z, _ := getZone(seated.ZoneID)
		return expStartOutcome{}, refuseAdv(errExpStartBusy,
			"You're riding a party expedition in **%s** (Day %d). `!expedition leave` before starting your own.",
			z.Display, seated.CurrentDay)
	}
	if existing, _ := getActiveExpedition(uid); existing != nil {
		// A web order that already paid and already started this expedition on an
		// earlier tick lands here on the re-offer. Saying "you're already on
		// expedition" would be a rejection for the thing the order in fact did, so
		// the settled debit is what tells the two apart.
		if idemKey != "" && p.euro != nil && p.euro.HasExternalTx(idemKey) && existing.ZoneID == zone.ID {
			z, _ := getZone(existing.ZoneID)
			return expStartOutcome{Zone: z, Supplies: existing.Supplies,
				Cost: purchase.Cost(),
				Days: estimateDays(existing.Supplies.Max, existing.Supplies.DailyBurn)}, nil
		}
		z, _ := getZone(existing.ZoneID)
		return expStartOutcome{}, refuseAdv(errExpStartBusy,
			"You're already on expedition in **%s** (Day %d). Finish it or `!expedition abandon` first.",
			z.Display, existing.CurrentDay)
	}
	// A leader who extracted still holds their roster for the resume window, and
	// `!resume` only ever reaches the *newest* extracted row. Starting fresh on
	// top of one would orphan it: unreachable, un-reapable until the sweeper
	// catches it, with every member still seated and refused a run of their own.
	//
	// Only a row with a roster blocks. A solo extraction strands nobody, so
	// walking away from it stays a normal thing to do.
	if pending, _ := getResumableExpedition(uid); pending != nil {
		switch {
		case extractionLapsed(pending, time.Now().UTC()):
			// Past the window — reap it here rather than make them wait an hour
			// for the sweeper, and let the new expedition proceed. Route through
			// the shared reap so the freed members hear about it, same as the
			// sweeper and `!expedition abandon` do.
			if err := p.reapLapsedExtraction(pending); err != nil {
				slog.Warn("expedition: reap lapsed on start", "expedition", pending.ID, "err", err)
			}
		default:
			// A roster still holds; block. On a roster-read error, assume it is
			// occupied and refuse — proceeding would orphan a party we could not
			// confirm was empty, the one outcome this guard exists to prevent. A
			// solo extraction (n == 1) strands nobody, so walking away is fine.
			n, err := partySize(pending.ID)
			if err != nil || n > 1 {
				z, _ := getZone(pending.ZoneID)
				return expStartOutcome{}, refuseAdv(errExpStartBusy,
					"You extracted from **%s** on Day %d and your party is still waiting on you. `!resume` to lead them back in, or `!expedition abandon` to let it go — until you do one or the other, none of them can start a run of their own.",
					z.Display, pending.CurrentDay)
			}
		}
	}

	cost := float64(purchase.Cost())
	if p.euro == nil {
		return expStartOutcome{}, refuseAdv(errExpStartFailed, "Coin system unavailable — try again later.")
	}
	// Skip the affordability gate on a re-offer that already paid: the debit is a
	// settled fact and re-reading the now-lower balance would bounce a departure
	// the player has bought. Same reasoning as purchaseEquipmentTier's.
	if !(idemKey != "" && p.euro.HasExternalTx(idemKey)) {
		if balance := p.euro.GetBalance(uid); balance < cost {
			return expStartOutcome{}, refuseAdv(errExpStartBroke,
				"Not enough coins. Outfitting costs **%d** but you have **%.0f**.",
				int(cost), balance)
		}
	}
	if existing, _ := getActiveZoneRun(uid); existing != nil {
		z, _ := getZone(existing.ZoneID)
		return expStartOutcome{}, refuseAdv(errExpStartBusy,
			"You have an active single-session zone run in **%s**. Finish or `!zone abandon` before starting an expedition.",
			z.Display)
	}

	_, supplies, startLine, err := p.beginExpeditionIdem(uid, c.Level, zone, purchase, "expedition outfitting", idemKey)
	if err != nil {
		// beginExpedition refunds and tears down on every failure path, so the
		// player owes nothing and this is permanent rather than retryable — a retry
		// after a refund would find the guid-keyed debit already settled and hand
		// them the expedition for free.
		return expStartOutcome{}, refuseAdv(errExpStartFailed, "%s", err.Error())
	}
	markActedToday(uid)
	return expStartOutcome{
		Zone: zone, Supplies: supplies, Cost: purchase.Cost(),
		Days: estimateDays(supplies.Max, supplies.DailyBurn), StartLine: startLine,
	}, nil
}

// beginExpedition performs the non-interactive half of starting an expedition:
// supply freebies, the coin debit, persistence, the starting region's run, and
// the opening log entry. It refunds and tears down on every failure path, so a
// caller holding an error owes the player nothing.
//
// Callers own the balance check, the eligibility guards, and the player-facing
// messaging. This is the shared transaction under `!expedition start` and the
// boredom ticker (gogobee_boredom_plan.md §4); the returned errors are written
// as complete player-facing sentences so both callers can surface them as-is.
//
// It deliberately does NOT call markActedToday — an expedition the player did
// not ask for must not spend their daily action or count as them showing up.
func (p *AdventurePlugin) beginExpedition(uid id.UserID, charLevel int, zone ZoneDefinition, purchase SupplyPurchase, reason string) (*Expedition, ExpeditionSupplies, string, error) {
	return p.beginExpeditionIdem(uid, charLevel, zone, purchase, reason, "")
}

// beginExpeditionIdem is beginExpedition with the money keyed to an idempotency
// id. idemKey empty keeps the plain Debit/Credit pair, which is correct for the
// two callers that arrive exactly once (a Matrix command, the boredom ticker).
//
// A non-empty key comes from the web action queue, whose wire retries: the debit
// then lands at most once however many times the order is re-offered, and the
// refunds are keyed too so a torn-down start cannot refund on every tick. Note
// what this means for the caller — once a refund has happened, retrying is
// *unsafe*, because the guid-keyed debit will not charge again and the player
// would get the expedition for free. performExpeditionStart therefore treats
// every error from here as permanent.
func (p *AdventurePlugin) beginExpeditionIdem(uid id.UserID, charLevel int, zone ZoneDefinition, purchase SupplyPurchase, reason, idemKey string) (*Expedition, ExpeditionSupplies, string, error) {
	if p.euro == nil {
		return nil, ExpeditionSupplies{}, "", fmt.Errorf("Coin system unavailable — try again later.")
	}
	cost := float64(purchase.Cost())
	debit := func(why string) bool { return p.euro.Debit(uid, cost, why) }
	refund := func(why, suffix string) { p.euro.Credit(uid, cost, why) }
	if idemKey != "" {
		debit = func(why string) bool {
			ok, _, err := p.euro.DebitIdem(uid, cost, why, idemKey)
			return err == nil && ok
		}
		refund = func(why, suffix string) {
			if _, _, err := p.euro.CreditIdem(uid, cost, why, idemKey+":refund"+suffix); err != nil {
				slog.Error("expedition: outfitting refund failed", "user", uid, "order", idemKey, "err", err)
			}
		}
	}

	// Holiday perk: a complimentary standard pack is added to the supplies
	// snapshot without inflating the coin cost. Bypasses the per-tier cap
	// on purpose — it's a freebie on top of whatever the player bought.
	suppliesPurchase := purchase
	if isHol, _ := isHolidayToday(); isHol {
		suppliesPurchase.StandardPacks++
	}
	if pk := activeOmen().SupplyFreebiePacks; pk > 0 { // N7/B3 the Omen
		suppliesPurchase.StandardPacks += pk
	}
	supplies := makeSupplies(zone.Tier, suppliesPurchase)

	// Debit coins; bail on debit failure (race / cap).
	if !debit(reason + ": " + string(zone.ID)) {
		return nil, ExpeditionSupplies{}, "", fmt.Errorf("Couldn't debit outfitting cost (try again).")
	}

	exp, err := startExpedition(uid, zone.ID, "", supplies)
	if err != nil {
		// Refund on persistence failure.
		refund("expedition outfitting refund", "")
		return nil, ExpeditionSupplies{}, "", fmt.Errorf("Couldn't start expedition: %s", err)
	}

	// R2 — auto-spawn the DungeonRun for the starting region. Per-room
	// harvest depends on this; the existing zone-run !advance/!retreat
	// commands now operate on this run while the expedition is active.
	if _, err := ensureRegionRun(exp, charLevel); err != nil {
		// Refund and tear the expedition row back down — without a
		// linked run, harvest and rooms can't function.
		_ = abandonExpedition(uid)
		refund("expedition outfitting refund (run-spawn failed)", ":region")
		return nil, ExpeditionSupplies{}, "", fmt.Errorf("Couldn't outfit the first region: %s", err)
	}

	// Log the start with prewritten flavor.
	startLine := flavor.Pick(flavor.ExpeditionStart)
	_ = appendExpeditionLog(exp.ID, 1, "narrative", "expedition started", startLine)
	return exp, supplies, startLine, nil
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
	exp, _, err := activeExpeditionFor(ctx.Sender)
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

// Sentinels for the two ways abandoning can be refused, so the web action queue
// can pick a verdict without reading prose. Same contract as the start/resume
// family above: errors.Is classifies, Error() is the sentence Matrix has always
// sent.
var (
	errAbandonNothing   = errors.New("expedition abandon: nothing to abandon")
	errAbandonNotLeader = errors.New("expedition abandon: only the leader may call it")
)

// abandonOutcome is what closing the expedition did, for a caller describing it
// somewhere other than a DM.
type abandonOutcome struct {
	Zone      ZoneDefinition
	Day       int
	Extracted bool // it was already out and standing in town: loot and XP are kept
}

// performExpeditionAbandon is `!expedition abandon` minus the command framing.
// Shared with the web action queue so closing a run from a phone disbands the
// same roster, retires the same region runs, writes the same log line and tells
// the same party — the members hear it from their leader either way, because
// that is a fact about the expedition and not about which door was used.
//
// Like performExpeditionStart this does NOT take the per-user lock: its Matrix
// caller already holds it across the whole `!expedition` switch. applyWebAbandon
// takes it instead. Getting that backwards does not fail loudly — it wedges the
// player's lock forever and every later adventure command from them hangs.
func (p *AdventurePlugin) performExpeditionAbandon(uid id.UserID) (abandonOutcome, error) {
	exp, isLeader, err := activeExpeditionFor(uid)
	if err != nil {
		return abandonOutcome{}, err
	}
	if exp == nil {
		// An extracted expedition is still the owner's to close — it holds the
		// roster until the resume window lapses. Without this, a leader who
		// wanted out had to pay to `!resume` first just to abandon.
		if exp, err = getResumableExpedition(uid); err != nil {
			return abandonOutcome{}, err
		}
		isLeader = exp != nil
	}
	if exp == nil {
		return abandonOutcome{}, refuseAdv(errAbandonNothing, "No active expedition to abandon.")
	}
	if !isLeader {
		// Abandoning throws away everyone's day. A member leaves alone.
		return abandonOutcome{}, refuseAdv(errAbandonNotLeader,
			"Only your party leader can abandon the expedition. `!expedition leave` to walk out alone.")
	}
	zone, _ := getZone(exp.ZoneID)
	out := abandonOutcome{Zone: zone, Day: exp.CurrentDay, Extracted: exp.Status == ExpeditionStatusExtracting}
	audience := expeditionAudience(exp) // read before abandonExpedition disbands the roster
	if err := abandonExpedition(uid); err != nil {
		return abandonOutcome{}, err
	}
	markActedToday(uid)
	_ = retireAllRegionRuns(exp)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative", "expedition abandoned", "")
	// The roster is being disbanded out from under the members; they hear it from
	// their leader rather than discovering it the next time a command works again.
	for _, member := range audience {
		if member == uid {
			continue
		}
		if err := p.SendDM(member, fmt.Sprintf(
			"Your leader called off the expedition in **%s** on Day %d. You're free to start a run of your own.",
			zone.Display, exp.CurrentDay)); err != nil {
			slog.Warn("expedition: abandon DM failed", "user", member, "expedition", exp.ID, "err", err)
		}
	}
	// Emergence seam: see maybeRollPetArrivalOnEmerge. Inside the twin because
	// walking out of a dungeon is what rolls it, not saying so in a room.
	p.maybeRollPetArrivalOnEmerge(uid)
	return out, nil
}

func (p *AdventurePlugin) expeditionCmdAbandon(ctx MessageContext) error {
	out, err := p.performExpeditionAbandon(ctx.Sender)
	if err != nil {
		var refusal advRefusal
		if errors.As(err, &refusal) {
			return p.SendDM(ctx.Sender, refusal.Error())
		}
		return p.SendDM(ctx.Sender, "Couldn't abandon: "+err.Error())
	}
	// An extracted party is standing in town, not in the dungeon: their supplies
	// are already spent and their loot is already banked. Say the true thing.
	body := fmt.Sprintf(
		"Expedition in **%s** abandoned on Day %d. Supplies are forfeit. The dungeon remembers.",
		out.Zone.Display, out.Day)
	if out.Extracted {
		body = fmt.Sprintf(
			"You let the expedition in **%s** go. Day %d is where it ends — loot, XP, and coins are kept. The dungeon remembers.",
			out.Zone.Display, out.Day)
	}
	return p.SendDM(ctx.Sender, body)
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
	// runAutopilotWalk resolves through getActiveExpedition, so a member would
	// be told they are not on an expedition at all. Same refusal `!zone advance`
	// gives — this is the same walk, reached by its other name.
	if isPartyMember(ctx.Sender) {
		return p.SendDM(ctx.Sender,
			"Your party leader sets the pace. `!map` to see where you're standing.")
	}
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
// forkAutoPickTimeout — how long a background fork may sit unanswered before
// the autopilot picks a route itself.
//
// This was 8h, which reads as "the player gets first say" and behaves as "the
// expedition stops for a third of a day, every fork." A multi-day expedition
// crosses a lot of forks; at 8h apiece the autopilot spends more of its life
// parked than walking, and a player who is simply asleep loses a night per
// branch. 30m keeps a genuine first say for anyone actually at the keyboard and
// costs an absent player almost nothing.
const forkAutoPickTimeout = 30 * time.Minute

// rankForkOptions orders a fork's options by how much walking them is worth:
// somewhere new first, then the fatter edge (Weight is the author's own "this
// is the main line" signal), then menu order as the tiebreak so the pick is
// deterministic. Only unlocked options are returned.
//
// The old rule was "first unlocked option in the menu", which is edge-authoring
// order — meaningful to whoever wrote the graph, arbitrary to the player. It
// walked past unvisited branches to loop through cleared ones often enough to
// look broken.
func rankForkOptions(g ZoneGraph, run *DungeonRun, pf *pendingFork) []pendingChoice {
	weights := map[string]int{}
	for _, e := range g.outgoingEdges(run.CurrentNode) {
		weights[e.To] = e.Weight
	}
	visited := map[string]bool{}
	for _, n := range run.VisitedNodes {
		visited[n] = true
	}

	open := make([]pendingChoice, 0, len(pf.Options))
	for _, o := range pf.Options {
		if o.Unlocked {
			open = append(open, o)
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		vi, vj := visited[open[i].To], visited[open[j].To]
		if vi != vj {
			return !vi // unvisited first
		}
		if wi, wj := weights[open[i].To], weights[open[j].To]; wi != wj {
			return wi > wj
		}
		return open[i].Index < open[j].Index
	})
	return open
}

// autoPickStaleFork commits a stale background fork, advancing the run exactly
// as `!zone go <n>` would (advanceZoneRunNode + region-transition hook). The
// choice is logged as a narrative entry so the end-of-day digest can surface
// the decision the player missed.
//
// When every route is locked it does not give up: it spends a set of thieves'
// tools if the party is carrying any and one of the locks is the pickable kind.
// Returns false only when there is genuinely nothing it can do — the caller
// then backtracks rather than idling the expedition into the 24h reaper.
func (p *AdventurePlugin) autoPickStaleFork(exp *Expedition, run *DungeonRun, pf *pendingFork) bool {
	g, _ := loadZoneGraph(run.ZoneID)

	ranked := rankForkOptions(g, run, pf)
	note := "autopilot took the most promising path"
	var spendTool int64
	if len(ranked) == 0 {
		picked, toolID, ok := p.autoPickWithTools(run, pf)
		if !ok {
			return false
		}
		ranked = []pendingChoice{picked}
		spendTool = toolID
		note = "autopilot spent " + thievesToolsName + " on the only way forward"
	}
	chosen := ranked[0]

	if _, err := advanceZoneRunNode(run.RunID, chosen.To); err != nil {
		slog.Warn("expedition: auto-pick stale fork",
			"user", run.UserID, "run", run.RunID, "err", err)
		return false
	}
	// The door is behind us, so now the set is actually used up. Billing before
	// the advance would charge the player for a move that failed — and the
	// caller's backtrack would then clear the fork the tools just paid for.
	if spendTool != 0 {
		if err := removeAdvInventoryItem(spendTool); err != nil {
			slog.Warn("expedition: autopilot tools spend", "user", run.UserID, "err", err)
		}
		beatLock(run, chosen.Label, "picked")
	}
	fireGraphRegionTransition(run.UserID, g.Nodes[run.CurrentNode], g.Nodes[chosen.To])
	if exp != nil {
		_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
			fmt.Sprintf("%s: %s", note, chosen.Label), "")
	}
	return true
}

// autoPickWithTools finds a route a set of thieves' tools could open when every
// option on the fork is locked, so a bad Perception roll can't quietly end an
// expedition the player paid days into. It only ever fires when there is no free
// route left — the player's tools are their own, and the autopilot does not get
// to burn them for convenience.
//
// It reports the route and the inventory row to spend, but does not spend it:
// the caller charges the player only once the move has actually committed.
func (p *AdventurePlugin) autoPickWithTools(run *DungeonRun, pf *pendingFork) (pendingChoice, int64, bool) {
	toolID, ok := findThievesTools(id.UserID(run.UserID))
	if !ok {
		return pendingChoice{}, 0, false
	}
	for i := range pf.Options {
		if pf.Options[i].Unlocked || !pickableLock(pf.Options[i].Lock) {
			continue
		}
		// Local copy only — advanceZoneRunNode clears node_choices on success,
		// and on failure the fork must stay as locked as the player left it
		// rather than reading "open" for a set nobody paid for.
		chosen := pf.Options[i]
		chosen.Unlocked = true
		chosen.Reason = "opened with " + thievesToolsName
		return chosen, toolID, true
	}
	return pendingChoice{}, 0, false
}

// backtrackFromDeadFork walks the run back one room when a fork has no route
// the autopilot can take and no tools to buy one with. Without this the run
// simply sits there until the 24h stale reaper ends the expedition — a player
// losing days of progress to a die roll they never saw and could not answer.
// Backtracking at least returns them to a room with other exits.
//
// Returns false at the entry node, where there is nowhere behind to go, and
// wherever no fallback room would actually help — see backtrackTarget.
func (p *AdventurePlugin) backtrackFromDeadFork(exp *Expedition, run *DungeonRun) bool {
	g, ok := loadZoneGraph(run.ZoneID)
	if !ok {
		return false
	}
	target, ok := backtrackTarget(g, run)
	if !ok {
		return false
	}

	// Clear the fork first: it belongs to the node being left, and both
	// `!zone advance` and `!zone go` would otherwise resolve a prompt pointing
	// at a room the party is no longer standing in.
	if err := clearPendingFork(run.RunID); err != nil {
		slog.Warn("expedition: backtrack clear fork", "run", run.RunID, "err", err)
		return false
	}
	beatLock(run, "", "sealed")
	if _, err := revisitZoneRun(run.RunID, target, run.VisitedNodes); err != nil {
		slog.Warn("expedition: backtrack from dead fork", "run", run.RunID, "err", err)
		return false
	}
	if exp != nil {
		_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
			"every way on was sealed — the party doubled back", "")
	}
	return true
}

// backtrackTarget picks the room a dead fork falls back to: the most recently
// entered visited node that is genuinely joined to the current one by an edge
// and that still offers a way on other than the sealed room.
//
// Both halves are load-bearing. VisitedNodes is a first-entry ordered *set*, not
// a path stack (see appendVisited) — after any earlier backtrack the entry
// before CurrentNode can sit on a completely different branch, so stepping to it
// blind teleports the party across the map. `!revisit` refuses exactly that move
// via adjacentNodes, and the autopilot has no business doing what the player is
// forbidden from doing. The second half stops the other failure: falling back
// into a corridor whose only exit is the fork we just fled from just walks
// straight back in — and the lock rolls are seeded per (run, edge), so the
// result is identical every time. That is an infinite loop, not a recovery.
func backtrackTarget(g ZoneGraph, run *DungeonRun) (string, bool) {
	adj := adjacentNodes(g, run.CurrentNode)
	for i := pathIndexOf(run.VisitedNodes, run.CurrentNode) - 1; i >= 0; i-- {
		n := run.VisitedNodes[i]
		if !adj[n] {
			continue
		}
		for _, e := range g.outgoingEdges(n) {
			if e.To != run.CurrentNode {
				return n, true
			}
		}
	}
	return "", false
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
			stale := compact && time.Since(run.LastActionAt) > forkAutoPickTimeout
			picked := stale && p.autoPickStaleFork(exp, run, pf)
			// Stale and nothing takeable: every route locked, no tools. Back out
			// one room rather than sitting here until the 24h reaper ends an
			// expedition the player may be days into. The backtrack clears the
			// fork, so the next tick walks from the previous room normally.
			if stale && !picked && p.backtrackFromDeadFork(exp, run) {
				return autopilotWalkResult{
					finalMsg: "🔒 Every way on was sealed. The party doubled back to look for another line.",
					rooms:    0,
					reason:   stopFork,
				}
			}
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
