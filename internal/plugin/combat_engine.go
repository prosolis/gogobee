package plugin

import "math/rand/v2"

// ── Core Types ───────────────────────────────────────────────────────────────

type CombatStats struct {
	MaxHP int
	// StartHP is the HP the combatant *enters* this fight at. Zero means
	// "use MaxHP" (full health). Wounded carry-over uses this so MaxHP
	// stays stable across fights — display reads "100/123", not "100/100".
	StartHP int
	// HPBonus is the absolute HP players gain from equipment / arena sets /
	// housing on top of their D&D character sheet HP. DerivePlayerStats
	// computes this; applyDnDPlayerLayer adds it to c.HPMax to form the
	// final combat MaxHP. Kept separate so persistence to dnd_character can
	// simply clamp endHP to c.HPMax — gear cushion doesn't carry over.
	HPBonus   int
	Attack    int
	Defense   int
	Speed     int
	CritRate  float64 // superseded by nat-20 crits but still consumed by autoCrit logic & equipment scaling.
	DodgeRate float64 // not queried by hit resolution but still computed for narrative scaling.
	BlockRate float64 // still used to halve damage on a successful hit.

	// D&D layer. AC and AttackBonus drive d20-vs-AC hit resolution.
	// Set via dnd_combat.go's applyDnDPlayerLayer / applyDnDArenaMonsterLayer / applyDnDDungeonMonsterLayer.
	AC          int
	AttackBonus int

	// Phase 8 — equipment-driven damage. When Weapon is non-nil, the d20
	// attack path rolls weapon damage dice + AbilityModForDamage instead of
	// the legacy calcDamage penetration formula. When nil, legacy math
	// applies (used by monsters and any combatant without a D&D weapon).
	Weapon              *WeaponProfile
	AbilityModForDamage int
	WeaponProficient    bool // false → -4 attack penalty (appendix §8 implementation note)
	TwoHandedMode       bool // true + versatile weapon → use larger versatile die

	// FireAttacker tags monsters whose signature damage is fire (red dragons,
	// fire elementals, hell hounds, magmin, balor, …). The enemy-attack path
	// halves incoming damage when this is set on the enemy and FireResist is
	// set on the player. Carried via toCombatStats; tuned generator derives it
	// from SRD attack DamageType; hand-authored entries set it explicitly.
	FireAttacker bool
}

type CombatModifiers struct {
	DamageBonus      float64 // additive: 0 = neutral, 0.25 = +25% damage. Applied as (1 + DamageBonus).
	DamageReduct     float64 // multiplicative damage-taken reduction, 1.0 = neutral
	DeathSave        bool    // Sovereign reprieve — survive one lethal hit
	PetAttackProc    float64
	PetAttackDmg     int
	PetDeflectProc   float64
	PetWhiffProc     float64 // pet distracts enemy → guaranteed miss
	SniperKillProc   float64 // Arina instant-kill
	MistyHealProc    float64
	MistyHealAmt     int
	CrowdRevengeProc float64 // Misty debuff: chance per round of crowd damage
	CrowdRevengeDmg  int     // Misty debuff: damage per proc
	HealItem         int     // consumable: HP restored per heal trigger
	// HealItemCharges is the number of times the heal-at-<50%-HP trigger
	// can fire. 1 = legacy one-shot. Boss fights set this from inventory
	// healing-item count so a stocked-up player can sustain through long
	// fights instead of the heal becoming a single weak trigger.
	HealItemCharges int
	// InitiativeBias adds to the player's initiative roll each round.
	// Used by the DM mood system to tilt who-goes-first: positive favors
	// the player (Effusive mood), negative favors the enemy (Hostile).
	InitiativeBias float64
	WardCharges    int     // consumable: hits fully absorbed
	SporeCloud     int     // consumable: rounds of 15% enemy miss chance
	ReflectNext    float64 // consumable: fraction of next hit reflected
	AutoCritFirst  bool    // consumable: first player hit is auto-crit
	FlatDmgStart   int     // consumable: flat damage to enemy pre-combat

	// D&D race passives (Phase 3 — race traits with combat hooks).
	LuckyReroll  bool // Halfling: reroll the first nat 1 of the fight
	RageReady    bool // Orc: when HP first drops <50%, next attack deals +50% damage
	PoisonResist bool // Dwarf: poison tick damage halved
	FireResist   bool // Tiefling: incoming damage from fire-tagged sources halved

	// Phase 10 SUB2a — subclass combat hooks.
	// CritThreshold: lowest d20 roll that crits. 0 = use default (20).
	// Champion L5 Improved Critical sets this to 19; Champion L15
	// Superior Critical (SUB3) will set 18.
	CritThreshold int
	// BerserkerRage: while true, +RageMeleeDmg flat damage per hit and
	// incoming weapon damage halved (PhysicalResistRage). Frenzy also
	// adds FrenzyDmgBonus on top to model the bonus-attack-per-turn that
	// one-shot combat can't represent literally. Set by armed `rage`
	// ability for Berserker subclass.
	BerserkerRage      bool
	RageMeleeDmg       int     // flat damage per hit while raging (5: +2)
	PhysicalResistRage bool    // halve incoming physical damage while raging
	FrenzyDmgBonus     float64 // multiplicative dmg bump while raging (Frenzy approximation)

	// Phase 10 SUB2a-ii — Battle Master + Assassin.
	// FirstAttackBonus: flat bonus added to the player's first d20 attack
	// roll only (Battle Master Precision Attack ≈ +d8). Consumed on first
	// roll regardless of hit/miss (5e: "before or after rolling").
	// AssassinateAdvantage: re-roll the player's first miss (better of two
	// d20s on the first attack) — proxy for "advantage vs. creatures that
	// haven't acted yet".
	// AssassinateBonusDmg: flat extra damage stacked on the first hit (rides
	// the existing AutoCritFirst doubling — proxy for Death Strike's
	// "crits vs. surprised").
	FirstAttackBonus     int
	AssassinateAdvantage bool
	AssassinateBonusDmg  int

	// Phase 10 SUB3c — Cleric Divine Strike. Flat bonus damage on every
	// weapon hit (5e: "once per turn", which lands ~per round in our model).
	// Damage type (radiant/weapon/poison) varies by Cleric subclass but is
	// not tracked in our engine. Only fires on the weapon-dice damage path —
	// no Weapon → no Divine Strike.
	DivineStrikePerHit int

	// Phase 10 SUB2b — Mage subclasses.
	// ArcaneWardHP: flat HP buffer absorbed before player HP. Refilled at the
	// start of each combat by Abjuration L5+ (2× Mage level, +prof at L7).
	// Persists across rounds within a single combat; not refunded between fights.
	// GrimHarvestSlot/Necrotic: snapshot of the queued spell stashed by
	// applyPendingCast for the post-combat Grim Harvest hook (Necromancy L5+).
	// Heal fires only if the spell event is what dropped the enemy to 0.
	ArcaneWardHP        int
	GrimHarvestSlot     int
	GrimHarvestNecrotic bool

	// Phase 9 — pending spell resolution. Set by applyPendingCast in
	// dnd_spell_combat.go before SimulateCombat runs. SpellPreDamage is
	// dealt as a pre-combat event with SpellPreDamageDesc as the narrative
	// hook (spell name + flavor). SpellEnemySkipFirst causes the enemy to
	// skip its attack on the first round (Hold Person, Sleep, etc.).
	// Buffs (Bless, Mage Armor, Hunter's Mark) are folded into stats/mods
	// directly and don't surface here.
	SpellPreDamage      int
	SpellPreDamageDesc  string
	SpellEnemySkipFirst bool
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
	// D&D layer fields. Set only on events from the d20-vs-AC resolution path.
	// Roll is the raw d20 (1..20); RollAgainst is the target AC.
	Roll        int
	RollAgainst int
}

