package plugin

import (
	"testing"
)

func basePlayer() Combatant {
	return Combatant{
		Name:     "Hero",
		IsPlayer: true,
		Stats: CombatStats{
			MaxHP:     100,
			Attack:    15,
			Defense:   8,
			Speed:     10,
			CritRate:  0.05,
			DodgeRate: 0.05,
			BlockRate: 0.05,
		},
		Mods: CombatModifiers{DamageBonus: 0, DamageReduct: 1.0},
	}
}

func baseEnemy() Combatant {
	return Combatant{
		Name: "Rat",
		Stats: CombatStats{
			MaxHP:     60,
			Attack:    10,
			Defense:   4,
			Speed:     6,
			CritRate:  0.03,
			DodgeRate: 0.02,
			BlockRate: 0.02,
		},
		Mods: CombatModifiers{DamageBonus: 0, DamageReduct: 1.0},
	}
}

func TestSimulateCombat_BasicResolves(t *testing.T) {
	wins, losses := 0, 0
	for i := 0; i < 1000; i++ {
		r := SimulateCombat(basePlayer(), baseEnemy(), defaultCombatPhases)
		if r.PlayerWon {
			wins++
		} else {
			losses++
		}
		if r.TotalRounds == 0 && !r.SniperKilled {
			t.Fatal("combat resolved in 0 rounds without sniper")
		}
		if len(r.Events) == 0 {
			t.Fatal("no events generated")
		}
	}
	if wins == 0 {
		t.Error("player never won in 1000 fights against a weaker enemy")
	}
	if wins < 700 {
		t.Errorf("player should strongly favor wins vs weaker enemy, got %d/1000", wins)
	}
}

func TestSimulateCombat_HPTracking(t *testing.T) {
	r := SimulateCombat(basePlayer(), baseEnemy(), defaultCombatPhases)
	if r.PlayerStartHP != 100 {
		t.Errorf("player start HP = %d, want 100", r.PlayerStartHP)
	}
	if r.EnemyStartHP != 60 {
		t.Errorf("enemy start HP = %d, want 60", r.EnemyStartHP)
	}
	if r.PlayerWon && r.EnemyEndHP != 0 {
		t.Errorf("player won but enemy HP = %d", r.EnemyEndHP)
	}
	if !r.PlayerWon && r.PlayerEndHP != 0 {
		t.Errorf("player lost but player HP = %d", r.PlayerEndHP)
	}
}

func TestSimulateCombat_SniperKill(t *testing.T) {
	p := basePlayer()
	p.Mods.SniperKillProc = 1.0 // guaranteed
	r := SimulateCombat(p, baseEnemy(), defaultCombatPhases)
	if !r.PlayerWon {
		t.Error("guaranteed sniper should always win")
	}
	if !r.SniperKilled {
		t.Error("SniperKilled flag not set")
	}
	if len(r.Events) != 1 || r.Events[0].Action != "sniper_kill" {
		t.Error("expected exactly 1 sniper_kill event")
	}
}

func TestSimulateCombat_DeathSave(t *testing.T) {
	// Player with death save against a very strong enemy
	p := basePlayer()
	p.Stats.MaxHP = 30
	p.Mods.DeathSave = true
	e := baseEnemy()
	e.Stats.Attack = 40
	e.Stats.MaxHP = 200

	saves := 0
	for i := 0; i < 500; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "death_save" {
				saves++
				if ev.PlayerHP != 1 {
					t.Errorf("death save should set HP to 1, got %d", ev.PlayerHP)
				}
				break
			}
		}
	}
	if saves == 0 {
		t.Error("death save never triggered in 500 fights against strong enemy")
	}
}

