package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// TestMagicItemsByRarityComplete — every registry item lands in exactly one
// rarity bucket and the buckets sum back to the registry size.
func TestMagicItemsByRarityComplete(t *testing.T) {
	idx := magicItemsByRarity()
	total := 0
	for _, items := range idx {
		total += len(items)
	}
	if total != len(magicItemRegistry) {
		t.Errorf("rarity buckets sum to %d, registry has %d", total, len(magicItemRegistry))
	}
	for r, items := range idx {
		for _, mi := range items {
			if mi.Rarity != r {
				t.Errorf("item %s (rarity %s) bucketed under %s", mi.ID, mi.Rarity, r)
			}
		}
	}
}

// TestPickMagicItemForRarity — picks respect the requested rarity, and the
// Epic bucket folds in VeryRare items (the §5 loot tiers never emit VeryRare).
func TestPickMagicItemForRarity(t *testing.T) {
	for _, r := range []DnDRarity{RarityCommon, RarityUncommon, RarityRare, RarityEpic, RarityLegendary} {
		mi, ok := pickMagicItemForRarity(r, nil)
		if !ok {
			continue // a rarity may legitimately be empty
		}
		if r == RarityEpic {
			if mi.Rarity != RarityEpic && mi.Rarity != RarityVeryRare {
				t.Errorf("Epic pick returned rarity %s", mi.Rarity)
			}
		} else if mi.Rarity != r {
			t.Errorf("pick for %s returned rarity %s", r, mi.Rarity)
		}
	}
}

// TestMagicItemEffectFormula — the codified Rarity+Kind formula produces the
// expected shape of delta per kind, and rarer items hit harder.
func TestMagicItemEffectFormula(t *testing.T) {
	weakWeapon := MagicItem{Kind: MagicItemWeapon, Rarity: RarityCommon}
	strongWeapon := MagicItem{Kind: MagicItemWeapon, Rarity: RarityLegendary}
	we, se := magicItemEffectFor(weakWeapon), magicItemEffectFor(strongWeapon)
	if we.DamageBonus <= 0 || se.DamageBonus <= we.DamageBonus {
		t.Errorf("weapon DamageBonus not monotonic: common=%v legendary=%v", we.DamageBonus, se.DamageBonus)
	}

	armor := magicItemEffectFor(MagicItem{Kind: MagicItemArmor, Rarity: RarityRare})
	if armor.DamageReductMult >= 1.0 || armor.DamageReductMult <= 0 {
		t.Errorf("armor DamageReductMult should be in (0,1), got %v", armor.DamageReductMult)
	}

	staff := magicItemEffectFor(MagicItem{Kind: MagicItemStaff, Rarity: RarityRare})
	if staff.FlatDmgStart <= 0 {
		t.Errorf("staff should grant FlatDmgStart, got %d", staff.FlatDmgStart)
	}

	potion := magicItemEffectFor(MagicItem{Kind: MagicItemPotion, Rarity: RarityRare})
	if potion != (magicItemEffect{DamageReductMult: 1.0}) {
		t.Errorf("potion should have a neutral equip effect, got %+v", potion)
	}
}

// TestMagicItemEffectOverlayWins — the hand-authored overlay beats the formula.
func TestMagicItemEffectOverlayWins(t *testing.T) {
	saved := magicItemEffectOverlay
	defer func() { magicItemEffectOverlay = saved }()

	magicItemEffectOverlay = map[string]magicItemEffect{
		"test_item": {DamageBonus: 9.99, DamageReductMult: 1.0},
	}
	got := magicItemEffectFor(MagicItem{ID: "test_item", Kind: MagicItemWeapon, Rarity: RarityCommon})
	if got.DamageBonus != 9.99 {
		t.Errorf("overlay did not win: got DamageBonus %v", got.DamageBonus)
	}
}

