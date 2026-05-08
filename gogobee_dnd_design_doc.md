# GogoBee — D&D Integration Design Document
> **Version:** 1.0  
> **Status:** Draft  
> **Scope:** Adventure system overhaul + character system introduction  
> **Target:** Claude Code implementation sessions  
> **See also:** `gogobee_equipment_appendix.md` — full weapon, armor, magic item, and ammo tables

---

## ⚠️ Protected Files — DO NOT MODIFY

The following files must never be rewritten, summarized, shortened, or restructured without explicit human approval:

- All `*_flavor.go` / `*_flavor.txt` files
- Any file containing the header: `DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE`
- NPC spec files (Thom Krooke, Misty, Arina)

---

## 1. Overview & Goals

GogoBee is a Matrix-based bot game built in Go for the TwinBee community. This document specifies the integration of Dungeons & Dragons–inspired mechanics into the existing adventure system. The goal is to deepen gameplay, reward long-term engagement, and create meaningful character identity — without breaking existing player progress or bot infrastructure.

### Design Principles

- **Non-destructive migration.** All existing player data (coins, pets, fishing rank, arena streaks, personality archetypes) must be preserved and mapped forward.
- **Incremental delivery.** Systems are delivered in phases. Each phase must be independently stable before the next begins.
- **Flavor-first.** Mechanical changes must be reflected in flavor text. No dry stat outputs.
- **Bot-native UX.** All interactions happen via Matrix message commands. No external UI.
- **Config-driven.** All tunable values (XP curves, damage multipliers, stat caps, DC thresholds) must be externalized as constants or config — never hardcoded magic numbers.

---

## 2. Existing System Inventory

Before implementing new systems, Claude Code must treat the following as the current ground truth:

| System | Status | Notes |
|---|---|---|
| Adventure engine | Active | Encounter resolution, flavor text, loot |
| Fishing system | Active | Ranks, flavor text, species tables |
| Pet system | Active | Pet bonuses, flavor text |
| Arena / streak system | Active | PvP-style streak tracking |
| Euro economy | Active | Coin earn/spend, Thom Krooke shop |
| Personality archetypes | Active | Player type classification |
| Housing / mortgage mechanics | Specced | FRED API ARM rate integration |
| NPC system | Active | Thom Krooke, Misty, Arina |
| Chat level / perk system | Specced | |
| Tarot system | Active | |
| Blackjack / Uno / Hangman | Active | |

---

## 3. Character System

### 3.1 Races

Races provide passive stat modifiers and unlock flavor interactions. Chosen once at character creation; changeable only via `!respec` (with a cooldown or cost TBD).

| Race | STR | DEX | CON | INT | WIS | CHA | Special Passive |
|---|---|---|---|---|---|---|---|
| Human | +0 | +0 | +0 | +0 | +0 | +0 | +1 to any stat (player choice); +1 free skill proficiency |
| Elf | +0 | +2 | -1 | +1 | +1 | +0 | Darkvision; immune to sleep effects |
| Dwarf | +1 | -1 | +2 | +0 | +1 | -1 | Poison resistance; +bonus vs. underground enemies |
| Halfling | +0 | +2 | +0 | +0 | +1 | +0 | Lucky: once per combat, reroll any natural 1 |
| Orc | +3 | -1 | +2 | -1 | -1 | -1 | Rage: once per combat, +50% damage for one turn |
| Tiefling | +0 | +1 | +0 | +1 | +0 | +2 | Fire resistance; +bonus on CHA skill checks |
| Half-Elf | +0 | +1 | +0 | +1 | +0 | +2 | Two bonus skill proficiencies |

### 3.2 Classes

Classes define combat role, ability access, and equipment proficiency. Chosen at character creation alongside race.

| Class | Primary Stats | HP Die | Armor Prof. | Weapon Prof. | Role |
|---|---|---|---|---|---|
| Fighter | STR, CON | d10 | All | All | Sustained melee damage, tanking |
| Rogue | DEX, INT | d8 | Light | Simple, hand crossbow | Burst damage, stealth, utility |
| Mage | INT, WIS | d6 | None | Daggers, staves | Spell burst, crowd control |
| Cleric | WIS, CHA | d8 | Medium, shields | Simple | Healing, buffs, undead bonus |
| Ranger | DEX, WIS | d8 | Light, medium | Simple, martial ranged | Nature affinity, pet/fishing bonuses |

