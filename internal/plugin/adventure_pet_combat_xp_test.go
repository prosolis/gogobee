package plugin

import (
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newPetXPTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// TestGrantPetCombatXPPersists is the regression guard for the bug this fixes:
// petGrantXP existed but nothing called it, so an un-babysat pet sat at its
// adoption level forever. A win must move XP on disk.
func TestGrantPetCombatXPPersists(t *testing.T) {
	newPetXPTestDB(t)
	uid := id.UserID("@petxp:test")

	pet := PetState{Type: "dog", Name: "Rex", Arrived: true, Level: 1, XP: 0}
	if err := upsertPlayerMetaPetState(uid, pet); err != nil {
		t.Fatal(err)
	}

	if leveled := grantPetCombatXP(uid); len(leveled) != 0 {
		t.Errorf("one win should not level a fresh pet, got %v", leveled)
	}

	got, err := loadPetState(uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.XP != int(petXPPerAction*100) {
		t.Errorf("XP = %d, want %d", got.XP, int(petXPPerAction*100))
	}
	if got.Level != 1 {
		t.Errorf("Level = %d, want 1", got.Level)
	}
}

// TestGrantPetCombatXPLevelsBothSlots checks the second pet earns off the same
// win, matching the babysit trickle — combat only reads the two pets' averaged
// procs, so leveling both is not a power spike.
func TestGrantPetCombatXPLevelsBothSlots(t *testing.T) {
	newPetXPTestDB(t)
	uid := id.UserID("@petxp2:test")

	// Both one grant short of level 2 (needs 10 XP = 1000 centi-XP).
	short := 1000 - int(petXPPerAction*100)
	if err := upsertPlayerMetaPetState(uid,
		PetState{Type: "dog", Name: "Rex", Arrived: true, Level: 1, XP: short}); err != nil {
		t.Fatal(err)
	}
	if err := upsertPlayerMetaPet2State(uid,
		PetState{Type: "cat", Name: "Whiskers", Arrived: true, Level: 1, XP: short}); err != nil {
		t.Fatal(err)
	}

	leveled := grantPetCombatXP(uid)
	if len(leveled) != 2 {
		t.Fatalf("expected both pets to level, got %v", leveled)
	}

	p1, _ := loadPetState(uid)
	p2, _ := loadPet2State(uid)
	if p1.Level != 2 || p2.Level != 2 {
		t.Errorf("levels = %d/%d, want 2/2", p1.Level, p2.Level)
	}
}

// TestGrantPetCombatXPIgnoresChasedAway — a pet that isn't with you doesn't
// fight, so it doesn't earn.
func TestGrantPetCombatXPIgnoresChasedAway(t *testing.T) {
	newPetXPTestDB(t)
	uid := id.UserID("@petxp3:test")

	if err := upsertPlayerMetaPetState(uid, PetState{
		Type: "dog", Name: "Rex", Arrived: true, ChasedAway: true, Level: 3, XP: 100,
	}); err != nil {
		t.Fatal(err)
	}

	grantPetCombatXP(uid)

	got, _ := loadPetState(uid)
	if got.XP != 100 {
		t.Errorf("chased-away pet gained XP: %d, want 100", got.XP)
	}
}

// TestGrantPetCombatXPCapsAtTen — a maxed pet stops earning rather than
// accumulating dead XP.
func TestGrantPetCombatXPCapsAtTen(t *testing.T) {
	newPetXPTestDB(t)
	uid := id.UserID("@petxp4:test")

	if err := upsertPlayerMetaPetState(uid, PetState{
		Type: "dog", Name: "Rex", Arrived: true, Level: 10, XP: 0,
	}); err != nil {
		t.Fatal(err)
	}

	if leveled := grantPetCombatXP(uid); len(leveled) != 0 {
		t.Errorf("L10 pet should not level, got %v", leveled)
	}
	got, _ := loadPetState(uid)
	if got.XP != 0 || got.Level != 10 {
		t.Errorf("L10 pet moved: level %d xp %d", got.Level, got.XP)
	}
}
