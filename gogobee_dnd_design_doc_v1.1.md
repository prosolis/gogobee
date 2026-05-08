# GogoBee — D&D Integration Design Document (v1.1, annotated)

> **Version:** 1.1 — integration revision
> **Status:** Draft, supersedes v1.0 for implementation purposes
> **Predecessor:** `gogobee_dnd_design_doc.md` (v1.0) — kept as the vision document
> **Scope:** Same as v1.0, reconciled against the existing `internal/plugin/adventure_*.go` and `internal/db/db.go` codebase
> **See also:** `gogobee_equipment_appendix.md`

---

## ⚠️ Protected Files — DO NOT MODIFY

Unchanged from v1.0. All `*_flavor.go` / `*_flavor.txt` files, files containing the `DO NOT REWRITE` header, and NPC spec files (Thom Krooke, Misty, Arina) are immutable. New D&D flavor lives in **new** files (`dnd_flavor_*.go`), never as edits to existing flavor.

---

## 0. Why v1.1 exists

v1.0 was written without auditing the live adventure system. A literal implementation would destroy working features and player progress. This revision:

1. **Treats the existing adventure systems as ground truth.** The D&D layer is *additive*, not a replacement. `adventure_characters` and friends keep their columns and semantics; D&D adds nullable columns and new tables alongside.
2. **Preserves all player progress.** Every column in `adventure_characters`, `adventure_equipment`, `adventure_inventory`, `adventure_treasures`, `adventure_buffs`, `user_archetypes`, `arena_stats`, `adventure_rival_records`, `adventure_babysit_log`, `holdem_scores`, `euro_balances`, etc. is preserved unchanged. Migrations are **column-adds with defaults only** — no drops, no renames.
3. **Gates D&D content on opt-in.** A `pending_setup` flag means non-opted-in players continue with the current adventure system unchanged. D&D-flavored output (`!sheet`, combat resolution narration) is only shown to players who have completed `!setup`.
4. **Re-uses what already exists** rather than reinventing it: `combat_level` becomes D&D level, `adventure_treasures` becomes the attunement substrate, `adventure_equipment` slots map forward, `user_archetypes` drives class/race inference (already designed for this).

Wherever this revision diverges from v1.0, the divergence is annotated with **[REVISED]** and a one-line rationale.

---

## 1. Overview & Goals

Unchanged from v1.0 except for one added principle:

- **Layered, not replaced.** D&D mechanics sit on top of the existing adventure systems. The world layer (skills, encounters, loot, babysit, rivals, housing) keeps running; the character layer (race/class/HP/AC/abilities) is a new lens over it.

---

## 2. Existing System Inventory — actual state

**[REVISED — v1.0's table was aspirational. This is what's actually in `internal/db/db.go` and `internal/plugin/`:]**

### 2.1 Player tables (preserve every column)

