# J3 — T5 dragons_lair universal wall (initial trace)

**Date:** 2026-05-17
**Corpus:** `sim_results/j3_traces.jsonl` — 60 expeditions (6 representative classes × L12 × dragons_lair × n=10), per-round traces enabled. Hypothesis-sift only; not a balance sweep.
**Pre-state:** post-J2 picker (`baseline_j2a_v2_all10.jsonl` shows 0% clear across all 10 classes at L12 dragons_lair).

## Boss-room aggregates (L12 vs `boss_*` of dragons_lair)

| class   | n  | reachBoss | won | rounds | pDmg  | eDmg  | enemyHPMax | enemyHP@end | playerHPMax | playerHP@end | casts | consume |
|---------|----|-----------|-----|--------|-------|-------|------------|-------------|-------------|--------------|-------|---------|
| fighter | 10 | 10        | 0   | 8.8    | 122.5 | 207.8 | **546**    | 423.5       | 173.6       | 0            | 0     | 14      |
| ranger  | 10 | 10        | 0   | 8.8    | 133.4 | 197.1 | **546**    | 412.6       | 131.0       | 0            | 0     | 18      |
| paladin | 10 | 10        | 0   | 8.3    | 76.0  | 200.0 | **546**    | 470.0       | 169.4       | 0            | 0     | 16      |
| rogue   | 10 | 9         | 0   | 6.6    | 63.7  | 168.6 | **546**    | 482.3       | 133.2       | 0            | 0     | 11      |
| druid   | 10 | 9         | 0   | 4.8    | 122.4 | 136.9 | **546**    | 423.6       | 131.0       | 0            | 34    | 9       |
| mage    | 10 | 6         | 0   | 5.2    | 92.7  | 129.2 | **546**    | 453.3       | 112.5       | 0            | 26    | 5       |

## Diagnosis

T5 dragons_lair boss is at **546 max HP**. Solo characters (player HP 110–175) take 5–9 rounds to die, dealing 60–130 player damage in that time — they chew down ~10–25% of the boss before being TPK'd. This is **party content tuned for solo by accident**.

Per-round math: clearing 546 HP in ~10 rounds requires ~55 DPR sustained. Solo player DPR caps around 15–25 even on fighter (122/8.8 = 13.9 mean) and druid (122/4.8 = 25.5 — burst before dying). A 4-character party at the same DPR clears in ~7 rounds, which is the survivable window the boss seems tuned around.

Plan §J3 listed three hypotheses; this trace rules them out cleanly:

1. ~~`autoResolveCombat` unfair (only swings).~~ **Closed by J2a.** Druid/mage are now casting (34/26 casts in 10 fights) and still 0% clear.
2. ~~Boss HP pool just too deep / multi-phase.~~ **No multi-phase complication observed** — fights are linear monotone HP descent, the boss just doesn't have a phase mechanic exposed in the trace. The HP pool *itself* is the issue.
3. **Party scaling missing — boss is tuned for 4-character parties.** Confirmed by the consistent ~25% boss-HP-eaten-at-player-death across every class.

## Recommended J3 lever

Per `gogobee_harvest_charges_plan.md` §J3 likely-levers, the difficulty memo allows *lifting trailers without pushing monster scaling*. **Reducing boss scaling at low party counts is in-bounds.**

- **Auto-scale boss HP (and probably boss damage) by participant count.** A simple `bossHP *= (0.4 + 0.15*participants)` style ramp would put solo at 0.55× (≈ 300 HP from 546) and 4-up at full. That brings a 5–9 round solo fight in line with the boss-HP curve already validated at T3/T4 (post-J2 manor: 60%+ clear for top classes; underdark: 30–100%).
- **Alternative:** Document dragons_lair as raid content. Surface a verb-style warning in `!zone` help ("Bring friends — this dragon eats solo adventurers."). Cheaper but doesn't lift the trailer end of the solo population, which `feedback_difficulty_target` says we should.

Recommendation: **try party-scaling first** — it's the lever that both keeps the boss difficult for parties AND respects the "lift trailers" feedback. Document-only is the fallback if scaling proves intrusive.

## Open question for the operator

What's the *intended* T5 boss audience — solo or party? If solo, scaling is required. If party, dragons_lair should be flagged as raid content and not appear in solo-play recommendations. The sim has been running solo throughout, which the plan's J-phase open questions already flagged needs an operator answer.

Out of scope here: implementing the scaling lever. That's a prod change in the boss HP / damage path. Picking up that work would need the operator's call on the audience question first.
