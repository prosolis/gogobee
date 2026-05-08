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