type CombatResult struct {
	PlayerWon bool
	// TimedOut is true when the fight ran out the phase clock without a
	// kill on either side and was decided by the HP-percentage tiebreak.
	// Callers should treat a timeout loss as a retreat / escape — the
	// player didn't actually take a fatal blow, so character-death side
	// effects (markAdventureDead, respawn timer) should NOT fire.
	TimedOut      bool
	Events        []CombatEvent
	PlayerStartHP int // MaxHP — display denominator
	// PlayerEntryHP is the actual HP the player entered combat with (== MaxHP
	// at full health, less when wounded carry-over via applyDnDHPScaling).
	// Used by injectConsumableEvents so pre-combat narration shows the
	// wounded entry state instead of lying "47/47" on the consumable line.
	PlayerEntryHP int
	EnemyStartHP  int
	EnemyEntryHP  int
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
	Phase      string // "opening", "clash", "decisive", "any"
	ProcChance float64
	// Effect — the mechanic the ability applies. Implemented in applyAbility:
	// stand-in attacks (cleave, lifesteal); riders on the normal attack (poison,
	// enrage, armor_break, stun, bonus_damage, aoe, aoe_fire, death_aoe, execute,
	// self_heal, max_hp_drain); stateful effects armed here and read by the
	// shared resolution primitives (evade, block, advantage, retaliate,
	// regenerate, survive_at_1, stat_drain, debuff, spell_resist, reveal_action,
	// fear_immune, ally_buff). Anything else is a silent no-op.
	Effect string
}

// ── Default Phase Definitions ────────────────────────────────────────────────

// Sudden Death is a fallback phase that runs only when none of the earlier
// phases produced a kill. Adds enough rounds to push total combat length
// to ~10 in the worst case, so the absolute-HP tiebreaker below is rarely
// hit. Players reported the prior 6-round insta-timeout felt arbitrary
// when both sides still had plenty of HP.
var defaultCombatPhases = []CombatPhase{
	{"Opening", 2, 0.6, 0.8, 1.5, 0.15},
	{"Clash", 3, 1.2, 1.0, 0.8, 0.08},
	{"Decisive", 1, 1.0, 0.7, 1.0, 0.05},
	{"Sudden Death", 3, 1.1, 0.6, 1.0, 0.05},
}

var dungeonCombatPhases = []CombatPhase{
	{"Opening", 1, 0.8, 0.8, 1.2, 0.10},
	{"Clash", 2, 1.0, 1.0, 0.8, 0.08},
	{"Decisive", 1, 1.0, 0.7, 1.0, 0.05},
	{"Sudden Death", 4, 1.1, 0.6, 1.0, 0.05},
}

// bossCombatPhases extends the regular dungeon phases for boss encounters.
// Boss HP pools (97–546) are too large for auto-resolve to close in the
// 8-round dungeon budget — players hit a wall and time out. Doubling the
// Sudden Death budget gives sustained pressure a chance to close the gap.
// Real fix is the turn-based-bosses refactor; this is the bridge tuning.
var bossCombatPhases = []CombatPhase{
	{"Opening", 1, 0.8, 0.8, 1.2, 0.10},
	{"Clash", 3, 1.0, 1.0, 0.8, 0.08},
	{"Decisive", 2, 1.0, 0.7, 1.0, 0.05},
	{"Sudden Death", 10, 1.1, 0.6, 1.0, 0.05},
}

