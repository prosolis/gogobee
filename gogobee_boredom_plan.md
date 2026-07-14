# Bored Adventurers — autonomous expedition starts

**Premise.** A player who stops tending their adventurer doesn't stop having an
adventurer. After 24h with no Adventure action, the character gets restless and
leaves on an expedition by itself. It goes with the gear it already has and the
cheapest supplies it can afford. If you never arm it, the results are bad — and
that's the mechanic, not a bug.

## Why this is small

The autopilot is already almost fully autonomous. `runAutopilotWalkDriven`
(expedition_autorun.go:168) walks rooms, drives elite and boss fights on the real
turn engine, auto-camps, auto-harvests, rolls day rollovers, and auto-picks a
fork after 8h of silence. Nothing in that path ever needs a human.

The one thing that still needs a human is the *start*: `!expedition start <zone>
<loadout>` (dnd_expedition_cmd.go:281). So this feature is one new ticker that
performs that start, plus a clock to decide when.

## Decisions (locked)

| Question | Answer |
|---|---|
| Idle clock | **Any action against Adventure**, from any interface — not Matrix chatter, not autopilot writes |
| Threshold | 24h idle → the adventurer leaves. No warning DM. |
| Re-trigger | After a bored run ends, another 24h of silence starts another. Forever. |
| Zone | Easiest zone whose **level band** contains the character. Never trivial low-tier farming. |
| "Raid-shaped" | = **multi-region** zones (underdark, dragons_lair, abyss_portal). Avoided — *unless* they're the only at-band option (L13+), then allowed. |
| Supplies | **Lean** loadout (cheapest). Broke → no run. |
| Gear | Never bought, never equipped. This is the whole point. |

## §1 — The clock: `last_player_action_at`

The existing timestamps are all unusable:

- `player_meta.last_active_at` — auto-bumped on **every character save**
  (adventure_character.go:584). The autopilot saves constantly, so a bored
  character would refresh its own idle clock and never qualify again.
- `user_stats.updated_at` — bumped on every Matrix message in any room
  (stats.go:103). That's chat presence, not *game* action. Rejected per the
  "any action against Adventure, not just in Matrix" rule.
- `daily_activity` / `loadAdvDailyActivity` — unified across
  `dnd_expedition_log`, so the autopilot's own walk entries count as player
  activity. Same self-refresh problem.

**New:** `player_meta.last_player_action_at DATETIME`, written *only* by
`markPlayerAction(uid)` — a direct UPDATE, never routed through
`saveAdvCharacter`, so no ticker or autopilot path can touch it. Any future
non-Matrix entry point (web, Pete, API) calls the same helper.

Stamped from `AdventurePlugin.OnMessage` when the message is a real player
action: an `!` command in `advActionCommands`, or a reply to a pending
interaction (`p.pending`, adventure.go:889 — shop/blacksmith/pet prompts).

`advActionCommands` is the 47 names actually routed in `OnMessage`'s if-chain.
Note `Commands()` (adventure.go:158) only registers **28** — it's a help
surface, not a routing table, and is missing `expedition`, `zone`, `fight`,
`camp`, `extract`, `resume`, and more. A test greps the `p.IsCommand(ctx.Body,
"…")` literals out of adventure.go and asserts each one is in the set, so a
future command can't silently fall out of the clock.

## §2 — Zone pick: `pickBoredomZone(level, uid)`

`zonesForLevel` is wrong for this: it filters on **tier only** and ignores the
`LevelMin`/`LevelMax` fields that already exist on `ZoneDefinition` — which is
why a level-1 player can currently walk into a T3 zone. The boredom picker uses
the bands instead.

1. Candidates = zones where `LevelMin <= level <= LevelMax`.
2. Prefer **non**-multi-region (`IsMultiRegionZone`, dnd_expedition_region.go:135).
   Among those, lowest `Tier` — the "easiest at-level" rule.
3. Ties broken by **least-recently-started by this player**, so a bored
   adventurer rotates zones instead of grinding one forever.
4. If step 2 is empty, retry allowing multi-region. This only bites at L13+,
   where `dragons_lair` and `abyss_portal` are the only at-band zones and both
   are multi-region.
5. Still empty → no run.

Worked examples:

