package plugin

import "sort"

// Phase 11 D1a — zone registry. Implements `gogobee_dungeon_zones.md` §5.
//
// D1a (this file) ships zone *definitions* only — names, tiers, level
// ranges, enemy rosters by bestiary ID, boss stat block, and loot stubs.
// State machine (DungeonRun), commands (!zone enter/advance/etc), and
// TwinBee GM mood land in subsequent D1 sub-phases (D1b–D1d).
//
// Tier 1 ships first (Goblin Warrens, Crypt of Valdris). Tiers 2–5 are
// added in D2/D3/D4/D5. Loot table item IDs reference equipment/treasure
// IDs that may not yet exist in the equipment registry; that wiring
// happens at drop time, not at zone-definition time, so unknown IDs are
// not an error here — they're the contract that future content fulfils.

type ZoneID string

const (
	ZoneGoblinWarrens   ZoneID = "goblin_warrens"
	ZoneCryptValdris    ZoneID = "crypt_valdris"
	ZoneForestShadows   ZoneID = "forest_shadows"
	ZoneSunkenTemple    ZoneID = "sunken_temple"
	ZoneManorBlackspire ZoneID = "manor_blackspire"
	ZoneUnderforge      ZoneID = "underforge"
	ZoneUnderdark       ZoneID = "underdark"
	ZoneFeywildCrossing ZoneID = "feywild_crossing"
	ZoneDragonsLair     ZoneID = "dragons_lair"
	ZoneAbyssPortal     ZoneID = "abyss_portal"
)

// ZoneTier — 1..5. Player level ranges per design doc §2.
type ZoneTier int

const (
	ZoneTierBeginner   ZoneTier = 1
	ZoneTierApprentice ZoneTier = 2
	ZoneTierJourneyman ZoneTier = 3
	ZoneTierVeteran    ZoneTier = 4
	ZoneTierLegendary  ZoneTier = 5
)

// ZoneEnemy is a roster entry. BestiaryID must resolve via dndBestiary
// at spawn time (D1c+). SpawnWeight biases procedural room population —
// higher = appears more often in exploration rooms.
type ZoneEnemy struct {
	BestiaryID  string
	SpawnWeight int  // 1..10; default 5
	IsElite     bool // appears only in Elite rooms
}

// ZoneBoss — single boss for the zone. Multi-phase encounters expose
// PhaseTwoAt as a fraction of MaxHP (0.5 = phase 2 starts at 50% HP).
// Abilities is descriptive for now (D1a is data-only); D5+ wires real
// boss-ability hooks into the combat engine.
type ZoneBoss struct {
	BestiaryID  string  // canonical ID; bestiary entry added with the zone
	Name        string  // display name for the boss intro line
	CR          float32 // for tier validation only
	HP          int
	AC          int
	PhaseTwoAt  float64 // fraction of MaxHP triggering phase 2; 0 = no phase 2
	Abilities   []string
	Description string // one-line lore for boss-entry narration
}

// ZoneLootEntry — drop chance + item id. UniqueAlways marks story drops
// that always appear (e.g. quest items, phylactery shards).
type ZoneLootEntry struct {
	ItemID       string
	DropChance   float64 // 0..1; ignored if UniqueAlways
	UniqueAlways bool    // always drops; used for quest items
	Note         string  // free-text descriptor (unique-item flavor)
}

// ZoneDefinition — full zone descriptor. Room count is the design-doc's
// "6–8 rooms per run", scaled by tier.
type ZoneDefinition struct {
	ID         ZoneID
	Display    string
	Tier       ZoneTier
	LevelMin   int
	LevelMax   int
	Faction    string
	Atmosphere string // one-line biome description
	Hook       string // entry-room narration seed (TwinBee voice, italic)
	MinRooms   int
	MaxRooms   int
	Enemies    []ZoneEnemy
	Boss       ZoneBoss
	Loot       []ZoneLootEntry
	FlavorFile string // expected flavor file name (D6 deliverable)
}

// dndZoneRegistry — keyed by ZoneID. Tier 1 lands in D1a; remaining
// tiers join in D2/D3/D4/D5. zoneOrder preserves design-doc ordering.
var dndZoneRegistry = map[ZoneID]ZoneDefinition{}
var zoneOrder = []ZoneID{}

func registerZone(z ZoneDefinition) {
	if _, dup := dndZoneRegistry[z.ID]; dup {
		// Programmer error: duplicate definition. Panic at init makes
		// this surface immediately on test start rather than in prod.
		panic("duplicate zone registration: " + string(z.ID))
	}
	dndZoneRegistry[z.ID] = z
	zoneOrder = append(zoneOrder, z.ID)
}

func init() {
	registerZone(zoneGoblinWarrens())
	registerZone(zoneCryptValdris())
}

// getZone returns the definition by ID and ok=false if unknown.
func getZone(id ZoneID) (ZoneDefinition, bool) {
	z, ok := dndZoneRegistry[id]
	return z, ok
}

