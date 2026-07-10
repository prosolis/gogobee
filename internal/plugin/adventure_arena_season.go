package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Arena seasons ───────────────────────────────────────────────────────────
//
// Quarterly standings (gogobee_engagement_plan.md C4). The plan called for a
// quarterly *reset* of arena_stats; this derives season standings from
// arena_history.created_at instead. Same player-visible effect — the board
// clears every quarter — but lifetime totals survive for `!arena stats`, and
// there is no destructive job that can fire twice or half-way.
//
// Season titles are archived to their own table rather than player_meta.title:
// that column already carries the Survivalist milestone, and a season champion
// overwriting someone's expedition title would silently destroy it.

// arenaSeasonTitleKinds are the two crowns awarded per season.
const (
	arenaTitleEarnings = "earnings"
	arenaTitleStreak   = "streak"
)

// arenaSeasonKey names the quarter containing t, e.g. "2026-Q3".
func arenaSeasonKey(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%d-Q%d", t.Year(), (int(t.Month())-1)/3+1)
}

// arenaSeasonStart is the first instant of the quarter containing t.
func arenaSeasonStart(t time.Time) time.Time {
	t = t.UTC()
	firstMonth := time.Month(((int(t.Month())-1)/3)*3 + 1)
	return time.Date(t.Year(), firstMonth, 1, 0, 0, 0, 0, time.UTC)
}

// arenaSeasonBounds returns [start, end) for the quarter containing t.
func arenaSeasonBounds(t time.Time) (time.Time, time.Time) {
	start := arenaSeasonStart(t)
	return start, start.AddDate(0, 3, 0)
}

// previousArenaSeason returns the key and bounds of the quarter before t's.
func previousArenaSeason(t time.Time) (string, time.Time, time.Time) {
	prev := arenaSeasonStart(t).AddDate(0, -1, 0) // any instant inside the prior quarter
	start, end := arenaSeasonBounds(prev)
	return arenaSeasonKey(prev), start, end
}

