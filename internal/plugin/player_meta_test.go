package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase L2 step 5 — backfill idempotency. Mirrors the R1 archiveOrphanZoneRuns
// test pattern: run twice, second run is a no-op (INSERT OR IGNORE), and
// dual-writes layered after the backfill don't get clobbered.

func TestUpsertPlayerMetaArena_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-rt:example")
	if err := upsertPlayerMetaArena(uid, 4, 2, 0); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	m, err := loadPlayerMeta(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.ArenaWins != 4 || m.ArenaLosses != 2 {
		t.Errorf("round-trip 1: got %+v want wins=4 losses=2", m)
	}

	// Update path
	if err := upsertPlayerMetaArena(uid, 5, 2, 0); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	m2, _ := loadPlayerMeta(uid)
	if m2.ArenaWins != 5 {
		t.Errorf("round-trip 2: got wins=%d want 5", m2.ArenaWins)
	}

	// Missing user → zero-valued struct, no error
	mZ, err := loadPlayerMeta(id.UserID("@meta-rt-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if mZ.ArenaWins != 0 || mZ.ArenaLosses != 0 {
		t.Errorf("missing user not zero: %+v", mZ)
	}
}

// Phase L4f-prep — DisplayName migration. Backfill is idempotent and only
// touches empty rows; dual-write through createAdvCharacter / saveAdvCharacter
// stays consistent across re-runs; loadDisplayName falls back to AdvCharacter
// when player_meta hasn't been populated yet.

// Phase L4a — Hospital migration. Backfill is idempotent and only touches
// zero-valued rows; the upsert helper round-trips; loadHospitalVisits falls
// back to AdvCharacter when player_meta hasn't been populated yet.

func TestUpsertPlayerMetaHospitalVisits_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-hv-rt:example")
	if err := upsertPlayerMetaHospitalVisits(uid, 2); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadHospitalVisits(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 2 {
		t.Errorf("round-trip: got %d want 2", got)
	}
	if err := upsertPlayerMetaHospitalVisits(uid, 7); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := loadHospitalVisits(uid)
	if got2 != 7 {
		t.Errorf("update: got %d want 7", got2)
	}
}

// Phase L4b — Rival migration. Backfill is idempotent and only fills rows
// whose rival columns are still default zero; the upsert helper round-trips;
// loadRivalState falls back to AdvCharacter when player_meta hasn't been
// populated yet.

func TestUpsertPlayerMetaRivalState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-rv-rt:example")
	if err := upsertPlayerMetaRivalState(uid, 1, false); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	pool, notified, err := loadRivalState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pool != 1 || notified {
		t.Errorf("round-trip 1: got pool=%d notified=%v want 1, false", pool, notified)
	}
	if err := upsertPlayerMetaRivalState(uid, 1, true); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	pool2, notified2, _ := loadRivalState(uid)
	if pool2 != 1 || !notified2 {
		t.Errorf("round-trip 2: got pool=%d notified=%v want 1, true", pool2, notified2)
	}
}

func TestCreateAdvCharacter_DualWritesDisplayName(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-dn-create:example")
	if err := createAdvCharacter(uid, "FreshName"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	got, err := loadDisplayName(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != "FreshName" {
		t.Errorf("create dual-write: got %q want %q", got, "FreshName")
	}
}

// Phase L4c — Masterwork migration. Backfill is idempotent and only touches
// zero-valued rows; the upsert helper round-trips; loadMasterworkDrops falls
// back to AdvCharacter when player_meta hasn't been populated yet.

func TestUpsertPlayerMetaMasterworkDrops_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-mw-rt:example")
	if err := upsertPlayerMetaMasterworkDrops(uid, 1); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadMasterworkDrops(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 1 {
		t.Errorf("round-trip: got %d want 1", got)
	}
	if err := upsertPlayerMetaMasterworkDrops(uid, 9); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := loadMasterworkDrops(uid)
	if got2 != 9 {
		t.Errorf("update: got %d want 9", got2)
	}
}

// Phase L4d — Pet state migration. Backfill is idempotent and only touches
// rows with empty pet_type; the upsert helper round-trips the full PetState
// (including the four flag bools encoded into pet_flags_json); loadPetState
// falls back to AdvCharacter when player_meta hasn't been populated yet.

func TestUpsertPlayerMetaPetState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-pet-rt:example")
	in := PetState{
		Type:               "dog",
		Name:               "Boon",
		XP:                 1234,
		Level:              7,
		ArmorTier:          3,
		SupplyShopUnlocked: true,
		Level10Date:        "2026-05-09",
		Arrived:            true,
		ChasedAway:         true,
		Reactivated:        true,
		MorningDefense:     true,
	}
	if err := upsertPlayerMetaPetState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadPetState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != in {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, in)
	}

	// Mutating one field updates only that field via the same upsert.
	in.Level = 8
	in.XP = 0
	in.MorningDefense = false
	if err := upsertPlayerMetaPetState(uid, in); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := loadPetState(uid)
	if got2 != in {
		t.Errorf("update mismatch:\n got=%+v\nwant=%+v", got2, in)
	}
}

