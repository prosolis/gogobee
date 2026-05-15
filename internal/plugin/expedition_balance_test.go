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

// phase1TierCenterline maps each tier to its centerline player level.
// Originally the design doc's median per tier; bumped where the
// gear-ladder boundaries (gearTier in dnd_class_balance.go: 5/9/13/17)
// pushed the median into a gear bracket *below* the zone's tier. The
// harness's classLoadout is shared with the class-balance pass, and
// retuning its boundaries would shift class-balance numbers too, so the
// adjustment lands here instead: pick the lowest level inside each
// tier's design-doc level range that resolves to gearTier == tier.
//
//	T1 (L1-4)   → 3   gearTier 1 = mundane              ✓
//	T2 (L3-7)   → 5   gearTier 2 = +1 weapon/armor      ✓
//	T3 (L5-10)  → 9   gearTier 3 = +2 (was L8 → +1)
//	T4 (L7-15)  → 13  gearTier 4 = +3 (was L11 → +2)
//	T5 (L10-20) → 17  gearTier 5 = +3 (was L15 → +3, but bumped
//	                 for consistency with the rule, no stat impact)
//
// All centerlines stay inside their design-doc range. One level per
// tier keeps the matrix at zones × 1, not zones × range. Phase 4 may
// widen this if a tier band looks level-sensitive.
var phase1TierCenterline = map[ZoneTier]int{
	ZoneTierBeginner:   3,
	ZoneTierApprentice: 5,
	ZoneTierJourneyman: 9,
	ZoneTierVeteran:    13,
	ZoneTierLegendary:  17,
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

// TestExpeditionBalance_Phase2_CadenceCalibration sweeps the harness's
// daytime combat-interrupt cadence across {1,2,3,4} rolls/day and logs
// the full matrix per cadence. Diagnostic-only — no gates beyond the
// Phase 1 wiring pathologies.
//
// Why this exists: Phase 1's baseline came back uniformly 0% / 100%
// death at every cell, and the commit message named cadence as the
// suspected lever. We can't compare against a live trace
// (prod is too low-traffic; the corpus is one in-flight expedition
// with zero interrupt rows), so the cadence constant is itself a
// tunable. This test surfaces the curve so Phase 2's global-lever
// pass starts from a cadence where the matrix has signal — i.e. where
// T1 is roughly in the 70–90% band without flooring T5 to 100%.
//
// Cell shape mirrors Phase 1: every registered zone × its tier-
// centerline level, Fighter, 200 trials. The only thing that varies
// across runs is HarvestRollsPerDay.
func TestExpeditionBalance_Phase2_CadenceCalibration(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 2 cadence sweep is heavy; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0xCAFEC1DE
	cadences := []int{1, 2, 3, 4}

	t.Logf("phase2 cadence calibration — %d zones × %d cadences × %d trials, Fighter @ tier-centerline level",
		len(zoneOrder), len(cadences), trialsPerCell)

	for _, rolls := range cadences {
		// Per-tier aggregation for the headline view.
		type tierStat struct {
			cells int
			sumC  float64
			lo    float64
			hi    float64
		}
		tierStats := map[ZoneTier]*tierStat{}

		t.Logf("─── HarvestRollsPerDay=%d ───", rolls)
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
				ZoneID:             id,
				Class:              ClassFighter,
				Level:              level,
				Supplies:           makeSupplies(zone.Tier, SupplyPurchase{StandardPacks: 3}),
				CampType:           CampTypeStandard,
				HarvestRollsPerDay: rolls,
			}
			// Seed schedule: same base + cell offset as Phase 1, plus a
			// cadence-dependent salt so different cadences don't sample
			// the same correlated streams.
			seed := baseSeed + uint64(i)*1_000_003 + uint64(rolls)*7919
			r := runExpeditionBalanceCell(profile, trialsPerCell, seed)

			c := r.CompletionRate() * 100
			t.Logf("CELL  rolls=%d  %-18s T%d  L%-2d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%% med_days=%2d med_threat=%3d encs=%4.1f hp_left=%5.1f%%",
				rolls, zone.ID, zone.Tier, r.Profile.Level,
				c,
				r.DeathRate()*100,
				float64(r.StarvedOuts)/float64(r.Trials)*100,
				r.MedianDays, r.MedianThreatEnd,
				r.AvgEncounters, r.AvgHPRemainingPct*100,
			)

			ts, ok := tierStats[zone.Tier]
			if !ok {
				ts = &tierStat{lo: math.Inf(1), hi: math.Inf(-1)}
				tierStats[zone.Tier] = ts
			}
			ts.cells++
			ts.sumC += c
			if c < ts.lo {
				ts.lo = c
			}
			if c > ts.hi {
				ts.hi = c
			}
		}

		tiers := []ZoneTier{
			ZoneTierBeginner, ZoneTierApprentice, ZoneTierJourneyman,
			ZoneTierVeteran, ZoneTierLegendary,
		}
		for _, tier := range tiers {
			ts := tierStats[tier]
			if ts == nil || ts.cells == 0 {
				continue
			}
			mean := ts.sumC / float64(ts.cells)
			t.Logf("TIER  rolls=%d  T%d  n=%d  mean_comp=%5.1f%%  spread=%5.1f pp",
				rolls, tier, ts.cells, mean, ts.hi-ts.lo)
		}
	}
}

