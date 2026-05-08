package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 9 SP4 — ensureSpellsForCharacter migration / idempotency tests.

func mageCharacter(uid id.UserID, level int) *DnDCharacter {
	return &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassMage, Level: level,
		STR: 8, DEX: 14, CON: 12, INT: 17, WIS: 12, CHA: 10,
		HPMax: 10, HPCurrent: 10, ArmorClass: 12,
	}
}

// TestEnsureSpells_Idempotent — second call leaves the known list unchanged
// and does not duplicate rows.
func TestEnsureSpells_Idempotent(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@migrate_idem:example")
	c := mageCharacter(uid, 5)
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	first, _ := listKnownSpells(uid)
	if len(first) == 0 {
		t.Fatal("first ensureSpells: no known spells granted")
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	second, _ := listKnownSpells(uid)
	if len(second) != len(first) {
		t.Errorf("idempotency: first=%d, second=%d", len(first), len(second))
	}
	// Spell IDs must match exactly (set equality).
	wantIDs := map[string]bool{}
	for _, r := range first {
		wantIDs[r.SpellID] = true
	}
	for _, r := range second {
		if !wantIDs[r.SpellID] {
			t.Errorf("second pass introduced new spell: %s", r.SpellID)
		}
	}
}

// TestEnsureSpells_RefreshesSlotPoolOnLevelChange — when level shifts between
// sessions, the slot pool re-syncs to the new level even if spells already
// exist (the early-exit branch still re-runs setSpellSlotsForLevel).
func TestEnsureSpells_RefreshesSlotPoolOnLevelChange(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@migrate_level:example")

	c := mageCharacter(uid, 1)
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	got, _ := getSpellSlots(uid)
	if got[1][0] != 2 || got[2][0] != 0 {
		t.Errorf("L1 Mage slot pool: %v, want only L1=2", got)
	}

	// Level up between sessions.
	c.Level = 5
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	got, _ = getSpellSlots(uid)
	want := slotsForClassLevel(ClassMage, 5)
	for lvl, total := range want {
		if got[lvl][0] != total {
			t.Errorf("post-level-up L%d slots: got total=%d, want %d", lvl, got[lvl][0], total)
		}
	}
}

// TestEnsureSpells_NonCasterIsNoOp — Fighter call writes nothing.
func TestEnsureSpells_NonCasterIsNoOp(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@migrate_fighter:example")
	c := &DnDCharacter{UserID: uid, Class: ClassFighter, Race: RaceHuman, Level: 5}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	rows, _ := listKnownSpells(uid)
	if len(rows) != 0 {
		t.Errorf("non-caster got known spells: %+v", rows)
	}
	slots, _ := getSpellSlots(uid)
	if len(slots) != 0 {
		t.Errorf("non-caster got slot pool: %+v", slots)
	}
}

// TestEnsureSpells_RangerL1CantripsOnly — Ranger L1 has no slots, and the
// granted spells are all cantrips (Level 0).
func TestEnsureSpells_RangerL1CantripsOnly(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@migrate_ranger:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassRanger, Level: 1,
		STR: 12, DEX: 16, CON: 13, INT: 10, WIS: 14, CHA: 8,
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	rows, _ := listKnownSpells(uid)
	if len(rows) == 0 {
		t.Fatal("ranger L1 got nothing — expected at least cantrips")
	}
	for _, r := range rows {
		s, ok := lookupSpell(r.SpellID)
		if !ok {
			t.Errorf("unknown spell %q in known list", r.SpellID)
			continue
		}
		if s.Level != 0 {
			t.Errorf("ranger L1 got non-cantrip %s (L%d); expected cantrips only",
				s.ID, s.Level)
		}
	}
	slots, _ := getSpellSlots(uid)
	if len(slots) != 0 {
		t.Errorf("ranger L1 has slot pool: %+v, want empty", slots)
	}
}

// TestEnsureSpells_LevelAppropriateDefaultsExist — every default spell at
// representative levels is present in the registry and within the player's
// max slot level (or a cantrip).
func TestEnsureSpells_LevelAppropriateDefaultsExist(t *testing.T) {
	for _, class := range []DnDClass{ClassMage, ClassCleric, ClassRanger} {
		for _, lvl := range []int{1, 2, 5, 9, 13, 20} {
			maxSlot := highestAvailableSlot(class, lvl)
			for _, sid := range defaultKnownSpells(class, lvl) {
				s, ok := lookupSpell(sid)
				if !ok {
					t.Errorf("%s L%d default %q missing from registry", class, lvl, sid)
					continue
				}
				if s.Level > maxSlot && s.Level > 0 {
					t.Errorf("%s L%d default %s requires L%d slot but max is L%d",
						class, lvl, sid, s.Level, maxSlot)
				}
				// Class-list sanity.
				ok = false
				for _, cl := range s.Classes {
					if cl == class {
						ok = true
						break
					}
				}
				if !ok {
					t.Errorf("%s L%d default %s is not on the %s class list",
						class, lvl, sid, class)
				}
			}
		}
	}
}

// TestEnsureSpells_ClericDefaultsAutoPrepared — auto-migrated Cleric defaults
// are flagged prepared so !cast works immediately. (Pre-SP4 contract; SP4
// keeps it for migration backward-compat — players can later !prepare clear.)
func TestEnsureSpells_ClericDefaultsAutoPrepared(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@migrate_cleric:example")
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassCleric, Level: 5,
		STR: 12, DEX: 12, CON: 14, INT: 10, WIS: 16, CHA: 12,
	}
	if err := ensureSpellsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	rows, _ := listKnownSpells(uid)
	if len(rows) == 0 {
		t.Fatal("cleric got no defaults")
	}
	for _, r := range rows {
		if !r.Prepared {
			t.Errorf("cleric default %s not prepared after migration", r.SpellID)
		}
	}
}
