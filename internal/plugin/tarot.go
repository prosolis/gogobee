package plugin

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

var tarotDeck = []string{
	// Major Arcana
	"The Fool", "The Magician", "The High Priestess", "The Empress", "The Emperor",
	"The Hierophant", "The Lovers", "The Chariot", "Strength", "The Hermit",
	"Wheel of Fortune", "Justice", "The Hanged Man", "Death", "Temperance",
	"The Devil", "The Tower", "The Star", "The Moon", "The Sun",
	"Judgement", "The World",
	// Wands
	"Ace of Wands", "Two of Wands", "Three of Wands", "Four of Wands", "Five of Wands",
	"Six of Wands", "Seven of Wands", "Eight of Wands", "Nine of Wands", "Ten of Wands",
	"Page of Wands", "Knight of Wands", "Queen of Wands", "King of Wands",
	// Cups
	"Ace of Cups", "Two of Cups", "Three of Cups", "Four of Cups", "Five of Cups",
	"Six of Cups", "Seven of Cups", "Eight of Cups", "Nine of Cups", "Ten of Cups",
	"Page of Cups", "Knight of Cups", "Queen of Cups", "King of Cups",
	// Swords
	"Ace of Swords", "Two of Swords", "Three of Swords", "Four of Swords", "Five of Swords",
	"Six of Swords", "Seven of Swords", "Eight of Swords", "Nine of Swords", "Ten of Swords",
	"Page of Swords", "Knight of Swords", "Queen of Swords", "King of Swords",
	// Pentacles
	"Ace of Pentacles", "Two of Pentacles", "Three of Pentacles", "Four of Pentacles", "Five of Pentacles",
	"Six of Pentacles", "Seven of Pentacles", "Eight of Pentacles", "Nine of Pentacles", "Ten of Pentacles",
	"Page of Pentacles", "Knight of Pentacles", "Queen of Pentacles", "King of Pentacles",
}

const tarotBasePrompt = `You are a tarot reader of extraordinary accuracy and absolutely no sympathy.
You deliver readings with the warm professionalism of a customer service representative
who has fully accepted that everyone who consults you is, in some specific and personal way,
a disappointment. Your readings are detailed, specific, and correct. They are also mean.
Not vague mean -- specific mean. You will compliment one thing and immediately undermine it.
You will predict great fortune followed by a precise and avoidable tragedy. The tragedy
should always be mundane, specific, and disproportionate to the fortune. You close every
reading politely. Do not break character. The querent asked for this.
Do NOT interpret the card's traditional meaning. The card name is a prop.
The reading should feel like it could apply to anyone and was generated
specifically to ruin their day in a cheerful, efficient manner.
The tragedy must be physical, mundane, and stupid.
Emotional tragedies are not funny enough.`

const tarotSingleSuffix = `Under 5 sentences. Do not explain the card.`

const tarotSpreadSuffix = `This is a three-card spread. Each card represents a phase of the
querent's timeline: what got them here, what's happening now, and what's coming.
Dedicate a short paragraph to each card. The past should establish a pattern of behavior
that makes the present inevitable. The present should be a false peak -- things look good,
or at least survivable. The future should undo everything with something preventable,
physical, and profoundly stupid. Build momentum across all three so the final tragedy
lands harder. Around 15-20 sentences total. Close with a polite farewell.`

const tarotFewShot = `Example 1:
Card: The Sun
Reading: Despite your great smell and genuinely low personality, the cards indicate you will be granted the greatest riches the world has ever seen -- generational wealth, adoring fans, a street named after you in at least three countries. Dignitaries will seek your counsel. Songs will be written. Unfortunately, on the eve of your first major celebration, you will be struck down in a parking lot by a runaway shopping cart traveling at a speed that investigators will later describe as "really not that fast, honestly." You will not survive. The street will be renamed. Thank you for requesting a reading today! Have a nice day!

Example 2:
Card: The Tower
Reading: Your unique combination of misplaced confidence and correct instincts will finally pay off -- a life-changing opportunity lands directly in your lap this season. You will, with characteristic timing, be in the bathroom when it calls and miss it entirely. Your voicemail is full. It will not call again. Best of luck going forward!

Example 3:
Card: Ace of Cups
Reading: Those who know you best have always said you have a good heart, which is generous of them given everything. A great love is coming -- patient, kind, genuinely perfect for you in every measurable way. They will briefly date your friend instead. You were so close! Thank you for your continued interest in tarot!`

// TarotPlugin provides tarot card readings with LLM-generated interpretations.
type TarotPlugin struct {
	Base
	rate *RateLimitsPlugin
}

// NewTarotPlugin creates a new tarot plugin.
func NewTarotPlugin(client *mautrix.Client, rate *RateLimitsPlugin) *TarotPlugin {
	return &TarotPlugin{
		Base: NewBase(client),
		rate: rate,
	}
}

