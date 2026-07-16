package plugin

// Magic items — Open5e SRD import.
//
// Per the open5e integration plan, magic items are the one genuinely net-new
// surface: EquipmentDef is a flavor ladder, not a structured magic-item table,
// so there is nothing to extend. This file is the single new, non-dnd_-prefixed
// table the plan allows for them.
//
// Like the bestiary staging table, this is a vendored *reference registry* —
// 237 SRD magic items with classified Kind/Rarity/Slot/attunement and a
// rarity-bracket sell value. It is deliberately NOT wired into the zone loot
// tables or Luigi's shop yet; that integration is a separate follow-up pass.
// buildSRDMagicItems() is the generated dump (see magic_items_srd_data.go);
// magicItemOverlay is the hand-authored refinement layer that wins on ID
// collision, mirroring the spell- and bestiary-import pattern.

// MagicItemKind is the coarse Open5e item category — the first word of Open5e's
// free-text "type" field, normalised.
type MagicItemKind string

const (
	MagicItemWondrous MagicItemKind = "wondrous"
	MagicItemWeapon   MagicItemKind = "weapon"
	MagicItemArmor    MagicItemKind = "armor"
	MagicItemRing     MagicItemKind = "ring"
	MagicItemWand     MagicItemKind = "wand"
	MagicItemRod      MagicItemKind = "rod"
	MagicItemStaff    MagicItemKind = "staff"
	MagicItemPotion   MagicItemKind = "potion"
	MagicItemScroll   MagicItemKind = "scroll"
)

// MagicItem is one classified SRD magic item.
type MagicItem struct {
	ID         string
	Name       string
	Kind       MagicItemKind
	Rarity     DnDRarity
	Attunement bool
	// Slot is a best-effort equip slot. It is "" for consumables (potions,
	// scrolls) and for wondrous items whose name carries no wearable noun.
	Slot  DnDSlot
	Value int    // sell value, derived from the §8.1-aligned rarity bracket
	Desc  string // first-sentence summary of the SRD description
}

// magicItemOverlay — hand-authored magic items that win on ID collision with
// the generated SRD dump. Corrections (or wholly new items) land here rather
// than being edited into the generated file. Today it carries the T6 signature
// items (postgame_magic_items.go); this is a plain var reference, not an init()
// append, so it resolves before buildMagicItemRegistry() reads it.
var magicItemOverlay = postgameSignatureMagicItems

// magicItemRegistry is the merged lookup table: the generated SRD dump with the
// hand-authored overlay layered on top.
var magicItemRegistry = buildMagicItemRegistry()

func buildMagicItemRegistry() map[string]MagicItem {
	reg := make(map[string]MagicItem)
	for _, mi := range buildSRDMagicItems() {
		reg[mi.ID] = mi
	}
	for _, mi := range magicItemOverlay {
		reg[mi.ID] = mi // hand-authored wins
	}
	return reg
}
