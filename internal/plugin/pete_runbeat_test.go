package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// seedBeatRun writes a dnd_zone_run row directly. The beat pusher resolves a
// run's owner through this table to decide whether the log may leave the box, so
// a test that skips it is testing a code path production never takes.
func seedBeatRun(t *testing.T, runID string, uid id.UserID) {
	t.Helper()
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_zone_run
			(run_id, user_id, zone_id, total_rooms, rooms_cleared, gm_mood,
			 current_node, visited_nodes, node_choices, rooms_traversed)
		VALUES (?, ?, 'goblin_warrens', 8, '[]', 50, 'goblin_warrens.r1', '["goblin_warrens.r1"]', '{}', 1)`,
		runID, string(uid)); err != nil {
		t.Fatalf("seed zone run: %v", err)
	}
}

func beatKinds(t *testing.T, runID string) []string {
	t.Helper()
	rows, err := db.Get().Query(
		`SELECT kind FROM pete_run_beat WHERE run_id = ? ORDER BY seq ASC`, runID)
	if err != nil {
		t.Fatalf("read beats: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		out = append(out, k)
	}
	return out
}

// TestRunBeatSeqIsMonotonicPerRun. (run_id, seq) is the identity Pete is
// idempotent on and it is also the render order, so a repeated or missing number
// is either a lost beat or a duplicated one. Two runs walking at once must not
// share a counter.
func TestRunBeatSeqIsMonotonicPerRun(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)

	for i := 0; i < 3; i++ {
		recordRunBeat("run-a", peteclient.RunBeat{Kind: "room", Room: i + 1})
		recordRunBeat("run-b", peteclient.RunBeat{Kind: "room", Room: i + 1})
	}

	for _, run := range []string{"run-a", "run-b"} {
		rows, err := db.Get().Query(
			`SELECT seq FROM pete_run_beat WHERE run_id = ? ORDER BY seq ASC`, run)
		if err != nil {
			t.Fatal(err)
		}
		var seqs []int64
		for rows.Next() {
			var s int64
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			seqs = append(seqs, s)
		}
		rows.Close()
		if len(seqs) != 3 {
			t.Fatalf("%s: %d beats, want 3", run, len(seqs))
		}
		for i, s := range seqs {
			if s != int64(i+1) {
				t.Errorf("%s: seq[%d] = %d, want %d", run, i, s, i+1)
			}
		}
	}
}

// TestRunBeatsAreDroppedForOptedOutPlayers is the privacy guard, and it is
// stricter than the board's.
//
// The board omits an opted-out player from a snapshot. The liveblog would be a
// room-by-room account of where somebody is and what is happening to them, which
// is the most exposing surface in the whole plan — so the rule here is that the
// beats never leave the box at all. They are retired locally instead, so the
// buffer can't fill up with rows that will never ship.
//
// It is also the pin on the connection-pool deadlock this originally shipped
// with: resolving an owner requires a second query, and doing that with the beat
// cursor still open waits forever on a one-connection pool. If loadRunBeatBatch
// ever goes back to resolving inside its own rows loop, this test stops failing
// and starts HANGING — which is what it did the first time, and is why the note
// is here rather than in a comment nobody reads at 3am.
func TestRunBeatsAreDroppedForOptedOutPlayers(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@shy:test", "Quack", &now, &now)
	seedRosterPlayer(t, "@loud:test", "Josie", &now, &now)
	seedBeatRun(t, "run-shy", "@shy:test")
	seedBeatRun(t, "run-loud", "@loud:test")
	setNewsOptout("@shy:test", true)

	recordRunBeat("run-shy", peteclient.RunBeat{Kind: "room", Room: 2})
	recordRunBeat("run-loud", peteclient.RunBeat{Kind: "room", Room: 2})

	send, drop, err := loadRunBeatBatch(100)
	if err != nil {
		t.Fatalf("loadRunBeatBatch: %v", err)
	}
	if len(send) != 1 || send[0].RunID != "run-loud" {
		t.Fatalf("sendable beats = %+v, want only run-loud", send)
	}
	if len(drop) != 1 || drop[0].RunID != "run-shy" {
		t.Fatalf("dropped beats = %+v, want only run-shy", drop)
	}

	// Retiring means marked sent, not deleted — one code path for "done with this
	// row", and the retention sweep reaps it on its own clock.
	if err := markRunBeatsSent(drop); err != nil {
		t.Fatalf("retire: %v", err)
	}
	send, drop, _ = loadRunBeatBatch(100)
	if len(drop) != 0 {
		t.Errorf("retired beats came back: %+v", drop)
	}
	if len(send) != 1 {
		t.Errorf("retiring the opted-out beats disturbed the rest: %+v", send)
	}
}

// TestRunBeatsRefuseAnUnresolvableRun. The liveblog is public. A run whose owner
// can't be resolved is not a run we know is safe to publish — "don't know who
// this is" is not a basis for saying where they are.
func TestRunBeatsRefuseAnUnresolvableRun(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)

	recordRunBeat("run-ghost", peteclient.RunBeat{Kind: "room", Room: 1})

	send, drop, err := loadRunBeatBatch(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(send) != 0 {
		t.Errorf("beats for an unknown run were queued for publication: %+v", send)
	}
	if len(drop) != 1 {
		t.Errorf("orphan beats = %d, want 1 retired", len(drop))
	}
}

// TestRunEndIsFirstWriterWins. A run ends once, but it passes through more than
// one place that can say so: a death in the combat resolver goes on to call
// abandonZoneRun, and the expedition layer retires completed runs. The specific
// outcome is filed first and must survive the generic one behind it — a log that
// says "abandoned" about somebody who was killed is worse than no log.
func TestRunEndIsFirstWriterWins(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@a:test", "Josie", &now, &now)
	seedBeatRun(t, "run-a", "@a:test")
	run, err := getZoneRun("run-a")
	if err != nil || run == nil {
		t.Fatalf("load run: %v", err)
	}

	beatRunEnd(run, "died")
	beatRunEnd(run, "abandoned")
	beatRunEnd(run, "cleared")

	if kinds := beatKinds(t, "run-a"); len(kinds) != 1 || kinds[0] != "end" {
		t.Fatalf("beats = %v, want exactly one end beat", kinds)
	}
	send, _, err := loadRunBeatBatch(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(send) != 1 || send[0].Outcome != "died" {
		t.Errorf("stored outcome = %+v, want the first (specific) close", send)
	}
}

// TestRunBeatsAreANoOpWhenTheSeamIsOff. The whole channel hangs off the same
// master switch as the dispatch queue: with news emission off, nothing is
// recorded at all, so turning it off doesn't quietly accrue a buffer that floods
// Pete the moment it comes back on.
func TestRunBeatsAreANoOpWhenTheSeamIsOff(t *testing.T) {
	newBoredomTestDB(t)

	recordRunBeat("run-a", peteclient.RunBeat{Kind: "room", Room: 1})
	if kinds := beatKinds(t, "run-a"); len(kinds) != 0 {
		t.Errorf("recorded %v with the seam disabled", kinds)
	}
}

// TestBeatCombatReadsTheOutcome pins the three fight endings apart. "Outlasted
// by the monster" and "killed by the monster" are the same losing branch in the
// engine and mechanically different events — one starts a respawn timer and the
// other doesn't — so the log must not collapse them.
func TestBeatCombatReadsTheOutcome(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@a:test", "Josie", &now, &now)
	seedBeatRun(t, "run-a", "@a:test")
	run, _ := getZoneRun("run-a")

	beatCombat(run, "Rat", false, false, true, false, 30, 24, 30, 1, 0)
	beatCombat(run, "Aldric", false, true, false, true, 24, 8, 30, 0, 2)
	beatCombat(run, "The Rotmother", false, true, false, false, 8, 0, 30, 0, 0)

	send, _, err := loadRunBeatBatch(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(send) != 3 {
		t.Fatalf("got %d combat beats, want 3", len(send))
	}
	want := []string{"won", "retreat", "down"}
	for i, w := range want {
		if send[i].Outcome != w {
			t.Errorf("beat %d outcome = %q, want %q", i, send[i].Outcome, w)
		}
	}
	if send[0].Amount != 6 || send[0].HP != 24 || send[0].HPMax != 30 {
		t.Errorf("won beat lost its numbers: %+v", send[0])
	}
	if send[0].Crits != 1 {
		t.Errorf("crits = %d, want 1", send[0].Crits)
	}
	if send[1].RoomKind != "boss" {
		t.Errorf("room kind = %q, want boss", send[1].RoomKind)
	}
}

// TestBeatCombatNeverReportsNegativeDamage. A party that healed through a fight
// finishes on more HP than it started with. "Took −4 damage" is not a fact.
func TestBeatCombatNeverReportsNegativeDamage(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@a:test", "Josie", &now, &now)
	seedBeatRun(t, "run-a", "@a:test")
	run, _ := getZoneRun("run-a")

	beatCombat(run, "Rat", false, false, true, false, 20, 28, 30, 0, 0)

	send, _, _ := loadRunBeatBatch(10)
	if len(send) != 1 || send[0].Amount != 0 {
		t.Fatalf("amount = %+v, want 0", send)
	}
}

// TestBeatHaulPicksTheBiggestYieldDeterministically. Go's map order is random,
// so a "mostly X" line built off a range would name a different resource every
// time the same room was rendered.
func TestBeatHaulPicksTheBiggestYieldDeterministically(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@a:test", "Josie", &now, &now)
	for i, runID := range []string{"run-1", "run-2", "run-3", "run-4", "run-5"} {
		seedBeatRun(t, runID, "@a:test")
		run, _ := getZoneRun(runID)
		beatHaul(run, autoHarvestSummary{
			Yields: map[string]int{"ironcap": 5, "moss": 2, "flint": 1},
			Names:  map[string]string{"ironcap": "Ironcap", "moss": "Moss", "flint": "Flint"},
		})
		_ = i
	}
	send, _, err := loadRunBeatBatch(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(send) != 5 {
		t.Fatalf("got %d haul beats, want 5", len(send))
	}
	for _, b := range send {
		if b.Target != "Ironcap" {
			t.Fatalf("haul named %q, want Ironcap every time", b.Target)
		}
		if b.Amount != 8 || b.Count != 3 {
			t.Errorf("haul totals = %d over %d kinds, want 8 over 3", b.Amount, b.Count)
		}
	}

	// Nothing gathered is not a beat.
	seedBeatRun(t, "run-empty", "@a:test")
	empty, _ := getZoneRun("run-empty")
	beatHaul(empty, autoHarvestSummary{})
	if kinds := beatKinds(t, "run-empty"); len(kinds) != 0 {
		t.Errorf("an empty haul produced %v", kinds)
	}
}

// TestStartingARunOpensItsLog is the end-to-end seam check on the game side: the
// engine primitive every zone entry goes through files the one beat that carries
// identity, so nothing downstream has to be told who is walking.
func TestStartingARunOpensItsLog(t *testing.T) {
	newBoredomTestDB(t)
	enablePeteSeam(t)
	now := time.Now().UTC()

	seedRosterPlayer(t, "@a:test", "Josie", &now, &now)
	run, err := startZoneRun("@a:test", "goblin_warrens", 5, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	send, _, err := loadRunBeatBatch(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(send) != 1 || send[0].Kind != "start" {
		t.Fatalf("beats = %+v, want one start", send)
	}
	b := send[0]
	if b.RunID != run.RunID {
		t.Errorf("start beat run = %q, want %q", b.RunID, run.RunID)
	}
	if b.Name != "Josie" || b.Level != 5 {
		t.Errorf("start beat identity = %q L%d, want Josie L5", b.Name, b.Level)
	}
	if b.Token == "" || b.Token != eventToken("@a:test", "roster") {
		t.Errorf("start beat token = %q, want the public board token", b.Token)
	}
	if b.TotalRooms != run.TotalRooms {
		t.Errorf("start beat rooms = %d, want %d", b.TotalRooms, run.TotalRooms)
	}
}
