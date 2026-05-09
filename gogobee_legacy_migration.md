# GogoBee — Legacy Adventure → D&D Engine Migration Plan
> **Companion to:** `gogobee_dnd_design_doc.md`, `gogobee_expedition_system.md`, `gogobee_resource_combat_integration.md`
> **Version:** 0.1 (planning draft)
> **Status:** Pre-implementation — scoping
> **Branch:** `adv-2.0`

---

## 0. Why this doc exists

Phase R (resource/combat integration) shipped the new harvest/combat surface and gated the player-facing legacy daily-loop dispatch. The internal helpers (`resolveAdvAction`, `advEligibleLocations`, `AdvLocation` tier data, `AdventureCharacter`, `CombatLevel`, `combat_engine.go`) are still alive because non-deprecated subsystems consume them: arena, co-op dungeons, babysit/scheduler, hospital, rival, masterwork, pets, housing, mortgage, twinbee morning DM.

This doc inventories every surviving caller and lays out a phased deletion path. The endgame is: `AdventureCharacter` deleted, `CombatLevel` field gone, `combat_engine.go` removed, `combat_bridge.go` removed, branching dungeon paths become buildable on a clean schema.

**Non-goals.** This is not a redesign. We are not adding features during the migration. New mechanics (branching paths, multi-region expansions, new arena formats) are sequenced *after* L5 teardown lands.

---

## 1. Inventory — what's still on the legacy engine

| Subsystem | Files | LOC | AdvCharacter coupling | CombatLevel coupling | D&D parallel | Risk |
|---|---|---|---|---|---|---|
| Babysit/scheduler | `adventure_babysit.go`, `adventure_scheduler.go` | 1,169 | mutates: CombatXP, Alive, ActionTakenToday | reads | none (auto-actions) | low |
| Arena | `adventure_arena*.go` (4 files) | 1,228 | mutates: Alive, ArenaWins/Losses, CombatXP | gates tier, scales rewards | `dnd_zone_combat.go` | high |
| Co-op dungeons | `coop_dungeon*.go` (9 files) | 4,027 | reads-only (minLevel gate, liability) | gates participation | `dnd_expedition_combat.go` | medium |
| Hospital | `adventure_hospital.go` | 288 | mutates: Alive, HospitalVisits | cost = level × 25k/125k | none | low |
| Rival | `adventure_rival.go` | 866 | mutates: RivalPool, CombatXP | unlock at L5, stake calc | none | medium |
| Masterwork | `adventure_masterwork.go` | 543 | reads equip, no char mutation | none | none | low |
| Pets | `adventure_pets.go` | 448 | mutates pet fields on AdvCharacter | none | none | low |
| Housing/mortgage | `adventure_housing.go`, `adventure_mortgage.go` | 922 | mutates House* fields | none | none | low |
| TwinBee/render | `adventure_render.go`, `adventure_twinbee.go` | 1,472 | reads stats; simulator calls `resolveAdvAction` | reads | none | low |
| Combat bridge | `combat_bridge.go` | 363 | translation layer | — | — | (delete) |

**Source of truth:** `AdventureCharacter` is defined at `adventure_character.go:27` (76 fields). `DnDCharacter` at `dnd.go:152`.

**Live `resolveAdvAction` callers (post-R1):** `adventure_babysit.go:349`, `adventure_twinbee.go:135` (simulator).

---

## 2. Architectural decision — char unification strategy

Three options were considered:

**A. Big-bang field merge.** Move every surviving AdvCharacter field onto DnDCharacter, then s/AdventureCharacter/DnDCharacter/g. Rejected: unsafe, no incremental landing, breaks every test for one PR.

**B. Two-character coexistence forever.** Keep both, route by feature. Rejected: this is what we have now; doesn't actually retire anything.

**C. Phased subsystem migration with shared "extra" record.** ✅ Chosen. Each subsystem is migrated to read/write D&D state where there's a parallel, and to a new `player_meta` table for fields that don't fit on a character (HouseTier, RivalPool, ArenaWins, BabysitActive, MistyLastSeen, etc.). When all subsystems are migrated, AdvCharacter is deleted in a single L5 commit.

The `player_meta` table is the holding pen for non-stat per-user state that's currently squatting on AdvCharacter. It's not a redesign — it's a structural move so the deletion can happen cleanly.

### 2.1 `player_meta` schema (proposed)

> **Naming note.** New tables/types in this migration deliberately avoid the `dnd_` prefix used by older code (legal/trademark exposure flagged by user). Existing `dnd_*` symbols stay in place — only new surface uses neutral names.

