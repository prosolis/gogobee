package plugin

import (
	"log/slog"
	"os"

	"gogobee/internal/db"
)

// purgeLegacyAdvCharacterTable drops the `adventure_characters` table when
// the operator opts in via GOGOBEE_LEGACY_PURGE=1. Phase L5 close-out: the
// table has been frozen since L5h (saveAdvCharacter fans out into player_meta
// only), and the loader rewire moved every read off it. This is the final
// step.
//
// The pre-D&D rollback snapshot table (`adventure_characters_pre_dnd`) is
// preserved — it's a separate insurance copy and a future
// `GOGOBEE_PRE_DND_PURGE=1` pass can drop it once the soak window expires.
//
// Idempotent: skips silently if the table is already gone (or never existed
// on a fresh install).
func purgeLegacyAdvCharacterTable() {
	if os.Getenv("GOGOBEE_LEGACY_PURGE") != "1" {
		return
	}
	d := db.Get()
	var name string
	err := d.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='adventure_characters'`,
	).Scan(&name)
	if err != nil {
		// sql.ErrNoRows or any scan error → nothing to drop.
		return
	}
	if _, err := d.Exec(`DROP TABLE adventure_characters`); err != nil {
		slog.Error("L5 purge: drop adventure_characters failed", "err", err)
		return
	}
	slog.Warn("L5 purge: adventure_characters table dropped (GOGOBEE_LEGACY_PURGE=1)")
}
