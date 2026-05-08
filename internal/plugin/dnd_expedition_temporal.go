package plugin

import (
	"fmt"
	"strings"

	"gogobee/internal/flavor"
)

// Phase 12 E3 — Zone-specific Temporal Events.
// Spec: gogobee_expedition_system.md §7.
//
// Temporal events fire at briefing rollover and produce two outputs:
//   - extraHarsh: forces double supply burn for the burn fired by THIS
//     briefing (overrides the §4.1 zone HarshMod when the multiplier
//     should be 2× regardless of tier — e.g. tidal flooding §7.1).
//   - one or more flavor-bearing log entries (warnings, event narration).
//
// applyZoneTemporalPreBurn runs BEFORE applyDailyBurn so the burn
// reflects the temporal effect for the day being entered. State that
// persists across days (Heat stacks, Portal instability) is mutated
// here as well — the per-day increment is the temporal event.
//
// Combat-side knobs (tidal +1d6 cold, Underforge +1d4 fire, etc.) are
// exposed via ZoneTemporalEffects for the combat engine to read; the
// engine hook lands with the combat-link phase.

// ZoneTemporalEffects bundles the per-zone, per-day combat-side
// modifications produced by temporal events. Combat code reads this
// from the active expedition; nothing reads it yet (combat-link phase).
type ZoneTemporalEffects struct {
	// TidalActive — Sunken Temple Day 6 only. Combat: +1d6 cold per
	// turn while wading; kuo-toa +2 AC, +1d4 attack rolls.
	TidalActive bool

	// UnderforgeHeatStacks — §7.3, 0–10. 1–3 ambient, 4–6 +1d4 fire
	// per round in combat, 7–9 disadvantage on CON saves, 10 -2 to
	// all rolls + forced rest/extraction.
	UnderforgeHeatStacks int

	// UnderforgeHeatBand classifies the stacks into the spec's
	// behavior bands: "ambient", "warning", "supply", "critical".
	UnderforgeHeatBand string
}

// UnderforgeHeatBandFor returns the §7.3 band label for a heat count.
func UnderforgeHeatBandFor(stacks int) string {
	switch {
	case stacks <= 0:
		return "none"
	case stacks <= 3:
		return "ambient"
	case stacks <= 6:
		return "warning"
	case stacks <= 9:
		return "supply"
	default:
		return "critical"
	}
}

// zoneTemporalHarsh reports whether per-zone temporal state forces
// harsh-conditions burn for THIS day's burn calculation. (Force-double
// — overriding HarshMod with a 2× floor — uses the separate pre-burn
// hook instead.)
func zoneTemporalHarsh(e *Expedition) bool {
	switch e.ZoneID {
	case ZoneUnderforge:
		// §7.3: heat 7–9 → supply burn +50%. Heat 10 also harsh.
		return e.TemporalStack >= 7
	}
	return false
}

// applyZoneTemporalPreBurn runs at the top of deliverBriefing, before
// applyDailyBurn. It mutates e (state increments, in-memory only —
// caller persists the relevant fields) and returns whether this
// briefing's burn should be force-doubled regardless of HarshMod.
//
// nextDay is the day-number we're rolling INTO (i.e. e.CurrentDay+1
// at call site, since the briefing increments after burn).
func applyZoneTemporalPreBurn(e *Expedition, nextDay int) (extraHarsh bool) {
	switch e.ZoneID {
	case ZoneSunkenTemple:
		extraHarsh = sunkenTempleTemporalPreBurn(e, nextDay)
	}
	return extraHarsh
}

// ManorResetCycleDays is the §7.2 reset cadence — non-boss rooms
// respawn one enemy each on the morning of every Nth day.
const ManorResetCycleDays = 3

// applyZoneTemporalPostRollover runs AFTER advance + threat drift.
// It appends per-zone narration log entries for the day that just
// became current. Returns lines to append to the briefing body.
func applyZoneTemporalPostRollover(e *Expedition) []string {
	switch e.ZoneID {
	case ZoneSunkenTemple:
		return sunkenTempleTemporalPostRollover(e)
	case ZoneManorBlackspire:
		return manorTemporalPostRollover(e)
	case ZoneUnderforge:
		return underforgeTemporalPostRollover(e)
	}
	return nil
}

// UnderforgeMaxHeat is the §7.3 cap.
const UnderforgeMaxHeat = 10

// ─────────────────────────────────────────────────────────────────────
// §7.1 — Sunken Temple, Tidal Event (Day 6)
// ─────────────────────────────────────────────────────────────────────

// sunkenTempleTemporalPreBurn doubles burn on Day 6 (tidal peak). Skips
// entirely if the boss has been defeated — the spec's no-op clause.
func sunkenTempleTemporalPreBurn(e *Expedition, nextDay int) bool {
	if e.BossDefeated {
		return false
	}
	return nextDay == 6
}

