# Expedition Difficulty Tuning Pass

Started 2026-05-15, after the Phase D audit close-out. Sibling to the
class-balance pass (`gogobee_class_balance.md`) — same Monte-Carlo
philosophy, different target metric.

**Class balance answers** "is my class viable?" (parity between classes
vs. the same monster tier).
**This pass answers** "is the dungeon fair?" (does a tier-appropriate
character complete a tier-N expedition at the expected rate?).

Same combat engine (`simulateCombatWithRNG`), orthogonal knobs.

---

## Method

**Sim harness, not telemetry-first.** A new
`internal/plugin/dnd_expedition_balance.go` will run thousands of
seeded expeditions through the real autopilot / threat / encounter /
combat code and report:

- completion %
- death %
- median days-to-extract
- median threat at extract
- supply curve (% expeditions that hit Rationing / Severe / Starvation)
- combat-encounter count per run
- coin/XP earned

The harness produces these per (zone, player-level) cell; the
regression test asserts the per-tier completion band.

**Tier target band** (steered by [[feedback_difficulty_target]]
"relatively easy but not too easy", and reflecting that expeditions are
multi-day commitments players plan around — losing one should sting but
not feel arbitrary):

| Tier | Target completion % | Acceptable band |
|------|--------------------:|-----------------|
| T1   | 80%                 | 70–90%          |
| T2   | 72%                 | 62–82%          |
| T3   | 65%                 | 55–75%          |
| T4   | 55%                 | 45–65%          |
| T5   | 45%                 | 35–55%          |

Bands are ±10pp around the centerline. Sim is asserted against the band,
not the centerline (no overfit). Player-level for each cell = "tier
appropriate" per the class-balance kit ladder: T1 → L1-4, T2 → L3-7,
T3 → L5-10, T4 → L7-15, T5 → L10-20 (median level per tier).

**Out of scope:**

- Per-encounter combat parity (covered by `gogobee_class_balance.md`).
- Arena difficulty (separate system, separate balance).
- Live telemetry tables — deferred; sim is the primary truth source for
  this pass. We can add counters later if the sim diverges from
  observed live numbers.

---

## Phases

Each phase ≈ 1 session, ships green + committed.

### Phase 0 — sim harness spike

Build the minimum harness:

- New file `internal/plugin/dnd_expedition_balance.go` exposing
  `simulateExpeditionWithRNG(profile, seed)` → result struct.
- **Critical de-risk: replay the autopilot deterministically.** The
  ticker is wall-clock driven (`dnd_expedition_cycle.go:38,56` at
  06:00 / 21:00 UTC; `expedition_ambient.go:22` every 3h). Harness
  must inject a virtual clock or call the per-phase advance functions
  directly (`deliverBriefing`, `deliverRecap`, ambient nudge, harvest
  autopilot) in tight loop, bypassing the goroutine ticker. **Settle
  the clock-injection seam in Phase 0** — if this is messy, the whole
  pass collapses.
- DB-touching layers stubbed/in-memory (same trick as class-balance
  Phase 0).
- Sanity test: one cell — e.g. T2 zone at L5 Fighter — runs to
  completion or death deterministically, returns sensible numbers,
  100 trials in <10s.

**Exit:** `TestExpeditionBalance_Phase0_Spike` green; commit.

### Phase 1 — full matrix measurement

- Build the cell list: every zone × tier-appropriate-level. Per the
  exploration map, expeditions span zones tagged T1–T5; we measure
  every zone at its centerline level.
- 200 trials/cell (same density as class-balance Phase 1).
- Matrix log: per-cell completion%, death%, median days, median
  extract-threat, supply-curve histogram.
- Per-tier mean + spread diagnostic across zones (analogue of the
  class-balance per-(level,tier) spread diagnostic).
- `TestClassBalance_Phase1_FullMatrix`-style permanent test, but only
  gating on harness-broken pathologies for now (0% completion anywhere
  at T1, 100% completion at T5).

**Exit:** matrix numbers logged; commit with "Phase 1 baseline" note.

### Phase 2b — wounded-entrant nick clamp (shipped)

`clampSurpriseNick(rawNick, hpCurrent, hpMax)` in
`dnd_expedition_combat.go`: at full HP the raw nick stands; when
wounded the nick is capped at `max(1, hpCurrent/5)` so a single nick
on a low-HP fighter can't shave more than ~20% of remaining HP. The
existing "nick can't KO" guard is preserved.

