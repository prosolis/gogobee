package plugin

import (
	"context"
	"fmt"
	"math/rand/v2"
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

type suit int

const (
	spades suit = iota
	hearts
	diamonds
	clubs
)

var suitSymbols = [4]string{"♠", "♥", "♦", "♣"}

type card struct {
	Rank int // 1=Ace, 2-10, 11=J, 12=Q, 13=K
	Suit suit
}

func (c card) String() string {
	ranks := [14]string{"", "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	return ranks[c.Rank] + suitSymbols[c.Suit]
}

func (c card) value() int {
	if c.Rank >= 10 {
		return 10
	}
	return c.Rank // Ace = 1, handled in handValue
}

type deck struct {
	cards []card
}

func newDeck() *deck {
	d := &deck{cards: make([]card, 0, 52)}
	for s := spades; s <= clubs; s++ {
		for r := 1; r <= 13; r++ {
			d.cards = append(d.cards, card{r, s})
		}
	}
	// Shuffle
	rand.Shuffle(len(d.cards), func(i, j int) {
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	})
	return d
}

func (d *deck) draw() card {
	if len(d.cards) == 0 {
		// Reshuffle a fresh deck if exhausted (extremely rare)
		*d = *newDeck()
	}
	c := d.cards[0]
	d.cards = d.cards[1:]
	return c
}

func handValue(cards []card) (int, bool) {
	total := 0
	aces := 0
	for _, c := range cards {
		if c.Rank == 1 {
			aces++
			total += 11
		} else {
			total += c.value()
		}
	}
	soft := aces > 0 && total <= 21
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	if aces == 0 {
		soft = false
	}
	return total, soft
}

func handStr(cards []card) string {
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

func isBlackjack(cards []card) bool {
	if len(cards) != 2 {
		return false
	}
	v, _ := handValue(cards)
	return v == 21
}

// ---------------------------------------------------------------------------
// Blackjack game state
// ---------------------------------------------------------------------------

type bjPlayer struct {
	UserID id.UserID
	Bet    float64
	Hand   []card
	Done   bool
	Bust   bool
}

func (p *bjPlayer) value() int {
	v, _ := handValue(p.Hand)
	return v
}

type bjTable struct {
	players     []*bjPlayer
	dealer      []card
	deck        *deck
	joinTimer   *time.Timer
	turnTimer   *time.Timer
	currentTurn int
	phase       string // "joining", "playing", "done"
	roomID      id.RoomID
}

// ---------------------------------------------------------------------------
// Blackjack config
// ---------------------------------------------------------------------------

type bjConfig struct {
	TimeoutSeconds     int
	AutoplayThreshold  int
	MinBet             float64
	MaxBet             float64
	DebtLimit          float64
}

func loadBJConfig() bjConfig {
	return bjConfig{
		TimeoutSeconds:    envInt("BLACKJACK_TIMEOUT_SECONDS", 60),
		AutoplayThreshold: envInt("BLACKJACK_AUTOPLAY_THRESHOLD", 15),
		MinBet:            envFloat("BLACKJACK_MIN_BET", 1),
		MaxBet:            envFloat("BLACKJACK_MAX_BET", 500),
		DebtLimit:         envFloat("BLACKJACK_DEBT_LIMIT", 1000),
	}
}

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

type BlackjackPlugin struct {
	Base
	euro   *EuroPlugin
	cfg    bjConfig
	mu     sync.Mutex
	tables map[id.RoomID]*bjTable
}

func NewBlackjackPlugin(client *mautrix.Client, euro *EuroPlugin) *BlackjackPlugin {
	return &BlackjackPlugin{
		Base:   NewBase(client),
		euro:   euro,
		cfg:    loadBJConfig(),
		tables: make(map[id.RoomID]*bjTable),
	}
}

func (p *BlackjackPlugin) Name() string { return "blackjack" }

func (p *BlackjackPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "blackjack", Description: "Start or join a Blackjack table", Usage: "!blackjack €amount | !blackjack leave", Category: "Games"},
		{Name: "hit", Description: "Take a card in Blackjack", Usage: "!hit", Category: "Games"},
		{Name: "stand", Description: "End your turn in Blackjack", Usage: "!stand", Category: "Games"},
		{Name: "bjboard", Description: "Blackjack leaderboard", Usage: "!bjboard", Category: "Games"},
	}
}

func (p *BlackjackPlugin) Init() error { return nil }

func (p *BlackjackPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *BlackjackPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "bjboard"):
		return p.handleBoard(ctx)
	case p.IsCommand(ctx.Body, "blackjack"):
		if !isGamesRoom(ctx.RoomID) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
		}
		return p.handleBlackjack(ctx)
	case p.IsCommand(ctx.Body, "hit"):
		if !isGamesRoom(ctx.RoomID) {
			return nil
		}
		return p.handleHit(ctx)
	case p.IsCommand(ctx.Body, "stand"):
		if !isGamesRoom(ctx.RoomID) {
			return nil
		}
		return p.handleStand(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Blackjack command handlers
// ---------------------------------------------------------------------------

func (p *BlackjackPlugin) handleBlackjack(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "blackjack"))

	if strings.EqualFold(args, "leave") {
		return p.handleLeave(ctx)
	}

	// Parse bet amount
	amountStr := strings.TrimPrefix(args, "€")
	var bet float64
	if _, err := fmt.Sscanf(amountStr, "%f", &bet); err != nil || bet <= 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Usage: `!blackjack €amount` (min €%d, max €%d)", int(p.cfg.MinBet), int(p.cfg.MaxBet)))
	}

	if bet < p.cfg.MinBet {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Minimum bet is €%d.", int(p.cfg.MinBet)))
	}
	if bet > p.cfg.MaxBet {
		bet = p.cfg.MaxBet
	}

	// Check balance
	balance := p.euro.GetBalance(ctx.Sender)
	maxAvailable := balance + p.cfg.DebtLimit
	if maxAvailable <= 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"🚫 You're at your debt limit. Earn some euros before playing.")
	}
	if bet > maxAvailable {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("You can bet up to €%d (balance: €%d).", int(maxAvailable), int(balance)))
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	table, exists := p.tables[ctx.RoomID]

	if exists && table.phase == "joining" {
		// Join existing table
		for _, pl := range table.players {
			if pl.UserID == ctx.Sender {
				return p.SendReply(ctx.RoomID, ctx.EventID, "You're already at the table!")
			}
		}
		if len(table.players) >= 2 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Table is full (max 2 players).")
		}

		if !p.euro.Debit(ctx.Sender, bet, "blackjack_bet") {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to place bet.")
		}

		table.players = append(table.players, &bjPlayer{UserID: ctx.Sender, Bet: bet})
		name := p.bjDisplayName(ctx.Sender)
		_ = p.SendMessage(ctx.RoomID,
			fmt.Sprintf("🃏 **%s** joins the table! Bet: €%d\nTable is full — dealing!", name, int(bet)))

		if table.joinTimer != nil {
			table.joinTimer.Stop()
		}
		p.startRound(ctx.RoomID, table)
		return nil
	}

	if exists && table.phase == "playing" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "A round is in progress. Wait for it to finish.")
	}

	// Create new table
	if !p.euro.Debit(ctx.Sender, bet, "blackjack_bet") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to place bet.")
	}

	table = &bjTable{
		players: []*bjPlayer{{UserID: ctx.Sender, Bet: bet}},
		deck:    newDeck(),
		phase:   "joining",
		roomID:  ctx.RoomID,
	}
	p.tables[ctx.RoomID] = table

	name := p.bjDisplayName(ctx.Sender)
	_ = p.SendMessage(ctx.RoomID,
		fmt.Sprintf("🃏 **%s** opens a Blackjack table! Bet: €%d\nJoin with `!blackjack €amount` (60 seconds to join)",
			name, int(bet)))

	// Start join timer
	table.joinTimer = time.AfterFunc(60*time.Second, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if table.phase == "joining" {
			p.startRound(ctx.RoomID, table)
		}
	})

	return nil
}

