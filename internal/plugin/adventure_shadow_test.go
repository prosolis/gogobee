package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newShadowTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// freshChar builds a same-day character so shadowPlayerAgeDays reads 1 day.
func freshChar(user id.UserID) *AdventureCharacter {
	return &AdventureCharacter{
		UserID:      user,
		DisplayName: "Tester",
		CreatedAt:   time.Now().UTC(),
	}
}

func TestShadowName_DeterministicAndInPool(t *testing.T) {
	user := id.UserID("@name:test.invalid")
	first := shadowNameFor("Aria", user)
	if again := shadowNameFor("Aria", user); again != first {
		t.Errorf("name not deterministic: %q vs %q", first, again)
	}
	inPool := false
	for _, n := range shadowNames {
		if n == first {
			inPool = true
			break
		}
	}
	if !inPool {
		t.Errorf("generated name %q not in shadowNames pool", first)
	}
	// Empty display name falls back to the user id and still yields a pool name.
	if got := shadowNameFor("", user); got == "" {
		t.Error("empty display name produced an empty shadow name")
	}
}

// TestShadowFlavor_NoThirdPersonTwinBee: every Shadow line TwinBee speaks obeys
// the campaign voice rule — he never refers to himself in the third person
// (feedback_twinbee_voice). Same guard the journal reactions carry.
func TestShadowFlavor_NoThirdPersonTwinBee(t *testing.T) {
	pools := [][]string{shadowAheadLines, shadowBehindLines, shadowNeckLines}
	for _, pool := range pools {
		for _, line := range pool {
			if strings.Contains(line, "TwinBee") {
				t.Errorf("third-person TwinBee in Shadow pool line: %q", line)
			}
			if !strings.Contains(line, "%s") {
				t.Errorf("race-pressure line missing the name slot: %q", line)
			}
		}
	}
	s := &shadowState{Name: "Vael"}
	rendered := []string{
		shadowLeftPageLine(s),
		shadowBeatYouHereLine(s),
		shadowCrowLine(s, 24),
		shadowRacePressureLine(s, 3, 1),
		shadowRacePressureLine(s, 3, -1),
		shadowRacePressureLine(s, 3, 0),
	}
	for _, r := range rendered {
		if strings.Contains(r, "TwinBee") {
			t.Errorf("third-person TwinBee in rendered Shadow line: %q", r)
		}
		if !strings.Contains(r, "Vael") {
			t.Errorf("rendered line dropped the Shadow's name: %q", r)
		}
	}
}

func TestShadowRacePressure_Deterministic(t *testing.T) {
	s := &shadowState{Name: "Vael", DayCounter: 2}
	first := shadowRacePressureLine(s, 4, 1)
	if again := shadowRacePressureLine(s, 4, 1); again != first {
		t.Errorf("race pressure not deterministic: %q vs %q", first, again)
	}
	// Direction picks a different pool: ahead vs behind must differ in wording.
	ahead := shadowRacePressureLine(s, 4, 2)
	behind := shadowRacePressureLine(s, 4, -2)
	if ahead == behind {
		t.Errorf("ahead and behind produced the same line: %q", ahead)
	}
}

func TestShadowAdvance_CreepsAndIsIdempotent(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@creep:test.invalid")
	char := freshChar(user)

	p.advanceShadow(char)
	s, err := loadShadow(user)
	if err != nil || s == nil {
		t.Fatalf("shadow not born: %v", err)
	}
	if s.Progress <= 0 {
		t.Errorf("a born shadow should have crept forward, got progress %v", s.Progress)
	}
	if s.Name == "" {
		t.Error("born shadow has no name")
	}
	firstProg := s.Progress

	// Second advance the same UTC day is a no-op.
	p.advanceShadow(char)
	s2, _ := loadShadow(user)
	if s2.Progress != firstProg {
		t.Errorf("same-day re-advance moved the shadow: %v -> %v", firstProg, s2.Progress)
	}

	// Roll the clock back a day and it advances again.
	s2.LastAdvanced = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	if err := upsertShadow(s2); err != nil {
		t.Fatal(err)
	}
	p.advanceShadow(char)
	s3, _ := loadShadow(user)
	if s3.Progress <= firstProg {
		t.Errorf("a new day should have advanced the shadow: %v -> %v", firstProg, s3.Progress)
	}
}

