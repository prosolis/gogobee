# Freebee

A full-featured Matrix community bot with end-to-end encryption support, a plugin architecture, silent passive tracking, daily scheduled posts, and a suite of community engagement features.

Runs on Node.js with TypeScript, SQLite, and `matrix-js-sdk` with automatic Rust E2EE.

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
- [Supported Timezones](#supported-timezones)
- [External APIs](#external-apis)
- [Architecture](#architecture)
- [Database](#database)
- [Troubleshooting](#troubleshooting)
- [Backup & Recovery](#backup--recovery)

---

## Features

- **End-to-end encryption** via `matrix-js-sdk` with Rust crypto — automatic key management, session rotation, and key gossiping out of the box
- **Automatic token renewal** — set `MATRIX_BOT_PASSWORD` and the bot refreshes expired access tokens on its own
- **Crypto reset flow** — set `CRYPTO_RESET=true` to wipe and re-establish E2EE keys in one restart, with automatic server-side device cleanup
- **Plugin architecture** — modular design with 35+ independent plugins
- **Silent passive tracking** — streaks, stats, achievements, keyword alerts, Markov corpus, and presence tracked without announcements
- **Scheduled daily posts** — Word of the Day, holidays, game releases, birthdays, anime airings, movie releases, and weekly concert digests
- **Multi-calendar holiday support** — Gregorian, Hebrew, Hijri, and Asian calendars (JP, CN, KR, IN, TH, VN, TW, PH) with non-Western holiday featuring
- **XP & leveling system** with cooldown-based anti-farming and Twinbee/Parodius-themed level-up announcements
- **Reputation system** with natural language thanks detection (LLM-powered sarcasm-aware mode when Ollama is active)
- **32 unlockable achievements** evaluated silently on every message
- **Trivia system** with OpenTDB integration, time-weighted scoring, and threaded conversations
- **Game backlog manager**, now-playing status, and HowLongToBeat integration
- **Stock quotes** via Finnhub with watchlists
- **Anime tracking** via MyAnimeList/Jikan with watchlists and airing alerts
- **Movie & TV tracking** via TMDB with watchlists and release alerts
- **Concert tracking** via Bandsintown with artist watchlists and weekly digests
- **LLM-powered passive classifier** (optional, via Ollama) — multilingual profanity, insult, sentiment, gratitude, and WOTD correctness detection with emoji reactions and robust JSON repair
- **`!howami` roast command** — LLM generates a personalized roast based on your actual stats, achievements, and behavior patterns
- **`!vibe` room energy check** — LLM reads recent messages and describes the current chat mood
- **`!tldr` conversation summary** — catch up on what you missed with an LLM-generated summary of recent messages
- **URL preview enrichment** — automatic og:tag extraction for shared links
- **Markov chain text generation** — per-user trigram models for chaos
- **Lookup tools** — Wikipedia, dictionary, and translation (LibreTranslate)
- **Presence system** — away/afk/back status with auto-clear and `!whois` profile cards
- **Community countdowns** — public and private event countdowns
- **Welcome system** — greets new users with bonus XP and provides `!help` via DM
- **Reminder system** with natural language time parsing
- **Weather system** — current conditions via OpenWeatherMap with intelligent geocoding: city names, zip/postal codes (with optional country suffix), `@user` timezone-based lookup, US-preferred disambiguation for ambiguous cities, dual °C/°F display, feels-like temperature, humidity, and wind speed
- **Reaction polls**, dice rolling, world clock, and more
- **Inline calculator** — math expressions, percentages, unit conversions via mathjs
- **QR code generator** — generates and posts QR code images to the room
- **Reaction leaderboard** — silently tracks emoji reactions, revealed with `!emojiboard`
- **Per-command rate limiting** for paid API endpoints

---

## Requirements

- **Node.js 22+**
- **npm** (comes with Node.js)
- A Matrix account for the bot with an access token or password

---

## Installation

### From Source

```bash
git clone https://github.com/your-org/freebee.git
cd freebee
npm install
npm run build
```

### With Docker

```bash
git clone https://github.com/your-org/freebee.git
cd freebee
cp .env.example .env
# Edit .env with your configuration
docker compose up -d
```

---

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```bash
cp .env.example .env
```

### Required Variables

| Variable | Description |
|---|---|
| `MATRIX_HOMESERVER_URL` | Your Matrix homeserver (e.g., `https://matrix.example.com`) |
| `MATRIX_BOT_USER_ID` | Bot's full user ID (e.g., `@freebee:example.com`) |
| `MATRIX_ACCESS_TOKEN` | Bot's access token (or set `MATRIX_BOT_PASSWORD` instead) |

### Authentication

You can authenticate the bot in two ways:

1. **Access token only** — set `MATRIX_ACCESS_TOKEN`. If it expires, the bot exits.
2. **Password (recommended)** — set `MATRIX_BOT_PASSWORD`. The bot validates the token on startup and automatically obtains a new one via password login if it's expired or missing. The new token is persisted to `.env`.

Both can be set simultaneously for maximum resilience.

### Recommended Variables

| Variable | Default | Description |
|---|---|---|
| `MATRIX_BOT_PASSWORD` | *(empty)* | Bot account password for automatic token renewal |
| `BOT_ROOMS` | *(empty)* | Comma-separated room IDs for scheduled posts |
| `BOT_ADMIN_USERS` | *(empty)* | Comma-separated admin user IDs |
| `BOT_PREFIX` | `!` | Command prefix |
| `DATA_DIR` | `./data` | Directory for database, crypto store, and logs |
| `LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `BOT_DEFAULT_CITY` | *(empty)* | Default city for concert digests and location features |

### API Keys

| Variable | Required For | How to Get |
|---|---|---|
| `RAWG_API_KEY` | `!releases`, daily game releases post | [rawg.io/apidocs](https://rawg.io/apidocs) (free) |
| `WORDNIK_API_KEY` | Word of the Day post | [developer.wordnik.com](https://developer.wordnik.com/) (free) |
| `CALENDARIFIC_API_KEY` | Holiday data (Western + Asian holidays) | [calendarific.com](https://calendarific.com/api-documentation) (free tier) |
| `OPENWEATHER_API_KEY` | `!weather` command | [openweathermap.org](https://openweathermap.org/api) (free tier) |
| `FINNHUB_API_KEY` | `!stock` command | [finnhub.io](https://finnhub.io/) (free, 60 calls/min) |
| `BANDSINTOWN_API_KEY` | `!concerts` command | [bandsintown.com](https://artists.bandsintown.com/) (free) |
| `TMDB_API_KEY` | `!movie`, `!tv` commands | [themoviedb.org](https://www.themoviedb.org/documentation/api) (free) |

HebCal (Jewish calendar), Aladhan (Islamic calendar), Jikan/MAL (anime), OpenTDB (trivia), Wikipedia, and Free Dictionary are free and require no API keys.

### Optional: Self-hosted Services

| Variable | Default | Description |
|---|---|---|
| `LIBRETRANSLATE_URL` | *(empty)* | URL for self-hosted LibreTranslate instance (e.g., `http://localhost:5000`) |
| `OLLAMA_HOST` | *(empty)* | Ollama endpoint for LLM classifier (e.g., `http://localhost:11434`) |
| `OLLAMA_MODEL` | *(empty)* | Ollama model name (e.g., `qwen2.5:1.5b`) |
| `LLM_SAMPLE_RATE` | `0.15` | Fraction of non-keyword messages randomly sampled for LLM classification (0.0–1.0) |
| `ASIAN_HOLIDAY_COUNTRIES` | `JP,CN,KR,IN,TH,VN,TW,PH` | Comma-separated country codes for Asian holiday data |

LLM features (profanity/insult/sentiment/gratitude detection, smart WOTD validation) are **only active when both `OLLAMA_HOST` and `OLLAMA_MODEL` are set**. Leave them blank to disable. The classifier uses a 16k context window, a 30-second request timeout, and exponential backoff (5s–5min) when Ollama is unreachable. When active, the LLM replaces keyword-based thanks detection with sarcasm-aware gratitude detection for the reputation system.

### Rate Limits

| Variable | Default | Description |
|---|---|---|
| `RATELIMIT_WEATHER` | `5` | Max `!weather` calls per user per day |
| `RATELIMIT_TRANSLATE` | `20` | Max `!translate` calls per user per day |
| `RATELIMIT_CONCERTS` | `10` | Max `!concerts` calls per user per day |

Admins (`BOT_ADMIN_USERS`) bypass all rate limits. Set to `0` for unlimited.

### Feature Flags

| Variable | Default | Description |
|---|---|---|
| `FEATURE_URL_PREVIEW` | `true` | Enable automatic URL og:tag previews |
| `FEATURE_SHADE` | `false` | Enable shade plugin (stub) |
| `CRYPTO_RESET` | `false` | Set to `true` to wipe and re-establish E2EE keys on next startup (auto-deletes old device from server) |

### Scheduler Times

All times are in **24-hour UTC format**.

| Variable | Default | Description |
|---|---|---|
| `SCHEDULE_HOLIDAYS_HOUR` | `7` | Hour for holiday post |
| `SCHEDULE_HOLIDAYS_MINUTE` | `0` | Minute for holiday post |
| `SCHEDULE_RELEASES_HOUR` | `19` | Hour for game releases post |
| `SCHEDULE_RELEASES_MINUTE` | `0` | Minute for game releases post |

Additional scheduled jobs with DB-configured defaults:

| Job | Default Time | Description |
|---|---|---|
| `wotd` | 08:00 | Word of the Day |
| `birthday_check` | 07:05 | Birthday announcements |
| `anime_releases` | 19:30 | Anime airing alerts |
| `movie_releases` | 20:00 | Movie/TV release alerts |
| `concert_digest` | 10:00 | Weekly concert digest (Sundays only) |

All schedule times can be adjusted at runtime with `!schedule <job> <HH:MM>`.

### Getting a Matrix Access Token

1. Log into your bot's Matrix account via Element or another client
2. Go to **Settings > Help & About > Advanced** and copy the access token
3. Alternatively, use the Matrix client-server API:
   ```bash
   curl -X POST "https://your-homeserver/_matrix/client/v3/login" \
     -H "Content-Type: application/json" \
     -d '{"type":"m.login.password","user":"freebee","password":"your-password"}'
   ```
4. Copy the `access_token` from the response

Or just set `MATRIX_BOT_PASSWORD` and let the bot handle token management automatically.

---

## Running the Bot

### Development

```bash
npm run dev
```

### Production

```bash
npm run build
npm start
```

### Docker

```bash
docker compose up -d

# View logs
docker compose logs -f freebee

# Restart
docker compose restart
```

The bot will auto-join rooms when invited.

---

## Commands

All commands use the configured prefix (default: `!`). Use `!help` to get a full command list sent to your DMs.

### XP & Leveling

| Command | Description |
|---|---|
| `!rank [@user]` | Show XP, level, and progress bar |
| `!leaderboard [n]` | Top N users by XP (default 10, max 25) |

XP is earned passively: **10 XP per message** with a 30-second cooldown to prevent farming. The level curve is linear: level N requires N x 100 total XP. When a user levels up, the bot announces it with a random Twinbee/Parodius-themed power-up message (bell combos, laser modes, shield activations, etc.).

### Reputation

| Command | Description |
|---|---|
| `!rep [@user]` | Show reputation score |
| `!repboard` | Top 10 by reputation |

Reputation is granted automatically when the bot detects natural "thanks" messages directed at another user (e.g., "thanks @user:server.com", "ty @user:server.com"). There's a 24-hour cooldown per giver/receiver pair. The bot reacts with a checkmark to acknowledge.

When the LLM classifier is active, keyword-based detection is replaced with sarcasm-aware gratitude detection — sarcastic thanks like "thanks for nothing" won't grant rep.

Receiving reputation also grants **5 bonus XP**.

### Statistics

| Command | Description |
|---|---|
| `!stats [@user]` | Full message metrics (words, links, images, questions, etc.) |
| `!rankings [category]` | Leaderboard by category |
| `!personality [@user]` | Your community archetype |

Valid ranking categories: `messages`, `words`, `links`, `images`, `questions`, `emojis`, `streak`

### Streaks

| Command | Description |
|---|---|
| `!streak [@user]` | Current and record streak (in days) |
| `!firstboard` | Early bird leaderboard (who posts first each day) |

### Reminders

| Command | Description |
|---|---|
| `!remindme <time> <message>` | Set a reminder with natural language time |
| `!reminders` | List your pending reminders |
| `!unremind <id>` | Cancel a reminder by ID |

Time parsing examples:
- `!remindme in 30 minutes check the oven`
- `!remindme tomorrow at 3pm meeting with team`
- `!remindme next friday submit report`

Reminders are checked every 30 seconds.

### User Settings & Tools

| Command | Description |
|---|---|
| `!settz <city\|IANA>` | Set your timezone (e.g., `!settz Tokyo`, `!settz America/New_York`) |
| `!mytz` | Show your timezone and current local time |
| `!timezone list` | World clock for everyone in the room who has set a timezone |
| `!quote [@user]` | Random starred quote (star a message to save it) |
| `!np [game]` | Set your now-playing status |
| `!np @user` | Check what someone is playing |
| `!np list` | See what everyone is playing |
| `!backlog add <game>` | Add a game to your backlog |
| `!backlog list` | View your backlog |
| `!backlog random` | Random pick from your backlog |
| `!backlog done <game>` | Mark a game as completed |
| `!watch <keyword>` | Get a DM when someone mentions a keyword |
| `!watching` | List your keyword watches |
| `!unwatch <keyword\|id>` | Remove a keyword watch |

### Presence

| Command | Description |
|---|---|
| `!away [message]` | Set away status with optional message |
| `!afk [message]` | Set AFK status with optional message |
| `!back` | Clear away/afk status |
| `!whois @user` | Full profile card (status, timezone, now playing, rep, level, streak) |

Away/AFK status auto-clears silently when the user sends any message.

### Weather

| Command | Description |
|---|---|
| `!weather <city>` | Current weather for a city name (e.g., `!weather Tokyo`) |
| `!weather <zip[,CC]>` | Weather by zip/postal code — defaults to US, append country code for others (e.g., `!weather 10115,DE`) |
| `!weather @user` | Weather for a user's location, derived from their `!settz` timezone |

Weather uses the OpenWeatherMap Geocoding API to resolve locations before fetching conditions. For ambiguous city names (e.g., "Springfield"), US matches are preferred. Output includes current conditions, temperature in both °C and °F, feels-like temperature, humidity, and wind speed.

### Fun

| Command | Description |
|---|---|
| `!roll [N]d<sides>[+/-mod]` | Dice roller (e.g., `!roll d20`, `!roll 2d6+3`) |
| `!8ball <question>` | Magic 8-Ball |
| `!coin` | Coin flip |
| `!time <city\|@user>, ...` | World clock (comma-separated, supports users) |
| `!hltb <game>` | HowLongToBeat lookup (main, extra, completionist) |
| `!twinbee` | Random TwinBee/Parodius lore fact |
| `!poll "Q" "A" "B" ...` | Reaction poll (max 10 options, auto-reacts with number emojis) |
| `!calc <expression>` | Inline calculator (e.g., `!calc 15% of 84`, `!calc sqrt(144)`) |
| `!qr <text or URL>` | Generate a QR code image and post it to the room |
| `!dadjoke` | Random dad joke |
| `!randomwiki` | Random Wikipedia article summary |

### Reactions

| Command | Description |
|---|---|
| `!emojiboard` | Reaction leaderboard (top givers, top receivers, most used emoji) |

### Trivia

| Command | Description |
|---|---|
| `!trivia [category] [easy\|medium\|hard]` | Start a trivia question |
| `!trivia stop` | Cancel active question (admin or asker only) |
| `!trivia scores [@user]` | Trivia leaderboard or individual scores |
| `!trivia scores month` | Current month leaderboard |
| `!trivia categories` | List available categories |
| `!trivia fastest` | Speed hall of fame |

Answer by typing the letter (A/B/C/D) or True/False. First correct answer wins. Scoring is time-weighted: full 100 points within 3 seconds, decaying linearly to 0 at timeout (default 20 seconds, configurable via `TRIVIA_TIMEOUT_SECONDS`).

**Threading:** The first `!trivia` command in a room creates a dedicated trivia thread. All questions, answers, and subcommands (`scores`, `categories`, `fastest`, `stop`) are posted within that thread to keep the main room clean. New questions can be started from the main room (with optional category/difficulty), but answers and subcommands must be typed in the thread.

### Game Releases

| Command | Description |
|---|---|
| `!releases` | Today's game releases |
| `!releases week` | Releases in the next 7 days |
| `!releases month` | Releases in the next 30 days |
| `!releases search <game>` | Search for a game's release info |

### Game Lookup

| Command | Description |
|---|---|
| `!game <game>` | Look up any game (name, year, platforms, developer, genre, metacritic, summary) |
| `!retro <game>` | Alias for `!game` |

Results are cached for 7 days. Uses the same RAWG API key as `!releases`.

### Word of the Day

| Command | Description |
|---|---|
| `!wotd` | Show today's Word of the Day (word, definition, example, bonus XP info) |

### Holidays

| Command | Description |
|---|---|
| `!holidays` | Full holiday and observance list for today |
| `!holidays week` | Holidays in the next 7 days |
| `!holidays month` | Holidays in the next 30 days |

### Birthdays

| Command | Description |
|---|---|
| `!birthday set <month> <day> [year]` | Store your birthday (year optional for age privacy) |
| `!birthday [@user]` | View a birthday |
| `!birthdays` | Upcoming birthdays in the next 30 days |
| `!birthday remove` | Delete your birthday |

Birthday announcements post automatically at 07:05 UTC with **100 bonus XP**.

### Stocks

| Command | Description |
|---|---|
| `!stock <TICKER>` | Stock price, change, 52-week range |
| `!stock <T1> <T2> <T3>` | Multi-ticker lookup |
| `!stockwatch <TICKER>` | Add to personal watchlist |
| `!stockwatch list` | Show watchlist with cached prices |
| `!stockwatch remove <TICKER>` | Remove from watchlist |

Requires `FINNHUB_API_KEY`. Per-user 60-second cooldown.

### Concerts

| Command | Description |
|---|---|
| `!concerts <artist>` | Upcoming shows for an artist |
| `!concerts watch <artist>` | DM when artist announces a show |
| `!concerts watching` | List watched artists |
| `!concerts unwatch <artist>` | Remove artist watch |

Requires `BANDSINTOWN_API_KEY`. Weekly digest posts on Sundays.

### Anime

| Command | Description |
|---|---|
| `!anime search <title>` | Search MyAnimeList (top 3 results) |
| `!anime <title or MAL ID>` | Full details (synopsis, score, genres, schedule) |
| `!anime watch <title>` | Add to anime watchlist |
| `!anime watching` | Your anime watchlist |
| `!anime unwatch <title\|id>` | Remove from watchlist |
| `!anime season` | Current season top 10 |
| `!anime upcoming` | Next season preview |

Daily airing alerts post at 19:30 UTC for watchlisted shows.

### Movies & TV

| Command | Description |
|---|---|
| `!movie <title>` | Movie details (rating, runtime, genres, overview) |
| `!tv <title>` | TV show details (seasons, status, next episode) |
| `!movie watch <title>` | Add movie to watchlist |
| `!tv watch <title>` | Add TV show to watchlist |
| `!movie watching` | Combined movie + TV watchlist |
| `!movie unwatch <title>` | Remove from watchlist |
| `!upcoming movies [week\|month]` | Upcoming theatrical releases |

Requires `TMDB_API_KEY`. Daily release alerts post at 20:00 UTC.

### Lookup

| Command | Description |
|---|---|
| `!wiki <topic>` | Wikipedia summary |
| `!define <word>` | Dictionary definition (Free Dictionary API) |
| `!translate [lang] <text>` | Translate text (requires self-hosted LibreTranslate) |
| `!urban <term>` | Urban Dictionary lookup (cached 24h) |

Translation defaults to English. Specify a 2-letter language code as the first word to change target (e.g., `!translate pt Hello, how are you?`).

### Countdowns

| Command | Description |
|---|---|
| `!countdown add "<label>" <YYYY-MM-DD>` | Add a public countdown |
| `!countdown private "<label>" <YYYY-MM-DD>` | Add a personal countdown |
| `!countdown` | List all public + your private countdowns |
| `!countdown mine` | Your countdowns only |
| `!countdown remove <id>` | Delete your countdown |
| `!countdown <id>` | Single countdown detail |

Passed countdowns show as completed for 7 days, then auto-archive.

### Markov

| Command | Description |
|---|---|
| `!markov @user` | Generate a sentence in that user's style |
| `!markov` | Generate from the whole room's corpus |
| `!markov me` | Generate in your own voice |

Per-user trigram Markov chain trained on message history. Requires at least 50 messages. No content filtering — intentional chaos.

### LLM Classifier (Optional)

These commands are only available when `OLLAMA_HOST` and `OLLAMA_MODEL` are configured.

| Command | Description |
|---|---|
| `!potty [@user]` | Profanity stats (total + severity breakdown) |
| `!pottyboard` | Room profanity leaderboard |
| `!insults [@user]` | Insult stats (thrown/received) |
| `!insultboard` | Most prolific insulters leaderboard |
| `!wotd attempts` | Who tried to use today's WOTD and whether they got credit |
| `!howami [@user]` | Get roasted based on your actual stats (XP, messages, profanity, trivia, achievements, sentiment, etc.) |
| `!vibe` | LLM reads recent messages and describes the current room energy (5-min cooldown per room) |
| `!tldr [n]` | Summarize the last N messages (default: all buffered, max 50) for someone catching up |

The classifier also provides emoji reactions as real-time feedback (with automatic retry on connection failures):
- **Profanity**: 🫣 (mild), 😲 (moderate), 🤬 (severe)
- **Insults**: 🖕 (bot targeted), 🎯 (direct), 💨 (indirect)
- **WOTD**: 📖 (correct usage), 🤔 (incorrect)
- **Sentiment** (when no other reaction applies): random emoji from pools for happy, sad, angry, excited, funny, love, and scared

`!vibe` and `!tldr` share a rolling in-memory buffer of the last 50 text messages per room. The buffer is not persisted — it resets on restart. At least 10 messages are required before either command works.

### Achievements

| Command | Description |
|---|---|
| `!achievements [@user]` | View unlocked achievements |

### Help

| Command | Description |
|---|---|
| `!help` | Full command list grouped by plugin (sent as a DM) |

### Admin

| Command | Description |
|---|---|
| `!schedule <job> <HH:MM>` | Change a scheduled post time (UTC) |
| `!botinfo` | Bot diagnostics (uptime, messages processed, DB size, LLM status, estimated tokens) |

Jobs: `wotd`, `holidays`, `releases`, `birthday_check`, `anime_releases`, `movie_releases`, `concert_digest`

---

## Passive Features

These features run silently on every message. The bot never announces passive tracking — commands are the only reveal mechanism.

| Feature | Description |
|---|---|
| **XP tracking** | Grants 10 XP per message (30s cooldown); announces level-ups with Twinbee/Parodius-themed messages |
| **Stats tracking** | Counts words, links, images, questions, emojis, message length, hourly/daily distributions |
| **Streak tracking** | Tracks daily activity and consecutive-day streaks |
| **First! tracking** | Records who posts the first message each day per room |
| **Thanks detection** | Detects thank-you messages and grants reputation |
| **WOTD detection** | Grants 25 bonus XP for using the word of the day |
| **Achievement evaluation** | Checks all 32 achievements on every message |
| **Keyword alerts** | DMs users when watched keywords appear |
| **Quote saving** | Star-reacting a message saves it as a quote |
| **Presence tracking** | Updates last-seen time; auto-clears away/afk on message |
| **Markov corpus** | Collects non-command messages for text generation (capped at 10,000 per user per room) |
| **URL previews** | Extracts og:title and og:description from shared links (if `FEATURE_URL_PREVIEW=true`) |
| **Welcome detection** | Greets first-time users with 25 XP |
| **Reaction tracking** | Logs who gives reactions to whom and which emoji, for `!emojiboard` |
| **Conversation milestones** | Tracks total room messages, celebrates at 1k, 5k, 10k, 25k, 50k, 100k, 250k, 500k, and 1M |
| **LLM classification** | Classifies profanity (multilingual), insults, sentiment, gratitude, and WOTD correctness via Ollama (if configured). Uses keyword pre-filter, non-ASCII detection, and random sampling (`LLM_SAMPLE_RATE`) to catch profanity in any language. Provides emoji reactions as feedback. Includes exponential backoff and robust JSON repair for small model output. |
| **Trivia answer detection** | Intercepts letter answers (A/B/C/D) when a question is active (within the trivia thread) |

---

## Scheduled Posts

The bot posts automated content to rooms listed in `BOT_ROOMS`.

### Word of the Day (default: 08:00 UTC)
Posts a word from Wordnik with its definition, part of speech, and an example sentence. Users who use the word in a message that day earn **25 bonus XP** (once per user per day). Use `!wotd` at any time to see the current word without waiting for the scheduled post.

### Holidays & Observances (default: 07:00 UTC)
Posts a morning summary combining data from multiple calendar sources:
- **Calendarific (US)** — civic and Western holidays
- **Calendarific (Asian)** — holidays from Japan, China, South Korea, India, Thailand, Vietnam, Taiwan, and Philippines (configurable via `ASIAN_HOLIDAY_COUNTRIES`)
- **HebCal** — Jewish calendar events and Hebrew date
- **Aladhan** — Islamic holidays and Hijri date

The post includes a multi-calendar date header (Gregorian + Hebrew + Hijri), sections for religious observances, Asian holidays (with country labels), and other observances, plus a featured highlight that **prefers non-Western holidays** when available. Use `!holidays week` or `!holidays month` to look ahead — range queries are cached for 1 hour to avoid excessive API calls.

### Birthday Announcements (default: 07:05 UTC)
Checks all stored birthdays against today's date. Posts a birthday message to each relevant room, DMs the birthday person, and grants **100 bonus XP**. If year is stored, includes the person's age.

### Game Releases (default: 19:00 UTC)
Posts today's game releases from RAWG with platform info. Cross-references the HLTB cache to append estimated playtimes when available. Also checks the release watchlist and sends DM notifications to users watching for specific games.

### Anime Airings (default: 19:30 UTC)
Posts a summary of watchlisted anime airing today. DMs users whose watchlisted shows have new episodes.

### Movie & TV Releases (default: 20:00 UTC)
Posts watchlisted movie and TV releases for today. DMs users when their watchlisted titles release.

### Concert Digest (default: 10:00 UTC, Sundays only)
Posts "This Week in Live Music" with concerts in the next 7 days for watched artists. DMs users watching artists with upcoming shows. Only fires on Sundays.

### Prefetch (default: 00:05 UTC)
Fetches API data for Holidays and WOTD ahead of their scheduled post times and caches it in the `daily_prefetch` table. This ensures the actual post jobs have no network dependency and can deliver content even if the APIs are temporarily down. Each source is fetched independently — one failure doesn't block the others.

### Nightly Maintenance (default: 00:15 UTC)
Prunes old data to prevent database bloat. Retention policies: `llm_classifications` (30 days), `xp_log` (30 days), `command_usage` (7 days), `rep_cooldowns` (2 days), `daily_prefetch` (3 days), all cache tables (7–30 days depending on type), and `shade_log` (30 days). Runs after prefetch so fresh data is never pruned.

---

## Achievements

All 32 achievements are evaluated silently. Use `!achievements` to see your progress.

| Achievement | Condition |
|---|---|
| Encyclopedist | Write 100,000 total words |
| Linkdump | Post 500 links |
| Night Shift | Send 100 messages between 00:00–04:00 UTC |
| Riddler | Ask 500 questions |
| Show Don't Tell | Post 200 images |
| Hemingway | Send 1,000 messages averaging under 5 words each |
| Tolstoy | Send 1,000 messages averaging over 50 words each |
| Logophile | Use a word longer than 15 characters |
| Omnipresent | Be active 30 unique days in a single calendar month |
| Early Bird Legend | Hold First! for 30 days total |
| Streak Week | Achieve a 7-day streak |
| Streak Month | Achieve a 30-day streak |
| Beloved | Earn 50 reputation points |
| Gamer | Have 10 items in your backlog |
| Completionist | Complete 10 backlog items |
| First Blood | First correct trivia answer |
| The Scholar | 100 correct trivia answers |
| Speed Demon | Correct trivia answer in under 2 seconds |
| On a Roll | 10 correct trivia answers in a row |
| Birthday Bee | Had your birthday celebrated by Freebee |
| Word Nerd | Used the WOTD correctly 10 times |
| Nice Try | Attempted to game the WOTD 5 times |
| Needs Soap | 50 profanity detections |
| Sailor Mouth | 500 profanity detections |
| The Roaster | 50 insults thrown |
| Punching Bag | Targeted 50 times |
| Welcome Wagon | First message ever in this room |
| Countdown Keeper | 5 active countdowns simultaneously |
| Markov Victim | Had someone run !markov on you |
| Stonks | 5 tickers on stock watchlist |
| Certified Weeaboo | 10 anime on watchlist |
| Cinephile | 10 movies/TV shows on watchlist |
| Concert Goer | Watching 5 artists |

---

## Personality Archetypes

The `!personality` command assigns an archetype based on your messaging patterns. First match wins:

| Archetype | Criteria |
|---|---|
| The Chatterbox | 1,000+ messages with avg < 10 words |
| The Novelist | Avg > 40 words per message |
| The Inquisitor | 30%+ of messages are questions |
| The Linkmaster | 20%+ of messages contain links |
| The Shutterbug | 15%+ of messages contain images |
| The Night Owl | 40%+ of messages sent between 00:00–03:59 UTC |
| The Early Bird | 40%+ of messages sent between 05:00–09:59 UTC |
| The Enthusiast | 30%+ of messages contain exclamation marks |
| The Regular | Default fallback |

---

## Supported Timezones

The `!settz` and `!time` commands accept either a city name or a full IANA timezone identifier (e.g., `America/New_York`).

Built-in city shortcuts:

| City | Timezone |
|---|---|
| New York, NYC | America/New_York |
| Los Angeles, LA | America/Los_Angeles |
| Chicago | America/Chicago |
| Denver | America/Denver |
| Phoenix | America/Phoenix |
| Honolulu, Hawaii | Pacific/Honolulu |
| Anchorage | America/Anchorage |
| Toronto | America/Toronto |
| Vancouver | America/Vancouver |
| Mexico City | America/Mexico_City |
| Sao Paulo | America/Sao_Paulo |
| London | Europe/London |
| Paris | Europe/Paris |
| Berlin | Europe/Berlin |
| Amsterdam | Europe/Amsterdam |
| Rome | Europe/Rome |
| Madrid | Europe/Madrid |
| Lisbon | Europe/Lisbon |
| Oslo | Europe/Oslo |
| Stockholm | Europe/Stockholm |
| Helsinki | Europe/Helsinki |
| Warsaw | Europe/Warsaw |
| Prague | Europe/Prague |
| Vienna | Europe/Vienna |
| Zurich | Europe/Zurich |
| Athens | Europe/Athens |
| Istanbul | Europe/Istanbul |
| Moscow | Europe/Moscow |
| Dubai | Asia/Dubai |
| Mumbai, Delhi | Asia/Kolkata |
| Bangkok | Asia/Bangkok |
| Jakarta | Asia/Jakarta |
| Singapore | Asia/Singapore |
| Kuala Lumpur | Asia/Kuala_Lumpur |
| Hong Kong | Asia/Hong_Kong |
| Taipei | Asia/Taipei |
| Manila | Asia/Manila |
| Shanghai, Beijing | Asia/Shanghai |
| Seoul | Asia/Seoul |
| Tokyo | Asia/Tokyo |
| Sydney | Australia/Sydney |
| Melbourne | Australia/Melbourne |
| Auckland | Pacific/Auckland |
| Cairo | Africa/Cairo |
| Johannesburg | Africa/Johannesburg |

Any valid IANA timezone (e.g., `US/Eastern`, `Europe/Zurich`) also works.

---

## External APIs

| API | Used By | Purpose | Key Required |
|---|---|---|---|
| [Wordnik](https://developer.wordnik.com/) | Word of the Day | Daily word, definition, examples | Yes |
| [RAWG](https://rawg.io/apidocs) | Game releases | Release dates, platforms, ratings | Yes |
| [Calendarific](https://calendarific.com/) | Holidays | Civic, Western, and Asian holidays | Yes |
| [HebCal](https://www.hebcal.com/home/developer-apis) | Holidays | Hebrew calendar, Jewish observances | No |
| [Aladhan](https://aladhan.com/prayer-times-api) | Holidays | Hijri calendar, Islamic observances | No |
| [OpenWeatherMap](https://openweathermap.org/api) | `!weather` | Geocoding and current weather conditions | Yes |
| [Finnhub](https://finnhub.io/) | `!stock` | Stock quotes, company profiles, metrics | Yes |
| [Bandsintown](https://artists.bandsintown.com/) | `!concerts` | Concert listings, artist events | Yes |
| [TMDB](https://www.themoviedb.org/documentation/api) | `!movie`, `!tv` | Movie/TV details, upcoming releases | Yes |
| [Jikan/MAL](https://jikan.moe/) | `!anime` | Anime search, details, seasons | No |
| [OpenTDB](https://opentdb.com/) | `!trivia` | Trivia questions | No |
| [Wikipedia](https://www.mediawiki.org/wiki/REST_API) | `!wiki` | Article summaries | No |
| [Free Dictionary](https://dictionaryapi.dev/) | `!define` | Word definitions | No |
| [LibreTranslate](https://libretranslate.com/) | `!translate` | Text translation (self-hosted) | No |
| [HowLongToBeat](https://howlongtobeat.com/) | `!hltb`, release enrichment | Game completion times (via scraper library) | No |
| [icanhazdadjoke](https://icanhazdadjoke.com/) | `!dadjoke` | Random dad jokes | No |
| [Urban Dictionary](https://urbandictionary.com/) | `!urban` | Slang/term definitions | No |
| [Ollama](https://ollama.ai/) | LLM classifier | Profanity/insult/WOTD classification (self-hosted) | No |

All API calls are wrapped in try/catch. If any API is down, the bot logs a warning and skips that feature — it never crashes over a failed API call.

---

## Architecture

```
freebee/
├── package.json
├── tsconfig.json
├── .env.example
├── .env                       # never committed
├── .gitignore
├── Dockerfile
├── docker-compose.yml
├── data/                      # runtime data (gitignored)
│   ├── freebee.db             # SQLite database
│   ├── crypto-js/             # E2EE device identity (BACK THIS UP)
│   ├── device.json            # device ID persistence
│   └── freebee.log            # rotating log file
└── src/
    ├── index.ts               # entry point, token management, plugin wiring
    ├── matrix-client.ts       # BotClient wrapper around matrix-js-sdk
    ├── db/
    │   └── index.ts           # schema (39 tables), initDb(), getDb()
    ├── utils/
    │   ├── auth.ts            # token validation & password login
    │   ├── logger.ts          # winston logger
    │   └── parser.ts          # message parsing, XP math, archetypes
    ├── plugins/
    │   ├── base.ts            # Plugin class, PluginRegistry
    │   ├── xp.ts              # XP & leveling
    │   ├── reputation.ts      # thanks detection, rep points
    │   ├── stats.ts           # message metrics
    │   ├── streaks.ts         # daily streaks, First!
    │   ├── reminders.ts       # reminder system
    │   ├── user.ts            # timezones, quotes, backlog, keyword alerts
    │   ├── fun.ts             # dice, 8ball, polls, weather, HLTB
    │   ├── wotd.ts            # Word of the Day
    │   ├── holidays.ts        # multi-calendar holidays
    │   ├── gaming.ts          # game releases (RAWG)
    │   ├── daily.ts           # DailyScheduler (9 jobs incl. prefetch & maintenance)
    │   ├── achievements.ts    # 32 silent achievements
    │   ├── ratelimits.ts      # per-command daily quota middleware
    │   ├── birthday.ts        # birthday storage & announcements
    │   ├── trivia.ts          # OpenTDB trivia with scoring
    │   ├── llm-passive.ts     # Ollama classifier (optional)
    │   ├── stocks.ts          # Finnhub stock quotes
    │   ├── concerts.ts        # Bandsintown concerts
    │   ├── anime.ts           # Jikan/MAL anime tracking
    │   ├── movies.ts          # TMDB movie/TV tracking
    │   ├── lookup.ts          # wiki, define, translate
    │   ├── presence.ts        # away/afk/back, whois
    │   ├── countdown.ts       # community countdowns
    │   ├── welcome.ts         # new user greeting, !help
    │   ├── markov.ts          # trigram Markov chains
    │   ├── urls.ts            # URL og:tag previews
    │   ├── tools.ts           # calc, QR code generator
    │   ├── reactions.ts       # reaction tracking, emojiboard
    │   ├── botinfo.ts         # admin diagnostics
    │   ├── retro.ts           # game lookup (RAWG) — !game and !retro
    │   ├── howami.ts          # LLM-powered stat roasts
    │   ├── vibe.ts            # room vibe check + !tldr summaries
    │   └── shade.ts           # STUB — feature flagged off
    └── workers/
        └── llm-classifier.ts  # STUB — superseded by llm-passive.ts
```

### Plugin System

Every plugin extends the `Plugin` abstract class and implements:
- `name` — plugin identifier
- `commands` — list of command definitions
- `onMessage(ctx)` — called for every message (both passive tracking and command handling)
- `onReaction(ctx)` — optional, called for reactions (e.g., star → quote)

Plugins interact with Matrix through the `IMatrixClient` interface defined in `base.ts`. The `BotClient` class in `matrix-client.ts` wraps `matrix-js-sdk` and implements this interface, allowing SDK changes without touching plugin code.

Plugins are registered with `PluginRegistry`, which dispatches messages and reactions to all plugins in order. XP is registered first since other plugins depend on it. Welcome is registered last so it can list all commands via `!help`.

### Key Design Decisions

- **Almost all passive tracking is silent.** The bot never announces streaks, First!, keyword watches, or achievements — commands are the only reveal mechanism. The one exception is **level-ups**, which get a celebratory Twinbee/Parodius-themed announcement.
- **XpPlugin.grantXp() is the single XP entry point** — Birthday, WOTD, Trivia, Welcome, Reputation, and LLM classifier all route through it.
- **LLM features are fully optional.** Gated by `OLLAMA_HOST` and `OLLAMA_MODEL` — both must be set for classifier to activate. All LLM commands return "not configured" otherwise. The classifier queue is capped at 50 items, requests time out after 30 seconds, a 16k context window is configured for Ollama, and exponential backoff (5s–5min) prevents hammering a down Ollama instance. When active, LLM replaces keyword-based rep granting with sarcasm-aware gratitude detection.
- **Rate limits are centralized** in `RateLimitsPlugin`. Paid-API commands call `checkLimit()` before requests. Admins bypass all limits.
- **Stats are computed from stored primitives**, not pre-aggregated. `!rankings` does live SQL queries.
- **Markov corpus is capped** at 10,000 rows per user per room. Oldest pruned on overflow.
- **URL previews are text-only** — no image reposting, one clean line of description.
- **Presence auto-clears** on any message from an away user. Silent.
- **`!help` always DMs** — never floods the room with a command list.
- **Trivia is one-at-a-time per room** and runs in a dedicated Matrix thread. Concurrent sessions not supported.
- **Concert digest is Sunday-only** — job fires daily, handler checks day-of-week internally.
- **The scheduler reads config from the DB each tick** so `!schedule` changes take effect without restart.

---

## Database

Freebee uses SQLite via `better-sqlite3` in WAL mode. The database file is at `DATA_DIR/freebee.db`.

### Tables

| Table | Purpose |
|---|---|
| `users` | XP, level, rep, timezone per (user, room) |
| `user_stats` | Message metrics, distributions, streaks |
| `xp_log` | Audit trail for XP grants |
| `rep_cooldowns` | 24h giver-to-receiver cooldown |
| `reminders` | Pending reminders with fired flag |
| `daily_activity` | One row per (user, room, date) for streak calc |
| `daily_first` | One row per (room, date) — who posted first |
| `wotd_log` | Posted words by date |
| `wotd_usage` | Who used the word, XP awarded |
| `holidays_log` | Full holiday JSON per room per date |
| `releases_cache` | RAWG data cache |
| `release_watchlist` | Per-user game release watches |
| `hltb_cache` | HLTB results (24h TTL) |
| `achievements` | Unlocked achievements per (user, room) |
| `quotes` | Star-saved messages |
| `now_playing` | Current game per (user, room) |
| `backlog` | Game backlog with completed flag |
| `predictions` | Logged predictions (future feature) |
| `keyword_watches` | Per-user keyword DM alerts |
| `scheduler_config` | Job name, hour, minute, enabled flag |
| `shade_log` | Future shade tracking (schema only) |
| `shade_optout` | Future opt-out list (schema only) |
| `birthdays` | Birthday storage (month, day, optional year) |
| `birthday_fired` | Dedup birthday notifications per year |
| `trivia_sessions` | Trivia question log |
| `trivia_scores` | Per-user trivia aggregates and streaks |
| `llm_classifications` | Raw LLM classifier output |
| `potty_mouth` | Profanity aggregates per user |
| `insult_log` | Insult aggregates (thrown/received) |
| `stocks_cache` | Finnhub quote/profile cache (15m/24h TTL) |
| `stock_watchlist` | Per-user stock ticker watches |
| `command_usage` | Per-user daily command rate limit counters |
| `concerts_cache` | Bandsintown event cache (6h TTL) |
| `concert_watchlist` | Per-user artist watches |
| `anime_watchlist` | Per-user anime watches |
| `anime_cache` | Jikan API cache (24h TTL) |
| `movie_watchlist` | Per-user movie/TV watches |
| `movie_cache` | TMDB API cache (24h TTL) |
| `countdowns` | Public + private countdowns |
| `presence` | Away/afk/online status per user per room |
| `markov_corpus` | Per-user message history for Markov chains |
| `urban_cache` | Urban Dictionary result cache (24h TTL) |
| `room_milestones` | Room message count and last celebrated milestone |
| `reaction_log` | Emoji reaction tracking (giver, receiver, emoji) |
| `retro_cache` | RAWG retro game lookup cache (7d TTL) |
| `url_cache` | URL og:tag preview cache (24h TTL) |
| `sentiment_stats` | Aggregated sentiment counts per (user, room) |
| `daily_prefetch` | Cached API data for scheduled posts (holidays, WOTD) |

All tables are created automatically on first run.

---

## Troubleshooting

### Bot won't start

- **"Missing required env vars"** — Make sure `MATRIX_HOMESERVER_URL` and `MATRIX_BOT_USER_ID` are set. Provide either `MATRIX_ACCESS_TOKEN` or `MATRIX_BOT_PASSWORD`.
- **"Database not initialized"** — The `DATA_DIR` directory must be writable. Check permissions.
- **Native module errors** — Run `npm rebuild` after switching Node.js versions. `better-sqlite3` has native bindings.
- **"Login failed"** — If using password auth, verify the password is correct. Special characters in passwords are handled properly by the bot (unlike manual curl commands).

### Bot doesn't respond to messages

- Verify the bot has joined the room. The bot auto-joins on invite, but check its room list.
- Check that the bot's user ID in `MATRIX_BOT_USER_ID` exactly matches the account the access token belongs to (the bot ignores its own messages using this ID).
- Make sure messages use the correct prefix (default `!`).
- Check `data/freebee.log` for errors.

### Bot can't decrypt messages

- The crypto store at `DATA_DIR/crypto-js/` and device identity at `DATA_DIR/device.json` must be persisted across restarts. Losing them means the bot creates a new device and must re-establish sessions.
- If using Docker, make sure the `./data` volume mount is working correctly.
- If the crypto store is corrupted, run with `CRYPTO_RESET=true` to cleanly wipe and re-establish keys. This deletes the old device from the server, wipes the local crypto store and `device.json`, and sends a key exchange announcement to `BOT_ROOMS`. The flag is single-use — remove it after the reset.
- After a crypto reset, other users' clients will need to exchange keys with the bot's new device. This happens automatically — `matrix-js-sdk` handles key gossiping, device tracking, and session management without manual intervention.

### Scheduled posts aren't working

- Make sure `BOT_ROOMS` is set with at least one room ID.
- Verify API keys are set for the relevant feature (`WORDNIK_API_KEY` for WOTD, `RAWG_API_KEY` for releases, `CALENDARIFIC_API_KEY` for holidays, etc.).
- Schedule times are in **UTC**. Use `!schedule` to check/adjust.
- The scheduler checks every 60 seconds, so there may be up to a 60-second delay.

### Reminders don't fire

- Reminders are checked every 30 seconds. Very short reminders (< 30 seconds) may be slightly delayed.
- Reminder times are stored as UTC. If your times seem off, set your timezone with `!settz`.

### HLTB lookups fail

- The `howlongtobeat` library scrapes the HLTB website. It can break when the site changes. Check for library updates: `npm update howlongtobeat`.
- Results are cached for 24 hours to reduce scraping load.

### Weather returns wrong city

- The weather command uses OpenWeather's Geocoding API to resolve locations. For ambiguous city names (e.g., "Springfield"), it prefers US matches.
- To specify a country, append a country code: `!weather Springfield,GB`.
- Zip codes default to US. For other countries, append the code: `!weather 10115,DE`.

### LLM features show "not configured"

- Both `OLLAMA_HOST` and `OLLAMA_MODEL` must be set and non-empty.
- Verify Ollama is running and accessible at the specified host.
- The bot connects to Ollama via HTTP — no authentication is needed.
- The classifier includes robust JSON repair for small models that produce malformed output (trailing commas, single quotes, unquoted keys, markdown fences, etc.).

### Rate limit errors

- Users get a quota message when they exceed their daily limit for rate-limited commands.
- Admins (`BOT_ADMIN_USERS`) bypass all rate limits.
- Limits reset at midnight UTC.

### High memory usage

- SQLite WAL mode can accumulate large WAL files under heavy write load. The database checkpoints automatically, but you can force one by restarting the bot.

### Permission errors in Docker

- The `data/` directory must be writable by the container's user. If you see permission errors:
  ```bash
  chmod -R 777 ./data  # or match the container's UID
  ```

---

## Backup & Recovery

### What to back up

1. **`DATA_DIR/crypto-js/`** and **`DATA_DIR/device.json`** — The bot's E2EE device identity and crypto state. Losing these means the bot creates a new device and must re-establish all sessions. **This is the most critical data to back up.**
2. **`DATA_DIR/freebee.db`** — All user data, stats, achievements, reminders, quotes, watchlists, Markov corpus, etc.
3. **`.env`** — Your configuration (contains secrets, store securely).

### Backup script example

```bash
#!/bin/bash
BACKUP_DIR="/backups/freebee/$(date +%Y-%m-%d)"
mkdir -p "$BACKUP_DIR"
cp -r ./data/crypto-js "$BACKUP_DIR/"
cp ./data/device.json "$BACKUP_DIR/"
cp ./data/freebee.db "$BACKUP_DIR/"
echo "Backup complete: $BACKUP_DIR"
```

### Recovery

1. Stop the bot
2. Restore `crypto-js/` directory, `device.json`, and `freebee.db` to the `DATA_DIR` location
3. Start the bot

If only the database is lost (crypto store intact), the bot will create a fresh database but retain its device identity. All user data will be reset.

If only the crypto store is lost (database intact), delete the old `crypto-js/` directory and `device.json`, restart the bot, and it will create a new device identity. User data is preserved. Other clients will automatically share keys with the new device as they send messages.

---

## License

MIT