// Phase L4e — House state migration. Backfill is idempotent and only touches
// rows that haven't been migrated yet (tier=0 AND loan_balance=0); the upsert
// helper round-trips the full HouseState; loadHouseState falls back to
// AdvCharacter when player_meta hasn't been populated yet.

func TestUpsertPlayerMetaSkillState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-skill-rt:example")
	in := SkillState{CombatLevel: 12, CombatXP: 450, MiningSkill: 8, MiningXP: 200, ForagingSkill: 6, ForagingXP: 150, FishingSkill: 5, FishingXP: 90}
	if err := upsertPlayerMetaSkillState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadSkillState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != in {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, in)
	}

	in.CombatLevel = 13
	in.CombatXP = 0
	if err := upsertPlayerMetaSkillState(uid, in); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := loadSkillState(uid)
	if got2 != in {
		t.Errorf("update mismatch:\n got=%+v\nwant=%+v", got2, in)
	}
}

func TestUpsertPlayerMetaBabysitState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-baby-rt:example")
	expires := time.Now().UTC().Add(14 * 24 * time.Hour).Truncate(time.Second)
	in := BabysitState{
		Active:           true,
		ExpiresAt:        &expires,
		SkillFocus:       "foraging",
		AutoBabysit:      true,
		AutoBabysitFocus: "mining",
	}
	if err := upsertPlayerMetaBabysitState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadBabysitState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !got.Active || got.SkillFocus != "foraging" || !got.AutoBabysit || got.AutoBabysitFocus != "mining" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Errorf("expires_at: got %v want %v", got.ExpiresAt, expires)
	}

	// Cancel → inactive falls through, but verify upsert wrote 0.
	in.Active = false
	in.ExpiresAt = nil
	if err := upsertPlayerMetaBabysitState(uid, in); err != nil {
		t.Fatalf("upsert cancel: %v", err)
	}
	var activeInt int
	if err := db.Get().QueryRow(`SELECT babysit_active FROM player_meta WHERE user_id = ?`, string(uid)).Scan(&activeInt); err != nil {
		t.Fatalf("verify cancel: %v", err)
	}
	if activeInt != 0 {
		t.Errorf("cancel should write 0: got %d", activeInt)
	}
}

func TestUpsertPlayerMetaNPCState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-npc-rt:example")
	mistyBuff := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	in := NPCState{
		MistyBuffExpires:    &mistyBuff,
		NPCMsgCount:         12,
		NPCMsgCountDate:     "2026-05-09",
		MistyRollTarget:     8,
		ArinaRollTarget:     6,
		MistyEncounterCount: 2,
		MistyDonatedCount:   1,
		ThomAnimalLineFired: true,
		RobbieVisitCount:    4,
	}
	if err := upsertPlayerMetaNPCState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadNPCState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.NPCMsgCount != 12 || got.NPCMsgCountDate != "2026-05-09" ||
		got.MistyRollTarget != 8 || got.ArinaRollTarget != 6 ||
		got.MistyEncounterCount != 2 || got.MistyDonatedCount != 1 ||
		!got.ThomAnimalLineFired || got.RobbieVisitCount != 4 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.MistyBuffExpires == nil || !got.MistyBuffExpires.Equal(mistyBuff) {
		t.Errorf("misty_buff_expires: got %v want %v", got.MistyBuffExpires, mistyBuff)
	}
}