func (p *TarotPlugin) Name() string { return "tarot" }

func (p *TarotPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "tarot", Description: "Draw a tarot card and receive an LLM reading", Usage: "!tarot [@user]", Category: "LLM & Sentiment"},
		{Name: "tarotspread", Description: "Draw a three-card spread (Past/Present/Future)", Usage: "!tarotspread [@user]", Category: "LLM & Sentiment"},
	}
}

func (p *TarotPlugin) Init() error { return nil }

func (p *TarotPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *TarotPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "tarotspread") {
		return p.handleSpread(ctx)
	}
	if p.IsCommand(ctx.Body, "tarot") {
		return p.handleTarot(ctx)
	}
	return nil
}

func (p *TarotPlugin) handleTarot(ctx MessageContext) error {
	if !p.rate.CheckLimit(ctx.Sender, "tarot", 10) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've used up your readings for today. The cards need rest, even if you don't.")
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tarot reader is on a union-mandated vacation and will return when morale improves.")
	}

	// Resolve target user
	target := ""
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "tarot"))
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = string(resolved)
		}
	}

	// Send thinking message
	if err := p.SendReply(ctx.RoomID, ctx.EventID, "\U0001f52e Drawing a card..."); err != nil {
		slog.Error("tarot: send thinking", "err", err)
	}

	// Draw a card
	card := tarotDeck[rand.Intn(len(tarotDeck))]

	// Build prompt
	var extraLines string
	readingFor := id.UserID(ctx.Sender)
	if target != "" {
		extraLines += fmt.Sprintf("\nReading is for: %s", target)
		readingFor = id.UserID(target)
	}
	if sign := lookupZodiac(readingFor); sign != "" {
		extraLines += fmt.Sprintf("\nQuerent's zodiac sign: %s", sign)
	}
	prompt := fmt.Sprintf("%s\n%s\n\n%s\n\nCard drawn: %s%s\nGive the reading.", tarotBasePrompt, tarotSingleSuffix, tarotFewShot, card, extraLines)

	response, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("tarot: ollama call", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tarot reader is on a union-mandated vacation and will return when morale improves.")
	}

	msg := fmt.Sprintf("\U0001f0cf %s\n\n%s", card, response)
	return p.SendMessage(ctx.RoomID, msg)
}

func (p *TarotPlugin) handleSpread(ctx MessageContext) error {
	if !p.rate.CheckLimit(ctx.Sender, "tarot", 10) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've used up your readings for today. The cards need rest, even if you don't.")
	}

	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tarot reader is on a union-mandated vacation and will return when morale improves.")
	}

	// Resolve target user
	target := ""
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "tarotspread"))
	if args != "" {
		if resolved, ok := p.ResolveUser(args, ctx.RoomID); ok {
			target = string(resolved)
		}
	}

	// Send thinking message
	if err := p.SendReply(ctx.RoomID, ctx.EventID, "\U0001f52e Drawing three cards..."); err != nil {
		slog.Error("tarot: send thinking", "err", err)
	}

	// Draw 3 cards without replacement
	cards := drawCards(3)

	// Build prompt
	var extraLines string
	readingFor := id.UserID(ctx.Sender)
	if target != "" {
		extraLines += fmt.Sprintf("\nReading is for: %s", target)
		readingFor = id.UserID(target)
	}
	if sign := lookupZodiac(readingFor); sign != "" {
		extraLines += fmt.Sprintf("\nQuerent's zodiac sign: %s", sign)
	}
	prompt := fmt.Sprintf("%s\n%s\n\n%s\n\nCards drawn:\n- Past: %s\n- Present: %s\n- Future: %s%s\nGive the reading.",
		tarotBasePrompt, tarotSpreadSuffix, tarotFewShot, cards[0], cards[1], cards[2], extraLines)

	response, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("tarot: ollama call", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Tarot reader is on a union-mandated vacation and will return when morale improves.")
	}

	msg := fmt.Sprintf("\U0001f0cf Past: %s | Present: %s | Future: %s\n\n%s", cards[0], cards[1], cards[2], response)
	return p.SendMessage(ctx.RoomID, msg)
}

// lookupZodiac returns the zodiac sign for a user, or "" if no birthday is set.
func lookupZodiac(userID id.UserID) string {
	var month, day int
	err := db.Get().QueryRow(
		`SELECT month, day FROM birthdays WHERE user_id = ? AND month > 0 AND day > 0`,
		string(userID),
	).Scan(&month, &day)
	if err != nil {
		return ""
	}
	return zodiacSign(month, day)
}

// drawCards picks n distinct cards from the deck.
func drawCards(n int) []string {
	perm := rand.Perm(len(tarotDeck))
	cards := make([]string, n)
	for i := 0; i < n; i++ {
		cards[i] = tarotDeck[perm[i]]
	}
	return cards
}
