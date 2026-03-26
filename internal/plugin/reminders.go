package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"github.com/google/uuid"
	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// RemindersPlugin handles user reminders with natural language time parsing.
type RemindersPlugin struct {
	Base
	parser *when.Parser
}

// NewRemindersPlugin creates a new RemindersPlugin.
func NewRemindersPlugin(client *mautrix.Client) *RemindersPlugin {
	w := when.New(nil)
	w.Add(en.All...)
	w.Add(common.All...)
	return &RemindersPlugin{
		Base:   NewBase(client),
		parser: w,
	}
}

func (p *RemindersPlugin) Name() string { return "reminders" }

func (p *RemindersPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "remindme", Description: "Set a reminder", Usage: "!remindme <time expression> <message>", Category: "Personal"},
		{Name: "reminders", Description: "List your pending reminders", Usage: "!reminders", Category: "Personal"},
		{Name: "unremind", Description: "Cancel a reminder", Usage: "!unremind <id>", Category: "Personal"},
	}
}

func (p *RemindersPlugin) Init() error { return nil }

func (p *RemindersPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *RemindersPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "remindme"):
		return p.handleRemindMe(ctx)
	case p.IsCommand(ctx.Body, "reminders"):
		return p.handleListReminders(ctx)
	case p.IsCommand(ctx.Body, "unremind"):
		return p.handleUnremind(ctx)
	}
	return nil
}

func (p *RemindersPlugin) handleRemindMe(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "remindme")
	if args == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !remindme <time expression> <message>\nExamples: !remindme in 30 minutes check the oven, !remindme tomorrow at 9am meeting")
	}

	result, err := p.parser.Parse(args, time.Now())
	if err != nil || result == nil {
		return p.SendMessage(ctx.RoomID, "I couldn't understand that time expression. Try something like: in 30 minutes, tomorrow at 9am, in 2 hours")
	}

	fireAt := result.Time
	if fireAt.Before(time.Now()) {
		return p.SendMessage(ctx.RoomID, "That time is in the past! Please specify a future time.")
	}

	// Extract message: everything not part of the time expression
	message := strings.TrimSpace(args[:result.Index] + args[result.Index+len(result.Text):])
	if message == "" {
		message = "Reminder!"
	}

	reminderID := uuid.New().String()[:8]

	_, err = db.Get().Exec(
		`INSERT INTO reminders (id, user_id, room_id, message, fire_at, fired)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		reminderID, string(ctx.Sender), string(ctx.RoomID), message, fireAt.Unix(),
	)
	if err != nil {
		slog.Error("reminders: insert", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to save reminder.")
	}

	durStr := formatDuration(time.Until(fireAt))
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("⏰ Reminder set! I'll remind you %s (ID: %s)\n\"%s\"", durStr, reminderID, message))
}

func (p *RemindersPlugin) handleListReminders(ctx MessageContext) error {
	rows, err := db.Get().Query(
		`SELECT id, message, fire_at
		 FROM reminders
		 WHERE user_id = ? AND fired = 0
		 ORDER BY fire_at ASC
		 LIMIT 20`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("reminders: query", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch reminders.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("📋 Your pending reminders:\n")
	found := false
	for rows.Next() {
		var reminderID, message string
		var fireAt int64
		if err := rows.Scan(&reminderID, &message, &fireAt); err != nil {
			continue
		}
		found = true
		t := time.Unix(fireAt, 0)
		sb.WriteString(fmt.Sprintf("  • [%s] %s — %s\n", reminderID, message, t.Format("Jan 2 15:04 MST")))
	}

	if !found {
		return p.SendMessage(ctx.RoomID, "You have no pending reminders.")
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}

func (p *RemindersPlugin) handleUnremind(ctx MessageContext) error {
	reminderID := strings.TrimSpace(p.GetArgs(ctx.Body, "unremind"))
	if reminderID == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !unremind <id>")
	}

	result, err := db.Get().Exec(
		`DELETE FROM reminders WHERE id = ? AND user_id = ? AND fired = 0`,
		reminderID, string(ctx.Sender),
	)
	if err != nil {
		slog.Error("reminders: delete", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to cancel reminder.")
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return p.SendMessage(ctx.RoomID, "Reminder not found or already fired.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("🗑️ Reminder %s cancelled.", reminderID))
}

// FirePendingReminders checks for due reminders and sends them. Called by the scheduler.
func FirePendingReminders(client *mautrix.Client) {
	now := time.Now().Unix()
	rows, err := db.Get().Query(
		`SELECT id, user_id, room_id, message
		 FROM reminders
		 WHERE fired = 0 AND fire_at <= ?`,
		now,
	)
	if err != nil {
		slog.Error("reminders: query pending", "err", err)
		return
	}
	defer rows.Close()

	base := NewBase(client)

	// Collect all pending reminders first, then close the rows.
	// Iterating rows while also writing to the DB can cause SQLite lock issues.
	type pendingReminder struct {
		ID, UserID, RoomID, Message string
	}
	var pending []pendingReminder
	for rows.Next() {
		var r pendingReminder
		if err := rows.Scan(&r.ID, &r.UserID, &r.RoomID, &r.Message); err != nil {
			slog.Error("reminders: scan row", "err", err)
			continue
		}
		pending = append(pending, r)
	}
	rows.Close()

	for _, r := range pending {
		// Mark fired BEFORE sending so a crash doesn't re-fire on restart.
		_, err := db.Get().Exec(`UPDATE reminders SET fired = 1 WHERE id = ?`, r.ID)
		if err != nil {
			slog.Error("reminders: mark fired", "err", err, "id", r.ID)
			continue
		}

		msg := fmt.Sprintf("⏰ Reminder for %s: %s", r.UserID, r.Message)
		if err := base.SendMessage(id.RoomID(r.RoomID), msg); err != nil {
			slog.Error("reminders: send reminder", "err", err, "id", r.ID)
		}
	}
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("in %d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "in 1 minute"
		}
		return fmt.Sprintf("in %d minutes", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			if hours == 1 {
				return "in 1 hour"
			}
			return fmt.Sprintf("in %d hours", hours)
		}
		return fmt.Sprintf("in %d hours and %d minutes", hours, mins)
	}
	days := int(d.Hours()) / 24
	if days == 1 {
		return "in 1 day"
	}
	return fmt.Sprintf("in %d days", days)
}
