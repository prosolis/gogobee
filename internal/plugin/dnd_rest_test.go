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

func TestShortRest_CooldownEnforced(t *testing.T) {
	setupRestTestDB(t)
	uid := id.UserID("@short_cd:example")
	makeRestTestChar(t, uid, 3)
	p := &AdventurePlugin{}

	// First rest succeeds
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got1, _ := LoadDnDCharacter(uid)
	hpAfterFirst := got1.HPCurrent

	// Second immediate rest should NOT heal further (cooldown blocks).
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	got2, _ := LoadDnDCharacter(uid)
	if got2.HPCurrent != hpAfterFirst {
		t.Errorf("cooldown not enforced: HP changed from %d → %d on second rest",
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
