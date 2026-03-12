package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	mu       sync.Mutex
	globalDB *sql.DB
)

// Init opens (or creates) the SQLite database and runs migrations.
func Init(dataDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if globalDB != nil {
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "gogobee.db")
	d, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	d.SetMaxOpenConns(1) // SQLite is single-writer

	if err := runMigrations(d); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	globalDB = d
	slog.Info("database initialized", "path", dbPath)
	return nil
}

// Get returns the global database handle. Panics if Init was not called.
func Get() *sql.DB {
	if globalDB == nil {
		panic("db.Get() called before db.Init()")
	}
	return globalDB
}

func runMigrations(d *sql.DB) error {
	_, err := d.Exec(schema)
	return err
}

// JobCompleted checks if a scheduled job has already completed for the given date key.
// Use date "2006-01-02" for daily jobs, or "2006-W01" style for weekly jobs.
func JobCompleted(jobName, dateKey string) bool {
	var completed int
	err := Get().QueryRow(
		`SELECT completed FROM daily_prefetch WHERE job_name = ? AND date = ?`,
		jobName, dateKey,
	).Scan(&completed)
	return err == nil && completed == 1
}

// MarkJobCompleted marks a scheduled job as completed for the given date key.
func MarkJobCompleted(jobName, dateKey string) {
	_, err := Get().Exec(
		`INSERT INTO daily_prefetch (job_name, date, completed) VALUES (?, ?, 1)
		 ON CONFLICT(job_name, date) DO UPDATE SET completed = 1`,
		jobName, dateKey,
	)
	if err != nil {
		slog.Error("mark job completed", "job", jobName, "date", dateKey, "err", err)
	}
}

// CacheGet returns cached data for the given key if it exists and is within ttlSeconds.
// Returns empty string if not cached or expired.
func CacheGet(key string, ttlSeconds int) string {
	d := Get()
	var data string
	err := d.QueryRow(
		`SELECT data FROM api_cache WHERE cache_key = ? AND cached_at > unixepoch() - ?`,
		key, ttlSeconds,
	).Scan(&data)
	if err != nil {
		return ""
	}
	return data
}

// CacheSet stores data in the generic API cache.
func CacheSet(key, data string) {
	d := Get()
	_, err := d.Exec(
		`INSERT INTO api_cache (cache_key, data, cached_at) VALUES (?, ?, unixepoch())
		 ON CONFLICT(cache_key) DO UPDATE SET data = ?, cached_at = unixepoch()`,
		key, data, data,
	)
	if err != nil {
		slog.Error("cache set", "key", key, "err", err)
	}
}

