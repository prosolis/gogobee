package plugin

import (
	"strings"
	"testing"
)

// ── Pet XP & Leveling ──────────────────────────────────────────────────────

func TestPetXPToNextLevel(t *testing.T) {
	tests := []struct {
		level    int
		expected int
	}{
		{0, 10},
		{1, 10},
		{2, 10},
		{4, 20},
		{5, 35},
		{7, 35},
		{8, 50},
		{9, 50},
	}
	for _, tt := range tests {
		if got := petXPToNextLevel(tt.level); got != tt.expected {
			t.Errorf("petXPToNextLevel(%d) = %d, want %d", tt.level, got, tt.expected)
		}
	}
}

func TestPetXPToNextLevel_Increasing(t *testing.T) {
	// XP requirements should never decrease as level increases
	prev := petXPToNextLevel(0)
	for level := 1; level < 10; level++ {
		curr := petXPToNextLevel(level)
		if curr < prev {
			t.Errorf("XP for level %d (%d) should not be less than level %d (%d)", level, curr, level-1, prev)
		}
		prev = curr
	}
}

func TestPetGrantXP_LevelsUp(t *testing.T) {
	char := &AdventureCharacter{
		PetType:    "dog",
		PetName:    "Rex",
		PetArrived: true,
		PetLevel:   1,
		PetXP:      950, // close to needing 1000 (10 * 100 centixp)
	}
	leveled := petGrantXP(char)
	if !leveled {
		t.Error("should have leveled up")
	}
	if char.PetLevel != 2 {
		t.Errorf("pet level should be 2, got %d", char.PetLevel)
	}
}

func TestPetGrantXP_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	if petGrantXP(char) {
		t.Error("should not grant XP without a pet")
	}
}

func TestPetGrantXP_MaxLevel(t *testing.T) {
	char := &AdventureCharacter{
		PetType:    "cat",
		PetName:    "Luna",
		PetArrived: true,
		PetLevel:   10,
		PetXP:      9999,
	}
	if petGrantXP(char) {
		t.Error("should not level up past max level 10")
	}
}

func TestPetGrantXP_ChasedAway(t *testing.T) {
	char := &AdventureCharacter{
		PetType:      "dog",
		PetName:      "Rex",
		PetArrived:   true,
		PetChasedAway: true,
		PetLevel:     1,
		PetXP:        0,
	}
	if petGrantXP(char) {
		t.Error("should not grant XP to chased-away pet")
	}
}

func TestPetGrantXP_SetsLevel10Date(t *testing.T) {
	char := &AdventureCharacter{
		PetType:    "dog",
		PetName:    "Rex",
		PetArrived: true,
		PetLevel:   9,
		PetXP:      4999, // needs 5000 (50 * 100) for level 9→10
	}
	petGrantXP(char)
	if char.PetLevel == 10 && char.PetLevel10Date == "" {
		t.Error("reaching level 10 should set PetLevel10Date")
	}
}

// ── Pet Combat Probabilities ───────────────────────────────────────────────

func TestPetAttackChance_ScalesWithLevel(t *testing.T) {
	prev := petAttackChance(0)
	for level := 1; level <= 10; level++ {
		curr := petAttackChance(level)
		if curr <= prev {
			t.Errorf("attack chance at level %d (%.3f) should exceed level %d (%.3f)", level, curr, level-1, prev)
		}
		prev = curr
	}
}

func TestPetAttackChance_Level10(t *testing.T) {
	chance := petAttackChance(10)
	// 3% + 10*1.5% = 18%
	if chance < 0.179 || chance > 0.181 {
		t.Errorf("level 10 attack chance: got %.3f, want 0.18", chance)
	}
}

func TestPetDeflectChance_ScalesWithLevel(t *testing.T) {
	prev := petDeflectChance(0, 0)
	for level := 1; level <= 10; level++ {
		curr := petDeflectChance(level, 0)
		if curr <= prev {
			t.Errorf("deflect chance at level %d (%.3f) should exceed level %d (%.3f)", level, curr, level-1, prev)
		}
		prev = curr
	}
}

func TestPetDeflectChance_ArmorBonus(t *testing.T) {
	baseChance := petDeflectChance(5, 0)
	armoredChance := petDeflectChance(5, 3)
	if armoredChance <= baseChance {
		t.Errorf("armored deflect (%.3f) should exceed base (%.3f)", armoredChance, baseChance)
	}
}

func TestPetDitchRecoveryChance_ScalesWithLevel(t *testing.T) {
	prev := petDitchRecoveryChance(0)
	for level := 1; level <= 10; level++ {
		curr := petDitchRecoveryChance(level)
		if curr <= prev {
			t.Errorf("ditch chance at level %d (%.3f) should exceed level %d (%.3f)", level, curr, level-1, prev)
		}
		prev = curr
	}
}

func TestPetDitchRecoveryChance_Level10(t *testing.T) {
	chance := petDitchRecoveryChance(10)
	// 5% + 10*2% = 25%
	if chance < 0.249 || chance > 0.251 {
		t.Errorf("level 10 ditch chance: got %.3f, want 0.25", chance)
	}
}

