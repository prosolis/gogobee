# Revisit / Backtrack Navigation — Design Plan

Lets players walk back to previously visited rooms within an active zone
run to re-harvest, re-fork (e.g. after acquiring a key), or pick up
something they skipped. Forward-only navigation has been the rule since
Phase G; this plan retires that assumption deliberately.

## Goals

- Re-harvest depleted nodes after a long rest (primary use case).
- Allow general exploration freedom — wander back, look around.
- Re-pick a fork after acquiring a key or changing strategy.
- Preserve the existing pacing pressure (threat clock, SU drain) but
  make backtracking *cheaper* than fresh exploration to reflect that
  the route is already known.

## Non-goals

- Teleporting to arbitrary discovered rooms.
- Backtracking during `!expedition run` autopilot (autopilot is
  forward-only by design).
- Re-fighting cleared encounters or re-arming disarmed traps.

## Player-facing surface

- **`!revisit <N>`** — walks one edge from the current node toward a
  previously-visited node. `N` is the 1-indexed room number printed in
  `!map`'s Path strip.
  - Must be in `VisitedNodes`.
  - Must be adjacent to `CurrentNode` via an edge in the zone graph
    (forward or reverse direction; graph edges are directed but we
    allow reverse traversal for revisit).
  - Costs threat and SU at a reduced rate (see Cost Model).
  - No combat re-roll, no trap re-arm in cleared rooms.

- **`!map`** — already shows the numbered Path strip; players read N
  from there.

- **`!zone advance`** post-revisit — unchanged. From any node, picks
  the first outgoing edge (or shows the fork prompt). Re-forking a
  previously-resolved fork node clears the prior choice silently.

## Cost model

> **CORRECTED 2026-07-09 during R1.** The table below originally claimed a
> fresh `!zone advance` costs +2 threat / −1.0 SU. It does not. Verified at
> HEAD: **movement itself is free.** There is no per-room tick anywhere.
> - **Supplies burn per *day*,** not per room — `applyDailyBurn` is called
>   only from `dnd_expedition_cycle.go:108` (the day rollover),
>   `dnd_expedition_extract.go:52`, and region transit. Never from advance.
> - **Threat comes from *combat*,** not from walking:
>   `applyRoomCombatThreatForUser` (+5 normal, +8 elite/boss) fires from the
>   three combat-resolution sites, plus daily drift, harvest noise (+2), and
>   ambient. `zoneCmdAdvance` / `advanceOnce` touch neither clock.
>
> This inverts R2's central premise. A revisited room fires no combat
> (terminal `CombatSession` rows gate re-entry), so backtracking is already
> free — the design problem is not *discounting* a cost, it's that there is
> **no pacing pressure to discount**. Whatever R2 charges is a net-new cost.

| Move | Threat tick | SU tick |
|------|-------------|---------|
| Fresh-room `!zone advance` | **0** (actual) | **0** (actual) |
| Combat resolved in a room | +5, +8 elite/boss | 0 |
| Day rollover | drift (mood-scaled) | −daily burn |
| `!revisit <N>` (one edge) | **TBD — see below** | n/a (no per-move SU exists) |

**DECIDED 2026-07-09 — option (1), +1 threat per revisit edge.** Shipped in
R2 as `revisitThreatCost`. Standalone (non-expedition) zone runs have no
threat clock and so pay nothing. A `revisitSiegeGuard` refuses the move at
threat ≥ 99: siege is one-way sticky, and a player must not be able to strand
their own expedition on a move they made to go pick up a herb.

Options as they were weighed, cheapest first:
1. **Charge +1 threat per revisit edge.** One `applyThreatDelta` call,
   mirrors "harvest noise". Reads as "you stir up attention doubling back."
   Cheap, honest, and it's the only clock revisit can plausibly touch.
2. **Charge nothing.** Backtracking is already free of combat; the real
   cost is the day clock, which a long detour burns anyway. Simplest, and
   arguably correct — the pacing pressure is the multi-day supply budget,
   not the step count.
3. **Charge a fractional day.** Rejected: the day counter is the spine of
   every milestone, digest and anchored event. Don't make it fractional.