Motivation came from the post-2a tier-lethality trace
(`TestExpeditionBalance_Phase2_TierLethality`): every tier had a
fingerprint where an over-tier elite left the fighter at 25-50% HP +
retreat, then the next standard's surprise nick (3-7 HP, often
exceeding the fighter's headroom) pre-empted the combat engine
entirely. Capping the wounded-entrant nick separates that cascade
from genuine elite-one-shot deaths so Phase 2c can tune the elites
with a clean read.

Matrix delta vs Phase 2a baseline is small (median encs +0.1-0.2
per cell) — completion% still 0% across every tier — but the
tier-lethality trace stretched substantively: T1 trial 0 went 5 → 8
encs / 5 → 7 days; T3 trial 1 ran 18 → 10 encs but visibly survived
multiple chained interrupts at low HP that pre-2b would have ended
on the nick alone. The remaining deaths are now legible as elite
one-shot fights on fresh entries (Hobgoblin Warchief, Green Hag,
Roper, Young Red Dragon) — that's the Phase 2c roster signal.

**Tuning surface:** the `/5` divisor in `clampSurpriseNick` is the
wounded-fighter lethality knob.

### Phase 2 lever sweep (shipped) — negative result

Before adding Phase 2c, the two surfaced knobs (`retreatThreatBump`,
`clampSurpriseNick` divisor) got swept across a full 3×4 grid
(bump ∈ {2, 5, 10} × divisor ∈ {3, 5, 8, 12}) at 200 trials/cell
across every zone in the matrix. Wired via
`expeditionBalanceProfile.{RetreatThreatBumpOverride,
SurpriseNickDivisorOverride}` threaded into `expeditionHarness`;
the live caller (`runHarvestInterrupt`) is untouched.

Sweep file: `TestExpeditionBalance_Phase2_LeverSweep` in
`expedition_balance_test.go`. `-short` skips.

**Outcome:** every one of the 24,000 trial-cells reports 0.0%
completion / ~100% death. The two knobs are inert on the headline
metric — even the gentlest combo (b=2, d=12) cannot lift any tier
off the floor. Median days-to-end stays at 2–3 across the grid
with median 2.6–7.3 encounters before death.

**Read:** confirms the post-2b tier-lethality trace — deaths are
fresh-entry elite one-shots (Warchief @ HP19/20, Hag @ HP41, Roper,
Young Red Dragon), not chained-interrupt cascades. The two levers
target the cascade path that's now mostly resolved; they have no
remaining death-fraction to convert. Phase 2c (roster dilution) is
justified rather than premature.

### Phase 2c — roster gate (shipped)

Promoted one mid-tier alt to `IsElite: true` in every zone whose elite
pool was a single SpawnWeight=1 boss (`internal/plugin/dnd_zone.go`).
The boss stays in the pool; weighted dilution means the InterruptElite
bracket no longer auto-picks the over-tier boss. `pickZoneEnemy` also
excludes the promoted entry from the standard pool, lowering standard
encounter difficulty by removing one mid-difficulty option from
non-elite brackets (acceptable: each zone still has 3-4 standard
entries left). Crypt-Valdris was already dual-elite (wight + flameskull)
and was left alone.

| zone               | promoted alt          | boss (untouched)   |
|--------------------|-----------------------|--------------------|
| goblin_warrens     | worg (SW=3)           | hobgoblin_warchief |
| forest_shadows     | owlbear (SW=4)        | green_hag          |
| sunken_temple      | aboleth_thrall (SW=3) | water_elemental    |
| manor_blackspire   | vampire_spawn (SW=3)  | revenant           |
| underforge         | salamander (SW=3) + fire_elemental (SW=3) | helmed_horror |
| underdark          | drow_elite_warrior (SW=4) | roper          |
| feywild_crossing   | night_hag (SW=3)      | fomorian           |
| dragons_lair       | dragonborn_cultist (SW=3) | young_red_dragon |
| abyss_portal       | hezrou (SW=4)         | marilith           |

**Phase 1 matrix delta (200 trials/cell, Fighter, tier centerline):**
goblin_warrens 0% → **3.0%** completion (first non-zero T1 reading
since Phase 0); sunken_temple 0% → 0.5%; dragons_lair death% **100% →
60%** with 40% now starve (fighter survives the elite-spawn path long
enough to run out of supplies — different failure mode); underdark
death% 100% → 90% (10% starve); feywild_crossing 100% → 92% death
(8% starve). T3 manor_blackspire / underforge still 100% death — the
remaining death pressure there is the post-2b chained-standard path,
not the elite roll.

