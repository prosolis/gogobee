package plugin

// Resource & Combat Integration §3 — Zone resource tables.
// Per-zone harvestable materials/items, indexed by harvest action.
// Section 3 of gogobee_resource_combat_integration.md is the source of truth;
// when adding a new resource, update both the spec and this registry.

// HarvestAction identifies which command harvests a resource.
type HarvestAction string

const (
	HarvestForage   HarvestAction = "forage"
	HarvestMine     HarvestAction = "mine"
	HarvestScavenge HarvestAction = "scavenge"
	HarvestEssence  HarvestAction = "essence"
	HarvestCommune  HarvestAction = "commune"
	HarvestFish     HarvestAction = "fish"
)

// ZoneResource defines a harvestable material or item for a zone (§7).
// MaxCharges = node-count-per-region for the per-region MVP scope.
type ZoneResource struct {
	ID         string
	Name       string
	ZoneID     ZoneID
	Action     HarvestAction
	DC         int
	Rarity     DnDRarity
	Type       string // "material", "item", "fish", "quest"
	SellValue  int    // base coins at Thom Krooke (§8.1 midpoint)
	MaxCharges int    // node depletion budget per region per long-rest cycle
	// RequiresKill: monster kind that must be defeated in the region first
	// (empty = no prerequisite). Resolved against expedition kill-log when
	// combat-link wires in; for now treated as "never available" if non-empty
	// unless RegionState records the kill.
	RequiresKill string
	// ClassRestrict: empty for any-class; otherwise only this class can pull
	// the resource (mind flayer brain matter, drow poison post-kill, etc.).
	ClassRestrict DnDClass
	// ConditionalEvent: zone-event ID that gates availability (e.g. tidal
	// crystals only during Day 6 Tidal Event). Empty = always available.
	ConditionalEvent string
}

