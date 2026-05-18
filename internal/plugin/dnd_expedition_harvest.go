package plugin

// Phase R2 — Expedition harvest nodes & commands (full implementation).
// Spec: gogobee_resource_combat_integration.md §2 + §3.
//
// Architecture:
//   - On !expedition start, ensureRegionRun spawns a DungeonRun for the
//     current region and pins it via Expedition.RunID. Multi-region zones
//     get one run per region, indexed in RegionState["region_runs"].
//   - On !region travel, the outgoing region's run is abandoned and a
//     fresh run is spawned for the new region.
//   - HarvestNode tables are keyed by (regionKey, roomIdx) under
//     RegionState["harvest_nodes"]. Each room's nodes are lazy-seeded
//     from the §3 zone registry on first harvest in that room.
//   - Harvest only succeeds in cleared rooms (entry rooms auto-pass;
//     non-entry require roomIdx ∈ DungeonRun.RoomsCleared).
//   - Long rest at camp replenishes every node across every room and
//     region (§2.3).
//
// Deferred to R3 (combat-link):
//   - Combat Interrupt rolls (§4.2)
//   - RequiresKill resources (writer for RegionState["kills"] not yet wired)

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// RegionState slot keys used by the harvest layer.
//
//	RegionState["harvest_nodes"][regionKey][roomKey] = []HarvestNode
//	RegionState["region_runs"][regionKey]            = runID string
const (
	regionStateHarvestKey = "harvest_nodes"
	regionStateRegionRuns = "region_runs"
)

// HarvestNode is one resource node in a specific room.
type HarvestNode struct {
	ResourceID     string `json:"resource_id"`
	CurrentCharges int    `json:"current_charges"`
	MaxCharges     int    `json:"max_charges"`
}

// HarvestOutcome enumerates §2.2 outcomes.
type HarvestOutcome string

const (
	OutcomeNothing  HarvestOutcome = "nothing"
	OutcomeCommon   HarvestOutcome = "common"
	OutcomeStandard HarvestOutcome = "standard"
	OutcomeRich     HarvestOutcome = "rich"
)

// resolveHarvestOutcome maps (roll, dc) to the §2.2 bracket.
func resolveHarvestOutcome(roll, dc int) HarvestOutcome {
	delta := roll - dc
	switch {
	case delta >= 5:
		return OutcomeRich
	case delta >= 0:
		return OutcomeStandard
	case delta >= -4:
		return OutcomeCommon
	default:
		return OutcomeNothing
	}
}

// classHarvestBonus implements §2.1 +2 class affinity.
func classHarvestBonus(class DnDClass, action HarvestAction) int {
	match := false
	switch action {
	case HarvestForage, HarvestFish:
		match = class == ClassRanger
	case HarvestMine:
		match = class == ClassFighter
	case HarvestScavenge:
		match = class == ClassRogue
	case HarvestEssence:
		match = class == ClassMage
	case HarvestCommune:
		match = class == ClassCleric
	}
	if match {
		return 2
	}
	return 0
}

// harvestActionAbility returns the §2.1 ability modifier for `action`.
func harvestActionAbility(c *DnDCharacter, action HarvestAction) int {
	switch action {
	case HarvestForage:
		return abilityModifier(c.WIS)
	case HarvestMine:
		return abilityModifier(c.STR)
	case HarvestScavenge:
		return abilityModifier(c.INT)
	case HarvestEssence:
		return abilityModifier(c.INT)
	case HarvestCommune:
		return abilityModifier(c.WIS)
	case HarvestFish:
		return abilityModifier(c.DEX)
	}
	return 0
}

// regionHarvestKey returns the storage key for the player's current region.
// Single-region zones store under "".
func regionHarvestKey(e *Expedition) string { return e.CurrentRegion }

// nodeHarvestKey returns the JSON-safe storage key for a graph node id.
// Phase G6: harvest tables are keyed by node id (e.g. "crypt_valdris.r3"),
// not by raw room index. Linear-zone runs derive a node id with the
// `<zone>.r<N+1>` scheme (deriveLegacyNodeID), so this collapses to a
// stable key for the existing room-walk model. Branching zones use the
// hand-authored node ids and get distinct keys per branch.
func nodeHarvestKey(nodeID string) string { return nodeID }

