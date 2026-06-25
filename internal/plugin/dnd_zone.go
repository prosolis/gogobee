package plugin

import (
	"fmt"
	"sort"
)

// Phase 11 D1a — zone registry. Implements `gogobee_dungeon_zones.md` §5.
//
// D1a (this file) ships zone *definitions* only — names, tiers, level
// ranges, enemy rosters by bestiary ID, boss stat block, and loot stubs.
// State machine (DungeonRun), commands (!zone enter/advance/etc), and
// TwinBee DM mood land in subsequent D1 sub-phases (D1b–D1d).
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

	// ZoneArena is a synthetic ID — never registered via registerZone, so
	// !zone enter can't reach it. It exists to route renderBossOutcome's
	// twinBeeLine calls when an arena fight (resolveArenaBoss, post-L2)
	// reuses the boss-shaped narrative path. The Nat20/Nat1/BossDeath/
	// PlayerDeath pools are generic, so ZoneArena falls through the
	// existing zoneRoomEntryPool / bossEntryPool / zoneLorePool /
	// bossPhaseTwoPool switches to the right defaults.
	ZoneArena ZoneID = "arena"
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
	Hook       string // entry-room narration seed (I voice, italic)
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
	registerZone(zoneForestShadows())
	registerZone(zoneSunkenTemple())
	registerZone(zoneManorBlackspire())
	registerZone(zoneUnderforge())
	registerZone(zoneUnderdark())
	registerZone(zoneFeywildCrossing())
	registerZone(zoneDragonsLair())
	registerZone(zoneAbyssPortal())
}

// getZone returns the definition by ID and ok=false if unknown.
func getZone(id ZoneID) (ZoneDefinition, bool) {
	z, ok := dndZoneRegistry[id]
	return z, ok
}

