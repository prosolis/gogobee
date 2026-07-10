package plugin

import (
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func TestWellRestedTempHP_ByTier(t *testing.T) {
	cases := []struct {
		tier, hpMax, want int
	}{
		{0, 100, 0},  // no house
		{1, 100, 0},  // tier-1 shack: nothing
		{2, 100, 8},  // 8%
		{3, 100, 12}, // 12%
		{4, 100, 16}, // 16%
		{2, 3, 1},    // rounds to <1 → floored to 1
	}
	for _, c := range cases {
		if got := wellRestedTempHP(c.hpMax, c.tier); got != c.want {
			t.Errorf("wellRestedTempHP(hpMax=%d, tier=%d) = %d, want %d", c.hpMax, c.tier, got, c.want)
		}
	}
}

func TestWellRestedSlotBonus_PlacesAtHighestLevel(t *testing.T) {
	pool := map[int]int{1: 4, 2: 3, 3: 2} // highest available = 3

	if got := wellRestedSlotBonus(1, pool); got != nil {
		t.Errorf("tier 1 → %v, want nil", got)
	}
	if got := wellRestedSlotBonus(0, pool); got != nil {
		t.Errorf("tier 0 → %v, want nil", got)
	}
	if got := wellRestedSlotBonus(3, nil); got != nil {
		t.Errorf("non-caster (nil pool) → %v, want nil", got)
	}

	for tier, wantN := range map[int]int{2: 1, 3: 2, 4: 3} {
		got := wellRestedSlotBonus(tier, pool)
		if len(got) != 1 || got[3] != wantN {
			t.Errorf("tier %d → %v, want {3: %d}", tier, got, wantN)
		}
	}
}

func TestApplyDnDHPScaling_TempHPCushion(t *testing.T) {
	// Golden-safety: TempHP==0 must leave a full-HP character untouched
	// (StartHP stays 0 = "enter at MaxHP", MaxHP unchanged).
	stats := CombatStats{MaxHP: 100, HPBonus: 10}
	full := &DnDCharacter{HPMax: 100, HPCurrent: 100, TempHP: 0}
	applyDnDHPScaling(&stats, full)
	if stats.MaxHP != 100 || stats.StartHP != 0 {
		t.Fatalf("TempHP=0 full HP: MaxHP=%d StartHP=%d, want 100/0", stats.MaxHP, stats.StartHP)
	}

	// Full HP + cushion: MaxHP grows by TempHP, StartHP stays the sentinel so
	// combat enters at the bumped max (full + cushion).
	stats = CombatStats{MaxHP: 100, HPBonus: 10}
	rested := &DnDCharacter{HPMax: 100, HPCurrent: 100, TempHP: 15}
	applyDnDHPScaling(&stats, rested)
	if stats.MaxHP != 115 || stats.StartHP != 0 {
		t.Fatalf("full HP + cushion: MaxHP=%d StartHP=%d, want 115/0", stats.MaxHP, stats.StartHP)
	}

	// Wounded + cushion: entry HP is current + gear bonus + cushion, MaxHP grows.
	stats = CombatStats{MaxHP: 100, HPBonus: 10}
	wounded := &DnDCharacter{HPMax: 100, HPCurrent: 40, TempHP: 15}
	applyDnDHPScaling(&stats, wounded)
	if stats.MaxHP != 115 || stats.StartHP != 65 { // 40 + 10 + 15
		t.Fatalf("wounded + cushion: MaxHP=%d StartHP=%d, want 115/65", stats.MaxHP, stats.StartHP)
	}
}

func TestApplyLongRestSpellSlots_GrantAndExpiry(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	uid := id.UserID("@rested_caster:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassCleric, Level: 5,
		STR: 10, DEX: 12, CON: 14, INT: 10, WIS: 16, CHA: 10,
	}
	c.HPMax = computeMaxHP(c.Class, abilityModifier(c.CON), c.Level)
	c.HPCurrent = c.HPMax
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}
	if err := setSpellSlotsForCharacter(c); err != nil {
		t.Fatalf("setSpellSlotsForCharacter: %v", err)
	}

	base, _ := getSpellSlots(uid)
	if len(base) == 0 {
		t.Fatal("cleric L5 should have a base slot pool")
	}
	highest := 0
	for lvl := range base {
		if lvl > highest {
			highest = lvl
		}
	}

	// Home rest at tier 4 → +3 at highest level, used reset to 0.
	bonus, err := applyLongRestSpellSlots(c, 4)
	if err != nil {
		t.Fatalf("applyLongRestSpellSlots(tier4): %v", err)
	}
	if bonus[highest] != 3 {
		t.Fatalf("tier-4 bonus = %v, want {%d: 3}", bonus, highest)
	}
	after, _ := getSpellSlots(uid)
	if after[highest][0] != base[highest][0]+3 {
		t.Errorf("highest total = %d, want base+3 = %d", after[highest][0], base[highest][0]+3)
	}
	for lvl, pair := range after {
		if pair[1] != 0 {
			t.Errorf("slot L%d used = %d after rest, want 0", lvl, pair[1])
		}
	}

	// Next rest at the inn (tier 0) → bonus expires, totals back to base.
	if _, err := applyLongRestSpellSlots(c, 0); err != nil {
		t.Fatalf("applyLongRestSpellSlots(tier0): %v", err)
	}
	reset, _ := getSpellSlots(uid)
	for lvl, pair := range reset {
		if pair[0] != base[lvl][0] {
			t.Errorf("L%d total = %d after expiry, want base %d", lvl, pair[0], base[lvl][0])
		}
	}

	// Tier-1 house grants nothing.
	if bonus, _ := applyLongRestSpellSlots(c, 1); bonus != nil {
		t.Errorf("tier-1 bonus = %v, want nil", bonus)
	}
}

