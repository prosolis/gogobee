package plugin

import (
	"database/sql"
	"errors"
	"log/slog"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// N3/P6a — resolving shared state for a player who owns none of it.
//
// Every ownership lookup in the adventure module keys on a user id:
// getActiveExpedition reads dnd_expedition.user_id, getActiveZoneRun reads
// dnd_zone_run.user_id. A party member owns neither row. The leader's expedition
// is the single source of truth for the clock, the threat track and the supply
// pool, and the run it points at is the party's shared position in the dungeon.
//
// So a member asking either "am I on an expedition" or "where am I standing"
// gets nil from both, and every command built on them quietly tells them they
// are not playing. activeExpeditionFor (P4) answers the first question; this
// file answers the second, and gives the DM seams the audience they never had.

// activeZoneRunFor resolves the zone run a player is walking, whether they own
// the row or ride it as a party member. isLeader reports which — it is true for
// a solo player and a standalone (non-expedition) run alike, since both own
// their run.
//
// For an owner this is exactly getActiveZoneRun, side effects and all. That
// matters: getActiveZoneRun carries the §4.3 idle-timeout reap, which
// force-extracts the wrapping expedition. It must keep firing off the owner's
// own lookup and must never be re-entered on a member's behalf, or a member
// glancing at the map could reap the leader's run out from under them.
func activeZoneRunFor(userID id.UserID) (run *DungeonRun, isLeader bool, err error) {
	if r, err := getActiveZoneRun(userID); err != nil || r != nil {
		return r, true, err
	}
	e, isLeader, err := activeExpeditionFor(userID)
	switch {
	case err != nil || e == nil:
		return nil, false, err
	case isLeader:
		// getActiveZoneRun already answered for this expedition's owner: the run
		// is unspawned, reaped, or finished. Don't go looking behind its back.
		return nil, true, nil
	case e.RunID == "":
		// The leader hasn't taken the first step yet; the run is created lazily.
		return nil, false, nil
	}
	r, err := getZoneRun(e.RunID)
	if err != nil || r == nil || !r.IsActive() {
		return nil, false, err
	}
	return r, false, nil
}

// N3/P6d — the two seams the read-rewire needed.

// msgLeaderPicksPath is the refusal a member gets from every command that would
// move the party through the dungeon graph — `!zone go`, `!revisit`. One string,
// because they are one decision from the player's side.
const msgLeaderPicksPath = "Your party leader picks the path. `!map` to see where you're standing."

// isPartyMember reports whether this player rides someone else's *active*
// expedition. It is the question the leader-only copy gates actually ask, and
// it asks it without touching getActiveZoneRun — whose §4.3 idle reap must
// never fire on a member's behalf (see activeZoneRunFor).
//
// Prefer this over `run != nil && !isLeader`: activeZoneRunFor reports
// isLeader=false for a player with no run *anywhere*, so a bare !isLeader test
// tells a solo player with nothing in flight to go ask their leader.
func isPartyMember(userID id.UserID) bool {
	e, isLeader, err := activeExpeditionFor(userID)
	return err == nil && e != nil && !isLeader
}

// seatedExpeditionFor answers "is this player committed to somebody else's
// expedition right now" for the guards that refuse a second adventure.
//
// It deliberately spans `extracting` as well as `active`, which
// activeExpeditionFor does not. `extracting` is a seven-day resumable limbo:
// releaseParty is *not* called on it, so the roster survives and `!resume`
// brings the party back. A member is still seated for that whole window, and a
// guard that only sees 'active' would let them open a private run which then
// wins every activeZoneRunFor lookup once the leader resumes.
func seatedExpeditionFor(userID id.UserID) (*Expedition, error) {
	row := db.Get().QueryRow(`
		SELECT`+expeditionSelectCols+`
		  FROM dnd_expedition e
		  JOIN expedition_party p ON p.expedition_id = e.expedition_id
		 WHERE p.user_id = ? AND p.role <> 'leader'
		   AND e.status IN ('active', 'extracting')
		 ORDER BY e.start_date DESC
		 LIMIT 1`, string(userID))
	e, err := scanExpedition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// expeditionAudience is every player an expedition-scoped DM must reach: the
// owner alone for a solo run, the whole roster for a party.
//
// partyMemberIDs collapses an empty roster to the owner, so a solo expedition
// resolves to exactly the one user it always DM'd. A roster read that fails
// degrades to the owner rather than dropping the message — a player who misses
// their briefing because SQLite hiccuped is a worse outcome than a member who
// misses theirs.
//
// A hired companion is dropped here, and this is the chokepoint that keeps him
// out of every DM seam at once: the briefing, the recap, the digest, the
// extraction notice — and, crucially, the per-member side effects that ride the
// fan-out rather than the message. maybeRollPetArrivalOnEmerge would offer Pete
// a pet and park a pending interaction awaiting a reply that never comes;
// maybeFireAnchoredEvent would claim him a daily event slot and DM him a
// choice. He is not a person; he does not get mail. See isCompanionSeat.
func expeditionAudience(e *Expedition) []id.UserID {
	if e == nil || e.UserID == "" {
		return nil
	}
	seats, err := partyHumans(e.ID, e.UserID)
	if err != nil {
		slog.Warn("expedition: party roster read failed, DMing owner only",
			"expedition", e.ID, "err", err)
		return []id.UserID{id.UserID(e.UserID)}
	}
	out := make([]id.UserID, 0, len(seats))
	for _, s := range seats {
		out = append(out, s.UserID)
	}
	return out
}

// expeditionSeats is every body that sits down in a fight: the whole roster,
// hired companion included. It is deliberately NOT expeditionAudience — that one
// drops the companion because he does not get mail, and a combat roster built
// from it would seat everyone the leader paid for except the one he paid for.
//
// Mail and seats are different sets. Anything that sends is an audience;
// anything that fights is a seat.
func expeditionSeats(e *Expedition) []id.UserID {
	if e == nil || e.UserID == "" {
		return nil
	}
	ids, err := partyMemberIDs(e.ID, e.UserID)
	if err != nil {
		slog.Warn("expedition: party roster read failed, seating owner only",
			"expedition", e.ID, "err", err)
		return []id.UserID{id.UserID(e.UserID)}
	}
	return ids
}

// fanOutExpeditionDM sends one expedition-scoped body to every seated member.
//
// perReader, when non-nil, folds that reader's own character-scoped content into
// the shared body — the morning pet event, say. It runs once per member, in
// roster order, and may mutate that member's character sheet. This is the same
// shape P5's RenderPartyTurnRound settled on for combat narration: the body is
// the party's, the decoration is the reader's.
func (p *AdventurePlugin) fanOutExpeditionDM(e *Expedition, body string, perReader func(id.UserID, string) string) {
	for _, uid := range expeditionAudience(e) {
		text := body
		if perReader != nil {
			text = perReader(uid, text)
		}
		if err := p.SendDM(uid, text); err != nil {
			slog.Warn("expedition: fan-out DM failed", "user", uid, "expedition", e.ID, "err", err)
		}
	}
}
