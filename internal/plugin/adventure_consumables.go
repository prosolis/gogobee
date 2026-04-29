package plugin

import (
	"log/slog"
	"math/rand/v2"

	"maunium.net/go/mautrix/id"
)

// ── Consumable Definitions ───────────────────────────────────────────────────
// Items that are auto-consumed during combat. Players don't choose when to use
// them — the engine selects up to 2 (1 offensive + 1 defensive) based on threat
// assessment to avoid wasting items on trivial fights.

type ConsumableEffect string

const (
	EffectHeal      ConsumableEffect = "heal"
	EffectDefBoost  ConsumableEffect = "def_boost"
	EffectAtkBoost  ConsumableEffect = "atk_boost"
	EffectWard      ConsumableEffect = "ward"
	EffectSpeedBoost ConsumableEffect = "speed_boost"
	EffectCritBoost ConsumableEffect = "crit_boost"
	EffectFlatDmg   ConsumableEffect = "flat_dmg"
	EffectSpore     ConsumableEffect = "spore"
	EffectReflect   ConsumableEffect = "reflect"
	EffectAutoCrit  ConsumableEffect = "auto_crit"
)

type ConsumableDef struct {
	Name     string
	Effect   ConsumableEffect
	Value    float64 // meaning depends on effect
	Tier     int
	Buyable  bool
	Price    int64
	Slot     string // "offensive" or "defensive"
}

// ConsumableItem is an AdvItem that has consumable properties.
type ConsumableItem struct {
	InventoryID int64
	Def         *ConsumableDef
}

var consumableDefs = []ConsumableDef{
	// T1
	{Name: "Berry Poultice", Effect: EffectHeal, Value: 12, Tier: 1, Buyable: true, Price: 400, Slot: "defensive"},
	// T2 — drop only (forage, mine, dungeon)
	{Name: "Herb Salve", Effect: EffectHeal, Value: 20, Tier: 2, Slot: "defensive"},
	{Name: "Mushroom Brew", Effect: EffectDefBoost, Value: 0.20, Tier: 2, Slot: "defensive"},
	{Name: "Coal Bomb", Effect: EffectFlatDmg, Value: 8, Tier: 2, Slot: "offensive"},
	// T3 — drop only
	{Name: "Goblin Grease", Effect: EffectAtkBoost, Value: 0.25, Tier: 3, Slot: "offensive"},
	{Name: "Quartz Ward", Effect: EffectWard, Value: 1, Tier: 3, Slot: "defensive"},
	{Name: "Blooper Ink Vial", Effect: EffectSpeedBoost, Value: 0.30, Tier: 3, Slot: "offensive"},
	{Name: "Spore Cloud", Effect: EffectSpore, Value: 2, Tier: 3, Slot: "defensive"},
	// T4
	{Name: "Spirit Tonic", Effect: EffectHeal, Value: 40, Tier: 4, Buyable: false, Slot: "defensive"},
	{Name: "Sapphire Elixir", Effect: EffectCritBoost, Value: 0.15, Tier: 4, Buyable: false, Slot: "offensive"},
	// T5
	{Name: "Ancient Artifact Oil", Effect: EffectAutoCrit, Value: 0.35, Tier: 5, Buyable: false, Slot: "offensive"},
	{Name: "Voidstone Shard", Effect: EffectReflect, Value: 0.50, Tier: 5, Buyable: false, Slot: "defensive"},
}

// consumableDefByName returns the definition for a consumable by name.
func consumableDefByName(name string) *ConsumableDef {
	for i := range consumableDefs {
		if consumableDefs[i].Name == name {
			return &consumableDefs[i]
		}
	}
	return nil
}

// ── Threat Assessment ────────────────────────────────────────────────────────

type threatLevel int

const (
	threatTrivial     threatLevel = iota // < 0.4
	threatEasy                           // 0.4–0.7
	threatCompetitive                    // 0.7–1.0
	threatDangerous                      // > 1.0
)

func assessThreat(player, enemy CombatStats) threatLevel {
	playerPower := float64(player.MaxHP + player.Attack*3)
	enemyPower := float64(enemy.MaxHP + enemy.Attack*3)
	if playerPower == 0 {
		return threatDangerous
	}
	ratio := enemyPower / playerPower
	switch {
	case ratio < 0.4:
		return threatTrivial
	case ratio < 0.7:
		return threatEasy
	case ratio < 1.0:
		return threatCompetitive
	default:
		return threatDangerous
	}
}

