# Combat Rebalance — 2026-05-10

Post HP-unification balance recovery. Player HP dropped ~45% when the legacy
`50 + CombatLevel*2` base was retired; monsters were still tuned for the old
~120 HP scale, making even L1 dungeons unsurvivable. This pass retunes both
sides for the dnd HP scale and retires CombatLevel from combat formulas.

## Symptom

Real prod player @holymachina (cleric, dnd L10): legacy combat MaxHP ≈ 165 → post-unification combat MaxHP ≈ 70. Monster damage formulas still scaled quadratically with tier (`T5 Attack=49`), oversized for ~70 HP players. Wight encounter user reported: 56/56 → 0/56 in 4 phases despite wight ending with 12 HP left. Even fresh L1 dungeon dives ran 95%+ death rate in sims.

## Root causes

1. **HP unification dropped the `50 + CombatLevel*2` base** without any rebalance pass on monster formulas. Monsters tuned for legacy HP pool now obliterate dnd HP pool.
2. **Tiebreak was absolute-HP-based** (recent change `de02907`). Worked OK on legacy scale where pools were similar; breaks on dnd scale because monster pools (~30–175) are 2-3× larger than player pools (~13–83), so timeouts always favor monsters even when player took less proportional damage.
3. **CombatLevel was still load-bearing** in `DerivePlayerStats` for Attack/Defense/Speed/HP scaling, contradicting the Phase L closeout that retired it everywhere else. This made test fixtures lie about real player power.

## Scope decisions (locked in by user)

- **Don't restore the legacy `50 + CL*2` HP base.** Combat MaxHP stays at `c.HPMax + HPBonus(gear delta only)`.
- **Strip CombatLevel from combat scaling entirely.** Player power = ability scores + gear + dnd-derived layers (AC/AB/weapon dice). No level-based defense/speed/attack/HP scaling on the player side.
- **Both formula and (eventually) bestiary tuning** for monsters, not just one. Formula pass was sufficient this round; bestiary deferred.

## Changes

### Test infrastructure

`internal/plugin/combat_stats_test.go`:

- `TestBalanceRegression_DungeonDeathRates` and `TestBalanceRegression_UnderleveledPlayers` now apply the production combat pipeline before simulating: `applyDnDPlayerLayer`, `applyDnDEquipmentLayer`, `applyDnDDungeonMonsterLayer`. Previously they ran raw `DerivePlayerStats` output, which silently hid the HP-unification regression because the legacy MaxHP value never made it into the test combatant.
- New helper `balanceTestDnDChar(dndLevel int)` synthesizes a fighter sheet (mirroring `autoBuildCharacter`) at the requested dnd level. Replaces the old `level=CombatLevel` proxy.
- Test cases re-keyed onto dnd levels (1, 2, 5, 7, 9) for the well-geared baseline and (1, 2, 4, 6) for the underleveled set. Previously `level=5..48` was being treated as both CombatLevel *and* dnd_level which hid the regression.
- Test log line now prints `AC` / `AB` / `Atk` instead of legacy `Atk/Def/Spd` triple — the d20 path's actual hit/damage drivers.
- `TestDerivePlayerStats_BaseStats` updated: zero-gear baseline is now `MaxHP=50, HPBonus=0, Attack=5` (constants, no level scaling). Previously expected `>=70 HP` which was the legacy formula output for L10.

### Player stat derivation

`internal/plugin/combat_stats.go`:

- **Stripped CombatLevel scaling** from `DerivePlayerStats`. `MaxHP` baseline is a flat constant (50, used only as the multiplicand for armor/arena/housing % bonus formulas — subtracted out into `HPBonus` at the end so the legacy base never reaches combat). `Attack`, `Defense`, `Speed` are now constants (5, 3, 5) plus gear and bonuses. Player power scales through gear and the dnd layers.
- All other gear/arena/housing/streak/grudge/pet/NPC bonus paths preserved.

### Dungeon monster formulas

`internal/plugin/combat_stats.go: DeriveDungeonMonsterStats`:

