package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Resource & Combat Integration §5 — Zone-Contextual Loot Tables.
//
// Drops fire on combat victory (room/elite/boss/harvest-interrupt/patrol).
// Drop tier is selected from enemy CR per the §5 bracket table; the item
// itself is rolled from a per-zone slate so a goblin warrens uncommon
// reads differently than an underdark uncommon. Sell values follow the
// §8.1 rarity brackets so R5's `!sell` can pay out cleanly later.
//
// New file lives next to dnd_zone_loot.go; no existing files touched
// outside the combat-resolution call sites that invoke `dropZoneLoot`.

// LootTier is the dropped-item rarity bracket selected by enemy CR.
type LootTier string

const (
	LootTierCommon    LootTier = "common"
	LootTierUncommon  LootTier = "uncommon"
	LootTierRare      LootTier = "rare"
	LootTierEpic      LootTier = "epic"
	LootTierLegendary LootTier = "legendary"
)

// ZoneLootDrop is one item slot in a zone's loot table.
type ZoneLootDrop struct {
	Name      string
	ItemType  string // "consumable", "scroll", "trinket", "gear", "trophy", "currency"
	BaseValue int    // §8.1 midpoint
}

// rarityFor maps the loot tier to a DnDRarity (kept separate so the
// §8.1 sell brackets line up cleanly with the equipment rarity scheme).
func (t LootTier) rarity() DnDRarity {
	switch t {
	case LootTierCommon:
		return RarityCommon
	case LootTierUncommon:
		return RarityUncommon
	case LootTierRare:
		return RarityRare
	case LootTierEpic:
		return RarityEpic
	case LootTierLegendary:
		return RarityLegendary
	}
	return RarityCommon
}

// dropTierFromCR implements the §5 enemy-CR → drop-tier table.
//   - guaranteed: tier that always drops if a drop fires.
//   - bonus:     a higher-tier upgrade roll (empty = no upgrade roll).
//   - bonusPct:  chance the drop is upgraded to `bonus`.
//   - dropChance: probability the slot drops at all (1.0 = guaranteed slot).
func dropTierFromCR(cr float32, isBoss bool) (guaranteed LootTier, bonus LootTier, bonusPct float64, dropChance float64) {
	if isBoss {
		// §5: bosses minimum Rare, with a small Epic upgrade.
		return LootTierRare, LootTierEpic, 0.25, 1.0
	}
	switch {
	case cr >= 13:
		return LootTierEpic, LootTierLegendary, 0.05, 1.0
	case cr >= 9:
		return LootTierRare, LootTierEpic, 0.10, 1.0
	case cr >= 5:
		return LootTierUncommon, LootTierRare, 0.15, 1.0
	case cr >= 2:
		return LootTierCommon, LootTierUncommon, 0.30, 1.0
	default: // CR 0–1
		return LootTierCommon, "", 0.0, 0.70
	}
}