func (p *BlackjackPlugin) handleLeave(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	table, exists := p.tables[ctx.RoomID]
	if !exists || table.phase != "joining" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No table to leave, or the round has started.")
	}

	for i, pl := range table.players {
		if pl.UserID == ctx.Sender {
			// Return bet
			p.euro.Credit(ctx.Sender, pl.Bet, "blackjack_leave_refund")
			table.players = append(table.players[:i], table.players[i+1:]...)

			if len(table.players) == 0 {
				if table.joinTimer != nil {
					table.joinTimer.Stop()
				}
				delete(p.tables, ctx.RoomID)
				return p.SendMessage(ctx.RoomID, "🃏 Table closed — all players left.")
			}
			name := p.bjDisplayName(ctx.Sender)
			return p.SendMessage(ctx.RoomID,
				fmt.Sprintf("🃏 **%s** left the table. Bet refunded.", name))
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, "You're not at the table.")
}

// startRound must be called with p.mu held.
func (p *BlackjackPlugin) startRound(roomID id.RoomID, table *bjTable) {
	table.phase = "playing"

	// Deal 2 cards to each player and dealer
	for range 2 {
		for _, pl := range table.players {
			pl.Hand = append(pl.Hand, table.deck.draw())
		}
		table.dealer = append(table.dealer, table.deck.draw())
	}

	// Check for immediate blackjacks
	for _, pl := range table.players {
		if isBlackjack(pl.Hand) {
			pl.Done = true
		}
	}

	// Display initial state
	_ = p.SendMessage(roomID, p.renderTable(table, false))

	// Find first active player
	table.currentTurn = -1
	p.advanceTurn(roomID, table)
}

