package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// Holiday represents a single holiday entry.
type Holiday struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Country     string `json:"country"`
	Type        string `json:"type"`
	Date        string `json:"date"`
}

// holidaysDayData holds all holidays for a given date.
type holidaysDayData struct {
	Holidays []Holiday `json:"holidays"`
}

// calendarificResponse is the Calendarific API response structure.
type calendarificResponse struct {
	Response struct {
		Holidays []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Country     struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"country"`
			Type []string `json:"type"`
			Date struct {
				ISO string `json:"iso"`
			} `json:"date"`
		} `json:"holidays"`
	} `json:"response"`
}

// hebcalResponse is the HebCal API response structure.
type hebcalResponse struct {
	Items []struct {
		Title    string `json:"title"`
		Date     string `json:"date"`
		Category string `json:"category"`
		Memo     string `json:"memo"`
	} `json:"items"`
}

// aladhanResponse is the Aladhan API response for Hijri date conversion.
type aladhanResponse struct {
	Data struct {
		Hijri struct {
			Date  string `json:"date"`
			Month struct {
				En string `json:"en"`
			} `json:"month"`
			Year string `json:"year"`
			Day  string `json:"day"`
		} `json:"hijri"`
	} `json:"data"`
}

// HolidaysPlugin provides multi-calendar holiday announcements.
type HolidaysPlugin struct {
	Base
	calendarificKey string
	countries       []string
	httpClient      *http.Client
}

// NewHolidaysPlugin creates a new HolidaysPlugin.
func NewHolidaysPlugin(client *mautrix.Client) *HolidaysPlugin {
	countries := []string{
		"US", "GB", "CA", "AU", "PT", "IN", "JP",
		"DE", "FR", "BR", "MX", "IT", "ES", "KR",
		"NL", "SE", "NO", "IE", "NZ", "ZA", "PH",
	}
	if env := os.Getenv("HOLIDAY_COUNTRIES"); env != "" {
		countries = nil
		for _, c := range strings.Split(env, ",") {
			c = strings.TrimSpace(strings.ToUpper(c))
			if c != "" {
				countries = append(countries, c)
			}
		}
	}

	return &HolidaysPlugin{
		Base:            NewBase(client),
		calendarificKey: os.Getenv("CALENDARIFIC_API_KEY"),
		countries:       countries,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *HolidaysPlugin) Name() string { return "holidays" }

func (p *HolidaysPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "holidays", Description: "Show today's holidays (or week/month)", Usage: "!holidays [week|month]", Category: "Holidays"},
	}
}

func (p *HolidaysPlugin) Init() error { return nil }

func (p *HolidaysPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *HolidaysPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "holidays") {
		return p.handleHolidays(ctx)
	}
	return nil
}

// Prefetch fetches today's holidays from all sources and stores them.
func (p *HolidaysPlugin) Prefetch() error {
	today := time.Now().UTC()
	dateStr := today.Format("2006-01-02")

	d := db.Get()
	var exists int
	err := d.QueryRow(`SELECT 1 FROM holidays_log WHERE date = ?`, dateStr).Scan(&exists)
	if err == nil {
		slog.Info("holidays: already fetched for today", "date", dateStr)
		return nil
	}

	var allHolidays []Holiday

	// Fetch from Calendarific for multiple countries
	if p.calendarificKey != "" {
		for _, country := range p.countries {
			holidays, err := p.fetchCalendarific(today, country)
			if err != nil {
				slog.Error("holidays: calendarific fetch failed", "country", country, "err", err)
			} else {
				allHolidays = append(allHolidays, holidays...)
			}
		}
	}

	// Fetch from HebCal
	holidays, err := p.fetchHebCal(today)
	if err != nil {
		slog.Error("holidays: hebcal fetch failed", "err", err)
	} else {
		allHolidays = append(allHolidays, holidays...)
	}

	// Fetch Islamic date info from Aladhan
	islamicInfo, err := p.fetchAladhan(today)
	if err != nil {
		slog.Error("holidays: aladhan fetch failed", "err", err)
	} else if islamicInfo != "" {
		allHolidays = append(allHolidays, Holiday{
			Name:    "Islamic Date",
			Description: islamicInfo,
			Country: "International",
			Type:    "islamic-calendar",
			Date:    dateStr,
		})
	}

	// Deduplicate by holiday name (case-insensitive)
	allHolidays = dedupeHolidays(allHolidays)

	data := holidaysDayData{Holidays: allHolidays}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("holidays: marshal: %w", err)
	}

	_, err = d.Exec(
		`INSERT INTO holidays_log (date, data, posted) VALUES (?, ?, 0)
		 ON CONFLICT(date) DO UPDATE SET data = ?`,
		dateStr, string(jsonData), string(jsonData),
	)
	if err != nil {
		return fmt.Errorf("holidays: store: %w", err)
	}

	slog.Info("holidays: prefetched", "date", dateStr, "count", len(allHolidays))
	return nil
}

// PostHolidays posts today's holidays to the given room.
func (p *HolidaysPlugin) PostHolidays(roomID id.RoomID) error {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	// Per-room dedup
	roomKey := fmt.Sprintf("%s:%s", today, roomID)
	if db.JobCompleted("holidays", roomKey) {
		slog.Info("holidays: already posted today", "date", today, "room", roomID)
		return nil
	}

	var dataStr string
	err := d.QueryRow(
		`SELECT data FROM holidays_log WHERE date = ?`, today,
	).Scan(&dataStr)
	if err == sql.ErrNoRows {
		slog.Warn("holidays: no entry for today, attempting prefetch", "date", today)
		if err := p.Prefetch(); err != nil {
			return fmt.Errorf("holidays: prefetch failed: %w", err)
		}
		err = d.QueryRow(
			`SELECT data FROM holidays_log WHERE date = ?`, today,
		).Scan(&dataStr)
		if err != nil {
			return fmt.Errorf("holidays: still no entry after prefetch: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("holidays: query: %w", err)
	}

	var data holidaysDayData
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return fmt.Errorf("holidays: unmarshal: %w", err)
	}

	if len(data.Holidays) == 0 {
		slog.Info("holidays: no holidays to post today")
		return nil
	}

	msg := p.formatHolidays(today, data.Holidays)
	if err := p.SendMessage(roomID, msg); err != nil {
		return fmt.Errorf("holidays: send: %w", err)
	}

	db.MarkJobCompleted("holidays", roomKey)

	return nil
}

func (p *HolidaysPlugin) handleHolidays(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "holidays"))

	switch strings.ToLower(args) {
	case "week":
		return p.handleHolidaysRange(ctx, 7)
	case "month":
		return p.handleHolidaysRange(ctx, 30)
	default:
		return p.handleHolidaysToday(ctx)
	}
}

func (p *HolidaysPlugin) handleHolidaysToday(ctx MessageContext) error {
	today := time.Now().UTC().Format("2006-01-02")
	d := db.Get()

	var dataStr string
	err := d.QueryRow(`SELECT data FROM holidays_log WHERE date = ?`, today).Scan(&dataStr)
	if err == sql.ErrNoRows {
		// Prefetch on demand
		if pfErr := p.Prefetch(); pfErr != nil {
			slog.Error("holidays: on-demand prefetch failed", "err", pfErr)
			return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch holiday data. Try again later.")
		}
		err = d.QueryRow(`SELECT data FROM holidays_log WHERE date = ?`, today).Scan(&dataStr)
	}
	if err != nil {
		slog.Error("holidays: query", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch holiday data.")
	}

	var data holidaysDayData
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		slog.Error("holidays: unmarshal", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to parse holiday data.")
	}

	if len(data.Holidays) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No holidays today.")
	}

	msg := p.formatHolidays(today, data.Holidays)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *HolidaysPlugin) handleHolidaysRange(ctx MessageContext, days int) error {
	d := db.Get()
	now := time.Now().UTC()

	var sb strings.Builder
	label := "This Week"
	if days == 30 {
		label = "This Month"
	}
	sb.WriteString(fmt.Sprintf("Holidays — %s\n", label))

	found := false
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, i)
		dateStr := date.Format("2006-01-02")

		var dataStr string
		err := d.QueryRow(`SELECT data FROM holidays_log WHERE date = ?`, dateStr).Scan(&dataStr)
		if err != nil {
			continue
		}

		var data holidaysDayData
		if json.Unmarshal([]byte(dataStr), &data) != nil || len(data.Holidays) == 0 {
			continue
		}

		found = true
		sb.WriteString(fmt.Sprintf("\n%s (%s):\n", dateStr, date.Format("Monday")))
		for _, h := range p.selectFeatured(data.Holidays) {
			sb.WriteString(fmt.Sprintf("  - %s", h.Name))
			if h.Country != "" && h.Country != "International" {
				sb.WriteString(fmt.Sprintf(" [%s]", h.Country))
			}
			sb.WriteString("\n")
		}
	}

	if !found {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No holiday data available for that range.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *HolidaysPlugin) formatHolidays(date string, holidays []Holiday) string {
	var sb strings.Builder
	t, _ := time.Parse("2006-01-02", date)
	sb.WriteString(fmt.Sprintf("Today's Holidays — %s\n", t.Format("Monday, January 2, 2006")))

	featured := p.selectFeatured(holidays)
	for _, h := range featured {
		sb.WriteString(fmt.Sprintf("\n- %s", h.Name))
		if h.Country != "" && h.Country != "International" {
			sb.WriteString(fmt.Sprintf(" [%s]", h.Country))
		}
		if h.Description != "" {
			sb.WriteString(fmt.Sprintf("\n  %s", h.Description))
		}
	}

	remaining := len(holidays) - len(featured)
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("\n\n...and %d more. Use !holidays for the full list.", remaining))
	}

	return sb.String()
}

// selectFeatured picks the most interesting holidays to highlight (up to 8).
func (p *HolidaysPlugin) selectFeatured(holidays []Holiday) []Holiday {
	if len(holidays) <= 8 {
		return holidays
	}

	// Prioritize national holidays and religious observances
	priorityTypes := map[string]bool{
		"national": true, "public": true, "religious": true,
		"observance": true, "jewish": true, "islamic-calendar": true,
	}

	var featured, rest []Holiday
	for _, h := range holidays {
		if priorityTypes[strings.ToLower(h.Type)] {
			featured = append(featured, h)
		} else {
			rest = append(rest, h)
		}
	}

	// Fill remaining slots
	for _, h := range rest {
		if len(featured) >= 8 {
			break
		}
		featured = append(featured, h)
	}

	return featured
}

// normalizeHolidayName strips common suffixes like "Day", "Eve" for dedup matching.
func normalizeHolidayName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, " day")
	n = strings.TrimSuffix(n, "'s")
	n = strings.TrimSuffix(n, "s")
	return n
}

// dedupeHolidays removes duplicate holidays by name or description.
func dedupeHolidays(holidays []Holiday) []Holiday {
	seenName := make(map[string]int) // normalized name -> index in result
	seenDesc := make(map[string]int) // lowercase description -> index in result

	var result []Holiday
	for _, h := range holidays {
		nameLower := normalizeHolidayName(h.Name)
		descLower := strings.ToLower(h.Description)

		// Dedupe by name
		if idx, ok := seenName[nameLower]; ok {
			result[idx].Country = "International"
			continue
		}

		// Dedupe by description (catches "Eight Hours Day" == "Labour Day" etc.)
		if descLower != "" {
			if idx, ok := seenDesc[descLower]; ok {
				result[idx].Country = "International"
				continue
			}
		}

		seenName[nameLower] = len(result)
		if descLower != "" {
			seenDesc[descLower] = len(result)
		}
		result = append(result, h)
	}
	return result
}

func (p *HolidaysPlugin) fetchCalendarific(date time.Time, country string) ([]Holiday, error) {
	url := fmt.Sprintf(
		"https://calendarific.com/api/v2/holidays?api_key=%s&country=%s&year=%d&month=%d&day=%d",
		p.calendarificKey, country, date.Year(), int(date.Month()), date.Day(),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendarific returned status %d", resp.StatusCode)
	}

	var calResp calendarificResponse
	if err := json.NewDecoder(resp.Body).Decode(&calResp); err != nil {
		return nil, err
	}

	// Types to exclude — too regional to be interesting
	skipTypes := map[string]bool{
		"Local holiday":        true,
		"Common local holiday": true,
		"Local observance":     true,
		"Clock change/Daylight Saving Time": true,
	}

	var holidays []Holiday
	for _, h := range calResp.Response.Holidays {
		hType := "other"
		if len(h.Type) > 0 {
			hType = h.Type[0]
		}
		if skipTypes[hType] {
			continue
		}
		holidays = append(holidays, Holiday{
			Name:        h.Name,
			Description: h.Description,
			Country:     h.Country.Name,
			Type:        hType,
			Date:        date.Format("2006-01-02"),
		})
	}

	return holidays, nil
}

func (p *HolidaysPlugin) fetchHebCal(date time.Time) ([]Holiday, error) {
	url := fmt.Sprintf(
		"https://www.hebcal.com/hebcal?v=1&cfg=json&year=%d&month=%d",
		date.Year(), int(date.Month()),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hebcal returned status %d", resp.StatusCode)
	}

	var hebResp hebcalResponse
	if err := json.NewDecoder(resp.Body).Decode(&hebResp); err != nil {
		return nil, err
	}

	dateStr := date.Format("2006-01-02")
	var holidays []Holiday
	for _, item := range hebResp.Items {
		// HebCal dates are in YYYY-MM-DD format; match today
		if len(item.Date) >= 10 && item.Date[:10] == dateStr {
			holidays = append(holidays, Holiday{
				Name:        item.Title,
				Description: item.Memo,
				Country:     "International",
				Type:        "jewish",
				Date:        dateStr,
			})
		}
	}

	return holidays, nil
}

func (p *HolidaysPlugin) fetchAladhan(date time.Time) (string, error) {
	url := fmt.Sprintf(
		"https://api.aladhan.com/v1/gToH/%02d-%02d-%d",
		date.Day(), int(date.Month()), date.Year(),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aladhan returned status %d", resp.StatusCode)
	}

	var aResp aladhanResponse
	if err := json.NewDecoder(resp.Body).Decode(&aResp); err != nil {
		return "", err
	}

	hijri := aResp.Data.Hijri
	if hijri.Day != "" && hijri.Month.En != "" && hijri.Year != "" {
		return fmt.Sprintf("%s %s, %s AH", hijri.Day, hijri.Month.En, hijri.Year), nil
	}

	return "", nil
}