**Class–Archetype Inference (for existing user migration):**

| Personality Archetype | Suggested Class | Suggested Race |
|---|---|---|
| Arena-heavy / aggressive | Fighter | Orc or Human |
| Economy-focused / trader | Rogue | Halfling or Human |
| Lore-seeker / chat-heavy | Mage | Elf or Tiefling |
| Supportive / community | Cleric | Dwarf or Half-Elf |
| Fishing / exploration | Ranger | Human or Elf |

### 3.3 Ability Scores

The six D&D ability scores apply across all systems. Base values are set at character creation (point-buy or roll — implementation decision TBD), then modified by race and class.

| Score | Abbreviation | Governs |
|---|---|---|
| Strength | STR | Melee damage, carry weight, STR skill checks |
| Dexterity | DEX | Dodge chance, initiative, ranged, DEX skill checks |
| Constitution | CON | Max HP, resist conditions |
| Intelligence | INT | Spell power, lore, crafting, INT skill checks |
| Wisdom | WIS | Healing power, perception, WIS skill checks |
| Charisma | CHA | NPC interactions, shop discounts, CHA skill checks |

**Stat Modifier Formula:** `modifier = floor((score - 10) / 2)`  
Standard range: 8–18 at creation (pre-racial bonuses). Hard cap: 20.

### 3.4 Go Data Model — Character Sheet

```go
// CharacterSheet represents the full D&D character state for a player.
type CharacterSheet struct {
    PlayerID  string    `json:"player_id"`
    Race      Race      `json:"race"`
    Class     Class     `json:"class"`
    Level     int       `json:"level"`
    XP        int       `json:"xp"`
    Stats     BaseStats `json:"stats"`
    HP        HPBlock   `json:"hp"`
    Abilities []Ability `json:"abilities"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type BaseStats struct {
    STR int `json:"str"`
    DEX int `json:"dex"`
    CON int `json:"con"`
    INT int `json:"int"`
    WIS int `json:"wis"`
    CHA int `json:"cha"`
}

type HPBlock struct {
    Current int `json:"current"`
    Max     int `json:"max"`
    TempHP  int `json:"temp_hp"`
}

type Race  string
type Class string

const (
    RaceHuman   Race = "human"
    RaceElf     Race = "elf"
    RaceDwarf   Race = "dwarf"
    RaceHalfling Race = "halfling"
    RaceOrc     Race = "orc"
    RaceTiefling Race = "tiefling"
    RaceHalfElf Race = "half_elf"

    ClassFighter Class = "fighter"
    ClassRogue   Class = "rogue"
    ClassMage    Class = "mage"
    ClassCleric  Class = "cleric"
    ClassRanger  Class = "ranger"
)
```

---

## 4. Leveling System

### 4.1 XP Curve

| Level | XP Required (cumulative) | Abilities Unlocked |
|---|---|---|
| 1 | 0 | Class starter ability |
| 2 | 300 | +1 stat point |
| 3 | 900 | Class ability tier 2 |
| 5 | 2700 | Subclass selection prompt |
| 7 | 6500 | Class ability tier 3 |
| 10 | 14000 | Prestige ability |
| 15 | 34000 | Elite ability |
| 20 | 85000 | Legendary ability |

XP is earned from: adventure encounters, arena wins, fishing (Ranger bonus), completing daily quests, NPC interactions.

### 4.2 Level-Up Events

On level-up, the bot:
1. Posts a congratulatory message with flavor text (class-specific)
2. Displays new HP max
3. Announces any unlocked abilities
4. Prompts for stat point allocation if applicable
5. Prompts for subclass selection at level 5

---

## 5. Combat Engine Overhaul

### 5.1 Combat Resolution Flow

```
1. INITIATIVE
   Both combatants roll d20 + DEX modifier.
   Higher initiative acts first.
   Ties broken by DEX score, then random.

2. ATTACK ROLL (per turn)
   Attacker rolls d20 + relevant stat modifier + proficiency bonus.
   Proficiency bonus = ceil(level / 4) + 1

3. HIT CHECK
   Attack roll >= target's Armor Class (AC) → HIT
   Attack roll < AC → MISS (flavor text generated)
   Natural 20 → CRITICAL HIT (damage doubled, special crit flavor)
   Natural 1 → FUMBLE (miss + possible debuff, fumble flavor)

