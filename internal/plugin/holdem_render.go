package plugin

import (
	"fmt"
	"strings"

	"github.com/chehsunliu/poker"
)

// cardGlyphMap maps poker card strings (e.g. "As") to Unicode glyphs.
var cardGlyphMap map[string]string

func init() {
	cardGlyphMap = make(map[string]string, 52)

	// Unicode playing card block. Q and K skip a codepoint (Knight sits between J and Q).
	// Spades: U+1F0A1..
	// Hearts: U+1F0B1..
	// Diamonds: U+1F0C1..
	// Clubs: U+1F0D1..
	type suitInfo struct {
		letter string
		base   rune
	}
	suits := []suitInfo{
		{"s", 0x1F0A0}, // Spades
		{"h", 0x1F0B0}, // Hearts
		{"d", 0x1F0C0}, // Diamonds
		{"c", 0x1F0D0}, // Clubs
	}

	// Rank offsets within each suit block.
	// A=1, 2=2, ..., 10=10, J=11, Q=13(skip Knight at 12), K=14
	rankOffsets := map[string]int{
		"A": 1, "2": 2, "3": 3, "4": 4, "5": 5,
		"6": 6, "7": 7, "8": 8, "9": 9, "T": 10,
		"J": 11, "Q": 13, "K": 14,
	}

	for _, s := range suits {
		for rank, offset := range rankOffsets {
			cardStr := rank + s.letter
			glyph := string(rune(s.base + rune(offset)))
			cardGlyphMap[cardStr] = glyph
		}
	}
}

// holdemSuitSymbols maps poker library suit letters to display symbols.
var holdemSuitSymbols = map[byte]string{
	's': "♠", 'h': "♥", 'd': "♦", 'c': "♣",
}

// rankDisplay converts library rank chars to display. "T" -> "10", rest unchanged.
var rankDisplay = map[byte]string{
	'2': "2", '3': "3", '4': "4", '5': "5", '6': "6",
	'7': "7", '8': "8", '9': "9", 'T': "10",
	'J': "J", 'Q': "Q", 'K': "K", 'A': "A",
}

// renderCard renders a card as "🂡 (A♠)".
func renderCard(c poker.Card) string {
	s := c.String() // e.g. "As", "Td"
	glyph := cardGlyphMap[s]
	if glyph == "" {
		glyph = "🂠"
	}
	rank := rankDisplay[s[0]]
	suit := holdemSuitSymbols[s[1]]
	return fmt.Sprintf("%s (%s%s)", glyph, rank, suit)
}

// renderCards renders multiple cards separated by double space.
func renderCards(cards []poker.Card) string {
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = renderCard(c)
	}
	return strings.Join(parts, "  ")
}

// renderHoleCards renders a 2-card hole hand.
func renderHoleCards(hole [2]poker.Card) string {
	return renderCard(hole[0]) + "  " + renderCard(hole[1])
}

