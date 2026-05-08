package plugin

import (
	"strings"
	"testing"
)

// Phase 9 SP4 — AoE flag spells must still resolve correctly through
// applySpellDamageSave even though combat has only one enemy. Verify Fireball,
// Burning Hands, and Cone of Cold land legal damage in both save-fail and
// save-success branches.

func TestApplySpellDamageSave_FireballRange(t *testing.T) {
	spell, ok := lookupSpell("fireball")
	if !ok {
		t.Fatal("fireball missing from registry")
	}
	if !spell.AOE {
		t.Errorf("fireball should be AOE")
	}

	c := &DnDCharacter{Class: ClassMage, Level: 5, INT: 18}
	enemy := &CombatStats{AttackBonus: 0, AC: 12}

	// DC 99 → save virtually always fails → full damage. 8d6 = 8..48.
	for i := 0; i < 30; i++ {
		mods := &CombatModifiers{}
		applySpellDamageSave(spell, 99, c, mods, enemy, 3)
		if mods.SpellPreDamage < 8 || mods.SpellPreDamage > 48 {
			t.Errorf("fireball DC99 dmg=%d outside 8..48", mods.SpellPreDamage)
			break
		}
	}
	// DC 1 → save virtually always succeeds → half damage (floor 1).
	// 8d6/2 = 4..24.
	for i := 0; i < 30; i++ {
		mods := &CombatModifiers{}
		applySpellDamageSave(spell, 1, c, mods, enemy, 3)
		if mods.SpellPreDamage < 1 || mods.SpellPreDamage > 24 {
			t.Errorf("fireball DC1 (saved) dmg=%d outside 1..24", mods.SpellPreDamage)
			break
		}
		if !strings.Contains(mods.SpellPreDamageDesc, "saved") {
			t.Errorf("fireball DC1 desc=%q, want 'saved'", mods.SpellPreDamageDesc)
			break
		}
	}
}

func TestApplySpellDamageSave_BurningHands(t *testing.T) {
	spell, _ := lookupSpell("burning_hands")
	if !spell.AOE {
		t.Errorf("burning_hands should be AOE")
	}
	c := &DnDCharacter{Class: ClassMage, Level: 1, INT: 16}
	enemy := &CombatStats{AttackBonus: 0}
	// L1 cast, 3d6 → 3..18 on full damage.
	for i := 0; i < 30; i++ {
		mods := &CombatModifiers{}
		applySpellDamageSave(spell, 99, c, mods, enemy, 1)
		if mods.SpellPreDamage < 3 || mods.SpellPreDamage > 18 {
			t.Errorf("burning_hands dmg=%d outside 3..18", mods.SpellPreDamage)
			break
		}
	}
	// Upcast to L3: rollSpellDamageDice adds (3-1)=2 dice → 5d6 = 5..30.
	for i := 0; i < 30; i++ {
		mods := &CombatModifiers{}
		applySpellDamageSave(spell, 99, c, mods, enemy, 3)
		if mods.SpellPreDamage < 5 || mods.SpellPreDamage > 30 {
			t.Errorf("burning_hands upcast L3 dmg=%d outside 5..30", mods.SpellPreDamage)
			break
		}
	}
}

func TestApplySpellDamageSave_ConeOfCold(t *testing.T) {
	spell, _ := lookupSpell("cone_of_cold")
	if !spell.AOE {
		t.Errorf("cone_of_cold should be AOE")
	}
	c := &DnDCharacter{Class: ClassMage, Level: 9, INT: 18}
	enemy := &CombatStats{AttackBonus: 0}
	// 8d8 = 8..64.
	for i := 0; i < 30; i++ {
		mods := &CombatModifiers{}
		applySpellDamageSave(spell, 99, c, mods, enemy, 5)
		if mods.SpellPreDamage < 8 || mods.SpellPreDamage > 64 {
			t.Errorf("cone_of_cold dmg=%d outside 8..64", mods.SpellPreDamage)
			break
		}
	}
}
