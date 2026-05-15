package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// freshShopTestDB spins up an empty DB in a tempdir for the sell-path tests.
// Distinct from setupAuditTestDB, which clones the prod snapshot.
func freshShopTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// TestSellAll_PreservesMagicItems — B1 (data-loss blocker): `sell all` must
// route around curios (Type=="magic_item") so a careless liquidation can't
// silently delete a Legendary Ring. Junk in the same bag still sells.
func TestSellAll_PreservesMagicItems(t *testing.T) {
	freshShopTestDB(t)
	uid := id.UserID("@sellall_magic:test")

	curio := AdvItem{Name: "Ring of Protection", Type: "magic_item", Tier: 3, Value: 500}
	junk := AdvItem{Name: "Iron Ore", Type: "material", Tier: 1, Value: 40}
	if err := addAdvInventoryItem(uid, curio); err != nil {
		t.Fatal(err)
	}
	if err := addAdvInventoryItem(uid, junk); err != nil {
		t.Fatal(err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	p := &AdventurePlugin{euro: euro}

	msg := p.advSellAll(uid)
	if !strings.Contains(msg, "Sold 1 item") {
		t.Errorf("expected exactly one item sold, msg=%q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "curio") {
		t.Errorf("sell-all summary should call out kept curios, msg=%q", msg)
	}

	left, err := loadAdvInventory(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Type != "magic_item" {
		t.Fatalf("magic item not preserved: %+v", left)
	}

	// Payout reflects only the junk's €40 (5% pot cut → ~€38 credited).
	bal := euro.GetBalance(uid)
	if bal <= 0 || bal > 40 {
		t.Errorf("payout %v outside expected (0,40] for €40 junk", bal)
	}
}

// TestSellAll_OnlyMagicItems — when the bag is curios only, sell-all reroutes
// to !adventure equip-magic and touches nothing.
func TestSellAll_OnlyMagicItems(t *testing.T) {
	freshShopTestDB(t)
	uid := id.UserID("@sellall_only_magic:test")

	if err := addAdvInventoryItem(uid, AdvItem{Name: "Cloak of Elvenkind", Type: "magic_item", Value: 300}); err != nil {
		t.Fatal(err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	p := &AdventurePlugin{euro: euro}

	msg := p.advSellAll(uid)
	if !strings.Contains(msg, "equip-magic") {
		t.Errorf("expected reroute to equip-magic, msg=%q", msg)
	}
	if euro.GetBalance(uid) != 0 {
		t.Errorf("balance moved on curio-only sell-all: %v", euro.GetBalance(uid))
	}
	left, _ := loadAdvInventory(uid)
	if len(left) != 1 {
		t.Errorf("curio deleted by sell-all: %+v", left)
	}
}

// TestSellItem_MagicItemRerouted — explicit `sell <curio name>` must refuse
// and point the player at the bonding command.
func TestSellItem_MagicItemRerouted(t *testing.T) {
	freshShopTestDB(t)
	uid := id.UserID("@sellone_magic:test")

	curio := AdvItem{Name: "Amulet of Health", Type: "magic_item", Value: 750}
	if err := addAdvInventoryItem(uid, curio); err != nil {
		t.Fatal(err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	p := &AdventurePlugin{euro: euro}

	msg := p.advSellItem(uid, "amulet")
	if !strings.Contains(msg, "equip-magic") {
		t.Errorf("expected reroute to equip-magic, msg=%q", msg)
	}
	if euro.GetBalance(uid) != 0 {
		t.Errorf("balance moved on refused curio sale: %v", euro.GetBalance(uid))
	}
	left, _ := loadAdvInventory(uid)
	if len(left) != 1 {
		t.Errorf("curio deleted by refused sale: %+v", left)
	}
}
