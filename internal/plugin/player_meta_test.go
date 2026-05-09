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
