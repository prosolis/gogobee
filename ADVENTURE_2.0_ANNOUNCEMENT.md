# Adventure 2.0 — D&D Layer

**Status:** shipping to prod from branch `adv-2.0`.
**Compatibility:** fully additive. Every existing character, treasure, balance,
streak, and inventory row is preserved. Players who don't opt in keep the
classic adventure experience unchanged.

---

## TL;DR

Adventure 2.0 grafts a full D&D-style character layer onto the existing
adventure system, then builds two new endgame loops on top of it: **single-
session dungeon zones** and **multi-day expeditions**. It introduces ten
hand-tuned zones across five tiers, a real-time threat clock, harvest nodes
per room, multi-region travel, an extract/resume cycle, and TwinBee — a
zone-narrating mood-driven dungeon master who reacts to what you do.

If you've been running `!adventure`, nothing breaks. If you run `!setup`, the
whole game opens up.

---

## What's new

### 1. A D&D character, on top of your existing one

Run `!setup` once. You pick a **race** and **class**, your existing
`combat_level` becomes your D&D level, and the system inherits your
treasures and equipment as attunements. From that point on every fight,
forage, and arena run is resolved through D&D mechanics — STR/DEX/CON/INT/WIS/CHA,
HP, AC, ability checks, saving throws, the lot.

- **5 classes:** Fighter, Mage, Cleric, Ranger, Rogue
- **6 races** with distinct passives
- **HP / AC / ability scores** with proper modifiers
- **Resource pools** per class (stamina, spell slots, channel divinity, etc.)
- `!sheet` shows everything: stats, bonuses, equipped gear, attunements

Players who haven't run `!setup` see the legacy adventure UI as before.

### 2. Subclasses & level-up features (L3 → L15)

At L3 you specialize — every class has 3 subclasses, each with its own
passives, abilities, and signature moves through L15.

| Class | Subclasses |
|---|---|
| Fighter | Champion, Battle Master, Berserker |
| Mage | Evocation, Abjuration, Necromancy, Arcane Trickster |
| Cleric | Life, War, Trickery |
| Ranger | Hunter, Beast Master, Gloom Stalker |
| Rogue | Thief, Assassin, Arcane Trickster |

Subclass features unlock at L5, L7, L10, L15 — extra attacks, spell slot
upgrades, signature spells, beast companions, sneak-attack scaling, divine
strike, etc. Use `!subclass` to inspect and pick.

### 3. Spells & spellbooks

Mages, Clerics, and Rangers cast for real now. Spell slots are tracked,
combat consumes them, and Mages have a **spellbook cap** that grows with
level — pick which spells to learn at level-up via `!learn`. Spells fire
automatically through the combat AI when the situation favors them.

### 4. Forward-simulating combat engine

Combat is no longer a one-shot dice roll. The engine plays out the encounter
turn by turn, threading consumables, monster abilities, status effects,
class resources, crits and fumbles, and renders a real combat narrative.
Every fight in arena, encounters, expeditions, and zones runs through it.

### 5. Dungeon zones (single-session)

`!zone` — pick a zone, run it, come out with loot.

- **10 zones, 5 tiers**, level bands L1 through L20
  - T1: Goblin Warrens, Crypt of Valdris
  - T2: Forest Shadows, Sunken Temple
  - T3: Haunted Manor, Underforge
  - T4: Underdark Outpost, Feywild Crossing
  - T5: Dragon's Lair, Abyss Portal
- **Procedural rooms:** entry → exploration → trap → elite → boss, with the
  middle padded out per zone
- **Per-zone bestiary, traps, atmosphere, and flavor pools**
- **Boss phase-two narration** when a boss crosses 50% HP
- **TwinBee** — your in-zone DM. Has a 0–100 mood score that shifts on
  crits, deaths, taunts, compliments, zone completions. Mood gates the
  flavor of every aside, gloat, and recap you read.
- `!zone map` — ASCII layout `E──?──T──★──☠` with your current marker
- `!zone lore` — TwinBee shares zone history
- `!zone taunt` / `!zone compliment` — directly poke or flatter your DM

### 6. Multi-day expeditions

`!expedition` — the long form. Pack supplies, head out, manage your day-by-day
survival, come home with the haul (or not).

- **Outfitting:** buy supply packs at start; standard or deluxe; cost scales
  with zone tier
- **Real-time day cycle:** 06:00 morning briefing, 21:00 evening recap
  (configurable per environment)
- **Supply burn:** rations tick down; harsh zones burn faster; running out
  triggers a forced extraction
- **Threat Clock** (0–100): every action you take is noticed. As threat
  rises you cross bands — Quiet → Stirring → Alert → Hostile → Siege —
  each band changes monster density, patrol odds, and night events
- **Patrol encounters** at Alert+: fights can interrupt you while traversing
  cleared rooms
- **Wandering monsters** at night
- **Siege Mode** at threat 80+: fortified camp branches, special warnings
- **Camp:** `!camp rough` (free, no protection) vs `!camp standard`
  (consumes supplies, grants overnight rest)
- **Milestones** (§13): First Night, Deep Dive, Long Haul, etc.
- **Expedition log:** every event recorded; `!expedition log` shows the trail

### 7. Multi-region zones

Higher-tier zones are **multi-region**. The Underdark, Feywild Crossing,
Dragon's Lair, and Abyss Portal each have multiple connected regions.

- `!region` — see where you are, where you can travel, what you've explored
- **Inter-region travel** consumes supplies and ticks threat
- **Base camp** is a persistent waypoint per expedition — anchor your
  return, retreat to safety, regroup
