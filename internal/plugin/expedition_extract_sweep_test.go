package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// seedExtraction inserts an 'extracting' row whose window closed `age` ago.
// A negative age is a live extraction.
func seedExtraction(t *testing.T, expeditionID string, owner id.UserID, age time.Duration) {
	t.Helper()
	seedExpedition(t, expeditionID, owner, ExpeditionStatusExtracting)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET completed_at = ? WHERE expedition_id = ?`,
		time.Now().UTC().Add(-extractResumeWindow-age), expeditionID); err != nil {
		t.Fatal(err)
	}
}

func seatedOn(t *testing.T, member id.UserID) string {
	t.Helper()
	e, err := seatedExpeditionFor(member)
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		return ""
	}
	return e.ID
}

// The window is the only thing bounding how long a leader's roster holds their
// party. Nothing else flips an 'extracting' row terminal unless the leader
// personally types !resume, so if the sweep misses, members are stuck forever.
func TestSweepLapsedExtractions_ReleasesTheRosterItStranded(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@sweep-leader:example.org")
	member := id.UserID("@sweep-member:example.org")

	seedExtraction(t, "exp-lapsed", leader, time.Hour)
	if err := joinParty("exp-lapsed", member); err != nil {
		t.Fatal(err)
	}
	if got := seatedOn(t, member); got != "exp-lapsed" {
		t.Fatalf("member seated on %q before sweep, want exp-lapsed", got)
	}

	(&AdventurePlugin{}).sweepLapsedExtractions(time.Now().UTC())

	e, err := getExpedition("exp-lapsed")
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != ExpeditionStatusFailed {
		t.Errorf("status = %q, want failed", e.Status)
	}
	if got := seatedOn(t, member); got != "" {
		t.Errorf("member still seated on %q after sweep", got)
	}
	if n, _ := partySize("exp-lapsed"); n != 1 {
		t.Errorf("roster survived the sweep: partySize = %d", n)
	}
}

// The mirror: a leader who extracted an hour ago still has six days to !resume,
// and their party must still be there when they do.
func TestSweepLapsedExtractions_LeavesALiveExtractionAlone(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@sweep-live-leader:example.org")
	member := id.UserID("@sweep-live-member:example.org")

	seedExtraction(t, "exp-live", leader, -6*24*time.Hour) // one day in
	if err := joinParty("exp-live", member); err != nil {
		t.Fatal(err)
	}

	(&AdventurePlugin{}).sweepLapsedExtractions(time.Now().UTC())

	e, _ := getExpedition("exp-live")
	if e.Status != ExpeditionStatusExtracting {
		t.Errorf("status = %q, want extracting — the resume window is still open", e.Status)
	}
	if got := seatedOn(t, member); got != "exp-live" {
		t.Errorf("member unseated from a resumable expedition (seated on %q)", got)
	}
}

// completed_at is the only clock the window has. A row without one can never
// lapse, so it must read as already lapsed rather than as immortal.
func TestExtractionLapsed_NullCompletedAtIsLapsed(t *testing.T) {
	now := time.Now().UTC()
	if !extractionLapsed(&Expedition{}, now) {
		t.Error("nil completed_at should read as lapsed")
	}
	fresh := now.Add(-time.Hour)
	if extractionLapsed(&Expedition{CompletedAt: &fresh}, now) {
		t.Error("an hour-old extraction is not lapsed")
	}
	old := now.Add(-extractResumeWindow - time.Second)
	if !extractionLapsed(&Expedition{CompletedAt: &old}, now) {
		t.Error("a row past the window is lapsed")
	}
}

// A leader must be able to let an extracted expedition go without paying to
// !resume it first — that is the escape hatch !expedition start points them at.
func TestAbandonExpedition_ReachesAnExtractedRowAndFreesTheParty(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@abandon-extracted-leader:example.org")
	member := id.UserID("@abandon-extracted-member:example.org")

	seedExtraction(t, "exp-extracted", leader, -24*time.Hour) // live, resumable
	if err := joinParty("exp-extracted", member); err != nil {
		t.Fatal(err)
	}

	if err := abandonExpedition(leader); err != nil {
		t.Fatalf("abandonExpedition on an extracting row: %v", err)
	}
	e, _ := getExpedition("exp-extracted")
	if e.Status != ExpeditionStatusAbandoned {
		t.Errorf("status = %q, want abandoned", e.Status)
	}
	if got := seatedOn(t, member); got != "" {
		t.Errorf("member still seated on %q", got)
	}
}

// expeditionCmdStart's run-spawn rollback calls abandonExpedition to tear down
// the row it just created. A leader who also owns an older extracted row must
// not have that one reaped instead.
func TestAbandonExpedition_PrefersTheActiveRowOverAnExtractedOne(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@abandon-prefers-active:example.org")

	seedExtraction(t, "exp-old-extracted", leader, -24*time.Hour)
	seedExpedition(t, "exp-new-active", leader, ExpeditionStatusActive)

	if err := abandonExpedition(leader); err != nil {
		t.Fatal(err)
	}
	if e, _ := getExpedition("exp-new-active"); e.Status != ExpeditionStatusAbandoned {
		t.Errorf("active row status = %q, want abandoned", e.Status)
	}
	if e, _ := getExpedition("exp-old-extracted"); e.Status != ExpeditionStatusExtracting {
		t.Errorf("extracted row was collateral damage: status = %q", e.Status)
	}
}

func TestAbandonExpedition_NoLiveRow(t *testing.T) {
	setupEmptyTestDB(t)
	if err := abandonExpedition(id.UserID("@abandon-nothing:example.org")); err != ErrNoActiveExpedition {
		t.Errorf("err = %v, want ErrNoActiveExpedition", err)
	}
}