// legacyRoomIdxFromNodeID parses a derived node id back to its 0-based
// room index. Returns ok=false for hand-authored ids that don't match
// the `<zone>.r<N+1>` scheme — there's no legacy storage to migrate
// for those, so the caller skips the fallback.
func legacyRoomIdxFromNodeID(nodeID string, zoneID ZoneID) (int, bool) {
	prefix := string(zoneID) + ".r"
	if !strings.HasPrefix(nodeID, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(nodeID, prefix))
	if err != nil || n < 1 {
		return 0, false
	}
	return n - 1, true
}

// loadHarvestNodes returns the (region, node)'s nodes, lazy-seeding from
// the §3 registry on first access. Caller must persist via
// saveHarvestNodes after mutation.
//
// Migration: rows written before G6 used the room-index integer as the
// storage key. When a node-id-keyed entry is missing and the id maps
// back to a legacy room index, we read from the old key so existing
// expeditions don't lose harvest state mid-flight. The next save writes
// under the new key.
func loadHarvestNodes(e *Expedition, nodeID string) []HarvestNode {
	if e == nil {
		return nil
	}
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	table := loadHarvestTable(e)
	regionKey := regionHarvestKey(e)
	roomKey := nodeHarvestKey(nodeID)

	regionMap, ok := table[regionKey]
	if !ok {
		regionMap = map[string][]HarvestNode{}
		table[regionKey] = regionMap
	}
	if existing, ok := regionMap[roomKey]; ok {
		return existing
	}
	if idx, ok := legacyRoomIdxFromNodeID(nodeID, e.ZoneID); ok {
		if existing, ok := regionMap[strconv.Itoa(idx)]; ok {
			return existing
		}
	}
	seeded := seedRoomNodes(e.ZoneID)
	regionMap[roomKey] = seeded
	table[regionKey] = regionMap
	e.RegionState[regionStateHarvestKey] = table
	return seeded
}

// saveHarvestNodes writes the supplied node slice into the (region, node)
// slot and persists the expedition state.
func saveHarvestNodes(e *Expedition, nodeID string, nodes []HarvestNode) error {
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	table := loadHarvestTable(e)
	regionKey := regionHarvestKey(e)
	roomKey := nodeHarvestKey(nodeID)
	regionMap, ok := table[regionKey]
	if !ok {
		regionMap = map[string][]HarvestNode{}
	}
	regionMap[roomKey] = nodes
	if idx, ok := legacyRoomIdxFromNodeID(nodeID, e.ZoneID); ok {
		// Drop the legacy room-idx entry once the node-id key is live so
		// the table doesn't carry parallel state forever.
		delete(regionMap, strconv.Itoa(idx))
	}
	table[regionKey] = regionMap
	e.RegionState[regionStateHarvestKey] = table
	return persistRegionState(e)
}

// loadHarvestTable parses RegionState["harvest_nodes"] into the typed
// nested-map shape, regardless of how JSON unmarshalled it.
func loadHarvestTable(e *Expedition) map[string]map[string][]HarvestNode {
	table := map[string]map[string][]HarvestNode{}
	if e == nil || e.RegionState == nil {
		return table
	}
	raw, ok := e.RegionState[regionStateHarvestKey]
	if !ok {
		return table
	}
	switch v := raw.(type) {
	case map[string]map[string][]HarvestNode:
		return v
	default:
		// JSON path: re-marshal the generic shape and re-parse into the
		// typed nested shape.
		b, err := json.Marshal(v)
		if err != nil {
			return table
		}
		_ = json.Unmarshal(b, &table)
	}
	return table
}

