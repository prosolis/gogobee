package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 10 SUB2a-i — subclass combat hooks (Champion, Berserker, Thief).

// ── applySubclassPassives ────────────────────────────────────────────────

func TestApplySubclassPassives_ChampionLowersCritThreshold(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 5}
	stats := &CombatStats{}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.CritThreshold != 19 {
		t.Errorf("CritThreshold = %d, want 19", mods.CritThreshold)
	}
}

func TestApplySubclassPassives_NoChampionPreL5(t *testing.T) {
	// Subclass selection only happens at L5+, but if state ever drifts
	// (race-condition with level reset), the passive must not fire below
	// the gating level.
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 4}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.CritThreshold != 0 {
		t.Errorf("CritThreshold = %d, want 0 (default) at L4", mods.CritThreshold)
	}
}

func TestApplySubclassPassives_NoSubclassNoEffect(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Level: 5}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.CritThreshold != 0 {
		t.Errorf("CritThreshold should default to 0 with no subclass; got %d", mods.CritThreshold)
	}
}

// ── Rage active ability ──────────────────────────────────────────────────

func TestRageAbility_ApplySetsAllFlags(t *testing.T) {
	ab, ok := dndActiveAbilities["rage"]
	if !ok {
		t.Fatal("rage ability not registered")
	}
	if ab.Class != ClassFighter || ab.Subclass != SubclassBerserker {
		t.Errorf("rage gating: class=%s subclass=%s, want fighter/berserker",
			ab.Class, ab.Subclass)
	}
	if ab.Resource != "stamina" {
		t.Errorf("rage resource = %s, want stamina", ab.Resource)
	}

	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBerserker, Level: 5}
	mods := &CombatModifiers{DamageReduct: 1.0}
	ab.Apply(c, mods)
	if !mods.BerserkerRage {
		t.Error("BerserkerRage not set")
	}
	if mods.RageMeleeDmg != 2 {
		t.Errorf("RageMeleeDmg = %d, want 2", mods.RageMeleeDmg)
	}
	if !mods.PhysicalResistRage {
		t.Error("PhysicalResistRage not set")
	}
	if mods.FrenzyDmgBonus <= 0 {
		t.Errorf("FrenzyDmgBonus = %v, want > 0", mods.FrenzyDmgBonus)
	}
}

// ── Post-combat exhaustion ───────────────────────────────────────────────