// zoneLootTables — full §5 registry: 10 zones × 5 tiers, biome-flavored.
// Common slots lean on shared exploration items (rations, scrap, salve)
// re-skinned per zone; rares pull from each zone's signature gear; the
// legendary row is single-entry per zone and reads as a trophy.
var zoneLootTables = map[ZoneID]map[LootTier][]ZoneLootDrop{
	ZoneGoblinWarrens: {
		LootTierCommon: {
			{Name: "Goblin Field Ration", ItemType: "consumable", BaseValue: 8},
			{Name: "Stolen Coin Pouch", ItemType: "currency", BaseValue: 12},
			{Name: "Worg-Tooth Trinket", ItemType: "trinket", BaseValue: 10},
		},
		LootTierUncommon: {
			{Name: "Hobgoblin Captain's Insignia", ItemType: "trophy", BaseValue: 45},
			{Name: "Crude Alchemy Vial", ItemType: "consumable", BaseValue: 35},
			{Name: "Goblin-Forged Shortblade", ItemType: "gear", BaseValue: 50},
		},
		LootTierRare: {
			{Name: "War Standard Banner", ItemType: "trophy", BaseValue: 200},
			{Name: "Shaman's Carved Stave", ItemType: "gear", BaseValue: 220},
		},
		LootTierEpic: {
			{Name: "Hobgoblin General's Cuirass", ItemType: "gear", BaseValue: 800},
		},
		LootTierLegendary: {
			{Name: "King Grobnar's Iron Crown", ItemType: "trophy", BaseValue: 4500},
		},
	},
	ZoneCryptValdris: {
		LootTierCommon: {
			{Name: "Tarnished Burial Coin", ItemType: "currency", BaseValue: 10},
			{Name: "Cracked Bone Charm", ItemType: "trinket", BaseValue: 8},
			{Name: "Funeral-Wrap Linen", ItemType: "trinket", BaseValue: 9},
		},
		LootTierUncommon: {
			{Name: "Pre-Empire Signet Ring", ItemType: "trinket", BaseValue: 50},
			{Name: "Necrotic-Etched Dagger", ItemType: "gear", BaseValue: 55},
			{Name: "Sealed Funerary Vial", ItemType: "consumable", BaseValue: 40},
		},
		LootTierRare: {
			{Name: "Valdris Scriptorium Tablet", ItemType: "trophy", BaseValue: 240},
			{Name: "Silver-Inlaid Reliquary", ItemType: "trinket", BaseValue: 220},
		},
		LootTierEpic: {
			{Name: "Crypt-Lord's Burial Mantle", ItemType: "gear", BaseValue: 850},
		},
		LootTierLegendary: {
			{Name: "Valdris's Sealing Sigil", ItemType: "trophy", BaseValue: 5000},
		},
	},
	ZoneForestShadows: {
		LootTierCommon: {
			{Name: "Foraged Trail Loaf", ItemType: "consumable", BaseValue: 9},
			{Name: "Carved Wooden Talisman", ItemType: "trinket", BaseValue: 11},
			{Name: "Bundle of Shadow Reed", ItemType: "trinket", BaseValue: 8},
		},
		LootTierUncommon: {
			{Name: "Hunter's Bone Bow Charm", ItemType: "trinket", BaseValue: 50},
			{Name: "Dryad-Touched Salve", ItemType: "consumable", BaseValue: 45},
			{Name: "Owlbear-Talon Knife", ItemType: "gear", BaseValue: 55},
		},
		LootTierRare: {
			{Name: "Fey-Stained Scroll Case", ItemType: "scroll", BaseValue: 230},
			{Name: "Elder Druid's Walking Stave", ItemType: "gear", BaseValue: 240},
		},
		LootTierEpic: {
			{Name: "Greenwarden's Ironbark Vest", ItemType: "gear", BaseValue: 900},
		},
		LootTierLegendary: {
			{Name: "Heart-Wood of the First Grove", ItemType: "trophy", BaseValue: 5500},
		},
	},
	ZoneSunkenTemple: {
		LootTierCommon: {
			{Name: "Salt-Crusted Coin", ItemType: "currency", BaseValue: 9},
			{Name: "Pearl Sliver", ItemType: "trinket", BaseValue: 12},
			{Name: "Pressure-Sealed Vial", ItemType: "consumable", BaseValue: 11},
		},
		LootTierUncommon: {
			{Name: "Kuo-toa Spear-Head", ItemType: "gear", BaseValue: 50},
			{Name: "Coral-Set Pendant", ItemType: "trinket", BaseValue: 55},
			{Name: "Tide-Blessed Censer", ItemType: "trinket", BaseValue: 45},
		},
		LootTierRare: {
			{Name: "Drowned Acolyte's Tome", ItemType: "scroll", BaseValue: 240},
			{Name: "Aboleth-Glass Lens", ItemType: "trinket", BaseValue: 250},
		},
		LootTierEpic: {
			{Name: "Tide-Caller's Trident Head", ItemType: "gear", BaseValue: 950},
		},
		LootTierLegendary: {
			{Name: "Dar'eth's Drowned Crown", ItemType: "trophy", BaseValue: 6000},
		},
	},
	ZoneManorBlackspire: {
		LootTierCommon: {
			{Name: "Tarnished Silver Spoon", ItemType: "trinket", BaseValue: 10},
			{Name: "Forty-Year-Old Tincture", ItemType: "consumable", BaseValue: 12},
			{Name: "Mourner's Cameo", ItemType: "trinket", BaseValue: 11},
		},
		LootTierUncommon: {
			{Name: "Vampire-Hunter's Stake Set", ItemType: "gear", BaseValue: 55},
			{Name: "Cursed Family Locket", ItemType: "trinket", BaseValue: 50},
			{Name: "Estate-Sealed Wine Bottle", ItemType: "consumable", BaseValue: 45},
		},
		LootTierRare: {
			{Name: "Blackspire Family Diary", ItemType: "scroll", BaseValue: 240},
			{Name: "Ghost-Iron Candelabrum", ItemType: "trinket", BaseValue: 230},
		},
		LootTierEpic: {
			{Name: "Manor-Lord's Funeral Coat", ItemType: "gear", BaseValue: 950},
		},
		LootTierLegendary: {
			{Name: "Blackspire Patriarch's Death Mask", ItemType: "trophy", BaseValue: 6000},
		},
	},
	ZoneUnderforge: {
		LootTierCommon: {
			{Name: "Forge-Slag Trinket", ItemType: "trinket", BaseValue: 11},
			{Name: "Iron-Filing Pouch", ItemType: "trinket", BaseValue: 9},
			{Name: "Dwarven Hardtack", ItemType: "consumable", BaseValue: 10},
		},
		LootTierUncommon: {
			{Name: "Azer-Forged Hammer Head", ItemType: "gear", BaseValue: 55},
			{Name: "Heat-Etched Sigil Plate", ItemType: "trinket", BaseValue: 50},
			{Name: "Salamander-Oil Flask", ItemType: "consumable", BaseValue: 50},
		},
		LootTierRare: {
			{Name: "Master-Smith's Ledger Page", ItemType: "scroll", BaseValue: 250},
			{Name: "Mithral-Thread Gauntlet", ItemType: "gear", BaseValue: 280},
		},
		LootTierEpic: {
			{Name: "Forge-Marshal's Anvil-Charm", ItemType: "trinket", BaseValue: 1000},
		},
		LootTierLegendary: {
			{Name: "The First Hammer's Striking Head", ItemType: "trophy", BaseValue: 7000},
		},
	},
	ZoneUnderdark: {
		LootTierCommon: {
			{Name: "Cave-Spider Silk Skein", ItemType: "trinket", BaseValue: 11},
			{Name: "Dim-Glow Mushroom Bundle", ItemType: "consumable", BaseValue: 10},
			{Name: "Drow-Worked Coin", ItemType: "currency", BaseValue: 12},
		},
		LootTierUncommon: {
			{Name: "Drow Hand-Crossbow Bolt Case", ItemType: "gear", BaseValue: 55},
			{Name: "Faerzress-Etched Charm", ItemType: "trinket", BaseValue: 50},
			{Name: "Diluted Drow Sleep Vial", ItemType: "consumable", BaseValue: 50},
		},
		LootTierRare: {
			{Name: "Mind Flayer Codex Page", ItemType: "scroll", BaseValue: 270},
			{Name: "Drow-Adamantine Bracer", ItemType: "gear", BaseValue: 290},
		},
		LootTierEpic: {
			{Name: "Matron's House-Sigil Brooch", ItemType: "trinket", BaseValue: 1100},
		},
		LootTierLegendary: {
			{Name: "Eyeless King's Skull-Crown", ItemType: "trophy", BaseValue: 7500},
		},
	},
	ZoneFeywildCrossing: {
		LootTierCommon: {
			{Name: "Pressed Fey Petal", ItemType: "trinket", BaseValue: 12},
			{Name: "Honey-Wax Candle", ItemType: "consumable", BaseValue: 10},
			{Name: "Dream-Spun Thread", ItemType: "trinket", BaseValue: 11},
		},
		LootTierUncommon: {
			{Name: "Pixie-Wing Glassine", ItemType: "trinket", BaseValue: 55},
			{Name: "Wisp-Lantern Charm", ItemType: "trinket", BaseValue: 60},
			{Name: "Hag-Brewed Bitter Cordial", ItemType: "consumable", BaseValue: 50},
		},
		LootTierRare: {
			{Name: "Time-Locked Music Box", ItemType: "trinket", BaseValue: 280},
			{Name: "Fey Court Invitation Scroll", ItemType: "scroll", BaseValue: 260},
		},
		LootTierEpic: {
			{Name: "Summer-Knight's Thorn Buckler", ItemType: "gear", BaseValue: 1100},
		},
		LootTierLegendary: {
			{Name: "Thornmother's Heart-Thorn Reliquary", ItemType: "trophy", BaseValue: 8000},
		},
	},
	ZoneDragonsLair: {
		LootTierCommon: {
			{Name: "Half-Melted Gold Coin", ItemType: "currency", BaseValue: 13},
			{Name: "Volcanic Glass Sliver", ItemType: "trinket", BaseValue: 11},
			{Name: "Kobold Field Pouch", ItemType: "consumable", BaseValue: 10},
		},
		LootTierUncommon: {
			{Name: "Drake-Scale Bracer", ItemType: "gear", BaseValue: 55},
			{Name: "Kobold Trap-Mechanism", ItemType: "trinket", BaseValue: 50},
			{Name: "Drake-Blood Cordial", ItemType: "consumable", BaseValue: 50},
		},
		LootTierRare: {
			{Name: "Hoard-Inventory Page", ItemType: "scroll", BaseValue: 280},
			{Name: "Dragonfire-Forged Spike", ItemType: "gear", BaseValue: 290},
		},
		LootTierEpic: {
			{Name: "Wyrmguard's Cinder-Cloak Clasp", ItemType: "trinket", BaseValue: 1150},
		},
		LootTierLegendary: {
			{Name: "Infernax's Tooth-Reliquary", ItemType: "trophy", BaseValue: 9000},
		},
	},
	ZoneAbyssPortal: {
		LootTierCommon: {
			{Name: "Brimstone-Caked Coin", ItemType: "currency", BaseValue: 13},
			{Name: "Sulfur-Wax Stub", ItemType: "consumable", BaseValue: 11},
			{Name: "Demon-Ichor Smear-Vial", ItemType: "trinket", BaseValue: 12},
		},
		LootTierUncommon: {
			{Name: "Quasit-Bound Ring", ItemType: "trinket", BaseValue: 55},
			{Name: "Corrupted-Metal Buckle", ItemType: "gear", BaseValue: 50},
			{Name: "Planar-Shard Pendant", ItemType: "trinket", BaseValue: 55},
		},
		LootTierRare: {
			{Name: "Abyssal-Marked Codex Sheet", ItemType: "scroll", BaseValue: 290},
			{Name: "Marilith-Steel Edge", ItemType: "gear", BaseValue: 300},
		},
		LootTierEpic: {
			{Name: "Nalfeshnee's Honor-Sigil", ItemType: "trinket", BaseValue: 1200},
		},
		LootTierLegendary: {
			{Name: "Belaxath's Bound Truename Tablet", ItemType: "trophy", BaseValue: 10000},
		},
	},
}

