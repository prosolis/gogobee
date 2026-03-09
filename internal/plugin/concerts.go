package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// bandsintownEvent represents an event from the Bandsintown API.
type bandsintownEvent struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	DateTime string `json:"datetime"`
	Title    string `json:"title"`
	Venue    struct {
		Name      string `json:"name"`
		City      string `json:"city"`
		Region    string `json:"region"`
		Country   string `json:"country"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"venue"`
	Lineup    []string `json:"lineup"`
	OnSaleAt  string   `json:"on_sale_datetime"`
	Offers    []struct {
		Type   string `json:"type"`
		URL    string `json:"url"`
		Status string `json:"status"`
	} `json:"offers"`
}

// ConcertsPlugin provides concert lookups via Bandsintown.
type ConcertsPlugin struct {
	Base
	apiKey      string
	httpClient  *http.Client
	rateLimiter *RateLimitsPlugin
	mu          sync.Mutex
	cooldowns   map[string]time.Time // keyed by userID+artist
}

// NewConcertsPlugin creates a new ConcertsPlugin.
func NewConcertsPlugin(client *mautrix.Client, rateLimiter *RateLimitsPlugin) *ConcertsPlugin {
	return &ConcertsPlugin{
		Base:        NewBase(client),
		apiKey:      os.Getenv("BANDSINTOWN_API_KEY"),
		rateLimiter: rateLimiter,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cooldowns: make(map[string]time.Time),
	}
}

func (p *ConcertsPlugin) Name() string { return "concerts" }

func (p *ConcertsPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "concerts", Description: "Search upcoming concerts for an artist", Usage: "!concerts <artist>", Category: "Entertainment"},
		{Name: "concerts watch", Description: "Watch an artist for concert updates", Usage: "!concerts watch <artist>", Category: "Entertainment"},
		{Name: "concerts watching", Description: "List your watched artists", Usage: "!concerts watching", Category: "Entertainment"},
		{Name: "concerts unwatch", Description: "Stop watching an artist", Usage: "!concerts unwatch <artist>", Category: "Entertainment"},
	}
}

func (p *ConcertsPlugin) Init() error { return nil }

func (p *ConcertsPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *ConcertsPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "concerts") {
		return nil
	}

	args := p.GetArgs(ctx.Body, "concerts")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !concerts <artist> | !concerts watch|watching|unwatch <artist>")
	}

	parts := strings.SplitN(args, " ", 2)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "watch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !concerts watch <artist>")
		}
		return p.handleWatch(ctx, strings.TrimSpace(parts[1]))
	case "watching":
		return p.handleWatching(ctx)
	case "unwatch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !concerts unwatch <artist>")
		}
		return p.handleUnwatch(ctx, strings.TrimSpace(parts[1]))
	default:
		return p.handleSearch(ctx, args)
	}
}

