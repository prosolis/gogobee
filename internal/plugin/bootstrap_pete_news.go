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

// backfillZoneFirsts seeds news_realm_firsts from history and emits one PRIORITY
// realm-first dispatch per zone, attributed to its earliest boss-defeating
// clearer. SQLite returns the user_id from the same row as MIN(completed_at)
// (bare-column min/max rule), so the (zone, first clearer, time) triple is
// consistent. Returns the count emitted.
func (p *AdventurePlugin) backfillZoneFirsts() int {
	rows, err := db.Get().Query(
		`SELECT zone_id, user_id, MIN(completed_at)
		   FROM dnd_zone_run
		  WHERE boss_defeated = 1 AND completed_at IS NOT NULL AND abandoned = 0
		  GROUP BY zone_id`)
	if err != nil {
		slog.Error("backfill: zone-firsts query", "err", err)
		return 0
	}
	type first struct {
		zoneID, userID, completedAt string
	}
	var firsts []first
	for rows.Next() {
		var f first
		if err := rows.Scan(&f.zoneID, &f.userID, &f.completedAt); err != nil {
			slog.Error("backfill: zone-firsts scan", "err", err)
			continue
		}
		firsts = append(firsts, f)
	}
	rows.Close()

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
		lvl := 0
		if dc, _ := LoadDnDCharacter(uid); dc != nil {
			lvl = dc.Level
		}
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("zone:%s:%s:%d", userHash(uid), f.zoneID, ts.Unix()),
			EventType:  "zone_first",
			Tier:       "priority",
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
		lvl := 0
		if dc, _ := LoadDnDCharacter(uid); dc != nil {
			lvl = dc.Level
		}
		emitFact(peteclient.Fact{
			GUID:       fmt.Sprintf("death:%s:%s", userHash(uid), d.date),
			EventType:  "death",
			Tier:       "priority",
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
			GUID:       fmt.Sprintf("achv:%s:%s", userHash(uid), s.achID),
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