| Table | Key columns | Status |
|---|---|---|
| `adventure_characters` | `user_id` (PK), `display_name`, `combat_level`, `combat_xp`, `mining_skill`/`mining_xp`, `foraging_skill`/`foraging_xp`, `fishing_skill`/`fishing_xp`, `alive`, `dead_until`, `action_taken_today`, `holiday_action_taken`, `current_streak`, `best_streak`, `arena_wins`, `arena_losses`, `invasion_score`, `pet_*`, `house_*`, `babysit_*`, `auto_babysit`, `misty_buff_expires`, `misty_debuff_expires`, `arina_buff_expires` | **Preserve all.** D&D columns added nullable. |
| `adventure_equipment` | `(user_id, slot)` PK, `tier`, `condition`, `name`, `actions_used`, `masterwork`, `skill_source` | Preserve. Add nullable D&D columns: `rarity`, `stat_bonus_json`, `attuned`. |
| `adventure_inventory` | `id` (PK), `user_id`, `name`, `item_type`, `tier`, `value`, `slot`, `skill_source` | Preserve. Add nullable `rarity`, `stat_bonus_json`. |
| `adventure_treasures` | `(user_id, treasure_key, bonus_type)` UNIQUE, `bonus_value` (REAL) | **Preserve and re-use as attunement substrate** (Section 7.3). |
| `adventure_buffs` | `user_id`, `buff_type`, `buff_name`, `modifier`, `expires_at` | Preserve. Used for D&D conditions. |
| `user_archetypes` | `(user_id, archetype)` PK, `category`, `signal_score`, `flavor`, `assigned_at` | Preserve. Drives migration inference (Section 11). |
| `arena_runs`, `arena_history`, `arena_stats` | streaks, win/loss, history | Preserve. Combat resolution swaps under the hood; records stay. |
| `adventure_rival_challenges`, `adventure_rival_records` | challenge state, head-to-head records | Preserve. RPS resolver replaced; tables untouched. |
| `adventure_babysit_log` | activity, outcome, gold, XP, items_dropped per day | Preserve. Babysit stays independent. |
| `euro_balances` | coin balance | Preserve. Coins are euros; no separate D&D gold. |
| `hangman_scores`, `blackjack_scores`, `holdem_scores` | per-game scores | Preserve. Independent of D&D layer. |

### 2.2 New tables for D&D layer

| Table | Purpose |
|---|---|
| `dnd_character` | Per-player D&D state: race, class, hp_current, hp_max, temp_hp, ac, str/dex/con/int/wis/cha, dnd_level, pending_setup, created_at, updated_at. **One row per player after `!setup`; absent before.** |
| `dnd_abilities` | `(user_id, ability_id)` — known abilities and uses-remaining of the relevant resource. |
| `dnd_resources` | `(user_id, resource_type)` — current Stamina/Focus/Mana/DivineFavor; reset on rest. |
| `dnd_combat_state` | Active combat encounter (round, turn, conditions JSON, log JSON). Cleared on combat end. |
| `adventure_characters_pre_dnd` | One-shot mirror of `adventure_characters` taken at deploy time. Rollback safety net. |

### 2.3 Things that don't yet exist anywhere

- HP, AC, ability scores, conditions in any persistent form
- Initiative, attack rolls, saving throws — current arena/rival are RPS or level-comparison
- Global player XP separate from skill XP

These are genuinely new and require fresh implementation; everything else is layering.

---

## 3. Character System

### 3.1 Races, 3.2 Classes, 3.3 Ability Scores

Tables unchanged from v1.0. The race/class tables and stat-modifier formula stand.

**[REVISED]** Class ↔ archetype inference is now keyed to the *real* archetype names from `archetype.go`, not the abstract categories in v1.0:

| Archetype (real) | Suggested Class | Suggested Race |
|---|---|---|
| Arena Champion / Dungeon Crawler | Fighter | Orc / Human |
| The Merchant / Whale | Rogue | Halfling / Human |
| Novelist / Philosopher / Wordsmith / Linkmaster | Mage | Elf / Tiefling |
| Cheerleader / Hype Machine / Patron / Reactor | Cleric | Dwarf / Half-Elf |
| The Angler / The Forager / The Adventurer | Ranger | Human / Elf |
| Fallback (Regular, Wildcard, no clear signal) | Human Fighter | (presented as suggestion only) |

When a player has multiple archetypes, the highest `signal_score` wins. Ties broken by archetype priority list defined in code.

### 3.4 Go data model

**[REVISED]** Drop the v1.0 `CharacterSheet` struct in favor of a model that mirrors the actual table layout and keeps `adventure_characters` as the source of truth for everything that already exists there.