// rollZoneLoot rolls a single combat-victory drop. Returns ok=false when
// the drop chance fails (low-CR misses) or the zone has no table.
//
// Resolution order:
//  1. Pick guaranteed/bonus tier from CR via dropTierFromCR.
//  2. If dropChance fails → no drop.
//  3. Bonus upgrade roll → bonusPct chance to step up the tier.
//  4. Pick a uniform random entry from the zone's slate at that tier;
//     fall back down a tier if the slate is empty.
func rollZoneLoot(zoneID ZoneID, cr float32, isBoss bool, rng *rand.Rand) (ZoneLootDrop, LootTier, bool) {
	guaranteed, bonus, bonusPct, dropChance := dropTierFromCR(cr, isBoss)
	if rngFloat(rng) >= dropChance {
		return ZoneLootDrop{}, "", false
	}
	tier := guaranteed
	if bonus != "" && rngFloat(rng) < bonusPct {
		tier = bonus
	}
	zone, ok := zoneLootTables[zoneID]
	if !ok {
		return ZoneLootDrop{}, "", false
	}
	if entry, ok := pickLootEntry(zone, tier, rng); ok {
		return entry, tier, true
	}
	// Fallback: walk down tiers if the chosen one is empty.
	for _, fallback := range []LootTier{LootTierEpic, LootTierRare, LootTierUncommon, LootTierCommon} {
		if fallback == tier {
			continue
		}
		if entry, ok := pickLootEntry(zone, fallback, rng); ok {
			return entry, fallback, true
		}
	}
	return ZoneLootDrop{}, "", false
}

