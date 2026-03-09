package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// jikanAnime represents an anime entry from the Jikan API.
type jikanAnime struct {
	MalID    int    `json:"mal_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	TitleEng string `json:"title_english"`
	Type     string `json:"type"`
	Episodes int    `json:"episodes"`
	Status   string `json:"status"`
	Score    float64 `json:"score"`
	Synopsis string `json:"synopsis"`
	Aired    struct {
		String string `json:"string"`
		From   string `json:"from"`
		To     string `json:"to"`
	} `json:"aired"`
	Broadcast struct {
		Day  string `json:"day"`
		Time string `json:"time"`
		TZ   string `json:"timezone"`
	} `json:"broadcast"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Studios []struct {
		Name string `json:"name"`
	} `json:"studios"`
	Season string `json:"season"`
	Year   int    `json:"year"`
}

// jikanSearchResponse is the Jikan search API response.
type jikanSearchResponse struct {
	Data       []jikanAnime `json:"data"`
	Pagination struct {
		HasNext int `json:"has_next_page"`
	} `json:"pagination"`
}

// jikanSeasonResponse is the Jikan season API response.
type jikanSeasonResponse struct {
	Data []jikanAnime `json:"data"`
}

// AnimePlugin provides anime lookups via the Jikan/MAL API.
type AnimePlugin struct {
	Base
	httpClient *http.Client
	mu         sync.Mutex
	lastCall   time.Time // for rate limiting Jikan (400ms between calls)
}

// NewAnimePlugin creates a new AnimePlugin.
func NewAnimePlugin(client *mautrix.Client) *AnimePlugin {
	return &AnimePlugin{
		Base: NewBase(client),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *AnimePlugin) Name() string { return "anime" }

func (p *AnimePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "anime", Description: "Search for anime info", Usage: "!anime <title>", Category: "Entertainment"},
		{Name: "anime watch", Description: "Add anime to your watchlist", Usage: "!anime watch <title>", Category: "Entertainment"},
		{Name: "anime watching", Description: "List your anime watchlist", Usage: "!anime watching", Category: "Entertainment"},
		{Name: "anime unwatch", Description: "Remove anime from watchlist by MAL ID", Usage: "!anime unwatch <id>", Category: "Entertainment"},
		{Name: "anime season", Description: "Show current season anime", Usage: "!anime season", Category: "Entertainment"},
		{Name: "anime upcoming", Description: "Show upcoming anime", Usage: "!anime upcoming", Category: "Entertainment"},
	}
}

func (p *AnimePlugin) Init() error { return nil }

func (p *AnimePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *AnimePlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "anime") {
		return nil
	}

	args := p.GetArgs(ctx.Body, "anime")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: !anime <title> | !anime watch|watching|unwatch|season|upcoming")
	}

	parts := strings.SplitN(args, " ", 2)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "watch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !anime watch <title>")
		}
		return p.handleWatch(ctx, strings.TrimSpace(parts[1]))
	case "watching":
		return p.handleWatching(ctx)
	case "unwatch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !anime unwatch <id>")
		}
		return p.handleUnwatch(ctx, strings.TrimSpace(parts[1]))
	case "season":
		return p.handleSeason(ctx)
	case "upcoming":
		return p.handleUpcoming(ctx)
	default:
		return p.handleSearch(ctx, args)
	}
}

// rateLimit enforces the 400ms delay between Jikan API calls.
func (p *AnimePlugin) rateLimit() {
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := time.Since(p.lastCall)
	if elapsed < 400*time.Millisecond {
		time.Sleep(400*time.Millisecond - elapsed)
	}
	p.lastCall = time.Now()
}

func (p *AnimePlugin) jikanGet(apiURL string, target interface{}) error {
	p.rateLimit()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jikan returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (p *AnimePlugin) handleSearch(ctx MessageContext, query string) error {
	d := db.Get()

	// Search via API
	encoded := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.jikan.moe/v4/anime?q=%s&limit=3&sfw=true", encoded)

	var searchResp jikanSearchResponse
	if err := p.jikanGet(apiURL, &searchResp); err != nil {
		slog.Error("anime: search failed", "query", query, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for anime.")
	}

	if len(searchResp.Data) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No anime found for \"%s\".", query))
	}

	anime := searchResp.Data[0]

	// Cache the result
	data, _ := json.Marshal(anime)
	if _, err := d.Exec(
		`INSERT INTO anime_cache (mal_id, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(mal_id) DO UPDATE SET data = ?, cached_at = ?`,
		anime.MalID, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	); err != nil {
		slog.Error("anime: cache write", "err", err)
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, p.formatAnime(anime))
}

