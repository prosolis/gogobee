# Code review follow-ups — `n1-restoration` (2026-07-10)

Review scope: `git diff main...HEAD`, 99 files / ~16.7k lines (N1, N2, R1, R2, N3 P1–P7).
Method: 8 finder angles → dedup → 1 verifier per correctness candidate.
Baseline before and after: `go build`, `go vet`, `go test ./internal/...` all green.

Working-tree doc — **do not commit** (per project convention on `gogobee_*.md`).

**Status.** Fixes 1–5 shipped in `1f21156`. Deferred items **A** and **B** fixed
2026-07-10 (`d76c63b`); A turned out to have a deeper root cause and surfaced
three new findings, **M**, **N** and **O**. Item **C** fixed 2026-07-10 — its
premise was wrong and the bug underneath it was worse (the seven-day resume
window was never enforced; see below). Item **M** fixed 2026-07-10 — two of its
three claims were wrong (the mage hooks *do* run on the turn path); the one real
defect, Grim Harvest, had a different cause than recorded. Items **D**, **I**,
**L**, **N** fixed. Item **K** verified 2026-07-10 — the anchor gap is a real,
confirmed narrowing, left as an owner design call (no code change). Item **H**
measured 2026-07-10 and **declined** — its "per-message cascade" premise was
overstated, the hot path is already optimized, and the proposed
`MessageContext` thread-through is disproportionate churn with a staleness
regression surface.

**All correctness work is committed.** Everything remaining is deliberate, not a
defect: **E** (intentional, roster-size death rule), **F** (`grantAutorunGrace`
stopgap, deferred to R5 by design), **G** (party-combat DB chatter — a perf
refactor whose only safe fix is a fight-lifetime roster cache, i.e. the same
staleness surface H was declined for; deferred), **H** (declined, above),
**J** (`!sell` anchor — PLAUSIBLE, presence == any chat, left per convention),
**K** (owner design call, above), **O** (already fixed as a side-effect of A;
only the sim re-baseline is pending, which is operational).

**Method note.** Both C and M were mis-diagnosed in the same way: the finder read
one call site, concluded "this is the only caller", and wrote it down. Verify the
callers before planning the fix.

**Third pass (2026-07-10, `88c5fcd`).** `/code-review high --fix` over the pushed
follow-up stack (`git diff origin/main...HEAD`, 7 commits). Five fixes shipped;
they harden the item-C extraction work and the item-N Misty ordering rather than
opening new ground. See "Third pass" below.

---

## Before you commit: split the formatting from the fixes

A repo-wide `gofmt -w ./internal ./cmd` ran after the review. It reformatted
**105 files that have nothing to do with these findings** — they were already
non-conforming on `main` (struct-field alignment, mostly), and gofmt is
semantics-preserving, so nothing changed behaviourally.

Net state: **117 files dirty, only 13 carry review fixes.** Commit them apart, or
the fixes become unreviewable inside a wall of realignment.

The 13 fix-bearing files:

```
internal/plugin/adventure.go               *  internal/plugin/dnd_expedition_supplies.go  *
internal/plugin/adventure_arena_season.go     internal/plugin/dnd_zone_cmd.go             *
internal/plugin/combat_cmd.go                 internal/plugin/expedition_party.go
internal/plugin/combat_party_turn.go          internal/plugin/expedition_party_cmd.go
internal/plugin/dnd_expedition.go          *  internal/plugin/expedition_party_resolve.go
internal/plugin/dnd_expedition_combat.go      internal/plugin/zone_combat_party.go
internal/plugin/combat_engine_party_test.go
```

Plus one new file: `internal/plugin/zone_combat_party_casualty_test.go`.

The four marked `*` carry **both** a fix and gofmt realignment
(`adventure.go`, `dnd_expedition.go`, `dnd_expedition_supplies.go`,
`dnd_zone_cmd.go`), so a clean split needs `git add -p` on those rather than a
straight per-file split. The other nine are fix-only and can be staged whole.

---

## Fixed in this pass

| # | File | Defect |
|---|------|--------|
| 1 | `combat_party_turn.go` | Settle-to-terminal dropped the entire close-out |
| 2 | `expedition_party_cmd.go` | Party member permanently soft-locked on leader extract |
| 3 | `expedition_party_cmd.go` | Lost update on the shared supply pool |
| 4 | `dnd_expedition_combat.go` | Elite harvest interrupt granted no elite loot |
| 5 | `adventure_arena_season.go` | Transient failure permanently lost a season crown |

Plus cleanups: deleted dead `partySurvivors`; collapsed the `zoneCombatRoster`
alias into `fightRoster`; `partyCasualtyLine` now uses `joinNames`; consolidated
four copies of the expedition column projection into `expeditionSelectCols`;
corrected two false doc comments on `applyDailyBurn`/`applyDailyBurnP`; `replyDM`
no longer sends a blank DM.

### 1. Close-out dropped when the settle turns lethal — the worst one

`beginCombatTurn` calls `settleCombatSession` to resolve a phase the engine owes
(a fight parked mid enemy-turn after a restart). That settle can end the fight.
The old code then saw `!sess.IsActive()` and returned "you're not in a fight."

The terminal status was already persisted, so nothing ever paid the party out:
no XP, no loot, no `markAdventureDead`, no `abandonZoneRun`, no
`forceExtractExpeditionForRunLoss`. The reaper cannot rescue it either —
`listExpiredCombatSessions` filters `status = 'active'`, and the session is now
terminal. The win or the death simply evaporates.