func pickLootEntry(zone map[LootTier][]ZoneLootDrop, tier LootTier, rng *rand.Rand) (ZoneLootDrop, bool) {
	slate := zone[tier]
	if len(slate) == 0 {
		return ZoneLootDrop{}, false
	}
	idx := rngIntN(rng, len(slate))
	return slate[idx], true
}

// rngFloat / rngIntN: nil-safe wrappers so tests can pass a seeded rng
// while production paths use the package-global generator.
func rngFloat(rng *rand.Rand) float64 {
	if rng == nil {
		return rand.Float64()
	}
	return rng.Float64()
}

func rngIntN(rng *rand.Rand, n int) int {
	if n <= 0 {
		return 0
	}
	if rng == nil {
		return rand.IntN(n)
	}
	return rng.IntN(n)
}

// dropZoneLoot is the combat-resolution call site. Rolls a drop, deposits
// it into adventure_inventory if one fired, and returns a narration block
// (empty when no drop). Reuses §5 LootDropCommon/Uncommon/Rare/Legendary
// flavor pools — no new flavor file (per feedback_reuse_existing_flavor).
// magicItemDropChance — once a drop fires, the chance it is substituted with
// an Open5e SRD registry magic item of the rolled tier's rarity instead of
// the biome slate item. Keeps magic items a treat, not the norm.
const magicItemDropChance = 0.15

// Treasure-roll weights by the kind of fight that earned the roll. The base
// rates (advTreasureDropRates, 1.5%→0.15%) are tuned for one roll per day of
// legacy activity; expedition combat rolls far more often, so standard kills
// stay at ×1 and the weight goes to the moments a player remembers.
const (
	advTreasureWeightStandard  = 1.0
	advTreasureWeightElite     = 2.0
	advTreasureWeightBoss      = 4.0
	advTreasureWeightZoneClear = 1.0
)

// advLocForZone adapts a zone into the AdvLocation shape the treasure and
// masterwork systems were written against, back when every drop came from a
// legacy daily activity. Zones are dungeons.
func advLocForZone(zoneID ZoneID) *AdvLocation {
	name := string(zoneID)
	if z, ok := getZone(zoneID); ok {
		name = z.Display
	}
	return &AdvLocation{
		Name:     name,
		Activity: AdvActivityDungeon,
		Tier:     zoneTierFromID(zoneID),
	}
}

// rollZoneTreasure gives the treasure table a shot at a zone moment. Silent
// on the overwhelmingly common no-drop path; checkTreasureDrop owns all
// player-facing output (discovery DM, auto-swap, T5 room announce).
func (p *AdventurePlugin) rollZoneTreasure(userID id.UserID, zoneID ZoneID, weight float64) {
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return
	}
	p.checkTreasureDrop(userID, char, advLocForZone(zoneID), weight)
}

// rollZoneMasterwork gives the masterwork catalog a shot at an elite or boss
// kill. Arena gear stays the premium line (×1.5 effective tier vs ×1.25), so
// this competes with the shop, not with the arena.
func (p *AdventurePlugin) rollZoneMasterwork(userID id.UserID, zoneID ZoneID, isBoss bool) {
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return
	}
	outcome := AdvOutcomeSuccess
	if isBoss {
		outcome = AdvOutcomeExceptional
	}
	p.checkMasterworkDrop(userID, equip, advLocForZone(zoneID), outcome)
}

// advIngredientDropChance — per-win chance a crafting ingredient drops.
//
// Every recipe ingredient in adventure_consumables.go lives in the legacy
// gathering tables (advDungeonLoot/advMiningLoot/advForagingLoot/
// advFishingLoot), which the Phase R transition left with no live caller:
// generateAdvLoot is only reachable from resolveDungeonAction, and nothing
// calls that. Rather than re-key 24 ingredients into the per-zone slates,
// zone combat draws from the legacy tables directly at the zone's tier —
// their values are already tuned, and every recipe becomes craftable again.
const advIngredientDropChance = 0.15

// advIngredientActivities are the four legacy tables a zone kill can draw an
// ingredient from. Zone tier indexes the table tier directly.
var advIngredientActivities = []AdvActivityType{
	AdvActivityDungeon, AdvActivityMining, AdvActivityForaging, AdvActivityFishing,
}

// rollZoneIngredient draws one crafting ingredient from a random legacy
// gathering table at the zone's tier. Returns nil on the common no-drop path.
func rollZoneIngredient(zoneTier int) *AdvItem {
	chance := advIngredientDropChance
	if m := activeOmen().ConsumableChanceMult; m > 1.0 { // N7/B3 the Omen
		chance *= m
	}
	if rand.Float64() >= chance {
		return nil
	}
	act := advIngredientActivities[rand.IntN(len(advIngredientActivities))]
	defs := advLootTable(act)[zoneTier]
	if len(defs) == 0 {
		return nil
	}
	d := defs[rand.IntN(len(defs))]
	value := d.MinValue
	if d.MaxValue > d.MinValue {
		value += rand.Int64N(d.MaxValue - d.MinValue + 1)
	}
	return &AdvItem{
		Name:        d.Name,
		Type:        d.Type,
		Tier:        zoneTier,
		Value:       value,
		SkillSource: "zone_ingredient:" + string(act),
	}
}