4. DAMAGE ROLL (on hit)
   Damage = weapon damage die + STR or DEX modifier (class-dependent)
   Apply resistances, vulnerabilities, conditions

5. CONDITION CHECKS
   Check for active conditions (poisoned, stunned, blessed, etc.)
   Apply saving throws where required

6. REPEAT until one combatant reaches 0 HP
```

### 5.2 Armor Class

AC is derived from equipped armor + DEX modifier (where applicable):

| Armor Type | Base AC | DEX Bonus | Classes |
|---|---|---|---|
| Unarmored | 10 | +DEX mod | Mage, Rogue |
| Light Armor | 11 | +DEX mod | Rogue, Ranger, Cleric |
| Medium Armor | 13 | +DEX mod (max +2) | Fighter, Ranger, Cleric |
| Heavy Armor | 16 | None | Fighter |
| Shield | +2 | — | Any proficient class |

### 5.3 Conditions

| Condition | Effect | Save to Remove |
|---|---|---|
| Poisoned | -1d4 HP per turn; -2 to attack rolls | CON DC 12 |
| Stunned | Skip turn | CON DC 14 |
| Frightened | -2 to all rolls; must flee if possible | WIS DC 12 |
| Blessed | +1d4 to attack and saving throws | N/A (duration-based) |
| Burning | -1d6 HP per turn | DEX DC 10 |
| Silenced | Cannot cast spells | WIS DC 13 |

### 5.4 Go Data Model — Combat

```go
type CombatState struct {
    PlayerID    string       `json:"player_id"`
    EnemyID     string       `json:"enemy_id"`
    Round       int          `json:"round"`
    PlayerTurn  bool         `json:"player_turn"`
    Conditions  []Condition  `json:"conditions"`
    Log         []CombatLine `json:"log"`
}

type CombatLine struct {
    Round   int    `json:"round"`
    Actor   string `json:"actor"`
    Action  string `json:"action"`
    Flavor  string `json:"flavor"`
    Damage  int    `json:"damage"`
    Roll    int    `json:"roll"`
}

type Condition struct {
    Name     string    `json:"name"`
    Duration int       `json:"duration"` // turns remaining; -1 = permanent until saved
    SaveStat string    `json:"save_stat"`
    SaveDC   int       `json:"save_dc"`
}
```

---

## 6. Abilities & Spells

### 6.1 Ability Types

- **Passive:** Always active. Applied automatically during combat resolution.
- **Active:** Player-triggered via `!use <ability>`. Consumes resources (mana/rage/focus).
- **Reactive:** Triggers on specific conditions (e.g., "when hit", "on critical hit").

### 6.2 Resource Systems by Class

| Class | Resource | Resets On |
|---|---|---|
| Fighter | Stamina (3 pts) | Short rest or long rest |
| Rogue | Focus (2 pts) | Short rest |
| Mage | Mana / Spell Slots | Long rest only |
| Cleric | Divine Favor (3 pts) | Long rest |
| Ranger | Focus (2 pts) | Short rest |

### 6.3 Starter Abilities by Class (Level 1)

| Class | Ability | Type | Effect |
|---|---|---|---|
| Fighter | Second Wind | Active (1 Stamina) | Heal 1d10 + level HP |
| Rogue | Sneak Attack | Passive | +1d6 damage when attacking with advantage or flanking |
| Mage | Magic Missile | Active (1 Slot) | 3 darts, 1d4+1 force damage each, auto-hit |
| Cleric | Healing Word | Active (1 Favor) | Restore 1d4 + WIS modifier HP to self or ally |
| Ranger | Hunter's Mark | Active (1 Focus) | Mark target; +1d6 damage to marked target for 3 turns |

---

## 7. Equipment & Item System

### 7.1 Item Rarity

| Rarity | Color Code | Drop Rate | Stat Bonus Range |
|---|---|---|---|
| Common | ⬜ | 60% | +0 to +1 |
| Uncommon | 🟩 | 25% | +1 to +2 |
| Rare | 🟦 | 10% | +2 to +4 |
| Epic | 🟪 | 4% | +4 to +6 |
| Legendary | 🟧 | 1% | +6 to +10, special property |

### 7.2 Equipment Slots

```
Head | Chest | Legs | Hands | Feet
Weapon (main hand) | Off-hand (shield or weapon)
Ring (x2) | Amulet
```

### 7.3 Attunement

- Players may attune to a maximum of **3 magic items** at a time.
- Attuning requires a short rest.
- Unequipping an attuned item frees the slot immediately.
- Non-attuned magic items provide no bonus.

### 7.4 Go Data Model — Equipment

```go
type Inventory struct {
    PlayerID  string            `json:"player_id"`
    Equipped  EquipmentSlots    `json:"equipped"`
    Bag       []Item            `json:"bag"`
    Attuned   []string          `json:"attuned"` // item IDs, max 3
}

