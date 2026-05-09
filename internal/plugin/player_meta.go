package plugin

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// player_meta is the Adv 2.0 holding pen for non-stat per-user state
// migrating off adventure_characters (gogobee_legacy_migration.md §2.1).
// Phase L2 step 5 lands the table + arena counters; later phases extend
// the schema and add their own helpers here.

// PlayerMeta is the in-memory mirror of the player_meta row. Only the
// fields a phase has migrated are meaningful; unmigrated fields stay
// zero-valued and are sourced from AdvCharacter.
type PlayerMeta struct {
	UserID                  id.UserID
	ArenaWins               int
	ArenaLosses             int
	InvasionScore           int
	DisplayName             string
	HospitalVisits          int
	MasterworkDropsReceived int
}

// loadPlayerMeta reads the player_meta row for a user. Returns a
// zero-valued struct (with UserID set) when no row exists — callers
// should treat absence as "all fields zero", matching the column
// defaults.
func loadPlayerMeta(userID id.UserID) (*PlayerMeta, error) {
	m := &PlayerMeta{UserID: userID}
	err := db.Get().QueryRow(
		`SELECT arena_wins, arena_losses, invasion_score, display_name, hospital_visits, masterwork_drops_received FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&m.ArenaWins, &m.ArenaLosses, &m.InvasionScore, &m.DisplayName, &m.HospitalVisits, &m.MasterworkDropsReceived)
	if err == sql.ErrNoRows {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// upsertPlayerMetaArena writes arena_wins / arena_losses / invasion_score
// for a user. Creates the row if missing. Used by the dual-write path
// during the L1–L4 migration (§11): every arena counter mutation goes
// through both AdvCharacter and player_meta until soak completes.
func upsertPlayerMetaArena(userID id.UserID, wins, losses, invasionScore int) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, arena_wins, arena_losses, invasion_score)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   arena_wins = excluded.arena_wins,
		   arena_losses = excluded.arena_losses,
		   invasion_score = excluded.invasion_score`,
		string(userID), wins, losses, invasionScore,
	)
	return err
}

// backfillPlayerMetaArena copies arena_wins / arena_losses / invasion_score
// from adventure_characters into player_meta for any user_id that doesn't
// already have a row. Idempotent: INSERT OR IGNORE skips users that have
// already been backfilled (or that wrote a row through the dual-write
// path post-deploy). Logs the row count touched, matching Phase R1's
// archiveOrphanZoneRuns precedent.
func backfillPlayerMetaArena() error {
	res, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id, arena_wins, arena_losses, invasion_score)
		SELECT user_id, arena_wins, arena_losses, invasion_score FROM adventure_characters
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: arena counters backfilled", "rows", n)
	return nil
}

// upsertPlayerMetaDisplayName writes display_name for a user, leaving other
// columns untouched. Used by the dual-write path during the L4f-prep
// DisplayName migration: every place that mutates AdvCharacter.DisplayName
// also calls this so player_meta.display_name stays in sync until soak
// completes and readers flip over (gogobee_legacy_migration.md §7).
func upsertPlayerMetaDisplayName(userID id.UserID, displayName string) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, display_name) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET display_name = excluded.display_name`,
		string(userID), displayName,
	)
	return err
}

// loadDisplayName returns the player's display name from player_meta when
// present, falling back to adventure_characters.display_name during the
// L4f-prep soak window. Empty string if the user has no row in either
// table. Callers should prefer this helper over reading char.DisplayName
// directly so the eventual reader flip is a no-op at the call site.
func loadDisplayName(userID id.UserID) (string, error) {
	var name string
	err := db.Get().QueryRow(
		`SELECT display_name FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&name)
	if err == nil && name != "" {
		return name, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	// Fallback during soak.
	err = db.Get().QueryRow(
		`SELECT display_name FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

// upsertPlayerMetaHospitalVisits writes hospital_visits for a user, leaving
// other columns untouched. Used by the dual-write path during the L4a
// Hospital migration: every increment on AdvCharacter.HospitalVisits also
// calls this so player_meta.hospital_visits stays in sync until soak
// completes (gogobee_legacy_migration.md §6.1, §11).
func upsertPlayerMetaHospitalVisits(userID id.UserID, visits int) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, hospital_visits) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET hospital_visits = excluded.hospital_visits`,
		string(userID), visits,
	)
	return err
}

// loadHospitalVisits returns the player's hospital visit count from
// player_meta when present, falling back to adventure_characters during
// the L4a soak window. Zero if the user has no row in either table.
func loadHospitalVisits(userID id.UserID) (int, error) {
	var visits int
	err := db.Get().QueryRow(
		`SELECT hospital_visits FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&visits)
	if err == nil && visits > 0 {
		return visits, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	// player_meta row missing or zero → fall back to AdvCharacter for the
	// soak window. A zero value in player_meta could legitimately mean "no
	// visits yet", but the fallback only returns non-zero if the legacy
	// column has a higher value, so this is safe either way.
	var legacy int
	err = db.Get().QueryRow(
		`SELECT hospital_visits FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacy)
	if err == sql.ErrNoRows {
		return visits, nil
	}
	if err != nil {
		return 0, err
	}
	if legacy > visits {
		return legacy, nil
	}
	return visits, nil
}

// backfillPlayerMetaHospitalVisits copies adventure_characters.hospital_visits
// into player_meta.hospital_visits for any row whose value is still the
// default zero. Idempotent: only updates rows that haven't been populated
// yet (via backfill or dual-write).
func backfillPlayerMetaHospitalVisits() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET hospital_visits = (
			SELECT hospital_visits FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE hospital_visits = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND adventure_characters.hospital_visits > 0
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: hospital_visits backfilled", "rows", n)
	return nil
}

