package plugin

import (
	"errors"
	"testing"

	"maunium.net/go/mautrix/id"
)

// The companion's whole contract is "he fights, and he is not a player". These
// tests pin both halves — and specifically the seams where an NPC seat would
// otherwise be silently treated as a person.

func TestCompanion_HiredSeatIsNotAMouth(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@leader:example.org")
	seedExpedition(t, "exp-hire", owner, "active")
	seatLeaderFixture(t, "exp-hire")

	if err := hireCompanion("exp-hire", ClassCleric, 4); err != nil {
		t.Fatalf("hireCompanion: %v", err)
	}

	// The roster holds two seats...
	members, err := partyMembers("exp-hire")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("roster has %d seats, want 2 (leader + companion)", len(members))
	}

	// ...but only one of them eats. partySize feeds the daily supply burn and the
	// "your party is still waiting on you" lock-out; counting the companion would
	// bill the leader for rations he never bought, and strand him out of his next
	// expedition behind a party of one bot.
	n, err := partySize("exp-hire")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("partySize = %d, want 1 — the companion is not a mouth", n)
	}
}

func TestCompanion_GetsNoMailButTakesASeat(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@leader2:example.org")
	seedExpedition(t, "exp-mail", owner, "active")
	seatLeaderFixture(t, "exp-mail")
	if err := hireCompanion("exp-mail", ClassFighter, 3); err != nil {
		t.Fatalf("hireCompanion: %v", err)
	}
	exp, err := getExpedition("exp-mail")
	if err != nil || exp == nil {
		t.Fatalf("getExpedition: %v", err)
	}

	// Mail and seats are different sets. Every DM seam reads the audience; the
	// combat roster reads the seats. Getting this backwards either DMs a bot or
	// charges a leader for a body that never sits down.
	for _, uid := range expeditionAudience(exp) {
		if isCompanionSeat(uid) {
			t.Fatal("companion is in the DM audience — he does not get mail")
		}
	}
	var seated bool
	for _, uid := range expeditionSeats(exp) {
		if isCompanionSeat(uid) {
			seated = true
		}
	}
	if !seated {
		t.Fatal("companion is not in the fight roster — he was paid for and never sat down")
	}
}

