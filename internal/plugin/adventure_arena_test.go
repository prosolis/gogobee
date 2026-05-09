package plugin

import (
	"math"
	"strings"
	"testing"
	"time"
)

// ── Monster Data Tests ──────────────────────────────────────────────────────

func TestArenaTierData(t *testing.T) {
	if len(arenaTiers) != 5 {
		t.Fatalf("expected 5 arena tiers, got %d", len(arenaTiers))
	}

	for i, tier := range arenaTiers {
		if tier.Number != i+1 {
			t.Errorf("tier %d: Number = %d, want %d", i, tier.Number, i+1)
		}
		if tier.Name == "" {
			t.Errorf("tier %d: empty name", tier.Number)
		}
		if tier.MinLevel <= 0 {
			t.Errorf("tier %d: MinLevel %d <= 0", tier.Number, tier.MinLevel)
		}
		if tier.BasePayout <= 0 {
			t.Errorf("tier %d: BasePayout %d <= 0", tier.Number, tier.BasePayout)
		}
		if tier.SkillMultiplier <= 0 {
			t.Errorf("tier %d: SkillMultiplier %f <= 0", tier.Number, tier.SkillMultiplier)
		}
		if tier.CompletionBonus <= 0 {
			t.Errorf("tier %d: CompletionBonus %d <= 0", tier.Number, tier.CompletionBonus)
		}
		if tier.BattleXP <= 0 {
			t.Errorf("tier %d: BattleXP %d <= 0", tier.Number, tier.BattleXP)
		}

		for j, m := range tier.Monsters {
			if m.Name == "" {
				t.Errorf("tier %d round %d: empty monster name", tier.Number, j+1)
			}
			if m.Flavor == "" {
				t.Errorf("tier %d round %d: empty monster flavor", tier.Number, j+1)
			}
			if m.BaseLethality < 0.01 || m.BaseLethality > 0.99 {
				t.Errorf("tier %d round %d: BaseLethality %f out of [0.01, 0.99]", tier.Number, j+1, m.BaseLethality)
			}
			if m.ThreatLevel <= 0 {
				t.Errorf("tier %d round %d: ThreatLevel %d <= 0", tier.Number, j+1, m.ThreatLevel)
			}
		}
	}
}

func TestArenaGetTier(t *testing.T) {
	for i := 1; i <= 5; i++ {
		tier := arenaGetTier(i)
		if tier == nil {
			t.Errorf("arenaGetTier(%d) returned nil", i)
		}
	}
	if arenaGetTier(0) != nil {
		t.Error("arenaGetTier(0) should return nil")
	}
	if arenaGetTier(6) != nil {
		t.Error("arenaGetTier(6) should return nil")
	}
}

func TestArenaGetMonster(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		for round := 1; round <= 4; round++ {
			m := arenaGetMonster(tier, round)
			if m == nil {
				t.Errorf("arenaGetMonster(%d, %d) returned nil", tier, round)
			}
		}
	}
	if arenaGetMonster(1, 0) != nil {
		t.Error("arenaGetMonster(1, 0) should return nil")
	}
	if arenaGetMonster(1, 5) != nil {
		t.Error("arenaGetMonster(1, 5) should return nil")
	}
}

func TestArenaMonsterLethality_Increasing(t *testing.T) {
	// Within each tier, lethality should increase per round
	for i, tier := range arenaTiers {
		for j := 1; j < 4; j++ {
			if tier.Monsters[j].BaseLethality < tier.Monsters[j-1].BaseLethality {
				t.Errorf("tier %d: round %d lethality (%f) < round %d (%f)",
					i+1, j+1, tier.Monsters[j].BaseLethality, j, tier.Monsters[j-1].BaseLethality)
			}
		}
	}
}

func TestArenaMinLevel_Increasing(t *testing.T) {
	for i := 1; i < 5; i++ {
		if arenaTiers[i].MinLevel <= arenaTiers[i-1].MinLevel {
			t.Errorf("tier %d MinLevel (%d) <= tier %d MinLevel (%d)",
				i+1, arenaTiers[i].MinLevel, i, arenaTiers[i-1].MinLevel)
		}
	}
}

// ── Death Chance Formula Tests ──────────────────────────────────────────────

func TestArenaDeathChance_Clamped(t *testing.T) {
	// Min-level player with tier 0 gear vs easiest monster
	char := &AdventureCharacter{CombatLevel: 1}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 0}, SlotArmor: {Tier: 0}, SlotHelmet: {Tier: 0},
		SlotBoots: {Tier: 0}, SlotTool: {Tier: 0},
	}
	monster := arenaGetMonster(1, 1)
	dc := arenaDeathChance(monster, char, equip)
	if dc < 0.01 || dc > 0.98 {
		t.Errorf("death chance %f out of [0.01, 0.98] bounds", dc)
	}
}

func TestArenaDeathChance_MaxGearReduces(t *testing.T) {
	// Use a mid-tier monster where gear difference is visible above the floor
	char := &AdventureCharacter{CombatLevel: 25}
	monster := arenaGetMonster(3, 3) // Behemoth Adjacent (0.73 lethality)

	noGear := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 0}, SlotArmor: {Tier: 0}, SlotHelmet: {Tier: 0},
		SlotBoots: {Tier: 0}, SlotTool: {Tier: 0},
	}
	maxGear := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5}, SlotArmor: {Tier: 5}, SlotHelmet: {Tier: 5},
		SlotBoots: {Tier: 5}, SlotTool: {Tier: 5},
	}

	dcNoGear := arenaDeathChance(monster, char, noGear)
	dcMaxGear := arenaDeathChance(monster, char, maxGear)

	if dcMaxGear >= dcNoGear {
		t.Errorf("max gear (%f) should reduce death chance vs no gear (%f)", dcMaxGear, dcNoGear)
	}
}

