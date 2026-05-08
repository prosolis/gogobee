# GogoBee — Housing & Babysitter Systems
> **Companion to:** `gogobee_dnd_design_doc.md`, `gogobee_expedition_system.md`
> **Version:** 1.0
> **Status:** Housing — complete spec. Babysitter — design options (decision pending).

---

## ⚠️ Protected Files — DO NOT MODIFY

All existing `*_flavor.go` / `*_flavor.txt` files are off-limits.
Housing and babysitter flavor goes in `twinbee_housing_flavor.go` — new file only.

---

## Part One: Housing System

---

## 1. Overview

Housing gives players a persistent home base — a physical address in the GogoBee world that persists between expeditions, provides mechanical benefits, and represents long-term investment. It is the anchor of the non-expedition economy.

Housing integrates with:
- **Long rests** (home = free long rest, always available)
- **Storage** (inventory overflow, rare item safekeeping)
- **Crafting** (certain recipes require a home workshop)
- **Passive income** (upgraded properties generate coins over time)
- **Expedition prep** (supply storage, base camp fast-travel anchor)
- **Pet housing** (pets need somewhere to live when you're away)
- **Babysitter system** (see Part Two)
- **Thom Krooke** (mortgage holder, property broker)

---

## 2. Property Tiers

Players begin homeless and progress through tiers by purchasing or renting. Each tier unlocks new capabilities.

| Tier | Property Type | Acquisition | Base Cost | Long Rest | Storage | Passive Income |
|---|---|---|---|---|---|---|
| 0 | Homeless | Default | — | Inn only (costs coins) | None | None |
| 1 | Rented Room | `!rent room` | 15 coins/week | ✅ | 10 slots | None |
| 2 | Rented Apartment | `!rent apartment` | 40 coins/week | ✅ | 25 slots | None |
| 3 | Purchased Cottage | `!buy cottage` | 800 coins (mortgage available) | ✅ | 50 slots | 5 coins/day |
| 4 | Purchased House | `!buy house` | 2,500 coins (mortgage available) | ✅ | 100 slots | 15 coins/day |
| 5 | Manor | `!buy manor` | 8,000 coins (mortgage available) | ✅ | 250 slots | 40 coins/day |
| 6 | Estate | `!buy estate` | 25,000 coins (mortgage available) | ✅ | Unlimited | 100 coins/day |

### 2.1 Rent vs. Buy

**Renting:**
- Weekly payment auto-deducted (Thom Krooke collects)
- No equity, no passive income, no upgrade path
- Miss two consecutive payments → eviction (Tier 0, possessions move to storage for 7 days)
- No mortgage complexity; simple and accessible early game

**Buying:**
- One-time purchase OR mortgage financing through Thom Krooke (see Section 4)
- Owned property generates passive income
- Upgradeable with rooms, amenities, workshop
- Cannot be repossessed unless mortgage defaults (3 consecutive missed payments)

---

## 3. Property Upgrades

Owned properties (Tier 3+) can be upgraded with rooms and amenities. Upgrades are permanent and increase property value.

### 3.1 Upgrade Catalog

| Upgrade | Cost | Prerequisite | Benefit |
|---|---|---|---|
| **Storage Expansion** | 200 coins | Any owned | +50 storage slots |
| **Workshop** | 500 coins | Cottage+ | Unlocks home crafting (no Thom Krooke visit required) |
| **Herb Garden** | 300 coins | Cottage+ | Passive: 1d4 common materials/day (zone-agnostic) |
| **Forge** | 750 coins | House+ | Unlocks metal crafting at home; +1 to crafted weapon/armor quality |
| **Library** | 600 coins | House+ | Passive: +1 INT for all INT skill checks while at home |
| **Training Grounds** | 800 coins | House+ | Short rest restores +1d6 additional HP |
| **Guest Room** | 400 coins | House+ | Future: NPC visitors; Babysitter eligible housing (see Part Two) |
| **Vault** | 1,000 coins | Manor+ | Secure storage: items here cannot be lost on death |
| **Trophy Room** | 500 coins | Manor+ | Display legendary items; passive +5% XP gain |
| **Expedition Outpost** | 1,200 coins | Manor+ | Base Camp fast-travel cost reduced; supply storage for active expeditions |
| **Greenhouse** | 900 coins | Estate | Passive: 1d6 uncommon materials/day (zone-agnostic) |
| **Scriptorium** | 1,500 coins | Estate | Passive: crafting recipe discovery without `!lore` checks |

### 3.2 Property Value

Each upgrade adds to the property's **assessed value**, which affects:
- Passive income calculation (higher value = more income)
- Mortgage refinancing options
- Future resale (sell at 70% of current assessed value)

---

## 4. Mortgage System

The mortgage system is GogoBee's primary long-term financial mechanic, pegged to real-world ARM rates via the FRED API.

### 4.1 Mortgage Structure

When a player takes a mortgage via `!buy <property> --mortgage`:

1. **Down payment:** 20% of property cost paid upfront
2. **Principal:** Remaining 80% financed by Thom Krooke
3. **Interest rate:** Current FRED ARM rate + 2% (Thom Krooke's margin)
4. **Term:** 30 in-game weeks (real-time weeks)
5. **Payment frequency:** Weekly, auto-deducted Sunday at reset

**Example at current ARM rate (~6.5% + 2% = 8.5%):**
```
Cottage: 800 coins
Down payment: 160 coins
Principal: 640 coins
Weekly payment: ~25 coins/week for 30 weeks
Total paid: ~750 coins (principal + interest)
```

### 4.2 ARM Rate Integration

The FRED API is queried weekly (Sunday reset) to fetch the current 5/1 ARM rate.  
Rate updates are announced by Thom Krooke via Matrix message to the TwinBee room.

```go
// FREDRateConfig holds the FRED API configuration for ARM rate fetching.
type FREDRateConfig struct {
    SeriesID    string  `json:"series_id"`    // "MORTGAGE5US"
    MarginPct   float32 `json:"margin_pct"`   // Thom Krooke's margin (2.0)
    LastFetched time.Time `json:"last_fetched"`
    CurrentRate float32 `json:"current_rate"` // base rate from FRED
    EffectiveRate float32 `json:"effective_rate"` // base + margin
}
```

### 4.3 Mortgage Events

| Event | Trigger | Consequence |
|---|---|---|
| Rate increase >0.5% | FRED weekly update | Thom Krooke announces; payment adjusts next week |
| Rate decrease >0.5% | FRED weekly update | Thom Krooke announces; payment adjusts next week |
| Missed payment | Insufficient coins on Sunday | Warning from Thom Krooke; 10% penalty added |
| Two missed payments | Second consecutive miss | Thom Krooke visits in person (flavor event); 20% penalty cumulative |
| Three missed payments | Third consecutive miss | Default: property seized; equity returned at 50% |
| Early payoff | `!mortgage payoff` | No penalty; Thom Krooke is genuinely pleased |
| Refinance | `!mortgage refi` | Reset term at current rate; available once per 10 weeks |

### 4.4 Thom Krooke Rate Announcements

Thom Krooke posts to the TwinBee Matrix room weekly with the rate update. These are in Thom's voice (Tom Nook parody) and contain the actual FRED rate:

> *"Good news, friends! The ARM rate this week sits at 7.2% — and with my modest service margin, your mortgage rate is 9.2%. As always, I am here to help. Payments process Sunday. Thank you!"*

> *"The rate has moved up a bit this week — 8.1%, so 10.1% with my fee. Nothing to worry about! Think of it as an investment in your future. Thom Krooke is always here. Always."*

### 4.5 Go Data Model — Mortgage

```go
type Mortgage struct {
    ID              string    `json:"id"`
    PlayerID        string    `json:"player_id"`
    PropertyID      string    `json:"property_id"`
    Principal       int       `json:"principal"`
    Remaining       int       `json:"remaining"`
    DownPayment     int       `json:"down_payment"`
    WeeklyPayment   int       `json:"weekly_payment"`
    InterestRate    float32   `json:"interest_rate"`
    TermWeeks       int       `json:"term_weeks"`
    WeeksRemaining  int       `json:"weeks_remaining"`
    MissedPayments  int       `json:"missed_payments"`
    TotalPaid       int       `json:"total_paid"`
    StartedAt       time.Time `json:"started_at"`
    PaidOffAt       *time.Time `json:"paid_off_at"`
    Defaulted       bool      `json:"defaulted"`
}
```

---

## 5. Property State & Commands

### 5.1 Player Property Record

```go
type PlayerProperty struct {
    PlayerID        string    `json:"player_id"`
    PropertyID      string    `json:"property_id"`
    Tier            int       `json:"tier"`
    Type            string    `json:"type"`        // rented, owned
    Name            string    `json:"name"`        // player-named
    Upgrades        []string  `json:"upgrades"`    // upgrade IDs
    AssessedValue   int       `json:"assessed_value"`
    PassiveIncome   int       `json:"passive_income"` // coins/day
    StorageCapacity int       `json:"storage_capacity"`
    StorageUsed     int       `json:"storage_used"`
    MortgageID      *string   `json:"mortgage_id"`
    AcquiredAt      time.Time `json:"acquired_at"`
    LastRentPaid    *time.Time `json:"last_rent_paid"`
}
```

### 5.2 Housing Commands

| Command | Description |
|---|---|
| `!home` | Summary of current property, upgrades, and status |
| `!home name <name>` | Name your property |
| `!rent room` / `!rent apartment` | Rent at current tier |
| `!buy <cottage/house/manor/estate>` | Purchase outright |
| `!buy <property> --mortgage` | Purchase with Thom Krooke financing |
| `!upgrade <type>` | Purchase a property upgrade |
| `!mortgage status` | Current mortgage balance, rate, payments remaining |
| `!mortgage payoff` | Pay off remaining mortgage balance |
| `!mortgage refi` | Refinance at current FRED rate |
| `!storage` | View storage inventory |
| `!storage deposit <item>` | Store item at home |
| `!storage withdraw <item>` | Retrieve item from home storage |
| `!sell property` | Sell at 70% assessed value (must own outright) |
| `!passive income` | Check daily passive income and last payout |

---

## 6. Housing × Expedition Integration

### 6.1 Long Rest at Home

Owning or renting any property provides free long rests via `!rest long` with no inn cost and no cooldown — home is always available.

The **Expedition Outpost** upgrade extends this: the outpost links to your active Base Camp in a Tier 4–5 zone, allowing one fast-travel per expedition back to base camp from home (and return) without consuming a full travel day.

### 6.2 Supply Storage

Players with owned properties (Tier 3+) can pre-stage expedition supplies at home:

- `!storage deposit supplies <N>` — store SU at home
- Supplies stored at home are available for `!expedition start` without visiting Thom Krooke
- Maximum staged supply = 2× the property's base storage tier
- Herb Garden and Greenhouse upgrades contribute passive SU daily

### 6.3 Passive Income Collection

Passive income accrues daily at reset. Collected automatically into the player's coin balance. Thom Krooke sends a brief weekly summary:

> *"Good morning! Your property generated 105 coins this week. Minus your mortgage payment of 25 coins — a net gain of 80. Wonderful, yes?"*

---

---

## Part Two: Babysitter System — Design Options

---

## 7. What Is Known

The Babysitter system exists in the GogoBee design. Its purpose is not yet fully defined. The following is known:

- It is a named system — "Babysitter" is intentional, not a placeholder
- It relates to what happens while the player is **away** (on expedition, offline, or otherwise occupied)
- It likely connects to housing (something to watch over)
- It possibly connects to pets (something to care for)
- It is separate enough from the housing system to warrant its own name

The following are **design options** — mutually exclusive directions. One should be chosen before implementation begins.

---

## 8. Design Option A — The Offline Caretaker

**Concept:** The Babysitter is an NPC hired to manage the player's property and pets while they are on expedition or offline. A literal babysitter for your home.

**Mechanics:**
- Hired via `!hire babysitter` at a daily coin cost
- While active, the Babysitter:
  - Feeds and cares for pets (prevents pet happiness decay, if that mechanic exists)
  - Collects passive income from property upgrades (without the Babysitter, income queues but doesn't process)
  - Waters the Herb Garden / Greenhouse (without care, passive material yield halves after 3 days)
  - Accepts deliveries from Thom Krooke (supply orders placed before expedition are ready on return)
- Babysitter has a **competence level** (1–5) that affects reliability:
  - Level 1: Occasionally forgets things (20% chance of missed task per day)
  - Level 3: Reliable (5% miss chance)
  - Level 5: Perfect (0% miss chance; also occasionally leaves a small gift)
- Babysitter levels up with continued employment (XP from days worked)
- A level 5 Babysitter is a relationship worth maintaining

**Cost:**
| Babysitter Level | Daily Cost |
|---|---|
| 1 | 3 coins/day |
| 2 | 6 coins/day |
| 3 | 10 coins/day |
| 4 | 15 coins/day |
| 5 | 22 coins/day |

**Flavor:** The Babysitter is a named NPC (name TBD) with a personality. They leave notes. TwinBee reads the notes aloud on expedition return. The notes are the flavor text.

---

## 9. Design Option B — The Passive Expedition Support

**Concept:** The Babysitter is an active support role during expeditions — a companion back at Base Camp or home who provides passive benefits to the expedition in progress.

**Mechanics:**
- Activated via `!babysitter deploy` at expedition start (costs coins per day of expedition)
- While deployed, the Babysitter:
  - Reduces supply burn by 10% (they're handling logistics from the rear)
  - Once per expedition, can deliver a supply drop to the player's Base Camp (costs coins + 1 expedition day)
  - Manages the Threat Clock passively (-1 per day via "covering your tracks" at home base)
  - Sends morning intel: +1 to first Perception/Investigation check of each expedition day
- Babysitter cannot enter combat zones — support only
- Deactivates if player is forced to extract (they come to get you)

**Cost:** 8 coins/day of expedition, prepaid at start.

**Flavor:** The Babysitter communicates via Matrix messages — brief field reports in the morning, encouraging notes in the evening, occasional observations that are either useful or cryptic depending on TwinBee's mood.

---

## 10. Design Option C — The Passive Skill Trainer

**Concept:** The Babysitter is a trainer the player hires to develop skills while they are doing other things — offline training.

**Mechanics:**
- Hired to train a specific skill or stat for a real-time duration
- While training:
  - The targeted skill/stat gains slow XP passively (cannot exceed 50% of the next threshold via training alone)
  - Training costs coins per hour of real time
  - Player does not need to be online
- Training is interrupted if the player uses the relevant skill in an expedition (expedition XP takes over)
- Maximum one trainer active at a time
- Training has a cap: cannot use training to level up — only to approach the threshold faster

**Cost:** 2–10 coins/hour depending on stat trained (CHA most expensive; STR cheapest).

**Flavor:** The trainer has opinions. If you ignore their training by going on expedition, they note it. If you complete a session they trained you for and perform well, they take appropriate credit.

---

## 11. Design Option D — The Community Role

**Concept:** The Babysitter is a community-facing role — a player (or designated bot persona) who watches over the TwinBee Matrix room while a player is on expedition, acting as a liaison.

**Mechanics:**
- One Babysitter role active per expedition (could be another player or an NPC fill-in)
- Babysitter player can:
  - Check on the expedition player's status via `!babysit status <player>`
  - Send one item or supply cache to an active expedition (costs the Babysitter coins)
  - Vote to activate a community Boost (community milestone modifier to TwinBee mood)
- In return, the Babysitter earns a commission from the expedition's loot (5% of coin value on completion)
- If no player accepts the Babysitter role, an NPC fills it at reduced effectiveness

**Flavor:** This makes expeditions a community event rather than solo. The Matrix room has a stake in the expedition's outcome. TwinBee acknowledges the Babysitter in morning briefings.

---

## 12. Design Decision: Option A — The Offline Caretaker

**Selected.** The Babysitter is an NPC hired to manage the player's property and pets while they are on expedition or offline.

---

## 13. Babysitter NPC — Pastel

**Name:** Pastel
**Source:** Recurring support character from the TwinBee series (Konami). In the games, Pastel is the capable, warm-hearted civilian presence while the TwinBee ships are out fighting. In GogoBee, she's the person keeping everything together while you're in the dungeon.

**Personality:** Warm, slightly tired, completely reliable. Pastel has seen players return from bad expeditions and good ones and treats both the same — with a note on the table and everything in order. She is not impressed by legendary loot. She is genuinely pleased when the pets are happy. She does not complain. She notices everything.

**Voice:** Domestic, understated, occasionally dry. Contrasts directly with Thom Krooke's relentless salesmanship and TwinBee's dramatic narration. Pastel's register is: *this is what I did today, everything is fine, the cat knocked something over.*

**Level progression:**
- **Level 1:** Enthusiastic but scattered. Means well. 20% chance of missed task per day.
- **Level 2:** Getting the hang of it. 12% miss chance. Notes are more organized.
- **Level 3:** Found her rhythm. 5% miss chance. Reliable, efficient, minimal fuss.
- **Level 4:** Anticipatory. 1% miss chance. Occasionally handles things before they become problems.
- **Level 5:** Perfect. 0% miss chance. Sometimes leaves a small gift. The right gift.

```go
type BabysitterNPC struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`       // "Pastel"
    Level       int       `json:"level"`      // 1–5
    FlavorRef   string    `json:"flavor_ref"` // "pastel"
    HiredByID   string    `json:"hired_by_id"`
    HiredAt     time.Time `json:"hired_at"`
    DailyRate   int       `json:"daily_rate"`
    DaysWorked  int       `json:"days_worked"`
    XP          int       `json:"xp"`         // levels up with days worked
    MissChance  float32   `json:"miss_chance"` // 0.20→0.12→0.05→0.01→0.00
}
```

---

## 14. Implementation Phases — Housing

### Phase H1 — Core Housing
- [ ] `PlayerProperty` struct and DB schema
- [ ] Tier 0–2 (homeless, rented room, rented apartment)
- [ ] Rent payment auto-deduction (weekly)
- [ ] Eviction system
- [ ] `!home`, `!rent` commands
- [ ] Free long rest at rented/owned property

### Phase H2 — Ownership & Mortgage
- [ ] Tier 3–6 (cottage through estate)
- [ ] `Mortgage` struct and DB schema
- [ ] FRED API ARM rate integration (weekly fetch)
- [ ] `!buy`, `!mortgage status`, `!mortgage payoff`, `!mortgage refi` commands
- [ ] Thom Krooke rate announcement posts
- [ ] Missed payment and default handling

### Phase H3 — Upgrades & Passive Systems
- [ ] Upgrade catalog implementation
- [ ] Passive income daily accrual
- [ ] Storage system (`!storage` commands)
- [ ] Herb Garden / Greenhouse passive material yield
- [ ] Workshop and Forge crafting unlock

### Phase H4 — Expedition Integration
- [ ] Supply staging at home
- [ ] Expedition Outpost fast-travel link
- [ ] Vault (death-proof storage)
- [ ] Trophy Room XP bonus

### Phase H5 — Babysitter (post-design decision)
- [ ] Implement chosen option
- [ ] Babysitter NPC creation (name, personality, flavor text)
- [ ] `twinbee_housing_flavor.go` population

---

*End of Housing & Babysitter Document.*
*Babysitter direction requires human decision before Phase H5 implementation.*
*Reference alongside all companion docs in Claude Code sessions.*
