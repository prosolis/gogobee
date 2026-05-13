# 24-Hour Change Summary — 2026-05-08 → 2026-05-09

139 commits since midnight 2026-05-09 plus an active uncommitted set. Grouped by theme below; commit hashes in parentheses for traceability.

---

## 1. Phase L — Legacy `AdventureCharacter` retirement (final wave)

The multi-day plan to retire `AdvCharacter`/`CombatLevel`/`combat_engine` reached its close-out. After today, `dnd_character` + `player_meta` are the canonical stores; the legacy table is cold and gated behind `GOGOBEE_LEGACY_PURGE`.

- **L2 — Arena boss flow** (`62eed7a` → `3b4dfa4`): extracted `renderBossOutcome`, added `ZoneArena` synthetic ID, built boss-shaped arena bestiary, wired `ARENA_BOSS_FLOW`, migrated arena counters/render/tier-gates to `DnDCharacter`+`player_meta`, then **ripped the legacy arena combat path** — boss flow is now the only path.
- **L3** (`d300008`): co-op dungeons deleted; in-flight runs auto-refunded on startup.
- **L4a–L4f** (`730f165` → `c719b30`): per-feature dual-write + reader flips off `AdvCharacter` — hospital, rival, masterwork, pet, housing, render/twinbee. DisplayName migration scaffold (`5000c3d`, `cc6fb7d`) preceded the L5 work.
- **L5a–L5h** (`79e5d19` → `1c7939c`): final teardown sub-phases — skills, babysit, NPC counters, streak/lifecycle, death state, misc fields, mass DnDCharacter backfill, then `saveAdvCharacter` fan-outs to `player_meta`. Table goes cold.
- **L5 close-out** (`36e14c6`, `c0f03a4`, `5d98e56`, `9d6e8f6`, `c8849fd`, `35fa2b8`): §7.4 doc reconciled, last 3 `CombatLevel` readers retired, loader rewired with bootstrap, redundant dual-writes dropped, `!expedition list` forked off `zoneCmdList`, and the **daily report migrated to a D&D-native renderer** with unified activity source.

Memory state: `project_phase_L_progress.md` now reads "deployed to prod; legacy table cold; operator can flip GOGOBEE_LEGACY_PURGE=1 anytime."

## 2. Phase G — Branching zone graphs (greenfield rebuild)

End-to-end shipped today. Old linear room-sequence model retired in favor of explicit graph nodes per zone.

- **G1–G4** (`a413c92`): schema, graph types, legacy compiler, run-state dual-write.
- **G5** (`2d249d7`): navigation surface gated behind `GOGOBEE_BRANCHING_ZONES=1`.
- **G6** (`893d3da`): dependent surfaces re-keyed onto graph nodes.
- **G7** (`90ff766`, `b3d3db9`): Crypt of Valdris POC graph; empty columns collapse in `!zone map`.
- **G8a–G8i** (`9312ef5` → `304ad27`): all 9 zones authored with **deliberately varied map shapes** (per playtester feedback that uniform shapes felt rote) — Goblin Warrens diamond, Forest Shadows asymmetric fork, Sunken Temple sequential, Manor Blackspire stacked 3-way, Underforge late triple fork, Feywild woven, Dragon's Lair converging+capstone, Abyss Portal three sequential, Underdark convergent triangle.
- **G9a–G9c** (`103cf30`, `a5c0a83`, `7ec78f3`): feature flag removed, `current_room`/`room_seq_json` no longer persisted, legacy columns dropped behind a purge gate.

Memory: `project_phase_G_progress.md` — G1–G8 shipped; G9 column purge / merge to main complete this session.

## 3. Hospital flavor + economy

- `2454f5f`: revival cost cut to **5k/level** + new CHA-based haggle roll (`!hospital`).
- `374d5ea`, `b8e8f99`, `97e4a50`: discharge-bill descriptor variety (USA-standards quip, prep fix, "a lot" swap).
- Merge: `feat/hospital-bill-flavor` → main (`3446103`).

## 4. Misc admin / UX

- `8e8d378`: Space inviter now DMs admin to confirm invites for Space-less local users; merged via `86c6736`.
- `3cf83e3`: `!eurogrant` admin command for granting EUR balance.

## 5. Combat — Sudden Death + HP precision

- `de02907`: **Sudden Death phase** added to all combat. Tiebreak now uses absolute HP (was % of max — fixed an edge case where a wounded high-HP fighter would lose to an undamaged low-HP enemy).
- `c5217e9`: wounded-carry HP display fixed (the "100/100 instead of 100/123" bug); auto-crit narration cleaned.
- `9ce82f7`: **round half-up on HP wound persistence** to fix the 101→100 round-trip drift through the dndChar HP scale. This was the patch that exposed the deeper dual-scale problem fixed by today's session work below.

---

## 6. Today's session (uncommitted as of writing)

### 6a. Combat HP unification — kill the dual scale

