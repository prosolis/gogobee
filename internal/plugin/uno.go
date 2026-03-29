package plugin

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// ---------------------------------------------------------------------------
// Card types
// ---------------------------------------------------------------------------

type unoColor int

const (
	unoRed unoColor = iota
	unoBlue
	unoYellow
	unoGreen
	unoWild // wilds have no color until played
)

func (c unoColor) String() string {
	switch c {
	case unoRed:
		return "Red"
	case unoBlue:
		return "Blue"
	case unoYellow:
		return "Yellow"
	case unoGreen:
		return "Green"
	default:
		return "Wild"
	}
}

func (c unoColor) Emoji() string {
	switch c {
	case unoRed:
		return "🟥"
	case unoBlue:
		return "🟦"
	case unoYellow:
		return "🟨"
	case unoGreen:
		return "🟩"
	default:
		return "🃏"
	}
}

type unoValue int

const (
	unoZero unoValue = iota
	unoOne
	unoTwo
	unoThree
	unoFour
	unoFive
	unoSix
	unoSeven
	unoEight
	unoNine
	unoSkip
	unoReverse // acts as skip in 2-player
	unoDrawTwo
	unoWildCard
	unoWildDrawFour
	// No Mercy card types
	unoSkipEveryone      // colored — skips all other players
	unoDrawFour          // colored Draw Four (NOT wild)
	unoDiscardAll        // colored — discard all of that color from hand
	unoWildReverseDraw4  // wild — reverse + draw 4 + color change
	unoWildDrawSix       // wild — draw 6
	unoWildDrawTen       // wild — draw 10
	unoWildColorRoulette // wild — next player flips until chosen color
)

func (v unoValue) String() string {
	switch {
	case v <= unoNine:
		return fmt.Sprintf("%d", int(v))
	case v == unoSkip:
		return "Skip"
	case v == unoReverse:
		return "Reverse"
	case v == unoDrawTwo:
		return "Draw Two"
	case v == unoWildCard:
		return "Wild"
	case v == unoWildDrawFour:
		return "Wild Draw Four"
	case v == unoSkipEveryone:
		return "Skip Everyone"
	case v == unoDrawFour:
		return "Draw Four"
	case v == unoDiscardAll:
		return "Discard All"
	case v == unoWildReverseDraw4:
		return "Wild Reverse Draw Four"
	case v == unoWildDrawSix:
		return "Wild Draw Six"
	case v == unoWildDrawTen:
		return "Wild Draw Ten"
	case v == unoWildColorRoulette:
		return "Wild Color Roulette"
	default:
		return "?"
	}
}

func (v unoValue) isAction() bool {
	return v == unoSkip || v == unoReverse || v == unoDrawTwo ||
		v == unoWildCard || v == unoWildDrawFour ||
		v == unoSkipEveryone || v == unoDrawFour || v == unoDiscardAll ||
		v == unoWildReverseDraw4 || v == unoWildDrawSix ||
		v == unoWildDrawTen || v == unoWildColorRoulette
}

type unoCard struct {
	Color unoColor
	Value unoValue
}

func (c unoCard) isWild() bool {
	return c.Value == unoWildCard || c.Value == unoWildDrawFour ||
		c.Value == unoWildReverseDraw4 || c.Value == unoWildDrawSix ||
		c.Value == unoWildDrawTen || c.Value == unoWildColorRoulette
}

func (c unoCard) Display() string {
	if c.isWild() {
		return fmt.Sprintf("%s %s", unoWild.Emoji(), c.Value)
	}
	return fmt.Sprintf("%s %s", c.Color.Emoji(), c.Value)
}

// DisplayWithColor shows a wild card with its chosen color.
func (c unoCard) DisplayWithColor(chosenColor unoColor) string {
	if c.isWild() {
		return fmt.Sprintf("%s %s", chosenColor.Emoji(), c.Value)
	}
	return c.Display()
}

func (c unoCard) canPlayOn(top unoCard, topColor unoColor) bool {
	if c.isWild() {
		return true
	}
	return c.Color == topColor || c.Value == top.Value
}

// ---------------------------------------------------------------------------
// Deck
// ---------------------------------------------------------------------------

func newUnoDeck() []unoCard {
	var cards []unoCard
	colors := []unoColor{unoRed, unoBlue, unoYellow, unoGreen}

	for _, color := range colors {
		// One zero per color
		cards = append(cards, unoCard{color, unoZero})
		// Two of each 1-9, Skip, Reverse, Draw Two
		for v := unoOne; v <= unoDrawTwo; v++ {
			cards = append(cards, unoCard{color, v})
			cards = append(cards, unoCard{color, v})
		}
	}
	// 4 Wild, 4 Wild Draw Four
	for i := 0; i < 4; i++ {
		cards = append(cards, unoCard{unoWild, unoWildCard})
		cards = append(cards, unoCard{unoWild, unoWildDrawFour})
	}

	// Shuffle
	rand.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
	return cards
}

// ---------------------------------------------------------------------------
// Game state
// ---------------------------------------------------------------------------

type unoPhase int

const (
	unoPhasePlay unoPhase = iota
	unoPhaseChooseColor
	unoPhaseDrawnPlayable // player drew a playable card, yes/no
)

type unoGame struct {
	playerID    id.UserID
	roomID      id.RoomID // games room where it started
	dmRoomID    id.RoomID // DM room for gameplay
	wager       float64
	playerHand  []unoCard
	botHand     []unoCard
	drawPile    []unoCard
	discardTop  unoCard
	topColor    unoColor // effective color (matters for wilds)
	phase       unoPhase
	drawnCard   *unoCard // card drawn this turn (for draw-then-play)
	pendingCard *unoCard // wild card waiting for color choice
	calledUno   bool     // player called uno this round
	bookDown    bool     // gogobee is paying attention
	turns       int
	startedAt   time.Time
	done        bool // set true when game ends — prevents double-completion

	// No Mercy mode
	noMercy       bool
	sevenZeroRule bool
	stackTotal    int // cumulative draw penalty during stacking
	stackMinValue int // minimum draw value to stack (0 = not stacking)

	// Sudden death (long-game point scoring)
	suddenDeath     bool
	suddenDeathTurn int // turn at which game ends

	idleTimer    *time.Timer
	warningTimer *time.Timer
}

func (g *unoGame) draw(n int) []unoCard {
	var drawn []unoCard
	for i := 0; i < n; i++ {
		if len(g.drawPile) == 0 {
			g.reshuffleDiscard()
		}
		if len(g.drawPile) == 0 {
			break // truly empty, shouldn't happen with 108 cards
		}
		drawn = append(drawn, g.drawPile[0])
		g.drawPile = g.drawPile[1:]
	}
	return drawn
}