The `!fight` start path and the reaper both already do settle-then-`closePartyRound`;
`beginCombatTurn` was the one settle site that skipped it. Now it closes out and
announces, matching them.

### 2. Member soft-locked when the leader extracts and walks away

`seatedExpeditionFor` (the guard) spans `status IN ('active','extracting')`.
`expeditionForMember` (what `!expedition leave` resolved through) filtered
`status = 'active'` only. During the leader's 7-day extracting limbo a member was
therefore refused any new adventure by the guard, and told "No active expedition"
by the very command the guard pointed them at. Nothing sweeps stale `extracting`
rows, and only the *leader* can trigger the transition that clears them — so if
the leader quit, the member was stuck forever with no self-service recovery.

`!expedition leave` now resolves the seat through `seatedExpeditionFor`, so the
exit sees every state the gate sees. (`seatedExpeditionFor` already excludes
leaders, so they still fall through to the `!extract` message.)

### 3. Lost update on the shared supply pool

`updateSupplies` rewrites `supplies_json` wholesale. `expeditionCmdAccept` folded
a new member's packs onto an `exp` snapshot read ~60 lines earlier, with no lock.
Handlers run one goroutine per event (mautrix `AsyncHandlers`, never overridden),
so two invitees accepting at once genuinely interleave: both read pool `P`, one
writes `P+a`, the other `P+b` — a member's packs vanish, or spent SU is
resurrected by the day-burn tick.

Note `advUserLock` cannot fix this: it is keyed by sender, so two members take
two different mutexes. Added `advExpeditionLock(expID)` and re-read the pool
under it. **This closes accept-vs-accept only** — see deferred item B.

### 4. Elite harvest interrupt paid standard loot

`runHarvestInterrupt` computed `elite := kind == InterruptElite`, used it to pick
an elite enemy and to pick elite narration, then passed a hardcoded `false` as
`isElite` to `closeOutZoneWin`. Since `dropZoneLoot` gates on `isBoss || isElite`,
beating an elite interrupt skipped the masterwork roll and used the 1.0 standard
treasure weight instead of 2.0 — while the identical elite fought via `!zone`
paid out correctly.

Now passes the live `elite`. The param only flows into `dropZoneLoot`, so this
adds the masterwork roll and doubles treasure weight for surviving seats; threat,
XP, and narration are untouched. A small reward buff on a fight players already
win, consistent with lifting trailers rather than nerfing.

### 5. A failed season crown was lost permanently, not retried

`arenaSeasonRollover` logged-and-`continue`d when `recordArenaSeasonTitle`
failed, then called `db.MarkJobCompleted` unconditionally. `JobCompleted`
short-circuits every future run for that quarter, so a transient SQLite `BUSY`
meant the crown was never awarded — ever.

`recordArenaSeasonTitle` is idempotent (`arena_season_titles` has
`PRIMARY KEY (season, kind)`, insert is `ON CONFLICT DO NOTHING`) and a past
season's data is frozen, so retrying is safe. The job is now marked complete only
when no crown failed. The "no entrants" path still closes the season correctly.

---

## Fixed in the second pass (2026-07-10) — items A and B

### A. Turn-based combat skipped achievements and post-combat subclass state

The root cause was one level deeper than this doc originally recorded, and it is
worth writing down because the surface symptom was misleading.

`buildZoneCombatants` used to call `applyArmedAbility`, which applies an ability's
mods **and then clears `c.ArmedAbility` and saves the sheet**. The turn engine
calls that same builder again on *every* `!attack` / `!cast` / `!consume`. So in a
turn-based fight:

- Round 1's build fired the ability, spent the resource, cleared the arm flag.
- Round 2+ rebuilt the character, found `ArmedAbility == ""`, and produced a
  combatant with **none of the ability's mods**.

A Berserker paid stamina and got exactly one round of `BerserkerRage`,
`RageMeleeDmg`, `PhysicalResistRage`, `FrenzyDmgBonus`. Every entry in
`dndActiveAbilities` had the same shape. `mods.BerserkerRage` was not merely
unread at close-out — by then it no longer existed.

Fixed by splitting arming into its two halves:

- `consumeArmedAbility(c)` — the mutation. Disarms, saves, returns the id. Runs
  **once**, at fight start.
