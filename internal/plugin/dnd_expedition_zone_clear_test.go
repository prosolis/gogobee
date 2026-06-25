package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Single-region zone: clearing the run completes the wrapping expedition
// and flips BossDefeated.
func TestFinalizeExpeditionOnZoneClear_SingleRegionCompletes(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zclear-single:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "run-1",
		ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if exp.RunID != "run-1" {
		t.Fatalf("setup: RunID = %q", exp.RunID)
	}

	p := &AdventurePlugin{}
	p.finalizeExpeditionOnZoneClear(uid, "run-1")

	if act, _ := getActiveExpedition(uid); act != nil {
		t.Fatalf("expedition still active after zone clear: status=%s", act.Status)
	}
	loaded, _ := getExpedition(exp.ID)
	if loaded.Status != ExpeditionStatusComplete {
		t.Errorf("status = %q, want %q", loaded.Status, ExpeditionStatusComplete)
	}
	if !loaded.BossDefeated {
		t.Error("BossDefeated not set after single-region zone clear")
	}
}

// A run that isn't the expedition's current run must not complete it.
func TestFinalizeExpeditionOnZoneClear_RunMismatchNoop(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zclear-mismatch:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "run-1",
		ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	p.finalizeExpeditionOnZoneClear(uid, "some-other-run")

	loaded, _ := getExpedition(exp.ID)
	if loaded.Status != ExpeditionStatusActive {
		t.Errorf("status = %q, want active (mismatched run should be no-op)", loaded.Status)
	}
}

// Multi-region zone: clearing a non-boss region marks it cleared but leaves
// the expedition active so inter-region travel can continue.
func TestFinalizeExpeditionOnZoneClear_MidZoneRegionStaysActive(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zclear-midzone:example.org")
	defer cleanupExpeditions(uid)

	// CurrentRegion defaults to the first region (underdark_surface_tunnels,
	// not the zone boss).
	exp, err := startExpedition(uid, ZoneUnderdark, "run-1",
		ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	p.finalizeExpeditionOnZoneClear(uid, "run-1")

	loaded, _ := getExpedition(exp.ID)
	if loaded.Status != ExpeditionStatusActive {
		t.Errorf("status = %q, want active after non-boss region clear", loaded.Status)
	}
	if !IsRegionCleared(loaded, "underdark_surface_tunnels") {
		t.Error("first region not marked cleared")
	}
	if loaded.BossDefeated {
		t.Error("BossDefeated set on a non-boss region clear")
	}
}

// Multi-region zone: clearing the zone-boss region completes the expedition.
func TestFinalizeExpeditionOnZoneClear_ZoneBossRegionCompletes(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zclear-zoneboss:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "run-1",
		ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := setCurrentRegion(exp, "underdark_deep_throne"); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	p.finalizeExpeditionOnZoneClear(uid, "run-1")

	loaded, _ := getExpedition(exp.ID)
	if loaded.Status != ExpeditionStatusComplete {
		t.Errorf("status = %q, want complete after zone-boss region clear", loaded.Status)
	}
	if !loaded.BossDefeated {
		t.Error("BossDefeated not set after zone-boss region clear")
	}
}
