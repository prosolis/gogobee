package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Pricing ─────────────────────────────────────────────────────────────────

var blacksmithBaseRates = [6]int{1, 3, 8, 20, 55, 150}

func blacksmithRepairCost(eq *AdvEquipment) int {
	if eq == nil || eq.Condition >= 100 {
		return 0
	}
	// Masterwork and Arena items use the next tier's base rate.
	tier := eq.Tier
	if eq.Masterwork || eq.ArenaTier > 0 {
		tier++
		if tier > 5 {
			tier = 5
		}
	}
	if tier < 0 {
		tier = 0
	}
	if tier > 5 {
		tier = 5
	}
	baseRate := float64(blacksmithBaseRates[tier])
	damage := float64(100 - eq.Condition)
	condMult := 1.0 + damage/100.0
	costPerPoint := baseRate * condMult
	return int(math.Ceil(costPerPoint * damage))
}

// ── Pending Interaction Types ───────────────────────────────────────────────

type advPendingBlacksmithSlot struct{}

type advPendingBlacksmithConfirm struct {
	Slots []EquipmentSlot
	Costs map[EquipmentSlot]int
	Total int
}

// ── Command Handlers ────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleBlacksmithCmd(ctx MessageContext) error {
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	text := renderBlacksmithShop(equip)

	// Store pending interaction for slot selection.
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "blacksmith_slot",
		Data:      &advPendingBlacksmithSlot{},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	p.advMarkMenuSent(ctx.Sender)

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleRepairAllCmd(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	return p.buildRepairAllConfirm(ctx.Sender, equip)
}

func (p *AdventurePlugin) handleRepairSlotCmd(ctx MessageContext, slotName string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	return p.buildRepairSlotConfirm(ctx.Sender, equip, slotName)
}

// ── Slot Selection Resolution ───────────────────────────────────────────────

func (p *AdventurePlugin) resolveBlacksmithSlotChoice(ctx MessageContext, interaction *advPendingInteraction) error {
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply != "all" && parseSlotName(reply) == "" {
		// Re-store pending and reprompt.
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Reply with a slot name (weapon, armor, helmet, boots, tool) or \"all\".")
	}

	// Lock and reload fresh equipment (the cached data may be stale).
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	if reply == "all" {
		return p.buildRepairAllConfirm(ctx.Sender, equip)
	}

	return p.buildRepairSlotConfirm(ctx.Sender, equip, reply)
}

// ── Confirm Resolution ──────────────────────────────────────────────────────

func (p *AdventurePlugin) resolveBlacksmithConfirm(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	data := interaction.Data.(*advPendingBlacksmithConfirm)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))

	if reply == "yes" || reply == "y" || reply == "confirm" {
		return p.executeRepair(ctx.Sender, data)
	}

	return p.SendDM(ctx.Sender, "Repair cancelled. The blacksmith watches you leave. They'll be here when you're ready.")
}

// ── Builders ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) buildRepairAllConfirm(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment) error {
	var damaged []EquipmentSlot
	costs := make(map[EquipmentSlot]int)
	total := 0

	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil {
			continue
		}
		cost := blacksmithRepairCost(eq)
		if cost > 0 {
			damaged = append(damaged, slot)
			costs[slot] = cost
			total += cost
		}
	}

	if len(damaged) == 0 {
		return p.SendDM(userID, "⚒️ All your equipment is at full condition. "+pickBlacksmithFlavor(blacksmithFullCondition))
	}

	balance := p.euro.GetBalance(userID)
	if balance < float64(total) {
		// Can't afford everything — suggest cheapest slot.
		cheapestSlot := damaged[0]
		cheapestCost := costs[damaged[0]]
		for _, s := range damaged[1:] {
			if costs[s] < cheapestCost {
				cheapestSlot = s
				cheapestCost = costs[s]
			}
		}
		return p.SendDM(userID, fmt.Sprintf("⚒️ Repairing everything costs €%d. You have €%.0f.\n\n"+
			"The blacksmith suggests starting with your %s (€%d). They say this with some reluctance — they wanted to do everything.\n\n"+
			"Use `!adventure repair %s` to repair just that slot.",
			total, balance, slotTitle(cheapestSlot), cheapestCost, string(cheapestSlot)))
	}

	// Build breakdown.
	var sb strings.Builder
	sb.WriteString("⚒️ **Repair All — Confirmation**\n\n")
	for _, slot := range damaged {
		eq := equip[slot]
		sb.WriteString(fmt.Sprintf("  %s %s: [%d/100] → [100/100]  €%d\n",
			slotEmoji(slot), slotTitle(slot), eq.Condition, costs[slot]))
	}
	sb.WriteString(fmt.Sprintf("\n**Total: €%d**\n\nReply **yes** to confirm or **no** to cancel.", total))

	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "blacksmith_confirm",
		Data:      &advPendingBlacksmithConfirm{Slots: damaged, Costs: costs, Total: total},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})

	return p.SendDM(userID, sb.String())
}

