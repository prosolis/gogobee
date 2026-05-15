package plugin

import (
	"fmt"
	"sort"
	"strings"
)

func (p *AdventurePlugin) handleDnDAbilitiesCmd(ctx MessageContext) error {
	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender, "No Adv 2.0 character yet — run `!setup` (or just enter combat and we'll auto-build one).")
	}
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	ab := dndClassAbilities[c.Class]

	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s %s — Abilities**\n\n", ri.Display, ci.Display))
	b.WriteString(fmt.Sprintf("**Class passive — %s**\n  %s\n\n", ab.Name, ab.Description))
	b.WriteString(fmt.Sprintf("**Race trait**\n  %s\n", ri.Passive))

	// Active abilities (Phase 6 + Phase 10 subclass-gated)
	actives := characterActiveAbilities(c)
	if len(actives) > 0 {
		resType, _ := classResourceMax(c.Class)
		cur, max, _ := getResource(c.UserID, resType)
		b.WriteString(fmt.Sprintf("\n**Active abilities** (%s %d/%d)\n", resType, cur, max))
		for _, a := range actives {
			b.WriteString(fmt.Sprintf("  • **%s** (1 %s) — %s\n", a.Name, a.Resource, a.Description))
		}
		b.WriteString("\nUse `!arm <ability>` to ready one for your next combat. Refreshes on long rest.\n")
		if c.ArmedAbility != "" {
			b.WriteString(fmt.Sprintf("\n_Currently armed: **%s**_\n", displayAbility(c.ArmedAbility)))
		}
	}
	return p.SendDM(ctx.Sender, b.String())
}

// !sheet — read-only D&D character sheet renderer.
//
// Sources:
//   dnd_character (D&D layer)
//   player_meta (skills, pet, housing — for at-a-glance context)
//   adventure_equipment (current gear)
//   adventure_treasures (= attunement substrate per v1.1 §7.4)

func (p *AdventurePlugin) handleDnDSheetCmd(ctx MessageContext) error {
	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your sheet: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender,
			"You don't have an Adv 2.0 character yet.\n\nRun `!setup` to begin character creation. "+
				"Your existing adventure progress (skills, pets, coins, arena streak) will be preserved.")
	}

	advChar, _ := loadAdvCharacter(ctx.Sender)
	equip, _ := loadAdvEquipment(ctx.Sender)
	treasures, _ := loadAdvTreasureBonuses(ctx.Sender)
	meta, _ := loadPlayerMeta(ctx.Sender)
	house, _ := loadHouseState(ctx.Sender)
	magicEquip, _ := loadEquippedMagicItems(ctx.Sender)

	return p.SendDM(ctx.Sender, renderDnDSheet(c, advChar, meta, house, equip, treasures, magicEquip))
}