// seedRoomNodes constructs the initial node set for a single room from
// the zone registry. Each ZoneResource becomes one node sized to its
// MaxCharges entry.
func seedRoomNodes(zoneID ZoneID) []HarvestNode {
	all := ZoneResourcesAll(zoneID)
	nodes := make([]HarvestNode, 0, len(all))
	for _, r := range all {
		max := r.MaxCharges
		if max <= 0 {
			max = 1
		}
		nodes = append(nodes, HarvestNode{
			ResourceID:     r.ID,
			CurrentCharges: max,
			MaxCharges:     max,
		})
	}
	return nodes
}

// ReplenishHarvestNodes resets every node's CurrentCharges across every
// (region, room) tracked under this expedition. Called from the long-rest
// path (processOvernightCamp).
func ReplenishHarvestNodes(e *Expedition) error {
	if e == nil {
		return nil
	}
	table := loadHarvestTable(e)
	if len(table) == 0 {
		return nil
	}
	for regionKey, regionMap := range table {
		for roomKey, nodes := range regionMap {
			for i := range nodes {
				nodes[i].CurrentCharges = nodes[i].MaxCharges
			}
			regionMap[roomKey] = nodes
		}
		table[regionKey] = regionMap
	}
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	e.RegionState[regionStateHarvestKey] = table
	return persistRegionState(e)
}

// regionKillsContains reports whether RegionState["kills"][region] holds
// `kind`. Combat-link writes this list (R3); until then it's empty.
func regionKillsContains(e *Expedition, kind string) bool {
	if e == nil || e.RegionState == nil || kind == "" {
		return false
	}
	raw, ok := e.RegionState["kills"]
	if !ok {
		return false
	}
	table := map[string][]string{}
	switch v := raw.(type) {
	case map[string][]string:
		table = v
	case map[string]any:
		if b, err := json.Marshal(v); err == nil {
			_ = json.Unmarshal(b, &table)
		}
	default:
		return false
	}
	for _, k := range table[regionHarvestKey(e)] {
		if k == kind {
			return true
		}
	}
	return false
}

// pickAvailableNode picks the lowest-DC eligible node matching `action`
// in `nodes`. Returns the node index or -1 if none usable.
func pickAvailableNode(nodes []HarvestNode, e *Expedition, char *DnDCharacter, action HarvestAction) int {
	bestIdx := -1
	bestDC := 1<<31 - 1
	for i := range nodes {
		n := &nodes[i]
		if n.CurrentCharges <= 0 {
			continue
		}
		res, ok := FindZoneResource(e.ZoneID, n.ResourceID)
		if !ok || res.Action != action {
			continue
		}
		if res.ClassRestrict != "" && res.ClassRestrict != char.Class {
			continue
		}
		if res.RequiresKill != "" && !regionKillsContains(e, res.RequiresKill) {
			continue
		}
		if res.ConditionalEvent != "" && !regionEventActive(e, res.ConditionalEvent) {
			continue
		}
		if res.DC < bestDC {
			bestDC = res.DC
			bestIdx = i
		}
	}
	return bestIdx
}

// regionEventActive checks RegionState for an event flag (e.g. tidal_event).
func regionEventActive(e *Expedition, eventID string) bool {
	if e == nil || e.RegionState == nil {
		return false
	}
	switch eventID {
	case "tidal_event":
		v, _ := e.RegionState["tidal_active"].(bool)
		return v
	}
	return false
}

// ── room-state helpers ──────────────────────────────────────────────────────

// currentRoomCleared reports whether the player's current room is safe
// for harvest. Entry/Exploration/Trap rooms allow harvest unconditionally
// — risk is modeled by the §4.2 Combat Interrupt roll inside the harvest
// path, not by gating the command. Elite/Boss rooms stay gated: harvest
// is only allowed once they're in run.RoomsCleared.
//
// Why this isn't "is the current room in RoomsCleared": the runtime
// flow couples combat resolution with room transition inside !zone
// advance, so the current room index is *never* in RoomsCleared until
// the player has already moved past it. A strict cleared-only gate
// makes harvest in any non-entry room impossible.
func currentRoomCleared(run *DungeonRun) bool {
	if run == nil {
		return false
	}
	switch run.CurrentRoomType() {
	case RoomEntry, RoomExploration, RoomTrap:
		return true
	}
	for _, idx := range run.RoomsCleared {
		if idx == run.CurrentRoom {
			return true
		}
	}
	return false
}

