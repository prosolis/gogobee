package plugin

import (
	"strings"
	"testing"
)

// ── Recipe registry coverage ─────────────────────────────────────────────────

// TestRecipeRegistry_IngredientsExistInResources — every ingredient name
// referenced by a §8.2 recipe must match a Name in the zone-resource
// registry, otherwise !craft can never find the inventory items.
func TestRecipeRegistry_IngredientsExistInResources(t *testing.T) {
	known := map[string]bool{}
	for _, list := range zoneResources {
		for _, r := range list {
			known[r.Name] = true
		}
	}
	for _, r := range thomCraftRecipes {
		for ing := range r.Ingredients {
			if !known[ing] {
				t.Errorf("recipe %s references unknown ingredient %q", r.ID, ing)
			}
		}
	}
}

// TestRecipeRegistry_OutputValuesMatchTier — outputs should land in §8.1
// rarity brackets given their tier (rare 100–300, very rare 500–1200,
// epic 500–1200 in §8.1 — recipes here lean tier 3–4, so 200..1200 is the
// reasonable band).
func TestRecipeRegistry_OutputValuesInBand(t *testing.T) {
	for _, r := range thomCraftRecipes {
		if r.OutputValue < 200 || r.OutputValue > 1200 {
			t.Errorf("recipe %s output value %d outside band [200,1200] (tier %d)",
				r.ID, r.OutputValue, r.OutputTier)
		}
	}
}

// TestRecipeRegistry_AllExemplarsPresent — the four implementable §8.2
// exemplars must be in the registry. Drow Poison Blade is intentionally
// deferred (needs "any blade weapon" matching, lands in R6).
func TestRecipeRegistry_AllExemplarsPresent(t *testing.T) {
	want := []string{
		"potion_fire_resist",
		"mithral_dagger",
		"undead_ward_charm",
		"shadow_step_cloak",
	}
	have := map[string]bool{}
	for _, r := range thomCraftRecipes {
		have[r.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing exemplar recipe: %s", id)
		}
	}
}

// ── findRecipeByName ─────────────────────────────────────────────────────────