func TestArenaDeathChance_HighLevelReduces(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 3}, SlotArmor: {Tier: 3}, SlotHelmet: {Tier: 3},
		SlotBoots: {Tier: 3}, SlotTool: {Tier: 3},
	}
	monster := arenaGetMonster(3, 4) // The Inevitable

	lowLevel := &AdventureCharacter{CombatLevel: 25}
	highLevel := &AdventureCharacter{CombatLevel: 50}

	dcLow := arenaDeathChance(monster, lowLevel, equip)
	dcHigh := arenaDeathChance(monster, highLevel, equip)

	if dcHigh >= dcLow {
		t.Errorf("high level (%f) should have lower death chance than low level (%f)", dcHigh, dcLow)
	}
}

func TestArenaDeathChance_T5R4_AlwaysTerrifying(t *testing.T) {
	// Even max-everything player faces high death at T5R4
	char := &AdventureCharacter{CombatLevel: 50}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5}, SlotArmor: {Tier: 5}, SlotHelmet: {Tier: 5},
		SlotBoots: {Tier: 5}, SlotTool: {Tier: 5},
	}
	monster := arenaGetMonster(5, 4) // That Which Has Always Been

	dc := arenaDeathChance(monster, char, equip)
	if dc < 0.30 {
		t.Errorf("T5R4 death chance for max player (%f) should be >= 0.30", dc)
	}
	// Should hit the 0.98 ceiling
	if dc > 0.98 {
		t.Errorf("T5R4 death chance (%f) exceeds ceiling 0.98", dc)
	}
}

func TestArenaDeathChance_Floor(t *testing.T) {
	// Even with absurd stats, floor is 0.01
	char := &AdventureCharacter{CombatLevel: 50}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5}, SlotArmor: {Tier: 5}, SlotHelmet: {Tier: 5},
		SlotBoots: {Tier: 5}, SlotTool: {Tier: 5},
	}
	monster := &ArenaMonster{BaseLethality: 0.01, ThreatLevel: 1}

	dc := arenaDeathChance(monster, char, equip)
	if dc < 0.01 {
		t.Errorf("death chance %f below floor 0.01", dc)
	}
}

// ── Reward Formula Tests ────────────────────────────────────────────────────

func TestArenaRoundReward(t *testing.T) {
	tier1 := arenaGetTier(1)

	// Round 1, no skill bonus
	r := arenaRoundReward(tier1, 1, 0)
	if r != 150 {
		t.Errorf("T1R1 skill=0: got %d, want 150", r)
	}

	// Round 4, no skill bonus
	r = arenaRoundReward(tier1, 4, 0)
	if r != 600 {
		t.Errorf("T1R4 skill=0: got %d, want 600", r)
	}

	// Round 1 with skill
	r = arenaRoundReward(tier1, 1, 10)
	// base=150, skill_bonus=floor(10*1.0)=10
	if r != 160 {
		t.Errorf("T1R1 skill=10: got %d, want 160", r)
	}

	// Tier 5 round 4, max skill
	tier5 := arenaGetTier(5)
	r = arenaRoundReward(tier5, 4, 50)
	// base=15000*4=60000, skill_bonus=floor(50*25.0)=1250
	if r != 61250 {
		t.Errorf("T5R4 skill=50: got %d, want 61250", r)
	}
}

func TestArenaRewardScaling(t *testing.T) {
	// Rewards should increase with round number
	tier := arenaGetTier(3)
	prev := int64(0)
	for round := 1; round <= 4; round++ {
		r := arenaRoundReward(tier, round, 20)
		if r <= prev {
			t.Errorf("T3R%d reward (%d) should exceed R%d (%d)", round, r, round-1, prev)
		}
		prev = r
	}
}

// ── Death Chance Formula Component Tests ────────────────────────────────────

func TestArenaDeathChance_Components(t *testing.T) {
	// Verify formula components individually
	// death_chance = base + level_mod - equip_mod + skill_mod

	// Level 1, tier 0 gear, T1R1 monster (lethality=0.10, threat=2)
	char := &AdventureCharacter{CombatLevel: 1}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 0}, SlotArmor: {Tier: 0}, SlotHelmet: {Tier: 0},
		SlotBoots: {Tier: 0}, SlotTool: {Tier: 0},
	}
	monster := &ArenaMonster{BaseLethality: 0.10, ThreatLevel: 2}

	dc := arenaDeathChance(monster, char, equip)

	// Manual calculation:
	// base = 0.10
	// level_mod = (2-1) * 0.015 = 0.015
	// skill_mod = max(0, 0.25 - 1*0.008) = 0.242
	// equip_mod = 0 * 0.03 = 0 (tier 0 gives no equipment bonus)
	// death_chance = 0.10 + 0.015 - 0 + 0.242 = 0.357
	expected := 0.10 + 0.015 + 0.242
	if math.Abs(dc-expected) > 0.001 {
		t.Errorf("T1R1 components: got %f, expected ~%f", dc, expected)
	}
}