// grantZoneItem deposits a rolled item and returns its narration line.
func (p *AdventurePlugin) grantZoneItem(userID id.UserID, item *AdvItem, icon string) string {
	if err := addAdvInventoryItem(userID, *item); err != nil {
		slog.Error("zone loot: failed to add item", "user", userID, "item", item.Name, "err", err)
		return ""
	}
	return fmt.Sprintf("%s **%s** — %d coin baseline.", icon, item.Name, item.Value)
}

func (p *AdventurePlugin) dropZoneLoot(userID id.UserID, zoneID ZoneID, monster DnDMonsterTemplate, isBoss, isElite bool) string {
	// Treasure rolls on every win, independent of whether the zone loot
	// table produced an item — the two systems are separate rewards.
	weight := advTreasureWeightStandard
	switch {
	case isBoss:
		weight = advTreasureWeightBoss
	case isElite:
		weight = advTreasureWeightElite
	}
	p.rollZoneTreasure(userID, zoneID, weight)

	// Masterwork is an elite/boss reward only — it's a permanent gear
	// upgrade, not a per-kill trinket.
	if isBoss || isElite {
		p.rollZoneMasterwork(userID, zoneID, isBoss)
	}

	// Consumables and ingredients roll on any win, and independently of the
	// zone slate below — a dry slate roll shouldn't cost the player these.
	zTier := zoneTierFromID(zoneID)
	var extra []string
	if item := RollConsumableDrop(AdvActivityDungeon, zTier); item != nil {
		if line := p.grantZoneItem(userID, item, "🧪"); line != "" {
			extra = append(extra, line)
		}
	}
	if item := rollZoneIngredient(zTier); item != nil {
		if line := p.grantZoneItem(userID, item, "🌿"); line != "" {
			extra = append(extra, line)
		}
	}
	// The Hollow King campaign (N5/D1). Elites carry the scattered pages; the
	// gating boss gets an epilogue instead (D1b), not a page.
	if isElite {
		if line := p.maybeDropJournalPage(userID, nil); line != "" {
			extra = append(extra, line)
		}
	}
	trailer := strings.Join(extra, "\n")

	entry, tier, ok := rollZoneLoot(zoneID, monster.CR, isBoss, nil)
	if !ok {
		return trailer
	}

	// Magic-item substitution: swap the biome slate item for a registry
	// magic item of the same rarity tier (see magic_items_gameplay.go).
	if rngFloat(nil) < magicItemDropChance {
		if mi, miOK := pickMagicItemForRarity(tier.rarity(), nil); miOK {
			return joinLootLines(p.dropMagicItemLoot(userID, mi, tier), trailer)
		}
	}

	tierVal := tier
	item := AdvItem{
		Name:        entry.Name,
		Type:        entry.ItemType,
		Tier:        zTier,
		Value:       int64(entry.BaseValue),
		SkillSource: fmt.Sprintf("zone_loot:%s:%s", zoneID, tierVal),
	}
	if err := addAdvInventoryItem(userID, item); err != nil {
		return joinLootLines(fmt.Sprintf("_(Loot drop persistence error: %v.)_", err), trailer)
	}
	var b strings.Builder
	if line := lootFlavorLine(tierVal); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	icon := lootIcon(tierVal)
	desc := zoneItemDescription(zoneID, entry.Name)
	if desc != "" {
		b.WriteString(fmt.Sprintf("%s **%s** (%s) — %d coin baseline. _%s_",
			icon, entry.Name, tierVal, entry.BaseValue, desc))
	} else {
		b.WriteString(fmt.Sprintf("%s **%s** (%s) — %d coin baseline.",
			icon, entry.Name, tierVal, entry.BaseValue))
	}
	return joinLootLines(b.String(), trailer)
}

// joinLootLines stitches the zone-slate narration together with the
// consumable/ingredient trailer, tolerating either being empty.
func joinLootLines(parts ...string) string {
	var kept []string
	for _, s := range parts {
		if s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "\n")
}

// dropMagicItemLoot deposits a registry magic item as a combat-victory drop
// and returns its narration block. Potions/scrolls land as "consumable" so
// the combat pipeline auto-uses them; everything else is a "magic_item"
// sellable/equippable via the Curios shelf and `!adventure equip-magic`.
func (p *AdventurePlugin) dropMagicItemLoot(userID id.UserID, mi MagicItem, tier LootTier) string {
	item := magicItemSell(mi)
	item.SkillSource = "magic_item:" + mi.ID
	if err := addAdvInventoryItem(userID, item); err != nil {
		// Players see a tidy line; the raw err goes to the log so we can
		// still chase the bug without leaking SQL into the chat.
		slog.Error("magic-item: loot drop persist failed",
			"user", userID, "item", mi.ID, "err", err)
		return "_(The drop slips through your fingers — try again later.)_"
	}
	var b strings.Builder
	if line := lootFlavorLine(tier); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	// Pretty-print the rarity (very_rare → Very Rare) and call attunement
	// what it does, not what 5e named it. Potions/scrolls earn their own
	// auto-use tag so players don't think they need to do something with it.
	tag := magicItemRarityLabel(rarityLootTierNum(mi.Rarity))
	if mi.Attunement {
		tag += " · needs bonding"
	}
	if mi.Kind == MagicItemPotion || mi.Kind == MagicItemScroll {
		tag += " · auto-uses in combat"
	}
	b.WriteString(fmt.Sprintf("✨ **%s** _(%s)_ — %d coin baseline.",
		mi.Name, tag, mi.Value))
	if mi.Desc != "" {
		b.WriteString(fmt.Sprintf(" _%s_", mi.Desc))
	}
	return b.String()
}

