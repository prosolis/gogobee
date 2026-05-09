package plugin

import (
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// ── pure-function tests (no DB) ────────────────────────────────────────────

func TestResolveHarvestOutcome_Brackets(t *testing.T) {
	cases := []struct {
		roll, dc int
		want     HarvestOutcome
	}{
		{20, 10, OutcomeRich},     // +10 = rich
		{15, 10, OutcomeRich},     // exactly +5
		{14, 10, OutcomeStandard}, // +4
		{10, 10, OutcomeStandard}, // exact
		{9, 10, OutcomeCommon},    // -1
		{6, 10, OutcomeCommon},    // -4
		{5, 10, OutcomeNothing},   // -5 = nothing
		{1, 20, OutcomeNothing},
	}
	for _, c := range cases {
		got := resolveHarvestOutcome(c.roll, c.dc)
		if got != c.want {
			t.Errorf("resolveHarvestOutcome(%d, %d) = %s, want %s",
				c.roll, c.dc, got, c.want)
		}
	}
}

func TestClassHarvestBonus_Affinity(t *testing.T) {
	cases := []struct {
		class  DnDClass
		action HarvestAction
		want   int
	}{
		{ClassRanger, HarvestForage, 2},
		{ClassRanger, HarvestFish, 2},
		{ClassRanger, HarvestMine, 0},
		{ClassFighter, HarvestMine, 2},
		{ClassRogue, HarvestScavenge, 2},
		{ClassMage, HarvestEssence, 2},
		{ClassCleric, HarvestCommune, 2},
		{ClassCleric, HarvestEssence, 0},
	}
	for _, c := range cases {
		got := classHarvestBonus(c.class, c.action)
		if got != c.want {
			t.Errorf("classHarvestBonus(%s, %s) = %d, want %d",
				c.class, c.action, got, c.want)
		}
	}
}

func TestZoneResourcesByAction_AllZonesPopulated(t *testing.T) {
	zones := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneForestShadows,
		ZoneSunkenTemple, ZoneManorBlackspire, ZoneUnderforge,
		ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal,
	}
	for _, z := range zones {
		all := ZoneResourcesAll(z)
		if len(all) == 0 {
			t.Errorf("zone %s has empty registry", z)
		}
		// Every entry has zone id backfilled by init().
		for _, r := range all {
			if r.ZoneID != z {
				t.Errorf("zone %s entry %s has wrong zone id %s", z, r.ID, r.ZoneID)
			}
			if r.Action == "" || r.Name == "" || r.ID == "" {
				t.Errorf("zone %s entry malformed: %+v", z, r)
			}
		}
	}
}

func TestSeedRoomNodes_MatchesRegistry(t *testing.T) {
	nodes := seedRoomNodes(ZoneGoblinWarrens)
	all := ZoneResourcesAll(ZoneGoblinWarrens)
	if len(nodes) != len(all) {
		t.Fatalf("seeded %d, registry %d", len(nodes), len(all))
	}
	for i, n := range nodes {
		if n.CurrentCharges <= 0 {
			t.Errorf("node %d charged 0 at seed", i)
		}
		if n.CurrentCharges != n.MaxCharges {
			t.Errorf("node %d charges %d != max %d", i, n.CurrentCharges, n.MaxCharges)
		}
	}
}

func TestPickAvailableNode_PrefersLowestDC(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneGoblinWarrens}
	char := &DnDCharacter{Class: ClassFighter, STR: 14}
	nodes := []HarvestNode{
		{ResourceID: "scrap_iron", CurrentCharges: 2, MaxCharges: 2}, // DC 10 mine
	}
	idx := pickAvailableNode(nodes, exp, char, HarvestMine)
	if idx != 0 {
		t.Errorf("idx = %d, want 0 (scrap iron)", idx)
	}
	// Wrong action returns -1.
	if idx := pickAvailableNode(nodes, exp, char, HarvestForage); idx != -1 {
		t.Errorf("forage on mine-only registry = %d, want -1", idx)
	}
	// Class-restricted: shaman reagents are mage-only.
	mageOnly := []HarvestNode{
		{ResourceID: "shaman_reagents", CurrentCharges: 1, MaxCharges: 1},
	}
	if idx := pickAvailableNode(mageOnly, exp, char, HarvestEssence); idx != -1 {
		t.Errorf("fighter on shaman reagents = %d, want -1 (mage-only)", idx)
	}
	mageChar := &DnDCharacter{Class: ClassMage, INT: 16}
	if idx := pickAvailableNode(mageOnly, exp, mageChar, HarvestEssence); idx != 0 {
		t.Errorf("mage on shaman reagents = %d, want 0", idx)
	}
}

func TestPickAvailableNode_SkipsRequiresKill(t *testing.T) {
	exp := &Expedition{ZoneID: ZoneGoblinWarrens, RegionState: map[string]any{}}
	char := &DnDCharacter{Class: ClassRogue, INT: 12}
	nodes := []HarvestNode{
		{ResourceID: "worg_pelt", CurrentCharges: 1, MaxCharges: 1}, // requires worg kill
	}
	if idx := pickAvailableNode(nodes, exp, char, HarvestScavenge); idx != -1 {
		t.Errorf("requires-kill resource selectable without kill: idx=%d", idx)
	}
	// With kill recorded, becomes available.
	exp.RegionState["kills"] = map[string][]string{"": {"worg"}}
	if idx := pickAvailableNode(nodes, exp, char, HarvestScavenge); idx != 0 {
		t.Errorf("post-kill, idx = %d, want 0", idx)
	}
}

