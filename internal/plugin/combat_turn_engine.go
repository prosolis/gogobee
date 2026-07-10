package plugin

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
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

// enemySeat is the sentinel roster index the monster occupies in a turn order.
// Player seats are the roster indices 0..len(actors)-1.
const enemySeat = -1

// turnEngine wraps a combatState reconstructed from a persisted CombatSession
// so a single phase can be resolved and then written back.
//
// players is the seated roster in roster order; order is this round's seat
// sequence, which interleaves the roster with enemySeat. A solo fight's order
// is always [0, enemySeat] — the fixed player-then-enemy sequence the engine
// has always run — so no initiative is rolled and its RNG stream is untouched.
type turnEngine struct {
	sess    *CombatSession
	players []*Combatant
	enemy   *Combatant
	order   []int
	st      *combatState
	result  *CombatResult
}

// combatSessionSeed hashes the session id into the base PCG seed.
func combatSessionSeed(sess *CombatSession) uint64 {
	var seed uint64 = 1469598103934665603 // FNV-ish offset basis
	for _, c := range sess.SessionID {
		seed = (seed ^ uint64(c)) * 1099511628211
	}
	return seed
}

// combatSessionRNG seeds a deterministic generator from the session id and the
// phase being resolved, so a fight replays reproducibly no matter how many bot
// restarts occur mid-fight. Each (round, phase) gets a distinct stream — a flat
// per-session seed would correlate the player's and enemy's d20s within a round.
//
// This is the solo form of combatSessionStepRNG, kept as the name the engine's
// single-seat callers and tests use.
func combatSessionRNG(sess *CombatSession) *rand.Rand {
	return combatSessionStepRNG(sess, 0)
}

// combatSessionStepRNG seeds the generator for one step resolved by one seat.
// A round holds one player_turn step per party member, and they all share a
// (round, phase) pair — so the acting seat is mixed into the *seed* to keep
// their d20s independent. Seat 0 and the enemy sentinel mix to nothing, which
// is what makes a solo fight draw exactly the pre-roster stream.
func combatSessionStepRNG(sess *CombatSession, seat int) *rand.Rand {
	seed := combatSessionSeed(sess)
	if seat > 0 {
		seed ^= uint64(seat) * 0x9E3779B97F4A7C15 // golden-ratio odd multiplier
	}
	stream := uint64(sess.Round)*4 + phaseOrdinal(sess.Phase)
	return rand.New(rand.NewPCG(seed, stream))
}

// combatInitiativeRNG seeds the once-per-round initiative roll. It takes the
// round explicitly because stepRoundEnd must order the round it is opening,
// not the one it is closing, and sess.Round is only advanced by commit.
// A distinct seed mix keeps it clear of every step stream.
func combatInitiativeRNG(sess *CombatSession, round int) *rand.Rand {
	return rand.New(rand.NewPCG(combatSessionSeed(sess)^0xD1B54A32D192ED03, uint64(round)))
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

// turnOrder returns the seat sequence for one round: the player roster and the
// enemy sentinel, highest initiative first. Initiative mirrors the auto-resolve
// engine's formula (speed + d10 + InitiativeBias); ties break toward the lower
// seat, and the enemy loses every tie.
//
// A solo roster short-circuits to the engine's historical fixed order and rolls
// nothing — the turn-based engine has never had initiative, and giving one seat
// a coin-flip on who swings first would be a live balance change.
func turnOrder(sess *CombatSession, round int, players []*Combatant, enemy *Combatant) []int {
	if len(players) <= 1 {
		return []int{0, enemySeat}
	}
	type entry struct {
		seat int
		init float64
	}
	rng := combatInitiativeRNG(sess, round)
	entries := make([]entry, 0, len(players)+1)
	for i, p := range players {
		entries = append(entries, entry{i, float64(p.Stats.Speed) + rngFloat(rng)*10 + p.Mods.InitiativeBias})
	}
	entries = append(entries, entry{enemySeat, float64(enemy.Stats.Speed) + rngFloat(rng)*10})
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.init != b.init {
			return a.init > b.init
		}
		if (a.seat == enemySeat) != (b.seat == enemySeat) {
			return b.seat == enemySeat
		}
		return a.seat < b.seat
	})
	order := make([]int, len(entries))
	for i, e := range entries {
		order[i] = e.seat
	}
	return order
}

// phaseForSeat names the phase under which a seat resolves its turn.
func phaseForSeat(seat int) string {
	if seat == enemySeat {
		return CombatPhaseEnemyTurn
	}
	return CombatPhasePlayerTurn
}