```go
// DnDCharacter is the new persistent layer. Joined with adventure_characters at read time.
type DnDCharacter struct {
    UserID       string
    Race         Race
    Class        Class
    DnDLevel     int       // derived from combat_level at migration; advances independently after
    HP           HPBlock
    AC           int
    Stats        BaseStats // STR/DEX/CON/INT/WIS/CHA
    PendingSetup bool      // true until !setup completes
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// CharacterView is a read-only join used by !sheet and combat.
// Always construct via repository.LoadCharacterView(userID) — never instantiate directly.
type CharacterView struct {
    Adv  AdventureCharacter // existing struct from adventure_character.go
    DnD  *DnDCharacter      // nil when player hasn't run !setup
    Eq   []EquipmentRow     // existing
    Inv  []InventoryRow     // existing
    Treas []TreasureRow     // existing — surfaces as attunements when DnD != nil
}
```

`Race`, `Class`, `BaseStats`, `HPBlock` constants and types follow v1.0 exactly.

---

## 4. Leveling System

**[REVISED — this is the largest change from v1.0.]**

v1.0 specifies a fresh XP curve (300, 900, 2700…) with no relationship to existing skill XP. That throws away hours of player progress. Instead:

### 4.1 Initial mapping (one-time, at `!setup` completion)

```
dnd_level = max(1, combat_level)         // direct adoption — combat_level is literally a combat level
xp_carryover = 0                         // dnd has its own XP track going forward
```

Skill levels (mining/foraging/fishing) and their XP pools are **untouched**. They keep advancing on their own from gathering activities.

### 4.2 New XP sources for D&D level

D&D level advances from:
- Combat encounters (arena wins, rival duels, monster kills) — primary source
- Quest/event completions
- NPC skill-check successes (small amounts)

**Not** from gathering. Mining XP keeps going to mining; that channel is unchanged.

### 4.3 XP curve from level N onward

The cumulative table from v1.0 (`L2=300, L3=900, L5=2700, L10=14000, L20=85000`) applies, but **as XP earned past initial level**, not from zero. A player migrated in at `combat_level=8` starts D&D Level 8 with 0/required-for-9 XP.

### 4.4 Level-up flow

Same as v1.0 (5 steps, including stat point allocation and subclass prompt at L5). Level-up flavor must be class-specific and lives in a new `dnd_flavor_levelup.go` (new file, not editing existing flavor).

---

## 5. Combat Engine — staged rollout

**[REVISED]** v1.0 specifies the full D&D combat flow as if combat were greenfield. It isn't — arena and rival have semantics today, plus history. Stage the rollout:

### 5.1 Stage A — Arena conversion (do first)

- Replace the level-comparison + 50/50 winner logic with full D&D resolution (initiative → attack rolls vs. AC → damage → conditions).
- **Preserve**: `arena_stats`, `arena_history`, `current_streak`, `best_streak`, `arena_wins`, `arena_losses` rows. New combat writes to the same tables.
- Streak bonuses (+5%/3-streak, etc.) become combat modifiers in the new resolver.
- Non-opted-in players (no `dnd_character` row) still hit the legacy resolver. The arena gracefully runs both.

### 5.2 Stage B — Rival conversion (do second)

- Replace RPS in `adventure_rival.go` with D&D combat.
- `adventure_rival_records` and `adventure_rival_challenges` schemas unchanged. New resolver writes the same rows.
- Stake calculation (`(combatLevel/5)*1000` euros) preserved.

### 5.3 Stage C — Encounter combat (new)

- New monsters / dungeon encounters use D&D combat from day one. No legacy path.

### 5.4 Combat resolution flow, AC, conditions, data model

Sections 5.1, 5.2, 5.3, 5.4 of v1.0 stand as the spec for **how** combat works. Apply at the resolver layer; persist via `dnd_combat_state` (new table).

---

## 6. Abilities & Spells

Unchanged from v1.0. Resources persist in `dnd_resources` (new table). Reset triggered by `!rest short`/`!rest long` (Section 10 below — note the rename).

---

## 7. Equipment & Item System

### 7.1, 7.2 Rarity & Slots

Tables stand from v1.0.

### 7.3 Slot mapping — additive, not replacement

**[REVISED]** Existing `adventure_equipment` has 5 slots. Map them forward instead of orphaning gear:

