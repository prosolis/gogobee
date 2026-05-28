# Long Expeditions — multi-session plan

> Goal: dungeon expeditions become real multi-day journeys (T1 ≈ 2 days → T5 ≈ 7 days). Player picks **camp supplies** at launch, then watches it play out. Camp pitch/break, elite engagement, and **boss engagement** all become autonomous. Foreground `!camp` / `!fight` survive as overrides, not the expected path.

## Status @ 2026-05-28

**Core track (D1–D7) — DONE.** All phases shipped; v1 of the long-expedition mechanics is live on `long-expeditions-d1`.

**§6 decisions (2026-05-28) re-opened two threads** that ride after D8:
- **D9** — T1–T3 room-count bump toward the 3–5× target band (D1 landed T1–T3 at ~2×). Sim-first: read T1/T2 day-counts from a fresh corpus *after* D8 lands.
- **D10** — T4/T5 anchor-room variety (2nd elite + 2nd trap mid-zone). Bundle with D9 to avoid touching graphs twice.

**Active work — §8 (J3 caster sim-picker):** D8-a / D8-b / D8-prereq / D8-d-diagnostic shipped; **D8-c (concentration), D8-d-fix (AC floor lift), D8-e (martial regression triage)** queued. D8 is sequentially the gate before D9/D10 — room counts only make sense once the sim accurately scores the classes that will walk through them.

**Sequence:** D8-c → D8-d-fix → D8-e → D9 → D10. Re-baseline once at the end.

## 1. Why the current shape is "too short"

| Tier | Zones | Rooms | Real-time exit |
|------|-------|------|----------------|
| T1   | Goblin Warrens, Crypt Valdris       | 6–7  | usually <1 calendar day |
| T2   | Forest Shadows, Sunken Temple       | 7–8  | usually 1 day |
| T3   | Manor Blackspire, Underforge        | 7–9  | 1–2 days |
| T4   | Underdark, Feywild Crossing         | 8–10 (×regions) | 2–3 days |
| T5   | Dragon's Lair, Abyss Portal         | 9–10 (×regions) | 2–4 days |

(Room counts from `internal/plugin/dnd_zone.go:70-134`; tier zones from memory `project_expedition_difficulty`.)

Because the autopilot walks ~3 rooms / 2h (`expedition_autorun.go:43,53`) and boss/elite stops are manual, a player who pays attention clears T1–T3 in an evening. Briefing/recap UTC ticks fire but rarely matter — the expedition is over before "day 2" ever lands.

## 2. Target shape

| Tier | Target duration | New room budget (single-region) | Notes |
|------|-----------------|---------------------------------|-------|
| T1   | 2 days  | 12–14 | one autopilot-camp |
| T2   | 3 days  | 16–20 | two camps |
| T3   | 4 days  | 22–26 | three camps; threat starts to bite |
| T4   | 5–6 days | 28–34 (split across 3–4 regions) | base camp emerges |
| T5   | 7 days  | 36–44 (split across 3–4 regions) | full siege/temporal pressure |

(Numbers are first-draft anchors; class-balance sim drives the actual dial in D7.)

Duration is measured in **expedition-days advanced by autopilot camp**, not real-time UTC days. Each autopilot night-camp = day++. Real-time UTC still drives the morning briefing DM cadence, but day-count and supply burn become event-driven (camp = sleep = day++ = burn).

The autopilot decides:
- when to camp (and which camp type)
- whether to engage elites and bosses, or to camp first and retry
- whether to extract early on starvation/HP collapse

The player decides:
- supply pack purchases at launch (only meaningful pre-expedition choice)
- whether to override (`!camp <kind>`, `!fight`, `!expedition extract`)

## 3. Phasing

Built across multiple sessions; each phase ships independently with tests and a small DM-side feedback loop.

### D1 — Length + room-count rework
**Files:** `dnd_zone.go` (per-zone Min/MaxRooms), `dnd_expedition_region.go` (multi-region region sizes), `dnd_zone_run.go:generateRoomSequence` (still terminates Entry → … → Elite → Boss, but with more Explorations).
**Work:**
- Re-pitch Min/MaxRooms per zone to the table in §2.
- Multi-region zones: extend per-region room budgets so the total stays in band when summed.
- Keep the fixed Entry / Trap / Elite / Boss anchors; only Exploration count scales.
- Consider 2 traps + 2 elites for T4–T5 to break up the long middle (deferred decision — see §6).
- Update existing zone graphs in `zone_graph_*.go` for the new lengths. *Auditing whether each zone has a hand-tuned graph or relies on the linear compiler is part of D1 scoping.*

**Exit criteria:** `expedition-sim` shows mean-room-cleared per zone in the new band; no test fixtures hard-code old counts.

### D2 — Autopilot camp scheduler

**D2-a (shipped 2026-05-27):** new `expedition_autocamp.go` with pure `decideAutopilotCamp` + `pitchAutopilotCamp` + dwell-window lifecycle (`shouldSkipAutoRunForCamp`, `breakAutoCampIfDue`). Wired into `tryAutoRun`: skip while inside `minAutoCampDwell` (4h), break + walk past it, post-walk scheduler pitches HP-low rest camps and region-boss base-camp waypoints. CampState gained `AutoPitched` so auto- vs player-pitched lifetimes diverge. Day rollover is still UTC-anchored; D2-b moves it event-anchored.

