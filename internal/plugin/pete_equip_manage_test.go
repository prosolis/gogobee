package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Ask 7: the headless equipment mutators the web equip queue drives. These pin the
// rules that cross the wire — downgrade block, max tier, insufficient funds,
// idempotent replay (one debit), masterwork evict/overwrite/take-off — at the
// gogobee end, on real rows.

// seedEquipPlayer stands up a playable adventurer (player_meta + tier-0 gear) with
// a euro plugin funded to `bankroll`, and returns the wired AdventurePlugin.
func seedEquipPlayer(t *testing.T, uid id.UserID, bankroll float64) *AdventurePlugin {
	t.Helper()
	if err := createAdvCharacter(uid, "Rurina"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	if bankroll > 0 {
		euro.Credit(uid, bankroll, "test bankroll")
	}
	return &AdventurePlugin{euro: euro}
}

func slotOf(t *testing.T, uid id.UserID, slot EquipmentSlot) *AdvEquipment {
	t.Helper()
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		t.Fatalf("loadAdvEquipment: %v", err)
	}
	return equip[slot]
}

// TestPurchaseEquipmentTierHappyAndIdempotentDebit: buying the next tier debits
// once and raises the slot, and the euro move is keyed on the order guid so a
// re-offer moves no money. (A re-offer never re-enters this function in prod — the
// equip_applied_orders ledger short-circuits it — but the guid is the belt to that
// suspenders, and the retry-after-save-fault path below leans on it directly.)
func TestPurchaseEquipmentTierHappyAndIdempotentDebit(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@rurina:test")
	p := seedEquipPlayer(t, uid, 100000)

	before := p.euro.GetBalance(uid)
	price := equipmentTiers[SlotBoots][1].Price // Dead Man's Boots, €75

	status, _, retry := p.purchaseEquipmentTier(uid, SlotBoots, 1, "guid-up-1")
	if retry || status != "applied" {
		t.Fatalf("upgrade = %q retry=%v, want applied", status, retry)
	}
	if got := slotOf(t, uid, SlotBoots); got.Tier != 1 || got.Name != equipmentTiers[SlotBoots][1].Name {
		t.Fatalf("boots slot = %+v, want tier 1", got)
	}
	if got := p.euro.GetBalance(uid); got != before-price {
		t.Fatalf("balance = %.2f, want %.2f (one debit of %.2f)", got, before-price, price)
	}
	// The guid is now a settled money move: a replayed debit on it is a no-op.
	if !p.euro.HasExternalTx("guid-up-1") {
		t.Fatal("the upgrade debit was not logged under the order guid")
	}
	if ok, _, err := p.euro.DebitIdem(uid, price, "adventure_equip_upgrade", "guid-up-1"); err != nil || !ok {
		t.Fatalf("replayed debit = ok:%v err:%v, want a no-op ok", ok, err)
	}
	if got := p.euro.GetBalance(uid); got != before-price {
		t.Fatalf("replayed debit double-charged: balance = %.2f, want %.2f", got, before-price)
	}

	// The retry-after-save-fault path: a prior attempt debited but its slot write
	// never landed, so the slot is still tier 0. The retry must skip the debit
	// (guid already settled) and finish the write, moving no further money.
	helmetGUID := "guid-helm"
	hprice := equipmentTiers[SlotHelmet][1].Price
	if ok, _, err := p.euro.DebitIdem(uid, hprice, "adventure_equip_upgrade", helmetGUID); err != nil || !ok {
		t.Fatalf("seed prior debit = ok:%v err:%v", ok, err)
	}
	mid := p.euro.GetBalance(uid)
	status, _, retry = p.purchaseEquipmentTier(uid, SlotHelmet, 1, helmetGUID)
	if retry || status != "applied" {
		t.Fatalf("retry after fault = %q retry=%v, want applied", status, retry)
	}
	if got := slotOf(t, uid, SlotHelmet); got.Tier != 1 {
		t.Fatalf("helmet not upgraded on retry: tier %d", got.Tier)
	}
	if got := p.euro.GetBalance(uid); got != mid {
		t.Fatalf("retry re-debited: balance %.2f → %.2f", mid, got)
	}
}

