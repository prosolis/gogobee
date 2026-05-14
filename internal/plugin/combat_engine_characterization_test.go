package plugin

// Characterization test for the auto-resolve combat engine.
//
// This pins the *exact* event stream SimulateCombat produces for a curated set
// of scenarios, driven by a seeded RNG so the output is reproducible. It is a
// safety net for the upcoming shared-primitives extraction (turn-based engine
// work): any change that perturbs the auto-resolve event sequence will fail
// here, forcing a deliberate `-update` regeneration + diff review.
//
// It asserts behavior, not correctness — a failing diff means "you changed
// something", not "you broke something". Review the diff, and if the change is
// intended, regenerate with:
//
//	go test ./internal/plugin -run TestCombatCharacterization -update

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateCharacterization = flag.Bool("update", false, "regenerate combat characterization golden file")

// charScenario is one pinned combat setup. The same player/enemy/phases are
// run across a fixed set of seeds so the snapshot captures multiple RNG
// branches (hit/miss/crit/proc) of the same configuration.
type charScenario struct {
	name   string
	player Combatant
	enemy  Combatant
	phases []CombatPhase
}

func characterizationScenarios() []charScenario {
	// Local builders so the scenarios don't depend on mutation order of the
	// shared basePlayer()/baseEnemy() helpers.
	weaponPlayer := func() Combatant {
		p := basePlayer()
		p.Stats.Weapon = weaponByID("wpn_longsword")
		return p
	}

	return []charScenario{
		{"basic", basePlayer(), baseEnemy(), defaultCombatPhases},
		{"dungeon_phases", basePlayer(), baseEnemy(), dungeonCombatPhases},
		{"boss_phases", func() Combatant {
			e := baseEnemy()
			e.Stats.MaxHP = 220
			e.Stats.Attack = 22
			return basePlayer()
		}(), func() Combatant {
			e := baseEnemy()
			e.Name = "Boss"
			e.Stats.MaxHP = 220
			e.Stats.Attack = 22
			return e
		}(), bossCombatPhases},
		{"strong_enemy", func() Combatant {
			p := basePlayer()
			p.Stats.MaxHP = 50
			p.Stats.Attack = 8
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.MaxHP = 200
			e.Stats.Attack = 30
			e.Stats.Defense = 12
			return e
		}(), defaultCombatPhases},
		{"sniper_kill", func() Combatant {
			p := basePlayer()
			p.Mods.SniperKillProc = 1.0
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"flat_dmg_start", func() Combatant {
			p := basePlayer()
			p.Mods.FlatDmgStart = 20
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"spell_pre_damage", func() Combatant {
			p := basePlayer()
			p.Mods.SpellPreDamage = 18
			p.Mods.SpellPreDamageDesc = "Fireball"
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"spell_enemy_skip_first", func() Combatant {
			p := basePlayer()
			p.Mods.SpellPreDamage = 5
			p.Mods.SpellPreDamageDesc = "Hold Person"
			p.Mods.SpellEnemySkipFirst = true
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"heal_item", func() Combatant {
			p := basePlayer()
			p.Mods.HealItem = 30
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 20
			return e
		}(), defaultCombatPhases},
		{"ward_absorb", func() Combatant {
			p := basePlayer()
			p.Mods.WardCharges = 1
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 30
			return e
		}(), defaultCombatPhases},
		{"spore_cloud", func() Combatant {
			p := basePlayer()
			p.Mods.SporeCloud = 10
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 15
			return e
		}(), defaultCombatPhases},
		{"reflect_next", func() Combatant {
			p := basePlayer()
			p.Mods.ReflectNext = 0.5
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 20
			return e
		}(), defaultCombatPhases},
		{"auto_crit_first", func() Combatant {
			p := basePlayer()
			p.Mods.AutoCritFirst = true
			p.Stats.CritRate = 0
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"pet_attack", func() Combatant {
			p := basePlayer()
			p.Mods.PetAttackProc = 1.0
			p.Mods.PetAttackDmg = 10
			return p
		}(), baseEnemy(), defaultCombatPhases},
		{"misty_heal", func() Combatant {
			p := basePlayer()
			p.Mods.MistyHealProc = 1.0
			p.Mods.MistyHealAmt = 10
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 15
			return e
		}(), defaultCombatPhases},
		{"death_save", func() Combatant {
			p := basePlayer()
			p.Stats.MaxHP = 30
			p.Mods.DeathSave = true
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.Attack = 40
			e.Stats.MaxHP = 200
			return e
		}(), defaultCombatPhases},
		{"ability_poison", basePlayer(), func() Combatant {
			e := baseEnemy()
			e.Ability = &MonsterAbility{Name: "Venom Bite", Phase: "any", ProcChance: 1.0, Effect: "poison"}
			return e
		}(), defaultCombatPhases},
		{"ability_enrage", func() Combatant {
			p := basePlayer()
			p.Stats.Attack = 12
			return p
		}(), func() Combatant {
			e := baseEnemy()
			e.Stats.MaxHP = 50
			e.Stats.Attack = 6
			e.Ability = &MonsterAbility{Name: "Berserk", Phase: "any", ProcChance: 1.0, Effect: "enrage"}
			return e
		}(), defaultCombatPhases},
		{"ability_armor_break", basePlayer(), func() Combatant {
			e := baseEnemy()
			e.Ability = &MonsterAbility{Name: "Rend", Phase: "opening", ProcChance: 1.0, Effect: "armor_break"}
			return e
		}(), defaultCombatPhases},
		{"ability_stun", basePlayer(), func() Combatant {
			e := baseEnemy()
			e.Ability = &MonsterAbility{Name: "Bash", Phase: "any", ProcChance: 1.0, Effect: "stun"}
			return e
		}(), defaultCombatPhases},
		{"ability_lifesteal", basePlayer(), func() Combatant {
			e := baseEnemy()
			e.Stats.MaxHP = 100
			e.Ability = &MonsterAbility{Name: "Drain", Phase: "any", ProcChance: 1.0, Effect: "lifesteal"}
			return e
		}(), defaultCombatPhases},
		{"ability_cleave", basePlayer(), func() Combatant {
			e := baseEnemy()
			e.Ability = &MonsterAbility{Name: "Cleave", Phase: "clash", ProcChance: 1.0, Effect: "cleave"}
			return e
		}(), defaultCombatPhases},
		{"weapon_profile", weaponPlayer(), baseEnemy(), defaultCombatPhases},
	}
}

// charSeeds drives each scenario. Multiple seeds widen RNG-branch coverage
// without exploding the golden file.
var charSeeds = []uint64{1, 2, 3, 7, 42}

func TestCombatCharacterization(t *testing.T) {
	var b strings.Builder
	for _, sc := range characterizationScenarios() {
		for _, seed := range charSeeds {
			// PCG with a per-(scenario,seed) stream so scenarios don't share
			// a sequence. The literal constant just disambiguates the stream.
			rng := rand.New(rand.NewPCG(seed, 0xC0FFEE))
			res := simulateCombatWithRNG(sc.player, sc.enemy, sc.phases, rng)
			fmt.Fprintf(&b, "=== %s seed=%d ===\n", sc.name, seed)
			b.WriteString(formatResult(res))
			b.WriteString("\n")
		}
	}
	got := b.String()

	goldenPath := filepath.Join("testdata", "combat_characterization.golden")
	if *updateCharacterization {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("combat event stream diverged from golden file.\n"+
			"If this change is intentional, regenerate with:\n"+
			"  go test ./internal/plugin -run TestCombatCharacterization -update\n"+
			"then review the diff.\n\n%s", firstDiff(string(want), got))
	}
}

// formatResult serializes a CombatResult to a stable, line-oriented text form.
// Every event is dumped; summary fields follow. Field order is fixed so a diff
// pinpoints exactly what moved.
func formatResult(r CombatResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "result: won=%v timedOut=%v rounds=%d\n", r.PlayerWon, r.TimedOut, r.TotalRounds)
	fmt.Fprintf(&b, "  hp: playerStart=%d playerEntry=%d playerEnd=%d enemyStart=%d enemyEntry=%d enemyEnd=%d\n",
		r.PlayerStartHP, r.PlayerEntryHP, r.PlayerEndHP, r.EnemyStartHP, r.EnemyEntryHP, r.EnemyEndHP)
	fmt.Fprintf(&b, "  flags: nearDeath=%v pet=%v petDeflect=%v sniper=%v misty=%v closeness=%.4f\n",
		r.NearDeath, r.PetAttacked, r.PetDeflected, r.SniperKilled, r.MistyHealed, r.Closeness)
	for i, e := range r.Events {
		fmt.Fprintf(&b, "  [%02d] r%d %-12s %-8s %-16s dmg=%-5d php=%-4d ehp=%-4d roll=%d/%d desc=%q\n",
			i, e.Round, e.Phase, e.Actor, e.Action, e.Damage, e.PlayerHP, e.EnemyHP, e.Roll, e.RollAgainst, e.Desc)
	}
	return b.String()
}

// firstDiff returns a short window around the first differing line, so test
// output points straight at the divergence instead of dumping the whole file.
func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			lo := i - 3
			if lo < 0 {
				lo = 0
			}
			var b strings.Builder
			fmt.Fprintf(&b, "first divergence at line %d:\n", i+1)
			for j := lo; j < i; j++ {
				if j < len(gl) {
					fmt.Fprintf(&b, "  ctx: %s\n", gl[j])
				}
			}
			fmt.Fprintf(&b, "  want: %s\n", w)
			fmt.Fprintf(&b, "  got:  %s\n", g)
			return b.String()
		}
	}
	return "(files differ but no line-level diff found — trailing whitespace?)"
}
