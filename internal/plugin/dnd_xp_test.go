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
		{1, 300},   // L1→L2: 300
		{2, 600},   // L2→L3: 900-300
		{4, 1000},  // L4→L5: 2700-1700
		{6, 2200},  // L6→L7: 6500-4300
		{19, 11000}, // L19→L20: 85000-74000
		{20, 0},    // capped
		{0, 300},   // clamp to 1
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
	// Fighter d10 + CON+2 at L1 = 12. Per-level after: 6+2 = 8. L5 = 12 + 4*8 = 44.
	if got.HPMax != 44 {
		t.Errorf("L5 HPMax = %d, want 44", got.HPMax)
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
	}{
		{ClassFighter, 0.05, 0, false, 0},
		{ClassRogue, 0, 0, true, 0},
		{ClassMage, 0, 1, false, 0},
		{ClassCleric, 0, 0, false, 5},
		{ClassRanger, 0.05, 1, false, 0},
	}
	for _, tc := range cases {
		stats := CombatStats{AttackBonus: 5}
		mods := CombatModifiers{}
		applyClassPassives(&stats, &mods, &DnDCharacter{Class: tc.class})
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
	}
}
