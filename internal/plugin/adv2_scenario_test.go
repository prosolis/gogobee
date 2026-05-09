package plugin

import (
	"math/rand/v2"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Adv 2.0 scenario playthrough: drives a realistic player session against
// a copy of the prod DB, verifying happy-path behavior end-to-end across
// the !zone state machine, the !expedition multi-day system, and the
// !harvest economy. SendDM is a no-op without a Matrix client, so the
// asserts target persisted state. Logs are verbose (t.Logf) so the run
// reads like a transcript.

// ── Scenario A — single-session zone run ────────────────────────────────────

func TestAdv2Scenario_ZoneRunGoblinWarrens(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@adv2-scenA:example")
	t.Cleanup(func() { cleanupZoneRuns(uid); cleanupExpeditions(uid) })

	// Beefy L5 fighter so combat doesn't trivially TPK (still RNG, but 5
	// is well past goblin warrens' L1–L3 band).
	if err := createAdvCharacter(uid, "scenA"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
		HPMax: 60, HPCurrent: 60, ArmorClass: 18,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 500, "scenA bankroll")
	p := &AdventurePlugin{euro: euro}

	// !zone (list) — should not error, no run created.
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, ""); err != nil {
		t.Fatalf("zone list: %v", err)
	}
	if got, _ := getActiveZoneRun(uid); got != nil {
		t.Fatal("zone list should not create a run")
	}

	// !zone enter goblin_warrens
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatalf("zone enter: %v", err)
	}
	run, err := getActiveZoneRun(uid)
	if err != nil || run == nil {
		t.Fatalf("expected active run after enter: %v", err)
	}
	t.Logf("entered %s — %d rooms, mood=%d, first room=%s",
		run.ZoneID, run.TotalRooms, run.DMMood, run.CurrentRoomType())
	if run.DMMood != 50 {
		t.Errorf("starting mood = %d, want 50", run.DMMood)
	}
	if run.CurrentRoom != 0 || run.CurrentRoomType() != RoomEntry {
		t.Errorf("first room not entry: idx=%d type=%s", run.CurrentRoom, run.CurrentRoomType())
	}

	// !zone status (cheap smoke)
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "status"); err != nil {
		t.Fatalf("zone status: %v", err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "map"); err != nil {
		t.Fatalf("zone map: %v", err)
	}

	// Drive !zone advance until the run terminates (cleared, died, or
	// abandoned). Cap iterations as a safety net in case advance becomes
	// a no-op due to a regression.
	maxSteps := run.TotalRooms + 4
	clearedRoomTypes := []RoomType{}
	for step := 0; step < maxSteps; step++ {
		before, _ := getActiveZoneRun(uid)
		if before == nil {
			t.Logf("step %d: run terminated (cleared/died/abandoned)", step)
			break
		}
		prevType := before.CurrentRoomType()
		prevHP := c.HPCurrent
		if cur, _ := LoadDnDCharacter(uid); cur != nil {
			prevHP = cur.HPCurrent
		}
		t.Logf("step %d: in room %d/%d (%s), mood=%d, HP=%d",
			step, before.CurrentRoom+1, before.TotalRooms, prevType, before.DMMood, prevHP)
		if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "advance"); err != nil {
			t.Fatalf("zone advance step %d: %v", step, err)
		}
		clearedRoomTypes = append(clearedRoomTypes, prevType)
	}
	final, _ := getActiveZoneRun(uid)
	raw := getZoneRunByUser(t, uid) // helper below — most-recent row
	if final != nil {
		t.Logf("final state: still active at room %d/%d — likely advance regression",
			final.CurrentRoom+1, final.TotalRooms)
		t.Errorf("run did not terminate within %d steps", maxSteps)
	} else if raw != nil {
		boss := raw.BossDefeated
		t.Logf("run %s ended: bossDefeated=%v abandoned=%v rooms_cleared=%d/%d loot=%d mood=%d",
			raw.RunID, boss, raw.Abandoned, len(raw.RoomsCleared), raw.TotalRooms,
			len(raw.LootCollected), raw.DMMood)
		t.Logf("clearedRoomTypes: %v", clearedRoomTypes)
		if !boss && !raw.Abandoned && raw.CompletedAt == nil {
			t.Errorf("run row in indeterminate state: %+v", raw)
		}
	}
}

// getZoneRunByUser fetches the most-recent zone-run row for a user
// regardless of active/abandoned/cleared. Helper for the scenario test.
func getZoneRunByUser(t *testing.T, uid id.UserID) *DungeonRun {
	t.Helper()
	row := db.Get().QueryRow(`
		SELECT run_id FROM dnd_zone_run
		WHERE user_id = ?
		ORDER BY started_at DESC LIMIT 1
	`, string(uid))
	var runID string
	if err := row.Scan(&runID); err != nil {
		return nil
	}
	r, _ := getZoneRun(runID)
	return r
}

// ── Scenario B — multi-day expedition ───────────────────────────────────────

