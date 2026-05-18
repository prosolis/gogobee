package plugin

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// CombatStatuses is the serialized between-round effect state for a fight: the
// subset of combatState that genuinely persists round-to-round, plus the
// fight-scoped buffs a mid-fight !cast / !consume layers on. Everything here
// round-trips combatState -> Statuses -> combatState across every engine step,
// so a fight resumes from exact mid-state after a suspend or bot restart.
//
// All fields are scalar by design — CombatStatuses must stay comparable so the
// persistence round-trip can be asserted with ==.
type CombatStatuses struct {
	// Monster-ability effects — the original between-round persistence set.
	PoisonTicks   int     `json:"poison_ticks,omitempty"`
	PoisonDmg     int     `json:"poison_dmg,omitempty"`
	StunPlayer    bool    `json:"stun_player,omitempty"`
	Enraged       bool    `json:"enraged,omitempty"`
	ArmorBroken   bool    `json:"armor_broken,omitempty"`
	ArmorBreakAmt float64 `json:"armor_break_amt,omitempty"`
	// EnemySkipNext carries a control-spell skip (Hold Person, Sleep) from the
	// player_turn phase it was cast in to the enemy_turn phase that consumes it
	// — those phases resolve as separate engine steps with separate in-memory
	// combatState, so the flag must survive the commit between them.
	EnemySkipNext bool `json:"enemy_skip_next,omitempty"`

	// PetProcReady is the per-fight pet-attack outcome. Auto-resolve rolls the
	// pet proc every round; a manual fight can run many rounds, so the roll is
	// decided once at fight start (rollCombatSessionPetProc) and parked here.
	// The pet then lands a single hit on the player's first acting turn, which
	// clears the flag — persisted so a suspend/resume or reaper auto-play sees
	// the same outcome.
	PetProcReady bool `json:"pet_proc_ready,omitempty"`

	// Fight-scoped depleting resources — mirror the combatState charges that
	// genuinely carry round-to-round. Seeded at fight start (Arcane Ward) or by
	// a mid-fight !cast / !consume, restored into combatState on every resume,
	// written back on every commit so a depleting charge can't silently reset
	// when a fight is suspended and resumed.
	WardCharges     int     `json:"ward_charges,omitempty"`
	SporeRounds     int     `json:"spore_rounds,omitempty"`
	ReflectFrac     float64 `json:"reflect_frac,omitempty"`
	AutoCritFirst   bool    `json:"auto_crit_first,omitempty"`
	ArcaneWardHP    int     `json:"arcane_ward_hp,omitempty"`
	HealChargesLeft int     `json:"heal_charges_left,omitempty"`

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
	PlayerAtkDrain     int     `json:"player_atk_drain,omitempty"`
	PlayerACDebuff     int     `json:"player_ac_debuff,omitempty"`
	MaxHPDrain         int     `json:"max_hp_drain,omitempty"`

	// Slice-4 monster-ability effects — the former flavor-only placeholders.
	// EnemyRevealNext is a one-shot; the other three persist for the fight.
	EnemySpellResist bool `json:"enemy_spell_resist,omitempty"`
	EnemyRevealNext  bool `json:"enemy_reveal_next,omitempty"`
	EnemyFearImmune  bool `json:"enemy_fear_immune,omitempty"`
	EnemyAtkBuff     int  `json:"enemy_atk_buff,omitempty"`

	// Persistent stat buffs from mid-fight !cast / !consume, accumulated as
	// deltas against the freshly-rebuilt combatant. applySessionBuffs folds
	// these back onto the player every round; diffTurnBuff produces them.
	// BuffDamageReductMul is multiplicative (0 = none / 1.0 neutral).
	BuffACBonus         int     `json:"buff_ac_bonus,omitempty"`
	BuffAtkBonus        int     `json:"buff_atk_bonus,omitempty"`
	BuffSpeedBonus      int     `json:"buff_speed_bonus,omitempty"`
	BuffPetDmg          int     `json:"buff_pet_dmg,omitempty"`
	BuffCritRate        float64 `json:"buff_crit_rate,omitempty"`
	BuffDamageBonus     float64 `json:"buff_damage_bonus,omitempty"`
	BuffPetProc         float64 `json:"buff_pet_proc,omitempty"`
	BuffDamageReductMul float64 `json:"buff_damage_reduct_mul,omitempty"`
}

// applyBuffDelta folds one resolved buff (the result of a !cast / !consume
// player turn) into the persisted fight-scoped state. Stat components
// accumulate as deltas; depleting resources add to their running counters.
func (s *CombatStatuses) applyBuffDelta(d turnBuffDelta) {
	s.BuffACBonus += d.dAC
	s.BuffAtkBonus += d.dAtk
	s.BuffSpeedBonus += d.dSpeed
	s.BuffPetDmg += d.dPetDmg
	s.BuffCritRate += d.dCrit
	s.BuffDamageBonus += d.dDmgBonus
	s.BuffPetProc += d.dPetProc
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
func seedCombatSessionOneShots(s *CombatSession, playerMods CombatModifiers) bool {
	st := &s.Statuses
	st.WardCharges = playerMods.WardCharges
	st.SporeRounds = playerMods.SporeCloud
	st.ReflectFrac = playerMods.ReflectNext
	st.AutoCritFirst = playerMods.AutoCritFirst
	st.ArcaneWardHP = playerMods.ArcaneWardHP
	st.HealChargesLeft = playerMods.HealItemCharges
	return st.WardCharges != 0 || st.SporeRounds != 0 || st.ReflectFrac != 0 ||
		st.AutoCritFirst || st.ArcaneWardHP != 0 || st.HealChargesLeft != 0
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
}

// IsActive reports whether the fight is still in flight.
func (s *CombatSession) IsActive() bool { return s.Status == CombatStatusActive }

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
	started_at, last_action_at, expires_at`

// newCombatSessionID — 16-char hex token. Same scheme as zone runs / expeditions.
func newCombatSessionID() string {
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
	return s, err
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
	return s, err
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
	return s, err
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
		&s.StartedAt, &s.LastActionAt, &s.ExpiresAt,
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
func saveCombatSession(s *CombatSession) error {
	statusesJSON, _ := json.Marshal(s.Statuses)
	logJSON, _ := json.Marshal(s.TurnLog)
	now := time.Now().UTC()
	if _, err := db.Get().Exec(`
		UPDATE combat_session
		   SET round = ?, phase = ?,
		       player_hp = ?, enemy_hp = ?,
		       statuses_json = ?, turn_log_json = ?,
		       status = ?, last_action_at = ?
		 WHERE session_id = ?`,
		s.Round, s.Phase, s.PlayerHP, s.EnemyHP,
		string(statusesJSON), string(logJSON), s.Status, now, s.SessionID,
	); err != nil {
		return err
	}
	s.LastActionAt = now
	return nil
}

// listExpiredCombatSessions returns every active session past its expires_at.
// The timeout reaper (reapExpiredCombatSessions) auto-plays each of these to a
// real win/loss from persisted mid-state — per the 2026-05-13 decision, an
// abandoned fight is finished by the engine, not flatly marked as a retreat.
func listExpiredCombatSessions() ([]*CombatSession, error) {
	rows, err := db.Get().Query(`SELECT `+combatSessionCols+`
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
	return out, rows.Err()
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
