# Pete / Gogobee Adventure News Integration — quick plan

> High-level only. No assumptions about current code shape; codebase has moved
> since Pete was last touched. This is a scoping doc, not an implementation plan.

## What this is

A news.parodia.dev section where Pete reports on Adventure module activity:
deaths, boss defeats, new players, notable events, and periodic generated
recaps. Distinct from the in-bot commands (`!town`, `!graveyard`, `!rivals
board`) that already surface this data on request — this makes the same kind
of information ambient and discoverable instead of command-gated.

## Relationship to existing plans

- **Converges with "Phase 16 Pete" (the deferred Pete bot) — doesn't collide.**
  The news feed is the near-term, separable slice and ships first. But the
  Phase 16 bot turns out to be the thing that makes the feed *first-person*: an
  adventuring Pete is an embedded correspondent who files dispatches from
  dungeons he's actually in. Feed first, bot later, same character. See "Pete as
  a character" below.
- **Overlaps with Track E (E3 — surfacing the buried social data).** E3 builds
  `!town`, `!graveyard`, `!rivals board` as pull-based bot commands. Pete's
  news feed is the push-based counterpart to the same underlying data. Ideally
  they share one source of truth rather than each growing its own read path.
- **Depends on C3 (World boss / "the Siege") shipping.** That's sequenced in
  N6, well after N4's town work. Pete's biggest story beat — the town under
  attack — doesn't exist as an event to report on until C3 lands. Fine to plan
  for now, but the event-feed and stats sections can go live well before this
  piece is reachable.

## Content types

1. **Event feed (low effort, high value)**
   - Deaths (source, location) — same data `!graveyard` already surfaces.
   - Boss defeats — first-clears, notable kills.
   - New player arrivals.
   - Zone first-clears / notable milestones.
   - **World boss / "the Siege" (C3).** This is the marquee story beat: a
     named boss camps outside town for 72h with a shared community HP pool.
     Pete should report both ends of it — the announcement/start of a Siege,
     and the resolution (boss falls and payout goes out, or it survives and
     "loots the town," a visible pot tribute). The town-attack framing already
     baked into C3's design (it's explicitly a communal threat, not just a
     tougher solo fight) makes it the best fit for the generated-article
     treatment, not just a log line.
   - This is closest to a straightforward log-to-article pass. Good starting
     point since it needs the least judgment about what's "interesting."

2. **Generated articles (higher effort, more judgment required)**
   - Narrative recaps of notable happenings — a boss kill or first-clear
     written up as a short story beat rather than a log line, in the same
     spirit as the flavor-voice work already done elsewhere in the bot
     ecosystem.
   - LLM-authored via Gogobee's existing inference access, following the same
     pattern Petal already established: model as a config value behind an
     abstraction, not hardcoded. Pete's news writing shouldn't need its own
     bespoke integration if that seam already exists.
   - Event data (who, what, where, stakes) stays structured and feeds the
     model as context; the model's job is prose, not fact invention. Worth
     deciding early whether there's a review/approval step before publish, or
     whether it goes straight to the site — the risk profile is different
     between "reads a little dry" and "confidently wrong about who died."
   - Open question: live per-event, or batched into a daily/weekly digest.
     Batching gives editorial control over what's worth writing up instead of
     reporting every routine kill, and is also cheaper on inference calls than
     firing one generation per event — probably the safer default to start.

3. **Player stats**
   - Needs the same leak-check the engagement plan already flags for E3:
     nothing that ranks or badges donation counts (Misty/Arina), since that
     could let players reverse-engineer secret NPC buffs. Tax-ledger totals
     were called out as safe; donation counts were not. Any stat Pete
     considers for public display should be run through that same filter
     before it's greenlit, not decided ad hoc per-stat.
   - Safer starting stats: playtime/level distributions, zone clear counts,
     arena standings, rival board — anything already deemed room-safe under
     E3's design.

## Sequencing suggestion

1. Event feed first (deaths, boss defeats, new players) — mechanical, low
   judgment, reuses E3's data model.
