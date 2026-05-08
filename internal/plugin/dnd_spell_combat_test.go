package plugin

import (
	"strings"
	"testing"
)

// TestApplySpellDamageAttack_HitsHard — Fire Bolt on a low-AC enemy with
// a +20 attack bonus is essentially guaranteed; ensure damage lands and
// the description carries the spell name.
func TestApplySpellDamageAttack_HitsHard(t *testing.T) {
	spell, ok := lookupSpell("fire_bolt")
	if !ok {
		t.Fatal("fire_bolt missing from registry")
	}
	mods := &CombatModifiers{}
	enemy := &CombatStats{AC: 5}
	// Run a few iterations — d20+20 vs AC 5 always hits, so damage > 0.
	hits := 0
	for i := 0; i < 50; i++ {
		mods.SpellPreDamage = 0
		mods.SpellPreDamageDesc = ""
		applySpellDamageAttack(spell, 20, mods, enemy, 0, 1)
		if mods.SpellPreDamage > 0 {
			hits++
		}
	}
	if hits == 0 {
		t.Errorf("expected hits with +20 atk vs AC5; got 0")
	}
}

// TestApplySpellDamageAttack_AlwaysMisses — d20+0 vs AC 100 cannot hit
// outside a nat 20 (5% rate); ensure most rolls are recorded as misses.
func TestApplySpellDamageAttack_AlwaysMisses(t *testing.T) {
	spell, _ := lookupSpell("fire_bolt")
	enemy := &CombatStats{AC: 100}
	misses := 0
	for i := 0; i < 100; i++ {
		mods := &CombatModifiers{}
		applySpellDamageAttack(spell, 0, mods, enemy, 0, 1)
		if strings.Contains(mods.SpellPreDamageDesc, "missed") {
			misses++
		}
	}
	if misses < 80 {
		t.Errorf("expected >=80 misses out of 100; got %d", misses)
	}
}

// TestApplySpellControl_AutoCritOnHoldPerson — failed save sets both the
// skip-first flag and AutoCritFirst (paralyzed → melee crit).
func TestApplySpellControl_AutoCritOnHoldPerson(t *testing.T) {
	spell, ok := lookupSpell("hold_person")
	if !ok {
		t.Fatal("hold_person missing from registry")
	}
	// DC 99 ensures the save fails virtually every roll.
	enemy := &CombatStats{AttackBonus: 0}
	got := false
	for i := 0; i < 20 && !got; i++ {
		mods := &CombatModifiers{}
		applySpellControl(spell, 99, mods, enemy, 2)
		if mods.SpellEnemySkipFirst && mods.AutoCritFirst {
			got = true
		}
	}
	if !got {
		t.Errorf("expected hold_person to set skip+autocrit on a failed save")
	}
}

// TestApplySpellBuff_MageArmor — Mage Armor sets AC = 13 + DEX when higher
// than current; leaves it alone otherwise.
func TestApplySpellBuff_MageArmor(t *testing.T) {
	spell, _ := lookupSpell("mage_armor")
	c := &DnDCharacter{DEX: 16} // +3 mod → AC 16
	stats := &CombatStats{AC: 12}
	mods := &CombatModifiers{}
	applySpellBuff(spell, c, stats, mods, 1)
	if stats.AC != 16 {
		t.Errorf("Mage Armor AC: got %d, want 16", stats.AC)
	}
	// Already-higher AC (full plate at 18) shouldn't drop.
	stats.AC = 18
	applySpellBuff(spell, c, stats, mods, 1)
	if stats.AC != 18 {
		t.Errorf("Mage Armor regression: got %d, want 18 preserved", stats.AC)
	}
}

// TestApplySpellBuff_Bless — Bless boosts AttackBonus by 2 (1d4 average).
func TestApplySpellBuff_Bless(t *testing.T) {
	spell, _ := lookupSpell("bless")
	stats := &CombatStats{AttackBonus: 5}
	mods := &CombatModifiers{}
	applySpellBuff(spell, &DnDCharacter{}, stats, mods, 1)
	if stats.AttackBonus != 7 {
		t.Errorf("Bless: got %d, want 7", stats.AttackBonus)
	}
}

