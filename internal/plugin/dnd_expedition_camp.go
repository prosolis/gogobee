package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E1e — basic camp system (rough + standard).
// Spec: gogobee_expedition_system.md §5. Fortified and Base camps are
// deliverables of E2 (Threat Clock & Night Phase) and E4 (Multi-Region)
// respectively — !camp explicitly rejects those types here so the
// surface is forward-compatible.

const (
	CampTypeRough     = "rough"
	CampTypeStandard  = "standard"
	CampTypeFortified = "fortified"
	CampTypeBase      = "base"
)

// campSupplyCost — extra SU consumed at pitch time (§5.1).
var campSupplyCost = map[string]float32{
	CampTypeRough:     0.5,
	CampTypeStandard:  1.0,
	CampTypeFortified: 2.0,
	CampTypeBase:      3.0,
}

// !camp <type> — establish a camp on the current expedition.
//
// Validation (§5.2):
//   - cannot camp in an active-enemy room
//   - cannot camp in a trap room
//   - cannot camp in a boss room
//   - non-cleared room ⇒ forced rough camp regardless of intent
//
// Until !advance is wired into the expedition layer (a later phase),
// the player is treated as standing at the entry room (cleared by
// definition), so all rooms read as cleared and camp validation only
// rejects the never-applicable boss/trap/active cases.

func (p *AdventurePlugin) handleCampCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender,
			"No Adv 2.0 character yet — run `!setup` first.")
	}

	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender,
			"No active expedition. `!camp` only works during an expedition. Use `!expedition start` first.")
	}

	args = strings.TrimSpace(strings.ToLower(args))
	requested, rest := splitFirstWord(args)
	if requested == "" {
		return p.SendDM(ctx.Sender, campHelpText(exp))
	}
	if requested == "break" || requested == "down" || requested == "leave" {
		return p.campBreak(ctx, exp)
	}
	switch requested {
	case "rough", "standard":
		// allowed in E1e
	case "fortified":
		if !exp.BossDefeated {
			return p.SendDM(ctx.Sender,
				"Fortified camps require a defeated zone boss. Clear the zone first.")
		}
	case "base":
		// E4d: §11.1 — base camps unlock per region after the region
		// boss is defeated, and only at base-camp-eligible sites.
		if !IsMultiRegionZone(exp.ZoneID) {
			return p.SendDM(ctx.Sender,
				"Base camps are a multi-region waypoint feature — only available in Tier 4–5 zones (Underdark / Dragon's Lair / Abyss Portal).")
		}
		region, ok := CurrentRegion(exp)
		if !ok {
			return p.SendDM(ctx.Sender,
				"Couldn't resolve your current region — try `!region` first.")
		}
		if !region.BaseCampSite {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"**%s** isn't a base-camp-eligible site. Look for a region whose boss has fallen and the geography is defensible.",
				region.Name))
		}
		if !IsRegionCleared(exp, region.ID) {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"You can pitch a base camp in **%s** only after defeating its region boss (%s).",
				region.Name, region.RegionBoss))
		}
	default:
		return p.SendDM(ctx.Sender,
			"Unknown camp type. Try `rough` (any location) or `standard` (cleared rooms).")
	}
	_ = rest

	return p.campPitch(ctx, exp, requested)
}

func campHelpText(exp *Expedition) string {
	var b strings.Builder
	b.WriteString("**!camp <type>** — establish camp during an expedition.\n\n")
	b.WriteString("`!camp rough` — partial rest (any location, +0.5 SU, high night risk)\n")
	b.WriteString("`!camp standard` — full long rest (cleared room, +1 SU, medium risk)\n")
	b.WriteString("`!camp fortified` — long rest + bonus (zone boss defeated, +2 SU, low risk)\n")
	b.WriteString("`!camp base` — persistent waypoint (region-boss-cleared base-camp site, +3 SU, very low risk)\n")
	b.WriteString("`!camp break` — break camp\n\n")
	if exp.Camp != nil && exp.Camp.Active {
		b.WriteString(fmt.Sprintf("_Currently camped: **%s** (room %d)._",
			exp.Camp.Type, exp.Camp.RoomIndex+1))
	}
	return b.String()
}