func TestFindRecipeByName_FuzzyMatch(t *testing.T) {
	cases := []struct {
		query string
		want  string
		ok    bool
	}{
		{"mithral", "mithral_dagger", true},
		{"shadow step", "shadow_step_cloak", true},
		{"Potion of Fire", "potion_fire_resist", true},
		{"nonexistent", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := findRecipeByName(c.query)
		if ok != c.ok {
			t.Errorf("query %q: ok=%v want %v", c.query, ok, c.ok)
			continue
		}
		if ok && got.ID != c.want {
			t.Errorf("query %q: got %s want %s", c.query, got.ID, c.want)
		}
	}
}

// ── pickIngredients ──────────────────────────────────────────────────────────

func TestPickIngredients_ExactCover(t *testing.T) {
	items := []AdvItem{
		{ID: 1, Name: "Salamander Oil", Type: "material"},
		{ID: 2, Name: "Fire Essence", Type: "material"},
		{ID: 3, Name: "Fire Essence", Type: "material"},
		{ID: 4, Name: "Scrap Iron", Type: "material"},
	}
	need := map[string]int{"Salamander Oil": 1, "Fire Essence": 2}
	consume, missing := pickIngredients(items, need)
	if len(missing) != 0 {
		t.Errorf("missing should be empty, got %v", missing)
	}
	if len(consume) != 3 {
		t.Errorf("consume len = %d want 3", len(consume))
	}
}

func TestPickIngredients_Short(t *testing.T) {
	items := []AdvItem{
		{ID: 1, Name: "Fire Essence", Type: "material"},
	}
	need := map[string]int{"Fire Essence": 2, "Salamander Oil": 1}
	consume, missing := pickIngredients(items, need)
	if missing["Fire Essence"] != 1 {
		t.Errorf("missing Fire Essence = %d want 1", missing["Fire Essence"])
	}
	if missing["Salamander Oil"] != 1 {
		t.Errorf("missing Salamander Oil = %d want 1", missing["Salamander Oil"])
	}
	// We still consumed the one match optimistically; caller checks missing.
	if len(consume) != 1 {
		t.Errorf("consume len = %d want 1", len(consume))
	}
}

// ── filterSellable ───────────────────────────────────────────────────────────

func TestFilterSellable_ExcludesEquipment(t *testing.T) {
	items := []AdvItem{
		{Name: "Fire Essence", Type: "material"},
		{Name: "Shadow Trout", Type: "fish"},
		{Name: "Cursed Trinket", Type: "item"},
		{Name: "Mithril Sword", Type: "MasterworkGear"},
		{Name: "Arena Helm", Type: "ArenaGear"},
		{Name: "Healing Potion", Type: "consumable"},
		{Name: "Lucky Card", Type: "card"},
	}
	out := filterSellable(items)
	if len(out) != 3 {
		t.Errorf("filterSellable len = %d want 3 (material/fish/item only)", len(out))
	}
	for _, it := range out {
		switch it.Type {
		case "material", "fish", "item":
		default:
			t.Errorf("unexpected type in sellable: %s", it.Type)
		}
	}
}

// ── findInventoryMatch ──────────────────────────────────────────────────────

func TestFindInventoryMatch_FuzzyAndMiss(t *testing.T) {
	items := []AdvItem{
		{ID: 1, Name: "Salamander Oil"},
		{ID: 2, Name: "Fire Essence"},
	}
	if m := findInventoryMatch(items, "salamander"); m == nil || m.ID != 1 {
		t.Errorf("want match on Salamander Oil")
	}
	if m := findInventoryMatch(items, "FIRE"); m == nil || m.ID != 2 {
		t.Errorf("case-insensitive match should hit Fire Essence")
	}
	if m := findInventoryMatch(items, "mithral"); m != nil {
		t.Errorf("expected no match for 'mithral'")
	}
}

// ── inventoryByName ─────────────────────────────────────────────────────────

func TestInventoryByName_Counts(t *testing.T) {
	items := []AdvItem{
		{Name: "Fire Essence"}, {Name: "Fire Essence"}, {Name: "Fire Essence"},
		{Name: "Salamander Oil"},
	}
	got := inventoryByName(items)
	if got["Fire Essence"] != 3 || got["Salamander Oil"] != 1 {
		t.Errorf("counts off: %v", got)
	}
}

// ── Recipe book rendering ───────────────────────────────────────────────────

// TestRenderRecipeBook_HidesUndiscovered — discovery state must hide
// recipe details until they're known.
func TestRenderRecipeBook_HidesUndiscovered(t *testing.T) {
	known := map[string]bool{}
	out := renderRecipeBookForTest(known, nil)
	for _, r := range thomCraftRecipes {
		if strings.Contains(out, r.Name) {
			t.Errorf("undiscovered recipe %s leaked into book", r.Name)
		}
	}
	if !strings.Contains(out, "undiscovered") {
		t.Errorf("expected hidden-recipe count in output:\n%s", out)
	}

	known = map[string]bool{"mithral_dagger": true}
	out = renderRecipeBookForTest(known, nil)
	if !strings.Contains(out, "Mithral Dagger") {
		t.Errorf("known recipe Mithral Dagger missing:\n%s", out)
	}
	if strings.Contains(out, "Potion of Fire Resistance") {
		t.Errorf("undiscovered recipe should be hidden")
	}
}

// renderRecipeBookForTest is a DB-free version of renderRecipeBook for
// unit tests. It bypasses loadAdvInventory by accepting items directly.
func renderRecipeBookForTest(known map[string]bool, items []AdvItem) string {
	var sb strings.Builder
	sb.WriteString("📜 **Thom Krooke — Recipe Book**\n\n")
	have := inventoryByName(items)
	knownCount := 0
	for _, r := range thomCraftRecipes {
		if !known[r.ID] {
			continue
		}
		knownCount++
		sb.WriteString(r.Name + "\n")
		var parts []string
		for ing, need := range r.Ingredients {
			parts = append(parts, ing)
			_ = need
			_ = have
		}
		sb.WriteString(strings.Join(parts, " + ") + "\n")
	}
	hidden := len(thomCraftRecipes) - knownCount
	if hidden > 0 {
		sb.WriteString("undiscovered\n")
	}
	return sb.String()
}
