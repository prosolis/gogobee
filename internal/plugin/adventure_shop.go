package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

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

// ── Pending Interaction Types ───────────────────────────────────────────────

type advPendingShopCategory struct {
	ShowAll bool
}

type advPendingShopItem struct {
	Slot    EquipmentSlot
	ShowAll bool
}

type advPendingShopConfirm struct {
	Slot EquipmentSlot
	Tier int
}

// ── Session Tracking ────────────────────────────────────────────────────────

type advShopSession struct {
	StartedAt   time.Time
	ItemsBought int
}

func (p *AdventurePlugin) shopSessionGet(userID id.UserID) *advShopSession {
	if val, ok := p.shopSessions.Load(string(userID)); ok {
		return val.(*advShopSession)
	}
	return nil
}

func (p *AdventurePlugin) shopSessionStart(userID id.UserID) {
	if p.shopSessionGet(userID) == nil {
		p.shopSessions.Store(string(userID), &advShopSession{
			StartedAt: time.Now(),
		})
	}
}

func (p *AdventurePlugin) shopSessionBump(userID id.UserID) {
	sess := p.shopSessionGet(userID)
	if sess != nil {
		sess.ItemsBought++
	}
}

func (p *AdventurePlugin) shopSessionEnd(userID id.UserID) {
	p.shopSessions.Delete(string(userID))
}

// ── Browse Timeout ──────────────────────────────────────────────────────────

// shopNudgeGen tracks a generation counter per user so only the latest nudge fires.
// Key: userID string, Value: *int64 (atomic counter)
func (p *AdventurePlugin) shopScheduleBrowseNudge(userID id.UserID) {
	// Bump generation so any prior goroutine becomes stale.
	var gen int64
	if val, ok := p.shopSessions.Load(string(userID) + ":nudge_gen"); ok {
		gen = val.(int64) + 1
	}
	p.shopSessions.Store(string(userID)+":nudge_gen", gen)

	capturedGen := gen
	go func() {
		time.Sleep(2 * time.Minute)
		// Check if this goroutine is still the latest.
		if val, ok := p.shopSessions.Load(string(userID) + ":nudge_gen"); !ok || val.(int64) != capturedGen {
			return
		}
		val, ok := p.pending.Load(string(userID))
		if !ok {
			return
		}
		pi, ok := val.(*advPendingInteraction)
		if !ok || !strings.HasPrefix(pi.Type, "shop_") {
			return
		}
		flavor, _ := advPickFlavor(luigiBrowseTimeout, userID, "luigi_browse")
		p.SendDM(userID, fmt.Sprintf("*%s*", flavor))
	}()
}

// ── Display: Luigi Greeting + Category Menu ─────────────────────────────────

func luigiShopGreeting(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment, balance float64, showAll bool) string {
	var sb strings.Builder

	// Check if fully maxed out.
	allMaxed := true
	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil || eq.Tier < 5 {
			allMaxed = false
			break
		}
	}

	if allMaxed {
		flavor, _ := advPickFlavor(luigiMaxedOut, userID, "luigi_maxed")
		sb.WriteString(fmt.Sprintf("🛒 **Luigi's**\n\n*%s*", flavor))
		return sb.String()
	}

	greet, _ := advPickFlavor(luigiGreetings, userID, "luigi_greet")
	sb.WriteString(fmt.Sprintf("🛒 **Luigi's**\n💰 Balance: €%.0f\n\n", balance))
	sb.WriteString(fmt.Sprintf("*%s*\n\n", greet))

	sb.WriteString("⚔️  Weapons       🛡️  Armor\n")
	sb.WriteString("🪖  Helmets       👢  Boots\n")
	sb.WriteString("⛏️  Tools\n\n")

	if showAll {
		flavor, _ := advPickFlavor(luigiShowAllComment, userID, "luigi_showall")
		sb.WriteString(fmt.Sprintf("*%s*\n\n", flavor))
	}

	sb.WriteString("Reply with a category name to browse.")
	return sb.String()
}

// ── Display: Category Item List ─────────────────────────────────────────────

