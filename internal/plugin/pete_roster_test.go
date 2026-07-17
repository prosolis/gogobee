package plugin

import (
	"sort"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// seedRosterPlayer puts a real, playable character on the board: a player_meta
// row (which carries the idle clock and the display name) plus a confirmed
// dnd_character. Real rows on purpose — the snapshot query reads two DATETIME
// columns, and the modernc affinity trap only fires against actual stored
// values, never against a hand-built struct.
func seedRosterPlayer(t *testing.T, uid id.UserID, name string, created, lastAction *time.Time) {
	t.Helper()
	if _, err := db.Get().Exec(
		`INSERT INTO player_meta (user_id, display_name, alive, created_at, last_player_action_at)
		 VALUES (?, ?, 1, ?, ?)`,
		string(uid), name, created, lastAction); err != nil {
		t.Fatalf("seed player_meta: %v", err)
	}
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
		PendingSetup: false,
	}); err != nil {
		t.Fatalf("seed character: %v", err)
	}
}

// TestRosterSnapshotReadsTheClock is the scan-affinity guard. The snapshot
// selects last_player_action_at and created_at as declared DATETIME columns and
// folds them in Go; a COALESCE() in the SQL would erase the affinity, the Scan
// would fail, and buildRosterSnapshot would return an error — publishing an
// empty board (every adventurer vanishes from the public page) rather than
// anything obviously broken.
func TestRosterSnapshotReadsTheClock(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-50 * time.Hour)
	recent := now.Add(-3 * time.Hour)

	seedRosterPlayer(t, "@quiet:test", "Quack", &old, &old)
	seedRosterPlayer(t, "@recent:test", "Josie", &old, &recent)
	// Never acted at all: the fold falls through to created_at.
	seedRosterPlayer(t, "@never:test", "Camcast", &old, nil)

	snap, err := buildRosterSnapshot(now, nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	if len(snap.Adventurers) != 3 {
		t.Fatalf("board has %d adventurers, want 3", len(snap.Adventurers))
	}
	if snap.SnapshotAt != now.Unix() {
		t.Errorf("snapshot_at = %d, want %d", snap.SnapshotAt, now.Unix())
	}

	byName := map[string]int{}
	for _, a := range snap.Adventurers {
		byName[a.Name] = a.IdleHours
		if a.Status != "idle" {
			t.Errorf("%s status = %q, want idle (nobody is on an expedition)", a.Name, a.Status)
		}
		if a.Token == "" {
			t.Errorf("%s has no board token", a.Name)
		}
	}
	if got := byName["Quack"]; got != 50 {
		t.Errorf("Quack idle hours = %d, want 50 — the clock didn't survive the scan", got)
	}
	if got := byName["Josie"]; got != 3 {
		t.Errorf("Josie idle hours = %d, want 3", got)
	}
	if got := byName["Camcast"]; got != 50 {
		t.Errorf("Camcast idle hours = %d, want 50 (fell through to created_at)", got)
	}
}

// TestRosterOmitsOptedOut is the privacy contract. Pete *replaces* its whole
// board with this payload, so omission is what enforces the opt-out. Anonymizing
// instead would be a fig leaf: a standing row with class + level + zone names the
// player anyway.
func TestRosterOmitsOptedOut(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-30 * time.Hour)

	seedRosterPlayer(t, "@shown:test", "Josie", &old, &old)
	seedRosterPlayer(t, "@hidden:test", "Quack", &old, &old)
	setNewsOptout("@hidden:test", true)

	snap, err := buildRosterSnapshot(now, nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	if len(snap.Adventurers) != 1 {
		t.Fatalf("board has %d adventurers, want 1", len(snap.Adventurers))
	}
	if snap.Adventurers[0].Name != "Quack" && snap.Adventurers[0].Name != "Josie" {
		t.Fatalf("unexpected adventurer %q", snap.Adventurers[0].Name)
	}
	if snap.Adventurers[0].Name == "Quack" {
		t.Error("an opted-out player is on the public board")
	}
}

// TestRosterTokenIsNotAnEventToken: the board token must not be one of the
// player's event tokens, or a standing row would become the key that links all
// their dispatches back together — the exact unlinkability eventToken exists to
// provide.
func TestRosterTokenIsNotAnEventToken(t *testing.T) {
	newBoredomTestDB(t)
	uid := id.UserID("@josie:test")

	board := eventToken(uid, "roster")
	if board == eventToken(uid, "arrival") || board == eventToken(uid, "death:crypt:1") {
		t.Error("board token collides with an event token — the board would deanonymize the feed")
	}
	if board != eventToken(uid, "roster") {
		t.Error("board token not stable — the row would churn identity every snapshot")
	}
}