// zoneResources is the full Section 3 registry.
var zoneResources = map[ZoneID][]ZoneResource{
	ZoneGoblinWarrens: {
		{ID: "scrap_iron", Name: "Scrap Iron", Action: HarvestMine, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 10, MaxCharges: 2},
		{ID: "alchemy_pouch", Name: "Goblin Alchemy Pouch", Action: HarvestScavenge, DC: 13, Rarity: RarityUncommon, Type: "material", SellValue: 40, MaxCharges: 1},
		{ID: "worg_pelt", Name: "Worg Pelt", Action: HarvestScavenge, DC: 11, Rarity: RarityUncommon, Type: "material", SellValue: 35, MaxCharges: 1, RequiresKill: "worg"},
		{ID: "crude_weapon_cache", Name: "Crude Weapon Cache", Action: HarvestScavenge, DC: 15, Rarity: RarityUncommon, Type: "item", SellValue: 50, MaxCharges: 1},
		{ID: "war_standard", Name: "Hobgoblin War Standard", Action: HarvestScavenge, DC: 18, Rarity: RarityRare, Type: "material", SellValue: 200, MaxCharges: 1},
		{ID: "shaman_reagents", Name: "Goblin Shaman's Reagents", Action: HarvestEssence, DC: 17, Rarity: RarityRare, Type: "material", SellValue: 180, MaxCharges: 1, ClassRestrict: ClassMage},
	},
	ZoneCryptValdris: {
		{ID: "grave_soil", Name: "Grave Soil", Action: HarvestScavenge, DC: 9, Rarity: RarityCommon, Type: "material", SellValue: 8, MaxCharges: 2},
		{ID: "bone_dust", Name: "Bone Dust", Action: HarvestScavenge, DC: 9, Rarity: RarityCommon, Type: "material", SellValue: 7, MaxCharges: 2},
		{ID: "ancient_coin", Name: "Ancient Coin (pre-Empire)", Action: HarvestScavenge, DC: 14, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1},
		{ID: "necrotic_essence", Name: "Necrotic Essence", Action: HarvestEssence, DC: 15, Rarity: RarityUncommon, Type: "material", SellValue: 45, MaxCharges: 1},
		{ID: "silver_grave_dust", Name: "Silver Grave Dust", Action: HarvestCommune, DC: 17, Rarity: RarityRare, Type: "material", SellValue: 200, MaxCharges: 1, ClassRestrict: ClassCleric},
		{ID: "valdris_seal_fragment", Name: "Valdris's Seal Fragment", Action: HarvestScavenge, DC: 20, Rarity: RarityRare, Type: "quest", SellValue: 250, MaxCharges: 1},
	},
	ZoneForestShadows: {
		{ID: "shadow_herb", Name: "Shadow Herb", Action: HarvestForage, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 10, MaxCharges: 2},
		{ID: "beast_pelt", Name: "Beast Pelt", Action: HarvestForage, DC: 11, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 1, RequiresKill: "beast"},
		{ID: "darkwood", Name: "Darkwood", Action: HarvestMine, DC: 13, Rarity: RarityUncommon, Type: "material", SellValue: 40, MaxCharges: 1},
		{ID: "fey_bloom", Name: "Corrupted Fey Bloom", Action: HarvestForage, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1},
		{ID: "owlbear_feather", Name: "Owlbear Feather", Action: HarvestScavenge, DC: 12, Rarity: RarityUncommon, Type: "material", SellValue: 35, MaxCharges: 1, RequiresKill: "owlbear"},
		{ID: "dryads_tears", Name: "Dryad's Tears", Action: HarvestEssence, DC: 18, Rarity: RarityRare, Type: "material", SellValue: 220, MaxCharges: 1},
		{ID: "biolum_fungi", Name: "Bioluminescent Fungi", Action: HarvestForage, DC: 17, Rarity: RarityRare, Type: "material", SellValue: 200, MaxCharges: 1},
	},
	ZoneSunkenTemple: {
		{ID: "sea_glass", Name: "Sea Glass Shard", Action: HarvestScavenge, DC: 9, Rarity: RarityCommon, Type: "material", SellValue: 7, MaxCharges: 2},
		{ID: "deep_coral", Name: "Deep Coral", Action: HarvestMine, DC: 11, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 1},
		{ID: "kuotoa_scale", Name: "Kuo-toa Scale", Action: HarvestScavenge, DC: 12, Rarity: RarityUncommon, Type: "material", SellValue: 35, MaxCharges: 1, RequiresKill: "kuo-toa"},
		{ID: "black_pearl", Name: "Black Pearl", Action: HarvestFish, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1},
		{ID: "aboleth_mucus", Name: "Aboleth Mucus (refined)", Action: HarvestEssence, DC: 18, Rarity: RarityRare, Type: "material", SellValue: 230, MaxCharges: 1, ClassRestrict: ClassMage},
		{ID: "temple_relic", Name: "Ancient Temple Relic", Action: HarvestScavenge, DC: 20, Rarity: RarityRare, Type: "item", SellValue: 250, MaxCharges: 1},
		{ID: "tidal_crystal", Name: "Tidal Crystal", Action: HarvestMine, DC: 17, Rarity: RarityRare, Type: "material", SellValue: 200, MaxCharges: 1, ConditionalEvent: "tidal_event"},
	},
	ZoneManorBlackspire: {
		{ID: "spectral_grave_dust", Name: "Grave Dust (spectral)", Action: HarvestScavenge, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 2},
		{ID: "cursed_trinket", Name: "Cursed Trinket", Action: HarvestScavenge, DC: 12, Rarity: RarityCommon, Type: "item", SellValue: 14, MaxCharges: 1},
		{ID: "ghost_silk", Name: "Ghost Silk", Action: HarvestEssence, DC: 15, Rarity: RarityUncommon, Type: "material", SellValue: 45, MaxCharges: 1},
		{ID: "vampire_ash", Name: "Vampire Ash", Action: HarvestScavenge, DC: 14, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1, RequiresKill: "vampire_spawn"},
		{ID: "shadow_essence", Name: "Shadow Essence", Action: HarvestEssence, DC: 18, Rarity: RarityRare, Type: "material", SellValue: 220, MaxCharges: 1},
		{ID: "blackspire_volume", Name: "Blackspire Library Volume", Action: HarvestScavenge, DC: 19, Rarity: RarityRare, Type: "item", SellValue: 240, MaxCharges: 1},
		{ID: "wraith_core", Name: "Wraith Core", Action: HarvestCommune, DC: 20, Rarity: RarityRare, Type: "material", SellValue: 260, MaxCharges: 1, ClassRestrict: ClassCleric, RequiresKill: "wraith"},
	},
	ZoneUnderforge: {
		{ID: "iron_ore", Name: "Iron Ore", Action: HarvestMine, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 10, MaxCharges: 2},
		{ID: "volcanic_glass", Name: "Volcanic Glass", Action: HarvestMine, DC: 11, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 2},
		{ID: "azer_ingot", Name: "Azer Metal Ingot", Action: HarvestMine, DC: 14, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1, RequiresKill: "azer"},
		{ID: "fire_essence", Name: "Fire Essence", Action: HarvestEssence, DC: 15, Rarity: RarityUncommon, Type: "material", SellValue: 45, MaxCharges: 1},
		{ID: "mithral_trace", Name: "Mithral Trace", Action: HarvestMine, DC: 20, Rarity: RarityRare, Type: "material", SellValue: 280, MaxCharges: 1},
		{ID: "forge_core_fragment", Name: "Forge Core Fragment", Action: HarvestScavenge, DC: 17, Rarity: RarityRare, Type: "material", SellValue: 200, MaxCharges: 1, RequiresKill: "construct"},
		{ID: "salamander_oil", Name: "Salamander Oil", Action: HarvestEssence, DC: 16, Rarity: RarityRare, Type: "material", SellValue: 210, MaxCharges: 1, RequiresKill: "salamander"},
	},
	ZoneUnderdark: {
		{ID: "ud_mushroom", Name: "Underdark Mushroom", Action: HarvestForage, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 10, MaxCharges: 2},
		{ID: "spider_silk", Name: "Cave Spider Silk", Action: HarvestForage, DC: 12, Rarity: RarityCommon, Type: "material", SellValue: 14, MaxCharges: 2},
		{ID: "drow_poison", Name: "Drow Poison (diluted)", Action: HarvestScavenge, DC: 14, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1, RequiresKill: "drow"},
		{ID: "faerzress_crystal", Name: "Faerzress Crystal", Action: HarvestMine, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1},
		{ID: "mind_flayer_brain", Name: "Mind Flayer Brain Matter", Action: HarvestEssence, DC: 19, Rarity: RarityRare, Type: "material", SellValue: 250, MaxCharges: 1, ClassRestrict: ClassMage, RequiresKill: "illithid"},
		{ID: "drow_adamantine", Name: "Drow Adamantine", Action: HarvestMine, DC: 21, Rarity: RarityRare, Type: "material", SellValue: 290, MaxCharges: 1},
		{ID: "deep_dragon_chip", Name: "Deep Dragon Scale Chip", Action: HarvestScavenge, DC: 23, Rarity: RarityVeryRare, Type: "material", SellValue: 800, MaxCharges: 1},
	},
	ZoneFeywildCrossing: {
		{ID: "fey_dust", Name: "Fey Dust", Action: HarvestForage, DC: 11, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 2},
		{ID: "enchanted_petal", Name: "Enchanted Petal", Action: HarvestForage, DC: 12, Rarity: RarityCommon, Type: "material", SellValue: 13, MaxCharges: 2},
		{ID: "pixie_wing", Name: "Pixie Wing Fragments", Action: HarvestEssence, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1},
		{ID: "wisp_essence", Name: "Will-o-Wisp Essence", Action: HarvestEssence, DC: 17, Rarity: RarityUncommon, Type: "material", SellValue: 60, MaxCharges: 1, RequiresKill: "will_o_wisp"},
		{ID: "hag_hair", Name: "Hag Hair", Action: HarvestCommune, DC: 18, Rarity: RarityRare, Type: "material", SellValue: 220, MaxCharges: 1, ClassRestrict: ClassCleric, RequiresKill: "hag"},
		{ID: "timelock_amber", Name: "Timelock Amber", Action: HarvestForage, DC: 21, Rarity: RarityRare, Type: "material", SellValue: 290, MaxCharges: 1},
		{ID: "thornmother_thorn", Name: "Thornmother's Thorn", Action: HarvestScavenge, DC: 25, Rarity: RarityVeryRare, Type: "material", SellValue: 1000, MaxCharges: 1, RequiresKill: "thornmother"},
	},
	ZoneDragonsLair: {
		{ID: "ancient_gold", Name: "Ancient Gold Coin", Action: HarvestScavenge, DC: 10, Rarity: RarityCommon, Type: "material", SellValue: 12, MaxCharges: 2},
		{ID: "volcanic_gem", Name: "Volcanic Gem", Action: HarvestMine, DC: 12, Rarity: RarityCommon, Type: "material", SellValue: 13, MaxCharges: 2},
		{ID: "drake_blood", Name: "Drake Blood", Action: HarvestEssence, DC: 14, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1, RequiresKill: "drake"},
		{ID: "dragon_scale_fragment", Name: "Dragon Scale Fragment", Action: HarvestScavenge, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1, RequiresKill: "drake"},
		{ID: "dragonfire_coal", Name: "Dragonfire Coal", Action: HarvestMine, DC: 19, Rarity: RarityRare, Type: "material", SellValue: 240, MaxCharges: 1},
		{ID: "kobold_trap", Name: "Kobold Trap Mechanism", Action: HarvestScavenge, DC: 18, Rarity: RarityRare, Type: "item", SellValue: 220, MaxCharges: 1},
		{ID: "infernax_scale", Name: "Infernax Scale", Action: HarvestScavenge, DC: 25, Rarity: RarityLegendary, Type: "material", SellValue: 6000, MaxCharges: 1, RequiresKill: "infernax"},
	},
	ZoneAbyssPortal: {
		{ID: "brimstone_shard", Name: "Brimstone Shard", Action: HarvestMine, DC: 11, Rarity: RarityCommon, Type: "material", SellValue: 13, MaxCharges: 2},
		{ID: "demon_ichor", Name: "Demon Ichor", Action: HarvestScavenge, DC: 12, Rarity: RarityCommon, Type: "material", SellValue: 14, MaxCharges: 2, RequiresKill: "demon"},
		{ID: "planar_shard", Name: "Planar Shard", Action: HarvestMine, DC: 15, Rarity: RarityUncommon, Type: "material", SellValue: 50, MaxCharges: 1},
		{ID: "corrupted_metal", Name: "Corrupted Metal", Action: HarvestScavenge, DC: 16, Rarity: RarityUncommon, Type: "material", SellValue: 55, MaxCharges: 1},
		{ID: "void_essence", Name: "Void Essence", Action: HarvestEssence, DC: 20, Rarity: RarityRare, Type: "material", SellValue: 280, MaxCharges: 1},
		{ID: "abyssal_ichor", Name: "Abyssal Ichor (concentrated)", Action: HarvestEssence, DC: 22, Rarity: RarityRare, Type: "material", SellValue: 300, MaxCharges: 1, ClassRestrict: ClassMage, RequiresKill: "balor"},
		{ID: "portal_fragment", Name: "Portal Fragment", Action: HarvestScavenge, DC: 25, Rarity: RarityLegendary, Type: "material", SellValue: 6500, MaxCharges: 1, RequiresKill: "portal_boss"},
	},
}

// init backfills derived fields (ZoneID) so callers don't have to set them
// twice in the literal.
func init() {
	for zid, list := range zoneResources {
		for i := range list {
			list[i].ZoneID = zid
		}
		zoneResources[zid] = list
	}
}

// ZoneResourcesByAction returns all resources in zoneID gathered by action.
// Returns nil if the zone has no entries (single-zone-not-yet-populated case).
func ZoneResourcesByAction(zoneID ZoneID, action HarvestAction) []ZoneResource {
	all := zoneResources[zoneID]
	out := make([]ZoneResource, 0, len(all))
	for _, r := range all {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

// ZoneResourcesAll returns the full registry slice for zoneID.
func ZoneResourcesAll(zoneID ZoneID) []ZoneResource {
	return zoneResources[zoneID]
}

// FindZoneResource looks up a single resource by zone+id. Returns ok=false
// if no match.
func FindZoneResource(zoneID ZoneID, resourceID string) (ZoneResource, bool) {
	for _, r := range zoneResources[zoneID] {
		if r.ID == resourceID {
			return r, true
		}
	}
	return ZoneResource{}, false
}