func (p *AdventurePlugin) buildRepairSlotConfirm(userID id.UserID, equip map[EquipmentSlot]*AdvEquipment, slotName string) error {
	slot := parseSlotName(slotName)
	if slot == "" {
		return p.SendDM(userID, "Unknown slot. Valid slots: weapon, armor, helmet, boots, tool.")
	}

	eq := equip[slot]
	if eq == nil {
		return p.SendDM(userID, fmt.Sprintf("You don't have any %s equipped.", slotTitle(slot)))
	}

	if eq.Condition >= 100 {
		return p.SendDM(userID, fmt.Sprintf("⚒️ %s %s — %s", slotEmoji(slot), eq.Name, pickBlacksmithFlavor(blacksmithFullCondition)))
	}

	cost := blacksmithRepairCost(eq)
	balance := p.euro.GetBalance(userID)
	if balance < float64(cost) {
		return p.SendDM(userID, fmt.Sprintf("⚒️ Repairing your %s costs €%d. You have €%.0f. The blacksmith waits.",
			slotTitle(slot), cost, balance))
	}

	// Build inspection message.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚒️ %s %s — **%s** [%d/100]\n\n",
		slotEmoji(slot), slotTitle(slot), eq.Name, eq.Condition))

	// Special flavor for masterwork, arena, or broken.
	if eq.Condition == 0 {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", pickBlacksmithFlavor(blacksmithBrokenCondition)))
	} else if eq.Masterwork {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", pickBlacksmithFlavor(blacksmithMasterwork)))
	} else if eq.ArenaTier > 0 {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", pickBlacksmithFlavor(blacksmithArena)))
	} else {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", pickBlacksmithFlavor(blacksmithInspection)))
	}

	sb.WriteString(fmt.Sprintf("Repair to 100: **€%d**\n\nReply **yes** to confirm or **no** to cancel.", cost))

	costs := map[EquipmentSlot]int{slot: cost}
	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "blacksmith_confirm",
		Data:      &advPendingBlacksmithConfirm{Slots: []EquipmentSlot{slot}, Costs: costs, Total: cost},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})

	return p.SendDM(userID, sb.String())
}

// ── Repair Execution ────────────────────────────────────────────────────────

