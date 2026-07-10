package plugin

import (
	"math/rand/v2"
	"testing"
)

func TestEnemyAttackProfile_Fallback(t *testing.T) {
	// An unregistered creature makes a single attack derived from its template
	// stats — same behaviour as before the multiattack upgrade.
	stats := CombatStats{Attack: 9, AttackBonus: 4}
	got := enemyAttackProfile("not_in_registry", stats)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (single fallback attack)", len(got))
	}
	if got[0].Damage != 9 || got[0].AttackBonus != 4 {
		t.Errorf("fallback attack = %+v, want damage 9 / bonus 4", got[0])
	}
}

func TestEnemyAttackProfile_Registered(t *testing.T) {
	// A registered elite returns its full multiattack profile.
	got := enemyAttackProfile("owlbear", CombatStats{Attack: 99, AttackBonus: 99})
	if len(got) != 2 {
		t.Fatalf("owlbear len = %d, want 2 (Beak + Claws)", len(got))
	}
	// Profile values win over the template stats.
	if got[0].AttackBonus == 99 || got[1].AttackBonus == 99 {
		t.Errorf("profile should not inherit template AttackBonus: %+v", got)
	}

	// A zero per-attack AttackBonus falls back to the (tier-scaled) template
	// bonus so authoring can leave it blank.
	srdProfiles["__test_zero_bonus"] = SRDProfile{Attacks: []SRDAttack{{Name: "Slam", Damage: 5}}}
	defer delete(srdProfiles, "__test_zero_bonus")
	zb := enemyAttackProfile("__test_zero_bonus", CombatStats{Attack: 1, AttackBonus: 7})
	if zb[0].AttackBonus != 7 {
		t.Errorf("zero-bonus attack = %+v, want AttackBonus 7 from template", zb[0])
	}
}

// testCombatState seats a single actor, the shape every direct-primitive test
// wants. The engine reads per-actor state through the embedded cursor, so a
// combatState with a nil actor panics on the first st.playerHP touch.
func testCombatState(playerHP, enemyHP, round int, rng *rand.Rand) *combatState {
	a := &actor{playerHP: playerHP}
	return &combatState{
		actor:   a,
		actors:  []*actor{a},
		enemyHP: enemyHP,
		round:   round,
		rng:     rng,
	}
}

func TestTurnAbilityFires(t *testing.T) {
	enemy := baseEnemy() // MaxHP 60
	st := testCombatState(0, 0, 0, rand.New(rand.NewPCG(1, 1)))

	cases := []struct {
		name     string
		phase    string
		round    int
		enemyHP  int
		eligible bool
	}{
		{"any always eligible", "any", 5, 60, true},
		{"opening round 1", "opening", 1, 60, true},
		{"opening past round 1", "opening", 2, 60, false},
		{"clash past round 1", "clash", 2, 60, true},
		{"clash round 1", "clash", 1, 60, false},
		{"decisive when bloodied", "decisive", 3, 20, true},
		{"decisive when healthy", "decisive", 3, 60, false},
	}
	for _, c := range cases {
		st.round = c.round
		st.enemyHP = c.enemyHP
		// ProcChance 1.0: fires iff eligible. ProcChance 0.0: never fires.
		alwaysProc := &MonsterAbility{Phase: c.phase, ProcChance: 1.0}
		if got := turnAbilityFires(alwaysProc, st, &enemy); got != c.eligible {
			t.Errorf("%s: fired=%v, want %v", c.name, got, c.eligible)
		}
		neverProc := &MonsterAbility{Phase: c.phase, ProcChance: 0.0}
		if turnAbilityFires(neverProc, st, &enemy) {
			t.Errorf("%s: a 0%% proc chance ability should never fire", c.name)
		}
	}
}