// TestPurchaseEquipmentTierRejections: a downgrade, a special-gear slot, the top
// tier, and an empty wallet all bounce without moving the slot or the money.
func TestPurchaseEquipmentTierRejections(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@rurina:test")
	p := seedEquipPlayer(t, uid, 100000)

	// Lift boots to tier 3 so we have something to (not) downgrade from.
	if s, _, _ := p.purchaseEquipmentTier(uid, SlotBoots, 3, "seed-t3"); s != "applied" {
		t.Fatalf("seed to tier 3 = %q", s)
	}
	// Same tier or lower is a downgrade.
	if s, _, _ := p.purchaseEquipmentTier(uid, SlotBoots, 3, "g-eq"); s != "rejected_downgrade" {
		t.Errorf("re-buy same tier = %q, want rejected_downgrade", s)
	}
	if s, _, _ := p.purchaseEquipmentTier(uid, SlotBoots, 2, "g-down"); s != "rejected_downgrade" {
		t.Errorf("buy lower tier = %q, want rejected_downgrade", s)
	}
	// Past the top tier.
	if s, _, _ := p.purchaseEquipmentTier(uid, SlotBoots, 6, "g-max"); s != "rejected_max_tier" {
		t.Errorf("buy tier 6 = %q, want rejected_max_tier", s)
	}
	// A special piece in the slot: buying a plain tier over it strips the bonus.
	weapon := slotOf(t, uid, SlotWeapon)
	weapon.Masterwork = true
	weapon.Tier = 2
	weapon.Name = "Miner's Masterwork Blade"
	weapon.SkillSource = "mining"
	if err := saveAdvEquipment(uid, weapon); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := p.purchaseEquipmentTier(uid, SlotWeapon, 3, "g-special"); s != "rejected_downgrade" {
		t.Errorf("buy plain over masterwork = %q, want rejected_downgrade", s)
	}

	// Insufficient funds: a broke player can't buy a €30000 tier-5 weapon.
	broke := id.UserID("@broke:test")
	pb := seedEquipPlayer(t, broke, 0)
	bal := pb.euro.GetBalance(broke)
	if s, _, _ := pb.purchaseEquipmentTier(broke, SlotWeapon, 5, "g-broke"); s != "rejected_insufficient_funds" {
		t.Errorf("broke upgrade = %q, want rejected_insufficient_funds", s)
	}
	if got := pb.euro.GetBalance(broke); got != bal {
		t.Errorf("a rejected upgrade moved money: %.2f → %.2f", bal, got)
	}
	if got := slotOf(t, broke, SlotWeapon); got.Tier != 0 {
		t.Errorf("a rejected upgrade changed the slot: tier %d", got.Tier)
	}
}

// TestRepairSlotHappyAndNoop: repairing a damaged slot debits the blacksmith cost
// and restores condition; repairing a full slot is a no-op that charges nothing.
func TestRepairSlotHappyAndNoop(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@rurina:test")
	p := seedEquipPlayer(t, uid, 100000)

	weapon := slotOf(t, uid, SlotWeapon)
	weapon.Condition = 50
	if err := saveAdvEquipment(uid, weapon); err != nil {
		t.Fatal(err)
	}
	cost := blacksmithRepairCost(weapon)
	if cost <= 0 {
		t.Fatalf("expected a positive repair cost, got %d", cost)
	}
	before := p.euro.GetBalance(uid)

	status, _, retry := p.repairSlot(uid, SlotWeapon, "guid-rep-1")
	if retry || status != "applied" {
		t.Fatalf("repair = %q retry=%v, want applied", status, retry)
	}
	if got := slotOf(t, uid, SlotWeapon); got.Condition != 100 {
		t.Fatalf("condition = %d, want 100", got.Condition)
	}
	if got := p.euro.GetBalance(uid); got != before-float64(cost) {
		t.Fatalf("balance = %.2f, want %.2f (debit %d)", got, before-float64(cost), cost)
	}

	// Now at full condition: a fresh repair is an applied no-op, no charge.
	after := p.euro.GetBalance(uid)
	status, _, _ = p.repairSlot(uid, SlotWeapon, "guid-rep-2")
	if status != "applied" {
		t.Fatalf("no-op repair = %q, want applied", status)
	}
	if got := p.euro.GetBalance(uid); got != after {
		t.Errorf("no-op repair charged: %.2f → %.2f", after, got)
	}
}

