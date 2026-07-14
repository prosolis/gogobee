# Mischief Makers — paid monster hits on live expeditions

Working-tree plan doc. Do not commit.

## The pitch (restated)

Parodia members spend euros to send a monster — grunt / mob / elite / boss — at a
player who is out on an expedition. Orders come from Pete's Adventure web page
(and from Matrix). The games room hears that a contract is out and gets a window
to help the target… or "help" them. Surviving an attempt pays the target.

## What the codebase already gives us (verified 2026-07-13)

| Need | Existing machinery |
|---|---|
| Currency + escrow | `EuroPlugin.Debit/Credit` (`euro.go:364,:395`), duel escrow pattern (`adventure_duel.go:422,:529,:667`), community pot (`adventure_rival.go:193-241`) |
| "Is X out right now?" | `getActiveExpedition` (`dnd_expedition.go:215`), roster already pushed to Pete every 2 min |
| Mid-run combat injection | `runHarvestInterrupt` (`dnd_expedition_combat.go:121`) — picks enemy, surprise nick, `runZoneCombatRoster`, full win/retreat/death close-out. **This is the template.** |
| Safe-delivery gating | ambient ticker's CAS claim + `hasActiveCombatSession` + quiet-window checks (`expedition_ambient.go:97,:109,:156`) |
| Grunt→boss ladder | Arena tiers 1–5 (`adventure_arena_monsters.go`), `arenaMonsterToTemplate` (`:251`); zone pools tag `IsElite` |
| Games-room announce | `gamesRoom()` + world-boss announce helpers (`adventure_worldboss.go:528-566`) |
| Respond-within-window | duel challenge window + atomic single-winner claim (`adventure_duel.go:43,:85`) |
| Survival payout kit | `euro.Credit`, `rollAdvTreasureDrop`, `consumableCache`, renown, world-boss payout bundle (`adventure_worldboss.go:466-493`) |
| Pete pipe | durable fact queue, bearer auth, event-type taxonomy; **strictly one-way gogobee→Pete today** |

Hard constraint (Pete `internal/web/roster.go:23-25`): *"Pete has no route back
into the game box's network and we are not opening one."* We honor that: the web
storefront stores orders in Pete's DB and **gogobee polls Pete** (gogobee stays
the only initiator, same tailnet + bearer pattern as ingest, just a GET).

## Design

### Contract lifecycle

```
placed ──announce──▶ open (help/"help" window, default 60 min)
   │                    │
   │                    ├──▶ delivered  (next safe tick: target active, no combat,
   │                    │                not in briefing/recap quiet window)
   │                    │       ├── target survives  → SURVIVED payout + unseal buyer
   │                    │       └── target defeated  → DOWNED consequences
   │                    └──▶ fizzled    (expedition ended first → refund minus rake)
   └── rejected at claim time (funds/eligibility) → never announced
```

- New table `mischief_contracts`: id, buyer_id, target_id, tier, fee, status
  (`pending|open|delivered|fizzled|rejected`), signed (bool), escalation_count,
  blessing json, source (`matrix|web`), created_at, window_ends_at, resolved_at.
  Status transitions via conditional UPDATE (CAS), mirroring `deliverAmbient`.
- One live contract per target at a time; delivery fires from a sibling of the
  ambient ticker after `window_ends_at`.

### The four tiers — PRICED (M0 done 2026-07-13)

Monster strength is **relative to the target**: pull from the target's own
bracket **zone pools** (grunt/mob = non-elite, elite = `IsElite`, boss = the
zone boss). NOT the arena ladder — arena `BaseLethality` is a legacy death
chance; arena T5 one-shots L20s and would make every tier an execution.

**M0 survival sweep** (n=2000/cell, `SimulateCombat` via the class-balance
harness builds at full HP, zone-pool monsters, ambush nick on elite/boss;
throwaway harness left skip-gated at
`internal/plugin/mischief_pricing_sweep_test.go`, env `MISCHIEF_SWEEP=1`):

| target build | grunt | mob (3 chained) | elite | boss |
|---|---|---|---|---|
| paladin L1 | 99.8% | 98.2% | 57.6% | 15.3% |
| mage L4 | 100% | 97.8% | 49.5% | 0.0% |
| mage L8 evoc | 100% | 100% | 56.5% | 0.4% |
| fighter L12 champ | 100% | 100% | 100% | 99.9% |
| rogue L20 AT (prosolis) | 100% | 100% | 100% | 35.9% |
| cleric L20 life (holymachina) | 100% | 100% | 100% | 14.1% |

