# GogoBee — Expedition System
> **Companion to:** `gogobee_dnd_design_doc.md`, `gogobee_dungeon_zones.md`
> **Version:** 1.0
> **Status:** Draft

---

## ⚠️ Protected Files — DO NOT MODIFY

All existing `*_flavor.go` / `*_flavor.txt` files are off-limits.
New expedition flavor text goes in `twinbee_expedition_flavor.go` — a new file, never edits to existing ones.

---

## 1. Design Philosophy

Single-session dungeon runs are replaced by **Expeditions** — persistent, multi-day (sometimes multi-week) explorations tied to real calendar time. An Expedition is not a sprint. It is a campaign.

### What changes

- Zones are no longer completed in one sitting. They unfold over real days.
- The player logs in each day to take actions, advance, rest, manage supplies.
- TwinBee delivers a **Morning Briefing** and an **Evening Recap** every real-world day the expedition is active.
- Resources deplete over time. Poor planning ends expeditions early.
- The dungeon has a **Threat Clock** — the longer you stay, the more aware the zone becomes.
- Some events only happen on specific days, specific times, or after specific triggers.

### What doesn't change

- All combat mechanics from the main design doc apply unchanged.
- Loot, XP, and progression work identically.
- Players can still retreat, rest, and use all existing commands.
- Zones are still the same zones with the same bosses.

---

## 2. Expedition Duration by Zone Tier

| Tier | Zone | Minimum Days | Maximum Days | Notes |
|---|---|---|---|---|
| 1 | Goblin Warrens | 2 | 4 | Shortest possible expedition |
| 1 | Crypt of Valdris | 2 | 5 | Boss requires full rest before attempt |
| 2 | Forest of Shadows | 4 | 8 | Zone scales spatially |
| 2 | Sunken Temple | 4 | 9 | Tidal event on Day 6 (see Section 7) |
| 3 | Haunted Manor | 6 | 12 | Manor fully resets every 3 in-game nights |
| 3 | The Underforge | 5 | 10 | Heat accumulation mechanic |
| 4 | The Underdark | 10 | 21 | Multi-region; truly exhausting |
| 4 | Feywild Crossing | 8 | 14 | Time distortion (see Section 7) |
| 5 | Dragon's Lair | 14 | 30 | Multi-wing structure |
| 5 | The Abyss Portal | 21 | ∞ | Portal destabilizes over time; hard exit pressure |

**Minimum days** = the absolute floor if a player advances aggressively, rests optimally, and encounters no complications.  
**Maximum days** = soft cap. After this, the Threat Clock maxes out and the expedition enters **Siege Mode** (see Section 8).

---

## 3. The Expedition Day Cycle

Each real-world day an expedition is active follows this structure:

```
06:00 — MORNING BRIEFING (TwinBee)
         Status summary: HP, supplies, current location,
         Threat Clock level, any overnight events.

06:00–21:00 — ACTIVE WINDOW
         Player takes actions: advance, search, fight,
         trade, camp, rest, interact with NPCs.
         No hard limit on actions per day — limited by
         resources, HP, and supply consumption.

21:00 — EVENING RECAP (TwinBee)
         Summary of the day: rooms cleared, loot found,
         XP gained, supplies consumed.
         TwinBee delivers a narrative coda.
         Threat Clock advances if player is in a
         dangerous position.

21:00–06:00 — NIGHT PHASE
         Camp triggers if the player has camped.
         Wandering monster check (see Section 6).
         Environmental events possible (Section 7).
         Supply consumption continues passively.
```

---

## 4. Supply System

Expeditions consume **Supplies** — an abstracted resource representing food, water, torches, rope, ammunition, and minor consumables. Supplies are purchased before departure and cannot be restocked mid-expedition unless a Supply Cache is found or a resupply route is established.

### 4.1 Supply Units

One **Supply Unit (SU)** represents one day's worth of basic provisions for one character.

| Zone Tier | Base SU Cost Per Day | Harsh Conditions Multiplier |
|---|---|---|
| 1 | 1 SU/day | ×1 |
| 2 | 1.5 SU/day | ×1.5 |
| 3 | 2 SU/day | ×2 |
| 4 | 3 SU/day | ×2.5 |
| 5 | 4 SU/day | ×3 |

