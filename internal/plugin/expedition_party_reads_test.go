package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// N3/P6d — the player-facing `getActive*` reads, rewired.
//
// SendDM is a no-op without a Matrix client, so these tests can only observe the
// state a command path left behind. That covers the cases that carry state:
//
//   - reads that were blind to a member and let them adventure twice
//     (`!zone enter`, `!expedition start`, `!sell`), including across the
//     `extracting` limbo where the seat outlives the expedition's 'active' status;
//   - reads that write, and so must not write on a member's behalf
//     (`!resources` seed-persists the leader's whole region_state blob;
//     applyMoodDecayIfStale writes an absolute gm_mood);
//   - the one read whose rewire deliberately does move shared state — `!zone
//     taunt` swings the party's DM mood, safe only because that write is an
//     atomic delta.
//
// The remaining rewires are copy-only — a member used to be told they had no
// expedition, and is now told whose it is — and both spellings leave the database
// exactly as they found it, so there is nothing here to assert. Their seam,
// activeExpeditionFor / activeZoneRunFor / isPartyMember, is pinned in
// expedition_party_resolve_test.go.

// seatedMember stands up a leader with an active expedition and zone run, plus
// a seated member holding a real character sheet. Returns both user ids.
func seatedMember(t *testing.T, tag string) (leader, member id.UserID) {
	t.Helper()
	leader = id.UserID("@lead-" + tag + ":example.org")
	member = id.UserID("@member-" + tag + ":example.org")
	seedExpedition(t, "exp-"+tag, leader, "active")
	seedZoneRun(t, "run-exp-"+tag, leader)
	if err := joinParty("exp-"+tag, member); err != nil {
		t.Fatal(err)
	}
	zoneCmdTestCharacter(t, leader, 1)
	zoneCmdTestCharacter(t, member, 1)
	return leader, member
}

// startZoneRun only refuses a player who owns an active run, and a member owns
// none — so before P6d a member could `!zone enter` a private dungeon while
// riding the leader's. That run would then win every activeZoneRunFor lookup
// they made, quietly cutting them out of the party's state.
func TestZoneCmdEnter_SeatedMemberCannotOpenTheirOwnRun(t *testing.T) {
	setupEmptyTestDB(t)
	_, member := seatedMember(t, "enter")

	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: member}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if run, err := getActiveZoneRun(member); err != nil || run != nil {
		t.Fatalf("member opened their own run %v (err %v); they are seated in a party", run, err)
	}
}

// The same hole at the expedition seam: getActiveExpedition is blind to a
// member, so the "already on expedition" guard waved them through.
func TestExpeditionCmdStart_SeatedMemberIsRefused(t *testing.T) {
	setupEmptyTestDB(t)
	_, member := seatedMember(t, "start")

	// Fund them: a member refused for want of coins would pass this test for
	// the wrong reason.
	euro := &EuroPlugin{}
	euro.ensureBalance(member)
	euro.Credit(member, 500, "party reads test")
	p := &AdventurePlugin{euro: euro}

	c, err := LoadDnDCharacter(member)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.expeditionCmdStart(MessageContext{Sender: member}, c, "goblin_warrens 1s"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if own, err := getActiveExpedition(member); err != nil || own != nil {
		t.Fatalf("member started their own expedition %v (err %v) while seated in a party", own, err)
	}
}

// A member is mid-expedition even though they own no expedition row, so Thom
// Krooke must turn them away like anyone else standing in a dungeon.
func TestSellCmd_SeatedMemberIsRefusedMidExpedition(t *testing.T) {
	setupEmptyTestDB(t)
	_, member := seatedMember(t, "sell")
	if err := addAdvInventoryItem(member, AdvItem{
		Name: "goblin ear", Type: "material", Tier: 1, Value: 5,
	}); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleResourceSellCmd(MessageContext{Sender: member}, "all"); err != nil {
		t.Fatalf("sell: %v", err)
	}
	items, err := loadAdvInventory(member)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("inventory holds %d items, want the 1 seeded; a seated member sold mid-expedition", len(items))
	}
}

