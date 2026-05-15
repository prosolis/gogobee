# GogoBee — D&D Class Balance Pass

> **Companion to:** `gogobee_dnd_design_doc_v1.1.md`, `gogobee_spell_system.md`, `gogobee_subclass_system.md`
> **Version:** 0.1 (planning draft)
> **Status:** Pre-implementation — Phase 0 spike in progress
> **Sibling work:** the race-balance pass (`internal/plugin/dnd_race_balance.go`, `TestRaceBalance`) — same spirit, different method (see §1).

---

## 0. Why this doc exists

The race-balance pass tuned the seven races to equal *effective power* via a
hand-weighted scoring model. That model is a **proxy** — races don't fight, so
there is nothing to measure directly.

Classes are different: combat is **one-shot auto-resolved** through a single
`simulateCombatWithRNG` call (`combat_engine.go:338`), and the RNG is
**seedable**. So class balance does not need a proxy model — we can run
thousands of real, reproducible fights and read the win rates directly.

This doc plans that work. Endgame: a `TestClassBalance` regression fixture that
holds per-tier win-rate parity across all 10 classes × 30 subclasses, plus the
tuning pass that gets us there.

**Non-goals.** Not a combat-engine redesign. Not adding classes/subclasses.
"Balanced" means balanced *in gogobee's one-shot engine* — the model collapses
tabletop action economy, and that is accepted.

---

## 1. Method — measure, don't model

| | Race pass | Class pass |
|---|---|---|
| Power source | ability mods only | full combat: HP/AC, weapons, spells, passives, subclass tiers |
| Approach | hand-weighted scoring proxy | Monte Carlo over the real engine |
| Trust | heuristic weights (estimated) | empirical win rates (measured) |
| Artifact | `dnd_race_balance.go` + `TestRaceBalance` | `dnd_class_balance.go` + `TestClassBalance` |

---

## 2. The matrix (full)

For each of the 10 classes:
- `subclass = none` at L1–L4
- each of its 5 subclasses at the L5 / L7 / L10 / L15 / L20 tier-unlock checkpoints

→ ~60 build profiles × level checkpoints × ~6 monster tiers. Each cell = N
seeded fights through `simulateCombatWithRNG`. One-shot sims are cheap; compute
is not the constraint.

**Opponents:** the existing per-tier monster scaling curves
(`dnd_combat.go:35–51`) — dungeon Tier and arena ThreatLevel — used as a fixed
gauntlet.

**Builds:** standard array assigned by each class's stat priority
(`dnd_combat.go:185–218`), Human race (neutral baseline, +1 all), plus a
standardized equipment loadout (see §3).

---

## 3. The two hard parts (Phase 0 de-risks these)

1. **Equipment loadout policy.** Combat uses equipment-driven AC and real weapon
   dice. The per-class, per-level kit must be standardized *fairly* or every
   downstream number is garbage. Derive from existing starting-gear / level
   tables if they exist; otherwise define a canonical tiered kit.
2. **Spell-selection policy.** Caster damage flows through `applyPendingCast`.
   Without a "what would a player cast here" heuristic per class/level, all 8
   casters fight as naked weapon-users and read as broken — a measurement
   artifact, not real imbalance.

---

## 4. Balance rule

Per-tier **win-rate parity**: at each difficulty tier, every class within a
tolerance band of the cross-class mean win rate. Band set empirically after the
first full run (the race pass's ±0.5 was set the same way). Damage-dealt,
HP-remaining, and near-death-rate are logged as diagnostics, not asserted.

---

## 5. Phases (one phase ≈ one session; each ends green + committed)

- **Phase 0 — spike.** Harness skeleton; equipment + spell-selection policies;
  run *Fighter vs. Mage only* across tiers and sanity-check plausibility
  (both win something; casters not at 0%). If the numbers are implausible, fix
  the policies before trusting anything. **Done — commit 0878b4e.**
- **Phase 1 — harness + matrix.** Generalize to all 10 classes × 30 subclasses;
  `TestClassBalance` logs the full report. No tuning yet — just measurement.
  **Done.** Subclass field plumbed through the harness, `applySubclassPassives`
  wired in to match live combat order, `buildPhase1Profiles` produces 190 rows
  (10 × 4 pre-subclass + 10 × 3 × 5 post-subclass), `TestClassBalance_Phase1_FullMatrix`
  logs the cells plus per-class tier means and ranges. Phase-2 calibration baseline:
  at T4 the cross-class spread of *mean* win rate runs Bard 0.62 → Fighter 0.80
  (~18pp); at T5 0.48 → 0.64 (~16pp); casters trail martials at the post-unlock
  tier (T3) by ~20pp.
- **Phase 2 — tuning pass. Done.** First insight: per-class-mean is dominated
  by floor+ceiling saturation (L10+ pinned at 1.0; L1-4 caster cells at high
  tier pinned at 0.0), masking the actual gaps. The matrix log now also
  emits a per-(level, tier) cross-class spread — the cells with real signal.
  Tuned levers, in priority order:
  - **Class passives** (`dnd_passives.go`): caster trailers (Bard, Mage,
    Warlock, Sorcerer) gained level + casting-stat scaled FlatDmgStart bursts
    so their L1-4 chassis isn't a quarterstaff + one weak spell against a T2-T3
    monster; small DamageBonus riders (Mage/Bard/Sorcerer/Rogue +5%, Warlock
    10→12%) and +1 attack for Bard/Warlock close the steady-DPS gap. Added a
    `clampNonNeg` helper so ability-mod-scaled additions never go negative on
    sheets with sub-10 casting stats.
  - **Subclass L5 tiers** (`dnd_subclass_combat.go`): the three Sorcerer
    L5 picks (Wild/Storm/Draconic) and Warlock Great Old One were defense-only
    or near-inert pre-tune; each gained a DamageBonus +0.10 (or FlatDmgStart
    burst for Storm) so the L5 chassis can press through a T4 monster.

  **Parity band locked** in `TestClassBalance_Phase1_FullMatrix`: cross-class
  spread ≤ 35pp on the *in-tier* diagonal — the (level, tier) cells where the
  level is appropriate for the tier (T1: L1-4, T2: L3-7, T3: L5-10, T4: L7-15,
  T5: L10-20). Off-tier cells (e.g. L1 mage at T3 dungeon) are still logged but
  not asserted: those are level-vs-tier mismatches, and casters at L1-4 can't
  muscle through a T3 monster on a single L1-slot spell the way martials muscle
  through with weapon dice. Observed worst in-tier cell after tuning: L3/T2 at
  ~26pp, L1/T1 at ~24pp, L5/T3 at ~20pp; the 35pp band gives ~9pp Monte-Carlo
  headroom at 200 trials/cell. **← current**
- **Phase 3 (if needed) — second-order.** Subclass-vs-subclass spread within a
  class; the Paladin/Rogue MAD question if the data shows it bites. The L7/T5
  cell (47pp spread, *off-tier*) is the most obvious next target if the
  underleveled-cell experience needs lifting.

## 6. Tuning levers (priority order)

1. Class passives — `dnd_passives.go`
2. Subclass L5/7/10/15 tiers — `dnd_subclass_combat.go`
3. Spell damage dice & upcast — `dnd_spells_data.go`, `dnd_spells_srd_data.go`
4. Spell slot progression — `dnd_spells.go`
5. AC floor / armor proficiency — `dnd.go:164–178`
6. Attack bonus class matrix — `dnd_combat.go:22–33`
7. Monster scaling curves — `dnd_combat.go:35–51` (last resort; affects PvE feel)
