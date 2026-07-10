package plugin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// N3/P5 — the session layer.
//
// Everything here exercises a seated party, which production cannot yet build:
// P6 supplies the invite. The point of testing it now is that P5's mistakes are
// silent ones — a buff landing on the wrong sheet, a latch that resets on every
// step, a solo row that grew a key — and none of them announce themselves.

// ── Seat addressing ──────────────────────────────────────────────────────────

// partySession builds a 3-seat roster in memory, with each seat's HP and
// statuses distinguishable so a mix-up cannot pass.
func partySession(phase string, seatHP ...int) *CombatSession {
	s := turnSession(phase, seatHP[0], 100)
	s.UserID = "@leader:x"
	for i, hp := range seatHP[1:] {
		s.Participants = append(s.Participants, CombatParticipant{
			Seat: i + 1, UserID: memberID(i + 1), HP: hp, HPMax: hp + 10,
		})
	}
	s.rosterSize = len(seatHP)
	return s
}

func memberID(seat int) string {
	return string(rune('a'+seat-1)) + "member:x"
}

func TestSeatAccessors_AddressTheRightRow(t *testing.T) {
	s := partySession(CombatPhasePlayerTurn, 40, 55, 70)
	s.PlayerHPMax = 90

	for seat, want := range []int{40, 55, 70} {
		if got := s.seatHP(seat); got != want {
			t.Errorf("seatHP(%d) = %d, want %d", seat, got, want)
		}
	}
	if got := s.seatHPMax(0); got != 90 {
		t.Errorf("seatHPMax(0) = %d, want the session row's 90", got)
	}
	if got := s.seatHPMax(2); got != 80 {
		t.Errorf("seatHPMax(2) = %d, want the participant row's 80", got)
	}
	if got := s.seatUserID(0); got != "@leader:x" {
		t.Errorf("seatUserID(0) = %q, want the session owner", got)
	}
	if got := s.seatUserID(1); got != memberID(1) {
		t.Errorf("seatUserID(1) = %q, want %q", got, memberID(1))
	}
}

func TestSeatOf_FindsMembersAndRejectsStrangers(t *testing.T) {
	s := partySession(CombatPhasePlayerTurn, 40, 55, 70)

	if seat, ok := s.seatOf("@leader:x"); !ok || seat != 0 {
		t.Errorf("seatOf(leader) = (%d,%v), want (0,true)", seat, ok)
	}
	if seat, ok := s.seatOf(id.UserID(memberID(2))); !ok || seat != 2 {
		t.Errorf("seatOf(member 2) = (%d,%v), want (2,true)", seat, ok)
	}
	if _, ok := s.seatOf("@nobody:x"); ok {
		t.Error("seatOf(stranger) reported a seat — an outsider could act in the fight")
	}
}

// A member's mid-fight buff must land on their own sheet. Before P5 the cast
// path folded every delta into the session's embedded ActorStatuses — seat 0 —
// so a party member casting Shield on themselves armoured the leader instead.
func TestActorStatusesPtr_WritesToTheCastingSeat(t *testing.T) {
	s := partySession(CombatPhasePlayerTurn, 40, 55, 70)

	s.actorStatusesPtr(2).applyBuffDelta(turnBuffDelta{dAC: 5})

	if got := s.Statuses.BuffACBonus; got != 0 {
		t.Errorf("seat 0 picked up seat 2's buff: BuffACBonus = %d, want 0", got)
	}
	if got := s.Participants[1].Statuses.BuffACBonus; got != 5 {
		t.Errorf("seat 2's BuffACBonus = %d, want 5", got)
	}
}

// ── The autopilot latch ──────────────────────────────────────────────────────

// The latch is per-seat session state with no combatState counterpart, so it
// rides through the engine on snapshotActor's carry-over of the prior snapshot —
// the same road the Buff* deltas take. If it did not, an away member would be
// handed the wheel back on every single step and the party would stall anew each
// round.
func TestSnapshotActor_CarriesTheAutopilotLatchAcrossAStep(t *testing.T) {
	c := basePlayer()
	prior := ActorStatuses{Autopilot: true, BuffACBonus: 3}

	got := snapshotActor(newActor(&c), prior)

	if !got.Autopilot {
		t.Error("snapshotActor dropped the autopilot latch — an away member unlatches every step")
	}
	if got.BuffACBonus != 3 {
		t.Errorf("snapshotActor dropped a carried-over buff: BuffACBonus = %d, want 3", got.BuffACBonus)
	}
}

