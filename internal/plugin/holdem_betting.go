package plugin

import (
	"fmt"
	"sort"

	"maunium.net/go/mautrix/id"
)

// postBlinds posts SB and BB, handling heads-up edge case.
func (g *HoldemGame) postBlinds() (sbIdx, bbIdx int) {
	inHand := g.inHandPlayers()
	n := len(inHand)

	if n == 2 {
		// Heads-up: dealer posts SB, other posts BB.
		sbIdx = g.DealerIdx
		bbIdx = g.nextActiveIdx(g.DealerIdx)
	} else {
		sbIdx = g.nextActiveIdx(g.DealerIdx)
		bbIdx = g.nextActiveIdx(sbIdx)
	}

	// Post small blind.
	sb := g.Players[sbIdx]
	sbAmount := g.SmallBlind
	if sbAmount > sb.Stack {
		sbAmount = sb.Stack
	}
	sb.Stack -= sbAmount
	sb.Bet = sbAmount
	sb.TotalBet = sbAmount
	if sb.Stack == 0 {
		sb.State = PlayerAllIn
	}

	// Post big blind.
	bb := g.Players[bbIdx]
	bbAmount := g.BigBlind
	if bbAmount > bb.Stack {
		bbAmount = bb.Stack
	}
	bb.Stack -= bbAmount
	bb.Bet = bbAmount
	bb.TotalBet = bbAmount
	if bb.Stack == 0 {
		bb.State = PlayerAllIn
	}

	g.CurrentBet = g.BigBlind
	g.MinRaise = g.BigBlind
	g.LastAggressorIdx = bbIdx // BB has option

	return sbIdx, bbIdx
}

// firstToActPreflop returns the seat index of the first player to act preflop.
func (g *HoldemGame) firstToActPreflop(bbIdx int) int {
	n := len(g.Players)
	if n == 2 {
		// Heads-up: dealer/SB acts first preflop.
		return g.DealerIdx
	}
	// UTG = next active after BB.
	return g.nextCanActIdx(bbIdx)
}

// firstToActPostflop returns the first player to act on post-flop streets.
func (g *HoldemGame) firstToActPostflop() int {
	// First active player after dealer.
	return g.nextCanActIdx(g.DealerIdx)
}

// ActionResult describes what happened after an action.
type ActionResult struct {
	Announcement string
	HandOver     bool   // only 1 player remains
	StreetOver   bool   // street betting is complete
	AllAllIn     bool   // all remaining players are all-in
}

// doFold processes a fold action.
func (g *HoldemGame) doFold(seatIdx int) ActionResult {
	p := g.Players[seatIdx]
	p.State = PlayerFolded
	g.StreetHistory += "f"

	ann := renderActionAnnouncement(p.DisplayName, "fold", 0)

	if g.activeCount() == 1 {
		return ActionResult{Announcement: ann, HandOver: true}
	}

	return ActionResult{
		Announcement: ann,
		StreetOver:   g.isStreetComplete(g.nextCanActIdx(seatIdx)),
		AllAllIn:     g.canActCount() == 0,
	}
}

// doCheck processes a check action. Returns error string if invalid.
func (g *HoldemGame) doCheck(seatIdx int) (ActionResult, string) {
	p := g.Players[seatIdx]
	if p.Bet < g.CurrentBet {
		return ActionResult{}, "You must call, raise, or fold — there's a bet to you."
	}

	g.StreetHistory += "c"
	ann := renderActionAnnouncement(p.DisplayName, "check", 0)
	return ActionResult{
		Announcement: ann,
		StreetOver:   g.isStreetComplete(g.nextCanActIdx(seatIdx)),
	}, ""
}

// doCall processes a call action. Returns error string if nothing to call.
func (g *HoldemGame) doCall(seatIdx int) (ActionResult, string) {
	p := g.Players[seatIdx]
	toCall := g.CurrentBet - p.Bet
	if toCall <= 0 {
		return ActionResult{}, "Nothing to call. Use `!holdem check` instead."
	}
	if toCall > p.Stack {
		toCall = p.Stack
	}

	p.Stack -= toCall
	p.Bet += toCall
	p.TotalBet += toCall

	action := "call"
	if p.Stack == 0 {
		p.State = PlayerAllIn
		action = "allin"
		g.StreetHistory += "a"
	} else {
		g.StreetHistory += "c"
	}

	ann := renderActionAnnouncement(p.DisplayName, action, toCall)

	if g.activeCount() == 1 {
		return ActionResult{Announcement: ann, HandOver: true}, ""
	}

	return ActionResult{
		Announcement: ann,
		StreetOver:   g.isStreetComplete(g.nextCanActIdx(seatIdx)),
		AllAllIn:     g.canActCount() == 0,
	}, ""
}

