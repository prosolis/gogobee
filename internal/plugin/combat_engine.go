package plugin

import "math/rand/v2"

// ── Core Types ───────────────────────────────────────────────────────────────

type CombatStats struct {
	MaxHP     int
	Attack    int
	Defense   int
	Speed     int
	CritRate  float64
	DodgeRate float64
	BlockRate float64
}

type CombatModifiers struct {
	DamageBonus    float64 // additive: 0 = neutral, 0.25 = +25% damage. Applied as (1 + DamageBonus).
	DamageReduct   float64 // multiplicative damage-taken reduction, 1.0 = neutral
	DeathSave      bool    // Sovereign reprieve — survive one lethal hit
	PetAttackProc  float64
	PetAttackDmg   int
	PetDeflectProc float64
	PetWhiffProc   float64 // pet distracts enemy → guaranteed miss
	SniperKillProc float64 // Arina instant-kill
	MistyHealProc  float64
	MistyHealAmt   int
	CrowdRevengeProc float64 // Misty debuff: chance per round of crowd damage
	CrowdRevengeDmg  int    // Misty debuff: damage per proc
	HealItem         int    // consumable: HP restored at <50% HP (one-shot)
	WardCharges      int    // consumable: hits fully absorbed
	SporeCloud     int     // consumable: rounds of 15% enemy miss chance
	ReflectNext    float64 // consumable: fraction of next hit reflected
	AutoCritFirst  bool    // consumable: first player hit is auto-crit
	FlatDmgStart   int     // consumable: flat damage to enemy pre-combat
}

type Combatant struct {
	Name     string
	Stats    CombatStats
	Mods     CombatModifiers
	IsPlayer bool
	Ability  *MonsterAbility // non-nil for monsters with a special ability
}

type CombatPhase struct {
	Name            string
	Rounds          int
	AttackWeight    float64
	DefenseWeight   float64
	SpeedWeight     float64
	EnvironmentProc float64
}

type CombatEvent struct {
	Round    int
	Phase    string
	Actor    string // "player", "enemy", "pet", "environment", "npc", "consumable"
	Action   string
	Damage   int
	PlayerHP int
	EnemyHP  int
	Desc     string // optional flavor (item name, ability name)
}

type CombatResult struct {
	PlayerWon     bool
	Events        []CombatEvent
	PlayerStartHP int
	EnemyStartHP  int
	PlayerEndHP   int
	EnemyEndHP    int
	TotalRounds   int
	Closeness     float64
	NearDeath     bool
	PetAttacked   bool
	PetDeflected  bool
	SniperKilled  bool
	MistyHealed   bool
}

// ── Monster Abilities ────────────────────────────────────────────────────────

type MonsterAbility struct {
	Name       string
	Phase      string  // "opening", "clash", "decisive", "any"
	ProcChance float64
	Effect     string  // "poison", "enrage", "armor_break", "stun", "lifesteal", "cleave"
}

// ── Default Phase Definitions ────────────────────────────────────────────────

var defaultCombatPhases = []CombatPhase{
	{"Opening", 2, 0.6, 0.8, 1.5, 0.15},
	{"Clash", 3, 1.2, 1.0, 0.8, 0.08},
	{"Decisive", 1, 1.0, 0.7, 1.0, 0.05},
}

var dungeonCombatPhases = []CombatPhase{
	{"Opening", 1, 0.8, 0.8, 1.2, 0.10},
	{"Clash", 2, 1.0, 1.0, 0.8, 0.08},
	{"Decisive", 1, 1.0, 0.7, 1.0, 0.05},
}

// ── Simulation ───────────────────────────────────────────────────────────────

// combatState tracks mutable state during the simulation.
type combatState struct {
	playerHP int
	enemyHP  int

	// Consumable one-shots
	healUsed    bool
	wardCharges int
	sporeRounds int
	reflectFrac float64
	autoCrit    bool

	// Monster ability effects
	poisonTicks  int
	poisonDmg    int
	stunPlayer   bool
	enraged      bool
	armorBroken  bool
	armorBreakAmt float64

	// Sovereign reprieve
	deathSaveUsed bool

	round  int
	events []CombatEvent
}

