package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E3a — Sunken Temple tidal event.

// setExpDay rewrites the current_day column for tests.
func setExpDay(t *testing.T, expID string, day int) {
	t.Helper()
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET current_day = ? WHERE expedition_id = ?`,
		day, expID); err != nil {
		t.Fatalf("set current_day: %v", err)
	}
}

func TestSunkenTemple_TidalWarningDay4(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-d4:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 3) // → briefing rolls to Day 4
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 4 {
		t.Fatalf("CurrentDay = %d, want 4", got.CurrentDay)
	}
	// Warning day: NO doubled burn — single 1.5 SU burn.
	if got.Supplies.Current != startSU-1.5 {
		t.Errorf("supplies = %v, want %v (single burn)", got.Supplies.Current, startSU-1.5)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	foundWarn := false
	for _, e := range entries {
		if e.Type == "temporal" && strings.Contains(e.Summary, "tidal warning") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Error("expected a 'temporal' tidal-warning log entry on Day 4")
	}
}

func TestSunkenTemple_TidalEventDay6_ForcesDoubleBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-d6:example")
	defer cleanupExpeditions(uid)

	// Tier-2 supplies: HarshMod 1.5 normally. Tidal must force 2×.
	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 5) // → rolls to Day 6
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 6 {
		t.Fatalf("CurrentDay = %d, want 6", got.CurrentDay)
	}
	// Forced 2× burn = 1.5 * 2 = 3.0 SU
	wantBurn := float32(3.0)
	if startSU-got.Supplies.Current != wantBurn {
		t.Errorf("burn = %v, want %v (forced 2× tidal)", startSU-got.Supplies.Current, wantBurn)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	foundEvent := false
	for _, e := range entries {
		if e.Type == "temporal" && strings.Contains(e.Summary, "tidal peak") {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Error("expected a 'temporal' tidal-peak log entry on Day 6")
	}
}

func TestSunkenTemple_BossDefeatedSilencesTidal(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-boss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET boss_defeated = 1, current_day = 5 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	// Single normal burn, no tidal multiplier.
	if got.Supplies.Current != startSU-1.5 {
		t.Errorf("supplies = %v, want %v (no tidal mult)", got.Supplies.Current, startSU-1.5)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	for _, e := range entries {
		if e.Type == "temporal" {
			t.Errorf("did not expect temporal log entry post-boss; got %+v", e)
		}
	}
}

func TestManor_NightlyReset_FiresEveryThreeDays(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-manor-reset:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneManorBlackspire, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}

	cases := []struct {
		fromDay   int
		wantReset bool
	}{
		{1, false}, // → Day 2
		{2, true},  // → Day 3
		{3, false}, // → Day 4
		{4, false}, // → Day 5
		{5, true},  // → Day 6
		{8, true},  // → Day 9
	}
	for _, c := range cases {
		setExpDay(t, exp.ID, c.fromDay)
		// Reset last_briefing_at so the next briefing actually fires.
		if _, err := db.Get().Exec(
			`UPDATE dnd_expedition SET last_briefing_at = NULL WHERE expedition_id = ?`, exp.ID); err != nil {
			t.Fatal(err)
		}
		fresh, _ := getExpedition(exp.ID)
		// Drain log entries from prior iterations so we count only today's.
		preCount := manorResetLogCount(t, exp.ID)
		if err := p.deliverBriefing(fresh, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		got := manorResetLogCount(t, exp.ID)
		gained := got - preCount
		if c.wantReset && gained == 0 {
			t.Errorf("from Day %d → expected a manor-reset entry; got none", c.fromDay)
		}
		if !c.wantReset && gained != 0 {
			t.Errorf("from Day %d → expected no reset; got %d new entries", c.fromDay, gained)
		}
	}
}

func manorResetLogCount(t *testing.T, expID string) int {
	t.Helper()
	rows, err := db.Get().Query(`
		SELECT COUNT(*) FROM dnd_expedition_log
		 WHERE expedition_id = ? AND entry_type = 'temporal'
		   AND summary LIKE 'manor reset%'`, expID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0
	}
	var n int
	_ = rows.Scan(&n)
	return n
}

func TestManor_BossDefeatedSilencesReset(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-manor-boss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneManorBlackspire, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET boss_defeated = 1, current_day = 2 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if got := manorResetLogCount(t, exp.ID); got != 0 {
		t.Errorf("boss-defeated manor should not reset; got %d entries", got)
	}
}

func TestManorReset_PredicateOnly(t *testing.T) {
	e := &Expedition{ZoneID: ZoneManorBlackspire}
	for _, d := range []int{0, 1, 2, 4, 5, 7, 8} {
		if ManorReset(e, d) {
			t.Errorf("ManorReset(day %d) = true; want false", d)
		}
	}
	for _, d := range []int{3, 6, 9, 12, 30} {
		if !ManorReset(e, d) {
			t.Errorf("ManorReset(day %d) = false; want true", d)
		}
	}
	// Wrong zone is always false.
	other := &Expedition{ZoneID: ZoneGoblinWarrens}
	if ManorReset(other, 3) {
		t.Error("ManorReset on non-Manor zone should be false")
	}
}

func TestSunkenTemple_NoTidalOnNonZone(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-other:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 20, Max: 20, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 5)
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.Supplies.Current != startSU-1 {
		t.Errorf("non-tidal zone burned %v, want 1", startSU-got.Supplies.Current)
	}
}
