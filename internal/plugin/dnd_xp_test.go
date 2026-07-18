package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func TestDnDXPToNextLevel(t *testing.T) {
	cases := []struct{ level, want int }{
		{1, 300},    // L1→L2: 300
		{2, 600},    // L2→L3: 900-300
		{4, 1000},   // L4→L5: 2700-1700
		{6, 2200},   // L6→L7: 6500-4300
		{19, 11000}, // L19→L20: 85000-74000
		{20, 0},     // capped
		{0, 300},    // clamp to 1
	}
	for _, c := range cases {
		if got := dndXPToNextLevel(c.level); got != c.want {
			t.Errorf("dndXPToNextLevel(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestDnDXPTableMonotonic(t *testing.T) {
	// Each level threshold must strictly exceed the previous.
	for i := 1; i < len(dndXPTable); i++ {
		if dndXPTable[i] <= dndXPTable[i-1] {
			t.Errorf("dndXPTable[%d]=%d not > dndXPTable[%d]=%d",
				i, dndXPTable[i], i-1, dndXPTable[i-1])
		}
	}
}

func TestArenaCombatXP(t *testing.T) {
	winLow := arenaCombatXP(CombatResult{PlayerWon: true}, 1)
	if winLow < 30 {
		t.Errorf("low-threat win XP = %d, want ≥ 30 (floor)", winLow)
	}
	winHigh := arenaCombatXP(CombatResult{PlayerWon: true}, 20)
	if winHigh != 240 {
		t.Errorf("threat-20 win XP = %d, want 240", winHigh)
	}
	winND := arenaCombatXP(CombatResult{PlayerWon: true, NearDeath: true}, 20)
	if winND != 300 { // 240 * 1.25
		t.Errorf("threat-20 near-death win XP = %d, want 300", winND)
	}
	loss := arenaCombatXP(CombatResult{PlayerWon: false}, 20)
	if loss != 60 { // 240 * 0.25
		t.Errorf("threat-20 loss XP = %d, want 60", loss)
	}
}

func TestDungeonCombatXP(t *testing.T) {
	if got := dungeonCombatXP(CombatResult{PlayerWon: true}, 1); got != 30 {
		t.Errorf("T1 win XP = %d, want 30 (floor)", got)
	}
	if got := dungeonCombatXP(CombatResult{PlayerWon: true}, 5); got != 750 {
		t.Errorf("T5 win XP = %d, want 750", got)
	}
}

// TestGrantDnDXP_NoOpOnMissingChar — the function should silently no-op
// when the player has no D&D character.
func TestGrantDnDXP_NoOpOnMissingChar(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	p := &AdventurePlugin{}
	events, err := p.grantDnDXP(id.UserID("@nobody:nowhere.invalid"), 1000)
	if err != nil {
		t.Fatalf("grantDnDXP on missing char: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events on missing char, want 0", len(events))
	}
}

// TestGrantDnDXP_LevelUpCascade — grant a huge XP amount and verify that
// multiple level-ups happen and HP/level/AC update consistently.
func TestGrantDnDXP_LevelUpCascade(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	// Build a fresh L1 D&D char in the test DB.
	uid := id.UserID("@xp_test:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 1,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
		ArmorClass: 16,
	}
	conMod := abilityModifier(c.CON)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	// Grant 3000 XP — should push from L1 to L4 (300+600+900=1800 < 3000 < 2700+900=3600).
	// L1→L2: 300, total used 300, remaining 2700
	// L2→L3: 600, total used 900, remaining 2100
	// L3→L4: 800 (1700-900), total used 1700, remaining 1300
	// L4→L5: 1000 (2700-1700), total used 2700, remaining 300
	// Should land at L5 with 300 XP carryover.
	p := &AdventurePlugin{}
	events, err := p.grantDnDXP(uid, 3000)
	if err != nil {
		t.Fatalf("grantDnDXP: %v", err)
	}
	if len(events) != 4 {
		t.Errorf("got %d level-ups, want 4 (L2,L3,L4,L5)", len(events))
	}

	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("load post-grant: %v", err)
	}
	if got.Level != 5 {
		t.Errorf("level = %d, want 5", got.Level)
	}
	if got.XP != 300 {
		t.Errorf("xp carryover = %d, want 300", got.XP)
	}
	// Fighter d10 + CON+2 at L1 = 12. Per-level after: 6+2 = 8. L5 = 12 + 4*8 = 44 raw.
	// Phase 5-B multiplies by phase5BHPMult (1.5) → 66.
	if got.HPMax != 66 {
		t.Errorf("L5 HPMax = %d, want 66 (44 raw × phase5BHPMult)", got.HPMax)
	}
	// Each level-up bumps HPCurrent by the gain. Started at full (12), gained
	// 8 four times = 12+32 = 44. So HPCurrent = HPMax = 44.
	if got.HPCurrent != got.HPMax {
		t.Errorf("HPCurrent = %d, want HPMax = %d", got.HPCurrent, got.HPMax)
	}
}

// TestGrantDnDXP_CapsAtL20 — XP overflow at level 20 is clamped.
func TestGrantDnDXP_CapsAtL20(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	uid := id.UserID("@cap_test:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 19,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
		ArmorClass: 16,
	}
	c.HPMax = 100
	c.HPCurrent = 100
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if _, err := p.grantDnDXP(uid, 999999); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Level != dndMaxLevel {
		t.Errorf("level = %d, want %d", got.Level, dndMaxLevel)
	}
	if got.XP != 0 {
		t.Errorf("XP at cap = %d, want 0 (overflow dropped)", got.XP)
	}
}

func TestApplyClassPassives(t *testing.T) {
	cases := []struct {
		class           DnDClass
		wantDmgBonus    float64
		wantAtkBonusAdd int
		wantAutoCrit    bool
		wantHealItem    int
		wantDmgReduct   float64
		wantFlatStart   int
		wantInitBias    float64
	}{
		// Fighter-ceiling rebaseline (2026-07-16): Fighter picked up a -2 to-hit
		// sub-swing trim; Rogue's steady rider flipped +0.05→-0.10 and it took a
		// -3 to-hit trim (both offset a new 2nd swing gated at L5, not seen here);
		// Druid took a survival trim (DR 0.95→0.19) + -0.20 damage; Bard a DR 0.4
		// survival trim; Sorcerer a +1 to-hit to match the blasters; Paladin a
		// 0.9 DR survival trim. Caster CantripPerRound / Defense adds are
		// not asserted here.
		{ClassFighter, 0.05, -2, false, 0, 1.0, 0, 0},
		// Phase 2 class-balance rebalance: rogue picked up +5% damage,
		// Mage/Bard/Warlock gained a level-scaled FlatDmgStart burst, Sorcerer's
		// burst now also scales with level, and Warlock picked up +1 attack.
		// CHA/INT are 0 in this zero-value sheet → abilityModifier = -5,
		// clamped to 0 by clampNonNeg. Paladin at L1 is still 4 + 0.
		// Phase 3 class-balance: Druid picked up a WIS-scaled FlatDmgStart burst
		// (lvl 1 + clamp(mod(WIS=0)) = 1), and Sorcerer's burst base went 3→5
		// (5 + 1 + clamp(mod(CHA=10)=0) = 6).
		{ClassRogue, -0.10, -3, true, 0, 1.0, 0, 0},
		{ClassMage, 0.05, 1, false, 0, 1.0, 1, 0},
		{ClassCleric, 0, 0, false, 5, 1.0, 0, 0},
		// Class-identity audit (2026-05-16): Ranger Hunter's Mark is now
		// per-hit HuntersMarkDie (not asserted here — same shape as
		// SneakAttackDie); Paladin Divine Smite is now per-hit
		// DivineStrikePerHit (not asserted here). The +5% damage and
		// FlatDmgStart compensation riders are gone; +1 to-hit stays on
		// Ranger as the "read prey tells" half.
		{ClassRanger, 0, 1, false, 0, 1.0, 0, 0},
		{ClassDruid, -0.20, 0, false, 0, 0.95 * 0.2, 1, 0},
		{ClassBard, 0.05, 1, false, 0, 0.4, 1, 1},
		{ClassSorcerer, 0.05, 1, false, 0, 1.0, 6, 0},
		{ClassWarlock, 0.12, 1, false, 0, 1.0, 1, 0},
		{ClassPaladin, 0, 0, false, 0, 0.9, 0, 0},
	}
	for _, tc := range cases {
		stats := CombatStats{AttackBonus: 5}
		mods := CombatModifiers{DamageReduct: 1.0}
		applyClassPassives(&stats, &mods, &DnDCharacter{Class: tc.class, Level: 1, CHA: 10})
		if mods.DamageBonus != tc.wantDmgBonus {
			t.Errorf("%s: DamageBonus=%v, want %v", tc.class, mods.DamageBonus, tc.wantDmgBonus)
		}
		if stats.AttackBonus-5 != tc.wantAtkBonusAdd {
			t.Errorf("%s: AttackBonus add=%d, want %d", tc.class, stats.AttackBonus-5, tc.wantAtkBonusAdd)
		}
		if mods.AutoCritFirst != tc.wantAutoCrit {
			t.Errorf("%s: AutoCritFirst=%v, want %v", tc.class, mods.AutoCritFirst, tc.wantAutoCrit)
		}
		if mods.HealItem != tc.wantHealItem {
			t.Errorf("%s: HealItem=%d, want %d", tc.class, mods.HealItem, tc.wantHealItem)
		}
		if mods.DamageReduct != tc.wantDmgReduct {
			t.Errorf("%s: DamageReduct=%v, want %v", tc.class, mods.DamageReduct, tc.wantDmgReduct)
		}
		if mods.FlatDmgStart != tc.wantFlatStart {
			t.Errorf("%s: FlatDmgStart=%d, want %d", tc.class, mods.FlatDmgStart, tc.wantFlatStart)
		}
		if mods.InitiativeBias != tc.wantInitBias {
			t.Errorf("%s: InitiativeBias=%v, want %v", tc.class, mods.InitiativeBias, tc.wantInitBias)
		}
	}
}
