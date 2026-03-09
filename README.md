# GogoBee

Matrix community bot with E2EE, 35+ plugins, passive tracking, scheduled posts, and optional LLM features.

Written in Go using [mautrix-go](https://github.com/mautrix/go) for encryption and [modernc.org/sqlite](https://modernc.org/sqlite) for storage. Successor to the TypeScript "Freebee" bot.

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

- **E2EE that actually works** - mautrix-go with goolm (pure Go). Crypto state lives in SQLite so device keys survive restarts. Cross-signing bootstraps on first run. Verify once, done.
- **No CGo, no system deps** - builds to a single static binary. Cross-compile to whatever you want.
- **35+ plugins** with dependency injection and ordered registration
- **Passive tracking** - XP, stats, streaks, achievements, markov corpus, keyword alerts, all running silently
- **Scheduled posts** via [robfig/cron](https://github.com/robfig/cron) - WOTD, holidays, game releases, birthdays, anime/movie releases, concert digests
- **LLM integration** (optional) - Ollama-powered sentiment analysis, roast profiles, room vibes, conversation summaries
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

### Services (optional)

| Variable | Description |
|----------|-------------|
| `OLLAMA_HOST` | Ollama server URL, e.g. `http://localhost:11434` |
| `OLLAMA_MODEL` | Model name, e.g. `llama3.2` |
| `LIBRETRANSLATE_URL` | LibreTranslate instance for `!translate` |

### Feature Flags

| Variable | Description |
|----------|-------------|
| `FEATURE_URL_PREVIEW` | Set to anything to enable automatic URL previews |
| `FEATURE_SHADE` | Set to anything to enable the shade plugin (stub) |

### Rate Limits

| Variable | Default | Description |
|----------|---------|-------------|
| `RATELIMIT_TRANSLATE` | `20` | Daily translation limit per user |

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

1. Start the bot. It logs in, creates a device, and sets up cross-signing automatically.
2. Verify the bot's device from your main Matrix account (Element, etc).
3. That's it. E2EE works across restarts from here on out.

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
| `!time [city]` | World clock |
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
| `!quote` | Random saved quote |
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

### LLM (requires Ollama)
| Command | Description |
|---------|-------------|
| `!howami [@user]` | Roast profile |
| `!vibe` | Room energy check |
| `!tldr` | Summarize recent chat |
| `!potty [@user]` | Profanity count |
| `!pottyboard` | Profanity leaderboard |
| `!insults [@user]` | Insult stats |
| `!insultboard` | Insult leaderboard |

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
- **URL previews** - OG tag extraction (feature-flagged, off by default)
- **Reactions** - logs all reactions for `!emojiboard`
- **LLM classification** - sentiment, profanity, insults, WOTD usage (needs Ollama)
- **Quotes** - star-react any message to save it

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
| Every 30s | Reminders | Fires pending reminders |

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
| [LibreTranslate](https://libretranslate.com) | Self-host | Translation |
| [Ollama](https://ollama.ai) | Self-host | LLM features |

---

## Architecture

```
gogobee/
├── main.go                  # Entry point, plugin registration, cron setup
├── go.mod / go.sum
├── internal/
│   ├── bot/
│   │   ├── client.go        # mautrix client + E2EE (cryptohelper + goolm)
│   │   └── dispatch.go      # Plugin registry, event dispatch
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
│   │   └── ratelimits.go    # Rate limiting
│   └── util/
│       ├── auth.go          # Matrix login, token check
│       ├── logger.go        # slog logging
│       └── parser.go        # Message parsing, XP math, archetypes
```

### Why Go?

**E2EE** - The TS version used `matrix-js-sdk` with `fake-indexeddb` for an in-memory crypto store. Every restart wiped device keys and required re-verification in all encrypted rooms. mautrix-go stores crypto state in SQLite. Verify once, it sticks.

**Deployment** - Pure Go, no CGo. `go build -tags goolm` gives you a static binary with zero system dependencies. The TS version needed Node.js, npm, a C compiler for better-sqlite3, and libolm.

**Scheduler** - Replaced a hand-rolled 60s tick loop with robfig/cron. Standard cron expressions, less code, fewer bugs.

**Plugins** - Go interfaces + struct embedding instead of abstract classes. Same pattern, less boilerplate.

---

## Database

Single SQLite file at `$DATA_DIR/gogobee.db`. Schema auto-creates on first run. WAL mode enabled.

40+ tables covering users, XP, stats, streaks, reputation, reminders, trivia, achievements, quotes, backlog, keyword watches, scheduler config, birthdays, LLM classifications, stocks, concerts, anime, movies, countdowns, presence, markov corpus, reaction log, and various caches.

### Backup

```bash
# safe to run while the bot is up (WAL mode)
sqlite3 data/gogobee.db ".backup data/gogobee-backup.db"
```

---

## Troubleshooting

### E2EE

E2EE should just work after the initial device verification. If something goes wrong:

1. On first run, the bot sets up cross-signing automatically. Verify its device from your account once.
2. After restarts, the bot reuses its saved device and crypto state. No re-verification needed.
3. If things are really broken, delete `data/device.json` and `data/gogobee.db` to start fresh. You'll need to verify again.

### Bot not responding in encrypted rooms

- Make sure you verified the bot's device
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