```sql
CREATE TABLE player_meta (
    user_id            TEXT PRIMARY KEY,
    -- arena
    arena_wins         INTEGER NOT NULL DEFAULT 0,
    arena_losses       INTEGER NOT NULL DEFAULT 0,
    invasion_score     INTEGER NOT NULL DEFAULT 0,
    -- rival
    rival_pool         INTEGER NOT NULL DEFAULT 0,
    rival_unlocked_notified INTEGER NOT NULL DEFAULT 0,
    -- housing
    house_tier         INTEGER NOT NULL DEFAULT 0,
    house_loan_balance INTEGER NOT NULL DEFAULT 0,
    house_loan_frozen  INTEGER NOT NULL DEFAULT 0,
    house_missed_payments INTEGER NOT NULL DEFAULT 0,
    house_autopay      INTEGER NOT NULL DEFAULT 0,
    house_current_rate REAL NOT NULL DEFAULT 0,
    -- pets
    pet_type           TEXT NOT NULL DEFAULT '',
    pet_name           TEXT NOT NULL DEFAULT '',
    pet_xp             INTEGER NOT NULL DEFAULT 0,
    pet_level          INTEGER NOT NULL DEFAULT 0,
    pet_armor_tier     INTEGER NOT NULL DEFAULT 0,
    pet_flags_json     TEXT NOT NULL DEFAULT '{}',  -- chased_away/reactivated/arrived/morning_defense/etc
    -- npcs
    misty_last_seen    INTEGER,
    arina_last_seen    INTEGER,
    misty_buff_expires INTEGER,
    misty_debuff_expires INTEGER,
    arina_buff_expires INTEGER,
    misty_encounter_count INTEGER NOT NULL DEFAULT 0,
    misty_donated_count INTEGER NOT NULL DEFAULT 0,
    misty_roll_target  INTEGER NOT NULL DEFAULT 0,
    arina_roll_target  INTEGER NOT NULL DEFAULT 0,
    -- babysit
    babysit_active     INTEGER NOT NULL DEFAULT 0,
    babysit_expires_at INTEGER,
    babysit_skill_focus TEXT NOT NULL DEFAULT '',
    auto_babysit       INTEGER NOT NULL DEFAULT 0,
    auto_babysit_focus TEXT NOT NULL DEFAULT '',
    -- hospital / death
    hospital_visits    INTEGER NOT NULL DEFAULT 0,
    last_death_date    TEXT NOT NULL DEFAULT '',
    death_source       TEXT NOT NULL DEFAULT '',
    death_location     TEXT NOT NULL DEFAULT '',
    last_pardon_used   INTEGER,
    death_reprieve_last INTEGER,
    -- streaks / chores
    current_streak     INTEGER NOT NULL DEFAULT 0,
    best_streak        INTEGER NOT NULL DEFAULT 0,
    last_action_date   TEXT NOT NULL DEFAULT '',
    grudge_location    TEXT NOT NULL DEFAULT '',
    holiday_action_taken INTEGER NOT NULL DEFAULT 0,
    streak_decayed     INTEGER NOT NULL DEFAULT 0,
    -- shop / drops
    masterwork_drops_received INTEGER NOT NULL DEFAULT 0,
    treasures_locked   INTEGER NOT NULL DEFAULT 0,
    pet_supply_shop_unlocked INTEGER NOT NULL DEFAULT 0,
    pet_level_10_date  TEXT NOT NULL DEFAULT '',
    thom_animal_line_fired INTEGER NOT NULL DEFAULT 0,
    -- chat / npc msg
    npc_msg_count      INTEGER NOT NULL DEFAULT 0,
    npc_msg_count_date TEXT NOT NULL DEFAULT '',
    robbie_visit_count INTEGER NOT NULL DEFAULT 0,
    crafts_succeeded   INTEGER NOT NULL DEFAULT 0
);
```

### 2.2 Stat mapping — AdvCharacter → DnDCharacter

**Decisions locked in by user 2026-05-08:**
- **CombatLevel→Level:** direct copy. CombatLevel doesn't carry semantic meaning beyond "starting Level for the new system." No compression formula. Players keep their numeric value as their D&D Level.
- **CombatXP→XP:** **discarded.** Players start fresh on the new XP track (CombatXP is not migrated). The Level set above is their floor; XP resets to 0 at that level's threshold.
- **AdvEquipment:** kept as-is. Stays at `AdvEquipment` type and `adventure_equipment` table. No rename to `dnd_*` (per naming guidance — see §2.0 callout above).