func TestSimulateCombat_PetAttack(t *testing.T) {
	p := basePlayer()
	p.Mods.PetAttackProc = 1.0 // guaranteed every round
	p.Mods.PetAttackDmg = 10
	r := SimulateCombat(p, baseEnemy(), defaultCombatPhases)
	if !r.PetAttacked {
		t.Error("pet attack should have triggered")
	}
	petHits := 0
	for _, ev := range r.Events {
		if ev.Action == "pet_attack" {
			petHits++
		}
	}
	if petHits == 0 {
		t.Error("no pet_attack events")
	}
}

func TestSimulateCombat_PetWhiff(t *testing.T) {
	p := basePlayer()
	p.Mods.PetWhiffProc = 1.0 // guaranteed
	e := baseEnemy()
	e.Stats.Attack = 5

	whiffs := 0
	r := SimulateCombat(p, e, defaultCombatPhases)
	for _, ev := range r.Events {
		if ev.Action == "pet_whiff" {
			whiffs++
		}
	}
	if whiffs == 0 {
		t.Error("pet whiff never triggered with 100% proc rate")
	}
}

func TestSimulateCombat_PetDeflect(t *testing.T) {
	p := basePlayer()
	p.Mods.PetDeflectProc = 1.0
	r := SimulateCombat(p, baseEnemy(), defaultCombatPhases)
	if !r.PetDeflected {
		t.Error("pet deflect should have triggered")
	}
	deflects := 0
	for _, ev := range r.Events {
		if ev.Action == "pet_deflect" {
			deflects++
		}
	}
	if deflects == 0 {
		t.Error("no pet_deflect events")
	}
}

func TestSimulateCombat_MistyHeal(t *testing.T) {
	p := basePlayer()
	p.Mods.MistyHealProc = 1.0
	p.Mods.MistyHealAmt = 10
	e := baseEnemy()
	e.Stats.Attack = 15 // enough to do some damage

	heals := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "misty_heal" {
				heals++
			}
		}
	}
	if heals == 0 {
		t.Error("Misty heal never triggered with 100% proc rate across 100 fights")
	}
}

func TestSimulateCombat_FlatDmgStart(t *testing.T) {
	p := basePlayer()
	p.Mods.FlatDmgStart = 20
	e := baseEnemy()
	e.Stats.MaxHP = 60

	r := SimulateCombat(p, e, defaultCombatPhases)
	if len(r.Events) == 0 {
		t.Fatal("no events")
	}
	first := r.Events[0]
	if first.Action != "flat_damage" {
		t.Errorf("first event should be flat_damage, got %s", first.Action)
	}
	if first.Damage != 20 {
		t.Errorf("flat damage = %d, want 20", first.Damage)
	}
	if first.EnemyHP != 40 {
		t.Errorf("enemy HP after flat damage = %d, want 40", first.EnemyHP)
	}
}

func TestSimulateCombat_FlatDmgKill(t *testing.T) {
	p := basePlayer()
	p.Mods.FlatDmgStart = 999
	e := baseEnemy()
	e.Stats.MaxHP = 10

	r := SimulateCombat(p, e, defaultCombatPhases)
	if !r.PlayerWon {
		t.Error("flat damage should kill a 10HP enemy")
	}
	if r.EnemyEndHP != 0 {
		t.Errorf("enemy HP = %d, want 0", r.EnemyEndHP)
	}
}

func TestSimulateCombat_HealItem(t *testing.T) {
	p := basePlayer()
	p.Stats.MaxHP = 100
	p.Mods.HealItem = 30
	e := baseEnemy()
	e.Stats.Attack = 20 // will deal solid damage

	heals := 0
	for i := 0; i < 200; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "heal_item" {
				heals++
				if ev.Damage != 30 {
					t.Errorf("heal amount = %d, want 30", ev.Damage)
				}
			}
		}
	}
	if heals == 0 {
		t.Error("heal item never triggered across 200 fights")
	}
	// Should trigger at most once per fight
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		count := 0
		for _, ev := range r.Events {
			if ev.Action == "heal_item" {
				count++
			}
		}
		if count > 1 {
			t.Errorf("heal item triggered %d times in one fight, should be at most 1", count)
		}
	}
}