**Harsh conditions** are triggered by: Threat Clock above 60, heat accumulation events, tidal flooding, Feywild time distortion.

### 4.2 Supply Procurement

| Source | SU Yield | Cost | Notes |
|---|---|---|---|
| Thom Krooke standard pack | 10 SU | 50 coins | Max 3 packs per expedition |
| Thom Krooke deluxe pack | 20 SU | 90 coins | Max 1 per expedition |
| Foraged (Ranger, WIS DC 12) | 1d4 SU | Free | Once per day; fails in indoor zones |
| Supply Cache (dungeon loot) | 2d6 SU | — | Random find; not guaranteed |
| NPC Trader (certain zones) | Variable | Variable | Zone-specific |

### 4.3 Supply Depletion Effects

| SU Remaining | Effect |
|---|---|
| > 25% | Normal operation |
| 10–25% | Rationing: -1 to all rolls from fatigue |
| 1–9% | Severe rationing: -2 to all rolls; no long rests |
| 0 | Starvation: -1d4 CON per day; forced extraction if CON reaches 0 |

### 4.4 Go Data Model — Supplies

```go
type ExpeditionSupplies struct {
    Current     float32 `json:"current"`      // current SU
    Max         float32 `json:"max"`          // purchased at outset
    DailyBurn   float32 `json:"daily_burn"`   // base rate for this zone tier
    HarshMod    float32 `json:"harsh_mod"`    // multiplier if harsh conditions active
    Foraging    bool    `json:"foraging"`     // Ranger forage attempt used today
}
```

---

## 5. Camp System

Camping is the primary mechanism for long rests during an expedition. It is not free — it costs time, supplies, and exposes the player to overnight risk.

### 5.1 Camp Types

| Camp Type | Setup Requirement | Rest Quality | Supply Cost | Night Risk |
|---|---|---|---|---|
| **Rough Camp** | Any location | Partial (50% HP, no slots) | +0.5 SU | High |
| **Standard Camp** | Safe room (cleared) | Full long rest | +1 SU | Medium |
| **Fortified Camp** | Boss-cleared room or cache site | Full long rest + bonus | +2 SU | Low |
| **Base Camp** | Expedition Day 3+ in Tier 4–5 zones | Full long rest + extended | +3 SU | Very Low |

**Fortified Camp bonus:** +1d6 HP on wake; TwinBee narrates a peaceful night (rare flavor event).  
**Base Camp:** Establishes a persistent waypoint. Player can fast-travel back to Base Camp in 1 action. Required for Tier 5 zones.

### 5.2 Camp Placement Rules