func SimulateCombat(player, enemy Combatant, phases []CombatPhase) CombatResult {
	st := &combatState{
		playerHP:    player.Stats.MaxHP,
		enemyHP:     enemy.Stats.MaxHP,
		wardCharges: player.Mods.WardCharges,
		sporeRounds: player.Mods.SporeCloud,
		reflectFrac: player.Mods.ReflectNext,
		autoCrit:    player.Mods.AutoCritFirst,
	}

	result := CombatResult{
		PlayerStartHP: player.Stats.MaxHP,
		EnemyStartHP:  enemy.Stats.MaxHP,
	}

	// Pre-combat: Arina sniper check
	if player.Mods.SniperKillProc > 0 && rand.Float64() < player.Mods.SniperKillProc {
		st.enemyHP = 0
		st.events = append(st.events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "npc", Action: "sniper_kill",
			Damage: enemy.Stats.MaxHP, PlayerHP: st.playerHP, EnemyHP: 0,
			Desc: "Arina",
		})
		result.SniperKilled = true
		return finalize(result, st, player, enemy)
	}

	// Pre-combat: Coal Bomb / flat start damage
	if player.Mods.FlatDmgStart > 0 {
		dmg := player.Mods.FlatDmgStart
		st.enemyHP = max(0, st.enemyHP-dmg)
		st.events = append(st.events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "flat_damage",
			Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.enemyHP <= 0 {
			return finalize(result, st, player, enemy)
		}
	}

	// Pre-combat: Misty gourmet is now a per-round heal, no pre-combat damage

	// Main simulation loop
	for _, phase := range phases {
		roundsThisPhase := phase.Rounds
		// Add slight variance: ±1 round for non-Decisive phases
		if phase.Name != "Decisive" && roundsThisPhase > 1 {
			roundsThisPhase += rand.IntN(2) // 0 or +1
		}
		for r := 0; r < roundsThisPhase; r++ {
			st.round++
			if simulateRound(st, &player, &enemy, &phase, &result) {
				return finalize(result, st, player, enemy)
			}
		}
	}

	// If we exhaust all phases without a kill, the side with more HP% wins.
	playerMax := max(1, player.Stats.MaxHP)
	enemyMax := max(1, enemy.Stats.MaxHP)
	playerPct := float64(st.playerHP) / float64(playerMax)
	enemyPct := float64(st.enemyHP) / float64(enemyMax)
	if playerPct < enemyPct {
		st.playerHP = 0
	} else {
		st.enemyHP = 0
	}
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: "exhaust", Actor: "system", Action: "timeout",
		PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})
	return finalize(result, st, player, enemy)
}

