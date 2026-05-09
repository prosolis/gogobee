package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"gogobee/internal/flavor"

	"maunium.net/go/mautrix/id"
)

// Phase 12 E2b — Wandering monster system & night phase resolution.
// Spec: gogobee_expedition_system.md §6.
//
// The check fires once per night during the 21:00 recap when the player
// has an active camp. Result is logged with a TwinBee narration and
// surfaced in the recap body. Combat resolution itself ("player wakes
// to combat") is deferred — expeditions don't yet host their own combat
// loop, so for E2b a triggered encounter persists as a NightEvents
// entry on the camp and a flavored "night" log line. The combat-link
// phase will read pending night encounters at !advance time.

// NightOutcome enumerates the d20 result buckets per §6.1.
type NightOutcome int

const (
	NightOutcomePeaceful NightOutcome = iota
	NightOutcomePassage
	NightOutcomeMinor
	NightOutcomeStandard
	NightOutcomeElite
	NightOutcomeAmbush
)

// NightCheck is the resolved roll + chosen monster (if any).
type NightCheck struct {
	Outcome      NightOutcome
	Roll         int    // raw d20
	Total        int    // roll + modifiers
	ThreatMod    int    // §6.1 threat modifier
	CampMod      int    // rough +3, fortified -4
	ClassMod     int    // ranger -2 (wilderness)
	MonsterID    string // bestiary id of the encounter, if any
	MonsterName  string
	Summary      string // short factual line
	Flavor       string // TwinBee voice line, may be empty
	ThreatBumped bool   // signs-of-passage adds +2 threat
}

// Class-tag for ranger discount: §6.3 "while in a wilderness zone".
// We treat outdoor / forest / underdark / feywild zones as wilderness.
var wildernessZones = map[ZoneID]bool{
	ZoneForestShadows:   true,
	ZoneSunkenTemple:    true, // tidal exterior
	ZoneUnderdark:       true,
	ZoneFeywildCrossing: true,
	ZoneDragonsLair:     true, // mountain
}

// resolveWanderingCheck rolls the d20 and applies modifiers per §6.1–6.3.
// charClass is the active character's class (used for ranger/rogue mods);
// pass "" if unknown. roll1d20 is injectable for testing.
func resolveWanderingCheck(e *Expedition, charClass DnDClass, roll1d20 func() int) NightCheck {
	if e.Camp == nil || !e.Camp.Active {
		return NightCheck{Outcome: NightOutcomePeaceful, Summary: "no camp — no night check"}
	}
	if roll1d20 == nil {
		roll1d20 = func() int { return rand.IntN(20) + 1 }
	}
	r := roll1d20()

	threatMod := 0
	if e.ThreatLevel > 30 {
		// §6.1: +1 per full 10 points above 30. Level 40 → +1; 35 → +0.
		threatMod = (e.ThreatLevel - 30) / 10
	}

	// Babysit safe-rest: an active subscription gives Standard camps the
	// fortified-tier night-risk reduction (the babysitter is watching).
	effectiveCamp := e.Camp.Type
	if effectiveCamp == CampTypeStandard && BabysitSafeRest(id.UserID(e.UserID)) {
		effectiveCamp = CampTypeFortified
	}
	campMod := 0
	switch effectiveCamp {
	case CampTypeRough:
		campMod = 3
	case CampTypeFortified:
		campMod = -4
	case CampTypeBase:
		campMod = -6
	}

	classMod := 0
	if charClass == ClassRanger && wildernessZones[e.ZoneID] {
		classMod = -2
	}

	total := r + threatMod + campMod + classMod
	out := wanderingOutcome(total)

	nc := NightCheck{
		Outcome:   out,
		Roll:      r,
		Total:     total,
		ThreatMod: threatMod,
		CampMod:   campMod,
		ClassMod:  classMod,
	}

	switch out {
	case NightOutcomePeaceful:
		nc.Summary = "Peaceful night."
	case NightOutcomePassage:
		nc.Summary = "Signs of passage near camp; no encounter."
		nc.ThreatBumped = true
	case NightOutcomeMinor, NightOutcomeStandard, NightOutcomeElite, NightOutcomeAmbush:
		mid, mname := pickWanderingMonster(e.ZoneID, out)
		nc.MonsterID = mid
		nc.MonsterName = mname
		nc.Summary = fmt.Sprintf("%s — %s.", outcomeLabel(out), describeMonsterCount(out, mname))
	}

	nc.Flavor = wanderingFlavor(out, e.ThreatLevel, e.SiegeMode)
	return nc
}

// wanderingOutcome buckets the modified total per §6.1.
func wanderingOutcome(total int) NightOutcome {
	switch {
	case total >= 20:
		return NightOutcomeAmbush
	case total >= 18:
		return NightOutcomeElite
	case total >= 15:
		return NightOutcomeStandard
	case total >= 11:
		return NightOutcomeMinor
	case total >= 6:
		return NightOutcomePassage
	default:
		return NightOutcomePeaceful
	}
}

