package plugin

import (
	"fmt"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// bootstrapPeteNewsBackfill replays the back-catalogue through the news emit
// path the first time the seam comes up enabled, so a launch doesn't open onto
// an empty section. It covers the three tractable, high-signal sources:
//
//  1. Realm-first zone clears — the earliest boss-defeated run of each zone. This
//     also SEEDS news_realm_firsts, so a later live clear of an already-cleared
//     zone tiers as a BULLETIN repeat, not a mis-announced "first ever".
//  2. Deaths — every player_meta row that carries a death (graveyard content).
//  3. Single-holder achievements — rarity-gated to one holder, matching the live
//     milestone rule so routine unlocks don't flood the backlog.
//
// Every fact here is a BULLETIN, like every fact gogobee files (1cbd68a). Priority
// is the one tier that makes Pete post a live Matrix beat, and TwinBee already
// narrates these moments in the games room as they happen — a priority fact would
// have Pete read his version back to the people who watched it. Nothing in the
// back catalogue is news, least of all a death from three weeks ago.
//
// NoPush independently short-circuits Pete's beat today, so this is belt AND
// braces: the tier is the rule, and it must not depend on NoPush staying in front
// of it. Rendering keys off event_type, not tier, so nothing on the site moves.
//
// Backfilled facts are backdated to when they happened and marked NoPush (no
// web-push spam on launch). GUIDs are deterministic (keyed on the historical
// timestamp), so a re-run — or a later live emit of the same event — dedups on
// Pete's side. Guarded one-shot; kept (never deleted at close-out) so a fresh
// deploy doesn't ghost-town again. See pete_adventure_news_plan.md gap #7.
func (p *AdventurePlugin) bootstrapPeteNewsBackfill() {
	const jobName = "pete_news_backfill_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}
	// The backfill IS the launch content, so it should fire the first boot the
	// seam is actually live — not get silently consumed on a boot where emission
	// is off. Leave the job unmarked until both switches are on.
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}

	firsts := p.backfillZoneFirsts()
	deaths := backfillDeaths()
	achv := p.backfillSoloAchievements()

	db.MarkJobCompleted(jobName, "once")
	slog.Warn("bootstrap: pete news backfill complete",
		"zone_firsts", firsts, "deaths", deaths, "achievements", achv)
}

// zoneFirstClear is one zone's earliest boss kill: who did it and when.
type zoneFirstClear struct {
	zoneID, userID, completedAt string
}

// zoneFirstClears reads the earliest boss-defeating run of every zone.
//
// The filter is `boss_defeated = 1` and NOTHING else, and that is the whole
// point. `abandoned` does not mean anybody gave up — abandonZoneRunByID exists
// to retire a run whose boss is ALREADY DEAD when the expedition travels onward
// (dnd_zone_run.go), so in prod 30 of 32 boss kills carry abandoned = 1. An
// `AND abandoned = 0` here drew a realm where 2 zones had ever been beaten
// instead of 9. It is the same filter loadRealmClearStats documents; do not
// reintroduce it.
//
// SQLite returns the user_id from the same row as MIN(completed_at) (bare-column
// min/max rule), so the (zone, first clearer, time) triple is internally
// consistent.
func zoneFirstClears() []zoneFirstClear {
	rows, err := db.Get().Query(
		`SELECT zone_id, user_id, MIN(completed_at)
		   FROM dnd_zone_run
		  WHERE boss_defeated = 1 AND completed_at IS NOT NULL
		  GROUP BY zone_id`)
	if err != nil {
		slog.Error("backfill: zone-firsts query", "err", err)
		return nil
	}
	defer rows.Close()

	var firsts []zoneFirstClear
	for rows.Next() {
		var f zoneFirstClear
		if err := rows.Scan(&f.zoneID, &f.userID, &f.completedAt); err != nil {
			slog.Error("backfill: zone-firsts scan", "err", err)
			continue
		}
		firsts = append(firsts, f)
	}
	return firsts
}

// bootstrapRealmFirstsReseed repairs the zone half of news_realm_firsts.
//
// The ledger is what claimRealmFirst tiers live dispatches against, so a zone
// whose first clear the original one-shot missed is a spurious PRIORITY "realm
// first" waiting to fire the next time somebody clears it, months after the
// fact. Two things were wrong with what got seeded:
//
//  1. The `abandoned = 0` filter above, which is why prod holds 6 zones where
//     the run history knows 9.
//  2. first_at is claimRealmFirst's unixepoch() — when the claim was RECORDED,
//     not when the clear happened. Every backfilled prod row carries the one
//     minute the job ran.
//
// This is a re-SEED, not a re-run: it writes the ledger and emits nothing at
// all, so no historical realm-first dispatch reaches the room. It has its own
// job name because the original one-shot's gate is already marked, and per
// feedback_loader_rewire_needs_bootstrap it stays in place afterwards — a fresh
// deploy runs it as an ordinary bootstrap.
//
// It runs unconditionally on the news seam's switches, unlike the backfill: a
// ledger that is correct only when emission happens to be on is a ledger that
// mis-tiers the first dispatch after somebody flips it.
//
// A zone claim with no surviving run behind it is left exactly as it is. The run
// history is the better record of both who and when, but only where it has one.
func bootstrapRealmFirstsReseed() {
	const jobName = "pete_realm_firsts_reseed_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}

	seeded := 0
	for _, f := range zoneFirstClears() {
		ts, ok := parseSQLiteTime(f.completedAt)
		if !ok {
			slog.Warn("reseed: unparseable clear time", "zone", f.zoneID, "at", f.completedAt)
			continue
		}
		// Upsert, not INSERT OR IGNORE: the six rows that already exist carry the
		// wrong date and correcting them is half of what this job is for.
		db.Exec("realm-firsts reseed",
			`INSERT INTO news_realm_firsts (kind, target, first_at) VALUES ('zone', ?, ?)
			 ON CONFLICT(kind, target) DO UPDATE SET first_at = excluded.first_at`,
			f.zoneID, ts.Unix())
		seeded++
	}

	db.MarkJobCompleted(jobName, "once")
	slog.Warn("bootstrap: realm-firsts ledger reseeded", "zones", seeded)
}