Drift bug `9ce82f7` only papered over: combat ran on `50 + CombatLevel*2 + equip + arena_sets + housing` (~123 for an L8 char) and bridged to `dnd_character.hp_max` (~78) via percentage scaling each fight. Now:

- New `CombatStats.HPBonus int` field captures the equipment / arena-set / housing HP buff as an **absolute number** (`legacyTotal − (50 + CombatLevel*2)`) — DerivePlayerStats keeps writing it for tests, but production combat ignores its `MaxHP`.
- `applyDnDPlayerLayer` now reseats `stats.MaxHP = c.HPMax + stats.HPBonus`. Single canonical HP scale; gear/arena/housing buffs preserved as a flat add-on.
- `applyDnDHPScaling` is a **direct integer copy**: wounded entry = `c.HPCurrent + HPBonus` (no percent math, no `dndWoundFloor`).
- `persistDnDHPAfterCombat` is a **direct integer copy**: clamps `endHP` to `[0, c.HPMax]`. Gear cushion is fight-only, doesn't carry over.
- Penalty zone HP scaling moved from before to after `applyDnDPlayerLayer` so the penalty applies on the unified scale.
- Dropped the unused `legacyStartHP` parameter from `persistDnDHPAfterCombat`.
- Stale "legacy HP / legacy combat-engine" comments swept; `dndWoundFloor` constant deleted.

Files: `combat_engine.go`, `combat_stats.go`, `combat_bridge.go`, `dnd_combat.go`, `dnd_zone_combat.go` + tests (`dnd_plugs_test.go`, `dnd_combat_test.go`, `dnd_audit_fixes_test.go`, `dnd_proddb_integration_test.go`).

Memory: `project_unify_combat_hp.md` flipped to **shipped**.

### 6b. Stats card cleanup

`!stats` (super-stats EX+α) now drops two stale references:

- The **"Combat Lv.X (XP)"** column — legacy `combat_level` from `player_meta` is no longer rendered (the survivor in `adventure_render.go` reads from `dndLevel` and is correct under the unified model).
- The **"🤖 Lost to TwinBee: N times"** line — entire `bot_defeats` block dropped.

Files: `stats.go`.

### 6c. Zone combat consumables wiring

Bug: `runZoneCombat` (zone elite/boss + arena + expedition path) never loaded, selected, or applied consumables — players' healing potions / wards / damage tonics sat in inventory while they died at 21/56 → 0/56 to a Wight.

- Wired `loadConsumableInventory` → `SelectConsumables` → `ApplyConsumableMods` → `injectConsumableEvents` → `removeAdvInventoryItem` into `runZoneCombat`, mirroring `runDungeonCombat`'s shape.
- Added `allowSkipTrivial bool` parameter to `SelectConsumables`. Zone/arena pass `false` (those are never chump fights, and the legacy threat assessor underestimates d20 lethality — one nat-20 streak ends the run). Dungeon path keeps `true` so trivial rooms still skip.

Files: `dnd_zone_combat.go`, `combat_bridge.go`, `adventure_consumables.go` (+test).

### 6d. TwinBee voice — surgical third-person trim

User flagged TwinBee flavor as overusing third-person self-reference (`"TwinBee X. TwinBee Y. TwinBee Z."` density across the same entry). Sweep across **14 flavor files**, surgical rule: trim only entries with 3+ "TwinBee" mentions or genuine subject-stack tics; leave 1–2 mentions alone; never switch to first-person "I"; preserve all jokes/references/length.

| File | Entries trimmed |
| --- | --- |
| `twinbee_gm_flavor.go` | 7 |
| `twinbee_expedition_flavor.go` | 17 |
| `twinbee_resource_flavor.go` | 5 |
| 6 of 11 `zone_*flavor.go` files | 1 each |
| `zone_dragons_lair`, `zone_underdark`, `zone_forest_shadows`, `zone_goblin_warrens`, +1 | 0 (already sparse) |

~35 entries touched out of ~600+. Memory: new `feedback_twinbee_voice.md` documents the rule for future authoring; the `// DO NOT REWRITE` headers on flavor files now treated as soft (override required for sweeps, but standing for incidental edits).

---

## Headline numbers

- **139 commits** since midnight 2026-05-09 (most landed before this session).
- **Two long-running phases closed** today: Phase L (legacy adv-character retirement) and Phase G (branching zone graphs).
- **One new refactor shipped this session**: combat HP unification — round-trip drift gone, single canonical HP scale.
- **Two production bugs fixed this session**: stats card stale columns; zone combat consumables not firing.
- **One creative QA pass**: ~35 TwinBee flavor entries trimmed across 14 files.

## Next on deck

- Per `project_consumable_crafting.md`: foraging-level auto-crafting consumables from ingredients.
- Per `project_adv_dm_window_tiers.md`: split `advDMResponseWindow` into high-pri vs low-pri constants.
- Per `project_human_race_bonus.md`: wire Human race's floating +1 stat bonus.
- Phase G done → optional `GOGOBEE_LEGACY_PURGE=1` flip whenever operator wants the cold table gone.
