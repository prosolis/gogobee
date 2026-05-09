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
	p := &AdventurePlugin{}
	now := time.Now().UTC()
	if err := p.deliverBriefing(exp, now); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 2 {
		t.Errorf("current_day = %d, want 2", got.CurrentDay)
	}
	if got.Supplies.Current != 9 {
		t.Errorf("supplies = %v, want 9", got.Supplies.Current)
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

func TestDeliverBriefing_HarshConditionsBurnFaster(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-cycle-harsh:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
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
	if got.Supplies.Current != 8 { // 1 base × 2 harsh = 2 SU
		t.Errorf("harsh burn supplies = %v, want 8", got.Supplies.Current)
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
