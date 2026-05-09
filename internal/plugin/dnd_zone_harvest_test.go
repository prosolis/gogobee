package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Standalone-zone-run harvest path: with no active expedition but an
// active !zone enter run, !forage / !mine / !fish should land yields
// against per-run node storage. Mirrors the bug Rurina hit where harvest
// commands silently bounced because the handler hard-required an
// expedition.

func TestStandaloneHarvest_RoutesToZoneRun(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@stand-harvest-route:example")
	zoneCmdTestCharacter(t, uid, 4)
	t.Cleanup(func() { cleanupZoneRuns(uid); cleanupExpeditions(uid) })

	run, err := startZoneRun(uid, ZoneForestShadows, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Mark room 0 (entry) as cleared so harvest is allowed there.
	if !currentRoomCleared(run) {
		t.Fatalf("entry room should auto-clear")
	}

	p := &AdventurePlugin{}
	// No expedition exists for this user — handler must fall through to
	// the standalone path and not bounce with the expedition-required
	// error message.
	if err := p.handleHarvestCmd(MessageContext{Sender: uid}, HarvestForage); err != nil {
		t.Fatalf("handleHarvestCmd: %v", err)
	}

	// Side-effect check: the run row should now have non-empty
	// harvest_nodes_json since saveStandaloneHarvestNodes ran.
	var raw string
	row := db.Get().QueryRow(
		`SELECT harvest_nodes_json FROM dnd_zone_run WHERE run_id = ?`, run.RunID)
	if err := row.Scan(&raw); err != nil {
		t.Fatalf("scan harvest_nodes_json: %v", err)
	}
	if raw == "" || raw == "{}" {
		t.Errorf("expected harvest_nodes_json populated after standalone harvest, got %q", raw)
	}
	if !strings.Contains(raw, "\"0\":") {
		t.Errorf("expected room 0 entry in harvest table, got %q", raw)
	}
}

func TestStandaloneHarvest_BouncesWhenNoActiveAnything(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@stand-harvest-noctx:example")
	zoneCmdTestCharacter(t, uid, 1)

	p := &AdventurePlugin{}
	// No expedition, no zone run — should bounce cleanly with the new
	// combined hint, not panic on a nil run.
	if err := p.handleHarvestCmd(MessageContext{Sender: uid}, HarvestForage); err != nil {
		t.Fatalf("handleHarvestCmd: %v", err)
	}
}

func TestStandaloneHarvest_FishGatedToFishingZones(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@stand-harvest-fish:example")
	zoneCmdTestCharacter(t, uid, 1)
	t.Cleanup(func() { cleanupZoneRuns(uid) })

	// Goblin Warrens isn't a fishing zone — !fish must reject.
	if _, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil); err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	if err := p.handleHarvestCmd(MessageContext{Sender: uid}, HarvestFish); err != nil {
		t.Fatalf("handleHarvestCmd: %v", err)
	}
	// (no easy way to assert the rejection without capturing SendDM —
	// the no-error return + no node persistence is the contract.)
	var raw string
	_ = db.Get().QueryRow(
		`SELECT COALESCE(harvest_nodes_json,'') FROM dnd_zone_run
		  WHERE user_id = ? ORDER BY started_at DESC LIMIT 1`, string(uid)).Scan(&raw)
	if raw != "" && raw != "{}" {
		t.Errorf("fish in non-fishing zone should not seed nodes; got %q", raw)
	}
}