// TestExpeditionBalance_Phase2_LethalityProbe runs a handful of trials
// at the cleanest cell (T1 goblin_warrens L3 Fighter, gear-aligned so
// gear mismatch isn't a factor) and logs per-fight details: monster,
// max HP, surprise nick, HP pre/post fight, enemy AC/atk, outcome.
//
// The cadence sweep (TestExpeditionBalance_Phase2_CadenceCalibration)
// proved cadence isn't the dominant lever — even at rolls=1 the T1
// cell sits at 2% completion with bimodal hp_left (100% for survivors,
// 0% for the dead). This probe is the next diagnostic step: see
// whether the lethality comes from the pre-combat nick, the picked
// monster's tier-floored stats, or the combat fold itself.
//
// Diagnostic-only — no assertions beyond "produced trace lines."
// Next session reads the output and picks Phase 2's real first lever.
func TestExpeditionBalance_Phase2_LethalityProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 2 lethality probe writes verbose log; -short skips it")
	}

	const trials = 5
	const baseSeed uint64 = 0x1E7A11

	profile := expeditionBalanceProfile{
		ZoneID:             ZoneGoblinWarrens,
		Class:              ClassFighter,
		Level:              3,
		Supplies:           makeSupplies(ZoneTierBeginner, SupplyPurchase{StandardPacks: 3}),
		CampType:           CampTypeStandard,
		HarvestRollsPerDay: 1, // sparse cadence so each fight is legible
	}

	t.Logf("phase2 lethality probe — %d trials at %s L%d %s (rolls=1)",
		trials, profile.ZoneID, profile.Level, profile.Class)

	for trial := 0; trial < trials; trial++ {
		exp := newHarnessExpedition(profile)
		char := buildHarnessCharacter(classBalanceProfile{
			Class: profile.Class,
			Level: profile.Level,
		})
		t.Logf("─── trial %d: hp_max=%d ───", trial, char.HPMax)
		h := &expeditionHarness{
			exp:         exp,
			char:        char,
			rng:         newHarnessRNG(baseSeed + uint64(trial)),
			rollsPerDay: profile.HarvestRollsPerDay,
			traceFight: func(line string) {
				t.Logf("  %s", line)
			},
		}
		for {
			res := h.advanceExpeditionOneDay()
			if res.EndedReason != "" {
				t.Logf("END trial %d: reason=%s days=%d threat=%d encs=%d hp_left_pct=%.1f%%",
					trial, res.EndedReason, res.DaysElapsed, res.ThreatAtEnd,
					res.CombatEncounters, res.HPRemainingPct*100)
				break
			}
		}
	}
}

// TestExpeditionBalance_Phase2_TierLethality is the tier-walking
// companion to the T1-only lethality probe. Phase 1's matrix shows
// uniform 0% across every tier at the default rolls=4 cadence, which
// the T1/rolls=1 probe alone can't explain (its bimodal-warchief
// finding wouldn't show at T5 vs an L17 fighter). This probe traces
// every fight at the matrix cadence across one zone per tier so we
// can read whether the deaths are elite-driven, standard-fight
// chained, or something the harness combat fold itself is doing.
//
// Diagnostic-only — no assertions.
func TestExpeditionBalance_Phase2_TierLethality(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 2 tier-lethality probe writes verbose log; -short skips it")
	}

	const trialsPerTier = 3
	const baseSeed uint64 = 0x7E11A1
	cells := []struct {
		zone ZoneID
		tier ZoneTier
	}{
		{ZoneGoblinWarrens, ZoneTierBeginner},
		{ZoneForestShadows, ZoneTierApprentice},
		{ZoneUnderforge, ZoneTierJourneyman},
		{ZoneUnderdark, ZoneTierVeteran},
		{ZoneDragonsLair, ZoneTierLegendary},
	}
	for _, cell := range cells {
		level := phase1TierCenterline[cell.tier]
		profile := expeditionBalanceProfile{
			ZoneID:             cell.zone,
			Class:              ClassFighter,
			Level:              level,
			Supplies:           makeSupplies(cell.tier, SupplyPurchase{StandardPacks: 3}),
			CampType:           CampTypeStandard,
			HarvestRollsPerDay: 4, // matrix default, not the sparse probe
		}
		t.Logf("═══ %s T%d L%d Fighter rolls=4 ═══", cell.zone, cell.tier, level)
		for trial := 0; trial < trialsPerTier; trial++ {
			exp := newHarnessExpedition(profile)
			char := buildHarnessCharacter(classBalanceProfile{
				Class: profile.Class,
				Level: profile.Level,
			})
			t.Logf("─── %s trial %d: hp_max=%d ───", cell.zone, trial, char.HPMax)
			h := &expeditionHarness{
				exp:         exp,
				char:        char,
				rng:         newHarnessRNG(baseSeed + uint64(trial) + uint64(cell.tier)*101),
				rollsPerDay: profile.HarvestRollsPerDay,
				traceFight: func(line string) {
					t.Logf("  %s", line)
				},
			}
			for {
				res := h.advanceExpeditionOneDay()
				if res.EndedReason != "" {
					t.Logf("END %s trial %d: reason=%s days=%d threat=%d encs=%d hp_left_pct=%.1f%%",
						cell.zone, trial, res.EndedReason, res.DaysElapsed, res.ThreatAtEnd,
						res.CombatEncounters, res.HPRemainingPct*100)
					break
				}
			}
		}
	}
}

