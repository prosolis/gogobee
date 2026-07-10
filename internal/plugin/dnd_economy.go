package plugin

// Phase R5 — Thom Krooke economy: !sell, !craft, !lore.
// Spec: gogobee_resource_combat_integration.md §8.
//
// !sell      Post-expedition only. Liquidates harvested materials/fish/items
//            from inventory at the §8.1 sell-value baseline already stored
//            in AdvItem.Value. CHA Persuasion DC 17 — pass = +15% on the
//            entire transaction. Excludes equipment (Masterwork/Arena/etc.).
// !craft     Manual crafting at Thom Krooke from the §8.2 exemplar recipes.
//            Requires the recipe to be discovered (!lore) and all named
//            ingredients in inventory. Consumes ingredients, deposits the
//            output as a new AdvItem.
// !lore      INT/Arcana DC 15 — on success, reveals one undiscovered recipe.
//            Up to one attempt per long rest (uses dnd_character.last_long_rest_at
//            via the existing rest plumbing as a soft cooldown anchor).

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"strings"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── Recipe Registry (§8.2) ───────────────────────────────────────────────────

// ThomCraftRecipe is one §8.2 recipe. Ingredient names match the registry
// Name field exactly so inventory lookups succeed without fuzzy matching.
type ThomCraftRecipe struct {
	ID          string
	Name        string
	Ingredients map[string]int // Name → count (exact match)
	// BladeWeaponCount is the count of generic "any blade weapon"
	// inventory items consumed (§8.2 Drow Poison Blade). 0 = none.
	BladeWeaponCount int
	OutputName       string
	OutputType       string // material | item | weapon | armor | accessory | consumable
	OutputTier       int
	OutputValue      int64
	Description      string
	DiscoveryHint    string // shown when newly discovered via !lore
}

var thomCraftRecipes = []ThomCraftRecipe{
	{
		ID:   "potion_fire_resist",
		Name: "Potion of Fire Resistance",
		Ingredients: map[string]int{
			"Salamander Oil": 1,
			"Fire Essence":   2,
		},
		OutputName:    "Potion of Fire Resistance",
		OutputType:    "consumable",
		OutputTier:    3,
		OutputValue:   220,
		Description:   "Resist fire damage for one combat. (Effect wired in R6 polish.)",
		DiscoveryHint: "Thom rumbles: \"Salamander oil thinned with two pinches of fire essence. Don't ask me how I know. The dosage matters.\"",
	},
	{
		ID:   "mithral_dagger",
		Name: "Mithral Dagger",
		Ingredients: map[string]int{
			"Mithral Trace": 1,
			"Scrap Iron":    3,
		},
		OutputName:    "Mithral Dagger",
		OutputType:    "weapon",
		OutputTier:    4,
		OutputValue:   650,
		Description:   "Light blade. Bypasses light-armor proficiency restriction. (Effect wired in R6 polish.)",
		DiscoveryHint: "Thom: \"Mithral trace alloyed with three measures of scrap iron. The cheap iron is what holds the edge. Most apprentices get that backwards.\"",
	},
	{
		ID:   "undead_ward_charm",
		Name: "Undead Ward Charm",
		Ingredients: map[string]int{
			"Silver Grave Dust": 1,
			"Wraith Core":       1,
		},
		OutputName:    "Undead Ward Charm",
		OutputType:    "accessory",
		OutputTier:    4,
		OutputValue:   720,
		Description:   "+2 to saves vs. undead. (Effect wired in R6 polish.)",
		DiscoveryHint: "Thom turns the dust through his fingers. \"Silver dust, wraith's core. Bind them, the wraith forgets it ever wanted you. Strange but reliable.\"",
	},
	{
		ID:   "shadow_step_cloak",
		Name: "Shadow Step Cloak",
		Ingredients: map[string]int{
			"Ghost Silk":     3,
			"Shadow Essence": 1,
		},
		OutputName:    "Shadow Step Cloak",
		OutputType:    "armor",
		OutputTier:    4,
		OutputValue:   780,
		Description:   "Light armor. Stealth advantage 1/day. (Effect wired in R6 polish.)",
		DiscoveryHint: "Thom: \"Three lengths of ghost silk woven against one drop of shadow essence. The silk fights you while you stitch. That's how you know it's working.\"",
	},
	{
		ID:   "drow_poison_blade",
		Name: "Drow Poison Blade",
		Ingredients: map[string]int{
			"Drow Poison (diluted)": 1,
		},
		BladeWeaponCount: 1,
		OutputName:       "Drow Poison Blade",
		OutputType:       "weapon",
		OutputTier:       4,
		OutputValue:      540,
		Description:      "Coated blade — sleep-on-crit (DC 14 CON save) for 3 combats. (Effect wired in R6 polish.)",
		DiscoveryHint:    "Thom turns a glass vial in the lamplight. \"Drow brew, drawn down to a thin coat. Bring me any blade you trust — sword, dagger, scimitar, makes no odds. The poison picks the edge.\"",
	},
}