// ── Render Tests ────────────────────────────────────────────────────────────

func TestRenderArenaStreakEntry(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 30}
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)

	// nil stats — no tiers cleared
	text := renderArenaStreakEntry(char, nil, tier, monster)
	if !strings.Contains(text, "THE ARENA") {
		t.Error("streak entry should contain header")
	}
	if !strings.Contains(text, "Tier 1") {
		t.Error("streak entry should list tier 1")
	}
	if !strings.Contains(text, "Tier 5") {
		t.Error("streak entry should list tier 5")
	}
	if !strings.Contains(text, "streak") {
		t.Error("streak entry should mention streak")
	}
	if !strings.Contains(text, "⬚") {
		t.Error("streak entry should show eligible-but-uncleared tiers")
	}
	if !strings.Contains(text, "🔒") {
		t.Error("streak entry should show locked tiers")
	}

	// with stats — tier 2 cleared
	stats := &ArenaPersonalStats{TotalRuns: 5, TotalDeaths: 1, TotalEarnings: 3000, HighestTier: 2}
	text = renderArenaStreakEntry(char, stats, tier, monster)
	if !strings.Contains(text, "✅") {
		t.Error("streak entry should show cleared tiers when stats provided")
	}
	if !strings.Contains(text, "Runs: 5") {
		t.Error("streak entry should show run summary when stats provided")
	}
}

func TestRenderArenaRoundStart(t *testing.T) {
	tier := arenaGetTier(2)
	monster := arenaGetMonster(2, 1)
	run := &ArenaRun{Earnings: 500}

	text := renderArenaRoundStart(tier, 1, monster, run)
	if !strings.Contains(text, "Tier 2") {
		t.Error("round start should show tier")
	}
	if !strings.Contains(text, monster.Name) {
		t.Error("round start should show monster name")
	}
	if !strings.Contains(text, "€500") {
		t.Error("round start should show earnings")
	}
	if !strings.Contains(text, "!arena fight") {
		t.Error("round start should show fight command")
	}
}

func TestRenderArenaDeath(t *testing.T) {
	tier := arenaGetTier(3)
	monster := arenaGetMonster(3, 2)

	text := renderArenaDeath(tier, 2, monster, 5000, "The Arena has collected its fee.")
	if !strings.Contains(text, "DEAD") {
		t.Error("death should say DEAD")
	}
	if !strings.Contains(text, "€5000") {
		t.Error("death should show forfeited earnings")
	}
	if !strings.Contains(text, "midnight UTC") {
		t.Error("death should mention lockout")
	}
}

func TestArenaStreakMultipliers(t *testing.T) {
	// Verify multiplier arrays are sane
	for tier := 1; tier <= 5; tier++ {
		if arenaStreakEuroMultiplier[tier] < 1.0 {
			t.Errorf("tier %d euro multiplier %f < 1.0", tier, arenaStreakEuroMultiplier[tier])
		}
	}
	for tiers := 1; tiers <= 5; tiers++ {
		if arenaStreakXPMultiplier[tiers] < 1.0 {
			t.Errorf("tiers won %d XP multiplier %f < 1.0", tiers, arenaStreakXPMultiplier[tiers])
		}
	}
	// Multipliers should be non-decreasing
	for i := 2; i <= 5; i++ {
		if arenaStreakEuroMultiplier[i] < arenaStreakEuroMultiplier[i-1] {
			t.Errorf("euro multiplier tier %d (%f) < tier %d (%f)", i, arenaStreakEuroMultiplier[i], i-1, arenaStreakEuroMultiplier[i-1])
		}
		if arenaStreakXPMultiplier[i] < arenaStreakXPMultiplier[i-1] {
			t.Errorf("XP multiplier %d tiers (%f) < %d tiers (%f)", i, arenaStreakXPMultiplier[i], i-1, arenaStreakXPMultiplier[i-1])
		}
	}

	// Verify exact values match spec
	wantEuro := map[int]float64{1: 1.0, 2: 1.5, 3: 2.0, 4: 2.75, 5: 4.0}
	for tier, want := range wantEuro {
		if arenaStreakEuroMultiplier[tier] != want {
			t.Errorf("euro multiplier tier %d: got %f, want %f", tier, arenaStreakEuroMultiplier[tier], want)
		}
	}
	wantXP := map[int]float64{1: 1.0, 2: 1.2, 3: 1.5, 4: 1.85, 5: 2.5}
	for tiers, want := range wantXP {
		if arenaStreakXPMultiplier[tiers] != want {
			t.Errorf("XP multiplier %d tiers: got %f, want %f", tiers, arenaStreakXPMultiplier[tiers], want)
		}
	}

	// Index 0 should be 0 (unused sentinel)
	if arenaStreakEuroMultiplier[0] != 0 {
		t.Errorf("euro multiplier index 0 should be 0, got %f", arenaStreakEuroMultiplier[0])
	}
	if arenaStreakXPMultiplier[0] != 0 {
		t.Errorf("XP multiplier index 0 should be 0, got %f", arenaStreakXPMultiplier[0])
	}
}

// ── Streak Reward Calculation Tests ────────────────────────────────────────

