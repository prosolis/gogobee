package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func vaultTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func seedVaultItem(t *testing.T, user id.UserID, name string, value int64) {
	t.Helper()
	if err := addAdvInventoryItem(user, AdvItem{Name: name, Type: "treasure", Tier: 3, Value: value}); err != nil {
		t.Fatal(err)
	}
}

// TestVaultStoreTakeRoundTrip: an item vaulted leaves the active inventory and
// joins the vault; taken back, it reverses exactly.
func TestVaultStoreTakeRoundTrip(t *testing.T) {
	vaultTestDB(t)
	user := id.UserID("@vault:test.invalid")
	seedVaultItem(t, user, "Jeweled Crown", 5000)

	if got := vaultStoreItem(user, "crown"); !strings.Contains(got, "Stowed") {
		t.Fatalf("store reply = %q, want a stow confirmation", got)
	}

	inv, _ := loadAdvInventory(user)
	if len(inv) != 0 {
		t.Fatalf("active inventory = %d items, want 0 (item is vaulted)", len(inv))
	}
	vault, _ := loadAdvVault(user)
	if len(vault) != 1 || vault[0].Name != "Jeweled Crown" {
		t.Fatalf("vault = %+v, want the crown", vault)
	}

	if got := vaultTakeItem(user, "crown"); !strings.Contains(got, "back into your pack") {
		t.Fatalf("take reply = %q, want a retrieval confirmation", got)
	}
	inv, _ = loadAdvInventory(user)
	if len(inv) != 1 {
		t.Fatalf("active inventory = %d items, want 1 after take", len(inv))
	}
	if n := advVaultCount(user); n != 0 {
		t.Fatalf("vault count = %d, want 0", n)
	}
}

// TestVaultCapacity: the 10th item stores, the 11th is refused, and the refusal
// leaves the item in the active inventory.
func TestVaultCapacity(t *testing.T) {
	vaultTestDB(t)
	user := id.UserID("@vaultcap:test.invalid")
	for i := 0; i < vaultCapacity; i++ {
		seedVaultItem(t, user, "Gem", 100)
		if got := vaultStoreItem(user, "Gem"); !strings.Contains(got, "Stowed") {
			t.Fatalf("store %d reply = %q, want success", i, got)
		}
	}
	if n := advVaultCount(user); n != vaultCapacity {
		t.Fatalf("vault count = %d, want %d", n, vaultCapacity)
	}

	seedVaultItem(t, user, "Overflow Gem", 100)
	got := vaultStoreItem(user, "Overflow Gem")
	if !strings.Contains(got, "full") {
		t.Fatalf("11th store reply = %q, want a full-vault refusal", got)
	}
	// The refused item is still in play.
	inv, _ := loadAdvInventory(user)
	if len(inv) != 1 || inv[0].Name != "Overflow Gem" {
		t.Fatalf("active inventory = %+v, want the overflow gem left in play", inv)
	}
}

// TestVaultShieldsFromSellAll: `!sell all` liquidates loadAdvInventory, which a
// vaulted item is no longer part of — the whole point of the vault.
func TestVaultShieldsFromSellAll(t *testing.T) {
	vaultTestDB(t)
	user := id.UserID("@vaultsell:test.invalid")
	seedVaultItem(t, user, "Heirloom", 9000)
	seedVaultItem(t, user, "Junk", 5)

	if got := vaultStoreItem(user, "Heirloom"); !strings.Contains(got, "Stowed") {
		t.Fatalf("store reply = %q", got)
	}
	// What sell-all would consume:
	sellable, _ := loadAdvInventory(user)
	if len(sellable) != 1 || sellable[0].Name != "Junk" {
		t.Fatalf("sellable inventory = %+v, want only Junk (heirloom sheltered)", sellable)
	}
}

// TestVaultGate: below Tier 4 the vault is locked; at Tier 4 it opens.
func TestVaultGate(t *testing.T) {
	locked := &AdventureCharacter{HouseTier: 3}
	if ok, msg := vaultUnlocked(locked); ok || msg == "" {
		t.Fatalf("tier 3: ok=%v msg=%q, want locked with a refusal", ok, msg)
	}
	open := &AdventureCharacter{HouseTier: 4}
	if ok, msg := vaultUnlocked(open); !ok || msg != "" {
		t.Fatalf("tier 4: ok=%v msg=%q, want unlocked", ok, msg)
	}
}

// TestSetItemVaultedIsOwnerScoped: another user's id cannot be flipped.
func TestSetItemVaultedIsOwnerScoped(t *testing.T) {
	vaultTestDB(t)
	owner := id.UserID("@owner:test.invalid")
	intruder := id.UserID("@intruder:test.invalid")
	seedVaultItem(t, owner, "Relic", 1000)
	items, _ := loadAdvInventory(owner)
	if len(items) != 1 {
		t.Fatalf("setup: owner inventory = %d, want 1", len(items))
	}
	moved, err := setItemVaulted(intruder, items[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Fatalf("intruder moved the owner's item; want no-op")
	}
	if advVaultCount(owner) != 0 {
		t.Fatalf("owner's item was vaulted by an intruder")
	}
}