// bladeWeaponKeywords are the name-substrings that qualify an AdvItem
// (Type=="weapon") as a blade for the §8.2 "any blade weapon" matcher.
var bladeWeaponKeywords = []string{
	"sword", "dagger", "blade", "scimitar", "rapier",
	"knife", "cutlass", "saber", "katana", "falchion",
	"shortsword", "longsword", "greatsword",
}

// isBladeWeapon reports whether an inventory item qualifies as a blade
// for crafting purposes — type "weapon" plus a blade keyword in the name.
func isBladeWeapon(it AdvItem) bool {
	if it.Type != "weapon" {
		return false
	}
	low := strings.ToLower(it.Name)
	for _, kw := range bladeWeaponKeywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// findRecipeByID returns the recipe with the given ID.
func findRecipeByID(id string) (ThomCraftRecipe, bool) {
	for _, r := range thomCraftRecipes {
		if r.ID == id {
			return r, true
		}
	}
	return ThomCraftRecipe{}, false
}

// findRecipeByName fuzzy-matches a recipe name (case-insensitive prefix or
// substring). Returns ok=false if zero or >1 matches.
func findRecipeByName(query string) (ThomCraftRecipe, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ThomCraftRecipe{}, false
	}
	var matches []ThomCraftRecipe
	for _, r := range thomCraftRecipes {
		if strings.Contains(strings.ToLower(r.Name), query) {
			matches = append(matches, r)
		}
	}
	if len(matches) != 1 {
		return ThomCraftRecipe{}, false
	}
	return matches[0], true
}

// ── Recipe discovery persistence ─────────────────────────────────────────────

