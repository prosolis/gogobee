package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
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
		{Name: "emojiboard", Description: "Top emoji givers, receivers, and most used emojis", Usage: "!emojiboard [@user | received [@user] | givers <emoji> | week]", Category: "Reactions"},
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

// roomDisplayName returns a human-readable room name, falling back to the room ID.
func (p *ReactionsPlugin) roomDisplayName(roomID id.RoomID) string {
	var nameEvt event.RoomNameEventContent
	err := p.Client.StateEvent(context.Background(), roomID, event.StateRoomName, "", &nameEvt)
	if err == nil && nameEvt.Name != "" {
		return nameEvt.Name
	}
	return string(roomID)
}

func (p *ReactionsPlugin) handleEmojiboard(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "emojiboard"))
	lower := strings.ToLower(args)

	switch {
	case args == "":
		return p.emojiboardTop(ctx, 0)
	case lower == "week":
		return p.emojiboardTop(ctx, 7)
	case strings.HasPrefix(lower, "received"):
		rest := args[len("received"):]
		return p.emojiboardReceived(ctx, strings.TrimSpace(rest))
	case strings.HasPrefix(lower, "givers "):
		rest := args[len("givers "):]
		return p.emojiboardGivers(ctx, strings.TrimSpace(rest))
	default:
		// Treat as @user lookup (emojis given by user)
		return p.emojiboardUser(ctx, args)
	}
}

