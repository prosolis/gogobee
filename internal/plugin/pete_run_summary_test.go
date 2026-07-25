package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// finishRun writes a small realistic run's beats and closes it.
func finishRun(runID, name string) {
	recordRunBeat(runID, peteclient.RunBeat{Kind: "start", Token: "tok", Name: name,
		Level: 14, Zone: "Crypt of Valdris", TotalRooms: 9, Room: 1})
	recordRunBeat(runID, peteclient.RunBeat{Kind: "combat", Room: 2, Target: "Bone Chanter",
		Outcome: "won", Amount: 7, HP: 61, HPMax: 68})
	recordRunBeat(runID, peteclient.RunBeat{Kind: "trap", Room: 3, Outcome: "sprung",
		Amount: 22, HP: 39, HPMax: 68})
	recordRunBeat(runID, peteclient.RunBeat{Kind: "end", Room: 3, Outcome: "died"})
}

// TestSummarySweepPicksAFinishedRunOnce. The sweep runs every two minutes
// forever, so the property that matters is not that it finds a run — it is that
// it lets one go. A run that stayed pickable would author a fresh summary every
// tick for the rest of the week.
func TestSummarySweepPicksAFinishedRunOnce(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	seedBeatRun(t, "run-done", id.UserID("@josie:example.com"))
	finishRun("run-done", "Josie")

	if got := nextRunNeedingSummary(); got != "run-done" {
		t.Fatalf("sweep didn't find the finished run: %q", got)
	}
	// Filing the closing beat is what retires it, whether or not any prose was
	// authored into it.
	recordRunBeat("run-done", peteclient.RunBeat{Kind: "summary"})
	if got := nextRunNeedingSummary(); got != "" {
		t.Errorf("run came back round after its summary beat was filed: %q", got)
	}
}

// TestSummarySweepIgnoresARunStillWalking. A summary is a reading of a finished
// run. Writing one over a run in progress would be an ending invented before
// there was one.
func TestSummarySweepIgnoresARunStillWalking(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	seedBeatRun(t, "run-live", id.UserID("@josie:example.com"))
	recordRunBeat("run-live", peteclient.RunBeat{Kind: "start", Name: "Josie", Zone: "Crypt"})
	recordRunBeat("run-live", peteclient.RunBeat{Kind: "combat", Target: "Rat", Outcome: "won"})

	if got := nextRunNeedingSummary(); got != "" {
		t.Errorf("picked a run that hasn't ended: %q", got)
	}
}

// TestSummarySweepRetiresAnOptedOutRun. Their beats never leave the box, so
// there is nothing on Pete for a summary to attach to — but the run must still
// stop being picked, or the sweep spends a generation on it every tick and
// throws the result away.
func TestSummarySweepRetiresAnOptedOutRun(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	uid := id.UserID("@quiet:example.com")
	seedBeatRun(t, "run-quiet", uid)
	finishRun("run-quiet", "Quack")
	setNewsOptout(uid, true)

	if got := nextRunNeedingSummary(); got != "" {
		t.Errorf("offered an opted-out player's run for summarising: %q", got)
	}
	kinds := beatKinds(t, "run-quiet")
	if kinds[len(kinds)-1] != "summary" {
		t.Errorf("opted-out run wasn't closed out; kinds = %v", kinds)
	}
	// And it stays closed out.
	if got := nextRunNeedingSummary(); got != "" {
		t.Errorf("opted-out run came back round: %q", got)
	}
}

