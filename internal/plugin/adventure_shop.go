package plugin

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// ── Shop Listings ────────────────────────────────────────────────────────────

func advShopListings(equip map[EquipmentSlot]*AdvEquipment, balance float64) string {
	var sb strings.Builder
	sb.WriteString("🛒 **Equipment Shop**\n")
	sb.WriteString(fmt.Sprintf("💰 Your balance: €%.0f\n\n", balance))

	for _, slot := range allSlots {
		current := equip[slot]
		currentTier := 0
		if current != nil {
			currentTier = current.Tier
		}

		defs := equipmentTiers[slot]
		currentName := "None"
		if current != nil {
			currentName = current.Name
		}
		sb.WriteString(fmt.Sprintf("**%s** (current: %s, Tier %d)\n", strings.Title(string(slot)), currentName, currentTier))

		hasUpgrades := false
		for _, def := range defs {
			if def.Tier <= currentTier || def.Price == 0 {
				continue
			}
			hasUpgrades = true
			affordable := ""
			if balance < def.Price {
				affordable = " ❌"
			}
			sb.WriteString(fmt.Sprintf("  Tier %d: %s — €%.0f%s\n", def.Tier, def.Name, def.Price, affordable))
			sb.WriteString(fmt.Sprintf("    _%s_\n", advTruncate(def.Description, 120)))
		}
		if !hasUpgrades {
			sb.WriteString("  ✨ Max tier reached!\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("To buy: reply with `buy <item name>`\n")
	sb.WriteString("Example: `buy Sad Iron Sword`")
	return sb.String()
}

func advTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ── Find Shop Item ───────────────────────────────────────────────────────────

func advFindShopItem(name string) (EquipmentSlot, *EquipmentDef, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, slot := range allSlots {
		for i := range equipmentTiers[slot] {
			def := &equipmentTiers[slot][i]
			if def.Price == 0 {
				continue // can't buy tier 0
			}
			if strings.EqualFold(def.Name, name) || containsFold(def.Name, name) {
				return slot, def, true
			}
		}
	}
	return "", nil, false
}

// ── Buy Equipment ────────────────────────────────────────────────────────────

func (p *AdventurePlugin) advBuyEquipment(userID id.UserID, slot EquipmentSlot, def *EquipmentDef, equip map[EquipmentSlot]*AdvEquipment) string {
	current := equip[slot]
	if current != nil && current.Tier >= def.Tier {
		return fmt.Sprintf("You already have %s (Tier %d). %s is Tier %d. That's not an upgrade, that's a lateral move at best.",
			current.Name, current.Tier, def.Name, def.Tier)
	}

	balance := p.euro.GetBalance(userID)
	if balance < def.Price {
		return fmt.Sprintf("You cannot afford %s. You have €%.0f. %s costs €%.0f. "+
			"The gap between these numbers is both mathematical and deeply personal. "+
			"Go do something about it. Not gambling. You know what I mean.",
			def.Name, balance, def.Name, def.Price)
	}

	if !p.euro.Debit(userID, def.Price, "adventure_shop_"+string(slot)) {
		return "Transaction failed. The economy is having a moment."
	}

	// Update equipment
	eq := &AdvEquipment{
		Slot:       slot,
		Tier:       def.Tier,
		Condition:  100,
		Name:       def.Name,
		ActionsUsed: 0,
	}
	if err := saveAdvEquipment(userID, eq); err != nil {
		// Refund on DB error
		p.euro.Credit(userID, def.Price, "adventure_shop_refund")
		return "Something went wrong saving your equipment. You've been refunded."
	}
	equip[slot] = eq

	return fmt.Sprintf("You have purchased **%s** for €%.0f.\n\n_%s_\n\n"+
		"This is an upgrade over what you had. That bar was underground, but you cleared it. "+
		"Equip it now before you do something stupid with the money instead.",
		def.Name, def.Price, def.Description)
}

// ── Sell Items ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) advSellAll(userID id.UserID) string {
	items, err := clearAdvInventory(userID)
	if err != nil {
		return "Failed to access your inventory. Try again."
	}
	if len(items) == 0 {
		return "Your inventory is empty. There is nothing to sell. This is a metaphor for something but now is not the time."
	}

	var total float64
	for _, item := range items {
		total += float64(item.Value)
	}

	p.euro.Credit(userID, total, "adventure_sell_all")

	return fmt.Sprintf("Sold %d items for **€%.0f**.\n\nThe merchant took everything without comment. "+
		"This is the most respect anyone has shown your loot collection. Take the money.",
		len(items), total)
}

func (p *AdventurePlugin) advSellItem(userID id.UserID, itemName string) string {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return "Failed to access your inventory."
	}

	// Fuzzy match
	for _, item := range items {
		if containsFold(item.Name, itemName) {
			if err := removeAdvInventoryItem(item.ID); err != nil {
				return "Failed to sell that item."
			}
			p.euro.Credit(userID, float64(item.Value), "adventure_sell_"+item.Name)
			return fmt.Sprintf("Sold **%s** for **€%d**. The merchant nodded. That's it. That's the transaction.", item.Name, item.Value)
		}
	}

	return fmt.Sprintf("No item matching '%s' found in your inventory.", itemName)
}

// ── Inventory Display ────────────────────────────────────────────────────────

func advInventoryDisplay(userID id.UserID) string {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return "Failed to load inventory."
	}
	if len(items) == 0 {
		return "🎒 Your inventory is empty. Go adventure."
	}

	var sb strings.Builder
	sb.WriteString("🎒 **Inventory**\n\n")

	var total int64
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  %s (T%d %s) — €%d\n", item.Name, item.Tier, item.Type, item.Value))
		total += item.Value
	}
	sb.WriteString(fmt.Sprintf("\n%d items — total value ~€%d", len(items), total))
	sb.WriteString("\n\nTo sell: `!adventure sell all` or `!adventure sell <item>`")
	return sb.String()
}
