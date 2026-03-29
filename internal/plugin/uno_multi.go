package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ---------------------------------------------------------------------------
// Multiplayer UNO types
// ---------------------------------------------------------------------------

type unoMultiPhase int

const (
	unoMultiPhasePlay             unoMultiPhase = iota
	unoMultiPhaseChooseColor                    // active player must pick a color
	unoMultiPhaseDrawnPlayable                  // active player drew a playable card, yes/no
	unoMultiPhaseChallenge                      // next player may challenge a Wild Draw Four
	unoMultiPhaseChooseSwapTarget               // No Mercy: player played a 7, must choose swap target
)

type unoMultiPlayer struct {
	userID      id.UserID
	dmRoomID    id.RoomID
	hand        []unoCard
	calledUno   bool
	isBot       bool
	active      bool // false if forfeited/left
	autoPlays   int  // consecutive auto-plays
}

type unoMultiGame struct {
	id         string
	roomID     id.RoomID // games room
	ante       float64
	players    []*unoMultiPlayer // in turn order (includes bot)
	currentIdx int
	direction  int // +1 or -1
	drawPile   []unoCard
	discardTop unoCard
	topColor   unoColor
	phase      unoMultiPhase
	drawnCard   *unoCard  // card drawn this turn
	pendingCard *unoCard // wild waiting for color

	// Wild Draw Four challenge state
	wd4Player    *unoMultiPlayer // who played the WD4
	wd4Victim    *unoMultiPlayer // who can challenge
	wd4PrevColor unoColor        // color before the wild was played
	turns      int
	turnID     int // monotonic, used to invalidate stale timers
	startedAt  time.Time
	done       bool
	bookDown   bool

	// No Mercy mode
	noMercy       bool
	sevenZeroRule bool
	stackTotal    int // cumulative draw penalty during stacking
	stackMinValue int // minimum draw value to stack (0 = not stacking)

	// Sudden death (long-game point scoring)
	suddenDeath     bool
	suddenDeathTurn int

	timer         *time.Timer
	inactiveTimer *time.Timer // 10-minute game timeout
	mu            sync.Mutex  // per-game lock
}

type unoMultiLobby struct {
	roomID        id.RoomID
	creator       id.UserID
	ante          float64
	players       []id.UserID
	createdAt     time.Time
	timer         *time.Timer
	noMercy       bool
	sevenZeroRule bool
}

// ---------------------------------------------------------------------------
// Game helpers
// ---------------------------------------------------------------------------

func (g *unoMultiGame) currentPlayer() *unoMultiPlayer {
	return g.players[g.currentIdx]
}

func (g *unoMultiGame) nextActiveIdx() int {
	idx := g.currentIdx
	n := len(g.players)
	for i := 0; i < n; i++ {
		idx = (idx + g.direction + n) % n
		if g.players[idx].active {
			return idx
		}
	}
	return g.currentIdx // only one left
}

func (g *unoMultiGame) activePlayers() []*unoMultiPlayer {
	var active []*unoMultiPlayer
	for _, p := range g.players {
		if p.active {
			active = append(active, p)
		}
	}
	return active
}

func (g *unoMultiGame) activeHumanCount() int {
	count := 0
	for _, p := range g.players {
		if p.active && !p.isBot {
			count++
		}
	}
	return count
}

func (g *unoMultiGame) minOpponentCards(excludeIdx int) int {
	min := 999
	for i, p := range g.players {
		if i != excludeIdx && p.active && len(p.hand) < min {
			min = len(p.hand)
		}
	}
	return min
}

func (g *unoMultiGame) playerByUserID(userID id.UserID) *unoMultiPlayer {
	for _, p := range g.players {
		if p.userID == userID {
			return p
		}
	}
	return nil
}

func (g *unoMultiGame) draw(n int) []unoCard {
	var drawn []unoCard
	for i := 0; i < n; i++ {
		if len(g.drawPile) == 0 {
			g.reshuffleDiscard()
		}
		if len(g.drawPile) == 0 {
			break
		}
		drawn = append(drawn, g.drawPile[0])
		g.drawPile = g.drawPile[1:]
	}
	return drawn
}

func (g *unoMultiGame) reshuffleDiscard() {
	var fresh []unoCard
	if g.noMercy {
		fresh = newNoMercyDeck()
	} else {
		fresh = newUnoDeck()
	}
	inPlay := make(map[unoCard]int)
	for _, p := range g.players {
		for _, c := range p.hand {
			inPlay[c]++
		}
	}
	inPlay[g.discardTop]++

	var pile []unoCard
	for _, c := range fresh {
		if inPlay[c] > 0 {
			inPlay[c]--
			continue
		}
		pile = append(pile, c)
	}
	rand.Shuffle(len(pile), func(i, j int) { pile[i], pile[j] = pile[j], pile[i] })
	g.drawPile = pile
}

func (g *unoMultiGame) hasPlayable(hand []unoCard) bool {
	for _, c := range hand {
		if c.canPlayOn(g.discardTop, g.topColor) {
			return true
		}
	}
	return false
}

func (g *unoMultiGame) updateBookState() bool {
	// Bot pays attention when any opponent has <= 3 cards
	shouldBeDown := false
	for _, p := range g.players {
		if !p.isBot && p.active && len(p.hand) <= 3 {
			shouldBeDown = true
			break
		}
	}
	if shouldBeDown != g.bookDown {
		g.bookDown = shouldBeDown
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Lobby commands
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleMultiStart(ctx MessageContext, amountStr string, noMercy, sevenZeroRule bool) error {
	if !isGamesRoom(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Multiplayer Uno can only be started in the games channel!")
	}

	amountStr = strings.TrimPrefix(amountStr, "€")
	var amount float64
	fmt.Sscanf(amountStr, "%f", &amount)

	minBet := envFloat("UNO_MULTI_MIN_BET", envFloat("UNO_MIN_BET", 10))
	maxBet := envFloat("UNO_MULTI_MAX_BET", 500)
	if amount < minBet {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Minimum ante is €%d. Usage: `!uno start €amount`", int(minBet)))
	}
	if amount > maxBet {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Maximum ante is €%d.", int(maxBet)))
	}

	p.mu.Lock()
	if _, exists := p.lobbies[ctx.RoomID]; exists {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "A lobby is already open! Use `!uno join` to join or `!uno cancel` to cancel it.")
	}

	// Check player isn't already in a game
	if _, active := p.games[ctx.Sender]; active {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "You already have a solo Uno game in progress!")
	}
	for _, mg := range p.multiGames {
		if !mg.done {
			for _, pl := range mg.players {
				if pl.userID == ctx.Sender && pl.active && !pl.isBot {
					p.mu.Unlock()
					return p.SendReply(ctx.RoomID, ctx.EventID, "You're already in a multiplayer Uno game!")
				}
			}
		}
	}

	// Debit ante while still holding the lock
	if !p.euro.Debit(ctx.Sender, amount, "uno_multi_ante") {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Insufficient balance for that ante.")
	}

	timeout := envInt("UNO_MULTI_LOBBY_TIMEOUT", 300)
	lobby := &unoMultiLobby{
		roomID:        ctx.RoomID,
		creator:       ctx.Sender,
		ante:          amount,
		players:       []id.UserID{ctx.Sender},
		createdAt:     time.Now(),
		noMercy:       noMercy,
		sevenZeroRule: sevenZeroRule,
	}

	lobby.timer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		p.lobbyExpired(ctx.RoomID)
	})

	p.lobbies[ctx.RoomID] = lobby
	p.mu.Unlock()

	creatorName := p.DisplayName(ctx.Sender)
	modeTag := ""
	if noMercy {
		modeTag = " 🔥 NO MERCY"
		if sevenZeroRule {
			modeTag += " (7-0)"
		}
	}
	return p.SendMessage(ctx.RoomID, fmt.Sprintf(
		"🃏 **UNO Lobby**%s — Ante: €%d\nPlayers (1/4):\n  1. %s (host)\n\nType `!uno join` to join or `!uno go` to start!",
		modeTag, int(amount), creatorName))
}

func (p *UnoPlugin) handleMultiJoin(ctx MessageContext) error {
	if !isGamesRoom(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Use this command in the games channel!")
	}

	p.mu.Lock()
	lobby, exists := p.lobbies[ctx.RoomID]
	if !exists {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No lobby open. Start one with `!uno start €amount`.")
	}

	// Check if already in lobby
	for _, uid := range lobby.players {
		if uid == ctx.Sender {
			p.mu.Unlock()
			return p.SendReply(ctx.RoomID, ctx.EventID, "You're already in the lobby!")
		}
	}

	if len(lobby.players) >= 4 {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Lobby is full! (4/4 players)")
	}

	// Check player isn't in another game
	if _, active := p.games[ctx.Sender]; active {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "You already have a solo Uno game in progress!")
	}
	for _, mg := range p.multiGames {
		if !mg.done {
			for _, pl := range mg.players {
				if pl.userID == ctx.Sender && pl.active && !pl.isBot {
					p.mu.Unlock()
					return p.SendReply(ctx.RoomID, ctx.EventID, "You're already in a multiplayer Uno game!")
				}
			}
		}
	}

	// Debit ante while still holding the lock
	if !p.euro.Debit(ctx.Sender, lobby.ante, "uno_multi_ante") {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Insufficient balance for the ante.")
	}

	lobby.players = append(lobby.players, ctx.Sender)
	count := len(lobby.players)
	p.mu.Unlock()

	// Build player list
	var sb strings.Builder
	lobbyModeTag := ""
	if lobby.noMercy {
		lobbyModeTag = " 🔥 NO MERCY"
		if lobby.sevenZeroRule {
			lobbyModeTag += " (7-0)"
		}
	}
	sb.WriteString(fmt.Sprintf("🃏 **UNO Lobby**%s — Ante: €%d\nPlayers (%d/4):\n", lobbyModeTag, int(lobby.ante), count))
	for i, uid := range lobby.players {
		name := p.DisplayName(uid)
		label := ""
		if uid == lobby.creator {
			label = " (host)"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, name, label))
	}
	sb.WriteString("\nType `!uno join` to join or `!uno go` to start!")

	return p.SendMessage(ctx.RoomID, sb.String())
}