// TestExpeditionBalance_Phase2_LeverSweep is the tuning sweep the
// plan doc's Phase 2 "global lever tuning" step calls for. Phases 2a
// and 2b surfaced two knobs — retreatThreatBump (the threat penalty
// on a wounded-but-alive break-off) and the surprise-nick wounded-
// entrant divisor (the wounded-fighter lethality clamp) — but the
// post-2b matrix still reads uniform 0% completion across every
// tier. The question this test answers: do alternate settings of
// those two knobs lift the centerline off the floor, and if so by
// how much per tier?
//
// Shape mirrors Phase2_CadenceCalibration: one cell per tier (zone
// at tier centerline level), Fighter, default cadence (rolls=4),
// 200 trials, full grid of (bump × divisor). Diagnostic-only — no
// assertions; the plan-doc test that gates the ±10pp band is added
// after this sweep names a winner.
//
// Cost: 9 lever combos × |zones| × 200 trials. -short skips the
// sweep entirely.
func TestExpeditionBalance_Phase2_LeverSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 2 lever sweep is heavy; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0xB001E5

	// Live values are bump=5 (Phase 2a) and divisor=5 (Phase 2b). The
	// sweep walks two settings tighter (gentler) and one looser
	// (harsher) for each knob so the live point sits in the middle of
	// the grid and we can read the slope in both directions.
	bumps := []int{2, 5, 10}        // threat-bump per retreat
	divisors := []int{3, 5, 8, 12}  // /N wounded-nick divisor; bigger = gentler

	t.Logf("phase2 lever sweep — %d zones × %d bump × %d divisor × %d trials, Fighter @ tier centerline (rolls=4)",
		len(zoneOrder), len(bumps), len(divisors), trialsPerCell)

	type tierStat struct {
		cells int
		sumC  float64
		lo    float64
		hi    float64
	}

	for _, bump := range bumps {
		for _, div := range divisors {
			tierStats := map[ZoneTier]*tierStat{}
			t.Logf("─── retreatThreatBump=%d  surpriseNickDivisor=%d ───", bump, div)
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
					ZoneID:                      id,
					Class:                       ClassFighter,
					Level:                       level,
					Supplies:                    makeSupplies(zone.Tier, SupplyPurchase{StandardPacks: 3}),
					CampType:                    CampTypeStandard,
					RetreatThreatBumpOverride:   bump,
					SurpriseNickDivisorOverride: div,
				}
				// Seed schedule: Phase 1 base + cell offset, plus a
				// lever-dependent salt so each (bump, div) cell sees a
				// fresh RNG stream rather than aliasing earlier sweeps.
				seed := baseSeed + uint64(i)*1_000_003 + uint64(bump)*101 + uint64(div)*7919
				r := runExpeditionBalanceCell(profile, trialsPerCell, seed)

				c := r.CompletionRate() * 100
				t.Logf("CELL  b=%-2d d=%-2d  %-18s T%d  L%-2d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%% med_days=%2d med_threat=%3d encs=%4.1f hp_left=%5.1f%%",
					bump, div, zone.ID, zone.Tier, r.Profile.Level,
					c,
					r.DeathRate()*100,
					float64(r.StarvedOuts)/float64(r.Trials)*100,
					r.MedianDays, r.MedianThreatEnd,
					r.AvgEncounters, r.AvgHPRemainingPct*100,
				)

				ts, ok := tierStats[zone.Tier]
				if !ok {
					ts = &tierStat{lo: math.Inf(1), hi: math.Inf(-1)}
					tierStats[zone.Tier] = ts
				}
				ts.cells++
				ts.sumC += c
				if c < ts.lo {
					ts.lo = c
				}
				if c > ts.hi {
					ts.hi = c
				}
			}

			tiers := []ZoneTier{
				ZoneTierBeginner, ZoneTierApprentice, ZoneTierJourneyman,
				ZoneTierVeteran, ZoneTierLegendary,
			}
			for _, tier := range tiers {
				ts := tierStats[tier]
				if ts == nil || ts.cells == 0 {
					continue
				}
				mean := ts.sumC / float64(ts.cells)
				t.Logf("TIER  b=%-2d d=%-2d  T%d  n=%d  mean_comp=%5.1f%%  spread=%5.1f pp",
					bump, div, tier, ts.cells, mean, ts.hi-ts.lo)
			}
		}
	}
}