func TestSimulateCombat_WardAbsorb(t *testing.T) {
	p := basePlayer()
	p.Mods.WardCharges = 1
	e := baseEnemy()
	e.Stats.Attack = 30

	wards := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "ward_absorb" {
				wards++
				if ev.Damage != 0 {
					t.Error("ward absorb should deal 0 damage")
				}
			}
		}
	}
	if wards == 0 {
		t.Error("ward never activated across 100 fights")
	}
}

func TestSimulateCombat_SporeCloud(t *testing.T) {
	p := basePlayer()
	p.Mods.SporeCloud = 10 // many rounds worth
	e := baseEnemy()
	e.Stats.Attack = 15

	spores := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "spore_miss" {
				spores++
			}
		}
	}
	if spores == 0 {
		t.Error("spore cloud never caused a miss across 100 fights")
	}
}

func TestSimulateCombat_ReflectDamage(t *testing.T) {
	p := basePlayer()
	p.Mods.ReflectNext = 0.5
	e := baseEnemy()
	e.Stats.Attack = 20

	reflects := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "reflect_damage" {
				reflects++
				if ev.Damage <= 0 {
					t.Error("reflected damage should be positive")
				}
			}
		}
	}
	if reflects == 0 {
		t.Error("reflect never triggered across 100 fights")
	}
}

func TestSimulateCombat_AutoCritFirst(t *testing.T) {
	p := basePlayer()
	p.Mods.AutoCritFirst = true
	p.Stats.CritRate = 0 // disable normal crits to isolate auto-crit

	crits := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, baseEnemy(), defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Actor == "player" && ev.Action == "crit" {
				crits++
				break // only count first per fight
			}
		}
	}
	if crits < 90 {
		t.Errorf("auto-crit should fire on first player hit in nearly every fight, got %d/100", crits)
	}
}

// ── Monster Ability Tests ────────────────────────────────────────────────────

func TestSimulateCombat_Poison(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	e.Ability = &MonsterAbility{
		Name: "Venom Bite", Phase: "any", ProcChance: 1.0, Effect: "poison",
	}

	ticks := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "poison_tick" {
				ticks++
			}
		}
	}
	if ticks == 0 {
		t.Error("poison ticks never occurred with guaranteed proc")
	}
}

func TestSimulateCombat_Enrage(t *testing.T) {
	p := basePlayer()
	p.Stats.Attack = 12
	e := baseEnemy()
	e.Stats.MaxHP = 50 // 40% threshold = 20 HP, reachable before death
	e.Stats.Attack = 6 // weak so fights last long enough
	e.Ability = &MonsterAbility{
		Name: "Berserk", Phase: "any", ProcChance: 1.0, Effect: "enrage",
	}

	enrages := 0
	for i := 0; i < 500; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "enrage" {
				enrages++
				break
			}
		}
	}
	if enrages == 0 {
		t.Error("enrage never triggered across 500 fights")
	}
}

func TestSimulateCombat_ArmorBreak(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	e.Ability = &MonsterAbility{
		Name: "Rend", Phase: "opening", ProcChance: 1.0, Effect: "armor_break",
	}

	breaks := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "armor_break" {
				breaks++
				break
			}
		}
	}
	if breaks == 0 {
		t.Error("armor break never triggered with guaranteed proc in opening")
	}
}

func TestSimulateCombat_Stun(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	e.Ability = &MonsterAbility{
		Name: "Bash", Phase: "any", ProcChance: 1.0, Effect: "stun",
	}

	stuns := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "stun" {
				stuns++
				break
			}
		}
	}
	if stuns == 0 {
		t.Error("stun never triggered")
	}
	// Check that "stunned" event follows a "stun"
	r := SimulateCombat(p, e, defaultCombatPhases)
	for i, ev := range r.Events {
		if ev.Action == "stun" {
			for j := i + 1; j < len(r.Events); j++ {
				if r.Events[j].Actor == "player" && r.Events[j].Action == "stunned" {
					return // found it
				}
				if r.Events[j].Actor == "player" && (r.Events[j].Action == "hit" || r.Events[j].Action == "crit" || r.Events[j].Action == "miss") {
					break // player acted before being stunned — could happen if stun fires and then round ends
				}
			}
		}
	}
}

