package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bootstrapCasterHPRefresh recomputes hp_max for caster characters after
// the J3 D8-d-fix caster HP multiplier (casterHPMult in dnd.go) shipped.
// Without this, existing caster rows keep their pre-lift hp_max until
// the next computeMaxHP recall (level-up / reset). Mirrors
// bootstrapPhase5BHPRefresh — only ever raises hp_max, preserves the
// absolute wound (delta added to hp_current, capped at new max). Run
// once per startup, idempotent via db.JobCompleted.
func bootstrapCasterHPRefresh() {
	const jobName = "caster_hp_refresh_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}

	d := db.Get()
	rows, err := d.Query(`
		SELECT user_id, class, con_score, dnd_level, hp_max, hp_current
		FROM dnd_character WHERE dnd_level > 0`)
	if err != nil {
		slog.Error("caster hp refresh: enumerate failed", "err", err)
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
			slog.Warn("caster hp refresh: scan failed", "err", err)
			continue
		}
		r.class = DnDClass(classStr)
		batch = append(batch, r)
	}

	refreshed := 0
	for _, r := range batch {
		if casterHPMult(r.class) == 1.0 {
			continue
		}
		conMod := abilityModifier(r.conScore)
		newMax := computeMaxHP(r.class, conMod, r.level)
		if newMax <= r.hpMax {
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
			slog.Warn("caster hp refresh: update failed", "user", r.userID, "err", err)
			continue
		}
		refreshed++
	}

	db.MarkJobCompleted(jobName, "once")
	if refreshed > 0 {
		slog.Info("caster hp refresh: refreshed caster HPMax to D8-d-fix floor", "count", refreshed)
	}
}