func (p *UnoPlugin) handleMultiLeave(ctx MessageContext) error {
	p.mu.Lock()
	lobby, exists := p.lobbies[ctx.RoomID]
	if !exists {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No lobby open.")
	}

	found := false
	for i, uid := range lobby.players {
		if uid == ctx.Sender {
			lobby.players = append(lobby.players[:i], lobby.players[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not in the lobby.")
	}

	// If creator leaves, cancel the lobby
	if ctx.Sender == lobby.creator || len(lobby.players) == 0 {
		lobby.timer.Stop()
		delete(p.lobbies, ctx.RoomID)
		// Refund remaining players
		for _, uid := range lobby.players {
			p.euro.Credit(uid, lobby.ante, "uno_multi_refund")
		}
		p.mu.Unlock()
		p.euro.Credit(ctx.Sender, lobby.ante, "uno_multi_refund")
		return p.SendMessage(ctx.RoomID, "🃏 Lobby cancelled — all antes refunded.")
	}
	p.mu.Unlock()

	// Refund the leaving player
	p.euro.Credit(ctx.Sender, lobby.ante, "uno_multi_refund")
	name := p.DisplayName(ctx.Sender)
	return p.SendMessage(ctx.RoomID, fmt.Sprintf("🃏 **%s** left the lobby. (%d/4 players)", name, len(lobby.players)))
}

func (p *UnoPlugin) handleMultiCancel(ctx MessageContext) error {
	p.mu.Lock()
	lobby, exists := p.lobbies[ctx.RoomID]
	if !exists {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No lobby open.")
	}

	if ctx.Sender != lobby.creator && !p.IsAdmin(ctx.Sender) {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only the host can cancel the lobby.")
	}

	lobby.timer.Stop()
	players := lobby.players
	ante := lobby.ante
	delete(p.lobbies, ctx.RoomID)
	p.mu.Unlock()

	for _, uid := range players {
		p.euro.Credit(uid, ante, "uno_multi_refund")
	}

	return p.SendMessage(ctx.RoomID, "🃏 Lobby cancelled — all antes refunded.")
}

func (p *UnoPlugin) lobbyExpired(roomID id.RoomID) {
	p.mu.Lock()
	lobby, exists := p.lobbies[roomID]
	if !exists {
		p.mu.Unlock()
		return
	}
	players := lobby.players
	ante := lobby.ante
	delete(p.lobbies, roomID)
	p.mu.Unlock()

	for _, uid := range players {
		p.euro.Credit(uid, ante, "uno_multi_refund")
	}

	p.SendMessage(roomID, "🃏 Lobby expired — all antes refunded.")
}

func (p *UnoPlugin) handleMultiGo(ctx MessageContext) error {
	p.mu.Lock()
	lobby, exists := p.lobbies[ctx.RoomID]
	if !exists {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "No lobby open.")
	}

	if ctx.Sender != lobby.creator {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Only the host can start the game.")
	}

	if len(lobby.players) < 2 {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Need at least 2 players to start.")
	}

	lobby.timer.Stop()
	players := lobby.players
	ante := lobby.ante
	roomID := lobby.roomID
	noMercy := lobby.noMercy
	sevenZeroRule := lobby.sevenZeroRule
	delete(p.lobbies, ctx.RoomID)
	p.mu.Unlock()

	// Resolve DM rooms for all players
	var resolved []playerDMPair
	for _, uid := range players {
		dmRoom, err := p.GetDMRoom(uid)
		if err != nil {
			slog.Error("uno_multi: failed to get DM room", "user", uid, "err", err)
			// Refund all and abort
			for _, u := range players {
				p.euro.Credit(u, ante, "uno_multi_refund")
			}
			return p.SendMessage(roomID, fmt.Sprintf("🃏 Game cancelled — couldn't open DMs with %s.", p.DisplayName(uid)))
		}
		resolved = append(resolved, playerDMPair{uid, dmRoom})
	}

	// Build game
	game := p.initMultiGame(resolved, roomID, ante, noMercy, sevenZeroRule)

	p.mu.Lock()
	p.multiGames[game.id] = game
	for _, pl := range game.players {
		if !pl.isBot {
			p.dmToMulti[pl.dmRoomID] = game.id
		}
	}
	p.mu.Unlock()

	// Announce
	var sb strings.Builder
	bn := unoBotName()
	modeTag := ""
	if noMercy {
		modeTag = " 🔥 NO MERCY"
		if sevenZeroRule {
			modeTag += " (7-0)"
		}
	}
	sb.WriteString(fmt.Sprintf("🃏 **Multiplayer UNO!**%s Pot: €%d\n\nPlayers:\n", modeTag, int(ante)*len(players)))
	for i, pl := range game.players {
		name := p.DisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		marker := ""
		if i == game.currentIdx {
			marker = " ← first turn"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, name, marker))
	}
	startComment := pickCommentary("start")
	if noMercy {
		startComment = pickNoMercyCommentary("nomercy_start")
	}
	sb.WriteString(fmt.Sprintf("\nStarting card: %s\n%s\n\n[Check your DMs!]",
		game.discardTop.DisplayWithColor(game.topColor), startComment))
	p.SendMessage(roomID, sb.String())

	// Start first turn
	game.mu.Lock()
	defer game.mu.Unlock()
	p.startInactivityTimer(game)
	p.executeMultiTurn(game)

	return nil
}

// ---------------------------------------------------------------------------
// Game initialization
// ---------------------------------------------------------------------------

type playerDMPair struct {
	userID   id.UserID
	dmRoomID id.RoomID
}

func (p *UnoPlugin) initMultiGame(players []playerDMPair, roomID id.RoomID, ante float64, noMercy, sevenZeroRule bool) *unoMultiGame {
	var deck []unoCard
	if noMercy {
		deck = newNoMercyDeck()
	} else {
		deck = newUnoDeck()
	}
	cardsPerPlayer := 7
	cardIdx := 0

	// Build players list and deal cards
	var unshuffled []*unoMultiPlayer
	for _, pd := range players {
		hand := make([]unoCard, cardsPerPlayer)
		copy(hand, deck[cardIdx:cardIdx+cardsPerPlayer])
		cardIdx += cardsPerPlayer
		unshuffled = append(unshuffled, &unoMultiPlayer{
			userID:   pd.userID,
			dmRoomID: pd.dmRoomID,
			hand:     hand,
			active:   true,
		})
	}
	// Bot
	botHand := make([]unoCard, cardsPerPlayer)
	copy(botHand, deck[cardIdx:cardIdx+cardsPerPlayer])
	cardIdx += cardsPerPlayer
	bot := &unoMultiPlayer{
		userID: id.UserID(unoBotName()),
		hand:   botHand,
		isBot:  true,
		active: true,
	}
	unshuffled = append(unshuffled, bot)

	// Shuffle turn order
	rand.Shuffle(len(unshuffled), func(i, j int) { unshuffled[i], unshuffled[j] = unshuffled[j], unshuffled[i] })

	// Remaining deck
	remaining := make([]unoCard, len(deck)-cardIdx)
	copy(remaining, deck[cardIdx:])

	// Starting card — must be a number card
	var startCard unoCard
	startIdx := -1
	for i, c := range remaining {
		if !c.Value.isAction() && !c.isWild() {
			startCard = c
			startIdx = i
			break
		}
	}
	if startIdx >= 0 {
		remaining = append(remaining[:startIdx], remaining[startIdx+1:]...)
	} else {
		startCard = remaining[0]
		remaining = remaining[1:]
	}

	gameID := fmt.Sprintf("multi_%d", time.Now().UnixNano())

	return &unoMultiGame{
		id:            gameID,
		roomID:        roomID,
		ante:          ante,
		players:       unshuffled,
		currentIdx:    0,
		direction:     1,
		drawPile:      remaining,
		discardTop:    startCard,
		topColor:      startCard.Color,
		phase:         unoMultiPhasePlay,
		turns:         0,
		turnID:        0,
		startedAt:     time.Now(),
		noMercy:       noMercy,
		sevenZeroRule: sevenZeroRule,
	}
}

// ---------------------------------------------------------------------------
// Turn execution engine
// ---------------------------------------------------------------------------

// executeMultiTurn runs bot turns synchronously and sets up the next human turn.
// Caller must hold game.mu.
func (p *UnoPlugin) executeMultiTurn(game *unoMultiGame) {
	var roomBuf strings.Builder
	botTurnsInRow := 0
	loopMsgsSent := 0 // track messages sent across loop iterations to add jitter

	for {
		if game.done {
			return
		}

		// Jitter between rapid-fire messages to avoid Matrix rate limits.
		// Drop the lock during sleep so DM handlers aren't blocked.
		if loopMsgsSent > 0 {
			savedTurn := game.turnID
			game.mu.Unlock()
			time.Sleep(time.Duration(300+rand.IntN(400)) * time.Millisecond)
			game.mu.Lock()
			// A DM handler may have advanced the game while we slept.
			if game.done || game.turnID != savedTurn {
				return
			}
		}

		player := game.currentPlayer()

		// Skip eliminated players (e.g. mercy-killed during their own turn)
		if !player.active {
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
			continue
		}

		if player.isBot {
			botTurnsInRow++
			if botTurnsInRow > 10 {
				// Safety: shouldn't happen, but prevent infinite loops
				break
			}
			p.botMultiTurn(game, &roomBuf)
			loopMsgsSent++
			if game.done {
				if roomBuf.Len() > 0 {
					p.SendMessage(game.roomID, roomBuf.String())
				}
				return
			}
			if p.checkMultiSuddenDeath(game) {
				if roomBuf.Len() > 0 {
					p.SendMessage(game.roomID, roomBuf.String())
				}
				return
			}
			continue
		}

		// Human's turn — flush bot play buffer first so the turn announcement arrives
		// before any auto-action happens.
		if roomBuf.Len() > 0 {
			p.SendMessage(game.roomID, roomBuf.String())
			roomBuf.Reset()
		}

		// No Mercy stacking: check if player must absorb
		if game.noMercy && game.stackMinValue > 0 {
			if !hasStackableCard(player.hand, game.topColor, game.stackMinValue) {
				name := p.DisplayName(player.userID)
				drawn := game.draw(game.stackTotal)
				player.hand = append(player.hand, drawn...)
				p.SendMessage(player.dmRoomID, fmt.Sprintf("💥 No stackable card! You draw %d cards.\n%s",
					game.stackTotal, pickNoMercyCommentary("stack_absorbed")))
				absorbMsg := fmt.Sprintf("💥 %s absorbs the stack! Draws %d cards. (%d cards now)",
					name, game.stackTotal, len(player.hand))
				game.stackTotal = 0
				game.stackMinValue = 0
				if p.checkMultiMercyElimination(game, player) {
					p.SendMessage(game.roomID, absorbMsg)
					loopMsgsSent++
					if game.done {
						return
					}
					game.currentIdx = game.nextActiveIdx()
					game.turnID++
					continue
				}
				// Skip this player's turn
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				p.SendMessage(game.roomID, absorbMsg+"\n"+p.nextTurnLabel(game))
				loopMsgsSent++
				continue
			}
			// Player has stackable cards — show stacking prompt
			game.phase = unoMultiPhasePlay
			p.sendMultiHandDisplayStacking(game, player)
			p.startMultiAutoPlayTimer(game)
			return
		}

		// Auto-draw if no playable cards
		if !game.hasPlayable(player.hand) {
			if game.noMercy {
				// Draw until playable
				var allDrawn []unoCard
				var playableCard *unoCard
				for {
					cards := game.draw(1)
					if len(cards) == 0 {
						break
					}
					card := cards[0]
					player.hand = append(player.hand, card)
					allDrawn = append(allDrawn, card)
					if p.checkMultiMercyElimination(game, player) {
						if game.done {
							return
						}
						game.currentIdx = game.nextActiveIdx()
						game.turnID++
						break
					}
					if card.canPlayOn(game.discardTop, game.topColor) {
						playableCard = &card
						break
					}
				}
				if !player.active {
					continue // mercy-killed
				}
				name := p.DisplayName(player.userID)
				if len(allDrawn) == 0 {
					p.SendMessage(player.dmRoomID, "No playable cards and deck is empty. Turn passes.")
					game.currentIdx = game.nextActiveIdx()
					game.turnID++
					p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s has no playable cards and deck is empty. Turn passes. (%d cards)\n%s", name, len(player.hand), p.nextTurnLabel(game)))
					loopMsgsSent += 2
					continue
				}
				if playableCard != nil {
					game.drawnCard = playableCard
					game.phase = unoMultiPhaseDrawnPlayable
					p.SendMessage(player.dmRoomID,
						fmt.Sprintf("No playable cards — drew %d card(s): %s\nLast card is playable! Play it? (**yes** / **no**)",
							len(allDrawn), formatDrawnCards(allDrawn)))
					p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws %d card(s). (%d cards)", name, len(allDrawn), len(player.hand)))
					p.startMultiAutoPlayTimer(game)
					return
				}
				p.SendMessage(player.dmRoomID,
					fmt.Sprintf("No playable cards — drew %d card(s). None playable. Turn passes.", len(allDrawn)))
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws %d card(s). Turn passes. (%d cards)\n%s", name, len(allDrawn), len(player.hand), p.nextTurnLabel(game)))
				loopMsgsSent += 2
				continue
			}

			// Classic: draw 1
			drawn := game.draw(1)
			if len(drawn) == 0 {
				name := p.DisplayName(player.userID)
				p.SendMessage(player.dmRoomID, "No playable cards and deck is empty. Turn passes.")
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s has no playable cards and deck is empty. Turn passes. (%d cards)\n%s", name, len(player.hand), p.nextTurnLabel(game)))
				loopMsgsSent += 2
				continue
			}

			card := drawn[0]
			player.hand = append(player.hand, card)

			if card.canPlayOn(game.discardTop, game.topColor) {
				game.drawnCard = &card
				game.phase = unoMultiPhaseDrawnPlayable
				p.SendMessage(player.dmRoomID,
					fmt.Sprintf("No playable cards — drew automatically: %s\nIt's playable! Play it? (**yes** / **no**)", card.Display()))
				p.startMultiAutoPlayTimer(game)
				return
			}

			name := p.DisplayName(player.userID)
			p.SendMessage(player.dmRoomID,
				fmt.Sprintf("No playable cards — drew automatically: %s\nNot playable. Turn passes.", card.Display()))
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
			p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes. (%d cards)\n%s", name, len(player.hand), p.nextTurnLabel(game)))
			loopMsgsSent += 2
			continue
		}

		// Show hand and wait for input
		game.phase = unoMultiPhasePlay
		p.sendMultiHandDisplay(game, player)
		p.startMultiAutoPlayTimer(game)
		return
	}
}

// advanceAndExecute moves to the next player and calls executeMultiTurn.
// Caller must hold game.mu.
func (p *UnoPlugin) advanceAndExecute(game *unoMultiGame) {
	game.currentIdx = game.nextActiveIdx()
	game.turnID++
	game.turns++
	if p.checkMultiSuddenDeath(game) {
		return
	}
	p.executeMultiTurn(game)
}

// nextTurnLabel returns "It's X's turn." for the current player.
func (p *UnoPlugin) nextTurnLabel(game *unoMultiGame) string {
	next := game.currentPlayer()
	name := p.DisplayName(next.userID)
	if next.isBot {
		name = unoBotName()
	}
	return fmt.Sprintf("It's %s's turn.", name)
}

// ---------------------------------------------------------------------------
// DM input handling
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleMultiDMInput(ctx MessageContext, game *unoMultiGame) error {
	game.mu.Lock()
	defer game.mu.Unlock()

	if game.done {
		return nil
	}

	// Status query — available to any player, any time
	input := strings.TrimSpace(strings.ToLower(ctx.Body))
	if strings.HasPrefix(input, "!") {
		input = strings.TrimPrefix(input, "!")
	}
	if input == "uno" || input == "" {
		// Could be status request or UNO call — only treat as status if not active player
		current := game.currentPlayer()
		if current.userID != ctx.Sender {
			caller := game.playerByUserID(ctx.Sender)
			if caller != nil && caller.active {
				p.sendMultiStatus(game, caller)
			}
			return nil
		}
	}

	player := game.currentPlayer()
	if player.userID != ctx.Sender || player.isBot {
		return nil // not this player's turn
	}

	// Reset auto-play counter and inactivity timer on manual input
	player.autoPlays = 0
	p.startInactivityTimer(game)

	// Cancel turn timer
	if game.timer != nil {
		game.timer.Stop()
	}

	if input == "quit" || input == "forfeit" {
		p.multiPlayerForfeit(game, player)
		return nil
	}

	switch game.phase {
	case unoMultiPhaseChallenge:
		return p.handleMultiChallengeInput(game, player, input)

	case unoMultiPhaseChooseColor:
		return p.handleMultiColorChoice(game, player, input)

	case unoMultiPhaseDrawnPlayable:
		return p.handleMultiDrawnPlayable(game, player, input)

	case unoMultiPhaseChooseSwapTarget:
		return p.handleMultiSwapChoice(game, player, input)

	case unoMultiPhasePlay:
		if input == "uno" {
			player.calledUno = true
			p.SendMessage(player.dmRoomID, "✅ UNO called!")
			return nil
		}
		// No Mercy stacking: accept
		if game.noMercy && game.stackMinValue > 0 && (input == "accept" || input == "a") {
			name := p.DisplayName(player.userID)
			drawn := game.draw(game.stackTotal)
			player.hand = append(player.hand, drawn...)
			p.SendMessage(player.dmRoomID, fmt.Sprintf("💥 You accept the stack and draw %d cards.\n%s",
				game.stackTotal, pickNoMercyCommentary("stack_absorbed")))
			absorbMsg := fmt.Sprintf("💥 %s absorbs the stack! Draws %d cards. (%d cards now)",
				name, game.stackTotal, len(player.hand))
			game.stackTotal = 0
			game.stackMinValue = 0
			if p.checkMultiMercyElimination(game, player) {
				p.SendMessage(game.roomID, absorbMsg)
				if game.done {
					return nil
				}
				p.advanceAndExecute(game)
				return nil
			}
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
			game.turns++
			p.SendMessage(game.roomID, absorbMsg+"\n"+p.nextTurnLabel(game))
			if p.checkMultiSuddenDeath(game) {
				return nil
			}
			p.executeMultiTurn(game)
			return nil
		}
		// "accept" alias handled above; "draw" during stacking = same as accept
		if input == "draw" {
			if game.noMercy && game.stackMinValue > 0 {
				p.SendMessage(player.dmRoomID, "You must play a draw card to stack, or type **accept** to draw the stack.")
				return nil
			}
			return p.handleMultiPlayerDraw(game, player)
		}
		var cardIdx int
		if _, err := fmt.Sscanf(input, "%d", &cardIdx); err != nil || cardIdx < 1 || cardIdx > len(player.hand) {
			if game.noMercy && game.stackMinValue > 0 {
				p.SendMessage(player.dmRoomID,
					fmt.Sprintf("Reply with a card number (1-%d) to stack, or **accept** to draw %d cards.", len(player.hand), game.stackTotal))
			} else {
				p.SendMessage(player.dmRoomID,
					fmt.Sprintf("Reply with a number (1-%d) to play, or **draw** to draw.", len(player.hand)))
			}
			return nil
		}
		return p.handleMultiPlayerPlay(game, player, cardIdx-1)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Play handlers
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleMultiChallengeInput(game *unoMultiGame, player *unoMultiPlayer, input string) error {
	switch input {
	case "challenge", "c":
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("⚡ %s challenges the Wild Draw Four!", name))
		p.resolveWD4Challenge(game, true)
	case "accept", "a":
		p.resolveWD4Challenge(game, false)
	default:
		p.SendMessage(player.dmRoomID, "Type **challenge** or **accept**.")
	}
	return nil
}

func (p *UnoPlugin) handleMultiPlayerPlay(game *unoMultiGame, player *unoMultiPlayer, idx int) error {
	card := player.hand[idx]

	// No Mercy stacking validation
	if game.noMercy && game.stackMinValue > 0 {
		if !card.canPlayOnStacking(game.topColor, game.stackMinValue) {
			p.SendMessage(player.dmRoomID,
				fmt.Sprintf("You can't stack %s — need a draw card (matching color or wild). Type **accept** to draw %d.",
					card.Display(), game.stackTotal))
			return nil
		}
	} else if !card.canPlayOn(game.discardTop, game.topColor) {
		p.SendMessage(player.dmRoomID, fmt.Sprintf("You can't play %s on %s.",
			card.Display(), game.discardTop.DisplayWithColor(game.topColor)))
		return nil
	}

	// UNO penalty check — had 2 cards, didn't call UNO (skip during stacking)
	if game.stackMinValue == 0 && len(player.hand) == 2 && !player.calledUno {
		drawn := game.draw(2)
		player.hand = append(player.hand, drawn...)
		player.calledUno = false
		p.SendMessage(player.dmRoomID, "⚠️ You forgot to call UNO! Draw 2 as penalty.")
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s forgot to call UNO! 2 card penalty.", name))
		if game.noMercy {
			if p.checkMultiMercyElimination(game, player) {
				if game.done {
					return nil
				}
				p.advanceAndExecute(game)
				return nil
			}
		}
		p.sendMultiHandDisplay(game, player)
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	// Play the card
	player.hand = append(player.hand[:idx], player.hand[idx+1:]...)
	game.discardTop = card
	game.turns++

	// UNO tracking
	if len(player.hand) == 2 {
		player.calledUno = false
	}

	// No Mercy: Discard All — remove remaining cards of same color
	if game.noMercy && card.Value == unoDiscardAll {
		discarded := discardAllOfColor(&player.hand, card.Color)
		if discarded > 0 {
			p.SendMessage(player.dmRoomID, fmt.Sprintf("Discarded %d additional %s cards!", discarded, card.Color))
		}
	}

	// Wild — need color choice
	if card.isWild() {
		game.pendingCard = &card
		if card.Value == unoWildDrawFour {
			game.wd4PrevColor = game.topColor
		}
		game.phase = unoMultiPhaseChooseColor
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("You played **%s**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green", card.Value))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	game.topColor = card.Color

	// No Mercy: 7-0 rule
	if game.noMercy && game.sevenZeroRule {
		if card.Value == unoSeven && len(game.activePlayers()) > 2 {
			// Choose swap target
			game.phase = unoMultiPhaseChooseSwapTarget
			var sb strings.Builder
			sb.WriteString("You played a **7**! Choose a player to swap hands with:\n")
			i := 1
			for _, pl := range game.players {
				if pl == player || !pl.active {
					continue
				}
				name := p.DisplayName(pl.userID)
				if pl.isBot {
					name = unoBotName()
				}
				sb.WriteString(fmt.Sprintf("%d. %s (%d cards)\n", i, name, len(pl.hand)))
				i++
			}
			p.SendMessage(player.dmRoomID, sb.String())
			p.startMultiAutoPlayTimer(game)
			return nil
		}
		if card.Value == unoSeven {
			// 2-player: swap with the other player
			for _, pl := range game.players {
				if pl != player && pl.active {
					swapHandsMulti(player, pl)
					name := p.DisplayName(player.userID)
					otherName := p.DisplayName(pl.userID)
					if pl.isBot {
						otherName = unoBotName()
					}
					p.SendMessage(game.roomID, fmt.Sprintf("🔄 %s swaps hands with %s! %s",
						name, otherName, pickNoMercyCommentary("hand_swap")))
					if !pl.isBot {
						p.SendMessage(pl.dmRoomID, fmt.Sprintf("🔄 %s swapped hands with you! You now have %d cards.", name, len(pl.hand)))
					}
					break
				}
			}
		}
		if card.Value == unoZero {
			rotateHandsMulti(game)
			p.SendMessage(game.roomID, fmt.Sprintf("🔄 Hands rotated! %s", pickNoMercyCommentary("hand_rotate")))
			// Notify all active human players about their new hand size
			for _, pl := range game.players {
				if pl.active && !pl.isBot {
					p.SendMessage(pl.dmRoomID, fmt.Sprintf("🔄 Hands rotated! You now have %d cards.", len(pl.hand)))
				}
			}
		}
	}

	// Check win (after discard all, after swap)
	if len(player.hand) == 0 {
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s plays: %s", name, card.Display()))
		p.multiPlayerWins(game, player)
		return nil
	}

	// Apply effects and announce
	p.applyAndAnnounce(game, player, card)
	return nil
}

func (p *UnoPlugin) handleMultiColorChoice(game *unoMultiGame, player *unoMultiPlayer, input string) error {
	var color unoColor
	switch input {
	case "1", "red":
		color = unoRed
	case "2", "blue":
		color = unoBlue
	case "3", "yellow":
		color = unoYellow
	case "4", "green":
		color = unoGreen
	default:
		p.SendMessage(player.dmRoomID, "Choose a color: **1** (Red), **2** (Blue), **3** (Yellow), **4** (Green)")
		return nil
	}

	game.topColor = color
	pendingCard := game.pendingCard
	game.pendingCard = nil
	game.phase = unoMultiPhasePlay

	p.SendMessage(player.dmRoomID, fmt.Sprintf("Color set to %s %s.", color.Emoji(), color))

	// Check win
	if len(player.hand) == 0 {
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s plays: %s (chose %s %s)",
			name, pendingCard.Display(), color.Emoji(), color))
		p.multiPlayerWins(game, player)
		return nil
	}

	// No Mercy: Color Roulette — next player flips until chosen color
	if game.noMercy && pendingCard != nil && pendingCard.Value == unoWildColorRoulette {
		name := p.DisplayName(player.userID)
		nextIdx := game.nextActiveIdx()
		target := game.players[nextIdx]
		targetName := p.DisplayName(target.userID)
		if target.isBot {
			targetName = unoBotName()
		}

		flipped := p.executeColorRouletteMulti(game, target, color)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s plays: %s (chose %s %s)\n🎰 Color Roulette! %s flips %d cards until finding %s.\n%s",
			name, pendingCard.Display(), color.Emoji(), color,
			targetName, len(flipped), color, pickNoMercyCommentary("color_roulette")))
		if !target.isBot {
			p.SendMessage(target.dmRoomID, fmt.Sprintf("🎰 Color Roulette! You drew %d cards. You now have %d cards.", len(flipped), len(target.hand)))
		}

		if p.checkMultiMercyElimination(game, target) {
			if game.done {
				return nil
			}
		}

		// Skip the target player, advance to next
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
		if p.checkMultiSuddenDeath(game) {
			return nil
		}
		p.executeMultiTurn(game)
		return nil
	}

	// Apply effects
	p.applyAndAnnounce(game, player, *pendingCard)
	return nil
}