func renderDnDSheet(c *DnDCharacter, adv *AdventureCharacter, meta *PlayerMeta, house HouseState, equip map[EquipmentSlot]*AdvEquipment, treasures []AdvTreasureBonus, magicEquip map[DnDSlot]EquippedMagicItem) string {
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	mods := c.Modifiers()

	var b strings.Builder
	name := string(c.UserID)
	if adv != nil && adv.DisplayName != "" {
		name = adv.DisplayName
	}
	b.WriteString(fmt.Sprintf("⚔️ **%s** — Level %d %s %s\n", name, c.Level, ri.Display, ci.Display))
	if c.Subclass != "" {
		if si, ok := subclassInfo(c.Subclass); ok {
			b.WriteString(fmt.Sprintf("    _%s_\n", si.Display))
		}
	} else if c.Level >= dndSubclassMinLevel {
		b.WriteString("    _Subclass: unchosen — run `!subclass`_\n")
	}
	b.WriteString(strings.Repeat("─", 36) + "\n")

	// Vitals
	b.WriteString(fmt.Sprintf("**HP** %d/%d", c.HPCurrent, c.HPMax))
	if c.TempHP > 0 {
		b.WriteString(fmt.Sprintf(" (+%d temp)", c.TempHP))
	}
	b.WriteString(fmt.Sprintf("    **AC** %d", c.ArmorClass))
	if c.Level >= dndMaxLevel {
		b.WriteString("    **XP** capped (L20)\n")
	} else {
		b.WriteString(fmt.Sprintf("    **XP** %d / %d (next L%d)\n",
			c.XP, dndXPToNextLevel(c.Level), c.Level+1))
	}

	// Ability scores with modifiers
	b.WriteString(fmt.Sprintf("STR %2d (%+d)   DEX %2d (%+d)   CON %2d (%+d)\n",
		c.STR, mods[0], c.DEX, mods[1], c.CON, mods[2]))
	b.WriteString(fmt.Sprintf("INT %2d (%+d)   WIS %2d (%+d)   CHA %2d (%+d)\n",
		c.INT, mods[3], c.WIS, mods[4], c.CHA, mods[5]))
	b.WriteString(fmt.Sprintf("\n_%s_\n", ri.Passive))

	// Equipment — D&D 10-slot view, with rarity inferred from legacy fields.
	b.WriteString("\n**Equipment**\n")
	dndEquip := map[DnDSlot]*AdvEquipment{}
	for _, slot := range allSlots {
		eq, ok := equip[slot]
		if !ok || eq == nil {
			continue
		}
		dndEquip[mapLegacySlot(slot)] = eq
	}
	anyEquipped := false
	for _, ds := range dndSlotOrder {
		eq := dndEquip[ds]
		if eq == nil {
			continue
		}
		anyEquipped = true
		rarity := equipmentRarity(eq)
		tag := ""
		if eq.Masterwork {
			tag = " ★"
		}
		b.WriteString(fmt.Sprintf("  %s %-9s T%d  %d%%%s  %s _(%s)_\n",
			rarityIcon(rarity), string(ds), eq.Tier, eq.Condition, tag, eq.Name, rarity))
	}
	if !anyEquipped {
		b.WriteString("  _(none equipped)_\n")
	}

	// Magic items — registry items worn in the D&D slots (separate from the
	// legacy tier-gear above). Attunement items mark themselves "(inactive)"
	// when worn but not bonded — they grant nothing until the player frees
	// an attunement slot via `!adventure equip-magic`.
	if len(magicEquip) > 0 {
		attunedNow := countAttunedMagicItems(magicEquip)
		b.WriteString("\n**Magic Items**\n")
		for _, ds := range dndSlotOrder {
			e, ok := magicEquip[ds]
			if !ok || e.Item.ID == "" {
				continue
			}
			status := ""
			if e.Item.Attunement {
				switch {
				case e.Attuned:
					status = fmt.Sprintf(" — bonded (%d/%d)", attunedNow, dndMagicItemAttuneLimit)
				case attunedNow >= dndMagicItemAttuneLimit:
					status = " — **(inactive)** _bond cap full_"
				default:
					status = " — **(inactive)** _unbonded_"
				}
			}
			b.WriteString(fmt.Sprintf("  %s %-9s %s _(%s)_%s\n    %s\n",
				rarityIcon(e.Item.Rarity), string(ds), e.Item.Name, e.Item.Rarity,
				status, magicItemEffectSummary(e.Item)))
		}
	}

	// Treasure bonuses (re-using adventure_treasures per v1.1 §7.4).
	// Player-facing label kept distinct from the magic-item "bond" vocab
	// so the sheet doesn't read as two competing attunement systems.
	if len(treasures) > 0 {
		b.WriteString("\n**Treasures**\n")
		// Group by treasure_key — one treasure can have multiple bonuses.
		byKey := map[string][]AdvTreasureBonus{}
		var keys []string
		for _, t := range treasures {
			if _, seen := byKey[t.TreasureKey]; !seen {
				keys = append(keys, t.TreasureKey)
			}
			byKey[t.TreasureKey] = append(byKey[t.TreasureKey], t)
		}
		sort.Strings(keys)
		for _, k := range keys {
			bonuses := byKey[k]
			name := bonuses[0].Name
			parts := make([]string, 0, len(bonuses))
			for _, bn := range bonuses {
				parts = append(parts, fmt.Sprintf("%s %+.1f", bn.BonusType, bn.BonusValue))
			}
			b.WriteString(fmt.Sprintf("  • %s — %s\n", name, strings.Join(parts, ", ")))
		}
	}

	// Legacy adventure context — preserved progress at a glance
	if adv != nil {
		b.WriteString("\n**Adventure progress** _(preserved)_\n")
		b.WriteString(fmt.Sprintf("  Mining %d  Foraging %d  Fishing %d\n",
			adv.MiningSkill, adv.ForagingSkill, adv.FishingSkill))
		wins, losses := adv.ArenaWins, adv.ArenaLosses
		if meta != nil {
			wins, losses = meta.ArenaWins, meta.ArenaLosses
		}
		b.WriteString(fmt.Sprintf("  Arena %dW/%dL   Streak %d (best %d)\n",
			wins, losses, adv.CurrentStreak, adv.BestStreak))
		if adv.PetName != "" {
			b.WriteString(fmt.Sprintf("  Pet: %s the %s (lv %d)\n", adv.PetName, adv.PetType, adv.PetLevel))
		}
		if house.Tier > 0 {
			b.WriteString(fmt.Sprintf("  Housing: tier %d\n", house.Tier))
		}
	}

	return b.String()
}
