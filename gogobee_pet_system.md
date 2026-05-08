# GogoBee — Pet System
> **Companion to:** `gogobee_dnd_design_doc.md`, `gogobee_resource_combat_integration.md`
> **Version:** 1.0

---

## 1. Overview

Pets are companions that provide passive bonuses, expedition benefits, and community flavor. Every player can own one pet at a time (expandable with housing upgrades). Pets are obtained from zones, Thom Krooke's shop, or special events.

Pets are not combat-active (no independent attack actions) unless the player is a **Ranger** — Rangers have a deep bond with their pet that unlocks active combat participation.

---

## 2. Pet Species

Each pet species is tied to one or more zones. Some are available at Thom Krooke's shop; others are expedition-exclusive drops.

### Tier 1 Pets (Common — Thom Krooke shop or Tier 1–2 zones)

| Pet | Species | Zone Origin | Passive Bonus | Ranger Bonus |
|---|---|---|---|---|
| **Pip** the Sparrow | Bird | Forest of Shadows | +5% XP from all sources | Scout: +2 Perception in outdoor zones |
| **Grit** the Terrier | Dog | Goblin Warrens | +10% coin drops from enemies | Flush: once/combat, enemy loses Stealth |
| **Marble** the Cat | Cat | Haunted Manor | +1 to Stealth checks | Pounce: +1d4 on first attack vs. surprised enemy |
| **Flint** the Toad | Toad | Sunken Temple | Poison resistance (half damage) | Toxic Spit: on-hit chance to Poison enemy (CON DC 11) |
| **Cinder** the Lizard | Lizard | Underforge | Fire resistance (half damage) | Heat Body: melee attackers take 1d4 fire |
| **Dusk** the Bat | Bat | Crypt of Valdris | Darkvision (no darkness penalty) | Echolocate: enemy Stealth checks at disadvantage |

### Tier 2 Pets (Uncommon — Tier 3–4 zones or special shop)

| Pet | Species | Zone Origin | Passive Bonus | Ranger Bonus |
|---|---|---|---|---|
| **Vex** the Imp | Imp | Abyss Portal | +2 to INT checks | Diabolism: once/combat, impose disadvantage on one enemy save |
| **Luma** the Will-o-Wisp | Wisp | Feywild Crossing | Immune to Frightened condition | Feylight: blind one enemy for 1 turn (WIS DC 14) |
| **Kobb** the Kobold Scholar | Kobold | Dragon's Lair | +2 to all Arcana checks | Trap Sense: automatic Trap Detected result once per zone |
| **Mire** the Shadow Cat | Shadow Cat | Underdark | Advantage on Stealth in darkness | Phase: pass through one trap without triggering (1/expedition) |
| **Thorn** the Vine Sprite | Sprite | Forest of Shadows | +10% harvested material yield | Tangle: Restrain one enemy for 1 turn (STR DC 13) |
| **Salt** the Crab | Giant Crab | Sunken Temple | Advantage on Athletics (grapple/shove) | Shell Shield: once/combat, grant +3 AC for 1 turn |

### Tier 3 Pets (Rare — Tier 5 zones or legendary drops)

| Pet | Species | Zone Origin | Passive Bonus | Ranger Bonus |
|---|---|---|---|---|
| **Ember** the Drake Hatchling | Drake | Dragon's Lair | +15% fire damage dealt | Breath: 2d6 fire in cone (DEX DC 13); 1/combat |
| **Nyx** the Shadow | Shadow | Haunted Manor (boss) | Immune to necrotic damage | Life Drain: on-hit, steal 1d6 HP from enemy |
| **Shard** the Crystal Elemental | Elemental | The Abyss Portal | Spell Save DC +1 | Prismatic Burst: on crit, random elemental damage +2d6 |
| **Wilder** the Displacer Beast Cub | Displacer Beast | Forest of Shadows (rare) | First attack each combat has disadvantage vs. player | Phase Shift: once/combat, one missed attack becomes a miss regardless of roll |

### Legendary Pets (Unique — world drops, events only)

| Pet | Species | Origin | Passive Bonus | Ranger Bonus |
|---|---|---|---|---|
| **Blaze** the Phoenix Chick | Phoenix | Dragon's Lair boss (1% drop) | On player death: once per week, auto-stabilize instead of dying | Rebirth Flame: on death, deal 3d10 fire to all enemies in room |
| **Echo** the Aboleth Spawn | Aboleth | Sunken Temple boss (0.5% drop) | +3 to all INT checks; telepathy flavor text unlocked | Mind Rend: once/combat, one enemy skips their turn (WIS DC 18) |

---

## 3. Pet Acquisition

### 3.1 Thom Krooke's Shop
Tier 1 pets available for purchase at base cost:

| Pet Tier | Shop Cost |
|---|---|
| Tier 1 | 150 coins |

Thom Krooke rotates stock weekly — not all Tier 1 pets available simultaneously. Two–three available at any time.

### 3.2 Expedition Drops
Pets drop from zone-specific enemies and boss encounters:

| Drop Type | Trigger | Drop Rate |
|---|---|---|
| Zone companion | Specific enemy killed in native zone | 3–8% |
| Boss companion | Boss defeated | 10–20% for Tier 1–2; 5% for Tier 3 |
| Legendary | Specific world conditions | 0.5–1% |