func (p *UnoPlugin) handleMultiPlayerDraw(game *unoMultiGame, player *unoMultiPlayer) error {
	// No Mercy: draw until playable
	if game.noMercy {
		return p.handleMultiPlayerDrawNoMercy(game, player)
	}

	drawn := game.draw(1)
	if len(drawn) == 0 {
		p.SendMessage(player.dmRoomID, "No cards left to draw! Turn passes.")
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s can't draw — deck is empty. Turn passes.", name))
		p.advanceAndExecute(game)
		return nil
	}

	card := drawn[0]
	player.hand = append(player.hand, card)

	if card.canPlayOn(game.discardTop, game.topColor) {
		game.drawnCard = &card
		game.phase = unoMultiPhaseDrawnPlayable
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("You drew: %s\nIt's playable! Play it? (**yes** / **no**)", card.Display()))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	name := p.DisplayName(player.userID)
	p.SendMessage(player.dmRoomID, fmt.Sprintf("You drew: %s\nNot playable. Turn passes.", card.Display()))
	p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes. (%d cards)", name, len(player.hand)))
	p.advanceAndExecute(game)
	return nil
}

func (p *UnoPlugin) handleMultiPlayerDrawNoMercy(game *unoMultiGame, player *unoMultiPlayer) error {
	var allDrawn []unoCard
	var playableCard *unoCard

	for {
		drawn := game.draw(1)
		if len(drawn) == 0 {
			break
		}
		card := drawn[0]
		player.hand = append(player.hand, card)
		allDrawn = append(allDrawn, card)
		if p.checkMultiMercyElimination(game, player) {
			if game.done {
				return nil
			}
			p.advanceAndExecute(game)
			return nil
		}
		if card.canPlayOn(game.discardTop, game.topColor) {
			playableCard = &card
			break
		}
	}

	name := p.DisplayName(player.userID)

	if len(allDrawn) == 0 {
		p.SendMessage(player.dmRoomID, "No cards left to draw! Turn passes.")
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s can't draw — deck is empty. Turn passes.", name))
		p.advanceAndExecute(game)
		return nil
	}

	if playableCard != nil {
		game.drawnCard = playableCard
		game.phase = unoMultiPhaseDrawnPlayable
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("Drew %d card(s): %s\nLast card is playable! Play it? (**yes** / **no**)",
				len(allDrawn), formatDrawnCards(allDrawn)))
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws %d card(s). (%d cards)", name, len(allDrawn), len(player.hand)))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	p.SendMessage(player.dmRoomID,
		fmt.Sprintf("Drew %d card(s). None playable. Turn passes.", len(allDrawn)))
	p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws %d card(s). Turn passes. (%d cards)", name, len(allDrawn), len(player.hand)))
	p.advanceAndExecute(game)
	return nil
}

