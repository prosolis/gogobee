# GogoBee D&D Integration — Session Summary

> **Scope:** End-to-end build of the D&D layer from design through audit cleanup, audit fixes, flavor wiring, branding rebrand to "Adv 2.0", and Phase 8 (equipment & damage math).
> **Status:** Deploy-ready. **122 D&D tests** passing, full suite + vet clean, prod-DB integration verified, root build with `-tags goolm` clean.
> **Source design:** Eleven design docs at the repo root — see `gogobee_design_index.md` for the canonical list and dependency graph. Phases 1-7 + audit fixes were built from `gogobee_dnd_design_doc.md` v1.0 / `gogobee_dnd_design_doc_v1.1.md` (implementation revision); Phase 8 from `gogobee_equipment_appendix.md`.
> **What's next:** Phase 9 — spell system (`gogobee_spell_system.md`). See the auto-loaded memory entry `project_dnd_phase_plan.md` for the full Phase 8-16 roadmap.

---

## 1. Design documents produced

| Document | Purpose |
|---|---|
| `gogobee_dnd_design_doc.md` (existing v1.0) | Vision document. Untouched. |
| `gogobee_dnd_design_doc_v1.1.md` (new) | Implementation revision. Reconciles v1.0 against the actual codebase: real schema, real archetype names, real combat engine. Replaces v1.0's Phase 14 with a smaller-scoped phase plan. Annotated with `[REVISED]` tags everywhere it diverges from v1.0. |
| `gogobee_dnd_session_summary.md` (this file) | Session-wide change log. |
| `gogobee_design_index.md` | **Canonical map of all 11 design docs** + their dependency graph + reading order by intent. New sessions should read this first. |

The original design doc treated combat as greenfield. An audit revealed `combat_engine.go`, `combat_bridge.go`, `combat_stats.go`, and `combat_narrative.go` (~3,500 LOC) already existed in production. v1.1 was rewritten around that constraint: the D&D layer would be additive, not a replacement.

---

## 2. Phase-by-phase delivery

### Phase 1 — Layering proof
- Schema: `dnd_character`, `dnd_abilities`, `dnd_resources`, `dnd_combat_state`, plus a one-shot `adventure_characters_pre_dnd` snapshot table populated at deploy as a rollback safety net.
- Nullable D&D columns added to `adventure_equipment` / `adventure_inventory` (rarity, stat_bonus_json, attuned).
- Race/class types and tables (7 races, 5 classes), repository functions, stat math (`abilityModifier`, `computeMaxHP`, `computeAC`).
- `!setup` flow with persistent draft state (`pending_setup=1`) — survives bot restarts.
- `!sheet` read-only renderer with archetype-driven inference.

### Phase 2 — d20-vs-AC combat
- `CombatStats` extended with `AC` and `AttackBonus`; `CombatEvent` with `Roll` and `RollAgainst`.
- `resolvePlayerAttack` and `resolveEnemyAttack` rewritten as d20-vs-AC. Nat 20 = auto-hit + crit, nat 1 = auto-miss tagged "fumble".
- `applyDnDPlayerLayer` / `applyDnDArenaMonsterLayer` / `applyDnDDungeonMonsterLayer` populate AC and attack bonuses based on class/level and threat/tier.
- `persistDnDHPAfterCombat` writes proportional damage back to the sheet.
- **Auto-migration** on first combat: legacy players with no `dnd_character` row get one built from archetype inference + class-tuned standard array. `auto_migrated=1` flag enables free `!setup` rebuild without consuming `!respec` cooldown.
- **Legacy combat ripped out**: dual-path branching deleted, `DnDMode` field removed. Single d20 path for all combat.

### Phase 3 — XP, level-up, class passives
- `dndXPTable` cumulative table anchored to v1.0's design values; `dndXPToNextLevel` returns segment cost.
- `grantDnDXP` handles cascading level-ups, recomputes HP max, increments current HP by the gain, sends a level-up DM. Caps cleanly at L20.
- XP grants on every arena and dungeon combat, with near-death win bonuses.
- One class passive per class via `applyClassPassives`: Fighter +5% damage, Rogue auto-crit-first, Mage +1 attack bonus, Cleric +5 HP heal at low HP, Ranger +5% damage and +1 attack bonus.
- `!abilities` lists class + race passives. `!sheet` shows XP progress.

