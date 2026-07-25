package plugin

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The Siege war room, pushed to Pete.
//
// The Siege is the only mechanic where the whole town works on one object, and
// until now it existed exclusively in Matrix — which means anybody not in the
// room at the time never knew it happened. A communal event nobody can see is a
// communal event that fails.
//
// It rides the roster ticker and follows the roster's rules exactly, because it
// is the same kind of thing: a snapshot of what is currently true, pushed whole,
// replacing whatever Pete had, dropped rather than retried on failure. A retried
// snapshot would be a lie about how much HP is left.
//
// The one place it deliberately departs from the board is the opt-out. The board
// omits an opted-out player entirely — a row showing class + level + zone is
// trivially re-identifiable, so absence is the only honest option there. Here a
// contributor is anonymised instead of dropped: their damage is part of what the
// town did to the boss, and a defender board that quietly deleted it would
// understate the shared effort and stop the numbers adding up. An opted-out
// player who has NOT fought is still omitted — there is nothing to account for,
// so naming their absence would be exposure for nothing.

// siegeHistoryLimit bounds the "sieges past" table. A Siege a month means this is
// years of history; the cap only exists so the payload can't grow without bound.
const siegeHistoryLimit = 24

// pushSiege builds and sends the war room. Mirrors pushRoster: transitions are
// logged, the steady state is silent.
var siegePushOK bool

func (p *AdventurePlugin) pushSiege() {
	snap, err := buildSiegeSnapshot(time.Now().UTC())
	if err != nil {
		slog.Error("siege: build snapshot failed", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rosterPushTimeout)
	defer cancel()

	if err := peteclient.PushSiege(ctx, snap); err != nil {
		if siegePushOK {
			slog.Warn("siege: push failed, war room will go stale on Pete", "err", err)
		} else {
			slog.Debug("siege: push failed, dropping snapshot", "err", err)
		}
		siegePushOK = false
		return
	}
	if !siegePushOK {
		slog.Info("siege: war room accepted by Pete", "active", snap.Active, "defenders", len(snap.Defenders))
		siegePushOK = true
	}
}

// buildSiegeSnapshot assembles the whole war room: the live boss, the muster,
// and the history.
func buildSiegeSnapshot(now time.Time) (peteclient.SiegeSnapshot, error) {
	snap := peteclient.SiegeSnapshot{SnapshotAt: now.Unix()}

	hist, err := loadResolvedWorldBosses(siegeHistoryLimit)
	if err != nil {
		return snap, err
	}
	snap.History = hist

	boss, err := loadActiveWorldBoss()
	if err != nil {
		return snap, err
	}
	if boss == nil {
		return snap, nil // no Siege camped: a real answer, not an empty snapshot
	}

	snap.Active = true
	snap.BossID = boss.ID
	snap.BossName = boss.Name
	snap.Tier = boss.Tier
	snap.HPCurrent = boss.HPCurrent
	snap.HPMax = boss.HPMax
	snap.StartsAt = boss.StartsAt.Unix()
	snap.EndsAt = boss.EndsAt.Unix()

	defenders, boutsToday, err := buildSiegeMuster(boss.ID, now)
	if err != nil {
		return snap, err
	}
	snap.Defenders = defenders
	snap.BoutsToday = boutsToday
	return snap, nil
}

// buildSiegeMuster returns every alive adventurer's standing against this boss,
// ranked, plus how many bouts have been taken today.
//
// It starts from the contribution rows rather than from the roster so a
// contributor who has since died (or whose player_meta row went away) still
// appears — the damage they did is on the boss whether they are standing or not.
// The alive roster is then folded in on top to produce the zero-fight rows that
// make the "bout still going spare" column exist.
func buildSiegeMuster(bossID int64, now time.Time) ([]peteclient.SiegeDefender, int, error) {
	contribs, err := loadWorldBossContribs(bossID)
	if err != nil {
		return nil, 0, err
	}
	today := now.Format("2006-01-02")

	byUser := make(map[id.UserID]worldBossContrib, len(contribs))
	for _, c := range contribs {
		byUser[c.UserID] = c
	}

	// Everyone alive, so the un-fought have a row to stand in.
	rows, err := db.Get().Query(`SELECT user_id FROM player_meta WHERE alive = 1`)
	if err != nil {
		return nil, 0, err
	}
	order := make([]id.UserID, 0, len(contribs))
	seen := make(map[id.UserID]bool, len(contribs))
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return nil, 0, err
		}
		u := id.UserID(uid)
		if !seen[u] {
			seen[u] = true
			order = append(order, u)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// Contributors who are no longer on the alive roster still owe the board a row.
	for _, c := range contribs {
		if !seen[c.UserID] {
			seen[c.UserID] = true
			order = append(order, c.UserID)
		}
	}

	boutsToday := 0
	out := make([]peteclient.SiegeDefender, 0, len(order))
	for _, uid := range order {
		c, fought := byUser[uid]
		if fought && c.LastFightDate == today {
			boutsToday++
		}
		optedOut := isNewsOptedOut(uid)
		if optedOut && !fought {
			continue // nothing to account for; naming the absence is exposure for nothing
		}

		d := peteclient.SiegeDefender{Name: anonName}
		if fought {
			d.Fights = c.Fights
			d.Damage = c.Damage
			d.FoughtToday = c.LastFightDate == today
		}
		if !optedOut {
			name := charName(uid)
			if name == "" {
				// No character name means no honest way to render the row: never fall
				// back to a Matrix handle on a public page. A contributor in this state
				// keeps their damage, anonymously; a non-contributor is simply dropped.
				if !fought {
					continue
				}
			} else {
				d.Name = name
				d.Token = eventToken(uid, "roster")
				if ch, err := LoadDnDCharacter(uid); err == nil && ch != nil && !ch.PendingSetup {
					d.Level = ch.Level
				}
			}
		}
		out = append(out, d)
	}

	// Rank: damage, then bouts, then name. Deterministic to the last key so an
	// unchanged muster produces a byte-identical snapshot and Pete's board doesn't
	// reshuffle itself every two minutes.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Damage != out[j].Damage {
			return out[i].Damage > out[j].Damage
		}
		if out[i].Fights != out[j].Fights {
			return out[i].Fights > out[j].Fights
		}
		return out[i].Name < out[j].Name
	})
	return out, boutsToday, nil
}

