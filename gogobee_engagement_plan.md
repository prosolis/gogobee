# Adventure Engagement Plan — new modes, mechanics, story

> Goal: make the Adventure module more engaging across the whole player
> lifecycle — fix the drop systems the Phase R transition orphaned, give
> the endgame a real chase, add player-driven modes (co-op, duels, world
> bosses), thread story through the zones, and surface the social data
> that already exists. Working-tree doc; do not commit (feedback_dont_commit_plan_mds).

## Status @ 2026-07-11

**THE PLAN IS COMPLETE AND DEPLOYED. N1–N7 are all shipped, merged to `main`,
and live in prod as of 2026-07-11** — prod jumped 69 commits (`c07d228` →
`f4a4c0d`), taking N5 + N6 + N7 and the Pete news seam live in one restart.
Startup was clean (appservice token valid, crypto DB current, cross-signing
preserved, no errors); the `rooms_traversed` bootstrap backfilled 67 rows and the
Pete news backfill emitted 16 facts. Prod is now the first real exercise of
Renown, the Omen, seasonal events, the Hollow King arc, the world boss and duels
— watch for their first live firings.

| Phase | State | Landed at |
|-------|-------|-----------|
| N1 Restoration | ✅ merged | `e8f4863` / `c9df282` / `b5493a0` |
| N2 The Chase | ✅ merged | `adcc621` — see [[project_n2_the_chase]] |
| N3 Parties | ✅ merged | P1–P8; ledger below — see [[project_n3_parties]] |
| N4 The Town | ✅ merged | E1/E2/E3 — see [[project_n4_the_town]] |
| N5 The Hollow King | ✅ merged | `81b2359` — see [[project_n5_hollow_king]] |
| N6 Rivalry & Siege | ✅ merged | `401b17c` — see [[project_n6_rivalry_siege]] |
| N7 Living World | ✅ merged | `a6f1de4` (B2 Renown, B3 Omen, B4 achievements, E4 Seasonal) — see [[project_n7_living_world]] |
| C5 Revisit R1+R2 | ✅ merged | `31e3d69` / `ae5762f` — see `gogobee_revisit_plan.md` |

Since N7 landed, `main` has also taken the sim-real-char harness (`3f4b4ec`), the
`player_meta` auto-migration seed (`fedd357`), and the Pete Adventure News seam
(`f4a4c0d`). `main` is **4 commits ahead of `origin/main`** — unpushed.

### Deploy record (2026-07-11)

Pete first, then gogobee, per the durable-queue ordering. Pete needed a code fix
found at deploy time (`82d1c6e`): `posting.enabled=false` — how Pete's RSS
chatter is kept out of the Matrix rooms — *also* nil'd `advPost`, which killed
live adventure beats **and** the digest loop, making "news on the web only,
adventure live in the games room" inexpressible. `advPost` is now gated on
`[adventure]` alone, since adventure facts are push-based and never enter the
RSS pipeline or the round-robin rotation.

Live surfaces: `news.parodia.dev/adventure` (feed + permalinks + daily 17:00 UTC
digest) and PRIORITY beats to the games room. The cold-start backfill emitted 16
facts (2 zone-firsts, 3 deaths, 11 achievements) with **zero** Matrix posts —
backdated + NoPush suppressed the live push exactly as designed.

### Loose ends carried out of the plan (none blocking)

- **N3 OPEN — no seat can heal another seat. Clerics must be able to heal the
  party.** The N-body engine seats N players against 1 monster, but every heal
  in it is *self-scoped*: `MistyHealProc`/`MistyHealAmt` heal the actor,
  `HealItem`/`HealItemCharges` fire the actor's own <50% trigger, and there is no
  ally-targeted action anywhere (`grep ActionHeal|healAlly|TargetSeat` →
  nothing; the only seat-targeting in the engine is `enemyTargetSeat`, i.e. who
  the *monster* swings at). So a party cleric is a cleric in chassis and passives
  only — they cannot put a single HP back on a friend. Nobody noticed because
  N3 shipped without a support role to test it with; hiring a Cleric companion
  (`!expedition hire cleric`) is what surfaced it, since role-fill promises a
  healer and the engine cannot deliver one.
  Needs: an ally-targeted heal action (`!cast cure @user` / an auto-picked heal
  for autopiloted + companion seats), a target argument through
  `PlayerAction` → `runPartyCombatRound`, and a heal-the-lowest-seat rule in
  `pickAutoCombatActionForSeat`. **This will move the golden** (it adds an
  action the picker can choose), so it wants a deliberate re-baseline, not a
  drive-by. Affects human clerics and the companion equally.
- **N3 perf:** `partyCombatantsForSession` rebuilds every seat's sheet per phase
  step — ~2N sheet builds per dispatch. This is
  [[project_combat_session_cache_deferred]] coming due.
- **N3 balance:** the enemy-multiattack gap was deliberately left open in P8
  (closing it would move the golden) — [[project_sim_multiattack_gap]].
- **Companion balance is unmeasured.** Pete is statted below-median by
  construction (level −1, no masterwork gear, no magic items, no subclass, no
  armed ability) but nobody has run a win-rate sweep with vs without him. The
  plan's rule is "help, never a carry" — that is currently an assertion, not a
  measurement. Wants an `expedition-sim` sweep with a companion seat.
- **N2:** arena season titles are stored (`loadArenaSeasonTitles`) but nothing
  renders them; natural home is `!sheet` or the E3 `!town` registry. Tempering is
  untested end-to-end at the handler level (no Matrix client stub in the plugin
  package). `loadArenaLeaderboard()` is live with no caller.
- **N2 prod watch:** tempering is an uncapped euro sink; watch whether €150k reads
  as too cheap once T5 clears are routine. The material (1/clear) is the real gate.
- **A6 prod watch:** events fire at digest 8% / sell 5% / arena cashout 5%, one per
  player per UTC day. Bump `advEventChanceDigest` if it reads too quiet.
- **Re-baseline class balance** against the post-N3 engine — the corpus of record
  is still [[project_d8prereq_baseline]], which predates parties.

Party navigation should read `CurrentNode` as position and `RoomsTraversed` as
effort (see the R1 note below — `CurrentRoom` is *derived*, not persisted, and
moves backwards on a backtrack; it is not the field party code wants).

---

*Everything below this line is the phase-by-phase build record, kept for the
decisions and gotchas it documents. It is history, not a work queue.*

### N3 — the plan's C1 combat claim was wrong (verified 2026-07-09 @ `ae5762f`)

C1 below says *"the turn engine already loads multiple combatants per
session"* and treats seating N players as an extension. **It is false.**

- `combatantsForSession` returns a `(player, enemy)` **pair**
  (`combat_session_build.go:114`).
- `turnEngine` holds one `player`, one `enemy` (`combat_turn_engine.go:77`).
- `CombatSession` has scalar `PlayerHP`/`EnemyHP` and **no participants
  table** — combatants are rebuilt every round, never persisted.
- The turn engine has **no initiative**. It is a fixed
  `player_turn → enemy_turn → round_end` phase machine; `InitiativeBias` is
  read only by `SimulateCombat`.
- `grep '\[\]Combatant'` matches **nothing repo-wide**, arena included.
- C1 names `simPickCombatAction` as the autopilot fallback. No such function;
  it is `pickAutoCombatAction` (`expedition_sim.go:898`). The name survives
  only in a stale comment.

Two resolvers coexist: the **turn engine** (manual play + expedition-boss
autopilot via `autoDriveCombat`) and **`SimulateCombat`** (harvest interrupts,
patrols, arena, class-balance harness).

So C1 is a rewrite of the most heavily-tuned code in the repo, not an
extension. Estimate is **8–12 sessions**, not 4–5. Decision taken: do the full
multi-combatant rewrite (chosen over a shared-enemy-HP-pool model and a
"fellowship" model where members fight separate encounters). Scope: **N players
vs 1 monster** in v1, roster shaped so M>1 monsters is a later additive change.

**The balance corpus does not have to die.** `SimulateCombat` is the core of
the harness that produced `d8prereq_corpus`. If the N-body core draws RNG in
the same order for the degenerate 1-player case, every solo baseline replays
bit-for-bit. `TestCombatCharacterization` (golden:
`internal/plugin/testdata/combat_characterization.golden`) is the proof.
**If that golden moves during N3, solo balance moved — stop and investigate.**
Regenerate only deliberately:
`go test ./internal/plugin -run TestCombatCharacterization -update`.

#### N3 phase ledger

| Phase | State | Landed in | Notes |
|-------|-------|-----------|-------|
| P1 widen the golden | ✅ done | `fc0dff7` | 22→57 scenarios, 2047→7468 lines. Old golden is a byte-exact prefix of the new one. |
| P2 roster data model | ✅ done | `41f98b7` | `combatState` embeds `*actor`; `seat(i)` cursor. Golden byte-identical. |
| P3 initiative + N-body turn engine | ✅ done | `ec614e8` | `turnOrder`, `phaseForSeat`, `turnIdxForPhase`, `combatSessionStepRNG`, `enemyTarget`, `advancePartyCombatSession`. Golden byte-identical. |
| P4 schema | ✅ done | `d7d0230` | `ActorStatuses` split, `combat_participant`, `roster_size`, `expedition_party`. No `party_id`, no bootstraps — see below. Golden byte-identical. |
| P5 session layer | ✅ done | `e8d0619` | Multi-participant sessions, fight lock, turn ownership, 3-min turn deadline + autopilot latch, reaper roster, per-seat close-out. Golden byte-identical, `go test ./...` green. |
| P6a member-resolution + lifecycle | ✅ done | `0f144fa` | `activeZoneRunFor`, `expeditionAudience`, `fanOutExpeditionDM`, `briefingPetPrefix`, `releaseParty`. Golden byte-identical. |
| P6b invite/roster commands | ✅ done | `1928f75` | `expedition_invite` table, `inviteToParty`, `latestInviteFor`, `addSupplyPurchase`, `!expedition invite/accept/decline/party/leave`. Golden byte-identical. |
| P6c party combat + leader gates + burn | ✅ done | | Seats the roster in `handleFightCmd`; seat-aware `autoDriveCombat`; leader-only fork/`!extract`; burn × size × 0.8. All 5 review findings fixed before it landed (below). Golden byte-identical, `go test ./...` green, solo T1 sim clears. |
| P6d rewire the `getActive*` reads | ✅ done | | Split out of P6c. Every player-facing read now resolves through `activeExpeditionFor` / `activeZoneRunFor` / the new `isPartyMember` / `seatedExpeditionFor`. Four review findings fixed before it landed (below). Golden byte-identical, `go test ./...` green, solo T1 sim clears. |
| P7 sim `-party N` | ✅ done | `32e3148` | `-party N` / `-party-classes` seat followers through the real invite/accept commands. Golden byte-identical, `go test ./...` green. Band read after P6e — see below. |
| P6e seat the party in inline room + patrol combat | ✅ done | `08d3053` | N-body auto-resolve. `combat_engine_party.go` + `zone_combat_party.go`. Golden byte-identical; solo T5 unregressed. Band now reads **100% at T5** — see P8. |
| P8 enemy action economy | ✅ done | `0d18cea` | Roster-scaled re-targeted enemy actions/round (fractional: 2.4 duo, 2N−1 for N≥3) + party enemy HP ×1.15, both engines, solo-exempt (golden byte-identical). Band: solo f70/c27 → duo f75/c31 → trio f90/c72, monotonic. Multiattack gap left open (would move golden). |

#### P7 — the harness works, and it found that a party is only a party twice

Written 2026-07-09/10. New: `-party N` + `-party-classes` on `cmd/expedition-sim`,
`SimRunner.RunPartyExpedition`, `seatParty`, `simRunEndOutcome`,
`expedition_sim_party_test.go`. `Base.DisplayName` gained a nil-`Client` guard —
the party commands call it, and the headless sim has no Matrix client.

Followers are seated through the **real** `!expedition invite` / `!expedition
accept heavy` pair, so the sim measures the tier gate, the busy guard and the
supply pooling rather than a hand-built roster that cannot fail. Both commands
answer a refusal with a DM and a `nil` error, so `seatParty` verifies the seat
against `partySize` afterwards and **halts the run** if anyone was refused: a
party reading taken from a walk that was secretly solo is worse than no reading.

