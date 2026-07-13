package plugin

import (
	"os"
	"regexp"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// pickBoredomZone reads expedition history to break ties, so it needs a DB.
func newBoredomTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// TestAdvActionCommandsCoverDispatch pins the boredom idle clock to the actual
// command router.
//
// advActionCommands has to list every command AdventurePlugin.OnMessage routes,
// because a command missing from the set is a command the player can use
// without their adventurer noticing they showed up — so it sits there idle,
// gets "bored", and walks into a dungeon while they're actively playing.
//
// Commands() can't be used as the source of truth: it registers 28 names for
// the help surface and omits expedition, zone, fight, camp, extract and resume.
// So the router is the source of truth, and this greps it.
func TestAdvActionCommandsCoverDispatch(t *testing.T) {
	src, err := os.ReadFile("adventure.go")
	if err != nil {
		t.Fatalf("read adventure.go: %v", err)
	}
	re := regexp.MustCompile(`p\.IsCommand\(ctx\.Body, "([a-z0-9-]+)"\)`)
	matches := re.FindAllSubmatch(src, -1)
	if len(matches) < 40 {
		t.Fatalf("only found %d IsCommand call sites in adventure.go — the regex has "+
			"drifted from the dispatcher and this test is no longer guarding anything", len(matches))
	}
	for _, m := range matches {
		name := string(m[1])
		if !advActionCommands[name] {
			t.Errorf("!%s is routed by OnMessage but missing from advActionCommands — "+
				"players using it would not reset their boredom clock", name)
		}
	}
}

// TestPickBoredomZoneBands walks the level bands and asserts the picker's
// rules: easiest at-level, no raid zones while an alternative exists, and the
// L13+ fallback into the raid.
func TestPickBoredomZoneBands(t *testing.T) {
	newBoredomTestDB(t)
	cases := []struct {
		level int
		want  ZoneID
		why   string
	}{
		{level: 1, want: ZoneGoblinWarrens, why: "T1 band, tie broken by never-visited order"},
		{level: 5, want: ZoneForestShadows, why: "L5 is in both the T2 and T3 bands — take the easier"},
		{level: 10, want: ZoneUnderdark, why: "T4 band; underdark is multi-region but NOT a raid, so it stays eligible. Both T4 zones are unvisited here, so the tie falls through to registration order"},
		{level: 15, want: ZoneDragonsLair, why: "only raid zones in band at L15 — fallback fires, lowest tier first"},
	}
	for _, tc := range cases {
		got, ok := pickBoredomZone(id.UserID("@nobody:test"), tc.level)
		if !ok {
			t.Errorf("L%d: no zone picked (%s)", tc.level, tc.why)
			continue
		}
		if got.ID != tc.want {
			t.Errorf("L%d: picked %s, want %s (%s)", tc.level, got.ID, tc.want, tc.why)
		}
	}
}

// TestPickBoredomZoneAvoidsRaidsWhenPossible is the rule the player actually
// asked for, stated directly: below the endgame, a bored adventurer never gets
// sent into party-tuned content.
func TestPickBoredomZoneAvoidsRaidsWhenPossible(t *testing.T) {
	newBoredomTestDB(t)
	for level := 1; level <= 12; level++ {
		z, ok := pickBoredomZone(id.UserID("@nobody:test"), level)
		if !ok {
			continue
		}
		if raidContentWarning(z.ID) != "" {
			t.Errorf("L%d: picked raid zone %s while a non-raid zone was in band", level, z.ID)
		}
	}
}