// renderTableView builds the DM table view for a specific player.
func renderTableView(g *HoldemGame, viewerIdx int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🎰 **Texas Hold'em** | %s\n", g.Street.String()))

	// Pot info.
	if len(g.SidePots) > 0 {
		total := int64(0)
		for _, sp := range g.SidePots {
			total += sp.Amount
		}
		sb.WriteString(fmt.Sprintf("Pot: €%d", total))
		for i, sp := range g.SidePots {
			sb.WriteString(fmt.Sprintf(" | Side %d: €%d", i+1, sp.Amount))
		}
		sb.WriteString("\n")
	} else {
		// Include outstanding bets in displayed pot.
		totalPot := g.Pot
		for _, p := range g.Players {
			totalPot += p.Bet
		}
		sb.WriteString(fmt.Sprintf("Pot: €%d\n", totalPot))
	}

	// Board.
	if len(g.Community) > 0 {
		sb.WriteString(fmt.Sprintf("Board: %s\n", renderCards(g.Community)))
	} else {
		sb.WriteString("Board: —\n")
	}

	sb.WriteString("\n**Seats:**\n")

	for i, p := range g.Players {
		if p.State == PlayerSatOut {
			sb.WriteString(fmt.Sprintf("  %s — *(sitting out)*\n", p.DisplayName))
			continue
		}

		marker := ""
		if i == g.DealerIdx {
			marker = " 🔘"
		}

		actionMarker := ""
		if i == g.ActionIdx && g.HandInProgress && g.Street != StreetShowdown {
			actionMarker = " ← action"
		}

		switch p.State {
		case PlayerFolded:
			sb.WriteString(fmt.Sprintf("  %s%s — €%d [folded]%s\n", p.DisplayName, marker, p.Stack, actionMarker))
		case PlayerAllIn:
			sb.WriteString(fmt.Sprintf("  %s%s — €%d [ALL IN]%s\n", p.DisplayName, marker, p.Stack, actionMarker))
		default:
			betStr := ""
			if p.Bet > 0 {
				betStr = fmt.Sprintf(" | bet: €%d", p.Bet)
			}
			sb.WriteString(fmt.Sprintf("  %s%s — €%d%s%s\n", p.DisplayName, marker, p.Stack, betStr, actionMarker))
		}
	}

	// If viewer is the action player, show action prompt.
	if viewerIdx == g.ActionIdx && g.HandInProgress && g.Street != StreetShowdown {
		p := g.Players[viewerIdx]
		if p.State == PlayerActive {
			sb.WriteString("\n")
			toCall := g.CurrentBet - p.Bet
			if toCall > p.Stack {
				toCall = p.Stack
			}
			if toCall > 0 {
				minRaiseTo := g.CurrentBet + g.MinRaise
				sb.WriteString(fmt.Sprintf("To call: €%d | Min raise to: €%d\n", toCall, minRaiseTo))
				sb.WriteString("`!holdem call` `!holdem raise <amount>` `!holdem allin` `!holdem fold`\n")
			} else {
				minRaiseTo := g.CurrentBet + g.MinRaise
				if g.CurrentBet == 0 {
					minRaiseTo = g.BigBlind
				}
				sb.WriteString(fmt.Sprintf("Check available | Min bet: €%d\n", minRaiseTo))
				sb.WriteString("`!holdem check` `!holdem raise <amount>` `!holdem allin` `!holdem fold`\n")
			}
		}
	}

	return sb.String()
}

// renderPrivateHand builds the private hand DM for a player.
func renderPrivateHand(g *HoldemGame, playerIdx int) string {
	p := g.Players[playerIdx]
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🃏 Your hand: %s\n", renderHoleCards(p.Hole)))

	if len(g.Community) > 0 {
		sb.WriteString(fmt.Sprintf("Board: %s\n", renderCards(g.Community)))
	}

	totalPot := g.Pot
	for _, pp := range g.Players {
		totalPot += pp.Bet
	}

	toCall := g.CurrentBet - p.Bet
	if toCall < 0 {
		toCall = 0
	}
	if toCall > p.Stack {
		toCall = p.Stack
	}

	sb.WriteString(fmt.Sprintf("Stack: €%d | Pot: €%d | To call: €%d\n", p.Stack, totalPot, toCall))

	return sb.String()
}

// renderActionAnnouncement formats an action for room/DM announcement.
func renderActionAnnouncement(name, action string, amount int64) string {
	switch action {
	case "fold":
		return fmt.Sprintf("**%s** folds.", name)
	case "check":
		return fmt.Sprintf("**%s** checks.", name)
	case "call":
		return fmt.Sprintf("**%s** calls €%d.", name, amount)
	case "raise":
		return fmt.Sprintf("**%s** raises to €%d.", name, amount)
	case "allin":
		return fmt.Sprintf("**%s** is ALL IN for €%d! 🚨", name, amount)
	default:
		return fmt.Sprintf("**%s** acts.", name)
	}
}

