package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

// cityTimezones maps common city names to IANA timezone strings.
var cityTimezones = map[string]string{
	"new york":      "America/New_York",
	"nyc":           "America/New_York",
	"los angeles":   "America/Los_Angeles",
	"la":            "America/Los_Angeles",
	"chicago":       "America/Chicago",
	"denver":        "America/Denver",
	"london":        "Europe/London",
	"paris":         "Europe/Paris",
	"berlin":        "Europe/Berlin",
	"tokyo":         "Asia/Tokyo",
	"sydney":        "Australia/Sydney",
	"dubai":         "Asia/Dubai",
	"moscow":        "Europe/Moscow",
	"mumbai":        "Asia/Kolkata",
	"beijing":       "Asia/Shanghai",
	"shanghai":      "Asia/Shanghai",
	"singapore":     "Asia/Singapore",
	"hong kong":     "Asia/Hong_Kong",
	"seoul":         "Asia/Seoul",
	"toronto":       "America/Toronto",
	"vancouver":     "America/Vancouver",
	"sao paulo":     "America/Sao_Paulo",
	"mexico city":   "America/Mexico_City",
	"cairo":         "Africa/Cairo",
	"johannesburg":  "Africa/Johannesburg",
	"auckland":      "Pacific/Auckland",
	"honolulu":      "Pacific/Honolulu",
	"hawaii":        "Pacific/Honolulu",
	"anchorage":     "America/Anchorage",
	"amsterdam":     "Europe/Amsterdam",
	"rome":          "Europe/Rome",
	"madrid":        "Europe/Madrid",
	"lisbon":        "Europe/Lisbon",
	"bangkok":       "Asia/Bangkok",
	"jakarta":       "Asia/Jakarta",
	"manila":        "Asia/Manila",
	"taipei":        "Asia/Taipei",
	"utc":           "UTC",
	"gmt":           "UTC",
}

var eightBallResponses = []string{
	"It is certain.", "It is decidedly so.", "Without a doubt.",
	"Yes, definitely.", "You may rely on it.", "As I see it, yes.",
	"Most likely.", "Outlook good.", "Yes.", "Signs point to yes.",
	"Reply hazy, try again.", "Ask again later.", "Better not tell you now.",
	"Cannot predict now.", "Concentrate and ask again.",
	"Don't count on it.", "My reply is no.", "My sources say no.",
	"Outlook not so good.", "Very doubtful.",
}

var twinbeeFacts = []string{
	"TwinBee was first released in 1985 by Konami for arcades in Japan.",
	"The TwinBee series is known as a 'cute 'em up' — a cute-themed shoot 'em up.",
	"TwinBee's main characters are sentient, bell-powered fighter ships.",
	"The bells in TwinBee change color when shot, granting different power-ups.",
	"WinBee, the pink ship, is piloted by Pastel in the later games.",
	"TwinBee Rainbow Bell Adventure is a rare platformer spinoff for the SNES.",
	"GwinBee is the green third ship, piloted by Mint in TwinBee Yahho!",
	"The TwinBee anime, 'TwinBee Paradise', aired as a radio drama in Japan.",
	"Detana!! TwinBee is considered one of the best entries and was a PC Engine hit.",
	"TwinBee Yahho! (1995) was the last major arcade release in the series.",
	"Pop'n TwinBee for the SNES featured a 2-player co-op mode with combined attacks.",
	"In TwinBee games, you punch clouds to release bells for power-ups.",
	"The TwinBee series inspired parts of Konami's Parodius games.",
	"Light and Pastel, the pilots of TwinBee and WinBee, are childhood friends.",
	"Dr. Cinnamon is the inventor who created the TwinBee ships.",
	"TwinBee 3: Poko Poko Daimaou featured an overworld map, mixing RPG and shooter elements.",
}

var diceRe = regexp.MustCompile(`(?i)^(\d+)?d(\d+)([+-]\d+)?$`)

// FunPlugin provides various fun and utility commands.
type FunPlugin struct {
	Base
	rateLimiter *RateLimitsPlugin
}

// NewFunPlugin creates a new FunPlugin.
func NewFunPlugin(client *mautrix.Client, rateLimiter *RateLimitsPlugin) *FunPlugin {
	return &FunPlugin{
		Base:        NewBase(client),
		rateLimiter: rateLimiter,
	}
}

func (p *FunPlugin) Name() string { return "fun" }