func (p *UnoPlugin) handleMultiDrawnPlayable(game *unoMultiGame, player *unoMultiPlayer, input string) error {
	if input != "yes" && input != "y" && input != "no" && input != "n" {
		p.SendMessage(player.dmRoomID, "Play the drawn card? (**yes** / **no**)")
		return nil
	}

	drawnCard := *game.drawnCard
	game.drawnCard = nil
	game.phase = unoMultiPhasePlay

	if input == "no" || input == "n" {
		name := p.DisplayName(player.userID)
		p.SendMessage(player.dmRoomID, "Card kept. Turn passes.")
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes. (%d cards)", name, len(player.hand)))
		p.advanceAndExecute(game)
		return nil
	}

	// Play the drawn card
	for i, c := range player.hand {
		if c == drawnCard {
			player.hand = append(player.hand[:i], player.hand[i+1:]...)
			break
		}
	}
	game.discardTop = drawnCard
	game.turns++

	if len(player.hand) == 2 {
		player.calledUno = false
	}

	if drawnCard.isWild() {
		game.pendingCard = &drawnCard
		if drawnCard.Value == unoWildDrawFour {
			game.wd4PrevColor = game.topColor
		}
		game.phase = unoMultiPhaseChooseColor
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("You played **%s**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green", drawnCard.Value))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	game.topColor = drawnCard.Color

	if len(player.hand) == 0 {
		name := p.DisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s plays: %s", name, drawnCard.Display()))
		p.multiPlayerWins(game, player)
		return nil
	}

	p.applyAndAnnounce(game, player, drawnCard)
	return nil
}

// ---------------------------------------------------------------------------
// Card effects & turn announcement
// ---------------------------------------------------------------------------

// cardEffectResult describes what happened when a card's effects were applied.
type cardEffectResult struct {
	skippedName    string // non-empty if a player was skipped
	reversed       bool   // true if direction was reversed
	drawnCount     int    // cards drawn by the victim (2 or 4)
	needsChallenge bool   // true if WD4 challenge phase should start
	stackPending   bool   // true if draw stacking is in progress (No Mercy)
}