| AdvCharacter | DnDCharacter destination | Notes |
|---|---|---|
| `CombatLevel` | `Level` | Direct copy on backfill; no formula. |
| `CombatXP` | — | **Not migrated.** XP starts at the floor of the new Level. |
| `MiningSkill` / `ForagingSkill` / `FishingSkill` | retained as new fields on DnDCharacter | These are *skills*, not class levels. Add `MiningSkill/ForagingSkill/FishingSkill int` to DnDCharacter; keep XP fields too. |
| `Alive` / `DeadUntil` | DnDCharacter `HPCurrent==0` + `DeadUntil` | Need to add `DeadUntil *time.Time` to DnDCharacter. |
| `Title` | new field on DnDCharacter | Direct copy. |
| equipment (`AdvEquipment`) | stays at `AdvEquipment` / `adventure_equipment` | Type and table name unchanged. Read paths drop AdvCharacter unwrap; no rename pass. |

### 2.3 `CombatLevel` retuning

Legacy CombatLevel scaled 1–50ish through grindy XP. D&D Level scales 1–20. Hospital/rival/co-op gates that read CombatLevel will be re-tuned to D&D Level brackets:

| Legacy gate | New gate |
|---|---|
| Hospital cost = `CombatLevel × 25k` (or × 125k post-cap) | `Level × 50k` (or × 250k at L20+) |
| Rival unlock at CombatLevel ≥ 5 | Rival unlock at Level ≥ 3 |
| Arena tier brackets (every 10 CL) | Arena tier brackets (every 4 levels) |
| Co-op dungeon minLevel | Re-tune per dungeon (table in L3 section) |

These thresholds are placeholders — final tuning happens during each phase based on playtest.

---

## 3. Phase L1 — Babysit & scheduler

**Goal:** Kill the last live `resolveAdvAction` caller. Auto-actions become expedition-aware.

**Files touched:** `adventure_babysit.go`, `adventure_scheduler.go`, `adventure_twinbee.go` (simulator), `adventure_activities.go` (delete `resolveAdvAction` + `advEligibleLocations` after callers gone).

**Design:**
- Babysit currently picks one of three skills (mining/foraging/fishing) and runs `resolveAdvAction` once a day for the user. The new flow:
  - If user has an active expedition → babysit attempts a single harvest action (`!forage`/`!mine`/`!fish` per skill focus) inside the user's current room. Goes through `handleHarvestCmd` headlessly with `babysit:true` flag so combat interrupts force-extract instead of running combat. (User isn't online; can't fight.)
  - If user has no active expedition → babysit posts a TwinBee nudge (one per real day, rate-limited to once/3d) suggesting `!expedition start`. No XP awarded.
- `auto_babysit` becomes "auto-harvest while expedition active." `babysit_skill_focus` carries through unchanged.
- `adventure_twinbee.go:135` simulator (uses fake CombatLevel=35 character) — replace with a fixed flavor-only narration. The simulator was always fake; deleting the AdvCharacter dependency is mechanical.

**Steps:**
1. Add `auto_babysit` / `babysit_*` columns to `player_meta` (per §2.1) — migration + read/write helpers.
2. Add `babysit:true` flag to harvest dispatch path; on combat-interrupt branches Standard/Elite/Patrol, force-extract the region run instead of entering combat. Test.
3. Rewrite `babysitDo` to call new harvest path. Remove `resolveAdvAction` call. Test (covers: active expedition, no expedition, harvest yields → inventory, combat interrupt force-extract).
4. Rewrite scheduler entrypoint analogously.
5. Replace twinbee simulator with static flavor.
6. Delete `resolveAdvAction`, `advEligibleLocations`, `AdvActionResult`, `parseActivityLocation` from `adventure_activities.go`. Audit for last callers via grep.
7. `go test ./... && go vet`.

**Tests added:** `adventure_babysit_test.go` (currently absent — create). Cases: babysit-with-expedition harvests, babysit-without-expedition nudges, combat-interrupt during babysit force-extracts cleanly, daily rate-limit holds.

**Exit criteria:**
- `grep resolveAdvAction internal/plugin/` returns 0 hits.
- `go test ./...` clean.
- Production users with `auto_babysit=true` and an active expedition see harvest deposits, not legacy activity output.

**Risk:** Babysit users have a behavior change (now auto-harvests instead of always rolling). Announce in `ADVENTURE_2.0_ANNOUNCEMENT.md`.

---

## 4. Phase L2 — Arena (rebuild on Adventure 2.0 boss flow)

