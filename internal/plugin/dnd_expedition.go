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

// Phase 12 E1a — Expedition data model and persistence.
// Implements `gogobee_expedition_system.md` §4.4 (Supplies), §5.3 (Camp),
// §8.4 (Threat Clock), and §9 (Expedition State). One persisted Expedition
// per active campaign; players may have at most one row with status='active'
// (enforced in code).
//
// E1a ships the data layer only: structs, schema (in db.go), persistence
// helpers, and a round-trip test. Day cycle (E1d), supply procurement (E1b),
// commands (E1c), and camp interactions (E1e) wire on top in subsequent
// commits. Threat clock events / siege mode are scaffolded but not driven
// until E2.

// Expedition status values.
const (
	ExpeditionStatusActive     = "active"
	ExpeditionStatusExtracting = "extracting"
	ExpeditionStatusComplete   = "complete"
	ExpeditionStatusFailed     = "failed"
	ExpeditionStatusAbandoned  = "abandoned"
)

// ExpeditionSupplies — §4.4.
type ExpeditionSupplies struct {
	Current       float32 `json:"current"`        // current SU
	Max           float32 `json:"max"`            // purchased at outset
	DailyBurn     float32 `json:"daily_burn"`     // base rate for this zone tier
	HarshMod      float32 `json:"harsh_mod"`      // multiplier if harsh conditions active
	ForagedToday  bool    `json:"foraged_today"`  // Ranger forage attempt used today
	PacksStandard int     `json:"packs_standard"` // count purchased (max 3)
	PacksDeluxe   int     `json:"packs_deluxe"`   // count purchased (max 1)
}

// CampState — §5.3.
type CampState struct {
	Active        bool      `json:"active"`
	Type          string    `json:"camp_type"` // rough|standard|fortified|base
	RoomIndex     int       `json:"room_index"`
	EstablishedAt time.Time `json:"established_at"`
	NightEvents   []string  `json:"night_events"`
	// RestApplied is set when the long-rest effects (HP refill, spell slots,
	// threat -5 etc.) have already been applied at pitch time. processOvernightCamp
	// uses it to skip re-applying so the night cycle just breaks the camp.
	RestApplied bool `json:"rest_applied,omitempty"`
	// AutoPitched is set when the long-expedition autopilot pitched this
	// camp. The autorun ticker breaks an auto-pitched camp itself after a
	// minimum dwell so the walk can keep moving; player-pitched camps stay
	// up until the player breaks them (or moves on).
	AutoPitched bool `json:"auto_pitched,omitempty"`
}

// ThreatEvent — §8.4.
type ThreatEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Delta     int       `json:"delta"`
	Reason    string    `json:"reason"`
}

// Expedition — §9. The in-memory shape of a dnd_expedition row.
type Expedition struct {
	ID             string
	UserID         string
	ZoneID         ZoneID
	RunID          string // optional pointer to dnd_zone_run.run_id
	Status         string
	StartDate      time.Time
	CurrentDay     int
	CurrentRegion  string
	BossDefeated   bool
	Supplies       ExpeditionSupplies
	Camp           *CampState
	ThreatLevel    int
	SiegeMode      bool
	ThreatEvents   []ThreatEvent
	TemporalStack  int
	RegionState    map[string]any
	XPEarned       int
	CoinsEarned    int
	DMMood         int
	LastBriefingAt *time.Time
	LastRecapAt    *time.Time
	// LastAmbientKind — the Kind of the most recent ambient event the
	// ticker fired (e.g. "pack_rat", "monologue"). Empty when no ambient
	// has fired yet. Used by pickAmbientEvent for back-to-back anti-repeat.
	LastAmbientKind string
	LastActivity    time.Time
	CompletedAt     *time.Time
}

// IsActive reports whether the expedition is in flight.
func (e *Expedition) IsActive() bool {
	return e.Status == ExpeditionStatusActive || e.Status == ExpeditionStatusExtracting
}

// ExpeditionEntry — one row in dnd_expedition_log; mirrors §9.
type ExpeditionEntry struct {
	EntryID      int64
	ExpeditionID string
	Day          int
	Timestamp    time.Time
	Type         string
	Summary      string
	Flavor       string
}