// backfillZoneFirsts seeds news_realm_firsts from history and emits one
// dispatch per zone, attributed to its earliest boss-defeating clearer.
// Returns the count emitted.
func (p *AdventurePlugin) backfillZoneFirsts() int {
	firsts := zoneFirstClears()

	n := 0
	for _, f := range firsts {
		uid := id.UserID(f.userID)
		// Seed the ledger regardless of whether we can render a dispatch, so the
		// realm-first tiering is correct even for an unnamed straggler.
		claimRealmFirst("zone", f.zoneID)

		name := charName(uid)
		if name == "" {
			continue
		}
		ts, ok := parseSQLiteTime(f.completedAt)
		if !ok {
			continue
		}
		zone := zoneOrFallback(ZoneID(f.zoneID))
		lvl := charLevel(uid)
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("zone_first:%s:%s:%d", eventToken(uid, fmt.Sprintf("%s:%d", f.zoneID, ts.Unix())), f.zoneID, ts.Unix()),
			EventType:  "zone_first",
			Tier:       "bulletin",
			Subject:    name,
			Zone:       zone.Display,
			Boss:       zone.Boss.Name,
			Level:      lvl,
			Outcome:    "cleared",
			OccurredAt: ts.Unix(),
			NoPush:     true,
		}, uid, "")
		n++
	}
	return n
}

// backfillDeaths emits one death dispatch per player_meta death record. Deaths
// are day-granular (last_death_date), so the dispatch is backdated to that day.
func backfillDeaths() int {
	rows, err := db.Get().Query(
		`SELECT user_id, last_death_date, death_location
		   FROM player_meta
		  WHERE last_death_date != ''`)
	if err != nil {
		slog.Error("backfill: deaths query", "err", err)
		return 0
	}
	type death struct {
		userID, date, location string
	}
	var deaths []death
	for rows.Next() {
		var d death
		if err := rows.Scan(&d.userID, &d.date, &d.location); err != nil {
			slog.Error("backfill: deaths scan", "err", err)
			continue
		}
		deaths = append(deaths, d)
	}
	rows.Close()

	n := 0
	for _, d := range deaths {
		uid := id.UserID(d.userID)
		name := charName(uid)
		if name == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", d.date)
		if err != nil {
			continue
		}
		lvl := charLevel(uid)
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("death:%s:%s", eventToken(uid, d.date), d.date),
			EventType:  "death",
			Tier:       "bulletin",
			Subject:    name,
			Zone:       d.location,
			Level:      lvl,
			Outcome:    "lost",
			OccurredAt: day.UTC().Unix(),
			NoPush:     true,
		}, uid, "")
		n++
	}
	return n
}

// backfillSoloAchievements emits a milestone dispatch for every achievement held
// by exactly one player — the same rarity gate the live path uses — so the
// backlog carries the genuinely distinctive unlocks, not routine ones.
func (p *AdventurePlugin) backfillSoloAchievements() int {
	rows, err := db.Get().Query(
		`SELECT achievement_id, user_id, unlocked_at
		   FROM achievements
		  GROUP BY achievement_id
		 HAVING COUNT(*) = 1`)
	if err != nil {
		slog.Error("backfill: achievements query", "err", err)
		return 0
	}
	type solo struct {
		achID, userID string
		unlockedAt    int64
	}
	var solos []solo
	for rows.Next() {
		var s solo
		if err := rows.Scan(&s.achID, &s.userID, &s.unlockedAt); err != nil {
			slog.Error("backfill: achievements scan", "err", err)
			continue
		}
		solos = append(solos, s)
	}
	rows.Close()

	n := 0
	for _, s := range solos {
		uid := id.UserID(s.userID)
		name := charName(uid)
		if name == "" {
			continue
		}
		label := s.achID
		if p.achievements != nil {
			for _, a := range p.achievements.achievements {
				if a.ID == s.achID {
					label = a.Name
					break
				}
			}
		}
		ts := s.unlockedAt
		if ts == 0 {
			ts = nowUnix()
		}
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("milestone:%s:%s", eventToken(uid, s.achID), s.achID),
			EventType:  "milestone",
			Tier:       "bulletin",
			Subject:    name,
			Milestone:  label,
			OccurredAt: ts,
			NoPush:     true,
		}, uid, "")
		n++
	}
	return n
}
