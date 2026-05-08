package plugin

import "testing"

// Phase 10 SUB2d — Ranger subclasses (Hunter / Beast Master / Gloom Stalker).

// ── Hunter ──────────────────────────────────────────────────────────────

func TestApplySubclassPassives_HunterL5DamageBonus(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassHunter, Level: 5}
	stats := &CombatStats{AC: 14}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.DamageBonus < 0.099 || mods.DamageBonus > 0.101 {
		t.Errorf("Hunter L5 DamageBonus = %v, want ~0.10", mods.DamageBonus)
	}
	if stats.AC != 14 {
		t.Errorf("Hunter L5 AC = %d, want 14 (no Defensive Tactics yet)", stats.AC)
	}
}

func TestApplySubclassPassives_HunterL7AC(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassHunter, Level: 7}
	stats := &CombatStats{AC: 14}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AC != 15 {
		t.Errorf("Hunter L7 AC = %d, want 15 (+1 Defensive Tactics)", stats.AC)
	}
}

func TestApplySubclassPassives_HunterPreL5(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassHunter, Level: 4}
	stats := &CombatStats{AC: 14}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.DamageBonus != 0 || stats.AC != 14 {
		t.Errorf("pre-L5 Hunter leaked: dmg=%v ac=%d", mods.DamageBonus, stats.AC)
	}
}

// ── Beast Master ────────────────────────────────────────────────────────

func TestApplySubclassPassives_BeastMasterL5SetsCompanion(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 5}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc < 0.199 || mods.PetAttackProc > 0.201 {
		t.Errorf("BM L5 PetAttackProc = %v, want ~0.20", mods.PetAttackProc)
	}
	// L5 → 2 + 5/2 = 4.
	if mods.PetAttackDmg != 4 {
		t.Errorf("BM L5 PetAttackDmg = %d, want 4", mods.PetAttackDmg)
	}
}

func TestApplySubclassPassives_BeastMasterL7BestialFury(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 7}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc < 0.349 || mods.PetAttackProc > 0.351 {
		t.Errorf("BM L7 PetAttackProc = %v, want ~0.35", mods.PetAttackProc)
	}
	// L7 → 2 + 7/2 + 3 = 2 + 3 + 3 = 8.
	if mods.PetAttackDmg != 8 {
		t.Errorf("BM L7 PetAttackDmg = %d, want 8", mods.PetAttackDmg)
	}
}

// If the player already has a stronger pet (e.g. via the legacy adventure
// pet path in combat_stats.go), the subclass floor must not downgrade it.
func TestApplySubclassPassives_BeastMasterDoesntDowngradeStrongerPet(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 5}
	mods := &CombatModifiers{
		DamageReduct:  1.0,
		PetAttackProc: 0.50,
		PetAttackDmg:  10,
	}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc != 0.50 {
		t.Errorf("PetAttackProc = %v, want 0.50 preserved", mods.PetAttackProc)
	}
	if mods.PetAttackDmg != 10 {
		t.Errorf("PetAttackDmg = %d, want 10 preserved", mods.PetAttackDmg)
	}
}

func TestApplySubclassPassives_BeastMasterPreL5(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 4}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc != 0 || mods.PetAttackDmg != 0 {
		t.Errorf("pre-L5 BM leaked pet flags: proc=%v dmg=%d", mods.PetAttackProc, mods.PetAttackDmg)
	}
}

// ── Gloom Stalker ───────────────────────────────────────────────────────

func TestApplySubclassPassives_GloomStalkerL5DreadAmbusher(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 5,
		WIS: 16, // mod +3
	}
	stats := &CombatStats{Speed: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.Speed != 13 {
		t.Errorf("GS L5 Speed = %d, want 13 (10 + WIS +3)", stats.Speed)
	}
	if mods.AssassinateBonusDmg != 8 {
		t.Errorf("GS L5 AssassinateBonusDmg = %d, want 8 (5 + WIS +3)", mods.AssassinateBonusDmg)
	}
	if mods.AssassinateAdvantage {
		t.Error("GS L5 should not yet have AssassinateAdvantage (Stalker's Flurry gates at L7)")
	}
}