func TestSeatNeedsNoHuman(t *testing.T) {
	s := partySession(CombatPhasePlayerTurn, 40, 0, 70)
	s.Participants[1].Statuses.Autopilot = true

	if s.seatNeedsNoHuman(0) {
		t.Error("a standing, unlatched seat needs its human")
	}
	if !s.seatNeedsNoHuman(1) {
		t.Error("a downed seat must not block the round")
	}
	if !s.seatNeedsNoHuman(2) {
		t.Error("a latched seat must resolve without waiting")
	}
}

// ── Wire compatibility ───────────────────────────────────────────────────────

// Two fields landed on persisted structs in P5. Both must vanish from a solo
// row, which is every row prod holds today: statuses_json and turn_log_json are
// decoded by readers that predate them, and a fight in flight across the deploy
// must resume byte-identically.
func TestP5Fields_StayOffSoloRows(t *testing.T) {
	raw, err := json.Marshal(CombatStatuses{ActorStatuses: ActorStatuses{PoisonTicks: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "autopilot") {
		t.Errorf("a solo statuses_json carries the autopilot key: %s", raw)
	}

	raw, err = json.Marshal(CombatEvent{Actor: "player", Action: "hit", Damage: 4})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Seat") {
		t.Errorf("a solo turn_log event carries the Seat key: %s", raw)
	}

	// …and both must survive the round trip when they are set.
	raw, _ = json.Marshal(ActorStatuses{Autopilot: true})
	var back ActorStatuses
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Autopilot {
		t.Errorf("autopilot did not round-trip: %s", raw)
	}
}

// ── Event attribution ────────────────────────────────────────────────────────

// biasedParty builds a roster whose initiative order is fixed at [0,1,2,enemy],
// so a test can park the cursor on a known seat.
func biasedParty() ([]*Combatant, *Combatant) {
	a, b, c := basePlayer(), basePlayer(), basePlayer()
	a.Name, b.Name, c.Name = "Ada", "Bram", "Cass"
	a.Mods.InitiativeBias, b.Mods.InitiativeBias, c.Mods.InitiativeBias = 300, 200, 100
	e := baseEnemy()
	e.Stats.Speed = 1
	return []*Combatant{&a, &b, &c}, &e
}

func TestTurnEngine_StampsEventsWithTheSeatThatActed(t *testing.T) {
	players, enemy := biasedParty()
	sess := partySession(CombatPhasePlayerTurn, 100, 100, 100)
	sess.Statuses.TurnIdx = 1 // Bram's turn

	events, err := partyStep(sess, players, enemy, PlayerAction{Kind: ActionAttack})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("Bram's turn produced no events")
	}
	for _, e := range events {
		if e.Seat != 1 {
			t.Errorf("event %+v stamped seat %d, want 1 (Bram swung)", e, e.Seat)
		}
	}
}

// Solo events must all be seat 0 — that is what keeps the field omitted from
// every solo turn_log_json.
func TestTurnEngine_SoloEventsAreAllSeatZero(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 100, 100)
	p, e := basePlayer(), baseEnemy()

	events, err := partyStep(sess, []*Combatant{&p}, &e, PlayerAction{Kind: ActionAttack})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Seat != 0 {
			t.Errorf("solo event %+v stamped seat %d, want 0", ev, ev.Seat)
		}
	}
}

// The flavor pool speaks in the second person. A round must therefore read
// differently to each member: your own swing is "You score 9 damage", your
// ally's is "**Cass** hits Rat for 9". Getting this backwards tells three people
// they each personally landed the same blow.
func TestRenderPartyTurnRound_SecondPersonForTheReaderThirdForAllies(t *testing.T) {
	events := []CombatEvent{
		{Actor: "player", Action: "hit", Damage: 7, Seat: 0},
		{Actor: "player", Action: "hit", Damage: 9, Seat: 2},
	}
	names := []string{"Ada", "Bram", "Cass"}

	// The reader's own line comes from the flavor pool, whose phrasings vary
	// ("You score 9 damage", "A measured strike. 9 damage."). What is invariant
	// is that it never names them in the third person, and that the ally's line
	// always does.
	ada := RenderPartyTurnRound(events, names, "Rat", 0)
	if strings.Contains(ada, "**Ada**") {
		t.Errorf("Ada's own swing was rendered in the third person:\n%s", ada)
	}
	if !strings.Contains(ada, "**Cass** hits Rat for 9.") {
		t.Errorf("Ada should read Cass's swing in the third person:\n%s", ada)
	}
	if strings.Contains(ada, "Bram") {
		t.Errorf("Bram did nothing this round but was named:\n%s", ada)
	}

	cass := RenderPartyTurnRound(events, names, "Rat", 2)
	if strings.Contains(cass, "**Cass**") {
		t.Errorf("Cass's own swing was rendered in the third person:\n%s", cass)
	}
	if !strings.Contains(cass, "**Ada** hits Rat for 7.") {
		t.Errorf("Cass should read Ada's swing in the third person:\n%s", cass)
	}
}

