package plugin

// Ask 7: full equipment management from the web.
//
// The magic-item equip path (magic_items_gameplay.go) only ever touched the DnD
// slots — off_hand, rings, and the like — which are almost always empty. Almost
// everything a player actually wears lives in the OTHER two systems: the 5
// standard EquipmentSlots (weapon/armor/helmet/boots/tool), whose power is the
// slot's integer Tier, and the masterwork/arena pieces that get equipped INTO a
// standard slot. This file is the game-side of managing all of that from Pete:
//
//   - applyMasterworkEquip / applyMasterworkUnequip — move a masterwork/arena
//     piece between the pack and a standard slot (no money).
//   - purchaseEquipmentTier — buy the next standard tier with euros (confirm-gated
//     on the web), the headless twin of the shop's advBuyEquipment.
//   - repairSlot — mend a slot's condition with euros, the headless twin of the
//     blacksmith's executeRepair.
//   - buildEquipSlotViews — the owner-only snapshot the web panel renders from.
//
// The two euro-spending mutators run on the retrying poll wire, so every money
// move goes through the idempotent euro variants keyed on the order guid: a
// re-offered order that already debited skips the charge and just re-runs the
// idempotent slot write. That is why neither refunds on a later DB fault — a
// refund keyed on a fresh id, followed by a guid-guarded retry that no longer
// re-debits, would hand the player both the gear and their money back. The casino
// escrow (pete_games.go) settles the same way: idempotent move, then retry.

import (
	"errors"
	"fmt"
	"log/slog"

	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

// errEquipDowngrade is a permanent refusal: the incoming piece is no better than
// what is worn. Downgrades are blocked by user decision (equip and upgrade both).
var errEquipDowngrade = errors.New("equip: would be a downgrade")

// mwEquipOutcome is what a masterwork/arena equip did, for the verdict note.
type mwEquipOutcome struct {
	Name        string
	Slot        EquipmentSlot
	Tier        int
	Arena       bool
	SwappedBack string // the special occupant evicted back to the pack, or ""
}

// applyMasterworkEquip wears one masterwork/arena backpack piece into its standard
// slot, mirroring the magic path's anti-duplication ordering: evict any special
// occupant, then remove the incoming row FIRST, then write the slot, restoring the
// row on a save fault. One implementation of that ordering per family of gear —
// the DM confirm handler (adventure_masterwork.go) is the message-carrying twin.
func applyMasterworkEquip(uid id.UserID, it AdvItem) (mwEquipOutcome, error) {
	if it.Slot == "" || (it.Type != "MasterworkGear" && it.Type != "ArenaGear") {
		return mwEquipOutcome{}, errItemNotEquippable
	}
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return mwEquipOutcome{}, err
	}
	slot := it.Slot
	cur := equip[slot]

	// Downgrade block: the incoming effective tier must beat the current occupant.
	incoming := &AdvEquipment{Tier: it.Tier}
	if it.Type == "ArenaGear" {
		incoming.ArenaTier = it.Tier
	} else {
		incoming.Masterwork = true
	}
	if advEffectiveTier(incoming) <= advEffectiveTier(cur) {
		return mwEquipOutcome{}, errEquipDowngrade
	}

	// Evict a special occupant back to the pack. A plain shop-tier occupant is not
	// an item — it is just the slot's tier — so it is overwritten, not evicted, the
	// same as the DM confirm handler and the shop.
	var swappedBack string
	if cur != nil && (cur.Masterwork || cur.ArenaTier > 0) {
		old := AdvItem{Name: cur.Name, Type: "MasterworkGear", Tier: cur.Tier, Slot: slot, SkillSource: cur.SkillSource}
		if cur.ArenaTier > 0 {
			old.Type = "ArenaGear"
		}
		if err := addAdvInventoryItem(uid, old); err != nil {
			return mwEquipOutcome{}, err
		}
		swappedBack = cur.Name
	}

	// Destructive op first: pull the incoming row before writing the slot, so a save
	// fault can't leave it both worn and in the pack. Restore it on failure.
	if err := removeAdvInventoryItem(it.ID); err != nil {
		return mwEquipOutcome{}, err
	}
	eq := cur
	if eq == nil {
		eq = &AdvEquipment{Slot: slot}
	}
	eq.Tier = it.Tier
	eq.Condition = 100
	eq.Name = it.Name
	eq.ActionsUsed = 0
	if it.Type == "ArenaGear" {
		eq.Masterwork = false
		eq.SkillSource = ""
		eq.ArenaTier = it.Tier
		eq.ArenaSet = ""
		if gs := arenaGearByName(it.Name); gs != nil {
			eq.ArenaSet = gs.SetKey
		}
	} else {
		eq.ArenaTier = 0
		eq.ArenaSet = ""
		eq.Masterwork = true
		eq.SkillSource = it.SkillSource
	}
	if err := saveAdvEquipment(uid, eq); err != nil {
		restored := AdvItem{Name: it.Name, Type: it.Type, Tier: it.Tier, Value: it.Value, Slot: it.Slot, SkillSource: it.SkillSource}
		if rbErr := addAdvInventoryItem(uid, restored); rbErr != nil {
			slog.Error("equip: masterwork save failed AND inventory rollback failed",
				"user", uid, "item", it.Name, "save_err", err, "rollback_err", rbErr)
		}
		return mwEquipOutcome{}, err
	}
	return mwEquipOutcome{Name: it.Name, Slot: slot, Tier: it.Tier, Arena: it.Type == "ArenaGear", SwappedBack: swappedBack}, nil
}