Recommend **(1)** at +1: it's the smallest lever that makes a detour a real
choice, and it rides an existing, tested seam.

## State model

The hard problem: `CurrentRoom` is currently derived as
`len(VisitedNodes)-1` (dnd_zone_run.go:377), which assumes
monotonically-growing visit history. Backtracking breaks that.

### Schema changes

- **`VisitedNodes`** — unchanged semantics: ordered set of nodes ever
  entered, first-entry order. Stays append-only when entering a *new*
  node; revisits do not append.
- **`CurrentNode`** — already the source-of-truth for "where am I."
- **`CurrentRoom`** — repurpose from `len(VisitedNodes)-1` to "index of
  CurrentNode within VisitedNodes" (the room's first-entry index, which
  is the path-relative label the player sees).
- **`RoomsTraversed`** (NEW, int column on `dnd_zone_run`) — monotonic
  step counter. Drives threat/SU ticks. Bootstrapped to
  `len(VisitedNodes)` for existing rows.

### Audit pass required

Every read of `r.CurrentRoom` and `len(r.VisitedNodes)` needs to be
classified:

- **"Path index" reads (keep as CurrentRoom)**: display headers
  ("Room 3/7"), encounter ID derivation (`encounterIDForRoom`), enemy
  salt seeds, harvest room keys, status/abandon strings, camp room
  index.
- **"Progress counter" reads (switch to RoomsTraversed)**: threat
  clock ticks, supplies ticks, ambient narration cadence, autopilot
  preflight, fatigue/exhaustion accrual if any.

Concrete callsite list will be produced during implementation; estimate
~10–15 callsites total.

## Fork re-pick

Re-entering a fork node clears its resolved choice in `node_choices`
(or equivalent). Next `!zone advance` shows the fork prompt fresh,
evaluated against current inventory (so a newly-acquired key unlocks
previously-locked edges).

Silent — no warning. Graph-back navigation means the player has
walked back through their previously-chosen path; nothing in
`VisitedNodes` is destroyed; the alternate branch they previously
took is still revisitable. There is no "discard" semantics.

## RoomsCleared semantics

Idempotent — exiting a room a second time doesn't re-append to
`RoomsCleared`. The "advanced past" flag is sticky.

## Preflight checks (rejections)

`!revisit` rejects with a clear DM when:

- Target room is not in `VisitedNodes` ("You haven't been there yet.")
- Target equals `CurrentNode` (no-op)
- Target is not adjacent to `CurrentNode` via any graph edge
  ("Room X isn't directly connected — backtrack one step at a time.")
- Active combat session in current room
  ("Finish the fight first.")
- Threat clock would tick past max (use existing camp-eligibility
  preflight pattern)
- SU would drop below survival floor (same)
- Character is dead / expedition is paused (same gates as `!zone advance`)
- Autopilot run is active

## Rollout phases

Tentative ordering. Each phase ships independently.

### R1 — schema + audit ✅ **DONE 2026-07-09** (branch `n1-restoration`)

Shipped: `rooms_traversed` column + `bootstrapRoomsTraversed` backfill;
`CurrentRoom` re-derived via `pathIndexOf(VisitedNodes, CurrentNode)`;
`appendVisited` / `appendClearedRoom` made set-semantic; `narrationCadence`
routed off `RoomsTraversed`; `advanceZoneRunNode` now returns the moved-to
path index. Tests in `zone_revisit_r1_test.go`. Full suite green. No
player-visible behavior change.

**Audit outcome — the callsite split was smaller than estimated (~10–15).**
Almost every `CurrentRoom` read is a *path index* and correctly stays put:
enemy/trap salts (`pickZoneEnemy`, `pickZoneTrap`, `rollTrapDamage`), loot
RNG seed, `encounterIDForRoom`, harvest room keys, camp `RoomIndex`, map
render, "Room X/Y" headers. Keying those on the node's first-entry index is
exactly what makes a revisited room resolve to *the same room*.

Only two reads were progress-shaped:
- `narrationCadence` → now `RoomsTraversed-1`, so a backtracking player
  draws fresh flavor instead of replaying the lines they read on the way in.
