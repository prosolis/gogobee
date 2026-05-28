package plugin

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

func TestDecideAutopilotCamp_SkipsWhenCamped(t *testing.T) {
	in := autoCampInputs{
		Camp:     &CampState{Active: true, Type: CampTypeRough},
		Run:      &DungeonRun{CurrentRoom: 0, RoomsCleared: []int{0}},
		HPCur:    1, HPMax: 20, // very low — would otherwise camp
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	if _, ok := decideAutopilotCamp(in); ok {
		t.Errorf("expected no camp when already camped")
	}
}

func TestDecideAutopilotCamp_SkipsHealthyHP(t *testing.T) {
	in := autoCampInputs{
		Run:      &DungeonRun{CurrentRoom: 0, RoomsCleared: []int{0}},
		HPCur:    20, HPMax: 20,
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	if _, ok := decideAutopilotCamp(in); ok {
		t.Errorf("expected no camp when HP full")
	}
}

func TestDecideAutopilotCamp_LowHPClearedPitchesStandard(t *testing.T) {
	in := autoCampInputs{
		Run:      &DungeonRun{CurrentRoom: 1, RoomsCleared: []int{0, 1}},
		HPCur:    5, HPMax: 20, // 25% — below 55% threshold
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	d, ok := decideAutopilotCamp(in)
	if !ok || d.Kind != CampTypeStandard {
		t.Errorf("expected standard camp, got %+v ok=%v", d, ok)
	}
}

func TestDecideAutopilotCamp_LowHPUnclearedPitchesRough(t *testing.T) {
	in := autoCampInputs{
		Run:      &DungeonRun{CurrentRoom: 2, RoomsCleared: []int{0, 1}},
		HPCur:    5, HPMax: 20,
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	d, ok := decideAutopilotCamp(in)
	if !ok || d.Kind != CampTypeRough {
		t.Errorf("expected rough camp, got %+v ok=%v", d, ok)
	}
}

func TestDecideAutopilotCamp_DowngradesWhenSuppliesTight(t *testing.T) {
	in := autoCampInputs{
		Run:      &DungeonRun{CurrentRoom: 1, RoomsCleared: []int{0, 1}},
		HPCur:    5, HPMax: 20,
		Supplies: ExpeditionSupplies{Current: 0.6, Max: 5, DailyBurn: 1}, // can't afford 1.0
	}
	d, ok := decideAutopilotCamp(in)
	if !ok || d.Kind != CampTypeRough {
		t.Errorf("expected downgrade to rough, got %+v ok=%v", d, ok)
	}
}

func TestDecideAutopilotCamp_SkipsBossRoom(t *testing.T) {
	in := autoCampInputs{
		Run: &DungeonRun{
			CurrentRoom: 0, RoomsCleared: []int{},
			RoomSeq: []RoomType{RoomBoss},
		},
		HPCur: 1, HPMax: 20,
	}
	if _, ok := decideAutopilotCamp(in); ok {
		t.Errorf("expected no camp in boss room")
	}
}

func TestDecideAutopilotCamp_BaseCampWhenRegionCleared(t *testing.T) {
	in := autoCampInputs{
		Run:           &DungeonRun{CurrentRoom: 0, RoomsCleared: []int{0}},
		Multi:         true,
		RegionCleared: true,
		BaseSite:      true,
		BaseAlready:   false,
		HPCur:         20, HPMax: 20, // healthy — still pitch base waypoint
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	d, ok := decideAutopilotCamp(in)
	if !ok || d.Kind != CampTypeBase {
		t.Errorf("expected base camp, got %+v ok=%v", d, ok)
	}
}

func TestDecideAutopilotCamp_BaseCampSkippedIfAlreadyPersisted(t *testing.T) {
	in := autoCampInputs{
		Run:           &DungeonRun{CurrentRoom: 0, RoomsCleared: []int{0}},
		Multi:         true,
		RegionCleared: true,
		BaseSite:      true,
		BaseAlready:   true,
		HPCur:         20, HPMax: 20,
		Supplies: ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
	}
	if _, ok := decideAutopilotCamp(in); ok {
		t.Errorf("expected no second base-camp pitch")
	}
}

func TestShouldSkipAutoRunForCamp(t *testing.T) {
	now := time.Now().UTC()
	stickyPlayerCamp := &Expedition{Camp: &CampState{
		Active: true, Type: CampTypeStandard, AutoPitched: false,
		EstablishedAt: now.Add(-10 * time.Hour),
	}}
	if !shouldSkipAutoRunForCamp(stickyPlayerCamp, now) {
		t.Error("player camp should always block autorun")
	}

	freshAuto := &Expedition{Camp: &CampState{
		Active: true, Type: CampTypeStandard, AutoPitched: true,
		EstablishedAt: now.Add(-1 * time.Hour),
	}}
	if !shouldSkipAutoRunForCamp(freshAuto, now) {
		t.Error("auto camp within dwell window should skip")
	}

	staleAuto := &Expedition{Camp: &CampState{
		Active: true, Type: CampTypeStandard, AutoPitched: true,
		EstablishedAt: now.Add(-(minAutoCampDwell + time.Minute)),
	}}
	if shouldSkipAutoRunForCamp(staleAuto, now) {
		t.Error("auto camp past dwell should let walk proceed")
	}

	none := &Expedition{}
	if shouldSkipAutoRunForCamp(none, now) {
		t.Error("no camp should not skip")
	}
}

func TestPitchAutopilotCamp_DeductsSuppliesAndRestores(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@autocamp-pitch:example")
	campTestCharacter(t, uid, 1)
	// Damage the character so the standard rest can heal them.
	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = 4
	_ = SaveDnDCharacter(c)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	block, err := p.pitchAutopilotCamp(exp, autoCampDecision{
		Kind: CampTypeStandard, Reason: "test pitch",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if block == "" {
		t.Error("expected non-empty camp block")
	}

	fresh, _ := getActiveExpedition(uid)
	if fresh.Camp == nil || !fresh.Camp.Active {
		t.Fatal("expected camp pitched")
	}
	if !fresh.Camp.AutoPitched {
		t.Error("expected AutoPitched flag set")
	}
	if !fresh.Camp.RestApplied {
		t.Error("expected RestApplied flag set")
	}
	if fresh.Supplies.Current != 4.0 {
		t.Errorf("supplies = %v, want 4.0 after standard pitch", fresh.Supplies.Current)
	}
	cc, _ := LoadDnDCharacter(uid)
	if cc.HPCurrent != cc.HPMax {
		t.Errorf("HP = %d/%d, want full after standard rest", cc.HPCurrent, cc.HPMax)
	}
}

func TestBreakAutoCampIfDue_RespectsDwellWindow(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@autocamp-break:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	exp.Camp = &CampState{
		Active: true, Type: CampTypeRough, AutoPitched: true,
		EstablishedAt: now.Add(-1 * time.Hour),
	}
	if err := updateCamp(exp.ID, exp.Camp); err != nil {
		t.Fatal(err)
	}
	if breakAutoCampIfDue(exp, now) {
		t.Error("camp inside dwell window should not be broken")
	}

	exp.Camp.EstablishedAt = now.Add(-(minAutoCampDwell + time.Minute))
	if err := updateCamp(exp.ID, exp.Camp); err != nil {
		t.Fatal(err)
	}
	if !breakAutoCampIfDue(exp, now) {
		t.Error("camp past dwell window should be broken")
	}
	fresh, _ := getActiveExpedition(uid)
	if fresh.Camp != nil && fresh.Camp.Active {
		t.Errorf("expected camp cleared, got %+v", fresh.Camp)
	}
}

func TestDecideAutopilotCamp_NightTriggerOnEventAnchored(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-(nightCampWindow + time.Hour))
	in := autoCampInputs{
		Run:            &DungeonRun{CurrentRoom: 1, RoomsCleared: []int{0, 1}},
		HPCur:          20, HPMax: 20, // healthy — only the night window should pitch
		Supplies:       ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
		Now:            now,
		EventAnchored:  true,
		LastBriefingAt: &stale,
	}
	d, ok := decideAutopilotCamp(in)
	if !ok {
		t.Fatal("expected night camp pitch")
	}
	if !d.Night {
		t.Errorf("Night = false, want true (end of day)")
	}
	if d.Kind != CampTypeStandard {
		t.Errorf("Kind = %s, want standard (cleared room)", d.Kind)
	}
}

func TestDecideAutopilotCamp_NightSkippedOnLegacy(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-24 * time.Hour)
	in := autoCampInputs{
		Run:            &DungeonRun{CurrentRoom: 1, RoomsCleared: []int{0, 1}},
		HPCur:          20, HPMax: 20,
		Supplies:       ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
		Now:            now,
		EventAnchored:  false, // legacy — only HP-low triggers
		LastBriefingAt: &stale,
	}
	if _, ok := decideAutopilotCamp(in); ok {
		t.Error("legacy expedition should not pitch a night camp on time alone")
	}
}

func TestDecideAutopilotCamp_NightFlagSetEvenOnHPLow(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-(nightCampWindow + time.Hour))
	in := autoCampInputs{
		Run:            &DungeonRun{CurrentRoom: 1, RoomsCleared: []int{0, 1}},
		HPCur:          5, HPMax: 20, // also low
		Supplies:       ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1},
		Now:            now,
		EventAnchored:  true,
		LastBriefingAt: &stale,
	}
	d, ok := decideAutopilotCamp(in)
	if !ok || !d.Night {
		t.Errorf("expected Night pitch even when HP also low, got %+v ok=%v", d, ok)
	}
}

func TestPitchAutopilotCamp_NightRunsProcessNightCamp(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@autocamp-night:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Force event-anchored for this test.
	saved := eventAnchoredCutoff
	eventAnchoredCutoff = exp.StartDate.Add(-time.Minute)
	defer func() { eventAnchoredCutoff = saved }()

	startDay := exp.CurrentDay
	startSU := exp.Supplies.Current
	p := &AdventurePlugin{}
	_, err = p.pitchAutopilotCamp(exp, autoCampDecision{
		Kind: CampTypeStandard, Reason: "night-camp test", Night: true,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := getActiveExpedition(uid)
	if got.CurrentDay != startDay+1 {
		t.Errorf("day = %d, want %d (night camp advances day)", got.CurrentDay, startDay+1)
	}
	// Night burn (0.5 SU) + camp cost (1.0 SU) = 1.5 SU; 5 - 1.5 = 3.5.
	wantSU := startSU - 0.5 - campSupplyCost[CampTypeStandard]
	if got.Supplies.Current != wantSU {
		t.Errorf("supplies = %v, want %v (night burn + camp cost)", got.Supplies.Current, wantSU)
	}
	if got.LastBriefingAt == nil {
		t.Error("LastBriefingAt should be stamped by night rollover")
	}
}

func TestBreakAutoCampIfDue_LeavesPlayerCampAlone(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@autocamp-playercamp:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	exp.Camp = &CampState{
		Active: true, Type: CampTypeStandard, AutoPitched: false,
		EstablishedAt: now.Add(-10 * time.Hour),
	}
	if err := updateCamp(exp.ID, exp.Camp); err != nil {
		t.Fatal(err)
	}
	if breakAutoCampIfDue(exp, now) {
		t.Error("player camp must not be auto-broken")
	}
}