func TestShadowAdvance_PendingBitForUnclearedZone(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@pending:test.invalid")
	char := freshChar(user)

	// Seed the shadow one step short of clearing zone 0, dated yesterday so the
	// next advance runs. The player has cleared nothing.
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	seed := newShadow(user, char.DisplayName)
	seed.Progress = 0.95
	seed.LastAdvanced = yesterday
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	p.advanceShadow(char)
	s, _ := loadShadow(user)
	if s.ZonesCleared < 1 {
		t.Fatalf("shadow should have cleared zone 0, zones_cleared=%d prog=%v", s.ZonesCleared, s.Progress)
	}
	if !shadowBitSet(s.PendingMask, 0) {
		t.Errorf("shadow cleared zone 0 before the player but left no pending page (mask=%b)", s.PendingMask)
	}
}

func TestShadowAdvance_NoPendingWhenPlayerClearedFirst(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@nopending:test.invalid")
	char := freshChar(user)

	// Player already cleared zone 0.
	insertClearedExpedition(t, user, zoneOrder[0], ExpeditionStatusComplete, 1, "{}")

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	seed := newShadow(user, char.DisplayName)
	seed.Progress = 0.95
	seed.LastAdvanced = yesterday
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	p.advanceShadow(char)
	s, _ := loadShadow(user)
	if s.ZonesCleared < 1 {
		t.Fatalf("shadow should have cleared zone 0, zones_cleared=%d", s.ZonesCleared)
	}
	if shadowBitSet(s.PendingMask, 0) {
		t.Errorf("player cleared zone 0 first — no page should wait (mask=%b)", s.PendingMask)
	}
}

func TestShadowAdvance_LeadCapKeepsItARace(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@leadcap:test.invalid")
	char := freshChar(user) // player has cleared nothing → count 0

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	seed := newShadow(user, char.DisplayName)
	seed.Progress = shadowMaxLead // already at the cap for a 0-clear player
	seed.ZonesCleared = 2
	seed.LastAdvanced = yesterday
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	p.advanceShadow(char)
	s, _ := loadShadow(user)
	if s.Progress > shadowMaxLead+0.0001 {
		t.Errorf("shadow ran past the lead cap: progress %v > %v", s.Progress, shadowMaxLead)
	}
}

func TestShadowZoneClear_PendingGrantsWaitingPage(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@page:test.invalid")

	idx := 2
	seed := newShadow(user, "Tester")
	seed.Progress = 5
	seed.ZonesCleared = 5
	seed.PendingMask = shadowSetBit(0, idx)
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	line := p.shadowOnPlayerZoneClear(user, zoneOrder[idx])
	if strings.TrimSpace(line) == "" {
		t.Fatal("a waiting page should have produced a clear-message line")
	}
	if !strings.Contains(line, "page") {
		t.Errorf("expected a journal-page grant in the line, got: %q", line)
	}
	// Bit cleared so a re-clear can't re-award.
	s, _ := loadShadow(user)
	if shadowBitSet(s.PendingMask, idx) {
		t.Error("pending bit not cleared after the page was granted")
	}
	// The page actually landed.
	if mask, _ := loadJournalPages(user); journalPageCount(mask) != 1 {
		t.Errorf("expected exactly one journal page granted, got %d", journalPageCount(mask))
	}

	// A second clear of the same zone grants nothing more.
	again := p.shadowOnPlayerZoneClear(user, zoneOrder[idx])
	if again != "" {
		t.Errorf("re-clearing a paid-out zone should be silent, got: %q", again)
	}
	if mask, _ := loadJournalPages(user); journalPageCount(mask) != 1 {
		t.Errorf("re-clear granted a second page (count %d)", journalPageCount(mask))
	}
}

