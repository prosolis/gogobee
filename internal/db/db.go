package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	mu       sync.Mutex
	globalDB *sql.DB
	dataPath string
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

	if err := snapshotPreDnD(d); err != nil {
		slog.Error("db: pre-D&D snapshot failed (non-fatal)", "err", err)
	}

	globalDB = d
	dataPath = dataDir
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

// Close closes the database connection. Call on shutdown.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if globalDB != nil {
		if err := globalDB.Close(); err != nil {
			slog.Error("db: close failed", "err", err)
		}
		globalDB = nil
	}
}

func runMigrations(d *sql.DB) error {
	if _, err := d.Exec(schema); err != nil {
		return err
	}
	// Column migrations — ALTER TABLE ADD COLUMN is a no-op if it already
	// exists in SQLite (we just swallow "duplicate column name" errors).
	columnMigrations := []string{
		`ALTER TABLE adventure_characters ADD COLUMN holiday_action_taken INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE wordle_puzzles ADD COLUMN category TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_equipment ADD COLUMN arena_tier INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_equipment ADD COLUMN arena_set TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_equipment ADD COLUMN masterwork INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN death_reprieve_last DATETIME`,
		`ALTER TABLE adventure_equipment ADD COLUMN skill_source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN masterwork_drops_received INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_inventory ADD COLUMN slot TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_inventory ADD COLUMN skill_source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE user_stats ADD COLUMN fancy_words INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN rival_pool INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN rival_unlocked_notified INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN babysit_active INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN babysit_expires_at DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN babysit_skill_focus TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN hospital_visits INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN robbie_visit_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE wordle_stats ADD COLUMN total_earned INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN last_death_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN combat_actions_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN harvest_actions_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN last_pardon_used DATETIME`,
		`ALTER TABLE arena_runs ADD COLUMN tier_earnings INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE arena_runs ADD COLUMN xp_accumulated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_last_seen DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN arina_last_seen DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_buff_expires DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_debuff_expires DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN arina_buff_expires DATETIME`,
		`ALTER TABLE adventure_characters ADD COLUMN npc_msg_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN npc_msg_count_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_roll_target INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN arina_roll_target INTEGER NOT NULL DEFAULT 0`,
		// Housing
		`ALTER TABLE adventure_characters ADD COLUMN house_tier INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN house_loan_balance INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN house_loan_frozen INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN house_missed_payments INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN house_autopay INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN house_current_rate REAL NOT NULL DEFAULT 0`,
		// Pets
		`ALTER TABLE adventure_characters ADD COLUMN pet_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_xp INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_level INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_armor_tier INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_chased_away INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_reactivated INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_arrived INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_encounter_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN misty_donated_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN thom_animal_line_fired INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_supply_shop_unlocked INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_level10_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN pet_morning_defense INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN auto_babysit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN streak_decayed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE coop_dungeon_runs ADD COLUMN last_resolved_day INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE coop_dungeon_members ADD COLUMN member_payout INTEGER`,
		`ALTER TABLE coop_dungeon_runs ADD COLUMN invite_post_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE coop_dungeon_gifts ADD COLUMN expires_at DATETIME`,
		`ALTER TABLE coop_dungeon_gifts ADD COLUMN applied_at DATETIME`,
		`ALTER TABLE coop_dungeon_gifts ADD COLUMN stack_lead_id INTEGER`,
		`ALTER TABLE adventure_characters ADD COLUMN crafts_succeeded INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_characters ADD COLUMN death_source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN death_location TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN auto_babysit_focus TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_characters ADD COLUMN treasures_locked INTEGER NOT NULL DEFAULT 0`,
		// D&D layer (Phase 1) — additive columns on existing equipment/inventory tables
		`ALTER TABLE adventure_equipment ADD COLUMN dnd_rarity TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_equipment ADD COLUMN dnd_stat_bonus_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_equipment ADD COLUMN dnd_attuned INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE adventure_inventory ADD COLUMN dnd_rarity TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE adventure_inventory ADD COLUMN dnd_stat_bonus_json TEXT NOT NULL DEFAULT ''`,
		// Phase 2 cleanup: auto-migrated chars (created on first combat for
		// players who never ran !setup). !setup can freely overwrite these.
		`ALTER TABLE dnd_character ADD COLUMN auto_migrated INTEGER NOT NULL DEFAULT 0`,
		// Phase 6 rest mechanics
		`ALTER TABLE dnd_character ADD COLUMN last_short_rest_at DATETIME`,
		`ALTER TABLE dnd_character ADD COLUMN last_long_rest_at DATETIME`,
		// Phase 6 active abilities: armed for next combat
		`ALTER TABLE dnd_character ADD COLUMN armed_ability TEXT NOT NULL DEFAULT ''`,
		// Audit fix F: prevent onboarding DM from re-firing after !setup cancel
		`ALTER TABLE dnd_character ADD COLUMN onboarding_sent INTEGER NOT NULL DEFAULT 0`,
		// Phase 9 — spell system. pending_cast holds a JSON blob describing
		// the queued spell (id, slot level, target, rolled effect args) that
		// fires on the next one-shot combat. concentration_* track the
		// currently-active concentration spell for buff persistence.
		`ALTER TABLE dnd_character ADD COLUMN pending_cast TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dnd_character ADD COLUMN concentration_spell TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dnd_character ADD COLUMN concentration_expires_at DATETIME`,
		// Phase 10 — subclass system. Subclass id is set at L5 via !subclass.
		// last_subclass_respec_at gates the 30-day cooldown for changing it
		// (separate from last_respec_at, which gates the full character wipe).
		`ALTER TABLE dnd_character ADD COLUMN subclass TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dnd_character ADD COLUMN last_subclass_respec_at DATETIME`,
		// Phase 10 SUB2a — exhaustion levels. Berserker Frenzy increments
		// this after each rage'd combat. Cleared on long rest. Reused by
		// other classes once exhaustion-inducing mechanics arrive in SUB3+.
		`ALTER TABLE dnd_character ADD COLUMN exhaustion INTEGER NOT NULL DEFAULT 0`,
		// Standalone-zone-run harvest: stores a per-room HarvestNode map for
		// !zone enter sessions that aren't tied to an expedition. Expedition
		// runs continue to use expedition.region_state for the same data.
		`ALTER TABLE dnd_zone_run ADD COLUMN harvest_nodes_json TEXT NOT NULL DEFAULT '{}'`,
		// Adv 2.0 Phase L4f-prep — DisplayName migration off AdvCharacter.
		// player_meta gets the canonical column; AdvCharacter.display_name
		// keeps dual-writing during soak (gogobee_legacy_migration.md §7).
		`ALTER TABLE player_meta ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		// Adv 2.0 Phase L4a — Hospital migration off AdvCharacter.
		// HospitalVisits moves to player_meta; AdvCharacter.hospital_visits
		// keeps dual-writing during soak (gogobee_legacy_migration.md §6.1).
		`ALTER TABLE player_meta ADD COLUMN hospital_visits INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L4b — Rival migration off AdvCharacter.
		// RivalPool / RivalUnlockedNotified move to player_meta; AdvCharacter
		// columns keep dual-writing during soak (gogobee_legacy_migration.md §6.2).
		`ALTER TABLE player_meta ADD COLUMN rival_pool INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN rival_unlocked_notified INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L4c — Masterwork migration off AdvCharacter.
		// MasterworkDropsReceived moves to player_meta; AdvCharacter column
		// keeps dual-writing during soak (gogobee_legacy_migration.md §6.3).
		`ALTER TABLE player_meta ADD COLUMN masterwork_drops_received INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L4d — Pets migration off AdvCharacter.
		// PetType / PetName / PetXP / PetLevel / PetArmorTier and the four
		// pet flags (arrived, chased_away, reactivated, morning_defense)
		// move to player_meta; AdvCharacter columns keep dual-writing during
		// soak (gogobee_legacy_migration.md §6.4).
		`ALTER TABLE player_meta ADD COLUMN pet_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN pet_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN pet_xp INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN pet_level INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN pet_armor_tier INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN pet_flags_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE player_meta ADD COLUMN pet_supply_shop_unlocked INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN pet_level_10_date TEXT NOT NULL DEFAULT ''`,
		// Adv 2.0 Phase L4e — Housing & mortgage migration off AdvCharacter.
		// HouseTier / HouseLoanBalance / HouseLoanFrozen / HouseMissedPayments
		// / HouseAutopay / HouseCurrentRate move to player_meta; AdvCharacter
		// columns keep dual-writing during soak (gogobee_legacy_migration.md §6.5).
		`ALTER TABLE player_meta ADD COLUMN house_tier INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN house_loan_balance INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN house_loan_frozen INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN house_missed_payments INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN house_autopay INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN house_current_rate REAL NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L5a — Skills migration off AdvCharacter.
		// CombatLevel / MiningSkill / ForagingSkill / FishingSkill and their
		// XP counters move to player_meta; AdvCharacter columns keep
		// dual-writing during soak (gogobee_legacy_migration.md §7.3 L5a).
		// CombatLevel/CombatXP are transitional — dropped after L5g's
		// DnDCharacter mass-backfill retires the dndLevelFromCombatLevel
		// fallback.
		`ALTER TABLE player_meta ADD COLUMN combat_level INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN combat_xp INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN mining_skill INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN mining_xp INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN foraging_skill INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN foraging_xp INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN fishing_skill INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN fishing_xp INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L5b — Babysit state migration off AdvCharacter.
		// BabysitActive / BabysitExpiresAt / BabysitSkillFocus / AutoBabysit /
		// AutoBabysitFocus move to player_meta; AdvCharacter columns keep
		// dual-writing during soak (gogobee_legacy_migration.md §7.3 L5b).
		`ALTER TABLE player_meta ADD COLUMN babysit_active INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN babysit_expires_at DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN babysit_skill_focus TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN auto_babysit INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN auto_babysit_focus TEXT NOT NULL DEFAULT ''`,
		// Adv 2.0 Phase L5c — NPC counters & debuff timestamps off AdvCharacter.
		// Misty/Arina/Robbie/Thom counters and buff/debuff expiry times move
		// to player_meta (gogobee_legacy_migration.md §7.3 L5c). Hidden
		// discovery mechanics — never surface in player-facing output.
		`ALTER TABLE player_meta ADD COLUMN misty_last_seen DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN arina_last_seen DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN misty_buff_expires DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN misty_debuff_expires DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN arina_buff_expires DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN npc_msg_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN npc_msg_count_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN misty_roll_target INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN arina_roll_target INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN misty_encounter_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN misty_donated_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN thom_animal_line_fired INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN robbie_visit_count INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase L5d — Streak/action/lifecycle migration off AdvCharacter.
		// Streak (current/best/decayed/last_action_date), per-day action flags
		// (action_taken_today/holiday_action_taken/combat_actions_used/
		// harvest_actions_used), and lifecycle timestamps (created_at /
		// last_active_at) move to player_meta (gogobee_legacy_migration.md
		// §7.3 L5d).
		`ALTER TABLE player_meta ADD COLUMN current_streak INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN best_streak INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN last_action_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN streak_decayed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN action_taken_today INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN holiday_action_taken INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN combat_actions_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN harvest_actions_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN created_at DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN last_active_at DATETIME`,
		// Adv 2.0 Phase L5e — Death state migration off AdvCharacter.
		// Alive / DeadUntil / DeathReprieveLast / LastDeathDate /
		// LastPardonUsed / GrudgeLocation / DeathSource / DeathLocation move
		// to player_meta. Dual-write strategy switches to inside
		// saveAdvCharacter (death state mutates at ~50 save sites; per-site
		// upserts would be too noisy — gogobee_legacy_migration.md §7.3
		// L5e). `Alive` defaults to 1 since legacy created characters are
		// alive, and a never-migrated row should fall through to the legacy
		// table via loadDeathState.
		`ALTER TABLE player_meta ADD COLUMN alive INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE player_meta ADD COLUMN dead_until DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN death_reprieve_last DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN last_death_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN last_pardon_used DATETIME`,
		`ALTER TABLE player_meta ADD COLUMN grudge_location TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN death_source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN death_location TEXT NOT NULL DEFAULT ''`,
		// Adv 2.0 Phase L5f — Misc fields (Title, TreasuresLocked,
		// CraftsSucceeded) off AdvCharacter (gogobee_legacy_migration.md
		// §7.3 L5f). Dual-write inside saveAdvCharacter (same as L5e).
		`ALTER TABLE player_meta ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE player_meta ADD COLUMN treasures_locked INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE player_meta ADD COLUMN crafts_succeeded INTEGER NOT NULL DEFAULT 0`,
		// Adv 2.0 Phase G1 — branching zone graph run-state columns.
		// current_node / visited_nodes / node_choices live alongside the
		// legacy current_room / room_seq_json during the migration; the
		// linear columns retire in G9 (gogobee_branching_zones_plan.md §2).
		`ALTER TABLE dnd_zone_run ADD COLUMN current_node TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE dnd_zone_run ADD COLUMN visited_nodes TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE dnd_zone_run ADD COLUMN node_choices TEXT NOT NULL DEFAULT '{}'`,
		// 2026-05-10 immersion pass: short rest = hit-dice charges (1/level),
		// long rest restores them. resting_until gates !zone enter and
		// !expedition start so a freshly-rested character can't immediately
		// jump back into combat — they're actually resting for the duration.
		`ALTER TABLE dnd_character ADD COLUMN short_rest_charges INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dnd_character ADD COLUMN resting_until DATETIME`,
		// Phase 12 E7 (expedition autopilot Phase 3) — ambient ticker timestamp.
		// Real-time between-day events fire at most once per ambientCooldown
		// while an expedition is active; idempotent CAS on this column.
		`ALTER TABLE dnd_expedition ADD COLUMN last_ambient_at DATETIME`,
	}
	for _, stmt := range columnMigrations {
		if _, err := d.Exec(stmt); err != nil {
			msg := err.Error()
			// "duplicate column name" means it already exists — safe to ignore.
			// "no such table: adventure_characters" is expected after the
			// L5 close-out purge (GOGOBEE_LEGACY_PURGE=1).
			if strings.Contains(msg, "duplicate column") {
				continue
			}
			if strings.Contains(msg, "no such table: adventure_characters") {
				continue
			}
			return fmt.Errorf("migration %q: %w", stmt, err)
		}
	}
	return nil
}

// snapshotPreDnD copies the current adventure_characters rows into
// adventure_characters_pre_dnd as a one-shot rollback safety net for the
// D&D-layer migration. Idempotent: only inserts rows for user_ids that
// aren't already snapshotted. Skips silently once the source table has
// been dropped by the L5 close-out purge (GOGOBEE_LEGACY_PURGE=1).
func snapshotPreDnD(d *sql.DB) error {
	var name string
	if err := d.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='adventure_characters'`,
	).Scan(&name); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	_, err := d.Exec(`
		INSERT OR IGNORE INTO adventure_characters_pre_dnd (user_id, snapshot_json)
		SELECT user_id,
		       json_object(
		         'combat_level', combat_level,
		         'combat_xp', combat_xp,
		         'mining_skill', mining_skill, 'mining_xp', mining_xp,
		         'foraging_skill', foraging_skill, 'foraging_xp', foraging_xp,
		         'fishing_skill', fishing_skill, 'fishing_xp', fishing_xp,
		         'arena_wins', arena_wins, 'arena_losses', arena_losses,
		         'current_streak', current_streak, 'best_streak', best_streak
		       )
		FROM adventure_characters
	`)
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
	Exec("mark job completed",
		`INSERT INTO daily_prefetch (job_name, date, completed) VALUES (?, ?, 1)
		 ON CONFLICT(job_name, date) DO UPDATE SET completed = 1`,
		jobName, dateKey,
	)
}