func (g *unoGame) reshuffleDiscard() {
	// Rebuild deck from cards not in play
	var fresh []unoCard
	if g.noMercy {
		fresh = newNoMercyDeck()
	} else {
		fresh = newUnoDeck()
	}
	inPlay := make(map[unoCard]int)

	for _, c := range g.playerHand {
		inPlay[c]++
	}
	for _, c := range g.botHand {
		inPlay[c]++
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

func (g *unoGame) hasPlayable() bool {
	for _, c := range g.playerHand {
		if c.canPlayOn(g.discardTop, g.topColor) {
			return true
		}
	}
	return false
}

func (g *unoGame) updateBookState() bool {
	wasDown := g.bookDown
	g.bookDown = len(g.playerHand) == 2
	return wasDown != g.bookDown
}

// ---------------------------------------------------------------------------
// GogoBee commentary
// ---------------------------------------------------------------------------

var unoCommentary = map[string][]string{
	"start": {
		"*GogoBee sets the book down, marks the page carefully, and looks up with an expression of polite interest.*\n\n\"Oh, we're doing this. Alright. 💛\"\n\n*deals cards*",
	},
	"bot_play_normal": {
		"GogoBee plays: %s. 💛 *doesn't look up*",
		"GogoBee plays: %s. 💛 *turns a page*",
	},
	"bot_play_bookdown": {
		"GogoBee plays: %s. 💛",
	},
	"bot_draw_normal": {
		"GogoBee draws a card. 💛 *doesn't look up*",
	},
	"bot_draw_bookdown": {
		"GogoBee draws a card. 💛",
	},
	"bot_draw_two": {
		"\"Oh, sorry about that. Draw two. 💛\" *turns a page*",
	},
	"bot_draw_two_bookdown": {
		"\"Oh, sorry about that. Draw two. 💛\"",
	},
	"bot_wild_draw_four": {
		"\"So sorry. Draw four. 💛\" *doesn't look up*",
	},
	"bot_wild_draw_four_bookdown": {
		"\"So sorry. Draw four. 💛\"",
	},
	"book_down": {
		"*GogoBee sets the book down.*\n\"Hm. 💛\"",
	},
	"book_up": {
		"*picks the book back up*\n\"Mm. 💛\"",
	},
	"bot_uno": {
		"\"Uno, by the way. 💛\" *glances up briefly*",
	},
	"player_forgot_uno": {
		"\"Oh, you forgot to say uno. Draw two. 💛\" *turns a page*",
	},
	"bot_win": {
		"\"Oh that's unfortunate. Better luck next time. 💛\"\n*shuffles deck before you've even left the table*",
	},
	"bot_lose": {
		"\"Oh! You got me. Well done. 💛\"\n*resumes reading*",
	},
	"bot_lose_empty_pot": {
		"\"...I see. The pot is yours. 💛\"\n*marks the page and sets the book down properly this time*\n\"Don't get comfortable.\"",
	},
	"long_game": {
		"\"Still going, are we? 💛\" *glances at bookmark*",
	},
	"sudden_death": {
		"*GogoBee puts the book down.* \"This has gone on long enough. 💛\"",
		"*GogoBee glances at the clock.* \"Time to settle this. 💛\"",
	},
	"sudden_death_win": {
		"\"Well played. The numbers don't lie. 💛\" *resumes reading*",
	},
	"sudden_death_lose": {
		"\"The math was on my side. 💛\" *turns a page*",
	},
}

func unoBotName() string {
	name := os.Getenv("BOT_DISPLAY_NAME")
	if name == "" {
		return "GogoBee"
	}
	return name
}

func pickCommentary(key string) string {
	lines := unoCommentary[key]
	if len(lines) == 0 {
		return ""
	}
	line := lines[rand.IntN(len(lines))]
	return strings.ReplaceAll(line, "GogoBee", unoBotName())
}

// ---------------------------------------------------------------------------
// UNO Plugin
// ---------------------------------------------------------------------------

type UnoPlugin struct {
	Base
	euro *EuroPlugin

	mu    sync.Mutex
	games map[id.UserID]*unoGame // solo: one game per player

	// reverse lookup: DM room -> player (solo)
	dmToPlayer map[id.RoomID]id.UserID

	// Multiplayer
	lobbies    map[id.RoomID]*unoMultiLobby // one lobby per room
	multiGames map[string]*unoMultiGame     // game ID -> active game
	dmToMulti  map[id.RoomID]string         // DM room -> game ID
}

func NewUnoPlugin(client *mautrix.Client, euro *EuroPlugin) *UnoPlugin {
	return &UnoPlugin{
		Base:       NewBase(client),
		euro:       euro,
		games:      make(map[id.UserID]*unoGame),
		dmToPlayer: make(map[id.RoomID]id.UserID),
		lobbies:    make(map[id.RoomID]*unoMultiLobby),
		multiGames: make(map[string]*unoMultiGame),
		dmToMulti:  make(map[id.RoomID]string),
	}
}

func (p *UnoPlugin) Name() string { return "uno" }

func (p *UnoPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "uno", Description: "Solo or multiplayer Uno", Usage: "!uno [nomercy [7-0]] €amount | !uno start [nomercy [7-0]] €amount | !uno join | !uno go", Category: "Games"},
		{Name: "uno_pot", Description: "Show the community pot balance", Usage: "!uno_pot", Category: "Games"},
	}
}

func (p *UnoPlugin) Init() error { return nil }

func (p *UnoPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *UnoPlugin) OnMessage(ctx MessageContext) error {
	// Room commands
	if p.IsCommand(ctx.Body, "uno_pot") {
		return p.handlePotCheck(ctx)
	}
	if p.IsCommand(ctx.Body, "uno") {
		args := strings.TrimSpace(p.GetArgs(ctx.Body, "uno"))
		lower := strings.ToLower(args)

		// Multiplayer subcommands
		switch {
		case strings.HasPrefix(lower, "start "):
			rest := strings.TrimSpace(args[6:])
			noMercy, sevenZero, amountStr := parseNoMercyFlags(rest)
			return p.handleMultiStart(ctx, amountStr, noMercy, sevenZero)
		case lower == "join":
			return p.handleMultiJoin(ctx)
		case lower == "go":
			return p.handleMultiGo(ctx)
		case lower == "leave":
			return p.handleMultiLeave(ctx)
		case lower == "cancel":
			return p.handleMultiCancel(ctx)
		}

		// Solo challenge: !uno [nomercy [7-0]] €amount
		noMercy, sevenZero, amountStr := parseNoMercyFlags(args)
		if isGamesRoom(ctx.RoomID) {
			return p.handleChallenge(ctx, ctx.RoomID, noMercy, sevenZero, amountStr)
		}
		// Allow starting from DM — announce to games room
		gr := gamesRoom()
		if gr == "" {
			return nil
		}
		return p.handleChallenge(ctx, id.RoomID(gr), noMercy, sevenZero, amountStr)
	}

	// DM gameplay — check solo games first, then multiplayer
	p.mu.Lock()
	playerID, isSoloDM := p.dmToPlayer[ctx.RoomID]
	if isSoloDM && playerID == ctx.Sender {
		game := p.games[playerID]
		if game != nil && !game.done {
			p.mu.Unlock()
			return p.handleDMInput(ctx, game)
		}
	}

	gameID, isMultiDM := p.dmToMulti[ctx.RoomID]
	if isMultiDM {
		mg := p.multiGames[gameID]
		if mg != nil && !mg.done {
			p.mu.Unlock()
			return p.handleMultiDMInput(ctx, mg)
		}
	}
	p.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Pot management
// ---------------------------------------------------------------------------

func (p *UnoPlugin) getPot() float64 {
	d := db.Get()
	var balance float64
	err := d.QueryRow("SELECT balance FROM uno_pot WHERE id = 1").Scan(&balance)
	if err != nil {
		d.Exec("INSERT OR IGNORE INTO uno_pot (id, balance) VALUES (1, 0)")
		return 0
	}
	return balance
}

func (p *UnoPlugin) addToPot(amount float64) {
	d := db.Get()
	d.Exec("INSERT OR IGNORE INTO uno_pot (id, balance) VALUES (1, 0)")
	d.Exec("UPDATE uno_pot SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1", amount)
}

// claimFromPot atomically claims up to amount from the pot. Returns actual payout.
func (p *UnoPlugin) claimFromPot(amount float64) float64 {
	d := db.Get()
	d.Exec("INSERT OR IGNORE INTO uno_pot (id, balance) VALUES (1, 0)")

	// Atomic: clamp to available balance, never go negative
	var payout float64
	err := d.QueryRow(
		`UPDATE uno_pot SET balance = MAX(0, balance - ?), updated_at = CURRENT_TIMESTAMP
		 WHERE id = 1 RETURNING balance + MIN(balance, ?) - balance`,
		amount, amount,
	).Scan(&payout)
	if err != nil {
		// Fallback: read-then-write (less atomic but functional)
		pot := p.getPot()
		payout = amount
		if pot < amount {
			payout = pot
		}
		if payout < 0 {
			payout = 0
		}
		d.Exec("UPDATE uno_pot SET balance = MAX(0, balance - ?), updated_at = CURRENT_TIMESTAMP WHERE id = 1", payout)
	}
	return payout
}

func (p *UnoPlugin) handlePotCheck(ctx MessageContext) error {
	pot := p.getPot()
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("🃏 Community pot: €%d. %s is waiting. 💛", int(pot), unoBotName()))
}

