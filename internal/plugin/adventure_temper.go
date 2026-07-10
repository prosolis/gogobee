package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Tempering ───────────────────────────────────────────────────────────────
//
// The blacksmith's endgame role: push an owned magic item one rung up the
// rarity ladder (gogobee_engagement_plan.md B1). Rarity flows through the
// existing scalar formulas in magic_items_gameplay.go, so tempering adds no
// new combat math — and because it caps at Legendary it accelerates reaching
// the existing best-in-slot ceiling rather than raising it. Every duplicate
// drop becomes a candidate instead of vendor trash.
//
// Rarity itself lives on the registry definition, so a tempered instance
// records its progress as a `temper` step count on its own row; effective
// rarity is derived on read (see temperedItem).

// temperMaterialNames are the legendary crafting materials the final rung
// consumes. Both drop as UniqueAlways entries on their T5 zone slate, so one
// clear of the Underforge or the Abyss Portal yields exactly one.
var temperMaterialNames = []string{"Thyraks Core", "Portal Fragment"}

// temperStepCost is what one rung costs, keyed by the *target* rarity's tier.
// The euro costs and foraging gates mirror the crafting recipe tiers in
// adventure_consumables.go, so the blacksmith reads as an extension of the
// crafting ladder rather than a parallel economy.
type temperStepCost struct {
	Euros         int
	MinForaging   int
	NeedsMaterial bool
}

// Only the final rung demands a T5 material. Gating the lower rungs on one too
// would make tempering unreachable until a player already clears T5 content —
// which is exactly when they stop needing it.
var temperCosts = map[int]temperStepCost{
	2: {Euros: 10_000, MinForaging: 15},
	3: {Euros: 25_000, MinForaging: 20},
	4: {Euros: 60_000, MinForaging: 25},
	5: {Euros: 150_000, MinForaging: 30, NeedsMaterial: true},
}

// temperTarget is one temperable item, from either side of the equip line.
type temperTarget struct {
	Equipped bool
	Slot     DnDSlot // set when Equipped
	InvID    int64   // set when not Equipped
	Base     MagicItem
	Temper   int
}

// Effective is the item as it stands today, before this temper.
func (t temperTarget) Effective() MagicItem { return temperedItem(t.Base, t.Temper) }

// NextRarity is the rung this temper would buy.
func (t temperTarget) NextRarity() DnDRarity { return temperedRarity(t.Base.Rarity, t.Temper+1) }

// cost is the price of this target's next rung.
func (t temperTarget) cost() temperStepCost { return temperCosts[rarityLootTierNum(t.NextRarity())] }

// sameItemAs reports whether other is the same physical instance, used to
// re-find a target after a fresh reload so a concurrent equip/sell can't let a
// stale pending confirm temper the wrong thing.
func (t temperTarget) sameItemAs(other temperTarget) bool {
	if t.Equipped != other.Equipped {
		return false
	}
	if t.Equipped {
		return t.Slot == other.Slot && t.Base.ID == other.Base.ID
	}
	return t.InvID == other.InvID
}

type advPendingTemperPick struct {
	Targets []temperTarget
}

type advPendingTemperConfirm struct {
	Target   temperTarget
	Material *AdvItem // consumed on confirm; nil when the rung needs none
}

// collectTemperTargets gathers every magic item the player owns that still has
// a rung ahead of it — worn or stowed. Potions and scrolls are excluded: they
// carry a zero combat effect, so a rarity bump would buy nothing.
func collectTemperTargets(userID id.UserID) ([]temperTarget, error) {
	var out []temperTarget

	equipped, err := loadEquippedMagicItems(userID)
	if err != nil {
		return nil, err
	}
	for _, ds := range dndSlotOrder {
		e, ok := equipped[ds]
		if !ok || e.Item.ID == "" {
			continue
		}
		if temperStepsToLegendary(e.Item.Rarity, e.Temper) == 0 {
			continue
		}
		out = append(out, temperTarget{Equipped: true, Slot: ds, Base: e.Item, Temper: e.Temper})
	}

	items, err := loadAdvInventory(userID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Type != "magic_item" {
			continue
		}
		mi, ok := magicItemFromAdvItem(it)
		if !ok || mi.Kind == MagicItemPotion || mi.Kind == MagicItemScroll {
			continue
		}
		if temperStepsToLegendary(mi.Rarity, it.Temper) == 0 {
			continue
		}
		out = append(out, temperTarget{InvID: it.ID, Base: mi, Temper: it.Temper})
	}
	return out, nil
}

// findTemperMaterial returns the first legendary material row in inventory.
func findTemperMaterial(items []AdvItem) (AdvItem, bool) {
	for _, it := range items {
		for _, name := range temperMaterialNames {
			if it.Name == name {
				return it, true
			}
		}
	}
	return AdvItem{}, false
}