// mwUnequipOutcome is what a masterwork/arena take-off did.
type mwUnequipOutcome struct {
	Name string
	Slot EquipmentSlot
}

// applyMasterworkUnequip takes a worn masterwork/arena piece off a standard slot,
// returns it to the pack, and resets the slot to its tier-0 default. A plain
// shop-tier slot has nothing round-trippable (its tier is not an item), so that is
// errSlotEmpty — reverting a shop tier is not a take-off. The 5 slot rows are an
// invariant (PK user_id+slot), so the row is reset, never deleted. Destructive op
// first — reset the slot, then mint the pack row, restoring the slot on failure —
// mirroring the magic unequip so a fault can't duplicate the piece.
func applyMasterworkUnequip(uid id.UserID, slot EquipmentSlot) (mwUnequipOutcome, error) {
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return mwUnequipOutcome{}, err
	}
	cur := equip[slot]
	if cur == nil || (!cur.Masterwork && cur.ArenaTier == 0) {
		return mwUnequipOutcome{}, errSlotEmpty
	}
	prev := *cur // snapshot for rollback

	def0 := equipmentTiers[slot][0]
	reset := &AdvEquipment{Slot: slot, Tier: 0, Condition: 100, Name: def0.Name, ActionsUsed: 0, ArenaTier: 0, ArenaSet: "", Masterwork: false, SkillSource: ""}
	if err := saveAdvEquipment(uid, reset); err != nil {
		return mwUnequipOutcome{}, err
	}

	old := AdvItem{Name: cur.Name, Type: "MasterworkGear", Tier: cur.Tier, Slot: slot, SkillSource: cur.SkillSource}
	if cur.ArenaTier > 0 {
		old.Type = "ArenaGear"
	}
	if err := addAdvInventoryItem(uid, old); err != nil {
		if rbErr := saveAdvEquipment(uid, &prev); rbErr != nil {
			slog.Error("equip: masterwork take-off failed AND slot rollback failed",
				"user", uid, "slot", slot, "add_err", err, "rollback_err", rbErr)
		}
		return mwUnequipOutcome{}, err
	}
	return mwUnequipOutcome{Name: cur.Name, Slot: slot}, nil
}

// purchaseEquipmentTier buys a standard slot's tier with euros — the headless twin
// of advBuyEquipment, minus flavor. It returns a terminal verdict for Pete or
// retry=true for a transient fault. Money moves once, keyed on the order guid; the
// web only ever offers the next tier over a PLAIN shop-tier slot (buildEquipSlotViews
// suppresses the offer on special gear), so there is no occupant to evict here and
// the whole body is idempotent under a re-offered order.
func (p *AdventurePlugin) purchaseEquipmentTier(uid id.UserID, slot EquipmentSlot, tier int, guid string) (status, detail string, retry bool) {
	defs, ok := equipmentTiers[slot]
	if !ok {
		return "rejected_not_equippable", "That isn't an equipment slot.", false
	}
	if tier < 1 || tier >= len(defs) {
		// tier 0 is the free default, not a purchase; >= len is past the top tier.
		return "rejected_max_tier", "That slot is already at the top tier.", false
	}
	def := defs[tier]

	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return "", "", true
	}
	cur := equip[slot]
	if cur != nil {
		// Buying a shop tier over a special piece strips its bonus — a downgrade in
		// practice even when the raw number rises. Take it off first, then buy.
		if cur.Masterwork || cur.ArenaTier > 0 {
			return "rejected_downgrade", "Take off your special gear in that slot before buying a tier.", false
		}
		if cur.Tier >= def.Tier {
			return "rejected_downgrade", "You already have that tier or better.", false
		}
	}

	price := def.Price
	if !p.euro.HasExternalTx(guid) {
		ok, _, err := p.euro.DebitIdem(uid, price, "adventure_equip_upgrade", guid)
		if err != nil {
			return "", "", true
		}
		if !ok {
			return "rejected_insufficient_funds", fmt.Sprintf("That upgrade costs €%.0f and you can't cover it.", price), false
		}
	}

	eq := &AdvEquipment{Slot: slot, Tier: def.Tier, Condition: 100, Name: def.Name, ActionsUsed: 0}
	if err := saveAdvEquipment(uid, eq); err != nil {
		// No refund: the debit is guid-idempotent, so the next poll re-runs this with
		// the charge already settled and only the (idempotent) slot write left to do.
		// Refunding here would double-pay once that retry lands the gear.
		return "", "", true
	}
	return "applied", fmt.Sprintf("Upgraded your %s to %s (T%d) for €%.0f.", slot, def.Name, def.Tier, price), false
}

