package plugin

import (
	"strings"
	"testing"
	"time"

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
	want := map[GMMoodEvent]int{
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
	a := twinBeeLine(ZoneGoblinWarrens, GMRoomEntry, "abc123", 2)
	b := twinBeeLine(ZoneGoblinWarrens, GMRoomEntry, "abc123", 2)
	if a != b {
		t.Errorf("non-deterministic:\n  a=%q\n  b=%q", a, b)
	}
	if !strings.HasPrefix(a, "🎭 **TwinBee:**") {
		t.Errorf("missing TwinBee prefix: %q", a)
	}
	// Different room index should typically yield a different line
	// (chance of accidental collision is 1/N; with the Goblin+Generic
	// pool that's tiny but theoretically possible — only assert prefix).
	c := twinBeeLine(ZoneGoblinWarrens, GMRoomEntry, "abc123", 3)
	if !strings.HasPrefix(c, "🎭 **TwinBee:**") {
		t.Errorf("missing prefix on alt salt: %q", c)
	}
}

func TestTwinBeeLine_BossEntryUsesNamedPool(t *testing.T) {
	// Grol's pool is a single hand-crafted line.
	got := twinBeeLine(ZoneGoblinWarrens, GMBossEntry, "any", 0)
	if !strings.Contains(got, "Grol") {
		t.Errorf("Goblin Warrens boss entry should mention Grol; got %q", got)
	}
	got = twinBeeLine(ZoneCryptValdris, GMBossEntry, "any", 0)
	if !strings.Contains(got, "Valdris") {
		t.Errorf("Crypt boss entry should mention Valdris; got %q", got)
	}
}

func TestTwinBeeLine_UnknownKindReturnsEmpty(t *testing.T) {
	if got := twinBeeLine(ZoneGoblinWarrens, GMNarrationType("not_a_real_kind"), "x", 0); got != "" {
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
	if run.GMMood != 50 {
		t.Fatalf("starting mood = %d, want 50", run.GMMood)
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
	if r.GMMood != 0 {
		t.Errorf("after extreme negative stack: mood = %d, want 0 (clamped)", r.GMMood)
	}

	// Clamp upward
	for i := 0; i < 20; i++ {
		_, _ = applyMoodEvent(run.RunID, MoodEventCommunityMilestone)
	}
	r, _ = getZoneRun(run.RunID)
	if r.GMMood != 100 {
		t.Errorf("after extreme positive stack: mood = %d, want 100 (clamped)", r.GMMood)
	}
}

func TestApplyMoodEvent_UnknownEventErrors(t *testing.T) {
	if _, err := applyMoodEvent("fake-run", GMMoodEvent("ghosts")); err == nil {
		t.Error("expected error on unknown mood event")
	}
}

func TestZoneCmd_AdvanceFinalRoomBumpsMood(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-mood-complete:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)

	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	run, _ := getActiveZoneRun(uid)
	// Advance through every room. zone_complete fires on the boss-room
	// advance and bumps mood by +10 (50 → 60).
	for i := 0; i < run.TotalRooms; i++ {
		if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "advance"); err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
	}
	final, err := getZoneRun(run.RunID)
	if err != nil || final == nil {
		t.Fatal("could not fetch completed run")
	}
	if !final.BossDefeated {
		t.Error("boss should be defeated after final advance")
	}
	if final.GMMood != 60 {
		t.Errorf("post-completion mood = %d, want 60 (50 + zone_complete +10)", final.GMMood)
	}
}