Reading: grunt/mob are pure theater (+HP/supply attrition); elite is a real
coin-flip for at-bracket targets and trivial for overleveled martials; boss is
expedition-ending for casters at any level and a genuine 1-in-3 scare even for
the L20 rogue. Caveats: harness builds ≠ real sheets, full-HP-at-delivery is
optimistic (mid-run wounds raise real danger), no pets/party/consumables.

**Prod economy snapshot** (2026-07-13, 23 wallets): median balance ≈ €500;
whales prosolis €180k / holymachina €149k / nonk €62k hold 89% of all money.
Daily earn: whales ~€1.8–2.3k, mid-tier €20–500, casuals <€10. Reference
sinks: lottery ticket €100, duel escrow €1.5–5k, daily share ~€455, endgame
shop items €11–45k. Only 5 characters exist as targets (two L20s, L4, two
L1s) — the whales are both the richest buyers *and* the only high-bracket
targets, so boss-tier is effectively their PvP toy; casuals buy theater.

| Tier | Fee | Signed (+25%) | Survival payout (of fee) | Extras on survival |
|---|---|---|---|---|
| grunt | €40 | €50 | 40% → €16 | flavor line |
| mob | €100 | €125 | 50% → €50 | treasure roll (standard) |
| elite | €350 | €438 | 65% → €228 | treasure roll (elite) + renown |
| boss | €1,200 | €1,500 | 75% → €900 | elite treasure + consumable cache + renown + Pete milestone |

Rationale: grunt under the lottery-ticket impulse line so anyone can play;
mob = exactly one lottery ticket; elite ≈ one active day of mid-tier earnings
(a considered purchase, and its ~50% at-bracket survival odds are honest
stakes); boss in duel-stake territory — whale money, priced like the "end an
expedition" button it is against caster targets. Escalation cost = the
tier-delta fee; blessings €25 flat.

Anti-collusion invariant: **total payout (fee + escalations) capped at 75%**
in every case, so collusion is strictly dominated by `!baltransfer`, which is
free. No danger multiplier needed — the cap does all the work.

Money flow: survival payout to target, remainder → community pot. Defeat →
entire fee to community pot (never back to the buyer — glory only). Fizzle →
90% refund, 10% rake to pot.

Boss-tier friction (0–14% caster survival means "boss ≈ certain maiming"):
max 1 boss contract per target per week, on top of the global limits below.

### Death policy (recommendation: mischief maims, it doesn't murder)

`runHarvestInterrupt`'s loss path perma-kills (`abandonZoneRun` +
`forcedExtractExpedition` + mark dead). For a *purchased* attack that lands
while the victim is offline, perma-death feels like paid murder, not mischief.
Recommended: on defeat, apply the world-boss HP floor (`worldBossFloorHP`
pattern), force-extract the expedition (run-loss seam already exists), drop a
chunk of un-banked expedition coins/supplies, +threat. Character lives.
**Open decision** — could make boss tier lethal for stakes, but then it needs an
opt-in (hardcore flag) or it's a griefing lever.

### Anonymity + the unseal twist

Default: contracts are anonymous — "someone in town has put coin on
<target>'s head." Buyer can pay +25% to *sign* it (taunt rights).
**If the target survives, the contract is unsealed** — Pete names the buyer in
the survival bulletin. Risk of exposure is the natural brake on casual griefing,
and it makes survival announcements delicious.

### The games-room window (help or "help")

On placement, announce in `GAMES_ROOM` (world-boss style) + Pete priority beat.
During the window (default 60 min):

- `!mischief bless @target` — pay a small fee (~€15) for temp HP / +AC on the
  incoming fight (well-rested-style bundle). Stacks, capped (say 3 blessings).
- `!mischief escalate @target` — pay ~half the next-tier delta to bump the
  contract one tier, max one step total, boss can't escalate. Escalation money
  joins the payout escrow — piling on raises the target's survival jackpot.
- Victim gets a TwinBee DM (first person, per voice rules): word has reached
  them that someone wants them dead. Pure flavor — expeditions are autonomous —
  but it lets them rally blessings.

Blessing/escalation state lives on the contract row (real rows, no phantom
per-fight state — lesson from the companion free-lunch bugs).

