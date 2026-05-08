package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// E5a: voluntary extraction burns one day's supplies, advances the day,
// flips status to 'extracting', and stamps completed_at.
func TestVoluntaryExtract_FlipsToExtracting(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-extract-voluntary:example")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	startDay := exp.CurrentDay

	updated, err := voluntaryExtractExpedition(uid)
	if err != nil {
		t.Fatalf("voluntaryExtractExpedition: %v", err)
	}
	if updated.Status != ExpeditionStatusExtracting {
		t.Errorf("status = %q, want extracting", updated.Status)
	}
	if updated.CurrentDay != startDay+1 {
		t.Errorf("current_day = %d, want %d", updated.CurrentDay, startDay+1)
	}
	got, _ := getExpedition(exp.ID)
	if got.Supplies.Current != 9 {
		t.Errorf("supplies after extract = %.1f, want 9", got.Supplies.Current)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}

	// Active query should now return nothing — extracting is post-extract.
	if active, _ := getActiveExpedition(uid); active != nil {
		t.Errorf("getActiveExpedition still returns row: %s", active.Status)
	}
}

func TestVoluntaryExtract_NoActive(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-extract-noactive:example")
	defer cleanupExpeditions(uid)

	if _, err := voluntaryExtractExpedition(uid); err != ErrNoActiveExpedition {
		t.Errorf("err = %v, want ErrNoActiveExpedition", err)
	}
}

// E5b: forced extraction flips to 'abandoned' and reports the 20% coin tax.
func TestForcedExtract_AbandonedAndTax(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-extract-forced:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET coins_earned = 100 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}

	got, tax, err := forcedExtractExpedition(exp.ID, "supplies depleted")
	if err != nil {
		t.Fatalf("forcedExtractExpedition: %v", err)
	}
	if tax != 20 {
		t.Errorf("tax = %d, want 20 (20%% of 100)", tax)
	}
	if got.Status != ExpeditionStatusAbandoned {
		t.Errorf("status = %q, want abandoned", got.Status)
	}
	persisted, _ := getExpedition(exp.ID)
	if persisted.Status != ExpeditionStatusAbandoned {
		t.Errorf("persisted status = %q", persisted.Status)
	}
	if persisted.CompletedAt == nil {
		t.Error("expected completed_at after forced extract")
	}
}

// E5c: resume restores 'active' status, fresh supplies, preserves threat.
func TestResume_FreshSuppliesPreservesThreat(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-resume-ok:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 35, "test"); err != nil {
		t.Fatal(err)
	}
	if err := updateTemporalStack(exp.ID, 12); err != nil {
		t.Fatal(err)
	}
	if _, err := voluntaryExtractExpedition(uid); err != nil {
		t.Fatal(err)
	}

	resumable, err := getResumableExpedition(uid)
	if err != nil || resumable == nil {
		t.Fatalf("getResumableExpedition: %v / %v", resumable, err)
	}
	if resumable.ID != exp.ID {
		t.Error("wrong row")
	}

	freshSupplies := ExpeditionSupplies{Current: 20, Max: 20, DailyBurn: 1, HarshMod: 1}
	if err := resumeExpedition(resumable.ID, freshSupplies); err != nil {
		t.Fatalf("resumeExpedition: %v", err)
	}
	got, _ := getExpedition(exp.ID)
	if got.Status != ExpeditionStatusActive {
		t.Errorf("status = %q, want active", got.Status)
	}
	if got.Supplies.Current != 20 {
		t.Errorf("supplies.Current = %.1f, want 20", got.Supplies.Current)
	}
	if got.ThreatLevel != 35 {
		t.Errorf("threat = %d, want 35 (preserved)", got.ThreatLevel)
	}
	if got.TemporalStack != 12 {
		t.Errorf("temporal = %d, want 12 (preserved)", got.TemporalStack)
	}
	if got.CompletedAt != nil {
		t.Error("completed_at should be cleared on resume")
	}
}

// E5c: resume window expires after 7 real days; getResumableExpedition still
// returns the row (caller decides), but the command path should reject.
func TestResume_WindowExpired(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-resume-expired:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := voluntaryExtractExpedition(uid); err != nil {
		t.Fatal(err)
	}
	// Backdate completed_at well past the 7-day window.
	stale := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET completed_at = ? WHERE expedition_id = ?`,
		stale, exp.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := getResumableExpedition(uid)
	if got == nil || got.CompletedAt == nil {
		t.Fatal("expected resumable row with completed_at set")
	}
	if time.Since(*got.CompletedAt) <= extractResumeWindow {
		t.Errorf("expected window to be expired (since=%v, window=%v)",
			time.Since(*got.CompletedAt), extractResumeWindow)
	}
}
