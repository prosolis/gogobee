package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// An armed ability is consumed once, at fight start, and its id is parked on the
// seat. Everything here defends that split.
//
// Before it existed, buildZoneCombatants consumed the ability itself — and the
// turn-based engine calls that builder again on every !attack. So a Berserker
// raged on round 1, the builder cleared their armed flag and saved, and round 2
// rebuilt them with no rage at all. They paid stamina for one round of a buff
// that is supposed to span the fight, and the close-out could not see the rage
// it was supposed to charge exhaustion for.

// ragingBerserker is a Fighter who has already `!arm`ed rage.
func ragingBerserker(t *testing.T, uid id.UserID) *DnDCharacter {
	t.Helper()
	if err := createAdvCharacter(uid, string(uid)); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Subclass: SubclassBerserker, Level: 5,
		STR: 16, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 10,
		HPMax: 40, HPCurrent: 40, ArmorClass: 15,
		ArmedAbility: "rage",
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// ── the split ────────────────────────────────────────────────────────────────

// consumeArmedAbility disarms and reports; applyAbilityByID applies and does not
// disarm. Calling the second one twice must be indistinguishable from once.
func TestApplyAbilityByID_IsPureAndRepeatable(t *testing.T) {
	setupEmptyTestDB(t)
	c := ragingBerserker(t, "@pure:example.org")

	armed := consumeArmedAbility(c)
	if armed != "rage" {
		t.Fatalf("consumeArmedAbility = %q, want rage", armed)
	}
	if c.ArmedAbility != "" {
		t.Errorf("character still armed after consume: %q", c.ArmedAbility)
	}
	if again := consumeArmedAbility(c); again != "" {
		t.Errorf("second consume returned %q, want \"\" — the ability is spent", again)
	}

	// Same id, applied to two independent mod sets, yields the same rage.
	for i, want := range []bool{true, true} {
		var mods CombatModifiers
		name, fired := applyAbilityByID(c, armed, &mods)
		if !fired || name != "Rage" {
			t.Fatalf("apply #%d: fired=%v name=%q", i+1, fired, name)
		}
		if mods.BerserkerRage != want {
			t.Errorf("apply #%d: BerserkerRage = %v, want %v", i+1, mods.BerserkerRage, want)
		}
		if mods.RageMeleeDmg != 2 || !mods.PhysicalResistRage {
			t.Errorf("apply #%d: rage did not carry its full mod set: %+v", i+1, mods)
		}
	}

	// And it never writes back to the sheet.
	got, _ := LoadDnDCharacter(c.UserID)
	if got.ArmedAbility != "" {
		t.Errorf("applyAbilityByID re-armed the sheet: %q", got.ArmedAbility)
	}
}

func TestApplyAbilityByID_UnknownAndEmptyAreNoOps(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter}
	for _, id := range []string{"", "no_such_ability"} {
		var mods CombatModifiers
		if _, fired := applyAbilityByID(c, id, &mods); fired {
			t.Errorf("applyAbilityByID(%q) fired", id)
		}
		if mods.BerserkerRage {
			t.Errorf("applyAbilityByID(%q) set rage", id)
		}
	}
}

// ── the fight-start seat ─────────────────────────────────────────────────────