func luigiCategoryView(userID id.UserID, slot EquipmentSlot, equip map[EquipmentSlot]*AdvEquipment, balance float64, showAll bool) string {
	var sb strings.Builder

	current := equip[slot]
	currentTier := 0
	currentName := "None"
	isMWOrArena := false
	if current != nil {
		currentTier = current.Tier
		currentName = current.Name
		isMWOrArena = current.Masterwork || current.ArenaTier > 0
	}

	emoji := slotEmoji(slot)
	title := strings.ToUpper(string(slot))

	// Category intro
	if intros, ok := luigiCategoryIntros[slot]; ok && len(intros) > 0 {
		intro, _ := advPickFlavor(intros, userID, "luigi_cat_"+string(slot))
		sb.WriteString(fmt.Sprintf("*%s*\n\n", intro))
	}

	sb.WriteString(fmt.Sprintf("%s **%s** — Your current: %s (T%d)", emoji, title, currentName, currentTier))
	if isMWOrArena {
		sb.WriteString(" ⭐")
	}
	sb.WriteString("\n\n")

	if isMWOrArena {
		ack, _ := advPickFlavor(luigiMasterworkAck, userID, "luigi_mw_ack")
		sb.WriteString(fmt.Sprintf("*%s*\n\n", ack))
	}

	defs := equipmentTiers[slot]
	hasItems := false

	// Show current equipped item first.
	if current != nil {
		for _, def := range defs {
			if def.Tier == currentTier {
				sb.WriteString(fmt.Sprintf("🟢 %-28s T%d   €%.0f    Currently equipped\n",
					def.Name, def.Tier, def.Price))
				break
			}
		}
	}

	for _, def := range defs {
		if def.Price == 0 {
			continue // skip tier 0
		}
		if def.Tier == currentTier {
			continue // already shown as 🟢
		}
		if !showAll && def.Tier < currentTier {
			continue // skip downgrades unless show all
		}

		hasItems = true

		// Upgrade indicator
		var indicator string
		switch {
		case def.Tier > currentTier:
			indicator = "⬆️"
		case def.Tier == currentTier:
			indicator = "➡️"
		default:
			indicator = "⬇️"
		}

		// Affordability
		afford := "✅"
		if balance < def.Price {
			afford = "💸"
		}

		// One-liner
		oneLiner := ""
		if line, ok := luigiItemOneLiners[luigiItemKey{slot, def.Tier}]; ok {
			oneLiner = fmt.Sprintf("  \"%s\"", line)
		}

		sb.WriteString(fmt.Sprintf("%s %-28s T%d   €%-8.0f %s%s\n",
			indicator, def.Name, def.Tier, def.Price, afford, oneLiner))
	}

	// Occasional unprompted Luigi commentary (~30% chance).
	if hasItems && rand.IntN(100) < 30 {
		comment, _ := advPickFlavor(luigiCommentary, userID, "luigi_comment")
		sb.WriteString(fmt.Sprintf("\n*%s*\n", comment))
	}

	if !hasItems {
		sb.WriteString("✨ You've reached max tier! Nothing left to buy here.\n")
	}

	if !showAll && currentTier > 1 {
		sb.WriteString(fmt.Sprintf("\nTiers below T%d omitted. Use `!adventure shop show all` to see everything.\n", currentTier))
	}

	sb.WriteString("\nReply with an item name to buy, or \"back\" to return to categories.")
	return sb.String()
}

// ── Display: Item Confirm ───────────────────────────────────────────────────

