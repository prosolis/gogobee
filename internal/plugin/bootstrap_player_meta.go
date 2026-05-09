package plugin

import (
	"database/sql"
	"log/slog"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// bootstrapPlayerMetaFromLegacy migrates rows from adventure_characters into
// player_meta for any user_id not already present, using the canonical
// upsertAllPlayerMetaFromAdvChar fan-out. Idempotent — re-runs touch zero
// rows once every legacy character has been migrated.
//
// Required when deploying L5 close-out code onto a database that hasn't gone
// through the L4-L5h dual-write soak (fresh restores from old backups, dev
// environments cloned from older snapshots, or — most importantly — a prod
// upgrade that skipped the soak window). Without this, the rewired
// loadAdvCharacter (which sources from player_meta) returns zero characters
// and silently strands every account.
//
// No-ops post-purge — the sqlite_master existence check short-circuits once
// adventure_characters is dropped.
func bootstrapPlayerMetaFromLegacy() {
	d := db.Get()

	var legacyExists int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='adventure_characters'`,
	).Scan(&legacyExists); err != nil || legacyExists == 0 {
		return
	}

	rows, err := d.Query(`
		SELECT user_id FROM adventure_characters
		WHERE user_id NOT IN (SELECT user_id FROM player_meta)`)
	if err != nil {
		slog.Error("bootstrap: legacy enumeration failed", "err", err)
		return
	}
	var todo []id.UserID
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			slog.Error("bootstrap: scan failed", "err", err)
			return
		}
		todo = append(todo, id.UserID(uid))
	}
	rows.Close()

	if len(todo) == 0 {
		return
	}

	migrated := 0
	for _, uid := range todo {
		char, err := loadAdvCharacterFromLegacyTable(uid)
		if err != nil {
			slog.Error("bootstrap: failed to load legacy AdvCharacter", "user", uid, "err", err)
			continue
		}
		// Seed the player_meta row first so the fan-out's per-subsystem
		// upserts have something to ON CONFLICT update.
		if _, err := d.Exec(
			`INSERT INTO player_meta (user_id, display_name, created_at, last_active_at, alive)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(user_id) DO NOTHING`,
			string(uid), char.DisplayName, char.CreatedAt, char.LastActiveAt,
			boolToInt(char.Alive),
		); err != nil {
			slog.Error("bootstrap: failed to seed player_meta", "user", uid, "err", err)
			continue
		}
		if err := upsertAllPlayerMetaFromAdvChar(char); err != nil {
			slog.Error("bootstrap: fan-out failed", "user", uid, "err", err)
			continue
		}
		migrated++
	}
	slog.Warn("bootstrap: legacy AdvCharacter rows migrated to player_meta",
		"migrated", migrated, "total", len(todo))
}

// loadAdvCharacterFromLegacyTable runs the pre-L5h 70-column SELECT against
// adventure_characters. Used only by the bootstrap path; the live loader
// goes through player_meta + applyPlayerMetaOverlay.
func loadAdvCharacterFromLegacyTable(userID id.UserID) (*AdventureCharacter, error) {
	d := db.Get()
	c := &AdventureCharacter{}
	var alive, actionTaken, holidayTaken, rivalUnlocked, babysitAct int
	var deadUntil, reprieveLast, babysitExp, pardonUsed sql.NullTime
	var mistyLastSeen, arinaLastSeen, mistyBuffExp, mistyDebuffExp, arinaBuffExp sql.NullTime
	var houseFrozen, houseAutopay int
	var petChasedAway, petReactivated, petArrived, thomAnimalLine, petSupplyUnlocked, petMorningDef int
	var autoBabysit, streakDecayed, treasuresLocked int

	err := d.QueryRow(`
		SELECT user_id, display_name,
		       combat_level, mining_skill, foraging_skill, fishing_skill,
		       combat_xp, mining_xp, foraging_xp, fishing_xp,
		       alive, dead_until, action_taken_today, holiday_action_taken,
		       arena_wins, arena_losses, invasion_score, title,
		       current_streak, best_streak, last_action_date, grudge_location,
		       created_at, last_active_at, death_reprieve_last,
		       masterwork_drops_received,
		       rival_pool, rival_unlocked_notified,
		       babysit_active, babysit_expires_at, babysit_skill_focus,
		       hospital_visits, robbie_visit_count, last_death_date,
		       combat_actions_used, harvest_actions_used,
		       last_pardon_used,
		       misty_last_seen, arina_last_seen,
		       misty_buff_expires, misty_debuff_expires, arina_buff_expires,
		       npc_msg_count, npc_msg_count_date,
		       misty_roll_target, arina_roll_target,
		       house_tier, house_loan_balance, house_loan_frozen, house_missed_payments,
		       house_autopay, house_current_rate,
		       pet_type, pet_name, pet_xp, pet_level, pet_armor_tier,
		       pet_chased_away, pet_reactivated, pet_arrived,
		       misty_encounter_count, misty_donated_count,
		       thom_animal_line_fired, pet_supply_shop_unlocked, pet_level10_date,
		       pet_morning_defense, auto_babysit, streak_decayed, crafts_succeeded,
		       death_source, death_location, auto_babysit_focus, treasures_locked
		FROM adventure_characters WHERE user_id = ?`, string(userID)).Scan(
		&c.UserID, &c.DisplayName,
		&c.CombatLevel, &c.MiningSkill, &c.ForagingSkill, &c.FishingSkill,
		&c.CombatXP, &c.MiningXP, &c.ForagingXP, &c.FishingXP,
		&alive, &deadUntil, &actionTaken, &holidayTaken,
		&c.ArenaWins, &c.ArenaLosses, &c.InvasionScore, &c.Title,
		&c.CurrentStreak, &c.BestStreak, &c.LastActionDate, &c.GrudgeLocation,
		&c.CreatedAt, &c.LastActiveAt, &reprieveLast,
		&c.MasterworkDropsReceived,
		&c.RivalPool, &rivalUnlocked,
		&babysitAct, &babysitExp, &c.BabysitSkillFocus,
		&c.HospitalVisits, &c.RobbieVisitCount, &c.LastDeathDate,
		&c.CombatActionsUsed, &c.HarvestActionsUsed,
		&pardonUsed,
		&mistyLastSeen, &arinaLastSeen,
		&mistyBuffExp, &mistyDebuffExp, &arinaBuffExp,
		&c.NPCMsgCount, &c.NPCMsgCountDate,
		&c.MistyRollTarget, &c.ArinaRollTarget,
		&c.HouseTier, &c.HouseLoanBalance, &houseFrozen, &c.HouseMissedPayments,
		&houseAutopay, &c.HouseCurrentRate,
		&c.PetType, &c.PetName, &c.PetXP, &c.PetLevel, &c.PetArmorTier,
		&petChasedAway, &petReactivated, &petArrived,
		&c.MistyEncounterCount, &c.MistyDonatedCount,
		&thomAnimalLine, &petSupplyUnlocked, &c.PetLevel10Date,
		&petMorningDef, &autoBabysit, &streakDecayed, &c.CraftsSucceeded,
		&c.DeathSource, &c.DeathLocation, &c.AutoBabysitFocus, &treasuresLocked,
	)
	if err != nil {
		return nil, err
	}
	c.Alive = alive == 1
	c.ActionTakenToday = actionTaken == 1
	c.HolidayActionTaken = holidayTaken == 1
	c.RivalUnlockedNotified = rivalUnlocked == 1
	c.BabysitActive = babysitAct == 1
	c.HouseLoanFrozen = houseFrozen == 1
	c.HouseAutopay = houseAutopay == 1
	c.PetChasedAway = petChasedAway == 1
	c.PetReactivated = petReactivated == 1
	c.PetArrived = petArrived == 1
	c.ThomAnimalLineFired = thomAnimalLine == 1
	c.PetSupplyShopUnlocked = petSupplyUnlocked == 1
	c.PetMorningDefense = petMorningDef == 1
	c.AutoBabysit = autoBabysit == 1
	c.StreakDecayed = streakDecayed == 1
	c.TreasuresLocked = treasuresLocked == 1
	if deadUntil.Valid {
		c.DeadUntil = &deadUntil.Time
	}
	if reprieveLast.Valid {
		c.DeathReprieveLast = &reprieveLast.Time
	}
	if babysitExp.Valid {
		c.BabysitExpiresAt = &babysitExp.Time
	}
	if pardonUsed.Valid {
		c.LastPardonUsed = &pardonUsed.Time
	}
	if mistyLastSeen.Valid {
		c.MistyLastSeen = &mistyLastSeen.Time
	}
	if arinaLastSeen.Valid {
		c.ArinaLastSeen = &arinaLastSeen.Time
	}
	if mistyBuffExp.Valid {
		c.MistyBuffExpires = &mistyBuffExp.Time
	}
	if mistyDebuffExp.Valid {
		c.MistyDebuffExpires = &mistyDebuffExp.Time
	}
	if arinaBuffExp.Valid {
		c.ArinaBuffExpires = &arinaBuffExp.Time
	}
	return c, nil
}
