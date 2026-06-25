package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E1d — day cycle tests. We can't easily fast-forward UTC time,
// so the tests drive deliverBriefing / deliverRecap directly with a
// synthetic "now" and verify state transitions. Ticker scheduling is a
// thin wrapper over those.

// rewindToLegacyAnchor backdates an expedition's start_date to before
// eventAnchoredCutoff so deliverBriefing exercises the legacy UTC-anchored
// mutator path. Tests of the D2-b event-anchored path should NOT call this.
func rewindToLegacyAnchor(t *testing.T, exp *Expedition) {
	t.Helper()
	before := eventAnchoredCutoff.Add(-24 * time.Hour)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = ?`,
		before, exp.ID); err != nil {
		t.Fatal(err)
	}
	exp.StartDate = before
}

func TestPickMorningBriefing_DayBands(t *testing.T) {
	cases := []struct {
		day  int
		want []string // any-of pool
	}{
		{1, []string{"First morning", "Day one", "day two"}},
		{3, []string{"Day three", "Three days"}},
		{7, []string{"One week", "Seven days", "A week underground"}},
		{14, []string{"Two weeks", "Fourteen days", "Day fifteen"}},
		{21, []string{"Three weeks", "Day twenty-one"}},
	}
	for _, c := range cases {
		got := pickMorningBriefing(c.day)
		matched := false
		for _, prefix := range c.want {
			if strings.Contains(got, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("Day %d picked %q — none of %v matched", c.day, got, c.want)
		}
	}
	// Generic fallback for off-band days.
	if got := pickMorningBriefing(2); got == "" {
		t.Error("generic fallback empty")
	}
	if got := pickMorningBriefing(99); got == "" {
		t.Error("generic fallback empty")
	}
}

func TestDeliverBriefing_AdvancesDayAndBurnsSupplies(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-cycle-brief:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	rewindToLegacyAnchor(t, exp)
	p := &AdventurePlugin{}
	now := time.Now().UTC()
	if err := p.deliverBriefing(exp, now); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 2 {
		t.Errorf("current_day = %d, want 2", got.CurrentDay)
	}
	// Phase 5-B: applyDailyBurn scales by phase5BDailyBurnRatePct=50, so
	// raw burn 1 × 0.5 = 0.5 SU. Current 10 - 0.5 = 9.5.
	if got.Supplies.Current != 9.5 {
		t.Errorf("supplies = %v, want 9.5", got.Supplies.Current)
	}
	if got.LastBriefingAt == nil {
		t.Error("LastBriefingAt should be set")
	}
	// Briefing log entry should exist.
	entries, _ := recentExpeditionLog(exp.ID, 5)
	foundBriefing := false
	for _, e := range entries {
		if e.Type == "briefing" {
			foundBriefing = true
		}
	}
	if !foundBriefing {
		t.Error("briefing log entry missing")
	}
}

// useEventAnchored fences the eventAnchoredCutoff to before the given
// expedition's start_date so the D2-b path is taken. Test-scoped via t.Cleanup.
func useEventAnchored(t *testing.T, exp *Expedition) {
	t.Helper()
	saved := eventAnchoredCutoff
	eventAnchoredCutoff = exp.StartDate.Add(-time.Minute)
	t.Cleanup(func() { eventAnchoredCutoff = saved })
}

func TestDeliverBriefing_EventAnchoredSkipsMutators(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-evt-skip:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	// last_briefing_at is NULL and start_date is "now-ish", so the safety
	// net should NOT fire — sub-28h since start.
	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 1 {
		t.Errorf("day = %d, want 1 (event-anchored briefing should not advance day)", got.CurrentDay)
	}
	if got.Supplies.Current != 10 {
		t.Errorf("supplies = %v, want 10 (event-anchored briefing should not burn)", got.Supplies.Current)
	}
}

func TestDeliverBriefing_EventAnchoredSafetyNetForces(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-evt-safety:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	// Backdate start_date so > nightSafetyNet has elapsed with no rollover.
	before := time.Now().UTC().Add(-(nightSafetyNet + time.Hour))
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = ?`,
		before, exp.ID); err != nil {
		t.Fatal(err)
	}
	exp.StartDate = before

	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 2 {
		t.Errorf("day = %d, want 2 (safety net should force rollover)", got.CurrentDay)
	}
	if got.Supplies.Current >= 10 {
		t.Errorf("supplies = %v, want < 10 (safety net should burn)", got.Supplies.Current)
	}
}