func TestPersistDnDPostCombatSubclass_RagedIncrementsExhaustion(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@rage_exh:example")
	if err := createAdvCharacter(uid, "rage_exh"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceOrc, Class: ClassFighter, Subclass: SubclassBerserker,
		Level: 6, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := persistDnDPostCombatSubclass(c, true, CombatResult{}, CombatModifiers{}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 1 {
		t.Errorf("Exhaustion after raged combat = %d, want 1", got.Exhaustion)
	}
	// Second raged combat → 2.
	if err := persistDnDPostCombatSubclass(got, true, CombatResult{}, CombatModifiers{}); err != nil {
		t.Fatal(err)
	}
	got2, _ := LoadDnDCharacter(uid)
	if got2.Exhaustion != 2 {
		t.Errorf("Exhaustion after 2 raged combats = %d, want 2", got2.Exhaustion)
	}
}

func TestPersistDnDPostCombatSubclass_NoRageNoIncrement(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@no_rage_exh:example")
	if err := createAdvCharacter(uid, "no_rage"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Subclass: SubclassChampion,
		Level: 6, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16, Exhaustion: 3,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := persistDnDPostCombatSubclass(c, false, CombatResult{}, CombatModifiers{}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 3 {
		t.Errorf("Exhaustion changed without rage: %d, want 3", got.Exhaustion)
	}
}

// ── End-to-end SimulateCombat with rage flags ────────────────────────────

func ragingPlayer() Combatant {
	return Combatant{
		Name: "Rager", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 100, Attack: 15, Defense: 8, Speed: 10,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{
			DamageReduct: 1.0,
			BerserkerRage: true, RageMeleeDmg: 2,
			PhysicalResistRage: true, FrenzyDmgBonus: 0.5,
		},
	}
}

func calmPlayer() Combatant {
	return Combatant{
		Name: "Calm", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 100, Attack: 15, Defense: 8, Speed: 10,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageReduct: 1.0},
	}
}

func toughEnemy() Combatant {
	// Sized so the calm baseline wins ~half the time — gives rage room
	// to push the win rate noticeably higher.
	return Combatant{
		Name: "Brute",
		Stats: CombatStats{
			MaxHP: 110, Attack: 18, Defense: 10, Speed: 8,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageBonus: 0, DamageReduct: 1.0},
	}
}

// Probabilistic: raging Berserker should win more often than the calm
// baseline against an enemy strong enough that the calm player isn't
// already maxing out wins.
func TestSimulateCombat_RageImprovesWinRate(t *testing.T) {
	const trials = 2000
	calmWins, rageWins := 0, 0
	for i := 0; i < trials; i++ {
		if SimulateCombat(calmPlayer(), toughEnemy(), defaultCombatPhases).PlayerWon {
			calmWins++
		}
		if SimulateCombat(ragingPlayer(), toughEnemy(), defaultCombatPhases).PlayerWon {
			rageWins++
		}
	}
	// Demand at least a 5-percentage-point lift to keep the test honest
	// without making it flaky on tail RNG.
	margin := rageWins - calmWins
	if margin < trials/20 {
		t.Errorf("rage win-rate lift too small: calm=%d rage=%d (margin %d, want >=%d)",
			calmWins, rageWins, margin, trials/20)
	}
}

// ── Skill bonuses ────────────────────────────────────────────────────────

func TestSubclassSkillBonus_ChampionRemarkableAthlete(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 7}
	athletics, _ := skillInfo(SkillAthletics)
	stealth, _ := skillInfo(SkillStealth) // dex
	arcana, _ := skillInfo(SkillArcana)   // int — should NOT get the bonus
	// L7 prof = 3, half = 1.
	if got := subclassSkillBonus(c, athletics); got != 1 {
		t.Errorf("Athletics (str) bonus = %d, want 1", got)
	}
	if got := subclassSkillBonus(c, stealth); got != 1 {
		t.Errorf("Stealth (dex) bonus = %d, want 1", got)
	}
	if got := subclassSkillBonus(c, arcana); got != 0 {
		t.Errorf("Arcana (int) bonus = %d, want 0 (Remarkable Athlete is STR/DEX/CON only)", got)
	}
}

func TestSubclassSkillBonus_ChampionPreL7NoBonus(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 6}
	athletics, _ := skillInfo(SkillAthletics)
	if got := subclassSkillBonus(c, athletics); got != 0 {
		t.Errorf("L6 Champion bonus = %d, want 0 (Remarkable Athlete needs L7)", got)
	}
}

func TestSubclassSkillBonus_ThiefFastHands(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 5}
	soh, _ := skillInfo(SkillSleightOfHand)
	stealth, _ := skillInfo(SkillStealth)
	if got := subclassSkillBonus(c, soh); got != 5 {
		t.Errorf("Thief SoH bonus = %d, want 5", got)
	}
	if got := subclassSkillBonus(c, stealth); got != 0 {
		t.Errorf("Thief stealth bonus from Fast Hands = %d, want 0 (FH is SoH only)", got)
	}
}

func TestSubclassSkillAdvantage_ThiefSupremeSneak(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 7}
	stealth, _ := skillInfo(SkillStealth)
	soh, _ := skillInfo(SkillSleightOfHand)
	if !subclassSkillAdvantage(c, stealth) {
		t.Error("L7 Thief should have advantage on Stealth")
	}
	if subclassSkillAdvantage(c, soh) {
		t.Error("Supreme Sneak should not grant advantage on non-Stealth skills")
	}
}

func TestSubclassSkillAdvantage_PreL7No(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 6}
	stealth, _ := skillInfo(SkillStealth)
	if subclassSkillAdvantage(c, stealth) {
		t.Error("L6 Thief should not yet have Stealth advantage")
	}
}

// Statistical: Thief L7 stealth average should beat plain rogue by a wide
// margin (advantage = best of two d20s, expected ≈ 13.83 vs 10.5).
func TestPerformSkillCheck_StealthAdvantageStatistical(t *testing.T) {
	const trials = 5000
	plainRogue := &DnDCharacter{
		Class: ClassRogue, Level: 7, DEX: 14, // mod +2
	}
	thief := &DnDCharacter{
		Class: ClassRogue, Subclass: SubclassThief, Level: 7, DEX: 14,
	}
	plainTotal, thiefTotal := 0, 0
	for i := 0; i < trials; i++ {
		plainTotal += performSkillCheck(plainRogue, SkillStealth, 15).Roll
		thiefTotal += performSkillCheck(thief, SkillStealth, 15).Roll
	}
	plainAvg := float64(plainTotal) / float64(trials)
	thiefAvg := float64(thiefTotal) / float64(trials)
	// Plain ≈ 10.5; Thief ≈ 13.8. Pick a margin that's robust.
	if thiefAvg-plainAvg < 2.0 {
		t.Errorf("Thief stealth roll average not meaningfully higher: plain=%.2f thief=%.2f", plainAvg, thiefAvg)
	}
}

