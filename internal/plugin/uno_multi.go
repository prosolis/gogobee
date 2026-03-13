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
	unoMultiPhasePlay        unoMultiPhase = iota
	unoMultiPhaseChooseColor               // active player must pick a color
	unoMultiPhaseDrawnPlayable             // active player drew a playable card, yes/no
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
	drawnCard  *unoCard  // card drawn this turn
	pendingCard *unoCard // wild waiting for color
	turns      int
	turnID     int // monotonic, used to invalidate stale timers
	startedAt  time.Time
	done       bool
	bookDown   bool

	timer *time.Timer
	mu    sync.Mutex // per-game lock
}

type unoMultiLobby struct {
	roomID    id.RoomID
	creator   id.UserID
	ante      float64
	players   []id.UserID
	createdAt time.Time
	timer     *time.Timer
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
	fresh := newUnoDeck()
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

func (p *UnoPlugin) handleMultiStart(ctx MessageContext, amountStr string) error {
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
	p.mu.Unlock()

	// Debit ante
	if !p.euro.Debit(ctx.Sender, amount, "uno_multi_ante") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Insufficient balance for that ante.")
	}

	timeout := envInt("UNO_MULTI_LOBBY_TIMEOUT", 300)
	lobby := &unoMultiLobby{
		roomID:    ctx.RoomID,
		creator:   ctx.Sender,
		ante:      amount,
		players:   []id.UserID{ctx.Sender},
		createdAt: time.Now(),
	}

	lobby.timer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		p.lobbyExpired(ctx.RoomID)
	})

	p.mu.Lock()
	p.lobbies[ctx.RoomID] = lobby
	p.mu.Unlock()

	creatorName := p.unoDisplayName(ctx.Sender)
	return p.SendMessage(ctx.RoomID, fmt.Sprintf(
		"🃏 **UNO Lobby** — Ante: €%d\nPlayers (1/4):\n  1. %s (host)\n\nType `!uno join` to join or `!uno go` to start!",
		int(amount), creatorName))
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
	p.mu.Unlock()

	// Debit ante
	if !p.euro.Debit(ctx.Sender, lobby.ante, "uno_multi_ante") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Insufficient balance for the ante.")
	}

	p.mu.Lock()
	lobby.players = append(lobby.players, ctx.Sender)
	count := len(lobby.players)
	p.mu.Unlock()

	// Build player list
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🃏 **UNO Lobby** — Ante: €%d\nPlayers (%d/4):\n", int(lobby.ante), count))
	for i, uid := range lobby.players {
		name := p.unoDisplayName(uid)
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
	name := p.unoDisplayName(ctx.Sender)
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
			return p.SendMessage(roomID, fmt.Sprintf("🃏 Game cancelled — couldn't open DMs with %s.", p.unoDisplayName(uid)))
		}
		resolved = append(resolved, playerDMPair{uid, dmRoom})
	}

	// Build game
	game := p.initMultiGame(resolved, roomID, ante)

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
	sb.WriteString(fmt.Sprintf("🃏 **Multiplayer UNO!** Pot: €%d\n\nPlayers:\n", int(ante)*len(players)))
	for i, pl := range game.players {
		name := p.unoDisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		marker := ""
		if i == game.currentIdx {
			marker = " ← first turn"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, name, marker))
	}
	sb.WriteString(fmt.Sprintf("\nStarting card: %s\n%s\n\n[Check your DMs!]",
		game.discardTop.DisplayWithColor(game.topColor), pickCommentary("start")))
	p.SendMessage(roomID, sb.String())

	// Start first turn
	game.mu.Lock()
	defer game.mu.Unlock()
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