**The finding: the roster is seated at elite/boss doorways and nowhere else.**
`advanceOnce` stops the walk at those two room types (`stopElite` / `stopBoss`)
and hands them to `autoDriveCombat` → `startPartyCombatSession`. *Every other
fight* — exploration rooms, §4.1 patrol encounters, harvest interrupts — goes
through `resolveRoom` / `tryPatrolEncounter` / `runHarvestInterrupt`, which call
`SimulateCombat` against `ctx.Sender`. `ctx.Sender` is the leader, because P6d
made `!zone advance` and `!expedition run` leader-only.

So on a 38-room T5 expedition the party fights **2–3 rooms together** and the
leader solos the other ~35. And because only the leader owns the run and
expedition rows, their death tears both down for everyone. Measured, party of 3,
n=15/cell, L15+16 × `dragons_lair` + `abyss_portal`:

| roster | class | clear% | leader_down% | tpk% | member deaths |
|--------|-------|--------|--------------|------|---------------|
| solo | fighter | 47–67% | — | — | — |
| party 3 | fighter | **100%** | 0% | 0% | **0 / 120 seats** |
| solo | cleric | 13–47% | — | — | — |
| party 3 | cleric | 33–53% | 13–40% | 0% | **0 / 120 seats** |

**Zero member deaths and zero TPKs in 240 seats.** Every party failure is the
leader dying alone while two untouched members stand at full HP. That is what
`simRunEndOutcome`'s new `leader_down` label exists to say out loud; the old code
called it `tpk`, which is how it stayed invisible through P5 and P6.

**Do not apply the C1 contingency (+35% monster HP per member) on these numbers.**
Fighter's 100% is not "parties overshoot the 61–79% band" — it is a 3v1 boss on
top of trash the leader was already clearing solo. An HP scalar applied to *all*
monsters would make the trash the leader fights alone deadlier while barely
denting the doorway fights, i.e. punish exactly the wrong rooms. Cleric's
33–53% is the same story read from the other end: the lift over solo is real but
it lands entirely on the two fights that were never the problem.

**P6e, before P7's band reading.** Route inline room/patrol/harvest combat
through the party engine when `isPartyMember` / a roster exists. Notes:

- `resolveCombatRoom` already takes a `userID` and calls `runZoneCombat`; the
  N-body core is `SimulateCombat`, which P2 widened to a roster. The seam is
  there — it is the *callers* that still pass one player.
- Patrols and harvest interrupts have "retreat continues the run" semantics
  (`retreatThreatBump`), unlike a gating elite. Per-seat close-out has to respect
  that: a member who retreats is not dead.
- Watch the golden. Solo must keep drawing RNG in the same order, exactly as it
  did through P2–P6d.
- Once P6e lands, re-run: `expedition-sim -matrix -party 3 -classes fighter,cleric
  -levels 15,16 -zones dragons_lair,abyss_portal -runs 15 -bank 20000`, and only
  then decide on a party-size monster scalar.

**Two bugs a `/code-review` caught before this landed:**

1. **An unknown class was not an error.** `applyClassBaselineStats` and
   `computeMaxHP` both fall through to a default, so `-class fightr` (or a typo
   in `-party-classes`) built a **1-HP** character, walked it into the zone, and
   reported an ordinary `tpk` / `cleared` for a character nobody asked to
   simulate. This bit the leader's own `-class` too, long before P7. *Fixed* in
   `BuildCharacter` rather than in the flag parser, so every sim caller inherits
   the guard.
2. **The roster short-rest healed the dead.** The post-win rest loop runs for
   every seat, and `handleDnDShortRest` does not gate on the death flag — so a
   member killed in the boss fight the party *won* got rested back above 0 HP,
   and vanished from `res.Members[].EndHP` as a casualty. *Fixed:* skip seats
   `simSeatAlive` says are down.

Also fixed here: `SimResult.Outcome` used to call any run-ending event `tpk`,
including a **timeout loss** — which `resolveCombatRoom` deliberately does *not*
mark as a death ("mechanically ran out the clock"). The vocabulary is now
`tpk` (whole roster dead) / `leader_down` / `fled` (run over, leader alive),
read off the `AdventureCharacter` death flag rather than off HP, which the
close-out leaves in no particular state. **Solo labels are unchanged for a real
death** — for a one-seat roster, leader-dead *is* all-dead — but a solo timeout
loss now scores `fled` where it used to score `tpk`. `cleared%`, the band metric,
is untouched, so the `d8prereq_corpus` baselines still compare. That last claim
is established from the diff, not from a re-run: **the sim is unseeded**, so at
n=15 a cell swings ±13pp on RNG alone and a before/after sweep proves nothing.
No line that sets `cleared` moved, and for a one-seat roster the new short-rest
loop calls `maybeShortRest` with exactly the old `ctx`.

#### P6e — the party fights every room, and the monster's turn doesn't scale

Written 2026-07-10, landed `08d3053`. New: `combat_engine_party.go`
(`simulateParty`, `simulatePartyRound`, `roundInitiative`, `enemyTargetSeat`,
`eventsForSeat`, `finalizeParty`), `zone_combat_party.go` (`runZoneCombatRoster`,
`zoneCombatRoster`, `closeOutZoneWin`, `closeOutZoneLoss`, `partyCasualtyLine`),
`combat_engine_party_test.go`. Touched: `combat_engine.go` (`simulateRound` and
`finalize` moved out / deleted), `combat_turn_engine.go` (`enemyTarget` collapsed
onto the shared `enemyTargetSeat`), `dnd_zone_cmd.go`, `dnd_zone_combat.go`,
`dnd_expedition_combat.go`.

**P6e's own note above was wrong about the seam.** It said "the N-body core is
`SimulateCombat`, which P2 widened to a roster — the seam is there, it is the
*callers* that still pass one player." `SimulateCombat` built a **one-seat roster
internally**; there was no N-body entry point. What *was* true, and what made this
cheap, is that the resolution primitives (`resolvePlayerSwings`,
`resolveEnemyAttack`, `applyAbility`, `trySave`) take `(st, player, enemy)` where
`player` is always `st.c` — the cursor's own `Combatant` — because the turn engine
has called them that way since P3. **The primitives needed no change at all.**
Only the round loop did.

Solo stays bit-identical by copying P3's exemptions, not by branching:
`enemyTargetSeat` draws nothing for a one-seat roster, `roundInitiative` draws one
player roll then one enemy roll (the pre-roster order, ties to the player), and
every per-seat loop runs once. `TestCombatCharacterization` is byte-identical and
`TestSimulateCombat_IsTheOneSeatPartyCase` pins the delegation event-for-event over
40 seeds. A solo T5 re-sweep reads fighter 47–73% / cleric 20–33%, matching P7's
solo numbers inside the ±13pp the unseeded sim gives at n=15.

Decisions worth keeping:

- **`runZoneCombat` stays the explicit solo entry point.** `adventure_arena.go`
  calls it, and an arena fighter must never drag their party into the ring. The
  expedition callers pass `zoneCombatRoster(walker)` themselves.
- **Death is per seat, off HP, never off the fight's terminal status.** A
  timed-out *party* can still have lost somebody, so both the retreat branches
  (`runHarvestInterrupt`, `tryPatrolEncounter`) now mark casualties while keeping
  the run alive. For one seat `PlayerEndHP <= 0` is exactly the old `!TimedOut`
  rule, because a solo player at 0 HP has already ended the fight.
- **The close-out splits on the P5 seam.** Character-scoped effects (HP, XP,
  achievements, subclass, heal items burned, Misty's repair, loot) fan out per
  seat; run-scoped ones (threat, kill record, teardown) stay with the owner.
- **Seat-filtered event logs.** `Seats[i].Events` is that character's sub-log for a
  party, and the whole log by identity for a solo fight — otherwise a member's
  potions get burned for the leader's heals. Unstamped fight-scoped events (enemy
  regen, the timeout) read as the leader's.
- **A preserved quirk, deliberately not fixed here.** A *solo* player can win at
  0 HP — a retaliate aura kills the swinger on the same blow that drops the enemy,
  and `resolvePlayerAttack` returns before `enemyDown` is consumed — and is not
  marked dead. A *party* marks its downed seats dead on a win, which is what
  `finishPartyWin` has always done. Changing the solo case would move the sim's
  outcome labels; it is not P6e's business.

**The band reading, and why C1's contingency is the wrong lever.** Party of 3,
n=15/cell, L15+16 × `dragons_lair` + `abyss_portal`, 120 runs:

| roster | class | clear% | member deaths | tpk |
|--------|-------|--------|---------------|-----|
| solo | fighter | 47–73% | — | — |
| solo | cleric | 20–33% | — | — |
| party 3 | fighter | **100%** | 0 / 240 seats | 0 |
| party 3 | cleric | **100%** | 0 / 240 seats | 0 |

Still zero member deaths — but for the opposite reason to P7's. The members really
are fighting now: they take damage, earn XP, and level mid-run. The monster cannot
keep up. **The enemy takes one turn per round and swings at one seat**, so a party
of N deals ×N damage, the fight lasts 1/N as long, and each member absorbs roughly
**1/N² of the solo incoming**. A T5 member finished a 39-room expedition down 12 HP.

So an HP scalar cannot close this, and `+35%/member` is not merely too small.
Multiplying enemy HP by ×N restores the fight's *duration*, but the enemy still
spends one attack per round on one seat — per-member damage taken stays at 1/N of
solo however much HP you add. **HP is the wrong axis**, which is the whole finding.
Not "HP is forbidden": it simply does not touch the term that produced the 1/N².

### P8 — scale the enemy's action economy (✅ done, `0d18cea`)

Shipped as designed below, with one change the sim forced: the action count is a
**fractional expectation** (2.4 for a duo, 2N−1 for N≥3, realised as a per-round
coin-flip), plus a party-only enemy HP ×1.15. Solo is exempt on both levers and the
golden is byte-identical. Final T5 band is monotonic by party size and never below
the solo rate: solo f70/c27 → duo f75/c31 → trio f90/c72. Lessons: linear-N still
cleared 100%; fighter is HP-insensitive (100% even at HP ×1.6, so *actions* are the
load-bearing lever, exactly as predicted); the integer action lever is too coarse at
N=2, hence the fraction. The multiattack gap was deliberately NOT closed — it would
move the golden. Original brief follows.

Give the enemy attacks-per-round proportional to the seated roster, so a party of
three faces three swings a round rather than one. That is the axis the 1/N² comes
from. It is also the same lever as a long-standing sim gap: `SimulateCombat`
ignores enemy multiattack entirely while the turn engine loops the full profile
(project_sim_multiattack_gap), which is what made prod autopilot secretly easier
than manual play and drove the D8-e/D8-f T4/T5 difficulty lift. P8 closes both.

Attack count is the tool that fits, not a tool chosen around a constraint — no
axis is off-limits here (project_difficulty_target). Combine it with HP, damage or
AC if the band asks for it.

Notes for the executor:

- Solo must draw RNG in the same order or the golden moves and the corpus dies.
  One seat ⇒ one enemy attack: the loop has to collapse exactly as
  `enemyTargetSeat` and `roundInitiative` already do.
- Re-target per swing, not per round, or three attacks all land on one member and
  a party is a coin-flip on who gets focused.
- `abilityDealtDamage` currently suppresses *the* enemy attack. With N swings,
  decide whether a cleave/lifesteal round suppresses all of them or one.
- Whether N swings is right, or `1 + (N-1)×k` for some k < 1, is an empirical
  question — a party should be *easier* than solo (that is the point of a party),
  just not 100%.
- Re-run `expedition-sim -matrix -party 3 -party-classes fighter,cleric -classes
  fighter,cleric -levels 15,16 -zones dragons_lair,abyss_portal -runs 15 -bank
  20000`. Target is the T4/T5 band for a competent build, ~60–75%
  (project_d8f2_t4_difficulty), read off **martial leaders**; casters trail by a
  player-side class gap that monster tuning provably cannot compress.
- The sim is unseeded: ±13pp per cell at n=15. Raise `-runs` before reading a
  10pp move as real.

**P6a/P6b decisions.**

