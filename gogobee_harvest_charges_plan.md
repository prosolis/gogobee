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

### J2 — Mage T2+ wall

**Hypothesis menu:**

1. **No defensive layer.** Mage AC = 10 + DEX (no armor proficiency). At T2+ monster Attack, 70%+ hit rate every round.
2. **Slot economy.** L7 mage has 4 L1 + 3 L2 + 2 L3 slots — enough for one elite, not a manor's worth.
3. **No Shield spell auto-cast.** 5e Mage gets `Shield` (reaction: +5 AC until next turn). Verify it exists and fires.
4. **Mage Armor not active.** `Mage Armor` (+3 AC for 8h) should be a pre-walk default; check whether the autopilot starts it.

**Investigation steps:**

- Grep for `MageArmor|ShieldSpell|mageArmor`. If they exist, why doesn't the sim cast them?
- Trace a single mage L7 manor run: does the character ever cast a defensive spell?
- Check `dnd_spells.go` for any auto-cast pre-combat hook the sim could opt into.

**Likely levers:**

- **Wire Mage Armor as a long-rest auto-buff** when not already active and the mage has spell slots. Production characters benefit too.
- **Add Shield-spell reaction** in combat: if available slot + about-to-be-hit + AC < threshold, burn a L1 slot for +5 AC. Reaction-based survival lever 5e mages depend on.
- **Slot economy:** H5 partial short-rest refresh (already in this plan) helps mage longevity directly. May land alongside J2 if it's the cleanest fix.

**Definition of done:** Mage L12 forest clears ≥60%; manor clears non-zero; T1 yield density doesn't drop more than 10%.

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

J1 and J2 are independent and can ship in either order. J1 is probably easier (Second Wind is a well-understood beat); J2 has more design surface (auto-cast policy, Shield-spell reactions). If only one ships, J1 has the cleaner DoD.

J3 can ship in parallel with J1/J2. If sim-artifact, fix is in expedition_sim.go and doesn't touch balance. If a real wall, depends on J1/J2 outcomes — a buffed Fighter or Mage might already reach the boss-doorway more often.

J4 is the gate. Don't merge a class buff without the validation matrix showing the leader didn't regress.

After Phase J converges, return to H4 with a meaningful baseline.

### Open questions (J phases)

- **Sim party scaling.** Sim character = solo player, party of 4, or band? Production is solo. Tune for solo or we make group play trivial. Document the choice.
- **Subclass coverage.** Sim builds vanilla-class synthetics (no subclass). Subclass passives at L3+ probably matter a lot for Fighter and Mage. Pick a "best" subclass for the sim or sweep across subclasses in a separate pass.
- **Mage Armor as auto-buff.** Player-side spell they cast, or a passive we toggle? Auto-toggling preserves accessibility-over-crunch but removes a (small) tactical decision. Probably worth auto-toggling and noting it in per-class help.
- **Boss consumable refresh.** At T5 elite gates, should the autopilot offer a "rest and re-stock" prompt before the boss? Mirrors real-game tension; closes the sim/prod gap.

### Out of scope for Phase J (explicitly)

- Reworking the d20 combat engine or threat-tier scaling.
- New zone content or content-tier changes.
- Touching loot tables (that's H4, post-J).
- Subclass redesign — only verifying existing subclass passives fire.
- Multi-character party mechanics — single-char tuning only.
