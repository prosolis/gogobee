# J2a — autoResolveCombat cast+consume picker (verification sweep)

**Date:** 2026-05-17
**Corpus:** `sim_results/j2a_traces.jsonl` — 240 runs (6 classes × {manor_blackspire, underdark} × L12 × n=20)
**Pre-state corpus:** `sim_results/j2_traces.jsonl` (same shape, picker disabled). See [j2_findings.md](j2_findings.md) for the strawman diagnosis that motivated this fix.

## What J2a shipped

- `internal/plugin/expedition_sim.go`:
  - New `simPickCombatAction(uid, sess)` decision tree:
    1. HP < 40% MaxHP + heal consumable in inventory → `!consume <heal>`
    2. Caster with a usable damage spell (slot or cantrip) → `!cast <spell>`
    3. Otherwise → `!attack`
  - New `simPickSpell(c, uid)` ranks prepared damage-effect spells by slot level (descending) then expected damage from the dice string. Reaction-cast spells excluded; non-cantrips require an available slot at their native level (no upcasting in v1).
  - `BuildCharacter` now calls `ensureSpellsForCharacter` so the synthetic spellbook is populated (prod players hit this on character creation; the sim path skipped it).
  - `autoResolveCombat` dispatches via the picker instead of always swinging.

Built on top of the per-round `-trace` plumbing landed earlier in J2 (`SimCombatSummary.Events`, `SetSimIncludeTrace`).

## Verification (n=20 per cell, L12 only — same cells as j2_findings)

| class    | zone             | n  | reachBoss | won | lost | rnd  | pDmg  | eDmg  | slotRuns | casts | consumeRuns | eHP@loss |
|----------|------------------|----|-----------|-----|------|------|-------|-------|----------|-------|-------------|----------|
| mage     | manor_blackspire | 20 | 15        | **12** | 3 | 7.2  | 133.0 | 74.3  | 15 / 15  | 93    | 12          | 83.3     |
| mage     | underdark        | 20 | 8         | 3   | 5    | 7.0  | 143.8 | 78.6  | 8 / 8    | 54    | 2           | 34.2     |
| cleric   | manor_blackspire | 20 | 6         | 2   | 4    | 11.3 | 101.3 | 146.2 | 6 / 6    | 58    | 5           | 64.8     |
| cleric   | underdark        | 20 | 4         | 0   | 4    | 7.0  | 74.5  | 89.5  | 4 / 4    | 28    | 0           | 87.5     |
| sorcerer | manor_blackspire | 20 | 13        | **9** | 4 | 8.0  | 130.9 | 87.3  | 13 / 13  | 91    | 8           | 53.2     |
| sorcerer | underdark        | 20 | 7         | 1   | 6    | 5.9  | 89.6  | 67.4  | 7 / 7    | 40    | 1           | 86.2     |
| warlock  | manor_blackspire | 20 | 12        | 6   | 6    | 13.2 | 131.0 | 140.6 | 12 / 12  | 141   | 9           | 45.5     |
| warlock  | underdark        | 20 | 15        | 0   | 15   | 6.8  | 74.3  | 82.5  | 15 / 15  | 101   | 1           | 87.7     |
| bard     | manor_blackspire | 20 | 12        | 0   | 12   | 10.6 | 68.1  | 128.2 | 12 / 12  | 117   | 6           | 75.9     |
| bard     | underdark        | 20 | 12        | 0   | 12   | 11.2 | 74.3  | 108.8 | 12 / 12  | 132   | 2           | 87.7     |
| druid    | manor_blackspire | 20 | 15        | **14** | 1 | 6.9 | 142.6 | 84.5  | 15 / 15  | 87    | 11          | 79.0     |
| druid    | underdark        | 20 | 18        | 3   | 15   | 5.1  | 123.3 | 82.3  | 18 / 18  | 85    | 5           | 47.5     |

`slotRuns` = boss fights where ≥1 spell_cast fired. `casts` = total spell_cast events. `consumeRuns` = fights with ≥1 use_consumable. `eHP@loss` = mean enemy HP remaining when player lost.

## DoD check

- **J2a DoD: ≥80% of boss fights produce a cast or consumable use.** **MET** — every cell shows `slotRuns == reachBoss` (100% cast rate).
- Manor mean clear (5 trailer casters): mage 60%, sorcerer 45%, warlock 30%, bard 0%, cleric 10%. Mage and sorcerer clear the ≥40% J2 target on the strength of the sim fix alone; warlock/cleric are close, bard is the one outlier still at zero.
- Druid jumped 0% → 70% manor / 0% → 15% underdark. Pre-J2a it was the lone mid-band class on weapon-attack alone; with cast on top, druid pulls into the top tier.

## What's still broken and why

- **Bard 0% clear despite firing 117 casts/cell.** Bard's prepared damage spell registry is thin: `vicious_mockery` cantrip (4d4 at L11) and `shatter` L2 (3d8) are the only damage-effect options in `defaultKnownSpells`. Every other bard spell is control/buff/heal, which the v1 picker skips. Bard's average pDmg 68 in 11-round fights = 6/round — that's accurate for the spell pool, not a picker bug. Fix candidates: (a) extend bard's default known-spell list with a damage L3+; (b) widen picker to score `EffectControl` (hold_person, hypnotic_pattern) which trivializes enemy turns and effectively converts to damage by buying free player rounds; (c) include healing spells in the consume-priority path. **(b)** has the highest expected ROI and lifts cleric and warlock too.
- **Cleric still bottom-of-table.** 10% manor / 0% underdark. Cleric's damage-effect spells: `sacred_flame` cantrip (4d8 at L11) — that's it. Same shape problem as bard. `spiritual_weapon`, `bless`, `guiding_bolt`, `cure_wounds` aren't reachable by the v1 picker. Fix: same as bard — score control spells and recognise heal spells as alternatives to consumables (cleric has 40-HP healing word at low slot cost).
- **Warlock underdark 0%.** Different shape: high cast rate (101 casts/15 fights = 7/round), high player damage (74), but loses anyway because enemy damage averaged 82.5 and warlock has no defensive lever. Underdark boss is dropping warlock before warlock can drop it. This is a class-balance problem the post-J2b corpus needs to confirm before we move on.

## Implications for J2b (the big re-baseline)

A full n=100 all-classes baseline (`sim_results/baseline_j2a_all10.jsonl`) is kicked off; that replaces `baseline_j1_all10.jsonl` as the canonical post-fix corpus. Expected shape:

- Mage / sorcerer / druid pull into mid-band (40–70% L12 clear range).
- Bard / cleric stay at bottom, but the *reason* is a thin damage-spell pool, not a sim artifact.
- Warlock somewhere middle; partial-clear depending on zone.
- Martials should be unchanged (picker is a no-op for non-casters with no low-HP heal trigger).

If that picture holds, the right J2 follow-up is **picker widening** (control + heal scoring) rather than per-class balance riders. Cleaner than five class buffs.

## Files touched

- `internal/plugin/expedition_sim.go` — picker, BuildCharacter spell-seed, dispatch in autoResolveCombat.
- `cmd/expedition-sim/main.go` — already had `-trace` from earlier in J2.

No prod paths changed. The fix is sim-scoped.
