package plugin

import (
	"math/rand/v2"
	"testing"
)

// seededRNG gives each test its own deterministic stream, so none of these can
// join the flaky set that rode the package global before P4 seeded them.
func seededRNG(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
}

// The load-bearing claim of P6e: a one-seat roster draws from the RNG in exactly
// the pre-roster order, so SimulateCombat is the degenerate case of the N-body
// engine and the d8prereq_corpus baselines still compare. TestCombatCharacterization
// is the real proof (57 scenarios); this pins the delegation itself.
func TestSimulateCombat_IsTheOneSeatPartyCase(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		solo := simulateCombatWithRNG(basePlayer(), baseEnemy(), dungeonCombatPhases, seededRNG(seed))
		party := simulatePartyWithRNG([]Combatant{basePlayer()}, baseEnemy(), dungeonCombatPhases, seededRNG(seed))

		if len(party.Seats) != 1 {
			t.Fatalf("seed %d: one player seated %d seats", seed, len(party.Seats))
		}
		got := party.Seats[0]
		if got.PlayerWon != solo.PlayerWon || got.PlayerEndHP != solo.PlayerEndHP ||
			got.EnemyEndHP != solo.EnemyEndHP || got.TotalRounds != solo.TotalRounds ||
			got.TimedOut != solo.TimedOut {
			t.Fatalf("seed %d: one-seat party diverged from solo\n solo=%+v\nparty=%+v", seed, solo, got)
		}
		if len(got.Events) != len(solo.Events) {
			t.Fatalf("seed %d: event count %d != solo %d", seed, len(got.Events), len(solo.Events))
		}
		for i := range solo.Events {
			if got.Events[i] != solo.Events[i] {
				t.Fatalf("seed %d: event %d differs\n solo=%+v\nparty=%+v", seed, i, solo.Events[i], got.Events[i])
			}
		}
	}
}

// A solo fight never stamps a seat, because seat 0 is the zero value and the
// field is omitempty. If this regresses, the golden file moves.
func TestSimulateCombat_SoloEventsCarryNoSeat(t *testing.T) {
	res := simulateCombatWithRNG(basePlayer(), baseEnemy(), dungeonCombatPhases, seededRNG(7))
	for i, e := range res.Events {
		if e.Seat != 0 {
			t.Fatalf("event %d (%s/%s) stamped seat %d in a solo fight", i, e.Phase, e.Action, e.Seat)
		}
	}
}

// The whole point of P6e. Three characters swinging at one monster must land more
// player attacks per round than one character does — before this, the members
// stood in the doorway while the leader fought.
func TestSimulateParty_EverySeatSwings(t *testing.T) {
	tank := baseEnemy()
	tank.Stats.MaxHP = 5000 // outlast the phase clock so we can count swings

	countSwings := func(events []CombatEvent, seat int) int {
		n := 0
		for _, e := range events {
			if e.Actor == "player" && e.Seat == seat && e.Roll > 0 {
				n++
			}
		}
		return n
	}

	res := simulatePartyWithRNG(
		[]Combatant{basePlayer(), basePlayer(), basePlayer()}, tank, dungeonCombatPhases, seededRNG(11))

	if len(res.Seats) != 3 {
		t.Fatalf("seated %d, want 3", len(res.Seats))
	}
	for seat := range 3 {
		if got := countSwings(res.Events, seat); got == 0 {
			t.Fatalf("seat %d never swung — the party is not fighting together", seat)
		}
	}
}