// renderWinnerAnnouncement formats a winner message.
func renderWinnerAnnouncement(name string, amount int64, handName string, showCards bool, hole [2]poker.Card) string {
	if showCards {
		return fmt.Sprintf("🏆 **%s** wins €%d with %s! %s", name, amount, handName, renderHoleCards(hole))
	}
	return fmt.Sprintf("🏆 **%s** wins €%d!", name, amount)
}

// renderShowdownLine formats one player's showdown result.
func renderShowdownLine(name string, hole [2]poker.Card, handName string, won int64) string {
	if won > 0 {
		return fmt.Sprintf("  %s: %s — %s (won €%d)", name, renderHoleCards(hole), handName, won)
	}
	return fmt.Sprintf("  %s: %s — %s", name, renderHoleCards(hole), handName)
}

// renderUncalledBetReturn formats the uncalled bet message.
func renderUncalledBetReturn(name string, amount int64) string {
	return fmt.Sprintf("↩️ Uncalled bet of €%d returned to **%s**.", amount, name)
}

// renderStartAnnouncement formats the hand start message for the room.
func renderStartAnnouncement(g *HoldemGame) string {
	var sb strings.Builder
	sb.WriteString("🎰 **Texas Hold'em** — Hand starting!\n\n")

	sb.WriteString("**Players:**\n")
	for i, p := range g.Players {
		if p.State == PlayerSatOut {
			continue
		}
		pos := g.positionLabel(i)
		sb.WriteString(fmt.Sprintf("  %s (%s) — €%d\n", p.DisplayName, pos, p.Stack))
	}

	sb.WriteString(fmt.Sprintf("\nBlinds: €%d / €%d\n", g.SmallBlind, g.BigBlind))
	return sb.String()
}

// renderEndAnnouncement formats the hand end message for the room.
func renderEndAnnouncement(results []showdownResult, g *HoldemGame) string {
	var sb strings.Builder
	sb.WriteString("🎰 **Texas Hold'em** — Hand complete!\n\n")

	if len(g.Community) > 0 {
		sb.WriteString(fmt.Sprintf("Board: %s\n\n", renderCards(g.Community)))
	}

	if len(results) > 0 {
		sb.WriteString("**Results:**\n")
		for _, r := range results {
			sb.WriteString(r.line + "\n")
		}
	}

	sb.WriteString("\n**Stacks:**\n")
	for _, p := range g.Players {
		if p.State == PlayerSatOut {
			continue
		}
		delta := p.Stack - p.OpeningStack
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		sb.WriteString(fmt.Sprintf("  %s — €%d (%s%d)\n", p.DisplayName, p.Stack, sign, delta))
	}

	return sb.String()
}

// renderHelpMessage returns the help text for !holdem help.
func renderHelpMessage() string {
	return "🎰 **Texas Hold'em Commands**\n\n" +
		"**Lobby:**\n" +
		"`!holdem join` — Sit down at the table\n" +
		"`!holdem leave` — Leave the table\n" +
		"`!holdem start` — Start dealing (≥2 players)\n" +
		"`!holdem addbot` — Add an AI opponent\n\n" +
		"**In-Game (your turn):**\n" +
		"`!holdem fold` — Fold your hand\n" +
		"`!holdem check` — Check (no bet to call)\n" +
		"`!holdem call` — Call the current bet\n" +
		"`!holdem raise <amount>` — Raise to a total of €amount\n" +
		"`!holdem allin` — Go all-in\n\n" +
		"**Other:**\n" +
		"`!holdem status` — Get current table state (DM)\n" +
		"`!holdem help` — Show this message\n\n" +
		"**DM-Only:**\n" +
		"`!holdem tips on/off` — Toggle coaching tips\n"
}

// showdownResult holds one player's showdown display line.
type showdownResult struct {
	line string
}