// RunMaintenance purges stale data from cache tables, old rate limits,
// expired logs, and runs SQLite optimization. Intended to run daily.
func RunMaintenance() {
	d := Get()
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	cutoff7d := now.AddDate(0, 0, -7).Unix()
	cutoff30d := now.AddDate(0, 0, -30).Unix()
	date30d := now.AddDate(0, 0, -30).Format("2006-01-02")
	date90d := now.AddDate(0, 0, -90).Format("2006-01-02")

	queries := []struct {
		label string
		sql   string
		args  []interface{}
	}{
		// Cache tables — purge entries older than their effective TTL
		{"api_cache", `DELETE FROM api_cache WHERE cached_at < ?`, []interface{}{cutoff7d}},
		{"releases_cache", `DELETE FROM releases_cache WHERE cached_at < ?`, []interface{}{cutoff7d}},
		{"hltb_cache", `DELETE FROM hltb_cache WHERE cached_at < ?`, []interface{}{cutoff7d}},
		{"stocks_cache", `DELETE FROM stocks_cache WHERE cached_at < ?`, []interface{}{cutoff7d}},
		{"concerts_cache", `DELETE FROM concerts_cache WHERE cached_at < ?`, []interface{}{cutoff7d}},
		{"anime_cache", `DELETE FROM anime_cache WHERE cached_at < ?`, []interface{}{cutoff30d}},
		{"movie_cache", `DELETE FROM movie_cache WHERE cached_at < ?`, []interface{}{cutoff30d}},
		{"retro_cache", `DELETE FROM retro_cache WHERE cached_at < ?`, []interface{}{cutoff30d}},
		{"urban_cache", `DELETE FROM urban_cache WHERE cached_at < ?`, []interface{}{cutoff30d}},
		{"url_cache", `DELETE FROM url_cache WHERE cached_at < ?`, []interface{}{cutoff30d}},

		// Rate limits — purge entries older than today
		{"rate_limits", `DELETE FROM rate_limits WHERE date < ?`, []interface{}{today}},

		// Daily prefetch log — keep 30 days
		{"daily_prefetch", `DELETE FROM daily_prefetch WHERE date < ?`, []interface{}{date30d}},

		// Holiday and WOTD logs — keep 90 days
		{"holidays_log", `DELETE FROM holidays_log WHERE date < ?`, []interface{}{date90d}},
		{"wotd_log", `DELETE FROM wotd_log WHERE date < ?`, []interface{}{date90d}},
		{"wotd_usage", `DELETE FROM wotd_usage WHERE date < ?`, []interface{}{date90d}},

		// LLM classifications — keep 30 days
		{"llm_classifications", `DELETE FROM llm_classifications WHERE timestamp < ?`, []interface{}{cutoff30d}},

		// Daily activity older than 1 year
		{"daily_activity", `DELETE FROM daily_activity WHERE date < ?`, []interface{}{now.AddDate(-1, 0, 0).Format("2006-01-02")}},
	}

	totalDeleted := int64(0)
	for _, q := range queries {
		res, err := d.Exec(q.sql, q.args...)
		if err != nil {
			slog.Error("maintenance: "+q.label, "err", err)
			continue
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			slog.Info("maintenance: purged", "table", q.label, "rows", n)
			totalDeleted += n
		}
	}

	// SQLite optimization
	if _, err := d.Exec(`PRAGMA optimize`); err != nil {
		slog.Error("maintenance: pragma optimize", "err", err)
	}

	slog.Info("maintenance: complete", "total_purged", totalDeleted)
}

