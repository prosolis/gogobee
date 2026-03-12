package plugin

import (
	"math/rand/v2"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// FlipPlugin handles !flip (coin flip) restricted to the games room.
type FlipPlugin struct {
	Base
}

func NewFlipPlugin(client *mautrix.Client) *FlipPlugin {
	return &FlipPlugin{Base: Base{Client: client}}
}

func (p *FlipPlugin) Name() string { return "flip" }

func (p *FlipPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "flip", Description: "Flip a coin", Usage: "!flip", Category: "Games"},
		{Name: "games", Description: "List available games", Usage: "!games", Category: "Games"},
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
			"**!blackjack €amount** — Blackjack (1-2 players vs dealer)\n"+
			"**!trivia** — Trivia questions\n\n"+
			"**Economy:**\n"+
			"**!balance** — Check your euros\n"+
			"**!baltop** — Euro leaderboard\n"+
			"**!baltransfer @user €amount** — Send euros\n"+
			"**!hangboard** — Hangman leaderboard\n"+
			"**!bjboard** — Blackjack leaderboard"+
			roomNote)
}

// redirectToGamesRoom returns the room ID for games-restricted redirect.
func redirectToGamesRoom(sender id.UserID) string {
	_ = sender
	return "Games are only available in the games channel!"
}