### Delivery mechanics

New `runMischiefInterrupt`, modeled line-for-line on `runHarvestInterrupt`:
pick monster per tier table → apply blessings to the roster → surprise nick
(attacker's privilege) → `runZoneCombatRoster(fightRoster(target), …)` → custom
close-out. Do **not** call `recordZoneKill`/advance zone state — the fight is
extrinsic to the dungeon. Party seats fight together and earn seat XP as usual;
the survival purse goes to the target (leader).

Fizzle: if the expedition ends before delivery, refund fee minus 10% rake
(deadpan Pete line: the monster arrived to an empty dungeon).

### Rate limits & eligibility (anti-grief)

- Target must have an active expedition **and** be level ≥ 3 (or similar floor).
- One live contract per target; per-target cooldown 24 h after resolution.
- Per-buyer cap: 2 contracts/day. Can't target yourself. Escrow debited at
  placement via `euro.Debit` (respects the debt floor for free).
- Boredom-ticker auto-expeditions: **targetable** (they're real runs and it's
  funny), but revisit if it feels bad in practice.
- Opt-out: none for v1 — being in the world means being in the world — but keep
  the decision explicitly revisitable. News anonymization (`!news optout`)
  still applies to Pete-facing names as it does today.

### Pete web storefront (the reverse pipe)

**Decision (2026-07-13): mischief UI requires Authentik sign-in on Pete.**
Pete's OIDC layer already exists (`[web.auth]`, disabled by default) and the
callback already parses `preferred_username` from the ID token
(`auth.go:236-250`) — it just isn't persisted into the `pete_session` cookie.
**VERIFIED 2026-07-13** on parodia.dev (`matrix-mas-1`,
`/home/reala/matrix/compose/mas/config.yaml`): MAS imports localpart with
`action: require, template: '{{ user.preferred_username }}'` — so
**Authentik username == Matrix localpart**, guaranteed. Identity is free:
add PreferredUsername to `SessionUser`, gogobee maps it to
`@<username>:<server>` (lowercase it — Matrix localparts must be lowercase,
Authentik usernames may not be). No link codes.

The one-way *network* rule (`roster.go:23-25`) stays intact — both new data
flows are gogobee-initiated:

- **Balances out (push):** gogobee already pushes a roster snapshot every
  2 min; add euro-balance entries for linked users on the same tick. Pete
  renders "~€X" and greys out unaffordable tiers. Advisory only; up to 2 min
  stale is fine.
- **Orders in (poll):** signed-in buyer POSTs the order form → `mischief_orders`
  row in pete.db keyed by preferred_username, status `pending`. gogobee's
  peteclient grows a poll loop: `GET /api/mischief/pending` (bearer) every
  30 s, claims each order (`POST /api/mischief/claim`, idempotent on order
  GUID), then validates in-game.
- **Money truth lives only in gogobee:** the authoritative check is
  `euro.Debit` at claim time. A stale web balance just means an occasional
  "order bounced — insufficient funds" status (rendered from a fact) + TwinBee
  DM to the buyer. Pete never writes a balance, so no double-spend surface.

Order targets: roster board (already live on `/adventure`) grows a "send
trouble" button per entry (roster token identifies the target — already
stable + non-deanonymizing). Pete renders order status from resulting facts,
never from live game state.

Ops note: enabling `[web.auth]` on prod Pete needs the Authentik OAuth client
provisioned + config.toml edit on the box (config is box-only).

### New Pete event types (deploy Pete FIRST — unknown types 400 → park forever)

`mischief_contract` (priority: a hit is out, tier, anonymous-or-signed),
`mischief_survived` (priority: target thwarted it — unseal buyer here),
`mischief_downed` (priority), `mischief_fizzled` (bulletin),
`mischief_link` (internal, no render — or handle out-of-band). Deadpan voice
throughout; check `adventure_flavor_*.go` for reusable lines before writing new
flavor (standing rule).

## Phases

- **M0 — price the tiers. DONE 2026-07-13** (single-fight sweep + prod
  economy snapshot; see tier table above). Follow-up for M1 close-out: re-run
  the sweep through the *real* delivery path once `runMischiefInterrupt`
  exists, since full-HP single fights understate mid-run danger.
