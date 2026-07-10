package plugin

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Phase 13 — Turn-based combat state machine.
//
// Where SimulateCombat runs a whole fight in one call, the turn-based engine
// advances a persisted CombatSession one phase at a time:
//
//	player_turn -> enemy_turn -> round_end -> player_turn (next round) -> ...
//
// each `step` resolving exactly one phase so a manual elite/boss fight can be
// suspended and resumed (or reaped) from exact mid-state. Attack resolution
// reuses the shared primitives in combat_primitives.go / combat_engine.go, so
// a hit lands identically to auto-resolve. The round_end phase is where
// between-round status effects (poison, etc.) tick — the deferred poison tick
// from the schema commit lands here, now that the round-loop shape exists.

// errCombatSessionOver is returned by step when the fight has already reached
// a terminal status — callers should not keep advancing it.
var errCombatSessionOver = errors.New("combat session already over")

// turnCombatPhase is the single neutral phase the turn-based engine resolves
// attacks under. Auto-resolve sequences named phases with weight curves; a
// manual duel has no phase clock, so weights are flat 1.0 — neutral for the
// legacy penetration formula (calcDamage) and irrelevant on the weapon-dice
// path. The name surfaces on every CombatEvent the fight emits.
var turnCombatPhase = CombatPhase{
	Name:          "Duel",
	Rounds:        0,
	AttackWeight:  1.0,
	DefenseWeight: 1.0,
	SpeedWeight:   1.0,
}

// Player action kinds accepted by the state machine on a player_turn step.
const (
	ActionAttack  = "attack"
	ActionFlee    = "flee"
	ActionCast    = "cast"
	ActionConsume = "consume"
)

// PlayerAction is the player's choice for a player_turn step. It is ignored on
// enemy_turn / round_end steps (pass the zero value).
type PlayerAction struct {
	Kind string
	// Effect is set for ActionCast / ActionConsume: a pre-resolved outcome
	// handed in by the command handler. The handler owns spell/item lookup,
	// dice rolls, and the resource spend; the engine just applies the HP
	// deltas and emits the event so the round flows on into the enemy turn.
	Effect *turnActionEffect
}

// turnActionEffect is the resolved payload of a !cast / !consume player turn.
// Damage, heal, and the one-round enemy-skip all land within the casting round.
type turnActionEffect struct {
	Label       string // "Fireball — crit!", "Berry Poultice"
	Action      string // CombatEvent.Action: "spell_cast" or "use_consumable"
	EnemyDamage int
	PlayerHeal  int
	EnemySkip   bool // control spell: enemy forfeits its attack this round
	// ConcentrationDmg arms a per-round aura tick when a concentration damage
	// spell is cast: EnemyDamage is the burst that lands this round, this is
	// what re-ticks at every round_end after. Zero for one-shot spells; a
	// non-zero value overwrites any aura already running (5e: one
	// concentration at a time).
	ConcentrationDmg int
}

// turnEngine wraps a combatState reconstructed from a persisted CombatSession
// so a single phase can be resolved and then written back.
type turnEngine struct {
	sess   *CombatSession
	player *Combatant
	enemy  *Combatant
	st     *combatState
	result *CombatResult
}

// combatSessionRNG seeds a deterministic generator from the session id and the
// phase being resolved, so a fight replays reproducibly no matter how many bot
// restarts occur mid-fight. Each (round, phase) gets a distinct stream — a flat
// per-session seed would correlate the player's and enemy's d20s within a round.
func combatSessionRNG(sess *CombatSession) *rand.Rand {
	var seed uint64 = 1469598103934665603 // FNV-ish offset basis
	for _, c := range sess.SessionID {
		seed = (seed ^ uint64(c)) * 1099511628211
	}
	stream := uint64(sess.Round)*4 + phaseOrdinal(sess.Phase)
	return rand.New(rand.NewPCG(seed, stream))
}

func phaseOrdinal(phase string) uint64 {
	switch phase {
	case CombatPhasePlayerTurn:
		return 0
	case CombatPhaseEnemyTurn:
		return 1
	case CombatPhaseRoundEnd:
		return 2
	default: // CombatPhaseOver
		return 3
	}
}

