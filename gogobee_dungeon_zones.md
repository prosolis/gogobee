# GogoBee — Dungeon Zones & TwinBee GM System
> **Companion to:** `gogobee_dnd_design_doc.md`, `gogobee_equipment_appendix.md`
> **Version:** 1.0
> **Status:** Draft

---

## ⚠️ Protected Files — DO NOT MODIFY

All existing `*_flavor.go` / `*_flavor.txt` files and any file with the header  
`DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE` are off-limits.  
New zone flavor text is **additive only** — new files, never edits to existing ones.

---

## 1. Design Philosophy

### TwinBee as GM

TwinBee is not a passive dice roller. TwinBee is the **Dungeon Master** — a character with a distinct voice, personality, and narrative presence. Every dungeon interaction passes through TwinBee's narration layer. TwinBee:

- Describes rooms, atmosphere, and transitions in prose
- Reacts to dice outcomes with flavor (not just "you hit for 8 damage")
- Tracks dungeon state (which rooms cleared, what was looted, boss alive/dead)
- Warns, misleads, celebrates, and mourns alongside the player
- Has a **GM Mood** that shifts based on community activity and player choices (see Section 3)

TwinBee speaks in a consistent narrative voice — dramatic but not overwrought, with a dry wit. Think a seasoned DM who's seen it all but still gets excited when someone rolls a nat 20.

### Zone Design Principles

- Every zone has a **biome identity**: visual language, enemy faction, lore, and environmental hazards that are internally consistent.
- Zones are **tiered by CR (Challenge Rating)** and gated by player level.
- Each zone has **5 room types**: Entry, Exploration, Trap, Elite, and Boss.
- Rooms are procedurally sequenced but not fully random — the boss room is always last.
- Zones have **flavor text files** (`zone_<name>_flavor.go`) that must be populated before the zone ships. No zone launches with placeholder text.
- Loot tables are zone-specific. A dungeon shouldn't drop jungle gear.

---

## 2. Zone Tier Overview

| Tier | Level Range | CR Range | Zones |
|---|---|---|---|
| 1 — Beginner | 1–3 | 1/8–1 | Goblin Warrens, The Crypt |
| 2 — Apprentice | 3–5 | 1–3 | Forest of Shadows, Sunken Temple |
| 3 — Journeyman | 5–8 | 3–6 | Haunted Manor, The Underforgе |
| 4 — Veteran | 8–12 | 6–10 | The Underdark, Feywild Crossing |
| 5 — Legendary | 12–20 | 10–20 | Dragon's Lair, The Abyss Portal |

Players cannot enter a zone more than **2 tiers above** their current level. TwinBee delivers an in-character warning if they try.

---

## 3. TwinBee GM System

### 3.1 Message Format

TwinBee GM messages are visually distinct from standard bot output. They use a consistent prefix and narrative tone:

```
🎭 TwinBee: The passage ahead smells of mildew and old iron.
            Torchlight catches something glinting in the corner —
            but so does something else.

            What do you do?
            [!advance] [!search] [!retreat]
```

Regular bot confirmations (stat updates, loot drops, XP gains) use standard output format. Only narrative moments go through the TwinBee GM voice.

### 3.2 GM Mood System

TwinBee's narration tone shifts based on a **Mood Score** (0–100, default 50):

| Mood Range | State | Narration Flavor |
|---|---|---|
| 80–100 | Elated | Generous descriptions, hints, bonus lore drops |
| 60–79 | Engaged | Standard D&D narration, balanced challenge |
| 40–59 | Neutral | By-the-book, efficient descriptions |
| 20–39 | Brooding | Ominous, shorter descriptions, harder encounters |
| 0–19 | Wrathful | Cryptic, no hints, elite enemy buffs, rare loot suppressed |

**Mood modifiers:**

| Event | Mood Change |
|---|---|
| Player completes a zone | +10 |
| Player dies | -5 |
| Natural 20 rolled | +3 |
| Natural 1 rolled | -2 |
| Community milestone (Lemmy/Matrix activity) | +15 |
| Long server downtime / maintenance | -20 |
| Player uses `!taunt` command vs. TwinBee | -10 (flavor only) |
| Player sends a `!compliment` to TwinBee | +5 |

Mood decays toward 50 at a rate of ±2 per hour (passive rebalancing).

### 3.3 TwinBee Voice Guidelines

These must be followed in all flavor text files for TwinBee GM lines:

- **Never use second person for room descriptions.** Not "You see a door" — "A door stands at the far end."
- **Always use second person for outcomes.** "You drive the blade home. The goblin crumples."
- **Nat 20 lines** must feel cinematic. One memorable sentence.
- **Nat 1 lines** must be funny but not cruel. Self-deprecating, not punishing.
- **Boss entry lines** get 3–4 sentences minimum. This is a moment.
- **Death lines** are respectful. No mockery. TwinBee honors the fallen.

