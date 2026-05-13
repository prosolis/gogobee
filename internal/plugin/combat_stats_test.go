package plugin

import (
	"testing"
)

func testEquip(tier int) map[EquipmentSlot]*AdvEquipment {
	equip := make(map[EquipmentSlot]*AdvEquipment)
	for _, slot := range allSlots {
		equip[slot] = &AdvEquipment{
			Slot:      slot,
			Tier:      tier,
			Condition: 100,
		}
	}
	return equip
}

func testChar(combatLevel int) *AdventureCharacter {
	return &AdventureCharacter{
		CombatLevel: combatLevel,
		Alive:       true,
	}
}

// balanceTestDnDChar synthesizes a fighter sheet at the given dnd level for
// balance regression sims. Mirrors ensureDnDCharacterForCombat →
// autoBuildCharacter. CombatLevel is dead — combat keys off this sheet
// (HP/AC/AB/weapon dice) plus gear bonuses, nothing else.
func balanceTestDnDChar(dndLevel int) *DnDCharacter {
	scores := classStatPriority(ClassFighter)
	c := &DnDCharacter{
		Class: ClassFighter, Level: dndLevel,
		STR: scores[0], DEX: scores[1], CON: scores[2],
		INT: scores[3], WIS: scores[4], CHA: scores[5],
	}
	conMod := abilityModifier(c.CON)
	dexMod := abilityModifier(c.DEX)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, dexMod)
	return c
}

func zeroBonuses() *AdvBonusSummary {
	return &AdvBonusSummary{}
}

func TestDerivePlayerStats_BaseStats(t *testing.T) {
	// Post HP-unification + CL-strip: DerivePlayerStats no longer scales
	// off CombatLevel. With zero gear, output is the constant baseline that
	// applyDnDPlayerLayer then layers c.HPMax / weapon dice / etc. on top.
	char := testChar(10)
	equip := testEquip(0)
	stats, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)

	if stats.MaxHP != 50 {
		t.Errorf("zero-gear MaxHP = %d, want 50 (legacy base used only for HPBonus calc)", stats.MaxHP)
	}
	if stats.HPBonus != 0 {
		t.Errorf("zero-gear HPBonus = %d, want 0 (no gear/arena/housing buffs)", stats.HPBonus)
	}
	if stats.Attack != 5 {
		t.Errorf("zero-gear Attack = %d, want 5 (legacy fallback only; production uses weapon dice)", stats.Attack)
	}
}

func TestDerivePlayerStats_EquipmentScales(t *testing.T) {
	char := testChar(10)
	statsT1, _ := DerivePlayerStats(char, testEquip(1), zeroBonuses(), 0, 0, false)
	statsT3, _ := DerivePlayerStats(char, testEquip(3), zeroBonuses(), 0, 0, false)
	statsT5, _ := DerivePlayerStats(char, testEquip(5), zeroBonuses(), 0, 0, false)

	if statsT3.Attack <= statsT1.Attack {
		t.Errorf("T3 Attack (%d) should exceed T1 (%d)", statsT3.Attack, statsT1.Attack)
	}
	if statsT5.Attack <= statsT3.Attack {
		t.Errorf("T5 Attack (%d) should exceed T3 (%d)", statsT5.Attack, statsT3.Attack)
	}
	if statsT5.Defense <= statsT1.Defense {
		t.Errorf("T5 Defense (%d) should exceed T1 (%d)", statsT5.Defense, statsT1.Defense)
	}
	if statsT5.Speed <= statsT1.Speed {
		t.Errorf("T5 Speed (%d) should exceed T1 (%d)", statsT5.Speed, statsT1.Speed)
	}
}