// resumeTurnEngine rebuilds the in-memory combatState from a persisted session.
// rng is the deterministic source for this step (see combatSessionRNG).
func resumeTurnEngine(sess *CombatSession, player, enemy *Combatant, rng *rand.Rand) *turnEngine {
	// The seated character's half of the persisted statuses. A single-seat
	// roster today; P4 gives combat_session per-participant rows and this
	// becomes one actor per member.
	seat0 := &actor{
		c:           player,
		playerHP:    sess.PlayerHP,
		poisonTicks: sess.Statuses.PoisonTicks,
		poisonDmg:   sess.Statuses.PoisonDmg,
		stunPlayer:  sess.Statuses.StunPlayer,
		// Fight-scoped depleting resources + once-per-fight one-shots: restored
		// from the persisted statuses so a charge or "already used" flag can't
		// reset across a suspend/resume. commit writes the updated values back.
		wardCharges:           sess.Statuses.WardCharges,
		sporeRounds:           sess.Statuses.SporeRounds,
		reflectFrac:           sess.Statuses.ReflectFrac,
		autoCrit:              sess.Statuses.AutoCritFirst,
		arcaneWardHP:          sess.Statuses.ArcaneWardHP,
		healChargesLeft:       sess.Statuses.HealChargesLeft,
		concentrationDmg:      sess.Statuses.ConcentrationDmg,
		deathSaveUsed:         sess.Statuses.DeathSaveUsed,
		luckyUsed:             sess.Statuses.LuckyUsed,
		raged:                 sess.Statuses.Raged,
		pendingRageAttack:     sess.Statuses.PendingRage,
		firstAttackBonusUsed:  sess.Statuses.FirstAtkBonusUsed,
		assassinateRerollUsed: sess.Statuses.AssassinateReroll,
		assassinateBonusUsed:  sess.Statuses.AssassinateBonus,
		// Enemy debuffs stacked onto this character specifically.
		playerAtkDrain: sess.Statuses.PlayerAtkDrain,
		playerACDebuff: sess.Statuses.PlayerACDebuff,
		maxHPDrain:     sess.Statuses.MaxHPDrain,
	}
	st := &combatState{
		actor:          seat0,
		actors:         []*actor{seat0},
		enemyHP:        sess.EnemyHP,
		round:          sess.Round,
		enraged:        sess.Statuses.Enraged,
		armorBroken:    sess.Statuses.ArmorBroken,
		armorBreakAmt:  sess.Statuses.ArmorBreakAmt,
		enemySkipFirst: sess.Statuses.EnemySkipNext,
		// Slice-3 stateful monster-ability effects — armed by applyAbility,
		// round-tripped here so they survive a suspend/resume or reaper auto-play.
		enemyEvadeNext:     sess.Statuses.EnemyEvadeNext,
		enemyBlockUp:       sess.Statuses.EnemyBlockUp,
		enemyAdvantage:     sess.Statuses.EnemyAdvantage,
		enemyRetaliateFrac: sess.Statuses.EnemyRetaliateFrac,
		enemyRegen:         sess.Statuses.EnemyRegen,
		enemySurviveArmed:  sess.Statuses.EnemySurviveArmed,
		// Slice-4 monster-ability effects — the former flavor-only placeholders.
		enemySpellResist: sess.Statuses.EnemySpellResist,
		enemyRevealNext:  sess.Statuses.EnemyRevealNext,
		enemyFearImmune:  sess.Statuses.EnemyFearImmune,
		enemyAtkBuff:     sess.Statuses.EnemyAtkBuff,
		rng:              rng,
	}
	return &turnEngine{
		sess:   sess,
		player: player,
		enemy:  enemy,
		st:     st,
		result: &CombatResult{},
	}
}

// step resolves exactly one phase of the fight and advances sess.Phase. The
// events generated this step are returned (also accumulated by commit into
// sess.TurnLog). It does not persist — call commit then saveCombatSession, or
// use advanceCombatSession which does both.
func (te *turnEngine) step(action PlayerAction) ([]CombatEvent, error) {
	if !te.sess.IsActive() {
		return nil, errCombatSessionOver
	}
	te.st.events = nil
	switch te.sess.Phase {
	case CombatPhasePlayerTurn:
		te.stepPlayerTurn(action)
	case CombatPhaseEnemyTurn:
		te.stepEnemyTurn()
	case CombatPhaseRoundEnd:
		te.stepRoundEnd()
	case CombatPhaseOver:
		return nil, errCombatSessionOver
	default:
		return nil, fmt.Errorf("combat session %s in unknown phase %q", te.sess.SessionID, te.sess.Phase)
	}
	return te.st.events, nil
}

