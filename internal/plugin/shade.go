package plugin

import (
	"os"

	"maunium.net/go/mautrix"
)

// ShadePlugin is a stub plugin for future shade/roast features.
type ShadePlugin struct {
	Base
	enabled bool
}

// NewShadePlugin creates a new shade plugin.
func NewShadePlugin(client *mautrix.Client) *ShadePlugin {
	return &ShadePlugin{
		Base:    NewBase(client),
		enabled: os.Getenv("FEATURE_SHADE") == "true" || os.Getenv("FEATURE_SHADE") == "1",
	}
}

func (p *ShadePlugin) Name() string { return "shade" }

func (p *ShadePlugin) Commands() []CommandDef {
	if !p.enabled {
		return nil
	}
	return []CommandDef{
		{Name: "shade", Description: "Throw some shade (coming soon)", Usage: "!shade", Category: "LLM & Sentiment"},
	}
}

func (p *ShadePlugin) Init() error { return nil }

func (p *ShadePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *ShadePlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}

	if p.IsCommand(ctx.Body, "shade") {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Coming soon.")
	}

	return nil
}