func (p *AnimePlugin) formatAnime(a jikanAnime) string {
	var sb strings.Builder

	title := a.Title
	if a.TitleEng != "" && a.TitleEng != a.Title {
		title = fmt.Sprintf("%s (%s)", a.TitleEng, a.Title)
	}

	sb.WriteString(fmt.Sprintf("%s\n", title))
	sb.WriteString(fmt.Sprintf("MAL ID: %d\n", a.MalID))

	if a.Type != "" {
		sb.WriteString(fmt.Sprintf("Type: %s", a.Type))
		if a.Episodes > 0 {
			sb.WriteString(fmt.Sprintf(" (%d episodes)", a.Episodes))
		}
		sb.WriteString("\n")
	}

	if a.Status != "" {
		sb.WriteString(fmt.Sprintf("Status: %s\n", a.Status))
	}

	if a.Score > 0 {
		sb.WriteString(fmt.Sprintf("Score: %.2f/10\n", a.Score))
	}

	if a.Aired.String != "" {
		sb.WriteString(fmt.Sprintf("Aired: %s\n", a.Aired.String))
	}

	if len(a.Genres) > 0 {
		genres := make([]string, len(a.Genres))
		for i, g := range a.Genres {
			genres[i] = g.Name
		}
		sb.WriteString(fmt.Sprintf("Genres: %s\n", strings.Join(genres, ", ")))
	}

	if len(a.Studios) > 0 {
		studios := make([]string, len(a.Studios))
		for i, s := range a.Studios {
			studios[i] = s.Name
		}
		sb.WriteString(fmt.Sprintf("Studios: %s\n", strings.Join(studios, ", ")))
	}

	if a.Broadcast.Day != "" {
		sb.WriteString(fmt.Sprintf("Broadcast: %s %s (%s)\n", a.Broadcast.Day, a.Broadcast.Time, a.Broadcast.TZ))
	}

	if a.Synopsis != "" {
		synopsis := a.Synopsis
		if len(synopsis) > 300 {
			synopsis = synopsis[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", synopsis))
	}

	if a.URL != "" {
		sb.WriteString(fmt.Sprintf("\n%s", a.URL))
	}

	return sb.String()
}

func (p *AnimePlugin) handleWatch(ctx MessageContext, title string) error {
	// Search for the anime first to get its MAL ID
	encoded := url.QueryEscape(title)
	apiURL := fmt.Sprintf("https://api.jikan.moe/v4/anime?q=%s&limit=1&sfw=true", encoded)

	var searchResp jikanSearchResponse
	if err := p.jikanGet(apiURL, &searchResp); err != nil {
		slog.Error("anime: watch search failed", "title", title, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for anime.")
	}

	if len(searchResp.Data) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No anime found for \"%s\".", title))
	}

	anime := searchResp.Data[0]
	d := db.Get()

	_, err := d.Exec(
		`INSERT INTO anime_watchlist (user_id, mal_id, title, room_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, mal_id) DO NOTHING`,
		string(ctx.Sender), anime.MalID, anime.Title, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("anime: watchlist add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add to watchlist.")
	}

	// Also cache the anime data
	data, _ := json.Marshal(anime)
	_, _ = d.Exec(
		`INSERT INTO anime_cache (mal_id, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(mal_id) DO UPDATE SET data = ?, cached_at = ?`,
		anime.MalID, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	)

	displayTitle := anime.Title
	if anime.TitleEng != "" {
		displayTitle = anime.TitleEng
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Added \"%s\" (MAL ID: %d) to your watchlist.", displayTitle, anime.MalID))
}