func TestApplySubclassPassives_GloomStalkerL7AddsAdvantage(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 7,
		WIS: 14, // mod +2
	}
	stats := &CombatStats{Speed: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if !mods.AssassinateAdvantage {
		t.Error("GS L7 should set AssassinateAdvantage")
	}
	if mods.AssassinateBonusDmg != 7 {
		t.Errorf("GS L7 AssassinateBonusDmg = %d, want 7 (5 + WIS +2)", mods.AssassinateBonusDmg)
	}
}

func TestApplySubclassPassives_GloomStalkerPreL5(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 4, WIS: 16,
	}
	stats := &CombatStats{Speed: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.Speed != 10 || mods.AssassinateBonusDmg != 0 || mods.AssassinateAdvantage {
		t.Errorf("pre-L5 GS leaked: speed=%d bonus=%d adv=%v",
			stats.Speed, mods.AssassinateBonusDmg, mods.AssassinateAdvantage)
	}
}

// ── Skill: Umbral Sight stealth advantage ───────────────────────────────

func TestSubclassSkillAdvantage_GloomStalkerStealth(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 5}
	stealth, _ := skillInfo(SkillStealth)
	soh, _ := skillInfo(SkillSleightOfHand)
	if !subclassSkillAdvantage(c, stealth) {
		t.Error("L5 Gloom Stalker should roll Stealth with advantage (Umbral Sight)")
	}
	if subclassSkillAdvantage(c, soh) {
		t.Error("Umbral Sight should not grant advantage on non-Stealth skills")
	}
}

func TestSubclassSkillAdvantage_GloomStalkerPreL5No(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 4}
	stealth, _ := skillInfo(SkillStealth)
	if subclassSkillAdvantage(c, stealth) {
		t.Error("pre-L5 Gloom Stalker should not yet have stealth advantage")
	}
}

// ── Phase 10 SUB3d — Hunter L10/L15 ─────────────────────────────────────

func TestApplySubclassPassives_HunterL10Multiattack(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassHunter, Level: 10}
	stats := &CombatStats{AC: 14}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	// L5 +0.10 + L10 +0.15 = 0.25
	if mods.DamageBonus < 0.249 || mods.DamageBonus > 0.251 {
		t.Errorf("Hunter L10 DamageBonus = %v, want ~0.25", mods.DamageBonus)
	}
	if mods.DamageReduct != 1.0 {
		t.Errorf("Hunter L10 DamageReduct = %v, want 1.0 (no Superior Defense yet)", mods.DamageReduct)
	}
}

func TestApplySubclassPassives_HunterL15SuperiorDefense(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassHunter, Level: 15}
	stats := &CombatStats{AC: 14}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.DamageReduct < 0.849 || mods.DamageReduct > 0.851 {
		t.Errorf("Hunter L15 DamageReduct = %v, want ~0.85", mods.DamageReduct)
	}
	if mods.DamageBonus < 0.249 || mods.DamageBonus > 0.251 {
		t.Errorf("Hunter L15 DamageBonus = %v, want ~0.25 (L5+L10 still on)", mods.DamageBonus)
	}
}

// ── Phase 10 SUB3d — Beast Master L10/L15 ───────────────────────────────

func TestApplySubclassPassives_BeastMasterL10ShareSpells(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc < 0.449 || mods.PetAttackProc > 0.451 {
		t.Errorf("BM L10 PetAttackProc = %v, want ~0.45", mods.PetAttackProc)
	}
	// L10 → 6 + 10/2 = 11
	if mods.PetAttackDmg != 11 {
		t.Errorf("BM L10 PetAttackDmg = %d, want 11", mods.PetAttackDmg)
	}
	if mods.DamageBonus < 0.049 || mods.DamageBonus > 0.051 {
		t.Errorf("BM L10 DamageBonus = %v, want ~0.05", mods.DamageBonus)
	}
}

