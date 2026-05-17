# J2b — Post-J2a all-classes baseline (n=100)

**Date:** 2026-05-17
**Corpus:** `sim_results/baseline_j2a_v2_all10.jsonl` (15,000 rows = 10 classes × 3 levels × 5 zones × n=100). Per-cell tabular summary in `sim_results/j2b_all10_summary.txt`.
**Replaces:** `sim_results/baseline_j1_all10.jsonl` as the post-sim-fix canonical corpus. The J1 numbers measured a strawman where casters never cast (see [j2_findings.md](j2_findings.md)).

## L12 leaderboard (mean %clr across all 5 zones)

| Class    | J1 baseline | J2b   | Δ      | J1 boss% | J2b boss% | Per-zone J2b %clr (T1 / T2 / T3 / T4 / T5) |
|----------|-------------|-------|--------|----------|-----------|--------------------------------------------|
| fighter  | 79.6        | 80.0  | **+0.4** | 100.0   | 100.0     | 100 / 100 / 100 / 100 / 0                  |
| ranger   | 78.6        | 80.0  | **+1.4** | 100.0   | 100.0     | 100 / 100 / 100 / 100 / 0                  |
| paladin  | 72.2        | 78.4  | **+6.2** | 99.6    | 99.2      | 100 / 100 / 96 / 96 / 0                    |
| rogue    | 70.6        | 76.8  | **+6.2** | 99.0    | 99.0      | 100 / 100 / 90 / 94 / 0                    |
| druid    | 38.6        | 61.6  | **+23.0** | 88.4   | 88.0      | 100 / 100 / 74 / 34 / 0                    |
| mage     | 20.0        | 53.4  | **+33.4** | 77.2   | 77.6      | 100 / 99 / 49 / 19 / 0                     |
| sorcerer | 20.0        | 50.6  | **+30.6** | 70.4   | 70.0      | 100 / 98 / 47 / 8 / 0                      |
| warlock  | 21.4        | 48.2  | **+26.8** | 82.2   | 81.6      | 100 / 100 / 34 / 7 / 0                     |
| bard     | 21.6        | 40.4  | **+18.8** | 82.0    | 84.4      | 100 / 100 / 2 / 0 / 0                      |
| cleric   | 18.6        | 39.0  | **+20.4** | 61.6    | 62.4      | 96 / 90 / 9 / 0 / 0                        |

## Headline

1. **No martial regression.** Fighter/Ranger/Paladin/Rogue all moved within +0.4 to +6.2pp, indistinguishable from sweep variance. The `simMartialFirstClass` gate added after the v1 sweep saved Ranger (which had regressed -35.8pp when the picker burned L3 slots on `lightning_arrow` instead of letting the weapon-attack + Hunter's Mark + Extra Attack chassis swing).
2. **Caster cliff is dissolved.** The pre-fix 19–22% cluster spread out to 39–62%. The picker alone — no balance changes — converted the trailers to mid-band classes.
3. **Druid is now top-of-band-2 at 61.6%.** Wild Resilience's ThornLash on weapon swings plus the new cast path stacks cleanly. Druid is still defensive-first by design; the cast layer turns boss rooms from "swing-and-bleed" into "burn-and-finish".
4. **T5 dragons_lair still walls everyone (0% clear across all 10 classes).** That's the J3 problem; J2 wasn't on the hook for it, and the sim-artifact half of J3's hypothesis 1 (`autoResolveCombat` never casting/consuming) is now closed by J2a.

## What's still trailing — bard and cleric

Bard L12 manor 2%, cleric L12 manor 9% — both reach the boss (~50–84%) and die there. **Not a sim artifact**; this is the spell pool. The v1 picker only scores damage-effect spells, and these two classes have thin damage rosters in `defaultKnownSpells`:

- **Bard** damage options: `vicious_mockery` (cantrip), `shatter` L2. Everything else in the bard kit is control / heal / buff, which the picker skips.
- **Cleric** damage options: `sacred_flame` (cantrip), `guiding_bolt` L1, `spiritual_weapon` L2, `spirit_guardians` L3, `flame_strike` L5. The non-damage options (`bless`, `healing_word`, `mass_cure_wounds`, etc.) are skipped too.

Both classes' identity spells are control (`hold_person`, `hypnotic_pattern`, `command`) and heal (`cure_wounds`, `healing_word`, `mass_cure_wounds`). The picker treats those as zero-value.

Note on cleric underdark: 87% of L12 underdark runs end as `tpk` with stop `ended` — that's expedition-side starvation, not boss-fight TPK (only 13% reach boss/elite). Cleric's exploration loop is brittle for unrelated reasons (low STR/CON build, supply burn). Out of J2 scope.

## J2 DoD check

Reframed DoD (`gogobee_harvest_charges_plan.md` §6.J2, updated 2026-05-17):

- **J2a — ≥80% of boss fights produce a cast or consumable.** **MET** (100% cast rate in the verification sweep, [j2a_findings.md](j2a_findings.md)).
- **J2b — caster L12 manor mean clear ≥ druid's pre-J2a 39%.** Mage 49 ✓, sorcerer 47 ✓, warlock 34 ✗ (close), bard 2 ✗, cleric 9 ✗. **3 of 5 pass.**
- **No martial leader regresses >10pp.** **MET** (all +0.4 to +6.2pp).

Two trailers still under the floor (bard, cleric). They're now bottlenecked on **spell-pool richness for the v1 damage-only picker**, not on the sim itself. The cleanest next move:

## J2c — tried, reverted (2026-05-17)

Implemented two widening levers, ran two n=100 sweeps, and reverted both:

1. **Heal-spell preempt** (cure_wounds / healing_word / mass_cure_wounds preferred over heal consumable at low HP). Regressed **druid -10pp**: slot heals at L12 heal ~10HP vs 40HP from a tier-4 Spirit Tonic, and the picker burned slots druid needed for damage spells.
2. **Control-spell scoring at 22** (denied-enemy-turn proxy). Regressed **warlock -6.6pp** — vampiric_touch L3 (10.5 expected over multiple rounds + heal) was outscored by hypnotic_pattern L3 (single denied turn). Dropping the score to 5 brought net deltas back to ±2pp but didn't lift bard or cleric, because the level-first sort prevents low-score control spells from ever winning over a higher-level damage option (which the trailer classes don't have at higher slots).

