# Gogobee — Harvest Charge Semantics & Auto-Harvest Parity

**Status:** plan, not yet implemented.
**Branch convention:** new branch off `open5e` (e.g. `harvest-charges` or `phase-H`).
**Prerequisite:** none — the autopilot harvest path is already in place; this is a behavior + parity change.
**Owner:** future session.

---

## 1. Goal & non-goals

### Goal

Retire the "retry-to-success" grind that the current harvest system encourages and replace it with Josie's deterministic-attempts model:

- A node spawns with N charges. Each charge = exactly one d20 attempt.
- **Success or failure both consume a charge.** No retries.
- The autopilot burns all charges in one room-pass, reporting a final tally.
- Rare+ nodes are no longer special-cased — they get auto-attempted like everything else, and a whiff is a whiff.
- `!zone advance` gains auto-harvest parity with `!expedition run` (today only the expedition autopilot auto-harvests; zone runs still require typing `!forage`/`!scavenge`/etc per room).

The user-facing pitch: stop typing `!mine`/`!scavenge`/etc. The walk handles it. Failure feels unlucky on purpose; that's the point.

### Non-goals

- Reworking the combat engine, threat system, or noise mechanics.
- Changing what *resources* exist or what zones they belong to (the registry stays as-is).
- Changing DCs per resource (calibration is a yield-quantity / charge-count knob, not a DC knob — see Phase H4).
- Changing class bonuses (`classHarvestBonus`) or per-action ability mapping (`harvestActionAbility`).
- Building any new "rare buff" or pre-roll consumable surface. None exists today and we're not adding one.

---

## 2. Current state (so future-me has the receipts)

- `internal/plugin/dnd_expedition_autopilot_harvest.go`:
  - `autoHarvestPerNodeAttempts = 8` — safety cap on retries.
  - Loop re-rolls on failure without consuming charges (comment at line 22-25).
  - `isRarePlus` filters Rare/Epic/VeryRare/Legendary nodes out of the auto pass.
  - `RarePending` + `renderRarePendingFooter` + `stopRareNode` produce the "Autopilot paused — rare yield nearby" UX.
- `internal/plugin/dnd_expedition_cmd.go:636` is the **only** caller of `autoHarvestRoom`.
- `internal/plugin/dnd_zone_cmd.go:417` (`zoneCmdAdvance`) → `advanceOnceWithOpts`: resolves one room and dispatches. **No harvest pass.** Players still type manual harvest commands per room.
- Manual harvest commands (`!forage`/`!scavenge`/`!mine`/`!fish`/`!essence`/`!commune`) live in `internal/plugin/dnd_expedition_harvest.go` (`handleHarvestCmd` at line 387). They operate on the *expedition's* current room.
- Charge semantics today: a node has a `Charges` count; success consumes one; failure does not. Failure can still trigger noise, which is a separate dice roll.

---

## 3. Phases

### H1 — Switch autopilot to Josie semantics

`internal/plugin/dnd_expedition_autopilot_harvest.go`

- Delete `autoHarvestPerNodeAttempts`, `isRarePlus`, `RarePending`, `renderRarePendingFooter`, and the `stopRareNode` stop reason.
- Inner loop becomes: for each node, roll once per remaining charge; each roll consumes a charge regardless of outcome; aggregate yields/fails.
- Per-room footer keeps the existing "+2 Iron, +1 Sage; 3 fails" shape — just driven by the new tally.
- Walk-end summary stays as-is; the totals roll up the same way.
- Noise interrupts: continue rolling per-attempt (parity with manual). A noise interrupt still does threat++ and lets the rest of the node's charges roll.
- Combat interrupts (Standard/Elite/Patrol): keep current hard-stop behavior.
- No SU surcharge change — autopilot still free.

**Files touched:** `dnd_expedition_autopilot_harvest.go`, plus any test that asserts on `RarePending` / `stopRareNode` (sweep `internal/plugin/*_test.go` for those strings).