func (p *AdventurePlugin) campPitch(ctx MessageContext, exp *Expedition, kind string) error {
	if exp.Camp != nil && exp.Camp.Active {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You're already camped (**%s**). `!camp break` first.", exp.Camp.Type))
	}

	// Room-context validation. Once !advance is wired into expeditions
	// this resolves to the actual current room from the linked DungeonRun.
	// For now, the entry room (always cleared) is the assumed location.
	cleared, problem := campLocationCheck(exp)
	if problem != "" {
		return p.SendDM(ctx.Sender, problem)
	}
	if !cleared && kind == CampTypeStandard {
		// §5.2: standard camp requires a cleared room. Reject explicitly
		// rather than silently downgrading — the player should make the call.
		return p.SendDM(ctx.Sender,
			"Standard camp needs a cleared room. This room isn't cleared yet — clear it first, or `!camp rough` for a partial rest here.")
	}

	cost, ok := campSupplyCost[kind]
	if !ok {
		cost = 0.5
	}
	if exp.Supplies.Current < cost {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Not enough supplies for that camp (need **%.1f SU**, have **%.1f SU**). Try `!camp rough` or break camp and conserve.",
			cost, exp.Supplies.Current))
	}

	// D2-b: event-anchored manual !camp counts as the night camp if it's
	// the first camp since the last rollover. Burn supplies *before* the
	// camp cost so the burn lands against pre-pitch supplies (matches the
	// legacy morning-burn ordering), then pitch, then drift.
	nightCamp := false
	var nightBurn float32
	var nightRoll nightRolloverResult
	if isEventAnchored(exp) {
		var since time.Duration
		if exp.LastBriefingAt != nil {
			since = time.Now().UTC().Sub(*exp.LastBriefingAt)
		} else {
			since = time.Now().UTC().Sub(exp.StartDate)
		}
		if since >= nightCampWindow {
			nightCamp = true
			burn, err := p.nightRolloverBurn(exp)
			if err != nil {
				return p.SendDM(ctx.Sender, "Couldn't burn night supplies: "+err.Error())
			}
			nightBurn = burn
		}
	}

	exp.Supplies.Current -= cost
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't deduct supplies: "+err.Error())
	}
	camp := &CampState{
		Active:        true,
		Type:          kind,
		RoomIndex:     campCurrentRoomIndex(exp),
		EstablishedAt: time.Now().UTC(),
		NightEvents:   []string{},
	}
	if err := updateCamp(exp.ID, camp); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't pitch camp: "+err.Error())
	}
	exp.Camp = camp

	// Apply the long-rest effects immediately so the player isn't waiting
	// until the next 06:00 UTC briefing for HP and spell slots to come
	// back. The flag tells processOvernightCamp not to re-apply at briefing.
	restSummary := applyCampRest(exp, kind)
	if nightCamp {
		nightRoll = p.nightRolloverDrift(exp, time.Now().UTC())
		nightRoll.Burn = nightBurn
	}
	camp.RestApplied = true
	if err := updateCamp(exp.ID, camp); err != nil {
		slog.Warn("camp: mark rest applied", "expedition", exp.ID, "err", err)
	}

	// E4d: pick the BaseCampEstablished pool for base camps; otherwise
	// the generic camp pool. Both already handle [N] day interpolation
	// for the base-camp first-pitch message.
	var line string
	if kind == CampTypeBase {
		line = flavor.Pick(flavor.BaseCampEstablished)
		line = strings.ReplaceAll(line, "[N]", fmt.Sprintf("%d", exp.CurrentDay))
	} else {
		line = flavor.Pick(flavor.CampEstablished)
	}
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "rest",
		fmt.Sprintf("camp pitched (%s) — %.1f SU consumed", kind, cost), line)

	// Mark the region as a persistent base-camp waypoint on first pitch.
	if kind == CampTypeBase {
		if region, ok := CurrentRegion(exp); ok {
			if _, err := addRegionListEntry(exp, regionStateBaseCampKey, region.ID); err != nil {
				return p.SendDM(ctx.Sender, "Couldn't record base camp waypoint: "+err.Error())
			}
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("⛺ **Camp established — %s.**\n", kind))
	b.WriteString(fmt.Sprintf("Supplies: %.1f / %.1f SU (−%.1f).\n",
		exp.Supplies.Current, exp.Supplies.Max, cost))
	if line != "" {
		b.WriteString("\n" + line + "\n")
	}
	if restSummary != "" {
		b.WriteString("\n" + restSummary + "\n")
	}
	switch kind {
	case CampTypeBase:
		b.WriteString("\n_Base camp — **waypoint persisted**. Camp here again at no eligibility cost on later returns._")
	}
	if nightCamp {
		b.WriteString(fmt.Sprintf("\n\n🌙 **Day %d.** _Overnight burn: %.1f SU._\n", exp.CurrentDay, nightRoll.Burn))
		for _, tl := range nightRoll.TemporalLines {
			b.WriteString("\n🌀 " + tl + "\n")
		}
		for _, ml := range nightRoll.MilestoneLines {
			b.WriteString("\n" + ml)
		}
	}
	return p.SendDM(ctx.Sender, b.String())
}

