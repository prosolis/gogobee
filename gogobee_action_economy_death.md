# GogoBee — Action Economy, Movement & Death
> **Amends:** `gogobee_dnd_design_doc.md` (Combat Engine section)
> **Version:** 1.0
> **Status:** Draft

---

## ⚠️ Protected Files — DO NOT MODIFY

All existing `*_flavor.go` / `*_flavor.txt` files are off-limits.
New action/death flavor goes in `twinbee_combat_flavor.go` — new file only.

---

## 1. Action Economy

Each combat turn a player has exactly four resources to spend:

```
┌─────────────────────────────────────────────────────┐
│  1 ACTION       The main thing you do this turn      │
│  1 BONUS ACTION A secondary, faster thing            │
│  1 REACTION     One response to something that       │
│                 happens (yours or enemy's turn)       │
│  MOVEMENT       Up to your Speed in feet             │
└─────────────────────────────────────────────────────┘
```

These reset at the start of each turn. Unused resources do not carry over.

---

## 2. Actions

### 2.1 Standard Actions

| Action | Command | Description |
|---|---|---|
| Attack | `!attack` | Make one weapon attack (additional attacks at higher levels) |
| Cast Spell | `!cast <spell>` | Cast a known spell using a spell slot or resource |
| Dash | `!dash` | Double movement speed this turn |
| Disengage | `!disengage` | Move without triggering opportunity attacks this turn |
| Dodge | `!dodge` | Attacks against you have disadvantage until next turn; DEX saves at advantage |
| Help | `!help` | Give an ally advantage on their next attack or ability check |
| Hide | `!hide` | Attempt to become hidden (Stealth vs. enemy Perception) |
| Ready | `!ready <trigger>` | Prepare an action to fire on a specified trigger |
| Use Item | `!use <item>` | Use a consumable (potion, scroll, etc.) |
| Search | `!search` | Examine surroundings for hidden things (Perception or Investigation) |
| Grapple | `!grapple` | Attempt to grab an enemy (STR vs. STR/DEX) |
| Shove | `!shove` | Push or knock prone an enemy (STR vs. STR/DEX) |

### 2.2 Bonus Actions

Bonus actions are only available if a class ability, spell, or item explicitly grants one. You cannot use a bonus action speculatively.

| Bonus Action Source | Who | Description |
|---|---|---|
| Off-hand attack | Any (Light weapon in off-hand) | Attack with off-hand weapon; no stat modifier to damage |
| Second Wind | Fighter | Heal 1d10 + level HP |
| Cunning Action | Rogue (Lv2+) | Dash, Disengage, or Hide as bonus action |
| Healing Word | Cleric | Restore 1d4 + WIS mod HP to self or ally |
| Hunter's Mark | Ranger | Apply Hunter's Mark to target |
| Rage | Orc racial / Berserker | Activate Rage mechanic |
| Misty Step | Tiefling / spell | Short-range teleport |
| Cunning Strike | Rogue (Lv5+) | Apply a Sneak Attack condition rider |

### 2.3 Reactions

Reactions fire in response to a trigger — on your turn or an enemy's turn. Once a reaction is used it is gone until the start of your next turn.

| Reaction | Who | Trigger | Effect |
|---|---|---|---|
| Opportunity Attack | Any | Enemy leaves melee range without Disengaging | One melee attack |
| Parry | Fighter (Lv5+) | Being hit by melee attack | Reduce damage by 1d8 + DEX modifier |
| Uncanny Dodge | Rogue (Lv5+) | Being hit by an attacker you can see | Halve the damage |
| Shield of Faith | Cleric | Ally drops to 0 HP | Spend 1 Divine Favor; ally makes death save at advantage |
| Interception | Fighter (Lv3+) | Ally within melee range hit | Reduce damage to ally by 1d10 + proficiency bonus |
| Counterspell | Mage (Lv5+) | Enemy casts a spell | Spend 1 spell slot to cancel the spell (INT DC = 10 + spell level) |
| Evasion | Rogue / Ranger (Lv7+) | Failing a DEX save | Succeed instead (or take half on actual fail) |

---

## 3. Movement

### 3.1 Base Speed by Race

