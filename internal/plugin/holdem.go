package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"github.com/chehsunliu/poker"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// holdemConfig holds configurable parameters.
type holdemConfig struct {
	SmallBlind      int64
	BigBlind        int64
	MinBuyin        int64
	MaxBuyin        int64
	TimeoutSeconds  int
	NPCName         string
	NPCHouseBalance int64
}

func loadHoldemConfig() holdemConfig {
	return holdemConfig{
		SmallBlind:      int64(envInt("HOLDEM_SMALL_BLIND", 10)),
		BigBlind:        int64(envInt("HOLDEM_BIG_BLIND", 20)),
		MinBuyin:        int64(envInt("HOLDEM_MIN_BUYIN", 200)),
		MaxBuyin:        int64(envInt("HOLDEM_MAX_BUYIN", 2000)),
		TimeoutSeconds:  envInt("HOLDEM_TIMEOUT_SECONDS", 90),
		NPCName:         envOrDefault("HOLDEM_NPC_NAME", "TwinBee"),
		NPCHouseBalance: int64(envInt("HOLDEM_NPC_HOUSE_BALANCE", 10000)),
	}
}

// HoldemPlugin is the Texas Hold'em game plugin.
type HoldemPlugin struct {
	Base
	euro   *EuroPlugin
	cfg    holdemConfig
	mu     sync.Mutex
	games  map[id.RoomID]*HoldemGame
	policy PolicyTable
}

// NewHoldemPlugin creates a new Texas Hold'em plugin.
func NewHoldemPlugin(client *mautrix.Client, euro *EuroPlugin) *HoldemPlugin {
	return &HoldemPlugin{
		Base:  NewBase(client),
		euro:  euro,
		cfg:   loadHoldemConfig(),
		games: make(map[id.RoomID]*HoldemGame),
	}
}

func (p *HoldemPlugin) Name() string { return "holdem" }

func (p *HoldemPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "holdem", Description: "Texas Hold'em poker", Usage: "!holdem join/start/fold/check/call/raise/allin/leave/help", Category: "Games"},
	}
}

func (p *HoldemPlugin) Init() error {
	// Try to load pre-trained CFR policy.
	policyPath := envOrDefault("HOLDEM_CFR_POLICY", "data/policy.gob")
	policy, err := LoadPolicy(policyPath)
	if err != nil {
		slog.Warn("holdem: no CFR policy loaded, NPC will use fallback strategy", "err", err)
		p.policy = make(PolicyTable)
	} else {
		p.policy = policy
	}
	return nil
}

func (p *HoldemPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *HoldemPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "holdem") {
		return nil
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "holdem"))

	// Check if this is a DM command.
	isDM := p.isDMRoom(ctx.RoomID)

	// DM-only commands.
	if strings.HasPrefix(args, "tips") {
		return p.handleTipsToggle(ctx, args, isDM)
	}

	// Room-only commands need games room check.
	if !isDM && !isGamesRoom(ctx.RoomID) {
		gr := gamesRoom()
		if gr != "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
		}
	}

	switch {
	case args == "join":
		return p.handleJoin(ctx)
	case args == "leave":
		return p.handleLeave(ctx)
	case args == "start":
		return p.handleStart(ctx)
	case args == "fold":
		return p.handleAction(ctx, "fold", 0)
	case args == "check":
		return p.handleAction(ctx, "check", 0)
	case args == "call":
		return p.handleAction(ctx, "call", 0)
	case strings.HasPrefix(args, "raise"):
		return p.handleRaise(ctx, args)
	case args == "allin":
		return p.handleAction(ctx, "allin", 0)
	case args == "status":
		return p.handleStatus(ctx)
	case args == "help":
		return p.SendReply(ctx.RoomID, ctx.EventID, renderHelpMessage())
	case args == "addbot":
		return p.handleAddBot(ctx)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Unknown command. Try `!holdem help`.")
	}
}

