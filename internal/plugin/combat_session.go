package plugin

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 13 — Turn-based combat session persistence.
//
// One persisted CombatSession per manual elite/boss fight. The schema
// (combat_session in db.go) was added in the prior commit; this layer adds
// the struct, accessors, and the timeout reaper, mirroring the dnd_expedition
// persistence shape. The intra-round phase state machine that drives a fight
// across these rows lives in combat_turn_engine.go.

// Combat session lifecycle status (combat_session.status).
const (
	CombatStatusActive  = "active"
	CombatStatusWon     = "won"
	CombatStatusLost    = "lost"
	CombatStatusFled    = "fled"
	CombatStatusExpired = "expired"
)

// Intra-round phase state-machine positions (combat_session.phase).
const (
	CombatPhasePlayerTurn = "player_turn"
	CombatPhaseEnemyTurn  = "enemy_turn"
	CombatPhaseRoundEnd   = "round_end"
	CombatPhaseOver       = "over"
)

// combatSessionTTL is how long a session may sit idle before the timeout
// reaper sweeps it. Matches the started_at + 1h note in the schema.
const combatSessionTTL = time.Hour

// ActorStatuses is the per-seat half of the persisted effect state: everything
// that belongs to one character rather than to the fight. It mirrors the fields
// of `actor` (combat_engine.go) that must survive a suspend/resume.
//
// The split follows P2's rule. A debuff the enemy stacked onto one character
// (stat_drain / debuff / max_hp_drain), that character's concentration, their
// depleting charges, and their once-per-fight one-shots are all per-actor — a
// party of three carries three independent copies. The enemy's own stance and
// the round cursor are fight-scoped and live on CombatStatuses.
//
// Seat 0's copy is embedded in the session row's statuses_json (see
// CombatStatuses); seats 1+ each get a combat_participant row carrying this
// struct alone. That asymmetry is deliberate: a solo fight writes exactly the
// bytes it wrote before parties existed, and no participant rows at all.
//
// All fields are scalar by design — ActorStatuses must stay comparable so the
// persistence round-trip can be asserted with ==.
type ActorStatuses struct {
	// Monster-ability effects landing on this character.
	PoisonTicks int  `json:"poison_ticks,omitempty"`
	PoisonDmg   int  `json:"poison_dmg,omitempty"`
	StunPlayer  bool `json:"stun_player,omitempty"`

	// Depleting resources — mirror the actor charges that genuinely carry
	// round-to-round. Seeded at fight start (Arcane Ward) or by a mid-fight
	// !cast / !consume, restored into the actor on every resume, written back on
	// every commit so a depleting charge can't silently reset when a fight is
	// suspended and resumed.
	WardCharges     int     `json:"ward_charges,omitempty"`
	SporeRounds     int     `json:"spore_rounds,omitempty"`
	ReflectFrac     float64 `json:"reflect_frac,omitempty"`
	AutoCritFirst   bool    `json:"auto_crit_first,omitempty"`
	ArcaneWardHP    int     `json:"arcane_ward_hp,omitempty"`
	HealChargesLeft int     `json:"heal_charges_left,omitempty"`

	// ConcentrationDmg is the per-round damage of this character's active
	// concentration AOE (Spirit Guardians, Spike Growth, Call Lightning,
	// Flaming Sphere…). A one-shot !cast lands its burst the casting round,
	// then this re-ticks the aura at round_end every round until the fight
	// ends or another concentration spell overwrites it — the lingering half
	// of the spell the engine used to drop on the floor, which left clerics
	// and druids with no sustained DPS once their burst landed.
	ConcentrationDmg int `json:"concentration_dmg,omitempty"`

	// ArmedAbility is the id of the active ability this character armed and
	// spent entering the fight (rage, second_wind, …). The resource is already
	// debited and the character disarmed; this is the record that lets every
	// rebuild of an in-flight fight re-apply the ability's mods, and lets the
	// close-out know a rage fired. Empty when nothing was armed.
	ArmedAbility string `json:"armed_ability,omitempty"`

	// GrimHarvestSlot / GrimHarvestNecrotic snapshot the most recent damaging
	// spell this seat cast, for the Necromancy Mage's post-combat kill-heal.
	// The auto-resolve path carries the same pair on CombatModifiers, stashed
	// once by applyPendingCast; the turn-based path can cast every round, so
	// the stash lives here and each damaging cast overwrites it. Whether the
	// heal actually fires is decided at close-out by grimHarvestHeal, which
	// asks whether the *last* spell_cast event is the one that dropped the
	// enemy to 0 — so a stale stash from an earlier, non-lethal cast is inert.
	GrimHarvestSlot     int  `json:"grim_harvest_slot,omitempty"`
	GrimHarvestNecrotic bool `json:"grim_harvest_necrotic,omitempty"`

	// Once-per-fight class/race/subclass one-shots: the "already used" flags.
	// Without persistence these reset every round on resume, letting a Halfling
	// reroll a nat 1 or an Orc rage every single round of a turn-based fight.
	DeathSaveUsed     bool `json:"death_save_used,omitempty"`
	LuckyUsed         bool `json:"lucky_used,omitempty"`
	Raged             bool `json:"raged,omitempty"`
	PendingRage       bool `json:"pending_rage,omitempty"`
	FirstAtkBonusUsed bool `json:"first_atk_bonus_used,omitempty"`
	AssassinateReroll bool `json:"assassinate_reroll_used,omitempty"`
	AssassinateBonus  bool `json:"assassinate_bonus_used,omitempty"`

	// Autopilot latches this seat onto the auto-picker for the rest of the
	// fight, set when its turn deadline lapses once (see partyTurnDeadline).
	// Without the latch an away member taxes the party the full deadline every
	// single round; with it, they cost one wait and then resolve instantly. Any
	// combat command from that member clears it — they typed, so they are back.
	//
	// Like the Buff* deltas it is session-layer state with no combatState
	// counterpart, so snapshotActor carries it over from the prior snapshot
	// rather than re-deriving it. omitempty keeps it off every solo row — a solo
	// fight has no turn deadline, only the 1h session reaper.
	Autopilot bool `json:"autopilot,omitempty"`

	// EngineDriven marks a seat that has NO human behind it and never will: a
	// hired companion today, an NPC ally or a Pete-led expedition tomorrow. It is
	// the answer to "who owns this turn", and it is deliberately NOT the same
	// question as Autopilot.
	//
	// Autopilot means "a human is away"; it is provisional, and a keystroke ends
	// it. EngineDriven means "there is nobody to come back", and nothing clears it.
	// Collapsing the two is a bug with teeth: the expedition autopilot drives a
	// party by dispatching each seat's turn as a command from that seat, so an
	// engine seat's own auto-played move arrived back at beginCombatTurn looking
	// exactly like a player returning to the keyboard, and cleared the very latch
	// that was moving it. The seat then stood in the fight doing nothing for the
	// rest of the fight — while the enemy it had inflated by 15% killed the party.
	//
	// So the property lives on the seat, not on an identity check, and no command
	// path can unset it.
	EngineDriven bool `json:"engine_driven,omitempty"`

	// Debuffs the enemy has stacked onto this character specifically.
	PlayerAtkDrain int `json:"player_atk_drain,omitempty"`
	PlayerACDebuff int `json:"player_ac_debuff,omitempty"`
	MaxHPDrain     int `json:"max_hp_drain,omitempty"`

	// Persistent stat buffs from this character's mid-fight !cast / !consume,
	// accumulated as deltas against their freshly-rebuilt combatant.
	// applySessionBuffs folds these back on every round; diffTurnBuff produces
	// them. BuffDamageReductMul is multiplicative (0 = none / 1.0 neutral).
	BuffACBonus         int     `json:"buff_ac_bonus,omitempty"`
	BuffAtkBonus        int     `json:"buff_atk_bonus,omitempty"`
	BuffSpeedBonus      int     `json:"buff_speed_bonus,omitempty"`
	BuffPetDmg          int     `json:"buff_pet_dmg,omitempty"`
	BuffCritRate        float64 `json:"buff_crit_rate,omitempty"`
	BuffDamageBonus     float64 `json:"buff_damage_bonus,omitempty"`
	BuffPetProc         float64 `json:"buff_pet_proc,omitempty"`
	BuffDamageReductMul float64 `json:"buff_damage_reduct_mul,omitempty"`
	BuffSpiritProc      float64 `json:"buff_spirit_proc,omitempty"`
	BuffSpiritDmg       int     `json:"buff_spirit_dmg,omitempty"`
}