| Existing slot | D&D slot |
|---|---|
| `weapon` | `main_hand` |
| `armor` | `chest` |
| `helmet` | `head` |
| `boots` | `feet` |
| `tool` | `off_hand` (or `hands`, by item type) |

Old gear keeps `tier`, `condition`, `masterwork`. Inferred `rarity` is computed from `tier` at migration (e.g., tier 1-2 → Common, 3-4 → Uncommon, 5-6 → Rare, 7+ → Epic). New rarity column is nullable; legacy items render fine without it.

D&D adds the new slots (`legs`, `hands` if not already mapped, `ring_1`, `ring_2`, `amulet`) — these start empty for migrated players.

### 7.4 Attunement = adventure_treasures

**[REVISED]** v1.0 invents a parallel attunement system. Don't. `adventure_treasures` already encodes *named bonuses with stat modifiers per player* — that's exactly an attunement record.

- The 3-item attunement cap = a row-count constraint on `adventure_treasures` per `user_id`.
- `bonus_type` and `bonus_value` (REAL) feed directly into stat resolution.
- New magic items granted post-D&D-launch insert rows into the same table.
- Legacy treasures are automatically "attuned" — no migration step required.

### 7.5 Go data model

Use a thin wrapper that joins `adventure_equipment` + `adventure_inventory` + `adventure_treasures` at read time. Don't introduce a new `Inventory`/`EquipmentSlots` struct that duplicates state.

---

## 8. Monster Bestiary

Unchanged from v1.0. New tables, new flavor file (`dnd_flavor_monsters.go`).

---

## 9. Skill Check System

Unchanged from v1.0. NPC wiring (Thom Krooke / Misty / Arina) is purely new logic; no schema change.

One nuance: the existing Misty/Arina buff/debuff timestamp columns (`misty_buff_expires`, `misty_debuff_expires`, `arina_buff_expires`) **stay as-is** and become the persistence layer for skill-check-granted buffs. Don't add parallel buff tracking.

---

## 10. Rest System — naming collision resolved

**[REVISED]**

`!adventure rest` already exists as a daily-action narrative activity. It's not the same thing as a D&D short/long rest.

Rename and split:

- **`!adventure camp`** — what `!adventure rest` is today. Daily-action narrative day-skip. (Old command remains as an alias for one release for backwards compat.)
- **`!rest short`** — D&D short rest. No daily-action cost. 1-hour cooldown. Recovers 1d6+CON HP; resets short-rest resources (Fighter Stamina, Rogue/Ranger Focus).
- **`!rest long`** — D&D long rest. Requires housing or paid inn use (Thom Krooke). 24-hour cooldown. Full reset.

Babysit interaction: babysitting is independent of rest. A player on auto-babysit can still `!rest short`. `!rest long` while babysitting is allowed and does not interrupt the babysit log.

---

## 11. Player Onboarding & Migration

### 11.1 New user flow

Unchanged from v1.0. A new player runs `!setup` and gets the 4-step race/class/stats/welcome flow. This creates their `adventure_characters` row (if absent) **and** their `dnd_character` row in one transaction.

### 11.2 Existing user migration

**[REVISED]** Migration is opt-in via `pending_setup`. No data is mutated until the player runs `!setup`.

At deploy:
1. Snapshot `adventure_characters` → `adventure_characters_pre_dnd` (rollback safety).
2. **No row changes.** No `dnd_character` rows created. Every existing player is implicitly `pending_setup = true` because the row simply doesn't exist yet.
3. Bot DMs each active player **once** with the v1.0 migration message (suggested race/class from archetype, "Yes / No, let me choose / Skip for now"). Reminder cadence per v1.0 (every 7 days, auto-assign at 30 days).

While `pending_setup` is true:
- All existing `!adventure ...` commands work exactly as before.
- D&D-only commands (`!sheet`, `!stats`, `!use`, `!rest short/long`, `!check`) prompt for setup.
- Arena/rival use the legacy resolver.

