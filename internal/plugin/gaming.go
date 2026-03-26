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
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// releaseListResponse is the top-level RAWG API response for game releases.
type releaseListResponse struct {
	Count   int             `json:"count"`
	Results []releaseEntry  `json:"results"`
}

// releaseEntry is a game entry from the RAWG releases API.
type releaseEntry struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Released  string `json:"released"`
	Rating    float64 `json:"rating"`
	Metacritic int   `json:"metacritic"`
	Platforms []struct {
		Platform struct {
			Name string `json:"name"`
		} `json:"platform"`
	} `json:"platforms"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
}

// GamingPlugin provides game release tracking using the RAWG API.
type GamingPlugin struct {
	Base
	apiKey     string
	httpClient *http.Client
}

// NewGamingPlugin creates a new GamingPlugin.
func NewGamingPlugin(client *mautrix.Client) *GamingPlugin {
	return &GamingPlugin{
		Base:   NewBase(client),
		apiKey: os.Getenv("RAWG_API_KEY"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *GamingPlugin) Name() string { return "gaming" }

func (p *GamingPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "releases", Description: "Show game releases", Usage: "!releases [month|search <query>]", Category: "Entertainment"},
		{Name: "releasewatch", Description: "Manage your game release watchlist", Usage: "!releasewatch add|list|remove <game>", Category: "Entertainment"},
	}
}

func (p *GamingPlugin) Init() error { return nil }

func (p *GamingPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *GamingPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "releases") {
		go func() {
			if err := p.handleReleases(ctx); err != nil {
				slog.Error("gaming: handler error", "err", err)
			}
		}()
		return nil
	}
	if p.IsCommand(ctx.Body, "releasewatch") {
		return p.handleReleaseWatch(ctx)
	}
	return nil
}

// PostReleases posts this week's notable game releases to the given room.
func (p *GamingPlugin) PostReleases(roomID id.RoomID) error {
	if p.apiKey == "" {
		slog.Warn("gaming: RAWG_API_KEY not set, skipping release post")
		return nil
	}

	// Check if already posted this week
	now := time.Now().UTC()
	year, week := now.ISOWeek()
	weekKey := fmt.Sprintf("%d-W%02d:%s", year, week, roomID)
	if db.JobCompleted("releases", weekKey) {
		slog.Info("gaming: already posted releases this week", "week", weekKey)
		return nil
	}

	startDate := now.Format("2006-01-02")
	endDate := now.AddDate(0, 0, 7).Format("2006-01-02")

	games, err := p.fetchReleases(startDate, endDate, "week")
	if err != nil {
		return fmt.Errorf("gaming: fetch releases: %w", err)
	}

	if len(games) == 0 {
		slog.Info("gaming: no notable releases this week")
		db.MarkJobCompleted("releases", weekKey)
		return nil
	}

	msg := p.formatReleases("This Week's Game Releases", games)
	if err := p.SendMessage(roomID, msg); err != nil {
		return err
	}

	db.MarkJobCompleted("releases", weekKey)
	return nil
}

func (p *GamingPlugin) handleReleases(ctx MessageContext) error {
	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Game release lookups are not configured (missing API key).")
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "releases"))

	switch {
	case args == "" || strings.ToLower(args) == "week":
		return p.handleWeekReleases(ctx)
	case strings.ToLower(args) == "month":
		return p.handleMonthReleases(ctx)
	case strings.HasPrefix(strings.ToLower(args), "search "):
		query := strings.TrimSpace(args[7:])
		return p.handleSearchReleases(ctx, query)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releases [month|search <query>]")
	}
}

func (p *GamingPlugin) handleWeekReleases(ctx MessageContext) error {
	now := time.Now().UTC()
	startDate := now.Format("2006-01-02")
	endDate := now.AddDate(0, 0, 7).Format("2006-01-02")

	games, err := p.fetchReleases(startDate, endDate, "week")
	if err != nil {
		slog.Error("gaming: fetch week releases", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch game releases.")
	}

	if len(games) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No notable game releases this week.")
	}

	msg := p.formatReleases("This Week's Game Releases", games)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *GamingPlugin) handleMonthReleases(ctx MessageContext) error {
	now := time.Now().UTC()
	startDate := now.Format("2006-01-02")
	endDate := now.AddDate(0, 1, 0).Format("2006-01-02")

	games, err := p.fetchReleases(startDate, endDate, "month")
	if err != nil {
		slog.Error("gaming: fetch month releases", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch game releases.")
	}

	if len(games) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No notable game releases this month.")
	}

	msg := p.formatReleases("This Month's Game Releases", games)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *GamingPlugin) handleSearchReleases(ctx MessageContext, query string) error {
	if query == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releases search <query>")
	}

	games, err := p.searchUpcoming(query)
	if err != nil {
		slog.Error("gaming: search releases", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search game releases.")
	}

	if len(games) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No upcoming releases found for \"%s\".", query))
	}

	msg := p.formatReleases(fmt.Sprintf("Search Results: \"%s\"", query), games)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *GamingPlugin) fetchReleases(startDate, endDate, cacheKey string) ([]releaseEntry, error) {
	d := db.Get()
	fullKey := fmt.Sprintf("releases_%s_%s_%s", cacheKey, startDate, endDate)

	// Check cache (1 hour TTL)
	var cached string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM releases_cache WHERE cache_key = ?`, fullKey,
	).Scan(&cached, &cachedAt)
	if err == nil && time.Now().UTC().Unix()-cachedAt < 3600 {
		var games []releaseEntry
		if json.Unmarshal([]byte(cached), &games) == nil {
			return games, nil
		}
	}

	// Fetch from RAWG
	apiURL := fmt.Sprintf(
		"https://api.rawg.io/api/games?key=%s&dates=%s,%s&ordering=-added&page_size=20",
		p.apiKey, startDate, endDate,
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RAWG returned status %d", resp.StatusCode)
	}

	var rawgResp releaseListResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawgResp); err != nil {
		return nil, err
	}

	games := rawgResp.Results

	// Cache results
	data, _ := json.Marshal(games)
	db.Exec("gaming: cache write",
		`INSERT INTO releases_cache (cache_key, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(cache_key) DO UPDATE SET data = ?, cached_at = ?`,
		fullKey, string(data), time.Now().UTC().Unix(), string(data), time.Now().UTC().Unix(),
	)

	return games, nil
}