// CombatStatuses is the serialized between-round effect state for a fight: the
// fight-scoped half (the enemy's stance, the round cursor) plus seat 0's
// ActorStatuses, embedded. Everything here round-trips combatState -> Statuses
// -> combatState across every engine step, so a fight resumes from exact
// mid-state after a suspend or bot restart.
//
// ActorStatuses is embedded anonymously and untagged, so it flattens into the
// same one-level JSON object the field list used to produce. Rows written
// before the split decode unchanged, and rows written after are byte-compatible
// with a reader that predates it.
//
// All fields are scalar by design — CombatStatuses must stay comparable so the
// persistence round-trip can be asserted with ==.
type CombatStatuses struct {
	// Seat 0's per-character state. Seats 1+ carry their own copy in
	// combat_participant rows.
	ActorStatuses

	Enraged       bool    `json:"enraged,omitempty"`
	ArmorBroken   bool    `json:"armor_broken,omitempty"`
	ArmorBreakAmt float64 `json:"armor_break_amt,omitempty"`
	// EnemySkipNext carries a control-spell skip (Hold Person, Sleep) from the
	// player_turn phase it was cast in to the enemy_turn phase that consumes it
	// — those phases resolve as separate engine steps with separate in-memory
	// combatState, so the flag must survive the commit between them. Holding the
	// enemy holds it for the whole party, hence fight-scoped.
	EnemySkipNext bool `json:"enemy_skip_next,omitempty"`

	// TurnIdx is the position within the round's initiative order (see
	// turnOrder) of whoever acts next. A solo fight's order is the fixed
	// [player, enemy], so TurnIdx mirrors the phase and the field is dead
	// weight — omitempty keeps it out of every solo row. Rows written before
	// this field existed decode as 0 and are reconciled against Phase on
	// resume, so a mid-fight upgrade lands on the right slot.
	TurnIdx int `json:"turn_idx,omitempty"`

	// Slice-3 stateful monster-ability effects — armed by applyAbility, read by
	// the shared resolution primitives, round-tripped through combatState so a
	// suspended/resumed fight (or a reaper auto-play) keeps the same effect
	// state. EnemyEvadeNext is a one-shot; the rest persist for the fight.
	EnemyEvadeNext     bool    `json:"enemy_evade_next,omitempty"`
	EnemyBlockUp       bool    `json:"enemy_block_up,omitempty"`
	EnemyAdvantage     bool    `json:"enemy_advantage,omitempty"`
	EnemyRetaliateFrac float64 `json:"enemy_retaliate_frac,omitempty"`
	EnemyRegen         int     `json:"enemy_regen,omitempty"`
	EnemySurviveArmed  bool    `json:"enemy_survive_armed,omitempty"`

	// Phylactery Verses (Tier-6 postgame, Valdris Ascendant). Unlike the
	// proc-armed EnemySurviveArmed one-shot, this is a *count* of rebirths seeded
	// once at fight start (seedBossRunStatuses) from how many of the zone's secret
	// Verses the player left un-found. Each consumed rebirth (enemyDown) revives
	// the boss to EnemyReviveHP. Both fields are frozen at seed time except
	// EnemyReviveCharges, which decrements as rebirths are spent — so they must
	// round-trip through combatState to survive a suspend/resume. Zero for every
	// other enemy, so omitempty keeps them off every non-Valdris row.
	EnemyReviveCharges int `json:"enemy_revive_charges,omitempty"`
	EnemyReviveHP      int `json:"enemy_revive_hp,omitempty"`

	// Slice-4 monster-ability effects — the former flavor-only placeholders.
	// EnemyRevealNext is a one-shot; the other three persist for the fight.
	EnemySpellResist bool `json:"enemy_spell_resist,omitempty"`
	EnemyRevealNext  bool `json:"enemy_reveal_next,omitempty"`
	EnemyFearImmune  bool `json:"enemy_fear_immune,omitempty"`
	EnemyAtkBuff     int  `json:"enemy_atk_buff,omitempty"`

	// Amendment (Tier-6 postgame, The Custodian of the Last Hour). An in-combat
	// Layer-2 hook resolved at round end (applyBossInCombatRoundEnd), not proc-
	// armed: EnemyRewindHP snapshots the boss's HP at the end of round 3 (0 until
	// captured); when the boss then crosses into phase 2 the hook restores it to
	// that snapshot exactly once and sets EnemyRewindUsed. Both round-trip through
	// combatState so the once-only rewind survives a suspend/resume. Zero/false
	// for every other enemy. The soft midnight timer past round 20 rides the
	// existing EnemyAtkBuff, so it needs no field of its own.
	EnemyRewindHP   int  `json:"enemy_rewind_hp,omitempty"`
	EnemyRewindUsed bool `json:"enemy_rewind_used,omitempty"`

	// Inversion Stitch (Tier-6 postgame, The Seamstress). An in-combat Layer-2
	// hook resolved at round end (applyBossInCombatRoundEnd), live only in the
	// boss's phase 2. InversionActive is the rounds-remaining of the inside-out
	// pulse during which player heals sting instead of mend (gated in
	// stepPlayerActionEffect); InversionTelegraph is the one-round warning before
	// a pulse activates. Both round-trip through combatState so a suspend/resume
	// can't lose or replay a pulse mid-fight. Zero/false for every other enemy.
	InversionActive    int  `json:"inversion_active,omitempty"`
	InversionTelegraph bool `json:"inversion_telegraph,omitempty"`
}

