package plugin

import (
	"fmt"
	"sort"
	"testing"
)

// fmtSpread renders a (min, max) winrate cell for the Phase-2 spread table.
// "min..max (Δpp)" with Δ in percentage points. Cells where every class is
// within 5pp render as "balanced" so the eye skips them.
func fmtSpread(minV, maxV float64) string {
	delta := maxV - minV
	return fmt.Sprintf("%.2f..%.2f (%2dpp)", minV, maxV, int(delta*100+0.5))
}

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

// Phase 1 — full matrix measurement. Per gogobee_class_balance.md §5
// Phase 1: "Generalize to all 10 classes × 30 subclasses; TestClassBalance
// logs the full report. No tuning yet — just measurement."
//
// This test does not assert balance. The only failures it catches are
// harness-broken pathologies — a profile that's 0% at T1 across the board
// (build can't damage anything), or an L1-pre-subclass build that's 100%
// at T5 (monster scaling collapsed). Per-tier parity bands land in Phase 2
// once we have data to calibrate the tolerance.
//
// Skipped under -short. 190 profiles × 5 tiers × 200 trials = 190k
// simulated fights; runs in a few seconds.
func TestClassBalance_Phase1_FullMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("phase-1 matrix — measurement only")
	}

	profiles := buildPhase1Profiles()
	const trials = 200
	results := runClassBalanceMatrix(profiles, trials)

	// Index results for table layout: rows = (class, subclass, level),
	// columns = tier. Group by class so the log reads class-by-class.
	type rowKey struct {
		Class    DnDClass
		Subclass DnDSubclass
		Level    int
	}
	rows := make(map[rowKey]map[int]classBalanceResult, len(profiles))
	for _, r := range results {
		k := rowKey{r.Profile.Class, r.Profile.Subclass, r.Profile.Level}
		if rows[k] == nil {
			rows[k] = make(map[int]classBalanceResult, 5)
		}
		rows[k][r.Tier] = r
	}

	t.Logf("class-balance Phase 1 — full matrix, %d trials/cell", trials)
	t.Logf("%-10s %-18s %-3s   T1     T2     T3     T4     T5", "class", "subclass", "lvl")

	// Per-tier accumulators for a tail summary — mean win rate by class
	// across all of its rows at each tier, plus the cross-class spread.
	type tierAgg struct {
		sum    float64
		count  int
		minVal float64
		maxVal float64
	}
	classTier := make(map[DnDClass]map[int]*tierAgg)
	for _, ci := range dndClasses {
		classTier[ci.Key] = map[int]*tierAgg{
			1: {minVal: 1}, 2: {minVal: 1}, 3: {minVal: 1},
			4: {minVal: 1}, 5: {minVal: 1},
		}
	}

	for _, ci := range dndClasses {
		// pre-subclass rows first, then each subclass's L5+ rows.
		for _, lvl := range phase1PreSubclassLevels {
			row := rows[rowKey{ci.Key, "", lvl}]
			t.Logf("%-10s %-18s %-3d   %.3f  %.3f  %.3f  %.3f  %.3f",
				ci.Key, "—", lvl,
				row[1].WinRate(), row[2].WinRate(), row[3].WinRate(),
				row[4].WinRate(), row[5].WinRate())
			for tier := 1; tier <= 5; tier++ {
				ta := classTier[ci.Key][tier]
				wr := row[tier].WinRate()
				ta.sum += wr
				ta.count++
				if wr < ta.minVal {
					ta.minVal = wr
				}
				if wr > ta.maxVal {
					ta.maxVal = wr
				}
			}
		}
		for _, si := range subclassesForClass(ci.Key) {
			for _, lvl := range phase1SubclassLevels {
				row := rows[rowKey{ci.Key, si.ID, lvl}]
				t.Logf("%-10s %-18s %-3d   %.3f  %.3f  %.3f  %.3f  %.3f",
					ci.Key, si.ID, lvl,
					row[1].WinRate(), row[2].WinRate(), row[3].WinRate(),
					row[4].WinRate(), row[5].WinRate())
				for tier := 1; tier <= 5; tier++ {
					ta := classTier[ci.Key][tier]
					wr := row[tier].WinRate()
					ta.sum += wr
					ta.count++
					if wr < ta.minVal {
						ta.minVal = wr
					}
					if wr > ta.maxVal {
						ta.maxVal = wr
					}
				}
			}
		}
	}

	// Per-class summary: mean win rate per tier, sorted by overall mean
	// (lowest first). Useful at a glance to spot the outliers Phase 2 will
	// tune.
	t.Logf("")
	t.Logf("per-class mean win rate by tier (range in brackets):")
	t.Logf("%-10s   T1            T2            T3            T4            T5", "class")
	classKeys := make([]DnDClass, 0, len(dndClasses))
	for _, ci := range dndClasses {
		classKeys = append(classKeys, ci.Key)
	}
	overall := func(c DnDClass) float64 {
		var s float64
		for tier := 1; tier <= 5; tier++ {
			ta := classTier[c][tier]
			if ta.count > 0 {
				s += ta.sum / float64(ta.count)
			}
		}
		return s
	}
	sort.SliceStable(classKeys, func(i, j int) bool {
		return overall(classKeys[i]) < overall(classKeys[j])
	})
	for _, c := range classKeys {
		t.Logf("%-10s   %.2f [%.2f-%.2f]  %.2f [%.2f-%.2f]  %.2f [%.2f-%.2f]  %.2f [%.2f-%.2f]  %.2f [%.2f-%.2f]",
			c,
			classTier[c][1].sum/float64(classTier[c][1].count), classTier[c][1].minVal, classTier[c][1].maxVal,
			classTier[c][2].sum/float64(classTier[c][2].count), classTier[c][2].minVal, classTier[c][2].maxVal,
			classTier[c][3].sum/float64(classTier[c][3].count), classTier[c][3].minVal, classTier[c][3].maxVal,
			classTier[c][4].sum/float64(classTier[c][4].count), classTier[c][4].minVal, classTier[c][4].maxVal,
			classTier[c][5].sum/float64(classTier[c][5].count), classTier[c][5].minVal, classTier[c][5].maxVal,
		)
	}

	// Phase-2 diagnostic: cross-class spread at each (level, tier) cell.
	// Within a level row, average subclasses per class (pre-subclass levels
	// have no subclass dimension so the "average" is the single cell). The
	// per-class-mean summary above is dominated by floor+ceiling saturation
	// (L1-4 at high tier ≈ 0 for casters; L10+ at all tiers ≈ 1 for everyone),
	// which masks the actual gaps Phase 2 needs to close. This view shows the
	// max-min spread per (level, tier) — the cells with the largest spread
	// are the ones to tune.
	allLevels := append(append([]int{}, phase1PreSubclassLevels...), phase1SubclassLevels...)
	t.Logf("")
	t.Logf("per-(level, tier) cross-class spread — class winrate is mean over subclasses (or single cell pre-L5):")
	t.Logf("%-5s   T1                T2                T3                T4                T5", "lvl")
	for _, lvl := range allLevels {
		var line string
		for tier := 1; tier <= 5; tier++ {
			minV, maxV := 1.0, 0.0
			for _, ci := range dndClasses {
				var sum float64
				var n int
				if lvl < 5 {
					if r, ok := rows[rowKey{ci.Key, "", lvl}]; ok {
						sum = r[tier].WinRate()
						n = 1
					}
				} else {
					for _, si := range subclassesForClass(ci.Key) {
						if r, ok := rows[rowKey{ci.Key, si.ID, lvl}]; ok {
							sum += r[tier].WinRate()
							n++
						}
					}
				}
				if n == 0 {
					continue
				}
				wr := sum / float64(n)
				if wr < minV {
					minV = wr
				}
				if wr > maxV {
					maxV = wr
				}
			}
			line += " " + fmtSpread(minV, maxV)
		}
		t.Logf("L%-4d %s", lvl, line)
	}

	// Phase-3 diagnostic — off-tier trailer per (level, tier). For each cell
	// where the level is *below* the in-tier band (e.g. L1 at T2-T5), print the
	// trailing class and its win rate. This is what Phase 3 lifts: the casual
	// player who walks into a slightly-too-hard dungeon underleveled. Not
	// asserted — Phase 3 tunes against the diagnostic numbers and the next
	// phase decides whether to lock a soft off-tier band.
	offTierLevels := map[int][]int{
		2: {1, 2},
		3: {1, 2, 3, 4},
		4: {1, 2, 3, 4, 5},
		5: {1, 2, 3, 4, 5, 7},
	}
	t.Logf("")
	t.Logf("off-tier trailer per (level, tier) — class winrate is mean over subclasses (or single cell pre-L5):")
	t.Logf("%-5s   T2                T3                T4                T5", "lvl")
	for _, lvl := range allLevels {
		var line string
		for tier := 2; tier <= 5; tier++ {
			levels := offTierLevels[tier]
			matched := false
			for _, l := range levels {
				if l == lvl {
					matched = true
					break
				}
			}
			if !matched {
				line += " " + "       —         "
				continue
			}
			var trailerClass DnDClass
			minV := 1.01
			for _, ci := range dndClasses {
				var sum float64
				var n int
				if lvl < 5 {
					if r, ok := rows[rowKey{ci.Key, "", lvl}]; ok {
						sum = r[tier].WinRate()
						n = 1
					}
				} else {
					for _, si := range subclassesForClass(ci.Key) {
						if r, ok := rows[rowKey{ci.Key, si.ID, lvl}]; ok {
							sum += r[tier].WinRate()
							n++
						}
					}
				}
				if n == 0 {
					continue
				}
				wr := sum / float64(n)
				if wr < minV {
					minV, trailerClass = wr, ci.Key
				}
			}
			line += fmt.Sprintf(" %-10s %.2f  ", trailerClass, minV)
		}
		t.Logf("L%-4d %s", lvl, line)
	}

	// Harness-broken gates first.
	for _, r := range results {
		if r.Tier == 1 && r.WinRate() == 0 {
			t.Errorf("%s/%s L%d T1 win rate is 0%% — the build can't damage anything; loadout or spell policy is dead",
				r.Profile.Class, r.Profile.Subclass, r.Profile.Level)
		}
		if r.Tier == 5 && r.Profile.Level == 1 && r.WinRate() == 1 {
			t.Errorf("%s L1 T5 win rate is 100%% — monster scaling looks broken", r.Profile.Class)
		}
	}

	// Phase 2 parity band — locked at 35pp cross-class spread for in-tier
	// cells (the (level, tier) pairs where every class is "level-appropriate"
	// for the tier). Off-tier cells — L1 mage at T5, L1 fighter at T5 etc. —
	// aren't asserted: those are level-vs-tier mismatches, and casters at L1-4
	// cannot muscle through a T2-T3 monster on a single low-slot spell + a
	// quarterstaff the way martials muscle through with weapon dice. They
	// stay in the diagnostic log above.
	//
	// In-tier ranges below are calibrated empirically from the post-Phase-2
	// matrix: each cell's mean and spread is informative (not pinned to 0 or
	// 1), and a 35pp band gives Monte-Carlo headroom (~5pp at 200 trials/cell)
	// over the 29pp worst in-tier spread the tuned harness produces.
	inTierLevels := map[int][]int{
		1: {1, 2, 3, 4},
		2: {3, 4, 5, 7},
		3: {5, 7, 10},
		4: {7, 10, 15},
		5: {10, 15, 20},
	}
	const parityBandPP = 35
	for tier := 1; tier <= 5; tier++ {
		for _, lvl := range inTierLevels[tier] {
			minV, maxV := 1.0, 0.0
			var leader, trailer DnDClass
			for _, ci := range dndClasses {
				var sum float64
				var n int
				if lvl < 5 {
					if r, ok := rows[rowKey{ci.Key, "", lvl}]; ok {
						sum = r[tier].WinRate()
						n = 1
					}
				} else {
					for _, si := range subclassesForClass(ci.Key) {
						if r, ok := rows[rowKey{ci.Key, si.ID, lvl}]; ok {
							sum += r[tier].WinRate()
							n++
						}
					}
				}
				if n == 0 {
					continue
				}
				wr := sum / float64(n)
				if wr < minV {
					minV, trailer = wr, ci.Key
				}
				if wr > maxV {
					maxV, leader = wr, ci.Key
				}
			}
			spread := int((maxV-minV)*100 + 0.5)
			if spread > parityBandPP {
				t.Errorf("in-tier parity violated at L%d/T%d: spread %dpp > band %dpp (leader %s %.2f, trailer %s %.2f)",
					lvl, tier, spread, parityBandPP, leader, maxV, trailer, minV)
			}
		}
	}
}