// applyCardEffects applies skip/reverse/draw effects and advances the turn.
// Caller must hold game.mu.
func (p *UnoPlugin) applyCardEffects(game *unoMultiGame, card unoCard) cardEffectResult {
	var result cardEffectResult

	nextIdx := game.nextActiveIdx()
	nextPlayer := game.players[nextIdx]
	nextName := p.DisplayName(nextPlayer.userID)
	if nextPlayer.isBot {
		nextName = unoBotName()
	}

	switch card.Value {
	case unoSkip:
		result.skippedName = nextName
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoSkipEveryone:
		if game.noMercy {
			// Skip all others — current player goes again
			var skippedNames []string
			for _, pl := range game.players {
				if pl != game.currentPlayer() && pl.active {
					name := p.DisplayName(pl.userID)
					if pl.isBot {
						name = unoBotName()
					}
					skippedNames = append(skippedNames, name)
				}
			}
			result.skippedName = strings.Join(skippedNames, ", ")
			// Don't advance — current player keeps their turn
			game.turnID++
		} else {
			// Treat as skip in classic
			result.skippedName = nextName
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoReverse:
		game.direction *= -1
		result.reversed = true
		if len(game.activePlayers()) == 2 {
			result.skippedName = nextName
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoDrawTwo, unoDrawFour:
		if game.noMercy {
			// No Mercy: stacking
			dv := cardDrawValue(card.Value)
			game.stackTotal += dv
			game.stackMinValue = dv
			result.stackPending = true
			result.skippedName = nextName
			result.drawnCount = game.stackTotal
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else if card.Value == unoDrawTwo {
			// Classic Draw Two
			drawn := game.draw(2)
			nextPlayer.hand = append(nextPlayer.hand, drawn...)
			result.skippedName = nextName
			result.drawnCount = 2
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			// DrawFour shouldn't appear in classic, but handle gracefully
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoWildDrawFour:
		if game.noMercy {
			// Should not appear in No Mercy deck, but handle gracefully
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			result.needsChallenge = true
			result.skippedName = nextName
		}

	case unoWildReverseDraw4:
		if game.noMercy {
			game.direction *= -1
			result.reversed = true
			dv := cardDrawValue(card.Value)
			game.stackTotal += dv
			game.stackMinValue = dv
			result.stackPending = true
			// After reverse, get next player in new direction
			nextIdx = game.nextActiveIdx()
			nextPlayer = game.players[nextIdx]
			nextName = p.DisplayName(nextPlayer.userID)
			if nextPlayer.isBot {
				nextName = unoBotName()
			}
			result.skippedName = nextName
			result.drawnCount = game.stackTotal
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoWildDrawSix, unoWildDrawTen:
		if game.noMercy {
			dv := cardDrawValue(card.Value)
			game.stackTotal += dv
			game.stackMinValue = dv
			result.stackPending = true
			result.skippedName = nextName
			result.drawnCount = game.stackTotal
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoWildColorRoulette:
		// Color roulette effect is handled in color choice handler, not here
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoDiscardAll:
		// Discard effect already applied before calling this function
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	default:
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	}

	return result
}

// writeEffectLines appends human-readable effect descriptions to a string builder.
func writeEffectLines(sb *strings.Builder, eff cardEffectResult) {
	if eff.needsChallenge {
		sb.WriteString(fmt.Sprintf("\n  %s may challenge! ⚡", eff.skippedName))
	} else if eff.stackPending {
		sb.WriteString(fmt.Sprintf("\n  🔥 Stack incoming! (+%d) — %s must stack or absorb!", eff.drawnCount, eff.skippedName))
		if eff.reversed {
			sb.WriteString("\n  Direction reversed!")
		}
	} else if eff.drawnCount > 0 {
		sb.WriteString(fmt.Sprintf("\n  %s draws %d and is skipped!", eff.skippedName, eff.drawnCount))
	} else if eff.skippedName != "" && eff.reversed {
		sb.WriteString(fmt.Sprintf("\n  %s is skipped! (reverse)", eff.skippedName))
	} else if eff.skippedName != "" {
		sb.WriteString(fmt.Sprintf("\n  %s is skipped!", eff.skippedName))
	} else if eff.reversed {
		sb.WriteString("\n  Direction reversed!")
	}
}

func (p *UnoPlugin) applyAndAnnounce(game *unoMultiGame, player *unoMultiPlayer, card unoCard) {
	name := p.DisplayName(player.userID)
	var roomMsg strings.Builder

	roomMsg.WriteString(fmt.Sprintf("🃏 %s plays: %s", name, card.DisplayWithColor(game.topColor)))

	eff := p.applyCardEffects(game, card)
	writeEffectLines(&roomMsg, eff)

	// UNO announcement
	if len(player.hand) == 1 && player.calledUno {
		roomMsg.WriteString(fmt.Sprintf("\n  %s calls UNO! 🔥", name))
	}

	// Wild Draw Four — enter challenge phase
	if eff.needsChallenge {
		p.SendMessage(game.roomID, roomMsg.String())
		p.startWD4Challenge(game, player)
		return
	}

	// Book state commentary (occasionally)
	if game.turns%4 == 0 {
		if changed := game.updateBookState(); changed {
			if game.bookDown {
				roomMsg.WriteString("\n\n" + pickCommentary("book_down"))
			} else {
				roomMsg.WriteString("\n\n" + pickCommentary("book_up"))
			}
		}
	}

	// Next player
	nextUp := game.currentPlayer()
	nextUpName := p.DisplayName(nextUp.userID)
	if nextUp.isBot {
		nextUpName = unoBotName()
	}
	roomMsg.WriteString(fmt.Sprintf("\n  It's %s's turn.", nextUpName))

	p.SendMessage(game.roomID, roomMsg.String())
	if p.checkMultiSuddenDeath(game) {
		return
	}
	p.executeMultiTurn(game)
}

// ---------------------------------------------------------------------------
// Wild Draw Four challenge
// ---------------------------------------------------------------------------

// startWD4Challenge enters the challenge phase. The victim can challenge or accept.
// Caller must hold game.mu.
func (p *UnoPlugin) startWD4Challenge(game *unoMultiGame, wd4Player *unoMultiPlayer) {
	nextIdx := game.nextActiveIdx()
	victim := game.players[nextIdx]

	game.wd4Player = wd4Player
	game.wd4Victim = victim
	game.phase = unoMultiPhaseChallenge
	game.currentIdx = nextIdx
	game.turnID++

	playerName := p.DisplayName(wd4Player.userID)
	if wd4Player.isBot {
		playerName = unoBotName()
	}

	if victim.isBot {
		// Bot decides whether to challenge
		p.botHandleWD4Challenge(game)
		return
	}

	p.SendMessage(victim.dmRoomID,
		fmt.Sprintf("⚡ **%s** played Wild Draw Four!\nYou can **challenge** — if they had a %s %s card, they draw 4 instead.\nIf the challenge fails, you draw 6.\n\nType **challenge** or **accept**.",
			playerName, game.wd4PrevColor.Emoji(), game.wd4PrevColor))
	p.startMultiAutoPlayTimer(game)
}

// resolveWD4Challenge resolves the challenge. Caller must hold game.mu.
func (p *UnoPlugin) resolveWD4Challenge(game *unoMultiGame, challenged bool) {
	wd4Player := game.wd4Player
	victim := game.wd4Victim
	wd4Name := p.DisplayName(wd4Player.userID)
	if wd4Player.isBot {
		wd4Name = unoBotName()
	}
	victimName := p.DisplayName(victim.userID)
	if victim.isBot {
		victimName = unoBotName()
	}

	// Clear challenge state
	game.wd4Player = nil
	game.wd4Victim = nil
	game.phase = unoMultiPhasePlay

	if !challenged {
		// Victim accepts — draw 4, get skipped
		drawn := game.draw(4)
		victim.hand = append(victim.hand, drawn...)
		p.SendMessage(game.roomID,
			fmt.Sprintf("🃏 %s accepts the Wild Draw Four. Draws 4 and is skipped!", victimName))
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
		if p.checkMultiSuddenDeath(game) {
			return
		}
		p.executeMultiTurn(game)
		return
	}

	// Check if the WD4 player had a card matching the previous color
	hadMatch := false
	for _, c := range wd4Player.hand {
		if c.Color == game.wd4PrevColor {
			hadMatch = true
			break
		}
	}

	if hadMatch {
		// Challenge succeeds — WD4 player drew illegally, they draw 4
		drawn := game.draw(4)
		wd4Player.hand = append(wd4Player.hand, drawn...)
		p.SendMessage(game.roomID,
			fmt.Sprintf("⚡ **Challenge successful!** %s had a %s %s card. %s draws 4!",
				wd4Name, game.wd4PrevColor.Emoji(), game.wd4PrevColor, wd4Name))
		// Victim is NOT skipped — turn continues from victim
		game.turnID++
		if p.checkMultiSuddenDeath(game) {
			return
		}
		p.executeMultiTurn(game)
	} else {
		// Challenge fails — victim draws 6 (4 + 2 penalty)
		drawn := game.draw(6)
		victim.hand = append(victim.hand, drawn...)
		p.SendMessage(game.roomID,
			fmt.Sprintf("⚡ **Challenge failed!** %s played legally. %s draws 6!",
				wd4Name, victimName))
		// Victim is skipped
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
		if p.checkMultiSuddenDeath(game) {
			return
		}
		p.executeMultiTurn(game)
	}
}

// botHandleWD4Challenge decides whether the bot challenges a WD4. Caller must hold game.mu.
func (p *UnoPlugin) botHandleWD4Challenge(game *unoMultiGame) {
	// More cards = more likely they had a matching color = more likely to challenge.
	// 1-3 cards: 10%, 4-5: 30%, 6-7: 50%, 8+: 70%
	cards := len(game.wd4Player.hand)
	threshold := 10
	if cards >= 8 {
		threshold = 70
	} else if cards >= 6 {
		threshold = 50
	} else if cards >= 4 {
		threshold = 30
	}
	challenged := rand.IntN(100) < threshold
	bn := unoBotName()

	if challenged {
		p.SendMessage(game.roomID, fmt.Sprintf("⚡ %s challenges the Wild Draw Four!", bn))
	} else {
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s accepts the Wild Draw Four.", bn))
	}

	p.resolveWD4Challenge(game, challenged)
}

// ---------------------------------------------------------------------------
// Bot turn (multiplayer)
// ---------------------------------------------------------------------------

func (p *UnoPlugin) botMultiTurn(game *unoMultiGame, roomBuf *strings.Builder) {
	bot := game.currentPlayer()
	bn := unoBotName()

	// No Mercy stacking: bot must stack or absorb
	if game.noMercy && game.stackMinValue > 0 {
		stackCard, sIdx := botPickStackCard(bot.hand, game.topColor, game.stackMinValue)
		if sIdx < 0 {
			// Bot absorbs the stack
			drawn := game.draw(game.stackTotal)
			bot.hand = append(bot.hand, drawn...)
			if roomBuf.Len() > 0 {
				roomBuf.WriteString("\n")
			}
			roomBuf.WriteString(fmt.Sprintf("💥 %s absorbs the stack! Draws %d cards. (%d cards now)",
				bn, game.stackTotal, len(bot.hand)))
			game.stackTotal = 0
			game.stackMinValue = 0
			if p.checkMultiMercyElimination(game, bot) {
				if roomBuf.Len() > 0 {
					p.SendMessage(game.roomID, roomBuf.String())
					roomBuf.Reset()
				}
				return
			}
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
			return
		}
		// Bot stacks
		bot.hand = append(bot.hand[:sIdx], bot.hand[sIdx+1:]...)
		game.discardTop = stackCard
		dv := cardDrawValue(stackCard.Value)
		game.stackTotal += dv
		game.stackMinValue = dv
		if stackCard.isWild() {
			game.topColor = botPickColor(bot.hand)
		} else {
			game.topColor = stackCard.Color
		}
		if roomBuf.Len() > 0 {
			roomBuf.WriteString("\n")
		}
		roomBuf.WriteString(fmt.Sprintf("🔥 %s stacks: %s! Total penalty: +%d",
			bn, stackCard.DisplayWithColor(game.topColor), game.stackTotal))
		if stackCard.Value == unoWildReverseDraw4 {
			game.direction *= -1
			roomBuf.WriteString(" (direction reversed!)")
		}
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
		game.turns++
		return
	}

	var card unoCard
	var idx int
	if game.noMercy {
		card, idx = botPickCardNoMercy(bot.hand, game.discardTop, game.topColor, game.bookDown, game.minOpponentCards(game.currentIdx), 0)
	} else {
		card, idx = botPickCard(bot.hand, game.discardTop, game.topColor, game.bookDown, game.minOpponentCards(game.currentIdx))
	}

	if idx < 0 {
		// Bot draws
		if game.noMercy {
			// Draw until playable
			var allDrawn []unoCard
			var playableCard *unoCard
			for {
				cards := game.draw(1)
				if len(cards) == 0 {
					break
				}
				c := cards[0]
				bot.hand = append(bot.hand, c)
				allDrawn = append(allDrawn, c)
				if p.checkMultiMercyElimination(game, bot) {
					if roomBuf.Len() > 0 {
						p.SendMessage(game.roomID, roomBuf.String())
						roomBuf.Reset()
					}
					return
				}
				if c.canPlayOn(game.discardTop, game.topColor) {
					playableCard = &c
					break
				}
			}
			if len(allDrawn) == 0 {
				if roomBuf.Len() > 0 {
					roomBuf.WriteString("\n")
				}
				roomBuf.WriteString(fmt.Sprintf("🃏 %s can't draw — deck empty. Turn passes.", bn))
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				return
			}
			if playableCard != nil {
				// Remove from hand and play
				for i := len(bot.hand) - 1; i >= 0; i-- {
					if bot.hand[i] == *playableCard {
						bot.hand = append(bot.hand[:i], bot.hand[i+1:]...)
						break
					}
				}
				if roomBuf.Len() > 0 {
					roomBuf.WriteString("\n")
				}
				roomBuf.WriteString(fmt.Sprintf("🃏 %s draws %d card(s).", bn, len(allDrawn)))
				card = *playableCard
				// Fall through to play the card below
			} else {
				if roomBuf.Len() > 0 {
					roomBuf.WriteString("\n")
				}
				roomBuf.WriteString(fmt.Sprintf("🃏 %s draws %d card(s). No playable card.", bn, len(allDrawn)))
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				return
			}
		} else {
			drawn := game.draw(1)
			if len(drawn) == 0 {
				if roomBuf.Len() > 0 {
					roomBuf.WriteString("\n")
				}
				roomBuf.WriteString(fmt.Sprintf("🃏 %s can't draw — deck empty. Turn passes.", bn))
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				return
			}

			bot.hand = append(bot.hand, drawn[0])

			if drawn[0].canPlayOn(game.discardTop, game.topColor) {
				bot.hand = bot.hand[:len(bot.hand)-1]
				card = drawn[0]
			} else {
				if game.turns%3 == 0 {
					commentKey := "bot_draw_normal"
					if game.bookDown {
						commentKey = "bot_draw_bookdown"
					}
					if roomBuf.Len() > 0 {
						roomBuf.WriteString("\n")
					}
					roomBuf.WriteString(pickCommentary(commentKey))
				} else {
					if roomBuf.Len() > 0 {
						roomBuf.WriteString("\n")
					}
					roomBuf.WriteString(fmt.Sprintf("🃏 %s draws a card.", bn))
				}
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
				return
			}
		}
	}

	// Play the card
	if idx >= 0 {
		bot.hand = append(bot.hand[:idx], bot.hand[idx+1:]...)
	}
	game.discardTop = card

	// No Mercy: Discard All — bot discards remaining cards of same color
	if game.noMercy && card.Value == unoDiscardAll {
		discardAllOfColor(&bot.hand, card.Color)
	}

	// Wild color choice
	if card.isWild() {
		if card.Value == unoWildDrawFour {
			game.wd4PrevColor = game.topColor
		}
		if game.noMercy && card.Value == unoWildColorRoulette {
			game.topColor = botRouletteColor(bot.hand)
		} else {
			game.topColor = botPickColor(bot.hand)
		}
	} else {
		game.topColor = card.Color
	}

	// No Mercy: 7-0 rule
	if game.noMercy && game.sevenZeroRule {
		if card.Value == unoSeven {
			target := botChooseSwapTarget(game, bot)
			if target != nil {
				swapHandsMulti(bot, target)
				if !target.isBot {
					p.SendMessage(target.dmRoomID, fmt.Sprintf("🔄 %s swapped hands with you! You now have %d cards.", bn, len(target.hand)))
				}
			}
		}
		if card.Value == unoZero {
			rotateHandsMulti(game)
			for _, pl := range game.players {
				if pl.active && !pl.isBot {
					p.SendMessage(pl.dmRoomID, fmt.Sprintf("🔄 Hands rotated! You now have %d cards.", len(pl.hand)))
				}
			}
		}
	}

	// Check bot win
	if len(bot.hand) == 0 {
		if roomBuf.Len() > 0 {
			roomBuf.WriteString("\n")
		}
		roomBuf.WriteString(fmt.Sprintf("🃏 %s plays: %s", bn, card.DisplayWithColor(game.topColor)))
		if roomBuf.Len() > 0 {
			p.SendMessage(game.roomID, roomBuf.String())
			roomBuf.Reset()
		}
		p.multiBotWins(game)
		return
	}

	// Bot UNO call
	if len(bot.hand) == 1 {
		if roomBuf.Len() > 0 {
			roomBuf.WriteString("\n")
		}
		roomBuf.WriteString(fmt.Sprintf("🃏 %s plays: %s\n  %s calls UNO! 🔥", bn, card.DisplayWithColor(game.topColor), bn))
	} else {
		if roomBuf.Len() > 0 {
			roomBuf.WriteString("\n")
		}
		// Add commentary occasionally
		if game.turns%3 == 0 {
			commentKey := "bot_play_normal"
			if game.bookDown {
				commentKey = "bot_play_bookdown"
			}
			roomBuf.WriteString(fmt.Sprintf("🃏 %s", fmt.Sprintf(pickCommentary(commentKey), card.DisplayWithColor(game.topColor))))
		} else {
			roomBuf.WriteString(fmt.Sprintf("🃏 %s plays: %s", bn, card.DisplayWithColor(game.topColor)))
		}
	}

	// Apply action effects
	eff := p.applyCardEffects(game, card)
	writeEffectLines(roomBuf, eff)

	// WD4 challenge — flush buffer and enter challenge phase
	if eff.needsChallenge {
		if roomBuf.Len() > 0 {
			p.SendMessage(game.roomID, roomBuf.String())
			roomBuf.Reset()
		}
		game.turns++
		p.startWD4Challenge(game, bot)
		return
	}

	// No Mercy: Color Roulette — next player flips cards
	// applyCardEffects already advanced currentIdx to the victim
	if game.noMercy && card.Value == unoWildColorRoulette {
		target := game.players[game.currentIdx]
		targetName := p.DisplayName(target.userID)
		if target.isBot {
			targetName = bn
		}
		flipped := p.executeColorRouletteMulti(game, target, game.topColor)
		roomBuf.WriteString(fmt.Sprintf("\n  🎰 Color Roulette! %s flips %d cards until finding %s %s.",
			targetName, len(flipped), game.topColor.Emoji(), game.topColor))
		if !target.isBot {
			p.SendMessage(target.dmRoomID, fmt.Sprintf("🎰 Color Roulette! You drew %d cards. You now have %d cards.", len(flipped), len(target.hand)))
		}

		if p.checkMultiMercyElimination(game, target) {
			if roomBuf.Len() > 0 {
				p.SendMessage(game.roomID, roomBuf.String())
				roomBuf.Reset()
			}
			if game.done {
				return
			}
		}

		// Skip past the roulette victim
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	}

	// No Mercy: 7-0 room announcements (swap already happened above)
	if game.noMercy && game.sevenZeroRule {
		if card.Value == unoSeven {
			roomBuf.WriteString("\n  🔄 Hands swapped!")
		}
		if card.Value == unoZero {
			roomBuf.WriteString("\n  🔄 Hands rotated!")
		}
	}

	// Next turn label (applyCardEffects already advanced currentIdx)
	nextUp := game.currentPlayer()
	nextUpName := p.DisplayName(nextUp.userID)
	if nextUp.isBot {
		nextUpName = unoBotName()
	}
	roomBuf.WriteString(fmt.Sprintf("\n  It's %s's turn.", nextUpName))

	game.turns++
}

// ---------------------------------------------------------------------------
// Auto-play timer
// ---------------------------------------------------------------------------

func (p *UnoPlugin) startMultiAutoPlayTimer(game *unoMultiGame) {
	if game.timer != nil {
		game.timer.Stop()
	}

	timeout := envInt("UNO_MULTI_TURN_TIMEOUT", 30)
	savedTurnID := game.turnID
	gameID := game.id

	game.timer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		p.mu.Lock()
		mg, exists := p.multiGames[gameID]
		p.mu.Unlock()
		if !exists {
			return
		}

		mg.mu.Lock()
		defer mg.mu.Unlock()

		if mg.done || mg.turnID != savedTurnID {
			return // turn already changed
		}

		player := mg.currentPlayer()
		if player.isBot {
			return
		}

		player.autoPlays++
		maxAutoPlays := envInt("UNO_MULTI_MAX_AUTOPLAY", 3)

		name := p.DisplayName(player.userID)

		if player.autoPlays >= maxAutoPlays {
			p.SendMessage(mg.roomID, fmt.Sprintf("🃏 %s was auto-played %d times in a row — forfeited!", name, maxAutoPlays))
			p.multiPlayerForfeit(mg, player)
			return
		}

		// Auto-play: find first playable non-action card
		p.autoPlayMultiTurn(mg, player)
	})
}

// startInactivityTimer starts (or resets) the 10-minute game timeout.
// If no human input occurs within the window, the game ends and remaining players are refunded.
// Caller must hold game.mu.
func (p *UnoPlugin) startInactivityTimer(game *unoMultiGame) {
	if game.inactiveTimer != nil {
		game.inactiveTimer.Stop()
	}

	timeout := envInt("UNO_MULTI_INACTIVITY_TIMEOUT", 600)
	gameID := game.id

	game.inactiveTimer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		p.mu.Lock()
		mg, exists := p.multiGames[gameID]
		p.mu.Unlock()
		if !exists {
			return
		}

		mg.mu.Lock()
		defer mg.mu.Unlock()

		if mg.done {
			return
		}

		mg.done = true
		if mg.timer != nil {
			mg.timer.Stop()
		}

		// Refund remaining human players
		for _, pl := range mg.players {
			if !pl.isBot && pl.active {
				p.euro.Credit(pl.userID, mg.ante, "uno_multi_timeout_refund")
			}
		}

		p.SendMessage(mg.roomID, "🃏 **Game timed out** — no human input for 10 minutes. All antes refunded.")
		p.recordMultiGame(mg, id.UserID(""), "timeout")
		p.cleanupMultiGame(mg)
	})
}

func (p *UnoPlugin) autoPlayMultiTurn(game *unoMultiGame, player *unoMultiPlayer) {
	name := p.DisplayName(player.userID)

	switch game.phase {
	case unoMultiPhaseChallenge:
		// Auto-play: accept the WD4 (safe default)
		p.SendMessage(player.dmRoomID, "*Auto-played:* Accepted Wild Draw Four.")
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Accepts the Wild Draw Four.", name))
		p.resolveWD4Challenge(game, false)
		return

	case unoMultiPhaseChooseColor:
		// Auto-pick most common color
		color := botPickColor(player.hand)
		game.topColor = color
		pendingCard := game.pendingCard
		game.pendingCard = nil
		game.phase = unoMultiPhasePlay

		p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Color set to %s %s.", color.Emoji(), color))

		if len(player.hand) == 0 {
			p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
				name, pendingCard.Display(), color.Emoji(), color))
			p.multiPlayerWins(game, player)
			return
		}

		p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
			name, pendingCard.Display(), color.Emoji(), color))

		// No Mercy: Color Roulette — apply roulette effect before generic effects
		if game.noMercy && pendingCard != nil && pendingCard.Value == unoWildColorRoulette {
			nextIdx := game.nextActiveIdx()
			target := game.players[nextIdx]
			targetName := p.DisplayName(target.userID)
			if target.isBot {
				targetName = unoBotName()
			}
			flipped := p.executeColorRouletteMulti(game, target, color)
			p.SendMessage(game.roomID, fmt.Sprintf("🎰 Color Roulette! %s flips %d cards until finding %s.\n%s",
				targetName, len(flipped), color, pickNoMercyCommentary("color_roulette")))
			if !target.isBot {
				p.SendMessage(target.dmRoomID, fmt.Sprintf("🎰 Color Roulette! You drew %d cards. You now have %d cards.", len(flipped), len(target.hand)))
			}
			if p.checkMultiMercyElimination(game, target) {
				if game.done {
					return
				}
			}
			// Skip past the roulette victim
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
			if p.checkMultiSuddenDeath(game) {
				return
			}
			p.executeMultiTurn(game)
			return
		}

		p.applyAutoEffects(game, player, *pendingCard)
		return

	case unoMultiPhaseDrawnPlayable:
		// Auto-play: play the drawn card
		drawnCard := *game.drawnCard
		game.drawnCard = nil
		game.phase = unoMultiPhasePlay

		for i, c := range player.hand {
			if c == drawnCard {
				player.hand = append(player.hand[:i], player.hand[i+1:]...)
				break
			}
		}
		game.discardTop = drawnCard
		game.turns++

		if drawnCard.isWild() {
			if drawnCard.Value == unoWildDrawFour {
				game.wd4PrevColor = game.topColor
			}
			color := botPickColor(player.hand)
			game.topColor = color
			p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* %s (chose %s %s).", drawnCard.Display(), color.Emoji(), color))

			if len(player.hand) == 0 {
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
					name, drawnCard.Display(), color.Emoji(), color))
				p.multiPlayerWins(game, player)
				return
			}

			p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
				name, drawnCard.Display(), color.Emoji(), color))
			p.applyAutoEffects(game, player, drawnCard)
			return
		}

		game.topColor = drawnCard.Color
		p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* %s.", drawnCard.Display()))

		if len(player.hand) == 0 {
			p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s", name, drawnCard.Display()))
			p.multiPlayerWins(game, player)
			return
		}

		p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s", name, drawnCard.Display()))
		p.applyAutoEffects(game, player, drawnCard)
		return

	case unoMultiPhaseChooseSwapTarget:
		// Auto-play: swap with player who has fewest cards
		target := botChooseSwapTarget(game, player)
		if target != nil {
			swapHandsMulti(player, target)
			targetName := p.DisplayName(target.userID)
			if target.isBot {
				targetName = unoBotName()
			}
			p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Swapped hands with %s.", targetName))
			p.SendMessage(game.roomID, fmt.Sprintf("🔄 *%s was auto-played.* Swaps hands with %s!", name, targetName))
		}
		game.phase = unoMultiPhasePlay
		if len(player.hand) == 0 {
			p.multiPlayerWins(game, player)
			return
		}
		p.advanceAndExecute(game)
		return

	case unoMultiPhasePlay:
		// No Mercy stacking: auto-accept
		if game.noMercy && game.stackMinValue > 0 {
			drawn := game.draw(game.stackTotal)
			player.hand = append(player.hand, drawn...)
			p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Accepted stack. Drew %d cards.", game.stackTotal))
			p.SendMessage(game.roomID, fmt.Sprintf("💥 *%s was auto-played.* Absorbs the stack! Draws %d cards. (%d cards now)",
				name, game.stackTotal, len(player.hand)))
			game.stackTotal = 0
			game.stackMinValue = 0
			if p.checkMultiMercyElimination(game, player) {
				if game.done {
					return
				}
			}
			p.advanceAndExecute(game)
			return
		}

		// Find first playable non-action card
		playIdx := -1
		for i, c := range player.hand {
			if c.canPlayOn(game.discardTop, game.topColor) && !c.Value.isAction() && c.Value != unoWildDrawFour {
				playIdx = i
				break
			}
		}
		// If only action cards, play first playable
		if playIdx < 0 {
			for i, c := range player.hand {
				if c.canPlayOn(game.discardTop, game.topColor) {
					playIdx = i
					break
				}
			}
		}

		if playIdx >= 0 {
			card := player.hand[playIdx]
			player.hand = append(player.hand[:playIdx], player.hand[playIdx+1:]...)
			game.discardTop = card
			game.turns++

			if card.isWild() {
				if card.Value == unoWildDrawFour {
					game.wd4PrevColor = game.topColor
				}
				color := botPickColor(player.hand)
				game.topColor = color
				p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* %s (chose %s %s).", card.Display(), color.Emoji(), color))

				if len(player.hand) == 0 {
					p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
						name, card.Display(), color.Emoji(), color))
					p.multiPlayerWins(game, player)
					return
				}

				p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s (chose %s %s)",
					name, card.Display(), color.Emoji(), color))
				p.applyAutoEffects(game, player, card)
				return
			}

			game.topColor = card.Color
			p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* %s.", card.Display()))

			if len(player.hand) == 0 {
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s", name, card.Display()))
				p.multiPlayerWins(game, player)
				return
			}

			p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Plays: %s", name, card.Display()))
			p.applyAutoEffects(game, player, card)
			return
		}

		// No playable card — draw
		drawn := game.draw(1)
		if len(drawn) == 0 {
			p.SendMessage(player.dmRoomID, "*Auto-played:* No cards to draw. Turn passes.")
			p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* No cards to draw.", name))
			p.advanceAndExecute(game)
			return
		}

		card := drawn[0]
		player.hand = append(player.hand, card)

		if card.canPlayOn(game.discardTop, game.topColor) {
			// Auto-play the drawn card
			for i, c := range player.hand {
				if c == card {
					player.hand = append(player.hand[:i], player.hand[i+1:]...)
					break
				}
			}
			game.discardTop = card
			game.turns++

			if card.isWild() {
				if card.Value == unoWildDrawFour {
					game.wd4PrevColor = game.topColor
				}
				color := botPickColor(player.hand)
				game.topColor = color
				p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Drew and played %s (chose %s %s).", card.Display(), color.Emoji(), color))
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Drew and played %s (chose %s %s)",
					name, card.Display(), color.Emoji(), color))
			} else {
				game.topColor = card.Color
				p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Drew and played %s.", card.Display()))
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Drew and played %s", name, card.Display()))
			}

			if len(player.hand) == 0 {
				p.multiPlayerWins(game, player)
				return
			}

			p.applyAutoEffects(game, player, card)
			return
		}

		p.SendMessage(player.dmRoomID, fmt.Sprintf("*Auto-played:* Drew %s. Not playable. Turn passes.", card.Display()))
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Draws a card. Turn passes. (%d cards)", name, len(player.hand)))
		p.advanceAndExecute(game)
	}
}

