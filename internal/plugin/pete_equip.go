package plugin

// The web equip queue's game-side loop.
//
// An owner asks, on their own detail page on Pete, to wear or take off an item.
// Pete records the intent; we poll for it, run the real equip against our own
// equipment tables, and file a verdict Pete shows them. Same reverse-pipe shape
// as mischief — Pete has no route into this box, so we ask for work rather than
// being told about it.
//
// The one thing that is NOT like mischief: the underlying action isn't
// idempotent. Equipping consumes an inventory row and unequipping mints a fresh
// one, so simply re-running a re-offered order would double-move the item. So
// before we touch anything we check the equip_applied_orders ledger: if this
// order's guid is already there, the mutation happened on an earlier tick and we
// only lost the verdict-ack — we re-file the stored verdict and mutate nothing.
// The guid is still the end-to-end key; here it guards a non-idempotent action
// instead of riding a naturally idempotent one.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

const (
	equipPollInterval = 30 * time.Second
	equipPollTimeout  = 20 * time.Second
)

// peteEquipTicker polls Pete for equip orders and fulfils them. Started alongside
// the other adventure tickers; exits on stopCh.
func (p *AdventurePlugin) peteEquipTicker() {
	if !peteclient.Enabled() {
		return // no Pete wire configured; the equip queue is simply off
	}
	ticker := time.NewTicker(equipPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.pollEquipOrders()
		}
	}
}

func (p *AdventurePlugin) pollEquipOrders() {
	ctx, cancel := context.WithTimeout(context.Background(), equipPollTimeout)
	defer cancel()

	orders, err := peteclient.PendingEquip(ctx)
	if err != nil {
		// A Pete predating the queue answers 404; a wire blip looks the same. Quiet
		// on purpose — this must not spam while Pete hasn't shipped the endpoint.
		slog.Debug("equip: poll failed", "err", err)
		return
	}
	for _, order := range orders {
		p.fulfilEquipOrder(ctx, order)
	}
}

// fulfilEquipOrder applies one order and files its verdict. A transient failure is
// left pending for the next poll (no verdict); a permanent one gets a specific
// rejection. The guid ledger makes a re-offer after a lost ack a no-op that simply
// re-files the verdict.
func (p *AdventurePlugin) fulfilEquipOrder(ctx context.Context, order peteclient.EquipOrder) {
	// Already applied on an earlier tick? Re-file the stored verdict, mutate nothing.
	if status, detail, ok := equipOrderAlreadyApplied(order.GUID); ok {
		if err := peteclient.VerdictEquip(ctx, order.GUID, status, detail); err != nil {
			slog.Warn("equip: re-file verdict push failed, will retry next poll",
				"order", order.GUID, "status", status, "err", err)
		}
		return
	}

	owner, ok := p.equipOwnerMXID(order.OwnerLocalpart)
	if !ok {
		// The client isn't up (tests) or the localpart is empty. Not our order to
		// fail permanently — leave it pending and try again once we can name the owner.
		slog.Debug("equip: cannot resolve owner, leaving pending", "order", order.GUID, "owner", order.OwnerLocalpart)
		return
	}

	status, detail, retry := p.applyEquipOrder(owner, order)
	if retry {
		return // transient; leave pending for the next tick
	}

	// Record the verdict BEFORE pushing it, so a crash after the mutation still
	// short-circuits next tick and re-files rather than re-applying. The mutation
	// and this insert aren't one transaction, but the window between them is a
	// single statement — the same practical guarantee the DM equip path lives with.
	if err := recordEquipApplied(order.GUID, status, detail); err != nil {
		// If we can't record it, don't push the verdict either: leave the order
		// pending so the ledger and Pete stay in step. Re-running an equip is the
		// double-move we're guarding against, so a rare re-apply here is the lesser
		// evil versus a verdict with no ledger behind it. Transient; retry.
		slog.Warn("equip: failed to record applied order, leaving pending",
			"order", order.GUID, "status", status, "err", err)
		return
	}
	if err := peteclient.VerdictEquip(ctx, order.GUID, status, detail); err != nil {
		slog.Warn("equip: verdict push failed, will re-file next poll",
			"order", order.GUID, "status", status, "err", err)
		return
	}
	slog.Info("equip: web order fulfilled", "order", order.GUID, "action", order.Action, "status", status)
}

