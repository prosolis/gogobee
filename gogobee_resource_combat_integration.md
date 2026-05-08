# GogoBee — Expedition Resource & Combat Integration
> **Companion to:** `gogobee_expedition_system.md`, `gogobee_dungeon_zones.md`
> **Version:** 1.0
> **Status:** Draft
> **Impact:** REPLACES standalone harvesting system and standalone dungeon run system

---

## ⚠️ Protected Files — DO NOT MODIFY

All existing `*_flavor.go` / `*_flavor.txt` files are off-limits.
New resource and combat flavor text goes in `twinbee_resource_flavor.go` — new file only.

---

## 1. System Consolidation

### What Is Being Replaced

The following standalone systems are **deprecated** and removed from the command surface:

| Deprecated System | Replaced By |
|---|---|
| Standalone `!adventure` command | Expedition exploration (`!advance`) |
| Standalone `!harvest` command | Zone harvesting within active expedition |
| Standalone dungeon runs (`DungeonRun`) | `Expedition` system entirely |
| Generic loot tables (zone-agnostic) | Zone-specific resource tables (this doc) |
| Generic combat events | Zone-contextual combat events (this doc) |

### What Is Preserved

| Existing System | Status | Notes |
|---|---|---|
| Fishing | Preserved + integrated | Fishing happens in water-bearing zones only; Ranger bonuses apply |
| Pet system | Preserved + integrated | Pet bonuses apply inside expeditions unchanged |
| Arena / streak system | Preserved, standalone | Arena remains a separate PvP-adjacent system |
| Economy / Thom Krooke | Preserved | Supply procurement, loot selling, all unchanged |
| NPC interactions | Preserved + expanded | NPCs can now appear as expedition events |

### Migration for Existing Players

Players with resources gathered under the old system retain those resources in their inventory. Old `DungeonRun` records are archived (not deleted). No player loses progress. The new expedition system becomes the only path to generate new resources from Day 1 of launch.

---

## 2. How Resources Work Inside Expeditions

Resources are gathered through **Harvest Actions** — a category of expedition action that occurs in cleared rooms. A harvest action:

- Costs one action slot per attempt (not a full day — multiple harvests possible per day)
- Requires an appropriate skill check (skill and DC vary by resource and zone)
- Can trigger a **Combat Interrupt** (Section 4)
- Yields zone-specific materials, not generic drops
- Has a daily attempt cap per resource node (nodes replenish on long rest)

### 2.1 Harvest Action Types

| Action | Command | Primary Skill | Who Benefits |
|---|---|---|---|
| Forage | `!forage` | WIS (Survival) | Ranger +2 bonus |
| Mine / Salvage | `!mine` | STR (Athletics) | Fighter +2 bonus |
| Fish | `!fish` | DEX (Sleight of Hand) | Ranger +bonus; existing fishing rank applies |
| Scavenge | `!scavenge` | INT (Investigation) | Rogue +2 bonus |
| Harvest Essence | `!essence` | INT (Arcana) | Mage +2 bonus |
| Commune | `!commune` | WIS (Insight) | Cleric +2 bonus; unlocks unique Cleric-only drops |

### 2.2 Harvest Yield Structure

Each harvest attempt produces one of four outcomes:

| Outcome | Roll Result | Yield |
|---|---|---|
| **Nothing** | Fail DC by 5+ | No yield; node not depleted |
| **Common yield** | Fail DC by 1–4 | 1 unit of base zone material |
| **Standard yield** | Meet DC | 1d3 units of base zone material |
| **Rich yield** | Beat DC by 5+ | 1d3 units base material + 1 bonus/rare material |

### 2.3 Node Depletion

Each room has a fixed number of harvest nodes per type (defined per zone, Section 3). Once a node is depleted it does not regenerate until long rest. TwinBee narrates node depletion:

> *"The patch is stripped. You've taken what it had. TwinBee notes the location for the return trip — if there is one."*

