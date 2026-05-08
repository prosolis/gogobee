package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// ── Registry coverage ────────────────────────────────────────────────────────

func TestSpellRegistryCoverage(t *testing.T) {
	// Phase 9 ships 76 in-scope spells + 3 reaction spells in the registry.
	// 79 total. Hard-bound so accidental drops surface in CI.
	if got := len(dndSpellRegistry); got < 76 {
		t.Errorf("spell registry has %d entries, want ≥76", got)
	}

	// Every spell must declare at least one class, a name, and a level 0..5.
	for id, s := range dndSpellRegistry {
		if s.Name == "" {
			t.Errorf("spell %q has empty Name", id)
		}
		if len(s.Classes) == 0 {
			t.Errorf("spell %q has no classes", id)
		}
		if s.Level < 0 || s.Level > 5 {
			t.Errorf("spell %q level %d out of [0,5]", id, s.Level)
		}
		if s.Effect == "" {
			t.Errorf("spell %q has empty Effect", id)
		}
	}
}

func TestEachClassHasSpellsAtEachAvailableLevel(t *testing.T) {
	// Mage at L9: should have at least one spell at every level 0..5.
	for _, lvl := range []int{0, 1, 2, 3, 4, 5} {
		if !classHasSpellAtLevel(ClassMage, lvl) {
			t.Errorf("Mage missing any L%d spell", lvl)
		}
	}
	// Cleric at L9: 0..5.
	for _, lvl := range []int{0, 1, 2, 3, 4, 5} {
		if !classHasSpellAtLevel(ClassCleric, lvl) {
			t.Errorf("Cleric missing any L%d spell", lvl)
		}
	}
	// Ranger at L13: 0..3.
	for _, lvl := range []int{0, 1, 2, 3} {
		if !classHasSpellAtLevel(ClassRanger, lvl) {
			t.Errorf("Ranger missing any L%d spell", lvl)
		}
	}
}

func classHasSpellAtLevel(class DnDClass, level int) bool {
	for _, s := range dndSpellRegistry {
		if s.Level != level {
			continue
		}
		for _, c := range s.Classes {
			if c == class {
				return true
			}
		}
	}
	return false
}

func TestParseSpellLooseInput(t *testing.T) {
	cases := []struct{ input, wantID string }{
		{"fire_bolt", "fire_bolt"},
		{"Fire Bolt", "fire_bolt"},
		{"fire-bolt", "fire_bolt"},
		{"  Magic Missile  ", "magic_missile"},
		{"hunters_mark", "hunters_mark"},
	}
	for _, tc := range cases {
		got, ok := parseSpell(tc.input)
		if !ok {
			t.Errorf("parseSpell(%q): not found", tc.input)
			continue
		}
		if got.ID != tc.wantID {
			t.Errorf("parseSpell(%q): id=%q, want %q", tc.input, got.ID, tc.wantID)
		}
	}
	if _, ok := parseSpell("totally not a spell"); ok {
		t.Errorf("parseSpell on garbage should miss")
	}
}

// ── Slot tables ──────────────────────────────────────────────────────────────

func TestSlotsForClassLevel(t *testing.T) {
	cases := []struct {
		class DnDClass
		level int
		want  map[int]int
	}{
		// Mage spec rows (gogobee_spell_system.md §1)
		{ClassMage, 1, map[int]int{1: 2}},
		{ClassMage, 5, map[int]int{1: 4, 2: 3, 3: 2}},
		{ClassMage, 9, map[int]int{1: 4, 2: 3, 3: 3, 4: 3, 5: 1}},
		// Cleric rows
		{ClassCleric, 1, map[int]int{1: 2}},
		{ClassCleric, 5, map[int]int{1: 4, 2: 3, 3: 2}},
		// Ranger half-caster
		{ClassRanger, 1, map[int]int{}},
		{ClassRanger, 5, map[int]int{1: 3}},
		{ClassRanger, 13, map[int]int{1: 3, 2: 2, 3: 1}},
		// Non-caster
		{ClassFighter, 5, nil},
	}
	for _, tc := range cases {
		got := slotsForClassLevel(tc.class, tc.level)
		if !mapEq(got, tc.want) {
			t.Errorf("slotsForClassLevel(%s, %d) = %v, want %v",
				tc.class, tc.level, got, tc.want)
		}
	}
}