func (p *BlackjackPlugin) handleHit(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	table, exists := p.tables[ctx.RoomID]
	if !exists || table.phase != "playing" {
		return nil
	}

	if table.currentTurn < 0 || table.currentTurn >= len(table.players) {
		return nil
	}

	player := table.players[table.currentTurn]
	if player.UserID != ctx.Sender {
		return nil // Not your turn
	}

	if table.turnTimer != nil {
		table.turnTimer.Stop()
	}

	player.Hand = append(player.Hand, table.deck.draw())
	v := player.value()

	if v > 21 {
		player.Bust = true
		player.Done = true
		name := p.bjDisplayName(player.UserID)
		_ = p.SendMessage(ctx.RoomID,
			fmt.Sprintf("💥 **%s** busts with %s (%d)!", name, handStr(player.Hand), v))
		p.advanceTurn(ctx.RoomID, table)
		return nil
	}

	if v == 21 {
		player.Done = true
		_ = p.SendMessage(ctx.RoomID, p.renderTable(table, false))
		p.advanceTurn(ctx.RoomID, table)
		return nil
	}

	_ = p.SendMessage(ctx.RoomID, p.renderTable(table, false))
	p.startTurnTimer(ctx.RoomID, table)
	return nil
}

func (p *BlackjackPlugin) handleStand(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	table, exists := p.tables[ctx.RoomID]
	if !exists || table.phase != "playing" {
		return nil
	}

	if table.currentTurn < 0 || table.currentTurn >= len(table.players) {
		return nil
	}

	player := table.players[table.currentTurn]
	if player.UserID != ctx.Sender {
		return nil
	}

	if table.turnTimer != nil {
		table.turnTimer.Stop()
	}

	player.Done = true
	p.advanceTurn(ctx.RoomID, table)
	return nil
}

// advanceTurn moves to the next player or dealer. Must be called with p.mu held.
func (p *BlackjackPlugin) advanceTurn(roomID id.RoomID, table *bjTable) {
	// Find next active player
	for i := table.currentTurn + 1; i < len(table.players); i++ {
		if !table.players[i].Done {
			table.currentTurn = i
			name := p.bjDisplayName(table.players[i].UserID)
			_ = p.SendMessage(roomID,
				fmt.Sprintf("👉 **%s**'s turn. `!hit` or `!stand` (%ds)", name, p.cfg.TimeoutSeconds))
			p.startTurnTimer(roomID, table)
			return
		}
	}

	// All players done — dealer's turn
	p.playDealer(roomID, table)
}

