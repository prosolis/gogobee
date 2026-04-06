package plugin

import "testing"

func TestAdvEquipmentScore_Empty(t *testing.T) {
	score := advEquipmentScore(map[EquipmentSlot]*AdvEquipment{})
	if score != 0 {
		t.Errorf("empty equipment should score 0, got %.2f", score)
	}
}

func TestAdvEquipmentScore_WeaponDoubled(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 3, Condition: 100},
		SlotArmor:  {Tier: 3, Condition: 100},
	}
	score := advEquipmentScore(equip)
	// Weapon: 3*2=6, Armor: 3
	if score != 9.0 {
		t.Errorf("got %.2f, want 9 (weapon 6 + armor 3)", score)
	}
}

func TestAdvEquipmentScore_LowConditionHalved(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotArmor: {Tier: 4, Condition: 49}, // below 50%
	}
	score := advEquipmentScore(equip)
	// Tier 4, halved = 2
	if score != 2.0 {
		t.Errorf("got %.2f, want 2 (tier 4 halved for low condition)", score)
	}
}

func TestAdvEquipmentScore_FullLoadout(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5, Condition: 100},
		SlotArmor:  {Tier: 5, Condition: 100},
		SlotHelmet: {Tier: 5, Condition: 100},
		SlotBoots:  {Tier: 5, Condition: 100},
		SlotTool:   {Tier: 5, Condition: 100},
	}
	score := advEquipmentScore(equip)
	// Weapon: 5*2=10, others: 5*4=20, total=30
	if score != 30.0 {
		t.Errorf("got %.2f, want 30", score)
	}
}

func TestXpToNextLevel(t *testing.T) {
	// Level 1: 100 + 3 = 103
	if xp := xpToNextLevel(1); xp != 103 {
		t.Errorf("level 1: got %d, want 103", xp)
	}
	// Level 50: 100 + 150 = 250
	if xp := xpToNextLevel(50); xp != 250 {
		t.Errorf("level 50: got %d, want 250", xp)
	}
	// Higher levels should require more XP
	for i := 1; i < 50; i++ {
		if xpToNextLevel(i) >= xpToNextLevel(i+1) {
			t.Errorf("xp for level %d should be less than level %d", i, i+1)
		}
	}
}

func TestAdvIsEligible_TooLowLevel(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 5}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 3, Condition: 100},
	}
	loc := &AdvLocation{Activity: AdvActivityDungeon, MinLevel: 20, MinEquipTier: 0}
	bonuses := &AdvBonusSummary{}

	eligible, _ := advIsEligible(char, equip, loc, bonuses)
	if eligible {
		t.Error("should not be eligible with combat level 5 for min level 20")
	}
}

func TestAdvIsEligible_TooLowEquipTier(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 30}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 1, Condition: 100},
		SlotArmor:  {Tier: 0, Condition: 100}, // this is the bottleneck
	}
	loc := &AdvLocation{Activity: AdvActivityDungeon, MinLevel: 1, MinEquipTier: 1}
	bonuses := &AdvBonusSummary{}

	eligible, _ := advIsEligible(char, equip, loc, bonuses)
	if eligible {
		t.Error("should not be eligible with min equip tier 0 for requirement 1")
	}
}

func TestAdvIsEligible_PenaltyZone(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 21} // within 3 of min 20
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100},
	}
	loc := &AdvLocation{Activity: AdvActivityDungeon, MinLevel: 20, MinEquipTier: 0}
	bonuses := &AdvBonusSummary{}

	eligible, penalty := advIsEligible(char, equip, loc, bonuses)
	if !eligible {
		t.Error("should be eligible at combat level 21 for min 20")
	}
	if !penalty {
		t.Error("should be in penalty zone (21-20=1, less than 3)")
	}
}

func TestAdvIsEligible_NoPenalty(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 25}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100},
	}
	loc := &AdvLocation{Activity: AdvActivityDungeon, MinLevel: 20, MinEquipTier: 0}
	bonuses := &AdvBonusSummary{}

	eligible, penalty := advIsEligible(char, equip, loc, bonuses)
	if !eligible {
		t.Error("should be eligible")
	}
	if penalty {
		t.Error("should NOT be in penalty zone (25-20=5, >= 3)")
	}
}

func TestCalculateAdvProbabilities_SumsTo100(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 10}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100},
		SlotArmor:  {Tier: 2, Condition: 100},
	}
	loc := &AdvLocation{
		Activity:     AdvActivityDungeon,
		BaseDeathPct: 18,
		EmptyPct:     15,
		MinLevel:     8,
	}
	bonuses := &AdvBonusSummary{}

	prob := calculateAdvProbabilities(char, equip, loc, bonuses, false)
	total := prob.DeathPct + prob.EmptyPct + prob.SuccessPct + prob.ExceptionalPct

	if total < 99.9 || total > 100.1 {
		t.Errorf("probabilities should sum to 100, got %.2f (death=%.1f empty=%.1f success=%.1f exceptional=%.1f)",
			total, prob.DeathPct, prob.EmptyPct, prob.SuccessPct, prob.ExceptionalPct)
	}
}

