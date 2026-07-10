package plugin

// Initiative + N-body turn-engine tests (N3/P3).
//
// The solo path is pinned bit-for-bit by TestCombatCharacterization for
// auto-resolve, and by the fixed-order assertions below for the turn engine.
// Everything else here exercises seats the production code cannot reach until
// P4 gives a session its participant rows.

import (
	"testing"
)

// partyStep drives one engine step over a roster without touching the DB.
func partyStep(sess *CombatSession, players []*Combatant, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	seat := enemySeat
	if sess.Phase == CombatPhasePlayerTurn {
		order := turnOrder(sess, sess.Round, players, enemy)
		seat = order[turnIdxForPhase(order, sess.Statuses.TurnIdx, sess.Phase)]
	}
	te := resumeTurnEngine(sess, players, enemy, combatSessionStepRNG(sess, seat))
	events, err := te.step(action)
	if err != nil {
		return nil, err
	}
	te.commit()
	return events, nil
}

func TestTurnOrder_SoloIsAlwaysPlayerThenEnemy(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 100, 100)
	p, e := basePlayer(), baseEnemy()
	// A blisteringly fast monster still swings second: the turn engine has
	// never rolled initiative for a duel, and P3 must not start.
	e.Stats.Speed = 999
	p.Mods.InitiativeBias = -50

	for round := 1; round <= 5; round++ {
		got := turnOrder(sess, round, []*Combatant{&p}, &e)
		if len(got) != 2 || got[0] != 0 || got[1] != enemySeat {
			t.Fatalf("round %d solo order = %v, want [0 %d]", round, got, enemySeat)
		}
	}
}

func TestTurnOrder_PartySeatsEveryoneOnceAndIsDeterministic(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 100, 100)
	a, b, c := basePlayer(), basePlayer(), basePlayer()
	e := baseEnemy()
	players := []*Combatant{&a, &b, &c}

	order := turnOrder(sess, 3, players, &e)
	if len(order) != 4 {
		t.Fatalf("order = %v, want 4 slots (3 players + enemy)", order)
	}
	seen := map[int]bool{}
	for _, seat := range order {
		if seen[seat] {
			t.Fatalf("seat %d appears twice in %v", seat, order)
		}
		seen[seat] = true
	}
	if !seen[0] || !seen[1] || !seen[2] || !seen[enemySeat] {
		t.Fatalf("order %v does not seat every combatant", order)
	}

	// Same (session, round) must rebuild the same order — that is what lets a
	// resumed fight land on the seat it left off on.
	again := turnOrder(sess, 3, players, &e)
	for i := range order {
		if order[i] != again[i] {
			t.Fatalf("order not stable across rebuilds: %v vs %v", order, again)
		}
	}
}

func TestTurnOrder_InitiativeBiasWins(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 100, 100)
	slow, fast := basePlayer(), basePlayer()
	fast.Mods.InitiativeBias = 100 // dwarfs the d10 spread
	e := baseEnemy()

	order := turnOrder(sess, 1, []*Combatant{&slow, &fast}, &e)
	if order[0] != 1 {
		t.Errorf("order = %v, want the biased seat 1 first", order)
	}
}

func TestCombatSessionStepRNG_SeatZeroMatchesLegacyStream(t *testing.T) {
	sess := &CombatSession{SessionID: "abc123", Round: 2, Phase: CombatPhasePlayerTurn}
	if combatSessionStepRNG(sess, 0).Uint64() != combatSessionRNG(sess).Uint64() {
		t.Error("seat 0 must draw the pre-roster stream — the solo turn engine depends on it")
	}
	// The enemy sentinel shares seat 0's seed; its phase already separates it.
	if combatSessionStepRNG(sess, enemySeat).Uint64() != combatSessionRNG(sess).Uint64() {
		t.Error("the enemy sentinel must not perturb the seed")
	}
	// Party members share a (round, phase) pair, so only the seed keeps their
	// d20s apart.
	if combatSessionStepRNG(sess, 1).Uint64() == combatSessionStepRNG(sess, 0).Uint64() {
		t.Error("seats 0 and 1 drew the same stream in the same phase")
	}
	if combatSessionStepRNG(sess, 1).Uint64() == combatSessionStepRNG(sess, 2).Uint64() {
		t.Error("seats 1 and 2 drew the same stream in the same phase")
	}
}

func TestTurnIdxForPhase_ReconcilesARowWrittenBeforeTheCursorExisted(t *testing.T) {
	solo := []int{0, enemySeat}
	// A fight suspended mid-enemy-turn decodes TurnIdx as 0. Phase is the older
	// field and wins, so the cursor must snap to the enemy's slot — otherwise
	// the engine re-runs the enemy turn forever.
	if got := turnIdxForPhase(solo, 0, CombatPhaseEnemyTurn); got != 1 {
		t.Errorf("legacy enemy_turn row reconciled to idx %d, want 1", got)
	}
	if got := turnIdxForPhase(solo, 0, CombatPhasePlayerTurn); got != 0 {
		t.Errorf("player_turn row = idx %d, want 0", got)
	}
	// An out-of-range cursor (roster shrank) falls back to the phase.
	if got := turnIdxForPhase(solo, 7, CombatPhaseEnemyTurn); got != 1 {
		t.Errorf("out-of-range cursor = idx %d, want 1", got)
	}
	// A party order keeps an already-consistent cursor exactly where it is.
	party := []int{1, enemySeat, 0}
	if got := turnIdxForPhase(party, 2, CombatPhasePlayerTurn); got != 2 {
		t.Errorf("consistent cursor moved to idx %d, want 2", got)
	}
}