// zoneOrFallback returns the registered ZoneDefinition or, if the id
// is unknown (corrupted DB row, dropped registration), a safe
// placeholder so display strings don't panic. Use this at non-fatal
// callsites that just need a Display/Hook/Atmosphere to print.
func zoneOrFallback(id ZoneID) ZoneDefinition {
	if z, ok := dndZoneRegistry[id]; ok {
		return z
	}
	return ZoneDefinition{
		ID:         id,
		Display:    fmt.Sprintf("(unknown zone %q)", string(id)),
		Hook:       "",
		Atmosphere: "",
		Tier:       1,
		LevelMin:   1,
		LevelMax:   1,
	}
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
		Hook:       "A network of fetid tunnels burrowed beneath the Merchant's Road. The smell arrives before the sounds — smoke, rot, and something worse. I advise keeping one hand on your blade.",
		MinRooms:   12,
		MaxRooms:   14,
		Enemies: []ZoneEnemy{
			{BestiaryID: "goblin_sneak", SpawnWeight: 7},
			{BestiaryID: "goblin_archer", SpawnWeight: 6},
			{BestiaryID: "hobgoblin_grunt", SpawnWeight: 5},
			{BestiaryID: "worg", SpawnWeight: 3, IsElite: true},
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
		Hook:       "The iron gate hangs open — someone left in a hurry. Carved into the stone above: \"HERE LIES VALDRIS. DO NOT.\" The rest has been chiseled away. I decline to speculate.",
		MinRooms:   12,
		MaxRooms:   14,
		Enemies: []ZoneEnemy{
			// Phase 4-B (outlier fix): the live roster was the only
			// dual-killer-elite zone (wight + flameskull, both CR3+ on
			// the T1 standards floor), so the InterruptElite bracket
			// was effectively "pick your one-shot." Phase 2c skipped
			// this zone because it was already dual-elite; Phase 4-A's
			// per-monster trace caught the consequence (167/183
			// deaths attributed to those two). Flameskull dropped here
			// (still in underforge T3 where its CR fits); specter
			// promoted from standard to the soft-elite dilutor slot
			// at SW=3, matching Phase 2c's worg/owlbear shape.
			{BestiaryID: "skeleton", SpawnWeight: 7},
			{BestiaryID: "zombie", SpawnWeight: 6},
			{BestiaryID: "shadow", SpawnWeight: 4},
			{BestiaryID: "specter", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "wight", SpawnWeight: 1, IsElite: true},
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

// ---- Tier 2 zone factories (Phase 11 D3a) -----------------------------------

func zoneForestShadows() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneForestShadows,
		Display:    "Forest of Shadows",
		Tier:       ZoneTierApprentice,
		LevelMin:   3,
		LevelMax:   5,
		Faction:    "Beasts, Fey-corrupted creatures, Bandits",
		Atmosphere: "Ancient forest, twisted paths, eerie silence, bioluminescent fungi, things in the canopy.",
		Hook:       "The forest was beautiful once. Travelers still say so, usually right before they stop saying anything at all. The trees lean in when you're not looking. I have noted this is not a metaphor.",
		MinRooms:   16,
		MaxRooms:   20,
		Enemies: []ZoneEnemy{
			// Phase 4-B (outlier fix): standard pool was carrying two
			// real killers — Displacer Beast (38% win as a standard
			// pick) and Bandit Captain (57% win). Re-flag Displacer
			// Beast as elite so it leaves the standard slot, joining
			// Owlbear/Hag as a third elite for pool dilution; trim
			// Bandit Captain SW so the soft Dire Wolf carries more of
			// the standard roll mass. Boss + standards now match the
			// sibling sunken_temple's lethality shape.
			//
			// Second pass: a first attempt (Displacer→elite alone)
			// only moved comp 0% → 0.5%. Phase 4-A re-trace showed
			// Bandit Captain still bleeding the standard pool (321
			// appearances, 54.8% win, 53 kills) and Owlbear monopolising
			// the elite roll at SW=4. Promoted Captain to elite as
			// well; cut Owlbear SW 4→2 so the 4-elite pool dilutes
			// instead of front-loading Owlbear (9% win). Standard
			// becomes Dire Wolf + Dryad, matching sunken_temple's
			// "all standards ≥90% win" floor.
			{BestiaryID: "dire_wolf", SpawnWeight: 6},
			{BestiaryID: "dryad_corrupted", SpawnWeight: 3},
			{BestiaryID: "owlbear", SpawnWeight: 2, IsElite: true},
			{BestiaryID: "bandit_captain", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "displacer_beast", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "green_hag", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_hollow_king",
			Name:        "The Hollow King",
			CR:          6,
			HP:          142,
			AC:          15,
			PhaseTwoAt:  0.40,
			Description: "What was once a forest guardian, now a vessel for something older and angrier. The antlers are real. The eyes are not.",
			Abilities: []string{
				"Corrupting Aura: melee-range targets WIS DC 14 each turn or lose bonus action",
				"Root Surge (recharge 5–6): Restrain (STR DC 15) + 2d8 bludgeoning",
				"Devour Light: extinguishes magical light for 2 turns; player AC -2",
				"Phase 2 (<40% HP): summons 2 Dire Wolves; gains Reckless Attack",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "the_hollow_crown", DropChance: 0.06, Note: "+2 WIS; Fey Sight (advantage on Perception in nature)"},
			{ItemID: "arm_hide_+2", DropChance: 0.10},
			{ItemID: "wpn_longbow_+1", DropChance: 0.12},
			{ItemID: "forest_essence", UniqueAlways: true, Note: "crafting material x1–3"},
		},
		FlavorFile: "zone_forest_shadows_flavor.go",
	}
}

func zoneSunkenTemple() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneSunkenTemple,
		Display:    "Sunken Temple of Dar'eth",
		Tier:       ZoneTierApprentice,
		LevelMin:   3,
		LevelMax:   5,
		Faction:    "Kuo-toa, Water Elementals, Aboleth-touched",
		Atmosphere: "Flooded stone chambers, barnacled pillars, salt smell, alien glyphs, things that swim in the dark water.",
		Hook:       "The tide went out thirty years ago and never fully came back. The temple stayed wet anyway. Something down there keeps it that way. I suggest waterproofing your spellbook.",
		MinRooms:   16,
		MaxRooms:   20,
		Enemies: []ZoneEnemy{
			{BestiaryID: "kuo_toa", SpawnWeight: 7},
			{BestiaryID: "kuo_toa_whip", SpawnWeight: 4},
			{BestiaryID: "merrow", SpawnWeight: 4},
			{BestiaryID: "aboleth_thrall", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "water_elemental", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_dreaming_aboleth",
			Name:        "The Dreaming Aboleth",
			CR:          10,
			HP:          135,
			AC:          17,
			PhaseTwoAt:  0,
			Description: "Ancient. Patient. It has been waiting in this temple since before the city above was built. It has had time to plan.",
			Abilities: []string{
				"Tentacle Multiattack: 3 hits; on-hit Diseased (no magical healing 24h until cured)",
				"Enslave (recharge 6): WIS DC 14 or Charmed; player skips turn, drifts toward Aboleth",
				"Mucus Cloud: melee attackers CON DC 14 or skin→membrane (6d6 acid if not submerged)",
				"Legendary Actions (3/round): Detect / Tail Swipe (2 LA) / Psychic Drain (3 LA, max-HP cut)",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "aboleths_eye", DropChance: 0.05, Note: "+2 INT; Telepathy (passive lore checks auto-succeed)"},
			{ItemID: "arm_breastplate_+1", DropChance: 0.10},
			{ItemID: "refined_aboleth_mucus", UniqueAlways: true, Note: "rare crafting material x1"},
			{ItemID: "coins_5d10x8", DropChance: 1.0, Note: "5d10 × 8 coins"},
		},
		FlavorFile: "zone_sunken_temple_flavor.go",
	}
}

// ---- Tier 3 zone factories (Phase 11 D4a) -----------------------------------

func zoneManorBlackspire() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneManorBlackspire,
		Display:    "Haunted Manor of Blackspire",
		Tier:       ZoneTierJourneyman,
		LevelMin:   5,
		LevelMax:   8,
		Faction:    "Undead, Shadows, Vampiric",
		Atmosphere: "Victorian decay, impossible architecture, portraits whose eyes follow movement, cold spots, locked rooms that weren't locked before.",
		Hook:       "The manor has been for sale for eleven years. Every buyer has either left immediately or not left at all. The real estate listing describes it as 'full of character.' I find this accurate.",
		MinRooms:   22,
		MaxRooms:   26,
		Enemies: []ZoneEnemy{
			// Phase 4-B (outlier fix): Wraith was the dominant
			// standard-pool killer (45 hp loss/win, 85 attributed
			// kills in Phase 4-A) — too punishing for the standard
			// slot at T3. Promoted to elite; Spawn+Wraith+Revenant
			// becomes a 3-elite pool with balanced dilution, leaving
			// Shadow/Poltergeist/Banshee as a clean standard floor
			// (all ≥92% win in the trace).
			//
			// Phase 5-C: under shipped HP×1.5/+3 floor, manor still
			// trailed at 53% (band 55-75). Trace named both Vampire
			// Spawn (72% win, 42 kills) and Revenant (16% win, 31
			// kills) as elite-pool killers — Revenant the worse per
			// appearance, but at SW=1 already. A first attempt
			// trimmed Vampire Spawn SW 3 → 2; that backfired by
			// shifting elite share onto Revenant (kills jumped to
			// 48). Final move: keep the killers' weights, dilute
			// the elite pool by promoting Banshee (99.6% win as
			// standard, 11hp loss) to elite at SW=2 — drops
			// Revenant's elite share ~14% → 11% without removing
			// it from rotation. Standard floor collapses to
			// Shadow + Poltergeist (both ≥99% win, ≤5hp loss).
			{BestiaryID: "shadow", SpawnWeight: 6},
			{BestiaryID: "poltergeist", SpawnWeight: 5},
			{BestiaryID: "banshee", SpawnWeight: 2, IsElite: true},
			{BestiaryID: "wraith", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "vampire_spawn", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "revenant", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_aldric_blackspire",
			Name:        "Lord Aldric Blackspire",
			CR:          13,
			HP:          144,
			AC:          16,
			PhaseTwoAt:  0,
			Description: "He has watched seven generations of his bloodline die in this house. He considers himself the only survivor. He may be right.",
			Abilities: []string{
				"Multiattack: Unarmed Strike + Bite (life drain) every turn",
				"Charm: WIS DC 17 or Charmed for 24h (broken by damage)",
				"Children of the Night (1/combat): summons 2d6 Bat Swarms or 3d6 Rats",
				"Mist Form: at 0 HP retreats to coffin; must destroy coffin in 30 turns or fully regenerates",
				"Legendary Resistance (3/combat); Phase 2 (Mist destroyed): all attacks have advantage; AoE Charm",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "blackspire_signet_ring", DropChance: 0.06, Note: "+2 CHA; undead NPCs treat you as neutral unless attacked"},
			{ItemID: "wpn_sword_of_wounding", DropChance: 0.10},
			{ItemID: "arm_armor_of_resistance_necrotic", DropChance: 0.08},
			{ItemID: "blackspire_deed", UniqueAlways: true, Note: "story/quest item"},
			{ItemID: "coins_8d10x10", DropChance: 1.0, Note: "8d10 × 10 coins"},
		},
		FlavorFile: "zone_manor_blackspire_flavor.go",
	}
}

