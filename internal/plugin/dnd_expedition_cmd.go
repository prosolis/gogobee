package plugin

import (
	"fmt"
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
		// If active, show status; otherwise help.
		if exp, _ := getActiveExpedition(ctx.Sender); exp != nil {
			return p.expeditionCmdStatus(ctx)
		}
		return p.SendDM(ctx.Sender, expeditionHelpText())
	case "help", "?":
		return p.SendDM(ctx.Sender, expeditionHelpText())
	case "list", "ls":
		return p.zoneCmdList(ctx, c)
	case "start", "begin", "go":
		return p.expeditionCmdStart(ctx, c, rest)
	case "status", "info":
		return p.expeditionCmdStatus(ctx)
	case "log", "history":
		return p.expeditionCmdLog(ctx)
	case "abandon", "quit":
		return p.expeditionCmdAbandon(ctx)
	default:
		return p.SendDM(ctx.Sender, expeditionHelpText())
	}
}

func expeditionHelpText() string {
	var b strings.Builder
	b.WriteString("**!expedition** — multi-day dungeon expeditions.\n\n")
	b.WriteString("`!expedition list` — show zones available at your level\n")
	b.WriteString("`!expedition start <zone> [Ns] [Md]` — outfit & begin\n")
	b.WriteString("    `Ns` = N standard packs (10 SU, 50 coins, max 3)\n")
	b.WriteString("    `Md` = M deluxe packs (20 SU, 90 coins, max 1)\n")
	b.WriteString("    default: `1s`\n")
	b.WriteString("`!expedition status` — current expedition snapshot\n")
	b.WriteString("`!expedition log` — last 5 log entries\n")
	b.WriteString("`!expedition abandon` — end the expedition (no rewards)")
	return b.String()
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

func (p *AdventurePlugin) expeditionCmdStart(ctx MessageContext, c *DnDCharacter, rest string) error {
	if rest == "" {
		return p.SendDM(ctx.Sender,
			"`!expedition start <zone> [Ns] [Md]` — pick from `!expedition list`. Example: `!expedition start goblin_warrens 2s` (2 standard packs).")
	}
	zoneTok, packTok := splitFirstWord(rest)
	available := zonesForLevel(c.Level)
	zoneID, ok := resolveZoneInput(zoneTok, available)
	if !ok {
		return p.SendDM(ctx.Sender,
			"Unknown zone for your level. Try `!expedition list`.")
	}
	purchase, err := parseSupplyArgs(packTok)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't parse supply packs: "+err.Error())
	}
	if err := purchase.Validate(); err != nil {
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

	zone, _ := getZone(zoneID)
	supplies := makeSupplies(zone.Tier, purchase)

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

	// Log the start with prewritten flavor.
	startLine := flavor.Pick(flavor.ExpeditionStart)
	_ = appendExpeditionLog(exp.ID, 1, "narrative", "expedition started", startLine)

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
	b.WriteString("Use `!expedition status` for the daily briefing format. Day 1 begins now.")
	return p.SendDM(ctx.Sender, b.String())
}

func estimateDays(maxSU, dailyBurn float32) int {
	if dailyBurn <= 0 {
		return 0
	}
	return int(math.Floor(float64(maxSU / dailyBurn)))
}

// ── status ──────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) expeditionCmdStatus(ctx MessageContext) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender,
			"No active expedition. Use `!expedition list` then `!expedition start <zone>`.")
	}
	zone, _ := getZone(exp.ZoneID)
	c, _ := LoadDnDCharacter(ctx.Sender)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎭 **TwinBee — Status, Day %d**\n\n", exp.CurrentDay))
	b.WriteString(fmt.Sprintf("📍 **Zone:** %s _(T%d)_\n", zone.Display, int(zone.Tier)))
	if c != nil {
		b.WriteString(fmt.Sprintf("❤️  **HP:** %d / %d\n", c.HPCurrent, c.HPMax))
	}
	b.WriteString(fmt.Sprintf("🎒 **Supplies:** %.1f / %.1f SU  _(burn %.1f/day → ~%d days left)_\n",
		exp.Supplies.Current, exp.Supplies.Max, currentBurn(exp),
		estimateDays(exp.Supplies.Current, currentBurn(exp))))
	b.WriteString(fmt.Sprintf("⏰ **Threat:** %d / 100 — %s\n",
		exp.ThreatLevel, threatThresholdLabel(exp.ThreatLevel, exp.SiegeMode)))
	if exp.TemporalStack != 0 {
		b.WriteString(fmt.Sprintf("🌡 **Zone stack:** %d\n", exp.TemporalStack))
	}
	if exp.Camp != nil && exp.Camp.Active {
		b.WriteString(fmt.Sprintf("⛺ **Camp:** %s (room %d)\n", exp.Camp.Type, exp.Camp.RoomIndex+1))
	}
	state := supplyDepletion(exp.Supplies)
	if state != SupplyNormal {
		b.WriteString(fmt.Sprintf("⚠ **%s** — roll modifier %d\n",
			depletionLabel(state), supplyRollModifier(state)))
	}
	b.WriteString(fmt.Sprintf("\nStarted: %s   Last activity: %s",
		exp.StartDate.Format("2006-01-02 15:04"),
		exp.LastActivity.Format("2006-01-02 15:04")))
	return p.SendDM(ctx.Sender, b.String())
}

func currentBurn(exp *Expedition) float32 {
	burn := exp.Supplies.DailyBurn
	if exp.ThreatLevel > 60 || exp.SiegeMode {
		// Harsh conditions: §4.1 / §8.3.
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
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative", "expedition abandoned", "")
	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"Expedition in **%s** abandoned on Day %d. Supplies are forfeit. The dungeon remembers.",
		zone.Display, exp.CurrentDay))
}

// helper: ensure we don't shadow id.UserID import in test harness.
var _ id.UserID