// ---------------------------------------------------------------------------
// Challenge
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleChallenge(ctx MessageContext, announceRoom id.RoomID, noMercy, sevenZeroRule bool, rawAmount string) error {
	reply := func(text string) error {
		return p.SendReply(ctx.RoomID, ctx.EventID, text)
	}

	amountStr := strings.TrimPrefix(strings.TrimSpace(rawAmount), "€")
	var amount float64
	fmt.Sscanf(amountStr, "%f", &amount)

	minBet := envFloat("UNO_MIN_BET", 10)
	if amount < minBet {
		return reply(fmt.Sprintf("Minimum wager is €%d. Usage: `!uno €amount`", int(minBet)))
	}

	// Hold lock for check-and-reserve to prevent TOCTOU double-challenge
	p.mu.Lock()
	if _, active := p.games[ctx.Sender]; active {
		p.mu.Unlock()
		return reply("You already have an Uno game in progress!")
	}
	// Reserve the slot with a placeholder so concurrent challenges are blocked
	p.games[ctx.Sender] = &unoGame{done: true} // placeholder
	p.mu.Unlock()

	// Debit wager
	if !p.euro.Debit(ctx.Sender, amount, "uno_wager") {
		p.mu.Lock()
		delete(p.games, ctx.Sender)
		p.mu.Unlock()
		return reply("Insufficient balance for that wager.")
	}

	// Get DM room
	dmRoom, err := p.GetDMRoom(ctx.Sender)
	if err != nil {
		p.euro.Credit(ctx.Sender, amount, "uno_wager_refund")
		p.mu.Lock()
		delete(p.games, ctx.Sender)
		p.mu.Unlock()
		return reply("Couldn't open a DM with you. Make sure you accept DMs from the bot.")
	}

	// Initialize game
	game := p.initGame(ctx.Sender, announceRoom, dmRoom, amount, noMercy, sevenZeroRule)

	p.mu.Lock()
	p.games[ctx.Sender] = game
	p.dmToPlayer[dmRoom] = ctx.Sender
	p.mu.Unlock()

	// Room announcement
	playerName := p.DisplayName(ctx.Sender)
	botName := unoBotName()
	modeTag := ""
	startComment := pickCommentary("start")
	if noMercy {
		modeTag = " 🔥 **NO MERCY**"
		startComment = pickNoMercyCommentary("nomercy_start")
		if sevenZeroRule {
			modeTag += " (7-0)"
		}
	}
	p.SendMessage(announceRoom, fmt.Sprintf(
		"🃏 **%s** has challenged %s to Uno!%s Stakes: €%d\n\n%s\n\n[Check your DMs to play.]",
		playerName, botName, modeTag, int(amount), startComment,
	))

	// Send initial hand to player (auto-draws if no playable cards)
	if err := p.playerTurnOrAutoDraw(game); err != nil {
		return err
	}
	p.resetIdleTimer(game)

	return nil
}

func (p *UnoPlugin) initGame(playerID id.UserID, roomID, dmRoom id.RoomID, wager float64, noMercy, sevenZeroRule bool) *unoGame {
	var deck []unoCard
	if noMercy {
		deck = newNoMercyDeck()
	} else {
		deck = newUnoDeck()
	}

	// Deal 7 cards each — copy into own slices to avoid shared backing array
	playerHand := make([]unoCard, 7)
	copy(playerHand, deck[:7])
	botHand := make([]unoCard, 7)
	copy(botHand, deck[7:14])
	remaining := make([]unoCard, len(deck)-14)
	copy(remaining, deck[14:])

	// Flip starting card — must be a number card (not action/wild)
	var startCard unoCard
	startIdx := -1
	for i, c := range remaining {
		if !c.Value.isAction() && !c.isWild() {
			startCard = c
			startIdx = i
			break
		}
	}

	var drawPile []unoCard
	if startIdx >= 0 {
		drawPile = make([]unoCard, 0, len(remaining)-1)
		drawPile = append(drawPile, remaining[:startIdx]...)
		drawPile = append(drawPile, remaining[startIdx+1:]...)
	} else {
		// All remaining are wilds (essentially impossible) — reshuffle whole deck
		if noMercy {
			deck = newNoMercyDeck()
		} else {
			deck = newUnoDeck()
		}
		copy(playerHand, deck[:7])
		copy(botHand, deck[7:14])
		startCard = deck[14]
		drawPile = make([]unoCard, len(deck)-15)
		copy(drawPile, deck[15:])
	}

	return &unoGame{
		playerID:      playerID,
		roomID:        roomID,
		dmRoomID:      dmRoom,
		wager:         wager,
		playerHand:    playerHand,
		botHand:       botHand,
		drawPile:      drawPile,
		discardTop:    startCard,
		topColor:      startCard.Color,
		phase:         unoPhasePlay,
		startedAt:     time.Now(),
		noMercy:       noMercy,
		sevenZeroRule: sevenZeroRule,
	}
}

// ---------------------------------------------------------------------------
// DM input handling
// ---------------------------------------------------------------------------

func (p *UnoPlugin) handleDMInput(ctx MessageContext, game *unoGame) error {
	p.mu.Lock()
	if game.done {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	p.resetIdleTimer(game)
	input := strings.TrimSpace(strings.ToLower(ctx.Body))

	// Quit at any time
	if input == "quit" || input == "forfeit" {
		return p.forfeitGame(game, false)
	}

	switch game.phase {
	case unoPhaseChooseColor:
		return p.handleColorChoice(game, input)

	case unoPhaseDrawnPlayable:
		return p.handleDrawnPlayable(game, input)

	case unoPhasePlay:
		if input == "uno" {
			game.calledUno = true
			return p.SendMessage(game.dmRoomID, "✅ UNO called!")
		}
		// No Mercy stacking: accept the stack
		if game.noMercy && game.stackMinValue > 0 && (input == "accept" || input == "a") {
			drawn := game.draw(game.stackTotal)
			game.playerHand = append(game.playerHand, drawn...)
			p.SendMessage(game.dmRoomID, fmt.Sprintf("💥 You accept the stack and draw %d cards.\n%s",
				game.stackTotal, pickNoMercyCommentary("stack_absorbed")))
			game.stackTotal = 0
			game.stackMinValue = 0
			if p.checkSoloMercyElimination(game) {
				return nil
			}
			return p.botTurn(game)
		}
		if input == "draw" {
			// No Mercy: during stacking, draw is not allowed — must stack or accept
			if game.noMercy && game.stackMinValue > 0 {
				return p.SendMessage(game.dmRoomID, "You must play a draw card to stack, or type **accept** to draw the stack.")
			}
			return p.handlePlayerDraw(game)
		}
		// Parse card number
		var cardIdx int
		if _, err := fmt.Sscanf(input, "%d", &cardIdx); err != nil || cardIdx < 1 || cardIdx > len(game.playerHand) {
			if game.noMercy && game.stackMinValue > 0 {
				return p.SendMessage(game.dmRoomID,
					fmt.Sprintf("Reply with a card number (1-%d) to stack, or **accept** to draw %d cards.", len(game.playerHand), game.stackTotal))
			}
			return p.SendMessage(game.dmRoomID,
				fmt.Sprintf("Reply with a number (1-%d) to play a card, or **draw** to draw.", len(game.playerHand)))
		}
		return p.handlePlayerPlay(game, cardIdx-1)
	}

	return nil
}

func (p *UnoPlugin) handlePlayerPlay(game *unoGame, idx int) error {
	card := game.playerHand[idx]

	// No Mercy stacking validation
	if game.noMercy && game.stackMinValue > 0 {
		if !card.canPlayOnStacking(game.topColor, game.stackMinValue) {
			return p.SendMessage(game.dmRoomID,
				fmt.Sprintf("You can't stack %s — need a draw card (matching color or wild). Type **accept** to draw %d.",
					card.Display(), game.stackTotal))
		}
	} else if !card.canPlayOn(game.discardTop, game.topColor) {
		return p.SendMessage(game.dmRoomID, fmt.Sprintf("You can't play %s on %s. Choose another card or **draw**.",
			card.Display(), game.discardTop.DisplayWithColor(game.topColor)))
	}

	// Check UNO call requirement
	if len(game.playerHand) == 2 && !game.calledUno {
		// Penalty! Draw 2
		drawn := game.draw(2)
		game.playerHand = append(game.playerHand, drawn...)
		game.calledUno = false

		p.SendMessage(game.dmRoomID, "⚠️ You forgot to call UNO! Draw 2 as penalty.\n"+pickCommentary("player_forgot_uno"))
		return p.playerTurnOrAutoDraw(game)
	}

	// Play the card
	game.playerHand = append(game.playerHand[:idx], game.playerHand[idx+1:]...)
	game.discardTop = card
	game.turns++

	if card.isWild() {
		game.pendingCard = &card
		game.phase = unoPhaseChooseColor
		return p.SendMessage(game.dmRoomID,
			"You played a **"+card.Value.String()+"**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green")
	}

	game.topColor = card.Color

	// Check player win
	if len(game.playerHand) == 0 {
		return p.playerWins(game)
	}

	// Check book state change
	bookMsg := ""
	if changed := game.updateBookState(); changed {
		if game.bookDown {
			bookMsg = "\n" + pickCommentary("book_down")
		} else {
			bookMsg = "\n" + pickCommentary("book_up")
		}
	}

	// Track UNO requirement — player must call "uno" before playing their next card
	if len(game.playerHand) == 2 {
		game.calledUno = false
	}

	p.SendMessage(game.dmRoomID, fmt.Sprintf("You played %s.%s", card.Display(), bookMsg))

	// No Mercy: Discard All — remove remaining cards of same color
	if game.noMercy && card.Value == unoDiscardAll {
		discarded := discardAllOfColor(&game.playerHand, card.Color)
		if discarded > 0 {
			p.SendMessage(game.dmRoomID, fmt.Sprintf("Discarded %d additional %s cards!", discarded, card.Color))
		}
		if len(game.playerHand) == 0 {
			return p.playerWins(game)
		}
	}

	// No Mercy: 7-0 rule
	if game.noMercy && game.sevenZeroRule {
		if card.Value == unoSeven || card.Value == unoZero {
			swapHandsSolo(game)
			p.SendMessage(game.dmRoomID, fmt.Sprintf("Hands swapped! You now have %d cards, %s has %d.",
				len(game.playerHand), unoBotName(), len(game.botHand)))
			if p.checkSoloMercyElimination(game) {
				return nil
			}
		}
	}

	// Apply action card effects
	if game.noMercy && isDrawCard(card.Value) {
		// No Mercy stacking: bot gets a chance to stack
		return p.soloPlayerPlaysDrawCard(game, card)
	}

	if card.Value == unoDrawTwo {
		drawn := game.draw(2)
		game.botHand = append(game.botHand, drawn...)
		p.SendMessage(game.dmRoomID, unoBotName()+" draws 2 cards and loses their turn.")
		return p.afterBotTurn(game)
	}

	if card.Value == unoSkip || card.Value == unoReverse {
		p.SendMessage(game.dmRoomID, unoBotName()+"'s turn is skipped!")
		return p.afterBotTurn(game)
	}

	// No Mercy: Skip Everyone (same as skip in 2-player)
	if game.noMercy && card.Value == unoSkipEveryone {
		p.SendMessage(game.dmRoomID, unoBotName()+"'s turn is skipped!")
		return p.afterBotTurn(game)
	}

	// Bot's turn
	return p.botTurn(game)
}

func (p *UnoPlugin) handleColorChoice(game *unoGame, input string) error {
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
		return p.SendMessage(game.dmRoomID, "Choose a color: **1** (Red), **2** (Blue), **3** (Yellow), **4** (Green)")
	}

	game.topColor = color
	game.phase = unoPhasePlay

	pendingCard := game.pendingCard
	game.pendingCard = nil

	// Check player win
	if len(game.playerHand) == 0 {
		p.SendMessage(game.dmRoomID, fmt.Sprintf("Color set to %s %s.", color.Emoji(), color))
		return p.playerWins(game)
	}

	// Check book state
	bookMsg := ""
	if changed := game.updateBookState(); changed {
		if game.bookDown {
			bookMsg = "\n" + pickCommentary("book_down")
		} else {
			bookMsg = "\n" + pickCommentary("book_up")
		}
	}

	p.SendMessage(game.dmRoomID, fmt.Sprintf("Color set to %s %s.%s", color.Emoji(), color, bookMsg))

	// No Mercy wild effects
	if game.noMercy && pendingCard != nil {
		switch pendingCard.Value {
		case unoWildReverseDraw4:
			// Reverse (acts as skip in 2-player) + stacking with +4
			return p.soloPlayerPlaysDrawCard(game, *pendingCard)
		case unoWildDrawSix:
			return p.soloPlayerPlaysDrawCard(game, *pendingCard)
		case unoWildDrawTen:
			return p.soloPlayerPlaysDrawCard(game, *pendingCard)
		case unoWildColorRoulette:
			// Next player (bot) flips until chosen color
			bn := unoBotName()
			flipped := p.executeColorRouletteSolo(game, false, color)
			p.SendMessage(game.dmRoomID, fmt.Sprintf("🎰 Color Roulette! %s flips %d cards until finding %s %s.\n%s",
				bn, len(flipped), color.Emoji(), color, pickNoMercyCommentary("color_roulette")))
			if p.checkSoloMercyElimination(game) {
				return nil
			}
			return p.afterBotTurn(game)
		}
	}

	// Wild Draw Four — bot draws 4 and loses turn (classic mode)
	if pendingCard != nil && pendingCard.Value == unoWildDrawFour {
		drawn := game.draw(4)
		game.botHand = append(game.botHand, drawn...)
		p.SendMessage(game.dmRoomID, unoBotName()+" draws 4 cards and loses their turn.")
		// Bot's turn was skipped, so it's player's turn again
		return p.afterBotTurn(game)
	}

	// Track UNO requirement
	if len(game.playerHand) == 2 {
		game.calledUno = false
	}

	// Regular wild — bot's turn
	return p.botTurn(game)
}

