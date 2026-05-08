# GogoBee — Remaining Systems Specification
> Covers: Skill Proficiency · Arena Integration · Misty & Arina · Pete Bot · Economy Balance · Crafting Recipes · Quest System
> **Version:** 1.0

---

# Part 1: Skill Proficiency System

## 1.1 Proficiency Bonus by Level

```
Level 1–4:   +2
Level 5–8:   +3
Level 9–12:  +4
Level 13–16: +5
Level 17–20: +6
```

## 1.2 Class Skill Proficiencies

Each class is proficient in a fixed set of skills. Proficiency adds the proficiency bonus to skill checks using that stat.

| Class | Proficient Skills (choose N from list at creation) |
|---|---|
| Fighter | Choose 2 from: Athletics, Acrobatics, Animal Handling, History, Insight, Intimidation, Perception, Survival |
| Rogue | Choose 4 from: Acrobatics, Athletics, Deception, Insight, Intimidation, Investigation, Perception, Performance, Persuasion, Sleight of Hand, Stealth |
| Mage | Choose 2 from: Arcana, History, Insight, Investigation, Medicine, Religion |
| Cleric | Choose 2 from: History, Insight, Medicine, Persuasion, Religion |
| Ranger | Choose 3 from: Animal Handling, Athletics, Insight, Investigation, Nature, Perception, Stealth, Survival |

## 1.3 Race Bonus Proficiencies

| Race | Bonus |
|---|---|
| Human | +1 skill of choice |
| Half-Elf | +2 skills of choice |
| All others | No bonus skill proficiency |

## 1.4 Expertise

At Level 6, Rogues choose 2 skills they are proficient in and gain **Expertise** — double proficiency bonus on those skills.

At Level 10, Rangers choose 1 skill for Expertise.

## 1.5 Skill → Stat Map (Full)

| Skill | Stat | Harvest Action | Notes |
|---|---|---|---|
| Athletics | STR | `!mine` (partial) | Climbing, grappling, forced entry |
| Acrobatics | DEX | — | Avoiding traps, balance |
| Sleight of Hand | DEX | `!fish` (partial) | Pickpocket, item manipulation |
| Stealth | DEX | `!hide` | Avoid detection; -Threat Clock |
| Arcana | INT | `!essence` (partial) | Identify magic, lore checks |
| History | INT | — | Zone lore, ancient knowledge |
| Investigation | INT | `!scavenge` (partial) | Find hidden items, trap disarm |
| Nature | WIS | `!forage` (partial) | Zone knowledge, plant ID |
| Perception | WIS | `!search` | Spot traps, enemies, hidden doors |
| Insight | WIS | NPC interactions | Detect lies, hidden motives |
| Survival | WIS | `!forage` (primary) | Track, navigate, forage |
| Animal Handling | WIS | Pet interactions | Pet happiness checks |
| Medicine | WIS | Stabilize (future party) | Death save assist |
| Religion | WIS/INT | Cleric lore | Undead/divine lore bonus |
| Intimidation | CHA | Combat morale | Enemy morale checks |
| Persuasion | CHA | Thom Krooke discount | NPC favor, pricing |
| Deception | CHA | NPC bypass | Disguise, misdirection |
| Performance | CHA | Community events | Flavor; morale effects |

## 1.6 Go Data Model

```go
type SkillProficiencies struct {
    PlayerID    string          `json:"player_id"`
    Proficient  []string        `json:"proficient"`  // skill IDs
    Expertise   []string        `json:"expertise"`   // subset of proficient
}

func SkillBonus(skill string, stats BaseStats, prof SkillProficiencies, profBonus int) int {
    base := statModifier(statForSkill(skill), stats)
    if contains(prof.Expertise, skill) {
        return base + (profBonus * 2)
    }
    if contains(prof.Proficient, skill) {
        return base + profBonus
    }
    return base
}
```

---

# Part 2: Arena System — D&D Integration

## 2.1 Overview

The Arena is a standalone PvP-adjacent system preserved from the original GogoBee. It now uses full D&D combat resolution. Two players (or a player vs. a scaled AI opponent) face off in a best-of-one encounter.