// applyBuffDelta folds one resolved buff (the result of a !cast / !consume
// player turn) into that character's persisted state. Stat components
// accumulate as deltas; depleting resources add to their running counters.
func (s *ActorStatuses) applyBuffDelta(d turnBuffDelta) {
	s.BuffACBonus += d.dAC
	s.BuffAtkBonus += d.dAtk
	s.BuffSpeedBonus += d.dSpeed
	s.BuffPetDmg += d.dPetDmg
	s.BuffCritRate += d.dCrit
	s.BuffDamageBonus += d.dDmgBonus
	s.BuffPetProc += d.dPetProc
	s.BuffSpiritProc += d.dSpiritProc
	s.BuffSpiritDmg += d.dSpiritDmg
	if d.dReductMul > 0 && d.dReductMul != 1 {
		if s.BuffDamageReductMul == 0 {
			s.BuffDamageReductMul = d.dReductMul
		} else {
			s.BuffDamageReductMul *= d.dReductMul
		}
	}
	s.WardCharges += d.ward
	s.SporeRounds += d.spore
	s.ReflectFrac += d.reflect
	if d.autoCrit {
		s.AutoCritFirst = true
	}
	s.ArcaneWardHP += d.arcaneWard
}

// seedCombatSessionOneShots copies the fight-start one-shot resources off the
// freshly-built player combatant into the session's persisted statuses. Only
// the Abjuration Arcane Ward is normally non-zero at fight start — the
// turn-based build deliberately omits pre-combat consumables and queued casts —
// but the full set is seeded for robustness. Returns true if anything was set.
func seedCombatSessionOneShots(s *CombatSession, seat CombatSeatSetup) bool {
	return seedActorOneShots(&s.Statuses.ActorStatuses, seat)
}

