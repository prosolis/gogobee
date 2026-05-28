package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase-R7 hardening tests covering the audit gaps: 24h auto-abandon,
// briefing idempotency, threat-70 warning idempotency, multi-region
// extract→resume, and starvation→forced extraction.

// ── 24-hour zone-run auto-abandon (§4.3) ────────────────────────────────────

func TestZoneRun_AutoAbandonsAfter24h(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@stale-run:example")
	defer cleanupExpeditions(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	// Backdate last_action_at to 25 hours ago.
	if _, err := dbExecZoneRunBackdate(run.RunID, 25*time.Hour); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	got, err := getActiveZoneRun(uid)
	if err != nil {
		t.Fatalf("getActiveZoneRun: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after 24h timeout, got run %q", got.RunID)
	}
	// And the row is marked abandoned.
	raw, _ := getZoneRun(run.RunID)
	if raw == nil || !raw.Abandoned {
		t.Errorf("run not abandoned: %+v", raw)
	}
}

func TestZoneRun_FreshRunNotAutoAbandoned(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@fresh-run:example")
	defer cleanupExpeditions(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := getActiveZoneRun(uid)
	if err != nil || got == nil {
		t.Fatalf("fresh run dropped: got=%v err=%v", got, err)
	}
	if got.RunID != run.RunID {
		t.Errorf("returned wrong run: %q vs %q", got.RunID, run.RunID)
	}
}

// ── Briefing idempotency: double-fire same day is a no-op ───────────────────

func TestDeliverBriefing_DoubleFireIsIdempotent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@brief-idem:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Backdate start so the briefing precondition (start_date < threshold) passes.
	if _, err := dbExecExpeditionBackdate(exp.ID, 24*time.Hour); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	p := &AdventurePlugin{}
	now := time.Date(2026, 5, 8, 6, 0, 0, 0, time.UTC)

	if err := p.deliverBriefing(exp, now); err != nil {
		t.Fatalf("first deliver: %v", err)
	}
	first, _ := getExpedition(exp.ID)
	day1 := first.CurrentDay
	supplies1 := first.Supplies.Current

	// Re-fetch as a fresh copy (simulates a second tick reading the row
	// before its claimed last_briefing_at lands in the loader filter).
	exp2, _ := getExpedition(exp.ID)
	exp2.Supplies = first.Supplies // pretend stale snapshot
	if err := p.deliverBriefing(exp2, now); err != nil {
		t.Fatalf("second deliver: %v", err)
	}
	second, _ := getExpedition(exp.ID)
	if second.CurrentDay != day1 {
		t.Errorf("day double-bumped: %d → %d", day1, second.CurrentDay)
	}
	if second.Supplies.Current != supplies1 {
		t.Errorf("supplies double-burned: %.1f → %.1f", supplies1, second.Supplies.Current)
	}
}

// ── Threat-70 warning fires once across multiple crossings ──────────────────

func TestApplyDailyThreatDrift_70WarningOnceOnly(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@threat-70-once:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 65, "seed"); err != nil {
		t.Fatal(err)
	}

	// Cross 70 the first time.
	// Push over 70 the first time.
	if err := applyThreatDelta(exp.ID, 6, "drift"); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	if _, _, err := applyDailyThreatDrift(exp); err != nil {
		t.Fatalf("first drift: %v", err)
	}

	// Drop below 70 then bump back over.
	if err := applyThreatDelta(exp.ID, -15, "fortified rest"); err != nil {
		t.Fatal(err)
	}
	if err := applyThreatDelta(exp.ID, 10, "loud combat"); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	if _, _, err := applyDailyThreatDrift(exp); err != nil {
		t.Fatalf("second drift: %v", err)
	}

	rows, err := recentExpeditionLog(exp.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	warnings := 0
	for _, r := range rows {
		if r.Type == "threat" && strings.Contains(strings.ToLower(r.Summary), "siege approaches") {
			warnings++
		}
	}
	if warnings > 1 {
		t.Errorf("threat-70 warning fired %d times across drift loop; want at most 1", warnings)
	}
}

// ── Starvation triggers forced extraction at briefing time ──────────────────

func TestDeliverBriefing_StarvationForcesExtraction(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@starve-extract:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 0.5, Max: 10, DailyBurn: 5, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	rewindToLegacyAnchor(t, exp)

	p := &AdventurePlugin{}
	now := time.Date(2026, 5, 8, 6, 0, 0, 0, time.UTC)
	if err := p.deliverBriefing(exp, now); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	got, _ := getExpedition(exp.ID)
	if got.Status != ExpeditionStatusAbandoned {
		t.Errorf("status = %q after starvation briefing, want abandoned", got.Status)
	}
}

// ── Multi-region extract→resume preserves region state ──────────────────────

func TestExtractResume_MultiRegionPreservesState(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@multiregion-resume:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "underdark_surface_tunnels",
		ExpeditionSupplies{Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Visit a second region and mark a region-boss kill.
	if _, err := MarkRegionVisited(exp, "underdark_drow_outpost"); err != nil {
		t.Fatalf("mark visited: %v", err)
	}
	if _, err := MarkRegionBossDefeated(exp, "underdark_drow_outpost"); err != nil {
		t.Fatalf("mark boss: %v", err)
	}

	// Extract.
	if _, err := voluntaryExtractExpedition(uid); err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Resume with fresh supplies.
	resumed, err := getResumableExpedition(uid)
	if err != nil || resumed == nil {
		t.Fatalf("getResumable: %v %v", resumed, err)
	}
	if err := resumeExpedition(resumed.ID, ExpeditionSupplies{
		Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1,
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// Re-load — region state must survive.
	live, _ := getActiveExpedition(uid)
	if live == nil {
		t.Fatal("post-resume: no active expedition")
	}
	if !IsRegionVisited(live, "underdark_drow_outpost") {
		t.Error("visited region lost on resume")
	}
	if !IsRegionCleared(live, "underdark_drow_outpost") {
		t.Error("region-cleared flag lost on resume")
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func dbExecZoneRunBackdate(runID string, ago time.Duration) (int64, error) {
	when := time.Now().UTC().Add(-ago)
	r, err := db.Get().Exec(
		`UPDATE dnd_zone_run SET last_action_at = ? WHERE run_id = ?`, when, runID)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

func dbExecExpeditionBackdate(expID string, ago time.Duration) (int64, error) {
	when := time.Now().UTC().Add(-ago)
	r, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = ?`, when, expID)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}