func (p *UnoPlugin) handlePlayerDraw(game *unoGame) error {
	// No Mercy: draw until playable
	if game.noMercy {
		return p.handlePlayerDrawNoMercy(game)
	}

	drawn := game.draw(1)
	if len(drawn) == 0 {
		p.SendMessage(game.dmRoomID, "No cards left to draw! Turn passes to "+unoBotName()+".")
		return p.botTurn(game)
	}

	card := drawn[0]
	game.playerHand = append(game.playerHand, card)

	if card.canPlayOn(game.discardTop, game.topColor) {
		game.drawnCard = &card
		game.phase = unoPhaseDrawnPlayable
		return p.SendMessage(game.dmRoomID,
			fmt.Sprintf("You drew: %s\nIt's playable! Play it? (**yes** / **no**)", card.Display()))
	}

	p.SendMessage(game.dmRoomID, fmt.Sprintf("You drew: %s\nNot playable. Turn passes to "+unoBotName()+".", card.Display()))
	return p.botTurn(game)
}

func (p *UnoPlugin) handlePlayerDrawNoMercy(game *unoGame) error {
	var allDrawn []unoCard
	var playableCard *unoCard

	for {
		drawn := game.draw(1)
		if len(drawn) == 0 {
			break
		}
		card := drawn[0]
		game.playerHand = append(game.playerHand, card)
		allDrawn = append(allDrawn, card)

		if p.checkSoloMercyElimination(game) {
			return nil
		}

		if card.canPlayOn(game.discardTop, game.topColor) {
			playableCard = &card
			break
		}
	}

	if len(allDrawn) == 0 {
		p.SendMessage(game.dmRoomID, "No cards left to draw! Turn passes to "+unoBotName()+".")
		return p.botTurn(game)
	}

	if playableCard != nil {
		game.drawnCard = playableCard
		game.phase = unoPhaseDrawnPlayable
		return p.SendMessage(game.dmRoomID,
			fmt.Sprintf("Drew %d card(s): %s\nLast card is playable! Play it? (**yes** / **no**)",
				len(allDrawn), formatDrawnCards(allDrawn)))
	}

	p.SendMessage(game.dmRoomID,
		fmt.Sprintf("Drew %d card(s): %s\nNo playable card found. Turn passes to %s.",
			len(allDrawn), formatDrawnCards(allDrawn), unoBotName()))
	return p.botTurn(game)
}

func (p *UnoPlugin) handleDrawnPlayable(game *unoGame, input string) error {
	if input != "yes" && input != "y" && input != "no" && input != "n" {
		return p.SendMessage(game.dmRoomID, "Play the drawn card? (**yes** / **no**)")
	}

	drawnCard := *game.drawnCard
	game.drawnCard = nil
	game.phase = unoPhasePlay

	if input == "no" || input == "n" {
		p.SendMessage(game.dmRoomID, "Card kept. Turn passes to "+unoBotName()+".")
		return p.botTurn(game)
	}

	// Play the drawn card — remove it from hand
	for i, c := range game.playerHand {
		if c == drawnCard {
			game.playerHand = append(game.playerHand[:i], game.playerHand[i+1:]...)
			break
		}
	}
	game.discardTop = drawnCard
	game.turns++

	if drawnCard.isWild() {
		game.pendingCard = &drawnCard
		game.phase = unoPhaseChooseColor
		return p.SendMessage(game.dmRoomID,
			"You played a **"+drawnCard.Value.String()+"**! Choose a color:\n1. 🟥 Red\n2. 🟦 Blue\n3. 🟨 Yellow\n4. 🟩 Green")
	}

	game.topColor = drawnCard.Color

	if len(game.playerHand) == 0 {
		return p.playerWins(game)
	}

	bookMsg := ""
	if changed := game.updateBookState(); changed {
		if game.bookDown {
			bookMsg = "\n" + pickCommentary("book_down")
		} else {
			bookMsg = "\n" + pickCommentary("book_up")
		}
	}
	p.SendMessage(game.dmRoomID, fmt.Sprintf("You played %s.%s", drawnCard.Display(), bookMsg))

	// No Mercy: Discard All
	if game.noMercy && drawnCard.Value == unoDiscardAll {
		discarded := discardAllOfColor(&game.playerHand, drawnCard.Color)
		if discarded > 0 {
			p.SendMessage(game.dmRoomID, fmt.Sprintf("Discarded %d additional %s cards!", discarded, drawnCard.Color))
		}
		if len(game.playerHand) == 0 {
			return p.playerWins(game)
		}
	}

	// No Mercy: 7-0 rule
	if game.noMercy && game.sevenZeroRule {
		if drawnCard.Value == unoSeven || drawnCard.Value == unoZero {
			swapHandsSolo(game)
			p.SendMessage(game.dmRoomID, fmt.Sprintf("Hands swapped! You now have %d cards, %s has %d.",
				len(game.playerHand), unoBotName(), len(game.botHand)))
			if p.checkSoloMercyElimination(game) {
				return nil
			}
		}
	}

	// No Mercy stacking for draw cards
	if game.noMercy && isDrawCard(drawnCard.Value) {
		return p.soloPlayerPlaysDrawCard(game, drawnCard)
	}

	if drawnCard.Value == unoDrawTwo {
		drawn := game.draw(2)
		game.botHand = append(game.botHand, drawn...)
		p.SendMessage(game.dmRoomID, unoBotName()+" draws 2 cards and loses their turn.")
		return p.afterBotTurn(game)
	}
	if drawnCard.Value == unoSkip || drawnCard.Value == unoReverse || drawnCard.Value == unoSkipEveryone {
		p.SendMessage(game.dmRoomID, unoBotName()+"'s turn is skipped!")
		return p.afterBotTurn(game)
	}

	return p.botTurn(game)
}