Corpora retained for reference: `sim_results/baseline_j2c_all10.jsonl` (heal-preempt + control@22) and `sim_results/baseline_j2c_v2_all10.jsonl` (control@5 only). The picker reverted to the J2b shape (damage-effect only, level-first sort, no heal-spell preempt).

**Real lever for bard/cleric trailing.** Their `defaultKnownSpells` damage rosters cap at L2 (bard: shatter; cleric: spiritual_weapon's 1d8). The picker can't pick higher-damage spells they don't have. Lifting these classes is therefore a **prod-level change to `defaultKnownSpells`** (add a damage option at L3+ for bard, a damage option at L4+ for cleric) — not a sim-picker change. Out of scope for J2; revisit alongside class identity / Open5e integration work.

## (original) Recommended J2c (now superseded by the section above)

**Widen the picker to score control + heal spells.**

- `EffectControl` (e.g. `hold_person`, `hypnotic_pattern`, `hideous_laughter`, `command`) should score as the *enemy damage it prevents over its expected effect duration* — i.e. each prevented enemy turn ≈ one round's worth of incoming damage. For a stunner that lands and lasts 1 round on a 30-dmg/round boss, that's a 30-equivalent. Better than most damage spells.
- `EffectSpellHeal` should compete with the consume-on-low-HP branch — if the cleric has `mass_cure_wounds` ready, it should fire at the low-HP threshold instead of a `Spirit Tonic`.
- `EffectBuffSelf` (e.g. `bless`, `shield_of_faith`) lifts to-hit chance / AC. Score in opening round of a fight only; subsequent rounds it's a waste.

This is one focused change in `simPickSpell` + `simPickCombatAction`. Expected impact: bard pulls into the 30–40% manor band (control spells are the bard identity); cleric pulls similarly. Martial leaders unchanged. Whether it's needed depends on operator: J2's reframed DoD allows shipping J2 with bard/cleric trailing if the prod target is "five classes mid-band, two below". Per [feedback_difficulty_target](.claude/memory/feedback_difficulty_target.md), the explicit ask is to lift trailers — that argues for landing J2c.

## Files touched this phase

- `internal/plugin/expedition_sim.go`:
  - `SimCombatSummary.Events`, `simIncludeTrace` + `SetSimIncludeTrace` (J2 diagnostic).
  - `simPickCombatAction`, `simPickSpell`, `simMartialFirstClass`, `simHealHPThresholdPct` (J2a picker + post-baseline fix).
  - `BuildCharacter` now seeds `ensureSpellsForCharacter`.
  - `autoResolveCombat` dispatches via the picker.
- `cmd/expedition-sim/main.go`: `-trace` flag.
- Docs: `sim_results/j2_findings.md`, `sim_results/j2a_findings.md`, `sim_results/j2b_findings.md`, `sim_results/j2b_all10_summary.txt`, plan §6.J2.

No prod combat / class / spell paths changed. Everything is in the sim harness.