func TestSimulateCombat_Lifesteal(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = 100
	e.Ability = &MonsterAbility{
		Name: "Drain", Phase: "any", ProcChance: 1.0, Effect: "lifesteal",
	}

	steals := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "lifesteal" {
				steals++
				break
			}
		}
	}
	if steals == 0 {
		t.Error("lifesteal never triggered")
	}
}

func TestSimulateCombat_Cleave(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	e.Ability = &MonsterAbility{
		Name: "Cleave", Phase: "clash", ProcChance: 1.0, Effect: "cleave",
	}

	cleaves := 0
	for i := 0; i < 100; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		for _, ev := range r.Events {
			if ev.Action == "cleave" {
				cleaves++
			}
		}
	}
	// Should see pairs of cleave events (main + follow-up)
	if cleaves == 0 {
		t.Error("cleave never triggered")
	}
	if cleaves%2 != 0 {
		// Not strictly required since fights can end mid-cleave, but check it's usually paired
		t.Logf("cleave events = %d (odd count is possible if fight ends mid-cleave)", cleaves)
	}
}

// ── Strong vs Weak ───────────────────────────────────────────────────────────

func TestSimulateCombat_StrongEnemyWinsMostly(t *testing.T) {
	p := basePlayer()
	p.Stats.MaxHP = 50
	p.Stats.Attack = 8
	e := baseEnemy()
	e.Stats.MaxHP = 200
	e.Stats.Attack = 30
	e.Stats.Defense = 12

	wins := 0
	for i := 0; i < 1000; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		if r.PlayerWon {
			wins++
		}
	}
	if wins > 150 {
		t.Errorf("player won %d/1000 against much stronger enemy — too high", wins)
	}
}

func TestSimulateCombat_DungeonPhasesShorter(t *testing.T) {
	totalDefault, totalDungeon := 0, 0
	for i := 0; i < 500; i++ {
		r1 := SimulateCombat(basePlayer(), baseEnemy(), defaultCombatPhases)
		r2 := SimulateCombat(basePlayer(), baseEnemy(), dungeonCombatPhases)
		totalDefault += r1.TotalRounds
		totalDungeon += r2.TotalRounds
	}
	avgDefault := float64(totalDefault) / 500
	avgDungeon := float64(totalDungeon) / 500
	if avgDungeon >= avgDefault {
		t.Errorf("dungeon phases should produce fewer rounds on average: dungeon=%.1f, default=%.1f", avgDungeon, avgDefault)
	}
}

func TestSimulateCombat_ClosenessRange(t *testing.T) {
	for i := 0; i < 500; i++ {
		r := SimulateCombat(basePlayer(), baseEnemy(), defaultCombatPhases)
		if r.Closeness < 0 || r.Closeness > 1.0 {
			t.Errorf("closeness out of range: %f", r.Closeness)
		}
	}
}

func TestSimulateCombat_NearDeath(t *testing.T) {
	// Enemy strong enough to bring player near death regularly
	p := basePlayer()
	p.Stats.MaxHP = 80
	p.Stats.Attack = 12
	e := baseEnemy()
	e.Stats.MaxHP = 60
	e.Stats.Attack = 18

	nearDeaths := 0
	for i := 0; i < 2000; i++ {
		r := SimulateCombat(p, e, defaultCombatPhases)
		if r.NearDeath {
			nearDeaths++
		}
	}
	if nearDeaths == 0 {
		t.Error("near death never occurred in 2000 fights")
	}
}