// harvestSuccessLine picks the action's generic success pool and
// occasionally layers a zone-specific Harvest* postscript.
func harvestSuccessLine(action HarvestAction, zoneID ZoneID) string {
	var generic []string
	switch action {
	case HarvestForage:
		generic = flavor.HarvestForageSuccess
	case HarvestMine:
		generic = flavor.HarvestMineSuccess
	case HarvestScavenge:
		generic = flavor.HarvestScavengeSuccess
	case HarvestEssence:
		generic = flavor.HarvestEssenceSuccess
	case HarvestCommune:
		generic = flavor.HarvestCommuneSuccess
	case HarvestFish:
		generic = flavor.HarvestFishSuccess
	}
	line := flavor.Pick(generic)
	if rand.IntN(3) == 0 {
		if zoneLine := zoneHarvestLine(zoneID); zoneLine != "" {
			return line + " " + zoneLine
		}
	}
	return line
}

// zoneHarvestLine picks one entry from the zone-specific Harvest* pool.
func zoneHarvestLine(zoneID ZoneID) string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return flavor.Pick(flavor.HarvestGoblinWarrens)
	case ZoneCryptValdris:
		return flavor.Pick(flavor.HarvestCryptValdris)
	case ZoneForestShadows:
		return flavor.Pick(flavor.HarvestForestShadows)
	case ZoneSunkenTemple:
		return flavor.Pick(flavor.HarvestSunkenTemple)
	case ZoneManorBlackspire:
		return flavor.Pick(flavor.HarvestHauntedManor)
	case ZoneUnderforge:
		return flavor.Pick(flavor.HarvestUnderforge)
	case ZoneUnderdark:
		return flavor.Pick(flavor.HarvestUnderdark)
	case ZoneFeywildCrossing:
		return flavor.Pick(flavor.HarvestFeywild)
	case ZoneDragonsLair:
		return flavor.Pick(flavor.HarvestDragonsLair)
	case ZoneAbyssPortal:
		return flavor.Pick(flavor.HarvestAbyssPortal)
	}
	return ""
}