**Definition of done:** existing autopilot tests pass with the new semantics; a Rare node in a walked room is consumed in-place, not stashed for manual takeover.

### H2 — Add auto-harvest to `!zone advance`

`internal/plugin/dnd_zone_cmd.go` + a small adapter

- `advanceOnceWithOpts` currently treats Exploration rooms as harvest-free. Wire a harvest pass into the Exploration resolution path so that walking into an Exploration room runs the same Josie auto-harvest.
- The catch: `autoHarvestRoom` today takes an `*Expedition`. A zone run is **not** an expedition — `getActiveZoneRun` returns a `*ZoneRun` with a different shape (no `ZoneID`-on-expedition, different room representation).
- Two options:
  - **(a)** Refactor `autoHarvestRoom` to take an interface that both `*Expedition` and `*ZoneRun` can satisfy (zone_id + current_room + threat hooks). Cleanest long-term.
  - **(b)** Write a thin `autoHarvestZoneRoom` that mirrors the autopilot loop but reads from `*ZoneRun`. Faster to ship, duplicates ~50 lines.
  - **Recommendation:** (a). The duplication risk is real and the interface is small.
- Foreground `!zone advance` and autopilot-driven `!zone advance` (compact mode) use the **same** harvest pass — no special casing.
- Elite/Boss/Trap rooms still don't harvest. Entry doesn't harvest. Only Exploration rooms run the pass.

**Definition of done:** entering an Exploration room via `!zone advance` produces a harvest footer identical in shape to the expedition autopilot one; no manual `!forage` needed mid-zone-run.

### H3 — Retire manual harvest commands

`internal/plugin/dnd_expedition_harvest.go` (`handleHarvestCmd`) + the bot dispatch wiring

