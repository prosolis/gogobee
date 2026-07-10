package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// N3/P6c — seating the roster, and paying for it.
//
// The invariant every test here defends is the same one: a solo player must be
// unable to tell that parties exist. The seat-0 owner, the burn rate, and the
// opening block are all shapes a solo fight has already written thousands of
// times, and the balance corpus is the receipt.

// ── who owns the fight ───────────────────────────────────────────────────────

// roster[0] is the fight's owner: its lock key and its session row.

// A bare zone run — no expedition row anywhere — must not fall over looking for
// a leader. This is the `!zone enter` path, which predates expeditions entirely.
func TestFightRoster_BareZoneRunOwnsItsOwnFight(t *testing.T) {
	setupEmptyTestDB(t)
	wanderer := id.UserID("@wanderer:example.org")

	if got := fightRoster(wanderer); len(got) != 1 || got[0] != wanderer {
		t.Errorf("fightRoster(wanderer) = %v, want just the player themselves", got)
	}
}

// The reason the lookup exists: a member's `!fight` opens the *leader's* fight,
// under the leader's lock and on the leader's session row.
func TestFightRoster_MembersFightBelongsToTheLeader(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}

	for _, who := range []id.UserID{member, leader} {
		if got := fightRoster(who)[0]; got != leader {
			t.Errorf("fightRoster(%s)[0] = %q, want the leader %q", who, got, leader)
		}
	}
}

// ── who sits down ────────────────────────────────────────────────────────────

func TestFightRoster_SoloSeatsExactlyOne(t *testing.T) {
	setupEmptyTestDB(t)
	solo := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", solo, "active")

	roster := fightRoster(solo)
	if len(roster) != 1 || roster[0] != solo {
		t.Fatalf("solo roster = %v, want [%s]", roster, solo)
	}
}

// Seat 0 is the leader whoever typed `!fight`. Every seat-0 invariant in the
// combat layer — the lock key, the once-only close-out effects, the leader-only
// `!flee` — reads the roster's head, so a member-first ordering would flee the
// leader's run on a member's say-so.
func TestFightRoster_LeaderIsAlwaysSeatZero(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	if err := joinParty("exp-1", member); err != nil {
		t.Fatal(err)
	}

	for _, who := range []id.UserID{leader, member} {
		roster := fightRoster(who)
		if len(roster) != 2 {
			t.Fatalf("fightRoster(%s) = %v, want 2 seats", who, roster)
		}
		if roster[0] != leader {
			t.Errorf("fightRoster(%s) seats %q at 0, want the leader", who, roster[0])
		}
	}
}

// ── who actually gets a seat ─────────────────────────────────────────────────