// zonesForLevel returns all zones a player at dndLevel may enter.
// Per design doc §2: a player cannot enter a zone more than 2 tiers
// above their current level. We approximate "current tier" as
// floor((level-1)/3)+1 so L1–3=T1, L4–6≈T2, etc., capping at T5.
// Result is sorted by tier ascending then declared order.
func zonesForLevel(dndLevel int) []ZoneDefinition {
	playerTier := (dndLevel-1)/3 + 1
	if playerTier < 1 {
		playerTier = 1
	}
	maxTier := playerTier + 2
	var out []ZoneDefinition
	for _, id := range zoneOrder {
		z := dndZoneRegistry[id]
		if int(z.Tier) <= maxTier {
			out = append(out, z)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Tier < out[j].Tier
	})
	return out
}

// zonesByTier — zones at exactly the given tier, in declared order.
func zonesByTier(t ZoneTier) []ZoneDefinition {
	var out []ZoneDefinition
	for _, id := range zoneOrder {
		if z := dndZoneRegistry[id]; z.Tier == t {
			out = append(out, z)
		}
	}
	return out
}

// allZones — every registered zone in declared (design-doc) order.
func allZones() []ZoneDefinition {
	out := make([]ZoneDefinition, 0, len(zoneOrder))
	for _, id := range zoneOrder {
		out = append(out, dndZoneRegistry[id])
	}
	return out
}

// ---- Tier 1 zone factories ---------------------------------------------------

func zoneGoblinWarrens() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneGoblinWarrens,
		Display:    "Goblin Warrens",
		Tier:       ZoneTierBeginner,
		LevelMin:   1,
		LevelMax:   3,
		Faction:    "Goblins, Hobgoblins",
		Atmosphere: "Low ceilings, torchlight, crude traps, cackling in the dark.",
		Hook:       "A network of fetid tunnels burrowed beneath the Merchant's Road. The smell arrives before the sounds — smoke, rot, and something worse. TwinBee advises keeping one hand on your blade.",
		MinRooms:   6,
		MaxRooms:   7,
		Enemies: []ZoneEnemy{
			{BestiaryID: "goblin_sneak", SpawnWeight: 7},
			{BestiaryID: "goblin_archer", SpawnWeight: 6},
			{BestiaryID: "hobgoblin_grunt", SpawnWeight: 5},
			{BestiaryID: "worg", SpawnWeight: 3},
			{BestiaryID: "goblin_shaman", SpawnWeight: 2},
			{BestiaryID: "hobgoblin_warchief", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_grol_unbroken",
			Name:        "Grol the Unbroken",
			CR:          3,
			HP:          65,
			AC:          17,
			PhaseTwoAt:  0,
			Description: "A Bugbear war-chief who has united three goblin clans under his banner. Wears a belt of troll teeth and smells like he earned them.",
			Abilities: []string{
				"Surprise Attack: +2d6 damage if player has not acted this combat",
				"Heart of Hruggek: crits deal max damage (no roll)",
				"Terrifying Roar (1/combat): allies +2 to hit for 2 turns; player WIS DC 13 or Frightened",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "wpn_handaxe_+1", DropChance: 0.15},
			{ItemID: "arm_studded_leather_+1", DropChance: 0.10},
			{ItemID: "grols_belt", DropChance: 0.05, Note: "+2 STR while equipped"},
			{ItemID: "coins_2d10x5", DropChance: 1.0, Note: "2d10 × 5 coins"},
		},
		FlavorFile: "zone_goblin_warrens_flavor.go",
	}
}

func zoneCryptValdris() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneCryptValdris,
		Display:    "The Crypt of Valdris",
		Tier:       ZoneTierBeginner,
		LevelMin:   1,
		LevelMax:   3,
		Faction:    "Undead",
		Atmosphere: "Stone corridors, dripping water, candles that shouldn't still be burning.",
		Hook:       "The iron gate hangs open — someone left in a hurry. Carved into the stone above: \"HERE LIES VALDRIS. DO NOT.\" The rest has been chiseled away. TwinBee declines to speculate.",
		MinRooms:   6,
		MaxRooms:   7,
		Enemies: []ZoneEnemy{
			{BestiaryID: "skeleton", SpawnWeight: 7},
			{BestiaryID: "zombie", SpawnWeight: 6},
			{BestiaryID: "shadow", SpawnWeight: 4},
			{BestiaryID: "specter", SpawnWeight: 3},
			{BestiaryID: "wight", SpawnWeight: 1, IsElite: true},
			{BestiaryID: "flameskull", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_valdris_unburied",
			Name:        "Valdris the Unburied",
			CR:          5,
			HP:          97,
			AC:          17,
			PhaseTwoAt:  0.50,
			Description: "A lich-aspirant who got most of the way there. Cunning enough to be dangerous, failed enough to be bitter.",
			Abilities: []string{
				"Corrupting Touch: +4d6 necrotic; target max HP reduced by damage dealt (long rest restores)",
				"Legendary Resistance (2/combat): auto-succeed one failed save",
				"Call of the Grave (recharge 5–6): summons 1d4 skeletons from room bones",
				"Phase 2 (<50% HP): Fly speed; spells deal +1d6 necrotic",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "valdris_phylactery_shard", UniqueAlways: true, Note: "quest item"},
			{ItemID: "arm_cloak_of_protection", DropChance: 0.12},
			{ItemID: "wpn_mace_of_smiting", DropChance: 0.08},
			{ItemID: "coins_3d10x4", DropChance: 1.0, Note: "3d10 × 4 coins"},
		},
		FlavorFile: "zone_crypt_valdris_flavor.go",
	}
}
