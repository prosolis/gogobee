package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 10 SUB2c — Cleric Channel Divinity (Life / War / Trickery).

// ── Life Domain ─────────────────────────────────────────────────────────

func TestLifeDomain_DiscipleOfLifeAddsToHeal(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassLifeDomain, Level: 5}
	cure, _ := lookupSpell("cure_wounds") // L1 heal
	if got := lifeDomainHealBonus(c, cure, 1); got != 3 {
		t.Errorf("Disciple of Life L1 slot bonus = %d, want 3 (2 + slot 1)", got)
	}
	if got := lifeDomainHealBonus(c, cure, 3); got != 5 {
		t.Errorf("upcast L3 bonus = %d, want 5 (2 + slot 3)", got)
	}
}

func TestLifeDomain_DiscipleOfLifePreL5Skipped(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassLifeDomain, Level: 4}
	cure, _ := lookupSpell("cure_wounds")
	if got := lifeDomainHealBonus(c, cure, 1); got != 0 {
		t.Errorf("pre-L5 Life Domain bonus = %d, want 0", got)
	}
}

func TestLifeDomain_DiscipleOfLifeNotForOtherSubclass(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassWarDomain, Level: 5}
	cure, _ := lookupSpell("cure_wounds")
	if got := lifeDomainHealBonus(c, cure, 1); got != 0 {
		t.Errorf("War Domain bonus = %d, want 0", got)
	}
}

func TestLifeDomain_DiscipleOfLifeOnlyOnHealSpells(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassLifeDomain, Level: 5}
	bolt, _ := lookupSpell("guiding_bolt") // damage spell
	if got := lifeDomainHealBonus(c, bolt, 1); got != 0 {
		t.Errorf("non-heal spell bonus = %d, want 0", got)
	}
}

// Preserve Life Channel Divinity — armed ability heals at low HP.
func TestPreserveLife_HealItemSetByArmedAbility(t *testing.T) {
	ab, ok := dndActiveAbilities["preserve_life"]
	if !ok {
		t.Fatal("preserve_life not registered")
	}
	if ab.Class != ClassCleric || ab.Subclass != SubclassLifeDomain {
		t.Errorf("preserve_life gating: class=%s sub=%s, want cleric/life_domain",
			ab.Class, ab.Subclass)
	}
	if ab.Resource != "channel_divinity" {
		t.Errorf("preserve_life resource = %s, want channel_divinity", ab.Resource)
	}
	c := &DnDCharacter{
		Class: ClassCleric, Subclass: SubclassLifeDomain, Level: 5,
		HPMax: 50,
	}
	mods := &CombatModifiers{}
	ab.Apply(c, mods)
	// 5×L = 25, capped at HPMax/2 = 25. Both equal here.
	if mods.HealItem != 25 {
		t.Errorf("preserve_life HealItem = %d, want 25 (5×L=25, cap HPMax/2=25)", mods.HealItem)
	}
}

func TestPreserveLife_CappedAtHalfMaxHP(t *testing.T) {
	ab := dndActiveAbilities["preserve_life"]
	// L7 → 5*7=35 raw; HPMax 40 → cap 20.
	c := &DnDCharacter{
		Class: ClassCleric, Subclass: SubclassLifeDomain, Level: 7,
		HPMax: 40,
	}
	mods := &CombatModifiers{}
	ab.Apply(c, mods)
	if mods.HealItem != 20 {
		t.Errorf("preserve_life capped HealItem = %d, want 20 (HPMax/2)", mods.HealItem)
	}
}

// ── War Domain ──────────────────────────────────────────────────────────

func TestWarDomain_WarPriestPassive(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassWarDomain, Level: 5}
	stats := &CombatStats{AttackBonus: 4}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AttackBonus != 5 {
		t.Errorf("War Priest AttackBonus = %d, want 5 (4 + 1)", stats.AttackBonus)
	}
	if mods.DamageBonus < 0.149 || mods.DamageBonus > 0.151 {
		t.Errorf("War Priest DamageBonus = %v, want ~0.15", mods.DamageBonus)
	}
}

func TestWarDomain_PreL5NoPassive(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassWarDomain, Level: 4}
	stats := &CombatStats{AttackBonus: 4}
	mods := &CombatModifiers{DamageReduct: 1.0}
	applySubclassPassives(stats, mods, c)
	if stats.AttackBonus != 4 || mods.DamageBonus != 0 {
		t.Errorf("pre-L5 War Domain leaked: atk=%d dmg=%v", stats.AttackBonus, mods.DamageBonus)
	}
}

func TestGuidedStrike_FirstAttackBonus(t *testing.T) {
	ab, ok := dndActiveAbilities["guided_strike"]
	if !ok {
		t.Fatal("guided_strike not registered")
	}
	if ab.Class != ClassCleric || ab.Subclass != SubclassWarDomain {
		t.Errorf("guided_strike gating: class=%s sub=%s", ab.Class, ab.Subclass)
	}
	if ab.Resource != "channel_divinity" {
		t.Errorf("guided_strike resource = %s, want channel_divinity", ab.Resource)
	}
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassWarDomain, Level: 5}
	mods := &CombatModifiers{}
	ab.Apply(c, mods)
	if mods.FirstAttackBonus != 10 {
		t.Errorf("guided_strike FirstAttackBonus = %d, want 10", mods.FirstAttackBonus)
	}
}

