# Combat engine — paying off the N-body debt

## Session 2026-07-11 (c) — §6 was never a balance problem

**IN FLIGHT: the verification sweep is still running on millenia** (`/tmp/s6_after.jsonl`,
10 classes × L10,L12 × 10 zones × n=25). Nothing below the diagnosis is measured yet.

### Committed

- `c9128fb` — §1's other half. Out-of-combat `!cast <heal> @friend`, scoped to the
  people on your expedition. `--target` had been advertised by `!help` and swallowed
  by the parser since SP2. Ally row is mutated with one guarded UPDATE in a
  transaction (gifting's precedent), not a read-modify-write under a second lock:
  two clerics healing each other would take their `advUserLock`s in opposite orders
  and deadlock. Refunds the slot on every path that heals nobody.
- §6 auto-cast (`zone_combat_autocast.go`) — see below. Tested, unmeasured.

### The finding: cleric was not weak, it was not being played

Baseline re-run on HEAD (`sim_results/s6_baseline_l10l12.jsonl`, 5000 rows) confirmed
cleric still dead last: **46.4% vs fighter 81.4%**. §1's self-heal moved it ~0, as
predicted. But the *shape* of the failures is the whole story:

| class | cleared | **fled** | tpk |
|---|---|---|---|
| fighter | 407 | **0** | 93 |
| ranger | 395 | **0** | 105 |
| cleric | 232 | **167** | 101 |

**Cleric dies LESS than mage, sorcerer or warlock.** Its entire 35pp deficit is
*fleeing* — 167 of 500 runs end with the player alive and the expedition over.
Instrumenting the three `stopEnded` sites: **every one of those runs died at
`stopEnded via ROOM`** — an ordinary room fight. Not a patrol, not an elite, not the
boss.

**Root.** An ordinary room is `runZoneCombatRoster` → `SimulateCombat` on an 8-round
clock: one breath, **no turn engine, no action picker**. The only spell that could
ever land there was a `PendingCast` the player queued BY HAND with `!cast` before
walking in. On autopilot nobody queues one — so for every room of every expedition, a
caster fought with a weapon and nothing else. A cleric has no Extra Attack, a mace,
and its whole kit unusable. Measured on the room path at L10 (loss% by entry HP):

| entry HP | fighter | druid | bard | mage | sorcerer | **cleric** |
|---|---|---|---|---|---|---|
| 100% | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% | 0.0% |
| 70% | 0.0% | 0.0% | 0.0% | 0.0% | 1.0% | **1.5%** |
| 35% | 0.0% | 0.0% | 0.5% | 0.5% | 10.0% | **8.0%** |

Small per room — and a run is ~16 rooms, and **one loss ends the expedition**
("the fight drags on, X outlasts you, you retreat"). It compounds into 33% of runs.

**The tell.** `dnd_class_balance.go:431` — the harness the class corpus was tuned on
— *always* hands a caster their best damage spell (`pickBestDamageSpell` +
`applyHarnessSpellCast`) before it simulates. So the numbers that say "cleric is a
weak class" modelled a cleric who casts; the live room modelled one who does not.
The corpus and the game disagreed about what a cleric is.

This is the same defect as everything else in this plan: **the action model is
narrower than the kit.** §1 (the picker never healed), §3 (the companion never
acted), Pete (`LoadDnDCharacter` → nil → `"attack"`), and now every caster in every
room. It is not a tuning dial and it must not be fixed with one.

### The change (unmeasured)

`autoCastForAutoResolve` — no queued spell + a real slot in hand → cast the best
damage spell, spend the slot, fold it in. Additive pre-damage, exactly as a
hand-queued `PendingCast` has always been, so it stays comparable to the corpus
rather than being a fresh mechanic.

- **Slot-aware, not free.** `pickBestDamageSpell` reads `slotsForClassLevel` — the
  *theoretical* class table. Reusing it here would be an infinite spell: the same
  "no row to persist onto, so it arrives fresh" bug that gave the companion an
  unlimited body ([[project_companion_free_lunches]]). The new picker reads the
  seat's actual remaining slots and `consumeSpellSlot`s.
- **Never upcasts.** The big slots are what the elite and the boss are for, and the
  turn engine spends them there. A picker that nukes a goblin with a 5th-level slot
  leaves the caster swinging a stick at the thing that matters.
- Cleric owns no damage spell at slot 2 or 4, so those stay unspendable in rooms —
  correct, and pinned by a test.
- Both goldens byte-identical (they pin the engine; this changes its *inputs* at the
  zone layer). Martials provably untouched.

### When the sweep lands

Read cleric's **fled** count, not just its clear rate — fled going to ~0 is the
thing this predicts. Expect all 6 casters to move; that is the deliberate
re-baseline. **Watch for overshoot on bard/druid** (already 67–72%): they get the
same buff and were not the problem. If they overshoot, the lever is this picker, not
monster scaling ([[project_difficulty_target]]).


## Session 2026-07-11 (b) — the companion can cast, and three free lunches fell out

**Committed: `01c2cb2` (§1 — the seat-scoped spellbook).** Everything below it is
working-tree.

The revisit-Pete step ("Pete cannot cast") turned out to be four separate defects
stacked on each other. The unit tests were green for all four; only the sweep with
a **control arm** ever told the truth. Read [[feedback_unit_tests_miss_engine_bugs]]
and then read it again.

### The one that was asked for

Every spell lookup is keyed on a user id and answered by a `dnd_*` table. Pete has
rows in none of them, by design. So `pickAutoCombatActionForSeat`'s first line —
`LoadDnDCharacter(uid)` → nil → `return "attack"` — meant **a hired Cleric swung a
mace, every turn, forever**. Role-fill hands a lone martial a Cleric, so that was
the *common* case. Fixed with a seat-scoped spellbook (`combat_seat_spells.go`):
humans delegate to the DB verbatim (both goldens byte-identical), the companion is
answered from his synthetic sheet.

### The three that were not

Each of these was found by measuring, and each is the same mistake: *"he has no row
to persist onto, so he arrives fresh."*

1. **His spell slots refilled every fight.** I parked the ledger on his combat
   *seat* — and a seat is per-session. A human rations one pool across a 30-room
   run and gets it back at camp; rationing it **is** the caster's game. Moved to
   `expedition_party.companion_slots_used`, refreshed at camp.
   *(Worth ~0pp on its own — an expedition only holds ~2 real fights, so the pool
   never binds. Correct, but not the lever. I predicted this one would be the whole
   answer and I was wrong.)*
2. **His body refilled every fight.** `combat_party_start.go` seated him at
   `player.Stats.MaxHP` and the close-out skipped him with the comment *"he arrives
   fresh next time"*. That is an infinite body: he soaked a share of every fight's
   incoming and then reset, while the humans beside him bled all the way to camp.
   **This was the carry.** Moved to `expedition_party.companion_hp`; healed at camp.
3. **No autopiloted caster has ever healed itself.** `simPickAllyHeal` skipped
   `i == seat` and bailed on `!IsParty()`. Between them: a solo cleric carries
   `cure_wounds` for a whole run and dies with a full pool of level-1 slots. Now
   `simPickHeal`: heal whoever is worst off, which is sometimes you.
   **It is NOT the answer to §6, and I guessed that it would be.** It moved solo
   66.1% → 66.2%: the picker reaches for a healing *consumable* first and the sim
   stocks them, so a human almost never falls through to the spell. The corpus is
   therefore undisturbed and no re-baseline is owed. Pete carries no consumables,
   so it is his only heal — which is the reason to keep it.

### What the arms actually said

640 runs/arm, 10 classes × L10,L12 × 4 zones. "Like-for-like" = the leaders whose
role-fill gives Pete a **Cleric**, compared against a human **Cleric** follower.

| arm | like-for-like clear | vs solo |
|---|---|---|
| solo | 68.2% | — |
| + a human cleric follower (leader's level, geared) | 79.7% | +11.5pp |
| + Pete, HEAD (free heal, cannot cast) | 77.1% | +8.9pp |
| + Pete, casting + free heal | **94.8%** | +26.6pp — **carry** |
| + Pete, casting + carries his wounds | **69.5%** | +1.3pp — **useless** |

**The reference arm is the whole point.** Measured against *solo*, even mace-only
Pete looked like a carry (+8.9pp) — but parties are *designed* to be safer, so solo
is the wrong yardstick. Measured against **a human follower**, the real finding
appears: a gearless, level-penalized hireling was out-clearing a fully-geared human
cleric *of the leader's own level* by 15pp. A hireling must never beat a player.
His party fled 5 runs out of 640; the human party fled 56.

### §2(a) SHIPPED — and it landed in band

`055f07d`. Seats carry a **SeatWeight**; both levers (boss HP, enemy action economy)
scale on the summed weight of the **living** seats instead of a head count. The
weight is **level-based** — priced against the leader, times a hireling discount
(`companionSeatWeight = 0.65`, the one tunable).

**Not** a power score: HP×damage would rank a cleric below a fighter and quietly
make every mixed *human* party easier — a difficulty regression smuggled in under a
bug fix.

The safety argument is one property: **a peer weighs exactly 1.0.** The curves
interpolate between P8's tuned integer knots — (1, 1.0), (2, 2.4), 2n−1 from 3 up —
so every integer input returns exactly what it always returned. Solo byte-identical,
a party of same-level humans byte-identical, both goldens unmoved. Only an *unequal*
roster lands between knots, which is the whole point. It also finishes §2(b): a
downed seat now buys the enemy nothing (§2(b) fixed only the head-count half — a
corpse still carried its full weight).

| | solo | + Pete | + human cleric peer |
|---|---|---|---|
| like-for-like clear | 69.0% | **76.8%** (+7.8pp) | 77.6% (+8.6pp) |

| band | solo | + Pete | lift |
|---|---|---|---|
| trailing (solo <40%) | 10.0% | 31.0% | **+21.0pp** |
| middle | 58.9% | 76.8% | +17.9pp |
| leading (solo ≥70%) | 93.5% | 99.2% | **+5.7pp** |

Help, never a carry — and he stays below a real human of the leader's level, which
is the invariant a hireling must never break.

### How it got there (the record of being wrong)

With all three free lunches gone, the same grid says (like-for-like subset):

| | solo | + human cleric peer | + Pete |
|---|---|---|---|
| clear | 69.0% | 77.6% (+8.6pp) | **66.1% (−2.9pp)** |

**Hiring him is now worse than going alone** — and that is §2, unmasked. The free
full-heal had been paying his enemy-scaling bill for him. A below-median seat costs
the boss +15% HP and lifts the enemy's action economy from 1 to 2.4 attacks a round,
and he does not give that much back.

I talked myself out of §2(a) mid-session, on the grounds that discounting a weak
seat would enlarge the carry. That was right *about the buggy build* and wrong about
the engine: with the infinite body removed, the plan's original diagnosis is exactly
correct and the sweep now argues **for** §2(a).

I talked myself **out** of §2(a) mid-session, on the grounds that discounting a weak
seat would enlarge the carry. That was right about the *buggy build* and wrong about
the engine: the infinite body had been paying the seat's scaling bill for it. With
the free lunches gone, the plan's original diagnosis was exactly correct. **The
order mattered** — §2(a) done first, on top of the free full-heal, would have made
things worse and looked like it was working.

## Still open

- A dropped companion returns at 1 HP (`companionSeatHP`'s floor) rather than being
  benched until camp. Deliberate: there is no companion-death rule and inventing one
  inside a bug fix would have been a second feature.
- **§6 is still open, and self-heal was NOT it.** Solo cleric is 28–34% against
  fighter's 100% on this grid. Re-read it off the current corpus before tuning.
- `companionSeatWeight = 0.65` is the single tunable and was not swept — it landed
  in band on the first value tried. If Pete ever needs to be weaker, raise it.
- Out-of-combat `!cast --target` (`dnd_cast.go`) is still self-only.
- **Deployable now.** Hiring him is a real, bounded help at every level.


> The party engine (N3) widened the *roster* from 1 to N but never widened the
> *action model*, the *scaling model*, or the *test net* to match. Everything
> below is a symptom of that one gap. Working-tree doc; do not commit
> (feedback_dont_commit_plan_mds).

## Status @ 2026-07-11 — §1–§5 SHIPPED (uncommitted, unmeasured)

All five sections are implemented and the suite is green. Nothing is committed.

| § | What | State |
|---|------|-------|
| §5 | Party golden + seat attribution | ✅ `party_characterization.golden` (1938 lines, 9 scenarios × 5 seeds) + `TestPartyCharacterization_OneSeatIsStillSolo`; sim trace gets `simTraceEvent` (seat always emitted). Wire format left alone — `TestP5Fields_StayOffSoloRows` was right and I was wrong. |
| §3 | Engine-driven seats | ✅ `ActorStatuses.EngineDriven`, permanent, no command clears it. `beginCombatTurn` now REFUSES a command from an engine seat. `autoDriveCombat` calls `driveEngineSeat` instead of impersonating the seat. |
| §2(b) | Living-seat action economy | ✅ `enemyActionsThisRound` counts `livingActors(st)`, not `len(st.actors)`. **Party golden moved deliberately** (survivor ends 59 HP, was 51). Solo golden byte-identical. |
| §4 | One roster accessor | ✅ `expeditionParty()` / `partyHumans()` / `PartySeat{Kind}`. Cannot return an empty party for a solo run — that is the level-1-companion bug, pinned by `TestParty_SoloExpeditionStillHasAParty`. |
| §1 | Ally-targeted actions | ✅ `turnActionEffect.AllyHeal/AllySeat`; `!cast cure wounds @alex` **and** `--target @alex` (the flag `!help` advertised and `parseCombatCast` silently swallowed since SP2). `simPickAllyHeal` so autopiloted/engine healers use it. Will not raise the dead. |

**Both goldens hold. Solo is byte-identical throughout — the balance corpus is
intact.**

### Post-fix sweep (2026-07-11, n=750/arm on millenia)

| | clear |
|---|---|
| solo | 48.5% |
| solo + hired Pete | **63.9%** (+15.3pp) |
| solo *(the 2-human cells)* | 65.7% |
| \+ a human cleric follower | **87.7%** (+22.0pp) |

Pete went from **−28pp (worse than nobody) to +15.3pp**, and the lift lands exactly
where the plan asked for it:

| band | n | mean lift |
|---|---|---|
| trailing cells (solo <40%) | 15 | **+28.0pp** |
| middle (40–70%) | 3 | +5.3pp |
| leading cells (solo ≥70%) | 12 | **+2.0pp** |

That is "help, never a carry", measured rather than asserted: he rescues the
players who were drowning and barely moves the ones who were already fine. §2(a)
may now be unnecessary — the §2(b) fix appears to have been most of it.

**⚠️ READ THE NOISE FLOOR BEFORE TRUSTING ANY CELL.** The sim is not seeded per
cell. The same binary, same cell (bard/L10/feywild, n=25), three times:
**36% / 56% / 56%** — a 20pp spread from RNG alone.

- **Per-cell numbers at n=25 are noise.** Do not tune against them.
- Only the **n=750 aggregates** carry signal. The solo arm's 51.9% → 48.5% drift
  across engine versions is inside that noise band (and the solo golden is
  byte-identical, so solo combat provably did not change).
- Anything huge (the old fighter/L10/feywild 100% → 4%) is real; anything under
  ~20pp on a single cell is not.
- **Next sweep: raise `-runs` to 100+ per cell** if per-cell reads are needed.

### What is NOT done

- **Measurement.** The engine moved under every number in this doc. A full
  re-sweep (solo / solo+Pete / 2-human-with-a-cleric) is in flight; every figure
  quoted below predates §1–§5 and is now **stale**.
- **§2(a)** — power-based enemy scaling. §2(b) only stopped corpses from buffing
  the boss. A *weak* living seat still costs a full seat's worth of enemy scaling,
  which is why hiring Pete could still hurt a strong leader.
- **§6** — solo cleric bump. Blocked on the re-baseline, deliberately.
- **Pete cannot cast.** `castActionForSeat` loads a sheet from the DB and he has
  none by design, so a hired "Cleric" *still* cannot heal. §1 fixed human clerics;
  the companion needs a synthetic spell pool (or to be restricted to martial
  chassis). **This is the revisit-Pete step.**
- Out-of-combat `!cast --target` (`dnd_cast.go`) is still self-only. Combat is
  where it mattered; that one can follow.

## Why this plan exists

The Pete companion (`!expedition hire`) was built in one session and worked on
every unit test while being, in the sweep, **worse than no companion at all**
(solo 65% clear → solo+Pete 33%). Chasing that found four defects. Only one of
them was Pete's. The rest were sitting in the engine, in prod, since N3:

| Symptom found via the companion | Who else it affects | Root |
|---|---|---|
| A hired Cleric can't heal anyone | **Every human cleric in every party** | §1 |
| A weak seat makes the party *worse* | Any under-levelled friend; every downed seat | §2 |
| The companion lost his turn after round 1 | Any engine-driven seat, incl. away-player autopilot | §3 |
| The companion was silently level 1 | Anything reading "the party" of a solo run | §4 |
| Nobody noticed any of it | — | §5 |

§5 is the one that matters most: **there is no party golden.** The
characterization golden (`combat_characterization.golden`, 7468 lines) runs
`simulateCombatWithRNG(player, enemy)` — one player, one enemy, every scenario.
Party combat has *no* pinned behaviour, so N3 shipped a healer class that cannot
heal and the corpus said nothing. Whatever else we do, that gets fixed.

**Sequencing:** §5 first (build the net), then §1–§4 (change the engine while the
net is under us). Doing it the other way round means changing the most heavily
tuned code in the repo with no way to see what moved.

---

## §1 — Actions cannot target another seat

**The bug.** Every heal in the engine is self-scoped. `MistyHealProc`/
`MistyHealAmt` heal the actor. `HealItem`/`HealItemCharges` fire the actor's own
sub-50% trigger. `grep` for an ally-targeted action returns nothing; the only
seat-targeting that exists anywhere is `enemyTargetSeat` — which seat the
*monster* swings at. A party cleric is a cleric in chassis and passives only:
**they cannot put one hit point back on a friend.**

**Root.** `PlayerAction{Kind, Effect}` (`combat_turn_engine.go:51`) has no target
field. It was written when "the player" was unambiguous, and N3 never revisited
it — the roster got wider, the action stayed pointed at the same two things (me,
and the enemy).

**Change.**
- `PlayerAction` gains a target: `TargetSeat int` (−1 = the enemy, the default,
  which keeps every existing call site meaning exactly what it means today).
- `turnActionEffect` grows an ally-heal payload; `runPartyCombatRound` applies HP
  deltas to `TargetSeat` rather than to the actor.
- `!cast <spell> @user` parses a target; solo ignores it.
- Cleric/Druid/Bard/Paladin get their heal spells actually wired to it.
- `pickAutoCombatActionForSeat` gains a heal-the-lowest-living-seat rule, so
  autopiloted and companion healers behave like a competent player.

**This moves the golden.** It adds an action the picker can choose. Deliberate
re-baseline, after §5.

**Risk.** Touching `runPartyCombatRound` is touching the most tuned code we own.
Mitigated by §5's party golden plus the existing solo golden staying byte-identical
(no solo fight has a second seat to target).

---

## §2 — Enemy scaling counts seats, not strength

**The bug.** P8 scales the enemy's action economy to the roster, and
`partyEnemyHPScale` gives +15% HP for any roster ≥ 2. Both count **seats**. So a
seat contributes its full cost to the enemy the moment it sits down, regardless
of what it actually brings — and keeps contributing that cost **after it dies**.

Measured, in the same cell (fighter L10, feywild):

| | clear |
|---|---|
| solo | 100% |
| \+ Pete as Cleric (a chassis he can't play) | 32% |
| \+ Pete as Fighter (his best case) | 68% |
| \+ a second *human* | 100% |

A deliberately below-median body is a **net negative at every level** — the boss
gains more than the seat gives back. That is not a Pete property. It is true of
any under-levelled friend you invite, and of every seat that goes down early
(the corpse keeps inflating the boss for the survivors).

**Root.** The scaling model assumes every seat is a median player. Nothing
enforces that, and two shipped features (parties, then companions) violate it.

**Change — pick one, and it is a design call, not a mechanical one:**
- **(a) Scale on contributed power, not seat count.** Enemy HP/actions scale on
  the roster's summed effective level relative to the leader's. Correct, and it
  fixes the under-levelled-friend case too. Most work; needs its own sweep.
- **(b) Scale on *living* seats, re-derived per round.** Cheap, and kills the
  dead-seat-still-inflates-the-boss bug on its own. Does not fix the
  weak-seat case.
- **(c) Don't scale for engine-driven seats at all.** Cheapest; makes hiring
  strictly positive. Risks "carry", and leaves the human-party half untouched.

Recommendation: **(b) now** (it is a straight bug — a dead man should not be
buffing the boss), then **(a)** as the real fix once §5 can see it.

---

## §3 — Engine-driven seats are a status flag, not a concept

**The bug.** A seat with nobody at the keyboard is modelled as
`ActorStatuses.Autopilot = true`. But `beginCombatTurn` clears that flag on any
command arriving from that seat — *"They typed, so they are here."* And
`autoDriveCombat` drives a party by **dispatching each seat's turn as a fake chat
command from that seat's Matrix ID**. So the autopilot driver's own move looked
like the player coming back, cleared the latch, and the seat went inert for the
rest of the fight.

That is what made the companion stand in fights doing nothing. The same shape is
live for away-player autopilot today, and it is only luck that a human eventually
types something and re-latches.

**Root.** There is no first-class notion of "this combatant is driven by the
engine". Ownership of a turn is inferred from a Matrix message, which is a
transport detail leaking into the combat engine.

**Change.**
- A seat is `DrivenBy: human | away | engine`, persisted, not a bool that any
  command can clear. `engine` is permanent; `away` is what a keystroke clears.
- `autoDriveCombat` stops impersonating seats. It calls the seat driver directly
  (`driveCompanionSeat` is the shape; generalize it to any non-human seat).
- Command handlers refuse any non-human seat outright, rather than half-working.

**Bonus.** This is the prerequisite for the deferred Phase-16 "Pete runs his own
expeditions", and for any future NPC ally.

---

## §4 — "the party" and "the roster" disagree

**The bug.** A solo expedition has **no `expedition_party` rows** (absence means
solo; the roster materializes on the first invite). So any code that answers
"who is in this party?" by reading the roster gets **nobody** for a solo player
— and a plausible-looking fallback silently takes over. That is exactly how the
companion got hired at **level 1** for every solo player: the one case the
feature exists for.

**Root.** Two representations of the same fact ("who is on this expedition") that
disagree in the solo case, with no single accessor. `partySize()` and
`partyMembers()` and `expeditionAudience()` and `expeditionSeats()` and
`fightRoster()` all answer slightly different questions and are easy to reach for
by the wrong name — I reached for the wrong one twice while building the
companion, once fixed by a test and once only by a 1500-run sweep.

**Change.** One accessor that always includes the owner —
`expeditionParty(expeditionID) []Member` with `Member.Kind = leader|member|companion`
— and every consumer derives its own view from it (mail excludes companions,
seats include them, supply burn counts mouths). Delete the ad-hoc set.

---

## §5 — There is no party golden (do this first)

**The bug.** `TestCombatCharacterization` pins *solo* combat, exhaustively (57
scenarios × seeds). Party combat has nothing. The entire N3 engine — initiative,
seat order, enemy targeting, action economy, close-out — is unpinned. That is why
a healer class that cannot heal shipped without a single test going red, and why
the companion's collapse was invisible until a 1500-run sweep.

**Change.**
- `TestPartyCharacterization` + `party_characterization.golden`: N-seat rosters
  (2 and 3), mixed classes, downed seats, an engine-driven seat, seeded and
  byte-pinned exactly like the solo one.
- A **balance regression sweep** in CI-able form: the `expedition-sim` matrix at
  low n, asserting clear-rates stay inside a band. The sweep is what caught all
  of this; it should not have to be run by hand.
- Fix event seat attribution: `CombatEvent.Seat` is `omitempty`, so seat 0 and
  "no seat" are indistinguishable in a trace — which cost real time during this
  investigation, sending me after a phantom "Pete never swings" bug that was
  partly a reporting artifact.

---

## §6 — Clerics are weak in solo (raised 2026-07-11)

Separate from the party gap, and real: the class corpus has cleric in the 46–56%
band against fighter's ~82% ([[project_d8prereq_baseline]]).

**Do not tune this yet, and specifically do not tune it in isolation.** Two
reasons:

1. §1 *is* the cleric fix in party play — an ally-targeted heal is the class's
   entire identity, and today they cannot use it on anybody. Whatever solo gap
   remains should be measured **after** that lands, or we buff them twice.
2. `d8prereq_corpus` predates parties entirely and the party engine has moved
   twice today (§2b, and §1 next). It is a stale baseline. **Re-baseline first**,
   then read the cleric gap off the new corpus.

Then, if solo cleric is still short: lift the trailer, per the standing rule
([[project_difficulty_target]]) — do not nerf the martial leaders and do not touch
monster scaling.

## Ordering

1. **§5** — build the net. Party golden + seat attribution. No behaviour change.
2. **§3** — engine-driven seats. Contained, no balance impact, unblocks the rest.
3. **§2(b)** — living-seat scaling. Straight bug fix; party golden will move once,
   deliberately.
4. **§4** — unify the roster accessor. Mechanical, guarded by §5.
5. **§1** — ally-targeted actions + heals. The big one. Moves both goldens;
   re-baseline the class corpus after ([[project_d8prereq_baseline]] predates
   parties entirely and is due anyway).
6. **§2(a)** — power-based scaling, then re-sweep the companion and re-decide his
   below-median premise. It may not need to survive: once the boss stops
   overcharging for him, "below median" may be exactly right.

## Meanwhile: the companion

He is **fixed and net-positive where he is meant to be** (+15.7pp in trailing
cells, ~neutral overall), but he still regresses strong solo leaders (§2) and
role-fill still hands martials a Cleric who cannot heal (§1). Two options until
this plan lands:

- **Ship him martial-only** (he plays chassis he can actually use — measured
  32% → 68%), and let §1 restore the healer fill later. Honest, cheap, no engine
  change.
- **Hold him** until §1/§2 land, and announce the rest of Adventure without him.

Do NOT bandaid by hand-tuning his stats until he "looks right" — his numbers are
not the problem, and tuning them would bake the engine's bugs into his stat block.