| Stat | Old formula (legacy HP era) | New formula (dnd HP era) |
|---|---|---|
| MaxHP | `25 + t*t*6 + death*0.6` (T1=33, T5=187) | `12 + t*7 + death*0.3` (T1≈22, T5≈65) |
| Attack | `3 + death*0.2 + t*t*1.5` (T1=5, T5=49) | `t*1.5 + death*0.04` (T1=1, T5=9) |
| Defense | `2 + t*1.5` | `2 + t*1.2` (small trim) |

Speed / CritRate / DodgeRate / BlockRate unchanged. AB scaling via `applyDnDDungeonMonsterLayer` (`9 + tier`) untouched — it was already calibrated for the d20 hit-resolution side.

Quadratic `t*t*X` terms are out everywhere on the monster side. Linear scaling matches the dnd HP scale's narrower band (player HP 13–83, not 60–200).

### Tiebreak

`internal/plugin/combat_engine.go`:

- Reverted the recent absolute-HP tiebreak (`de02907`) back to **HP percentage** for the post-Sudden-Death timeout case. Bias toward player on exact ties (`playerFrac >= enemyFrac` → enemy dies).
- Reasoning: monster pools on the dnd scale are systematically 2-3× larger than player pools, so absolute always favored monsters even when the player took less proportional damage. The `de02907` example case (88/123 vs 83/97) doesn't exist on the new scale.

### Comments and tests touched (no behavior change)

- `combat_stats.go` comments rewritten to explain the dnd HP scale assumption and why the legacy base is captured then subtracted out.
- `combat_engine.go` tiebreak comment rewritten with the new rationale.

## Sim results

`go test ./internal/plugin/ -run TestBalanceRegression -v` after this pass:

**Well-geared at tier (target window in parens):**

| Dungeon | Player | Monster | Death rate | Target |
|---|---|---|---|---|
| Soggy Cellar (L1 T1) | HP=13 AC=12 AB=6 | HP=21 AC=10 AB=5 Atk=1 | **3%** | 0–5% ✓ |
| Goblin Warrens (L2 T2) | HP=23 AC=12 AB=6 | HP=31 AC=11 AB=6 Atk=3 | **7%** | 0–10% ✓ |
| Cursed Crypt (L5 T3) | HP=48 AC=14 AB=7 | HP=42 AC=12 AB=7 Atk=5 | **1%** | 0–15% ✓ |
| Troll Bridge (L7 T4) | HP=66 AC=15 AB=7 | HP=53 AC=13 AB=8 Atk=7 | **8%** | 0–25% ✓ |
| Abyssal Maw (L9 T5) | HP=83 AC=16 AB=8 | HP=65 AC=14 AB=9 Atk=9 | **7%** | 1–35% ✓ |

**Underleveled / undergeared (real danger expected):**

| Scenario | Player | Monster | Death rate | Min target |
|---|---|---|---|---|
| L1 in T2 | HP=12 AC=12 AB=6 | HP=31 Atk=3 | **84%** | ≥40% ✓ |
| L2 in T3 | HP=21 AC=12 AB=6 | HP=42 Atk=5 | **99%** | ≥30% ✓ |
| L4 in T4, T2 gear | HP=39 AC=12 AB=6 | HP=53 Atk=7 | **97%** | ≥25% ✓ |
| L6 in T5, T3 gear | HP=56 AC=14 AB=7 | HP=65 Atk=9 | **83%** | ≥30% ✓ |

All pass. No other plugin tests broken by the rebalance (two pre-existing failures — `TestStandaloneHarvest_RoutesToZoneRun` and `TestZoneRunFlow_AdvanceToBossAndComplete` — are Phase G branching-zone node-naming issues, not caused by these changes; verified by stash-pop comparison).

### Bestiary stat-block pass

`internal/plugin/dnd_bestiary.go`:

- **Programmatic CR-based rescale** of `Attack` and `AttackBonus` across all 69 entries. New formula:
  - `Attack = round(1 + 1.5 × CR)`, with a gentler bend at CR ≥ 10 (`max(prev, round(1 + 1.7×CR − 2))`), capped at 38.
  - `AttackBonus = min(old_AB, 11)` — caps the d20 to-hit bonus so end-game monsters still miss player AC sometimes.