### Round 1 — race passives, freeze, respec, subclass stub
- Race passives via `applyRacePassives` and three new `CombatModifiers` fields: Halfling Lucky (reroll first nat 1), Orc Rage (+50% damage on first attack after dropping below 50% HP), Dwarf poison resistance (poison ticks halved).
- `combat_level` freeze: once a player has confirmed a D&D character, `checkAdvLevelUp("combat", ...)` becomes a no-op. Skill levels (mining/foraging/fishing) keep advancing independently.
- `!respec` command: 5000 euros, 7-day cooldown via `last_respec_at`. Auto-migrated chars use the existing free `!setup` path.
- Subclass-L5 placeholder line added to level-up DM when crossing L5.

### Round 2 — Phase 5 (skill checks + NPC wiring)
- 10 skills (Athletics, Acrobatics, Stealth, Arcana, Investigation, Perception, Insight, Persuasion, Intimidation, Deception) with correct stat mapping.
- DC table constants matching D&D 5e (Trivial 5 → Impossible 30).
- `performSkillCheck`: d20 + ability modifier + race bonus, with nat 20/1 auto-results.
- Race skill bonuses: Half-Elf +1 to all skills, Tiefling +2 on CHA-based.
- `!check <skill> [dc]` command.
- NPC wiring: passing Insight DC 12 refunds Misty donation; passing Arcana DC 14 refunds Arina investment; passing Persuasion DC 15 grants 10% session shop discount. Silent upside for opted-in players, no UX change for legacy.

### Round 3 — Phase 4 (equipment slot mapping + rarity)
- Read-time slot mapping: legacy 5 slots → D&D 10 slots (`mapLegacySlot`).
- Rarity inference from tier + masterwork + arena tier (`inferRarity`): Common → Legendary.
- `!sheet` renders equipment with rarity icons in D&D slot order.
- Full `!equip`/`!unequip`/`!attune` deferred — nothing to populate the new slots yet (legs, hands, rings, amulet) until a future loot pass.

### Round 4 — Phase 6 (rest system)
- `!rest short`: 1h cooldown, 1d6+CON HP recovery (doubled at L5+ per design doc), no daily-action cost.
- `!rest long`: 24h cooldown, full HP, requires housing (any tier) or pays Thom Krooke inn (200 euros).
- New columns `last_short_rest_at`, `last_long_rest_at` on `dnd_character`.
- Long rest also refreshes all class resources via `refreshAllResources`.

### Round 5 — Phase 7 partial (bestiary)
- Starter bestiary in `dnd_bestiary.go`: Goblin, Skeleton, Orc Grunt, Troll, Wyvern, Ancient Dragon. Each with HP, AC, attack stats, optional ability, XP value.
- `toCombatStats()` helper converts a template to the engine's stat shape.
- `dndBestiaryByCR(maxCR)` for procedural CR-appropriate selection.
- **Rival RPS conversion deliberately deferred** — the existing rival mini-game is narratively rich and works; the user confirmed leaving it alone.

