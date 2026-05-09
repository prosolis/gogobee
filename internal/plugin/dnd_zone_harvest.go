package plugin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"

	"gogobee/internal/db"
	"gogobee/internal/flavor"

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

// handleStandaloneHarvest is the !forage/!mine/!fish/etc. fallback for
// players in a single-session !zone enter run with no active expedition.
// Mirrors the core of handleHarvestCmd minus the expedition-only side
// effects (threat ticks, region kills, cursed-trinket drops, log entries).
func (p *AdventurePlugin) handleStandaloneHarvest(ctx MessageContext, char *DnDCharacter, action HarvestAction, run *DungeonRun) error {
	if action == HarvestFish && !isFishingZone(run.ZoneID) {
		return p.SendDM(ctx.Sender, "There's no water to fish in here. `!fish` works only in Forest of Shadows, Sunken Temple, Underdark, or Feywild Crossing.")
	}
	if !currentRoomCleared(run) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You can't harvest here — the room (%s) isn't cleared. Resolve combat first.",
			prettyRoomType(run.CurrentRoomType())))
	}

	nodeID := harvestNodeIDFor(run)
	nodes, err := loadStandaloneHarvestNodes(run.RunID, run.ZoneID, nodeID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load harvest state.")
	}

	// Reuse the expedition picker by passing a stub Expedition with just
	// ZoneID set. The kill-gated and event-conditional predicates inside
	// pickAvailableNode bail early on nil/empty RegionState — exactly the
	// behavior we want for standalone (no kill tracking, no events).
	stubExp := &Expedition{ZoneID: run.ZoneID}
	idx := pickAvailableNode(nodes, stubExp, char, action)
	if idx < 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Nothing left to %s in this room. Try `!resources` to see what's still gatherable.", action))
	}
	res, _ := FindZoneResource(run.ZoneID, nodes[idx].ResourceID)

	roll := rand.IntN(20) + 1
	mod := harvestActionAbility(char, action) + classHarvestBonus(char.Class, action)
	if action == HarvestFish {
		if adv, _ := loadAdvCharacter(ctx.Sender); adv != nil {
			mod += fishingSkillBonus(adv.FishingSkill)
		}
	}
	total := roll + mod
	outcome := resolveHarvestOutcome(total, res.DC)
	outcome = rangerRareCatchUpgrade(char.Class, action, outcome, nil)

	var b strings.Builder
	verb := strings.ToTitle(string(action)[:1]) + string(action)[1:]
	b.WriteString(fmt.Sprintf("**%s · %s** — d20=%d + %d = **%d** vs DC %d\n\n",
		verb, res.Name, roll, mod, total, res.DC))

	switch outcome {
	case OutcomeNothing:
		b.WriteString(flavor.Pick(flavor.HarvestFail))
		b.WriteString("\n\n_No yield. The node is still here — try again._")
	case OutcomeCommon:
		_ = grantHarvestYield(ctx.Sender, res, 1)
		b.WriteString(harvestSuccessLine(action, run.ZoneID))
		b.WriteString(fmt.Sprintf("\n\n_+1 %s._", res.Name))
		nodes[idx].CurrentCharges--
	case OutcomeStandard:
		qty := rand.IntN(3) + 1
		_ = grantHarvestYield(ctx.Sender, res, qty)
		b.WriteString(harvestSuccessLine(action, run.ZoneID))
		b.WriteString(fmt.Sprintf("\n\n_+%d %s._", qty, res.Name))
		nodes[idx].CurrentCharges--
	case OutcomeRich:
		qty := rand.IntN(3) + 1
		_ = grantHarvestYield(ctx.Sender, res, qty)
		b.WriteString(flavor.Pick(flavor.RichYield))
		b.WriteString(fmt.Sprintf("\n\n_+%d %s._", qty, res.Name))
		nodes[idx].CurrentCharges--
	}

	if outcome != OutcomeNothing && nodes[idx].CurrentCharges <= 0 {
		b.WriteString("\n\n")
		b.WriteString(flavor.Pick(flavor.NodeDepleted))
	}

	if err := saveStandaloneHarvestNodes(run.RunID, run.ZoneID, nodeID, nodes); err != nil {
		slog.Error("standalone harvest: save nodes", "user", ctx.Sender, "run", run.RunID, "err", err)
	}

	return p.SendDM(ctx.Sender, b.String())
}

// Compile-time check that loadAdvCharacter is reachable; silences "unused
// import" complaints if loadAdvCharacter changes signature in the future.
var _ id.UserID
