package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bootstrapRoomsTraversed backfills dnd_zone_run.rooms_traversed for rows
// that predate the revisit R1 column. Before R1 the step counter was implicit
// — len(visited_nodes) — because navigation was forward-only, so replaying
// that identity is an exact reconstruction, not an estimate.
//
// The ALTER lands DEFAULT 0, which would tell an in-flight run it has walked
// nowhere: ambient narration cadence would reset to the entry-room line and
// R2's revisit preflight would read a fresh run. Hence the backfill.
//
// Idempotent, and safe to run against live rows: every row written after R1
// inserts rooms_traversed >= 1 (the entry node counts as one traversal), so
// `rooms_traversed = 0` uniquely identifies a row this has not yet touched.
// Kept in-tree permanently — a fresh deploy restoring an old backup needs it
// (feedback_loader_rewire_needs_bootstrap).
func bootstrapRoomsTraversed() {
	res, err := db.Get().Exec(`
		UPDATE dnd_zone_run
		   SET rooms_traversed = MAX(json_array_length(visited_nodes), 1)
		 WHERE rooms_traversed = 0`)
	if err != nil {
		slog.Error("bootstrap: rooms_traversed backfill failed", "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Warn("bootstrap: rooms_traversed backfilled from visited_nodes", "rows", n)
	}
}