func TestCompanion_IsGloballyExclusive(t *testing.T) {
	setupEmptyTestDB(t)
	for _, e := range []struct {
		id    string
		owner id.UserID
	}{{"exp-a", "@a:example.org"}, {"exp-b", "@b:example.org"}} {
		seedExpedition(t, e.id, e.owner, "active")
		seatLeaderFixture(t, e.id)
	}

	if err := hireCompanion("exp-a", ClassMage, 5); err != nil {
		t.Fatalf("first hire: %v", err)
	}
	// He is one person. A second party cannot have him — "out on assignment" is
	// the scarcity knob, not a bug to route around.
	if err := hireCompanion("exp-b", ClassMage, 5); !errors.Is(err, ErrCompanionOnAssignment) {
		t.Errorf("second hire err = %v, want ErrCompanionOnAssignment", err)
	}
	// Re-hiring him into the party he's already with is its own answer.
	if err := hireCompanion("exp-a", ClassMage, 5); !errors.Is(err, ErrCompanionAlreadyHired) {
		t.Errorf("re-hire err = %v, want ErrCompanionAlreadyHired", err)
	}

	// Dismissed, he's available again.
	if err := dismissCompanion("exp-a"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if err := hireCompanion("exp-b", ClassMage, 5); err != nil {
		t.Errorf("hire after dismiss: %v, want success", err)
	}
}

// The bug that made the whole feature worse than useless: a SOLO expedition has
// no expedition_party rows at all (the roster only materializes on the first
// invite), so reading the roster to size the companion answered "nobody" for
// exactly the player who is hiring him — the one with no friends around. Every
// solo hire got a level-1 Pete, in whatever tier the leader was actually in.
func TestCompanion_SoloLeaderSizesHimCorrectly(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@lonely:example.org")
	seedExpedition(t, "exp-solo-hire", owner, "active")

	// A level-12 fighter, adventuring alone. No roster rows exist.
	if err := SaveDnDCharacter(&DnDCharacter{
		UserID: owner, Race: RaceHuman, Class: ClassFighter, Level: 12,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 10, CHA: 10,
		HPMax: 120, HPCurrent: 120, ArmorClass: 18,
	}); err != nil {
		t.Fatal(err)
	}

	if got := companionPartyLevel("exp-solo-hire"); got != 11 {
		t.Errorf("companionPartyLevel = %d, want 11 (the lone leader's 12, less the below-median step). "+
			"A level-1 companion in the leader's zone is worse than no companion at all.", got)
	}
	// And he fills the hole the lone fighter actually has, rather than defaulting
	// as if the party were empty.
	if got := companionRoleFill(companionPartyClasses("exp-solo-hire")); got != ClassCleric {
		t.Errorf("role fill for a lone fighter = %v, want cleric", got)
	}
}

// §4 — asking for "the party" must never answer "nobody".
//
// A solo expedition has no expedition_party rows (absence means solo; the roster
// materializes on the first invite). Every consumer that read the roster table
// directly therefore got an empty list for a solo player and fell back to
// whatever looked reasonable locally. That is how the companion was hired at
// level 1 for exactly the player the feature exists for.
//
// expeditionParty always includes the owner. If this test ever fails, that
// guarantee is gone and the same class of bug is back.
func TestParty_SoloExpeditionStillHasAParty(t *testing.T) {
	setupEmptyTestDB(t)
	owner := id.UserID("@alone:example.org")
	seedExpedition(t, "exp-alone", owner, "active")

	// No roster rows exist at all.
	rows, err := partyMembers("exp-alone")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("solo expedition has %d roster rows; the premise of this test is gone", len(rows))
	}

	// And yet the party is not empty.
	seats, err := expeditionParty("exp-alone", string(owner))
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 || seats[0].UserID != owner || seats[0].Kind != SeatLeader {
		t.Fatalf("expeditionParty on a solo run = %+v, want exactly the owner as leader. "+
			"An empty answer here is what hires a level-1 companion.", seats)
	}
	humans, err := partyHumans("exp-alone", string(owner))
	if err != nil || len(humans) != 1 {
		t.Fatalf("partyHumans on a solo run = %+v (err %v), want the owner", humans, err)
	}
}

func TestCompanion_RoleFillTakesTheHole(t *testing.T) {
	tests := []struct {
		name  string
		party []DnDClass
		want  DnDClass
	}{
		{"no healer", []DnDClass{ClassFighter, ClassRogue}, ClassCleric},
		{"no front line", []DnDClass{ClassCleric, ClassMage}, ClassFighter},
		{"no damage", []DnDClass{ClassCleric, ClassFighter}, ClassMage},
		// A paladin covers healer AND front line, so paladin+rogue has no hole at
		// all — it falls through to the default, a second body up front.
		{"a complete party gets the default", []DnDClass{ClassPaladin, ClassRogue}, ClassFighter},
		{"a paladin still leaves the damage hole", []DnDClass{ClassPaladin}, ClassMage},
		{"solo fighter needs the medic first", []DnDClass{ClassFighter}, ClassCleric},
		{"empty party", nil, ClassCleric},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := companionRoleFill(tc.party); got != tc.want {
				t.Errorf("companionRoleFill(%v) = %v, want %v", tc.party, got, tc.want)
			}
		})
	}
}

