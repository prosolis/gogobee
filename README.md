# GogoBee

Matrix community bot with E2EE, 43 plugins, passive tracking, scheduled posts, and optional LLM features.

Written in Go using [mautrix-go](https://github.com/mautrix/go) for encryption and [modernc.org/sqlite](https://modernc.org/sqlite) for storage.

---

## Table of Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Running the Bot](#running-the-bot)
- [Commands](#commands)
- [Passive Features](#passive-features)
- [Scheduled Posts](#scheduled-posts)
- [Achievements](#achievements)
- [Personality Archetypes](#personality-archetypes)
- [External APIs](#external-apis)
- [Architecture](#architecture)
- [Database](#database)
- [Troubleshooting](#troubleshooting)

---

## Features

- **E2EE that actually works** - mautrix-go with goolm (pure Go). Crypto state lives in SQLite so device keys survive restarts. Cross-signing bootstraps on first run — the bot self-verifies its own device.
- **No CGo, no system deps** - builds to a single static binary. Cross-compile to whatever you want.
- **43 plugins** with dependency injection and ordered registration
- **Games & economy** - Euro virtual currency, Hangman (collaborative, threaded, tiered scoring), Blackjack (1-4 players, auto-play timeout), UNO (solo vs bot or 2–4 player multiplayer via DMs, with optional No Mercy mode), Texas Hold'em (2-9 players, CFR-trained NPC bot, DM-based gameplay with Ollama coaching tips), Wordle (daily cooperative, Wordnik-powered, 5-7 letter words), all with channel restriction
- **Moderation system** (optional) - deterministic detection only, no LLM. Word list with leetspeak variation matching, text/image flood, repeated messages, mention flooding, link rate limiting, invite flooding, join/leave cycling. Three-strike ladder (warn → mute → ban). Admin room notifications, DMs over public callouts.
- **Passive tracking** - XP, stats, streaks, achievements, markov corpus, keyword alerts, all running silently
- **Scheduled posts** via [robfig/cron](https://github.com/robfig/cron) - WOTD, holidays, game releases, birthdays, anime/movie releases, concert digests, esteemed members
- **LLM integration** (optional) - Ollama-powered sentiment analysis, roast profiles, room vibes, tarot readings, conversation summaries
- **Encrypted quote wall** - AES-256-GCM encrypted quotes at rest, reply-to-save, search, leaderboard
- **Space groups** - automatic room grouping via membership overlap. Leaderboards, stats, and trivia scores span all rooms in a group. No Matrix Spaces API needed — the bot infers community boundaries from shared members.
- **SQLite everything** - one file, no external database needed

---

## Requirements

- Go 1.22+
- A Matrix homeserver account for the bot

Optional:
- [Ollama](https://ollama.ai) for LLM features
- API keys for various services (see [Configuration](#configuration))

---

## Installation

### From Source

```bash
git clone https://github.com/prosolis/gogobee.git
cd gogobee
cp .env.example .env   # edit with your settings
go build -tags goolm -o gogobee .
./gogobee
```

### Docker

```bash
docker build -t gogobee .
docker run --env-file .env -v ./data:/app/data gogobee
```

### Docker Compose

```bash
docker compose up -d
```

---

## Configuration

Everything is configured through environment variables or a `.env` file.

### Required

| Variable | Description |
|----------|-------------|
| `HOMESERVER_URL` | Matrix homeserver URL, e.g. `https://matrix.org` |
| `BOT_USER_ID` | Bot's Matrix user ID, e.g. `@gogobee:matrix.org` |
| `BOT_PASSWORD` | Bot's Matrix password |

### Core (optional)

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_DIR` | `./data` | Where the database and device files live |
| `BOT_DISPLAY_NAME` | `GogoBee` | Display name |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `ADMIN_USERS` | | Comma-separated admin user IDs |
| `BROADCAST_ROOMS` | | Comma-separated room IDs for scheduled posts |

### API Keys (optional)

| Variable | Service | Used By |
|----------|---------|---------|
| `RAWG_API_KEY` | [RAWG](https://rawg.io/apidocs) | `!game`, `!retro`, `!releases` |
| `WORDNIK_API_KEY` | [Wordnik](https://developer.wordnik.com) | Word of the Day |
| `CALENDARIFIC_API_KEY` | [Calendarific](https://calendarific.com) | Holiday posts |
| `OPENWEATHER_API_KEY` | [OpenWeather](https://openweathermap.org/api) | `!weather` |
| `FINNHUB_API_KEY` | [Finnhub](https://finnhub.io) | `!stock` |
| `BANDSINTOWN_API_KEY` | [Bandsintown](https://artists.bandsintown.com) | `!concerts` |
| `TMDB_API_KEY` | [TMDB](https://www.themoviedb.org/documentation/api) | `!movie`, `!tv`, `!upcoming` |
| `SERPAPI_KEY` | [SerpAPI](https://serpapi.com) | Esteemed member image fetching |

### Services (optional)

| Variable | Description |
|----------|-------------|
| `OLLAMA_HOST` | Ollama server URL, e.g. `http://localhost:11434` |
| `OLLAMA_MODEL` | Model name, e.g. `llama3.2` |
| `LIBRETRANSLATE_URL` | LibreTranslate instance for `!translate` |
| `LLM_SAMPLE_RATE` | Fraction of messages to classify (0.0–1.0, default `0.15`) |

### Encryption

| Variable | Description |
|----------|-------------|
| `QUOTE_ENCRYPTION_KEY` | Base64-encoded 32-byte AES-256 key for encrypted quote storage. Generate with `openssl rand -base64 32`. If unset, `!quote` is disabled. |

### Feature Flags

| Variable | Description |
|----------|-------------|
| `FEATURE_URL_PREVIEW` | Set to anything to enable automatic URL previews |
| `FEATURE_SHADE` | Set to anything to enable the shade plugin (stub) |
| `FEATURE_TRIVIA` | Set to `false` to disable trivia (default: enabled) |
| `FEATURE_ESTEEMED` | Set to anything to enable satirical esteemed member posts |
| `ESTEEMED_ROOM` | Room ID for esteemed member posts (separate from broadcast rooms) |
| `FEATURE_MODERATION` | Set to `true` to enable the moderation system (disabled by default) |

### Games & Economy

| Variable | Default | Description |
|----------|---------|-------------|
| `GAMES_ROOM` | | Room ID where game commands work (trivia, hangman, blackjack, holdem, wordle, flip) |
| `EURO_COOLDOWN_SECONDS` | `30` | Cooldown between passive euro earning per user |
| `EURO_DEBT_REMINDER` | `true` | Weekly DM reminder if player is in debt |
| `EURO_STARTING_CAP` | `2500` | Max starting balance seeded from corpus |
| `HANGMAN_MAX_WRONG_GUESSES` | `6` | Lives before game over |
| `HANGMAN_SOLUTION_BONUS_MULTIPLIER` | `2` | Bonus multiplier for early solution |
| `HANGMAN_PHRASE_FILE` | | Path to newline-delimited phrase file |
| `BLACKJACK_TIMEOUT_SECONDS` | `60` | Auto-play timeout per turn |
| `BLACKJACK_AUTOPLAY_THRESHOLD` | `15` | Stand at or above, hit below |
| `BLACKJACK_MIN_BET` | `1` | Minimum bet in euros |
| `BLACKJACK_MAX_BET` | `500` | Maximum bet per hand |
| `BLACKJACK_DEBT_LIMIT` | `1000` | Maximum debt before betting disabled |
| `UNO_MIN_BET` | `10` | Minimum wager in euros (solo) |
| `UNO_POT_TAUNT_THRESHOLD` | `500` | Pot size at which GogoBee starts taunting |
| `UNO_MULTI_MIN_BET` | `25` | Minimum ante for multiplayer UNO |
| `UNO_MULTI_MAX_BET` | `500` | Maximum ante for multiplayer UNO |
| `UNO_MULTI_LOBBY_TIMEOUT` | `300` | Lobby expiry in seconds |
| `UNO_MULTI_TURN_TIMEOUT` | `30` | Auto-play timeout in seconds |
| `UNO_MULTI_MAX_AUTOPLAY` | `3` | Consecutive auto-plays before forfeit |
| `HOLDEM_SMALL_BLIND` | `10` | Small blind amount |
| `HOLDEM_BIG_BLIND` | `20` | Big blind amount |
| `HOLDEM_MIN_BUYIN` | `200` | Minimum balance to join |
| `HOLDEM_MAX_BUYIN` | `2000` | Maximum stack at buy-in |
| `HOLDEM_TIMEOUT_SECONDS` | `90` | Action timeout per turn |
| `HOLDEM_NPC_NAME` | `TwinBee` | NPC bot display name |
| `HOLDEM_NPC_HOUSE_BALANCE` | `10000` | NPC starting bankroll |
| `HOLDEM_CFR_POLICY` | `data/policy.gob` | Path to CFR policy file |
| `WORDLE_DEFAULT_LENGTH` | `5` | Default word length (5, 6, or 7) |

### Moderation

All moderation settings are optional. The system is disabled unless `FEATURE_MODERATION=true`.

| Variable | Default | Description |
|----------|---------|-------------|
| `MOD_WORDLIST` | | Path to newline-delimited prohibited word list |
| `MOD_WORDLIST_VARIATIONS` | `true` | Check leetspeak/spaced variants |
| `MOD_STRIKE_EXPIRY_DAYS` | `30` | Days before strikes expire |
| `MOD_MUTE_DURATION_MINUTES` | `60` | Temp mute duration on strike 2 |
| `MOD_MAX_STRIKES` | `3` | Strikes before permanent ban |
| `MOD_FLOOD_MESSAGE_COUNT` | `5` | Messages in window = flood |
| `MOD_FLOOD_MESSAGE_WINDOW_SECONDS` | `10` | Text flood window |
| `MOD_FLOOD_IMAGE_COUNT` | `3` | Images/files in window = flood |
| `MOD_FLOOD_IMAGE_WINDOW_SECONDS` | `30` | Image flood window |
| `MOD_MAX_MESSAGE_LENGTH` | `2000` | Max chars per message (0 = disabled) |
| `MOD_REPEAT_COUNT` | `3` | Repeated messages before strike |
| `MOD_REPEAT_WINDOW_SECONDS` | `60` | Repeat detection window |
| `MOD_REPEAT_SIMILARITY_THRESHOLD` | `0.85` | How similar counts as "same" (0.0–1.0) |
| `MOD_MENTION_MAX` | `5` | Max unique @mentions per message |
| `MOD_MENTION_FLOOD_COUNT` | `3` | Mention-heavy messages in window |
| `MOD_MENTION_FLOOD_WINDOW_SECONDS` | `30` | Mention flood window |
| `MOD_LINK_RATE_NEW_MEMBER` | `3` | Max links per window (new members only) |
| `MOD_LINK_RATE_WINDOW_SECONDS` | `60` | Link rate window |
| `MOD_INVITE_MAX_PER_HOUR` | `5` | Max room invites per user per hour |
| `MOD_JOIN_LEAVE_COUNT` | `3` | Join/leave cycles before flag |
| `MOD_JOIN_LEAVE_WINDOW_MINUTES` | `10` | Join/leave window |
| `MOD_NEW_MEMBER_DAYS` | `14` | Days before a member is no longer "new" |
| `MOD_NEW_MEMBER_FLOOD_MULTIPLIER` | `0.5` | Multiply flood thresholds for new members |
| `MOD_ADMIN_ROOM` | | Dedicated room for mod notifications |
| `MOD_DM_ON_ACTION` | `true` | DM users when action is taken |

### Space Groups

| Variable | Default | Description |
|----------|---------|-------------|
| `SPACE_GROUP_THRESHOLD` | `50` | Percentage of the smaller room's members that must overlap to group rooms together (1–100) |
| `HOLIDAY_COUNTRIES` | `US` | Comma-separated ISO country codes for Calendarific holiday posts |

### Missing Members

| Variable | Default | Description |
|----------|---------|-------------|
| `MISSING_THRESHOLD_DAYS` | `14` | Days of inactivity before considered missing |
| `MISSING_MAX_DAYS` | `90` | Days after which they're considered gone, not missing |
| `MISSING_MIN_MESSAGES` | `10` | Minimum lifetime messages to be eligible |
| `MISSING_EXCLUDE_USERS` | | Comma-separated user IDs to never list as missing |

### Rate Limits

| Variable | Default | Description |
|----------|---------|-------------|
| `RATELIMIT_WEATHER` | `5` | Daily weather lookups per user |
| `RATELIMIT_TRANSLATE` | `20` | Daily translation limit per user |
| `RATELIMIT_CONCERTS` | `10` | Daily concert searches per user |

---

## Running the Bot

```bash
# dev
go run -tags goolm .

# prod
go build -tags goolm -o gogobee .
./gogobee
```

The `-tags goolm` flag selects the pure-Go crypto implementation. No C compiler or libolm needed.

### First Run

1. Start the bot. It logs in, creates a device, bootstraps cross-signing, and self-verifies automatically.
2. That's it. E2EE works across restarts from here on out.

---

## Commands

### XP & Leveling
| Command | Description |
|---------|-------------|
| `!rank` | Your level, XP, and progress |
| `!leaderboard` | Top 10 by XP |

### Reputation
| Command | Description |
|---------|-------------|
| `!rep [@user]` | Reputation count |
| `!repboard` | Top 10 by rep |

Rep is earned when someone thanks you. The bot detects this automatically.

### Stats & Personality
| Command | Description |
|---------|-------------|
| `!stats [@user]` | Message statistics |
| `!rankings [category]` | Rankings by words, links, questions, or emojis |
| `!personality` | Your community archetype |

### Streaks
| Command | Description |
|---------|-------------|
| `!streak` | Current and longest streak |
| `!firstboard` | Top first-posters-of-the-day |

### Trivia
| Command | Description |
|---------|-------------|
| `!trivia [category] [difficulty]` | Start a question |
| `!trivia scores` | Room leaderboard |
| `!trivia categories` | List categories |
| `!trivia fastest` | Fastest answers |
| `!trivia stop` | End current question |

### Economy
| Command | Description |
|---------|-------------|
| `!balance` | Check your euro balance |
| `!baltop` | Euro leaderboard |
| `!baltransfer @user €amount` | Send euros to another player |

### Games (games channel only)
| Command | Description |
|---------|-------------|
| `!flip` | Coin flip |
| `!games` | List available games |
| `!hangman start [easy\|medium\|hard\|extreme]` | Start a Hangman game (optional difficulty) |
| `!hangman [letter]` | Guess a letter |
| `!hangman [phrase]` | Attempt full solution |
| `!hangman submit [phrase]` | Submit a phrase to the pool |
| `!hangman skip` | Skip game (admin only) |
| `!hangboard` | Hangman leaderboard |

### Blackjack (games channel only)
| Command | Description |
|---------|-------------|
| `!blackjack €amount` | Start/join a Blackjack table (1-4 players) |
| `!hit` | Take a card |
| `!stand` | End your turn |
| `!blackjack leave` | Leave before game starts |
| `!bjboard` | Blackjack leaderboard |
| `!twinbeeboard` | GogoBee's victory record against players |

### UNO (games channel only)

UNO can be played solo (vs GogoBee) or multiplayer (2-4 players + bot). All gameplay happens in DMs. The games channel is used for lobby management and public announcements.

| Command | Description |
|---------|-------------|
| `!uno €amount` | Start a solo game vs GogoBee |
| `!uno nomercy €amount` | Start a solo No Mercy game |
| `!uno nomercy 7-0 €amount` | Solo No Mercy with 7-0 rule |
| `!uno start €amount` | Create a multiplayer lobby |
| `!uno start nomercy €amount` | Multiplayer No Mercy lobby |
| `!uno start nomercy 7-0 €amount` | Multiplayer No Mercy with 7-0 rule |
| `!uno join` | Join an open lobby |
| `!uno go` | Start the game (host only, 2+ players required) |
| `!uno leave` | Leave the lobby (refunds ante) |
| `!uno cancel` | Cancel the lobby (host/admin, refunds all) |

**DM commands during gameplay:** reply with a card number to play, `draw` to draw, `uno` to call UNO (required when you have 2 cards), `quit` to forfeit. During draw stacking, type `accept` to absorb the stack.

#### Classic Mode

Standard 108-card UNO deck. Draw one card per turn (pass if unplayable). Wild Draw Four can be challenged — if the challenger catches a bluff, the player draws 4 instead.

GogoBee has a personality system during solo play: she reads a book while playing, and puts it down when the game gets serious (opponent has few cards). Commentary changes based on book state.

#### No Mercy Mode

Based on UNO Show 'Em No Mercy (2023, Mattel). Bigger 168-card deck, meaner rules.

**New cards:**
| Card | Type | Effect |
|------|------|--------|
| Skip Everyone | Colored | Skips all other players — you go again |
| Discard All | Colored | Play it and discard every other card of that color from your hand |
| Draw Four | Colored | Like Wild Draw Four but matches by color (not wild) |
| Wild Reverse Draw Four | Wild | Reverse direction + draw 4 + pick a color |
| Wild Draw Six | Wild | Draw 6 + pick a color |
| Wild Draw Ten | Wild | Draw 10 + pick a color |
| Wild Color Roulette | Wild | Pick a color — next player flips cards from the deck until that color appears, keeping all flipped cards |

**Rule changes:**
- **Draw stacking** — when hit with a draw card, play a draw card of equal or higher value to pass the penalty to the next player. The stack keeps growing until someone can't (or won't) stack back. That player draws the entire total.
- **Draw until playable** — no more drawing one and passing. You keep drawing until you find a playable card.
- **Mercy rule** — reach 25 cards in your hand and you're eliminated.

#### 7-0 Rule (optional, No Mercy only)

Enabled by adding `7-0` to the command. When active:
- **Play a 7** — swap hands with another player of your choice (in multiplayer, you pick the target)
- **Play a 0** — all players pass their hand to the next player in play direction

### Texas Hold'em (games channel only)

No-limit Texas Hold'em poker for 2-9 players. Buy-in is debited from your euro balance when you sit down; your remaining stack is cashed out when you leave. Private cards and coaching tips are delivered via DM — the room only sees start/end announcements.

An optional AI opponent (NPC) uses a CFR-trained poker solver. Add one with `!holdem addbot`.

| Command | Description |
|---------|-------------|
| `!holdem join` | Sit down at the table |
| `!holdem leave` | Leave the table (cashes out stack) |
| `!holdem start` | Start dealing (2+ players) |
| `!holdem addbot` | Add a CFR-trained AI opponent |
| `!holdem fold` | Fold your hand |
| `!holdem check` | Check (no bet to call) |
| `!holdem call` | Call the current bet |
| `!holdem raise <amount>` | Raise to a total of amount |
| `!holdem allin` | Go all-in |
| `!holdem status` | Current table state (sent via DM) |
| `!holdem help` | Show in-game help |

**DM commands:** `!holdem tips on/off` — toggle coaching tips (equity + pot odds analysis, powered by Ollama with rules-based fallback).

#### NPC Bot

The NPC uses Counterfactual Regret Minimization (External Sampling MCCFR), trained via self-play. The policy table ships as `data/policy.gob` and is loaded at startup. The bot plays a mixed strategy — it randomizes actions according to its trained probability distribution, so it won't always make the same play in the same spot.

#### Training the NPC

A standalone training CLI is provided under `cmd/holdem-train/`. It requires the `training` build tag:

```bash
# Build the training CLI
go build -tags training -o holdem-train ./cmd/holdem-train/

# Train with 8 workers (defaults to all CPU cores if --workers is omitted)
./holdem-train --iterations 5000000 --workers 8 --output data/policy.gob

# Resume from a checkpoint
./holdem-train --iterations 5000000 --workers 8 --resume data/policy.gob.checkpoint --output data/policy.gob

# Validate the trained policy (10K hands vs random baseline)
./holdem-train --validate --output data/policy.gob
```

| Flag | Default | Description |
|------|---------|-------------|
| `--iterations` | `5000000` | Number of training iterations |
| `--workers` | `runtime.NumCPU()` | Parallel workers |
| `--output` | `data/policy.gob` | Output policy file |
| `--resume` | | Resume from checkpoint |
| `--validate` | `false` | Run validation instead of training |
| `--checkpoint-every` | `500000` | Checkpoint interval |
| `--seed` | `42` | Random seed |

Progress is logged with overall completion percentage and ETA. Checkpoints are saved every 30 seconds during parallel training.

### Wordle (games channel only)

Daily cooperative Wordle — one puzzle per day, the community works together with a shared 6-guess limit. Word selection and validation powered by the Wordnik API (`WORDNIK_API_KEY`). A new puzzle auto-posts at midnight UTC. Word length is configurable (5-7 letters). Falls back to a bundled word list if the API is unavailable.

| Command | Description |
|---------|-------------|
| `!wordle <word>` | Submit a guess for today's puzzle |
| `!wordle grid` | Re-post the current puzzle grid |
| `!wordle stats` | All-time leaderboard with community streak |
| `!wordle new` | Start a new puzzle (admin) |
| `!wordle new <5\|6\|7>` | New puzzle with specific word length (admin) |
| `!wordle skip` | Reveal answer and end puzzle (admin) |
| `!wordle help` | Show commands |

No economy integration — stats and leaderboard position are the reward.

### Reminders
| Command | Description |
|---------|-------------|
| `!remindme <time> <message>` | Set a reminder (natural language) |
| `!reminders` | Your pending reminders |
| `!unremind <id>` | Cancel a reminder |

### Presence
| Command | Description |
|---------|-------------|
| `!away [message]` | Set away status |
| `!afk [message]` | Same as away |
| `!back` | Clear away/afk |
| `!whois @user` | Profile card |

### Fun
| Command | Description |
|---------|-------------|
| `!roll [NdM+X]` | Dice (default 1d6) |
| `!8ball <question>` | Magic 8-ball |
| `!coin` | Coin flip |
| `!time [city\|@user]` | World clock (cities or user's timezone) |
| `!hltb <game>` | How Long To Beat |
| `!twinbee` | Twinbee series lore |
| `!poll <q> \| <a> \| <b>...` | Reaction poll |
| `!weather <location>` | Weather (needs API key) |
| `!dadjoke` | Dad joke |
| `!randomwiki` | Random Wikipedia article |

### Tools
| Command | Description |
|---------|-------------|
| `!calc <expression>` | Calculator, understands "5 plus 3" |
| `!qr <text>` | Generate QR code |

### Games & Entertainment
| Command | Description |
|---------|-------------|
| `!game <query>` / `!retro <query>` | Game lookup (RAWG) |
| `!releases [month\|search <q>]` | Game releases |
| `!releasewatch add\|list\|remove` | Release watchlist |
| `!stock <ticker>` | Stock quote (Finnhub) |
| `!stockwatch add\|list\|remove` | Stock watchlist |
| `!concerts <artist>` | Concert search |
| `!concerts watch\|watching\|unwatch` | Concert watchlist |
| `!anime <title>` | Anime search (MAL) |
| `!anime watch\|watching\|unwatch\|season\|upcoming` | Anime features |
| `!movie <title>` / `!tv <title>` | Movie/TV search (TMDB) |
| `!movie watch\|watching\|unwatch` | Movie watchlist |
| `!upcoming movies` | Upcoming movies |

### Lookup
| Command | Description |
|---------|-------------|
| `!wiki <topic>` | Wikipedia summary |
| `!define <word>` | Dictionary definition |
| `!urban <term>` | Urban Dictionary |
| `!translate [lang] <text>` | Translate (needs LibreTranslate) |

### Personal
| Command | Description |
|---------|-------------|
| `!settz <timezone>` | Set timezone (IANA) |
| `!mytz` | Your timezone and current time |
| `!timezone list` | Common timezones |
| `!np [track]` | Now playing |
| `!backlog add\|list\|random\|done` | Personal backlog |
| `!watch <keyword>` | DM alerts for a keyword |
| `!watching` | List keyword watches |
| `!unwatch <keyword>` | Remove watch |

### Countdowns
| Command | Description |
|---------|-------------|
| `!countdown add "<label>" <YYYY-MM-DD>` | Public countdown |
| `!countdown private "<label>" <YYYY-MM-DD>` | Private countdown |
| `!countdown mine` | Your countdowns |
| `!countdown remove <id>` | Remove one |
| `!countdown [id]` | List all or show one |

### Birthdays
| Command | Description |
|---------|-------------|
| `!birthday set <MM-DD[-YYYY]>` | Set birthday |
| `!birthday remove` | Remove birthday |
| `!birthday show` | Show yours |
| `!birthdays` | Upcoming (next 30 days) |
| `!horoscope` | Daily horoscope (requires birthday to be set) |

### Community
| Command | Description |
|---------|-------------|
| `!quote` | Random quote (or reply to a message to save it) |
| `!quote @user` | Random quote from a specific user |
| `!quote search <text>` | Search quotes |
| `!quote "text" -- @user` | Manually save a quote attributed to a user |
| `!quote delete <id>` | Delete a quote (admin only) |
| `!quoteboard` | Top 5 most-quoted members |
| `!missing` | List members who haven't posted recently |
| `!missing post [@user]` | Generate a milk carton poster for the longest-absent (or specified) member |
| `!haveyouseenthem @user` | Generate a milk carton missing poster for a user |

### LLM (requires Ollama)
| Command | Description |
|---------|-------------|
| `!howami [@user]` | Roast profile |
| `!vibe` | Room energy check |
| `!tldr` | Summarize recent chat |
| `!sentiment [@user]` | Sentiment breakdown (10 categories) |
| `!potty [@user]` | Profanity count |
| `!pottyboard` | Profanity leaderboard |
| `!insults [@user]` | Insult stats |
| `!insultboard` | Insult leaderboard |
| `!tarot [@user]` | Draw a tarot card + LLM reading |
| `!tarotspread [@user]` | Three-card spread (Past/Present/Future) |

### Moderation (admin only, requires `FEATURE_MODERATION=true`)
| Command | Description |
|---------|-------------|
| `!mod warn @user [reason]` | Issue a manual warning (counts as a strike) |
| `!mod mute @user [duration]` | Manual mute (does not consume a strike) |
| `!mod unmute @user` | Remove mute |
| `!mod ban @user [reason]` | Permanent ban |
| `!mod strikes @user` | Show active strikes |
| `!mod forgive @user` | Clear all active strikes |
| `!mod history @user` | Full moderation history |
| `!mod reload` | Reload word list from disk |
| `!mod status` | Show current moderation config |
| `!mod test @user` | Simulate next violation without taking action |

### Other
| Command | Description |
|---------|-------------|
| `!achievements [@user]` | Unlocked achievements |
| `!wotd` | Today's Word of the Day (use it in chat for 25 XP) |
| `!botinfo` | Bot diagnostics (admin only) |
| `!help` | DMs the full command list |

---

## Passive Features

All of these run in the background without any commands:

- **XP** - 10 XP per message with 30s cooldown. Level-up announcements use Twinbee/Parodius themed messages.
- **Stats** - tracks words, chars, links, images, questions, emojis, and time-of-day patterns
- **Streaks** - consecutive days active, first poster of the day
- **Reputation** - detects "thanks", "ty", "thx", etc. with 24h cooldown per pair
- **Achievements** - 32 of them, checked silently on every message
- **Markov chains** - collects messages for `!markov` generation (10k cap per user)
- **Keyword alerts** - DMs you when someone says your watched keywords
- **Presence** - auto-clears away/afk when you send a message
- **Room milestones** - announces at 1k, 5k, 10k, 25k, 50k, 100k, 250k, 500k, 1M messages
- **Euro earning** - earn €0.50–€10 per message based on word count (30s cooldown). Starting balance seeded from corpus history (capped at €2,500)
- **URL previews** - OG tag extraction (feature-flagged, off by default)
- **Reactions** - logs all reactions for `!emojiboard`
- **Space groups** - rooms with sufficient member overlap are automatically grouped. Leaderboards, trivia scores, and other per-room features aggregate across the group. Recomputed hourly; persisted to SQLite. Uses strict clique-based grouping (every room must meet the threshold with every other room in the group).
- **LLM classification** - sentiment (10 categories), profanity, insults, WOTD usage (needs Ollama)
- **Message buffer** - last 50 messages per room held in memory for `!vibe` and `!tldr`. Not persisted to disk; resets on restart. Uptime reported when insufficient messages are buffered.

---

## Scheduled Posts

Uses [robfig/cron](https://github.com/robfig/cron). All times UTC.

| Time | Job | What it does |
|------|-----|--------------|
| 00:05 | Prefetch | Grabs WOTD data ahead of time |
| 06:00 | Birthdays | Birthday shoutouts + 100 XP |
| 07:00 | Holidays | Multi-calendar holidays (US, Asian, Jewish, Islamic) |
| 08:00 | WOTD | Posts the Word of the Day |
| 09:00 Mon | Releases | Weekly game releases |
| 10:00 | Anime | Anime airing today |
| 11:00 | Movies | Movie releases today |
| 12:00 Sun | Concerts | Weekly concert digest |
| 13:00 Wed/Sun | Esteemed | Satirical esteemed community member posts (feature-flagged) |
| Every 30s | Reminders | Fires pending reminders |
| Hourly | Space groups | Refreshes room membership overlap and group mappings |
| 03:00 | Maintenance | Purges stale caches, old rate limits, expired logs; runs SQLite optimize |

---

## Achievements

32 achievements:

**Message Milestones** - first_message, 100_messages, 1000_messages, 10000_messages

**Time-Based** - night_owl (100 night msgs), early_bird (100 morning msgs)

**Content** - wordsmith (avg >8 words), link_collector (50 links), shutterbug (20 images), question_master (100 questions)

**Social** - welcome_wagon (first message), rep_magnet (10 rep received)

**Streaks** - week_streak (7 days), month_streak (30 days)

**Trivia** - trivia_novice (10 correct), trivia_master (100 correct)

**Special** - markov_victim (got markov'd), logophile (used 10 WOTDs)

---

## Personality Archetypes

Assigned based on your message patterns:

| Archetype | What it means |
|-----------|---------------|
| Chatterbox | You talk a lot |
| Novelist | Long messages |
| Inquisitor | Always asking questions |
| Linkmaster | Shares lots of links |
| Shutterbug | Lots of images |
| Enthusiast | Exclamation marks! |
| Regular | Solid community member |

---

## Sentiment Classification

Every message (subject to `LLM_SAMPLE_RATE`) is classified by Ollama into one of 10 sentiment categories. The bot reacts with an emoji when the confidence score is strong enough (|score| > 0.5). Per-user counts are tracked in the database and viewable via `!sentiment`.

| Sentiment | Emoji | Score range | Example |
|-----------|-------|-------------|---------|
| Positive | 👍 | > 0.5 | "This is awesome, great work!" |
| Excited | 🔥 | > 0.5 | "OH MY GOD I can't wait for this!!" |
| Supportive | 🤗 | > 0.5 | "You've got this, don't give up" |
| Grateful | 💜 | > 0.5 | "Thank you so much for helping me" |
| Humorous | 😂 | > 0.5 | "lmao that's the funniest thing I've seen all day" |
| Curious | 🧐 | > 0.5 | "How does that work exactly?" |
| Neutral | — | — | "I'll be back in 10 minutes" |
| Sarcastic | 🤨 | < -0.5 | "Oh sure, that'll definitely work" |
| Frustrated | 😮‍💨 | < -0.5 | "I've been trying to fix this for three hours" |
| Negative | 👎 | < -0.5 | "This is broken and nobody cares" |

The LLM also returns a float score (-1.0 to 1.0) for each message. These scores are averaged per user to derive an overall mood shown in `!sentiment` output and fed into `!howami` roast profiles.

---

## External APIs

All optional. The bot works fine without any of them, you just won't have those specific features.

| Service | Free? | What for |
|---------|-------|----------|
| [RAWG](https://rawg.io/apidocs) | Yes | Game lookups, releases |
| [Wordnik](https://developer.wordnik.com) | Yes | Word of the Day |
| [Calendarific](https://calendarific.com) | Yes (1k/mo) | Holiday calendar |
| [HebCal](https://www.hebcal.com) | Yes, no key | Jewish holidays |
| [Aladhan](https://aladhan.com/prayer-times-api) | Yes, no key | Islamic dates |
| [OpenWeather](https://openweathermap.org/api) | Yes (1k/day) | Weather |
| [Finnhub](https://finnhub.io) | Yes | Stock quotes |
| [Bandsintown](https://artists.bandsintown.com) | Yes | Concert data |
| [Jikan/MAL](https://jikan.moe) | Yes, no key | Anime data |
| [TMDB](https://www.themoviedb.org) | Yes | Movie/TV data |
| [OpenTDB](https://opentdb.com) | Yes, no key | Trivia questions |
| [Wikipedia](https://en.wikipedia.org/api/rest_v1/) | Yes, no key | Wiki summaries |
| [Free Dictionary](https://dictionaryapi.dev) | Yes, no key | Definitions |
| [Urban Dictionary](https://rapidapi.com/community/api/urban-dictionary) | Yes, no key | Slang |
| [icanhazdadjoke](https://icanhazdadjoke.com) | Yes, no key | Dad jokes |
| [SerpAPI](https://serpapi.com) | Free (100/mo) | Image search for esteemed members |
| [HowLongToBeat](https://howlongtobeat.com) | Yes, no key | Game completion times |
| [LibreTranslate](https://libretranslate.com) | Self-host | Translation |
| [Ollama](https://ollama.ai) | Self-host | LLM features (sentiment, tarot, vibes, roasts) |

---

## Architecture

```
gogobee/
├── main.go                  # Entry point, plugin registration, cron setup
├── go.mod / go.sum
├── data/
│   └── policy.gob           # Pre-trained CFR policy for Hold'em NPC
├── cmd/
│   ├── holdem-train/
│   │   └── main.go          # CFR training CLI (build tag: training)
│   └── holdem-seed/
│       └── main.go          # Seed policy generator
├── internal/
│   ├── bot/
│   │   ├── client.go        # mautrix client + E2EE (cryptohelper + goolm)
│   │   └── dispatch.go      # Plugin registry, event dispatch
│   ├── crypto/
│   │   └── crypto.go        # AES-256-GCM encryption, HMAC-SHA256
│   ├── db/
│   │   └── db.go            # SQLite schema (40+ tables), migrations
│   ├── plugin/
│   │   ├── plugin.go        # Plugin interface, Base helpers, context types
│   │   ├── xp.go            # XP & leveling
│   │   ├── reputation.go    # Thanks detection
│   │   ├── stats.go         # Message stats & milestones
│   │   ├── streaks.go       # Daily streaks
│   │   ├── trivia.go        # OpenTDB trivia
│   │   ├── reminders.go     # Reminders
│   │   ├── presence.go      # Away/AFK
│   │   ├── fun.go           # Dice, 8ball, polls, weather, etc.
│   │   ├── tools.go         # Calculator, QR codes
│   │   ├── user.go          # Timezone, quotes, backlog, keyword watches
│   │   ├── welcome.go       # New user detection, !help
│   │   ├── achievements.go  # 32 achievements
│   │   ├── reactions.go     # Reaction logging, emojiboard
│   │   ├── markov.go        # Markov chains
│   │   ├── urls.go          # URL previews
│   │   ├── llm_passive.go   # Ollama sentiment/profanity
│   │   ├── wotd.go          # Word of the Day
│   │   ├── holidays.go      # Multi-calendar holidays
│   │   ├── gaming.go        # Game releases
│   │   ├── birthday.go      # Birthdays
│   │   ├── retro.go         # Game lookups (RAWG)
│   │   ├── lookup.go        # Wiki, dictionary, urban, translate
│   │   ├── countdown.go     # Countdowns
│   │   ├── stocks.go        # Stocks
│   │   ├── concerts.go      # Concerts
│   │   ├── anime.go         # Anime
│   │   ├── movies.go        # Movies/TV
│   │   ├── botinfo.go       # Admin diagnostics
│   │   ├── howami.go        # LLM roasts
│   │   ├── vibe.go          # Room vibe, TLDR
│   │   ├── shade.go         # Stub
│   │   ├── milkcarton.go    # Missing member milk carton posters
│   │   ├── quotewall.go     # Encrypted quote wall (AES-256-GCM)
│   │   ├── tarot.go         # LLM-powered tarot readings
│   │   ├── horoscope.go     # Daily horoscopes
│   │   ├── euro.go          # Euro virtual currency
│   │   ├── flip.go          # Coin flip, !games
│   │   ├── hangman.go       # Collaborative Hangman
│   │   ├── blackjack.go     # Multiplayer Blackjack
│   │   ├── uno.go           # Solo UNO vs bot (DM-based)
│   │   ├── uno_multi.go     # Multiplayer UNO (lobby + DM turns)
│   │   ├── uno_nomercy.go   # No Mercy mode (deck, stacking, mercy rule, 7-0, bot AI)
│   │   ├── holdem.go        # Texas Hold'em plugin (commands, game lifecycle, NPC)
│   │   ├── holdem_game.go   # Game state, player types, deck, position labels
│   │   ├── holdem_betting.go # Blinds, action validation, side pots
│   │   ├── holdem_eval.go   # Hand evaluation, showdown, settlement
│   │   ├── holdem_render.go # Card glyphs, table view, announcements
│   │   ├── holdem_tips.go   # Ollama coaching tips with equity analysis
│   │   ├── holdem_equity.go # Monte Carlo equity engine
│   │   ├── holdem_cfr.go    # CFR policy table, NPC action selection, training
│   │   ├── wordle.go        # Daily Wordle plugin (commands, lifecycle, scheduler)
│   │   ├── wordle_game.go   # Puzzle state, scoring algorithm, letter tracking
│   │   ├── wordle_render.go # Emoji grid, keyboard, announcements, leaderboard
│   │   ├── wordle_wordnik.go # Wordnik API (random word, validation, definitions)
│   │   ├── wordle_fallback.go # Emergency word list loader
│   │   ├── esteemed.go      # Satirical esteemed member posts
│   │   ├── moderation.go   # Moderation system (strikes, word list, flood detection)
│   │   └── ratelimits.go    # Rate limiting
│   └── util/
│       ├── auth.go          # Matrix login, token check
│       ├── logger.go        # slog logging
│       └── parser.go        # Message parsing, XP math, archetypes
```

### Why Go?

**E2EE** - This project went through three SDK iterations: `matrix-bot-sdk` (no E2EE support), `matrix-js-sdk` (E2EE via `fake-indexeddb` with an in-memory crypto store that wiped device keys on every restart), and finally `mautrix-go` which stores crypto state in SQLite with cross-signing bootstrap. The bot self-verifies its own device on startup.

**Deployment** - Pure Go, no CGo. `go build -tags goolm` gives you a static binary with zero system dependencies. The TypeScript version needed Node.js, npm, a C compiler for better-sqlite3, and libolm.

**Scheduler** - Replaced a hand-rolled 60s tick loop with robfig/cron. Standard cron expressions, less code, fewer bugs.

**Plugins** - Go interfaces + struct embedding instead of abstract classes. Same pattern, less boilerplate.

---

## Database

Single SQLite file at `$DATA_DIR/gogobee.db`. Schema auto-creates on first run. WAL mode enabled.

40+ tables covering users, XP, stats, streaks, reputation, reminders, trivia, achievements, encrypted quotes (AES-256-GCM), backlog, keyword watches, scheduler config, birthdays, horoscopes, LLM classifications, stocks, concerts, anime, movies, countdowns, presence, markov corpus, reaction log, and various caches.

### Backup

```bash
# safe to run while the bot is up (WAL mode)
sqlite3 data/gogobee.db ".backup data/gogobee-backup.db"
```

---

## Troubleshooting

### E2EE

E2EE should just work. The bot bootstraps cross-signing and self-verifies its device on first run.

1. After restarts, the bot reuses its saved device and crypto state. No manual steps needed.
2. If things are really broken, delete `data/device.json` and `data/gogobee.db` to start fresh.

### Bot not responding in encrypted rooms

- Check that the bot joined the room (it auto-joins on invite)
- Look at logs for decryption errors

### API commands not working

- Check that the relevant API key is set in `.env`
- Look at logs for error messages
- Most APIs have rate limits; the bot caches responses to stay within them

### Build errors about libolm

```bash
# use the goolm tag:
go build -tags goolm -o gogobee .
```

---

## License

MIT