type EquipmentSlots struct {
    Head      *Item `json:"head"`
    Chest     *Item `json:"chest"`
    Legs      *Item `json:"legs"`
    Hands     *Item `json:"hands"`
    Feet      *Item `json:"feet"`
    MainHand  *Item `json:"main_hand"`
    OffHand   *Item `json:"off_hand"`
    Ring1     *Item `json:"ring_1"`
    Ring2     *Item `json:"ring_2"`
    Amulet    *Item `json:"amulet"`
}

type Item struct {
    ID         string            `json:"id"`
    Name       string            `json:"name"`
    Rarity     string            `json:"rarity"`
    Slot       string            `json:"slot"`
    StatBonus  map[string]int    `json:"stat_bonus"` // e.g. {"str": 2, "dex": 1}
    MagicProp  string            `json:"magic_prop"` // empty string if none
    Attunement bool              `json:"attunement"`
    ClassReq   []Class           `json:"class_req"` // empty = any class
}
```

---

## 8. Monster Bestiary

Every enemy has a stat block. Flavor text generation must reference class-specific encounter descriptions.

### 8.1 Stat Block Schema

```go
type MonsterStatBlock struct {
    ID           string            `json:"id"`
    Name         string            `json:"name"`
    CR           float32           `json:"cr"` // Challenge Rating
    HP           int               `json:"hp"`
    AC           int               `json:"ac"`
    STR          int               `json:"str"`
    DEX          int               `json:"dex"`
    CON          int               `json:"con"`
    INT          int               `json:"int"`
    WIS          int               `json:"wis"`
    CHA          int               `json:"cha"`
    Resistances  []string          `json:"resistances"`
    Immunities   []string          `json:"immunities"`
    Abilities    []MonsterAbility  `json:"abilities"`
    LootTable    []LootEntry       `json:"loot_table"`
    XPValue      int               `json:"xp_value"`
    FlavorRef    string            `json:"flavor_ref"` // key into flavor text map
}

