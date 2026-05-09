package plugin

import (
	"fmt"
	"strings"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E2a — Threat Clock state machine.
// Spec: gogobee_expedition_system.md §8.
//
// E1a already shipped the persistence (applyThreatDelta, ThreatEvent,
// threat_level / threat_siege columns). This file layers the threshold
// model on top: labels, per-band combat effects, threshold-crossing log
// flavor, and the daily threat drift applied at briefing rollover.
//
// Combat-driven modifiers (combat-in-new-room +5, escape +10, loud-ability
// +8) require the combat engine to know about expeditions, which it does
// not yet. They land alongside the !advance/combat hookup in a later phase;
// this file is wired into the bits we *can* drive: day rollover + boss
// defeat + manual threat events from camp/extraction logic.

// ThreatBand is the bracketed state of the threat clock.
type ThreatBand int

const (
	ThreatBandQuiet ThreatBand = iota
	ThreatBandStirring
	ThreatBandAlert
	ThreatBandHostile
	ThreatBandSiege
)

// ThreatBandInfo bundles the per-band display + combat-side knobs.
// AC/Init/Perception adjustments are applied by the combat engine when
// it reads the active expedition's threat band; SupplyMult feeds into
// currentBurn() (already wired); ShortRest disabled is checked by the
// rest plumbing once expedition combat is wired.
type ThreatBandInfo struct {
	Band       ThreatBand
	Label      string
	Perception int     // enemy passive perception bonus
	AC         int     // enemy AC bonus
	InitAdv    bool    // enemy initiative advantage
	TrapsArm   bool    // re-arm cleared traps each day
	SupplyMult float32 // multiplier on daily supply burn
	NoShortRest bool   // siege-mode only
}

// threatBandFor returns the band for a given level, honouring siege override.
func threatBandFor(level int, siege bool) ThreatBand {
	if siege || level >= 81 {
		return ThreatBandSiege
	}
	switch {
	case level >= 61:
		return ThreatBandHostile
	case level >= 41:
		return ThreatBandAlert
	case level >= 21:
		return ThreatBandStirring
	default:
		return ThreatBandQuiet
	}
}

// threatBandInfo returns the descriptor for a band.
func threatBandInfo(b ThreatBand) ThreatBandInfo {
	switch b {
	case ThreatBandStirring:
		return ThreatBandInfo{Band: b, Label: "Stirring", Perception: 1, SupplyMult: 1}
	case ThreatBandAlert:
		return ThreatBandInfo{Band: b, Label: "Alert", AC: 2, Perception: 2, SupplyMult: 1}
	case ThreatBandHostile:
		return ThreatBandInfo{Band: b, Label: "Hostile", AC: 2, Perception: 2, InitAdv: true, TrapsArm: true, SupplyMult: 1}
	case ThreatBandSiege:
		return ThreatBandInfo{Band: b, Label: "Siege", AC: 2, Perception: 2, InitAdv: true, TrapsArm: true, SupplyMult: 2, NoShortRest: true}
	}
	return ThreatBandInfo{Band: ThreatBandQuiet, Label: "Quiet", SupplyMult: 1}
}

// dailyThreatDrift is the +3/day base (§8.1) plus the DMMood-driven mod
// (Wrathful +5, Elated -3). Mood bands map: effusive (≥80) → Elated,
// hostile (<20) → Wrathful, the middle three are neutral.
func dailyThreatDrift(dmMood int) (int, string) {
	base := 3
	mod := 0
	tag := ""
	switch {
	case dmMood >= 80:
		mod = -3
		tag = "elated"
	case dmMood < 20:
		mod = 5
		tag = "wrathful"
	}
	delta := base + mod
	reason := "day passes in zone"
	if tag != "" {
		reason = fmt.Sprintf("day passes in zone (%s mood mod %+d)", tag, mod)
	}
	return delta, reason
}

// applyDailyThreatDrift applies the per-day threat increment at briefing
// rollover. No-op once the boss has been defeated (the zone has nothing
// left to escalate). Returns the delta actually applied.
func applyDailyThreatDrift(e *Expedition) (int, string, error) {
	if e.BossDefeated {
		return 0, "", nil
	}
	delta, reason := dailyThreatDrift(e.DMMood)
	if delta == 0 {
		return 0, reason, nil
	}
	prevLevel := e.ThreatLevel
	prevBand := threatBandFor(e.ThreatLevel, e.SiegeMode)
	if err := applyThreatDelta(e.ID, delta, reason); err != nil {
		return 0, reason, err
	}
	// Mirror the change in-memory so the caller can render it.
	e.ThreatLevel += delta
	if e.ThreatLevel > 100 {
		e.ThreatLevel = 100
	}
	if e.ThreatLevel < 0 {
		e.ThreatLevel = 0
	}
	if e.ThreatLevel >= 100 {
		e.SiegeMode = true
	}
	newBand := threatBandFor(e.ThreatLevel, e.SiegeMode)
	if newBand != prevBand && newBand > prevBand {
		_ = appendThreatTransitionLog(e, newBand)
	}
	// E2c: §8.3 — one-time-per-expedition warning when crossing 70.
	// Track in RegionState so a drop-and-recross (e.g. fortified-camp
	// -5 followed by daily +3 drift) doesn't re-fire the beat.
	if prevLevel < 70 && e.ThreatLevel >= 70 {
		warned, _ := e.RegionState["siege_warning_fired"].(bool)
		if !warned {
			_ = appendApproachingSiegeLog(e)
			if e.RegionState == nil {
				e.RegionState = map[string]any{}
			}
			e.RegionState["siege_warning_fired"] = true
			_ = persistRegionState(e)
		}
	}
	return delta, reason, nil
}

// appendApproachingSiegeLog records the §8.3 "begin warning at 70" beat
// once per expedition. Caller is responsible for only firing it on the
// crossing edge (see applyDailyThreatDrift).
func appendApproachingSiegeLog(e *Expedition) error {
	line := flavor.Pick(flavor.ThreatClockApproachingSiege)
	return appendExpeditionLog(e.ID, e.CurrentDay, "threat",
		"threat clock past 70 — siege approaches", line)
}

// appendThreatTransitionLog records a band-crossing in the expedition log
// with the prewritten TwinBee narration for that band.
func appendThreatTransitionLog(e *Expedition, band ThreatBand) error {
	var pool []string
	switch band {
	case ThreatBandStirring:
		pool = flavor.ThreatClockStirring
	case ThreatBandAlert:
		pool = flavor.ThreatClockAlert
	case ThreatBandHostile:
		pool = flavor.ThreatClockHostile
	case ThreatBandSiege:
		pool = flavor.ThreatClockSiege
	default:
		return nil
	}
	line := flavor.Pick(pool)
	info := threatBandInfo(band)
	summary := fmt.Sprintf("threat clock crossed into %s (%d/100)", info.Label, e.ThreatLevel)
	return appendExpeditionLog(e.ID, e.CurrentDay, "threat", summary, line)
}

// handleThreatCmd surfaces the current threat clock state — level, band,
// per-band combat effects, and the last few ThreatEvent log entries.
func (p *AdventurePlugin) handleThreatCmd(ctx MessageContext) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender,
			"No active expedition. Threat clock only ticks during expeditions.")
	}
	band := threatBandFor(exp.ThreatLevel, exp.SiegeMode)
	info := threatBandInfo(band)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⏰ **Threat Clock — %d / 100 — %s**\n\n", exp.ThreatLevel, info.Label))
	b.WriteString(threatBandEffectsBlock(info))
	if len(exp.ThreatEvents) > 0 {
		b.WriteString("\n**Recent threat events:**\n")
		// Show last 5 newest-last for chronological reading.
		start := 0
		if len(exp.ThreatEvents) > 5 {
			start = len(exp.ThreatEvents) - 5
		}
		for _, ev := range exp.ThreatEvents[start:] {
			sign := "+"
			if ev.Delta < 0 {
				sign = ""
			}
			b.WriteString(fmt.Sprintf("  • %s%d — %s _(at %s)_\n",
				sign, ev.Delta, ev.Reason, ev.Timestamp.Format("2006-01-02 15:04")))
		}
	}
	if exp.SiegeMode {
		b.WriteString("\n_Siege Mode is active. The dungeon's final form. It cannot be reduced — only finished or escaped._\n")
	} else if exp.ThreatLevel >= 70 {
		b.WriteString("\n_Past 70: the window for stealth has closed. Finish or extract._\n")
	}
	return p.SendDM(ctx.Sender, b.String())
}