// stepSeat is the seat that resolves the session's current phase: the acting
// player on a player_turn, the enemy sentinel on anything else.
//
// It is the *single* derivation of "whose step is this". advancePartyCombatSession
// needs it before the engine is resumed, because the seat seeds that step's RNG;
// the command layer needs it to tell a member it is not their turn. Both read
// only (sessionID, round, roster), so they agree by construction.
func stepSeat(sess *CombatSession, players []*Combatant, enemy *Combatant) int {
	if sess.Phase != CombatPhasePlayerTurn {
		return enemySeat
	}
	order := turnOrder(sess, sess.Round, players, enemy)
	return order[turnIdxForPhase(order, sess.Statuses.TurnIdx, sess.Phase)]
}

// actingSeat names the player seat currently on the clock. The false return
// means the fight is not waiting on anybody: it is mid enemy-turn, mid
// round-end, or over.
func actingSeat(sess *CombatSession, players []*Combatant, enemy *Combatant) (int, bool) {
	if !sess.IsActive() || sess.Phase != CombatPhasePlayerTurn {
		return 0, false
	}
	return stepSeat(sess, players, enemy), true
}

// turnIdxForPhase reconciles a persisted cursor against the persisted phase.
// They can disagree for exactly one reason: a fight that was in flight when
// TurnIdx was introduced decodes as 0 regardless of whose turn it is. Phase is
// the older, load-bearing field, so it wins; the cursor snaps to the first slot
// of the matching kind. On a solo order ([player, enemy]) that is exact.
func turnIdxForPhase(order []int, idx int, phase string) int {
	if idx >= 0 && idx < len(order) && phaseForSeat(order[idx]) == phase {
		return idx
	}
	for i, seat := range order {
		if phaseForSeat(seat) == phase {
			return i
		}
	}
	return 0
}

// resumeTurnEngine rebuilds the in-memory combatState from a persisted session.
// rng is the deterministic source for this step (see combatSessionStepRNG).
//
// restoreActor rebuilds one seat from its persisted state: live HP, the pool
// ceiling, and the per-character effect set. Depleting resources and the
// once-per-fight "already used" flags are restored rather than rearmed, so a
// charge can't silently reset across a suspend/resume or a reaper auto-play.
// snapshotActor is its exact inverse — keep the two field lists in step.
func restoreActor(c *Combatant, hp, hpMax int, s ActorStatuses) *actor {
	return &actor{
		c:        c,
		playerHP: hp,
		hpMax:    hpMax,

		poisonTicks: s.PoisonTicks,
		poisonDmg:   s.PoisonDmg,
		stunPlayer:  s.StunPlayer,

		wardCharges:     s.WardCharges,
		sporeRounds:     s.SporeRounds,
		reflectFrac:     s.ReflectFrac,
		autoCrit:        s.AutoCritFirst,
		arcaneWardHP:    s.ArcaneWardHP,
		healChargesLeft: s.HealChargesLeft,

		concentrationDmg: s.ConcentrationDmg,

		deathSaveUsed:         s.DeathSaveUsed,
		luckyUsed:             s.LuckyUsed,
		raged:                 s.Raged,
		pendingRageAttack:     s.PendingRage,
		firstAttackBonusUsed:  s.FirstAtkBonusUsed,
		assassinateRerollUsed: s.AssassinateReroll,
		assassinateBonusUsed:  s.AssassinateBonus,

		// Enemy debuffs stacked onto this character specifically.
		playerAtkDrain: s.PlayerAtkDrain,
		playerACDebuff: s.PlayerACDebuff,
		maxHPDrain:     s.MaxHPDrain,
	}
}

// snapshotActor folds one seat's live state back into its persisted form. The
// Buff* deltas are owned by the command layer (a !cast / !consume folds them in
// via applyBuffDelta) and are not combatState fields, so they are carried over
// from the prior snapshot rather than re-derived.
func snapshotActor(a *actor, prior ActorStatuses) ActorStatuses {
	s := prior
	s.PoisonTicks = a.poisonTicks
	s.PoisonDmg = a.poisonDmg
	s.StunPlayer = a.stunPlayer

	s.WardCharges = a.wardCharges
	s.SporeRounds = a.sporeRounds
	s.ReflectFrac = a.reflectFrac
	s.AutoCritFirst = a.autoCrit
	s.ArcaneWardHP = a.arcaneWardHP
	s.HealChargesLeft = a.healChargesLeft

	s.ConcentrationDmg = a.concentrationDmg

	s.DeathSaveUsed = a.deathSaveUsed
	s.LuckyUsed = a.luckyUsed
	s.Raged = a.raged
	s.PendingRage = a.pendingRageAttack
	s.FirstAtkBonusUsed = a.firstAttackBonusUsed
	s.AssassinateReroll = a.assassinateRerollUsed
	s.AssassinateBonus = a.assassinateBonusUsed

	s.PlayerAtkDrain = a.playerAtkDrain
	s.PlayerACDebuff = a.playerACDebuff
	s.MaxHPDrain = a.maxHPDrain
	return s
}