func TestDerivePlayerStats_ConditionDegrades(t *testing.T) {
	char := testChar(10)
	fullEquip := testEquip(3)
	damagedEquip := testEquip(3)
	for _, eq := range damagedEquip {
		eq.Condition = 20
	}

	statsFull, _ := DerivePlayerStats(char, fullEquip, zeroBonuses(), 0, 0, false)
	statsDmg, _ := DerivePlayerStats(char, damagedEquip, zeroBonuses(), 0, 0, false)

	if statsDmg.Attack >= statsFull.Attack {
		t.Errorf("damaged Attack (%d) should be less than full (%d)", statsDmg.Attack, statsFull.Attack)
	}
	if statsDmg.Defense >= statsFull.Defense {
		t.Errorf("damaged Defense (%d) should be less than full (%d)", statsDmg.Defense, statsFull.Defense)
	}
}

func TestDerivePlayerStats_ChampionsSet(t *testing.T) {
	char := testChar(20)
	equip := testEquip(3)
	for _, eq := range equip {
		eq.ArenaTier = 3
		eq.ArenaSet = "champions"
	}

	stats, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)

	// Compare against same tier without set
	equip2 := testEquip(3)
	for _, eq := range equip2 {
		eq.ArenaTier = 3
	}
	statsNoSet, _ := DerivePlayerStats(char, equip2, zeroBonuses(), 0, 0, false)

	if stats.MaxHP <= statsNoSet.MaxHP {
		t.Errorf("champions MaxHP (%d) should exceed no-set (%d)", stats.MaxHP, statsNoSet.MaxHP)
	}
	if stats.Attack <= statsNoSet.Attack {
		t.Errorf("champions Attack (%d) should exceed no-set (%d)", stats.Attack, statsNoSet.Attack)
	}
}

func TestDerivePlayerStats_StreakBonuses(t *testing.T) {
	char := testChar(15)
	equip := testEquip(2)

	stats0, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)
	stats7, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 7, false)
	stats30, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 30, false)

	if stats7.Attack <= stats0.Attack {
		t.Errorf("7-day streak Attack (%d) should exceed 0-streak (%d)", stats7.Attack, stats0.Attack)
	}
	if stats30.Attack <= stats7.Attack {
		t.Errorf("30-day streak Attack (%d) should exceed 7-streak (%d)", stats30.Attack, stats7.Attack)
	}
}

func TestDerivePlayerStats_GrudgeBonus(t *testing.T) {
	char := testChar(15)
	equip := testEquip(2)

	statsNo, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)
	statsYes, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, true)

	if statsYes.Attack <= statsNo.Attack {
		t.Errorf("grudge Attack (%d) should exceed no-grudge (%d)", statsYes.Attack, statsNo.Attack)
	}
}

func TestDerivePlayerStats_HousingHP(t *testing.T) {
	char := testChar(10)
	char.HouseTier = 4 // +20% HP
	equip := testEquip(2)

	stats, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)

	charNoHouse := testChar(10)
	statsNoHouse, _ := DerivePlayerStats(charNoHouse, equip, zeroBonuses(), 0, 0, false)

	if stats.MaxHP <= statsNoHouse.MaxHP {
		t.Errorf("T4 housing MaxHP (%d) should exceed no-house (%d)", stats.MaxHP, statsNoHouse.MaxHP)
	}
}

func TestDerivePlayerStats_PetModifiers(t *testing.T) {
	char := testChar(10)
	char.PetType = "cat"
	char.PetArrived = true
	char.PetLevel = 5

	_, mods := DerivePlayerStats(char, testEquip(2), zeroBonuses(), 0, 0, false)

	if mods.PetAttackProc == 0 {
		t.Error("pet attack proc should be non-zero")
	}
	if mods.PetDeflectProc == 0 {
		t.Error("pet deflect proc should be non-zero")
	}
	if mods.PetWhiffProc == 0 {
		t.Error("pet whiff proc should be non-zero")
	}
	if mods.PetAttackDmg == 0 {
		t.Error("pet attack damage should be non-zero")
	}
}

func TestDerivePlayerStats_ChatPerks(t *testing.T) {
	char := testChar(10)
	equip := testEquip(2)

	stats0, _ := DerivePlayerStats(char, equip, zeroBonuses(), 0, 0, false)
	stats50, _ := DerivePlayerStats(char, equip, zeroBonuses(), 50, 0, false)

	if stats50.CritRate <= stats0.CritRate {
		t.Errorf("L50 chat CritRate (%.3f) should exceed L0 (%.3f)", stats50.CritRate, stats0.CritRate)
	}
}