// TestApplyAbility_Slice2Effects covers the immediate-resolution monster
// ability effects: damage riders and the enemy self-heal. All resolve fully
// within applyAbility with no persistent state.
func TestApplyAbility_Slice2Effects(t *testing.T) {
	newState := func(playerHP, enemyHP int) (*combatState, Combatant, Combatant) {
		st := testCombatState(playerHP, enemyHP, 1, rand.New(rand.NewPCG(7, 7)))
		return st, basePlayer(), baseEnemy()
	}
	phase := &turnCombatPhase
	res := &CombatResult{}

	damageEffects := []string{"bonus_damage", "aoe", "aoe_fire", "death_aoe", "execute"}
	for _, eff := range damageEffects {
		st, player, enemy := newState(100, 60)
		enemy.Ability = &MonsterAbility{Name: "Test " + eff, Effect: eff}
		if applyAbility(st, &player, &enemy, phase, res) {
			t.Errorf("%s: should not down a full-HP player", eff)
		}
		if st.playerHP >= 100 {
			t.Errorf("%s: player HP = %d, want < 100 (damage rider)", eff, st.playerHP)
		}
		if len(st.events) != 1 || st.events[0].Damage <= 0 {
			t.Errorf("%s: want one event with positive damage, got %+v", eff, st.events)
		}
	}

	// execute hits harder once the player is under 30% HP.
	stLow, pl, en := newState(20, 60)
	en.Ability = &MonsterAbility{Name: "Wail", Effect: "execute"}
	applyAbility(stLow, &pl, &en, phase, res)
	lowDmg := 20 - stLow.playerHP
	stHigh, pl2, en2 := newState(100, 60)
	en2.Ability = &MonsterAbility{Name: "Wail", Effect: "execute"}
	applyAbility(stHigh, &pl2, &en2, phase, res)
	highDmg := 100 - stHigh.playerHP
	if lowDmg <= highDmg {
		t.Errorf("execute: low-HP damage %d should exceed full-HP damage %d", lowDmg, highDmg)
	}

	// self_heal restores enemy HP, capped at MaxHP.
	stHeal, plH, enH := newState(100, 30)
	enH.Ability = &MonsterAbility{Name: "Regrow", Effect: "self_heal"}
	applyAbility(stHeal, &plH, &enH, phase, res)
	if stHeal.enemyHP <= 30 || stHeal.enemyHP > enH.Stats.MaxHP {
		t.Errorf("self_heal: enemy HP = %d, want in (30, %d]", stHeal.enemyHP, enH.Stats.MaxHP)
	}

	// A damage rider that drops the player returns true (no death save armed).
	stKill, plK, enK := newState(1, 60)
	enK.Ability = &MonsterAbility{Name: "Smash", Effect: "bonus_damage"}
	if !applyAbility(stKill, &plK, &enK, phase, res) {
		t.Error("bonus_damage: lethal hit on a 1-HP player should return true")
	}
}