2. Stats section next, gated through the leak-check before anything ships.
3. Generated articles last — highest creative/quality bar, and benefits from
   having the event feed already running as its raw material.

## Resolved decisions

- **Public-facing — yes.** The wall isn't what protects players; the leak-check
  and name-scoping are, and those apply either way. So publish publicly, but
  only the public-safe subset, decided on purpose:
  - **Safe to publish:** boss first-clears, Siege announcements/resolutions,
    zone clear counts, arena standings, aggregate level/playtime distributions,
    the rival board.
  - **Community-only or omit:** anything donation-derived (Misty/Arina secret
    buffs), anything that names a Matrix account, and raw per-player time series
    fine-grained enough to diff and reverse-engineer a hidden buff. Public
    raises the adversary from "a curious member" to "anyone with a scraper," so
    the leak-check is load-bearing, not advisory.
  - **Name-scoping:** report on **character names only**, never the Matrix
    handle behind them. Consider a per-player opt-out of being named in
    articles. Decide before launch — retroactively scrubbing an indexed/cached
    public page is the URL-preview problem again.

## Pete as a character (forward tier — after the feed + section land)

The most ambitious slice, and the payoff for the whole arc: Pete stops being a
wire service that reports on others and becomes a **playable character in the
world** — the realm's embedded correspondent. Converges with the deferred
Phase 16 Pete bot. Two surfaces, both feeding the news:

1. **Solo runs.** Pete runs his own expeditions on autopilot (the existing
   `runAutopilotWalk` path the sim already drives) and files first-person
   dispatches: *"Your correspondent nearly didn't make it out of the
   Underforge."* His deaths/level-ups/kills emit the same fact payload every
   player does, so the news writes itself — and the fact-guard is trivially safe
   when the subject is Pete himself.

2. **Hireable companion (class-fluid).** Players short on help can hire Pete
   into their party. Unlike a player character he is **role-fluid** — he fills
   the gap: no healer → Cleric, no damage → Mage, need a body up front →
   Fighter. Default behavior is auto-fill the missing role, with a manual
   override. When hired, that expedition becomes a dispatch too: *"Filled in as
   cleric for [party] today — nobody died on my watch."*

3. **Standing rival (the house challenger).** Players can challenge Pete to a
   duel any time via the existing C2 staked/no-death system — so there's always
   an opponent even when the game is quiet (same trailing-case lift as hiring,
   for the PvP side). He **responds to win and loss himself**, first person and
   gracious either way — his losses are the better content. First time anyone
   beats him is an alert. Rival results feed the news via `adventure_rival_records`.

### Identity — Pete owns Pete (corrected after reading the code)

`@pete:parodia.dev` is already an **independent bot** (pete repo) with its own
account; gogobee ignores it via `IGNORED_BOTS`. So Pete is NOT a gogobee
appservice ghost — his Matrix presence is his own bot, and **Pete owns his own
voice** (persona, templates, editorial). See `project_pete_bot_architecture`.

- **Pete's surfaces (all his bot):** the news website, his Matrix posts, and —
  for the character tier — duel reactions / hire banter / replies. He's already
  an in-room command bot (`!post`, `!petestats`), so he has the interactive
  skeleton; the character behaviors extend it.
- **gogobee's job:** emit game-event *facts* to Pete, and provide LLM *compute*
  via a generic endpoint. It does NOT voice Pete. The `📣 Pete:` broadcast lines
  currently hardcoded in gogobee's flavor file are the old two-Petes situation
  and should migrate to facts Pete voices.
- **LLM:** Pete stays LLM-free in his own codebase (deliberate — it was
  removed). When he wants prose he calls gogobee's generic tailnet inference
  endpoint. Default is template-only; LLM is opt-in per marquee beat.

Why this fits the existing guardrails:

- **It's the sanctioned difficulty lever.** A short-handed / missing-a-role
  party is the trailing case; a hireable Pete lifts exactly that party without
  nerfing leaders or touching monster scaling. Tune him as a competent
  *below-median* member of whatever class he plays — help, never a carry.