---

## 3. Zone Resource Tables

### Zone 01 — Goblin Warrens

**Biome:** Underground tunnels, crude construction, scavenged materials  
**Primary Gathering Actions:** `!scavenge`, `!mine`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Scrap Iron | Common material | `!mine` | 10 | Used in basic weapon repairs |
| Goblin Alchemy Pouch | Uncommon material | `!scavenge` | 13 | Random minor potion component |
| Worg Pelt | Uncommon material | `!scavenge` (post-Worg kill) | 11 | Requires Worg defeated in room |
| Crude Weapon Cache | Uncommon item | `!scavenge` | 15 | Random Common weapon drop |
| Hobgoblin War Standard | Rare material | `!scavenge` | 18 | High sell value; Thom Krooke special interest |
| Goblin Shaman's Reagents | Rare material | `!essence` | 17 | Mage only; spell component batch |

**Zone Fishing:** None.  
**Node Count Per Room:** Scrap Iron ×2, Alchemy Pouch ×1, Worg Pelt ×1 (conditional), Cache ×1.

---

### Zone 02 — Crypt of Valdris

**Biome:** Stone crypts, necrotic energy, ancient burial goods  
**Primary Gathering Actions:** `!scavenge`, `!essence`, `!commune`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Grave Soil | Common material | `!scavenge` | 9 | Alchemy base; Cleric crafting component |
| Bone Dust | Common material | `!scavenge` | 9 | Spell component; sell value low |
| Ancient Coin (pre-Empire) | Uncommon material | `!scavenge` | 14 | Thom Krooke pays premium |
| Necrotic Essence | Uncommon material | `!essence` | 15 | Magic item crafting; volatile |
| Silver Grave Dust | Rare material | `!commune` | 17 | Cleric only; enhances undead-specific abilities |
| Valdris's Seal Fragment | Rare quest material | `!scavenge` | 20 | Lore item; possible future quest chain |

**Zone Fishing:** None.  
**Node Count Per Room:** Grave Soil ×2, Bone Dust ×2, Ancient Coin ×1, Necrotic Essence ×1.

---

### Zone 03 — Forest of Shadows

**Biome:** Ancient corrupted woodland, streams, fauna, fey-adjacent  
**Primary Gathering Actions:** `!forage`, `!essence`, `!fish`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Shadow Herb | Common material | `!forage` | 10 | Potion base; anti-fear properties |
| Beast Pelt | Common material | `!forage` (post-kill) | 11 | Requires beast kill in room; Ranger bonus |
| Darkwood | Uncommon material | `!mine` | 13 | Weapon crafting; bows, staves |
| Corrupted Fey Bloom | Uncommon material | `!forage` | 16 | Rare; potion of resistance component |
| Owlbear Feather | Uncommon material | `!scavenge` (post-kill) | 12 | Requires Owlbear kill; fletching, rare item component |
| Dryad's Tears | Rare material | `!essence` | 18 | Mage/Ranger only; regeneration potion component |
| Bioluminescent Fungi | Rare material | `!forage` | 17 | Light source crafting; high sell value |

**Zone Fishing:** Forest streams, outdoor exploration rooms only.

| Fish | Rarity | DC | Notes |
|---|---|---|---|
| Shadow Trout | Common | 11 | Edible; 0.5 SU value |
| Gloomfin | Uncommon | 15 | Sell value; potion component |
| Phantom Carp | Rare | 20 | Cleric material; high sell value |

**Node Count Per Room:** Shadow Herb ×2, Beast Pelt ×1 (conditional), Darkwood ×1, Fungi ×1.

---

### Zone 04 — Sunken Temple of Dar'eth

