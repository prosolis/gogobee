package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// newPartySimRunner wires a runner the way cmd/expedition-sim does, minus the
// temp DB (the caller has already called setupZoneRunTestDB).
func newPartySimRunner() *SimRunner {
	euro := &EuroPlugin{}
	return &SimRunner{P: &AdventurePlugin{euro: euro}, Euro: euro}
}

// seatPartyFixture builds a leader and n followers at the given level, banks
// them, and starts the leader's expedition. Returns the roster.
func seatPartyFixture(t *testing.T, s *SimRunner, tag string, followers int, bank float64) (id.UserID, []id.UserID) {
	t.Helper()
	leader := id.UserID("@" + tag + ":example")
	if _, err := s.BuildCharacter(leader, ClassFighter, 3); err != nil {
		t.Fatalf("build leader: %v", err)
	}
	s.Euro.Credit(leader, 1000, "test")
	var members []id.UserID
	for i := 0; i < followers; i++ {
		m := id.UserID("@" + tag + "-m" + string(rune('1'+i)) + ":example")
		if _, err := s.BuildCharacter(m, ClassFighter, 3); err != nil {
			t.Fatalf("build follower %d: %v", i, err)
		}
		s.Euro.Credit(m, bank, "test")
		members = append(members, m)
	}
	ctx := MessageContext{Sender: leader}
	if err := s.P.handleDnDExpeditionCmd(ctx, "start "+string(ZoneGoblinWarrens)+" heavy"); err != nil {
		t.Fatalf("start: %v", err)
	}
	return leader, members
}

// N3/P7: seatParty drives the real `!expedition invite` / `!expedition accept`
// pair, so a seated follower's packs land in the shared pool. Pooling raises
// Current *and* Max — supplyDepletion reads the ratio, so lifting only Current
// would read as the party starving the moment it set off (P6a).
func TestSeatParty_PoolsSuppliesAndSeatsEveryone(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()
	leader, members := seatPartyFixture(t, s, "sim-seat-ok", 2, 1000)
	defer cleanupExpeditions(leader)

	solo, err := getActiveExpedition(leader)
	if err != nil || solo == nil {
		t.Fatalf("expedition did not start: %v", err)
	}
	soloSU, soloMax := solo.Supplies.Current, solo.Supplies.Max

	if err := s.seatParty(leader, members); err != nil {
		t.Fatalf("seatParty: %v", err)
	}

	exp, _ := getActiveExpedition(leader)
	if exp == nil {
		t.Fatal("expedition vanished while seating")
	}
	size, err := partySize(exp.ID)
	if err != nil {
		t.Fatalf("partySize: %v", err)
	}
	if size != 3 {
		t.Fatalf("roster = %d, want 3 (leader + 2)", size)
	}
	// Three identical "heavy" purchases: the pool is their sum.
	if want := soloSU * 3; exp.Supplies.Current != want {
		t.Errorf("pooled Current = %v, want %v", exp.Supplies.Current, want)
	}
	if want := soloMax * 3; exp.Supplies.Max != want {
		t.Errorf("pooled Max = %v, want %v", exp.Supplies.Max, want)
	}
}

// A follower who cannot pay is refused by expeditionCmdAccept — which answers
// with a DM and a nil error. seatParty must not read that as a seat: a party
// run that quietly walked as a solo would report a T5 clear rate for a roster
// that never existed.
func TestSeatParty_RefusedFollowerHaltsTheRun(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()
	leader, members := seatPartyFixture(t, s, "sim-seat-broke", 1, 0)
	defer cleanupExpeditions(leader)

	err := s.seatParty(leader, members)
	if err == nil {
		t.Fatal("seatParty accepted a follower who could not afford a loadout")
	}
	if !strings.Contains(err.Error(), "roster seated 1 of 2") {
		t.Errorf("error = %q, want the roster-count refusal", err)
	}
}

// Regression: a party could never seat a follower into a Tier 6 post-game
// zone. expeditionCmdAccept ran the plain zoneOpenToLevel level gate first, and
// T6 zones are excluded from zonesForLevel by design, so every unlocked member
// was refused ("beyond you for now") before the post-game check could admit
// them. The intended party-only endgame was unreachable. The fix routes T6
// zones through postgameUnlocked instead of the level gate.
func TestSeatParty_PostgameZoneSeatsUnlockedMembers(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()

	leader := id.UserID("@sim-pg-ok:example")
	member := id.UserID("@sim-pg-ok-m1:example")
	for _, u := range []id.UserID{leader, member} {
		if _, err := s.BuildCharacter(u, ClassFighter, postgameLevelFloor+2); err != nil {
			t.Fatalf("build %s: %v", u, err)
		}
		s.Euro.Credit(u, 50000, "test")
		if err := s.SeedPostgameUnlock(u); err != nil {
			t.Fatalf("seed unlock %s: %v", u, err)
		}
	}
	defer cleanupExpeditions(leader)

	ctx := MessageContext{Sender: leader}
	if err := s.P.handleDnDExpeditionCmd(ctx, "start "+string(ZoneOssuaryAscendant)+" heavy"); err != nil {
		t.Fatalf("start T6 expedition: %v", err)
	}
	if err := s.seatParty(leader, []id.UserID{member}); err != nil {
		t.Fatalf("seatParty refused an unlocked member into a T6 zone: %v", err)
	}
	exp, _ := getActiveExpedition(leader)
	if exp == nil {
		t.Fatal("expedition vanished while seating")
	}
	if size, err := partySize(exp.ID); err != nil || size != 2 {
		t.Fatalf("roster = %d (%v), want 2 (leader + 1 unlocked member)", size, err)
	}
}

