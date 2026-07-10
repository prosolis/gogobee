package plugin

import (
	"errors"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// seedExpedition inserts the minimum dnd_expedition row the party layer reads:
// the owner, the status, and enough non-null columns for scanExpedition.
func seedExpedition(t *testing.T, expeditionID string, owner id.UserID, status string) {
	t.Helper()
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, run_id, status, start_date)
		VALUES (?, ?, 'goblin_warrens', ?, ?, ?)`,
		expeditionID, string(owner), "run-"+expeditionID, status, time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
}

// seatLeaderFixture seats the expedition's leader the way the invite path does —
// through seatLeader in its own transaction — reading the owner off the row that
// seedExpedition already wrote. Replaces the retired openParty helper.
func seatLeaderFixture(t *testing.T, expeditionID string) {
	t.Helper()
	tx, err := db.Get().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := seatLeader(tx, expeditionID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("seatLeader %s: %v", expeditionID, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestParty_SoloExpeditionHasNoRoster(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", owner, "active")

	members, err := partyMembers("exp-solo")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Errorf("solo expedition has %d roster rows, want 0", len(members))
	}
	// Absent must read as "one player", not "zero players" — every fan-out seam
	// loops over this.
	if n, err := partySize("exp-solo"); err != nil || n != 1 {
		t.Errorf("partySize = %d (%v), want 1", n, err)
	}
	ids, err := partyMemberIDs("exp-solo", string(owner))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != owner {
		t.Errorf("partyMemberIDs = %v, want [%s]", ids, owner)
	}
}

func TestParty_OpenIsIdempotent(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", owner, "active")

	// seatLeader is idempotent — a second call on the same expedition is a no-op.
	for i := 0; i < 2; i++ {
		seatLeaderFixture(t, "exp-1")
	}
	members, err := partyMembers("exp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || !members[0].IsLeader() {
		t.Fatalf("roster = %+v, want one leader", members)
	}
}

func TestParty_JoinSeatsMembersLeaderFirst(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead2:example.org")
	seedExpedition(t, "exp-2", owner, "active")
	seatLeaderFixture(t, "exp-2")
	for _, u := range []id.UserID{"@b:x", "@c:x"} {
		if err := joinParty("exp-2", u); err != nil {
			t.Fatalf("joinParty %s: %v", u, err)
		}
	}

	members, err := partyMembers("exp-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("roster = %d, want 3", len(members))
	}
	if !members[0].IsLeader() || members[0].UserID != string(owner) {
		t.Errorf("leader is not first: %+v", members)
	}
	if n, err := partySize("exp-2"); err != nil || n != 3 {
		t.Errorf("partySize = %d (%v), want 3", n, err)
	}
}

// An invite that lands before anyone called openParty must still produce a
// roster with the leader on it — partyMemberIDs reads the roster, not the
// expedition row, so a leaderless party would silently stop DMing the leader.
func TestParty_JoinSeatsTheLeaderIfMissing(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead-implicit:example.org")
	seedExpedition(t, "exp-implicit", owner, "active")

	// No openParty call at all.
	if err := joinParty("exp-implicit", "@b:x"); err != nil {
		t.Fatal(err)
	}
	members, err := partyMembers("exp-implicit")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("roster = %+v, want leader + member", members)
	}
	if !members[0].IsLeader() || members[0].UserID != string(owner) {
		t.Errorf("leader missing from roster: %+v", members)
	}
	// And the leader cannot then be double-seated as a member.
	if err := joinParty("exp-implicit", owner); !errors.Is(err, ErrAlreadyInParty) {
		t.Errorf("leader join: err = %v, want ErrAlreadyInParty", err)
	}
}

func TestParty_JoinRefusesFullRoster(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead3:example.org")
	seedExpedition(t, "exp-3", owner, "active")
	seatLeaderFixture(t, "exp-3")
	for _, u := range []id.UserID{"@b:x", "@c:x"} {
		if err := joinParty("exp-3", u); err != nil {
			t.Fatal(err)
		}
	}
	// Leader + 2 == expeditionPartyMax.
	if err := joinParty("exp-3", "@d:x"); !errors.Is(err, ErrPartyFull) {
		t.Errorf("4th seat: err = %v, want ErrPartyFull", err)
	}
}

func TestParty_JoinRefusesDuplicate(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead4:example.org")
	seedExpedition(t, "exp-4", owner, "active")
	seatLeaderFixture(t, "exp-4")
	if err := joinParty("exp-4", "@b:x"); err != nil {
		t.Fatal(err)
	}
	if err := joinParty("exp-4", "@b:x"); !errors.Is(err, ErrAlreadyInParty) {
		t.Errorf("re-join: err = %v, want ErrAlreadyInParty", err)
	}
	// The leader is already seated, so inviting them back is a duplicate too.
	if err := joinParty("exp-4", owner); !errors.Is(err, ErrAlreadyInParty) {
		t.Errorf("leader re-join: err = %v, want ErrAlreadyInParty", err)
	}
}

// "One active expedition per user" is code-enforced for owners
// (startExpedition). Membership must not be a way around it: a player who owns
// a run, or already rides someone else's, cannot take a third seat.
func TestParty_JoinRefusesPlayerBusyElsewhere(t *testing.T) {
	setupEmptyTestDB(t)
	seedExpedition(t, "exp-5", "@lead5:example.org", "active")
	seedExpedition(t, "exp-6", "@lead6:example.org", "active")
	seatLeaderFixture(t, "exp-5")
	seatLeaderFixture(t, "exp-6")

	// Owns their own active expedition.
	seedExpedition(t, "exp-own", "@busy:example.org", "active")
	if err := joinParty("exp-5", "@busy:example.org"); !errors.Is(err, ErrPlayerBusyElsewhere) {
		t.Errorf("owner of another run: err = %v, want ErrPlayerBusyElsewhere", err)
	}

	// Already seated on another party.
	if err := joinParty("exp-5", "@rider:example.org"); err != nil {
		t.Fatal(err)
	}
	if err := joinParty("exp-6", "@rider:example.org"); !errors.Is(err, ErrPlayerBusyElsewhere) {
		t.Errorf("rider of another party: err = %v, want ErrPlayerBusyElsewhere", err)
	}

	// A leader is seated on their own party, so they are busy too.
	if err := joinParty("exp-6", "@lead5:example.org"); !errors.Is(err, ErrPlayerBusyElsewhere) {
		t.Errorf("leader of another party: err = %v, want ErrPlayerBusyElsewhere", err)
	}
}

func TestParty_LeaveRemovesMemberButNotLeader(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead7:example.org")
	seedExpedition(t, "exp-7", owner, "active")
	seatLeaderFixture(t, "exp-7")
	if err := joinParty("exp-7", "@b:x"); err != nil {
		t.Fatal(err)
	}

	if err := leaveParty("exp-7", "@b:x"); err != nil {
		t.Fatalf("leaveParty: %v", err)
	}
	if n, _ := partySize("exp-7"); n != 1 {
		t.Errorf("after leave, partySize = %d, want 1", n)
	}
	// The expedition row hangs off the leader; they extract, they don't leave.
	if err := leaveParty("exp-7", owner); !errors.Is(err, ErrNotPartyLeader) {
		t.Errorf("leader leave: err = %v, want ErrNotPartyLeader", err)
	}
	// Leaving frees the member for their own next run.
	seedExpedition(t, "exp-7b", "@lead7b:example.org", "active")
	seatLeaderFixture(t, "exp-7b")
	if err := joinParty("exp-7b", "@b:x"); err != nil {
		t.Errorf("departed member could not rejoin elsewhere: %v", err)
	}
}

func TestParty_DisbandFreesEveryone(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead8:example.org")
	seedExpedition(t, "exp-8", owner, "complete")
	seatLeaderFixture(t, "exp-8")
	if err := joinParty("exp-8", "@b:x"); err != nil {
		t.Fatal(err)
	}
	if err := disbandParty("exp-8"); err != nil {
		t.Fatal(err)
	}
	if n, _ := partySize("exp-8"); n != 1 {
		t.Errorf("after disband, partySize = %d, want 1 (no rows)", n)
	}
	seedExpedition(t, "exp-8b", "@lead8b:example.org", "active")
	seatLeaderFixture(t, "exp-8b")
	if err := joinParty("exp-8b", "@b:x"); err != nil {
		t.Errorf("disbanded member still looks busy: %v", err)
	}
}

// getActiveExpedition keys on dnd_expedition.user_id, so it is blind to party
// members. activeExpeditionFor is the lookup every player-facing command wants.
func TestParty_ActiveExpeditionForResolvesBothRoles(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lead9:example.org")
	member := id.UserID("@member9:example.org")
	seedExpedition(t, "exp-9", owner, "active")
	seatLeaderFixture(t, "exp-9")
	if err := joinParty("exp-9", member); err != nil {
		t.Fatal(err)
	}

	// The member owns no row, so the legacy lookup misses them entirely.
	if e, err := getActiveExpedition(member); err != nil || e != nil {
		t.Errorf("getActiveExpedition found a row for a member: %v / %v", e, err)
	}

	e, isLeader, err := activeExpeditionFor(owner)
	if err != nil || e == nil || !isLeader || e.ID != "exp-9" {
		t.Errorf("leader lookup: %+v leader=%v err=%v", e, isLeader, err)
	}
	e, isLeader, err = activeExpeditionFor(member)
	if err != nil || e == nil || isLeader || e.ID != "exp-9" {
		t.Errorf("member lookup: %+v leader=%v err=%v", e, isLeader, err)
	}
	// The member resolves to the leader's expedition, clock and all.
	if e != nil && e.UserID != string(owner) {
		t.Errorf("member resolved to expedition owned by %q, want %q", e.UserID, owner)
	}

	// A stranger is on no expedition at all.
	e, _, err = activeExpeditionFor("@nobody:example.org")
	if err != nil || e != nil {
		t.Errorf("stranger lookup: %+v / %v", e, err)
	}
}