func (p *UnoPlugin) initMultiGame(players []playerDMPair, roomID id.RoomID, ante float64) *unoMultiGame {
	deck := newUnoDeck()
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

	// Starting card — must not be Wild
	var startCard unoCard
	startIdx := -1
	for i, c := range remaining {
		if c.Value != unoWildCard && c.Value != unoWildDrawFour {
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
		id:         gameID,
		roomID:     roomID,
		ante:       ante,
		players:    unshuffled,
		currentIdx: 0,
		direction:  1,
		drawPile:   remaining,
		discardTop: startCard,
		topColor:   startCard.Color,
		phase:      unoMultiPhasePlay,
		turns:      0,
		turnID:     0,
		startedAt:  time.Now(),
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

	for {
		if game.done {
			return
		}

		player := game.currentPlayer()

		if player.isBot {
			botTurnsInRow++
			if botTurnsInRow > 10 {
				// Safety: shouldn't happen, but prevent infinite loops
				break
			}
			p.botMultiTurn(game, &roomBuf)
			if game.done {
				if roomBuf.Len() > 0 {
					p.SendMessage(game.roomID, roomBuf.String())
				}
				return
			}
			continue
		}

		// Human's turn — flush room buffer
		if roomBuf.Len() > 0 {
			p.SendMessage(game.roomID, roomBuf.String())
			roomBuf.Reset()
		}

		// Auto-draw if no playable cards
		if !game.hasPlayable(player.hand) {
			drawn := game.draw(1)
			if len(drawn) == 0 {
				// Empty deck, no playable cards — pass
				name := p.unoDisplayName(player.userID)
				p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s has no playable cards and deck is empty. Turn passes.", name))
				p.SendMessage(player.dmRoomID, "No playable cards and deck is empty. Turn passes.")
				game.currentIdx = game.nextActiveIdx()
				game.turnID++
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

			name := p.unoDisplayName(player.userID)
			p.SendMessage(player.dmRoomID,
				fmt.Sprintf("No playable cards — drew automatically: %s\nNot playable. Turn passes.", card.Display()))
			p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes.", name))
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
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
	p.executeMultiTurn(game)
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

	// Reset auto-play counter on manual input
	player.autoPlays = 0

	// Cancel timer
	if game.timer != nil {
		game.timer.Stop()
	}

	if input == "quit" || input == "forfeit" {
		p.multiPlayerForfeit(game, player)
		return nil
	}

	switch game.phase {
	case unoMultiPhaseChooseColor:
		return p.handleMultiColorChoice(game, player, input)

	case unoMultiPhaseDrawnPlayable:
		return p.handleMultiDrawnPlayable(game, player, input)

	case unoMultiPhasePlay:
		if input == "uno" {
			player.calledUno = true
			p.SendMessage(player.dmRoomID, "✅ UNO called!")
			return nil
		}
		if input == "draw" {
			return p.handleMultiPlayerDraw(game, player)
		}
		var cardIdx int
		if _, err := fmt.Sscanf(input, "%d", &cardIdx); err != nil || cardIdx < 1 || cardIdx > len(player.hand) {
			p.SendMessage(player.dmRoomID,
				fmt.Sprintf("Reply with a number (1-%d) to play, or **draw** to draw.", len(player.hand)))
			return nil
		}
		return p.handleMultiPlayerPlay(game, player, cardIdx-1)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Play handlers
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleMultiPlayerPlay(game *unoMultiGame, player *unoMultiPlayer, idx int) error {
	card := player.hand[idx]

	if !card.canPlayOn(game.discardTop, game.topColor) {
		p.SendMessage(player.dmRoomID, fmt.Sprintf("You can't play %s on %s.",
			card.Display(), game.discardTop.DisplayWithColor(game.topColor)))
		return nil
	}

	// UNO penalty check — had 2 cards, didn't call UNO
	if len(player.hand) == 2 && !player.calledUno {
		drawn := game.draw(2)
		player.hand = append(player.hand, drawn...)
		player.calledUno = false
		p.SendMessage(player.dmRoomID, "⚠️ You forgot to call UNO! Draw 2 as penalty.")
		name := p.unoDisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s forgot to call UNO! 2 card penalty.", name))
		// Re-show hand
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

	// Wild — need color choice
	if card.Value == unoWildCard || card.Value == unoWildDrawFour {
		game.pendingCard = &card
		game.phase = unoMultiPhaseChooseColor
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("You played **%s**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green", card.Value))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	game.topColor = card.Color

	// Check win
	if len(player.hand) == 0 {
		name := p.unoDisplayName(player.userID)
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
		name := p.unoDisplayName(player.userID)
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s plays: %s (chose %s %s)",
			name, pendingCard.Display(), color.Emoji(), color))
		p.multiPlayerWins(game, player)
		return nil
	}

	// Apply effects
	p.applyAndAnnounce(game, player, *pendingCard)
	return nil
}

func (p *UnoPlugin) handleMultiPlayerDraw(game *unoMultiGame, player *unoMultiPlayer) error {
	drawn := game.draw(1)
	if len(drawn) == 0 {
		p.SendMessage(player.dmRoomID, "No cards left to draw! Turn passes.")
		name := p.unoDisplayName(player.userID)
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

	name := p.unoDisplayName(player.userID)
	p.SendMessage(player.dmRoomID, fmt.Sprintf("You drew: %s\nNot playable. Turn passes.", card.Display()))
	p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes.", name))
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
		name := p.unoDisplayName(player.userID)
		p.SendMessage(player.dmRoomID, "Card kept. Turn passes.")
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 %s draws a card. Turn passes.", name))
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

	if drawnCard.Value == unoWildCard || drawnCard.Value == unoWildDrawFour {
		game.pendingCard = &drawnCard
		game.phase = unoMultiPhaseChooseColor
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("You played **%s**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green", drawnCard.Value))
		p.startMultiAutoPlayTimer(game)
		return nil
	}

	game.topColor = drawnCard.Color

	if len(player.hand) == 0 {
		name := p.unoDisplayName(player.userID)
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

func (p *UnoPlugin) applyAndAnnounce(game *unoMultiGame, player *unoMultiPlayer, card unoCard) {
	name := p.unoDisplayName(player.userID)
	var roomMsg strings.Builder

	roomMsg.WriteString(fmt.Sprintf("🃏 %s plays: %s", name, card.DisplayWithColor(game.topColor)))

	// Determine next player for effects
	nextIdx := game.nextActiveIdx()
	nextPlayer := game.players[nextIdx]
	nextName := p.unoDisplayName(nextPlayer.userID)
	if nextPlayer.isBot {
		nextName = unoBotName()
	}

	switch card.Value {
	case unoSkip:
		// In multiplayer, skip always skips the next player
		roomMsg.WriteString(fmt.Sprintf("\n  %s is skipped!", nextName))
		// Advance past the skipped player
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoReverse:
		game.direction *= -1
		activePlayers := game.activePlayers()
		if len(activePlayers) == 2 {
			// 2-player: reverse = skip
			roomMsg.WriteString(fmt.Sprintf("\n  %s is skipped! (reverse)", nextName))
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			roomMsg.WriteString("\n  Direction reversed!")
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoDrawTwo:
		drawn := game.draw(2)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		roomMsg.WriteString(fmt.Sprintf("\n  %s draws 2 and is skipped!", nextName))
		// Skip past the victim
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoWildDrawFour:
		drawn := game.draw(4)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		roomMsg.WriteString(fmt.Sprintf("\n  %s draws 4 and is skipped!", nextName))
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	default:
		// Normal card
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
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

	// UNO announcement
	if len(player.hand) == 1 && player.calledUno {
		roomMsg.WriteString(fmt.Sprintf("\n  %s calls UNO! 🔥", name))
	}

	// Next player
	nextUp := game.currentPlayer()
	nextUpName := p.unoDisplayName(nextUp.userID)
	if nextUp.isBot {
		nextUpName = unoBotName()
	}
	roomMsg.WriteString(fmt.Sprintf("\n  It's %s's turn.", nextUpName))

	p.SendMessage(game.roomID, roomMsg.String())
	p.executeMultiTurn(game)
}

// ---------------------------------------------------------------------------
// Bot turn (multiplayer)
// ---------------------------------------------------------------------------

func (p *UnoPlugin) botMultiTurn(game *unoMultiGame, roomBuf *strings.Builder) {
	bot := game.currentPlayer()
	bn := unoBotName()

	card, idx := botPickCard(bot.hand, game.discardTop, game.topColor, game.bookDown, game.minOpponentCards(game.currentIdx))

	if idx < 0 {
		// Bot draws
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
			// Bot drew, not playable
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

	// Play the card
	if idx >= 0 {
		bot.hand = append(bot.hand[:idx], bot.hand[idx+1:]...)
	}
	game.discardTop = card

	// Wild color choice
	if card.Value == unoWildCard || card.Value == unoWildDrawFour {
		game.topColor = botPickColor(bot.hand)
	} else {
		game.topColor = card.Color
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
	nextIdx := game.nextActiveIdx()
	nextPlayer := game.players[nextIdx]
	nextName := p.unoDisplayName(nextPlayer.userID)
	if nextPlayer.isBot {
		nextName = bn
	}

	switch card.Value {
	case unoSkip:
		roomBuf.WriteString(fmt.Sprintf("\n  %s is skipped!", nextName))
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoReverse:
		game.direction *= -1
		if len(game.activePlayers()) == 2 {
			roomBuf.WriteString(fmt.Sprintf("\n  %s is skipped! (reverse)", nextName))
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			roomBuf.WriteString("\n  Direction reversed!")
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}

	case unoDrawTwo:
		drawn := game.draw(2)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		roomBuf.WriteString(fmt.Sprintf("\n  %s draws 2 and is skipped!", nextName))
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	case unoWildDrawFour:
		drawn := game.draw(4)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		roomBuf.WriteString(fmt.Sprintf("\n  %s draws 4 and is skipped!", nextName))
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++

	default:
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	}

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

		name := p.unoDisplayName(player.userID)

		if player.autoPlays >= maxAutoPlays {
			p.SendMessage(mg.roomID, fmt.Sprintf("🃏 %s was auto-played %d times in a row — forfeited!", name, maxAutoPlays))
			p.multiPlayerForfeit(mg, player)
			return
		}

		// Auto-play: find first playable non-action card
		p.autoPlayMultiTurn(mg, player)
	})
}

func (p *UnoPlugin) autoPlayMultiTurn(game *unoMultiGame, player *unoMultiPlayer) {
	name := p.unoDisplayName(player.userID)

	switch game.phase {
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

		if drawnCard.Value == unoWildCard || drawnCard.Value == unoWildDrawFour {
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

	case unoMultiPhasePlay:
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

			if card.Value == unoWildCard || card.Value == unoWildDrawFour {
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

			if card.Value == unoWildCard || card.Value == unoWildDrawFour {
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
		p.SendMessage(game.roomID, fmt.Sprintf("🃏 *%s was auto-played.* Draws a card. Turn passes.", name))
		p.advanceAndExecute(game)
	}
}

// applyAutoEffects applies card effects after an auto-play and advances the turn.
func (p *UnoPlugin) applyAutoEffects(game *unoMultiGame, player *unoMultiPlayer, card unoCard) {
	nextIdx := game.nextActiveIdx()
	nextPlayer := game.players[nextIdx]

	switch card.Value {
	case unoSkip:
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	case unoReverse:
		game.direction *= -1
		if len(game.activePlayers()) == 2 {
			game.currentIdx = nextIdx
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		} else {
			game.currentIdx = game.nextActiveIdx()
			game.turnID++
		}
	case unoDrawTwo:
		drawn := game.draw(2)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	case unoWildDrawFour:
		drawn := game.draw(4)
		nextPlayer.hand = append(nextPlayer.hand, drawn...)
		game.currentIdx = nextIdx
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	default:
		game.currentIdx = game.nextActiveIdx()
		game.turnID++
	}

	p.executeMultiTurn(game)
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

	// Calculate pot (all human antes)
	humanCount := 0
	for _, pl := range game.players {
		if !pl.isBot {
			humanCount++
		}
	}
	totalPot := game.ante * float64(humanCount)

	name := p.unoDisplayName(winner.userID)
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

	p.recordMultiGame(game, id.UserID("bot"), "bot_win")
	p.cleanupMultiGame(game)
}

func (p *UnoPlugin) multiPlayerForfeit(game *unoMultiGame, player *unoMultiPlayer) {
	player.active = false
	name := p.unoDisplayName(player.userID)

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
	d := db.Get()

	playerIDs := make([]string, 0)
	for _, pl := range game.players {
		if !pl.isBot {
			playerIDs = append(playerIDs, string(pl.userID))
		}
	}

	humanCount := len(playerIDs)
	totalPot := game.ante * float64(humanCount)

	_, err := d.Exec(
		`INSERT INTO uno_multi_games (room_id, ante, pot_total, winner_id, player_ids, result, turns, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(game.roomID), game.ante, totalPot, string(winnerID),
		strings.Join(playerIDs, ","), result, game.turns,
		game.startedAt.UTC().Format("2006-01-02 15:04:05"),
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		slog.Error("uno_multi: failed to record game", "err", err)
	}
}

// ---------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------

func (p *UnoPlugin) sendMultiStatus(game *unoMultiGame, player *unoMultiPlayer) {
	var sb strings.Builder
	bn := unoBotName()
	current := game.currentPlayer()
	currentName := p.unoDisplayName(current.userID)
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
		name := p.unoDisplayName(pl.userID)
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
		name := p.unoDisplayName(pl.userID)
		if pl.isBot {
			name = bn
		}
		counts = append(counts, fmt.Sprintf("%s (%d)", name, len(pl.hand)))
	}
	sb.WriteString(strings.Join(counts, " | "))

	sb.WriteString("\n\nReply with a card number to play, or **draw** to draw.")

	p.SendMessage(player.dmRoomID, sb.String())
}
