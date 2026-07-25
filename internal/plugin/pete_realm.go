package plugin

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The realm snapshot: the world map, the hall of firsts, and the board.
//
// Everything Pete has shown so far is either the present moment (the roster, the
// Siege bar) or one thing that happened (a dispatch, a run log). None of it says
// what the realm *is* — that there are thirty-odd named places with a difficulty
// order, that some of them have never been beaten by anybody, and that the
// people playing have a history against them. That is what this carries.
//
// Snapshot semantics, like the roster and the Siege: pushed whole, replaces
// Pete's copy, dropped rather than retried on failure. The difference is the
// clock. The roster is a photograph of where people are standing and is worth
// re-taking every two minutes; a realm-first is a thing that happened once, ever,
// and re-deriving the whole ledger plus three aggregate scans at that rate would
// be pure waste. So this rides the same ticker at a much longer stride.
const (
	// realmPushInterval — how often the realm is recomputed and pushed. The
	// fastest-moving field in the whole snapshot is a zone's occupant list, and a
	// ten-minute-old answer to "who is in the Sunken Vault" is still a true and
	// useful one. Everything else moves on the scale of days.
	realmPushInterval = 10 * time.Minute

	// realmMaxOccupants bounds the per-zone occupant list. A realm has tens of
	// players; this only stops a pathological case spooling a huge payload.
	realmMaxOccupants = 50
)

// realmLastPush is when the realm snapshot last went out. Zero means never, so
// the first tick after start-up always pushes — an operator restarting the bot
// should not have to wait ten minutes to see whether the wire works.
var realmLastPush time.Time

// realmPushOK mirrors rosterPushOK: log the transitions and nothing else.
var realmPushOK bool