// TestExpeditionBalance_Phase3_GlobalLeverSweep is the global-tuning
// sweep the plan doc's "Phase 3 — global lever tuning" step calls for.
// Phase 2c (roster gate) lifted T1 goblin_warrens off the floor (~3%)
// but every other tier still reads 0% in the post-2c Phase 1 matrix.
// The Phase 2 lever sweep proved the wounded-cascade knobs are inert
// once the clamp is in place — deaths are now fresh-entry elite
// one-shots and multi-day AC/init creep, not chained nicks.
//
// This sweep walks two of the global knobs called out in the plan-doc
// "Phase 3 — global lever tuning" section:
//
//   eliteInterruptThreshold (live=19, total roll cutoff for Elite
//   bracket) — directly controls how often fresh-entry elite fights
//   trigger during the daytime harvest pipeline. Sweep {17, 19, 23}:
//   17 = more elites (slope check below live), 19 = live baseline,
//   23 = elites only when roll+mod ≥ 23 (rare even at T5).
//
//   threatDriftBase (live=3, daily threat clock drift before mood-mod)
//   — slows the multi-day AC/init/supply-burn creep that compounds
//   over a 14-day expedition. Sweep {1, 3, 5}: 1 = nearly flat, 3 =
//   live, 5 = harsher.
//
// 3×3 = 9 combos × 10 zones × 200 trials/cell. Diagnostic-only — no
// gates beyond the Phase 1 wiring sanity. -short skips.
func TestExpeditionBalance_Phase3_GlobalLeverSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 3 global-lever sweep is heavy; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0x9101E5

	eliteThresholds := []int{17, 19, 23}
	driftBases := []int{1, 3, 5}

	t.Logf("phase3 global-lever sweep — %d zones × %d elite-thresholds × %d drift-bases × %d trials, Fighter @ tier centerline (rolls=4)",
		len(zoneOrder), len(eliteThresholds), len(driftBases), trialsPerCell)

	type tierStat struct {
		cells int
		sumC  float64
		lo    float64
		hi    float64
	}

	for _, elite := range eliteThresholds {
		for _, drift := range driftBases {
			tierStats := map[ZoneTier]*tierStat{}
			t.Logf("─── eliteInterruptThreshold=%d  threatDriftBase=%d ───", elite, drift)
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
					ZoneID:                          id,
					Class:                           ClassFighter,
					Level:                           level,
					Supplies:                        makeSupplies(zone.Tier, SupplyPurchase{StandardPacks: 3}),
					CampType:                        CampTypeStandard,
					EliteInterruptThresholdOverride: elite,
					ThreatDriftBaseOverride:         drift,
				}
				seed := baseSeed + uint64(i)*1_000_003 + uint64(elite)*101 + uint64(drift)*7919
				r := runExpeditionBalanceCell(profile, trialsPerCell, seed)

				c := r.CompletionRate() * 100
				t.Logf("CELL  e=%-2d d=%-1d  %-18s T%d  L%-2d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%% med_days=%2d med_threat=%3d encs=%4.1f hp_left=%5.1f%%",
					elite, drift, zone.ID, zone.Tier, r.Profile.Level,
					c,
					r.DeathRate()*100,
					float64(r.StarvedOuts)/float64(r.Trials)*100,
					r.MedianDays, r.MedianThreatEnd,
					r.AvgEncounters, r.AvgHPRemainingPct*100,
				)

				ts, ok := tierStats[zone.Tier]
				if !ok {
					ts = &tierStat{lo: math.Inf(1), hi: math.Inf(-1)}
					tierStats[zone.Tier] = ts
				}
				ts.cells++
				ts.sumC += c
				if c < ts.lo {
					ts.lo = c
				}
				if c > ts.hi {
					ts.hi = c
				}
			}

			tiers := []ZoneTier{
				ZoneTierBeginner, ZoneTierApprentice, ZoneTierJourneyman,
				ZoneTierVeteran, ZoneTierLegendary,
			}
			for _, tier := range tiers {
				ts := tierStats[tier]
				if ts == nil || ts.cells == 0 {
					continue
				}
				mean := ts.sumC / float64(ts.cells)
				t.Logf("TIER  e=%-2d d=%-1d  T%d  n=%d  mean_comp=%5.1f%%  spread=%5.1f pp",
					elite, drift, tier, ts.cells, mean, ts.hi-ts.lo)
			}
		}
	}
}

