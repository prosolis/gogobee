package plugin

import "testing"

// TestOmenForWeek_Deterministic — the same ISO week always yields the same omen.
func TestOmenForWeek_Deterministic(t *testing.T) {
	a := omenForWeek(2026, 28)
	b := omenForWeek(2026, 28)
	if a.Key != b.Key {
		t.Fatalf("omenForWeek not deterministic: %q vs %q", a.Key, b.Key)
	}
}

// TestOmenForWeek_AdvancesEachWeek — consecutive weeks step by exactly one table
// entry, so the schedule rotates rather than sticking or skipping.
func TestOmenForWeek_AdvancesEachWeek(t *testing.T) {
	n := len(omenTable)
	for w := 1; w <= n; w++ {
		cur := omenForWeek(2026, w)
		next := omenForWeek(2026, w+1)
		wantNext := omenTable[((2026*53)+w+1)%n]
		if next.Key != wantNext.Key {
			t.Errorf("week %d→%d: got %q, want %q", w, w+1, next.Key, wantNext.Key)
		}
		if cur.Key == next.Key {
			t.Errorf("week %d and %d produced the same omen %q (should advance)", w, w+1, cur.Key)
		}
	}
}

// TestOmenForWeek_TotalOverYearBoundary — never panics across week/year edges,
// including ISO week 53.
func TestOmenForWeek_TotalOverYearBoundary(t *testing.T) {
	for y := 2020; y <= 2030; y++ {
		for w := 1; w <= 53; w++ {
			o := omenForWeek(y, w)
			if o.Key == "" {
				t.Fatalf("omenForWeek(%d,%d) returned zero omen", y, w)
			}
		}
	}
}

// TestOmenTable_NonCombatOnly — every omen carries at least one effect and the
// struct has no combat lever, so the launch set can never move the golden or the
// balance corpus. §B3.
func TestOmenTable_NonCombatOnly(t *testing.T) {
	for _, o := range omenTable {
		hasEffect := o.HarvestYieldBonus > 0 || o.SupplyFreebiePacks > 0 ||
			o.StartMoodBonus > 0 || o.ArenaPayoutMult > 1.0 ||
			o.ConsumableChanceMult > 1.0 || o.ThreatDriftReduce > 0
		if !hasEffect {
			t.Errorf("omen %q has no active effect", o.Key)
		}
		if o.Name == "" || o.TwinBee == "" {
			t.Errorf("omen %q missing player-facing copy", o.Key)
		}
	}
}

// TestActiveOmen_SimDisabled — the balance sim neutralizes the omen so corpus
// results are wall-clock-independent. §B3.
func TestActiveOmen_SimDisabled(t *testing.T) {
	defer func() { simOmenDisabled = false }()
	simOmenDisabled = true
	o := activeOmen()
	if o.Key != "none" {
		t.Errorf("sim-disabled omen = %q, want none", o.Key)
	}
	if o.HarvestYieldBonus != 0 || o.SupplyFreebiePacks != 0 || o.StartMoodBonus != 0 ||
		o.ArenaPayoutMult != 0 || o.ConsumableChanceMult != 0 || o.ThreatDriftReduce != 0 {
		t.Errorf("sim-disabled omen must have no effect, got %+v", o)
	}
}

// TestOmenKeysUnique — no duplicate keys (a dup would make the rotation land on
// the same omen two of every len(omenTable) weeks).
func TestOmenKeysUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, o := range omenTable {
		if seen[o.Key] {
			t.Errorf("duplicate omen key %q", o.Key)
		}
		seen[o.Key] = true
	}
}
