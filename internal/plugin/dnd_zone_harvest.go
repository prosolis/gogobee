package plugin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// Standalone-zone-run harvest. The expedition path uses
// expedition.region_state to keep per-(region,room) HarvestNode state;
// !zone enter runs aren't tied to any expedition, so we mirror the
// per-room storage onto dnd_zone_run.harvest_nodes_json instead.
//
// Region-scoped expedition features (kill-gated nodes, conditional
// events, cursed trinket drops, threat ticks) don't fire here — those
// belong to the expedition system. Standalone harvest is the casual
// flow: roll d20 vs DC, get yield, decrement node, save.

// loadStandaloneHarvestNodes reads the run's harvest map and returns
// the nodes for the given graph node. Lazy-seeds from the zone registry
// on first access. Caller persists via saveStandaloneHarvestNodes.
//
// G6 migration: rows written before the re-key used the room-index
// integer as the storage key. When the node-id key is absent and the
// id maps back to a legacy room index, we read from the old key so
// in-flight runs don't lose harvest state. The next save writes under
// the new key (and drops the old one).
func loadStandaloneHarvestNodes(runID string, zoneID ZoneID, nodeID string) ([]HarvestNode, error) {
	table, err := loadStandaloneHarvestTable(runID)
	if err != nil {
		return nil, err
	}
	roomKey := nodeHarvestKey(nodeID)
	if existing, ok := table[roomKey]; ok {
		return existing, nil
	}
	if idx, ok := legacyRoomIdxFromNodeID(nodeID, zoneID); ok {
		if existing, ok := table[strconv.Itoa(idx)]; ok {
			return existing, nil
		}
	}
	return seedRoomNodes(zoneID), nil
}

// saveStandaloneHarvestNodes writes nodes back into the run's harvest map.
func saveStandaloneHarvestNodes(runID string, zoneID ZoneID, nodeID string, nodes []HarvestNode) error {
	table, err := loadStandaloneHarvestTable(runID)
	if err != nil {
		return err
	}
	table[nodeHarvestKey(nodeID)] = nodes
	if idx, ok := legacyRoomIdxFromNodeID(nodeID, zoneID); ok {
		delete(table, strconv.Itoa(idx))
	}
	raw, err := json.Marshal(table)
	if err != nil {
		return fmt.Errorf("marshal harvest table: %w", err)
	}
	_, err = db.Get().Exec(
		`UPDATE dnd_zone_run SET harvest_nodes_json = ?, last_action_at = CURRENT_TIMESTAMP WHERE run_id = ?`,
		string(raw), runID)
	return err
}

func loadStandaloneHarvestTable(runID string) (map[string][]HarvestNode, error) {
	row := db.Get().QueryRow(
		`SELECT harvest_nodes_json FROM dnd_zone_run WHERE run_id = ?`, runID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string][]HarvestNode{}, nil
		}
		return nil, err
	}
	if raw == "" || raw == "{}" {
		return map[string][]HarvestNode{}, nil
	}
	table := map[string][]HarvestNode{}
	if err := json.Unmarshal([]byte(raw), &table); err != nil {
		// Corrupt blob — fall through with empty so harvest re-seeds.
		slog.Warn("standalone harvest: corrupt nodes json", "run", runID, "err", err)
		return map[string][]HarvestNode{}, nil
	}
	return table, nil
}

// Compile-time check that loadAdvCharacter is reachable; silences "unused
// import" complaints if loadAdvCharacter changes signature in the future.
var _ id.UserID
