import Database from "better-sqlite3";
import path from "path";
import logger from "../utils/logger";

let db: Database.Database;

export function getDb(): Database.Database {
  if (!db) throw new Error("Database not initialized. Call initDb() first.");
  return db;
}

export function initDb(dataDir: string): Database.Database {
  const dbPath = path.join(dataDir, "freebee.db");
  db = new Database(dbPath);

  db.pragma("journal_mode = WAL");
  db.pragma("foreign_keys = ON");

  createTables();
  runMigrations();
  seedSchedulerDefaults();

  logger.info(`Database initialized at ${dbPath}`);
  return db;
}

function createTables(): void {
  db.exec(`
    CREATE TABLE IF NOT EXISTS users (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      xp INTEGER NOT NULL DEFAULT 0,
      level INTEGER NOT NULL DEFAULT 1,
      rep INTEGER NOT NULL DEFAULT 0,
      timezone TEXT,
      PRIMARY KEY (user_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS user_stats (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      total_messages INTEGER NOT NULL DEFAULT 0,
      total_words INTEGER NOT NULL DEFAULT 0,
      total_characters INTEGER NOT NULL DEFAULT 0,
      total_links INTEGER NOT NULL DEFAULT 0,
      total_images INTEGER NOT NULL DEFAULT 0,
      total_questions INTEGER NOT NULL DEFAULT 0,
      total_exclamations INTEGER NOT NULL DEFAULT 0,
      total_emojis INTEGER NOT NULL DEFAULT 0,
      longest_message INTEGER NOT NULL DEFAULT 0,
      shortest_message INTEGER,
      avg_words_per_message REAL NOT NULL DEFAULT 0,
      hourly_distribution TEXT NOT NULL DEFAULT '{}',
      daily_distribution TEXT NOT NULL DEFAULT '{}',
      current_streak INTEGER NOT NULL DEFAULT 0,
      longest_streak INTEGER NOT NULL DEFAULT 0,
      last_active_date TEXT,
      PRIMARY KEY (user_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS xp_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      amount INTEGER NOT NULL,
      reason TEXT NOT NULL,
      granted_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS rep_cooldowns (
      giver_id TEXT NOT NULL,
      receiver_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      granted_at TEXT NOT NULL DEFAULT (datetime('now')),
      PRIMARY KEY (giver_id, receiver_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS reminders (
      id TEXT PRIMARY KEY,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      message TEXT NOT NULL,
      remind_at TEXT NOT NULL,
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      fired INTEGER NOT NULL DEFAULT 0
    );

    CREATE TABLE IF NOT EXISTS daily_activity (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      date TEXT NOT NULL,
      message_count INTEGER NOT NULL DEFAULT 1,
      PRIMARY KEY (user_id, room_id, date)
    );

    CREATE TABLE IF NOT EXISTS daily_first (
      room_id TEXT NOT NULL,
      date TEXT NOT NULL,
      user_id TEXT NOT NULL,
      timestamp TEXT NOT NULL,
      PRIMARY KEY (room_id, date)
    );

    CREATE TABLE IF NOT EXISTS wotd_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      room_id TEXT NOT NULL,
      date TEXT NOT NULL,
      word TEXT NOT NULL,
      definition TEXT,
      example TEXT,
      part_of_speech TEXT,
      posted_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(room_id, date)
    );

    CREATE TABLE IF NOT EXISTS wotd_usage (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      date TEXT NOT NULL,
      xp_awarded INTEGER NOT NULL DEFAULT 0,
      detected_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(user_id, room_id, date)
    );

    CREATE TABLE IF NOT EXISTS holidays_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      room_id TEXT NOT NULL,
      date TEXT NOT NULL,
      holidays_json TEXT NOT NULL,
      posted_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(room_id, date)
    );

    CREATE TABLE IF NOT EXISTS releases_cache (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      game_name TEXT NOT NULL,
      game_slug TEXT,
      release_date TEXT,
      platforms TEXT,
      genres TEXT,
      rating REAL,
      data_json TEXT NOT NULL,
      fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS release_watchlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      game_name TEXT NOT NULL,
      game_slug TEXT,
      release_date TEXT,
      notified_day_before INTEGER NOT NULL DEFAULT 0,
      notified_day_of INTEGER NOT NULL DEFAULT 0,
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(user_id, room_id, game_name)
    );

    CREATE TABLE IF NOT EXISTS hltb_cache (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      game_name TEXT NOT NULL,
      search_term TEXT NOT NULL,
      main_story REAL,
      main_extra REAL,
      completionist REAL,
      data_json TEXT NOT NULL,
      fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS achievements (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      achievement_key TEXT NOT NULL,
      unlocked_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(user_id, room_id, achievement_key)
    );

    CREATE TABLE IF NOT EXISTS quotes (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      message TEXT NOT NULL,
      quoted_by TEXT NOT NULL,
      event_id TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS now_playing (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      game TEXT NOT NULL,
      started_at TEXT NOT NULL DEFAULT (datetime('now')),
      PRIMARY KEY (user_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS backlog (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      game TEXT NOT NULL,
      completed INTEGER NOT NULL DEFAULT 0,
      added_at TEXT NOT NULL DEFAULT (datetime('now')),
      completed_at TEXT,
      UNIQUE(user_id, room_id, game)
    );

    CREATE TABLE IF NOT EXISTS predictions (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      prediction TEXT NOT NULL,
      outcome TEXT,
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      resolved_at TEXT
    );

    CREATE TABLE IF NOT EXISTS keyword_watches (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      keyword TEXT NOT NULL,
      created_at TEXT NOT NULL DEFAULT (datetime('now')),
      UNIQUE(user_id, room_id, keyword)
    );

    CREATE TABLE IF NOT EXISTS scheduler_config (
      job_name TEXT PRIMARY KEY,
      hour INTEGER NOT NULL,
      minute INTEGER NOT NULL,
      enabled INTEGER NOT NULL DEFAULT 1
    );

    CREATE TABLE IF NOT EXISTS shade_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      target_id TEXT,
      room_id TEXT NOT NULL,
      message TEXT NOT NULL,
      confidence REAL,
      classified_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS shade_optout (
      user_id TEXT PRIMARY KEY,
      opted_out_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    -- Birthdays
    CREATE TABLE IF NOT EXISTS birthdays (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      month INTEGER NOT NULL,
      day INTEGER NOT NULL,
      year INTEGER,
      PRIMARY KEY (user_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS birthday_fired (
      user_id TEXT NOT NULL,
      year INTEGER NOT NULL,
      PRIMARY KEY (user_id, year)
    );

    -- Trivia
    CREATE TABLE IF NOT EXISTS trivia_sessions (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      room_id TEXT NOT NULL,
      question TEXT NOT NULL,
      answer TEXT NOT NULL,
      category TEXT,
      difficulty TEXT,
      asked_at INTEGER DEFAULT (unixepoch()),
      answered_by TEXT,
      answered_at INTEGER,
      correct INTEGER DEFAULT 0
    );

    CREATE TABLE IF NOT EXISTS trivia_scores (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      total_correct INTEGER DEFAULT 0,
      total_points INTEGER DEFAULT 0,
      total_answered INTEGER DEFAULT 0,
      current_streak INTEGER DEFAULT 0,
      best_streak INTEGER DEFAULT 0,
      fastest_ms INTEGER,
      PRIMARY KEY (user_id, room_id)
    );

    -- LLM passive classification
    CREATE TABLE IF NOT EXISTS llm_classifications (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      event_id TEXT NOT NULL,
      profanity INTEGER DEFAULT 0,
      profanity_severity INTEGER DEFAULT 0,
      insult INTEGER DEFAULT 0,
      insult_type TEXT,
      insult_target TEXT,
      insult_severity INTEGER DEFAULT 0,
      sentiment TEXT DEFAULT 'neutral',
      gratitude INTEGER DEFAULT 0,
      gratitude_toward TEXT,
      wotd_used INTEGER DEFAULT 0,
      wotd_correct INTEGER DEFAULT 0,
      classified_at INTEGER DEFAULT (unixepoch())
    );

    CREATE TABLE IF NOT EXISTS potty_mouth (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      total INTEGER DEFAULT 0,
      severity_1 INTEGER DEFAULT 0,
      severity_2 INTEGER DEFAULT 0,
      severity_3 INTEGER DEFAULT 0,
      PRIMARY KEY (user_id, room_id)
    );

    CREATE TABLE IF NOT EXISTS insult_log (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      total_thrown INTEGER DEFAULT 0,
      direct_thrown INTEGER DEFAULT 0,
      indirect_thrown INTEGER DEFAULT 0,
      times_targeted INTEGER DEFAULT 0,
      PRIMARY KEY (user_id, room_id)
    );

    -- Stocks
    CREATE TABLE IF NOT EXISTS stocks_cache (
      ticker TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch())
    );

    CREATE TABLE IF NOT EXISTS stock_watchlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      ticker TEXT NOT NULL,
      created_at INTEGER DEFAULT (unixepoch()),
      UNIQUE(user_id, room_id, ticker)
    );

    -- Rate limiting
    CREATE TABLE IF NOT EXISTS command_usage (
      user_id TEXT NOT NULL,
      command TEXT NOT NULL,
      date TEXT NOT NULL,
      count INTEGER DEFAULT 0,
      PRIMARY KEY (user_id, command, date)
    );

    -- Concerts
    CREATE TABLE IF NOT EXISTS concerts_cache (
      location_key TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch())
    );

    CREATE TABLE IF NOT EXISTS concert_watchlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      artist TEXT NOT NULL,
      created_at INTEGER DEFAULT (unixepoch()),
      UNIQUE(user_id, room_id, artist)
    );

    -- Anime
    CREATE TABLE IF NOT EXISTS anime_watchlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      mal_id INTEGER NOT NULL,
      title TEXT NOT NULL,
      airing_date TEXT,
      notified INTEGER DEFAULT 0,
      created_at INTEGER DEFAULT (unixepoch())
    );

    CREATE TABLE IF NOT EXISTS anime_cache (
      mal_id INTEGER PRIMARY KEY,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch())
    );

    -- Movies/TV
    CREATE TABLE IF NOT EXISTS movie_watchlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      tmdb_id INTEGER NOT NULL,
      title TEXT NOT NULL,
      media_type TEXT NOT NULL,
      release_date TEXT,
      notified INTEGER DEFAULT 0,
      created_at INTEGER DEFAULT (unixepoch())
    );

    CREATE TABLE IF NOT EXISTS movie_cache (
      tmdb_id INTEGER NOT NULL,
      media_type TEXT NOT NULL,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch()),
      PRIMARY KEY (tmdb_id, media_type)
    );

    -- Countdowns
    CREATE TABLE IF NOT EXISTS countdowns (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      label TEXT NOT NULL,
      target_date TEXT NOT NULL,
      public INTEGER DEFAULT 1,
      completed INTEGER DEFAULT 0,
      created_at INTEGER DEFAULT (unixepoch())
    );

    -- Presence
    CREATE TABLE IF NOT EXISTS presence (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      status TEXT NOT NULL DEFAULT 'online',
      status_message TEXT,
      updated_at INTEGER DEFAULT (unixepoch()),
      PRIMARY KEY (user_id, room_id)
    );

    -- Markov corpus
    CREATE TABLE IF NOT EXISTS markov_corpus (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      message TEXT NOT NULL,
      added_at INTEGER DEFAULT (unixepoch())
    );
    CREATE INDEX IF NOT EXISTS idx_markov_user_room ON markov_corpus (user_id, room_id);

    -- Retro game cache (GiantBomb)
    CREATE TABLE IF NOT EXISTS retro_cache (
      search_term TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch())
    );

    -- Urban Dictionary cache
    CREATE TABLE IF NOT EXISTS urban_cache (
      term TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      cached_at INTEGER DEFAULT (unixepoch())
    );

    -- Room message milestones
    CREATE TABLE IF NOT EXISTS room_milestones (
      room_id TEXT PRIMARY KEY,
      total_messages INTEGER NOT NULL DEFAULT 0,
      last_milestone INTEGER NOT NULL DEFAULT 0
    );

    -- Reaction tracking
    CREATE TABLE IF NOT EXISTS reaction_log (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      giver_id TEXT NOT NULL,
      receiver_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      emoji TEXT NOT NULL,
      event_id TEXT NOT NULL,
      created_at INTEGER DEFAULT (unixepoch())
    );
    CREATE INDEX IF NOT EXISTS idx_reaction_room ON reaction_log (room_id);

    -- Sentiment aggregates (rolled up from llm_classifications)
    CREATE TABLE IF NOT EXISTS sentiment_stats (
      user_id TEXT NOT NULL,
      room_id TEXT NOT NULL,
      neutral INTEGER DEFAULT 0,
      happy INTEGER DEFAULT 0,
      sad INTEGER DEFAULT 0,
      angry INTEGER DEFAULT 0,
      excited INTEGER DEFAULT 0,
      funny INTEGER DEFAULT 0,
      love INTEGER DEFAULT 0,
      scared INTEGER DEFAULT 0,
      total_classified INTEGER DEFAULT 0,
      PRIMARY KEY (user_id, room_id)
    );

    -- Daily prefetch cache (API data fetched early, posted later)
    CREATE TABLE IF NOT EXISTS daily_prefetch (
      job_name TEXT NOT NULL,
      date TEXT NOT NULL,
      data TEXT NOT NULL,
      fetched_at INTEGER DEFAULT (unixepoch()),
      PRIMARY KEY (job_name, date)
    );

    -- URL preview cache
    CREATE TABLE IF NOT EXISTS url_cache (
      url TEXT PRIMARY KEY,
      title TEXT,
      description TEXT,
      cached_at INTEGER DEFAULT (unixepoch())
    );
  `);
}