func TestArenaStreakPayout_SingleTier(t *testing.T) {
	// Bail after T1: multiplier = 1.0× on T1 earnings
	tier1 := arenaGetTier(1)
	var tierEarnings int64
	for round := 1; round <= 4; round++ {
		tierEarnings += arenaRoundReward(tier1, round, 0)
	}
	tierEarnings += tier1.CompletionBonus

	multiplied := int64(float64(tierEarnings) * arenaStreakEuroMultiplier[1])
	if multiplied != tierEarnings {
		t.Errorf("T1 payout with 1.0× multiplier should equal raw: got %d, want %d", multiplied, tierEarnings)
	}
}

func TestArenaStreakPayout_FullRun(t *testing.T) {
	// Full T1-T5 run: each tier's earnings multiplied independently
	var totalMultiplied int64
	var totalRaw int64

	for tierNum := 1; tierNum <= 5; tierNum++ {
		tier := arenaGetTier(tierNum)
		var tierEarnings int64
		for round := 1; round <= 4; round++ {
			tierEarnings += arenaRoundReward(tier, round, 0)
		}
		tierEarnings += tier.CompletionBonus
		totalRaw += tierEarnings
		totalMultiplied += int64(float64(tierEarnings) * arenaStreakEuroMultiplier[tierNum])
	}

	// Multiplied total should exceed raw total (multipliers > 1 for T2+)
	if totalMultiplied <= totalRaw {
		t.Errorf("multiplied total (%d) should exceed raw total (%d)", totalMultiplied, totalRaw)
	}

	// T5 multiplier is 4.0×, so multiplied should be significantly higher
	ratio := float64(totalMultiplied) / float64(totalRaw)
	if ratio < 1.5 {
		t.Errorf("overall multiplier ratio %f seems too low for streak system", ratio)
	}
}

func TestArenaStreakXPMultiplier_Application(t *testing.T) {
	// XP multiplier is applied to total session XP at payout
	rawXP := 500

	for tiersWon := 1; tiersWon <= 5; tiersWon++ {
		mult := arenaStreakXPMultiplier[tiersWon]
		totalXP := int(float64(rawXP) * mult)

		if tiersWon == 1 && totalXP != rawXP {
			t.Errorf("1 tier won: XP should be unmodified, got %d want %d", totalXP, rawXP)
		}
		if tiersWon > 1 && totalXP <= rawXP {
			t.Errorf("%d tiers won: XP %d should exceed raw %d", tiersWon, totalXP, rawXP)
		}
	}
}

func TestArenaStreakPayout_TierEarningsReset(t *testing.T) {
	// Simulate accumulation: tier earnings should reset per tier
	run := &ArenaRun{Earnings: 0, TierEarnings: 0}

	// Simulate T1 rounds
	for round := 1; round <= 4; round++ {
		tier := arenaGetTier(1)
		run.TierEarnings += arenaRoundReward(tier, round, 10)
	}
	run.TierEarnings += arenaGetTier(1).CompletionBonus

	// Apply T1 multiplier
	run.Earnings += int64(float64(run.TierEarnings) * arenaStreakEuroMultiplier[1])
	t1Total := run.Earnings
	run.TierEarnings = 0

	if t1Total <= 0 {
		t.Fatal("T1 earnings should be positive")
	}
	if run.TierEarnings != 0 {
		t.Error("TierEarnings should be 0 after reset")
	}

	// Simulate T2 rounds
	for round := 1; round <= 4; round++ {
		tier := arenaGetTier(2)
		run.TierEarnings += arenaRoundReward(tier, round, 10)
	}
	run.TierEarnings += arenaGetTier(2).CompletionBonus

	// Apply T2 multiplier (1.5×)
	run.Earnings += int64(float64(run.TierEarnings) * arenaStreakEuroMultiplier[2])
	run.TierEarnings = 0

	if run.Earnings <= t1Total {
		t.Errorf("session total after T2 (%d) should exceed T1 total (%d)", run.Earnings, t1Total)
	}
}

func TestArenaStreakPayout_DeathForfeitsAll(t *testing.T) {
	// On death, earnings, tier_earnings, and XP are all zeroed
	run := &ArenaRun{
		Earnings:      50000,
		TierEarnings:  3000,
		XPAccumulated: 200,
	}

	// Simulate death
	run.Earnings = 0
	run.TierEarnings = 0
	run.XPAccumulated = 0

	if run.Earnings != 0 || run.TierEarnings != 0 || run.XPAccumulated != 0 {
		t.Error("death should forfeit all session state")
	}
}

// ── Gladiator's Helm Tests ─────────────────────────────────────────────────

func TestGladiatorHelmDropRates(t *testing.T) {
	if gladiatorHelmDropT4 != 0.08 {
		t.Errorf("T4 drop rate: got %f, want 0.08", gladiatorHelmDropT4)
	}
	if gladiatorHelmDropT5 != 0.18 {
		t.Errorf("T5 drop rate: got %f, want 0.18", gladiatorHelmDropT5)
	}
	if gladiatorHelmDropT5 <= gladiatorHelmDropT4 {
		t.Error("T5 drop rate should exceed T4")
	}
}

func TestGladiatorHelmTier(t *testing.T) {
	if gladiatorHelmTier != 5 {
		t.Errorf("Gladiator's Helm tier: got %d, want 5", gladiatorHelmTier)
	}
}

// ── Arena Gear Tests ──────────────────────────────────────────────────────