// simulateRound runs one round. Returns true if combat is over.
func simulateRound(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult) bool {
	phaseName := phase.Name

	// Monster ability: check at round start
	abilityDealtDamage := false
	if enemy.Ability != nil {
		if abilityFires(enemy.Ability, phaseName, st) {
			if applyAbility(st, player, enemy, phase, result) {
				return true
			}
			// Cleave and lifesteal deal damage — skip normal enemy attack this round
			switch enemy.Ability.Effect {
			case "cleave", "lifesteal":
				abilityDealtDamage = true
			}
		}
	}

	// Poison tick from previous round
	if st.poisonTicks > 0 {
		st.playerHP = max(0, st.playerHP-st.poisonDmg)
		st.poisonTicks--
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "poison_tick",
			Damage: st.poisonDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			if trySave(st, player, phaseName) {
				// survived
			} else {
				return true
			}
		}
	}

	// Pet whiff: if proc'd, enemy's attack this round is a guaranteed miss
	petWhiff := player.Mods.PetWhiffProc > 0 && rand.Float64() < player.Mods.PetWhiffProc
	// Pet deflect: halves incoming damage to player this round
	petDeflect := player.Mods.PetDeflectProc > 0 && rand.Float64() < player.Mods.PetDeflectProc
	if petDeflect {
		result.PetDeflected = true
	}

	// Spore cloud: enemy has 15% miss chance (decremented when enemy actually attacks)
	sporeMiss := st.sporeRounds > 0 && rand.Float64() < 0.15

	// Determine initiative
	playerSpeed := float64(player.Stats.Speed) * phase.SpeedWeight
	enemySpeed := float64(enemy.Stats.Speed) * phase.SpeedWeight
	playerInit := playerSpeed + rand.Float64()*10
	enemyInit := enemySpeed + rand.Float64()*10
	playerFirst := playerInit >= enemyInit

	if playerFirst {
		if resolvePlayerAttack(st, player, enemy, phase, result) {
			return true
		}
		if !abilityDealtDamage {
			if resolveEnemyAttack(st, player, enemy, phase, result, petWhiff, petDeflect, sporeMiss) {
				return true
			}
		}
	} else {
		if !abilityDealtDamage {
			if resolveEnemyAttack(st, player, enemy, phase, result, petWhiff, petDeflect, sporeMiss) {
				return true
			}
		}
		if resolvePlayerAttack(st, player, enemy, phase, result) {
			return true
		}
	}

	// Environmental hazard
	if phase.EnvironmentProc > 0 && rand.Float64() < phase.EnvironmentProc {
		envDmg := 2 + rand.IntN(5)
		st.playerHP = max(0, st.playerHP-envDmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "environment", Action: "environmental",
			Damage: envDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			if !trySave(st, player, phaseName) {
				return true
			}
		}
	}

	// Misty crowd revenge (debuff for declining Misty)
	if player.Mods.CrowdRevengeProc > 0 && rand.Float64() < player.Mods.CrowdRevengeProc {
		dmg := player.Mods.CrowdRevengeDmg
		st.playerHP = max(0, st.playerHP-dmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "environment", Action: "crowd_revenge",
			Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: "Misty's crowd",
		})
		if st.playerHP <= 0 {
			if !trySave(st, player, phaseName) {
				return true
			}
		}
	}

	// Pet attack
	if player.Mods.PetAttackProc > 0 && rand.Float64() < player.Mods.PetAttackProc {
		petDmg := player.Mods.PetAttackDmg + rand.IntN(5)
		st.enemyHP = max(0, st.enemyHP-petDmg)
		result.PetAttacked = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_attack",
			Damage: petDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.enemyHP <= 0 {
			return true
		}
	}

	// Misty heal
	if player.Mods.MistyHealProc > 0 && rand.Float64() < player.Mods.MistyHealProc {
		healAmt := player.Mods.MistyHealAmt
		st.playerHP = min(player.Stats.MaxHP, st.playerHP+healAmt)
		result.MistyHealed = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "npc", Action: "misty_heal",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: "Misty",
		})
	}

	// Consumable heal: triggers once when player drops below 50%
	if !st.healUsed && player.Mods.HealItem > 0 &&
		st.playerHP > 0 && st.playerHP < player.Stats.MaxHP/2 {
		st.healUsed = true
		healAmt := player.Mods.HealItem
		st.playerHP = min(player.Stats.MaxHP, st.playerHP+healAmt)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "heal_item",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	return false
}

// ── Attack Resolution ────────────────────────────────────────────────────────

