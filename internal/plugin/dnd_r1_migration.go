package plugin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"gogobee/internal/db"
)

// archiveOrphanZoneRuns is the Phase R1 deprecation migration. The old
// `!adventure dungeon` activity could leave dnd_zone_run rows in active
// state that aren't tied to any expedition. R2+ guarantees every new run
// is spawned by ensureRegionRun and pointed at by an active expedition
// (either as exp.run_id or via RegionState["region_runs"]). Anything
// active in dnd_zone_run that doesn't appear in that preserved set is a
// leftover from the legacy path and gets archived.
//
// Idempotent: re-running the migration after the orphan set is empty is
// a no-op. Safe to call on every startup.
//
// Returns the number of rows updated.
func archiveOrphanZoneRuns() (int, error) {
	preserved, err := loadActiveRegionRunIDs()
	if err != nil {
		return 0, fmt.Errorf("collect preserved run ids: %w", err)
	}

	if len(preserved) == 0 {
		res, err := db.Get().Exec(`
			UPDATE dnd_zone_run
			   SET abandoned = 1,
			       completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
			       last_action_at = CURRENT_TIMESTAMP
			 WHERE abandoned = 0`)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}

	placeholders := make([]string, 0, len(preserved))
	args := make([]any, 0, len(preserved))
	for id := range preserved {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := `
		UPDATE dnd_zone_run
		   SET abandoned = 1,
		       completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
		       last_action_at = CURRENT_TIMESTAMP
		 WHERE abandoned = 0
		   AND run_id NOT IN (` + strings.Join(placeholders, ",") + `)`
	res, err := db.Get().Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// loadActiveRegionRunIDs scans every active expedition and returns the set
// of run IDs that should be preserved: each expedition's exp.run_id plus
// every value in its RegionState["region_runs"] map.
func loadActiveRegionRunIDs() (map[string]struct{}, error) {
	rows, err := db.Get().Query(`
		SELECT run_id, region_state
		  FROM dnd_expedition
		 WHERE status = 'active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var runID, regionJSON sql.NullString
		if err := rows.Scan(&runID, &regionJSON); err != nil {
			return nil, err
		}
		if runID.Valid && runID.String != "" {
			out[runID.String] = struct{}{}
		}
		if !regionJSON.Valid || regionJSON.String == "" {
			continue
		}
		var state map[string]any
		if err := json.Unmarshal([]byte(regionJSON.String), &state); err != nil {
			continue
		}
		raw, ok := state[regionStateRegionRuns]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case map[string]any:
			for _, vv := range v {
				if s, ok := vv.(string); ok && s != "" {
					out[s] = struct{}{}
				}
			}
		}
	}
	return out, rows.Err()
}
