package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// UserPlugin provides personal utility commands: timezone, quotes, now-playing, backlog, keyword watches.
type UserPlugin struct {
	Base
}

// NewUserPlugin creates a new user plugin.
func NewUserPlugin(client *mautrix.Client) *UserPlugin {
	return &UserPlugin{
		Base: NewBase(client),
	}
}

func (p *UserPlugin) Name() string { return "user" }

func (p *UserPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "settz", Description: "Set your timezone (IANA format)", Usage: "!settz America/New_York", Category: "Personal"},
		{Name: "mytz", Description: "Show your timezone and current time", Usage: "!mytz", Category: "Personal"},
		{Name: "timezone", Description: "List common timezones", Usage: "!timezone list", Category: "Personal"},
		{Name: "quote", Description: "Show a random saved quote from this room", Usage: "!quote", Category: "Personal"},
		{Name: "np", Description: "Set or show now playing", Usage: "!np [track]", Category: "Personal"},
		{Name: "backlog", Description: "Manage your personal backlog", Usage: "!backlog add/list/random/done", Category: "Personal"},
		{Name: "watch", Description: "Watch a keyword for DM alerts", Usage: "!watch <keyword>", Category: "Personal"},
		{Name: "watching", Description: "List your keyword watches", Usage: "!watching", Category: "Personal"},
		{Name: "unwatch", Description: "Remove a keyword watch", Usage: "!unwatch <keyword>", Category: "Personal"},
	}
}

func (p *UserPlugin) Init() error { return nil }

func (p *UserPlugin) OnReaction(ctx ReactionContext) error {
	if ctx.Emoji != "\u2B50" { // ⭐
		return nil
	}

	// Fetch the target event to get the quoted message
	evt, err := p.Client.GetEvent(context.Background(), ctx.RoomID, ctx.TargetEvent)
	if err != nil {
		slog.Error("user: fetch event for quote", "err", err)
		return nil
	}

	if err := evt.Content.ParseRaw(evt.Type); err != nil {
		slog.Error("user: parse event content for quote", "err", err)
		return nil
	}

	content := evt.Content.AsMessage()
	if content == nil || content.Body == "" {
		return nil
	}

	d := db.Get()
	_, err = d.Exec(
		`INSERT INTO quotes (room_id, user_id, quote_text, saved_by) VALUES (?, ?, ?, ?)`,
		string(ctx.RoomID), string(evt.Sender), content.Body, string(ctx.Sender),
	)
	if err != nil {
		slog.Error("user: save quote", "err", err)
		return nil
	}

	return p.SendReact(ctx.RoomID, ctx.EventID, "\u2705") // ✅
}

func (p *UserPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "settz"):
		return p.handleSetTZ(ctx)
	case p.IsCommand(ctx.Body, "mytz"):
		return p.handleMyTZ(ctx)
	case p.IsCommand(ctx.Body, "timezone"):
		return p.handleTimezoneList(ctx)
	case p.IsCommand(ctx.Body, "quote"):
		return p.handleQuote(ctx)
	case p.IsCommand(ctx.Body, "np"):
		return p.handleNP(ctx)
	case p.IsCommand(ctx.Body, "backlog"):
		return p.handleBacklog(ctx)
	case p.IsCommand(ctx.Body, "watch"):
		return p.handleWatch(ctx)
	case p.IsCommand(ctx.Body, "watching"):
		return p.handleWatching(ctx)
	case p.IsCommand(ctx.Body, "unwatch"):
		return p.handleUnwatch(ctx)
	}

	// Passive: check keyword watches
	p.checkKeywordWatches(ctx)

	return nil
}

