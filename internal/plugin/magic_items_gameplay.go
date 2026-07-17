package plugin

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
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

// magicItemsByRarity buckets the registry by rarity. Built once via
// sync.Once — concurrent loot rolls on cold start would otherwise race
// on the cache assignment below.
var (
	magicItemsByRarityOnce  sync.Once
	magicItemsByRarityCache map[DnDRarity][]MagicItem
)

func magicItemsByRarity() map[DnDRarity][]MagicItem {
	magicItemsByRarityOnce.Do(func() {
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
	})
	return magicItemsByRarityCache
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
func magicItemSell(mi MagicItem) AdvItem { return magicItemSellAt(mi, 0) }

// magicItemSellAt is magicItemSell for an instance carrying temper steps: the
// row it produces is priced and tiered at the item's effective rarity, and
// remembers the temper so equipping it again restores the upgrade.
func magicItemSellAt(mi MagicItem, temper int) AdvItem {
	typ := "magic_item"
	if mi.Kind == MagicItemPotion || mi.Kind == MagicItemScroll {
		typ = "consumable"
	}
	eff := temperedItem(mi, temper)
	return AdvItem{
		Name:   eff.Name,
		Type:   typ,
		Tier:   rarityLootTierNum(eff.Rarity),
		Value:  int64(eff.Value),
		Temper: temper,
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

// ── Tempering: per-instance rarity above the definition's base ──────────────

// temperLadder is the rarity ladder tempering walks. VeryRare is absent by
// design: it collapses onto Epic in both rarityLootTierNum and
// rarityPowerScalar, so treating it as a distinct rung would sell the player
// a step that changes no number.
var temperLadder = []DnDRarity{RarityCommon, RarityUncommon, RarityRare, RarityEpic, RarityLegendary}

// temperRung locates a rarity on temperLadder, folding VeryRare onto Epic.
func temperRung(r DnDRarity) int {
	if r == RarityVeryRare {
		r = RarityEpic
	}
	for i, lr := range temperLadder {
		if lr == r {
			return i
		}
	}
	return 0
}

// temperedRarity applies steps rungs of tempering to a base rarity, clamped at
// Legendary. Callers derive this on read — the base rarity on the registry
// definition stays authoritative, so an item can never double-bump across a
// load/save round-trip.
func temperedRarity(base DnDRarity, steps int) DnDRarity {
	if steps <= 0 {
		return base
	}
	rung := temperRung(base) + steps
	if rung >= len(temperLadder) {
		rung = len(temperLadder) - 1
	}
	return temperLadder[rung]
}

// temperStepsToLegendary reports how many tempers an item at this base rarity
// and temper count still has ahead of it.
func temperStepsToLegendary(base DnDRarity, steps int) int {
	return (len(temperLadder) - 1) - temperRung(temperedRarity(base, steps))
}

// temperedItem returns a copy of mi at its effective rarity. Everything that
// reads rarity for gameplay — effects, sell value, display — goes through here.
func temperedItem(mi MagicItem, steps int) MagicItem {
	if steps <= 0 {
		return mi
	}
	eff := temperedRarity(mi.Rarity, steps)
	if eff == mi.Rarity {
		return mi
	}
	// Value tracks the power axis, so a tempered item is worth what an item
	// of its effective rarity is worth.
	scaled := float64(mi.Value) * (rarityPowerScalar(eff) / rarityPowerScalar(mi.Rarity))
	mi.Value = int(scaled)
	mi.Rarity = eff
	return mi
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
// codified formula. Corrections land here rather than being folded into the
// formula, mirroring magicItemOverlay in magic_items.go. Today it carries the
// T6 signature items' bespoke combat numbers (postgame_magic_items.go).
var magicItemEffectOverlay = postgameSignatureEffects

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
	Item    MagicItem // registry definition, at its BASE rarity
	Attuned bool
	Temper  int
}

// Effective is the item as the game should see it: base definition plus any
// tempering the player has paid for. Item stays at base rarity so re-saving an
// equipped row can't compound the bump.
func (e EquippedMagicItem) Effective() MagicItem { return temperedItem(e.Item, e.Temper) }

// dndMagicItemAttuneLimit — 5e's three-attunement cap. Items that require
// attunement only grant their effect while attuned, and a player may hold at
// most this many attunements at once.
const dndMagicItemAttuneLimit = 3

func loadEquippedMagicItems(userID id.UserID) (map[DnDSlot]EquippedMagicItem, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT slot, item_id, attuned, temper FROM magic_item_equipped WHERE user_id = ?`,
		string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[DnDSlot]EquippedMagicItem)
	for rows.Next() {
		var slot, itemID string
		var attuned, temper int
		if err := rows.Scan(&slot, &itemID, &attuned, &temper); err != nil {
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
			Temper:  temper,
		}
	}
	return out, rows.Err()
}

// equipMagicItem writes (or replaces) the item in its slot. attuned is only
// honoured when the item requires attunement. temper rides along from the
// inventory row so tempering survives being worn.
func equipMagicItem(userID id.UserID, slot DnDSlot, itemID string, attuned bool, temper int) error {
	d := db.Get()
	a := 0
	if attuned {
		a = 1
	}
	_, err := d.Exec(`
		INSERT INTO magic_item_equipped (user_id, slot, item_id, attuned, temper)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, slot) DO UPDATE SET
			item_id = excluded.item_id, attuned = excluded.attuned, temper = excluded.temper`,
		string(userID), string(slot), itemID, a, temper)
	return err
}

// temperEquippedItem bumps the temper on a worn item in place.
func temperEquippedItem(userID id.UserID, slot DnDSlot, temper int) error {
	_, err := db.Get().Exec(`UPDATE magic_item_equipped SET temper = ? WHERE user_id = ? AND slot = ?`,
		temper, string(userID), string(slot))
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

// reconcileMagicAttunements bonds any worn attunement item that is sitting
// inert while a bond slot is free. Bonding is strictly beneficial (there are no
// cursed items), so the only legitimate reason for a worn attunement item to be
// unbonded is the hard cap — the moment a slot frees, every straggler should
// light up. Without this, an item equipped while at cap stays permanently inert:
// the equip picker only lists inventory, so a slotted item can never be reached
// to re-attempt bonding. Idempotent; returns the names of items it newly bonded.
func reconcileMagicAttunements(userID id.UserID) ([]string, error) {
	equipped, err := loadEquippedMagicItems(userID)
	if err != nil {
		return nil, err
	}
	used := countAttunedMagicItems(equipped)
	if used >= dndMagicItemAttuneLimit {
		return nil, nil
	}
	var bonded []string
	for _, ds := range dndSlotOrder { // deterministic order when slots are scarce
		if used >= dndMagicItemAttuneLimit {
			break
		}
		e, ok := equipped[ds]
		if !ok || e.Item.ID == "" || !e.Item.Attunement || e.Attuned {
			continue
		}
		if _, err := db.Get().Exec(
			`UPDATE magic_item_equipped SET attuned = 1 WHERE user_id = ? AND slot = ?`,
			string(userID), string(ds)); err != nil {
			return bonded, err
		}
		bonded = append(bonded, e.Item.Name)
		used++
	}
	return bonded, nil
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
		eff := magicItemEffectFor(e.Effective())
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

// activeMagicItemsLine renders a one-shot combat-start line listing the
// player's currently-contributing magic items (attunement items only count
// while bonded). Returns "" when nothing is active so callers can no-op.
//
// Mirrors the "you fight better because of X" surfacing pattern players
// already get for class passives — the system is invisible otherwise, and
// invisible magic items are functionally indistinguishable from no magic
// items at all.
func activeMagicItemsLine(userID id.UserID) string {
	equipped, err := loadEquippedMagicItems(userID)
	if err != nil || len(equipped) == 0 {
		return ""
	}
	var names []string
	for _, ds := range dndSlotOrder {
		e, ok := equipped[ds]
		if !ok || e.Item.ID == "" {
			continue
		}
		if e.Item.Attunement && !e.Attuned {
			continue // worn but inert — contributes nothing
		}
		names = append(names, e.Item.Name)
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return fmt.Sprintf("✨ _%s hums into the fight._", names[0])
	}
	return fmt.Sprintf("✨ _Your curios stir: %s._", strings.Join(names, ", "))
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

// The equip mutation, message-free, shared by the DM resolver above and the web
// equip queue (pete_equip.go). Splitting it out is deliberate: the ordering below
// — evict occupant, then remove-from-inventory *before* equip, with a rollback if
// equip fails — is the whole defence against item duplication, and a second copy
// of it living in the web path is exactly where that defence would silently rot.
// One implementation, two callers.

// errItemNotEquippable is a *permanent* refusal (not a magic item, or a slotless
// curio) — the caller should reject the request, not retry it. Every other error
// from applyMagicEquip is a transient DB fault the caller may retry.
var errItemNotEquippable = errors.New("magic-item: not equippable")

// errSlotEmpty is applyMagicUnequip's permanent refusal: nothing is in that slot.
var errSlotEmpty = errors.New("magic-item: slot empty")

// magicEquipOutcome is what an equip did, for the caller to narrate. BondsBefore
// is the attunement count *after* any swap-eviction and *before* this item, so a
// caller reporting "bonded (N/3)" adds one.
type magicEquipOutcome struct {
	Effective   MagicItem // the item as worn, tempering folded in
	SwappedBack string    // name of the occupant sent back to inventory, or ""
	Bonded      bool      // this item took a bond just now
	AtCap       bool      // worn but inert: it wants a bond and all 3 are used
	BondsBefore int       // bonds in use before this item was worn
	Healed      []string  // stragglers a freed slot let bond, post-equip
}

// applyMagicEquip wears one inventory item, preserving the anti-duplication
// ordering. It mutates the equipment tables and sends nothing.
func applyMagicEquip(userID id.UserID, it AdvItem) (magicEquipOutcome, error) {
	mi, ok := magicItemFromAdvItem(it)
	if !ok || mi.Slot == "" {
		return magicEquipOutcome{}, errItemNotEquippable
	}
	equipped, err := loadEquippedMagicItems(userID)
	if err != nil {
		return magicEquipOutcome{}, err
	}

	// Return whatever occupies that slot to inventory at full value, and drop it
	// from the local map so the bond count below reflects the post-swap state.
	var swappedBack string
	if prev, exists := equipped[mi.Slot]; exists && prev.Item.ID != "" {
		back := magicItemSellAt(prev.Item, prev.Temper)
		back.SkillSource = "magic_item:" + prev.Item.ID
		if err := addAdvInventoryItem(userID, back); err != nil {
			return magicEquipOutcome{}, err
		}
		swappedBack = prev.Item.Name
		delete(equipped, mi.Slot)
	}

	bondsBefore := countAttunedMagicItems(equipped)
	bonded, atCap := false, false
	if mi.Attunement {
		if bondsBefore >= dndMagicItemAttuneLimit {
			atCap = true
		} else {
			bonded = true
		}
	}

	// Remove the inventory row FIRST, then equip; if equip fails, restore the row.
	// The reverse order left a transient failure with the item both worn and in the
	// pack — a free duplicate.
	if err := removeAdvInventoryItem(it.ID); err != nil {
		slog.Error("magic-item: failed to remove from inventory before equip",
			"user", userID, "item", mi.ID, "err", err)
		return magicEquipOutcome{}, err
	}
	if err := equipMagicItem(userID, mi.Slot, mi.ID, bonded, it.Temper); err != nil {
		restored := magicItemSellAt(mi, it.Temper)
		restored.Value = it.Value
		restored.SkillSource = "magic_item:" + mi.ID
		if rbErr := addAdvInventoryItem(userID, restored); rbErr != nil {
			slog.Error("magic-item: equip failed AND inventory rollback failed",
				"user", userID, "item", mi.ID, "equip_err", err, "rollback_err", rbErr)
		}
		return magicEquipOutcome{}, err
	}

	// Swapping the occupant out may have freed a bond slot — light up any straggler.
	healed, _ := reconcileMagicAttunements(userID)
	return magicEquipOutcome{
		Effective:   temperedItem(mi, it.Temper),
		SwappedBack: swappedBack,
		Bonded:      bonded,
		AtCap:       atCap,
		BondsBefore: bondsBefore,
		Healed:      healed,
	}, nil
}

// magicUnequipOutcome is what an unequip did, for the caller to narrate.
type magicUnequipOutcome struct {
	Item   MagicItem // the item taken off, as it was worn
	Healed []string  // stragglers the freed bond slot let bond
}

// applyMagicUnequip takes the item off a slot and returns it to inventory,
// mirroring the equip ordering (destructive op first, restore on failure). Sends
// nothing.
func applyMagicUnequip(userID id.UserID, slot DnDSlot) (magicUnequipOutcome, error) {
	equipped, err := loadEquippedMagicItems(userID)
	if err != nil {
		return magicUnequipOutcome{}, err
	}
	e, ok := equipped[slot]
	if !ok || e.Item.ID == "" {
		return magicUnequipOutcome{}, errSlotEmpty
	}
	if err := unequipMagicItem(userID, slot); err != nil {
		return magicUnequipOutcome{}, err
	}
	back := magicItemSellAt(e.Item, e.Temper)
	back.SkillSource = "magic_item:" + e.Item.ID
	if err := addAdvInventoryItem(userID, back); err != nil {
		if rbErr := equipMagicItem(userID, slot, e.Item.ID, e.Attuned, e.Temper); rbErr != nil {
			slog.Error("magic-item: unequip failed AND re-equip rollback failed",
				"user", userID, "item", e.Item.ID, "inv_err", err, "rollback_err", rbErr)
		}
		return magicUnequipOutcome{}, err
	}
	healed, _ := reconcileMagicAttunements(userID)
	return magicUnequipOutcome{Item: e.Effective(), Healed: healed}, nil
}

func (p *AdventurePlugin) handleEquipMagicCmd(ctx MessageContext) error {
	// Self-heal first: bond any worn item stranded inert while a slot is free
	// (e.g. equipped at cap, then a bond slot opened). This is the only path a
	// slotted-but-inert item can be reached from, since the picker below lists
	// inventory only.
	healed, _ := reconcileMagicAttunements(ctx.Sender)

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
	healNote := ""
	if len(healed) > 0 {
		healNote = fmt.Sprintf("🔗 A bond slot freed up — **%s** bonded and is now active.\n\n",
			strings.Join(healed, "**, **"))
	}

	if len(magic) == 0 {
		msg := "No curios to equip yet — they drop from zones or Luigi's 🔮 shelf."
		if healNote != "" {
			msg = strings.TrimSpace(healNote)
		}
		return p.SendDM(ctx.Sender, msg)
	}

	equipped, _ := loadEquippedMagicItems(ctx.Sender)
	var sb strings.Builder
	sb.WriteString(healNote)
	sb.WriteString("🔮 **Equippable magic items:**\n\n")
	for i, it := range magic {
		base, _ := magicItemFromAdvItem(it)
		mi := temperedItem(base, it.Temper)
		curDesc := "empty"
		if cur, ok := equipped[mi.Slot]; ok && cur.Item.ID != "" {
			curDesc = cur.Item.Name
		}
		att := ""
		if mi.Attunement {
			att = " _(needs bonding)_"
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** _(%s)_ → %s slot — currently: %s%s\n    %s\n",
			i+1, mi.Name, magicItemRarityLabel(rarityLootTierNum(mi.Rarity)), mi.Slot, curDesc, att, magicItemEffectSummary(mi)))
	}
	sb.WriteString(fmt.Sprintf("\nBonds in use: %d/%d\n",
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

	idx, ok := parseMenuIndex(reply, len(data.Items))
	if !ok {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Invalid selection. Reply with a number from the list, or \"cancel\".")
	}

	it := data.Items[idx]
	out, err := applyMagicEquip(ctx.Sender, it)
	if errors.Is(err, errItemNotEquippable) {
		return p.SendDM(ctx.Sender, "That item can't be equipped anymore.")
	}
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to equip that item.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✨ **%s** equipped in your %s slot — %s.",
		out.Effective.Name, out.Effective.Slot, magicItemEffectSummary(out.Effective)))
	if out.SwappedBack != "" {
		sb.WriteString(fmt.Sprintf("\n📦 **%s** moved back to inventory.", out.SwappedBack))
	}
	switch {
	case out.Bonded:
		sb.WriteString(fmt.Sprintf("\nBonded (%d/%d bond slots used).",
			out.BondsBefore+1, dndMagicItemAttuneLimit))
	case out.AtCap:
		sb.WriteString(fmt.Sprintf("\n⚠️ All %d bond slots are full — it's worn but **inert** until you free one (`!adventure unequip-magic` to take a bonded item off; this one bonds automatically once a slot opens).",
			dndMagicItemAttuneLimit))
	}
	if len(out.Healed) > 0 {
		sb.WriteString(fmt.Sprintf("\n🔗 A freed bond slot also activated **%s**.",
			strings.Join(out.Healed, "**, **")))
	}
	return p.SendDM(ctx.Sender, sb.String())
}

// ── Unequip command (`!adventure unequip-magic`) ─────────────────────────────

// advPendingMagicUnequip is the pending-interaction payload for the numbered
// unequip picker. It lists slots (not inventory rows) because the item lives in
// magic_item_equipped, not adventure_inventory.
type advPendingMagicUnequip struct {
	Slots []DnDSlot
}

func (p *AdventurePlugin) handleUnequipMagicCmd(ctx MessageContext) error {
	equipped, err := loadEquippedMagicItems(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your equipped magic items.")
	}
	var sb strings.Builder
	sb.WriteString("🔮 **Worn magic items** — reply with a number to take one off (it returns to your inventory), or \"cancel\".\n\n")
	var slots []DnDSlot
	for _, ds := range dndSlotOrder { // stable numbering across repeated calls
		e, ok := equipped[ds]
		if !ok || e.Item.ID == "" {
			continue
		}
		slots = append(slots, ds)
		mi := e.Effective()
		status := ""
		if mi.Attunement {
			if e.Attuned {
				status = " — bonded"
			} else {
				status = " — _(inert)_"
			}
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** _(%s)_ → %s slot%s\n",
			len(slots), mi.Name, mi.Rarity, ds, status))
	}
	if len(slots) == 0 {
		return p.SendDM(ctx.Sender, "You aren't wearing any magic items. `!adventure equip-magic` to put one on.")
	}

	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "magic_unequip",
		Data:      &advPendingMagicUnequip{Slots: slots},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	return p.SendDM(ctx.Sender, sb.String())
}

func (p *AdventurePlugin) resolveMagicUnequipReply(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingMagicUnequip)
	reply := strings.TrimSpace(ctx.Body)
	if strings.EqualFold(reply, "cancel") {
		return p.SendDM(ctx.Sender, "Unequip cancelled.")
	}
	idx, ok := parseMenuIndex(reply, len(data.Slots))
	if !ok {
		p.pending.Store(string(ctx.Sender), interaction)
		return p.SendDM(ctx.Sender, "Invalid selection. Reply with a number from the list, or \"cancel\".")
	}

	slot := data.Slots[idx]
	out, err := applyMagicUnequip(ctx.Sender, slot)
	if errors.Is(err, errSlotEmpty) {
		return p.SendDM(ctx.Sender, "That slot is already empty.")
	}
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to take that item off.")
	}

	msg := fmt.Sprintf("📦 **%s** taken off your %s slot and returned to inventory.", out.Item.Name, slot)
	// Freeing a bonded slot may let a worn-but-inert item finally bond.
	if len(out.Healed) > 0 {
		msg += fmt.Sprintf("\n🔗 That freed a bond slot — **%s** is now active.",
			strings.Join(out.Healed, "**, **"))
	}
	return p.SendDM(ctx.Sender, msg)
}