func (p *AdventurePlugin) executeRepair(userID id.UserID, data *advPendingBlacksmithConfirm) error {
	// Reload equipment fresh and recompute costs to avoid stale-price TOCTOU.
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return p.SendDM(userID, "Something went wrong loading your equipment.")
	}

	// Recompute actual costs from current condition.
	actualTotal := 0
	actualCosts := make(map[EquipmentSlot]int)
	for _, slot := range data.Slots {
		eq := equip[slot]
		if eq == nil {
			continue
		}
		cost := blacksmithRepairCost(eq)
		if cost > 0 {
			actualCosts[slot] = cost
			actualTotal += cost
		}
	}

	if actualTotal == 0 {
		return p.SendDM(userID, "⚒️ Your equipment is already at full condition. No repair needed.")
	}

	// Cap at the quoted price so the user never pays more than they agreed to.
	if actualTotal > data.Total {
		actualTotal = data.Total
	}

	if !p.euro.Debit(userID, float64(actualTotal), "blacksmith_repair") {
		return p.SendDM(userID, "Payment failed. You don't have enough gold.")
	}

	var repaired []string
	repairedCost := 0
	for _, slot := range data.Slots {
		eq := equip[slot]
		if eq == nil || eq.Condition >= 100 {
			continue
		}
		eq.Condition = 100
		if err := saveAdvEquipment(userID, eq); err != nil {
			slog.Error("blacksmith: failed to save repaired equipment", "user", userID, "slot", slot, "err", err)
			continue
		}
		repaired = append(repaired, fmt.Sprintf("%s %s", slotEmoji(slot), eq.Name))
		repairedCost += actualCosts[slot]
	}

	if len(repaired) == 0 {
		p.euro.Credit(userID, float64(actualTotal), "blacksmith_refund")
		return p.SendDM(userID, "Nothing was repaired. Gold refunded.")
	}

	// Partial refund if some slots failed to save.
	if repairedCost < actualTotal {
		refund := actualTotal - repairedCost
		p.euro.Credit(userID, float64(refund), "blacksmith_partial_refund")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚒️ **Repair Complete** — €%d\n\n", repairedCost))
	for _, item := range repaired {
		sb.WriteString(fmt.Sprintf("  %s → [100/100]\n", item))
	}
	sb.WriteString(fmt.Sprintf("\n*%s*\n\n", pickBlacksmithFlavor(blacksmithPayment)))
	sb.WriteString(fmt.Sprintf("*%s*", pickBlacksmithFlavor(blacksmithCompletion)))

	return p.SendDM(userID, sb.String())
}

// ── Shop Display ────────────────────────────────────────────────────────────

func renderBlacksmithShop(equip map[EquipmentSlot]*AdvEquipment) string {
	var sb strings.Builder

	sb.WriteString("⚒️ **The Blacksmith**\n\n")
	sb.WriteString(fmt.Sprintf("*%s*\n\n", pickBlacksmithFlavor(blacksmithGreetings)))
	sb.WriteString("Your equipment:\n")

	hasDamaged := false
	for _, slot := range allSlots {
		eq := equip[slot]
		if eq == nil {
			continue
		}

		marker := ""
		if eq.Masterwork {
			marker = " \u2b50"
		} else if eq.ArenaTier > 0 {
			marker = " \u2694\ufe0f"
		}

		if eq.Condition >= 100 {
			sb.WriteString(fmt.Sprintf("  %s  %s:%s  T%d  [100/100]  \u2713 No repair needed\n",
				slotEmoji(slot), padRight(eq.Name+marker, 22), "", eq.Tier))
		} else {
			hasDamaged = true
			cost := blacksmithRepairCost(eq)
			sb.WriteString(fmt.Sprintf("  %s  %s:%s  T%d  [%d/100]   Repair to 100: \u20ac%d\n",
				slotEmoji(slot), padRight(eq.Name+marker, 22), "", eq.Tier, eq.Condition, cost))
		}
	}

	if !hasDamaged {
		sb.WriteString(fmt.Sprintf("\n*%s*", pickBlacksmithFlavor(blacksmithFullCondition)))
	} else {
		sb.WriteString("\nReply with the slot name to repair (weapon / armor / helmet / boots / tool) or \"all\" to repair everything.")
	}

	return sb.String()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func parseSlotName(s string) EquipmentSlot {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "weapon", "wep", "sword":
		return SlotWeapon
	case "armor", "armour", "chest":
		return SlotArmor
	case "helmet", "helm", "hat", "head":
		return SlotHelmet
	case "boots", "boot", "shoes", "feet":
		return SlotBoots
	case "tool", "pick", "pickaxe":
		return SlotTool
	}
	return ""
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
