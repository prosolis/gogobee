# H5-sim re-baseline — does prod short-rest close the caster gap?

**Corpus:** `baseline_h5sim_all10.jsonl` (15k rows, n=100). Summary table: `h5sim_all10_summary.txt`. **Baseline for comparison:** `baseline_j2a_v2_all10.jsonl` (J2b).

## TL;DR

**No.** Wiring short-rest into the sim between rooms gives mage L12 manor a real lift (+8pp clear rate) but does not move the bard/cleric trailers in the cells that matter:

| class  | lvl | zone             | J2b %clr | H5sim %clr |  Δ |
| ------ | --- | ---------------- | -------: | ---------: | -: |
| bard   | 12  | manor_blackspire |        2 |          1 | −1 |
| cleric | 12  | manor_blackspire |        9 |         16 | +7 |
| bard   | 12  | underdark        |        0 |          0 |  0 |
| cleric | 12  | underdark        |        0 |          0 |  0 |
| mage   | 12  | manor_blackspire |       49 |         57 | +8 |

J2c (defaultKnownSpells widening — bard damage option at L3+, cleric at L4+) is still the lever. The hypothesis that H5 alone would close it is falsified.

## Notable regressions (n=100, ~±10pp 95% CI — borderline)

| class    | lvl | zone             | J2b %clr | H5sim %clr |  Δ |
| -------- | --- | ---------------- | -------: | ---------: | -: |
| warlock  | 12  | manor_blackspire |       34 |         25 | −9 |
| sorcerer | 12  | manor_blackspire |       47 |         38 | −9 |
| cleric   |  3  | goblin_warrens   |       15 |          6 | −9 |
| ranger   |  3  | goblin_warrens   |       64 |         54 | −10 |

n=100 standard error on a 50/50 cell is ~5pp, so these are ~2σ. Could be real (the picker prefers high-tier slots and short-rest refreshes low-tier first, so a mid-zone short rest may flush the high-tier-slot budget by encouraging more L2/L3 spends), or noise. Re-run at n=300 to disambiguate before declaring this a sim defect.

## Cells that lifted

Mage L12 manor +8, cleric L12 manor +7, fighter L3 goblin +10, druid L7 forest +8, sorcerer L7 forest +4, mage L7 goblin +3. Pattern: classes that already had viable damage rosters benefited; classes whose damage rosters are thin (bard/cleric) did not.

## Verdict

- Ship H5+sim short-rest as-is — the prod fix (full-HP gate loosened) is correct independently and the sim now reflects what a competent player does.
- J2c (defaultKnownSpells widening) is still required to lift bard/cleric trailers.
- T5 dragons_lair untouched (0% across all 10 classes) — J3 territory.
- Re-baseline at n=300 on the four borderline regressions before tuning the picker.