**Decision (2026-05-08):** Arena migrates to the **Adventure 2.0 boss flow** — the same staged narrative path zone bosses run through (`resolveBossRoom` in `dnd_zone_cmd.go:630`). Each arena fight is treated as a self-contained boss encounter: full combat log via `RenderCombatLog`, TwinBee mood lines on Nat20/Nat1, phase-two narration when the enemy crosses its `PhaseTwoAt` HP threshold, win/loss outcome blocks. Arena stops being "numbers vs numbers" and becomes a narrated event with the same texture as a zone boss room.

This is also the right shape because zone-boss plumbing (bestiary, mood events, phase transitions, kill records, loot drops) is already built and tested. We re-use it rather than re-implementing.

**Files touched:**
- `adventure_arena.go` — entrypoint, gating, payout. Heavy rewrite.
- `adventure_arena_combat.go` — round driver. Replaced by a call into `runZoneCombat` + a new arena-specific staged renderer (or direct re-use of the zone boss render — see step 4).
- `adventure_arena_render.go` — reads switch to DnDCharacter + player_meta.
- `adventure_arena_monsters.go` — **deleted as standalone**. Arena monsters are folded into the existing bestiary as boss-shaped entries (with `PhaseTwoAt`).
- `adventure_arena_test.go` — ported.
- New (small): `adventure_arena_flavor.go` — only if zone TwinBee pools don't cover arena context. Check existing `adventure_flavor_*.go` first per the reuse-flavor feedback rule before creating this file.

**Design specifics:**

- **Arena monster definitions.** Each existing `ArenaMonster` becomes a `BestiaryEntry` shape: `{ID, Name, HP, AC, AB, DamageDie, CR, PhaseTwoAt, Tags}`. Arena IDs are namespaced `arena_<tier>_<slug>` so they don't collide with zone bestiary IDs. Stored in a new `arenaBosses` map (still in `adventure_arena_monsters.go` if the file is kept; otherwise inlined). Tier brackets controlled by Arena tier (see below).
- **Combat invocation.** Arena round driver builds `BossEncounter{Monster, PhaseTwoAt, ZoneIDForFlavor}` and calls a small new function `resolveArenaBoss(userID, encounter)` that wraps the same `runZoneCombat` → `RenderCombatLog` → mood/phase-two lines flow as `resolveBossRoom`. **Important:** arena is not inside a zone, so we factor the staged-narration body out of `resolveBossRoom` into a shared helper `renderBossOutcome(...)` that both call sites use. (`resolveBossRoom` keeps its zone-specific glue: zone loot drop, `applyBossDefeatThreat`, `abandonZoneRun`. Arena keeps its glue: Arena win/loss counters, payout, no zone loot.)
- **TwinBee flavor in arena.** `twinBeeLine(zoneID, ...)` is keyed by zone; Arena passes a synthetic `ZoneArena` ID. Add `ZoneArena` to the zone enum and populate a small TwinBee mood/Nat20/Nat1/BossDeath/PlayerDeath pool — reuse existing pools where possible (per `feedback_reuse_existing_flavor`); only add new lines if the existing pools genuinely don't read right in arena context.
- **Phase-two thresholds.** Arena bosses get `PhaseTwoAt` defaults: tier 1–2 = no phase two (numeric drama only); tier 3 = 50% HP; tier 4–5 = 50% HP + a flavor barb. This gives arena fights a structural beat without overengineering.
- **Tier gating.** `tierFromCombatLevel` → `tierFromLevel`. Brackets:

  | DnDCharacter.Level | Arena tier |
  |---|---|
  | 1–3 | I (Pup) |
  | 4–7 | II (Pit) |
  | 8–12 | III (Crucible) |
  | 13–17 | IV (Coliseum) |
  | 18–20 | V (Apex) |

- **State.** `ArenaWins`, `ArenaLosses`, `InvasionScore` move to `player_meta` (per §2.1). XP awards go through whatever the existing D&D XP path is (single source of truth — verify in `dnd_sheet.go` during implementation).
- **Death.** Arena death now flows through the same `markAdventureDead(userID, "arena", arenaTierLabel)` hook the zone boss path uses. Hospital revive (post-L4a) heals from this same state.

