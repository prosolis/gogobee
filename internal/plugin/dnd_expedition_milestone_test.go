package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestRecordMaxThreat_OnlyClimbs(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-maxthreat-climb:example")
	defer cleanupExpeditions(uid)
	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	e, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	e.ThreatLevel = 30
	if err := recordMaxThreat(e); err != nil {
		t.Fatalf("recordMaxThreat: %v", err)
	}
	// Persist may have failed; manually inspect in-memory state.
	if got, _ := e.RegionState[regionStateMaxThreatKey].(float64); got != 30 {
		t.Errorf("after first record, max = %v, want 30", got)
	}
	e.ThreatLevel = 20
	_ = recordMaxThreat(e)
	if got, _ := e.RegionState[regionStateMaxThreatKey].(float64); got != 30 {
		t.Errorf("after lower threat, max = %v, want 30 (no rewind)", got)
	}
	e.ThreatLevel = 55
	_ = recordMaxThreat(e)
	if got, _ := e.RegionState[regionStateMaxThreatKey].(float64); got != 55 {
		t.Errorf("after higher threat, max = %v, want 55", got)
	}
	if got := maxThreatSeen(e); got != 55 {
		t.Errorf("maxThreatSeen = %d, want 55", got)
	}
}

func TestCheckDailyMilestones_FirstNight(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-firstnight:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.CurrentDay = 2
	p := &AdventurePlugin{}

	lines := p.checkDailyMilestones(exp)
	if len(lines) != 1 {
		t.Fatalf("expected 1 milestone awarded, got %d", len(lines))
	}
	if !HasMilestone(exp, MilestoneKeyFirstNight) {
		t.Error("first_night not recorded in RegionState")
	}
	// Re-check is idempotent.
	again := p.checkDailyMilestones(exp)
	if len(again) != 0 {
		t.Errorf("second pass should award nothing, got %d", len(again))
	}
}

func TestCheckDailyMilestones_WeekOneAndTwoWeeks(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-weeks:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.CurrentDay = 8
	p := &AdventurePlugin{}
	lines := p.checkDailyMilestones(exp)
	// Day 8 awards both First Night and Week One in one pass.
	if len(lines) != 2 {
		t.Fatalf("day 8 should award 2 (first_night + week_one), got %d", len(lines))
	}
	if !HasMilestone(exp, MilestoneKeyWeekOne) {
		t.Error("week_one not recorded")
	}

	exp.CurrentDay = 15
	lines = p.checkDailyMilestones(exp)
	if len(lines) != 1 {
		t.Fatalf("day 15 should award 1 new (two_weeks), got %d", len(lines))
	}
	if !HasMilestone(exp, MilestoneKeyTwoWeeks) {
		t.Error("two_weeks not recorded")
	}
}

func TestAwardCompletionMilestones_PatientZero(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-patient:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusComplete
	exp.XPEarned = 1000
	exp.ThreatLevel = 40
	_ = recordMaxThreat(exp)

	p := &AdventurePlugin{}
	lines := p.AwardCompletionMilestones(exp, false)
	if len(lines) == 0 {
		t.Fatal("expected Patient Zero (max threat 40)")
	}
	if !HasMilestone(exp, MilestoneKeyPatientZero) {
		t.Error("patient_zero not recorded")
	}
}

func TestAwardCompletionMilestones_PatientZero_ThresholdMissed(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-patient-missed:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusComplete
	exp.ThreatLevel = 75
	_ = recordMaxThreat(exp)

	p := &AdventurePlugin{}
	p.AwardCompletionMilestones(exp, false)
	if HasMilestone(exp, MilestoneKeyPatientZero) {
		t.Error("patient_zero awarded despite max threat 75")
	}
}

func TestAwardCompletionMilestones_NotCalledOnNonComplete(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-noncomplete:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusAbandoned

	p := &AdventurePlugin{}
	if lines := p.AwardCompletionMilestones(exp, true); len(lines) != 0 {
		t.Errorf("abandoned should award nothing, got %d", len(lines))
	}
}

