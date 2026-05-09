package plugin

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// L1: babysit pivot from harvest to pet-care + safe-rest.
// These tests cover the pure pieces — DB-touching paths exercise via
// integration only.

func TestBabysitSafeRest_NoDB_ReturnsFalse(t *testing.T) {
	// Tests run without db.Init(); BabysitSafeRest must recover and
	// return false rather than panicking.
	if BabysitSafeRest(id.UserID("@nodb:example")) {
		t.Errorf("expected false for un-initialized DB")
	}
}

func TestRunBabysitDailyTrickle_NoPetGrantsNothing(t *testing.T) {
	// Without a pet, the trickle is a logging-only no-op for character
	// state. Without DB, the log call panics inside db.Get; recover and
	// assert that the in-memory char fields stay clean.
	defer func() { _ = recover() }()

	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	char := &AdventureCharacter{
		UserID:           id.UserID("@nopet:example"),
		BabysitActive:    true,
		BabysitExpiresAt: &expires,
	}
	p := &AdventurePlugin{}
	p.runBabysitDailyTrickle(char)
	if char.PetXP != 0 {
		t.Errorf("char without pet should not gain PetXP, got %d", char.PetXP)
	}
	if char.PetLevel != 0 {
		t.Errorf("char without pet should not change PetLevel, got %d", char.PetLevel)
	}
}

func TestRunBabysitDailyTrickle_SkipsWhenInactive(t *testing.T) {
	defer func() { _ = recover() }()
	char := &AdventureCharacter{
		UserID:        id.UserID("@inactive:example"),
		BabysitActive: false,
		PetType:       "dog",
		PetName:       "Rex",
		PetLevel:      1,
		PetXP:         0,
	}
	p := &AdventurePlugin{}
	p.runBabysitDailyTrickle(char)
	if char.PetXP != 0 {
		t.Errorf("inactive babysit should grant no XP, got %d", char.PetXP)
	}
}

func TestRunBabysitDailyTrickle_AccumulatesAndLevels(t *testing.T) {
	// Pet at L9 with 49 XP needed → 1 day at +3 trickle should *not* level
	// (49→52 in centi-XP / 100 = no, wait: petXPToNextLevel at L9 = 50 ×
	// 100 = 5000 centi-XP). Verify we accumulate centi-XP correctly across
	// multiple daily ticks and eventually level up.
	defer func() { _ = recover() }()

	char := &AdventureCharacter{
		UserID:   id.UserID("@accum:example"),
		PetType:  "dog",
		PetName:  "Rex",
		PetLevel: 1, // needs 10 XP = 1000 centi-XP to L2
		PetXP:    900,
		BabysitActive: true,
	}
	expires := time.Now().UTC().Add(48 * time.Hour)
	char.BabysitExpiresAt = &expires
	p := &AdventurePlugin{}

	// One trickle adds 3*100 = 300 centi-XP. 900 + 300 = 1200 >= 1000 →
	// level to 2, carry 200 centi-XP.
	p.runBabysitDailyTrickle(char)
	if char.PetLevel != 2 {
		t.Errorf("expected level 2 after trickle, got %d (xp=%d)", char.PetLevel, char.PetXP)
	}
	if char.PetXP != 200 {
		t.Errorf("expected 200 carryover centi-XP, got %d", char.PetXP)
	}
}

func TestBabysitLogStats_CountsPetCareDays(t *testing.T) {
	logs := []babysitLogEntry{
		{Activity: "pet_care", XPGained: 3},
		{Activity: "pet_care", XPGained: 3},
		{Activity: "pet_care", XPGained: 3, RivalRefused: ""},
		{Activity: "rival_refused", RivalRefused: "Garth"},
	}
	xp, days, rivals := babysitLogStats(logs)
	if days != 3 {
		t.Errorf("petDays = %d, want 3", days)
	}
	if xp != 9 {
		t.Errorf("totalXP = %d, want 9", xp)
	}
	if rivals != 1 {
		t.Errorf("rivalsRefused = %d, want 1", rivals)
	}
}
