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

func TestTurnAbilityFires(t *testing.T) {
	enemy := baseEnemy() // MaxHP 60
	st := &combatState{rng: rand.New(rand.NewPCG(1, 1))}

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
// ability effects: damage riders, the enemy self-heal, and the flavor-only
// placeholders. All resolve fully within applyAbility with no persistent state.
func TestApplyAbility_Slice2Effects(t *testing.T) {
	newState := func(playerHP, enemyHP int) (*combatState, Combatant, Combatant) {
		st := &combatState{
			playerHP: playerHP, enemyHP: enemyHP, round: 1,
			rng: rand.New(rand.NewPCG(7, 7)),
		}
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

	// Flavor-only placeholders emit an event but change no HP.
	for _, eff := range []string{"spell_resist", "reveal_action", "fear_immune", "ally_buff"} {
		st, player, enemy := newState(100, 60)
		enemy.Ability = &MonsterAbility{Name: "Test " + eff, Effect: eff}
		applyAbility(st, &player, &enemy, phase, res)
		if st.playerHP != 100 || st.enemyHP != 60 {
			t.Errorf("%s: HP changed (%d/%d), want 100/60 (flavor-only)", eff, st.playerHP, st.enemyHP)
		}
		if len(st.events) != 1 || st.events[0].Action != "ability_flavor" {
			t.Errorf("%s: want one ability_flavor event, got %+v", eff, st.events)
		}
	}

	// A damage rider that drops the player returns true (no death save armed).
	stKill, plK, enK := newState(1, 60)
	enK.Ability = &MonsterAbility{Name: "Smash", Effect: "bonus_damage"}
	if !applyAbility(stKill, &plK, &enK, phase, res) {
		t.Error("bonus_damage: lethal hit on a 1-HP player should return true")
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
