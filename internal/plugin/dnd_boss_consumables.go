package plugin

import (
	"log/slog"
	"sort"

	"maunium.net/go/mautrix/id"
)

// Auto-consumable rule (kept deliberately simple): the only pre-fight
// auto-pick is the panic heal. Other consumables (Coal Bombs, Ink Vials,
// Wards, etc.) sit in inventory until the player explicitly engages them
// — no threat-assessor magic deciding to burn a Voidstone Shard on a
// shadow.
//
// Heal triggers in the engine when player HP drops below 60% MaxHP. Each
// trigger consumes one charge; charges = number of healing consumables
// in the inventory. Post-combat, that many heal items are removed from
// the inventory bag (cheapest tier first so players burn through low
// stock before high-tier potions).
//
// The HealItem mod amount is the BEST heal value in inventory — every
// fire heals at the highest-tier-available rate. Slight balance pad
// (favors player), big simplification (engine doesn't need a per-charge
// values slice).

// setupAutoHealFromInventory wires the heal-on-low-HP behavior from the
// player's current consumable inventory. Returns the count of charges
// it set up (0 if no healing items at all).
func setupAutoHealFromInventory(consumables []ConsumableItem, mods *CombatModifiers) int {
	bestValue := 0
	count := 0
	for i := range consumables {
		c := &consumables[i]
		if c.Def == nil || c.Def.Effect != EffectHeal {
			continue
		}
		count++
		if v := int(c.Def.Value); v > bestValue {
			bestValue = v
		}
	}
	if count == 0 {
		return 0
	}
	mods.HealItem = bestValue
	if mods.HealItemCharges < count {
		mods.HealItemCharges = count
	}
	return count
}

// isBossPhases compares by slice header (same backing array) — boss fights
// are routed through bossCombatPhases (defined in combat_engine.go) and the
// caller passes that exact reference.
func isBossPhases(phases []CombatPhase) bool {
	if len(phases) != len(bossCombatPhases) {
		return false
	}
	if len(phases) == 0 {
		return false
	}
	return &phases[0] == &bossCombatPhases[0]
}

// countHealEventsFired counts how many heal_item events the engine emitted
// during this fight — equals the number of heal charges actually consumed
// and therefore the number of inventory items to remove.
func countHealEventsFired(result CombatResult) int {
	n := 0
	for _, e := range result.Events {
		if e.Action == "heal_item" {
			n++
		}
	}
	return n
}

// consumeFiredHealingItems removes `fired` healing consumables from the
// player's inventory, cheapest-tier first so players burn cheap stock
// before high-tier potions. No-op if fired <= 0.
func consumeFiredHealingItems(userID id.UserID, fired int) {
	if fired <= 0 {
		return
	}
	items, err := loadAdvInventory(userID)
	if err != nil {
		slog.Error("auto-heal cleanup: load inventory failed", "user", userID, "err", err)
		return
	}
	// Filter to heal items, sort cheapest-tier-first.
	type entry struct {
		id    int64
		tier  int
		name  string
	}
	var heals []entry
	for _, item := range items {
		if item.Type != "consumable" {
			continue
		}
		def := consumableDefByName(item.Name)
		if def == nil || def.Effect != EffectHeal {
			continue
		}
		heals = append(heals, entry{id: item.ID, tier: def.Tier, name: item.Name})
	}
	sort.SliceStable(heals, func(i, j int) bool { return heals[i].tier < heals[j].tier })
	removed := 0
	for _, h := range heals {
		if removed >= fired {
			break
		}
		if rerr := removeAdvInventoryItem(h.id); rerr != nil {
			slog.Error("auto-heal cleanup: remove failed", "user", userID, "item", h.name, "err", rerr)
			continue
		}
		removed++
	}
	if removed < fired {
		slog.Warn("auto-heal cleanup: short on inventory",
			"user", userID, "wanted", fired, "removed", removed)
	}
}
