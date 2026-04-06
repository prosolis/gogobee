package plugin

import (
	"strings"
	"testing"
)

// ── Display Tests ───────────────────────────────────────────────────────────

func TestLuigiShopGreeting_Basic(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100},
	}
	text := luigiShopGreeting("@user:test", equip, 5000, false)

	if !strings.Contains(text, "Luigi's") {
		t.Error("greeting should contain Luigi's")
	}
	if !strings.Contains(text, "€5000") {
		t.Error("greeting should show balance")
	}
	if !strings.Contains(text, "Weapons") {
		t.Error("greeting should show category grid")
	}
	if !strings.Contains(text, "Reply with a category") {
		t.Error("greeting should prompt for category")
	}
}

func TestLuigiShopGreeting_MaxedOut(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5, Condition: 100},
		SlotArmor:  {Tier: 5, Condition: 100},
		SlotHelmet: {Tier: 5, Condition: 100},
		SlotBoots:  {Tier: 5, Condition: 100},
		SlotTool:   {Tier: 5, Condition: 100},
	}
	text := luigiShopGreeting("@user:test", equip, 99999, false)

	// Should NOT show category grid.
	if strings.Contains(text, "Reply with a category") {
		t.Error("maxed-out greeting should not show category prompt")
	}
	// Should use maxed-out flavor.
	if !strings.Contains(text, "fully kitted") {
		t.Error("maxed-out greeting should mention being fully kitted")
	}
}

func TestLuigiShopGreeting_ShowAll(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{}
	text := luigiShopGreeting("@user:test", equip, 1000, true)

	// Should contain a show-all comment.
	if !strings.Contains(text, "judgment") && !strings.Contains(text, "thoroughness") {
		t.Error("show all greeting should contain a show-all comment")
	}
}

func TestLuigiCategoryView_UpgradeIndicators(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 2, Condition: 100, Name: "Dull Steel Sword of Mediocrity"},
	}
	text := luigiCategoryView("@user:test", SlotWeapon, equip, 50000, false)

	// Should show current equipped.
	if !strings.Contains(text, "🟢") {
		t.Error("should show 🟢 for currently equipped item")
	}
	// Should show upgrade indicators.
	if !strings.Contains(text, "⬆️") {
		t.Error("should show ⬆️ for items above current tier")
	}
	// Should NOT show T1 (below current tier) by default.
	if strings.Contains(text, "Sad Iron Sword") {
		t.Error("should not show items below current tier when showAll=false")
	}
}

func TestLuigiCategoryView_ShowAll(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 3, Condition: 100, Name: "Sword (It's Fine)"},
	}
	text := luigiCategoryView("@user:test", SlotWeapon, equip, 50000, true)

	// Should show items below current tier.
	if !strings.Contains(text, "Sad Iron Sword") {
		t.Error("show all should include items below current tier")
	}
	// Below-tier items should show ⬇️.
	if !strings.Contains(text, "⬇️") {
		t.Error("show all should show ⬇️ for items below current tier")
	}
}

func TestLuigiCategoryView_Affordability(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 0, Condition: 100, Name: "Basic Ass Sword"},
	}
	// Can afford T1 (€100) but not T2 (€450).
	text := luigiCategoryView("@user:test", SlotWeapon, equip, 200, false)

	if !strings.Contains(text, "✅") {
		t.Error("should show ✅ for affordable items")
	}
	if !strings.Contains(text, "💸") {
		t.Error("should show 💸 for unaffordable items")
	}
}

func TestLuigiCategoryView_MasterworkAck(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 3, Condition: 100, Name: "MW Blade", Masterwork: true},
	}
	text := luigiCategoryView("@user:test", SlotWeapon, equip, 50000, false)

	if !strings.Contains(text, "⭐") {
		t.Error("should show ⭐ for masterwork gear")
	}
	// The masterwork acknowledgement pool has varied text — just check it's referenced.
	hasMWAck := strings.Contains(text, "Masterwork") || strings.Contains(text, "Arena gear") || strings.Contains(text, "can't beat that")
	if !hasMWAck {
		t.Error("should acknowledge masterwork/arena gear")
	}
}