// TestApplyAbility_Slice3Effects covers the stateful monster-ability effects:
// applyAbility arms a combatState flag/counter (and emits an announcement for
// all but evade), and the shared resolution primitives read that state.
func TestApplyAbility_Slice3Effects(t *testing.T) {
	newState := func(playerHP, enemyHP int) (*combatState, Combatant, Combatant) {
		st := testCombatState(playerHP, enemyHP, 1, rand.New(rand.NewPCG(9, 9)))
		return st, basePlayer(), baseEnemy()
	}
	phase := &turnCombatPhase
	res := &CombatResult{}

	// Each effect arms its combatState field and (except evade) announces itself.
	st, pl, en := newState(100, 60)
	en.Ability = &MonsterAbility{Name: "Scurry", Effect: "evade"}
	applyAbility(st, &pl, &en, phase, res)
	if !st.enemyEvadeNext || len(st.events) != 0 {
		t.Errorf("evade: want enemyEvadeNext set and no event, got flag=%v events=%d", st.enemyEvadeNext, len(st.events))
	}

	armChecks := []struct {
		effect string
		armed  func(*combatState) bool
	}{
		{"block", func(s *combatState) bool { return s.enemyBlockUp }},
		{"advantage", func(s *combatState) bool { return s.enemyAdvantage }},
		{"retaliate", func(s *combatState) bool { return s.enemyRetaliateFrac > 0 }},
		{"regenerate", func(s *combatState) bool { return s.enemyRegen > 0 }},
		{"survive_at_1", func(s *combatState) bool { return s.enemySurviveArmed }},
		{"stat_drain", func(s *combatState) bool { return s.playerAtkDrain > 0 }},
		{"debuff", func(s *combatState) bool { return s.playerACDebuff > 0 }},
	}
	for _, c := range armChecks {
		st, pl, en := newState(100, 60)
		en.Ability = &MonsterAbility{Name: "Test " + c.effect, Effect: c.effect}
		if applyAbility(st, &pl, &en, phase, res) {
			t.Errorf("%s: should not down a full-HP player", c.effect)
		}
		if !c.armed(st) {
			t.Errorf("%s: combatState flag not armed", c.effect)
		}
		if len(st.events) != 1 {
			t.Errorf("%s: want one announcement event, got %d", c.effect, len(st.events))
		}
	}

	// max_hp_drain lowers effective MaxHP and deals that much immediate damage.
	stD, plD, enD := newState(100, 60)
	enD.Ability = &MonsterAbility{Name: "Corrupting Touch", Effect: "max_hp_drain"}
	applyAbility(stD, &plD, &enD, phase, res)
	if stD.maxHPDrain <= 0 || stD.playerHP != 100-stD.maxHPDrain {
		t.Errorf("max_hp_drain: drain=%d playerHP=%d, want playerHP = 100-drain", stD.maxHPDrain, stD.playerHP)
	}

	// enemyDown lets survive_at_1 cheat death exactly once.
	stS := testCombatState(100, 0, 1, rand.New(rand.NewPCG(13, 13)))
	stS.enemySurviveArmed = true
	if enemyDown(stS, "Duel") {
		t.Error("survive_at_1: armed enemy at 0 HP should not be down")
	}
	if stS.enemyHP != 1 || stS.enemySurviveArmed {
		t.Errorf("survive_at_1: want enemyHP=1 and flag cleared, got hp=%d armed=%v", stS.enemyHP, stS.enemySurviveArmed)
	}
	stS.enemyHP = 0
	if !enemyDown(stS, "Duel") {
		t.Error("survive_at_1: second drop to 0 should be lethal (one-shot spent)")
	}

	// evade is consumed by the next player weapon attack — a guaranteed miss.
	stE, plE, enE := newState(100, 60)
	stE.enemyEvadeNext = true
	if resolvePlayerAttack(stE, &plE, &enE, phase, res) {
		t.Error("evade: a whiffed attack should not end the fight")
	}
	if stE.enemyEvadeNext || stE.enemyHP != 60 {
		t.Errorf("evade: want flag cleared and enemyHP unchanged, got flag=%v hp=%d", stE.enemyEvadeNext, stE.enemyHP)
	}

	// retaliate reflects a fraction of every player hit back at the player.
	stR, plR, enR := newState(100, 60)
	stR.enemyRetaliateFrac = 0.5
	resolvePlayerAttack(stR, &plR, &enR, phase, res)
	if stR.playerHP >= 100 {
		t.Errorf("retaliate: player HP = %d, want < 100 (reflected damage)", stR.playerHP)
	}
}

