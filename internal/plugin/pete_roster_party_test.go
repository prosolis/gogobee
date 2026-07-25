package plugin

import (
	"testing"
	"time"

	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// TestSeatedMemberIsNotIdleInTown is the gap W7 closes. The board resolved an
// expedition with getActiveExpedition, which keys on dnd_expedition.user_id — so
// a party member, who owns no row of their own, read as "idle in town" while
// standing in a dungeon. The regression is silent: the page renders fine, it just
// says the wrong thing about where somebody is.
func TestSeatedMemberIsNotIdleInTown(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-30 * time.Hour)

	leader := id.UserID("@leader:test")
	member := id.UserID("@member:test")
	seedRosterPlayer(t, leader, "Josie", &old, &old)
	seedRosterPlayer(t, member, "Camcast", &old, &old)

	seedExpedition(t, "exp-shared", leader, "active")
	seatLeaderFixture(t, "exp-shared")
	if err := joinParty("exp-shared", member); err != nil {
		t.Fatalf("joinParty: %v", err)
	}

	snap, err := buildRosterSnapshot(now, nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	byName := map[string]int{}
	for i, a := range snap.Adventurers {
		byName[a.Name] = i
	}
	for _, name := range []string{"Josie", "Camcast"} {
		i, ok := byName[name]
		if !ok {
			t.Fatalf("%s is not on the board", name)
		}
		if got := snap.Adventurers[i].Status; got != "expedition" {
			t.Errorf("%s status = %q, want expedition", name, got)
		}
		if snap.Adventurers[i].Zone == "" {
			t.Errorf("%s is on an expedition with no zone named", name)
		}
	}
}

// TestPartySeatsNameTheWholeRoster covers the shape of the seat list: leader
// first, every human named with a linkable token, and the hireling named without
// one (he has no board row to link to).
func TestPartySeatsNameTheWholeRoster(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-30 * time.Hour)

	leader := id.UserID("@leader:test")
	member := id.UserID("@member:test")
	seedRosterPlayer(t, leader, "Josie", &old, &old)
	seedRosterPlayer(t, member, "Camcast", &old, &old)

	seedExpedition(t, "exp-shared", leader, "active")
	seatLeaderFixture(t, "exp-shared")
	if err := joinParty("exp-shared", member); err != nil {
		t.Fatalf("joinParty: %v", err)
	}
	if err := joinParty("exp-shared", companionUserID()); err != nil {
		t.Fatalf("hire companion: %v", err)
	}

	seats := seatsForOwner(t, now, "Josie")
	if len(seats) != 3 {
		t.Fatalf("party has %d seats, want 3: %+v", len(seats), seats)
	}
	if seats[0].Kind != "leader" || seats[0].Name != "Josie" {
		t.Errorf("first seat = %+v, want the leader Josie", seats[0])
	}
	if seats[0].Token == "" || seats[0].Level == 0 {
		t.Errorf("leader seat is unlinkable or levelless: %+v", seats[0])
	}
	var companion, human int
	for _, s := range seats {
		switch s.Kind {
		case "companion":
			companion++
			if s.Name != companionDisplayName {
				t.Errorf("companion seat named %q, want %q", s.Name, companionDisplayName)
			}
			if s.Token != "" {
				t.Errorf("companion seat carries a board token %q; he has no board row", s.Token)
			}
		case "leader", "member":
			human++
			if s.Name == "" || s.Token == "" {
				t.Errorf("human seat %+v is missing its name/token pair", s)
			}
		default:
			t.Errorf("unknown seat kind %q", s.Kind)
		}
	}
	if companion != 1 || human != 2 {
		t.Errorf("seats = %d human + %d companion, want 2 + 1", human, companion)
	}
}

// TestSoloRunPublishesNoParty: expeditionParty always hands back at least the
// leader, so a naive render would draw every solo player a party of one.
func TestSoloRunPublishesNoParty(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-30 * time.Hour)

	solo := id.UserID("@solo:test")
	seedRosterPlayer(t, solo, "Josie", &old, &old)
	seedExpedition(t, "exp-solo", solo, "active")

	if seats := seatsForOwner(t, now, "Josie"); seats != nil {
		t.Errorf("solo run published a party of %d: %+v", len(seats), seats)
	}

	// And with only the leader seated, which is what the roster table looks like
	// between materialising and the first invite landing.
	seatLeaderFixture(t, "exp-solo")
	if seats := seatsForOwner(t, now, "Josie"); seats != nil {
		t.Errorf("leader-only roster published a party of %d: %+v", len(seats), seats)
	}
}

// TestOptedOutSeatIsAnonymisedNotDropped is the privacy contract for this
// surface, and it is deliberately NOT the board's rule. The board omits an
// opted-out player outright; a party seat is anonymised, because a party of three
// that renders as a pair is a false statement about the run everyone can see the
// supply burn and threat level of.
func TestOptedOutSeatIsAnonymisedNotDropped(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-30 * time.Hour)

	leader := id.UserID("@leader:test")
	hidden := id.UserID("@hidden:test")
	seedRosterPlayer(t, leader, "Josie", &old, &old)
	seedRosterPlayer(t, hidden, "Quack", &old, &old)
	setNewsOptout(hidden, true)

	seedExpedition(t, "exp-shared", leader, "active")
	seatLeaderFixture(t, "exp-shared")
	if err := joinParty("exp-shared", hidden); err != nil {
		t.Fatalf("joinParty: %v", err)
	}

	seats := seatsForOwner(t, now, "Josie")
	if len(seats) != 2 {
		t.Fatalf("party has %d seats, want 2 — an opted-out seat was dropped, not anonymised: %+v",
			len(seats), seats)
	}
	for _, s := range seats {
		if s.Name == "Quack" {
			t.Error("an opted-out player is named on a party roster")
		}
	}
	var blank int
	for _, s := range seats {
		if s.Name == "" {
			blank++
			if s.Token != "" || s.Level != 0 {
				t.Errorf("anonymised seat still carries a token or level: %+v", s)
			}
		}
	}
	if blank != 1 {
		t.Errorf("%d anonymous seats, want exactly 1", blank)
	}
}

// seatsForOwner pulls one named adventurer's published party out of a whole
// snapshot, which is the only way to reach it — Party rides RosterDetail, so this
// also proves the wiring in buildRosterSnapshot and not just partySeatViews.
func seatsForOwner(t *testing.T, now time.Time, name string) []peteclient.PartySeatView {
	t.Helper()
	snap, err := buildRosterSnapshot(now, nil)
	if err != nil {
		t.Fatalf("buildRosterSnapshot: %v", err)
	}
	for _, a := range snap.Adventurers {
		if a.Name == name && a.Detail != nil {
			return a.Detail.Party
		}
	}
	t.Fatalf("%s is not on the board with a detail sheet", name)
	return nil
}