- **The whole party layer was dead code.** `partyMemberIDs`, `disbandParty`,
  `openParty`, `joinParty`, `activeExpeditionFor` had zero callers outside their
  own test. P4 built the tables; P6a/P6b are what wired them.
- **A member owns no zone-run row either**, not just no expedition row — the plan
  and P4's notes only flagged the latter. `!fight`, `!zone status`, `!map`,
  `!revisit` all resolve through `getActiveZoneRun(sender)`. `activeZoneRunFor`
  is the twin of `activeExpeditionFor`; a member reaches the run in two hops via
  the leader's `exp.RunID`.
- **A member must never call `getActiveZoneRun` on the leader's behalf.** That
  function carries the §4.3 idle reap, which force-extracts the wrapping
  expedition — a member glancing at the map would end everyone's run. Pinned by
  `TestActiveZoneRunFor_MemberDoesNotReapAStaleRun`.
- **DMs are per-reader, not per-expedition** — the same lesson P5 learned about
  combat narration. The briefing's *body* is expedition-scoped but its pet-event
  prefix is not: each member has their own pet and their own sheet. Hence
  `fanOutExpeditionDM(e, body, perReader)`. The digest's A6 event anchor rolls
  per member too (it debits a per-player daily slot).
- **`releaseParty` fires on `completeExpedition` / `abandonExpedition` /
  `forcedExtractExpedition`, and deliberately NOT on `voluntaryExtract`** —
  `extracting` is a 7-day resumable limbo and `!resume` must bring the party
  back. The roster clears when the window lapses to `failed`, which routes
  through `completeExpedition` like everything else. A seated member is barred
  from adventuring elsewhere, so a roster outliving its expedition strands them.
- **The plan's invite window is a race, not a window.** "Before the first walk"
  is at most `autoRunMinExpeditionAge` = 30 min, and the leader's own
  `!expedition run` can close it sooner. Changed to **all of Day 1**, plus:
  *an unanswered invite pins the autopilot* (`loadExpeditionsForAutoRun` skips
  expeditions with a live invite), bounded by `expeditionInviteTTL` = 2h **in the
  query itself**, so a forgotten invite costs an afternoon, not the expedition.
  Supplies burn at the night rollover, so a Day-1 latecomer pays and receives
  exactly what a gate-joiner does.
- **Pooling raises `Current` *and* `Max`.** `supplyDepletion` reads the ratio;
  folding in only `Current` would read as the party suddenly starving. Burn rate
  is untouched — scaling it by party size is P6c.
- **Outstanding invites count against `expeditionPartyMax`**, or a leader asks
  four people and three accept.
- **A member's supplies stay in the pool on `!leave`.** They were spent on the
  expedition, not lent to it; a refund would let someone starve the party on
  their way out.
- `assertNotAdventuring` guards expeditions and rosters but **not bare zone
  runs**, so `accept` checks `getActiveZoneRun` itself, as `startExpedition` does.
- **A party is not a taxi:** `zoneOpenToLevel` gates the invitee on the same tier
  rule the leader faced. Multiple pending invites are legal; the freshest wins
  `!expedition accept`, and `joinParty` refuses the rest.

#### P6d — what was built, and the 4 findings that were fixed before it shipped

Written 2026-07-09. New file `expedition_party_reads_test.go`; new seams in
`expedition_party_resolve.go`. Touched: `dnd_zone_cmd.go`, `dnd_zone_cmd_graph.go`,
`dnd_zone_narration.go`, `zone_revisit.go`, `dnd_expedition_cmd.go`,
`dnd_expedition_map.go`, `dnd_expedition_threat.go`, `dnd_expedition_harvest.go`,
`dnd_expedition_camp.go`, `dnd_expedition_region_cmd.go`, `dnd_expedition_extract.go`,
`dnd_economy.go`.

The reads split three ways, and the split is the design:

- **Reads a member should see** resolve through `activeExpeditionFor` /
  `activeZoneRunFor`: `!zone status/map/lore/list`, `!expedition status/log/list`,
  `!map`, `!threat`, `!resources`, `!region` (list), `!camp` (bare).
- **Leader-only actions** get member-specific copy rather than "you have no
  expedition": `!zone advance`, `!expedition run`, `!zone abandon`,
  `!expedition abandon`, `!revisit`, `!region travel`, `!camp <kind>`, `!resume`.
- **Busy-guards must refuse a member**, and did not: `!zone enter`,
  `!expedition start`, `!sell`. These were the real bugs — a seated member could
  open a private dungeon, outfit a rival expedition, or run a shop from inside
  the leader's boss room.

**The four findings, and how each was fixed:**

1. **`!resources` clobbered the leader's `region_state`.** It looks like a pure
   read, but it seed-persists harvest nodes, and `saveHarvestNodes` →
   `persistRegionState` is a last-write-wins UPDATE of the *whole blob* — which
   also carries region kills, event gates and the temporal stack. Rewiring the
   read to `activeExpeditionFor` is exactly what made the write reachable: a
   member's `!resources` mid-walk would resurrect harvested nodes and revert
   region progression from a stale snapshot, under their own lock. *Fixed:*
   seed-persist only when `isLeader`. `seedRoomNodes` is a pure function of the
   zone, so a member re-derives identical nodes without writing.
2. **`!zone taunt` raced the leader's mood.** `applyMoodEvent` is an atomic
   `gm_mood + delta`, so a member moving the party's gauge is safe and intended.
   Its neighbour `applyMoodDecayIfStale` is **not**: it writes gm_mood as an
   *absolute* value derived from the caller's snapshot. Every command takes the
   *sender's* `advUserLock`, so a member running decay against the leader's run
   holds the wrong mutex and drops whatever the leader's walk banked. *Fixed:*
   `applyMoodDecayIfStale(r, owner bool)` — the constraint lives with the write,
   not at three call sites. Decay is time-based maintenance; the owner's next
   command applies the same correction.
3. **The seat outlives `status='active'`.** `expeditionForMember` filters on
   `active`, but `releaseParty` is deliberately *not* called on `extracting` — a
   seven-day resumable limbo (see P6a). So `activeExpeditionFor` goes blind for
   the whole window while the roster still holds the member, and the new
   busy-guards waved them through: open a private run, and it wins every
   `activeZoneRunFor` lookup the moment the leader `!resume`s. *Fixed:*
   `seatedExpeditionFor` spans `active` **and** `extracting`; it is what the
   busy-guards ask.
4. **`!expedition run` was still member-blind** — the most-typed walk command,
   and the sibling of the `!zone advance` this phase gated. *Fixed:* same refusal.

Also folded in: `isPartyMember(userID)` replaces the `run != nil && !isLeader`
spelling at the copy gates. **`activeZoneRunFor` reports `isLeader=false` for a
player with no run anywhere**, so a bare `!isLeader` tells a solo player with
nothing in flight to go ask their leader — a footgun that cost this phase one
bug during authoring and would have cost the next call site another. It also
avoids re-resolving the run `advanceOnce` is about to resolve, which was firing
the §4.3 idle reap twice per `!zone advance`. `msgLeaderPicksPath` de-duplicates
the fork/revisit refusal, and `!revisit` now answers the fight before the route.

Deliberately **not** changed: a member can still swing the party's DM mood with
`!zone taunt` (griefing surface, but a party is consensual, and the write is
atomic); `!sell` now refuses a member for the leader's whole expedition, even for
loot from an earlier solo trip — same rule a leader lives under.

#### P6c — what was built, and the 5 findings that were fixed before it shipped

Written 2026-07-09. `go build ./...`, `go vet ./...`, `go test ./...` all green;
`TestCombatCharacterization` byte-identical; a real solo T1 expedition clears
end-to-end through the rewritten seam via `cmd/expedition-sim` (elite + boss both
resolved). A high-effort `/code-review` turned up five real problems the suite did
not catch; all five are fixed in the commit.

New files: `combat_party_start.go` (roster assembly + opening block),
`combat_party_start_test.go`. Touched: `combat_cmd.go`, `combat_session.go`,
`combat_party_turn.go`, `dnd_cast.go`, `dnd_rest.go`, `expedition_sim.go`,
`dnd_expedition_supplies.go`, `dnd_expedition_cycle.go`,
`dnd_expedition_extract.go`, `dnd_expedition_region_cmd.go`, `dnd_zone_cmd_graph.go`.

Design as built:
- **Seat 0 is always the expedition leader**, whoever typed `!fight`. Every seat-0
  invariant P4/P5 laid down rests on it: the lock key, the once-only close-out
  effects, leader-only `!flee`. `fightRoster` resolves it *before* the lock (it must
  not touch `getActiveZoneRun` — that carries the §4.3 idle reap), and `roster[0]`
  is the owner, so there is one lookup rather than two.
- A downed **member** sits the fight out; a downed **leader** refuses it for
  everyone, with copy that differs for leader vs member (`seatZeroRefusal`).
- `autoDriveCombat` re-targets `ctx.Sender` to `sess.seatUserID(actingSeat(...))`
  each iteration, gated on `cur.IsParty()`. Its round cap is now scaled by roster
  size, because it counts **dispatches**, not rounds.
- Burn: `applyExpeditionDailyBurn(e, harsh, siege)` wraps `applyDailyBurnP` with
  `expeditionBurnRatePct(e.ID)` = `50 × N × 4/5`. **The 0.8 is an exact 4/5 ratio,
  not a float** — `int(float32(50*3) * 0.8)` truncates to 119, a silent permanent
  tax on every party of three. Solo returns exactly 50.

**The five findings, and how each was fixed:**

1. **The enemy lost its round-1 turn whenever it won initiative.** P3 code, only
   reachable once a party is seated. `startCombatSession` writes
   `Phase: player_turn, TurnIdx: 0` unconditionally; on resume `turnIdxForPhase`
   snaps the cursor to the first `player_turn` slot, so an `order[0] == enemySeat`
   round-1 sequence skipped the enemy entirely. Round 2+ was always correct because
   `stepRoundEnd` sets `Phase = phaseForSeat(te.order[0])`. Solo is immune (order is
   hardcoded `[0, enemySeat]`).
   *Fixed:* `startPartyCombatSession` now takes the built `*Combatant` enemy, derives
   round 1's order, and sets `Phase` from `order[0]`. `buildFightSeats` hands back the
   `*Combatant`s it used to discard (`CombatSeatSetup.C`), which also kills the
   second full roster rebuild the opening block was doing. `handleFightCmd` then
   `settleCombatSession`s before announcing, so a monster that swings first is
   *narrated* in the opening block rather than silently subtracting HP; if that
   opening wipes the party, `closePartyRound` runs the close-out. Pinned by
   `TestStartPartyCombatSession_EnemyThatWinsInitiativeOpensTheRound` and its
   party-wins twin.
2. **`!cast` was member-blind.** `handleDnDCastCmd` gated on
   `getActiveCombatSession(ctx.Sender)`, nil for a member, so a member's `!cast` in a
   party fight fell through to the out-of-combat path and queued a `PendingCast`
   instead of acting. *Fixed:* `activeCombatSessionFor`.
3. **`!rest` was member-blind.** `restBlockedReason` used `hasActiveCombatSession` +
   `getActiveExpedition`, both of which answer "no" for a member — a seated member
   could `!rest` to full HP mid party boss fight. *Fixed:* `activeCombatSessionFor` +
   `activeExpeditionFor`, with member-specific copy (a member cannot `!extract`).
4. **A downed member who typed `!fight` got no reply.** They opened the fight for
   everyone, were skipped from seating, and `announcePartyFightStart` only DMs seated
   members. *Fixed:* `buildFightSeats` returns `senderSkip` — the sender's own reason
   for being left out — and `handleFightCmd` answers with it. The one-seat tail
   handles the case where the skip leaves a *solo* session: the block goes to the
   leader by `SendDM`, the skip to the sender.
5. **`!extract` was leader-only but still DM'd only the leader.** *Fixed:*
   `fanOutExpeditionDM` reaches the whole roster (the roster deliberately outlives a
   voluntary extract), with a per-reader tail because only the leader can `!resume`;
   `maybeRollPetArrivalOnEmerge` now rolls for every member, since every member
   surfaced.

Also folded in from the same review: `fightOwner` deleted (it was `fightRoster(s)[0]`),
the "Your move" line unified behind `partyMovePrompt`.