func (p *FunPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "roll", Description: "Roll dice", Usage: "!roll [NdM+X]", Category: "Fun & Games"},
		{Name: "8ball", Description: "Magic 8-ball", Usage: "!8ball <question>", Category: "Fun & Games"},
		{Name: "coin", Description: "Flip a coin", Usage: "!coin", Category: "Fun & Games"},
		{Name: "time", Description: "World clock or user's local time", Usage: "!time [city|@user]", Category: "Lookup & Reference"},
		{Name: "hltb", Description: "HowLongToBeat lookup", Usage: "!hltb <game>", Category: "Lookup & Reference"},
		{Name: "twinbee", Description: "Random Twinbee lore/trivia", Usage: "!twinbee", Category: "Fun & Games"},
		{Name: "poll", Description: "Create a reaction poll", Usage: "!poll <question> | <option1> | <option2> ...", Category: "Fun & Games"},
		{Name: "weather", Description: "Weather lookup", Usage: "!weather <location>", Category: "Lookup & Reference"},
		{Name: "dadjoke", Description: "Random dad joke", Usage: "!dadjoke", Category: "Fun & Games"},
		{Name: "randomwiki", Description: "Random Wikipedia article", Usage: "!randomwiki", Category: "Fun & Games"},
	}
}

func (p *FunPlugin) Init() error { return nil }

func (p *FunPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *FunPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "roll"):
		return p.handleRoll(ctx)
	case p.IsCommand(ctx.Body, "8ball"):
		return p.handleEightBall(ctx)
	case p.IsCommand(ctx.Body, "coin"):
		return p.handleCoin(ctx)
	case p.IsCommand(ctx.Body, "time"):
		return p.handleTime(ctx)
	case p.IsCommand(ctx.Body, "hltb"):
		return p.handleHLTB(ctx)
	case p.IsCommand(ctx.Body, "twinbee"):
		return p.handleTwinbee(ctx)
	case p.IsCommand(ctx.Body, "poll"):
		return p.handlePoll(ctx)
	case p.IsCommand(ctx.Body, "weather"):
		return p.handleWeather(ctx)
	case p.IsCommand(ctx.Body, "dadjoke"):
		return p.handleDadJoke(ctx)
	case p.IsCommand(ctx.Body, "randomwiki"):
		return p.handleRandomWiki(ctx)
	}
	return nil
}

func (p *FunPlugin) handleRoll(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "roll"))

	numDice := 1
	sides := 6
	modifier := 0

	if args != "" {
		matches := diceRe.FindStringSubmatch(args)
		if matches == nil {
			return p.SendMessage(ctx.RoomID, "Invalid dice format. Use NdM+X (e.g., 2d6+3, d20, 4d8-1)")
		}
		if matches[1] != "" {
			numDice, _ = strconv.Atoi(matches[1])
		}
		sides, _ = strconv.Atoi(matches[2])
		if matches[3] != "" {
			modifier, _ = strconv.Atoi(matches[3])
		}
	}

	if numDice < 1 || numDice > 100 {
		return p.SendMessage(ctx.RoomID, "Number of dice must be between 1 and 100.")
	}
	if sides < 2 || sides > 1000 {
		return p.SendMessage(ctx.RoomID, "Number of sides must be between 2 and 1000.")
	}

	rolls := make([]int, numDice)
	total := 0
	for i := range numDice {
		rolls[i] = rand.IntN(sides) + 1
		total += rolls[i]
	}
	total += modifier

	var result string
	if numDice == 1 && modifier == 0 {
		result = fmt.Sprintf("🎲 You rolled a **%d** (d%d)", total, sides)
	} else {
		rollStrs := make([]string, len(rolls))
		for i, r := range rolls {
			rollStrs[i] = strconv.Itoa(r)
		}
		modStr := ""
		if modifier > 0 {
			modStr = fmt.Sprintf("+%d", modifier)
		} else if modifier < 0 {
			modStr = fmt.Sprintf("%d", modifier)
		}
		result = fmt.Sprintf("🎲 Rolled %dd%d%s: [%s] = **%d**", numDice, sides, modStr, strings.Join(rollStrs, ", "), total)
	}

	return p.SendMessage(ctx.RoomID, result)
}

func (p *FunPlugin) handleEightBall(ctx MessageContext) error {
	question := strings.TrimSpace(p.GetArgs(ctx.Body, "8ball"))
	if question == "" {
		return p.SendMessage(ctx.RoomID, "🎱 You need to ask a question!")
	}

	response := eightBallResponses[rand.IntN(len(eightBallResponses))]
	return p.SendMessage(ctx.RoomID, fmt.Sprintf("🎱 %s", response))
}

