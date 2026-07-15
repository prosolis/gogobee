package plugin

import (
	"context"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The live adventurer board pushed to Pete (gogobee_boredom_plan.md's sibling —
// see the roster section of the Pete plan).
//
// Everything else we send Pete is an accomplishment: a death, a clear, a
// milestone. Those are clippings — they read as archive the moment they land, no
// matter how fast we deliver them. The board is the other kind of thing: state
// that is *currently true*, which is the only thing that can make a page feel
// alive. So it is a snapshot, pushed whole, replacing whatever Pete had.
//
// It is also, by design, a target list. The plan is to let people who aren't
// even playing hire assassins and mobs against adventurers who are out in the
// world right now — so the board carries a stable per-player token and real zone
// depth, not just a pretty display string, and it shows the zone *live* while
// they're still in it.
const (
	// rosterTickInterval — how often we push. Pete's staleness window is several
	// times this, so a missed push or two is invisible; a real outage isn't.
	rosterTickInterval = 2 * time.Minute

	// rosterPushTimeout — the push is dropped on failure, never retried (a stale
	// snapshot is a lie, and the next tick carries the truth), so it must not be
	// able to pile up.
	rosterPushTimeout = 15 * time.Second
)

// peteRosterTicker pushes the board to Pete forever.
func (p *AdventurePlugin) peteRosterTicker() {
	if !peteclient.Enabled() {
		return
	}
	ticker := time.NewTicker(rosterTickInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !newsEmissionOn() {
			continue // master switch off: the board goes stale on Pete and says so
		}
		p.pushRoster()
	}
}

// rosterPushOK tracks the last push's outcome so we can log the transitions and
// nothing else. A push every 2 minutes forever is far too noisy to log at INFO,
// but total silence is worse: a ticker that is succeeding quietly looks exactly
// like one that never started, and that ambiguity already cost an operator a
// wrong-turn debug during the first deploy. So: say something the first time it
// works, say something when it breaks, say something when it recovers.
var rosterPushOK bool

func (p *AdventurePlugin) pushRoster() {
	snap, err := buildRosterSnapshot(time.Now().UTC(), p.euro)
	if err != nil {
		slog.Error("roster: build snapshot failed", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()

	if err := peteclient.PushRoster(ctx, snap); err != nil {
		// The failure itself is not alarming — the next tick retries by simply
		// being a fresher snapshot, and if we stay down Pete's board correctly
		// stops claiming to be live. Only the *transition* is worth a line.
		if rosterPushOK {
			slog.Warn("roster: push failed, board will go stale on Pete", "err", err)
		} else {
			slog.Debug("roster: push failed, dropping snapshot", "err", err)
		}
		rosterPushOK = false
		return
	}

	if !rosterPushOK {
		slog.Info("roster: board accepted by Pete — live adventurer board is publishing",
			"adventurers", len(snap.Adventurers))
		rosterPushOK = true
	}
}

// buildRosterSnapshot assembles the complete board.
//
// Complete is the contract: Pete *replaces* its board with this, so anyone we
// omit drops off the public page. That is exactly how the opt-out is enforced —
// an opted-out player is simply never in the payload, rather than being sent and
// anonymized. A standing row showing class + level + zone is trivially
// re-identifiable (there is one level-14 cleric), so "an adventurer" would have
// been a fig leaf; absence is the only honest option.
func buildRosterSnapshot(now time.Time, euro *EuroPlugin) (peteclient.RosterSnapshot, error) {
	snap := peteclient.RosterSnapshot{SnapshotAt: now.Unix(), Tiers: mischiefTierCatalog()}

	// Both DATETIME columns selected raw and folded in Go — NOT COALESCE()'d in
	// SQL. modernc.org/sqlite rebuilds a time.Time from the column's *declared*
	// type, and COALESCE() erases that affinity: the value comes back a string
	// and the Scan fails. Same trap playerIsIdle documents.
	rows, err := db.Get().Query(`
		SELECT user_id, last_player_action_at, created_at
		  FROM player_meta
		 WHERE alive = 1`)
	if err != nil {
		return snap, err
	}
	defer rows.Close()

	type player struct {
		uid        id.UserID
		lastAction *time.Time
	}
	var players []player
	for rows.Next() {
		var uid string
		var lastAction, created *time.Time
		if err := rows.Scan(&uid, &lastAction, &created); err != nil {
			return snap, err
		}
		if lastAction == nil {
			lastAction = created
		}
		players = append(players, player{id.UserID(uid), lastAction})
	}
	if err := rows.Err(); err != nil {
		return snap, err
	}

	for _, pl := range players {
		// The buyer's own advisory balance rides along in a separate keyspace,
		// keyed by localpart (their sign-in name), and is collected *before* the
		// opt-out skip: opt-out hides a player from the public board, but their own
		// balance is private on Pete — only ever read for the user asking about
		// themselves — so there is no reason to deny an opted-out player the
		// storefront's affordability hint. localpart is lowercase, matching how the
		// buyer signs in and how a web order's username is resolved back to an MXID.
		if euro != nil {
			if lp := localpartOf(pl.uid); lp != "" {
				snap.Balances = append(snap.Balances, peteclient.MischiefBalance{
					Username: lp,
					Euro:     euro.GetBalance(pl.uid),
				})
			}
		}

		if isNewsOptedOut(pl.uid) {
			continue
		}
		c, err := LoadDnDCharacter(pl.uid)
		if err != nil || c == nil || c.PendingSetup {
			continue // no character to show; a half-made one has no name yet
		}
		name := charName(pl.uid)
		if name == "" {
			continue // never fall back to a Matrix handle on a public page
		}

		e := peteclient.RosterEntry{
			// Stable per-player board token: salted, so it can't be recomputed
			// from the handle, and distinct from every event token, so the board
			// doesn't become the key that links a player's dispatches together.
			Token:     eventToken(pl.uid, "roster"),
			Name:      name,
			Level:     c.Level,
			ClassRace: classRaceLabel(c),
			Status:    "idle",
		}

		if exp, _ := getActiveExpedition(pl.uid); exp != nil {
			zone := zoneOrFallback(exp.ZoneID)
			e.Status = "expedition"
			e.Zone = zone.Display
			e.Day = exp.CurrentDay
			if IsMultiRegionZone(exp.ZoneID) {
				if r, ok := CurrentRegion(exp); ok {
					e.Region = r.Name
				}
			}
		} else if pl.lastAction != nil {
			if h := int(now.Sub(*pl.lastAction).Hours()); h > 0 {
				e.IdleHours = h
			}
		}
		snap.Adventurers = append(snap.Adventurers, e)
	}
	return snap, nil
}

// resolveRosterToken maps a board token back to the adventurer it names. The
// token is a one-way HMAC (eventToken), so it can't be inverted — instead we
// recompute every live player's token and match. The salt is DB-persisted, so a
// token minted before a restart still resolves after one. O(players) per call,
// which is nothing at realm scale and only runs when a web order is being placed.
func resolveRosterToken(token string) (id.UserID, bool) {
	if token == "" {
		return "", false
	}
	rows, err := db.Get().Query(`SELECT user_id FROM player_meta WHERE alive = 1`)
	if err != nil {
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		if eventToken(id.UserID(uid), "roster") == token {
			return id.UserID(uid), true
		}
	}
	return "", false
}

// mischiefTierCatalog is the storefront price list, pushed on every tick so the
// web shop always renders gogobee's current prices — a fee retune here reaches
// Pete within a snapshot, and Pete never hardcodes a number of its own. The
// signed fee is the same +25% sink a Matrix buyer pays to put their name on it.
func mischiefTierCatalog() []peteclient.MischiefTier {
	out := make([]peteclient.MischiefTier, 0, len(mischiefTiers))
	for _, t := range mischiefTiers {
		out = append(out, peteclient.MischiefTier{
			Key:       t.Key,
			Display:   t.Display,
			Fee:       t.Fee,
			SignedFee: mischiefSignedFee(t.Fee),
			Blurb:     t.Blurb,
		})
	}
	return out
}
