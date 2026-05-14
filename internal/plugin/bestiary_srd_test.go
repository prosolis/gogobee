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