// lootFlavorLine maps a tier to one of the existing TwinBee loot pools.
// Epic falls through to the Rare pool (next-best fit); Legendary uses its
// own pool. No new flavor file written here.
func lootFlavorLine(tier LootTier) string {
	switch tier {
	case LootTierCommon:
		return flavor.Pick(flavor.LootDropCommon)
	case LootTierUncommon:
		return flavor.Pick(flavor.LootDropUncommon)
	case LootTierRare, LootTierEpic:
		return flavor.Pick(flavor.LootDropRare)
	case LootTierLegendary:
		return flavor.Pick(flavor.LootDropLegendary)
	}
	return ""
}

func lootIcon(tier LootTier) string {
	switch tier {
	case LootTierCommon:
		return "🎁"
	case LootTierUncommon:
		return "🎁✨"
	case LootTierRare:
		return "💎"
	case LootTierEpic:
		return "💎✨"
	case LootTierLegendary:
		return "👑"
	}
	return "🎁"
}

// zoneItemDescription is the §5 zone-contextual item flavor table.
// Populated for Potion of Healing across all 10 zones (the design
// doc's exemplar) plus a per-item flavor matrix for the entries each
// zone's loot table actually drops (R4b polish). Falls back to "" so
// callers render the bare name when no flavor is on file.
func zoneItemDescription(zoneID ZoneID, itemName string) string {
	if itemName == "Potion of Healing" {
		return potionOfHealingZoneFlavor[zoneID]
	}
	if zoneMap, ok := zoneItemFlavor[zoneID]; ok {
		if line, ok := zoneMap[itemName]; ok {
			return line
		}
	}
	return ""
}

