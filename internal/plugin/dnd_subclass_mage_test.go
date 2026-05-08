package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 10 SUB2b — Evocation, Abjuration, Necromancy.

// ── Evocation ───────────────────────────────────────────────────────────

func TestEvocation_EmpoweredEvocationAddsINTMod(t *testing.T) {
	spell, _ := lookupSpell("fire_bolt") // school=evocation
	c := &DnDCharacter{
		Class: ClassMage, Subclass: SubclassEvocation, Level: 7, INT: 18, // +4 mod
	}
	mods := &CombatModifiers{SpellPreDamage: 5}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.SpellPreDamage != 9 {
		t.Errorf("Empowered Evocation L7 INT 18: SpellPreDamage = %d, want 9 (5+4)", mods.SpellPreDamage)
	}
}

func TestEvocation_EmpoweredEvocationMinBonusOne(t *testing.T) {
	spell, _ := lookupSpell("fire_bolt")
	c := &DnDCharacter{
		Class: ClassMage, Subclass: SubclassEvocation, Level: 7, INT: 8, // -1 mod
	}
	mods := &CombatModifiers{SpellPreDamage: 5}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.SpellPreDamage != 6 {
		t.Errorf("Empowered Evocation INT 8 (negative mod): SpellPreDamage = %d, want 6 (min +1)", mods.SpellPreDamage)
	}
}

func TestEvocation_EmpoweredEvocationPreL7Skipped(t *testing.T) {
	spell, _ := lookupSpell("fire_bolt")
	c := &DnDCharacter{
		Class: ClassMage, Subclass: SubclassEvocation, Level: 6, INT: 18,
	}
	mods := &CombatModifiers{SpellPreDamage: 5}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.SpellPreDamage != 5 {
		t.Errorf("Empowered Evocation should not fire pre-L7: SpellPreDamage = %d, want 5", mods.SpellPreDamage)
	}
}

func TestEvocation_EmpoweredEvocationOnlyForEvocationSpells(t *testing.T) {
	spell, _ := lookupSpell("chill_touch") // school=necromancy
	c := &DnDCharacter{
		Class: ClassMage, Subclass: SubclassEvocation, Level: 7, INT: 18,
	}
	mods := &CombatModifiers{SpellPreDamage: 5}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.SpellPreDamage != 5 {
		t.Errorf("Empowered Evocation should not fire on non-evocation school: SpellPreDamage = %d, want 5", mods.SpellPreDamage)
	}
}

// ── Abjuration ──────────────────────────────────────────────────────────

func TestAbjuration_ArcaneWardL5(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassAbjuration, Level: 5}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.ArcaneWardHP != 10 {
		t.Errorf("Abjuration L5 ArcaneWardHP = %d, want 10 (2× level)", mods.ArcaneWardHP)
	}
}

func TestAbjuration_ArcaneWardL7AddsProf(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassAbjuration, Level: 7}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	// 2*7 + prof(L7=3) = 17.
	if mods.ArcaneWardHP != 17 {
		t.Errorf("Abjuration L7 ArcaneWardHP = %d, want 17 (14 + prof 3)", mods.ArcaneWardHP)
	}
}

func TestAbjuration_PreL5NoWard(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassAbjuration, Level: 4}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.ArcaneWardHP != 0 {
		t.Errorf("Abjuration pre-L5 ArcaneWardHP = %d, want 0", mods.ArcaneWardHP)
	}
}

// End-to-end: a sturdy Arcane Ward should reduce damage taken when the
// engine resolves enemy attacks.
func TestSimulateCombat_ArcaneWardAbsorbsDamage(t *testing.T) {
	const trials = 600
	build := func(ward int) Combatant {
		return Combatant{
			Name: "Mage", IsPlayer: true,
			Stats: CombatStats{
				MaxHP: 60, Attack: 10, Defense: 8, Speed: 10, AttackBonus: 4, AC: 14,
				CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
			},
			Mods: CombatModifiers{DamageReduct: 1.0, ArcaneWardHP: ward},
		}
	}
	enemy := Combatant{
		Name: "Brute",
		Stats: CombatStats{
			MaxHP: 80, Attack: 12, Defense: 6, Speed: 8, AC: 13, AttackBonus: 4,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageReduct: 1.0},
	}
	plainEndHP, wardEndHP := 0, 0
	for i := 0; i < trials; i++ {
		plainEndHP += SimulateCombat(build(0), enemy, defaultCombatPhases).PlayerEndHP
		wardEndHP += SimulateCombat(build(40), enemy, defaultCombatPhases).PlayerEndHP
	}
	if wardEndHP <= plainEndHP {
		t.Errorf("Arcane Ward should improve survivability: plain=%d ward=%d (sum HP)", plainEndHP, wardEndHP)
	}
}

