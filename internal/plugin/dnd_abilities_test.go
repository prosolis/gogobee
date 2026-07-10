package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func setupAbilitiesTestDB(t *testing.T) {
	t.Helper()
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
}

func TestActiveAbilityCatalogue(t *testing.T) {
	required := map[string]DnDClass{
		"second_wind":   ClassFighter,
		"magic_missile": ClassMage,
		"healing_word":  ClassCleric,
	}
	for id, class := range required {
		ab, ok := dndActiveAbilities[id]
		if !ok {
			t.Errorf("missing active ability: %s", id)
			continue
		}
		if ab.Class != class {
			t.Errorf("%s class = %s, want %s", id, ab.Class, class)
		}
		if ab.Apply == nil {
			t.Errorf("%s has nil Apply func", id)
		}
	}
}

func TestParseAbility(t *testing.T) {
	cases := []struct {
		in string
		id string
		ok bool
	}{
		{"second_wind", "second_wind", true},
		{"second wind", "second_wind", true},
		{"second-wind", "second_wind", true},
		{"Second Wind", "second_wind", true},
		{"magic_missile", "magic_missile", true},
		{"frostbolt", "", false},
	}
	for _, c := range cases {
		ab, ok := parseAbility(c.in)
		if ok != c.ok {
			t.Errorf("parseAbility(%q) ok = %v, want %v", c.in, ok, c.ok)
		}
		if ok && ab.ID != c.id {
			t.Errorf("parseAbility(%q) id = %q, want %q", c.in, ab.ID, c.id)
		}
	}
}

func TestClassResourceMax(t *testing.T) {
	cases := []struct {
		class DnDClass
		typ   string
		max   int
	}{
		{ClassFighter, "stamina", 3},
		{ClassRogue, "focus", 2},
		{ClassMage, "spell_slot", 1},
		{ClassCleric, "favor", 3},
		{ClassRanger, "focus", 2},
	}
	for _, c := range cases {
		typ, max := classResourceMax(c.class)
		if typ != c.typ || max != c.max {
			t.Errorf("%s: got (%s, %d), want (%s, %d)", c.class, typ, max, c.typ, c.max)
		}
	}
}

func TestInitResources_Idempotent(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@init_res:example")

	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}
	cur, max, _ := getResource(uid, "stamina")
	if cur != 3 || max != 3 {
		t.Errorf("post-init: cur=%d max=%d, want 3/3", cur, max)
	}

	// Second call should not bump.
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}
	cur2, _, _ := getResource(uid, "stamina")
	if cur2 != cur {
		t.Errorf("init not idempotent: cur went %d → %d", cur, cur2)
	}
}

func TestSpendAndRefreshResource(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@spend_test:example")
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		ok, err := spendResource(uid, "stamina", 1)
		if err != nil || !ok {
			t.Fatalf("spend %d failed: ok=%v err=%v", i, ok, err)
		}
	}
	// 4th spend should fail (drained).
	ok, _ := spendResource(uid, "stamina", 1)
	if ok {
		t.Error("4th spend succeeded; should drain at 0")
	}

	if err := refreshAllResources(uid); err != nil {
		t.Fatal(err)
	}
	cur, max, _ := getResource(uid, "stamina")
	if cur != max || cur != 3 {
		t.Errorf("post-refresh: cur=%d max=%d, want 3/3", cur, max)
	}
}

func TestApplyArmedAbility_FighterSecondWind(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@arm_fighter:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 12,
		ArmedAbility: "second_wind",
	}
	c.HPMax = computeMaxHP(c.Class, abilityModifier(c.CON), c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	mods := CombatModifiers{}
	name, fired := applyArmedAbility(c, &mods)
	if !fired {
		t.Fatal("ability did not fire")
	}
	if name != "Second Wind" {
		t.Errorf("name = %q, want Second Wind", name)
	}
	wantHeal := 5 + c.Level // 5 + 5 = 10
	if mods.HealItem != wantHeal {
		t.Errorf("HealItem = %d, want %d", mods.HealItem, wantHeal)
	}

	// Armed flag cleared, persisted.
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "" {
		t.Errorf("ArmedAbility not cleared: %q", got.ArmedAbility)
	}
}

func TestApplyArmedAbility_MageMagicMissile(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@arm_mage:example")
	c := &DnDCharacter{
		UserID: uid, Class: ClassMage, Race: RaceHuman, Level: 1,
		INT: 14, ArmedAbility: "magic_missile", HPMax: 10, HPCurrent: 10,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	mods := CombatModifiers{}
	_, fired := applyArmedAbility(c, &mods)
	if !fired {
		t.Fatal("magic missile did not fire")
	}
	wantDmg := 9 + abilityModifier(c.INT) // 9 + 2 = 11
	if mods.FlatDmgStart != wantDmg {
		t.Errorf("FlatDmgStart = %d, want %d", mods.FlatDmgStart, wantDmg)
	}
}

func TestApplyArmedAbility_NoArm(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, ArmedAbility: ""}
	mods := CombatModifiers{}
	_, fired := applyArmedAbility(c, &mods)
	if fired {
		t.Error("fired with no armed ability")
	}
}

// TestArmCommand_FullFlow integrates !arm via the handler: spends resource,
// sets armed_ability, surfaces in combat, clears.
func TestArmCommand_FullFlow(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@arm_flow:example")

	if err := createAdvCharacter(uid, "arm_flow"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 3,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
	}
	c.HPMax = computeMaxHP(c.Class, abilityModifier(c.CON), c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, c.Class); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "second_wind"); err != nil {
		t.Fatal(err)
	}

	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "second_wind" {
		t.Errorf("ArmedAbility = %q, want second_wind", got.ArmedAbility)
	}
	cur, max, _ := getResource(uid, "stamina")
	if cur != 2 || max != 3 {
		t.Errorf("stamina = %d/%d, want 2/3", cur, max)
	}

	// Trying to arm again with one already armed should fail (without
	// changing state).
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "second_wind"); err != nil {
		t.Fatal(err)
	}
	got2, _ := LoadDnDCharacter(uid)
	if got2.ArmedAbility != "second_wind" {
		t.Errorf("double-arm changed state: %q", got2.ArmedAbility)
	}
	cur2, _, _ := getResource(uid, "stamina")
	if cur2 != 2 {
		t.Errorf("double-arm spent extra resource: cur=%d", cur2)
	}
}
