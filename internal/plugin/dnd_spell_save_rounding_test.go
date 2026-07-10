package plugin

import "testing"

// Phase 9 SP4 — save-half rounding edge cases.

func TestHalveSavedDamage_Boundaries(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1},   // 0 dmg saved → 1 (floor kicks in even on no-damage spells)
		{1, 1},   // 1 / 2 = 0 → floor 1
		{2, 1},   // 2 / 2 = 1
		{3, 1},   // 3 / 2 = 1 (rounds down)
		{10, 5},  // even
		{11, 5},  // odd rounds down
		{99, 49}, // large odd
		{100, 50},
	}
	for _, c := range cases {
		if got := halveSavedDamage(c.in); got != c.want {
			t.Errorf("halveSavedDamage(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestEnemySpellSaveMod_Scales — heuristic save mod is enemy.AttackBonus / 2.
// Sanity-check that T1-ish (+2..+4) → +1..+2 and T5-ish (+10..+12) → +5..+6.
func TestEnemySpellSaveMod_Scales(t *testing.T) {
	cases := []struct {
		atk  int
		want int
	}{
		{0, 0},   // unstatted enemy
		{2, 1},   // T1
		{4, 2},   // T2
		{6, 3},   // T3
		{8, 4},   // T4
		{10, 5},  // T5 floor
		{12, 6},  // T5 boss
		{20, 10}, // outsized boss
	}
	for _, c := range cases {
		enemy := &CombatStats{AttackBonus: c.atk}
		if got := enemySpellSaveMod(enemy); got != c.want {
			t.Errorf("enemySpellSaveMod(atk=%d) = %d, want %d", c.atk, got, c.want)
		}
	}
	// Monotonic non-decreasing across the range: a stronger enemy should never
	// have a worse save mod than a weaker one.
	prev := -1
	for atk := 0; atk <= 20; atk++ {
		got := enemySpellSaveMod(&CombatStats{AttackBonus: atk})
		if got < prev {
			t.Errorf("save mod regressed at atk=%d: %d < prev %d", atk, got, prev)
		}
		prev = got
	}
}

// TestEnemySpellSaveMod_NilSafe — defensive: nil enemy yields 0 (callers
// should never pass nil, but the helper shouldn't panic).
func TestEnemySpellSaveMod_NilSafe(t *testing.T) {
	if got := enemySpellSaveMod(nil); got != 0 {
		t.Errorf("nil enemy save mod = %d, want 0", got)
	}
}
