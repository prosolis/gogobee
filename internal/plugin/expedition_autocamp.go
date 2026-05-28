package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Long-expedition D2-a — autopilot camp scheduler.
//
// The autorun ticker (expedition_autorun.go) calls into here after the
// walk loop returns so the dungeon can pitch its own camp when the party
// is low on HP, or to drop a base-camp waypoint after a region clear.
// Day-rollover semantics are still UTC-anchored in D2-a; D2-b moves
// day++/burn into a night-camp helper this scheduler will eventually
// trigger.
//
// An auto-pitched camp is treated as a transient rest stop: the next
// autorun tick past minAutoCampDwell breaks it itself so the walk can
// continue. Player-pitched camps (`!camp`) never get broken by the
// scheduler — those are an explicit player intent and stay up until
// the player moves on or breaks them.

const (
	// minAutoCampDwell — how long an auto-pitched camp stays up before
	// the autorun ticker breaks it on its own. The autorun cooldown is
	// 2h, so a dwell > 2h guarantees the camp spans at least one full
	// tick of "the player is resting" before the walk resumes.
	minAutoCampDwell = 4 * time.Hour

	// autoCampHPPct — pitch a rest camp when current HP is at or below
	// this fraction of max. Set above autopilotLowHPPct (0.30) so the
	// scheduler camps *before* the walk-preflight gives up.
	autoCampHPPct = 0.55

	// nightCampWindow — D2-b. Event-anchored expeditions roll the day on
	// autopilot night-camp pitch. After this many hours since the last
	// rollover, the next eligible camp is treated as the night camp
	// (Night=true) and triggers processNightCamp. 16h gives the player
	// most of an active day before the engine starts wanting to bed
	// down; a healthy party with no HP pressure still camps at the end
	// of the day rather than walking forever.
	nightCampWindow = 16 * time.Hour
)

// autoCampInputs is the minimal snapshot decideAutopilotCamp needs.
// Pulled out so the decision is pure / cheap to test without DB setup.
type autoCampInputs struct {
	Camp          *CampState
	Run           *DungeonRun
	Multi         bool
	RegionCleared bool
	BaseSite      bool
	BaseAlready   bool
	HPCur, HPMax  int
	Supplies      ExpeditionSupplies
	// D2-b: event-anchored rollover inputs.
	Now            time.Time
	EventAnchored  bool
	LastBriefingAt *time.Time
	StartDate      time.Time
}

// autoCampDecision — what to pitch and why. Reason is a short log-line
// fragment ("region boss cleared", "HP low"). Night=true means the
// caller should run processNightCamp as part of the pitch so the day++/
// supply burn / threat drift happen alongside the rest.
type autoCampDecision struct {
	Kind   string
	Reason string
	Night  bool
}

// decideAutopilotCamp returns the camp to pitch (or ok=false). Pure;
// the executor does the DB work.
func decideAutopilotCamp(in autoCampInputs) (autoCampDecision, bool) {
	if in.Camp != nil && in.Camp.Active {
		return autoCampDecision{}, false
	}
	if in.Run == nil {
		return autoCampDecision{}, false
	}
	rt := in.Run.CurrentRoomType()
	if rt == RoomBoss || rt == RoomTrap {
		return autoCampDecision{}, false
	}
	cleared := false
	for _, idx := range in.Run.RoomsCleared {
		if idx == in.Run.CurrentRoom {
			cleared = true
			break
		}
	}

	// D2-b: night-camp window — for event-anchored expeditions, the
	// autopilot pitches the day's rollover camp when enough real time
	// has elapsed since the last rollover. A flag, not a separate
	// branch, so each pitch below can become a night camp.
	night := false
	if in.EventAnchored {
		var since time.Duration
		if in.LastBriefingAt != nil {
			since = in.Now.Sub(*in.LastBriefingAt)
		} else {
			since = in.Now.Sub(in.StartDate)
		}
		night = since >= nightCampWindow
	}

	// Heuristic — region base-camp waypoint. Eager: pitch once per
	// eligible region after its boss is down. BaseAlready stops the
	// next tick from re-pitching the same waypoint.
	if in.Multi && in.RegionCleared && in.BaseSite && !in.BaseAlready && cleared {
		if in.Supplies.Current >= campSupplyCost[CampTypeBase] {
			return autoCampDecision{
				Kind: CampTypeBase, Night: night,
				Reason: "region boss cleared — pitching base camp waypoint",
			}, true
		}
	}

	// Heuristic — HP-low rest OR end-of-day night camp. Standard if
	// cleared, rough otherwise. LowSU as a *trigger* would just dig
	// the hole deeper; the walk's preflight pauses on low-SU instead.
	lowHP := in.HPMax > 0 && float64(in.HPCur) <= float64(in.HPMax)*autoCampHPPct
	if !lowHP && !night {
		return autoCampDecision{}, false
	}
	kind := CampTypeRough
	if cleared {
		kind = CampTypeStandard
	}
	cost := campSupplyCost[kind]
	if in.Supplies.Current < cost {
		if kind == CampTypeStandard && in.Supplies.Current >= campSupplyCost[CampTypeRough] {
			kind = CampTypeRough
		} else {
			return autoCampDecision{}, false
		}
	}
	reason := "HP low — pitching rest camp"
	switch {
	case night && lowHP:
		reason = "HP low + end of day — pitching night camp"
	case night:
		reason = "end of day — pitching night camp"
	}
	return autoCampDecision{Kind: kind, Reason: reason, Night: night}, true
}