// loadArenaSeasonLeaderboard aggregates arena_history over [start, end) into
// the same shape the lifetime board renders.
func loadArenaSeasonLeaderboard(start, end time.Time) ([]ArenaLeaderboardEntry, error) {
	rows, err := db.Get().Query(`
		SELECT h.user_id, COALESCE(c.display_name, h.user_id),
		       SUM(h.earnings),
		       MAX(h.tier),
		       SUM(CASE WHEN h.tier = 5 AND h.outcome = 'completed' THEN 1 ELSE 0 END),
		       COUNT(*),
		       SUM(CASE WHEN h.outcome = 'dead' THEN 1 ELSE 0 END)
		FROM arena_history h
		LEFT JOIN player_meta c ON c.user_id = h.user_id
		WHERE h.created_at >= ? AND h.created_at < ?
		GROUP BY h.user_id
		ORDER BY SUM(h.earnings) DESC
		LIMIT 10`, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ArenaLeaderboardEntry
	for rows.Next() {
		var e ArenaLeaderboardEntry
		var uid string
		if err := rows.Scan(&uid, &e.DisplayName, &e.TotalEarnings, &e.HighestTier,
			&e.Tier5Completions, &e.TotalRuns, &e.TotalDeaths); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// arenaSeasonChampion finds the single top row for a season by the given
// metric. Returns ok=false when nobody entered the arena that quarter.
func arenaSeasonChampion(kind string, start, end time.Time) (id.UserID, int64, bool) {
	var metric string
	switch kind {
	case arenaTitleEarnings:
		metric = "SUM(earnings)"
	case arenaTitleStreak:
		metric = "MAX(rounds_survived)"
	default:
		return "", 0, false
	}

	// Only runs that earned or survived something can crown anyone: a season of
	// nothing but deaths at round zero should award no streak title.
	q := fmt.Sprintf(`
		SELECT user_id, %s AS metric
		FROM arena_history
		WHERE created_at >= ? AND created_at < ?
		GROUP BY user_id
		HAVING metric > 0
		ORDER BY metric DESC, user_id ASC
		LIMIT 1`, metric)

	var uid string
	var value int64
	err := db.Get().QueryRow(q, start.Unix(), end.Unix()).Scan(&uid, &value)
	if err == sql.ErrNoRows {
		return "", 0, false
	}
	if err != nil {
		slog.Error("arena season: champion query", "kind", kind, "err", err)
		return "", 0, false
	}
	return id.UserID(uid), value, true
}

// recordArenaSeasonTitle archives a crown. Idempotent on (season, kind).
func recordArenaSeasonTitle(season, kind string, userID id.UserID, value int64, at time.Time) error {
	_, err := db.Get().Exec(`
		INSERT INTO arena_season_titles (season, kind, user_id, value, awarded_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(season, kind) DO NOTHING`,
		season, kind, string(userID), value, at.Unix())
	return err
}

// loadArenaSeasonTitles returns every crown a player has ever taken.
func loadArenaSeasonTitles(userID id.UserID) ([]string, error) {
	rows, err := db.Get().Query(`
		SELECT season, kind FROM arena_season_titles
		WHERE user_id = ? ORDER BY season DESC`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var season, kind string
		if err := rows.Scan(&season, &kind); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%s %s", season, arenaTitleName(kind)))
	}
	return out, rows.Err()
}

func arenaTitleName(kind string) string {
	switch kind {
	case arenaTitleEarnings:
		return "Coinlord of the Arena"
	case arenaTitleStreak:
		return "Longest Walk"
	}
	return kind
}

// ── Rollover ────────────────────────────────────────────────────────────────

// arenaSeasonRollover awards the previous season's crowns exactly once, and
// announces them. Safe to call every midnight: JobCompleted keyed on the season
// makes it a no-op for the rest of the quarter, and a bot that was down on the
// rollover day still catches up the next time it wakes.
func (p *AdventurePlugin) arenaSeasonRollover(now time.Time) {
	season, start, end := previousArenaSeason(now)
	jobName := "arena_season_rollover"
	if db.JobCompleted(jobName, season) {
		return
	}
	// Guard against awarding a season that hasn't finished yet — only possible
	// if a caller passes a doctored clock.
	if !now.UTC().After(end) {
		return
	}

	var lines []string
	failed := false
	for _, kind := range []string{arenaTitleEarnings, arenaTitleStreak} {
		uid, value, ok := arenaSeasonChampion(kind, start, end)
		if !ok {
			continue
		}
		if err := recordArenaSeasonTitle(season, kind, uid, value, now); err != nil {
			slog.Error("arena season: record title failed", "season", season, "kind", kind, "err", err)
			failed = true
			continue // don't announce a crown we failed to persist
		}
		name, _ := loadDisplayName(uid)
		switch kind {
		case arenaTitleEarnings:
			lines = append(lines, fmt.Sprintf("🥇 **%s** — _%s_ (€%d earned)",
				name, arenaTitleName(kind), value))
		case arenaTitleStreak:
			lines = append(lines, fmt.Sprintf("🏃 **%s** — _%s_ (%d rounds in one run)",
				name, arenaTitleName(kind), value))
		}
	}

	// Marking the job done is what stops the next midnight from retrying, so a
	// crown we failed to persist must not mark it. recordArenaSeasonTitle is
	// idempotent on (season, kind), and a past season's data is frozen, so the
	// retry re-derives the same champions and no-ops the ones already stored.
	if failed {
		slog.Warn("arena season: deferring completion after title failure", "season", season)
		return
	}
	db.MarkJobCompleted(jobName, season)
	if len(lines) == 0 {
		slog.Info("arena season: closed with no entrants", "season", season)
		return
	}
	slog.Info("arena season: crowned", "season", season, "titles", len(lines))

	gr := gamesRoom()
	if gr == "" {
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚔️ **Arena Season %s has ended.**\n\n", season))
	sb.WriteString(strings.Join(lines, "\n"))
	sb.WriteString("\n\nThe board is clear. Season " + arenaSeasonKey(now) + " starts now.")
	p.SendMessage(id.RoomID(gr), sb.String())
}