// ── Selection Logic ──────────────────────────────────────────────────────────

// SelectConsumables picks up to 2 items (1 offensive + 1 defensive) from inventory.
// contentTier caps consumable tier to the content being fought (0 = no cap).
func SelectConsumables(inventory []ConsumableItem, playerStats, enemyStats CombatStats, arenaRound int, contentTier int) []ConsumableItem {
	threat := assessThreat(playerStats, enemyStats)
	if threat == threatTrivial {
		return nil
	}

	maxTier := maxConsumableTier(threat, arenaRound)
	if contentTier > 0 && contentTier < maxTier {
		maxTier = contentTier
	}

	var bestOffensive, bestDefensive *ConsumableItem

	for i := range inventory {
		item := &inventory[i]
		if item.Def.Tier > maxTier {
			continue
		}

		switch item.Def.Slot {
		case "offensive":
			if bestOffensive == nil || betterOffensive(item, bestOffensive, threat) {
				bestOffensive = item
			}
		case "defensive":
			if bestDefensive == nil || betterDefensive(item, bestDefensive, threat, playerStats, enemyStats) {
				bestDefensive = item
			}
		}
	}

	var selected []ConsumableItem
	if bestOffensive != nil {
		selected = append(selected, *bestOffensive)
	}
	if bestDefensive != nil {
		selected = append(selected, *bestDefensive)
	}
	return selected
}

func maxConsumableTier(threat threatLevel, arenaRound int) int {
	base := 1
	switch threat {
	case threatEasy:
		base = 2
	case threatCompetitive:
		base = 3
	case threatDangerous:
		base = 5
	}
	// Arena: spend freely on later rounds
	if arenaRound >= 3 {
		base = min(5, base+1)
	}
	return base
}

// betterOffensive prefers lowest tier that's appropriate for the threat.
func betterOffensive(candidate, current *ConsumableItem, threat threatLevel) bool {
	if threat == threatDangerous {
		return candidate.Def.Tier > current.Def.Tier
	}
	return candidate.Def.Tier < current.Def.Tier
}

// betterDefensive picks heal items for longer fights, wards for burst threats.
func betterDefensive(candidate, current *ConsumableItem, threat threatLevel, player, enemy CombatStats) bool {
	if threat == threatDangerous {
		return candidate.Def.Tier > current.Def.Tier
	}
	// Prefer wards against high-attack enemies
	if enemy.Attack > player.MaxHP/4 && candidate.Def.Effect == EffectWard {
		return true
	}
	return candidate.Def.Tier < current.Def.Tier
}

// ── Modifier Application ─────────────────────────────────────────────────────

// consumableDropTable maps activity types to consumable names available at each tier.
// Drop chance is 15% per activity completion at T2+.
var consumableDropTable = map[AdvActivityType]map[int][]string{
	AdvActivityForaging: {
		2: {"Herb Salve"},
		3: {"Spore Cloud"},
		4: {"Spirit Tonic"},
		5: {"Spirit Tonic", "Voidstone Shard"},
	},
	AdvActivityMining: {
		2: {"Coal Bomb"},
		3: {"Quartz Ward"},
		4: {"Sapphire Elixir"},
		5: {"Voidstone Shard"},
	},
	AdvActivityFishing: {
		2: {"Herb Salve"},
		3: {"Blooper Ink Vial"},
		4: {"Blooper Ink Vial", "Spirit Tonic"},
		5: {"Blooper Ink Vial", "Voidstone Shard"},
	},
	AdvActivityDungeon: {
		2: {"Herb Salve", "Coal Bomb"},
		3: {"Goblin Grease", "Quartz Ward"},
		4: {"Sapphire Elixir", "Spirit Tonic"},
		5: {"Ancient Artifact Oil", "Voidstone Shard"},
	},
}

// RollConsumableDrop checks whether a consumable item drops from an activity.
// Returns a consumable AdvItem or nil.
func RollConsumableDrop(activity AdvActivityType, tier int) *AdvItem {
	actTable, ok := consumableDropTable[activity]
	if !ok {
		return nil
	}
	names, ok := actTable[tier]
	if !ok || len(names) == 0 {
		return nil
	}
	if rand.Float64() >= 0.15 {
		return nil
	}
	name := names[rand.IntN(len(names))]
	def := consumableDefByName(name)
	if def == nil {
		return nil
	}
	sellValue := def.Price / 2
	if sellValue == 0 {
		tierSellValues := map[int]int64{1: 200, 2: 600, 3: 1500, 4: 3000, 5: 6000}
		sellValue = tierSellValues[def.Tier]
	}
	return &AdvItem{
		Name:  def.Name,
		Type:  "consumable",
		Tier:  def.Tier,
		Value: sellValue,
	}
}

