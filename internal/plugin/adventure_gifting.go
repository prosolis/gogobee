package plugin

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// N4/E2 item gifting. `!give <item> @user` hands a consumable or non-magic gear
// item to another adventurer. The sender pays a 5% handling fee (of item value)
// to the community pot — the same civic tax every payout pays — and is capped at
// a few gifts a day so the feature can't become a twink-funnel for a fresh alt.
// Magic items are deliberately non-giftable: attunement and the BiS economy stay
// personal (gogobee_engagement_plan.md §E2).
const (
	giftDailyCap = 3
	giftTaxRate  = 0.05
)

// giftableTypes is the allowlist of AdvItem.Type values a player may gift.
// Everything in adventure_inventory is unequipped by definition (equipped gear
// lives in adventure_equipment / magic_item_equipped), so "unequipped" needs no
// separate check. Magic items ("magic_item") and raw materials are excluded.
var giftableTypes = map[string]bool{
	"consumable":     true,
	"MasterworkGear": true,
	"ArenaGear":      true,
}

// isGiftableItem reports whether an inventory item may be gifted, and a reason
// when it may not.
func isGiftableItem(it AdvItem) (bool, string) {
	if it.Type == "magic_item" {
		return false, "Magic items stay personal — attunement doesn't transfer. Try a consumable or a piece of gear."
	}
	if !giftableTypes[it.Type] {
		return false, "You can only gift consumables and non-magic gear. Raw materials and treasures aren't giftable."
	}
	return true, ""
}

// pickGiftableItem resolves the item a `!give` query names, preferring a giftable
// match so a non-giftable item (a magic item, raw materials) that also matches
// can't shadow a giftable one and block the gift — the way plain findInventoryMatch
// would, since it returns the first match across the whole inventory. Returns
// (item, "") on success; (nil, "") when nothing matches at all; and (nil, reason)
// when only non-giftable items match, so the caller can explain why.
func pickGiftableItem(inv []AdvItem, query string) (*AdvItem, string) {
	match := findInventoryMatch(inv, query)
	if match == nil {
		return nil, ""
	}
	if ok, _ := isGiftableItem(*match); ok {
		return match, ""
	}
	giftables := make([]AdvItem, 0, len(inv))
	for _, it := range inv {
		if ok, _ := isGiftableItem(it); ok {
			giftables = append(giftables, it)
		}
	}
	if alt := findInventoryMatch(giftables, query); alt != nil {
		return alt, ""
	}
	_, reason := isGiftableItem(*match)
	return nil, reason
}

// giftCountToday returns how many gifts the sender has already sent on the given
// UTC day. The cap is enforced against the persisted log, so a restart can't
// reset someone's daily allowance.
func giftCountToday(sender id.UserID, day string) int {
	var n int
	_ = db.Get().QueryRow(
		`SELECT COUNT(*) FROM adventure_gift_log WHERE sender = ? AND gift_day = ?`,
		string(sender), day).Scan(&n)
	return n
}

// logGift records a completed gift for the daily cap and the audit trail.
func logGift(sender, recipient id.UserID, itemName string, value int64, day string) error {
	_, err := db.Get().Exec(
		`INSERT INTO adventure_gift_log (sender, recipient, item_name, value, gift_day)
		 VALUES (?, ?, ?, ?, ?)`,
		string(sender), string(recipient), itemName, value, day)
	return err
}

// transferInventoryItem moves one inventory row from one owner to another. Scoped
// to the current owner so a stale id can't move a row that has already changed
// hands, and forces in_vault=0 so a gift always lands in the recipient's active
// pack rather than a phantom vault slot.
func transferInventoryItem(itemID int64, from, to id.UserID) (bool, error) {
	res, err := db.Get().Exec(
		`UPDATE adventure_inventory SET user_id = ?, in_vault = 0 WHERE id = ? AND user_id = ?`,
		string(to), itemID, string(from))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// handleGiveCmd routes `!give <item> @user`.
func (p *AdventurePlugin) handleGiveCmd(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "give"))

	// Serialize a sender's own gifts so two near-simultaneous !give commands
	// can't both clear the daily cap or double-spend the same item row.
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` to create one first.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. Your things stay where they are for now.")
	}

	// The target is the trailing token; everything before it is the item name.
	fields := strings.Fields(args)
	if len(fields) < 2 {
		return p.SendDM(ctx.Sender, "Usage: `!give <item> @user` — e.g. `!give Health Potion @alex`.")
	}
	targetRaw := fields[len(fields)-1]
	itemQuery := strings.TrimSpace(strings.TrimSuffix(args, targetRaw))

	target, ok := p.ResolveUser(targetRaw, ctx.RoomID)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Couldn't find a user matching %q. Mention them or use their exact name.", targetRaw))
	}
	if target == ctx.Sender {
		return p.SendDM(ctx.Sender, "Regifting to yourself? That's just moving things around.")
	}

	// Recipient must have completed !setup.
	rc, rerr := LoadDnDCharacter(target)
	if rerr != nil || rc == nil || rc.PendingSetup {
		name := p.DisplayName(target)
		return p.SendDM(ctx.Sender, fmt.Sprintf("%s hasn't set up an adventurer yet — they need to run `!setup` before they can receive gifts.", name))
	}

	day := time.Now().UTC().Format("2006-01-02")
	if sent := giftCountToday(ctx.Sender, day); sent >= giftDailyCap {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You've already sent %d gifts today (the daily limit). Try again tomorrow.", sent))
	}

	inv, err := loadAdvInventory(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't reach your inventory. Try again in a moment.")
	}
	match, reason := pickGiftableItem(inv, itemQuery)
	if match == nil {
		if reason == "" {
			return p.SendDM(ctx.Sender, fmt.Sprintf("No inventory item matches %q. Check `!adventure inventory`.", itemQuery))
		}
		return p.SendDM(ctx.Sender, reason)
	}

	tax := int(math.Round(float64(match.Value) * giftTaxRate))
	if tax > 0 {
		if bal := p.euro.GetBalance(ctx.Sender); bal < float64(tax) {
			return p.SendDM(ctx.Sender, fmt.Sprintf("Gifting **%s** carries a €%d handling fee (5%% of its value), and you only have €%.0f.", match.Name, tax, bal))
		}
		if !p.euro.Debit(ctx.Sender, float64(tax), "adventure_gift_tax") {
			return p.SendDM(ctx.Sender, "Transaction failed. The economy is having a moment.")
		}
	}

	moved, err := transferInventoryItem(match.ID, ctx.Sender, target)
	if err != nil || !moved {
		if tax > 0 {
			p.euro.Credit(ctx.Sender, float64(tax), "adventure_gift_refund")
		}
		return p.SendDM(ctx.Sender, "Couldn't hand the item over. Nothing changed — try again in a moment.")
	}
	if tax > 0 {
		communityPotAdd(tax)
		trackTaxPaid(ctx.Sender, tax)
	}
	_ = logGift(ctx.Sender, target, match.Name, match.Value, day)

	senderName := p.DisplayName(ctx.Sender)
	recipName := p.DisplayName(target)
	_ = p.SendDM(target, fmt.Sprintf("🎁 **%s** sent you **%s**! It's in your inventory now — `!adventure inventory` to take a look.", senderName, match.Name))

	remaining := giftDailyCap - giftCountToday(ctx.Sender, day)
	feeNote := ""
	if tax > 0 {
		feeNote = fmt.Sprintf(" (€%d handling fee to the community pot)", tax)
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("🎁 Sent **%s** to %s%s. %d gift(s) left today.", match.Name, recipName, feeNote, remaining))
}
