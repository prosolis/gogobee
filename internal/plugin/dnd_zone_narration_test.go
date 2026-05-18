package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/flavor"

	"maunium.net/go/mautrix/id"
)

// Phase 11 D1d — TwinBee narration + mood event tests.

func TestMoodBand_Boundaries(t *testing.T) {
	cases := []struct {
		score int
		want  MoodBand
	}{
		{0, MoodBandHostile}, {19, MoodBandHostile},
		{20, MoodBandGrumpy}, {39, MoodBandGrumpy},
		{40, MoodBandNeutral}, {59, MoodBandNeutral},
		{60, MoodBandFriendly}, {79, MoodBandFriendly},
		{80, MoodBandEffusive}, {100, MoodBandEffusive},
	}
	for _, c := range cases {
		if got := moodBand(c.score); got != c.want {
			t.Errorf("moodBand(%d) = %d, want %d", c.score, got, c.want)
		}
	}
}

func TestMoodEventDelta_DesignDocTable(t *testing.T) {
	want := map[DMMoodEvent]int{
		MoodEventZoneComplete:       +10,
		MoodEventPlayerDeath:        -5,
		MoodEventNat20:              +3,
		MoodEventNat1:               -2,
		MoodEventCommunityMilestone: +15,
		MoodEventDowntime:           -20,
		MoodEventTaunt:              -10,
		MoodEventCompliment:         +5,
	}
	for ev, delta := range want {
		if got := MoodEventDelta(ev); got != delta {
			t.Errorf("delta(%s) = %d, want %d", ev, got, delta)
		}
	}
}

func TestPassiveDecayMood_DriftsTowardFifty(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		score    int
		hoursAgo int
		want     int
	}{
		{"high-score-decays-down", 80, 5, 70},   // -2/hr * 5
		{"high-clamps-at-50", 80, 100, 50},      // would overshoot
		{"low-score-decays-up", 10, 5, 20},      // +2/hr * 5
		{"low-clamps-at-50", 10, 100, 50},       // would overshoot
		{"already-50-no-change", 50, 100, 50},   // neutral stays neutral
		{"fresh-no-decay", 80, 0, 80},           // no time elapsed
		{"sub-hour-no-decay", 80, 0, 80},        // hours==0 → no drift
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			last := now.Add(-time.Duration(c.hoursAgo) * time.Hour)
			if got := passiveDecayMood(c.score, last, now); got != c.want {
				t.Errorf("decay(%d, %dh ago) = %d, want %d", c.score, c.hoursAgo, got, c.want)
			}
		})
	}
}

func TestTwinBeeLine_DeterministicAcrossCalls(t *testing.T) {
	a := twinBeeLine(ZoneGoblinWarrens, DMRoomEntry, "abc123", 2)
	b := twinBeeLine(ZoneGoblinWarrens, DMRoomEntry, "abc123", 2)
	if a != b {
		t.Errorf("non-deterministic:\n  a=%q\n  b=%q", a, b)
	}
	if !strings.HasPrefix(a, "🎭 **TwinBee:**") {
		t.Errorf("missing TwinBee prefix: %q", a)
	}
	// Different room index should typically yield a different line
	// (chance of accidental collision is 1/N; with the Goblin+Generic
	// pool that's tiny but theoretically possible — only assert prefix).
	c := twinBeeLine(ZoneGoblinWarrens, DMRoomEntry, "abc123", 3)
	if !strings.HasPrefix(c, "🎭 **TwinBee:**") {
		t.Errorf("missing prefix on alt salt: %q", c)
	}
}

func TestTwinBeeLine_BossEntryUsesNamedPool(t *testing.T) {
	// Grol's pool is a single hand-crafted line.
	got := twinBeeLine(ZoneGoblinWarrens, DMBossEntry, "any", 0)
	if !strings.Contains(got, "Grol") {
		t.Errorf("Goblin Warrens boss entry should mention Grol; got %q", got)
	}
	got = twinBeeLine(ZoneCryptValdris, DMBossEntry, "any", 0)
	if !strings.Contains(got, "Valdris") {
		t.Errorf("Crypt boss entry should mention Valdris; got %q", got)
	}
}

