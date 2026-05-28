package plugin

import (
	"testing"
)

func TestAbilityModifier(t *testing.T) {
	cases := []struct {
		score, want int
	}{
		{1, -5},
		{8, -1},
		{9, -1},
		{10, 0},
		{11, 0},
		{12, 1},
		{13, 1},
		{14, 2},
		{15, 2},
		{16, 3},
		{18, 4},
		{20, 5},
	}
	for _, c := range cases {
		got := abilityModifier(c.score)
		if got != c.want {
			t.Errorf("abilityModifier(%d) = %d, want %d", c.score, got, c.want)
		}
	}
}

func TestIsStandardArray(t *testing.T) {
	good := [][]int{
		{15, 14, 13, 12, 10, 8},
		{8, 10, 12, 13, 14, 15},
		{14, 8, 15, 10, 13, 12},
	}
	for _, g := range good {
		var arr [6]int
		copy(arr[:], g)
		if !isStandardArray(arr) {
			t.Errorf("isStandardArray(%v) = false, want true", g)
		}
	}
	bad := [][]int{
		{15, 15, 13, 12, 10, 8}, // duplicate 15
		{16, 14, 13, 12, 10, 8}, // out-of-range
		{15, 14, 13, 12, 10, 9}, // 9 instead of 8
		{15, 14, 13, 12, 11, 8}, // 11 instead of 10
	}
	for _, ba := range bad {
		var arr [6]int
		copy(arr[:], ba)
		if isStandardArray(arr) {
			t.Errorf("isStandardArray(%v) = true, want false", ba)
		}
	}
}

func TestComputeMaxHP_FighterLevel1(t *testing.T) {
	// Fighter d10, CON +2 → L1 raw HP = 10 + 2 = 12; Phase 5-B
	// multiplies by phase5BHPMult (1.5, rounded), so → 18.
	got := computeMaxHP(ClassFighter, 2, 1)
	if got != 18 {
		t.Errorf("Fighter L1 (CON+2) = %d, want 18 (12 raw × phase5BHPMult)", got)
	}
}

func TestComputeMaxHP_MageLevel5(t *testing.T) {
	// Mage d6, CON +1
	// L1: 6 + 1 = 7
	// L2-5: 4 levels × (avg 4 + 1) = 4 × 5 = 20
	// Raw total: 27; Phase 5-B × casterHPMult: 27 × 1.5 × 1.25 = 50.625 → 51.
	got := computeMaxHP(ClassMage, 1, 5)
	if got != 51 {
		t.Errorf("Mage L5 (CON+1) = %d, want 51 (27 raw × phase5BHPMult × casterHPMult)", got)
	}
}

func TestComputeMaxHP_FloorAt1(t *testing.T) {
	// Pathological: very negative CON, low level → still ≥1 per level
	got := computeMaxHP(ClassMage, -5, 1)
	if got < 1 {
		t.Errorf("HP floor violated: got %d", got)
	}
}

func TestComputeAC(t *testing.T) {
	cases := []struct {
		class  DnDClass
		dexMod int
		want   int
	}{
		{ClassFighter, 0, 16},  // 10 + 0 + 6
		{ClassFighter, 2, 18},  // 10 + 2 + 6
		{ClassRogue, 3, 14},    // 10 + 3 + 1
		{ClassMage, 0, 12},     // 10 + 0 + 2 (D8-d-fix caster floor)
		{ClassSorcerer, 1, 13}, // 10 + 1 + 2
		{ClassCleric, 1, 16},   // 10 + 1 + 5 (D8-d-fix caster floor)
		{ClassDruid, 0, 15},    // 10 + 0 + 5
		{ClassBard, 2, 15},     // 10 + 2 + 3 (D8-d-fix caster floor)
		{ClassWarlock, 0, 13},  // 10 + 0 + 3
		{ClassRanger, 2, 15},   // 10 + 2 + 3
	}
	for _, c := range cases {
		got := computeAC(c.class, c.dexMod)
		if got != c.want {
			t.Errorf("computeAC(%s, dex%+d) = %d, want %d", c.class, c.dexMod, got, c.want)
		}
	}
}

func TestApplyRaceMods(t *testing.T) {
	// Elf: STR +0, DEX +3, CON -1, INT +2, WIS +3, CHA +0
	base := [6]int{10, 10, 10, 10, 10, 10}
	got := applyRaceMods(RaceElf, base)
	want := [6]int{10, 13, 9, 12, 13, 10}
	if got != want {
		t.Errorf("applyRaceMods(Elf) = %v, want %v", got, want)
	}
	// Orc: STR +6, DEX -1, CON +3, INT -1, WIS -1, CHA +0
	got = applyRaceMods(RaceOrc, base)
	want = [6]int{16, 9, 13, 9, 9, 10}
	if got != want {
		t.Errorf("applyRaceMods(Orc) = %v, want %v", got, want)
	}
}