// seedActorOneShots copies one character's fight-start one-shot resources onto
// their persisted statuses. Seat 0's live on the session row; a party member's
// live on their participant row, and each seat reads its own combatant's mods —
// a party Abjurer brings their own Arcane Ward, not the leader's.
func seedActorOneShots(st *ActorStatuses, seat CombatSeatSetup) bool {
	playerMods := seat.Mods
	st.WardCharges = playerMods.WardCharges
	st.SporeRounds = playerMods.SporeCloud
	st.ReflectFrac = playerMods.ReflectNext
	st.AutoCritFirst = playerMods.AutoCritFirst
	st.ArcaneWardHP = playerMods.ArcaneWardHP
	st.HealChargesLeft = playerMods.HealItemCharges
	st.ArmedAbility = seat.ArmedAbility
	st.EngineDriven = seat.EngineDriven
	return st.EngineDriven || st.WardCharges != 0 || st.SporeRounds != 0 || st.ReflectFrac != 0 ||
		st.AutoCritFirst || st.ArcaneWardHP != 0 || st.HealChargesLeft != 0 ||
		st.ArmedAbility != ""
}

// CombatParticipant is one party member's seat in a fight, from seat 1 up.
// Seat 0 is the session row itself (UserID / PlayerHP / Statuses.ActorStatuses)
// and never appears here.
type CombatParticipant struct {
	Seat     int
	UserID   string
	HP       int
	HPMax    int
	Statuses ActorStatuses
}

// CombatSession is the in-memory shape of a combat_session row.
type CombatSession struct {
	SessionID    string
	UserID       string
	RunID        string
	EncounterID  string
	EnemyID      string
	Round        int
	Phase        string
	PlayerHP     int
	PlayerHPMax  int
	EnemyHP      int
	EnemyHPMax   int
	Statuses     CombatStatuses
	TurnLog      []CombatEvent
	Status       string
	StartedAt    time.Time
	LastActionAt time.Time
	ExpiresAt    time.Time

	// Participants are the party's seats 1..N-1, ordered by seat and contiguous
	// (loadCombatParticipants enforces both). Empty for a solo fight — the only
	// kind that existed before N3 — so nothing in the solo path allocates or
	// queries for it.
	Participants []CombatParticipant

	// rosterSize mirrors the combat_session.roster_size column as scanned. It
	// exists only so a loader can tell "solo, skip the participant query" from
	// "party, go fetch the seats" in one round trip. In memory Participants is
	// the single source of truth; every write re-derives the column from it.
	rosterSize int
}

// IsActive reports whether the fight is still in flight.
func (s *CombatSession) IsActive() bool { return s.Status == CombatStatusActive }

// RosterSize is the number of seated player characters: seat 0 plus the party.
func (s *CombatSession) RosterSize() int { return 1 + len(s.Participants) }

// IsParty reports whether more than one character is seated.
func (s *CombatSession) IsParty() bool { return len(s.Participants) > 0 }

// SeatUserIDs lists every seated member's user id in seat order.
func (s *CombatSession) SeatUserIDs() []string {
	out := make([]string, 0, s.RosterSize())
	out = append(out, s.UserID)
	for _, p := range s.Participants {
		out = append(out, p.UserID)
	}
	return out
}

// actorStatusesForSeat returns the persisted per-character state for a seat.
// Seat 0 reads through to the session's embedded copy.
func (s *CombatSession) actorStatusesForSeat(seat int) ActorStatuses {
	if seat == 0 {
		return s.Statuses.ActorStatuses
	}
	return s.Participants[seat-1].Statuses
}

// actorStatusesPtr is actorStatusesForSeat for writers. The mid-fight buff path
// (!cast / !consume) folds its delta into the *casting* seat's statuses; before
// P5 it wrote unconditionally to the session's embedded copy, which is seat 0 —
// so a party member's buff would have landed on the leader.
func (s *CombatSession) actorStatusesPtr(seat int) *ActorStatuses {
	if seat == 0 {
		return &s.Statuses.ActorStatuses
	}
	return &s.Participants[seat-1].Statuses
}

// seatHP / seatHPMax read one seat's HP pool. Seat 0's lives on the session row
// (PlayerHP / PlayerHPMax); seats 1+ carry their own on their participant row.
func (s *CombatSession) seatHP(seat int) int {
	if seat == 0 {
		return s.PlayerHP
	}
	return s.Participants[seat-1].HP
}

func (s *CombatSession) seatHPMax(seat int) int {
	if seat == 0 {
		return s.PlayerHPMax
	}
	return s.Participants[seat-1].HPMax
}

// seatUserID names the player sitting at a seat.
func (s *CombatSession) seatUserID(seat int) string {
	if seat == 0 {
		return s.UserID
	}
	return s.Participants[seat-1].UserID
}