// TestExpeditionBalance_Phase3B_NickSupplySweep is the second
// global-tuning sweep from gogobee_expedition_difficulty.md. Phase 3-A
// surfaced the elite-bracket threshold as the dominant T1–T3 lever
// (e=23/d=1 lifted T1 from 3% → 24%) but exposed a tail-side
// fingerprint shift: T4/T5 dragons_lair death dropped 60% → 24% while
// starvation climbed to 75% — the fighter now survives elites long
// enough to run out of food. Phase 3-A picked the best cell but it
// still leaves every tier well below target band (T1 70-90%, T2
// 62-82%, …, T5 ~40%).
//
// Phase 3-B holds the Phase 3-A best cell (eliteInterruptThreshold=23,
// threatDriftBase=1) and walks two more global knobs:
//
//   surpriseNickFloor (live=tier, i.e. 1..5 by zone tier) — the raw
//   surprise-round nick on a fresh-HP entry. Lower floor = less
//   per-fight chip on T4/T5 standard fights, where the live tier=4-5
//   floor is the biggest single contributor to the wear-down curve
//   that funnels survivors into starvation. Sweep {-1 (disable, =0),
//   1 (flat), 0-sentinel (live tier)}.
//
//   supplyBurnRatePct (live=100, %) — daily supply burn scalar. Lower
//   = more days of margin. T4 and T5 burns are 3×/4× the T1 baseline;
//   halving them ought to convert the starvation outs we saw in
//   Phase 3-A into actual completions if survivability is the only
//   remaining blocker, or leave them dying in combat if it isn't.
//   Sweep {100 (live), 75, 50}.
//
// 3×3 = 9 combos × 10 zones × 200 trials/cell, all on top of the
// Phase 3-A best cell. Diagnostic-only — no gates beyond Phase 1
// wiring sanity. -short skips.
func TestExpeditionBalance_Phase3B_NickSupplySweep(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 3-B nick/supply sweep is heavy; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0xB1C5E2
	// Phase 3-A best cell — held constant across this sweep.
	const eliteThreshold = 23
	const driftBase = 1

	nickFloors := []int{-1, 1, 0} // -1 disables floor; 1 flat-1; 0 = use live tier
	burnPcts := []int{50, 75, 100}

	t.Logf("phase3-B nick/supply sweep — %d zones × %d nick-floors × %d burn-pcts × %d trials, Fighter @ tier centerline (rolls=4); base e=%d d=%d",
		len(zoneOrder), len(nickFloors), len(burnPcts), trialsPerCell, eliteThreshold, driftBase)

	type tierStat struct {
		cells int
		sumC  float64
		lo    float64
		hi    float64
	}

	nickLabel := func(f int) string {
		switch {
		case f == 0:
			return "tier"
		case f < 0:
			return "0"
		default:
			return fmt.Sprintf("%d", f)
		}
	}

	for _, floor := range nickFloors {
		for _, burn := range burnPcts {
			tierStats := map[ZoneTier]*tierStat{}
			t.Logf("─── surpriseNickFloor=%s  supplyBurnPct=%d ───", nickLabel(floor), burn)
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
					ZoneID:                          id,
					Class:                           ClassFighter,
					Level:                           level,
					Supplies:                        makeSupplies(zone.Tier, SupplyPurchase{StandardPacks: 3}),
					CampType:                        CampTypeStandard,
					EliteInterruptThresholdOverride: eliteThreshold,
					ThreatDriftBaseOverride:         driftBase,
					SurpriseNickFloorOverride:       floor,
					SupplyBurnRatePctOverride:       burn,
				}
				seed := baseSeed + uint64(i)*1_000_003 +
					uint64(floor+2)*1009 + uint64(burn)*7919
				r := runExpeditionBalanceCell(profile, trialsPerCell, seed)

				c := r.CompletionRate() * 100
				t.Logf("CELL  f=%-4s b=%-3d  %-18s T%d  L%-2d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%% med_days=%2d med_threat=%3d encs=%4.1f hp_left=%5.1f%%",
					nickLabel(floor), burn, zone.ID, zone.Tier, r.Profile.Level,
					c,
					r.DeathRate()*100,
					float64(r.StarvedOuts)/float64(r.Trials)*100,
					r.MedianDays, r.MedianThreatEnd,
					r.AvgEncounters, r.AvgHPRemainingPct*100,
				)

				ts, ok := tierStats[zone.Tier]
				if !ok {
					ts = &tierStat{lo: math.Inf(1), hi: math.Inf(-1)}
					tierStats[zone.Tier] = ts
				}
				ts.cells++
				ts.sumC += c
				if c < ts.lo {
					ts.lo = c
				}
				if c > ts.hi {
					ts.hi = c
				}
			}

			tiers := []ZoneTier{
				ZoneTierBeginner, ZoneTierApprentice, ZoneTierJourneyman,
				ZoneTierVeteran, ZoneTierLegendary,
			}
			for _, tier := range tiers {
				ts := tierStats[tier]
				if ts == nil || ts.cells == 0 {
					continue
				}
				mean := ts.sumC / float64(ts.cells)
				t.Logf("TIER  f=%-4s b=%-3d  T%d  n=%d  mean_comp=%5.1f%%  spread=%5.1f pp",
					nickLabel(floor), burn, tier, ts.cells, mean, ts.hi-ts.lo)
			}
		}
	}
}