**D2-b (shipped 2026-05-27):** event-anchored day rollover. `dnd_expedition_cycle.go` adds `eventAnchoredCutoff` + `isEventAnchored`, splits the briefing body into `nightRolloverBurn` + `nightRolloverDrift` (+ a convenience `processNightCamp` that runs both back-to-back). For expeditions started ≥ cutoff, `deliverBriefing` skips mutators and posts a re-engagement DM; if `last_briefing_at` is older than `nightSafetyNet` (28h) it force-fires `processNightCamp` so a stalled autopilot doesn't freeze the expedition. The autopilot scheduler grew a `Night` flag — `decideAutopilotCamp` sets it when `time.Since(LastBriefingAt) ≥ nightCampWindow` (16h); `pitchAutopilotCamp` then runs the burn → camp → rest → drift sequence in one pitch. Manual `!camp <type>` runs the same flow when it's the first camp since the last rollover. Legacy UTC-anchored expeditions (start_date < cutoff) keep the original mutator flow via the same staged helpers, preserving rest-before-drift ordering through `processOvernightCamp`. Tests fence cutoff to year 9999 in `TestMain` so existing legacy assertions stand; new tests exercise the night decision, night-pitch rollover, event-anchored skip, and safety-net force.


**Files:** new `expedition_autocamp.go`; hooks in `expedition_autorun.go:tryAutoRun`; reuse `dnd_expedition_camp.go:campPitch` / `applyCampRest`.
**Heuristics (in priority order):**
1. **Day-budget pacing** — pitch when rooms-walked-this-day ≥ tier-specific day-room target. Standard if room cleared, rough if not.
2. **Resource gates** — pitch if HP < 35% of max, or supplies projected to cover <1 more day's burn (`exp.Supplies.Current < exp.Supplies.DailyBurn * harshMod`).
3. **Post-elite breath** — auto-pitch a standard camp in the cleared elite room.
4. **Region boss cleared in multi-region zone** — auto-pitch base camp at first eligible site.
5. **Don't pitch when** mid-fight, in a trap/boss room, in a fork-pending state, or already camped.