| Race | Speed |
|---|---|
| Human | 30 ft |
| Elf | 35 ft |
| Dwarf | 25 ft |
| Halfling | 25 ft |
| Orc | 30 ft |
| Tiefling | 30 ft |
| Half-Elf | 30 ft |

### 3.2 Speed Modifiers

| Source | Speed Change |
|---|---|
| Heavy Armor + no STR requirement met | -10 ft |
| Encumbered (carry weight) | -10 ft |
| Heavily Encumbered | -20 ft |
| Haste (spell / potion) | +30 ft |
| Restrained condition | 0 ft (cannot move) |
| Prone condition | Half speed to stand; crawling at half speed |
| Web / Entangled | Half speed until freed |
| Difficult Terrain | Half speed while traversing |

### 3.3 In-Bot Movement Abstraction

Physical movement in feet is tracked in combat for opportunity attacks and AoE targeting. Outside of combat, movement is abstracted into expedition actions — rooms are the unit of exploration, not feet.

**In combat, movement matters for:**
- Entering or leaving melee range (triggers opportunity attacks)
- Moving out of an AoE effect
- Closing distance to use a melee weapon from a ranged position
- Using Dash to reposition after a Disengage

**Movement commands in combat:**

| Command | Description |
|---|---|
| `!move melee` | Close to melee range with target |
| `!move ranged` | Fall back to ranged distance |
| `!move <direction>` | Move relative to current position |
| `!dash` | Double movement as action |
| `!disengage` | Move safely (no OA) as action |

TwinBee narrates all movement. The player never manages feet directly — they state intent, TwinBee resolves geometry.

### 3.4 Difficult Terrain

Certain room conditions create Difficult Terrain that halves movement:

| Zone | Difficult Terrain Source |
|---|---|
| Sunken Temple | Flooded rooms (water, knee-deep or higher) |
| Forest of Shadows | Dense undergrowth, root-covered floors |
| Haunted Manor | Collapsed floor sections, furniture hazards |
| Underforge | Cooled lava flows, slag piles |
| Underdark | Stalagmite fields, cave-in debris |
| Feywild Crossing | Entangling vines, dreamlike terrain shifts |
| Dragon's Lair | Coin drifts, melted-gold floors |
| Abyss Portal | Reality fractures, unstable ground |

TwinBee flags Difficult Terrain on Room Entry. Players do not need to ask.

---

## 4. Opportunity Attacks

An Opportunity Attack fires as a Reaction when an enemy leaves your melee reach without using the Disengage action.

- One melee attack, resolved immediately
- Uses your Reaction for the turn
- Enemies can also trigger OAs against the player
- Certain abilities suppress OAs:
  - **Disengage action** (any class)
  - **Cunning Action: Disengage** (Rogue bonus action)
  - **Blur** (Quickling enemy ability)
  - **Misty Step** (teleportation bypasses OA)

TwinBee announces incoming OA opportunities: *"It's moving. You have a reaction if you want it."*

---

## 5. Ready Action

The Ready action lets you prepare an action to fire on a specific trigger condition, using your Reaction when it fires.

**Syntax:** `!ready <action> when <trigger>`

**Examples:**
```
!ready attack when the goblin moves into melee range
!ready cast magic missile when an enemy casts a spell
!ready dodge when the dragon uses breath weapon
```

If the trigger doesn't occur before the start of your next turn, the readied action expires — the action and the reaction are both spent.

TwinBee confirms the readied action and narrates the trigger condition resolution:
> *"The goblin moves. Your readied attack fires."*

---

## 6. Multi-Attack

Some classes gain additional attacks at higher levels. These are resolved within a single Action.

| Class | Level | Attacks Per Action |
|---|---|---|
| Fighter | 1–4 | 1 |
| Fighter | 5–10 | 2 |
| Fighter | 11–19 | 3 |
| Fighter | 20 | 4 |
| Ranger | 1–4 | 1 |
| Ranger | 5+ | 2 |
| Rogue | Any | 1 (Sneak Attack compensates) |
| Mage | Any | 1 (spells compensate) |
| Cleric | 1–7 | 1 |
| Cleric | 8+ | 2 |

