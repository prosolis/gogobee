package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"gogobee/internal/flavor"

	"maunium.net/go/mautrix/id"
)

// Phase 12 E4c — !region command surface and inter-region travel.
// Spec: gogobee_expedition_system.md §11.3.
//
// Subcommands:
//
//	!region                       → list regions w/ status; current first
//	!region list                  → same as bare
//	!region travel [<id>|next]    → travel to the next region (linear)
//
// Travel rules (§11.3):
//   - 1 in-game day burned (current_day += 1, daily-burn applied)
//   - 1 wandering monster check fires during transit (no camp protection)
//   - Region-transition narration logged to the expedition log
//
// Travel is linear (Order N → Order N+1) in E4c — branching transitions
// are out of scope for the design doc as written. Players cannot travel
// past the last region (the zone-boss region).

func (p *AdventurePlugin) handleRegionCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender,
			"No active expedition. `!region` only works during an expedition.")
	}
	if !IsMultiRegionZone(exp.ZoneID) {
		return p.SendDM(ctx.Sender,
			"This zone has no regions — it's a single sub-dungeon. `!region` only applies to Tier 4–5 multi-region zones.")
	}

	args = strings.TrimSpace(strings.ToLower(args))
	sub, _ := splitFirstWord(args)
	switch sub {
	case "", "list", "ls", "status":
		return p.SendDM(ctx.Sender, renderRegionList(exp))
	case "travel", "advance", "go", "next":
		return p.regionCmdTravel(ctx, exp)
	case "help", "?":
		return p.SendDM(ctx.Sender, regionHelpText())
	default:
		return p.SendDM(ctx.Sender, regionHelpText())
	}
}

func regionHelpText() string {
	var b strings.Builder
	b.WriteString("**!region** — multi-region zone navigation.\n\n")
	b.WriteString("`!region` — list regions and progress\n")
	b.WriteString("`!region travel` — move to the next region (1 in-game day, supply burn, transit wandering check)\n")
	return b.String()
}

func renderRegionList(exp *Expedition) string {
	rs := regionsForZone(exp.ZoneID)
	zone, _ := getZone(exp.ZoneID)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗺 **%s — Regions**\n\n", zone.Display))
	for _, r := range rs {
		marker := "  "
		switch {
		case r.ID == exp.CurrentRegion:
			marker = "▶ "
		case IsRegionCleared(exp, r.ID):
			marker = "✓ "
		case IsRegionVisited(exp, r.ID):
			marker = "· "
		}
		bossNote := r.RegionBoss
		if r.IsZoneBoss {
			bossNote += " ★"
		}
		baseTag := ""
		if r.BaseCampSite {
			if HasBaseCampAt(exp, r.ID) {
				baseTag = "  ⛺"
			} else if IsRegionCleared(exp, r.ID) {
				baseTag = "  ⛺ (eligible)"
			}
		}
		b.WriteString(fmt.Sprintf("%s**%d. %s** — _%s_%s\n",
			marker, r.Order, r.Name, bossNote, baseTag))
	}
	b.WriteString("\n_▶ current  ✓ cleared  · visited  ★ zone boss  ⛺ base camp_\n")
	if next, ok := nextRegion(exp.ZoneID, exp.CurrentRegion); ok {
		b.WriteString(fmt.Sprintf("\nNext: **%s** — `!region travel`", next.Name))
	} else {
		b.WriteString("\n_End of the line — the zone boss waits in this region._")
	}
	return b.String()
}

func (p *AdventurePlugin) regionCmdTravel(ctx MessageContext, exp *Expedition) error {
	if exp.Camp != nil && exp.Camp.Active {
		return p.SendDM(ctx.Sender,
			"You can't travel between regions while camped. `!camp break` first.")
	}
	cur, ok := CurrentRegion(exp)
	if !ok {
		return p.SendDM(ctx.Sender, "Current region is not set — this expedition's state is unusual; try `!region` to refresh.")
	}
	next, ok := nextRegion(exp.ZoneID, cur.ID)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You're already in **%s** — the final region of this zone. Defeat the zone boss to complete the expedition.",
			cur.Name))
	}
	narrative, err := p.advanceToNextRegion(ctx.Sender, exp, cur, next)
	if err != nil {
		return p.SendDM(ctx.Sender, "Region transit failed: "+err.Error())
	}
	return p.SendDM(ctx.Sender, narrative)
}

