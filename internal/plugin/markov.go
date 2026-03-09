package plugin

import (
	"fmt"
	"log/slog"
	"math/rand"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// MarkovPlugin collects messages and generates trigram-based text.
type MarkovPlugin struct {
	Base
}

// NewMarkovPlugin creates a new Markov chain plugin.
func NewMarkovPlugin(client *mautrix.Client) *MarkovPlugin {
	return &MarkovPlugin{
		Base: NewBase(client),
	}
}

func (p *MarkovPlugin) Name() string { return "markov" }

func (p *MarkovPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "markov", Description: "Generate Markov chain text from a user's messages", Usage: "!markov [@user|me]", Category: "Fun & Games"},
	}
}

func (p *MarkovPlugin) Init() error { return nil }

func (p *MarkovPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *MarkovPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "markov") {
		return p.handleMarkov(ctx)
	}

	// Passive: collect non-command messages
	if !ctx.IsCommand {
		p.collectMessage(ctx.Sender, ctx.Body)
	}

	return nil
}

// collectMessage stores a message in the markov_corpus, capping at 10,000 per user.
func (p *MarkovPlugin) collectMessage(userID id.UserID, text string) {
	// Skip very short messages
	if len(strings.Fields(text)) < 3 {
		return
	}

	d := db.Get()

	_, err := d.Exec(
		`INSERT INTO markov_corpus (user_id, text) VALUES (?, ?)`,
		string(userID), text,
	)
	if err != nil {
		slog.Error("markov: insert message", "err", err)
		return
	}

	// Cap at 10,000 messages per user — delete oldest excess
	_, err = d.Exec(
		`DELETE FROM markov_corpus WHERE user_id = ? AND id NOT IN (
			SELECT id FROM markov_corpus WHERE user_id = ? ORDER BY id DESC LIMIT 10000
		)`,
		string(userID), string(userID),
	)
	if err != nil {
		slog.Error("markov: prune corpus", "err", err)
	}
}

func (p *MarkovPlugin) handleMarkov(ctx MessageContext) error {
	d := db.Get()
	args := p.GetArgs(ctx.Body, "markov")

	var targetUser id.UserID

	switch {
	case args == "":
		// No argument — pick a random user from the corpus
		var randomUser string
		err := d.QueryRow(
			`SELECT user_id FROM markov_corpus ORDER BY RANDOM() LIMIT 1`,
		).Scan(&randomUser)
		if err != nil {
			return p.SendReply(ctx.RoomID, ctx.EventID, "No Markov data available yet.")
		}
		targetUser = id.UserID(randomUser)
	case args == "me":
		targetUser = ctx.Sender
	default:
		// Treat as user ID
		cleaned := strings.TrimSpace(strings.TrimPrefix(args, "@"))
		targetUser = id.UserID(cleaned)
	}

	// Fetch corpus for the user
	rows, err := d.Query(
		`SELECT text FROM markov_corpus WHERE user_id = ?`,
		string(targetUser),
	)
	if err != nil {
		slog.Error("markov: query corpus", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load Markov data.")
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			continue
		}
		texts = append(texts, t)
	}

	if len(texts) < 10 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Not enough data for %s (need at least 10 messages).", string(targetUser)))
	}

	// Build trigram model and generate
	result := generateMarkov(texts, 50)
	if result == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to generate Markov text.")
	}

	msg := fmt.Sprintf("[%s]: %s", string(targetUser), result)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// trigram key
type trigramKey struct {
	w1, w2 string
}

// generateMarkov builds a trigram model from texts and generates output.
func generateMarkov(texts []string, maxWords int) string {
	chain := make(map[trigramKey][]string)
	var starters []trigramKey

	for _, text := range texts {
		words := strings.Fields(text)
		if len(words) < 3 {
			continue
		}

		starters = append(starters, trigramKey{words[0], words[1]})

		for i := 0; i < len(words)-2; i++ {
			key := trigramKey{words[i], words[i+1]}
			chain[key] = append(chain[key], words[i+2])
		}
	}

	if len(starters) == 0 {
		return ""
	}

	// Pick a random starter
	start := starters[rand.Intn(len(starters))]
	result := []string{start.w1, start.w2}

	for len(result) < maxWords {
		key := trigramKey{result[len(result)-2], result[len(result)-1]}
		nextWords, ok := chain[key]
		if !ok || len(nextWords) == 0 {
			break
		}
		next := nextWords[rand.Intn(len(nextWords))]
		result = append(result, next)

		// Stop at sentence-ending punctuation sometimes
		if len(result) > 8 && endsWithPunctuation(next) && rand.Float64() < 0.3 {
			break
		}
	}

	return strings.Join(result, " ")
}

func endsWithPunctuation(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '.' || last == '!' || last == '?'
}