// applyAutoEffects applies card effects after an auto-play and advances the turn.
func (p *UnoPlugin) applyAutoEffects(game *unoMultiGame, player *unoMultiPlayer, card unoCard) {
	eff := p.applyCardEffects(game, card)
	if eff.needsChallenge {
		p.startWD4Challenge(game, player)
		return
	}
	if p.checkMultiSuddenDeath(game) {
		return
	}
	p.executeMultiTurn(game)
}

// ---------------------------------------------------------------------------
// Sudden Death (multiplayer — only when 2 active players remain)
// ---------------------------------------------------------------------------

// checkMultiSuddenDeath checks whether sudden death should be announced or resolved.
// Only applies when exactly 2 active players remain. Returns true if the game ended.
// Caller must hold game.mu.
func (p *UnoPlugin) checkMultiSuddenDeath(game *unoMultiGame) bool {
	if game.done {
		return true
	}

	active := game.activePlayers()
	if len(active) != 2 {
		return false
	}

	// Resolve
	if game.suddenDeath && game.turns >= game.suddenDeathTurn {
		p.multiSuddenDeathWinner(game, active)
		return true
	}

	// Announce
	if !game.suddenDeath && game.turns >= suddenDeathAnnounce {
		game.suddenDeath = true
		game.suddenDeathTurn = game.turns + suddenDeathCountdown
		remaining := game.suddenDeathTurn - game.turns

		p.SendMessage(game.roomID, fmt.Sprintf(
			"⏰ **SUDDEN DEATH!** Match ends in %d turns — lowest hand value wins!\n\n%s",
			remaining, pickCommentary("sudden_death")))

		for _, pl := range active {
			if !pl.isBot {
				p.SendMessage(pl.dmRoomID, fmt.Sprintf(
					"⏰ **Sudden Death!** %d turns remaining — dump your high-value cards!", remaining))
			}
		}
		return false
	}

	// Countdown reminders
	if game.suddenDeath {
		remaining := game.suddenDeathTurn - game.turns
		switch remaining {
		case 10:
			p.SendMessage(game.roomID, "⏰ **10 turns remaining!**")
		case 5:
			p.SendMessage(game.roomID, "⏰ **5 turns remaining!**")
		}
	}

	return false
}