Arena is **not** an expedition. It happens outside zones. No supplies consumed. No Threat Clock. Hospital rules apply on death.

## 2.2 Arena Combat Rules

- Full D&D action economy applies (Action, Bonus, Reaction, Movement)
- Both combatants start at full HP with all resources restored
- Spell slots, Rage uses, and Superiority Dice reset at arena start
- No loot drops — streak rewards replace loot
- No Concentration spells that last beyond the fight (no Hunters Mark persisting round-to-round against a different target — this is a single encounter)
- Banned: Raise Dead, Animate Dead, Polymorph (too disruptive in PvP)

## 2.3 Streak System

Winning consecutive arena matches builds a streak. Streak resets on any loss or player death.

| Streak | Reward |
|---|---|
| 3 | +25 coins |
| 5 | +60 coins + Uncommon loot roll |
| 7 | +120 coins + Rare loot roll |
| 10 | +250 coins + title: "Streak Warrior" |
| 15 | +500 coins + Epic loot roll + title: "Arena Champion" |
| 20 | +1,000 coins + Legendary loot roll + title: "The Unbroken" |
| 25+ | +1,500 coins/win + Legendary loot roll; TwinBee makes a speech |

## 2.4 Matchmaking

- Prefer within 3 levels of player
- If no match available: AI opponent at player's level (named, with personality — see 2.5)
- Players can challenge specific users: `!arena challenge @player`

## 2.5 Named AI Arena Opponents

When no human opponent is available, TwinBee deploys a named AI fighter:

| Name | Class/Subclass | Level | Personality |
|---|---|---|---|
| **Vance the Unimpressed** | Fighter/Champion | Scales ±2 | Bored. Has seen everything. Still wins. |
| **Sparrow** | Rogue/Thief | Scales ±2 | Fast, chatty, apologizes when she hits you. |
| **The Unmarked** | Mage/Abjuration | Scales ±2 | Says nothing. Blocks everything. Eventually wins. |
| **Brother Aldric** | Cleric/War | Scales ±2 | Cheerful. Heals himself constantly. Annoyingly patient. |
| **Fenn** | Ranger/Gloom Stalker | Scales ±2 | You won't see him until it's too late. |

## 2.6 Arena Commands

| Command | Description |
|---|---|
| `!arena` | Enter the arena queue |
| `!arena challenge @player` | Challenge a specific player |
| `!arena streak` | Check current streak and rewards |
| `!arena leaderboard` | Top 10 streaks in TwinBee community |
| `!arena history` | Last 5 arena results |

---

# Part 3: Misty & Arina — Full NPC Specs

## 3.1 Misty

**Full name:** Misty  
**Reference:** Pokémon Gym Leader parody — the one who always seemed like she was about to challenge you even when she was being helpful.  
**Role:** Quest-giver, Insight specialist, aquatic zone liaison  
**Location:** Sunken Temple (primary), occasionally the docks near Thom Krooke's shop  

**Personality:** Confident, competitive, secretly warm. Misty has done everything you're attempting and done it faster and she will absolutely tell you so. But she'll also give you the information you need, because she respects effort even when she doesn't show it. Her advice is blunt and accurate.

**Voice:** Direct, slightly impatient, occasional dry pride. Never cruel. *"You're going into the Sunken Temple? Fine. Try not to drown immediately."*

**Skill Interactions:**
- Persuasion DC 12 → Misty provides a map of the current Sunken Temple room layout (reduces Perception DC by 2 for one run)
- Insight DC 14 → Misty reveals a hidden quest node in the Sunken Temple
- Intimidation DC 16 → Misty gives you a discount on aquatic fishing gear (Thom Krooke liaison)

**Quest Hooks:**
1. *The Coral Throne* — Collect 5 Deep Coral from the Sunken Temple within one expedition. Reward: Uncommon aquatic weapon + 200 XP.
2. *Harpooned* — Defeat 3 Merrow in a single expedition day. Reward: Merrow Harpoon item + 150 XP.
3. *The Lanternfish* — Catch a Dar'eth Lanternfish. Reward: Rare crafting component + 300 XP.
4. *Tidal Timing* — Reach the Boss room before the Day 6 Tidal Event fires. Reward: Tidal Crystal (guaranteed) + 400 XP + Misty's begrudging respect (CHA +1 in her presence permanently).