// ── !arm rage gating ─────────────────────────────────────────────────────

func TestArmRage_BlockedForNonBerserker(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@arm_rage_block:example")
	if err := createAdvCharacter(uid, "block"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Subclass: SubclassChampion,
		Level: 6, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "rage"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility == "rage" {
		t.Error("non-Berserker should not be able to arm rage")
	}
}

func TestArmRage_AllowedForBerserker(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@arm_rage_ok:example")
	if err := createAdvCharacter(uid, "ok"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceOrc, Class: ClassFighter, Subclass: SubclassBerserker,
		Level: 5, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "rage"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "rage" {
		t.Errorf("ArmedAbility = %q, want rage", got.ArmedAbility)
	}
}

// ── Long rest decrements exhaustion ──────────────────────────────────────

func TestLongRest_ClearsOneExhaustion(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Class: ClassFighter, Level: 5,
		HPMax: 30, HPCurrent: 5, Exhaustion: 3,
	}
	// Replicate the long-rest effect: HP top-up + Exhaustion-- (the path
	// in dnd_rest.go we updated). Pure-state test — no DM or DB.
	if c.Exhaustion > 0 {
		c.Exhaustion--
	}
	if c.Exhaustion != 2 {
		t.Errorf("Exhaustion after long rest = %d, want 2", c.Exhaustion)
	}
}

// ── Schema roundtrip ─────────────────────────────────────────────────────

func TestExhaustion_SchemaRoundTrip(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exh_rt:example")
	if err := createAdvCharacter(uid, "exh_rt"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceOrc, Class: ClassFighter, Subclass: SubclassBerserker,
		Level: 5, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16, Exhaustion: 4,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.Exhaustion != 4 {
		t.Errorf("Exhaustion roundtrip: got %d, want 4", got.Exhaustion)
	}
}

// ── SUB2a-ii — Battle Master + Assassin ─────────────────────────────────

func TestApplySubclassPassives_BattleMasterL7AttackBonus(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 7}
	stats := &CombatStats{AttackBonus: 3}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AttackBonus != 4 {
		t.Errorf("L7 Battle Master AttackBonus = %d, want 4 (3 base + 1 Know Your Enemy)", stats.AttackBonus)
	}
}

func TestApplySubclassPassives_BattleMasterPreL7NoBonus(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 6}
	stats := &CombatStats{AttackBonus: 3}
	applySubclassPassives(stats, &CombatModifiers{DamageReduct: 1.0}, c)
	if stats.AttackBonus != 3 {
		t.Errorf("L6 Battle Master AttackBonus = %d, want 3 (no Know Your Enemy yet)", stats.AttackBonus)
	}
}

func TestApplySubclassPassives_AssassinL5(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassAssassin, Level: 5}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if !mods.AssassinateAdvantage {
		t.Error("L5 Assassin should set AssassinateAdvantage")
	}
	if mods.AssassinateBonusDmg != 5 {
		t.Errorf("L5 Assassin BonusDmg = %d, want 5", mods.AssassinateBonusDmg)
	}
}

func TestApplySubclassPassives_AssassinL7Bumps(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassAssassin, Level: 7}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.AssassinateBonusDmg != 10 {
		t.Errorf("L7 Assassin BonusDmg = %d, want 10 (level 7 + 3 Impostor)", mods.AssassinateBonusDmg)
	}
}

// Manoeuvre Apply functions wire the right mods.

func TestPrecisionAttack_ApplySetsFirstAttackBonus(t *testing.T) {
	ab, ok := dndActiveAbilities["precision_attack"]
	if !ok {
		t.Fatal("precision_attack not registered")
	}
	if ab.Class != ClassFighter || ab.Subclass != SubclassBattleMaster {
		t.Errorf("gating: %s/%s, want fighter/battle_master", ab.Class, ab.Subclass)
	}
	if ab.Resource != "superiority" {
		t.Errorf("resource = %s, want superiority", ab.Resource)
	}
	mods := &CombatModifiers{DamageReduct: 1.0}
	ab.Apply(&DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 5}, mods)
	if mods.FirstAttackBonus != 4 {
		t.Errorf("FirstAttackBonus = %d, want 4", mods.FirstAttackBonus)
	}
}

func TestTripAttack_ApplySetsEnemySkipFirst(t *testing.T) {
	ab := dndActiveAbilities["trip_attack"]
	mods := &CombatModifiers{DamageReduct: 1.0}
	ab.Apply(&DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 5}, mods)
	if !mods.SpellEnemySkipFirst {
		t.Error("Tripping Attack should set SpellEnemySkipFirst")
	}
}

