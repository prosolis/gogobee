package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func cleanupExpeditions(uid id.UserID) {
	_, _ = db.Get().Exec(`DELETE FROM dnd_expedition_log
		WHERE expedition_id IN (SELECT expedition_id FROM dnd_expedition WHERE user_id = ?)`,
		string(uid))
	_, _ = db.Get().Exec(`DELETE FROM dnd_expedition WHERE user_id = ?`, string(uid))
}

func TestStartExpedition_RoundTrip(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-roundtrip:example.org")
	defer cleanupExpeditions(uid)

	supplies := ExpeditionSupplies{
		Current:       10,
		Max:           10,
		DailyBurn:     1,
		HarshMod:      1,
		PacksStandard: 1,
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatalf("startExpedition: %v", err)
	}
	if exp.Status != ExpeditionStatusActive {
		t.Errorf("status = %q", exp.Status)
	}
	if exp.CurrentDay != 1 {
		t.Errorf("current_day = %d", exp.CurrentDay)
	}
	if !exp.IsActive() {
		t.Error("expected IsActive")
	}

	got, err := getActiveExpedition(uid)
	if err != nil || got == nil {
		t.Fatalf("getActiveExpedition: %v / %v", got, err)
	}
	if got.ID != exp.ID {
		t.Errorf("id mismatch")
	}
	if got.ZoneID != ZoneGoblinWarrens {
		t.Errorf("zone = %s", got.ZoneID)
	}
	if got.Supplies.Current != 10 || got.Supplies.DailyBurn != 1 {
		t.Errorf("supplies round-trip wrong: %+v", got.Supplies)
	}
	if got.DMMood != 50 {
		t.Errorf("gm_mood = %d", got.DMMood)
	}
	if got.Camp != nil {
		t.Errorf("expected no camp at start")
	}
}

func TestStartExpedition_RejectsConcurrent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-concur:example.org")
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := startExpedition(uid, ZoneCryptValdris, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1}); err != ErrExpeditionAlreadyActive {
		t.Errorf("err = %v, want ErrExpeditionAlreadyActive", err)
	}
}

func TestExpedition_AbandonAndRestart(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-abandon:example.org")
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1}); err != nil {
		t.Fatal(err)
	}
	if err := abandonExpedition(uid); err != nil {
		t.Fatal(err)
	}
	got, err := getActiveExpedition(uid)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected no active expedition after abandon, got %+v", got)
	}
	// Should be allowed to start again now.
	if _, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1}); err != nil {
		t.Errorf("expected restart allowed, got %v", err)
	}
}

func TestExpedition_UpdateSuppliesAndCamp(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-update:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	exp.Supplies.Current = 7.5
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		t.Fatal(err)
	}
	camp := &CampState{Active: true, Type: "standard", RoomIndex: 2}
	if err := updateCamp(exp.ID, camp); err != nil {
		t.Fatal(err)
	}
	got, err := getActiveExpedition(uid)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", got, err)
	}
	if got.Supplies.Current != 7.5 {
		t.Errorf("supplies.current = %v", got.Supplies.Current)
	}
	if got.Camp == nil || got.Camp.Type != "standard" || got.Camp.RoomIndex != 2 {
		t.Errorf("camp = %+v", got.Camp)
	}
	// Break camp.
	if err := updateCamp(exp.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = getActiveExpedition(uid)
	if got.Camp != nil {
		t.Errorf("expected camp nil after break, got %+v", got.Camp)
	}
}

func TestExpedition_ThreatDeltaAndSiege(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-threat:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 30, "test bump"); err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 80, "siege test"); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != 100 {
		t.Errorf("threat = %d, want clamped to 100", got.ThreatLevel)
	}
	if !got.SiegeMode {
		t.Error("expected siege mode at 100")
	}
	if len(got.ThreatEvents) != 2 {
		t.Errorf("threat_events len = %d, want 2", len(got.ThreatEvents))
	}
	// Negative delta clamps to 0.
	if err := applyThreatDelta(exp.ID, -200, "reset"); err != nil {
		t.Fatal(err)
	}
	got, _ = getExpedition(exp.ID)
	if got.ThreatLevel != 0 {
		t.Errorf("threat after negative delta = %d", got.ThreatLevel)
	}
	// Siege mode is sticky once tripped.
	if !got.SiegeMode {
		t.Error("expected siege mode to remain set")
	}
}

func TestExpedition_LogAppendAndRecent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-log:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 7; i++ {
		if err := appendExpeditionLog(exp.ID, i, "action", "advance", "TwinBee notes."); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := recentExpeditionLog(exp.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].Day != 7 {
		t.Errorf("newest day = %d, want 7", entries[0].Day)
	}
}

func TestExpedition_AdvanceDay(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-day:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := advanceExpeditionDay(exp.ID); err != nil {
		t.Fatal(err)
	}
	if err := advanceExpeditionDay(exp.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 3 {
		t.Errorf("current_day = %d, want 3", got.CurrentDay)
	}
}
