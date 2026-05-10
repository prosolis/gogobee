package plugin

import (
	"log/slog"
	"os"
	"strings"

	"gogobee/internal/db"
)

// purgeLegacyZoneRunColumns drops the dnd_zone_run.current_room and
// dnd_zone_run.room_seq_json columns when the operator opts in via
// GOGOBEE_BRANCHING_PURGE=1. Phase G9 close-out: G9a removed the
// runtime gate, G9b stopped persisting the columns, and this is the
// final step.
//
// Idempotent: skips silently when the columns are already gone (or
// never existed on a fresh install — the schema in db.go no longer
// creates them).
func purgeLegacyZoneRunColumns() {
	if os.Getenv("GOGOBEE_BRANCHING_PURGE") != "1" {
		return
	}
	d := db.Get()
	for _, col := range []string{"current_room", "room_seq_json"} {
		if _, err := d.Exec("ALTER TABLE dnd_zone_run DROP COLUMN " + col); err != nil {
			msg := err.Error()
			// "no such column" → already dropped (or fresh install).
			if strings.Contains(msg, "no such column") {
				continue
			}
			slog.Error("G9 purge: drop column failed", "col", col, "err", err)
			return
		}
		slog.Warn("G9 purge: dnd_zone_run column dropped (GOGOBEE_BRANCHING_PURGE=1)", "col", col)
	}
}