// TestApplyAbility_Slice4Effects covers the former flavor-only placeholders,
// now backed by real state: applyAbility arms a combatState field and announces
// itself, and the shared resolution primitives / helpers read that state.
func TestApplyAbility_Slice4Effects(t *testing.T) {
	newState := func(playerHP, enemyHP int) (*combatState, Combatant, Combatant) {
		st := testCombatState(playerHP, enemyHP, 1, rand.New(rand.NewPCG(11, 11)))
		return st, basePlayer(), baseEnemy()
	}
	phase := &turnCombatPhase
	res := &CombatResult{}

	armChecks := []struct {
		effect string
		armed  func(*combatState) bool
	}{
		{"spell_resist", func(s *combatState) bool { return s.enemySpellResist }},
		{"reveal_action", func(s *combatState) bool { return s.enemyRevealNext }},
		{"fear_immune", func(s *combatState) bool { return s.enemyFearImmune }},
		{"ally_buff", func(s *combatState) bool { return s.enemyAtkBuff > 0 }},
	}
	for _, c := range armChecks {
		st, pl, en := newState(100, 60)
		en.Ability = &MonsterAbility{Name: "Test " + c.effect, Effect: c.effect}
		if applyAbility(st, &pl, &en, phase, res) {
			t.Errorf("%s: should not down a full-HP player", c.effect)
		}
		if !c.armed(st) {
			t.Errorf("%s: combatState field not armed", c.effect)
		}
		if st.playerHP != 100 || st.enemyHP != 60 {
			t.Errorf("%s: HP changed (%d/%d), want 100/60", c.effect, st.playerHP, st.enemyHP)
		}
		if len(st.events) != 1 {
			t.Errorf("%s: want one announcement event, got %d", c.effect, len(st.events))
		}
	}

	// The passive immunities are honoured straight off the ability profile,
	// before the per-round proc has armed the flag.
	st, _, en := newState(100, 60)
	en.Ability = &MonsterAbility{Name: "Spell Immunity", Effect: "spell_resist"}
	if !enemyResistsSpells(&en, st) {
		t.Error("enemyResistsSpells: should be true from the ability profile alone")
	}
	en.Ability = &MonsterAbility{Name: "Draconic Loyalty", Effect: "fear_immune"}
	if !enemyImmuneToControl(&en, st) {
		t.Error("enemyImmuneToControl: should be true from the ability profile alone")
	}

	// ally_buff accumulates but is capped.
	stB, plB, enB := newState(100, 60)
	enB.Ability = &MonsterAbility{Name: "Rally", Effect: "ally_buff"}
	for i := 0; i < 20; i++ {
		applyAbility(stB, &plB, &enB, phase, res)
	}
	if stB.enemyAtkBuff != 15 {
		t.Errorf("ally_buff: accumulated to %d, want the 15 cap", stB.enemyAtkBuff)
	}
	if got := enemyAttackStat(&enB, stB, 1.0); got != enB.Stats.Attack+15 {
		t.Errorf("enemyAttackStat: got %d, want %d (base + buff)", got, enB.Stats.Attack+15)
	}

	// reveal_action is consumed by the next player weapon attack.
	stR, plR, enR := newState(100, 60)
	stR.enemyRevealNext = true
	resolvePlayerAttack(stR, &plR, &enR, phase, res)
	if stR.enemyRevealNext {
		t.Error("reveal_action: flag should be cleared after the player attack")
	}
}

// TestTurnEngine_Multiattack drives a single enemy_turn and confirms a
// registered multiattack creature resolves one attack roll per profile entry,
// while an unregistered creature still resolves exactly one.
func TestTurnEngine_Multiattack(t *testing.T) {
	countEnemyEvents := func(events []CombatEvent) int {
		n := 0
		for _, e := range events {
			if e.Actor == "enemy" {
				n++
			}
		}
		return n
	}

	// Registered: owlbear has a 2-attack profile. Pools are huge so the fight
	// can't end mid-loop and every swing emits its own enemy event.
	sess := turnSession(CombatPhaseEnemyTurn, 100000, 100000)
	sess.EnemyID = "owlbear"
	player, enemy := basePlayer(), baseEnemy()
	events, err := stepEngine(sess, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	if got := countEnemyEvents(events); got != 2 {
		t.Errorf("owlbear enemy events = %d, want 2 (multiattack profile)", got)
	}

	// Unregistered: a single attack, as before the upgrade.
	sess2 := turnSession(CombatPhaseEnemyTurn, 100000, 100000)
	sess2.EnemyID = "goblin"
	events2, err := stepEngine(sess2, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	if got := countEnemyEvents(events2); got != 1 {
		t.Errorf("goblin enemy events = %d, want 1 (single-attack fallback)", got)
	}
}