// ── Simulation ───────────────────────────────────────────────────────────────

// combatState tracks mutable state during the simulation.
type combatState struct {
	playerHP int
	enemyHP  int

	// Consumable one-shots
	healChargesLeft int // remaining heal-at-<50% triggers
	wardCharges     int
	sporeRounds     int
	reflectFrac     float64
	autoCrit        bool

	// Monster ability effects
	poisonTicks   int
	poisonDmg     int
	stunPlayer    bool
	enraged       bool
	armorBroken   bool
	armorBreakAmt float64

	// Sovereign reprieve
	deathSaveUsed bool

	// D&D race-passive state
	luckyUsed         bool // Halfling Lucky reroll consumed
	raged             bool // Orc Rage already triggered this fight
	pendingRageAttack bool // next player attack gets +50% damage

	// Phase 9 spell — enemy skip-first-attack (consumed on the first round
	// the enemy would otherwise attack).
	enemySkipFirst bool

	// Phase 13 turn-based — pet attack decided once at fight start; the pet
	// strikes once on the player's first acting turn, which clears this.
	petProcReady bool

	// Phase 10 SUB2a-ii first-attack one-shots.
	firstAttackBonusUsed  bool
	assassinateRerollUsed bool
	assassinateBonusUsed  bool

	// Phase 10 SUB2b — Abjuration Arcane Ward HP buffer.
	arcaneWardHP int

	// Phase 13 bestiary slice 3 — stateful monster-ability effects. Each is
	// armed by applyAbility and read by the shared resolution primitives, so
	// both engines honour them; the turn-based engine additionally round-trips
	// them through CombatStatuses so they survive a suspend/resume.
	enemyEvadeNext     bool    // evade: next player weapon attack auto-misses
	enemyBlockUp       bool    // block: enemy holds a parry stance (~50% block on player hits)
	enemyAdvantage     bool    // advantage: enemy rolls its attacks with advantage
	enemyRetaliateFrac float64 // retaliate: fraction of a player hit reflected back
	enemyRegen         int     // regenerate: enemy heals this much each round end
	enemySurviveArmed  bool    // survive_at_1: enemy cheats death once, dropping to 1 HP
	playerAtkDrain     int     // stat_drain: flat reduction to the player's hit damage
	playerACDebuff     int     // debuff: flat reduction to the player's effective AC
	maxHPDrain         int     // max_hp_drain: reduction to the player's effective MaxHP

	// Phase 13 bestiary slice 4 — the former flavor-only placeholders, now
	// backed by real state.
	enemySpellResist bool // spell_resist: player spell damage against this enemy is halved
	enemyRevealNext  bool // reveal_action: the player's next weapon attack is rolled at disadvantage
	enemyFearImmune  bool // fear_immune: player control spells (enemy-skip) fizzle against this enemy
	enemyAtkBuff     int  // ally_buff: flat, accumulating bonus to the enemy's attack damage

	round  int
	events []CombatEvent

	// rng, when non-nil, is the deterministic source the turn-based engine
	// and the timeout reaper seed per session so a fight can be resumed and
	// replayed reproducibly. Auto-resolve (SimulateCombat called without a
	// session) leaves this nil and the engine falls back to the package
	// global — behaviorally identical to the pre-injection code.
	rng *rand.Rand
}

// roll / randFloat draw from the session's injected rng when present,
// else the package global (see rngIntN / rngFloat in dnd_zone_loot.go).
// Auto-resolve leaves st.rng nil — behaviorally identical to the
// pre-injection code; the turn-based engine and the timeout reaper seed
// it per session so a fight can be resumed and replayed reproducibly.
func (st *combatState) roll(n int) int     { return rngIntN(st.rng, n) }
func (st *combatState) randFloat() float64 { return rngFloat(st.rng) }

func SimulateCombat(player, enemy Combatant, phases []CombatPhase) CombatResult {
	return simulateCombatWithRNG(player, enemy, phases, nil)
}

