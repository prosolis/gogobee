package plugin

import (
	"math/rand/v2"
	"strings"
)

// Metadata for each TwinBee floor event. Encodes the per-option success
// modifiers and TwinBee's recommendation. Authored by reading the flavor
// entries — TwinBee's "wrong detail" hint usually points to which option is
// actually correct.
//
// Modifier ranges: −15 (clearly bad) to +12 (clearly good). Sum across
// options is roughly net-zero so randomly-voting parties don't trend +EV.

type CoopEventCategory string

const (
	CoopCatObstacle    CoopEventCategory = "obstacle"
	CoopCatOpportunity CoopEventCategory = "opportunity"
	CoopCatCrisis      CoopEventCategory = "crisis"
	CoopCatEncounter   CoopEventCategory = "encounter"
)

type coopEventOption struct {
	label    string
	modifier int
}

type coopEventMeta struct {
	options     []coopEventOption
	recommended string // A/B/C — TwinBee's pick
}

// One entry per event in the corresponding flavor pool, in the same order.
// Keep these arrays in sync with coop_flavor_twinbee.go.

var coopObstacleMeta = []coopEventMeta{
	// 1. Cave-in: "definitely the left side" — too emphatic; clearing properly is right.
	{options: []coopEventOption{{"A", -8}, {"B", 0}, {"C", 12}}, recommended: "A"},
	// 2. Locked gate: "third tumbler might be a pressure plate" — A triggers it.
	{options: []coopEventOption{{"A", -12}, {"B", -3}, {"C", 10}}, recommended: "A"},
	// 3. Other party with axe: "axe is just decoration" — almost certainly not.
	{options: []coopEventOption{{"A", -15}, {"B", 8}, {"C", -5}}, recommended: "A"},
	// 4. Flooded section: "torches stopped twenty meters back" — water deeper than claimed.
	{options: []coopEventOption{{"A", -10}, {"B", 0}, {"C", 10}}, recommended: "A"},
	// 5. Collapsed bridge: "chains might be part of a trap" — they are.
	{options: []coopEventOption{{"A", -12}, {"B", 0}, {"C", 8}}, recommended: "A"},
}

var coopOpportunityMeta = []coopEventMeta{
	// 1. Vault: "scratches around the lock from previous attempts" — trapped.
	{options: []coopEventOption{{"A", -12}, {"B", 3}}, recommended: "A"},
	// 2. Side chamber: "shapes seem to be breathing" — they are. Mimics or worse.
	{options: []coopEventOption{{"A", -15}, {"B", 4}}, recommended: "A"},
	// 3. Wounded enemy with full pack: "one eye open, just how they sleep" — ambush.
	{options: []coopEventOption{{"A", -10}, {"B", 2}}, recommended: "A"},
	// 4. Unmarked door: "good feeling, ignore the smell" — TwinBee's right this time.
	{options: []coopEventOption{{"A", 10}, {"B", -3}}, recommended: "A"},
	// 5. Merchant Thom: usually fine to browse, sometimes a trap. Slight positive.
	{options: []coopEventOption{{"A", 6}, {"B", 0}}, recommended: "A"},
}

var coopCrisisMeta = []coopEventMeta{
	// 1. Trap, "left lever, definitely not the right one" — C (find another way) safest.
	{options: []coopEventOption{{"A", -10}, {"B", 5}, {"C", 8}}, recommended: "A"},
	// 2. Equipment corrosion: "load-bearing thing might be nothing" — pay to address.
	{options: []coopEventOption{{"A", 8}, {"B", -10}, {"C", 0}}, recommended: "A"},
	// 3. Separated party member, guard-room hint: TwinBee actually right that they went left;
	//    Option C ("wait here") is fine too — cheaper and works.
	{options: []coopEventOption{{"A", 4}, {"B", -2}, {"C", 6}}, recommended: "A"},
	// 4. Something following, "consistent distance is reassuring" — predator pacing.
	//    Pay the deterrent.
	{options: []coopEventOption{{"A", -8}, {"B", 8}, {"C", 0}}, recommended: "A"},
	// 5. Cave-in with "active" ceiling: pay to shore up.
	{options: []coopEventOption{{"A", -10}, {"B", 10}, {"C", -2}}, recommended: "A"},
}

