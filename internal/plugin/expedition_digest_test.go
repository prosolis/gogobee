package plugin

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// Coverage for the once-a-day cadence (2026-07-26). The behavioural tests
// below are the point of the file: before this change nothing asserted that
// the recap and ambient paths sent a DM, so nothing would have caught them
// silently continuing to send — or, worse, silently dropping the mechanical
// effect along with the message.

func TestRenderOvernightDigest_CollapsesWalksAndSkipsFrameTypes(t *testing.T) {
	entries := []ExpeditionEntry{
		{Type: "walk", Summary: "walked into the sump"},
		{Type: "walk", Summary: "walked into the gallery"},
		{Type: "walk", Summary: "walked into the stair"},
		{Type: "briefing", Summary: "morning briefing — 1.0 SU consumed overnight"},
		{Type: "narrative", Summary: "the corridor bends left"},
		{Type: "ambient", Summary: "ambient: pack_rat — Supplies -0.5"},
		{Type: "night", Summary: "Signs of passage near camp; no encounter."},
		{Type: "recap", Summary: "evening recap — 6 log entries today"},
	}
	got := renderOvernightDigest(entries)

	if !strings.Contains(got, "walked 3 rooms") {
		t.Errorf("walks not collapsed to a count:\n%s", got)
	}
	if !strings.Contains(got, "ambient: pack_rat") {
		t.Errorf("ambient entry missing from digest:\n%s", got)
	}
	if !strings.Contains(got, "Signs of passage") {
		t.Errorf("night check missing from digest:\n%s", got)
	}
	for _, unwanted := range []string{"morning briefing", "evening recap", "corridor bends"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("digest echoed frame/narration entry %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderOvernightDigest_CapsAndReportsOverflow(t *testing.T) {
	var entries []ExpeditionEntry
	for i := 0; i < digestMaxLines+3; i++ {
		entries = append(entries, ExpeditionEntry{
			Type:    "ambient",
			Summary: fmt.Sprintf("ambient event %d", i),
		})
	}
	got := renderOvernightDigest(entries)

	if n := strings.Count(got, "ambient event"); n != digestMaxLines {
		t.Errorf("digest carried %d lines, want the cap of %d:\n%s", n, digestMaxLines, got)
	}
	// A truncated digest must say so — otherwise it reads as the whole day.
	if !strings.Contains(got, "and 3 more") {
		t.Errorf("overflow not reported:\n%s", got)
	}
}

func TestRenderOvernightDigest_EmptyWhenNothingNotable(t *testing.T) {
	entries := []ExpeditionEntry{
		{Type: "briefing", Summary: "morning briefing"},
		{Type: "narrative", Summary: "dust everywhere"},
	}
	if got := renderOvernightDigest(entries); got != "" {
		t.Errorf("want no digest block for an unremarkable day, got:\n%s", got)
	}
}

func TestAdventureWhoURL_UsesRosterTokenNotHandle(t *testing.T) {
	// The roster token is salted from a DB-persisted secret.
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest-url:example")
	got := adventureWhoURL(uid)

	want := "/adventure/who/" + eventToken(uid, "roster")
	if !strings.HasSuffix(got, want) {
		t.Errorf("who URL = %q, want suffix %q", got, want)
	}
	if strings.Contains(got, "digest-url") {
		t.Errorf("who URL leaked the Matrix handle: %q", got)
	}
}

func TestAdventureWhoURL_FallsBackToFeedWithoutUser(t *testing.T) {
	setupZoneRunTestDB(t)
	if got := adventureWhoURL(""); got != adventureFeedURL() {
		t.Errorf("empty user should fall back to the feed, got %q", got)
	}
}

// TestBuildAutoRunDM_NightCampIsSilent — the autopilot's end-of-day digest was
// the last recurring second DM of the day. It goes quiet; the camp writes its
// own `rest` log entry, so the morning briefing still reports it.
func TestBuildAutoRunDM_NightCampIsSilent(t *testing.T) {
	r := autopilotWalkResult{rooms: 4, reason: stopOK, stream: []string{"…walked…"}}
	camp := "\n\n⛺ **Autopilot camp** — night"
	body, ok := buildAutoRunDM("expid", r, camp, autoCampDecision{
		Kind: CampTypeStandard, Night: true,
	})
	if ok || body != "" {
		t.Errorf("night camp should be silent, got ok=%v body=%q", ok, body)
	}
}

// The two interactive surfaces the night-camp cut must not touch.
func TestBuildAutoRunDM_KeepSetStillSurfaces(t *testing.T) {
	fork := autopilotWalkResult{rooms: 1, reason: stopFork, finalMsg: "pick a path"}
	if body, ok := buildAutoRunDM("expid", fork, "", autoCampDecision{}); !ok ||
		!strings.Contains(body, "pick a path") {
		t.Errorf("fork must still surface, got ok=%v body=%q", ok, body)
	}

	hold := autopilotWalkResult{rooms: 2, reason: stopBossSafety}
	holdCamp := "\n\n⛺ **Rest camp**"
	if body, ok := buildAutoRunDM("expid", hold, holdCamp, autoCampDecision{
		Reason: "boss-safety hold — resting before re-engaging",
	}); !ok || !strings.Contains(body, "Holding before the boss") {
		t.Errorf("boss-safety hold must still surface, got ok=%v body=%q", ok, body)
	}
}

// TestDeliverAmbient_SilentButStillLogs — the ambient event still fires and
// still records itself; it just stops DMing.
func TestDeliverAmbient_SilentButStillLogs(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest-ambient:example")
	defer cleanupExpeditions(uid)

	p := &AdventurePlugin{}
	sink := installSink(p)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.deliverAmbient(exp, exp.StartDate.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if dms := sink.dmsTo(uid); len(dms) != 0 {
		t.Errorf("ambient sent %d DM(s), want 0:\n%s", len(dms), strings.Join(dms, "\n---\n"))
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	found := false
	for _, e := range entries {
		if e.Type == "ambient" {
			found = true
		}
	}
	if !found {
		t.Error("ambient event fired without writing a log entry — the digest would lose it")
	}
}

// TestDeliverRecap_SilentButStillRunsNightCheck — the recap's mechanical half
// (wandering check, threat bump) must survive the message going away.
func TestDeliverRecap_SilentButStillRunsNightCheck(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest-recap:example")
	defer cleanupExpeditions(uid)

	p := &AdventurePlugin{}
	sink := installSink(p)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	exp.Camp = &CampState{Active: true, Type: CampTypeStandard, EstablishedAt: exp.StartDate}
	if err := updateCamp(exp.ID, exp.Camp); err != nil {
		t.Fatal(err)
	}

	recapAt := exp.StartDate.Add(12 * time.Hour)
	if err := p.deliverRecap(exp, recapAt); err != nil {
		t.Fatal(err)
	}

	if dms := sink.dmsTo(uid); len(dms) != 0 {
		t.Errorf("recap sent %d DM(s), want 0:\n%s", len(dms), strings.Join(dms, "\n---\n"))
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	sawNight, sawRecap := false, false
	for _, e := range entries {
		switch e.Type {
		case "night":
			sawNight = true
		case "recap":
			sawRecap = true
		}
	}
	if !sawNight {
		t.Error("night wandering check did not run — the recap dropped its mechanics, not just its DM")
	}
	if !sawRecap {
		t.Error("recap log entry missing — the site loses its day boundary")
	}
}

// TestDeliverBriefing_IsTheOneDailyMessage — the payoff: one DM, carrying the
// prior day's silent activity and the reader's own site link.
func TestDeliverBriefing_CarriesDigestAndSiteLink(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@digest-briefing:example")
	defer cleanupExpeditions(uid)

	p := &AdventurePlugin{}
	sink := installSink(p)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Stand in for the day that just went by silently.
	if err := appendExpeditionLog(exp.ID, exp.CurrentDay, "ambient",
		"ambient: pack_rat — Supplies -0.5", "Something nibbled the stores."); err != nil {
		t.Fatal(err)
	}

	if err := p.deliverBriefing(exp, exp.StartDate.Add(20*time.Hour)); err != nil {
		t.Fatal(err)
	}

	dms := sink.dmsTo(uid)
	if len(dms) != 1 {
		t.Fatalf("briefing sent %d DM(s), want exactly 1:\n%s", len(dms), strings.Join(dms, "\n---\n"))
	}
	body := dms[0]
	if !strings.Contains(body, "Since yesterday") {
		t.Errorf("briefing missing the overnight digest block:\n%s", body)
	}
	if !strings.Contains(body, "ambient: pack_rat") {
		t.Errorf("digest did not carry the silent ambient event:\n%s", body)
	}
	if !strings.Contains(body, adventureWhoURL(uid)) {
		t.Errorf("briefing missing the reader's own site link:\n%s", body)
	}
}
