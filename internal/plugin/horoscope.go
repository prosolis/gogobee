package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// zodiacSign maps month/day to a zodiac sign name.
func zodiacSign(month, day int) string {
	switch {
	case (month == 3 && day >= 21) || (month == 4 && day <= 19):
		return "aries"
	case (month == 4 && day >= 20) || (month == 5 && day <= 20):
		return "taurus"
	case (month == 5 && day >= 21) || (month == 6 && day <= 20):
		return "gemini"
	case (month == 6 && day >= 21) || (month == 7 && day <= 22):
		return "cancer"
	case (month == 7 && day >= 23) || (month == 8 && day <= 22):
		return "leo"
	case (month == 8 && day >= 23) || (month == 9 && day <= 22):
		return "virgo"
	case (month == 9 && day >= 23) || (month == 10 && day <= 22):
		return "libra"
	case (month == 10 && day >= 23) || (month == 11 && day <= 21):
		return "scorpio"
	case (month == 11 && day >= 22) || (month == 12 && day <= 21):
		return "sagittarius"
	case (month == 12 && day >= 22) || (month == 1 && day <= 19):
		return "capricorn"
	case (month == 1 && day >= 20) || (month == 2 && day <= 18):
		return "aquarius"
	case (month == 2 && day >= 19) || (month == 3 && day <= 20):
		return "pisces"
	}
	return ""
}

var zodiacEmoji = map[string]string{
	"aries":       "♈",
	"taurus":      "♉",
	"gemini":      "♊",
	"cancer":      "♋",
	"leo":         "♌",
	"virgo":       "♍",
	"libra":       "♎",
	"scorpio":     "♏",
	"sagittarius": "♐",
	"capricorn":   "♑",
	"aquarius":    "♒",
	"pisces":      "♓",
}

// HoroscopePlugin provides daily horoscope readings based on user birthdays.
type HoroscopePlugin struct {
	Base
}

// NewHoroscopePlugin creates a new HoroscopePlugin.
func NewHoroscopePlugin(client *mautrix.Client) *HoroscopePlugin {
	return &HoroscopePlugin{
		Base: NewBase(client),
	}
}

func (p *HoroscopePlugin) Name() string { return "horoscope" }

func (p *HoroscopePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "horoscope", Description: "Get your daily horoscope (requires birthday)", Usage: "!horoscope", Category: "Fun & Games"},
	}
}

func (p *HoroscopePlugin) Init() error { return nil }

func (p *HoroscopePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *HoroscopePlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "horoscope") {
		return p.handleHoroscope(ctx)
	}
	return nil
}

func (p *HoroscopePlugin) handleHoroscope(ctx MessageContext) error {
	// Check if user has a birthday set
	d := db.Get()
	var month, day int
	err := d.QueryRow(
		`SELECT month, day FROM birthdays WHERE user_id = ? AND month > 0 AND day > 0`,
		string(ctx.Sender),
	).Scan(&month, &day)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"You need to set your birthday first! Use !birthday set <MM-DD> to get your horoscope.")
	}

	sign := zodiacSign(month, day)
	if sign == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not determine your zodiac sign. Check your birthday with !birthday show.")
	}

	// Check cache (horoscopes are daily, cache for 6 hours)
	cacheKey := fmt.Sprintf("horoscope:%s", sign)
	if cached := db.CacheGet(cacheKey, 21600); cached != "" {
		emoji := zodiacEmoji[sign]
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s **%s Horoscope** (%s)\n\n%s", emoji, titleCase(sign), time.Now().UTC().Format("Jan 2"), cached))
	}

	// Fetch from API
	horoscope, err := fetchHoroscope(sign)
	if err != nil {
		slog.Error("horoscope: fetch failed", "err", err, "sign", sign)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch your horoscope. Try again later.")
	}

	// Cache the result
	db.CacheSet(cacheKey, horoscope)

	emoji := zodiacEmoji[sign]
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("%s **%s Horoscope** (%s)\n\n%s", emoji, titleCase(sign), time.Now().UTC().Format("Jan 2"), horoscope))
}

func fetchHoroscope(sign string) (string, error) {
	url := fmt.Sprintf("https://freehoroscopeapi.com/api/v1/get-horoscope/daily?sign=%s", sign)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var result struct {
		Data struct {
			Horoscope string `json:"horoscope"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.Data.Horoscope == "" {
		return "", fmt.Errorf("empty horoscope in response")
	}

	return strings.TrimSpace(result.Data.Horoscope), nil
}

// PostDailyHoroscopes posts all 12 horoscopes to a room as a daily digest.
func (p *HoroscopePlugin) PostDailyHoroscopes(roomID id.RoomID) {
	dateKey := time.Now().UTC().Format("2006-01-02")
	if db.JobCompleted("horoscope", dateKey+":"+string(roomID)) {
		return
	}

	signs := []string{
		"aries", "taurus", "gemini", "cancer", "leo", "virgo",
		"libra", "scorpio", "sagittarius", "capricorn", "aquarius", "pisces",
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daily Horoscopes — %s\n\n", time.Now().UTC().Format("Monday, January 2")))

	fetched := 0
	for _, sign := range signs {
		horoscope, err := fetchHoroscope(sign)
		if err != nil {
			slog.Error("horoscope: daily fetch", "sign", sign, "err", err)
			continue
		}

		// Cache each one
		db.CacheSet(fmt.Sprintf("horoscope:%s", sign), horoscope)

		emoji := zodiacEmoji[sign]
		sb.WriteString(fmt.Sprintf("%s **%s**\n%s\n\n", emoji, titleCase(sign), horoscope))
		fetched++
	}

	if fetched == 0 {
		slog.Error("horoscope: no signs fetched for daily post")
		return
	}

	sb.WriteString("Set your birthday with !birthday set <MM-DD> to get personalized readings!")

	if err := p.SendMessage(roomID, sb.String()); err != nil {
		slog.Error("horoscope: post daily", "room", roomID, "err", err)
		return
	}

	db.MarkJobCompleted("horoscope", dateKey+":"+string(roomID))
}