Multi-attack does not apply to spells unless the spell explicitly says so (e.g., Eldritch Blast at higher levels).

**Command:** `!attack` always resolves the full multi-attack sequence. TwinBee narrates each hit and miss individually.

---

## 7. Death Saves

When a player reaches 0 HP they are **Downed** — unconscious, incapacitated, but not yet dead. They begin rolling Death Saving Throws.

### 7.1 Death Save Mechanics

At the start of each of the Downed player's turns, TwinBee rolls 1d20 automatically:

| Roll | Outcome |
|---|---|
| 10+ | **Success.** One success mark. |
| 9 or below | **Failure.** One failure mark. |
| Natural 20 | **Critical Success.** Regain 1 HP immediately; stand up; all death save marks clear. |
| Natural 1 | **Critical Failure.** Two failure marks. |

**Three successes:** Player is **Stabilized** — remains unconscious but no longer making death saves. Regains consciousness with 1 HP after 1d4 turns.

**Three failures:** Player **Dies** — triggers the Hospital system (Section 8).

### 7.2 Stabilizing Another Character (Future Party System)

When party mechanics are implemented: an ally can use their Action to make a DC 10 Medicine check. On success: Downed player is Stabilized immediately.

A Potion of Healing administered to a Downed player automatically Stabilizes them and restores the potion's HP value.

### 7.3 Damage While Downed

If a Downed player takes damage:
- Any damage = one automatic failure mark
- Critical hit = two automatic failure marks
- This can accelerate death before the save sequence completes

TwinBee narrates damage to a Downed player with appropriate gravity.

### 7.4 Go Data Model — Death Saves

```go
type DeathSaveState struct {
    PlayerID   string    `json:"player_id"`
    Downed     bool      `json:"downed"`
    Successes  int       `json:"successes"`  // 0–3
    Failures   int       `json:"failures"`   // 0–3
    Stabilized bool      `json:"stabilized"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

---

## 8. The Hospital — Death & Recovery

Death is not the end. GogoBee's Hospital system handles player death with dignity, real mechanical consequences, and affordable recovery costs.

### 8.1 Death Triggers Hospital

When a player accumulates three Death Save failures:
- Combat ends (they cannot continue)
- Forced extraction triggers (body is recovered — narrative convention)
- Player is moved to Hospital status
- Expedition is marked Abandoned
- Hospital fee is calculated (Section 8.3)

### 8.2 What You Keep on Death

| Asset | Status on Death |
|---|---|
| Character level and XP | **Kept** |
| Equipped gear | **Kept** |
| Inventory items | **Kept** |
| Coins | **Kept** (minus Hospital fee) |
| Harvested materials (current run) | **Lost** (dropped on death) |
| Expedition progress | **Lost** (run abandoned; resume not available after death) |
| Arena streak | **Reset to 0** |
| Pet | **Kept** |
| Housing | **Kept** |

### 8.3 Hospital Costs

Costs are tiered by player level — affordable at all tiers, meaningful without being punishing.

| Player Level | Base Hospital Fee | Notes |
|---|---|---|
| 1–3 | 20 coins | Effectively free early game |
| 4–6 | 50 coins | Minor setback |
| 7–9 | 120 coins | Noticeable but recoverable in one expedition |
| 10–12 | 250 coins | Worth avoiding; not catastrophic |
| 13–15 | 400 coins | Significant; consider playing carefully |
| 16–18 | 600 coins | Serious money; elite play expected |
| 19–20 | 900 coins | The price of power |

**Modifiers:**

| Condition | Fee Modifier |
|---|---|
| Death in Tier 1–2 zone | -20% |
| Death in Tier 3 zone | Base |
| Death in Tier 4 zone | +15% |
| Death in Tier 5 zone | +30% |
| Insufficient coins (debt) | Fee deferred; -10% XP gain until paid |
| Cleric in active party (future) | -25% (they helped stabilize you) |

**Fee cap:** Hospital fee never exceeds 40% of the player's current coin balance, regardless of tier. Nobody gets wiped out by a single bad run.

### 8.4 Recovery Time

After paying the Hospital fee, the player must complete a **Recovery Rest** before re-entering any expedition.