func TestCurrentRoomCleared_EntryAutoPasses(t *testing.T) {
	run := &DungeonRun{
		CurrentRoom:  0,
		RoomSeq:      []RoomType{RoomEntry, RoomExploration, RoomBoss},
		RoomsCleared: []int{},
	}
	if !currentRoomCleared(run) {
		t.Error("entry room should auto-clear")
	}
	run.CurrentRoom = 1
	if currentRoomCleared(run) {
		t.Error("non-cleared exploration room should not be cleared")
	}
	run.RoomsCleared = []int{1}
	if !currentRoomCleared(run) {
		t.Error("room in RoomsCleared should report cleared")
	}
}

// ── round-trip tests (require DB) ──────────────────────────────────────────

func cleanupHarvestState(uid id.UserID) {
	cleanupExpeditions(uid)
	_, _ = db.Get().Exec(`DELETE FROM dnd_zone_run WHERE user_id = ?`, string(uid))
	_, _ = db.Get().Exec(`DELETE FROM adventure_inventory WHERE user_id = ?`, string(uid))
}

func TestEnsureRegionRun_LinksExpeditionToRun(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@harvest-ensure:example.org")
	defer cleanupHarvestState(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	run, err := ensureRegionRun(exp, 1)
	if err != nil {
		t.Fatalf("ensureRegionRun: %v", err)
	}
	if run.RunID == "" {
		t.Error("run id empty")
	}
	if exp.RunID != run.RunID {
		t.Errorf("exp.RunID = %q, want %q", exp.RunID, run.RunID)
	}
	// Idempotent: calling again returns the same run.
	again, err := ensureRegionRun(exp, 1)
	if err != nil || again.RunID != run.RunID {
		t.Errorf("second ensureRegionRun returned different run: %v vs %v", again, run)
	}
	// And the mapping persists.
	runs := regionRunsMap(exp)
	if runs[""] != run.RunID {
		t.Errorf("region_runs[\"\"]=%q, want %q", runs[""], run.RunID)
	}
}

func TestRetireRegionRun_AbandonsAndClears(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@harvest-retire:example.org")
	defer cleanupHarvestState(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	run, err := ensureRegionRun(exp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := retireRegionRun(exp, ""); err != nil {
		t.Fatalf("retireRegionRun: %v", err)
	}
	// Underlying run should be marked abandoned.
	got, _ := getZoneRun(run.RunID)
	if got == nil || !got.Abandoned {
		t.Errorf("expected run abandoned, got %+v", got)
	}
	// Mapping cleared.
	if got := regionRunsMap(exp); got[""] != "" {
		t.Errorf("region_runs not cleared: %+v", got)
	}
	// exp.RunID cleared.
	if exp.RunID != "" {
		t.Errorf("exp.RunID = %q, want empty", exp.RunID)
	}
}

func TestReplenishHarvestNodes_RestoresAllRooms(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@harvest-replenish:example.org")
	defer cleanupHarvestState(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}

	// Seed two rooms and drain one node in each.
	r0 := loadHarvestNodes(exp, 0)
	r0[0].CurrentCharges = 0
	if err := saveHarvestNodes(exp, 0, r0); err != nil {
		t.Fatal(err)
	}
	r1 := loadHarvestNodes(exp, 1)
	r1[0].CurrentCharges = 0
	if err := saveHarvestNodes(exp, 1, r1); err != nil {
		t.Fatal(err)
	}

	if err := ReplenishHarvestNodes(exp); err != nil {
		t.Fatal(err)
	}
	for _, idx := range []int{0, 1} {
		nodes := loadHarvestNodes(exp, idx)
		for i, n := range nodes {
			if n.CurrentCharges != n.MaxCharges {
				t.Errorf("room %d node %d not replenished: %d/%d",
					idx, i, n.CurrentCharges, n.MaxCharges)
			}
		}
	}
}

func TestSaveHarvestNodes_PersistsAcrossLoad(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@harvest-persist:example.org")
	defer cleanupHarvestState(uid)

	supplies := ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1, HarshMod: 1}
	exp, err := startExpedition(uid, ZoneGoblinWarrens, "", supplies)
	if err != nil {
		t.Fatal(err)
	}
	nodes := loadHarvestNodes(exp, 2)
	nodes[0].CurrentCharges = 0
	if err := saveHarvestNodes(exp, 2, nodes); err != nil {
		t.Fatal(err)
	}
	// Reload from DB.
	reloaded, err := getActiveExpedition(uid)
	if err != nil {
		t.Fatal(err)
	}
	got := loadHarvestNodes(reloaded, 2)
	if got[0].CurrentCharges != 0 {
		t.Errorf("after reload, room 2 node 0 charges = %d, want 0", got[0].CurrentCharges)
	}
	// Other rooms should still be untouched (lazy-seeded fresh).
	other := loadHarvestNodes(reloaded, 5)
	for _, n := range other {
		if n.CurrentCharges != n.MaxCharges {
			t.Errorf("room 5 leaked depletion: %+v", n)
		}
	}
}