func TestAdv2Scenario_ExpeditionCryptValdris(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@adv2-scenB:example")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })

	if err := createAdvCharacter(uid, "scenB"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassRanger, Level: 5,
		STR: 14, DEX: 18, CON: 14, INT: 10, WIS: 14, CHA: 10,
		HPMax: 50, HPCurrent: 50, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 1000, "scenB bankroll")
	p := &AdventurePlugin{euro: euro}

	// !expedition list
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "list"); err != nil {
		t.Fatalf("expedition list: %v", err)
	}

	// !expedition start crypt_valdris with 2 standard packs
	balBefore := euro.GetBalance(uid)
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start crypt_valdris 2s"); err != nil {
		t.Fatalf("expedition start: %v", err)
	}
	exp, err := getActiveExpedition(uid)
	if err != nil || exp == nil {
		t.Fatalf("expected active expedition: %v", err)
	}
	balAfter := euro.GetBalance(uid)
	t.Logf("expedition %s started: zone=%s supplies=%d/%d threat=%d cost=%.0f→%.0f",
		exp.ID, exp.ZoneID, int(exp.Supplies.Current), int(exp.Supplies.Max),
		exp.ThreatLevel, balBefore, balAfter)
	if exp.ZoneID != ZoneCryptValdris {
		t.Errorf("zone = %s, want crypt_valdris", exp.ZoneID)
	}
	if balAfter >= balBefore {
		t.Errorf("expected outfitting to debit coins (%.2f → %.2f)", balBefore, balAfter)
	}

	// Backdate start so deliverBriefing's same-day guard passes.
	if _, err := dbExecExpeditionBackdate(exp.ID, 24*time.Hour); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Drive 3 daily briefings — verify supply burn + day advance.
	now := time.Date(2026, 5, 8, 6, 0, 0, 0, time.UTC)
	for day := 1; day <= 3; day++ {
		fresh, _ := getExpedition(exp.ID)
		if fresh == nil {
			t.Logf("expedition ended before day %d", day)
			break
		}
		preDay := fresh.CurrentDay
		preSupplies := fresh.Supplies.Current
		preThreat := fresh.ThreatLevel
		if err := p.deliverBriefing(fresh, now); err != nil {
			t.Fatalf("deliverBriefing day %d: %v", day, err)
		}
		// Advance the wallclock + roll the start back another 24h so the
		// next briefing's day-guard fires.
		now = now.Add(24 * time.Hour)
		if _, err := dbExecExpeditionBackdate(exp.ID, 24*time.Hour); err != nil {
			t.Fatalf("backdate day %d: %v", day, err)
		}
		post, _ := getExpedition(exp.ID)
		t.Logf("day %d: day=%d→%d supplies=%v→%v threat=%d→%d",
			day, preDay, post.CurrentDay, preSupplies, post.Supplies.Current, preThreat, post.ThreatLevel)
		if post.CurrentDay <= preDay {
			t.Errorf("day did not advance: %d → %d", preDay, post.CurrentDay)
		}
	}

	// !expedition status / log
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "status"); err != nil {
		t.Fatalf("expedition status: %v", err)
	}
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "log"); err != nil {
		t.Fatalf("expedition log: %v", err)
	}

	entries, _ := recentExpeditionLog(exp.ID, 10)
	t.Logf("expedition log has %d entries", len(entries))
	for i, e := range entries {
		t.Logf("  [%d] day=%d type=%s summary=%q", i, e.Day, e.Type, e.Summary)
	}
	if len(entries) == 0 {
		t.Error("expected at least one log entry")
	}

	// !expedition abandon — clean state for next test runs.
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "abandon"); err != nil {
		t.Fatalf("expedition abandon: %v", err)
	}
	if got, _ := getActiveExpedition(uid); got != nil {
		t.Error("expected no active expedition after abandon")
	}
}

// ── Scenario C — harvest in an active expedition ────────────────────────────

func TestAdv2Scenario_HarvestForestShadows(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@adv2-scenC:example")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })

	if err := createAdvCharacter(uid, "scenC"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassRanger, Level: 4,
		STR: 12, DEX: 18, CON: 14, INT: 10, WIS: 16, CHA: 10,
		HPMax: 40, HPCurrent: 40, ArmorClass: 15,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatalf("SaveDnDCharacter: %v", err)
	}

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 500, "scenC bankroll")
	p := &AdventurePlugin{euro: euro}

	// Forage-friendly expedition.
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start forest_shadows"); err != nil {
		t.Fatalf("expedition start: %v", err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp == nil {
		t.Fatal("no expedition")
	}

	// Drive a region run so harvest has a current room context.
	run, err := ensureRegionRun(exp, c.Level)
	if err != nil {
		t.Fatalf("ensureRegionRun: %v", err)
	}
	t.Logf("region run %s in zone %s (%d rooms)", run.RunID, run.ZoneID, run.TotalRooms)

	// Seed harvest nodes for the entry room (they're auto-seeded by
	// loadHarvestNodes on first read, but confirm we get at least one).
	roomIdx := currentRoomIndexFor(exp)
	nodeID := currentNodeIDFor(exp)
	nodes := loadHarvestNodes(exp, nodeID)
	t.Logf("room %d has %d harvest nodes", roomIdx, len(nodes))
	if len(nodes) == 0 {
		t.Error("expected at least one harvest node seeded")
	}

	// Try each harvest action a couple of times and watch for crashes.
	// Outcomes are RNG-driven; we mainly want no panics + coherent state.
	rng := rand.New(rand.NewPCG(7, 13))
	_ = rng
	for _, action := range []HarvestAction{HarvestForage, HarvestMine} {
		for i := 0; i < 3; i++ {
			if err := p.handleHarvestCmd(MessageContext{Sender: uid}, action); err != nil {
				t.Fatalf("handleHarvestCmd(%s) attempt %d: %v", action, i, err)
			}
			fresh, _ := getExpedition(exp.ID)
			if fresh == nil {
				t.Logf("expedition ended during harvest %s #%d", action, i)
				break
			}
			t.Logf("after %s #%d: supplies=%v threat=%d", action, i, fresh.Supplies.Current, fresh.ThreatLevel)
		}
	}

	// !resources output path (no-op SendDM but should not error).
	if err := p.handleResourcesCmd(MessageContext{Sender: uid}); err != nil {
		t.Fatalf("handleResourcesCmd: %v", err)
	}
}