func (p *BlackjackPlugin) startTurnTimer(roomID id.RoomID, table *bjTable) {
	turnIdx := table.currentTurn
	table.turnTimer = time.AfterFunc(time.Duration(p.cfg.TimeoutSeconds)*time.Second, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		// Verify table still exists and it's the same turn
		t, exists := p.tables[roomID]
		if !exists || t != table || t.currentTurn != turnIdx {
			return
		}

		// Use looked-up t, not captured table pointer
		player := t.players[turnIdx]
		v := player.value()
		name := p.bjDisplayName(player.UserID)

		if v >= p.cfg.AutoplayThreshold {
			_ = p.SendMessage(roomID,
				fmt.Sprintf("⏱️ **%s** timed out — auto-playing (stand)", name))
			player.Done = true
			p.advanceTurn(roomID, t)
		} else {
			_ = p.SendMessage(roomID,
				fmt.Sprintf("⏱️ **%s** timed out — auto-playing (hit)", name))
			player.Hand = append(player.Hand, t.deck.draw())
			v = player.value()
			if v > 21 {
				player.Bust = true
				player.Done = true
				_ = p.SendMessage(roomID,
					fmt.Sprintf("💥 **%s** busts with %s (%d)!", name, handStr(player.Hand), v))
				p.advanceTurn(roomID, t)
			} else if v >= p.cfg.AutoplayThreshold {
				player.Done = true
				p.advanceTurn(roomID, t)
			} else {
				// Still below threshold, restart timer
				p.startTurnTimer(roomID, t)
			}
		}
	})
}

// playDealer plays the dealer hand. Must be called with p.mu held.
func (p *BlackjackPlugin) playDealer(roomID id.RoomID, table *bjTable) {
	// Check if all players busted
	allBust := true
	for _, pl := range table.players {
		if !pl.Bust {
			allBust = false
			break
		}
	}

	if !allBust {
		// Dealer plays: hit on soft 17, stand on hard 17+, stand on soft 18+
		for {
			v, soft := handValue(table.dealer)
			if v < 17 || (v == 17 && soft) {
				table.dealer = append(table.dealer, table.deck.draw())
			} else {
				break
			}
		}
	}

	p.resolveRound(roomID, table)
}

func (p *BlackjackPlugin) resolveRound(roomID id.RoomID, table *bjTable) {
	table.phase = "done"
	dealerValue, _ := handValue(table.dealer)
	dealerBust := dealerValue > 21
	dealerBJ := isBlackjack(table.dealer)

	var sb strings.Builder
	sb.WriteString("🃏 **Round over!**\n\n")

	for _, pl := range table.players {
		name := p.bjDisplayName(pl.UserID)
		playerValue := pl.value()
		playerBJ := isBlackjack(pl.Hand)

		var result string
		var payout float64

		switch {
		case pl.Bust:
			result = "Bust"
			payout = 0 // Already deducted

		case playerBJ && dealerBJ:
			result = "Push (both Blackjack)"
			payout = pl.Bet // Return bet
			p.euro.Credit(pl.UserID, payout, "blackjack_push")

		case playerBJ:
			result = "Blackjack!"
			payout = pl.Bet + pl.Bet*1.5 // Return bet + 1.5x
			p.euro.Credit(pl.UserID, payout, "blackjack_win")

		case dealerBJ:
			result = "Dealer Blackjack"
			payout = 0

		case dealerBust:
			result = "Win (dealer bust)!"
			payout = pl.Bet * 2
			p.euro.Credit(pl.UserID, payout, "blackjack_win")

		case playerValue > dealerValue:
			result = fmt.Sprintf("Win! %d vs %d", playerValue, dealerValue)
			payout = pl.Bet * 2
			p.euro.Credit(pl.UserID, payout, "blackjack_win")

		case playerValue == dealerValue:
			result = "Push"
			payout = pl.Bet
			p.euro.Credit(pl.UserID, payout, "blackjack_push")

		default:
			result = fmt.Sprintf("Loss. %d vs %d", playerValue, dealerValue)
			payout = 0
		}

		net := payout - pl.Bet
		newBalance := p.euro.GetBalance(pl.UserID)
		netStr := fmt.Sprintf("€%d", int(net))
		if net > 0 {
			netStr = fmt.Sprintf("+€%d", int(net))
		}

		sb.WriteString(fmt.Sprintf("**%s**: %s  %s — %s  (balance: €%d)\n",
			name, handStr(pl.Hand), result, netStr, int(newBalance)))

		// Record score
		p.recordBJScore(pl.UserID, net)
	}

	sb.WriteString(fmt.Sprintf("\nDealer: %s  (%d)\n", handStr(table.dealer), dealerValue))

	// Stop any pending timers before cleanup
	if table.turnTimer != nil {
		table.turnTimer.Stop()
	}
	if table.joinTimer != nil {
		table.joinTimer.Stop()
	}
	delete(p.tables, roomID)
	_ = p.SendMessage(roomID, sb.String())
}

