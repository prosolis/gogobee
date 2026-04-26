package plugin

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// Monte Carlo balance analysis for co-op dungeons.
//
// Run with:  go test ./internal/plugin -run TestCoopBalanceReport -v
// Skipped by default (requires -run filter to invoke); pure-function, no DB.

const (
	balanceTrials = 50_000
	balanceTax    = 0.05 // matches coopAdventureRake
)

type fundingPlan struct {
	name string
	// per-player tiers; length = party size
	tiers []CoopFundingTier
	// optional fixed per-player level bonus (mimics a profile of avg or
	// veteran party). 0 = at-minimum (default), positive = stronger party.
	levelBonusEach int
	petBonusEach   int
}

func TestCoopBalanceReport(t *testing.T) {
	if testing.Short() {
		t.Skip("balance report skipped in short mode")
	}
	t.Parallel()

	rng := rand.New(rand.NewPCG(42, 42))

	profiles := []struct {
		label       string
		levelBonus  int
		petBonus    int
	}{
		{"at-minimum (no pets)", 0, 0},
		{"average (level+5, pet 5)", 2, 1},
		{"veteran (level+10, pet 10)", 4, 2},
	}

	for tier := 1; tier <= 5; tier++ {
		def := coopTierTable[tier]
		fmt.Printf("\n══ Tier %d (%s) — %d days, base failure %d%%/floor, reward €%d ══\n",
			tier, def.difficulty, def.totalDays, def.baseFailurePct, def.rewardBase)

		for _, prof := range profiles {
			fmt.Printf("\n  Party profile: %s\n", prof.label)
			fmt.Printf("  %-28s %-10s %-12s %-12s %-12s %-12s\n",
				"strategy", "P(win)", "E[reward]", "E[funding]", "E[net]", "E[net/day]")
			for _, plan := range balancePlans() {
				plan.levelBonusEach = prof.levelBonus
				plan.petBonusEach = prof.petBonus
				pSuccess, eReward, eCost, eNet := simulatePlan(rng, tier, def, plan)
				perDay := eNet / float64(def.totalDays)
				fmt.Printf("  %-28s %-10.3f €%-11.0f €%-11.0f €%-11.0f €%-11.0f\n",
					plan.name, pSuccess, eReward, eCost, eNet, perDay)
			}
		}
	}
	fmt.Println()
	fmt.Println("Note: E[net] is per-player. Funding is non-refundable. Combat-action opportunity cost not included.")
}

func anyNoneFunder(plan fundingPlan) bool {
	for _, t := range plan.tiers {
		if t == CoopFundNone {
			return true
		}
	}
	return false
}

// balancePlans returns a representative set of party funding strategies.
// Party size = 4 unless name says otherwise.
func balancePlans() []fundingPlan {
	mk := func(n int, t CoopFundingTier) []CoopFundingTier {
		out := make([]CoopFundingTier, n)
		for i := range out {
			out[i] = t
		}
		return out
	}
	return []fundingPlan{
		{name: "4× Minimal", tiers: mk(4, CoopFundMinimal)},
		{name: "4× Standard", tiers: mk(4, CoopFundStandard)},
		{name: "4× Aggressive", tiers: mk(4, CoopFundAggressive)},
		{name: "4× All-In", tiers: mk(4, CoopFundAllIn)},
		{name: "4× mixed (2 Std, 2 Agg)", tiers: []CoopFundingTier{CoopFundStandard, CoopFundStandard, CoopFundAggressive, CoopFundAggressive}},
		{name: "4× sandbag (3 Std, 1 None)", tiers: []CoopFundingTier{CoopFundStandard, CoopFundStandard, CoopFundStandard, CoopFundNone}},
		{name: "3× Standard", tiers: mk(3, CoopFundStandard)},
		{name: "3× Aggressive", tiers: mk(3, CoopFundAggressive)},
		{name: "2× Standard", tiers: mk(2, CoopFundStandard)},
		{name: "2× Aggressive", tiers: mk(2, CoopFundAggressive)},
		{name: "2× All-In", tiers: mk(2, CoopFundAllIn)},
	}
}

// simulatePlan runs the trials and returns:
//
//	P(success), E[reward share post-tax], E[funding spent per player], E[net per player]
//
// Funding is paid every day regardless of wipe. Reward is split evenly across
// the party on success only. Per-player figures average the contribution across
// the actual party members under the plan.
func simulatePlan(rng *rand.Rand, tier int, def coopTierDef, plan fundingPlan) (float64, float64, float64, float64) {
	partySize := len(plan.tiers)
	if partySize < coopMinPartySize {
		return 0, 0, 0, 0
	}

	// Compute per-floor success% under this plan (deterministic given plan).
	totalMod := 0
	for _, t := range plan.tiers {
		totalMod += coopFundingTable[t].modifier
	}
	// Per-player level + pet bonuses applied across the whole party.
	totalMod += partySize * (plan.levelBonusEach + plan.petBonusEach)
	successPct := 100 - def.baseFailurePct + totalMod
	if successPct < 5 {
		successPct = 5
	}
	if successPct > 95 {
		successPct = 95
	}

	wins := 0
	for trial := 0; trial < balanceTrials; trial++ {
		runWon := true
		for floor := 0; floor < def.totalDays; floor++ {
			if rng.IntN(100) >= successPct {
				runWon = false
				break
			}
		}
		if runWon {
			wins++
		}
	}
	pWin := float64(wins) / float64(balanceTrials)

	// Average per-player numbers across the party. Funding cost = daily cost × days,
	// paid regardless of outcome. Reward = share post-tax × P(win).
	var sumCost, sumReward float64
	share := float64(def.rewardBase) / float64(partySize)
	postTaxShare := share * (1 - balanceTax)
	for _, t := range plan.tiers {
		dailyCost := float64(coopFundingTable[t].cost)
		sumCost += dailyCost * float64(def.totalDays)
		sumReward += postTaxShare * pWin
	}
	avgCost := sumCost / float64(partySize)
	avgReward := sumReward / float64(partySize)
	avgNet := avgReward - avgCost
	return pWin, avgReward, avgCost, avgNet
}