type LootEntry struct {
    ItemID   string  `json:"item_id"`
    Weight   float32 `json:"weight"` // probability weight, not percentage
}
```

### 8.2 Starter Bestiary Entries

| Monster | CR | HP | AC | Special |
|---|---|---|---|---|
| Goblin | 1/4 | 7 | 13 | Nimble Escape (disengage/hide as bonus) |
| Skeleton | 1/4 | 13 | 13 | Immune to poison; resist piercing |
| Orc Grunt | 1/2 | 15 | 13 | Aggressive: bonus action move toward enemy |
| Troll | 5 | 84 | 15 | Regeneration: +10 HP/turn unless fire/acid damage this turn |
| Wyvern | 6 | 110 | 13 | Multiattack; poison sting (CON DC 15) |
| Ancient Dragon | 20 | 367 | 22 | Legendary actions; breath weapon; frightful presence |

---

## 9. Skill Check System

Skill checks resolve non-combat interactions. Roll d20 + relevant stat modifier vs. a Difficulty Class (DC).

### 9.1 Skill → Stat Mapping

| Skill | Stat | Example Uses |
|---|---|---|
| Athletics | STR | Climbing, forcing doors |
| Acrobatics | DEX | Avoiding traps, balancing |
| Stealth | DEX | Ambush, avoiding detection |
| Arcana | INT | Identifying magic items, lore |
| Investigation | INT | Finding hidden items, clues |
| Perception | WIS | Spotting enemies, noticing changes |
| Insight | WIS | Detecting NPC deception |
| Persuasion | CHA | Shop discounts (Thom Krooke), quest rewards |
| Intimidation | CHA | Enemy morale checks |
| Deception | CHA | Bypassing guards |

### 9.2 DC Reference

| Difficulty | DC |
|---|---|
| Trivial | 5 |
| Easy | 10 |
| Medium | 15 |
| Hard | 20 |
| Very Hard | 25 |
| Near Impossible | 30 |

### 9.3 NPC Integration

- **Thom Krooke** (Persuasion, CHA): Passing DC 15 Persuasion unlocks a 10% shop discount for the session.
- **Misty** (Insight, WIS): Passing DC 12 Insight reveals a hidden quest option or bonus item.
- **Arina** (Arcana, INT): Passing DC 14 Arcana unlocks a crafting recipe or item identification for free.

---

## 10. Rest System

### 10.1 Short Rest

- Command: `!rest short`
- Duration: No in-game time cost
- Cooldown: 1 hour real-time between short rests
- Effects:
  - Recover HP equal to 1d6 + CON modifier (x2 at levels 5+)
  - Reset short-rest abilities (Rogue Focus, Fighter Stamina)
  - Does NOT reset spell slots

### 10.2 Long Rest

- Command: `!rest long`
- Duration: Requires housing (Thom Krooke inn/owned property) or camp item
- Cooldown: 24 hours real-time between long rests
- Effects:
  - Full HP recovery
  - Full resource reset (all classes)
  - Spell slots restored
  - Conditions cleared (except curses)

### 10.3 Housing Integration

Players with purchased housing (see housing/mortgage mechanic spec) get access to long rests at any time via `!rest long` from their home. Inn use costs coins (Thom Krooke shop price: configurable).

---

## 11. Player Onboarding & Migration

### 11.1 New User Flow

Triggered on first interaction with any adventure, combat, or D&D command:

```
Step 1 — Race selection  (numbered choices, bot message)
Step 2 — Class selection (numbered choices, bot message)
Step 3 — Stat summary confirmation (show derived stats, HP, AC)
Step 4 — Welcome message with class-specific flavor text
```

No DM blast. No walls of text. Each step is a separate message with clear numbered options. Players can type `!setup` at any time to restart.

### 11.2 Existing User Migration

**Trigger:** First interaction with any D&D-layer command post-launch.

**Flow:**

```
Bot: ⚔️ The adventure system has evolved!
     Your coins, pets, fishing rank, and arena streak are safe.
     
     Based on your history, we think you might be:
     
     🗡️ Human Fighter
     
     [1] Yes, that's me
     [2] No, let me choose
     [3] Skip for now — remind me later
```

Option 3 sets a `pending_setup` flag. Bot re-prompts every 7 days until setup is complete. All non-D&D commands continue to work normally. D&D-gated commands return a friendly prompt to complete setup.

### 11.3 Auto-Assignment (Fallback)

After 30 days with `pending_setup = true`, the system auto-assigns:
- Race and Class based on archetype inference table (Section 3.2)
- Notifies the player: "We gave you a starting class! Use `!setup` to change it anytime."

### 11.4 Data Migration Map

| Existing Field | Migration Action |
|---|---|
| `coins` | Preserved unchanged |
| `pet` | Preserved; Ranger gets passive pet bonus on top |
| `fishing_rank` | Preserved; Ranger gets fishing affinity bonus |
| `arena_streak` | Preserved; now modified by class bonuses in combat |
| `personality_archetype` | Used for class/race inference; retained as flavor field |
| `housing` (if live) | Maps to long rest eligibility |
| `chat_level` (if live) | Maps to XP starting value (formula TBD) |

---

## 12. Cross-System Integration

### 12.1 Arena System

- Combat now uses full D&D resolution (initiative, AC, attack rolls, conditions)
- Arena streak bonuses scale with level (e.g., +5% damage per 3-streak, additive with class bonuses)
- Class-specific arena flavor text required (see flavor file conventions)

### 12.2 Pet System

- All pets: +5% XP gain (passive)
- Ranger pets: Additionally grant +2 to Perception checks and +1d4 to first attack each combat
- Pet flavor text on combat entry must reference the pet name

### 12.3 Fishing System

- Fishing rank bonuses unchanged
- Ranger: +20% rare fish catch rate; access to exclusive `Ranger's Rod` item (combat-usable as a thrown weapon)
- High-value fish can now be sold to Thom Krooke for coins via `!sell fish`

### 12.4 Economy / Thom Krooke

