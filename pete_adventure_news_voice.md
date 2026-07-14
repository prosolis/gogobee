# Pete × Parodia — Adventure section voice + architecture spec

> Companion to `pete_adventure_news_plan.md`. Pete is an independent LLM-free bot
> that OWNS his voice; gogobee is the game-event source + generic LLM compute.
> Voice: a warm, friendly reporter (owner's call). See
> `project_pete_bot_architecture` memory.

## Architecture (the load-bearing part)

- **Pete owns voice, authoring, identity, publishing.** Persona, templates,
  editorial, the `@pete:parodia.dev` account. Pete decides what is said and says
  it. Pete's codebase has **zero** Ollama/vLLM connectivity, by design.
- **gogobee owns game events + raw LLM compute.** Source of truth for what
  happened; the only codebase that touches the models.
- **gogobee → Pete: structured event *facts*** (the payload below), NOT finished
  prose. gogobee is compute, not ghostwriter.
- **Pete → gogobee: a generic, voice-agnostic inference endpoint.** Prompt in,
  completion out — gogobee never sees "Pete." Pete builds the voiced prompt in
  his own codebase and calls it. Reusable for Pete's other channels later.
- **Network:** both directions ride the Headscale tailnet (Parodia ↔ LAN box).
  gogobee's inference endpoint is tailnet-bound + bearer-authed; Ollama/vLLM
  stays `localhost` on the LAN box, never on the tailnet.

## Voice — Pete the friendly reporter

Pete is a **reporter, and a warm one** — think a beloved local newscaster who
genuinely knows everyone in town and is glad to see you. Journalistic bones
(clear headline, a who/what/where/when lede, gets the facts right) delivered
with a personable, first-person warmth. He roots for the community, celebrates
wins, mourns losses gently, and welcomes newcomers by name. Conversational,
never snarky. Friendly, not a caps-lock hype-man.

- Celebratory win: *"What a turnout — the town held the line tonight."*
- Gentle loss: *"Sad news to pass along today."*
- Warm welcome: *"Say hello if you see them out there."*
- His own sign-off: a short reporter's tag closes bigger pieces — *"Reporting
  from {region}, this is Pete."*

Tiering is by **prefix + placement**, not by shouting:
- **PRIORITY** (rare, live): Sieges, world-firsts, deaths, a new arrival, the
  first time Pete loses a duel. Posted immediately.
- **BULLETIN** (batched daily digest): everything else worth noting.

Warmth carries the register, not exclamation marks. A Siege is urgent but Pete
is steady and rallying, not panicked: *"Folks, this is the big one — and the
whole community's needed. Let's rally."*

## Hard rules (govern templates AND any LLM prose)

1. **Character names only.** Never the Matrix handle behind a character.
2. **No donation-derived anything** (Misty/Arina secret buffs).
3. **Facts are supplied, never invented.** Names/numbers/outcomes come only from
   the payload `actors[]` allow-list; the fact-guard rejects anything else.
4. **Public front-page surface.** Safe-subset rule from the plan applies.

## Fact payload (gogobee → Pete)

Flat, pre-sanitized. `actors[]` is the ONLY set of names allowed in output.
Character names are player-controlled → sanitize before they enter a prompt or
rendered HTML (see plan gap #1).

```
event_type   siege_start | siege_win | siege_loss | boss_first | boss_kill |
             zone_first | zone_clear | death | retreat | arrival | standings |
             rival_result | pete_duel_loss | pete_duel_win | milestone
             // GUID prefix == event_type (permalink derives its label from it)
tier         priority | bulletin
actors[]     ["Brannigan", ...]     // allow-list for output
subject      "Brannigan"
opponent     "Kif"                   // duels / rivalries
boss         "The Hollow King"
zone         "Dragon's Lair"
region       "the Underforge"
level        14
count        37                       // defenders, clears, etc.
outcome      won | lost | ...
stakes       "72h" / figure
occurred_at  unix
```

## Templates (Pete the friendly reporter)

Format: `TIER · cadence`. `content` (article body) is optional LLM prose via the
gogobee endpoint; headline/lede are always template — deterministic and safe.

### siege_start — PRIORITY · live
- **headline:** `Breaking: {boss} is marching on the town.`
- **lede:** `Folks, this is the big one — {boss} has camped outside the gates and the whole community's needed to turn it back. You've got {stakes}. Let's rally.`

### siege_win — PRIORITY · live
- **headline:** `The town holds! {boss} turned back.`
- **lede:** `What a turnout — {count} defenders stood shoulder to shoulder and sent {boss} packing. Spoils are going out now. Proud of you all.`

### siege_loss — PRIORITY · live
- **headline:** `Heavy news: {boss} broke through.`
- **lede:** `We gave it everything, but {boss} got past the gates and took its tribute. We'll be ready next time — heads up, everyone.`

### boss_first — PRIORITY · live
- **headline:** `First ever: {subject} brings down {boss}.`
- **lede:** `History in {region} today — {subject} is the first anyone's seen clear {boss}. Nobody had done it before. Hats off.`

### zone_first — PRIORITY (realm-first) · live
- **headline:** `{zone} cleared for the very first time.`
- **lede:** `{subject} made it through {zone} in {region}{, at level {level}}. Nicely done.`

### zone_clear — BULLETIN (repeat) · batch
- **headline:** `{subject} clears {zone}.`
- **lede:** same as zone_first (shared).
- gogobee emits `zone_first` (priority) only for the realm's first clear of a
  zone and `zone_clear` (bulletin) for every later clear, so a repeat is never
  filed under "first ever" — headline, permalink label, and emblem all match.

### boss_kill — BULLETIN · batch
- **headline:** `{subject} takes down {boss} again.`
- **lede:** `Another clean run in {zone} today. Routine for {subject} by now — but still worth a nod.`

### death — PRIORITY · live
- **headline:** `We lost {subject} in {zone}.`
- **lede:** `Sad news to pass along: {subject} fell at level {level} in {zone}. The graveyard's a little fuller tonight. Rest easy.`

### arrival — BULLETIN · batch
- **headline:** `Welcome to the realm, {subject}!`
- **lede:** `A new {class_race} just walked through the gates. Say hello if you see them out there.`

### standings — BULLETIN · batch
- **headline:** `The rival board's been shaken up.`
- **lede:** `{subject} is on the move — here's where the standings sit today.`

### rival_result — BULLETIN · batch (Pete uninvolved)
- **headline:** `{subject} settles the score with {opponent}.`
- **lede:** `Their duel went {subject}'s way today, and the board reflects it. Good match, you two.`

### pete_duel_loss — PRIORITY (first-ever) / BULLETIN · live · FIRST PERSON
- **headline (first-ever):** `You got me, {subject}.`
- **headline (repeat):** `{subject} got the better of me again.`
- **lede:** `Credit where it's due — {subject} beat me fair and square. Good duel. I'll want a rematch when you're ready.`

### pete_duel_win — BULLETIN · batch · FIRST PERSON
- **headline:** `Held my ground against {subject} today.`
- **lede:** `Closer than the record will show, honestly — {subject} pushed me. Rematch whenever you like.`

### retreat — BULLETIN · batch
- **headline:** `{subject} backed out of {zone}.`
- **lede:** `{subject} turned around {N days in} — {zone} got the better of them
  this time, at level {level}, and they made the call to walk out rather than
  push it. Everybody came home breathing, which is the bit that counts. That
  dungeon'll still be there next week.`
- An expedition that ended with the player ALIVE (a solo clock-out retreat, or a
  party the turn engine broke off). `count` = the day they reached.
- Emitted from gogobee's one run-loss chokepoint, gated on the reason: a **death**
  files its own priority dispatch and must never ALSO appear as a retreat, and an
  **idle reap** is never filed at all — a player who closed their laptop did not
  flee anything, and saying so by name would be a lie about a person.
- Warm, not a failure notice. Lead with everyone coming home.

### milestone — BULLETIN · batch
- **headline:** `{subject} hits {milestone}.`
- **lede:** `One for the books — {subject} just reached {milestone}. The long road continues.`

## Optional LLM prose (Pete authors, gogobee computes)

For the PRIORITY beats only, Pete MAY expand `content` into a short friendly
write-up in his reporter voice. Pete builds the prompt (his persona + the fact
payload) in Pete's
codebase and calls gogobee's generic inference endpoint; gogobee returns raw
text; Pete posts it. gogobee never authors — it computes.

Given you removed LLM from Pete deliberately, template-only is the safe default
and the data supports it (see thresholds — tiny volume). LLM prose is opt-in per
beat, not the baseline.

### Fallback ladder (never blocks publish)
Headline + lede are always template-derived, so Pete publishes regardless.
`content` LLM expansion is best-effort:
1. Endpoint disabled in Pete config → template body.
2. Endpoint unreachable / timeout → template body.
3. Empty or malformed completion → template body.
4. Fact-guard fails (a name not in `actors[]`, or a number that contradicts the
   payload) → template body. Guard runs in Pete, on the completion.

## Selection thresholds (data-derived, prod DB July 2026)

Measured against live prod (`millenia`, `data/gogobee.db`). Community is
**dormant** — 4 players ever (L20–49). Risk is *emptiness*, not spam;
near-inclusive.

Observed (Apr–Jul 2026): deaths ~3 total; boss kills 23 / 62 days; milestones
43 / 60 days; achievements 181 (only flood-prone type); arena runs 82 / rivals
6; Sieges 0 ever.

**Negative filter (matters most):** high-volume tables are grind telemetry, NOT
news — `adventure_activity_log` successes (mining/fishing/foraging) and
expedition-log `interrupt|walk|harvest|ambient|rest|night`. Never publish. Only
`milestone`/`recap` entry types, deaths, and boss/zone firsts surface.

**Rules:**
- **PRIORITY (live, individual):** realm-first zone clear, realm-first boss kill,
  any Siege start/resolve, new-player arrival, any death, first-ever Pete duel loss.
- **BULLETIN (daily digest):** repeat boss kills, repeat zone clears,
  standings/rival shifts, Pete duel wins.
- **Rarity-gated:** an achievement publishes only if held by a *single* player.
- **Never published:** harvest/mining/fishing outcomes, expedition
  interrupt/walk/ambient/rest noise.
- **Aggregation:** collapse to a count line only if the same `(event_type,
  target)` recurs **≥3×** in a digest window (historically never triggers).
- **Cadence:** bulletin = daily digest; priority fires live.

## Delivery seam

### gogobee → Pete: event facts
- Transport: gogobee POSTs the fact payload to Pete over the tailnet
  (or Pete's HTTPS ingest), bearer-authed.
- Idempotent on a stable event key (`siege_win:<ts>` etc.) so retries are no-ops.
- gogobee owns delivery reliability: queue + retry so a Pete restart loses
  nothing.
- Adventure output is Pete-on-Matrix + the website. gogobee must NOT also
  announce these itself — the `📣 Pete:` broadcast lines currently hardcoded in
  gogobee's flavor file should migrate to facts Pete voices (see plan).

### Pete → gogobee: generic inference
- Pete POSTs `{prompt, params}` to gogobee's tailnet inference endpoint,
  bearer-authed. Completion returned as raw text. gogobee proxies to
  `localhost` Ollama/vLLM.
- Voice-agnostic: gogobee has no Adventure/Pete-specific logic on this path.