// sunkenTempleTemporalPostRollover writes warnings on Day 4/5 and the
// event narration on Day 6. Boss-kill before Day 6 silences all of it.
func sunkenTempleTemporalPostRollover(e *Expedition) []string {
	if e.BossDefeated {
		return nil
	}
	day := e.CurrentDay
	switch day {
	case 4, 5:
		line := flavor.Pick(flavor.SunkenTempleTidalWarning)
		line = strings.ReplaceAll(line, "[N]", fmt.Sprintf("%d", day))
		summary := fmt.Sprintf("tidal warning — peak in %d day(s)", 6-day)
		_ = appendExpeditionLog(e.ID, day, "temporal", summary, line)
		return []string{line}
	case 6:
		line := flavor.Pick(flavor.SunkenTempleTidalEvent)
		_ = appendExpeditionLog(e.ID, day, "temporal",
			"tidal peak — flooded rooms hostile, supply burn doubled", line)
		return []string{line}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────
// §7.2 — Haunted Manor of Blackspire, Nightly Reset
// ─────────────────────────────────────────────────────────────────────

// ManorReset reports whether the briefing rolling onto `day` is the
// reset morning (every 3rd day: 3, 6, 9, ...). Boss-defeated suppresses.
func ManorReset(e *Expedition, day int) bool {
	if e.BossDefeated || e.ZoneID != ZoneManorBlackspire {
		return false
	}
	return day >= ManorResetCycleDays && day%ManorResetCycleDays == 0
}

// manorTemporalPostRollover writes a "house reset" entry on Day 3, 6,
// 9, ... and a quiet pre-reset warning on the day after that (Day 4,
// 7, ...) — which corresponds to the briefing fired on the morning
// after the warning night, so we anchor narration at the briefing.
//
// Spec §7.2: "TwinBee notes on Night 2: 'The house has been quiet.
// That will not last.' On Night 3 morning: 'Overnight, something moved
// back in.'" — we fire the reset narration on the reset morning. The
// quiet-warning Night 2 line lives on the recap path (E2b's deliverRecap
// will add it as a separate hook in a future commit if desired); for
// E3b we keep the rule simple: only the reset morning fires here.
func manorTemporalPostRollover(e *Expedition) []string {
	day := e.CurrentDay
	if !ManorReset(e, day) {
		return nil
	}
	line := flavor.Pick(flavor.HauntedManorResetMorning)
	_ = appendExpeditionLog(e.ID, day, "temporal",
		fmt.Sprintf("manor reset — non-boss rooms respawned one enemy each (cycle day %d)", day),
		line)
	return []string{line}
}

// ─────────────────────────────────────────────────────────────────────
// §7.3 — The Underforge, Heat Accumulation
// ─────────────────────────────────────────────────────────────────────

// underforgeTemporalPostRollover increments Heat Stacks by 1/day (cap
// 10), persists the change, and emits narration on band-crossing into
// "warning" (4) and "critical" (10). Boss-defeated suppresses further
// accumulation but does not reset existing heat — the pit stays hot.
func underforgeTemporalPostRollover(e *Expedition) []string {
	if e.BossDefeated {
		return nil
	}
	prev := e.TemporalStack
	if prev >= UnderforgeMaxHeat {
		return nil
	}
	next := prev + 1
	if next > UnderforgeMaxHeat {
		next = UnderforgeMaxHeat
	}
	e.TemporalStack = next
	if err := updateTemporalStack(e.ID, next); err != nil {
		return nil
	}
	prevBand := UnderforgeHeatBandFor(prev)
	newBand := UnderforgeHeatBandFor(next)

	var lines []string
	switch {
	case prevBand != "warning" && newBand == "warning":
		// First crossing into 4–6.
		line := flavor.Pick(flavor.UnderforgHeapWarning)
		line = strings.ReplaceAll(line, "[N]", fmt.Sprintf("%d", next))
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			fmt.Sprintf("heat stack %d — warning band: +1d4 fire damage in combat rounds", next),
			line)
		lines = append(lines, line)
	case prevBand != "supply" && newBand == "supply":
		// Crossed into 7–9: supply +50%, dis on CON saves.
		line := flavor.Pick(flavor.UnderforgHeapWarning)
		line = strings.ReplaceAll(line, "[N]", fmt.Sprintf("%d", next))
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			fmt.Sprintf("heat stack %d — supply burn +50%%, disadvantage on CON saves", next),
			line)
		lines = append(lines, line)
	case prevBand != "critical" && newBand == "critical":
		// Crossed into 10: exhaustion threshold.
		line := flavor.Pick(flavor.UnderforgHeapCritical)
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			"heat stack 10 — exhaustion: -2 to all rolls; rest in fortified camp or extract",
			line)
		lines = append(lines, line)
	}
	return lines
}

// reduceUnderforgeHeat is called by processOvernightCamp when the
// active camp is fortified (or base) in the Underforge — long rest
// drops heat stacks by 2 per §7.3. Returns the new stack count.
func reduceUnderforgeHeat(e *Expedition) int {
	if e.ZoneID != ZoneUnderforge {
		return e.TemporalStack
	}
	next := e.TemporalStack - 2
	if next < 0 {
		next = 0
	}
	if next == e.TemporalStack {
		return next
	}
	e.TemporalStack = next
	_ = updateTemporalStack(e.ID, next)
	return next
}