// seatOf locates a player on the roster. The false return is the "you are not in
// this fight" answer every party-aware command needs before it touches a turn.
func (s *CombatSession) seatOf(userID id.UserID) (int, bool) {
	u := string(userID)
	if s.UserID == u {
		return 0, true
	}
	for i, p := range s.Participants {
		if p.UserID == u {
			return i + 1, true
		}
	}
	return 0, false
}

// seatAlive reports whether a seat is still standing. A downed seat forfeits its
// turn silently rather than blocking the round on a corpse.
func (s *CombatSession) seatAlive(seat int) bool { return s.seatHP(seat) > 0 }

// seatIsEngineDriven reports whether a seat has no human behind it at all — a
// hired companion, an NPC ally. Permanent for the life of the fight; no command
// clears it. Contrast seatIsAutopiloted, which is a human who stepped away.
func (s *CombatSession) seatIsEngineDriven(seat int) bool {
	return s.actorStatusesForSeat(seat).EngineDriven
}

// seatIsAutopiloted reports whether the engine, rather than a person, decides
// this seat's action — because its human is away (a lapsed turn deadline) or
// because it never had one.
//
// Every driver in the codebase reads this, which is exactly why the two reasons
// share one accessor: the picker does not care WHY nobody is typing. What differs
// is who is allowed to take the wheel back, and that question is asked in
// beginCombatTurn against seatIsEngineDriven — never here.
func (s *CombatSession) seatIsAutopiloted(seat int) bool {
	st := s.actorStatusesForSeat(seat)
	return st.Autopilot || st.EngineDriven
}

// seatNeedsNoHuman reports whether the engine can resolve a seat's turn without
// waiting on its player: it is down (forfeits silently), latched onto autopilot,
// or has no human to wait for. driveCombatRound keeps stepping while this holds,
// so a round only comes to rest on a live human's turn.
func (s *CombatSession) seatNeedsNoHuman(seat int) bool {
	return !s.seatAlive(seat) || s.seatIsAutopiloted(seat)
}

// Errors returned by the combat session layer.
var (
	ErrCombatSessionAlreadyActive = errors.New("combat session already active for player")
	ErrNoActiveCombatSession      = errors.New("no active combat session for player")
)

// combatSessionCols is the shared SELECT column list, kept in sync with
// scanCombatSession's Scan order.
const combatSessionCols = `
	session_id, user_id, run_id, encounter_id, enemy_id,
	round, phase, player_hp, player_hp_max, enemy_hp, enemy_hp_max,
	statuses_json, turn_log_json, status,
	started_at, last_action_at, expires_at, roster_size`

// newCombatSessionID — 16-char hex token. Same scheme as zone runs / expeditions.
func newCombatSessionID() string {
	if simSeedOn() {
		return simHexToken()
	}
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// Vanishingly unlikely; fall through with a zeroed prefix.
	}
	return hex.EncodeToString(b[:])
}

// startCombatSession opens a new manual fight for the player. The caller has
// already resolved the encounter and built the enemy/player HP pools. At most
// one row per user may be 'active' — enforced here, not by a DB constraint.
func startCombatSession(userID id.UserID, runID, encounterID, enemyID string, playerHP, playerHPMax, enemyHP, enemyHPMax int) (*CombatSession, error) {
	existing, err := getActiveCombatSession(userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCombatSessionAlreadyActive
	}

	now := time.Now().UTC()
	s := &CombatSession{
		SessionID:    newCombatSessionID(),
		UserID:       string(userID),
		RunID:        runID,
		EncounterID:  encounterID,
		EnemyID:      enemyID,
		Round:        1,
		Phase:        CombatPhasePlayerTurn,
		PlayerHP:     playerHP,
		PlayerHPMax:  playerHPMax,
		EnemyHP:      enemyHP,
		EnemyHPMax:   enemyHPMax,
		TurnLog:      []CombatEvent{},
		Status:       CombatStatusActive,
		StartedAt:    now,
		LastActionAt: now,
		ExpiresAt:    now.Add(combatSessionTTL),
	}
	statusesJSON, _ := json.Marshal(s.Statuses)
	logJSON, _ := json.Marshal(s.TurnLog)

	if _, err := db.Get().Exec(`
		INSERT INTO combat_session
			(session_id, user_id, run_id, encounter_id, enemy_id,
			 round, phase, player_hp, player_hp_max, enemy_hp, enemy_hp_max,
			 statuses_json, turn_log_json, status,
			 started_at, last_action_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)`,
		s.SessionID, s.UserID, s.RunID, s.EncounterID, s.EnemyID,
		s.Phase, s.PlayerHP, s.PlayerHPMax, s.EnemyHP, s.EnemyHPMax,
		string(statusesJSON), string(logJSON), now, now, s.ExpiresAt,
	); err != nil {
		return nil, fmt.Errorf("insert combat session: %w", err)
	}
	return s, nil
}