func mapEq(a, b map[int]int) bool {
	if (a == nil) != (b == nil) && (len(a)+len(b)) != 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// ── DC math ──────────────────────────────────────────────────────────────────

func TestSpellSaveDC(t *testing.T) {
	// Mage L5, INT 18 (mod +4). Prof L5 = +3. DC = 8 + 3 + 4 = 15.
	c := &DnDCharacter{Class: ClassMage, Level: 5, INT: 18}
	if got := spellSaveDC(c); got != 15 {
		t.Errorf("Mage L5 INT18 DC = %d, want 15", got)
	}
	// Cleric L9, WIS 16 (mod +3). Prof L9 = +4. DC = 8 + 4 + 3 = 15.
	c = &DnDCharacter{Class: ClassCleric, Level: 9, WIS: 16}
	if got := spellSaveDC(c); got != 15 {
		t.Errorf("Cleric L9 WIS16 DC = %d, want 15", got)
	}
	// Ranger L4, WIS 14 (mod +2). Prof L4 = +2. DC = 8 + 2 + 2 = 12.
	c = &DnDCharacter{Class: ClassRanger, Level: 4, WIS: 14}
	if got := spellSaveDC(c); got != 12 {
		t.Errorf("Ranger L4 WIS14 DC = %d, want 12", got)
	}
	// Non-caster falls back to 8 + prof + 0.
	c = &DnDCharacter{Class: ClassFighter, Level: 5, INT: 18}
	if got := spellSaveDC(c); got != 11 {
		t.Errorf("Fighter L5 fallback DC = %d, want 11", got)
	}
}

func TestSpellAttackBonus(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Level: 5, INT: 18}
	if got := spellAttackBonus(c); got != 7 {
		t.Errorf("Mage L5 INT18 atk = %d, want 7", got)
	}
}

// ── Default known list ──────────────────────────────────────────────────────

func TestDefaultKnownSpellsExistInRegistry(t *testing.T) {
	for _, class := range []DnDClass{ClassMage, ClassCleric, ClassRanger} {
		for _, lvl := range []int{1, 3, 5, 9, 13, 20} {
			for _, sid := range defaultKnownSpells(class, lvl) {
				if _, ok := lookupSpell(sid); !ok {
					t.Errorf("default for %s L%d references unknown spell %q",
						class, lvl, sid)
				}
			}
		}
	}
}

func TestDefaultKnownSpellsScaleWithLevel(t *testing.T) {
	// Higher-level Mage knows strictly more spells than L1.
	low := len(defaultKnownSpells(ClassMage, 1))
	hi := len(defaultKnownSpells(ClassMage, 9))
	if hi <= low {
		t.Errorf("Mage L9 known (%d) should exceed L1 (%d)", hi, low)
	}
	// Ranger L1 knows almost nothing (no spells until L2).
	if got := len(defaultKnownSpells(ClassRanger, 1)); got > 2 {
		t.Errorf("Ranger L1 default %d spells, want ≤2 cantrips", got)
	}
}

// ── Concentration helper ────────────────────────────────────────────────────

func TestSetConcentrationSupersedes(t *testing.T) {
	c := &DnDCharacter{}
	prev := setConcentration(c, "bless", 0)
	if prev != "" {
		t.Errorf("first concentration: prev=%q, want empty", prev)
	}
	prev = setConcentration(c, "hunters_mark", 0)
	if prev != "bless" {
		t.Errorf("second concentration: prev=%q, want bless", prev)
	}
	if c.ConcentrationSpell != "hunters_mark" {
		t.Errorf("after super: active=%q, want hunters_mark", c.ConcentrationSpell)
	}
}

// ── Migration safety ────────────────────────────────────────────────────────

func TestEnsureSpellsForCharacterNonCasterIsNoOp(t *testing.T) {
	c := &DnDCharacter{UserID: id.UserID("@test:fighter"), Class: ClassFighter, Level: 5}
	if err := ensureSpellsForCharacter(c); err != nil {
		// Note: this hits the DB; in env without DB, function should still
		// return nil for non-caster before any DB call. Verify that early exit.
		t.Errorf("ensureSpellsForCharacter on Fighter: %v", err)
	}
}

func TestRangerNoSpellsBeforeLevel2(t *testing.T) {
	if got := len(slotsForClassLevel(ClassRanger, 1)); got != 0 {
		t.Errorf("Ranger L1 slot count = %d, want 0", got)
	}
}