- `!map` (E6a) — full multi-region map renderer with visited markers

### 8. Per-zone temporal events

Every T2+ zone has a **signature time-based effect**:

- **Sunken Temple** — tidal cycle floods/recedes rooms
- **Haunted Manor** — nightly reset; cleared rooms come back
- **Underforge** — heat accumulation; gear damage rises
- **Feywild Crossing** — time distortion; days pass at non-uniform rates
- **Dragon's Lair** — awareness pulses warn the dragon directly
- **Abyss Portal** — destabilization stack threatens forced collapse

These are enumerated and active at all expedition tiers, layered on top
of the threat clock.

### 9. Extract & resume

`!extract` — bail safely from an expedition. You keep what you've found,
the expedition row goes to `extracting` status, and you can `!resume`
later to pick up from your base camp on the same expedition. Multi-region
extract→resume preserves your current region and visited set.

### 10. Harvest nodes per room

Foraging, mining, and (in coastal zones) fishing now happen **inside** zones.

- Every room gets a seeded set of **harvest nodes** — finite, room-scoped
- Harvest actions roll against zone-specific DCs with class bonuses
  (Ranger forage/fish, Fighter mine, etc.)
- **Class-specific yield bonuses** — Rangers find rare game; Fighters
  break tougher ore; Mages divine arcane components
- **Zone-condition effects** — Tidal Rising blocks fishing; Heat blocks
  mining; nightly reset re-seeds nodes; etc.
- **Per-zone resource tables** — every zone has its own loot pool

### 11. Fishing zones

Coastal/aquatic zones (Sunken Temple, Forest Shadows streams, Feywild
Crossing) support `!harvest fish` with their own resource pools, fishing
DCs, and **Ranger rare-catch upgrades**.

### 12. Zone loot drops

- **Boss kills** drop from a per-zone loot table
- **Elite kills** drop from a tier-scoped table
- Loot persists into your inventory and can be sold, equipped, or
  attuned via the existing flow

### 13. Economy integration

All Adv 2.0 systems use real coins. Outfitting expeditions debits at start;
extracted loot credits at completion; harvest yields stack into your
existing inventory; arena multipliers and streak XP scale through D&D
combat. No parallel currency.

### 14. TwinBee narration coverage

Every zone × every narration type (room entry, combat start, combat end,
crits, fumbles, player death, zone complete, mood asides, lore, boss
entry, elite entry, taunt/compliment responses) has a non-empty pool.
A coverage test (`TestNarrationCoverage_AllZonesAllTypes`) locks this in
so future zone additions can't ship with empty pools.

---

## Player commands cheat sheet

| Command | What it does |
|---|---|
| `!setup` | Opt into D&D — pick race + class, inherits your existing progress |
| `!sheet` | Full character sheet |
| `!learn` | Mages — pick a spell at level-up |
| `!subclass` | Inspect / select subclass at L3 |
| `!zone` / `!zone list` | Show available zones |
| `!zone enter <id>` | Start a single-session run |
| `!zone advance` | Resolve current room |
| `!zone status` / `!zone map` / `!zone lore` | Run inspection |
| `!zone abandon` | End run, no rewards |
| `!zone taunt` / `!zone compliment` | Poke TwinBee |
| `!expedition list` | Zones available at your level |
| `!expedition start <zone> [Ns] [Md]` | Outfit and begin |
| `!expedition status` / `!expedition log` | Daily briefing + history |
| `!camp rough` / `!camp standard` | Make camp |
| `!region` | Multi-region inspection + travel |
| `!map` | Full multi-region map |
| `!harvest forage` / `!harvest mine` / `!harvest fish` | Work the room |
| `!resources` | Inventory of harvested materials |
| `!threat` | Current threat clock |
| `!extract` | Bail safely from an expedition |
| `!resume` | Pick up an extracted expedition |

---

## Compatibility & rollout

- **Additive migrations only.** No columns dropped, no tables renamed. Every
  pre-2.0 column is preserved with original semantics.
- **Opt-in.** A `pending_setup` flag keeps non-opted players on the legacy
  flow. Output formats only switch after `!setup` completes.
- **Treasures → attunements.** Existing treasure rows are interpreted as
  attunements rather than re-issued.
- **`combat_level` → D&D level.** Same column, new lens.
- **Equipment slots** carry forward 1:1.

If you've been playing pre-2.0, your save is intact. Run `!setup` when you
want the new layer. Don't run it and nothing about your day changes.

---

## Developer notes

- **Branch:** `adv-2.0`, 104 commits ahead of `main`
- **Test coverage:** full `internal/plugin` suite green, including a new
  scenario playthrough (`adv2_scenario_test.go`) that drives end-to-end
  flows against a copy of the prod database
- **Phases shipped on this branch:**
  - Phases 1–9 — character system, abilities, combat engine, spells (SP1–SP4)
  - Phase 10 — subclasses (SUB1–SUB3d, all classes through L15)
  - Phase 11 — dungeon zones D1–D11 (10 zones + TwinBee + map + coverage test)
  - Phase 12 — expeditions E1–E6 (supplies, threat clock, day cycle, camp,
    multi-region, extract/resume, milestones, flavor)
  - Phase R — resource/combat integration (R1–R7, including post-completion
    audit hardening)
- **Deferred (pre-existing systems required):**
  - Lemmy/Matrix activity → mood signal hooks (waiting on activity aggregation)
  - Pete bot zone announcements (Phase 16 deliverable)