func zoneUnderforge() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneUnderforge,
		Display:    "The Underforge",
		Tier:       ZoneTierJourneyman,
		LevelMin:   5,
		LevelMax:   8,
		Faction:    "Fire Elementals, Constructs, Salamanders, Azers",
		Atmosphere: "Volcanic caverns, rivers of cooling lava, ancient dwarven stonework, the constant bass note of something very large moving below.",
		Hook:       "The dwarven forge-city of Kharak Dûn was not abandoned. It was sealed from the outside. I do not have information on what they were sealing in.",
		MinRooms:   22,
		MaxRooms:   26,
		Enemies: []ZoneEnemy{
			// Phase 5-C: underforge trailed at 49.5% (band 55-75)
			// under the shipped HP×1.5/+3 floor. Trace named Fire
			// Elemental (57 kills, 61hp loss/win) as the dominant
			// elite killer with Salamander (33 kills) second.
			// Helmed Horror was the only 100%-win elite but sat at
			// SW=1, contributing almost no dilution. Lifted Helmed
			// Horror SW 1 → 3 so the elite pool runs three roughly-
			// equal slots and Fire Elemental's share drops from
			// ~44% to ~33% of elite picks.
			{BestiaryID: "magmin", SpawnWeight: 6},
			{BestiaryID: "azer", SpawnWeight: 5},
			{BestiaryID: "flameskull", SpawnWeight: 4},
			{BestiaryID: "salamander", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "fire_elemental", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "helmed_horror", SpawnWeight: 3, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_emberlord_thyrak",
			Name:        "Emberlord Thyrak",
			CR:          9,
			HP:          178,
			AC:          18,
			PhaseTwoAt:  0.50,
			Description: "The forge-golem that ran Kharak Dûn's furnaces for three centuries. When the dwarves left, it kept working. It has been improving the designs.",
			Abilities: []string{
				"Molten Strike: +4d6 fire on hit; target armor -1 AC until repaired (long rest)",
				"Forge Breath (recharge 5–6): 30-ft cone, 10d6 fire, DEX DC 16 half",
				"Living Forge: heals 15 HP/turn while standing on lava/forge tiles",
				"Construct Resilience: immune poison/psychic/charm/exhaustion; resist non-magical physical",
				"Phase 2 (<50% HP): Forge Breath recharge 4–6; spawns 2 Fire Elementals",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "thyraks_core", UniqueAlways: true, Note: "crafting material for legendary weapons"},
			{ItemID: "arm_adamantine_armor", DropChance: 0.12},
			{ItemID: "wpn_flame_tongue", DropChance: 0.10},
			{ItemID: "azer_ingots", UniqueAlways: true, Note: "1d6 ingots; high-value Thom Krooke sale"},
		},
		FlavorFile: "zone_underforge_flavor.go",
	}
}