// threatBandEffectsBlock renders the band's combat/supply effects.
func threatBandEffectsBlock(info ThreatBandInfo) string {
	if info.Band == ThreatBandQuiet {
		return "_The zone is unaware of you. No modifications in effect._\n"
	}
	var b strings.Builder
	b.WriteString("**Effects:**\n")
	if info.Perception > 0 {
		b.WriteString(fmt.Sprintf("  • Enemy perception +%d\n", info.Perception))
	}
	if info.AC > 0 {
		b.WriteString(fmt.Sprintf("  • Enemy AC +%d\n", info.AC))
	}
	if info.InitAdv {
		b.WriteString("  • Enemies have initiative advantage\n")
	}
	if info.TrapsArm {
		b.WriteString("  • Cleared traps re-arm overnight\n")
	}
	if info.SupplyMult > 1 {
		b.WriteString(fmt.Sprintf("  • Supply burn ×%.1f\n", info.SupplyMult))
	}
	if info.NoShortRest {
		b.WriteString("  • Short rests blocked (siege)\n")
	}
	return b.String()
}

// applyBossDefeatThreat records the -20 threat drop when a zone boss is
// killed (§8.1). Caller wires it from the combat completion path once
// expedition combat is hooked up.
func applyBossDefeatThreat(expID string) error {
	return applyThreatDelta(expID, -20, "boss defeated")
}

// applyRoomCombatThreat records the +5 threat bump per §8.1 when the
// player resolves combat in a new (non-boss) room. Silent no-op for
// standalone zone runs (no active expedition).
func applyRoomCombatThreatForUser(userID id.UserID, elite bool) {
	exp, err := getActiveExpedition(userID)
	if err != nil || exp == nil {
		return
	}
	delta := 5
	if elite {
		delta = 8 // elite encounters are louder
	}
	_ = applyThreatDelta(exp.ID, delta, "combat in new room")
}