// startPartyCombatSession opens a fight for a seated roster. seats[0] owns the
// session row (and is the expedition's leader); seats 1..N-1 get their own
// combat_participant rows. Each seat carries the HP pool and the fight-start
// one-shot resources (Abjuration's Arcane Ward, …) of its own character.
//
// A one-seat roster is exactly startCombatSession: no participant rows, no
// roster_size bump, and the single unwrapped INSERT the solo path has always
// issued. That is the invariant the whole balance corpus rests on.
func (p *AdventurePlugin) startPartyCombatSession(
	runID, encounterID, enemyID string, enemy *Combatant, seats []CombatSeatSetup,
) (*CombatSession, error) {
	if len(seats) == 0 {
		return nil, fmt.Errorf("start combat session: empty roster")
	}
	// Party-only enemy HP bump (solo roster scales by 1.0). The per-turn rebuild
	// in partyCombatantsForSession applies the identical scalar to the enemy's
	// Stats.MaxHP, so the persisted current HP and the rebuilt max never drift.
	enemyHP := scaledEnemyMaxHP(enemy.Stats.MaxHP, seatSetupWeight(seats))
	owner := seats[0]
	sess, err := startCombatSession(owner.UserID, runID, encounterID, enemyID,
		owner.HP, owner.HPMax, enemyHP, enemyHP)
	if err != nil {
		return nil, err
	}

	// Seat 0's one-shots live on the session row; seeding them is a mutation of
	// sess.Statuses that the save below flushes along with the participants.
	dirty := seedCombatSessionOneShots(sess, owner)

	if len(seats) > 1 {
		ps := make([]CombatParticipant, 0, len(seats)-1)
		for i, s := range seats[1:] {
			var st ActorStatuses
			seedActorOneShots(&st, s)
			ps = append(ps, CombatParticipant{
				Seat: i + 1, UserID: string(s.UserID), HP: s.HP, HPMax: s.HPMax, Statuses: st,
			})
		}
		if err := insertCombatParticipants(sess.SessionID, ps); err != nil {
			return nil, fmt.Errorf("seat party: %w", err)
		}
		sess.Participants = ps
		sess.rosterSize = len(seats)

		// startCombatSession parks every fight on a player_turn, because a solo
		// order is hardcoded [player, enemy] and the player always leads. A party
		// rolls for initiative and the monster can win it, so round 1's phase has
		// to come from the order rather than from that assumption — otherwise the
		// cursor snaps forward to the first player slot on resume and the enemy
		// silently forfeits its opening turn. Round 2+ already derives this in
		// stepRoundEnd.
		order := turnOrder(sess, sess.Round, seatCombatants(seats), enemy)
		sess.Phase = phaseForSeat(order[0])
		sess.Statuses.TurnIdx = 0
		dirty = true
	}

	if dirty {
		if err := saveCombatSession(sess); err != nil {
			return nil, fmt.Errorf("seed combat session: %w", err)
		}
	}
	return sess, nil
}

// CombatSeatSetup is one character's entry into a fight: who they are, the HP
// pool they bring, the modifiers their fight-start one-shots are read off, and
// the built combatant itself — which the initiative roll needs, and which the
// caller threads on to the opening block rather than rebuilding the roster.
type CombatSeatSetup struct {
	UserID id.UserID
	HP     int
	HPMax  int
	Mods   CombatModifiers
	C      *Combatant
	// ArmedAbility is the ability id this seat consumed entering the fight, ""
	// if they armed nothing. It is persisted onto the seat's statuses so every
	// later rebuild can re-apply the ability without re-spending it.
	ArmedAbility string
	// EngineDriven seats this combatant with no human behind it — the hired
	// companion today. It resolves from the opening round rather than after the
	// 3-minute away-player deadline (nobody is coming, so waiting out a deadline
	// would idle the fight and then announce him to the party as absent), and no
	// command can hand the wheel back to a player who does not exist.
	EngineDriven bool
}

// getActiveCombatSession returns the player's in-flight fight, or (nil, nil).
func getActiveCombatSession(userID id.UserID) (*CombatSession, error) {
	row := db.Get().QueryRow(`SELECT `+combatSessionCols+`
		  FROM combat_session
		 WHERE user_id = ? AND status = 'active'
		 ORDER BY started_at DESC
		 LIMIT 1`, string(userID))
	s, err := scanCombatSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, hydrateCombatParticipants(s)
}

// hasActiveCombatSession reports whether the player is currently locked into a
// turn-based elite/boss fight. Daily-cron and periodic-ticker paths consult
// this to avoid firing DMs or mutating run state into a live fight: an active
// session locks the run (no `!zone advance`), so the player can't move it
// forward and a cron path shouldn't either. The session carries a 1h TTL, so
// the collision window is bounded — but real, since a fight can straddle any
// scheduled tick. On a lookup error this returns false (fail-open): a missed
// guard is a confusing DM, not corruption.
func hasActiveCombatSession(userID id.UserID) bool {
	s, err := getActiveCombatSession(userID)
	if err != nil {
		slog.Warn("combat: cron guard failed to check active session", "user", userID, "err", err)
		return false
	}
	return s != nil
}

