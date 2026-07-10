package plugin

import (
	"math/rand/v2"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestJournalGrant_DBRoundTripIsAtomicOR(t *testing.T) {
	townTestDB(t)
	u := id.UserID("@page:test.invalid")

	// No row yet → zero pages.
	if mask, err := loadJournalPages(u); err != nil || mask != 0 {
		t.Fatalf("fresh player: mask=%d err=%v, want 0/nil", mask, err)
	}

	// First grant creates the row (ON CONFLICT INSERT path).
	if err := grantJournalPageDB(u, 5); err != nil {
		t.Fatalf("grant page 5: %v", err)
	}
	// Second grant ORs into the existing row without clobbering the first.
	if err := grantJournalPageDB(u, 2); err != nil {
		t.Fatalf("grant page 2: %v", err)
	}
	// Re-granting a found page is idempotent.
	if err := grantJournalPageDB(u, 5); err != nil {
		t.Fatalf("re-grant page 5: %v", err)
	}

	mask, err := loadJournalPages(u)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !journalPageFound(mask, 2) || !journalPageFound(mask, 5) {
		t.Fatalf("pages 2 and 5 should be found, mask=%b", mask)
	}
	if journalPageCount(mask) != 2 {
		t.Fatalf("count=%d, want 2 (mask=%b)", journalPageCount(mask), mask)
	}

	// Overlay populates the character field the viewer reads.
	var c AdventureCharacter
	c.UserID = u
	applyPlayerMetaOverlay(&c)
	if c.JournalPages != mask {
		t.Fatalf("overlay JournalPages=%b, want %b", c.JournalPages, mask)
	}
}

func TestJournalCatalog_WellFormed(t *testing.T) {
	if journalTotalPages != len(journalPages) {
		t.Fatalf("journalTotalPages %d != len(journalPages) %d", journalTotalPages, len(journalPages))
	}
	if journalTotalPages < 1 || journalTotalPages > 63 {
		t.Fatalf("journal must fit an int64 bitmask (1..63); got %d", journalTotalPages)
	}
	if journalTotalPages > len(romanNumerals) {
		t.Fatalf("more pages (%d) than roman numerals (%d)", journalTotalPages, len(romanNumerals))
	}
	seen := map[string]bool{}
	for i, jp := range journalPages {
		if strings.TrimSpace(jp.Title) == "" {
			t.Errorf("page %d has empty title", i+1)
		}
		if strings.TrimSpace(jp.Text) == "" {
			t.Errorf("page %d (%q) has empty text", i+1, jp.Title)
		}
		if seen[jp.Title] {
			t.Errorf("duplicate page title %q", jp.Title)
		}
		seen[jp.Title] = true
	}
}

func TestJournalBitmask_GrantQueryCount(t *testing.T) {
	var mask int64
	if journalPageCount(mask) != 0 {
		t.Fatalf("empty mask should count 0")
	}
	if journalPageFound(mask, 1) {
		t.Fatalf("page 1 should be unfound on empty mask")
	}
	mask = setJournalPageBit(mask, 3)
	mask = setJournalPageBit(mask, 1)
	if !journalPageFound(mask, 3) || !journalPageFound(mask, 1) {
		t.Fatalf("pages 1 and 3 should be found")
	}
	if journalPageFound(mask, 2) {
		t.Fatalf("page 2 should still be unfound")
	}
	if got := journalPageCount(mask); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	// Re-setting a found page is idempotent.
	if again := setJournalPageBit(mask, 3); again != mask {
		t.Fatalf("re-granting page 3 changed the mask")
	}
}

func TestPickUnfoundJournalPage_ExhaustsToZero(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var mask int64
	found := map[int]bool{}
	for i := 0; i < journalTotalPages; i++ {
		page := pickUnfoundJournalPage(mask, rng)
		if page < 1 || page > journalTotalPages {
			t.Fatalf("pick %d out of range", page)
		}
		if found[page] {
			t.Fatalf("pick returned already-found page %d", page)
		}
		found[page] = true
		mask = setJournalPageBit(mask, page)
	}
	if !journalComplete(mask) {
		t.Fatalf("mask should be complete after %d picks", journalTotalPages)
	}
	if page := pickUnfoundJournalPage(mask, rng); page != 0 {
		t.Fatalf("pick on complete mask = %d, want 0", page)
	}
}

func TestRenderJournal_EmptyPartialFull(t *testing.T) {
	// Empty.
	empty := renderJournal(0)
	if !strings.Contains(empty, "0 / ") {
		t.Errorf("empty journal should show 0 found:\n%s", empty)
	}
	if strings.Contains(empty, "…") {
		t.Errorf("empty journal should not render a gap marker:\n%s", empty)
	}

	// Partial with a gap: pages 1 and 3 found, 2 missing → exactly one "…".
	var partial int64
	partial = setJournalPageBit(partial, 1)
	partial = setJournalPageBit(partial, 3)
	pv := renderJournal(partial)
	if !strings.Contains(pv, journalPages[0].Title) || !strings.Contains(pv, journalPages[2].Title) {
		t.Errorf("partial journal missing a found page title:\n%s", pv)
	}
	if strings.Contains(pv, journalPages[1].Title) {
		t.Errorf("partial journal leaked an unfound page's text:\n%s", pv)
	}
	if !strings.Contains(pv, "…") {
		t.Errorf("partial journal with a gap should render '…':\n%s", pv)
	}

	// Full: every title present, no gap, completion line.
	var full int64
	for i := 1; i <= journalTotalPages; i++ {
		full = setJournalPageBit(full, i)
	}
	fv := renderJournal(full)
	if strings.Contains(fv, "…") {
		t.Errorf("complete journal should have no gaps:\n%s", fv)
	}
	for _, jp := range journalPages {
		if !strings.Contains(fv, jp.Title) {
			t.Errorf("complete journal missing page %q", jp.Title)
		}
	}
	if !journalComplete(full) {
		t.Fatalf("full mask should be complete")
	}
}

func TestJournalDropChance_Sane(t *testing.T) {
	if journalPageDropChance <= 0 || journalPageDropChance >= 1 {
		t.Fatalf("drop chance %v out of (0,1)", journalPageDropChance)
	}
}

func TestBossEpilogues_EveryRegisteredZoneHasOne(t *testing.T) {
	for _, z := range allZones() {
		if strings.TrimSpace(bossEpilogueLine(z.ID)) == "" {
			t.Errorf("zone %s (%s) has no boss epilogue", z.ID, z.Display)
		}
	}
	// The synthetic arena is not a campaign zone — it must have none.
	if bossEpilogueLine(ZoneArena) != "" {
		t.Errorf("arena should have no boss epilogue")
	}
	if bossEpilogueLine(ZoneID("nonexistent")) != "" {
		t.Errorf("unknown zone should have no epilogue")
	}
}

func TestHollowKingFinaleMonster_ClonesBelaxathWithBump(t *testing.T) {
	base := dndBestiary["boss_belaxath"]
	if base.ID == "" {
		t.Fatal("boss_belaxath missing from bestiary — finale clones it")
	}
	m := hollowKingFinaleMonster()
	if m.ID != "boss_hollow_king_finale" {
		t.Errorf("finale ID = %q", m.ID)
	}
	if m.Name == base.Name || m.Name == "" {
		t.Errorf("finale name should differ from Belaxath: %q", m.Name)
	}
	if m.HP <= base.HP {
		t.Errorf("finale HP %d should exceed base %d (capstone bump)", m.HP, base.HP)
	}
	// No new mechanics: the ability and defensive profile carry over unchanged.
	if m.Ability != base.Ability {
		t.Errorf("finale ability pointer changed — should reuse Belaxath's, no new mechanics")
	}
	if m.AC != base.AC || m.CR != base.CR {
		t.Errorf("finale AC/CR drifted from base: AC %d/%d CR %v/%v", m.AC, base.AC, m.CR, base.CR)
	}
}

func TestEpilogueRewardOnce_FlagRoundTrip(t *testing.T) {
	townTestDB(t)
	u := id.UserID("@finale:test.invalid")

	if got, err := loadEpilogueCleared(u); err != nil || got {
		t.Fatalf("fresh player: cleared=%v err=%v, want false/nil", got, err)
	}
	if err := markEpilogueClearedDB(u); err != nil {
		t.Fatalf("mark cleared: %v", err)
	}
	if got, err := loadEpilogueCleared(u); err != nil || !got {
		t.Fatalf("after mark: cleared=%v err=%v, want true/nil", got, err)
	}
	// Idempotent.
	if err := markEpilogueClearedDB(u); err != nil {
		t.Fatalf("re-mark: %v", err)
	}

	var c AdventureCharacter
	c.UserID = u
	applyPlayerMetaOverlay(&c)
	if !c.EpilogueCleared {
		t.Fatalf("overlay did not carry EpilogueCleared")
	}
}

func TestEpilogueUnlocked_Gates(t *testing.T) {
	townTestDB(t)
	p := &AdventurePlugin{}

	// Incomplete journal → refused, page-count message.
	partial := &AdventureCharacter{UserID: id.UserID("@u1:test.invalid")}
	partial.JournalPages = setJournalPageBit(0, 1)
	ok, why := p.epilogueUnlocked(partial)
	if ok {
		t.Fatalf("incomplete journal should not unlock")
	}
	if !strings.Contains(why, "journal pages") {
		t.Errorf("expected page-count refusal, got: %s", why)
	}

	// Complete journal but no Tier-5 clears → refused, T5 message.
	full := &AdventureCharacter{UserID: id.UserID("@u2:test.invalid")}
	for i := 1; i <= journalTotalPages; i++ {
		full.JournalPages = setJournalPageBit(full.JournalPages, i)
	}
	ok, why = p.epilogueUnlocked(full)
	if ok {
		t.Fatalf("no T5 clears should not unlock")
	}
	if !strings.Contains(why, "Tier-5") {
		t.Errorf("expected Tier-5 refusal, got: %s", why)
	}
}

func TestFinishEpilogueWin_RepeatIsFlavorOnly(t *testing.T) {
	// The already-cleared branch is pure flavour: no reward, no client calls.
	p := &AdventurePlugin{}
	repeat := p.finishEpilogueWin(id.UserID("@u:test.invalid"), true)
	if strings.Contains(repeat, finaleTitle) {
		t.Errorf("a repeat clear must not re-award the title: %s", repeat)
	}
	if strings.TrimSpace(repeat) == "" {
		t.Errorf("a repeat clear should still say something")
	}
}

func TestTwinBeeJournalReaction_DeterministicAndGuarded(t *testing.T) {
	if twinBeeJournalReaction(3, 0) != "" {
		t.Errorf("zero pages should produce no reaction")
	}
	first := twinBeeJournalReaction(3, 2)
	if first == "" {
		t.Fatalf("a found page should produce a reaction")
	}
	if again := twinBeeJournalReaction(3, 2); again != first {
		t.Errorf("reaction not deterministic: %q vs %q", first, again)
	}
	// Voice guard: TwinBee never speaks of himself in the third person
	// (feedback_twinbee_voice). None of the pooled lines may contain "TwinBee".
	for _, line := range twinBeeJournalReactions {
		if strings.Contains(line, "TwinBee") {
			t.Errorf("third-person TwinBee reference in reaction: %q", line)
		}
	}
}