**Biome:** Flooded stone, tidal, aquatic, aboleth-touched  
**Primary Gathering Actions:** `!fish`, `!scavenge`, `!essence`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Sea Glass Shard | Common material | `!scavenge` | 9 | Crafting filler; low value |
| Deep Coral | Common material | `!mine` | 11 | Jewelry component; Thom Krooke moderate value |
| Kuo-toa Scale | Uncommon material | `!scavenge` (post-kill) | 12 | Armor crafting component |
| Black Pearl | Uncommon material | `!fish` | 16 | High sell value; accessory component |
| Aboleth Mucus (refined) | Rare material | `!essence` | 18 | Mage only; psychic spell amplifier |
| Ancient Temple Relic | Rare item | `!scavenge` | 20 | Random Uncommon item; lore value |
| Tidal Crystal | Rare material | `!mine` (Tidal Event only) | 17 | Only available during Day 6 Tidal Event; spell focus component |

**Zone Fishing:** All flooded rooms. Bonus fish nodes during Tidal Event.

| Fish | Rarity | DC | Notes |
|---|---|---|---|
| Blind Cave Fish | Common | 10 | Edible; 0.5 SU |
| Merrow Eel | Uncommon | 14 | Potion component; slight poison property |
| Dar'eth Lanternfish | Rare | 19 | Bioluminescent; crafting and sell value |
| Aboleth Spawn (juvenile) | Very Rare | 23 | Requires Ranger + DC 23; unique Ranger trophy |

**Node Count Per Room:** Sea Glass ×2, Deep Coral ×1, Kuo-toa Scale ×1 (conditional), Pearl ×1.

---

### Zone 05 — Haunted Manor of Blackspire

**Biome:** Victorian decay, cursed objects, spectral residue  
**Primary Gathering Actions:** `!scavenge`, `!essence`, `!commune`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Grave Dust (spectral) | Common material | `!scavenge` | 10 | Upgraded version of Crypt variant; more potent |
| Cursed Trinket | Common item | `!scavenge` | 12 | Random debuff item; sell only; do not equip |
| Ghost Silk | Uncommon material | `!essence` | 15 | Armor crafting; +DEX light armor component |
| Vampire Ash | Uncommon material | `!scavenge` (post-Vampire kill) | 14 | Requires Vampire Spawn kill; holy weapon component |
| Shadow Essence | Rare material | `!essence` | 18 | Rogue item component; stealth enhancement |
| Blackspire Library Volume | Rare lore item | `!scavenge` | 19 | Increases INT by 1 for one dungeon run (consumable); lore value |
| Wraith Core | Rare material | `!commune` | 20 | Cleric only; requires Wraith kill; undead ward component |

**Zone Fishing:** None.  
**Special:** Every `!scavenge` in the Manor has a 10% chance of yielding a **Cursed Trinket** as a secondary drop regardless of node. TwinBee warns about this once, clearly, and then does not warn again.

---

### Zone 06 — The Underforge

**Biome:** Volcanic caverns, ancient dwarven metalwork, fire elementals  
**Primary Gathering Actions:** `!mine`, `!essence`, `!scavenge`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Iron Ore | Common material | `!mine` | 10 | Weapon/armor base material |
| Volcanic Glass | Common material | `!mine` | 11 | Blade component; slightly fragile |
| Azer Metal Ingot | Uncommon material | `!mine` (post-Azer kill) | 14 | Requires Azer kill; always-hot; unique properties |
| Fire Essence | Uncommon material | `!essence` | 15 | Mage crafting; fire spell amplifier |
| Mithral Trace | Rare material | `!mine` | 20 | Very rare; mithral armor component |
| Forge Core Fragment | Rare material | `!scavenge` (post-Construct kill) | 17 | Construct parts; golem-crafting future use |
| Salamander Oil | Rare material | `!essence` (post-Salamander kill) | 16 | Fire resistance potion component; high value |

**Zone Fishing:** None.  
**Heat Interaction:** At Heat Stack 7+, all `!mine` DCs increase by 3. At Heat Stack 10, harvesting is impossible until stacks reduce.

---

### Zone 07 — The Underdark

