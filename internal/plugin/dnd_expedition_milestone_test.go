package plugin

import (
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
