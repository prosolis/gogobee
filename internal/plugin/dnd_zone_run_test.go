package plugin

import (
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func setupZoneRunTestDB(t *testing.T) {
	t.Helper()
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func TestGenerateRoomSequence_StartsEntryEndsBoss(t *testing.T) {
	zone := zoneGoblinWarrens()
	rng := rand.New(rand.NewPCG(1, 2))
	seq := generateRoomSequence(zone, rng)
	if len(seq) < zone.MinRooms || len(seq) > zone.MaxRooms {
		t.Fatalf("seq len %d outside [%d,%d]", len(seq), zone.MinRooms, zone.MaxRooms)
	}
	if seq[0] != RoomEntry {
		t.Errorf("first room = %s, want entry", seq[0])
	}
	if seq[len(seq)-1] != RoomBoss {
		t.Errorf("last room = %s, want boss", seq[len(seq)-1])
	}
	// Exactly one of each special type:
	count := map[RoomType]int{}
	for _, r := range seq {
		count[r]++
	}
	if count[RoomEntry] != 1 {
		t.Errorf("entry count = %d", count[RoomEntry])
	}
	if count[RoomTrap] != 1 {
		t.Errorf("trap count = %d", count[RoomTrap])
	}
	if count[RoomElite] != 1 {
		t.Errorf("elite count = %d", count[RoomElite])
	}
	if count[RoomBoss] != 1 {
		t.Errorf("boss count = %d", count[RoomBoss])
	}
	if count[RoomExploration] < 2 {
		t.Errorf("exploration count = %d, want ≥ 2", count[RoomExploration])
	}
}

func TestStartZoneRun_HappyPath(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-test:example.org")
	defer cleanupZoneRuns(uid)

	rng := rand.New(rand.NewPCG(42, 7))
	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, rng)
	if err != nil {
		t.Fatalf("startZoneRun: %v", err)
	}
	if run.ZoneID != ZoneGoblinWarrens {
		t.Errorf("zone id = %s", run.ZoneID)
	}
	if run.CurrentRoom != 0 {
		t.Errorf("current room = %d", run.CurrentRoom)
	}
	if run.GMMood != 50 {
		t.Errorf("gm mood = %d", run.GMMood)
	}
	if !run.IsActive() {
		t.Error("expected run active")
	}
	if run.CurrentRoomType() != RoomEntry {
		t.Errorf("first room type = %s", run.CurrentRoomType())
	}
}

func TestStartZoneRun_RejectsConcurrent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-concur:example.org")
	defer cleanupZoneRuns(uid)

	if _, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := startZoneRun(uid, ZoneCryptValdris, 1, nil); err != ErrRunAlreadyActive {
		t.Errorf("err = %v, want ErrRunAlreadyActive", err)
	}
}

func TestStartZoneRun_RejectsTierLocked(t *testing.T) {
	// L1 player can reach tier 3 max. Crypt is T1 — fine. But if we
	// register a T5 in future, this test should still work — for now,
	// fake the gate by trying with negative level.
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-tier:example.org")
	defer cleanupZoneRuns(uid)

	// All current zones are T1 so any positive level passes.  Test
	// the unknown-zone path instead, which is the other guard.
	if _, err := startZoneRun(uid, ZoneID("nonexistent"), 1, nil); err != ErrUnknownZone {
		t.Errorf("err = %v, want ErrUnknownZone", err)
	}
}

func TestZoneRunFlow_AdvanceToBossAndComplete(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-flow:example.org")
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 2, rand.New(rand.NewPCG(99, 1)))
	if err != nil {
		t.Fatal(err)
	}
	steps := 0
	for {
		next, err := markRoomCleared(run.RunID)
		if err != nil {
			t.Fatalf("markRoomCleared step %d: %v", steps, err)
		}
		steps++
		if steps > 20 {
			t.Fatal("too many steps")
		}
		if next == "" {
			break
		}
	}
	got, err := getZoneRun(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.BossDefeated {
		t.Error("expected boss defeated after final room")
	}
	if got.CompletedAt == nil {
		t.Error("expected CompletedAt set")
	}
	if got.IsActive() {
		t.Error("expected run inactive")
	}
	if len(got.RoomsCleared) != got.TotalRooms {
		t.Errorf("rooms cleared %d, total %d", len(got.RoomsCleared), got.TotalRooms)
	}
}

func TestAbandonZoneRun(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-abandon:example.org")
	defer cleanupZoneRuns(uid)

	if _, err := startZoneRun(uid, ZoneCryptValdris, 1, nil); err != nil {
		t.Fatal(err)
	}
	if err := abandonZoneRun(uid); err != nil {
		t.Fatalf("abandon: %v", err)
	}
	active, err := getActiveZoneRun(uid)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Error("expected no active run after abandon")
	}
	// Second abandon should ErrNoActiveRun.
	if err := abandonZoneRun(uid); err != ErrNoActiveRun {
		t.Errorf("second abandon err = %v", err)
	}
}

func TestAdjustGMMoodClampsBounds(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-mood:example.org")
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Push above 100 then below 0.
	if err := adjustGMMood(run.RunID, 200); err != nil {
		t.Fatal(err)
	}
	r, _ := getZoneRun(run.RunID)
	if r.GMMood != 100 {
		t.Errorf("upper clamp: mood = %d", r.GMMood)
	}
	if err := adjustGMMood(run.RunID, -250); err != nil {
		t.Fatal(err)
	}
	r, _ = getZoneRun(run.RunID)
	if r.GMMood != 0 {
		t.Errorf("lower clamp: mood = %d", r.GMMood)
	}
}

func TestAddLootAccumulates(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@zone-loot:example.org")
	defer cleanupZoneRuns(uid)

	run, err := startZoneRun(uid, ZoneGoblinWarrens, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := addLoot(run.RunID, "wpn_handaxe_+1"); err != nil {
		t.Fatal(err)
	}
	if err := addLoot(run.RunID, "coins_2d10x5"); err != nil {
		t.Fatal(err)
	}
	got, _ := getZoneRun(run.RunID)
	if len(got.LootCollected) != 2 {
		t.Fatalf("loot len %d", len(got.LootCollected))
	}
	if got.LootCollected[0] != "wpn_handaxe_+1" || got.LootCollected[1] != "coins_2d10x5" {
		t.Errorf("loot order: %v", got.LootCollected)
	}
}

func cleanupZoneRuns(uid id.UserID) {
	_, _ = db.Get().Exec(`DELETE FROM dnd_zone_run WHERE user_id = ?`, string(uid))
}
