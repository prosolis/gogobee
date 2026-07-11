package plugin

import (
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// TestAutoMigrationSeedsPlayerMeta guards the camcast straggler regression: a
// brand-new player whose first-ever adventure action auto-migrates a character
// (e.g. !rest before !setup) must come out with a canonical player_meta seed,
// or every legacy-layer command (expeditions, arena, world boss, town…) fails
// with "sql: no rows". Before the ensurePlayerMetaSeed guard this produced a
// confirmed dnd_character with zero player_meta rows.
func TestAutoMigrationSeedsPlayerMeta(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@fresh-automigrant:example.org")

	p := &AdventurePlugin{euro: &EuroPlugin{}}
	p.Sink = &captureSink{}

	// First-ever adventure action for a player with no character at all.
	if err := p.handleDnDShortRest(MessageContext{Sender: uid}); err != nil {
		t.Fatalf("handleDnDShortRest: %v", err)
	}

	d := db.Get()
	var dndRows, pmRows, equipRows, pending int
	d.QueryRow(`SELECT COUNT(*) FROM dnd_character WHERE user_id=?`, string(uid)).Scan(&dndRows)
	d.QueryRow(`SELECT COUNT(*) FROM player_meta WHERE user_id=?`, string(uid)).Scan(&pmRows)
	d.QueryRow(`SELECT COUNT(*) FROM adventure_equipment WHERE user_id=?`, string(uid)).Scan(&equipRows)
	d.QueryRow(`SELECT COALESCE(MAX(pending_setup),-1) FROM dnd_character WHERE user_id=?`, string(uid)).Scan(&pending)

	if dndRows != 1 {
		t.Fatalf("expected 1 auto-migrated dnd_character, got %d", dndRows)
	}
	if pending != 0 {
		t.Errorf("auto-migrated character should be confirmed (pending_setup=0), got %d", pending)
	}
	if pmRows != 1 {
		t.Errorf("player_meta not seeded for auto-migrant: got %d rows, want 1 (camcast straggler regression)", pmRows)
	}
	// createAdvCharacter seeds tier-0 gear in all five slots.
	if equipRows != len(allSlots) {
		t.Errorf("tier-0 equipment not seeded: got %d rows, want %d", equipRows, len(allSlots))
	}

	// The character must now load through the legacy layer that camcast choked on.
	if _, err := loadAdvCharacter(uid); err != nil {
		t.Errorf("loadAdvCharacter after auto-migration: %v (this is the exact failure camcast hit)", err)
	}
}

// TestEnsurePlayerMetaSeedIdempotent proves the guard never duplicates a legacy
// player's equipment — createAdvCharacter's tier-0 insert has no conflict guard,
// so the seed must be conditional on player_meta being absent.
func TestEnsurePlayerMetaSeedIdempotent(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@legacy:example.org")

	// Seed once (fresh), then again (should be a no-op).
	if err := ensurePlayerMetaSeed(uid); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := ensurePlayerMetaSeed(uid); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	d := db.Get()
	var pmRows, equipRows int
	d.QueryRow(`SELECT COUNT(*) FROM player_meta WHERE user_id=?`, string(uid)).Scan(&pmRows)
	d.QueryRow(`SELECT COUNT(*) FROM adventure_equipment WHERE user_id=?`, string(uid)).Scan(&equipRows)
	if pmRows != 1 {
		t.Errorf("player_meta rows = %d, want 1 (no duplication)", pmRows)
	}
	if equipRows != len(allSlots) {
		t.Errorf("equipment rows = %d, want %d (guard must not re-insert gear)", equipRows, len(allSlots))
	}
}
