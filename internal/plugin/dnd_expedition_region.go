package plugin

import "gogobee/internal/db"

// Phase 12 E4a — Multi-Region Zone data model and registry.
// Spec: gogobee_expedition_system.md §11.
//
// Tier 4–5 zones (Underdark, Dragon's Lair, Abyss Portal) are too
// large for a single sequence of rooms; they are divided into Regions,
// each functioning as a sub-dungeon with its own room sequence, region
// boss, base-camp eligibility, enemy subset and loot table.
//
// E4a ships the data layer: ExpeditionRegion struct, the per-zone
// region registries, and persistence helpers that serialize cleared /
// boss-defeated state into Expedition.RegionState.
//
// E4b wires hooks for marking a region boss defeated / region cleared
// and surfaces the current-region label in !expedition status. E4c
// adds the !region travel command (1 in-game day + supply burn +
// wandering check). E4d flips !camp base from the deferred branch in
// dnd_expedition_camp.go to the per-region waypoint.

// ExpeditionRegion is the in-memory shape of one sub-dungeon within
// a multi-region zone. (Spec §11.4.) Mirrors the spec struct verbatim
// except `LootTable` is intentionally omitted here — region loot is
// drawn from the parent zone's loot table for now; a region-specific
// override lands with the loot/encounter rebalance phase.
type ExpeditionRegion struct {
	ID           string   // stable id, e.g. "underdark_drow_outpost"
	ZoneID       ZoneID   // parent zone
	Name         string   // display name, e.g. "Drow Outpost"
	Order        int      // 1-indexed order within the zone
	BaseCampSite bool     // §11.1 — eligible for !camp base post-boss
	RegionBoss   string   // descriptive name (§11.2 column 3)
	IsZoneBoss   bool     // last region — boss is the zone boss itself
	EnemySubset  []string // bestiary IDs (subset of zone roster)
}

// regionsByZone is the static registry. Only multi-region zones appear;
// single-region zones (everything else) return nil from regionsForZone.
var regionsByZone = map[ZoneID][]ExpeditionRegion{
	ZoneUnderdark: {
		{
			ID: "underdark_surface_tunnels", ZoneID: ZoneUnderdark, Order: 1,
			Name: "Surface Tunnels", BaseCampSite: false,
			RegionBoss:  "Tunnel Predator (mini-boss)",
			EnemySubset: []string{"hook_horror"},
		},
		{
			ID: "underdark_drow_outpost", ZoneID: ZoneUnderdark, Order: 2,
			Name: "Drow Outpost", BaseCampSite: true,
			RegionBoss:  "Drow Elite Captain",
			EnemySubset: []string{"drow", "drow_elite_warrior", "drow_mage"},
		},
		{
			ID: "underdark_illithid_warren", ZoneID: ZoneUnderdark, Order: 3,
			Name: "Illithid Warren", BaseCampSite: true,
			RegionBoss:  "Mind Flayer Elder",
			EnemySubset: []string{"mind_flayer", "roper"},
		},
		{
			ID: "underdark_deep_throne", ZoneID: ZoneUnderdark, Order: 4,
			Name: "The Deep Throne", BaseCampSite: false,
			RegionBoss: "Ilvaras Xunyl (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"drow", "drow_elite_warrior", "drow_mage"},
		},
	},
	ZoneDragonsLair: {
		{
			ID: "dragons_lair_kobold_warrens", ZoneID: ZoneDragonsLair, Order: 1,
			Name: "Kobold Warrens", BaseCampSite: false,
			RegionBoss:  "Kobold Warchief",
			EnemySubset: []string{"kobold", "kobold_scale_sorcerer"},
		},
		{
			ID: "dragons_lair_drake_pens", ZoneID: ZoneDragonsLair, Order: 2,
			Name: "Drake Pens", BaseCampSite: true,
			RegionBoss:  "Elder Drake",
			EnemySubset: []string{"guard_drake", "dragonborn_cultist"},
		},
		{
			ID: "dragons_lair_the_vault", ZoneID: ZoneDragonsLair, Order: 3,
			Name: "The Vault", BaseCampSite: true,
			RegionBoss:  "Hoard Wyrmling Pair",
			EnemySubset: []string{"young_red_dragon", "dragonborn_cultist"},
		},
		{
			ID: "dragons_lair_infernax_chamber", ZoneID: ZoneDragonsLair, Order: 4,
			Name: "Infernax's Chamber", BaseCampSite: false,
			RegionBoss: "Infernax (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"young_red_dragon"},
		},
	},
	ZoneAbyssPortal: {
		{
			ID: "abyss_outer_rift", ZoneID: ZoneAbyssPortal, Order: 1,
			Name: "Outer Rift", BaseCampSite: false,
			RegionBoss:  "Vrock Commander",
			EnemySubset: []string{"quasit", "vrock"},
		},
		{
			ID: "abyss_demon_assembly", ZoneID: ZoneAbyssPortal, Order: 2,
			Name: "Demon Assembly", BaseCampSite: true,
			RegionBoss:  "Nalfeshnee",
			EnemySubset: []string{"vrock", "hezrou", "nalfeshnee"},
		},
		{
			ID: "abyss_wardens_post", ZoneID: ZoneAbyssPortal, Order: 3,
			Name: "The Warden's Post", BaseCampSite: true,
			RegionBoss:  "Marilith Warden",
			EnemySubset: []string{"hezrou", "nalfeshnee", "marilith"},
		},
		{
			ID: "abyss_the_tear", ZoneID: ZoneAbyssPortal, Order: 4,
			Name: "The Tear", BaseCampSite: false,
			RegionBoss: "Belaxath (Zone Boss)", IsZoneBoss: true,
			EnemySubset: []string{"marilith"},
		},
	},
}