Auto-break already happens on move (`autoBreakCampOnMove` in `dnd_expedition_camp.go:401`). Autopilot just needs to *not* break a freshly-pitched camp by immediately walking on — gate by `time.Since(camp.EstablishedAt) > minCampDwell` (15min real-time? or simply: don't auto-walk while camped).

**Day-rollover semantics change (decided event-anchored):** today, day++ happens at the 06:00 UTC briefing (`dnd_expedition_cycle.go:236`). After D2, the autopilot **night-camp pitch** is the event that fires day++, supply burn, overnight rest, and threat drift — in one atomic rollover the camp scheduler triggers. The 06:00 UTC briefing tick survives only as a re-engagement DM anchor (and a safety net: if for some reason no camp pitched in N hours, force one). Concretely:
- Move the body of `deliverBriefing` (`dnd_expedition_cycle.go:186-`) into a `processNightCamp(exp)` helper called from the autopilot camp scheduler when it pitches a *night* camp (rough/standard/fortified — not mid-day breath stops).
- Distinguish "night camp" (advances day) from "rest stop" (doesn't). Heuristic: a camp pitched at the end of the day-budget room count is night; one pitched purely for HP/SU recovery mid-day is a rest stop. Both apply `applyCampRest`; only night also runs supply burn + day++ + threat drift.
- The existing UTC briefing ticker becomes: if `last_briefing_at` is older than ~20h *and* the user is active, post a "Day N — here's where you stand" re-engagement DM. It calls **no** mutators.
- Manual `!camp <type>` from the player still does what it does today (apply rest immediately) but **also** counts as a night camp if invoked with no other camp today — keep parity with the autopilot behavior so the override doesn't accidentally stall day advancement.

**Migration:** in-flight expeditions at deploy time keep their existing 06:00 UTC day++ until they end (fence by `start_date`). New expeditions use the event-anchored model.

**Exit criteria:** sim shows autopilot pitches the right kind of camp at the right room with no player input; supply economy still survives a 7-day T5.

### D3 — Autonomous elite + boss engagement
**Files:** `dnd_zone_cmd.go:497-535` (the prev==RoomElite / RoomBoss branch).

**Shipped 2026-05-27.** `dnd_zone_cmd.go` adds `stopBossSafety` + `bossSafetyGate(uid, exp)` (HP < 80%, supplies < daily burn, exhaustion ≥ 3 — the "active low status" interpretation). The elite/boss doorway branch drops the `prev == RoomBoss || !compact` carve-out: in compact mode bosses fall through to the same `resolveCombatRoom` path elites have used, after the gate clears. `resolveCombatRoom` selects monster + label + loot-drop by `run.CurrentRoomType()` so the same call site handles boss kills (zone.Boss bestiary, "👑 Boss — name down", boss-loot drop, elite-tier threat bump). Loss on auto-resolved boss falls through to the existing player-death narration / forceExtractExpeditionForRunLoss path — no special treatment.

The walk loop (`dnd_expedition_cmd.go:717`) only breaks at the elite/boss doorway when `!compact`; compact lets the next iteration auto-resolve. `stopBossSafety` is excluded from the rooms-walked tally and its `autopilotFooter` returns "" — `res.final` already carries the held-back line.

When the gate trips, `tryAutoRun` calls `pitchBossSafetyCamp(exp)` (new in `expedition_autocamp.go`): force-pitches Standard (or Rough fallback) regardless of `decideAutopilotCamp`'s HP threshold or its RoomBoss room-type block, with the same `Night` event-anchored rollover handling. Dwell expires → next tick retries the boss. Foreground `!fight` and foreground `!expedition run` are unchanged: both still stop at the boss doorway for the player who wants to watch. Sim path is unaffected — `stopBossSafety` falls into `expedition_sim.go`'s default branch (tick-a-day, retry next walk).

**Open question:** do we want a "set autopilot off" toggle for players who actively want to play the boss themselves? My take: default-on, single-flag per expedition (`autopilot_engage`) settable at start or via `!expedition autopilot off`. Defer until D3 lands and we see how it reads.

**Exit criteria:** sim runs a full T5 expedition launch → boss kill with zero player commands besides `!expedition start`.

### D4 — DM volume + day surfaces
The big risk: a 7-day T5 with autopilot walking, camping, fighting elites, and killing bosses will spam DMs unless we bundle. The user has already pushed back on recap chatter (`feedback_skip_recaps`).

**D4-a (shipped 2026-05-27):** autopilot DM bundling. `expedition_autorun.go:tryAutoRun` now drops per-tick auto-walk DMs in compact mode. New surface rules:
- Fork / death / run-complete still DM the walk stream — interactive + climax beats.
- A Night=true camp pitch flushes the day as an end-of-day digest (`renderEndOfDayDigest` in new `expedition_autorun_digest.go`) followed by the camp block.
- Boss-safety camp pitch gets a short "holding before the boss" header + camp block (walk stream dropped).
- Non-night auto-camp (mid-day rest / base-camp waypoint) surfaces the camp block by itself.
- Everything else — uneventful walks, preflight pauses, harvest interrupts — goes silent.
Each successful background walk now logs a `walk` entry (`appendExpeditionLog(... "walk" ...)`) so the digest can count rooms walked from structured logs (the raw `r.stream` narration is no longer persisted across ticks). Digest groups counts of walk/harvest/interrupt and inlines threat/milestone/narrative lines for the prior day (`dayExpeditionLog` helper). `maybeAutoCamp` + `pitchBossSafetyCamp` now return the `autoCampDecision` so `tryAutoRun` can branch on `dec.Night`.

**D4-b (shipped 2026-05-27):** morning re-engagement DM anchored to user activity, not 06:00 UTC. `fireExpeditionBriefings` now skips event-anchored expeditions whose `last_activity` is older than today's 06:00 UTC threshold (and we're still inside `nightSafetyNet` — stalled-autopilot force-fires still win). New `maybeDeliverDeferredBriefing` runs at the top of `AdventurePlugin.OnMessage` on every inbound message in any room: it stamps `last_activity` (so the next ticker pass sees the player as present, even from non-bot chatter) and posts the deferred briefing if one is owed (CAS-gated by today's 06:00 threshold; pre-06:00 lazy fires are suppressed to avoid double-emit with the ticker sweep that follows). Day-1 inbounds don't trigger a briefing before the first night camp has happened.

**Remaining work:**
- TwinBee voice + no-recap copy pass on the digest once the shape settles.

**Exit criteria:** a 7-day T5 produces ≤ 10 DMs across the whole expedition (1 launch + 6 end-of-day + 1 boss + 1 run-complete + 1 emergency).

### D5 — Supplies economics retune

**D5-a (shipped 2026-05-27):** per-tier pack caps. `dnd_expedition_supplies.go` retires the global `SupplyPackStandardMax`/`SupplyPackDeluxeMax` constants in favor of `supplyPackCaps(tier) (std, dlx int)` — T1/T2: (2,1); T3: (3,1) unchanged; T4: (5,1); T5: (7,2). `SupplyPurchase.Validate` is now `Validate(tier ZoneTier)`; both call sites (`!expedition start`, `!resume`) pass the resolved zone's tier. The cap clears `DailyBurn(raw) × intendedDays × 1.3` for every tier under the §2 target durations even with harsh×3 layered on top, so a player who wants to buy for the long shape can. DailyBurn / harsh-multiplier / `phase5BDailyBurnRatePct` are unchanged here — they're a D7 lever, alongside the sim work that should unblock empirical baselining. The help text now describes the cap as "scales by zone tier"; the holiday +1 standard pack still bypasses the cap on purpose (it's a freebie on top).

> **Note:** the original "Empirical (sim-driven)" path was blocked: under D2-b event-anchored expeditions, `SimRunner.TickDay` calls `deliverBriefing` → `deliverBriefingEventAnchored`, which reads `time.Now().UTC()` for its safety-net check, so synthetic ticks never advance `CurrentDay` and `DaysAtEnd` stays at 0. Re-baselining off real day-counts requires teaching the sim to drive the event-anchored rollover (D7).

**D5-b (shipped 2026-05-27):** "Pick your loadout" preset prompt at launch. `!expedition start <zone>` with no pack arg now DMs a Lean / Balanced / Heavy menu (sized per zone tier via new `loadoutPurchase(tier, SupplyLoadout)` in `dnd_expedition_supplies.go`); player replies with `!expedition start <zone> lean|balanced|heavy` (short forms `l`/`b`/`h` also work). `!resume` got the same treatment. Raw `Ns Md` syntax remains the advanced override — `resolveLoadoutOrParse` in `dnd_expedition_cmd.go` tries a single-token preset first, falls back to `parseSupplyArgs`. Heavy always equals `supplyPackCaps` (the D5-a harsh×3 ceiling); Lean covers intended days at raw burn; Balanced sits between. `parseSupplyArgs("")` still returns 1 standard for any tier — it's no longer reachable from `!expedition start` (the empty path is intercepted to prompt), but the `1s` default is preserved so future callers don't get surprise zero-pack purchases. Help text updated; two existing scenario tests now pass `lean` explicitly, three new tests cover preset resolution + the prompt-vs-start guard.

**D5-c (shipped 2026-05-27):** Ranger forage wired against the new caps. Audit found `SupplyForageMaxSU = 4` defined in `dnd_expedition_supplies.go` since Phase 12 E1b but never referenced — `ForagedToday` was only ever reset to `false`, never set to `true`, so the §4.2 "Ranger, WIS DC 12, 1d4 SU/day" perk had never actually granted supplies. New helper `applyRangerForage(e, c, rng)` rolls 1d4 SU once per day for Ranger characters, headroom-capped against `Supplies.Max` so a Heavy loadout doesn't overflow its purchased ceiling; non-Rangers and post-roll repeats are no-ops. Fires at the top of `nightRolloverBurn` (before the daily burn) so the +SU lands on today's bag and a "forage" log entry flows into the end-of-day digest via a new `renderEndOfDayDigest` case. No DC roll — accessibility over crunch (`feedback_accessibility_over_dnd_crunch`); the DC-12 gate would've added a fail case with no upside given the new headroom. Average +2.5 SU/day off a Ranger is a quiet Lean-loadout cushion (~1 extra day of T5 late-stage burn over a 7-day run), not a Heavy multiplier. Tests cover ranger-grants, non-ranger no-op, repeat-day no-op, headroom-cap, full-bag-still-stamps-flag, nil-safety.

**Remaining work (D5-d+):**
- DailyBurn / `phase5BDailyBurnRatePct` revisit once D7 sim measures real elapsed-day counts.

**Exit criteria:** balanced loadout completes intended-duration expedition in ≥ 85% of sim runs without starvation extraction.

### D6 — Player-facing surface cleanup

**D6 (shipped 2026-05-27):** player-facing copy now matches the autopilot-first reality.
- `expeditionHelpText` rewritten around the loop "pick a zone → pick a loadout → `!expedition run` watches it play out"; commands grouped into **Run an expedition** / **Mid-expedition** / **Overrides** with a one-line shape paragraph at the top.
- `!camp` and `!fight` were already absent from the primary `!expedition` help; both are now explicitly listed under **Overrides** and framed as "autopilot covers these — only reach for them if you want manual control." `campHelpText` itself opens with the same override framing so a player who types `!camp` lands on the right mental model.
- `expeditionCmdStatusImpl` (default `!expedition status`) now leads with **Day X / ~Y expected** (driven by new `expeditionTargetDays(tier ZoneTier)` in `dnd_expedition_supplies.go` — T1=2 → T5=7, the §2 anchor table), adds a **Rooms: N / Total in this region** line under the region marker (sourced from `getActiveZoneRun(...).CurrentRoom/TotalRooms`), and appends a **Recent:** block with the last 3 log entries (summary fallback to flavor). Supplies and threat lines unchanged; the `--debug` view is untouched.
- No new public command surface, no schema change, no test churn (no existing test asserted on the rewritten strings); full `go test ./...` green.

### D7 — Sim + class re-baseline

**D7-a (shipped 2026-05-27):** `SimRunner.TickDay` now drives event-anchored expeditions. `expedition_sim.go` short-circuits when `isEventAnchored(exp)` is true: instead of calling `deliverBriefing` (whose `deliverBriefingEventAnchored` branch reads `time.Now().UTC()` and would never trip the safety-net under synthetic ticks), it runs `nightRolloverBurn` → optional `applyCampRest(Standard)` if affordable (Rough fallback, skipped on low-SU) → `nightRolloverDrift(briefAt)`. Mirrors `pitchAutopilotCamp` with `Night=true`. Production paths untouched. New `expedition_sim_test.go` covers (i) 3 ticks advancing day 1→4 with supply burn + stamped `LastBriefingAt`, and (ii) the low-SU branch where the rest is skipped but burn/drift still fire. Unblocks D5-d empirical re-baseline of `phase5BDailyBurnRatePct` and the class corpus re-run.

**D7-b (shipped 2026-05-27):** `SimRunner.RunExpedition` now drives the production camp scheduler under a synthetic clock (`simWalkInterval = autoRunCooldown`, 2h per walk). After every soft-stop walk it calls `maybeAutoCamp(exp, simNow)`; `stopBossSafety` routes to `pitchBossSafetyCamp(exp, simNow)`. The helpers (`applyAutoCamp` / `applyAutoCampBossSafety`) advance `simNow` past `minAutoCampDwell` (4h) and break the auto-pitched camp via `breakAutoCampIfDue` so the next walk proceeds. `maybeAutoCamp`, `pitchAutopilotCamp`, and `pitchBossSafetyCamp` gained a `now time.Time` parameter (the only prod caller, `tryAutoRun`, still passes `time.Now().UTC()`). Effect: HP-low mid-day rests, multi-region base-camp waypoints, Night-camp rollovers, and boss-safety holds all fire under the sim — the D7-a `tickEventAnchoredRollover` shortcut is no longer used by RunExpedition (kept on TickDay for tests + the pre-cutoff legacy path). Coverage in `expedition_sim_test.go`: Night-camp day advance, HP-low mid-day rest, boss-safety force-pitch, no-op when neither trigger fires.

**D7-c (shipped 2026-05-27):** `cmd/expedition-sim` gained a `-days N` flag — when set, `RunExpedition` exits with `Outcome="day_capped"` after the Nth synthetic day rollover; 0 (default) is unbounded and only the existing `-cap` walk safety net applies. `SimResult` gained `DaySnapshots []SimDaySnapshot` ({Day, HPCurrent, HPMax, Supplies, Threat, Rooms}), captured at expedition start (Day 0), after every `applyAutoCamp`/`applyAutoCampBossSafety` Night-camp rollover, and one final end-of-run entry (reads `getExpedition(mostRecentExpeditionID(...))` when the row already extracted so closed runs still close their trajectory). `RunExpedition` signature picked up the `maxDays int` parameter; sole caller is the CLI. Coverage: `TestSimRunner_CaptureDaySnapshot_PopulatesFields` exercises the helper in isolation (HP from the live character row, SU/threat/day from the live expedition, Rooms from `res.Rooms`). Unblocks empirical D5-d retune of `phase5BDailyBurnRatePct` against per-day SU draws.

**D7-d (shipped 2026-05-27):** class corpus re-run + D5-d retune decision. First fixed a sim regression `expedition_sim.go` introduced by D5-b — bare `expedition start <zone>` now returns the loadout prompt instead of outfitting, so the sim was halting before persisting any expedition; harness now passes the `heavy` preset. Corpus: `sim_results/d7d_corpus.jsonl` (n=100 × 10 classes × 5 zones × L10 = 5000 runs, `heavy` preset, `-cap 300`). Analysis: `sim_results/d7d_analyze.py`. Writeup: `sim_results/d7d_findings.md`. **Key reads:** (i) leaderboard mirrors [[project_j2_sim_artifact]] — martials 78–82%, casters 21–42%, ~40pp cluster gap unchanged by long-expedition mechanics; (ii) bard/cleric trailers **not** relieved by autopilot camp pacing — cleric actually regressed at L10 (21% vs J2b L12 39%) on spell-pool richness, J3-territory; (iii) T3 hits target 4-day duration (median 3), T4 hits 5–6 days (median 5), T5 hits 5 of 7 (only 21 clears); (iv) **D5-d retune: no change.** `phase5BDailyBurnRatePct=50` + per-tier `DailyBurn` stay as-is — heavy-preset cleared runs end with 78–98% SU remaining; supply economy is not a binding constraint and players lose to HP zero, not starvation. New memory entry: [[project_d7d_baseline]].

**Remaining work (D7-e+):** none in the core D7 scope. Two follow-up phases (D9 room-count bump, D10 T4/T5 anchor variety) were opened by the 2026-05-28 §6 resolution and are sequenced after D8. See §9 below.

## 8. J3 caster-picker rework (next session)

**Why split out:** D7-d corpus confirmed the martial/caster gap is class-roster + sim-picker driven, not autopilot/burn/pacing driven. Long-expedition mechanics are settled; the trailer fix lives in the sim picker, not the expedition shell.

**D8-a (shipped 2026-05-27, this session):** quick data-layer cleanup. Bard L1 prepared list gained `thunderwave`; L2 gained `heat_metal`. Cleric L1 gained `inflict_wounds`. `shatter` overlay Classes broadened to mage/bard/sorcerer (overlay was already unioning via `mergeClassList`, so this is defensive). New `simPickSpiritualWeapon` runs before `simPickSpell` in `simPickCombatAction`: cleric with an L2 slot + spiritual_weapon prepared + `sess.Statuses.BuffSpiritProc == 0` opens the fight with it, picking up the existing `spiritWeaponStrike` per-round bonus-action attack. **Measured impact (L10, n=100/zone, heavy preset):** cleric 21.0 → 23.2% (+2.2pp), bard 34.2 → 32.8% (within noise). T3+ wall unchanged — the data fixes are correct but the picker math is what's binding. Files touched: `internal/plugin/dnd_spells_data.go:192`, `internal/plugin/dnd_spells.go:822,881,884`, `internal/plugin/expedition_sim.go:simPickCombatAction + new simPickSpiritualWeapon`. All green.

**D8-b (implemented 2026-05-27, **inert** — see D8-prereq below): sim picker upcasting.** `simPickSpell` (`internal/plugin/expedition_sim.go:933`) now enumerates one candidate per available slot ≥ native for every prepared damage spell, scored via `spellExpectedDamage(sp, slotLevel, c.Level)`. Cantrips contribute a single slot-0 candidate. Tie-break: highest slot first, then highest expDmg. When the winning slot exceeds the spell's native level, the picker returns `"<id> --upcast N"` so `parseCombatCast` upcasts in the engine. Build + tests green.

**Measured impact: nothing.** d8b corpus (`sim_results/d8b_corpus.jsonl`, same matrix as d7d) lands every class within ±1.5pp of the d7d baseline — bard −1.0, cleric −0.4, mage −1.2, sorcerer +1.4, warlock −0.8, druid +0.6, martials ±0.2. Pure noise band. Expected bard +5–10pp / mage+sorcerer+warlock +2–8pp never materialized.

**Root cause (discovered post-corpus, this session):** `simPickSpell` is **dead code** in the current sim path. Commit `68ed8e7` ("Long expeditions D3: compact autopilot auto-resolves boss rooms") added a `!compact` gate at `dnd_expedition_cmd.go:810` so compact autopilot inline-resolves boss/elite rooms via `resolveCombatRoom` → `runZoneCombat` → `SimulateCombat` instead of returning `stopBoss/stopElite` for `autoResolveCombat` to handle. The sim uses `compact=true` (`expedition_sim.go:433`), so `autoResolveCombat` — the only caller of `simPickCombatAction`/`simPickSpell` — never fires. Verified empirically: a stderr-traced `simPickSpell` produced zero hits across full bard/cleric L10 runs.

This rewrites the picker-era retrospective. **J2's `baseline_j2a_v2_all10.jsonl` (bard 40%, cleric 39%) was measured before D3, with the picker live.** d7d's L10 baseline (bard 34%, cleric 21%) was measured *after* D3, with the picker disconnected — that ~6–18pp drop is the post-D3 caster regression, not "long-expedition mechanics didn't help casters" as the d7d writeup framed it. D8-a's "+2.2pp cleric" was also noise (1σ ≈ 1.8pp on n=500).

**D8-prereq (SHIPPED 2026-05-28, commit `631764b`):** split the `compact` flag in `runAutopilotWalk` / `advanceOnceWithOpts` into `compact` + `inlineBossCombat`. Production background autorun (`expedition_autorun.go:173`) passes `(true, true)` — unchanged inline auto-resolve. Sim (`expedition_sim.go:432`) passes `(true, false)` so boss/elite doorways return `stopBoss`/`stopElite` after the safety gate; `autoResolveCombat` → `simPickCombatAction` → `simPickSpell` / `simPickSpiritualWeapon` now drive boss/elite fights through the turn-based `!fight` engine. D8-b upcasting went live with the rewire. Foreground (`!expedition run`) flow unchanged (compact=false short-circuits).

Also parallelized matrix mode via subprocess workers (`cmd/expedition-sim/main.go`, `-jobs` flag; defaults to NumCPU). Each worker is its own process so the plugin's global SQLite handle isn't a contention point.

**Measured impact (5000-run L10 corpus, `sim_results/d8prereq_corpus.jsonl` — full diff in `sim_results/d8prereq_findings.md`):**

| Class    | d7d  | d8prereq | Δ      |
|----------|-----:|---------:|-------:|
| cleric   | 21.0 |   46.8   | +25.8  |
| sorcerer | 29.4 |   52.0   | +22.6  |
| warlock  | 35.4 |   55.6   | +20.2  |
| mage     | 36.4 |   54.2   | +17.8  |
| druid    | 42.2 |   56.6   | +14.4  |
| bard     | 34.2 |   46.2   | +12.0  |
| (martials) | ~80 |   ~78    |  -2 to -4 (noise + T4/T5 turn-engine regression) |

Caster/martial gap closed from ~40pp to ~22pp. T2/T3 zones are now the picker payoff (forest_shadows 75 → 99, manor_blackspire 45 → 75). **T4 caster wall persists** (every caster 0% at underdark); **T5 is universally 0%** including martials. Both require separate diagnostics — picker isn't the binding constraint past T3.

**D8-c (SHIPPED 2026-05-28):** concentration-damage multiplier. `spellExpectedDamage` (`internal/plugin/dnd_class_balance.go:280`) multiplies expected damage by 3 when `sp.Concentration && sp.Effect ∈ {EffectDamageSave, EffectDamageAuto}`. `EffectDamageAttack` excluded by design — single-target attack-roll spells aren't typically concentration; hex-style cases get their lift from mods. Test: `TestSpellExpectedDamage_ConcentrationMultiplier`.

**Measured impact (d8c_corpus.jsonl vs d8prereq, full diff in `sim_results/d8c_findings.md`):** every class within ±2.4pp on the leaderboard (1σ ≈ 2.2pp on n=500) — *macro-leaderboard unchanged*. Picker swap landed where expected at **T3 manor_blackspire**: bard +11pp (32→43), mage +10pp (71→81), sorcerer +9pp (60→69), druid +5pp manor + +1pp underdark. T4/T5 caster wall unchanged (still 0% for every caster except druid 0→1 underdark) — confirms [[project_d8d_diagnostic]]: past T3 the binding constraint is HP+AC, not picker quality.

**Side discovery (engine modelling gap):** cleric stayed flat across all zones despite having spirit_guardians (L3, concentration, save) prepared — and warlock dropped 6pp at manor. Likely the sim's `EffectDamageSave` path resolves the AOE save *once* rather than re-ticking per round while concentration holds, so the multiplier overrates spells the engine then under-delivers on. **Filed as a separate follow-up** (concentration re-tick gap in `SimulateCombat`/turn-engine); does *not* unwind D8-c — bard/mage/sorcerer/druid picker swaps are clean and within plan. Cross-link [[project_concentration_retick_gap]] when written.

**D8-d (DIAGNOSTIC SHIPPED 2026-05-28):** T4 caster wall diagnosed; no code change landed. Writeup: `sim_results/d8d_findings.md`. **Hypothesis disambiguation across cleric/bard/mage/sorcerer/warlock/druid L10 underdark (n=3–5 each):** (a) multi-region transit damage — REJECTED. All casters TPK in r1 (`underdark_surface_tunnels`) before any cross-region transit; `Combats=0` (the elite/boss session table is empty); deaths are entirely inline `SimulateCombat` mob rooms. (b) T4 attack-bonus vs caster AC — confirmed contributor. `computeAC` floor gives casters AC 11–14 vs martial 16–17; T4 standard roster (Atk +5 to +7) hits ~60–65%. (c) HP scaling — confirmed contributor. Caster HPMax 93–110 vs martial 141 (~30% gap). (d) Heal-stock — minor lever, NOT the lever. Direct experiment: bumped `simConsumableBundle` T4 from 2 → 5 Spirit Tonics, re-ran n=5 per class. Median rooms-before-TPK lifted +3–5 but 0/5 clears across every caster. Reverted. **Recommended tuning lever (next session, NOT shipped here):** lift `computeAC` class floors (cleric/druid 3 → 5, bard/warlock 1 → 3, mage/sorcerer 0 → 2) — single function, easy to revert, doesn't move the martial leaders. Validate against `d8prereq_corpus.jsonl`. Hit-dice rescale is the second option but is bigger surface area.

**D8-e (TODO next session): martial T4/T5 regression triage.** Paladin -18pp T4 / fighter -6pp T4 -5pp T5 / ranger -13pp T5 — the cost of moving boss/elite fights from inline `SimulateCombat` to the turn-based `!fight` engine. Two possibilities: (1) turn-engine plays martials slightly worse than inline-sim (action economy / multiattack handling differs — see [[project_sim_multiattack_gap]]); (2) inline-sim was over-rosy and the new numbers are the real ones. Read: trace one paladin underdark run pre/post and diff damage exchanges. If turn-engine is correct, prod martial numbers were inflated and we should not "fix" the regression.

**Sequence:** D8-c → D8-d → D8-e. D8-c is the smallest lever and confirms the picker model. D8-d is the highest-value (caster T4 is the next gap to close). D8-e is informational — settles whether to trust the new numbers.

**Open questions for D8:**
1. ~~D8-b upcast aggressively or conservatively?~~ **Resolved:** aggressive — the T3 lift (mage +61, warlock +65, druid +68) confirms the model.
2. D8-c concentration-rounds factor: 3 fixed, or scale with tier (longer fights at T4/T5)? Fixed is simpler; tier-scaled is more correct.
3. ~~Cleric T3+ cliff~~ — **partial resolution:** picker rewire took cleric T3 0% → 39%. Cleric T4 is still 0% (and so is every other caster); now subsumed by D8-d.

## 4. Cross-cutting risks

- **Existing in-flight expeditions** at deploy time. Either fence by `start_date` (old expeditions finish under old rules) or write a one-shot migration that retunes their room counts. Lean: fence — these are typically <2 days.
- **The 24h zone-run inactivity timeout** (`dnd_zone_run.go:254`) — must be raised, or the autopilot must self-poke `last_action_at` per tick. With multi-day expeditions, a quiet stretch overnight is now expected.
- **Idle reaper** (commit `0f72484`, see `expedition_autorun.go` references) — already holds streak on autopilot; verify it tolerates 5-day expeditions.
- **Babysit perk** — `BabysitSafeRest` already upgrades Standard → Fortified at rest (`dnd_expedition_camp.go:241-245`); confirm interaction with autopilot picking Standard (no change needed; the upgrade fires inside `applyCampRest`).
- **Multi-region travel still unwired in autopilot** (memory `project_multiregion_travel_unwired`). For T4/T5 to actually take 5–7 days, autopilot **must** auto-advance regions on region-clear. D2/D3 covers this implicitly but call it out — likely a small dedicated commit.
- **Pet arrival** (memory `project_pet_event_reachability`) — emergence seam is at run-complete; longer expedition just delays the roll, no code change needed.
- **Twinbee voice + secret NPC buffs** (memories `feedback_twinbee_voice`, `feedback_npc_buffs_are_secret`) — none of the new copy can break these.

## 5. Schema impact

Likely none. Existing columns cover it:
- `current_day` → autopilot increments via `advanceExpeditionDay`.
- `camp_json`, `supplies_json`, `region_state`, `threat_*` already in place.
- One **possible** new column: `expected_days` int (set at launch from zone tier + loadout) so status/end-of-day DMs can render "Day 3/5". Cheaper to derive than store — defer.

## 6. Open questions for next session

1. ~~Day-advance semantics~~ — **decided 2026-05-27: event-anchored.** day++ fires on autopilot camp-pitch (autopilot night-camp = sleep = day burn + briefing). UTC clock becomes a re-engagement-DM anchor only, not a state mutator. See §3-D2 for implementation impact.
2. ~~Boss autopilot default~~ — **decided 2026-05-28: default ON.** D3 already ships compact-mode boss auto-resolve gated by `bossSafetyGate` (HP/SU/exhaustion). No opt-out flag for now; revisit if players want manual final-blow framing.
3. ~~Per-tier room counts: sim-first vs ship-and-refine~~ — **decided 2026-05-28: sim-first refinement in D7-d.** D1 shipped a guess; we're much closer now. Measured vs prior: T1 ~2x (7→16), T2 ~1.8x (9→19), T3 ~2x (11→35), T4 ~3x (10→46), T5 ~3x (13→47). Target band is 3–5x; T1–T3 likely want another bump but defer until the D7-d sim corpus reads day-counts. Don't churn graphs blindly.
4. ~~Extra anchor rooms (T4/T5)~~ — **decided 2026-05-28: yes, add anchor-room variety in T4/T5.** Second elite + second trap mid-zone (not just rely on multi-region/patrols/threat). Slot into D7-d alongside the room-count retune so we don't re-touch the graphs twice.
5. ~~Mobile/AFK / DM cadence~~ — **decided in D4-a (already shipped):** event-anchored to in-game day. Night-camp pitch flushes the day as a digest; fork/death/run-complete still DM the walk stream; mid-day rests/waypoints surface only the camp block; per-tick auto-walks silent. No real-time pacing knob.

## 7. Sequence

**Original D1–D7 plan (all shipped):** D1 → D2 → D3 → D5 → D4 → D6 → D7.

**Current sequence (post-2026-05-28):** D8-c → D8-d-fix → D8-e → D9 → D10 → final re-baseline.

| Phase | What | Blocks |
|------|------|--------|
| D8-c  | Concentration-damage multiplier in `spellExpectedDamage` | D8-d-fix read |
| D8-d-fix | Lift `computeAC` caster floors (cleric/druid 3→5, bard/warlock 1→3, mage/sorcerer 0→2) | D8-e read |
| D8-e  | Diff turn-engine vs inline-sim for paladin/fighter/ranger on T4/T5; decide whether martial regression is real or a "fix" target | D9 (need stable class baseline) |
| D9    | T1–T3 room-count bump toward 3–5× band, sim-validated against fresh corpus | D10 |
| D10   | T4/T5 anchor-room variety (2nd elite + 2nd trap mid-zone) | final re-baseline |
| —     | One final L10 + L12 corpus pass to confirm tier day-counts + class spread | — |

## 9. Post-§6 follow-ups (D9 / D10)

### D9 — T1–T3 room-count bump

**Why now:** §6 fork 3 resolved to "sim-first refinement"; D7-d corpus shows T3 hitting only median 3 days (target 4) and T1/T2 weren't even measured because the corpus only matrixed T3+. T1–T3 graphs landed at ~2× prior length in D1; target band is 3–5×.

**Plan:**
- Pull T1/T2 day-counts from a post-D8 corpus run (matrix needs to include `goblin_warrens`, `crypt_valdris`, `forest_shadows`, `sunken_temple` at L4/L6).
- Lift graph sizes only if median days < tier target (T1=2, T2=3, T3=4). Don't churn graphs that already hit target.
- Reuse the D1-a..e fork/anchor/merge patterns (`internal/plugin/zone_graph_*.go`); preserve existing topology — only add Exploration nodes + the second-anchor slot D10 will fill.
- One-shot bootstrap: existing in-flight expeditions fence by `start_date` per §4 (consistent with D1 deploy).

**Risk:** if D8-d-fix lifts caster T3 clears materially, day-counts move too; measure after D8 stabilizes, not before.

### D10 — T4/T5 anchor-room variety

**Why now:** §6 fork 4 resolved "yes". T4/T5 zones run 30–50 rooms with only one Trap + one Elite anchor; the long middle reads same-y. Bundle with D9 so we touch each `zone_graph_*.go` once.

**Plan:**
- Add a second Trap + second Elite anchor mid-zone for all T4/T5 zones (`underdark`, `feywild_crossing`, `dragons_lair`, `abyss_portal`).
- Place the second Elite at the region-boundary spurs (good narrative justification — region-guardian rather than zone-guardian) so multi-region travel finally gets a teeth-y interrupt.
- Place the second Trap on the long Exploration runs of fork branches, not on the merge node (keeps fork-choice loot/risk asymmetric).
- Preserve longest-path length within current band — added anchors replace Exploration nodes, not append.

**Validation:** same final corpus pass as D9. Look for boss-clear rate dipping by <5pp (anchor difficulty is intended, not punishing) and median day-count stable.

### Cross-links picked up here
- [[project_multiregion_travel_unwired]] — boss-clear → next-region auto-advance is wired (verified 2026-05-27); D10's region-boundary elites give it something to interrupt.
- [[project_j3_caster_picker]] / [[project_d8d_diagnostic]] — D8 gates the whole sequence.
- [[project_d8prereq_baseline]] — the corpus to diff against, not d7d.