// §3 — an engine-driven seat's latch is permanent, and no command can take it.
//
// This is the invariant that, when it did not exist, made the companion stand in
// fights doing nothing. The expedition autopilot drives a party by dispatching
// each seat's turn AS that seat; his own auto-played move therefore arrived at
// beginCombatTurn looking exactly like a human returning to the keyboard, and the
// "they typed, so they're here" branch cleared the latch that was moving him.
// After round 1 he never acted again — while the boss he had inflated by 15% HP
// killed the party. The sweep measured it at -27pp clear rate.
//
// Autopilot is provisional (a human is away; a keystroke ends it). EngineDriven is
// not (there is nobody to come back). They must never collapse into one flag.
func TestCompanion_EngineSeatKeepsTheWheel(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	uid := id.UserID("@lead:example.org")

	monster := dndBestiary["goblin"]
	c, _, _ := p.companionCombatant(ClassFighter, 8, monster, 2, 0)
	enemy := Combatant{Name: "x", Stats: CombatStats{MaxHP: 500, AC: 12, Attack: 4, AttackBonus: 4}}

	sess, err := p.startPartyCombatSession("run-e", "enc", "goblin", &enemy, []CombatSeatSetup{
		{UserID: uid, HP: 200, HPMax: 200, Mods: c.Mods, C: &c},
		{UserID: companionUserID(), HP: 120, HPMax: 120, Mods: c.Mods, C: &c, EngineDriven: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !sess.seatIsEngineDriven(1) {
		t.Fatal("companion seat is not engine-driven")
	}
	if sess.seatIsEngineDriven(0) {
		t.Fatal("the human's seat came out engine-driven")
	}
	// The engine drives it without waiting on anybody...
	if !sess.seatIsAutopiloted(1) || !sess.seatNeedsNoHuman(1) {
		t.Fatal("an engine seat must resolve without waiting for a human")
	}
	// ...and the human's seat still waits for its human.
	if sess.seatIsAutopiloted(0) {
		t.Fatal("the human's seat was latched onto autopilot at seating")
	}

	// The wheel cannot be taken back, because there is nobody to take it. This is
	// the exact line that used to strand him: a "keystroke" from his own id.
	sess.actorStatusesPtr(1).Autopilot = false
	if !sess.seatIsAutopiloted(1) {
		t.Fatal("clearing Autopilot stranded the engine seat — EngineDriven must keep it moving")
	}
}

// The one that actually matters: he has to FIGHT. Every other guard in this
// file is about keeping him out of things, and a companion synthesized down to
// zeroed stats would pass all of them while standing in the fight doing nothing
// — a hire that silently buys the leader an empty chair.
func TestCompanion_ActuallySwings(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}

	monster := dndBestiary["goblin"]
	tank := monster
	tank.HP = 5000 // outlast the phase clock, so a real swing has time to land

	pete, _, _ := p.companionCombatant(ClassFighter, 6, tank, 2, 0)

	if pete.Stats.MaxHP <= 0 || pete.Stats.Attack <= 0 || pete.Stats.AC <= 0 {
		t.Fatalf("companion built with dead stats (%d HP / %d atk / %d AC) — he'd stand in the fight and do nothing",
			pete.Stats.MaxHP, pete.Stats.Attack, pete.Stats.AC)
	}

	enemy := Combatant{Name: tank.Name, Stats: CombatStats{MaxHP: 5000, AC: 10, Attack: 1, AttackBonus: 1}}
	res := simulatePartyWithRNG([]Combatant{pete}, enemy, dungeonCombatPhases, seededRNG(7))

	var swings int
	for _, e := range res.Events {
		if e.Actor == "player" && e.Seat == 0 && e.Roll > 0 {
			swings++
		}
	}
	if swings == 0 {
		t.Fatal("the companion never swung — he was hired, seated, and did nothing")
	}
}

func TestCompanion_SheetIsBelowMedianAndNeverPersisted(t *testing.T) {
	setupEmptyTestDB(t)

	// He is statted like a player of his level, so he never drifts from the tuned
	// math — but he arrives a level down, which is the whole "help, never a carry"
	// rule expressed in one number.
	dc := companionSheet(ClassFighter, 7)
	if dc.Level != 7 || dc.HPMax <= 0 {
		t.Fatalf("companionSheet = level %d / %d HP, want a real level-7 chassis", dc.Level, dc.HPMax)
	}

	// And he leaves no trace: a player_meta row is the thing that would turn him
	// into a real character everywhere (graveyard, news, XP, leaderboards), so the
	// synthesis must not have written one.
	if c, _ := LoadDnDCharacter(companionUserID()); c != nil {
		t.Fatal("companion has a persisted dnd_character row — he is a player now, which is the one thing he must never be")
	}
	if _, err := loadAdvCharacter(companionUserID()); err == nil {
		t.Fatal("companion has a persisted player_meta row — ensureDnDCharacterForCombat will auto-build him a character on his first swing")
	}
}