// parseTemperIndex reads a leading 1-indexed number and bounds-checks it.
func parseTemperIndex(reply string, n int) (int, bool) {
	idx, parsed := 0, false
	for _, c := range strings.TrimSpace(reply) {
		if c < '0' || c > '9' {
			break
		}
		idx = idx*10 + int(c-'0')
		parsed = true
	}
	idx--
	if !parsed || idx < 0 || idx >= n {
		return 0, false
	}
	return idx, true
}

// ── Command ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleTemperCmd(ctx MessageContext) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	targets, err := collectTemperTargets(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to read your curios.")
	}
	if len(targets) == 0 {
		return p.SendDM(ctx.Sender,
			"_He looks over your kit and shrugs._ Nothing here I can push any further. "+
				"Bring me something that isn't already Legendary.")
	}

	greeting, _ := advPickFlavor(blacksmithTemperGreetings, ctx.Sender, "temper_greet")

	var sb strings.Builder
	sb.WriteString("🔨 **Tempering**\n\n")
	sb.WriteString(greeting)
	sb.WriteString("\n\n")
	for i, t := range targets {
		cur := t.Effective()
		cost := t.cost()
		where := "stowed"
		if t.Equipped {
			where = "worn — " + string(t.Slot)
		}
		gate := ""
		switch {
		case char.ForagingSkill < cost.MinForaging:
			gate = fmt.Sprintf(" — 🔒 needs Foraging %d", cost.MinForaging)
		case cost.NeedsMaterial:
			gate = " — needs a legendary material"
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** _(%s → %s)_ — €%d _(%s)_%s\n",
			i+1, cur.Name, cur.Rarity, t.NextRarity(), cost.Euros, where, gate))
	}
	sb.WriteString("\nReply with a number to temper it, or \"cancel\".")

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "temper_pick",
		Data:      &advPendingTemperPick{Targets: targets},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	return p.SendDM(ctx.Sender, sb.String())
}

func (p *AdventurePlugin) resolveTemperPick(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingTemperPick)
	reply := strings.TrimSpace(ctx.Body)
	if strings.EqualFold(reply, "cancel") {
		return p.SendDM(ctx.Sender, "_The forge keeps burning._ Come back when you've decided.")
	}
	idx, ok := parseTemperIndex(reply, len(data.Targets))
	if !ok {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Reply with a number from the list, or \"cancel\".")
	}
	return p.buildTemperConfirm(ctx.Sender, data.Targets[idx])
}

