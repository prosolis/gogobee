package plugin

import (
	"encoding/json"
	"fmt"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// AdvDailyActivity is one row of "what a player did" on a given UTC date.
// Unifies the legacy adventure_activity_log table with the D&D-layer zone
// run / expedition log tables so the daily report can render a coherent
// view regardless of which subsystem the action ran through.
type AdvDailyActivity struct {
	UserID    id.UserID
	Source    string // "zone_run" | "expedition" | "rest_legacy" | "legacy"
	Activity  string // "zone" | "expedition" | "rest" | legacy activity_type
	Location  string // zone display name or legacy location
	Outcome   string // "completed" | "abandoned" | "boss_defeated" | "in_progress" | "rest" | legacy outcome
	Summary   string // one-line human description
	LootValue int64  // 0 for D&D-layer entries (loot stays in run/expedition state)
	Timestamp time.Time
}

// loadAdvDailyActivity unions activity rows from every action surface for
// the given UTC date (format "2006-01-02"). Returns map keyed by user_id.
//
// Sources (in chronological order on output):
//   - adventure_activity_log (legacy, mostly !rest post-Phase-R)
//   - dnd_zone_run         (zone runs whose last_action_at falls on date)
//   - dnd_expedition_log   (expedition log entries dated today, deduped per
//     expedition into one rollup line)
//
// Each user's slice is sorted oldest→newest. Empty slice when the player
// had no activity on this date.
func loadAdvDailyActivity(date string) (map[id.UserID][]AdvDailyActivity, error) {
	out := make(map[id.UserID][]AdvDailyActivity)
	d := db.Get()

	// 1. Legacy adventure_activity_log
	rows, err := d.Query(`
		SELECT user_id, activity_type, COALESCE(location,''), outcome, loot_value, logged_at
		FROM adventure_activity_log
		WHERE logged_at >= ? AND logged_at < DATE(?, '+1 day')
		ORDER BY logged_at`, date, date)
	if err != nil {
		return nil, fmt.Errorf("legacy log: %w", err)
	}
	for rows.Next() {
		var (
			uid, activity, location, outcome string
			loot                             int64
			ts                               time.Time
		)
		if err := rows.Scan(&uid, &activity, &location, &outcome, &loot, &ts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("legacy log scan: %w", err)
		}
		userID := id.UserID(uid)
		entry := AdvDailyActivity{
			UserID:    userID,
			Source:    "legacy",
			Activity:  activity,
			Location:  location,
			Outcome:   outcome,
			LootValue: loot,
			Timestamp: ts,
		}
		if activity == string(AdvActivityRest) {
			entry.Source = "rest_legacy"
			entry.Summary = "Rested. Streak preserved."
		} else {
			entry.Summary = fmt.Sprintf("%s in %s (%s)", activity, location, outcome)
		}
		out[userID] = append(out[userID], entry)
	}
	rows.Close()

	// Pre-load (user_id, zone_id) of active expeditions. Zone runs are now
	// exclusively spawned by the expedition layer, and the expedition flips
	// a region's run to abandoned on !region travel / inactivity timeout /
	// forced extraction. Those transitions are internal — the expedition
	// rollup below is the source of truth for the player. Skip zone_run
	// entries that match an active expedition to avoid a misleading
	// "Withdrew from <zone>" line while the player is still on it.
	activeExp := make(map[string]struct{})
	expRows, err := d.Query(`
		SELECT user_id, zone_id FROM dnd_expedition
		 WHERE status IN ('active','extracting')`)
	if err != nil {
		return nil, fmt.Errorf("active expeditions: %w", err)
	}
	for expRows.Next() {
		var u, z string
		if err := expRows.Scan(&u, &z); err != nil {
			expRows.Close()
			return nil, fmt.Errorf("active expedition scan: %w", err)
		}
		activeExp[u+"|"+z] = struct{}{}
	}
	expRows.Close()

	// 2. dnd_zone_run — rows touched today. Progress count is derived
	// from len(visited_nodes) — current_room retired in G9.
	rows, err = d.Query(`
		SELECT user_id, zone_id, visited_nodes, total_rooms, abandoned,
		       boss_defeated, completed_at, last_action_at
		FROM dnd_zone_run
		WHERE last_action_at >= ? AND last_action_at < DATE(?, '+1 day')
		ORDER BY last_action_at`, date, date)
	if err != nil {
		return nil, fmt.Errorf("zone runs: %w", err)
	}
	for rows.Next() {
		var (
			uid, zoneID                        string
			visitedJSON                        string
			totalRooms                         int
			abandoned, bossDefeated            int
			completedAt                        *time.Time
			lastAction                         time.Time
		)
		if err := rows.Scan(&uid, &zoneID, &visitedJSON, &totalRooms,
			&abandoned, &bossDefeated, &completedAt, &lastAction); err != nil {
			rows.Close()
			return nil, fmt.Errorf("zone run scan: %w", err)
		}
		var visited []string
		_ = json.Unmarshal([]byte(visitedJSON), &visited)
		currentRoom := len(visited) - 1
		if currentRoom < 0 {
			currentRoom = 0
		}
		if _, onExp := activeExp[uid+"|"+zoneID]; onExp {
			continue
		}
		userID := id.UserID(uid)
		zoneDef := zoneOrFallback(ZoneID(zoneID))

		var outcome, summary string
		switch {
		case bossDefeated == 1:
			outcome = "boss_defeated"
			summary = fmt.Sprintf("Cleared %s — boss down (%d/%d rooms).", zoneDef.Display, currentRoom, totalRooms)
		case completedAt != nil && abandoned == 0:
			outcome = "completed"
			summary = fmt.Sprintf("Cleared %s (%d/%d rooms).", zoneDef.Display, currentRoom, totalRooms)
		case abandoned == 1:
			outcome = "abandoned"
			summary = fmt.Sprintf("Withdrew from %s at room %d/%d.", zoneDef.Display, currentRoom, totalRooms)
		default:
			outcome = "in_progress"
			summary = fmt.Sprintf("Mid-run in %s (%d/%d rooms).", zoneDef.Display, currentRoom, totalRooms)
		}
		out[userID] = append(out[userID], AdvDailyActivity{
			UserID:    userID,
			Source:    "zone_run",
			Activity:  "zone",
			Location:  zoneDef.Display,
			Outcome:   outcome,
			Summary:   summary,
			Timestamp: lastAction,
		})
	}
	rows.Close()

	// 3. dnd_expedition_log — pick the most-recent player-visible entry
	//    per expedition for users active today. We rollup per expedition
	//    rather than per row so the report doesn't list every camp/combat
	//    beat separately.
	rows, err = d.Query(`
		SELECT e.user_id, e.zone_id, e.current_day, e.status,
		       l.entry_type, l.summary, l.timestamp
		FROM dnd_expedition_log l
		JOIN dnd_expedition e ON e.expedition_id = l.expedition_id
		WHERE l.timestamp >= ? AND l.timestamp < DATE(?, '+1 day')
		  AND l.entry_type IN ('action','combat','event','recap')
		ORDER BY l.timestamp`, date, date)
	if err != nil {
		return nil, fmt.Errorf("expedition log: %w", err)
	}
	type expKey struct {
		uid, zoneID string
	}
	mostRecentExp := make(map[expKey]AdvDailyActivity)
	for rows.Next() {
		var (
			uid, zoneID, status, entryType, summary string
			currentDay                              int
			ts                                      time.Time
		)
		if err := rows.Scan(&uid, &zoneID, &currentDay, &status,
			&entryType, &summary, &ts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("expedition log scan: %w", err)
		}
		userID := id.UserID(uid)
		zoneDef := zoneOrFallback(ZoneID(zoneID))
		k := expKey{string(userID), zoneID}
		mostRecentExp[k] = AdvDailyActivity{
			UserID:   userID,
			Source:   "expedition",
			Activity: "expedition",
			Location: zoneDef.Display,
			Outcome:  status,
			Summary: fmt.Sprintf("Expedition in %s, day %d — %s",
				zoneDef.Display, currentDay, summary),
			Timestamp: ts,
		}
	}
	rows.Close()
	for _, entry := range mostRecentExp {
		out[entry.UserID] = append(out[entry.UserID], entry)
	}

	// Sort each user's slice oldest→newest so callers can pick the
	// representative entry deterministically (typically the latest).
	for uid := range out {
		entries := out[uid]
		// Insertion-sort is fine here — slices are tiny.
		for i := 1; i < len(entries); i++ {
			for j := i; j > 0 && entries[j-1].Timestamp.After(entries[j].Timestamp); j-- {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			}
		}
		out[uid] = entries
	}

	return out, nil
}