// ---- Tier 4-5 zone factories (Phase 11 D5a) ---------------------------------

func zoneUnderdark() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneUnderdark,
		Display:    "The Underdark",
		Tier:       ZoneTierVeteran,
		LevelMin:   8,
		LevelMax:   12,
		Faction:    "Drow, Mind Flayers, Beholders (far), Ropers, Hook Horrors",
		Atmosphere: "Absolute darkness, phosphorescent mushroom groves, vast underground seas, carved drow cities in the distance, things older than the surface world.",
		Hook:       "There is a world below the world. It has its own cities, its own wars, its own sky — which is stone, and has never once been kind. I speak more quietly here. Something might be listening.",
		MinRooms:   28,
		MaxRooms:   34,
		Enemies: []ZoneEnemy{
			// Phase 5-C: underdark ran 88% (band 45-65, way over)
			// at the T4 centerline — its sibling feywild sat at
			// 54%, a 30pp asymmetry. Per [[feedback_difficulty_target]]
			// the bulk of Phase 5-C lifts feywild rather than nerfing
			// the leader; this is the "light underdark trim" piece.
			// Drow at SW=7 owned nearly half of all standard rolls
			// (1801/4164) at 100% win / 1.1hp loss — a free-HP
			// filler. Trimmed SW 7 → 5 so the standard pool shifts
			// toward Hook Horror (17.8hp/win) and Drow Mage
			// (16.4hp/win) — still safe but with real attrition.
			{BestiaryID: "drow", SpawnWeight: 5},
			{BestiaryID: "drow_elite_warrior", SpawnWeight: 4, IsElite: true},
			{BestiaryID: "drow_mage", SpawnWeight: 3},
			{BestiaryID: "mind_flayer", SpawnWeight: 2},
			{BestiaryID: "hook_horror", SpawnWeight: 4},
			{BestiaryID: "roper", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_ilvaras_xunyl",
			Name:        "Ilvaras Xunyl, Drow High Priestess",
			CR:          12,
			HP:          162,
			AC:          16,
			PhaseTwoAt:  0.35,
			Description: "She has killed four previous expeditions from the surface. She keeps their weapons as trophies. She has run out of wall space.",
			Abilities: []string{
				"Spells: Flame Strike, Dispel Magic, Divine Word (CHA DC 18), Insect Plague",
				"Lolth's Favour: 1/combat auto-succeed a failed save",
				"Summon Spiders (1/combat): 2d6 Giant Spiders fill the room",
				"Legendary Actions (3): Melee (1 LA), Cast Cantrip (1 LA), Drain Life (3 LA — 4d10 necrotic)",
				"Phase 2 (<35% HP): Lolth's Avatar overlay — +3 AC; spells cast as 1 higher slot",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "xunyls_diadem", DropChance: 0.05, Note: "+3 CHA; Drow Spells (Darkness, Faerie Fire 1/day each)"},
			{ItemID: "arm_drow_chain_+2", DropChance: 0.08},
			{ItemID: "wpn_dagger_of_venom", DropChance: 0.10},
			{ItemID: "spider_silk", UniqueAlways: true, Note: "2d4 high-value crafting material"},
			{ItemID: "coins_15d10x12", DropChance: 1.0, Note: "15d10 × 12 coins"},
		},
		FlavorFile: "zone_underdark_flavor.go",
	}
}