// A member going down is not the end of the fight. Only an empty roster is.
// This is the bug P7 measured from the outside: every party loss was the leader
// dying alone, because a member was never in the fight to begin with.
func TestSimulateParty_DownedMemberDoesNotEndTheFight(t *testing.T) {
	glass := basePlayer()
	glass.Stats.MaxHP = 1
	glass.Stats.Defense = 0

	brute := basePlayer()
	brute.Stats.MaxHP = 4000

	killer := baseEnemy()
	killer.Stats.MaxHP = 3000
	killer.Stats.Attack = 40

	res := simulatePartyWithRNG([]Combatant{brute, glass}, killer, dungeonCombatPhases, seededRNG(3))

	if res.Seats[1].PlayerEndHP > 0 {
		t.Skip("the glass cannon survived this seed; nothing to assert")
	}
	if !res.AnySurvivor() {
		t.Fatalf("both seats down — pick a seed where the brute lives")
	}
	if res.TotalRounds <= 1 {
		t.Fatalf("fight ended in %d round(s) when a member fell; the roster should have fought on",
			res.TotalRounds)
	}
	// The brute must have kept swinging after the member fell.
	lastMemberEvent, lastBruteEvent := -1, -1
	for i, e := range res.Events {
		if e.Seat == 1 && e.Actor == "player" {
			lastMemberEvent = i
		}
		if e.Seat == 0 && e.Actor == "player" && e.Roll > 0 {
			lastBruteEvent = i
		}
	}
	if lastBruteEvent < lastMemberEvent {
		t.Fatalf("the fight stopped when seat 1 fell (last brute swing %d, last member event %d)",
			lastBruteEvent, lastMemberEvent)
	}
}

// P8 scaling constants: solo is exempt on both levers (1 action, ×1.0 HP) so the
// characterization golden and the d8prereq corpus are untouched. A party's action
// budget is a fractional expectation — 2.4 for a duo (so it lands between soloing
// and a trio, where an integer 2 vs 3 has no room), 2N−1 for N≥3 — plus ×1.15
// enemy HP.
func TestP8PartyScaling_SoloExemptPartyScaled(t *testing.T) {
	if got := partyActionExpectation(1); got != 1 {
		t.Fatalf("solo action expectation = %v, want 1", got)
	}
	if got := partyEnemyHPScale(1); got != 1.0 {
		t.Fatalf("solo HP scale = %v, want 1.0", got)
	}
	if got := scaledEnemyMaxHP(200, 1); got != 200 {
		t.Fatalf("solo enemy HP = %d, want 200 (unscaled)", got)
	}
	if got := partyActionExpectation(2); got != 2.4 {
		t.Fatalf("duo action expectation = %v, want 2.4", got)
	}
	// The curve now takes a fractional weight rather than a head count, so that a
	// below-median seat costs the enemy less than a peer does. Every INTEGER input
	// must still return exactly what it always returned — that is what keeps solo
	// and a party of peers byte-identical, and the balance corpus with them.
	for n := 3; n <= 5; n++ {
		if got, want := partyActionExpectation(float64(n)), float64(2*n-1); got != want {
			t.Fatalf("party of %d: action expectation = %v, want %v", n, got, want)
		}
	}
	// A duo carrying a half-strength body sits between soloing and a true duo.
	if got := partyActionExpectation(1.5); got <= 1 || got >= 2.4 {
		t.Fatalf("weight 1.5 action expectation = %v, want strictly between 1 and 2.4", got)
	}
	if got := partyEnemyHPScale(1.5); got <= 1.0 || got >= 1.15 {
		t.Fatalf("weight 1.5 HP scale = %v, want strictly between 1.0 and 1.15", got)
	}
	if got := partyEnemyHPScale(3); got != 1.15 {
		t.Fatalf("party HP scale = %v, want 1.15", got)
	}
	// int(200*1.15) truncates 229.99… to 229; the 1-HP floor is immaterial.
	if got := scaledEnemyMaxHP(200, 3); got != 229 {
		t.Fatalf("party enemy HP = %d, want 229 (trunc 200*1.15)", got)
	}
}