- Remove `!forage`, `!scavenge`, `!mine`, `!fish`, `!essence`, `!commune` from the command surface entirely. They become no-ops with a deprecation DM: *"Harvest is automatic now — just walk. (`!expedition run` or `!zone advance`)"*
- After two weeks of soak (operator's call) the deprecation DM gets deleted and the commands fall through to "unknown command."
- Sweep `!help` / class help text / zone help text for references to manual harvest verbs.
- Per memory: keep TwinBee voice; surface verbs not jargon.

**Why retire and not keep as a fallback:** the entire point of the redesign is to delete the grind-to-success behavior. Keeping manual commands reintroduces exactly that loop ("the auto roll whiffed, let me type `!forage` and re-roll until it pops"). A clean retirement is more honest than a vestigial command.

**Definition of done:** manual harvest verbs are dead weight; `!help` no longer mentions them; the zone/expedition flow is the only path to yields.

### H4 — Calibrate economy

`gogobee_class_balance.md` references the Monte Carlo harness. Reuse or extend it.

- The Josie nerf: expected yield per node drops by `(1 - success_rate)`. For a 60% node, that's a 40% material cut.
- Run a baseline sim: snapshot per-expedition yield by zone tier under current rules, then under Josie rules.
- Tune **charge counts** first (probably bump the 1–3 range to 2–5) before touching success rates or DCs. Per the difficulty memo: lift trailers, don't nerf leaders.
- Re-run the sim and compare against the pre-change baseline. Target: median yield within 90–110% of today's median; variance allowed to widen.
- Update `dnd_resource_registry.go` charge fields per the calibration result.
- Document the calibration math in this file's appendix so we can audit it later.

**Definition of done:** Monte Carlo shows yield parity within ±10% across tiers; rare nodes feel rare (success rate visibly < 50%) but not punishing across a full expedition.

**⚠ Blocked on Phase J (below).** First hardened-sim sweep showed yield is gated by survival, not charge counts: Fighter/Mage TPK at T3+, no class extracts T5 boss. Tuning charges before fixing survival would over-tune against a strawman. Run J0–J3 first, then this phase.

**Shipped 2026-05-17 (post-J).** Uncommon/Rare MaxCharges bumped 1 → 2 (53 resources). Yield deltas vs J2b baseline: −2.5% to +4.7% across the four cleared zones — within band but with much less leverage than expected. Full write-up in `sim_results/h4_findings.md`: the binding constraint on per-room yield is interrupt-truncation, not charge count. The Common consolation bracket (delta ≥ -4 yields +1) already absorbs most of the Josie nerf. Future yield-tuning effort should target `resolveCombatInterrupt` rates, not charges. H4 closes.

### H5 — Partial spell-slot refresh on short rest

`internal/plugin/dnd_rest.go` + `internal/plugin/dnd_spells.go`

Same QoL theme as the harvest changes: casters today get exactly one slot reload per real-world day (long rest, 24h cooldown + 8h lockout). Short rest is HP-only. That's the actual reason mages feel anemic mid-expedition.

**Behavior change:**

- Short rest gains a partial slot refresh alongside the existing 1d6+CON heal and the hit-dice-charge spend.
- Formula (starting point — calibration runs alongside H4):
  - Refresh **all** slots of level 1.
  - Refresh **floor(character_level / 4)** additional slots at the next-available tier, lowest-first.
  - Examples:
    - L1–3 caster: all L1 slots back.
    - L4–7 caster: all L1 + 1 slot at L2 (or next-available tier if L2 already full).
    - L8–11 caster: all L1 + 2 slots in tiers ≥2, lowest-first.
    - L12+: scales up; capped naturally by the slot table.
- Lockout, hit-dice cost, and HP heal all unchanged.
- Long rest still does a full slot wipe-and-restore (no change there).

**Implementation:**

- Add `partialRefreshSpellSlots(userID, c)` in `dnd_spells.go` that walks the slot table lowest-to-highest, restoring per the formula above. Pure SQL — find slots with `used > 0`, decrement.
- Call it from `handleDnDShortRest` after the HP/charge update, before save.
- Expedition camp (`dnd_expedition_camp.go:285`) already does a full `refreshSpellSlots` — leave it alone; camp is effectively a long rest in disguise.

**Footer:** short rest DM gets one more line: "_Spell slots restored: 2 (L1), 1 (L2)._" when applicable. Suppress the line when the caster has zero refreshable slots (martials, casters at full).

**Files touched:** `dnd_rest.go`, `dnd_spells.go`, short-rest test(s).

**Definition of done:** a level-5 mage who burns all their L1 slots can short-rest, eat the 1h lockout, and come back with L1s topped off plus one L2 back. Martials see no DM change. Long rest behavior is byte-identical to today.

**Open question for H5:** per-short-rest cap — do we cap total slots refreshed per short rest (e.g. at most 4) to prevent very-high-level casters from short-resting back to nearly full? Probably yes, but the cap should only bite at L16+ where the formula goes silly. Settle during H4 calibration when we have sim numbers.

---

## 4. Phasing rationale

H1 first because it's the smallest behavior change on the smallest surface and it's the prerequisite for everything else.

H2 next because the parity gap is glaring once H1 ships: expeditions auto-harvest, zones still make you type. We should not soak that mismatch in production.

H3 only after H1+H2 are stable — players need a week or two of the auto flow before we delete the manual escape hatch, otherwise the rollback story is harder.

H4 runs in parallel with H1 (the sim is independent of the code change) but lands after H1 so we can calibrate against real Josie behavior. Bumping charge counts before H1 ships would just make today's grind faster.

H5 is independent of H1–H4 and can ship at any point — the harvest changes don't touch the rest/slot code paths. Sequenced last only because the harvest sweep is the primary driver of this branch; if a caster player surfaces sooner, promote H5.

---

## 5. Open questions

- **Per-room noise budget under Josie**: a 3-charge node now means 3 noise rolls instead of 1–8. Effective noise rate per node could spike. Decide in H1 whether to cap noise rolls per-node or accept the new rate.
- **Yield quantity per success**: keep current per-resource qty rolls (varies 1–N) or simplify to flat +1 per charge? Recommended: keep current — variance is good theater.
- **Rare-only feedback**: should a Rare+ success ping with a special line ("✨ Rare yield!") to compensate for the deleted pause UX? Cheap to add, makes the score feel earned.
- **Manual-cmd retirement runway**: 2 weeks of deprecation DMs is a guess. Operator's call based on player chatter.

---

## 6. Phase J — Class survival & T5 boss wall

H4 calibration surfaced a bigger problem than charge counts. The hardened expedition-sim (race mods + tier-appropriate gear + lean consumables) shows a sharp class-survival cliff that has to be fixed before charge-count tuning can be trusted.

### 6.1 Why this is here

Sim sweep (450 runs, classes × levels × zones, n=10) at L12 with kitted gear + 2 heals + 1 buff:

| Class | T1 goblin | T2 forest | T3 manor | T4 underdark | T5 dragons |
|-------|-----------|-----------|----------|--------------|------------|
| Fighter L12 | 100% clr | 90% clr | **0%** | **0%** | 0% |
| Mage L12    | 100% clr (14.2 mean yld, best cell) | **0%** | **0%** | **0%** | 0% |
| Rogue L12   | 100% clr | 100% clr | 40% clr | 90% clr | 0% |

Three problems, three threads. Per [feedback_difficulty_target](.claude/memory/feedback_difficulty_target.md): lift trailers, don't nerf leaders or push monster scaling. Per [feedback_accessibility_over_dnd_crunch](.claude/memory/feedback_accessibility_over_dnd_crunch.md): fixes surface as outcomes/verbs, not new dice math.

### J0 — Baseline corpus

Before any tuning, freeze a reproducible baseline:

- Re-run the hardened+consumables matrix at n≥30/cell to tighten variance. The 10-run sweep showed ±20pp noise on mixed cells; we need ≤5pp resolution to detect 10pp movement.
- Persist `sim_results/baseline_j0.jsonl` in-repo as the comparison anchor.
- Extend `sim_results/summarize.sh` with "% clears", "p50 yield among cleared", and "boss-reached %" columns.

**Definition of done:** a single shell script reproduces the baseline in one command; numbers in the table above are within ±5pp of the next sweep at the same n.

### J1 — Fighter T3+ wall

**Hypothesis menu** (cheapest to verify first):

1. **AC plateau.** Fighter AC stops scaling at plate (T3 armor); T3+ monster Attack outpaces it.
2. **No damage-mitigation passive.** Rogue has Cunning Action; Cleric/Druid have heals; Fighter has just HP.
3. **Second Wind missing.** 5e Fighter has Second Wind (bonus action: heal 1d10+lvl, recharges short rest). Grep `secondWind|SecondWind` to confirm.
4. **MaxHP scaling per level too flat.** Fighter d10 HP should already be best-in-class; verify `computeMaxHP`.

**Investigation steps:**

- Add an "encounter trace" to the sim that logs per-round Attack, AC, HP for player and enemy.
- Cross-check `applyClassPassives` for Fighter: what does Fighter actually get at L7, L11 beyond ExtraAttacks?
- Compare Fighter mid-fight HP curve at T3 vs Rogue's. If Fighter takes 1.5x damage per turn → mitigation gap. If they kill at the same rate but get out-paced → AC/Defense gap.

**Likely levers** (priority order, conditional on investigation):

- **Add Second Wind**: short-rest-rechargeable self-heal (1d10 + level). Restores the 5e identity beat without pushing monster scaling.
- **Action Surge** (5e gives at L2 — verify; might be missing). Extra attack burst once per short rest.
- **Subclass crit-range expansion** for Champion. Already partially wired (`dnd_subclass_combat.go:447`). Verify it fires for the sim character.
- *Last resort:* MaxHP rider per level for Fighter only.

**Definition of done:** Fighter L12 manor clears jump from 0% to **≥50%** at n=30; T2 forest clears stay ≥80% (don't break the leader); T4 underdark clears non-zero.

### J2 — Caster boss-survival cliff

**Scope (reframed 2026-05-17 after post-J1 n=100 sweep, `sim_results/baseline_j1_all10.jsonl`):** five caster classes — **mage, cleric, sorcerer, warlock, bard** — all cluster at 19–22% L12 mean %clr across the 5 zones, vs four martial leaders (fighter/ranger/paladin/rogue) at 70–80%. They reach the boss room 62–82% of the time but TPK there. So the gap is **boss-fight burst/durability**, not zone traversal.

Druid sits between the bands at 39% L12 clr (88% boss-reach) — the only mid-band class — because Wild Shape gives a real HP buffer. That's the diagnostic: the trailers lack a durability or burst lever that pulls them through the final fight.

**Cell snapshot (L12, %clr | %boss):**

| Class    | goblin (T1) | forest (T2) | manor (T3) | underdark (T4) | dragons (T5) |
|----------|-------------|-------------|------------|----------------|--------------|
| mage     | 100\|100    | 0\|99       | 0\|63      | 0\|56          | 0\|68        |
| cleric   | 93\|93      | 0\|87       | 0\|70      | 0\|8           | 0\|50        |
| sorcerer | 100\|100    | 0\|97       | 0\|57      | 0\|38          | 0\|60        |
| warlock  | 100\|100    | 7\|100      | 0\|71      | 0\|62          | 0\|78        |
| bard     | 100\|100    | 8\|100      | 0\|70      | 0\|66          | 0\|74        |
| *druid (ref)* | *100\|100* | *93\|100* | *0\|74* | *90\|100* | *0\|78* |

Cleric is doubly broken: lowest boss-reach (62% mean) **and** lowest clear (19%) — pure-support kit doesn't carry solo expeditions even before the boss.

**Hypothesis menu (cheapest first):**

1. **Damage falls off a cliff at mid-level boss HP.** L12 boss HP grows ~linearly; cantrip damage scales (Fire Bolt 2d10 at L11) but slot-spell burst is gated by 2–3 high-level slots used earlier in the zone. Trace `EnemyHPEnd` at TPK across the 5 classes vs druid.
2. **No durability backstop.** Druid (Wild Shape ≈ +temp HP pool) clears 39%; the five trailers have no equivalent. Mage's `Shield` reaction and `Mage Armor` may already exist but aren't firing in turn-engine boss rooms — verify.
3. **Slot/resource economy.** L7/L12 casters reach boss with slots spent; H5 partial short-rest refresh (already planned) is the obvious lever and might solo-fix several of the five.
4. **Cleric class identity.** Bottom-of-band even at boss-reach. Whatever differentiated cleric from "AC 16 melee with no Extra Attack" probably isn't wired into turn engine (e.g. Channel Divinity, Spiritual Weapon auto-cast).

**Investigation steps:**

- Pull per-round combat traces from boss rooms for mage/cleric/sorc/warlock/bard at L12 manor and underdark. Look at: did a slot spell ever fire? did a reaction fire? what was player HP when boss died vs when player died?
- Grep `Shield\b|MageArmor|SpiritualWeapon|ChannelDivinity|EldritchBlast|HealingWord` for existing wiring; check whether each fires from `combat_turn_engine.go` paths.
- Cross-check whether the J1 [[project_j1_turn_engine_fix]] turn-engine seam exposed any *caster* wiring that was previously only firing in `SimulateCombat`. If yes, that's the cheapest fix: same pattern.
- Compare druid L12 underdark trace (90% clr) to cleric L12 underdark (0% clr / 8% boss-reach) — what's druid doing that cleric isn't?

**Findings (2026-05-17 trace sweep, `sim_results/j2_findings.md`):** zero slot casts and zero mid-fight consumable uses across 240 boss-room combats. `autoResolveCombat` dispatches `!attack` only — the entire caster-cliff diagnosis was built on a strawman where casters couldn't cast and didn't quaff heals. Druid's outperformance is its `ThornLashDmg` passive firing on the weapon-attack path, not Wild Shape durability (Wild Shape isn't wired into autoResolveCombat at all). Reaction-spell hypothesis is also moot: `dnd_spells.go` skips reaction-cast spells globally — no reaction window exists yet. Hypothesis 3 (slot economy) is confirmed-but-inverted: casters reach boss with slots intact because the sim refuses to spend them.

**Reframed levers (replaces the original menu):**

- **J2a — Teach `autoResolveCombat` to cast and consume.** Before each `!attack`, mirror the prod player decision: heal at low HP if a heal consumable is in inventory; otherwise cast the highest-impact available slot/cantrip; fall back to weapon swing. Class-blind first cut is fine — pick by damage-vs-remaining-enemy-HP, with a small heal-trigger at player HP < ~40%. Surface: `expedition_sim.go:530`. Heal-side mirrors `combat_bridge.go`'s `SelectConsumables`.
- **J2b — Re-baseline.** Re-run the n=100 all-class corpus with the fixed `autoResolveCombat`. The current `baseline_j1_all10.jsonl` cell numbers do not measure caster boss-survival; they measure "caster forced to swing a stick". Only after J2b do we have a real read on whether a class-level lever is needed.
- *Deferred until after J2b's real read:* shared boss-room cushion, cleric-specific intervention, H5 short-rest refresh wired to J2. Don't pre-commit balance changes against a strawman.
- *Out of scope:* reaction-spell wiring — the reaction phase doesn't exist; lifting that ban is its own surface (post-J2).
- *Avoid:* per-class HP/AC stat riders. Per accessibility-over-crunch, prefer surfacing a verb (cast, ward, channel) over inflating numbers.

**Definition of done (reframed 2026-05-17):**
- **J2a:** at least one of {slot cast, cantrip cast, mid-fight heal-consumable} fires in ≥80% of boss-room fights for every caster class in a small (n≥10) verification sweep. ✅ **MET** — 100% cast rate in `sim_results/j2a_findings.md`.
- **J2b:** new `sim_results/baseline_j2a_v2_all10.jsonl` (n=100). 3 of 5 trailer casters cleared the floor: mage 49% / sorcerer 47% manor at L12 (>40%). Warlock 34%, bard 2%, cleric 9% — these three are now spell-pool-bottlenecked, not sim-artifact-bottlenecked. Writeup: `sim_results/j2b_findings.md`.
- *No martial leader cell drops by more than 10pp from `baseline_j1_all10.jsonl`* — ✅ **MET** (all +0.4 to +6.2pp). Required adding `simMartialFirstClass` after a v1 sweep showed Ranger regressed -35.8pp when the picker burned L3 slots; the gate skips cast-mode for half-caster martials.

**J2c — tried + reverted 2026-05-17.** Both proposed widening levers regressed: heal-spell preempt cost druid 10pp (slot heals are 10HP vs 40HP consumables); control-spell scoring at 22 cost warlock 6.6pp (hypnotic_pattern beat vampiric_touch wrongly), and at 5 the level-first sort made it inert. Picker reverted to the J2b shape. Bard/cleric trailing is a prod-level `defaultKnownSpells` problem (their damage rosters cap at L2), not a sim-picker problem — separate decision. Corpora `baseline_j2c_all10.jsonl` and `baseline_j2c_v2_all10.jsonl` retained for the post-mortem in `sim_results/j2b_findings.md`.

T5 dragons_lair stays J3's problem — J2 isn't on the hook for it. (J2a will partially overlap J3 hypothesis 1, which already flagged this exact sim-artifact for T5.)

### J3 — T5 boss universal wall

**Hypothesis menu:**

1. **Sim auto-resolve unfair.** `autoResolveCombat` (expedition_sim.go:277) loops `!attack` only — no consumable mid-fight, no rest, no spell cast for casters. Real players can do all of these.
2. **Boss HP pool too deep.** Dragons_lair boss is multi-phase; auto-attack loop can't keep up with regen / phase shifts.
3. **No party scaling.** Sim is one character; T5 boss may be tuned for parties.

**Investigation steps:**

- Add an "actions taken" counter to `autoResolveCombat` — confirm it never fires anything but attack.
- Hand-resolve one L12 rogue dragons_lair boss fight to see whether a player using consumables wins. If yes → sim artifact. If no → over-tuned for solo.
- Check `gogobee_expedition_difficulty.md` Phase 5-C reconciliation: that doc said dragons_lair was in-band, but the sim says nobody clears it.

**Likely levers** (depending on investigation):

- **If sim artifact:** harden `autoResolveCombat` to mirror `combat_bridge.go`'s `SelectConsumables` + spell-cast paths. Re-test.
- **If over-tuned for solo:** scale boss HP / damage by party size. The difficulty memo allows lifting trailers without pushing monster scaling — *reducing* boss scaling at low party counts is in-bounds.
- **If by-design raid content:** document it. Mark dragons_lair as "needs party" in the zone registry and surface that in `!zone` help. Per accessibility-over-crunch, the surface is a verb-style warning ("Bring friends — this dragon eats solo adventurers"), not a numeric DC.

**Definition of done:** at least one configuration (class + party + consumables) shows ≥30% extract or clear at L13+ dragons_lair in the sim. OR documented as raid-content with player-facing surface.

### J4 — Re-run validation matrix

After each of J1/J2/J3 lands:

- Re-run hardened+consumables n=30 matrix.
- Diff vs J0 baseline. Report deltas in `sim_results/j_phase_diff.md`.
- Watch for regressions in leader cells (rogue T2/T3 dropping). Lifting trailers must not nerf leaders.

**Definition of done:** all three phases meet their per-phase DoD at n=30, and no leader cell drops by more than 10pp.

### Phasing rationale (J phases)

J0 first — n=10 noise floor is too high to read 10pp movements; without a tighter baseline we'll chase ghosts.

J1 shipped 2026-05-17 (commit 519964f) — turn-engine Extra Attack wired. J2 is now reframed (above) from "Mage walls" to a five-class caster boss-survival problem spanning mage/cleric/sorc/warlock/bard; investigation may share a lever or split into per-class fixes after the per-round trace. Cleric likely needs its own intervention regardless.

J3 can ship in parallel with J2. If sim-artifact, fix is in expedition_sim.go and doesn't touch balance. If a real wall, depends on J2 outcomes — a more durable caster reaching the boss room with slots intact might already reach the boss-doorway more often.

J4 is the gate. Don't merge a class buff without the validation matrix showing the leader didn't regress.

After Phase J converges, return to H4 with a meaningful baseline.

### Open questions (J phases)

- **Sim party scaling.** Sim character = solo player, party of 4, or band? Production is solo. Tune for solo or we make group play trivial. Document the choice.
- **Subclass coverage.** Sim builds vanilla-class synthetics (no subclass). Subclass passives at L3+ probably matter a lot — particularly for the five J2 trailers where the subclass often *is* the survival kit (e.g. Bard College of Valor's extra attack, Cleric domains' bonus-action burst). Pick a "best" subclass per class for the sim or sweep across subclasses in a separate pass before pre-committing J2 levers.
- **Mage Armor as auto-buff.** Player-side spell they cast, or a passive we toggle? Auto-toggling preserves accessibility-over-crunch but removes a (small) tactical decision. Probably worth auto-toggling and noting it in per-class help.
- **Boss consumable refresh.** At T5 elite gates, should the autopilot offer a "rest and re-stock" prompt before the boss? Mirrors real-game tension; closes the sim/prod gap.

### Out of scope for Phase J (explicitly)

- Reworking the d20 combat engine or threat-tier scaling.
- New zone content or content-tier changes.
- Touching loot tables (that's H4, post-J).
- Subclass redesign — only verifying existing subclass passives fire.
- Multi-character party mechanics — single-char tuning only.