// applyCampRest runs the long-rest effects (HP/spells/threat/heat) for the
// given camp kind and returns a player-facing summary. Shared by camp pitch
// (immediate rest) and overnight rollover (legacy deferred path). Does NOT
// break the camp — callers decide that.
func applyCampRest(e *Expedition, kind string) string {
	uid := id.UserID(e.UserID)
	c, _ := LoadDnDCharacter(uid)
	if c == nil {
		return ""
	}
	// Babysit safe-rest: an active subscription promotes a Standard camp
	// to Fortified for rest purposes (no need for boss-cleared arrangements).
	// Rough/Base are unchanged — Rough still implies no shelter, and Base
	// already exceeds Fortified.
	babysitUpgraded := false
	if kind == CampTypeStandard && BabysitSafeRest(uid) {
		kind = CampTypeFortified
		babysitUpgraded = true
	}
	prevHP := c.HPCurrent
	bonusHP := 0

	switch kind {
	case CampTypeRough:
		half := c.HPMax / 2
		if c.HPCurrent < half {
			c.HPCurrent = half
		}
	case CampTypeStandard:
		c.HPCurrent = c.HPMax
		c.TempHP = 0
		if c.Exhaustion > 0 {
			c.Exhaustion--
		}
	case CampTypeFortified, CampTypeBase:
		c.HPCurrent = c.HPMax
		c.TempHP = 0
		if c.Exhaustion > 0 {
			c.Exhaustion--
		}
		bonusHP = 1 + rand.IntN(6)
		c.HPCurrent += bonusHP
		if c.HPCurrent > c.HPMax {
			c.HPCurrent = c.HPMax
		}
	}
	_ = SaveDnDCharacter(c)
	if kind == CampTypeStandard || kind == CampTypeFortified || kind == CampTypeBase {
		_ = refreshAllResources(uid)
		_ = refreshSpellSlots(uid)
		_ = ReplenishHarvestNodes(e)
	}

	// Threat reduction (§8.1: -5 for fortified long rest).
	if kind == CampTypeFortified || kind == CampTypeBase {
		_ = applyThreatDelta(e.ID, -5, "long rest in fortified camp")
	}

	// §7.3 Underforge: fortified rest drops heat stacks by 2.
	heatReduced := 0
	if (kind == CampTypeFortified || kind == CampTypeBase) && e.ZoneID == ZoneUnderforge {
		before := e.TemporalStack
		after := reduceUnderforgeHeat(e)
		heatReduced = before - after
	}

	switch kind {
	case CampTypeRough:
		if c.HPCurrent > prevHP {
			return fmt.Sprintf("Rough rest: HP %d → %d.", prevHP, c.HPCurrent)
		}
		return "Rough rest: no HP gain (already above the half-HP floor)."
	case CampTypeStandard:
		return fmt.Sprintf("Long rest: HP %d → %d, spell slots & resources refreshed.", prevHP, c.HPCurrent)
	case CampTypeFortified, CampTypeBase:
		label := "Fortified rest"
		if babysitUpgraded {
			label = "Fortified rest (babysitter watching the camp)"
		}
		summary := fmt.Sprintf("%s: HP %d → %d (+1d6 = %d bonus); threat clock −5; resources refreshed.",
			label, prevHP, c.HPCurrent, bonusHP)
		if heatReduced > 0 {
			summary += fmt.Sprintf(" Heat stacks −%d.", heatReduced)
		}
		return summary
	}
	return ""
}