func loadKnownRecipes(userID id.UserID) (map[string]bool, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT recipe_id FROM dnd_known_recipe WHERE user_id = ?`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			return nil, err
		}
		known[rid] = true
	}
	return known, rows.Err()
}

func recordKnownRecipe(userID id.UserID, recipeID string) error {
	d := db.Get()
	_, err := d.Exec(`
		INSERT OR IGNORE INTO dnd_known_recipe (user_id, recipe_id) VALUES (?, ?)`,
		string(userID), recipeID)
	return err
}

// ── !sell ────────────────────────────────────────────────────────────────────

// handleResourceSellCmd is the top-level !sell handler. The legacy
// `!adventure sell` path remains intact; this is the new R5 surface that
// pays out at §8.1 baselines and applies the CHA DC 17 +15% bump.
//
// Forms:
//
//	!sell           — list sellable inventory + total at base price
//	!sell list      — alias of bare !sell
//	!sell all       — liquidate everything sellable in one transaction
//	!sell <name>    — fuzzy-match a single item by name
func (p *AdventurePlugin) handleResourceSellCmd(ctx MessageContext, args string) error {
	args = strings.TrimSpace(args)
	lower := strings.ToLower(args)

	// Post-expedition gate. A seated member is mid-expedition too — reading
	// only their own row would let them run a shop from inside the dungeon.
	exp, _, err := activeExpeditionFor(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't check expedition status.")
	}
	if exp != nil && exp.IsActive() {
		return p.SendDM(ctx.Sender,
			"Thom Krooke doesn't deal with you mid-expedition. Extract first, then bring the goods to him.")
	}

	items, err := loadAdvInventory(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load inventory.")
	}
	sellable := filterSellable(items)
	if len(sellable) == 0 {
		return p.SendDM(ctx.Sender,
			"You've got nothing Thom Krooke wants to buy. Bring back something from an expedition first.")
	}

	if args == "" || lower == "list" {
		return p.SendDM(ctx.Sender, renderSellList(sellable))
	}

	var toSell []AdvItem
	if lower == "all" {
		toSell = sellable
	} else {
		match := findInventoryMatch(sellable, args)
		if match == nil {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"No sellable item matching '%s'. Try `!sell list` to see what Thom will take.", args))
		}
		toSell = []AdvItem{*match}
	}

	return p.executeResourceSell(ctx.Sender, toSell)
}

// filterSellable returns inventory items eligible for the §8.1 path:
// harvest materials, fish, and zone-loot items. Equipment, consumables,
// arena gear, and cards are left to the legacy `!adventure sell`.
func filterSellable(items []AdvItem) []AdvItem {
	var out []AdvItem
	for _, it := range items {
		switch it.Type {
		case "material", "fish", "item":
			out = append(out, it)
		}
	}
	return out
}

// findInventoryMatch returns the first sellable item whose name contains
// the query (case-insensitive). Returns nil if no match.
func findInventoryMatch(items []AdvItem, query string) *AdvItem {
	q := strings.ToLower(query)
	for i := range items {
		if strings.Contains(strings.ToLower(items[i].Name), q) {
			return &items[i]
		}
	}
	return nil
}

// renderSellList shows what Thom will buy with running totals.
func renderSellList(items []AdvItem) string {
	var sb strings.Builder
	sb.WriteString("**Thom Krooke** runs his thumb down the page.\n\n")

	grouped := map[string]struct {
		count int
		value int64
	}{}
	order := []string{}
	for _, it := range items {
		g, seen := grouped[it.Name]
		if !seen {
			order = append(order, it.Name)
		}
		g.count++
		g.value += it.Value
		grouped[it.Name] = g
	}

	var total int64
	for _, name := range order {
		g := grouped[name]
		total += g.value
		sb.WriteString(fmt.Sprintf("• %s × %d — €%d\n", name, g.count, g.value))
	}
	sb.WriteString(fmt.Sprintf("\nBase total: **€%d** across %d item(s).\n", total, len(items)))
	sb.WriteString("\n`!sell all` to liquidate everything · `!sell <name>` for a single item.")
	sb.WriteString("\nThom will roll a Persuasion check (DC 17) — pass = +15% on the take.")
	return sb.String()
}

// executeResourceSell consumes the items, rolls one CHA Persuasion check,
// credits the payout, and DMs the result.
func (p *AdventurePlugin) executeResourceSell(userID id.UserID, items []AdvItem) error {
	if len(items) == 0 {
		return p.SendDM(userID, "Nothing to sell.")
	}

	var base int64
	for _, it := range items {
		base += it.Value
	}

	// CHA Persuasion DC 17 — single roll for the whole transaction.
	var bumpApplied bool
	var rollLine string
	if char, err := LoadDnDCharacter(userID); err == nil && char != nil && !char.PendingSetup {
		res := performSkillCheck(char, SkillPersuasion, 17)
		bumpApplied = res.Success
		if res.Auto {
			rollLine = fmt.Sprintf("_Persuasion (auto-%s)_",
				ternary(res.Success, "success on a nat 20", "fail on a nat 1"))
		} else {
			rollLine = fmt.Sprintf("_Persuasion: rolled %d + %d = %d vs DC 17 — %s_",
				res.Roll, res.Mod, res.Total, ternary(res.Success, "+15%", "no bump"))
		}
	}

	final := base
	if bumpApplied {
		final = base + (base*15)/100
	}

	// Remove items.
	removed := 0
	for _, it := range items {
		if err := removeAdvInventoryItem(it.ID); err == nil {
			removed++
		}
	}
	if removed == 0 {
		return p.SendDM(userID, "Couldn't remove items from inventory. Nothing sold.")
	}

	if p.euro != nil {
		reason := "dnd_resource_sell"
		if len(items) == 1 {
			reason = "dnd_resource_sell_" + items[0].Name
		}
		p.euro.Credit(userID, float64(final), reason)
	}

	var sb strings.Builder
	sb.WriteString("**Thom Krooke** finishes counting and slides a stack across the counter.\n\n")
	if len(items) == 1 {
		sb.WriteString(fmt.Sprintf("Sold **%s** at base €%d.\n", items[0].Name, base))
	} else {
		sb.WriteString(fmt.Sprintf("Sold **%d items** at base total €%d.\n", removed, base))
	}
	if rollLine != "" {
		sb.WriteString(rollLine + "\n")
	}
	if bumpApplied {
		sb.WriteString(fmt.Sprintf("Final payout: **€%d** (+15%%).\n", final))
		sb.WriteString("\n_Thom: \"Reasonable.\"_")
	} else {
		sb.WriteString(fmt.Sprintf("Final payout: **€%d**.\n", final))
		sb.WriteString("\n_Thom: \"Standard rate. Don't take it personally.\"_")
	}
	return p.SendDM(userID, sb.String())
}

func ternary(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

// ── !craft ───────────────────────────────────────────────────────────────────

// handleCraftCmd dispatches `!craft`, `!craft list`, and `!craft <recipe>`.
func (p *AdventurePlugin) handleCraftCmd(ctx MessageContext, args string) error {
	args = strings.TrimSpace(args)
	lower := strings.ToLower(args)

	known, err := loadKnownRecipes(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your recipe book.")
	}

	if args == "" || lower == "list" {
		return p.SendDM(ctx.Sender, renderRecipeBook(ctx.Sender, known))
	}

	recipe, ok := findRecipeByName(args)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"No recipe matches '%s'. Try `!craft list` to see what you've discovered.", args))
	}
	if !known[recipe.ID] {
		return p.SendDM(ctx.Sender,
			"_Thom: \"Doesn't ring a bell. If you don't know the recipe, I don't either.\"_\n\nDiscover it via `!lore` first.")
	}

	return p.executeCraft(ctx.Sender, recipe)
}

// renderRecipeBook lists known recipes, ingredient lists, and a count of
// undiscovered ones.
func renderRecipeBook(userID id.UserID, known map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("📜 **Thom Krooke — Recipe Book**\n\n")

	items, _ := loadAdvInventory(userID)
	have := inventoryByName(items)

	knownCount := 0
	for _, r := range thomCraftRecipes {
		if !known[r.ID] {
			continue
		}
		knownCount++
		sb.WriteString(fmt.Sprintf("**%s**\n", r.Name))
		var parts []string
		ready := true
		for ing, need := range r.Ingredients {
			got := have[ing]
			parts = append(parts, fmt.Sprintf("%s ×%d (%d)", ing, need, got))
			if got < need {
				ready = false
			}
		}
		if r.BladeWeaponCount > 0 {
			haveBlades := 0
			for _, it := range items {
				if isBladeWeapon(it) {
					haveBlades++
				}
			}
			parts = append(parts, fmt.Sprintf("any blade weapon ×%d (%d)", r.BladeWeaponCount, haveBlades))
			if haveBlades < r.BladeWeaponCount {
				ready = false
			}
		}
		sb.WriteString("  Ingredients: " + strings.Join(parts, " + ") + "\n")
		sb.WriteString("  " + r.Description + "\n")
		if ready {
			sb.WriteString("  ✅ Ready to craft.\n")
		}
		sb.WriteString("\n")
	}

	if knownCount == 0 {
		sb.WriteString("_You don't know any recipes yet. Try `!lore` at Thom Krooke's._\n\n")
	}

	hidden := len(thomCraftRecipes) - knownCount
	if hidden > 0 {
		sb.WriteString(fmt.Sprintf("_%d undiscovered recipe(s). Use `!lore` to dig._", hidden))
	} else {
		sb.WriteString("_All recipes discovered._")
	}
	return sb.String()
}

// inventoryByName counts items grouped by name.
func inventoryByName(items []AdvItem) map[string]int {
	out := map[string]int{}
	for _, it := range items {
		out[it.Name]++
	}
	return out
}

// executeCraft consumes ingredients and deposits the output. Returns a DM.
func (p *AdventurePlugin) executeCraft(userID id.UserID, r ThomCraftRecipe) error {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return p.SendDM(userID, "Couldn't load inventory.")
	}

	// Pick item IDs to consume — first-fit per ingredient name plus any
	// generic "blade weapon" slots required by the recipe.
	consume, missing := pickIngredients(items, r.Ingredients)
	bladeIDs, bladeShort := pickBladeWeapons(items, r.BladeWeaponCount, consume)
	consume = append(consume, bladeIDs...)
	if len(missing) > 0 || bladeShort > 0 {
		var lines []string
		for ing, short := range missing {
			lines = append(lines, fmt.Sprintf("  • %s — short %d", ing, short))
		}
		if bladeShort > 0 {
			lines = append(lines, fmt.Sprintf("  • any blade weapon — short %d", bladeShort))
		}
		return p.SendDM(userID, fmt.Sprintf(
			"_Thom: \"Not enough material. Come back when you've got the rest.\"_\n\n%s",
			strings.Join(lines, "\n")))
	}

	for _, itemID := range consume {
		if err := removeAdvInventoryItem(itemID); err != nil {
			return p.SendDM(userID, "Crafting failed mid-step. Inventory may be partially consumed.")
		}
	}

	out := AdvItem{
		Name:  r.OutputName,
		Type:  r.OutputType,
		Tier:  r.OutputTier,
		Value: r.OutputValue,
	}
	if err := addAdvInventoryItem(userID, out); err != nil {
		return p.SendDM(userID, "Crafted item couldn't be deposited. Tell an admin.")
	}

	return p.SendDM(userID, fmt.Sprintf(
		"**Thom Krooke** lays the materials out, works for a moment, and slides the result back to you.\n\n"+
			"Crafted: **%s** _(tier %d, sell value €%d)_\n%s",
		r.OutputName, r.OutputTier, r.OutputValue, r.Description))
}

// pickBladeWeapons returns inventory IDs covering the requested generic
// blade-weapon count, skipping any IDs already in `exclude`. Returns the
// chosen IDs plus a shortfall count when inventory can't cover.
func pickBladeWeapons(items []AdvItem, count int, exclude []int64) ([]int64, int) {
	if count <= 0 {
		return nil, 0
	}
	used := map[int64]bool{}
	for _, id := range exclude {
		used[id] = true
	}
	var picked []int64
	for _, it := range items {
		if len(picked) >= count {
			break
		}
		if used[it.ID] {
			continue
		}
		if isBladeWeapon(it) {
			picked = append(picked, it.ID)
			used[it.ID] = true
		}
	}
	return picked, count - len(picked)
}

// pickIngredients walks the inventory and selects item IDs to consume,
// covering all required (Name → count). Returns (idsToRemove, missingByName).
func pickIngredients(items []AdvItem, need map[string]int) ([]int64, map[string]int) {
	remaining := map[string]int{}
	for k, v := range need {
		remaining[k] = v
	}
	var consume []int64
	for _, it := range items {
		if remaining[it.Name] > 0 {
			consume = append(consume, it.ID)
			remaining[it.Name]--
		}
	}
	missing := map[string]int{}
	for name, left := range remaining {
		if left > 0 {
			missing[name] = left
		}
	}
	return consume, missing
}

// ── !lore ────────────────────────────────────────────────────────────────────

// handleLoreCmd rolls INT/Arcana DC 15. On success, reveals one
// undiscovered recipe at random.
func (p *AdventurePlugin) handleLoreCmd(ctx MessageContext) error {
	char, err := LoadDnDCharacter(ctx.Sender)
	if err != nil || char == nil || char.PendingSetup {
		return p.SendDM(ctx.Sender,
			"You need a D&D character to dig through Thom Krooke's lore stacks. Run `!setup`.")
	}

	known, err := loadKnownRecipes(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your recipe book.")
	}

	var undiscovered []ThomCraftRecipe
	for _, r := range thomCraftRecipes {
		if !known[r.ID] {
			undiscovered = append(undiscovered, r)
		}
	}
	if len(undiscovered) == 0 {
		return p.SendDM(ctx.Sender,
			"_Thom: \"You've already pulled every loose page out of these shelves. Nothing new to find.\"_")
	}

	res := performSkillCheck(char, SkillArcana, 15)
	var sb strings.Builder
	sb.WriteString("**Thom Krooke** waves at the stacks. \"Help yourself. Don't drop anything.\"\n\n")
	if res.Auto {
		sb.WriteString(fmt.Sprintf("_Arcana (auto-%s)_\n",
			ternary(res.Success, "success on a nat 20", "fail on a nat 1")))
	} else {
		sb.WriteString(fmt.Sprintf("_Arcana: rolled %d + %d = %d vs DC 15_\n",
			res.Roll, res.Mod, res.Total))
	}

	if !res.Success {
		sb.WriteString("\nThe pages don't connect for you today. Come back later.")
		return p.SendDM(ctx.Sender, sb.String())
	}

	pick := undiscovered[rand.IntN(len(undiscovered))]
	if err := recordKnownRecipe(ctx.Sender, pick.ID); err != nil {
		return p.SendDM(ctx.Sender, "Lore check passed but the page wouldn't stay copied. Tell an admin.")
	}
	sb.WriteString(fmt.Sprintf("\n📖 **New recipe discovered: %s**\n%s\n\n_Use `!craft list` to review your book._",
		pick.Name, pick.DiscoveryHint))
	return p.SendDM(ctx.Sender, sb.String())
}

// ── Test helper ──────────────────────────────────────────────────────────────

// resetKnownRecipesForTest wipes recipe discovery for a user. Test-only.
func resetKnownRecipesForTest(userID id.UserID) error {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM dnd_known_recipe WHERE user_id = ?`, string(userID))
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}
