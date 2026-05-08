package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func campTestCharacter(t *testing.T, uid id.UserID, level int) {
	t.Helper()
	if err := createAdvCharacter(uid, "campcmd"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 14, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 10,
		HPMax: 20, HPCurrent: 20, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
}

func TestCampCmd_NoExpeditionBlocked(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-noexp:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "rough"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp != nil {
		t.Error("no expedition should exist")
	}
}

func TestCampCmd_RoughDeducts0_5SU(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-rough:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "rough"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp == nil || !exp.Camp.Active {
		t.Fatal("expected camp pitched")
	}
	if exp.Camp.Type != CampTypeRough {
		t.Errorf("camp type = %q, want rough", exp.Camp.Type)
	}
	if exp.Supplies.Current != 4.5 {
		t.Errorf("supplies = %v, want 4.5 after 0.5 SU rough cost", exp.Supplies.Current)
	}
}

func TestCampCmd_StandardDeducts1SU(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-standard:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "standard"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp == nil || exp.Camp.Type != CampTypeStandard {
		t.Errorf("camp = %+v, want standard", exp.Camp)
	}
	if exp.Supplies.Current != 4.0 {
		t.Errorf("supplies = %v, want 4.0 after 1.0 SU standard cost", exp.Supplies.Current)
	}
}

func TestCampCmd_RejectsDoublePitch(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-double:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "rough"); err != nil {
		t.Fatal(err)
	}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "standard"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp == nil || exp.Camp.Type != CampTypeRough {
		t.Errorf("camp should still be rough after rejected second pitch, got %+v", exp.Camp)
	}
}

func TestCampCmd_BreakClearsCamp(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-break:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "rough"); err != nil {
		t.Fatal(err)
	}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "break"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp != nil {
		t.Errorf("camp should be cleared, got %+v", exp.Camp)
	}
}

func TestCampCmd_RejectsFortifiedAndBase(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-locked:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "fortified"); err != nil {
		t.Fatal(err)
	}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "base"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp != nil {
		t.Errorf("camp should not be pitched for locked types, got %+v", exp.Camp)
	}
}

func TestCampCmd_BaseRejectsNonBaseCampSiteRegion(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-base-bad-site:example")
	campTestCharacter(t, uid, 12)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Surface Tunnels has BaseCampSite=false. Even if we mark it
	// cleared, !camp base should reject.
	if _, err := MarkRegionBossDefeated(exp, "underdark_surface_tunnels"); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "base"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := getActiveExpedition(uid)
	if loaded.Camp != nil {
		t.Error("base camp should not pitch in non-base-camp-site region")
	}
}

func TestCampCmd_BaseRejectsUnclearedRegion(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-base-uncleared:example")
	campTestCharacter(t, uid, 12)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Move to Drow Outpost (BaseCampSite=true) but don't clear it.
	if err := setCurrentRegion(exp, "underdark_drow_outpost"); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "base"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := getActiveExpedition(uid)
	if loaded.Camp != nil {
		t.Error("base camp should not pitch before region boss is cleared")
	}
}

func TestCampCmd_BasePitchPersistsWaypoint(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-base-pitch:example")
	campTestCharacter(t, uid, 12)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := setCurrentRegion(exp, "underdark_drow_outpost"); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkRegionBossDefeated(exp, "underdark_drow_outpost"); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "base"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := getActiveExpedition(uid)
	if loaded.Camp == nil || loaded.Camp.Type != CampTypeBase {
		t.Fatalf("expected base camp pitched; got %+v", loaded.Camp)
	}
	if loaded.Supplies.Current != 7 { // 10 - 3 SU base cost
		t.Errorf("supplies after base pitch = %v, want 7", loaded.Supplies.Current)
	}
	if !HasBaseCampAt(loaded, "underdark_drow_outpost") {
		t.Error("base camp waypoint not persisted to RegionState")
	}
}

func TestCampCmd_InsufficientSuppliesRejected(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-broke:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 0.3, Max: 5, DailyBurn: 1, HarshMod: 1}); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "rough"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp.Camp != nil {
		t.Errorf("camp should not be pitched with 0.3 SU vs 0.5 cost, got %+v", exp.Camp)
	}
}

func TestCampLocationCheck_BossRoomBlocks(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-boss:example")
	defer cleanupZoneRuns(uid)
	defer cleanupExpeditions(uid)

	// Pre-create a zone run, then advance current_room to the boss room.
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Walk to the boss room directly (last index).
	bossIdx := -1
	for i, rt := range run.RoomSeq {
		if rt == RoomBoss {
			bossIdx = i
		}
	}
	if bossIdx < 0 {
		t.Fatal("no boss room in seq")
	}
	exp := &Expedition{RunID: run.RunID}
	// Force current_room to bossIdx.
	if _, err := db.Get().Exec(`UPDATE dnd_zone_run SET current_room = ? WHERE run_id = ?`, bossIdx, run.RunID); err != nil {
		t.Fatal(err)
	}
	cleared, problem := campLocationCheck(exp)
	if problem == "" {
		t.Error("expected boss-room rejection")
	}
	if cleared {
		t.Error("boss room should not read as cleared")
	}
}
