package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// TestProdDB_DnDLayer exercises the D&D layer against a copy of the real
// prod database. Verifies that:
//   - Existing AdventureCharacter rows load cleanly
//   - ensureDnDCharacterForCombat creates a sensible auto-migrated sheet
//   - The auto-migrated row is well-formed (HP > 0, AC ≥ 10, level ≥ 1)
//   - !setup overwrite path works on auto-migrated chars
//   - HP persistence after combat doesn't corrupt the sheet
//
// Skips silently if the prod DB isn't present.
func TestProdDB_DnDLayer(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present at " + src)
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	copyTestFile(t, src, dst)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(db.Close)

	// 1. Load every adventure character. They must scan without error.
	chars, err := loadAllAdvCharacters()
	if err != nil {
		t.Fatalf("loadAllAdvCharacters: %v", err)
	}
	if len(chars) == 0 {
		t.Fatal("no adventure characters in prod DB")
	}
	t.Logf("loaded %d real adventure characters", len(chars))

	// 2. For each, verify they have NO dnd_character row yet, then auto-migrate
	//    and verify the resulting row is sensible. Track which chars we
	//    actually migrated so steps 3+ can target one of those.
	migrated := make(map[id.UserID]bool)
	for i := range chars {
		char := &chars[i]
		uid := char.UserID

		existing, err := LoadDnDCharacter(uid)
		if err != nil {
			t.Errorf("user=%s: LoadDnDCharacter pre-migrate failed: %v", uid, err)
			continue
		}
		if existing != nil {
			// Real prod DB has accumulated rows from live bot use.
			// Skip these — the auto-migrate path is exercised by
			// other characters in the loop.
			t.Logf("user=%s: skipping (already has dnd_character row)", uid)
			continue
		}

		// Auto-migrate via the production code path.
		dnd, fresh, err := ensureDnDCharacterForCombat(uid, char)
		if err != nil {
			t.Errorf("user=%s: ensureDnDCharacterForCombat failed: %v", uid, err)
			continue
		}
		if dnd == nil {
			t.Errorf("user=%s: ensureDnDCharacterForCombat returned nil", uid)
			continue
		}
		if !fresh {
			t.Errorf("user=%s: expected fresh=true on first auto-migrate", uid)
		}

		// Sanity invariants on the auto-migrated sheet.
		if dnd.UserID != uid {
			t.Errorf("user=%s: dnd.UserID=%s mismatch", uid, dnd.UserID)
		}
		if !dnd.AutoMigrated {
			t.Errorf("user=%s: AutoMigrated=false on fresh auto-migration", uid)
		}
		if dnd.PendingSetup {
			t.Errorf("user=%s: PendingSetup=true on fresh auto-migration", uid)
		}
		if dnd.Race == "" || dnd.Class == "" {
			t.Errorf("user=%s: race/class empty: race=%q class=%q", uid, dnd.Race, dnd.Class)
		}
		if dnd.Level < 1 {
			t.Errorf("user=%s: level %d < 1", uid, dnd.Level)
		}
		// Migrated level = combat_level/5 clamped to [1, 20].
		expectLevel := dndLevelFromCombatLevel(char.CombatLevel)
		if dnd.Level != expectLevel {
			t.Errorf("user=%s: dnd.Level=%d, want %d (from combat_level=%d)",
				uid, dnd.Level, expectLevel, char.CombatLevel)
		}
		if dnd.HPMax < 1 || dnd.HPCurrent != dnd.HPMax {
			t.Errorf("user=%s: HP invariant: max=%d cur=%d", uid, dnd.HPMax, dnd.HPCurrent)
		}
		if dnd.ArmorClass < 10 {
			t.Errorf("user=%s: AC %d < 10 floor", uid, dnd.ArmorClass)
		}
		// Stat scores after racial mods should still be in legal range (1..20).
		for j, score := range []int{dnd.STR, dnd.DEX, dnd.CON, dnd.INT, dnd.WIS, dnd.CHA} {
			if score < 1 || score > 20 {
				t.Errorf("user=%s: stat[%d]=%d out of range", uid, j, score)
			}
		}

		migrated[uid] = true
		t.Logf("auto-migrated user=%s → L%d %s %s  HP=%d  AC=%d  STR/DEX/CON/INT/WIS/CHA=%d/%d/%d/%d/%d/%d",
			uid, dnd.Level, dnd.Race, dnd.Class, dnd.HPMax, dnd.ArmorClass,
			dnd.STR, dnd.DEX, dnd.CON, dnd.INT, dnd.WIS, dnd.CHA)
	}

	if len(migrated) == 0 {
		t.Skip("no characters available for auto-migrate (all pre-existing); steps 3+ require a fresh migrate")
	}

	// 3. Re-load each auto-migrated char — round-trip integrity.
	for i := range chars {
		uid := chars[i].UserID
		if !migrated[uid] {
			continue
		}
		dnd, err := LoadDnDCharacter(uid)
		if err != nil {
			t.Errorf("user=%s: re-load failed: %v", uid, err)
			continue
		}
		if dnd == nil {
			t.Errorf("user=%s: re-load returned nil (auto-migrate didn't persist)", uid)
			continue
		}
		if !dnd.AutoMigrated || dnd.PendingSetup {
			t.Errorf("user=%s: post-reload flags wrong: auto=%v pending=%v",
				uid, dnd.AutoMigrated, dnd.PendingSetup)
		}
	}

	// 4. ensureDnDCharacterForCombat is idempotent — second call returns the
	//    same row, doesn't create a duplicate. Pick the first migrated char.
	var idemIdx int
	for i := range chars {
		if migrated[chars[i].UserID] {
			idemIdx = i
			break
		}
	}
	uid := chars[idemIdx].UserID
	first, _ := LoadDnDCharacter(uid)
	second, freshAgain, err := ensureDnDCharacterForCombat(uid, &chars[idemIdx])
	if err != nil {
		t.Errorf("idempotent ensure failed: %v", err)
	}
	if freshAgain {
		t.Error("second ensure call returned fresh=true; should be false (already migrated)")
	}
	if first != nil && second != nil {
		if first.Race != second.Race || first.Class != second.Class {
			t.Errorf("ensure returned different sheet on second call: %v vs %v",
				first, second)
		}
	}

	// 5. HP persistence smoke test — simulate a combat that left the player
	//    at 50% HP. Verify dnd_character.hp_current shifts proportionally.
	persistDnDHPAfterCombat(uid, 100, 50)
	after, err := LoadDnDCharacter(uid)
	if err != nil || after == nil {
		t.Fatalf("post-persist load: %v", err)
	}
	want := after.HPMax / 2
	if after.HPCurrent < want-1 || after.HPCurrent > want+1 {
		t.Errorf("hp_current=%d after 50%% loss; want ~%d (hp_max=%d)",
			after.HPCurrent, want, after.HPMax)
	}
	t.Logf("HP persistence verified: %d/%d after 50%% combat damage", after.HPCurrent, after.HPMax)

	// 6. !setup overwrite path: an auto-migrated char should be wipeable via
	//    loadOrInitDraft → DELETE → fresh draft.
	draft, err := loadOrInitDraft(uid)
	if err != nil {
		t.Fatalf("loadOrInitDraft on auto-migrated: %v", err)
	}
	if !draft.PendingSetup {
		t.Error("draft should have PendingSetup=true")
	}
	// The auto-migrated row should now be deleted.
	cleared, _ := LoadDnDCharacter(uid)
	if cleared != nil {
		t.Errorf("auto-migrated row not cleared by loadOrInitDraft: %+v", cleared)
	}

	// 7. Adventure characters list should still be loadable without
	//    interference from D&D-layer joins.
	chars2, err := loadAllAdvCharacters()
	if err != nil {
		t.Fatalf("post-test reload failed: %v", err)
	}
	if len(chars2) != len(chars) {
		t.Errorf("char count drift: pre=%d post=%d", len(chars), len(chars2))
	}
}

func copyTestFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

// silence unused warning if id pkg drifts
var _ = id.UserID("")
