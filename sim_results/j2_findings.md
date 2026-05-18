# J2 — Caster boss-survival traces (per-round event sweep)

**Date:** 2026-05-17
**Corpus:** `sim_results/j2_traces.jsonl` — 240 expeditions
**Cells:** {mage, cleric, sorcerer, warlock, bard, druid} × {manor_blackspire, underdark} × L12 × n=20
**Method:** New `-trace` flag on `cmd/expedition-sim` attaches the raw `CombatEvent` stream to the last combat (the boss/elite room) of every expedition. Aggregated in `/tmp/analyze_j2.py`.

## Headline

The caster cliff in `baseline_j1_all10.jsonl` is **not a class-balance problem. It is a sim-artifact**: `autoResolveCombat` dispatches `!attack` only — never `!cast`, never `!consume`. Across 240 boss-room fights:

| Event type                    | Count |
|-------------------------------|------:|
| `player` `spell_cast`         | **0** |
| `player` `use_consumable`     | **0** |
| `consumable` events           | **0** |
| `player` `hit` (weapon swing) |   381 |
| `player` `thorn_lash` (druid passive) | 261 |

Every "caster boss-fight" the matrix has scored to date is a melee swing-fest with the caster's spell slots untouched and their consumables sitting in inventory.

## Class-level per-cell boss-fight rollup (L12)

| class    | zone             | n  | reachBoss | won | lost | rnd | pDmg | eDmg | slotRuns | casts | noSlot | eHPleft |
|----------|------------------|----|-----------|-----|------|-----|------|------|----------|-------|--------|---------|
| mage     | manor_blackspire | 20 | 10        | 0   | 10   | 7.4 | 27.4 | 89   | **0**    | 0     | 10     | 116.6   |
| mage     | underdark        | 20 | 13        | 0   | 13   | 5.4 | 28.7 | 68.1 | **0**    | 0     | 13     | 133.3   |
| cleric   | manor_blackspire | 20 | 7         | 0   | 7    | 7.6 | 28.1 | 101  | **0**    | 0     | 7      | 115.9   |
| cleric   | underdark        | 20 | 1         | 0   | 1    | 2   | 12   | 26   | **0**    | 0     | 1      | 150     |
| sorcerer | manor_blackspire | 20 | 11        | 0   | 11   | 6.8 | 16.9 | 86.2 | **0**    | 0     | 11     | 127.1   |
| sorcerer | underdark        | 20 | 6         | 0   | 6    | 6.5 | 26.8 | 79   | **0**    | 0     | 6      | 135.2   |
| warlock  | manor_blackspire | 20 | 10        | 0   | 10   | 8.8 | 29.9 | 108.4| **0**    | 0     | 10     | 114.1   |
| warlock  | underdark        | 20 | 15        | 0   | 15   | 8.6 | 53.4 | 96.1 | **0**    | 0     | 15     | 108.6   |
| bard     | manor_blackspire | 20 | 14        | 0   | 14   | 9.1 | 29.9 | 103.8| **0**    | 0     | 14     | 114.1   |
| bard     | underdark        | 20 | 10        | 0   | 10   | 7.8 | 46.9 | 76.5 | **0**    | 0     | 10     | 115.1   |
| *druid*  | manor_blackspire | 20 | 15        | 0   | 15   | 8.1 | 77.8 | 107.5| **0**    | 0     | 15     | 66.2    |
| *druid*  | underdark        | 20 | 14        | 0   | 14   | 6.5 | 72.3 | 81.1 | **0**    | 0     | 14     | 89.7    |

Columns: `slotRuns` = boss fights where ≥1 slot spell fired; `casts` = total slot casts; `noSlot` = boss fights with zero casts; `eHPleft` = mean enemy HP remaining at player-loss (the "how close did we come" number).

## Why druid is the lone outlier at 39% mean clear

Druid's `ThornLashDmg = 2 + level/4` passive (`dnd_passives.go:210`, Class-identity audit 2026-05-16) fires every time the enemy lands a hit on the player — 5 reactive damage at L12, ×~10–15 enemy hits per fight = 50–75 incidental player damage. That alone accounts for the ~70+ mean `pDmg` druid posts vs 17–53 for the trailers. **Druid is not "tankier than the trailers" — druid is the only class whose damage profile doesn't depend on the spell action**. Wild Shape, the actual durability beat the plan called out, is not wired through `autoResolveCombat` at all; thorn lash is.

## Reaction-spell hypothesis (plan §6.J2 H2) is moot for now

`dnd_spells.go` already documents that reaction-cast spells (`shield`, `counterspell`, `hellish_rebuke`) are explicitly skipped today — no reaction window in combat. So even if `autoResolveCombat` were upgraded, Shield-style auto-fires aren't a J2 lever until a reaction phase exists.

## Conclusion — J2 is hypothesis-3-shaped but the cause is upstream

The plan listed four hypotheses; the trace evidence speaks to them as follows:

1. ~~Damage falls off vs boss HP~~ — confounded: cantrip damage was never tested; player only swung weapons.
2. ~~No durability backstop~~ — Druid's outperformance is offense (thorn lash), not durability. So the durability gap, if it exists, can't be read from this corpus.
3. **Slot/resource economy** — slots aren't being spent in boss fights at all. Confirms casters reach boss "with slots intact" (the plan's optimistic read), but in the worst possible way: the sim refuses to spend them.
4. Cleric identity — `simAutoArmDefaultFor` does pre-arm Healing Word for Cleric (`dnd_abilities.go:375`), and that fires on the armed-ability path before combat. But mid-fight casts of Spiritual Weapon / Bless never fire.

## Recommended J2 lever (replaces the original menu)

**J2a — Teach `autoResolveCombat` to cast and consume.**
Before each `!attack`, the sim should mirror "what a competent prod player would do":

- If HP < ~40% MaxHP and a heal-tagged consumable is in inventory → dispatch `handleConsumeCmd` with that item.
- Else if the character is a spellcaster with available slots and a known damage/control spell that lands → dispatch `handleCastCmd` (priority: damage spell that finishes the enemy → control/stun → buff → cantrip).
- Else swing.

The picker can be small and class-blind for the first cut — pick the highest-level slot whose damage roll most efficiently reduces remaining enemy HP, fall back to a cantrip when slots are dry. Mirror `combat_bridge.go`'s `SelectConsumables` for the heal side.

**J2b — Re-run baseline matrix with the fixed `autoResolveCombat`.**
The current n=100 baseline measures a strawman. Once J2a lands we get a real read on caster boss-survival. Only then is it worth picking class-level levers (or concluding "casters are fine, the sim was lying").

**Out of J2 scope for now:** Reaction wiring (Shield, Counterspell). Per `dnd_spells.go`'s own comments, the reaction phase doesn't exist yet; lifting that ban is its own surface.

## Side note — `autoCombatRoundCap` vs the actual constant

Plan mentions `autoCombatRoundCap = 200`; verified in `expedition_sim.go:528`. No issue, just confirming the harness isn't truncating fights early.
