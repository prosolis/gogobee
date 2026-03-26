package plugin

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// ── Shop Listings ────────────────────────────────────────────────────────────

// slotEmoji returns a display emoji for a slot category.
func slotEmoji(slot EquipmentSlot) string {
	switch slot {
	case SlotWeapon:
		return "⚔️"
	case SlotArmor:
		return "🛡️"
	case SlotHelmet:
		return "🪖"
	case SlotBoots:
		return "👢"
	case SlotTool:
		return "⛏️"
	default:
		return "📦"
	}
}

// slotTitle returns a capitalized display name for a slot.
func slotTitle(slot EquipmentSlot) string {
	s := string(slot)
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// advShopOverview shows a compact category menu with current equipment and next upgrade.
func advShopOverview(equip map[EquipmentSlot]*AdvEquipment, balance float64) string {
	var sb strings.Builder
	sb.WriteString("🛒 **Equipment Shop**\n")
	sb.WriteString(fmt.Sprintf("💰 Balance: €%.0f\n\n", balance))

	for _, slot := range allSlots {
		current := equip[slot]
		currentTier := 0
		currentName := "None"
		if current != nil {
			currentTier = current.Tier
			currentName = current.Name
		}

		emoji := slotEmoji(slot)
		title := slotTitle(slot)

		// Find next upgrade.
		defs := equipmentTiers[slot]
		nextUpgrade := ""
		for _, def := range defs {
			if def.Tier > currentTier && def.Price > 0 {
				nextUpgrade = fmt.Sprintf("Next: %s — €%.0f", def.Name, def.Price)
				break
			}
		}

		sb.WriteString(fmt.Sprintf("%s **%s** — %s (T%d)\n", emoji, title, currentName, currentTier))
		if nextUpgrade != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", nextUpgrade))
		} else {
			sb.WriteString("    ✨ Maxed out!\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Browse a category: `!adventure shop <category>`\n")
	sb.WriteString("Categories: `weapon` · `armor` · `helmet` · `boots` · `tool`")
	return sb.String()
}

var shopCategoryAliases = map[string]EquipmentSlot{
	"weapon": SlotWeapon, "weapons": SlotWeapon, "sword": SlotWeapon, "swords": SlotWeapon,
	"armor": SlotArmor, "armour": SlotArmor,
	"helmet": SlotHelmet, "helm": SlotHelmet, "helmets": SlotHelmet,
	"boots": SlotBoots, "boot": SlotBoots,
	"tool": SlotTool, "tools": SlotTool, "pickaxe": SlotTool,
}

// advParseShopCategory maps user input to an EquipmentSlot.
func advParseShopCategory(input string) EquipmentSlot {
	return shopCategoryAliases[strings.ToLower(strings.TrimSpace(input))]
}

// advShopCategory shows detailed listings for a single equipment slot.
func advShopCategory(slot EquipmentSlot, equip map[EquipmentSlot]*AdvEquipment, balance float64) string {
	var sb strings.Builder

	current := equip[slot]
	currentTier := 0
	currentName := "None"
	if current != nil {
		currentTier = current.Tier
		currentName = current.Name
	}

	emoji := slotEmoji(slot)
	title := slotTitle(slot)

	sb.WriteString(fmt.Sprintf("%s **%s Shop**\n", emoji, title))
	sb.WriteString(fmt.Sprintf("💰 Balance: €%.0f\n", balance))
	sb.WriteString(fmt.Sprintf("Equipped: **%s** (Tier %d)\n\n", currentName, currentTier))

	defs := equipmentTiers[slot]
	hasUpgrades := false
	for _, def := range defs {
		if def.Tier <= currentTier || def.Price == 0 {
			continue
		}
		hasUpgrades = true

		priceTag := fmt.Sprintf("€%.0f", def.Price)
		if balance < def.Price {
			priceTag += "  💸 can't afford"
		} else {
			priceTag += "  ✅ affordable"
		}

		sb.WriteString(fmt.Sprintf("**Tier %d: %s** — %s\n", def.Tier, def.Name, priceTag))
		sb.WriteString(fmt.Sprintf("%s\n\n", def.Description))
	}

	if !hasUpgrades {
		sb.WriteString("✨ You've reached max tier! Nothing left to buy here.\n\n")
	}

	sb.WriteString("To buy: `!adventure buy <item name>` or `!adventure buy <tier> <category>`\n")
	sb.WriteString("Example: `!adventure buy Enchanted Blade` or `!adventure buy 4 sword`\n")
	sb.WriteString("Back to overview: `!adventure shop`")
	return sb.String()
}

// ── Find Shop Item ───────────────────────────────────────────────────────────

// normalizeQuotes replaces common Unicode quotes/apostrophes with ASCII equivalents.
var quoteReplacer = strings.NewReplacer(
	"\u2018", "'", "\u2019", "'", // left/right single curly quotes
	"\u201C", "\"", "\u201D", "\"", // left/right double curly quotes
	"\u2032", "'", // prime
)

func normalizeQuotes(s string) string {
	return quoteReplacer.Replace(s)
}

func advFindShopItem(name string) (EquipmentSlot, *EquipmentDef, bool) {
	name = normalizeQuotes(strings.TrimSpace(name))

	// Support tier+category shorthand: "3 sword", "tier 3 weapon", "t3 boots", etc.
	if slot, def, ok := advFindByTierShorthand(name); ok {
		return slot, def, true
	}

	for _, slot := range allSlots {
		for i := range equipmentTiers[slot] {
			def := &equipmentTiers[slot][i]
			if def.Price == 0 {
				continue // can't buy tier 0
			}
			if strings.EqualFold(def.Name, name) || containsFold(normalizeQuotes(def.Name), name) {
				return slot, def, true
			}
		}
	}
	return "", nil, false
}

// advFindByTierShorthand matches patterns like "3 sword", "tier 3 weapon", "t3 boots".
func advFindByTierShorthand(input string) (EquipmentSlot, *EquipmentDef, bool) {
	lower := strings.ToLower(input)

	// Strip optional "tier " or "t" prefix.
	lower = strings.TrimPrefix(lower, "tier ")
	lower = strings.TrimPrefix(lower, "t")

	// Expect "<number> <category>" or "<number><category>".
	parts := strings.SplitN(strings.TrimSpace(lower), " ", 2)
	if len(parts) < 2 {
		return "", nil, false
	}

	tierStr := strings.TrimSpace(parts[0])
	category := strings.TrimSpace(parts[1])

	tier := 0
	for _, c := range tierStr {
		if c < '0' || c > '9' {
			return "", nil, false
		}
		tier = tier*10 + int(c-'0')
	}

	slot := advParseShopCategory(category)
	if slot == "" {
		return "", nil, false
	}

	defs := equipmentTiers[slot]
	for i := range defs {
		if defs[i].Tier == tier && defs[i].Price > 0 {
			return slot, &defs[i], true
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