- Cannot camp in a room with active enemies.
- Cannot camp in a trap room (even if the trap is disarmed).
- Cannot camp in a Boss room (the room knows it's a boss room).
- Camping in a non-cleared room = Rough Camp regardless of intent.

### 5.3 Go Data Model — Camp

```go
type CampState struct {
    Active       bool      `json:"active"`
    Type         string    `json:"camp_type"`   // rough, standard, fortified, base
    RoomID       string    `json:"room_id"`
    EstablishedAt time.Time `json:"established_at"`
    NightEvents  []string  `json:"night_events"` // log of overnight occurrences
}
```

---

## 6. Wandering Monster System

During the **Night Phase**, if the player has camped, a wandering monster check fires. The result depends on camp type and Threat Clock level.

### 6.1 Wandering Monster Table

Roll 1d20 + Threat Clock modifier:

| Roll | Outcome |
|---|---|
| 1–5 | Peaceful night. TwinBee narrates quiet. |
| 6–10 | Signs of passage. No encounter; Threat Clock +2. |
| 11–14 | Minor encounter: 1–2 low-CR enemies from zone roster. Player wakes to combat. |
| 15–17 | Standard encounter: standard zone enemy group. |
| 18–19 | Elite encounter: Elite-tier enemy from zone. |
| 20+ | Ambush: enemy has surprise round. Elite-tier. TwinBee delivers a terse, sleep-interrupted narration. |

**Threat Clock modifier:** +1 per 10 points of Threat Clock above 30.

### 6.2 Rough Camp Night Risk Modifier

Rough Camp adds +3 to all wandering monster rolls.

### 6.3 Avoiding Night Encounters

- **Ranger passive:** -2 to all wandering monster rolls while in a wilderness zone.
- **Rogue passive:** On a roll of 11–14, may attempt a Stealth check (DC 14) to avoid the encounter entirely.
- **Fortified Camp:** -4 to all wandering monster rolls.

---

## 7. Zone-Specific Temporal Events

These events fire on specific expedition days, times, or conditions. They are not random — they are scheduled by the zone and announced by TwinBee in advance (vaguely).

### 7.1 Sunken Temple — Tidal Event (Day 6)

On Expedition Day 6, the tidal cycle peaks. All flooded rooms gain:
- +1d6 cold damage per turn while wading
- Kuo-toa enemies gain +2 AC and +1d4 to attack rolls
- Supply burn doubles for 24 hours (water infiltration)

TwinBee warns on Day 4: *"The water is rising. Something about the rhythm of it has changed."*  
TwinBee warns on Day 5: *"Tomorrow the temple will be fully claimed by the tide. Plan accordingly."*

If the player has reached the Boss room before Day 6: no event fires.

### 7.2 Haunted Manor — Nightly Reset (Every 3 In-Game Nights)

Every third night in the Manor, non-boss rooms respawn one enemy (not a full reset — one enemy per room, chosen from zone roster). This represents the house reasserting itself.

TwinBee notes on Night 2: *"The house has been quiet. That will not last."*  
On Night 3 morning: *"Overnight, something moved back in. Several somethings, actually."*

### 7.3 The Underforge — Heat Accumulation

Each day spent in the Underforge adds 1 Heat Stack (max 10). Effects:

| Heat Stacks | Effect |
|---|---|
| 1–3 | Flavor only ("the air shimmers") |
| 4–6 | +1d4 fire damage at start of each combat round |
| 7–9 | Disadvantage on CON saves; Supply burn +50% |
| 10 | Exhaustion: -2 to all rolls; forced 24hr rest or extraction |

Heat stacks reduce by 2 per long rest in a Fortified Camp. Full extraction resets all stacks.

### 7.4 The Feywild Crossing — Time Distortion

Time in the Feywild is unreliable. Each day the player enters an Exploration Room, roll 1d6:

| Roll | Time Effect |
|---|---|
| 1–2 | Normal day passes |
| 3–4 | Half-day: only 0.5 SU consumed; next room generates instantly |
| 5 | Double-day: 2 SU consumed; an extra wandering monster check fires |
| 6 | Time Loop: player re-enters the same room (new enemies; loot already taken) |

TwinBee tracks time distortion and reports it in the Morning Briefing with increasing bewilderment.

### 7.5 Dragon's Lair — Awareness Pulses

Infernax sleeps lightly. Every 3 days, an Awareness Pulse fires:
- Kobold patrols increase by 1 enemy per active room
- Threat Clock +10
- TwinBee delivers an Awareness Pulse narration (see `twinbee_expedition_flavor.go`)

On Day 14: Infernax wakes. If the player has not reached the Boss room by Day 14, Infernax begins active hunting — wandering monster checks occur every 6 hours and may produce a Dragon encounter (CR 15 young dragon proxy) until the player reaches the final chamber.

### 7.6 The Abyss Portal — Destabilization

The portal grows unstable over time. Each day, Portal Instability increases by 5 (starts at 0, max 100).

| Instability | Effect |
|---|---|
| 0–20 | Normal operations |
| 21–40 | Psychic pressure: -1 to WIS rolls |
| 41–60 | Reality warps: rooms occasionally shift order |
| 61–80 | Demon surges: wandering monster check every 12 hours |
| 81–99 | Unraveling: -2 to all rolls; Supply burn doubles |
| 100 | Collapse: player is forcibly extracted; expedition fails; portal requires a new event to reopen |

Instability is reduced by 10 for each major story objective completed in the zone.

---

## 8. The Threat Clock

The Threat Clock (0–100) represents how aware the zone's faction has become of the player's presence. It is the dungeon's version of stealth — the longer you're here, the more organized the opposition becomes.

### 8.1 Threat Clock Modifiers

| Event | Threat Change |
|---|---|
| Combat in a new room | +5 |
| Enemy killed before raising alarm | +0 |
| Enemy escapes combat | +10 |
| Player uses loud ability (Fireball, Thunderwave) | +8 |
| Rogue uses Stealth to move between rooms | -3 |
| Long rest in Fortified Camp | -5 |
| Boss defeated | -20 |
| Day passes in zone | +3 |
| TwinBee Mood: Wrathful | +5/day additional |
| TwinBee Mood: Elated | -3/day |

### 8.2 Threat Clock Thresholds

| Level | Threshold | Effect |
|---|---|---|
| Quiet | 0–20 | No modifications. TwinBee describes an unaware zone. |
| Stirring | 21–40 | Enemies in uncleared rooms gain +1 to Perception; wandering monster +1 |
| Alert | 41–60 | Enemies gain +2 AC; wandering monster +2; reinforcements possible |
| Hostile | 61–80 | All enemies have initiative advantage; room traps re-arm |
| Siege | 81–100 | **Siege Mode** (see Section 8.3) |

### 8.3 Siege Mode

When Threat Clock hits 100, the zone enters Siege Mode:
- All rooms with cleared enemies respawn one enemy (once)
- Boss gains +20 HP and Legendary Resistance +1
- Supply burn doubles
- No short rests possible (too dangerous)
- TwinBee delivers a Siege Mode narration and does not sugarcoat it
- Siege Mode cannot be reduced — it is the dungeon's final form

TwinBee begins warning at Threat Clock 70:  
*"They know you're here. Not a suspicion anymore. A certainty. The question now is whether you finish before they organize."*

### 8.4 Go Data Model — Threat Clock

```go
type ThreatClock struct {
    Level       int       `json:"level"`        // 0–100
    SiegeMode   bool      `json:"siege_mode"`
    LastUpdated time.Time `json:"last_updated"`
    Events      []ThreatEvent `json:"events"`
}

type ThreatEvent struct {
    Timestamp time.Time `json:"timestamp"`
    Delta     int       `json:"delta"`
    Reason    string    `json:"reason"`
}
```

---

## 9. Expedition State Model

```go
type Expedition struct {
    ID              string            `json:"id"`
    PlayerID        string            `json:"player_id"`
    ZoneID          string            `json:"zone_id"`
    StartDate       time.Time         `json:"start_date"`
    CurrentDay      int               `json:"current_day"`
    Status          string            `json:"status"`    // active, extracting, complete, failed
    CurrentRegion   string            `json:"current_region"`
    CurrentRoomID   string            `json:"current_room_id"`
    RoomsCleared    []string          `json:"rooms_cleared"`
    BossDefeated    bool              `json:"boss_defeated"`
    Supplies        ExpeditionSupplies `json:"supplies"`
    Camp            *CampState        `json:"camp"`
    ThreatClock     ThreatClock       `json:"threat_clock"`
    TemporalStack   int               `json:"temporal_stack"`  // zone-specific (heat, instability, etc.)
    Log             []ExpeditionEntry `json:"log"`
    LootCollected   []string          `json:"loot_collected"`
    XPEarned        int               `json:"xp_earned"`
    GMMood          int               `json:"gm_mood"`
    LastActivity    time.Time         `json:"last_activity"`
}

type ExpeditionEntry struct {
    Day       int       `json:"day"`
    Timestamp time.Time `json:"timestamp"`
    Type      string    `json:"type"`    // action, combat, rest, event, narrative
    Summary   string    `json:"summary"`
    Flavor    string    `json:"flavor"`  // TwinBee line for this entry
}
```

---

## 10. Extraction

Extraction is leaving the zone before completion — intentionally or by force.

### 10.1 Voluntary Extraction

Command: `!extract`

- Player may extract at any time from any cleared room.
- Extraction costs 1 in-game day (finding the way out).
- Player keeps all loot and XP earned to that point.
- Expedition marked as Incomplete — can be resumed (see Section 10.3).
- TwinBee delivers an extraction narration.

### 10.2 Forced Extraction

Triggered by: HP reaching 0, supplies reaching 0, Abyss Portal collapse.

- Player loses 20% of coins earned during expedition (chaos tax).
- All loot carried out is kept.
- Expedition marked as Abandoned.
- Cannot be resumed — must restart.
- Threat Clock resets to 20 on re-entry (zone remembers you, but time has passed).
- TwinBee delivers a forced extraction narration (respectful; never mocking).

### 10.3 Expedition Resume

Voluntarily extracted expeditions can be resumed within **7 real days**:

- Resumes from the last cleared room.
- Supplies must be repurchased (what was carried is gone — you extracted).
- Threat Clock resumes at extraction value.
- Zone-specific temporal stacks (Heat, Instability) resume at extraction value.
- TwinBee delivers a return narration: *"Back again. TwinBee noted the door. TwinBee notes it opening."*

After 7 days, the expedition expires and must restart.

---

## 11. Multi-Region Zones (Tier 4–5)

The Underdark, Dragon's Lair, and Abyss Portal are too large for a single dungeon map. They are divided into **Regions**, each functioning as a sub-dungeon.

### 11.1 Region Structure

Each Region contains:
- Its own room sequence (Entry → Exploration → Trap → Exploration → Elite → Boss)
- A **Region Boss** (not the Zone Boss — a mid-tier milestone)
- A potential **Base Camp** site (unlocked after clearing Region Boss)
- Region-specific enemies (subset of zone roster)
- Region-specific loot table

### 11.2 Region Progression

| Zone | Regions | Region Boss |
|---|---|---|
| The Underdark | Surface Tunnels → Drow Outpost → Illithid Warren → The Deep Throne | Drow Elite Captain → Mind Flayer Elder → Ilvaras Xunyl (Zone Boss) |
| Dragon's Lair | Kobold Warrens → Drake Pens → The Vault → Infernax's Chamber | Kobold Warchief → Elder Drake → Infernax (Zone Boss) |
| The Abyss Portal | Outer Rift → Demon Assembly → The Warden's Post → The Tear | Vrock Commander → Nalfeshnee → Belaxath (Zone Boss) |

### 11.3 Region Transitions

Moving between regions costs 1 full in-game day of travel. TwinBee narrates the transition with specific regional flavor. Supply burn applies during travel. Wandering monster check fires once during transit.

### 11.4 Go Data Model — Region

```go
type ExpeditionRegion struct {
    ID            string    `json:"id"`
    ZoneID        string    `json:"zone_id"`
    Name          string    `json:"name"`
    Order         int       `json:"order"`
    Cleared       bool      `json:"cleared"`
    BossDefeated  bool      `json:"boss_defeated"`
    BaseCampSite  bool      `json:"base_camp_site"`
    EnemySubset   []string  `json:"enemy_subset"`
    LootTable     []LootEntry `json:"loot_table"`
    FlavorRef     string    `json:"flavor_ref"`
}
```

---

## 12. TwinBee Daily Briefing & Recap Format

### 12.1 Morning Briefing (06:00)

```
🎭 TwinBee — Morning Briefing, Day [N]

📍 Location:    [Zone Name] — [Region if applicable] — [Room description]
❤️  HP:          [current] / [max]
🎒 Supplies:    [SU remaining] / [SU max] ([days remaining estimate])
⏰ Threat:      [Threat Clock level] — [Threshold name]
🌡️  [Zone stack]: [Heat/Instability/Time value if applicable]

[Overnight events, if any — TwinBee narrates what happened while you slept]

[TwinBee morning flavor line — see twinbee_expedition_flavor.go]

Available actions: [!advance] [!search] [!camp] [!rest] [!extract] [!status]
```

### 12.2 Evening Recap (21:00)

```
🎭 TwinBee — Evening Recap, Day [N]

Today's progress:
  Rooms cleared:  [N]
  Enemies defeated: [N]
  Loot found:     [item list or "nothing notable"]
  XP earned:      [N]
  Supplies used:  [N SU]

[TwinBee evening narrative — see twinbee_expedition_flavor.go]

[Camp prompt if player hasn't camped: "You should rest. The dungeon does not."]
```

---

## 13. Expedition Milestones & Bonus Rewards

Milestones reward players who engage with the expedition's full arc rather than rushing.

| Milestone | Trigger | Reward |
|---|---|---|
| First Night | Survive Night 1 | +50 XP; TwinBee delivers a "you're still here" line |
| Cartographer | Search every room before advancing | Bonus loot roll in next Elite room |
| Patient Zero | Complete expedition without Threat Clock above 50 | +10% XP bonus on zone complete |
| Survivalist | Complete Tier 3+ expedition with no forced extraction | Unique title + cosmetic item |
| Week One | Expedition Day 7 survived | +200 XP; Thom Krooke discount next visit |
| Two Weeks | Expedition Day 14 survived | +500 XP; permanent +1 to primary class stat |
| The Long Game | Complete a Tier 5 zone | Legendary item guaranteed; TwinBee delivers a personal note |

---

## 14. Expedition Commands Reference

| Command | Description |
|---|---|
| `!expedition start <zone>` | Begin a new expedition (opens supply purchase flow) |
| `!expedition status` | Full expedition status summary |
| `!expedition log` | Last 5 expedition log entries with TwinBee narration |
| `!advance` | Move to next room (costs time; triggers encounters) |
| `!retreat` | Return to previous cleared room |
| `!search` | Search current room (Perception/Investigation check) |
| `!camp <type>` | Establish camp (rough / standard / fortified / base) |
| `!rest short` | Short rest at current location |
| `!rest long` | Long rest (requires Standard camp or better) |
| `!supplies` | Check supply status and daily burn rate |
| `!threat` | Check Threat Clock level and current threshold |
| `!extract` | Begin voluntary extraction |
| `!resume` | Resume a previously extracted expedition |
| `!map` | Display expedition progress as ASCII region/room map |

---

## 15. Implementation Phases — Expedition System

### Phase E1 — Core Infrastructure
- [ ] `Expedition` struct and DB schema
- [ ] Real-time day cycle engine (06:00/21:00 triggers)
- [ ] Morning Briefing and Evening Recap generation
- [ ] `!expedition start`, `!expedition status` commands
- [ ] Supply system (procurement, depletion, effects)
- [ ] Basic camp system (rough and standard)

### Phase E2 — Threat Clock & Night Phase
- [ ] Threat Clock state machine
- [ ] Wandering monster system
- [ ] Night phase event resolution
- [ ] Siege Mode implementation
- [ ] Fortified Camp and Base Camp

### Phase E3 — Zone Temporal Events
- [ ] Tidal Event (Sunken Temple)
- [ ] Nightly Reset (Haunted Manor)
- [ ] Heat Accumulation (Underforge)
- [ ] Time Distortion (Feywild)
- [ ] Awareness Pulses (Dragon's Lair)
- [ ] Portal Destabilization (Abyss Portal)

### Phase E4 — Multi-Region Zones
- [ ] Region system and region boss structure
- [ ] Region transition (travel day, supply burn, encounter)
- [ ] Base Camp as persistent waypoint
- [ ] Underdark, Dragon's Lair, Abyss Portal region maps

### Phase E5 — Extraction & Resume
- [ ] Voluntary extraction flow
- [ ] Forced extraction flow
- [ ] Expedition resume (7-day window)
- [ ] Expedition log persistence

### Phase E6 — Milestones & Polish
- [ ] Milestone tracking and reward delivery
- [ ] `twinbee_expedition_flavor.go` full population (briefings, recaps, event narrations)
- [ ] ASCII map renderer (multi-region aware)
- [ ] Pete bot expedition update posts (daily highlights for Matrix room)

---

*End of Expedition System Document.*
*Reference alongside all companion docs in Claude Code sessions.*
