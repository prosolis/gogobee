package plugin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

type CoopRun struct {
	ID              int
	Tier            int
	Status          string
	LeaderID        id.UserID
	CurrentDay      int
	TotalDays       int
	BaseDifficulty  int
	GoldPool        int
	RewardTotal     int
	LastResolvedDay int
	InvitePostID    id.EventID
	CreatedAt       time.Time
	LockedAt        *time.Time
	CompletedAt     *time.Time
}

type CoopMember struct {
	RunID            int
	UserID           id.UserID
	TurnOrder        int
	TotalContributed int
	IsLiability      bool
	DailyFunding     map[int]CoopFundingTier // day → tier
}

func createCoopRun(tier int, leader id.UserID, totalDays, baseDifficulty int) (*CoopRun, error) {
	d := db.Get()
	res, err := d.Exec(`INSERT INTO coop_dungeon_runs (tier, leader_id, total_days, base_difficulty)
		VALUES (?, ?, ?, ?)`, tier, string(leader), totalDays, baseDifficulty)
	if err != nil {
		return nil, err
	}
	id64, _ := res.LastInsertId()
	return loadCoopRun(int(id64))
}

func loadCoopRun(runID int) (*CoopRun, error) {
	d := db.Get()
	r := &CoopRun{}
	var leader string
	var lockedAt, completedAt sql.NullTime
	err := d.QueryRow(`SELECT id, tier, status, leader_id, current_day, total_days,
		base_difficulty, gold_pool, reward_total, last_resolved_day, invite_post_id, created_at, locked_at, completed_at
		FROM coop_dungeon_runs WHERE id = ?`, runID).Scan(
		&r.ID, &r.Tier, &r.Status, &leader, &r.CurrentDay, &r.TotalDays,
		&r.BaseDifficulty, &r.GoldPool, &r.RewardTotal, &r.LastResolvedDay,
		(*string)(&r.InvitePostID),
		&r.CreatedAt, &lockedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	r.LeaderID = id.UserID(leader)
	if lockedAt.Valid {
		t := lockedAt.Time
		r.LockedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		r.CompletedAt = &t
	}
	return r, nil
}

func loadOpenCoopRuns() ([]*CoopRun, error) {
	return queryCoopRuns(`status = 'open' ORDER BY created_at DESC`)
}

func loadActiveCoopRuns() ([]*CoopRun, error) {
	return queryCoopRuns(`status = 'active' ORDER BY id`)
}

func loadMostRecentOpenCoopRun() (*CoopRun, error) {
	runs, err := queryCoopRuns(`status = 'open' ORDER BY created_at DESC LIMIT 1`)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return runs[0], nil
}

func queryCoopRuns(whereClause string) ([]*CoopRun, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, tier, status, leader_id, current_day, total_days,
		base_difficulty, gold_pool, reward_total, last_resolved_day, invite_post_id, created_at, locked_at, completed_at
		FROM coop_dungeon_runs WHERE ` + whereClause)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*CoopRun
	for rows.Next() {
		r := &CoopRun{}
		var leader string
		var lockedAt, completedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.Tier, &r.Status, &leader, &r.CurrentDay, &r.TotalDays,
			&r.BaseDifficulty, &r.GoldPool, &r.RewardTotal, &r.LastResolvedDay,
			(*string)(&r.InvitePostID),
			&r.CreatedAt, &lockedAt, &completedAt); err != nil {
			return nil, err
		}
		r.LeaderID = id.UserID(leader)
		if lockedAt.Valid {
			t := lockedAt.Time
			r.LockedAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			r.CompletedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadCoopRunForUser(userID id.UserID) (*CoopRun, error) {
	d := db.Get()
	var runID int
	err := d.QueryRow(`SELECT m.run_id FROM coop_dungeon_members m
		JOIN coop_dungeon_runs r ON r.id = m.run_id
		WHERE m.user_id = ? AND r.status IN ('open','active')
		ORDER BY r.id DESC LIMIT 1`, string(userID)).Scan(&runID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return loadCoopRun(runID)
}

func setCoopRunStatus(runID int, status string) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs SET status = ? WHERE id = ?`, status, runID)
	return err
}