// fightTestChar creates the full character stack buildZoneCombatants reads:
// the adventure character, then the D&D sheet its HP snapshot comes from.
func fightTestChar(t *testing.T, uid id.UserID, hp int) {
	t.Helper()
	if err := createAdvCharacter(uid, string(uid)); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 14, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: hp, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFightSeats_SoloSeatsExactlyThePlayer(t *testing.T) {
	setupEmptyTestDB(t)
	solo := id.UserID("@solo:example.org")
	fightTestChar(t, solo, 30)

	seats, enemy, skip, refusal := (&AdventurePlugin{}).buildFightSeats(
		solo, []id.UserID{solo}, dndBestiary["goblin"], 1, 0)
	if refusal != "" || skip != "" {
		t.Fatalf("solo fight refused: %s / %s", refusal, skip)
	}
	if len(seats) != 1 || seats[0].UserID != solo {
		t.Fatalf("seats = %+v, want exactly the solo player", seats)
	}
	if seats[0].HP != 30 || seats[0].HPMax != 30 {
		t.Errorf("seat HP = %d/%d, want 30/30", seats[0].HP, seats[0].HPMax)
	}
	if seats[0].C == nil || seats[0].C.Name == "" {
		t.Errorf("seat carries no built combatant: %+v", seats[0])
	}
	if enemy == nil || enemy.Name == "" || enemy.Stats.MaxHP <= 0 {
		t.Errorf("enemy not built: %+v", enemy)
	}
}

// A downed member is not a blocked party: they sit the fight out, and the seats
// that remain still line up leader-first.
func TestBuildFightSeats_DownedMemberSitsOut(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	standing := id.UserID("@standing:example.org")
	downed := id.UserID("@downed:example.org")
	fightTestChar(t, leader, 30)
	fightTestChar(t, standing, 25)
	fightTestChar(t, downed, 0)

	roster := []id.UserID{leader, downed, standing}
	seats, _, skip, refusal := (&AdventurePlugin{}).buildFightSeats(
		leader, roster, dndBestiary["goblin"], 1, 0)
	if refusal != "" {
		t.Fatalf("party refused over a downed member: %s", refusal)
	}
	if skip != "" {
		t.Errorf("the leader was seated, so nothing was skipped on their behalf: %q", skip)
	}
	if len(seats) != 2 {
		t.Fatalf("seated %d, want the leader and the standing member", len(seats))
	}
	if seats[0].UserID != leader || seats[1].UserID != standing {
		t.Errorf("seats = [%s %s], want [leader standing]", seats[0].UserID, seats[1].UserID)
	}

	// The one who was left behind typed `!fight` too, and silence is not an answer.
	_, _, skip, refusal = (&AdventurePlugin{}).buildFightSeats(
		downed, roster, dndBestiary["goblin"], 1, 0)
	if refusal != "" {
		t.Fatalf("a downed member must not refuse the party's fight: %s", refusal)
	}
	if !strings.Contains(skip, "`!rest`") {
		t.Errorf("the downed member should be told to rest, got %q", skip)
	}
}

// Seat 0 is not optional, and the copy depends on who is reading it.
func TestBuildFightSeats_DownedLeaderRefusesTheFightForEveryone(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	member := id.UserID("@member:example.org")
	fightTestChar(t, leader, 0)
	fightTestChar(t, member, 25)

	roster := []id.UserID{leader, member}
	p := &AdventurePlugin{}

	seats, _, _, refusal := p.buildFightSeats(leader, roster, dndBestiary["goblin"], 1, 0)
	if len(seats) != 0 || refusal == "" {
		t.Fatalf("downed leader seated %d players, refusal %q", len(seats), refusal)
	}
	if !strings.Contains(refusal, "`!rest`") {
		t.Errorf("the leader should be told to rest, got %q", refusal)
	}

	_, _, _, refusal = p.buildFightSeats(member, roster, dndBestiary["goblin"], 1, 0)
	if !strings.Contains(refusal, "leader") {
		t.Errorf("the member should be told it is the leader holding things up, got %q", refusal)
	}
	if strings.Contains(refusal, "`!rest`") {
		t.Errorf("the member cannot rest on the leader's behalf, got %q", refusal)
	}
}

// ── the opening block ────────────────────────────────────────────────────────

// initiativeNames walks the turn order, which carries the enemy as sentinel seat
// -1. Indexing players with it would panic on every party's first `!fight`.
func TestInitiativeNames_RendersTheEnemySentinelNotAPanic(t *testing.T) {
	players, enemy := biasedParty()
	sess := partySession(CombatPhasePlayerTurn, 100, 100, 100)

	names := initiativeNames(players, sess, enemy)
	if len(names) != 4 {
		t.Fatalf("order = %v, want 3 players + the enemy", names)
	}
	// biasedParty stacks initiative 300/200/100 against a speed-1 enemy.
	if want := []string{"Ada", "Bram", "Cass", enemy.Name}; strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", names, want)
	}
}

func TestInitiativeNames_SoloIsPlayerThenEnemy(t *testing.T) {
	p := basePlayer()
	e := baseEnemy()
	sess := turnSession(CombatPhasePlayerTurn, 100, 100)

	if got := initiativeNames([]*Combatant{&p}, sess, &e); len(got) != 2 || got[1] != e.Name {
		t.Errorf("solo order = %v, want [player, enemy]", got)
	}
}

