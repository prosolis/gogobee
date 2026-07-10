package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

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

// consumableDefByName returns the definition for a consumable by name. Falls
// through to the magic-item registry so SRD potions/scrolls picked up as loot
// or bought from Luigi's Curios shelf resolve in combat without being added
// to the hardcoded consumableDefs table (see magic_items_gameplay.go).
func consumableDefByName(name string) *ConsumableDef {
	for i := range consumableDefs {
		if consumableDefs[i].Name == name {
			return &consumableDefs[i]
		}
	}
	return magicItemConsumableDefByName(name)
}

// ── Threat Assessment ────────────────────────────────────────────────────────

type threatLevel int

const (
	threatTrivial     threatLevel = iota // < 0.4
	threatEasy                           // 0.4–0.7
	threatCompetitive                    // 0.7–1.0
	threatDangerous                      // > 1.0
)

// assessThreat estimates rounds-to-kill in both directions using the same
// penetration model as the combat engine, then bands the threat by the ratio
// of player-RTK to enemy-RTK. Old additive HP+Attack model ignored Defense and
// rated even fights as "trivial" — players died after consumables were skipped.
func assessThreat(player, enemy CombatStats) threatLevel {
	const K = 40.0
	dprPlayer := float64(player.Attack) * (K / (K + float64(enemy.Defense)))
	dprEnemy := float64(enemy.Attack) * (K / (K + float64(player.Defense)))
	if dprPlayer < 1 {
		dprPlayer = 1
	}
	if dprEnemy < 1 {
		dprEnemy = 1
	}
	rtkEnemy := float64(enemy.MaxHP) / dprPlayer  // rounds for player to kill enemy
	rtkPlayer := float64(player.MaxHP) / dprEnemy // rounds for enemy to kill player
	if rtkPlayer <= 0 {
		return threatDangerous
	}
	// ratio < 1: player wins the race (lower = more dominant). >1: enemy wins.
	ratio := rtkEnemy / rtkPlayer
	switch {
	case ratio < 0.35:
		return threatTrivial
	case ratio < 0.65:
		return threatEasy
	case ratio < 1.0:
		return threatCompetitive
	default:
		return threatDangerous
	}
}

// ── Selection Logic ──────────────────────────────────────────────────────────

