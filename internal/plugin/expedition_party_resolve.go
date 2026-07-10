package plugin

import (
	"log/slog"

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

// expeditionAudience is every player an expedition-scoped DM must reach: the
// owner alone for a solo run, the whole roster for a party.
//
// partyMemberIDs collapses an empty roster to the owner, so a solo expedition
// resolves to exactly the one user it always DM'd. A roster read that fails
// degrades to the owner rather than dropping the message — a player who misses
// their briefing because SQLite hiccuped is a worse outcome than a member who
// misses theirs.
func expeditionAudience(e *Expedition) []id.UserID {
	if e == nil || e.UserID == "" {
		return nil
	}
	ids, err := partyMemberIDs(e.ID, e.UserID)
	if err != nil {
		slog.Warn("expedition: party roster read failed, DMing owner only",
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