// ---------------------------------------------------------------------------
// Solo stacking (No Mercy)
// ---------------------------------------------------------------------------

// soloPlayerPlaysDrawCard handles when the player plays a draw card in No Mercy.
// The bot gets a chance to stack back.
func (p *UnoPlugin) soloPlayerPlaysDrawCard(game *unoGame, card unoCard) error {
	dv := cardDrawValue(card.Value)
	game.stackTotal += dv
	game.stackMinValue = dv

	// Reverse effect for Wild Reverse Draw Four
	if card.Value == unoWildReverseDraw4 {
		// In 2-player, reverse = skip, so effectively bot is the target
		// Direction doesn't matter in solo but we note the reverse
	}

	// Bot tries to stack
	return p.soloBotHandleStack(game)
}

// soloBotHandleStack has the bot try to stack or absorb.
func (p *UnoPlugin) soloBotHandleStack(game *unoGame) error {
	bn := unoBotName()

	// Bot tries to find a stackable card
	stackCard, idx := botPickStackCard(game.botHand, game.topColor, game.stackMinValue)
	if idx < 0 {
		// Bot absorbs
		drawn := game.draw(game.stackTotal)
		game.botHand = append(game.botHand, drawn...)
		p.SendMessage(game.dmRoomID, fmt.Sprintf("💥 %s can't stack! Draws %d cards. (%d cards now)\n%s",
			bn, game.stackTotal, len(game.botHand), pickNoMercyCommentary("stack_absorbed")))
		game.stackTotal = 0
		game.stackMinValue = 0
		if p.checkSoloMercyElimination(game) {
			return nil
		}
		return p.afterBotTurn(game)
	}

	// Bot stacks
	game.botHand = append(game.botHand[:idx], game.botHand[idx+1:]...)
	game.discardTop = stackCard
	dv := cardDrawValue(stackCard.Value)
	game.stackTotal += dv
	game.stackMinValue = dv
	game.turns++

	if stackCard.isWild() {
		game.topColor = botPickColor(game.botHand)
	} else {
		game.topColor = stackCard.Color
	}

	p.SendMessage(game.dmRoomID, fmt.Sprintf("🔥 %s stacks: %s! Total penalty: +%d",
		bn, stackCard.DisplayWithColor(game.topColor), game.stackTotal))

	// Player must now respond to the stack
	return p.playerTurnOrAutoDraw(game)
}

// ---------------------------------------------------------------------------
// Bot turn
// ---------------------------------------------------------------------------

func (p *UnoPlugin) botTurn(game *unoGame) error {
	if p.checkSuddenDeath(game) {
		return nil
	}

	// No Mercy stacking: if a stack is pending, bot must handle it
	if game.noMercy && game.stackMinValue > 0 {
		return p.soloBotHandleStack(game)
	}

	// Long game commentary (DM only)
	if game.turns > 0 && game.turns%30 == 0 {
		p.SendMessage(game.dmRoomID, pickCommentary("long_game"))
	}

	card, idx := p.botChooseCard(game)
	if idx < 0 {
		// Bot draws
		if game.noMercy {
			return p.botDrawNoMercy(game)
		}

		drawn := game.draw(1)
		if len(drawn) == 0 {
			p.SendMessage(game.dmRoomID, unoBotName()+" can't draw — no cards left. Turn passes.")
			game.phase = unoPhasePlay
			p.sendHandDisplay(game)
			return nil
		}
		game.botHand = append(game.botHand, drawn[0])

		if drawn[0].canPlayOn(game.discardTop, game.topColor) {
			game.botHand = game.botHand[:len(game.botHand)-1]
			return p.botPlaysCard(game, drawn[0])
		}

		commentKey := "bot_draw_normal"
		if game.bookDown {
			commentKey = "bot_draw_bookdown"
		}
		p.SendMessage(game.dmRoomID, pickCommentary(commentKey))
		return p.playerTurnOrAutoDraw(game)
	}

	// Play the card
	game.botHand = append(game.botHand[:idx], game.botHand[idx+1:]...)
	return p.botPlaysCard(game, card)
}

// botDrawNoMercy draws until playable for the bot in No Mercy mode.
func (p *UnoPlugin) botDrawNoMercy(game *unoGame) error {
	bn := unoBotName()
	var allDrawn []unoCard
	var playableCard *unoCard

	for {
		drawn := game.draw(1)
		if len(drawn) == 0 {
			break
		}
		card := drawn[0]
		game.botHand = append(game.botHand, card)
		allDrawn = append(allDrawn, card)
		if p.checkSoloMercyElimination(game) {
			return nil
		}
		if card.canPlayOn(game.discardTop, game.topColor) {
			playableCard = &card
			break
		}
	}

	if len(allDrawn) == 0 {
		p.SendMessage(game.dmRoomID, bn+" can't draw — no cards left. Turn passes.")
		game.phase = unoPhasePlay
		p.sendHandDisplay(game)
		return nil
	}

	if playableCard != nil {
		// Remove the playable card from bot's hand
		for i := len(game.botHand) - 1; i >= 0; i-- {
			if game.botHand[i] == *playableCard {
				game.botHand = append(game.botHand[:i], game.botHand[i+1:]...)
				break
			}
		}
		commentKey := "bot_draw_normal"
		if game.bookDown {
			commentKey = "bot_draw_bookdown"
		}
		p.SendMessage(game.dmRoomID, fmt.Sprintf("%s drew %d card(s). %s",
			bn, len(allDrawn), pickCommentary(commentKey)))
		return p.botPlaysCard(game, *playableCard)
	}

	commentKey := "bot_draw_normal"
	if game.bookDown {
		commentKey = "bot_draw_bookdown"
	}
	p.SendMessage(game.dmRoomID, fmt.Sprintf("%s drew %d card(s). No playable card. %s",
		bn, len(allDrawn), pickCommentary(commentKey)))
	return p.playerTurnOrAutoDraw(game)
}