func (p *UserPlugin) handleSetTZ(ctx MessageContext) error {
	tz := strings.TrimSpace(p.GetArgs(ctx.Body, "settz"))
	if tz == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !settz <IANA timezone> (e.g. America/New_York)")
	}

	_, err := time.LoadLocation(tz)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid timezone: %s. Use IANA format like America/New_York.", tz))
	}

	d := db.Get()
	_, err = d.Exec(
		`INSERT INTO birthdays (user_id, month, day, timezone) VALUES (?, 0, 0, ?)
		 ON CONFLICT(user_id) DO UPDATE SET timezone = ?`,
		string(ctx.Sender), tz, tz,
	)
	if err != nil {
		slog.Error("user: set timezone", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save timezone.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Timezone set to %s.", tz))
}

func (p *UserPlugin) handleMyTZ(ctx MessageContext) error {
	d := db.Get()
	var tz string
	err := d.QueryRow(
		`SELECT timezone FROM birthdays WHERE user_id = ?`,
		string(ctx.Sender),
	).Scan(&tz)
	if err != nil || tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
		tz = "UTC"
	}

	now := time.Now().In(loc)
	msg := fmt.Sprintf("Your timezone: %s\nCurrent time: %s", tz, now.Format("Monday, January 2, 2006 3:04 PM"))
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *UserPlugin) handleTimezoneList(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "timezone"))
	if args != "list" {
		return nil
	}

	zones := []string{
		"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
		"America/Sao_Paulo", "Europe/London", "Europe/Paris", "Europe/Berlin",
		"Asia/Tokyo", "Asia/Shanghai", "Asia/Kolkata", "Asia/Seoul",
		"Australia/Sydney", "Pacific/Auckland", "UTC",
	}

	var sb strings.Builder
	sb.WriteString("Common timezones:\n\n")
	for _, z := range zones {
		loc, err := time.LoadLocation(z)
		if err != nil {
			continue
		}
		now := time.Now().In(loc)
		sb.WriteString(fmt.Sprintf("  %s — %s\n", z, now.Format("3:04 PM")))
	}
	sb.WriteString("\nSet yours with: !settz <timezone>")

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *UserPlugin) handleQuote(ctx MessageContext) error {
	d := db.Get()
	var quoteText, userID string
	err := d.QueryRow(
		`SELECT quote_text, user_id FROM quotes WHERE room_id = ? ORDER BY RANDOM() LIMIT 1`,
		string(ctx.RoomID),
	).Scan(&quoteText, &userID)
	if err == sql.ErrNoRows {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No quotes saved in this room yet. React with a star to save one!")
	}
	if err != nil {
		slog.Error("user: random quote", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch a quote.")
	}

	msg := fmt.Sprintf("\"%s\"\n  — %s", quoteText, userID)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *UserPlugin) handleNP(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "np"))
	d := db.Get()

	if args == "" {
		// Show current now playing
		var track string
		var updatedAt int64
		err := d.QueryRow(
			`SELECT track, updated_at FROM now_playing WHERE user_id = ?`,
			string(ctx.Sender),
		).Scan(&track, &updatedAt)
		if err == sql.ErrNoRows {
			return p.SendReply(ctx.RoomID, ctx.EventID, "You don't have anything playing. Use !np <track> to set it.")
		}
		if err != nil {
			slog.Error("user: get np", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch now playing.")
		}
		t := time.Unix(updatedAt, 0).UTC()
		msg := fmt.Sprintf("Now playing for %s: %s (set %s)", string(ctx.Sender), track, t.Format("Jan 2 3:04 PM UTC"))
		return p.SendReply(ctx.RoomID, ctx.EventID, msg)
	}

	// Set now playing
	now := time.Now().UTC().Unix()
	_, err := d.Exec(
		`INSERT INTO now_playing (user_id, track, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET track = ?, updated_at = ?`,
		string(ctx.Sender), args, now, args, now,
	)
	if err != nil {
		slog.Error("user: set np", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save now playing.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Now playing: %s", args))
}

func (p *UserPlugin) handleBacklog(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "backlog"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !backlog add <item> | list | random | done <id>")
	}

	d := db.Get()

	if strings.HasPrefix(strings.ToLower(args), "add ") {
		item := strings.TrimSpace(args[4:])
		if item == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Please provide an item to add.")
		}
		_, err := d.Exec(
			`INSERT INTO backlog (user_id, item) VALUES (?, ?)`,
			string(ctx.Sender), item,
		)
		if err != nil {
			slog.Error("user: backlog add", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add to backlog.")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Added to backlog: %s", item))
	}

	if strings.ToLower(args) == "list" {
		rows, err := d.Query(
			`SELECT id, item FROM backlog WHERE user_id = ? AND done = 0 ORDER BY created_at DESC`,
			string(ctx.Sender),
		)
		if err != nil {
			slog.Error("user: backlog list", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load backlog.")
		}
		defer rows.Close()

		var sb strings.Builder
		sb.WriteString("Your backlog:\n\n")
		count := 0
		for rows.Next() {
			var itemID int
			var item string
			if err := rows.Scan(&itemID, &item); err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", itemID, item))
			count++
		}
		if count == 0 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Your backlog is empty!")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
	}

	if strings.ToLower(args) == "random" {
		var itemID int
		var item string
		err := d.QueryRow(
			`SELECT id, item FROM backlog WHERE user_id = ? AND done = 0 ORDER BY RANDOM() LIMIT 1`,
			string(ctx.Sender),
		).Scan(&itemID, &item)
		if err == sql.ErrNoRows {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Your backlog is empty!")
		}
		if err != nil {
			slog.Error("user: backlog random", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to pick from backlog.")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Random pick from your backlog: [%d] %s", itemID, item))
	}

	if strings.HasPrefix(strings.ToLower(args), "done ") {
		idStr := strings.TrimSpace(args[5:])
		result, err := d.Exec(
			`UPDATE backlog SET done = 1 WHERE id = ? AND user_id = ?`,
			idStr, string(ctx.Sender),
		)
		if err != nil {
			slog.Error("user: backlog done", "err", err)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to mark as done.")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Item not found or not yours.")
		}
		return p.SendReply(ctx.RoomID, ctx.EventID, "Marked as done!")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !backlog add <item> | list | random | done <id>")
}

func (p *UserPlugin) handleWatch(ctx MessageContext) error {
	keyword := strings.ToLower(strings.TrimSpace(p.GetArgs(ctx.Body, "watch")))
	if keyword == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !watch <keyword>")
	}

	d := db.Get()
	_, err := d.Exec(
		`INSERT OR IGNORE INTO keyword_watches (user_id, keyword, room_id) VALUES (?, ?, ?)`,
		string(ctx.Sender), keyword, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("user: add watch", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add keyword watch.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Watching for \"%s\". You'll get a DM when it's mentioned.", keyword))
}