func (te *turnEngine) stepPlayerTurn(action PlayerAction) {
	switch action.Kind {
	case ActionFlee:
		te.st.events = append(te.st.events, CombatEvent{
			Round: te.st.round, Phase: turnCombatPhase.Name, Actor: "player", Action: "flee",
			PlayerHP: te.st.playerHP, EnemyHP: te.st.enemyHP,
		})
		te.finish(CombatStatusFled)
		return
	case ActionCast, ActionConsume:
		te.stepPlayerActionEffect(action.Effect)
		return
	}
	// Default: weapon attack. resolvePlayerSwings rolls the base swing plus
	// Mods.ExtraAttacks follow-ups (5e Extra Attack at Fighter L5/L11/L20,
	// Ranger/Paladin L5, Bard College of Valor L7). One !attack press in the
	// turn-based engine == one 5e Attack action == all swings in sequence.
	// Before this, the turn path called single-swing resolvePlayerAttack and
	// Extra Attack only fired in the auto-resolve SimulateCombat path; that
	// left every elite/boss !fight at L11+ short two swings/turn, which the
	// J1 boss-trace surfaced as Fighter's T3+ wall.
	// Returns true once the fight is decided — usually the enemy is down,
	// but a retaliate aura can drop the player on their own swing, so
	// disambiguate the outcome by HP.
	if resolvePlayerSwings(te.st, te.player, te.enemy, &turnCombatPhase, te.result) {
		if te.st.playerHP <= 0 {
			te.finish(CombatStatusLost)
		} else {
			te.finish(CombatStatusWon)
		}
		return
	}
	if te.petStrike() {
		te.finish(CombatStatusWon)
		return
	}
	if te.spiritWeaponStrike() {
		te.finish(CombatStatusWon)
		return
	}
	te.sess.Phase = CombatPhaseEnemyTurn
}

// petStrike resolves the player's pet attack for a turn-based fight. The pet
// rolls fresh on every player-acting turn (PetAttackProc), mirroring the
// auto-resolve engine's per-round chance rather than a once-per-fight strike.
// The roll rides the per-(round,phase) step RNG, so a suspend/resume or reaper
// auto-play of the same turn reproduces the same outcome. Damage reuses the
// auto-resolve formula (PetAttackDmg + d5), and PetAttackDmg already carries any
// mid-fight buff delta via applySessionBuffs. Returns true if the strike dropped
// the enemy.
func (te *turnEngine) petStrike() bool {
	st := te.st
	if te.player.Mods.PetAttackProc <= 0 || st.randFloat() >= te.player.Mods.PetAttackProc {
		return false
	}
	petDmg := te.player.Mods.PetAttackDmg + st.roll(5)
	st.enemyHP = max(0, st.enemyHP-petDmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: turnCombatPhase.Name, Actor: "pet", Action: "pet_attack",
		Damage: petDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})
	return enemyDown(st, turnCombatPhase.Name)
}

// spiritWeaponStrike resolves the spell's bonus-action attack each round when
// the spiritual_weapon buff is active. Same per-turn cadence as petStrike, but
// rolls and narrates on its own channel so the spectral mace doesn't borrow
// pet flavor on a petless caster. Returns true if the strike dropped the enemy.
func (te *turnEngine) spiritWeaponStrike() bool {
	st := te.st
	if te.player.Mods.SpiritWeaponProc <= 0 || st.randFloat() >= te.player.Mods.SpiritWeaponProc {
		return false
	}
	dmg := te.player.Mods.SpiritWeaponDmg + st.roll(5)
	st.enemyHP = max(0, st.enemyHP-dmg)
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: turnCombatPhase.Name, Actor: "spirit_weapon", Action: "spirit_weapon_strike",
		Damage: dmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
	})
	return enemyDown(st, turnCombatPhase.Name)
}