// P8: the enemy's action economy scales with the roster. A party of N faces up to
// N attack-actions a round, each re-targeted, so the enemy's damage spreads across
// the roster instead of pinning one seat — the 1/N² exposure that made P6e's party
// a 100%-clear faceroll. Solo stays at exactly one swing a round, the pre-party
// behaviour the characterization golden pins.
func TestSimulateParty_EnemyActionEconomyScalesWithRoster(t *testing.T) {
	enemyRollsPerRound := func(res PartyCombatResult) map[int]int {
		perRound := map[int]int{}
		for _, e := range res.Events {
			if e.Actor == "enemy" && e.Roll > 0 {
				perRound[e.Round]++
			}
		}
		return perRound
	}

	// Solo: never more than one enemy swing in a round.
	soloTank := baseEnemy()
	soloTank.Stats.MaxHP = 5000
	solo := simulatePartyWithRNG(
		[]Combatant{basePlayer()}, soloTank, dungeonCombatPhases, seededRNG(23))
	if len(enemyRollsPerRound(solo)) == 0 {
		t.Fatal("solo: the enemy never attacked")
	}
	for round, n := range enemyRollsPerRound(solo) {
		if n > 1 {
			t.Fatalf("solo round %d: enemy swung %d times, want at most 1", round, n)
		}
	}

	// Party of 3: no round exceeds the roster's action budget, and at least one
	// round sees more than one swing — the enemy is really taking extra actions.
	tank := baseEnemy()
	tank.Stats.MaxHP = 5000
	party := simulatePartyWithRNG(
		[]Combatant{basePlayer(), basePlayer(), basePlayer()}, tank, dungeonCombatPhases, seededRNG(23))
	budget := int(partyActionExpectation(3))
	if partyActionExpectation(3) > float64(budget) {
		budget++ // ceil: the coin-flip round can add one action
	}
	sawMulti := false
	for round, n := range enemyRollsPerRound(party) {
		if n > budget {
			t.Fatalf("party round %d: enemy swung %d times, over the %d-action budget", round, n, budget)
		}
		if n > 1 {
			sawMulti = true
		}
	}
	if !sawMulti {
		t.Fatal("a party of 3 never faced more than one enemy swing in a round — action economy did not scale")
	}
}

// Over many seeds the enemy must spread its attention across the roster, or
// "targeting" is really "always seat 0" and members take no risk.
func TestSimulateParty_EnemySpreadsItsTargetsAcrossTheRoster(t *testing.T) {
	hit := map[int]bool{}
	for seed := uint64(1); seed <= 30; seed++ {
		tank := baseEnemy()
		tank.Stats.MaxHP = 5000
		res := simulatePartyWithRNG(
			[]Combatant{basePlayer(), basePlayer(), basePlayer()}, tank, dungeonCombatPhases, seededRNG(seed))
		for _, e := range res.Events {
			if e.Actor == "enemy" && e.Roll > 0 {
				hit[e.Seat] = true
			}
		}
	}
	for seat := range 3 {
		if !hit[seat] {
			t.Fatalf("seat %d was never targeted across 30 seeds — the enemy is not really choosing", seat)
		}
	}
}

// Per-seat close-out reads its own events, not the party's. If eventsForSeat
// leaked, a member's potions would be burned for the leader's heals.
func TestEventsForSeat_PartitionsTheLog(t *testing.T) {
	events := []CombatEvent{
		{Action: "heal_item", Seat: 0},
		{Action: "heal_item", Seat: 1},
		{Action: "heal_item", Seat: 1},
		{Action: "regen_tick"}, // unstamped: reads as the leader's
	}
	if got := len(eventsForSeat(events, 0)); got != 2 {
		t.Fatalf("seat 0 saw %d events, want 2", got)
	}
	if got := countHealEventsFired(CombatResult{Events: eventsForSeat(events, 1)}); got != 2 {
		t.Fatalf("seat 1 fired %d heal items, want 2", got)
	}
	if got := countHealEventsFired(CombatResult{Events: eventsForSeat(events, 0)}); got != 1 {
		t.Fatalf("seat 0 fired %d heal items, want 1", got)
	}
}

// A party that wins with a casualty is a win, and the casualty is still a
// casualty: survival is read per seat off HP, never off the fight's outcome.
func TestAnySurvivor_ReadsHPNotOutcome(t *testing.T) {
	res := PartyCombatResult{
		PlayerWon: true,
		Seats: []CombatResult{
			{PlayerEndHP: 12},
			{PlayerEndHP: 0},
			{PlayerEndHP: 3},
		},
	}
	if !res.AnySurvivor() {
		t.Fatal("AnySurvivor said nobody lived")
	}

	// A won fight nobody walked away from is still nobody walking away.
	wiped := PartyCombatResult{PlayerWon: true, Seats: []CombatResult{{PlayerEndHP: 0}}}
	if wiped.AnySurvivor() {
		t.Fatal("AnySurvivor counted a downed seat as standing")
	}
}