func TestUpsertPlayerMetaLifecycleState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-life-rt:example")
	created := time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Second)
	in := LifecycleState{
		CurrentStreak:      4,
		BestStreak:         10,
		LastActionDate:     "2026-05-09",
		StreakDecayed:      false,
		ActionTakenToday:   true,
		HolidayActionTaken: false,
		CombatActionsUsed:  2,
		HarvestActionsUsed: 1,
		CreatedAt:          &created,
	}
	if err := upsertPlayerMetaLifecycleState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadLifecycleState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CurrentStreak != 4 || got.BestStreak != 10 || got.LastActionDate != "2026-05-09" ||
		!got.ActionTakenToday || got.CombatActionsUsed != 2 || got.HarvestActionsUsed != 1 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.CreatedAt == nil || !got.CreatedAt.Equal(created) {
		t.Errorf("created_at: got %v want %v", got.CreatedAt, created)
	}
}

func TestResetAllPlayerMetaDailyActions(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-life-reset:example")
	if err := createAdvCharacter(uid, "Resetter"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	in := LifecycleState{
		LastActionDate:     "2020-01-01", // way in the past so the reset matches
		ActionTakenToday:   true,
		HolidayActionTaken: true,
		CombatActionsUsed:  3,
		HarvestActionsUsed: 2,
	}
	if err := upsertPlayerMetaLifecycleState(uid, in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := resetAllPlayerMetaDailyActions(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	got, _ := loadLifecycleState(uid)
	if got.ActionTakenToday || got.HolidayActionTaken ||
		got.CombatActionsUsed != 0 || got.HarvestActionsUsed != 0 {
		t.Errorf("after reset: got %+v", got)
	}
}

func TestUpsertPlayerMetaDeathState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-death-rt:example")
	deadUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	pardon := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	in := DeathState{
		Alive:          false,
		DeadUntil:      &deadUntil,
		LastDeathDate:  "2026-05-09",
		LastPardonUsed: &pardon,
		GrudgeLocation: "the Abyss",
		DeathSource:    "arena",
		DeathLocation:  "the Arena",
	}
	if err := upsertPlayerMetaDeathState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadDeathState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Alive || got.LastDeathDate != "2026-05-09" || got.GrudgeLocation != "the Abyss" ||
		got.DeathSource != "arena" || got.DeathLocation != "the Arena" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if got.DeadUntil == nil || !got.DeadUntil.Equal(deadUntil) {
		t.Errorf("dead_until: got %v want %v", got.DeadUntil, deadUntil)
	}
	if got.LastPardonUsed == nil || !got.LastPardonUsed.Equal(pardon) {
		t.Errorf("last_pardon_used: got %v want %v", got.LastPardonUsed, pardon)
	}
}

func TestSaveAdvCharacter_DualWritesDeathState(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-death-sw:example")
	if err := createAdvCharacter(uid, "Saver"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	char, err := loadAdvCharacter(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	deadUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	char.Alive = false
	char.DeadUntil = &deadUntil
	char.GrudgeLocation = "the Pit"
	if err := saveAdvCharacter(char); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := loadDeathState(uid)
	if err != nil {
		t.Fatalf("loadDeathState: %v", err)
	}
	if got.Alive || got.GrudgeLocation != "the Pit" {
		t.Errorf("save did not propagate to player_meta: got %+v", got)
	}
}

func TestUpsertPlayerMetaMiscState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-misc-rt:example")
	in := MiscState{Title: "Champion", TreasuresLocked: true, CraftsSucceeded: 12}
	if err := upsertPlayerMetaMiscState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadMiscState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, in)
	}
}

func TestUpsertPlayerMetaHouseState_RoundTrip(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-house-rt:example")
	in := HouseState{
		Tier:           4,
		LoanBalance:    120000,
		LoanFrozen:     true,
		MissedPayments: 7,
		Autopay:        true,
		CurrentRate:    5.875,
	}
	if err := upsertPlayerMetaHouseState(uid, in); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	got, err := loadHouseState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != in {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, in)
	}

	// Pay-down updates the columns via the same upsert.
	in.LoanBalance = 0
	in.LoanFrozen = false
	in.MissedPayments = 0
	in.Autopay = false
	if err := upsertPlayerMetaHouseState(uid, in); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := loadHouseState(uid)
	if got2 != in {
		t.Errorf("update mismatch:\n got=%+v\nwant=%+v", got2, in)
	}
}
