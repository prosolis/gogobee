package plugin

import (
	"strings"
	"testing"
)

// ── Housing Tier Definitions ───────────────────────────────────────────────

func TestHouseTierByTier_Valid(t *testing.T) {
	for _, def := range houseTiers {
		got := houseTierByTier(def.Tier)
		if got == nil {
			t.Errorf("houseTierByTier(%d) returned nil", def.Tier)
			continue
		}
		if got.Tier != def.Tier {
			t.Errorf("houseTierByTier(%d).Tier = %d", def.Tier, got.Tier)
		}
	}
}

func TestHouseTierByTier_Invalid(t *testing.T) {
	if houseTierByTier(0) != nil {
		t.Error("tier 0 (no house) should return nil")
	}
	if houseTierByTier(99) != nil {
		t.Error("tier 99 should return nil")
	}
}

func TestHouseNextTier(t *testing.T) {
	// From no house (tier 0) → tier 1
	def := houseNextTier(0)
	if def == nil || def.Tier != 1 {
		t.Error("next tier from 0 should be 1")
	}
	// From tier 1 → tier 2
	def = houseNextTier(1)
	if def == nil || def.Tier != 2 {
		t.Error("next tier from 1 should be 2")
	}
	// From max tier → nil
	def = houseNextTier(4)
	if def != nil {
		t.Error("next tier from max should be nil")
	}
}

// ── Closing Costs ──────────────────────────────────────────────────────────

func TestHouseClosingCosts(t *testing.T) {
	// 11% of 75000 = 8250
	costs := houseClosingCosts(75000)
	if costs != 8250 {
		t.Errorf("closing costs on 75000: got %d, want 8250", costs)
	}
}

func TestHouseTotalCost(t *testing.T) {
	// 75000 + 11% = 83250
	total := houseTotalCost(75000)
	if total != 83250 {
		t.Errorf("total cost on 75000: got %d, want 83250", total)
	}
}

// ── Weekly Payment ─────────────────────────────────────────────────────────

func TestHouseWeeklyPayment_Normal(t *testing.T) {
	// 100000 balance at 6.5%: interest = 100000*0.065/52 ≈ 125, principal = 100000*0.005 = 500
	// Total ≈ 625
	payment := houseWeeklyPayment(100000, 6.5)
	if payment < 600 || payment > 650 {
		t.Errorf("weekly payment on 100k at 6.5%%: got %d, want ~625", payment)
	}
}

func TestHouseWeeklyPayment_ZeroBalance(t *testing.T) {
	if houseWeeklyPayment(0, 6.5) != 0 {
		t.Error("zero balance should produce zero payment")
	}
}

func TestHouseWeeklyPayment_ZeroRate(t *testing.T) {
	if houseWeeklyPayment(100000, 0) != 0 {
		t.Error("zero rate should produce zero payment")
	}
}

func TestHouseWeeklyPayment_MinimumOne(t *testing.T) {
	// Very small balance should still yield at least 1
	payment := houseWeeklyPayment(1, 0.01)
	if payment < 1 {
		t.Errorf("tiny balance should produce at least 1, got %d", payment)
	}
}

// ── Missed Payment Penalty ─────────────────────────────────────────────────

func TestHouseMissedPenalty(t *testing.T) {
	// 5% of 625 = 31
	penalty := houseMissedPenalty(625)
	if penalty != 31 {
		t.Errorf("missed penalty on 625 weekly: got %d, want 31", penalty)
	}
}

func TestHouseMissedPenalty_Minimum(t *testing.T) {
	// Tiny payment should still yield at least 1
	penalty := houseMissedPenalty(1)
	if penalty < 1 {
		t.Errorf("penalty should be at least 1, got %d", penalty)
	}
}

// ── Character Helper Methods ───────────────────────────────────────────────

func TestHasHouse(t *testing.T) {
	tests := []struct {
		name     string
		tier     int
		loan     int
		expected bool
	}{
		{"no house", 0, 0, false},
		{"base house", 1, 0, true},
		{"loan only", 0, 50000, true},
		{"livable with loan", 2, 100000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := &AdventureCharacter{HouseTier: tt.tier, HouseLoanBalance: tt.loan}
			if char.HasHouse() != tt.expected {
				t.Errorf("HasHouse() = %v, want %v", char.HasHouse(), tt.expected)
			}
		})
	}
}

