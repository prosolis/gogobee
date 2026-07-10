package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// N3/P4 — expedition party membership.
//
// A co-op expedition is one dnd_expedition row plus an expedition_party roster.
// The leader's row stays the single source of truth for the shared clock, the
// threat track, and the supply pool; members reference it rather than owning a
// row of their own. There is no party_id column — expedition_id already
// identifies the party, and a second key would be a second answer to "who is in
// this party".
//
// Absence means solo. Every expedition that existed before N3 has no rows here,
// which is exactly the right reading, so nothing needs backfilling. A solo
// expedition started after N3 writes no rows either: the roster materializes on
// the first successful invite (P6), and until then partyMembers returns empty.

// Party roles (expedition_party.role).
const (
	PartyRoleLeader = "leader"
	PartyRoleMember = "member"
)

// expeditionPartyMax is the roster ceiling including the leader. C1 scopes v1
// to 2–3 players; the combat roster and the supply pool are both sized off this.
const expeditionPartyMax = 3

// Errors returned by the party layer.
var (
	ErrPartyFull           = errors.New("expedition party is full")
	ErrAlreadyInParty      = errors.New("player is already in this party")
	ErrNotPartyLeader      = errors.New("only the party leader may do that")
	ErrPlayerBusyElsewhere = errors.New("player already has an active expedition")
)

// PartyMember is one seat on an expedition roster.
type PartyMember struct {
	ExpeditionID string
	UserID       string
	Role         string
	JoinedAt     time.Time
}

// IsLeader reports whether this member owns the expedition row.
func (m PartyMember) IsLeader() bool { return m.Role == PartyRoleLeader }

// partyMembers returns the roster in join order, leader first. A solo
// expedition has no rows and returns an empty slice — callers should treat
// len(roster) == 0 and len(roster) == 1 alike, as "one player".
func partyMembers(expeditionID string) ([]PartyMember, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id, user_id, role, joined_at
		  FROM expedition_party
		 WHERE expedition_id = ?
		 ORDER BY (role <> 'leader'), joined_at ASC`, expeditionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PartyMember
	for rows.Next() {
		var m PartyMember
		if err := rows.Scan(&m.ExpeditionID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// partyMemberIDs is partyMembers reduced to user ids, leader first. It is what
// the fan-out seams (digest, briefing, recap) want: a solo expedition yields
// just the owner, so a caller can loop unconditionally.
func partyMemberIDs(expeditionID, ownerID string) ([]id.UserID, error) {
	members, err := partyMembers(expeditionID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []id.UserID{id.UserID(ownerID)}, nil
	}
	out := make([]id.UserID, 0, len(members))
	for _, m := range members {
		out = append(out, id.UserID(m.UserID))
	}
	return out, nil
}

// partySize is the number of seated players: 1 for a solo expedition.
func partySize(expeditionID string) (int, error) {
	var n int
	err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM expedition_party WHERE expedition_id = ?`,
		expeditionID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 1, nil
	}
	return n, nil
}

// openParty seats the leader, converting a solo expedition row into a party of
// one. It is idempotent: a second call on the same expedition is a no-op, so
// the invite path can call it unconditionally before adding a member.
func openParty(expeditionID string, leaderID id.UserID) error {
	_, err := db.Get().Exec(`
		INSERT INTO expedition_party (expedition_id, user_id, role)
		VALUES (?, ?, 'leader')
		ON CONFLICT (expedition_id, user_id) DO NOTHING`,
		expeditionID, string(leaderID))
	return err
}

// joinParty seats a member. It refuses a full roster, a duplicate, and a player
// who already leads or rides an expedition of their own — the "one active
// expedition per user" rule that startExpedition enforces in code, extended to
// cover membership. The check and the insert share a transaction so two
// simultaneous invites cannot both find the last seat free.
//
// It seats the leader first if nobody has. A roster whose leader is missing
// would quietly drop them from every fan-out (partyMemberIDs reads the roster,
// not the expedition row), so the ordering is enforced here rather than left to
// each caller to remember.
func joinParty(expeditionID string, userID id.UserID) error {
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := seatLeader(tx, expeditionID); err != nil {
		return err
	}

	var n int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM expedition_party WHERE expedition_id = ?`,
		expeditionID).Scan(&n); err != nil {
		return err
	}
	if n >= expeditionPartyMax {
		return ErrPartyFull
	}

	if err := assertNotAdventuring(tx, expeditionID, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO expedition_party (expedition_id, user_id, role)
		VALUES (?, ?, 'member')`,
		expeditionID, string(userID)); err != nil {
		return fmt.Errorf("seat %s: %w", userID, err)
	}
	return tx.Commit()
}

