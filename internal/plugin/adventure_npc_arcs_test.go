package plugin

import (
	"strings"
	"testing"
)

// D2 NPC arcs — pure-helper coverage. None of these touch combat, so the
// characterization golden is unaffected; these just pin the arc thresholds,
// gift tiers, and the keepsake shape.

func TestMistyArcBeat_FiresOnlyAtThresholds(t *testing.T) {
	want := map[int]bool{5: true, 15: true, 30: true}
	for count := 0; count <= 40; count++ {
		got := mistyArcBeat(count)
		if want[count] {
			if got == "" {
				t.Errorf("encounter %d: expected an arc beat, got none", count)
			}
		} else if got != "" {
			t.Errorf("encounter %d: expected no arc beat, got %q", count, got)
		}
	}
}

func TestMistyArcBeats_DoNotLeakBuffMechanics(t *testing.T) {
	// The arena effects are a hidden discovery mechanic; arc copy must never
	// tie a donation to a combat outcome.
	banned := []string{"arena", "damage", "buff", "combat", "attack", "heal", "bonus"}
	for count, beat := range mistyArcBeats {
		lower := strings.ToLower(beat)
		for _, b := range banned {
			if strings.Contains(lower, b) {
				t.Errorf("beat %d leaks mechanic word %q: %s", count, b, beat)
			}
		}
	}
}

func TestRobbieGiftTier_MatchesArenaBands(t *testing.T) {
	cases := []struct {
		level int
		tier  int
	}{
		{1, 1}, {3, 1}, {4, 2}, {7, 2}, {8, 3}, {12, 3},
		{13, 4}, {17, 4}, {18, 5}, {20, 5},
	}
	for _, c := range cases {
		if got := robbieGiftTier(c.level); got != c.tier {
			t.Errorf("robbieGiftTier(%d) = %d, want %d", c.level, got, c.tier)
		}
	}
}

func TestRobbieGiftCadence(t *testing.T) {
	// The gift lands on every 10th visit and no other.
	for visit := 1; visit <= 35; visit++ {
		gives := visit%robbieGiftEveryNVisits == 0
		if gives != (visit == 10 || visit == 20 || visit == 30) {
			t.Errorf("visit %d: unexpected gift cadence", visit)
		}
	}
}

func TestThomPetTreat_IsRobbieSafeKeepsake(t *testing.T) {
	treat := thomPetTreatItem()
	if treat.Name == "" {
		t.Fatal("treat has no name")
	}
	// Robbie's sweep skips "card" items, so the keepsake survives him.
	if treat.Type != "card" {
		t.Errorf("treat type = %q, want card so Robbie's sweep skips it", treat.Type)
	}
	if treat.Value != 0 {
		t.Errorf("treat value = %d, want 0 (inert keepsake)", treat.Value)
	}
	// Confirm the sweep filter actually excludes it.
	if got := robbieQualifyingItems([]AdvItem{treat}, nil); len(got) != 0 {
		t.Errorf("Robbie would take the treat: %+v", got)
	}
}

func TestThomFinalLetterPet_SubstitutesPetName(t *testing.T) {
	letter := strings.ReplaceAll(thomFinalLetterPet, "{pet}", "Biscuit")
	if strings.Contains(letter, "{pet}") {
		t.Error("unsubstituted {pet} placeholder remains")
	}
	if !strings.Contains(letter, "Biscuit") {
		t.Error("pet name not substituted into the letter")
	}
}