// Errors returned by the expedition layer.
var (
	ErrExpeditionAlreadyActive = errors.New("expedition already active for player")
	ErrNoActiveExpedition      = errors.New("no active expedition for player")
)

// newExpeditionID — 16-char hex token. Same scheme as zone runs.
func newExpeditionID() string {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// Vanishingly unlikely; fall through with a zeroed prefix.
	}
	return hex.EncodeToString(b[:])
}

// startExpedition creates a new expedition for the player. The caller must
// have already validated zone access and supply purchase totals.
// runID may be empty — the zone run is created lazily by E1c command flow.
func startExpedition(userID id.UserID, zoneID ZoneID, runID string, supplies ExpeditionSupplies) (*Expedition, error) {
	existing, err := getActiveExpedition(userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrExpeditionAlreadyActive
	}

	now := time.Now().UTC()
	// Multi-region zones (§11) start the player in the first region;
	// single-region zones leave CurrentRegion empty.
	currentRegion := ""
	if first, ok := firstRegion(zoneID); ok {
		currentRegion = first.ID
	}
	regionState := map[string]any{}
	if currentRegion != "" {
		regionState[regionStateVisitedKey] = []string{currentRegion}
	}
	startMood := 50
	if isHol, _ := isHolidayToday(); isHol {
		startMood = 55
	}
	exp := &Expedition{
		ID:            newExpeditionID(),
		UserID:        string(userID),
		ZoneID:        zoneID,
		RunID:         runID,
		Status:        ExpeditionStatusActive,
		StartDate:     now,
		CurrentDay:    1,
		CurrentRegion: currentRegion,
		Supplies:      supplies,
		ThreatEvents:  []ThreatEvent{},
		RegionState:   regionState,
		DMMood:        startMood,
		LastActivity:  now,
	}
	supJSON, _ := json.Marshal(supplies)
	regJSON, _ := json.Marshal(exp.RegionState)
	threatJSON, _ := json.Marshal(exp.ThreatEvents)

	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition
			(expedition_id, user_id, zone_id, run_id, status,
			 start_date, current_day, current_region, supplies_json,
			 threat_events, region_state, gm_mood, last_activity)
		VALUES (?, ?, ?, ?, 'active', ?, 1, ?, ?, ?, ?, ?, ?)`,
		exp.ID, exp.UserID, string(zoneID), nullableString(runID),
		now, currentRegion, string(supJSON), string(threatJSON), string(regJSON), startMood, now,
	); err != nil {
		return nil, fmt.Errorf("insert expedition: %w", err)
	}
	return exp, nil
}

// getActiveExpedition returns the player's in-flight expedition, or (nil, nil).
// 'extracting' rows are post-extraction (resumable) — see getResumableExpedition.
func getActiveExpedition(userID id.UserID) (*Expedition, error) {
	row := db.Get().QueryRow(`
		SELECT expedition_id, user_id, zone_id, run_id, status,
		       start_date, current_day, current_region, boss_defeated,
		       supplies_json, camp_json, threat_level, threat_siege,
		       threat_events, temporal_stack, region_state,
		       xp_earned, coins_earned, gm_mood,
		       last_briefing_at, last_recap_at, last_ambient_kind,
		       last_activity, completed_at
		  FROM dnd_expedition
		 WHERE user_id = ?
		   AND status = 'active'
		 ORDER BY start_date DESC
		 LIMIT 1`, string(userID))
	e, err := scanExpedition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// getExpedition fetches by ID regardless of status. Test/admin use.
func getExpedition(id string) (*Expedition, error) {
	row := db.Get().QueryRow(`
		SELECT expedition_id, user_id, zone_id, run_id, status,
		       start_date, current_day, current_region, boss_defeated,
		       supplies_json, camp_json, threat_level, threat_siege,
		       threat_events, temporal_stack, region_state,
		       xp_earned, coins_earned, gm_mood,
		       last_briefing_at, last_recap_at, last_ambient_kind,
		       last_activity, completed_at
		  FROM dnd_expedition WHERE expedition_id = ?`, id)
	e, err := scanExpedition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func scanExpedition(row scanner) (*Expedition, error) {
	var (
		e               Expedition
		zoneID          string
		runID           sql.NullString
		suppliesJSON    string
		campJSON        sql.NullString
		threatJSON      string
		regionJSON      string
		bossI           int
		siegeI          int
		lastBriefingRaw sql.NullTime
		lastRecapRaw    sql.NullTime
		completedRaw    sql.NullTime
	)
	if err := row.Scan(
		&e.ID, &e.UserID, &zoneID, &runID, &e.Status,
		&e.StartDate, &e.CurrentDay, &e.CurrentRegion, &bossI,
		&suppliesJSON, &campJSON, &e.ThreatLevel, &siegeI,
		&threatJSON, &e.TemporalStack, &regionJSON,
		&e.XPEarned, &e.CoinsEarned, &e.DMMood,
		&lastBriefingRaw, &lastRecapRaw, &e.LastAmbientKind,
		&e.LastActivity, &completedRaw,
	); err != nil {
		return nil, err
	}
	e.ZoneID = ZoneID(zoneID)
	if runID.Valid {
		e.RunID = runID.String
	}
	e.BossDefeated = bossI != 0
	e.SiegeMode = siegeI != 0
	if err := json.Unmarshal([]byte(suppliesJSON), &e.Supplies); err != nil {
		return nil, fmt.Errorf("decode supplies_json: %w", err)
	}
	if campJSON.Valid && campJSON.String != "" {
		var c CampState
		if err := json.Unmarshal([]byte(campJSON.String), &c); err != nil {
			return nil, fmt.Errorf("decode camp_json: %w", err)
		}
		e.Camp = &c
	}
	if threatJSON != "" {
		if err := json.Unmarshal([]byte(threatJSON), &e.ThreatEvents); err != nil {
			return nil, fmt.Errorf("decode threat_events: %w", err)
		}
	}
	if e.ThreatEvents == nil {
		e.ThreatEvents = []ThreatEvent{}
	}
	if regionJSON != "" {
		if err := json.Unmarshal([]byte(regionJSON), &e.RegionState); err != nil {
			slog.Warn("expedition: region_state decode failed; falling back to empty",
				"expedition", e.ID, "err", err)
		}
	}
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	if lastBriefingRaw.Valid {
		t := lastBriefingRaw.Time
		e.LastBriefingAt = &t
	}
	if lastRecapRaw.Valid {
		t := lastRecapRaw.Time
		e.LastRecapAt = &t
	}
	if completedRaw.Valid {
		t := completedRaw.Time
		e.CompletedAt = &t
	}
	return &e, nil
}

// updateSupplies persists a new supplies snapshot.
func updateSupplies(expID string, s ExpeditionSupplies) error {
	b, _ := json.Marshal(s)
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET supplies_json = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, string(b), expID)
	return err
}

// updateCamp persists camp state. Pass nil to break camp.
func updateCamp(expID string, c *CampState) error {
	var arg any
	if c == nil {
		arg = nil
	} else {
		b, _ := json.Marshal(c)
		arg = string(b)
	}
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET camp_json = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, arg, expID)
	return err
}

// setExpeditionRunID writes the expedition's linked zone-run pointer.
// Used during region transitions and on initial spawn (R2).
func setExpeditionRunID(expID, runID string) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET run_id = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, nullableString(runID), expID)
	return err
}

// abandonExpedition flags the active expedition as abandoned. Idempotent.
func abandonExpedition(userID id.UserID) error {
	e, err := getActiveExpedition(userID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNoActiveExpedition
	}
	if _, err = db.Get().Exec(`
		UPDATE dnd_expedition
		   SET status = 'abandoned',
		       completed_at = CURRENT_TIMESTAMP,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, e.ID); err != nil {
		return err
	}
	releaseParty(e.ID)
	return nil
}