// TestSoloVsCoopDailyIncome compares expected gold-per-day for a competent
// solo dungeon grinder vs a co-op party member at the optimal funding strategy.
// Co-op should be meaningfully more rewarding per day than solo.
//
// Solo model: a "competent" player whose skill matches the location's MinLevel
// and gear matches MinEquipTier. Uses the actual loot tables from
// adventure_activities.go.
func TestSoloVsCoopDailyIncome(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Per-tier solo expected income (avg item value × items-per-haul, weighted
	// by outcome probabilities, after 5% community tax, minus a rough death
	// cost that captures gear repair + hospital).
	type soloPerTier struct {
		successPct, exceptionalPct, deathPct float64
		successHaul, exceptionalHaul         float64
		deathCost                            float64
	}
	// Probabilities computed from calculateAdvProbabilities() with skill =
	// MinLevel and eqScore matching MinEquipTier (rough but consistent).
	// Death costs assume hospital insurance + lower blacksmith repair rates.
	solo := map[int]soloPerTier{
		1: {0.74, 0.10, 0.01, 11.6, 29.0, 100},
		2: {0.67, 0.10, 0.08, 63.75, 159.0, 400},
		3: {0.60, 0.10, 0.18, 337.5, 844.0, 1200},
		4: {0.55, 0.08, 0.21, 1700.0, 4250.0, 3000},
		5: {0.61, 0.08, 0.20, 9500.0, 23750.0, 6000},
	}
	soloDaily := func(s soloPerTier) float64 {
		gross := s.successPct*s.successHaul + s.exceptionalPct*s.exceptionalHaul
		afterTax := gross * (1 - balanceTax)
		return afterTax - s.deathPct*s.deathCost
	}

	rng := rand.New(rand.NewPCG(7, 7))

	fmt.Println("\nSolo dungeon vs co-op (€/day per player at optimal funding strategy):")
	fmt.Printf("%-6s %-14s %-26s %-14s %-10s\n", "tier", "solo €/day", "co-op optimal", "co-op €/day", "ratio")
	for tier := 1; tier <= 5; tier++ {
		def := coopTierTable[tier]
		soloIncome := soloDaily(solo[tier])

		// Find best 4-player plan by E[net]/day, assuming an "average" party
		// profile (levelBonus=2, petBonus=1 per player). Restricting to
		// 4-player keeps the comparison apples-to-apples.
		var bestPlan fundingPlan
		bestPerDay := -1.0e18
		for _, plan := range balancePlans() {
			if len(plan.tiers) != 4 {
				continue
			}
			// Skip free-rider plans: an "optimal" that only works because
			// one member contributes nothing isn't a coordinated strategy.
			if anyNoneFunder(plan) {
				continue
			}
			plan.levelBonusEach = 2
			plan.petBonusEach = 1
			_, _, _, eNet := simulatePlan(rng, tier, def, plan)
			perDay := eNet / float64(def.totalDays)
			if perDay > bestPerDay {
				bestPerDay = perDay
				bestPlan = plan
			}
		}
		ratio := bestPerDay / soloIncome
		flag := "✓"
		if ratio < 1.5 {
			flag = "⚠ insufficient"
		}
		fmt.Printf("T%-5d €%-13.0f %-26s €%-13.0f %.2fx %s\n",
			tier, soloIncome, bestPlan.name, bestPerDay, ratio, flag)
	}
	fmt.Println("\n⚠ marker = co-op fails the 1.5× solo threshold; bump rewardBase.")
}

// TestCoopBalanceSweep prints, for each tier, the success% needed for E[net]>=0
// at common funding levels. Helps spot tiers where the formula is upside-down.
func TestCoopBalanceSweep(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	fmt.Println("\nBreakeven analysis — minimum P(win) for E[net]≥0 per player at party size 4:")
	fmt.Printf("%-12s %-12s %-12s %-12s %-12s\n", "tier", "Minimal", "Standard", "Aggressive", "All-In")
	for tier := 1; tier <= 5; tier++ {
		def := coopTierTable[tier]
		share := float64(def.rewardBase) / 4.0 * (1 - balanceTax)
		row := []string{}
		for _, ft := range []CoopFundingTier{CoopFundMinimal, CoopFundStandard, CoopFundAggressive, CoopFundAllIn} {
			cost := float64(coopFundingTable[ft].cost) * float64(def.totalDays)
			breakeven := cost / share
			if breakeven > 1 {
				row = append(row, ">100% (impossible)")
			} else {
				row = append(row, fmt.Sprintf("%.1f%%", breakeven*100))
			}
		}
		fmt.Printf("T%-11d %-12s %-12s %-12s %-12s\n", tier, row[0], row[1], row[2], row[3])
	}
	fmt.Println()
}
