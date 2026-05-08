package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E2d — Fortified camp branch.

func TestCampCmd_FortifiedRequiresBossDefeated(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-fortified-pre:example")
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
	got, _ := getActiveExpedition(uid)
	if got.Camp != nil && got.Camp.Active {
		t.Error("fortified camp should be rejected before boss defeated")
	}
}

func TestCampCmd_FortifiedAfterBossSucceeds(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-fortified-post:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(`UPDATE dnd_expedition SET boss_defeated = 1 WHERE expedition_id = ?`, exp.ID); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleCampCmd(MessageContext{Sender: uid}, "fortified"); err != nil {
		t.Fatal(err)
	}
	got, _ := getActiveExpedition(uid)
	if got.Camp == nil || !got.Camp.Active || got.Camp.Type != CampTypeFortified {
		t.Errorf("expected active fortified camp, got %+v", got.Camp)
	}
	if got.Supplies.Current != 3 { // 5 − 2 SU
		t.Errorf("supplies after fortified pitch = %v, want 3", got.Supplies.Current)
	}
}

func TestProcessOvernightCamp_StandardRefillsHP(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-rest-std:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = 5
	_ = SaveDnDCharacter(c)

	exp, _ := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	exp.Camp = &CampState{Active: true, Type: CampTypeStandard, EstablishedAt: time.Now().UTC()}
	_ = updateCamp(exp.ID, exp.Camp)

	summary := processOvernightCamp(exp)
	if !strings.Contains(strings.ToLower(summary), "long rest") {
		t.Errorf("standard rest summary = %q", summary)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != got.HPMax {
		t.Errorf("HP after standard rest = %d, want max %d", got.HPCurrent, got.HPMax)
	}
	postExp, _ := getExpedition(exp.ID)
	if postExp.Camp != nil {
		t.Error("camp should auto-break after rest")
	}
}

func TestProcessOvernightCamp_FortifiedReducesThreatAndAddsBonus(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-rest-fort:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = c.HPMax // already full; bonus would clip at max
	c.HPMax = 30
	c.HPCurrent = 30
	_ = SaveDnDCharacter(c)

	exp, _ := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err := applyThreatDelta(exp.ID, 40, "seed"); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	exp.Camp = &CampState{Active: true, Type: CampTypeFortified, EstablishedAt: time.Now().UTC()}
	_ = updateCamp(exp.ID, exp.Camp)

	summary := processOvernightCamp(exp)
	if !strings.Contains(strings.ToLower(summary), "fortified rest") {
		t.Errorf("fortified rest summary = %q", summary)
	}
	postExp, _ := getExpedition(exp.ID)
	if postExp.ThreatLevel != 35 {
		t.Errorf("threat after fortified rest = %d, want 35", postExp.ThreatLevel)
	}
	if postExp.Camp != nil {
		t.Error("camp should auto-break after rest")
	}
}

func TestProcessOvernightCamp_RoughHalfHPFloor(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@camp-rest-rough:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	c, _ := LoadDnDCharacter(uid)
	c.HPMax = 20
	c.HPCurrent = 4 // way under half
	_ = SaveDnDCharacter(c)

	exp, _ := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	exp.Camp = &CampState{Active: true, Type: CampTypeRough, EstablishedAt: time.Now().UTC()}
	_ = updateCamp(exp.ID, exp.Camp)

	_ = processOvernightCamp(exp)
	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != 10 {
		t.Errorf("rough rest HP = %d, want 10 (half of 20)", got.HPCurrent)
	}
}