// simulateCombatWithRNG is the deterministic core of the auto-resolve engine.
// SimulateCombat passes nil (package-global rand — production auto-resolve).
// The characterization test and the turn-based engine pass a seeded *rand.Rand
// so a fight is fully reproducible. Passing nil is behaviorally identical to
// the pre-injection code.
func simulateCombatWithRNG(player, enemy Combatant, phases []CombatPhase, rng *rand.Rand) CombatResult {
	playerStart := player.Stats.MaxHP
	if player.Stats.StartHP > 0 && player.Stats.StartHP < player.Stats.MaxHP {
		playerStart = player.Stats.StartHP
	}
	enemyStart := enemy.Stats.MaxHP
	if enemy.Stats.StartHP > 0 && enemy.Stats.StartHP < enemy.Stats.MaxHP {
		enemyStart = enemy.Stats.StartHP
	}
	st := &combatState{
		playerHP:       playerStart,
		enemyHP:        enemyStart,
		wardCharges:    player.Mods.WardCharges,
		sporeRounds:    player.Mods.SporeCloud,
		reflectFrac:    player.Mods.ReflectNext,
		autoCrit:       player.Mods.AutoCritFirst,
		enemySkipFirst: player.Mods.SpellEnemySkipFirst,
		arcaneWardHP:   player.Mods.ArcaneWardHP,
		rng:            rng,
	}
	// HealItemCharges: explicit count overrides legacy one-shot. Backfill
	// to 1 charge if the caller set a HealItem amount but no count.
	st.healChargesLeft = player.Mods.HealItemCharges
	if st.healChargesLeft == 0 && player.Mods.HealItem > 0 {
		st.healChargesLeft = 1
	}

	result := CombatResult{
		PlayerStartHP: player.Stats.MaxHP,
		PlayerEntryHP: playerStart,
		EnemyStartHP:  enemy.Stats.MaxHP,
		EnemyEntryHP:  enemyStart,
	}

	// Pre-combat: Arina sniper check
	if player.Mods.SniperKillProc > 0 && st.randFloat() < player.Mods.SniperKillProc {
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

	// Pre-combat: queued spell. Resolved by applyPendingCast() before this
	// runs — the modifiers carry the resolved damage and narrative hook.
	if player.Mods.SpellPreDamageDesc != "" {
		dmg := player.Mods.SpellPreDamage
		resisted := dmg > 0 && enemyResistsSpells(&enemy, st)
		if resisted {
			dmg = max(1, dmg/2)
		}
		if dmg > 0 {
			st.enemyHP = max(0, st.enemyHP-dmg)
		}
		st.events = append(st.events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "player", Action: "spell_cast",
			Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: player.Mods.SpellPreDamageDesc,
		})
		if resisted {
			st.events = append(st.events, CombatEvent{
				Round: 0, Phase: "pre_combat", Actor: "enemy", Action: "spell_fizzle",
				Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}
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
			roundsThisPhase += st.roll(2) // 0 or +1
		}
		for r := 0; r < roundsThisPhase; r++ {
			st.round++
			if simulateRound(st, &player, &enemy, &phase, &result) {
				return finalize(result, st, player, enemy)
			}
		}
	}

	// If we exhaust all phases without a kill, tiebreak by HP percentage
	// to decide the *outcome* (PlayerWon flag) — but DO NOT zero out HP
	// on the loser. Timeout = retreat, not lethal blow. Caller treats a
	// timeout loss as "fight ended, no character death".
	//
	// Absolute-HP tiebreak (briefly used on the legacy ~120 HP scale) was
	// wrong post HP-unification: monster pools (~30–175) are 2-3× player
	// pools (~13–83), so absolute always favored the larger combatant
	// even when the player took less proportional damage. Slight bias
	// toward the player on exact ties (frac >=).
	result.TimedOut = true
	playerFrac := float64(st.playerHP) / float64(max(1, player.Stats.MaxHP))
	enemyFrac := float64(st.enemyHP) / float64(max(1, enemy.Stats.MaxHP))
	playerWonTiebreak := playerFrac >= enemyFrac
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: "exhaust", Actor: "system", Action: "timeout",
		PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})
	out := finalize(result, st, player, enemy)
	out.PlayerWon = playerWonTiebreak
	return out
}