// BuyableConsumables returns all consumable defs that can be purchased from the shop.
func BuyableConsumables() []ConsumableDef {
	var result []ConsumableDef
	for _, def := range consumableDefs {
		if def.Buyable {
			result = append(result, def)
		}
	}
	return result
}

// ApplyConsumableMods adjusts the CombatStats and CombatModifiers for selected consumables.
func ApplyConsumableMods(stats *CombatStats, mods *CombatModifiers, items []ConsumableItem) {
	for _, item := range items {
		switch item.Def.Effect {
		case EffectHeal:
			mods.HealItem = int(item.Def.Value)
		case EffectDefBoost:
			mods.DamageReduct *= (1 - item.Def.Value) // 0.20 → 0.80x damage taken
		case EffectAtkBoost:
			mods.DamageBonus += item.Def.Value
		case EffectWard:
			mods.WardCharges = int(item.Def.Value)
		case EffectSpeedBoost:
			stats.Speed = int(float64(stats.Speed) * (1 + item.Def.Value))
		case EffectCritBoost:
			stats.CritRate += item.Def.Value
		case EffectFlatDmg:
			mods.FlatDmgStart = int(item.Def.Value)
		case EffectSpore:
			mods.SporeCloud = int(item.Def.Value)
		case EffectReflect:
			mods.ReflectNext = item.Def.Value
		case EffectAutoCrit:
			mods.AutoCritFirst = true
			mods.DamageBonus += item.Def.Value // +35% attack too
		}
	}
}

// ── Crafting System ─────────────────────────────────────────────────────────
// Auto-crafting happens before combat. If the player has sufficient foraging
// level and the right ingredients in inventory, consumables are assembled
// automatically. Failed crafts consume one ingredient.

type CraftingRecipe struct {
	Result      string   // consumable name to produce
	Ingredients []string // required item names (matched by inventory Name)
	MinForaging int      // foraging level required
	Tier        int
}

var craftingRecipes = []CraftingRecipe{
	// T1 — Foraging 10
	{Result: "Berry Poultice", Ingredients: []string{"Berries", "Common Herbs"}, MinForaging: 10, Tier: 1},
	// T2 — Foraging 15
	{Result: "Herb Salve", Ingredients: []string{"Rare Herbs", "Honey"}, MinForaging: 15, Tier: 2},
	{Result: "Mushroom Brew", Ingredients: []string{"Mushrooms", "Hardwood"}, MinForaging: 15, Tier: 2},
	{Result: "Coal Bomb", Ingredients: []string{"Coal", "Saltpetre"}, MinForaging: 15, Tier: 2},
	// T3 — Foraging 20
	{Result: "Goblin Grease", Ingredients: []string{"Goblin Trinket", "Honey"}, MinForaging: 20, Tier: 3},
	{Result: "Quartz Ward", Ingredients: []string{"Quartz", "Iron Ore"}, MinForaging: 20, Tier: 3},
	{Result: "Blooper Ink Vial", Ingredients: []string{"Blooper Ink", "River Pearl"}, MinForaging: 20, Tier: 3},
	{Result: "Spore Cloud", Ingredients: []string{"Spores", "Rare Herbs"}, MinForaging: 20, Tier: 3},
	// T4 — Foraging 25
	{Result: "Spirit Tonic", Ingredients: []string{"Spirit Herbs", "Starfruit"}, MinForaging: 25, Tier: 4},
	{Result: "Sapphire Elixir", Ingredients: []string{"Sapphire", "Dragon Crystal"}, MinForaging: 25, Tier: 4},
	// T5 — Foraging 30
	{Result: "Ancient Artifact Oil", Ingredients: []string{"Ancient Artifact", "Dragon Scale"}, MinForaging: 30, Tier: 5},
	{Result: "Voidstone Shard", Ingredients: []string{"Voidstone", "Mythril Ore"}, MinForaging: 30, Tier: 5},
}

// craftingSuccessRate returns the success chance for a recipe given foraging level.
// Base rate is 50% at minimum level, +3% per 5 levels above minimum, capped at 95%.
func craftingSuccessRate(foragingLevel, minForaging int) float64 {
	base := 0.50
	levelsAbove := foragingLevel - minForaging
	bonus := float64(levelsAbove) / 5.0 * 0.03
	rate := base + bonus
	if rate > 0.95 {
		return 0.95
	}
	return rate
}