// pushRealm recomputes and sends the realm snapshot, at most once per
// realmPushInterval however often the ticker calls it.
func (p *AdventurePlugin) pushRealm() {
	now := time.Now().UTC()
	if !realmLastPush.IsZero() && now.Sub(realmLastPush) < realmPushInterval {
		return
	}

	snap, err := buildRealmSnapshot(now)
	if err != nil {
		slog.Error("realm: build snapshot failed", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()

	if err := peteclient.PushRealm(ctx, snap); err != nil {
		if realmPushOK {
			slog.Warn("realm: push failed, realm pages will go stale on Pete", "err", err)
		} else {
			slog.Debug("realm: push failed, dropping snapshot", "err", err)
		}
		realmPushOK = false
		// Deliberately NOT stamping realmLastPush: a failed push should be retried
		// on the next roster tick, not ten minutes from now. The stamp is a
		// "we already told Pete this" marker, and we didn't.
		return
	}

	realmLastPush = now
	if !realmPushOK {
		slog.Info("realm: snapshot accepted by Pete — realm pages are publishing",
			"zones", len(snap.Zones), "firsts", len(snap.Firsts), "standings", len(snap.Standings))
		realmPushOK = true
	}
}

// realmClearStats is the per-zone clear history, read in one pass.
type realmClearStats struct {
	clears       int
	clearers     int
	firstUser    id.UserID
	firstClearAt int64
}

// buildRealmSnapshot assembles the whole realm from the game's own tables.
//
// The three aggregate reads are done up front and once each, keyed into maps,
// rather than per-zone or per-player: this runs against the live DB on a ticker
// and a query-per-zone loop over a registry that only grows is the kind of thing
// that is fine until it isn't.
func buildRealmSnapshot(now time.Time) (peteclient.RealmSnapshot, error) {
	snap := peteclient.RealmSnapshot{SnapshotAt: now.Unix()}

	clearsByZone, err := loadRealmClearStats()
	if err != nil {
		return snap, err
	}
	occupantsByZone := loadRealmOccupants()

	// ── Zones ───────────────────────────────────────────────────────────────
	// zoneOrder is the design-doc ordering and is what the page draws in, so the
	// realm reads the way it was designed rather than the way a map iterates.
	for _, zid := range zoneOrder {
		def, ok := dndZoneRegistry[zid]
		if !ok {
			continue
		}
		z := peteclient.RealmZone{
			ID:         string(def.ID),
			Display:    def.Display,
			Tier:       int(def.Tier),
			LevelMin:   def.LevelMin,
			LevelMax:   def.LevelMax,
			Faction:    def.Faction,
			Atmosphere: def.Atmosphere,
			Postgame:   def.Tier == ZoneTierMythic,
			Occupants:  occupantsByZone[string(def.ID)],
		}
		if st, ok := clearsByZone[string(def.ID)]; ok {
			z.Clears = st.clears
			z.Clearers = st.clearers
			z.FirstClearAt = st.firstClearAt
			// An opted-out first-clearer keeps the claim and loses the identity —
			// the Siege contributor rule, and for the same reason. Deleting the
			// claim outright would leave the zone drawn as never-cleared, which is
			// a false statement about the realm rather than a withheld one.
			if !isNewsOptedOut(st.firstUser) {
				z.FirstClearBy = charName(st.firstUser)
				if z.FirstClearBy != "" {
					z.FirstClearToken = eventToken(st.firstUser, "roster")
				}
			}
		}
		snap.Zones = append(snap.Zones, z)
	}

	snap.Firsts = loadRealmFirsts(clearsByZone)

	// The three pages must not be able to contradict each other about the same
	// zone. loadRealmFirsts derives the zone half of the hall from clearsByZone —
	// the same map the tiles and the board use — but it also carries any ledger
	// claim with no surviving run behind it, and that case would otherwise draw a
	// place as never-beaten on the map while the hall named the year it fell.
	//
	// So an unbacked claim floors the clear count at one. Deliberately a floor and
	// not an assignment: where the run history is intact it is the better answer
	// and this only fills a hole. Nothing is attributed — the zone reads "cleared,
	// by somebody", which is exactly what is known about it.
	for i := range snap.Zones {
		if snap.Zones[i].Clears > 0 {
			continue
		}
		for _, f := range snap.Firsts {
			if f.Kind == "zone" && f.Target == snap.Zones[i].ID {
				snap.Zones[i].Clears = 1
				snap.Zones[i].Clearers = 1
				break
			}
		}
	}

	snap.Standings, err = loadRealmStandings(clearsByZone)
	if err != nil {
		return snap, err
	}
	return snap, nil
}

// loadRealmClearStats reads every zone's clear history in one pass: how many
// successful runs, how many distinct people managed it, and who did it first.
//
// MIN(completed_at) with a bare user_id column is SQLite's bare-column min/max
// rule — the user_id comes from the same row the minimum came from, so the
// (zone, first clearer, when) triple is internally consistent. backfillZoneFirsts
// relies on exactly this and has done since the news seam shipped.
//
// completed_at is selected raw as a string and parsed in Go rather than being
// wrapped in anything: modernc.org/sqlite rebuilds a time.Time from the column's
// declared type, and passing a DATETIME through MIN() alongside an aggregate is
// close enough to the COALESCE() trap that it is not worth finding out. The
// string always parses — it is written by SQLite's own CURRENT_TIMESTAMP.
//
// NOTE the absence of `AND abandoned = 0`, which looks like it belongs here and
// does not. `abandoned` does not mean "the player gave up" — it means the run
// ROW was retired, and abandonZoneRunByID exists specifically to retire a run
// whose boss is already dead when the expedition travels onward (see its comment
// in dnd_zone_run.go). In prod, 30 of the realm's 32 boss kills carry
// abandoned = 1. Filtering them out drew a map on which almost nothing had ever
// been beaten, while the hall of firsts — reading a different table — said six
// zones had been. boss_defeated = 1 is the clear, full stop.
func loadRealmClearStats() (map[string]realmClearStats, error) {
	rows, err := db.Get().Query(`
		SELECT zone_id,
		       COUNT(*)                AS clears,
		       COUNT(DISTINCT user_id) AS clearers,
		       user_id,
		       MIN(completed_at)
		  FROM dnd_zone_run
		 WHERE boss_defeated = 1 AND completed_at IS NOT NULL
		 GROUP BY zone_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]realmClearStats{}
	for rows.Next() {
		var zoneID, userID, completedAt string
		var st realmClearStats
		if err := rows.Scan(&zoneID, &st.clears, &st.clearers, &userID, &completedAt); err != nil {
			return nil, err
		}
		st.firstUser = id.UserID(userID)
		if ts, ok := parseSQLiteTime(completedAt); ok {
			st.firstClearAt = ts.Unix()
		}
		out[zoneID] = st
	}
	return out, rows.Err()
}

// loadRealmOccupants answers "who is in there right now", per zone.
//
// Presence is dropped for an opted-out player rather than anonymised. Unlike a
// first clear it is not part of a tally that stops adding up without them, and
// it is the same live-location fact the run liveblog refuses to publish — an
// anonymous "somebody is in the Drowned Star" next to a roster showing exactly
// one person out on expedition is not an anonymisation.
//
// Errors are swallowed to nil: an unreadable expedition table should cost the
// realm map its occupant dots, not the whole page.
func loadRealmOccupants() map[string][]peteclient.RealmOccupant {
	rows, err := db.Get().Query(
		`SELECT user_id, zone_id, current_day FROM dnd_expedition WHERE status = 'active'`)
	if err != nil {
		slog.Error("realm: occupants query", "err", err)
		return nil
	}
	defer rows.Close()

	type live struct {
		uid    id.UserID
		zoneID string
		day    int
	}
	var found []live
	for rows.Next() {
		var uid, zoneID string
		var day int
		if err := rows.Scan(&uid, &zoneID, &day); err != nil {
			slog.Error("realm: occupants scan", "err", err)
			return nil
		}
		found = append(found, live{id.UserID(uid), zoneID, day})
	}
	if err := rows.Err(); err != nil {
		slog.Error("realm: occupants rows", "err", err)
		return nil
	}

	// Names and opt-out are resolved only after the cursor is drained. The pool
	// is one connection wide and charName reads the DB; resolving inside the loop
	// is the deadlock W2a shipped and then had to fix.
	out := map[string][]peteclient.RealmOccupant{}
	for _, l := range found {
		if isNewsOptedOut(l.uid) {
			continue
		}
		name := charName(l.uid)
		if name == "" {
			continue // never fall back to a Matrix handle on a public page
		}
		if len(out[l.zoneID]) >= realmMaxOccupants {
			continue
		}
		out[l.zoneID] = append(out[l.zoneID], peteclient.RealmOccupant{
			Token: eventToken(l.uid, "roster"),
			Name:  name,
			Level: charLevel(l.uid),
			Day:   l.day,
		})
	}
	for zid := range out {
		sort.Slice(out[zid], func(i, j int) bool { return out[zid][i].Name < out[zid][j].Name })
	}
	return out
}

// loadRealmFirsts renders news_realm_firsts as a history book.
//
// The ledger stores only (kind, target, first_at) — it exists to tier a dispatch,
// not to remember who. The holder is recovered here from the game's own history:
// a zone first from the earliest boss-defeating run, a treasure first from the
// earliest surviving row in adventure_treasures. Both can come back empty — a
// treasure that was found and later discarded leaves no owner anywhere — and an
// unattributed first is rendered as one rather than dropped. It still happened.
// loadRealmFirsts renders the hall of firsts, and it takes the clear stats
// rather than reading the ledger alone, for two reasons that only showed up
// against real prod data:
//
//  1. news_realm_firsts is INCOMPLETE for zones. It has only been written since
//     the news seam went live, and the one-shot that seeded it filtered on
//     `abandoned = 0` — the same wrong filter loadRealmClearStats documents — so
//     it missed every zone whose clears were all retired runs. In prod it holds 6
//     zones where the run history knows 9.
//  2. Its first_at is when the CLAIM was recorded, not when the thing happened.
//     Every backfilled row in prod carries the same timestamp: the minute the
//     backfill ran. A history book dated by when somebody wrote it down is not
//     much of a history book.
//
// So the zone half is derived from the run history, which is complete and
// correctly dated, and the ledger supplies the kinds the run history knows
// nothing about (treasures, and whatever ships next) plus any zone claim with no
// surviving run behind it. That also makes the hall agree with the board by
// construction: both count a zone-first as "you were the first to clear it".
func loadRealmFirsts(clearsByZone map[string]realmClearStats) []peteclient.RealmFirst {
	rows, err := db.Get().Query(
		`SELECT kind, target, first_at FROM news_realm_firsts ORDER BY first_at ASC`)
	if err != nil {
		slog.Error("realm: firsts query", "err", err)
		return nil
	}
	defer rows.Close()

	var out []peteclient.RealmFirst
	for rows.Next() {
		var f peteclient.RealmFirst
		if err := rows.Scan(&f.Kind, &f.Target, &f.AtUnix); err != nil {
			slog.Error("realm: firsts scan", "err", err)
			return nil
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		slog.Error("realm: firsts rows", "err", err)
		return nil
	}

	// Same discipline as the occupants: the cursor is closed before anything
	// else touches the database.
	rows.Close()

	// Drop the ledger's zone rows wherever the run history has the same zone —
	// it is the better record of both who and when. A claim with no run behind it
	// survives, unattributed, and is what floors that zone's clear count in
	// buildRealmSnapshot.
	kept := out[:0]
	for _, f := range out {
		if f.Kind == "zone" {
			if _, ok := clearsByZone[f.Target]; ok {
				continue
			}
		}
		kept = append(kept, f)
	}
	out = kept

	// The zone half, from the authority the map and the board also use.
	for zoneID, st := range clearsByZone {
		zone := zoneOrFallback(ZoneID(zoneID))
		f := peteclient.RealmFirst{
			Kind:    "zone",
			Target:  zoneID,
			Display: zone.Display,
			Tier:    int(zone.Tier),
			AtUnix:  st.firstClearAt,
		}
		if !isNewsOptedOut(st.firstUser) {
			if name := charName(st.firstUser); name != "" {
				f.Holder = name
				f.Token = eventToken(st.firstUser, "roster")
			}
		}
		out = append(out, f)
	}

	for i := range out {
		switch out[i].Kind {
		case "zone":
			if out[i].Display == "" {
				zone := zoneOrFallback(ZoneID(out[i].Target))
				out[i].Display = zone.Display
				out[i].Tier = int(zone.Tier)
				out[i].Holder, out[i].Token = realmFirstZoneHolder(out[i].Target)
			}
		case "treasure":
			if def := lookupAdvTreasureDef(out[i].Target); def != nil {
				out[i].Display = def.Name
				out[i].Tier = def.Tier
			} else {
				out[i].Display = out[i].Target
			}
			out[i].Holder, out[i].Token = realmFirstTreasureHolder(out[i].Target)
		default:
			// A kind nobody has taught this function about still belongs in the
			// hall — it is a genuine realm-first and the ledger says so. It just
			// arrives with the raw target as its name, which is the same
			// degrade-don't-drop rule the unknown event_type inversion settled on.
			out[i].Display = out[i].Target
		}
	}

	// Oldest first: the order it happened in. Pete regroups it newest-year-first
	// for the page, but the wire carries history in history's order.
	sort.Slice(out, func(i, j int) bool {
		if out[i].AtUnix != out[j].AtUnix {
			return out[i].AtUnix < out[j].AtUnix
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// realmFirstZoneHolder names the earliest clearer of a zone. Returns ("", "")
// when the run history no longer has one, and ("Name", "") when it does but the
// player has opted out — the claim survives the anonymisation, the link does not.
func realmFirstZoneHolder(zoneID string) (name, token string) {
	var userID string
	err := db.Get().QueryRow(`
		SELECT user_id
		  FROM dnd_zone_run
		 WHERE zone_id = ? AND boss_defeated = 1 AND completed_at IS NOT NULL
		 ORDER BY completed_at ASC
		 LIMIT 1`, zoneID).Scan(&userID)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Error("realm: zone-first holder", "zone", zoneID, "err", err)
		}
		return "", ""
	}
	uid := id.UserID(userID)
	if isNewsOptedOut(uid) {
		return "", ""
	}
	name = charName(uid)
	if name == "" {
		return "", ""
	}
	return name, eventToken(uid, "roster")
}

// realmFirstTreasureHolder names the earliest holder of a treasure key. A
// treasure writes one row per bonus, so the MIN is over what may be several rows
// for the same acquisition; that is fine, they share a timestamp.
func realmFirstTreasureHolder(key string) (name, token string) {
	var userID string
	err := db.Get().QueryRow(`
		SELECT user_id
		  FROM adventure_treasures
		 WHERE treasure_key = ?
		 ORDER BY acquired_at ASC
		 LIMIT 1`, key).Scan(&userID)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Error("realm: treasure-first holder", "key", key, "err", err)
		}
		return "", ""
	}
	uid := id.UserID(userID)
	if isNewsOptedOut(uid) {
		return "", ""
	}
	name = charName(uid)
	if name == "" {
		return "", ""
	}
	return name, eventToken(uid, "roster")
}

// loadRealmStandings builds the board: one line per living, named, opted-in
// adventurer, every number a lifetime total.
//
// It walks player_meta the way buildRosterSnapshot does, and for the same
// reason — that is the list of people who exist, and a standings table assembled
// by grouping the run history instead would silently include characters that have
// since been deleted or never finished setup.
func loadRealmStandings(clearsByZone map[string]realmClearStats) ([]peteclient.RealmStanding, error) {
	rows, err := db.Get().Query(`SELECT user_id FROM player_meta WHERE alive = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uids []id.UserID
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		uids = append(uids, id.UserID(uid))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Who holds how many realm-firsts, from the same authority the zone column
	// uses — so a zone's "first cleared by X" and X's firsts count can never
	// disagree with each other.
	firstsBy := map[id.UserID]int{}
	for _, st := range clearsByZone {
		firstsBy[st.firstUser]++
	}

	perZone, err := loadRealmPlayerClears()
	if err != nil {
		return nil, err
	}
	siege := loadRealmSiegeTotals()

	var out []peteclient.RealmStanding
	for _, uid := range uids {
		if isNewsOptedOut(uid) {
			continue // the board omits an opted-out player outright, as it always has
		}
		c, err := LoadDnDCharacter(uid)
		if err != nil || c == nil || c.PendingSetup {
			continue
		}
		name := charName(uid)
		if name == "" {
			continue
		}
		s := peteclient.RealmStanding{
			Token:       eventToken(uid, "roster"),
			Name:        name,
			Level:       c.Level,
			ClassRace:   classRaceLabel(c),
			Firsts:      firstsBy[uid],
			SiegeDamage: siege[uid].damage,
			SiegeFights: siege[uid].fights,
		}
		for zoneID, n := range perZone[uid] {
			s.Clears += n
			s.Zones++
			if t := int(zoneOrFallback(ZoneID(zoneID)).Tier); t > s.DeepestTier {
				s.DeepestTier = t
			}
		}
		out = append(out, s)
	}

	// Ranked here, not on Pete: the ordering is a statement about the game
	// ("deepest tier beaten, then how much of the realm you have beaten"), and
	// the game is the thing that gets to make it. Name breaks the tie so the
	// board is stable between snapshots that are otherwise identical.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.DeepestTier != b.DeepestTier:
			return a.DeepestTier > b.DeepestTier
		case a.Zones != b.Zones:
			return a.Zones > b.Zones
		case a.Clears != b.Clears:
			return a.Clears > b.Clears
		case a.Level != b.Level:
			return a.Level > b.Level
		default:
			return a.Name < b.Name
		}
	})
	return out, nil
}

