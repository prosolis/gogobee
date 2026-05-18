package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 9 SP4 — Mage `!spells learn` cap + level-up nudge.

// TestMageKnownSpellsCapPerLevel — formula: 6 + 2*(level-1).
func TestMageKnownSpellsCapPerLevel(t *testing.T) {
	cases := []struct {
		level, cap int
	}{
		{1, 6},
		{2, 8},
		{3, 10},
		{5, 14},
		{9, 22},
		{20, 44},
	}
	for _, tc := range cases {
		if got := mageKnownSpellsCap(tc.level); got != tc.cap {
			t.Errorf("mageKnownSpellsCap(L%d)=%d, want %d", tc.level, got, tc.cap)
		}
	}
}

func setupMageForLearn(t *testing.T, uid id.UserID, level int) *DnDCharacter {
	t.Helper()
	if err := createAdvCharacter(uid, "mage_learn"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassMage, Level: level,
		STR: 8, DEX: 14, CON: 12, INT: 17, WIS: 12, CHA: 10,
		HPMax: 20, HPCurrent: 20, ArmorClass: 12,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := setSpellSlotsForLevel(uid, ClassMage, level); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestMageLearnCapEnforced — at L1 the cap is 6 leveled spells. Pre-fill 6;
// the 7th attempt is refused without writing to the spellbook.
func TestMageLearnCapEnforced(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@mage_cap:example")
	c := setupMageForLearn(t, uid, 1)

	// Six leveled mage spells (all L1-eligible).
	pre := []string{
		"magic_missile", "thunderwave", "mage_armor", "burning_hands",
		"grease", "sleep",
	}
	for _, sid := range pre {
		if err := addKnownSpell(uid, sid, "class", true); err != nil {
			t.Fatal(err)
		}
	}
	// Cantrips: don't count.
	if err := addKnownSpell(uid, "fire_bolt", "class", true); err != nil {
		t.Fatal(err)
	}

	count, _ := mageLeveledKnownCount(uid)
	if count != 6 {
		t.Fatalf("setup: leveled count=%d, want 6", count)
	}

	p := &AdventurePlugin{}
	// 7th leveled spell — must be refused.
	if err := p.handleSpellsLearn(MessageContext{Sender: uid}, c, "chromatic_orb"); err != nil {
		t.Fatal(err)
	}
	if known, _, _ := playerKnowsSpell(uid, "chromatic_orb"); known {
		t.Errorf("over-cap learn slipped through")
	}

	// A cantrip should still be learnable beyond the cap.
	if err := p.handleSpellsLearn(MessageContext{Sender: uid}, c, "minor_illusion"); err != nil {
		t.Fatal(err)
	}
	if known, _, _ := playerKnowsSpell(uid, "minor_illusion"); !known {
		t.Errorf("cantrip blocked by leveled-spell cap")
	}
}

// TestMageLearnAcceptsUpToCap — fresh Mage at L1 can learn 6 leveled spells
// before the cap kicks in.
func TestMageLearnAcceptsUpToCap(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@mage_fill:example")
	c := setupMageForLearn(t, uid, 1)

	wantList := []string{
		"magic_missile", "thunderwave", "mage_armor", "burning_hands",
		"grease", "sleep",
	}
	p := &AdventurePlugin{}
	for _, sid := range wantList {
		if err := p.handleSpellsLearn(MessageContext{Sender: uid}, c, sid); err != nil {
			t.Fatal(err)
		}
		if known, _, _ := playerKnowsSpell(uid, sid); !known {
			t.Errorf("under-cap learn rejected: %s", sid)
		}
	}
	if got, _ := mageLeveledKnownCount(uid); got != 6 {
		t.Errorf("after 6 learns, count=%d, want 6", got)
	}
}

// TestMageLevelUpDMNudge — buildLevelUpMessage surfaces the spellbook headroom
// after a Mage levels up.
func TestMageLevelUpDMNudge(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@mage_nudge:example")
	c := setupMageForLearn(t, uid, 2) // post-level-up state

	// 5 leveled known + cantrips. At L2 cap = 8 → headroom = 3.
	for _, sid := range []string{
		"magic_missile", "mage_armor", "shield", "burning_hands", "grease",
	} {
		_ = addKnownSpell(uid, sid, "class", true)
	}
	_ = addKnownSpell(uid, "fire_bolt", "class", true)

	events := []LevelUpEvent{{NewLevel: 2, HPGain: 5}}
	msg := buildLevelUpMessage(c, events, "")

	if !strings.Contains(msg, "Spells available to learn: **3**") {
		t.Errorf("nudge missing or wrong count; msg=\n%s", msg)
	}
	if !strings.Contains(msg, "!spells learn") {
		t.Errorf("nudge missing command hint; msg=\n%s", msg)
	}
}

// TestNonMageLevelUpNoSpellNudge — Fighter levelling never sees the nudge.
func TestNonMageLevelUpNoSpellNudge(t *testing.T) {
	c := &DnDCharacter{
		Race: RaceHuman, Class: ClassFighter, Level: 5,
		HPMax: 50, HPCurrent: 50,
	}
	msg := buildLevelUpMessage(c, []LevelUpEvent{{NewLevel: 5, HPGain: 8}}, "")
	if strings.Contains(msg, "Spells available to learn") {
		t.Errorf("Fighter level-up shouldn't show spell nudge; msg=\n%s", msg)
	}
}

// TestMageLevelUpDeltaIsTwo — leveling from L1 → L2 increases the cap by 2.
func TestMageLevelUpDeltaIsTwo(t *testing.T) {
	for lvl := 1; lvl < 20; lvl++ {
		got := mageKnownSpellsCap(lvl+1) - mageKnownSpellsCap(lvl)
		if got != 2 {
			t.Errorf("delta L%d→L%d = %d, want 2", lvl, lvl+1, got)
		}
	}
}

// TestMageSpellbookLineInRender — renderSpellsList shows the budget for Mage.
func TestMageSpellbookLineInRender(t *testing.T) {
	setupAbilitiesTestDB(t)
	uid := id.UserID("@mage_render:example")
	c := setupMageForLearn(t, uid, 3)
	for _, sid := range []string{"magic_missile", "mage_armor"} {
		_ = addKnownSpell(uid, sid, "class", true)
	}

	out := renderSpellsList(c)
	// L3 cap = 10, count = 2. UX S7 jargon sweep (8bf7d35) reshaped
	// the line to "**Spellbook:** N / M leveled spells learned" and
	// dropped the redundant "(can learn N more)" trailer — the
	// "N / M" already conveys remaining capacity. Assertion tracks
	// the live format; if the line ever loses the count or the cap
	// the substrings still pin the regression.
	if !strings.Contains(out, "**Spellbook:**") ||
		!strings.Contains(out, "2 / 10 leveled spells learned") {
		t.Errorf("renderSpellsList missing/wrong Spellbook line; got\n%s", out)
	}
}