func (p *UnoPlugin) botPlaysCard(game *unoGame, card unoCard) error {
	game.discardTop = card
	game.turns++

	// Choose color for wilds
	if card.isWild() {
		game.topColor = p.botChooseColor(game)
	} else {
		game.topColor = card.Color
	}

	displayCard := card.DisplayWithColor(game.topColor)
	bn := unoBotName()

	// Build a single DM message (all commentary stays in DM)
	var dm strings.Builder

	switch card.Value {
	case unoDrawTwo:
		if game.noMercy {
			// No Mercy: stacking
			dm.WriteString(fmt.Sprintf("%s plays: %s — stack incoming! (+%d total)",
				bn, displayCard, cardDrawValue(card.Value)))
			p.SendMessage(game.dmRoomID, dm.String())
			game.stackTotal = cardDrawValue(card.Value)
			game.stackMinValue = cardDrawValue(card.Value)
			return p.playerTurnOrAutoDraw(game)
		}
		commentKey := "bot_draw_two"
		if game.bookDown {
			commentKey = "bot_draw_two_bookdown"
		}
		dm.WriteString(fmt.Sprintf("%s plays: %s\n%s\nYou draw 2 cards and lose your turn.",
			bn, displayCard, pickCommentary(commentKey)))
		drawn := game.draw(2)
		game.playerHand = append(game.playerHand, drawn...)

	case unoDrawFour:
		// Only exists in No Mercy — colored draw four
		dm.WriteString(fmt.Sprintf("%s plays: %s — stack incoming! (+%d total)",
			bn, displayCard, cardDrawValue(card.Value)))
		p.SendMessage(game.dmRoomID, dm.String())
		game.stackTotal = cardDrawValue(card.Value)
		game.stackMinValue = cardDrawValue(card.Value)
		return p.playerTurnOrAutoDraw(game)

	case unoWildDrawFour:
		commentKey := "bot_wild_draw_four"
		if game.bookDown {
			commentKey = "bot_wild_draw_four_bookdown"
		}
		dm.WriteString(fmt.Sprintf("%s plays: %s (chose %s %s)\n%s\nYou draw 4 cards and lose your turn.",
			bn, card.Display(), game.topColor.Emoji(), game.topColor, pickCommentary(commentKey)))
		drawn := game.draw(4)
		game.playerHand = append(game.playerHand, drawn...)

	case unoWildReverseDraw4:
		dm.WriteString(fmt.Sprintf("%s plays: %s (chose %s %s) — stack incoming! (+%d total)",
			bn, card.Display(), game.topColor.Emoji(), game.topColor, cardDrawValue(card.Value)))
		p.SendMessage(game.dmRoomID, dm.String())
		game.stackTotal = cardDrawValue(card.Value)
		game.stackMinValue = cardDrawValue(card.Value)
		return p.playerTurnOrAutoDraw(game)

	case unoWildDrawSix:
		dm.WriteString(fmt.Sprintf("%s plays: %s (chose %s %s) — stack incoming! (+%d total)",
			bn, card.Display(), game.topColor.Emoji(), game.topColor, cardDrawValue(card.Value)))
		p.SendMessage(game.dmRoomID, dm.String())
		game.stackTotal = cardDrawValue(card.Value)
		game.stackMinValue = cardDrawValue(card.Value)
		return p.playerTurnOrAutoDraw(game)

	case unoWildDrawTen:
		dm.WriteString(fmt.Sprintf("%s plays: %s (chose %s %s) — stack incoming! (+%d total)",
			bn, card.Display(), game.topColor.Emoji(), game.topColor, cardDrawValue(card.Value)))
		p.SendMessage(game.dmRoomID, dm.String())
		game.stackTotal = cardDrawValue(card.Value)
		game.stackMinValue = cardDrawValue(card.Value)
		return p.playerTurnOrAutoDraw(game)

	case unoWildColorRoulette:
		flipped := p.executeColorRouletteSolo(game, true, game.topColor)
		dm.WriteString(fmt.Sprintf("%s plays: %s (chose %s %s)\n🎰 Color Roulette! You flip %d cards.\n%s",
			bn, card.Display(), game.topColor.Emoji(), game.topColor,
			len(flipped), pickNoMercyCommentary("color_roulette")))
		p.SendMessage(game.dmRoomID, dm.String())
		if p.checkSoloMercyElimination(game) {
			return nil
		}
		// Player was the victim — bot goes again (skip effect)
		return p.botTurn(game)

	case unoSkip, unoReverse:
		commentKey := "bot_play_normal"
		if game.bookDown {
			commentKey = "bot_play_bookdown"
		}
		dm.WriteString(fmt.Sprintf(pickCommentary(commentKey), displayCard))
		dm.WriteString("\nYour turn is skipped!")

	case unoSkipEveryone:
		commentKey := "bot_play_normal"
		if game.bookDown {
			commentKey = "bot_play_bookdown"
		}
		dm.WriteString(fmt.Sprintf(pickCommentary(commentKey), displayCard))
		dm.WriteString("\nYour turn is skipped!")

	case unoDiscardAll:
		discarded := discardAllOfColor(&game.botHand, card.Color)
		commentKey := "bot_play_normal"
		if game.bookDown {
			commentKey = "bot_play_bookdown"
		}
		dm.WriteString(fmt.Sprintf(pickCommentary(commentKey), displayCard))
		if discarded > 0 {
			dm.WriteString(fmt.Sprintf("\n%s discards %d additional %s cards!", bn, discarded, card.Color))
		}

	default:
		commentKey := "bot_play_normal"
		if game.bookDown {
			commentKey = "bot_play_bookdown"
		}
		dm.WriteString(fmt.Sprintf(pickCommentary(commentKey), displayCard))
	}

	// No Mercy: 7-0 rule effects from bot
	if game.noMercy && game.sevenZeroRule {
		if card.Value == unoSeven || card.Value == unoZero {
			swapHandsSolo(game)
			dm.WriteString(fmt.Sprintf("\n%s\nHands swapped! You now have %d cards.",
				pickNoMercyCommentary("hand_swap"), len(game.playerHand)))
			if p.checkSoloMercyElimination(game) {
				p.SendMessage(game.dmRoomID, dm.String())
				return nil
			}
		}
	}

	// Check bot win (after discard all, etc.)
	if len(game.botHand) == 0 {
		p.SendMessage(game.dmRoomID, dm.String())
		return p.botWins(game)
	}

	// Bot UNO call
	if len(game.botHand) == 1 {
		dm.WriteString("\n\n" + pickCommentary("bot_uno"))
	}

	// Book state
	if changed := game.updateBookState(); changed {
		if game.bookDown {
			dm.WriteString("\n\n" + pickCommentary("book_down"))
		} else {
			dm.WriteString("\n\n" + pickCommentary("book_up"))
		}
	}

	// Skip/Reverse — bot goes again in 2-player
	if card.Value == unoSkip || card.Value == unoReverse ||
		card.Value == unoSkipEveryone {
		p.SendMessage(game.dmRoomID, dm.String())
		return p.botTurn(game)
	}

	p.SendMessage(game.dmRoomID, dm.String())

	// Mercy check after player received cards (color roulette victim, etc.)
	if game.noMercy {
		if p.checkSoloMercyElimination(game) {
			return nil
		}
	}

	return p.playerTurnOrAutoDraw(game)
}

// ---------------------------------------------------------------------------
// Bot AI (shared between solo and multiplayer)
// ---------------------------------------------------------------------------

// botPickCard selects the best card to play from the given hand.
// opponentMinCards is the smallest hand size among opponents.
func botPickCard(hand []unoCard, discardTop unoCard, topColor unoColor, bookDown bool, opponentMinCards int) (unoCard, int) {
	var playable []int
	for i, c := range hand {
		if c.canPlayOn(discardTop, topColor) {
			playable = append(playable, i)
		}
	}

	if len(playable) == 0 {
		return unoCard{}, -1
	}

	if bookDown {
		return botPickAggressive(hand, playable)
	}
	return botPickNormal(hand, topColor, playable, opponentMinCards)
}

func botPickNormal(hand []unoCard, topColor unoColor, playable []int, opponentMinCards int) (unoCard, int) {
	var actions, numbers, wd4s []int

	for _, i := range playable {
		c := hand[i]
		switch {
		case c.Value == unoWildDrawFour:
			wd4s = append(wd4s, i)
		case c.Value.isAction():
			actions = append(actions, i)
		default:
			numbers = append(numbers, i)
		}
	}

	// Sometimes play WD4 even when we have other options (20% chance)
	// This makes the challenge mechanic meaningful against the bot.
	if len(wd4s) > 0 && (len(actions) > 0 || len(numbers) > 0) && rand.IntN(5) == 0 {
		idx := wd4s[0]
		return hand[idx], idx
	}

	// Save WD4 unless opponent is close to winning
	if opponentMinCards > 3 {
		if len(actions) > 0 {
			for _, i := range actions {
				if hand[i].Color == topColor {
					return hand[i], i
				}
			}
			idx := actions[0]
			return hand[idx], idx
		}
		if len(numbers) > 0 {
			for _, i := range numbers {
				if hand[i].Color == topColor {
					return hand[i], i
				}
			}
			idx := numbers[0]
			return hand[idx], idx
		}
	}

	// Opponent close to winning or no other choice
	if len(wd4s) > 0 {
		idx := wd4s[0]
		return hand[idx], idx
	}
	if len(actions) > 0 {
		idx := actions[0]
		return hand[idx], idx
	}
	idx := numbers[0]
	return hand[idx], idx
}

