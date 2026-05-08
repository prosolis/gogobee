package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E2a — Threat Clock state-machine tests.

func TestThreatBandFor(t *testing.T) {
	cases := []struct {
		level int
		siege bool
		want  ThreatBand
	}{
		{0, false, ThreatBandQuiet},
		{20, false, ThreatBandQuiet},
		{21, false, ThreatBandStirring},
		{40, false, ThreatBandStirring},
		{41, false, ThreatBandAlert},
		{60, false, ThreatBandAlert},
		{61, false, ThreatBandHostile},
		{80, false, ThreatBandHostile},
		{81, false, ThreatBandSiege},
		{100, false, ThreatBandSiege},
		{50, true, ThreatBandSiege},
	}
	for _, c := range cases {
		got := threatBandFor(c.level, c.siege)
		if got != c.want {
			t.Errorf("threatBandFor(%d, %v) = %v, want %v", c.level, c.siege, got, c.want)
		}
	}
}

func TestThreatBandInfo_SiegeFlags(t *testing.T) {
	info := threatBandInfo(ThreatBandSiege)
	if !info.NoShortRest {
		t.Error("siege should disable short rests")
	}
	if info.SupplyMult != 2 {
		t.Errorf("siege supply mult = %v, want 2", info.SupplyMult)
	}
	if !info.InitAdv || !info.TrapsArm {
		t.Error("siege should grant init advantage and re-arm traps")
	}
}

func TestDailyThreatDrift_MoodMod(t *testing.T) {
	cases := []struct {
		mood int
		want int
	}{
		{50, 3},  // neutral
		{60, 3},  // friendly (still neutral for threat)
		{80, 0},  // effusive → elated → -3
		{19, 8},  // hostile → wrathful → +5
		{0, 8},   // hostile lower bound
	}
	for _, c := range cases {
		got, _ := dailyThreatDrift(c.mood)
		if got != c.want {
			t.Errorf("mood=%d drift=%d, want %d", c.mood, got, c.want)
		}
	}
}

func TestApplyDailyThreatDrift_PersistsAndCrossesThreshold(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-threat-drift:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Pre-load to 19 so a +3 drift crosses Stirring boundary.
	if err := applyThreatDelta(exp.ID, 19, "test seed"); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)

	delta, _, err := applyDailyThreatDrift(exp)
	if err != nil {
		t.Fatal(err)
	}
	if delta != 3 {
		t.Errorf("delta = %d, want 3", delta)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != 22 {
		t.Errorf("threat = %d, want 22", got.ThreatLevel)
	}
	// Threshold transition log entry should exist with TwinBee narration.
	entries, _ := recentExpeditionLog(exp.ID, 10)
	foundTransition := false
	for _, e := range entries {
		if e.Type == "threat" && strings.Contains(strings.ToLower(e.Summary), "stirring") {
			if e.Flavor == "" {
				t.Error("threat transition missing flavor line")
			}
			foundTransition = true
		}
	}
	if !foundTransition {
		t.Error("expected threat transition log entry into Stirring")
	}
}

func TestApplyDailyThreatDrift_NoOpAfterBoss(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-threat-postboss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(`UPDATE dnd_expedition SET boss_defeated = 1 WHERE expedition_id = ?`, exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	delta, _, err := applyDailyThreatDrift(exp)
	if err != nil {
		t.Fatal(err)
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0 after boss kill", delta)
	}
}

func TestDeliverBriefing_AppliesThreatDrift(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-brief-drift:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != 3 {
		t.Errorf("post-briefing threat = %d, want 3 (mood-neutral drift)", got.ThreatLevel)
	}
}

func TestApplyBossDefeatThreat_DropsLevel(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-boss-threat:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 50, "seed"); err != nil {
		t.Fatal(err)
	}
	if err := applyBossDefeatThreat(exp.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != 30 {
		t.Errorf("post-boss threat = %d, want 30", got.ThreatLevel)
	}
}