function runMigrations(): void {
  const cols = db
    .prepare(`PRAGMA table_info(llm_classifications)`)
    .all() as { name: string }[];
  if (cols.length > 0) {
    if (!cols.find((c) => c.name === "sentiment")) {
      db.exec(`ALTER TABLE llm_classifications ADD COLUMN sentiment TEXT DEFAULT 'neutral'`);
      logger.info("Migration: added sentiment column to llm_classifications");
    }
    if (!cols.find((c) => c.name === "gratitude")) {
      db.exec(`ALTER TABLE llm_classifications ADD COLUMN gratitude INTEGER DEFAULT 0`);
      db.exec(`ALTER TABLE llm_classifications ADD COLUMN gratitude_toward TEXT`);
      logger.info("Migration: added gratitude columns to llm_classifications");
    }
  }

  // Backfill sentiment_stats from existing llm_classifications
  const sentimentCount = db.prepare(`SELECT COUNT(*) as c FROM sentiment_stats`).get() as { c: number };
  const classCount = db.prepare(`SELECT COUNT(*) as c FROM llm_classifications`).get() as { c: number };
  if (sentimentCount.c === 0 && classCount.c > 0) {
    db.exec(`
      INSERT INTO sentiment_stats (user_id, room_id, neutral, happy, sad, angry, excited, funny, love, scared, total_classified)
      SELECT user_id, room_id,
        SUM(CASE WHEN sentiment = 'neutral' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'happy' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'sad' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'angry' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'excited' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'funny' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'love' THEN 1 ELSE 0 END),
        SUM(CASE WHEN sentiment = 'scared' THEN 1 ELSE 0 END),
        COUNT(*)
      FROM llm_classifications GROUP BY user_id, room_id
    `);
    logger.info("Migration: backfilled sentiment_stats from llm_classifications");
  }
}

function seedSchedulerDefaults(): void {
  const insert = db.prepare(`
    INSERT OR IGNORE INTO scheduler_config (job_name, hour, minute, enabled)
    VALUES (?, ?, ?, ?)
  `);

  const holidaysHour = parseInt(process.env.SCHEDULE_HOLIDAYS_HOUR ?? "7", 10);
  const holidaysMinute = parseInt(process.env.SCHEDULE_HOLIDAYS_MINUTE ?? "0", 10);
  const releasesHour = parseInt(process.env.SCHEDULE_RELEASES_HOUR ?? "19", 10);
  const releasesMinute = parseInt(process.env.SCHEDULE_RELEASES_MINUTE ?? "0", 10);

  insert.run("prefetch", 0, 5, 1);
  insert.run("maintenance", 0, 15, 1);
  insert.run("holidays", holidaysHour, holidaysMinute, 1);
  insert.run("releases", releasesHour, releasesMinute, 1);
  insert.run("wotd", 8, 0, 1);
  insert.run("birthday_check", 7, 5, 1);
  insert.run("anime_releases", 19, 30, 1);
  insert.run("movie_releases", 20, 0, 1);
  insert.run("concert_digest", 10, 0, 1);
}