Still deferred (perf, not correctness): `actingSeatForAutopilot` calls
`partyCombatantsForSession` purely to translate seat→UserID and `beginCombatTurn`
then rebuilds the roster again — ~2N sheet rebuilds per dispatch. This is
project_combat_session_cache_deferred coming due; P7 will run it at volume.

Pre-existing, **not** caused by P6c (verified against HEAD in a clean worktree):
`expedition-sim -zone manor_of_whispers -level 10` halts with "expedition did not
persist after start", at any `-bank`. T1 `goblin_warrens` clears fine.

#### P6c — the blocker P5/P6 did not anticipate

`autoDriveCombat` (`expedition_sim.go:794`) is the production autopilot *and* the
sim harness. It calls `handleFightCmd(ctx)` then loops
`pickAutoCombatAction(ctx.Sender, cur)` → `handleAttackCmd(ctx)` — always as the
**same sender**. The moment `handleFightCmd` seats a roster, seats 1+ reject that
sender with `beginCombatTurn`'s "not your turn" and the loop spins to
`autoCombatRoundCap`.

So P6c cannot just swap `startCombatSession` → `startPartyCombatSession`. It must
also give `autoDriveCombat` a seat-aware loop: read `actingSeat(sess, players,
enemy)`, re-target `ctx.Sender` to `sess.seatUserID(seat)`, dispatch. **Gate the
whole thing on `sess.IsParty()`** — the solo path must stay bit-identical or the
`d8prereq_corpus` baselines move.

Other P6c notes gathered but not yet acted on:

