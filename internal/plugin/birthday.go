package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// BirthdayPlugin manages user birthdays with reminders and announcements.
type BirthdayPlugin struct {
	Base
	xp *XPPlugin
}

// NewBirthdayPlugin creates a new BirthdayPlugin.
func NewBirthdayPlugin(client *mautrix.Client, xp *XPPlugin) *BirthdayPlugin {
	return &BirthdayPlugin{
		Base: NewBase(client),
		xp:   xp,
	}
}

func (p *BirthdayPlugin) Name() string { return "birthday" }

func (p *BirthdayPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "birthday", Description: "Set, show, or remove your birthday", Usage: "!birthday set <MM-DD> | !birthday set <MM-DD-YYYY> | !birthday show | !birthday remove", Category: "Personal"},
		{Name: "birthdays", Description: "Show upcoming birthdays (next 30 days)", Usage: "!birthdays", Category: "Personal"},
	}
}

func (p *BirthdayPlugin) Init() error { return nil }

func (p *BirthdayPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *BirthdayPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "birthdays") {
		return p.handleUpcoming(ctx)
	}
	if p.IsCommand(ctx.Body, "birthday") {
		return p.handleBirthday(ctx)
	}
	return nil
}

func (p *BirthdayPlugin) handleBirthday(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "birthday"))
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !birthday set <MM-DD> | !birthday show | !birthday remove")
	}

	sub := strings.ToLower(parts[0])
	switch sub {
	case "set":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !birthday set <MM-DD> or !birthday set <MM-DD-YYYY>")
		}
		return p.handleSet(ctx, strings.TrimSpace(parts[1]))
	case "show":
		return p.handleShow(ctx)
	case "remove":
		return p.handleRemove(ctx)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !birthday set <MM-DD> | !birthday show | !birthday remove")
	}
}

