package plugin

import (
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newRenownTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func TestRenownLevelFor(t *testing.T) {
	cases := []struct{ xp, want int }{
		{0, 0},
		{-100, 0},
		{renownXPPerLevel - 1, 0},
		{renownXPPerLevel, 1},
		{renownXPPerLevel*3 + 12, 3},
		{renownXPPerLevel * 30, 30},
	}
	for _, c := range cases {
		if got := renownLevelFor(c.xp); got != c.want {
			t.Errorf("renownLevelFor(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}

func TestRenownXPIntoLevel(t *testing.T) {
	into, cost := renownXPIntoLevel(renownXPPerLevel + 500)
	if cost != renownXPPerLevel {
		t.Errorf("cost = %d, want %d", cost, renownXPPerLevel)
	}
	if into != 500 {
		t.Errorf("into = %d, want 500", into)
	}
}

func TestRenownRankLadderMonotonic(t *testing.T) {
	// Rank thresholds must strictly increase so renownRankFor is well-defined.
	for i := 1; i < len(renownRanks); i++ {
		if renownRanks[i].MinLevel <= renownRanks[i-1].MinLevel {
			t.Errorf("rank %d threshold %d not > %d", i, renownRanks[i].MinLevel, renownRanks[i-1].MinLevel)
		}
	}
	if renownRankFor(0) != "" {
		t.Errorf("rank at 0 should be empty")
	}
	if renownRankFor(1) != "Renowned" {
		t.Errorf("rank at 1 = %q, want Renowned", renownRankFor(1))
	}
	if renownRankFor(4) != "Storied" {
		t.Errorf("rank at 4 = %q, want Storied (threshold 3)", renownRankFor(4))
	}
	if renownRankFor(1000) != "Eternal" {
		t.Errorf("top rank = %q, want Eternal", renownRankFor(1000))
	}
}

func TestRenownMarker(t *testing.T) {
	if renownMarker(0) != "" {
		t.Errorf("marker at 0 should be empty")
	}
	if got := renownMarker(7); got != "✦7" {
		t.Errorf("marker(7) = %q, want ✦7", got)
	}
}

// TestApplyRenownBonuses_CombatNeutralAndCapped — renown grants only the two
// combat-neutral economy levers (loot/XP), capped at +15%/+20%, and must never
// touch a lever that combat_stats.go maps to a combat stat. §B2.
func TestApplyRenownBonuses_CombatNeutralAndCapped(t *testing.T) {
	// Below the first step: no effect.
	b := &AdvBonusSummary{}
	applyRenownBonuses(b, 2)
	if b.XPMultiplier != 0 || b.LootQuality != 0 {
		t.Errorf("renown < step should be inert, got %+v", b)
	}

	// One step at renown 3.
	b = &AdvBonusSummary{}
	applyRenownBonuses(b, 3)
	if b.XPMultiplier != 2 || b.LootQuality != 1.5 {
		t.Errorf("one step wrong: %+v", b)
	}

	// At renown 30 and beyond, capped at +20% XP / +15% loot.
	for _, lvl := range []int{30, 45, 300} {
		b = &AdvBonusSummary{}
		applyRenownBonuses(b, lvl)
		if b.XPMultiplier != 20 || b.LootQuality != 15 {
			t.Errorf("renown %d not capped: %+v", lvl, b)
		}
	}

	// Never combat-stat inflation: the levers combat_stats.go reads
	// (DeathModifier→Defense, SuccessBonus→Attack, ExceptionalBonus→CritRate)
	// and the skill/combat levers must all stay at zero at any renown level.
	b = &AdvBonusSummary{}
	applyRenownBonuses(b, 300)
	if b.DeathModifier != 0 || b.SuccessBonus != 0 || b.ExceptionalBonus != 0 {
		t.Errorf("renown leaked into a combat-stat lever: %+v", b)
	}
	if b.CombatBonus != 0 || b.MiningBonus != 0 || b.ForagingBonus != 0 || b.FishingBonus != 0 {
		t.Errorf("renown leaked into combat/skill levers: %+v", b)
	}
}

// TestAddRenownXP_Accumulates — cumulative += with correct before/after.
func TestAddRenownXP_Accumulates(t *testing.T) {
	newRenownTestDB(t)
	uid := id.UserID("@renown_add:example")
	if err := createAdvCharacter(uid, "Renowner"); err != nil {
		t.Fatal(err)
	}

	before, after, err := addRenownXP(uid, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if before != 0 || after != 10000 {
		t.Errorf("first add: before=%d after=%d, want 0/10000", before, after)
	}

	before, after, err = addRenownXP(uid, 20000)
	if err != nil {
		t.Fatal(err)
	}
	if before != 10000 || after != 30000 {
		t.Errorf("second add: before=%d after=%d, want 10000/30000", before, after)
	}

	got, err := loadRenownXP(uid)
	if err != nil || got != 30000 {
		t.Errorf("loadRenownXP = %d (err %v), want 30000", got, err)
	}
	if renownLevelForUser(uid) != 1 {
		t.Errorf("renownLevelForUser = %d, want 1", renownLevelForUser(uid))
	}
}

// TestRenownAtLeast — the B4 achievement gate reads the derived level correctly.
func TestRenownAtLeast(t *testing.T) {
	newRenownTestDB(t)
	uid := id.UserID("@renown_ach:example")
	if err := createAdvCharacter(uid, "Achiever"); err != nil {
		t.Fatal(err)
	}
	d := db.Get()
	if renownAtLeast(d, uid, 1) {
		t.Errorf("no renown yet, should not meet level 1")
	}
	if _, _, err := addRenownXP(uid, renownXPPerLevel*5); err != nil {
		t.Fatal(err)
	}
	if !renownAtLeast(d, uid, 5) {
		t.Errorf("renown 5 should meet level 5")
	}
	if renownAtLeast(d, uid, 6) {
		t.Errorf("renown 5 should not meet level 6")
	}
	// Absent player → false, not a crash.
	if renownAtLeast(d, id.UserID("@nobody:nowhere.invalid"), 1) {
		t.Errorf("absent player should not meet any renown level")
	}
}

// TestRenownOverlay — applyPlayerMetaOverlay populates RenownXP and RenownLevel.
func TestRenownOverlay(t *testing.T) {
	newRenownTestDB(t)
	uid := id.UserID("@renown_overlay:example")
	if err := createAdvCharacter(uid, "Overlaid"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := addRenownXP(uid, renownXPPerLevel*5+7); err != nil {
		t.Fatal(err)
	}
	c, err := loadAdvCharacter(uid)
	if err != nil || c == nil {
		t.Fatalf("load: %v", err)
	}
	if c.RenownXP != renownXPPerLevel*5+7 {
		t.Errorf("overlay RenownXP = %d, want %d", c.RenownXP, renownXPPerLevel*5+7)
	}
	if c.RenownLevel() != 5 {
		t.Errorf("RenownLevel() = %d, want 5", c.RenownLevel())
	}
}

// TestGrantDnDXP_OverflowBecomesRenown — at the cap, grantDnDXP routes overflow
// into renown_xp and zeroes dnd_xp (the pre-N7 cap invariant is preserved).
func TestGrantDnDXP_OverflowBecomesRenown(t *testing.T) {
	newRenownTestDB(t)
	uid := id.UserID("@renown_grant:example")
	if err := createAdvCharacter(uid, "Capped"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: dndMaxLevel,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12, ArmorClass: 16,
	}
	c.HPMax = 100
	c.HPCurrent = 100
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	events, err := p.grantDnDXP(uid, renownXPPerLevel+5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("got %d level-up events at cap, want 0", len(events))
	}

	got, _ := LoadDnDCharacter(uid)
	if got.Level != dndMaxLevel {
		t.Errorf("level = %d, want %d", got.Level, dndMaxLevel)
	}
	if got.XP != 0 {
		t.Errorf("dnd_xp = %d, want 0 (overflow moved to renown)", got.XP)
	}
	if xp, _ := loadRenownXP(uid); xp != renownXPPerLevel+5000 {
		t.Errorf("renown_xp = %d, want %d", xp, renownXPPerLevel+5000)
	}
	if renownLevelForUser(uid) != 1 {
		t.Errorf("renown level = %d, want 1", renownLevelForUser(uid))
	}
}