- **M1 — core engine, Matrix-only. DONE 2026-07-13 (uncommitted, not deployed).**
  `adventure_mischief.go` (tiers/pricing/persistence/eligibility/`!mischief`),
  `adventure_mischief_deliver.go` (ticker + `runMischiefInterrupt` + close-outs),
  `mischief_contracts` table, Pete's 4 `mischief_*` event types. 17 tests,
  4 of them end-to-end through the real fight + euro + extraction.

  Decisions taken during implementation (all inside the plan's intent):
  - **Bracket monster-selection promoted out of the M0 test** into prod
    (`mischiefBracketZone`/`mischiefMonsters`), so the sweep and the live
    delivery now drive *the same* selection code — the fee table can't drift
    away from the fight it priced.
  - **Survival is read off the target's HP, not `PlayerWon`.** The engine's
    timeout is a retreat, not a lethal blow: a target who ran out the clock
    with HP left held the thing off, and a bought monster that merely outlasted
    them hasn't earned a maiming. (M0 counted `!PlayerWon` as death, so real
    survival rates are a touch *better* than priced. Pricing stands.)
  - **No-perma-death extended to the whole party** (`floorMischiefRoster`, run
    on BOTH outcomes). A leader can win a fight their friend went down in, and
    the delivery skips `closeOutZoneWin` — without the floor, the member is left
    alive at 0 HP, which every `HPCurrent <= 0` gate reads as broken.
  - **One-live-contract-per-target is a partial UNIQUE INDEX**, not a read-then-
    write check: placement holds only the *buyer's* lock, so two buyers racing at
    one victim would both pass an in-code check. The loser is refunded.
  - **Stale-delivery sweep** (`delivering` + 15 min grace → full refund). Without
    it a crash mid-fight strands the row: target permanently un-targetable,
    buyer's money gone.
  - All contract timestamps bind as Go `time.Time` — never `CURRENT_TIMESTAMP`.
    The driver stores RFC3339 (`2026-07-14T03:48:52Z`); a SQL-side stamp writes
    `2026-07-14 03:48:52`, and the two compare lexicographically wrong.
  - Fizzle DMs + rake, 90% refund; unstageable/stranded contracts refund 100%
    (our fault, not a bet they lost).

  **M1 close-out sweep DONE 2026-07-13** (`mischief_delivery_sweep_test.go`,
  `MISCHIEF_SWEEP=1`, n=400/cell, 108 cells through the REAL delivery path:
  `runMischiefInterrupt` → `runZoneCombatRoster`). Arms: entry HP 100/70/40%
  (100% = control, reproduces M0 through the new path) × wards 0/3 on elite+boss.

  | target | tier | 100% HP | 70% HP | 40% HP | 70% +3 wards |
  |---|---|---|---|---|---|
  | paladin L3 | elite | 84.8% | 73.2% | 62.7% | 80.5% |
  | paladin L3 | boss | 87.8% | 45.8% | 8.0% | 88.0% |
  | mage L4 | elite | 66.0% | 29.5% | 4.0% | 64.0% |
  | mage L8 evoc | elite | 90.2% | 72.2% | 15.0% | 84.2% |
  | mage L8 evoc | boss | 6.5% | 2.0% | 0.2% | 7.8% |
  | fighter L12 | boss | 88.2% | 48.0% | 6.8% | 89.8% |
  | rogue L20 | boss | 19.2% | 2.0% | 0.0% | 18.5% |
  | cleric L20 | boss | 42.0% | 6.0% | 0.0% | 42.8% |

  (grunt/mob: 100% for everyone except mage L4, who can lose a wounded mob.)

  Three findings:
  - **The M0 table was priced on a fight we don't deliver.** The control arm
    diverges from M0 in BOTH directions: up where an engine timeout now counts as
    survival (paladin boss 15→88%), and *down* where the turn engine loops a
    boss's full multiattack profile and `SimulateCombat` doesn't (fighter boss
    99.9→88%, rogue boss 36→19%) — see [[project_sim_multiattack_gap]]. Fees kept
    (the tiers still do what they were priced to do); this table is now the
    reference.
  - **Wounding is the dominant variable, and M0 was blind to it.** Boss at a
    realistic mid-run 70% HP collapses across the board (fighter 88→48, cleric
    42→6, rogue 19→2); at 40% it is near-zero for everyone. Boss really is the
    "end their expedition" button, as priced.
  - **Wards were too cheap** (see M2 below): 3 of them buy ~+40pp.