// stepPlayerActionEffect resolves a !cast / !consume turn: the command handler
// has already rolled the spell / picked the item and spent the resource, so the
// engine only applies the HP deltas and emits the event before handing off to
// the enemy turn. A control-spell skip is parked in st.enemySkipFirst, which
// commit() persists as Statuses.EnemySkipNext for the enemy_turn step to read.
func (te *turnEngine) stepPlayerActionEffect(eff *turnActionEffect) {
	st := te.st
	if eff == nil {
		// Defensive: a kind with no payload just passes the turn.
		te.sess.Phase = CombatPhaseEnemyTurn
		return
	}
	action := eff.Action
	if action == "" {
		action = "spell_cast"
	}
	// spell_resist (Spell Immunity): a spell's damage against the enemy is
	// halved. Consumables that happen to deal damage are not spells, so the
	// resist is gated on the spell_cast action.
	enemyDmg := eff.EnemyDamage
	spellFizzled := enemyDmg > 0 && action == "spell_cast" && enemyResistsSpells(te.enemy, st)
	if spellFizzled {
		enemyDmg = max(1, enemyDmg/2)
	}
	if enemyDmg > 0 {
		st.enemyHP = max(0, st.enemyHP-enemyDmg)
	}
	if eff.PlayerHeal > 0 {
		// Respect any max_hp_drain monster ability — a drained player can't be
		// healed back past the lowered ceiling.
		hpCap := max(1, te.sess.PlayerHPMax-st.maxHPDrain)
		st.playerHP = min(hpCap, st.playerHP+eff.PlayerHeal)
	}
	// Arm / replace the concentration aura. A new concentration cast overwrites
	// the old one (5e: one concentration at a time); non-concentration casts
	// leave any running aura alone.
	if eff.ConcentrationDmg > 0 {
		st.concentrationDmg = eff.ConcentrationDmg
	}
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: turnCombatPhase.Name, Actor: "player", Action: action,
		Damage: enemyDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Desc: eff.Label,
	})
	if spellFizzled {
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: turnCombatPhase.Name, Actor: "enemy", Action: "spell_fizzle",
			Damage: enemyDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
	if enemyDown(st, turnCombatPhase.Name) {
		te.finish(CombatStatusWon)
		return
	}
	if te.petStrike() {
		te.finish(CombatStatusWon)
		return
	}
	if te.spiritWeaponStrike() {
		te.finish(CombatStatusWon)
		return
	}
	if eff.EnemySkip {
		// fear_immune enemies shrug off control spells — the skip never arms.
		if enemyImmuneToControl(te.enemy, st) {
			st.events = append(st.events, CombatEvent{
				Round: st.round, Phase: turnCombatPhase.Name, Actor: "enemy", Action: "fear_resist",
				PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
			})
		} else {
			st.enemySkipFirst = true
		}
	}
	te.sess.Phase = CombatPhaseEnemyTurn
}

