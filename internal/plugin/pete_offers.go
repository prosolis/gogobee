package plugin

// The offer half of the web action queue: what a signed-in owner is allowed to
// ask for, and what it costs.
//
// W5a's two verbs needed none of this — "pull out" and "take your bout" have no
// arguments and no price. W5b's three do, and the page cannot invent either: the
// zone list is level-gated and postgame-gated per player, and every price scales
// with level. So gogobee quotes them here, on the self-detail push that already
// carries the owner's private panels, and Pete renders the quote without doing
// any arithmetic of its own.
//
// A quote is NOT a permission. It is up to two minutes stale by the time anybody
// clicks it, so every one of these is re-resolved against the game's own tables
// when the order lands (performExpeditionStart re-runs availableZonesFor,
// performBabysitPurchase re-reads the level). What the offer list buys is a page
// that does not show a button which is certain to be refused.

import (
	"time"

	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

// advLoadoutKeys is the order the three presets are offered in — cheapest first,
// which is also how renderLoadoutPrompt lists them in Matrix.
var advLoadoutKeys = []SupplyLoadout{LoadoutLean, LoadoutBalanced, LoadoutHeavy}

// loadoutOffersFor prices the three supply presets at a tier. Days is the
// provisions estimate at that tier's calm daily burn — the same number
// `!expedition start` prints, and deliberately the pessimistic one: the holiday
// and Omen freebie packs are added at departure, so a run can outlast its quote
// but never fall short of it.
func loadoutOffersFor(tier ZoneTier) []peteclient.LoadoutOffer {
	out := make([]peteclient.LoadoutOffer, 0, len(advLoadoutKeys))
	for _, l := range advLoadoutKeys {
		pp := loadoutPurchase(tier, l)
		sup := makeSupplies(tier, pp)
		out = append(out, peteclient.LoadoutOffer{
			Key:   loadoutName(l),
			Name:  loadoutName(l),
			Blurb: loadoutBlurb(l),
			Cost:  pp.Cost(),
			Days:  estimateDays(sup.Max, sup.DailyBurn),
		})
	}
	return out
}

// zoneOffersFor is where this adventurer may set out for right now, priced.
//
// It returns nothing at all when they cannot leave — already on an expedition,
// seated in somebody else's, or mid zone-run. That is not a second permission
// check duplicating performExpeditionStart's; it is what stops the page offering
// a departure it can already tell will be refused.
func zoneOffersFor(uid id.UserID) []peteclient.ZoneOffer {
	if seated, _ := seatedExpeditionFor(uid); seated != nil {
		return nil
	}
	if existing, _ := getActiveExpedition(uid); existing != nil {
		return nil
	}
	if run, _ := getActiveZoneRun(uid); run != nil {
		return nil
	}
	zones := availableZonesFor(uid, dndLevelForUser(uid))
	out := make([]peteclient.ZoneOffer, 0, len(zones))
	for _, z := range zones {
		out = append(out, peteclient.ZoneOffer{
			ID:       string(z.ID),
			Display:  z.Display,
			Tier:     int(z.Tier),
			Hook:     z.Hook,
			Postgame: z.Tier == ZoneTierMythic,
			Loadouts: loadoutOffersFor(z.Tier),
		})
	}
	return out
}

// resumeOfferFor is the extracted expedition still waiting to be walked back
// into. Nil when there is none, when the window has already lapsed, or when the
// player is out again — a lapsed row is left for the sweeper to reap rather than
// reaped here, because a push builder should not be quietly ending expeditions.
func resumeOfferFor(uid id.UserID) *peteclient.ResumeOffer {
	if existing, _ := getActiveExpedition(uid); existing != nil {
		return nil
	}
	exp, err := getResumableExpedition(uid)
	if err != nil || exp == nil {
		return nil
	}
	if extractionLapsed(exp, time.Now().UTC()) {
		return nil
	}
	zone, _ := getZone(exp.ZoneID)
	off := &peteclient.ResumeOffer{
		ZoneID:   string(exp.ZoneID),
		Display:  zone.Display,
		Tier:     int(zone.Tier),
		Day:      exp.CurrentDay,
		Loadouts: loadoutOffersFor(zone.Tier),
	}
	if exp.CompletedAt != nil {
		off.ExpiresAt = exp.CompletedAt.Add(extractResumeWindow).Unix()
	}
	return off
}

// babysitOfferFor is the sitter's standing and the two prices they charge. It is
// pushed even when a sitter is already engaged: "somebody is already looking
// after your pet until Tuesday" is exactly what the page should say instead of a
// buy button.
func babysitOfferFor(adv *AdventureCharacter) *peteclient.BabysitOffer {
	if adv == nil {
		return nil
	}
	daily := babysitDailyCost(dndLevelForUser(adv.UserID))
	off := &peteclient.BabysitOffer{
		Active:    adv.BabysitActive,
		WeekCost:  daily * 7,
		MonthCost: daily * 30,
	}
	if adv.BabysitActive && adv.BabysitExpiresAt != nil {
		off.ExpiresAt = adv.BabysitExpiresAt.Unix()
	}
	return off
}