// completeExpedition marks the expedition complete (boss defeated or extracted).
func completeExpedition(expID string, status string) error {
	if _, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET status = ?,
		       completed_at = CURRENT_TIMESTAMP,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, status, expID); err != nil {
		return err
	}
	releaseParty(expID)
	return nil
}

// releaseParty clears the roster of an expedition that has reached a terminal
// status, freeing every member to start a run of their own. A member is barred
// from adventuring anywhere else while seated (assertNotAdventuring), so a
// roster that outlives its expedition strands the whole party.
//
// It is deliberately *not* called on the 'extracting' status: that is a
// seven-day resumable limbo, and `!resume` must bring the party back with it.
// The roster is cleared when the resume window lapses and the row flips to
// 'failed' — which routes through completeExpedition like everything else.
//
// A failure here is logged, not returned: the expedition is already terminal by
// the time we get here, and refusing to finish it over a stale roster row would
// leave the leader stuck instead of the members.
func releaseParty(expID string) {
	if err := disbandParty(expID); err != nil {
		slog.Warn("expedition: disband party", "expedition", expID, "err", err)
	}
	// An unanswered invite to a finished expedition would otherwise sit there
	// until its TTL, letting someone accept their way onto a corpse.
	if err := clearExpeditionInvites(expID); err != nil {
		slog.Warn("expedition: clear invites", "expedition", expID, "err", err)
	}
}

