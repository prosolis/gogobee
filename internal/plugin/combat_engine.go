package plugin

import "math/rand/v2"

// ── Core Types ───────────────────────────────────────────────────────────────

type CombatStats struct {
	MaxHP     int
	// StartHP is the HP the combatant *enters* this fight at. Zero means
	// "use MaxHP" (full health). Wounded carry-over uses this so MaxHP
	// stays stable across fights — display reads "100/123", not "100/100".
	StartHP   int
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
	HealItem         int    // consumable: HP restored per heal trigger
	// HealItemCharges is the number of times the heal-at-<50%-HP trigger
	// can fire. 1 = legacy one-shot. Boss fights set this from inventory
	// healing-item count so a stocked-up player can sustain through long
	// fights instead of the heal becoming a single weak trigger.
	HealItemCharges  int
	// InitiativeBias adds to the player's initiative roll each round.
	// Used by the DM mood system to tilt who-goes-first: positive favors
	// the player (Effusive mood), negative favors the enemy (Hostile).
	InitiativeBias   float64
	WardCharges      int    // consumable: hits fully absorbed
	SporeCloud     int     // consumable: rounds of 15% enemy miss chance
	ReflectNext    float64 // consumable: fraction of next hit reflected
	AutoCritFirst  bool    // consumable: first player hit is auto-crit
	FlatDmgStart   int     // consumable: flat damage to enemy pre-combat

	// D&D race passives (Phase 3 — race traits with combat hooks).
	LuckyReroll  bool // Halfling: reroll the first nat 1 of the fight
	RageReady    bool // Orc: when HP first drops <50%, next attack deals +50% damage
	PoisonResist bool // Dwarf: poison tick damage halved

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
	PlayerWon     bool
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
	Phase      string  // "opening", "clash", "decisive", "any"
	ProcChance float64
	Effect     string  // "poison", "enrage", "armor_break", "stun", "lifesteal", "cleave"
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

	// D&D race-passive state
	luckyUsed         bool // Halfling Lucky reroll consumed
	raged             bool // Orc Rage already triggered this fight
	pendingRageAttack bool // next player attack gets +50% damage

	// Phase 9 spell — enemy skip-first-attack (consumed on the first round
	// the enemy would otherwise attack).
	enemySkipFirst bool

	// Phase 10 SUB2a-ii first-attack one-shots.
	firstAttackBonusUsed  bool
	assassinateRerollUsed bool
	assassinateBonusUsed  bool

	// Phase 10 SUB2b — Abjuration Arcane Ward HP buffer.
	arcaneWardHP int

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
	playerStart := player.Stats.MaxHP
	if player.Stats.StartHP > 0 && player.Stats.StartHP < player.Stats.MaxHP {
		playerStart = player.Stats.StartHP
	}
	enemyStart := enemy.Stats.MaxHP
	if enemy.Stats.StartHP > 0 && enemy.Stats.StartHP < enemy.Stats.MaxHP {
		enemyStart = enemy.Stats.StartHP
	}
	st := &combatState{
		playerHP:    playerStart,
		enemyHP:     enemyStart,
		wardCharges:    player.Mods.WardCharges,
		sporeRounds:    player.Mods.SporeCloud,
		reflectFrac:    player.Mods.ReflectNext,
		autoCrit:       player.Mods.AutoCritFirst,
		enemySkipFirst: player.Mods.SpellEnemySkipFirst,
		arcaneWardHP:   player.Mods.ArcaneWardHP,
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
		if dmg > 0 {
			st.enemyHP = max(0, st.enemyHP-dmg)
		}
		st.events = append(st.events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "player", Action: "spell_cast",
			Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Desc: player.Mods.SpellPreDamageDesc,
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
		enemyHeldThisRound = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "spell_held",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
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
		if st.enemyHP <= 0 {
			return true
		}
	}

	// Misty heal
	if player.Mods.MistyHealProc > 0 && st.randFloat() < player.Mods.MistyHealProc {
		healAmt := player.Mods.MistyHealAmt
		st.playerHP = min(player.Stats.MaxHP, st.playerHP+healAmt)
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
		st.playerHP = min(player.Stats.MaxHP, st.playerHP+healAmt)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "consumable", Action: "heal_item",
			Damage: healAmt, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
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
	// Phase 10 SUB2a — Champion Improved Critical lowers the crit floor.
	// CritThreshold==0 means "use default" (nat 20). Champion sets 19.
	critFloor := 20
	if player.Mods.CritThreshold > 0 && player.Mods.CritThreshold < 20 {
		critFloor = player.Mods.CritThreshold
	}
	isCritRoll := roll >= critFloor
	// Class proficiency penalty (appendix §8): -4 attack with a non-proficient weapon.
	attackBonus := player.Stats.AttackBonus
	if player.Stats.Weapon != nil && !player.Stats.WeaponProficient {
		attackBonus -= 4
	}
	// Phase 10 SUB2a-ii — Battle Master Precision Attack: +d8 (modeled as
	// flat +4) to the first attack roll only. Consumed even on miss.
	if player.Mods.FirstAttackBonus > 0 && !st.firstAttackBonusUsed {
		attackBonus += player.Mods.FirstAttackBonus
		st.firstAttackBonusUsed = true
	}
	total := roll + attackBonus

	if isFumble || (!isCritRoll && total < enemy.Stats.AC) {
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

	blocked := enemy.Stats.BlockRate > 0 && st.randFloat() < enemy.Stats.BlockRate
	if blocked {
		dmg = max(1, dmg/2)
	}

	isCrit := isCritRoll
	autoCritFired := false
	if !isCritRoll && st.autoCrit {
		isCrit = true
		autoCritFired = true
		st.autoCrit = false
	} else if st.autoCrit && isCritRoll {
		// Natural crit consumes the auto-crit charge but doesn't get the
		// "passive fired" flavor — the dice already explain it.
		st.autoCrit = false
	}
	if isCrit {
		// Crit: double damage. (5e rolls extra dice; we double total to
		// match the engine's pre-Phase-8 crit semantics.)
		dmg *= 2
	}
	// Orc Rage: +50% damage on this attack, then consume.
	if st.pendingRageAttack {
		dmg = int(float64(dmg) * 1.5)
		st.pendingRageAttack = false
	}
	// Phase 10 SUB2a — Berserker rage: +flat per hit, plus Frenzy
	// multiplicative bump that approximates the bonus-attack-per-turn we
	// can't model in one-shot combat.
	if player.Mods.BerserkerRage {
		if player.Mods.RageMeleeDmg > 0 {
			dmg += player.Mods.RageMeleeDmg
		}
		if player.Mods.FrenzyDmgBonus > 0 {
			dmg = int(float64(dmg) * (1 + player.Mods.FrenzyDmgBonus))
		}
	}
	// Phase 10 SUB3c — Divine Strike. Flat per-hit bonus on weapon hits only
	// (5e specs "weapon hit"; no Weapon means we're on the legacy/non-weapon
	// damage path and Divine Strike doesn't apply). Lands every hit because
	// our 1v1 model has no concept of "once per turn" turn boundaries.
	if player.Mods.DivineStrikePerHit > 0 && player.Stats.Weapon != nil {
		dmg += player.Mods.DivineStrikePerHit
	}
	// Phase 10 SUB2a-ii — Assassin Death Strike proxy: bonus damage on the
	// first hit only. Stacks on top of the Rogue's Sneak Attack auto-crit
	// (which already doubled the base damage above) — the bonus itself is
	// applied AFTER the crit doubling so the math is "double base + flat
	// surprise damage". Consumed on first hit regardless of crit status.
	if player.Mods.AssassinateBonusDmg > 0 && !st.assassinateBonusUsed {
		dmg += player.Mods.AssassinateBonusDmg
		st.assassinateBonusUsed = true
	}
	dmg = max(1, dmg)

	action := "hit"
	desc := ""
	if isCrit {
		action = "crit"
		if autoCritFired {
			desc = "auto_crit"
		}
	} else if blocked {
		action = "block"
	}

	st.enemyHP = max(0, st.enemyHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: phaseName, Actor: "player", Action: action,
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		Roll: roll, RollAgainst: enemy.Stats.AC, Desc: desc,
	})
	return st.enemyHP <= 0
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
	isFumble := roll == 1
	isNat20 := roll == 20
	total := roll + enemy.Stats.AttackBonus

	if isFumble || (!isNat20 && total < player.Stats.AC) {
		desc := ""
		if isFumble {
			desc = "fumble"
		}
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: phaseName, Actor: "enemy", Action: "miss",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			Roll: roll, RollAgainst: player.Stats.AC, Desc: desc,
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
	dmg := calcDamage(st.rng, int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
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
		Roll: roll, RollAgainst: player.Stats.AC,
	})

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
		dmg := calcDamage(st.rng, int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
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
		dmg1 := calcDamage(st.rng, int(float64(enemy.Stats.Attack)*atkMult), phase.AttackWeight, enemy.Mods.DamageBonus,
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