// TestRaceBalance runs the weighted balance pass and logs the report.
// Standard Human baseline is 6.0 under every class; a race's best-fit
// score is its realistic effective-power ceiling. The assertion is a
// generous guard rail — see classStatWeights for the model.
func TestRaceBalance(t *testing.T) {
	report := computeRaceBalance()
	t.Logf("weighted race-balance pass (Human baseline = %.1f)", raceBalanceBaseline)
	t.Logf("%-10s %-9s %6s  %-9s %6s  %6s  %+6s",
		"race", "best-fit", "score", "worst-fit", "score", "avg", "Δ")
	for _, rb := range report {
		t.Logf("%-10s %-9s %6.2f  %-9s %6.2f  %6.2f  %+6.2f",
			rb.Race, rb.BestClass, rb.BestScore,
			rb.WorstClass, rb.WorstScore, rb.AvgScore, rb.Delta())
	}
	// Balance rule: equal *average* power. Every race's mean score across
	// all playable classes must land within tolerance of the Human
	// baseline. Best-fit/worst-fit spread is intentional race identity —
	// a spiky race trades a higher ceiling for a lower floor — so only
	// the average is asserted.
	const tolerance = 0.5
	for _, rb := range report {
		if d := rb.AvgScore - raceBalanceBaseline; d < -tolerance || d > tolerance {
			t.Errorf("%s avg %.2f is %.2f off the %.1f baseline (tolerance %.1f)",
				rb.Race, rb.AvgScore, d, raceBalanceBaseline, tolerance)
		}
	}
}

func TestParseRaceClass(t *testing.T) {
	if r, ok := parseRace("Elf"); !ok || r != RaceElf {
		t.Errorf("parseRace(Elf) = %v, %v", r, ok)
	}
	if r, ok := parseRace("half-elf"); !ok || r != RaceHalfElf {
		t.Errorf("parseRace(half-elf) = %v, %v", r, ok)
	}
	if _, ok := parseRace("dragonborn"); ok {
		t.Errorf("parseRace(dragonborn) = ok, want false")
	}
	if c, ok := parseClass("Fighter"); !ok || c != ClassFighter {
		t.Errorf("parseClass(Fighter) = %v, %v", c, ok)
	}
	if _, ok := parseClass("monk"); ok {
		t.Errorf("parseClass(monk) = ok, want false")
	}
}

func TestStatsAssigned(t *testing.T) {
	defaultC := &DnDCharacter{STR: 8, DEX: 8, CON: 8, INT: 8, WIS: 8, CHA: 8}
	if statsAssigned(defaultC) {
		t.Error("statsAssigned(all-8s) = true, want false")
	}
	withStats := &DnDCharacter{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8}
	if !statsAssigned(withStats) {
		t.Error("statsAssigned(real stats) = false, want true")
	}
}

func TestParseStatsArg(t *testing.T) {
	want := [6]int{15, 14, 13, 12, 10, 8}
	cases := []string{
		"15 14 13 12 10 8",
		"15, 14, 13, 12, 10, 8",
		"(15, 14, 13, 12, 10, 8)",
		"[15,14,13,12,10,8]",
		"{15 14 13 12 10 8}",
		"  15,14 ,13, 12 ,10,  8  ",
	}
	for _, in := range cases {
		got, err := parseStatsArg(in)
		if err != nil {
			t.Errorf("parseStatsArg(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseStatsArg(%q) = %v, want %v", in, got, want)
		}
	}

	bad := []string{
		"",                       // empty
		"15 14 13 12 10",         // 5 numbers
		"15 14 13 12 10 8 7",     // 7 numbers
		"15 14 13 12 10 abc",     // non-number
		"banana",                 // garbage
	}
	for _, in := range bad {
		if _, err := parseStatsArg(in); err == nil {
			t.Errorf("parseStatsArg(%q) should have errored", in)
		}
	}

	// A different permutation should still parse and just return the
	// numbers in order — validation against the standard array is
	// a separate concern (isStandardArray).
	got, err := parseStatsArg("8, 10, 12, 13, 14, 15")
	if err != nil {
		t.Fatal(err)
	}
	if got != [6]int{8, 10, 12, 13, 14, 15} {
		t.Errorf("permutation order not preserved: %v", got)
	}
}

func TestDnDLevelFromCombatLevel(t *testing.T) {
	cases := []struct{ combat, want int }{
		{0, 1},   // floor
		{1, 1},
		{4, 1},
		{5, 1},
		{9, 1},
		{10, 2},
		{15, 3},
		{20, 4},  // nonk
		{24, 4},  // quack
		{25, 5},
		{28, 5},  // prosolis
		{30, 6},
		{45, 9},
		{49, 9},  // holymachina
		{50, 10},
		{99, 19},
		{100, 20}, // clamp
		{500, 20}, // clamp
	}
	for _, c := range cases {
		got := dndLevelFromCombatLevel(c.combat)
		if got != c.want {
			t.Errorf("dndLevelFromCombatLevel(%d) = %d, want %d", c.combat, got, c.want)
		}
	}
}

func TestModifiersOnCharacter(t *testing.T) {
	c := &DnDCharacter{STR: 16, DEX: 12, CON: 14, INT: 10, WIS: 8, CHA: 18}
	mods := c.Modifiers()
	want := [6]int{3, 1, 2, 0, -1, 4}
	if mods != want {
		t.Errorf("Modifiers() = %v, want %v", mods, want)
	}
}