// regionsForZone returns the ordered region list for `z`, or nil if `z`
// is a single-region zone.
func regionsForZone(z ZoneID) []ExpeditionRegion {
	rs, ok := regionsByZone[z]
	if !ok {
		return nil
	}
	out := make([]ExpeditionRegion, len(rs))
	copy(out, rs)
	return out
}

// IsMultiRegionZone reports whether `z` has a region structure.
func IsMultiRegionZone(z ZoneID) bool {
	_, ok := regionsByZone[z]
	return ok
}

// findRegion looks up by ID within a zone. Returns the region and true,
// or zero-value and false.
func findRegion(z ZoneID, regionID string) (ExpeditionRegion, bool) {
	for _, r := range regionsForZone(z) {
		if r.ID == regionID {
			return r, true
		}
	}
	return ExpeditionRegion{}, false
}

// regionByOrder returns the region at 1-indexed `order`, or zero+false.
func regionByOrder(z ZoneID, order int) (ExpeditionRegion, bool) {
	for _, r := range regionsForZone(z) {
		if r.Order == order {
			return r, true
		}
	}
	return ExpeditionRegion{}, false
}

// firstRegion returns the entry region for a multi-region zone (Order
// == 1), or zero+false for single-region zones.
func firstRegion(z ZoneID) (ExpeditionRegion, bool) {
	return regionByOrder(z, 1)
}

// nextRegion returns the next-in-order region after `cur`, or zero+false
// if `cur` is the last region.
func nextRegion(z ZoneID, cur string) (ExpeditionRegion, bool) {
	r, ok := findRegion(z, cur)
	if !ok {
		return ExpeditionRegion{}, false
	}
	return regionByOrder(z, r.Order+1)
}

// CurrentRegion returns the player's current ExpeditionRegion, or
// zero+false if the zone is single-region or the stored CurrentRegion
// is empty / unrecognized.
func CurrentRegion(e *Expedition) (ExpeditionRegion, bool) {
	if e == nil || !IsMultiRegionZone(e.ZoneID) {
		return ExpeditionRegion{}, false
	}
	if e.CurrentRegion == "" {
		return firstRegion(e.ZoneID)
	}
	return findRegion(e.ZoneID, e.CurrentRegion)
}