// maybeAutoCamp gathers DB state, calls decideAutopilotCamp, and pitches
// when the decision says to. Returns the player-facing camp block (or
// "" if no camp was pitched), along with the decision and an ok flag.
// D4-a uses the Night bit on the decision to switch tryAutoRun's DM
// rendering into end-of-day digest mode.
func (p *AdventurePlugin) maybeAutoCamp(exp *Expedition) (string, autoCampDecision, bool) {
	uid := id.UserID(exp.UserID)

	var run *DungeonRun
	if exp.RunID != "" {
		if r, err := getZoneRun(exp.RunID); err == nil {
			run = r
		}
	}

	multi := IsMultiRegionZone(exp.ZoneID)
	regionCleared := false
	baseSite := false
	baseAlready := false
	if multi {
		if region, ok := CurrentRegion(exp); ok {
			regionCleared = IsRegionCleared(exp, region.ID)
			baseSite = region.BaseCampSite
			baseAlready = HasBaseCampAt(exp, region.ID)
		}
	}

	hpCur, hpMax := dndHPSnapshot(uid)
	in := autoCampInputs{
		Camp:           exp.Camp,
		Run:            run,
		Multi:          multi,
		RegionCleared:  regionCleared,
		BaseSite:       baseSite,
		BaseAlready:    baseAlready,
		HPCur:          hpCur,
		HPMax:          hpMax,
		Supplies:       exp.Supplies,
		Now:            time.Now().UTC(),
		EventAnchored:  isEventAnchored(exp),
		LastBriefingAt: exp.LastBriefingAt,
		StartDate:      exp.StartDate,
	}
	d, ok := decideAutopilotCamp(in)
	if !ok {
		return "", autoCampDecision{}, false
	}
	block, err := p.pitchAutopilotCamp(exp, d)
	if err != nil {
		slog.Warn("autopilot camp: pitch failed", "expedition", exp.ID, "kind", d.Kind, "err", err)
		return "", autoCampDecision{}, false
	}
	return block, d, true
}