| Level | At-band | Picked |
|---|---|---|
| 5 | forest_shadows, sunken_temple (T2), manor, underforge (T3) | a T2 zone |
| 10 | underdark (multi), feywild_crossing | feywild_crossing |
| 12 | underdark (multi), feywild (T4), dragons_lair (multi, T5) | feywild_crossing |
| 15 | dragons_lair (multi), abyss_portal (multi) | dragons_lair — fallback fires |

## §3 — The run

Ticker `expeditionBoredomTicker`, 30m interval, skips the ambient quiet window.

Candidates: `player_meta` rows, `alive = 1`, `COALESCE(last_player_action_at,
created_at) < now-24h`, and `last_boredom_at` NULL or older than the 24h
cooldown. (COALESCE on `created_at` so a character made and never played still
qualifies, but not on its first tick.)

Per-user skips, all reusing existing guards: resting lockout, seated on someone
else's party, active expedition, resumable (extracted, pending `!resume`)
expedition, active zone run, active combat session.

CAS-claim `last_boredom_at` **before** starting, so a crash mid-start can't loop.
Then: `loadoutPurchase(zone.Tier, LoadoutLean)` → balance check → shared
`beginExpedition` helper → DM.

A zero-supply start is impossible (`SupplyPurchase.Validate`,
dnd_expedition_supplies.go:270, hard-rejects it), so the cheapest bored run costs
50 coins at T1 and 250 at T5. Can't afford it → no run, one DM (rate-limited by
the cooldown we already claimed).

## §4 — Refactor: `beginExpedition`

`expeditionCmdStart` is prompt-layer (:282–309) followed by a reusable
transaction (:369–424): balance check, debit, holiday/omen freebies,
`startExpedition`, `ensureRegionRun`, log, refund-on-failure. Extract that second
half into `beginExpedition(uid, c, zone, purchase, reason)` so the command and
the boredom ticker share one code path and one refund story.

## §5 — Why the results will be bad (no work required)

This all already exists; the feature just points it at neglected characters:

- The autopilot **never buys or equips anything** — verified, no `buy`/`equip`/
  `Debit` calls anywhere in expedition_autorun.go, expedition_autocamp.go, or
  dnd_expedition_autopilot_harvest.go. Stale gear stays stale.
- Lean supplies deplete fast → **Rationing** (−1 attack/skill) at 25% →
  **Severe** (−2, long rests blocked) at 10% → **Starvation** at 0, which forces
  an extract with a **20% coin tax** on the haul (dnd_expedition_cycle.go:140).
- The boss-safety gate holds the autopilot out of boss rooms below 80% HP, so an
  under-geared character stalls, camps, and burns supplies it doesn't have.

The neglected adventurer grinds mediocre, half-starved runs on rusting gear and
comes home taxed. Exactly the intent.

## §6 — Streaks: no credit for a run you didn't ask for

Bored runs write `dnd_expedition_log` entries, which `loadAdvDailyActivity`
counts as player activity — so without care a bored run would **suppress the
idle-shame DM and preserve the daily streak** for someone who hasn't shown up in
weeks.

Two halves, and only one needed fixing:

- **Bumping** the streak was already safe. `engaged` reduces to
  `LastActionDate == today|yesterday` (the `CombatActionsUsed`/
  `HarvestActionsUsed` clauses are dead — nothing in the tree increments them),
  and `LastActionDate` is only set by `markActedToday`, which is called only
  from real player commands. `beginExpedition` deliberately does not call it.
- **Shielding** was not. Both hold branches in `midnightReset` (the activity
  oracle, and the active-expedition safety net) would spare an absent player.

Fixed with `dnd_expedition.boredom` + `isBoredomDriven(uid, now)`, which requires
*both* halves: the player hasn't acted in 24h **and** their most recent
expedition is one the adventurer started for itself. A manually-started
expedition still holds the streak while the autopilot walks it — that behavior is
untouched. Reading the *most recent* expedition rather than the active one keeps
it honest on the night after a bored run ends, when its logs would otherwise
still read as player activity.

## §7 — Robbie

Robbie sweeps every non-slotted inventory item daily (ores, fish, junk,
treasure — `robbieQualifyingItems` takes all of them unconditionally) and pays
`Value / 4`. The autopilot deposits harvested loot straight into
`adventure_inventory` (dnd_expedition_harvest.go:478, dnd_zone_loot.go), so:

**Robbie funds the boredom loop.** Bored run → auto-harvested junk → Robbie
converts it to coins → pays for the next lean pack → forever. The "they go broke
and stop" poverty floor does not exist. Abandoned characters are perpetual
motion, and that is the accepted design.

What was *not* accepted was the noise: Robbie posts a public room announcement
per visit, so every abandoned character would file a daily bulletin about
somebody who stopped playing weeks ago. He now visits and pays silently for idle
players (adventure_robbie.go), gated on the same clock.

## §8 — The bug this nearly shipped with

`playerIsIdle` originally scanned `COALESCE(last_player_action_at, created_at)`
straight into a `*time.Time`, and `lastExpeditionByZone` scanned
`MAX(start_date)`. **modernc.org/sqlite rebuilds a `time.Time` from the column's
declared type, and both `COALESCE()` and `MAX()` erase it** — the value comes
back a string and the Scan fails.

`lastExpeditionByZone` failed loudly. `playerIsIdle` failed *open*: an unreadable
row counts as idle, so it returned `true` for every player alive, including one
who had acted an hour earlier. Shipped, that would have marched the entire server
into dungeons at once and silenced Robbie for everybody.

The unit tests missed it at first because they ran against empty tables, so the
scan never executed. Both now select the declared columns and fold in Go.

Note the asymmetry: `loadBoredomCandidates` uses `COALESCE` too and is fine —
it's evaluated *inside* SQL and never scanned. Only Go-side scans through an
aggregate or COALESCE lose the type.

## §9 — Status

Built, vetted, full suite green. **Not deployed, not committed.** Working tree
carries: db.go (3 columns), expedition_boredom.go (new), expedition_boredom_test.go
+ expedition_boredom_clock_test.go (new), dnd_expedition_cmd.go (beginExpedition
extraction), adventure.go (clock stamp + ticker), adventure_scheduler.go (idle
reaper gate), adventure_robbie.go (silent for idle), twinbee_expedition_flavor.go
(ExpeditionBoredomStart).

## §10 — Verified end-to-end against prod data

`internal/plugin/exercise_boredom_prod_test.go` (build tag `prodexercise`), run
against a throwaway copy of the live DB:

```
GOGOBEE_PROD_DB_DIR=<dbcopy> go test -tags prodexercise -run TestExerciseBoredomProd -v ./internal/plugin/
```

PASS. What it actually proved, on real characters with real gear:

- **The clock gates the sweep.** A control player stamped as having acted an hour
  ago was not a candidate and was not charged; every player who departed passed
  `playerIsIdle`. This is the assertion that would have caught the COALESCE
  fail-open bug (§8), which is why the control arm is not optional.
- **Real departures.** 3 of 5 candidates left. `@holymachina` (L20 cleric) →
  `dragons_lair`, `@quack` (L4 mage) → `forest_shadows`, `@camcast` (L1) →
  `crypt_valdris`. The other two stayed home for the right reasons and the test
  names them: one has no character, one never finished creation.
- **L20 into the raid, as designed.** `dragons_lair` is the only zone in
  `@holymachina`'s band, so the fallback fired and the departure DM carried the
  raid warning. Exactly the decision that was locked.
- **It bought nothing.** Coins debited equal the lean pack to the coin (250 at
  T5, 50 at T1/T2), and the equipment fingerprint is byte-identical across the
  run.
- **Flagged and cooled.** `boredom = 1` on every row, `isBoredomDriven` true, and
  a second tick 30 minutes later charged nobody a second time.
- **The autopilot walks it.** Rooms accumulate across segments.

And it drew blood on the first run: **`@quack` (L4 mage, stale gear) died four
rooms in.** That is the mechanic, working, on the very first exercise. Death is
`Kill()` — `alive = 0`, 6h respawn, auto-revived by the scheduler ticker
(adventure_scheduler.go:72), and `loadBoredomCandidates` requires `alive = 1`, so
a dead adventurer sits out its six hours and then gets restless again. Death
pauses the loop; it doesn't end it.

## §11 — Status

Built, vetted, full suite green, exercised end-to-end against prod data.
**Not deployed, not committed.**