func (te *turnEngine) stepEnemyTurn() {
	// A control spell cast last phase forfeits the enemy's attack this round —
	// unless the enemy is fear_immune, in which case the control fizzled and it
	// acts as normal.
	if te.st.enemySkipFirst {
		te.st.enemySkipFirst = false
		if enemyImmuneToControl(te.enemy, te.st) {
			te.st.events = append(te.st.events, CombatEvent{
				Round: te.st.round, Phase: turnCombatPhase.Name, Actor: "enemy", Action: "fear_resist",
				PlayerHP: te.st.playerHP, EnemyHP: te.st.enemyHP,
			})
		} else {
			te.st.events = append(te.st.events, CombatEvent{
				Round: te.st.round, Phase: turnCombatPhase.Name, Actor: "enemy", Action: "spell_held",
				PlayerHP: te.st.playerHP, EnemyHP: te.st.enemyHP,
			})
			te.sess.Phase = CombatPhaseRoundEnd
			return
		}
	}

	// Monster ability fires at the top of the enemy turn. cleave / lifesteal
	// resolve their own damage and stand in for the attack action this round;
	// every other effect (poison / stun / enrage / armor_break) is a rider that
	// leaves the multiattack below intact. applyAbility returns true when the
	// player went down without a save.
	abilityDealtDamage := false
	if te.enemy.Ability != nil && turnAbilityFires(te.enemy.Ability, te.st, te.enemy) {
		if applyAbility(te.st, te.player, te.enemy, &turnCombatPhase, te.result) {
			te.finish(CombatStatusLost)
			return
		}
		switch te.enemy.Ability.Effect {
		case "cleave", "lifesteal":
			abilityDealtDamage = true
		}
	}

	if !abilityDealtDamage {
		// Pet defensive procs are a single proc per enemy turn: roll once, then
		// spend it on the first swing only. Whiff makes that one swing a
		// guaranteed miss; deflect halves its damage. Against a multiattack the
		// remaining swings resolve normally — a single proc shouldn't nullify a
		// boss's whole multiattack round. (This deliberately diverges from the
		// auto-resolve engine's apply-to-all model.)
		petWhiff := te.player.Mods.PetWhiffProc > 0 && te.st.randFloat() < te.player.Mods.PetWhiffProc
		petDeflect := te.player.Mods.PetDeflectProc > 0 && te.st.randFloat() < te.player.Mods.PetDeflectProc
		if petDeflect {
			te.result.PetDeflected = true
		}

		// SRD multiattack: each profile entry is one attack roll resolved
		// through the shared primitive. A registered elite/boss swings its full
		// profile; everyone else gets a single attack from the template stats.
		// resolveEnemyAttack returns true when the fight is decided — either the
		// player went down without a death save, or a reflect consumable killed
		// the enemy. Disambiguate by inspecting HP.
		for i, atk := range enemyAttackProfile(te.sess.EnemyID, te.enemy.Stats) {
			swing := *te.enemy
			swing.Stats.Attack = atk.Damage
			swing.Stats.AttackBonus = atk.AttackBonus
			// Spend the proc on the first swing only; later swings see false.
			swingWhiff := petWhiff && i == 0
			swingDeflect := petDeflect && i == 0
			decided := resolveEnemyAttack(te.st, te.player, &swing, &turnCombatPhase, te.result, swingWhiff, swingDeflect, false)
			if te.st.playerHP <= 0 {
				te.finish(CombatStatusLost)
				return
			}
			if decided || te.st.enemyHP <= 0 {
				te.finish(CombatStatusWon)
				return
			}
		}
	}
	te.sess.Phase = CombatPhaseRoundEnd
}

// turnAbilityFires decides whether a monster ability triggers this enemy turn.
// The auto-resolve engine gates abilities on its named phase clock (Opening /
// Clash / Decisive); the turn-based duel has no phase clock, so the phase tag
// is remapped to fight progress: "opening" is round-1 only, "decisive" unlocks
// once the enemy is bloodied (<40% HP), "clash" covers the rounds in between,
// and "any" is always eligible. ProcChance is rolled once eligible.
func turnAbilityFires(ability *MonsterAbility, st *combatState, enemy *Combatant) bool {
	eligible := false
	switch ability.Phase {
	case "any":
		eligible = true
	case "opening":
		eligible = st.round <= 1
	case "clash":
		eligible = st.round > 1
	case "decisive":
		eligible = st.enemyHP < int(float64(enemy.Stats.MaxHP)*0.4)
	}
	if !eligible {
		return false
	}
	return st.randFloat() < ability.ProcChance
}