// simulateRound runs one round. Returns true if combat is over.
func simulateRound(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult) bool {
	phaseName := phase.Name

	// Phase 9 spell: control effect (Hold Person, Sleep, Command) skips the
	// enemy's attack for one round. Logged as a dedicated event so narrative
	// can read it as a held/stunned beat rather than a generic miss.
	enemyHeldThisRound := false
	if st.enemySkipFirst {
		st.enemySkipFirst = false
		if enemyImmuneToControl(enemy, st) {
			// fear_immune: the control spell can't take hold — the enemy acts
			// as normal this round.
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "fear_resist",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		} else {
			enemyHeldThisRound = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "spell_held",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}
	}

	// Monster ability: check at round start
	abilityDealtDamage := enemyHeldThisRound
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
	petWhiff := player.Mods.PetWhiffProc > 0 && st.randFloat() < player.Mods.PetWhiffProc
	// Pet deflect: halves incoming damage to player this round
	petDeflect := player.Mods.PetDeflectProc > 0 && st.randFloat() < player.Mods.PetDeflectProc
	if petDeflect {
		result.PetDeflected = true
	}

	// Spore cloud: enemy has 15% miss chance (decremented when enemy actually attacks)
	sporeMiss := st.sporeRounds > 0 && st.randFloat() < 0.15

	// Determine initiative. DM mood (Effusive/Hostile) biases the player's
	// roll via player.Mods.InitiativeBias — +X means player goes first
	// more often, -X means the enemy does.
	playerSpeed := float64(player.Stats.Speed) * phase.SpeedWeight
	enemySpeed := float64(enemy.Stats.Speed) * phase.SpeedWeight
	playerInit := playerSpeed + st.randFloat()*10 + player.Mods.InitiativeBias
	enemyInit := enemySpeed + st.randFloat()*10
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
	if phase.EnvironmentProc > 0 && st.randFloat() < phase.EnvironmentProc {
		envDmg := 2 + st.roll(5)
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
	if player.Mods.CrowdRevengeProc > 0 && st.randFloat() < player.Mods.CrowdRevengeProc {
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
	if player.Mods.PetAttackProc > 0 && st.randFloat() < player.Mods.PetAttackProc {
		petDmg := player.Mods.PetAttackDmg + st.roll(5)
		st.enemyHP = max(0, st.enemyHP-petDmg)
		result.PetAttacked = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_attack",
			Damage: petDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if enemyDown(st, phaseName) {
			return true
		}
	}

	// Misty heal
	if player.Mods.MistyHealProc > 0 && st.randFloat() < player.Mods.MistyHealProc {
		healAmt := player.Mods.MistyHealAmt
		st.playerHP = min(effPlayerMaxHP(player, st), st.playerHP+healAmt)
		result.MistyHealed = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "npc", Action: "misty_heal",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: "Misty",
		})
	}

	// Consumable heal: triggers when player drops below 60% HP. Fires up
	// to HealItemCharges times per fight (1 = legacy one-shot; bosses can
	// stack from inventory).
	//
	// Threshold is 60% rather than 50% to give low-HP classes (cleric,
	// mage) more breathing room — at 50% a cleric was bleeding into the
	// danger zone before the heal fired.
	if st.healChargesLeft > 0 && player.Mods.HealItem > 0 &&
		st.playerHP > 0 && st.playerHP*5 < player.Stats.MaxHP*3 {
		st.healChargesLeft--
		healAmt := player.Mods.HealItem
		st.playerHP = min(effPlayerMaxHP(player, st), st.playerHP+healAmt)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "heal_item",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	// Regenerate (monster ability): the enemy knits its wounds at the close of
	// every round once the ability has armed st.enemyRegen.
	if st.enemyRegen > 0 && st.enemyHP > 0 && st.enemyHP < enemy.Stats.MaxHP {
		st.enemyHP = min(enemy.Stats.MaxHP, st.enemyHP+st.enemyRegen)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "regen_tick",
			Damage: st.enemyRegen, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	return false
}

// ── Attack Resolution ────────────────────────────────────────────────────────

// resolvePlayerAttack — d20 + AttackBonus vs enemy AC.
// Nat 20 = auto-hit + crit. Nat 1 = auto-miss tagged "fumble".
// Block (on hit) halves damage. autoCrit consumable forces a crit on hit.
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

	// Evade (monster ability): the enemy slipped out of reach — this swing
	// finds nothing. Consumed here, armed by applyAbility's "evade" case.
	if st.enemyEvadeNext {
		st.enemyEvadeNext = false
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "evade",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	// Orc Rage: trigger on the first attack after dropping below 50% HP.
	// Use HP*2 < MaxHP rather than HP < MaxHP/2 so the threshold is exact
	// regardless of MaxHP parity (avoids per-character drift on odd MaxHP).
	if player.Mods.RageReady && !st.raged && st.playerHP > 0 &&
		st.playerHP*2 < player.Stats.MaxHP {
		st.raged = true
		st.pendingRageAttack = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "rage",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Desc: "Orc Rage",
		})
	}

	roll := 1 + st.roll(20)
	// Reveal action (monster ability): the enemy read this swing coming — it's
	// rolled at disadvantage (2d20, keep the lower). One-shot, consumed here.
	if st.enemyRevealNext {
		st.enemyRevealNext = false
		if alt := 1 + st.roll(20); alt < roll {
			roll = alt
		}
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "revealed",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
	// Halfling Lucky: reroll the first nat 1 of the fight.
	if roll == 1 && player.Mods.LuckyReroll && !st.luckyUsed {
		st.luckyUsed = true
		newRoll := 1 + st.roll(20)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "lucky_reroll",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Roll: newRoll, RollAgainst: enemy.Stats.AC, Desc: "Halfling Lucky",
		})
		roll = newRoll
	}
	// Phase 10 SUB2a-ii — Assassin advantage: re-roll the first attack of
	// the fight, take the better of two d20s. Models 5e's "advantage vs.
	// creatures that haven't acted yet" applied to the opening strike only.
	if player.Mods.AssassinateAdvantage && !st.assassinateRerollUsed {
		st.assassinateRerollUsed = true
		alt := 1 + st.roll(20)
		if alt > roll {
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "player", Action: "assassinate_advantage",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
				Roll: alt, RollAgainst: enemy.Stats.AC, Desc: "Assassinate",
			})
			roll = alt
		}
	}
	isFumble := roll == 1
	critFloor := attackCritFloor(player.Mods)
	isCritRoll := roll >= critFloor
	attackBonus := effectiveAttackBonus(player.Stats)
	// Phase 10 SUB2a-ii — Battle Master Precision Attack: +d8 (modeled as
	// flat +4) to the first attack roll only. Consumed even on miss.
	if player.Mods.FirstAttackBonus > 0 && !st.firstAttackBonusUsed {
		attackBonus += player.Mods.FirstAttackBonus
		st.firstAttackBonusUsed = true
	}
	total := roll + attackBonus

	if !attackConnects(roll, total, enemy.Stats.AC, critFloor) {
		desc := ""
		if isFumble {
			desc = "fumble"
		}
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Roll: roll, RollAgainst: enemy.Stats.AC, Desc: desc,
		})
		return false
	}

	// Damage roll: weapon dice path (Phase 8) or legacy penetration formula.
	var dmg int
	if player.Stats.Weapon != nil {
		// Unproficient wielders don't add their ability mod to damage.
		mod := player.Stats.AbilityModForDamage
		if !player.Stats.WeaponProficient {
			mod = 0
		}
		total, _ := rollWeaponDamage(st.rng, player.Stats.Weapon, mod, player.Stats.TwoHandedMode)
		dmg = total
		// Class damage bonus / streak bonus / etc. layered on top via DamageBonus.
		if player.Mods.DamageBonus > 0 {
			dmg = int(float64(dmg) * (1 + player.Mods.DamageBonus))
		}
		// Apply enemy damage reduction (consumables, sets) the same way calcDamage does.
		if enemy.Mods.DamageReduct > 0 && enemy.Mods.DamageReduct != 1.0 {
			dmg = int(float64(dmg) * enemy.Mods.DamageReduct)
		}
	} else {
		dmg = calcDamage(st.rng, player.Stats.Attack, phase.AttackWeight, player.Mods.DamageBonus,
			enemy.Stats.Defense, phase.DefenseWeight, enemy.Mods.DamageReduct)
	}

	// Stat drain (monster ability): the player's strength has been sapped — a
	// flat, accumulating reduction to the damage every hit deals.
	if st.playerAtkDrain > 0 {
		dmg = max(1, dmg-st.playerAtkDrain)
	}

	blocked := enemy.Stats.BlockRate > 0 && st.randFloat() < enemy.Stats.BlockRate
	// Parry stance (monster ability): an enemy holding a block stance rolls a
	// further ~50% chance to halve the hit. Guarded by enemyBlockUp so the
	// extra randFloat is only drawn once the ability has actually fired.
	if !blocked && st.enemyBlockUp && st.randFloat() < 0.5 {
		blocked = true
	}
	if blocked {
		dmg = max(1, dmg/2)
	}

	dmg, action, desc := applyPlayerHitDamageMods(st, player, dmg, isCritRoll, blocked)

	st.enemyHP = max(0, st.enemyHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "player", Action: action,
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		Roll: roll, RollAgainst: enemy.Stats.AC, Desc: desc,
	})

	// Retaliate (monster ability): a damaging aura reflects a fraction of the
	// hit straight back. Resolved per player hit; can drop the player, so this
	// path can return true with the enemy still standing — callers disambiguate
	// the outcome by inspecting HP (see stepPlayerTurn).
	if st.enemyRetaliateFrac > 0 {
		retal := max(1, int(float64(dmg)*st.enemyRetaliateFrac))
		st.playerHP = max(0, st.playerHP-retal)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "retaliate",
			Damage: retal, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 && !trySave(st, player, phaseName) {
			return true
		}
	}
	return enemyDown(st, phaseName)
}