// The seat carries the id forward, and the sheet is disarmed exactly once.
func TestBuildFightSeats_ConsumesTheAbilityOnceAndCarriesItOnTheSeat(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@berserk:example.org")
	ragingBerserker(t, uid)

	seats, _, _, refusal := (&AdventurePlugin{}).buildFightSeats(
		uid, []id.UserID{uid}, dndBestiary["goblin"], 1, 0)
	if refusal != "" {
		t.Fatalf("fight refused: %s", refusal)
	}
	if len(seats) != 1 {
		t.Fatalf("seats = %d, want 1", len(seats))
	}
	if seats[0].ArmedAbility != "rage" {
		t.Errorf("seat.ArmedAbility = %q, want rage", seats[0].ArmedAbility)
	}
	if !seats[0].Mods.BerserkerRage {
		t.Error("seat built without rage in its mods")
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "" {
		t.Errorf("sheet still armed after fight start: %q", got.ArmedAbility)
	}
}

// The regression itself: rebuilding the combatant — what every !attack does —
// must reproduce the rage from the persisted id, not from the (now cleared)
// sheet. Two rebuilds in a row, both raging.
func TestBuildZoneCombatants_RebuildKeepsTheRageForTheWholeFight(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@rebuild:example.org")
	ragingBerserker(t, uid)
	p := &AdventurePlugin{}

	seats, _, _, refusal := p.buildFightSeats(uid, []id.UserID{uid}, dndBestiary["goblin"], 1, 0)
	if refusal != "" {
		t.Fatalf("fight refused: %s", refusal)
	}
	armed := seats[0].ArmedAbility

	for round := 2; round <= 3; round++ {
		player, _, _, err := p.buildZoneCombatants(uid, dndBestiary["goblin"], 1, 0, armed)
		if err != nil {
			t.Fatalf("round %d rebuild: %v", round, err)
		}
		if !player.Mods.BerserkerRage {
			t.Errorf("round %d: rage evaporated on rebuild", round)
		}
		if player.Mods.RageMeleeDmg != 2 {
			t.Errorf("round %d: RageMeleeDmg = %d, want 2", round, player.Mods.RageMeleeDmg)
		}
	}

	// A seat that armed nothing rebuilds without rage — the id is the only source.
	player, _, _, err := p.buildZoneCombatants(uid, dndBestiary["goblin"], 1, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if player.Mods.BerserkerRage {
		t.Error("unarmed rebuild raged anyway")
	}
}

// A member who is down sits the fight out. They must not be charged the ability
// they had readied for it — the refusal is checked before the arm.
func TestBuildFightSeats_SatOutMemberKeepsTheirArmedAbility(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	downed := id.UserID("@downed:example.org")
	fightTestChar(t, leader, 30)
	ragingBerserker(t, downed)

	// Drop the member to 0 HP with rage still armed.
	c, _ := LoadDnDCharacter(downed)
	c.HPCurrent = 0
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}

	seats, _, _, refusal := (&AdventurePlugin{}).buildFightSeats(
		leader, []id.UserID{leader, downed}, dndBestiary["goblin"], 1, 0)
	if refusal != "" {
		t.Fatalf("fight refused: %s", refusal)
	}
	if len(seats) != 1 || seats[0].UserID != leader {
		t.Fatalf("seats = %+v, want the leader alone", seats)
	}
	got, _ := LoadDnDCharacter(downed)
	if got.ArmedAbility != "rage" {
		t.Errorf("downed member was disarmed for a fight they never joined: %q", got.ArmedAbility)
	}
}

// ── the close-out ────────────────────────────────────────────────────────────

func TestSeatFightStartMods_ReadsTheRageOffTheSeatsStatuses(t *testing.T) {
	c := &DnDCharacter{Class: ClassFighter, Subclass: SubclassBerserker, Level: 5}

	raging := &CombatSession{}
	raging.Statuses.ArmedAbility = "rage"
	if !seatFightStartMods(raging, 0, c).BerserkerRage {
		t.Error("seat 0 armed rage, mods say otherwise")
	}

	// Seat 1 reads its own statuses, not the leader's.
	party := &CombatSession{Participants: []CombatParticipant{{Seat: 1}}}
	party.Statuses.ArmedAbility = "rage"
	if seatFightStartMods(party, 1, c).BerserkerRage {
		t.Error("a member inherited the leader's rage")
	}
}

func TestSeatCombatResult_ReadsWinAndNearDeathOffTheSession(t *testing.T) {
	sess := &CombatSession{
		Status: CombatStatusWon, Round: 4, PlayerHP: 5, PlayerHPMax: 40, EnemyHP: 0,
		TurnLog: []CombatEvent{
			{Action: "attack", Seat: 0},
			{Action: "use_consumable", Seat: 1},
		},
	}
	got := seatCombatResult(sess, 0)
	if !got.PlayerWon {
		t.Error("PlayerWon = false on a won session")
	}
	if !got.NearDeath {
		t.Error("5/40 HP is under the 15% near-death line")
	}
	if len(got.Events) != 1 || got.Events[0].Action != "attack" {
		t.Errorf("seat 0 got seat 1's events: %+v", got.Events)
	}

	sess.PlayerHP = 30
	if seatCombatResult(sess, 0).NearDeath {
		t.Error("30/40 HP is not near death")
	}
	sess.Status = CombatStatusLost
	sess.PlayerHP = 0
	if seatCombatResult(sess, 0).PlayerWon {
		t.Error("PlayerWon = true on a lost session")
	}
}

// Item A: the manual kill now costs the Berserker their exhaustion, exactly as
// the auto-resolved one always has.
func TestPostCombatBookkeepingForSeat_RageCostsExhaustionOnTheTurnBasedPath(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@exhausted:example.org")
	ragingBerserker(t, uid)

	sess := &CombatSession{
		UserID: string(uid), Status: CombatStatusWon, Round: 3,
		PlayerHP: 12, PlayerHPMax: 40,
	}
	sess.Statuses.ArmedAbility = "rage"

	(&AdventurePlugin{}).postCombatBookkeepingForSeat(sess, 0)

	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 1 {
		t.Errorf("Exhaustion = %d after a raging win, want 1", got.Exhaustion)
	}
}

// A fight nobody raged in leaves the sheet alone.
func TestPostCombatBookkeepingForSeat_NoRageNoExhaustion(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@calm:example.org")
	fightTestChar(t, uid, 30)

	sess := &CombatSession{
		UserID: string(uid), Status: CombatStatusWon, Round: 2,
		PlayerHP: 20, PlayerHPMax: 30,
	}
	(&AdventurePlugin{}).postCombatBookkeepingForSeat(sess, 0)

	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 0 {
		t.Errorf("Exhaustion = %d with no rage, want 0", got.Exhaustion)
	}
}

// Losing while raging still exhausts you — the close-out runs on every terminal
// status, not just the win. This is the half the turn-based path used to skip.
func TestPostCombatBookkeepingForSeat_RageCostsExhaustionOnALoss(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@dead:example.org")
	ragingBerserker(t, uid)

	sess := &CombatSession{
		UserID: string(uid), Status: CombatStatusLost, Round: 6,
		PlayerHP: 0, PlayerHPMax: 40,
	}
	sess.Statuses.ArmedAbility = "rage"

	(&AdventurePlugin{}).postCombatBookkeepingForSeat(sess, 0)

	got, _ := LoadDnDCharacter(uid)
	if got.Exhaustion != 1 {
		t.Errorf("Exhaustion = %d after a raging loss, want 1", got.Exhaustion)
	}
}
