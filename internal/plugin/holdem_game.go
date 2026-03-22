package plugin

import (
	"math/rand/v2"
	"time"

	"github.com/chehsunliu/poker"
	"maunium.net/go/mautrix/id"
)

// Street represents the current phase of a Hold'em hand.
type Street int

const (
	StreetPreFlop Street = iota
	StreetFlop
	StreetTurn
	StreetRiver
	StreetShowdown
)

func (s Street) String() string {
	switch s {
	case StreetPreFlop:
		return "Pre-Flop"
	case StreetFlop:
		return "Flop"
	case StreetTurn:
		return "Turn"
	case StreetRiver:
		return "River"
	case StreetShowdown:
		return "Showdown"
	default:
		return "Unknown"
	}
}

// PlayerState tracks a player's status within a hand.
type PlayerState int

const (
	PlayerActive PlayerState = iota
	PlayerFolded
	PlayerAllIn
	PlayerSatOut
)

// SidePot represents a pot with specific eligible players.
type SidePot struct {
	Amount   int64
	Eligible []id.UserID
}

// HoldemPlayer represents a seated player.
type HoldemPlayer struct {
	UserID       id.UserID
	DisplayName  string
	Stack        int64
	OpeningStack int64
	Hole         [2]poker.Card
	Bet          int64 // committed this street
	TotalBet     int64 // committed this hand
	State        PlayerState
	TipsEnabled  bool
	SittingOut   bool
	WantsLeave   bool
	IsNPC        bool
	DMRoomID     id.RoomID
}

// HoldemGame holds all state for one table.
type HoldemGame struct {
	RoomID            id.RoomID
	Players           []*HoldemPlayer
	Community         []poker.Card
	Deck              []poker.Card
	DeckPos           int // position in the deck
	Pot               int64
	SidePots          []SidePot
	Street            Street
	DealerIdx         int
	ActionIdx         int
	CurrentBet        int64
	MinRaise          int64
	SmallBlind        int64
	BigBlind          int64
	LastAggressorIdx  int
	WaitingForPlayers bool
	HandInProgress    bool
	StreetHistory     string // action chars for current street (f/c/r/R/a) for CFR policy lookup

	actionTimer  *time.Timer
	warningTimer *time.Timer
}

// newShuffledDeck creates a shuffled 52-card deck.
func newShuffledDeck() []poker.Card {
	cards := allCards()
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return cards
}

// drawCard draws the next card from the deck.
func (g *HoldemGame) drawCard() poker.Card {
	c := g.Deck[g.DeckPos]
	g.DeckPos++
	return c
}

// burnCard discards the top card (standard casino burn).
func (g *HoldemGame) burnCard() {
	g.DeckPos++
}

// activePlayers returns players who are Active or AllIn.
func (g *HoldemGame) activePlayers() []*HoldemPlayer {
	var result []*HoldemPlayer
	for _, p := range g.Players {
		if p.State == PlayerActive || p.State == PlayerAllIn {
			result = append(result, p)
		}
	}
	return result
}

// activeCount returns the number of Active or AllIn players.
func (g *HoldemGame) activeCount() int {
	n := 0
	for _, p := range g.Players {
		if p.State == PlayerActive || p.State == PlayerAllIn {
			n++
		}
	}
	return n
}

// canActCount returns the number of players who can still act (Active with stack > 0).
func (g *HoldemGame) canActCount() int {
	n := 0
	for _, p := range g.Players {
		if p.State == PlayerActive {
			n++
		}
	}
	return n
}

// nextActiveIdx returns the next seat index after idx with Active or AllIn state.
func (g *HoldemGame) nextActiveIdx(idx int) int {
	n := len(g.Players)
	for i := 1; i < n; i++ {
		next := (idx + i) % n
		p := g.Players[next]
		if p.State == PlayerActive || p.State == PlayerAllIn {
			return next
		}
	}
	return idx
}

// nextCanActIdx returns the next seat with Active state (can take actions).
func (g *HoldemGame) nextCanActIdx(idx int) int {
	n := len(g.Players)
	for i := 1; i < n; i++ {
		next := (idx + i) % n
		if g.Players[next].State == PlayerActive {
			return next
		}
	}
	return idx
}

// inHandPlayers returns players participating in the current hand (not SatOut).
func (g *HoldemGame) inHandPlayers() []*HoldemPlayer {
	var result []*HoldemPlayer
	for _, p := range g.Players {
		if p.State != PlayerSatOut {
			result = append(result, p)
		}
	}
	return result
}

// playerByUserID finds a player by their Matrix user ID.
func (g *HoldemGame) playerByUserID(uid id.UserID) *HoldemPlayer {
	for _, p := range g.Players {
		if p.UserID == uid {
			return p
		}
	}
	return nil
}

// playerIdx returns the seat index for a user ID, or -1.
func (g *HoldemGame) playerIdx(uid id.UserID) int {
	for i, p := range g.Players {
		if p.UserID == uid {
			return i
		}
	}
	return -1
}

// resetStreetBets clears per-street bet tracking.
func (g *HoldemGame) resetStreetBets() {
	for _, p := range g.Players {
		p.Bet = 0
	}
	g.CurrentBet = 0
	g.MinRaise = g.BigBlind
	g.StreetHistory = ""
}

// collectPot moves all bets into the pot.
func (g *HoldemGame) collectPot() {
	for _, p := range g.Players {
		g.Pot += p.Bet
		p.Bet = 0
	}
}

// positionLabel returns BTN/SB/BB/UTG/MP/CO for a seat index.
func (g *HoldemGame) positionLabel(seatIdx int) string {
	n := len(g.inHandPlayers())
	if n <= 1 {
		return ""
	}

	if seatIdx == g.DealerIdx {
		return "BTN"
	}

	headsUp := n == 2
	if headsUp {
		// In heads-up, dealer is SB; other is BB.
		return "BB"
	}

	sbIdx := g.nextActiveIdx(g.DealerIdx)
	bbIdx := g.nextActiveIdx(sbIdx)

	if seatIdx == sbIdx {
		return "SB"
	}
	if seatIdx == bbIdx {
		return "BB"
	}

	utgIdx := g.nextActiveIdx(bbIdx)
	if seatIdx == utgIdx {
		return "UTG"
	}

	// Rough labeling for remaining seats.
	// Count distance from UTG.
	dist := 0
	cur := utgIdx
	for i := 0; i < n; i++ {
		cur = g.nextActiveIdx(cur)
		dist++
		if cur == seatIdx {
			break
		}
	}

	remaining := n - 4 // seats after UTG before BTN
	if remaining <= 0 {
		return "MP"
	}
	if dist >= remaining {
		return "CO"
	}
	return "MP"
}
