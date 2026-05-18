# H4 — Charge calibration findings

**Date:** 2026-05-17
**Corpus:** `h4_calibration_v1_all10.jsonl` (15k rows, 10 classes × L3/7/12 × 5 zones × n=100). Summary: `h4_calibration_v1_summary.txt`.
**Baseline for comparison:** `baseline_j2a_v2_all10.jsonl` (post-J2b picker, current shipped state).
**Change under test:** every `RarityUncommon` and `RarityRare` resource bumped from `MaxCharges: 1` → `MaxCharges: 2` in `dnd_resource_registry.go` (53 resources). Commons (already mostly 2), VeryRare/Legendary (trophy tier) untouched.

## TL;DR

**Charge counts are not the binding constraint on yield.** The bump from 1 → 2 charges on every Uncommon and Rare resource produces a tiny net change in cleared-run yield, ranging from −2.5% (forest) to +4.7% (goblin). The H4 DoD (±10% yield parity) is satisfied trivially because the lever has almost no leverage.

| zone             | cleared mean yield (J2b) | cleared mean yield (H4) |   Δ% |
| ---------------- | -----------------------: | ----------------------: | ---: |
| goblin_warrens   |                     5.99 |                    6.27 | +4.7 |
| forest_shadows   |                     8.17 |                    7.97 | −2.5 |
| manor_blackspire |                     5.50 |                    5.37 | −2.4 |
| underdark        |                     6.30 |                    6.16 | −2.2 |
| dragons_lair     |                        — |                       — |    — |

All deltas are within n=100 noise (~±5% on these means).

## Why charge counts don't move the needle

Two compounding mechanics cap per-room yield well below "all charges spent":

1. **Standard/Elite/Patrol interrupts abort the entire room harvest pass.** `autoHarvestRoom` returns immediately on any non-Noise interrupt (`internal/plugin/dnd_expedition_autopilot_harvest.go:160-175`). Bumping charges *adds more interrupt rolls per room*, which slightly *increases* the chance the room ends before higher-rarity actions even fire — the Forage action runs first, so its extra charges can squander the room budget before Fish/Essence/Commune get a swing.
2. **The Common consolation bracket** (`resolveHarvestOutcome`, delta -1 to -4) already softens the Josie nerf: even a "failed" roll within 4 below DC yields +1. P(Nothing) is only 0-50% across the realistic DC×modifier range, so the Josie penalty per charge was ~25-50%, not 100%.

Net effect: doubling charges roughly doubles the *potential* yield ceiling per resource, but the per-room interrupt-truncation budget is fixed. Yield reshuffles between resources within a room (Fey Bloom +55% in forest, Darkwood −36%, Biolum Fungi −35% — see `mean_yields_by_name` slice) without changing the per-run total.

## What this means for the H4 DoD

The plan's DoD reads:

> Monte Carlo shows yield parity within ±10% across tiers; rare nodes feel rare (success rate visibly < 50%) but not punishing across a full expedition.

Both legs are met:
- **Parity:** all four sampled zones land within ±5% of the J2b baseline.
- **Rare-feel:** charge bumps don't change DCs; success rate per attempt is unchanged. A 1→2 MC bump only means "two swings instead of one" on the same DC, so rares still take two attempts on average to land.

The implicit goal — restore pre-Josie yields — is **not** met. We don't have a pre-Josie sim corpus to compare against (H1 shipped before the sim was built). Analytically, pre-Josie yields would be ~1.3× (low DC) to 2× (high DC) the post-Josie per-charge expectation. H4 closes maybe 5-30% of that gap depending on zone, because of the interrupt cap.

## Recommendation

**Ship the bump as-is.** It's a narrative/feel win (more swings, longer node depletion arcs) and a small yield lift in goblin_warrens — the entry zone. The cost is zero (registry-only change, no code touched, all tests green).

**Do not chase the rest of the gap by tuning interrupts.** The difficulty memo (`feedback_difficulty_target`) says lift trailers without pushing monster scaling, and harvest-interrupt frequency is part of the per-zone threat economy. Reducing interrupt rate would lift yield universally but also weaken the threat-loop feedback (`§4.2 Combat Interrupt`). Out of scope for this branch.

**Document the finding** so a future tuner doesn't reach for the charge knob again expecting big swings.

## Files

- `dnd_resource_registry.go` — 53 resources bumped.
- `h4_calibration_v1_all10.jsonl` / `.err` — sim corpus.
- `h4_calibration_v1_summary.txt` — per-cell summary.

## Open items rolled forward

- **Interrupt-frequency tune** (not H4): if harvest yields ever need to lift another 20-30%, the right lever is `resolveCombatInterrupt` rates in `dnd_expedition_harvest.go`, not charge counts. Park as a future tuning option.
- **Pre-Josie baseline corpus** (not needed): a one-shot git revert of H1 + sim run would give the true target. Skipping unless someone disputes the "interrupt-cap is the real ceiling" finding.