// zoneItemFlavor — per-zone, per-item flavor for the zone loot tables
// above. Keys must match `ZoneLootDrop.Name` exactly. Items not listed
// here render with no italicized flavor line, just the name + tier +
// coin baseline. Tone target: matches the §5 Potion-of-Healing
// exemplar — short, in-character, biome-distinctive.
var zoneItemFlavor = map[ZoneID]map[string]string{
	ZoneGoblinWarrens: {
		"Goblin Field Ration":          "Wrapped in burlap. Smells like onions, fear, and someone's last meal.",
		"Stolen Coin Pouch":            "Mismatched currency, three different empires represented. The goblins didn't discriminate.",
		"Worg-Tooth Trinket":           "Strung on sinew. Still has bone fragments embedded in the gum-line.",
		"Hobgoblin Captain's Insignia": "Bronze, dented, and unmistakably stolen from somewhere it mattered.",
		"Crude Alchemy Vial":           "The label is in goblin shorthand. The contents fizz when you tilt it.",
		"Goblin-Forged Shortblade":     "Edge holds. Balance is wrong. Built by hand, not by a smith.",
		"War Standard Banner":          "Heraldry of three warbands stitched over each other. The bottom layer is older than the empire.",
		"Shaman's Carved Stave":        "Knotwood, rune-burned. Hums faintly when you stop paying attention to it.",
		"Hobgoblin General's Cuirass":  "Plate-and-leather, embossed with a sigil that's been scraped halfway off and re-stamped.",
		"King Grobnar's Iron Crown":    "Three sizes too large for any goblin who ever lived. Whatever wore this first wasn't a goblin.",
	},
	ZoneCryptValdris: {
		"Tarnished Burial Coin":      "Stamped with a face nobody alive remembers. Heavier than it should be.",
		"Cracked Bone Charm":         "Wrist-strung. Whoever wore it didn't make it out.",
		"Funeral-Wrap Linen":         "Brittle, brown, and still smelling faintly of myrrh.",
		"Pre-Empire Signet Ring":     "The crest pre-dates any standing kingdom. The metal is still warm somehow.",
		"Necrotic-Etched Dagger":     "Black-veined steel. Cold to hold, colder to put down.",
		"Sealed Funerary Vial":       "Wax-stoppered. Whatever's inside has had centuries to think about itself.",
		"Valdris Scriptorium Tablet": "Stone, carved in a script that splits opinion among the few scholars who can read it.",
		"Silver-Inlaid Reliquary":    "Empty now. Someone took whatever was inside and didn't replace it.",
		"Crypt-Lord's Burial Mantle": "Heavy embroidery. The threads aren't thread.",
		"Valdris's Sealing Sigil":    "It hums. The Crypt is older than the empire. This is older than the Crypt.",
	},
	ZoneForestShadows: {
		"Foraged Trail Loaf":            "Dense, dark, and faintly bitter. Travelers' bread, baked under a canopy that doesn't quite let light through.",
		"Carved Wooden Talisman":        "Birch, knife-shaped, etched with a ward against something the carver wouldn't name.",
		"Bundle of Shadow Reed":         "Cut along the streambank. Smells like cold water and rot.",
		"Hunter's Bone Bow Charm":       "Small. Femur-bone of something none of the local hunters will identify.",
		"Dryad-Touched Salve":           "Pine-resin base. Heals fast. Tastes like sap if you're foolish enough to try.",
		"Owlbear-Talon Knife":           "Talon set into a wood handle. The talon is still sharp and the wood is older than the kill.",
		"Fey-Stained Scroll Case":       "The leather has faint iridescence under direct light. Don't ask where it was tanned.",
		"Elder Druid's Walking Stave":   "Living wood, still budding. Doesn't take kindly to being indoors.",
		"Greenwarden's Ironbark Vest":   "Bark-plated, woven with sapling fibre. The forest grew this for someone.",
		"Heart-Wood of the First Grove": "A palm-sized chunk of something that pulses if you hold it long enough. Don't.",
	},
	ZoneSunkenTemple: {
		"Salt-Crusted Coin":          "The denomination is unreadable but the metal is real silver. Bring it to Thom Krooke; he knows the era.",
		"Pearl Sliver":               "Fragment, not a whole pearl. Something cracked it open trying to eat what was inside.",
		"Pressure-Sealed Vial":       "Cold to touch. Pop the seal and it whistles air for a full second before it stops.",
		"Kuo-toa Spear-Head":         "Bone-and-shell composite. Wickedly barbed. Still smells like the deep.",
		"Coral-Set Pendant":          "The coral is bleached except where it touched the wearer's skin.",
		"Tide-Blessed Censer":        "Brass, pitted. Still holds a faint trace of incense and old prayer.",
		"Drowned Acolyte's Tome":     "Pages stuck together. The ones that separate cleanly are the ones you should read carefully.",
		"Aboleth-Glass Lens":         "Looks ordinary. Look through it and you see things ten feet to the left of where they actually are.",
		"Tide-Caller's Trident Head": "The barbs are folded inward. Whoever forged this didn't want it pulled out clean.",
		"Dar'eth's Drowned Crown":    "Coral-encrusted gold. Fits no human skull comfortably.",
	},
	ZoneManorBlackspire: {
		"Tarnished Silver Spoon":            "Estate-marked. The set it belonged to is presumably still in a drawer somewhere upstairs.",
		"Forty-Year-Old Tincture":           "Label dated 1486. Still potent. The Blackspires kept good apothecaries.",
		"Mourner's Cameo":                   "A widow's profile, ivory-on-jet. Likeness unknown; mood unmistakable.",
		"Vampire-Hunter's Stake Set":        "Three stakes, one mallet, all hawthorn. The mallet has been used.",
		"Cursed Family Locket":              "Don't open it. (You'll open it. They always do.)",
		"Estate-Sealed Wine Bottle":         "Wax intact. The label's faded but the vintage is legible — and good.",
		"Blackspire Family Diary":           "Penmanship is neat for the first hundred pages and then becomes other things.",
		"Ghost-Iron Candelabrum":            "Cold even when lit. The flames lean toward you regardless of where you stand.",
		"Manor-Lord's Funeral Coat":         "Tailored. Mourning-black. Smells faintly of formaldehyde and rosewater.",
		"Blackspire Patriarch's Death Mask": "Plaster, eyes closed. The expression is wrong for someone who died in their sleep.",
	},
	ZoneUnderforge: {
		"Forge-Slag Trinket":               "A cooled drip of something that used to be molten. Dwarves keep these. Nobody knows why.",
		"Iron-Filing Pouch":                "Heavy for its size. Magnetic. Useful, somehow, to someone, eventually.",
		"Dwarven Hardtack":                 "You could break a tooth on this. The dwarves who made it would consider that a feature.",
		"Azer-Forged Hammer Head":          "Still warm. Will be warm tomorrow. Will be warm a hundred years from now.",
		"Heat-Etched Sigil Plate":          "The runes were burned in by hand. The hand that did it was not a hand that minded heat.",
		"Salamander-Oil Flask":             "Sealed in glass. The oil ripples even when nothing else does.",
		"Master-Smith's Ledger Page":       "Old dwarven shorthand. Inventory of a forge that stopped recording entries the year the elementals woke up.",
		"Mithral-Thread Gauntlet":          "Light enough to be wrong. Sharper than it has any right to be along the knuckles.",
		"Forge-Marshal's Anvil-Charm":      "A miniature anvil on a chain. Heavy enough to be impractical. Worn anyway.",
		"The First Hammer's Striking Head": "The dwarven creation-myths name this. They don't agree on which dwarf swung it.",
	},
	ZoneUnderdark: {
		"Cave-Spider Silk Skein":       "Wound on a bone-spool. Tensile strength embarrassing to surface looms.",
		"Dim-Glow Mushroom Bundle":     "Tied with hair. Edible. Slightly hallucinogenic. Drow eat them as a snack.",
		"Drow-Worked Coin":             "Etched on both sides. The faces are not faces.",
		"Drow Hand-Crossbow Bolt Case": "Six bolts. Two are tipped with something dark. Two are tipped with something darker. Two are bare.",
		"Faerzress-Etched Charm":       "Magic-disrupting. Useful as a hex-breaker; useless inside an active wardline.",
		"Diluted Drow Sleep Vial":      "House-watered for trade with the surface. Still drops a person flat in twenty seconds.",
		"Mind Flayer Codex Page":       "The script reads itself into your head if you stare too long. Don't.",
		"Drow-Adamantine Bracer":       "Forged in fae-fire. Black with a violet undersheen. Doesn't tarnish.",
		"Matron's House-Sigil Brooch":  "Eight-legged crest. The matron who wore it is presumably no longer wearing it.",
		"Eyeless King's Skull-Crown":   "Bone-set, with sockets where stones should go. Worn smooth by something patient.",
	},
	ZoneFeywildCrossing: {
		"Pressed Fey Petal":                   "Color shifts when you blink. Smells like a season you don't have a name for.",
		"Honey-Wax Candle":                    "Burns blue. The wax pools upward.",
		"Dream-Spun Thread":                   "Spider silk except it sings if you pluck it.",
		"Pixie-Wing Glassine":                 "Pressed wing-membrane between two slips of glass. Iridescence depends on who is watching.",
		"Wisp-Lantern Charm":                  "Glows softly when held; goes dark when set down. Doesn't ask permission to do either.",
		"Hag-Brewed Bitter Cordial":           "Black, syrupy, and tastes exactly like one specific regret.",
		"Time-Locked Music Box":               "Plays a tune that's three notes longer than the box should physically be able to hold.",
		"Fey Court Invitation Scroll":         "Sealed in honeycomb wax. The invitation is for a party that hasn't happened yet, or already did.",
		"Summer-Knight's Thorn Buckler":       "Thorn-bossed, briar-rimmed. The thorns flex when struck and then re-form.",
		"Thornmother's Heart-Thorn Reliquary": "Sealed bronze, briar-wrapped. The thorn inside still pulses on a slow rhythm.",
	},
	ZoneDragonsLair: {
		"Half-Melted Gold Coin":          "Slumped flat by heat. Still gold. Still legal tender if Thom Krooke can read the year.",
		"Volcanic Glass Sliver":          "Sharper than steel and a quarter the weight. Edges chip on impact.",
		"Kobold Field Pouch":             "Trail rations, a fire-starter, two prayer-beads to a god named Kurtulmak.",
		"Drake-Scale Bracer":             "Overlapping scales, still attached to a strip of original drake-hide. Heat-resistant.",
		"Kobold Trap-Mechanism":          "Pre-built, single-use, and surprisingly clever. Read the inscribed instructions before activating.",
		"Drake-Blood Cordial":            "Distilled fire. Burns going down. Fire-resistance for an hour and a hangover for a day.",
		"Hoard-Inventory Page":           "An accountant's record of items the dragon owns. The dragon does not know it lost this page.",
		"Dragonfire-Forged Spike":        "Blackened steel, never cools. Driven into stone with the hilt-end first.",
		"Wyrmguard's Cinder-Cloak Clasp": "Brass-and-obsidian. The clasp is dragonshape. The wearer was, briefly, a problem.",
		"Infernax's Tooth-Reliquary":     "A single tooth, mounted in gold-and-bone. The dragon is younger by exactly this tooth.",
	},
	ZoneAbyssPortal: {
		"Brimstone-Caked Coin":             "Sulfur-yellow at the edges. Burns the nose if you sniff it. (You won't sniff it twice.)",
		"Sulfur-Wax Stub":                  "Half-burned demon-tallow. Doesn't smell like tallow.",
		"Demon-Ichor Smear-Vial":           "Stoppered tight. The contents move on their own when you turn away.",
		"Quasit-Bound Ring":                "Tiny, plain, and humming. Don't put it on. There's something in there.",
		"Corrupted-Metal Buckle":           "Was steel once. Is something else now. The shape held; the metal didn't.",
		"Planar-Shard Pendant":             "Looks like a fragment of a much larger reality. Held wrong, it cuts where there's nothing to cut.",
		"Abyssal-Marked Codex Sheet":       "Vellum, demon-scribed. The script reads in a voice that isn't yours.",
		"Marilith-Steel Edge":              "Six-bladed weapon-fragment, broken from a longer edge. The break is centuries old. The edge is sharp.",
		"Nalfeshnee's Honor-Sigil":         "A demon's idea of dignity. Brass, blood-stained, embossed with a name you can't pronounce.",
		"Belaxath's Bound Truename Tablet": "Stone, demon-runed. The truename is bound by inscription. Read it and the binding holds. Speak it and it doesn't.",
	},
}

