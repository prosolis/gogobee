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

// ExpeditionPartyMax exports the ceiling for cmd/expedition-sim's -party flag,
// so the harness cannot ask for a roster the seating layer will refuse.
const ExpeditionPartyMax = expeditionPartyMax

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

// PartySeatKind is what a seat *is*. The three answers differ in ways that matter
// at almost every seam, and conflating any two of them has already cost us a bug:
// a companion is a seat but not a mouth and not a mailbox; a leader owns the
// expedition row that everyone else references.
type PartySeatKind int

const (
	SeatLeader PartySeatKind = iota
	SeatMember
	SeatCompanion
)

// PartySeat is one body on an expedition.
type PartySeat struct {
	UserID id.UserID
	Kind   PartySeatKind
}

// IsHuman reports whether there is a person behind this seat — someone who can be
// DM'd, can earn loot, eats supplies, and can die.
func (s PartySeat) IsHuman() bool { return s.Kind != SeatCompanion }

// expeditionParty is THE answer to "who is on this expedition". Every other view
// — who gets mail, who sits down in a fight, who eats — is derived from it.
//
// It ALWAYS includes the owner. That is the whole point, and it is not a
// convenience: a solo expedition has **no expedition_party rows at all** (the
// roster only materializes on the first invite — see partyMembers), so any code
// that answers "who is in this party?" by reading the roster table gets *nobody*
// for a solo player, and then quietly falls back to whatever looked sensible at
// the call site.
//
// That is not hypothetical. It is exactly how the hired companion came out at
// **level 1** for every solo player — the one player the feature exists for. The
// level was averaged over the party; the party read as empty; the fallback was 1.
// He then walked into a tier-4 zone as a level-1 body, died on contact, and left
// the leader fighting a boss that had been inflated on his account. A 1500-run
// sweep is what found it, because nothing else could.
//
// So: there is no way to ask this function for the party and be handed an empty
// list. If the expedition exists, it has at least a leader.
func expeditionParty(expeditionID, ownerID string) ([]PartySeat, error) {
	rows, err := partyMembers(expeditionID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		// Solo: no roster rows. The owner IS the party.
		if ownerID == "" {
			return nil, nil
		}
		return []PartySeat{{UserID: id.UserID(ownerID), Kind: SeatLeader}}, nil
	}
	out := make([]PartySeat, 0, len(rows))
	for _, m := range rows {
		kind := SeatMember
		switch {
		case isCompanionUser(m.UserID):
			kind = SeatCompanion
		case m.IsLeader():
			kind = SeatLeader
		}
		out = append(out, PartySeat{UserID: id.UserID(m.UserID), Kind: kind})
	}
	return out, nil
}

// partyHumans is the party minus the companion: everyone with a person behind
// them. This is the set that gets mail, earns loot, eats supplies, and can die.
func partyHumans(expeditionID, ownerID string) ([]PartySeat, error) {
	seats, err := expeditionParty(expeditionID, ownerID)
	if err != nil {
		return nil, err
	}
	out := seats[:0]
	for _, s := range seats {
		if s.IsHuman() {
			out = append(out, s)
		}
	}
	return out, nil
}

// partyMemberIDs is the whole party as ids, leader first — every body, companion
// included. Callers that want only people want partyHumans.
func partyMemberIDs(expeditionID, ownerID string) ([]id.UserID, error) {
	seats, err := expeditionParty(expeditionID, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]id.UserID, 0, len(seats))
	for _, s := range seats {
		out = append(out, s.UserID)
	}
	return out, nil
}

// partySize is the number of seated *players*: 1 for a solo expedition.
//
// A hired companion is not counted, and both of this function's consumers want
// it that way:
//
//   - expeditionBurnRatePct scales the daily supply burn by party size. Pete
//     never bought a pack — members buy their own on !expedition accept, and he
//     accepts nothing — so counting him would bill the leader a 60% higher burn
//     for a mouth that brought its own rations. He is on expenses.
//   - the "your party is still waiting on you" gate blocks a leader from
//     starting a new run while an extracted party is pending. A roster holding
//     nobody but Pete is not a party waiting on anyone, and counting him would
//     lock the leader out of the game until they abandoned the run.
//
// The combat roster is a different question with a different answer: Pete IS a
// body in the fight, so CombatSession.RosterSize() counts him and the enemy-HP
// scalar feels him. Seats and mouths are not the same set.
func partySize(expeditionID string) (int, error) {
	var n int
	err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM expedition_party
		  WHERE expedition_id = ? AND user_id <> ?`,
		expeditionID, string(companionUserID())).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 1, nil
	}
	return n, nil
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

// seatLeader converts a solo expedition row into a party of one, inside a
// caller's transaction. It reads the owner off the expedition row rather than
// trusting a passed-in id, so the roster's leader can never disagree with
// dnd_expedition.user_id. Idempotent (ON CONFLICT DO NOTHING), so the invite
// path can call it unconditionally before adding a member.
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
		SELECT`+expeditionSelectCols+`
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
