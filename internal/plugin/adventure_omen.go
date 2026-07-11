package plugin

import (
	"time"
)

// N7/B3 — the Omen: one rotating world modifier per ISO week
// (gogobee_engagement_plan.md §B3).
//
// The active omen is a pure function of the ISO (year, week): omenTable indexed
// by (year*53 + week) % len, so it advances by exactly one entry each week and
// needs no schema, no ticker state, and no persistence. Every seam reads
// activeOmen() the same way isHolidayToday() is read, and a zero-valued effect
// field means "this omen doesn't touch that seam."
//
// Launch-set rule (§B3): every omen is a buff-with-texture on a NON-combat
// lever — harvest, supplies, expedition mood/threat, arena payout, ingredient
// drops. Nothing touches SimulateCombat or the turn engine: the omen is keyed
// on the real clock, so a combat-affecting omen would make the combat golden
// and the balance corpus week-dependent. The plan's "elites +2 ATK" mutator is
// deliberately omitted for exactly that reason.

type omen struct {
	Key     string // stable id (tests, logs)
	Name    string // player-facing name, e.g. "Bountiful Harvest"
	TwinBee string // first-person announce blurb (TwinBee voice)

	// Effect fields — zero == no effect at that seam.
	HarvestYieldBonus    int     // + units per harvest grant
	SupplyFreebiePacks   int     // + complimentary standard packs at outfitting
	StartMoodBonus       int     // + starting DM mood on a new expedition
	ArenaPayoutMult      float64 // >1 scales arena net earnings
	ConsumableChanceMult float64 // >1 scales the per-win ingredient drop chance
	ThreatDriftReduce    int     // subtract from the daily threat *rise* (floored at hold-steady)
}

// simOmenDisabled neutralizes the weekly Omen for the balance sim, so a corpus
// sweep's results never depend on which wall-clock week it was run in. Set true
// by NewSimRunner (mirrors simAutoArmEnabled). Every seam reads activeOmen(),
// which returns the no-effect omen while this is set.
var simOmenDisabled bool

// omenTable is the weekly rotation. Keep it a set of distinct non-combat levers;
// order is the rotation order. Adding an entry reshuffles the schedule but never
// breaks determinism (still a pure function of the week).
var omenTable = []omen{
	{
		Key: "bountiful_harvest", Name: "Bountiful Harvest",
		TwinBee:           "The land's feeling generous this week — every gather I make comes up with a little extra in hand.",
		HarvestYieldBonus: 1,
	},
	{
		Key: "quartermasters_blessing", Name: "Quartermaster's Blessing",
		TwinBee:            "Somebody left the storerooms unlocked. Outfitting an expedition this week? There's a free pack in it, and I'm setting out in good spirits.",
		SupplyFreebiePacks: 1,
		StartMoodBonus:     5,
	},
	{
		Key: "golden_purse", Name: "Golden Purse",
		TwinBee:         "The arena crowd's flush this week — purses are paying out fat. Twenty percent over the odds, if you can win it.",
		ArenaPayoutMult: 1.20,
	},
	{
		Key: "overflowing_satchels", Name: "Overflowing Satchels",
		TwinBee:              "Reagents are turning up everywhere I look — twice as often as usual. Good week to stock the crafting shelf.",
		ConsumableChanceMult: 2.0,
	},
	{
		Key: "still_waters", Name: "Still Waters",
		TwinBee:           "It's quiet out there. Whatever's hunting us is slow to rouse this week — the daily dread holds steady instead of creeping up.",
		ThreatDriftReduce: 1,
	},
}

// omenForWeek returns the omen for an ISO (year, week). Pure and total.
func omenForWeek(year, week int) omen {
	idx := ((year*53)+week)%len(omenTable) + len(omenTable)
	return omenTable[idx%len(omenTable)]
}

// activeOmen returns this week's omen (UTC ISO week), or a no-effect omen when
// the balance sim has disabled it.
func activeOmen() omen {
	if simOmenDisabled {
		return omen{Key: "none", Name: "None"}
	}
	// N7/E4 — a live season's themed omen overrides the weekly rotation. Kept
	// behind the simOmenDisabled guard above so the balance sim never sees it.
	if s, ok := activeSeason(); ok {
		return s.Omen
	}
	y, w := time.Now().UTC().ISOWeek()
	return omenForWeek(y, w)
}

// omenMorningLine is the compact one-liner surfaced in the morning DM (the §B3
// announce seam). Empty is never returned — an omen is always active — but the
// caller may still choose when to show it.
func omenMorningLine(o omen) string {
	return "🔮 **The Omen — " + o.Name + ".** _" + o.TwinBee + "_"
}