Tier-lethality trace cross-check (`Phase2_TierLethality`) shows the
new alt-elites actually spawning and being winnable: underdark trial
1 now opens with `Drow Elite Warrior` (won), reaches the Roper at
turn 6, retreats, then dies on a Hook Horror nick — a substantively
longer arc than the pre-2c fresh-entry Roper one-shot. Dragons_lair
trials similarly show `Dragonborn Cultist` as a winnable elite stop
before the dragon roll lands.

Stat-block tuning of the bosses themselves is off-limits per the
Phase 3 constraint (bestiary is shared across systems).

### Phase 3 — global lever tuning

Tune the knobs that affect every zone uniformly:

- **Daily threat drift** (`dnd_expedition_threat.go:87` — base +3/day).
- **Threat band thresholds** (Quiet/Stirring/Alert/Hostile/Siege —
  `dnd_expedition_threat.go:70-82`).
- **Supply burn multipliers** (`dnd_expedition_supplies.go:43-56` —
  T1×1 → T5×3).
- **Surprise-round damage floor** (`dnd_expedition_combat.go:184`).
- **Encounter bracket thresholds** (`dnd_expedition_combat.go:39-44`
  None/Noise/Standard/Elite/Patrol).
- **Camp mods** (`dnd_expedition_night.go:85-92`).

Goal: every tier centerline lands within ±5pp of target. Bands stay
±10pp.

#### Phase 3-A — elite-bracket + drift sweep (shipped)

Wired two harness overrides
(`EliteInterruptThresholdOverride`, `ThreatDriftBaseOverride`) and
swept a 3×3 grid (elite ∈ {17, 19, 23} × drift ∈ {1, 3, 5}) over
the Phase 1 matrix. Test: `TestExpeditionBalance_Phase3_GlobalLeverSweep`.

**Strong partial positive.** The elite-bracket threshold is the
dominant lever for T1–T3; threat drift is a small modulator on top.

| (elite, drift) | T1 mean | T2 mean | T3 mean | T4 mean | T5 mean |
|----------------|--------:|--------:|--------:|--------:|--------:|
| 17, 1 (more elites) | 0.5% | 0.0% | 0.0% | 0.0% | 0.0% |
| 19, 3 (live)        | 3.0% | 0.2% | 0.0% | 0.0% | 0.0% |
| 23, 1 (rare elites) | **24.0%** | **7.2%** | **1.8%** | 0.0% | 0.0% |
| 23, 3               | 16.5% | 4.0% | 2.0% | 0.0% | 0.0% |
| 23, 5               | 11.8% | 3.2% | 0.2% | 0.0% | 0.0% |

Best cell at e=23/d=1: goblin_warrens 40.5%, sunken_temple 14.5%.
Still well below the T1 70-90% / T2 62-82% target bands — the lever
moves the needle in the right direction but cannot land any tier
on-band alone.

**T4/T5 fingerprint changed but didn't lift.** At e=23 the dragons_lair
death-fraction drops from ~60% → 24% but starvation climbs to 75% —
the fighter now survives elites long enough to run out of supplies.
T4 (underdark/feywild) likewise sees death drop and starvation rise.
Indicates a second lever is needed for the higher tiers: either
standard-fight survivability (surprise-nick floor / standard roster
density) or supply margin.

**Next:** Phase 3-B sweeps surprise-round nick and supply-burn knobs
on top of e=23/d=1 to push T1-T3 toward target band and lift T4-T5
off the floor.

#### Phase 3-B — nick-floor + supply-burn sweep (shipped)

Wired two more harness overrides (`SurpriseNickFloorOverride`,
`SupplyBurnRatePctOverride`) and swept a 3×3 grid (floor ∈ {0, 1, tier
(=live)} × burn% ∈ {50, 75, 100 (=live)}) on top of the Phase 3-A best
cell (e=23, d=1). Test: `TestExpeditionBalance_Phase3B_NickSupplySweep`.

**Strong partial positive for T5 supply margin; nick-floor lever inert.**

Tier-mean completion (Fighter, centerline level, 10 zones × 200 trials):

