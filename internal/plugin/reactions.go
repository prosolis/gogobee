package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// ReactionsPlugin logs reactions and provides emoji usage statistics.
type ReactionsPlugin struct {
	Base
}

// NewReactionsPlugin creates a new reactions plugin.
func NewReactionsPlugin(client *mautrix.Client) *ReactionsPlugin {
	return &ReactionsPlugin{
		Base: NewBase(client),
	}
}

func (p *ReactionsPlugin) Name() string { return "reactions" }

func (p *ReactionsPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "emojiboard", Description: "Top emoji givers, receivers, and most used emojis", Usage: "!emojiboard", Category: "Reactions"},
	}
}

func (p *ReactionsPlugin) Init() error { return nil }

func (p *ReactionsPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "emojiboard") {
		return p.handleEmojiboard(ctx)
	}
	return nil
}

func (p *ReactionsPlugin) OnReaction(ctx ReactionContext) error {
	// Resolve the target event's sender via Matrix API
	targetUser, err := p.resolveEventSender(ctx.RoomID, ctx.TargetEvent)
	if err != nil {
		slog.Error("reactions: resolve target event sender", "err", err, "event", ctx.TargetEvent)
		return nil // Don't fail the whole handler
	}

	// Don't log self-reactions
	if ctx.Sender == targetUser {
		return nil
	}

	db.Exec("reactions: log reaction",
		`INSERT INTO reaction_log (room_id, event_id, sender, target_user, emoji) VALUES (?, ?, ?, ?, ?)`,
		string(ctx.RoomID), string(ctx.TargetEvent), string(ctx.Sender), string(targetUser), ctx.Emoji,
	)

	return nil
}

// resolveEventSender fetches an event from the Matrix API to determine its sender.
func (p *ReactionsPlugin) resolveEventSender(roomID id.RoomID, eventID id.EventID) (id.UserID, error) {
	evt, err := p.Client.GetEvent(context.Background(), roomID, eventID)
	if err != nil {
		return "", fmt.Errorf("get event %s: %w", eventID, err)
	}
	return evt.Sender, nil
}

func (p *ReactionsPlugin) handleEmojiboard(ctx MessageContext) error {
	members := p.RoomMembers(ctx.RoomID)

	d := db.Get()
	var sb strings.Builder

	// Top 10 emoji givers
	sb.WriteString("--- Top 10 Emoji Givers ---\n\n")
	rows, err := d.Query(
		`SELECT sender, COUNT(*) as cnt FROM reaction_log GROUP BY sender ORDER BY cnt DESC`,
	)
	if err != nil {
		slog.Error("reactions: givers query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	giverCount := appendUserBoard(&sb, rows, members)
	rows.Close()

	// Top 10 emoji receivers
	sb.WriteString("\n--- Top 10 Emoji Receivers ---\n\n")
	rows, err = d.Query(
		`SELECT target_user, COUNT(*) as cnt FROM reaction_log GROUP BY target_user ORDER BY cnt DESC`,
	)
	if err != nil {
		slog.Error("reactions: receivers query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	receiverCount := appendUserBoard(&sb, rows, members)
	rows.Close()

	// Top 10 most used emojis (no user filtering needed)
	sb.WriteString("\n--- Top 10 Most Used Emojis ---\n\n")
	rows, err = d.Query(
		`SELECT emoji, COUNT(*) as cnt FROM reaction_log GROUP BY emoji ORDER BY cnt DESC LIMIT 10`,
	)
	if err != nil {
		slog.Error("reactions: emoji query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	emojiCount := appendEmojiBoard(&sb, rows)
	rows.Close()

	if giverCount == 0 && receiverCount == 0 && emojiCount == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No reaction data yet.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// appendUserBoard writes ranked user lines from query rows, filtered by room members.
func appendUserBoard(sb *strings.Builder, rows *sql.Rows, members map[id.UserID]bool) int {
	medals := []string{"🥇", "🥈", "🥉"}
	i := 0
	for rows.Next() && i < 10 {
		var name string
		var cnt int
		if err := rows.Scan(&name, &cnt); err != nil {
			continue
		}
		if members != nil && !members[id.UserID(name)] {
			continue
		}
		prefix := fmt.Sprintf("#%d", i+1)
		if i < len(medals) {
			prefix = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s — %s reactions\n", prefix, name, formatNumber(cnt)))
		i++
	}
	return i
}

// appendEmojiBoard writes ranked emoji lines.
func appendEmojiBoard(sb *strings.Builder, rows *sql.Rows) int {
	i := 0
	for rows.Next() {
		var emoji string
		var cnt int
		if err := rows.Scan(&emoji, &cnt); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("%d. %s — %s times\n", i+1, emoji, formatNumber(cnt)))
		i++
	}
	return i
}