// TestMasterworkEquipEvictsOverwritesAndTakesOff: a masterwork piece equips into a
// plain slot (overwriting the tier), a better one evicts the special occupant back
// to the pack, a worse one is blocked, and take-off resets the slot to tier 0.
func TestMasterworkEquipEvictsOverwritesAndTakesOff(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@rurina:test")
	seedEquipPlayer(t, uid, 0)

	mw := func(name string, tier int) AdvItem {
		if err := addAdvInventoryItem(uid, AdvItem{Name: name, Type: "MasterworkGear", Tier: tier, Slot: SlotWeapon, SkillSource: "mining"}); err != nil {
			t.Fatal(err)
		}
		inv, _ := loadAdvInventory(uid)
		for _, it := range inv {
			if it.Name == name {
				return it
			}
		}
		t.Fatalf("just-added item %q not in inventory", name)
		return AdvItem{}
	}

	// Equip a T3 masterwork over the tier-0 plain weapon: it overwrites, no eviction.
	out, err := applyMasterworkEquip(uid, mw("Deepforged Blade", 3))
	if err != nil {
		t.Fatalf("equip T3: %v", err)
	}
	if out.SwappedBack != "" {
		t.Errorf("plain occupant should be overwritten, not evicted; got swap %q", out.SwappedBack)
	}
	if got := slotOf(t, uid, SlotWeapon); !got.Masterwork || got.Tier != 3 || got.Name != "Deepforged Blade" {
		t.Fatalf("weapon = %+v, want masterwork T3 Deepforged Blade", got)
	}

	// A better masterwork (T4) evicts the T3 back to the pack.
	out, err = applyMasterworkEquip(uid, mw("Sunforged Blade", 4))
	if err != nil {
		t.Fatalf("equip T4: %v", err)
	}
	if out.SwappedBack != "Deepforged Blade" {
		t.Errorf("evicted = %q, want Deepforged Blade", out.SwappedBack)
	}
	invHas := func(name string) bool {
		inv, _ := loadAdvInventory(uid)
		for _, it := range inv {
			if it.Name == name {
				return true
			}
		}
		return false
	}
	if !invHas("Deepforged Blade") {
		t.Error("the evicted T3 masterwork is not back in the pack")
	}

	// A worse masterwork (T2) is a blocked downgrade.
	if _, err := applyMasterworkEquip(uid, mw("Rusty Masterwork", 2)); err != errEquipDowngrade {
		t.Errorf("equip worse masterwork err = %v, want errEquipDowngrade", err)
	}

	// Take off the worn T4: it returns to the pack and the slot resets to tier 0.
	un, err := applyMasterworkUnequip(uid, SlotWeapon)
	if err != nil {
		t.Fatalf("take off: %v", err)
	}
	if un.Name != "Sunforged Blade" {
		t.Errorf("took off %q, want Sunforged Blade", un.Name)
	}
	if got := slotOf(t, uid, SlotWeapon); got.Masterwork || got.Tier != 0 || got.Name != equipmentTiers[SlotWeapon][0].Name {
		t.Fatalf("weapon after take-off = %+v, want tier-0 default", got)
	}
	if !invHas("Sunforged Blade") {
		t.Error("the taken-off masterwork is not back in the pack")
	}

	// Taking off a plain slot has nothing round-trippable.
	if _, err := applyMasterworkUnequip(uid, SlotArmor); err != errSlotEmpty {
		t.Errorf("take off plain slot err = %v, want errSlotEmpty", err)
	}
}

// TestBuildEquipSlotViews: the panel snapshot offers an upgrade only over a plain
// sub-max slot, take-off only on special gear, and a repair cost only when damaged.
func TestBuildEquipSlotViews(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@rurina:test")
	seedEquipPlayer(t, uid, 0)

	// Boots: masterwork T3, damaged → take off + repair, no upgrade offer.
	boots := slotOf(t, uid, SlotBoots)
	boots.Masterwork = true
	boots.Tier = 3
	boots.Name = "The Wandering Sole"
	boots.Condition = 70
	if err := saveAdvEquipment(uid, boots); err != nil {
		t.Fatal(err)
	}
	// Helmet: plain T2 → upgrade to T3 offered, no take off, no repair.
	helmet := slotOf(t, uid, SlotHelmet)
	helmet.Tier = 2
	helmet.Name = equipmentTiers[SlotHelmet][2].Name
	if err := saveAdvEquipment(uid, helmet); err != nil {
		t.Fatal(err)
	}

	views := buildEquipSlotViews(uid)
	bySlot := map[string]struct {
		takeOff  bool
		nextTier int
		repair   int
	}{}
	for _, v := range views {
		bySlot[v.Slot] = struct {
			takeOff  bool
			nextTier int
			repair   int
		}{v.CanTakeOff, v.NextTier, v.RepairCost}
	}
	if len(views) != len(allSlots) {
		t.Fatalf("got %d slot views, want %d", len(views), len(allSlots))
	}
	if b := bySlot["boots"]; !b.takeOff || b.nextTier != 0 || b.repair <= 0 {
		t.Errorf("boots view = %+v, want take-off, no upgrade, positive repair", b)
	}
	if h := bySlot["helmet"]; h.takeOff || h.nextTier != 3 || h.repair != 0 {
		t.Errorf("helmet view = %+v, want upgrade to T3, no take-off, no repair", h)
	}
	// A pristine tier-0 slot: upgrade offered to T1, no take-off, no repair.
	if w := bySlot["weapon"]; w.takeOff || w.nextTier != 1 || w.repair != 0 {
		t.Errorf("weapon view = %+v, want upgrade to T1", w)
	}
}