func TestTurnEngine_SoloRoundIsPlayerEnemyRoundEnd(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 10000, 10000)
	p, e := basePlayer(), baseEnemy()
	players := []*Combatant{&p}

	for _, want := range []string{CombatPhaseEnemyTurn, CombatPhaseRoundEnd, CombatPhasePlayerTurn} {
		if _, err := partyStep(sess, players, &e, PlayerAction{Kind: ActionAttack}); err != nil {
			t.Fatal(err)
		}
		if sess.Phase != want {
			t.Fatalf("phase = %q, want %q", sess.Phase, want)
		}
	}
	if sess.Round != 2 {
		t.Errorf("round = %d, want 2 after one full round", sess.Round)
	}
	if sess.Statuses.TurnIdx != 0 {
		t.Errorf("solo cursor = %d, want 0 — it should never leave the first slot at a player turn", sess.Statuses.TurnIdx)
	}
}

func TestTurnEngine_PartyRoundGivesEverySeatATurn(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 10000, 10000)
	a, b := basePlayer(), basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = 100000
	players := []*Combatant{&a, &b}
	order := turnOrder(sess, 1, players, &e)

	// Walk the round slot by slot, asserting each phase matches the order.
	for i, seat := range order {
		if sess.Phase != phaseForSeat(seat) {
			t.Fatalf("slot %d: phase = %q, want %q for seat %d", i, sess.Phase, phaseForSeat(seat), seat)
		}
		if sess.Statuses.TurnIdx != i {
			t.Fatalf("slot %d: cursor = %d", i, sess.Statuses.TurnIdx)
		}
		if _, err := partyStep(sess, players, &e, PlayerAction{Kind: ActionAttack}); err != nil {
			t.Fatal(err)
		}
	}
	if sess.Phase != CombatPhaseRoundEnd {
		t.Fatalf("phase after every seat acted = %q, want round_end", sess.Phase)
	}
	if _, err := partyStep(sess, players, &e, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Round != 2 || sess.Statuses.TurnIdx != 0 {
		t.Errorf("round_end left round=%d cursor=%d, want 2 and 0", sess.Round, sess.Statuses.TurnIdx)
	}
}

func TestTurnEngine_DownedSeatForfeitsItsTurnSilently(t *testing.T) {
	// Force a party order whose first slot is a player, then kill that player.
	sess := turnSession(CombatPhasePlayerTurn, 10000, 10000)
	a, b := basePlayer(), basePlayer()
	e := baseEnemy()
	players := []*Combatant{&a, &b}
	order := turnOrder(sess, 1, players, &e)
	first := order[0]
	if first == enemySeat {
		t.Skip("this round's initiative put the enemy first; the seat-skip path is covered by the next slot")
	}

	te := resumeTurnEngine(sess, players, &e, combatSessionStepRNG(sess, first))
	te.st.actors[first].playerHP = 0
	events, err := te.step(PlayerAction{Kind: ActionAttack})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("a downed seat emitted %d events, want none", len(events))
	}
	if !sess.IsActive() {
		t.Error("one downed member ended the fight while another still stands")
	}
	if sess.Statuses.TurnIdx != 1 {
		t.Errorf("cursor = %d, want the next slot", sess.Statuses.TurnIdx)
	}
}

func TestTurnEngine_EnemyOnlySwingsAtStandingMembers(t *testing.T) {
	sess := turnSession(CombatPhaseEnemyTurn, 10000, 10000)
	a, b := basePlayer(), basePlayer()
	a.Stats.MaxHP, b.Stats.MaxHP = 10000, 10000
	e := baseEnemy()
	players := []*Combatant{&a, &b}

	// Seat 0 is down. Every enemy turn must land on seat 1, never on the corpse,
	// and never end the fight.
	for round := 1; round <= 25; round++ {
		sess.Round = round
		sess.Phase = CombatPhaseEnemyTurn
		sess.Statuses.TurnIdx = 1
		te := resumeTurnEngine(sess, players, &e, combatSessionStepRNG(sess, enemySeat))
		te.st.actors[0].playerHP = 0
		before := te.st.actors[1].playerHP
		te.step(PlayerAction{})
		if te.st.actors[0].playerHP != 0 {
			t.Fatal("the enemy healed a corpse")
		}
		if !sess.IsActive() {
			t.Fatalf("round %d: fight ended with a member still standing (%d HP)", round, before)
		}
	}
}

func TestTurnEngine_RosterWipeEndsTheFight(t *testing.T) {
	sess := turnSession(CombatPhaseEnemyTurn, 1, 10000)
	a, b := basePlayer(), basePlayer()
	e := baseEnemy()
	players := []*Combatant{&a, &b}

	te := resumeTurnEngine(sess, players, &e, combatSessionStepRNG(sess, enemySeat))
	te.st.actors[0].playerHP = 0
	te.st.actors[1].playerHP = 0
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Status != CombatStatusLost {
		t.Errorf("status = %q, want %q once the whole roster is down", sess.Status, CombatStatusLost)
	}
}

// commit persists seat 0, not whoever the cursor happens to be parked on. The
// enemy turn parks it on its target; round_end walks it across the roster.
func TestTurnEngine_CommitPersistsSeatZeroNotTheCursor(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 500, 10000)
	a, b := basePlayer(), basePlayer()
	e := baseEnemy()
	players := []*Combatant{&a, &b}

	te := resumeTurnEngine(sess, players, &e, combatSessionStepRNG(sess, enemySeat))
	te.st.actors[0].playerHP = 300
	te.st.actors[1].playerHP = 77
	te.st.actors[1].luckyUsed = true
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	te.commit()

	if sess.PlayerHP != 300 {
		t.Errorf("persisted PlayerHP = %d, want seat 0's 300", sess.PlayerHP)
	}
	if sess.Statuses.LuckyUsed {
		t.Error("seat 1's consumed Lucky reroll leaked onto the session row")
	}
}
