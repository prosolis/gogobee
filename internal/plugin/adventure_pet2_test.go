package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// TestDerivePlayerStats_OnePetIsByteIdentical pins that a single pet still
// produces exactly the pre-two-pet mod values (the averaging code must reduce
// to identity over one element, or the combat golden moves).
func TestDerivePlayerStats_OnePetIsByteIdentical(t *testing.T) {
	char := testChar(10)
	char.PetType = "dog"
	char.PetName = "Rex"
	char.PetArrived = true
	char.PetLevel = 6
	char.PetArmorTier = 2

	_, stats := DerivePlayerStats(char, testEquip(0), zeroBonuses(), 0, 0, false)

	if stats.PetAttackProc != petAttackChance(6) {
		t.Errorf("PetAttackProc = %v, want %v (identity over one pet)", stats.PetAttackProc, petAttackChance(6))
	}
	if stats.PetDeflectProc != petDeflectChance(6, 2) {
		t.Errorf("PetDeflectProc = %v, want %v", stats.PetDeflectProc, petDeflectChance(6, 2))
	}
	if want := 0.01 + 6*0.005; stats.PetWhiffProc != want {
		t.Errorf("PetWhiffProc = %v, want %v", stats.PetWhiffProc, want)
	}
	if stats.PetAttackDmg != 3+6 {
		t.Errorf("PetAttackDmg = %d, want %d", stats.PetAttackDmg, 3+6)
	}
}

// TestDerivePlayerStats_TwoEqualPetsMatchOne: two identical pets average to the
// same procs as one — a pair is not a stat spike.
func TestDerivePlayerStats_TwoEqualPetsMatchOne(t *testing.T) {
	char := testChar(10)
	char.PetType, char.PetName, char.PetArrived, char.PetLevel, char.PetArmorTier = "dog", "Rex", true, 8, 3
	char.Pet2Type, char.Pet2Name, char.Pet2Arrived, char.Pet2Level, char.Pet2ArmorTier = "cat", "Luna", true, 8, 3

	_, stats := DerivePlayerStats(char, testEquip(0), zeroBonuses(), 0, 0, false)

	if stats.PetAttackProc != petAttackChance(8) {
		t.Errorf("two L8 pets PetAttackProc = %v, want one-pet %v", stats.PetAttackProc, petAttackChance(8))
	}
	if stats.PetAttackDmg != 3+8 {
		t.Errorf("two L8 pets PetAttackDmg = %d, want %d", stats.PetAttackDmg, 3+8)
	}
}

// TestDerivePlayerStats_TwoUnequalPetsAverage: mixed levels average, never
// exceeding the stronger pet's solo contribution.
func TestDerivePlayerStats_TwoUnequalPetsAverage(t *testing.T) {
	char := testChar(10)
	char.PetType, char.PetArrived, char.PetLevel, char.PetArmorTier = "dog", true, 10, 0
	char.Pet2Type, char.Pet2Arrived, char.Pet2Level, char.Pet2ArmorTier = "cat", true, 2, 0

	_, stats := DerivePlayerStats(char, testEquip(0), zeroBonuses(), 0, 0, false)

	wantAtk := (petAttackChance(10) + petAttackChance(2)) / 2
	if stats.PetAttackProc != wantAtk {
		t.Errorf("PetAttackProc = %v, want avg %v", stats.PetAttackProc, wantAtk)
	}
	// Averaged proc must sit strictly between the two pets' solo values.
	if !(stats.PetAttackProc < petAttackChance(10) && stats.PetAttackProc > petAttackChance(2)) {
		t.Errorf("averaged proc %v not between %v and %v", stats.PetAttackProc, petAttackChance(2), petAttackChance(10))
	}
	if want := 3 + (10+2+1)/2; stats.PetAttackDmg != want { // rounded avg level = 6
		t.Errorf("PetAttackDmg = %d, want %d", stats.PetAttackDmg, want)
	}
}

// TestDerivePlayerStats_Pet2OnlyStillContributes: if the first pet is chased
// away but the second remains, combat still sees the second pet.
func TestDerivePlayerStats_Pet2OnlyStillContributes(t *testing.T) {
	char := testChar(10)
	char.PetType, char.PetArrived, char.PetChasedAway, char.PetLevel = "dog", true, true, 5
	char.Pet2Type, char.Pet2Arrived, char.Pet2Level = "cat", true, 4

	_, stats := DerivePlayerStats(char, testEquip(0), zeroBonuses(), 0, 0, false)

	if !char.HasPet() && stats.PetAttackProc != petAttackChance(4) {
		t.Errorf("pet2-only PetAttackProc = %v, want %v", stats.PetAttackProc, petAttackChance(4))
	}
}