// resolveEnemyAttack — enemy rolls d20 + AttackBonus vs player AC.
// Pet whiff and spore-cloud miss preempt the roll. Ward absorbs a hit fully.
// Block halves damage. Reflect bounces a fraction back. Death save fires if HP hits 0.
func resolveEnemyAttack(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult, petWhiff, petDeflect, sporeMiss bool) bool {
	phaseName := phase.Name

	if st.sporeRounds > 0 {
		st.sporeRounds--
	}

	if petWhiff {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "pet", Action: "pet_whiff",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	if sporeMiss {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "spore_miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}

	roll := 1 + st.roll(20)
	// Advantage (monster ability): the enemy rolls a second d20 and keeps the
	// better. Guarded by enemyAdvantage so the extra roll is only drawn once the
	// ability has fired.
	if st.enemyAdvantage {
		if alt := 1 + st.roll(20); alt > roll {
			roll = alt
		}
	}
	isFumble := roll == 1
	isNat20 := roll == 20
	total := roll + effectiveAttackBonus(enemy.Stats)

	// Debuff (monster ability): a flat, accumulating reduction to the player's
	// effective AC, so the enemy's attacks land more often. Floored at 1.
	// Guarded so an undebuffed player's AC passes through untouched.
	targetAC := player.Stats.AC
	if st.playerACDebuff > 0 {
		targetAC = max(1, player.Stats.AC-st.playerACDebuff)
	}

	if !attackConnects(roll, total, targetAC, 20) {
		desc := ""
		if isFumble {
			desc = "fumble"
		}
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Roll: roll, RollAgainst: targetAC, Desc: desc,
		})
		return false
	}

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
	dmg := calcDamage(st.rng, enemyAttackStat(enemy, st, atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
		playerDefense(player, st), phase.DefenseWeight, player.Mods.DamageReduct)

	blocked := player.Stats.BlockRate > 0 && st.randFloat() < player.Stats.BlockRate
	if blocked {
		dmg = max(1, dmg/2)
	}

	isCrit := isNat20
	if isCrit {
		dmg *= 2
	}
	// Phase 10 SUB2a — Berserker rage halves incoming weapon damage.
	// Applied AFTER crit-doubling so the resistance survives crits.
	if player.Mods.BerserkerRage && player.Mods.PhysicalResistRage {
		dmg = max(1, dmg/2)
	}
	// Tiefling fire resistance — halve the enemy's primary attack when the
	// monster's signature damage is fire (FireAttacker, set on the template).
	// Aligned with the Berserker pattern: applied after crit-doubling so the
	// resistance survives a nat 20.
	if player.Mods.FireResist && enemy.Stats.FireAttacker {
		dmg = max(1, dmg/2)
	}
	dmg = max(1, dmg)

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

	// Phase 10 SUB2b — Abjuration Arcane Ward absorbs incoming damage before
	// it hits player HP. Only wired into the standard enemy attack path; tick
	// effects (poison, environment, lifesteal, cleave) bypass the ward in this
	// model — those represent damage-over-time / tactical hits where the
	// abstraction "magical aegis" reads thinner narratively.
	if st.arcaneWardHP > 0 && dmg > 0 {
		absorbed := dmg
		if absorbed > st.arcaneWardHP {
			absorbed = st.arcaneWardHP
		}
		st.arcaneWardHP -= absorbed
		dmg -= absorbed
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "player", Action: "arcane_ward",
			Damage: absorbed, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: "Arcane Ward",
		})
	}

	st.playerHP = max(0, st.playerHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "enemy", Action: action,
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		Roll: roll, RollAgainst: targetAC,
	})

	if st.reflectFrac > 0 {
		reflected := max(1, int(float64(dmg)*st.reflectFrac))
		st.reflectFrac = 0
		st.enemyHP = max(0, st.enemyHP-reflected)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "reflect_damage",
			Damage: reflected, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if enemyDown(st, phaseName) {
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
	return st.randFloat() < ability.ProcChance
}

func applyAbility(st *combatState, player, enemy *Combatant, phase *CombatPhase, result *CombatResult) bool {
	ab := enemy.Ability
	phaseName := phase.Name

	switch ab.Effect {
	case "poison":
		st.poisonTicks = 2
		st.poisonDmg = 3 + st.roll(3)
		if player.Mods.PoisonResist {
			st.poisonDmg = max(1, st.poisonDmg/2)
		}
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
		dmg := calcDamage(st.rng, enemyAttackStat(enemy, st, atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
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
		dmg1 := calcDamage(st.rng, enemyAttackStat(enemy, st, atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
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

	case "bonus_damage":
		// A single extra strike riding on top of the round's normal attack —
		// the multiattack below is left intact (this is a rider, not a stand-in).
		dmg := abilityHitDamage(st, player, enemy, phase, 0.6, 1.0)
		st.playerHP = max(0, st.playerHP-dmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "bonus_damage",
			Damage: dmg, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			return !trySave(st, player, phaseName)
		}

	case "aoe", "aoe_fire", "death_aoe":
		// An area burst that partly bypasses armor (defWeight cut to 0.4), so a
		// high-defense build still feels it. Rider on top of the normal attack.
		dmg := abilityHitDamage(st, player, enemy, phase, 0.7, 0.4)
		// Tiefling fire resistance: the aoe_fire effect name encodes a fire
		// burst (Burning Hands, Fireball, Fire Breath). Halve for FireResist.
		// Generic "aoe" / "death_aoe" don't always carry fire damage, so we
		// gate on the effect string — the FireAttacker flag on the monster
		// covers fire-themed creatures whose death/aura uses a non-aoe_fire
		// effect (e.g., magmin's death_aoe burst rides their FireAttacker tag
		// via the primary-attack halving path).
		if ab.Effect == "aoe_fire" && player.Mods.FireResist {
			dmg = max(1, dmg/2)
		}
		st.playerHP = max(0, st.playerHP-dmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "aoe",
			Damage: dmg, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			return !trySave(st, player, phaseName)
		}

	case "execute":
		// Finishing blow: lands hard once the player is under 30% HP, a glancing
		// hit otherwise so the ability isn't dead weight at full health.
		mult := 0.5
		if st.playerHP*100 < player.Stats.MaxHP*30 {
			mult = 1.3
		}
		dmg := abilityHitDamage(st, player, enemy, phase, mult, 0.7)
		st.playerHP = max(0, st.playerHP-dmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "execute",
			Damage: dmg, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			return !trySave(st, player, phaseName)
		}

	case "self_heal":
		heal := enemy.Stats.MaxHP/5 + st.roll(max(1, enemy.Stats.MaxHP/10))
		st.enemyHP = min(enemy.Stats.MaxHP, st.enemyHP+heal)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "self_heal",
			Damage: heal, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "evade":
		// The enemy slips out of reach — its next-attacked-against weapon swing
		// finds nothing. The flag is consumed (and the event emitted) by
		// resolvePlayerAttack, so a fight log between here and there reads as the
		// enemy turning evasive and the strike whiffing on the following beat.
		st.enemyEvadeNext = true

	case "block":
		// The enemy settles into a parry stance for the rest of the fight: every
		// player hit from here on rolls a ~50% chance to be halved (on top of any
		// innate BlockRate). resolvePlayerAttack reads st.enemyBlockUp.
		if !st.enemyBlockUp {
			st.enemyBlockUp = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "parry_stance",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "advantage":
		// The enemy gains the upper hand — it rolls its attacks with advantage
		// (best of two d20s) for the rest of the fight. resolveEnemyAttack reads
		// st.enemyAdvantage.
		if !st.enemyAdvantage {
			st.enemyAdvantage = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "advantage",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "retaliate":
		// A damaging aura: from now on a fraction of every player hit is reflected
		// straight back. resolvePlayerAttack applies the reflected damage per hit.
		if st.enemyRetaliateFrac < 0.5 {
			st.enemyRetaliateFrac = 0.5
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "retaliate_aura",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "regenerate":
		// The enemy starts knitting its wounds every round. The actual healing
		// ticks in the round_end phase (turn-based) / round start (auto-resolve);
		// here we just arm it.
		if st.enemyRegen == 0 {
			st.enemyRegen = max(1, enemy.Stats.MaxHP/12)
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "regenerate",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "survive_at_1":
		// The enemy cheats death once: the next blow that would drop it to 0
		// leaves it at 1 HP instead (see enemyDown). Arm it the first time the
		// ability procs; it stays armed until spent.
		if !st.enemySurviveArmed {
			st.enemySurviveArmed = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "survive_armed",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "stat_drain":
		// Saps the player's strength — a flat, accumulating reduction to the
		// damage their hits deal. Capped so a long fight can't zero them out.
		drain := 2 + st.roll(3)
		st.playerAtkDrain = min(12, st.playerAtkDrain+drain)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "stat_drain",
			Damage: drain, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "debuff":
		// Fouls the player's defence — a flat, accumulating reduction to their
		// effective AC, so the enemy's attacks land more often. Capped.
		drain := 1 + st.roll(2)
		st.playerACDebuff = min(6, st.playerACDebuff+drain)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "debuff",
			Damage: drain, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "max_hp_drain":
		// Drains life force: lowers the player's effective MaxHP and deals that
		// much immediate damage (the drain hits current HP too). Effective MaxHP
		// is floored at 1 via the accumulating cap.
		headroom := max(0, player.Stats.MaxHP-1-st.maxHPDrain)
		drain := min(headroom, player.Stats.MaxHP/10+st.roll(max(1, player.Stats.MaxHP/20)))
		st.maxHPDrain += drain
		st.playerHP = max(0, st.playerHP-drain)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "max_hp_drain",
			Damage: drain, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			return !trySave(st, player, phaseName)
		}

	case "spell_resist":
		// Spell Immunity: the enemy is wrapped in anti-magic. Player spell damage
		// (the pre-combat cast in auto-resolve, mid-fight !cast in the turn engine)
		// is halved — read by enemyResistsSpells. Arm once; it's a passive, so a
		// repeat proc is a silent no-op.
		if !st.enemySpellResist {
			st.enemySpellResist = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "spell_resist",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "reveal_action":
		// The enemy reads the player's intent — their next weapon swing comes in
		// at disadvantage (2d20 keep-lower). One-shot, consumed by the next
		// resolvePlayerAttack. Re-armed (and re-announced) on every proc.
		st.enemyRevealNext = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "reveal_armed",
			Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})

	case "fear_immune":
		// The enemy's resolve can't be broken: player control spells (Hold Person,
		// Sleep, Command — the enemy-skip mechanic) fizzle against it. Read by
		// enemyImmuneToControl. Arm once; a passive, so a repeat proc is a no-op.
		if !st.enemyFearImmune {
			st.enemyFearImmune = true
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: phaseName, Actor: "enemy", Action: "fear_immune",
				Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		}

	case "ally_buff":
		// The enemy rallies — a flat, accumulating bonus to the damage its attacks
		// deal (read by enemyAttackStat). Capped so a long fight can't run away.
		buff := 2 + st.roll(3)
		st.enemyAtkBuff = min(15, st.enemyAtkBuff+buff)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "ally_buff",
			Damage: buff, Desc: ab.Name, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}

	return false
}

// abilityHitDamage resolves a monster-ability damage rider through the shared
// penetration formula (calcDamage), so a rider scales identically to a normal
// hit. atkFrac scales the enemy's Attack stat; defFrac scales how much of the
// player's Defense applies (1.0 = a normal hit, lower = an armor-piercing
// burst). Enrage's 1.5x attack multiplier is honored, matching cleave/lifesteal.
func abilityHitDamage(st *combatState, player, enemy *Combatant, phase *CombatPhase, atkFrac, defFrac float64) int {
	mult := atkFrac
	if st.enraged {
		mult *= 1.5
	}
	dmg := calcDamage(st.rng, enemyAttackStat(enemy, st, mult), phase.AttackWeight, enemy.Mods.DamageBonus,
		playerDefense(player, st), phase.DefenseWeight*defFrac, player.Mods.DamageReduct)
	return max(1, dmg)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// calcDamage uses a penetration model: defense provides diminishing returns
// reduction rather than flat subtraction, so damage is never fully negated.
// Formula: rawAtk * reduction where reduction = K / (K + effectiveDef).
// K=40 means 40 defense halves incoming damage; 80 defense reduces by 67%.
//
// A ±15% per-hit jitter is applied so successive hits in the same phase don't
// produce identical numbers — the previous flat-damage output read as scripted.
func calcDamage(rng *rand.Rand, attack int, atkWeight, dmgBonus float64, defense int, defWeight, dmgReduct float64) int {
	const K = 40.0
	rawAtk := float64(attack) * atkWeight * (1 + dmgBonus)
	effectiveDef := float64(defense) * defWeight * dmgReduct
	reduction := K / (K + effectiveDef)
	dmg := rawAtk * reduction
	jitter := 0.85 + rngFloat(rng)*0.30 // 0.85 .. 1.15
	dmg *= jitter
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

// enemyAttackStat is the enemy's effective Attack for a damage roll: the base
// stat scaled by mult (1.0 = normal, 1.5 = enraged, <1 = a partial-power rider)
// plus any accumulated ally_buff bonus. With no buff (every characterization
// scenario) it collapses to the pre-slice-4 expression.
func enemyAttackStat(enemy *Combatant, st *combatState, mult float64) int {
	return max(1, int(float64(enemy.Stats.Attack)*mult)+st.enemyAtkBuff)
}

// enemyResistsSpells reports whether the enemy's spell_resist (Spell Immunity)
// ability is in play. It's a passive — the per-round proc may not have armed
// st.enemySpellResist yet, so the ability profile is checked too, letting the
// pre-combat spell and a round-1 mid-fight cast be resisted all the same.
func enemyResistsSpells(enemy *Combatant, st *combatState) bool {
	return st.enemySpellResist || (enemy.Ability != nil && enemy.Ability.Effect == "spell_resist")
}

// enemyImmuneToControl reports whether the enemy's fear_immune ability is in
// play. Like spell_resist it's a passive, so the ability profile is checked
// alongside the armed flag.
func enemyImmuneToControl(enemy *Combatant, st *combatState) bool {
	return st.enemyFearImmune || (enemy.Ability != nil && enemy.Ability.Effect == "fear_immune")
}

// effPlayerMaxHP is the player's MaxHP after any max_hp_drain monster ability,
// floored at 1. Heal clamps use this so a drained player can't be topped back
// up past the lowered ceiling. With no drain (every characterization scenario)
// it returns player.Stats.MaxHP unchanged.
func effPlayerMaxHP(player *Combatant, st *combatState) int {
	return max(1, player.Stats.MaxHP-st.maxHPDrain)
}

// enemyDown reports whether the enemy is actually dead. It is the single choke
// point every "did this drop the enemy" check routes through, so the
// survive_at_1 ability can cheat death exactly once: an armed enemy at/below 0
// HP is restored to 1, the flag is spent, and the fight continues. With no
// ability armed (every auto-resolve characterization scenario), this is a plain
// st.enemyHP <= 0 with no side effects.
func enemyDown(st *combatState, phaseName string) bool {
	if st.enemyHP > 0 {
		return false
	}
	if st.enemySurviveArmed {
		st.enemySurviveArmed = false
		st.enemyHP = 1
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "survive_at_1",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		return false
	}
	return true
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