func TestDerivePlayerStats_RateCaps(t *testing.T) {
	char := testChar(50)
	equip := testEquip(5)
	for _, eq := range equip {
		eq.ArenaTier = 5
		eq.ArenaSet = "bloodied"
	}
	bonuses := &AdvBonusSummary{ExceptionalBonus: 50}

	stats, _ := DerivePlayerStats(char, equip, bonuses, 50, 0, false)

	if stats.CritRate > 0.50 {
		t.Errorf("CritRate %f exceeds 50%% cap", stats.CritRate)
	}
	if stats.DodgeRate > 0.40 {
		t.Errorf("DodgeRate %f exceeds 40%% cap", stats.DodgeRate)
	}
	if stats.BlockRate > 0.40 {
		t.Errorf("BlockRate %f exceeds 40%% cap", stats.BlockRate)
	}
}

// ── Monster Stat Tests ───────────────────────────────────────────────────────

func TestDeriveArenaMonsterStats_Scales(t *testing.T) {
	weak := &ArenaMonster{Name: "Rat", BaseLethality: 0.10, ThreatLevel: 3}
	strong := &ArenaMonster{Name: "Dragon", BaseLethality: 0.85, ThreatLevel: 70}

	sW, _ := DeriveArenaMonsterStats(weak)
	sS, _ := DeriveArenaMonsterStats(strong)

	if sS.MaxHP <= sW.MaxHP {
		t.Error("strong monster should have more HP")
	}
	if sS.Attack <= sW.Attack {
		t.Error("strong monster should have more Attack")
	}
}

func TestDeriveDungeonMonsterStats_TierScaling(t *testing.T) {
	t1 := &AdvLocation{Tier: 1, BaseDeathPct: 8}
	t5 := &AdvLocation{Tier: 5, BaseDeathPct: 60}

	s1, _ := DeriveDungeonMonsterStats(t1)
	s5, _ := DeriveDungeonMonsterStats(t5)

	if s5.MaxHP <= s1.MaxHP {
		t.Error("T5 dungeon monster should have more HP than T1")
	}
	if s5.Attack <= s1.Attack {
		t.Error("T5 dungeon monster should have more Attack than T1")
	}
}

// ── Balance Regression: 10k Sims ─────────────────────────────────────────────

func TestBalanceRegression_DungeonDeathRates(t *testing.T) {
	// Players at the right level with tier-appropriate gear should be dominant.
	// Death comes from bad luck (crits, environmental hazards) — not raw stat disadvantage.
	// Underleveled/underequipped players face higher death rates (tested separately).
	// These are "well-equipped at tier" baselines.
	type tc struct {
		level    int
		equipT   int
		dungeon  AdvLocation
		maxDeath float64
		minDeath float64
	}
	// `level` is the dnd_level (CombatLevel is no longer consulted for combat
	// scaling — gear + dnd sheet drive everything).
	cases := []tc{
		{1, 1, advDungeons[0], 0.05, 0.0},    // T1: trivial with proper gear
		{2, 2, advDungeons[1], 0.10, 0.0},    // T2: very easy at level
		{5, 3, advDungeons[2], 0.15, 0.0},    // T3: low risk at level with T3 gear
		{7, 4, advDungeons[3], 0.25, 0.0},    // T4: some risk even geared
		{9, 5, advDungeons[4], 0.35, 0.01},   // T5: real danger — monster stats catch up
	}

	for _, c := range cases {
		char := testChar(c.level)
		equip := testEquip(c.equipT)
		bonuses := zeroBonuses()
		stats, mods := DerivePlayerStats(char, equip, bonuses, 0, 0, false)
		eStats, eMods := DeriveDungeonMonsterStats(&c.dungeon)

		// Apply the D&D layers so the test reflects production combat
		// HP/AC/AB/weapon-dice (post HP-unification, raw DerivePlayerStats
		// no longer matches what players actually fight with).
		dndChar := balanceTestDnDChar(c.level)
		applyDnDPlayerLayer(&stats, dndChar)
		applyDnDEquipmentLayer(&stats, dndChar, equip)
		applyDnDDungeonMonsterLayer(&eStats, c.dungeon.Tier)

		player := Combatant{Name: "Hero", Stats: stats, Mods: mods, IsPlayer: true}
		enemy := Combatant{Name: c.dungeon.Name, Stats: eStats, Mods: eMods}

		deaths := 0
		const N = 10000
		for i := 0; i < N; i++ {
			r := SimulateCombat(player, enemy, dungeonCombatPhases)
			if !r.PlayerWon {
				deaths++
			}
		}
		rate := float64(deaths) / float64(N)
		t.Logf("%s (L%d T%d): P[HP=%d AC=%d AB=%d Def=%d] E[HP=%d AC=%d AB=%d Atk=%d] → death=%.2f",
			c.dungeon.Name, c.level, c.equipT,
			stats.MaxHP, stats.AC, stats.AttackBonus, stats.Defense,
			eStats.MaxHP, eStats.AC, eStats.AttackBonus, eStats.Attack,
			rate)
		if rate < c.minDeath || rate > c.maxDeath {
			t.Errorf("  FAIL: death rate %.2f outside [%.2f, %.2f]",
				rate, c.minDeath, c.maxDeath)
		}
	}
}