// A downed ally is the one thing a member must not miss in their own DM.
func TestRenderAllySeatEvent_FlagsAnAllyGoingDown(t *testing.T) {
	got := renderAllySeatEvent(
		CombatEvent{Actor: "enemy", Action: "hit", Damage: 12, PlayerHP: 0, Seat: 1}, "Bram", "Rat")
	if !strings.Contains(got, "is down") {
		t.Errorf("an ally dropping to 0 HP should be called out, got %q", got)
	}

	standing := renderAllySeatEvent(
		CombatEvent{Actor: "enemy", Action: "hit", Damage: 3, PlayerHP: 20, Seat: 1}, "Bram", "Rat")
	if strings.Contains(standing, "is down") {
		t.Errorf("a standing ally was reported down: %q", standing)
	}
}

// An event whose seat has fallen off the roster falls back to seat 0 rather
// than panicking on a replayed turn_log.
func TestRenderPartyTurnRound_OutOfRangeSeatFallsBack(t *testing.T) {
	got := RenderPartyTurnRound(
		[]CombatEvent{{Actor: "player", Action: "hit", Damage: 3, Seat: 9}}, []string{"Ada"}, "Rat", 0)
	if got == "" || strings.Contains(got, "without a clean blow") {
		t.Errorf("out-of-range seat should still render a line, got:\n%s", got)
	}
}

// ── The turn deadline ────────────────────────────────────────────────────────