// processOvernightCamp handles the briefing-time half of the camp lifecycle:
// auto-breaks the camp, and applies the long rest only if it wasn't already
// applied at pitch time. Returns a one-line summary for the briefing body, or
// "" if there was nothing to do.
func processOvernightCamp(e *Expedition) string {
	if e.Camp == nil || !e.Camp.Active {
		return ""
	}
	kind := e.Camp.Type
	alreadyApplied := e.Camp.RestApplied
	_ = updateCamp(e.ID, nil)
	e.Camp = nil
	if alreadyApplied {
		return ""
	}
	return applyCampRest(e, kind)
}

func (p *AdventurePlugin) campBreak(ctx MessageContext, exp *Expedition) error {
	if exp.Camp == nil || !exp.Camp.Active {
		return p.SendDM(ctx.Sender, "No camp to break.")
	}
	if err := updateCamp(exp.ID, nil); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't break camp: "+err.Error())
	}
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative", "camp broken", "")
	return p.SendDM(ctx.Sender, "Camp struck. The dungeon awaits.")
}

// campLocationCheck enforces §5.2 placement rules. Returns (clearedRoom, problem).
// Until !advance wires into expeditions, the player is at the entry room
// (always cleared, never trap/boss/active-enemy) so this currently always
// returns (true, "").
func campLocationCheck(exp *Expedition) (cleared bool, problem string) {
	if exp.RunID == "" {
		// No room state — treat as the entry room. Cleared by definition.
		return true, ""
	}
	run, err := getZoneRun(exp.RunID)
	if err != nil || run == nil {
		// Run was wiped/abandoned out from under us; behave like entry.
		return true, ""
	}
	rt := run.CurrentRoomType()
	switch rt {
	case RoomBoss:
		return false, "You can't camp in a boss room. The room knows it's a boss room."
	case RoomTrap:
		return false, "You can't camp in a trap room — even a disarmed one."
	}
	for _, idx := range run.RoomsCleared {
		if idx == run.CurrentRoom {
			return true, ""
		}
	}
	// Not yet advanced-past, but the only thing that bars a rest is a live
	// fight. Forward-only navigation means players pause right after a kill
	// before advancing, and peaceful/exploration/loot rooms never spawn an
	// encounter at all — both are safe to rest in. The "cleared" flag would
	// otherwise reject standard camp here with a misleading "clear it first".
	encID := encounterIDForRoom(run.CurrentRoom)
	sess, err := getCombatSessionForEncounter(run.RunID, encID)
	if err != nil {
		// Can't read the encounter's combat state. Fail open like the
		// run-lookup path above rather than blocking standard camp with an
		// empty (misleading "not cleared") rejection — a DB hiccup shouldn't
		// strand a player who only wants to rest.
		slog.Warn("camp: combat session lookup failed; allowing camp",
			"run", run.RunID, "encounter", encID, "err", err)
		return true, ""
	}
	if sess != nil && sess.Status == CombatStatusActive {
		return false, "You can't camp mid-fight — finish the encounter first."
	}
	return true, ""
}

// autoBreakCampOnMove strikes an active camp when the player has moved
// to a different room than the one camp was pitched in. Camp is a
// stationary intent (long-rest at this spot until the next briefing);
// once the party walks on — whether by autopilot, manual !advance, or
// fork pick — the tent stops mattering. Overnight rest effects are not
// applied here (those only land at briefing time via
// processOvernightCamp). Returns the camp type that was struck, or ""
// if no break happened. Safe to call when there's no expedition.
func autoBreakCampOnMove(userID id.UserID) string {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return ""
	}
	if exp.Camp == nil || !exp.Camp.Active {
		return ""
	}
	if exp.RunID == "" {
		return ""
	}
	run, err := getZoneRun(exp.RunID)
	if err != nil || run == nil {
		return ""
	}
	if run.CurrentRoom == exp.Camp.RoomIndex {
		return ""
	}
	kind := exp.Camp.Type
	if err := updateCamp(exp.ID, nil); err != nil {
		slog.Warn("camp: auto-break on move failed", "expedition", exp.ID, "err", err)
		return ""
	}
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative", "camp struck (party moved on)", "")
	return kind
}

// campCurrentRoomIndex returns 0 (entry) when no room context exists.
func campCurrentRoomIndex(exp *Expedition) int {
	if exp.RunID == "" {
		return 0
	}
	run, err := getZoneRun(exp.RunID)
	if err != nil || run == nil {
		return 0
	}
	return run.CurrentRoom
}