func TestCalculateAdvProbabilities_PenaltyIncreasesRisk(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 10}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100},
	}
	loc := &AdvLocation{
		Activity:     AdvActivityDungeon,
		BaseDeathPct: 18,
		EmptyPct:     15,
		MinLevel:     8,
	}
	bonuses := &AdvBonusSummary{}

	noPenalty := calculateAdvProbabilities(char, equip, loc, bonuses, false)
	withPenalty := calculateAdvProbabilities(char, equip, loc, bonuses, true)

	if withPenalty.DeathPct <= noPenalty.DeathPct {
		t.Errorf("penalty zone should increase death %% (%.1f vs %.1f)", withPenalty.DeathPct, noPenalty.DeathPct)
	}
}

func TestCalculateAdvProbabilities_BetterGearReducesDeath(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 10}
	loc := &AdvLocation{
		Activity:     AdvActivityDungeon,
		BaseDeathPct: 30,
		EmptyPct:     15,
		MinLevel:     8,
	}
	bonuses := &AdvBonusSummary{}

	weakGear := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 1, Condition: 100},
	}
	strongGear := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5, Condition: 100},
		SlotArmor:  {Tier: 5, Condition: 100},
		SlotHelmet: {Tier: 5, Condition: 100},
		SlotBoots:  {Tier: 5, Condition: 100},
		SlotTool:   {Tier: 5, Condition: 100},
	}

	weakProb := calculateAdvProbabilities(char, weakGear, loc, bonuses, false)
	strongProb := calculateAdvProbabilities(char, strongGear, loc, bonuses, false)

	if strongProb.DeathPct >= weakProb.DeathPct {
		t.Errorf("better gear should reduce death %% (strong=%.1f vs weak=%.1f)", strongProb.DeathPct, weakProb.DeathPct)
	}
}

func TestAdvEffectiveSkill(t *testing.T) {
	char := &AdventureCharacter{
		CombatLevel:  10,
		MiningSkill:  15,
		ForagingSkill: 20,
	}
	bonuses := &AdvBonusSummary{
		CombatBonus:  5,
		MiningBonus:  3,
		ForagingBonus: 0,
	}

	if s := advEffectiveSkill(char, AdvActivityDungeon, bonuses); s != 15 {
		t.Errorf("dungeon: got %d, want 15 (10+5)", s)
	}
	if s := advEffectiveSkill(char, AdvActivityMining, bonuses); s != 18 {
		t.Errorf("mining: got %d, want 18 (15+3)", s)
	}
	if s := advEffectiveSkill(char, AdvActivityForaging, bonuses); s != 20 {
		t.Errorf("foraging: got %d, want 20 (20+0)", s)
	}
}

func TestAdvParseShopCategory(t *testing.T) {
	tests := []struct {
		input string
		want  EquipmentSlot
	}{
		{"weapon", SlotWeapon},
		{"sword", SlotWeapon},
		{"swords", SlotWeapon},
		{"armor", SlotArmor},
		{"armour", SlotArmor},
		{"helmet", SlotHelmet},
		{"helm", SlotHelmet},
		{"boots", SlotBoots},
		{"boot", SlotBoots},
		{"tool", SlotTool},
		{"pickaxe", SlotTool},
		{"  WEAPON  ", SlotWeapon}, // trimmed + lowered
		{"nonsense", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := advParseShopCategory(tt.input)
		if got != tt.want {
			t.Errorf("advParseShopCategory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlotEmoji(t *testing.T) {
	if e := slotEmoji(SlotWeapon); e != "⚔️" {
		t.Errorf("weapon emoji: got %q", e)
	}
	if e := slotEmoji(SlotArmor); e != "🛡️" {
		t.Errorf("armor emoji: got %q", e)
	}
	// Default case
	if e := slotEmoji("unknown"); e != "📦" {
		t.Errorf("unknown slot emoji: got %q, want 📦", e)
	}
}

func TestSlotTitle(t *testing.T) {
	if s := slotTitle(SlotWeapon); s != "Weapon" {
		t.Errorf("got %q, want Weapon", s)
	}
	if s := slotTitle(SlotBoots); s != "Boots" {
		t.Errorf("got %q, want Boots", s)
	}
	if s := slotTitle(""); s != "" {
		t.Errorf("empty slot should return empty, got %q", s)
	}
}

func TestComputeAdvBonuses(t *testing.T) {
	treasures := []AdvTreasureBonus{
		{BonusType: "combat_level", BonusValue: 3},
		{BonusType: "mining_skill", BonusValue: 2},
		{BonusType: "all_skills", BonusValue: 1},
		{BonusType: "death_chance", BonusValue: -2.5},
	}

	bonuses := computeAdvBonuses(treasures, nil, 0, false)

	if bonuses.CombatBonus != 4 { // 3 + 1 from all_skills
		t.Errorf("CombatBonus: got %d, want 4", bonuses.CombatBonus)
	}
	if bonuses.MiningBonus != 3 { // 2 + 1 from all_skills
		t.Errorf("MiningBonus: got %d, want 3", bonuses.MiningBonus)
	}
	if bonuses.ForagingBonus != 1 { // 1 from all_skills
		t.Errorf("ForagingBonus: got %d, want 1", bonuses.ForagingBonus)
	}
	if bonuses.DeathModifier != -2.5 {
		t.Errorf("DeathModifier: got %f, want -2.5", bonuses.DeathModifier)
	}
}