// loadResolvedWorldBosses reads the closed-out Sieges, newest first, each with
// its defender count and the contributor who turned up most.
func loadResolvedWorldBosses(limit int) ([]peteclient.SiegePast, error) {
	// resolved_at is selected raw and folded in Go, never COALESCE()'d in SQL:
	// modernc.org/sqlite rebuilds a time.Time from the column's DECLARED type and
	// COALESCE erases that affinity, so the Scan would fail. Same trap
	// buildRosterSnapshot documents.
	rows, err := db.Get().Query(`
		SELECT id, name, tier, hp_max, hp_current, status, resolved_at, ends_at
		  FROM world_boss
		 WHERE status IN ('defeated', 'survived')
		 ORDER BY id DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []peteclient.SiegePast
	for rows.Next() {
		var (
			h                  peteclient.SiegePast
			resolvedAt, endsAt *time.Time
		)
		if err := rows.Scan(&h.BossID, &h.BossName, &h.Tier, &h.HPMax, &h.HPRemaining,
			&h.Outcome, &resolvedAt, &endsAt); err != nil {
			return nil, err
		}
		// A boss resolved by the ticker has resolved_at; a legacy row might not.
		// The window's close is the honest fallback — it is when the Siege ended
		// either way, and it is never null.
		switch {
		case resolvedAt != nil:
			h.EndedAt = resolvedAt.Unix()
		case endsAt != nil:
			h.EndedAt = endsAt.Unix()
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		n, mvp, fights, err := worldBossMuster(out[i].BossID)
		if err != nil {
			slog.Warn("siege: history muster load failed", "boss", out[i].BossID, "err", err)
			continue
		}
		out[i].Defenders = n
		out[i].MVP = mvp
		out[i].MVPFights = fights
	}
	return out, nil
}

// worldBossMuster reports how many people fought a boss and who fought it most.
// The MVP is by fights, not damage — the same accessibility call the payout
// split makes (computeWorldBossPayouts): turning up is the contribution the
// mechanic actually asks for. An opted-out MVP is anonymised, never dropped;
// the count behind the name is a fact about the town.
func worldBossMuster(bossID int64) (defenders int, mvp string, mvpFights int, err error) {
	contribs, err := loadWorldBossContribs(bossID)
	if err != nil {
		return 0, "", 0, err
	}
	// loadWorldBossContribs already orders by fights desc, damage desc, so the
	// first row with any fights at all is the MVP.
	for _, c := range contribs {
		if c.Fights <= 0 {
			continue
		}
		defenders++
		if mvp == "" {
			mvp = anonName
			if !isNewsOptedOut(c.UserID) {
				if name := charName(c.UserID); name != "" {
					mvp = name
				}
			}
			mvpFights = c.Fights
		}
	}
	return defenders, mvp, mvpFights, nil
}

// ── Dispatches ───────────────────────────────────────────────────────────────

// emitSiegeStart files the "a boss is at the gates" dispatch. PRIORITY: this is
// the one beat where the right response is for everyone to look up right now,
// and unlike a zone clear it is not something TwinBee has already announced to
// the same people — the games-room shout and this go to different rooms.
//
// Realm-level, so there is no subject player and no opt-out to apply.
func emitSiegeStart(boss *worldBossState) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	emitFact(peteclient.Fact{
		GUID:       siegeGUID("siege_start", boss.ID),
		EventType:  "siege_start",
		Tier:       "priority",
		Boss:       boss.Name,
		Level:      boss.Tier,
		Stakes:     siegeWindowPhrase(boss),
		OccurredAt: boss.StartsAt.Unix(),
	}, "", "")
}

// emitSiegeWin files the "the town held" dispatch. Count is the number of
// defenders, which is what Pete's template reads to say how many stood.
func emitSiegeWin(boss *worldBossState, defenders int) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	emitFact(peteclient.Fact{
		GUID:       siegeGUID("siege_win", boss.ID),
		EventType:  "siege_win",
		Tier:       "priority",
		Boss:       boss.Name,
		Level:      boss.Tier,
		Count:      defenders,
		Outcome:    "defeated",
		OccurredAt: nowUnix(),
	}, "", "")
}

// emitSiegeLoss files the "it broke through" dispatch.
func emitSiegeLoss(boss *worldBossState) {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	emitFact(peteclient.Fact{
		GUID:       siegeGUID("siege_loss", boss.ID),
		EventType:  "siege_loss",
		Tier:       "priority",
		Boss:       boss.Name,
		Level:      boss.Tier,
		Outcome:    "survived",
		OccurredAt: nowUnix(),
	}, "", "")
}

// siegeGUID keys a Siege dispatch on the boss row id, which is unique and stable
// for the life of the event. That makes each of the three beats fire at most
// once per Siege however many times its resolution path is re-entered — the
// status guard in setWorldBossStatus already dedupes the payout, and this dedupes
// the news the same way.
func siegeGUID(eventType string, bossID int64) string {
	return eventType + ":" + strconv.FormatInt(bossID, 10)
}

// siegeWindowPhrase is the deadline as Pete's siege_start template wants it —
// "You've got %s" — so it must read as a duration, not a timestamp.
func siegeWindowPhrase(boss *worldBossState) string {
	h := int(boss.EndsAt.Sub(boss.StartsAt).Hours())
	switch {
	case h <= 0:
		return "no time at all"
	case h == 24:
		return "a day"
	case h%24 == 0:
		return strconv.Itoa(h/24) + " days"
	default:
		return strconv.Itoa(h) + " hours"
	}
}