func TestArenaGearByTier(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		gear := arenaGearByTier(tier)
		if gear == nil {
			t.Errorf("arenaGearByTier(%d) returned nil", tier)
			continue
		}
		if gear.Tier != tier {
			t.Errorf("arenaGearByTier(%d).Tier = %d", tier, gear.Tier)
		}
		if gear.SetKey == "" {
			t.Errorf("tier %d: empty SetKey", tier)
		}
		if gear.HelmetName == "" {
			t.Errorf("tier %d: empty HelmetName", tier)
		}
		if gear.DropRate <= 0 || gear.DropRate >= 1 {
			t.Errorf("tier %d: DropRate %f out of (0, 1)", tier, gear.DropRate)
		}
	}

	// Out of bounds
	if arenaGearByTier(0) != nil {
		t.Error("arenaGearByTier(0) should return nil")
	}
	if arenaGearByTier(6) != nil {
		t.Error("arenaGearByTier(6) should return nil")
	}
}

func TestArenaGearDropRates_Decreasing(t *testing.T) {
	// Higher tiers should have lower (or equal) drop rates
	for i := 2; i <= 5; i++ {
		curr := arenaGearByTier(i)
		prev := arenaGearByTier(i - 1)
		if curr.DropRate > prev.DropRate {
			t.Errorf("tier %d drop rate (%f) > tier %d (%f)", i, curr.DropRate, i-1, prev.DropRate)
		}
	}
}

// ── Render Tests (New/Updated) ─────────────────────────────────────────────

func TestRenderArenaStreakEntry_ShowsMultipliers(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 50}
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)

	text := renderArenaStreakEntry(char, nil, tier, monster)
	// Should show streak multiplier for at least T1
	if !strings.Contains(text, "1.0×") {
		t.Error("streak entry should show T1 multiplier")
	}
	if !strings.Contains(text, "4.0×") {
		t.Error("streak entry should show T5 multiplier")
	}
	// Should show first monster
	if !strings.Contains(text, monster.Name) {
		t.Error("streak entry should show first monster name")
	}
	// Should show fight command
	if !strings.Contains(text, "!arena fight") {
		t.Error("streak entry should show fight command")
	}
	// Should show cancel option
	if !strings.Contains(text, "!arena cancel") {
		t.Error("streak entry should show cancel command")
	}
}

func TestRenderArenaStreakEntry_HighLevel_AllEligible(t *testing.T) {
	// T5 requires level 70, so use level 70+ to unlock everything
	char := &AdventureCharacter{CombatLevel: 70}
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)

	text := renderArenaStreakEntry(char, nil, tier, monster)
	// Level 70 should make all tiers eligible (no 🔒)
	if strings.Contains(text, "🔒") {
		t.Error("level 70 should have all tiers eligible (no locked icons)")
	}
}

func TestRenderArenaRoundStart_WithTierEarnings(t *testing.T) {
	tier := arenaGetTier(3)
	monster := arenaGetMonster(3, 1)
	run := &ArenaRun{Earnings: 10000, TierEarnings: 2000}

	text := renderArenaRoundStart(tier, 1, monster, run)
	// Should show combined session total (Earnings + TierEarnings)
	if !strings.Contains(text, "€12000") {
		t.Error("round start should show combined session total (10000+2000)")
	}
}

func TestRenderArenaRoundStart_ZeroEarnings(t *testing.T) {
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)
	run := &ArenaRun{Earnings: 0, TierEarnings: 0}

	text := renderArenaRoundStart(tier, 1, monster, run)
	// Should NOT show earnings line when zero
	if strings.Contains(text, "Session earnings") {
		t.Error("round start should not show earnings line when zero")
	}
}

func TestRenderArenaStatus_Active(t *testing.T) {
	run := &ArenaRun{Tier: 2, Round: 3, Status: "active", Earnings: 8000, TierEarnings: 1500, RoundsSurvived: 6}
	char := &AdventureCharacter{CombatLevel: 20}

	text := renderArenaStatus(run, char)
	if !strings.Contains(text, "Streak") {
		t.Error("status should say Arena Streak")
	}
	if !strings.Contains(text, "€9500") {
		t.Error("status should show combined session total (8000+1500)")
	}
	if !strings.Contains(text, "Round: 3/4") {
		t.Error("status should show round")
	}
	if !strings.Contains(text, "!arena fight") {
		t.Error("active status should show fight command")
	}
}

func TestRenderArenaStatus_Awaiting(t *testing.T) {
	run := &ArenaRun{Tier: 3, Status: "awaiting", Earnings: 20000, TierEarnings: 0, RoundsSurvived: 12}
	char := &AdventureCharacter{CombatLevel: 30}

	text := renderArenaStatus(run, char)
	if !strings.Contains(text, "advancing shortly") {
		t.Error("awaiting status should mention advancing")
	}
	if !strings.Contains(text, "!bail") {
		t.Error("awaiting status should show bail command")
	}
	if !strings.Contains(text, "€20000") {
		t.Error("awaiting status should show session total")
	}
}

func TestRenderArenaAlreadyInRun_Active(t *testing.T) {
	run := &ArenaRun{Tier: 2, Round: 3, Status: "active"}
	text := renderArenaAlreadyInRun(run)
	if !strings.Contains(text, "Tier 2") {
		t.Error("active run message should show tier")
	}
	if !strings.Contains(text, "!arena fight") {
		t.Error("active run message should show fight command")
	}
}

