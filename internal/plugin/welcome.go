package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

// WelcomeRegistry is the interface for retrieving all registered commands (for !help).
type WelcomeRegistry interface {
	GetCommands() []CommandDef
}

// WelcomePlugin detects new users and provides the !help command.
type WelcomePlugin struct {
	Base
	xp       *XPPlugin
	registry WelcomeRegistry
}

// NewWelcomePlugin creates a new welcome plugin.
func NewWelcomePlugin(client *mautrix.Client, xp *XPPlugin, registry WelcomeRegistry) *WelcomePlugin {
	return &WelcomePlugin{
		Base:     NewBase(client),
		xp:       xp,
		registry: registry,
	}
}

func (p *WelcomePlugin) Name() string { return "welcome" }

func (p *WelcomePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "help", Description: "DM you a list of all bot commands", Usage: "!help", Category: "Info"},
	}
}

func (p *WelcomePlugin) Init() error { return nil }

func (p *WelcomePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *WelcomePlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "help") {
		return p.handleHelp(ctx)
	}

	// Passive: detect new users by checking for "welcome_wagon" achievement
	p.checkNewUser(ctx)

	return nil
}

func (p *WelcomePlugin) checkNewUser(ctx MessageContext) {
	d := db.Get()

	var exists int
	err := d.QueryRow(
		`SELECT 1 FROM achievements WHERE user_id = ? AND achievement_id = 'welcome_wagon'`,
		string(ctx.Sender),
	).Scan(&exists)

	if err == nil {
		// Already has the achievement, not a new user
		return
	}
	if err != sql.ErrNoRows {
		slog.Error("welcome: check achievement", "err", err)
		return
	}

	// New user! Grant achievement
	_, err = d.Exec(
		`INSERT OR IGNORE INTO achievements (user_id, achievement_id) VALUES (?, 'welcome_wagon')`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("welcome: grant achievement", "err", err)
		return
	}

	// Grant 25 XP
	if p.xp != nil {
		p.xp.GrantXP(ctx.Sender, 25, "welcome")
	}

	// Send welcome message
	botName := os.Getenv("BOT_DISPLAY_NAME")
	if botName == "" {
		botName = "GogoBee"
	}
	welcome := fmt.Sprintf(
		"Welcome to the community, %s! I'm %s, your friendly community bot.\n\n"+
			"Here are some things you can do:\n"+
			"  - !help — see all available commands\n"+
			"  - !rank — check your XP and level\n"+
			"  - !streak — see your activity streak\n"+
			"  - !trivia — play trivia games\n\n"+
			"You've been awarded 25 XP as a welcome gift! Have fun!",
		string(ctx.Sender), botName,
	)
	if err := p.SendMessage(ctx.RoomID, welcome); err != nil {
		slog.Error("welcome: send message", "err", err)
	}
}

// categoryOrder defines the display order for help categories.
var categoryOrder = []string{
	"Fun & Games",
	"Leveling & Stats",
	"Entertainment",
	"Lookup & Reference",
	"LLM & Sentiment",
	"Personal",
	"Holidays",
	"Reactions",
	"Info",
}

// categoryEmojis maps categories to display emojis.
var categoryEmojis = map[string]string{
	"Fun & Games":        "🎲",
	"Leveling & Stats":   "📊",
	"Entertainment":      "🎬",
	"Lookup & Reference": "📖",
	"LLM & Sentiment":    "🧠",
	"Personal":           "👤",
	"Holidays":           "🎉",
	"Reactions":          "😎",
	"Info":               "ℹ️",
	"Admin":              "🔧",
}

func (p *WelcomePlugin) handleHelp(ctx MessageContext) error {
	cmds := p.registry.GetCommands()
	if len(cmds) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No commands available.")
	}

	// Group by category
	grouped := make(map[string][]CommandDef)
	var adminCmds []CommandDef
	for _, cmd := range cmds {
		if cmd.AdminOnly {
			adminCmds = append(adminCmds, cmd)
			continue
		}
		cat := cmd.Category
		if cat == "" {
			cat = "Other"
		}
		grouped[cat] = append(grouped[cat], cmd)
	}

	var sb strings.Builder
	helpBotName := os.Getenv("BOT_DISPLAY_NAME")
	if helpBotName == "" {
		helpBotName = "GogoBee"
	}
	sb.WriteString(fmt.Sprintf("%s Commands\n", helpBotName))

	for _, cat := range categoryOrder {
		cmds, ok := grouped[cat]
		if !ok || len(cmds) == 0 {
			continue
		}
		emoji := categoryEmojis[cat]
		sb.WriteString(fmt.Sprintf("\n%s %s\n", emoji, cat))
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("  %s — %s\n", cmd.Usage, cmd.Description))
		}
		delete(grouped, cat)
	}

	// Any uncategorized leftovers
	for cat, cmds := range grouped {
		if len(cmds) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", cat))
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("  %s — %s\n", cmd.Usage, cmd.Description))
		}
	}

	if p.IsAdmin(ctx.Sender) && len(adminCmds) > 0 {
		emoji := categoryEmojis["Admin"]
		sb.WriteString(fmt.Sprintf("\n%s Admin\n", emoji))
		for _, cmd := range adminCmds {
			sb.WriteString(fmt.Sprintf("  %s — %s\n", cmd.Usage, cmd.Description))
		}
	}

	// DM the help message
	if err := p.SendDM(ctx.Sender, sb.String()); err != nil {
		slog.Error("welcome: send help DM", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to send help DM. Do you have DMs enabled?")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, "Help sent to your DMs!")
}