func TestLuigiCategoryView_MaxedSlot(t *testing.T) {
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 5, Condition: 100, Name: "Vorpal Sword"},
	}
	text := luigiCategoryView("@user:test", SlotWeapon, equip, 50000, false)

	if !strings.Contains(text, "max tier") {
		t.Error("should show max-tier message when nothing to buy")
	}
}

// ── Item Confirm Tests ──────────────────────────────────────────────���───────

func TestLuigiItemConfirm_Upgrade(t *testing.T) {
	def := &EquipmentDef{Name: "Silver Blade", Tier: 3, Price: 1500}
	current := &AdvEquipment{Tier: 2, Name: "Dull Steel Sword"}
	text := luigiItemConfirm("@user:test", SlotWeapon, def, current, 5000)

	if !strings.Contains(text, "⬆️") {
		t.Error("should show upgrade indicator")
	}
	if !strings.Contains(text, "upgrade from") {
		t.Error("should mention it's an upgrade")
	}
	if !strings.Contains(text, "€5000") {
		t.Error("should show current balance")
	}
	if !strings.Contains(text, "€3500") {
		t.Error("should show balance after purchase")
	}
	if !strings.Contains(text, "Confirm?") {
		t.Error("should prompt for confirmation")
	}
}

func TestLuigiItemConfirm_NoCurrent(t *testing.T) {
	def := &EquipmentDef{Name: "Sad Iron Sword", Tier: 1, Price: 100}
	text := luigiItemConfirm("@user:test", SlotWeapon, def, nil, 1000)

	if !strings.Contains(text, "upgrade from") {
		t.Error("should say upgrade from None")
	}
}

// ── Item Lookup Tests ──────────────────────────────────────────────��────────

func TestAdvFindShopItemInSlot_ExactMatch(t *testing.T) {
	slot, def, ok := advFindShopItemInSlot("Enchanted Plate", SlotArmor)
	if !ok {
		t.Fatal("should find Enchanted Plate in armor slot")
	}
	if slot != SlotArmor {
		t.Errorf("slot should be armor, got %s", slot)
	}
	if def.Tier != 4 {
		t.Errorf("Enchanted Plate should be tier 4, got %d", def.Tier)
	}
}

func TestAdvFindShopItemInSlot_WrongSlot(t *testing.T) {
	_, _, ok := advFindShopItemInSlot("Dragonscale", SlotWeapon)
	if ok {
		t.Error("should not find armor item in weapon slot")
	}
}

func TestAdvFindShopItemInSlot_PartialMatch(t *testing.T) {
	slot, def, ok := advFindShopItemInSlot("Enchanted Blade", SlotWeapon)
	if !ok {
		t.Fatal("should find Enchanted Blade by partial match")
	}
	if slot != SlotWeapon {
		t.Errorf("slot should be weapon, got %s", slot)
	}
	if def.Tier != 4 {
		t.Errorf("Enchanted Blade should be tier 4, got %d", def.Tier)
	}
}

func TestAdvFindShopItemInSlot_CaseInsensitive(t *testing.T) {
	_, _, ok := advFindShopItemInSlot("enchanted blade", SlotWeapon)
	if !ok {
		t.Error("should find item case-insensitively")
	}
}

func TestAdvFindShopItemInSlot_NoTier0(t *testing.T) {
	_, _, ok := advFindShopItemInSlot("Basic Ass Sword", SlotWeapon)
	if ok {
		t.Error("should not find tier 0 items (price 0)")
	}
}

func TestAdvFindShopItem_TierShorthand(t *testing.T) {
	slot, def, ok := advFindShopItem("3 sword")
	if !ok {
		t.Fatal("should find via tier shorthand")
	}
	if slot != SlotWeapon {
		t.Errorf("slot should be weapon, got %s", slot)
	}
	if def.Tier != 3 {
		t.Errorf("tier should be 3, got %d", def.Tier)
	}
}

func TestAdvFindShopItem_TierPrefix(t *testing.T) {
	_, def, ok := advFindShopItem("tier 5 boots")
	if !ok {
		t.Fatal("should find via 'tier 5 boots'")
	}
	if def.Tier != 5 {
		t.Errorf("tier should be 5, got %d", def.Tier)
	}
}

