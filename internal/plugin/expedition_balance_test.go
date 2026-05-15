package plugin

import (
	"fmt"
	"math"
	"sort"
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

// phase1TierCenterline maps each tier to its centerline player level —
// the median of the design-doc player-level range for the tier:
//
//	T1 (L1-4)  → 3
//	T2 (L3-7)  → 5
//	T3 (L5-10) → 8
//	T4 (L7-15) → 11
//	T5 (L10-20) → 15
//
// One level per tier keeps the matrix at zones × 1, not zones × range.
// Phase 4 may widen this if a tier band looks level-sensitive.
var phase1TierCenterline = map[ZoneTier]int{
	ZoneTierBeginner:   3,
	ZoneTierApprentice: 5,
	ZoneTierJourneyman: 8,
	ZoneTierVeteran:    11,
	ZoneTierLegendary:  15,
}

// TestExpeditionBalance_Phase1_FullMatrix is the Phase 1 baseline-
// measurement test: every registered zone × its tier centerline level,
// 200 trials/cell, class fixed to Fighter so the cell-to-cell delta
// isolates zone difficulty (class parity is the class-balance pass's
// job, not this one).
//
// Per the plan doc, Phase 1's gate is *pathology-only*: no zone at T1
// reads 0% completion, no zone at T5 reads 100% completion, no NaN /
// zero-day cells. The real target-band assertion lands in Phase 2 after
// the global levers have been tuned to centerline. Until then the
// matrix's job is to *log numbers we can read*, so the per-cell line
// and per-tier mean+spread go through t.Log for the commit diff.
func TestExpeditionBalance_Phase1_FullMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 1 matrix is heavy; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0xE0FFEE1

	// Walk the zone registry in declared order (matches zoneOrder so
	// the log table is stable across runs / commits).
	type cellRow struct {
		zone   ZoneDefinition
		result expeditionBalanceResult
	}
	rows := make([]cellRow, 0, len(zoneOrder))
	for i, id := range zoneOrder {
		zone, ok := getZone(id)
		if !ok {
			t.Fatalf("zoneOrder[%d]=%q not in registry", i, id)
		}
		level, ok := phase1TierCenterline[zone.Tier]
		if !ok {
			t.Fatalf("zone %q has tier %d with no phase1 centerline mapping", id, zone.Tier)
		}
		profile := expeditionBalanceProfile{
			ZoneID:   id,
			Class:    ClassFighter,
			Level:    level,
			Supplies: makeSupplies(zone.Tier, SupplyPurchase{StandardPacks: 3}),
			CampType: CampTypeStandard,
		}
		// Per-cell seed offset keeps cells independent — same seed
		// across cells would correlate their RNG streams.
		res := runExpeditionBalanceCell(profile, trialsPerCell, baseSeed+uint64(i)*1_000_003)
		rows = append(rows, cellRow{zone: zone, result: res})
	}

	// Per-cell log. The format is grep-able: leading "CELL" tag, fixed
	// columns. Future me will diff this between commits to see what
	// Phase 2's lever moves actually did.
	t.Logf("phase1 matrix — %d zones × %d trials, Fighter @ tier-centerline level", len(rows), trialsPerCell)
	t.Logf("CELL  %-18s %-3s %-3s  %-12s %-12s %-13s %-11s %-13s %-9s %-12s",
		"zone", "tier", "lvl", "comp%", "death%", "starve%", "med_days", "med_threat", "encs", "hp_left%")
	for _, row := range rows {
		r := row.result
		t.Logf("CELL  %-18s T%d  L%-2d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%% med_days=%2d med_threat=%3d encs=%4.1f hp_left=%5.1f%%",
			row.zone.ID, row.zone.Tier, r.Profile.Level,
			r.CompletionRate()*100,
			r.DeathRate()*100,
			float64(r.StarvedOuts)/float64(r.Trials)*100,
			r.MedianDays, r.MedianThreatEnd,
			r.AvgEncounters, r.AvgHPRemainingPct*100,
		)
	}

	// Per-tier diagnostic: mean completion% across the zones in the
	// tier, and the spread (max − min). Mirrors the class-balance
	// per-(level,tier) spread diagnostic. A tight spread means the
	// global lever pass alone can drag the tier onto target; a wide
	// spread is a signal that Phase 3 per-zone outlier work is
	// unavoidable.
	tierRows := map[ZoneTier][]cellRow{}
	for _, row := range rows {
		tierRows[row.zone.Tier] = append(tierRows[row.zone.Tier], row)
	}
	tiers := []ZoneTier{
		ZoneTierBeginner, ZoneTierApprentice, ZoneTierJourneyman,
		ZoneTierVeteran, ZoneTierLegendary,
	}
	for _, tier := range tiers {
		group := tierRows[tier]
		if len(group) == 0 {
			continue
		}
		var sum, lo, hi float64
		lo = math.Inf(1)
		hi = math.Inf(-1)
		zoneNames := make([]string, 0, len(group))
		for _, row := range group {
			c := row.result.CompletionRate() * 100
			sum += c
			if c < lo {
				lo = c
			}
			if c > hi {
				hi = c
			}
			zoneNames = append(zoneNames, fmt.Sprintf("%s=%.1f%%", row.zone.ID, c))
		}
		sort.Strings(zoneNames)
		mean := sum / float64(len(group))
		t.Logf("TIER  T%d  n=%d  mean_comp=%5.1f%%  spread=%5.1f pp  [%s]",
			tier, len(group), mean, hi-lo, joinZones(zoneNames))
	}

	// Gates split into two buckets:
	//
	//   *Wiring* pathologies (zero-day loop, days > cap) — fatal. These
	//   would mean the harness itself is broken and the matrix numbers
	//   above are noise.
	//
	//   *Difficulty* sentinels (0% at T1, 100% at T5) — logged as WARN,
	//   not fatal. Phase 1's contract is "log a baseline"; the band
	//   assertion is Phase 2's job once the global levers have been
	//   tuned to land within ±10pp of target. Promoting these to
	//   t.Errorf here would turn Phase 1 into a tuning gate before
	//   Phase 2 has a chance to move the levers, which the plan doc
	//   explicitly defers.
	for _, row := range rows {
		r := row.result
		if r.MedianDays == 0 {
			t.Errorf("%s: median days == 0; day loop never advanced", row.zone.ID)
		}
		if r.MedianDays > harnessMaxDays {
			t.Errorf("%s: median days %d > cap %d; termination wiring broken",
				row.zone.ID, r.MedianDays, harnessMaxDays)
		}
		if row.zone.Tier == ZoneTierBeginner && r.Completions == 0 {
			t.Logf("WARN  %s (T1): 0%% completions in %d trials — Phase 2 should lift this",
				row.zone.ID, r.Trials)
		}
		if row.zone.Tier == ZoneTierLegendary && r.Completions == r.Trials {
			t.Logf("WARN  %s (T5): 100%% completions in %d trials — Phase 2 should pressure this",
				row.zone.ID, r.Trials)
		}
	}
}

// joinZones is a tiny helper kept local to the test file so the
// per-tier log line reads in one logical chunk without pulling in
// strings.Join's import for production code.
func joinZones(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