func zoneFeywildCrossing() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneFeywildCrossing,
		Display:    "Feywild Crossing",
		Tier:       ZoneTierVeteran,
		LevelMin:   8,
		LevelMax:   12,
		Faction:    "Hags, Redcaps, Will-o-Wisps, Fomorians, Unseelie Fey",
		Atmosphere: "Impossible beauty, treacherous whimsy, time distortion, rules that change without notice, bargains with terrible fine print.",
		Hook:       "The veil between worlds is thin here. Colors are too saturated. The mushrooms are too large. A small creature made of starlight just offered you a deal. I advise extreme caution regarding deals.",
		MinRooms:   28,
		MaxRooms:   34,
		Enemies: []ZoneEnemy{
			// Phase 5-C: feywild trailed at 54% (band 45-65, but
			// the design goal was to close the 30pp gap with its
			// underdark sibling at 88%). Trace named Fomorian as
			// the dominant killer (65 kills, 50% win, SW=1) — a
			// 2-elite pool with Night Hag at SW=3 meant Fomorian
			// owned ~25% of elite picks despite its low weight,
			// and that bracket is where the deaths came from.
			// Promoted Green Hag to elite (SW=2): adds a softer
			// 98%-win elite slot to dilute Fomorian's share, and
			// pulls a 16hp/win standard out of the standard pool
			// so the standard floor collapses to Redcap + Will-o-
			// Wisp + Quickling — all ≥99% win, ≤11hp loss.
			{BestiaryID: "redcap", SpawnWeight: 6},
			{BestiaryID: "will_o_wisp", SpawnWeight: 5},
			{BestiaryID: "quickling", SpawnWeight: 5},
			{BestiaryID: "green_hag", SpawnWeight: 2, IsElite: true},
			{BestiaryID: "night_hag", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "fomorian", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_thornmother",
			Name:        "The Thornmother",
			CR:          11,
			HP:          187,
			AC:          17,
			PhaseTwoAt:  0.30,
			Description: "She has three names. She will offer to tell you all three. She will not tell you what accepting means. The flowers around her throne are beautiful and they are not flowers.",
			Abilities: []string{
				"Coven Magic: extra spell slots scaled by GM Mood (Elated/Engaged TwinBee = stronger Thornmother)",
				"Beguiling Bargain (1/combat): offers a deal — accept a debuff for a permanent minor buff",
				"Thorned Grasp: Restrained + 4d6 piercing/turn, CON DC 16 to break free",
				"Shapechange (1/combat): adopts a player's appearance; 50% miss vs her until DC 17 Investigation",
				"Phase 2 (<30% HP): True Form — illusions drop; +4d6 psychic on attacks; coven summons 2 Night Hags",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "thornmothers_bargain", UniqueAlways: true, Note: "ring; +4 to one stat / -2 to another (player choice; permanent)"},
			{ItemID: "wpn_rapier_of_puncturing_+2", DropChance: 0.08},
			{ItemID: "arm_cloak_of_protection_+2", DropChance: 0.08},
			{ItemID: "feywild_essence", UniqueAlways: true, Note: "1d4 legendary crafting material"},
		},
		FlavorFile: "zone_feywild_crossing_flavor.go",
	}
}