func botPickAggressive(hand []unoCard, playable []int) (unoCard, int) {
	var wd4s, actions, numbers []int

	for _, i := range playable {
		c := hand[i]
		switch {
		case c.Value == unoWildDrawFour:
			wd4s = append(wd4s, i)
		case c.Value.isAction():
			actions = append(actions, i)
		default:
			numbers = append(numbers, i)
		}
	}

	if len(wd4s) > 0 {
		idx := wd4s[0]
		return hand[idx], idx
	}
	if len(actions) > 0 {
		idx := actions[0]
		return hand[idx], idx
	}
	idx := numbers[0]
	return hand[idx], idx
}

func botPickColor(hand []unoCard) unoColor {
	counts := map[unoColor]int{}
	for _, c := range hand {
		if c.Color != unoWild {
			counts[c.Color]++
		}
	}

	best := unoRed
	bestCount := 0
	for _, color := range []unoColor{unoRed, unoBlue, unoYellow, unoGreen} {
		if counts[color] > bestCount {
			bestCount = counts[color]
			best = color
		}
	}
	return best
}

// Solo wrappers for backward compatibility
func (p *UnoPlugin) botChooseCard(game *unoGame) (unoCard, int) {
	if game.noMercy {
		return botPickCardNoMercy(game.botHand, game.discardTop, game.topColor, game.bookDown, len(game.playerHand), game.stackMinValue)
	}
	return botPickCard(game.botHand, game.discardTop, game.topColor, game.bookDown, len(game.playerHand))
}

func (p *UnoPlugin) botChooseColor(game *unoGame) unoColor {
	return botPickColor(game.botHand)
}

// ---------------------------------------------------------------------------
// Sudden Death (long-game point scoring for 2-player matches)
// ---------------------------------------------------------------------------

const (
	suddenDeathAnnounce  = 80 // turn to announce sudden death
	suddenDeathCountdown = 20 // turns after announcement
)

// checkSuddenDeath checks whether sudden death should be announced or resolved.
// Returns true if the game ended.
func (p *UnoPlugin) checkSuddenDeath(game *unoGame) bool {
	if game.done {
		return true
	}

	// Resolve: game ends at the deadline
	if game.suddenDeath && game.turns >= game.suddenDeathTurn {
		p.suddenDeathWinner(game)
		return true
	}

	// Announce
	if !game.suddenDeath && game.turns >= suddenDeathAnnounce {
		game.suddenDeath = true
		game.suddenDeathTurn = game.turns + suddenDeathCountdown
		remaining := game.suddenDeathTurn - game.turns

		ann := fmt.Sprintf("⏰ **SUDDEN DEATH!** This match ends in %d turns — lowest hand value wins!\n\n%s",
			remaining, pickCommentary("sudden_death"))
		p.SendMessage(game.dmRoomID, ann)
		p.SendMessage(game.roomID, fmt.Sprintf("⏰ **Sudden Death activated!** %s's match ends in %d turns.",
			p.DisplayName(game.playerID), remaining))
		return false
	}

	// Countdown reminders (DM only)
	if game.suddenDeath {
		remaining := game.suddenDeathTurn - game.turns
		switch remaining {
		case 10:
			p.SendMessage(game.dmRoomID, "⏰ **10 turns remaining!**")
		case 5:
			p.SendMessage(game.dmRoomID, "⏰ **5 turns remaining!**")
		}
	}

	return false
}

func (p *UnoPlugin) suddenDeathWinner(game *unoGame) {
	p.mu.Lock()
	if game.done {
		p.mu.Unlock()
		return
	}
	game.done = true
	p.mu.Unlock()

	playerScore := scoreHand(game.playerHand)
	botScore := scoreHand(game.botHand)
	bn := unoBotName()
	playerName := p.DisplayName(game.playerID)

	// Build score breakdown
	breakdown := fmt.Sprintf("**Final Scores:**\n%s: %s\n%s: %s",
		playerName, formatHandScore(game.playerHand),
		bn, formatHandScore(game.botHand))

	var playerWins bool
	switch {
	case playerScore < botScore:
		playerWins = true
	case botScore < playerScore:
		playerWins = false
	case len(game.playerHand) < len(game.botHand):
		// Tiebreaker: fewer cards
		playerWins = true
		breakdown += "\n*Tiebreaker: fewer cards!*"
	case len(game.botHand) < len(game.playerHand):
		playerWins = false
		breakdown += "\n*Tiebreaker: fewer cards!*"
	default:
		// Final tiebreaker: player wins (bot had the advantage of going second in the countdown)
		playerWins = true
		breakdown += "\n*Tiebreaker: dead even — player takes it!*"
	}

	p.SendMessage(game.dmRoomID, fmt.Sprintf("⏰ **TIME'S UP — SUDDEN DEATH!**\n\n%s", breakdown))

	potBefore := p.getPot()

	if playerWins {
		payout := p.claimFromPot(game.wager)
		totalPayout := game.wager + payout
		p.euro.Credit(game.playerID, totalPayout, "uno_win")

		newPot := p.getPot()
		p.SendMessage(game.dmRoomID, "🎉 **You win on points!**")
		p.SendMessage(game.roomID, fmt.Sprintf(
			"⏰ **Sudden Death!** %s wins on points!\n%s\n€%d claimed from the community pot. (Pot: €%d)\n\n%s",
			playerName, breakdown, int(payout), int(newPot), pickCommentary("sudden_death_win")))

		p.recordGame(game, "sudden_death_player", potBefore)
	} else {
		p.addToPot(game.wager)
		newPot := p.getPot()

		p.SendMessage(game.dmRoomID, fmt.Sprintf("💀 **%s wins on points.** Better luck next time.", bn))
		p.SendMessage(game.roomID, fmt.Sprintf(
			"⏰ **Sudden Death!** %s wins on points!\n%s\n%s's €%d added to the community pot. (Pot: €%d)\n\n%s",
			bn, breakdown, playerName, int(game.wager), int(newPot), pickCommentary("sudden_death_lose")))

		recordBotDefeat(game.playerID, "uno")
		p.recordGame(game, "sudden_death_bot", potBefore)
	}

	p.cleanupGame(game)
}

// afterBotTurn is called when the bot's turn was skipped (player played skip/reverse/draw two).
// Shows the hand display for the player's next turn.
func (p *UnoPlugin) afterBotTurn(game *unoGame) error {
	if p.checkSuddenDeath(game) {
		return nil
	}
	return p.playerTurnOrAutoDraw(game)
}

// ---------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------

func (p *UnoPlugin) appendHandDisplay(game *unoGame, sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf("🃏 **Your turn!**\nDiscard pile: %s\n\n**Your hand:**\n",
		game.discardTop.DisplayWithColor(game.topColor)))

	for i, c := range game.playerHand {
		playable := ""
		if c.canPlayOn(game.discardTop, game.topColor) {
			playable = " ✅"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, c.Display(), playable))
	}

	sb.WriteString(fmt.Sprintf("\n%s has %d cards.", unoBotName(), len(game.botHand)))
	if game.noMercy && len(game.playerHand) >= 20 {
		sb.WriteString(fmt.Sprintf("\n⚠️ **You have %d cards! (25 = eliminated)**", len(game.playerHand)))
	}
	sb.WriteString("\n\nReply with a card number to play, or **draw** to draw.")
}

func (p *UnoPlugin) sendHandDisplay(game *unoGame) {
	var sb strings.Builder
	p.appendHandDisplay(game, &sb)
	p.SendMessage(game.dmRoomID, sb.String())
}

// sendHandDisplayStacking shows hand during stacking — only stackable cards are playable.
func (p *UnoPlugin) sendHandDisplayStacking(game *unoGame) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ **Stack incoming: +%d!**\nDiscard pile: %s\n\n**Your hand:**\n",
		game.stackTotal, game.discardTop.DisplayWithColor(game.topColor)))

	for i, c := range game.playerHand {
		marker := ""
		if c.canPlayOnStacking(game.topColor, game.stackMinValue) {
			marker = " ✅"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, c.Display(), marker))
	}

	sb.WriteString(fmt.Sprintf("\n%s has %d cards.", unoBotName(), len(game.botHand)))
	sb.WriteString("\n\nPlay a draw card to stack, or type **accept** to draw the stack.")

	p.SendMessage(game.dmRoomID, sb.String())
}