// pitchAutopilotCamp performs the same state mutations as the player
// !camp path (supply debit, camp row, applyCampRest, log entry, base-
// camp waypoint persist) without DMing — the autorun ticker bundles
// the returned block into the single auto-walk DM. When d.Night is true
// (D2-b event-anchored rollover), the day++/burn/threat-drift fire here
// too: nightRolloverBurn before the camp cost (so the burn lands on
// pre-pitch supplies, matching the legacy morning-burn ordering), then
// applyCampRest, then nightRolloverDrift.
func (p *AdventurePlugin) pitchAutopilotCamp(exp *Expedition, d autoCampDecision) (string, error) {
	var (
		nightBurn  float32
		nightTemp  []string
		nightMile  []string
		nightDid   bool
		nightForce bool
	)
	if d.Night {
		burn, err := p.nightRolloverBurn(exp)
		if err != nil {
			return "", err
		}
		nightBurn = burn
		nightDid = true
		if exp.Status != ExpeditionStatusActive {
			// Starvation during burn is rare (burn alone doesn't trigger
			// starvation — that needs supplies <= 0 *after* burn), but
			// guard anyway so we don't pitch a camp on an abandoned exp.
			drift := p.nightRolloverDrift(exp, time.Now().UTC())
			nightTemp = drift.TemporalLines
			nightMile = drift.MilestoneLines
			nightForce = true
			return renderAutoCampBlock(exp, d, 0, "", "", nightBurn, nightTemp, nightMile, nightDid, nightForce), nil
		}
	}
	cost := campSupplyCost[d.Kind]
	if exp.Supplies.Current < cost {
		return "", fmt.Errorf("insufficient supplies (have %.1f, need %.1f)", exp.Supplies.Current, cost)
	}
	exp.Supplies.Current -= cost
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		return "", err
	}
	camp := &CampState{
		Active:        true,
		Type:          d.Kind,
		RoomIndex:     campCurrentRoomIndex(exp),
		EstablishedAt: time.Now().UTC(),
		NightEvents:   []string{},
		AutoPitched:   true,
	}
	if err := updateCamp(exp.ID, camp); err != nil {
		return "", err
	}
	exp.Camp = camp

	restSummary := applyCampRest(exp, d.Kind)
	camp.RestApplied = true
	if err := updateCamp(exp.ID, camp); err != nil {
		slog.Warn("autopilot camp: mark rest applied", "expedition", exp.ID, "err", err)
	}

	var line string
	if d.Kind == CampTypeBase {
		line = flavor.Pick(flavor.BaseCampEstablished)
		line = strings.ReplaceAll(line, "[N]", fmt.Sprintf("%d", exp.CurrentDay))
	} else {
		line = flavor.Pick(flavor.CampEstablished)
	}
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "rest",
		fmt.Sprintf("autopilot camp pitched (%s) — %s — %.1f SU consumed", d.Kind, d.Reason, cost), line)

	if d.Kind == CampTypeBase {
		if region, ok := CurrentRegion(exp); ok {
			if _, err := addRegionListEntry(exp, regionStateBaseCampKey, region.ID); err != nil {
				slog.Warn("autopilot camp: persist base camp waypoint", "expedition", exp.ID, "err", err)
			}
		}
	}

	if d.Night {
		drift := p.nightRolloverDrift(exp, time.Now().UTC())
		nightTemp = drift.TemporalLines
		nightMile = drift.MilestoneLines
	}

	return renderAutoCampBlock(exp, d, cost, line, restSummary,
		nightBurn, nightTemp, nightMile, nightDid, nightForce), nil
}

// renderAutoCampBlock formats the autopilot-camp section appended to
// the auto-walk DM. Kept short — the autorun DM already opens with the
// walk narration. nightBurn/nightTemp/nightMile carry the D2-b rollover
// side effects when the pitch was a night camp.
func renderAutoCampBlock(exp *Expedition, d autoCampDecision, cost float32, flavorLine, restSummary string,
	nightBurn float32, nightTemp []string, nightMile []string, nightDid bool, nightForce bool) string {
	if nightForce {
		out := fmt.Sprintf("\n\n🌙 **Day %d.** _Supplies burned (−%.1f SU); the rollover fired but the camp couldn't pitch._\n", exp.CurrentDay, nightBurn)
		for _, tl := range nightTemp {
			out += "\n🌀 " + tl + "\n"
		}
		for _, ml := range nightMile {
			out += "\n" + ml
		}
		return out
	}
	out := fmt.Sprintf("\n\n⛺ **Autopilot camp — %s.** _%s_\n", d.Kind, d.Reason)
	out += fmt.Sprintf("Supplies: %.1f / %.1f SU (−%.1f).\n",
		exp.Supplies.Current, exp.Supplies.Max, cost)
	if flavorLine != "" {
		out += "\n" + flavorLine + "\n"
	}
	if restSummary != "" {
		out += "\n" + restSummary + "\n"
	}
	if d.Kind == CampTypeBase {
		out += "\n_Base camp — **waypoint persisted**._"
	}
	if nightDid {
		out += fmt.Sprintf("\n\n🌙 **Day %d.** _Overnight burn: %.1f SU._\n", exp.CurrentDay, nightBurn)
		for _, tl := range nightTemp {
			out += "\n🌀 " + tl + "\n"
		}
		for _, ml := range nightMile {
			out += "\n" + ml
		}
	}
	return out
}