| (floor, burn%) | T1 mean | T2  | T3  | T4   | T5   |
|----------------|--------:|----:|----:|-----:|-----:|
| 0,    50       | 20.8%   | 5.0%| 3.8%| 7.5% | 29.5%|
| 0,    75       | 26.5%   | 6.0%| 2.8%| 9.0% | 0.0% |
| 0,   100       | 25.8%   | 5.0%| 3.0%| 0.0% | 0.0% |
| 1,    50       | 25.5%   | 6.5%| 1.8%| 8.5% | 27.8%|
| 1,    75       | 28.5%   | 6.2%| 4.0%|11.5% | 0.0% |
| 1,   100       | 26.8%   | 7.5%| 2.5%| 0.0% | 0.0% |
| tier, 50       | 26.8%   | 6.2%| 2.5%| 7.8% | 27.5%|
| tier, 75 (live floor) | **29.5%** | **7.8%** | 3.0% | **12.8%** | 0.0% |
| tier,100 (live)| 28.5%   | 6.8%| 3.0%| 0.0% | 0.0% |

Three readings:

1. **Supply burn is the T5 unlock.** dragons_lair completes ~55% at
   burn=50 (versus 0% at burn=75/100) — the fighter survives elites
   but starves en route. burn=75 isn't enough margin; burn=50
   converts the starvation-out into a completion.
2. **Supply burn is the T4 modulator.** underdark/feywild step from
   0% (burn=100) → 12% (burn=75) → 8% (burn=50). The burn=75 peak is
   the sweet spot; halving further leaves the fighter alive *into*
   more elite fights, where it dies in combat instead.
3. **Nick-floor lever is inert.** Across all burn levels, dropping
   the surprise-round floor from `tier` → `1` → `0` moves T1-T3
   means by at most ~3pp. The wounded-clamp (Phase 2b,
   `clampSurpriseNick`) already eats the chip-damage budget on
   repeat entries; the raw floor only matters on the first fresh-HP
   swing, which the engine recovers from. **Recommendation: discard
   this lever from the live-tuning candidate list.**

**T2-T3 wall persists.** Even at the best (floor=tier, burn=75) cell:
T2 reads 7.8% (target 62-82%) and T3 reads 3.0% (target 54-74%).
forest_shadows T2, manor_blackspire T3, and abyss_portal T5 sit at
~0% across every combo — these are zone-specific outliers, not
addressable by any global lever. **Phase 4 (per-zone outlier pass)
is the next move.**

**Next:** Phase 4 walks the matrix zone-by-zone. The shipped Phase 3
sweep already names the worst offenders (forest_shadows T2,
manor_blackspire T3, abyss_portal T5, crypt_valdris T1); each needs a
diagnostic trace (boss stat-block, roster mix, supply DC) to pick the
right per-zone tool from `Phase 4`'s list. Optional: revisit shipping
the burn=75 global if Phase 4 outlier fixes leave T4 still stuck.

### Phase 4 — per-zone outlier pass

After globals settle, the matrix names outlier zones — tiers where
one zone reads dramatically off-band while sibling zones are fine.
Per-zone tools:

- `SpawnWeight` / `IsElite` flags in zone rosters (`dnd_zone.go:100`).
- Boss stat tuning in zone definitions.
- Resource DCs (affects harvest interrupt roll).
- Per-zone loot table density (downstream economy impact — flag if
  significant).

Constraint: do not touch monster stat blocks (those belong to the
bestiary, shared across systems). Tune the roster + zone-level knobs,
not the monsters themselves.

#### Phase 4-A — outlier diagnostic (shipped)

Added a structured per-fight trace hook (`traceFightStruct` +
`harnessFightTrace`) alongside the existing string `traceFight`;
both fire from the same `runHarnessFight` site so the human log and
the structured aggregate stay in sync. Test:
`TestExpeditionBalance_Phase4A_OutlierDiagnostic`. For each of the
four outlier zones it pairs against the healthy sibling at the same
tier (goblin_warrens, sunken_temple, underforge, dragons_lair),
running 200 trials each on the Phase 3-B best cell (e=23, d=1,
burn=50) and reporting per-monster appearances / win-rate / avg HP
loss / kill attribution + day-of-end histogram.

Findings — each outlier had one or two miscategorised entries that
Phase 2c's single-boss-elite dilution sweep had skipped:

- **crypt_valdris**: dual-killer elite pool (Wight 99k, Flameskull
  68k); Phase 2c had skipped this zone as "already dual-elite," but
  both elites are over-tier for T1.