func TestTwinBeeLine_UnknownKindReturnsEmpty(t *testing.T) {
	if got := twinBeeLine(ZoneGoblinWarrens, DMNarrationType("not_a_real_kind"), "x", 0); got != "" {
		t.Errorf("expected empty for unknown kind, got %q", got)
	}
}

func TestApplyMoodEvent_PersistsClampedDelta(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-mood-evt:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.DMMood != 50 {
		t.Fatalf("starting mood = %d, want 50", run.DMMood)
	}

	// +10 → 60
	got, err := applyMoodEvent(run.RunID, MoodEventZoneComplete)
	if err != nil {
		t.Fatalf("apply zone_complete: %v", err)
	}
	if got != 60 {
		t.Errorf("after zone_complete: mood = %d, want 60", got)
	}

	// -20 → 40
	got, _ = applyMoodEvent(run.RunID, MoodEventDowntime)
	if got != 40 {
		t.Errorf("after downtime: mood = %d, want 40", got)
	}

	// Clamp test: huge negative stack → 0
	for i := 0; i < 20; i++ {
		_, _ = applyMoodEvent(run.RunID, MoodEventDowntime)
	}
	r, _ := getZoneRun(run.RunID)
	if r.DMMood != 0 {
		t.Errorf("after extreme negative stack: mood = %d, want 0 (clamped)", r.DMMood)
	}

	// Clamp upward
	for i := 0; i < 20; i++ {
		_, _ = applyMoodEvent(run.RunID, MoodEventCommunityMilestone)
	}
	r, _ = getZoneRun(run.RunID)
	if r.DMMood != 100 {
		t.Errorf("after extreme positive stack: mood = %d, want 100 (clamped)", r.DMMood)
	}
}

func TestApplyMoodEvent_UnknownEventErrors(t *testing.T) {
	if _, err := applyMoodEvent("fake-run", DMMoodEvent("ghosts")); err == nil {
		t.Error("expected error on unknown mood event")
	}
}

// ── Phase 11 D2b — Tier 1 zone flavor file routing ──────────────────────────