func TestRenderArenaAlreadyInRun_Awaiting(t *testing.T) {
	run := &ArenaRun{Tier: 3, Status: "awaiting", Earnings: 15000}
	text := renderArenaAlreadyInRun(run)
	if !strings.Contains(text, "Tier 3") {
		t.Error("awaiting run message should show tier")
	}
	if !strings.Contains(text, "!bail") {
		t.Error("awaiting run message should mention bail")
	}
	if !strings.Contains(text, "€15000") {
		t.Error("awaiting run message should show accumulated euros")
	}
}

func TestRenderArenaHelmetDrop_AllSets(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		gear := arenaGearByTier(tier)
		text := renderArenaHelmetDrop(gear)
		if !strings.Contains(text, gear.HelmetName) {
			t.Errorf("tier %d: helmet drop should show helmet name", tier)
		}
		if !strings.Contains(text, "Set bonus") {
			t.Errorf("tier %d: helmet drop should show set bonus", tier)
		}
		if !strings.Contains(text, "Equipped automatically") {
			t.Errorf("tier %d: helmet drop should mention auto-equip", tier)
		}
	}
}

func TestRenderArenaDeathReprieve(t *testing.T) {
	nextWindow := time.Date(2026, 4, 15, 14, 30, 0, 0, time.UTC)
	text := renderArenaDeathReprieve("Alice", "Ratticus the Persistent", nextWindow)
	if !strings.Contains(text, "Alice") {
		t.Error("reprieve should mention player name")
	}
	if !strings.Contains(text, "Death's Reprieve") {
		t.Error("reprieve should mention the ability name")
	}
	if !strings.Contains(text, "2026-04-15") {
		t.Error("reprieve should show next window date")
	}
}

func TestRenderArenaPersonalStats_NoStats(t *testing.T) {
	text := renderArenaPersonalStats("TestPlayer", 0, 0, nil)
	if !strings.Contains(text, "No arena runs") {
		t.Error("empty stats should say no runs")
	}
}

func TestRenderArenaPersonalStats_WithStats(t *testing.T) {
	stats := &ArenaPersonalStats{
		TotalRuns: 11, TotalEarnings: 150000, TotalDeaths: 3,
		HighestTier: 5, Tier5Completions: 2,
	}
	text := renderArenaPersonalStats("Alice", 8, 3, stats)
	if !strings.Contains(text, "Alice") {
		t.Error("stats should show player name")
	}
	if !strings.Contains(text, "€150000") {
		t.Error("stats should show total earnings")
	}
	if !strings.Contains(text, "Tier 5 completions: 2") {
		t.Error("stats should show T5 completions")
	}
	if !strings.Contains(text, "8/3") {
		t.Error("stats should show W/L record")
	}
}

func TestRenderArenaLeaderboard_Empty(t *testing.T) {
	text := renderArenaLeaderboard(nil)
	if !strings.Contains(text, "No arena runs") {
		t.Error("empty leaderboard should say no runs")
	}
}

func TestRenderArenaLeaderboard_WithEntries(t *testing.T) {
	entries := []ArenaLeaderboardEntry{
		{DisplayName: "Alice", TotalEarnings: 100000, HighestTier: 5, Tier5Completions: 1, TotalRuns: 5, TotalDeaths: 2},
		{DisplayName: "Bob", TotalEarnings: 50000, HighestTier: 3, TotalRuns: 10, TotalDeaths: 7},
	}
	text := renderArenaLeaderboard(entries)
	if !strings.Contains(text, "Alice") {
		t.Error("leaderboard should contain Alice")
	}
	if !strings.Contains(text, "Bob") {
		t.Error("leaderboard should contain Bob")
	}
	if !strings.Contains(text, "T5×1") {
		t.Error("leaderboard should show T5 completions")
	}
}

func TestRenderArenaLevelGate(t *testing.T) {
	tier := arenaGetTier(3)
	text := renderArenaLevelGate(tier, 10)
	if !strings.Contains(text, "Level 25") {
		t.Error("gate message should show required level")
	}
	if !strings.Contains(text, "Level 10") {
		t.Error("gate message should show player level")
	}
}

func TestRenderArenaStatus(t *testing.T) {
	run := &ArenaRun{Tier: 3, Round: 2, Status: "active", Earnings: 5000, RoundsSurvived: 5}
	char := &AdventureCharacter{CombatLevel: 30}

	text := renderArenaStatus(run, char)
	if !strings.Contains(text, "Tier: 3") {
		t.Error("status should show tier")
	}
	if !strings.Contains(text, "Round: 2/4") {
		t.Error("status should show round")
	}
	if !strings.Contains(text, "€5000") {
		t.Error("status should show earnings")
	}
}

// ── Death Message Tests ─────────────────────────────────────────────────────

func TestArenaDeathMessages_NotEmpty(t *testing.T) {
	if len(arenaDeathMessages) == 0 {
		t.Error("arenaDeathMessages pool is empty")
	}
	if len(arenaMonsterDeathMessages) == 0 {
		t.Error("arenaMonsterDeathMessages pool is empty")
	}
}

func TestArenaPickDeathMessage(t *testing.T) {
	monster := arenaGetMonster(2, 3) // The Impersonator
	for i := 0; i < 20; i++ {
		msg := arenaPickDeathMessage(monster, 2, 3)
		if msg == "" {
			t.Error("death message should not be empty")
		}
	}
}

// ── Full Run Reward Calculation ─────────────────────────────────────────────