### 3.4 TwinBee Commands

| Command | Description |
|---|---|
| `!advance` | Move to the next room |
| `!search` | Search current room for hidden items/traps (Perception check) |
| `!retreat` | Return to previous room (no encounter re-trigger) |
| `!flee` | Attempt to escape combat (DEX check vs. enemy) |
| `!lore` | Ask TwinBee for zone flavor/history (WIS or INT check for bonus detail) |
| `!taunt` | Taunt TwinBee (mood penalty, funny response) |
| `!compliment` | Compliment TwinBee (mood bonus, modest response) |
| `!status` | TwinBee reports current room, cleared rooms, HP, conditions |
| `!map` | Show dungeon progress as ASCII room map |

---

## 4. Dungeon Structure

### 4.1 Room Sequence

Every dungeon run follows this structure:

```
[Entry Room]
    ↓
[Exploration Room x1–2]
    ↓
[Trap Room]
    ↓
[Exploration Room x1–2]
    ↓
[Elite Room]
    ↓
[Boss Room]
```

Total rooms per run: **6–8**. Room count scales slightly with zone tier.

### 4.2 Room Types

**Entry Room**
- No combat. Atmospheric description only.
- TwinBee sets the scene. Mood-influenced prose.
- May contain a passive Perception check hint (DC 10) about what's ahead.

**Exploration Room**
- 1–2 standard enemies from the zone roster.
- May contain a search opportunity after combat (hidden loot, lore, or trap trigger).
- Loot drop on enemy defeat.