// upsertPlayerMetaRivalState writes rival_pool / rival_unlocked_notified for
// a user, leaving other columns untouched. Used by the dual-write path during
// the L4b Rival migration: every place that mutates AdvCharacter.RivalPool /
// RivalUnlockedNotified also calls this so player_meta stays in sync until
// soak completes (gogobee_legacy_migration.md §6.2, §11).
func upsertPlayerMetaRivalState(userID id.UserID, pool int, notified bool) error {
	notifiedInt := 0
	if notified {
		notifiedInt = 1
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, rival_pool, rival_unlocked_notified) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   rival_pool = excluded.rival_pool,
		   rival_unlocked_notified = excluded.rival_unlocked_notified`,
		string(userID), pool, notifiedInt,
	)
	return err
}

// loadRivalState returns rival_pool / rival_unlocked_notified for a user from
// player_meta when populated, falling back to adventure_characters during the
// L4b soak window. Treats a missing row as (0, false). The fallback returns
// the AdvCharacter values whenever player_meta has the unmigrated default
// (pool=0, notified=0) — a true "not yet in pool" state and the pre-backfill
// state are indistinguishable at the column level, but the legacy column
// only ever moves toward unlocked, so picking the higher of the two values
// is safe.
func loadRivalState(userID id.UserID) (pool int, notified bool, err error) {
	var notifiedInt int
	err = db.Get().QueryRow(
		`SELECT rival_pool, rival_unlocked_notified FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&pool, &notifiedInt)
	if err == nil && (pool > 0 || notifiedInt > 0) {
		return pool, notifiedInt == 1, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}
	// Fallback during soak.
	var legacyPool, legacyNotified int
	err = db.Get().QueryRow(
		`SELECT rival_pool, rival_unlocked_notified FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacyPool, &legacyNotified)
	if err == sql.ErrNoRows {
		return pool, notifiedInt == 1, nil
	}
	if err != nil {
		return 0, false, err
	}
	if legacyPool > pool {
		pool = legacyPool
	}
	if legacyNotified > notifiedInt {
		notifiedInt = legacyNotified
	}
	return pool, notifiedInt == 1, nil
}

// backfillPlayerMetaRivalState copies rival_pool / rival_unlocked_notified
// from adventure_characters into player_meta for any row whose values are
// still the default zero. Idempotent: only updates rows that haven't been
// populated yet (via backfill or dual-write).
func backfillPlayerMetaRivalState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET rival_pool = (
			SELECT rival_pool FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		),
		    rival_unlocked_notified = (
			SELECT rival_unlocked_notified FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE rival_pool = 0 AND rival_unlocked_notified = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND (adventure_characters.rival_pool > 0
			       OR adventure_characters.rival_unlocked_notified > 0)
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: rival state backfilled", "rows", n)
	return nil
}

// upsertPlayerMetaMasterworkDrops writes masterwork_drops_received for a user,
// leaving other columns untouched. Used by the dual-write path during the
// L4c Masterwork migration: every increment on AdvCharacter.MasterworkDropsReceived
// also calls this so player_meta stays in sync until soak completes
// (gogobee_legacy_migration.md §6.3, §11).
func upsertPlayerMetaMasterworkDrops(userID id.UserID, drops int) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, masterwork_drops_received) VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET masterwork_drops_received = excluded.masterwork_drops_received`,
		string(userID), drops,
	)
	return err
}

// loadMasterworkDrops returns the player's masterwork drop count from
// player_meta when populated, falling back to adventure_characters during
// the L4c soak window. Zero if the user has no row in either table. Mirrors
// loadHospitalVisits: the legacy column only ever moves up, so taking the
// higher of the two values is safe across the dual-write window.
func loadMasterworkDrops(userID id.UserID) (int, error) {
	var drops int
	err := db.Get().QueryRow(
		`SELECT masterwork_drops_received FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&drops)
	if err == nil && drops > 0 {
		return drops, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	var legacy int
	err = db.Get().QueryRow(
		`SELECT masterwork_drops_received FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacy)
	if err == sql.ErrNoRows {
		return drops, nil
	}
	if err != nil {
		return 0, err
	}
	if legacy > drops {
		return legacy, nil
	}
	return drops, nil
}

// backfillPlayerMetaMasterworkDrops copies adventure_characters.masterwork_drops_received
// into player_meta.masterwork_drops_received for any row whose value is still
// the default zero. Idempotent: only updates rows that haven't been populated
// yet (via backfill or dual-write).
func backfillPlayerMetaMasterworkDrops() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET masterwork_drops_received = (
			SELECT masterwork_drops_received FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE masterwork_drops_received = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND adventure_characters.masterwork_drops_received > 0
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: masterwork_drops_received backfilled", "rows", n)
	return nil
}

// PetState is the in-memory mirror of player_meta's pet_* columns. Phase L4d
// ports pet ownership and progression off AdvCharacter (gogobee_legacy_migration.md
// §6.4). The four flag bools serialize as a single pet_flags_json column to
// avoid an explosion of boolean ALTERs when future pet behaviors land.
type PetState struct {
	Type               string
	Name               string
	XP                 int
	Level              int
	ArmorTier          int
	SupplyShopUnlocked bool
	Level10Date        string
	Arrived            bool
	ChasedAway         bool
	Reactivated        bool
	MorningDefense     bool
}

// HasPet returns true if the player has an active pet (arrived, not chased
// away). Mirrors AdventureCharacter.HasPet() for the cross-file pet-helper
// flip in L4d.
func (s PetState) HasPet() bool {
	return s.Type != "" && s.Arrived && !s.ChasedAway
}

// petFlagsJSON is the on-disk encoding of the four pet bools. Adding a new
// flag is an additive JSON field — old rows decode as false.
type petFlagsJSON struct {
	Arrived        bool `json:"arrived"`
	ChasedAway     bool `json:"chased_away"`
	Reactivated    bool `json:"reactivated"`
	MorningDefense bool `json:"morning_defense"`
}