// repairSlot mends one standard slot's condition with euros — the headless twin of
// the blacksmith's executeRepair. Idempotent on the order guid: the debit runs
// once, and setting condition to 100 is itself idempotent, so a re-offered order is
// safe with no refund.
func (p *AdventurePlugin) repairSlot(uid id.UserID, slot EquipmentSlot, guid string) (status, detail string, retry bool) {
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return "", "", true
	}
	eq := equip[slot]
	if eq == nil {
		return "rejected_not_worn", "There's nothing in that slot to repair.", false
	}
	cost := blacksmithRepairCost(eq)
	if cost <= 0 {
		// Already full — nothing to charge for. Report it as applied so the order
		// reaches a terminal state rather than parking.
		return "applied", "That piece was already at full condition.", false
	}
	if !p.euro.HasExternalTx(guid) {
		ok, _, err := p.euro.DebitIdem(uid, float64(cost), "adventure_repair", guid)
		if err != nil {
			return "", "", true
		}
		if !ok {
			return "rejected_insufficient_funds", fmt.Sprintf("The repair costs €%d and you can't cover it.", cost), false
		}
	}
	eq.Condition = 100
	if err := saveAdvEquipment(uid, eq); err != nil {
		return "", "", true // retry; the idempotent debit means no double-charge
	}
	return "applied", fmt.Sprintf("Repaired your %s for €%d.", eq.Name, cost), false
}

// buildEquipSlotViews is the owner-only snapshot of the 5 standard slots the web
// management panel renders from. Worn masterwork/arena pieces surface here (via
// CanTakeOff), not in the magic Equipped set. An upgrade is offered only over a
// plain shop-tier slot below max — a special piece is taken off, not shop-upgraded.
func buildEquipSlotViews(uid id.UserID) []peteclient.EquipSlotView {
	equip, err := loadAdvEquipment(uid)
	if err != nil {
		return nil
	}
	var out []peteclient.EquipSlotView
	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil {
			continue
		}
		v := peteclient.EquipSlotView{
			Slot:       string(slot),
			Name:       eq.Name,
			Tier:       eq.Tier,
			Condition:  eq.Condition,
			Masterwork: eq.Masterwork,
			ArenaTier:  eq.ArenaTier,
			CanTakeOff: eq.Masterwork || eq.ArenaTier > 0,
			RepairCost: blacksmithRepairCost(eq),
		}
		if !eq.Masterwork && eq.ArenaTier == 0 && eq.Tier < 5 {
			next := equipmentTiers[slot][eq.Tier+1]
			v.NextTier = next.Tier
			v.NextName = next.Name
			v.NextPrice = next.Price
		}
		out = append(out, v)
	}
	return out
}

// masterworkEquipDetail turns a masterwork/arena equip outcome into the verdict
// note Pete shows.
func masterworkEquipDetail(out mwEquipOutcome) string {
	kind := "masterwork"
	if out.Arena {
		kind = "arena"
	}
	b := fmt.Sprintf("Now worn in your %s slot (%s T%d).", out.Slot, kind, out.Tier)
	if out.SwappedBack != "" {
		b += fmt.Sprintf(" %s went back to your pack.", out.SwappedBack)
	}
	return b
}

// isEquipmentSlot reports whether a slot string names one of the 5 standard slots.
// The magic DnD slots and the standard slots are disjoint vocabularies, so this
// alone routes an unequip to the right path.
func isEquipmentSlot(slot string) bool {
	for _, s := range allSlots {
		if string(s) == slot {
			return true
		}
	}
	return false
}
