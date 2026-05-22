package plugin

import (
	"database/sql"
	"encoding/json"
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

// loadDisplayName returns the player's display name from player_meta.
// Empty string if the user has no row.
func loadDisplayName(userID id.UserID) (string, error) {
	var name string
	err := db.Get().QueryRow(
		`SELECT display_name FROM player_meta WHERE user_id = ?`,
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
// player_meta. Zero if the user has no row.
func loadHospitalVisits(userID id.UserID) (int, error) {
	var visits int
	err := db.Get().QueryRow(
		`SELECT hospital_visits FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&visits)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return visits, err
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

// loadRivalState returns rival_pool / rival_unlocked_notified for a user
// from player_meta. (0, false) if the user has no row.
func loadRivalState(userID id.UserID) (pool int, notified bool, err error) {
	var notifiedInt int
	err = db.Get().QueryRow(
		`SELECT rival_pool, rival_unlocked_notified FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&pool, &notifiedInt)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return pool, notifiedInt == 1, nil
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
// player_meta. Zero if the user has no row.
func loadMasterworkDrops(userID id.UserID) (int, error) {
	var drops int
	err := db.Get().QueryRow(
		`SELECT masterwork_drops_received FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&drops)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return drops, err
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

// loadPetState returns the player's pet state from player_meta. Empty
// PetState{} if the user has no row.
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
	if err == sql.ErrNoRows {
		return PetState{}, nil
	}
	if err != nil {
		return PetState{}, err
	}
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

// loadHouseState returns the player's house state from player_meta. Empty
// HouseState{} if the user has no row.
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
	if err == sql.ErrNoRows {
		return HouseState{}, nil
	}
	if err != nil {
		return HouseState{}, err
	}
	s.LoanFrozen = frozenInt == 1
	s.Autopay = autopayInt == 1
	return s, nil
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

// HasSkills returns true when any skill or XP field is non-zero.
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

// loadSkillState returns the player's skill state from player_meta.
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
	if err == sql.ErrNoRows {
		return SkillState{}, nil
	}
	return s, err
}

// skillStateFromAdvChar projects the skill-related fields off an
// AdventureCharacter into a SkillState. Used by saveAdvCharacter's fan-out
// to mirror skill columns into player_meta.
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

// loadBabysitState returns the player's babysit state from player_meta.
// Empty BabysitState{} if the user has no row.
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
	if err == sql.ErrNoRows {
		return BabysitState{}, nil
	}
	if err != nil {
		return BabysitState{}, err
	}
	s.Active = activeInt == 1
	s.AutoBabysit = autoInt == 1
	if expires.Valid {
		t := expires.Time.UTC()
		s.ExpiresAt = &t
	}
	return s, nil
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

// loadNPCState returns the NPC state from player_meta.
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
	if err == sql.ErrNoRows {
		return NPCState{}, nil
	}
	if err != nil {
		return NPCState{}, err
	}
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
	return s, nil
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

// LifecycleState mirrors player_meta's streak/action/lifecycle columns.
// Phase L5d ports these fields off AdvCharacter (gogobee_legacy_migration.md
// §7.3 L5d). Streak fields update on the daily morning DM tick; action
// flags reset by `resetAllAdvDailyActions` and only flip true when the
// player rests; lifecycle timestamps are set at character creation and
// auto-bumped by every saveAdvCharacter via CURRENT_TIMESTAMP.
type LifecycleState struct {
	CurrentStreak      int
	BestStreak         int
	LastActionDate     string
	StreakDecayed      bool
	ActionTakenToday   bool
	HolidayActionTaken bool
	CombatActionsUsed  int
	HarvestActionsUsed int
	CreatedAt          *time.Time
	LastActiveAt       *time.Time
}

// HasLifecycle returns true when any lifecycle field is non-zero — used
// as the "row is migrated" marker in loadLifecycleState. CreatedAt is set
// at character creation, so any migrated row will have it.
func (s LifecycleState) HasLifecycle() bool {
	return s.CreatedAt != nil || s.LastActiveAt != nil ||
		s.CurrentStreak > 0 || s.BestStreak > 0 ||
		s.LastActionDate != "" || s.StreakDecayed ||
		s.ActionTakenToday || s.HolidayActionTaken ||
		s.CombatActionsUsed > 0 || s.HarvestActionsUsed > 0
}

// upsertPlayerMetaLifecycleState writes the full lifecycle column set for
// a user. Used by the dual-write path during the L5d soak window.
// CreatedAt and LastActiveAt are written when non-nil; a nil value leaves
// the column NULL.
func upsertPlayerMetaLifecycleState(userID id.UserID, s LifecycleState) error {
	streakDecayed := 0
	if s.StreakDecayed {
		streakDecayed = 1
	}
	actionTaken := 0
	if s.ActionTakenToday {
		actionTaken = 1
	}
	holidayTaken := 0
	if s.HolidayActionTaken {
		holidayTaken = 1
	}
	var createdAt, lastActiveAt interface{}
	if s.CreatedAt != nil {
		createdAt = s.CreatedAt.UTC()
	}
	if s.LastActiveAt != nil {
		lastActiveAt = s.LastActiveAt.UTC()
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, current_streak, best_streak, last_action_date, streak_decayed,
		   action_taken_today, holiday_action_taken,
		   combat_actions_used, harvest_actions_used,
		   created_at, last_active_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   current_streak = excluded.current_streak,
		   best_streak = excluded.best_streak,
		   last_action_date = excluded.last_action_date,
		   streak_decayed = excluded.streak_decayed,
		   action_taken_today = excluded.action_taken_today,
		   holiday_action_taken = excluded.holiday_action_taken,
		   combat_actions_used = excluded.combat_actions_used,
		   harvest_actions_used = excluded.harvest_actions_used,
		   created_at = excluded.created_at,
		   last_active_at = excluded.last_active_at`,
		string(userID), s.CurrentStreak, s.BestStreak, s.LastActionDate, streakDecayed,
		actionTaken, holidayTaken,
		s.CombatActionsUsed, s.HarvestActionsUsed,
		createdAt, lastActiveAt,
	)
	return err
}

// loadLifecycleState returns lifecycle state from player_meta.
func loadLifecycleState(userID id.UserID) (LifecycleState, error) {
	var (
		s                                 LifecycleState
		streakDecayed, actionTaken, holidayTaken int
		createdAt, lastActiveAt           sql.NullTime
	)
	err := db.Get().QueryRow(
		`SELECT current_streak, best_streak, last_action_date, streak_decayed,
		        action_taken_today, holiday_action_taken,
		        combat_actions_used, harvest_actions_used,
		        created_at, last_active_at
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(
		&s.CurrentStreak, &s.BestStreak, &s.LastActionDate, &streakDecayed,
		&actionTaken, &holidayTaken,
		&s.CombatActionsUsed, &s.HarvestActionsUsed,
		&createdAt, &lastActiveAt,
	)
	if err == sql.ErrNoRows {
		return LifecycleState{}, nil
	}
	if err != nil {
		return LifecycleState{}, err
	}
	s.StreakDecayed = streakDecayed == 1
	s.ActionTakenToday = actionTaken == 1
	s.HolidayActionTaken = holidayTaken == 1
	if createdAt.Valid {
		t := createdAt.Time.UTC()
		s.CreatedAt = &t
	}
	if lastActiveAt.Valid {
		t := lastActiveAt.Time.UTC()
		s.LastActiveAt = &t
	}
	return s, nil
}

// lifecycleStateFromAdvChar projects lifecycle fields off an
// AdventureCharacter into a LifecycleState. Used by the dual-write path
// during the L5d soak window.
func lifecycleStateFromAdvChar(c *AdventureCharacter) LifecycleState {
	out := LifecycleState{
		CurrentStreak:      c.CurrentStreak,
		BestStreak:         c.BestStreak,
		LastActionDate:     c.LastActionDate,
		StreakDecayed:      c.StreakDecayed,
		ActionTakenToday:   c.ActionTakenToday,
		HolidayActionTaken: c.HolidayActionTaken,
		CombatActionsUsed:  c.CombatActionsUsed,
		HarvestActionsUsed: c.HarvestActionsUsed,
	}
	if !c.CreatedAt.IsZero() {
		t := c.CreatedAt.UTC()
		out.CreatedAt = &t
	}
	if !c.LastActiveAt.IsZero() {
		t := c.LastActiveAt.UTC()
		out.LastActiveAt = &t
	}
	return out
}

// resetAllPlayerMetaDailyActions parallels resetAllAdvDailyActions:
// resets the action_taken_today / holiday_action_taken /
// combat_actions_used / harvest_actions_used columns in player_meta for
// any row whose last_action_date is older than today (or NULL/empty).
// Used by the daily reset tick during the L5d soak window.
func resetAllPlayerMetaDailyActions() error {
	today := time.Now().UTC().Format("2006-01-02")
	_, err := db.Get().Exec(`
		UPDATE player_meta
		SET action_taken_today = 0,
		    holiday_action_taken = 0,
		    combat_actions_used = 0,
		    harvest_actions_used = 0
		WHERE last_action_date < ? OR last_action_date = ''`, today)
	return err
}

// resetAllPetMorningDefense clears the one-day pet morning-defense buff for
// every player. The buff is a fresh morning grant (25% roll in the briefing /
// overworld DM), so it must not survive past the midnight rollover. The flag
// lives inside pet_flags_json (see petFlagsJSON), so patch that key in place;
// only rows where it's currently set are touched.
func resetAllPetMorningDefense() error {
	_, err := db.Get().Exec(`
		UPDATE player_meta
		   SET pet_flags_json = json_set(pet_flags_json, '$.morning_defense', json('false'))
		 WHERE json_extract(pet_flags_json, '$.morning_defense') = 1`)
	return err
}

// DeathState mirrors player_meta's death-state columns. Phase L5e ports
// these fields off AdvCharacter (gogobee_legacy_migration.md §7.3 L5e).
// Mutation surface is large (~50 saveAdvCharacter sites touch death
// state), so the dual-write pattern switches to inside saveAdvCharacter
// itself rather than per-site upserts.
type DeathState struct {
	Alive             bool
	DeadUntil         *time.Time
	DeathReprieveLast *time.Time
	LastDeathDate     string
	LastPardonUsed    *time.Time
	GrudgeLocation    string
	DeathSource       string
	DeathLocation     string
}

// upsertPlayerMetaDeathState writes the full death column set for a user.
// Called from inside saveAdvCharacter during the L5e soak window.
func upsertPlayerMetaDeathState(userID id.UserID, s DeathState) error {
	alive := 0
	if s.Alive {
		alive = 1
	}
	var deadUntil, reprieve, pardon interface{}
	if s.DeadUntil != nil {
		deadUntil = s.DeadUntil.UTC()
	}
	if s.DeathReprieveLast != nil {
		reprieve = s.DeathReprieveLast.UTC()
	}
	if s.LastPardonUsed != nil {
		pardon = s.LastPardonUsed.UTC()
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (
		   user_id, alive, dead_until, death_reprieve_last,
		   last_death_date, last_pardon_used,
		   grudge_location, death_source, death_location
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   alive = excluded.alive,
		   dead_until = excluded.dead_until,
		   death_reprieve_last = excluded.death_reprieve_last,
		   last_death_date = excluded.last_death_date,
		   last_pardon_used = excluded.last_pardon_used,
		   grudge_location = excluded.grudge_location,
		   death_source = excluded.death_source,
		   death_location = excluded.death_location`,
		string(userID), alive, deadUntil, reprieve,
		s.LastDeathDate, pardon,
		s.GrudgeLocation, s.DeathSource, s.DeathLocation,
	)
	return err
}

// loadDeathState returns death state from player_meta. Default
// DeathState{Alive: true} if the user has no row.
func loadDeathState(userID id.UserID) (DeathState, error) {
	var (
		s                           DeathState
		aliveInt                    int
		deadUntil, reprieve, pardon sql.NullTime
	)
	err := db.Get().QueryRow(
		`SELECT alive, dead_until, death_reprieve_last,
		        last_death_date, last_pardon_used,
		        grudge_location, death_source, death_location
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(
		&aliveInt, &deadUntil, &reprieve,
		&s.LastDeathDate, &pardon,
		&s.GrudgeLocation, &s.DeathSource, &s.DeathLocation,
	)
	if err == sql.ErrNoRows {
		return DeathState{Alive: true}, nil
	}
	if err != nil {
		return DeathState{}, err
	}
	s.Alive = aliveInt == 1
	if deadUntil.Valid {
		t := deadUntil.Time.UTC()
		s.DeadUntil = &t
	}
	if reprieve.Valid {
		t := reprieve.Time.UTC()
		s.DeathReprieveLast = &t
	}
	if pardon.Valid {
		t := pardon.Time.UTC()
		s.LastPardonUsed = &t
	}
	return s, nil
}

// deathStateFromAdvChar projects death fields off an AdventureCharacter.
func deathStateFromAdvChar(c *AdventureCharacter) DeathState {
	return DeathState{
		Alive:             c.Alive,
		DeadUntil:         c.DeadUntil,
		DeathReprieveLast: c.DeathReprieveLast,
		LastDeathDate:     c.LastDeathDate,
		LastPardonUsed:    c.LastPardonUsed,
		GrudgeLocation:    c.GrudgeLocation,
		DeathSource:       c.DeathSource,
		DeathLocation:     c.DeathLocation,
	}
}

// MiscState mirrors player_meta's miscellaneous columns: Title,
// TreasuresLocked, CraftsSucceeded. Phase L5f ports these fields off
// AdvCharacter (gogobee_legacy_migration.md §7.3 L5f). Dual-write rides
// `saveAdvCharacter` alongside the L5e death state, since these fields
// either rarely mutate or already mutate at every save site.
type MiscState struct {
	Title            string
	TreasuresLocked  bool
	CraftsSucceeded  int
}

func upsertPlayerMetaMiscState(userID id.UserID, s MiscState) error {
	locked := 0
	if s.TreasuresLocked {
		locked = 1
	}
	_, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, title, treasures_locked, crafts_succeeded)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   title = excluded.title,
		   treasures_locked = excluded.treasures_locked,
		   crafts_succeeded = excluded.crafts_succeeded`,
		string(userID), s.Title, locked, s.CraftsSucceeded,
	)
	return err
}

func loadMiscState(userID id.UserID) (MiscState, error) {
	var (
		s         MiscState
		lockedInt int
	)
	err := db.Get().QueryRow(
		`SELECT title, treasures_locked, crafts_succeeded
		   FROM player_meta WHERE user_id = ?`,
		string(userID),
	).Scan(&s.Title, &lockedInt, &s.CraftsSucceeded)
	if err == sql.ErrNoRows {
		return MiscState{}, nil
	}
	if err != nil {
		return MiscState{}, err
	}
	s.TreasuresLocked = lockedInt == 1
	return s, nil
}

func miscStateFromAdvChar(c *AdventureCharacter) MiscState {
	return MiscState{
		Title:           c.Title,
		TreasuresLocked: c.TreasuresLocked,
		CraftsSucceeded: c.CraftsSucceeded,
	}
}

// applyPlayerMetaOverlay populates an AdventureCharacter from player_meta.
// Post-L5 close-out: loadAdvCharacter no longer SELECTs from
// adventure_characters — it default-inits the struct and overlays every
// migrated subsystem's value from player_meta here. Errors are skipped — a
// partial overlay is safer than a failed load (each field falls back to its
// Go zero value, except Alive which is pre-set true).
func applyPlayerMetaOverlay(c *AdventureCharacter) {
	if c == nil {
		return
	}
	uid := c.UserID

	if name, err := loadDisplayName(uid); err == nil && name != "" {
		c.DisplayName = name
	}
	if visits, err := loadHospitalVisits(uid); err == nil {
		c.HospitalVisits = visits
	}
	if pm, err := loadPlayerMeta(uid); err == nil && pm != nil {
		c.ArenaWins = pm.ArenaWins
		c.ArenaLosses = pm.ArenaLosses
		c.InvasionScore = pm.InvasionScore
	}
	if drops, err := loadMasterworkDrops(uid); err == nil {
		c.MasterworkDropsReceived = drops
	}
	if pool, notified, err := loadRivalState(uid); err == nil {
		c.RivalPool = pool
		c.RivalUnlockedNotified = notified
	}
	if s, err := loadSkillState(uid); err == nil {
		c.CombatLevel = s.CombatLevel
		c.CombatXP = s.CombatXP
		c.MiningSkill = s.MiningSkill
		c.MiningXP = s.MiningXP
		c.ForagingSkill = s.ForagingSkill
		c.ForagingXP = s.ForagingXP
		c.FishingSkill = s.FishingSkill
		c.FishingXP = s.FishingXP
	}
	if s, err := loadBabysitState(uid); err == nil {
		c.BabysitActive = s.Active
		c.BabysitExpiresAt = s.ExpiresAt
		c.BabysitSkillFocus = s.SkillFocus
		c.AutoBabysit = s.AutoBabysit
		c.AutoBabysitFocus = s.AutoBabysitFocus
	}
	if s, err := loadNPCState(uid); err == nil {
		c.MistyLastSeen = s.MistyLastSeen
		c.ArinaLastSeen = s.ArinaLastSeen
		c.MistyBuffExpires = s.MistyBuffExpires
		c.MistyDebuffExpires = s.MistyDebuffExpires
		c.ArinaBuffExpires = s.ArinaBuffExpires
		c.NPCMsgCount = s.NPCMsgCount
		c.NPCMsgCountDate = s.NPCMsgCountDate
		c.MistyRollTarget = s.MistyRollTarget
		c.ArinaRollTarget = s.ArinaRollTarget
		c.MistyEncounterCount = s.MistyEncounterCount
		c.MistyDonatedCount = s.MistyDonatedCount
		c.ThomAnimalLineFired = s.ThomAnimalLineFired
		c.RobbieVisitCount = s.RobbieVisitCount
	}
	if s, err := loadLifecycleState(uid); err == nil {
		c.CurrentStreak = s.CurrentStreak
		c.BestStreak = s.BestStreak
		c.LastActionDate = s.LastActionDate
		c.StreakDecayed = s.StreakDecayed
		c.ActionTakenToday = s.ActionTakenToday
		c.HolidayActionTaken = s.HolidayActionTaken
		c.CombatActionsUsed = s.CombatActionsUsed
		c.HarvestActionsUsed = s.HarvestActionsUsed
		if s.CreatedAt != nil {
			c.CreatedAt = *s.CreatedAt
		}
		if s.LastActiveAt != nil {
			c.LastActiveAt = *s.LastActiveAt
		}
	}
	if s, err := loadDeathState(uid); err == nil {
		c.Alive = s.Alive
		c.DeadUntil = s.DeadUntil
		c.DeathReprieveLast = s.DeathReprieveLast
		c.LastDeathDate = s.LastDeathDate
		c.LastPardonUsed = s.LastPardonUsed
		c.GrudgeLocation = s.GrudgeLocation
		c.DeathSource = s.DeathSource
		c.DeathLocation = s.DeathLocation
	}
	if s, err := loadMiscState(uid); err == nil {
		c.Title = s.Title
		c.TreasuresLocked = s.TreasuresLocked
		c.CraftsSucceeded = s.CraftsSucceeded
	}
	if s, err := loadPetState(uid); err == nil {
		c.PetType = s.Type
		c.PetName = s.Name
		c.PetXP = s.XP
		c.PetLevel = s.Level
		c.PetArmorTier = s.ArmorTier
		c.PetSupplyShopUnlocked = s.SupplyShopUnlocked
		c.PetLevel10Date = s.Level10Date
		c.PetArrived = s.Arrived
		c.PetChasedAway = s.ChasedAway
		c.PetReactivated = s.Reactivated
		c.PetMorningDefense = s.MorningDefense
	}
	if s, err := loadHouseState(uid); err == nil {
		c.HouseTier = s.Tier
		c.HouseLoanBalance = s.LoanBalance
		c.HouseLoanFrozen = s.LoanFrozen
		c.HouseMissedPayments = s.MissedPayments
		c.HouseAutopay = s.Autopay
		c.HouseCurrentRate = s.CurrentRate
	}
}

// upsertAllPlayerMetaFromAdvChar writes every player_meta-mirrored subsystem's
// state in one shot from an AdventureCharacter. Phase L5h: saveAdvCharacter
// stops writing adventure_characters and instead routes through this fan-out,
// keeping every load helper's read path canonical against player_meta. Returns
// the first error encountered; partial writes are possible and acceptable
// since each upsert is its own atomic transaction.
func upsertAllPlayerMetaFromAdvChar(c *AdventureCharacter) error {
	uid := c.UserID
	if err := upsertPlayerMetaDisplayName(uid, c.DisplayName); err != nil {
		return err
	}
	if err := upsertPlayerMetaHospitalVisits(uid, c.HospitalVisits); err != nil {
		return err
	}
	if err := upsertPlayerMetaArena(uid, c.ArenaWins, c.ArenaLosses, c.InvasionScore); err != nil {
		return err
	}
	if err := upsertPlayerMetaMasterworkDrops(uid, c.MasterworkDropsReceived); err != nil {
		return err
	}
	if err := upsertPlayerMetaRivalState(uid, c.RivalPool, c.RivalUnlockedNotified); err != nil {
		return err
	}
	if err := upsertPlayerMetaSkillState(uid, skillStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaBabysitState(uid, babysitStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaNPCState(uid, npcStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaLifecycleState(uid, lifecycleStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaDeathState(uid, deathStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaMiscState(uid, miscStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaPetState(uid, petStateFromAdvChar(c)); err != nil {
		return err
	}
	if err := upsertPlayerMetaHouseState(uid, houseStateFromAdvChar(c)); err != nil {
		return err
	}
	return nil
}
