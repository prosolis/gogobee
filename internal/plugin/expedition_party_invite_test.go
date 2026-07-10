package plugin

import (
	"errors"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func TestInvite_SeatsLeaderAndRecordsTheAsk(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	invitee := id.UserID("@friend:example.org")
	seedExpedition(t, "exp-1", leader, "active")

	if err := inviteToParty("exp-1", invitee, leader); err != nil {
		t.Fatal(err)
	}

	// The leader is seated even though nobody has accepted — a roster whose
	// leader is missing drops them from every fan-out.
	members, err := partyMembers("exp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || !members[0].IsLeader() {
		t.Fatalf("roster = %v, want the leader alone", members)
	}

	inv, err := latestInviteFor(invitee)
	if err != nil {
		t.Fatal(err)
	}
	if inv.ExpeditionID != "exp-1" || inv.InvitedBy != string(leader) {
		t.Errorf("invite = %+v", inv)
	}
}

func TestInvite_RefusesADuplicateAsk(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	invitee := id.UserID("@friend:example.org")
	seedExpedition(t, "exp-1", leader, "active")

	if err := inviteToParty("exp-1", invitee, leader); err != nil {
		t.Fatal(err)
	}
	if err := inviteToParty("exp-1", invitee, leader); !errors.Is(err, ErrAlreadyInvited) {
		t.Errorf("second invite err = %v, want ErrAlreadyInvited", err)
	}
}

// Outstanding invites count against the ceiling. Otherwise a leader asks four
// people, three accept, and the roster overflows expeditionPartyMax.
func TestInvite_PendingInvitesCountAgainstTheCeiling(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")

	// Leader + 2 invites == expeditionPartyMax (3).
	for _, u := range []id.UserID{"@a:example.org", "@b:example.org"} {
		if err := inviteToParty("exp-1", u, leader); err != nil {
			t.Fatalf("invite %s: %v", u, err)
		}
	}
	if err := inviteToParty("exp-1", "@c:example.org", leader); !errors.Is(err, ErrPartyFull) {
		t.Errorf("third invite err = %v, want ErrPartyFull", err)
	}
}

func TestInvite_RefusesAPlayerWhoIsAlreadyAdventuring(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	busy := id.UserID("@busy:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	seedExpedition(t, "exp-2", busy, "active")

	if err := inviteToParty("exp-1", busy, leader); !errors.Is(err, ErrPlayerBusyElsewhere) {
		t.Errorf("invite err = %v, want ErrPlayerBusyElsewhere", err)
	}
}

func TestInvite_ExpiresPastTTL(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	invitee := id.UserID("@friend:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	if err := inviteToParty("exp-1", invitee, leader); err != nil {
		t.Fatal(err)
	}

	stale := time.Now().UTC().Add(-2 * expeditionInviteTTL)
	if _, err := db.Get().Exec(
		`UPDATE expedition_invite SET invited_at = ? WHERE user_id = ?`, stale, string(invitee)); err != nil {
		t.Fatal(err)
	}

	if _, err := latestInviteFor(invitee); !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("stale invite err = %v, want ErrInviteNotFound", err)
	}
	// And the seat it was holding is free again.
	if err := inviteToParty("exp-1", "@a:example.org", leader); err != nil {
		t.Errorf("expired invite still holds a seat: %v", err)
	}
}

func TestPurgeExpiredInvites_ReclaimsOnlyStaleRows(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	for _, u := range []id.UserID{"@fresh:example.org", "@stale:example.org"} {
		if err := inviteToParty("exp-1", u, leader); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Get().Exec(`UPDATE expedition_invite SET invited_at = ? WHERE user_id = ?`,
		time.Now().UTC().Add(-2*expeditionInviteTTL), "@stale:example.org"); err != nil {
		t.Fatal(err)
	}

	purgeExpiredInvites()

	var n int
	if err := db.Get().QueryRow(`SELECT COUNT(*) FROM expedition_invite`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("invite rows = %d, want 1 (the fresh one)", n)
	}
}

// ── the invite window ────────────────────────────────────────────────────────

func TestInviteWindowOpen(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    Expedition
		want bool
	}{
		{"day 1, active", Expedition{Status: ExpeditionStatusActive, CurrentDay: 1}, true},
		{"day 2", Expedition{Status: ExpeditionStatusActive, CurrentDay: 2}, false},
		{"boss down", Expedition{Status: ExpeditionStatusActive, CurrentDay: 1, BossDefeated: true}, false},
		{"extracting", Expedition{Status: ExpeditionStatusExtracting, CurrentDay: 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inviteWindowOpen(&tc.e); got != tc.want {
				t.Errorf("inviteWindowOpen = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── autopilot suppression ────────────────────────────────────────────────────

// The invite window is 30 minutes of autopilot grace; an unanswered invite has
// to hold the expedition still, or the leader walks off without their friend.
func TestAutoRun_PendingInviteHoldsTheExpedition(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	// Age the expedition past autoRunMinExpeditionAge so only the invite can
	// be what holds it.
	old := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = 'exp-1'`, old); err != nil {
		t.Fatal(err)
	}
	runCutoff := time.Now().UTC()
	ageCutoff := time.Now().UTC().Add(-autoRunMinExpeditionAge)

	ids, err := loadExpeditionsForAutoRun(runCutoff, ageCutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("without an invite, autorun sees %v; want [exp-1]", ids)
	}

	if err := inviteToParty("exp-1", "@friend:example.org", leader); err != nil {
		t.Fatal(err)
	}
	ids, err = loadExpeditionsForAutoRun(runCutoff, ageCutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("autopilot walked an expedition with a pending invite: %v", ids)
	}
}

// The hold is bounded: a forgotten invite must not freeze an expedition forever.
func TestAutoRun_ExpiredInviteReleasesTheExpedition(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	old := time.Now().UTC().Add(-4 * time.Hour)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET start_date = ? WHERE expedition_id = 'exp-1'`, old); err != nil {
		t.Fatal(err)
	}
	if err := inviteToParty("exp-1", "@friend:example.org", leader); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(`UPDATE expedition_invite SET invited_at = ?`,
		time.Now().UTC().Add(-2*expeditionInviteTTL)); err != nil {
		t.Fatal(err)
	}

	ids, err := loadExpeditionsForAutoRun(time.Now().UTC(), time.Now().UTC().Add(-autoRunMinExpeditionAge))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Errorf("expired invite still pins the autopilot: %v", ids)
	}
}

// ── terminal cleanup ─────────────────────────────────────────────────────────

// An invite outliving its expedition would let someone accept onto a corpse.
func TestReleaseParty_ClearsPendingInvites(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	invitee := id.UserID("@friend:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	if err := inviteToParty("exp-1", invitee, leader); err != nil {
		t.Fatal(err)
	}

	if err := completeExpedition("exp-1", ExpeditionStatusComplete); err != nil {
		t.Fatal(err)
	}

	if _, err := latestInviteFor(invitee); !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("invite survived expedition completion: %v", err)
	}
}

// ── the supply pool ──────────────────────────────────────────────────────────

// Both Current and Max rise: supplyDepletion reads the ratio, and a member
// arriving with a full pack must not read as the party suddenly starving.
func TestAddSupplyPurchase_PoolsBothCurrentAndMax(t *testing.T) {
	base := makeSupplies(ZoneTier(1), SupplyPurchase{StandardPacks: 2})
	before := supplyDepletion(base)

	joined := addSupplyPurchase(base, SupplyPurchase{StandardPacks: 2})

	if joined.Current != base.Current*2 || joined.Max != base.Max*2 {
		t.Errorf("pooled = %.1f/%.1f, want %.1f/%.1f",
			joined.Current, joined.Max, base.Current*2, base.Max*2)
	}
	if joined.PacksStandard != 4 {
		t.Errorf("PacksStandard = %d, want 4", joined.PacksStandard)
	}
	if got := supplyDepletion(joined); got != before {
		t.Errorf("depletion state moved on join: %v -> %v", before, got)
	}
	// The burn rate is the zone's, not the party's — P6c scales it by size.
	if joined.DailyBurn != base.DailyBurn {
		t.Errorf("DailyBurn = %v, want %v", joined.DailyBurn, base.DailyBurn)
	}
}
