package plugin

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Party characterization — the net that did not exist.
//
// TestCombatCharacterization pins SOLO combat exhaustively, and it has been the
// tripwire for every balance change since. Party combat had nothing. The whole
// N-body layer — initiative order, per-seat resolution, enemy targeting, the P8
// action-economy and HP scaling, downed-seat handling — shipped unpinned.
//
// What that cost: N3 shipped a Cleric class that cannot heal a single ally (no
// action in the engine can target another seat) and not one test went red. The
// hired companion then stood in fights doing nothing, and the unit tests stayed
// green through that too. It took a 1500-run expedition sweep to see either.
//
// So: pin it. Any change to the party engine now has to state, in the diff, what
// it moved. Regenerate ONLY on purpose:
//
//	go test ./internal/plugin -run TestPartyCharacterization -update
//
// This golden covers simulateParty — the auto-resolve engine, which decides most
// expedition rooms and which the balance harness is built on. The turn engine's
// party half (manual play + boss fights) needs DB fixtures and is pinned by
// combat_turn_party_test.go; widening THAT into a golden is the follow-up.

var updatePartyGolden = flag.Bool("update-party", false, "rewrite the party characterization golden")

// partyScenario is one pinned N-body fight.
type partyScenario struct {
	name   string
	seats  []Combatant
	enemy  Combatant
	phases []CombatPhase
}

// seatOfClass shapes a seat that stands in for a class archetype. These are
// deliberately engine-level (stat blocks, not sheets): the golden pins what the
// ENGINE does with a roster, not what the character layer feeds it.
func seatOfClass(name string, hp, atk, def, speed int) Combatant {
	c := basePlayer()
	c.Name = name
	c.Stats.MaxHP = hp
	c.Stats.Attack = atk
	c.Stats.Defense = def
	c.Stats.Speed = speed
	return c
}

func partyCharacterizationScenarios() []partyScenario {
	tank := seatOfClass("Tank", 160, 14, 12, 8)
	striker := seatOfClass("Striker", 90, 22, 6, 14)
	support := seatOfClass("Support", 110, 11, 9, 10)
	// The seat this whole plan is about: a body that is real but below median.
	// If scaling ever stops overcharging for it, THIS line is what moves.
	weak := seatOfClass("Weak", 70, 8, 5, 9)
	// A seat that will fall early. Its corpse must not keep buffing the enemy —
	// when §2 lands, this scenario is the one that proves it.
	glass := seatOfClass("Glass", 12, 18, 0, 16)

	return []partyScenario{
		{"duo/even", []Combatant{tank, striker}, tankyEnemy(), dungeonCombatPhases},
		{"duo/tank+support", []Combatant{tank, support}, tankyEnemy(), dungeonCombatPhases},
		{"duo/median+weak", []Combatant{tank, weak}, tankyEnemy(), dungeonCombatPhases},
		{"duo/glass falls early", []Combatant{tank, glass}, hardHitEnemy(), dungeonCombatPhases},
		{"trio/even", []Combatant{tank, striker, support}, tankyEnemy(), dungeonCombatPhases},
		{"trio/one weak seat", []Combatant{tank, striker, weak}, tankyEnemy(), dungeonCombatPhases},
		{"trio/two glass seats", []Combatant{tank, glass, glass}, hardHitEnemy(), dungeonCombatPhases},
		{"duo/vs ability enemy", []Combatant{tank, striker},
			abilityEnemy("Wither", "poison", "Duel"), dungeonCombatPhases},
		// The degenerate case. A one-seat roster MUST stay bit-identical to solo —
		// it is the invariant the entire balance corpus rests on, and the reason
		// the solo golden is allowed to stay untouched while this file grows.
		{"solo/one-seat roster", []Combatant{basePlayer()}, baseEnemy(), dungeonCombatPhases},
	}
}

// formatPartyResult prints the seat on EVERY line. The solo formatter does not
// (it cannot; there is only one seat), and CombatEvent.Seat is `omitempty`, so a
// JSON trace renders seat 0 and "no seat" identically — which sent me chasing a
// phantom "the companion never swings" bug for an hour. A party golden that
// could not tell you WHO acted would be worth very little.
func formatPartyResult(r PartyCombatResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "result: won=%v timedOut=%v rounds=%d survivors=%v\n",
		r.PlayerWon, r.TimedOut, r.TotalRounds, r.AnySurvivor())
	fmt.Fprintf(&b, "  enemy: start=%d entry=%d end=%d\n", r.EnemyStartHP, r.EnemyEntryHP, r.EnemyEndHP)
	for i, s := range r.Seats {
		fmt.Fprintf(&b, "  seat[%d]: start=%d entry=%d end=%d\n",
			i, s.PlayerStartHP, s.PlayerEntryHP, s.PlayerEndHP)
	}
	for i, e := range r.Events {
		fmt.Fprintf(&b, "  [%02d] r%d seat=%d %-12s %-8s %-16s dmg=%-5d php=%-4d ehp=%-4d roll=%d/%d desc=%q\n",
			i, e.Round, e.Seat, e.Phase, e.Actor, e.Action, e.Damage, e.PlayerHP, e.EnemyHP,
			e.Roll, e.RollAgainst, e.Desc)
	}
	return b.String()
}

func TestPartyCharacterization(t *testing.T) {
	var b strings.Builder
	for _, sc := range partyCharacterizationScenarios() {
		for _, seed := range charSeeds {
			rng := rand.New(rand.NewPCG(seed, 0xC0FFEE))
			res := simulatePartyWithRNG(sc.seats, sc.enemy, sc.phases, rng)
			fmt.Fprintf(&b, "=== %s seed=%d ===\n", sc.name, seed)
			b.WriteString(formatPartyResult(res))
			b.WriteString("\n")
		}
	}
	got := b.String()

	path := filepath.Join("testdata", "party_characterization.golden")
	if *updatePartyGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("party golden rewritten:", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read party golden (run with -update-party to create it): %v", err)
	}
	if string(want) != got {
		t.Fatalf("PARTY ENGINE BEHAVIOUR MOVED.\n\n%s\n\n"+
			"If this was deliberate, say so in the commit and regenerate:\n"+
			"  go test ./internal/plugin -run TestPartyCharacterization -update-party",
			firstDiff(string(want), got))
	}
}

// The one-seat roster is the solo engine. If this ever fails, the N-body path has
// stopped being a superset of the path the entire balance corpus was measured on,
// and every baseline in the repo is suspect.
func TestPartyCharacterization_OneSeatIsStillSolo(t *testing.T) {
	for _, seed := range charSeeds {
		solo := simulateCombatWithRNG(basePlayer(), baseEnemy(), dungeonCombatPhases,
			rand.New(rand.NewPCG(seed, 0xC0FFEE)))
		party := simulatePartyWithRNG([]Combatant{basePlayer()}, baseEnemy(), dungeonCombatPhases,
			rand.New(rand.NewPCG(seed, 0xC0FFEE)))

		if solo.PlayerWon != party.PlayerWon || solo.TotalRounds != party.TotalRounds ||
			solo.EnemyEndHP != party.EnemyEndHP || solo.PlayerEndHP != party.Seats[0].PlayerEndHP {
			t.Fatalf("seed %d: a one-seat roster diverged from solo\n solo:  won=%v rounds=%d ehp=%d php=%d\n party: won=%v rounds=%d ehp=%d php=%d",
				seed,
				solo.PlayerWon, solo.TotalRounds, solo.EnemyEndHP, solo.PlayerEndHP,
				party.PlayerWon, party.TotalRounds, party.EnemyEndHP, party.Seats[0].PlayerEndHP)
		}
	}
}
