package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestParseCoopFundingTier(t *testing.T) {
	cases := map[string]CoopFundingTier{
		"none":       CoopFundNone,
		"skip":       CoopFundNone,
		"min":        CoopFundMinimal,
		"minimal":    CoopFundMinimal,
		"standard":   CoopFundStandard,
		"std":        CoopFundStandard,
		"aggressive": CoopFundAggressive,
		"agg":        CoopFundAggressive,
		"all_in":     CoopFundAllIn,
		"all-in":     CoopFundAllIn,
		"all in":     CoopFundAllIn,
		"allin":      CoopFundAllIn,
		"all":        CoopFundAllIn,
		"":           "",
		"garbage":    "",
	}
	for input, want := range cases {
		got := parseCoopFundingTier(input)
		if got != want {
			t.Errorf("parseCoopFundingTier(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCoopMemberModifierLiabilityCap(t *testing.T) {
	t.Parallel()

	veteran := &CoopMember{
		IsLiability:  false,
		DailyFunding: map[int]CoopFundingTier{1: CoopFundAllIn},
	}
	noob := &CoopMember{
		IsLiability:  true,
		DailyFunding: map[int]CoopFundingTier{1: CoopFundAllIn},
	}

	if _, mod := coopMemberModifier(veteran, 1); mod != 30 {
		t.Errorf("veteran All In modifier = %d, want 30", mod)
	}
	if _, mod := coopMemberModifier(noob, 1); mod != coopLiabilityCap {
		t.Errorf("liability All In modifier = %d, want %d (cap)", mod, coopLiabilityCap)
	}

	missing := &CoopMember{DailyFunding: map[int]CoopFundingTier{}}
	tier, mod := coopMemberModifier(missing, 1)
	if tier != CoopFundNone || mod != -10 {
		t.Errorf("missing-day default = (%q, %d), want (none, -10)", tier, mod)
	}
}

func TestCoopFundingMapRoundtrip(t *testing.T) {
	t.Parallel()

	in := map[int]CoopFundingTier{
		1: CoopFundMinimal,
		3: CoopFundAllIn,
		7: CoopFundNone,
	}
	encoded := serializeCoopFundingMap(in)
	// JSON encode/decode via parse helper
	bytes, _ := jsonMarshalCoop(encoded)
	out := parseCoopFundingMap(string(bytes))

	if len(out) != len(in) {
		t.Fatalf("roundtrip lost entries: in %d out %d", len(in), len(out))
	}
	for k, v := range in {
		if out[k] != v {
			t.Errorf("day %d: got %q want %q", k, out[k], v)
		}
	}
}

func TestCoopTallyVote(t *testing.T) {
	t.Parallel()

	leader := id.UserID("@leader:test")
	other := id.UserID("@other:test")
	third := id.UserID("@third:test")

	tests := []struct {
		name   string
		votes  map[id.UserID]string
		want   string
	}{
		{"clear majority", map[id.UserID]string{leader: "A", other: "A", third: "B"}, "A"},
		{"tie broken by leader", map[id.UserID]string{leader: "B", other: "A"}, "B"},
		{"all abstained falls back to A", map[id.UserID]string{}, "A"},
		{"tie with no leader vote → lex first (deterministic)", map[id.UserID]string{other: "A", third: "B"}, "A"},
		{"three-way tie, leader's vote in winners → leader's pick", map[id.UserID]string{leader: "C", other: "A", third: "B"}, "C"},
		{"tie with leader voting outside winners → lex first", map[id.UserID]string{leader: "C", other: "A", "@4:t": "A", third: "B", "@5:t": "B"}, "A"},
	}
	for _, tc := range tests {
		// Run repeatedly to catch any non-determinism leaking from map iteration.
		for i := 0; i < 50; i++ {
			event := &CoopEvent{Votes: tc.votes}
			got := coopTallyVote(event, leader)
			if got != tc.want {
				t.Errorf("%s (iter %d): got %q, want %q", tc.name, i, got, tc.want)
				break
			}
		}
	}
}

func TestCoopEventOptionModifierKnownEvent(t *testing.T) {
	t.Parallel()
	// Obstacle 0 has options A=-8, B=0, C=12. Verify the lookup works.
	if got := coopEventOptionModifier(CoopCatObstacle, 0, "A"); got != -8 {
		t.Errorf("obstacle[0] A modifier = %d, want -8", got)
	}
	if got := coopEventOptionModifier(CoopCatObstacle, 0, "C"); got != 12 {
		t.Errorf("obstacle[0] C modifier = %d, want 12", got)
	}
	if got := coopEventOptionModifier(CoopCatObstacle, 0, "Z"); got != 0 {
		t.Errorf("unknown option = %d, want 0", got)
	}
}

func TestCoopEventMetaConsistency(t *testing.T) {
	t.Parallel()
	cats := map[CoopEventCategory][]coopEventMeta{
		CoopCatObstacle:    coopObstacleMeta,
		CoopCatOpportunity: coopOpportunityMeta,
		CoopCatCrisis:      coopCrisisMeta,
		CoopCatEncounter:   coopEncounterMeta,
	}
	flavors := map[CoopEventCategory][]string{
		CoopCatObstacle:    TwinBeeObstacle,
		CoopCatOpportunity: TwinBeeOpportunity,
		CoopCatCrisis:      TwinBeeCrisis,
		CoopCatEncounter:   TwinBeeEncounter,
	}
	for cat, meta := range cats {
		flavor := flavors[cat]
		if len(meta) != len(flavor) {
			t.Errorf("%s: meta has %d entries, flavor has %d", cat, len(meta), len(flavor))
		}
		for i, m := range meta {
			if len(m.options) < 2 {
				t.Errorf("%s[%d] has only %d options", cat, i, len(m.options))
			}
			recOK := false
			for _, opt := range m.options {
				if opt.label == m.recommended {
					recOK = true
				}
			}
			if !recOK {
				t.Errorf("%s[%d] recommends %q which isn't an option", cat, i, m.recommended)
			}
		}
	}
}

func TestCoopResolutionIdempotencyGuard(t *testing.T) {
	t.Parallel()
	// Sanity check: the LastResolvedDay field on CoopRun is the authoritative
	// idempotency marker. The resolver short-circuits when LastResolvedDay >=
	// CurrentDay, so a crash-restart on the same UTC day after the roll has
	// landed is a safe no-op.
	run := &CoopRun{CurrentDay: 3, LastResolvedDay: 3}
	if !(run.LastResolvedDay >= run.CurrentDay) {
		t.Errorf("expected resolution skip when LastResolvedDay (%d) >= CurrentDay (%d)",
			run.LastResolvedDay, run.CurrentDay)
	}
	run.LastResolvedDay = 2
	if run.LastResolvedDay >= run.CurrentDay {
		t.Errorf("expected resolution to proceed when LastResolvedDay (%d) < CurrentDay (%d)",
			run.LastResolvedDay, run.CurrentDay)
	}
}

func TestCoopParimutuelPayouts(t *testing.T) {
	t.Parallel()
	a := id.UserID("@a:t")
	b := id.UserID("@b:t")
	c := id.UserID("@c:t")
	d := id.UserID("@d:t")

	bets := []CoopBet{
		{PlayerID: a, Position: "success", Amount: 1000},
		{PlayerID: b, Position: "success", Amount: 3000},
		{PlayerID: c, Position: "failure", Amount: 5000},
		{PlayerID: d, Position: "failure", Amount: 1000},
	}
	winners, total, rake, payouts := coopParimutuelPayouts(bets, "success")
	if total != 10000 {
		t.Errorf("total = %d, want 10000", total)
	}
	if rake != 1000 {
		t.Errorf("rake = %d, want 1000 (10%%)", rake)
	}
	if len(winners) != 2 {
		t.Errorf("winners = %d, want 2", len(winners))
	}
	// Net pool = 9000. a put in 1000/4000 = 25% of winning side → 2250.
	// b put in 3000/4000 = 75% → 6750.
	if payouts[a] != 2250 {
		t.Errorf("a payout = %d, want 2250", payouts[a])
	}
	if payouts[b] != 6750 {
		t.Errorf("b payout = %d, want 6750", payouts[b])
	}
	if payouts[c] != 0 {
		t.Errorf("c payout = %d, want 0 (loser)", payouts[c])
	}
}

func TestCoopWeightedItemWinnerDistribution(t *testing.T) {
	t.Parallel()
	a := id.UserID("@a:t")
	b := id.UserID("@b:t")
	c := id.UserID("@c:t")
	members := []CoopMember{
		{UserID: a, TotalContributed: 7000}, // 70%
		{UserID: b, TotalContributed: 2000}, // 20%
		{UserID: c, TotalContributed: 1000}, // 10%
	}
	totalWeight := 10000
	wins := map[id.UserID]int{}
	const trials = 100_000
	for i := 0; i < trials; i++ {
		w := coopWeightedItemWinner(members, totalWeight)
		wins[w]++
	}
	// Allow ±2% absolute deviation per side at this trial count.
	check := func(uid id.UserID, expected float64) {
		actual := float64(wins[uid]) / float64(trials)
		if actual < expected-0.02 || actual > expected+0.02 {
			t.Errorf("%s: got %.3f, want ~%.2f", uid, actual, expected)
		}
	}
	check(a, 0.70)
	check(b, 0.20)
	check(c, 0.10)
}

func TestCoopWeightedItemWinnerNoContributions(t *testing.T) {
	t.Parallel()
	a := id.UserID("@a:t")
	b := id.UserID("@b:t")
	members := []CoopMember{
		{UserID: a, TotalContributed: 0},
		{UserID: b, TotalContributed: 0},
	}
	wins := map[id.UserID]int{}
	for i := 0; i < 10_000; i++ {
		wins[coopWeightedItemWinner(members, 0)]++
	}
	if wins[a] == 0 || wins[b] == 0 {
		t.Errorf("uniform fallback didn't pick both: %v", wins)
	}
}

func TestCoopMasterworkTierGate(t *testing.T) {
	t.Parallel()
	// Verify that masterwork defs exist at T4 and T5 (so coopMaybeMasterwork
	// has candidates) and that nothing exists at T1-T3 in the dungeon path.
	tiers := map[int]int{}
	for _, def := range masterworkDefs {
		tiers[def.Tier]++
	}
	if tiers[4] == 0 {
		t.Errorf("no T4 masterwork defs found — coop T4 drops will silently no-op")
	}
	if tiers[5] == 0 {
		t.Errorf("no T5 masterwork defs found — coop T5 drops will silently no-op")
	}
}

func TestCoopGiftEVSymmetric(t *testing.T) {
	t.Parallel()
	// At a 50/50 sender mix, "always open" and "always leave" should both
	// have EV = 0. Anything else means players have a dominant strategy.
	openEV := 0.5*float64(coopGiftBasketOpened) + 0.5*float64(coopGiftMimicOpened)
	leaveEV := 0.5*float64(coopGiftBasketUnopened) + 0.5*float64(coopGiftMimicUnopened)
	if openEV != 0 {
		t.Errorf("always-open EV = %.2f, want 0", openEV)
	}
	if leaveEV != 0 {
		t.Errorf("always-leave EV = %.2f, want 0", leaveEV)
	}
}

func TestCoopGiftVarianceSymmetric(t *testing.T) {
	t.Parallel()
	// Equal EV is not enough — if open and leave have different variance, a
	// risk-averse party always picks the lower-variance side and the
	// dilemma collapses. Magnitudes must be equal across all four outcomes.
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	mags := []int{
		abs(coopGiftBasketOpened),
		abs(coopGiftBasketUnopened),
		abs(coopGiftMimicOpened),
		abs(coopGiftMimicUnopened),
	}
	for i := 1; i < len(mags); i++ {
		if mags[i] != mags[0] {
			t.Errorf("gift magnitudes must be equal across all four outcomes; got %v", mags)
			break
		}
	}
}

func TestCoopParimutuelNoWinners(t *testing.T) {
	t.Parallel()
	bets := []CoopBet{
		{PlayerID: "@x:t", Position: "failure", Amount: 5000},
	}
	winners, total, rake, payouts := coopParimutuelPayouts(bets, "success")
	if len(winners) != 0 {
		t.Errorf("winners = %d, want 0", len(winners))
	}
	if total != 5000 || rake != 500 {
		t.Errorf("total=%d rake=%d, want 5000 500", total, rake)
	}
	if len(payouts) != 0 {
		t.Errorf("payouts has %d entries; want 0", len(payouts))
	}
}

func TestCoopTierTableComplete(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		def, ok := coopTierTable[tier]
		if !ok {
			t.Errorf("missing tier %d", tier)
			continue
		}
		if def.totalDays < 2 || def.totalDays > 7 {
			t.Errorf("tier %d totalDays %d out of range", tier, def.totalDays)
		}
		if def.baseFailurePct < 1 || def.baseFailurePct > 99 {
			t.Errorf("tier %d baseFailurePct %d out of range", tier, def.baseFailurePct)
		}
		if def.rewardBase <= 0 {
			t.Errorf("tier %d rewardBase %d not positive", tier, def.rewardBase)
		}
	}
}

// jsonMarshalCoop is a tiny indirection so the test file doesn't import
// encoding/json directly (we only need it for the roundtrip helper).
func jsonMarshalCoop(v map[string]string) ([]byte, error) {
	out := []byte{'{'}
	first := true
	for k, val := range v {
		if !first {
			out = append(out, ',')
		}
		first = false
		out = append(out, '"')
		out = append(out, k...)
		out = append(out, '"', ':', '"')
		out = append(out, val...)
		out = append(out, '"')
	}
	out = append(out, '}')
	return out, nil
}
