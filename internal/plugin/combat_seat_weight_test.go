package plugin

import "testing"

// §2(a) — the enemy charges for what a seat BRINGS, not for the fact that it sat
// down.
//
// Both scaling levers (the +15% boss HP and the 1 → 2.4 attacks-a-round action
// economy) counted seats. A seat count charges the same for an under-levelled
// friend, a hired NPC, and a true peer — so a below-median body cost a full seat's
// worth of boss and did not give a full seat's worth back. Measured: hiring the
// companion was *worse than going alone* (66.1% against solo's 69.0%) once his
// free full-heal was taken away.
//
// The invariant that makes this safe to ship: every seat that IS a peer still
// weighs exactly 1.0, so solo and a party of equals are byte-identical and the
// balance corpus does not move. Only an unequal roster lands between the knots.

func TestSeatWeight_APeerWeighsExactlyOne(t *testing.T) {
	// The leader, and a friend of the leader's level: both full price.
	if got := seatWeight(10, 10, false); got != 1.0 {
		t.Errorf("a peer weighs %v, want exactly 1.0 — a party of equals must not move", got)
	}
	// Out-levelling the leader does not make the boss harder for everybody else.
	if got := seatWeight(14, 10, false); got != 1.0 {
		t.Errorf("a higher-level friend weighs %v, want 1.0 (capped)", got)
	}
}

func TestSeatWeight_TheUnderLevelledAndTheHiredCostLess(t *testing.T) {
	// The under-levelled friend — the case that has nothing to do with the
	// companion and has been live since parties shipped.
	half := seatWeight(5, 10, false)
	if half >= 1.0 || half <= seatWeightFloor {
		t.Errorf("a level-5 friend of a level-10 leader weighs %v, want between the floor and 1.0", half)
	}
	// The hireling pays the same level penalty AND the discount for everything a
	// player accrues that he never will (subclass, magic items, Masterwork gear).
	hired := seatWeight(9, 10, true)
	peerAtSameLevel := seatWeight(9, 10, false)
	if hired >= peerAtSameLevel {
		t.Errorf("the hireling weighs %v and a human of his level weighs %v — he must cost the enemy less",
			hired, peerAtSameLevel)
	}
	// But a body is never free: it is still one more thing the boss has to kill.
	if got := seatWeight(1, 20, true); got < seatWeightFloor {
		t.Errorf("a hopelessly under-levelled hireling weighs %v, below the floor %v", got, seatWeightFloor)
	}
}

// The levers themselves: integer knots byte-exact, fractions in between.
func TestSeatWeight_ScalingIsExactAtThePeerKnots(t *testing.T) {
	// Solo and a party of peers reproduce the pre-§2(a) numbers exactly. This is
	// the whole safety argument — if either of these drifts, the corpus is invalid.
	for _, tc := range []struct {
		weight float64
		acts   float64
		hp     float64
	}{
		{1, 1, 1.0},    // solo
		{2, 2.4, 1.15}, // a duo of peers
		{3, 5, 1.15},   // a trio of peers
	} {
		if got := partyActionExpectation(tc.weight); got != tc.acts {
			t.Errorf("weight %v: enemy actions = %v, want exactly %v", tc.weight, got, tc.acts)
		}
		if got := partyEnemyHPScale(tc.weight); got != tc.hp {
			t.Errorf("weight %v: enemy HP scale = %v, want exactly %v", tc.weight, got, tc.hp)
		}
	}

	// A leader plus a hireling is strictly cheaper than a leader plus a peer, and
	// strictly dearer than soloing. Bringing him must cost the boss *something* —
	// that is what stops a free body from becoming a carry.
	duoWithHire := 1 + seatWeight(9, 10, true)
	if a := partyActionExpectation(duoWithHire); a <= 1 || a >= 2.4 {
		t.Errorf("leader + hireling buys the enemy %v actions, want strictly between 1 and 2.4", a)
	}
	if h := partyEnemyHPScale(duoWithHire); h <= 1.0 || h >= 1.15 {
		t.Errorf("leader + hireling scales boss HP by %v, want strictly between 1.0 and 1.15", h)
	}
}

// A seat that is DOWN buys the enemy nothing. §2(b) fixed the head-count half of
// this; the weight half has to hold too, or a corpse keeps paying for swings.
func TestSeatWeight_TheDeadBuyTheEnemyNothing(t *testing.T) {
	alive := &Combatant{SeatWeight: 1}
	hire := &Combatant{SeatWeight: 0.6}
	st := &combatState{actors: []*actor{
		{c: alive, playerHP: 40},
		{c: hire, playerHP: 0}, // dropped
	}}
	if got := livingWeight(st); got != 1 {
		t.Errorf("living weight with a downed hireling = %v, want 1 (only the survivor pays)", got)
	}
	st.actors[1].playerHP = 20
	if got := livingWeight(st); got != 1.6 {
		t.Errorf("living weight with the hireling up = %v, want 1.6", got)
	}
}

// An unset weight reads as a full peer, so every combatant built before this
// existed — and every test that builds one by hand — is priced exactly as before.
func TestSeatWeight_UnsetIsAPeer(t *testing.T) {
	if got := combatantWeight(&Combatant{}); got != 1 {
		t.Errorf("a combatant with no weight set counts %v, want 1", got)
	}
}