func (p *UserPlugin) handleWatching(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT keyword FROM keyword_watches WHERE user_id = ?`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("user: list watches", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watches.")
	}
	defer rows.Close()

	var keywords []string
	for rows.Next() {
		var kw string
		if err := rows.Scan(&kw); err != nil {
			continue
		}
		keywords = append(keywords, kw)
	}

	if len(keywords) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not watching any keywords. Use !watch <keyword> to start.")
	}

	msg := "Your keyword watches:\n\n"
	for _, kw := range keywords {
		msg += fmt.Sprintf("  - %s\n", kw)
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *UserPlugin) handleUnwatch(ctx MessageContext) error {
	keyword := strings.ToLower(strings.TrimSpace(p.GetArgs(ctx.Body, "unwatch")))
	if keyword == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !unwatch <keyword>")
	}

	d := db.Get()
	result, err := d.Exec(
		`DELETE FROM keyword_watches WHERE user_id = ? AND keyword = ?`,
		string(ctx.Sender), keyword,
	)
	if err != nil {
		slog.Error("user: remove watch", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove keyword watch.")
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("You weren't watching \"%s\".", keyword))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Stopped watching \"%s\".", keyword))
}

func (p *UserPlugin) checkKeywordWatches(ctx MessageContext) {
	// Skip bot commands
	if ctx.IsCommand {
		return
	}

	d := db.Get()
	bodyLower := strings.ToLower(ctx.Body)

	rows, err := d.Query(`SELECT user_id, keyword FROM keyword_watches WHERE room_id = ?`, string(ctx.RoomID))
	if err != nil {
		slog.Error("user: check watches", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var watcherID, keyword string
		if err := rows.Scan(&watcherID, &keyword); err != nil {
			continue
		}

		// Don't alert the sender about their own messages
		if id.UserID(watcherID) == ctx.Sender {
			continue
		}

		if strings.Contains(bodyLower, keyword) {
			preview := ctx.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			dmMsg := fmt.Sprintf("Keyword alert: \"%s\" was mentioned by %s in %s:\n\n%s",
				keyword, string(ctx.Sender), string(ctx.RoomID), preview)

			// Use a goroutine to avoid blocking message dispatch
			go func(uid id.UserID, msg string) {
				if err := p.SendDM(uid, msg); err != nil {
					slog.Error("user: send watch DM", "err", err, "user", uid)
				}
			}(id.UserID(watcherID), dmMsg)

		}
	}
}