func lockCoopRun(runID int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs
		SET status = 'active', locked_at = CURRENT_TIMESTAMP, current_day = 1
		WHERE id = ? AND status = 'open'`, runID)
	return err
}

func advanceCoopDay(runID, newDay int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs SET current_day = ? WHERE id = ?`, newDay, runID)
	return err
}

func completeCoopRun(runID int, status string, reward int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs
		SET status = ?, reward_total = ?, completed_at = CURRENT_TIMESTAMP
		WHERE id = ?`, status, reward, runID)
	return err
}

func setCoopRunInvitePostID(runID int, postID id.EventID) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs SET invite_post_id = ? WHERE id = ?`, string(postID), runID)
	return err
}

// setCoopRunLastResolvedDay marks a day as resolved for crash-resume idempotency.
// Resolution is idempotent: re-entering coopResolveFloor for the same day is a no-op.
func setCoopRunLastResolvedDay(runID, day int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs SET last_resolved_day = ? WHERE id = ?`, day, runID)
	return err
}

func addCoopRunPool(runID, delta int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_runs SET gold_pool = gold_pool + ? WHERE id = ?`, delta, runID)
	return err
}

// ── Members ─────────────────────────────────────────────────────────────────

func addCoopMember(runID int, userID id.UserID, turnOrder int, liability bool) error {
	d := db.Get()
	libVal := 0
	if liability {
		libVal = 1
	}
	_, err := d.Exec(`INSERT INTO coop_dungeon_members
		(run_id, user_id, turn_order, is_liability, daily_funding) VALUES (?, ?, ?, ?, '{}')`,
		runID, string(userID), turnOrder, libVal)
	return err
}

func loadCoopMembers(runID int) ([]CoopMember, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT run_id, user_id, turn_order, total_contributed, is_liability, daily_funding
		FROM coop_dungeon_members WHERE run_id = ? ORDER BY turn_order`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CoopMember
	for rows.Next() {
		m := CoopMember{}
		var userID, fundingJSON string
		var liability int
		if err := rows.Scan(&m.RunID, &userID, &m.TurnOrder, &m.TotalContributed, &liability, &fundingJSON); err != nil {
			return nil, err
		}
		m.UserID = id.UserID(userID)
		m.IsLiability = liability != 0
		m.DailyFunding = parseCoopFundingMap(fundingJSON)
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadCoopMember(runID int, userID id.UserID) (*CoopMember, error) {
	d := db.Get()
	m := &CoopMember{}
	var uid, fundingJSON string
	var liability int
	err := d.QueryRow(`SELECT run_id, user_id, turn_order, total_contributed, is_liability, daily_funding
		FROM coop_dungeon_members WHERE run_id = ? AND user_id = ?`, runID, string(userID)).Scan(
		&m.RunID, &uid, &m.TurnOrder, &m.TotalContributed, &liability, &fundingJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.UserID = id.UserID(uid)
	m.IsLiability = liability != 0
	m.DailyFunding = parseCoopFundingMap(fundingJSON)
	return m, nil
}

// claimCoopMemberPayout atomically claims a member's reward slot. Returns true
// only if this call wins the race (member_payout was NULL and we set it).
// Prevents double-credit on crash-retry.
func claimCoopMemberPayout(runID int, userID id.UserID, payout int) (bool, error) {
	d := db.Get()
	res, err := d.Exec(`UPDATE coop_dungeon_members SET member_payout = ?
		WHERE run_id = ? AND user_id = ? AND member_payout IS NULL`,
		payout, runID, string(userID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func saveCoopMemberFunding(m *CoopMember) error {
	d := db.Get()
	jsonBytes, err := json.Marshal(serializeCoopFundingMap(m.DailyFunding))
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE coop_dungeon_members
		SET daily_funding = ?, total_contributed = ?
		WHERE run_id = ? AND user_id = ?`,
		string(jsonBytes), m.TotalContributed, m.RunID, string(m.UserID))
	return err
}

func parseCoopFundingMap(s string) map[int]CoopFundingTier {
	out := map[int]CoopFundingTier{}
	if s == "" || s == "{}" {
		return out
	}
	raw := map[string]string{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		slog.Error("coop: parse funding map", "raw", s, "err", err)
		return out
	}
	for k, v := range raw {
		day, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		out[day] = CoopFundingTier(v)
	}
	return out
}