Drop flavor: TwinBee narrates the pet encounter, not a standard loot drop. The pet chooses you as much as you choose it.

### 3.3 Community Events
Certain pets are only obtainable during seasonal or community milestone events. These are announced by Pete bot.

---

## 4. Pet Leveling

Pets gain XP alongside the player — a percentage of all earned XP is shared with the active pet.

| Player Level Range | Pet XP Share |
|---|---|
| 1–5 | 10% of player XP |
| 6–10 | 8% of player XP |
| 11–15 | 6% of player XP |
| 16–20 | 5% of player XP |

### 4.1 Pet Level Thresholds

| Pet Level | XP Required | Benefit |
|---|---|---|
| 1 | 0 | Base passive bonus |
| 2 | 200 | Passive bonus +25% |
| 3 | 600 | Passive bonus +50%; Ranger bond ability improves |
| 4 | 1,500 | Passive bonus +75%; new flavor text unlocked |
| 5 | 3,000 | Passive bonus ×2; Ranger bond ability at full strength |

### 4.2 Pet Happiness

Pets have a **Happiness** score (0–100, default 60) that modifies their effectiveness:

| Happiness | Passive Bonus Modifier |
|---|---|
| 80–100 | +20% |
| 50–79 | Normal |
| 25–49 | -20% |
| 0–24 | Passive disabled; pet is sulking |

**Happiness modifiers:**

| Event | Change |
|---|---|
| Fed daily (Pastel or manual) | +5 |
| Expedition completed | +10 |
| Player death | -10 |
| Player offline > 3 days without Pastel | -15/day |
| Pastel hired (active) | Happiness maintained at current level (no decay) |
| Rare item found (player excited) | +3 |
| Long rest at home | +8 |

**Feeding command:** `!pet feed` — costs 2 coins/day (Pastel handles this automatically if hired).

---

## 5. Ranger Pet Bond

Rangers have a unique **Pet Bond** that upgrades with Ranger level and pet level. The bond activates the Ranger Bonus column from Section 2.

### 5.1 Bond Tiers

| Tier | Requirement | Effect |
|---|---|---|
| **Bonded** | Ranger + any pet | Ranger Bonus active; pet participates in combat (flavor) |
| **Trusted** | Ranger Lv5 + Pet Lv2 | Ranger Bonus improves (see each pet); pet Happiness decay halved |
| **Linked** | Ranger Lv10 + Pet Lv4 | Pet gains a second ability (see Section 5.2); pet XP share increases to 15% |
| **Fused** | Ranger Lv15 + Pet Lv5 | Once/long rest: pet ability recharges mid-combat |

### 5.2 Pet Second Abilities (Linked Bond)

| Pet | Second Ability |
|---|---|
| Pip the Sparrow | Alarm: warns of wandering monsters before they attack (no surprise round) |
| Grit the Terrier | Fetch: retrieve one dropped item per combat without using an action |
| Marble the Cat | Distract: enemy loses Reaction for 1 turn |
| Vex the Imp | Curse: target has -2 to saves for 2 turns |
| Ember the Drake | Wing Buffet: AoE knockback (DEX DC 14 or Prone) |
| Nyx the Shadow | Merge: Ranger becomes Invisible for 1 turn (once/combat) |

---

## 6. Pet Housing

Pets require housing space. Base housing supports **one pet**. Additional pets (future multi-pet system) require:

- **Cottage+:** supports 2 pets
- **House+:** supports 3 pets
- **Manor+:** supports 5 pets

Without adequate housing, a second pet cannot be adopted until the first is released or housing is upgraded.

---

## 7. Pet Commands

| Command | Description |
|---|---|
| `!pet` | Show current pet status, level, happiness, bonuses |
| `!pet adopt <name>` | Adopt a pet from expedition drop (confirmation required) |
| `!pet buy` | Open Thom Krooke pet shop |
| `!pet feed` | Feed pet (+happiness; 2 coins) |
| `!pet name <name>` | Rename your pet |
| `!pet release` | Release pet (cannot be undone; XP lost) |
| `!pet bond` | Show Ranger bond tier and requirements |

---

## 8. Go Data Structures

```go
type PlayerPet struct {
    PlayerID    string    `json:"player_id"`
    PetID       string    `json:"pet_id"`        // species ID
    Name        string    `json:"name"`          // player-given name
    Level       int       `json:"level"`         // 1–5
    XP          int       `json:"xp"`
    Happiness   int       `json:"happiness"`     // 0–100
    BondTier    string    `json:"bond_tier"`     // bonded, trusted, linked, fused
    AdoptedAt   time.Time `json:"adopted_at"`
    LastFed     time.Time `json:"last_fed"`
}

type PetSpecies struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    Species       string `json:"species"`
    Tier          int    `json:"tier"`           // 1, 2, 3, legendary
    ZoneOrigin    string `json:"zone_origin"`
    PassiveBonus  string `json:"passive_bonus"`  // description
    RangerBonus   string `json:"ranger_bonus"`   // description
    ShopAvailable bool   `json:"shop_available"`
    ShopCost      int    `json:"shop_cost"`
    DropRate      float32 `json:"drop_rate"`
    FlavorRef     string `json:"flavor_ref"`
}
```

---
*End of Pet System.*