- Equipment available at Thom Krooke's shop (Common and Uncommon only; Rare+ are dungeon drops)
- CHA Persuasion checks apply to all purchases
- Housing still functions as the primary long rest enabler
- New shop category: Potions (healing potions, antitoxin, spell components)

---

## 13. Commands Reference

| Command | Description |
|---|---|
| `!setup` | Begin or restart character creation |
| `!sheet` | Display your character sheet |
| `!stats` | Show ability scores and modifiers |
| `!level` | Show current level and XP progress |
| `!abilities` | List known abilities and resources |
| `!equip <item>` | Equip an item from inventory |
| `!unequip <slot>` | Unequip item from slot |
| `!attune <item>` | Attune to a magic item |
| `!inventory` | List all carried items |
| `!use <ability>` | Activate an active ability |
| `!rest short` | Take a short rest |
| `!rest long` | Take a long rest (requires inn/home) |
| `!roll <dice>` | Roll arbitrary dice (e.g., `!roll 2d6+3`) |
| `!check <skill>` | Make a skill check |
| `!respec` | Respec race/class (cooldown/cost TBD) |

---

## 14. Implementation Phases

### Phase 1 — Character Foundation
- [ ] `CharacterSheet` struct and DB schema
- [ ] Race and class constants, stat modifier functions
- [ ] New user onboarding flow
- [ ] Existing user migration flow (archetype inference + `pending_setup` flag)
- [ ] `!setup`, `!sheet`, `!stats` commands

### Phase 2 — Combat Engine Overhaul
- [ ] Initiative system
- [ ] Attack roll vs. AC resolution
- [ ] Critical hit / fumble handling
- [ ] Condition system
- [ ] Saving throws
- [ ] Update arena system to use new combat engine

### Phase 3 — Leveling & Abilities
- [ ] XP tracking and level-up triggers
- [ ] Level-up notification flow (flavor text required)
- [ ] Starter ability per class (Level 1)
- [ ] `!use`, `!abilities` commands
- [ ] Resource tracking (Stamina, Focus, Mana, Divine Favor)

### Phase 4 — Equipment & Loot
- [ ] Item struct and inventory DB schema
- [ ] Equipment slots and equip/unequip
- [ ] Attunement system (3-item limit)
- [ ] Loot table integration into adventure/combat drops
- [ ] Thom Krooke shop expansion (equipment + potions)

### Phase 5 — Skill Checks & NPC Integration
- [ ] Skill check resolution function
- [ ] DC table constants
- [ ] Wire Thom Krooke, Misty, Arina interactions to skill checks
- [ ] `!check` command

### Phase 6 — Rest System & Housing
- [ ] Short rest implementation
- [ ] Long rest implementation
- [ ] Housing check for long rest eligibility
- [ ] Inn cost at Thom Krooke
- [ ] `!rest` command

### Phase 7 — Monster Bestiary & Cross-System Polish
- [ ] Full starter bestiary (Section 8.2 minimum)
- [ ] Monster-specific flavor text (do not touch existing flavor files — add new)
- [ ] Cross-system bonuses (pet/fishing/housing integration)
- [ ] Pete bot announcement post draft

---

## 15. Configuration Constants

All of the following must be defined as named constants or config entries — never inline:

```go
const (
    MaxAbilityScore         = 20
    AttunementLimit         = 3
    ShortRestCooldownHours  = 1
    LongRestCooldownHours   = 24
    MigrationAutoAssignDays = 30
    MigrationReminderDays   = 7
    ProficiencyBase         = 2      // added to ceil(level/4)
    CritMultiplier          = 2      // damage multiplier on natural 20
    ShopDiscountCHA         = 0.10   // 10% discount on passing Persuasion
    RangerFishRateBonus     = 0.20   // +20% rare fish catch
    PetXPBonus              = 0.05   // +5% XP all pets
)
```

---

## 16. Out of Scope (This Version)

The following are explicitly deferred to future design docs:

- Multiplayer / party system
- PvP with full D&D ruleset (arena streak system is sufficient for now)
- Subclass trees (specced at Level 5 prompt, full spec TBD)
- Crafting system
- World map / dungeon zones
- Spell list expansion beyond Level 1 starters
- Prestige / multi-classing

---

*End of document. All implementation sessions using this doc must treat Section ⚠️ Protected Files as an absolute constraint.*