// CraftResult records what happened during auto-crafting for narrative output.
type CraftResult struct {
	Recipe  *CraftingRecipe
	Success bool
}

// autoCraftConsumables scans the player's inventory for craftable recipes and
// attempts to craft the highest-tier recipe available. Consumes ingredients on
// attempt; on failure one ingredient is lost. Returns crafted items and results.
func autoCraftConsumables(userID id.UserID, items []AdvItem, foragingLevel int) ([]CraftResult, []AdvItem) {
	if foragingLevel < 10 {
		return nil, items
	}

	remaining := make([]AdvItem, len(items))
	copy(remaining, items)

	var results []CraftResult
	crafted := 0
	maxCrafts := 1 + (foragingLevel-10)/10 // 1 at lv10, 2 at lv20, 3 at lv30

	// Try recipes from highest tier down
	for attempt := 0; attempt < maxCrafts; attempt++ {
		bestRecipe, ingredientIDs := findBestCraftable(remaining, foragingLevel)
		if bestRecipe == nil {
			break
		}

		rate := craftingSuccessRate(foragingLevel, bestRecipe.MinForaging)
		success := rand.Float64() < rate

		if success {
			// Remove all ingredients from inventory
			for _, id := range ingredientIDs {
				removeAdvInventoryItem(id)
			}
			remaining = removeItemsByIDs(remaining, ingredientIDs)

			// Add crafted consumable to inventory
			def := consumableDefByName(bestRecipe.Result)
			if def == nil {
				continue
			}
			sellValue := def.Price / 2
			if sellValue == 0 {
				tierSellValues := map[int]int64{1: 200, 2: 600, 3: 1500, 4: 3000, 5: 6000}
				sellValue = tierSellValues[def.Tier]
			}
			craftedItem := AdvItem{
				Name:  def.Name,
				Type:  "consumable",
				Tier:  def.Tier,
				Value: sellValue,
			}
			if err := addAdvInventoryItem(userID, craftedItem); err != nil {
				slog.Error("crafting: failed to add crafted item", "item", def.Name, "err", err)
				continue
			}
			results = append(results, CraftResult{Recipe: bestRecipe, Success: true})
			crafted++
		} else {
			// Failure: all ingredients destroyed
			for _, id := range ingredientIDs {
				removeAdvInventoryItem(id)
			}
			remaining = removeItemsByIDs(remaining, ingredientIDs)
			results = append(results, CraftResult{Recipe: bestRecipe, Success: false})
		}
	}

	// Reload remaining if we crafted anything (IDs changed)
	if crafted > 0 {
		reloaded, err := loadAdvInventory(userID)
		if err == nil {
			remaining = reloaded
		}
	}

	return results, remaining
}

// findBestCraftable returns the highest-tier recipe the player can craft
// from their current inventory, along with the inventory IDs of the ingredients.
func findBestCraftable(items []AdvItem, foragingLevel int) (*CraftingRecipe, []int64) {
	var bestRecipe *CraftingRecipe
	var bestIDs []int64

	for i := range craftingRecipes {
		recipe := &craftingRecipes[i]
		if foragingLevel < recipe.MinForaging {
			continue
		}

		ids := matchIngredients(items, recipe.Ingredients)
		if ids == nil {
			continue
		}

		if bestRecipe == nil || recipe.Tier > bestRecipe.Tier {
			bestRecipe = recipe
			bestIDs = ids
		}
	}

	return bestRecipe, bestIDs
}

// matchIngredients checks if inventory contains all ingredients for a recipe.
// Returns the inventory item IDs to consume, or nil if not all found.
func matchIngredients(items []AdvItem, ingredients []string) []int64 {
	used := make(map[int]bool)
	var ids []int64

	for _, need := range ingredients {
		found := false
		for j, item := range items {
			if used[j] {
				continue
			}
			if item.Name == need && item.Type != "consumable" && item.Type != "MasterworkGear" && item.Type != "ArenaGear" && item.Type != "card" {
				used[j] = true
				ids = append(ids, item.ID)
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return ids
}

func removeItemsByIDs(items []AdvItem, ids []int64) []AdvItem {
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result []AdvItem
	for _, item := range items {
		if !idSet[item.ID] {
			result = append(result, item)
		}
	}
	return result
}