func (p *HoldemPlugin) isDMRoom(roomID id.RoomID) bool {
	// DM rooms typically have exactly 2 members.
	members := p.RoomMembers(roomID)
	return len(members) <= 2
}

func (p *HoldemPlugin) handleTipsToggle(ctx MessageContext, args string, isDM bool) error {
	if !isDM {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tips can only be toggled in DM. Send `!holdem tips on/off` directly to me.")
	}

	if p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tips are always on for admins. 🐝")
	}

	parts := strings.Fields(args)
	if len(parts) < 2 {
		pref := loadTipsPref(ctx.Sender)
		status := "on"
		if !pref {
			status = "off"
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Tips are currently **%s**. Use `!holdem tips on/off` to change.", status))
	}

	switch parts[1] {
	case "on":
		saveTipsPref(ctx.Sender, true)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tips enabled. You'll receive coaching on your turn.")
	case "off":
		saveTipsPref(ctx.Sender, false)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tips disabled.")
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Use `!holdem tips on` or `!holdem tips off`.")
	}
}

func (p *HoldemPlugin) handleJoin(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]

	if game == nil {
		// Create a new game lobby.
		game = &HoldemGame{
			RoomID:            ctx.RoomID,
			SmallBlind:        p.cfg.SmallBlind,
			BigBlind:          p.cfg.BigBlind,
			DealerIdx:         -1, // so first nextActiveIdx lands on seat 0
			WaitingForPlayers: true,
		}
		p.games[ctx.RoomID] = game
	}

	// Check if already seated.
	if game.playerByUserID(ctx.Sender) != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're already at the table.")
	}

	// Max 9 players.
	if len(game.Players) >= 9 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Table is full (9 players max).")
	}

	// Check balance.
	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(p.cfg.MinBuyin) {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Minimum buy-in is €%d. Your balance: €%.0f.", p.cfg.MinBuyin, balance))
	}

	// Resolve DM room.
	dmRoom, err := p.GetDMRoom(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't open a DM with you. Check your privacy settings.")
	}

	// Get display name.
	name := p.holdemDisplayName(ctx.Sender)

	// Determine buy-in (capped at MaxBuyin).
	buyin := int64(balance)
	if buyin > p.cfg.MaxBuyin {
		buyin = p.cfg.MaxBuyin
	}

	// Debit buy-in immediately to prevent double-spending across tables.
	if !p.euro.Debit(ctx.Sender, float64(buyin), "holdem_buyin") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to reserve buy-in.")
	}

	state := PlayerActive
	sittingOut := false
	if game.HandInProgress {
		state = PlayerSatOut
		sittingOut = true
	}

	player := &HoldemPlayer{
		UserID:       ctx.Sender,
		DisplayName:  name,
		Stack:        buyin,
		OpeningStack: buyin,
		State:        state,
		TipsEnabled:  loadTipsPref(ctx.Sender) || p.IsAdmin(ctx.Sender),
		SittingOut:   sittingOut,
		DMRoomID:     dmRoom,
	}

	game.Players = append(game.Players, player)

	if sittingOut {
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("**%s** will join the next hand. (€%d buy-in)", name, buyin))
	}

	return p.SendMessage(ctx.RoomID,
		fmt.Sprintf("**%s** joined the table. (€%d buy-in) — %d players seated. Use `!holdem start` when ready.",
			name, buyin, len(game.Players)))
}