func luigiItemConfirm(userID id.UserID, slot EquipmentSlot, def *EquipmentDef, current *AdvEquipment, balance float64) string {
	var sb strings.Builder

	currentTier := 0
	currentName := "None"
	if current != nil {
		currentTier = current.Tier
		currentName = current.Name
	}

	// Upgrade indicator
	indicator := "⬆️"
	if def.Tier == currentTier {
		indicator = "➡️"
	} else if def.Tier < currentTier {
		indicator = "⬇️"
	}

	sb.WriteString(fmt.Sprintf("%s **%s** — T%d %s — €%.0f\n\n", indicator, def.Name, def.Tier, slotTitle(slot), def.Price))

	if def.Tier > currentTier {
		sb.WriteString(fmt.Sprintf("An upgrade from your %s (T%d).\n", currentName, currentTier))
	} else if def.Tier == currentTier {
		sb.WriteString(fmt.Sprintf("Same tier as your current %s.\n", currentName))
	} else {
		sb.WriteString(fmt.Sprintf("A downgrade from your %s (T%d).\n", currentName, currentTier))
	}

	// Full Luigi description
	if desc, ok := luigiItemDescriptions[luigiItemKey{slot, def.Tier}]; ok {
		sb.WriteString(fmt.Sprintf("\n\"%s\"\n", desc))
	}

	afterBalance := balance - def.Price
	sb.WriteString(fmt.Sprintf("\nYour balance: €%.0f  →  €%.0f after purchase\n\n", balance, afterBalance))
	sb.WriteString("Confirm? (yes / no)")

	return sb.String()
}

// ── Resolver: Category Choice ───────────────────────────────────────────────

func (p *AdventurePlugin) resolveShopCategoryChoice(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingShopCategory)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply == "back" || reply == "exit" || reply == "cancel" {
		p.shopSessionEnd(ctx.Sender)
		flavor, _ := advPickFlavor(luigiCancellation, ctx.Sender, "luigi_cancel")
		return p.SendDM(ctx.Sender, fmt.Sprintf("*%s*", flavor))
	}

	slot := advParseShopCategory(reply)
	if slot == "" {
		// Re-store pending and reprompt.
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "I didn't catch that. Reply with a category: weapons, armor, helmets, boots, or tools.")
	}

	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	balance := p.euro.GetBalance(ctx.Sender)

	text := luigiCategoryView(ctx.Sender, slot, equip, balance, data.ShowAll)
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "shop_item",
		Data:      &advPendingShopItem{Slot: slot, ShowAll: data.ShowAll},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	p.advMarkMenuSent(ctx.Sender)
	p.shopScheduleBrowseNudge(ctx.Sender)

	return p.SendDM(ctx.Sender, text)
}

// ── Resolver: Item Choice ───────────────────────────────────────────────────

func (p *AdventurePlugin) resolveShopItemChoice(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingShopItem)
	reply := strings.TrimSpace(ctx.Body)
	lower := strings.ToLower(reply)

	if lower == "back" {
		// Return to category menu.
		_, equip, err := p.ensureCharacter(ctx.Sender)
		if err != nil {
			return p.SendDM(ctx.Sender, "Failed to load your character.")
		}
		balance := p.euro.GetBalance(ctx.Sender)

		text := luigiShopGreeting(ctx.Sender, equip, balance, data.ShowAll)
		p.pending.Store(string(ctx.Sender), &advPendingInteraction{
			Type:      "shop_category",
			Data:      &advPendingShopCategory{ShowAll: data.ShowAll},
			ExpiresAt: time.Now().Add(advDMResponseWindow),
		})
		p.advMarkMenuSent(ctx.Sender)
		return p.SendDM(ctx.Sender, text)
	}

	if lower == "exit" || lower == "cancel" {
		p.shopSessionEnd(ctx.Sender)
		flavor, _ := advPickFlavor(luigiCancellation, ctx.Sender, "luigi_cancel")
		return p.SendDM(ctx.Sender, fmt.Sprintf("*%s*", flavor))
	}

	// Try to find the item scoped to the current slot first, then globally.
	slot, def, found := advFindShopItemInSlot(reply, data.Slot)
	if !found {
		slot, def, found = advFindShopItem(reply)
	}
	if !found {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, fmt.Sprintf("I don't have anything matching \"%s\". Reply with an item name from the list, or \"back\" to return.", reply))
	}

	// Load fresh data.
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	balance := p.euro.GetBalance(ctx.Sender)

	current := equip[slot]

	// Check Arena/Masterwork block — only block if shop item isn't clearly better.
	// Arena gear always blocks (1.5x multiplier, earned through combat).
	// Masterwork blocks only if the shop item's tier doesn't exceed the MW effective tier.
	if current != nil && current.ArenaTier > 0 {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, fmt.Sprintf("You have **%s** (Arena gear) in that slot. A shop item can't replace that. You earned it.\n\nPick something else, or \"back\" to return.",
			current.Name))
	}
	if current != nil && current.Masterwork && float64(def.Tier) <= advEffectiveTier(current) {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, fmt.Sprintf("You have **%s** (Masterwork ⭐) in that slot. That T%d shop item isn't an upgrade over your T%d Masterwork.\n\nPick something else, or \"back\" to return.",
			current.Name, def.Tier, current.Tier))
	}

	text := luigiItemConfirm(ctx.Sender, slot, def, current, balance)
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "shop_confirm",
		Data:      &advPendingShopConfirm{Slot: slot, Tier: def.Tier},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	p.advMarkMenuSent(ctx.Sender)
	p.shopScheduleBrowseNudge(ctx.Sender)

	return p.SendDM(ctx.Sender, text)
}