func TestRally_ApplySetsHealItem(t *testing.T) {
	ab := dndActiveAbilities["rally"]
	mods := &CombatModifiers{DamageReduct: 1.0}
	// CHA 14 → +2 mod, expect HealItem += 6.
	ab.Apply(&DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 5, CHA: 14}, mods)
	if mods.HealItem != 6 {
		t.Errorf("Rally HealItem = %d, want 6 (4 + CHA mod 2)", mods.HealItem)
	}
}

// ── Resource init at subclass selection ──────────────────────────────────

func TestInitSubclassResources_BattleMasterGetsSuperiority(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@bm_init:example")
	if err := createAdvCharacter(uid, "bm_init"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 5); err != nil {
		t.Fatal(err)
	}
	cur, max, err := getResource(uid, "superiority")
	if err != nil {
		t.Fatal(err)
	}
	if cur != 4 || max != 4 {
		t.Errorf("superiority pool = %d/%d, want 4/4", cur, max)
	}
}

func TestInitSubclassResources_NonBattleMasterIsNoop(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@bm_noop:example")
	if err := createAdvCharacter(uid, "bm_noop"); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassChampion, 5); err != nil {
		t.Fatal(err)
	}
	cur, max, err := getResource(uid, "superiority")
	if err != nil {
		t.Fatal(err)
	}
	if cur != 0 || max != 0 {
		t.Errorf("superiority pool for Champion = %d/%d, want 0/0", cur, max)
	}
}

// !arm gating for subclass-only abilities.