// ── Pet Combat Results ─────────────────────────────────────────────────────

func TestPetRollCombatActions_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	result := petRollCombatActions(char, "Goblin")
	if result != nil {
		t.Error("should return nil without a pet")
	}
}

func TestPetRollDitchRecovery_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	if petRollDitchRecovery(char) {
		t.Error("should return false without a pet")
	}
}

func TestPetVictoryText_Dog(t *testing.T) {
	char := &AdventureCharacter{PetType: "dog", PetName: "Rex", PetArrived: true}
	text := petVictoryText(char)
	if text == "" {
		t.Error("should return victory text for dog")
	}
}

func TestPetVictoryText_Cat(t *testing.T) {
	char := &AdventureCharacter{PetType: "cat", PetName: "Luna", PetArrived: true}
	text := petVictoryText(char)
	if text == "" {
		t.Error("should return victory text for cat")
	}
}

func TestPetVictoryText_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	if petVictoryText(char) != "" {
		t.Error("should return empty string without pet")
	}
}

func TestPetDeathText_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	if petDeathText(char) != "" {
		t.Error("should return empty string without pet")
	}
}

// ── Pet Arrival Logic ──────────────────────────────────────────────────────

func TestPetShouldArrive_NoHouse(t *testing.T) {
	char := &AdventureCharacter{}
	if petShouldArrive(char, HouseState{Tier: 0}) {
		t.Error("should not arrive without house")
	}
}

func TestPetShouldArrive_BaseHouseOnly(t *testing.T) {
	char := &AdventureCharacter{}
	if petShouldArrive(char, HouseState{Tier: 1}) { // Base house, not yet Livable
		t.Error("should not arrive with only base house (need tier 2+)")
	}
}

func TestPetShouldArrive_AlreadyArrived(t *testing.T) {
	char := &AdventureCharacter{PetArrived: true}
	if petShouldArrive(char, HouseState{Tier: 2}) {
		t.Error("should not trigger if pet already arrived")
	}
}

func TestPetShouldArrive_ChasedAway(t *testing.T) {
	char := &AdventureCharacter{PetChasedAway: true}
	if petShouldArrive(char, HouseState{Tier: 2}) {
		t.Error("should not trigger if pet was chased away (and not reactivated)")
	}
}

func TestPetShouldArrive_Reactivated(t *testing.T) {
	char := &AdventureCharacter{PetChasedAway: true, PetReactivated: true}
	// With reactivation, should always return true
	if !petShouldArrive(char, HouseState{Tier: 2}) {
		t.Error("reactivated pet should always trigger arrival")
	}
}

// ── HasPet ─────────────────────────────────────────────────────────────────

func TestHasPet(t *testing.T) {
	tests := []struct {
		name     string
		char     AdventureCharacter
		expected bool
	}{
		{"no pet", AdventureCharacter{}, false},
		{"has pet", AdventureCharacter{PetType: "dog", PetName: "Rex", PetArrived: true}, true},
		{"chased away", AdventureCharacter{PetType: "dog", PetName: "Rex", PetArrived: true, PetChasedAway: true}, false},
		{"not arrived", AdventureCharacter{PetType: "dog", PetName: "Rex"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.char.HasPet() != tt.expected {
				t.Errorf("HasPet() = %v, want %v", tt.char.HasPet(), tt.expected)
			}
		})
	}
}

// ── Morning Pet Events ─────────────────────────────────────────────────────

func TestPetMorningEvent_NoPet(t *testing.T) {
	char := &AdventureCharacter{}
	if petMorningEvent(char) != "" {
		t.Error("should return empty string without pet")
	}
}

// ── Misty Housing Hint ─────────────────────────────────────────────────────

func TestMistyHousingHint_NoEncounters(t *testing.T) {
	char := &AdventureCharacter{MistyEncounterCount: 0}
	if mistyHousingHint(char, HouseState{}) != "" {
		t.Error("should not hint with 0 encounters")
	}
}

func TestMistyHousingHint_HasHouse(t *testing.T) {
	char := &AdventureCharacter{MistyEncounterCount: 3}
	if mistyHousingHint(char, HouseState{Tier: 1}) != "" {
		t.Error("should not hint if player has house")
	}
}

func TestMistyHousingHint_Donated(t *testing.T) {
	char := &AdventureCharacter{MistyEncounterCount: 2, MistyDonatedCount: 1}
	hint := mistyHousingHint(char, HouseState{})
	if !strings.Contains(hint, "house") {
		t.Errorf("donated player hint should mention house, got: %q", hint)
	}
}

func TestMistyHousingHint_NotDonated(t *testing.T) {
	char := &AdventureCharacter{MistyEncounterCount: 2, MistyDonatedCount: 0}
	hint := mistyHousingHint(char, HouseState{})
	if hint != "Thank you for your time." {
		t.Errorf("non-donor hint should be dismissive, got: %q", hint)
	}
}