**Steps:**
1. Factor `renderBossOutcome` helper out of `resolveBossRoom`. Two callers post-extraction: zone boss path and (TBD) arena path. Smoke-test the zone boss path is unchanged via `adventure_arena_test.go` parity once arena lands on it.
2. Add `ZoneArena` ID + minimal TwinBee mood pools (reuse where possible).
3. Convert arena monsters to boss-shaped bestiary entries with `PhaseTwoAt`.
4. Build `resolveArenaBoss(userID, encounter)` calling `runZoneCombat` + `renderBossOutcome`. Wire into existing arena entrypoint in `adventure_arena.go`.
5. Migrate `ArenaWins/Losses/InvasionScore` to `player_meta`. Dual-write per §11.
6. Swap render reads to DnDCharacter + player_meta.
7. Port `tierFromCombatLevel` → `tierFromLevel` with new brackets.
8. Port `adventure_arena_test.go`. Add a new test asserting arena win surfaces the staged combat log + TwinBee BossDeath line.
9. Drop AdvCharacter imports from arena files. **DEFERRED (2026-05-09):** blocked on DisplayName migration (L4f-prep, see §7) plus L4 hospital/pets/XP work. Arena code still needs `char.DisplayName` (stats DM, T5/helm announces, render), `char.PetName` (pet-recovery game-room msg), `char.DeathReprieveLast`, `char.CombatXP`, `char.Alive`/`transitionDeath`. Counter dual-writes to `char.ArenaWins/Losses/InvasionScore` could be dropped now (player_meta is source of truth post-step 6), but the surrounding `loadAdvCharacter`/`saveAdvCharacter` calls stay for the other fields, so step 9's exit grep can't pass at L2 time. Revisit after DisplayName migration ships and L4 (hospital/pets/render) lands.
10. `go test ./... && go vet`.

**Feature flag.** ~~Originally planned an `ARENA_BOSS_FLOW=1` soak.~~ Cancelled 2026-05-09: shipped no-flag, no legacy fallback. The legacy `runArenaCombat` / `RenderCombatLogArena` / `renderArenaCombatFinalMessage` / `renderArenaOutcome` / `arenaWin/LoseCloser` paths were deleted in the same commit. Boss-flow errors surface to the player and abort the round rather than falling back.

**Exit criteria:**
- ~~`grep -l 'AdventureCharacter\|CombatLevel\|combat_engine' internal/plugin/adventure_arena*.go` empty.~~ **Deferred** with step 9 — see note above. The grep will only clear once DisplayName migration + L4 (hospital/pets/XP/render) ship; L2 closes without it.
- Arena win produces a staged combat log + TwinBee BossDeath line + payout, just like a zone boss.
- Arena loss produces a TwinBee PlayerDeath line + arena death flag, no zone-run side effects.
- Existing arena DB rows readable; ArenaWins backfill from AdvCharacter into `player_meta` (§8).

**Risk:**
- D&D combat is swingier than legacy CombatPower; tune HP/AC tables against in-flight playtest data (no flag soak — legacy path is gone).
- Refactoring `resolveBossRoom` to extract `renderBossOutcome` could regress zone bosses. Mitigate: do the extraction in step 1 alone, ship it, run a full zone-boss test pass before continuing to step 2.

---

## 5. Phase L3 — Co-op dungeons (DELETION, not migration) — SHIPPED 2026-05-09

**Decision (2026-05-08):** Drop co-op dungeons entirely. Do **not** migrate. The user has a different design in mind for the future co-op slot; the current implementation is being retired wholesale.

**What shipped (2026-05-09):**
- 11 `coop_*.go` files deleted (no `coop_dungeon_balance.go` ever existed on disk — only `coop_dungeon_balance_test.go`).
- `internal/plugin/cleanup_l3.go` added: on every startup, cancels any `coop_dungeon_runs` left in `open`/`active`, refunds `coop_dungeon_members.total_contributed` and unsettled `coop_dungeon_bets` via `EuroPlugin.Credit`. Idempotent via per-run status-guarded UPDATE — only the writer that flips status to `cancelled` performs the refund for that run, so a crashed-then-restarted process can't double-credit. Tables left intact for historical querying; SQL drop deferred to a future GOGOBEE_COOP_PURGE pass.
- `adventure.go`: stripped `!coop` dispatch, `coopTicker` goroutine, startup `lockCoopCombatActions` call, `!coop` help line, ticker comment. Replaced startup hook with `closeAndRefundLegacyCoopRuns(p.euro)`.
- `adventure_scheduler.go`: removed `activeCoopMemberSet` load + skip branch and post-reset `lockCoopCombatActions` call.
- `adventure_render.go`: removed `renderCoopTeaser`, `pickCoopTeaserCandidate`, the `loadCoopRunForUser` block, and the in-coop combat-locked status line. Replaced with a temporary closure announcement in the morning DM ("**Co-op dungeons closed for now** — `!expedition` is the way forward; a better co-op design is on the way."). Marked with a TODO to remove after 2026-05-16.
- `adventure_followups_test.go`: removed `TestPickCoopTeaserCandidate_SkipsLeader`.

