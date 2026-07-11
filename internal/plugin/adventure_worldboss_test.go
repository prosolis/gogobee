package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func newWorldBossTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func TestWorldBossName_DeterministicAndInPool(t *testing.T) {
	first := worldBossNameFor("2026-07")
	if again := worldBossNameFor("2026-07"); again != first {
		t.Errorf("name not deterministic: %q vs %q", first, again)
	}
	inPool := false
	for _, n := range worldBossNames {
		if n == first {
			inPool = true
			break
		}
	}
	if !inPool {
		t.Errorf("generated name %q not in worldBossNames pool", first)
	}
}

func TestMedianInt(t *testing.T) {
	cases := []struct {
		in   []int
		want int
	}{
		{nil, 0},
		{[]int{5}, 5},
		{[]int{3, 1, 2}, 2},         // odd, unsorted
		{[]int{4, 2, 8, 6}, 5},      // even → mean of the two middles (4,6)
		{[]int{10, 10, 10, 10}, 10}, // even, all equal
	}
	for _, c := range cases {
		if got := medianInt(c.in); got != c.want {
			t.Errorf("medianInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTierForCombinedLevel(t *testing.T) {
	cases := []struct{ level, want int }{
		{0, worldBossMinTier},
		{11, worldBossMinTier},
		{12, 4},
		{20, 4},
		{21, 5},
		{99, 5},
	}
	for _, c := range cases {
		if got := tierForCombinedLevel(c.level); got != c.want {
			t.Errorf("tierForCombinedLevel(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestWorldBoss_InsertLoadRoundTrip(t *testing.T) {
	newWorldBossTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	id0, err := insertWorldBoss("Gorloth", 5, 2400, now, now.Add(worldBossWindow))
	if err != nil {
		t.Fatal(err)
	}
	active, err := loadActiveWorldBoss()
	if err != nil || active == nil {
		t.Fatalf("loadActiveWorldBoss: %v (nil=%v)", err, active == nil)
	}
	if active.ID != id0 || active.Name != "Gorloth" || active.Tier != 5 ||
		active.HPMax != 2400 || active.HPCurrent != 2400 || active.Status != "active" {
		t.Errorf("round-trip mismatch: %+v", active)
	}
}

func TestApplyWorldBossDamage_ClampsAndFells(t *testing.T) {
	newWorldBossTestDB(t)
	now := time.Now().UTC()
	bossID, err := insertWorldBoss("Kravok", 3, 100, now, now.Add(worldBossWindow))
	if err != nil {
		t.Fatal(err)
	}

	rem, killed, err := applyWorldBossDamage(bossID, 40)
	if err != nil || rem != 60 || killed {
		t.Fatalf("first hit: rem=%d killed=%v err=%v (want 60,false)", rem, killed, err)
	}

	// Overkill clamps at 0 and reports the fell.
	rem, killed, err = applyWorldBossDamage(bossID, 999)
	if err != nil || rem != 0 || !killed {
		t.Fatalf("overkill: rem=%d killed=%v err=%v (want 0,true)", rem, killed, err)
	}

	// Negative damage is treated as zero.
	if rem, _, err := applyWorldBossDamage(bossID, -5); err != nil || rem != 0 {
		t.Fatalf("negative: rem=%d err=%v", rem, err)
	}
}

func TestSetWorldBossStatus_ResolvesOnce(t *testing.T) {
	newWorldBossTestDB(t)
	now := time.Now().UTC()
	bossID, err := insertWorldBoss("Ymirok", 4, 500, now, now.Add(worldBossWindow))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := setWorldBossStatus(bossID, "defeated"); err != nil || !ok {
		t.Fatalf("first close-out: ok=%v err=%v (want true)", ok, err)
	}
	// A second close-out is a no-op — the boss is no longer active.
	if ok, err := setWorldBossStatus(bossID, "survived"); err != nil || ok {
		t.Fatalf("double close-out: ok=%v err=%v (want false)", ok, err)
	}
	// And damage no longer lands on a resolved boss.
	if rem, killed, _ := applyWorldBossDamage(bossID, 500); rem != 500 || killed {
		t.Errorf("damage on resolved boss changed pool: rem=%d killed=%v", rem, killed)
	}
}

func TestWorldBossContrib_UpsertAccumulates(t *testing.T) {
	newWorldBossTestDB(t)
	user := id.UserID("@a:test.invalid")
	if err := upsertWorldBossContrib(1, user, 30, "2026-07-01"); err != nil {
		t.Fatal(err)
	}
	if err := upsertWorldBossContrib(1, user, 45, "2026-07-02"); err != nil {
		t.Fatal(err)
	}
	c, err := loadWorldBossContrib(1, user)
	if err != nil || c == nil {
		t.Fatalf("load: %v (nil=%v)", err, c == nil)
	}
	if c.Fights != 2 || c.Damage != 75 || c.LastFightDate != "2026-07-02" {
		t.Errorf("accumulate mismatch: %+v", c)
	}
}

func TestLoadWorldBossContribs_OrderedByFights(t *testing.T) {
	newWorldBossTestDB(t)
	a := id.UserID("@a:test.invalid")
	b := id.UserID("@b:test.invalid")
	// a: 1 fight; b: 2 fights → b sorts first.
	upsertWorldBossContrib(7, a, 100, "2026-07-01")
	upsertWorldBossContrib(7, b, 10, "2026-07-01")
	upsertWorldBossContrib(7, b, 10, "2026-07-02")
	got, err := loadWorldBossContribs(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UserID != b || got[1].UserID != a {
		t.Errorf("order = %+v, want b then a", got)
	}
}

func TestComputeWorldBossPayouts_ScalesByFights(t *testing.T) {
	contribs := []worldBossContrib{
		{UserID: "@a:t", Fights: 3, Damage: 999},
		{UserID: "@b:t", Fights: 1, Damage: 10},
		{UserID: "@c:t", Fights: 0, Damage: 0}, // no fights → no payout
	}
	pays := computeWorldBossPayouts(contribs, 1000)
	if len(pays) != 2 {
		t.Fatalf("got %d payouts, want 2 (zero-fight excluded)", len(pays))
	}
	if pays[0].Euro != 3000 || pays[1].Euro != 1000 {
		t.Errorf("payouts = %d,%d want 3000,1000", pays[0].Euro, pays[1].Euro)
	}
}

// TestWorldBossSpawnPlan_SizesToActiveTown seeds three active max-level players
// and checks the boss is T5 with a pool scaled to the turnout.
func TestWorldBossSpawnPlan_SizesToActiveTown(t *testing.T) {
	newWorldBossTestDB(t)
	today := time.Now().UTC().Format("2006-01-02")
	for _, u := range []string{"@a:test.invalid", "@b:test.invalid", "@c:test.invalid"} {
		c := &AdventureCharacter{UserID: id.UserID(u), DisplayName: "P", Alive: true,
			ForagingSkill: 30, CreatedAt: time.Now().UTC()}
		if err := saveAdvCharacter(c); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Get().Exec(
			`INSERT INTO daily_activity (user_id, date, message_count) VALUES (?, ?, 1)`,
			u, today); err != nil {
			t.Fatal(err)
		}
	}
	tier, hpMax, activeN := worldBossSpawnPlan()
	if activeN != 3 {
		t.Errorf("activeN = %d, want 3", activeN)
	}
	if tier != 5 {
		t.Errorf("tier = %d, want 5 (combined >= 21)", tier)
	}
	// bouts = 3 × 2.0 = 6; perBout at T5 = 400 → pool 2400.
	perBout, _, _, _, _ := arenaTierBaseStats(5)
	if want := perBout * 6; hpMax != want {
		t.Errorf("hpMax = %d, want %d", hpMax, want)
	}
}

// TestWorldBossSpawnPlan_EmptyTownGetsFloorBoss: no active players still yields a
// beatable floor boss so a manual spawn works.
func TestWorldBossSpawnPlan_EmptyTownGetsFloorBoss(t *testing.T) {
	newWorldBossTestDB(t)
	tier, hpMax, activeN := worldBossSpawnPlan()
	if activeN != 0 {
		t.Errorf("activeN = %d, want 0", activeN)
	}
	if tier != worldBossMinTier {
		t.Errorf("tier = %d, want floor %d", tier, worldBossMinTier)
	}
	perBout, _, _, _, _ := arenaTierBaseStats(worldBossMinTier)
	if want := perBout * worldBossMinBouts; hpMax != want {
		t.Errorf("hpMax = %d, want %d (min bouts floor)", hpMax, want)
	}
}

func TestSpawnWorldBoss_RefusesWhenOneIsActive(t *testing.T) {
	newWorldBossTestDB(t)
	p := &AdventurePlugin{}
	first, err := p.spawnWorldBoss("2026-07")
	if err != nil || first == nil {
		t.Fatalf("first spawn: %v (nil=%v)", err, first == nil)
	}
	second, err := p.spawnWorldBoss("2026-07")
	if err == nil {
		t.Error("second spawn should refuse while one is active")
	}
	if second == nil || second.ID != first.ID {
		t.Error("refusal should return the existing active boss")
	}
}

// TestResolveWorldBossSurvived_DebitsPot exercises the survive path end to end:
// the boss closes out and the pot loses its tribute.
func TestResolveWorldBossSurvived_DebitsPot(t *testing.T) {
	newWorldBossTestDB(t)
	communityPotAdd(1000)
	now := time.Now().UTC()
	bossID, err := insertWorldBoss("Vornath", 5, 2400, now.Add(-worldBossWindow), now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	boss, _ := loadWorldBoss(bossID)
	p := &AdventurePlugin{}
	p.resolveWorldBossSurvived(boss)

	after, _ := loadWorldBoss(bossID)
	if after.Status != "survived" {
		t.Errorf("status = %q, want survived", after.Status)
	}
	// tribute = 20% of 1000 = 200 → pot left with 800.
	if bal := communityPotBalance(); bal != 800 {
		t.Errorf("pot = %d, want 800 after 20%% tribute", bal)
	}
}
