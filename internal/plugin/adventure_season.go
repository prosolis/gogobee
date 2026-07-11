package plugin

import (
	"time"
)

// N7/E4 — Seasonal events (gogobee_engagement_plan.md §E4).
//
// A season is a 1-week skin anchored to a real holiday. Unlike isHolidayToday()
// (a single UTC day, worth a double-action) and unlike the Omen (an ISO-week
// rotation), a season spans a window around its anchor date and layers three
// things on top of the world for that week:
//
//   1. A themed world modifier — a reskinned Omen that OVERRIDES the weekly
//      rotation (see activeOmen). It reuses the omen effect fields, so the same
//      non-combat rule holds: a season never touches SimulateCombat or the turn
//      engine. That is enforced structurally — activeSeason() honours
//      simOmenDisabled exactly as activeOmen() does, so the balance sim never
//      sees a season through any path, and the season Omen only reaches the sim's
//      omen seams behind activeOmen's own simOmenDisabled guard.
//   2. A limited-time curio shelf at Luigi's — a curated selection of existing
//      registry items (dailyCuriosStock). No new power enters the game; the shelf
//      just changes which items rotate in for the week.
//   3. A themed visitor on the road — a season-gated ambient event that leaves a
//      small keepsake and a coin gift (expedition_ambient.go). No combat: the
//      ambient seam has never opened a fight and this does not change that.
//
// A season is a pure function of the UTC date and the anchor calendar, so it
// needs no schema, no ticker state, and no persistence — the same discipline as
// the Omen and the holiday calendar.

// seasonHalfWidth is the number of days on each side of the anchor date that the
// season is live. 3 → a 7-day window (anchor ± 3).
const seasonHalfWidth = 3

// seasonVisitor is the themed ambient event a season adds to the road. It never
// resolves combat — it leaves a sellable keepsake and a small coin gift.
type seasonVisitor struct {
	Weight        int      // relative weight in the ambient pick
	FlavorPool    []string // scene narration lines for the visit
	Keepsake      string   // sellable trophy left behind
	KeepsakeValue int64    // coin baseline of the keepsake
	Coins         int      // flat coin gift
}

// season is one holiday skin.
type season struct {
	Key      string                   // stable id (tests, logs)
	Name     string                   // player-facing, e.g. "Hallowtide"
	Emoji    string                   // banner emoji
	Anchor   func(year int) time.Time // the anchor date for a given year (UTC)
	Blurb    string                   // one-line "what's live this week" banner copy
	Omen     omen                     // themed override omen (non-combat fields only)
	CurioIDs []string                 // curated registry IDs for Luigi's shelf
	Visitor  seasonVisitor            // the road visitor
}