// TestDerivePlayerStats_NoPetsZero: the golden's own case — no pets, no procs.
func TestDerivePlayerStats_NoPetsZero(t *testing.T) {
	_, stats := DerivePlayerStats(testChar(10), testEquip(0), zeroBonuses(), 0, 0, false)
	if stats.PetAttackProc != 0 || stats.PetDeflectProc != 0 || stats.PetWhiffProc != 0 || stats.PetAttackDmg != 0 {
		t.Errorf("petless character got pet mods: %+v", stats)
	}
}

func TestPetShouldArrive2_Gating(t *testing.T) {
	pet1 := PetState{Type: "dog", Arrived: true}
	tier4 := HouseState{Tier: 4}
	tier3 := HouseState{Tier: 3}

	if petShouldArrive2(pet1, PetState{}, tier3) {
		t.Error("second pet must not arrive below Tier 4")
	}
	if petShouldArrive2(PetState{}, PetState{}, tier4) {
		t.Error("second pet needs an established first pet")
	}
	if petShouldArrive2(pet1, PetState{Arrived: true}, tier4) {
		t.Error("already-arrived second pet must not re-roll")
	}
	if petShouldArrive2(pet1, PetState{ChasedAway: true}, tier4) {
		t.Error("chased-away second pet does not re-arrive")
	}
}

// TestPet2StoreRoundTrip exercises the pet2_* columns and the AdvChar mirror.
func TestPet2StoreRoundTrip(t *testing.T) {
	townTestDB(t)
	user := id.UserID("@pet2:test.invalid")

	char := &AdventureCharacter{UserID: user}
	char.PetType, char.PetName, char.PetArrived, char.PetLevel = "dog", "Rex", true, 3
	char.Pet2Type, char.Pet2Name, char.Pet2Arrived, char.Pet2Level, char.Pet2ArmorTier = "cat", "Luna", true, 5, 2
	if err := saveAdvCharacter(char); err != nil {
		t.Fatal(err)
	}

	got, err := loadAdvCharacter(user)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.HasPet2() || got.Pet2Name != "Luna" || got.Pet2Level != 5 || got.Pet2ArmorTier != 2 || got.Pet2Type != "cat" {
		t.Fatalf("pet2 did not round-trip: %+v", got)
	}
	if !got.HasPet() || got.PetName != "Rex" {
		t.Fatalf("pet1 clobbered by pet2 write: %+v", got)
	}

	// Direct slot loader agrees.
	p2, _ := loadPet2State(user)
	if p2.Name != "Luna" || p2.Level != 5 {
		t.Fatalf("loadPet2State mismatch: %+v", p2)
	}
}

// TestPetShowcase_IncludesBothSlots verifies the town showcase surfaces second
// pets and hides a chased-away one.
func TestPetShowcase_IncludesBothSlots(t *testing.T) {
	townTestDB(t)

	a := &AdventureCharacter{UserID: id.UserID("@a:test.invalid")}
	a.PetType, a.PetName, a.PetArrived, a.PetLevel = "dog", "Rex", true, 4
	a.Pet2Type, a.Pet2Name, a.Pet2Arrived, a.Pet2Level = "cat", "Luna", true, 9
	if err := saveAdvCharacter(a); err != nil {
		t.Fatal(err)
	}
	b := &AdventureCharacter{UserID: id.UserID("@b:test.invalid")}
	b.Pet2Type, b.Pet2Name, b.Pet2Arrived, b.Pet2Level, b.Pet2ChasedAway = "dog", "Ghost", true, 7, true
	if err := saveAdvCharacter(b); err != nil {
		t.Fatal(err)
	}

	pets, err := loadPetShowcase(townListCap)
	if err != nil {
		t.Fatal(err)
	}
	if len(pets) != 2 {
		t.Fatalf("want 2 active pets (Rex + Luna), got %d: %+v", len(pets), pets)
	}
	// Sorted by level desc → Luna (9) before Rex (4).
	if !strings.Contains(pets[0].Name, "Luna") || !strings.Contains(pets[1].Name, "Rex") {
		t.Fatalf("showcase not level-ordered across slots: %+v", pets)
	}
	for _, p := range pets {
		if strings.Contains(p.Name, "Ghost") {
			t.Errorf("chased-away second pet should be hidden: %+v", p)
		}
	}
}