func (p *FunPlugin) handleCoin(ctx MessageContext) error {
	if rand.IntN(2) == 0 {
		return p.SendMessage(ctx.RoomID, "🪙 **Heads!**")
	}
	return p.SendMessage(ctx.RoomID, "🪙 **Tails!**")
}

func (p *FunPlugin) handleTime(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "time"))
	if args == "" {
		// Show a few major cities
		cities := []string{"new york", "london", "tokyo", "sydney"}
		var sb strings.Builder
		sb.WriteString("🕐 World Clock:\n")
		for _, city := range cities {
			tz, _ := time.LoadLocation(cityTimezones[city])
			t := time.Now().In(tz)
			sb.WriteString(fmt.Sprintf("  • %s: %s\n", titleCase(city), t.Format("Mon Jan 2 15:04 MST")))
		}
		sb.WriteString("\nUse !time <city> or !time <@user> for a specific location or person.")
		return p.SendMessage(ctx.RoomID, sb.String())
	}

	// If it looks like a Matrix user ID, go straight to user lookup
	if strings.HasPrefix(args, "@") && strings.Contains(args, ":") {
		return p.showUserTime(ctx, args)
	}

	// Try city map first (cheap, deterministic lookup — avoids username collisions)
	argsLower := strings.ToLower(args)
	if tzName, ok := cityTimezones[argsLower]; ok {
		loc, _ := time.LoadLocation(tzName)
		t := time.Now().In(loc)
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("🕐 %s: **%s**", titleCase(argsLower), t.Format("Monday, January 2, 2006 3:04 PM MST")))
	}

	// Try raw IANA timezone (preserve original case — IANA names are case-sensitive)
	if loc, err := time.LoadLocation(args); err == nil {
		t := time.Now().In(loc)
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("🕐 %s: **%s**", args, t.Format("Monday, January 2, 2006 3:04 PM MST")))
	}

	// Fall back to user lookup
	return p.showUserTime(ctx, args)
}

func (p *FunPlugin) showUserTime(ctx MessageContext, input string) error {
	resolved, ok := p.ResolveUser(input, ctx.RoomID)
	if !ok {
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("Unknown city, timezone, or user: %s", input))
	}

	d := db.Get()
	var tz string
	err := d.QueryRow(`SELECT timezone FROM birthdays WHERE user_id = ?`, string(resolved)).Scan(&tz)
	if err != nil || tz == "" {
		return p.SendMessage(ctx.RoomID,
			fmt.Sprintf("%s hasn't set their timezone yet. They can use !settz <timezone> to set it.", string(resolved)))
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("Invalid timezone stored for %s: %s", string(resolved), tz))
	}

	t := time.Now().In(loc)
	return p.SendMessage(ctx.RoomID,
		fmt.Sprintf("🕐 %s: **%s** (%s)", string(resolved), t.Format("Monday, January 2, 2006 3:04 PM"), tz))
}

func (p *FunPlugin) handleHLTB(ctx MessageContext) error {
	gameName := strings.TrimSpace(p.GetArgs(ctx.Body, "hltb"))
	if gameName == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !hltb <game name>")
	}

	// Check cache (24 hour TTL)
	var cachedData string
	var cachedAt int64
	err := db.Get().QueryRow(
		`SELECT data, cached_at FROM hltb_cache WHERE game_name = ?`,
		strings.ToLower(gameName),
	).Scan(&cachedData, &cachedAt)
	if err == nil && time.Now().Unix()-cachedAt < 86400 {
		return p.SendMessage(ctx.RoomID, cachedData)
	}

	// Scrape HLTB
	result, err := scrapeHLTB(gameName)
	if err != nil {
		slog.Error("hltb: scrape failed", "err", err, "game", gameName)
		return p.SendMessage(ctx.RoomID, "Failed to look up that game on HowLongToBeat.")
	}

	if result == "" {
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("No results found for \"%s\" on HowLongToBeat.", gameName))
	}

	// Cache result
	_, _ = db.Get().Exec(
		`INSERT INTO hltb_cache (game_name, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(game_name) DO UPDATE SET data = ?, cached_at = ?`,
		strings.ToLower(gameName), result, time.Now().Unix(), result, time.Now().Unix(),
	)

	return p.SendMessage(ctx.RoomID, result)
}