// ── progression state stored in RegionState ────────────────────────
//
// RegionState["regions_cleared"]      []string  — region IDs whose
//                                                 region boss has been
//                                                 defeated.
// RegionState["regions_visited"]      []string  — region IDs the player
//                                                 has set foot in.
// RegionState["base_camps"]           []string  — region IDs where a
//                                                 persistent base camp
//                                                 has been pitched.

const (
	regionStateClearedKey  = "regions_cleared"
	regionStateVisitedKey  = "regions_visited"
	regionStateBaseCampKey = "base_camps"
)

// regionListFromState reads a string-list slot from RegionState. JSON
// round-trips from sqlite turn []string into []any, so we accept both.
func regionListFromState(e *Expedition, key string) []string {
	if e == nil || e.RegionState == nil {
		return nil
	}
	raw, ok := e.RegionState[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// regionListContains reports membership.
func regionListContains(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// addRegionListEntry adds `id` to the list at RegionState[key] if it
// isn't already there. Persists. Returns true if the list changed.
func addRegionListEntry(e *Expedition, key, id string) (bool, error) {
	cur := regionListFromState(e, key)
	if regionListContains(cur, id) {
		return false, nil
	}
	cur = append(cur, id)
	if e.RegionState == nil {
		e.RegionState = map[string]any{}
	}
	e.RegionState[key] = cur
	return true, persistRegionState(e)
}

// IsRegionCleared reports whether the region's boss has been defeated.
func IsRegionCleared(e *Expedition, regionID string) bool {
	return regionListContains(regionListFromState(e, regionStateClearedKey), regionID)
}

// IsRegionVisited reports whether the player has entered the region.
func IsRegionVisited(e *Expedition, regionID string) bool {
	return regionListContains(regionListFromState(e, regionStateVisitedKey), regionID)
}

// HasBaseCampAt reports whether a persistent base camp exists in the
// region (§11.1: unlocked only after region boss is cleared).
func HasBaseCampAt(e *Expedition, regionID string) bool {
	return regionListContains(regionListFromState(e, regionStateBaseCampKey), regionID)
}

// MarkRegionVisited records that the player has set foot in `regionID`.
// Idempotent. Returns true if state changed.
func MarkRegionVisited(e *Expedition, regionID string) (bool, error) {
	if e == nil || regionID == "" {
		return false, nil
	}
	return addRegionListEntry(e, regionStateVisitedKey, regionID)
}

// MarkRegionBossDefeated records that the region boss for `regionID`
// has been killed. If the region's IsZoneBoss flag is set, the
// expedition's BossDefeated zone-level flag is also flipped (§7
// suppression hooks read it). Idempotent. Returns true if state
// changed.
//
// Combat-link phase calls this from the per-region boss-kill resolver.
func MarkRegionBossDefeated(e *Expedition, regionID string) (bool, error) {
	if e == nil || regionID == "" {
		return false, nil
	}
	region, ok := findRegion(e.ZoneID, regionID)
	if !ok {
		return false, nil
	}
	changed, err := addRegionListEntry(e, regionStateClearedKey, regionID)
	if err != nil {
		return changed, err
	}
	if region.IsZoneBoss && !e.BossDefeated {
		e.BossDefeated = true
		if _, err := db.Get().Exec(`
			UPDATE dnd_expedition
			   SET boss_defeated = 1,
			       last_activity = CURRENT_TIMESTAMP
			 WHERE expedition_id = ?`, e.ID); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// setCurrentRegion writes a new CurrentRegion. Used by E4c travel.
func setCurrentRegion(e *Expedition, regionID string) error {
	e.CurrentRegion = regionID
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET current_region = ?,
		       last_activity = CURRENT_TIMESTAMP
		 WHERE expedition_id = ?`, regionID, e.ID)
	return err
}