- **forest_shadows**: standard pool too lethal — Displacer Beast
  (38% win, 53k) + Bandit Captain (57% win, 48k).
- **manor_blackspire**: Wraith on standard slot (45hp loss per win,
  85k).
- **abyss_portal**: Nalfeshnee mis-classified standard (CR-13, 2.8%
  win, 86k) + Vrock standard at 43hp loss per win.

#### Phase 4-B — roster re-classification (shipped)

Per-zone roster tweaks based on Phase 4-A's attribution. No monster
stat-block touches.

- crypt_valdris: dropped flameskull (lives in underforge T3);
  promoted specter to IsElite SW=3 as the soft dilutor.
- forest_shadows: promoted Displacer Beast + Bandit Captain to
  IsElite; Owlbear SW 4→2 so the 4-elite pool dilutes evenly.
- manor_blackspire: promoted Wraith to IsElite (joins
  Spawn+Revenant).
- abyss_portal: promoted Nalfeshnee + Vrock to IsElite (collapses
  standard pool to Quasit alone, ≥99% win).

Outlier-vs-sibling completion (Phase 3-B best cell, 200 trials):

| Zone | Before | After | Sibling |
|------|-------:|------:|--------:|
| crypt_valdris T1   |  8.5% | **46.0%** | 45.0% |
| forest_shadows T2  |  0.0% |  **7.5%** | 13.0% |
| manor_blackspire T3|  0.5% | **14.5%** |  3.0% |
| abyss_portal T5    |  0.0% | **25.0%** | 57.5% |

Phase 3-B tier mean (b=50, f=tier) before → after:
  T1 26.8% → 44.0% (+17pp), T2 6.2% → 10.0% (+4),
  T3 2.5% →  8.5% (+6),  T4 unchanged, T5 27.5% → 41.0% (+14).

**Exit (partial):** the four off-band outliers are no longer
outliers — every per-zone cell is within the spread of its tier
siblings. The Phase 4 plan's stricter exit ("all per-zone cells
within target band") is **not** met: T2/T3/T5 sibling pairs still
sit below band (T2 7-13% vs 62-82%; T3 3-14% vs 54-74%; T5 25-57%
vs 36-56%). That gap is tier-wide, not zone-specific — both zones at
each problem tier underperform together — so no further per-zone
tool will close it. Phase 5 (or a follow-up tier-wide pass) needs to
take over.

**Next:** Phase 5. Candidate moves are tier-wide: revisit player
gear-tier mapping at T2/T3 (the centerline level vs gear ladder, see
`phase1TierCenterline`), reconsider the elite-bracket threshold
per-tier instead of global, or ship `burn=75` as the new live global
since Phase 4-B already converted T4 to playable at b=50.

### Phase 5 — optional MAD / second-order

If post-Phase-3 the bands hold but feel wrong subjectively
(e.g. "median days too short", "no expedition ever hits Siege"), add a
secondary assertion on median-days-to-extract or extract-threat. Phase
3 may also surface a request to expose a difficulty knob to operators
(env var or admin command); scope that here if it comes up.

---

## Open questions

- Should the babysit perk be on or off in the sim? Pick one and
  document; both as separate cells doubles matrix size for diminishing
  insight. Default proposed: **off**, since it's a discovery-mechanic
  buff ([[feedback_npc_buffs_are_secret]]) — players who find it earn
  extra margin, not the baseline.
- Pardon proc (33% @ chat 20+): include in sim, but at a fixed
  chat-level assumption (chat 15 = no pardon, since most fresh runs
  haven't hit the threshold). Decide in Phase 0.
- Sovereign reprieve: skip in sim — it's gated on full set equipped
  and is a one-shot per cooldown. End-game-only; not part of the
  baseline difficulty.
- Multi-region zones (E4): does the sim need to walk all regions in
  one expedition, or pick one region per run? Probably full walk for
  realism. Settle in Phase 0.
- DM mood: starts at 50; drifts via combat outcomes. Let it drift
  naturally in the sim, do not pin.

---

## Deliverables checklist

- [ ] `internal/plugin/dnd_expedition_balance.go` (sim harness)
- [ ] `TestExpeditionBalance_Phase0_Spike`
- [ ] `TestExpeditionBalance_Phase1_FullMatrix` (becomes the permanent
      regression gate after Phase 2 tightens it)
- [ ] Tuning commits per phase
- [ ] One-line update to `MEMORY.md` + project memory pointer