**Exit criteria — met:**
- `grep -rn 'coop_dungeon\|CoopDungeon' internal/plugin/ --include='*.go'` returns only the cleanup migration in `cleanup_l3.go` and a startup hook in `adventure.go` — no live system code references.
- `go vet ./...` clean.
- `go test ./...` clean.

**Open follow-ups:**
- Remove the morning-DM closure announcement after 2026-05-16 (one-week soak).
- Future GOGOBEE_COOP_PURGE pass drops `coop_dungeon_runs`/`_members`/`_events`/`_bets`/`_gifts` tables and the `cleanup_l3.go` startup hook.

---

## 6. Phase L4 — Stat/perk gates

**Goal:** Hospital, rival, masterwork, pets, housing, mortgage all read DnDCharacter + `player_meta` exclusively. AdvCharacter still exists but is unreferenced outside its own file.

### 6.1 L4a — Hospital
- `adventure_hospital.go`: cost formula `CombatLevel × 25k` → `Level × 50k`. `HospitalVisits` → `player_meta`. Revive flips DnDCharacter HP from 0 → full instead of `Alive=true`.

**Status (2026-05-09):** SHIPPED on `adv-2.0`.
- `player_meta.hospital_visits` column added via columnMigration in `internal/db/db.go`.
- `hospitalCostsForUser(char)` returns `(before, after)` using DnDCharacter.Level × 50k (5× before insurance), with a `dndLevelFromCombatLevel` fallback when no D&D row exists yet. Replaces the inline `CombatLevel × {125k,25k}` math at all three sites (handleHospitalCmd, resolveHospitalPay, sendHospitalAd).
- `loadHospitalVisits(userID)` reads `player_meta.hospital_visits` → falls back to `adventure_characters.hospital_visits` during soak.
- `upsertPlayerMetaHospitalVisits(userID, visits)` dual-writes the post-revive count alongside `saveAdvCharacter`.
- `backfillPlayerMetaHospitalVisits()` runs on every Init; idempotent (only fills zero rows so dual-writes survive re-runs).
- Revive (paid + on-demand + race-after-timer-expired) now also restores `DnDCharacter.HPCurrent = HPMax` via `LoadDnDCharacter` / `SaveDnDCharacter`. `char.Alive=true` still flips during soak — other systems (arena, scheduler) still read `Alive` and migrate later.
- Tests: `TestPlayerMetaHospitalVisitsBackfill_Idempotent`, `TestLoadHospitalVisits_FallsBackToAdvCharacter`, `TestUpsertPlayerMetaHospitalVisits_RoundTrip` (skip without prod DB, matching the L2/L4f-prep pattern).

### 6.2 L4b — Rival
- `adventure_rival.go`: unlock at Level ≥ 3 (was CombatLevel ≥ 5). RivalPool → `player_meta`. Duel combat already uses `combat_engine` — port to `simulateCombat` here too (small extra step).
- Move rival flavor into existing `adventure_flavor_rival.go` — no new file.

### 6.3 L4c — Masterwork
- `adventure_masterwork.go`: no AdvCharacter mutations today; only equipment reads. Port `advMasterworkSkillBonus` to take `*DnDCharacter` (skill fields added in §2.2). MasterworkDropsReceived → `player_meta`.

### 6.4 L4d — Pets
- `adventure_pets.go`: pet fields move from AdvCharacter to `player_meta` (see §2.1 `pet_*` columns). Pet combat hooks (PetMorningDefense, pet damage rolls) target DnDCharacter HP.