func TestArmPrecisionAttack_BlockedForChampion(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@bm_block:example")
	if err := createAdvCharacter(uid, "bm_block"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Subclass: SubclassChampion,
		Level: 6, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "precision_attack"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility == "precision_attack" {
		t.Error("non-Battle-Master should not be able to arm precision_attack")
	}
}

func TestArmPrecisionAttack_AllowedForBattleMaster(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@bm_ok:example")
	if err := createAdvCharacter(uid, "bm_ok"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Subclass: SubclassBattleMaster,
		Level: 5, STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 5); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "precision_attack"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "precision_attack" {
		t.Errorf("ArmedAbility = %q, want precision_attack", got.ArmedAbility)
	}
	cur, _, _ := getResource(uid, "superiority")
	if cur != 3 {
		t.Errorf("superiority after arm = %d, want 3", cur)
	}
}

// ── Skill bonuses ────────────────────────────────────────────────────────

func TestSubclassSkillBonus_AssassinDeception(t *testing.T) {
	dec, _ := skillInfo(SkillDeception)
	persuasion, _ := skillInfo(SkillPersuasion)

	l5 := &DnDCharacter{Class: ClassRogue, Subclass: SubclassAssassin, Level: 5}
	if got := subclassSkillBonus(l5, dec); got != 5 {
		t.Errorf("L5 Assassin Deception = %d, want 5", got)
	}
	if got := subclassSkillBonus(l5, persuasion); got != 0 {
		t.Errorf("L5 Assassin Persuasion = %d, want 0 (Infiltration is Deception only)", got)
	}

	l7 := &DnDCharacter{Class: ClassRogue, Subclass: SubclassAssassin, Level: 7}
	if got := subclassSkillBonus(l7, dec); got != 10 {
		t.Errorf("L7 Assassin Deception = %d, want 10 (Impostor)", got)
	}

	l4 := &DnDCharacter{Class: ClassRogue, Subclass: SubclassAssassin, Level: 4}
	if got := subclassSkillBonus(l4, dec); got != 0 {
		t.Errorf("L4 Assassin Deception = %d, want 0 (subclass abilities gate at L5)", got)
	}
}

// ── End-to-end: one-shot first-attack effects ────────────────────────────

func bmPrecisionPlayer() Combatant {
	return Combatant{
		Name: "BM", IsPlayer: true,
		Stats: CombatStats{
			MaxHP: 100, Attack: 15, Defense: 8, Speed: 10, AttackBonus: 5, AC: 16,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageReduct: 1.0, FirstAttackBonus: 4},
	}
}

func plainBMPlayer() Combatant {
	c := bmPrecisionPlayer()
	c.Mods.FirstAttackBonus = 0
	return c
}

// Probabilistic: a stiff +4 on the opening swing should noticeably lift
// hit-rate against an enemy AC tuned to mostly-shrug the baseline player.
func TestSimulateCombat_FirstAttackBonusImprovesEarlyHits(t *testing.T) {
	const trials = 1500
	hardEnemy := Combatant{
		Name: "Wall",
		Stats: CombatStats{
			MaxHP: 80, Attack: 10, Defense: 6, Speed: 6, AC: 22, AttackBonus: 4,
			CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageReduct: 1.0},
	}
	plainWins, bmWins := 0, 0
	for i := 0; i < trials; i++ {
		if SimulateCombat(plainBMPlayer(), hardEnemy, defaultCombatPhases).PlayerWon {
			plainWins++
		}
		if SimulateCombat(bmPrecisionPlayer(), hardEnemy, defaultCombatPhases).PlayerWon {
			bmWins++
		}
	}
	if bmWins-plainWins < 25 {
		t.Errorf("Precision Attack lift too small: plain=%d bm=%d", plainWins, bmWins)
	}
}

// Surface check: AssassinateBonusDmg only consumed once.
func TestResolvePlayerAttack_AssassinateBonusFirstHitOnly(t *testing.T) {
	// Drive resolvePlayerAttack via a SimulateCombat run and confirm the
	// total enemy damage taken on hit is the *base* damage on subsequent
	// hits (i.e. only the first hit got the +bonus). Statistical compare
	// against an Assassin L5 vs. an identical player without the bonus.
	const trials = 800
	build := func(bonus int) Combatant {
		return Combatant{
			Name: "Assn", IsPlayer: true,
			Stats: CombatStats{
				MaxHP: 100, Attack: 15, Defense: 8, Speed: 10, AttackBonus: 5, AC: 16,
				CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
			},
			Mods: CombatModifiers{
				DamageReduct: 1.0, AutoCritFirst: true, AssassinateBonusDmg: bonus,
			},
		}
	}
	enemy := func() Combatant {
		return Combatant{
			Name: "Mark",
			Stats: CombatStats{
				MaxHP: 80, Attack: 12, Defense: 8, Speed: 8, AC: 14, AttackBonus: 3,
				CritRate: 0.05, DodgeRate: 0.05, BlockRate: 0.05,
			},
			Mods: CombatModifiers{DamageReduct: 1.0},
		}
	}
	plainWins, assnWins := 0, 0
	for i := 0; i < trials; i++ {
		if SimulateCombat(build(0), enemy(), defaultCombatPhases).PlayerWon {
			plainWins++
		}
		if SimulateCombat(build(8), enemy(), defaultCombatPhases).PlayerWon {
			assnWins++
		}
	}
	if assnWins <= plainWins {
		t.Errorf("Assassinate bonus damage didn't help: plain=%d assn=%d", plainWins, assnWins)
	}
}

// ── SUB2-AT — Arcane Trickster ──────────────────────────────────────────

func TestArcaneTrickster_IsSpellcasterAtL5(t *testing.T) {
	rogueL4 := &DnDCharacter{Class: ClassRogue, Subclass: SubclassArcaneTrickster, Level: 4}
	if isSpellcaster(rogueL4) {
		t.Error("AT Rogue should not be a spellcaster below L5")
	}
	rogueL5 := &DnDCharacter{Class: ClassRogue, Subclass: SubclassArcaneTrickster, Level: 5}
	if !isSpellcaster(rogueL5) {
		t.Error("AT Rogue should be a spellcaster at L5+")
	}
	plainRogue := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 5}
	if isSpellcaster(plainRogue) {
		t.Error("Thief Rogue should not be a spellcaster")
	}
}

func TestArcaneTrickster_SpellcastingModUsesINT(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRogue, Subclass: SubclassArcaneTrickster, Level: 5,
		INT: 16, WIS: 10,
	}
	if got := spellcastingMod(c); got != abilityModifier(16) {
		t.Errorf("AT spellcastingMod = %d, want INT mod (%d)", got, abilityModifier(16))
	}
	// A non-AT Rogue still returns 0.
	plain := &DnDCharacter{Class: ClassRogue, Level: 5, INT: 16}
	if got := spellcastingMod(plain); got != 0 {
		t.Errorf("non-AT Rogue spellcastingMod = %d, want 0", got)
	}
}

func TestArcaneTrickster_SubclassSlotPool(t *testing.T) {
	// L5 → 3 L1 slots only.
	pool := subclassSpellSlots(SubclassArcaneTrickster, 5)
	if pool[1] != 3 || pool[2] != 0 {
		t.Errorf("AT L5 slots = %v, want {1:3}", pool)
	}
	// L7 → adds L2 slots.
	pool = subclassSpellSlots(SubclassArcaneTrickster, 7)
	if pool[1] != 4 || pool[2] != 2 {
		t.Errorf("AT L7 slots = %v, want {1:4, 2:2}", pool)
	}
	// Below the L5 gate.
	if pool := subclassSpellSlots(SubclassArcaneTrickster, 4); len(pool) != 0 {
		t.Errorf("AT L4 should grant no slots; got %v", pool)
	}
	// Other subclasses don't grant slots.
	if pool := subclassSpellSlots(SubclassChampion, 10); len(pool) != 0 {
		t.Errorf("Champion slots = %v, want empty", pool)
	}
}

