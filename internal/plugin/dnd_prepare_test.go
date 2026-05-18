package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 9 SP4 — !prepare command tests for Cleric.

func setupClericForPrepare(t *testing.T, uid id.UserID, wis, level int) *DnDCharacter {
	t.Helper()
	if err := createAdvCharacter(uid, "prep_test"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassCleric, Level: level,
		STR: 12, DEX: 12, CON: 14, INT: 10, WIS: wis, CHA: 12,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := setSpellSlotsForLevel(uid, ClassCleric, level); err != nil {
		t.Fatal(err)
	}
	return c
}

func countPrepared(t *testing.T, uid id.UserID) int {
	t.Helper()
	rows, err := listKnownSpells(uid)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range rows {
		s, ok := lookupSpell(r.SpellID)
		if !ok || s.Level == 0 {
			continue
		}
		if r.Prepared {
			n++
		}
	}
	return n
}

func isPrepared(t *testing.T, uid id.UserID, spellID string) bool {
	t.Helper()
	_, prepared, err := playerKnowsSpell(uid, spellID)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

// TestPrepareCap — Cleric L3 with WIS 16 (mod +3): cap = 3 + 3 = 6.
// Adding a 7th leveled prep is rejected; cantrips don't count toward the cap.
func TestPrepareCap(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@prep_cap:example")
	setupClericForPrepare(t, uid, 16 /*WIS*/, 3 /*level*/)

	// Cantrips: not counted by the cap. Pre-grant some Cleric cantrips
	// already-prepared — they must not block leveled-spell prep.
	for _, sid := range []string{"sacred_flame", "guidance", "mending"} {
		if err := addKnownSpell(uid, sid, "class", true); err != nil {
			t.Fatal(err)
		}
	}

	// Add 7 known leveled spells, all unprepared.
	leveled := []string{
		"cure_wounds", "healing_word", "bless", "guiding_bolt",
		"shield_of_faith", "spiritual_weapon", "aid",
	}
	for _, sid := range leveled {
		if err := addKnownSpell(uid, sid, "class", false); err != nil {
			t.Fatal(err)
		}
	}

	p := &AdventurePlugin{}
	// Prepare 6 — all should succeed.
	for i := 0; i < 6; i++ {
		if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, leveled[i]); err != nil {
			t.Fatal(err)
		}
		if !isPrepared(t, uid, leveled[i]) {
			t.Fatalf("expected %s to be prepared after handleDnDPrepareCmd", leveled[i])
		}
	}
	if got := countPrepared(t, uid); got != 6 {
		t.Fatalf("after 6 preps, count=%d, want 6", got)
	}

	// 7th must be rejected — handler returns nil but state should not change.
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, leveled[6]); err != nil {
		t.Fatal(err)
	}
	if isPrepared(t, uid, leveled[6]) {
		t.Errorf("7th spell prepared past cap of 6")
	}
	if got := countPrepared(t, uid); got != 6 {
		t.Errorf("over-cap prep changed count: got %d, want 6", got)
	}
}

// TestPrepareCapFloor — Cleric L1, WIS 1 (mod -5): raw cap = -4, floored to 1.
func TestPrepareCapFloor(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@prep_floor:example")
	setupClericForPrepare(t, uid, 1 /*WIS — mod -5*/, 1)

	for _, sid := range []string{"cure_wounds", "bless"} {
		if err := addKnownSpell(uid, sid, "class", false); err != nil {
			t.Fatal(err)
		}
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "cure_wounds"); err != nil {
		t.Fatal(err)
	}
	if !isPrepared(t, uid, "cure_wounds") {
		t.Fatal("first prep should succeed at floored cap of 1")
	}
	// Second must be rejected.
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "bless"); err != nil {
		t.Fatal(err)
	}
	if isPrepared(t, uid, "bless") {
		t.Errorf("second prep slipped past floor cap of 1")
	}
}

// TestPrepareClearAllowsCast — `!prepare clear <spell>` removes the prep flag,
// and `!cast` then refuses the unprepared spell (slot is preserved).
func TestPrepareClearAllowsCast(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@prep_clear:example")
	setupClericForPrepare(t, uid, 14 /*+2*/, 3)

	if err := addKnownSpell(uid, "cure_wounds", "class", true); err != nil {
		t.Fatal(err)
	}
	if !isPrepared(t, uid, "cure_wounds") {
		t.Fatal("setup: cure_wounds not prepared")
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "clear cure_wounds"); err != nil {
		t.Fatal(err)
	}
	if isPrepared(t, uid, "cure_wounds") {
		t.Errorf("prep clear failed to remove flag")
	}

	// Slot count before cast attempt.
	beforeSlots, _ := getSpellSlots(uid)
	beforeUsed := beforeSlots[1][1]

	// !cast cure_wounds while unprepared — handler returns nil (DM error),
	// but no slot consumed, no pending_cast.
	if err := p.handleDnDCastCmd(MessageContext{Sender: uid}, "cure_wounds"); err != nil {
		t.Fatal(err)
	}
	afterSlots, _ := getSpellSlots(uid)
	afterUsed := afterSlots[1][1]
	if afterUsed != beforeUsed {
		t.Errorf("unprepared cast consumed a slot: used %d → %d", beforeUsed, afterUsed)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.PendingCast != "" {
		t.Errorf("unprepared cast queued pending_cast=%q", got.PendingCast)
	}
}

// TestPrepareCantripsSkipCap — preparing a cantrip is a no-op (cantrips are
// always prepared). Cap is not consulted; counts don't shift.
func TestPrepareCantripsSkipCap(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@prep_cantrip:example")
	setupClericForPrepare(t, uid, 10 /*+0, level 1: cap floored to 1*/, 1)

	if err := addKnownSpell(uid, "sacred_flame", "class", false); err != nil {
		t.Fatal(err)
	}
	// Fill the leveled cap so a leveled !prepare would be refused.
	if err := addKnownSpell(uid, "cure_wounds", "class", true); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "sacred_flame"); err != nil {
		t.Fatal(err)
	}
	// Cantrip-prep doesn't toggle the prepared flag (handler short-circuits
	// with "Cantrips are always prepared." before calling setSpellPrepared).
	// What matters is that it didn't push a leveled-spell off and didn't
	// change the leveled prep count.
	if got := countPrepared(t, uid); got != 1 {
		t.Errorf("cantrip prep changed leveled count: got %d, want 1", got)
	}
}

// TestPrepareUnknownSpellErrors — !prepare on garbage/unknown spell DMs an
// error and never touches the DB.
func TestPrepareUnknownSpellErrors(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@prep_unknown:example")
	setupClericForPrepare(t, uid, 14, 1)

	p := &AdventurePlugin{}
	// Unknown spell name.
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "totally not a spell"); err != nil {
		t.Fatal(err)
	}
	rows, _ := listKnownSpells(uid)
	if len(rows) != 0 {
		t.Errorf("unknown spell prep populated known list: %+v", rows)
	}

	// Spell exists but isn't on the player's known list.
	if err := p.handleDnDPrepareCmd(MessageContext{Sender: uid}, "fireball"); err != nil {
		t.Fatal(err)
	}
	if isPrepared(t, uid, "fireball") {
		t.Errorf("fireball prepared without being known")
	}
}