// upsertPlayerMetaPetState writes the full pet column set for a user. Pet
// state mutates as a unit (arrival sets type/name/level/flags together;
// level-ups touch xp/level/level10date), so this helper takes the whole
// PetState rather than splitting per-field. Used by the dual-write path
// during the L4d soak window.
func upsertPlayerMetaPetState(userID id.UserID, s PetState) error {
	flags, err := json.Marshal(petFlagsJSON{
		Arrived:        s.Arrived,
		ChasedAway:     s.ChasedAway,
		Reactivated:    s.Reactivated,
		MorningDefense: s.MorningDefense,
	})
	if err != nil {
		return err
	}
	supplyInt := 0
	if s.SupplyShopUnlocked {
		supplyInt = 1
	}
	_, err = db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, pet_type, pet_name, pet_xp, pet_level, pet_armor_tier,
		   pet_flags_json, pet_supply_shop_unlocked, pet_level_10_date
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   pet_type = excluded.pet_type,
		   pet_name = excluded.pet_name,
		   pet_xp = excluded.pet_xp,
		   pet_level = excluded.pet_level,
		   pet_armor_tier = excluded.pet_armor_tier,
		   pet_flags_json = excluded.pet_flags_json,
		   pet_supply_shop_unlocked = excluded.pet_supply_shop_unlocked,
		   pet_level_10_date = excluded.pet_level_10_date`,
		string(userID), s.Type, s.Name, s.XP, s.Level, s.ArmorTier,
		string(flags), supplyInt, s.Level10Date,
	)
	return err
}

// loadPetState returns the player's pet state from player_meta when populated,
// falling back to adventure_characters during the L4d soak window. A
// player_meta row whose pet_type is empty is treated as unmigrated and falls
// through to AdvCharacter — matching the "type is the canonical
// has-a-pet marker" semantics used by HasPet().
func loadPetState(userID id.UserID) (PetState, error) {
	var (
		s         PetState
		flagsRaw  string
		supplyInt int
	)
	err := db.Get().QueryRow(
		`SELECT pet_type, pet_name, pet_xp, pet_level, pet_armor_tier,
		        pet_flags_json, pet_supply_shop_unlocked, pet_level_10_date
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&s.Type, &s.Name, &s.XP, &s.Level, &s.ArmorTier,
		&flagsRaw, &supplyInt, &s.Level10Date)
	if err == nil && s.Type != "" {
		s.SupplyShopUnlocked = supplyInt == 1
		var f petFlagsJSON
		if flagsRaw != "" {
			_ = json.Unmarshal([]byte(flagsRaw), &f)
		}
		s.Arrived = f.Arrived
		s.ChasedAway = f.ChasedAway
		s.Reactivated = f.Reactivated
		s.MorningDefense = f.MorningDefense
		return s, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return PetState{}, err
	}
	// Fallback to AdvCharacter during soak.
	var (
		legacyType, legacyName, legacyL10Date         string
		legacyXP, legacyLevel, legacyArmor            int
		legacySupply                                  int
		arrived, chased, reactivated, morningDefense  int
	)
	err = db.Get().QueryRow(
		`SELECT pet_type, pet_name, pet_xp, pet_level, pet_armor_tier,
		        pet_arrived, pet_chased_away, pet_reactivated, pet_morning_defense,
		        pet_supply_shop_unlocked, pet_level_10_date
		   FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacyType, &legacyName, &legacyXP, &legacyLevel, &legacyArmor,
		&arrived, &chased, &reactivated, &morningDefense,
		&legacySupply, &legacyL10Date)
	if err == sql.ErrNoRows {
		return PetState{}, nil
	}
	if err != nil {
		return PetState{}, err
	}
	return PetState{
		Type:               legacyType,
		Name:               legacyName,
		XP:                 legacyXP,
		Level:              legacyLevel,
		ArmorTier:          legacyArmor,
		SupplyShopUnlocked: legacySupply == 1,
		Level10Date:        legacyL10Date,
		Arrived:            arrived == 1,
		ChasedAway:         chased == 1,
		Reactivated:        reactivated == 1,
		MorningDefense:     morningDefense == 1,
	}, nil
}

// backfillPlayerMetaPetState copies the pet_* columns from adventure_characters
// into player_meta for any row whose pet_type is still the empty default.
// Idempotent: only updates rows that haven't been populated yet (via backfill
// or dual-write). Pet flags are encoded as JSON to match the runtime helper.
func backfillPlayerMetaPetState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	rows, err := db.Get().Query(`
		SELECT ac.user_id, ac.pet_type, ac.pet_name, ac.pet_xp, ac.pet_level,
		       ac.pet_armor_tier, ac.pet_arrived, ac.pet_chased_away,
		       ac.pet_reactivated, ac.pet_morning_defense,
		       ac.pet_supply_shop_unlocked, ac.pet_level_10_date
		  FROM adventure_characters ac
		  JOIN player_meta pm ON pm.user_id = ac.user_id
		 WHERE pm.pet_type = ''
		   AND ac.pet_type <> ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pending struct {
		uid id.UserID
		s   PetState
	}
	var todo []pending
	for rows.Next() {
		var (
			uid                                          string
			s                                            PetState
			arrived, chased, reactivated, morningDefense int
			supply                                       int
		)
		if err := rows.Scan(&uid, &s.Type, &s.Name, &s.XP, &s.Level,
			&s.ArmorTier, &arrived, &chased, &reactivated, &morningDefense,
			&supply, &s.Level10Date); err != nil {
			return err
		}
		s.Arrived = arrived == 1
		s.ChasedAway = chased == 1
		s.Reactivated = reactivated == 1
		s.MorningDefense = morningDefense == 1
		s.SupplyShopUnlocked = supply == 1
		todo = append(todo, pending{uid: id.UserID(uid), s: s})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range todo {
		if err := upsertPlayerMetaPetState(p.uid, p.s); err != nil {
			return err
		}
	}
	slog.Info("player_meta: pet state backfilled", "rows", len(todo))
	return nil
}

// petStateFromAdvChar projects the pet-related fields off an AdventureCharacter
// into a PetState. Used by the dual-write path: every site that mutates
// AdvCharacter pet fields then calls upsertPlayerMetaPetState with this
// projection so the column moves cleanly during the L4d soak window.
func petStateFromAdvChar(c *AdventureCharacter) PetState {
	return PetState{
		Type:               c.PetType,
		Name:               c.PetName,
		XP:                 c.PetXP,
		Level:              c.PetLevel,
		ArmorTier:          c.PetArmorTier,
		SupplyShopUnlocked: c.PetSupplyShopUnlocked,
		Level10Date:        c.PetLevel10Date,
		Arrived:            c.PetArrived,
		ChasedAway:         c.PetChasedAway,
		Reactivated:        c.PetReactivated,
		MorningDefense:     c.PetMorningDefense,
	}
}

// HouseState is the in-memory mirror of player_meta's house_* columns. Phase
// L4e ports housing/mortgage off AdvCharacter (gogobee_legacy_migration.md
// §6.5). All six fields mutate together at known sites (purchase, payoff,
// payment, autopay toggle, weekly tick, rate refresh), so the helper takes
// the whole struct rather than splitting per-field.
type HouseState struct {
	Tier            int
	LoanBalance     int
	LoanFrozen      bool
	MissedPayments  int
	Autopay         bool
	CurrentRate     float64
}

// HasHouse mirrors AdventureCharacter.HasHouse: a player counts as housed
// if they own any tier or carry an outstanding loan. Used as the "row is
// migrated" marker in loadHouseState — a zero/zero/zero state is
// indistinguishable from "no row yet" so it falls through to the legacy
// table during the soak window.
func (s HouseState) HasHouse() bool { return s.Tier > 0 || s.LoanBalance > 0 }

// upsertPlayerMetaHouseState writes the full house column set for a user.
// Used by the dual-write path during the L4e soak window.
func upsertPlayerMetaHouseState(userID id.UserID, s HouseState) error {
	frozen := 0
	if s.LoanFrozen {
		frozen = 1
	}
	autopay := 0
	if s.Autopay {
		autopay = 1
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, house_tier, house_loan_balance, house_loan_frozen,
		   house_missed_payments, house_autopay, house_current_rate
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   house_tier = excluded.house_tier,
		   house_loan_balance = excluded.house_loan_balance,
		   house_loan_frozen = excluded.house_loan_frozen,
		   house_missed_payments = excluded.house_missed_payments,
		   house_autopay = excluded.house_autopay,
		   house_current_rate = excluded.house_current_rate`,
		string(userID), s.Tier, s.LoanBalance, frozen,
		s.MissedPayments, autopay, s.CurrentRate,
	)
	return err
}

// loadHouseState returns the player's house state from player_meta when
// populated (HasHouse true), falling back to adventure_characters during
// the L4e soak window. A row with tier=0 and loan_balance=0 is treated as
// unmigrated and falls through — matching the canonical "tier > 0 || loan
// > 0" has-a-house marker used elsewhere.
func loadHouseState(userID id.UserID) (HouseState, error) {
	var (
		s          HouseState
		frozenInt  int
		autopayInt int
	)
	err := db.Get().QueryRow(
		`SELECT house_tier, house_loan_balance, house_loan_frozen,
		        house_missed_payments, house_autopay, house_current_rate
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&s.Tier, &s.LoanBalance, &frozenInt,
		&s.MissedPayments, &autopayInt, &s.CurrentRate)
	if err == nil && (s.Tier > 0 || s.LoanBalance > 0) {
		s.LoanFrozen = frozenInt == 1
		s.Autopay = autopayInt == 1
		return s, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return HouseState{}, err
	}
	// Fallback to AdvCharacter during soak.
	var (
		legacyTier, legacyBalance, legacyMissed int
		legacyFrozen, legacyAutopay             int
		legacyRate                              float64
	)
	err = db.Get().QueryRow(
		`SELECT house_tier, house_loan_balance, house_loan_frozen,
		        house_missed_payments, house_autopay, house_current_rate
		   FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacyTier, &legacyBalance, &legacyFrozen,
		&legacyMissed, &legacyAutopay, &legacyRate)
	if err == sql.ErrNoRows {
		return HouseState{}, nil
	}
	if err != nil {
		return HouseState{}, err
	}
	return HouseState{
		Tier:           legacyTier,
		LoanBalance:    legacyBalance,
		LoanFrozen:     legacyFrozen == 1,
		MissedPayments: legacyMissed,
		Autopay:        legacyAutopay == 1,
		CurrentRate:    legacyRate,
	}, nil
}

// backfillPlayerMetaHouseState copies the house_* columns from
// adventure_characters into player_meta for any row whose house_tier and
// house_loan_balance are both still the default zero. Idempotent: only
// updates rows that haven't been populated yet (via backfill or
// dual-write).
func backfillPlayerMetaHouseState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET house_tier = (SELECT house_tier FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    house_loan_balance = (SELECT house_loan_balance FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    house_loan_frozen = (SELECT house_loan_frozen FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    house_missed_payments = (SELECT house_missed_payments FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    house_autopay = (SELECT house_autopay FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    house_current_rate = (SELECT house_current_rate FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id)
		WHERE house_tier = 0
		  AND house_loan_balance = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters ac
			WHERE ac.user_id = player_meta.user_id
			  AND (ac.house_tier > 0 OR ac.house_loan_balance > 0)
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: house state backfilled", "rows", n)
	return nil
}

// houseStateFromAdvChar projects the housing-related fields off an
// AdventureCharacter into a HouseState. Used by the dual-write path:
// every site that mutates AdvCharacter house fields then calls
// upsertPlayerMetaHouseState with this projection so the columns move
// cleanly during the L4e soak window.
func houseStateFromAdvChar(c *AdventureCharacter) HouseState {
	return HouseState{
		Tier:           c.HouseTier,
		LoanBalance:    c.HouseLoanBalance,
		LoanFrozen:     c.HouseLoanFrozen,
		MissedPayments: c.HouseMissedPayments,
		Autopay:        c.HouseAutopay,
		CurrentRate:    c.HouseCurrentRate,
	}
}

// SkillState is the in-memory mirror of player_meta's skill columns. Phase
// L5a ports CombatLevel/CombatXP and the three harvest skills + their XP
// off AdvCharacter (gogobee_legacy_migration.md §7.3 L5a). CombatLevel and
// CombatXP are transitional — they're dropped after L5g's DnDCharacter
// mass-backfill retires the legacy CombatLevel-derived fallback in
// hospital/rival/zone/sheet/onboarding.
type SkillState struct {
	CombatLevel   int
	CombatXP      int
	MiningSkill   int
	MiningXP      int
	ForagingSkill int
	ForagingXP    int
	FishingSkill  int
	FishingXP     int
}

// HasSkills returns true when any skill or XP field is non-zero. Used as
// the "row is migrated" marker in loadSkillState — an all-zero row is
// indistinguishable from "no row yet" so it falls through to the legacy
// table during the soak window. CombatLevel starts at 1 for any created
// character (see createAdvCharacter), so the only fully-zero state is a
// truly unmigrated row.
func (s SkillState) HasSkills() bool {
	return s.CombatLevel > 0 || s.MiningSkill > 0 || s.ForagingSkill > 0 || s.FishingSkill > 0 ||
		s.CombatXP > 0 || s.MiningXP > 0 || s.ForagingXP > 0 || s.FishingXP > 0
}

// upsertPlayerMetaSkillState writes the full skill column set for a user.
// Used by the dual-write path during the L5a soak window.
func upsertPlayerMetaSkillState(userID id.UserID, s SkillState) error {
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, combat_level, combat_xp,
		   mining_skill, mining_xp,
		   foraging_skill, foraging_xp,
		   fishing_skill, fishing_xp
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   combat_level = excluded.combat_level,
		   combat_xp = excluded.combat_xp,
		   mining_skill = excluded.mining_skill,
		   mining_xp = excluded.mining_xp,
		   foraging_skill = excluded.foraging_skill,
		   foraging_xp = excluded.foraging_xp,
		   fishing_skill = excluded.fishing_skill,
		   fishing_xp = excluded.fishing_xp`,
		string(userID),
		s.CombatLevel, s.CombatXP,
		s.MiningSkill, s.MiningXP,
		s.ForagingSkill, s.ForagingXP,
		s.FishingSkill, s.FishingXP,
	)
	return err
}

// loadSkillState returns the player's skill state from player_meta when
// populated (HasSkills true), falling back to adventure_characters during
// the L5a soak window.
func loadSkillState(userID id.UserID) (SkillState, error) {
	var s SkillState
	err := db.Get().QueryRow(
		`SELECT combat_level, combat_xp,
		        mining_skill, mining_xp,
		        foraging_skill, foraging_xp,
		        fishing_skill, fishing_xp
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(
		&s.CombatLevel, &s.CombatXP,
		&s.MiningSkill, &s.MiningXP,
		&s.ForagingSkill, &s.ForagingXP,
		&s.FishingSkill, &s.FishingXP,
	)
	if err == nil && s.HasSkills() {
		return s, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return SkillState{}, err
	}
	// Fallback to AdvCharacter during soak.
	var legacy SkillState
	err = db.Get().QueryRow(
		`SELECT combat_level, combat_xp,
		        mining_skill, mining_xp,
		        foraging_skill, foraging_xp,
		        fishing_skill, fishing_xp
		   FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(
		&legacy.CombatLevel, &legacy.CombatXP,
		&legacy.MiningSkill, &legacy.MiningXP,
		&legacy.ForagingSkill, &legacy.ForagingXP,
		&legacy.FishingSkill, &legacy.FishingXP,
	)
	if err == sql.ErrNoRows {
		return SkillState{}, nil
	}
	if err != nil {
		return SkillState{}, err
	}
	return legacy, nil
}

// backfillPlayerMetaSkillState copies the skill columns from
// adventure_characters into player_meta for any row whose skill columns
// are all still zero. Idempotent: only updates rows that haven't been
// populated yet (via backfill or dual-write).
func backfillPlayerMetaSkillState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET combat_level   = (SELECT combat_level   FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    combat_xp      = (SELECT combat_xp      FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    mining_skill   = (SELECT mining_skill   FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    mining_xp      = (SELECT mining_xp      FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    foraging_skill = (SELECT foraging_skill FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    foraging_xp    = (SELECT foraging_xp    FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    fishing_skill  = (SELECT fishing_skill  FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    fishing_xp     = (SELECT fishing_xp     FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id)
		WHERE combat_level = 0 AND combat_xp = 0
		  AND mining_skill = 0 AND mining_xp = 0
		  AND foraging_skill = 0 AND foraging_xp = 0
		  AND fishing_skill = 0 AND fishing_xp = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters ac
			WHERE ac.user_id = player_meta.user_id
			  AND (ac.combat_level > 0 OR ac.mining_skill > 0 OR ac.foraging_skill > 0 OR ac.fishing_skill > 0
			       OR ac.combat_xp > 0 OR ac.mining_xp > 0 OR ac.foraging_xp > 0 OR ac.fishing_xp > 0)
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: skill state backfilled", "rows", n)
	return nil
}

// skillStateFromAdvChar projects the skill-related fields off an
// AdventureCharacter into a SkillState. Used by the dual-write path: every
// site that mutates AdvCharacter skill or XP fields then calls
// upsertPlayerMetaSkillState with this projection so the columns move
// cleanly during the L5a soak window.
func skillStateFromAdvChar(c *AdventureCharacter) SkillState {
	return SkillState{
		CombatLevel:   c.CombatLevel,
		CombatXP:      c.CombatXP,
		MiningSkill:   c.MiningSkill,
		MiningXP:      c.MiningXP,
		ForagingSkill: c.ForagingSkill,
		ForagingXP:    c.ForagingXP,
		FishingSkill:  c.FishingSkill,
		FishingXP:     c.FishingXP,
	}
}

// BabysitState is the in-memory mirror of player_meta's babysit columns.
// Phase L5b ports babysit state off AdvCharacter (gogobee_legacy_migration.md
// §7.3 L5b). Five fields: the active service (Active + ExpiresAt +
// SkillFocus — legacy slot, no longer mutated) and the auto-babysit
// preference (AutoBabysit + AutoBabysitFocus, dormant in current code but
// preserved for the schema migration).
type BabysitState struct {
	Active           bool
	ExpiresAt        *time.Time
	SkillFocus       string
	AutoBabysit      bool
	AutoBabysitFocus string
}

// IsActive returns true when the babysit service is on. Used as the "row
// is migrated" marker in loadBabysitState; an idle row is
// indistinguishable from a never-populated row, so it falls through to the
// legacy table during the soak window.
func (s BabysitState) IsActive() bool { return s.Active }

// upsertPlayerMetaBabysitState writes the full babysit column set for a
// user. Used by the dual-write path during the L5b soak window.
func upsertPlayerMetaBabysitState(userID id.UserID, s BabysitState) error {
	active := 0
	if s.Active {
		active = 1
	}
	auto := 0
	if s.AutoBabysit {
		auto = 1
	}
	var expires interface{}
	if s.ExpiresAt != nil {
		expires = s.ExpiresAt.UTC()
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, babysit_active, babysit_expires_at, babysit_skill_focus,
		   auto_babysit, auto_babysit_focus
		 ) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   babysit_active = excluded.babysit_active,
		   babysit_expires_at = excluded.babysit_expires_at,
		   babysit_skill_focus = excluded.babysit_skill_focus,
		   auto_babysit = excluded.auto_babysit,
		   auto_babysit_focus = excluded.auto_babysit_focus`,
		string(userID), active, expires, s.SkillFocus, auto, s.AutoBabysitFocus,
	)
	return err
}

// loadBabysitState returns the player's babysit state from player_meta
// when active, falling back to adventure_characters during the L5b soak
// window. An inactive row falls through to the legacy table since an
// inactive player_meta row is indistinguishable from an unmigrated one.
func loadBabysitState(userID id.UserID) (BabysitState, error) {
	var (
		s         BabysitState
		activeInt int
		autoInt   int
		expires   sql.NullTime
	)
	err := db.Get().QueryRow(
		`SELECT babysit_active, babysit_expires_at, babysit_skill_focus,
		        auto_babysit, auto_babysit_focus
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&activeInt, &expires, &s.SkillFocus, &autoInt, &s.AutoBabysitFocus)
	if err == nil && activeInt == 1 {
		s.Active = true
		s.AutoBabysit = autoInt == 1
		if expires.Valid {
			t := expires.Time.UTC()
			s.ExpiresAt = &t
		}
		return s, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return BabysitState{}, err
	}
	// Fallback to AdvCharacter during soak.
	var (
		legacyActive  int
		legacyExpires sql.NullTime
		legacyFocus   string
		legacyAuto    int
		legacyAutoFoc string
	)
	err = db.Get().QueryRow(
		`SELECT babysit_active, babysit_expires_at, babysit_skill_focus,
		        auto_babysit, auto_babysit_focus
		   FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(&legacyActive, &legacyExpires, &legacyFocus, &legacyAuto, &legacyAutoFoc)
	if err == sql.ErrNoRows {
		return BabysitState{}, nil
	}
	if err != nil {
		return BabysitState{}, err
	}
	out := BabysitState{
		Active:           legacyActive == 1,
		SkillFocus:       legacyFocus,
		AutoBabysit:      legacyAuto == 1,
		AutoBabysitFocus: legacyAutoFoc,
	}
	if legacyExpires.Valid {
		t := legacyExpires.Time.UTC()
		out.ExpiresAt = &t
	}
	return out, nil
}

// backfillPlayerMetaBabysitState copies the babysit columns from
// adventure_characters into player_meta for any row whose babysit_active
// is still 0 AND the legacy row has babysit_active = 1. Idempotent: only
// fills inactive rows whose legacy counterpart is active.
func backfillPlayerMetaBabysitState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET babysit_active      = (SELECT babysit_active      FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    babysit_expires_at  = (SELECT babysit_expires_at  FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    babysit_skill_focus = (SELECT babysit_skill_focus FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    auto_babysit        = (SELECT auto_babysit        FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    auto_babysit_focus  = (SELECT auto_babysit_focus  FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id)
		WHERE babysit_active = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters ac
			WHERE ac.user_id = player_meta.user_id
			  AND ac.babysit_active = 1
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: babysit state backfilled", "rows", n)
	return nil
}

// babysitStateFromAdvChar projects the babysit-related fields off an
// AdventureCharacter into a BabysitState. Used by the dual-write path
// during the L5b soak window.
func babysitStateFromAdvChar(c *AdventureCharacter) BabysitState {
	return BabysitState{
		Active:           c.BabysitActive,
		ExpiresAt:        c.BabysitExpiresAt,
		SkillFocus:       c.BabysitSkillFocus,
		AutoBabysit:      c.AutoBabysit,
		AutoBabysitFocus: c.AutoBabysitFocus,
	}
}

// NPCState mirrors player_meta's NPC counter/timestamp columns. Phase
// L5c ports Misty/Arina/Robbie/Thom counters + buff/debuff expiries off
// AdvCharacter (gogobee_legacy_migration.md §7.3 L5c). Hidden discovery
// mechanics — never surface in player-facing output.
type NPCState struct {
	MistyLastSeen       *time.Time
	ArinaLastSeen       *time.Time
	MistyBuffExpires    *time.Time
	MistyDebuffExpires  *time.Time
	ArinaBuffExpires    *time.Time
	NPCMsgCount         int
	NPCMsgCountDate     string
	MistyRollTarget     int
	ArinaRollTarget     int
	MistyEncounterCount int
	MistyDonatedCount   int
	ThomAnimalLineFired bool
	RobbieVisitCount    int
}

// HasNPCActivity returns true when any counter or timestamp is non-zero —
// the "row is migrated" marker for the loadNPCState fallback path.
func (s NPCState) HasNPCActivity() bool {
	return s.MistyLastSeen != nil || s.ArinaLastSeen != nil ||
		s.MistyBuffExpires != nil || s.MistyDebuffExpires != nil ||
		s.ArinaBuffExpires != nil ||
		s.NPCMsgCount > 0 || s.NPCMsgCountDate != "" ||
		s.MistyRollTarget > 0 || s.ArinaRollTarget > 0 ||
		s.MistyEncounterCount > 0 || s.MistyDonatedCount > 0 ||
		s.ThomAnimalLineFired || s.RobbieVisitCount > 0
}

// upsertPlayerMetaNPCState writes the full NPC column set for a user.
// Used by the dual-write path during the L5c soak window.
func upsertPlayerMetaNPCState(userID id.UserID, s NPCState) error {
	thom := 0
	if s.ThomAnimalLineFired {
		thom = 1
	}
	var (
		mistyLast, arinaLast, mistyBuff, mistyDebuff, arinaBuff interface{}
	)
	if s.MistyLastSeen != nil {
		mistyLast = s.MistyLastSeen.UTC()
	}
	if s.ArinaLastSeen != nil {
		arinaLast = s.ArinaLastSeen.UTC()
	}
	if s.MistyBuffExpires != nil {
		mistyBuff = s.MistyBuffExpires.UTC()
	}
	if s.MistyDebuffExpires != nil {
		mistyDebuff = s.MistyDebuffExpires.UTC()
	}
	if s.ArinaBuffExpires != nil {
		arinaBuff = s.ArinaBuffExpires.UTC()
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, misty_last_seen, arina_last_seen,
		   misty_buff_expires, misty_debuff_expires, arina_buff_expires,
		   npc_msg_count, npc_msg_count_date,
		   misty_roll_target, arina_roll_target,
		   misty_encounter_count, misty_donated_count,
		   thom_animal_line_fired, robbie_visit_count
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   misty_last_seen = excluded.misty_last_seen,
		   arina_last_seen = excluded.arina_last_seen,
		   misty_buff_expires = excluded.misty_buff_expires,
		   misty_debuff_expires = excluded.misty_debuff_expires,
		   arina_buff_expires = excluded.arina_buff_expires,
		   npc_msg_count = excluded.npc_msg_count,
		   npc_msg_count_date = excluded.npc_msg_count_date,
		   misty_roll_target = excluded.misty_roll_target,
		   arina_roll_target = excluded.arina_roll_target,
		   misty_encounter_count = excluded.misty_encounter_count,
		   misty_donated_count = excluded.misty_donated_count,
		   thom_animal_line_fired = excluded.thom_animal_line_fired,
		   robbie_visit_count = excluded.robbie_visit_count`,
		string(userID), mistyLast, arinaLast, mistyBuff, mistyDebuff, arinaBuff,
		s.NPCMsgCount, s.NPCMsgCountDate,
		s.MistyRollTarget, s.ArinaRollTarget,
		s.MistyEncounterCount, s.MistyDonatedCount,
		thom, s.RobbieVisitCount,
	)
	return err
}

// loadNPCState returns the NPC state from player_meta when populated,
// otherwise falls back to adventure_characters during the L5c soak window.
func loadNPCState(userID id.UserID) (NPCState, error) {
	var (
		s                                                       NPCState
		mistyLast, arinaLast, mistyBuff, mistyDebuff, arinaBuff sql.NullTime
		thomInt                                                 int
	)
	err := db.Get().QueryRow(
		`SELECT misty_last_seen, arina_last_seen,
		        misty_buff_expires, misty_debuff_expires, arina_buff_expires,
		        npc_msg_count, npc_msg_count_date,
		        misty_roll_target, arina_roll_target,
		        misty_encounter_count, misty_donated_count,
		        thom_animal_line_fired, robbie_visit_count
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(
		&mistyLast, &arinaLast, &mistyBuff, &mistyDebuff, &arinaBuff,
		&s.NPCMsgCount, &s.NPCMsgCountDate,
		&s.MistyRollTarget, &s.ArinaRollTarget,
		&s.MistyEncounterCount, &s.MistyDonatedCount,
		&thomInt, &s.RobbieVisitCount,
	)
	if err == nil {
		s.ThomAnimalLineFired = thomInt == 1
		if mistyLast.Valid {
			t := mistyLast.Time.UTC()
			s.MistyLastSeen = &t
		}
		if arinaLast.Valid {
			t := arinaLast.Time.UTC()
			s.ArinaLastSeen = &t
		}
		if mistyBuff.Valid {
			t := mistyBuff.Time.UTC()
			s.MistyBuffExpires = &t
		}
		if mistyDebuff.Valid {
			t := mistyDebuff.Time.UTC()
			s.MistyDebuffExpires = &t
		}
		if arinaBuff.Valid {
			t := arinaBuff.Time.UTC()
			s.ArinaBuffExpires = &t
		}
		if s.HasNPCActivity() {
			return s, nil
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return NPCState{}, err
	}
	// Fallback to AdvCharacter during soak.
	var (
		legacy                                                                    NPCState
		legacyMisty, legacyArinaLast, legacyMBuff, legacyMDebuff, legacyABuff     sql.NullTime
		legacyThom                                                                int
	)
	err = db.Get().QueryRow(
		`SELECT misty_last_seen, arina_last_seen,
		        misty_buff_expires, misty_debuff_expires, arina_buff_expires,
		        npc_msg_count, npc_msg_count_date,
		        misty_roll_target, arina_roll_target,
		        misty_encounter_count, misty_donated_count,
		        thom_animal_line_fired, robbie_visit_count
		   FROM adventure_characters WHERE user_id = ?`,
		string(userID),
	).Scan(
		&legacyMisty, &legacyArinaLast, &legacyMBuff, &legacyMDebuff, &legacyABuff,
		&legacy.NPCMsgCount, &legacy.NPCMsgCountDate,
		&legacy.MistyRollTarget, &legacy.ArinaRollTarget,
		&legacy.MistyEncounterCount, &legacy.MistyDonatedCount,
		&legacyThom, &legacy.RobbieVisitCount,
	)
	if err == sql.ErrNoRows {
		return NPCState{}, nil
	}
	if err != nil {
		return NPCState{}, err
	}
	legacy.ThomAnimalLineFired = legacyThom == 1
	if legacyMisty.Valid {
		t := legacyMisty.Time.UTC()
		legacy.MistyLastSeen = &t
	}
	if legacyArinaLast.Valid {
		t := legacyArinaLast.Time.UTC()
		legacy.ArinaLastSeen = &t
	}
	if legacyMBuff.Valid {
		t := legacyMBuff.Time.UTC()
		legacy.MistyBuffExpires = &t
	}
	if legacyMDebuff.Valid {
		t := legacyMDebuff.Time.UTC()
		legacy.MistyDebuffExpires = &t
	}
	if legacyABuff.Valid {
		t := legacyABuff.Time.UTC()
		legacy.ArinaBuffExpires = &t
	}
	return legacy, nil
}

// backfillPlayerMetaNPCState copies NPC columns from adventure_characters
// into player_meta for any row where every NPC field is still zero AND
// the legacy row has any non-zero NPC field. Idempotent.
func backfillPlayerMetaNPCState() error {
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET misty_last_seen        = (SELECT misty_last_seen        FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    arina_last_seen        = (SELECT arina_last_seen        FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    misty_buff_expires     = (SELECT misty_buff_expires     FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    misty_debuff_expires   = (SELECT misty_debuff_expires   FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    arina_buff_expires     = (SELECT arina_buff_expires     FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    npc_msg_count          = (SELECT npc_msg_count          FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    npc_msg_count_date     = (SELECT npc_msg_count_date     FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    misty_roll_target      = (SELECT misty_roll_target      FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    arina_roll_target      = (SELECT arina_roll_target      FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    misty_encounter_count  = (SELECT misty_encounter_count  FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    misty_donated_count    = (SELECT misty_donated_count    FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    thom_animal_line_fired = (SELECT thom_animal_line_fired FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id),
		    robbie_visit_count     = (SELECT robbie_visit_count     FROM adventure_characters ac WHERE ac.user_id = player_meta.user_id)
		WHERE misty_last_seen IS NULL AND arina_last_seen IS NULL
		  AND misty_buff_expires IS NULL AND misty_debuff_expires IS NULL
		  AND arina_buff_expires IS NULL
		  AND npc_msg_count = 0 AND npc_msg_count_date = ''
		  AND misty_roll_target = 0 AND arina_roll_target = 0
		  AND misty_encounter_count = 0 AND misty_donated_count = 0
		  AND thom_animal_line_fired = 0 AND robbie_visit_count = 0
		  AND EXISTS (
			SELECT 1 FROM adventure_characters ac
			WHERE ac.user_id = player_meta.user_id
			  AND (ac.misty_last_seen IS NOT NULL OR ac.arina_last_seen IS NOT NULL
			       OR ac.misty_buff_expires IS NOT NULL OR ac.misty_debuff_expires IS NOT NULL
			       OR ac.arina_buff_expires IS NOT NULL
			       OR ac.npc_msg_count > 0 OR ac.npc_msg_count_date <> ''
			       OR ac.misty_roll_target > 0 OR ac.arina_roll_target > 0
			       OR ac.misty_encounter_count > 0 OR ac.misty_donated_count > 0
			       OR ac.thom_animal_line_fired = 1 OR ac.robbie_visit_count > 0)
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: NPC state backfilled", "rows", n)
	return nil
}

// npcStateFromAdvChar projects NPC fields off an AdventureCharacter into
// an NPCState. Used by the dual-write path during L5c soak.
func npcStateFromAdvChar(c *AdventureCharacter) NPCState {
	return NPCState{
		MistyLastSeen:       c.MistyLastSeen,
		ArinaLastSeen:       c.ArinaLastSeen,
		MistyBuffExpires:    c.MistyBuffExpires,
		MistyDebuffExpires:  c.MistyDebuffExpires,
		ArinaBuffExpires:    c.ArinaBuffExpires,
		NPCMsgCount:         c.NPCMsgCount,
		NPCMsgCountDate:     c.NPCMsgCountDate,
		MistyRollTarget:     c.MistyRollTarget,
		ArinaRollTarget:     c.ArinaRollTarget,
		MistyEncounterCount: c.MistyEncounterCount,
		MistyDonatedCount:   c.MistyDonatedCount,
		ThomAnimalLineFired: c.ThomAnimalLineFired,
		RobbieVisitCount:    c.RobbieVisitCount,
	}
}

// backfillPlayerMetaDisplayName copies adventure_characters.display_name
// into player_meta.display_name for any row whose display_name is still
// the empty default. Idempotent: safe to re-run; only updates rows that
// haven't been populated yet (either by backfill or the dual-write path).
func backfillPlayerMetaDisplayName() error {
	// Ensure a player_meta row exists for every adventure_characters user
	// (arena backfill already does this, but display_name backfill must
	// not assume order — re-run cheaply via INSERT OR IGNORE).
	if _, err := db.Get().Exec(`
		INSERT OR IGNORE INTO player_meta (user_id)
		SELECT user_id FROM adventure_characters
	`); err != nil {
		return err
	}
	res, err := db.Get().Exec(`
		UPDATE player_meta
		SET display_name = (
			SELECT display_name FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
		)
		WHERE display_name = ''
		  AND EXISTS (
			SELECT 1 FROM adventure_characters
			WHERE adventure_characters.user_id = player_meta.user_id
			  AND adventure_characters.display_name <> ''
		  )
	`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("player_meta: display_name backfilled", "rows", n)
	return nil
}
