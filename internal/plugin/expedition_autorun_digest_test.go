package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// Long-expedition D4-a — verifies the EoD digest renderer pulls only
// the prior day's structured entries and that buildAutoRunDM's surface
// rules suppress / surface the right ticks.

func TestRenderEndOfDayDigest_GroupsByType(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	day := exp.CurrentDay
	for _, e := range []struct{ kind, summary, flavor string }{
		{"walk", "auto-walk: 3 room(s)", ""},
		{"walk", "auto-walk: 2 room(s)", ""},
		{"harvest", "autopilot harvest oak success", ""},
		{"harvest", "autopilot harvest iron failure", ""}, // not counted
		{"interrupt", "noise alarm total=1", ""},
		{"threat", "drift", "The dungeon stirs awake."},
		{"milestone", "milestone awarded: first-night", ""},
		{"narrative", "autopilot broke camp (dwell elapsed)", ""}, // filtered
		{"narrative", "voluntary extraction prompt", ""},
	} {
		if err := appendExpeditionLog(exp.ID, day, e.kind, e.summary, e.flavor); err != nil {
			t.Fatal(err)
		}
	}
	// Entry from a different day must NOT bleed in.
	if err := appendExpeditionLog(exp.ID, day+1, "walk", "next day", ""); err != nil {
		t.Fatal(err)
	}

	got := renderEndOfDayDigest(exp.ID, day)
	if got == "" {
		t.Fatal("expected non-empty digest")
	}
	wantSubs := []string{
		"Walked **2**",
		"Handled **1** interrupt",
		"**1** harvest",
		"dungeon stirs awake",
		"milestone awarded: first-night",
		"voluntary extraction prompt",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("digest missing %q\n--- got ---\n%s", s, got)
		}
	}
	if strings.Contains(got, "autopilot broke camp") {
		t.Errorf("digest should filter housekeeping narrative\n%s", got)
	}
	if strings.Contains(got, "next day") {
		t.Errorf("digest leaked entry from a later day\n%s", got)
	}
}

func TestRenderEndOfDayDigest_EmptyDayReturnsEmpty(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest-empty:example")
	campTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 5, Max: 5, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := renderEndOfDayDigest(exp.ID, exp.CurrentDay); got != "" {
		t.Errorf("expected empty digest for a day with no entries, got %q", got)
	}
}

func TestBuildAutoRunDM_SuppressesQuietTick(t *testing.T) {
	r := autopilotWalkResult{rooms: 3, reason: stopOK, stream: []string{"…walked…"}}
	body, ok := buildAutoRunDM("expid", r, "", autoCampDecision{})
	if ok || body != "" {
		t.Errorf("quiet stopOK tick should be silent, got ok=%v body=%q", ok, body)
	}
}

func TestBuildAutoRunDM_ForkSurfaces(t *testing.T) {
	r := autopilotWalkResult{rooms: 1, reason: stopFork, finalMsg: "pick a path"}
	body, ok := buildAutoRunDM("expid", r, "", autoCampDecision{})
	if !ok || !strings.Contains(body, "pick a path") {
		t.Errorf("fork should surface, got ok=%v body=%q", ok, body)
	}
}

func TestBuildAutoRunDM_NonNightCampSurfaces(t *testing.T) {
	r := autopilotWalkResult{rooms: 2, reason: stopPreflight, finalMsg: "low hp"}
	camp := "\n\n⛺ **Autopilot camp**"
	body, ok := buildAutoRunDM("expid", r, camp, autoCampDecision{Kind: CampTypeRough})
	if !ok || !strings.Contains(body, "Autopilot camp") {
		t.Errorf("mid-day camp pitch should surface camp block, got ok=%v body=%q", ok, body)
	}
	// Walk stream/final message dropped — D4-a suppresses the walk narration.
	if strings.Contains(body, "low hp") {
		t.Errorf("non-night camp DM should not carry walk finalMsg, got %q", body)
	}
}

func TestBuildAutoRunDM_BossSafetyHasHoldHeader(t *testing.T) {
	r := autopilotWalkResult{rooms: 0, reason: stopBossSafety}
	camp := "\n\n⛺ **Autopilot camp — Standard.**"
	dec := autoCampDecision{Kind: CampTypeStandard, Reason: "boss-safety hold — resting before re-engaging"}
	body, ok := buildAutoRunDM("expid", r, camp, dec)
	if !ok || !strings.Contains(body, "Holding before the boss") {
		t.Errorf("boss-safety camp should prepend hold header, got ok=%v body=%q", ok, body)
	}
}