// pitchBossSafetyCamp force-pitches a rest camp after the compact
// autopilot bailed out before the boss (stopBossSafety). Bypasses the
// decideAutopilotCamp HP threshold and its RoomBoss room-type block —
// the boss doorway is *exactly* where this camp belongs. Picks the best
// kind the supplies will pay for (Standard > Rough); returns "" if even
// Rough is too expensive (extract-soon territory). The returned block is
// appended to the autorun DM by the caller.
//
// Day-rollover handling mirrors decideAutopilotCamp: when enough real
// time has elapsed since the last briefing on an event-anchored
// expedition, the pitch carries Night=true and runs the burn/drift
// alongside the rest.
func (p *AdventurePlugin) pitchBossSafetyCamp(exp *Expedition) (string, autoCampDecision, bool) {
	if exp == nil || exp.Status != ExpeditionStatusActive {
		return "", autoCampDecision{}, false
	}
	if exp.Camp != nil && exp.Camp.Active {
		// Already resting — nothing to pitch. The dwell window will pass
		// and the next autorun tick will retry the boss engagement.
		return "", autoCampDecision{}, false
	}
	kind := CampTypeStandard
	if exp.Supplies.Current < campSupplyCost[kind] {
		kind = CampTypeRough
	}
	if exp.Supplies.Current < campSupplyCost[kind] {
		// No SU even for Rough — autopilot can't help here. The walk's
		// preflight will surface low-SU on the next tick if the player
		// doesn't extract.
		return "", autoCampDecision{}, false
	}
	night := false
	if isEventAnchored(exp) {
		var since time.Duration
		if exp.LastBriefingAt != nil {
			since = time.Since(*exp.LastBriefingAt)
		} else {
			since = time.Since(exp.StartDate)
		}
		night = since >= nightCampWindow
	}
	d := autoCampDecision{
		Kind:   kind,
		Reason: "boss-safety hold — resting before re-engaging",
		Night:  night,
	}
	block, err := p.pitchAutopilotCamp(exp, d)
	if err != nil {
		slog.Warn("autopilot boss-safety camp: pitch failed", "expedition", exp.ID, "kind", kind, "err", err)
		return "", autoCampDecision{}, false
	}
	return block, d, true
}

// breakAutoCampIfDue tears down an auto-pitched camp once it has dwelled
// for minAutoCampDwell. Returns true when it broke a camp (so the
// caller knows the walk should proceed this tick). Player-pitched camps
// are left untouched.
func breakAutoCampIfDue(exp *Expedition, now time.Time) bool {
	if exp.Camp == nil || !exp.Camp.Active || !exp.Camp.AutoPitched {
		return false
	}
	if now.Sub(exp.Camp.EstablishedAt) < minAutoCampDwell {
		return false
	}
	if err := updateCamp(exp.ID, nil); err != nil {
		slog.Warn("autopilot camp: break failed", "expedition", exp.ID, "err", err)
		return false
	}
	_ = appendExpeditionLog(exp.ID, exp.CurrentDay, "narrative",
		"autopilot broke camp (dwell elapsed)", "")
	exp.Camp = nil
	return true
}

// shouldSkipAutoRunForCamp reports whether the autorun ticker should
// skip this expedition entirely because an auto- or player-pitched
// camp is still within its dwell window. Returns true when the ticker
// should bail before walking.
func shouldSkipAutoRunForCamp(exp *Expedition, now time.Time) bool {
	if exp.Camp == nil || !exp.Camp.Active {
		return false
	}
	// Player camps are sticky — never walked through by autopilot until
	// the player explicitly breaks or moves.
	if !exp.Camp.AutoPitched {
		return true
	}
	// Auto-pitched camps: skip while still in dwell; the next tick past
	// the window will break + walk in breakAutoCampIfDue.
	return now.Sub(exp.Camp.EstablishedAt) < minAutoCampDwell
}