// pickRichBonus picks a higher-rarity zone resource (different ID, any
// action) as the §2.2 rich-yield bonus drop. Returns nil if no candidate.
func pickRichBonus(zoneID ZoneID, baseID string) *ZoneResource {
	all := ZoneResourcesAll(zoneID)
	candidates := make([]ZoneResource, 0, len(all))
	for _, r := range all {
		if r.ID == baseID {
			continue
		}
		switch r.Rarity {
		case RarityUncommon, RarityRare, RarityVeryRare:
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	pick := candidates[rand.IntN(len(candidates))]
	return &pick
}

// grantHarvestYield deposits the resource into the player's inventory.
func grantHarvestYield(userID id.UserID, res ZoneResource, qty int) error {
	if qty <= 0 {
		return nil
	}
	if isHol, _ := isHolidayToday(); isHol {
		qty++
	}
	tier := zoneTierFromID(res.ZoneID)
	for i := 0; i < qty; i++ {
		item := AdvItem{
			Name:        res.Name,
			Type:        "material",
			Tier:        tier,
			Value:       int64(res.SellValue),
			SkillSource: string(res.Action),
		}
		if err := addAdvInventoryItem(userID, item); err != nil {
			return err
		}
	}
	return nil
}

// zoneTierFromID returns the zone's tier for sell-value bookkeeping.
func zoneTierFromID(zoneID ZoneID) int {
	z, ok := getZone(zoneID)
	if !ok {
		return 1
	}
	return int(z.Tier)
}

// ── !resources ────────────────────────────────────────────────────────────

// handleResourcesCmd lists active nodes in the current room.
func (p *AdventurePlugin) handleResourcesCmd(ctx MessageContext) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load expedition state.")
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "You're not on an expedition. Start one with `!expedition start <zone>`.")
	}
	char, err := LoadDnDCharacter(ctx.Sender)
	if err != nil || char == nil {
		return p.SendDM(ctx.Sender, "No D&D character found. Run `!setup` first.")
	}
	if exp.RunID == "" {
		return p.SendDM(ctx.Sender, "No active region run. Try `!region` to refresh, or restart the expedition.")
	}
	run, err := getZoneRun(exp.RunID)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "Couldn't load region state.")
	}
	nodeID := harvestNodeIDFor(run)
	nodes := loadHarvestNodes(exp, nodeID)
	_ = saveHarvestNodes(exp, nodeID, nodes) // persist seed if first touch

	zone, _ := getZone(exp.ZoneID)
	regionLabel := exp.CurrentRegion
	if regionLabel == "" {
		regionLabel = string(exp.ZoneID)
	}
	cleared := currentRoomCleared(run)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Resources — %s · %s · Room %d/%d (%s)**\n\n",
		zone.Display, regionLabel, run.CurrentRoom+1, run.TotalRooms, prettyRoomType(run.CurrentRoomType())))
	if !cleared {
		b.WriteString("_Room not cleared — harvest blocked until combat resolves._\n\n")
	}

	any := false
	for _, n := range nodes {
		res, ok := FindZoneResource(exp.ZoneID, n.ResourceID)
		if !ok {
			continue
		}
		gates := []string{}
		if res.ClassRestrict != "" && res.ClassRestrict != char.Class {
			gates = append(gates, fmt.Sprintf("%s only", strings.Title(string(res.ClassRestrict))))
		}
		if res.RequiresKill != "" && !regionKillsContains(exp, res.RequiresKill) {
			gates = append(gates, "post-"+res.RequiresKill+" kill")
		}
		if res.ConditionalEvent != "" && !regionEventActive(exp, res.ConditionalEvent) {
			gates = append(gates, res.ConditionalEvent+" only")
		}
		status := fmt.Sprintf("%d/%d", n.CurrentCharges, n.MaxCharges)
		if n.CurrentCharges == 0 {
			status = "depleted"
		}
		gateStr := ""
		if len(gates) > 0 {
			gateStr = " _(" + strings.Join(gates, "; ") + ")_"
		}
		b.WriteString(fmt.Sprintf("• `!%s` · **%s** (DC %d, %s) — %s%s\n",
			res.Action, res.Name, res.DC, res.Rarity, status, gateStr))
		any = true
	}
	if !any {
		b.WriteString("_Zone has no harvest registry._")
	} else {
		b.WriteString("\n_Long rest at camp replenishes nodes across every room._")
	}
	return p.SendDM(ctx.Sender, b.String())
}

// ── region-run linkage (auto-spawn DungeonRun per region) ─────────────────