func (p *ConcertsPlugin) handleSearch(ctx MessageContext, artist string) error {
	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Concert lookups are not configured (missing API key).")
	}

	// Daily rate limit (configurable, default 10/day to match TS version)
	concertLimit := 10
	if v := os.Getenv("RATELIMIT_CONCERTS"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &concertLimit); n != 1 || err != nil {
			concertLimit = 10
		}
	}
	if p.rateLimiter != nil && !p.rateLimiter.CheckLimit(ctx.Sender, "concerts", concertLimit) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Concert search rate limit reached for today.")
	}

	// Per-search cooldown
	cooldownKey := string(ctx.Sender) + ":" + strings.ToLower(artist)
	p.mu.Lock()
	if last, ok := p.cooldowns[cooldownKey]; ok && time.Since(last) < 30*time.Second {
		p.mu.Unlock()
		return p.SendReply(ctx.RoomID, ctx.EventID, "Please wait a moment before searching for this artist again.")
	}
	p.cooldowns[cooldownKey] = time.Now()
	p.mu.Unlock()

	events, err := p.fetchEvents(artist)
	if err != nil {
		slog.Error("concerts: fetch failed", "artist", artist, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch concert data.")
	}

	if len(events) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No upcoming concerts found for %s.", artist))
	}

	// Show up to 5 events
	limit := 5
	if len(events) < limit {
		limit = len(events)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Upcoming concerts for %s:\n\n", artist))

	for i := 0; i < limit; i++ {
		ev := events[i]
		dateStr := p.formatEventDate(ev.DateTime)
		location := p.formatLocation(ev.Venue.City, ev.Venue.Region, ev.Venue.Country)
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, dateStr))
		sb.WriteString(fmt.Sprintf("   Venue: %s\n", ev.Venue.Name))
		sb.WriteString(fmt.Sprintf("   Location: %s\n", location))
		if ev.URL != "" {
			sb.WriteString(fmt.Sprintf("   Tickets: %s\n", ev.URL))
		}
		if i < limit-1 {
			sb.WriteString("\n")
		}
	}

	if len(events) > limit {
		sb.WriteString(fmt.Sprintf("\n...and %d more events.", len(events)-limit))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ConcertsPlugin) fetchEvents(artist string) ([]bandsintownEvent, error) {
	d := db.Get()
	artistKey := strings.ToLower(artist)

	// Check cache (6h TTL)
	var cached string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM concerts_cache WHERE artist = ?`, artistKey,
	).Scan(&cached, &cachedAt)
	if err == nil && time.Now().Unix()-cachedAt < 6*3600 {
		var events []bandsintownEvent
		if json.Unmarshal([]byte(cached), &events) == nil {
			return events, nil
		}
	}

	// Fetch from API
	encoded := url.PathEscape(artist)
	apiURL := fmt.Sprintf("https://rest.bandsintown.com/artists/%s/events?app_id=%s&date=upcoming",
		encoded, p.apiKey)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bandsintown returned status %d", resp.StatusCode)
	}

	var events []bandsintownEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	// Update cache
	data, _ := json.Marshal(events)
	_, err = d.Exec(
		`INSERT INTO concerts_cache (artist, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(artist) DO UPDATE SET data = ?, cached_at = ?`,
		artistKey, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	)
	if err != nil {
		slog.Error("concerts: cache write", "err", err)
	}

	return events, nil
}

func (p *ConcertsPlugin) formatEventDate(dateStr string) string {
	layouts := []string{
		"2006-01-02T15:04:05",
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t.Format("Mon, Jan 2, 2006 3:04 PM")
		}
	}
	return dateStr
}

func (p *ConcertsPlugin) formatLocation(city, region, country string) string {
	parts := []string{}
	if city != "" {
		parts = append(parts, city)
	}
	if region != "" {
		parts = append(parts, region)
	}
	if country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}

func (p *ConcertsPlugin) handleWatch(ctx MessageContext, artist string) error {
	d := db.Get()
	artistKey := strings.ToLower(artist)
	_, err := d.Exec(
		`INSERT INTO concert_watchlist (user_id, artist, room_id) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, artist) DO NOTHING`,
		string(ctx.Sender), artistKey, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("concerts: watch add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add artist to watchlist.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Now watching %s for concert updates.", artist))
}

func (p *ConcertsPlugin) handleWatching(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT artist FROM concert_watchlist WHERE user_id = ? ORDER BY artist`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("concerts: watching list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watchlist.")
	}
	defer rows.Close()

	var artists []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			continue
		}
		artists = append(artists, a)
	}

	if len(artists) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You're not watching any artists. Use !concerts watch <artist> to start.")
	}

	var sb strings.Builder
	sb.WriteString("Your Concert Watchlist:\n")
	for _, a := range artists {
		sb.WriteString(fmt.Sprintf("  - %s\n", a))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ConcertsPlugin) handleUnwatch(ctx MessageContext, artist string) error {
	d := db.Get()
	artistKey := strings.ToLower(artist)
	res, err := d.Exec(
		`DELETE FROM concert_watchlist WHERE user_id = ? AND artist = ?`,
		string(ctx.Sender), artistKey,
	)
	if err != nil {
		slog.Error("concerts: unwatch", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove artist from watchlist.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s is not in your watchlist.", artist))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Stopped watching %s.", artist))
}