func TestComposeBossEntry_AppendsAbilityCalloutForTier1(t *testing.T) {
	got := composeBossEntry(ZoneGoblinWarrens, "run-d2b", 7)
	if !strings.Contains(got, "Grol") {
		t.Errorf("Warrens boss-entry should mention Grol: %q", got)
	}
	// Two TwinBee prefixes = entry + callout suffix.
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Warrens boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}

	got = composeBossEntry(ZoneCryptValdris, "run-d2b", 7)
	if !strings.Contains(got, "Valdris") {
		t.Errorf("Crypt boss-entry should mention Valdris: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Crypt boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}
}

// (Dragon's Lair gained signature callouts and an elite pool in D5b; see
// the Tier 4-5 routing tests below for its current behavior.)

func TestComposeBossEntry_DeterministicAcrossCalls(t *testing.T) {
	a := composeBossEntry(ZoneGoblinWarrens, "stable-run", 4)
	b := composeBossEntry(ZoneGoblinWarrens, "stable-run", 4)
	if a != b {
		t.Errorf("compose not deterministic:\n  a=%q\n  b=%q", a, b)
	}
}

func TestEliteRoomEntryLine_RoutesPerZone(t *testing.T) {
	w := eliteRoomEntryLine(ZoneGoblinWarrens, "run-elite", 3)
	if w == "" || !strings.HasPrefix(w, "🎭 **TwinBee:**") {
		t.Errorf("Warrens elite line missing/no prefix: %q", w)
	}
	c := eliteRoomEntryLine(ZoneCryptValdris, "run-elite", 3)
	if c == "" || !strings.HasPrefix(c, "🎭 **TwinBee:**") {
		t.Errorf("Crypt elite line missing/no prefix: %q", c)
	}
	if w == c {
		t.Errorf("Warrens and Crypt elite lines collided on same salt — pools shouldn't overlap")
	}
}

func TestZoneLorePool_PrependsZoneSpecific(t *testing.T) {
	w := zoneLorePool(ZoneGoblinWarrens)
	if len(w) <= len(flavor.LoreLines) {
		t.Errorf("Warrens lore pool should be larger than generic; got %d vs generic %d",
			len(w), len(flavor.LoreLines))
	}
}

// ── Phase 11 D3c — Tier 2 zone flavor file routing ──────────────────────────

func TestComposeBossEntry_AppendsAbilityCalloutForTier2(t *testing.T) {
	got := composeBossEntry(ZoneForestShadows, "run-d3c", 5)
	if !strings.Contains(got, "Hollow King") {
		t.Errorf("Forest boss-entry should mention Hollow King: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Forest boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}

	got = composeBossEntry(ZoneSunkenTemple, "run-d3c", 5)
	if !strings.Contains(got, "Aboleth") {
		t.Errorf("Sunken Temple boss-entry should mention Aboleth: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Sunken Temple boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}
}

func TestEliteRoomEntryLine_Tier2Routes(t *testing.T) {
	f := eliteRoomEntryLine(ZoneForestShadows, "run-elite-t2", 3)
	if f == "" || !strings.HasPrefix(f, "🎭 **TwinBee:**") {
		t.Errorf("Forest elite line missing/no prefix: %q", f)
	}
	s := eliteRoomEntryLine(ZoneSunkenTemple, "run-elite-t2", 3)
	if s == "" || !strings.HasPrefix(s, "🎭 **TwinBee:**") {
		t.Errorf("Sunken Temple elite line missing/no prefix: %q", s)
	}
	if f == s {
		t.Errorf("Forest and Sunken Temple elite lines collided on same salt")
	}
}

func TestZoneLorePool_Tier2PrependsZoneSpecific(t *testing.T) {
	f := zoneLorePool(ZoneForestShadows)
	if len(f) <= len(flavor.LoreLines) {
		t.Errorf("Forest lore pool should be larger than generic; got %d vs generic %d",
			len(f), len(flavor.LoreLines))
	}
	s := zoneLorePool(ZoneSunkenTemple)
	if len(s) <= len(flavor.LoreLines) {
		t.Errorf("Sunken Temple lore pool should be larger than generic; got %d vs generic %d",
			len(s), len(flavor.LoreLines))
	}
}

func TestRoomEntryPool_Tier2PrependsZoneSpecific(t *testing.T) {
	f := zoneRoomEntryPool(ZoneForestShadows)
	if len(f) <= len(flavor.RoomEntryGeneric) {
		t.Errorf("Forest room-entry pool should overlay zone-specific lines; got %d", len(f))
	}
	s := zoneRoomEntryPool(ZoneSunkenTemple)
	if len(s) <= len(flavor.RoomEntryGeneric) {
		t.Errorf("Sunken Temple room-entry pool should overlay zone-specific lines; got %d", len(s))
	}
}

// ── Phase 11 D4c — Tier 3 zone flavor file routing ──────────────────────────

func TestComposeBossEntry_AppendsAbilityCalloutForTier3(t *testing.T) {
	got := composeBossEntry(ZoneManorBlackspire, "run-d4c", 5)
	if !strings.Contains(got, "Aldric") {
		t.Errorf("Manor boss-entry should mention Aldric: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Manor boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}

	got = composeBossEntry(ZoneUnderforge, "run-d4c", 5)
	if !strings.Contains(got, "Thyrak") {
		t.Errorf("Underforge boss-entry should mention Thyrak: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Underforge boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}
}

func TestEliteRoomEntryLine_Tier3Routes(t *testing.T) {
	m := eliteRoomEntryLine(ZoneManorBlackspire, "run-elite-t3", 3)
	if m == "" || !strings.HasPrefix(m, "🎭 **TwinBee:**") {
		t.Errorf("Manor elite line missing/no prefix: %q", m)
	}
	u := eliteRoomEntryLine(ZoneUnderforge, "run-elite-t3", 3)
	if u == "" || !strings.HasPrefix(u, "🎭 **TwinBee:**") {
		t.Errorf("Underforge elite line missing/no prefix: %q", u)
	}
	if m == u {
		t.Errorf("Manor and Underforge elite lines collided on same salt")
	}
}

func TestZoneLorePool_Tier3PrependsZoneSpecific(t *testing.T) {
	m := zoneLorePool(ZoneManorBlackspire)
	if len(m) <= len(flavor.LoreLines) {
		t.Errorf("Manor lore pool should be larger than generic; got %d vs generic %d",
			len(m), len(flavor.LoreLines))
	}
	u := zoneLorePool(ZoneUnderforge)
	if len(u) <= len(flavor.LoreLines) {
		t.Errorf("Underforge lore pool should be larger than generic; got %d vs generic %d",
			len(u), len(flavor.LoreLines))
	}
}

func TestRoomEntryPool_Tier3PrependsZoneSpecific(t *testing.T) {
	m := zoneRoomEntryPool(ZoneManorBlackspire)
	if len(m) <= len(flavor.RoomEntryGeneric) {
		t.Errorf("Manor room-entry pool should overlay zone-specific lines; got %d", len(m))
	}
	u := zoneRoomEntryPool(ZoneUnderforge)
	if len(u) <= len(flavor.RoomEntryGeneric) {
		t.Errorf("Underforge room-entry pool should overlay zone-specific lines; got %d", len(u))
	}
}

func TestZoneCmd_AdvanceFinalRoomBumpsMood(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-mood-complete:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	// D1e wired real combat into !zone advance, so the cmd-level path
	// is no longer deterministic for an L1 fighter against the Goblin
	// Warrens roster. Drive the persistence layer directly to assert
	// the mood-on-completion contract.
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Phase G: branching paths through a diamond may complete the run in
	// fewer steps than TotalRooms. Drive markRoomCleared until it returns
	// the empty room type (boss kill / dead-end) or we hit a safety cap.
	for i := 0; i < run.TotalRooms+2; i++ {
		next, err := markRoomCleared(run.RunID)
		if err != nil {
			t.Fatalf("markRoomCleared %d: %v", i, err)
		}
		if next == "" {
			break
		}
	}
	if _, err := applyMoodEvent(run.RunID, MoodEventZoneComplete); err != nil {
		t.Fatal(err)
	}
	final, err := getZoneRun(run.RunID)
	if err != nil || final == nil {
		t.Fatal("could not fetch completed run")
	}
	if !final.BossDefeated {
		t.Error("boss should be defeated after final markRoomCleared")
	}
	if final.DMMood != 60 {
		t.Errorf("post-completion mood = %d, want 60 (50 + zone_complete +10)", final.DMMood)
	}
}

// ── Phase 11 D5b — Tier 4-5 zone flavor file routing ────────────────────────

func TestComposeBossEntry_AppendsAbilityCalloutForTier4(t *testing.T) {
	got := composeBossEntry(ZoneUnderdark, "run-d5b", 5)
	if !strings.Contains(got, "Ilvaras") {
		t.Errorf("Underdark boss-entry should mention Ilvaras: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Underdark boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}

	got = composeBossEntry(ZoneFeywildCrossing, "run-d5b", 5)
	if !strings.Contains(got, "Thornmother") {
		t.Errorf("Feywild boss-entry should mention Thornmother: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Feywild boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}
}

func TestComposeBossEntry_AppendsAbilityCalloutForTier5(t *testing.T) {
	got := composeBossEntry(ZoneDragonsLair, "run-d5b", 5)
	if !strings.Contains(got, "Infernax") {
		t.Errorf("Dragon's Lair boss-entry should mention Infernax: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Dragon's Lair boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}

	got = composeBossEntry(ZoneAbyssPortal, "run-d5b", 5)
	if !strings.Contains(got, "Belaxath") {
		t.Errorf("Abyss boss-entry should mention Belaxath: %q", got)
	}
	if n := strings.Count(got, "🎭 **TwinBee:**"); n != 2 {
		t.Errorf("Abyss boss-entry expected 2 TwinBee blocks (entry+callout), got %d:\n%s", n, got)
	}
}

func TestEliteRoomEntryLine_Tier4And5Routes(t *testing.T) {
	u := eliteRoomEntryLine(ZoneUnderdark, "run-elite-t45", 3)
	if u == "" || !strings.HasPrefix(u, "🎭 **TwinBee:**") {
		t.Errorf("Underdark elite line missing/no prefix: %q", u)
	}
	f := eliteRoomEntryLine(ZoneFeywildCrossing, "run-elite-t45", 3)
	if f == "" || !strings.HasPrefix(f, "🎭 **TwinBee:**") {
		t.Errorf("Feywild elite line missing/no prefix: %q", f)
	}
	d := eliteRoomEntryLine(ZoneDragonsLair, "run-elite-t45", 3)
	if d == "" || !strings.HasPrefix(d, "🎭 **TwinBee:**") {
		t.Errorf("Dragon's Lair elite line missing/no prefix: %q", d)
	}
	a := eliteRoomEntryLine(ZoneAbyssPortal, "run-elite-t45", 3)
	if a == "" || !strings.HasPrefix(a, "🎭 **TwinBee:**") {
		t.Errorf("Abyss elite line missing/no prefix: %q", a)
	}
	if u == f || u == d || u == a || f == d || f == a || d == a {
		t.Errorf("Tier 4-5 elite lines collided on same salt — pools shouldn't overlap")
	}
}

func TestZoneLorePool_Tier4And5PrependsZoneSpecific(t *testing.T) {
	for _, z := range []ZoneID{ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal} {
		if got := zoneLorePool(z); len(got) <= len(flavor.LoreLines) {
			t.Errorf("%s lore pool should be larger than generic; got %d vs generic %d",
				z, len(got), len(flavor.LoreLines))
		}
	}
}

func TestRoomEntryPool_Tier4And5PrependsZoneSpecific(t *testing.T) {
	// All four T4-5 zones should now overlay zone-specific room entries.
	for _, z := range []ZoneID{ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal} {
		if got := zoneRoomEntryPool(z); len(got) <= len(flavor.RoomEntryGeneric) {
			t.Errorf("%s room-entry pool should overlay zone-specific lines; got %d", z, len(got))
		}
	}
}

// ── Phase 11 D6 — boss phase-two narration ──────────────────────────────────

func TestBossPhaseTwoPool_RoutesEightZones(t *testing.T) {
	cases := map[ZoneID]int{
		ZoneCryptValdris:    len(flavor.ValdrisPhaseTwoLines),
		ZoneForestShadows:   len(flavor.HollowKingPhaseTwoLines),
		ZoneManorBlackspire: len(flavor.AldricPhaseTwoLines),
		ZoneUnderforge:      len(flavor.ThyrakPhaseTwoLines),
		ZoneUnderdark:       len(flavor.IlvarasPhaseTwoLines),
		ZoneFeywildCrossing: len(flavor.ThornmotherPhaseTwoLines),
		ZoneDragonsLair:     len(flavor.InfernaxPhaseTwoLines),
		ZoneAbyssPortal:     len(flavor.BelaxathPhaseTwoLines),
	}
	for z, want := range cases {
		got := bossPhaseTwoPool(z)
		if len(got) == 0 || len(got) != want {
			t.Errorf("%s phase-two pool len mismatch: got %d, want %d", z, len(got), want)
		}
	}
	// Bosses without a phase-two transition return nil.
	for _, z := range []ZoneID{ZoneGoblinWarrens, ZoneSunkenTemple} {
		if got := bossPhaseTwoPool(z); got != nil {
			t.Errorf("%s should not have a phase-two pool (no PhaseTwoAt)", z)
		}
	}
}

func TestBossPhaseTwoLine_PrependsTwinBee(t *testing.T) {
	line := bossPhaseTwoLine(ZoneCryptValdris, "run-d6", 4)
	if !strings.HasPrefix(line, "🎭 **TwinBee:**") {
		t.Errorf("phase-two line missing TwinBee prefix: %q", line)
	}
	if bossPhaseTwoLine(ZoneGoblinWarrens, "run-d6", 4) != "" {
		t.Errorf("Grol has no phase-two pool — expected empty string")
	}
}

func TestPhaseTwoCrossedInEvents(t *testing.T) {
	startHP := 100
	threshold := 0.5
	events := []CombatEvent{{EnemyHP: 80}, {EnemyHP: 60}, {EnemyHP: 40}}
	if !phaseTwoCrossedInEvents(events, startHP, threshold) {
		t.Errorf("expected crossing at EnemyHP=40 (cutoff=50)")
	}
	if phaseTwoCrossedInEvents(events[:2], startHP, threshold) {
		t.Errorf("did not cross 50%% by event 2 (EnemyHP=60); should be false")
	}
	if phaseTwoCrossedInEvents(events, startHP, 0) {
		t.Errorf("threshold 0 means no phase 2 — should be false")
	}
	if phaseTwoCrossedInEvents(events, 0, threshold) {
		t.Errorf("startHP 0 — should be false (degenerate)")
	}
}

// ── Phase 11 D8 — mood-banded TwinBee aside ─────────────────────────────────

func TestMoodAsideLine_AllBandsExceptNeutral(t *testing.T) {
	// Strict Neutral (40–59) stays silent — TwinBee has nothing
	// notable to say. Every other band produces an aside.
	for _, mood := range []int{40, 50, 59} {
		if got := moodAsideLine(mood, "run-aside", 3); got != "" {
			t.Errorf("mood=%d (neutral) should produce no aside; got %q", mood, got)
		}
	}
	for label, moods := range map[string][]int{
		"hostile":  {0, 19},
		"grumpy":   {20, 39},
		"friendly": {60, 79},
		"effusive": {80, 100},
	} {
		for _, mood := range moods {
			got := moodAsideLine(mood, "run-aside", 3)
			if got == "" || !strings.HasPrefix(got, "🎭 **TwinBee:**") {
				t.Errorf("%s mood=%d should produce aside; got %q", label, mood, got)
			}
		}
	}
}

func TestMoodAsideLine_DrawsFromCorrectPool(t *testing.T) {
	hostile := moodAsideLine(5, "run-aside", 1)
	effusive := moodAsideLine(95, "run-aside", 1)
	// Each line must come from its band's pool.
	if !lineInPool(hostile, flavor.MoodAsidesHostile) {
		t.Errorf("hostile aside %q not in MoodAsidesHostile", hostile)
	}
	if !lineInPool(effusive, flavor.MoodAsidesEffusive) {
		t.Errorf("effusive aside %q not in MoodAsidesEffusive", effusive)
	}
}

func TestMoodAsideLine_DeterministicPerRoom(t *testing.T) {
	a := moodAsideLine(10, "run-D8", 5)
	b := moodAsideLine(10, "run-D8", 5)
	if a != b {
		t.Errorf("same (mood, runID, roomIdx) should produce same aside: %q vs %q", a, b)
	}
}

// lineInPool returns true if rendered (with TwinBee prefix) corresponds to
// any line in pool.
func lineInPool(rendered string, pool []string) bool {
	const prefix = "🎭 **TwinBee:** "
	if !strings.HasPrefix(rendered, prefix) {
		return false
	}
	body := strings.TrimPrefix(rendered, prefix)
	for _, line := range pool {
		if line == body {
			return true
		}
	}
	return false
}