func TestApplySubclassPassives_BeastMasterL15SuperiorBond(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 15}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc < 0.549 || mods.PetAttackProc > 0.551 {
		t.Errorf("BM L15 PetAttackProc = %v, want ~0.55", mods.PetAttackProc)
	}
	// L15 → 8 + 15/2 = 8 + 7 = 15
	if mods.PetAttackDmg != 15 {
		t.Errorf("BM L15 PetAttackDmg = %d, want 15", mods.PetAttackDmg)
	}
	if mods.DamageReduct < 0.919 || mods.DamageReduct > 0.921 {
		t.Errorf("BM L15 DamageReduct = %v, want ~0.92", mods.DamageReduct)
	}
}

// L10/L15 must not downgrade a stronger external pet floor.
func TestApplySubclassPassives_BeastMasterL15DoesntDowngradeStrongerPet(t *testing.T) {
	c := &DnDCharacter{Class: ClassRanger, Subclass: SubclassBeastMaster, Level: 15}
	mods := &CombatModifiers{
		DamageReduct:  1.0,
		PetAttackProc: 0.80,
		PetAttackDmg:  25,
	}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.PetAttackProc != 0.80 {
		t.Errorf("PetAttackProc = %v, want 0.80 preserved", mods.PetAttackProc)
	}
	if mods.PetAttackDmg != 25 {
		t.Errorf("PetAttackDmg = %d, want 25 preserved", mods.PetAttackDmg)
	}
}

// ── Phase 10 SUB3d — Gloom Stalker L10/L15 ──────────────────────────────

func TestApplySubclassPassives_GloomStalkerL10StalkersFlurry(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 10,
		WIS: 14, // mod +2
	}
	stats := &CombatStats{Speed: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.DamageBonus < 0.119 || mods.DamageBonus > 0.121 {
		t.Errorf("GS L10 DamageBonus = %v, want ~0.12", mods.DamageBonus)
	}
	if !mods.AssassinateAdvantage {
		t.Error("GS L10 should still have AssassinateAdvantage from L7")
	}
	if mods.DamageReduct != 1.0 {
		t.Errorf("GS L10 DamageReduct = %v, want 1.0 (no Shadowy Dodge yet)", mods.DamageReduct)
	}
}

func TestApplySubclassPassives_GloomStalkerL15ShadowyDodge(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRanger, Subclass: SubclassGloomStalker, Level: 15,
		WIS: 14, // mod +2
	}
	stats := &CombatStats{Speed: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.DamageReduct < 0.899 || mods.DamageReduct > 0.901 {
		t.Errorf("GS L15 DamageReduct = %v, want ~0.90", mods.DamageReduct)
	}
	if mods.DamageBonus < 0.119 || mods.DamageBonus > 0.121 {
		t.Errorf("GS L15 DamageBonus = %v, want ~0.12 (L10 still on)", mods.DamageBonus)
	}
}

// ── End-to-end: Hunter colossus-slayer damage lift ──────────────────────

func TestSimulateCombat_HunterDamageBonusImprovesWinRate(t *testing.T) {
	const trials = 1500
	build := func(bonus float64) Combatant {
		return Combatant{
			Name: "Ranger", IsPlayer: true,
			Stats: CombatStats{
				MaxHP: 100, Attack: 15, Defense: 8, Speed: 10, AttackBonus: 5, AC: 16,
				CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
			},
			Mods: CombatModifiers{DamageReduct: 1.0, DamageBonus: bonus},
		}
	}
	enemy := func() Combatant {
		return Combatant{
			Name: "Brute",
			Stats: CombatStats{
				MaxHP: 110, Attack: 14, Defense: 10, Speed: 8, AC: 14, AttackBonus: 3,
				CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
			},
			Mods: CombatModifiers{DamageReduct: 1.0},
		}
	}
	plain, hunter := 0, 0
	for i := 0; i < trials; i++ {
		if SimulateCombat(build(0), enemy(), defaultCombatPhases).PlayerWon {
			plain++
		}
		if SimulateCombat(build(0.10), enemy(), defaultCombatPhases).PlayerWon {
			hunter++
		}
	}
	if hunter <= plain {
		t.Errorf("Hunter +10%% damage didn't help: plain=%d hunter=%d", plain, hunter)
	}
}