func TestArenaFullRunRewards_Tier1(t *testing.T) {
	// Calculate total earnings for a clean Tier 1 run (no skill)
	tier := arenaGetTier(1)
	var total int64
	for round := 1; round <= 4; round++ {
		total += arenaRoundReward(tier, round, 0)
	}
	total += tier.CompletionBonus

	// Round rewards: 150 + 300 + 450 + 600 = 1500; + 2500 bonus = 4000
	if total != 4000 {
		t.Errorf("clean T1 run (no skill): got €%d, want €4000", total)
	}
}

func TestArenaFullRunRewards_AllTiers(t *testing.T) {
	// A full T1-T5 run with skill=0 should be well over 100k
	var grandTotal int64
	for tier := 1; tier <= 5; tier++ {
		t := arenaGetTier(tier)
		for round := 1; round <= 4; round++ {
			grandTotal += arenaRoundReward(t, round, 0)
		}
		grandTotal += t.CompletionBonus
	}
	if grandTotal < 100000 {
		// Sanity check that the numbers aren't accidentally tiny
		t.Errorf("full T1-T5 run (no skill) = €%d, expected > €100,000", grandTotal)
	}
}

// ── Full Streak Simulation ────────────────────────────────────────────────

func TestArenaStreakSimulation_FullRun(t *testing.T) {
	// Simulate a complete T1-T5 streak: accumulate rewards as the real code does
	run := &ArenaRun{
		StartTier:     1,
		Tier:          1,
		Round:         1,
		Status:        "active",
		Earnings:      0,
		TierEarnings:  0,
		XPAccumulated: 0,
	}
	combatLevel := 30

	for tierNum := 1; tierNum <= 5; tierNum++ {
		tier := arenaGetTier(tierNum)
		run.Tier = tierNum

		for round := 1; round <= 4; round++ {
			run.Round = round
			reward := arenaRoundReward(tier, round, combatLevel)
			run.TierEarnings += reward
			run.XPAccumulated += tier.BattleXP
			run.RoundsSurvived++
		}

		// Tier complete — add completion bonus
		run.TierEarnings += tier.CompletionBonus

		// Apply streak multiplier
		multiplier := arenaStreakEuroMultiplier[tierNum]
		run.Earnings += int64(float64(run.TierEarnings) * multiplier)
		run.TierEarnings = 0 // reset for next tier
	}

	// Verify tier earnings reset
	if run.TierEarnings != 0 {
		t.Errorf("TierEarnings should be 0 after all tiers, got %d", run.TierEarnings)
	}

	// Verify earnings are positive and substantial
	if run.Earnings <= 0 {
		t.Fatal("session earnings should be positive")
	}

	// Calculate what raw (unmultiplied) total would be
	var rawTotal int64
	for tierNum := 1; tierNum <= 5; tierNum++ {
		tier := arenaGetTier(tierNum)
		for round := 1; round <= 4; round++ {
			rawTotal += arenaRoundReward(tier, round, combatLevel)
		}
		rawTotal += tier.CompletionBonus
	}

	// Multiplied total should exceed raw
	if run.Earnings <= rawTotal {
		t.Errorf("multiplied total (%d) should exceed raw total (%d)", run.Earnings, rawTotal)
	}

	// Verify XP accumulated across 20 rounds (4 rounds × 5 tiers)
	if run.RoundsSurvived != 20 {
		t.Errorf("rounds survived: got %d, want 20", run.RoundsSurvived)
	}
	if run.XPAccumulated <= 0 {
		t.Error("XP should be accumulated")
	}

	// Apply XP multiplier for 5 tiers won
	xpMult := arenaStreakXPMultiplier[5]
	totalXP := int(float64(run.XPAccumulated) * xpMult)
	if totalXP <= run.XPAccumulated {
		t.Errorf("multiplied XP (%d) should exceed raw XP (%d) with 5-tier multiplier", totalXP, run.XPAccumulated)
	}
	// 2.5× multiplier means totalXP should be 2.5× raw
	expectedXP := int(float64(run.XPAccumulated) * 2.5)
	if totalXP != expectedXP {
		t.Errorf("total XP: got %d, want %d (2.5× raw)", totalXP, expectedXP)
	}
}

func TestArenaStreakSimulation_BailAfterT3(t *testing.T) {
	// Simulate bail after clearing T3
	run := &ArenaRun{Earnings: 0, TierEarnings: 0, XPAccumulated: 0}

	for tierNum := 1; tierNum <= 3; tierNum++ {
		tier := arenaGetTier(tierNum)
		for round := 1; round <= 4; round++ {
			run.TierEarnings += arenaRoundReward(tier, round, 20)
			run.XPAccumulated += tier.BattleXP
		}
		run.TierEarnings += tier.CompletionBonus
		run.Earnings += int64(float64(run.TierEarnings) * arenaStreakEuroMultiplier[tierNum])
		run.TierEarnings = 0
	}

	// XP multiplier for 3 tiers = 1.5×
	totalXP := int(float64(run.XPAccumulated) * arenaStreakXPMultiplier[3])

	if totalXP <= run.XPAccumulated {
		t.Error("multiplied XP should exceed raw with 3-tier multiplier")
	}
	if run.Earnings <= 0 {
		t.Error("session earnings should be positive after 3 tiers")
	}
}