func zoneDragonsLair() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneDragonsLair,
		Display:    "Dragon's Lair (Infernus Peak)",
		Tier:       ZoneTierLegendary,
		LevelMin:   12,
		LevelMax:   20,
		Faction:    "Kobolds, Drakes, Young Dragons, Wyrm",
		Atmosphere: "Scorched stone, rivers of gold coins half-melted into the floor, kobold warrens as outer defenses, growing heat, the unmistakable smell of something ancient and enormous.",
		Hook:       "The mountain has not erupted in forty years. The locals say it is dormant. The locals are wrong about what lives in mountains. I have prepared an unusually long entry description for this one.",
		MinRooms:   36,
		MaxRooms:   44,
		Enemies: []ZoneEnemy{
			{BestiaryID: "kobold", SpawnWeight: 7},
			{BestiaryID: "guard_drake", SpawnWeight: 5},
			{BestiaryID: "kobold_scale_sorcerer", SpawnWeight: 4},
			{BestiaryID: "dragonborn_cultist", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "young_red_dragon", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_infernax",
			Name:        "Infernax the Undying",
			CR:          24,
			HP:          546,
			AC:          22,
			PhaseTwoAt:  0.50,
			Description: "Ancient Red Dragon. He remembers when the surface civilizations were just fires. He found the fires amusing. He finds their descendants less so.",
			Abilities: []string{
				"Multiattack: Bite + 2 Claws",
				"Fire Breath (recharge 5–6): 90-ft cone, 26d6 fire, DEX DC 24 half",
				"Frightful Presence: WIS DC 21 or Frightened 1 min",
				"Legendary Resistance (3/combat): auto-succeed failed save",
				"Legendary Actions (3): Detect (1), Tail Attack (1), Wing Attack (2 — AoE knockback DEX DC 22)",
				"Lair Actions (init 20): Magma eruption (2d6 fire), Volcanic gases (CON DC 13 Poisoned), Tremor (DEX DC 15 Prone)",
				"Phase 2 (<50% HP): Fire Breath recharge 4–6; fire damage ignores resistance",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "scale_of_infernax", UniqueAlways: true, Note: "legendary crafting anchor for legendary fire weapons"},
			{ItemID: "arm_dragon_scale_mail_red", DropChance: 0.15},
			{ItemID: "wpn_flame_tongue_+3", DropChance: 0.10},
			{ItemID: "ring_of_fire_resistance", DropChance: 0.12},
			{ItemID: "coins_50d10x100", DropChance: 1.0, Note: "Dragon Hoard: 50d10 × 100 coins"},
		},
		FlavorFile: "zone_dragons_lair_flavor.go",
	}
}

