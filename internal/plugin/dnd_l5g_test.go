package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// TestL5gBackfillDnDCharacters_Idempotent verifies that the L5g mass-backfill:
//   - creates a DnDCharacter for an adventure_characters row that lacks one,
//   - seeds Level from CombatLevel via dndLevelFromCombatLevel,
//   - is idempotent (re-running does not clobber an already-populated row),
//   - does not touch users who already have a confirmed (or pending) D&D row.
func TestL5gBackfillDnDCharacters_Idempotent(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@l5g-bf:example")
	if err := createAdvCharacter(uid, "Backfiller"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	if _, err := db.Get().Exec(
		`UPDATE adventure_characters SET combat_level = ? WHERE user_id = ?`,
		25, string(uid),
	); err != nil {
		t.Fatalf("seed combat_level: %v", err)
	}
	// Ensure no D&D row pre-exists.
	if _, err := db.Get().Exec(`DELETE FROM dnd_character WHERE user_id = ?`, string(uid)); err != nil {
		t.Fatalf("clear dnd_character: %v", err)
	}

	if err := backfillDnDCharactersFromAdv(); err != nil {
		t.Fatalf("backfill 1: %v", err)
	}

	c, err := LoadDnDCharacter(uid)
	if err != nil {
		t.Fatalf("load after backfill: %v", err)
	}
	if c == nil {
		t.Fatalf("expected D&D character row after backfill, got nil")
	}
	wantLevel := dndLevelFromCombatLevel(25)
	if c.Level != wantLevel {
		t.Errorf("Level = %d, want %d", c.Level, wantLevel)
	}
	if c.PendingSetup {
		t.Errorf("backfilled row should be PendingSetup=false")
	}
	if !c.AutoMigrated {
		t.Errorf("backfilled row should be AutoMigrated=true")
	}
	if c.HPMax <= 0 || c.HPCurrent <= 0 {
		t.Errorf("backfilled row missing HP: max=%d cur=%d", c.HPMax, c.HPCurrent)
	}

	// Mutate the row to prove a second backfill is idempotent (does not
	// overwrite the existing row).
	c.Level = 12
	c.HPCurrent = 5
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("save mutated: %v", err)
	}
	if err := backfillDnDCharactersFromAdv(); err != nil {
		t.Fatalf("backfill 2: %v", err)
	}
	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Level != 12 || got.HPCurrent != 5 {
		t.Errorf("backfill clobbered live row: Level=%d HPCurrent=%d", got.Level, got.HPCurrent)
	}
}

// TestL5gBackfillDnDCharacters_SkipsPendingSetup ensures users mid-!setup
// (PendingSetup=1, partial draft) are not overwritten.
func TestL5gBackfillDnDCharacters_SkipsPendingSetup(t *testing.T) {
	setupAuditTestDB(t)

	uid := id.UserID("@l5g-pending:example")
	if err := createAdvCharacter(uid, "Drafter"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	draft := &DnDCharacter{
		UserID:       uid,
		Race:         RaceElf,
		Class:        ClassMage,
		Level:        3,
		PendingSetup: true,
	}
	if err := SaveDnDCharacter(draft); err != nil {
		t.Fatalf("save draft: %v", err)
	}

	if err := backfillDnDCharactersFromAdv(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.PendingSetup {
		t.Errorf("backfill clobbered pending draft (PendingSetup flipped to false)")
	}
	if got.Race != RaceElf || got.Class != ClassMage {
		t.Errorf("backfill mutated draft race/class: got %s/%s", got.Race, got.Class)
	}
}