func serializeCoopFundingMap(m map[int]CoopFundingTier) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[fmt.Sprintf("%d", k)] = string(v)
	}
	return out
}

// ── Floor Events ────────────────────────────────────────────────────────────

type CoopEvent struct {
	RunID           int
	Day             int
	Category        CoopEventCategory
	EventIndex      int
	Recommended     string
	Votes           map[id.UserID]string // userID → option label
	WinningVote     string               // "" until resolved
	Outcome         string               // "" until resolved; "success" or "failure"
	ModifierApplied int
	PostEventID     id.EventID
}

func createCoopEvent(runID, day int, cat CoopEventCategory, idx int, recommended string) error {
	d := db.Get()
	// INSERT OR IGNORE: idempotent on retry. If the row already exists from a
	// prior tick, leave it untouched (don't re-roll the event index).
	_, err := d.Exec(`INSERT OR IGNORE INTO coop_dungeon_events
		(run_id, day, category, event_index, recommended) VALUES (?, ?, ?, ?, ?)`,
		runID, day, string(cat), idx, recommended)
	return err
}

func loadCoopEvent(runID, day int) (*CoopEvent, error) {
	d := db.Get()
	e := &CoopEvent{}
	var cat, votesJSON string
	var winning, outcome, postID sql.NullString
	err := d.QueryRow(`SELECT run_id, day, category, event_index, recommended,
		votes, winning_vote, outcome, modifier_applied, post_event_id
		FROM coop_dungeon_events WHERE run_id = ? AND day = ?`, runID, day).Scan(
		&e.RunID, &e.Day, &cat, &e.EventIndex, &e.Recommended,
		&votesJSON, &winning, &outcome, &e.ModifierApplied, &postID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Category = CoopEventCategory(cat)
	e.Votes = parseCoopVotes(votesJSON)
	if winning.Valid {
		e.WinningVote = winning.String
	}
	if outcome.Valid {
		e.Outcome = outcome.String
	}
	if postID.Valid {
		e.PostEventID = id.EventID(postID.String)
	}
	return e, nil
}

func saveCoopEventVotes(e *CoopEvent) error {
	d := db.Get()
	jsonBytes, err := json.Marshal(serializeCoopVotes(e.Votes))
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE coop_dungeon_events SET votes = ?
		WHERE run_id = ? AND day = ?`, string(jsonBytes), e.RunID, e.Day)
	return err
}

func saveCoopEventPostID(runID, day int, eventID id.EventID) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_events SET post_event_id = ?
		WHERE run_id = ? AND day = ?`, string(eventID), runID, day)
	return err
}

func resolveCoopEvent(runID, day int, winning, outcome string, modifier int) error {
	d := db.Get()
	_, err := d.Exec(`UPDATE coop_dungeon_events
		SET winning_vote = ?, outcome = ?, modifier_applied = ?
		WHERE run_id = ? AND day = ?`, winning, outcome, modifier, runID, day)
	return err
}

// CoopTwinBeeHelpfulness returns a score in [0,1] where 1 means TwinBee's
// recommendations have been winning + succeeding (or losing + failing). Used
// by phase 3 betting odds.
func coopTwinBeeHelpfulness(window int) float64 {
	d := db.Get()
	rows, err := d.Query(`SELECT recommended, winning_vote, outcome
		FROM coop_dungeon_events
		WHERE outcome IS NOT NULL
		ORDER BY rowid DESC LIMIT ?`, window)
	if err != nil {
		return 0.5
	}
	defer rows.Close()
	hits, total := 0, 0
	for rows.Next() {
		var rec, win, out string
		if err := rows.Scan(&rec, &win, &out); err != nil {
			continue
		}
		total++
		if rec == win && out == "success" {
			hits++
		} else if rec != win && out == "failure" {
			hits++
		}
	}
	if total == 0 {
		return 0.5
	}
	return float64(hits) / float64(total)
}

func parseCoopVotes(s string) map[id.UserID]string {
	out := map[id.UserID]string{}
	if s == "" || s == "{}" {
		return out
	}
	raw := map[string]string{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		slog.Error("coop: parse votes map", "raw", s, "err", err)
		return out
	}
	for k, v := range raw {
		out[id.UserID(k)] = v
	}
	return out
}

func serializeCoopVotes(m map[id.UserID]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[string(k)] = v
	}
	return out
}
