package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func setupRestTestDB(t *testing.T) {
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

func makeRestTestChar(t *testing.T, uid id.UserID, level int) *DnDCharacter {
	t.Helper()
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
	}
	conMod := abilityModifier(c.CON)
	c.HPMax = computeMaxHP(c.Class, conMod, level)
	c.HPCurrent = 1 // wounded
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	c.ShortRestCharges = level // charges = level (matches autoBuildCharacter)
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	// Also need an adventure_characters row so loadAdvCharacter doesn't fail.
	if err := createAdvCharacter(uid, "rest_test"); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestShortRest_HealsWithinExpectedRange(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_rest:example")
	c := makeRestTestChar(t, uid, 3) // L3 → 1d6+CON, no x2

	p := &AdventurePlugin{}
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	conMod := abilityModifier(c.CON) // +2
	// Heal range L1-4: 1d6 + 2 = 3..8
	healed := got.HPCurrent - 1
	if healed < 1+conMod || healed > 6+conMod {
		t.Errorf("healed %d HP, want %d..%d (1d6+%d)", healed, 1+conMod, 6+conMod, conMod)
	}
	if got.LastShortRestAt == nil {
		t.Error("LastShortRestAt not set")
	}
}

func TestShortRest_DoublesAtL5(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_rest_l5:example")
	c := makeRestTestChar(t, uid, 5) // L5 → 2*(1d6+CON)
	conMod := abilityModifier(c.CON)

	p := &AdventurePlugin{}
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	healed := got.HPCurrent - 1
	// Range: 2*(1+2)..2*(6+2) = 6..16
	low, high := 2*(1+conMod), 2*(6+conMod)
	if healed < low || healed > high {
		t.Errorf("L5 healed %d HP, want %d..%d", healed, low, high)
	}
}

func TestShortRest_ChargesEnforced(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_cd:example")
	c := makeRestTestChar(t, uid, 3)
	// Drain to last charge so the second rest hits the empty branch.
	c.ShortRestCharges = 1
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}

	// First rest succeeds and burns the last charge.
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got1, _ := LoadDnDCharacter(uid)
	if got1.ShortRestCharges != 0 {
		t.Errorf("charge not consumed: %d remaining", got1.ShortRestCharges)
	}
	hpAfterFirst := got1.HPCurrent

	// Second rest should bail (no charges) — HP unchanged.
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got2, _ := LoadDnDCharacter(uid)
	if got2.HPCurrent != hpAfterFirst {
		t.Errorf("charges not enforced: HP changed from %d → %d on second rest",
			hpAfterFirst, got2.HPCurrent)
	}
}

func TestShortRest_AlreadyFullHP(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_full:example")
	c := makeRestTestChar(t, uid, 3)
	c.HPCurrent = c.HPMax
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.LastShortRestAt != nil {
		t.Error("rest at full HP shouldn't consume cooldown")
	}
}