// hltbFetchToken gets a short-lived auth token from the HLTB finder init endpoint.
func hltbFetchToken() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://howlongtobeat.com/api/finder/init?t=%d", time.Now().UnixMilli())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://howlongtobeat.com")
	req.Header.Set("Origin", "https://howlongtobeat.com")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty token from HLTB")
	}
	return result.Token, nil
}

func scrapeHLTB(gameName string) (string, error) {
	token, err := hltbFetchToken()
	if err != nil {
		return "", fmt.Errorf("hltb token: %w", err)
	}

	payload := map[string]interface{}{
		"searchType":  "games",
		"searchTerms": []string{gameName},
		"searchPage":  1,
		"size":        1,
		"searchOptions": map[string]interface{}{
			"games": map[string]interface{}{
				"userId":        0,
				"platform":      "",
				"sortCategory":  "popular",
				"rangeCategory": "main",
				"rangeTime":     map[string]interface{}{"min": nil, "max": nil},
				"gameplay":      map[string]interface{}{"perspective": "", "flow": "", "genre": "", "subGenre": "", "difficulty": ""},
				"rangeYear":     map[string]interface{}{"min": "", "max": ""},
				"modifier":      "",
			},
			"users": map[string]interface{}{"sortCategory": "postcount"},
			"lists": map[string]interface{}{"sortCategory": "follows"},
			"filter": "",
			"sort":   0,
			"randomizer": 0,
		},
		"useCache": true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://howlongtobeat.com/api/finder", bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://howlongtobeat.com")
	req.Header.Set("Origin", "https://howlongtobeat.com")
	req.Header.Set("x-auth-token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("hltb HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			GameName     string  `json:"game_name"`
			CompMain     float64 `json:"comp_main"`
			CompPlus     float64 `json:"comp_plus"`
			CompAll      float64 `json:"comp_100"`
			ProfileDev   string  `json:"profile_dev"`
			ReleaseWorld int     `json:"release_world"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Data) == 0 {
		return "", nil
	}

	game := result.Data[0]

	formatTime := func(seconds float64) string {
		if seconds <= 0 {
			return "N/A"
		}
		hours := seconds / 3600.0
		if hours < 1 {
			return fmt.Sprintf("%.0f min", seconds/60.0)
		}
		return fmt.Sprintf("%.1f hours", hours)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\U0001f3ae **%s**\n", game.GameName))
	sb.WriteString(fmt.Sprintf("  Main Story: %s\n", formatTime(game.CompMain)))
	sb.WriteString(fmt.Sprintf("  Main + Extras: %s\n", formatTime(game.CompPlus)))
	sb.WriteString(fmt.Sprintf("  Completionist: %s\n", formatTime(game.CompAll)))
	if game.ProfileDev != "" {
		sb.WriteString(fmt.Sprintf("  Developer: %s\n", game.ProfileDev))
	}

	return sb.String(), nil
}

func (p *FunPlugin) handleTwinbee(ctx MessageContext) error {
	fact := twinbeeFacts[rand.IntN(len(twinbeeFacts))]
	return p.SendMessage(ctx.RoomID, fmt.Sprintf("🐝 **Twinbee Trivia:** %s", fact))
}

func (p *FunPlugin) handlePoll(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "poll")
	if args == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !poll <question> | <option1> | <option2> ...")
	}

	parts := strings.Split(args, "|")
	if len(parts) < 3 {
		return p.SendMessage(ctx.RoomID, "A poll needs a question and at least 2 options, separated by |")
	}

	if len(parts) > 11 {
		return p.SendMessage(ctx.RoomID, "Maximum 10 options allowed.")
	}

	question := strings.TrimSpace(parts[0])
	options := make([]string, 0, len(parts)-1)
	for _, opt := range parts[1:] {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" {
			options = append(options, trimmed)
		}
	}

	numberEmojis := []string{"1\uFE0F\u20E3", "2\uFE0F\u20E3", "3\uFE0F\u20E3", "4\uFE0F\u20E3", "5\uFE0F\u20E3",
		"6\uFE0F\u20E3", "7\uFE0F\u20E3", "8\uFE0F\u20E3", "9\uFE0F\u20E3", "\U0001F51F"}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 **Poll: %s**\n\n", question))
	for i, opt := range options {
		if i < len(numberEmojis) {
			sb.WriteString(fmt.Sprintf("%s %s\n", numberEmojis[i], opt))
		}
	}
	sb.WriteString("\nReact with the number to vote!")

	if err := p.SendMessage(ctx.RoomID, sb.String()); err != nil {
		return err
	}

	return nil
}

func (p *FunPlugin) handleWeather(ctx MessageContext) error {
	location := strings.TrimSpace(p.GetArgs(ctx.Body, "weather"))
	if location == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !weather <location>")
	}

	apiKey := os.Getenv("OPENWEATHER_API_KEY")
	if apiKey == "" {
		return p.SendMessage(ctx.RoomID, "Weather service is not configured (missing API key).")
	}

	// Rate limit (configurable, default 5/day)
	weatherLimit := 5
	if v := os.Getenv("RATELIMIT_WEATHER"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &weatherLimit); n != 1 || err != nil {
			weatherLimit = 5
		}
	}
	if p.rateLimiter != nil && !p.rateLimiter.CheckLimit(ctx.Sender, "weather", weatherLimit) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Weather lookup rate limit reached for today.")
	}

	// Check 1-hour cache (weather data doesn't change that fast)
	cacheKey := "weather:" + strings.ToLower(location)
	if cached := db.CacheGet(cacheKey, 3600); cached != "" {
		return p.SendMessage(ctx.RoomID, cached)
	}

	apiURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric",
		url.QueryEscape(location), apiKey)

	resp, err := http.Get(apiURL)
	if err != nil {
		slog.Error("weather: API request failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch weather data.")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("Location \"%s\" not found.", location))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to read weather data.")
	}

	var weather struct {
		Name string `json:"name"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
		Sys struct {
			Country string `json:"country"`
		} `json:"sys"`
	}

	if err := json.Unmarshal(body, &weather); err != nil {
		slog.Error("weather: parse response", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to parse weather data.")
	}

	desc := "N/A"
	if len(weather.Weather) > 0 {
		desc = weather.Weather[0].Description
	}

	tempF := weather.Main.Temp*9.0/5.0 + 32
	feelsLikeF := weather.Main.FeelsLike*9.0/5.0 + 32

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🌤️ **Weather in %s, %s**\n", weather.Name, weather.Sys.Country))
	sb.WriteString(fmt.Sprintf("  Condition: %s\n", titleCase(desc)))
	sb.WriteString(fmt.Sprintf("  Temperature: %.1f°C (%.1f°F)\n", weather.Main.Temp, tempF))
	sb.WriteString(fmt.Sprintf("  Feels Like: %.1f°C (%.1f°F)\n", weather.Main.FeelsLike, feelsLikeF))
	sb.WriteString(fmt.Sprintf("  Humidity: %d%%\n", weather.Main.Humidity))
	sb.WriteString(fmt.Sprintf("  Wind: %.1f m/s\n", weather.Wind.Speed))

	msg := sb.String()
	db.CacheSet(cacheKey, msg)
	return p.SendMessage(ctx.RoomID, msg)
}

func (p *FunPlugin) handleDadJoke(ctx MessageContext) error {
	req, err := http.NewRequest("GET", "https://icanhazdadjoke.com/", nil)
	if err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to fetch a dad joke.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GogoBee Matrix Bot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("dadjoke: request failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch a dad joke.")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to read dad joke response.")
	}

	var joke struct {
		Joke string `json:"joke"`
	}
	if err := json.Unmarshal(body, &joke); err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to parse dad joke.")
	}

	return p.SendMessage(ctx.RoomID, fmt.Sprintf("😄 %s", joke.Joke))
}

func (p *FunPlugin) handleRandomWiki(ctx MessageContext) error {
	req, err := http.NewRequest("GET", "https://en.wikipedia.org/api/rest_v1/page/random/summary", nil)
	if err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to fetch a random article.")
	}
	req.Header.Set("User-Agent", "GogoBee Matrix Bot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("randomwiki: request failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to fetch a random Wikipedia article.")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to read Wikipedia response.")
	}

	var article struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.Unmarshal(body, &article); err != nil {
		return p.SendMessage(ctx.RoomID, "Failed to parse Wikipedia response.")
	}

	extract := article.Extract
	if len(extract) > 300 {
		extract = extract[:300] + "..."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 **%s**\n", article.Title))
	sb.WriteString(extract)
	if article.ContentURLs.Desktop.Page != "" {
		sb.WriteString(fmt.Sprintf("\n🔗 %s", article.ContentURLs.Desktop.Page))
	}

	return p.SendMessage(ctx.RoomID, sb.String())
}

// titleCase capitalizes the first letter of each word (replacement for deprecated strings.Title).
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(rune(prev)) || prev == ' ' {
			prev = r
			return unicode.ToTitle(r)
		}
		prev = r
		return r
	}, s)
}