// TestExpeditionBalance_Phase4A_OutlierDiagnostic is the per-zone
// diagnostic the plan doc's Phase 4 calls for. Phase 3-B's global
// sweep wrung out the matrix-wide knobs and named four outlier zones
// that still read ~0% completion at every (elite-threshold, drift,
// nick-floor, supply-burn) combo: crypt_valdris T1, forest_shadows
// T2, manor_blackspire T3, abyss_portal T5. Each has a healthy
// sibling at the same tier (goblin_warrens, sunken_temple,
// underforge, dragons_lair) that completes at the matrix mean.
//
// This test holds the Phase 3-A/3-B best cell (e=23, d=1, burn=50,
// nick-floor=tier — recall nick-floor was inert) and pairs each
// outlier with its sibling. For every (zone, trial) it captures the
// structured fight trace via traceFightStruct and aggregates:
//
//   - Per-monster: appearances, win-rate, avg HP-loss-on-win,
//     attribution of the final losing fight ("kills"). Surfaces the
//     "one monster owns the lethality budget" pattern that single-
//     boss-pool elite gates already mitigated for the live healthy
//     zones (Phase 2c).
//   - Day-of-end histogram (1..14): clusters early-day deaths
//     (chained-interrupt collapse) vs late-day starve-outs vs
//     survives-to-cap.
//   - Elite vs Standard fight mix: confirms whether the outlier is
//     drawing disproportionate elites (roster weight problem) or
//     hitting normal cadence and losing baseline fights (stat-floor
//     problem).
//
// Constraint per the plan doc: do not touch monster stat blocks
// (those are bestiary, shared across systems). The output of this
// test names the per-zone tool to reach for — SpawnWeight rebalance,
// elite-flag toggle, supply-DC pull, or boss tuning (boss is
// zone-scoped, not bestiary-scoped, so it's in-bounds).
//
// Diagnostic-only — no gates. -short skips.
func TestExpeditionBalance_Phase4A_OutlierDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 4-A diagnostic walks 8 zones × 200 trials; -short skips it")
	}

	const trialsPerZone = 200
	const baseSeed uint64 = 0xF40A4D
	// Phase 3-A/3-B best cell — same as the Phase 3-B nick=tier b=50 row.
	const eliteThreshold = 23
	const driftBase = 1
	const supplyBurnPct = 50

	type pair struct {
		outlier ZoneID
		sibling ZoneID
		tier    ZoneTier
	}
	pairs := []pair{
		{ZoneCryptValdris, ZoneGoblinWarrens, ZoneTierBeginner},
		{ZoneForestShadows, ZoneSunkenTemple, ZoneTierApprentice},
		{ZoneManorBlackspire, ZoneUnderforge, ZoneTierJourneyman},
		{ZoneAbyssPortal, ZoneDragonsLair, ZoneTierLegendary},
	}

	type monsterStat struct {
		appearances    int
		standardCount  int
		eliteCount     int
		wins           int
		hpLossOnWin    int // sum, divided by wins at report time
		killAttributed int // last fight of a died_combat trial
	}
	type zoneAgg struct {
		profile      expeditionBalanceProfile
		completions  int
		deaths       int
		starves      int
		monsters     map[string]*monsterStat
		dayHist      [16]int // 1..14 + overflow bucket; index 0 unused
		eliteFights  int
		stdFights    int
		// for the trial currently in flight (mutated during traceFightStruct)
		lastFight harnessFightTrace
	}

	report := func(label string, agg *zoneAgg) {
		total := trialsPerZone
		compPct := float64(agg.completions) / float64(total) * 100
		deathPct := float64(agg.deaths) / float64(total) * 100
		starvePct := float64(agg.starves) / float64(total) * 100
		t.Logf("─── %-7s %-18s L%-2d  comp=%5.1f%%  death=%5.1f%%  starve=%5.1f%%  elite_fights=%d std_fights=%d ───",
			label, agg.profile.ZoneID,
			agg.profile.Level,
			compPct, deathPct, starvePct,
			agg.eliteFights, agg.stdFights)

		// Per-monster — sort by appearances desc for readability.
		names := make([]string, 0, len(agg.monsters))
		for n := range agg.monsters {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			return agg.monsters[names[i]].appearances > agg.monsters[names[j]].appearances
		})
		for _, n := range names {
			m := agg.monsters[n]
			winPct := 0.0
			if m.appearances > 0 {
				winPct = float64(m.wins) / float64(m.appearances) * 100
			}
			avgLoss := 0.0
			if m.wins > 0 {
				avgLoss = float64(m.hpLossOnWin) / float64(m.wins)
			}
			t.Logf("    MON  %-22s appear=%4d (std=%-3d elite=%-3d) win=%5.1f%% avg_hp_loss_on_win=%5.1f kills=%d",
				n, m.appearances, m.standardCount, m.eliteCount,
				winPct, avgLoss, m.killAttributed)
		}

		// Day-of-end histogram — squashes 14 into "14+" since the cap
		// terminates a trial at exactly day 14.
		bins := make([]string, 0, 14)
		for d := 1; d <= 14; d++ {
			if agg.dayHist[d] > 0 {
				bins = append(bins, fmt.Sprintf("d%d=%d", d, agg.dayHist[d]))
			}
		}
		t.Logf("    END-DAYS  %s", joinZones(bins))
	}

	for _, p := range pairs {
		level := phase1TierCenterline[p.tier]
		t.Logf("═══ T%d outlier=%s sibling=%s (L%d Fighter, e=%d d=%d burn=%d, %d trials each) ═══",
			p.tier, p.outlier, p.sibling, level,
			eliteThreshold, driftBase, supplyBurnPct, trialsPerZone)

		for _, zid := range []ZoneID{p.outlier, p.sibling} {
			profile := expeditionBalanceProfile{
				ZoneID:                          zid,
				Class:                           ClassFighter,
				Level:                           level,
				Supplies:                        makeSupplies(p.tier, SupplyPurchase{StandardPacks: 3}),
				CampType:                        CampTypeStandard,
				EliteInterruptThresholdOverride: eliteThreshold,
				ThreatDriftBaseOverride:         driftBase,
				SupplyBurnRatePctOverride:       supplyBurnPct,
			}
			agg := &zoneAgg{
				profile:  profile,
				monsters: map[string]*monsterStat{},
			}

			for trial := 0; trial < trialsPerZone; trial++ {
				exp := newHarnessExpedition(profile)
				char := buildHarnessCharacter(classBalanceProfile{
					Class: profile.Class,
					Level: profile.Level,
				})
				h := &expeditionHarness{
					exp:                             exp,
					char:                            char,
					rng:                             newHarnessRNG(baseSeed + uint64(trial) + uint64(p.tier)*131 + zoneSeedSalt(zid)),
					rollsPerDay:                     harnessHarvestRollsPerDay,
					eliteInterruptThresholdOverride: eliteThreshold,
					threatDriftBaseOverride:         driftBase,
					supplyBurnRatePctOverride:       supplyBurnPct,
					traceFightStruct: func(ft harnessFightTrace) {
						m, ok := agg.monsters[ft.MonsterName]
						if !ok {
							m = &monsterStat{}
							agg.monsters[ft.MonsterName] = m
						}
						m.appearances++
						if ft.Elite {
							m.eliteCount++
							agg.eliteFights++
						} else {
							m.standardCount++
							agg.stdFights++
						}
						if ft.Won {
							m.wins++
							loss := ft.HPPre - ft.HPPost
							if loss < 0 {
								loss = 0
							}
							m.hpLossOnWin += loss
						}
						agg.lastFight = ft
					},
				}
				var trialEnd expeditionTrialResult
				for {
					res := h.advanceExpeditionOneDay()
					if res.EndedReason != "" {
						trialEnd = res
						break
					}
				}
				switch {
				case trialEnd.Completed:
					agg.completions++
				case trialEnd.Died:
					agg.deaths++
					// Attribute the kill to the last fight's monster
					// (the one whose !PlayerWon return drove the
					// terminate call). Sound because the day loop
					// terminates immediately on combat loss.
					if m, ok := agg.monsters[agg.lastFight.MonsterName]; ok && !agg.lastFight.Won {
						m.killAttributed++
					}
				case trialEnd.StarvedOut:
					agg.starves++
				}
				d := trialEnd.DaysElapsed
				if d < 1 {
					d = 1
				}
				if d > 14 {
					d = 14
				}
				agg.dayHist[d]++
			}

			label := "OUTLIER"
			if zid == p.sibling {
				label = "SIBLING"
			}
			report(label, agg)
		}
	}
}