### 6.5 L4e — Housing & mortgage
- `adventure_housing.go`, `adventure_mortgage.go`: HouseTier/loan fields → `player_meta`. `houseHPBonus` → applied to `DnDCharacter.HPMax` calculation (already a parallel hook in `dnd_sheet.go`'s `recomputeHPMax`).
- Mortgage scheduler tick stays — only the field source changes.

### 6.6 L4f — TwinBee/render
- `adventure_render.go`: morning DM reads from DnDCharacter + `player_meta`. Coop teaser gating moves to DnDCharacter.Level.
- `adventure_twinbee.go`: simulator already addressed in L1. Remaining usage is read-only stat display — port to DnDCharacter.

**Steps (per sub-phase):** column add → backfill → swap reads → swap writes → port tests → grep check.

**Exit criteria L4 overall:** `grep -l 'AdventureCharacter\|CombatLevel' internal/plugin/adventure_{hospital,rival,masterwork,pets,housing,mortgage,render,twinbee}.go` empty.

**Risk:** Each sub-phase is independent — can interleave with playtest. Housing has the most fields; do it last in L4 since the column count is largest.

---

## 7. Phase L5 — Teardown

**Goal:** Delete legacy types, files, and `combat_engine`. Branching dungeon paths become buildable.

**Pre-condition: DisplayName migration (L4f-prep, ~2026-05-09 note).** The plan as originally written never picked a new home for `AdventureCharacter.DisplayName`. It is *not* legacy-specific — it's the player's chosen identity, used in ~250 spots across arena, rival, scheduler, expedition, holdem, coop. L5's pre-condition `grep AdventureCharacter == 0` cannot pass while DisplayName still lives on AdvCharacter.

**Recommendation:** add `display_name TEXT NOT NULL DEFAULT ''` to `player_meta` as a self-contained step before L4f. Backfill from `adventure_character.display_name` (same shape as §8). Add a `loadDisplayName(userID)` helper that reads from `player_meta` and falls back to AdvCharacter for one soak week, then flip readers. Per-call-site swap is mechanical (`char.DisplayName` → `loadDisplayName(uid)` or pass-through from caller).

**Sizing:** ~0.5 day for the schema/backfill/helper, then 1 day to swap the ~250 read sites in batches per file. Slot it after L2 wraps and before L4f starts.

**Status (2026-05-09):** schema/backfill/helper SHIPPED on `adv-2.0`.
- `player_meta.display_name` column added via column migration in `internal/db/db.go`.
- `backfillPlayerMetaDisplayName` runs on every Init: idempotent (`UPDATE ... WHERE display_name = ''` only touches empty rows, so dual-writes layered on top survive re-runs).
- `upsertPlayerMetaDisplayName(userID, name)` dual-writes from `createAdvCharacter` (inside the same tx, atomic with the AdvCharacter INSERT) and `saveAdvCharacter` (post-commit, error-logged but non-fatal).
- `loadDisplayName(userID)` reads `player_meta` → falls back to `adventure_characters.display_name` → empty string if neither exists.
- Tests: `TestPlayerMetaDisplayNameBackfill_Idempotent`, `TestLoadDisplayName_FallsBackToAdvCharacter`, `TestCreateAdvCharacter_DualWritesDisplayName`.

**Reader flip SHIPPED (2026-05-09).** All `char.DisplayName` / `c.DisplayName` reader sites in `internal/plugin/` were swapped to `loadDisplayName(userID)` (or pass-through equivalents) across: `combat_bridge.go`, `dnd_zone_cmd.go`, `dnd_expedition_combat.go`, `dnd_zone_combat.go`, `adventure_robbie.go`, `adventure_scheduler.go`, `adventure_hospital.go`, `adventure_events.go`, `adventure_render.go`, `adventure_masterwork.go`, `adventure.go`, `adventure_arena.go`. The only remaining `char.DisplayName` references are in `adventure_character.go` itself (scan into struct + `saveAdvCharacter` SQL + dual-write to `player_meta`) and the `player_meta.go` doc comment. `go vet` + `go test ./internal/plugin/...` clean.

**Pre-conditions:**
- L1–L4 all shipped.
- DisplayName migrated to `player_meta` (see above).
- `grep -rn 'AdventureCharacter' internal/plugin/ --include='*.go' | grep -v adventure_character.go | grep -v _test.go` returns 0.
- `grep -rn 'CombatLevel' internal/plugin/ --include='*.go'` returns 0 (or only in archive/migration code).

**Files deleted:**
- `adventure_character.go`
- `adventure_activities.go` (already gutted in L1)
- `combat_engine.go`
- `combat_engine_test.go`
- `combat_bridge.go`
- `combat_bridge_test.go`
- `combat_stats.go` (if unused; verify)
- `combat_stats_test.go`

**DB cleanup:**
- `adventure_character` table: drop after a 30-day grace period in case of rollback. Schedule with a migration script `dnd_l5_cleanup.go` that runs only when env var `GOGOBEE_LEGACY_PURGE=1` is set, to gate the destructive op. Default off; manual flip after confirming.
- Equipment table stays — it was always per-user, never per-AdvCharacter.

**Code cleanup:**
- Remove `AdvLocation` tier data unless something else picked it up.
- Remove all `AdvBonusSummary` plumbing.
- Final `go vet ./... && go test ./...` clean.

**Branching dungeon paths:** unblocked. The `dnd_zone_run` schema is the only one we still own; rebuild/expand it without AdvCharacter pressure. Out of scope for this doc — see `project_branching_paths_post_legacy.md` memory note.

---

## 8. Cross-cutting — DB backfills

Every phase that moves a field from `adventure_character` to `player_meta` runs the same shape of backfill:

```go
// In Init() or a one-shot migration runner
func backfillPlayerMeta(db *sql.DB) error {
    rows, _ := db.Query(`SELECT user_id, arena_wins, arena_losses, ... FROM adventure_character`)
    for rows.Next() {
        // INSERT OR IGNORE into player_meta
    }
}
```

**Idempotency rules (per Phase R1's `archiveOrphanZoneRuns` precedent):**
- All backfills use `INSERT OR IGNORE`.
- All backfills are safe to re-run.
- Each backfill logs the row count it touched.

**Order of backfills:**
1. L1 — `babysit_*`, `auto_babysit*` columns.
2. L2 — `arena_wins`, `arena_losses`, `invasion_score`.
3. L3 — none (co-op didn't store on AdvCharacter).
4. L4a — `hospital_visits`, `last_death_date`, `death_source`, `death_location`, `last_pardon_used`, `death_reprieve_last`.
5. L4b — `rival_pool`, `rival_unlocked_notified`.
6. L4c — `masterwork_drops_received`, `treasures_locked`.
7. L4d — pet fields.
8. L4e — house fields.
9. L4f — streak/title/grudge fields, NPC msg counters, NPC seen/buff timestamps.
10. L5 — drop `adventure_character` table.

---

## 9. Testing strategy

- Each phase ships with new tests for the migrated subsystem and a `*_migration_test.go` covering backfill idempotency (matching R1's pattern).
- Existing tests are *ported*, not deleted — same test names, fixtures swap from AdvCharacter to DnDCharacter.
- A new `legacy_residue_test.go` runs at the package level and `grep`s the source tree for forbidden symbols once each phase lands; this guards against regressions.

```go
// legacy_residue_test.go skeleton
func TestNoCombatLevelReferencesAfterL5(t *testing.T) {
    if !legacyMigrationDone { t.Skip() }
    // walk filepath, fail on grep "CombatLevel"
}
```

---

## 10. Sequence & rough sizing

| Phase | Est. session count | Blocking on |
|---|---|---|
| L1 — Babysit/scheduler | 1 | — |
| L2 — Arena (boss flow rebuild) | 2–3 | feature flag in place; `renderBossOutcome` extraction lands first |
| L3 — Co-op dungeons (DELETE) | 0.5 | — (independent of L2) |
| L4a — Hospital | 0.5 | — |
| L4b — Rival | 1 | — |
| L4c — Masterwork | 0.5 | DnDCharacter skill fields exist (§2.2) |
| L4d — Pets | 1 | `player_meta` schema |
| L4e — Housing/mortgage | 1 | — |
| L4f — TwinBee/render | 0.5 | L1–L4 done |
| L5 — Teardown | 1 | all above + 30-day grace |

**Total:** ~10 sessions over ~3–6 weeks of real time, including playtest soak between phases. Don't compress this into a single sprint — each phase needs production exposure before the next one builds on it.

---

## 11. Rollback strategy

Each phase is reversible until L5:
- L1–L4 do not delete columns or tables. They add `player_meta` columns and dual-read briefly during the swap.
- If a phase regresses, revert the read-site swap; backfill data still flows back to `adventure_character` because L1–L4 do **not** stop writing to AdvCharacter until §6 dual-write is dropped per phase.

**Dual-write rule:** for the duration of L1–L4, every write to a migrated field goes to **both** AdvCharacter and `player_meta`. Reads come from `player_meta`. This is removed phase-by-phase only after one full real week with no rollback triggered.

L5 is the irreversible step. Gate on `GOGOBEE_LEGACY_PURGE=1`.

---

## 12. Decisions locked (2026-05-08)

All four pre-implementation questions answered:

1. **CombatLevel→Level:** direct copy. CombatLevel doesn't have semantic meaning — it's just "starting Level." No formula, no cap-compression.
2. **AdvEquipment:** kept as-is. No rename. Type stays `AdvEquipment`, table stays `adventure_equipment`. (Naming guidance: avoid new `dnd_*` symbols throughout this migration — see §2.1 callout.)
3. **CombatXP→XP:** discarded. Players start fresh; new XP accumulates from 0 at the floor of their migrated Level.
4. **Co-op dungeons:** dropped entirely (see §5). Not migrated. Different design coming later.

No outstanding pre-implementation questions. Tuning questions during each phase (HP/AC tables, threshold brackets) get decided in the moment.

---

*End of Legacy Migration document.*
*Pair with companion docs in Claude Code sessions when working any L-phase.*