const schema = `
-- Users & XP
CREATE TABLE IF NOT EXISTS users (
	user_id TEXT PRIMARY KEY,
	display_name TEXT DEFAULT '',
	xp INTEGER DEFAULT 0,
	level INTEGER DEFAULT 0,
	last_xp_at INTEGER DEFAULT 0,
	created_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS user_stats (
	user_id TEXT PRIMARY KEY,
	total_messages INTEGER DEFAULT 0,
	total_words INTEGER DEFAULT 0,
	total_chars INTEGER DEFAULT 0,
	total_links INTEGER DEFAULT 0,
	total_images INTEGER DEFAULT 0,
	total_questions INTEGER DEFAULT 0,
	total_exclamations INTEGER DEFAULT 0,
	total_emojis INTEGER DEFAULT 0,
	night_messages INTEGER DEFAULT 0,
	morning_messages INTEGER DEFAULT 0,
	updated_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS xp_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	amount INTEGER NOT NULL,
	reason TEXT DEFAULT '',
	created_at INTEGER DEFAULT (unixepoch())
);

-- Reputation
CREATE TABLE IF NOT EXISTS rep_cooldowns (
	giver TEXT NOT NULL,
	receiver TEXT NOT NULL,
	last_given INTEGER DEFAULT (unixepoch()),
	PRIMARY KEY (giver, receiver)
);

-- Reminders
CREATE TABLE IF NOT EXISTS reminders (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	message TEXT NOT NULL,
	fire_at INTEGER NOT NULL,
	fired INTEGER DEFAULT 0,
	created_at INTEGER DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS idx_reminders_fire ON reminders(fired, fire_at);

-- Daily activity / streaks
CREATE TABLE IF NOT EXISTS daily_activity (
	user_id TEXT NOT NULL,
	date TEXT NOT NULL,
	message_count INTEGER DEFAULT 0,
	PRIMARY KEY (user_id, date)
);

CREATE TABLE IF NOT EXISTS daily_first (
	room_id TEXT NOT NULL,
	date TEXT NOT NULL,
	user_id TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	PRIMARY KEY (room_id, date)
);

-- Word of the Day
CREATE TABLE IF NOT EXISTS wotd_log (
	date TEXT PRIMARY KEY,
	word TEXT NOT NULL,
	definition TEXT NOT NULL,
	part_of_speech TEXT DEFAULT '',
	example TEXT DEFAULT '',
	posted INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS wotd_usage (
	user_id TEXT NOT NULL,
	date TEXT NOT NULL,
	count INTEGER DEFAULT 0,
	rewarded INTEGER DEFAULT 0,
	PRIMARY KEY (user_id, date)
);

-- Holidays
CREATE TABLE IF NOT EXISTS holidays_log (
	date TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	posted INTEGER DEFAULT 0
);

-- Game releases
CREATE TABLE IF NOT EXISTS releases_cache (
	cache_key TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS release_watchlist (
	user_id TEXT NOT NULL,
	game_name TEXT NOT NULL,
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, game_name)
);

-- HLTB cache
CREATE TABLE IF NOT EXISTS hltb_cache (
	game_name TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

-- Achievements
CREATE TABLE IF NOT EXISTS achievements (
	user_id TEXT NOT NULL,
	achievement_id TEXT NOT NULL,
	unlocked_at INTEGER DEFAULT (unixepoch()),
	PRIMARY KEY (user_id, achievement_id)
);

-- Quotes (encrypted at rest)
CREATE TABLE IF NOT EXISTS quotes (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id       TEXT NOT NULL,
	submitted_by  TEXT NOT NULL,
	saved_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	content_hmac  TEXT NOT NULL UNIQUE,
	quote_text    BLOB NOT NULL,
	attributed_to BLOB NOT NULL,
	context       BLOB
);

-- Now Playing
CREATE TABLE IF NOT EXISTS now_playing (
	user_id TEXT PRIMARY KEY,
	track TEXT NOT NULL,
	updated_at INTEGER DEFAULT (unixepoch())
);

-- Backlog
CREATE TABLE IF NOT EXISTS backlog (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	item TEXT NOT NULL,
	done INTEGER DEFAULT 0,
	created_at INTEGER DEFAULT (unixepoch())
);

-- Predictions (stub/future)
CREATE TABLE IF NOT EXISTS predictions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	prediction TEXT NOT NULL,
	outcome TEXT DEFAULT '',
	resolved INTEGER DEFAULT 0,
	created_at INTEGER DEFAULT (unixepoch())
);

-- Keyword watches
CREATE TABLE IF NOT EXISTS keyword_watches (
	user_id TEXT NOT NULL,
	keyword TEXT NOT NULL,
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, keyword)
);

-- Scheduler config
CREATE TABLE IF NOT EXISTS scheduler_config (
	job_name TEXT PRIMARY KEY,
	enabled INTEGER DEFAULT 1,
	cron_expr TEXT NOT NULL,
	last_run TEXT DEFAULT ''
);

-- Shade (stub)
CREATE TABLE IF NOT EXISTS shade_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	target_user TEXT NOT NULL,
	message TEXT NOT NULL,
	room_id TEXT NOT NULL,
	created_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS shade_optout (
	user_id TEXT PRIMARY KEY
);

-- Birthdays
CREATE TABLE IF NOT EXISTS birthdays (
	user_id TEXT PRIMARY KEY,
	month INTEGER NOT NULL,
	day INTEGER NOT NULL,
	year INTEGER DEFAULT 0,
	timezone TEXT DEFAULT ''
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
	category INTEGER DEFAULT 0,
	difficulty TEXT DEFAULT 'medium',
	question TEXT NOT NULL,
	correct_answer TEXT NOT NULL,
	incorrect_answers TEXT NOT NULL,
	question_type TEXT DEFAULT 'multiple',
	thread_id TEXT DEFAULT '',
	started_at INTEGER DEFAULT (unixepoch()),
	ended INTEGER DEFAULT 0,
	winner_id TEXT DEFAULT '',
	winner_time_ms INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS trivia_scores (
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	correct INTEGER DEFAULT 0,
	wrong INTEGER DEFAULT 0,
	total_score INTEGER DEFAULT 0,
	fastest_ms INTEGER DEFAULT 0,
	PRIMARY KEY (user_id, room_id)
);

-- LLM classifications
CREATE TABLE IF NOT EXISTS llm_classifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	message_text TEXT NOT NULL,
	sentiment TEXT DEFAULT '',
	sentiment_score REAL DEFAULT 0,
	topics TEXT DEFAULT '[]',
	profanity INTEGER DEFAULT 0,
	profanity_severity INTEGER DEFAULT 0,
	insult_target TEXT DEFAULT '',
	wotd_used INTEGER DEFAULT 0,
	gratitude_target TEXT DEFAULT '',
	created_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS potty_mouth (
	user_id TEXT PRIMARY KEY,
	count INTEGER DEFAULT 0,
	mild INTEGER DEFAULT 0,
	moderate INTEGER DEFAULT 0,
	scorching INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS insult_log (
	user_id TEXT PRIMARY KEY,
	times_insulted INTEGER DEFAULT 0,
	times_insulting INTEGER DEFAULT 0
);

-- Stocks
CREATE TABLE IF NOT EXISTS stocks_cache (
	ticker TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS stock_watchlist (
	user_id TEXT NOT NULL,
	ticker TEXT NOT NULL,
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, ticker)
);

-- Command usage
CREATE TABLE IF NOT EXISTS command_usage (
	command TEXT NOT NULL,
	user_id TEXT NOT NULL,
	count INTEGER DEFAULT 0,
	PRIMARY KEY (command, user_id)
);

-- Concerts
CREATE TABLE IF NOT EXISTS concerts_cache (
	artist TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS concert_watchlist (
	user_id TEXT NOT NULL,
	artist TEXT NOT NULL,
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, artist)
);

-- Anime
CREATE TABLE IF NOT EXISTS anime_watchlist (
	user_id TEXT NOT NULL,
	mal_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, mal_id)
);

CREATE TABLE IF NOT EXISTS anime_cache (
	mal_id INTEGER PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

-- Movies
CREATE TABLE IF NOT EXISTS movie_watchlist (
	user_id TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	media_type TEXT DEFAULT 'movie',
	room_id TEXT NOT NULL,
	PRIMARY KEY (user_id, tmdb_id)
);

CREATE TABLE IF NOT EXISTS movie_cache (
	tmdb_id INTEGER PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
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
	user_id TEXT PRIMARY KEY,
	status TEXT DEFAULT 'online',
	message TEXT DEFAULT '',
	updated_at INTEGER DEFAULT (unixepoch())
);

-- Markov
CREATE TABLE IF NOT EXISTS markov_corpus (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id TEXT NOT NULL,
	text TEXT NOT NULL,
	created_at INTEGER DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS idx_markov_user ON markov_corpus(user_id);

-- Retro/game lookup cache
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

-- Room milestones
CREATE TABLE IF NOT EXISTS room_milestones (
	room_id TEXT PRIMARY KEY,
	total_messages INTEGER DEFAULT 0,
	last_milestone INTEGER DEFAULT 0
);

-- Reaction log
CREATE TABLE IF NOT EXISTS reaction_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	sender TEXT NOT NULL,
	target_user TEXT NOT NULL,
	emoji TEXT NOT NULL,
	created_at INTEGER DEFAULT (unixepoch())
);

-- Sentiment stats (aggregated)
CREATE TABLE IF NOT EXISTS sentiment_stats (
	user_id TEXT PRIMARY KEY,
	positive INTEGER DEFAULT 0,
	negative INTEGER DEFAULT 0,
	neutral INTEGER DEFAULT 0,
	excited INTEGER DEFAULT 0,
	sarcastic INTEGER DEFAULT 0,
	frustrated INTEGER DEFAULT 0,
	curious INTEGER DEFAULT 0,
	grateful INTEGER DEFAULT 0,
	humorous INTEGER DEFAULT 0,
	supportive INTEGER DEFAULT 0,
	total_score REAL DEFAULT 0
);

-- Daily prefetch tracking
CREATE TABLE IF NOT EXISTS daily_prefetch (
	job_name TEXT NOT NULL,
	date TEXT NOT NULL,
	completed INTEGER DEFAULT 0,
	PRIMARY KEY (job_name, date)
);

-- URL preview cache
CREATE TABLE IF NOT EXISTS url_cache (
	url TEXT PRIMARY KEY,
	title TEXT DEFAULT '',
	description TEXT DEFAULT '',
	cached_at INTEGER DEFAULT (unixepoch())
);

-- Rate limits
CREATE TABLE IF NOT EXISTS rate_limits (
	user_id TEXT NOT NULL,
	action TEXT NOT NULL,
	date TEXT NOT NULL,
	count INTEGER DEFAULT 0,
	PRIMARY KEY (user_id, action, date)
);

-- Generic API response cache
CREATE TABLE IF NOT EXISTS api_cache (
	cache_key TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	cached_at INTEGER DEFAULT (unixepoch())
);

-- Moderation: strikes
CREATE TABLE IF NOT EXISTS mod_strikes (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      TEXT NOT NULL,
	room_id      TEXT NOT NULL,
	issued_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at   DATETIME NOT NULL,
	reason       TEXT NOT NULL,
	issued_by    TEXT NOT NULL,
	active       BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_mod_strikes_user ON mod_strikes(user_id, issued_at);

-- Moderation: action log
CREATE TABLE IF NOT EXISTS mod_actions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      TEXT NOT NULL,
	room_id      TEXT NOT NULL,
	action       TEXT NOT NULL,
	reason       TEXT,
	taken_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	taken_by     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mod_actions_user ON mod_actions(user_id, taken_at);

-- Space groups (rooms with overlapping membership)
CREATE TABLE IF NOT EXISTS space_groups (
	room_id    TEXT PRIMARY KEY,
	group_id   INTEGER NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

`

// SeedSchedulerDefaults inserts default scheduler jobs if they don't exist.
func SeedSchedulerDefaults(d *sql.DB) error {
	defaults := []struct {
		name string
		cron string
	}{
		{"prefetch", "5 0 * * *"},       // 00:05 daily
		{"maintenance", "0 3 * * *"},    // 03:00 daily
		{"wotd", "0 8 * * *"},           // 08:00 daily
		{"holidays", "0 7 * * *"},       // 07:00 daily
		{"releases", "0 9 * * 1"},       // 09:00 Monday
		{"birthday_check", "0 6 * * *"}, // 06:00 daily
		{"anime_releases", "0 10 * * *"},// 10:00 daily
		{"movie_releases", "0 11 * * *"},// 11:00 daily
		{"concert_digest", "0 12 * * 0"},// 12:00 Sunday
	}

	stmt, err := d.Prepare(`INSERT OR IGNORE INTO scheduler_config (job_name, cron_expr) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, def := range defaults {
		if _, err := stmt.Exec(def.name, def.cron); err != nil {
			return fmt.Errorf("seed scheduler %s: %w", def.name, err)
		}
	}
	return nil
}
