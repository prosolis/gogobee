package plugin

// Scenario tests run against a copy of the prod DB (data/gogobee.db).
// Gated on GOGOBEE_PRODDB_SCENARIOS=1 so they don't run on default
// `go test ./...` invocations. Pattern mirrors setupAuditTestDB.
//
// Run with: GOGOBEE_PRODDB_SCENARIOS=1 go test -run TestScenario_ -v \
//             ./internal/plugin/

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func requireScenarioEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("GOGOBEE_PRODDB_SCENARIOS") != "1" {
		t.Skip("scenario tests gated on GOGOBEE_PRODDB_SCENARIOS=1")
	}
}

// ── Scenario: Josie caster-aid bootstraps ──────────────────────────────────
//
// Verifies (against a copy of the live DB) that the two 2026-06-18 caster-aid
// bootstraps land for @holymachina: the spell backfill adds inflict_wounds to
// her known+prepared book, and the pet gift grants a L10 dog mirrored into
// player_meta. Both must be idempotent on a second run.
func TestScenario_JosieCasterAid(t *testing.T) {
	requireScenarioEnv(t)
	setupAuditTestDB(t)

	const uid = id.UserID("@holymachina:parodia.dev")

	hasInflict := func() bool {
		known, err := listKnownSpells(uid)
		if err != nil {
			t.Fatalf("list known: %v", err)
		}
		for _, k := range known {
			if k.SpellID == "inflict_wounds" {
				return k.Prepared
			}
		}
		return false
	}

	if hasInflict() {
		t.Fatal("precondition failed: target already knows inflict_wounds")
	}
	char, err := loadAdvCharacter(uid)
	if err != nil || char == nil {
		t.Fatalf("load target char: %v", err)
	}
	if char.PetArrived || char.PetType != "" {
		t.Fatal("precondition failed: target already has a pet")
	}

	// Run twice to prove idempotency.
	for i := 0; i < 2; i++ {
		bootstrapCasterSpellBackfill()
		bootstrapGrantStarterPet()
	}

	if !hasInflict() {
		t.Error("spell backfill did not add inflict_wounds as prepared")
	}

	got, err := loadAdvCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("reload target char: %v", err)
	}
	if !got.PetArrived || got.PetType != "dog" || got.PetLevel != 10 {
		t.Errorf("pet grant: arrived=%v type=%q level=%d, want true/dog/10",
			got.PetArrived, got.PetType, got.PetLevel)
	}
	pet, err := loadPetState(uid)
	if err != nil {
		t.Fatalf("load pet state: %v", err)
	}
	if !pet.HasPet() || pet.Level != 10 {
		t.Errorf("player_meta pet mirror: hasPet=%v level=%d, want true/10", pet.HasPet(), pet.Level)
	}
}