// SelectConsumables picks up to 2 items (1 offensive + 1 defensive) from inventory.
// contentTier is retained on the signature for callers but no longer caps
// what gets considered — if it's in your bag, the picker can use it.
// allowSkipTrivial=true lets the picker bail out of obvious wins to save
// items — appropriate for dungeon dives full of chump rooms, but wrong for
// arena/zone elite/boss encounters where the legacy threat assessor
// underestimates d20-mode lethality (one bad nat-20 streak ends the run).
//
// Resource-saving still happens via betterOffensive/betterDefensive, which
// prefer lower-tier items on non-Dangerous threats. Players with mixed
// inventories don't burn a Voidstone Shard on a goblin; players with only
// high-tier items now actually get to use them.
func SelectConsumables(inventory []ConsumableItem, playerStats, enemyStats CombatStats, arenaRound int, contentTier int, allowSkipTrivial bool) []ConsumableItem {
	_ = contentTier // retained for caller compatibility
	threat := assessThreat(playerStats, enemyStats)
	if threat == threatTrivial && allowSkipTrivial && arenaRound == 0 {
		return nil
	}

	var bestOffensive, bestDefensive *ConsumableItem

	for i := range inventory {
		item := &inventory[i]
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
	return consumableAdvItem(consumableDefByName(names[rand.IntN(len(names))]))
}

// consumableAdvItem builds the inventory row for a consumable def. Drop-only
// consumables carry no shop price, so their sell value falls back to a
// per-tier baseline.
func consumableAdvItem(def *ConsumableDef) *AdvItem {
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

// consumableCache draws n consumables from the dungeon drop pool at `tier`,
// used by guaranteed grants (milestone caches) rather than the drop roll.
// Tier 1 has no dungeon pool, so it yields the buyable Berry Poultice.
func consumableCache(tier, n int) []AdvItem {
	names := consumableDropTable[AdvActivityDungeon][tier]
	if len(names) == 0 {
		names = []string{"Berry Poultice"}
	}
	out := make([]AdvItem, 0, n)
	for range n {
		if item := consumableAdvItem(consumableDefByName(names[rand.IntN(len(names))])); item != nil {
			out = append(out, *item)
		}
	}
	return out
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

// renderRecipesKnown returns a player-facing list of recipes available at
// their current foraging level, plus a teaser line for the next unlock
// threshold. Hides exact ingredient lists for recipes the player hasn't
// unlocked — only the count of locked recipes is shown.
func renderRecipesKnown(foragingLevel int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🧪 **Crafting Recipes** — Foraging Lv.%d\n\n", foragingLevel))

	if foragingLevel < 10 {
		sb.WriteString("_Auto-crafting unlocks at Foraging Lv.10. Keep gathering._")
		return sb.String()
	}

	// Group recipes by tier, list those known.
	known := map[int][]CraftingRecipe{}
	totalKnown := 0
	totalLocked := 0
	nextUnlock := 0
	for _, r := range craftingRecipes {
		if r.MinForaging <= foragingLevel {
			known[r.Tier] = append(known[r.Tier], r)
			totalKnown++
		} else {
			totalLocked++
			if nextUnlock == 0 || r.MinForaging < nextUnlock {
				nextUnlock = r.MinForaging
			}
		}
	}

	for tier := 1; tier <= 5; tier++ {
		recipes := known[tier]
		if len(recipes) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("**Tier %d** (Foraging %d+)\n", tier, recipes[0].MinForaging))
		for _, r := range recipes {
			rate := craftingSuccessRate(foragingLevel, r.MinForaging) * 100
			sb.WriteString(fmt.Sprintf("  • %s — %s (%.0f%% success)\n",
				r.Result, strings.Join(r.Ingredients, " + "), rate))
		}
	}

	sb.WriteString(fmt.Sprintf("\nKnown: %d · ", totalKnown))
	if totalLocked == 0 {
		sb.WriteString("All recipes unlocked. ⭐")
	} else {
		sb.WriteString(fmt.Sprintf("Locked: %d (next unlock at Foraging Lv.%d)", totalLocked, nextUnlock))
	}

	maxCrafts := 1 + (foragingLevel-10)/10
	sb.WriteString(fmt.Sprintf("\nMax auto-crafts per combat: %d.", maxCrafts))
	return sb.String()
}

// craftXPRewards: per-tier foraging XP granted on successful (and tiny on
// failed) auto-crafts. Successful T1 ≈ 30% of a Foraging Success haul, scaling
// up so T5 grants are meaningful. Failures get a token consolation grant —
// you tried.
var craftXPSuccess = map[int]int{1: 12, 2: 25, 3: 40, 4: 60, 5: 90}
var craftXPFailure = map[int]int{1: 3, 2: 5, 3: 8, 4: 12, 5: 18}

// autoCraftConsumables scans the player's inventory for craftable recipes and
// attempts to craft the highest-tier recipe available. Consumes ingredients on
// attempt; on failure one ingredient is lost. Returns crafted items and results.
//
// Side effects: grants foraging XP per attempt (success > failure) and bumps
// the player's CraftsSucceeded counter on each success.
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
			// Foraging XP + lifetime counter bump.
			if char, err := loadAdvCharacter(userID); err == nil {
				char.ForagingXP += craftXPSuccess[bestRecipe.Tier]
				char.CraftsSucceeded++
				checkAdvLevelUp(char, "foraging")
				_ = saveAdvCharacter(char)
			}
		} else {
			// Failure: all ingredients destroyed
			for _, id := range ingredientIDs {
				removeAdvInventoryItem(id)
			}
			remaining = removeItemsByIDs(remaining, ingredientIDs)
			results = append(results, CraftResult{Recipe: bestRecipe, Success: false})
			// Token foraging XP for the attempt.
			if char, err := loadAdvCharacter(userID); err == nil {
				char.ForagingXP += craftXPFailure[bestRecipe.Tier]
				checkAdvLevelUp(char, "foraging")
				_ = saveAdvCharacter(char)
			}
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
