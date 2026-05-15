package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Magic-item gameplay layer — wires the vendored Open5e SRD registry
// (see magic_items.go) into three live surfaces:
//
//  1. Zone loot drops      — dnd_zone_loot.go rolls registry items by rarity.
//  2. Luigi's "Curios"     — adventure_shop.go sells a rotating slate.
//  3. Combat effects       — equippable items grant CombatModifiers from a
//     codified Rarity+Kind formula; potion/scroll items route through the
//     existing consumable pipeline.
//
// The effects are *formulaic*, not hand-authored per item — 237 items is too
// many to tune by hand, and this mirrors the bestiary tuning pass: a codified
// formula fills the gap, and magicItemEffectOverlay is the hand-authored
// refinement path that wins on ID collision.

// ── Rarity / loot indexing ──────────────────────────────────────────────────

// magicItemsByRarity buckets the registry by rarity. Built once, lazily.
var magicItemsByRarityCache map[DnDRarity][]MagicItem

func magicItemsByRarity() map[DnDRarity][]MagicItem {
	if magicItemsByRarityCache != nil {
		return magicItemsByRarityCache
	}
	idx := make(map[DnDRarity][]MagicItem)
	ids := make([]string, 0, len(magicItemRegistry))
	for id := range magicItemRegistry {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic bucket order
	for _, id := range ids {
		mi := magicItemRegistry[id]
		idx[mi.Rarity] = append(idx[mi.Rarity], mi)
	}
	magicItemsByRarityCache = idx
	return idx
}

// pickMagicItemForRarity returns a uniform-random registry item of the given
// rarity. The §5 loot tiers only span Common..Legendary, so VeryRare items
// fold into the Epic bucket. Returns ok=false when no item matches.
func pickMagicItemForRarity(r DnDRarity, rng *rand.Rand) (MagicItem, bool) {
	idx := magicItemsByRarity()
	pool := append([]MagicItem(nil), idx[r]...)
	if r == RarityEpic {
		pool = append(pool, idx[RarityVeryRare]...)
	}
	if len(pool) == 0 {
		return MagicItem{}, false
	}
	return pool[rngIntN(rng, len(pool))], true
}

// magicItemSell builds the AdvItem an inventory deposit uses for a magic item.
// Potions and scrolls land as "consumable" so the combat pipeline picks them
// up (see magicItemConsumableDefByName); everything else is a "magic_item".
func magicItemSell(mi MagicItem) AdvItem {
	typ := "magic_item"
	if mi.Kind == MagicItemPotion || mi.Kind == MagicItemScroll {
		typ = "consumable"
	}
	return AdvItem{
		Name:  mi.Name,
		Type:  typ,
		Tier:  rarityLootTierNum(mi.Rarity),
		Value: int64(mi.Value),
	}
}

// rarityLootTierNum maps a rarity onto the 1–5 tier scale used by AdvItem.Tier
// and the inventory display, so magic items sort sensibly next to gear.
func rarityLootTierNum(r DnDRarity) int {
	switch r {
	case RarityCommon:
		return 1
	case RarityUncommon:
		return 2
	case RarityRare:
		return 3
	case RarityVeryRare, RarityEpic:
		return 4
	case RarityLegendary:
		return 5
	}
	return 1
}

// ── Codified combat-effect formula ──────────────────────────────────────────

// magicItemEffect is the combat delta a single equipped magic item grants.
// DamageReductMult is multiplicative (1.0 = neutral); everything else is
// additive. Potion/scroll items carry a zero effect — they are consumables.
type magicItemEffect struct {
	DamageBonus      float64
	DamageReductMult float64
	FlatDmgStart     int
	InitiativeBias   float64
	MaxHP            int
}

// magicItemEffectOverlay — hand-authored per-item effects that win over the
// codified formula. Empty for now; corrections land here rather than being
// folded into the formula, mirroring magicItemOverlay in magic_items.go.
var magicItemEffectOverlay = map[string]magicItemEffect{}

// rarityPowerScalar is the codified power axis: rarer item, bigger delta.
func rarityPowerScalar(r DnDRarity) float64 {
	switch r {
	case RarityCommon:
		return 1
	case RarityUncommon:
		return 2
	case RarityRare:
		return 3
	case RarityVeryRare, RarityEpic:
		return 4
	case RarityLegendary:
		return 5
	}
	return 1
}

// magicItemEffectFor returns the combat delta for an item: the hand-authored
// overlay if present, else the codified Rarity+Kind formula.
func magicItemEffectFor(mi MagicItem) magicItemEffect {
	if eff, ok := magicItemEffectOverlay[mi.ID]; ok {
		return eff
	}
	s := rarityPowerScalar(mi.Rarity)
	eff := magicItemEffect{DamageReductMult: 1.0}
	switch mi.Kind {
	case MagicItemWeapon:
		eff.DamageBonus = 0.05 * s
	case MagicItemStaff, MagicItemWand, MagicItemRod:
		eff.DamageBonus = 0.03 * s
		eff.FlatDmgStart = int(s)
	case MagicItemArmor:
		eff.DamageReductMult = 1.0 - 0.04*s
	case MagicItemRing:
		eff.DamageReductMult = 1.0 - 0.02*s
		eff.DamageBonus = 0.02 * s
	case MagicItemWondrous:
		eff.InitiativeBias = 0.5 * s
		eff.MaxHP = 2 * int(s)
	default:
		// potion / scroll — consumables, no equip effect.
	}
	return eff
}

// ── Equipped-item persistence ───────────────────────────────────────────────

// EquippedMagicItem is one row of magic_item_equipped, resolved against the
// registry. Item is the registry entry; Attuned mirrors the DB column.
type EquippedMagicItem struct {
	Slot    DnDSlot
	Item    MagicItem
	Attuned bool
}

// dndMagicItemAttuneLimit — 5e's three-attunement cap. Items that require
// attunement only grant their effect while attuned, and a player may hold at
// most this many attunements at once.
const dndMagicItemAttuneLimit = 3

func loadEquippedMagicItems(userID id.UserID) (map[DnDSlot]EquippedMagicItem, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT slot, item_id, attuned FROM magic_item_equipped WHERE user_id = ?`,
		string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[DnDSlot]EquippedMagicItem)
	for rows.Next() {
		var slot, itemID string
		var attuned int
		if err := rows.Scan(&slot, &itemID, &attuned); err != nil {
			return nil, err
		}
		mi, ok := magicItemRegistry[itemID]
		if !ok {
			continue // registry shrank under us — skip stale rows
		}
		out[DnDSlot(slot)] = EquippedMagicItem{
			Slot:    DnDSlot(slot),
			Item:    mi,
			Attuned: attuned == 1,
		}
	}
	return out, rows.Err()
}

// equipMagicItem writes (or replaces) the item in its slot. attuned is only
// honoured when the item requires attunement.
func equipMagicItem(userID id.UserID, slot DnDSlot, itemID string, attuned bool) error {
	d := db.Get()
	a := 0
	if attuned {
		a = 1
	}
	_, err := d.Exec(`
		INSERT INTO magic_item_equipped (user_id, slot, item_id, attuned)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, slot) DO UPDATE SET item_id = excluded.item_id, attuned = excluded.attuned`,
		string(userID), string(slot), itemID, a)
	return err
}

func unequipMagicItem(userID id.UserID, slot DnDSlot) error {
	d := db.Get()
	_, err := d.Exec(`DELETE FROM magic_item_equipped WHERE user_id = ? AND slot = ?`,
		string(userID), string(slot))
	return err
}

// countAttunedMagicItems returns how many attunement slots the player is
// currently using — callers gate new attunements on dndMagicItemAttuneLimit.
func countAttunedMagicItems(equipped map[DnDSlot]EquippedMagicItem) int {
	n := 0
	for _, e := range equipped {
		if e.Attuned {
			n++
		}
	}
	return n
}

// ── Combat hook ─────────────────────────────────────────────────────────────

// applyMagicItemEffects layers the player's equipped magic items onto their
// combat stats/modifiers. Called from the combat bridges right after the
// class/race/subclass passives, so its deltas compose on top of them.
//
// Items that require attunement only count while attuned; non-attunement
// items always count once equipped.
func applyMagicItemEffects(stats *CombatStats, mods *CombatModifiers, userID id.UserID) {
	equipped, err := loadEquippedMagicItems(userID)
	if err != nil || len(equipped) == 0 {
		return
	}
	for _, e := range equipped {
		if e.Item.Attunement && !e.Attuned {
			continue // unattuned attunement item is inert
		}
		eff := magicItemEffectFor(e.Item)
		mods.DamageBonus += eff.DamageBonus
		if eff.DamageReductMult != 0 && eff.DamageReductMult != 1.0 {
			mods.DamageReduct *= eff.DamageReductMult
		}
		mods.FlatDmgStart += eff.FlatDmgStart
		mods.InitiativeBias += eff.InitiativeBias
		if eff.MaxHP != 0 {
			stats.MaxHP += eff.MaxHP
			if stats.StartHP > 0 {
				stats.StartHP += eff.MaxHP
			}
		}
	}
}

// ── Consumable bridge (potions / scrolls) ───────────────────────────────────

// magicItemConsumableDef classifies a potion/scroll magic item onto the
// existing ConsumableDef pipeline so it auto-resolves in combat. Effect is
// name-sniffed; value scales with rarity. Returns nil for non-consumable
// kinds. Buyable is false — these come from loot and the Curios shelf, not
// the Supplies menu.
func magicItemConsumableDef(mi MagicItem) *ConsumableDef {
	if mi.Kind != MagicItemPotion && mi.Kind != MagicItemScroll {
		return nil
	}
	s := rarityPowerScalar(mi.Rarity)
	name := strings.ToLower(mi.Name)
	def := &ConsumableDef{
		Name:    mi.Name,
		Tier:    rarityLootTierNum(mi.Rarity),
		Buyable: false,
		Price:   int64(mi.Value),
	}
	switch {
	case strings.Contains(name, "healing"), strings.Contains(name, "heal"),
		strings.Contains(name, "vitality"), strings.Contains(name, "life"):
		def.Effect = EffectHeal
		def.Value = 20 + 15*s
		def.Slot = "defensive"
	case strings.Contains(name, "fire"), strings.Contains(name, "lightning"),
		strings.Contains(name, "acid"), strings.Contains(name, "flame"):
		def.Effect = EffectFlatDmg
		def.Value = 6 * s
		def.Slot = "offensive"
	case strings.Contains(name, "protection"), strings.Contains(name, "shield"),
		strings.Contains(name, "warding"), strings.Contains(name, "resistance"):
		def.Effect = EffectWard
		def.Value = 1
		def.Slot = "defensive"
	case strings.Contains(name, "speed"), strings.Contains(name, "haste"),
		strings.Contains(name, "flying"):
		def.Effect = EffectSpeedBoost
		def.Value = 0.10 + 0.05*s
		def.Slot = "offensive"
	case strings.Contains(name, "heroism"), strings.Contains(name, "strength"),
		strings.Contains(name, "rage"), strings.Contains(name, "giant"):
		def.Effect = EffectAtkBoost
		def.Value = 0.05 * s
		def.Slot = "offensive"
	case strings.Contains(name, "stoneskin"), strings.Contains(name, "barkskin"),
		strings.Contains(name, "iron"):
		def.Effect = EffectDefBoost
		def.Value = 0.05 * s
		def.Slot = "defensive"
	default:
		// Generic potion → modest heal; generic scroll → modest pre-damage.
		if mi.Kind == MagicItemScroll {
			def.Effect = EffectFlatDmg
			def.Value = 4 * s
			def.Slot = "offensive"
		} else {
			def.Effect = EffectHeal
			def.Value = 15 + 10*s
			def.Slot = "defensive"
		}
	}
	return def
}

// magicItemConsumableDefByName resolves a consumable magic item by name. Used
// by consumableDefByName as a fall-through so loot/shop magic potions resolve
// in combat without being added to the hardcoded consumableDefs table.
func magicItemConsumableDefByName(name string) *ConsumableDef {
	for _, mi := range magicItemRegistry {
		if mi.Name == name {
			return magicItemConsumableDef(mi)
		}
	}
	return nil
}

// ── Equip command (`!adventure equip-magic`) ────────────────────────────────

// advPendingMagicEquip is the pending-interaction payload for the numbered
// equip-magic picker.
type advPendingMagicEquip struct {
	Items []AdvItem
}

// magicItemFromAdvItem resolves the registry entry behind an inventory row.
// Inventory rows carry "magic_item:<id>" in SkillSource; name lookup is the
// fallback for rows written before that convention or by other paths.
func magicItemFromAdvItem(it AdvItem) (MagicItem, bool) {
	if strings.HasPrefix(it.SkillSource, "magic_item:") {
		if mi, ok := magicItemRegistry[strings.TrimPrefix(it.SkillSource, "magic_item:")]; ok {
			return mi, true
		}
	}
	for _, mi := range magicItemRegistry {
		if mi.Name == it.Name {
			return mi, true
		}
	}
	return MagicItem{}, false
}

// magicItemEffectSummary renders the codified combat delta as a short,
// player-facing string (accessibility goal — surface the outcome, not math).
func magicItemEffectSummary(mi MagicItem) string {
	eff := magicItemEffectFor(mi)
	var parts []string
	if eff.DamageBonus > 0 {
		parts = append(parts, fmt.Sprintf("+%.0f%% damage", eff.DamageBonus*100))
	}
	if eff.DamageReductMult > 0 && eff.DamageReductMult < 1.0 {
		parts = append(parts, fmt.Sprintf("-%.0f%% damage taken", (1.0-eff.DamageReductMult)*100))
	}
	if eff.FlatDmgStart > 0 {
		parts = append(parts, fmt.Sprintf("%d opening damage", eff.FlatDmgStart))
	}
	if eff.MaxHP > 0 {
		parts = append(parts, fmt.Sprintf("+%d HP", eff.MaxHP))
	}
	if eff.InitiativeBias > 0 {
		parts = append(parts, "faster to act")
	}
	if len(parts) == 0 {
		return "no combat effect"
	}
	return strings.Join(parts, ", ")
}

func (p *AdventurePlugin) handleEquipMagicCmd(ctx MessageContext) error {
	items, err := loadAdvInventory(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to access your inventory.")
	}
	var magic []AdvItem
	for _, it := range items {
		if it.Type != "magic_item" {
			continue
		}
		mi, ok := magicItemFromAdvItem(it)
		if !ok || mi.Slot == "" {
			continue // unslotted curios can't be worn
		}
		magic = append(magic, it)
	}
	if len(magic) == 0 {
		return p.SendDM(ctx.Sender,
			"You have no equippable magic items. Slotted curios drop from zones and Luigi's Curios shelf; "+
				"potions and scrolls auto-use in combat instead.")
	}

	equipped, _ := loadEquippedMagicItems(ctx.Sender)
	var sb strings.Builder
	sb.WriteString("🔮 **Equippable magic items:**\n\n")
	for i, it := range magic {
		mi, _ := magicItemFromAdvItem(it)
		curDesc := "empty"
		if cur, ok := equipped[mi.Slot]; ok && cur.Item.ID != "" {
			curDesc = cur.Item.Name
		}
		att := ""
		if mi.Attunement {
			att = " _(needs attunement)_"
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s, %s) → %s slot — currently: %s%s\n    %s\n",
			i+1, mi.Name, mi.Kind, mi.Rarity, mi.Slot, curDesc, att, magicItemEffectSummary(mi)))
	}
	sb.WriteString(fmt.Sprintf("\nAttunements in use: %d/%d\n",
		countAttunedMagicItems(equipped), dndMagicItemAttuneLimit))
	sb.WriteString("Reply with a number to equip, or \"cancel\".")

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "magic_equip",
		Data:      &advPendingMagicEquip{Items: magic},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	return p.SendDM(ctx.Sender, sb.String())
}

func (p *AdventurePlugin) resolveMagicEquipReply(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingMagicEquip)
	reply := strings.TrimSpace(ctx.Body)
	if strings.EqualFold(reply, "cancel") {
		return p.SendDM(ctx.Sender, "Equip cancelled.")
	}

	idx := 0
	parsed := false
	for _, c := range reply {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
			parsed = true
		} else {
			break
		}
	}
	idx-- // 1-indexed → 0-indexed
	if !parsed || idx < 0 || idx >= len(data.Items) {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Invalid selection. Reply with a number from the list, or \"cancel\".")
	}

	it := data.Items[idx]
	mi, ok := magicItemFromAdvItem(it)
	if !ok || mi.Slot == "" {
		return p.SendDM(ctx.Sender, "That item can't be equipped anymore.")
	}

	equipped, err := loadEquippedMagicItems(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your equipped magic items.")
	}

	// Auto-attune when the item needs it and an attunement slot is free.
	// Otherwise it equips inert until the player frees a slot.
	attune := false
	atCap := false
	if mi.Attunement {
		if countAttunedMagicItems(equipped) >= dndMagicItemAttuneLimit {
			atCap = true
		} else {
			attune = true
		}
	}

	// Return whatever currently occupies that slot to inventory.
	if prev, exists := equipped[mi.Slot]; exists && prev.Item.ID != "" {
		back := magicItemSell(prev.Item)
		back.Value = int64(prev.Item.Value) / 2
		back.SkillSource = "magic_item:" + prev.Item.ID
		if err := addAdvInventoryItem(ctx.Sender, back); err != nil {
			return p.SendDM(ctx.Sender, "Failed to return your currently-equipped item to inventory.")
		}
	}
	if err := equipMagicItem(ctx.Sender, mi.Slot, mi.ID, attune); err != nil {
		return p.SendDM(ctx.Sender, "Failed to equip that item.")
	}
	if err := removeAdvInventoryItem(it.ID); err != nil {
		slog.Error("magic-item: failed to remove equipped item from inventory",
			"user", ctx.Sender, "item", mi.ID, "err", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✨ **%s** equipped in your %s slot — %s.",
		mi.Name, mi.Slot, magicItemEffectSummary(mi)))
	switch {
	case mi.Attunement && attune:
		sb.WriteString(fmt.Sprintf("\nAttuned (%d/%d attunement slots used).",
			countAttunedMagicItems(equipped)+1, dndMagicItemAttuneLimit))
	case mi.Attunement && atCap:
		sb.WriteString(fmt.Sprintf("\n⚠️ All %d attunement slots are full — it's worn but **inert** until you free one (`!adventure equip-magic` to swap).",
			dndMagicItemAttuneLimit))
	}
	return p.SendDM(ctx.Sender, sb.String())
}