// fixedAnchor builds an Anchor for a fixed month/day holiday.
func fixedAnchor(month time.Month, day int) func(int) time.Time {
	return func(year int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
}

// seasonTable is the set of anchor holidays that get a skin. Order is match
// order; the windows are disjoint so it never matters, but keep them disjoint.
var seasonTable = []season{
	{
		Key: "hallowtide", Name: "Hallowtide", Emoji: "🎃",
		Anchor: fixedAnchor(time.October, 31),
		Blurb:  "the veil's thin — spooky curios at Luigi's and things going bump on the road, all week",
		Omen: omen{
			Key: "hallowtide", Name: "Hallowtide",
			TwinBee:              "The veil's gone thin for Hallowtide — reagents and oddments are spilling through from somewhere I'd rather not name. I'm grabbing them while they're here.",
			ConsumableChanceMult: 2.0,
		},
		CurioIDs: []string{
			"cloak_of_arachnida", "demon_armor", "goggles_of_night",
			"slippers_of_spider_climbing", "staff_of_swarming_insects",
			"sword_of_life_stealing", "dagger_of_venom", "cloak_of_the_bat",
		},
		Visitor: seasonVisitor{
			Weight: 12,
			FlavorPool: []string{
				"A gourd-headed thing shuffles out of the dark, sets a little carved lantern on your bedroll, and shuffles right back into it.",
				"Something small and many-legged skitters past, drops a trinket at your feet as if in tribute, and is gone before you can look twice.",
				"A cold draft carries a whisper and a gift — a carved gourd, still faintly warm, left where you'll find it come morning.",
			},
			Keepsake: "Carved Gourd Lantern", KeepsakeValue: 60, Coins: 8,
		},
	},
	{
		Key: "midwinter", Name: "Midwinter Feast", Emoji: "❄️",
		Anchor: fixedAnchor(time.December, 25),
		Blurb:  "gifts by every door — a free pack for anyone heading out, warm curios at Luigi's, all week",
		Omen: omen{
			Key: "midwinter", Name: "Midwinter Feast",
			TwinBee:            "It's Midwinter, and someone's been leaving gifts by the outfitter's door. There's a free pack in it for anyone heading out — and I set off in high spirits.",
			SupplyFreebiePacks: 1,
			StartMoodBonus:     5,
		},
		CurioIDs: []string{
			"boots_of_the_winterlands", "frost_brand", "ring_of_warmth",
			"staff_of_frost", "potion_of_healing", "staff_of_healing",
		},
		Visitor: seasonVisitor{
			Weight: 12,
			FlavorPool: []string{
				"A bundled figure passes your camp without a word, leaves a frost-glass bauble hanging where the firelight catches it, and trudges on into the snow.",
				"You wake to find your pack a little heavier — a wrapped bauble and a handful of coin, and a single set of bootprints leading away.",
				"Bells, faint and far off. By the time they fade there's a bauble on your bedroll that wasn't there before.",
			},
			Keepsake: "Frost-Glass Bauble", KeepsakeValue: 60, Coins: 10,
		},
	},
	{
		Key: "sweethearts", Name: "Sweethearts' Revel", Emoji: "💗",
		Anchor: fixedAnchor(time.February, 14),
		Blurb:  "the arena crowd's throwing coin — fat purses, charming curios at Luigi's, all week",
		Omen: omen{
			Key: "sweethearts", Name: "Sweethearts' Revel",
			TwinBee:         "The whole town's giddy for Sweethearts' week — the arena crowd's throwing coin like confetti. Win pretty and the purse pays twenty over the odds.",
			ArenaPayoutMult: 1.20,
		},
		CurioIDs: []string{
			"eyes_of_charming", "philter_of_love", "staff_of_charming",
			"luck_blade", "stone_of_good_luck_luckstone", "pearl_of_power",
			"glamoured_studded_leather",
		},
		Visitor: seasonVisitor{
			Weight: 12,
			FlavorPool: []string{
				"A courier in festival colours finds you even out here, presses a ribbon-wrapped token into your hand with a wink, and hurries back the way they came.",
				"A paper heart, pinned to a token and left on your pack — 'from an admirer,' it says, and nothing else.",
				"Someone's left a ribbon-wrapped keepsake by the trail marker, addressed to no one and everyone. You pocket it.",
			},
			Keepsake: "Ribbon-Wrapped Token", KeepsakeValue: 55, Coins: 10,
		},
	},
	{
		Key: "first_bloom", Name: "First Bloom", Emoji: "🌷",
		Anchor: func(year int) time.Time { return easterDate(year) },
		Blurb:  "everything's growing eager — fuller harvests, green curios at Luigi's, all week",
		Omen: omen{
			Key: "first_bloom", Name: "First Bloom",
			TwinBee:           "First Bloom's on us — everything's growing twice as eager and the woods feel calm with it. Every gather comes up fuller, and the dread's slow to rise.",
			HarvestYieldBonus: 1,
			ThreatDriftReduce: 1,
		},
		CurioIDs: []string{
			"bag_of_beans", "potion_of_growth", "potion_of_animal_friendship",
			"ring_of_animal_influence", "horn_of_valhalla", "sword_of_life_stealing",
		},
		Visitor: seasonVisitor{
			Weight: 12,
			FlavorPool: []string{
				"A hare the size of a hound lopes up, regards you with unsettling calm, and leaves a pressed blossom on the ground before bounding off.",
				"New vines have crept over your gear in the night — and tucked among them, a perfect pressed blossom and a little coin, as if the woods were paying rent.",
				"The undergrowth parts on its own, sets a spring blossom at your feet, and closes again. You decide not to question it.",
			},
			Keepsake: "Pressed Spring Blossom", KeepsakeValue: 50, Coins: 8,
		},
	},
}

// activeSeason returns the season whose window contains today (UTC), or false.
// Honours simOmenDisabled so the balance sim never observes a season through any
// path — the same neutralisation activeOmen applies to the weekly rotation.
func activeSeason() (season, bool) {
	if simOmenDisabled {
		return season{}, false
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return seasonForDate(today)
}

// seasonForDate is the pure core of activeSeason, split out for tests. The
// anchor is checked against the target year and its neighbours so a window that
// straddles a year boundary still resolves.
func seasonForDate(today time.Time) (season, bool) {
	for _, s := range seasonTable {
		for _, yr := range []int{today.Year() - 1, today.Year(), today.Year() + 1} {
			a := s.Anchor(yr)
			lo := a.AddDate(0, 0, -seasonHalfWidth)
			hi := a.AddDate(0, 0, seasonHalfWidth)
			if !today.Before(lo) && !today.After(hi) {
				return s, true
			}
		}
	}
	return season{}, false
}

// seasonBannerLine is the "what's live this week" one-liner surfaced under the
// Omen line in the morning DM. Empty when no season is active.
func seasonBannerLine(s season) string {
	return s.Emoji + " **" + s.Name + "** is here — " + s.Blurb + "."
}