var coopEncounterMeta = []coopEventMeta{
	// 1. Guardian, "twelve seconds two out of three times" — timing window unreliable.
	{options: []coopEventOption{{"A", 4}, {"B", 6}, {"C", -10}}, recommended: "C"},
	// 2. Caged person, "alarm is decorative" — usually not. Trap, but freeing them is right ~half the time.
	{options: []coopEventOption{{"A", 2}, {"B", -3}, {"C", 5}}, recommended: "A"},
	// 3. Sleeping guard, second guard reflected in shiny — there's a second guard.
	{options: []coopEventOption{{"A", -10}, {"B", 0}, {"C", 6}}, recommended: "A"},
	// 4. Recurring merchant Thom — keep moving avoids whatever shenanigans.
	{options: []coopEventOption{{"A", 2}, {"B", -5}, {"C", 4}}, recommended: "A"},
	// 5. Unidentifiable thing — backing away is the safe call.
	{options: []coopEventOption{{"A", -8}, {"B", -2}, {"C", 8}}, recommended: "C"},
}

func coopEventCategoryMeta(cat CoopEventCategory) []coopEventMeta {
	switch cat {
	case CoopCatObstacle:
		return coopObstacleMeta
	case CoopCatOpportunity:
		return coopOpportunityMeta
	case CoopCatCrisis:
		return coopCrisisMeta
	case CoopCatEncounter:
		return coopEncounterMeta
	}
	return nil
}

func coopEventCategoryFlavor(cat CoopEventCategory) []string {
	switch cat {
	case CoopCatObstacle:
		return TwinBeeObstacle
	case CoopCatOpportunity:
		return TwinBeeOpportunity
	case CoopCatCrisis:
		return TwinBeeCrisis
	case CoopCatEncounter:
		return TwinBeeEncounter
	}
	return nil
}

// pickCoopEventCategory weights categories by tier. Lower tiers favor obstacle
// and opportunity (lighter); higher tiers weight toward crisis and encounter.
func pickCoopEventCategory(tier int) CoopEventCategory {
	weights := map[int]map[CoopEventCategory]int{
		1: {CoopCatObstacle: 5, CoopCatOpportunity: 4, CoopCatCrisis: 1, CoopCatEncounter: 1},
		2: {CoopCatObstacle: 4, CoopCatOpportunity: 3, CoopCatCrisis: 2, CoopCatEncounter: 2},
		3: {CoopCatObstacle: 3, CoopCatOpportunity: 3, CoopCatCrisis: 3, CoopCatEncounter: 2},
		4: {CoopCatObstacle: 2, CoopCatOpportunity: 2, CoopCatCrisis: 4, CoopCatEncounter: 3},
		5: {CoopCatObstacle: 2, CoopCatOpportunity: 2, CoopCatCrisis: 4, CoopCatEncounter: 4},
	}
	w, ok := weights[tier]
	if !ok {
		w = weights[3]
	}
	total := 0
	for _, x := range w {
		total += x
	}
	r := rand.IntN(total)
	for cat, x := range w {
		if r < x {
			return cat
		}
		r -= x
	}
	return CoopCatObstacle
}

// pickCoopEvent returns a random event index for the category.
func pickCoopEvent(cat CoopEventCategory) int {
	pool := coopEventCategoryMeta(cat)
	if len(pool) == 0 {
		return 0
	}
	return rand.IntN(len(pool))
}

// optionLabels returns the label list for an event ("A","B" or "A","B","C").
func coopEventOptionLabels(cat CoopEventCategory, idx int) []string {
	pool := coopEventCategoryMeta(cat)
	if idx >= len(pool) {
		return []string{"A", "B"}
	}
	out := make([]string, 0, len(pool[idx].options))
	for _, opt := range pool[idx].options {
		out = append(out, opt.label)
	}
	return out
}

// optionModifier returns the success modifier for the chosen option, or 0 if
// the option is unknown.
func coopEventOptionModifier(cat CoopEventCategory, idx int, label string) int {
	pool := coopEventCategoryMeta(cat)
	if idx >= len(pool) {
		return 0
	}
	for _, opt := range pool[idx].options {
		if opt.label == label {
			return opt.modifier
		}
	}
	return 0
}

// recommendedOption returns TwinBee's pick for the event.
func coopEventRecommended(cat CoopEventCategory, idx int) string {
	pool := coopEventCategoryMeta(cat)
	if idx >= len(pool) {
		return "A"
	}
	return pool[idx].recommended
}

// validateCoopVote uppercases and validates a vote against the available options.
func validateCoopVote(cat CoopEventCategory, idx int, raw string) (string, bool) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	for _, label := range coopEventOptionLabels(cat, idx) {
		if v == label {
			return v, true
		}
	}
	return "", false
}