// emojiboardTop shows the top 10 most-used reaction emojis in the room.
// If days > 0, scopes to that many days.
func (p *ReactionsPlugin) emojiboardTop(ctx MessageContext, days int) error {
	d := db.Get()
	roomID := string(ctx.RoomID)

	var timeClause string
	var queryArgs []any
	queryArgs = append(queryArgs, roomID)
	if days > 0 {
		cutoff := time.Now().Unix() - int64(days*86400)
		timeClause = " AND created_at >= ?"
		queryArgs = append(queryArgs, cutoff)
	}

	// Get total reactions
	var total int
	row := d.QueryRow(
		`SELECT COALESCE(COUNT(*), 0) FROM reaction_log WHERE room_id = ?`+timeClause,
		queryArgs...,
	)
	if err := row.Scan(&total); err != nil {
		slog.Error("reactions: emojiboard total query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}

	if total == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No reaction data yet.")
	}

	// Get top 10 emojis
	rows, err := d.Query(
		`SELECT emoji, COUNT(*) as cnt FROM reaction_log WHERE room_id = ?`+timeClause+
			` GROUP BY emoji ORDER BY cnt DESC LIMIT 10`,
		queryArgs...,
	)
	if err != nil {
		slog.Error("reactions: emojiboard query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	defer rows.Close()

	roomName := p.roomDisplayName(ctx.RoomID)
	var sb strings.Builder

	header := "🏆 Emoji Leaderboard"
	if days > 0 {
		header += fmt.Sprintf(" (last %d days)", days)
	}
	sb.WriteString(fmt.Sprintf("%s — %s\n\n", header, roomName))

	i := 0
	for rows.Next() {
		var emoji string
		var cnt int
		if err := rows.Scan(&emoji, &cnt); err != nil {
			continue
		}
		pct := cnt * 100 / total
		sb.WriteString(fmt.Sprintf("%d. %s — %s (%d%%)\n", i+1, emoji, formatNumber(cnt), pct))
		i++
	}

	sb.WriteString(fmt.Sprintf("\nTotal reactions: %s\n", formatNumber(total)))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// emojiboardUser shows the top emojis given by a specific user.
func (p *ReactionsPlugin) emojiboardUser(ctx MessageContext, userArg string) error {
	userID, ok := p.ResolveUser(userArg, ctx.RoomID)
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", userArg))
	}

	d := db.Get()
	roomID := string(ctx.RoomID)

	// Total for this user in this room
	var total int
	row := d.QueryRow(
		`SELECT COALESCE(COUNT(*), 0) FROM reaction_log WHERE room_id = ? AND sender = ?`,
		roomID, string(userID),
	)
	if err := row.Scan(&total); err != nil {
		slog.Error("reactions: emojiboard user total", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}

	if total == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s has no reactions in this room.", p.DisplayName(userID)))
	}

	rows, err := d.Query(
		`SELECT emoji, COUNT(*) as cnt FROM reaction_log WHERE room_id = ? AND sender = ? GROUP BY emoji ORDER BY cnt DESC LIMIT 10`,
		roomID, string(userID),
	)
	if err != nil {
		slog.Error("reactions: emojiboard user query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 Emojis Given by %s — %s\n\n", p.DisplayName(userID), p.roomDisplayName(ctx.RoomID)))

	i := 0
	for rows.Next() {
		var emoji string
		var cnt int
		if err := rows.Scan(&emoji, &cnt); err != nil {
			continue
		}
		pct := cnt * 100 / total
		sb.WriteString(fmt.Sprintf("%d. %s — %s (%d%%)\n", i+1, emoji, formatNumber(cnt), pct))
		i++
	}

	sb.WriteString(fmt.Sprintf("\nTotal reactions given: %s\n", formatNumber(total)))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// emojiboardReceived shows the top emojis received by a user.
func (p *ReactionsPlugin) emojiboardReceived(ctx MessageContext, userArg string) error {
	// If no user specified, default to the sender
	var userID id.UserID
	if userArg == "" {
		userID = ctx.Sender
	} else {
		var ok bool
		userID, ok = p.ResolveUser(userArg, ctx.RoomID)
		if !ok {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not resolve user: %s", userArg))
		}
	}

	d := db.Get()
	roomID := string(ctx.RoomID)

	var total int
	row := d.QueryRow(
		`SELECT COALESCE(COUNT(*), 0) FROM reaction_log WHERE room_id = ? AND target_user = ?`,
		roomID, string(userID),
	)
	if err := row.Scan(&total); err != nil {
		slog.Error("reactions: emojiboard received total", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}

	if total == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s has received no reactions in this room.", p.DisplayName(userID)))
	}

	rows, err := d.Query(
		`SELECT emoji, COUNT(*) as cnt FROM reaction_log WHERE room_id = ? AND target_user = ? GROUP BY emoji ORDER BY cnt DESC LIMIT 10`,
		roomID, string(userID),
	)
	if err != nil {
		slog.Error("reactions: emojiboard received query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 Emojis Received by %s — %s\n\n", p.DisplayName(userID), p.roomDisplayName(ctx.RoomID)))

	i := 0
	for rows.Next() {
		var emoji string
		var cnt int
		if err := rows.Scan(&emoji, &cnt); err != nil {
			continue
		}
		pct := cnt * 100 / total
		sb.WriteString(fmt.Sprintf("%d. %s — %s (%d%%)\n", i+1, emoji, formatNumber(cnt), pct))
		i++
	}

	sb.WriteString(fmt.Sprintf("\nTotal reactions received: %s\n", formatNumber(total)))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// emojiboardGivers shows the top 5 users who used a specific emoji most.
func (p *ReactionsPlugin) emojiboardGivers(ctx MessageContext, emoji string) error {
	if emoji == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !emojiboard givers <emoji>")
	}

	d := db.Get()
	roomID := string(ctx.RoomID)

	var total int
	row := d.QueryRow(
		`SELECT COALESCE(COUNT(*), 0) FROM reaction_log WHERE room_id = ? AND emoji = ?`,
		roomID, emoji,
	)
	if err := row.Scan(&total); err != nil {
		slog.Error("reactions: emojiboard givers total", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}

	if total == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No one has used %s in this room.", emoji))
	}

	rows, err := d.Query(
		`SELECT sender, COUNT(*) as cnt FROM reaction_log WHERE room_id = ? AND emoji = ? GROUP BY sender ORDER BY cnt DESC LIMIT 5`,
		roomID, emoji,
	)
	if err != nil {
		slog.Error("reactions: emojiboard givers query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load emojiboard.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 Top %s Givers — %s\n\n", emoji, p.roomDisplayName(ctx.RoomID)))

	i := 0
	for rows.Next() {
		var sender string
		var cnt int
		if err := rows.Scan(&sender, &cnt); err != nil {
			continue
		}
		pct := cnt * 100 / total
		sb.WriteString(fmt.Sprintf("%d. %s — %s (%d%%)\n", i+1, p.DisplayName(id.UserID(sender)), formatNumber(cnt), pct))
		i++
	}

	sb.WriteString(fmt.Sprintf("\nTotal %s reactions: %s\n", emoji, formatNumber(total)))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