func (p *AnimePlugin) handleWatching(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT mal_id, title FROM anime_watchlist WHERE user_id = ? ORDER BY title`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("anime: watching list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watchlist.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Your Anime Watchlist:\n\n")
	count := 0
	for rows.Next() {
		var malID int
		var title string
		if err := rows.Scan(&malID, &title); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s\n", malID, title))
		count++
	}

	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Your anime watchlist is empty. Use !anime watch <title> to add anime.")
	}

	sb.WriteString("\nUse !anime unwatch <id> to remove an entry.")
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *AnimePlugin) handleUnwatch(ctx MessageContext, idStr string) error {
	malID, err := strconv.Atoi(idStr)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Please provide a valid MAL ID number. Check !anime watching for your list.")
	}

	d := db.Get()
	res, err := d.Exec(
		`DELETE FROM anime_watchlist WHERE user_id = ? AND mal_id = ?`,
		string(ctx.Sender), malID,
	)
	if err != nil {
		slog.Error("anime: unwatch", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove from watchlist.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("MAL ID %d is not in your watchlist.", malID))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Removed MAL ID %d from your watchlist.", malID))
}

func (p *AnimePlugin) handleSeason(ctx MessageContext) error {
	apiURL := "https://api.jikan.moe/v4/seasons/now?limit=10&sfw=true"

	var resp jikanSeasonResponse
	if err := p.jikanGet(apiURL, &resp); err != nil {
		slog.Error("anime: season fetch failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch current season anime.")
	}

	if len(resp.Data) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No anime found for the current season.")
	}

	var sb strings.Builder
	sb.WriteString("Current Season Anime:\n\n")

	for i, a := range resp.Data {
		title := a.Title
		if a.TitleEng != "" {
			title = a.TitleEng
		}
		scoreStr := "N/A"
		if a.Score > 0 {
			scoreStr = fmt.Sprintf("%.1f", a.Score)
		}
		sb.WriteString(fmt.Sprintf("%d. %s [%s] - Score: %s\n", i+1, title, a.Type, scoreStr))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *AnimePlugin) handleUpcoming(ctx MessageContext) error {
	apiURL := "https://api.jikan.moe/v4/seasons/upcoming?limit=10&sfw=true"

	var resp jikanSeasonResponse
	if err := p.jikanGet(apiURL, &resp); err != nil {
		slog.Error("anime: upcoming fetch failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch upcoming anime.")
	}

	if len(resp.Data) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No upcoming anime found.")
	}

	var sb strings.Builder
	sb.WriteString("Upcoming Anime:\n\n")

	for i, a := range resp.Data {
		title := a.Title
		if a.TitleEng != "" {
			title = a.TitleEng
		}
		info := a.Type
		if a.Aired.String != "" {
			info += " | " + a.Aired.String
		}
		sb.WriteString(fmt.Sprintf("%d. %s [%s]\n", i+1, title, info))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// PostDailyReleases checks broadcast days and DMs watchers about anime airing today.
// Intended to be called by the scheduler daily.
func (p *AnimePlugin) PostDailyReleases(roomID id.RoomID) {
	todayKey := time.Now().UTC().Format("2006-01-02")
	if db.JobCompleted("anime_releases", todayKey) {
		slog.Info("anime: already sent daily releases", "date", todayKey)
		return
	}
	success := false
	defer func() {
		if success {
			db.MarkJobCompleted("anime_releases", todayKey)
		}
	}()

	d := db.Get()
	today := strings.ToLower(time.Now().Weekday().String())
	// Jikan uses plural day names like "Mondays", "Tuesdays", etc.
	todayPlural := today + "s"

	// Get all unique MAL IDs from watchlists
	rows, err := d.Query(`SELECT DISTINCT mal_id, title FROM anime_watchlist`)
	if err != nil {
		slog.Error("anime: daily releases query", "err", err)
		return
	}

	type animeEntry struct {
		malID int
		title string
	}
	var watchedAnime []animeEntry
	for rows.Next() {
		var e animeEntry
		if err := rows.Scan(&e.malID, &e.title); err != nil {
			continue
		}
		watchedAnime = append(watchedAnime, e)
	}
	rows.Close()

	for _, entry := range watchedAnime {
		// Check cache or fetch anime details
		var animeData jikanAnime
		var cached string
		var cachedAt int64

		err := d.QueryRow(
			`SELECT data, cached_at FROM anime_cache WHERE mal_id = ?`, entry.malID,
		).Scan(&cached, &cachedAt)

		needsFetch := err != nil || time.Now().Unix()-cachedAt > 24*3600

		if !needsFetch {
			if json.Unmarshal([]byte(cached), &animeData) != nil {
				needsFetch = true
			}
		}

		if needsFetch {
			apiURL := fmt.Sprintf("https://api.jikan.moe/v4/anime/%d", entry.malID)
			var resp struct {
				Data jikanAnime `json:"data"`
			}
			if err := p.jikanGet(apiURL, &resp); err != nil {
				slog.Error("anime: daily fetch", "mal_id", entry.malID, "err", err)
				continue
			}
			animeData = resp.Data

			// Update cache
			data, _ := json.Marshal(animeData)
			_, _ = d.Exec(
				`INSERT INTO anime_cache (mal_id, data, cached_at) VALUES (?, ?, ?)
				 ON CONFLICT(mal_id) DO UPDATE SET data = ?, cached_at = ?`,
				entry.malID, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
			)
		}

		// Check if this anime broadcasts today
		broadcastDay := strings.ToLower(animeData.Broadcast.Day)
		if broadcastDay != todayPlural && broadcastDay != today {
			continue
		}

		// Only notify for currently airing anime
		if animeData.Status != "Currently Airing" {
			continue
		}

		// Find watchers for this anime
		wRows, err := d.Query(
			`SELECT user_id FROM anime_watchlist WHERE mal_id = ?`, entry.malID,
		)
		if err != nil {
			continue
		}

		displayTitle := animeData.Title
		if animeData.TitleEng != "" {
			displayTitle = animeData.TitleEng
		}

		msg := fmt.Sprintf("New episode alert! %s airs today (%s %s %s).\n%s",
			displayTitle, animeData.Broadcast.Day, animeData.Broadcast.Time,
			animeData.Broadcast.TZ, animeData.URL)

		for wRows.Next() {
			var uid string
			if err := wRows.Scan(&uid); err != nil {
				continue
			}
			if err := p.SendDM(id.UserID(uid), msg); err != nil {
				slog.Error("anime: DM watcher", "user", uid, "err", err)
			}
		}
		wRows.Close()
	}

	success = true
	slog.Info("anime: daily releases check completed")
}