**Trap Room**
- No enemies. Environmental hazard.
- Player is warned by TwinBee (vaguely). Perception/Investigation check to detect.
- Failure triggers the trap effect.
- Success allows disarm (Thieves' Tools for Rogues; Athletics/DEX for others).

**Elite Room**
- 1 elite enemy (higher CR than standard zone enemies) or a named mini-boss.
- Guaranteed uncommon loot drop.
- TwinBee delivers a distinct entry description.

**Boss Room**
- 1 zone boss. Unique stat block, 2+ abilities, multi-phase if legendary tier.
- Guaranteed rare+ loot drop.
- Zone completion XP bonus on kill.
- TwinBee delivers a full boss introduction scene.
- On death: TwinBee delivers a zone-closing narration line.

### 4.3 Dungeon State Persistence

```go
type DungeonRun struct {
    RunID        string        `json:"run_id"`
    PlayerID     string        `json:"player_id"`
    ZoneID       string        `json:"zone_id"`
    CurrentRoom  int           `json:"current_room"`
    TotalRooms   int           `json:"total_rooms"`
    RoomsCleared []int         `json:"rooms_cleared"`
    BossDefeated bool          `json:"boss_defeated"`
    LootCollected []string     `json:"loot_collected"`  // item IDs
    GMMood       int           `json:"gm_mood"`
    StartedAt    time.Time     `json:"started_at"`
    CompletedAt  *time.Time    `json:"completed_at"`
    Abandoned    bool          `json:"abandoned"`
}
```

A dungeon run persists until: boss defeated, player dies, or player uses `!abandon`. Runs cannot be paused indefinitely — a 24-hour inactivity timer auto-abandons with no XP penalty but no loot.

---

## 5. Zone Specifications

---

### Zone 01 — Goblin Warrens
**Tier:** 1 | **Level Range:** 1–3 | **CR Range:** 1/8–1

> *A network of fetid tunnels burrowed beneath the Merchant's Road. The smell arrives before the sounds — smoke, rot, and something worse. TwinBee advises keeping one hand on your blade.*

**Atmosphere:** Low ceilings, torchlight, crude traps, cackling in the dark.  
**Faction:** Goblins, Hobgoblins  
**Flavor File:** `zone_goblin_warrens_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Goblin Sneak | 1/4 | 7 | 13 | Nimble Escape; pack tactics (+1d4 if ally adjacent) |
| Goblin Archer | 1/4 | 7 | 13 | Ranged only; Scurry (disengage after attack) |
| Hobgoblin Grunt | 1/2 | 11 | 18 | Martial Advantage (pack tactics variant); formation bonus |
| Worg | 1/2 | 26 | 13 | Knock Prone on hit (STR DC 13); Goblin rider option |
| Goblin Shaman | 1 | 22 | 11 | Casts Burning Hands (2d6 fire, DEX DC 11); Healing Word on allies |
| Hobgoblin Warchief | 2 | 52 | 18 | **ELITE** — Leadership aura (+1d4 to ally attacks); multiattack |

**Zone Boss: Grol the Unbroken**  
CR 3 | HP 65 | AC 17  
*A Bugbear war-chief who has united three goblin clans under his banner. Wears a belt of troll teeth and smells like he earned them.*

Abilities:
- **Surprise Attack:** +2d6 damage if player has not acted this combat
- **Heart of Hruggek:** Crits deal max damage (no roll)
- **Terrifying Roar** (1/combat): All enemies gain +2 to attack rolls for 2 turns; player must make WIS DC 13 or Frightened

Loot Table:
- `wpn_handaxe_+1` (15%)
- `arm_studded_leather_+1` (10%)
- `Grol's Belt` — unique: +2 STR while equipped (5%)
- Coins: 2d10 × 5

---

### Zone 02 — The Crypt of Valdris
**Tier:** 1 | **Level Range:** 1–3 | **CR Range:** 1/4–1

> *The iron gate hangs open — someone left in a hurry. Carved into the stone above: "HERE LIES VALDRIS. DO NOT." The rest has been chiseled away. TwinBee declines to speculate.*

**Atmosphere:** Stone corridors, dripping water, candles that shouldn't still be burning.  
**Faction:** Undead  
**Flavor File:** `zone_crypt_valdris_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Skeleton | 1/4 | 13 | 13 | Immune: poison, exhaustion; Resist: piercing, bludgeoning |
| Zombie | 1/4 | 22 | 8 | Undead Fortitude: CON DC 5+(damage dealt) or survive at 1 HP |
| Shadow | 1/2 | 16 | 12 | Strength Drain: hit reduces STR by 1d4 (recovered on long rest) |
| Specter | 1 | 22 | 12 | Incorporeal: non-magical weapons deal half damage; Life Drain |
| Wight | 3 | 45 | 14 | **ELITE** — Life Drain; raises slain humanoids as zombies |
| Flameskull | 4 | 40 | 13 | **ELITE** — Casts Fireball (8d6, DEX DC 13); Rejuvenation (returns in 1 hour unless holy water used) |

**Zone Boss: Valdris the Unburied**  
CR 5 | HP 97 | AC 17  
*A lich-aspirant who got most of the way there. Cunning enough to be dangerous, failed enough to be bitter.*

Abilities:
- **Corrupting Touch:** +4d6 necrotic damage; target max HP reduced by damage dealt (long rest restores)
- **Legendary Resistance** (2/combat): Auto-succeed one failed saving throw
- **Call of the Grave** (recharge 5–6): Summons 1d4 skeletons from room bones
- **Phase 2** (below 50% HP): Gains Fly speed; spells deal +1d6 necrotic

Loot Table:
- `Valdris's Phylactery Shard` — quest item (always drops)
- `arm_cloak_of_protection` (12%)
- `wpn_mace_of_smiting` (8%)
- Coins: 3d10 × 4

---

### Zone 03 — Forest of Shadows
**Tier:** 2 | **Level Range:** 3–5 | **CR Range:** 1–4

> *The forest was beautiful once. Travelers still say so, usually right before they stop saying anything at all. The trees lean in when you're not looking. TwinBee has noted this is not a metaphor.*

**Atmosphere:** Ancient forest, twisted paths, eerie silence, bioluminescent fungi, things in the canopy.  
**Faction:** Beasts, Fey-corrupted creatures, Bandits  
**Flavor File:** `zone_forest_shadows_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Dire Wolf | 1 | 37 | 14 | Pack Tactics; Knock Prone (STR DC 13) |
| Bandit Captain | 2 | 65 | 15 | Multiattack (3 hits); Parry reaction (+3 AC vs one attack) |
| Owlbear | 3 | 59 | 13 | Beak and Claw multiattack; Grapple on Claw hit |
| Dryad (Corrupted) | 2 | 22 | 11 | Barkskin self-buff (+2 AC); Fey Charm (WIS DC 14 or waste a turn) |
| Displacer Beast | 3 | 85 | 13 | Displacement: first hit on player each round has disadvantage; Avoidance |
| Green Hag | 3 | 82 | 17 | **ELITE** — Illusory Appearance; Invisible Passage; Weakness curse |

**Zone Boss: The Hollow King**  
CR 6 | HP 142 | AC 15  
*What was once a forest guardian, now a vessel for something older and angrier. The antlers are real. The eyes are not.*

Abilities:
- **Corrupting Aura:** Players within melee range make WIS DC 14 each turn or lose their bonus action
- **Root Surge** (recharge 5–6): Entangles player (Restrained, STR DC 15 to escape; takes 2d8 bludgeoning)
- **Devour Light:** Extinguishes magical light sources for 2 turns; player AC -2
- **Phase 2** (below 40% HP): Summons 2 Dire Wolves; gains Reckless Attack

Loot Table:
- `The Hollow Crown` — unique helm: +2 WIS; Fey Sight (advantage on Perception in nature) (6%)
- `arm_hide_+2` (10%)
- `wpn_longbow_+1` (12%)
- Forest Essence (crafting material, always drops x1–3)

---

### Zone 04 — Sunken Temple of Dar'eth
**Tier:** 2 | **Level Range:** 3–5 | **CR Range:** 2–5

> *The tide went out thirty years ago and never fully came back. The temple stayed wet anyway. Something down there keeps it that way. TwinBee suggests waterproofing your spellbook.*

**Atmosphere:** Flooded stone chambers, barnacled pillars, salt smell, alien glyphs, things that swim in the dark water.  
**Faction:** Kuo-toa, Water Elementals, Aboleth-touched  
**Flavor File:** `zone_sunken_temple_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Kuo-toa | 1/4 | 18 | 13 | Amphibious; Slippery (escape grapple auto); Sticky Shield reaction |
| Kuo-toa Whip | 1 | 65 | 11 | Divine Eminence (+2d6 psychic on hit); Pincer Staff (grapple) |
| Water Elemental | 5 | 114 | 14 | Whelm (Grapple + drowning, CON DC 13); Freeze reaction |
| Merrow | 2 | 45 | 13 | Harpoon pull (15 ft); Multiattack; amphibious |
| Aboleth Thrall | 3 | 60 | 12 | Psychic Bond (WIS DC 14 or reveal your next action to enemy); Mucus Cloud |

**Zone Boss: The Dreaming Aboleth**  
CR 10 | HP 135 | AC 17  
*Ancient. Patient. It has been waiting in this temple since before the city above was built. It has had time to plan.*

Abilities:
- **Tentacle Multiattack:** Three tentacle attacks; hit inflicts Diseased (no magical healing for 24 real hours unless cured)
- **Enslave** (recharge 6): WIS DC 14 or Charmed — player skips their turn and moves toward Aboleth
- **Mucus Cloud:** Melee attackers must make CON DC 14 or transform skin to membrane (take 6d6 acid damage if not submerged — flavor: gasping)
- **Legendary Actions (3/round):** Detect (free), Tail Swipe (2 LA), Psychic Drain (3 LA — 4d6 psychic, player max HP reduced by half damage dealt until long rest)

Loot Table:
- `Aboleth's Eye` — unique amulet: +2 INT; Telepathy (passive lore checks auto-succeed) (5%)
- `arm_breastplate_+1` (10%)
- Refined Aboleth Mucus (rare crafting material) (always x1)
- Coins: 5d10 × 8

---

### Zone 05 — Haunted Manor of Blackspire
**Tier:** 3 | **Level Range:** 5–8 | **CR Range:** 4–8

> *The manor has been for sale for eleven years. Every buyer has either left immediately or not left at all. The real estate listing describes it as 'full of character.' TwinBee finds this accurate.*

**Atmosphere:** Victorian decay, impossible architecture, portraits whose eyes follow movement, cold spots, locked rooms that weren't locked before.  
**Faction:** Undead, Shadows, Vampiric  
**Flavor File:** `zone_manor_blackspire_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Shadow | 1/2 | 16 | 12 | STR drain; resist non-magical; vulnerable to radiant |
| Wraith | 5 | 67 | 13 | Incorporeal movement; Life Drain (max HP reduction); sunlight sensitivity |
| Banshee | 4 | 58 | 12 | Wail (WIS DC 13 or drop to 0 HP — 1/combat); Corrupting Touch; undead nature |
| Vampire Spawn | 5 | 82 | 15 | Bite (life drain, heals vampire); Misty Step; charm gaze (WIS DC 13) |
| Poltergeist | 2 | 22 | 12 | Forceful slam (invisible, disadvantage to hit); Telekinetic Thrust |
| Revenant | 5 | 136 | 13 | **ELITE** — Regeneration (+10 HP/turn); cannot die permanently until goal fulfilled; STR DC 20 grapple |

**Zone Boss: Lord Aldric Blackspire (Vampire)**  
CR 13 | HP 144 | AC 16  
*He has watched seven generations of his bloodline die in this house. He considers himself the only survivor. He may be right.*

Abilities:
- **Multiattack:** Unarmed strike + bite every turn
- **Charm:** WIS DC 17 or Charmed for 24 hours (broken by damage)
- **Children of the Night** (1/combat): Summons 2d6 Swarms of Bats or 3d6 Rats
- **Mist Form:** When reduced to 0 HP, becomes mist and retreats to coffin (must destroy coffin in 30-turn window or he fully regenerates)
- **Legendary Resistance** (3/combat)
- **Phase 2 (Mist Form destroyed / cornered):** Enrages — all attacks have advantage; Charm becomes AoE (all players WIS DC 17)

Loot Table:
- `Blackspire Signet Ring` — unique: +2 CHA; undead NPCs treat you as neutral unless attacked (6%)
- `wpn_sword_of_wounding` (10%)
- `arm_armor_of_resistance_necrotic` (8%)
- Blackspire Deed (story/quest item, always drops)
- Coins: 8d10 × 10

---

### Zone 06 — The Underforge
**Tier:** 3 | **Level Range:** 5–8 | **CR Range:** 4–7

> *The dwarven forge-city of Kharak Dûn was not abandoned. It was sealed from the outside. TwinBee does not have information on what they were sealing in.*

**Atmosphere:** Volcanic caverns, rivers of cooling lava, ancient dwarven stonework, the constant bass note of something very large moving below.  
**Faction:** Fire Elementals, Constructs, Salamanders, Azers  
**Flavor File:** `zone_underforge_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Azer | 2 | 39 | 17 | Fire Aura (1d10 fire to melee attackers); immune to fire; heated weapons |
| Salamander | 5 | 90 | 15 | Constrict (grapple + 2d6 fire/turn); spear multiattack; fire aura |
| Fire Elemental | 5 | 102 | 13 | Fire Form (1d10 contact damage); immune to fire; Water Vulnerability |
| Helmed Horror | 4 | 60 | 20 | Spell Immunity (3 specific spells per instance); multiattack; magic resistance |
| Flameskull | 4 | 40 | 13 | Fireball (8d6, DEX DC 13); Rejuvenation |
| Magmin | 1/2 | 9 | 14 | Death Burst (2d6 fire, 10 ft radius on death); ignite flammable objects |

**Zone Boss: Emberlord Thyrak**  
CR 9 | HP 178 | AC 18  
*The forge-golem that ran Kharak Dûn's furnaces for three centuries. When the dwarves left, it kept working. It has been improving the designs.*

Abilities:
- **Molten Strike:** +4d6 fire damage on hit; target's armor loses 1 AC until repaired (long rest)
- **Forge Breath** (recharge 5–6): 30-ft cone, 10d6 fire damage, DEX DC 16 for half
- **Living Forge:** Heals 15 HP per turn while standing on lava/forge tiles (flavor mechanic)
- **Construct Resilience:** Immune to poison, psychic, charm, exhaustion; resistance to non-magical physical
- **Phase 2 (below 50% HP):** Forge Breath recharge becomes 4–6; spawns 2 Fire Elementals

Loot Table:
- `Thyrak's Core` — crafting material for legendary weapons (always drops)
- `arm_adamantine_armor` (12%)
- `wpn_flame_tongue` (10%)
- Azer Ingots × 1d6 (sell to Thom Krooke for high coin value)

---

### Zone 07 — The Underdark
**Tier:** 4 | **Level Range:** 8–12 | **CR Range:** 7–12

> *There is a world below the world. It has its own cities, its own wars, its own sky — which is stone, and has never once been kind. TwinBee speaks more quietly here. Something might be listening.*

**Atmosphere:** Absolute darkness, phosphorescent mushroom groves, vast underground seas, carved drow cities in the distance, things older than the surface world.  
**Faction:** Drow, Mind Flayers, Beholders (far), Ropers, Hook Horrors  
**Flavor File:** `zone_underdark_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Drow | 1/4 | 13 | 15 | Fey Ancestry (adv. vs charm); Sunlight Sensitivity; Hand Crossbow (poison bolt) |
| Drow Elite Warrior | 5 | 71 | 18 | Multiattack (3 hits); Parry; Drow Poison (unconscious, CON DC 13) |
| Drow Mage | 7 | 45 | 12 | Spells: Lightning Bolt, Fly, Darkness, Faerie Fire; Magic Resistance |
| Mind Flayer | 7 | 71 | 15 | Mind Blast (AoE psychic, INT DC 15 or Stunned); Extract Brain (instakill on stunned); Telepathy |
| Hook Horror | 3 | 75 | 15 | Echoes of Darkness (Blindsight 60 ft); grapple claws; pack tactics |
| Roper | 5 | 93 | 20 | 6 Tendrils (Grapple + restrained); Reel (pull 25 ft); False Appearance |

**Zone Boss: Ilvaras Xunyl, Drow High Priestess**  
CR 12 | HP 162 | AC 16  
*She has killed four previous expeditions from the surface. She keeps their weapons as trophies. She has run out of wall space.*

Abilities:
- **Spells:** Flame Strike, Dispel Magic, Divine Word (CHA DC 18 or Deafened/Blinded/Stunned/killed based on HP), Insect Plague
- **Lolth's Favour:** Once per combat, auto-succeed a failed save
- **Summon Spiders** (1/combat): 2d6 Giant Spiders fill the room
- **Legendary Actions (3):** Melee (1 LA), Cast Cantrip (1 LA), Drain Life (3 LA — 4d10 necrotic)
- **Phase 2 (below 35% HP):** Lolth's Avatar overlay — gains +3 AC, all spells cast as 1 higher slot level

Loot Table:
- `Xunyl's Diadem` — unique helm: +3 CHA; Drow Spells (Darkness, Faerie Fire 1/day each) (5%)
- `arm_drow_chain_+2` (8%)
- `wpn_dagger_of_venom` (10%)
- Spider Silk × 2d4 (high-value crafting material)
- Coins: 15d10 × 12

---

### Zone 08 — Feywild Crossing
**Tier:** 4 | **Level Range:** 8–12 | **CR Range:** 5–10

> *The veil between worlds is thin here. Colors are too saturated. The mushrooms are too large. A small creature made of starlight just offered you a deal. TwinBee advises extreme caution regarding deals.*

**Atmosphere:** Impossible beauty, treacherous whimsy, time distortion, rules that change without notice, bargains with terrible fine print.  
**Faction:** Hags, Redcaps, Will-o-Wisps, Fomorians, Unseelie Fey  
**Flavor File:** `zone_feywild_crossing_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Redcap | 3 | 45 | 13 | Iron Boots (bonus bludgeoning on hit); Outsize Strength; Red Cap (loses power without blood soaking) |
| Will-o-Wisp | 2 | 22 | 19 | Variable Illumination; Ephemeral (+13 to AC against opportunity attacks); drain life |
| Quickling | 1 | 10 | 16 | Blur (attacks at disadvantage); 3 attacks per turn; Evasion |
| Green Hag | 3 | 82 | 17 | Coven magic; Illusory Appearance; Weakness curse |
| Night Hag | 5 | 112 | 17 | Etherealness; Dream Haunting (lose long rest benefit); Heartstone |
| Fomorian | 8 | 149 | 14 | **ELITE** — Evil Eye (WIS DC 14: Frightened or Poisoned or Stunned — GM's choice); stomp AoE |

**Zone Boss: The Thornmother**  
CR 11 | HP 187 | AC 17  
*She has three names. She will offer to tell you all three. She will not tell you what accepting means. The flowers around her throne are beautiful and they are not flowers.*

Abilities:
- **Coven Magic:** Gains additional spell slots based on GM Mood (Elated/Engaged TwinBee = stronger Thornmother)
- **Beguiling Bargain** (1/combat): Offers player a deal — accept a debuff to receive a permanent minor buff. Player chooses.
- **Thorned Grasp:** Restrained + 4d6 piercing per turn, CON DC 16 to break free
- **Shapechange:** Once per combat, adopts a player's appearance; attacks against her have 50% miss chance until DC 17 Investigation check succeeds
- **Phase 2 (below 30% HP):** True Form revealed — all illusions drop; +4d6 psychic on all attacks; Coven summons 2 Night Hags

Loot Table:
- `The Thornmother's Bargain` — unique ring: choose one: +4 to one stat but -2 to another (permanent choice) (always offered, player must accept to receive)
- `wpn_rapier_of_puncturing_+2` (8%)
- `arm_cloak_of_protection_+2` (8%)
- Feywild Essence × 1d4 (legendary crafting material)

---

### Zone 09 — Dragon's Lair (Infernus Peak)
**Tier:** 5 | **Level Range:** 12–20 | **CR Range:** 10–20

> *The mountain has not erupted in forty years. The locals say it is dormant. The locals are wrong about what lives in mountains. TwinBee has prepared an unusually long entry description for this one.*

**Atmosphere:** Scorched stone, rivers of gold coins half-melted into the floor, kobold warrens as outer defenses, growing heat, the unmistakable smell of something ancient and enormous.  
**Faction:** Kobolds, Drakes, Young Dragons, Wyrm  
**Flavor File:** `zone_dragons_lair_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Kobold | 1/8 | 5 | 12 | Pack Tactics; Sunlight Sensitivity; Trap expertise |
| Guard Drake | 2 | 52 | 14 | Multiattack; Draconic Loyalty (immune to fear near dragons) |
| Kobold Scale Sorcerer | 1 | 27 | 15 | Spells: Chromatic Orb, Mage Armor, Shield; Draconic Sorcery |
| Young Red Dragon | 10 | 178 | 18 | Fire Breath (16d6, DC 21); Multiattack; Frightful Presence (WIS DC 16) |
| Dragonborn Cultist | 4 | 71 | 16 | Draconic Gift (breath weapon 2d6, DC 13); Fanatical Devotion (immune to fear) |

**Zone Boss: Infernax the Undying (Ancient Red Dragon)**  
CR 24 | HP 546 | AC 22  
*He remembers when the surface civilizations were just fires. He found the fires amusing. He finds their descendants less so.*

Abilities:
- **Multiattack:** Bite + 2 Claws
- **Fire Breath** (recharge 5–6): 90-ft cone, 26d6 fire damage, DEX DC 24 for half
- **Frightful Presence:** WIS DC 21 or Frightened for 1 minute (all new enemies entering combat)
- **Legendary Resistance** (3/combat): Auto-succeed failed save
- **Legendary Actions (3):** Detect (1), Tail Attack (1), Wing Attack (2 — AoE knockback DEX DC 22)
- **Lair Actions (on initiative count 20):** Magma eruption (2d6 fire all), Volcanic gases (CON DC 13 or Poisoned), Tremor (DEX DC 15 or Prone)
- **Phase 2 (below 50% HP):** Fire Breath recharge 4–6; all fire damage ignores resistance

Loot Table:
- `Scale of Infernax` — legendary material (always drops; crafting anchor for legendary fire weapons)
- `arm_dragon_scale_mail_red` (15%)
- `wpn_flame_tongue_+3` (10%)
- `Ring of Fire Resistance` (12%)
- Dragon Hoard: 50d10 × 100 coins (zone-unique gold flood mechanic)

---

### Zone 10 — The Abyss Portal
**Tier:** 5 | **Level Range:** 15–20 | **CR Range:** 15–20

> *Someone opened a door they should not have opened. The door is still open. Things are still coming through. TwinBee is not making jokes about this one.*

**Atmosphere:** Reality fractures, impossible geometry, constant low psychic pressure, the feeling of being watched by something that has no eyes.  
**Faction:** Demons, Fiends, Corrupted Celestials  
**Flavor File:** `zone_abyss_portal_flavor.go`

#### Enemy Roster

| Enemy | CR | HP | AC | Special |
|---|---|---|---|---|
| Quasit | 1 | 7 | 13 | Invisibility; Scare (WIS DC 10 or Frightened 1 min); poison claws |
| Vrock | 6 | 104 | 15 | Multiattack; Spores (CON DC 14 or 5d10 poison over 24 hrs); Stunning Screech (CON DC 14 or Stunned) |
| Hezrou | 8 | 136 | 16 | Stench (CON DC 14 or Poisoned — disadvantage on attacks); multiattack |
| Nalfeshnee | 13 | 184 | 18 | Multiattack (3 hits); Horror Nimbus (WIS DC 15 or Frightened — AoE); Magic Resistance |
| Marilith | 16 | 189 | 18 | 7 attacks per turn; Parry reaction; Reactive (extra reaction per round); Magic Resistance |

**Zone Boss: Belaxath the Undivided (Balor)**  
CR 19 | HP 262 | AC 19  
*The portal was not summoned. It was torn. Belaxath did the tearing from the other side, with purpose, over three decades. It is annoyed it took that long.*

Abilities:
- **Multiattack:** Longsword + Whip
- **Fire Aura:** 10d6 fire to all melee attackers each turn; weapon attacks deal +3d6 fire
- **Lightning Discharge** (recharge 5–6): 120-ft line, 12d6 lightning, DEX DC 20 half
- **Death Throes:** On death, explodes — 30-ft radius, 20d6 fire, DEX DC 20 half. Destroys non-legendary equipment on a failed save.
- **Magic Resistance:** Advantage on saves vs. spells
- **Legendary Resistance** (3/combat)
- **Demonic Resilience:** Resistant to cold, fire, lightning; immune to poison and non-magical physical
- **Phase 2 (below 40% HP):** Grows to Huge size; all attacks have advantage; Death Throes recharge becomes 4–6

Loot Table:
- `Shard of the Abyss` — legendary: quest item for portal-closing storyline (always drops)
- `wpn_demon_blade` — unique greatsword: +3, deals 2d6 necrotic bonus; wielder cannot be Charmed or Frightened (5%)
- `arm_demon_armor_+3` (5%)
- Portal Fragment × 1 (legendary crafting — required for highest-tier recipes)

---

## 6. Environmental Traps by Zone Tier

### Tier 1 Traps
| Trap | Trigger | Effect | Detect DC | Disarm DC |
|---|---|---|---|---|
| Pit Trap | Floor tile | 2d6 bludgeoning fall | Perception 12 | Thieves' Tools 10 |
| Tripwire Alarm | Doorway | Alerts enemies in next room | Perception 10 | Thieves' Tools 8 |
| Poison Dart | Chest/door | 1d4 piercing + CON DC 11 or Poisoned | Investigation 12 | Thieves' Tools 12 |

### Tier 2 Traps
| Trap | Trigger | Effect | Detect DC | Disarm DC |
|---|---|---|---|---|
| Flame Jet | Pressure plate | 3d6 fire, DEX DC 13 half | Perception 14 | Thieves' Tools 14 |
| Collapsing Ceiling | Rigged support | 4d6 bludgeoning AoE, DEX DC 14 half | Investigation 14 | Athletics 16 |
| Glyph of Warding | Doorframe | 5d8 lightning, DEX DC 14 half | Arcana 14 | Arcana 14 |

### Tier 3–5 Traps
| Trap | Trigger | Effect | Detect DC | Disarm DC |
|---|---|---|---|---|
| Antimagic Field | Room entry | Spells fail; magic items suppressed | Arcana 18 | Arcana 20 |
| Necrotic Miasma | Sarcophagus open | 6d6 necrotic + max HP -10 until long rest | Perception 16 | — (avoid only) |
| Symbol of Death | Inscription | INT DC 20 or drop to 0 HP | Investigation 20 | Arcana 22 |
| Planar Rift | Portal touch | Teleported to random room; 4d10 psychic | Arcana 20 | Arcana 22 |

---

## 7. Dungeon State Go Model — Extended

```go
type ZoneDefinition struct {
    ID            string          `json:"id"`
    Name          string          `json:"name"`
    Tier          int             `json:"tier"`
    LevelMin      int             `json:"level_min"`
    LevelMax      int             `json:"level_max"`
    FlavorRef     string          `json:"flavor_ref"`
    EnemyRoster   []string        `json:"enemy_roster"`   // MonsterStatBlock IDs
    BossID        string          `json:"boss_id"`
    TrapTable     []TrapEntry     `json:"trap_table"`
    LootTable     []LootEntry     `json:"loot_table"`
    RoomCount     RoomCountRange  `json:"room_count"`
    Atmosphere    string          `json:"atmosphere"`
}

type RoomCountRange struct {
    Min int `json:"min"`
    Max int `json:"max"`
}

type TrapEntry struct {
    TrapID    string  `json:"trap_id"`
    Weight    float32 `json:"weight"`
    TierMin   int     `json:"tier_min"`
}

type GMState struct {
    Mood         int       `json:"mood"`        // 0–100
    LastModified time.Time `json:"last_modified"`
}

// GMNarrationType classifies what kind of line TwinBee is generating.
type GMNarrationType string

const (
    GMRoomEntry    GMNarrationType = "room_entry"
    GMCombatStart  GMNarrationType = "combat_start"
    GMCombatEnd    GMNarrationType = "combat_end"
    GMNat20        GMNarrationType = "nat_20"
    GMNat1         GMNarrationType = "nat_1"
    GMBossEntry    GMNarrationType = "boss_entry"
    GMBossDeath    GMNarrationType = "boss_death"
    GMPlayerDeath  GMNarrationType = "player_death"
    GMZoneComplete GMNarrationType = "zone_complete"
    GMTrapDetected GMNarrationType = "trap_detected"
    GMTrapTripped  GMNarrationType = "trap_tripped"
    GMLore         GMNarrationType = "lore"
)
```

---

## 8. Implementation Phases — Dungeon System

### Phase D1 — Zone Infrastructure
- [ ] `ZoneDefinition` struct and zone registry
- [ ] `DungeonRun` state machine (room sequencing, persistence)
- [ ] `!advance`, `!retreat`, `!status`, `!map` commands
- [ ] TwinBee GM message format and mood system
- [ ] Entry Room narration pipeline

### Phase D2 — Tier 1 Zones
- [ ] Goblin Warrens (full enemy roster + boss)
- [ ] Crypt of Valdris (full enemy roster + boss)
- [ ] Trap room system (Tier 1 traps)
- [ ] Flavor text files: `zone_goblin_warrens_flavor.go`, `zone_crypt_valdris_flavor.go`

### Phase D3 — Tier 2 Zones
- [ ] Forest of Shadows
- [ ] Sunken Temple of Dar'eth
- [ ] Tier 2 traps
- [ ] Corresponding flavor files

### Phase D4 — Tier 3 Zones
- [ ] Haunted Manor of Blackspire
- [ ] The Underforge
- [ ] Elite room system
- [ ] Multi-phase boss system
- [ ] Corresponding flavor files

### Phase D5 — Tier 4–5 Zones
- [ ] The Underdark
- [ ] Feywild Crossing
- [ ] Dragon's Lair
- [ ] The Abyss Portal
- [ ] Legendary action system
- [ ] Lair action system
- [ ] Corresponding flavor files

### Phase D6 — GM Polish
- [ ] Full `GMNarrationType` flavor text pass (all types, all zones)
- [ ] TwinBee `!taunt` / `!compliment` interaction lines
- [ ] ASCII `!map` renderer
- [ ] GM Mood community trigger hooks (Lemmy/Matrix activity integration)
- [ ] Pete bot zone announcement posts

---

*End of Dungeon Zones & TwinBee GM Document.*  
*Reference alongside `gogobee_dnd_design_doc.md` and `gogobee_equipment_appendix.md` in all Claude Code sessions.*
