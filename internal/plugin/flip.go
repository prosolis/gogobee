package plugin

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"gogobee/internal/db"
)

// FlipPlugin handles !flip (coin flip) restricted to the games room.
type FlipPlugin struct {
	Base
}

func NewFlipPlugin(client *mautrix.Client) *FlipPlugin {
	return &FlipPlugin{Base: NewBase(client)}
}

func (p *FlipPlugin) Name() string { return "flip" }

func (p *FlipPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "flip", Description: "Flip a coin", Usage: "!flip", Category: "Games"},
		{Name: "games", Description: "List available games", Usage: "!games", Category: "Games"},
		{Name: "twinbeeboard", Description: "GogoBee's victory record", Usage: "!twinbeeboard", Category: "Games"},
	}
}

func (p *FlipPlugin) Init() error                        { return nil }
func (p *FlipPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *FlipPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "flip"):
		if !isGamesRoom(ctx.RoomID) {
			gr := gamesRoom()
			if gr != "" {
				return p.SendReply(ctx.RoomID, ctx.EventID, "Games are only available in the games channel!")
			}
		}
		return p.handleFlip(ctx)
	case p.IsCommand(ctx.Body, "games"):
		return p.handleGames(ctx)
	case p.IsCommand(ctx.Body, "twinbeeboard"):
		return p.handleLoseboard(ctx)
	}
	return nil
}

func (p *FlipPlugin) handleFlip(ctx MessageContext) error {
	if rand.IntN(2) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "🪙 **Heads!**")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, "🪙 **Tails!**")
}

func (p *FlipPlugin) handleGames(ctx MessageContext) error {
	gr := gamesRoom()
	roomNote := ""
	if gr != "" {
		roomNote = "\n\nAll games are played in the games channel."
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		"🎮 **Available Games**\n\n"+
			"**!flip** — Coin flip\n"+
			"**!hangman start** — Collaborative Hangman\n"+
			"**!blackjack €amount** — Blackjack (1-4 players vs dealer)\n"+
			"**!uno €amount** — UNO (solo or multiplayer, classic or No Mercy)\n"+
			"**!trivia** — Trivia questions\n\n"+
			"**Economy:**\n"+
			"**!balance** — Check your euros\n"+
			"**!baltop** — Euro leaderboard\n"+
			"**!baltransfer @user €amount** — Send euros\n"+
			"**!hangboard** — Hangman leaderboard\n"+
			"**!bjboard** — Blackjack leaderboard\n"+
			"**!twinbeeboard** — Bot defeat leaderboard"+
			roomNote)
}

func (p *FlipPlugin) handleLoseboard(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT user_id, game, losses FROM bot_defeats
		 WHERE losses > 0
		 ORDER BY losses DESC LIMIT 15`)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load twinbeeboard.")
	}
	defer rows.Close()

	// Aggregate total losses per user and track per-game breakdown
	type userStats struct {
		total     int
		breakdown map[string]int
	}
	users := make(map[string]*userStats)
	var order []string

	for rows.Next() {
		var uid, game string
		var losses int
		if err := rows.Scan(&uid, &game, &losses); err != nil {
			continue
		}
		if _, ok := users[uid]; !ok {
			users[uid] = &userStats{breakdown: make(map[string]int)}
			order = append(order, uid)
		}
		users[uid].total += losses
		users[uid].breakdown[game] = losses
	}

	if len(order) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "🐝 No victories yet. GogoBee is patient.")
	}

	// Sort by total losses descending
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if users[order[j]].total > users[order[i]].total {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	if len(order) > 10 {
		order = order[:10]
	}

	gameLabels := map[string]string{
		"blackjack": "BJ",
		"uno":       "UNO",
		"uno_multi": "UNO MP",
	}

	var sb strings.Builder
	sb.WriteString("🐝 **GogoBee's Trophy Wall**\n\n")
	for i, uid := range order {
		name := p.flipDisplayName(id.UserID(uid))
		stats := users[uid]
		var parts []string
		for game, count := range stats.breakdown {
			label := gameLabels[game]
			if label == "" {
				label = game
			}
			parts = append(parts, fmt.Sprintf("%s: %d", label, count))
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** — %d losses (%s)\n",
			i+1, name, stats.total, strings.Join(parts, ", ")))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// recordBotDefeat increments the bot defeat counter for a user in a specific game.
// This is a package-level function so all game plugins can call it.
func recordBotDefeat(userID id.UserID, game string) {
	d := db.Get()
	_, _ = d.Exec(
		`INSERT INTO bot_defeats (user_id, game, losses)
		 VALUES (?, ?, 1)
		 ON CONFLICT(user_id, game) DO UPDATE SET
		   losses = losses + 1`,
		string(userID), game,
	)
}

func (p *FlipPlugin) flipDisplayName(userID id.UserID) string {
	resp, err := p.Client.GetDisplayName(context.Background(), userID)
	if err != nil || resp.DisplayName == "" {
		return string(userID)
	}
	return resp.DisplayName
}

// redirectToGamesRoom returns the room ID for games-restricted redirect.
func redirectToGamesRoom(sender id.UserID) string {
	_ = sender
	return "Games are only available in the games channel!"
}