// ── Scenario 1: Phase 5-B HP bootstrap ─────────────────────────────────────
//
// Expected: bootstrapPhase5BHPRefresh() walks dnd_character rows where
// dnd_level > 0, refreshes hp_max upward toward computeMaxHP (which
// applies phase5BHPMult=1.5), bumps hp_current by the same delta, and
// marks the daily_prefetch job key so reruns are no-ops.
func TestScenario_Phase5BHPBootstrap(t *testing.T) {
	requireScenarioEnv(t)
	setupAuditTestDB(t)

	type charRow struct {
		userID    string
		class     string
		level     int
		conScore  int
		hpMax     int
		hpCurrent int
	}
	snapshot := func() map[string]charRow {
		rows, err := db.Get().Query(`
			SELECT user_id, class, dnd_level, con_score, hp_max, hp_current
			FROM dnd_character WHERE dnd_level > 0`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		out := map[string]charRow{}
		for rows.Next() {
			var r charRow
			if err := rows.Scan(&r.userID, &r.class, &r.level, &r.conScore, &r.hpMax, &r.hpCurrent); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out[r.userID] = r
		}
		return out
	}

	before := snapshot()
	t.Logf("[pre-bootstrap] %d characters with dnd_level > 0", len(before))
	for _, r := range before {
		t.Logf("  %s class=%q L%d con=%d hp=%d/%d",
			r.userID, r.class, r.level, r.conScore, r.hpCurrent, r.hpMax)
	}

	if db.JobCompleted("phase5b_hp_refresh_v1", "once") {
		t.Fatalf("job already marked completed before bootstrap — snapshot was post-bootstrap?")
	}

	bootstrapPhase5BHPRefresh()

	after := snapshot()
	if !db.JobCompleted("phase5b_hp_refresh_v1", "once") {
		t.Errorf("expected JobCompleted=true after bootstrap")
	}

	refreshed := 0
	for uid, b := range before {
		a := after[uid]
		conMod := abilityModifier(b.conScore)
		_, ok := classInfo(DnDClass(b.class))
		var expectedMax int
		if !ok {
			expectedMax = 1 // computeMaxHP returns 1 for unknown class.
		} else {
			expectedMax = computeMaxHP(DnDClass(b.class), conMod, b.level)
		}
		// Bootstrap skips rows where newMax <= oldMax (never lowers HP).
		if expectedMax <= b.hpMax {
			if a.hpMax != b.hpMax {
				t.Errorf("%s: expected hp_max unchanged (%d), got %d", uid, b.hpMax, a.hpMax)
			}
			continue
		}
		delta := expectedMax - b.hpMax
		expectedCurrent := b.hpCurrent + delta
		if expectedCurrent > expectedMax {
			expectedCurrent = expectedMax
		}
		if expectedCurrent < 1 {
			expectedCurrent = 1
		}
		if a.hpMax != expectedMax {
			t.Errorf("%s: hp_max want %d got %d", uid, expectedMax, a.hpMax)
		}
		if a.hpCurrent != expectedCurrent {
			t.Errorf("%s: hp_current want %d (was %d, +delta %d) got %d",
				uid, expectedCurrent, b.hpCurrent, delta, a.hpCurrent)
		}
		// Wound-preservation invariant: absolute wound (max-current) stays
		// constant unless clamped at floor 1 or at the new ceiling.
		preWound := b.hpMax - b.hpCurrent
		postWound := a.hpMax - a.hpCurrent
		if preWound != postWound && expectedCurrent != 1 && expectedCurrent != expectedMax {
			t.Errorf("%s: wound size changed pre=%d post=%d (no clamp expected)",
				uid, preWound, postWound)
		}
		refreshed++
		t.Logf("[refreshed] %s: hp_max %d→%d (+%d), hp_current %d→%d",
			uid, b.hpMax, a.hpMax, delta, b.hpCurrent, a.hpCurrent)
	}
	t.Logf("[post-bootstrap] %d/%d characters refreshed", refreshed, len(before))

	// Idempotency: second call is a no-op.
	bootstrapPhase5BHPRefresh()
	after2 := snapshot()
	for uid, a := range after {
		if after2[uid].hpMax != a.hpMax || after2[uid].hpCurrent != a.hpCurrent {
			t.Errorf("%s: second bootstrap call mutated HP", uid)
		}
	}
}

// ── Scenario 2: Magic-item plumbing ────────────────────────────────────────
//
// Expected:
//   - magic_item_equipped table exists (Phase 5 migration).
//   - magicItemRegistry is non-empty; rarity index covers every rarity.
//   - Slot classifier output (baked into magic_items_srd_data.go via gen)
//     puts known edge-case items in the right slot per the UX S4 fix.
//   - dailyCuriosStock() returns a non-empty rotating shelf.
func TestScenario_MagicItemPlumbing(t *testing.T) {
	requireScenarioEnv(t)
	setupAuditTestDB(t)

	// (a) Migration created the table.
	var n int
	err := db.Get().QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='magic_item_equipped'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("magic_item_equipped table missing (n=%d err=%v)", n, err)
	}

	// (b) Schema sanity — required columns.
	cols, err := db.Get().Query(`PRAGMA table_info(magic_item_equipped)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer cols.Close()
	have := map[string]bool{}
	for cols.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		_ = cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		have[name] = true
	}
	for _, want := range []string{"user_id", "item_id", "slot"} {
		if !have[want] {
			t.Errorf("magic_item_equipped missing column %q (have: %v)", want, have)
		}
	}

	// (c) Registry populated and rarity index covers every rarity.
	if len(magicItemRegistry) == 0 {
		t.Fatalf("magicItemRegistry is empty")
	}
	t.Logf("magicItemRegistry: %d items", len(magicItemRegistry))
	byRarity := magicItemsByRarity()
	for _, r := range []DnDRarity{RarityCommon, RarityUncommon, RarityRare, RarityVeryRare, RarityLegendary} {
		if len(byRarity[r]) == 0 {
			t.Errorf("rarity %q has no items in index", r)
		} else {
			t.Logf("  rarity %s: %d items", r, len(byRarity[r]))
		}
	}

	// (d) Slot baking — known edge-case items from UX S4 B4 should land
	// in the slot the fix intended. Slots are baked into the generated
	// data file by the importer's classifier; the lookup is a fixed table.
	type slotCheck struct {
		id          string
		wantSlot    DnDSlot
		mustNotBeRingForSubstring string // sanity vs word-boundary regressions
	}
	checks := []slotCheck{
		{"ring_of_protection", DnDSlotRing1, ""},
		{"boots_of_striding_and_springing", DnDSlotFeet, "springing"},
		{"gloves_of_missile_snaring", DnDSlotHands, "snaring"},
		{"bag_of_devouring", "", "devouring"}, // wondrous w/ no carryable noun
		{"cloak_of_displacement", DnDSlotCloak, ""},
	}
	for _, c := range checks {
		item, ok := magicItemRegistry[c.id]
		if !ok {
			t.Errorf("registry missing %q", c.id)
			continue
		}
		if c.wantSlot != "" && item.Slot != c.wantSlot {
			t.Errorf("%s: slot=%q want %q", c.id, item.Slot, c.wantSlot)
		}
		if item.Slot == DnDSlotRing1 || item.Slot == DnDSlotRing2 {
			if c.mustNotBeRingForSubstring != "" {
				t.Errorf("%s: misclassified as ring (substring trap %q)",
					c.id, c.mustNotBeRingForSubstring)
			}
		}
		t.Logf("  %s → kind=%s slot=%q rarity=%s attune=%v",
			c.id, item.Kind, item.Slot, item.Rarity, item.Attunement)
	}

	// (e) Daily curios shelf rotates and returns a non-empty list.
	shelf := dailyCuriosStock()
	if len(shelf) == 0 {
		t.Errorf("dailyCuriosStock returned empty")
	} else {
		t.Logf("dailyCuriosStock: %d items (first: %s @ %d, rarity %s)",
			len(shelf), shelf[0].Name, shelf[0].Value, shelf[0].Rarity)
	}
}

// ── Scenario 3: Expedition autopilot plumbing ──────────────────────────────
//
// Expected:
//   - dnd_expedition.last_ambient_at column exists (Phase 3 migration).
//   - autopilotFooter renders non-empty paused-state copy for pause
//     reasons; renders empty for terminal/already-narrated reasons.
//   - Ambient event pool has positive weights and non-empty flavor pools.
func TestScenario_ExpeditionAutopilotPlumbing(t *testing.T) {
	requireScenarioEnv(t)
	setupAuditTestDB(t)

	// (a) Migration column present.
	cols, err := db.Get().Query(`PRAGMA table_info(dnd_expedition)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer cols.Close()
	have := map[string]bool{}
	for cols.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		_ = cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		have[name] = true
	}
	if !have["last_ambient_at"] {
		t.Errorf("dnd_expedition.last_ambient_at missing — Phase 3 migration didn't run")
	}

	// (b) Stop-reason footers — pause reasons render copy, terminal
	// reasons render empty (death narration / completion block / etc.
	// is the final).
	type footerCheck struct {
		reason   stopReason
		wantText bool
	}
	for _, c := range []footerCheck{
		{stopFork, true},
		{stopElite, true},
		{stopBoss, true},
		{stopHarvestCombat, true},
		{stopOK, true}, // hit room cap → "stretch complete"
		{stopEnded, false},
		{stopComplete, false},
		{stopBlocked, false},
		{stopBossSafety, false}, // res.final carries the held-back line
	} {
		got := autopilotFooter(c.reason, 3)
		if c.wantText && got == "" {
			t.Errorf("stop reason %v: expected non-empty footer, got empty", c.reason)
		}
		if !c.wantText && got != "" {
			t.Errorf("stop reason %v: expected empty footer, got %q", c.reason, got)
		}
		t.Logf("  %v (3 rooms) → %q", c.reason, got)
	}

	// (c) Ambient event pool — every event has a positive weight and a
	// non-empty flavor pool. Build a temporary Expedition to satisfy
	// pickAmbientEvent's eligibility predicates without persisting.
	events := ambientEvents()
	if len(events) == 0 {
		t.Fatalf("ambientEvents() empty")
	}
	t.Logf("ambient pool: %d events", len(events))
	for _, ev := range events {
		if ev.Weight <= 0 {
			t.Errorf("ambient event %q has non-positive weight %d", ev.Kind, ev.Weight)
		}
		if len(ev.Pool) == 0 {
			t.Errorf("ambient event %q has empty pool", ev.Kind)
		} else {
			t.Logf("  %-22s weight=%d  pool=%d  sample=%q",
				ev.Kind, ev.Weight, len(ev.Pool), ev.Pool[0])
		}
	}
}

