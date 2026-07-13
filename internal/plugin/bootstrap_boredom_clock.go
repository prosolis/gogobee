package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bootstrapBoredomClock seeds player_meta.last_player_action_at for rows that
// predate the column (gogobee_boredom_plan.md §1).
//
// Without this, the column is NULL for every existing player on the deploy that
// adds it, and loadBoredomCandidates falls back to created_at — which is when
// the character was *made*, not when it was last played. Every alive character
// on the server is therefore months past the idle threshold on the first tick
// after the deploy, and the whole player base gets marched into a dungeon at
// once, coins debited, thirty minutes after restart. A player who typed a
// command a minute before the deploy is no exception: the clock has never heard
// of them.
//
// last_active_at is the best proxy we have for "the character did something
// recently" and is exactly right at deploy time: recent for anyone still
// playing, stale for anyone who isn't. It is unusable as the clock itself (the
// autopilot bumps it through saveAdvCharacter, so a bored character would keep
// refreshing its own idle clock — see markPlayerAction), which is why this is a
// one-time seed and not a fallback in the query.
//
// Re-running on every boot is safe and intentionally a no-op:
//   - a row seeded here, or stamped by a real action, is no longer NULL;
//   - a character the boredom ticker has already sent out (last_boredom_at set)
//     is skipped, so a restart mid-boredom can't hand it a fresh idle clock off
//     the autopilot's own writes.
func bootstrapBoredomClock() {
	res, err := db.Get().Exec(`
		UPDATE player_meta
		   SET last_player_action_at = COALESCE(last_active_at, created_at)
		 WHERE last_player_action_at IS NULL
		   AND last_boredom_at IS NULL
		   AND COALESCE(last_active_at, created_at) IS NOT NULL`)
	if err != nil {
		slog.Error("bootstrap: boredom clock seed failed", "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Warn("bootstrap: seeded boredom idle clock from last_active_at", "rows", n)
	}
}