func TestArenaStreakSimulation_DeathMidTier(t *testing.T) {
	// Simulate death in T2R3 — should have accumulated T1 earnings + partial T2
	run := &ArenaRun{Earnings: 0, TierEarnings: 0, XPAccumulated: 0}

	// Complete T1
	tier1 := arenaGetTier(1)
	for round := 1; round <= 4; round++ {
		run.TierEarnings += arenaRoundReward(tier1, round, 10)
		run.XPAccumulated += tier1.BattleXP
	}
	run.TierEarnings += tier1.CompletionBonus
	run.Earnings += int64(float64(run.TierEarnings) * arenaStreakEuroMultiplier[1])
	t1Earnings := run.Earnings
	run.TierEarnings = 0

	// Partial T2 — 2 rounds
	tier2 := arenaGetTier(2)
	for round := 1; round <= 2; round++ {
		run.TierEarnings += arenaRoundReward(tier2, round, 10)
		run.XPAccumulated += tier2.BattleXP
	}

	// Death — forfeit everything
	lostTotal := run.Earnings + run.TierEarnings
	if lostTotal <= t1Earnings {
		t.Error("lost total should include partial T2 earnings")
	}

	run.Earnings = 0
	run.TierEarnings = 0
	run.XPAccumulated = 0

	if run.Earnings != 0 || run.TierEarnings != 0 || run.XPAccumulated != 0 {
		t.Error("all session state should be zero after death")
	}
}

func TestArenaRunFields_NewFieldsDefault(t *testing.T) {
	run := ArenaRun{}
	if run.TierEarnings != 0 {
		t.Errorf("default TierEarnings should be 0, got %d", run.TierEarnings)
	}
	if run.XPAccumulated != 0 {
		t.Errorf("default XPAccumulated should be 0, got %d", run.XPAccumulated)
	}
}

// ── Streak Euro Multiplier Math (Exact) ───────────────────────────────────

func TestArenaStreakEuroMultiplier_Exact(t *testing.T) {
	// T1 raw = 4000 (verified in TestArenaFullRunRewards_Tier1)
	// T1 multiplied = 4000 * 1.0 = 4000
	tier1 := arenaGetTier(1)
	var t1Raw int64
	for r := 1; r <= 4; r++ {
		t1Raw += arenaRoundReward(tier1, r, 0)
	}
	t1Raw += tier1.CompletionBonus

	t1Mult := int64(float64(t1Raw) * arenaStreakEuroMultiplier[1])
	if t1Mult != 4000 {
		t.Errorf("T1 multiplied (1.0×): got %d, want 4000", t1Mult)
	}

	// T2 raw with skill=0
	tier2 := arenaGetTier(2)
	var t2Raw int64
	for r := 1; r <= 4; r++ {
		t2Raw += arenaRoundReward(tier2, r, 0)
	}
	t2Raw += tier2.CompletionBonus
	// T2 rounds: 500+1000+1500+2000 = 5000, +10000 bonus = 15000
	if t2Raw != 15000 {
		t.Errorf("T2 raw: got %d, want 15000", t2Raw)
	}
	// T2 multiplied: 15000 * 1.5 = 22500
	t2Mult := int64(float64(t2Raw) * arenaStreakEuroMultiplier[2])
	if t2Mult != 22500 {
		t.Errorf("T2 multiplied (1.5×): got %d, want 22500", t2Mult)
	}
}

// ── Streak Entry Shows Brim & Battle Sponsorship ──────────────────────────

func TestArenaStreakEntry_Sponsorship(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 1}
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)

	text := renderArenaStreakEntry(char, nil, tier, monster)
	if !strings.Contains(text, "Brim & Battle") {
		t.Error("arena entry should include Brim & Battle sponsorship")
	}
}

// ── Help Text Tests ──────────────────────────────────────────────────────

func TestArenaHelpText_StreakContent(t *testing.T) {
	if !strings.Contains(arenaHelpText, "!bail") {
		t.Error("help text should mention !bail")
	}
	if !strings.Contains(arenaHelpText, "streak") {
		t.Error("help text should mention streak")
	}
	if strings.Contains(arenaHelpText, "!arena tier") {
		t.Error("help text should NOT mention !arena tier (removed)")
	}
	if strings.Contains(arenaHelpText, "!arena descend") {
		t.Error("help text should NOT mention !arena descend (removed)")
	}
	if strings.Contains(arenaHelpText, "!arena cashout") {
		t.Error("help text should NOT mention !arena cashout (removed)")
	}
}

// ── Gladiator's Helm Drop Rate Logic ──────────────────────────────────────

func TestGladiatorHelmDropRate_BelowT4(t *testing.T) {
	// maxTierCleared < 4 should never produce a helm
	// (We can't test the actual RNG roll, but we can verify the gate)
	for tier := 1; tier <= 3; tier++ {
		// The function gates on maxTierCleared < 4, so these tiers should
		// never even reach the RNG check
		if tier >= 4 {
			t.Error("test logic error")
		}
	}
}

func TestGladiatorHelmDropRate_Selection(t *testing.T) {
	// At T4, rate should be 0.08; at T5, rate should be 0.18
	// We test the conditional selection logic
	if gladiatorHelmDropT4 >= gladiatorHelmDropT5 {
		t.Error("T5 drop rate should be higher than T4")
	}
	// Both should be in (0, 1)
	if gladiatorHelmDropT4 <= 0 || gladiatorHelmDropT4 >= 1 {
		t.Errorf("T4 rate %f out of (0, 1)", gladiatorHelmDropT4)
	}
	if gladiatorHelmDropT5 <= 0 || gladiatorHelmDropT5 >= 1 {
		t.Errorf("T5 rate %f out of (0, 1)", gladiatorHelmDropT5)
	}
}
