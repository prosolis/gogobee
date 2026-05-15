package plugin

// Phase 4 — D&D equipment view.
//
// Strategy per v1.1 §7.3: legacy adventure_equipment is the source of truth
// for what a player has equipped. The D&D layer maps the 5-slot legacy
// scheme onto the 10-slot D&D scheme at read time. No migration writes
// happen. The five new slots (legs, hands, ring_1, ring_2, amulet) are
// reserved for future drops and stay empty for now.

// DnDSlot is the canonical D&D equipment slot. legacy adventure_equipment
// stores items under EquipmentSlot (weapon/armor/helmet/boots/tool); we
// map those into D&D slots in mapLegacySlot.
type DnDSlot string

const (
	DnDSlotHead     DnDSlot = "head"
	DnDSlotChest    DnDSlot = "chest"
	DnDSlotLegs     DnDSlot = "legs"
	DnDSlotHands    DnDSlot = "hands"
	DnDSlotFeet     DnDSlot = "feet"
	DnDSlotMainHand DnDSlot = "main_hand"
	DnDSlotOffHand  DnDSlot = "off_hand"
	DnDSlotRing1    DnDSlot = "ring_1"
	DnDSlotRing2    DnDSlot = "ring_2"
	DnDSlotAmulet   DnDSlot = "amulet"
)

// dndSlotOrder controls display order for !sheet.
var dndSlotOrder = []DnDSlot{
	DnDSlotHead, DnDSlotChest, DnDSlotLegs, DnDSlotHands, DnDSlotFeet,
	DnDSlotMainHand, DnDSlotOffHand,
	DnDSlotRing1, DnDSlotRing2, DnDSlotAmulet,
}

// mapLegacySlot translates a legacy EquipmentSlot into the D&D slot it
// occupies in the new view. The legacy `tool` slot is mapped to off_hand
// since tools (pickaxes, fishing rods, etc.) function as off-hand items.
func mapLegacySlot(s EquipmentSlot) DnDSlot {
	switch s {
	case SlotWeapon:
		return DnDSlotMainHand
	case SlotArmor:
		return DnDSlotChest
	case SlotHelmet:
		return DnDSlotHead
	case SlotBoots:
		return DnDSlotFeet
	case SlotTool:
		return DnDSlotOffHand
	}
	return ""
}

// ── Rarity inference ─────────────────────────────────────────────────────────

// DnDRarity tags an item for D&D-style display. Inferred from the legacy
// tier + masterwork flag. New drops with explicit dnd_rarity (Phase 4+
// loot generation) skip this inference.
type DnDRarity string

const (
	RarityCommon    DnDRarity = "Common"
	RarityUncommon  DnDRarity = "Uncommon"
	RarityRare      DnDRarity = "Rare"
	RarityEpic      DnDRarity = "Epic"
	RarityVeryRare  DnDRarity = "VeryRare"
	RarityLegendary DnDRarity = "Legendary"
)

// rarityIcon — leading symbol for !sheet rendering (color stand-ins for
// terminals that don't render emoji color squares well).
func rarityIcon(r DnDRarity) string {
	switch r {
	case RarityCommon:
		return "⬜"
	case RarityUncommon:
		return "🟩"
	case RarityRare:
		return "🟦"
	case RarityEpic, RarityVeryRare:
		return "🟪"
	case RarityLegendary:
		return "🟧"
	}
	return ""
}

// inferRarity maps legacy gear (tier + masterwork + arena_set) to a D&D
// rarity tier. Used at read time when dnd_rarity is empty.
func inferRarity(tier int, masterwork bool, arenaTier int) DnDRarity {
	if masterwork {
		// Masterwork is the legacy version of "this gear is special" —
		// always treat as at least Rare to match its mechanical weight.
		if tier >= 5 {
			return RarityEpic
		}
		return RarityRare
	}
	if arenaTier > 0 {
		// Arena set gear: rarity scales with arena tier.
		switch arenaTier {
		case 1, 2:
			return RarityUncommon
		case 3:
			return RarityRare
		case 4:
			return RarityEpic
		default:
			return RarityLegendary
		}
	}
	switch {
	case tier <= 2:
		return RarityCommon
	case tier <= 4:
		return RarityUncommon
	case tier <= 6:
		return RarityRare
	case tier <= 8:
		return RarityEpic
	default:
		return RarityLegendary
	}
}

// equipmentRarity returns the rarity to display for an AdvEquipment row.
// Honors any explicit dnd_rarity on the row (Phase 4+ loot drops);
// otherwise falls back to inferRarity.
func equipmentRarity(eq *AdvEquipment) DnDRarity {
	if eq == nil {
		return RarityCommon
	}
	return inferRarity(eq.Tier, eq.Masterwork, eq.ArenaTier)
}