// `!resources` looks like a pure read, but it seed-persists harvest nodes — and
// saveHarvestNodes rewrites the leader's whole region_state blob (a last-write-
// wins UPDATE). A member holds only their own lock, so their snapshot must never
// be written back over the leader's walk. loadHarvestNodes re-derives the same
// nodes from seedRoomNodes, so the member loses nothing by not persisting.
func TestResourcesCmd_MemberDoesNotRewriteTheLeadersRegionState(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "res")

	// A sentinel only the leader's walk could have written. If the member's
	// !resources persists its snapshot, this is clobbered by the harvest table.
	sentinel := `{"party_only_marker":"survive"}`
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET region_state = ? WHERE user_id = ?`,
		sentinel, string(leader),
	); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleResourcesCmd(MessageContext{Sender: member}); err != nil {
		t.Fatalf("resources: %v", err)
	}

	var got string
	if err := db.Get().QueryRow(
		`SELECT region_state FROM dnd_expedition WHERE user_id = ?`, string(leader),
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != sentinel {
		t.Errorf("member's !resources rewrote the leader's region_state:\n got %s\nwant %s", got, sentinel)
	}
}

// applyMoodDecayIfStale writes gm_mood as an absolute value derived from the
// caller's snapshot, so it must refuse a non-owner: a member running it against
// the leader's run holds the wrong lock and would drop a concurrent update.
func TestApplyMoodDecayIfStale_RefusesANonOwner(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@decay:example.org")
	seedZoneRun(t, "run-decay", owner)

	run, err := getActiveZoneRun(owner)
	if err != nil || run == nil {
		t.Fatalf("run: %v, %v", run, err)
	}
	// Drag the run far enough into the past that passive decay has something
	// to correct, then hand it to a non-owner.
	run.LastActionAt = time.Now().UTC().Add(-72 * time.Hour)
	run.DMMood = 90

	if err := applyMoodDecayIfStale(run, false); err != nil {
		t.Fatal(err)
	}
	if run.DMMood != 90 {
		t.Errorf("non-owner decay mutated the snapshot: mood = %d, want 90", run.DMMood)
	}
	fresh, err := getZoneRun("run-decay")
	if err != nil || fresh == nil {
		t.Fatalf("reload: %v, %v", fresh, err)
	}
	if fresh.DMMood != 50 {
		t.Errorf("non-owner decay wrote gm_mood = %d; the row should be untouched at its 50 default", fresh.DMMood)
	}

	// The owner still gets the correction — the guard narrows who writes, not what.
	if err := applyMoodDecayIfStale(run, true); err != nil {
		t.Fatal(err)
	}
	if run.DMMood == 90 {
		t.Error("owner decay did not apply; the guard should only exclude non-owners")
	}
}

// `extracting` is a seven-day resumable limbo: releaseParty is deliberately NOT
// called, so the roster survives and `!resume` brings the party back. A member is
// still seated for that whole window, and a guard keyed on status='active' would
// wave them into a private run that then wins every activeZoneRunFor lookup once
// the leader resumes.
func TestSeatedGuards_HoldThroughTheExtractingWindow(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "extract")
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET status = 'extracting' WHERE user_id = ?`, string(leader),
	); err != nil {
		t.Fatal(err)
	}

	// activeExpeditionFor goes blind here — that is the trap seatedExpeditionFor exists for.
	if exp, _, err := activeExpeditionFor(member); err != nil || exp != nil {
		t.Fatalf("activeExpeditionFor during extracting = %v, %v; want nil (status filter)", exp, err)
	}
	seated, err := seatedExpeditionFor(member)
	if err != nil || seated == nil {
		t.Fatalf("seatedExpeditionFor during extracting = %v, %v; the seat outlives the status", seated, err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: member}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if run, err := getActiveZoneRun(member); err != nil || run != nil {
		t.Fatalf("member opened a private run %v (err %v) while seated on an extracting party", run, err)
	}
}

// Mood belongs to the run, not to whoever typed at TwinBee. A member's taunt
// moves the gauge the whole party is walking under — before P6d it moved
// nothing, because the member resolved to no run at all.
func TestZoneMoodInteraction_MemberMovesThePartyMood(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "mood")

	before, err := getActiveZoneRun(leader)
	if err != nil || before == nil {
		t.Fatalf("leader's run: %v, %v", before, err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: member}, "taunt"); err != nil {
		t.Fatalf("taunt: %v", err)
	}

	after, err := getActiveZoneRun(leader)
	if err != nil || after == nil {
		t.Fatalf("leader's run after taunt: %v, %v", after, err)
	}
	if after.DMMood >= before.DMMood {
		t.Errorf("DM mood %d → %d; a member's taunt should have annoyed TwinBee",
			before.DMMood, after.DMMood)
	}
}
