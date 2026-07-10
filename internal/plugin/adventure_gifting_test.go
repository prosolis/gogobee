package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestIsGiftableItem(t *testing.T) {
	cases := []struct {
		typ  string
		want bool
	}{
		{"consumable", true},
		{"MasterworkGear", true},
		{"ArenaGear", true},
		{"magic_item", false},
		{"treasure", false},
		{"ore", false},
		{"card", false},
	}
	for _, c := range cases {
		got, reason := isGiftableItem(AdvItem{Type: c.typ, Name: "X"})
		if got != c.want {
			t.Errorf("isGiftableItem(%q) = %v, want %v (reason %q)", c.typ, got, c.want, reason)
		}
		if !got && reason == "" {
			t.Errorf("isGiftableItem(%q) rejected with no reason", c.typ)
		}
	}
}

// TestPickGiftableItem: a non-giftable item must not shadow a giftable one that
// also matches the query.
func TestPickGiftableItem(t *testing.T) {
	inv := []AdvItem{
		{ID: 1, Name: "Dragon Scale", Type: "material", Tier: 5, Value: 9000},
		{ID: 2, Name: "Dragon Draught", Type: "consumable", Tier: 1, Value: 200},
	}
	// "dragon" matches the Scale first (higher tier), but the Draught is the
	// giftable one — pick it, don't refuse.
	got, reason := pickGiftableItem(inv, "dragon")
	if got == nil {
		t.Fatalf("pickGiftableItem returned nil (reason %q), want the giftable Draught", reason)
	}
	if got.Name != "Dragon Draught" {
		t.Errorf("picked %q, want Dragon Draught", got.Name)
	}

	// Only a non-giftable match → refuse with a reason.
	onlyMagic := []AdvItem{{ID: 3, Name: "Ring of Power", Type: "magic_item", Value: 5000}}
	got, reason = pickGiftableItem(onlyMagic, "ring")
	if got != nil || reason == "" {
		t.Errorf("magic-only match: got=%v reason=%q, want nil with a refusal reason", got, reason)
	}

	// No match at all → nil, empty reason.
	got, reason = pickGiftableItem(inv, "nonexistent")
	if got != nil || reason != "" {
		t.Errorf("no match: got=%v reason=%q, want nil/empty", got, reason)
	}
}

// TestTransferInventoryItem: an item moves owners exactly, and a wrong-owner
// transfer is a no-op.
func TestTransferInventoryItem(t *testing.T) {
	vaultTestDB(t)
	alice := id.UserID("@alice:test.invalid")
	bob := id.UserID("@bob:test.invalid")
	if err := addAdvInventoryItem(alice, AdvItem{Name: "Health Potion", Type: "consumable", Tier: 1, Value: 200}); err != nil {
		t.Fatal(err)
	}
	inv, _ := loadAdvInventory(alice)
	if len(inv) != 1 {
		t.Fatalf("setup: alice has %d items, want 1", len(inv))
	}
	itemID := inv[0].ID

	// Wrong owner can't move it.
	if moved, err := transferInventoryItem(itemID, bob, alice); err != nil || moved {
		t.Fatalf("wrong-owner transfer moved=%v err=%v, want no-op", moved, err)
	}

	// Real owner moves it to bob.
	moved, err := transferInventoryItem(itemID, alice, bob)
	if err != nil || !moved {
		t.Fatalf("transfer moved=%v err=%v, want success", moved, err)
	}
	if a, _ := loadAdvInventory(alice); len(a) != 0 {
		t.Errorf("alice still has %d items after gifting", len(a))
	}
	b, _ := loadAdvInventory(bob)
	if len(b) != 1 || b[0].Name != "Health Potion" {
		t.Errorf("bob's inventory = %+v, want the potion", b)
	}
}

// TestTransferUnvaultsItem: gifting a currently-vaulted item lands it in the
// recipient's active pack, not a phantom vault slot.
func TestTransferUnvaultsItem(t *testing.T) {
	vaultTestDB(t)
	alice := id.UserID("@alice2:test.invalid")
	bob := id.UserID("@bob2:test.invalid")
	if err := addAdvInventoryItem(alice, AdvItem{Name: "Elixir", Type: "consumable", Tier: 1, Value: 100}); err != nil {
		t.Fatal(err)
	}
	inv, _ := loadAdvInventory(alice)
	if _, err := setItemVaulted(alice, inv[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := transferInventoryItem(inv[0].ID, alice, bob); err != nil {
		t.Fatal(err)
	}
	// Bob sees it in active inventory (not vault).
	if b, _ := loadAdvInventory(bob); len(b) != 1 {
		t.Errorf("bob active inventory = %d, want 1 (item un-vaulted on gift)", len(b))
	}
	if bv, _ := loadAdvVault(bob); len(bv) != 0 {
		t.Errorf("bob vault = %d, want 0", len(bv))
	}
}

// TestGiftDailyCapCounting: logGift increments the day's count and the count is
// day-scoped.
func TestGiftDailyCapCounting(t *testing.T) {
	vaultTestDB(t)
	sender := id.UserID("@giver:test.invalid")
	recip := id.UserID("@taker:test.invalid")

	if n := giftCountToday(sender, "2026-07-10"); n != 0 {
		t.Fatalf("initial count = %d, want 0", n)
	}
	for i := 0; i < giftDailyCap; i++ {
		if err := logGift(sender, recip, "Potion", 100, "2026-07-10"); err != nil {
			t.Fatal(err)
		}
	}
	if n := giftCountToday(sender, "2026-07-10"); n != giftDailyCap {
		t.Errorf("count = %d, want %d", n, giftDailyCap)
	}
	// A different day is a fresh allowance.
	if n := giftCountToday(sender, "2026-07-11"); n != 0 {
		t.Errorf("next-day count = %d, want 0", n)
	}
}
