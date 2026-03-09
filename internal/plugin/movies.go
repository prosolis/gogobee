package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// tmdbSearchResult represents a result from the TMDB search API.
type tmdbSearchResult struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"` // for TV shows
	Overview     string  `json:"overview"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"` // for TV shows
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	MediaType    string  `json:"media_type"`
	PosterPath   string  `json:"poster_path"`
	GenreIDs     []int   `json:"genre_ids"`
}

// tmdbSearchResponse is the TMDB search API response.
type tmdbSearchResponse struct {
	Page         int                `json:"page"`
	Results      []tmdbSearchResult `json:"results"`
	TotalResults int                `json:"total_results"`
}

// tmdbMovieDetail has additional movie details.
type tmdbMovieDetail struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	ReleaseDate string  `json:"release_date"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
	Runtime     int     `json:"runtime"`
	Status      string  `json:"status"`
	Tagline     string  `json:"tagline"`
	Budget      int64   `json:"budget"`
	Revenue     int64   `json:"revenue"`
	Genres      []struct {
		Name string `json:"name"`
	} `json:"genres"`
}

// tmdbTVDetail has additional TV show details.
type tmdbTVDetail struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	FirstAirDate string  `json:"first_air_date"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	Status       string  `json:"status"`
	Tagline      string  `json:"tagline"`
	NumSeasons   int     `json:"number_of_seasons"`
	NumEpisodes  int     `json:"number_of_episodes"`
	Genres       []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Networks []struct {
		Name string `json:"name"`
	} `json:"networks"`
}

// tmdbUpcomingResponse is the TMDB upcoming movies response.
type tmdbUpcomingResponse struct {
	Results []tmdbSearchResult `json:"results"`
}

// MoviesPlugin provides movie and TV lookups via TMDB.
type MoviesPlugin struct {
	Base
	apiKey     string
	httpClient *http.Client
}

// NewMoviesPlugin creates a new MoviesPlugin.
func NewMoviesPlugin(client *mautrix.Client) *MoviesPlugin {
	return &MoviesPlugin{
		Base:   NewBase(client),
		apiKey: os.Getenv("TMDB_API_KEY"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *MoviesPlugin) Name() string { return "movies" }

func (p *MoviesPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "movie", Description: "Search for movie info", Usage: "!movie <title>", Category: "Entertainment"},
		{Name: "tv", Description: "Search for TV show info", Usage: "!tv <title>", Category: "Entertainment"},
		{Name: "movie watch", Description: "Add a movie to your watchlist", Usage: "!movie watch <title>", Category: "Entertainment"},
		{Name: "movie watching", Description: "List your movie watchlist", Usage: "!movie watching", Category: "Entertainment"},
		{Name: "movie unwatch", Description: "Remove from watchlist by TMDB ID", Usage: "!movie unwatch <id>", Category: "Entertainment"},
		{Name: "upcoming movies", Description: "Show upcoming movies", Usage: "!upcoming movies", Category: "Entertainment"},
	}
}

func (p *MoviesPlugin) Init() error { return nil }

func (p *MoviesPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *MoviesPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "movie") {
		return p.handleMovie(ctx)
	}
	if p.IsCommand(ctx.Body, "tv") {
		return p.handleTV(ctx)
	}
	if p.IsCommand(ctx.Body, "upcoming") {
		args := p.GetArgs(ctx.Body, "upcoming")
		if strings.ToLower(strings.TrimSpace(args)) == "movies" {
			return p.handleUpcoming(ctx)
		}
	}
	return nil
}

func (p *MoviesPlugin) handleMovie(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "movie")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: !movie <title> | !movie watch|watching|unwatch <title|id>")
	}

	parts := strings.SplitN(args, " ", 2)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "watch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !movie watch <title>")
		}
		return p.handleWatchMovie(ctx, strings.TrimSpace(parts[1]))
	case "watching":
		return p.handleWatching(ctx)
	case "unwatch":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !movie unwatch <id>")
		}
		return p.handleUnwatch(ctx, strings.TrimSpace(parts[1]))
	default:
		return p.handleMovieSearch(ctx, args)
	}
}