func zoneAbyssPortal() ZoneDefinition {
	return ZoneDefinition{
		ID:         ZoneAbyssPortal,
		Display:    "The Abyss Portal",
		Tier:       ZoneTierLegendary,
		LevelMin:   15,
		LevelMax:   20,
		Faction:    "Demons, Fiends, Corrupted Celestials",
		Atmosphere: "Reality fractures, impossible geometry, constant low psychic pressure, the feeling of being watched by something that has no eyes.",
		Hook:       "Someone opened a door they should not have opened. The door is still open. Things are still coming through. I am not making jokes about this one.",
		MinRooms:   36,
		MaxRooms:   44,
		Enemies: []ZoneEnemy{
			// Phase 4-B (outlier fix): Nalfeshnee was mis-classified
			// as a standard at T5 — Phase 4-A measured 2.8% win rate
			// across 180 standard-pool picks, attributing 86 kills
			// (43% of all deaths in the trace). Re-flagged as elite
			// where its CR-13 stat block belongs; the new elite pool
			// Hezrou+Nalfeshnee+Marilith mirrors the sibling
			// dragons_lair's three-elite shape. Vrock SW shaved 5→4
			// so the soft Quasit (≥99% win) takes a larger share of
			// the standard mass, since Vrock was the secondary
			// standard killer (53 kills, 44hp loss/win).
			//
			// Second pass: a first attempt (Nalf→elite, Vrock SW 5→4)
			// only moved comp 0% → 9%. Re-trace showed Vrock still
			// dominant in the standard pool (1013 appearances, 86.3%
			// win, but 43.6 hp loss per win → 65 kills via attrition).
			// The sibling dragons_lair posts zero standard-pool kills;
			// promoting Vrock to elite brings abyss in line. Standard
			// floor collapses to Quasit alone, which is fine — Quasit
			// posts ≥99.9% win in trace and the elite bracket
			// triggers ~7% of rolls anyway.
			{BestiaryID: "quasit", SpawnWeight: 6},
			{BestiaryID: "vrock", SpawnWeight: 4, IsElite: true},
			{BestiaryID: "hezrou", SpawnWeight: 4, IsElite: true},
			{BestiaryID: "nalfeshnee", SpawnWeight: 3, IsElite: true},
			{BestiaryID: "marilith", SpawnWeight: 1, IsElite: true},
		},
		Boss: ZoneBoss{
			BestiaryID:  "boss_belaxath",
			Name:        "Belaxath the Undivided",
			CR:          19,
			HP:          262,
			AC:          19,
			PhaseTwoAt:  0.40,
			Description: "Balor. The portal was not summoned. It was torn. Belaxath did the tearing from the other side, with purpose, over three decades. It is annoyed it took that long.",
			Abilities: []string{
				"Multiattack: Longsword + Whip",
				"Fire Aura: 10d6 fire to all melee attackers each turn; weapons +3d6 fire",
				"Lightning Discharge (recharge 5–6): 120-ft line, 12d6 lightning, DEX DC 20 half",
				"Death Throes: on death, 30-ft radius, 20d6 fire, DEX DC 20 half — destroys non-legendary equipment on failed save",
				"Magic Resistance: advantage on saves vs spells",
				"Legendary Resistance (3/combat)",
				"Demonic Resilience: resist cold/fire/lightning; immune poison; immune non-magical physical",
				"Phase 2 (<40% HP): grows to Huge size; advantage on all attacks; Death Throes recharge 4–6",
			},
		},
		Loot: []ZoneLootEntry{
			{ItemID: "shard_of_the_abyss", UniqueAlways: true, Note: "legendary quest item for portal-closing storyline"},
			{ItemID: "wpn_demon_blade", DropChance: 0.05, Note: "+3 greatsword; +2d6 necrotic; immune to Charm/Frightened"},
			{ItemID: "arm_demon_armor_+3", DropChance: 0.05},
			{ItemID: "portal_fragment", UniqueAlways: true, Note: "legendary crafting — required for highest-tier recipes"},
		},
		FlavorFile: "zone_abyss_portal_flavor.go",
	}
}
