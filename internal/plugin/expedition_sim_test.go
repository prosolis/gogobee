package plugin

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// D7-b: applyAutoCamp drives maybeAutoCamp under the sim's synthetic
// clock. When simNow is past the nightCampWindow on an event-anchored
// expedition, the scheduler pitches a Night camp — current_day++,
// supplies burn, last_briefing_at stamped. The helper then advances
// simNow past minAutoCampDwell and breaks the camp so the next walk
// proceeds.
func TestSimRunner_ApplyAutoCamp_NightCampAdvancesDay(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-applycamp-night:example")
	campTestCharacter(t, uid, 3)
	defer cleanupExpeditions(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 3, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, run.RunID,
		ExpeditionSupplies{Current: 20, Max: 20, DailyBurn: 2, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	sim := &SimRunner{P: &AdventurePlugin{}}
	// Park simNow past the night-camp window so decideAutopilotCamp
	// flags Night=true on a healthy character.
	simNow := exp.StartDate.Add(nightCampWindow + time.Hour)
	startDay := exp.CurrentDay
	startSU := exp.Supplies.Current
	before := simNow

	night, err := sim.applyAutoCamp(exp, &simNow)
	if err != nil {
		t.Fatalf("applyAutoCamp: %v", err)
	}
	if !night {
		t.Fatal("expected Night=true beyond nightCampWindow")
	}
	if simNow.Sub(before) < minAutoCampDwell {
		t.Errorf("simNow advanced %v, want >= %v", simNow.Sub(before), minAutoCampDwell)
	}

	got, _ := getExpedition(exp.ID)
	if got == nil {
		t.Fatal("expedition missing after night camp")
	}
	if got.CurrentDay != startDay+1 {
		t.Errorf("CurrentDay = %d, want %d", got.CurrentDay, startDay+1)
	}
	if got.Supplies.Current >= startSU {
		t.Errorf("Supplies.Current = %v, want < %v (burn should have landed)", got.Supplies.Current, startSU)
	}
	if got.Camp != nil && got.Camp.Active {
		t.Errorf("camp should have been broken after dwell, got %+v", got.Camp)
	}
}

// HP-low mid-day rest: under nightCampWindow with a hurt character,
// maybeAutoCamp pitches a Standard camp (cleared room → standard) but
// doesn't advance the day. HP gets restored by applyCampRest; DayTicks
// stays untouched.
func TestSimRunner_ApplyAutoCamp_HPLowMidDayRest(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-applycamp-hp:example")
	campTestCharacter(t, uid, 3)
	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = 1 // far below autoCampHPPct (55%)
	_ = SaveDnDCharacter(c)
	defer cleanupExpeditions(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 3, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, run.RunID,
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	sim := &SimRunner{P: &AdventurePlugin{}}
	// simNow well inside the active-day window — no Night pitch.
	simNow := exp.StartDate.Add(2 * time.Hour)
	startDay := exp.CurrentDay
	startHP := c.HPCurrent

	night, err := sim.applyAutoCamp(exp, &simNow)
	if err != nil {
		t.Fatalf("applyAutoCamp: %v", err)
	}
	if night {
		t.Error("expected non-Night pitch within active-day window")
	}

	got, _ := getExpedition(exp.ID)
	if got == nil {
		t.Fatal("expedition missing")
	}
	if got.CurrentDay != startDay {
		t.Errorf("CurrentDay = %d, want %d (no day advance on HP rest)", got.CurrentDay, startDay)
	}
	c2, _ := LoadDnDCharacter(uid)
	if c2 == nil || c2.HPCurrent <= startHP {
		t.Errorf("HPCurrent = %v, want > %v (rest should heal)", c2.HPCurrent, startHP)
	}
	if got.Camp != nil && got.Camp.Active {
		t.Errorf("camp should have been broken after dwell, got %+v", got.Camp)
	}
}

// stopBossSafety: pitchBossSafetyCamp force-pitches a camp regardless
// of the regular HP threshold or the boss-room block in decideAutopilotCamp.
// applyAutoCampBossSafety wraps that and runs the same dwell/break dance.
func TestSimRunner_ApplyAutoCampBossSafety_PitchesAndHeals(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-bosssafety:example")
	campTestCharacter(t, uid, 3)
	c, _ := LoadDnDCharacter(uid)
	// HP at 80% — above autoCampHPPct so maybeAutoCamp would NOT fire,
	// but the boss-safety path should still pitch.
	c.HPCurrent = (c.HPMax * 80) / 100
	if c.HPCurrent < 1 {
		c.HPCurrent = 1
	}
	_ = SaveDnDCharacter(c)
	defer cleanupExpeditions(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 3, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, run.RunID,
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	sim := &SimRunner{P: &AdventurePlugin{}}
	simNow := exp.StartDate.Add(2 * time.Hour)
	startSU := exp.Supplies.Current
	startHP := c.HPCurrent
	before := simNow

	if _, err := sim.applyAutoCampBossSafety(exp, &simNow); err != nil {
		t.Fatalf("applyAutoCampBossSafety: %v", err)
	}
	if simNow.Sub(before) < minAutoCampDwell {
		t.Errorf("simNow advanced %v, want >= %v", simNow.Sub(before), minAutoCampDwell)
	}

	got, _ := getExpedition(exp.ID)
	if got == nil {
		t.Fatal("expedition missing after boss-safety pitch")
	}
	if got.Supplies.Current >= startSU {
		t.Errorf("Supplies.Current = %v, want < %v (camp cost should have debited)", got.Supplies.Current, startSU)
	}
	c2, _ := LoadDnDCharacter(uid)
	if c2 == nil || c2.HPCurrent <= startHP {
		t.Errorf("HPCurrent = %v, want > %v (rest should heal)", c2.HPCurrent, startHP)
	}
	if got.Camp != nil && got.Camp.Active {
		t.Errorf("camp should have been broken after dwell, got %+v", got.Camp)
	}
}

// No trigger → no pitch: healthy HP, fresh simNow, no region cleared.
// maybeAutoCamp should bail and applyAutoCamp returns (false, nil).
func TestSimRunner_ApplyAutoCamp_NoTriggerIsNoop(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-applycamp-noop:example")
	campTestCharacter(t, uid, 3)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	sim := &SimRunner{P: &AdventurePlugin{}}
	simNow := exp.StartDate.Add(time.Hour)
	before := simNow

	night, err := sim.applyAutoCamp(exp, &simNow)
	if err != nil {
		t.Fatalf("applyAutoCamp: %v", err)
	}
	if night {
		t.Error("expected no Night pitch from no-op call")
	}
	if !simNow.Equal(before) {
		t.Errorf("simNow advanced %v on no-op call", simNow.Sub(before))
	}
}

// D7-a: SimRunner.TickDay must advance event-anchored expeditions. The
// production deliverBriefing branch reads wall-clock time for its safety-
// net check, so the sim path short-circuits to processNightCamp + a
// Standard rest. Without this fix, CurrentDay never increments under
// D2-b and supply-burn baselining is impossible.
func TestSimRunner_TickDay_EventAnchoredAdvancesDay(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-tickday-evt:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 20, Max: 20, DailyBurn: 2, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)

	sim := &SimRunner{P: &AdventurePlugin{}}
	for i := 0; i < 3; i++ {
		fresh, _ := getExpedition(exp.ID)
		if fresh == nil {
			t.Fatalf("expedition vanished after %d ticks", i)
		}
		if err := sim.TickDay(fresh); err != nil {
			t.Fatalf("TickDay #%d: %v", i+1, err)
		}
	}

	got, _ := getExpedition(exp.ID)
	if got == nil {
		t.Fatal("expedition missing after ticks")
	}
	if got.CurrentDay != 4 {
		t.Errorf("CurrentDay = %d, want 4 after 3 ticks (started at 1)", got.CurrentDay)
	}
	if got.Supplies.Current >= 20 {
		t.Errorf("Supplies.Current = %v, want < 20 (burn should have landed)", got.Supplies.Current)
	}
	if got.LastBriefingAt == nil {
		t.Fatal("LastBriefingAt nil — drift stamp didn't fire")
	}
	// Each tick stamps last_briefing_at at the synthetic briefAt
	// (start_date + N days at 06:00:30). After 3 ticks the stamp should
	// be at least 2 days past start_date.
	if got.LastBriefingAt.Sub(exp.StartDate) < 48*time.Hour {
		t.Errorf("LastBriefingAt %v not advanced enough vs start %v",
			got.LastBriefingAt, exp.StartDate)
	}
}

// Low-SU branch: a tight supplies budget shouldn't crash the sim — the
// rest is skipped, but burn/drift still fire so the day advances. This
// mirrors prod where a stalled autopilot in a low-SU state has its
// rollover force-fired by the safety net without a rest.
func TestSimRunner_TickDay_EventAnchoredLowSupplies(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@sim-tickday-lowsu:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		// 0.5 SU is below Rough camp cost (1 SU) but above zero, so the
		// rest branch is skipped while burn still proceeds.
		ExpeditionSupplies{Current: 0.5, Max: 20, DailyBurn: 0.2, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	useEventAnchored(t, exp)
	// Pin supplies low after start (startExpedition may reset them).
	exp.Supplies.Current = 0.5
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		t.Fatal(err)
	}

	sim := &SimRunner{P: &AdventurePlugin{}}
	if err := sim.TickDay(exp); err != nil {
		t.Fatalf("TickDay: %v", err)
	}
	got, _ := getExpedition(exp.ID)
	if got == nil {
		// Forced extraction from starvation is acceptable — the day still
		// advanced before extraction, which is what matters for the sim.
		return
	}
	if got.CurrentDay < 2 {
		t.Errorf("CurrentDay = %d, want >= 2", got.CurrentDay)
	}
}
