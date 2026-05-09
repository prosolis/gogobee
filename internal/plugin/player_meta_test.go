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

func TestPlayerMetaArenaBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uidA := id.UserID("@meta-bf-a:example")
	uidB := id.UserID("@meta-bf-b:example")
	if err := createAdvCharacter(uidA, "metaA"); err != nil {
		t.Fatalf("createAdvCharacter A: %v", err)
	}
	if err := createAdvCharacter(uidB, "metaB"); err != nil {
		t.Fatalf("createAdvCharacter B: %v", err)
	}
	// Stamp arena counters directly on adventure_characters so the backfill
	// has something interesting to copy.
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET arena_wins = ?, arena_losses = ?, invasion_score = ? WHERE user_id = ?`,
		7, 3, 0, string(uidA),
	); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET arena_wins = ?, arena_losses = ?, invasion_score = ? WHERE user_id = ?`,
		1, 1, 5, string(uidB),
	); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	// Wipe any pre-existing player_meta rows from the prod-DB copy so we're
	// testing the backfill from a clean slate for these two users.
	if _, err := db.Get().Exec(
		`DELETE FROM player_meta WHERE user_id IN (?, ?)`,
		string(uidA), string(uidB),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaArena(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}

	mA, _ := loadPlayerMeta(uidA)
	if mA.ArenaWins != 7 || mA.ArenaLosses != 3 || mA.InvasionScore != 0 {
		t.Errorf("A after backfill: got %+v want wins=7 losses=3 inv=0", mA)
	}
	mB, _ := loadPlayerMeta(uidB)
	if mB.ArenaWins != 1 || mB.ArenaLosses != 1 || mB.InvasionScore != 5 {
		t.Errorf("B after backfill: got %+v want wins=1 losses=1 inv=5", mB)
	}

	// Layer a dual-write update on top — INSERT OR IGNORE re-runs must not
	// clobber post-backfill state.
	if err := upsertPlayerMetaArena(uidA, 8, 3, 0); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaArena(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	mA2, _ := loadPlayerMeta(uidA)
	if mA2.ArenaWins != 8 {
		t.Errorf("backfill clobbered dual-write: got wins=%d want 8", mA2.ArenaWins)
	}
}

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

func TestPlayerMetaDisplayNameBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-dn-bf:example")
	if err := createAdvCharacter(uid, "DisplayedName"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	// Reset player_meta to empty display_name so the backfill has work.
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET display_name = '' WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaDisplayName(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadDisplayName(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != "DisplayedName" {
		t.Errorf("after backfill: got %q want %q", got, "DisplayedName")
	}

	// Layer a dual-write rename. Backfill re-run must NOT clobber it back
	// (it only touches rows with empty display_name).
	if err := upsertPlayerMetaDisplayName(uid, "Renamed"); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaDisplayName(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadDisplayName(uid)
	if got2 != "Renamed" {
		t.Errorf("backfill clobbered dual-write: got %q want %q", got2, "Renamed")
	}
}

func TestLoadDisplayName_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-dn-fb:example")
	if err := createAdvCharacter(uid, "FromAdvChar"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	// Simulate a row that exists in adventure_characters but whose
	// player_meta display_name hasn't been populated yet (pre-backfill,
	// pre-dual-write soak case).
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET display_name = '' WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadDisplayName(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != "FromAdvChar" {
		t.Errorf("fallback: got %q want %q", got, "FromAdvChar")
	}

	// Unknown user → empty string, no error.
	got2, err := loadDisplayName(id.UserID("@meta-dn-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2 != "" {
		t.Errorf("missing user: got %q want empty", got2)
	}
}

// Phase L4a — Hospital migration. Backfill is idempotent and only touches
// zero-valued rows; the upsert helper round-trips; loadHospitalVisits falls
// back to AdvCharacter when player_meta hasn't been populated yet.

func TestPlayerMetaHospitalVisitsBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-hv-bf:example")
	if err := createAdvCharacter(uid, "HVisitor"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET hospital_visits = ? WHERE user_id = ?`,
		4, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET hospital_visits = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaHospitalVisits(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadHospitalVisits(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 4 {
		t.Errorf("after backfill: got %d want 4", got)
	}

	// Layer a dual-write increment. Backfill re-run must NOT clobber it
	// back (only touches rows whose hospital_visits is still zero).
	if err := upsertPlayerMetaHospitalVisits(uid, 5); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaHospitalVisits(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadHospitalVisits(uid)
	if got2 != 5 {
		t.Errorf("backfill clobbered dual-write: got %d want 5", got2)
	}
}

func TestLoadHospitalVisits_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-hv-fb:example")
	if err := createAdvCharacter(uid, "Fallbacker"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET hospital_visits = ? WHERE user_id = ?`,
		3, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET hospital_visits = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadHospitalVisits(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 3 {
		t.Errorf("fallback: got %d want 3", got)
	}

	// Unknown user → 0, no error.
	got2, err := loadHospitalVisits(id.UserID("@meta-hv-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2 != 0 {
		t.Errorf("missing user: got %d want 0", got2)
	}
}

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

func TestPlayerMetaRivalStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-rv-bf:example")
	if err := createAdvCharacter(uid, "Rivaler"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET rival_pool = ?, rival_unlocked_notified = ? WHERE user_id = ?`,
		1, 1, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET rival_pool = 0, rival_unlocked_notified = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaRivalState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	pool, notified, err := loadRivalState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pool != 1 || !notified {
		t.Errorf("after backfill: got pool=%d notified=%v want 1, true", pool, notified)
	}

	// Layer a dual-write update. Backfill re-run must NOT clobber it back
	// (only touches rows whose rival columns are still both zero).
	if err := upsertPlayerMetaRivalState(uid, 1, false); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaRivalState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	pool2, notified2, _ := loadRivalState(uid)
	if pool2 != 1 || notified2 {
		t.Errorf("backfill clobbered dual-write: got pool=%d notified=%v want 1, false", pool2, notified2)
	}
}

func TestLoadRivalState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-rv-fb:example")
	if err := createAdvCharacter(uid, "RivalFallback"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET rival_pool = ?, rival_unlocked_notified = ? WHERE user_id = ?`,
		1, 1, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET rival_pool = 0, rival_unlocked_notified = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	pool, notified, err := loadRivalState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pool != 1 || !notified {
		t.Errorf("fallback: got pool=%d notified=%v want 1, true", pool, notified)
	}

	// Unknown user → zeros, no error.
	pool2, notified2, err := loadRivalState(id.UserID("@meta-rv-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if pool2 != 0 || notified2 {
		t.Errorf("missing user: got pool=%d notified=%v want 0, false", pool2, notified2)
	}
}

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

func TestPlayerMetaMasterworkDropsBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-mw-bf:example")
	if err := createAdvCharacter(uid, "Dropper"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET masterwork_drops_received = ? WHERE user_id = ?`,
		3, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET masterwork_drops_received = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaMasterworkDrops(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadMasterworkDrops(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 3 {
		t.Errorf("after backfill: got %d want 3", got)
	}

	// Layer a dual-write increment. Backfill re-run must NOT clobber it
	// back (only touches rows whose masterwork_drops_received is still zero).
	if err := upsertPlayerMetaMasterworkDrops(uid, 4); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaMasterworkDrops(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadMasterworkDrops(uid)
	if got2 != 4 {
		t.Errorf("backfill clobbered dual-write: got %d want 4", got2)
	}
}

func TestLoadMasterworkDrops_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-mw-fb:example")
	if err := createAdvCharacter(uid, "MWFallback"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET masterwork_drops_received = ? WHERE user_id = ?`,
		2, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET masterwork_drops_received = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadMasterworkDrops(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != 2 {
		t.Errorf("fallback: got %d want 2", got)
	}

	// Unknown user → 0, no error.
	got2, err := loadMasterworkDrops(id.UserID("@meta-mw-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2 != 0 {
		t.Errorf("missing user: got %d want 0", got2)
	}
}

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

func TestPlayerMetaPetStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-pet-bf:example")
	if err := createAdvCharacter(uid, "PetOwner"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET pet_type = ?, pet_name = ?, pet_xp = ?, pet_level = ?,
		        pet_armor_tier = ?, pet_arrived = ?, pet_chased_away = ?,
		        pet_reactivated = ?, pet_morning_defense = ?,
		        pet_supply_shop_unlocked = ?, pet_level_10_date = ?
		  WHERE user_id = ?`,
		"dog", "Rex", 750, 4, 2, 1, 0, 0, 1, 0, "", string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET pet_type = '' WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaPetState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadPetState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Type != "dog" || got.Name != "Rex" || got.XP != 750 || got.Level != 4 ||
		got.ArmorTier != 2 || !got.Arrived || got.ChasedAway || got.Reactivated ||
		!got.MorningDefense {
		t.Errorf("after backfill: got %+v", got)
	}

	// Layer a dual-write that promotes the pet (level-up + supply shop).
	got.Level = 10
	got.XP = 0
	got.Level10Date = "2026-05-09"
	got.SupplyShopUnlocked = true
	if err := upsertPlayerMetaPetState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaPetState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadPetState(uid)
	if got2.Level != 10 || !got2.SupplyShopUnlocked || got2.Level10Date != "2026-05-09" {
		t.Errorf("backfill clobbered dual-write: got %+v", got2)
	}
}

func TestLoadPetState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-pet-fb:example")
	if err := createAdvCharacter(uid, "PetFallback"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET pet_type = ?, pet_name = ?, pet_xp = ?, pet_level = ?,
		        pet_arrived = ?, pet_chased_away = ?, pet_reactivated = ?
		  WHERE user_id = ?`,
		"cat", "Whiskers", 50, 2, 1, 1, 1, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET pet_type = '' WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadPetState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Type != "cat" || got.Name != "Whiskers" || got.Level != 2 ||
		!got.Arrived || !got.ChasedAway || !got.Reactivated {
		t.Errorf("fallback: got %+v", got)
	}

	// Unknown user → zero PetState, no error.
	got2, err := loadPetState(id.UserID("@meta-pet-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2.Type != "" || got2.Level != 0 {
		t.Errorf("missing user: got %+v", got2)
	}
}

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

func TestPlayerMetaHouseStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-house-bf:example")
	if err := createAdvCharacter(uid, "HouseOwner"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET house_tier = ?, house_loan_balance = ?, house_loan_frozen = ?,
		        house_missed_payments = ?, house_autopay = ?, house_current_rate = ?
		  WHERE user_id = ?`,
		2, 50000, 0, 1, 1, 6.25, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET house_tier = 0, house_loan_balance = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaHouseState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadHouseState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Tier != 2 || got.LoanBalance != 50000 || got.LoanFrozen ||
		got.MissedPayments != 1 || !got.Autopay || got.CurrentRate != 6.25 {
		t.Errorf("after backfill: got %+v", got)
	}

	// Layer a dual-write that pays down the loan and toggles autopay off.
	got.LoanBalance = 25000
	got.MissedPayments = 0
	got.Autopay = false
	if err := upsertPlayerMetaHouseState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaHouseState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadHouseState(uid)
	if got2.LoanBalance != 25000 || got2.Autopay || got2.MissedPayments != 0 {
		t.Errorf("backfill clobbered dual-write: got %+v", got2)
	}
}

func TestLoadHouseState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-house-fb:example")
	if err := createAdvCharacter(uid, "HouseFallback"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET house_tier = ?, house_loan_balance = ?, house_loan_frozen = ?,
		        house_missed_payments = ?, house_autopay = ?, house_current_rate = ?
		  WHERE user_id = ?`,
		3, 75000, 1, 5, 0, 7.0, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET house_tier = 0, house_loan_balance = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadHouseState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Tier != 3 || got.LoanBalance != 75000 || !got.LoanFrozen ||
		got.MissedPayments != 5 || got.Autopay || got.CurrentRate != 7.0 {
		t.Errorf("fallback: got %+v", got)
	}

	// Unknown user → zero HouseState, no error.
	got2, err := loadHouseState(id.UserID("@meta-house-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2.Tier != 0 || got2.LoanBalance != 0 {
		t.Errorf("missing user: got %+v", got2)
	}
}

func TestPlayerMetaSkillStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-skill-bf:example")
	if err := createAdvCharacter(uid, "Skiller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET combat_level = ?, combat_xp = ?,
		        mining_skill = ?, mining_xp = ?,
		        foraging_skill = ?, foraging_xp = ?,
		        fishing_skill = ?, fishing_xp = ?
		  WHERE user_id = ?`,
		7, 250, 4, 100, 3, 80, 2, 60, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET combat_level = 0, combat_xp = 0,
		        mining_skill = 0, mining_xp = 0,
		        foraging_skill = 0, foraging_xp = 0,
		        fishing_skill = 0, fishing_xp = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaSkillState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadSkillState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := SkillState{CombatLevel: 7, CombatXP: 250, MiningSkill: 4, MiningXP: 100, ForagingSkill: 3, ForagingXP: 80, FishingSkill: 2, FishingXP: 60}
	if got != want {
		t.Errorf("after backfill: got %+v want %+v", got, want)
	}

	// Layer a dual-write (level-up bump combat to 8, drain xp).
	got.CombatLevel = 8
	got.CombatXP = 0
	if err := upsertPlayerMetaSkillState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaSkillState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadSkillState(uid)
	if got2.CombatLevel != 8 || got2.CombatXP != 0 {
		t.Errorf("backfill clobbered dual-write: got %+v", got2)
	}
}

func TestLoadSkillState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-skill-fb:example")
	if err := createAdvCharacter(uid, "Faller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET combat_level = ?, mining_skill = ?, foraging_skill = ?, fishing_skill = ?
		  WHERE user_id = ?`,
		5, 3, 2, 1, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET combat_level = 0, combat_xp = 0,
		        mining_skill = 0, mining_xp = 0,
		        foraging_skill = 0, foraging_xp = 0,
		        fishing_skill = 0, fishing_xp = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadSkillState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CombatLevel != 5 || got.MiningSkill != 3 || got.ForagingSkill != 2 || got.FishingSkill != 1 {
		t.Errorf("fallback: got %+v", got)
	}

	// Unknown user → zero SkillState, no error.
	got2, err := loadSkillState(id.UserID("@meta-skill-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2 != (SkillState{}) {
		t.Errorf("missing user: got %+v", got2)
	}
}

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

func TestPlayerMetaBabysitStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-baby-bf:example")
	if err := createAdvCharacter(uid, "Sitter"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Second)
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET babysit_active = ?, babysit_expires_at = ?, babysit_skill_focus = ?,
		        auto_babysit = ?, auto_babysit_focus = ?
		  WHERE user_id = ?`,
		1, expires, "mining", 1, "fishing", string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET babysit_active = 0, babysit_expires_at = NULL,
		        babysit_skill_focus = '', auto_babysit = 0, auto_babysit_focus = '' WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaBabysitState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadBabysitState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !got.Active || got.SkillFocus != "mining" || !got.AutoBabysit || got.AutoBabysitFocus != "fishing" {
		t.Errorf("after backfill: got %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Errorf("expires_at: got %v want %v", got.ExpiresAt, expires)
	}

	// Layer a dual-write: cancel.
	got.Active = false
	got.ExpiresAt = nil
	got.SkillFocus = ""
	if err := upsertPlayerMetaBabysitState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaBabysitState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadBabysitState(uid)
	if got2.Active {
		t.Errorf("backfill clobbered cancel: got %+v", got2)
	}
}

func TestLoadBabysitState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-baby-fb:example")
	if err := createAdvCharacter(uid, "Faller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	expires := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET babysit_active = ?, babysit_expires_at = ?
		  WHERE user_id = ?`,
		1, expires, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET babysit_active = 0, babysit_expires_at = NULL WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadBabysitState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !got.Active {
		t.Errorf("fallback should be active: got %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Errorf("expires_at: got %v want %v", got.ExpiresAt, expires)
	}

	// Unknown user → zero state, no error.
	got2, err := loadBabysitState(id.UserID("@meta-baby-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2.Active {
		t.Errorf("missing user: got %+v", got2)
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

func TestPlayerMetaNPCStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-npc-bf:example")
	if err := createAdvCharacter(uid, "NPCer"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	mistyLast := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET misty_last_seen = ?, npc_msg_count = ?, npc_msg_count_date = ?,
		        misty_encounter_count = ?, misty_donated_count = ?,
		        thom_animal_line_fired = ?, robbie_visit_count = ?
		  WHERE user_id = ?`,
		mistyLast, 4, "2026-05-09", 3, 1, 1, 2, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET misty_last_seen = NULL, arina_last_seen = NULL,
		        misty_buff_expires = NULL, misty_debuff_expires = NULL, arina_buff_expires = NULL,
		        npc_msg_count = 0, npc_msg_count_date = '',
		        misty_roll_target = 0, arina_roll_target = 0,
		        misty_encounter_count = 0, misty_donated_count = 0,
		        thom_animal_line_fired = 0, robbie_visit_count = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaNPCState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadNPCState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.NPCMsgCount != 4 || got.NPCMsgCountDate != "2026-05-09" ||
		got.MistyEncounterCount != 3 || got.MistyDonatedCount != 1 ||
		!got.ThomAnimalLineFired || got.RobbieVisitCount != 2 {
		t.Errorf("after backfill: got %+v", got)
	}
	if got.MistyLastSeen == nil || !got.MistyLastSeen.Equal(mistyLast) {
		t.Errorf("misty_last_seen: got %v want %v", got.MistyLastSeen, mistyLast)
	}

	// Layer a dual-write: bump robbie visits.
	got.RobbieVisitCount = 5
	if err := upsertPlayerMetaNPCState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaNPCState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadNPCState(uid)
	if got2.RobbieVisitCount != 5 {
		t.Errorf("backfill clobbered dual-write: got %+v", got2)
	}
}

func TestLoadNPCState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-npc-fb:example")
	if err := createAdvCharacter(uid, "Faller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET misty_encounter_count = ?, robbie_visit_count = ?
		  WHERE user_id = ?`,
		7, 3, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET misty_encounter_count = 0, robbie_visit_count = 0 WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	got, err := loadNPCState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.MistyEncounterCount != 7 || got.RobbieVisitCount != 3 {
		t.Errorf("fallback: got %+v", got)
	}

	got2, err := loadNPCState(id.UserID("@meta-npc-nobody:example"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if got2.HasNPCActivity() {
		t.Errorf("missing user: got %+v", got2)
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

func TestPlayerMetaLifecycleStateBackfill_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-life-bf:example")
	if err := createAdvCharacter(uid, "Lifer"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	created := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters
		    SET current_streak = ?, best_streak = ?, last_action_date = ?, streak_decayed = ?,
		        action_taken_today = ?, holiday_action_taken = ?,
		        combat_actions_used = ?, harvest_actions_used = ?,
		        created_at = ?
		  WHERE user_id = ?`,
		7, 12, "2026-05-08", 0, 1, 0, 1, 2, created, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET current_streak = 0, best_streak = 0, last_action_date = '',
		        streak_decayed = 0, action_taken_today = 0, holiday_action_taken = 0,
		        combat_actions_used = 0, harvest_actions_used = 0,
		        created_at = NULL, last_active_at = NULL WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if err := backfillPlayerMetaLifecycleState(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}
	got, err := loadLifecycleState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CurrentStreak != 7 || got.BestStreak != 12 || got.LastActionDate != "2026-05-08" ||
		!got.ActionTakenToday || got.CombatActionsUsed != 1 || got.HarvestActionsUsed != 2 {
		t.Errorf("after backfill: got %+v", got)
	}
	if got.CreatedAt == nil || !got.CreatedAt.Equal(created) {
		t.Errorf("created_at: got %v want %v", got.CreatedAt, created)
	}

	// Layer a dual-write: streak halve.
	got.CurrentStreak = 3
	got.StreakDecayed = true
	if err := upsertPlayerMetaLifecycleState(uid, got); err != nil {
		t.Fatalf("dual-write: %v", err)
	}
	if err := backfillPlayerMetaLifecycleState(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got2, _ := loadLifecycleState(uid)
	if got2.CurrentStreak != 3 || !got2.StreakDecayed {
		t.Errorf("backfill clobbered dual-write: got %+v", got2)
	}
}

func TestLoadLifecycleState_FallsBackToAdvCharacter(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@meta-life-fb:example")
	if err := createAdvCharacter(uid, "Faller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	// createAdvCharacter now seeds player_meta.created_at, so to test
	// fallback, force the player_meta row to have empty lifecycle.
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET current_streak = 0, best_streak = 0, last_action_date = '',
		        streak_decayed = 0, action_taken_today = 0, holiday_action_taken = 0,
		        combat_actions_used = 0, harvest_actions_used = 0,
		        created_at = NULL, last_active_at = NULL WHERE user_id = ?`,
		string(uid),
	); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET current_streak = ?, best_streak = ? WHERE user_id = ?`,
		5, 8, string(uid),
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := loadLifecycleState(uid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CurrentStreak != 5 || got.BestStreak != 8 {
		t.Errorf("fallback: got %+v", got)
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
