package plugin

import "math/rand/v2"

// ── Babysitting Service Flavor Pools ────────────────────────────────────────

// babysitConfirmLines are shown on purchase confirmation.
var babysitConfirmLines = []string{
	"Your adventurer is now in capable hands. Relatively speaking. We've done this before.",
	"Service begins immediately. Go do whatever it is you do when you're not doing this.",
	"Payment received. Your adventurer will be fine. Probably.",
	"We've looked at your skill levels. We know what needs work. We'll handle it. You handle whatever you're handling.",
}

// babysitRivalRefusalLines are used when a rival showed up during babysitting.
// All lines use exactly two %s: (rival display name, date string).
var babysitRivalRefusalLines = []string{
	"%s came by on %s with what appeared to be prepared remarks. They were turned away at the door. Their remarks went undelivered. This is probably for the best.",
	"%s attempted to initiate a duel on %s. The babysitter informed them this was not possible. They stood there for a moment. Then left.",
	"%s showed up on %s. The babysitter made them regret it in ways you could only dream of doing.",
}

// babysitDiaperLines appear once per summary, rotated.
var babysitDiaperLines = []string{
	"The diapers have been handled. We don't discuss the diapers.",
	"Diaper situation: resolved. No further details will be provided.",
	"Standard diaper protocols were followed. The adventurer was cooperative. Mostly.",
	"The diapers. They happened. They were handled. Moving on.",
}

// babysitHighlightLines fire on auto-babysit days when the haul was unusually
// good — making the babysitter feel like a companion rather than a silent
// insurance product. Each line takes one %s slot for the focused skill name.
var babysitHighlightLines = []string{
	"The babysitter says your kid was a natural at %s today. They want it noted.",
	"Solid day on the %s circuit, apparently. The babysitter looked smug about it.",
	"Whatever the babysitter did with your %s gear — it worked. The numbers don't lie.",
	"The babysitter handed in a %s haul and said nothing. The smug silence said plenty.",
	"Your kid is, against the odds, getting alarmingly good at %s under TwinBee's care.",
}

// pickBabysitFlavor returns a random entry from the given pool.
func pickBabysitFlavor(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}