// ── Resolver: Purchase Confirm ──────────────────────────────────────────────

func (p *AdventurePlugin) resolveShopConfirm(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	data := interaction.Data.(*advPendingShopConfirm)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply != "yes" && reply != "y" && reply != "confirm" {
		p.shopSessionEnd(ctx.Sender)
		flavor, _ := advPickFlavor(luigiCancellation, ctx.Sender, "luigi_cancel")
		return p.SendDM(ctx.Sender, fmt.Sprintf("*%s*", flavor))
	}

	// Reload fresh equipment and balance (TOCTOU protection).
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	balance := p.euro.GetBalance(ctx.Sender)

	// Find the def fresh.
	defs := equipmentTiers[data.Slot]
	var def *EquipmentDef
	for i := range defs {
		if defs[i].Tier == data.Tier {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		return p.SendDM(ctx.Sender, "Something went wrong. Item not found.")
	}

	current := equip[data.Slot]

	// Re-check Arena/Masterwork block.
	if current != nil && current.ArenaTier > 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Your **%s** is Arena gear. Luigi won't overwrite it.", current.Name))
	}
	if current != nil && current.Masterwork && float64(def.Tier) <= advEffectiveTier(current) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Your **%s** (Masterwork ⭐) is better than that T%d shop item.", current.Name, def.Tier))
	}

	// Affordability.
	if balance < def.Price {
		flavor, _ := advPickFlavor(luigiInsufficientFunds, ctx.Sender, "luigi_broke")
		return p.SendDM(ctx.Sender, fmt.Sprintf("*%s*\n\nYou have €%.0f. %s costs €%.0f.", flavor, balance, def.Name, def.Price))
	}

	// Debit.
	if !p.euro.Debit(ctx.Sender, def.Price, "adventure_shop_"+string(data.Slot)) {
		return p.SendDM(ctx.Sender, "Transaction failed. The economy is having a moment.")
	}

	// Move old gear to inventory before replacing.
	if current != nil && current.Tier > 0 {
		itemType := "ShopGear"
		var resaleValue int64
		if current.Masterwork {
			itemType = "MasterworkGear"
			// Masterwork has no resale value (it's special, not shop-priced)
		} else if current.Tier < len(equipmentTiers[data.Slot]) {
			resaleValue = int64(equipmentTiers[data.Slot][current.Tier].Price * 0.3)
		}
		err := addAdvInventoryItem(ctx.Sender, AdvItem{
			Name:        current.Name,
			Type:        itemType,
			Tier:        current.Tier,
			Value:       resaleValue,
			Slot:        data.Slot,
			SkillSource: current.SkillSource,
		})
		if err != nil {
			slog.Error("shop: failed to move old gear to inventory", "user", ctx.Sender, "slot", data.Slot, "err", err)
		}
	}

	// Save new equipment.
	eq := &AdvEquipment{
		Slot:        data.Slot,
		Tier:        def.Tier,
		Condition:   100,
		Name:        def.Name,
		ActionsUsed: 0,
	}
	if err := saveAdvEquipment(ctx.Sender, eq); err != nil {
		// Refund on DB error.
		p.euro.Credit(ctx.Sender, def.Price, "adventure_shop_refund")
		return p.SendDM(ctx.Sender, "Something went wrong saving your equipment. You've been refunded.")
	}

	// Build confirmation message.
	p.shopSessionBump(ctx.Sender)

	var confirmText string
	sess := p.shopSessionGet(ctx.Sender)
	isCombo := sess != nil && sess.ItemsBought >= 3

	if isCombo {
		flavor, _ := advPickFlavor(luigiComboConfirm, ctx.Sender, "luigi_combo")
		confirmText = fmt.Sprintf(flavor, int(def.Price), def.Name)
	} else if def.Tier == 5 {
		flavor, _ := advPickFlavor(luigiTier5Confirm, ctx.Sender, "luigi_t5")
		confirmText = fmt.Sprintf(flavor, int(def.Price), def.Name)
	} else {
		flavor, _ := advPickFlavor(luigiPurchaseConfirm, ctx.Sender, "luigi_buy")
		confirmText = fmt.Sprintf(flavor, def.Name, int(def.Price))
	}

	var sb strings.Builder
	sb.WriteString(confirmText)
	sb.WriteString(fmt.Sprintf("\n\n✅ **%s** equipped.", def.Name))
	if current != nil && current.Tier > 0 {
		sb.WriteString(fmt.Sprintf(" %s moved to inventory.", current.Name))
	}
	sb.WriteString(fmt.Sprintf("\nBalance: €%.0f", balance-def.Price))

	return p.SendDM(ctx.Sender, sb.String())
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

