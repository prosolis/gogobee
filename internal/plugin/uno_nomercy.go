package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// ---------------------------------------------------------------------------
// No Mercy helpers
// ---------------------------------------------------------------------------

// cardDrawValue returns the draw penalty for a card (0 if not a draw card).
func cardDrawValue(v unoValue) int {
	switch v {
	case unoDrawTwo:
		return 2
	case unoDrawFour, unoWildReverseDraw4:
		return 4
	case unoWildDrawSix:
		return 6
	case unoWildDrawTen:
		return 10
	default:
		return 0
	}
}

// cardPointValue returns the point value of a card for sudden-death scoring.
func cardPointValue(c unoCard) int {
	switch c.Value {
	case unoZero:
		return 0
	case unoOne:
		return 1
	case unoTwo:
		return 2
	case unoThree:
		return 3
	case unoFour:
		return 4
	case unoFive:
		return 5
	case unoSix:
		return 6
	case unoSeven:
		return 7
	case unoEight:
		return 8
	case unoNine:
		return 9
	case unoSkip, unoReverse, unoDrawTwo:
		return 20
	case unoSkipEveryone, unoDrawFour, unoDiscardAll:
		return 30
	case unoWildCard, unoWildDrawFour:
		return 50
	case unoWildReverseDraw4, unoWildDrawSix, unoWildColorRoulette:
		return 60
	case unoWildDrawTen:
		return 75
	default:
		return 0
	}
}

func scoreHand(hand []unoCard) int {
	total := 0
	for _, c := range hand {
		total += cardPointValue(c)
	}
	return total
}

func formatHandScore(hand []unoCard) string {
	score := scoreHand(hand)
	return fmt.Sprintf("%d cards, %d points", len(hand), score)
}

func isDrawCard(v unoValue) bool {
	return cardDrawValue(v) > 0
}

// canPlayOnStacking checks if a card can be played during a stacking phase.
// Any draw card can stack on any other draw card (no escalation requirement).
// Wild draws always match; colored draws must match topColor.
func (c unoCard) canPlayOnStacking(topColor unoColor, _ int) bool {
	if cardDrawValue(c.Value) == 0 {
		return false
	}
	if c.isWild() {
		return true
	}
	return c.Color == topColor
}

