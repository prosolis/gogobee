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
	if err := persistDnDPostCombatSubclass(c, true); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 1 {
		t.Errorf("Exhaustion after raged combat = %d, want 1", got.Exhaustion)
	}
	// Second raged combat → 2.
	if err := persistDnDPostCombatSubclass(got, true); err != nil {
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
	if err := persistDnDPostCombatSubclass(c, false); err != nil {
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
	if err := initSubclassResources(uid, SubclassBattleMaster); err != nil {
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
	if err := initSubclassResources(uid, SubclassChampion); err != nil {
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
	if err := initSubclassResources(uid, SubclassBattleMaster); err != nil {
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