// Every seat restores its own persisted per-character statuses: seat 0 from the
// session row's embedded ActorStatuses, seats 1+ from their combat_participant
// row. Without that, a party member's once-per-fight one-shots would rearm on
// every engine step. players[i] must be the combatant for seat i — the caller
// owns that ordering, and it must match sess.Participants[i-1].UserID.
func resumeTurnEngine(sess *CombatSession, players []*Combatant, enemy *Combatant, rng *rand.Rand) *turnEngine {
	seat0 := restoreActor(players[0], sess.PlayerHP, sess.PlayerHPMax, sess.Statuses.ActorStatuses)
	actors := make([]*actor, len(players))
	actors[0] = seat0
	for i := 1; i < len(players); i++ {
		// A roster longer than the persisted seats can only happen if a caller
		// hands us combatants the session never seated. Open those fresh rather
		// than panicking mid-fight; hydrateCombatParticipants already rejects
		// the reverse (a session claiming more seats than it persisted).
		if i-1 >= len(sess.Participants) {
			actors[i] = newActor(players[i])
			continue
		}
		p := sess.Participants[i-1]
		actors[i] = restoreActor(players[i], p.HP, p.HPMax, p.Statuses)
	}
	st := &combatState{
		actor:          seat0,
		actors:         actors,
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
	order := turnOrder(sess, sess.Round, players, enemy)
	sess.Statuses.TurnIdx = turnIdxForPhase(order, sess.Statuses.TurnIdx, sess.Phase)
	if sess.Phase == CombatPhasePlayerTurn {
		st.seat(order[sess.Statuses.TurnIdx])
	}
	return &turnEngine{
		sess:    sess,
		players: players,
		enemy:   enemy,
		order:   order,
		st:      st,
		result:  &CombatResult{},
	}
}

// advance moves the round cursor to the next seat, or to round_end once every
// seat has acted.
func (te *turnEngine) advance() {
	te.sess.Statuses.TurnIdx++
	if te.sess.Statuses.TurnIdx >= len(te.order) {
		te.sess.Phase = CombatPhaseRoundEnd
		return
	}
	te.sess.Phase = phaseForSeat(te.order[te.sess.Statuses.TurnIdx])
}

// step resolves exactly one phase of the fight and advances sess.Phase. The
// events generated this step are returned (also accumulated by commit into
// sess.TurnLog). It does not persist — call commit then saveCombatSession, or
// use advanceCombatSession which does both.
// stampSeat tags every event appended since mark with the roster seat it is
// about, so a party's play-by-play can name the right character. The primitives
// in combat_primitives.go emit against the cursor and know nothing of seats, so
// the tag is applied here, once per phase step, rather than at ~20 append sites.
//
// Solo stamps seat 0 onto seat-0 events — a no-op that omitempty erases.
func (te *turnEngine) stampSeat(mark, seat int) {
	for i := mark; i < len(te.st.events); i++ {
		te.st.events[i].Seat = seat
	}
}

func (te *turnEngine) step(action PlayerAction) ([]CombatEvent, error) {
	if !te.sess.IsActive() {
		return nil, errCombatSessionOver
	}
	te.st.events = nil
	switch te.sess.Phase {
	case CombatPhasePlayerTurn:
		// The cursor was seated on the acting player by resumeTurnEngine, and
		// stepPlayerTurn never moves it, so every event this phase emits — the
		// swings, the pet, the spirit weapon, the retaliate that kills them —
		// belongs to that seat.
		acting := te.st.seatIdx
		te.stepPlayerTurn(action)
		te.stampSeat(0, acting)
	case CombatPhaseEnemyTurn:
		te.stepEnemyTurn()
		// stepEnemyTurn seats its target before anything resolves, and the whole
		// phase lands on that one character.
		te.stampSeat(0, te.st.seatIdx)
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
	// A seat that went down earlier this round forfeits its turn silently.
	// Unreachable while the roster is solo — a downed solo player has already
	// ended the fight.
	if te.st.playerHP <= 0 {
		te.advance()
		return
	}
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
	// Returns true once the swing decided something — usually the enemy is
	// down, but a retaliate aura can drop the swinger on their own attack. A
	// downed swinger only ends the fight if nobody else is standing.
	// The outcome is read off HP rather than the bool: resolvePlayerSwings also
	// returns false when a retaliate aura kills the swinger between extra
	// attacks, and the old code walked that corpse into the enemy's turn.
	resolvePlayerSwings(te.st, te.st.c, te.enemy, &turnCombatPhase, te.result)
	if te.st.enemyHP <= 0 {
		te.finish(CombatStatusWon)
		return
	}
	if !te.st.anyAlive() {
		te.finish(CombatStatusLost)
		return
	}
	// A swinger who went down to retaliate gets no bonus-action follow-ups.
	// Solo never reaches here dead — anyAlive above would have ended the fight.
	if te.st.playerHP > 0 {
		if te.petStrike() {
			te.finish(CombatStatusWon)
			return
		}
		if te.spiritWeaponStrike() {
			te.finish(CombatStatusWon)
			return
		}
	}
	te.advance()
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
	if st.c.Mods.PetAttackProc <= 0 || st.randFloat() >= st.c.Mods.PetAttackProc {
		return false
	}
	petDmg := st.c.Mods.PetAttackDmg + st.roll(5)
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
	if st.c.Mods.SpiritWeaponProc <= 0 || st.randFloat() >= st.c.Mods.SpiritWeaponProc {
		return false
	}
	dmg := st.c.Mods.SpiritWeaponDmg + st.roll(5)
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
		te.advance()
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
		hpCap := max(1, st.hpMax-st.maxHPDrain)
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
	te.advance()
}

// enemyTarget picks the seat the monster swings at this turn: uniformly among
// the standing roster. A solo roster draws nothing, so its RNG stream keeps the
// pre-roster shape. Returns false when the whole roster is down.
func (te *turnEngine) enemyTarget() (int, bool) { return enemyTargetSeat(te.st) }

func (te *turnEngine) stepEnemyTurn() {
	// Seat the target before anything reads the cursor: the ability path, the
	// pet procs, and the swing loop all resolve against whoever is targeted.
	target, ok := te.enemyTarget()
	if !ok {
		te.finish(CombatStatusLost)
		return
	}
	te.st.seat(target)
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
			te.advance()
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
		if applyAbility(te.st, te.st.c, te.enemy, &turnCombatPhase, te.result) {
			// The target went down without a save. The fight only ends if it
			// took the last member with it; otherwise the enemy has spent its
			// turn on that kill.
			if !te.st.anyAlive() {
				te.finish(CombatStatusLost)
			} else {
				te.advance()
			}
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
		petWhiff := te.st.c.Mods.PetWhiffProc > 0 && te.st.randFloat() < te.st.c.Mods.PetWhiffProc
		petDeflect := te.st.c.Mods.PetDeflectProc > 0 && te.st.randFloat() < te.st.c.Mods.PetDeflectProc
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
			decided := resolveEnemyAttack(te.st, te.st.c, &swing, &turnCombatPhase, te.result, swingWhiff, swingDeflect, false)
			if te.st.playerHP <= 0 {
				// The target is down. The enemy stops swinging at a corpse; the
				// fight ends only if the roster is empty.
				if !te.st.anyAlive() {
					te.finish(CombatStatusLost)
					return
				}
				break
			}
			if decided || te.st.enemyHP <= 0 {
				te.finish(CombatStatusWon)
				return
			}
		}
	}
	te.advance()
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
// Per-actor effects (poison, concentration) tick once per seat, in seating
// order; the enemy's regen is fight-scoped and ticks once. A solo roster walks
// each loop exactly once, so it draws the same RNG the pre-roster engine did.
func (te *turnEngine) stepRoundEnd() {
	st := te.st
	for i := range st.actors {
		st.seat(i)
		if st.poisonTicks <= 0 {
			continue
		}
		mark := len(st.events)
		st.playerHP = max(0, st.playerHP-st.poisonDmg)
		st.poisonTicks--
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "poison_tick",
			Damage: st.poisonDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
		if st.playerHP <= 0 && !trySave(st, st.c, CombatPhaseRoundEnd) && !st.anyAlive() {
			te.stampSeat(mark, i)
			te.finish(CombatStatusLost)
			return
		}
		te.stampSeat(mark, i)
	}
	// Concentration aura (Spirit Guardians et al.): the lingering spell bites
	// the enemy each round it stays up. Concentration is per-caster, so every
	// seat holding one pulses. Ticks before enemy regen so a lethal pulse
	// settles the fight before the enemy knits its wounds back.
	for i := range st.actors {
		st.seat(i)
		if st.concentrationDmg <= 0 || st.enemyHP <= 0 || st.playerHP <= 0 {
			continue
		}
		st.enemyHP = max(0, st.enemyHP-st.concentrationDmg)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "player", Action: "concentration_tick",
			Damage: st.concentrationDmg, PlayerHP: st.playerHP, EnemyHP: st.enemyHP, Seat: i,
		})
		if st.enemyHP <= 0 {
			te.finish(CombatStatusWon)
			return
		}
	}
	st.seat(0)
	// Regenerate (monster ability): the enemy knits its wounds at round end.
	if st.enemyRegen > 0 && st.enemyHP > 0 && st.enemyHP < te.enemy.Stats.MaxHP {
		st.enemyHP = min(te.enemy.Stats.MaxHP, st.enemyHP+st.enemyRegen)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "regen_tick",
			Damage: st.enemyRegen, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
	st.round++
	// Initiative is re-rolled each round, so the next round's order is derived
	// here — off st.round, since commit has not yet pushed it onto the session.
	te.order = turnOrder(te.sess, st.round, te.players, te.enemy)
	te.sess.Statuses.TurnIdx = 0
	te.sess.Phase = phaseForSeat(te.order[0])
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
// rebuilt combatant by applySessionBuffs — so snapshotActor carries them over
// from the prior state rather than re-deriving them.
//
// Per-actor state is read off each seat by index, never off the cursor: the
// enemy turn parks the cursor on its target and round_end walks it across the
// roster, so whoever it points at when commit runs is an accident of the phase.
// Seat 0 lands on the session row; seats 1+ on their participant rows.
func (te *turnEngine) commit() {
	st := te.st
	te.sess.Round = st.round
	te.sess.EnemyHP = st.enemyHP

	seat0 := st.actors[0]
	te.sess.PlayerHP = seat0.playerHP
	s := &te.sess.Statuses
	s.ActorStatuses = snapshotActor(seat0, s.ActorStatuses)

	for i := 1; i < len(st.actors) && i-1 < len(te.sess.Participants); i++ {
		p := &te.sess.Participants[i-1]
		p.HP = st.actors[i].playerHP
		p.Statuses = snapshotActor(st.actors[i], p.Statuses)
	}

	// Fight-scoped: the enemy's own stance, and the debuffs it wears.
	s.Enraged = st.enraged
	s.ArmorBroken = st.armorBroken
	s.ArmorBreakAmt = st.armorBreakAmt
	s.EnemySkipNext = st.enemySkipFirst
	s.EnemyEvadeNext = st.enemyEvadeNext
	s.EnemyBlockUp = st.enemyBlockUp
	s.EnemyAdvantage = st.enemyAdvantage
	s.EnemyRetaliateFrac = st.enemyRetaliateFrac
	s.EnemyRegen = st.enemyRegen
	s.EnemySurviveArmed = st.enemySurviveArmed
	s.EnemySpellResist = st.enemySpellResist
	s.EnemyRevealNext = st.enemyRevealNext
	s.EnemyFearImmune = st.enemyFearImmune
	s.EnemyAtkBuff = st.enemyAtkBuff

	te.sess.TurnLog = append(te.sess.TurnLog, st.events...)
}

// advanceCombatSession resolves one phase of a solo fight and persists the
// result. It is the single entry point callers (commands, reaper) should use.
func advanceCombatSession(sess *CombatSession, player, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	return advancePartyCombatSession(sess, []*Combatant{player}, enemy, action)
}

// advancePartyCombatSession resolves one phase of the fight for the seated
// roster and persists the result: it derives the acting seat from this round's
// turn order, seeds that seat's deterministic RNG, resumes the engine, steps,
// commits, and saves. The passed sess is mutated in place. The events generated
// this step are returned.
func advancePartyCombatSession(sess *CombatSession, players []*Combatant, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	if sess == nil {
		return nil, ErrNoActiveCombatSession
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("combat session %s: empty roster", sess.SessionID)
	}
	// The seat is needed before the engine is resumed, because it seeds the
	// step's RNG — so the order is derived once here and once inside
	// resumeTurnEngine. Both derivations read only (sessionID, round, roster),
	// so they agree by construction.
	rng := combatSessionStepRNG(sess, stepSeat(sess, players, enemy))
	te := resumeTurnEngine(sess, players, enemy, rng)
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
