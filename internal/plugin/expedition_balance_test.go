package plugin

import (
	"testing"
)

// TestExpeditionBalance_Phase0_Spike is the sanity check the
// expedition-difficulty plan doc's Phase 0 calls for: one cell, 100
// trials, sensible numbers, fully deterministic from the seed.
//
// The cell — T2 zone (Crypt Valdris) × L5 Fighter — was chosen
// because it's the smallest cell that actually exercises every part
// of the seam: tier scaling raises monster AC/atk, supply burn lands
// at 1.5×, the player is built off the class-balance loadout ladder
// at the T2/+1 step, and the night phase has roster entries to draw
// from. If the seam works here, Phase 1's full-matrix expansion is
// mostly typing.
//
// Pass criteria are intentionally loose — we are not asserting a
// target band yet (Phase 2 does that), only that nothing is wired up
// so badly the result is degenerate (0%/100%/NaN/zero days). Anything
// in (0..100%) completion at this cell is a green Phase 0.
func TestExpeditionBalance_Phase0_Spike(t *testing.T) {
	const trials = 100
	profile := expeditionBalanceProfile{
		ZoneID: ZoneCryptValdris,
		Class:  ClassFighter,
		Level:  5,
		Supplies: makeSupplies(ZoneTierApprentice, SupplyPurchase{
			StandardPacks: 3,
			DeluxePacks:   0,
		}),
		CampType: CampTypeStandard,
	}

	res := runExpeditionBalanceCell(profile, trials, 0xC0FFEE)

	t.Logf("cell: %s L%d %s — completions=%d/%d (%.1f%%), deaths=%d, starved=%d, "+
		"median_days=%d, median_threat=%d, avg_encounters=%.1f, avg_hp_remaining=%.1f%%",
		profile.ZoneID, profile.Level, profile.Class,
		res.Completions, res.Trials, res.CompletionRate()*100,
		res.Deaths, res.StarvedOuts,
		res.MedianDays, res.MedianThreatEnd,
		res.AvgEncounters, res.AvgHPRemainingPct*100,
	)

	// Degenerate-outcome gates. These are harness-broken sentinels,
	// not difficulty assertions — Phase 2 layers the real band on
	// top of this same test.
	if res.Trials != trials {
		t.Fatalf("trial count mismatch: got %d, want %d", res.Trials, trials)
	}
	if res.Completions == 0 {
		t.Errorf("zero completions in %d trials at T2/L5 Fighter — the spike cell should not be unwinnable", trials)
	}
	if res.Completions == trials {
		t.Errorf("100%% completions in %d trials at T2/L5 Fighter — interrupt rolls / night checks not pressuring the run", trials)
	}
	if res.MedianDays == 0 {
		t.Fatalf("median days == 0; day loop never advanced")
	}
	if res.MedianDays > harnessMaxDays {
		t.Fatalf("median days %d > cap %d; termination wiring broken", res.MedianDays, harnessMaxDays)
	}
}

// TestExpeditionBalance_Phase0_SeedSpread confirms the RNG seam is
// actually wired — different seeds produce different trial outcomes
// across a small sample. Full byte-for-byte reproducibility under
// the same seed is *not* asserted at Phase 0: a couple of production
// helpers we lean on (surpriseRoundNick, pickWanderingMonster) draw
// from the package-global math/rand/v2, same as the class-balance
// harness. Phase 1 lifts those to seeded variants if matrix
// reproducibility becomes a real requirement; until then, "seeds
// differentiate" is the contract we can honestly hold.
func TestExpeditionBalance_Phase0_SeedSpread(t *testing.T) {
	profile := expeditionBalanceProfile{
		ZoneID: ZoneCryptValdris,
		Class:  ClassFighter,
		Level:  5,
		Supplies: makeSupplies(ZoneTierApprentice, SupplyPurchase{
			StandardPacks: 3,
		}),
		CampType: CampTypeStandard,
	}
	const seedA uint64 = 0xDEADBEEF
	const seedB uint64 = 0xFEEDFACE
	a := runExpeditionBalanceTrial(profile, seedA)
	b := runExpeditionBalanceTrial(profile, seedB)
	if a == b {
		t.Fatalf("two distinct seeds produced byte-identical trials — RNG seam may not be wired:\n a = %+v\n b = %+v", a, b)
	}
}