// regionRunsMap reads the regionKey→runID pointer table from RegionState.
func regionRunsMap(e *Expedition) map[string]string {
	out := map[string]string{}
	if e == nil || e.RegionState == nil {
		return out
	}
	raw, ok := e.RegionState[regionStateRegionRuns]
	if !ok {
		return out
	}
	switch v := raw.(type) {
	case map[string]string:
		return v
	case map[string]any:
		for k, vv := range v {
			if s, ok := vv.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

// setRegionRun persists `runID` as the region's run pointer and rewrites
// exp.RunID to match. The caller is responsible for actually creating
// (or having created) the run. Pass "" to clear.
func setRegionRun(e *Expedition, regionKey, runID string) error {
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	m := regionRunsMap(e)
	if runID == "" {
		delete(m, regionKey)
	} else {
		m[regionKey] = runID
	}
	e.RegionState[regionStateRegionRuns] = m
	if err := persistRegionState(e); err != nil {
		return err
	}
	e.RunID = runID
	return setExpeditionRunID(e.ID, runID)
}

// ensureRegionRun guarantees a DungeonRun exists for the player's current
// region and that exp.RunID points at it. If a run already exists for
// the region, it is reloaded; otherwise a fresh one is started after
// abandoning any other active run for the player.
//
// charLevel is needed for the zone-tier gate inside startZoneRun.
func ensureRegionRun(exp *Expedition, charLevel int) (*DungeonRun, error) {
	regionKey := regionHarvestKey(exp)
	runs := regionRunsMap(exp)
	if existing, ok := runs[regionKey]; ok && existing != "" {
		run, err := getZoneRun(existing)
		if err != nil {
			return nil, err
		}
		if run != nil {
			if exp.RunID != run.RunID {
				if err := setExpeditionRunID(exp.ID, run.RunID); err != nil {
					return nil, err
				}
				exp.RunID = run.RunID
			}
			return run, nil
		}
	}

	// No run mapped for this region; if a different active run exists for
	// the player (stale link from prior region), abandon it first.
	if other, _ := getActiveZoneRun(id.UserID(exp.UserID)); other != nil {
		if err := abandonZoneRunByID(other.RunID); err != nil {
			return nil, err
		}
	}

	run, err := startZoneRun(id.UserID(exp.UserID), exp.ZoneID, charLevel, nil)
	if err != nil {
		return nil, fmt.Errorf("ensureRegionRun: %w", err)
	}
	if err := setRegionRun(exp, regionKey, run.RunID); err != nil {
		return nil, err
	}
	return run, nil
}

// retireRegionRun abandons the player's run for the given regionKey (or
// the current region if regionKey == "" matches CurrentRegion). Idempotent.
// Used by handleRegionCmd "travel" before transitioning.
func retireRegionRun(exp *Expedition, regionKey string) error {
	runs := regionRunsMap(exp)
	runID, ok := runs[regionKey]
	if !ok || runID == "" {
		return nil
	}
	if err := abandonZoneRunByID(runID); err != nil {
		return err
	}
	return setRegionRun(exp, regionKey, "")
}

// retireAllRegionRuns abandons every region's linked run. Called from the
// expedition-end paths (abandon, complete, forced extract) so that the
// per-user "active zone run" guard stays clean for the next adventure.
func retireAllRegionRuns(exp *Expedition) error {
	if exp == nil {
		return nil
	}
	for region := range regionRunsMap(exp) {
		if err := retireRegionRun(exp, region); err != nil {
			return err
		}
	}
	// Also clear any unmapped active run for the user (defensive).
	if other, _ := getActiveZoneRun(id.UserID(exp.UserID)); other != nil {
		_ = abandonZoneRunByID(other.RunID)
	}
	return nil
}

// ── R6: Zone-condition harvest interactions (§3 zone notes) ──────────────────

// zoneConditionHarvestBlock returns a human-facing block message when the
// current zone state forbids harvesting entirely (Heat 10 in Underforge,
// Infernax awake in Dragon's Lair, Instability 81+ in Abyss). Empty
// string = harvesting allowed.
func zoneConditionHarvestBlock(exp *Expedition, action HarvestAction) string {
	switch exp.ZoneID {
	case ZoneUnderforge:
		if action == HarvestMine && exp.TemporalStack >= UnderforgeMaxHeat {
			return "_The forge stones are white-hot. The hammers shake out of your hands before they bite. Mining is impossible until the heat drops._"
		}
	case ZoneDragonsLair:
		if awake, _ := exp.RegionState["infernax_awake"].(bool); awake {
			return "_Infernax is hunting these halls now. There's no time to harvest — keep moving or extract._"
		}
	case ZoneAbyssPortal:
		if exp.TemporalStack >= 81 {
			return "_The Abyss is unraveling. Reality is too loose to anchor a harvest — what you reach for slides between your fingers._"
		}
	}
	return ""
}

// zoneConditionHarvestDCMod returns a (deltaDC, reason) pair applied to
// the per-zone harvest roll. Reason is shown alongside the dice line.
func zoneConditionHarvestDCMod(exp *Expedition, action HarvestAction) (int, string) {
	switch exp.ZoneID {
	case ZoneUnderforge:
		if action == HarvestMine && exp.TemporalStack >= 7 {
			return 3, "heat-baked stone"
		}
	case ZoneDragonsLair:
		// Awareness pulses fire on every DragonsLairAwarenessCycleDays.
		// Pulse 3+ = day 9+. Use day count as the proxy (matches §7.5
		// pulse cadence and the spec's "Awareness Pulse 3+" reading).
		pulses := 0
		if d := exp.CurrentDay; d > 0 {
			pulses = d / DragonsLairAwarenessCycleDays
		}
		if pulses >= 3 {
			return 4, "the lair is alert"
		}
	case ZoneAbyssPortal:
		if exp.TemporalStack >= 61 && exp.TemporalStack < 81 {
			return 5, "reality surges"
		}
	case ZoneFeywildCrossing:
		if action == HarvestForage {
			raw, _ := exp.RegionState["feywild_today"].(string)
			if FeywildDistortion(raw) == FeywildDistortionDouble {
				return -3, "double-day bloom"
			}
		}
	}
	return 0, ""
}

// manorCursedTrinketChance is §3 Zone 05 — every !scavenge in the Manor
// has a 10% secondary-drop chance regardless of node outcome.
const manorCursedTrinketChance = 10

// rollManorCursedTrinket returns true when a !scavenge in the Manor
// should also yield a Cursed Trinket. rng==nil falls back to math/rand.
func rollManorCursedTrinket(zoneID ZoneID, action HarvestAction, rng func(int) int) bool {
	if zoneID != ZoneManorBlackspire || action != HarvestScavenge {
		return false
	}
	roll := 0
	if rng != nil {
		roll = rng(100)
	} else {
		roll = rand.IntN(100)
	}
	return roll < manorCursedTrinketChance
}

// restoreHarvestNodesInRoom resets every node in (region, node) to its
// MaxCharges. Used by the Feywild Time Loop event (§7.4) — "previous
// harvest results in that room are nullified." Empty nodeID is a no-op
// (caller had no active run/node to target).
func restoreHarvestNodesInRoom(exp *Expedition, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	nodes := loadHarvestNodes(exp, nodeID)
	if len(nodes) == 0 {
		return false
	}
	changed := false
	for i := range nodes {
		if nodes[i].CurrentCharges < nodes[i].MaxCharges {
			nodes[i].CurrentCharges = nodes[i].MaxCharges
			changed = true
		}
	}
	if !changed {
		return false
	}
	// Write the mutated nodes back into the in-memory table; the
	// briefing/cycle flow persists RegionState downstream. Avoiding the
	// persist call here keeps the helper test-friendly without a DB.
	regionKey := regionHarvestKey(exp)
	roomKey := nodeHarvestKey(nodeID)
	table := loadHarvestTable(exp)
	regionMap := table[regionKey]
	if regionMap == nil {
		regionMap = map[string][]HarvestNode{}
	}
	regionMap[roomKey] = nodes
	table[regionKey] = regionMap
	exp.RegionState[regionStateHarvestKey] = table
	return true
}

// currentRoomIndexFor returns the room index of the active region run
// for an expedition, or -1 if no run is linked or the run can't load.
func currentRoomIndexFor(e *Expedition) int {
	if e == nil || e.RunID == "" {
		return -1
	}
	run, err := getZoneRun(e.RunID)
	if err != nil || run == nil {
		return -1
	}
	return run.CurrentRoom
}

// currentNodeIDFor returns the active region run's current graph node
// id, or "" if no run is linked / loadable. Used by harvest call sites
// that need to address the node-id-keyed harvest table from event
// handlers (no DungeonRun on hand).
func currentNodeIDFor(e *Expedition) string {
	if e == nil || e.RunID == "" {
		return ""
	}
	run, err := getZoneRun(e.RunID)
	if err != nil || run == nil {
		return ""
	}
	return harvestNodeIDFor(run)
}

// harvestNodeIDFor resolves the storage-key node id for a run. Prefers
// the dual-written CurrentNode; falls back to the legacy linear
// derivation when a row predates G4. Never returns "" for an active
// run with a populated RoomSeq.
func harvestNodeIDFor(run *DungeonRun) string {
	if run == nil {
		return ""
	}
	if run.CurrentNode != "" {
		return run.CurrentNode
	}
	return deriveLegacyNodeID(run.ZoneID, run.CurrentRoom)
}
