package plugin

import (
	"testing"

	"github.com/chehsunliu/poker"
	"maunium.net/go/mautrix/id"
)

func TestHandRank_HighCard(t *testing.T) {
	hole := [2]poker.Card{
		poker.NewCard("2s"),
		poker.NewCard("7h"),
	}
	community := []poker.Card{
		poker.NewCard("9d"),
		poker.NewCard("Jc"),
		poker.NewCard("Ks"),
		poker.NewCard("4h"),
		poker.NewCard("3d"),
	}
	rank, _ := handRank(hole, community)
	if rank <= 0 {
		t.Errorf("expected positive rank, got %d", rank)
	}
}

func TestHandRank_Pair(t *testing.T) {
	hole := [2]poker.Card{
		poker.NewCard("As"),
		poker.NewCard("Ah"),
	}
	community := []poker.Card{
		poker.NewCard("2d"),
		poker.NewCard("5c"),
		poker.NewCard("9s"),
		poker.NewCard("Jh"),
		poker.NewCard("Kd"),
	}
	_, name := handRank(hole, community)
	if name != "Pair" {
		t.Errorf("expected Pair, got %q", name)
	}
}

func TestHandRank_Flush(t *testing.T) {
	hole := [2]poker.Card{
		poker.NewCard("2s"),
		poker.NewCard("5s"),
	}
	community := []poker.Card{
		poker.NewCard("8s"),
		poker.NewCard("Js"),
		poker.NewCard("Ks"),
		poker.NewCard("3h"),
		poker.NewCard("7d"),
	}
	_, name := handRank(hole, community)
	if name != "Flush" {
		t.Errorf("expected Flush, got %q", name)
	}
}

func TestHandRank_FullHouse(t *testing.T) {
	hole := [2]poker.Card{
		poker.NewCard("As"),
		poker.NewCard("Ah"),
	}
	community := []poker.Card{
		poker.NewCard("Ad"),
		poker.NewCard("Kc"),
		poker.NewCard("Ks"),
		poker.NewCard("2h"),
		poker.NewCard("3d"),
	}
	_, name := handRank(hole, community)
	if name != "Full House" {
		t.Errorf("expected Full House, got %q", name)
	}
}

func TestHandRank_BetterHandHasLowerRank(t *testing.T) {
	community := []poker.Card{
		poker.NewCard("2d"),
		poker.NewCard("5c"),
		poker.NewCard("9s"),
		poker.NewCard("Jh"),
		poker.NewCard("Kd"),
	}
	// Player 1: pair of aces
	pairHole := [2]poker.Card{poker.NewCard("As"), poker.NewCard("Ah")}
	pairRank, _ := handRank(pairHole, community)

	// Player 2: high card 7
	highHole := [2]poker.Card{poker.NewCard("3s"), poker.NewCard("7h")}
	highRank, _ := handRank(highHole, community)

	// Lower rank = better hand in poker lib
	if pairRank >= highRank {
		t.Errorf("pair (rank %d) should beat high card (rank %d) — lower is better", pairRank, highRank)
	}
}

func TestDistributePot_SingleWinner(t *testing.T) {
	g := &HoldemGame{
		Players: make([]*HoldemPlayer, 2),
	}
	g.Players[0] = &HoldemPlayer{UserID: "@alice:test", Stack: 0}
	g.Players[1] = &HoldemPlayer{UserID: "@bob:test", Stack: 0}

	eligible := []id.UserID{"@alice:test", "@bob:test"}
	evaluated := []evaluatedPlayer{
		{seatIdx: 0, rank: 100, name: "Flush", userID: "@alice:test"},
		{seatIdx: 1, rank: 500, name: "Pair", userID: "@bob:test"},
	}
	winnings := make(map[id.UserID]int64)
	var results []showdownResult

	distributePot(g, 1000, eligible, evaluated, winnings, &results)

	if winnings["@alice:test"] != 1000 {
		t.Errorf("alice should win 1000, got %d", winnings["@alice:test"])
	}
	if winnings["@bob:test"] != 0 {
		t.Errorf("bob should win 0, got %d", winnings["@bob:test"])
	}
}

func TestDistributePot_SplitPot(t *testing.T) {
	g := &HoldemGame{
		Players: make([]*HoldemPlayer, 2),
	}
	g.Players[0] = &HoldemPlayer{UserID: "@alice:test", Stack: 0}
	g.Players[1] = &HoldemPlayer{UserID: "@bob:test", Stack: 0}

	eligible := []id.UserID{"@alice:test", "@bob:test"}
	evaluated := []evaluatedPlayer{
		{seatIdx: 0, rank: 100, name: "Flush", userID: "@alice:test"},
		{seatIdx: 1, rank: 100, name: "Flush", userID: "@bob:test"}, // tie
	}
	winnings := make(map[id.UserID]int64)
	var results []showdownResult

	distributePot(g, 1000, eligible, evaluated, winnings, &results)

	// Should split evenly; odd chip goes to leftmost seat
	if winnings["@alice:test"] != 500 {
		t.Errorf("alice should win 500, got %d", winnings["@alice:test"])
	}
	if winnings["@bob:test"] != 500 {
		t.Errorf("bob should win 500, got %d", winnings["@bob:test"])
	}
}

func TestDistributePot_OddChip(t *testing.T) {
	g := &HoldemGame{
		Players: make([]*HoldemPlayer, 3),
	}
	g.Players[0] = &HoldemPlayer{UserID: "@alice:test", Stack: 0}
	g.Players[1] = &HoldemPlayer{UserID: "@bob:test", Stack: 0}
	g.Players[2] = &HoldemPlayer{UserID: "@carol:test", Stack: 0}

	eligible := []id.UserID{"@alice:test", "@bob:test", "@carol:test"}
	evaluated := []evaluatedPlayer{
		{seatIdx: 0, rank: 100, name: "Flush", userID: "@alice:test"},
		{seatIdx: 1, rank: 100, name: "Flush", userID: "@bob:test"},
		{seatIdx: 2, rank: 100, name: "Flush", userID: "@carol:test"},
	}
	winnings := make(map[id.UserID]int64)
	var results []showdownResult

	distributePot(g, 100, eligible, evaluated, winnings, &results)

	total := winnings["@alice:test"] + winnings["@bob:test"] + winnings["@carol:test"]
	if total != 100 {
		t.Errorf("total winnings should be 100, got %d", total)
	}
	// Leftmost seat gets the odd chip
	if winnings["@alice:test"] < winnings["@bob:test"] || winnings["@alice:test"] < winnings["@carol:test"] {
		t.Errorf("leftmost seat should get odd chip: alice=%d bob=%d carol=%d",
			winnings["@alice:test"], winnings["@bob:test"], winnings["@carol:test"])
	}
}