// seatLeader is openParty inside a caller's transaction: it reads the owner off
// the expedition row rather than trusting a passed-in id, so the roster's leader
// can never disagree with dnd_expedition.user_id.
func seatLeader(tx *sql.Tx, expeditionID string) error {
	var owner string
	err := tx.QueryRow(
		`SELECT user_id FROM dnd_expedition WHERE expedition_id = ?`,
		expeditionID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("expedition %s not found", expeditionID)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO expedition_party (expedition_id, user_id, role)
		VALUES (?, ?, 'leader')
		ON CONFLICT (expedition_id, user_id) DO NOTHING`, expeditionID, owner)
	return err
}

// assertNotAdventuring fails if the player is already committed elsewhere:
// seated on this or any other party, or owning an active expedition row. Runs
// inside joinParty's transaction so the guard and the insert are atomic.
func assertNotAdventuring(tx *sql.Tx, expeditionID string, userID id.UserID) error {
	var seatedIn string
	err := tx.QueryRow(`
		SELECT expedition_id FROM expedition_party WHERE user_id = ? LIMIT 1`,
		string(userID)).Scan(&seatedIn)
	switch {
	case err == nil && seatedIn == expeditionID:
		return ErrAlreadyInParty
	case err == nil:
		return ErrPlayerBusyElsewhere
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	var owned int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM dnd_expedition
		 WHERE user_id = ? AND status IN ('active', 'extracting')`,
		string(userID)).Scan(&owned); err != nil {
		return err
	}
	if owned > 0 {
		return ErrPlayerBusyElsewhere
	}
	return nil
}

// leaveParty removes one member. The leader cannot leave — an expedition
// without its owner has no row to hang the shared clock on; the leader ends the
// run for everyone with !extract instead.
func leaveParty(expeditionID string, userID id.UserID) error {
	res, err := db.Get().Exec(`
		DELETE FROM expedition_party
		 WHERE expedition_id = ? AND user_id = ? AND role <> 'leader'`,
		expeditionID, string(userID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotPartyLeader
	}
	return nil
}

// disbandParty clears the roster. Called when an expedition reaches a terminal
// status, so every member is free to start their own next run.
func disbandParty(expeditionID string) error {
	_, err := db.Get().Exec(
		`DELETE FROM expedition_party WHERE expedition_id = ?`, expeditionID)
	return err
}

// expeditionForMember resolves the expedition a player is seated on but does
// not own. getActiveExpedition keys on dnd_expedition.user_id and so is blind to
// members; every player-facing lookup wants both, which is what
// activeExpeditionFor provides.
func expeditionForMember(userID id.UserID) (*Expedition, error) {
	row := db.Get().QueryRow(`
		SELECT e.expedition_id, e.user_id, e.zone_id, e.run_id, e.status,
		       e.start_date, e.current_day, e.current_region, e.boss_defeated,
		       e.supplies_json, e.camp_json, e.threat_level, e.threat_siege,
		       e.threat_events, e.temporal_stack, e.region_state,
		       e.xp_earned, e.coins_earned, e.gm_mood,
		       e.last_briefing_at, e.last_recap_at, e.last_ambient_kind,
		       e.last_activity, e.completed_at
		  FROM dnd_expedition e
		  JOIN expedition_party p ON p.expedition_id = e.expedition_id
		 WHERE p.user_id = ? AND p.role <> 'leader' AND e.status = 'active'
		 ORDER BY e.start_date DESC
		 LIMIT 1`, string(userID))
	e, err := scanExpedition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// activeExpeditionFor returns the expedition a player is on, whether they lead
// it or ride it, plus whether they lead it. This is the lookup every
// player-facing command wants; getActiveExpedition alone silently tells a party
// member they have no expedition.
func activeExpeditionFor(userID id.UserID) (e *Expedition, isLeader bool, err error) {
	if e, err = getActiveExpedition(userID); err != nil || e != nil {
		return e, e != nil, err
	}
	e, err = expeditionForMember(userID)
	return e, false, err
}
