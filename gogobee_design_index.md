# GogoBee Design Document Index

> **Read this first.** Then read the docs relevant to your work, in dependency order.
>
> All docs in this index live at the repo root (`/home/reala-misaki/git/gogobee/*.md`).
> If you're starting an implementation session, **read every entry below before opening code** — many systems cross-reference each other and reading one doc in isolation has historically led to under-scoped work.

---

## The corpus

| Doc | Scope | Status |
|---|---|---|
| `gogobee_dnd_design_doc.md` | v1.0 vision: D&D character system, classes, races, ability scores, leveling, combat resolution, abilities, equipment slots, monsters, skill checks, rest, onboarding/migration, NPC integration | Vision; superseded for implementation by v1.1 |
| `gogobee_dnd_design_doc_v1.1.md` | v1.0 reconciled against actual codebase: real schema, archetype names, layered (not replacement) approach, smaller phase plan | Implementation source-of-truth |
| `gogobee_dnd_session_summary.md` | What's actually been built so far across phases 1-7 + audit fixes + flavor wiring | Living record |
| `gogobee_design_index.md` | This file | — |

## System docs (companions to v1.0/v1.1)

| Doc | Owns | Hard dependencies |
|---|---|---|
| `gogobee_action_economy_death.md` | Turn-based action economy (Action/Bonus/Reaction/Movement), death saves, hospital recovery | None — foundation |
| `gogobee_equipment_appendix.md` | Weapon damage dice, armor AC tables, magic item templates, ammo, ComputeAC helper | Action economy |
| `gogobee_spell_system.md` | 79 spells (cantrip–L5), spell slots, concentration, three casters (Mage/Cleric/Ranger), spell save DC | Action economy, equipment (component pouch / focus) |
| `gogobee_subclass_system.md` | 15 subclasses (3 per class × 5 classes), L5 selection prompt, L5/7/10/15 abilities | Action economy, spells (Arcane Trickster) |
| `gogobee_dungeon_zones.md` | 10 named zones, TwinBee GM persona w/ Mood Score, 5 room types, named bosses with multi-phase mechanics, tier gating | Combat (any), monsters, equipment (loot), spells (boss casters) |
| `gogobee_expedition_system.md` | Multi-day persistent expeditions, day cycle (06:00 brief / 21:00 recap), threat clock, supply system, camp tiers, extraction, siege mode | Zones (zones get wrapped), action economy (combat events), resources |
| `gogobee_resource_combat_integration.md` | Harvest nodes per zone, combat-as-event (room/interrupt/patrol/ambush), zone-specific loot tables | Zones, expedition, action economy, equipment |
| `gogobee_housing_babysitter.md` | Housing tiers 0-6 (rent + mortgage), 13 property upgrades, FRED ARM rate integration, Pastel babysitter NPC | Economy (existing euro_balances), pets (Pastel feeds them) |
| `gogobee_pet_system.md` | 30 named pets, tier 1-3 + Legendary, Ranger Pet Bond (5 tiers), happiness, pet leveling | Subclass (Ranger), housing (pet capacity per tier) |
| `gogobee_remaining_systems.md` | Skill Proficiency selection, Arena polish, Misty/Arina quest system, Pete Bot broadcasts, Economy by tier, Crafting recipes | Various; the catch-all |

## Reading order by intent

**Implementing combat changes:** `dnd_design_doc_v1.1` → `action_economy_death` → `equipment_appendix` → `spell_system` → `subclass_system`.

**Implementing world content (zones, dungeons):** `dnd_design_doc_v1.1` → `dungeon_zones` → `expedition_system` → `resource_combat_integration` → relevant flavor files in `internal/flavor/`.

**Implementing economy/housing/pets/NPCs:** `dnd_design_doc_v1.1` → `housing_babysitter` → `pet_system` → `remaining_systems` (for quests) → `internal/flavor/twinbee_npc_flavor.go` and `twinbee_housing_flavor.go`.

**Onboarding to the project cold:** all of v1.1 → this index → `dnd_session_summary.md` → then deep-dive into the system you're touching.

## Cross-reference graph (one-line summary)

```
                        dnd_design_doc / v1.1 (root)
                                |
          ┌─────────────────────┼──────────────────────┐
          ↓                     ↓                      ↓
  action_economy_death   dungeon_zones          housing_babysitter
          │                     │                      │
          ├──→ equipment        ├──→ expedition        └──→ pet_system
          ├──→ spell_system     │       │                       │
          ├──→ subclass         │       └──→ resource_combat    │
          │       │             │                ↑              │
          │       └─────────────┴────────────────┘              │
          │                                                     │
          └─────────────→ remaining_systems ←───────────────────┘
                          (quests, arena, Pete bot, crafting,
                           skill profs, economy by tier)
```

## Flavor file locations

All flavor lives in `internal/flavor/` (not at the repo root). These files have `DO NOT REWRITE` headers — never edit content; new pools go in new files only.

- `twinbee_gm_flavor.go` — TwinBee GM narration: room entries per zone, boss entries per named boss, combat outcomes, nat 20/1, level up, rest, traps, conditions, saves, taunts/compliments
- `twinbee_expedition_flavor.go` — Morning briefings (Day 1/3/7/14/21), evening recaps, camp, supply warnings, threat clock states, per-zone hazards (tides, time distortion, awareness pulses, portal destabilization), extraction, milestones
- `twinbee_resource_flavor.go` — Harvest pools per type (forage, mine, scavenge, essence, commune, fish), per-zone harvest variants, fishing per-zone, loot drops by rarity, patrol encounters
- `twinbee_npc_flavor.go` — Misty + Arina + Pete pools (greetings, skill checks, quests, trust/favorite tiers, Pete broadcasts including patch notes)
- `twinbee_housing_flavor.go` — Thom Krooke (rent, buy, mortgage, default, eviction, passive income), Pastel (hire, fire, level notes 1-5, missed tasks, deliveries, gifts, returns), home arrival, long rest at home, property upgrades (Workshop, Herb Garden, Vault, Trophy Room, Expedition Outpost)
- `pick.go` — `flavor.Pick(pool []string) string` helper

## Discipline

- **Adding a new system:** add a new design doc under `gogobee_<system>_name.md`. Update this index in the same change.
- **Updating a system:** prefer adding a `Section X.Y — addendum` rather than rewriting prose. The agents read these docs verbatim.
- **Removing a system:** mark the doc as deprecated in this index; don't delete unless the entire system is gone.
- **Cross-references in design docs:** every doc should declare its companions in its header (`> Companion to: ...`). The dnd_design v1.0 doc didn't, which is why a session in 2026 spent six hours building 20% of the spec before noticing the rest existed.