// advanceToNextRegion performs an inter-region transition: burns the transit
// day + supplies, fires the transit wandering check, retires the outgoing
// region's DungeonRun, bumps CurrentRegion + the visited list, and spawns the
// incoming region's run (pinning exp.RunID). Returns the rendered transit
// narrative block. Shared by the manual `!region travel` command and the
// autopilot walk's mid-zone auto-advance — keeping the transit cost identical
// on both paths.
func (p *AdventurePlugin) advanceToNextRegion(userID id.UserID, exp *Expedition, cur, next ExpeditionRegion) (string, error) {
	// Burn one day of supplies (transit day).
	siege := exp.SiegeMode
	harsh := exp.ThreatLevel > 60 || zoneTemporalHarsh(exp)
	newSupplies, burned := applyExpeditionDailyBurn(exp, harsh, siege)
	exp.Supplies = newSupplies
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		return "", fmt.Errorf("apply transit supply burn: %w", err)
	}
	if err := advanceExpeditionDay(exp.ID); err != nil {
		return "", fmt.Errorf("advance expedition day: %w", err)
	}
	exp.CurrentDay++

	// Transit wandering check (no camp protection — campMod = 0).
	c, _ := LoadDnDCharacter(userID)
	var charClass DnDClass
	charLevel := 1
	if c != nil {
		charClass = c.Class
		charLevel = c.Level
	}
	tc := resolveTransitWanderingCheck(exp, charClass, nil)
	_ = processTransitWanderingCheck(exp, tc)

	// R2 — retire the outgoing region's DungeonRun before mutating
	// CurrentRegion so retireRegionRun keys the right region.
	if err := retireRegionRun(exp, cur.ID); err != nil {
		return "", fmt.Errorf("retire previous region run: %w", err)
	}

	// Update CurrentRegion + visited list.
	if err := setCurrentRegion(exp, next.ID); err != nil {
		return "", fmt.Errorf("update current region: %w", err)
	}
	if _, err := MarkRegionVisited(exp, next.ID); err != nil {
		return "", fmt.Errorf("mark region visited: %w", err)
	}

	// Spawn the new region's DungeonRun and pin exp.RunID.
	if _, err := ensureRegionRun(exp, charLevel); err != nil {
		return "", fmt.Errorf("outfit the new region: %w", err)
	}

	// Log + flavor.
	departure := strings.ReplaceAll(flavor.Pick(flavor.RegionTransitDeparture),
		"[REGION_NEXT]", next.Name)
	arrival := strings.ReplaceAll(flavor.Pick(flavor.RegionTransitArrival),
		"[REGION_NEXT]", next.Name)
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		fmt.Sprintf("region transit: %s → %s (Day %d, %.1f SU burned)",
			cur.Name, next.Name, exp.CurrentDay, burned),
		departure)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗺 **Region transit — %s → %s**\n\n", cur.Name, next.Name))
	b.WriteString(fmt.Sprintf("**Day:** %d → %d   **Supplies:** −%.1f SU (now %.1f / %.1f)\n",
		exp.CurrentDay-1, exp.CurrentDay, burned, exp.Supplies.Current, exp.Supplies.Max))
	if departure != "" {
		b.WriteString("\n" + departure + "\n")
	}
	b.WriteString("\n" + renderTransitWanderingCheck(tc) + "\n")
	if arrival != "" {
		b.WriteString("\n" + arrival + "\n")
	}
	if next.IsZoneBoss {
		b.WriteString("\n_★ Final region — the zone boss is here._")
	}
	return b.String(), nil
}

// resolveTransitWanderingCheck mirrors resolveWanderingCheck (§6.1) but
// operates without an active camp (campMod = 0). The check fires once
// per region transition per §11.3.
func resolveTransitWanderingCheck(e *Expedition, charClass DnDClass, roll1d20 func() int) NightCheck {
	if roll1d20 == nil {
		roll1d20 = func() int { return rand.IntN(20) + 1 }
	}
	r := roll1d20()
	threatMod := 0
	if e.ThreatLevel > 30 {
		threatMod = (e.ThreatLevel - 30) / 10
	}
	classMod := 0
	if charClass == ClassRanger && wildernessZones[e.ZoneID] {
		classMod = -2
	}
	total := r + threatMod + classMod
	out := wanderingOutcome(total)

	nc := NightCheck{
		Outcome:   out,
		Roll:      r,
		Total:     total,
		ThreatMod: threatMod,
		CampMod:   0,
		ClassMod:  classMod,
	}
	switch out {
	case NightOutcomePeaceful:
		nc.Summary = "Quiet transit."
	case NightOutcomePassage:
		nc.Summary = "Signs of pursuit along the route; no encounter."
		nc.ThreatBumped = true
	case NightOutcomeMinor, NightOutcomeStandard, NightOutcomeElite, NightOutcomeAmbush:
		mid, mname := pickWanderingMonster(e.ZoneID, out)
		nc.MonsterID = mid
		nc.MonsterName = mname
		nc.Summary = fmt.Sprintf("%s during transit — %s.", outcomeLabel(out), describeMonsterCount(out, mname))
	}
	nc.Flavor = wanderingFlavor(out, e.ThreatLevel, e.SiegeMode)
	return nc
}

// processTransitWanderingCheck applies side effects of a transit
// encounter: log entry + threat bump on signs-of-pursuit. Combat
// resolution itself defers to the combat-link phase, identical to the
// night-phase pattern (§6).
func processTransitWanderingCheck(e *Expedition, nc NightCheck) error {
	summary := nc.Summary
	if nc.MonsterName != "" {
		summary = fmt.Sprintf("%s [transit d20=%d, total=%d, mods: threat%+d class%+d]",
			summary, nc.Roll, nc.Total, nc.ThreatMod, nc.ClassMod)
	} else {
		summary = fmt.Sprintf("%s [transit d20=%d, total=%d]", summary, nc.Roll, nc.Total)
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "transit", summary, nc.Flavor); err != nil {
		return err
	}
	if nc.ThreatBumped {
		_ = applyThreatDelta(e.ID, 2, "signs of pursuit during transit")
	}
	return nil
}

func renderTransitWanderingCheck(nc NightCheck) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🐾 **Transit check — %s**\n", outcomeLabel(nc.Outcome)))
	if nc.Summary != "" {
		b.WriteString(nc.Summary + "\n")
	}
	b.WriteString(fmt.Sprintf("_d20=%d, total=%d (threat%+d, class%+d)_",
		nc.Roll, nc.Total, nc.ThreatMod, nc.ClassMod))
	if nc.Outcome == NightOutcomeMinor || nc.Outcome == NightOutcomeStandard ||
		nc.Outcome == NightOutcomeElite || nc.Outcome == NightOutcomeAmbush {
		b.WriteString("\n_Encounter pending — resolves at your next `!advance`._")
	}
	return b.String()
}
