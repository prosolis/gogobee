package plugin

import (
	"math/rand/v2"
	"strings"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// TestArchiveOrphanZoneRuns_LeavesLinkedRunsAlone verifies that a run
// pointed at by an active expedition.run_id survives the migration.
func TestArchiveOrphanZoneRuns_LeavesLinkedRunsAlone(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@r1-link-test:example.org")
	defer cleanupZoneRuns(uid)
	defer cleanupExpeditions(uid)

	rng := rand.New(rand.NewPCG(11, 7))
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, rng)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, run.RunID, ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1})
	if err != nil {
		t.Fatalf("startExpedition: %v", err)
	}
	_ = exp

	if _, err := archiveOrphanZoneRuns(); err != nil {
		t.Fatalf("archive: %v", err)
	}

	got, err := getZoneRun(run.RunID)
	if err != nil {
		t.Fatalf("getZoneRun: %v", err)
	}
	if got == nil || !got.IsActive() {
		t.Fatalf("linked run got archived; abandoned=%v", got != nil && !got.IsActive())
	}
}

// TestArchiveOrphanZoneRuns_RegionRunsPreserved verifies that runs only
// referenced via RegionState["region_runs"] (not exp.run_id) are kept.
func TestArchiveOrphanZoneRuns_RegionRunsPreserved(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@r1-region-test:example.org")
	defer cleanupZoneRuns(uid)
	defer cleanupExpeditions(uid)

	rng := rand.New(rand.NewPCG(13, 8))
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, rng)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{Max: 5, Current: 5, DailyBurn: 1})
	if err != nil {
		t.Fatalf("startExpedition: %v", err)
	}
	// Pin via region_runs only; exp.run_id stays empty.
	regionJSON := `{"region_runs":{"r1":"` + run.RunID + `"}}`
	if _, err := db.Get().Exec(`UPDATE dnd_expedition SET region_state = ? WHERE expedition_id = ?`, regionJSON, exp.ID); err != nil {
		t.Fatalf("set region_state: %v", err)
	}

	if _, err := archiveOrphanZoneRuns(); err != nil {
		t.Fatalf("archive: %v", err)
	}

	got, err := getZoneRun(run.RunID)
	if err != nil {
		t.Fatalf("getZoneRun: %v", err)
	}
	if got == nil || !got.IsActive() {
		t.Fatalf("region-linked run got archived")
	}
}

// TestArchiveOrphanZoneRuns_OrphanArchived verifies that an active zone
// run with no expedition pointing at it gets flagged abandoned.
func TestArchiveOrphanZoneRuns_OrphanArchived(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@r1-orphan-test:example.org")
	defer cleanupZoneRuns(uid)

	rng := rand.New(rand.NewPCG(17, 9))
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, rng)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}

	n, err := archiveOrphanZoneRuns()
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected ≥1 archived row, got %d", n)
	}

	got, err := getZoneRun(run.RunID)
	if err != nil {
		t.Fatalf("getZoneRun: %v", err)
	}
	if got == nil {
		t.Fatal("run vanished")
	}
	if got.IsActive() {
		t.Fatal("orphan run still active after archive")
	}
}

// TestArchiveOrphanZoneRuns_Idempotent: a second call is a no-op.
func TestArchiveOrphanZoneRuns_Idempotent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@r1-idempotent-test:example.org")
	defer cleanupZoneRuns(uid)

	rng := rand.New(rand.NewPCG(19, 11))
	if _, err := startZoneRun(uid, ZoneGoblinWarrens, 1, rng); err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	if _, err := archiveOrphanZoneRuns(); err != nil {
		t.Fatalf("archive #1: %v", err)
	}
	n, err := archiveOrphanZoneRuns()
	if err != nil {
		t.Fatalf("archive #2: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 on second call, got %d", n)
	}
}

// TestIsLegacyActivityInput covers the deprecation-gate detection: legacy
// keywords / numbers match, unrelated DM input doesn't.
func TestIsLegacyActivityInput(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1", true},
		{"2 deep mine", true},
		{"dungeon", true},
		{"forage", true},
		{"fish", true},
		{"f", true},
		{"Soggy Cellar", true}, // location name match
		{"", false},
		{"hello there", false},
		{"7", false},
		{"!expedition", false},
	}
	for _, c := range cases {
		got := isLegacyActivityInput(strings.ToLower(c.in), nil)
		if got != c.want {
			t.Errorf("isLegacyActivityInput(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestRenderLegacyActivityDeprecation: deprecation DM points at !expedition
// and the harvest entry-points so the user has somewhere to go.
func TestRenderLegacyActivityDeprecation(t *testing.T) {
	out := renderLegacyActivityDeprecation(nil)
	for _, want := range []string{"expedition", "!forage", "!mine", "retired"} {
		if !strings.Contains(out, want) {
			t.Errorf("deprecation DM missing %q; got:\n%s", want, out)
		}
	}
}