// ── what the party eats ──────────────────────────────────────────────────────

// The rate a solo expedition burns at is a tuned constant that the whole
// difficulty corpus (Phase 3-B / 5-B) was measured against. It must come out of
// the party-aware path untouched.
func TestExpeditionBurnRatePct_SoloIsTheShippedRate(t *testing.T) {
	setupEmptyTestDB(t)
	solo := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", solo, "active")

	if got := expeditionBurnRatePct("exp-solo"); got != phase5BDailyBurnRatePct {
		t.Errorf("solo burn rate = %d, want the shipped %d", got, phase5BDailyBurnRatePct)
	}
	// An expedition with no roster row at all is the pre-N3 world: same answer.
	if got := expeditionBurnRatePct("exp-never-seen"); got != phase5BDailyBurnRatePct {
		t.Errorf("rosterless burn rate = %d, want the shipped %d", got, phase5BDailyBurnRatePct)
	}
}

// N × 0.8: a party eats more than one player and less than N of them.
func TestExpeditionBurnRatePct_PartyEatsMoreButNotProRata(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")

	want := map[int]int{1: 50, 2: 80, 3: 120}
	for seat := 1; seat <= 2; seat++ {
		if err := joinParty("exp-1", id.UserID(memberID(seat))); err != nil {
			t.Fatal(err)
		}
		size := seat + 1
		if got := expeditionBurnRatePct("exp-1"); got != want[size] {
			t.Errorf("party of %d burns at %d%%, want %d%%", size, got, want[size])
		}
	}
}

// Bit-identity, not approximate agreement: a solo expedition's supplies snapshot
// after the party-aware burn must equal the one applyDailyBurn produced.
func TestApplyExpeditionDailyBurn_SoloMatchesApplyDailyBurnExactly(t *testing.T) {
	setupEmptyTestDB(t)
	solo := id.UserID("@solo:example.org")
	seedExpedition(t, "exp-solo", solo, "active")

	supplies := ExpeditionSupplies{Current: 40, Max: 40, DailyBurn: 3, HarshMod: 1.5}
	e := &Expedition{ID: "exp-solo", UserID: string(solo), Supplies: supplies}

	for _, tc := range []struct{ harsh, siege bool }{{false, false}, {true, false}, {false, true}} {
		wantS, wantBurn := applyDailyBurn(supplies, tc.harsh, tc.siege)
		gotS, gotBurn := applyExpeditionDailyBurn(e, tc.harsh, tc.siege)
		if gotBurn != wantBurn || gotS != wantS {
			t.Errorf("harsh=%v siege=%v: got (%v, %g), want (%v, %g)",
				tc.harsh, tc.siege, gotS, gotBurn, wantS, wantBurn)
		}
	}
}

func TestApplyExpeditionDailyBurn_PartyOfThreeBurnsTwoPointFourShares(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-1", leader, "active")
	for seat := 1; seat <= 2; seat++ {
		if err := joinParty("exp-1", id.UserID(memberID(seat))); err != nil {
			t.Fatal(err)
		}
	}

	supplies := ExpeditionSupplies{Current: 40, Max: 40, DailyBurn: 3, HarshMod: 1}
	e := &Expedition{ID: "exp-1", UserID: string(leader), Supplies: supplies}

	_, solo := applyDailyBurn(supplies, false, false)
	_, party := applyExpeditionDailyBurn(e, false, false)

	// DailyBurn 3 at the party-of-3 rate of 120% — exactly 2.4 solo shares, and
	// exactly the value the int rate yields. A float32 0.8 would land at 3.6000001.
	if want := float32(3.6); party != want {
		t.Errorf("party of 3 burned %v SU, want %v", party, want)
	}
	if party >= solo*3 {
		t.Errorf("party of 3 burned %g, which is no better than three solo runs (%g)", party, solo*3)
	}
	if party <= solo {
		t.Errorf("party of 3 burned %g, no more than one player alone (%g)", party, solo)
	}
}
