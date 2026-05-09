package plugin

import (
	"testing"

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