// playerTurnOrAutoDraw shows the hand if the player has playable cards,
// otherwise auto-draws and passes to the bot.
func (p *UnoPlugin) playerTurnOrAutoDraw(game *unoGame) error {
	if p.checkSuddenDeath(game) {
		return nil
	}

	// No Mercy stacking: if a stack is pending, show stack prompt
	if game.noMercy && game.stackMinValue > 0 {
		if hasStackableCard(game.playerHand, game.topColor, game.stackMinValue) {
			game.phase = unoPhasePlay
			p.sendHandDisplayStacking(game)
			return nil
		}
		// Player must absorb the stack
		drawn := game.draw(game.stackTotal)
		game.playerHand = append(game.playerHand, drawn...)
		p.SendMessage(game.dmRoomID, fmt.Sprintf("💥 No stackable card! You draw %d cards.\n%s",
			game.stackTotal, pickNoMercyCommentary("stack_absorbed")))
		game.stackTotal = 0
		game.stackMinValue = 0
		if p.checkSoloMercyElimination(game) {
			return nil
		}
		// After absorbing, bot's turn (player was skipped)
		return p.botTurn(game)
	}

	if game.hasPlayable() {
		game.phase = unoPhasePlay
		p.sendHandDisplay(game)
		return nil
	}

	// No Mercy: draw until playable
	if game.noMercy {
		var allDrawn []unoCard
		var playableCard *unoCard
		for {
			cards := game.draw(1)
			if len(cards) == 0 {
				break
			}
			card := cards[0]
			game.playerHand = append(game.playerHand, card)
			allDrawn = append(allDrawn, card)
			if p.checkSoloMercyElimination(game) {
				return nil
			}
			if card.canPlayOn(game.discardTop, game.topColor) {
				playableCard = &card
				break
			}
		}
		if len(allDrawn) == 0 {
			p.SendMessage(game.dmRoomID, "No playable cards and deck is empty.")
			game.phase = unoPhasePlay
			p.sendHandDisplay(game)
			return nil
		}
		if playableCard != nil {
			game.drawnCard = playableCard
			game.phase = unoPhaseDrawnPlayable
			return p.SendMessage(game.dmRoomID,
				fmt.Sprintf("No playable cards — drew %d card(s): %s\nLast card is playable! Play it? (**yes** / **no**)",
					len(allDrawn), formatDrawnCards(allDrawn)))
		}
		p.SendMessage(game.dmRoomID,
			fmt.Sprintf("No playable cards — drew %d card(s). None playable. Turn passes to %s.",
				len(allDrawn), unoBotName()))
		return p.botTurn(game)
	}

	// Classic: draw one
	drawn := game.draw(1)
	if len(drawn) == 0 {
		p.SendMessage(game.dmRoomID, "No playable cards and deck is empty.")
		game.phase = unoPhasePlay
		p.sendHandDisplay(game)
		return nil
	}

	card := drawn[0]
	game.playerHand = append(game.playerHand, card)

	if card.canPlayOn(game.discardTop, game.topColor) {
		game.drawnCard = &card
		game.phase = unoPhaseDrawnPlayable
		return p.SendMessage(game.dmRoomID,
			fmt.Sprintf("No playable cards — drew automatically: %s\nIt's playable! Play it? (**yes** / **no**)", card.Display()))
	}

	p.SendMessage(game.dmRoomID,
		fmt.Sprintf("No playable cards — drew automatically: %s\nNot playable. Turn passes to %s.", card.Display(), unoBotName()))
	return p.botTurn(game)
}

// ---------------------------------------------------------------------------
// Win/Loss
// ---------------------------------------------------------------------------

func (p *UnoPlugin) playerWins(game *unoGame) error {
	p.mu.Lock()
	if game.done {
		p.mu.Unlock()
		return nil
	}
	game.done = true
	p.mu.Unlock()

	potBefore := p.getPot()
	payout := p.claimFromPot(game.wager)
	totalPayout := game.wager + payout
	p.euro.Credit(game.playerID, totalPayout, "uno_win")

	newPot := p.getPot()
	playerName := p.DisplayName(game.playerID)

	p.SendMessage(game.dmRoomID, "🎉 **Uno out! You win!**")

	if newPot <= 0 {
		p.SendMessage(game.roomID, fmt.Sprintf(
			"🎉 **%s** has defeated "+unoBotName()+"! €%d claimed from the community pot.\n(Community pot remaining: €0)\n\n%s\n\n🃏 The community pot has been reset. Current pot: €0. For now.",
			playerName, int(payout), pickCommentary("bot_lose_empty_pot")))
	} else {
		p.SendMessage(game.roomID, fmt.Sprintf(
			"🎉 **%s** has defeated "+unoBotName()+"! €%d claimed from the community pot.\n(Community pot remaining: €%d)\n\n%s",
			playerName, int(payout), int(newPot), pickCommentary("bot_lose")))
	}

	p.recordGame(game, "player_win", potBefore)
	p.cleanupGame(game)
	return nil
}

func (p *UnoPlugin) botWins(game *unoGame) error {
	p.mu.Lock()
	if game.done {
		p.mu.Unlock()
		return nil
	}
	game.done = true
	p.mu.Unlock()

	potBefore := p.getPot()
	p.addToPot(game.wager)
	newPot := p.getPot()
	playerName := p.DisplayName(game.playerID)

	p.SendMessage(game.dmRoomID, "💀 **"+unoBotName()+" wins.** Better luck next time.")
	p.SendMessage(game.roomID, fmt.Sprintf(
		"💀 "+unoBotName()+" wins. **%s**'s €%d has been added to the community pot.\n(Community pot: €%d)\n\n%s",
		playerName, int(game.wager), int(newPot), pickCommentary("bot_win")))

	recordBotDefeat(game.playerID, "uno")
	p.recordGame(game, "gogobee_win", potBefore)
	p.cleanupGame(game)
	return nil
}

func (p *UnoPlugin) forfeitGame(game *unoGame, timeout bool) error {
	p.mu.Lock()
	if game.done {
		p.mu.Unlock()
		return nil
	}
	game.done = true
	p.mu.Unlock()

	potBefore := p.getPot()
	p.addToPot(game.wager)
	playerName := p.DisplayName(game.playerID)

	if timeout {
		p.SendMessage(game.dmRoomID,
			fmt.Sprintf("Game forfeited. Your €%d has been added to the community pot.", int(game.wager)))
		p.SendMessage(game.roomID,
			fmt.Sprintf("🚪 **%s** has forfeited. €%d added to the pot. 💛", playerName, int(game.wager)))
	} else {
		p.SendMessage(game.dmRoomID,
			fmt.Sprintf("You quit. Your €%d has been added to the community pot.", int(game.wager)))
		p.SendMessage(game.roomID,
			fmt.Sprintf("🚪 **%s** has quit. €%d added to the pot. 💛", playerName, int(game.wager)))
	}

	recordBotDefeat(game.playerID, "uno")
	p.recordGame(game, "gogobee_win", potBefore)
	p.cleanupGame(game)
	return nil
}

// ---------------------------------------------------------------------------
// Idle timers
// ---------------------------------------------------------------------------

func (p *UnoPlugin) resetIdleTimer(game *unoGame) {
	p.mu.Lock()
	if game.idleTimer != nil {
		game.idleTimer.Stop()
	}
	if game.warningTimer != nil {
		game.warningTimer.Stop()
	}
	playerID := game.playerID
	p.mu.Unlock()

	game.idleTimer = time.AfterFunc(5*time.Minute, func() {
		p.mu.Lock()
		g := p.games[playerID]
		if g == nil || g.done {
			p.mu.Unlock()
			return
		}
		dmRoom := g.dmRoomID
		p.mu.Unlock()

		p.SendMessage(dmRoom,
			"Still there? Reply with your move or type **quit** to forfeit.\n(You have 2 minutes before the game is forfeited.)")

		p.mu.Lock()
		if g.done {
			p.mu.Unlock()
			return
		}
		g.warningTimer = time.AfterFunc(2*time.Minute, func() {
			p.mu.Lock()
			g2 := p.games[playerID]
			if g2 == nil || g2.done {
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
			p.forfeitGame(g2, true)
		})
		p.mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// Cleanup and persistence
// ---------------------------------------------------------------------------

func (p *UnoPlugin) cleanupGame(game *unoGame) {
	p.mu.Lock()
	if game.idleTimer != nil {
		game.idleTimer.Stop()
	}
	if game.warningTimer != nil {
		game.warningTimer.Stop()
	}
	delete(p.games, game.playerID)
	delete(p.dmToPlayer, game.dmRoomID)
	p.mu.Unlock()
}

func (p *UnoPlugin) recordGame(game *unoGame, result string, potBefore float64) {
	db.Exec("uno: record game",
		`INSERT INTO uno_games (player_id, wager, result, pot_before, pot_after, turns, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		string(game.playerID), game.wager, result, potBefore, p.getPot(),
		game.turns, game.startedAt.UTC().Format("2006-01-02 15:04:05"),
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
}


