package plugin

import (
	"fmt"
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
	switch kind {
	case CampTypeRough:
		b.WriteString("\n_Rough camp — partial rest. HP recovers to 50%, no spell slots restored._")
	case CampTypeFortified:
		b.WriteString("\n_Fortified camp — long rest + 1d6 HP bonus on wake; threat clock −5; wandering rolls −4._")
	case CampTypeBase:
		b.WriteString("\n_Base camp — long rest + 1d6 HP bonus; threat clock −5; wandering rolls −6. **Waypoint persisted** — camp here again at no eligibility cost on later returns._")
	default:
		b.WriteString("\n_Standard camp — full long rest at the next morning briefing._")
	}
	return p.SendDM(ctx.Sender, b.String())
}

// processOvernightCamp applies the overnight long-rest effects of an
// active camp at briefing time (§3, §5.1, §8.1). Auto-breaks the camp
// after the rest. Returns a one-line summary for the briefing body.
//
// Effects:
//   - rough: HP recovered to at least 50% of max.
//   - standard: HP fully restored, spell slots refreshed, exhaustion -1.
//   - fortified: standard + 1d6 HP bonus on top, threat -5.
//   - base: same as fortified for the rest itself; persistent waypoint
//     mechanics land in E4.
//
// Returns "" if the expedition wasn't camped overnight.
func processOvernightCamp(e *Expedition) string {
	if e.Camp == nil || !e.Camp.Active {
		return ""
	}
	uid := id.UserID(e.UserID)
	c, _ := LoadDnDCharacter(uid)
	if c == nil {
		// No character to apply HP/spells to; just break the camp.
		_ = updateCamp(e.ID, nil)
		return ""
	}
	// Babysit safe-rest: an active subscription promotes a Standard camp
	// to Fortified for rest purposes (no need for boss-cleared arrangements).
	// Rough/Base are unchanged — Rough still implies no shelter, and Base
	// already exceeds Fortified.
	babysitUpgraded := false
	kind := e.Camp.Type
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

	// Auto-break the camp now that the rest has been applied.
	_ = updateCamp(e.ID, nil)
	e.Camp = nil

	// Pretty summary for the briefing body.
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
		return false, ""
	}
	if sess != nil && sess.Status == CombatStatusActive {
		return false, "You can't camp mid-fight — finish the encounter first."
	}
	return true, ""
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
