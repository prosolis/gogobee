package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bootstrapPhase5BHPRefresh recomputes hp_max for every dnd_character
// row after the Phase 5-B HP multiplier (phase5BHPMult in dnd.go)
// shipped. Without this, existing characters keep their pre-Phase-5-B
// hp_max until something else recomputes it (level-up, character reset,
// migration), so the durability lift wouldn't reach live players for
// weeks. See gogobee_expedition_difficulty.md Phase 5-B.
//
// Pattern mirrors bumpDexFloorForExistingCharacters: enumerate, derive
// new value, update only rows where the new value differs. hp_current
// is bumped by the same delta so a full-HP character stays at full HP
// after the bump; a wounded character keeps the same absolute wound
// (e.g. -7 HP) so the lift only raises the *ceiling*, not the present
// HP — players don't get a free heal on top of a permanent buff.
//
// Idempotent via the db.JobCompleted gate. The job key is bumped if we
// ever ship a second HP-curve change so the migration runs once per
// shipped change.
func bootstrapPhase5BHPRefresh() {
	const jobName = "phase5b_hp_refresh_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}

	d := db.Get()
	rows, err := d.Query(`
		SELECT user_id, class, con_score, dnd_level, hp_max, hp_current
		FROM dnd_character WHERE dnd_level > 0`)
	if err != nil {
		slog.Error("phase5b hp refresh: enumerate failed", "err", err)
		return
	}
	defer rows.Close()

	type row struct {
		userID    string
		class     DnDClass
		conScore  int
		level     int
		hpMax     int
		hpCurrent int
	}
	var batch []row
	for rows.Next() {
		var r row
		var classStr string
		if err := rows.Scan(&r.userID, &classStr, &r.conScore, &r.level, &r.hpMax, &r.hpCurrent); err != nil {
			slog.Warn("phase5b hp refresh: scan failed", "err", err)
			continue
		}
		r.class = DnDClass(classStr)
		batch = append(batch, r)
	}

	refreshed := 0
	for _, r := range batch {
		conMod := abilityModifier(r.conScore)
		newMax := computeMaxHP(r.class, conMod, r.level)
		if newMax <= r.hpMax {
			// Either already at/above target (re-runs, or the row was
			// hand-edited above formula), or the formula change reduced
			// HP for this class — leave it alone in that case so we
			// never *lower* a player's HP via bootstrap.
			continue
		}
		delta := newMax - r.hpMax
		newCurrent := r.hpCurrent + delta
		if newCurrent > newMax {
			newCurrent = newMax
		}
		if newCurrent < 1 {
			newCurrent = 1
		}
		if _, err := d.Exec(`UPDATE dnd_character
			SET hp_max = ?, hp_current = ?, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = ?`,
			newMax, newCurrent, r.userID); err != nil {
			slog.Warn("phase5b hp refresh: update failed", "user", r.userID, "err", err)
			continue
		}
		refreshed++
	}

	db.MarkJobCompleted(jobName, "once")
	if refreshed > 0 {
		slog.Info("phase5b hp refresh: refreshed character HPMax to Phase 5-B floor", "count", refreshed)
	}
}