**Go Struct:**
```go
type NPCMisty struct {
    PlayerRelation map[string]int `json:"player_relation"` // playerID → relationship score
    QuestsGiven    map[string][]string `json:"quests_given"` // playerID → quest IDs
}
```

---

## 3.2 Arina

**Full name:** Arina  
**Reference:** Arina from Twinkle Star Sprites (1996 SNK shooter) — a young witch who fights with stars and magic. High energy, genuinely enthusiastic about magical things.  
**Role:** Crafting specialist, Arcana quest-giver, magic item identifier  
**Location:** The Workshop district near Thom Krooke; also appears in Feywild and Underforge zones  

**Personality:** Genuinely excited about magic, slightly chaotic, deeply knowledgeable underneath the enthusiasm. Arina identifies magic items for free if you pass her Arcana check — she just wants to *look at it*. She keeps notes on everything and her notes are brilliant and unorganized.

**Voice:** Fast, warm, enthusiastic. Sometimes trails off into a tangent. Always lands back on the useful thing. *"Oh that's — wait, is that a Flame Tongue? Can I — I won't touch it, I just want to — okay I touched it a little. It's a +1 Flame Tongue, here's how the attunement works, also the previous owner probably had fire resistance built up, you should track that."*

**Skill Interactions:**
- Arcana DC 12 → Arina identifies any magic item for free (normally costs 50 coins at Thom Krooke)
- Investigation DC 14 → Arina reveals a crafting recipe the player doesn't have yet
- Persuasion DC 15 → Arina provides spell component materials at 30% discount