func (p *HoldemPlugin) handleLeave(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]
	if game == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active game here.")
	}

	player := game.playerByUserID(ctx.Sender)
	if player == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not at the table.")
	}

	if !game.HandInProgress || player.SittingOut {
		// Credit remaining stack back (buy-in was debited at join).
		if !player.IsNPC && player.Stack > 0 {
			p.euro.Credit(player.UserID, float64(player.Stack), "holdem_cashout")
		}
		// Remove immediately.
		p.removePlayer(game, ctx.Sender)
		p.SendMessage(ctx.RoomID, fmt.Sprintf("**%s** has left the table.", player.DisplayName))

		if len(game.Players) == 0 {
			delete(p.games, ctx.RoomID)
		}
		return nil
	}

	idx := game.playerIdx(ctx.Sender)
	if idx == game.ActionIdx {
		// It's their turn — fold and remove.
		game.doFold(idx)
		p.SendMessage(ctx.RoomID, fmt.Sprintf("**%s** folds and leaves the table.", player.DisplayName))

		// Credit remaining stack back (buy-in was debited at join).
		if player.Stack > 0 {
			p.euro.Credit(player.UserID, float64(player.Stack), "holdem_cashout")
		}
		p.removePlayer(game, ctx.Sender)

		if game.activeCount() <= 1 {
			p.finishHand(game)
		} else {
			p.advanceAction(game)
		}
		return nil
	}

	// Not their turn — flag for removal.
	player.WantsLeave = true
	return p.SendMessage(ctx.RoomID, fmt.Sprintf("**%s** will leave after this hand.", player.DisplayName))
}

func (p *HoldemPlugin) handleStart(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]
	if game == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No players seated. Use `!holdem join` first.")
	}

	if game.HandInProgress {
		return p.SendReply(ctx.RoomID, ctx.EventID, "A hand is already in progress.")
	}

	// Count non-sitting-out players.
	ready := 0
	for _, pl := range game.Players {
		if !pl.SittingOut {
			ready++
		}
	}

	if ready < 2 {
		hint := " Use `!holdem join`."
		if ready == 1 {
			hint = " Use `!holdem addbot` to add an AI opponent."
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, "Need at least 2 players to start."+hint)
	}

	p.startHand(game)
	return nil
}