- **Balance corpus stays clean.** Golden pins run solo, so baseline sims never
  hire Pete. Hiring is a live player-help feature entirely outside the baseline;
  the two paths never cross.
- **Gold sink.** Hiring costs gold per expedition — economy pressure of the same
  kind the tax-ledger / lottery tuning already wants. Scarcity is a knob:
  Pete "on assignment" (off covering a Siege) is both a reason he's sometimes
  unavailable and a story in itself.
- **Duel economy caution.** As a standing rival he's *beatable* by design — so
  his duel stakes must be bragging-rights / bounded, NOT real gold, or players
  farm the house challenger for income. Tuning-pass number; flagged so it isn't
  discovered as an exploit.
- **Guardrails.** Pete keeps clear of the Misty/Arina secret-buff mechanics (a
  public reporter benefiting from hidden buffs and reporting outcomes is a leak
  vector). His voice is his own — NOT TwinBee's.

Open tuning-pass questions (not blockers): hire cost (flat vs level-scaled);
his power scaling (fixed level vs scales-to-party); availability (always vs
cooldown / on-assignment scarcity).

## Gaps & risks register

The design's blind spot was treating the gogobee→Pete channel as internal and
trusted — it's neither once player-named characters and LLM prose flow onto a
public front page. Seven gaps, three of them must-solve-before-launch.

1. **[MUST — security] Untrusted input into LLM prompt + public HTML.** A
   character name is player-controlled. It reaches (a) the LLM prompt →
   prompt-injection, and (b) rendered `content` → XSS. Fix: names get escaped
   before the prompt; Adventure `content` goes through Pete's **existing** RSS
   sanitizer (it already strips inline scripts), never around it.
2. **[MUST] `article_url` has nowhere to point.** Schema is `NOT NULL`; a
   self-hosted event has no external article. Point it at a Pete permalink
   (`/adventure/<guid>` / the existing per-story reader page).
3. **[MUST] Visual identity.** No OG images → the section looks broken next to
   RSS cards. Ship per-event-type art (Siege banner, graveyard mark, Pete's
   avatar for his own dispatches).
4. **[RESOLVED] Selection/aggregation thresholds.** Data-derived from the prod
   DB — see the thresholds section in the voice spec. Dormant community → the
   risk is emptiness, not spam; near-inclusive, with grind telemetry filtered
   out and achievements rarity-gated.
5. **[RESOLVED — security] Opt-out.** Default-featured (informed, announced at
   launch), one gogobee command backed by a `news_optout` table (mirrors the
   existing `shade_optout`). Enforced at emit time. Opt-out **anonymizes, not
   deletes** ("an adventurer…"). Retroactive: `POST /ingest/adventure/scrub`
   anonymizes existing rows; whole section is `noindex` to bound the
   cached-forever problem.
6. **[RESOLVED — security] Kill-switch.** Global `adventure_news_enabled` flag
   in gogobee kills emission at the source (generation is gogobee-side, so this
   is the real switch). Per-story **admin delete on Pete**, reusing the OIDC
   `adminSubs` gate that already guards `/status`.
7. **[RESOLVED] Cold-start backfill.** Guarded one-shot in gogobee replays the
   back-catalog (realm-firsts, deaths, notable clears, single-holder
   achievements) through the emit path. Backdated `published_at`, dispatch tone,
   **no web-push**. Idempotent on `guid`; kept as a marked one-shot (not deleted
   at close-out) so fresh deploys don't re-ghost-town. At this volume, backfill
   is most of the launch content, not a nice-to-have.

## Open questions to settle before scoping further

- Does Pete write live off events, or on a batch cadence?
- Single source of truth for this data shared with E3's bot commands, or a
  separate read path for the news site?
- Voice: does Pete have an established voice yet, or does this news section
  define it?
- Which model tier for article generation — same one Petal's conversational
  side uses, or a separate config entry, given the different latency/quality
  tradeoffs of a batch job versus interactive chat?