- **M2 — the window. DONE 2026-07-13.** `!mischief bless @user` (€25, cap 3,
  +10% MaxHP temp HP each) · `!mischief escalate @user` (pays the tier delta,
  one step, boss is the ceiling) · victim DM on placement · escalator unsealed
  alongside the buyer on survival. 6 new tests (22 total).

  Decisions taken during implementation:
  - **Blessing fee is a pure sink** (community pot), never a purse top-up. If a
    ward raised the payout basis, warding a friend would become a money-routing
    move; buying them HP can't be.
  - **Ward price scales with the contract** — `max(€25, 10% of fee)`: grunt/mob
    €25, elite €35, boss €120 (€360 to cover someone completely). The close-out
    sweep measured 3 wards at ~+40pp (fighter vs boss at 70% HP: 48→90%), so a
    flat €25 let the room halve a €1,200 boss contract for €75 — pocket change
    against the tier the whole economy rests on. Reads the contract's *current*
    basis, so an escalation raises the price of saving its target too. The €25
    floor is knowingly above-market at grunt (3 wards > the €40 fee): nothing
    needs warding against theatre, and the floor exists to keep a ward an impulse.
  - **Escalation raises `fee` (the payout basis) AND `paid` together**, so the
    75% cap and "purse < outlay" survive an escalation untouched — asserted.
  - **Escalating into boss answers to the boss-per-target-per-week cap.**
    Otherwise the cap is bought around for the price of an elite plus the delta.
  - **Both window commands are CAS-then-refund**, like placement: the ward cap and
    the one-step limit live in the UPDATE's WHERE clause, because a scramble is
    exactly when four people press the button in the same second.
  - **The delivery re-reads the contract AFTER claiming it.** The due-sweep's copy
    is stale by construction — the window keeps writing to an open row up to the
    claim, so a last-second escalation was paid for and then delivered the *old*
    tier's monster. The claim is the fence. (Fixed + regression test.)
  - **The ward is temp HP** (the well-rested mechanism), added to whatever cushion
    the target already carried and subtracted again after the fight — a bought ward
    must not eat a home long rest. It refreshes per link of the mob chain; that only
    affects the one tier the sweep priced as theatre.
  - Blessers are named on the spot (helping is proud); escalators are anonymous
    until the survival unseals them, exactly like the buyer.
  - Fizzled/unstageable contracts do **not** refund blessers. Their €25 was charity,
    and it went to the pot. Revisit if it stings in practice.
  - Fixed a pre-existing wall-clock flake in the M1 fizzle test: it drove
    `fireMischiefDeliveries` with `time.Now()`, which refuses to run inside the
    ±1 h quiet windows around the 06:00 briefing and 21:00 recap.
- **M3 — Pete storefront (1 session, cross-repo).** Enable + extend OIDC
  (persist preferred_username; provision Authentik client), orders table +
  form + status rendering, balance push, gogobee poll/claim loop. First
  visitor-facing write path on Pete — auth-gated + rate-limited per user.
  (MAS localpart == Authentik preferred_username: verified 2026-07-13.)
- **M4 — polish.** Notoriety/renown for buyers, survival streaks → milestones,
  digest lines, boredom-expedition policy review, tune fees from live data.

## Decisions — ALL CONFIRMED by reala 2026-07-13

1. **No perma-death anywhere** — mischief maims: HP floor + force-extract +
   un-banked loot/supply loss + threat bump. Boss tier included.
2. **Anonymous by default; survival unseals the buyer** in Pete's bulletin;
   +25% to sign openly.
3. **Fizzle refund 90%**, 10% rake to community pot.
4. **Targets: level ≥ 3 with an active expedition**; boredom auto-runs are
   fair game. One live contract per target, 24 h post-resolution cooldown,
   2 contracts/day per buyer, 1 boss/target/week.
5. **Fee/payout table as priced above** (M0 sweep + prod economy).

## Standing-rule checklist

- No new `dnd_`-prefixed files/tables → `adventure_mischief*.go`, `mischief_*` tables.
- TwinBee first-person; Pete deadpan third-person announcer.
- Plan doc stays working-tree only.
- Pete deploys before gogobee whenever event types change.
- Sim sweeps: control arm + n≥750; run heavy sweeps on Parodia/millenia.
- Any loader/bootstrap added must keep its backfill as a one-shot bootstrap.