func (p *MoviesPlugin) handleMovieSearch(ctx MessageContext, query string) error {
	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Movie lookups are not configured (missing API key).")
	}

	detail, err := p.searchAndFetchMovie(query)
	if err != nil {
		slog.Error("movies: search failed", "query", query, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for movie.")
	}
	if detail == nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No movies found for \"%s\".", query))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, p.formatMovie(detail))
}

func (p *MoviesPlugin) searchAndFetchMovie(query string) (*tmdbMovieDetail, error) {
	encoded := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/movie?api_key=%s&query=%s&page=1",
		p.apiKey, encoded)

	var searchResp tmdbSearchResponse
	if err := p.tmdbGet(apiURL, &searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.Results) == 0 {
		return nil, nil
	}

	movieID := searchResp.Results[0].ID
	return p.fetchMovieDetail(movieID)
}

func (p *MoviesPlugin) fetchMovieDetail(movieID int) (*tmdbMovieDetail, error) {
	d := db.Get()

	// Check cache (24h TTL)
	var cached string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM movie_cache WHERE tmdb_id = ?`, movieID,
	).Scan(&cached, &cachedAt)
	if err == nil && time.Now().Unix()-cachedAt < 24*3600 {
		var detail tmdbMovieDetail
		if json.Unmarshal([]byte(cached), &detail) == nil {
			return &detail, nil
		}
	}

	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d?api_key=%s", movieID, p.apiKey)
	var detail tmdbMovieDetail
	if err := p.tmdbGet(apiURL, &detail); err != nil {
		return nil, err
	}

	// Update cache
	data, _ := json.Marshal(detail)
	_, err = d.Exec(
		`INSERT INTO movie_cache (tmdb_id, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(tmdb_id) DO UPDATE SET data = ?, cached_at = ?`,
		movieID, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	)
	if err != nil {
		slog.Error("movies: cache write", "err", err)
	}

	return &detail, nil
}

func (p *MoviesPlugin) formatMovie(m *tmdbMovieDetail) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s", m.Title))
	if m.ReleaseDate != "" {
		if t, err := time.Parse("2006-01-02", m.ReleaseDate); err == nil {
			sb.WriteString(fmt.Sprintf(" (%d)", t.Year()))
		}
	}
	sb.WriteString("\n")

	if m.Tagline != "" {
		sb.WriteString(fmt.Sprintf("\"%s\"\n", m.Tagline))
	}

	if m.VoteAverage > 0 {
		sb.WriteString(fmt.Sprintf("Rating: %.1f/10 (%d votes)\n", m.VoteAverage, m.VoteCount))
	}

	if m.Runtime > 0 {
		sb.WriteString(fmt.Sprintf("Runtime: %dh %dm\n", m.Runtime/60, m.Runtime%60))
	}

	if m.Status != "" {
		sb.WriteString(fmt.Sprintf("Status: %s\n", m.Status))
	}

	if len(m.Genres) > 0 {
		genres := make([]string, len(m.Genres))
		for i, g := range m.Genres {
			genres[i] = g.Name
		}
		sb.WriteString(fmt.Sprintf("Genres: %s\n", strings.Join(genres, ", ")))
	}

	if m.Overview != "" {
		overview := m.Overview
		if len(overview) > 400 {
			overview = overview[:400] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", overview))
	}

	sb.WriteString(fmt.Sprintf("\nhttps://www.themoviedb.org/movie/%d", m.ID))
	return sb.String()
}

func (p *MoviesPlugin) handleTV(ctx MessageContext) error {
	query := p.GetArgs(ctx.Body, "tv")
	if query == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !tv <title>")
	}

	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "TV lookups are not configured (missing API key).")
	}

	// Check 24h cache
	cacheKey := "tv:" + strings.ToLower(query)
	if cached := db.CacheGet(cacheKey, 86400); cached != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, cached)
	}

	encoded := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/tv?api_key=%s&query=%s&page=1",
		p.apiKey, encoded)

	var searchResp tmdbSearchResponse
	if err := p.tmdbGet(apiURL, &searchResp); err != nil {
		slog.Error("movies: tv search failed", "query", query, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for TV show.")
	}

	if len(searchResp.Results) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No TV shows found for \"%s\".", query))
	}

	tvID := searchResp.Results[0].ID
	detail, err := p.fetchTVDetail(tvID)
	if err != nil {
		slog.Error("movies: tv detail fetch failed", "id", tvID, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch TV show details.")
	}

	msg := p.formatTV(detail)
	db.CacheSet(cacheKey, msg)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *MoviesPlugin) fetchTVDetail(tvID int) (*tmdbTVDetail, error) {
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d?api_key=%s", tvID, p.apiKey)
	var detail tmdbTVDetail
	if err := p.tmdbGet(apiURL, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (p *MoviesPlugin) formatTV(tv *tmdbTVDetail) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s", tv.Name))
	if tv.FirstAirDate != "" {
		if t, err := time.Parse("2006-01-02", tv.FirstAirDate); err == nil {
			sb.WriteString(fmt.Sprintf(" (%d)", t.Year()))
		}
	}
	sb.WriteString("\n")

	if tv.Tagline != "" {
		sb.WriteString(fmt.Sprintf("\"%s\"\n", tv.Tagline))
	}

	if tv.VoteAverage > 0 {
		sb.WriteString(fmt.Sprintf("Rating: %.1f/10 (%d votes)\n", tv.VoteAverage, tv.VoteCount))
	}

	sb.WriteString(fmt.Sprintf("Seasons: %d | Episodes: %d\n", tv.NumSeasons, tv.NumEpisodes))

	if tv.Status != "" {
		sb.WriteString(fmt.Sprintf("Status: %s\n", tv.Status))
	}

	if len(tv.Genres) > 0 {
		genres := make([]string, len(tv.Genres))
		for i, g := range tv.Genres {
			genres[i] = g.Name
		}
		sb.WriteString(fmt.Sprintf("Genres: %s\n", strings.Join(genres, ", ")))
	}

	if len(tv.Networks) > 0 {
		networks := make([]string, len(tv.Networks))
		for i, n := range tv.Networks {
			networks[i] = n.Name
		}
		sb.WriteString(fmt.Sprintf("Networks: %s\n", strings.Join(networks, ", ")))
	}

	if tv.Overview != "" {
		overview := tv.Overview
		if len(overview) > 400 {
			overview = overview[:400] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", overview))
	}

	sb.WriteString(fmt.Sprintf("\nhttps://www.themoviedb.org/tv/%d", tv.ID))
	return sb.String()
}

func (p *MoviesPlugin) handleWatchMovie(ctx MessageContext, title string) error {
	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Movie lookups are not configured (missing API key).")
	}

	encoded := url.QueryEscape(title)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/movie?api_key=%s&query=%s&page=1",
		p.apiKey, encoded)

	var searchResp tmdbSearchResponse
	if err := p.tmdbGet(apiURL, &searchResp); err != nil {
		slog.Error("movies: watch search failed", "title", title, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for movie.")
	}

	if len(searchResp.Results) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No movies found for \"%s\".", title))
	}

	result := searchResp.Results[0]
	d := db.Get()

	_, err := d.Exec(
		`INSERT INTO movie_watchlist (user_id, tmdb_id, title, media_type, room_id) VALUES (?, ?, ?, 'movie', ?)
		 ON CONFLICT(user_id, tmdb_id) DO NOTHING`,
		string(ctx.Sender), result.ID, result.Title, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("movies: watchlist add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add to watchlist.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Added \"%s\" (TMDB ID: %d) to your movie watchlist.", result.Title, result.ID))
}

func (p *MoviesPlugin) handleWatching(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT tmdb_id, title, media_type FROM movie_watchlist WHERE user_id = ? ORDER BY title`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("movies: watching list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watchlist.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Your Movie/TV Watchlist:\n\n")
	count := 0
	for rows.Next() {
		var tmdbID int
		var title, mediaType string
		if err := rows.Scan(&tmdbID, &title, &mediaType); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s (%s)\n", tmdbID, title, mediaType))
		count++
	}

	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Your movie watchlist is empty. Use !movie watch <title> to add movies.")
	}

	sb.WriteString("\nUse !movie unwatch <id> to remove an entry.")
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MoviesPlugin) handleUnwatch(ctx MessageContext, idStr string) error {
	tmdbID, err := strconv.Atoi(idStr)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Please provide a valid TMDB ID number. Check !movie watching for your list.")
	}

	d := db.Get()
	res, err := d.Exec(
		`DELETE FROM movie_watchlist WHERE user_id = ? AND tmdb_id = ?`,
		string(ctx.Sender), tmdbID,
	)
	if err != nil {
		slog.Error("movies: unwatch", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove from watchlist.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("TMDB ID %d is not in your watchlist.", tmdbID))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Removed TMDB ID %d from your watchlist.", tmdbID))
}