// TestExpeditionBalance_Phase5A_TierWideSensitivity is the tier-wide
// counterpart to Phase 4-A. Phase 4-B closed the four named per-zone
// outliers, but the b=50 sweep still left T2/T3/T5 sibling *pairs*
// below the target band as a group (T2 7-13% vs 62-82%; T3 3-14% vs
// 54-74%; T5 25-57% vs 36-56%). Both zones at each problem tier
// underperform together, so no further per-zone tool will close it —
// the lever has to be tier-wide. The plan doc names three candidate
// moves:
//
//   1. Gear-tier centerline remap (phase1TierCenterline)   → axis L
//   2. Per-tier elite-bracket threshold (vs global)        → axis E
//   3. Ship burn=75 globally (Phase 3-B's high-burn cell)  → axis B
//
// This diagnostic does *not* pick the lever. It runs a one-axis-at-a-
// time sensitivity sweep on the six under-band zones and lets the
// numbers name the lever. Each axis holds the other two at Phase 3-B
// best (e=23, d=1, burn=50) and walks the named axis through three
// values. Three points per axis is enough to read the slope: flat-line
// means the lever is inert for that tier, monotone climb means it's
// the lever, non-monotone means a confound.
//
//   L: player level    = {centerline-2, centerline, centerline+2}
//   E: elite threshold = {18, 23, 28}
//   B: supply burn pct = {40, 50, 60}
//
// Per cell we log comp%/death%/starve% plus the elite-vs-standard
// fight share (so axis E's slope can be cross-checked against the
// fight-mix shift it actually produced). 200 trials/cell × 6 zones ×
// 9 cells = 10.8k trials — heavier than Phase 4-A (1.6k) but bounded
// and skipped under -short.
//
// Diagnostic-only — no gates. Output names the lever Phase 5-B
// should pull.
func TestExpeditionBalance_Phase5A_TierWideSensitivity(t *testing.T) {
	if testing.Short() {
		t.Skip("phase 5-A sensitivity sweep walks 6 zones × 9 cells × 200 trials; -short skips it")
	}

	const trialsPerCell = 200
	const baseSeed uint64 = 0xF50A5E

	// Phase 3-B best cell (held constant on the two non-axis levers).
	const eliteBaseline = 23
	const driftBase = 1
	const burnBaseline = 50

	type tierGroup struct {
		tier  ZoneTier
		zones []ZoneID
	}
	groups := []tierGroup{
		{ZoneTierApprentice, []ZoneID{ZoneForestShadows, ZoneSunkenTemple}},
		{ZoneTierJourneyman, []ZoneID{ZoneManorBlackspire, ZoneUnderforge}},
		{ZoneTierLegendary, []ZoneID{ZoneAbyssPortal, ZoneDragonsLair}},
	}

	// runCell is the inner-loop closure: one (zone, lever-triple) cell.
	// Returns comp/death/starve counts plus elite/standard fight share
	// so the caller can attribute the slope across axis values.
	type cellOut struct {
		comp, death, starve int
		eliteFights, stdFights int
	}
	runCell := func(zid ZoneID, tier ZoneTier, level, elite, burn int) cellOut {
		out := cellOut{}
		profile := expeditionBalanceProfile{
			ZoneID:                          zid,
			Class:                           ClassFighter,
			Level:                           level,
			Supplies:                        makeSupplies(tier, SupplyPurchase{StandardPacks: 3}),
			CampType:                        CampTypeStandard,
			EliteInterruptThresholdOverride: elite,
			ThreatDriftBaseOverride:         driftBase,
			SupplyBurnRatePctOverride:       burn,
		}
		for trial := 0; trial < trialsPerCell; trial++ {
			exp := newHarnessExpedition(profile)
			char := buildHarnessCharacter(classBalanceProfile{
				Class: profile.Class,
				Level: profile.Level,
			})
			// Seed mixes trial, tier, zone, and the *axis values* so
			// cells across the sweep draw distinct RNG streams. Without
			// the lever mix-in, e.g. L=centerline cells would
			// byte-match across axes and the slope reading would be
			// degenerate on the shared row.
			seed := baseSeed + uint64(trial) +
				uint64(tier)*131 + zoneSeedSalt(zid) +
				uint64(level)*1_000_003 +
				uint64(elite)*7_919 +
				uint64(burn)*17
			h := &expeditionHarness{
				exp:                             exp,
				char:                            char,
				rng:                             newHarnessRNG(seed),
				rollsPerDay:                     harnessHarvestRollsPerDay,
				eliteInterruptThresholdOverride: elite,
				threatDriftBaseOverride:         driftBase,
				supplyBurnRatePctOverride:       burn,
				traceFightStruct: func(ft harnessFightTrace) {
					if ft.Elite {
						out.eliteFights++
					} else {
						out.stdFights++
					}
				},
			}
			var trialEnd expeditionTrialResult
			for {
				res := h.advanceExpeditionOneDay()
				if res.EndedReason != "" {
					trialEnd = res
					break
				}
			}
			switch {
			case trialEnd.Completed:
				out.comp++
			case trialEnd.Died:
				out.death++
			case trialEnd.StarvedOut:
				out.starve++
			}
		}
		return out
	}

	logRow := func(axis, label string, zid ZoneID, val int, c cellOut) {
		total := trialsPerCell
		t.Logf("    %s  %-18s %s=%-3d  comp=%5.1f%% death=%5.1f%% starve=%5.1f%%  elite_fights=%-4d std_fights=%-4d",
			axis, zid, label, val,
			float64(c.comp)/float64(total)*100,
			float64(c.death)/float64(total)*100,
			float64(c.starve)/float64(total)*100,
			c.eliteFights, c.stdFights)
	}

	for _, g := range groups {
		center := phase1TierCenterline[g.tier]
		t.Logf("═══ T%d zones=%v centerline=L%d (Fighter, %d trials/cell, baselines e=%d d=%d burn=%d) ═══",
			g.tier, g.zones, center, trialsPerCell,
			eliteBaseline, driftBase, burnBaseline)

		// Axis L — player level. Centerline ± 2 keeps the move inside
		// each tier's design-doc level range for T2/T3/T5 (T2: L3-7,
		// centerline 5 → 3,5,7; T3: L5-10, centerline 9 → 7,9,11; T5:
		// L10-20, centerline 17 → 15,17,19). No gear-tier boundary is
		// crossed within ±2 for any of these (boundaries 5/9/13/17),
		// so axis L isolates the "level inside same gear bracket"
		// sensitivity. Phase 5-B may widen this if the slope is flat
		// here but the boundary is the real lever.
		t.Logf("  AXIS-L  player level ±2 (gear-tier mapping sensitivity)")
		for _, zid := range g.zones {
			for _, dl := range []int{-2, 0, +2} {
				lvl := center + dl
				out := runCell(zid, g.tier, lvl, eliteBaseline, burnBaseline)
				logRow("L", "lvl", zid, lvl, out)
			}
		}

		// Axis E — elite threshold. 23 is the Phase 3-B baseline; 18
		// is "more elites" (lower bar → more interrupts roll elite);
		// 28 is "fewer elites". Slope here disambiguates whether the
		// tier is gated by elite mix vs baseline fights.
		t.Logf("  AXIS-E  elite threshold ±5 (monster-side gate)")
		for _, zid := range g.zones {
			for _, eth := range []int{18, 23, 28} {
				out := runCell(zid, g.tier, center, eth, burnBaseline)
				logRow("E", "eth", zid, eth, out)
			}
		}

		// Axis B — supply burn pct. Phase 3-B's negative result on
		// burn=75 was tier-mean; reading this axis per under-band
		// tier may surface a tier where burn is *the* lever (e.g. T5
		// borderline 25-57% may close on burn alone). 40/50/60 brackets
		// the Phase 3-B baseline and the candidate burn=75 direction.
		t.Logf("  AXIS-B  supply burn ±10pp (resource pressure)")
		for _, zid := range g.zones {
			for _, b := range []int{40, 50, 60} {
				out := runCell(zid, g.tier, center, eliteBaseline, b)
				logRow("B", "burn", zid, b, out)
			}
		}
	}
}

// zoneSeedSalt produces a stable per-zone seed offset so each
// (outlier, sibling) draws from a distinct RNG stream without
// hand-numbering. fnv-like fold over the bytes; small footprint, no
// import needed.
func zoneSeedSalt(id ZoneID) uint64 {
	var s uint64 = 1469598103934665603
	for i := 0; i < len(id); i++ {
		s ^= uint64(id[i])
		s *= 1099511628211
	}
	return s
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