// ── Trickery Domain ─────────────────────────────────────────────────────

func TestTrickeryDomain_CloakOfShadowsL7Passive(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassTrickeryDomain, Level: 7}
	mods := &CombatModifiers{}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.SporeCloud != 2 {
		t.Errorf("Cloak of Shadows SporeCloud = %d, want 2", mods.SporeCloud)
	}
}

func TestTrickeryDomain_PreL7NoCloak(t *testing.T) {
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassTrickeryDomain, Level: 6}
	mods := &CombatModifiers{}
	applySubclassPassives(&CombatStats{}, mods, c)
	if mods.SporeCloud != 0 {
		t.Errorf("pre-L7 Trickery SporeCloud = %d, want 0", mods.SporeCloud)
	}
}

func TestInvokeDuplicity_AdvantageAndDamage(t *testing.T) {
	ab, ok := dndActiveAbilities["invoke_duplicity"]
	if !ok {
		t.Fatal("invoke_duplicity not registered")
	}
	if ab.Class != ClassCleric || ab.Subclass != SubclassTrickeryDomain {
		t.Errorf("invoke_duplicity gating: class=%s sub=%s", ab.Class, ab.Subclass)
	}
	c := &DnDCharacter{Class: ClassCleric, Subclass: SubclassTrickeryDomain, Level: 5}
	mods := &CombatModifiers{}
	ab.Apply(c, mods)
	if !mods.AssassinateAdvantage {
		t.Error("AssassinateAdvantage not set")
	}
	if mods.DamageBonus < 0.099 || mods.DamageBonus > 0.101 {
		t.Errorf("DamageBonus = %v, want ~0.10", mods.DamageBonus)
	}
}

// ── Channel Divinity resource pool ──────────────────────────────────────

func TestChannelDivinity_ResourceMaxForAllClericSubs(t *testing.T) {
	for _, sub := range []DnDSubclass{
		SubclassLifeDomain, SubclassWarDomain, SubclassTrickeryDomain,
	} {
		res, max := subclassResourceMax(sub)
		if res != "channel_divinity" {
			t.Errorf("%s resource = %q, want channel_divinity", sub, res)
		}
		if max != 2 {
			t.Errorf("%s channel_divinity max = %d, want 2", sub, max)
		}
	}
}

func TestChannelDivinity_InitAndSpend(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@cd_pool:example")
	if err := createAdvCharacter(uid, "cd_pool"); err != nil {
		t.Fatal(err)
	}
	if err := initSubclassResources(uid, SubclassLifeDomain); err != nil {
		t.Fatal(err)
	}
	cur, max, err := getResource(uid, "channel_divinity")
	if err != nil {
		t.Fatal(err)
	}
	if cur != 2 || max != 2 {
		t.Errorf("channel_divinity init = %d/%d, want 2/2", cur, max)
	}
	ok, err := spendResource(uid, "channel_divinity", 1)
	if err != nil || !ok {
		t.Fatalf("spend failed: ok=%v err=%v", ok, err)
	}
	cur, _, _ = getResource(uid, "channel_divinity")
	if cur != 1 {
		t.Errorf("after spend cur = %d, want 1", cur)
	}
}

// ── End-to-end heal hook ────────────────────────────────────────────────

func TestResolveHealOutOfCombat_LifeDomainBonusApplied(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@life_heal:example")
	if err := createAdvCharacter(uid, "life_heal"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassCleric, Subclass: SubclassLifeDomain,
		Level: 5, STR: 10, DEX: 10, CON: 12, INT: 8, WIS: 16, CHA: 12, // WIS +3
		HPMax: 40, HPCurrent: 1, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := setSpellSlotsForLevel(uid, ClassCleric, 5); err != nil {
		t.Fatal(err)
	}
	cure, _ := lookupSpell("cure_wounds")
	p := &AdventurePlugin{}
	// Run several casts to bracket the variance: heal range is
	// 1d8 + WIS(+3) + Disciple(2+1=3) = [7..14]. Without the +3 bonus,
	// max would be 11 — so any HP gain ≥ 12 proves the bonus landed.
	highWater := 0
	for i := 0; i < 20; i++ {
		c.HPCurrent = 1
		_ = SaveDnDCharacter(c)
		// Refund a slot for each iteration.
		_ = setSpellSlotsForLevel(uid, ClassCleric, 5)
		if err := p.resolveHealOutOfCombat(MessageContext{Sender: uid}, c, cure, 1); err != nil {
			t.Fatal(err)
		}
		got, _ := LoadDnDCharacter(uid)
		if got.HPCurrent-1 > highWater {
			highWater = got.HPCurrent - 1
		}
	}
	if highWater < 12 {
		t.Errorf("Life Domain heal high-water = %d, want >=12 (proves +3 Disciple bonus)", highWater)
	}
}
