package plugin

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

var countdownLabelRe = regexp.MustCompile(`"([^"]+)"\s+(\d{4}-\d{2}-\d{2})`)

// CountdownPlugin manages countdown timers to specific dates.
type CountdownPlugin struct {
	Base
}

// NewCountdownPlugin creates a new countdown plugin.
func NewCountdownPlugin(client *mautrix.Client) *CountdownPlugin {
	return &CountdownPlugin{
		Base: NewBase(client),
	}
}

func (p *CountdownPlugin) Name() string { return "countdown" }

func (p *CountdownPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "countdown", Description: "Manage countdowns to dates", Usage: "!countdown [add/private/mine/remove/id]", Category: "Personal"},
	}
}

func (p *CountdownPlugin) Init() error {
	// Auto-complete old countdowns
	p.completeOldCountdowns()
	return nil
}

func (p *CountdownPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *CountdownPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "countdown") {
		return nil
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "countdown"))

	switch {
	case strings.HasPrefix(strings.ToLower(args), "add "):
		return p.handleAdd(ctx, args[4:], true)
	case strings.HasPrefix(strings.ToLower(args), "private "):
		return p.handleAdd(ctx, args[8:], false)
	case strings.ToLower(args) == "mine":
		return p.handleMine(ctx)
	case strings.HasPrefix(strings.ToLower(args), "remove "):
		return p.handleRemove(ctx, strings.TrimSpace(args[7:]))
	case args == "":
		return p.handleList(ctx)
	default:
		// Try to parse as an ID
		if _, err := strconv.Atoi(args); err == nil {
			return p.handleShow(ctx, args)
		}
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: !countdown add \"<label>\" <YYYY-MM-DD> | private \"<label>\" <YYYY-MM-DD> | mine | remove <id> | [id]")
	}
}

func (p *CountdownPlugin) handleAdd(ctx MessageContext, input string, public bool) error {
	matches := countdownLabelRe.FindStringSubmatch(input)
	if matches == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: !countdown add \"<label>\" <YYYY-MM-DD>")
	}

	label := matches[1]
	dateStr := matches[2]

	_, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Invalid date format. Use YYYY-MM-DD.")
	}

	d := db.Get()
	isPublic := 1
	if !public {
		isPublic = 0
	}

	_, err = d.Exec(
		`INSERT INTO countdowns (user_id, room_id, label, target_date, public) VALUES (?, ?, ?, ?, ?)`,
		string(ctx.Sender), string(ctx.RoomID), label, dateStr, isPublic,
	)
	if err != nil {
		slog.Error("countdown: add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add countdown.")
	}

	visibility := "public"
	if !public {
		visibility = "private"
	}
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Countdown added (%s): \"%s\" on %s", visibility, label, dateStr))
}

func (p *CountdownPlugin) handleMine(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT id, label, target_date, public FROM countdowns
		 WHERE user_id = ? AND completed = 0
		 ORDER BY target_date ASC`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("countdown: mine", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your countdowns.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Your countdowns:\n\n")
	count := 0
	now := time.Now().UTC()

	for rows.Next() {
		var cdID int
		var label, targetDate string
		var public int
		if err := rows.Scan(&cdID, &label, &targetDate, &public); err != nil {
			continue
		}
		t, _ := time.Parse("2006-01-02", targetDate)
		days := int(t.Sub(now).Hours() / 24)
		vis := ""
		if public == 0 {
			vis = " (private)"
		}
		status := fmt.Sprintf("%d days", days)
		if days < 0 {
			status = fmt.Sprintf("%d days ago", -days)
		} else if days == 0 {
			status = "TODAY!"
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s — %s (%s)%s\n", cdID, label, targetDate, status, vis))
		count++
	}

	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You have no active countdowns.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *CountdownPlugin) handleRemove(ctx MessageContext, idStr string) error {
	d := db.Get()
	result, err := d.Exec(
		`DELETE FROM countdowns WHERE id = ? AND user_id = ?`,
		idStr, string(ctx.Sender),
	)
	if err != nil {
		slog.Error("countdown: remove", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove countdown.")
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Countdown not found or not yours.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, "Countdown removed.")
}

func (p *CountdownPlugin) handleList(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT id, label, target_date, user_id FROM countdowns
		 WHERE public = 1 AND completed = 0
		 ORDER BY target_date ASC LIMIT 20`,
	)
	if err != nil {
		slog.Error("countdown: list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load countdowns.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Active countdowns:\n\n")
	count := 0
	now := time.Now().UTC()

	for rows.Next() {
		var cdID int
		var label, targetDate, userID string
		if err := rows.Scan(&cdID, &label, &targetDate, &userID); err != nil {
			continue
		}
		t, _ := time.Parse("2006-01-02", targetDate)
		days := int(t.Sub(now).Hours() / 24)
		status := fmt.Sprintf("%d days", days)
		if days < 0 {
			status = fmt.Sprintf("%d days ago", -days)
		} else if days == 0 {
			status = "TODAY!"
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s — %s (%s) by %s\n", cdID, label, targetDate, status, userID))
		count++
	}

	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active countdowns. Add one with !countdown add \"<label>\" <date>")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *CountdownPlugin) handleShow(ctx MessageContext, idStr string) error {
	d := db.Get()
	var label, targetDate, userID string
	var public int
	err := d.QueryRow(
		`SELECT label, target_date, user_id, public FROM countdowns WHERE id = ? AND completed = 0`,
		idStr,
	).Scan(&label, &targetDate, &userID, &public)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Countdown not found.")
	}

	// Private countdowns only visible to owner
	if public == 0 && userID != string(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Countdown not found.")
	}

	t, _ := time.Parse("2006-01-02", targetDate)
	now := time.Now().UTC()
	diff := t.Sub(now)

	var status string
	if diff < 0 {
		absDays := int(-diff.Hours()) / 24
		status = fmt.Sprintf("%d days ago", absDays)
	} else {
		days := int(diff.Hours()) / 24
		hours := int(diff.Hours()) % 24
		if days == 0 {
			status = fmt.Sprintf("%d hours remaining!", hours)
		} else {
			status = fmt.Sprintf("%d days and %d hours", days, hours)
		}
	}

	msg := fmt.Sprintf("\"%s\" — %s\n%s (by %s)", label, targetDate, status, userID)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *CountdownPlugin) completeOldCountdowns() {
	d := db.Get()
	cutoff := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")
	_, err := d.Exec(
		`UPDATE countdowns SET completed = 1 WHERE target_date < ? AND completed = 0`,
		cutoff,
	)
	if err != nil {
		slog.Error("countdown: auto-complete", "err", err)
	}
}
