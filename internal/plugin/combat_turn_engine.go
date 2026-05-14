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
	st := &combatState{
		playerHP:       sess.PlayerHP,
		enemyHP:        sess.EnemyHP,
		round:          sess.Round,
		poisonTicks:    sess.Statuses.PoisonTicks,
		poisonDmg:      sess.Statuses.PoisonDmg,
		stunPlayer:     sess.Statuses.StunPlayer,
		enraged:        sess.Statuses.Enraged,
		armorBroken:    sess.Statuses.ArmorBroken,
		armorBreakAmt:  sess.Statuses.ArmorBreakAmt,
		enemySkipFirst: sess.Statuses.EnemySkipNext,
		// Fight-scoped depleting resources + once-per-fight one-shots: restored
		// from the persisted statuses so a charge or "already used" flag can't
		// reset across a suspend/resume. commit writes the updated values back.
		wardCharges:           sess.Statuses.WardCharges,
		sporeRounds:           sess.Statuses.SporeRounds,
		reflectFrac:           sess.Statuses.ReflectFrac,
		autoCrit:              sess.Statuses.AutoCritFirst,
		arcaneWardHP:          sess.Statuses.ArcaneWardHP,
		healChargesLeft:       sess.Statuses.HealChargesLeft,
		deathSaveUsed:         sess.Statuses.DeathSaveUsed,
		luckyUsed:             sess.Statuses.LuckyUsed,
		raged:                 sess.Statuses.Raged,
		pendingRageAttack:     sess.Statuses.PendingRage,
		firstAttackBonusUsed:  sess.Statuses.FirstAtkBonusUsed,
		assassinateRerollUsed: sess.Statuses.AssassinateReroll,
		assassinateBonusUsed:  sess.Statuses.AssassinateBonus,
		rng:                   rng,
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
	// Default: weapon attack. resolvePlayerAttack returns true once the
	// enemy is down.
	if resolvePlayerAttack(te.st, te.player, te.enemy, &turnCombatPhase, te.result) {
		te.finish(CombatStatusWon)
		return
	}
	te.sess.Phase = CombatPhaseEnemyTurn
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
	if eff.EnemyDamage > 0 {
		st.enemyHP = max(0, st.enemyHP-eff.EnemyDamage)
	}
	if eff.PlayerHeal > 0 {
		st.playerHP = min(te.sess.PlayerHPMax, st.playerHP+eff.PlayerHeal)
	}
	action := eff.Action
	if action == "" {
		action = "spell_cast"
	}
	st.events = append(st.events, CombatEvent{
		Round: st.round, Phase: turnCombatPhase.Name, Actor: "player", Action: action,
		Damage: eff.EnemyDamage, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Desc: eff.Label,
	})
	if st.enemyHP <= 0 {
		te.finish(CombatStatusWon)
		return
	}
	if eff.EnemySkip {
		st.enemySkipFirst = true
	}
	te.sess.Phase = CombatPhaseEnemyTurn
}

func (te *turnEngine) stepEnemyTurn() {
	// A control spell cast last phase forfeits the enemy's attack this round.
	if te.st.enemySkipFirst {
		te.st.enemySkipFirst = false
		te.st.events = append(te.st.events, CombatEvent{
			Round: te.st.round, Phase: turnCombatPhase.Name, Actor: "enemy", Action: "spell_held",
			PlayerHP: te.st.playerHP, EnemyHP: te.st.enemyHP,
		})
		te.sess.Phase = CombatPhaseRoundEnd
		return
	}
	// resolveEnemyAttack returns true when the fight is decided — either the
	// player went down without a death save, or a reflect consumable killed
	// the enemy. Disambiguate by inspecting HP.
	decided := resolveEnemyAttack(te.st, te.player, te.enemy, &turnCombatPhase, te.result, false, false, false)
	if te.st.playerHP <= 0 {
		te.finish(CombatStatusLost)
		return
	}
	if decided {
		te.finish(CombatStatusWon)
		return
	}
	te.sess.Phase = CombatPhaseRoundEnd
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
	s.DeathSaveUsed = st.deathSaveUsed
	s.LuckyUsed = st.luckyUsed
	s.Raged = st.raged
	s.PendingRage = st.pendingRageAttack
	s.FirstAtkBonusUsed = st.firstAttackBonusUsed
	s.AssassinateReroll = st.assassinateRerollUsed
	s.AssassinateBonus = st.assassinateBonusUsed
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