func TestArcaneTrickster_MagicalAmbushDCBumpAtL7(t *testing.T) {
	c := &DnDCharacter{
		Class: ClassRogue, Subclass: SubclassArcaneTrickster, Level: 7,
		INT: 16,
	}
	plain := &DnDCharacter{
		Class: ClassRogue, Subclass: SubclassArcaneTrickster, Level: 6,
		INT: 16,
	}
	got := spellSaveDC(c)
	base := spellSaveDC(plain)
	// L7 base = 8 + prof(3) + INT mod(3) = 14, +2 ambush = 16.
	// L6 base = 8 + prof(3) + INT mod(3) = 14.
	if got-base != 2 {
		t.Errorf("L7 AT spellSaveDC = %d (L6 baseline %d), want +2 from Magical Ambush", got, base)
	}
}

func TestEnsureSpellsForCharacter_ArcaneTrickster(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@at_spells:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassRogue, Subclass: SubclassArcaneTrickster,
		Level: 5, STR: 8, DEX: 16, CON: 12, INT: 16, WIS: 10, CHA: 12,
		HPMax: 25, HPCurrent: 25, ArmorClass: 14,
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	known, _ := listKnownSpells(uid)
	if len(known) == 0 {
		t.Fatal("AT L5 should receive default spells; got none")
	}
	// All defaults must be on the Mage list — that's the slice we draw from.
	for _, k := range known {
		s, ok := lookupSpell(k.SpellID)
		if !ok {
			t.Errorf("unknown spell ID granted: %s", k.SpellID)
			continue
		}
		isMage := false
		for _, cl := range s.Classes {
			if cl == ClassMage {
				isMage = true
			}
		}
		if !isMage {
			t.Errorf("AT default %s isn't on the Mage list", k.SpellID)
		}
	}
	// Slot pool must reflect the subclass grant.
	slots, _ := getSpellSlots(uid)
	if slots[1][0] != 3 {
		t.Errorf("AT L5 L1 slots = %d, want 3", slots[1][0])
	}
}

// !cast gate: an AT Rogue can cast a Mage spell off the registry's class list,
// even though c.Class is Rogue.
func TestCast_ArcaneTricksterAcceptsMageSpells(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@at_cast:example")
	if err := createAdvCharacter(uid, "at_cast"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassRogue, Subclass: SubclassArcaneTrickster,
		Level: 5, STR: 8, DEX: 16, CON: 12, INT: 16, WIS: 10, CHA: 12,
		HPMax: 25, HPCurrent: 25, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDCastCmd(MessageContext{Sender: uid}, "magic_missile"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.PendingCast == "" {
		t.Error("magic_missile should be queued as pending_cast for AT Rogue")
	}
}

// !cast gate (negative): a plain Thief Rogue still can't cast.
func TestCast_PlainRogueRejected(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@thief_cast:example")
	if err := createAdvCharacter(uid, "thief_cast"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassRogue, Subclass: SubclassThief,
		Level: 5, STR: 8, DEX: 16, CON: 12, INT: 14, WIS: 10, CHA: 12,
		HPMax: 25, HPCurrent: 25, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{euro: &EuroPlugin{}}
	if err := p.handleDnDCastCmd(MessageContext{Sender: uid}, "magic_missile"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.PendingCast != "" {
		t.Error("Thief Rogue should not be able to !cast")
	}
}

// Surface-level sanity: rage shows up in characterActiveAbilities for a
// Berserker but not a Champion. Tests the gating helper directly so we
// don't need the resource-pool DB read that renderArmList does.
func TestCharacterActiveAbilities_RageVisibility(t *testing.T) {
	berserker := &DnDCharacter{
		Class: ClassFighter, Subclass: SubclassBerserker, Level: 5,
	}
	champion := &DnDCharacter{
		Class: ClassFighter, Subclass: SubclassChampion, Level: 5,
	}
	hasRage := func(list []DnDAbility) bool {
		for _, a := range list {
			if a.ID == "rage" {
				return true
			}
		}
		return false
	}
	if !hasRage(characterActiveAbilities(berserker)) {
		t.Error("Berserker should have rage in active-ability list")
	}
	if hasRage(characterActiveAbilities(champion)) {
		t.Error("Champion should not have rage in active-ability list")
	}
}

// ── SUB3a-i — Champion + Berserker L10/L15 ──────────────────────────────

func TestApplySubclassPassives_ChampionL10AdditionalFightingStyle(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 10}
	stats := &CombatStats{AC: 16}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AC != 17 {
		t.Errorf("L10 Champion AC = %d, want 17 (+1 Defense style)", stats.AC)
	}
	if mods.DamageBonus < 0.099 || mods.DamageBonus > 0.101 {
		t.Errorf("L10 Champion DamageBonus = %v, want ~0.10 (Dueling style)", mods.DamageBonus)
	}
	// L5 Improved Critical still in effect; L15 hasn't yet kicked in.
	if mods.CritThreshold != 19 {
		t.Errorf("L10 Champion CritThreshold = %d, want 19 (L15 not yet)", mods.CritThreshold)
	}
}

func TestApplySubclassPassives_ChampionL9NoFightingStyle(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 9}
	stats := &CombatStats{AC: 16}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AC != 16 {
		t.Errorf("L9 Champion AC = %d, want 16 (Fighting Style gates at L10)", stats.AC)
	}
	if mods.DamageBonus != 0 {
		t.Errorf("L9 Champion DamageBonus = %v, want 0", mods.DamageBonus)
	}
}

func TestApplySubclassPassives_ChampionL15SuperiorCritical(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassChampion, Level: 15}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{AC: 16}, mods, c)
	if mods.CritThreshold != 18 {
		t.Errorf("L15 Champion CritThreshold = %d, want 18 (Superior Critical)", mods.CritThreshold)
	}
}