// PostWeeklyDigest sends a weekly concert digest to a room and DMs watchers about upcoming shows.
// Intended to be called by the scheduler on Sundays.
func (p *ConcertsPlugin) PostWeeklyDigest(roomID id.RoomID) {
	if p.apiKey == "" {
		slog.Warn("concerts: skipping weekly digest, no API key configured")
		return
	}

	year, week := time.Now().UTC().ISOWeek()
	weekKey := fmt.Sprintf("%d-W%02d", year, week)
	if db.JobCompleted("concert_digest", weekKey) {
		slog.Info("concerts: already sent digest this week", "week", weekKey)
		return
	}
	// Mark completed at the end only if we succeed (not deferred)
	success := false
	defer func() {
		if success {
			db.MarkJobCompleted("concert_digest", weekKey)
		}
	}()

	d := db.Get()

	// Get all unique watched artists
	rows, err := d.Query(`SELECT DISTINCT artist FROM concert_watchlist`)
	if err != nil {
		slog.Error("concerts: weekly digest query", "err", err)
		return
	}

	type watcherInfo struct {
		userID id.UserID
		roomID id.RoomID
	}

	artistWatchers := make(map[string][]watcherInfo)
	var allArtists []string

	// First collect all artists
	for rows.Next() {
		var artist string
		if err := rows.Scan(&artist); err != nil {
			continue
		}
		allArtists = append(allArtists, artist)
	}
	rows.Close()

	// Get watchers per artist
	for _, artist := range allArtists {
		wRows, err := d.Query(
			`SELECT user_id, room_id FROM concert_watchlist WHERE artist = ?`, artist,
		)
		if err != nil {
			continue
		}
		for wRows.Next() {
			var uid, rid string
			if err := wRows.Scan(&uid, &rid); err != nil {
				continue
			}
			artistWatchers[artist] = append(artistWatchers[artist], watcherInfo{
				userID: id.UserID(uid),
				roomID: id.RoomID(rid),
			})
		}
		wRows.Close()
	}

	// For each artist, fetch events and notify watchers
	for artist, watchers := range artistWatchers {
		events, err := p.fetchEvents(artist)
		if err != nil {
			slog.Error("concerts: weekly digest fetch", "artist", artist, "err", err)
			continue
		}

		if len(events) == 0 {
			continue
		}

		// Filter to events in the next 2 weeks
		now := time.Now()
		twoWeeks := now.Add(14 * 24 * time.Hour)
		var upcoming []bandsintownEvent
		for _, ev := range events {
			t, err := time.Parse("2006-01-02T15:04:05", ev.DateTime)
			if err != nil {
				t, err = time.Parse(time.RFC3339, ev.DateTime)
			}
			if err == nil && t.After(now) && t.Before(twoWeeks) {
				upcoming = append(upcoming, ev)
			}
		}

		if len(upcoming) == 0 {
			continue
		}

		// Build message
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Upcoming shows for %s (next 2 weeks):\n\n", artist))
		for i, ev := range upcoming {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("\n...and %d more.", len(upcoming)-5))
				break
			}
			dateStr := p.formatEventDate(ev.DateTime)
			location := p.formatLocation(ev.Venue.City, ev.Venue.Region, ev.Venue.Country)
			sb.WriteString(fmt.Sprintf("- %s @ %s (%s)\n", dateStr, ev.Venue.Name, location))
		}

		msg := sb.String()
		for _, w := range watchers {
			if err := p.SendDM(w.userID, msg); err != nil {
				slog.Error("concerts: DM watcher", "user", w.userID, "err", err)
			}
		}
	}

	success = true
	slog.Info("concerts: weekly digest completed")
}