// doRaise processes a raise action. raiseTo is the total bet amount.
func (g *HoldemGame) doRaise(seatIdx int, raiseTo int64) (ActionResult, string) {
	p := g.Players[seatIdx]

	minRaiseTo := g.CurrentBet + g.MinRaise
	maxRaiseTo := p.Bet + p.Stack

	if raiseTo < minRaiseTo && raiseTo < maxRaiseTo {
		return ActionResult{}, fmt.Sprintf("Minimum raise is to €%d.", minRaiseTo)
	}

	if raiseTo > maxRaiseTo {
		return ActionResult{}, fmt.Sprintf("You can raise to at most €%d (your stack).", maxRaiseTo)
	}

	raiseAmount := raiseTo - p.Bet
	actualRaise := raiseTo - g.CurrentBet

	p.Stack -= raiseAmount
	p.Bet = raiseTo
	p.TotalBet += raiseAmount

	if actualRaise > 0 {
		g.MinRaise = actualRaise
	}
	g.CurrentBet = raiseTo
	g.LastAggressorIdx = seatIdx

	action := "raise"
	if p.Stack == 0 {
		p.State = PlayerAllIn
		action = "allin"
		g.StreetHistory += "a"
	} else {
		// Approximate: >=75% of pot is a pot-size raise ('R'), otherwise half-pot ('r').
		totalPot := g.Pot
		for _, pp := range g.Players {
			totalPot += pp.Bet
		}
		if totalPot > 0 && float64(actualRaise) >= float64(totalPot)*0.75 {
			g.StreetHistory += "R"
		} else {
			g.StreetHistory += "r"
		}
	}

	ann := renderActionAnnouncement(p.DisplayName, action, raiseTo)

	return ActionResult{
		Announcement: ann,
		AllAllIn:     g.canActCount() == 0,
	}, ""
}

// doAllIn processes an all-in action.
func (g *HoldemGame) doAllIn(seatIdx int) ActionResult {
	p := g.Players[seatIdx]
	allInAmount := p.Stack
	totalBet := p.Bet + allInAmount

	p.Stack = 0
	p.Bet = totalBet
	p.TotalBet += allInAmount
	p.State = PlayerAllIn

	if totalBet > g.CurrentBet {
		actualRaise := totalBet - g.CurrentBet
		// Only reopen action if the raise meets the minimum.
		// A short all-in (under-raise) does NOT reopen betting.
		if actualRaise >= g.MinRaise {
			g.MinRaise = actualRaise
			g.LastAggressorIdx = seatIdx
		}
		g.CurrentBet = totalBet
	}

	g.StreetHistory += "a"
	ann := renderActionAnnouncement(p.DisplayName, "allin", totalBet)

	if g.activeCount() == 1 {
		return ActionResult{Announcement: ann, HandOver: true}
	}

	return ActionResult{
		Announcement: ann,
		StreetOver:   g.isStreetComplete(g.nextCanActIdx(seatIdx)),
		AllAllIn:     g.canActCount() == 0,
	}
}

// isStreetComplete checks if the betting round is done.
func (g *HoldemGame) isStreetComplete(nextIdx int) bool {
	// All active players have matched the current bet and action has returned to the last aggressor.
	if g.canActCount() == 0 {
		return true
	}

	// Check if all Active players have matched the bet.
	for _, p := range g.Players {
		if p.State == PlayerActive && p.Bet != g.CurrentBet {
			return false
		}
	}

	// If the last aggressor is all-in (can't act), the street is done when
	// all active players have matched the bet (already checked above).
	if g.Players[g.LastAggressorIdx].State == PlayerAllIn {
		return true
	}

	// Action must have gone around to the last aggressor.
	return nextIdx == g.LastAggressorIdx
}

// buildSidePots creates side pots when all-ins are present.
func (g *HoldemGame) buildSidePots() {
	// Collect all bets.
	g.collectPot()

	type betEntry struct {
		uid id.UserID
		bet int64
	}

	var entries []betEntry
	for _, p := range g.Players {
		if p.State != PlayerFolded && p.State != PlayerSatOut {
			entries = append(entries, betEntry{p.UserID, p.TotalBet})
		}
	}

	if len(entries) == 0 {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].bet < entries[j].bet
	})

	var sidePots []SidePot
	prevLevel := int64(0)

	for _, e := range entries {
		if e.bet <= prevLevel {
			continue
		}

		level := e.bet
		potSlice := int64(0)

		// All players who bet >= level contribute (level - prevLevel) each.
		var eligible []id.UserID
		for _, p := range g.Players {
			if p.State == PlayerFolded || p.State == PlayerSatOut {
				// Folded players still contributed up to their TotalBet.
				contrib := p.TotalBet - prevLevel
				if contrib > level-prevLevel {
					contrib = level - prevLevel
				}
				if contrib > 0 {
					potSlice += contrib
				}
				continue
			}

			contrib := p.TotalBet - prevLevel
			if contrib > level-prevLevel {
				contrib = level - prevLevel
			}
			if contrib > 0 {
				potSlice += contrib
			}
			if p.TotalBet >= level {
				eligible = append(eligible, p.UserID)
			}
		}

		if potSlice > 0 {
			sidePots = append(sidePots, SidePot{Amount: potSlice, Eligible: eligible})
		}

		prevLevel = level
	}

	if len(sidePots) > 0 {
		g.SidePots = sidePots
		g.Pot = 0
	}
}

// returnUncalledBet returns any unmatched portion of a bet to the player.
func (g *HoldemGame) returnUncalledBet() (name string, amount int64) {
	// Find the highest and second-highest bets among non-folded players.
	var highest, secondHighest int64
	var highestIdx int

	for i, p := range g.Players {
		if p.State == PlayerFolded || p.State == PlayerSatOut {
			continue
		}
		if p.TotalBet > highest {
			secondHighest = highest
			highest = p.TotalBet
			highestIdx = i
		} else if p.TotalBet > secondHighest {
			secondHighest = p.TotalBet
		}
	}

	excess := highest - secondHighest
	if excess > 0 && secondHighest > 0 {
		p := g.Players[highestIdx]
		p.Stack += excess
		p.TotalBet -= excess
		p.Bet -= excess
		if p.Bet < 0 {
			p.Bet = 0
		}
		return p.DisplayName, excess
	}

	return "", 0
}