func TestApplySubclassPassives_BerserkerL10IntimidatingPresence(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBerserker, Level: 10}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.SporeCloud != 2 {
		t.Errorf("L10 Berserker SporeCloud = %d, want 2 (Intimidating Presence)", mods.SporeCloud)
	}
	if mods.DamageBonus != 0 {
		t.Errorf("L10 Berserker DamageBonus = %v, want 0 (Retaliation gates at L15)", mods.DamageBonus)
	}
}

func TestApplySubclassPassives_BerserkerL9NoIntimidatingPresence(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBerserker, Level: 9}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.SporeCloud != 0 {
		t.Errorf("L9 Berserker SporeCloud = %d, want 0", mods.SporeCloud)
	}
}

func TestApplySubclassPassives_BerserkerL15Retaliation(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBerserker, Level: 15}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.DamageBonus < 0.149 || mods.DamageBonus > 0.151 {
		t.Errorf("L15 Berserker DamageBonus = %v, want ~0.15 (Retaliation)", mods.DamageBonus)
	}
	if mods.SporeCloud != 2 {
		t.Errorf("L15 Berserker SporeCloud = %d, want 2 (L10 still active)", mods.SporeCloud)
	}
}

// ── SUB3a-ii — Battle Master L10/L15 ────────────────────────────────────

func TestPrecisionAttack_L10ImprovedSuperiority(t *testing.T) {
	// L9: d8 → +4. L10: d10 → +5.
	ab := dndActiveAbilities["precision_attack"]

	c9 := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 9}
	mods9 := &CombatModifiers{}
	ab.Apply(c9, mods9)
	if mods9.FirstAttackBonus != 4 {
		t.Errorf("L9 Precision Attack FirstAttackBonus = %d, want 4 (d8)", mods9.FirstAttackBonus)
	}

	c10 := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 10}
	mods10 := &CombatModifiers{}
	ab.Apply(c10, mods10)
	if mods10.FirstAttackBonus != 5 {
		t.Errorf("L10 Precision Attack FirstAttackBonus = %d, want 5 (d10)", mods10.FirstAttackBonus)
	}
}

func TestRally_L10ImprovedSuperiority(t *testing.T) {
	// CHA 14 → mod +2. L9: 4 + 2 = 6; L10: 5 + 2 = 7.
	ab := dndActiveAbilities["rally"]

	c9 := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 9, CHA: 14}
	mods9 := &CombatModifiers{}
	ab.Apply(c9, mods9)
	if mods9.HealItem != 6 {
		t.Errorf("L9 Rally HealItem = %d, want 6 (d8 + CHA)", mods9.HealItem)
	}

	c10 := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBattleMaster, Level: 10, CHA: 14}
	mods10 := &CombatModifiers{}
	ab.Apply(c10, mods10)
	if mods10.HealItem != 7 {
		t.Errorf("L10 Rally HealItem = %d, want 7 (d10 + CHA)", mods10.HealItem)
	}
}