**Biome:** Vast underground, drow civilization, alien ecology, cave rivers  
**Primary Gathering Actions:** `!forage`, `!mine`, `!fish`, `!essence`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Underdark Mushroom | Common material | `!forage` | 10 | Edible (+1 SU); also potion base |
| Cave Spider Silk | Common material | `!forage` | 12 | Armor component; light and strong |
| Drow Poison (diluted) | Uncommon material | `!scavenge` (post-Drow kill) | 14 | Weapon coating; Unconscious condition on hit (save) |
| Faerzress Crystal | Uncommon material | `!mine` | 16 | Magic-disruptive; Mage spell component |
| Mind Flayer Brain Matter | Rare material | `!essence` (post-Illithid kill) | 19 | Mage only; INT buff component; deeply unpleasant to handle |
| Drow Adamantine | Rare material | `!mine` | 21 | Drow-forged; superior armor component |
| Deep Dragon Scale Chip | Very Rare material | `!scavenge` | 23 | World drop; legendary item component |

**Zone Fishing:** Underground rivers (specific Exploration rooms only — TwinBee marks them).

| Fish | Rarity | DC | Notes |
|---|---|---|---|
| Blind Albino Bass | Common | 11 | Edible; 0.5 SU |
| Glowing Cave Eel | Uncommon | 15 | Light source (temporary); sell value |
| Underdark Shark | Rare | 20 | Trophy fish; Ranger achievement |
| The Eyeless King | Legendary | 26 | Unique named fish; Ranger lv10+ only; Thom Krooke pays extraordinary amount |

---

### Zone 08 — Feywild Crossing

**Biome:** Impossible forest, fey magic, unstable time, dangerous beauty  
**Primary Gathering Actions:** `!forage`, `!essence`, `!commune`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Fey Dust | Common material | `!forage` | 11 | Spell component; mild time-distortion effect |
| Enchanted Petal | Common material | `!forage` | 12 | Potion of Charm component |
| Pixie Wing Fragments | Uncommon material | `!essence` | 16 | Very delicate; magic item component |
| Will-o-Wisp Essence | Uncommon material | `!essence` (post-kill) | 17 | Light magic; Mage light spell enhancement |
| Hag Hair | Rare material | `!commune` (post-Hag kill) | 18 | Cleric ward component; warding against fey charm |
| Timelock Amber | Rare material | `!forage` | 21 | Preserves items; rare crafting anchor |
| Thornmother's Thorn | Very Rare material | `!scavenge` (post-boss) | — | Boss drop only; legendary item component |

**Zone Fishing:** Fey streams only (time-distorted; fishing roll has chance of Time Distortion event).  
**Time Interaction:** On a Double-Day roll (Section 7.4 of expedition doc), all `!forage` DCs decrease by 3. On Time Loop, previous harvest results in that room are nullified — resources return to nodes.

---

### Zone 09 — Dragon's Lair

**Biome:** Volcanic mountain, kobold warrens, draconic power, ancient wealth  
**Primary Gathering Actions:** `!mine`, `!scavenge`, `!essence`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Ancient Gold Coin | Common material | `!scavenge` | 10 | Direct coin value; also historical item |
| Volcanic Gem | Common material | `!mine` | 12 | Jewelry; spell component |
| Drake Blood | Uncommon material | `!essence` (post-Drake kill) | 14 | Fire resistance potion component |
| Dragon Scale Fragment | Uncommon material | `!scavenge` (post-Drake kill) | 16 | Armor component; not boss-quality |
| Dragonfire Coal | Rare material | `!mine` | 19 | Burns indefinitely; heating, forging |
| Kobold Trap Mechanism | Rare item | `!scavenge` | 18 | Deployable trap (one-use combat item) |
| Infernax Scale | Legendary material | `!scavenge` (post-boss) | — | Boss drop only; legendary fire weapon component |