// RecordCrash logs a panic/crash event with version and plugin context.
func RecordCrash(botVersion, plugin, message string) {
	if globalDB == nil {
		return
	}
	Exec("crash log",
		`INSERT INTO crash_log (bot_version, plugin, message) VALUES (?, ?, ?)`,
		botVersion, plugin, message,
	)
}

// RecordTipAudit logs a poker tip with full context for bulk review.
func RecordTipAudit(userID, hole, board, street, position string, numActive int, equityPct float64, pot, toCall, stack int64, spr float64, handCat, drawDesc, preflopTier, action, tipText string) {
	if globalDB == nil {
		return
	}
	Exec("tip audit",
		`INSERT INTO holdem_tip_audit (user_id, hole, board, street, position, num_active, equity_pct, pot, to_call, stack, spr, hand_cat, draw_desc, preflop_tier, action, tip_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, hole, board, street, position, numActive, equityPct, pot, toCall, stack, spr, handCat, drawDesc, preflopTier, action, tipText,
	)
}

// RecordStartup logs a version startup event.
func RecordStartup(version, commit string) {
	Exec("version history",
		`INSERT INTO version_history (version, commit_sha) VALUES (?, ?)`,
		version, commit,
	)
}

// CrashCount returns the number of crashes for a given bot version.
func CrashCount(botVersion string) int {
	var count int
	_ = Get().QueryRow(`SELECT COUNT(*) FROM crash_log WHERE bot_version = ?`, botVersion).Scan(&count)
	return count
}

// CrashCountAll returns the total number of recorded crashes.
func CrashCountAll() int {
	var count int
	_ = Get().QueryRow(`SELECT COUNT(*) FROM crash_log`).Scan(&count)
	return count
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
	Exec("cache set",
		`INSERT INTO api_cache (cache_key, data, cached_at) VALUES (?, ?, unixepoch())
		 ON CONFLICT(cache_key) DO UPDATE SET data = ?, cached_at = unixepoch()`,
		key, data, data,
	)
}

// Backup creates a consistent snapshot of the database using VACUUM INTO.
// Keeps the last 7 daily backups, deleting older ones.
//
// Gated on the GOGOBEE_BACKUP_DIR env var: when unset, this is a no-op so
// dev environments don't accumulate snapshots. When set, that directory is
// used as the backup destination.
func Backup() error {
	backupDir := os.Getenv("GOGOBEE_BACKUP_DIR")
	if backupDir == "" {
		slog.Debug("backup skipped: GOGOBEE_BACKUP_DIR unset")
		return nil
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	filename := fmt.Sprintf("gogobee_%s.db", time.Now().UTC().Format("2006-01-02"))
	backupPath := filepath.Join(backupDir, filename)

	_, err := Get().Exec(fmt.Sprintf(`VACUUM INTO '%s'`, backupPath))
	if err != nil {
		return fmt.Errorf("vacuum into backup: %w", err)
	}

	slog.Info("database backup created", "path", backupPath)

	// Prune backups older than 7 days
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil // backup succeeded, prune failure is non-fatal
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -7)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(backupDir, e.Name()))
			slog.Info("pruned old backup", "file", e.Name())
		}
	}

	return nil
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

		// Daily activity older than 1 year
		{"daily_activity", `DELETE FROM daily_activity WHERE date < ?`, []interface{}{now.AddDate(-1, 0, 0).Format("2006-01-02")}},

		// Forex rates older than 2 years (analysis needs 52 weeks, keep buffer)
		{"forex_rates", `DELETE FROM forex_rates WHERE date < ?`, []interface{}{now.AddDate(-2, 0, 0).Format("2006-01-02")}},

		// Market snapshots older than 1 year
		{"market_snapshots", `DELETE FROM market_snapshots WHERE snapshot_date < ?`, []interface{}{now.AddDate(-1, 0, 0).Format("2006-01-02")}},
		{"market_daily_summary", `DELETE FROM market_daily_summary WHERE snapshot_date < ?`, []interface{}{now.AddDate(-1, 0, 0).Format("2006-01-02")}},
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

// Exec runs a write query, logging any error with the given label.
// Use for fire-and-forget writes where the error doesn't affect control flow.
func Exec(label, query string, args ...interface{}) {
	_, err := Get().Exec(query, args...)
	if err != nil {
		slog.Error("db: "+label, "err", err)
	}
}

// ExecResult runs a write query and returns the result, logging any error.
// Use when you need RowsAffected() or LastInsertId() but still want auto-logging.
func ExecResult(label, query string, args ...interface{}) sql.Result {
	res, err := Get().Exec(query, args...)
	if err != nil {
		slog.Error("db: "+label, "err", err)
	}
	return res
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
	fancy_words INTEGER DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS room_sentiment_stats (
	room_id TEXT PRIMARY KEY,
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

-- Euro economy
CREATE TABLE IF NOT EXISTS euro_balances (
	user_id      TEXT PRIMARY KEY,
	balance      REAL NOT NULL DEFAULT 0,
	updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_euro_bal_user ON euro_balances(user_id);

CREATE TABLE IF NOT EXISTS euro_transactions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      TEXT NOT NULL,
	amount       REAL NOT NULL,
	reason       TEXT NOT NULL,
	created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_euro_tx_user ON euro_transactions(user_id, created_at);

-- Hangman scores
CREATE TABLE IF NOT EXISTS hangman_scores (
	user_id       TEXT PRIMARY KEY,
	total_earned  REAL NOT NULL DEFAULT 0,
	games_played  INTEGER NOT NULL DEFAULT 0,
	games_won     INTEGER NOT NULL DEFAULT 0
);

-- Blackjack scores
CREATE TABLE IF NOT EXISTS blackjack_scores (
	user_id       TEXT PRIMARY KEY,
	total_earned  REAL NOT NULL DEFAULT 0,
	games_played  INTEGER NOT NULL DEFAULT 0,
	games_won     INTEGER NOT NULL DEFAULT 0
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

-- Uno
CREATE TABLE IF NOT EXISTS uno_pot (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	balance     REAL NOT NULL DEFAULT 0,
	updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS uno_games (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	player_id       TEXT NOT NULL,
	wager           REAL NOT NULL,
	result          TEXT NOT NULL,
	pot_before      REAL NOT NULL,
	pot_after       REAL NOT NULL,
	turns           INTEGER NOT NULL,
	started_at      DATETIME NOT NULL,
	ended_at        DATETIME NOT NULL
);

-- Uno multiplayer
CREATE TABLE IF NOT EXISTS uno_multi_games (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	room_id         TEXT NOT NULL,
	ante            REAL NOT NULL,
	pot_total       REAL NOT NULL,
	winner_id       TEXT NOT NULL,
	player_ids      TEXT NOT NULL,
	result          TEXT NOT NULL,
	turns           INTEGER NOT NULL,
	started_at      DATETIME NOT NULL,
	ended_at        DATETIME NOT NULL
);

-- Bot defeat tracking (unified across all games)
CREATE TABLE IF NOT EXISTS bot_defeats (
	user_id   TEXT NOT NULL,
	game      TEXT NOT NULL,
	losses    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, game)
);

-- Texas Hold'em
CREATE TABLE IF NOT EXISTS holdem_tips_prefs (
	user_id    TEXT PRIMARY KEY,
	enabled    INTEGER NOT NULL DEFAULT 1,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS holdem_scores (
	user_id      TEXT PRIMARY KEY,
	hands_played INTEGER NOT NULL DEFAULT 0,
	total_won    INTEGER NOT NULL DEFAULT 0,
	total_lost   INTEGER NOT NULL DEFAULT 0,
	biggest_pot  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS holdem_npc_balance (
	npc_name     TEXT PRIMARY KEY,
	balance      INTEGER NOT NULL DEFAULT 10000,
	hands_played INTEGER NOT NULL DEFAULT 0
);

-- Wordle
CREATE TABLE IF NOT EXISTS wordle_stats (
	user_id          TEXT PRIMARY KEY,
	display_name     TEXT NOT NULL,
	total_guesses    INTEGER NOT NULL DEFAULT 0,
	puzzles_played   INTEGER NOT NULL DEFAULT 0,
	puzzles_solved   INTEGER NOT NULL DEFAULT 0,
	winning_guesses  INTEGER NOT NULL DEFAULT 0,
	updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS wordle_puzzles (
	puzzle_id     TEXT NOT NULL,
	room_id       TEXT NOT NULL,
	answer        TEXT NOT NULL,
	word_length   INTEGER NOT NULL,
	category      TEXT NOT NULL DEFAULT '',
	solved        INTEGER NOT NULL DEFAULT 0,
	guess_count   INTEGER NOT NULL DEFAULT 0,
	started_at    DATETIME NOT NULL,
	solved_at     DATETIME,
	PRIMARY KEY (puzzle_id, room_id)
);

CREATE TABLE IF NOT EXISTS wordle_guesses (
	puzzle_id    TEXT NOT NULL,
	room_id      TEXT NOT NULL,
	guess_num    INTEGER NOT NULL,
	word         TEXT NOT NULL,
	player_id    TEXT NOT NULL,
	player_name  TEXT NOT NULL,
	guessed_at   DATETIME NOT NULL,
	PRIMARY KEY (puzzle_id, room_id, guess_num)
);

-- Space groups (rooms with overlapping membership)
CREATE TABLE IF NOT EXISTS space_groups (
	room_id    TEXT PRIMARY KEY,
	group_id   INTEGER NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- ── Adventure Plugin ────────────────────────────────────────────────────────

-- adventure_characters was retired in the L5 close-out. Per-user state lives
-- in player_meta now; the table is dropped via GOGOBEE_LEGACY_PURGE=1 and
-- the schema CREATE has been removed so it is not re-instated on next boot.

CREATE TABLE IF NOT EXISTS adventure_equipment (
	user_id      TEXT NOT NULL,
	slot         TEXT NOT NULL,
	tier         INTEGER NOT NULL DEFAULT 0,
	condition    INTEGER NOT NULL DEFAULT 100,
	name         TEXT NOT NULL,
	actions_used INTEGER NOT NULL DEFAULT 0,
	arena_tier   INTEGER NOT NULL DEFAULT 0,
	arena_set    TEXT NOT NULL DEFAULT '',
	masterwork   INTEGER NOT NULL DEFAULT 0,
	skill_source TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (user_id, slot)
);

CREATE TABLE IF NOT EXISTS adventure_inventory (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     TEXT NOT NULL,
	name        TEXT NOT NULL,
	item_type   TEXT NOT NULL,
	tier        INTEGER NOT NULL,
	value       INTEGER NOT NULL,
	slot         TEXT NOT NULL DEFAULT '',
	skill_source TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_adv_inv_user ON adventure_inventory(user_id);

CREATE TABLE IF NOT EXISTS adventure_activity_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       TEXT NOT NULL,
	activity_type TEXT NOT NULL,
	location      TEXT,
	outcome       TEXT NOT NULL,
	loot_value    INTEGER NOT NULL DEFAULT 0,
	xp_gained     INTEGER NOT NULL DEFAULT 0,
	flavor_key    TEXT,
	logged_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_adv_log_user ON adventure_activity_log(user_id, logged_at);

CREATE TABLE IF NOT EXISTS adventure_treasures (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      TEXT NOT NULL,
	treasure_key TEXT NOT NULL,
	name         TEXT NOT NULL,
	tier         INTEGER NOT NULL,
	bonus_type   TEXT NOT NULL,
	bonus_value  REAL NOT NULL,
	acquired_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, treasure_key, bonus_type)
);
CREATE INDEX IF NOT EXISTS idx_adv_treasure_user ON adventure_treasures(user_id);

-- Magic items equipped into the D&D 10-slot scheme (Open5e SRD registry).
-- One row per (user, DnDSlot). item_id references magicItemRegistry. Effects
-- are applied in combat from a codified Rarity+Kind formula; attunement gates
-- whether the item's effect counts (see magic_items_gameplay.go).
CREATE TABLE IF NOT EXISTS magic_item_equipped (
	user_id  TEXT NOT NULL,
	slot     TEXT NOT NULL,
	item_id  TEXT NOT NULL,
	attuned  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, slot)
);
CREATE INDEX IF NOT EXISTS idx_magic_item_equipped_user ON magic_item_equipped(user_id);

CREATE TABLE IF NOT EXISTS adventure_buffs (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    TEXT NOT NULL,
	buff_type  TEXT NOT NULL,
	buff_name  TEXT NOT NULL,
	modifier   REAL NOT NULL,
	expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_adv_buffs_user ON adventure_buffs(user_id, expires_at);

CREATE TABLE IF NOT EXISTS adventure_twinbee_log (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	activity_type     TEXT NOT NULL,
	location          TEXT NOT NULL,
	outcome           TEXT NOT NULL,
	loot_value        INTEGER NOT NULL DEFAULT 0,
	loot_desc         TEXT,
	participant_count INTEGER NOT NULL DEFAULT 0,
	gold_share        INTEGER NOT NULL DEFAULT 0,
	gift_count        INTEGER NOT NULL DEFAULT 0,
	logged_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- v2 stubs

CREATE TABLE IF NOT EXISTS adventure_market_listings (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	seller_id     TEXT NOT NULL,
	treasure_id   INTEGER NOT NULL,
	asking_price  INTEGER NOT NULL,
	listed_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at    DATETIME NOT NULL,
	sold_at       DATETIME,
	buyer_id      TEXT
);

CREATE TABLE IF NOT EXISTS adventure_invasions (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	horde_name        TEXT NOT NULL,
	horde_hp          INTEGER NOT NULL,
	horde_tier        INTEGER NOT NULL,
	outcome           TEXT,
	participant_count INTEGER NOT NULL DEFAULT 0,
	started_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ends_at           DATETIME NOT NULL,
	resolved_at       DATETIME
);

CREATE TABLE IF NOT EXISTS adventure_invasion_participants (
	invasion_id       INTEGER NOT NULL,
	user_id           TEXT NOT NULL,
	damage_dealt      INTEGER NOT NULL DEFAULT 0,
	xp_gained         INTEGER NOT NULL DEFAULT 0,
	loot_value        INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (invasion_id, user_id)
);

CREATE TABLE IF NOT EXISTS adventure_arena_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	challenger_id TEXT NOT NULL,
	defender_id   TEXT NOT NULL,
	winner_id     TEXT NOT NULL,
	xp_gained     INTEGER NOT NULL DEFAULT 0,
	logged_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS adventure_events_log (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      TEXT NOT NULL,
	event_key    TEXT NOT NULL,
	triggered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at   DATETIME NOT NULL,
	responded_at DATETIME,
	gold_awarded INTEGER NOT NULL DEFAULT 0,
	xp_awarded   INTEGER NOT NULL DEFAULT 0,
	outcome      TEXT NOT NULL DEFAULT 'pending'
);
CREATE INDEX IF NOT EXISTS idx_adv_events_user_outcome ON adventure_events_log(user_id, outcome);

-- Arena (Phase 2)
CREATE TABLE IF NOT EXISTS arena_runs (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id         TEXT NOT NULL,
	room_id         TEXT NOT NULL,
	start_tier      INTEGER NOT NULL,
	tier            INTEGER NOT NULL,
	round           INTEGER NOT NULL DEFAULT 1,
	status          TEXT NOT NULL DEFAULT 'active',
	earnings        INTEGER NOT NULL DEFAULT 0,
	rounds_survived INTEGER NOT NULL DEFAULT 0,
	last_monster    TEXT NOT NULL DEFAULT '',
	started_at      INTEGER NOT NULL,
	ended_at        INTEGER
);

CREATE INDEX IF NOT EXISTS idx_arena_runs_user ON arena_runs(user_id, status);

CREATE TABLE IF NOT EXISTS arena_history (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id         TEXT NOT NULL,
	start_tier      INTEGER NOT NULL,
	tier            INTEGER NOT NULL,
	rounds_survived INTEGER NOT NULL,
	earnings        INTEGER NOT NULL,
	outcome         TEXT NOT NULL,
	monster_name    TEXT NOT NULL,
	created_at      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS arena_stats (
	user_id             TEXT PRIMARY KEY,
	total_runs          INTEGER NOT NULL DEFAULT 0,
	total_earnings      INTEGER NOT NULL DEFAULT 0,
	total_deaths        INTEGER NOT NULL DEFAULT 0,
	highest_tier        INTEGER NOT NULL DEFAULT 0,
	tier5_completions   INTEGER NOT NULL DEFAULT 0,
	updated_at          INTEGER NOT NULL
);

-- Rival System
CREATE TABLE IF NOT EXISTS adventure_rival_records (
	user_id      TEXT NOT NULL,
	rival_id     TEXT NOT NULL,
	wins         INTEGER NOT NULL DEFAULT 0,
	losses       INTEGER NOT NULL DEFAULT 0,
	last_duel_at DATETIME,
	PRIMARY KEY (user_id, rival_id)
);

CREATE TABLE IF NOT EXISTS adventure_rival_challenges (
	challenge_id  TEXT PRIMARY KEY,
	challenger_id TEXT NOT NULL,
	challenged_id TEXT NOT NULL,
	stake         INTEGER NOT NULL,
	round         INTEGER NOT NULL DEFAULT 1,
	player_score  INTEGER NOT NULL DEFAULT 0,
	rival_score   INTEGER NOT NULL DEFAULT 0,
	expires_at    DATETIME NOT NULL,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rival_challenges_user ON adventure_rival_challenges(challenged_id, expires_at);

CREATE TABLE IF NOT EXISTS community_pot (
	id         INTEGER PRIMARY KEY DEFAULT 1,
	balance    INTEGER NOT NULL DEFAULT 0,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Babysitting Service
CREATE TABLE IF NOT EXISTS adventure_babysit_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       TEXT NOT NULL,
	log_date      DATE NOT NULL,
	activity      TEXT NOT NULL,
	outcome       TEXT NOT NULL,
	gold_earned   INTEGER NOT NULL DEFAULT 0,
	xp_gained     INTEGER NOT NULL DEFAULT 0,
	items_dropped TEXT DEFAULT NULL,
	rival_refused TEXT DEFAULT NULL,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_babysit_log_user ON adventure_babysit_log(user_id);

-- Forex
CREATE TABLE IF NOT EXISTS forex_rates (
	currency TEXT NOT NULL,
	date     TEXT NOT NULL,
	rate     REAL NOT NULL,
	PRIMARY KEY (currency, date)
);

CREATE TABLE IF NOT EXISTS forex_alerts (
	user_id   TEXT NOT NULL,
	currency  TEXT NOT NULL,
	threshold REAL NOT NULL,
	fired_at  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, currency, threshold)
);

-- Miniflux RSS
CREATE TABLE IF NOT EXISTS miniflux_subscriptions (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	feed_id     INTEGER NOT NULL,
	room_id     TEXT NOT NULL,
	paused      INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	UNIQUE(feed_id, room_id)
);

CREATE TABLE IF NOT EXISTS miniflux_seen (
	feed_id     INTEGER NOT NULL,
	entry_id    INTEGER NOT NULL,
	seen_at     INTEGER NOT NULL,
	PRIMARY KEY (feed_id, entry_id)
);

-- Market snapshots (daily index data)
CREATE TABLE IF NOT EXISTS market_snapshots (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	snapshot_date   TEXT NOT NULL,
	symbol          TEXT NOT NULL,
	display_name    TEXT NOT NULL,
	price           REAL,
	prev_close      REAL,
	change_pct      REAL,
	source          TEXT NOT NULL,
	pulled_at       INTEGER NOT NULL,
	UNIQUE(snapshot_date, symbol)
);
CREATE INDEX IF NOT EXISTS idx_market_snap_date ON market_snapshots(snapshot_date, symbol);

CREATE TABLE IF NOT EXISTS market_daily_summary (
	snapshot_date   TEXT PRIMARY KEY,
	summary         TEXT,
	generated_at    INTEGER
);

-- Archetype cache (recalculated nightly)
CREATE TABLE IF NOT EXISTS user_archetypes (
	user_id      TEXT NOT NULL,
	archetype    TEXT NOT NULL,
	category     TEXT NOT NULL DEFAULT '',
	signal_score REAL NOT NULL DEFAULT 0,
	flavor       TEXT NOT NULL DEFAULT '',
	assigned_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, archetype)
);
CREATE INDEX IF NOT EXISTS idx_user_archetypes_user ON user_archetypes(user_id, signal_score DESC);

-- Lottery tickets
CREATE TABLE IF NOT EXISTS lottery_tickets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     TEXT NOT NULL,
    week_start  DATE NOT NULL,
    numbers     TEXT NOT NULL,
    match_count INTEGER DEFAULT NULL,
    prize       INTEGER DEFAULT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_lottery_tickets_week ON lottery_tickets(week_start, user_id);

-- Lottery draw history
CREATE TABLE IF NOT EXISTS lottery_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    draw_date       DATE NOT NULL,
    winning_numbers TEXT NOT NULL,
    jackpot_winners INTEGER DEFAULT 0,
    jackpot_amount  INTEGER DEFAULT 0,
    match4_winners  INTEGER DEFAULT 0,
    match3_winners  INTEGER DEFAULT 0,
    match2_winners  INTEGER DEFAULT 0,
    match1_winners  INTEGER DEFAULT 0,
    pot_total       INTEGER NOT NULL,
    rolled_over     INTEGER DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS crash_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    bot_version TEXT NOT NULL,
    plugin      TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS version_history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    version    TEXT NOT NULL,
    commit_sha TEXT NOT NULL DEFAULT '',
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tax_ledger (
    user_id    TEXT PRIMARY KEY,
    total_paid INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS holdem_tip_audit (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     TEXT NOT NULL,
    hole        TEXT NOT NULL,
    board       TEXT NOT NULL DEFAULT '',
    street      TEXT NOT NULL,
    position    TEXT NOT NULL,
    num_active  INTEGER NOT NULL,
    equity_pct  REAL NOT NULL,
    pot         INTEGER NOT NULL,
    to_call     INTEGER NOT NULL,
    stack       INTEGER NOT NULL,
    spr         REAL NOT NULL,
    hand_cat    TEXT NOT NULL DEFAULT '',
    draw_desc   TEXT NOT NULL DEFAULT '',
    preflop_tier TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    tip_text    TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tip_audit_user ON holdem_tip_audit(user_id);
CREATE INDEX IF NOT EXISTS idx_tip_audit_action ON holdem_tip_audit(action, street);

-- Co-op Dungeon (party multi-day runs)
CREATE TABLE IF NOT EXISTS coop_dungeon_runs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tier              INTEGER NOT NULL,
    status            TEXT NOT NULL DEFAULT 'open',  -- open, active, complete, wiped, cancelled
    leader_id         TEXT NOT NULL,
    current_day       INTEGER NOT NULL DEFAULT 0,
    total_days        INTEGER NOT NULL,
    base_difficulty   INTEGER NOT NULL,              -- per-floor failure % at zero modifier
    gold_pool         INTEGER NOT NULL DEFAULT 0,    -- accumulated funding
    reward_total      INTEGER NOT NULL DEFAULT 0,    -- final reward (set on completion)
    last_resolved_day INTEGER NOT NULL DEFAULT 0,    -- crash-resume guard: highest day whose floor outcome is final
    invite_post_id    TEXT NOT NULL DEFAULT '',      -- Matrix event ID of the games-room invite (for pin/unpin)
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at         DATETIME,
    completed_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_coop_runs_status ON coop_dungeon_runs(status);

CREATE TABLE IF NOT EXISTS coop_dungeon_members (
    run_id            INTEGER NOT NULL,
    user_id           TEXT NOT NULL,
    turn_order        INTEGER NOT NULL,
    total_contributed INTEGER NOT NULL DEFAULT 0,
    is_liability      INTEGER NOT NULL DEFAULT 0,
    daily_funding     TEXT NOT NULL DEFAULT '{}',  -- JSON: {"1":"standard","2":"all_in",...}
    member_payout     INTEGER,                     -- NULL until reward credited; idempotency claim
    PRIMARY KEY (run_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_coop_members_user ON coop_dungeon_members(user_id);

CREATE TABLE IF NOT EXISTS coop_dungeon_events (
    run_id           INTEGER NOT NULL,
    day              INTEGER NOT NULL,
    category         TEXT NOT NULL,                  -- obstacle, opportunity, crisis, encounter
    event_index      INTEGER NOT NULL,               -- index into the category flavor pool
    recommended      TEXT NOT NULL,                  -- A, B, or C — TwinBee's pick
    votes            TEXT NOT NULL DEFAULT '{}',     -- JSON {"@user:host": "A"}
    winning_vote     TEXT DEFAULT NULL,
    outcome          TEXT DEFAULT NULL,              -- 'success' or 'failure' (after resolution)
    modifier_applied INTEGER DEFAULT 0,
    post_event_id    TEXT DEFAULT NULL,              -- Matrix event ID for live edit
    PRIMARY KEY (run_id, day)
);
CREATE INDEX IF NOT EXISTS idx_coop_events_outcome ON coop_dungeon_events(outcome);

CREATE TABLE IF NOT EXISTS coop_dungeon_bets (
    run_id     INTEGER NOT NULL,
    player_id  TEXT NOT NULL,
    position   TEXT NOT NULL,                 -- 'success' or 'failure'
    amount     INTEGER NOT NULL,
    placed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payout     INTEGER,                       -- NULL until run resolves
    PRIMARY KEY (run_id, player_id)
);
CREATE INDEX IF NOT EXISTS idx_coop_bets_run ON coop_dungeon_bets(run_id);

CREATE TABLE IF NOT EXISTS coop_dungeon_gifts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER NOT NULL,
    sender_id     TEXT NOT NULL,
    day           INTEGER NOT NULL,
    gift_type     TEXT NOT NULL,                 -- 'basket' or 'mimic'
    votes         TEXT NOT NULL DEFAULT '{}',    -- JSON {"@user:host": "open"|"leave"}
    vote_result   TEXT,                          -- 'opened' or 'left'
    outcome       TEXT,                          -- 'boost' or 'reduction'
    modifier      INTEGER NOT NULL DEFAULT 0,
    post_event_id TEXT,
    sent_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    DATETIME,                      -- voting closes; tally fires when reached
    resolved_at   DATETIME,                      -- vote tallied (vote_result/modifier set)
    applied_at    DATETIME,                      -- modifier merged into a floor success roll
    stack_lead_id INTEGER                        -- NULL for lead/standalone; follower points to lead's id
);
CREATE INDEX IF NOT EXISTS idx_coop_gifts_run_day ON coop_dungeon_gifts(run_id, day, vote_result);

-- ── D&D Layer (Phase 1) ────────────────────────────────────────────────────
-- Per-player D&D character state. Sits alongside adventure_characters; players
-- without a row here continue using the legacy adventure system unchanged.
-- pending_setup=1 means a draft (race/class/stats may be partial).
-- pending_setup=0 means !setup confirmed; D&D-gated commands available.
CREATE TABLE IF NOT EXISTS dnd_character (
    user_id        TEXT PRIMARY KEY,
    race           TEXT NOT NULL DEFAULT '',
    class          TEXT NOT NULL DEFAULT '',
    dnd_level      INTEGER NOT NULL DEFAULT 1,
    dnd_xp         INTEGER NOT NULL DEFAULT 0,
    str_score      INTEGER NOT NULL DEFAULT 8,
    dex_score      INTEGER NOT NULL DEFAULT 8,
    con_score      INTEGER NOT NULL DEFAULT 8,
    int_score      INTEGER NOT NULL DEFAULT 8,
    wis_score      INTEGER NOT NULL DEFAULT 8,
    cha_score      INTEGER NOT NULL DEFAULT 8,
    hp_current     INTEGER NOT NULL DEFAULT 0,
    hp_max         INTEGER NOT NULL DEFAULT 0,
    temp_hp        INTEGER NOT NULL DEFAULT 0,
    armor_class    INTEGER NOT NULL DEFAULT 10,
    pending_setup  INTEGER NOT NULL DEFAULT 1,
    last_respec_at DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dnd_abilities (
    user_id      TEXT NOT NULL,
    ability_id   TEXT NOT NULL,
    uses_left    INTEGER NOT NULL DEFAULT 0,
    acquired_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, ability_id)
);

CREATE TABLE IF NOT EXISTS dnd_resources (
    user_id        TEXT NOT NULL,
    resource_type  TEXT NOT NULL,    -- stamina|focus|mana|divine_favor|spell_slot_1
    current_value  INTEGER NOT NULL DEFAULT 0,
    max_value      INTEGER NOT NULL DEFAULT 0,
    last_reset_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, resource_type)
);

CREATE TABLE IF NOT EXISTS dnd_combat_state (
    user_id        TEXT PRIMARY KEY,
    enemy_id       TEXT NOT NULL DEFAULT '',
    round          INTEGER NOT NULL DEFAULT 0,
    player_turn    INTEGER NOT NULL DEFAULT 1,
    conditions_json TEXT NOT NULL DEFAULT '[]',
    log_json       TEXT NOT NULL DEFAULT '[]',
    started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ── D&D Layer (Phase 9 — spells) ──────────────────────────────────────────
-- Per-player known spell list. For Cleric, prepared=0 means the spell is on
-- the class list but not currently prepared (cast unavailable until prepared).
-- For Mage/Ranger, prepared is always 1 (spells known are always castable).
CREATE TABLE IF NOT EXISTS dnd_known_spells (
    user_id    TEXT NOT NULL,
    spell_id   TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT '',  -- class | subclass | racial
    prepared   INTEGER NOT NULL DEFAULT 1,
    learned_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, spell_id)
);

-- Per-player spell slot pool, one row per (user, slot_level). slot_level is
-- 1..5 (no cantrip row — cantrips have no slot cost). Refreshed on long rest.
CREATE TABLE IF NOT EXISTS dnd_spell_slots (
    user_id    TEXT NOT NULL,
    slot_level INTEGER NOT NULL,
    total      INTEGER NOT NULL DEFAULT 0,
    used       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, slot_level)
);

-- One-shot snapshot of adventure_characters taken at first deploy of the D&D layer.
-- Rollback safety net: if anything in adventure_characters diverges unexpectedly,
-- compare to this snapshot to detect data corruption.
CREATE TABLE IF NOT EXISTS adventure_characters_pre_dnd (
    user_id          TEXT PRIMARY KEY,
    snapshot_json    TEXT NOT NULL,
    snapshotted_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ── D&D Layer (Phase 11 D1b — zone runs) ───────────────────────────────────
-- A single in-progress or completed dungeon run. One row per run; players
-- may have at most one row with completed_at IS NULL AND abandoned = 0
-- (enforced in code, not via constraint, to keep migrations simple).
-- rooms_cleared is a JSON array of room indices the player has resolved
-- (combat won / trap survived / etc.). The zone-graph columns
-- (current_node, visited_nodes, node_choices) are added in Phase G1's
-- columnMigrations; the legacy linear current_room / room_seq_json
-- columns retired in G9.
CREATE TABLE IF NOT EXISTS dnd_zone_run (
    run_id          TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    zone_id         TEXT NOT NULL,
    total_rooms     INTEGER NOT NULL,
    rooms_cleared   TEXT NOT NULL DEFAULT '[]',
    boss_defeated   INTEGER NOT NULL DEFAULT 0,
    abandoned       INTEGER NOT NULL DEFAULT 0,
    loot_collected  TEXT NOT NULL DEFAULT '[]',
    gm_mood         INTEGER NOT NULL DEFAULT 50,
    started_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_action_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at    DATETIME
);
CREATE INDEX IF NOT EXISTS idx_zone_run_active
    ON dnd_zone_run(user_id, completed_at, abandoned);

-- ── D&D Layer (Phase 12 E1a — expeditions) ─────────────────────────────────
-- A multi-day expedition wrapping a zone run. The expedition tracks
-- real-world day count, supplies, camp state, threat clock, and zone-specific
-- temporal stacks (heat / instability / time distortion). One active row
-- per player (status='active'); enforced in code.
--
-- supplies_json:    serialized ExpeditionSupplies (current, max, daily_burn, harsh_mod, foraged_today)
-- camp_json:        serialized *CampState (NULL when no camp pitched)
-- threat_events:    serialized []ThreatEvent log
-- region_state:     serialized region progression for Tier 4-5 zones (E4)
CREATE TABLE IF NOT EXISTS dnd_expedition (
    expedition_id    TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL,
    zone_id          TEXT NOT NULL,
    run_id           TEXT,                              -- FK-ish: dnd_zone_run.run_id
    status           TEXT NOT NULL DEFAULT 'active',    -- active|extracting|complete|failed|abandoned
    start_date       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    current_day      INTEGER NOT NULL DEFAULT 1,
    current_region   TEXT NOT NULL DEFAULT '',
    boss_defeated    INTEGER NOT NULL DEFAULT 0,
    supplies_json    TEXT NOT NULL DEFAULT '{}',
    camp_json        TEXT,
    threat_level     INTEGER NOT NULL DEFAULT 0,
    threat_siege     INTEGER NOT NULL DEFAULT 0,
    threat_events    TEXT NOT NULL DEFAULT '[]',
    temporal_stack   INTEGER NOT NULL DEFAULT 0,        -- heat / instability / etc.
    region_state     TEXT NOT NULL DEFAULT '{}',
    xp_earned        INTEGER NOT NULL DEFAULT 0,
    coins_earned     INTEGER NOT NULL DEFAULT 0,
    gm_mood          INTEGER NOT NULL DEFAULT 50,
    last_briefing_at DATETIME,
    last_recap_at    DATETIME,
    last_ambient_at  DATETIME,
    last_activity    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_expedition_active
    ON dnd_expedition(user_id, status);
CREATE INDEX IF NOT EXISTS idx_expedition_run
    ON dnd_expedition(run_id);

-- One row per ExpeditionEntry (§9). Cheap to append; queried last-N for
-- !expedition log and to reconstruct overnight events for morning briefings.
CREATE TABLE IF NOT EXISTS dnd_expedition_log (
    entry_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    expedition_id   TEXT NOT NULL,
    day             INTEGER NOT NULL,
    timestamp       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    entry_type      TEXT NOT NULL,                      -- action|combat|rest|event|narrative|briefing|recap
    summary         TEXT NOT NULL DEFAULT '',
    flavor          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_expedition_log_recent
    ON dnd_expedition_log(expedition_id, timestamp DESC);

-- One row per (user_id, recipe_id) once the player has discovered a Thom
-- Krooke crafting recipe via !lore (Phase R5). Recipes are otherwise hidden
-- from !craft list output until discovered.
CREATE TABLE IF NOT EXISTS dnd_known_recipe (
    user_id       TEXT NOT NULL,
    recipe_id     TEXT NOT NULL,
    discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, recipe_id)
);

-- ── Adv 2.0 Phase G1 — branching zone graph (zones as directed graphs) ────
-- Replaces the linear room_seq_json model with a directed-graph topology.
-- A zone has many nodes (rooms, forks, secrets, boss) and edges connecting
-- them. Edges may be locked behind Perception checks, keys, level gates,
-- or region-clear flags. Authoring lives in Go (registerZoneGraph); these
-- tables are the persisted shape for runtime lookups + future tooling.
-- See gogobee_branching_zones_plan.md §2 for the full schema rationale.
CREATE TABLE IF NOT EXISTS zone_node (
    node_id        TEXT PRIMARY KEY,           -- "crypt_valdris.entry"
    zone_id        TEXT NOT NULL,
    region_id      TEXT NOT NULL DEFAULT '',   -- empty for single-region zones
    kind           TEXT NOT NULL,              -- entry|exploration|trap|elite|boss|harvest|rest_camp|secret|fork|merge
    label          TEXT NOT NULL DEFAULT '',   -- human-readable for !zone map
    is_entry       INTEGER NOT NULL DEFAULT 0,
    is_boss        INTEGER NOT NULL DEFAULT 0,
    pos_x          INTEGER NOT NULL DEFAULT 0,
    pos_y          INTEGER NOT NULL DEFAULT 0,
    content_json   TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_zone_node_zone ON zone_node(zone_id);

CREATE TABLE IF NOT EXISTS zone_edge (
    edge_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id        TEXT NOT NULL,
    from_node      TEXT NOT NULL,              -- FK-ish: zone_node.node_id
    to_node        TEXT NOT NULL,              -- FK-ish: zone_node.node_id
    lock_kind      TEXT NOT NULL DEFAULT 'none', -- none|perception_check|key_required|level_min|region_clear|stat_check
    lock_data_json TEXT NOT NULL DEFAULT '{}',
    hint           TEXT NOT NULL DEFAULT '',
    weight         INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_zone_edge_from ON zone_edge(zone_id, from_node);

-- ── Adv 2.0 Phase L2 step 5 — player_meta ──────────────────────────────────
-- Holding pen for non-stat per-user state migrating off adventure_characters
-- (gogobee_legacy_migration.md §2.1). Each L-phase ALTER TABLEs in the
-- columns it needs; this initial CREATE only covers L2's three arena
-- counters. The dual-write rule (§11) applies: writes go to both this
-- table and adventure_characters until a phase soaks clean for one week.
CREATE TABLE IF NOT EXISTS player_meta (
    user_id         TEXT PRIMARY KEY,
    arena_wins      INTEGER NOT NULL DEFAULT 0,
    arena_losses    INTEGER NOT NULL DEFAULT 0,
    invasion_score  INTEGER NOT NULL DEFAULT 0,
    display_name    TEXT NOT NULL DEFAULT '',
    hospital_visits INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS space_inviter_prompts (
    user_id          TEXT NOT NULL,
    prompt_sent_at   INTEGER NOT NULL,
    trigger_kind     TEXT NOT NULL,
    dm_room_id       TEXT NOT NULL,
    dm_event_id      TEXT NOT NULL,
    reply            TEXT,
    replied_at       INTEGER,
    invite_attempted INTEGER NOT NULL DEFAULT 0,
    invite_outcome   TEXT,
    invite_error     TEXT,
    PRIMARY KEY (user_id, prompt_sent_at)
);
CREATE INDEX IF NOT EXISTS idx_space_inviter_user ON space_inviter_prompts(user_id);

-- ── Turn-based combat — persistent per-fight session ───────────────────────
-- One row per manual elite/boss fight. Persists across bot restarts and
-- player away-from-keyboard so a fight can resume (or be auto-finished by
-- the timeout reaper) from exact mid-state. At most one row per user with
-- status='active' (enforced in code, not by constraint).
--
-- encounter_id:    room/node id within the run this fight belongs to.
-- enemy_id:        bestiary stat-block id for the elite/boss.
-- phase:           intra-round state machine position.
-- statuses_json:   serialized player + enemy status effects (poison, etc.).
-- turn_log_json:   per-round event stream, consumed by live narration.
-- expires_at:      started_at + 1h; reaper auto-plays the fight to a real
--                  win/loss from persisted state once this passes.
CREATE TABLE IF NOT EXISTS combat_session (
    session_id      TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    encounter_id    TEXT NOT NULL,
    enemy_id        TEXT NOT NULL,
    round           INTEGER NOT NULL DEFAULT 1,
    phase           TEXT NOT NULL DEFAULT 'player_turn', -- player_turn|enemy_turn|round_end|over
    player_hp       INTEGER NOT NULL,
    player_hp_max   INTEGER NOT NULL,
    enemy_hp        INTEGER NOT NULL,
    enemy_hp_max    INTEGER NOT NULL,
    statuses_json   TEXT NOT NULL DEFAULT '{}',
    turn_log_json   TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'active',      -- active|won|lost|fled|expired
    started_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_action_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_combat_session_active
    ON combat_session(user_id, status);
CREATE INDEX IF NOT EXISTS idx_combat_session_expiry
    ON combat_session(status, expires_at);
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