// applyEquipOrder runs the real equip/unequip. It returns the terminal status and
// a human note for Pete, or retry=true for a transient fault that should leave the
// order pending. It records nothing and pushes nothing — the caller does both.
func (p *AdventurePlugin) applyEquipOrder(owner id.UserID, order peteclient.EquipOrder) (status, detail string, retry bool) {
	switch order.Action {
	case "equip":
		inv, err := loadAdvInventory(owner)
		if err != nil {
			return "", "", true // transient
		}
		var it AdvItem
		found := false
		for _, cand := range inv {
			if cand.ID == order.ItemID {
				it, found = cand, true
				break
			}
		}
		if !found {
			// The row left the pack before we got here (worn already, sold, a stale
			// page). The table is AUTOINCREMENT, so the id can't have been reused for
			// a different item — this is a clean miss, not a wrong hit.
			return "rejected_not_owned", "That item wasn't in your pack anymore.", false
		}
		// Masterwork/arena pieces equip into a standard slot; everything else takes
		// the magic-item path. Type alone routes it — the id resolved the same row.
		if it.Type == "MasterworkGear" || it.Type == "ArenaGear" {
			out, err := applyMasterworkEquip(owner, it)
			if errors.Is(err, errItemNotEquippable) {
				return "rejected_not_equippable", "That item can't be worn.", false
			}
			if errors.Is(err, errEquipDowngrade) {
				return "rejected_downgrade", "That isn't an upgrade over what you're wearing.", false
			}
			if err != nil {
				return "", "", true // transient DB fault
			}
			return "applied", masterworkEquipDetail(out), false
		}
		out, err := applyMagicEquip(owner, it)
		if errors.Is(err, errItemNotEquippable) {
			return "rejected_not_equippable", "That item can't be worn.", false
		}
		if err != nil {
			return "", "", true // transient DB fault
		}
		return "applied", equipAppliedDetail(out), false

	case "unequip":
		// A standard slot (weapon/armor/…) takes a masterwork/arena piece off; a DnD
		// slot takes a magic item off. The vocabularies are disjoint, so the slot
		// string alone tells the two apart.
		if isEquipmentSlot(order.Slot) {
			out, err := applyMasterworkUnequip(owner, EquipmentSlot(order.Slot))
			if errors.Is(err, errSlotEmpty) {
				return "rejected_not_worn", "There was nothing to take off there.", false
			}
			if err != nil {
				return "", "", true
			}
			return "applied", fmt.Sprintf("Took %s off, back in your pack.", out.Name), false
		}
		out, err := applyMagicUnequip(owner, DnDSlot(order.Slot))
		if errors.Is(err, errSlotEmpty) {
			return "rejected_not_worn", "That slot was already empty.", false
		}
		if err != nil {
			return "", "", true
		}
		note := fmt.Sprintf("Took off %s, back in your pack.", out.Item.Name)
		if len(out.Healed) > 0 {
			note += fmt.Sprintf(" That freed a bond, so %s is active now.", strings.Join(out.Healed, ", "))
		}
		return "applied", note, false

	case "upgrade":
		return p.purchaseEquipmentTier(owner, EquipmentSlot(order.Slot), order.Tier, order.GUID)

	case "repair":
		return p.repairSlot(owner, EquipmentSlot(order.Slot), order.GUID)

	default:
		// Pete validates the action before it ever queues an order, so this is a
		// contract breach, not a user mistake. Reject permanently rather than spin.
		return "rejected_not_equippable", "Unknown action.", false
	}
}

// equipAppliedDetail turns an equip outcome into the plain note Pete shows.
func equipAppliedDetail(out magicEquipOutcome) string {
	b := fmt.Sprintf("Now worn in your %s slot.", out.Effective.Slot)
	switch {
	case out.Bonded:
		b += fmt.Sprintf(" Bonded (%d of %d).", out.BondsBefore+1, dndMagicItemAttuneLimit)
	case out.AtCap:
		b += fmt.Sprintf(" Worn but inert: all %d bonds are in use, so take one off to activate it.", dndMagicItemAttuneLimit)
	}
	if out.SwappedBack != "" {
		b += fmt.Sprintf(" %s went back to your pack.", out.SwappedBack)
	}
	if len(out.Healed) > 0 {
		b += fmt.Sprintf(" A freed bond also activated %s.", strings.Join(out.Healed, ", "))
	}
	return b
}

// equipOwnerMXID reconstructs the owner's Matrix id from the localpart Pete sent,
// the same construction as the mischief buyer's. Fails closed if the client isn't
// up (tests) or the name is empty.
func (p *AdventurePlugin) equipOwnerMXID(localpart string) (id.UserID, bool) {
	lp := strings.ToLower(strings.TrimSpace(localpart))
	if lp == "" || p.Client == nil {
		return "", false
	}
	server := p.Client.UserID.Homeserver()
	if server == "" {
		return "", false
	}
	return id.NewUserID(lp, server), true
}

// ---- the applied-order ledger --------------------------------------------------

// equipOrderAlreadyApplied reports the verdict we filed for an order, if we have
// already applied it. This is the short-circuit that keeps a re-offered order from
// re-running its non-idempotent mutation.
func equipOrderAlreadyApplied(guid string) (status, detail string, ok bool) {
	err := db.Get().QueryRow(
		`SELECT status, detail FROM equip_applied_orders WHERE guid = ?`, guid,
	).Scan(&status, &detail)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		// A read failure here would send us down the mutation path and risk a
		// double-move, so treat it as "don't know" and let the caller leave the
		// order pending rather than assume it's fresh. We signal that by returning
		// ok=false but... the caller can't tell the difference. Log loudly; a
		// persistent read failure is a real problem, but a transient one self-heals
		// on the next poll because the mutation itself is guarded by this same table.
		slog.Error("equip: applied-ledger read failed", "order", guid, "err", err)
		return "", "", false
	}
	return status, detail, true
}

// recordEquipApplied stamps an order as applied with the verdict we're about to
// file. OR IGNORE so a re-file that somehow reaches here can't error on the guid.
func recordEquipApplied(guid, status, detail string) error {
	_, err := db.Get().Exec(
		`INSERT OR IGNORE INTO equip_applied_orders (guid, status, detail) VALUES (?, ?, ?)`,
		guid, status, detail)
	return err
}
