package plugin

import (
	"fmt"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// expeditionRowSeq gives each synthetic row a unique primary key; two rows can
// otherwise differ only in boss_defeated.
var expeditionRowSeq int

// insertClearedExpedition writes an expedition row directly. status and
// boss_defeated are the two gates the achievement checks read.
func insertClearedExpedition(t *testing.T, user id.UserID, zoneID ZoneID, status string, bossDefeated int, regionState string) {
	t.Helper()
	expeditionRowSeq++
	_, err := db.Get().Exec(
		`INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, status, boss_defeated, region_state)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		fmt.Sprintf("exp-%d", expeditionRowSeq), string(user), string(zoneID), status, bossDefeated, regionState)
	if err != nil {
		t.Fatal(err)
	}
}

// zonesOfTier lists the zone ids at a tier, straight from the registry, so the
// test tracks content edits instead of hardcoding a zone roster.
func zonesOfTier(tier ZoneTier) []ZoneID {
	var out []ZoneID
	for _, z := range allZones() {
		if z.Tier == tier {
			out = append(out, z.ID)
		}
	}
	return out
}

func newAchievementTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

func TestExpeditionClearAchievements(t *testing.T) {
	newAchievementTestDB(t)
	d := db.Get()
	user := id.UserID("@clears:test.invalid")

	t1 := zonesOfTier(1)
	if len(t1) < 2 {
		t.Skipf("tier 1 has %d zones; test wants at least 2", len(t1))
	}

	if clearedAnyZoneOfTier(d, user, 1) {
		t.Error("clean slate reported a tier-1 clear")
	}

	// A boss-defeated complete run is the only thing that counts.
	insertClearedExpedition(t, user, t1[0], ExpeditionStatusComplete, 1, "{}")
	if !clearedAnyZoneOfTier(d, user, 1) {
		t.Error("cleared tier-1 zone not detected")
	}
	if clearedEveryZoneOfTier(d, user, 1) {
		t.Error("one of several tier-1 zones counted as all of them")
	}
	if clearedAnyZoneOfTier(d, user, 2) {
		t.Error("a tier-1 clear leaked into tier 2")
	}

	for _, z := range t1[1:] {
		insertClearedExpedition(t, user, z, ExpeditionStatusComplete, 1, "{}")
	}
	if !clearedEveryZoneOfTier(d, user, 1) {
		t.Error("clearing every tier-1 zone did not satisfy the master check")
	}
}

// TestExpeditionClearIgnoresUnfinishedRuns: an abandoned run, or one where the
// player walked out without killing the boss, must not count as a clear.
func TestExpeditionClearIgnoresUnfinishedRuns(t *testing.T) {
	newAchievementTestDB(t)
	d := db.Get()
	user := id.UserID("@partial:test.invalid")

	t2 := zonesOfTier(2)
	if len(t2) == 0 {
		t.Skip("no tier-2 zones")
	}

	insertClearedExpedition(t, user, t2[0], ExpeditionStatusAbandoned, 1, "{}")
	if clearedAnyZoneOfTier(d, user, 2) {
		t.Error("an abandoned expedition counted as a clear")
	}

	insertClearedExpedition(t, user, t2[0], ExpeditionStatusComplete, 0, "{}")
	if clearedAnyZoneOfTier(d, user, 2) {
		t.Error("a complete run with the boss alive counted as a clear")
	}

	insertClearedExpedition(t, user, t2[0], ExpeditionStatusComplete, 1, "{}")
	if !clearedAnyZoneOfTier(d, user, 2) {
		t.Error("a real clear was not detected")
	}
}

// TestClearedUnderThreat50 pins the presence-vs-zero distinction. recordMaxThreat
// samples only on day rollover and never writes a zero, so an empty region_state
// means "no threat history", not "threat stayed at zero".
func TestClearedUnderThreat50(t *testing.T) {
	newAchievementTestDB(t)
	d := db.Get()

	zones := zonesOfTier(1)
	if len(zones) == 0 {
		t.Skip("no tier-1 zones")
	}
	z := zones[0]

	quiet := id.UserID("@quiet:test.invalid")
	loud := id.UserID("@loud:test.invalid")
	sameDay := id.UserID("@sameday:test.invalid")

	insertClearedExpedition(t, quiet, z, ExpeditionStatusComplete, 1, `{"max_threat_seen":30}`)
	if !clearedUnderThreat50(d, quiet) {
		t.Error("peak threat 30 did not satisfy the under-50 check")
	}

	insertClearedExpedition(t, loud, z, ExpeditionStatusComplete, 1, `{"max_threat_seen":60}`)
	if clearedUnderThreat50(d, loud) {
		t.Error("peak threat 60 satisfied the under-50 check")
	}

	insertClearedExpedition(t, sameDay, z, ExpeditionStatusComplete, 1, "{}")
	if clearedUnderThreat50(d, sameDay) {
		t.Error("an expedition with no threat samples was awarded the quiet clear")
	}

	// Exactly 50 is not under 50.
	edge := id.UserID("@edge:test.invalid")
	insertClearedExpedition(t, edge, z, ExpeditionStatusComplete, 1, `{"max_threat_seen":50}`)
	if clearedUnderThreat50(d, edge) {
		t.Error("peak threat of exactly 50 counted as under 50")
	}
}

// TestExpeditionAchievementIDsRegistered guards the wiring: every expedition
// achievement referenced by the tier helpers must exist in the registry, and a
// tier with no zones must not ship a master achievement nobody can earn.
func TestExpeditionAchievementIDsRegistered(t *testing.T) {
	p := &AchievementsPlugin{}
	defs := p.buildAchievements()
	ids := map[string]bool{}
	for _, a := range defs {
		if ids[a.ID] {
			t.Errorf("duplicate achievement id %q", a.ID)
		}
		ids[a.ID] = true
	}

	for tier := ZoneTier(1); tier <= 5; tier++ {
		if len(zonesOfTier(tier)) == 0 {
			continue
		}
		for _, prefix := range []string{"expedition_clear_t", "expedition_master_t"} {
			id := prefix + string(rune('0'+tier))
			if !ids[id] {
				t.Errorf("tier %d has zones but %q is not registered", tier, id)
			}
		}
	}
	for _, id := range []string{"expedition_quiet_clear", "temper_legendary"} {
		if !ids[id] {
			t.Errorf("%q is not registered", id)
		}
	}
}