After `!setup`:
- `dnd_character` row is created.
- `dnd_level = combat_level` (Section 4.1).
- HP_max computed from class HP die + CON modifier × level. Current HP set to max.
- AC computed from equipped armor + DEX. (Old armor maps via Section 7.3.)
- Arena/rival flip to D&D resolver for this user.

### 11.3 Auto-assign

After 30 days `pending_setup` and at least one prior interaction, auto-create `dnd_character` using inferred race/class from archetype. Player notified once: "We started a sheet for you — `!setup` to change it."

### 11.4 Data preservation guarantees

| Existing data | Guarantee |
|---|---|
| `euro_balances` | Untouched. Coins do not convert. |
| Skill levels & XP (`mining_*`, `foraging_*`, `fishing_*`) | Untouched. Skill grind continues independently. |
| `combat_level` | Read once at `!setup` to seed `dnd_level`. After that, both fields exist and advance independently (combat_level keeps incrementing on legacy paths if any remain; dnd_level is the canonical character level for D&D). |
| `combat_xp` | Untouched. Used as legacy display. |
| Pets (`pet_*`) | Untouched. Ranger gets passive bonus *on top*. |
| `arena_wins`/`losses`/`current_streak`/`best_streak` | Untouched. New combats append to the same counters. |
| `adventure_inventory` | Untouched. Items render with inferred rarity until D&D fields populated by new drops. |
| `adventure_equipment` | Untouched. Slot mapping is read-time, not a migration. |
| `adventure_treasures` | Untouched. Becomes attunement display source. |
| `adventure_buffs` | Untouched. Used for new condition tracking too. |
| `user_archetypes` | Untouched. Drives inference; retained for flavor post-setup. |
| `house_tier`, `house_loan_*` | Untouched. Drives long-rest eligibility. |
| `adventure_babysit_log` and babysit fields | Untouched. Babysit independent of D&D. |
| `holdem_scores`, `hangman_scores`, `blackjack_scores` | Untouched. Out of D&D scope entirely. |

**Rollback plan:** drop the new D&D tables and the nullable D&D columns. `adventure_characters_pre_dnd` is the integrity check that nothing was clobbered. If anything in `adventure_characters` differs from the pre-DnD snapshot at rollback time, that's a bug — restore the affected rows from the snapshot.

---

## 12. Cross-System Integration

Substantively unchanged from v1.0, with these additions:

- **Babysit** is independent of D&D. Stays as-is.
- **Forex / lottery / coop dungeon / holdem** are out of D&D scope and unchanged.
- **Skill XP from gathering** does **not** feed D&D level. Two distinct progression tracks (skill grind vs. D&D character level) coexist.

---

## 13. Commands Reference

v1.0's table stands, with these additions/clarifications:

| Command | Note |
|---|---|
| `!adventure camp` | Renamed from `!adventure rest`. Old name kept as alias for one release. |
| `!rest short` / `!rest long` | New D&D rests, gated on `dnd_character` existing. |
| `!equip <item>` | **Subsumes** `!adventure equip`. Routes to legacy masterwork-equip path when item is masterwork; otherwise D&D equip. |
| `!inventory` | **Extends** `!adventure inventory` with rarity/attunement display. Legacy `!adventure inventory` continues to work. |
| `!sheet`, `!stats`, `!level`, `!abilities`, `!use`, `!check`, `!roll`, `!respec`, `!attune`, `!unequip`, `!setup` | All new, no current implementation. |

---

## 14. Implementation Phases — revised

**[REVISED — replaces v1.0 Section 14 in full.]**

### Phase 1 — Layering proof (smallest viable change)

Goal: prove D&D layers cleanly on existing data without changing any behavior for existing players.

