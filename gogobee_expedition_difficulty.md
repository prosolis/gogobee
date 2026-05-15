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

### Phase 2c — roster gate (next)

Promote softer mid-tier monsters to `IsElite: true` in zones whose
elite pool is a single tier-disproportionate boss (warchief / hag /
roper / dragon are all SpawnWeight=1 alone in their elite pool, so
an InterruptElite bracket roll = 100% pick of the over-tier boss).
Diluting the elite pool with one or two softer alternates at higher
SpawnWeight lets the fighter sometimes face a winnable elite,
without removing the over-tier boss as a possible spawn.

Stat-block tuning of the bosses themselves is off-limits per the
Phase 3 constraint (bestiary is shared across systems).

### Phase 2 — global lever tuning

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

**Exit:** test asserts the ±10pp band per tier; commit.

### Phase 3 — per-zone outlier pass

After globals settle, the matrix will name outlier zones — tiers
where one zone reads dramatically off-band while sibling zones are
fine. Per-zone tools:

- `SpawnWeight` / `IsElite` flags in zone rosters (`dnd_zone.go:100`).
- Boss stat tuning in zone definitions.
- Resource DCs (affects harvest interrupt roll).
- Per-zone loot table density (downstream economy impact — flag if
  significant).

Constraint: do not touch monster stat blocks (those belong to the
bestiary, shared across systems). Tune the roster + zone-level knobs,
not the monsters themselves.

**Exit:** all per-zone cells within band; commit.

### Phase 4 — optional MAD / second-order

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