// pickMagicItem returns a registry item matching want, so these tests read the
// real registry rather than pinning an item ID that a later SRD dump could drop.
func pickMagicItem(t *testing.T, want func(MagicItem) bool) MagicItem {
	t.Helper()
	var ids []string
	for id := range magicItemRegistry {
		ids = append(ids, id)
	}
	sort.Strings(ids) // map order is random; a flaky pick is a flaky test
	for _, id := range ids {
		if mi := magicItemRegistry[id]; want(mi) {
			return mi
		}
	}
	t.Skip("no registry item matches this shape")
	return MagicItem{}
}

// TestItemViewsKeepsTheRegistryPointerHome is the leak guard. SkillSource is two
// different things depending on the row: a player-facing skill name on
// masterwork gear ("mining"), and the internal "magic_item:<id>" pointer that
// resolves an inventory row back to the registry. Only the first is a fact about
// the item. Sending the second would put gogobee's internal IDs on a page, and
// Pete would have no way to tell them apart to filter them out.
func TestItemViewsKeepsTheRegistryPointerHome(t *testing.T) {
	newBoredomTestDB(t)
	mi := pickMagicItem(t, func(m MagicItem) bool { return m.Desc != "" && m.Slot != "" })

	views := itemViews([]AdvItem{
		{Name: mi.Name, Type: "magic_item", Tier: 3, Value: 100,
			SkillSource: "magic_item:" + mi.ID},
		{Name: "Miner's Pick", Type: "MasterworkGear", Tier: 3, Value: 300,
			Slot: SlotWeapon, SkillSource: "mining"},
	})

	if views[0].SkillSource != "" {
		t.Errorf("the magic_item registry pointer went out on the wire: %q", views[0].SkillSource)
	}
	if views[0].Desc != mi.Desc {
		t.Errorf("desc = %q, want the registry's %q", views[0].Desc, mi.Desc)
	}
	if views[0].Effect == "" {
		t.Error("a magic item should carry the engine's own effect summary")
	}
	if views[1].SkillSource != "mining" {
		t.Errorf("masterwork skill source = %q, want it kept", views[1].SkillSource)
	}
}

// TestItemViewsNeverClaimABackpackBond: equipping *moves* the row out of
// adventure_inventory into magic_item_equipped, so nothing in a backpack can
// hold a bond. Attuned must stay false here whatever the item wants, or the
// panel tells a player an unworn item is working for them.
func TestItemViewsNeverClaimABackpackBond(t *testing.T) {
	newBoredomTestDB(t)
	mi := pickMagicItem(t, func(m MagicItem) bool { return m.Attunement && m.Slot != "" })

	v := itemViews([]AdvItem{{Name: mi.Name, Type: "magic_item", Tier: 3,
		SkillSource: "magic_item:" + mi.ID}})[0]

	if !v.Attunement {
		t.Error("an attunement item should say it wants a bond")
	}
	if v.Attuned {
		t.Error("a backpack item claimed a bond it cannot hold")
	}
}

// TestEquippedViewsCarryBondState: the worn set is the only place Attuned means
// anything, and the only way the page can show that a worn item is sitting inert
// against the cap of three.
func TestEquippedViewsCarryBondState(t *testing.T) {
	newBoredomTestDB(t)
	uid := id.UserID("@josie:example.org")
	mi := pickMagicItem(t, func(m MagicItem) bool { return m.Attunement && m.Slot != "" })

	if err := equipMagicItem(uid, mi.Slot, mi.ID, false, 0); err != nil {
		t.Fatalf("equip: %v", err)
	}
	views := equippedViews(uid)
	if len(views) != 1 {
		t.Fatalf("equipped views = %d, want 1", len(views))
	}
	if views[0].Attuned {
		t.Error("an inert worn item was reported as bonded")
	}
	if views[0].Slot != string(mi.Slot) {
		t.Errorf("slot = %q, want %q", views[0].Slot, mi.Slot)
	}

	if err := equipMagicItem(uid, mi.Slot, mi.ID, true, 0); err != nil {
		t.Fatalf("re-equip bonded: %v", err)
	}
	if !equippedViews(uid)[0].Attuned {
		t.Error("a bonded worn item was reported as inert")
	}
}