func outcomeLabel(o NightOutcome) string {
	switch o {
	case NightOutcomePeaceful:
		return "Peaceful night"
	case NightOutcomePassage:
		return "Signs of passage"
	case NightOutcomeMinor:
		return "Minor encounter"
	case NightOutcomeStandard:
		return "Standard encounter"
	case NightOutcomeElite:
		return "Elite encounter"
	case NightOutcomeAmbush:
		return "Ambush"
	}
	return "Night check"
}

func describeMonsterCount(o NightOutcome, name string) string {
	switch o {
	case NightOutcomeMinor:
		return fmt.Sprintf("1–2 %s wake you", pluralize(name))
	case NightOutcomeStandard:
		return fmt.Sprintf("a group of %s closes in", pluralize(name))
	case NightOutcomeElite:
		return fmt.Sprintf("an elite %s steps out of the dark", name)
	case NightOutcomeAmbush:
		return fmt.Sprintf("an elite %s catches you mid-sleep — surprise round", name)
	}
	return name
}

func pluralize(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"), strings.HasSuffix(lower, "z"):
		return name + "es"
	default:
		return name + "s"
	}
}

// pickWanderingMonster pulls a roster entry from the zone, biased by elite
// flag for elite/ambush outcomes. Returns "", "" if the zone has no roster.
func pickWanderingMonster(zoneID ZoneID, o NightOutcome) (string, string) {
	zone, ok := getZone(zoneID)
	if !ok || len(zone.Enemies) == 0 {
		return "", ""
	}
	wantElite := o == NightOutcomeElite || o == NightOutcomeAmbush
	var pool []ZoneEnemy
	for _, en := range zone.Enemies {
		if en.IsElite == wantElite {
			pool = append(pool, en)
		}
	}
	if len(pool) == 0 {
		// Fallback: any enemy regardless of elite flag.
		pool = zone.Enemies
	}
	// Weighted pick by SpawnWeight; default 5 if zero.
	totalW := 0
	for _, en := range pool {
		w := en.SpawnWeight
		if w <= 0 {
			w = 5
		}
		totalW += w
	}
	pickW := rand.IntN(totalW) + 1
	cum := 0
	for _, en := range pool {
		w := en.SpawnWeight
		if w <= 0 {
			w = 5
		}
		cum += w
		if pickW <= cum {
			if m, ok := dndBestiary[en.BestiaryID]; ok {
				return en.BestiaryID, m.Name
			}
			return en.BestiaryID, en.BestiaryID
		}
	}
	return "", ""
}

// wanderingFlavor picks a TwinBee line from the existing ThreatClock pools
// when the band's voice fits the outcome; falls back to "" so the recap
// renders just the factual summary.
func wanderingFlavor(o NightOutcome, threat int, siege bool) string {
	if o == NightOutcomePeaceful || o == NightOutcomePassage {
		return ""
	}
	band := threatBandFor(threat, siege)
	switch band {
	case ThreatBandSiege:
		return flavor.Pick(flavor.ThreatClockSiege)
	case ThreatBandHostile:
		return flavor.Pick(flavor.ThreatClockHostile)
	case ThreatBandAlert:
		return flavor.Pick(flavor.ThreatClockAlert)
	case ThreatBandStirring:
		return flavor.Pick(flavor.ThreatClockStirring)
	}
	return ""
}

// renderNightCheck formats a recap-bottom block for the wandering result.
func renderNightCheck(nc NightCheck) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🌙 **Night phase — %s**\n", outcomeLabel(nc.Outcome)))
	if nc.Summary != "" {
		b.WriteString(nc.Summary + "\n")
	}
	b.WriteString(fmt.Sprintf("_d20=%d, total=%d (threat%+d, camp%+d, class%+d)_\n",
		nc.Roll, nc.Total, nc.ThreatMod, nc.CampMod, nc.ClassMod))
	if nc.Flavor != "" {
		b.WriteString("\n" + nc.Flavor + "\n")
	}
	if nc.Outcome == NightOutcomeMinor || nc.Outcome == NightOutcomeStandard ||
		nc.Outcome == NightOutcomeElite || nc.Outcome == NightOutcomeAmbush {
		b.WriteString("\n_Encounter pending — resolves at your next `!advance`._\n")
	}
	return b.String()
}

// processNightCheck applies side-effects: log entry, threat bump for
// signs-of-passage, persist NightEvents on the camp.
func processNightCheck(e *Expedition, nc NightCheck) error {
	summary := nc.Summary
	if nc.MonsterName != "" {
		summary = fmt.Sprintf("%s [d20=%d, total=%d, mods: threat%+d camp%+d class%+d]",
			summary, nc.Roll, nc.Total, nc.ThreatMod, nc.CampMod, nc.ClassMod)
	} else {
		summary = fmt.Sprintf("%s [d20=%d, total=%d]", summary, nc.Roll, nc.Total)
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "night", summary, nc.Flavor); err != nil {
		return err
	}
	if nc.ThreatBumped {
		_ = applyThreatDelta(e.ID, 2, "signs of passage near camp")
	}
	if e.Camp != nil {
		e.Camp.NightEvents = append(e.Camp.NightEvents, summary)
		if err := updateCamp(e.ID, e.Camp); err != nil {
			return err
		}
	}
	return nil
}