// ---------------------------------------------------------------------------
// Display
// ---------------------------------------------------------------------------

func (p *BlackjackPlugin) renderTable(table *bjTable, showDealer bool) string {
	var sb strings.Builder
	sb.WriteString("🃏 **Blackjack** — Round in progress\n\n")

	for _, pl := range table.players {
		name := p.bjDisplayName(pl.UserID)
		v := pl.value()
		extra := ""
		if isBlackjack(pl.Hand) {
			extra = " — Blackjack!"
		} else if v > 21 {
			extra = " — Bust!"
		}
		sb.WriteString(fmt.Sprintf("**%s**:  %s  (%d%s)\n", name, handStr(pl.Hand), v, extra))
	}

	if showDealer {
		dv, _ := handValue(table.dealer)
		sb.WriteString(fmt.Sprintf("Dealer:  %s  (%d)\n", handStr(table.dealer), dv))
	} else {
		// Show first card, hide second
		if len(table.dealer) >= 2 {
			sb.WriteString(fmt.Sprintf("Dealer:  %s 🂠  (? — one card hidden)\n", table.dealer[0].String()))
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Leaderboard and scoring
// ---------------------------------------------------------------------------

func (p *BlackjackPlugin) handleBoard(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, total_earned, games_played, games_won FROM blackjack_scores
		 ORDER BY total_earned DESC LIMIT 10`,
	)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch leaderboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("🃏 **Blackjack Leaderboard**\n\n")
	rank := 0
	for rows.Next() {
		var userID string
		var earned float64
		var played, won int
		rows.Scan(&userID, &earned, &played, &won)
		rank++
		name := p.bjDisplayName(id.UserID(userID))
		sb.WriteString(fmt.Sprintf("%d. **%s** — €%d (%d/%d W/L)\n", rank, name, int(earned), won, played-won))
	}

	if rank == 0 {
		sb.WriteString("No games played yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *BlackjackPlugin) recordBJScore(userID id.UserID, net float64) {
	d := db.Get()
	won := 0
	if net > 0 {
		won = 1
	}
	_, _ = d.Exec(
		`INSERT INTO blackjack_scores (user_id, total_earned, games_played, games_won)
		 VALUES (?, ?, 1, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   total_earned = total_earned + ?,
		   games_played = games_played + 1,
		   games_won = games_won + ?`,
		string(userID), net, won, net, won,
	)
}

func (p *BlackjackPlugin) bjDisplayName(userID id.UserID) string {
	resp, err := p.Client.GetDisplayName(context.Background(), userID)
	if err != nil || resp.DisplayName == "" {
		return string(userID)
	}
	return resp.DisplayName
}