- `handleFightCmd` takes `p.advUserLock(sender)`. A party fight must take
  `p.lockCombatFight(leader, sender)` instead (P5's ordering: fight lock first).
- `startPartyCombatSession` checks only seat 0 for an in-flight session. Guard
  every seat with `hasActiveCombatSession` before seating.
- Seat only members with HP > 0; a downed member sits the fight out (they own no
  close-out claim, and `finishPartyCombatSession` decides death per seat off HP).
- Leader-only surfaces still to gate: `zoneCmdGo` (fork), `handleExtractCmd`
  (currently just says "no active expedition" to a member — wrong copy, right
  outcome), `!flee` (P5 already made it leader-only in a party).
- Remaining `getActive*` reads to rewire: ~76 call sites, but only the
  player-facing ones matter — `dnd_zone_cmd.go` (status/map/lore/advance),
  `dnd_expedition_map.go`, `zone_revisit.go`, `dnd_expedition_threat.go`,
  `dnd_expedition_harvest.go` (`!resources`), `dnd_economy.go` (`!sell`),
  `dnd_expedition_camp.go`. The tickers and the sim keep `getActiveExpedition`.
- Burn scaling: `supplyDailyBurn(tier)` × `partySize(expID)` × 0.8, applied where
  `applyDailyBurn` is called (`nightRolloverBurn`, voluntary extract, region
  transit). `addSupplyPurchase` deliberately left `DailyBurn` alone.

**P5 decisions.** The turn timeout is **3 minutes, not the plan's 60s**, and it
latches. Reasons: the sweep rides the existing one-minute `eventTicker` (D4: no
net-new ticker), so any deadline really fires in `[d, d+1m)` — 60s would have
meant 60–120s, and this is an async Matrix bot whose expeditions run for *days*.
A lapsed deadline sets `ActorStatuses.Autopilot` (JSON, `omitempty`, no schema
bump) so the absent member costs the party **one** wait rather than one per
round; any combat command from them clears it. Solo sessions are never swept —
`roster_size > 1` is in the query — so a lone player still answers only to the
1h reaper.

New files: `combat_party_turn.go` (lock, turn ownership, autopilot, deadline
sweep), `combat_party_finish.go` (per-seat close-out), `combat_party_turn_test.go`.

Things P5 found that the plan did not anticipate:

- **The narration is second person.** `RenderTurnRound`'s pool says "You score 9
  damage", "A hit gets through your guard" — `playerName` was almost unused.
  Swapping names per seat would have told all three members they each landed the
  same blow. A round is now rendered **once per reader** (`RenderPartyTurnRound`
  takes a `viewerSeat`): your own events go through the untouched flavor pool,
  your allies' through a terse third-person summariser (`renderAllySeatEvent`).
  Solo renders the same bytes it always did.
- **`CombatEvent` gained `Seat int` (`json:"Seat,omitempty"`).** The golden
  formats fields explicitly rather than reflecting, so it did not move. Events
  are stamped once per phase step in `turnEngine.stampSeat`, not at the ~20
  append sites in `combat_primitives.go` — the primitives emit against the
  cursor and know nothing of seats. `combatState.seatIdx` trails `seat()`.
- **`runCombatRound` would have stalled a party on a corpse.** Its loop rested on
  any `player_turn`, and a downed seat still holds one. Split into
  `settleCombatSession` (drains enemy turn, round end, *and* downed seats) +
  `runPartyCombatRound`. `beginCombatTurn` settles before reading whose turn it
  is, which also fixes a latent solo bug: a fight interrupted mid `enemy_turn`
  used to resume parked there and silently eat the player's next `!attack`.
- **Buffs were landing on seat 0.** `handleCombatCastCmd`/`handleConsumeCmd`
  folded every delta into `sess.Statuses.applyBuffDelta` — the session's embedded
  copy, i.e. the leader. A member casting Shield on themselves would have
  armoured the leader. Now `sess.actorStatusesPtr(seat)`.
- **So was the auto-picker.** `pickAutoCombatAction` read `sess.PlayerHP` /
  `PlayerHPMax` / `Statuses.ConcentrationDmg` — all seat-0 fields. Generalised to
  `pickAutoCombatActionForSeat`; the old name is a seat-0 wrapper so the sim's
  behaviour is bit-identical. `simPickSpiritualWeapon` now takes `ActorStatuses`
  rather than the session.
- **Locking.** Three members typing `!attack` at once took three *different* user
  locks. `lockCombatFight(owner, sender)` takes the fight's lock (keyed on seat 0)
  then the member's own, always in that order; every other command in the repo
  takes at most one user lock, so no cycle closes. A solo fight's owner *is* the
  sender and `sync.Mutex` is not reentrant, so that case takes exactly one lock —
  `TestLockCombatFight_SoloTakesExactlyOneLock` hangs rather than fails if that
  regresses.
- **Close-out fans out along the data model's own seam.** Run- and
  expedition-scoped effects (threat, zone-kill record, boss-defeat threat, the
  loss/flee teardown) all resolve through `getActiveExpedition(userID)` or
  `getActiveZoneRun(userID)` and a member owns neither row — so they fire **once**,
  for the owner. Fanning them out would have tripled the threat one kill costs.
  Character-scoped effects (HP, XP, loot, death) fan out per seat. A member can
  be dead in a fight the party won, so death is decided per seat off HP, not off
  the session's terminal status.
- **The reaper stays attack-only.** It does *not* use the picker: finishing an
  abandoned fight should not quietly burn the player's spell slots and potions.
  The turn-deadline latch does use the picker, because that member is mid-fight
  with a party waiting on them. Deliberate asymmetry.
- **`!flee` is leader-only in a party** — it ends the run for everyone, the same
  reasoning that makes `!extract` leader-only. Not in the plan; revisit in P6.

Carried into P6:

- `startPartyCombatSession` has **no production caller**. `handleFightCmd` still
  opens a solo session off `startCombatSession`; wiring it is P6's job, once
  `joinParty` can actually seat somebody.
- `partyCombatantsForSession` rebuilds **every seat from its own sheet** — N× the
  per-round DB chatter `combatantsForSession` already had
  (project_combat_session_cache_deferred). Solo still issues exactly one build.
  A party of 3 pays it three times per phase step; worth a cache before P7's sim
  runs it at volume.
- `beginCombatTurn` probes for the session **unlocked**, then re-reads under the
  lock and revalidates (the reaper's own pattern). The window is real but the
  revalidation closes it.
- Nothing calls `activeExpeditionFor` yet either — still P6.

**P1 findings.** The golden pinned 22 scenarios and missed everything the
refactor was most likely to disturb: `ExtraAttacks` (the lever J1 moved) and
the rest of the 2026-05-16 class-identity mods, the race passives, and **all
13 Phase-13 bestiary slice-3/4 effects** (evade, block, advantage, retaliate,
regenerate, survive_at_1, stat_drain, debuff, max_hp_drain, spell_resist,
reveal_action, fear_immune, ally_buff) — none had a pin. Enemies are sized per
scenario so per-hit riders accumulate; abilities proc at 1.0 so the pin
captures the effect, not the roll.

**P2 decisions.** `combatState` embeds `*actor` **by pointer** — promoted
fields keep their names, so ~230 call sites (`st.playerHP`, `st.wardCharges`,
…) compile untouched, and `seat(i)` is a one-line cursor move. A *value* embed
would silently copy on `seat()`; `TestCombatState_SeatSwitchesPerActorStateOnly`
pins that. Split rule: the enemy's **stance** (evade/block/advantage/retaliate/
regen/survive) and hold-person are **fight-scoped** (holding the enemy holds it
for everyone); debuffs stacked on one character (stat_drain / debuff /
max_hp_drain) and concentration are **per-actor**. P2 landed the data model
only — nothing yet decides who the enemy swings at.

**P3 decisions.** Initiative is **solo-exempt**: `turnOrder` returns the fixed
`[player, enemy]` for a one-seat roster and rolls nothing. The turn engine has
never had initiative, so rolling one would have been a live balance change to
every manual elite/boss fight. Parties use the auto-resolve formula (`speed +
d10 + InitiativeBias`), re-rolled per round, derived from `(sessionID, round)`
so a resume rebuilds the same order.

RNG: every seat's `player_turn` in a round shares a `(round, phase)` pair, so
the acting seat is mixed into the PCG **seed**, not the stream. Seat 0 and the
enemy sentinel mix to zero → solo streams are bit-identical to pre-roster.

The round cursor is `Statuses.TurnIdx` (JSON, `omitempty` — no schema bump, no
solo row carries it). **Prod rows written before P3 decode it as 0**, so
`turnIdxForPhase` reconciles cursor-vs-`Phase` with `Phase` winning; without it
a suspended `enemy_turn` resumes into an infinite enemy turn. Keep that
reconciliation until every pre-P3 session has aged out (TTL 1h).

Latent solo bug fixed in passing: `resolvePlayerSwings` returns **false** when a
retaliate aura kills the swinger between extra attacks, and `stepPlayerTurn` used
to read the outcome off that bool — walking a corpse into the enemy's turn.
Outcome is now read off HP + `anyAlive()`.

Still single-seat in persistence: `commit()` writes seat 0 explicitly (the enemy
turn parks the cursor on its target; `round_end` walks it across the roster), and
`resumeTurnEngine` opens seats 1+ fresh from their `Mods`. **P4 must give seats
1+ their own persisted statuses**, or a party fight resets their once-per-fight
one-shots every step. *(Done in P4 — `restoreActor` / `snapshotActor` are exact
inverses, and `commit` now addresses seats by index rather than off the cursor.)*

**P4 decisions.** `CombatStatuses` split into a fight-scoped half (enemy stance,
`TurnIdx`) and a per-character `ActorStatuses`, embedded **anonymously and
untagged** so `statuses_json` stays the same flat one-level object. Prod rows
decode unchanged; `TestCombatStatuses_JSONStaysFlat` pins the flattening, because
adding a json tag to that embed would silently wipe every in-flight fight's
poison, charges, and one-shots on the next resume.

Seat 0 stays on `combat_session`; seats 1+ get `combat_participant` rows. The
asymmetry is deliberate — **a solo fight writes the exact bytes it wrote before
N3, and zero participant rows**. A new `roster_size` column (DEFAULT 1) guards
the read so the solo loader never issues the second query
(project_combat_session_cache_deferred: don't make the chatter worse). Party
saves wrap the session row + all seats in one transaction; solo keeps its single
unwrapped UPDATE.

**No `party_id` column.** The plan called for one on `dnd_expedition`, but
`expedition_id` *is* the party's identity — a second key would be a second answer
to "who is in this party", free to disagree with `expedition_party`. Cost: a
member owns no expedition row, so `getActiveExpedition` is blind to them.
`activeExpeditionFor(user) (e, isLeader, err)` is the lookup every player-facing
command must switch to in P6; `getActiveCombatSessionForMember` is its combat twin.

**No bootstraps.** Both new tables read *absent == solo*, and `roster_size`'s
DEFAULT 1 is correct for every pre-existing row — the same reasoning that let N2's
`temper` ALTERs ship without a backfill. Nothing to reconstruct.

`joinParty` seats the leader itself (reading the owner off `dnd_expedition`)
rather than trusting callers to call `openParty` first: `partyMemberIDs` reads the
roster, not the expedition row, so a leaderless roster would silently stop DMing
the leader. It also enforces "one active expedition per user" across membership,
in-transaction, so two simultaneous invites can't both take the last seat.

**Flaky-test pair fixed** (the one N1 flagged as "worth seeding at some point").
`TestSimulateCombat_FirstAttackBonusImprovesEarlyHits` and
`TestResolvePlayerAttack_AssassinateBonusFirstHitOnly` called `SimulateCombat`
with a nil RNG — the *package-global* stream — so their verdict depended on how
much randomness every test declared before them consumed. Adding P4's test files
shifted that stream and pushed the suite to ~2-in-3 failing. Both effects are
real, but both tests were underpowered: over 40 seeds, Precision averaged +127
wins against its +50 threshold with a **worst case of −42**, and Assassinate
averaged +12.8 with two seeds outright negative. Now seeded (`statCompareRNG`)
and raised to 24000 / 6000 trials, where all 40 seeds clear (worst +267 / +68).
Assassinate's `> 0` assertion became `≥ 25`. Suite is 5/5 green.

**Known gotchas for P6+.** `SendDM` resolves exactly one `id.UserID` to one DM
room (`plugin.go:702`); the whole digest/briefing/recap seam is
single-recipient with no fan-out anywhere. `dnd_expedition` and `dnd_zone_run`
are single-`user_id` rows, but "one active per user" is **code**-enforced
(`startExpedition`), not a schema constraint — so a membership table is clean.
Repo is not gofmt-clean at baseline (104 files); don't chase it.

### N2 item ledger

| Item | State | Landed in | Key symbols | Tests |
|------|-------|-----------|-------------|-------|
| B1 tempering | ✅ done | `adcc621` | `temperLadder`, `temperedRarity`, `temperedItem`, `temperStepsToLegendary`, `EquippedMagicItem.Effective`, `magicItemSellAt`, `temperEquippedItem`, `temperInventoryItem`, `handleTemperCmd` | `adventure_temper_test.go` |
| B4 achievements | ✅ done | `adcc621` | `clearedZoneIDs`, `clearedAnyZoneOfTier`, `clearedEveryZoneOfTier`, `clearedUnderThreat50`, ids `expedition_*` + `temper_legendary` | `achievements_expedition_test.go` |
| C4 arena seasons | ✅ done | `adcc621` | `arenaSeasonKey/Start/Bounds`, `previousArenaSeason`, `loadArenaSeasonLeaderboard`, `arenaSeasonChampion`, `arenaSeasonRollover` | `adventure_arena_season_test.go` |

### N2 decisions taken (deviations from the plan above)

- **Temper is a per-instance step count, not a rarity rewrite.** The plan
  assumed a magic item had a mutable rarity. It does not: `MagicItem.Rarity`
  is a field on the registry *definition*, re-resolved from
  `magicItemRegistry` on every load (`magic_items_gameplay.go`), and an
  equipped item is stored only as `(user, slot, item_id, attuned)`. So a
  `temper` column was added to `adventure_inventory` and
  `magic_item_equipped`; effective rarity is derived on read via
  `temperedItem()`. The base rarity is **never** written back — that's what
  makes a load/save round-trip idempotent (`TestTemperNeverDoubleBumps`).
  Both ALTERs default to 0, which is correct for every pre-existing row, so
  no bootstrap backfill was needed.
- **VeryRare is not a rung.** It collapses onto Epic in both
  `rarityLootTierNum` and `rarityPowerScalar`, so a VeryRare→Epic temper
  would change no number. The ladder is Common→Uncommon→Rare→Epic→Legendary,
  which is exactly the 4 steps the plan's cost table priced.
- **Materials already drop; the plan was wrong twice.** `thyraks_core` and
  `portal_fragment` are not "loot-note strings" needing promotion to real
  items at ~15% from T5 bosses. They are `UniqueAlways: true` entries on the
  Underforge / Abyss Portal slates (`dnd_zone.go`), materialized by
  `zoneLootToInventory` (`dnd_zone_combat.go`) into real `adventure_inventory`
  rows named "Thyraks Core" / "Portal Fragment" — at **100% per zone clear**,
  not 15%. `rollZoneLoot` (the slate one, `dnd_zone_combat.go:360`) fires once,
  on zone clear. Nothing needed adding; tempering just consumes them.
  `TestTemperMaterialsAreObtainable` guards the name match against slate edits.
- **Only the Legendary rung eats a material.** Gating every rung on a T5
  material makes tempering unreachable until a player already clears T5 —
  precisely when they no longer need it. Euros + Foraging gate the rest.
- **Quiet-clear requires the threat key's presence, not a low value.**
  `recordMaxThreat` has exactly one production caller
  (`dnd_expedition_cycle.go:148`, the day rollover) and its `cur <= prev`
  guard means it never stores a zero. An empty `region_state` therefore means
  "no threat samples" (a same-day clear), not "threat stayed at zero" —
  reading it as the latter handed the achievement to everyone.
- **No-death-T4 achievement is NOT shipped.** There is no per-expedition
  death counter — no column, no `deaths_during` field. It needs a schema bump
  to be checkable and was not faked. Pick it up if/when one lands.
- **`combat_pet_save` already existed** (`achievements.go`, granted from
  `combat_bridge.go`), so the plan's "pet-saved-my-life" item was already done.
- **Renown achievements skipped** — B2 (renown) hasn't shipped.
- **Arena seasons derive, they don't reset.** The plan called for a quarterly
  wipe of `arena_stats`. Instead season standings aggregate `arena_history`
  over `[quarter_start, quarter_end)`. Identical player-visible effect (the
  board clears each quarter), but lifetime totals survive for `!arena stats`
  and there is no destructive job that can double-fire or half-complete.
  `renderArenaLeaderboard` now takes a season label.
- **Season crowns go to their own table, not `player_meta.title`.** That
  column already carries the Survivalist milestone
  (`dnd_expedition_milestone.go`); a season champion would silently destroy
  someone's expedition title. New `arena_season_titles(season, kind, user_id,
  value, awarded_at)`, PK `(season, kind)`, insert is `DO NOTHING` so the
  rollover is safe to re-run.
- **Rollover self-heals.** `arenaSeasonRollover` runs every midnight off the
  existing ticker, dedups on `db.JobCompleted("arena_season_rollover", season)`,
  and refuses to close a season that hasn't ended. A bot down on Jan 1 crowns
  Q4 the next time it wakes. No net-new ticker (D4 discipline).

Open items carried out of N2:

- **`temper_legendary` is the only new event-driven achievement** — granted
  from `executeTemper`. The 11 others are passive query checks.
- **Season titles are stored but not yet surfaced.** `loadArenaSeasonTitles`
  exists and is tested; nothing renders it. Natural home is `!sheet` or the
  E3 `!town` registry. Cheap follow-up.
- **`!arena leaderboard` no longer shows lifetime standings.** It footers a
  pointer to `!arena stats`. If that reads badly in prod, add
  `!arena leaderboard lifetime` — `loadArenaLeaderboard()` is still live and
  now has no caller.
- **Tempering is untested end-to-end at the handler level** — same gap as A5:
  the plugin package has no Matrix client stub. `executeTemper`'s rollback
  paths (euro refund + material restore) are reasoned, not exercised.
- **Prod balance note:** tempering is a euro sink with no cap on how many
  items a rich player upgrades. Watch whether €150k reads as too cheap once
  T5 clears are routine — the material is the real gate, at 1/clear.

### N1 item ledger

| Item | State | Landed in | Key symbols | Tests |
|------|-------|-----------|-------------|-------|
| A1 treasure | ✅ done | `e8f4863` | `rollAdvTreasureDropDetailed(…, weight)`, `checkTreasureDrop(…, weight)`, `rollZoneTreasure`, `advLocForZone`, `advTreasureWeight*` | `adventure_treasure_test.go` |
| A2 masterwork | ✅ done | `e8f4863` | `masterworkDefForZone`, `masterworkSlotActivities`, dungeon case in `masterworkDropFlavorText` | `adventure_masterwork_test.go` |
| A3 consumables + ingredients | ✅ done | `e8f4863` | `rollZoneIngredient`, `advIngredientDropChance`, `advIngredientActivities`, `grantZoneItem`, `joinLootLines` | `adventure_ingredients_test.go` |
| A4 milestones | ✅ done | uncommitted | `grantTwoWeeksCache`, `grantSurvivalistTitle`, `grantLongGameLegendary`, `consumableCache`, `consumableAdvItem`, `milestoneAward.Extra` | `dnd_expedition_milestone_test.go` |
| A5 lottery pot | ✅ done | `c9df282` | `communityPotAdd` in `handleLotteryBuy` | none (no client stub; see below) |
| A6 mid-day events | ✅ done | uncommitted | `maybeFireAnchoredEvent`, `claimDailyEventSlot`, `releaseDailyEventSlot`, `advEventChance{Digest,Sell,Arena}` | `adventure_events_test.go` |

### A4 / A6 decisions taken

- **Two Weeks pays in supplies, not stats.** The `+5 max HP` idea was
  dropped: `stats.HPBonus` is built from gear/arena/housing in
  `combat_stats.go:88` with no expedition in scope, and threading one
  through would leak an expedition-only buff into the sim's balance
  corpus. Day 15 now restocks 3 days of rations (clamped to
  `Supplies.Max`) and grants 3 consumables from the zone-tier dungeon
  pool. Self-expires when the run ends; zero combat math.
- **Survivalist needed no schema bump.** `player_meta.title` already
  exists (`db.go:306`) and `saveAdvCharacter` → `upsertPlayerMetaMiscState`
  already persists `AdventureCharacter.Title`. Sets the title + posts a
  games-room notice.
- **Long Game** grants `pickMagicItemForRarity(RarityLegendary, nil)` via
  `dropMagicItemLoot(…, LootTierLegendary)`, rendered in the new
  `milestoneAward.Extra` block (below Notes, not italicized).
- **A6 anchors are digest 8% / sell 5% / arena cashout 5%**, with a
  one-event-per-player-per-UTC-day cap. A player who hits all three lands
  at ~1.19 events/week (was ~1 per 200 days). The `eventTicker` keeps only
  `expireAdvPendingEvents` + `reapExpiredCombatSessions`; the whole
  deferred roll-minute scheduler is gone. `tryTriggerEvent` now returns
  `bool` so a bail (dead / mid-fight / event already active) hands the
  day's slot back via `releaseDailyEventSlot`.
- **Digest anchor fires exactly on the digest branch.** `campDecision.Night`
  is true iff `buildAutoRunDM` renders the EoD digest — including the
  boss-safety camp path, which sets `Night` when past `nightCampWindow`.
- **`tryTriggerEvent` no longer gates on `HasActedToday`** — the anchor
  *is* the presence signal now.
- **A6's frequency test is deterministic.** It drives its own seeded
  `rand.New(rand.NewPCG(…))` over the chance constants + daily cap rather
  than sampling the global RNG, so it can't join the flaky-test set below.

**Seam of record:** everything in A1–A3 hangs off `dropZoneLoot`
(`dnd_zone_loot.go`), which now takes `(…, isBoss, isElite bool)` and is
called from 5 sites: `combat_cmd.go:290`, `dnd_zone_cmd.go:1019` + `:1124`,
`dnd_expedition_combat.go:209` + `:539`. Zone-clear's bonus treasure roll
lives in `finalizeExpeditionOnZoneClear` (`dnd_expedition_extract.go`).

Behaviour notes worth keeping:
- Treasure near-miss DMs fire **only** for weighted moments (boss/elite/zone
  clear). At ×1 on autopilot they'd land on ~3% of every kill.
- Masterwork is elite/boss only. Arena gear stays the premium line (×1.5
  effective tier vs masterwork ×1.25), so this competes with the shop.
- Consumables + ingredients roll on *any* win, independently of the zone
  slate — a dry slate roll doesn't cost the player either.

Survey corrections found while building (the plan's anchors were right;
these are things it *understated*):

1. **A2 was a no-op as specced.** The masterwork catalog is keyed only to
   mining/fishing/foraging, so `masterworkDefFor(AdvActivityDungeon, …)`
   returns nil. Fixed with `masterworkDefForZone`, which rolls across all
   three slot lines at the zone tier, plus a new dungeon flavor pool
   (flavor now follows `loc.Activity`, not the item's catalog line).
2. **A3's ingredient audit was worse than "needs an audit."**
   `combat_bridge.go`'s `resolveDungeonAction` — the only caller of
   `generateAdvLoot` — itself has **zero callers**. All four legacy loot
   tables were dead, so *every* ingredient was unobtainable and all 12
   recipes were uncraftable, not just the four named. Fixed by having
   `rollZoneIngredient` draw from the legacy tables directly at zone tier
   (15%/win), reviving them wholesale. Guard test:
   `TestEveryRecipeIngredientHasALiveSource`.
3. **Pre-existing flaky tests**, unrelated to N1:
   `TestResolvePlayerAttack_AssassinateBonusFirstHitOnly` and
   `TestSimulateCombat_FirstAttackBonusImprovesEarlyHits` are statistical
   and ride the global RNG — they fail intermittently on a *clean* tree
   too. A1 perturbs RNG ordering, so it changes which one flakes. Worth
   seeding them deterministically at some point.

Open items carried out of N1:

- **A5 has no handler-level test** — the plugin package has no Matrix
  client stub, so an end-to-end `handleLotteryBuy` test would mean
  building one. The change is one line after the persist guard.
- **Cartographer is still deferred** — it's map-coverage-shaped; leave it
  until C5 (revisit) ships, per the original plan.
- **Rates to watch in prod:** the A6 anchor chances (8/5/5) target ~1
  event/week for a player who does all three daily. Most players only hit
  the digest, so the realistic rate is ~0.56/week. Bump `advEventChanceDigest`
  if that reads too quiet.
- **Rates I picked, flag if wrong:** ingredient drop 15%/win
  (`advIngredientDropChance`); `RollConsumableDrop` kept its existing flat
  15%. On autopilot that's ~2–3 of each per active day. Consumables have
  no tier-1 dungeon entry, so T1 zones drop none.

Note the working tree also carries unrelated in-flight work (appservice,
link_thumbnail, safehttp, crosssigning) — stage N1 files explicitly.

Grounded in a 3-agent codebase survey (core loop / expedition depth /
social-meta) run 2026-07-09 against HEAD `a4f162d`.

## Executor guide (read first)

- All `file:line` anchors below were read at HEAD `a4f162d` on
  2026-07-09. **Verify every anchor against HEAD before building on it**
  (feedback_verify_audit_findings) — especially the "zero callers"
  claims, which a later session may have fixed.
- Each phase (N1–N7) ships independently: code + tests + `go test ./...`
  green. Within a phase, items are ordered; don't reorder across the
  dependency notes.
- Every schema change ships with a one-shot bootstrap that stays in the
  tree (feedback_loader_rewire_needs_bootstrap). Follow the
  `bootstrap_*.go` pattern (e.g. `bootstrap_player_meta.go`).
- Flavor: check `adventure_flavor_*.go` for reusable lines before writing
  new ones (feedback_reuse_existing_flavor). TwinBee lines are
  first-person / implicit-subject only, he/him or they
  (feedback_twinbee_voice, feedback_twinbee_is_male).
- Build/deploy: CGO=1 `-tags goolm` (Dockerfile's CGO=0 crashes). Never
  deploy with an empty local `data/gogobee.db`
  (feedback_empty_local_db_wipes_prod).
- Difficulty changes lift trailers, never nerf leaders
  (feedback_difficulty_target). Surface verbs/outcomes, hide math
  (feedback_accessibility_over_dnd_crunch).
- No new `dnd_`-prefixed files/types/tables (feedback_avoid_dnd_naming).
  New files use `adventure_*` / `expedition_*` / `zone_*` prefixes.
  Editing existing `dnd_*.go` files is fine.
- Misty/Arina buff mechanics are secret discovery content
  (feedback_npc_buffs_are_secret) — nothing player-facing may imply
  donation → buff. Flagged per-item below where it bites.

## 0. Where engagement actually leaks today (survey findings)

**The Phase R transition orphaned reward systems.** The legacy daily
activity loop was retired (`adventure.go:973-1022` now intercepts legacy
activity input with a deprecation DM), but three drop systems only fired
from it and now have **zero callers** (verified by call-site grep at
survey time):

- `checkTreasureDrop` / `rollAdvTreasureDrop` (`adventure.go:1075`) — the
  26-treasure catalog (`adventure_treasure.go:55-241`), tiered drop rates
  1.5%→0.15% (`adventure_treasure.go:43-51`), auto-swap-worst with 10-min
  undo, lock/unlock, T5 room announcements (`adventure.go:1075-1282`):
  all unreachable. No new treasure has dropped since Phase R. Existing
  owners keep bonuses via `computeAdvBonuses`.
- `checkMasterworkDrop` (`adventure_masterwork.go:161`) — 15-item MW
  catalog can no longer drop (arena helms are the only live MW source).
- `RollConsumableDrop` (`adventure_consumables.go:225`) — T2–T5 drop-only
  consumables (Spirit Tonic, Voidstone Shard, …) unobtainable except
  crafting; several craft ingredients ("Goblin Trinket", "Blooper Ink",
  "Ancient Artifact", "Dragon Scale") live in legacy loot tables
  (`adventure_activities.go:133-163`) whose only live caller is
  `combat_bridge.go:555` — whether they still flow needs an audit.

**Headline rewards are stubbed.** The Long Game (T5 completion) legendary
grant is "deferred to loot-grant hookup"
(`dnd_expedition_milestone.go:199`); Survivalist title, Two Weeks stat
bump, and the Cartographer milestone are likewise unwired. Zone loot
notes reference legendary crafting materials (`thyraks_core`,
`portal_fragment`) whose recipes don't exist.

**Endgame is thin.** L20 cap at 85,000 cumulative XP (`dnd_xp.go:50`), no
prestige. Gear best-in-slot is deterministic — one Legendary per slot,
rarity-scalar formulas in `magic_items_gameplay.go:125-167` (Legendary
weapon +25% dmg, armor −20% taken, wondrous +10 HP + init), 3-attunement
cap (`dndMagicItemAttuneLimit`) — reachable well before content runs out.
Post-cap loops: repeat T5, arena streaks, collection. T5 zones are
explicitly raid-shaped (`raidContentWarning`,
`dnd_expedition_cmd.go:358-373`) with multiplayer referenced in copy but
unbuilt.

**Money stops mattering.** Houses T3 (€300k) / T4 (€600k)
(`adventure_housing.go:22-57`, +11% fees) buy zero mechanics — the whole
housing payoff is the T2 pet gate (`petShouldArrive`,
`adventure_pets.go:193-207`). Shop says "nothing left to buy" at max
(`luigiMaxedOut`, `adventure_shop.go:182`). One buyable consumable
(Berry Poultice, `adventure_consumables.go:50`).

**No player-to-player play.** No item trading/gifting (only
`!baltransfer`, `euro.go:511-543`), no co-op, no player-initiated duels
(rival RPS is system-paired, asymmetric — `adventure_rival.go:325-380`),
no seasons. Rich `player_meta` data (tax_ledger, rival records,
death/grudge fields, pets, hospital visits) is persisted but unsurfaced.

**Events players never see.** Mid-day events roll 0.5%/player/day with a
2h expiry (`adventure_events.go:131`) — one sighting every ~200 days.

**Bug found in passing:** `!lottery pot` copy (`lottery.go:190-191`) says
ticket sales fund the pot, but `handleLotteryBuy` (`lottery.go:111`) only
debits the player — ticket euros are burned, never credited to
`community_pot`.

---

## Track A — Rewire the orphans (quick wins, do first)

The cheapest engagement per session in the plan: the content is already
written and tuned — it just lost its call sites.

### A1 — Treasure drops → expedition beats
**Hook:** the combat-win loot seam in `dnd_zone_loot.go` (where rarity is
derived from enemy CR) and the zone-clear path
(`finalizeExpeditionOnZoneClear`, `dnd_expedition_extract.go:126-171`).
Call `checkTreasureDrop(userID)` after the normal loot roll with a
room-type weight: boss kill ×4, elite ×2, secret room ×2, standard ×1,
zone clear = one bonus roll. Base rates stay the existing tiered
1.5%→0.15% table. The auto-swap/undo/lock/announce machinery
(`adventure.go:1075-1282`) is reused untouched — it takes over once the
drop lands.
**Accept:** a sim or test run through a boss kill produces a treasure at
forced-RNG; `!adventure treasures` shows it; T5 announce fires in the
games room. ~0.5 session.

### A2 — Masterwork drops → elite/boss kills
Same seam as A1: `checkMasterworkDrop` on elite/boss wins only. Keep
arena gear as the ×1.5 effective-tier line and MW as ×1.25
(`advEffectiveTier`, `adventure_character.go:381`) so arena stays
premium. **Accept:** forced roll grants an MW item; `!adventure equip`
lists it; blacksmith bills it one tier up
(`adventure_blacksmith.go:15-40` already handles this). ~0.25 session.

### A3 — Consumable drops + ingredient audit
Hook `RollConsumableDrop` into the same combat-win seam (any win, not
just elite/boss). Then audit: do "Goblin Trinket"-class ingredients
still reach players via `combat_bridge.go:555`? If not, add them to the
per-zone loot slates in `dnd_zone_loot.go` keyed by zone tier matching
the legacy table tiers (`adventure_activities.go:133-163`).
**Accept:** every recipe in `adventure_consumables.go` has all
ingredients obtainable from at least one live source; add a test that
walks the recipe list and asserts each ingredient appears in some live
loot table. ~0.5 session.

### A4 — Wire the stubbed milestone rewards
In `dnd_expedition_milestone.go`:
- **Long Game** (T5 completion): grant a guaranteed Legendary roll from
  the cleared zone's slate — call the existing magic-item grant path used
  by `dnd_zone_loot.go` with rarity forced to Legendary.
- **Survivalist** (T3+ completion): write title to `player_meta.title` +
  games-room announce.
- **Two Weeks** (day 15): smallest defensible bump — +5 max HP for the
  remainder of that expedition (apply via the expedition row, not the
  character, so it self-expires).
- **Cartographer:** leave deferred unless C5 (revisit) ships — it's
  map-coverage-shaped.
**Accept:** milestone tests extended to assert the grants. ~0.5 session.

### A5 — Lottery pot fix
Route ticket purchases into `communityPotAdd` (`handleLotteryBuy`,
`lottery.go:111`; pot helper lives in `adventure_rival.go:193-241`).
Mechanic-matches-copy, and strengthens the pot flywheel. Watch
pot-drain-vs-rake per project_lottery_economics — if weekly pot growth
jumps, tune prize tiers, not the ticket price. One-liner + test.

### A6 — Mid-day events resurrection
Retire the 0.5% daily roll in `adventure_events.go:131`. Re-anchor the
existing event content to moments the player is present:
- end-of-day digest (`expedition_autorun_digest.go`): ~10% chance an
  event rides along at the bottom;
- post-`!sell` at Thom's (`adventure_shop.go:833-939` sell path);
- arena cashout (`arenaCompleteSession`, `adventure_arena.go:518`).
Keep the existing 2h response window + `!adventure respond` plumbing.
**Accept:** target rate ~1 event/week for a daily player; add a
frequency test with fixed RNG. ~0.5 session.

**Track A total ≈ 2.5 sessions.** Ship as one phase; announce nothing —
players just start finding things again.

---

## Track B — Endgame & the item chase

### B1 — Legendary crafting ("tempering")
**Shape:** at the blacksmith (`adventure_blacksmith.go`, flavor in
`adventure_flavor_blacksmith.go`), `!adventure temper <item>` upgrades an
owned magic item one rarity step, capped at Legendary. Cost: one
legendary material (`thyraks_core` / `portal_fragment` — currently
loot-note strings in the T5 zone tables in `dnd_zone.go`; promote them to
real inventory items dropping from T5 bosses at ~15%) + euros scaled by
target rarity (suggest 10k/25k/60k/150k for Uncommon→Rare→Epic→Legendary)
+ a Foraging-level gate (reuse the recipe-tier gates at 10/15/20/25/30,
`adventure_consumables.go:307-338`).
**Why tempering over unique recipes:** every duplicate drop becomes a
potential input, and the blacksmith gets an endgame role. Rarity change
flows through the existing scalar formulas in
`magic_items_gameplay.go:125-167` — no new combat math.
**Accept:** temper path test (material + euros debited, rarity bumped,
attunement preserved); ceiling unchanged (cap at Legendary means B1
accelerates reaching the existing BiS, doesn't raise it — no re-baseline
needed). ~1.5 sessions.

### B2 — Renown (prestige without reset)
No character wipe — wiping fights the multi-day expedition identity.
At L20 (`dnd_xp.go:50`), overflow XP converts to **Renown** at a steep
rate (suggest 25,000 XP/renown level; hook where `checkAdvLevelUp` /
`dnd_xp.go` clamps at cap). Renown grants:
- title ladder announced in the games room (write to
  `player_meta.title`),
- a cosmetic marker on `!sheet` and the leaderboard
  (`adventure_render.go:1028-1032`),
- one small account perk per 3 renown levels drawn from the existing
  streak-grant vocabulary (+loot quality, −death penalty —
  `computeAdvBonuses`, `adventure_activities.go:322-337`), capped so
  renown ≤ streak-30 in total power. Never combat-stat inflation — the
  balance corpus stays valid.
**Schema:** `renown_level`, `renown_xp` on `player_meta` + bootstrap.
~1 session.

### B3 — Weekly mutators ("the Omen")
One rotating world modifier per ISO week, announced by TwinBee Monday
morning (first-person voice, via the existing morning DM seam in
`adventure_scheduler.go`). Implementation: new
`adventure_omen.go` with a table keyed by `(year, isoWeek) % len(table)`
so it's deterministic with no schema. Read it at the seams holidays
already use — the pattern is exactly:
- supply freebie: `dnd_expedition_cmd.go:300-307`
- expedition mood: `dnd_expedition.go:160-162`
- harvest yield: `dnd_expedition_harvest.go:463-465`
Launch set (all buffs-with-texture or trade-offs, never pure difficulty):
elites drop double loot but +2 ATK; threat drifts −25% overnight; harvest
+1 yield; arena payouts +20%; consumable drops ×2. ~1 session.

### B4 — Achievements refresh
`achievements.go` defs are chat-centric (adventure ones at lines
753-1133 are mostly streak/death). Add an expedition wing, all passive
checks against existing tables: first clear per tier (5), both-zones-set
per tier (5), no-death T4 clear, threat-under-50 clear (pairs with the
Patient Zero milestone), pet-saved-my-life (ditch recovery fired),
tempered-to-Legendary, renown 1/5/10. ~0.5 session.

---

## Track C — New modes

### C1 — Co-op expeditions ("parties") — the marquee feature
Already promised in shipped copy ("Until multiplayer expeditions
ship…", near `raidContentWarning`, `dnd_expedition_cmd.go:358-373`).
Scope v1 tightly:

**Player surface:** `!expedition invite @user` after `!expedition start`
but before the first walk; party of 2–3; invitee confirms with their own
loadout purchase (reuses `resolveLoadoutOrParse`). `!expedition party`
shows roster. Leader = inviter; leader gets fork prompts (8h auto-pick
unchanged, `forkAutoPickTimeout`, `dnd_expedition_cmd.go:676`).

**Model:** one shared zone run + threat clock + day counter; supplies
pool (sum of purchases; burn = per-tier rate × party size × 0.8 so
parties are slightly supply-efficient). One expedition row remains the
source of truth; members reference it.
**Schema:** `expedition_party(expedition_id, user_id, role, joined_at)`
+ `party_id` nullable on the expedition row + one-shot bootstrap
(existing solo expeditions get no party row — code treats absent as
solo).

**Combat:** the turn engine already loads multiple combatants per
session (`combatantsForSession` — note its known 6-load DB chatter,
project_combat_session_cache_deferred; don't make it worse, fix it here
if convenient). Extend `combat_session_build.go` to seat N player
characters; initiative interleaves; each member acts on their own turn
via DM (`!attack`/`!cast`/…) with a 60s per-turn timeout falling back to
the autopilot picker (`simPickCombatAction` already encodes sane
class-aware choices). Autopilot walks resolve party combat fully
automatically, same as solo.

**Death/extract:** member death → pet-rescue/hospital as solo, expedition
continues for the rest; `!extract` is leader-only and party-wide;
forced-extract rules unchanged.

**Loot/XP:** loot rolls per-member independently (no split arguments, no
dependency on trading); XP full to each member (accessibility over
crunch); milestones to all.

**DM surface:** each member gets their own end-of-day digest; walk
stream stays suppressed per D4 rules. No net-new tickers.

**T5 answer:** T5 is raid-shaped by design; a 2–3 player party at L15+
should hit the D11 61–79% leader band **without touching monster
tuning**. Before shipping, extend `cmd/expedition-sim` with a `-party N`
flag (seat N clones or N distinct classes) and validate the band
empirically. If parties overshoot (>90% T5), add a party-size monster HP
scalar (+35%/member) — HP only, never damage, per the difficulty rule.

**Accept:** sim clears a full T4 with a 2-party, zero manual commands;
solo path regression suite untouched; party of 2 casters beats their
solo baseline materially (this is also a trailer lift).
**~4–5 sessions. Biggest item; highest ceiling.** New files:
`expedition_party.go`, `expedition_party_test.go`.

### C2 — Player-initiated duels
`!duel @user [stake]` — real turn-based PvP through the existing turn
engine (`combat_turn_engine.go`), fought "at the arena" (flavor), best
of 1, **no death**: loser yields at 0 HP — no hospital bill, no respawn
timer, no death-source stamp. Stake escrowed on accept; winner 70%, pot
30% (mirrors rival economics, `adventure_rival.go:638-646`). 24h accept
window; per-pair 7-day cooldown (reuse the rival pair-cooldown pattern);
babysit auto-declines (parity with `adventure_rival.go` RPS). Results
write to `adventure_rival_records` so one W/L history covers both duel
kinds.
**Balance caveat:** PvP exposes class-vs-class deltas the PvE corpus
never measured. Frame as bragging rights; cap stakes at level×€500 so
imbalance can't be farmed; explicitly out of scope to balance classes
for PvP. New file: `adventure_duel.go`. ~2 sessions.

### C3 — World boss ("the Siege")
Monthly communal event, announced in the games room: a named boss
"camps outside town" for 72h with a shared HP pool.
**Model:** new table `world_boss(id, name, hp_max, hp_current, starts_at,
ends_at, status)` + `world_boss_contrib(boss_id, user_id, fights, damage)`.
Each player gets one fight per day against it — an arena-style bout
through `runZoneCombat` with a boss template scaled off the *median*
active-player level (reuse the TwinBee tier-selection idea,
`adventure_twinbee.go:22-91`); player damage dealt is subtracted from the
pool win or lose.
**Resolution:** boss falls → payout to every contributor scaled by
*fights fought, not damage share* (accessibility; reuse the TwinBee
distribution shape, `adventure_twinbee.go:239-320`), plus a consumable
cache (A3 pool) and one low-rate treasure roll each (A1). Boss survives →
"it loots the town": community pot pays a visible tribute
(`communityPotDebit`) — a pot **sink**, which the economy needs.
**Scheduling:** ride the existing 1-minute ticker + `db.JobCompleted`
dedup pattern in `adventure_scheduler.go`; no new ticker process. New
file: `adventure_worldboss.go`. ~2 sessions.

#### C3 build plan (N6, branch `n6-rivalry-siege`)

**Decisions locked (user, 2026-07-10):**
- **Spawn = both.** Auto on the 1st of each UTC month (dedup
  `db.JobCompleted("worldboss_spawn_"+YYYY-MM)`) **plus** an operator
  override `!adventure worldboss spawn`.
- **Defeat payout = minted** (`p.euro.Credit`, faucet) + item cache +
  one treasure roll per contributor, scaled by **fights fought** (not
  damage — accessibility). **Survive = pot tribute** (`communityPotDebit`,
  a sink). So the boss is faucet-on-win / sink-on-loss; the item drops are
  the only guaranteed mint.
- **HP = real cost, like the arena.** No HP restore; the bout persists HP
  the way `resolveArenaBoss` does. No death / no hospital, though — mirror
  arena's no-permadeath, not the expedition death path.

**Seams confirmed (2026-07-10 subagent map):**
- Single bout: `runZoneCombat(userID, monster DnDMonsterTemplate, tier,
  bossCombatPhases, dmMood)` → `CombatResult`. Damage to boss =
  `EnemyEntryHP - EnemyEndHP`. `runZoneCombatRoster` persists HP + grants
  XP internally — same as the arena, which is what "real HP cost" rides.
- Boss template: build a `DnDMonsterTemplate` directly (per-fight enemy is
  disposable; the shared pool lives in `world_boss.hp_current`). Scale off
  `arenaTierBaseStats(tier)` for the per-fight stat block.
- Ticker: add `p.worldBossTick()` to the `eventTicker` `case <-ticker.C:`
  block (`adventure_events.go:~110`). No new goroutine.
- Games room: `gr := gamesRoom(); if gr == "" { return }; p.SendMessage(gr, …)`.
- Pot: `communityPotAdd/Balance/Debit(int) bool`; debit returns false if short.
- Active players: `SELECT DISTINCT user_id FROM daily_activity WHERE date >= cutoff`
  (any-chat presence, feedback_presence_is_any_chat). No median helper exists —
  reduce combined level (`dndLevelForUser + Mining+Foraging+Fishing`) myself.
- Rival split reference is actually winner-30% / pot-70% (`adventure_rival.go:638`,
  the plan's "70/30" is inverted) — not reused here since defeat is minted.

**Isolation:** own tables `world_boss` + `world_boss_contrib`, own load/upsert,
ticker-driven, **outside** the `saveAdvCharacter` fan-out — same structural
isolation `adventure_shadow` earns (a char save can't clobber the pool). No
bootstrap: absent active row == no event.

**Phase ledger:**

| Phase | State | Notes |
|-------|-------|-------|
| W1 model + spawn + lifecycle | ✅ `c3e1226` | schema, `worldBossState`/contrib load+upsert, median scaling, `spawnWorldBoss`, `worldBossTick` (auto-spawn 1st + 72h expiry → resolution), games-room announce, resolution (defeat mint / survive tribute). No player fight yet. Pure logic unit-tested; golden byte-identical; suite green. |
| W2 fight + contrib + command | ✅ `127d252` | `!adventure worldboss [status\|fight\|spawn]` (alias `siege`), once/day gate (`advUserLock`-serialised), arena-style solo bout via `runZoneCombat`, atomic pool subtract, contrib record, no-death HP floor (`worldBossFloorHP`), defeat trips resolution after the bout DM. Bout core factored out + unit-tested with a real fightable char; suite green. |

**Tuning knobs to watch in prod:** `worldBossDefeatBaseEuro` (1500/fight —
minted faucet; a 10-player town clearing it ≈ €45k/mo minted), `worldBossBoutsPerPlayer`
(2.0 — pool size vs turnout), `worldBossTributePct` (0.20 — pot sink on survive).
All consts in `adventure_worldboss.go`. Not deployed (operator's schedule).

**Deliberately not built (candidate W3 / fold-in):** morning-briefing line while a
Siege is live (the Shadow has one; the spawn announce is games-room only right now),
and surfacing contribution in `!town`. Both cheap, both ride existing seams.

### C4 — Arena seasons
Quarterly reset of the arena leaderboard (`!arena leaderboard`,
`adventure_arena.go:52`) with a season title for top streak + top
earnings, archived to `player_meta` (`season_titles` JSON or a small
table). Announce season start/end via the evening summary seam.
~0.5 session.

### C5 — Revisit/backtrack (R1–R5)
Fully specced in `gogobee_revisit_plan.md` — `rooms_traversed` schema
split, `!revisit <N>` reverse-edge walk at reduced cost (threat +1 / SU
−0.5 vs +2/−1.0), fork re-pick, autopilot guard. **Sequencing note:** do
R1 (the `CurrentRoom` → `RoomsTraversed` audit) *before* C1 if possible —
party navigation should land on the reworked model, not be rewritten
after. ~3–4 sessions per its own estimate.

---

## Track D — Story & world

### D1 — The Campaign: a serialized zone arc
The 10 zones are mechanically linked but narratively disconnected. Add a
light campaign layer: a named antagonist (working name: **the Hollow
King**) threading T1→T5. Delivery uses only existing seams — no new
commands except one viewer:

- **Journal pages:** a loot-note class dropping from elites/secret rooms
  (hook = A1's seam), ~24 numbered fragments. Tracked as a bitmask/JSON
  in `player_meta`; `!adventure journal` renders found pages in order
  with gaps shown as "…".
- **Boss epilogues:** each zone boss death gets 2–3 lines tying it to the
  arc — append to the boss-kill narration path (`renderBossOutcome` /
  zone narration in `dnd_zone_narration.go`), flavor lives in a new
  `adventure_flavor_campaign.go`.
- **Finale:** all pages + both T5 zones cleared → unlocks a one-room
  epilogue encounter (variant of an existing T5 boss stat block from
  `dnd_zone.go`, new name/flavor, no new mechanics) via
  `!expedition start epilogue`; reward = unique title + one Legendary
  roll.
- TwinBee reacts to newly-found pages in the morning digest —
  first-person, curious, one line, never expository.
~1 session of plumbing + flavor passes that can ride along with any
phase. ~2 sessions total.

### D2 — NPC arcs
- **Misty:** 3 dialogue stages at 5/15/30 encounters
  (`MistyEncounterCount` already tracked, `adventure_npcs.go`); the
  existing housing hint (`adventure_pets.go:422-430`) and pet
  reactivation (`mistyReactivatePet`, `adventure_pets.go:437-439`) become
  stage beats. **Fiction deepens; mechanics stay hidden** — no new copy
  may connect donations to combat outcomes.
- **Robbie:** every ~10th visit (`RobbieVisitCount`,
  `adventure_robbie.go`) he *leaves* a consumable from the A3 pool — "for
  the trouble."
- **Thom:** on final mortgage payoff (`adventure_housing.go:553-576`
  region), one letter + a pet treat. Voice from
  `thom_krooke_mortgage_flavor.go`.
- **Pete bot** stays deferred (Phase 16) — out of scope.
~1 session, mostly flavor.

### D3 — The Shadow ✅ SHIPPED + MERGED (N6, `cc165be`, no deploy)

Built as spec'd: per-player `adventure_shadow` table (isolated from the
player_meta save fan-out so the ticker can't be clobbered; no bootstrap),
advanced once/UTC-day by `midnightReset` at ~1.3× the player's clear pace,
lead-capped so it stays a race. Surfaces: morning-briefing race one-liners
(TwinBee voice) + a zone-clear payoff in `finalizeExpeditionOnZoneClear` — a
set-once bonus-XP crow when the player was first, or a guaranteed D1 journal
page waiting in the boss room when the Shadow cleared first. `!adventure shadow`
status view. 3-finder+verify review folded in (crow farm, page-loss-on-transient,
false +XP claim all fixed). Combat golden byte-identical. Original spec below.

Per-player persistent NPC (name seeded from the player's, stored in
`player_meta`) who "runs" the same zones on a simulated schedule — a row
with `current_zone`, `day_counter`, `zones_cleared`, advanced by the
midnight ticker (~1.3× the player's own clear pace so it stays close).
Surfaces:
- morning briefing/digest one-liners ("passed the Shadow's camp — cold
  ashes, two days old") — TwinBee voice rules apply;
- player clears a zone first → small XP crow; Shadow clears first → a D1
  journal page waits in that boss room (tie-in).
Pure simulation theater — no combat, no punishment, race pressure only.
New file: `adventure_shadow.go`. ~1.5 sessions.

### D4 — Secret content pass ✅ SHIPPED (N5, branch `n5-hollow-king`)

Built: secret rooms now resolve as **no-combat treasure caches** (was: they
silently collapsed to a normal exploration fight; `LootBias` was dead code).
Each pays a guaranteed journal page + LootBias-weighted treasure roll +
guaranteed zone-tier consumable cache. Underdark got its missing secret
(throne_gallery fork → Lost Reliquary). Two cross-zone keys: Sunken Sigil
(Sunken Temple → Manor `sealed_reliquary`) and Underforge Seal (Underforge →
Underdark `sealed_forge_vault`). Seam is `resolveRoom` → `resolveSecretRoom`
(shared by manual/autopilot/sim); flavor + key catalog in
`adventure_flavor_campaign.go`. Graph validator + no-soft-lock pass; combat
golden byte-identical. Original spec below.

Zone graphs already support `secret` node kinds and
perception/key/stat-check edge locks (`zone_graph.go`, per-zone
`zone_graph_<zone>.go`). Add one secret room per T2+ zone (loot: A1
treasure roll + a D1 journal page) and two **cross-zone keys** (item
found in one zone unlocks an edge in another — e.g. a Sunken Temple
sigil opens a Manor Blackspire vault). Run the existing graph validator
+ the no-soft-lock test (added after the feywild fork1 incident) on
every touched graph. Makes revisit (C5) and re-running lower tiers
meaningful. ~1 session + graph edits.

---

## Track E — Social & economy

### E1 — Housing T3/T4 payoffs
- **T3 "Fine home":** **trophy room** — treasure cap 3→4 where the 4th
  slot is house-bound (active only while owned; enforce in
  `computeAdvBonuses`); **workshop** — +5% craft success at home (hook
  the success roll in `adventure_consumables.go:307-338`).
- **T4 "Estate":** **vault** — 10-slot item storage
  (`!adventure vault [store|take] <item>`; prerequisite plumbing for E2);
  long-rest at home counts as inn-quality (hook `dnd_rest.go` where
  housing already grants HP bonus — `adventure_character.go:279`);
  **second pet slot** — a second arrival roll unlocks via
  `petShouldArrive` (`adventure_pets.go:193-207`) with `HouseTier >= 4`;
  both pets roll combat procs at half weight (halve the per-level
  scalars in `adventure_pets.go:62-94` when two pets are active) so it's
  flavor-forward, not a stat spike. **✅ SHIPPED `57a0ea9`** — combat seam
  was `combat_stats.go` DerivePlayerStats (not the dead `petRollCombatActions`);
  two pets *average* their procs (byte-identical for one), morning-defense /
  ditch-recovery / supply-shop kept pet-1-only. N4 now complete.
Turns €900k of dead prestige tiers into the game's biggest sinks.
**Schema:** vault table + pet-2 fields on `player_meta` + bootstrap.
~1.5 sessions.

### E2 — Item gifting
`!give <item> @user` for consumables and unequipped non-magic gear. 5% of
item value to the community pot (`communityTax` pattern), daily cap of 3
gifts/sender (twink-funnel guard), recipient must have done `!setup`.
No magic-item gifting in v1 — attunement/BiS economy stays personal.
~1 session.

### E3 — Surfacing the buried social data ✅ SHIPPED (`27d6bfd`, N4)

`!town` / `!graveyard` / `!rivals board` — read-only boards off
`player_meta` / `tax_ledger` / `adventure_rival_records`. No schema, golden
byte-identical. Leak-check verified at HEAD (tax_ledger carries zero
Misty/Arina donation signal). `adventure_town.go` (+test). Gotcha logged:
`MAX(datetime)` loses SQLite type affinity — parse the text by hand. Remaining
N4 work: only E1's T4 second pet slot (invasive). Original spec below.


- `!town` — town registry: civic-pride board (lifetime `tax_ledger`
  contributions), housing street (name + house tier only), pet showcase
  (name/type/level/armor from `player_meta`).
- `!graveyard` — recent deaths with `death_source`/`death_location` from
  `player_meta`; the grudge-revenge bonus already exists
  (`adventure_activities.go:340-343`) — this just makes the story
  visible. Comedic-respectful tone; reuse hospital flavor voice.
- `!rivals board` — `adventure_rival_records` as room-wide standings
  (today it's per-user only, `adventure_rival.go:858-900`).
**Leak check:** nothing here may rank or badge Misty/Arina donation
counts — a "generosity" board would let players correlate donations with
performance and reverse-engineer the secret buffs. Tax-ledger totals are
safe (dominated by gambling rake); donation counts are not. ~1 session.

### E4 — Seasonal events
Pick 3–4 anchor holidays/year (calendar engine already computes ~35,
`adventure_holidays.go:34-101`) and attach a 1-week skin: a themed
mutator (B3 machinery with a holiday override slot), a limited-time curio
shelf at Luigi's (the daily-seeded shelf pattern, `dailyCuriosStock`,
`adventure_shop.go:1123-1166`, with a holiday item pool), and one holiday
variant elite seeded into active expeditions (reskin an existing stat
block, spawn via the ambient-event seam in `expedition_ambient.go`).
~1 session of machinery once B3 exists.

---

## Sequencing

Phases named to avoid the dnd prefix; each ships independently.

| Phase | Contents | Sessions | Why this order |
|-------|----------|---------:|----------------|
| **N1 — Restoration** | A1–A6 | ~2.5 | Pure recovered value; B1, C3, D1, D4, E1 all assume drops flow again |
| **N2 — The Chase** | B1 tempering, B4 achievements, C4 arena seasons | ~2.5 | Endgame chase online before new modes bring players back |
| **N3 — Parties** | C1 (+ sim `-party` flag; ideally C5's R1 schema pass first) | ~4–5 | Marquee; the designed answer to raid-shaped T5 |
| **N4 — The Town** | E1 housing, E3 registries, E2 gifting | ~3.5 | Money sinks + social surface while party meta settles |
| **N5 — The Hollow King** | D1 campaign, D2 NPC arcs, D4 secret pass | ~4 | Story layer; journal pages want A1/D4 loot seams live |
| **N6 — Rivalry & Siege** | C2 duels, C3 world boss, D3 the Shadow | ~5.5 | PvP/communal modes; benefit from N3's session-build work |
| **N7 — Living World** | B2 renown, B3 mutators, E4 seasonal events | ~3 | Long-horizon retention; mutator machinery feeds E4 |
| *(flex)* | C5 revisit R2–R5 (R1 pulled earlier if N3 needs it) | ~3.5 | Slot anywhere after N1 |

~28–30 sessions at listed scope. N1 first is unambiguous. If trimming to
a 10-session core: **N1 + B1 + C1** — fixes the leak, gives the endgame a
chase, delivers the promised multiplayer.

## Cross-cutting risks

- **Balance corpus staleness:** C1 parties shift effective power;
  re-baseline with the existing sim harness after N3 (current corpus of
  record: `sim_results/d8prereq_corpus.jsonl` lineage — check memory for
  the latest before diffing). B1 tempering caps at Legendary so it only
  accelerates reaching the existing ceiling.
- **DM volume:** every new system rides existing surfaces (digest,
  morning briefing, evening summary, games room). Hard rule: **no
  net-new scheduled DM in any phase** (D4 discipline;
  feedback_skip_recaps).
- **Community pot flows:** A5, C2, C3, E2 all touch pot in/out. Keep the
  `tax_ledger` pattern for every new flow; after N6, run a
  drain-vs-rake review like the lottery pass
  (project_lottery_economics).
- **Secret-buff leakage:** E3 and D2 are the danger zones — review both
  against feedback_npc_buffs_are_secret before ship.
- **Schema bumps** (party, vault, world boss, renown, journal, shadow):
  each ships with a one-shot bootstrap kept in-tree
  (feedback_loader_rewire_needs_bootstrap).
- **Zone graph edits** (D4, E4): always run the graph validator + the
  no-soft-lock test (feywild fork1 precedent,
  project_feywild_fork1_softlock).
- **Presence semantics:** any "active player" gating (C3 contribution,
  B3 announcements) uses the any-chat presence definition
  (feedback_presence_is_any_chat), not bot-command activity.