func TestShadowZoneClear_PlayerFirstCrows(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@crow:test.invalid")

	idx := 3
	seed := newShadow(user, "Tester")
	seed.Progress = 0 // shadow hasn't reached this zone
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	line := p.shadowOnPlayerZoneClear(user, zoneOrder[idx])
	if !strings.Contains(line, "XP") {
		t.Errorf("beating the shadow should crow a bonus, got: %q", line)
	}
	// No page granted on a crow.
	if mask, _ := loadJournalPages(user); journalPageCount(mask) != 0 {
		t.Errorf("a crow should not grant a page (count %d)", journalPageCount(mask))
	}
	// The crowed bit is set so a re-run of the same not-yet-reached zone can't
	// farm the XP again — second clear is silent.
	if s, _ := loadShadow(user); !shadowBitSet(s.CrowedMask, idx) {
		t.Error("crowed bit not set after the first crow")
	}
	if again := p.shadowOnPlayerZoneClear(user, zoneOrder[idx]); again != "" {
		t.Errorf("re-clearing an already-crowed zone should be silent, got: %q", again)
	}
}

func TestShadowZoneClear_ShadowPassedNoDebt(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@passed:test.invalid")

	idx := 1
	seed := newShadow(user, "Tester")
	seed.Progress = 5 // well past this zone
	seed.ZonesCleared = 5
	seed.PendingMask = 0 // nothing owed here
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}

	if line := p.shadowOnPlayerZoneClear(user, zoneOrder[idx]); line != "" {
		t.Errorf("a zone the shadow passed with nothing owed should be silent, got: %q", line)
	}
}

func TestShadowZoneClear_SilentBeforeBirth(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@unborn:test.invalid")
	if line := p.shadowOnPlayerZoneClear(user, zoneOrder[0]); line != "" {
		t.Errorf("no shadow yet should be silent, got: %q", line)
	}
}

func TestShadowBriefingLine_SilentUntilBorn(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	user := id.UserID("@brief:test.invalid")
	e := &Expedition{UserID: string(user), ZoneID: zoneOrder[0], CurrentDay: 2}

	if line := p.shadowBriefingLine(e); line != "" {
		t.Errorf("no shadow should mean no briefing line, got: %q", line)
	}

	seed := newShadow(user, "Tester")
	seed.ZonesCleared = 2
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}
	if line := p.shadowBriefingLine(e); strings.TrimSpace(line) == "" {
		t.Error("a born shadow should produce a briefing line")
	}
}

func TestHandleShadowCmd_DMsStatus(t *testing.T) {
	newShadowTestDB(t)
	p := &AdventurePlugin{}
	sink := installSink(p)
	user := id.UserID("@cmd:test.invalid")

	// Before birth: a gentle "no one's shadowing you yet".
	if err := p.handleShadowCmd(MessageContext{Sender: user}); err != nil {
		t.Fatal(err)
	}
	if dms := sink.dmsTo(user); len(dms) != 1 || !strings.Contains(dms[0], "shadowing you yet") {
		t.Errorf("expected an unborn-shadow DM, got: %v", sink.dmsTo(user))
	}

	seed := newShadow(user, "Tester")
	seed.ZonesCleared = 4
	if err := upsertShadow(seed); err != nil {
		t.Fatal(err)
	}
	if err := p.handleShadowCmd(MessageContext{Sender: user}); err != nil {
		t.Fatal(err)
	}
	dms := sink.dmsTo(user)
	if len(dms) != 2 || !strings.Contains(dms[1], seed.Name) {
		t.Errorf("expected a status DM naming the shadow, got: %v", dms)
	}
}