**Zone Fishing:** None.  
**Awareness Interaction:** At Awareness Pulse 3+, all harvest DCs increase by 4. After Infernax wakes (Day 14), harvesting is disabled — survival only.

---

### Zone 10 — The Abyss Portal

**Biome:** Fractured reality, demonic incursion, planar debris  
**Primary Gathering Actions:** `!essence`, `!scavenge`, `!mine`

| Resource | Type | Harvest Action | DC | Notes |
|---|---|---|---|---|
| Brimstone Shard | Common material | `!mine` | 11 | Fire damage component; unstable |
| Demon Ichor | Common material | `!scavenge` (post-kill) | 12 | Poison/acid component; corrosive |
| Planar Shard | Uncommon material | `!mine` | 15 | Reality fragment; Mage crafting anchor |
| Corrupted Metal | Uncommon material | `!scavenge` | 16 | Weapon component; chaotic properties |
| Void Essence | Rare material | `!essence` | 20 | Pure planar energy; legendary crafting only |
| Abyssal Ichor (concentrated) | Rare material | `!essence` (post-Balor region) | 22 | Mage only; most powerful spell amplifier in game |
| Portal Fragment | Legendary material | `!scavenge` (post-boss) | — | Boss drop; portal-closing quest item |

**Instability Interaction:** At Instability 61+, all harvest DCs increase by 5. At Instability 81+, harvesting is disabled. At Collapse, all unharvested nodes are lost.

---

## 4. Combat Events During Expeditions

Combat no longer occurs as a standalone triggered event. All combat happens **within the expedition context** as one of four event types.

### 4.1 Combat Event Types

**Room Combat**  
The standard encounter — enemies present in a new or reset room. Triggered by `!advance`. Uses full D&D combat resolution. Room is flagged Cleared on enemy defeat. Cleared rooms are safe for harvesting.

**Harvest Interrupt**  
A combat encounter triggered mid-harvest. The player was focused on gathering; enemies took advantage.

- Trigger: `!forage`/`!mine`/etc. with Combat Interrupt roll (see Section 4.2)
- Enemy is from zone roster at current room's tier
- Player does NOT get to roll initiative — enemy acts first (surprise round)
- Ranger passive: -3 to Combat Interrupt roll in wilderness zones
- Rogue passive: if Stealth was active, no surprise round

**Night Ambush**  
Wandering monster encounter during Night Phase. Player wakes to combat.
- Player HP is at current value (not full unless long rest was taken)
- First round: DEX save DC 12 or lose first turn (groggy)

**Patrol Encounter**  
At Threat Clock Alert+, patrols move through cleared rooms. Triggered on `!advance` between rooms, not inside rooms.
- Patrol is 1–2 enemies, weaker than room combat
- Defeating a patrol does not clear a room
- Fleeing a patrol costs Threat Clock +5

### 4.2 Combat Interrupt Roll

When a Harvest Action is attempted, roll 1d20 + zone tier:

| Roll | Outcome |
|---|---|
| 1–8 | No interrupt. Harvest proceeds. |
| 9–14 | Noise noticed. Threat Clock +2. No combat. |
| 15–18 | Harvest Interrupt: 1 standard enemy from zone roster. |
| 19–21 | Harvest Interrupt: elite enemy. Harvest node not depleted (you dropped it). |
| 22+ | Patrol Interrupt: 1d3 enemies. Harvest fails entirely. |

**Threat Clock modifier:** +1 per 20 Threat Clock points above 40.

### 4.3 Combat Outcomes Within Expeditions

**Victory:**
- Loot drops from zone loot table
- XP awarded
- Harvest can resume (node still active unless Patrol Interrupt)
- TwinBee delivers zone-contextual combat victory line

**Defeat (0 HP):**
- Forced extraction triggers (Section 10.2 of expedition doc)
- Any unbanked loot from current room is lost
- Carried loot is preserved