- `applyAbilityByID(c, id, mods)` — pure. No DB write, no disarm. Safe on every
  rebuild. (No ability's `Apply` writes to the character, so this is genuinely pure.)
- `armAbilityForFight(c, mods)` — consume + apply, for the auto-resolve callers
  that build and fight in one breath.

`buildZoneCombatants` now takes the already-consumed `armed` id and re-applies it.
The id rides on `ActorStatuses.ArmedAbility`, seeded per seat at fight start
(`seedActorOneShots`), so `partyCombatantsForSession` reproduces the ability on
every rebuild and the close-out can still see that a rage fired.

On top of that, the close-out itself: `postCombatBookkeeping(...)` in
`combat_bridge.go` now carries the achievements + subclass persistence, and all
four close-outs route through it — `runDungeonCombat`, `runZoneCombatRoster`,
`finishCombatSession`, `finishPartyCombatSession`. The turn-based pair reach it
via `postCombatBookkeepingForSeat`, which rebuilds the `CombatResult` the hooks
need from the persisted session (`seatCombatResult`) and re-derives the
fight-start mods (`seatFightStartMods`).

It fires on **every** terminal status, not just a win — a Berserker who rages and
loses is still exhausted, which is what auto-resolve always did.

Also fixed in passing: `buildFightSeats` and `runZoneCombatRoster` consumed the
armed ability *before* the checks that could sit a seat out, so a downed member
was disarmed for a fight they never joined. The refusals now run first.

**Balance note.** Turn-based Berserkers now keep rage for the whole fight instead
of one round. That is a buff, but it is the buff the player already paid for.
The class-balance corpus measures the auto-resolve path, so it is unmoved.

New tests: `internal/plugin/combat_armed_ability_test.go` (9 cases) —
purity/repeatability of `applyAbilityByID`, once-only consume, rage surviving
rebuilds, the sat-out member keeping their ability, per-seat statuses isolation,
and exhaustion on both a manual win and a manual loss.

The Grim Harvest half of this item was closed separately — see item **M**.

### B. The other six supply writers raced

All six now go through `withExpeditionSupplies` (`dnd_expedition.go`), which takes
`advExpeditionLock(expID)`, re-reads the expedition, hands the closure the fresh
row, and persists what it returns:

`dnd_expedition_cycle.go` (`nightRolloverBurn`, forage + burn in one write),
`dnd_expedition_milestone.go` (`grantTwoWeeksCache`),
`dnd_expedition_region_cmd.go` (`advanceToNextRegion` transit burn),
`dnd_expedition_camp.go` (`campPitch`), `expedition_autocamp.go`
(`pitchAutopilotCamp`), `expedition_ambient.go` (pack-rat drain).

Fix #3's hand-rolled lock in `expeditionCmdAccept` was folded onto the same
helper. Seven call sites, no nesting — verified none of the closure callees
re-enter the lock (`sync.Mutex` is not reentrant).

`expedition_sim.go:1421` is left alone on purpose: the sim is single-threaded and
takes no plugin locks.

The atomic-`json_set`-delta alternative is still the better long-term shape (it
would drop the lock entirely) but it couples `updateSupplies` to the blob format.

---

## Third pass (2026-07-10, `88c5fcd`) — `/code-review high` on the follow-up stack

Reviewed `git diff origin/main...HEAD` (the 7 pushed follow-up commits) with 8
finder angles → dedup → 1 verifier per candidate. The removed-behavior angle came
back empty: the close-out refactor (item D), `parseMenuIndex` (item I), the
GrimHarvest stash (item M), and the `endRunOnLoss`/`applyOwnerWinEffects`/
`grantSeatWinXP` extractions all verified behavior-preserving. Findings clustered
in the new item-C extraction guard, plus one ordering nuance in the item-N Misty
wiring. Five fixed:

1. **The extraction guard fell open on a DB error.** `expeditionCmdStart`'s
   resumable-guard did `switch n, err := partySize(pending.ID)` and gated the
   party-blocking refusal on `case err == nil && n > 1`. A transient roster-read
   error skipped both cases, fell through, and started a new expedition on top of
   the still-seated party — the exact orphaning the guard exists to prevent, in
   the one situation (a DB hiccup on the roster read) where it matters. Now checks
   `extractionLapsed` first (pure, no DB call on the reap path) and treats a
   `partySize` error as "assume occupied → refuse". Also drops the wasted `COUNT`
   the old switch-init ran on the lapsed-reap path.

2. **The start-path lapsed reap unseated members silently.** When a leader with a
   lapsed party extraction ran `!expedition start`, the reap called
   `completeExpedition(...Failed)` — which frees the roster — with no DM, unlike
   the hourly sweeper and `!expedition abandon`, which both notify. Extracted
   `reapLapsedExtraction(e)` (reap + notify the audience) as the single
   reap-with-notify path; the sweeper and the start-path reap both route through
   it, so a member is never silently unseated by whichever path reaches the row
   first.

3. **Misty's crowd swung before the concentration pulse.** Item N's new
   `stepRoundEnd` seat-loop ran `seatEndOfRound` (crowd revenge, which can end the
   fight) *before* the pre-existing concentration-aura tick. Concentration is
   turn-engine-only (no auto-resolve counterpart), so this was a within-engine
   ordering call, not a cross-engine divergence — but a caster whose lingering
   aura would kill the enemy that round could be dropped by crowd revenge first,
   contradicting the concentration block's own "a lethal pulse settles the fight"
   intent. Moved the Misty seat-loop to *after* the concentration tick: a lethal
   pulse now wins via `finish(CombatStatusWon)` before the end-of-round crowd
   swing; on any round the pulse doesn't end the fight, both procs fire exactly as
   before. (Owner-approved reorder — it was reported as a balance call and the
   answer was "move it".)

4. **Fourth copy of the event-log scan.** `seatCombatResult` hand-rolled a
   `misty_heal` scan loop. Promoted the test-only `hasAction(events, action)`
   helper to production (`combat_session_build.go`), used it there, and removed the
   duplicate test definition.

5. **New `dnd_`-prefixed file.** Renamed `dnd_expedition_extract_sweep_test.go` →
   `expedition_extract_sweep_test.go`, per the no-new-`dnd_`-names rule (the diff's
   three other new test files already avoided the prefix).

**Reported, not fixed** (deliberate):

- **Misty ordering** was the only correctness-shaped finding; the other angles
  agreed the stack was clean. Two lower items were left: `dnd_setup.go`'s
  respec/wipe disbands an extracted party with no member DM — but the disband is
  intended (item C hole #3) and the missing DM is outside the reviewed lines; and
  `abandonExpedition` re-queries the row the caller already resolved — the
  divergence a finder posited isn't reachable (the extracted-row branch only runs
  when no active row exists), so the extra indexed SELECT was left alone.

Full plugin suite green before and after. No auto-resolve path touched, so the
class-balance corpus and `combat_characterization.golden` do not move.

---

## Deferred — real, not fixed

### ~~M. Mage subclass spell hooks never run in turn-based combat~~ — TWO-THIRDS MIS-STATED. Fixed 2026-07-10.

**The premise was wrong.** `applyMageSubclassSpellHooks` has three non-test
callers, not one. `resolveTurnSpell` (`dnd_spell_combat.go:341`) has called it
since Phase 13 shipped per-round casting (`5cd343a`, 2026-05-14).

So on the turn-based surface:

- **Empowered Evocation** (L7, +INT to evocation damage) — *always worked.*
- **Evocation Overchannel** (L10, ×1.5 on a 1st–5th-level slot) — *always worked.*

Both only move `mods.SpellPreDamage`, which `resolveTurnSpell` returns as
`out.EnemyDamage`. Nothing was lost.

**Grim Harvest was the only real defect**, and its cause was narrower than "the
hooks don't run": the hooks ran, wrote `mods.GrimHarvestSlot` — and
`resolveTurnSpell` dropped that local `CombatModifiers` on the floor, because
`turnSpellOutcome` had no field to carry it out. A Necromancy Mage who killed
with a spell in a manual fight never healed.

The stash cannot ride on fight-start mods the way it does in auto-resolve: the
spell is cast *mid*-fight, and the turn engine rebuilds its combatants every
round. So it rides where every other mid-fight fact rides — the casting seat's
`ActorStatuses`:

1. `turnSpellOutcome` gained `GrimHarvestSlot` / `GrimHarvestNecrotic`.
2. `castActionForSeat` parks them on `actorStatusesPtr(seat)` when a cast lands.
   `snapshotActor` starts from the prior snapshot, so they survive `commit()`
   untouched, exactly as `ArmedAbility` does.
3. `seatFightStartMods` reads them back at close-out, in place of the comment
   that used to explain why they were permanently zero.

**And the killing-blow check was wrong for this surface.** `grimHarvestHeal`
scanned for the **first** `spell_cast` event and refused the heal if the enemy
survived it. Auto-resolve casts once, pre-combat, so first == last there; a
turn-based mage casts every round, so an opening cantrip that left the enemy
standing vetoed the heal the round-3 killing spell had earned. It now scans for
the **last** `spell_cast` — provably identical on the auto-resolve path, which is
why the golden corpus does not move.

Each damaging cast overwrites the stash, so it always describes the seat's most
recent landed spell — the only one that can have been lethal. A miss stashes
nothing, and a stale stash is inert because the last `spell_cast` event then
shows the enemy alive.

**Balance note.** This is a caster buff on the manual surface — 2×/3×/4× slot
level in HP, once, on a spell kill, for L5+ Necromancy Mages. It is the buff the
subclass was written to have and auto-resolve already paid out. Consistent with
the standing "lift trailers" stance. The class-balance corpus measures
auto-resolve and is unmoved.

New tests: `internal/plugin/combat_grim_harvest_turn_test.go` (7 cases) — the
stash surviving `resolveTurnSpell`, nobody-else-stashes, last-cast-not-first,
weapon-kill-after-a-spell, the `snapshotActor` round-trip, the end-to-end heal at
`postCombatBookkeepingForSeat`, and per-seat isolation in a party fight.

### N. The NPC procs do not exist in the turn engine (RE-VERIFIED 2026-07-10 — conclusion holds, evidence corrected, scope wider) — the debuff exploit fixed 2026-07-10.

**Verified before planning**, after items C and M both turned out to rest on a
bad "only one caller" claim. This one survives, but two of its three sentences
did not.

**The `CrowdRevengeProc` exploit is now closed** (see the fix note at the end of
this item). Closing it pulled `MistyHealProc` in with it: both are round-end
effects, and the honest hook runs the pair rather than the debuff alone — a
one-sided hook would have been a second parallel sibling of the kind item D
warns about. So Misty's **heal** is now live on the turn path too. That is a
player-favourable discovery mechanic and fits the "lift trailers" stance, so it
ships together; only `SniperKillProc` stays turn-dead, because it is a
pre-combat one-shot with no round-end seam to hang on.

**Correction 1 — the location was wrong.** `MistyHealProc` is not read inside
`simulatePartyWithRNG`. It is read at `combat_engine_party.go:520` inside
`endOfRoundForSeat`, a *helper*, which is exactly the shape that made C and M
wrong. So the question had to be asked properly: `endOfRoundForSeat` has one
caller, `simulatePartyRound` (`:418`), which is the auto-resolve round. The turn
engine runs its own `stepRoundEnd` and never calls it. Conclusion stands.
`SniperKillProc` (`:102`, genuinely in `simulatePartyWithRNG`, a pre-combat
one-shot) has no turn-engine counterpart either.

**Correction 2 — the procs are built, then ignored.** `buildZoneCombatants` calls
`DerivePlayerStats` (`combat_session_build.go:61`), which sets
`mods.MistyHealProc` / `mods.SniperKillProc` (`combat_stats.go:180,184`) on every
turn-based combatant. The mods are present on the struct every single round. No
turn-engine code path reads either field. It is a dangling write, not a missing
build.

**Correction 3 — there is a third proc, and it cuts the other way.**
`CrowdRevengeProc` (`:473`) sits in the same `endOfRoundForSeat` and is equally
absent from the turn engine. That one is the Misty **debuff**. So the asymmetry
is not "manual fights are worse": a buffed player loses their buff by fighting
manually, and a debuffed player *escapes their debuff* by doing the same. The
debuff half is the one worth caring about, because it is exploitable and needs no
discovery to exploit — just fight everything with `!attack`.

**Not gaps** — checked and explained, so nobody re-files them:

- `PetAttackProc` and `SpiritWeaponProc` *are* reimplemented in the turn engine
  (`petStrike`, `spiritWeaponStrike`), though they fire after a player action
  rather than at round end.
- `EnvironmentProc` is absent because `turnCombatPhase` is a single flat "Duel"
  phase with the proc at 0 — deliberate, not dropped.
- `HealItem`'s auto-heal never fires, but the turn engine seeds and snapshots
  `HealChargesLeft` and gives the player `!consume` instead. Player-driven by
  design, not a leak. (Worth confirming the charges aren't double-counted across
  the two surfaces.)

**The achievement claim held when written; the fix moved it.** At the time,
`combat_sniper_kill` and `combat_misty_clutch` were both unreachable from a
manual kill and `seatCombatResult` hardcoded both flags false — so "six
achievements is really four" was correct. Wiring the heal changed that:
`seatCombatResult` now reads `MistyHealed` back off the `misty_heal` event on the
log (the same way `combat_pet_save` has always been detected), so
**`combat_misty_clutch` is reachable turn-based now** — it is five, not four.
`combat_sniper_kill` stays unreachable. The other four were always reachable:
`combat_near_death` (computed by `seatCombatResult`), `combat_pet_save` (the turn
engine does call `resolveEnemyAttack`, which emits `pet_whiff`),
`combat_death_save` (`trySave` in `stepRoundEnd`), and `combat_consumable_used`
(`combat_cmd.go:773`).

(Keep this internal. Per project convention these procs are hidden discovery
mechanics and must not surface in help text, player docs, or DMs.)

**Fix (2026-07-10).** Both Misty procs were hoisted out of `endOfRoundForSeat`
into shared helpers — `mistyCrowdRevenge` and `mistyHeal` — and a `seatEndOfRound`
hook that runs the pair in the order the auto-resolve engine does (crowd first,
then, if the seat survived, the heal). `endOfRoundForSeat` now calls the helpers;
the turn engine's `stepRoundEnd` calls `seatEndOfRound` per seat, after the
poison tick so a heal can answer the round's damage. Both helpers short-circuit
before touching the RNG when their proc is unarmed, so a character with no Misty
history draws exactly the dice it drew before — the sim corpus and
`combat_characterization.golden` do not move (full suite confirms).

`renderAllySeatEvent` gained a `crowd_revenge` case: an ally sees the damage land
(silence would read as HP vanishing) but not Misty's name, matching the
neighbouring `misty_heal`'s neutral "recovers N" — the grudge stays the owner's
own discovery.

New tests: `combat_misty_turn_test.go` (6 cases) — the crowd landing on the
manual surface (the exploit), a lethal crowd ending a solo fight, the heal firing
and capping, no heal for a seat the crowd just dropped, the no-RNG-when-unarmed
property that protects the golden file, and an end-to-end wiring test through
`advanceCombatSession` that reports `PlayerHP=40` (the shipped exploit) if the
`stepRoundEnd` call is removed.

### O. `trySimAutoArm` used to re-arm every round — sim baselines will move (NEW)

Before item A's fix, `trySimAutoArm` lived inside `buildZoneCombatants`. On the
turn-based path that meant: every rebuild re-armed the class-default ability,
**spent another point of the resource**, and re-applied it. A simulated Fighter
got a fresh Second Wind heal every single round until stamina ran dry; a Cleric,
a Healing Word.

`expedition-sim`'s `autoDriveCombat` drives `handleAttackCmd`, i.e. the
turn-based path — so this inflated every turn-based sim result. It now arms once
per fight, which is what `simAutoArmEnabled`'s own doc comment says it models
("a competent real player who would `!arm` Second Wind before each fight").

**Any expedition-sim corpus that involved turn-based combat is stale.** The
pending final L10/L12 re-baseline should be run after this change, not before.
The pure auto-resolve corpus (`SimulateCombat` / `simulateParty`) is unaffected —
that path always armed exactly once.

### ~~C. Members and leaders disagree about an expedition during `extracting`~~ — MIS-STATED; the real bug was worse. Fixed 2026-07-10.

**The premise was wrong.** `getActiveExpedition` — the *leader's* lookup — also
filters `status = 'active'`. During `extracting` the leader gets "no active
expedition" from `!expedition`, `!map`, `!camp` &c. exactly as the member does.
They already agree, and honestly so: nobody is standing in the dungeon. Widening
`expeditionForMember` would have made the member the only player who can see a
paused expedition. Not done, and it should not be.

Reading for it surfaced the real defect underneath.

**The seven-day resume window was never enforced.** `releaseParty` fires only on
a terminal status, and `dnd_expedition.go:428` justifies skipping it for
`extracting` by asserting "the roster is cleared when the resume window lapses
and the row flips to `failed`". Nothing did that. The only `extracting → failed`
transition in the tree was inside `handleResumeCmd` — a lazy expiry that runs
only when **the leader personally types `!resume`**.

So a leader who quit, forgot, or simply started a different expedition left the
row `extracting` **forever**. `releaseParty` never ran, every member stayed
seated, and `assertNotAdventuring` kept refusing them a run of their own. Fix #2
gave them `!expedition leave` as an escape, which is why this was a permanent
nag rather than a permanent soft-lock — but a player should not have to find it.

Three holes, all closed:

1. **No sweeper.** `sweepLapsedExtractions` (`dnd_expedition_extract.go`) reaps
   every `extracting` row past `completed_at + 7d` through `completeExpedition`,
   which releases the roster, and DMs the audience. Hourly ticker
   (`expeditionExtractionSweepTicker`), plus a one-shot at `Start()` — a lapse
   that happened while the bot was down is blocking those members *now*. The
   audience is read before the close-out, since `completeExpedition` disbands the
   roster `expeditionAudience` reads. `handleResumeCmd`'s lazy expiry stays, now
   sharing the `extractionLapsed` predicate (which treats a NULL `completed_at`
   as lapsed — it is the only clock the window has).

2. **The leader could orphan their own party.** `!expedition start` checked only
   `getActiveExpedition`, so a leader could start fresh on top of an extracted
   row. `!resume` resolves the *newest* `extracting` row, so the old one became
   unreachable — un-resumable, un-abandonable, roster still held. It now refuses,
   but only when that row has a roster (`partySize > 1`); a solo extraction
   strands nobody, so walking away from one stays a normal thing to do. A lapsed
   row is reaped in place and the start proceeds.

3. **The leader had no way out but to pay.** `expeditionCmdAbandon` resolved
   through `activeExpeditionFor` and so could not see the extracted row it owns —
   the leader had to buy a `!resume` just to abandon. Both it and
   `abandonExpedition` now span `extracting` via `ownedLiveExpedition`, which
   sorts active rows first so `expeditionCmdStart`'s run-spawn rollback still
   tears down the row it just created rather than an older extracted one. This
   also fixes `dnd_setup.go:394,449` for free: a leader who rerolled their
   character used to strand their whole party.

Abandoning now DMs the members too, and says the true thing when the party is
standing in town rather than in the dungeon (supplies already spent, loot
already banked — not "supplies are forfeit").

New tests: `dnd_expedition_extract_sweep_test.go` (6 cases) — the sweep releasing
a stranded roster, leaving a live extraction alone, the NULL-`completed_at`
predicate, abandon reaching an extracted row and freeing the party, abandon
preferring the active row, and the no-live-row error.

Not covered: the two `expeditionCmdStart` / `expeditionCmdAbandon` refusals,
which sit above the DM boundary. Still the `dmSink` gap.

### ~~D. Two turn-based win close-outs must be kept in sync by hand~~ — effect-drift closed 2026-07-10.

`finishPartyWin` (`combat_party_finish.go`) was a hand-copied sibling of the
`CombatStatusWon` block in `finishCombatSession` (`combat_cmd.go`): mood
scan, kill record, threat, boss threat, XP + near-death, loot, TwinBee line,
continue hint. `finishPartyCombatSession` routes solo back to
`finishCombatSession`, so the two must stay equivalent forever, with nothing
forcing it. Item A was the first drift.

Item A's fix already pulled the *bookkeeping* out of both into
`postCombatBookkeepingForSeat`. This pass hoists the remaining duplicated
**effects** into three shared helpers in `combat_party_finish.go`, and routes
both close-outs through them:

- `applyOwnerWinEffects(owner, enemyID, elite) bool` — the zone-kill record,
  room threat, and boss-defeat threat drop; returns `bossOnExpedition`. Fires
  once, owner-scoped (a member owns neither row).
- `grantSeatWinXP(uid, hp, hpMax, monster, tier)` — near-death calc + XP grant.
  Per seat (solo calls it once with seat 0).
- `endRunOnLoss(owner, runID, death)` — mood event (death only) + `abandonZoneRun`
  + `forceExtractExpeditionForRunLoss`, shared by both the Lost and Fled paths.

What deliberately stays divergent, so this was **not** a full `closeOutSeat`
merge: the player-facing text genuinely differs (solo "You finished at N/M HP"
vs party "The party stands"; the party's 4-way boss/hint split vs the solo
2-way), and the death-on-win rule is roster-size-dependent by design — solo
`finishCombatSession` never marks a winner dead, `finishPartyWin` marks a seat
that won from 0 HP dead. That is item E and must not be unified. So the effect
lists are now a single copy each; the text is not.

Full plugin suite green; no behavior change (helpers are extract-and-call).

Note this is the *only* place the party work chose a parallel sibling. Elsewhere
the branch is a genuine N-body generalization, not a bolt-on: `SimulateCombat` →
`simulatePartyWithRNG`, `runCombatRound` → `runPartyCombatRound`, `runZoneCombat`
→ `runZoneCombatRoster`, `RenderTurnRound` → `RenderPartyTurnRound`.

### E. Death-on-win depends on roster size — intentional, leave it

`closeOutZoneWin` marks a downed seat dead on a win only when `len(seated) > 1`.
A solo player who wins at 0 HP (retaliate aura kills the swinger on the killing
blow) survives; a party member in the same spot dies.

This is **deliberate and documented in place** (`zone_combat_party.go:183-188`):
unifying it would move the sim's outcome labels and force a re-baseline of
`testdata/combat_characterization.golden`. Not a defect. Listed so the next
person to find it doesn't "fix" it by accident.

### F. `grantAutorunGrace` is a stopgap (already tracked as R5)

`zone_revisit.go:172` stamps `last_autorun_at` to buy one `autoRunCooldown`
window after a `!revisit`, because the autopilot ticker has no concept of
position-vs-frontier and will otherwise march the party back out of the room they
just backtracked to. The code says so itself and defers the real fix to R5. It is
coupled to the exact value of `autoRunCooldown` and to the ticker cadence.

### G. Party combat multiplies the known per-turn DB chatter

Already tracked as `project_combat_session_cache_deferred`, now worse:

- `partyCombatantsForSession` (`combat_session_build.go:137`) rebuilds every seat
  from scratch on every `!attack`/`!cast`/`!consume` (~6–8 SELECTs per seat).
  A 3-member, 5-round fight ≈ 270–360 SELECTs.
- `rebuildRoster` (`combat_cmd.go:620`) then rebuilds *all* seats again after a
  mid-fight buff, when only the casting seat changed.
- `runZoneCombatRoster` (`zone_combat_party.go:73`) loads char + equipment, then
  `buildZoneCombatants` loads both again and discards the first pair — once per
  seat per room, on the path that fights every room.
- `nudgeStalledPartyTurns` (`combat_party_turn.go:281`) does two session reads
  plus a full N-seat rebuild per stalled fight, serially, on the 1-minute ticker,
  while holding the fight lock.
- The enemy stat block is rebuilt once per seat and kept only for seat 0 (the
  code comments admit it). CPU-only, but pure waste.

Cheapest real fix: memoize the built roster for the fight's lifetime. Mid-fight
buffs are already tracked as `ActorStatuses` deltas and re-applied by
`applySessionBuffs`, so stats are reproducible without re-reading the DB.

### ~~H. Guard probes re-query per message~~ — MEASURED, DECLINED 2026-07-10.

`isPartyMember` → `activeExpeditionFor` is up to 2 queries; `activeZoneRunFor`
adds more. A solo player with nothing in flight costs 3 queries to conclude "not
in a party". The original write-up said this was "repeated by each guard a
handler consults" and proposed resolving seat/expedition/run once per message
into `MessageContext` and threading it.

**The premise was overstated, and the fix is disproportionate. Not done.**

- **No per-message guard cascade exists.** `OnMessage` dispatch is strictly
  one-handler-per-command (`if IsCommand(...) return handle(...)`). Nothing runs
  these resolvers for *every* message; the redundancy is intra-handler only.
- **The one hot inbound path is already optimized.** `zoneCmdAdvance` /
  `expeditionCmdRun` call `isPartyMember` directly *by design* — the code comment
  spells out that this avoids re-resolving the run (which `advanceOnce` is about
  to resolve) and double-firing the §4.3 idle reap. The authors already closed the
  hot case.
- **The remaining 2–3× callers are cold.** A per-function scan found ≥2 resolver
  calls only in `zoneCmdEnter`, `zoneCmdAbandon`, `expeditionCmdStart`, the status
  impls (all one-shot commands), and `runAutopilotWalk` (5×, but it is the
  *background* ticker path, not an inbound message). In those the calls are
  typically a guard-check on one branch then a resolve on another — not the same
  read twice.
- **The proposed fix has a real regression surface.** `MessageContext` is a
  shared cross-plugin value type (movies, vibe, miniflux, …); the resolvers are
  free functions used at 60+ sites. Threading a resolved snapshot means changing
  the shared type and every call site, and carrying a memo that goes stale the
  moment a handler writes the row mid-flight — the exact staleness-bug class items
  A/M/N just fixed. All to save a few cheap indexed local SQLite reads on a
  low-volume bot.

Cost/benefit is upside-down: high churn + a staleness regression surface for a
negligible perf gain on paths that are mostly already optimal. Left as-is
deliberately. Revisit only if a profiler ever shows these reads on a hot path.

### ~~I. `parseTemperIndex` is the third copy of the same menu parser~~ — Fixed 2026-07-10.

`adventure_temper.go` duplicated the "read leading digits, 1-indexed →
0-indexed, bounds-check" reply parser already inlined in `resolveMagicEquipReply`
(`magic_items_gameplay.go`) and `handleMasterworkEquipReply`
(`adventure_masterwork.go`). Promoted the temper copy to `parseMenuIndex(reply, n)`
and routed the other two through it.

Behaviour-preserving on all three: the old `parsed` flag was redundant with the
`idx < 0` guard (no leading digit leaves `idx=0`, then `idx--` → -1, rejected
either way), so no input changes verdict. The **retry disagreement the doc
flagged was deliberately left as-is** — this was a pure dedup, so magic-items and
temper still re-store the pending interaction on a bad parse and masterwork still
doesn't. Unifying that is a behaviour change and belongs in its own pass if
wanted. Full plugin suite green; the existing `parseMenuIndex` table test moved
with the rename.

### J. `!sell` fires the event anchor even when nothing sold (PLAUSIBLE)

`adventure.go:719` calls `maybeFireAnchoredEvent` unconditionally after
`handleSellCmd`, so `!sell nonexistentitem` can burn the player's one daily event
slot. Gating it needs `advSellItem`/`advSellAll` to return a `sold bool`.

**Probably leave it.** The anchor's stated design is presence-based — "a moment
the player is present and reading" — and a player who typed `!sell` and read the
reply *is* present. That matches the project's own rule that presence means any
chat, not a successful command. Flagged only so the choice is explicit.

### K. Confirm the A6 anchor set covers the player population — CONFIRMED REAL GAP 2026-07-10.

N1/A6 replaced the old "any daily-active player, any activity" mid-day event roll
with three presence anchors: the expedition Night digest, `!sell`, and arena
cashout. `tryTriggerEvent`'s `HasActedToday` gate was dropped with it.

**The code confirms the gap is real, not hypothetical.** The three, and only
three, `maybeFireAnchoredEvent` call sites are:

1. `expedition_autorun.go:312` — the end-of-day digest, and it is gated on
   `campDecision.Night`. A Night camp only happens on a **multi-day** expedition;
   a single-day dungeon/`!zone` run never reaches one.
2. `adventure.go:734` — after a `!sell` at Thom's.
3. `adventure_arena.go:633` — after an arena cashout.

So the excluded population is concrete: a player whose daily loop is
mining / foraging / fishing / babysit / single-day dungeon, who never sells at
Thom's, never enters the arena, and never runs a multi-day expedition, receives
**zero** mid-day events — the roll never even happens for them. That is exactly
the core daily-grind loop, so this is plausibly a large fraction of players, not
an edge case.

**Fourth anchor drafted 2026-07-10 (uncommitted, pending owner review).** The
on-theme, farm-resistant home is a foreground **single-day `!zone` clear** — the
climax DM the dungeon/harvest-via-walk player is demonstrably reading, and one
they cannot spam (a clear costs a full walk). Implemented as:

- `advEventChanceZoneClear = 0.08` in `adventure_events.go` (matches the digest;
  it is the day's climax moment, and it is the single tuning knob).
- `advanceResult.zoneCleared` set true only in the *full*-clear branch of
  `advanceOnceWithOpts` (not a mid-zone region clear, which continues the run and
  would fire per region).
- Rolled in `zoneCmdAdvance` after the clear DM streams, via the unchanged
  `maybeFireAnchoredEvent`. That is the foreground path only — autopilot
  (`runAutopilotWalk`) and party members (refused earlier by `isPartyMember`)
  never reach it. The per-day slot guard still caps everyone at one event/day.

Rates (see `TestZoneClearAnchorRate`): a zone-clear-only grind player at ~2
clears/day lands ~1.08 events/week — the same band as the fully-engaged
digest+sell+arena player (~1.19/week, unchanged). The rare all-four-in-one-day
player peaks at a ~1.65/week *probability* ceiling but still gets at most one
actual event/day.

**Two knobs remain the owner's call**, hence "pending review": the 0.08 chance,
and whether a single-day zone clear is even the right anchor for the grind loop
vs. `!resources` (rejected here as farmable — repeated `!resources` would just
re-roll the same daily slot) or leaving the grind loop event-free by design.
Full plugin suite green; no existing behaviour moved (the three prior anchors and
`TestAnchoredEventWeeklyRate` are untouched).

### ~~L. Dead helper: `openParty`~~ — Verified and removed 2026-07-10.

Confirmed dead: a repo-wide grep found `openParty` only in `expedition_party.go`
(the def) and `expedition_party_test.go`. The invite path goes `joinParty` →
`seatLeader`, which runs the same `INSERT ... role 'leader' ON CONFLICT DO
NOTHING` but reads the owner off the expedition row instead of trusting a
passed-in id. Deleted `openParty`; the eleven test call sites now use a
`seatLeaderFixture(t, expID)` helper that wraps `seatLeader` in a transaction
(the owner comes from the `seedExpedition` row, which always matched the id the
old calls passed). `seatLeader`'s doc comment absorbed the idempotency note that
justified `openParty`. Full plugin suite green.

---

## Test gap worth closing — ~~the DM seam~~ CLOSED 2026-07-10

`SendDM` went straight through `Base` to a live client, with no fake/sink seam,
so the handler-level paths behind fixes #1, #2 and #5 could not be unit-tested.
That is why the party close-out bug survived a suite this thorough: every party
test is unit-level and stops below the DM boundary.

**Fixed 2026-07-10.** Added a `MessageSink` interface and a `Base.Sink` field
(`plugin.go`). When non-nil it captures every outbound message — both the DM
route (`SendDM`/`SendDMID`) and the room route (`SendMessage`/`SendMessageID`,
which is what the arena announcement actually uses) — and the live client is
never touched. Nil in production, so behaviour is unchanged; the whole
`internal/...` suite is green with the field added. One seam, no call-site churn:
the 765 `SendDM`/`replyDM`/`SendMessage` sites are all downstream of these four
methods.

New tests (`message_sink_test.go`): a `captureSink` test double + `installSink`
helper; `TestMessageSink_CapturesDMAndRoom` proving both routes divert with the
right target/text and that the `*ID` variants report back an id; and
`TestExpeditionCmdLeave_DMsBothEnds`, which drives the real fix-#2 handler end to
end and asserts both DMs land (member "turned back", leader notified) — a path
that was literally a silent no-op in a unit test before. `beginCombatTurn`'s
terminal path and the arena rollover announcement are now reachable the same way
(seam proven for both the DM and room routes); dedicated end-to-end tests for
those two remain easy follow-ups if wanted.

Added this pass: `TestPartyCasualtyLine_JoinsNamesLikeTheRestOfTheBot`,
and `TestPartySurvivors_SplitsOnHPNotOnOutcome` was rewritten as
`TestAnySurvivor_ReadsHPNotOutcome` when its helper was deleted.

Added in the second pass (`combat_armed_ability_test.go`): the close-out
bookkeeping is now reachable below the DM boundary via
`postCombatBookkeepingForSeat(sess, seat)`, which takes a plain `*CombatSession`
and needs no client — so item A's behaviour is directly unit-tested even though
`finishCombatSession` itself still isn't. (The `dmSink` seam that would have
covered `finishCombatSession` itself now exists — see above.)

Not covered by tests: item B's locking. `withExpeditionSupplies` is exercised
incidentally by the existing expedition suite, but nothing asserts the mutual
exclusion. A `t.Parallel` accept-vs-burn race test under `-race` would.