| Player Level | Recovery Rest Duration (real time) |
|---|---|
| 1–5 | 2 hours |
| 6–10 | 4 hours |
| 11–15 | 8 hours |
| 16–20 | 12 hours |

During Recovery Rest the player can still use non-expedition commands: shop, chat, arena spectate, NPC interactions, crafting.

### 8.5 Hospital Commands

| Command | Description |
|---|---|
| `!hospital` | Check current Hospital status and fee |
| `!hospital pay` | Pay fee and begin recovery rest |
| `!hospital status` | Check recovery rest remaining time |

### 8.6 TwinBee on Death

TwinBee delivers death narration with complete respect. No mockery, no irony, no "I told you so." The death narration lines are in `twinbee_gm_flavor.go` under `PlayerDeath`. TwinBee's post-hospital return narration is in `twinbee_expedition_flavor.go` under `ExpeditionResume`.

Hospital arrival is narrated by Thom Krooke, not TwinBee — Thom runs the inn and maintains a working relationship with the local healers. The handoff is clean:

> *TwinBee delivers you to the threshold and steps back.*  
> *Thom Krooke opens the door.*  
> *"Again," Thom says, not unkindly.*

---

## 9. Go Data Structures — Action Economy

```go
// TurnResources tracks a player's available resources for the current combat turn.
type TurnResources struct {
    PlayerID      string `json:"player_id"`
    ActionUsed    bool   `json:"action_used"`
    BonusUsed     bool   `json:"bonus_used"`
    ReactionUsed  bool   `json:"reaction_used"`
    MovementUsed  int    `json:"movement_used"`  // feet used this turn
    MovementTotal int    `json:"movement_total"` // total available this turn
}

// ReadiedAction holds a pending ready action waiting for its trigger.
type ReadiedAction struct {
    PlayerID    string    `json:"player_id"`
    Action      string    `json:"action"`
    TriggerDesc string    `json:"trigger_desc"`
    ExpiresAt   int       `json:"expires_at"` // initiative count
    Fired       bool      `json:"fired"`
}

// HospitalRecord tracks a player's death and recovery state.
type HospitalRecord struct {
    PlayerID       string     `json:"player_id"`
    AdmittedAt     time.Time  `json:"admitted_at"`
    Fee            int        `json:"fee"`
    FeePaid        bool       `json:"fee_paid"`
    PaidAt         *time.Time `json:"paid_at"`
    RecoveryEnds   *time.Time `json:"recovery_ends"`
    ZoneDiedIn     string     `json:"zone_died_in"`
    ExpeditionID   string     `json:"expedition_id"`
    DeathSaves     DeathSaveState `json:"death_saves"`
}
```

---

## 10. Implementation Phases — Action Economy & Death

### Phase A1 — Action Economy
- [ ] `TurnResources` struct and per-turn reset logic
- [ ] Action handler: attack, cast, dash, dodge, disengage, hide, use item
- [ ] Bonus action handler: class ability gating
- [ ] Movement tracking in combat (melee/ranged positioning)
- [ ] Opportunity attack trigger system

### Phase A2 — Advanced Actions
- [ ] Ready action system (`ReadiedAction` struct, trigger resolution)
- [ ] Multi-attack resolution by class/level
- [ ] Reaction system (parry, uncanny dodge, counterspell, shield of faith)
- [ ] Difficult terrain flagging per zone/room

### Phase A3 — Death Saves
- [ ] `DeathSaveState` struct and DB schema
- [ ] Downed state: auto-roll death saves each turn
- [ ] Stabilization (3 successes, nat 20, potion)
- [ ] Death trigger (3 failures, nat 1 double-failure)
- [ ] Damage-while-downed failure marks

### Phase A4 — Hospital System
- [ ] `HospitalRecord` struct and DB schema
- [ ] Fee calculation (level + zone tier modifiers + cap)
- [ ] Recovery rest timer
- [ ] `!hospital`, `!hospital pay`, `!hospital status` commands
- [ ] Thom Krooke hospital admission narration
- [ ] Expedition abandonment on death (no resume)
- [ ] Debt system (insufficient coins)

---

*End of Action Economy, Movement & Death document.*
*Reference alongside all companion docs in Claude Code sessions.*