// ── Misty Pet Reactivation ─────────────────────────────────────────────────

func TestMistyReactivatePet_ChasedAway(t *testing.T) {
	char := &AdventureCharacter{PetChasedAway: true, PetReactivated: false}
	mistyReactivatePet(char)
	if !char.PetReactivated {
		t.Error("should set PetReactivated")
	}
}

func TestMistyReactivatePet_NotChasedAway(t *testing.T) {
	char := &AdventureCharacter{PetChasedAway: false, PetReactivated: false}
	mistyReactivatePet(char)
	if char.PetReactivated {
		t.Error("should not set PetReactivated if pet wasn't chased away")
	}
}

func TestMistyReactivatePet_AlreadyReactivated(t *testing.T) {
	char := &AdventureCharacter{PetChasedAway: true, PetReactivated: true}
	mistyReactivatePet(char)
	if !char.PetReactivated {
		t.Error("should remain reactivated")
	}
}

// ── Pet Supply Shop Unlock ─────────────────────────────────────────────────

func TestPetCheckSupplyShopUnlock_NotLevel10(t *testing.T) {
	char := &AdventureCharacter{PetLevel10Date: ""}
	if petCheckSupplyShopUnlock(char) {
		t.Error("should not unlock without level 10 date")
	}
}

func TestPetCheckSupplyShopUnlock_AlreadyUnlocked(t *testing.T) {
	char := &AdventureCharacter{PetSupplyShopUnlocked: true, PetLevel10Date: "2020-01-01"}
	if petCheckSupplyShopUnlock(char) {
		t.Error("should not re-trigger if already unlocked")
	}
}

func TestPetCheckSupplyShopUnlock_TooSoon(t *testing.T) {
	// Set date to today — not 7 days ago
	char := &AdventureCharacter{PetLevel10Date: "2099-01-01"}
	if petCheckSupplyShopUnlock(char) {
		t.Error("should not unlock before 7 days have passed")
	}
}

func TestPetCheckSupplyShopUnlock_Ready(t *testing.T) {
	char := &AdventureCharacter{PetLevel10Date: "2020-01-01"}
	if !petCheckSupplyShopUnlock(char) {
		t.Error("should unlock after 7+ days")
	}
}

// ── Ditch Recovery Text ────────────────────────────────────────────────────

func TestPetDitchRecoveryGameRoom_NoPet(t *testing.T) {
	text := petDitchRecoveryGameRoom("Player1", "", false)
	if !strings.Contains(text, "Player1") {
		t.Error("should contain player name")
	}
	if !strings.Contains(text, "stretcher") {
		t.Error("should mention stretcher for non-pet version")
	}
}

func TestPetDitchRecoveryGameRoom_WithPet(t *testing.T) {
	text := petDitchRecoveryGameRoom("Player1", "Rex", true)
	if !strings.Contains(text, "Rex") {
		t.Error("should contain pet name")
	}
	if !strings.Contains(text, "intervened") {
		t.Error("should mention pet intervention")
	}
}

// ── Flavor Pool Coverage ───────────────────────────────────────────────────

func TestPetFlavorPools_NonEmpty(t *testing.T) {
	pools := map[string][]string{
		"PetDogAttack":     PetDogAttack,
		"PetCatAttack":     PetCatAttack,
		"PetDogDeath":      PetDogDeath,
		"PetCatDeath":      PetCatDeath,
		"PetDogVictory":    PetDogVictory,
		"PetCatVictory":    PetCatVictory,
		"PetDogDeflect":    PetDogDeflect,
		"PetCatDeflect":    PetCatDeflect,
		"PetCatOffering":   PetCatOffering,
		"PetDogSmothering": PetDogSmothering,
	}
	for name, pool := range pools {
		if len(pool) == 0 {
			t.Errorf("flavor pool %s is empty", name)
		}
	}
}

func TestPetAttackFlavor_Placeholders(t *testing.T) {
	// All attack lines must have {damage}. {enemy} is optional — some lines
	// intentionally omit the enemy name for voice/style reasons (especially cat).
	for i, line := range PetDogAttack {
		if !strings.Contains(line, "{damage}") {
			t.Errorf("PetDogAttack[%d] missing {damage} placeholder", i)
		}
	}
	for i, line := range PetCatAttack {
		if !strings.Contains(line, "{damage}") {
			t.Errorf("PetCatAttack[%d] missing {damage} placeholder", i)
		}
	}
}

func TestPetDeflectFlavor_Placeholders(t *testing.T) {
	for i, line := range PetDogDeflect {
		if !strings.Contains(line, "{enemy}") {
			t.Errorf("PetDogDeflect[%d] missing {enemy} placeholder", i)
		}
	}
	for i, line := range PetCatDeflect {
		if !strings.Contains(line, "{enemy}") {
			t.Errorf("PetCatDeflect[%d] missing {enemy} placeholder", i)
		}
	}
}