// applyThreatDelta clamps the threat level to [0,100], records the event,
// and flips siege_mode when the level reaches 100.
func applyThreatDelta(expID string, delta int, reason string) error {
	e, err := getExpedition(expID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNoActiveExpedition
	}
	level := e.ThreatLevel + delta
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	// Spec §8.3: Siege Mode is one-way — the OR keeps siege sticky
	// even if a subsequent negative delta drops the underlying level.
	siege := e.SiegeMode || level >= 100
	events := append(e.ThreatEvents, ThreatEvent{
		Timestamp: time.Now().UTC(),
		Delta:     delta,
		Reason:    reason,
	})
	eventsJSON, _ := json.Marshal(events)
	_, err = db.Get().Exec(`
		UPDATE dnd_expedition
		   SET threat_level = ?,
		       threat_siege = ?,
		       threat_events = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`,
		level, boolToInt(siege), string(eventsJSON), expID)
	return err
}

// updateTemporalStack persists the zone-temporal stack (heat / instability).
func updateTemporalStack(expID string, stack int) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET temporal_stack = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, stack, expID)
	return err
}

// advanceExpeditionDay bumps current_day by one. Real-time day cycle (E1d)
// calls this from the 06:00 cron.
func advanceExpeditionDay(expID string) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET current_day = current_day + 1,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, expID)
	return err
}

// appendExpeditionLog inserts one log entry.
func appendExpeditionLog(expID string, day int, entryType, summary, flavor string) error {
	_, err := db.Get().Exec(`
		INSERT INTO dnd_expedition_log (expedition_id, day, entry_type, summary, flavor)
		VALUES (?, ?, ?, ?, ?)`, expID, day, entryType, summary, flavor)
	return err
}

// dayExpeditionLog returns every log entry recorded against the given
// (expedition, day) pair, oldest first. Used by the D4-a end-of-day
// digest to bundle a single rollup DM at night-camp pitch.
func dayExpeditionLog(expID string, day int) ([]ExpeditionEntry, error) {
	rows, err := db.Get().Query(`
		SELECT entry_id, expedition_id, day, timestamp, entry_type, summary, flavor
		  FROM dnd_expedition_log
		 WHERE expedition_id = ?
		   AND day = ?
		 ORDER BY entry_id`, expID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpeditionEntry
	for rows.Next() {
		var e ExpeditionEntry
		if err := rows.Scan(
			&e.EntryID, &e.ExpeditionID, &e.Day, &e.Timestamp,
			&e.Type, &e.Summary, &e.Flavor,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// recentExpeditionLog returns the last `limit` entries, newest first.
func recentExpeditionLog(expID string, limit int) ([]ExpeditionEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := db.Get().Query(`
		SELECT entry_id, expedition_id, day, timestamp, entry_type, summary, flavor
		  FROM dnd_expedition_log
		 WHERE expedition_id = ?
		 ORDER BY timestamp DESC, entry_id DESC
		 LIMIT ?`, expID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpeditionEntry
	for rows.Next() {
		var e ExpeditionEntry
		if err := rows.Scan(
			&e.EntryID, &e.ExpeditionID, &e.Day, &e.Timestamp,
			&e.Type, &e.Summary, &e.Flavor,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