func TestBalanceRegression_UnderleveledPlayers(t *testing.T) {
	// Underleveled / undergeared players should face real danger.
	type tc struct {
		name     string
		level    int
		equipT   int
		dungeon  AdvLocation
		minDeath float64
	}
	// `level` is dnd_level. Underleveled = dnd_level low for the dungeon tier.
	cases := []tc{
		{"L1 in T2 dungeon", 1, 0, advDungeons[1], 0.40},
		{"L2 in T3 dungeon", 2, 1, advDungeons[2], 0.30},
		{"L4 in T4 dungeon, T2 gear", 4, 2, advDungeons[3], 0.25},
		{"L6 in T5 dungeon, T3 gear", 6, 3, advDungeons[4], 0.30},
	}

	for _, c := range cases {
		char := testChar(c.level)
		equip := testEquip(c.equipT)
		bonuses := zeroBonuses()
		stats, mods := DerivePlayerStats(char, equip, bonuses, 0, 0, false)
		eStats, eMods := DeriveDungeonMonsterStats(&c.dungeon)

		// Apply the D&D layers so the test reflects production combat
		// HP/AC/AB/weapon-dice (post HP-unification, raw DerivePlayerStats
		// no longer matches what players actually fight with).
		dndChar := balanceTestDnDChar(c.level)
		applyDnDPlayerLayer(&stats, dndChar)
		applyDnDEquipmentLayer(&stats, dndChar, equip)
		applyDnDDungeonMonsterLayer(&eStats, c.dungeon.Tier)

		player := Combatant{Name: "Hero", Stats: stats, Mods: mods, IsPlayer: true}
		enemy := Combatant{Name: c.dungeon.Name, Stats: eStats, Mods: eMods}

		deaths := 0
		const N = 5000
		for i := 0; i < N; i++ {
			r := SimulateCombat(player, enemy, dungeonCombatPhases)
			if !r.PlayerWon {
				deaths++
			}
		}
		rate := float64(deaths) / float64(N)
		t.Logf("%s: P[HP=%d AC=%d AB=%d] E[HP=%d AC=%d AB=%d Atk=%d] → death=%.2f (want ≥%.2f)",
			c.name, stats.MaxHP, stats.AC, stats.AttackBonus,
			eStats.MaxHP, eStats.AC, eStats.AttackBonus, eStats.Attack, rate, c.minDeath)
		if rate < c.minDeath {
			t.Errorf("  FAIL: death rate %.2f below minimum %.2f", rate, c.minDeath)
		}
	}
}