// ── Scenario 4: Spell/help jargon regression ───────────────────────────────
//
// Expected: live merged spell registry has no banned-phrase leaks across
// Description. Mirrors TestSpellDescriptionsAreJargonFree at runtime so
// it shows up in this pass.
func TestScenario_SpellJargonRegression(t *testing.T) {
	requireScenarioEnv(t)
	setupAuditTestDB(t)

	// Mirror dnd_spells_prose_test's TestSpellDescriptionsAreJargonFree:
	// substring matches for jargon, regex for the SRD-importer placeholder
	// signature ("Whatever " + lowercase, distinct from legit contractions).
	bannedSubstrings := []string{
		"saving throw", "Saving Throw",
		"spell slot of", "Spell Slot of",
		"ability modifier", "Ability Modifier",
		"hit points equal to",
	}
	bannedRegexes := []*regexp.Regexp{
		regexp.MustCompile(`\bd(4|6|8|10|12|20|100)\b`),
		regexp.MustCompile(`\b\d+d\d+\b`),
		regexp.MustCompile(`\bWhatever [a-z]`),       // placeholder signature
		regexp.MustCompile(`(?:\.\.\.|…)\s*$`),       // trailing ellipsis
		regexp.MustCompile(`\b[a-z]{1,3}(?:\.\.\.|…)\s*$`), // truncated word
		regexp.MustCompile(`(?i)\bno larger than in any\b`),
	}
	totalSpells := 0
	leaks := 0
	for id, s := range dndSpellRegistry {
		totalSpells++
		desc := s.Description
		if desc == "" {
			continue
		}
		for _, b := range bannedSubstrings {
			if strings.Contains(desc, b) {
				t.Errorf("spell %q leaks banned phrase %q: %q", id, b, desc)
				leaks++
			}
		}
		for _, re := range bannedRegexes {
			if re.MatchString(desc) {
				t.Errorf("spell %q matches banned pattern %v: %q", id, re, desc)
				leaks++
			}
		}
	}
	t.Logf("scanned %d spells; %d jargon leaks", totalSpells, leaks)
}