// The post-game gate must still refuse a member who has NOT beaten both T5
// bosses, even in a party: no smuggling an ungated seat into a T6 zone. The
// leader is unlocked (so the expedition starts); the member is not.
func TestSeatParty_PostgameZoneRefusesUngatedMember(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()

	leader := id.UserID("@sim-pg-gate:example")
	member := id.UserID("@sim-pg-gate-m1:example")
	for _, u := range []id.UserID{leader, member} {
		if _, err := s.BuildCharacter(u, ClassFighter, postgameLevelFloor+2); err != nil {
			t.Fatalf("build %s: %v", u, err)
		}
		s.Euro.Credit(u, 50000, "test")
	}
	// Only the leader clears the T5 gate; the member stays ungated.
	if err := s.SeedPostgameUnlock(leader); err != nil {
		t.Fatalf("seed leader unlock: %v", err)
	}
	defer cleanupExpeditions(leader)

	ctx := MessageContext{Sender: leader}
	if err := s.P.handleDnDExpeditionCmd(ctx, "start "+string(ZoneOssuaryAscendant)+" heavy"); err != nil {
		t.Fatalf("start T6 expedition: %v", err)
	}
	if err := s.seatParty(leader, []id.UserID{member}); err == nil {
		t.Fatal("seatParty admitted an ungated member into a T6 zone")
	}
}

// A misspelled -class / -party-classes token used to build a character at the
// unknown-class fallback — 1 HP — and the run reported a perfectly normal
// outcome for it. Nothing downstream treats an unknown class as an error, so
// BuildCharacter has to.
func TestBuildCharacter_RejectsUnknownClass(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()

	if _, err := s.BuildCharacter(id.UserID("@sim-badclass:example"), DnDClass("fightr"), 8); err == nil {
		t.Fatal("BuildCharacter accepted the class 'fightr'")
	}
	// The real thing still builds.
	if _, err := s.BuildCharacter(id.UserID("@sim-goodclass:example"), ClassFighter, 8); err != nil {
		t.Fatalf("BuildCharacter(fighter): %v", err)
	}
}

// simRunEndOutcome separates the three ways a run can be over. The middle case
// is the one N3/P7 exists to surface: inline room and patrol combat is fought
// by the leader alone, so a party reaches "run over" with a dead leader and a
// roster that never drew a weapon. Calling that a TPK would hide it.
func TestSimRunEndOutcome_DistinguishesLeaderDeathFromAWipe(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()

	mk := func(tag string) id.UserID {
		uid := id.UserID("@" + tag + ":example")
		if _, err := s.BuildCharacter(uid, ClassFighter, 3); err != nil {
			t.Fatalf("build %s: %v", tag, err)
		}
		return uid
	}
	leader, m1 := mk("sim-end-leader"), mk("sim-end-m1")
	roster := []id.UserID{leader, m1}

	if got := simRunEndOutcome(roster); got != "fled" {
		t.Errorf("everyone alive: got %q, want %q", got, "fled")
	}

	markAdventureDead(leader, "zone", "Test")
	if got := simRunEndOutcome(roster); got != "leader_down" {
		t.Errorf("leader dead, member standing: got %q, want %q", got, "leader_down")
	}
	// A solo roster has no members to survive the leader, so leader-dead is a
	// wipe — the label the balance corpus has always seen.
	if got := simRunEndOutcome(roster[:1]); got != "tpk" {
		t.Errorf("solo, dead: got %q, want %q", got, "tpk")
	}

	markAdventureDead(m1, "zone", "Test")
	if got := simRunEndOutcome(roster); got != "tpk" {
		t.Errorf("whole roster dead: got %q, want %q", got, "tpk")
	}
}

// The solo path must not touch the party layer at all: no invite, no roster
// row, RosterSize 1 all the way into the turn engine. This is the property the
// d8prereq_corpus baselines rest on.
func TestRunPartyExpedition_SoloWritesNoRoster(t *testing.T) {
	setupZoneRunTestDB(t)
	s := newPartySimRunner()
	uid := id.UserID("@sim-solo-noroster:example")
	if _, err := s.BuildCharacter(uid, ClassFighter, 3); err != nil {
		t.Fatalf("build: %v", err)
	}
	s.Euro.Credit(uid, 1000, "test")
	defer cleanupExpeditions(uid)

	// One walk is enough to prove the seating step was skipped; we care about
	// the roster, not the outcome.
	res, err := s.RunPartyExpedition(uid, nil, ZoneGoblinWarrens, 1, 0)
	if err != nil {
		t.Fatalf("RunPartyExpedition: %v", err)
	}
	if res.PartySize != 1 {
		t.Errorf("PartySize = %d, want 1", res.PartySize)
	}
	if len(res.Members) != 0 {
		t.Errorf("Members = %v, want empty for a solo run", res.Members)
	}
	if id := mostRecentExpeditionID(uid); id != "" {
		size, err := partySize(id)
		if err != nil {
			t.Fatalf("partySize: %v", err)
		}
		if size != 0 {
			t.Errorf("solo run seated a roster of %d, want no rows at all", size)
		}
	}
}