// buildTemperConfirm re-checks every gate against fresh state before quoting a
// price, then parks a confirm. executeTemper re-checks them all again — this
// pass exists to give the player a real error message, not to be trusted.
func (p *AdventurePlugin) buildTemperConfirm(userID id.UserID, t temperTarget) error {
	char, _, err := p.ensureCharacter(userID)
	if err != nil {
		return p.SendDM(userID, "Failed to load your character.")
	}
	cost := t.cost()
	if char.ForagingSkill < cost.MinForaging {
		return p.SendDM(userID, fmt.Sprintf(
			"_He turns the piece over, then sets it down._ I could ruin this. Come back when you know your "+
				"materials better — **Foraging %d**. You're at %d.", cost.MinForaging, char.ForagingSkill))
	}

	items, err := loadAdvInventory(userID)
	if err != nil {
		return p.SendDM(userID, "Failed to read your inventory.")
	}
	var material *AdvItem
	if cost.NeedsMaterial {
		mat, ok := findTemperMaterial(items)
		if !ok {
			return p.SendDM(userID, fmt.Sprintf(
				"_He exhales through his teeth._ Legendary is a different conversation. I need a **%s** on this "+
					"table before I touch it. They come out of the deepest places — the Underforge, the Portal.",
				strings.Join(temperMaterialNames, "** or a **")))
		}
		material = &mat
	}

	if bal := p.euro.GetBalance(userID); bal < float64(cost.Euros) {
		return p.SendDM(userID, fmt.Sprintf(
			"_He names the price without apology._ €%d. You have €%.0f. The forge doesn't run on good intentions.",
			cost.Euros, bal))
	}

	cur := t.Effective()
	next := temperedItem(t.Base, t.Temper+1)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔨 **Temper %s?**\n\n", cur.Name))
	sb.WriteString(fmt.Sprintf("  %s _(%s)_ → %s _(%s)_\n", cur.Name, cur.Rarity, next.Name, next.Rarity))
	sb.WriteString(fmt.Sprintf("  %s\n  ↳ %s\n\n", magicItemEffectSummary(cur), magicItemEffectSummary(next)))
	sb.WriteString(fmt.Sprintf("  Cost: **€%d**", cost.Euros))
	if material != nil {
		sb.WriteString(fmt.Sprintf(" + one **%s**", material.Name))
	}
	sb.WriteString("\n\nReply \"yes\" to hand it over, or anything else to keep it as it is.")

	p.pending.Store(string(userID), &advPendingInteraction{
		Type:      "temper_confirm",
		Data:      &advPendingTemperConfirm{Target: t, Material: material},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	return p.SendDM(userID, sb.String())
}

func (p *AdventurePlugin) resolveTemperConfirm(ctx MessageContext, interaction *advPendingInteraction) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	data := interaction.Data.(*advPendingTemperConfirm)
	reply := strings.ToLower(strings.TrimSpace(ctx.Body))
	if reply != "yes" && reply != "y" && reply != "confirm" {
		return p.SendDM(ctx.Sender, "_He nods once and turns back to the fire._ It's a good piece as it is.")
	}
	return p.executeTemper(ctx.Sender, data)
}

// executeTemper re-validates against fresh state, then charges and applies.
// Order matters: the euro debit is the only atomic guard we have, so it goes
// first; the material and the temper write both roll back onto it.
func (p *AdventurePlugin) executeTemper(userID id.UserID, data *advPendingTemperConfirm) error {
	char, _, err := p.ensureCharacter(userID)
	if err != nil {
		return p.SendDM(userID, "Failed to load your character.")
	}

	// Re-find the target. Between the confirm prompt and this reply the
	// player may have equipped, sold, or already tempered it.
	fresh, err := collectTemperTargets(userID)
	if err != nil {
		return p.SendDM(userID, "Failed to read your curios.")
	}
	var target temperTarget
	found := false
	for _, t := range fresh {
		if t.sameItemAs(data.Target) {
			target, found = t, true
			break
		}
	}
	if !found {
		return p.SendDM(userID, "_He looks at the empty table._ That piece isn't here any more.")
	}
	if target.Temper != data.Target.Temper {
		return p.SendDM(userID, "_He raises an eyebrow._ Someone's already been at this one. Ask me again.")
	}

	cost := target.cost()
	if char.ForagingSkill < cost.MinForaging {
		return p.SendDM(userID, fmt.Sprintf("You need **Foraging %d** for that.", cost.MinForaging))
	}

	// Re-locate the material by row id — a concurrent craft may have eaten it.
	var material *AdvItem
	if cost.NeedsMaterial {
		items, err := loadAdvInventory(userID)
		if err != nil {
			return p.SendDM(userID, "Failed to read your inventory.")
		}
		mat, ok := findTemperMaterial(items)
		if !ok {
			return p.SendDM(userID, "_He taps the empty spot on the table._ The material's gone. Find another.")
		}
		material = &mat
	}

	if !p.euro.Debit(userID, float64(cost.Euros), "temper") {
		return p.SendDM(userID, fmt.Sprintf("Payment failed — tempering costs €%d.", cost.Euros))
	}

	refund := func(reason string) {
		p.euro.Credit(userID, float64(cost.Euros), "temper_refund")
		slog.Error("temper: rolled back", "user", userID, "item", target.Base.ID, "reason", reason)
	}

	if material != nil {
		if err := removeAdvInventoryItem(material.ID); err != nil {
			refund("material consume failed")
			return p.SendDM(userID, "Something went wrong at the forge. Your money is back.")
		}
	}

	newTemper := target.Temper + 1
	if target.Equipped {
		err = temperEquippedItem(userID, target.Slot, newTemper)
	} else {
		bumped := temperedItem(target.Base, newTemper)
		err = temperInventoryItem(target.InvID, newTemper, rarityLootTierNum(bumped.Rarity), int64(bumped.Value))
	}
	if err != nil {
		refund("temper write failed")
		if material != nil {
			if rbErr := addAdvInventoryItem(userID, *material); rbErr != nil {
				slog.Error("temper: material rollback failed",
					"user", userID, "material", material.Name, "err", rbErr)
			}
		}
		return p.SendDM(userID, "Something went wrong at the forge. Your money is back.")
	}

	result := temperedItem(target.Base, newTemper)
	line, _ := advPickFlavor(blacksmithTemperCompletion, userID, "temper_done")

	var sb strings.Builder
	sb.WriteString(line)
	sb.WriteString(fmt.Sprintf("\n\n🔨 **%s** is now **%s**.\n%s\n",
		result.Name, result.Rarity, magicItemEffectSummary(result)))
	if remaining := temperStepsToLegendary(target.Base.Rarity, newTemper); remaining > 0 {
		sb.WriteString(fmt.Sprintf("\n_%d more temper%s would make it Legendary._",
			remaining, pluralS(remaining)))
	}
	if err := p.SendDM(userID, sb.String()); err != nil {
		return err
	}

	if result.Rarity == RarityLegendary {
		if p.achievements != nil {
			p.achievements.GrantAchievement(userID, "temper_legendary")
		}
		if gr := gamesRoom(); gr != "" {
			displayName, _ := loadDisplayName(userID)
			p.SendMessage(id.RoomID(gr), fmt.Sprintf(
				"🔨 The forge in town ran hot all night. **%s** walked out with a **Legendary %s**.",
				displayName, result.Name))
		}
	}
	return nil
}