// TestSummaryPromptCarriesTheRunAndOnlyOneName.
//
// The prompt is the whole safety story on this side (Pete's guard is the other
// half, and it only ever sees the answer). A run log is full of monster names,
// and a model asked to write warmly about "the party" will happily invent a
// second member of it — which on a public page is words put in a real person's
// mouth. So the one name is stated twice and the facts are handed over as facts.
func TestSummaryPromptCarriesTheRunAndOnlyOneName(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	seedBeatRun(t, "run-p", id.UserID("@josie:example.com"))
	finishRun("run-p", "Josie")

	beats, err := loadRunBeatsForSummary("run-p")
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) != 4 {
		t.Fatalf("want 4 beats, got %d", len(beats))
	}
	if beats[0].Seq >= beats[len(beats)-1].Seq {
		t.Error("beats came back out of order; the log would read backwards")
	}
	if got := runSummarySubject(beats); got != "Josie" {
		t.Fatalf("subject = %q, want Josie", got)
	}

	p := buildRunSummaryPrompt("Josie", beats)
	for _, want := range []string{
		"The ONLY adventurer name you may use is: Josie",
		"Josie went into Crypt of Valdris, and they died down there",
		"killed Bone Chanter, taking 7 damage",
		"sprung a trap for 22 damage",
		"did not come home",
		"Three sentences at most",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

// TestUnattributedRunGetsNoSummary. Only the `start` beat carries identity. A
// run that lost it has no name to hand the guard, so Pete would reject any
// summary naming anybody — spending a generation to have it thrown away, and
// risking an anonymous paragraph about a player nobody can consent for.
func TestUnattributedRunGetsNoSummary(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	seedBeatRun(t, "run-anon", id.UserID("@josie:example.com"))
	recordRunBeat("run-anon", peteclient.RunBeat{Kind: "combat", Target: "Rat", Outcome: "won"})
	recordRunBeat("run-anon", peteclient.RunBeat{Kind: "end", Outcome: "cleared"})

	summary, name := authorRunSummary("run-anon")
	if summary != "" || name != "" {
		t.Errorf("authored over a run with no owner: name=%q summary=%q", name, summary)
	}
}

// TestParseRunSummaryTolerance mirrors parseDispatch's: the model wraps its
// answer in reasoning blocks, fences and apologies, and none of that is a reason
// to lose a summary that is sitting right there.
func TestParseRunSummaryTolerance(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"plain", `{"summary": "It went badly."}`, "It went badly."},
		{"think block", "<think>hmm</think>\n{\"summary\": \"It went badly.\"}", "It went badly."},
		{"fenced with chatter", "Sure!\n```json\n{\"summary\": \"It went badly.\"}\n```\n", "It went badly."},
		{"no json", "It went badly.", ""},
		{"empty summary", `{"summary": "   "}`, ""},
	}
	for _, c := range cases {
		if got := parseRunSummary(c.raw); got != c.want {
			t.Errorf("%s: parseRunSummary = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestDispatchRunLinkNeedsARecentRunWithBeats.
//
// latestRunIDForNews answers "which run is this dispatch about", and it is asked
// from call sites that have already let go of the run. Both of its guards are
// load-bearing: a run with no beats behind it would mint a dispatch link to a
// 404, and a stale run would attach a campaign death at the Empty Throne to
// whatever dungeon that player last walked.
func TestDispatchRunLinkNeedsARecentRunWithBeats(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	uid := id.UserID("@josie:example.com")

	// A run with no beats: pre-liveblog, or the seam was off while it walked.
	seedBeatRun(t, "run-silent", uid)
	if got := latestRunIDForNews(uid); got != "" {
		t.Errorf("linked a dispatch to a run Pete has never heard of: %q", got)
	}

	// The real one, closed seconds ago.
	seedBeatRun(t, "run-real", uid)
	finishRun("run-real", "Josie")
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET completed_at = CURRENT_TIMESTAMP WHERE run_id = 'run-real'`); err != nil {
		t.Fatal(err)
	}
	if got := latestRunIDForNews(uid); got != "run-real" {
		t.Errorf("run link = %q, want run-real", got)
	}

	// A day later, they die somewhere that isn't a dungeon at all.
	if _, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET completed_at = datetime('now', '-1 day') WHERE run_id = 'run-real'`); err != nil {
		t.Fatal(err)
	}
	if got := latestRunIDForNews(uid); got != "" {
		t.Errorf("attached an unrelated death to yesterday's expedition: %q", got)
	}
}