**Flee:**
- DEX check DC 13 to escape
- On success: leave room; Threat Clock +5; harvest abandoned
- On fail: enemy gets one free attack; then escape succeeds

---

## 5. Zone-Contextual Loot Tables

Zone loot drops from combat are separate from harvest nodes. These drop on enemy defeat and are zone-specific.

### Loot Drop Logic

Every combat victory generates a loot roll:
1. Roll for **drop tier** (based on enemy CR)
2. Roll on **zone loot table** for that tier
3. Apply **rarity roll** — base rates modified by Ranger pet bonus and class

| Enemy CR Range | Drop Tier | Guaranteed Drop? |
|---|---|---|
| 0–1 | Common only | No (70% chance) |
| 2–4 | Common or Uncommon | Yes (Common always; Uncommon 30%) |
| 5–8 | Uncommon or Rare | Yes (Uncommon always; Rare 15%) |
| 9–12 | Rare or Epic | Yes (Rare always; Epic 10%) |
| 13+ | Epic or Legendary | Yes (Epic always; Legendary 5%) |
| Boss | Rare minimum | Yes (see individual boss tables) |

### Zone Loot Flavor by Biome

Every item drop has a zone-contextual description. The same `Potion of Healing` found in the Goblin Warrens is described differently than one found in the Haunted Manor:

| Zone | Potion of Healing flavor |
|---|---|
| Goblin Warrens | A stolen vial, still stoppered with a rag. It smells like goblin and better days. |
| Crypt of Valdris | A glass vial embedded in a burial offering. Whoever left it didn't need it. |
| Forest of Shadows | Bark-sealed, filled with something deep red. It tastes like the forest before it went wrong. |
| Sunken Temple | Salt-rimed and pressure-sealed. A temple offering, preserved by the deep cold. |
| Haunted Manor | Found in the medicine cabinet, dated forty years ago. Remarkably, still effective. |
| Underforge | Hot to the touch even now. The dwarves made things to last. |
| Underdark | Drow-made. The formula is different. The effect is the same. The ingredients are not discussed. |
| Feywild Crossing | It shifts color when you look at it sideways. The fey make medicine the way they make everything: beautifully and with caveats. |
| Dragon's Lair | A kobold medic's field kit, looted from a belt pouch. The kobolds take care of their own. |
| Abyss Portal | Glows faintly red. Still works. TwinBee recommends not asking what's in it. |

---

## 6. Fishing System Integration

Fishing is preserved entirely but now occurs only within active expeditions, in zones and rooms that support it.

### 6.1 Fishing Zones

| Zone | Fishing Rooms | Fishing Window |
|---|---|---|
| Forest of Shadows | Outdoor exploration rooms with stream markers | Day phase only |
| Sunken Temple | All flooded rooms | Any time; bonus during Tidal Event |
| Underdark | Underground river rooms (TwinBee marks these) | Any time |
| Feywild Crossing | Fey stream rooms | Day phase; subject to Time Distortion |

All other zones: no fishing.

### 6.2 Fishing Mechanics Within Expeditions

- `!fish` uses the existing fishing rank system with no mechanical changes
- Fishing counts as a Harvest Action (costs an action slot)
- Fishing Interrupt roll uses the same Combat Interrupt table (Section 4.2)
- Ranger fishing bonuses (+20% rare catch rate) apply
- Fish caught in expeditions are added to inventory, not auto-converted to coins
- Fish can be eaten for SU value (see zone tables) or sold to Thom Krooke post-expedition

### 6.3 Expedition-Exclusive Fish

Certain fish only exist within expedition zones and cannot be caught via any other mechanic. These are marked in zone tables as "Expedition-only." They have unique sell values, crafting uses, or both.

---

## 7. Go Data Structures — Resource System