// makeRestTestMage builds a wounded mage (so short rest doesn't bail on
// full-HP) and provisions the class slot pool. The caller is responsible
// for burning slots before testing the refresh.
func makeRestTestMage(t *testing.T, uid id.UserID, level int) *DnDCharacter {
	t.Helper()
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassMage, Level: level,
		STR: 8, DEX: 13, CON: 12, INT: 16, WIS: 10, CHA: 12,
	}
	conMod := abilityModifier(c.CON)
	c.HPMax = computeMaxHP(c.Class, conMod, level)
	c.HPCurrent = 1
	c.ArmorClass = computeAC(c.Class, abilityModifier(c.DEX))
	c.ShortRestCharges = level
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := createAdvCharacter(uid, "mage_rest_test"); err != nil {
		t.Fatal(err)
	}
	if err := setSpellSlotsForLevel(uid, ClassMage, level); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPartialRefreshSpellSlots_L5MageToPsAtL2(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@partial_l5:example")
	makeRestTestMage(t, uid, 5) // L5 mage → 4 L1, 3 L2, 2 L3

	// Burn everything so the refresh has work to do.
	pool := slotsForClassLevel(ClassMage, 5)
	for lvl, total := range pool {
		for i := 0; i < total; i++ {
			if _, err := consumeSpellSlot(uid, lvl); err != nil {
				t.Fatal(err)
			}
		}
	}

	restored, err := partialRefreshSpellSlots(uid, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored[1]; got != pool[1] {
		t.Errorf("L1 restored = %d, want %d (all)", got, pool[1])
	}
	if got := restored[2]; got != 1 {
		t.Errorf("L2 restored = %d, want 1 (floor(5/4))", got)
	}
	if _, ok := restored[3]; ok {
		t.Errorf("L3 should not have been restored: %v", restored)
	}

	slots, _ := getSpellSlots(uid)
	if used := slots[1][1]; used != 0 {
		t.Errorf("L1 used after refresh = %d, want 0", used)
	}
	if used := slots[2][1]; used != pool[2]-1 {
		t.Errorf("L2 used after refresh = %d, want %d", used, pool[2]-1)
	}
	if used := slots[3][1]; used != pool[3] {
		t.Errorf("L3 used after refresh = %d, want %d (untouched)", used, pool[3])
	}
}

func TestPartialRefreshSpellSlots_NonCasterNoop(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@partial_fighter:example")
	makeRestTestChar(t, uid, 5) // fighter — no slots

	restored, err := partialRefreshSpellSlots(uid, 5)
	if err != nil {
		t.Fatal(err)
	}
	if restored != nil {
		t.Errorf("fighter got slots restored: %v", restored)
	}
}

func TestPartialRefreshSpellSlots_FullCasterReturnsEmpty(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@partial_full:example")
	makeRestTestMage(t, uid, 5) // slots already at used=0 from setup

	restored, err := partialRefreshSpellSlots(uid, 5)
	if err != nil {
		t.Fatal(err)
	}
	if restored != nil {
		t.Errorf("full mage got slots restored: %v", restored)
	}
}

func TestShortRest_RefreshesPartialSlotsForMage(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_mage:example")
	makeRestTestMage(t, uid, 5)
	pool := slotsForClassLevel(ClassMage, 5)
	for lvl, total := range pool {
		for i := 0; i < total; i++ {
			consumeSpellSlot(uid, lvl)
		}
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	slots, _ := getSpellSlots(uid)
	if used := slots[1][1]; used != 0 {
		t.Errorf("L1 used after short rest = %d, want 0", used)
	}
	if used := slots[2][1]; used != pool[2]-1 {
		t.Errorf("L2 used after short rest = %d, want %d", used, pool[2]-1)
	}
}

func TestDndShortRestSlotLine(t *testing.T) {
	if got := dndShortRestSlotLine(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := dndShortRestSlotLine(map[int]int{}); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
	got := dndShortRestSlotLine(map[int]int{1: 4, 2: 1})
	want := "Spell slots restored: 4 (L1), 1 (L2)."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLongRest_RequiresHousingOrInn(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@long_no_house:example")
	makeRestTestChar(t, uid, 3)
	// AdventureCharacter has HouseTier=0; player has no euros either by default.

	p := &AdventurePlugin{euro: nil} // no euro plugin → can't pay inn
	if err := p.handleDnDLongRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent == got.HPMax {
		t.Error("long rest succeeded without housing or inn payment")
	}
	if got.LastLongRestAt != nil {
		t.Error("LastLongRestAt set despite failed rest")
	}
}

func TestLongRest_WithHousing(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@long_house:example")
	makeRestTestChar(t, uid, 3)
	// Upgrade to housing.
	advChar, _ := loadAdvCharacter(uid)
	advChar.HouseTier = 2
	if err := saveAdvCharacter(advChar); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDLongRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != got.HPMax {
		t.Errorf("long rest with housing didn't fully heal: %d/%d", got.HPCurrent, got.HPMax)
	}
	if got.LastLongRestAt == nil {
		t.Error("LastLongRestAt not set after successful long rest")
	}
}

func TestLongRest_CooldownEnforced(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@long_cd:example")
	c := makeRestTestChar(t, uid, 3)
	advChar, _ := loadAdvCharacter(uid)
	advChar.HouseTier = 1
	saveAdvCharacter(advChar)

	now := time.Now().UTC().Add(-1 * time.Hour) // recent rest
	c.LastLongRestAt = &now
	c.HPCurrent = 1
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDLongRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != 1 {
		t.Errorf("long rest cooldown not enforced; HP went from 1 → %d", got.HPCurrent)
	}
}