// stepRoundEnd applies between-round status effects, then opens the next round.
func (te *turnEngine) stepRoundEnd() {
	st := te.st
	if st.poisonTicks > 0 {
		st.playerHP = max(0, st.playerHP-st.poisonDmg)
		st.poisonTicks--
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "poison_tick",
			Damage: st.poisonDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 {
			if !trySave(st, te.player, CombatPhaseRoundEnd) {
				te.finish(CombatStatusLost)
				return
			}
		}
	}
	// Concentration aura (Spirit Guardians et al.): the lingering spell bites
	// the enemy each round it stays up. Ticks before enemy regen so a lethal
	// pulse settles the fight before the enemy knits its wounds back.
	if st.concentrationDmg > 0 && st.enemyHP > 0 {
		st.enemyHP = max(0, st.enemyHP-st.concentrationDmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "player", Action: "concentration_tick",
			Damage: st.concentrationDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.enemyHP <= 0 {
			te.finish(CombatStatusWon)
			return
		}
	}
	// Regenerate (monster ability): the enemy knits its wounds at round end.
	if st.enemyRegen > 0 && st.enemyHP > 0 && st.enemyHP < te.enemy.Stats.MaxHP {
		st.enemyHP = min(te.enemy.Stats.MaxHP, st.enemyHP+st.enemyRegen)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "regen_tick",
			Damage: st.enemyRegen, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
	st.round++
	te.sess.Phase = CombatPhasePlayerTurn
}

// finish parks the session in a terminal status + the 'over' phase.
func (te *turnEngine) finish(status string) {
	te.sess.Status = status
	te.sess.Phase = CombatPhaseOver
}

// commit folds this step's combatState back into the session struct: HP,
// round, the between-round status snapshot, and the appended event log.
// Phase / Status were already set by step. saveCombatSession persists it.
//
// The Buff* stat deltas on Statuses are NOT combatState fields — they're owned
// by the command layer (a !cast / !consume folds them in) and applied to the
// rebuilt combatant by applySessionBuffs — so commit mutates Statuses in place
// rather than replacing it, leaving those deltas untouched.
func (te *turnEngine) commit() {
	st := te.st
	te.sess.Round = st.round
	te.sess.PlayerHP = st.playerHP
	te.sess.EnemyHP = st.enemyHP
	s := &te.sess.Statuses
	s.PoisonTicks = st.poisonTicks
	s.PoisonDmg = st.poisonDmg
	s.StunPlayer = st.stunPlayer
	s.Enraged = st.enraged
	s.ArmorBroken = st.armorBroken
	s.ArmorBreakAmt = st.armorBreakAmt
	s.EnemySkipNext = st.enemySkipFirst
	s.WardCharges = st.wardCharges
	s.SporeRounds = st.sporeRounds
	s.ReflectFrac = st.reflectFrac
	s.AutoCritFirst = st.autoCrit
	s.ArcaneWardHP = st.arcaneWardHP
	s.HealChargesLeft = st.healChargesLeft
	s.ConcentrationDmg = st.concentrationDmg
	s.DeathSaveUsed = st.deathSaveUsed
	s.LuckyUsed = st.luckyUsed
	s.Raged = st.raged
	s.PendingRage = st.pendingRageAttack
	s.FirstAtkBonusUsed = st.firstAttackBonusUsed
	s.AssassinateReroll = st.assassinateRerollUsed
	s.AssassinateBonus = st.assassinateBonusUsed
	s.EnemyEvadeNext = st.enemyEvadeNext
	s.EnemyBlockUp = st.enemyBlockUp
	s.EnemyAdvantage = st.enemyAdvantage
	s.EnemyRetaliateFrac = st.enemyRetaliateFrac
	s.EnemyRegen = st.enemyRegen
	s.EnemySurviveArmed = st.enemySurviveArmed
	s.PlayerAtkDrain = st.playerAtkDrain
	s.PlayerACDebuff = st.playerACDebuff
	s.MaxHPDrain = st.maxHPDrain
	s.EnemySpellResist = st.enemySpellResist
	s.EnemyRevealNext = st.enemyRevealNext
	s.EnemyFearImmune = st.enemyFearImmune
	s.EnemyAtkBuff = st.enemyAtkBuff
	te.sess.TurnLog = append(te.sess.TurnLog, st.events...)
}

// advanceCombatSession resolves one phase of the fight and persists the result.
// It is the single entry point callers (commands, reaper) should use: it seeds
// the deterministic RNG, resumes the engine, steps, commits, and saves. The
// passed sess is mutated in place. The events generated this step are returned.
func advanceCombatSession(sess *CombatSession, player, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	if sess == nil {
		return nil, ErrNoActiveCombatSession
	}
	rng := combatSessionRNG(sess)
	te := resumeTurnEngine(sess, player, enemy, rng)
	events, err := te.step(action)
	if err != nil {
		return nil, err
	}
	te.commit()
	if err := saveCombatSession(sess); err != nil {
		return events, fmt.Errorf("persist combat session %s: %w", sess.SessionID, err)
	}
	return events, nil
}