func TestAdvFindShopItem_TShorthand(t *testing.T) {
	_, def, ok := advFindShopItem("t4 armor")
	if !ok {
		t.Fatal("should find via 't4 armor'")
	}
	if def.Tier != 4 {
		t.Errorf("tier should be 4, got %d", def.Tier)
	}
}

// ── Flavor Coverage Tests ───────────────────────────────────────────────────

func TestLuigiItemDescriptions_Coverage(t *testing.T) {
	for _, slot := range allSlots {
		defs := equipmentTiers[slot]
		for _, def := range defs {
			if def.Price == 0 {
				continue // skip tier 0
			}
			key := luigiItemKey{slot, def.Tier}
			if _, ok := luigiItemDescriptions[key]; !ok {
				t.Errorf("missing Luigi item description for %s tier %d (%s)", slot, def.Tier, def.Name)
			}
			if _, ok := luigiItemOneLiners[key]; !ok {
				t.Errorf("missing Luigi one-liner for %s tier %d (%s)", slot, def.Tier, def.Name)
			}
		}
	}
}

func TestLuigiCategoryIntros_Coverage(t *testing.T) {
	for _, slot := range allSlots {
		intros, ok := luigiCategoryIntros[slot]
		if !ok || len(intros) == 0 {
			t.Errorf("missing Luigi category intros for slot %s", slot)
		}
	}
}

func TestLuigiFlavorPools_NonEmpty(t *testing.T) {
	pools := map[string][]string{
		"luigiGreetings":        luigiGreetings,
		"luigiPurchaseConfirm":  luigiPurchaseConfirm,
		"luigiTier5Confirm":     luigiTier5Confirm,
		"luigiComboConfirm":     luigiComboConfirm,
		"luigiInsufficientFunds": luigiInsufficientFunds,
		"luigiBrowseTimeout":    luigiBrowseTimeout,
		"luigiMaxedOut":         luigiMaxedOut,
		"luigiMasterworkAck":    luigiMasterworkAck,
		"luigiShowAllComment":   luigiShowAllComment,
		"luigiCommentary":       luigiCommentary,
		"luigiCancellation":     luigiCancellation,
	}
	for name, pool := range pools {
		if len(pool) == 0 {
			t.Errorf("flavor pool %s is empty", name)
		}
	}
}

// ── Format String Safety ────────────────────────────────────────────────────

func TestLuigiPurchaseConfirm_FormatStrings(t *testing.T) {
	for i, tmpl := range luigiPurchaseConfirm {
		// Should have %s then %d
		result := strings.Contains(tmpl, "%s") && strings.Contains(tmpl, "%d")
		if !result {
			t.Errorf("luigiPurchaseConfirm[%d] missing %%s or %%d placeholder", i)
		}
	}
}

func TestLuigiTier5Confirm_FormatStrings(t *testing.T) {
	for i, tmpl := range luigiTier5Confirm {
		result := strings.Contains(tmpl, "%s") && strings.Contains(tmpl, "%d")
		if !result {
			t.Errorf("luigiTier5Confirm[%d] missing %%s or %%d placeholder", i)
		}
	}
}

func TestLuigiComboConfirm_FormatStrings(t *testing.T) {
	for i, tmpl := range luigiComboConfirm {
		result := strings.Contains(tmpl, "%s") && strings.Contains(tmpl, "%d")
		if !result {
			t.Errorf("luigiComboConfirm[%d] missing %%s or %%d placeholder", i)
		}
	}
}

// ── Tier Bounds Safety ──────────────────────────────────────────────────────

func TestEquipmentTiers_ConsistentIndexing(t *testing.T) {
	for _, slot := range allSlots {
		defs := equipmentTiers[slot]
		for i, def := range defs {
			if def.Tier != i {
				t.Errorf("slot %s: tier %d at index %d (should match for safe direct indexing)", slot, def.Tier, i)
			}
		}
	}
}

func TestEquipmentTiers_SixTiersPerSlot(t *testing.T) {
	for _, slot := range allSlots {
		defs := equipmentTiers[slot]
		if len(defs) != 6 {
			t.Errorf("slot %s: expected 6 tiers (0-5), got %d", slot, len(defs))
		}
	}
}
