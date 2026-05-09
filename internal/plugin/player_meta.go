package plugin

import (
	"database/sql"
	"log/slog"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// player_meta is the Adv 2.0 holding pen for non-stat per-user state
// migrating off adventure_characters (gogobee_legacy_migration.md §2.1).
// Phase L2 step 5 lands the table + arena counters; later phases extend
// the schema and add their own helpers here.

// PlayerMeta is the in-memory mirror of the player_meta row. Only the
// fields a phase has migrated are meaningful; unmigrated fields stay
// zero-valued and are sourced from AdvCharacter.
type PlayerMeta struct {
	UserID         id.UserID
	ArenaWins      int
	ArenaLosses    int
	InvasionScore  int
	DisplayName    string
	HospitalVisits int
}

// loadPlayerMeta reads the player_meta row for a user. Returns a
// zero-valued struct (with UserID set) when no row exists — callers
// should treat absence as "all fields zero", matching the column
// defaults.
func loadPlayerMeta(userID id.UserID) (*PlayerMeta, error) {
	m := &PlayerMeta{UserID: userID}
	err := db.Get().QueryRow(
		`SELECT arena_wins, arena_losses, invasion_score, display_name, hospital_visits FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&m.ArenaWins, &m.ArenaLosses, &m.InvasionScore, &m.DisplayName, &m.HospitalVisits)
	if err == sql.ErrNoRows {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// upsertPlayerMetaArena writes arena_wins / arena_losses / invasion_score
// for a user. Creates the row if missing. Used by the dual-write path
// during the L1–L4 migration (§11): every arena counter mutation goes
// through both AdvCharacter and player_meta until soak completes.
func upsertPlayerMetaArena(userID id.UserID, wins, losses, invasionScore int) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, arena_wins, arena_losses, invasion_score)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   arena_wins = excluded.arena_wins,
		   arena_losses = excluded.arena_losses,
		   invasion_score = excluded.invasion_score`,
		string(userID), wins, losses, invasionScore,
	)
	return err
}

// backfillPlayerMetaArena copies arena_wins / arena_losses / invasion_score
// from adventure_characters into player_meta for any user_id that doesn't
// already have a row. Idempotent: INSERT OR IGNORE skips users that have
// already been backfilled (or that wrote a row through the dual-write
// path post-deploy). Logs the row count touched, matching Phase R1's
// archiveOrphanZoneRuns precedent.
func backfillPlayerMetaArena() error {
	res, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id, arena_wins, arena_losses, invasion_score)
		SELECT user_id, arena_wins, arena_losses, invasion_score FROM adventure_characters
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: arena counters backfilled", "rows", n)
	return nil
}

// upsertPlayerMetaDisplayName writes display_name for a user, leaving other
// columns untouched. Used by the dual-write path during the L4f-prep
// DisplayName migration: every place that mutates AdvCharacter.DisplayName
// also calls this so player_meta.display_name stays in sync until soak
// completes and readers flip over (gogobee_legacy_migration.md §7).
func upsertPlayerMetaDisplayName(userID id.UserID, displayName string) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, display_name) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET display_name = excluded.display_name`,
		string(userID), displayName,
	)
	return err
}

// loadDisplayName returns the player's display name from player_meta when
// present, falling back to adventure_characters.display_name during the
// L4f-prep soak window. Empty string if the user has no row in either
// table. Callers should prefer this helper over reading char.DisplayName
// directly so the eventual reader flip is a no-op at the call site.
func loadDisplayName(userID id.UserID) (string, error) {
	var name string
	err := db.Get().QueryRow(
		`SELECT display_name FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&name)
	if err == nil && name != "" {
		return name, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	// Fallback during soak.
	err = db.Get().QueryRow(
		`SELECT display_name FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

// upsertPlayerMetaHospitalVisits writes hospital_visits for a user, leaving
// other columns untouched. Used by the dual-write path during the L4a
// Hospital migration: every increment on AdvCharacter.HospitalVisits also
// calls this so player_meta.hospital_visits stays in sync until soak
// completes (gogobee_legacy_migration.md §6.1, §11).
func upsertPlayerMetaHospitalVisits(userID id.UserID, visits int) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, hospital_visits) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET hospital_visits = excluded.hospital_visits`,
		string(userID), visits,
	)
	return err
}

// loadHospitalVisits returns the player's hospital visit count from
// player_meta when present, falling back to adventure_characters during
// the L4a soak window. Zero if the user has no row in either table.
func loadHospitalVisits(userID id.UserID) (int, error) {
	var visits int
	err := db.Get().QueryRow(
		`SELECT hospital_visits FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&visits)
	if err == nil && visits > 0 {
		return visits, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	// player_meta row missing or zero → fall back to AdvCharacter for the
	// soak window. A zero value in player_meta could legitimately mean "no
	// visits yet", but the fallback only returns non-zero if the legacy
	// column has a higher value, so this is safe either way.
	var legacy int
	err = db.Get().QueryRow(
		`SELECT hospital_visits FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacy)
	if err == sql.ErrNoRows {
		return visits, nil
	}
	if err != nil {
		return 0, err
	}
	if legacy > visits {
		return legacy, nil
	}
	return visits, nil
}

// backfillPlayerMetaHospitalVisits copies adventure_characters.hospital_visits
// into player_meta.hospital_visits for any row whose value is still the
// default zero. Idempotent: only updates rows that haven't been populated
// yet (via backfill or dual-write).
func backfillPlayerMetaHospitalVisits() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET hospital_visits = (
			SELECT hospital_visits FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE hospital_visits = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND adventure_characters.hospital_visits > 0
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: hospital_visits backfilled", "rows", n)
	return nil
}

// backfillPlayerMetaDisplayName copies adventure_characters.display_name
// into player_meta.display_name for any row whose display_name is still
// the empty default. Idempotent: safe to re-run; only updates rows that
// haven't been populated yet (either by backfill or the dual-write path).
func backfillPlayerMetaDisplayName() error {
	// Ensure a player_meta row exists for every adventure_characters user
	// (arena backfill already does this, but display_name backfill must
	// not assume order — re-run cheaply via INSERT OR IGNORE).
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET display_name = (
			SELECT display_name FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE display_name = ''
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND adventure_characters.display_name <> ''
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: display_name backfilled", "rows", n)
	return nil
}