// getCombatSession fetches by ID regardless of status. Test/admin/reaper use.
func getCombatSession(sessionID string) (*CombatSession, error) {
	row := db.Get().QueryRow(`SELECT `+combatSessionCols+`
		  FROM combat_session WHERE session_id = ?`, sessionID)
	s, err := scanCombatSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, hydrateCombatParticipants(s)
}

// getCombatSessionForEncounter returns the most recent session for a
// (run, encounter) pair regardless of status, or (nil, nil) if none exists.
// The room-resolution path uses it to tell "elite not yet fought" (nil) apart
// from "elite already won" (a terminal session) without an active-only filter.
func getCombatSessionForEncounter(runID, encounterID string) (*CombatSession, error) {
	row := db.Get().QueryRow(`SELECT `+combatSessionCols+`
		  FROM combat_session
		 WHERE run_id = ? AND encounter_id = ?
		 ORDER BY started_at DESC
		 LIMIT 1`, runID, encounterID)
	s, err := scanCombatSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, hydrateCombatParticipants(s)
}

func scanCombatSession(row scanner) (*CombatSession, error) {
	var (
		s            CombatSession
		statusesJSON string
		logJSON      string
	)
	if err := row.Scan(
		&s.SessionID, &s.UserID, &s.RunID, &s.EncounterID, &s.EnemyID,
		&s.Round, &s.Phase, &s.PlayerHP, &s.PlayerHPMax, &s.EnemyHP, &s.EnemyHPMax,
		&statusesJSON, &logJSON, &s.Status,
		&s.StartedAt, &s.LastActionAt, &s.ExpiresAt, &s.rosterSize,
	); err != nil {
		return nil, err
	}
	if statusesJSON != "" {
		if err := json.Unmarshal([]byte(statusesJSON), &s.Statuses); err != nil {
			return nil, fmt.Errorf("decode statuses_json: %w", err)
		}
	}
	if logJSON != "" {
		if err := json.Unmarshal([]byte(logJSON), &s.TurnLog); err != nil {
			return nil, fmt.Errorf("decode turn_log_json: %w", err)
		}
	}
	if s.TurnLog == nil {
		s.TurnLog = []CombatEvent{}
	}
	return &s, nil
}

// saveCombatSession persists the mutable fields after a state-machine step.
// session_id / user_id / run_id / encounter_id / enemy_id / *_hp_max are
// immutable for a fight's lifetime and are not rewritten.
//
// A party fight also writes back its seats, in the same transaction as the
// session row: a crash between the two would leave seat 0 a round ahead of the
// rest of the roster. The solo path keeps its single unwrapped UPDATE — there
// are no seats to desync from, and it is the path the whole balance corpus and
// every pre-N3 fight take.
//
// roster_size is not rewritten here: insertCombatParticipants stamped it at
// fight start and the roster is fixed for the fight's lifetime (a dead member
// keeps their seat at 0 HP).
func saveCombatSession(s *CombatSession) error {
	statusesJSON, _ := json.Marshal(s.Statuses)
	logJSON, _ := json.Marshal(s.TurnLog)
	now := time.Now().UTC()

	if len(s.Participants) == 0 {
		if _, err := db.Get().Exec(saveCombatSessionSQL,
			s.Round, s.Phase, s.PlayerHP, s.EnemyHP,
			string(statusesJSON), string(logJSON), s.Status, now, s.SessionID,
		); err != nil {
			return err
		}
		s.LastActionAt = now
		return nil
	}

	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(saveCombatSessionSQL,
		s.Round, s.Phase, s.PlayerHP, s.EnemyHP,
		string(statusesJSON), string(logJSON), s.Status, now, s.SessionID,
	); err != nil {
		return err
	}
	if err := saveCombatParticipantsTx(tx, s.SessionID, s.Participants); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.LastActionAt = now
	return nil
}

const saveCombatSessionSQL = `
	UPDATE combat_session
	   SET round = ?, phase = ?,
	       player_hp = ?, enemy_hp = ?,
	       statuses_json = ?, turn_log_json = ?,
	       status = ?, last_action_at = ?
	 WHERE session_id = ?`