func (p *BirthdayPlugin) handleSet(ctx MessageContext, dateStr string) error {
	month, day, year, err := parseBirthdayDate(dateStr)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid date format: %s. Use MM-DD or MM-DD-YYYY.", err.Error()))
	}

	// Validate the date makes sense
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Invalid date. Month must be 1-12 and day must be 1-31.")
	}

	// Validate day for the given month
	if year > 0 {
		t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if t.Month() != time.Month(month) {
			return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid date: %s %d doesn't have %d days.", time.Month(month), year, day))
		}
	}

	d := db.Get()
	_, err = d.Exec(
		`INSERT INTO birthdays (user_id, month, day, year, timezone) VALUES (?, ?, ?, ?, 'UTC')
		 ON CONFLICT(user_id) DO UPDATE SET month = ?, day = ?, year = ?`,
		string(ctx.Sender), month, day, year, month, day, year,
	)
	if err != nil {
		slog.Error("birthday: set", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save your birthday.")
	}

	display := fmt.Sprintf("%s %d", time.Month(month), day)
	if year > 0 {
		display += fmt.Sprintf(", %d", year)
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Birthday set to %s!", display))
}

func (p *BirthdayPlugin) handleShow(ctx MessageContext) error {
	d := db.Get()
	var month, day, year int
	err := d.QueryRow(
		`SELECT month, day, year FROM birthdays WHERE user_id = ?`, string(ctx.Sender),
	).Scan(&month, &day, &year)
	if err == sql.ErrNoRows {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You haven't set your birthday yet. Use !birthday set <MM-DD>.")
	}
	if err != nil {
		slog.Error("birthday: show", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch your birthday.")
	}

	display := fmt.Sprintf("%s %d", time.Month(month), day)
	if year > 0 {
		display += fmt.Sprintf(", %d", year)
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Your birthday: %s", display))
}

func (p *BirthdayPlugin) handleRemove(ctx MessageContext) error {
	d := db.Get()
	res, err := d.Exec(`DELETE FROM birthdays WHERE user_id = ?`, string(ctx.Sender))
	if err != nil {
		slog.Error("birthday: remove", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove your birthday.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You don't have a birthday set.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, "Birthday removed.")
}

func (p *BirthdayPlugin) handleUpcoming(ctx MessageContext) error {
	d := db.Get()
	now := time.Now().UTC()

	rows, err := d.Query(`SELECT user_id, month, day, year FROM birthdays ORDER BY month, day`)
	if err != nil {
		slog.Error("birthday: upcoming query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch upcoming birthdays.")
	}
	defer rows.Close()

	type bdayEntry struct {
		UserID string
		Month  int
		Day    int
		Year   int
		DaysTo int
	}

	var upcoming []bdayEntry
	for rows.Next() {
		var userID string
		var month, day, year int
		if err := rows.Scan(&userID, &month, &day, &year); err != nil {
			continue
		}

		daysTo := daysUntilBirthday(now, month, day)
		if daysTo <= 30 {
			upcoming = append(upcoming, bdayEntry{
				UserID: userID,
				Month:  month,
				Day:    day,
				Year:   year,
				DaysTo: daysTo,
			})
		}
	}

	if len(upcoming) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No upcoming birthdays in the next 30 days.")
	}

	// Sort by days until birthday
	for i := 0; i < len(upcoming); i++ {
		for j := i + 1; j < len(upcoming); j++ {
			if upcoming[j].DaysTo < upcoming[i].DaysTo {
				upcoming[i], upcoming[j] = upcoming[j], upcoming[i]
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Upcoming Birthdays (next 30 days):\n")
	for _, b := range upcoming {
		display := fmt.Sprintf("%s %d", time.Month(b.Month), b.Day)
		if b.Year > 0 {
			age := now.Year() - b.Year
			// Adjust if birthday hasn't happened yet this year
			bdayThisYear := time.Date(now.Year(), time.Month(b.Month), b.Day, 0, 0, 0, 0, time.UTC)
			if now.Before(bdayThisYear) {
				age--
			}
			display += fmt.Sprintf(" (turning %d)", age+1)
		}

		when := "TODAY!"
		if b.DaysTo == 1 {
			when = "tomorrow"
		} else if b.DaysTo > 1 {
			when = fmt.Sprintf("in %d days", b.DaysTo)
		}

		sb.WriteString(fmt.Sprintf("  - %s: %s — %s\n", b.UserID, display, when))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// CheckAndPost checks for birthdays today and posts announcements.
func (p *BirthdayPlugin) CheckAndPost(roomID id.RoomID) error {
	now := time.Now().UTC()
	month := int(now.Month())
	day := now.Day()
	year := now.Year()

	d := db.Get()

	// Collect all birthday matches first to avoid nested queries on a single SQLite connection
	type bdayMatch struct {
		userID    string
		birthYear int
	}
	rows, err := d.Query(
		`SELECT user_id, year FROM birthdays WHERE month = ? AND day = ?`, month, day,
	)
	if err != nil {
		return fmt.Errorf("birthday: check query: %w", err)
	}
	var matches []bdayMatch
	for rows.Next() {
		var m bdayMatch
		if err := rows.Scan(&m.userID, &m.birthYear); err != nil {
			slog.Error("birthday: scan", "err", err)
			continue
		}
		matches = append(matches, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("birthday: rows iteration: %w", err)
	}

	for _, m := range matches {
		// Check if already fired this year
		var fired int
		err := d.QueryRow(
			`SELECT 1 FROM birthday_fired WHERE user_id = ? AND year = ?`, m.userID, year,
		).Scan(&fired)
		if err == nil {
			continue // Already fired this year
		}

		// Build announcement
		var ageStr string
		if m.birthYear > 0 {
			age := year - m.birthYear
			ageStr = fmt.Sprintf(" They're turning %d!", age)
		}

		announcement := fmt.Sprintf("Happy Birthday to %s!%s Have an amazing day!", m.userID, ageStr)
		if err := p.SendMessage(roomID, announcement); err != nil {
			slog.Error("birthday: send announcement", "user", m.userID, "err", err)
			continue
		}

		// Send DM to the birthday person
		dmMsg := fmt.Sprintf("Happy Birthday! The community wishes you a wonderful day!%s You've been granted 100 bonus XP as a birthday gift!", ageStr)
		if err := p.SendDM(id.UserID(m.userID), dmMsg); err != nil {
			slog.Error("birthday: send DM", "user", m.userID, "err", err)
		}

		// Grant 100 XP
		if p.xp != nil {
			p.xp.GrantXP(id.UserID(m.userID), 100, "birthday")
		} else {
			p.grantBirthdayXP(id.UserID(m.userID), 100)
		}

		// Mark as fired
		_, err = d.Exec(
			`INSERT INTO birthday_fired (user_id, year) VALUES (?, ?)
			 ON CONFLICT(user_id, year) DO NOTHING`,
			m.userID, year,
		)
		if err != nil {
			slog.Error("birthday: mark fired", "user", m.userID, "err", err)
		}
	}

	return nil
}

// grantBirthdayXP is a fallback that inserts XP directly via SQL if no XPPlugin is available.
func (p *BirthdayPlugin) grantBirthdayXP(userID id.UserID, amount int) {
	d := db.Get()

	_, err := d.Exec(
		`INSERT INTO users (user_id, xp, level, last_xp_at) VALUES (?, 0, 0, 0)
		 ON CONFLICT(user_id) DO NOTHING`, string(userID))
	if err != nil {
		slog.Error("birthday: ensure user", "err", err)
		return
	}

	_, err = d.Exec(
		`UPDATE users SET xp = xp + ?, last_xp_at = ? WHERE user_id = ?`,
		amount, time.Now().UTC().Unix(), string(userID))
	if err != nil {
		slog.Error("birthday: grant xp", "err", err)
		return
	}

	_, err = d.Exec(
		`INSERT INTO xp_log (user_id, amount, reason) VALUES (?, ?, ?)`,
		string(userID), amount, "birthday")
	if err != nil {
		slog.Error("birthday: log xp", "err", err)
	}
}

// parseBirthdayDate parses MM-DD or MM-DD-YYYY format.
func parseBirthdayDate(s string) (month, day, year int, err error) {
	parts := strings.Split(s, "-")
	switch len(parts) {
	case 2:
		// MM-DD
		month, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid month: %s", parts[0])
		}
		day, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid day: %s", parts[1])
		}
		return month, day, 0, nil
	case 3:
		// MM-DD-YYYY
		month, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid month: %s", parts[0])
		}
		day, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid day: %s", parts[1])
		}
		year, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid year: %s", parts[2])
		}
		return month, day, year, nil
	default:
		return 0, 0, 0, fmt.Errorf("use MM-DD or MM-DD-YYYY format")
	}
}

// daysUntilBirthday calculates days from now until the next occurrence of the given month/day.
func daysUntilBirthday(now time.Time, month, day int) int {
	thisYear := time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if thisYear.Before(today) {
		// Birthday already passed this year, calculate for next year
		thisYear = time.Date(now.Year()+1, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}

	return int(thisYear.Sub(today).Hours() / 24)
}