func (p *UnoPlugin) multiSuddenDeathWinner(game *unoMultiGame, active []*unoMultiPlayer) {
	if game.done {
		return
	}
	game.done = true

	if game.timer != nil {
		game.timer.Stop()
	}
	if game.inactiveTimer != nil {
		game.inactiveTimer.Stop()
	}

	a, b := active[0], active[1]
	aScore := scoreHand(a.hand)
	bScore := scoreHand(b.hand)

	nameOf := func(pl *unoMultiPlayer) string {
		if pl.isBot {
			return unoBotName()
		}
		return p.DisplayName(pl.userID)
	}

	breakdown := fmt.Sprintf("**Final Scores:**\n%s: %s\n%s: %s",
		nameOf(a), formatHandScore(a.hand),
		nameOf(b), formatHandScore(b.hand))

	var winner, loser *unoMultiPlayer
	switch {
	case aScore < bScore:
		winner, loser = a, b
	case bScore < aScore:
		winner, loser = b, a
	case len(a.hand) < len(b.hand):
		winner, loser = a, b
		breakdown += "\n*Tiebreaker: fewer cards!*"
	case len(b.hand) < len(a.hand):
		winner, loser = b, a
		breakdown += "\n*Tiebreaker: fewer cards!*"
	default:
		// Tiebreaker: player whose turn it is NOT wins
		current := game.currentPlayer()
		if current == a {
			winner, loser = b, a
		} else {
			winner, loser = a, b
		}
		breakdown += "\n*Tiebreaker: dead even — advantage to the opponent!*"
	}
	_ = loser // used implicitly via winner != loser

	p.SendMessage(game.roomID, fmt.Sprintf(
		"⏰ **SUDDEN DEATH!** %s wins on points!\n%s\n\n%s",
		nameOf(winner), breakdown, pickCommentary("sudden_death_win")))

	if winner.isBot {
		p.multiBotWins2(game)
	} else {
		p.multiPlayerWins2(game, winner)
	}
}