// potionOfHealingZoneFlavor — §5 Zone Loot Flavor by Biome, verbatim.
var potionOfHealingZoneFlavor = map[ZoneID]string{
	ZoneGoblinWarrens:   "A stolen vial, still stoppered with a rag. It smells like goblin and better days.",
	ZoneCryptValdris:    "A glass vial embedded in a burial offering. Whoever left it didn't need it.",
	ZoneForestShadows:   "Bark-sealed, filled with something deep red. It tastes like the forest before it went wrong.",
	ZoneSunkenTemple:    "Salt-rimed and pressure-sealed. A temple offering, preserved by the deep cold.",
	ZoneManorBlackspire: "Found in the medicine cabinet, dated forty years ago. Remarkably, still effective.",
	ZoneUnderforge:      "Hot to the touch even now. The dwarves made things to last.",
	ZoneUnderdark:       "Drow-made. The formula is different. The effect is the same. The ingredients are not discussed.",
	ZoneFeywildCrossing: "It shifts color when you look at it sideways. The fey make medicine the way they make everything: beautifully and with caveats.",
	ZoneDragonsLair:     "A kobold medic's field kit, looted from a belt pouch. The kobolds take care of their own.",
	ZoneAbyssPortal:     "Glows faintly red. Still works. I recommend not asking what's in it.",
}
