package plugin

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
)

// randIntN wraps math/rand/v2 IntN so feywildRollFn's seam stays
// dependency-free for tests.
func randIntN(n int) int { return rand.IntN(n) }

// persistRegionState writes the in-memory RegionState back to the DB.
func persistRegionState(e *Expedition) error {
	b, err := json.Marshal(e.RegionState)
	if err != nil {
		return err
	}
	_, err = db.Get().Exec(`
		UPDATE dnd_expedition
		   SET region_state = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, string(b), e.ID)
	return err
}

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

// TemporalBurnOverride lets a zone-temporal event short-circuit the
// default §4.1 burn pipeline with a fixed multiplier on DailyBurn.
// When Multiplier == 0, the default pipeline (HarshMod / siege) is
// used. Non-zero overrides the entire calculation.
type TemporalBurnOverride struct {
	Multiplier float32 // 0 = no override; 0.5 / 2.0 / etc. otherwise
	Reason     string  // short tag for telemetry
}

// applyZoneTemporalPreBurn runs at the top of deliverBriefing, before
// applyDailyBurn. It mutates e (state increments, in-memory only —
// caller persists the relevant fields) and returns an optional
// override on this morning's burn calculation.
//
// nextDay is the day-number we're rolling INTO (i.e. e.CurrentDay+1
// at call site, since the briefing increments after burn).
func applyZoneTemporalPreBurn(e *Expedition, nextDay int) TemporalBurnOverride {
	switch e.ZoneID {
	case ZoneSunkenTemple:
		if sunkenTempleTemporalPreBurn(e, nextDay) {
			return TemporalBurnOverride{Multiplier: 2.0, Reason: "sunken_temple_tidal"}
		}
	case ZoneFeywildCrossing:
		return feywildTemporalPreBurn(e, nextDay)
	}
	return TemporalBurnOverride{}
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
	case ZoneFeywildCrossing:
		return feywildTemporalPostRollover(e)
	case ZoneDragonsLair:
		return dragonsLairTemporalPostRollover(e)
	}
	return nil
}

// DragonsLairAwarenessCycleDays — §7.5 awareness pulse cadence.
const DragonsLairAwarenessCycleDays = 3

// DragonsLairAwakenDay — §7.5 Infernax wakes if not yet at the boss.
const DragonsLairAwakenDay = 14

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

// ─────────────────────────────────────────────────────────────────────
// §7.4 — Feywild Crossing, Time Distortion
// ─────────────────────────────────────────────────────────────────────

// FeywildDistortion is the resolved effect of a daily 1d6 distortion
// roll. Persisted to RegionState["feywild_today"] for the day so
// downstream systems (extra wandering monster check at recap, time
// loop signaling for the room engine) can read it.
type FeywildDistortion string

const (
	FeywildDistortionNormal FeywildDistortion = "normal"
	FeywildDistortionHalf   FeywildDistortion = "half"
	FeywildDistortionDouble FeywildDistortion = "double"
	FeywildDistortionLoop   FeywildDistortion = "loop"
)

// feywildRollFn is the test seam for the 1d6 distortion roll.
var feywildRollFn = func() int { return 1 + randIntN(6) }

// feywildResolveDistortion maps a 1d6 to the §7.4 effect bucket.
func feywildResolveDistortion(d6 int) FeywildDistortion {
	switch d6 {
	case 1, 2:
		return FeywildDistortionNormal
	case 3, 4:
		return FeywildDistortionHalf
	case 5:
		return FeywildDistortionDouble
	default:
		return FeywildDistortionLoop
	}
}

// feywildTemporalPreBurn rolls today's distortion, persists it to
// RegionState["feywild_today"], and returns the burn-override based on
// the result. Boss-defeated suppresses entirely.
func feywildTemporalPreBurn(e *Expedition, nextDay int) TemporalBurnOverride {
	if e.BossDefeated {
		return TemporalBurnOverride{}
	}
	d6 := feywildRollFn()
	effect := feywildResolveDistortion(d6)
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	e.RegionState["feywild_today"] = string(effect)
	e.RegionState["feywild_today_d6"] = d6
	_ = persistRegionState(e)

	switch effect {
	case FeywildDistortionHalf:
		return TemporalBurnOverride{Multiplier: 0.5, Reason: "feywild_half_day"}
	case FeywildDistortionDouble:
		return TemporalBurnOverride{Multiplier: 2.0, Reason: "feywild_double_day"}
	}
	return TemporalBurnOverride{}
}

// feywildTemporalPostRollover narrates today's distortion. Normal-day
// rolls produce no narration (they're the baseline).
func feywildTemporalPostRollover(e *Expedition) []string {
	if e.BossDefeated {
		return nil
	}
	raw, _ := e.RegionState["feywild_today"].(string)
	switch FeywildDistortion(raw) {
	case FeywildDistortionHalf:
		line := flavor.Pick(flavor.FeywildTimeDistortionHalf)
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			"feywild distortion: half-day (0.5× burn)", line)
		return []string{line}
	case FeywildDistortionDouble:
		line := flavor.Pick(flavor.FeywildTimeDistortionDouble)
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			"feywild distortion: double-day (2× burn, extra wandering check pending)", line)
		return []string{line}
	case FeywildDistortionLoop:
		line := flavor.Pick(flavor.FeywildTimeLoop)
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "temporal",
			"feywild distortion: time loop (room re-enters with new enemies; loot already taken)",
			line)
		return []string{line}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────
// §7.5 — Dragon's Lair, Awareness Pulses + Infernax Awakening
// ─────────────────────────────────────────────────────────────────────

// dragonsLairTemporalPostRollover fires an awareness pulse every 3
// days (threat +10, narration) and the Day-14 awakening event if the
// boss hasn't been killed yet.
func dragonsLairTemporalPostRollover(e *Expedition) []string {
	if e.BossDefeated {
		return nil
	}
	day := e.CurrentDay
	var lines []string

	// Day 14 awakening: one-time. Mark on RegionState; downstream
	// combat-link reads "infernax_awake" to switch wandering cadence
	// to every 6 hours and add the dragon encounter weighting.
	if day >= DragonsLairAwakenDay {
		if awake, _ := e.RegionState["infernax_awake"].(bool); !awake {
			e.RegionState["infernax_awake"] = true
			_ = persistRegionState(e)
			line := flavor.Pick(flavor.DragonsLairAwakenWarning)
			_ = appendExpeditionLog(e.ID, day, "temporal",
				"infernax awakens — active hunting begins, 6h wandering checks",
				line)
			lines = append(lines, line)
		}
	}

	// Awareness pulse every 3 days (3, 6, 9, 12, ...). On Day 14, the
	// awakening narration takes precedence — we still apply the +10
	// since the cadence math says it's a pulse day too.
	if day >= DragonsLairAwarenessCycleDays && day%DragonsLairAwarenessCycleDays == 0 {
		_ = applyThreatDelta(e.ID, 10, fmt.Sprintf("dragon awareness pulse (day %d)", day))
		// Refresh in-memory threat after the bump.
		e.ThreatLevel += 10
		if e.ThreatLevel > 100 {
			e.ThreatLevel = 100
		}
		// Skip the pulse narration if the awaken line already fired
		// today — they say largely the same thing in different keys.
		alreadyAwoke := false
		for _, l := range lines {
			if l != "" {
				alreadyAwoke = true
				break
			}
		}
		if !alreadyAwoke {
			line := flavor.Pick(flavor.DragonsLairAwarenessPulse)
			_ = appendExpeditionLog(e.ID, day, "temporal",
				fmt.Sprintf("awareness pulse — threat +10 (day %d)", day),
				line)
			lines = append(lines, line)
		}
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