// multiPlayerWins2 / multiBotWins2 handle payout without duplicate done/timer logic
// (already handled by multiSuddenDeathWinner).
func (p *UnoPlugin) multiPlayerWins2(game *unoMultiGame, winner *unoMultiPlayer) {
	humanCount := 0
	for _, pl := range game.players {
		if !pl.isBot {
			humanCount++
		}
	}
	totalPot := game.ante * float64(humanCount)
	p.euro.Credit(winner.userID, totalPot, "uno_multi_win")

	p.recordMultiGame(game, winner.userID, "sudden_death_win")
	p.cleanupMultiGame(game)
}

func (p *UnoPlugin) multiBotWins2(game *unoMultiGame) {
	humanCount := 0
	for _, pl := range game.players {
		if !pl.isBot {
			humanCount++
		}
	}
	totalPot := game.ante * float64(humanCount)
	p.addToPot(totalPot)

	for _, pl := range game.players {
		if !pl.isBot {
			recordBotDefeat(pl.userID, "uno_multi")
		}
	}
	p.recordMultiGame(game, id.UserID("bot"), "sudden_death_bot")
	p.cleanupMultiGame(game)
}

// ---------------------------------------------------------------------------
// Win / Forfeit / Cleanup
// ---------------------------------------------------------------------------

func (p *UnoPlugin) multiPlayerWins(game *unoMultiGame, winner *unoMultiPlayer) {
	if game.done {
		return
	}
	game.done = true

	if game.timer != nil {
		game.timer.Stop()
	}
	if game.inactiveTimer != nil {
		game.inactiveTimer.Stop()
	}

	// Calculate pot (all human antes)
	humanCount := 0
	for _, pl := range game.players {
		if !pl.isBot {
			humanCount++
		}
	}
	totalPot := game.ante * float64(humanCount)

	name := p.DisplayName(winner.userID)
	p.euro.Credit(winner.userID, totalPot, "uno_multi_win")

	p.SendMessage(game.roomID, fmt.Sprintf(
		"🎉 **%s wins Multiplayer UNO!**\nPot: €%d | Turns: %d\n\n%s",
		name, int(totalPot), game.turns, pickCommentary("player_win")))

	p.recordMultiGame(game, winner.userID, "win")
	p.cleanupMultiGame(game)
}

func (p *UnoPlugin) multiBotWins(game *unoMultiGame) {
	if game.done {
		return
	}
	game.done = true

	if game.timer != nil {
		game.timer.Stop()
	}
	if game.inactiveTimer != nil {
		game.inactiveTimer.Stop()
	}

	humanCount := 0
	for _, pl := range game.players {
		if !pl.isBot {
			humanCount++
		}
	}
	totalPot := game.ante * float64(humanCount)

	// Pot goes to community
	p.addToPot(totalPot)

	bn := unoBotName()
	p.SendMessage(game.roomID, fmt.Sprintf(
		"💀 **%s wins Multiplayer UNO!**\n€%d goes to the community pot.\n\n%s",
		bn, int(totalPot), pickCommentary("bot_win")))

	for _, pl := range game.players {
		if !pl.isBot {
			recordBotDefeat(pl.userID, "uno_multi")
		}
	}
	p.recordMultiGame(game, id.UserID("bot"), "bot_win")
	p.cleanupMultiGame(game)
}

func (p *UnoPlugin) multiPlayerForfeit(game *unoMultiGame, player *unoMultiPlayer) {
	player.active = false
	name := p.DisplayName(player.userID)

	// Shuffle cards back into draw pile
	game.drawPile = append(game.drawPile, player.hand...)
	rand.Shuffle(len(game.drawPile), func(i, j int) { game.drawPile[i], game.drawPile[j] = game.drawPile[j], game.drawPile[i] })
	player.hand = nil

	// Check if game should end
	active := game.activePlayers()
	if len(active) <= 1 {
		if len(active) == 1 {
			winner := active[0]
			if winner.isBot {
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s forfeited. All humans out!", name))
				p.multiBotWins(game)
			} else {
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s forfeited. Only one player remains!", name))
				p.multiPlayerWins(game, winner)
			}
		} else {
			game.done = true
			p.SendMessage(game.roomID, "🃏 Game ended — no players remaining.")
			p.cleanupMultiGame(game)
		}
		return
	}

	p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s forfeited! Game continues with %d players.", name, len(active)))

	// If it was this player's turn, advance
	if game.currentPlayer() == player || !game.currentPlayer().active {
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
		p.executeMultiTurn(game)
	}
}

func (p *UnoPlugin) cleanupMultiGame(game *unoMultiGame) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.multiGames, game.id)
	for roomID, gID := range p.dmToMulti {
		if gID == game.id {
			delete(p.dmToMulti, roomID)
		}
	}
}

func (p *UnoPlugin) recordMultiGame(game *unoMultiGame, winnerID id.UserID, result string) {
	playerIDs := make([]string, 0)
	for _, pl := range game.players {
		if !pl.isBot {
			playerIDs = append(playerIDs, string(pl.userID))
		}
	}

	humanCount := len(playerIDs)
	totalPot := game.ante * float64(humanCount)

	db.Exec("uno_multi: record game",
		`INSERT INTO uno_multi_games (room_id, ante, pot_total, winner_id, player_ids, result, turns, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(game.roomID), game.ante, totalPot, string(winnerID),
		strings.Join(playerIDs, ","), result, game.turns,
		game.startedAt.UTC().Format("2006-01-02 15:04:05"),
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
}

// ---------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------

func (p *UnoPlugin) sendMultiStatus(game *unoMultiGame, player *unoMultiPlayer) {
	var sb strings.Builder
	bn := unoBotName()
	current := game.currentPlayer()
	currentName := p.DisplayName(current.userID)
	if current.isBot {
		currentName = bn
	}

	sb.WriteString(fmt.Sprintf("🃏 **UNO Status**\nDiscard pile: %s\nCurrent turn: %s\n\n",
		game.discardTop.DisplayWithColor(game.topColor), currentName))

	sb.WriteString("**Your hand:**\n")
	for i, c := range player.hand {
		playable := ""
		if c.canPlayOn(game.discardTop, game.topColor) {
			playable = " ✅"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, c.Display(), playable))
	}

	sb.WriteString("\nCard counts: ")
	var counts []string
	for _, pl := range game.players {
		if pl == player || !pl.active {
			continue
		}
		name := p.DisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		counts = append(counts, fmt.Sprintf("%s (%d)", name, len(pl.hand)))
	}
	sb.WriteString(strings.Join(counts, " | "))

	p.SendMessage(player.dmRoomID, sb.String())
}

func (p *UnoPlugin) sendMultiHandDisplay(game *unoMultiGame, player *unoMultiPlayer) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🃏 **Your turn!**\nDiscard pile: %s\n\n**Your hand:**\n",
		game.discardTop.DisplayWithColor(game.topColor)))

	for i, c := range player.hand {
		playable := ""
		if c.canPlayOn(game.discardTop, game.topColor) {
			playable = " ✅"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, c.Display(), playable))
	}

	// Card counts for all opponents
	sb.WriteString("\nCard counts: ")
	counts := make([]string, 0)
	bn := unoBotName()
	for _, pl := range game.players {
		if pl == player || !pl.active {
			continue
		}
		name := p.DisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		counts = append(counts, fmt.Sprintf("%s (%d)", name, len(pl.hand)))
	}
	sb.WriteString(strings.Join(counts, " | "))

	if game.noMercy && len(player.hand) >= 20 {
		sb.WriteString(fmt.Sprintf("\n⚠️ **You have %d cards! (25 = eliminated)**", len(player.hand)))
	}

	sb.WriteString("\n\nReply with a card number to play, or **draw** to draw.")

	p.SendMessage(player.dmRoomID, sb.String())
}

// sendMultiHandDisplayStacking shows hand during stacking — only stackable cards are playable.
func (p *UnoPlugin) sendMultiHandDisplayStacking(game *unoMultiGame, player *unoMultiPlayer) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ **Stack incoming: +%d!**\nDiscard pile: %s\n\n**Your hand:**\n",
		game.stackTotal, game.discardTop.DisplayWithColor(game.topColor)))

	for i, c := range player.hand {
		marker := ""
		if c.canPlayOnStacking(game.topColor, game.stackMinValue) {
			marker = " ✅"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, c.Display(), marker))
	}

	sb.WriteString("\nCard counts: ")
	counts := make([]string, 0)
	bn := unoBotName()
	for _, pl := range game.players {
		if pl == player || !pl.active {
			continue
		}
		name := p.DisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		counts = append(counts, fmt.Sprintf("%s (%d)", name, len(pl.hand)))
	}
	sb.WriteString(strings.Join(counts, " | "))

	sb.WriteString(fmt.Sprintf("\n\nPlay a draw card to stack, or type **accept** to draw %d cards.", game.stackTotal))

	p.SendMessage(player.dmRoomID, sb.String())
}

// handleMultiSwapChoice handles the player choosing a swap target for the 7-0 rule.
func (p *UnoPlugin) handleMultiSwapChoice(game *unoMultiGame, player *unoMultiPlayer, input string) error {
	// Parse target number
	var targetIdx int
	if _, err := fmt.Sscanf(input, "%d", &targetIdx); err != nil || targetIdx < 1 {
		p.SendMessage(player.dmRoomID, "Choose a player number to swap hands with.")
		return nil
	}

	// Map number to active player
	i := 1
	var target *unoMultiPlayer
	for _, pl := range game.players {
		if pl == player || !pl.active {
			continue
		}
		if i == targetIdx {
			target = pl
			break
		}
		i++
	}

	if target == nil {
		p.SendMessage(player.dmRoomID, "Invalid player number. Try again.")
		return nil
	}

	swapHandsMulti(player, target)
	game.phase = unoMultiPhasePlay

	name := p.DisplayName(player.userID)
	targetName := p.DisplayName(target.userID)
	if target.isBot {
		targetName = unoBotName()
	}

	p.SendMessage(player.dmRoomID, fmt.Sprintf("Swapped hands with %s! You now have %d cards.", targetName, len(player.hand)))
	if !target.isBot {
		p.SendMessage(target.dmRoomID, fmt.Sprintf("🔄 %s swapped hands with you! You now have %d cards.", name, len(target.hand)))
	}
	p.SendMessage(game.roomID, fmt.Sprintf("🔄 %s swaps hands with %s! %s",
		name, targetName, pickNoMercyCommentary("hand_swap")))

	// Check win (if player got an empty hand from swap)
	if len(player.hand) == 0 {
		p.multiPlayerWins(game, player)
		return nil
	}

	// Continue to next turn
	p.advanceAndExecute(game)
	return nil
}