// listExpiredCombatSessions returns every active session past its expires_at.
// The timeout reaper (reapExpiredCombatSessions) auto-plays each of these to a
// real win/loss from persisted mid-state — per the 2026-05-13 decision, an
// abandoned fight is finished by the engine, not flatly marked as a retreat.
func listExpiredCombatSessions() ([]*CombatSession, error) {
	rows, err := db.Get().Query(`SELECT ` + combatSessionCols + `
		  FROM combat_session
		 WHERE status = 'active'
		   AND expires_at <= CURRENT_TIMESTAMP
		 ORDER BY started_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*CombatSession
	for rows.Next() {
		s, err := scanCombatSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Hydrate only after the cursor is closed: a nested query while iterating
	// can stall on a single-connection pool.
	rows.Close()
	for _, s := range out {
		if err := hydrateCombatParticipants(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ── Party seats (N3/P4) ──────────────────────────────────────────────────────

// hydrateCombatParticipants fills s.Participants for a party session. A solo
// session (roster_size 1, which is every row written before N3) returns
// immediately without touching combat_participant, so the common path stays at
// the single query it has always been.
func hydrateCombatParticipants(s *CombatSession) error {
	if s == nil || s.rosterSize <= 1 {
		return nil
	}
	ps, err := loadCombatParticipants(s.SessionID)
	if err != nil {
		return err
	}
	if got := 1 + len(ps); got != s.rosterSize {
		return fmt.Errorf("combat session %s: roster_size %d but %d seats persisted",
			s.SessionID, s.rosterSize, got)
	}
	s.Participants = ps
	return nil
}

// loadCombatParticipants returns seats 1..N-1 in seat order, verifying that the
// seats are contiguous from 1. The engine indexes the roster positionally, so a
// gap would silently shift every member down a seat — better to fail the load.
func loadCombatParticipants(sessionID string) ([]CombatParticipant, error) {
	rows, err := db.Get().Query(`
		SELECT seat, user_id, hp, hp_max, statuses_json
		  FROM combat_participant
		 WHERE session_id = ?
		 ORDER BY seat ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CombatParticipant
	for rows.Next() {
		var (
			p            CombatParticipant
			statusesJSON string
		)
		if err := rows.Scan(&p.Seat, &p.UserID, &p.HP, &p.HPMax, &statusesJSON); err != nil {
			return nil, err
		}
		if statusesJSON != "" {
			if err := json.Unmarshal([]byte(statusesJSON), &p.Statuses); err != nil {
				return nil, fmt.Errorf("decode participant statuses (seat %d): %w", p.Seat, err)
			}
		}
		if want := len(out) + 1; p.Seat != want {
			return nil, fmt.Errorf("combat session %s: seat %d out of order (want %d)",
				sessionID, p.Seat, want)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// insertCombatParticipants seats the party at fight start and stamps
// roster_size so later loads know to come back for them. Seat 0 is the session
// row's own user and is not written here; callers pass seats 1..N-1.
func insertCombatParticipants(sessionID string, ps []CombatParticipant) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, p := range ps {
		statusesJSON, _ := json.Marshal(p.Statuses)
		if _, err := tx.Exec(`
			INSERT INTO combat_participant (session_id, seat, user_id, hp, hp_max, statuses_json)
			VALUES (?, ?, ?, ?, ?, ?)`,
			sessionID, p.Seat, p.UserID, p.HP, p.HPMax, string(statusesJSON),
		); err != nil {
			return fmt.Errorf("insert participant seat %d: %w", p.Seat, err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE combat_session SET roster_size = ? WHERE session_id = ?`,
		1+len(ps), sessionID,
	); err != nil {
		return fmt.Errorf("stamp roster_size: %w", err)
	}
	return tx.Commit()
}

// saveCombatParticipantsTx writes back the mutable half of every seat after an
// engine step. session_id / seat / user_id / hp_max are immutable for a fight's
// lifetime and are not rewritten. It runs inside the caller's transaction so
// the seats commit together with the session row that indexes them.
func saveCombatParticipantsTx(tx *sql.Tx, sessionID string, ps []CombatParticipant) error {
	for _, p := range ps {
		statusesJSON, _ := json.Marshal(p.Statuses)
		if _, err := tx.Exec(`
			UPDATE combat_participant
			   SET hp = ?, statuses_json = ?
			 WHERE session_id = ? AND seat = ?`,
			p.HP, string(statusesJSON), sessionID, p.Seat,
		); err != nil {
			return fmt.Errorf("save participant seat %d: %w", p.Seat, err)
		}
	}
	return nil
}

// getActiveCombatSessionForMember finds the in-flight fight a player is seated
// in at seat 1+, i.e. the one their own user_id does not key. Seat 0's fight is
// getActiveCombatSession's job; this is the party-member equivalent, and it
// returns (nil, nil) when there is none.
func getActiveCombatSessionForMember(userID id.UserID) (*CombatSession, error) {
	row := db.Get().QueryRow(`SELECT `+prefixCols("cs", combatSessionCols)+`
		  FROM combat_session cs
		  JOIN combat_participant cp ON cp.session_id = cs.session_id
		 WHERE cp.user_id = ? AND cs.status = 'active'
		 ORDER BY cs.started_at DESC
		 LIMIT 1`, string(userID))
	s, err := scanCombatSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, hydrateCombatParticipants(s)
}

// prefixCols qualifies every column in a SELECT list with a table alias, so the
// shared combatSessionCols can be reused inside a join without ambiguity on the
// columns both tables carry (session_id, user_id, statuses_json).
func prefixCols(alias, cols string) string {
	fields := strings.Split(cols, ",")
	for i, f := range fields {
		fields[i] = alias + "." + strings.TrimSpace(f)
	}
	return strings.Join(fields, ", ")
}

// markCombatSessionExpired is the fallback terminal outcome for a stale session
// the reaper cannot auto-play (zone run gone, enemy missing from the bestiary,
// or a runaway fight that won't converge). It parks the row in 'expired'/'over'
// with no fatal-blow side effects — treated like a retreat.
func markCombatSessionExpired(sessionID string) error {
	_, err := db.Get().Exec(`
		UPDATE combat_session
		   SET status = 'expired',
		       phase = 'over',
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE session_id = ? AND status = 'active'`, sessionID)
	return err
}