```go
// HarvestNode represents a gatherable resource point in a room.
type HarvestNode struct {
    ID           string    `json:"id"`
    ZoneID       string    `json:"zone_id"`
    RoomID       string    `json:"room_id"`
    ResourceID   string    `json:"resource_id"`
    Action       string    `json:"action"`        // forage, mine, scavenge, essence, commune, fish
    DC           int       `json:"dc"`
    MaxCharges   int       `json:"max_charges"`   // uses before depletion
    CurrentCharges int     `json:"current_charges"`
    RequiresKill string    `json:"requires_kill"` // monster ID, empty if no kill required
    Conditional  string    `json:"conditional"`   // zone event ID that enables/disables node
}

// ZoneResource defines a harvestable material or item for a zone.
type ZoneResource struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    ZoneID       string   `json:"zone_id"`
    Rarity       string   `json:"rarity"`        // common, uncommon, rare, very_rare, legendary
    Type         string   `json:"type"`          // material, item, fish, quest
    SellValue    int      `json:"sell_value"`    // coins at Thom Krooke base price
    CraftingTags []string `json:"crafting_tags"` // what this can be used to make
    FlavorRef    string   `json:"flavor_ref"`    // key into zone resource flavor text
}

// HarvestAttempt is the result of a single harvest action.
type HarvestAttempt struct {
    PlayerID     string    `json:"player_id"`
    ExpeditionID string    `json:"expedition_id"`
    NodeID       string    `json:"node_id"`
    Action       string    `json:"action"`
    RollResult   int       `json:"roll_result"`
    DC           int       `json:"dc"`
    Outcome      string    `json:"outcome"`       // nothing, common, standard, rich
    Yield        []string  `json:"yield"`         // resource IDs yielded
    Interrupted  bool      `json:"interrupted"`   // combat interrupt triggered
    Timestamp    time.Time `json:"timestamp"`
}

// CombatEvent is a combat occurrence within an expedition.
type CombatEvent struct {
    ID           string    `json:"id"`
    ExpeditionID string    `json:"expedition_id"`
    Type         string    `json:"type"`          // room, harvest_interrupt, night_ambush, patrol
    EnemyIDs     []string  `json:"enemy_ids"`
    SurpriseRound bool     `json:"surprise_round"`
    Outcome      string    `json:"outcome"`       // victory, defeat, flee
    LootDrops    []string  `json:"loot_drops"`    // item IDs
    XPAwarded    int       `json:"xp_awarded"`
    ThreatDelta  int       `json:"threat_delta"`
    FlavorLine   string    `json:"flavor_line"`
    Timestamp    time.Time `json:"timestamp"`
}

// ZoneLootEntry defines a loot drop entry tied to a zone and CR range.
type ZoneLootEntry struct {
    ZoneID       string   `json:"zone_id"`
    CRMin        float32  `json:"cr_min"`
    CRMax        float32  `json:"cr_max"`
    ItemID       string   `json:"item_id"`
    Weight       float32  `json:"weight"`
    FlavorKey    string   `json:"flavor_key"` // zone-specific item description key
}
```

---

## 8. Thom Krooke — Resource Economy

Resources harvested during expeditions feed directly into Thom Krooke's economy. New shop mechanics:

### 8.1 Selling Resources

Post-expedition, all harvested materials can be sold via `!sell` at Thom Krooke's shop. Base prices:

| Rarity | Base Sell Price |
|---|---|
| Common | 5–15 coins |
| Uncommon | 25–60 coins |
| Rare | 100–300 coins |
| Very Rare | 500–1,200 coins |
| Legendary | 3,000–10,000 coins |

CHA Persuasion check (DC 15) applies the 10% discount from the main design doc to **purchases**, not sales. A separate CHA check (DC 17) can increase sell price by 15%.

### 8.2 Crafting at Thom Krooke

Certain resource combinations can be brought to Thom Krooke for crafting. Recipes are discovered via `!lore` checks or NPC interactions. Examples:

| Recipe | Materials | Output |
|---|---|---|
| Potion of Fire Resistance | Salamander Oil + Fire Essence × 2 | Potion (resist fire 1 combat) |
| Mithral Dagger | Mithral Trace + Scrap Iron × 3 | Mithral Dagger (light armor prof bypass) |
| Undead Ward Charm | Silver Grave Dust + Wraith Core | Amulet: +2 to saves vs. undead |
| Shadow Step Cloak | Ghost Silk × 3 + Shadow Essence | Light armor: Stealth advantage 1/day |
| Drow Poison Blade | Drow Poison + any blade weapon | Weapon: sleep-on-crit for 3 combats |

Full crafting recipe list is a separate document (deferred to Phase E6).

---

## 9. Command Surface Changes

### Deprecated Commands (removed)

| Command | Reason |
|---|---|
| `!adventure` | Replaced by `!expedition start` + `!advance` |
| `!harvest` | Replaced by `!forage`, `!mine`, `!scavenge`, `!essence`, `!commune` |
| `!dungeon` | Replaced by `!expedition start` |

### New Commands

| Command | Description |
|---|---|
| `!forage` | Forage in current cleared room |
| `!mine` | Mine/salvage in current cleared room |
| `!scavenge` | Scavenge in current cleared room |
| `!essence` | Harvest magical essence in current cleared room |
| `!commune` | Commune with spiritual energy (Cleric primary) |
| `!fish` | Fish in current room (water zones only) |
| `!inventory` | Full inventory including harvested materials |
| `!sell [item]` | Sell item or material to Thom Krooke (post-expedition) |
| `!craft` | Open crafting menu at Thom Krooke |
| `!resources` | List harvestable nodes in current room |

### Unchanged Commands

All expedition navigation commands (`!advance`, `!retreat`, `!search`, `!camp`, `!rest`, `!extract`) are unchanged. All combat commands unchanged. `!fish` syntax unchanged.

---

## 10. Implementation Phases — Resource & Combat Integration

### Phase R1 — System Deprecation & Migration
- [ ] Archive existing `DungeonRun` records
- [ ] Remove `!adventure`, `!harvest`, `!dungeon` command handlers
- [ ] Migrate existing player resource inventories to new schema
- [ ] Update Thom Krooke shop to accept new resource types

### Phase R2 — Harvest Node System
- [ ] `HarvestNode`, `ZoneResource`, `HarvestAttempt` structs and DB schema
- [ ] Harvest action handlers (`!forage`, `!mine`, `!scavenge`, `!essence`, `!commune`)
- [ ] Node depletion and long-rest replenishment
- [ ] `!resources` command (list active nodes in room)

### Phase R3 — Combat Event Integration
- [ ] `CombatEvent` struct and DB schema
- [ ] Harvest Interrupt roll system
- [ ] Patrol Encounter system (Threat Clock gated)
- [ ] Night Ambush integration with wandering monster system
- [ ] Zone-contextual combat victory/defeat flavor

### Phase R4 — Zone Loot & Resource Tables
- [ ] `ZoneLootEntry` tables for all 10 zones
- [ ] Zone-contextual item flavor text (all items × all zones — see `twinbee_resource_flavor.go`)
- [ ] Fishing integration (zone-specific fish tables, expedition-only fish)

### Phase R5 — Economy Integration
- [ ] Resource sell system at Thom Krooke
- [ ] CHA sell-price check
- [ ] Crafting recipe system (basic recipes from Section 8.2)
- [ ] `!craft` command and recipe discovery via `!lore`

### Phase R6 — Polish & Balance
- [ ] Node count tuning per zone (balance supply vs. reward)
- [ ] Combat Interrupt roll tuning (not too frequent, not too rare)
- [ ] `twinbee_resource_flavor.go` full population
- [ ] Zone condition interactions (Heat Stack DC increase, Instability lockout, etc.)

---

*End of Resource & Combat Integration Document.*
*Deprecates standalone harvesting and dungeon run systems entirely.*
*Reference alongside all companion docs in Claude Code sessions.*