// D4-b: the 06:00 ticker skips event-anchored expeditions whose player
// hasn't moved since before today's threshold; the briefing lands lazily
// on the next inbound message via maybeDeliverDeferredBriefing.
func TestFireBriefings_EventAnchoredIdleSkip(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-d4b-idle:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	// Synthetic now pinned past today's 06:00 UTC so the test outcome is
	// independent of wall-clock time of day.
	wall := time.Now().UTC()
	// Synthetic now is today's 06:30 UTC — just past the ticker threshold,
	// but well inside the 28h nightSafetyNet window so the safety-net
	// force path doesn't kick in.
	now := time.Date(wall.Year(), wall.Month(), wall.Day(), 6, 30, 0, 0, time.UTC)
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionBriefingHour, 0, 0, 0, time.UTC)
	idleActivity := threshold.Add(-2 * time.Hour)
	priorBriefing := now.Add(-24 * time.Hour)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET current_day = 3 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET last_activity = ?, last_briefing_at = ? WHERE expedition_id = ?`,
		idleActivity, priorBriefing, exp.ID); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	p.fireExpeditionBriefings(now)

	got, _ := getExpedition(exp.ID)
	if got.LastBriefingAt == nil || !got.LastBriefingAt.Equal(priorBriefing) {
		t.Fatalf("idle ticker should not have stamped last_briefing_at: got %v want %v",
			got.LastBriefingAt, priorBriefing)
	}

	// Simulate inbound message: lazy delivery should fire now.
	p.maybeDeliverDeferredBriefing(uid, now)
	got, _ = getExpedition(exp.ID)
	if got.LastBriefingAt == nil || got.LastBriefingAt.Equal(priorBriefing) {
		t.Fatalf("deferred delivery should have stamped a fresh last_briefing_at: got %v",
			got.LastBriefingAt)
	}
}

// D4-b: an active player (last_activity >= today's threshold) still gets
// the morning DM on the regular ticker — no idle skip.
func TestFireBriefings_EventAnchoredActivePlayerDelivers(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-d4b-active:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	wall := time.Now().UTC()
	now := time.Date(wall.Year(), wall.Month(), wall.Day(), 6, 30, 0, 0, time.UTC)
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionBriefingHour, 0, 0, 0, time.UTC)
	activeActivity := threshold.Add(15 * time.Minute)
	priorBriefing := now.Add(-24 * time.Hour)
	// Pin start_date before today's threshold. Left at the default (real
	// time.Now()), a suite run after 06:00 UTC lands start_date past the
	// threshold and loadExpeditionsNeedingBriefing (start_date < threshold)
	// filters the row out — a wall-clock-of-day flake. useEventAnchored runs
	// after, so its cutoff tracks the backdated start and the run stays
	// event-anchored.
	startAt := now.Add(-24 * time.Hour)
	exp.StartDate = startAt
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ?, last_activity = ?, last_briefing_at = ? WHERE expedition_id = ?`,
		startAt, activeActivity, priorBriefing, exp.ID); err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	p := &AdventurePlugin{}
	p.fireExpeditionBriefings(now)

	got, _ := getExpedition(exp.ID)
	if got.LastBriefingAt == nil || got.LastBriefingAt.Equal(priorBriefing) {
		t.Fatalf("active player should have received briefing: last_briefing_at=%v",
			got.LastBriefingAt)
	}
}

func TestDeliverBriefing_HarshConditionsBurnFaster(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-cycle-harsh:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	rewindToLegacyAnchor(t, exp)
	// Force harsh conditions via threat clock above 60.
	if err := applyThreatDelta(exp.ID, 70, "test"); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	// Phase 5-B: 1 base × 2 harsh × phase5B 50% = 1 SU; current 10 - 1 = 9.
	if got.Supplies.Current != 9 {
		t.Errorf("harsh burn supplies = %v, want 9", got.Supplies.Current)
	}
}

func TestLoadExpeditionsNeedingBriefing_FiltersByThreshold(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-cycle-load:example")
	defer cleanupExpeditions(uid)

	// Start an expedition; rewind start_date so threshold falls past it.
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().UTC().Add(-25 * time.Hour)
	if _, err := db.Get().Exec(`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = ?`,
		yesterday, exp.ID); err != nil {
		t.Fatal(err)
	}

	threshold := time.Now().UTC()
	got, err := loadExpeditionsNeedingBriefing(threshold)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range got {
		if e.ID == exp.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected expedition in needs-briefing list")
	}

	// Stamp briefing → should drop out.
	now := time.Now().UTC()
	if _, err := db.Get().Exec(`UPDATE dnd_expedition SET last_briefing_at = ? WHERE expedition_id = ?`,
		now, exp.ID); err != nil {
		t.Fatal(err)
	}
	got, err = loadExpeditionsNeedingBriefing(threshold.Add(-1 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got {
		if e.ID == exp.ID {
			t.Error("expedition should be filtered out after briefing stamped")
		}
	}
}

func TestDeliverRecap_StampsAndLogs(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-cycle-recap:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.deliverRecap(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.LastRecapAt == nil {
		t.Error("LastRecapAt should be set")
	}
	entries, _ := recentExpeditionLog(exp.ID, 5)
	foundRecap := false
	for _, e := range entries {
		if e.Type == "recap" {
			foundRecap = true
		}
	}
	if !foundRecap {
		t.Error("recap log entry missing")
	}
}

func TestPickEveningRecap_BossOverridesAll(t *testing.T) {
	exp := &Expedition{}
	today := []ExpeditionEntry{
		{Type: "combat"},
		{Type: "boss", Summary: "boss defeated"},
	}
	got := pickEveningRecap(exp, today)
	if got == "" {
		t.Fatal("expected boss-killed flavor")
	}
	// Boss-killed pool entries reference the kill in varied ways — "boss",
	// "thing in the chamber", "vacancy", etc. Match any of the recurring
	// motifs rather than the literal word.
	low := strings.ToLower(got)
	matched := false
	for _, marker := range []string{"boss", "thing in the chamber", "vacancy"} {
		if strings.Contains(low, marker) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected boss-killed pool, got %q", got)
	}
}

func TestPickEveningRecap_NothingHappenedFallback(t *testing.T) {
	exp := &Expedition{}
	today := []ExpeditionEntry{} // no entries today
	got := pickEveningRecap(exp, today)
	if got == "" {
		t.Error("expected fallback flavor")
	}
}