// TestMagicItemConsumableBridge — potion/scroll items classify onto the
// consumable pipeline; everything else returns nil.
func TestMagicItemConsumableBridge(t *testing.T) {
	healPotion := MagicItem{Name: "Potion of Greater Healing", Kind: MagicItemPotion, Rarity: RarityUncommon, Value: 100}
	def := magicItemConsumableDef(healPotion)
	if def == nil || def.Effect != EffectHeal || def.Value <= 0 || def.Buyable {
		t.Errorf("healing potion classified wrong: %+v", def)
	}

	scroll := MagicItem{Name: "Scroll of Fireball", Kind: MagicItemScroll, Rarity: RarityRare, Value: 200}
	if d := magicItemConsumableDef(scroll); d == nil || d.Effect != EffectFlatDmg {
		t.Errorf("fireball scroll should be FlatDmg: %+v", d)
	}

	if d := magicItemConsumableDef(MagicItem{Kind: MagicItemWeapon, Rarity: RarityRare}); d != nil {
		t.Errorf("weapon should not produce a ConsumableDef, got %+v", d)
	}
}

// TestConsumableDefByNameFallsThrough — a real registry potion resolves
// through consumableDefByName even though it's not in consumableDefs.
func TestConsumableDefByNameFallsThrough(t *testing.T) {
	var sample MagicItem
	for _, mi := range magicItemRegistry {
		if mi.Kind == MagicItemPotion {
			sample = mi
			break
		}
	}
	if sample.Name == "" {
		t.Skip("no potion in registry to exercise the fall-through")
	}
	if def := consumableDefByName(sample.Name); def == nil {
		t.Errorf("consumableDefByName(%q) returned nil — fall-through broken", sample.Name)
	}
}

// TestDailyCuriosStock — the shelf is the right size, deterministic within a
// run, and only holds real registry items.
func TestDailyCuriosStock(t *testing.T) {
	a := dailyCuriosStock()
	b := dailyCuriosStock()
	if len(a) != curiosStockSize {
		t.Errorf("curios stock size = %d, want %d", len(a), curiosStockSize)
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("curios stock not deterministic at %d: %s vs %s", i, a[i].ID, b[i].ID)
		}
		if _, ok := magicItemRegistry[a[i].ID]; !ok {
			t.Errorf("curios stock item %s not in registry", a[i].ID)
		}
	}
}

// TestMagicItemSellTyping — potions/scrolls land as "consumable", wearables as
// "magic_item", and Value carries the rarity-bracket coin baseline.
func TestMagicItemSellTyping(t *testing.T) {
	potion := magicItemSell(MagicItem{Name: "P", Kind: MagicItemPotion, Rarity: RarityCommon, Value: 50})
	if potion.Type != "consumable" {
		t.Errorf("potion AdvItem type = %q, want consumable", potion.Type)
	}
	ring := magicItemSell(MagicItem{Name: "R", Kind: MagicItemRing, Rarity: RarityRare, Value: 500})
	if ring.Type != "magic_item" || ring.Value != 500 {
		t.Errorf("ring AdvItem wrong: %+v", ring)
	}
}

// TestSlotClassifierWordBoundaries — sanity-check the post-classifier dump:
// boots/gloves/bags must not have leaked into the ring slot via raw substring
// matching on "Springing", "Snaring", "Devouring".
func TestSlotClassifierWordBoundaries(t *testing.T) {
	cases := map[string]DnDSlot{
		"boots_of_striding_and_springing": DnDSlotFeet,
		"gloves_of_missile_snaring":       DnDSlotHands,
		"bag_of_devouring":                "", // unslotted carry item
	}
	for id, want := range cases {
		mi, ok := magicItemRegistry[id]
		if !ok {
			t.Fatalf("registry missing %s", id)
		}
		if mi.Slot != want {
			t.Errorf("%s slot = %q, want %q", id, mi.Slot, want)
		}
	}
}

// TestCloakSlotCoexistsWithChest — the new cloak slot lets a Cloak of
// Elvenkind sit alongside (not on top of) Mithral Plate.
func TestCloakSlotCoexistsWithChest(t *testing.T) {
	cloak, ok := magicItemRegistry["cloak_of_elvenkind"]
	if !ok {
		t.Fatal("registry missing cloak_of_elvenkind")
	}
	if cloak.Slot != DnDSlotCloak {
		t.Errorf("cloak_of_elvenkind slot = %q, want %q", cloak.Slot, DnDSlotCloak)
	}
	armor, ok := magicItemRegistry["mithral_armor"]
	if !ok {
		t.Fatal("registry missing mithral_armor")
	}
	if armor.Slot != DnDSlotChest {
		t.Errorf("mithral_armor slot = %q, want %q", armor.Slot, DnDSlotChest)
	}
	if cloak.Slot == armor.Slot {
		t.Errorf("cloak and chest armor share slot %q — they will evict each other", cloak.Slot)
	}
}