// advFindShopItemInSlot finds an item scoped to a specific slot.
func advFindShopItemInSlot(name string, slot EquipmentSlot) (EquipmentSlot, *EquipmentDef, bool) {
	name = normalizeQuotes(strings.TrimSpace(name))

	for i := range equipmentTiers[slot] {
		def := &equipmentTiers[slot][i]
		if def.Price == 0 {
			continue
		}
		if strings.EqualFold(def.Name, name) || containsFold(normalizeQuotes(def.Name), name) {
			return slot, def, true
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

// ── Buy Equipment (legacy !adventure buy path) ──────────────────────────────

func (p *AdventurePlugin) advBuyEquipment(userID id.UserID, slot EquipmentSlot, def *EquipmentDef, equip map[EquipmentSlot]*AdvEquipment) string {
	current := equip[slot]
	if current != nil && current.Tier >= def.Tier {
		return fmt.Sprintf("You already have %s (Tier %d). %s is Tier %d. That's not an upgrade, that's a lateral move at best.",
			current.Name, current.Tier, def.Name, def.Tier)
	}
	// Block shop purchases that would overwrite arena gear (always) or
	// masterwork gear (only if shop item isn't clearly better).
	if current != nil && current.ArenaTier > 0 {
		return fmt.Sprintf("You already have **%s** (Arena gear). A shop item cannot replace this. "+
			"You earned that. Don't throw it away.",
			current.Name)
	}
	if current != nil && current.Masterwork && float64(def.Tier) <= advEffectiveTier(current) {
		return fmt.Sprintf("You already have **%s** (Masterwork ⭐ T%d). That T%d shop item isn't an upgrade.",
			current.Name, current.Tier, def.Tier)
	}

	balance := p.euro.GetBalance(userID)
	if balance < def.Price {
		flavor, _ := advPickFlavor(luigiInsufficientFunds, userID, "luigi_broke")
		return fmt.Sprintf("*%s*\n\nYou have €%.0f. %s costs €%.0f.", flavor, balance, def.Name, def.Price)
	}

	if !p.euro.Debit(userID, def.Price, "adventure_shop_"+string(slot)) {
		return "Transaction failed. The economy is having a moment."
	}

	// Move old gear to inventory.
	if current != nil && current.Tier > 0 {
		itemType := "ShopGear"
		var resaleValue int64
		if current.Masterwork {
			itemType = "MasterworkGear"
		} else if current.Tier < len(equipmentTiers[slot]) {
			resaleValue = int64(equipmentTiers[slot][current.Tier].Price * 0.3)
		}
		err := addAdvInventoryItem(userID, AdvItem{
			Name:        current.Name,
			Type:        itemType,
			Tier:        current.Tier,
			Value:       resaleValue,
			Slot:        slot,
			SkillSource: current.SkillSource,
		})
		if err != nil {
			slog.Error("shop: failed to move old gear to inventory", "user", userID, "slot", slot, "err", err)
		}
	}

	// Update equipment
	eq := &AdvEquipment{
		Slot:        slot,
		Tier:        def.Tier,
		Condition:   100,
		Name:        def.Name,
		ActionsUsed: 0,
	}
	if err := saveAdvEquipment(userID, eq); err != nil {
		// Refund on DB error
		p.euro.Credit(userID, def.Price, "adventure_shop_refund")
		return "Something went wrong saving your equipment. You've been refunded."
	}
	equip[slot] = eq

	var sb strings.Builder
	if def.Tier == 5 {
		flavor, _ := advPickFlavor(luigiTier5Confirm, userID, "luigi_t5")
		sb.WriteString(fmt.Sprintf(flavor, int(def.Price), def.Name))
	} else {
		flavor, _ := advPickFlavor(luigiPurchaseConfirm, userID, "luigi_buy")
		sb.WriteString(fmt.Sprintf(flavor, def.Name, int(def.Price)))
	}
	sb.WriteString(fmt.Sprintf("\n\n✅ **%s** equipped.", def.Name))
	if current != nil && current.Tier > 0 {
		sb.WriteString(fmt.Sprintf(" %s moved to inventory.", current.Name))
	}

	return sb.String()
}

// ── Sell Items ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) advSellAll(userID id.UserID) string {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return "Failed to access your inventory. Try again."
	}
	if len(items) == 0 {
		return "Your inventory is empty. There is nothing to sell. This is a metaphor for something but now is not the time."
	}

	var total float64
	var sold int
	var keptSpecial int
	for _, item := range items {
		if item.Type == "MasterworkGear" || item.Type == "ArenaGear" || item.Type == "card" {
			keptSpecial++
			continue
		}
		if err := removeAdvInventoryItem(item.ID); err != nil {
			continue
		}
		total += float64(item.Value)
		sold++
	}

	if sold == 0 {
		if keptSpecial > 0 {
			return "Your inventory only contains Masterwork and Arena gear. The merchant doesn't deal in that. Use `!adventure equip` instead."
		}
		return "Your inventory is empty. There is nothing to sell. This is a metaphor for something but now is not the time."
	}

	p.euro.Credit(userID, total, "adventure_sell_all")

	msg := fmt.Sprintf("Sold %d items for **€%.0f**.\n\nThe merchant took everything without comment. "+
		"This is the most respect anyone has shown your loot collection. Take the money.",
		sold, total)
	if keptSpecial > 0 {
		msg += fmt.Sprintf("\n\n(%d special gear items kept — the merchant knows better than to touch those.)", keptSpecial)
	}
	return msg
}

func (p *AdventurePlugin) advSellItem(userID id.UserID, itemName string) string {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return "Failed to access your inventory."
	}

	// Fuzzy match
	for _, item := range items {
		if containsFold(item.Name, itemName) {
			if item.Type == "MasterworkGear" || item.Type == "ArenaGear" || item.Type == "card" {
				return fmt.Sprintf("**%s** is special gear. The merchant won't touch it. Use `!adventure equip` to equip it.", item.Name)
			}
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
		if item.Type == "MasterworkGear" {
			sb.WriteString(fmt.Sprintf("  ⭐ %s (T%d Masterwork %s)\n", item.Name, item.Tier, slotTitle(item.Slot)))
		} else if item.Type == "ArenaGear" {
			sb.WriteString(fmt.Sprintf("  ⚔️ %s (T%d Arena %s)\n", item.Name, item.Tier, slotTitle(item.Slot)))
		} else if item.Type == "card" {
			sb.WriteString(fmt.Sprintf("  🃏 %s\n", item.Name))
		} else {
			sb.WriteString(fmt.Sprintf("  %s (T%d %s) — €%d\n", item.Name, item.Tier, item.Type, item.Value))
			total += item.Value
		}
	}
	sb.WriteString(fmt.Sprintf("\n%d items — sellable value ~€%d", len(items), total))
	sb.WriteString("\n\nTo sell: `!adventure sell all` or `!adventure sell <item>`")
	sb.WriteString("\nTo equip Masterwork gear: `!adventure equip`")
	return sb.String()
}
