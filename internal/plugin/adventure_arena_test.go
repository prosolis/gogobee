package plugin

import (
	"math"
	"strings"
	"testing"
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

func TestRenderArenaTierMenu(t *testing.T) {
	char := &AdventureCharacter{CombatLevel: 30}

	// nil stats — no tiers cleared
	text := renderArenaTierMenu(char, nil)
	if !strings.Contains(text, "THE ARENA") {
		t.Error("tier menu should contain header")
	}
	if !strings.Contains(text, "Tier 1") {
		t.Error("tier menu should list tier 1")
	}
	if !strings.Contains(text, "Tier 5") {
		t.Error("tier menu should list tier 5")
	}
	// Level 30 should have tiers 1-3 eligible (⬚) and 4-5 locked (🔒)
	if !strings.Contains(text, "⬚") {
		t.Error("tier menu should show eligible-but-uncleared tiers")
	}
	if !strings.Contains(text, "🔒") {
		t.Error("tier menu should show locked tiers")
	}

	// with stats — tier 2 cleared
	stats := &ArenaPersonalStats{TotalRuns: 5, TotalDeaths: 1, TotalEarnings: 3000, HighestTier: 2}
	text = renderArenaTierMenu(char, stats)
	if !strings.Contains(text, "✅") {
		t.Error("tier menu should show cleared tiers when stats provided")
	}
	if !strings.Contains(text, "Runs: 5") {
		t.Error("tier menu should show run summary when stats provided")
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

func TestRenderArenaSurvival(t *testing.T) {
	tier := arenaGetTier(1)
	monster := arenaGetMonster(1, 1)

	text := renderArenaSurvival(tier, 1, monster, 150, 10, 150)
	if !strings.Contains(text, "defeated") {
		t.Error("survival should mention defeat")
	}
	if !strings.Contains(text, "€150") {
		t.Error("survival should show reward")
	}
	if !strings.Contains(text, "+10") {
		t.Error("survival should show XP")
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

func TestRenderArenaTierComplete(t *testing.T) {
	tier := arenaGetTier(2)
	text := renderArenaTierComplete(tier, 10000, 15000)

	if !strings.Contains(text, "cleared") {
		t.Error("tier complete should say cleared")
	}
	if !strings.Contains(text, "€10000") {
		t.Error("tier complete should show completion bonus")
	}
	if !strings.Contains(text, "descend") {
		t.Error("tier complete should offer descend")
	}
	if !strings.Contains(text, "cashout") {
		t.Error("tier complete should offer cashout")
	}
	if !strings.Contains(text, "10 minutes") {
		t.Error("tier complete should mention deadline")
	}
}

func TestRenderArenaTier5Complete(t *testing.T) {
	// Non-full run
	text := renderArenaTier5Complete(650000, 3)
	if !strings.Contains(text, "CONQUERED") {
		t.Error("T5 complete should say conquered")
	}
	if !strings.Contains(text, "€650000") {
		t.Error("T5 complete should show earnings")
	}
	if strings.Contains(text, "All the way down") {
		t.Error("non-full run should NOT mention all the way down")
	}

	// Full run (started from tier 1)
	textFull := renderArenaTier5Complete(1000000, 1)
	if !strings.Contains(textFull, "All the way down") {
		t.Error("full run should mention all the way down")
	}
}

func TestRenderArenaCashout(t *testing.T) {
	text := renderArenaCashout(25000, 3)
	if !strings.Contains(text, "€25000") {
		t.Error("cashout should show amount")
	}
	if !strings.Contains(text, "Tier 3") {
		t.Error("cashout should show last tier")
	}
}

func TestRenderArenaAutoCashout(t *testing.T) {
	text := renderArenaAutoCashout(10000)
	if !strings.Contains(text, "Auto-cashout") {
		t.Error("auto-cashout should identify itself")
	}
	if !strings.Contains(text, "€10000") {
		t.Error("auto-cashout should show amount")
	}
	if !strings.Contains(text, "annoyed") {
		t.Error("auto-cashout should mention GogoBee is annoyed")
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