func resolvePlayerAttack(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult) bool {
	phaseName := phase.Name

	// Stun: player skips attack
	if st.stunPlayer {
		st.stunPlayer = false
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "stunned",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Enemy dodge
	enemyDodge := enemy.Stats.DodgeRate * phase.SpeedWeight
	if enemyDodge > 0 && rand.Float64() < enemyDodge {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Calculate damage
	dmg := calcDamage(player.Stats.Attack, phase.AttackWeight, player.Mods.DamageBonus,
		enemy.Stats.Defense, phase.DefenseWeight, enemy.Mods.DamageReduct)

	// Block: half damage
	blocked := enemy.Stats.BlockRate > 0 && rand.Float64() < enemy.Stats.BlockRate
	if blocked {
		dmg = max(1, dmg/2)
	}

	// Crit check
	critRate := player.Stats.CritRate
	if phaseName == "Decisive" {
		critRate *= 2
	}
	isCrit := false
	if st.autoCrit {
		isCrit = true
		st.autoCrit = false
		dmg *= 2
	} else if critRate > 0 && rand.Float64() < critRate {
		isCrit = true
		dmg *= 2
	}

	dmg = max(1, dmg)

	// Cleave handling for enemy is in resolveEnemyAttack
	action := "hit"
	if isCrit {
		action = "crit"
	} else if blocked {
		action = "block"
	}

	st.enemyHP = max(0, st.enemyHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "player", Action: action,
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})

	return st.enemyHP <= 0
}

func resolveEnemyAttack(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult, petWhiff, petDeflect, sporeMiss bool) bool {
	phaseName := phase.Name

	// Spore cloud round consumed when enemy attacks (even if whiffed/missed)
	if st.sporeRounds > 0 {
		st.sporeRounds--
	}

	// Pet whiff → guaranteed miss
	if petWhiff {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_whiff",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Spore cloud miss
	if sporeMiss {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "spore_miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Player dodge
	playerDodge := player.Stats.DodgeRate * phase.SpeedWeight
	if playerDodge > 0 && rand.Float64() < playerDodge {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Ward: absorb full hit
	if st.wardCharges > 0 {
		st.wardCharges--
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "ward_absorb",
			Damage: 0, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	atkMult := 1.0
	if st.enraged {
		atkMult = 1.5
	}
	dmg := calcDamage(int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
		playerDefense(player, st), phase.DefenseWeight, player.Mods.DamageReduct)

	// Player block
	blocked := player.Stats.BlockRate > 0 && rand.Float64() < player.Stats.BlockRate
	if blocked {
		dmg = max(1, dmg/2)
	}

	// Crit
	critRate := enemy.Stats.CritRate
	if phaseName == "Decisive" {
		critRate *= 2
	}
	isCrit := critRate > 0 && rand.Float64() < critRate
	if isCrit {
		dmg *= 2
	}

	dmg = max(1, dmg)

	// Pet deflect: halve damage
	if petDeflect {
		dmg = max(1, dmg/2)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_deflect",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	action := "hit"
	if isCrit {
		action = "crit"
	} else if blocked {
		action = "block"
	}

	st.playerHP = max(0, st.playerHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "enemy", Action: action,
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})

	// Reflect: bounce portion back (after player takes the hit)
	if st.reflectFrac > 0 {
		reflected := max(1, int(float64(dmg)*st.reflectFrac))
		st.reflectFrac = 0
		st.enemyHP = max(0, st.enemyHP-reflected)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "reflect_damage",
			Damage: reflected, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.enemyHP <= 0 {
			return true
		}
	}

	if st.playerHP <= 0 {
		return !trySave(st, player, phaseName)
	}
	return false
}

// ── Monster Ability Logic ────────────────────────────────────────────────────

func abilityFires(ability *MonsterAbility, phaseName string, st *combatState) bool {
	phaseMatch := ability.Phase == "any" ||
		(ability.Phase == "opening" && phaseName == "Opening") ||
		(ability.Phase == "clash" && phaseName == "Clash") ||
		(ability.Phase == "decisive" && phaseName == "Decisive")
	if !phaseMatch {
		return false
	}
	return rand.Float64() < ability.ProcChance
}

func applyAbility(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult) bool {
	ab := enemy.Ability
	phaseName := phase.Name

	switch ab.Effect {
	case "poison":
		st.poisonTicks = 2
		st.poisonDmg = 3 + rand.IntN(3)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "poison",
			Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "enrage":
		if !st.enraged && st.enemyHP < int(float64(enemy.Stats.MaxHP)*0.4) {
			st.enraged = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "enrage",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "armor_break":
		if !st.armorBroken {
			st.armorBroken = true
			st.armorBreakAmt = 0.30
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "armor_break",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "stun":
		st.stunPlayer = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "stun",
			Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "lifesteal":
		atkMult := 1.0
		if st.enraged {
			atkMult = 1.5
		}
		dmg := calcDamage(int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
			playerDefense(player, st), phase.DefenseWeight, player.Mods.DamageReduct)
		dmg = max(1, dmg)
		st.playerHP = max(0, st.playerHP-dmg)
		heal := dmg / 2
		st.enemyHP = min(enemy.Stats.MaxHP, st.enemyHP+heal)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "lifesteal",
			Damage: dmg, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			return !trySave(st, player, phaseName)
		}

	case "cleave":
		atkMult := 1.0
		if st.enraged {
			atkMult = 1.5
		}
		dmg1 := calcDamage(int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
			playerDefense(player, st), phase.DefenseWeight, player.Mods.DamageReduct)
		dmg1 = max(1, dmg1)
		st.playerHP = max(0, st.playerHP-dmg1)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "cleave",
			Damage: dmg1, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			if !trySave(st, player, phaseName) {
				return true
			}
			// Death save fired — skip follow-up hit (survived the first blow barely)
		} else {
			dmg2 := max(1, dmg1/2)
			st.playerHP = max(0, st.playerHP-dmg2)
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "cleave",
				Damage: dmg2, Desc: ab.Name + " (follow-up)", PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
			if st.playerHP <= 0 {
				return !trySave(st, player, phaseName)
			}
		}
	}

	return false
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// calcDamage uses a penetration model: defense provides diminishing returns
// reduction rather than flat subtraction, so damage is never fully negated.
// Formula: rawAtk * reduction where reduction = K / (K + effectiveDef).
// K=40 means 40 defense halves incoming damage; 80 defense reduces by 67%.
func calcDamage(attack int, atkWeight, dmgBonus float64, defense int, defWeight, dmgReduct float64) int {
	const K = 40.0
	rawAtk := float64(attack) * atkWeight * (1 + dmgBonus)
	effectiveDef := float64(defense) * defWeight * dmgReduct
	reduction := K / (K + effectiveDef)
	dmg := rawAtk * reduction
	if dmg < 1 {
		return 1
	}
	return int(dmg)
}

func playerDefense(player *Combatant, st *combatState) int {
	def := player.Stats.Defense
	if st.armorBroken {
		def = int(float64(def) * (1 - st.armorBreakAmt))
	}
	return def
}

func trySave(st *combatState, player *Combatant, phaseName string) bool {
	if player.Mods.DeathSave && !st.deathSaveUsed {
		st.deathSaveUsed = true
		st.playerHP = 1
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "death_save",
			PlayerHP: 1, EnemyHP: st.enemyHP, Desc: "Sovereign",
		})
		return true
	}
	return false
}

func finalize(result CombatResult, st *combatState, player, enemy Combatant) CombatResult {
	result.Events = st.events
	result.PlayerEndHP = st.playerHP
	result.EnemyEndHP = st.enemyHP
	result.TotalRounds = st.round
	result.PlayerWon = st.enemyHP <= 0

	playerMax := max(1, player.Stats.MaxHP)
	enemyMax := max(1, enemy.Stats.MaxHP)
	if result.PlayerWon && st.playerHP > 0 {
		result.NearDeath = float64(st.playerHP) < float64(playerMax)*0.15
		winnerRemaining := float64(st.playerHP) / float64(playerMax)
		result.Closeness = 1.0 - winnerRemaining
	} else if !result.PlayerWon {
		enemyRemaining := float64(st.enemyHP) / float64(enemyMax)
		result.NearDeath = enemyRemaining < 0.15
		result.Closeness = 1.0 - enemyRemaining
	}

	return result
}