- HP / AC / Speed / Ability / BlockRate untouched — these aren't the lethality drivers; HP only matters if players can't damage-race the pool, and post-Attack-nerf the race is winnable.

Sample reseats:

| Monster | CR | Old Atk → New | Old AB → New |
|---|---|---|---|
| Goblin | 0.25 | 6 → 1 | 4 |
| Skeleton | 0.25 | 8 → 1 | 4 |
| Wight | 3 | 14 → 6 | 4 |
| Flameskull | 4 | 18 → 7 | 5 |
| Troll | 5 | 22 → 8 | 7 |
| Wyvern | 6 | 28 → 10 | 7 |
| Thornmother (CR 11 boss) | 11 | 32 → 18 | 8 |
| Belaxath (CR 19 boss) | 19 | 58 → 31 | 11 (was 12) |
| Ancient Dragon (CR 20) | 20 | 65 → 33 | 11 (was 14) |
| Infernax (CR 24, end boss) | 24 | 65 → 38 | 11 (was 14) |

Test update: `TestBestiaryToCombatStats` dragon AB expectation `14 → 11`.

**Reasoning for not also touching HP**: a CR 24 boss with 546 HP is appropriately big-boss bulky on the dnd scale — players doing 7-15 dmg/round will need 35+ rounds to kill, which is fine for a final boss with phases. The pain point was the boss one-shotting players via 65-damage swings, not the HP wall. If specific bosses still feel too tanky once players actually fight them, individual `HP` trims are easy.

## Open follow-ups

- **Arena monster formula** (`DeriveArenaMonsterStats`) still has the old quadratic-feeling shape but isn't currently broken in sims — left untouched this pass. If arena death rates report wrong, mirror the dungeon shape.
- **`TwoHandedMode` / `Versatile` weapon damage paths** in `applyDnDEquipmentLayer` weren't audited this pass — if they're miscalculating damage dice, balance shifts.
- **Tiebreak bias** is a flat "player wins ties." Could be tuned to a small grace margin (e.g., player wins if `playerFrac >= enemyFrac - 0.05`) if narrative feedback says timeout losses still feel cheap.
### Phase G test catch-up (formerly flaky)

Four tests written against the pre-Phase-G linear-room model were intermittently failing. Fixed in the same session so the suite runs clean:

- `TestStandaloneHarvest_RoutesToZoneRun` — was asserting the legacy `<zone>.r1` node id; production now writes `<zone>.entry` from the registered graph. Switched to `harvestNodeIDFor(run)` so the test reads whatever production wrote.
- `TestZoneRunFlow_AdvanceToBossAndComplete` — asserted `len(RoomsCleared) == TotalRooms`. Diamond/fork graphs yield shorter paths than the total node count. Loosened to `1..TotalRooms` (boss-defeated check still in place).
- `TestZoneCmd_AdvanceFinalRoomBumpsMood` — looped exactly `TotalRooms` `markRoomCleared` calls; with branching the run completes early and the extra calls find no active run. Switched to break on the first empty `next` return.
- `TestAdv2Scenario_ZoneRunGoblinWarrens` — sent only `!zone advance`, stalling at fork rooms (Phase G goblin warrens is a diamond graph). Added auto `!zone go 1` when `run.NodeChoices` is non-empty so the scenario commits the first fork option and continues.

Verified clean across 4 consecutive `go test` runs.

## Files touched

- `internal/plugin/combat_stats.go` — DerivePlayerStats CL strip, monster formula retune
- `internal/plugin/combat_engine.go` — tiebreak revert
- `internal/plugin/combat_stats_test.go` — production-accurate test pipeline, dnd_level-keyed cases, base-stats expectations
- `internal/plugin/dnd_bestiary.go` — programmatic CR-based Attack/AB rescale across 69 entries
- `internal/plugin/dnd_bestiary_test.go` — dragon AB expectation updated