func TestTurnDeadlineLapsed(t *testing.T) {
	fresh := time.Now().UTC()
	stale := fresh.Add(-partyTurnDeadline - time.Second)

	cases := []struct {
		name  string
		phase string
		at    time.Time
		want  bool
	}{
		{"fresh player turn", CombatPhasePlayerTurn, fresh, false},
		{"stale player turn", CombatPhasePlayerTurn, stale, true},
		// The enemy's turn belongs to the engine, not to a player who might be
		// away — a fight parked there is mid-resolution, not stalled on anyone.
		{"stale enemy turn", CombatPhaseEnemyTurn, stale, false},
		{"stale round end", CombatPhaseRoundEnd, stale, false},
	}
	for _, tc := range cases {
		s := partySession(tc.phase, 40, 40)
		s.LastActionAt = tc.at
		if got := turnDeadlineLapsed(s); got != tc.want {
			t.Errorf("%s: turnDeadlineLapsed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ── Locking ──────────────────────────────────────────────────────────────────

// A solo fight's owner is the sender. sync.Mutex is not reentrant, so taking the
// fight lock and then the user lock would deadlock the very command every solo
// player types. This test hangs rather than fails if that regresses.
func TestLockCombatFight_SoloTakesExactlyOneLock(t *testing.T) {
	p := &AdventurePlugin{}
	done := make(chan struct{})
	go func() {
		release := p.lockCombatFight("@solo:x", "@solo:x")
		release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lockCombatFight self-deadlocked on a solo fight")
	}
}

// A party member's command holds both the fight's lock and their own, so no
// other command of theirs races the round they are resolving.
func TestLockCombatFight_PartyHoldsBothLocks(t *testing.T) {
	p := &AdventurePlugin{}
	release := p.lockCombatFight("@leader:x", "@member:x")

	if p.advUserLock("@leader:x").TryLock() {
		t.Error("the fight's lock (seat 0) was not held")
	}
	if p.advUserLock("@member:x").TryLock() {
		t.Error("the acting member's own lock was not held")
	}
	release()

	if !p.advUserLock("@leader:x").TryLock() {
		t.Error("release() left the fight's lock held")
	}
	if !p.advUserLock("@member:x").TryLock() {
		t.Error("release() left the member's lock held")
	}
}

func TestNotYourTurnMsg_NamesWhoWeAreWaitingOn(t *testing.T) {
	players, _ := biasedParty()
	if got := notYourTurnMsg(players, 1, true); !strings.Contains(got, "Bram") {
		t.Errorf("message should name the acting player, got %q", got)
	}
	if got := notYourTurnMsg(players, 0, false); strings.Contains(got, "Ada") {
		t.Errorf("nobody is on the clock mid-resolution, got %q", got)
	}
}

// ── Starting a fight ─────────────────────────────────────────────────────────

func TestStartPartyCombatSession_SoloWritesNoParticipantRows(t *testing.T) {
	setupEmptyTestDB(t)

	sess, err := (&AdventurePlugin{}).startPartyCombatSession("run1", "room3", "goblin", 60,
		[]CombatSeatSetup{{UserID: "@solo:x", HP: 40, HPMax: 40}})
	if err != nil {
		t.Fatal(err)
	}
	if sess.IsParty() || sess.RosterSize() != 1 {
		t.Fatalf("solo session reports a roster of %d", sess.RosterSize())
	}
	ps, err := loadCombatParticipants(sess.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 0 {
		t.Errorf("solo fight wrote %d participant rows, want 0", len(ps))
	}
}

// Each seat's fight-start one-shots are read off their own combatant. A party
// Abjurer brings their own Arcane Ward; the leader does not inherit it.
func TestStartPartyCombatSession_SeedsEachSeatsOwnOneShots(t *testing.T) {
	setupEmptyTestDB(t)

	sess, err := (&AdventurePlugin{}).startPartyCombatSession("run1", "room3", "goblin", 60, []CombatSeatSetup{
		{UserID: "@leader:x", HP: 40, HPMax: 40},
		{UserID: "@abjurer:x", HP: 30, HPMax: 30, Mods: CombatModifiers{ArcaneWardHP: 12, WardCharges: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sess.IsParty() || sess.RosterSize() != 2 {
		t.Fatalf("roster size = %d, want 2", sess.RosterSize())
	}
	if sess.Statuses.ArcaneWardHP != 0 {
		t.Errorf("the leader inherited the abjurer's ward: %d", sess.Statuses.ArcaneWardHP)
	}

	// …and it must be there after a round trip through the DB.
	back, err := getCombatSession(sess.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.actorStatusesForSeat(1); got.ArcaneWardHP != 12 || got.WardCharges != 2 {
		t.Errorf("seat 1 statuses after reload = %+v, want ArcaneWardHP 12 / WardCharges 2", got)
	}
	if back.seatHP(1) != 30 || back.seatUserID(1) != "@abjurer:x" {
		t.Errorf("seat 1 = %d HP / %s", back.seatHP(1), back.seatUserID(1))
	}
}

// ── The round driver ─────────────────────────────────────────────────────────

// The old loop rested on any player_turn. A party's downed seat still holds a
// player_turn, so the round would come to rest on a corpse and the fight would
// wait forever for a dead member to type `!attack`.
func TestSettleCombatSession_StepsPastADownedSeat(t *testing.T) {
	setupEmptyTestDB(t)
	players, enemy := biasedParty()

	sess, err := (&AdventurePlugin{}).startPartyCombatSession("run1", "room3", "goblin", 100, []CombatSeatSetup{
		{UserID: "@leader:x", HP: 100, HPMax: 100},
		{UserID: "@bram:x", HP: 100, HPMax: 100},
		{UserID: "@cass:x", HP: 100, HPMax: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Bram is down, and it is his turn.
	sess.Participants[0].HP = 0
	sess.Statuses.TurnIdx = 1

	if _, err := settleCombatSession(sess, players, enemy); err != nil {
		t.Fatal(err)
	}
	if !sess.IsActive() {
		t.Fatalf("fight ended; two members were still standing (status %q)", sess.Status)
	}
	seat, waiting := actingSeat(sess, players, enemy)
	if !waiting {
		t.Fatal("settle came to rest on no player's turn")
	}
	if seat != 2 {
		t.Errorf("settled on seat %d, want seat 2 — seat 1 is down and forfeits", seat)
	}
}

// Settling a session already parked on a standing player's turn must do nothing
// at all. Every solo fight sits exactly there between commands, and a stray step
// would resolve a round the player never asked for.
func TestSettleCombatSession_IsANoOpOnASoloPlayerTurn(t *testing.T) {
	setupEmptyTestDB(t)
	p, e := basePlayer(), baseEnemy()

	sess, err := (&AdventurePlugin{}).startPartyCombatSession("run1", "room3", "goblin", 60,
		[]CombatSeatSetup{{UserID: "@solo:x", HP: 40, HPMax: 40}})
	if err != nil {
		t.Fatal(err)
	}
	before := *sess

	events, err := settleCombatSession(sess, []*Combatant{&p}, &e)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("settle emitted %d events on an idle solo turn", len(events))
	}
	if sess.Round != before.Round || sess.Phase != before.Phase || sess.PlayerHP != before.PlayerHP {
		t.Errorf("settle advanced an idle solo fight: %d/%s/%d -> %d/%s/%d",
			before.Round, before.Phase, before.PlayerHP, sess.Round, sess.Phase, sess.PlayerHP)
	}
}
