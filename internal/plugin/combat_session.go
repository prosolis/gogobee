package plugin

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// CombatStatuses is the serialized between-round effect state for a fight —
// the subset of combatState fields that genuinely persist round-to-round
// (monster-ability effects). Fight-scoped one-shots (rage, Lucky reroll,
// ward/heal charges) are deliberately not carried here: they reset if a fight
// is resumed across a bot restart. Extending coverage to those one-shots is
// the command-wiring PR's job, once it reconstructs Combatants from a session.
type CombatStatuses struct {
	PoisonTicks   int     `json:"poison_ticks,omitempty"`
	PoisonDmg     int     `json:"poison_dmg,omitempty"`
	StunPlayer    bool    `json:"stun_player,omitempty"`
	Enraged       bool    `json:"enraged,omitempty"`
	ArmorBroken   bool    `json:"armor_broken,omitempty"`
	ArmorBreakAmt float64 `json:"armor_break_amt,omitempty"`
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

// sweepExpiredCombatSessions is the timeout reaper. It marks every active
// session past its expires_at as 'expired' (a terminal status alongside
// won/lost/fled) and parks it in the 'over' phase. Auto-playing an abandoned
// fight to a real win/loss needs Combatant reconstruction, which lands with
// the command-wiring PR; until then 'expired' is the safe terminal outcome —
// no fatal-blow side effects, treated like a retreat. Returns the sweep count.
func sweepExpiredCombatSessions() (int, error) {
	res, err := db.Get().Exec(`
		UPDATE combat_session
		   SET status = 'expired',
		       phase = 'over',
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE status = 'active'
		   AND expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