**Quest Hooks:**
1. *Essence Collection* — Harvest 3 units of Fire Essence from the Underforge. Reward: Arina crafts a Potion of Fire Resistance for free + 200 XP.
2. *Fey Study* — Return from the Feywild Crossing with Timelock Amber. Reward: Arina teaches a crafting recipe (player's choice from her list) + 250 XP.
3. *The Grand Experiment* — Bring Arina one material from each of 5 different zones. Reward: Arina crafts a unique item scaled to player level + 500 XP + permanent "Arina's Favorite" status (Arcana checks in her presence always succeed).
4. *Void Specimen* — Return from the Abyss Portal with Void Essence. Reward: Legendary crafting anchor + 600 XP + Arina is genuinely speechless for one full message, which she immediately recovers from.

---

# Part 4: Pete Bot

## 4.1 Overview

Pete is a sibling Matrix bot that posts community announcements, expedition updates, and game news to the TwinBee room. Pete is distinct from GogoBee — GogoBee handles player interactions; Pete handles broadcast.

Pete is named after Pete (from Pete's Dragon? No — Pete is just Pete. A reliable, solid name for a reliable, solid bot).

## 4.2 Pete's Trigger Events

| Event | Pete Posts |
|---|---|
| New zone unlocked (community milestone) | Zone announcement with flavor description |
| Player completes a Tier 5 zone | Public callout with player name + brief TwinBee summary |
| Streak milestone hit (10+) | Streak announcement |
| ARM rate change >0.5% | Mortgage rate update (Thom Krooke voice) |
| Server maintenance | Brief downtime notice |
| Expedition Day 7+ | Daily expedition bulletin (anonymized or opted-in) |
| Seasonal event start/end | Event announcement |
| Game update / patch | Changelog post |
| Community milestone (activity threshold) | TwinBee mood boost announcement |

## 4.3 Pete's Voice

Pete does not have TwinBee's drama or Thom Krooke's salesmanship. Pete is a professional. Pete posts what happened, when it happened, and what it means for you. Occasionally dry. Never excessive.

> *📣 Pete: Riesz has completed the Dragon's Lair on Day 18. Infernax is defeated. The mountain is, temporarily, quiet. Legendary loot confirmed.*

> *📣 Pete: ARM rate update — 7.4% this week. Effective mortgage rate: 9.4%. Payments process Sunday. Thom Krooke sends his regards and a reminder that refinancing is available.*

> *📣 Pete: TwinBee's mood is elevated this week following strong community activity. Dungeon events have been upgraded accordingly. Go while it lasts.*

## 4.4 Pete Bot Spec

```go
type PetePost struct {
    ID         string    `json:"id"`
    TriggerType string   `json:"trigger_type"`
    Content    string    `json:"content"`
    RoomID     string    `json:"room_id"`
    PostedAt   time.Time `json:"posted_at"`
    PlayerID   string    `json:"player_id,omitempty"` // if player-specific
}

// Pete trigger types
const (
    PeteTriggerZoneUnlock      = "zone_unlock"
    PeteTriggerTier5Complete   = "tier5_complete"
    PeteTriggerStreakMilestone  = "streak_milestone"
    PeteTriggerMortgageRate    = "mortgage_rate"
    PeteTriggerMaintenance     = "maintenance"
    PeteTriggerExpeditionDay7  = "expedition_day7"
    PeteTriggerSeasonalEvent   = "seasonal_event"
    PeteTriggerPatchNotes      = "patch_notes"
    PeteTriggerCommunityBoost  = "community_boost"
)
```

---

# Part 5: Economy Balance

## 5.1 Coin Sources (Daily Average by Level Range)

| Level | Source | Estimated Daily |
|---|---|---|
| 1–5 | Combat loot (Tier 1 zones) | 30–60 coins |
| 1–5 | Passive income (rented room) | 0 |
| 1–5 | Selling resources | 20–40 coins |
| 6–10 | Combat loot (Tier 2–3 zones) | 80–150 coins |
| 6–10 | Passive income (cottage) | 5 coins |
| 6–10 | Arena streak rewards | 10–30 coins/day average |
| 11–15 | Combat loot (Tier 4 zones) | 200–400 coins |
| 11–15 | Passive income (house) | 15 coins |
| 11–15 | Quest rewards | 50–150 coins |
| 16–20 | Combat loot (Tier 5 zones) | 500–1,500 coins |
| 16–20 | Passive income (manor+) | 40–100 coins |
| 16–20 | Legendary resource sales | 1,000–5,000 coins (rare) |

## 5.2 Coin Sinks (Weekly Average by Level Range)

| Level | Sink | Estimated Weekly |
|---|---|---|
| 1–5 | Rent | 15–40 coins |
| 1–5 | Supplies (Tier 1 expedition) | 50–100 coins |
| 1–5 | Hospital (if death) | 20–50 coins |
| 6–10 | Mortgage payment | ~25 coins |
| 6–10 | Supplies (Tier 2–3 expedition) | 120–250 coins |
| 6–10 | Property upgrades (amortized) | ~50 coins |
| 6–10 | Pastel (if hired) | 70 coins (L3 rate × 7) |
| 11–15 | Supplies (Tier 4 expedition) | 400–700 coins |
| 11–15 | Hospital (if death) | 250–400 coins |
| 16–20 | Supplies (Tier 5 expedition) | 800–1,500 coins |
| 16–20 | Hospital (if death) | 600–900 coins |

## 5.3 Balance Notes

**Early game (1–5):** Tight but survivable. Rent is the primary pressure. Players who die repeatedly will struggle. This is intentional — early mistakes have costs, but they're recoverable.

**Mid game (6–10):** Mortgage replaces rent. Passive income offsets slightly. The player is self-sustaining if playing actively. Pastel is a luxury, not a necessity.

**Late game (11–20):** Economy inverts. Players are coin-positive even with heavy expedition spending. The hospital costs become the primary risk at this tier. Legendary resource sales can be boom events.

**Soft caps:** No coin cap. No coin decay. Excess coins are stored. The estate's 100 coins/day passive is significant at high levels but doesn't break the economy — Tier 5 expedition supply costs alone consume it.

**Exploit check:** Passive income + no-expedition play cannot infinitely accumulate beyond ~700 coins/day at max property. This is below the cost of a single Tier 5 expedition supply kit. Players must engage with content to afford content.

---

# Part 6: Crafting Recipes

## 6.1 Basic Recipes (Thom Krooke shop, available from start)

| Recipe | Materials | Output | Cost (labor) |
|---|---|---|---|
| Healing Potion | Shadow Herb ×2 | Potion of Healing (2d4+2 HP) | 10 coins |
| Antitoxin | Shadow Herb ×1 + Grave Soil ×1 | Antitoxin (advantage vs. poison, 3 combats) | 15 coins |
| Torch Bundle | Volcanic Glass ×1 + Scrap Iron ×1 | 5 torches (removes darkness penalty) | 5 coins |
| Basic Arrow Bundle | Darkwood ×1 | 20 Arrows | 3 coins |
| Rope (50 ft) | Beast Pelt ×1 | Hemp Rope | 8 coins |

## 6.2 Intermediate Recipes (Requires Workshop upgrade)

| Recipe | Materials | Output | Cost (labor) |
|---|---|---|---|
| Greater Healing Potion | Shadow Herb ×3 + Corrupted Fey Bloom ×1 | Potion of Greater Healing (4d4+4 HP) | 25 coins |
| Potion of Fire Resistance | Salamander Oil ×1 + Fire Essence ×2 | Resist fire damage (1 combat) | 40 coins |
| Undead Ward Charm | Silver Grave Dust ×1 + Wraith Core ×1 | Amulet: +2 saves vs. undead | 60 coins |
| Shadow Step Cloak | Ghost Silk ×3 + Shadow Essence ×1 | Light armor: Stealth advantage 1/day | 80 coins |
| Drow Poison Blade Coating | Drow Poison ×1 | Weapon: sleep-on-crit for 3 combats | 35 coins |
| Mithral Dagger | Mithral Trace ×1 + Scrap Iron ×3 | Mithral Dagger (light armor prof bypass) | 120 coins |
| Feywild Focus | Fey Dust ×2 + Timelock Amber ×1 | Arcane Focus: +1 to Spell Save DC | 100 coins |
| Drake Scale Armor | Dragon Scale Fragment ×4 + Iron Ore ×3 | Scale Mail +1 | 150 coins |

## 6.3 Advanced Recipes (Requires Forge upgrade + Arina's recipes)

| Recipe | Materials | Output | Cost (labor) |
|---|---|---|---|
| Superior Healing Potion | Dryad's Tears ×1 + Shadow Herb ×4 + Fey Dust ×1 | Superior Healing (8d4+8 HP) | 100 coins |
| Aboleth Lens | Aboleth Mucus ×2 + Black Pearl ×2 | Helm: Telepathy; +2 INT | 200 coins |
| Venom Strike Dagger | Dagger of Venom + Salamander Oil ×1 + Drow Poison ×2 | Upgraded: poison DC increases by 3 | 180 coins |
| Underdark Silk Armor | Cave Spider Silk ×5 + Drow Adamantine ×1 | Studded Leather +2 | 300 coins |
| Planar Ward Ring | Planar Shard ×2 + Void Essence ×1 | Ring: +2 all saves vs. extra-planar creatures | 400 coins |
| Dragon Heart Blade | Thyrak's Core ×1 + Infernax Scale ×1 + Azer Metal Ingot ×2 | Legendary Greatsword: +3, 2d6 fire, fire immunity | 1,000 coins |
| Abyss Gate Key | Portal Fragment ×1 + Shard of the Abyss ×1 | Quest item: reopens the Abyss Portal after collapse | — |

## 6.4 Recipe Discovery

| Method | How |
|---|---|
| Default | Basic recipes known at start |
| `!lore` check (INT DC 14) | Reveals one unknown intermediate recipe |
| Arina quest completion | Grants one recipe of player's choice from intermediate list |
| Arina's Favorite status | Unlocks advanced recipe list |
| Expedition discovery | Random `!scavenge` finds a recipe scroll (adds to known recipes) |

---

# Part 7: Quest System

## 7.1 Quest Types

| Type | Reset | Giver | Reward Scale |
|---|---|---|---|
| Daily Quest | Every 24 hours | TwinBee, Thom Krooke | Small (XP + coins) |
| Weekly Quest | Every Sunday | Misty, Arina, Thom Krooke | Medium (XP + item) |
| NPC Chain Quest | Completion-gated | Misty, Arina | Large (XP + unique reward) |
| Zone Quest | Per expedition | TwinBee (in-zone) | Medium (XP + zone loot) |
| Community Quest | Community milestone | Pete announces | Large (shared reward) |

## 7.2 Daily Quests (Rotating Pool)

Three daily quests available per day. Each player gets the same pool; completion is individual.

| Quest | Requirement | Reward |
|---|---|---|
| First Blood | Win 1 combat in an expedition | 50 XP + 20 coins |
| Gatherer | Harvest 3 resource nodes | 50 XP + 15 coins |
| Rested | Complete a long rest at home | 30 XP + 10 coins |
| The Angler | Catch any fish | 40 XP + 20 coins |
| Keep Moving | Advance through 3 rooms in one day | 60 XP + 25 coins |
| Thom's Errand | Spend 50 coins at Thom Krooke | 40 XP + 20 coins |
| Night Watch | Survive a wandering monster encounter | 70 XP + 30 coins |
| True Aim | Land a critical hit | 60 XP + 25 coins |
| Untouchable | Win a combat without taking damage | 80 XP + 40 coins |

## 7.3 Weekly Quests (Rotating Pool)

One weekly quest per player per week, assigned Sunday.

| Quest | Requirement | Reward |
|---|---|---|
| The Long Road | Complete an expedition (any tier) | 300 XP + 100 coins |
| Bestiary Entry | Defeat 5 different enemy types | 250 XP + Uncommon loot |
| Supply Chain | Complete an expedition with ≥25% supplies remaining | 200 XP + 80 coins |
| Thom's Best Customer | Spend 200 coins at Thom Krooke this week | 150 XP + 10% shop discount next week |
| Ghost Protocol | Complete an expedition with Threat Clock never above 50 | 400 XP + Rare loot |
| Master Crafter | Craft 3 items this week | 250 XP + crafting materials |
| The Streak | Reach a 5-win arena streak | 300 XP + 150 coins |
| Pet Project | Reach Pet Level 3 | 200 XP + pet happiness +20 |

## 7.4 Zone Quests

Zone quests appear at expedition start, assigned by TwinBee. One per expedition.

| Zone | Quest | Reward |
|---|---|---|
| Goblin Warrens | Collect the War Standard before Day 3 | Uncommon weapon + 150 XP |
| Crypt of Valdris | Defeat Valdris without using a rest | Rare loot + 200 XP |
| Forest of Shadows | Find and catch a Phantom Carp | Rare crafting material + 200 XP |
| Sunken Temple | Survive the Tidal Event without retreating | Tidal Crystal (guaranteed) + 300 XP |
| Haunted Manor | Clear the manor in under 8 days | Rare loot + 300 XP |
| Underforge | Complete expedition at Heat Stack ≤5 | Mithral Trace (guaranteed) + 300 XP |
| Underdark | Defeat the Mind Flayer Elder without a party member being Stunned | Epic loot + 500 XP |
| Feywild Crossing | Accept the Thornmother's Bargain | Unique buff + 400 XP |
| Dragon's Lair | Reach Infernax before Day 10 | Epic loot + 600 XP |
| Abyss Portal | Close the portal before Instability reaches 80 | Legendary item + 800 XP |

## 7.5 Quest Commands

| Command | Description |
|---|---|
| `!quests` | Show active daily, weekly, and zone quests |
| `!quest complete` | Check quest completion status |
| `!quest history` | Last 10 completed quests |

## 7.6 Go Data Structures

```go
type PlayerQuest struct {
    PlayerID    string    `json:"player_id"`
    QuestID     string    `json:"quest_id"`
    Type        string    `json:"type"`        // daily, weekly, npc_chain, zone, community
    Status      string    `json:"status"`      // active, complete, expired
    Progress    int       `json:"progress"`    // current count toward requirement
    Target      int       `json:"target"`      // required count
    AssignedAt  time.Time `json:"assigned_at"`
    ExpiresAt   time.Time `json:"expires_at"`
    CompletedAt *time.Time `json:"completed_at"`
    RewardXP    int       `json:"reward_xp"`
    RewardCoins int       `json:"reward_coins"`
    RewardItem  string    `json:"reward_item,omitempty"`
}
```

---
*End of Remaining Systems Specification.*