func (p *GamingPlugin) searchUpcoming(query string) ([]releaseEntry, error) {
	now := time.Now().UTC()
	startDate := now.Format("2006-01-02")
	endDate := now.AddDate(1, 0, 0).Format("2006-01-02")

	apiURL := fmt.Sprintf(
		"https://api.rawg.io/api/games?key=%s&search=%s&dates=%s,%s&ordering=-added&page_size=10",
		p.apiKey, url.QueryEscape(query), startDate, endDate,
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RAWG search returned status %d", resp.StatusCode)
	}

	var rawgResp releaseListResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawgResp); err != nil {
		return nil, err
	}

	return rawgResp.Results, nil
}

func (p *GamingPlugin) formatReleases(title string, games []releaseEntry) string {
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n")

	limit := 15
	if len(games) < limit {
		limit = len(games)
	}

	for i := 0; i < limit; i++ {
		g := games[i]
		sb.WriteString(fmt.Sprintf("\n- %s", g.Name))
		if g.Released != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", g.Released))
		}

		var details []string
		if len(g.Platforms) > 0 {
			var plats []string
			for _, pl := range g.Platforms {
				plats = append(plats, pl.Platform.Name)
			}
			if len(plats) > 4 {
				plats = append(plats[:4], "...")
			}
			details = append(details, strings.Join(plats, ", "))
		}
		if len(g.Genres) > 0 {
			var genres []string
			for _, gn := range g.Genres {
				genres = append(genres, gn.Name)
			}
			details = append(details, strings.Join(genres, ", "))
		}
		if g.Metacritic > 0 {
			details = append(details, fmt.Sprintf("Metacritic: %d", g.Metacritic))
		}
		if len(details) > 0 {
			sb.WriteString(fmt.Sprintf("\n  %s", strings.Join(details, " | ")))
		}
	}

	if len(games) > limit {
		sb.WriteString(fmt.Sprintf("\n\n...and %d more.", len(games)-limit))
	}

	return sb.String()
}

func (p *GamingPlugin) handleReleaseWatch(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "releasewatch")
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releasewatch add|list|remove <game>")
	}

	sub := strings.ToLower(parts[0])
	switch sub {
	case "add":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releasewatch add <game>")
		}
		return p.watchlistAdd(ctx, strings.TrimSpace(parts[1]))
	case "remove":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releasewatch remove <game>")
		}
		return p.watchlistRemove(ctx, strings.TrimSpace(parts[1]))
	case "list":
		return p.watchlistList(ctx)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !releasewatch add|list|remove <game>")
	}
}

func (p *GamingPlugin) watchlistAdd(ctx MessageContext, game string) error {
	d := db.Get()
	_, err := d.Exec(
		`INSERT INTO release_watchlist (user_id, game_name, room_id) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, game_name) DO NOTHING`,
		string(ctx.Sender), game, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("gaming: watchlist add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add to watchlist.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Added \"%s\" to your release watchlist.", game))
}

func (p *GamingPlugin) watchlistRemove(ctx MessageContext, game string) error {
	d := db.Get()
	res, err := d.Exec(
		`DELETE FROM release_watchlist WHERE user_id = ? AND game_name = ?`,
		string(ctx.Sender), game,
	)
	if err != nil {
		slog.Error("gaming: watchlist remove", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove from watchlist.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("\"%s\" is not in your watchlist.", game))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Removed \"%s\" from your watchlist.", game))
}

func (p *GamingPlugin) watchlistList(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT game_name FROM release_watchlist WHERE user_id = ? ORDER BY game_name`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("gaming: watchlist list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watchlist.")
	}
	defer rows.Close()

	var games []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			continue
		}
		games = append(games, g)
	}

	if len(games) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Your release watchlist is empty. Use !releasewatch add <game> to add games.")
	}

	var sb strings.Builder
	sb.WriteString("Your Release Watchlist:\n")
	for _, g := range games {
		sb.WriteString(fmt.Sprintf("  - %s\n", g))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