// loadRealmPlayerClears returns clears[user][zone] = count, in one pass.
func loadRealmPlayerClears() (map[id.UserID]map[string]int, error) {
	rows, err := db.Get().Query(`
		SELECT user_id, zone_id, COUNT(*)
		  FROM dnd_zone_run
		 WHERE boss_defeated = 1 AND completed_at IS NOT NULL
		 GROUP BY user_id, zone_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[id.UserID]map[string]int{}
	for rows.Next() {
		var uid, zoneID string
		var n int
		if err := rows.Scan(&uid, &zoneID, &n); err != nil {
			return nil, err
		}
		u := id.UserID(uid)
		if out[u] == nil {
			out[u] = map[string]int{}
		}
		out[u][zoneID] = n
	}
	return out, rows.Err()
}

type realmSiegeTotal struct{ damage, fights int }

// loadRealmSiegeTotals sums every Siege a player has ever turned up to. Across
// all bosses, not just the live one — the war room already shows the current
// muster, and what the board is for is the person who has shown up to all six.
func loadRealmSiegeTotals() map[id.UserID]realmSiegeTotal {
	rows, err := db.Get().Query(
		`SELECT user_id, SUM(damage), SUM(fights) FROM world_boss_contrib GROUP BY user_id`)
	if err != nil {
		slog.Error("realm: siege totals query", "err", err)
		return nil
	}
	defer rows.Close()

	out := map[id.UserID]realmSiegeTotal{}
	for rows.Next() {
		var uid string
		var damage, fights int
		if err := rows.Scan(&uid, &damage, &fights); err != nil {
			slog.Error("realm: siege totals scan", "err", err)
			return nil
		}
		out[id.UserID(uid)] = realmSiegeTotal{damage, fights}
	}
	if err := rows.Err(); err != nil {
		slog.Error("realm: siege totals rows", "err", err)
		return nil
	}
	return out
}