func TestHouseHPBonus(t *testing.T) {
	tests := []struct {
		tier     int
		expected float64
	}{
		{0, 0},
		{1, 0},     // Base house — no bonus
		{2, 0.05},  // Livable
		{3, 0.12},  // Comfortable
		{4, 0.20},  // Established
	}
	for _, tt := range tests {
		char := &AdventureCharacter{HouseTier: tt.tier}
		if bonus := char.HouseHPBonus(); bonus != tt.expected {
			t.Errorf("tier %d: HouseHPBonus() = %f, want %f", tt.tier, bonus, tt.expected)
		}
	}
}

// ── Thom Greeting ──────────────────────────────────────────────────────────

func TestThomGreeting_PreAdoption(t *testing.T) {
	char := &AdventureCharacter{}
	greeting := thomGreeting(char)
	found := false
	for _, g := range thomGreetingsPreAdoption {
		if greeting == g {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pre-adoption greeting not from expected pool: %q", greeting)
	}
}

func TestThomGreeting_PostAdoption(t *testing.T) {
	char := &AdventureCharacter{PetType: "dog", PetName: "Rex", PetArrived: true, PetLevel: 5}
	greeting := thomGreeting(char)
	found := false
	for _, g := range thomGreetingsPostAdoption {
		if greeting == g {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("post-adoption greeting not from expected pool: %q", greeting)
	}
}

func TestThomGreeting_PostLevel10(t *testing.T) {
	char := &AdventureCharacter{PetType: "cat", PetName: "Whiskers", PetArrived: true, PetLevel: 10}
	greeting := thomGreeting(char)
	if !strings.Contains(greeting, "Whiskers") {
		t.Errorf("post-level-10 greeting should contain pet name, got: %q", greeting)
	}
}

// ── Thom Shop View ─────────────────────────────────────────────────────────

func TestThomShopView_NoHouse(t *testing.T) {
	char := &AdventureCharacter{}
	text := thomShopView(char, 100000)

	if !strings.Contains(text, "Krooke Realty") {
		t.Error("should contain shop name")
	}
	if !strings.Contains(text, "don't own a house") {
		t.Error("should mention no house")
	}
	if !strings.Contains(text, "Base House") {
		t.Error("should show first tier available")
	}
	if !strings.Contains(text, "€75000") {
		t.Error("should show base price")
	}
}

func TestThomShopView_WithLoan(t *testing.T) {
	char := &AdventureCharacter{HouseTier: 1, HouseLoanBalance: 50000, HouseCurrentRate: 6.5}
	text := thomShopView(char, 10000)

	if !strings.Contains(text, "Loan balance") {
		t.Error("should show loan balance")
	}
	if !strings.Contains(text, "Pay off") {
		t.Error("should mention paying off loan")
	}
}

func TestThomShopView_MaxTier(t *testing.T) {
	char := &AdventureCharacter{HouseTier: 4} // max tier
	text := thomShopView(char, 999999)

	if !strings.Contains(text, "Max tier") {
		t.Error("should mention max tier reached")
	}
}

func TestThomShopView_Autopay(t *testing.T) {
	char := &AdventureCharacter{HouseTier: 1, HouseLoanBalance: 50000, HouseCurrentRate: 6.5, HouseAutopay: true}
	text := thomShopView(char, 10000)

	if !strings.Contains(text, "autopay") {
		t.Error("should show autopay status")
	}
}

func TestThomShopView_Frozen(t *testing.T) {
	char := &AdventureCharacter{HouseTier: 1, HouseLoanBalance: 50000, HouseCurrentRate: 6.5, HouseLoanFrozen: true}
	text := thomShopView(char, 10000)

	if !strings.Contains(text, "FROZEN") {
		t.Error("should show frozen status")
	}
}

// ── Pet Armor ──────────────────────────────────────────────────────────────

func TestPetArmorDefs_BothTypes(t *testing.T) {
	dogDefs := petArmorDefs("dog")
	catDefs := petArmorDefs("cat")

	if len(dogDefs) != 5 {
		t.Errorf("dog armor: got %d tiers, want 5", len(dogDefs))
	}
	if len(catDefs) != 5 {
		t.Errorf("cat armor: got %d tiers, want 5", len(catDefs))
	}

	// Verify tiers are sequential 1-5
	for i, def := range dogDefs {
		if def.Tier != i+1 {
			t.Errorf("dog armor[%d].Tier = %d, want %d", i, def.Tier, i+1)
		}
	}

	// Deflect bonuses should increase with tier
	for i := 1; i < len(dogDefs); i++ {
		if dogDefs[i].DeflectBonus <= dogDefs[i-1].DeflectBonus {
			t.Errorf("dog armor tier %d deflect bonus should exceed tier %d", dogDefs[i].Tier, dogDefs[i-1].Tier)
		}
	}
}

func TestPetArmorShopView_HasUpgrades(t *testing.T) {
	char := &AdventureCharacter{PetType: "dog", PetName: "Rex", PetArrived: true, PetArmorTier: 0}
	text := petArmorShopView(char)

	if !strings.Contains(text, "⬆️") {
		t.Error("should show upgrade indicators")
	}
	if !strings.Contains(text, "petbuy") {
		t.Error("should show buy instructions")
	}
}

func TestPetArmorShopView_MaxTier(t *testing.T) {
	char := &AdventureCharacter{PetType: "cat", PetName: "Luna", PetArrived: true, PetArmorTier: 5}
	text := petArmorShopView(char)

	if !strings.Contains(text, "Max pet armor") {
		t.Error("should show max tier message")
	}
}

// ── Flavor Pool Coverage ───────────────────────────────────────────────────

func TestThomFlavorPools_NonEmpty(t *testing.T) {
	pools := map[string][]string{
		"thomGreetingsPreAdoption":  thomGreetingsPreAdoption,
		"thomGreetingsPostAdoption": thomGreetingsPostAdoption,
		"thomGreetingsPostLevel10":  thomGreetingsPostLevel10,
		"thomHouseSellingLines":     thomHouseSellingLines,
		"ThomRateIncrease":          ThomRateIncrease,
		"ThomRateDecrease":          ThomRateDecrease,
		"ThomRateIncreasePet":       ThomRateIncreasePet,
		"ThomRateDecreasePet":       ThomRateDecreasePet,
	}
	for name, pool := range pools {
		if len(pool) == 0 {
			t.Errorf("flavor pool %s is empty", name)
		}
	}
}

func TestThomRateFlavor_Placeholders(t *testing.T) {
	for i, line := range ThomRateIncrease {
		if !strings.Contains(line, "{amount}") {
			t.Errorf("ThomRateIncrease[%d] missing {amount} placeholder", i)
		}
	}
	for i, line := range ThomRateDecrease {
		if !strings.Contains(line, "{amount}") {
			t.Errorf("ThomRateDecrease[%d] missing {amount} placeholder", i)
		}
	}
	for i, line := range ThomRateIncreasePet {
		if !strings.Contains(line, "{amount}") {
			t.Errorf("ThomRateIncreasePet[%d] missing {amount} placeholder", i)
		}
		if !strings.Contains(line, "{pet_name}") {
			t.Errorf("ThomRateIncreasePet[%d] missing {pet_name} placeholder", i)
		}
	}
	for i, line := range ThomRateDecreasePet {
		if !strings.Contains(line, "{amount}") {
			t.Errorf("ThomRateDecreasePet[%d] missing {amount} placeholder", i)
		}
		if !strings.Contains(line, "{pet_name}") {
			t.Errorf("ThomRateDecreasePet[%d] missing {pet_name} placeholder", i)
		}
	}
}

// ── House Tiers Sanity ─────────────────────────────────────────────────────

func TestHouseTiers_PricesIncreasing(t *testing.T) {
	for i := 1; i < len(houseTiers); i++ {
		if houseTiers[i].BasePrice <= houseTiers[i-1].BasePrice {
			t.Errorf("tier %d price (%d) should exceed tier %d price (%d)",
				houseTiers[i].Tier, houseTiers[i].BasePrice,
				houseTiers[i-1].Tier, houseTiers[i-1].BasePrice)
		}
	}
}

func TestHouseTiers_SequentialTiers(t *testing.T) {
	for i, def := range houseTiers {
		if def.Tier != i+1 {
			t.Errorf("houseTiers[%d].Tier = %d, want %d", i, def.Tier, i+1)
		}
	}
}