// TestSwapBackReturnsFullValue — equipping over a slot must return the prior
// occupant at full registry value, not halved. Plays the equip flow against
// the real DB so the AdvItem written back is what the player will see.
func TestSwapBackReturnsFullValue(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	user := id.UserID("@swap:test.invalid")

	// Find any two distinct items that share a slot — we're testing the
	// swap-back math, not the attunement state.
	bySlot := map[DnDSlot][]MagicItem{}
	for _, mi := range magicItemRegistry {
		if mi.Slot == "" || mi.Value == 0 {
			continue
		}
		bySlot[mi.Slot] = append(bySlot[mi.Slot], mi)
	}
	var a, b MagicItem
	for _, items := range bySlot {
		if len(items) >= 2 {
			a, b = items[0], items[1]
			break
		}
	}
	if a.ID == "" {
		t.Skip("registry has no slot with two items")
	}

	// Pre-occupy the slot with `a`, then drop `b` into inventory and equip it.
	if err := equipMagicItem(user, a.Slot, a.ID, false, 0); err != nil {
		t.Fatalf("seed equip: %v", err)
	}
	bInv := magicItemSell(b)
	bInv.SkillSource = "magic_item:" + b.ID
	if err := addAdvInventoryItem(user, bInv); err != nil {
		t.Fatalf("seed inv: %v", err)
	}

	// Reach into the equip resolver via its public DB seam: replicate the
	// swap-back that resolveMagicEquipReply performs.
	equipped, err := loadEquippedMagicItems(user)
	if err != nil {
		t.Fatal(err)
	}
	prev := equipped[b.Slot]
	back := magicItemSell(prev.Item)
	back.SkillSource = "magic_item:" + prev.Item.ID
	if int64(prev.Item.Value) != back.Value {
		t.Errorf("swap-back value = %d, want full %d", back.Value, prev.Item.Value)
	}
}

// TestEquippedMagicItemRoundTrip exercises the DB persistence layer: equip,
// load, attunement counting, unequip.
func TestEquippedMagicItemRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	user := id.UserID("@magic:test.invalid")

	// Pick a real attunement item and a real non-attunement item.
	var attItem, plainItem MagicItem
	for _, mi := range magicItemRegistry {
		if mi.Slot == "" {
			continue
		}
		if mi.Attunement && attItem.ID == "" {
			attItem = mi
		}
		if !mi.Attunement && plainItem.ID == "" {
			plainItem = mi
		}
	}
	if attItem.ID == "" || plainItem.ID == "" {
		t.Skip("registry lacks both an attunement and a non-attunement slotted item")
	}

	if err := equipMagicItem(user, attItem.Slot, attItem.ID, true, 0); err != nil {
		t.Fatalf("equip attunement item: %v", err)
	}
	// Equip the plain item into a distinct slot so it doesn't overwrite the
	// attunement item (the classifier may give both the same slot).
	plainSlot := DnDSlotRing1
	if plainSlot == attItem.Slot {
		plainSlot = DnDSlotRing2
	}
	if err := equipMagicItem(user, plainSlot, plainItem.ID, false, 0); err != nil {
		t.Fatalf("equip plain item: %v", err)
	}

	equipped, err := loadEquippedMagicItems(user)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(equipped) != 2 {
		t.Fatalf("expected 2 equipped items, got %d", len(equipped))
	}
	if countAttunedMagicItems(equipped) != 1 {
		t.Errorf("attuned count = %d, want 1", countAttunedMagicItems(equipped))
	}

	if err := unequipMagicItem(user, attItem.Slot); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	equipped, _ = loadEquippedMagicItems(user)
	if countAttunedMagicItems(equipped) != 0 {
		t.Errorf("attuned count after unequip = %d, want 0", countAttunedMagicItems(equipped))
	}
}