func TestApplyLongRestSpellSlots_NonCaster(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	uid := id.UserID("@rested_fighter:example")
	c := &DnDCharacter{UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5, CON: 14}
	c.HPMax = computeMaxHP(c.Class, abilityModifier(c.CON), c.Level)
	c.HPCurrent = c.HPMax
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	bonus, err := applyLongRestSpellSlots(c, 4)
	if err != nil {
		t.Fatalf("applyLongRestSpellSlots: %v", err)
	}
	if bonus != nil {
		t.Errorf("non-caster bonus = %v, want nil", bonus)
	}
	if slots, _ := getSpellSlots(uid); len(slots) != 0 {
		t.Errorf("non-caster has %d slot rows, want 0", len(slots))
	}
}

// TestHomeWorkshop pins the T3 workshop craft bonus and its effect on the
// success-rate cap.
func TestHomeWorkshop(t *testing.T) {
	for tier, want := range map[int]float64{0: 0, 1: 0, 2: 0, 3: 0.05, 4: 0.05} {
		if got := homeWorkshopCraftBonus(tier); got != want {
			t.Errorf("homeWorkshopCraftBonus(%d) = %.2f, want %.2f", tier, got, want)
		}
	}
	// No workshop: base cap holds at 0.95 for a maxed forager.
	if got := craftingSuccessRate(100, 1, 0); got != 0.95 {
		t.Errorf("maxed forager, no workshop = %.3f, want 0.95", got)
	}
	// Workshop lifts the cap to 0.98.
	if got := craftingSuccessRate(100, 1, 0.05); got != 0.98 {
		t.Errorf("maxed forager, workshop = %.3f, want 0.98", got)
	}
	// Mid-level forager gets the flat +5% added below the cap.
	base := craftingSuccessRate(15, 10, 0)
	withShop := craftingSuccessRate(15, 10, 0.05)
	if diff := withShop - base; diff < 0.049 || diff > 0.051 {
		t.Errorf("workshop delta = %.3f, want ~0.05", diff)
	}
}