// hasStackableCard returns true if the hand contains a card that can be stacked.
func hasStackableCard(hand []unoCard, topColor unoColor, stackMinValue int) bool {
	for _, c := range hand {
		if c.canPlayOnStacking(topColor, stackMinValue) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// No Mercy deck (168 cards)
// ---------------------------------------------------------------------------

func newNoMercyDeck() []unoCard {
	var cards []unoCard
	colors := []unoColor{unoRed, unoBlue, unoYellow, unoGreen}

	for _, color := range colors {
		// Numbers 0-9: ×2 each
		for v := unoZero; v <= unoNine; v++ {
			cards = append(cards, unoCard{color, v})
			cards = append(cards, unoCard{color, v})
		}
		// Skip ×3
		for i := 0; i < 3; i++ {
			cards = append(cards, unoCard{color, unoSkip})
		}
		// Skip Everyone ×2
		for i := 0; i < 2; i++ {
			cards = append(cards, unoCard{color, unoSkipEveryone})
		}
		// Reverse ×4
		for i := 0; i < 4; i++ {
			cards = append(cards, unoCard{color, unoReverse})
		}
		// Draw Two ×2
		for i := 0; i < 2; i++ {
			cards = append(cards, unoCard{color, unoDrawTwo})
		}
		// Draw Four (colored) ×2
		for i := 0; i < 2; i++ {
			cards = append(cards, unoCard{color, unoDrawFour})
		}
		// Discard All ×3
		for i := 0; i < 3; i++ {
			cards = append(cards, unoCard{color, unoDiscardAll})
		}
	}

	// Wild Reverse Draw Four ×8
	for i := 0; i < 8; i++ {
		cards = append(cards, unoCard{unoWild, unoWildReverseDraw4})
	}
	// Wild Draw Six ×4
	for i := 0; i < 4; i++ {
		cards = append(cards, unoCard{unoWild, unoWildDrawSix})
	}
	// Wild Draw Ten ×4
	for i := 0; i < 4; i++ {
		cards = append(cards, unoCard{unoWild, unoWildDrawTen})
	}
	// Wild Color Roulette ×8
	for i := 0; i < 8; i++ {
		cards = append(cards, unoCard{unoWild, unoWildColorRoulette})
	}

	rand.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
	return cards
}

// ---------------------------------------------------------------------------
// Mercy Rule
// ---------------------------------------------------------------------------

const mercyLimit = 25

// checkSoloMercyElimination checks if the player or bot has 25+ cards.
// Returns true if someone was eliminated (game ends).
func (p *UnoPlugin) checkSoloMercyElimination(game *unoGame) bool {
	if !game.noMercy {
		return false
	}
	if len(game.playerHand) >= mercyLimit {
		p.SendMessage(game.dmRoomID,
			fmt.Sprintf("💀 **MERCY KILL!** You have %d cards — eliminated!", len(game.playerHand)))
		p.botWins(game)
		return true
	}
	if len(game.botHand) >= mercyLimit {
		bn := unoBotName()
		p.SendMessage(game.dmRoomID,
			fmt.Sprintf("💀 **MERCY KILL!** %s has %d cards — eliminated!", bn, len(game.botHand)))
		p.playerWins(game)
		return true
	}
	return false
}

// checkMultiMercyElimination checks if a multiplayer player has 25+ cards.
// Returns true if the player was eliminated.
func (p *UnoPlugin) checkMultiMercyElimination(game *unoMultiGame, player *unoMultiPlayer) bool {
	if !game.noMercy || len(player.hand) < mercyLimit {
		return false
	}

	player.active = false
	name := p.multiName(player)

	// Shuffle cards back into draw pile
	game.drawPile = append(game.drawPile, player.hand...)
	rand.Shuffle(len(game.drawPile), func(i, j int) { game.drawPile[i], game.drawPile[j] = game.drawPile[j], game.drawPile[i] })
	player.hand = nil

	p.SendMessage(game.roomID,
		fmt.Sprintf("💀 **MERCY KILL!** %s had %d+ cards — eliminated!\n\n%s",
			name, mercyLimit, pickNoMercyCommentary("mercy_kill")))

	if !player.isBot {
		p.SendMessage(player.dmRoomID,
			fmt.Sprintf("💀 You've been mercy-killed! (%d+ cards)", mercyLimit))
	}

	// Check if game should end
	active := game.activePlayers()
	if len(active) <= 1 {
		if len(active) == 1 {
			winner := active[0]
			if winner.isBot {
				p.multiBotWins(game, winner)
			} else {
				p.multiPlayerWins(game, winner)
			}
		} else {
			game.done = true
			p.SendMessage(game.roomID, "🃏 Game ended — no players remaining.")
			p.cleanupMultiGame(game)
		}
		return true
	}

	return true
}

// ---------------------------------------------------------------------------
// Draw-until-playable helpers
// ---------------------------------------------------------------------------

// formatDrawnCards formats a list of drawn cards for display.
func formatDrawnCards(cards []unoCard) string {
	if len(cards) == 1 {
		return cards[0].Display()
	}
	var parts []string
	for _, c := range cards {
		parts = append(parts, c.Display())
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Color Roulette
// ---------------------------------------------------------------------------

// executeColorRoulette flips cards from draw pile until the chosen color appears.
// All flipped cards are added to the target's hand.
// Returns the flipped cards for display.
func (p *UnoPlugin) executeColorRouletteSolo(game *unoGame, targetIsPlayer bool, chosenColor unoColor) []unoCard {
	var flipped []unoCard
	for {
		if len(game.drawPile) == 0 {
			game.reshuffleDiscard()
		}
		if len(game.drawPile) == 0 {
			break
		}
		card := game.drawPile[0]
		game.drawPile = game.drawPile[1:]
		flipped = append(flipped, card)
		if targetIsPlayer {
			game.playerHand = append(game.playerHand, card)
		} else {
			game.botHand = append(game.botHand, card)
		}
		if card.Color == chosenColor {
			break
		}
	}
	return flipped
}

func (p *UnoPlugin) executeColorRouletteMulti(game *unoMultiGame, target *unoMultiPlayer, chosenColor unoColor) []unoCard {
	var flipped []unoCard
	for {
		if len(game.drawPile) == 0 {
			game.reshuffleDiscard()
		}
		if len(game.drawPile) == 0 {
			break
		}
		card := game.drawPile[0]
		game.drawPile = game.drawPile[1:]
		flipped = append(flipped, card)
		target.hand = append(target.hand, card)
		if card.Color == chosenColor {
			break
		}
	}
	return flipped
}

// ---------------------------------------------------------------------------
// 7-0 Rule helpers
// ---------------------------------------------------------------------------

// rotateHandsMulti rotates all active players' hands in the play direction.
func rotateHandsMulti(game *unoMultiGame) {
	active := game.activePlayers()
	if len(active) < 2 {
		return
	}

	// Build ordered list of active players by turn order in current direction
	n := len(game.players)
	var ordered []*unoMultiPlayer
	idx := game.currentIdx
	for i := 0; i < n; i++ {
		p := game.players[idx]
		if p.active {
			ordered = append(ordered, p)
		}
		idx = (idx + game.direction + n) % n
	}

	if len(ordered) < 2 {
		return
	}

	// Save all hands
	hands := make([][]unoCard, len(ordered))
	for i, p := range ordered {
		hands[i] = p.hand
	}

	// Rotate: each player gets the hand of the player behind them (opposite of direction)
	for i := range ordered {
		prev := (i - 1 + len(ordered)) % len(ordered)
		ordered[i].hand = hands[prev]
	}
}

// swapHandsSolo swaps player and bot hands.
func swapHandsSolo(game *unoGame) {
	game.playerHand, game.botHand = game.botHand, game.playerHand
}

// swapHandsMulti swaps two players' hands.
func swapHandsMulti(a, b *unoMultiPlayer) {
	a.hand, b.hand = b.hand, a.hand
}

// ---------------------------------------------------------------------------
// Discard All helper
// ---------------------------------------------------------------------------

// discardAllOfColor removes all cards of the given color from the hand.
// Returns the count of additional cards removed (not counting the played card).
func discardAllOfColor(hand *[]unoCard, color unoColor) int {
	var kept []unoCard
	removed := 0
	for _, c := range *hand {
		if c.Color == color {
			removed++
		} else {
			kept = append(kept, c)
		}
	}
	*hand = kept
	return removed
}

// ---------------------------------------------------------------------------
// No Mercy Bot AI
// ---------------------------------------------------------------------------

// botPickCardNoMercy selects the best card for bot in No Mercy mode.
func botPickCardNoMercy(hand []unoCard, discardTop unoCard, topColor unoColor, bookDown bool, opponentMinCards int, stackMinValue int) (unoCard, int) {
	// During stacking, only stackable cards are valid
	if stackMinValue > 0 {
		return botPickStackCard(hand, topColor, stackMinValue)
	}

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
		return botPickAggressiveNoMercy(hand, topColor, playable)
	}
	return botPickNormalNoMercy(hand, topColor, playable, opponentMinCards)
}

func botPickStackCard(hand []unoCard, topColor unoColor, stackMinValue int) (unoCard, int) {
	var stackable []int
	for i, c := range hand {
		if c.canPlayOnStacking(topColor, stackMinValue) {
			stackable = append(stackable, i)
		}
	}
	if len(stackable) == 0 {
		return unoCard{}, -1 // bot must absorb
	}
	// Play the lowest-value stackable card to preserve bigger weapons
	bestIdx := stackable[0]
	bestVal := cardDrawValue(hand[bestIdx].Value)
	for _, i := range stackable[1:] {
		dv := cardDrawValue(hand[i].Value)
		if dv < bestVal {
			bestIdx = i
			bestVal = dv
		}
	}
	return hand[bestIdx], bestIdx
}

func botPickNormalNoMercy(hand []unoCard, topColor unoColor, playable []int, opponentMinCards int) (unoCard, int) {
	var draws, actions, numbers, discardAlls, skipEveryones, roulettes []int

	for _, i := range playable {
		c := hand[i]
		switch {
		case isDrawCard(c.Value):
			draws = append(draws, i)
		case c.Value == unoDiscardAll:
			discardAlls = append(discardAlls, i)
		case c.Value == unoSkipEveryone:
			skipEveryones = append(skipEveryones, i)
		case c.Value == unoWildColorRoulette:
			roulettes = append(roulettes, i)
		case c.Value.isAction():
			actions = append(actions, i)
		default:
			numbers = append(numbers, i)
		}
	}

	// Prioritize Discard All if we have 3+ cards of that color
	for _, i := range discardAlls {
		color := hand[i].Color
		count := 0
		for _, c := range hand {
			if c.Color == color {
				count++
			}
		}
		if count >= 3 {
			return hand[i], i
		}
	}

	// If opponent close to winning, go aggressive with draw cards
	if opponentMinCards <= 3 {
		if len(draws) > 0 {
			// Play highest draw value
			bestIdx := draws[0]
			bestVal := cardDrawValue(hand[bestIdx].Value)
			for _, i := range draws[1:] {
				dv := cardDrawValue(hand[i].Value)
				if dv > bestVal {
					bestIdx = i
					bestVal = dv
				}
			}
			return hand[bestIdx], bestIdx
		}
		if len(skipEveryones) > 0 {
			return hand[skipEveryones[0]], skipEveryones[0]
		}
	}

	// Normal priority: numbers > actions > discard all > skip everyone > draws > roulettes
	if len(numbers) > 0 {
		// Prefer color match
		for _, i := range numbers {
			if hand[i].Color == topColor {
				return hand[i], i
			}
		}
		return hand[numbers[0]], numbers[0]
	}
	if len(actions) > 0 {
		return hand[actions[0]], actions[0]
	}
	if len(discardAlls) > 0 {
		return hand[discardAlls[0]], discardAlls[0]
	}
	if len(skipEveryones) > 0 {
		return hand[skipEveryones[0]], skipEveryones[0]
	}
	if len(draws) > 0 {
		// Play lowest draw to save bigger ones
		bestIdx := draws[0]
		bestVal := cardDrawValue(hand[bestIdx].Value)
		for _, i := range draws[1:] {
			dv := cardDrawValue(hand[i].Value)
			if dv < bestVal {
				bestIdx = i
				bestVal = dv
			}
		}
		return hand[bestIdx], bestIdx
	}
	if len(roulettes) > 0 {
		return hand[roulettes[0]], roulettes[0]
	}
	return hand[playable[0]], playable[0]
}

func botPickAggressiveNoMercy(hand []unoCard, topColor unoColor, playable []int) (unoCard, int) {
	var draws, actions, others []int

	for _, i := range playable {
		c := hand[i]
		switch {
		case isDrawCard(c.Value):
			draws = append(draws, i)
		case c.Value.isAction():
			actions = append(actions, i)
		default:
			others = append(others, i)
		}
	}

	// Aggressive: play highest draw card first
	if len(draws) > 0 {
		bestIdx := draws[0]
		bestVal := cardDrawValue(hand[bestIdx].Value)
		for _, i := range draws[1:] {
			dv := cardDrawValue(hand[i].Value)
			if dv > bestVal {
				bestIdx = i
				bestVal = dv
			}
		}
		return hand[bestIdx], bestIdx
	}
	if len(actions) > 0 {
		return hand[actions[0]], actions[0]
	}
	if len(others) > 0 {
		return hand[others[0]], others[0]
	}
	return hand[playable[0]], playable[0]
}

// botChooseSwapTarget picks the player with the fewest cards (to steal a small hand).
func botChooseSwapTarget(game *unoMultiGame, bot *unoMultiPlayer) *unoMultiPlayer {
	var best *unoMultiPlayer
	bestCards := 999
	for _, p := range game.players {
		if p == bot || !p.active {
			continue
		}
		if len(p.hand) < bestCards {
			bestCards = len(p.hand)
			best = p
		}
	}
	return best
}

// botRouletteColor picks a color the bot has least of (to maximize damage).
func botRouletteColor(hand []unoCard) unoColor {
	counts := map[unoColor]int{}
	for _, c := range hand {
		if c.Color != unoWild {
			counts[c.Color]++
		}
	}
	// Pick color with fewest cards (opponent likely has fewer too)
	best := unoRed
	bestCount := 999
	for _, color := range []unoColor{unoRed, unoBlue, unoYellow, unoGreen} {
		if counts[color] < bestCount {
			bestCount = counts[color]
			best = color
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// No Mercy commentary
// ---------------------------------------------------------------------------

var noMercyCommentary = map[string][]string{
	"nomercy_start": {
		"*GogoBee sets the book down. Not gently.*\n\n\"No mercy? Fine. 💛\"\n\n*deals cards with unsettling precision*",
		"*GogoBee marks the page, closes the book with a snap.*\n\n\"Oh, you want to play *that* version. Okay. 💛\"\n\n*shuffles 168 cards without breaking eye contact*",
	},
	"mercy_kill": {
		"\"Sometimes the kindest thing is the quickest. 💛\" *doesn't look up*",
		"\"That's what mercy looks like. 💛\"",
		"\"...and that's why they call it No Mercy. 💛\"",
	},
	"stack_absorbed": {
		"\"That's a lot of cards. 💛\" *turns a page*",
		"\"Ouch. 💛\"",
	},
	"color_roulette": {
		"\"Let's see what fate has in store. 💛\"",
		"\"Flip, flip, flip... 💛\" *watches with mild interest*",
	},
	"discard_all": {
		"\"Oh, that's efficient. 💛\"",
	},
	"skip_everyone": {
		"\"Nobody gets a turn. How fun. 💛\"",
	},
	"hand_swap": {
		"\"Musical chairs, card edition. 💛\"",
		"\"Surprise. 💛\"",
	},
	"hand_rotate": {
		"\"Everyone pass your cards. Yes, all of them. 💛\"",
	},
}

func pickNoMercyCommentary(key string) string {
	lines := noMercyCommentary[key]
	if len(lines) == 0 {
		return ""
	}
	line := lines[rand.IntN(len(lines))]
	return strings.ReplaceAll(line, "GogoBee", unoBotName())
}

// ---------------------------------------------------------------------------
// No Mercy mode flags parser
// ---------------------------------------------------------------------------

// parseNoMercyFlags parses "nomercy [7-0] €amount" and returns noMercy, sevenZeroRule, and remaining amount string.
func parseNoMercyFlags(args string) (noMercy bool, sevenZeroRule bool, amountStr string) {
	lower := strings.ToLower(strings.TrimSpace(args))

	if !strings.HasPrefix(lower, "nomercy") {
		return false, false, args
	}

	rest := strings.TrimSpace(args[7:]) // len("nomercy") == 7
	lowerRest := strings.ToLower(rest)

	if strings.HasPrefix(lowerRest, "7-0") {
		return true, true, strings.TrimSpace(rest[3:])
	}

	return true, false, rest
}