- `nextIdx := run.CurrentRoom + 1` (`dnd_zone_cmd_graph.go`) — not a
  progress read but a latent bug: "+1" only labels correctly while the
  player is at the frontier. `advanceZoneRunNode` now returns the true index.

**Nothing to route off `RoomsTraversed` for threat/SU** — there are no
per-room ticks to route (see the corrected Cost model above). The counter
currently drives narration cadence and stands ready for R2's cost decision.

Two behaviors were hardened in passing, both no-ops under forward-only
navigation but load-bearing the moment R2 lands:
- `VisitedNodes` is an ordered **set** — a re-entered node is not
  re-appended, so room numbers never renumber under the player.
- `RoomsCleared` is **idempotent** — exiting a room twice can't inflate it
  past `TotalRooms` and skew the "N/M rooms" renders.

### R2 — `!revisit` command ✅ **DONE 2026-07-09** (branch `n1-restoration`)

Shipped: `!revisit <N>` (alias `!zone revisit`), `adjacentNodes` reverse-edge
traversal, full preflight, +1 threat, `!map` now prints a **Back to:** strip
of legal targets. Fork re-pick still deferred to R3 — revisit refuses while a
fork prompt is pending, because both `!zone advance` and `!zone go` resolve
`node_choices` without checking which node the player is standing on.

**"No combat/trap re-trigger logic needed" was wrong — and it was an exploit.**
Terminal `CombatSession` rows gate *only* Elite and Boss rooms, at the doorway
check in `advanceOnceWithOpts`. An Exploration room re-enters
`resolveCombatRoom` unconditionally and a Trap room re-arms. Left alone,
`!revisit` would have been an infinite farm: step back one room, `!zone
advance`, repeat for loot and XP. Fixed with a `RoomIsCleared(CurrentRoom)`
early-return at the top of `resolveRoom` — a no-op forward-only, since advance
clears a room only *after* resolving it. Guarded by
`TestRevisitedRoom_DoesNotReResolve` and `TestFreshRoom_StillResolves`.

**Autopilot needed a stopgap, sooner than R5 planned.** Autopilot has no
on/off switch — `tryAutoRun` is ticker-driven for every active expedition on a
2h CAS cooldown. Without intervention the background walker marches the player
straight back out of the room they just revisited, and the whole feature reads
as broken. `grantAutorunGrace` stamps `last_autorun_at` on revisit, buying one
full cooldown window. **R5 still owes the real policy decision** (refuse to
auto-walk from a non-frontier node, vs. walk back to the frontier first).

Tests in `zone_revisit_r2_test.go`. Full suite green.

### R3 — fork re-pick

- On revisit landing into a fork node, clear `node_choices` for that
  node.
- Next advance re-prompts.
- Tests: lock-with-key acquired between two fork visits, alternate
  branch selection, no-change re-pick.

### R4 — UX polish

- DM line on revisit explaining the cost ("You retrace your steps to
  Room 3 (the fork). Threat +1, SU −0.5.").
- Surface `!revisit` in `!help` and `!zone` help blocks.
- Ambient/narration audit: any "you press deeper into the dungeon"
  lines that no longer fit when player is backtracking.

### R5 — autopilot guard

- `!expedition run` refuses to start while at a non-frontier node?
  Or auto-walks back to frontier first? Decide based on R1–R4 feel.
  Likely: refuse with a "use !zone advance to return to the frontier
  first" hint.

## Open questions (lower priority)

- Should there be a small flavor-only narrative beat the first time
  the player backtracks in an expedition? ("TwinBee notes you doubling
  back — there's no shame in it. Sometimes the answer was in a room
  you'd already passed.")
- Does a babysitter's safe-rest affect revisit cost? (Probably not —
  babysitter is a camp upgrade, revisit is movement.)
- Multi-region expeditions — does revisit work across region
  boundaries within the same expedition? (Initial answer: no, only
  within the current zone run; cross-region travel is its own seam.)

## Effort estimate

Comparable to a small Phase pass (G6-sized). Roughly:

- R1: 1 session (schema bump + audit + tests).
- R2: 1 session.
- R3: 0.5 session.
- R4: 0.5 session.
- R5: 0.5 session.

Total: ~3–4 working sessions.
