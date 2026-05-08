package plugin

import (
	"testing"
)

func TestProficiencyBonus(t *testing.T) {
	cases := []struct{ level, want int }{
		{1, 2}, {2, 2}, {3, 2}, {4, 2},
		{5, 3}, {6, 3}, {7, 3}, {8, 3},
		{9, 4}, {12, 4},
		{13, 5}, {16, 5},
		{17, 6}, {20, 6},
	}
	for _, c := range cases {
		if got := proficiencyBonus(c.level); got != c.want {
			t.Errorf("proficiencyBonus(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestClassAttackStatMod(t *testing.T) {
	c := &DnDCharacter{STR: 16, DEX: 12, CON: 14, INT: 10, WIS: 8, CHA: 18}
	cases := []struct {
		class DnDClass
		want  int
	}{
		{ClassFighter, 3}, // STR 16 → +3
		{ClassRogue, 1},   // DEX 12 → +1
		{ClassRanger, 1},
		{ClassMage, 0},    // INT 10 → 0
		{ClassCleric, -1}, // WIS 8 → -1
	}
	for _, tc := range cases {
		c.Class = tc.class
		if got := classAttackStatMod(c); got != tc.want {
			t.Errorf("classAttackStatMod(%s, STR=%d/DEX=%d/INT=%d/WIS=%d) = %d, want %d",
				tc.class, c.STR, c.DEX, c.INT, c.WIS, got, tc.want)
		}
	}
}

func TestDnDPlayerAttackBonus(t *testing.T) {
	// L1 Fighter, STR 17 (+3 mod), prof 2, weapon bonus 2 → +7
	fighter := &DnDCharacter{Class: ClassFighter, Level: 1, STR: 17}
	if got := dndPlayerAttackBonus(fighter); got != 7 {
		t.Errorf("L1 Fighter STR17 attack bonus = %d, want 7", got)
	}
	// L5 Mage, INT 16 (+3), prof 3, no weapon bonus → +6
	mage := &DnDCharacter{Class: ClassMage, Level: 5, INT: 16}
	if got := dndPlayerAttackBonus(mage); got != 6 {
		t.Errorf("L5 Mage INT16 attack bonus = %d, want 6", got)
	}
}

func TestApplyDnDPlayerLayer(t *testing.T) {
	stats := CombatStats{MaxHP: 50, Attack: 10, Defense: 5}
	c := &DnDCharacter{Class: ClassFighter, Level: 1, STR: 17, ArmorClass: 16}
	applyDnDPlayerLayer(&stats, c)
	if stats.AC != 16 {
		t.Errorf("AC = %d, want 16", stats.AC)
	}
	if stats.AttackBonus != 7 {
		t.Errorf("AttackBonus = %d, want 7", stats.AttackBonus)
	}
	// HP/Attack scaling fields unchanged
	if stats.MaxHP != 50 || stats.Attack != 10 {
		t.Errorf("non-D&D fields mutated: %+v", stats)
	}
}

func TestApplyDnDArenaMonsterLayer(t *testing.T) {
	stats := CombatStats{MaxHP: 100, Attack: 20}
	applyDnDArenaMonsterLayer(&stats, 12)
	// AC base 10 + 12*0.25 = 13
	if stats.AC != 13 {
		t.Errorf("AC = %d, want 13", stats.AC)
	}
	// Atk base 4 + 12*0.30 = 7 (int trunc)
	if stats.AttackBonus != 7 {
		t.Errorf("AttackBonus = %d, want 7", stats.AttackBonus)
	}
}

func TestClassStatPriority(t *testing.T) {
	// Each class's array must contain {15,14,13,12,10,8} exactly once.
	want := standardArray
	for _, class := range []DnDClass{ClassFighter, ClassRogue, ClassMage, ClassCleric, ClassRanger} {
		got := classStatPriority(class)
		if !isStandardArray(got) {
			t.Errorf("classStatPriority(%s) = %v, not a permutation of %v", class, got, want)
		}
	}
	// Fighter: STR (idx 0) is the highest stat.
	f := classStatPriority(ClassFighter)
	if f[0] != 15 {
		t.Errorf("Fighter STR = %d, want 15 (primary stat)", f[0])
	}
	// Mage: INT (idx 3) is the highest stat.
	m := classStatPriority(ClassMage)
	if m[3] != 15 {
		t.Errorf("Mage INT = %d, want 15 (primary stat)", m[3])
	}
	// Cleric: WIS (idx 4) is the highest stat.
	c := classStatPriority(ClassCleric)
	if c[4] != 15 {
		t.Errorf("Cleric WIS = %d, want 15 (primary stat)", c[4])
	}
}

func TestApplyDnDDungeonMonsterLayer(t *testing.T) {
	stats := CombatStats{}
	applyDnDDungeonMonsterLayer(&stats, 1)
	if stats.AC != 10 || stats.AttackBonus != 5 {
		t.Errorf("T1 monster: AC=%d Atk=%d, want AC=10 Atk=5", stats.AC, stats.AttackBonus)
	}
	stats = CombatStats{}
	applyDnDDungeonMonsterLayer(&stats, 5)
	if stats.AC != 14 || stats.AttackBonus != 9 {
		t.Errorf("T5 monster: AC=%d Atk=%d, want AC=14 Atk=9", stats.AC, stats.AttackBonus)
	}
}

// TestSimulateCombat_RollsAppear: every attack event in a normal fight should
// carry a d20 Roll in [1,20] and a RollAgainst (target AC).
func TestSimulateCombat_RollsAppear(t *testing.T) {
	player := Combatant{
		Name: "P", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 50, Attack: 15, Defense: 5, Speed: 8,
			AC: 14, AttackBonus: 5,
		},
		Mods: CombatModifiers{DamageReduct: 1.0},
	}
	enemy := Combatant{
		Name: "E",
		Stats: CombatStats{
			MaxHP: 30, Attack: 10, Defense: 3, Speed: 5,
			AC: 12, AttackBonus: 4,
		},
		Mods: CombatModifiers{DamageReduct: 1.0},
	}
	result := SimulateCombat(player, enemy, defaultCombatPhases)
	attackEvents := 0
	for _, ev := range result.Events {
		if ev.Actor != "player" && ev.Actor != "enemy" {
			continue
		}
		if ev.Action != "hit" && ev.Action != "miss" && ev.Action != "crit" && ev.Action != "block" {
			continue
		}
		attackEvents++
		if ev.Roll < 1 || ev.Roll > 20 {
			t.Errorf("roll out of range: %+v", ev)
		}
		if ev.RollAgainst <= 0 {
			t.Errorf("RollAgainst should be set: %+v", ev)
		}
	}
	if attackEvents == 0 {
		t.Error("no attack events produced")
	}
}

// TestD20HitRate_PlayerDominant: player +10 attack vs AC 11 should hit ~95%
// (any roll 2+ hits, plus nat 20). Run many trials for statistical bound.
func TestD20HitRate_PlayerDominant(t *testing.T) {
	hits, total := 0, 2000
	for i := 0; i < total; i++ {
		// Single-round simulation: large player HP/attack, high attack bonus,
		// monster low HP/AC. Count hit/crit events from "player".
		player := Combatant{
			IsPlayer: true,
			Stats:    CombatStats{MaxHP: 1000, Attack: 1, Defense: 0, Speed: 100, AC: 99, AttackBonus: 10},
			Mods:     CombatModifiers{DamageReduct: 1.0},
		}
		enemy := Combatant{
			Stats: CombatStats{MaxHP: 100000, Attack: 0, Defense: 0, Speed: 1, AC: 11, AttackBonus: 0},
			Mods:  CombatModifiers{DamageReduct: 1.0},
		}
		// Run only one phase / one round to isolate first-attack hit rate.
		phases := []CombatPhase{{Name: "Test", Rounds: 1, AttackWeight: 1.0, DefenseWeight: 1.0, SpeedWeight: 1.0}}
		result := SimulateCombat(player, enemy, phases)
		for _, ev := range result.Events {
			if ev.Actor == "player" && (ev.Action == "hit" || ev.Action == "crit") {
				hits++
				break
			}
			if ev.Actor == "player" && ev.Action == "miss" {
				break
			}
		}
	}
	rate := float64(hits) / float64(total)
	// Expected: roll >=1 always (1=fumble miss, 2-20 hit). Nat 20 also hits. So 19/20 = 95%.
	if rate < 0.92 || rate > 0.98 {
		t.Errorf("player-dominant hit rate = %.3f, want ~0.95", rate)
	}
}

// TestD20FumbleAndCrit: nat 1 always misses, nat 20 always crits. Sample
// many rolls and confirm at least one of each occurs over many rounds.
func TestD20FumbleAndCrit(t *testing.T) {
	sawFumble, sawCrit := false, false
	player := Combatant{
		IsPlayer: true,
		Stats:    CombatStats{MaxHP: 100000, Attack: 5, Defense: 0, Speed: 50, AC: 10, AttackBonus: 5},
		Mods:     CombatModifiers{DamageReduct: 1.0},
	}
	enemy := Combatant{
		Stats: CombatStats{MaxHP: 100000, Attack: 1, Defense: 0, Speed: 50, AC: 13, AttackBonus: 0},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	phases := []CombatPhase{{Name: "Long", Rounds: 200, AttackWeight: 1.0, DefenseWeight: 1.0, SpeedWeight: 1.0}}
	result := SimulateCombat(player, enemy, phases)
	for _, ev := range result.Events {
		if ev.Actor != "player" {
			continue
		}
		if ev.Action == "miss" && ev.Desc == "fumble" {
			sawFumble = true
		}
		if ev.Action == "crit" && ev.Roll == 20 {
			sawCrit = true
		}
	}
	if !sawFumble {
		t.Error("never observed a fumble (nat 1) over 200 rounds")
	}
	if !sawCrit {
		t.Error("never observed a nat-20 crit over 200 rounds")
	}
}