func (p *HoldemPlugin) handleRaise(ctx MessageContext, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!holdem raise <amount>`")
	}

	amtStr := strings.TrimPrefix(parts[1], "€")
	amount, err := strconv.ParseInt(amtStr, 10, 64)
	if err != nil || amount <= 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Invalid amount. Usage: `!holdem raise <amount>`")
	}

	return p.handleAction(ctx, "raise", amount)
}

func (p *HoldemPlugin) handleAction(ctx MessageContext, action string, amount int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]
	if game == nil || !game.HandInProgress {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No hand in progress.")
	}

	seatIdx := game.playerIdx(ctx.Sender)
	if seatIdx < 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not at this table.")
	}

	if seatIdx != game.ActionIdx {
		return p.SendReply(ctx.RoomID, ctx.EventID, "It's not your turn.")
	}

	player := game.Players[seatIdx]
	if player.State != PlayerActive {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You can't act right now.")
	}

	var result ActionResult
	var errMsg string

	switch action {
	case "fold":
		result = game.doFold(seatIdx)
	case "check":
		result, errMsg = game.doCheck(seatIdx)
	case "call":
		result, errMsg = game.doCall(seatIdx)
	case "raise":
		result, errMsg = game.doRaise(seatIdx, amount)
	case "allin":
		result = game.doAllIn(seatIdx)
	}

	if errMsg != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, errMsg)
	}

	// Reset action timer.
	p.stopActionTimer(game)

	// Broadcast action to all players via DM.
	p.broadcastDM(game, result.Announcement)

	if result.HandOver {
		p.finishHand(game)
		return nil
	}

	if result.AllAllIn {
		p.runOutBoard(game)
		return nil
	}

	if result.StreetOver {
		p.advanceStreet(game)
		return nil
	}

	// Advance to next player.
	game.ActionIdx = game.nextCanActIdx(seatIdx)
	p.sendTurnNotifications(game)
	p.startActionTimer(game)
	return nil
}

func (p *HoldemPlugin) handleStatus(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]
	if game == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active game here.")
	}

	seatIdx := game.playerIdx(ctx.Sender)
	if seatIdx < 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not at this table.")
	}

	player := game.Players[seatIdx]
	hand := ""
	if game.HandInProgress && player.State != PlayerSatOut {
		hand = renderPrivateHand(game, seatIdx) + "\n"
	}
	view := renderTableView(game, seatIdx)
	return p.SendMessage(player.DMRoomID, hand+view)
}

func (p *HoldemPlugin) handleAddBot(ctx MessageContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	game := p.games[ctx.RoomID]
	if game == nil {
		game = &HoldemGame{
			RoomID:            ctx.RoomID,
			SmallBlind:        p.cfg.SmallBlind,
			BigBlind:          p.cfg.BigBlind,
			DealerIdx:         -1,
			WaitingForPlayers: true,
		}
		p.games[ctx.RoomID] = game
	}

	// Check if NPC already exists.
	for _, pl := range game.Players {
		if pl.IsNPC {
			return p.SendReply(ctx.RoomID, ctx.EventID, "An AI opponent is already at the table.")
		}
	}

	if len(game.Players) >= 9 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Table is full.")
	}

	// Get NPC house balance.
	npcBalance := p.getNPCBalance()
	buyin := npcBalance
	if buyin > p.cfg.MaxBuyin {
		buyin = p.cfg.MaxBuyin
	}
	if buyin < p.cfg.MinBuyin {
		buyin = p.cfg.MinBuyin
	}

	npc := &HoldemPlayer{
		UserID:       id.UserID(fmt.Sprintf("@npc_%s:local", strings.ToLower(p.cfg.NPCName))),
		DisplayName:  p.cfg.NPCName,
		Stack:        buyin,
		OpeningStack: buyin,
		State:        PlayerActive,
		IsNPC:        true,
	}

	if game.HandInProgress {
		npc.State = PlayerSatOut
		npc.SittingOut = true
	}

	game.Players = append(game.Players, npc)

	msg := fmt.Sprintf("🤖 **%s** is an AI opponent using a trained poker solver. (€%d buy-in)", p.cfg.NPCName, buyin)
	return p.SendMessage(ctx.RoomID, msg)
}

// --- Game lifecycle ---

func (p *HoldemPlugin) startHand(game *HoldemGame) {
	// Unsit players who were waiting.
	for _, pl := range game.Players {
		if pl.SittingOut {
			pl.SittingOut = false
			pl.State = PlayerActive
		}
	}

	// Remove players who want to leave.
	// Balance was already settled by settleNetDeltas at end of previous hand,
	// and OpeningStack was reset to Stack in endHand, so no settlement needed here.
	for i := len(game.Players) - 1; i >= 0; i-- {
		if game.Players[i].WantsLeave {
			game.Players = append(game.Players[:i], game.Players[i+1:]...)
		}
	}

	if len(game.Players) < 2 {
		p.SendMessage(game.RoomID, "Not enough players to start a hand.")
		return
	}

	// Reset game state.
	game.HandInProgress = true
	game.Street = StreetPreFlop
	game.Community = nil
	game.SidePots = nil
	game.Pot = 0
	game.Deck = newShuffledDeck()
	game.DeckPos = 0

	// Reset player state.
	for _, pl := range game.Players {
		pl.State = PlayerActive
		pl.Bet = 0
		pl.TotalBet = 0
		pl.Hole = [2]poker.Card{}
	}

	// Advance dealer.
	game.DealerIdx = game.nextActiveIdx(game.DealerIdx)

	// Post blinds.
	_, bbIdx := game.postBlinds()

	// Deal hole cards (no burn before initial deal — burns are before flop/turn/river only).
	for _, pl := range game.Players {
		if pl.State != PlayerSatOut {
			pl.Hole[0] = game.drawCard()
			pl.Hole[1] = game.drawCard()
		}
	}

	// Set first to act.
	game.ActionIdx = game.firstToActPreflop(bbIdx)

	// Announce to room.
	p.SendMessage(game.RoomID, renderStartAnnouncement(game))

	// Send hole cards to all players via DM.
	for i, pl := range game.Players {
		if pl.IsNPC || pl.State == PlayerSatOut {
			continue
		}
		view := renderPrivateHand(game, i)
		p.SendMessage(pl.DMRoomID, view)
	}

	// Send turn notification.
	p.sendTurnNotifications(game)
	p.startActionTimer(game)
}

func (p *HoldemPlugin) advanceStreet(game *HoldemGame) {
	game.collectPot()
	game.resetStreetBets()

	switch game.Street {
	case StreetPreFlop:
		game.Street = StreetFlop
		game.burnCard()
		game.Community = append(game.Community, game.drawCard(), game.drawCard(), game.drawCard())
	case StreetFlop:
		game.Street = StreetTurn
		game.burnCard()
		game.Community = append(game.Community, game.drawCard())
	case StreetTurn:
		game.Street = StreetRiver
		game.burnCard()
		game.Community = append(game.Community, game.drawCard())
	case StreetRiver:
		game.Street = StreetShowdown
		p.doShowdown(game)
		return
	}

	// Set action to first active player after dealer.
	game.ActionIdx = game.firstToActPostflop()
	game.LastAggressorIdx = game.ActionIdx

	// Notify all players of new board.
	for i, pl := range game.Players {
		if pl.IsNPC || pl.State == PlayerSatOut || pl.State == PlayerFolded {
			continue
		}
		view := renderTableView(game, i)
		p.SendMessage(pl.DMRoomID, view)
	}

	// Check if all players are all-in.
	if game.canActCount() <= 1 {
		if game.canActCount() == 0 {
			p.runOutBoard(game)
			return
		}
		// Only 1 player can act — if they're the only non-allin, auto-advance.
		if game.activeCount()-game.canActCount() > 0 {
			// There are all-in players, run out.
			p.runOutBoard(game)
			return
		}
	}

	p.sendTurnNotifications(game)
	p.startActionTimer(game)
}

func (p *HoldemPlugin) advanceAction(game *HoldemGame) {
	if game.activeCount() <= 1 {
		p.finishHand(game)
		return
	}

	if game.canActCount() == 0 {
		p.runOutBoard(game)
		return
	}

	// Find next active player.
	game.ActionIdx = game.nextCanActIdx(game.ActionIdx)

	if game.isStreetComplete(game.ActionIdx) {
		p.advanceStreet(game)
		return
	}

	p.sendTurnNotifications(game)
	p.startActionTimer(game)
}

func (p *HoldemPlugin) runOutBoard(game *HoldemGame) {
	game.collectPot()

	// Build side pots if needed.
	hasAllIn := false
	for _, pl := range game.Players {
		if pl.State == PlayerAllIn {
			hasAllIn = true
			break
		}
	}
	if hasAllIn {
		game.buildSidePots()
	}

	// Deal remaining community cards.
	for game.Street < StreetShowdown {
		switch game.Street {
		case StreetPreFlop:
			game.Street = StreetFlop
			game.burnCard()
			game.Community = append(game.Community, game.drawCard(), game.drawCard(), game.drawCard())
		case StreetFlop:
			game.Street = StreetTurn
			game.burnCard()
			game.Community = append(game.Community, game.drawCard())
		case StreetTurn:
			game.Street = StreetRiver
			game.burnCard()
			game.Community = append(game.Community, game.drawCard())
		case StreetRiver:
			game.Street = StreetShowdown
		}

		// Brief notification with new board.
		boardMsg := fmt.Sprintf("**%s** — Board: %s", game.Street.String(), renderCards(game.Community))
		p.broadcastDM(game, boardMsg)
	}

	p.doShowdown(game)
}

func (p *HoldemPlugin) doShowdown(game *HoldemGame) {
	game.collectPot()

	// Build side pots if needed.
	hasAllIn := false
	for _, pl := range game.Players {
		if pl.State == PlayerAllIn {
			hasAllIn = true
			break
		}
	}
	if hasAllIn && len(game.SidePots) == 0 {
		game.buildSidePots()
	}

	results, winnings := runShowdown(game)

	// Track bot defeats.
	for _, pl := range game.Players {
		if pl.IsNPC {
			continue
		}
		won := winnings[pl.UserID]
		if won == 0 && pl.State != PlayerFolded {
			// Lost at showdown — check if any NPC won.
			for _, npl := range game.Players {
				if npl.IsNPC && winnings[npl.UserID] > 0 {
					recordBotDefeat(pl.UserID, "holdem")
					break
				}
			}
		}
	}

	// Post end announcement to room.
	endAnn := renderEndAnnouncement(results, game)
	p.SendMessage(game.RoomID, endAnn)

	// Settle balances.
	settleNetDeltas(game, p.euro)

	// Update NPC balance.
	for _, pl := range game.Players {
		if pl.IsNPC {
			delta := pl.Stack - pl.OpeningStack
			p.updateNPCBalance(delta)
		}
	}

	// Record scores.
	p.recordScores(game, winnings)

	p.endHand(game)
}

func (p *HoldemPlugin) finishHand(game *HoldemGame) {
	p.stopActionTimer(game)

	// Return uncalled bet.
	name, amount := game.returnUncalledBet()
	if amount > 0 {
		msg := renderUncalledBetReturn(name, amount)
		p.broadcastDM(game, msg)
	}

	// Award pot to last remaining player.
	ann, winnerID := awardPotToLastPlayer(game)
	if ann != "" {
		p.SendMessage(game.RoomID, ann)
	}

	// Track bot defeats (if NPC won by everyone folding).
	for _, pl := range game.Players {
		if pl.IsNPC {
			continue
		}
		if pl.UserID != winnerID && pl.State == PlayerFolded {
			for _, npl := range game.Players {
				if npl.IsNPC && npl.UserID == winnerID {
					recordBotDefeat(pl.UserID, "holdem")
					break
				}
			}
		}
	}

	// Settle balances.
	settleNetDeltas(game, p.euro)

	// Update NPC balance.
	for _, pl := range game.Players {
		if pl.IsNPC {
			delta := pl.Stack - pl.OpeningStack
			p.updateNPCBalance(delta)
		}
	}

	p.endHand(game)
}

func (p *HoldemPlugin) endHand(game *HoldemGame) {
	p.stopActionTimer(game)
	game.HandInProgress = false
	game.Street = StreetPreFlop

	// Remove players who want to leave.
	for i := len(game.Players) - 1; i >= 0; i-- {
		pl := game.Players[i]
		if pl.WantsLeave || pl.Stack <= 0 {
			if !pl.IsNPC {
				p.SendMessage(game.RoomID, fmt.Sprintf("**%s** has left the table.", pl.DisplayName))
			}
			game.Players = append(game.Players[:i], game.Players[i+1:]...)
		}
	}

	// Reset opening stacks for next hand.
	for _, pl := range game.Players {
		pl.OpeningStack = pl.Stack
	}

	if len(game.Players) < 2 {
		p.SendMessage(game.RoomID, "Not enough players for another hand. Game over.")
		delete(p.games, game.RoomID)
		return
	}

	p.SendMessage(game.RoomID, fmt.Sprintf("Hand complete. %d players at the table. Type `!holdem start` for the next hand.", len(game.Players)))
}

// --- Notifications ---

func (p *HoldemPlugin) sendTurnNotifications(game *HoldemGame) {
	actionPlayer := game.Players[game.ActionIdx]

	if actionPlayer.IsNPC {
		// NPC acts automatically.
		go p.npcAct(game)
		return
	}

	// Send table view + private hand to action player.
	view := renderTableView(game, game.ActionIdx)
	hand := renderPrivateHand(game, game.ActionIdx)
	p.SendMessage(actionPlayer.DMRoomID, hand+"\n"+view)

	// Generate tip asynchronously — build context under lock, generate outside.
	if actionPlayer.TipsEnabled {
		tipCtx := buildTipContext(game, game.ActionIdx)
		dmRoom := actionPlayer.DMRoomID
		go func() {
			tip := generateTip(tipCtx)
			p.SendMessage(dmRoom, tip)
		}()
	}
}

func (p *HoldemPlugin) broadcastDM(game *HoldemGame, msg string) {
	for _, pl := range game.Players {
		if pl.IsNPC || pl.State == PlayerSatOut {
			continue
		}
		p.SendMessage(pl.DMRoomID, msg)
	}
}

// --- NPC ---

func (p *HoldemPlugin) npcAct(game *HoldemGame) {
	// Compute action under lock (reads game state safely).
	p.mu.Lock()
	if !game.HandInProgress {
		p.mu.Unlock()
		return
	}
	npcIdx := game.ActionIdx
	action, delay := NPCChooseAction(p.policy, game, npcIdx)
	p.mu.Unlock()

	// Human-like delay outside the lock.
	time.Sleep(delay)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Verify game state hasn't changed.
	if !game.HandInProgress || game.ActionIdx != npcIdx {
		return
	}

	actionStr, amount := cfrActionToGameAction(action, game, npcIdx)

	var result ActionResult
	switch actionStr {
	case "fold":
		result = game.doFold(npcIdx)
	case "check":
		result, _ = game.doCheck(npcIdx)
	case "call":
		result, _ = game.doCall(npcIdx)
	case "raise":
		result, _ = game.doRaise(npcIdx, amount)
	case "allin":
		result = game.doAllIn(npcIdx)
	}

	p.broadcastDM(game, result.Announcement)

	if result.HandOver {
		p.finishHand(game)
		return
	}

	if result.AllAllIn {
		p.runOutBoard(game)
		return
	}

	if result.StreetOver {
		p.advanceStreet(game)
		return
	}

	game.ActionIdx = game.nextCanActIdx(npcIdx)
	p.sendTurnNotifications(game)
	p.startActionTimer(game)
}

// --- Timers ---

func (p *HoldemPlugin) startActionTimer(game *HoldemGame) {
	p.stopActionTimer(game)

	timeout := time.Duration(p.cfg.TimeoutSeconds) * time.Second
	actionIdx := game.ActionIdx
	roomID := game.RoomID

	// Warning at 30s before timeout (if timeout > 35s).
	if p.cfg.TimeoutSeconds > 35 {
		warningDelay := timeout - 30*time.Second
		player := game.Players[actionIdx]
		dmRoom := player.DMRoomID
		game.warningTimer = time.AfterFunc(warningDelay, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			g := p.games[roomID]
			if g == nil || !g.HandInProgress || g.ActionIdx != actionIdx {
				return
			}
			pl := g.Players[actionIdx]
			if pl.IsNPC {
				return
			}
			p.SendMessage(dmRoom, "⏰ **30 seconds** remaining to act!")
		})
	}

	game.actionTimer = time.AfterFunc(timeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		g := p.games[roomID]
		if g == nil || !g.HandInProgress || g.ActionIdx != actionIdx {
			return
		}

		player := g.Players[actionIdx]
		if player.IsNPC {
			return
		}

		// Auto-action: check if possible, otherwise fold.
		toCall := g.CurrentBet - player.Bet
		if toCall <= 0 {
			result, _ := g.doCheck(actionIdx)
			p.broadcastDM(g, "⏰ "+result.Announcement+" (timeout)")
		} else {
			result := g.doFold(actionIdx)
			p.broadcastDM(g, "⏰ "+result.Announcement+" (timeout)")
		}

		if g.activeCount() <= 1 {
			p.finishHand(g)
			return
		}

		if g.canActCount() == 0 {
			p.runOutBoard(g)
			return
		}

		g.ActionIdx = g.nextCanActIdx(actionIdx)

		if g.isStreetComplete(g.ActionIdx) {
			p.advanceStreet(g)
			return
		}

		p.sendTurnNotifications(g)
		p.startActionTimer(g)
	})
}

func (p *HoldemPlugin) stopActionTimer(game *HoldemGame) {
	if game.warningTimer != nil {
		game.warningTimer.Stop()
		game.warningTimer = nil
	}
	if game.actionTimer != nil {
		game.actionTimer.Stop()
		game.actionTimer = nil
	}
}

// --- Helpers ---

func (p *HoldemPlugin) removePlayer(game *HoldemGame, uid id.UserID) {
	for i, pl := range game.Players {
		if pl.UserID == uid {
			game.Players = append(game.Players[:i], game.Players[i+1:]...)
			// Adjust seat indices that reference positions after the removed player.
			if game.DealerIdx >= i && game.DealerIdx > 0 {
				game.DealerIdx--
			}
			if game.ActionIdx >= i && game.ActionIdx > 0 {
				game.ActionIdx--
			}
			if game.LastAggressorIdx >= i && game.LastAggressorIdx > 0 {
				game.LastAggressorIdx--
			}
			// Wrap indices if they exceed the new length.
			if n := len(game.Players); n > 0 {
				game.DealerIdx %= n
				game.ActionIdx %= n
				game.LastAggressorIdx %= n
			}
			return
		}
	}
}

func (p *HoldemPlugin) holdemDisplayName(userID id.UserID) string {
	resp, err := p.Client.GetDisplayName(context.Background(), userID)
	if err != nil || resp.DisplayName == "" {
		return string(userID)
	}
	return resp.DisplayName
}

func (p *HoldemPlugin) getNPCBalance() int64 {
	d := db.Get()
	var balance int64
	err := d.QueryRow(`SELECT balance FROM holdem_npc_balance WHERE npc_name = ?`, p.cfg.NPCName).Scan(&balance)
	if err != nil {
		// Initialize.
		_, _ = d.Exec(`INSERT OR IGNORE INTO holdem_npc_balance (npc_name, balance) VALUES (?, ?)`,
			p.cfg.NPCName, p.cfg.NPCHouseBalance)
		return p.cfg.NPCHouseBalance
	}
	return balance
}

func (p *HoldemPlugin) updateNPCBalance(delta int64) {
	d := db.Get()
	_, _ = d.Exec(`UPDATE holdem_npc_balance SET balance = balance + ?, hands_played = hands_played + 1 WHERE npc_name = ?`,
		delta, p.cfg.NPCName)
}

func (p *HoldemPlugin) recordScores(game *HoldemGame, winnings map[id.UserID]int64) {
	d := db.Get()
	for _, pl := range game.Players {
		if pl.IsNPC {
			continue
		}
		won := winnings[pl.UserID]
		lost := int64(0)
		delta := pl.Stack - pl.OpeningStack
		if delta < 0 {
			lost = -delta
		}
		biggestPot := game.Pot
		for _, sp := range game.SidePots {
			if sp.Amount > biggestPot {
				biggestPot = sp.Amount
			}
		}

		_, _ = d.Exec(
			`INSERT INTO holdem_scores (user_id, hands_played, total_won, total_lost, biggest_pot)
			 VALUES (?, 1, ?, ?, ?)
			 ON CONFLICT(user_id) DO UPDATE SET
			   hands_played = hands_played + 1,
			   total_won = total_won + ?,
			   total_lost = total_lost + ?,
			   biggest_pot = MAX(biggest_pot, ?)`,
			string(pl.UserID), won, lost, biggestPot, won, lost, biggestPot,
		)
	}
}
