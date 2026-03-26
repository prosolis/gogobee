package plugin

import (
	"fmt"
	"sort"

	"github.com/chehsunliu/poker"
	"maunium.net/go/mautrix/id"
)

// handRank evaluates a player's best 5-card hand from 7 cards.
func handRank(hole [2]poker.Card, community []poker.Card) (int32, string) {
	cards := make([]poker.Card, 0, 7)
	cards = append(cards, hole[0], hole[1])
	cards = append(cards, community...)
	rank := poker.Evaluate(cards)
	return rank, poker.RankString(rank)
}

type evaluatedPlayer struct {
	seatIdx int
	rank    int32
	name    string
	userID  id.UserID
}

// runShowdown evaluates all hands and distributes pots. Returns showdown result lines and per-player winnings.
func runShowdown(g *HoldemGame) ([]showdownResult, map[id.UserID]int64) {
	winnings := make(map[id.UserID]int64)
	var results []showdownResult

	// Evaluate all non-folded players.
	var evaluated []evaluatedPlayer
	for i, p := range g.Players {
		if p.State == PlayerFolded || p.State == PlayerSatOut {
			continue
		}
		rank, name := handRank(p.Hole, g.Community)
		evaluated = append(evaluated, evaluatedPlayer{
			seatIdx: i,
			rank:    rank,
			name:    name,
			userID:  p.UserID,
		})
	}

	sort.Slice(evaluated, func(i, j int) bool {
		return evaluated[i].rank < evaluated[j].rank // lower = better
	})

	if len(g.SidePots) > 0 {
		// Distribute each side pot.
		for _, sp := range g.SidePots {
			distributePot(g, sp.Amount, sp.Eligible, evaluated, winnings, &results)
		}
	} else {
		// Single pot — collect outstanding bets first.
		g.collectPot()
		eligible := make([]id.UserID, 0)
		for _, e := range evaluated {
			eligible = append(eligible, e.userID)
		}
		distributePot(g, g.Pot, eligible, evaluated, winnings, &results)
		g.Pot = 0
	}

	// Add showdown lines for all players.
	var showdownLines []showdownResult
	for _, e := range evaluated {
		p := g.Players[e.seatIdx]
		won := winnings[p.UserID]
		line := renderShowdownLine(p.DisplayName, p.Hole, e.name, won)
		showdownLines = append(showdownLines, showdownResult{line: line})
	}

	return showdownLines, winnings
}

// distributePot distributes a pot among eligible winners.
func distributePot(g *HoldemGame, potAmount int64, eligible []id.UserID, evaluated []evaluatedPlayer, winnings map[id.UserID]int64, results *[]showdownResult) {
	if potAmount == 0 {
		return
	}

	eligibleSet := make(map[id.UserID]bool, len(eligible))
	for _, uid := range eligible {
		eligibleSet[uid] = true
	}

	// Find the best rank among eligible players.
	var winners []evaluatedPlayer
	bestRank := int32(7463)
	for _, e := range evaluated {
		if !eligibleSet[e.userID] {
			continue
		}
		if e.rank < bestRank {
			bestRank = e.rank
			winners = []evaluatedPlayer{e}
		} else if e.rank == bestRank {
			winners = append(winners, e)
		}
	}

	if len(winners) == 0 {
		return
	}

	// Split pot.
	share := potAmount / int64(len(winners))
	remainder := potAmount % int64(len(winners))

	for i, w := range winners {
		won := share
		if i == 0 {
			won += remainder // leftmost seat gets the odd chip
		}
		g.Players[w.seatIdx].Stack += won
		winnings[w.userID] += won
	}
}

// awardPotToLastPlayer awards the entire pot to the only remaining player (all others folded).
func awardPotToLastPlayer(g *HoldemGame) (string, id.UserID) {
	g.collectPot()

	var winner *HoldemPlayer
	for _, p := range g.Players {
		if p.State != PlayerFolded && p.State != PlayerSatOut {
			winner = p
			break
		}
	}

	if winner == nil {
		return "", ""
	}

	winner.Stack += g.Pot
	ann := fmt.Sprintf("🏆 **%s** wins €%d!", winner.DisplayName, g.Pot)
	g.Pot = 0
	return ann, winner.UserID
}

// settleNetDeltas is intentionally a no-op.
// Economy settlement happens at leave time: buy-in is debited at join,
// full remaining stack is credited at leave/cashout. Per-hand settlement
// would double-count because the stack already reflects cumulative results.
func settleNetDeltas(_ *HoldemGame, _ *EuroPlugin) {}