func (p *MoviesPlugin) handleUpcoming(ctx MessageContext) error {
	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Movie lookups are not configured (missing API key).")
	}

	// Check 6h cache
	cacheKey := "upcoming_movies"
	if cached := db.CacheGet(cacheKey, 86400); cached != "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, cached)
	}

	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/upcoming?api_key=%s&region=US&page=1", p.apiKey)

	var resp tmdbUpcomingResponse
	if err := p.tmdbGet(apiURL, &resp); err != nil {
		slog.Error("movies: upcoming fetch failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to fetch upcoming movies.")
	}

	if len(resp.Results) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No upcoming movies found.")
	}

	limit := 10
	if len(resp.Results) < limit {
		limit = len(resp.Results)
	}

	var sb strings.Builder
	sb.WriteString("Upcoming Movies:\n\n")

	for i := 0; i < limit; i++ {
		r := resp.Results[i]
		title := r.Title
		if title == "" {
			title = r.Name
		}
		dateStr := r.ReleaseDate
		if dateStr == "" {
			dateStr = "TBA"
		}
		ratingStr := ""
		if r.VoteAverage > 0 {
			ratingStr = fmt.Sprintf(" - %.1f/10", r.VoteAverage)
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s)%s\n", i+1, title, dateStr, ratingStr))
	}

	msg := sb.String()
	db.CacheSet(cacheKey, msg)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *MoviesPlugin) tmdbGet(apiURL string, target interface{}) error {
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
		return fmt.Errorf("tmdb returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// PostDailyReleases checks release dates against watchlist and DMs users about releases.
// Intended to be called by the scheduler daily.
func (p *MoviesPlugin) PostDailyReleases(roomID id.RoomID) {
	if p.apiKey == "" {
		slog.Warn("movies: skipping daily releases, no API key configured")
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	todayKey := fmt.Sprintf("%s:%s", today, roomID)
	if db.JobCompleted("movie_releases", todayKey) {
		slog.Info("movies: already sent daily releases", "date", todayKey)
		return
	}
	success := false
	defer func() {
		if success {
			db.MarkJobCompleted("movie_releases", todayKey)
		}
	}()

	d := db.Get()

	// Get all unique TMDB IDs from watchlists
	rows, err := d.Query(`SELECT DISTINCT tmdb_id, title, media_type FROM movie_watchlist`)
	if err != nil {
		slog.Error("movies: daily releases query", "err", err)
		return
	}

	type watchedItem struct {
		tmdbID    int
		title     string
		mediaType string
	}
	var items []watchedItem
	for rows.Next() {
		var item watchedItem
		if err := rows.Scan(&item.tmdbID, &item.title, &item.mediaType); err != nil {
			continue
		}
		items = append(items, item)
	}
	rows.Close()

	for _, item := range items {
		if item.mediaType != "movie" {
			continue
		}

		detail, err := p.fetchMovieDetail(item.tmdbID)
		if err != nil {
			slog.Error("movies: daily release fetch", "tmdb_id", item.tmdbID, "err", err)
			continue
		}

		if detail.ReleaseDate != today {
			continue
		}

		// Find watchers
		wRows, err := d.Query(
			`SELECT user_id FROM movie_watchlist WHERE tmdb_id = ?`, item.tmdbID,
		)
		if err != nil {
			continue
		}

		msg := fmt.Sprintf("Release day! \"%s\" is out today!\n%s\nhttps://www.themoviedb.org/movie/%d",
			detail.Title, detail.Tagline, detail.ID)

		for wRows.Next() {
			var uid string
			if err := wRows.Scan(&uid); err != nil {
				continue
			}
			if err := p.SendDM(id.UserID(uid), msg); err != nil {
				slog.Error("movies: DM watcher", "user", uid, "err", err)
			}
		}
		wRows.Close()
	}

	success = true
	slog.Info("movies: daily releases check completed")
}