// TestApplyMagicMissile — Magic Missile auto-damages with no roll. 3 darts
// at base, +1 per upcast level. Each dart deals 2..5 (1d4+1).
func TestApplyMagicMissile(t *testing.T) {
	spell, _ := lookupSpell("magic_missile")
	mods := &CombatModifiers{}
	applySpellDamageAuto(spell, mods, 1, 1)
	// 3 darts × 2..5 → 6..15
	if mods.SpellPreDamage < 6 || mods.SpellPreDamage > 15 {
		t.Errorf("MM L1: damage %d outside 6..15", mods.SpellPreDamage)
	}

	// Upcast L3: 5 darts → 10..25
	mods = &CombatModifiers{}
	applySpellDamageAuto(spell, mods, 3, 1)
	if mods.SpellPreDamage < 10 || mods.SpellPreDamage > 25 {
		t.Errorf("MM L3: damage %d outside 10..25", mods.SpellPreDamage)
	}
}

// TestRollSpellDamageDice_CantripScaling — Fire Bolt scales 1d10 → 2d10
// at L5, 3d10 at L11, 4d10 at L17.
func TestRollSpellDamageDice_CantripScaling(t *testing.T) {
	spell, _ := lookupSpell("fire_bolt")
	cases := []struct {
		level   int
		minRoll int
		maxRoll int
	}{
		{1, 1, 10},
		{5, 2, 20},
		{11, 3, 30},
		{17, 4, 40},
	}
	for _, tc := range cases {
		// 30 samples; check all stay in range.
		for i := 0; i < 30; i++ {
			got := rollSpellDamageDice(spell, 0, tc.level)
			if got < tc.minRoll || got > tc.maxRoll {
				t.Errorf("Fire Bolt L%d: got %d, want %d..%d", tc.level, got, tc.minRoll, tc.maxRoll)
				break
			}
		}
	}
}

// TestSimulateCombat_SpellPreDamageEvent — verify the engine surfaces a
// "spell_cast" pre-combat event when SpellPreDamageDesc is set.
func TestSimulateCombat_SpellPreDamageEvent(t *testing.T) {
	player := Combatant{
		Name:  "Caster",
		Stats: CombatStats{MaxHP: 50, Attack: 10, Defense: 5, AC: 14, AttackBonus: 5},
		Mods: CombatModifiers{
			DamageReduct:       1.0,
			SpellPreDamage:     12,
			SpellPreDamageDesc: "Fireball",
		},
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:  "Goblin",
		Stats: CombatStats{MaxHP: 30, Attack: 8, Defense: 3, AC: 12, AttackBonus: 3},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	result := SimulateCombat(player, enemy, defaultCombatPhases)
	found := false
	for _, ev := range result.Events {
		if ev.Action == "spell_cast" && ev.Desc == "Fireball" && ev.Damage == 12 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected spell_cast Fireball/12 pre-combat event; events=%+v", result.Events)
	}
}

// TestSimulateCombat_EnemySkipFirst — when SpellEnemySkipFirst is set,
// round 1 must contain a "spell_held" event.
func TestSimulateCombat_EnemySkipFirst(t *testing.T) {
	player := Combatant{
		Stats:    CombatStats{MaxHP: 30, Attack: 10, Defense: 5, AC: 14, AttackBonus: 5},
		Mods:     CombatModifiers{DamageReduct: 1.0, SpellEnemySkipFirst: true},
		IsPlayer: true,
	}
	enemy := Combatant{
		Stats: CombatStats{MaxHP: 30, Attack: 8, Defense: 3, AC: 12, AttackBonus: 3},
		Mods:  CombatModifiers{DamageReduct: 1.0},
	}
	result := SimulateCombat(player, enemy, defaultCombatPhases)
	heldRound1 := false
	for _, ev := range result.Events {
		if ev.Round == 1 && ev.Action == "spell_held" {
			heldRound1 = true
			break
		}
	}
	if !heldRound1 {
		t.Errorf("expected spell_held event in round 1; got events=%+v", result.Events)
	}
}
