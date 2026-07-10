package plugin

import (
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// TestTemperLadderWalk pins the rung order and the Legendary clamp. VeryRare
// must fold onto Epic — it shares Epic's tier and power scalar, so treating it
// as its own rung would sell a step that changes no number.
func TestTemperLadderWalk(t *testing.T) {
	tests := []struct {
		base  DnDRarity
		steps int
		want  DnDRarity
	}{
		{RarityCommon, 0, RarityCommon},
		{RarityCommon, 1, RarityUncommon},
		{RarityCommon, 4, RarityLegendary},
		{RarityCommon, 99, RarityLegendary}, // clamp, never past the ceiling
		{RarityRare, 1, RarityEpic},
		{RarityEpic, 1, RarityLegendary},
		{RarityLegendary, 1, RarityLegendary},
		{RarityVeryRare, 0, RarityVeryRare}, // untempered items keep their label
		{RarityVeryRare, 1, RarityLegendary},
		{RarityUncommon, -1, RarityUncommon}, // negative steps are inert
	}
	for _, tc := range tests {
		if got := temperedRarity(tc.base, tc.steps); got != tc.want {
			t.Errorf("temperedRarity(%s, %d) = %s, want %s", tc.base, tc.steps, got, tc.want)
		}
	}
}

func TestTemperStepsToLegendary(t *testing.T) {
	tests := []struct {
		base  DnDRarity
		steps int
		want  int
	}{
		{RarityCommon, 0, 4},
		{RarityCommon, 2, 2},
		{RarityCommon, 4, 0},
		{RarityCommon, 10, 0},
		{RarityVeryRare, 0, 1}, // folds onto Epic, one rung short
		{RarityLegendary, 0, 0},
	}
	for _, tc := range tests {
		if got := temperStepsToLegendary(tc.base, tc.steps); got != tc.want {
			t.Errorf("temperStepsToLegendary(%s, %d) = %d, want %d", tc.base, tc.steps, got, tc.want)
		}
	}
}

// TestTemperCostsCoverEveryRung guards against a ladder edit that leaves a rung
// priced at the zero value — a free temper.
func TestTemperCostsCoverEveryRung(t *testing.T) {
	for rung := 1; rung < len(temperLadder); rung++ {
		target := temperLadder[rung]
		cost, ok := temperCosts[rarityLootTierNum(target)]
		if !ok {
			t.Fatalf("no temper cost for target rarity %s", target)
		}
		if cost.Euros <= 0 || cost.MinForaging <= 0 {
			t.Errorf("temper to %s is free: %+v", target, cost)
		}
	}
	// Only the Legendary rung consumes a material.
	for tier, cost := range temperCosts {
		if want := tier == 5; cost.NeedsMaterial != want {
			t.Errorf("tier %d NeedsMaterial = %v, want %v", tier, cost.NeedsMaterial, want)
		}
	}
}

// TestTemperedItemScalesPowerAndValue asserts a tempered item is exactly an
// item of its effective rarity — the whole point of B1 is that rarity flows
// through the existing scalar formulas with no new combat math.
func TestTemperedItemScalesPowerAndValue(t *testing.T) {
	base := MagicItem{ID: "x", Name: "Thing", Kind: MagicItemWeapon, Rarity: RarityRare, Value: 300}

	if got := temperedItem(base, 0); got != base {
		t.Errorf("temper 0 mutated the item: %+v", got)
	}

	up := temperedItem(base, 1)
	if up.Rarity != RarityEpic {
		t.Fatalf("rarity = %s, want Epic", up.Rarity)
	}
	// Value tracks the power axis: Rare=3 → Epic=4.
	if wantVal := int(300 * (rarityPowerScalar(RarityEpic) / rarityPowerScalar(RarityRare))); up.Value != wantVal {
		t.Errorf("value = %d, want %d", up.Value, wantVal)
	}
	// A tempered Epic must fight exactly like a native Epic. (Its Value is
	// deliberately higher than a native Epic definition's — value tracks the
	// power axis, and the player paid to move it there.)
	native := base
	native.Rarity = RarityEpic
	if magicItemEffectFor(up) != magicItemEffectFor(native) {
		t.Errorf("tempered effect diverges from native Epic effect")
	}
	// Base must be untouched — temperedItem takes a copy.
	if base.Rarity != RarityRare || base.Value != 300 {
		t.Errorf("temperedItem mutated its argument: %+v", base)
	}
}

// TestTemperNeverDoubleBumps is the invariant the whole design hangs on: the
// stored row keeps BASE rarity plus a step count, so loading and re-saving an
// item any number of times must not compound the upgrade.
func TestTemperNeverDoubleBumps(t *testing.T) {
	base := MagicItem{ID: "x", Name: "Thing", Kind: MagicItemWeapon, Rarity: RarityCommon, Value: 100}
	e := EquippedMagicItem{Item: base, Temper: 2}

	first := e.Effective()
	// Simulate a load→save→load cycle: Item stays base, Temper stays put.
	again := EquippedMagicItem{Item: e.Item, Temper: e.Temper}.Effective()
	if first != again {
		t.Errorf("round-trip changed the item: %+v vs %+v", first, again)
	}
	if e.Item.Rarity != RarityCommon {
		t.Errorf("Effective() mutated the stored base rarity to %s", e.Item.Rarity)
	}
	if first.Rarity != RarityRare {
		t.Errorf("Common + 2 tempers = %s, want Rare", first.Rarity)
	}
}

// TestTemperSurvivesEquipRoundTrip: a tempered item that gets swapped back to
// inventory must carry its temper with it, priced at its effective rarity.
// This is the path resolveMagicEquipReply takes on a slot swap.
func TestTemperSurvivesEquipRoundTrip(t *testing.T) {
	base := MagicItem{ID: "x", Name: "Thing", Kind: MagicItemWeapon, Rarity: RarityCommon, Value: 100}

	back := magicItemSellAt(base, 3)
	if back.Temper != 3 {
		t.Errorf("swap-back lost temper: %d", back.Temper)
	}
	if back.Tier != rarityLootTierNum(RarityEpic) {
		t.Errorf("swap-back tier = %d, want Epic tier", back.Tier)
	}
	if back.Value <= int64(base.Value) {
		t.Errorf("swap-back value %d not scaled up from %d", back.Value, base.Value)
	}
	// An untempered sell must be byte-identical to the old behaviour.
	if magicItemSellAt(base, 0) != magicItemSell(base) {
		t.Errorf("magicItemSellAt(_, 0) diverges from magicItemSell")
	}
}

// TestTemperPersistence exercises the two DB writes: an equipped item's temper
// and a stowed item's temper both survive a reload, and loadEquippedMagicItems
// hands back a base-rarity Item with the step count beside it.
func TestTemperPersistence(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	user := id.UserID("@temper:test.invalid")

	var item MagicItem
	for _, mi := range magicItemRegistry {
		if mi.Slot != "" && mi.Rarity == RarityCommon {
			item = mi
			break
		}
	}
	if item.ID == "" {
		for _, mi := range magicItemRegistry {
			if mi.Slot != "" && temperStepsToLegendary(mi.Rarity, 0) > 0 {
				item = mi
				break
			}
		}
	}
	if item.ID == "" {
		t.Skip("no slotted sub-Legendary item in registry")
	}

	if err := equipMagicItem(user, item.Slot, item.ID, false, 0); err != nil {
		t.Fatal(err)
	}
	if err := temperEquippedItem(user, item.Slot, 1); err != nil {
		t.Fatal(err)
	}
	equipped, err := loadEquippedMagicItems(user)
	if err != nil {
		t.Fatal(err)
	}
	e := equipped[item.Slot]
	if e.Temper != 1 {
		t.Fatalf("equipped temper = %d, want 1", e.Temper)
	}
	if e.Item.Rarity != item.Rarity {
		t.Errorf("stored Item.Rarity = %s, want the untouched base %s", e.Item.Rarity, item.Rarity)
	}
	if want := temperedRarity(item.Rarity, 1); e.Effective().Rarity != want {
		t.Errorf("Effective().Rarity = %s, want %s", e.Effective().Rarity, want)
	}

	// Stowed side: a fresh inventory row defaults to temper 0, then bumps.
	inv := AdvItem{Name: item.Name, Type: "magic_item", Tier: 1, Value: 100,
		SkillSource: "magic_item:" + item.ID}
	if err := addAdvInventoryItem(user, inv); err != nil {
		t.Fatal(err)
	}
	items, err := loadAdvInventory(user)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Temper != 0 {
		t.Fatalf("fresh inventory row should default to temper 0, got %+v", items)
	}
	bumped := temperedItem(item, 2)
	if err := temperInventoryItem(items[0].ID, 2, rarityLootTierNum(bumped.Rarity), int64(bumped.Value)); err != nil {
		t.Fatal(err)
	}
	items, _ = loadAdvInventory(user)
	if items[0].Temper != 2 {
		t.Errorf("inventory temper = %d, want 2", items[0].Temper)
	}
	if items[0].Tier != rarityLootTierNum(bumped.Rarity) {
		t.Errorf("inventory tier = %d, want %d", items[0].Tier, rarityLootTierNum(bumped.Rarity))
	}
}

// TestCollectTemperTargetsFiltering: Legendary items and consumable-kind magic
// items (potions/scrolls, whose combat effect is zero) are not offerable.
func TestCollectTemperTargetsFiltering(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	user := id.UserID("@filter:test.invalid")

	var legendary, potion, upgradable MagicItem
	for _, mi := range magicItemRegistry {
		if legendary.ID == "" && mi.Rarity == RarityLegendary && mi.Slot != "" {
			legendary = mi
		}
		if potion.ID == "" && (mi.Kind == MagicItemPotion || mi.Kind == MagicItemScroll) {
			potion = mi
		}
		if upgradable.ID == "" && mi.Slot != "" && mi.Kind == MagicItemWeapon &&
			temperStepsToLegendary(mi.Rarity, 0) > 0 {
			upgradable = mi
		}
	}

	add := func(mi MagicItem) {
		if mi.ID == "" {
			return
		}
		if err := addAdvInventoryItem(user, AdvItem{
			Name: mi.Name, Type: "magic_item", Tier: rarityLootTierNum(mi.Rarity),
			Value: int64(mi.Value), SkillSource: "magic_item:" + mi.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add(legendary)
	add(potion)
	add(upgradable)

	targets, err := collectTemperTargets(user)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range targets {
		if legendary.ID != "" && tg.Base.ID == legendary.ID {
			t.Errorf("Legendary %s offered for tempering", legendary.Name)
		}
		if potion.ID != "" && tg.Base.ID == potion.ID {
			t.Errorf("consumable %s offered for tempering", potion.Name)
		}
	}
	if upgradable.ID != "" {
		found := false
		for _, tg := range targets {
			if tg.Base.ID == upgradable.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("upgradable %s (%s) was not offered", upgradable.Name, upgradable.Rarity)
		}
	}
}

// TestTemperMaterialsAreObtainable guards the economy: every material name the
// final rung demands must actually drop from some zone's loot slate. If a slate
// is renamed, tempering to Legendary silently becomes impossible.
func TestTemperMaterialsAreObtainable(t *testing.T) {
	live := map[string]bool{}
	for _, zone := range allZones() {
		for _, entry := range zone.Loot {
			live[titleCaseUnderscored(entry.ItemID)] = true
		}
	}
	for _, name := range temperMaterialNames {
		if !live[name] {
			t.Errorf("temper material %q drops from no zone loot slate", name)
		}
	}
}

// TestFindTemperMaterial picks the first material regardless of which one.
func TestFindTemperMaterial(t *testing.T) {
	if _, ok := findTemperMaterial(nil); ok {
		t.Error("found a material in an empty inventory")
	}
	if _, ok := findTemperMaterial([]AdvItem{{Name: "Iron Ore"}}); ok {
		t.Error("Iron Ore accepted as a legendary material")
	}
	for _, name := range temperMaterialNames {
		got, ok := findTemperMaterial([]AdvItem{{Name: "Iron Ore"}, {ID: 7, Name: name}})
		if !ok || got.ID != 7 {
			t.Errorf("did not find material %q", name)
		}
	}
}

func TestParseTemperIndex(t *testing.T) {
	tests := []struct {
		in     string
		n      int
		want   int
		wantOK bool
	}{
		{"1", 3, 0, true},
		{"3", 3, 2, true},
		{" 2 ", 3, 1, true},
		{"2. Thing", 3, 1, true},
		{"0", 3, 0, false},
		{"4", 3, 0, false},
		{"", 3, 0, false},
		{"cancel", 3, 0, false},
	}
	for _, tc := range tests {
		got, ok := parseMenuIndex(tc.in, tc.n)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("parseMenuIndex(%q, %d) = (%d, %v), want (%d, %v)",
				tc.in, tc.n, got, ok, tc.want, tc.wantOK)
		}
	}
}