// ── N1/A4 — milestone grants ────────────────────────────────────────────────

func TestCheckDailyMilestones_TwoWeeksGrantsSupplyCache(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-twoweeks-cache:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 5, Max: 30, DailyBurn: 2, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneManorBlackspire, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.CurrentDay = 15

	p := &AdventurePlugin{}
	if lines := p.checkDailyMilestones(exp); len(lines) == 0 {
		t.Fatal("expected milestones on day 15")
	}
	if !HasMilestone(exp, MilestoneKeyTwoWeeks) {
		t.Fatal("two_weeks not recorded")
	}

	// 5 + 3 days × 2 SU/day = 11, well under the 30 cap.
	if got := exp.Supplies.Current; got != 11 {
		t.Errorf("supplies after restock = %v, want 11", got)
	}
	stored, err := getExpedition(exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Supplies.Current; got != 11 {
		t.Errorf("persisted supplies = %v, want 11", got)
	}

	inv, err := loadAdvInventory(uid)
	if err != nil {
		t.Fatal(err)
	}
	var consumables int
	for _, it := range inv {
		if it.Type == "consumable" {
			consumables++
		}
	}
	if consumables != twoWeeksCacheSize {
		t.Errorf("cache granted %d consumables, want %d", consumables, twoWeeksCacheSize)
	}
}

func TestGrantTwoWeeksCache_RestockNeverExceedsMax(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-twoweeks-cap:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 29, Max: 30, DailyBurn: 2, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneManorBlackspire, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	p.grantTwoWeeksCache(exp)
	if got := exp.Supplies.Current; got != 30 {
		t.Errorf("supplies = %v, want capped at 30", got)
	}
}

func TestAwardCompletionMilestones_LongGameGrantsLegendary(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-longgame:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneDragonsLair, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusComplete

	p := &AdventurePlugin{}
	p.AwardCompletionMilestones(exp, false)
	if !HasMilestone(exp, MilestoneKeyLongGame) {
		t.Fatal("long_game not recorded")
	}

	inv, err := loadAdvInventory(uid)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, it := range inv {
		if strings.HasPrefix(it.SkillSource, "magic_item:") {
			found = true
		}
	}
	if !found {
		t.Errorf("no magic item granted; inventory = %+v", inv)
	}
}

func TestAwardCompletionMilestones_SurvivalistSetsTitle(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-survivalist:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneManorBlackspire, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusComplete

	p := &AdventurePlugin{}
	p.AwardCompletionMilestones(exp, false)
	if !HasMilestone(exp, MilestoneKeySurvivalist) {
		t.Fatal("survivalist not recorded")
	}
	char, err := loadAdvCharacter(uid)
	if err != nil {
		t.Fatal(err)
	}
	if char.Title != survivalistTitle {
		t.Errorf("title = %q, want %q", char.Title, survivalistTitle)
	}
}

func TestAwardCompletionMilestones_SurvivalistSkippedOnForcedExtract(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-milestone-survivalist-forced:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneManorBlackspire, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	exp.Status = ExpeditionStatusComplete

	p := &AdventurePlugin{}
	p.AwardCompletionMilestones(exp, true)
	if HasMilestone(exp, MilestoneKeySurvivalist) {
		t.Error("survivalist awarded despite a forced extraction")
	}
	char, _ := loadAdvCharacter(uid)
	if char != nil && char.Title == survivalistTitle {
		t.Error("title set despite a forced extraction")
	}
}

func TestConsumableCache_Tier1FallsBackToPoultice(t *testing.T) {
	items := consumableCache(1, 3)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	for _, it := range items {
		if it.Name != "Berry Poultice" {
			t.Errorf("tier-1 cache yielded %q, want Berry Poultice", it.Name)
		}
	}
}