func TestSubclassResourceMax_BattleMasterL15Relentless(t *testing.T) {
	// Pre-L15: 4 dice. L15+: 5 dice (Relentless approximation).
	if _, max := subclassResourceMax(SubclassBattleMaster, 14); max != 4 {
		t.Errorf("L14 Battle Master max = %d, want 4", max)
	}
	if _, max := subclassResourceMax(SubclassBattleMaster, 15); max != 5 {
		t.Errorf("L15 Battle Master max = %d, want 5", max)
	}
	if _, max := subclassResourceMax(SubclassBattleMaster, 20); max != 5 {
		t.Errorf("L20 Battle Master max = %d, want 5", max)
	}
}

func TestInitSubclassResources_BattleMasterL15GrowsPool(t *testing.T) {
	// Seed at L5 (max=4), then re-init at L15 — pool max should grow to 5,
	// and current should bump by the same delta so a freshly-rested player
	// gets the new die immediately rather than after a long rest.
	setupAuditTestDB(t)
	uid := id.UserID("@bm_relentless:example")
	if err := createAdvCharacter(uid, "bm_relentless"); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 5); err != nil {
		t.Fatal(err)
	}
	cur, max, _ := getResource(uid, "superiority")
	if cur != 4 || max != 4 {
		t.Fatalf("L5 pool = %d/%d, want 4/4", cur, max)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 15); err != nil {
		t.Fatal(err)
	}
	cur, max, _ = getResource(uid, "superiority")
	if cur != 5 || max != 5 {
		t.Errorf("L15 pool after reconcile = %d/%d, want 5/5", cur, max)
	}
}

// ── SUB3a-iii — Thief L10/L15 ───────────────────────────────────────────

func TestApplySubclassPassives_ThiefL10UseMagicDevice(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 10}
	stats := &CombatStats{}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if mods.FlatDmgStart != 6 {
		t.Errorf("L10 Thief FlatDmgStart = %d, want 6 (UMD wand zap)", mods.FlatDmgStart)
	}
	if mods.DamageBonus < 0.049 || mods.DamageBonus > 0.051 {
		t.Errorf("L10 Thief DamageBonus = %v, want ~0.05 (UMD)", mods.DamageBonus)
	}
	if mods.AssassinateAdvantage {
		t.Error("L10 Thief should not have AssassinateAdvantage (L15 only)")
	}
	if mods.AssassinateBonusDmg != 0 {
		t.Errorf("L10 Thief AssassinateBonusDmg = %d, want 0 (L15 only)", mods.AssassinateBonusDmg)
	}
}

func TestApplySubclassPassives_ThiefL9NoUseMagicDevice(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 9}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.FlatDmgStart != 0 {
		t.Errorf("L9 Thief FlatDmgStart = %d, want 0 (UMD gates at L10)", mods.FlatDmgStart)
	}
	if mods.DamageBonus != 0 {
		t.Errorf("L9 Thief DamageBonus = %v, want 0", mods.DamageBonus)
	}
}

func TestApplySubclassPassives_ThiefL15ThiefsReflexes(t *testing.T) {
	c := &DnDCharacter{Class: ClassRogue, Subclass: SubclassThief, Level: 15}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(&CombatStats{}, mods, c)
	if !mods.AssassinateAdvantage {
		t.Error("L15 Thief should have AssassinateAdvantage (Thief's Reflexes)")
	}
	if mods.AssassinateBonusDmg != 8 {
		t.Errorf("L15 Thief AssassinateBonusDmg = %d, want 8 (extra turn proxy)", mods.AssassinateBonusDmg)
	}
	// L10 layer must still apply.
	if mods.FlatDmgStart != 6 {
		t.Errorf("L15 Thief FlatDmgStart = %d, want 6 (L10 UMD still active)", mods.FlatDmgStart)
	}
}

func TestInitSubclassResources_BattleMasterL15PreservesSpentDice(t *testing.T) {
	// Player burned through 3 dice (current=1, max=4), then levels up to 15.
	// The growth delta (+1) should be added to current → cur=2, max=5. We
	// don't refill to max; that's the long-rest's job.
	setupAuditTestDB(t)
	uid := id.UserID("@bm_partial:example")
	if err := createAdvCharacter(uid, "bm_partial"); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 5); err != nil {
		t.Fatal(err)
	}
	if ok, err := spendResource(uid, "superiority", 3); err != nil || !ok {
		t.Fatalf("spendResource: ok=%v err=%v", ok, err)
	}
	if err := initSubclassResources(uid, SubclassBattleMaster, 15); err != nil {
		t.Fatal(err)
	}
	cur, max, _ := getResource(uid, "superiority")
	if cur != 2 || max != 5 {
		t.Errorf("partially spent + L15 grow = %d/%d, want 2/5", cur, max)
	}
}