- [ ] Schema: add `dnd_character`, `dnd_abilities`, `dnd_resources`, `dnd_combat_state` tables. Add nullable D&D columns to `adventure_equipment` / `adventure_inventory`.
- [ ] One-shot snapshot `adventure_characters_pre_dnd` at deploy.
- [ ] `combat_level` → D&D level mapping function (read-only).
- [ ] `!setup` flow (4-step) with archetype inference.
- [ ] `!sheet` rendering — read-only join over existing data + `dnd_character`. No behavior change.
- [ ] `pending_setup` gating on D&D-only commands.

Exit criteria: opted-in player sees correct `!sheet`. Non-opted-in player notices nothing different.

### Phase 2 — Combat engine + arena conversion (Stage A)

- [ ] Initiative, attack rolls vs. AC, crit/fumble, damage, conditions, saving throws.
- [ ] HP/AC computation on `!setup` and on level-up.
- [ ] Arena resolver swap for opted-in players. Legacy resolver kept for non-opted-in.
- [ ] `dnd_combat_state` persistence and combat log.

### Phase 3 — Abilities, resources, leveling

- [ ] XP tracking (D&D-level XP separate from skill XP).
- [ ] Level-up flow with class-specific flavor (new `dnd_flavor_levelup.go`).
- [ ] Starter abilities per class.
- [ ] `!use`, `!abilities` commands.
- [ ] Resource tracking and short/long rest reset.

### Phase 4 — Equipment expansion + attunement re-use

- [ ] Equipment slot mapping (Section 7.3).
- [ ] Rarity inference for legacy gear at read time.
- [ ] `adventure_treasures`-as-attunement (Section 7.4).
- [ ] `!equip`, `!unequip`, `!attune`, `!inventory` subsuming legacy commands.
- [ ] Thom Krooke shop additions (Common/Uncommon equipment, potions).

### Phase 5 — Skill checks + NPC integration

- [ ] Skill check resolver, DC table.
- [ ] Wire Thom Krooke (Persuasion), Misty (Insight), Arina (Arcana) using existing buff timestamp columns.
- [ ] `!check` command.

### Phase 6 — Rest system

- [ ] `!rest short` / `!rest long` with housing gating.
- [ ] Inn cost at Thom Krooke.
- [ ] Rename `!adventure rest` → `!adventure camp` with alias.

### Phase 7 — Monsters, rival conversion (Stage B), encounter combat (Stage C)

- [ ] Starter bestiary (Section 8.2 minimum) in new files.
- [ ] Rival RPS → D&D combat swap.
- [ ] Encounter-driven combat for new content.
- [ ] Cross-system polish (Ranger pet/fishing bonuses on top of existing bonuses).

---

## 15. Configuration Constants

Unchanged from v1.0. Add:

```go
const (
    DnDLevelFromCombatLevel = true // initial mapping at !setup
    PendingSetupReminderDays = 7
    PendingSetupAutoAssignDays = 30
    LegacyArenaResolverEnabled = true // until 100% of active players opted in
)
```

---

## 16. Out of Scope

Same as v1.0. Plus:

- **Forex, lottery, coop dungeon, holdem, blackjack, hangman, tarot, uno** — stay out of D&D.
- **Skill grind XP feeding D&D level** — explicitly rejected. Two tracks.
- **Coin → gold conversion** — euros stay euros.

---

## 17. Open questions (must be resolved before Phase 1 ships)

1. **Stat generation method** at `!setup`: point-buy (27 points, standard array) or 4d6-drop-lowest? (v1.0 left TBD.)
2. **Respec cost / cooldown.** (v1.0 TBD.)
3. **Inn cost** for long rest at Thom Krooke. (v1.0 TBD.)
4. **`combat_level` post-setup**: does it keep advancing on legacy paths, or freeze at setup time and only `dnd_level` moves forward? Recommend freeze.
5. **Subclass content** for L5 prompt. v1.0 defers; need at least placeholder choices to not block the level-up flow.

---

*End of v1.1. Differences from v1.0 are tagged **[REVISED]**. Where this document is silent, v1.0 is authoritative. Where they conflict, v1.1 wins for implementation purposes.*