// ── Necromancy ──────────────────────────────────────────────────────────

func TestNecromancy_GrimHarvestStashOnDamage(t *testing.T) {
	spell, _ := lookupSpell("chill_touch") // necrotic, school necromancy
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5, INT: 16}
	mods := &CombatModifiers{SpellPreDamage: 8}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.GrimHarvestSlot != 1 {
		t.Errorf("cantrip Grim Harvest slot = %d, want 1 (cantrip floor)", mods.GrimHarvestSlot)
	}
	if !mods.GrimHarvestNecrotic {
		t.Error("chill_touch should mark GrimHarvestNecrotic")
	}
}

func TestNecromancy_GrimHarvestStashLeveledSpell(t *testing.T) {
	spell, _ := lookupSpell("magic_missile") // force damage; school evocation
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5, INT: 16}
	mods := &CombatModifiers{SpellPreDamage: 12}
	applyMageSubclassSpellHooks(c, spell, 3, mods)
	if mods.GrimHarvestSlot != 3 {
		t.Errorf("magic_missile L3 Grim Harvest slot = %d, want 3", mods.GrimHarvestSlot)
	}
	if mods.GrimHarvestNecrotic {
		t.Error("magic_missile is force, not necrotic — should not set Necrotic flag")
	}
}

func TestNecromancy_GrimHarvestPreL5NoStash(t *testing.T) {
	spell, _ := lookupSpell("chill_touch")
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 4, INT: 16}
	mods := &CombatModifiers{SpellPreDamage: 8}
	applyMageSubclassSpellHooks(c, spell, 0, mods)
	if mods.GrimHarvestSlot != 0 {
		t.Errorf("pre-L5 Necromancer should not stash; slot = %d, want 0", mods.GrimHarvestSlot)
	}
}

func TestGrimHarvestHeal_NecroticTriple(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassMage, Subclass: SubclassNecromancy, Level: 5,
	}
	result := CombatResult{
		PlayerWon: true,
		Events: []CombatEvent{
			{Action: "spell_cast", EnemyHP: 0},
		},
	}
	mods := CombatModifiers{GrimHarvestSlot: 2, GrimHarvestNecrotic: true}
	if got := grimHarvestHeal(c, result, mods); got != 6 {
		t.Errorf("necrotic Grim Harvest L2 = %d, want 6 (3× slot)", got)
	}
}

func TestGrimHarvestHeal_NonNecroticDouble(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5}
	result := CombatResult{
		PlayerWon: true,
		Events:    []CombatEvent{{Action: "spell_cast", EnemyHP: 0}},
	}
	mods := CombatModifiers{GrimHarvestSlot: 3}
	if got := grimHarvestHeal(c, result, mods); got != 6 {
		t.Errorf("non-necrotic Grim Harvest L3 = %d, want 6 (2× slot)", got)
	}
}

func TestGrimHarvestHeal_OnlyOnSpellKill(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5}
	// Spell hit but didn't kill — combat finished with regular attack.
	result := CombatResult{
		PlayerWon: true,
		Events:    []CombatEvent{{Action: "spell_cast", EnemyHP: 5}},
	}
	mods := CombatModifiers{GrimHarvestSlot: 2, GrimHarvestNecrotic: true}
	if got := grimHarvestHeal(c, result, mods); got != 0 {
		t.Errorf("Grim Harvest fires on non-spell-kill: got %d, want 0", got)
	}
}

func TestGrimHarvestHeal_LossNoHeal(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5}
	result := CombatResult{
		PlayerWon: false,
		Events:    []CombatEvent{{Action: "spell_cast", EnemyHP: 0}},
	}
	mods := CombatModifiers{GrimHarvestSlot: 2}
	if got := grimHarvestHeal(c, result, mods); got != 0 {
		t.Errorf("Grim Harvest fires on loss: got %d, want 0", got)
	}
}

func TestPersistDnDPostCombatSubclass_GrimHarvestApplies(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@grim_heal:example")
	if err := createAdvCharacter(uid, "grim"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassMage, Subclass: SubclassNecromancy,
		Level: 5, STR: 8, DEX: 12, CON: 12, INT: 16, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 10, ArmorClass: 12,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	result := CombatResult{
		PlayerWon: true,
		Events:    []CombatEvent{{Action: "spell_cast", EnemyHP: 0}},
	}
	mods := CombatModifiers{GrimHarvestSlot: 2, GrimHarvestNecrotic: true}
	if err := persistDnDPostCombatSubclass(c, false, result, mods); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != 16 {
		t.Errorf("HP after Grim Harvest = %d, want 16 (10 + 6)", got.HPCurrent)
	}
}
