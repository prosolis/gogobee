package plugin

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

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