### Round 6 — active abilities + resources
- Pre-arm model (combat is one-shot, so `!use mid-fight` doesn't fit): `!arm <ability>` consumes a resource immediately and primes the ability for the next combat. `applyArmedAbility` fires it via existing `CombatModifiers` fields and clears the flag.
- Three starter active abilities: Fighter Second Wind, Mage Magic Missile, Cleric Healing Word. Rogue Sneak Attack and Ranger Hunter's Mark stay passive.
- Resource pools per class (`stamina`, `focus`, `spell_slot`, `favor`) initialized at setup confirm and on auto-migration. Refreshed on long rest.

### "Plug the holes" — quality-of-life additions
- **`!roll <dice>`** with full NdN+M notation (`2d6+3`, `d20`, `4d6-1`). Caps at 100 dice × 1000 sides. Single-d20 highlights nat 20 / nat 1.
- **`!stats`** — quick ability scores + modifiers + proficiency + AC view.
- **`!level`** — level + XP progress with percent-to-next.
- **Combat HP scaling**: wounded D&D players (sheet HP < max) fight with proportionally reduced legacy combat MaxHP, floor 25%. The rest system finally has gameplay teeth.
- **Roll summary line**: every fight ends with `🎲 d20 — N/M hit (X crits, Y fumbles). Best: nat 20!` injected into arena and dungeon outcome messages without touching protected flavor files.

### Onboarding DM
- One-shot welcome DM for legacy players (`combat_level >= 2`) on first auto-migration. Includes their actual level mapping (`Your previous level X is now D&D level Y`).
- Triggered via `freshMigrate` bool returned from `ensureDnDCharacterForCombat`; only fires the very first time a row is created.
- The full text the user wrote is in `dndOnboardingText` — TwinBee, Pinkerton agent, divide-by-five, the works.

---

## 3. Audit + remediation

After all features shipped, four parallel agents audited the D&D layer:

1. **Security & player-exploitability**
2. **Concurrency & data integrity**
3. **Code quality & correctness**
4. **Test coverage gaps**

Findings consolidated and de-duplicated into 9 actionable items (A through I). All 9 fixed.

| ID | Issue | Fix |
|---|---|---|
| **A** | No per-user lock around D&D handlers. Read-modify-write on `dnd_character` could race. | Acquired existing `advUserLock` at entry of every top-level D&D mutating command. Internal helpers (called from already-locked combat paths) intentionally stay lock-free. |
| **B** | `!respec` debited euros before saving the wipe. Save failure → euros lost, character intact. | Reordered: balance pre-check → save → resource wipe → debit. Save failure leaves all state intact; debit failure (rare race) grants a free respec rather than a destructive debit. |
| **C** | `!arm` spent resource before saving the armed flag. Save failure → resource wasted. | Reordered: save first → spend. On spend failure, revert armed flag and surface error. |
| **D** | Orc Rage threshold used `HP < MaxHP/2` integer division — drifted on odd MaxHP. | Replaced with `HP*2 < MaxHP`. Exact 50% threshold regardless of parity. |
| **E** | Respec didn't wipe old class's resource pool (Fighter→Mage left stale `stamina` rows). | `DELETE FROM dnd_resources WHERE user_id = ?` in the respec wipe path before the new class initializes via `initResources`. |
| **F** | Onboarding DM re-fired if a player canceled `!setup` mid-draft and then ran combat. | New `onboarding_sent` column on `dnd_character`, set on first successful send, never reset (survives respec, draft cancel, anything). |
| **G** | Persuasion shop discount session had no expiry. Player could chain across all purchases. | 30-minute TTL from session start (`dndPersuasionDiscountTTL`). Both pricing and announcement honor the expiry. |
| **H** | NPC refund hooks (Misty Insight / Arina Arcana) did separate Load + skill check + Credit. Architecturally fragile. | Covered by Fix A — encounter handlers already hold `advUserLock`, so the refund's `LoadDnDCharacter` is safe inside that lock. No code change needed. |
| **I** | `applyDnDHPScaling` lacked a post-scale `min(scaled, MaxHP)` clamp. Currently safe but fragile. | Added the bounds check defensively. |

10 new audit-fix tests added; all pass.

---

## 4. New files (production)

```
internal/plugin/dnd.go                  — types, repository, stat math
internal/plugin/dnd_setup.go            — !setup flow, !respec
internal/plugin/dnd_sheet.go            — !sheet, !abilities renderer
internal/plugin/dnd_combat.go           — auto-migration, HP persistence, HP scaling, roll summary
internal/plugin/dnd_passives.go         — class + race passives
internal/plugin/dnd_xp.go               — XP curve, grant, level-up DM
internal/plugin/dnd_skills.go           — skill checks, !check, NPC refund hooks
internal/plugin/dnd_equipment.go        — slot mapping, rarity inference
internal/plugin/dnd_rest.go             — !rest short / long
internal/plugin/dnd_abilities.go        — pre-arm model, resources
internal/plugin/dnd_bestiary.go         — starter monster templates
internal/plugin/dnd_misc_cmds.go        — !roll, !stats, !level
internal/plugin/dnd_onboarding.go       — legacy-player welcome DM
```

13 new production files, all additive.

## 5. Modified files

```
internal/db/db.go                      — schema additions, snapshot logic
internal/plugin/combat_engine.go       — d20 hit resolution (legacy path replaced),
                                         race-passive fields and triggers
internal/plugin/combat_bridge.go       — auto-migrate hook, D&D layer wiring,
                                         class+race passive application,
                                         armed-ability firing, HP persistence,
                                         XP grant, roll summary
internal/plugin/adventure.go           — D&D command routing in OnMessage,
                                         registry entries in Commands(),
                                         roll summary appended to dungeon output
internal/plugin/adventure_arena.go     — roll summary appended to win/death output
internal/plugin/adventure_character.go — combat_level freeze for D&D players
internal/plugin/adventure_npcs.go      — Misty/Arina skill-check refund hooks
internal/plugin/adventure_shop.go      — Persuasion discount session,
                                         price-factor applied at every Debit site,
                                         30-minute TTL
internal/plugin/plugin.go              — Base.SendDM made nil-Client-safe (test support)
```

## 6. Test files

```
internal/db/proddb_integration_test.go         — full migration on real prod DB copy
internal/plugin/dnd_test.go                    — types, math, parsing
internal/plugin/dnd_combat_test.go             — d20 path, monster layers, hit-rate stats
internal/plugin/dnd_round1_test.go             — race passives, combat_level freeze, respec format
internal/plugin/dnd_skills_test.go             — DC table, skill check math
internal/plugin/dnd_equipment_test.go          — slot mapping, rarity inference
internal/plugin/dnd_rest_test.go               — short/long rest mechanics, housing gating
internal/plugin/dnd_abilities_test.go          — resource pool, arm/spend, applyArmed
internal/plugin/dnd_bestiary_test.go           — starter monsters well-formed
internal/plugin/dnd_xp_test.go                 — curve, grant, level-up cascade, L20 cap
internal/plugin/dnd_onboarding_test.go         — message format, threshold, no-panic
internal/plugin/dnd_proddb_integration_test.go — real prod players auto-migrate cleanly
internal/plugin/dnd_plugs_test.go              — HP scaling, roll summary, dice parser
internal/plugin/dnd_audit_fixes_test.go        — all 9 audit findings covered
```

**Total: 90 D&D-layer tests** + 1 prod-DB-migration integration test in `internal/db`.

## 7. Schema changes

All additive. No drops, no renames.

### New tables
```sql
dnd_character             -- per-player D&D state (race, class, stats, HP, XP, flags)
dnd_abilities             -- per-ability uses-remaining (reserved for Phase 7+)
dnd_resources             -- per-class resource pools (stamina/focus/mana/favor)
dnd_combat_state          -- active encounter state (reserved)
adventure_characters_pre_dnd  -- one-shot rollback snapshot
```

### New columns on `adventure_equipment` / `adventure_inventory`
- `dnd_rarity TEXT NOT NULL DEFAULT ''`
- `dnd_stat_bonus_json TEXT NOT NULL DEFAULT ''`
- `dnd_attuned INTEGER NOT NULL DEFAULT 0` (equipment only)

### New columns on `dnd_character`
Added incrementally across phases:
- `auto_migrated`, `armed_ability`, `onboarding_sent`
- `last_short_rest_at`, `last_long_rest_at`

---

## 8. New player-facing commands

| Command | What it does |
|---|---|
| `!setup` | Begin or continue D&D character creation (race / class / stats / confirm) |
| `!setup race <name>` | Pick race |
| `!setup class <name>` | Pick class |
| `!setup stats <STR> <DEX> <CON> <INT> <WIS> <CHA>` | Assign standard array {15,14,13,12,10,8} |
| `!setup confirm` | Finalize |
| `!setup cancel` | Scrap draft |
| `!sheet` | Full character sheet with equipment, treasures, legacy progress |
| `!stats` | Ability scores + modifiers + proficiency + AC |
| `!level` | Current level + XP progress |
| `!abilities` | Class + race passives + active abilities + resource pool |
| `!arm <ability>` | Pre-arm an active ability for next combat |
| `!check <skill> [dc]` | Roll a skill check |
| `!rest short` | Short rest (1h cooldown, partial heal) |
| `!rest long` | Long rest (24h cooldown, full heal, requires housing or inn) |
| `!respec` | Wipe and rebuild character (5000 euros, 7-day cooldown) |
| `!roll <dice>` | Roll arbitrary dice (NdN+M notation) |

## 9. Player-visible behavior changes

- Existing players auto-migrate to a sensible inferred sheet on their next combat — no setup required, no progress lost. They get a one-shot welcome DM with their actual level mapping.
- Combat resolves with d20 + attack bonus vs AC. Every fight ends with a roll summary line.
- Class identity comes through every fight via passives (Fighter +damage, Rogue first-hit auto-crit, Mage attack bonus, Cleric low-HP heal, Ranger +damage and attack).
- Race traits matter: Halfling rerolls one nat 1 per fight, Orc rages at <50% HP, Dwarf takes half poison damage, Half-Elf and Tiefling get skill check bonuses.
- HP carries between fights. A wounded player who skips rest fights at reduced max HP.
- Levels advance based on combat XP. Skill XP (mining/foraging/fishing) advances independently and unaffected.
- Skill checks unlock NPC bonuses: Insight at Misty refunds, Arcana at Arina refunds, Persuasion at the shop grants 10% off for 30 minutes.

## 10. Level mapping (existing prod players)

Legacy `combat_level` mapped to D&D level via `dndLevelFromCombatLevel = clamp(1, combat_level/5, 20)`:

| Player | Legacy | D&D | Auto-built |
|---|---|---|---|
| holymachina | L49 | L9 | Dwarf Fighter |
| prosolis | L28 | L5 | Human Rogue |
| quack | L24 | L4 | Dwarf Fighter |
| nonk | L20 | L4 | Human Rogue |

Verified end-to-end against a copy of the prod database in `data/gogobee.db`.

## 11. What was deliberately deferred or pushed back on

- **Rival RPS → D&D combat**: confirmed by user to leave alone. The existing mini-game is narratively rich and works.
- **Equipment expansion (`!equip`/`!unequip` for new D&D slots)**: nothing to fill rings/amulet/legs/hands until shop or loot expansion. Rendering in `!sheet` works fine via the read-time mapping.
- **Active abilities for Rogue and Ranger**: their passives are strong; no actives needed yet.
- **Subclass mechanics at L5**: placeholder DM only; full content design pending.
- **D&D-specific death mechanic**: legacy `Alive`/`DeadUntil` retained. Sheet HP at 0 means "fight at 25% on next combat, rest to recover" — graceful via Fix #2.
- **Closer 5e fidelity (weapon damage dice, saving throws, full spell lists, action economy)**: per user direction, the d20-flavored adventure feel is the intended ceiling. Approachable beats faithful for a chat bot.
- **Dead `DodgeRate`/`CritRate` field-population code**: cosmetic; not worth regression risk.
- **Per-class short-vs-long rest split for resources**: all resources currently refresh on long rest only. Cheap follow-up if desired.

## 11.5 Audit fixes (post-Phase-7)

After all features shipped, four parallel audit agents reviewed the D&D layer. Findings consolidated and de-duplicated into 9 actionable items (A-I), all addressed:

- **A** — Per-user lock on D&D handlers (reuses existing `advUserLock`). Prevents double-arm, double-respec, level-up clobbers.
- **B** — `!respec` save-then-debit ordering. Save failure no longer destroys euros.
- **C** — `!arm` save-then-spend ordering with revert on failure.
- **D** — Orc Rage threshold off-by-one (`HP*2 < MaxHP` instead of `HP < MaxHP/2`).
- **E** — Resource pool wiped on respec (Fighter→Mage no longer leaves stale `stamina` rows).
- **F** — `onboarding_sent` column for one-shot welcome DM dedup across draft cancel/respec/etc.
- **G** — Persuasion shop discount expires after 30 minutes.
- **H** — NPC refund atomicity (covered by Fix A).
- **I** — `applyDnDHPScaling` post-clamp.

10 audit-fix tests added.

## 11.6 Flavor wiring (post-audit)

Five `twinbee_*_flavor.go` files were sitting in the repo root with conflicting `package flavor` declarations — never compiled, never wired. Moved to `internal/flavor/`, fixed one missing-comma syntax error, and wired the player-visible pools that mapped to existing D&D-layer features:

| Trigger | Pool | Where it surfaces |
|---|---|---|
| Combat starts (arena/dungeon) | `flavor.CombatStart` | Prepended to first phase message |
| Player wins (T1-T4 round, dungeon) | `flavor.CombatVictory` | Italicized in finalMessage |
| Player wins arena T5 round | `flavor.BossDeath` | Same slot, boss-specific |
| Player dies | `flavor.PlayerDeath` | Appended to death-output |
| Any nat 20 | `flavor.Nat20` | Roll summary, once per fight |
| Any nat 1 (no nat 20) | `flavor.Nat1` | Same slot |
| Level-up DM | `flavor.LevelUp` | Italicized prelude after banner |
| `!rest short` | `flavor.RestShort` | Appended |
| `!rest long` (housing) | `flavor.HomeLongRest` | Appended; falls back to `RestLong` |
| Misty encounter open | `MistyGreeting` ∪ legacy | ~2× variety |
| Arina encounter open | `ArinaGreeting` ∪ legacy | Same |
| Misty refund pass/fail | `MistyInsightSuccess` / `MistySkillFail` | Italicized |
| Arina refund pass/fail | `ArinaArcanaSuccess` / `ArinaSkillFail` | Italicized |
| Onboarding DM | `flavor.ExpeditionStart` | Random prelude |

Pools deferred to legacy-system migration (harvest events, housing announcements, scheduler tickers, Pete bot) and pools without features (quests, expedition system, traps, save throws, idle/lore chatback) remain unwired pending later phases.

## 11.7 Branding rebrand: D&D → Adv 2.0 (player-facing only)

Player-visible strings rebranded from "D&D" to "Adv 2.0":
- Command descriptions in `Commands()` registry (10 commands)
- `!setup` flow headers
- `!sheet` and `!rest` error messages
- Onboarding DM literal level-mapping line
- The deliberate "Dungeons & Drag-" / "d20 System" gag preserved verbatim (it's the joke about being legally distinct)

Internal type names, function names, file names, schema, and code comments still say "D&D" — accurate technical description, no risk to durable state. Full rationale in conversation history.

## 11.8 Phase 8 — Equipment & damage math

Built per `gogobee_equipment_appendix.md` (354 lines).

**`dnd_equipment_profiles.go` (new):**
- 36 weapons from appendix §2-§3 (10 simple melee, 4 simple ranged, 18 martial melee, 4 martial ranged) with damage dice, types, properties, versatile alt-dies
- 13 armors from appendix §5 (3 light, 5 medium, 4 heavy, 1 shield) with base AC, DEX caps, STR requirements, stealth flags
- `dndClassWeaponProficiency` and `dndClassArmorProficiency` per appendix §9 affinity matrix (Rogue's restricted martial list hardcoded)
- `rollWeaponDamage(weapon, abilityMod, twoHanded)` — real dice roll honoring versatile mode and magic bonus
- `computeArmorAC(armor, shield, dexMod)` — exact appendix `ComputeAC` semantics
- Legacy gear synthesis: `synthesizeWeaponProfile` / `synthesizeArmorProfile` / `synthesizeShield` map existing `AdvEquipment` rows to D&D profiles by tier+name keyword. Masterwork promotes; tier 6+ adds magic bonus up to +3.

**`combat_engine.go`:**
- `CombatStats` extended with `Weapon`, `AbilityModForDamage`, `WeaponProficient`, `TwoHandedMode` fields. Zero values = backward-compatible.
- `resolvePlayerAttack` branches: when `Stats.Weapon != nil`, rolls weapon dice; when nil, legacy `calcDamage`. Class proficiency penalty (-4 attack) at hit-check time.

**`dnd_combat.go`:**
- New `applyDnDEquipmentLayer(stats, c, equip)` — synthesizes profiles, populates the new fields, recomputes AC. Two-handed weapons forbid shields.
- `pickWeaponAbilityMod(weapon, c)` — STR for melee non-finesse, DEX for ranged, finesse picks the higher.

**`combat_bridge.go`:**
- Both arena and dungeon paths call `applyDnDEquipmentLayer` after `applyDnDPlayerLayer`. Equipment-derived AC overrides the placeholder class-armor-floor.

**24 new tests** covering weapon registry coverage, armor appendix values, ComputeAC math (unarmored, light, medium-capped, heavy, magic bonus), weapon damage roll bounds, versatile two-handed mode, damage floor, class proficiency matrices, legacy gear synthesis (tier mapping, masterwork promotion, high-tier magic), shield name matching, ability mod selection (STR/DEX/finesse), end-to-end equipment layer.

**Player-visible balance change for prod players:**

| User | Class | Synthesized weapon | Damage roll |
|---|---|---|---|
| holymachina | Dwarf Fighter L9 | T6+ Greatsword w/ +1 magic | 2d6 + STR mod + 1 |
| prosolis | Human Rogue L5 | T3-5 Longsword or Shortbow | 1d8 + best mod |
| quack | Dwarf Fighter L4 | T3-5 weapon | dice + STR mod |
| nonk | Human Rogue L4 | T3-5 weapon | dice + best mod |

Plus AC recomputed from real armor profiles.

**Deferred to Phase 8b:** magic item slots (rings/amulet), named magic weapons with special properties (Flame Tongue's +2d6 fire, etc.), `!equip`/`!unequip`/`!attune` commands, loot rarity flavor wiring, ammunition tracking, heavy armor STR-requirement penalty.

## 12. Final state (post-Phase 8)

- Build clean across `internal/...`
- Root build clean with `-tags goolm`
- `go vet` clean across `internal/...`
- **122 D&D-layer tests + 1 prod-DB migration integration test passing**
- All 9 audit findings closed
- Zero modifications to protected `internal/flavor/twinbee_*_flavor.go` content
- Snapshot rollback table populated; rollback path documented in v1.1 §11.4
- Onboarding flow handles all four prod players cleanly with verified output (now firing on `!setup`, `!check`, `!stats`, `!level`, `!rest`, `!arm` first-touch in addition to combat auto-migration)
- Phase 8 weapon dice + armor AC active for all four prod players on their next combat

## 13. Next phase: Phase 9 — Spell System

Per the auto-loaded memory entry `project_dnd_phase_plan.md` and `gogobee_design_index.md`, Phase 9 implements `gogobee_spell_system.md`: 79 spells, slots, concentration, `!cast` command, spell save DCs. Mage/Cleric/Ranger become genuinely playable distinctly.

Architecture commitment from earlier this session: **Path C hybrid combat** — one-shot for arena/random encounters, turn-based for boss fights only. Phase 9 should preserve approachability outside boss fights; spell mechanics in non-boss combat must fit the pre-arm pattern (consistent with current `!arm` for active abilities).

### Kickoff for next session

```
Read gogobee_design_index.md and gogobee_spell_system.md cold. Then open
gogobee_dnd_session_summary.md to see what's already shipped. Plan Phase 9
(spell system) per the recommended order in the auto-loaded memory entry.
Show me a tight phase plan before writing any code — what spells, what slots,
what the !cast command flow looks like, how concentration interacts with the
existing one-shot combat (Path C hybrid is locked in). Then once I confirm
scope, build it.
```

The codebase is deploy-ready. Existing players will see the new system on their next interaction — auto-migrated, onboarded, and never blocked from gameplay regardless of setup state. Phase 9-16 each add depth in a focused session.
