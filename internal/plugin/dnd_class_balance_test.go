package plugin

import (
	"testing"
)

// Phase 0 spike — Fighter vs. Mage sanity run. Per gogobee_class_balance.md
// §5 Phase 0: "run Fighter vs. Mage only across tiers and sanity-check
// plausibility (both win something; casters not at 0%)."
//
// This test is the gate before Phase 1 generalizes the matrix. It does
// NOT assert balance — only that the harness produces plausible numbers.
// Phase 2 promotes the assertions to per-tier win-rate parity bands.
//
// Skipped under -short. Even 200 trials × 2 classes × 5 tiers is fast
// (<1s on a laptop), but it's pure measurement noise to anything else.
func TestClassBalance_Phase0_FighterVsMage(t *testing.T) {
	if testing.Short() {
		t.Skip("phase-0 spike — measurement only")
	}

	profiles := []classBalanceProfile{
		{Class: ClassFighter, Level: 1},
		{Class: ClassFighter, Level: 3},
		{Class: ClassMage, Level: 1},
		{Class: ClassMage, Level: 3},
	}
	const trials = 400
	results := runClassBalanceMatrix(profiles, trials)

	t.Logf("class-balance Phase 0 — Fighter vs. Mage, %d trials/cell", trials)
	t.Logf("%-8s %-5s T1     T2     T3     T4     T5", "class", "lvl")
	type key struct {
		Class DnDClass
		Level int
	}
	byProf := make(map[key]map[int]classBalanceResult)
	for _, r := range results {
		k := key{r.Profile.Class, r.Profile.Level}
		if byProf[k] == nil {
			byProf[k] = make(map[int]classBalanceResult)
		}
		byProf[k][r.Tier] = r
	}
	for _, p := range profiles {
		row := byProf[key{p.Class, p.Level}]
		t.Logf("%-8s %-5d %.3f  %.3f  %.3f  %.3f  %.3f",
			p.Class, p.Level,
			row[1].WinRate(), row[2].WinRate(), row[3].WinRate(),
			row[4].WinRate(), row[5].WinRate())
	}

	// Plausibility gates — these are NOT the Phase 2 parity assertions.
	// They catch a fully broken harness: e.g. spells never resolving and
	// the Mage reading 0% across the board, or the Fighter losing every
	// T1 fight because the loadout layer didn't wire weapon dice in.
	for _, r := range results {
		// Every profile should win *something* at T1 (the entry-level
		// dungeon). 0% there means the build is incapable of damage —
		// either the equipment layer or the spell layer is dead.
		if r.Tier == 1 && r.WinRate() == 0 {
			t.Errorf("%s L%d T1 win rate is 0%% — the build can't deal damage; check loadout/spell policies",
				r.Profile.Class, r.Profile.Level)
		}
		// And every profile should lose *something* at T5 (the
		// endgame) at low level — a 100% win rate at T5 with an L1
		// build means monster scaling isn't doing its job and the
		// harness numbers downstream will be useless.
		if r.Tier == 5 && r.Profile.Level == 1 && r.WinRate() == 1 {
			t.Errorf("%s L1 T5 win rate is 100%% — monster scaling looks broken",
				r.Profile.Class)
		}
	}

	// Phase 0's specific concern from the doc: caster reads 0% because
	// no spell got queued. Cross-check that Mage T1 win rate is at least
	// in the ballpark of Fighter T1 — within a 50pp band. If Mage is
	// catastrophically below Fighter at the entry tier, the spell
	// selection policy isn't biting.
	fighterT1 := byProf[key{ClassFighter, 1}][1].WinRate()
	mageT1 := byProf[key{ClassMage, 1}][1].WinRate()
	if fighterT1-mageT1 > 0.50 {
		t.Errorf("Mage L1 T1 win rate %.2f vs Fighter %.2f — gap > 50pp suggests spell policy isn't firing",
			mageT1, fighterT1)
	}
}
