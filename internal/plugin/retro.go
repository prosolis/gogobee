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
)

// rawgGame represents a game from the RAWG API.
type rawgGame struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Released    string  `json:"released"`
	Rating      float64 `json:"rating"`
	RatingTop   int     `json:"rating_top"`
	Metacritic  int     `json:"metacritic"`
	Playtime    int     `json:"playtime"`
	Description string  `json:"description_raw"`
	Platforms   []struct {
		Platform struct {
			Name string `json:"name"`
		} `json:"platform"`
	} `json:"platforms"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Developers []struct {
		Name string `json:"name"`
	} `json:"developers"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
}

// rawgSearchResponse is the RAWG search API response.
type rawgSearchResponse struct {
	Count   int        `json:"count"`
	Results []rawgGame `json:"results"`
}

// retroCacheEntry holds cached retro search data.
type retroCacheEntry struct {
	Results []rawgGame `json:"results"`
	Detail  *rawgGame  `json:"detail,omitempty"`
}

// RetroPlugin provides retro game lookups via the RAWG API.
type RetroPlugin struct {
	Base
	apiKey     string
	httpClient *http.Client
}

// NewRetroPlugin creates a new RetroPlugin.
func NewRetroPlugin(client *mautrix.Client) *RetroPlugin {
	return &RetroPlugin{
		Base:   NewBase(client),
		apiKey: os.Getenv("RAWG_API_KEY"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *RetroPlugin) Name() string { return "retro" }

func (p *RetroPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "game", Description: "Search for a game", Usage: "!game <query>", Category: "Entertainment"},
		{Name: "retro", Description: "Search for a game (alias)", Usage: "!retro <query>", Category: "Entertainment"},
	}
}

func (p *RetroPlugin) Init() error { return nil }

func (p *RetroPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *RetroPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "game") {
		return p.handleSearch(ctx, p.GetArgs(ctx.Body, "game"))
	}
	if p.IsCommand(ctx.Body, "retro") {
		return p.handleSearch(ctx, p.GetArgs(ctx.Body, "retro"))
	}
	return nil
}

func (p *RetroPlugin) handleSearch(ctx MessageContext, query string) error {
	if query == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !game <query> or !retro <query>")
	}

	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Game lookups are not configured (missing API key).")
	}

	entry, err := p.fetchGames(query)
	if err != nil {
		slog.Error("retro: fetch failed", "query", query, "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to search for games.")
	}

	if len(entry.Results) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No games found for \"%s\".", query))
	}

	var sb strings.Builder

	// First result gets detailed info
	if entry.Detail != nil {
		sb.WriteString(p.formatGameDetail(entry.Detail))
	} else {
		sb.WriteString(p.formatGameBrief(entry.Results[0]))
	}

	// Additional results (2nd and 3rd) as brief entries
	limit := 3
	if len(entry.Results) < limit {
		limit = len(entry.Results)
	}
	if limit > 1 {
		sb.WriteString("\n\nOther results:\n")
		for i := 1; i < limit; i++ {
			sb.WriteString(p.formatGameBrief(entry.Results[i]))
			if i < limit-1 {
				sb.WriteString("\n")
			}
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *RetroPlugin) fetchGames(query string) (*retroCacheEntry, error) {
	d := db.Get()
	cacheKey := strings.ToLower(query)

	// Check cache (7-day TTL)
	var cached string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM retro_cache WHERE search_term = ?`, cacheKey,
	).Scan(&cached, &cachedAt)
	if err == nil && time.Now().Unix()-cachedAt < 7*24*3600 {
		var entry retroCacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return &entry, nil
		}
	}

	// Search API
	encoded := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.rawg.io/api/games?key=%s&search=%s&page_size=3",
		p.apiKey, encoded)

	var searchResp rawgSearchResponse
	if err := p.rawgGet(apiURL, &searchResp); err != nil {
		return nil, err
	}

	entry := &retroCacheEntry{
		Results: searchResp.Results,
	}

	// Fetch detailed info for the first result
	if len(searchResp.Results) > 0 {
		detailURL := fmt.Sprintf("https://api.rawg.io/api/games/%d?key=%s",
			searchResp.Results[0].ID, p.apiKey)
		var detail rawgGame
		if err := p.rawgGet(detailURL, &detail); err != nil {
			slog.Warn("retro: detail fetch failed, using search result", "err", err)
		} else {
			entry.Detail = &detail
		}
	}

	// Update cache
	data, _ := json.Marshal(entry)
	_, err = d.Exec(
		`INSERT INTO retro_cache (search_term, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(search_term) DO UPDATE SET data = ?, cached_at = ?`,
		cacheKey, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	)
	if err != nil {
		slog.Error("retro: cache write", "err", err)
	}

	return entry, nil
}

func (p *RetroPlugin) rawgGet(apiURL string, target interface{}) error {
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
		return fmt.Errorf("rawg returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (p *RetroPlugin) formatGameDetail(g *rawgGame) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s\n", g.Name))

	if g.Released != "" {
		sb.WriteString(fmt.Sprintf("Released: %s\n", g.Released))
	}

	if g.Rating > 0 {
		sb.WriteString(fmt.Sprintf("Rating: %.2f/5\n", g.Rating))
	}

	if g.Metacritic > 0 {
		sb.WriteString(fmt.Sprintf("Metacritic: %d\n", g.Metacritic))
	}

	if g.Playtime > 0 {
		sb.WriteString(fmt.Sprintf("Avg Playtime: %dh\n", g.Playtime))
	}

	if len(g.Platforms) > 0 {
		platforms := make([]string, 0, len(g.Platforms))
		for _, pl := range g.Platforms {
			if pl.Platform.Name != "" {
				platforms = append(platforms, pl.Platform.Name)
			}
		}
		if len(platforms) > 0 {
			sb.WriteString(fmt.Sprintf("Platforms: %s\n", strings.Join(platforms, ", ")))
		}
	}

	if len(g.Genres) > 0 {
		genres := make([]string, len(g.Genres))
		for i, gen := range g.Genres {
			genres[i] = gen.Name
		}
		sb.WriteString(fmt.Sprintf("Genres: %s\n", strings.Join(genres, ", ")))
	}

	if len(g.Developers) > 0 {
		devs := make([]string, len(g.Developers))
		for i, d := range g.Developers {
			devs[i] = d.Name
		}
		sb.WriteString(fmt.Sprintf("Developers: %s\n", strings.Join(devs, ", ")))
	}

	if len(g.Publishers) > 0 {
		pubs := make([]string, len(g.Publishers))
		for i, pub := range g.Publishers {
			pubs[i] = pub.Name
		}
		sb.WriteString(fmt.Sprintf("Publishers: %s\n", strings.Join(pubs, ", ")))
	}

	if g.Description != "" {
		desc := g.Description
		if len(desc) > 400 {
			desc = desc[:400] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n%s\n", desc))
	}

	if g.Slug != "" {
		sb.WriteString(fmt.Sprintf("\nhttps://rawg.io/games/%s", g.Slug))
	}

	return sb.String()
}

func (p *RetroPlugin) formatGameBrief(g rawgGame) string {
	parts := []string{g.Name}

	if g.Released != "" {
		parts = append(parts, fmt.Sprintf("(%s)", g.Released))
	}

	if g.Rating > 0 {
		parts = append(parts, fmt.Sprintf("- %.2f/5", g.Rating))
	}

	if len(g.Platforms) > 0 {
		platforms := make([]string, 0, len(g.Platforms))
		for _, pl := range g.Platforms {
			if pl.Platform.Name != "" {
				platforms = append(platforms, pl.Platform.Name)
			}
		}
		if len(platforms) > 0 {
			// Show up to 3 platforms to keep it brief
			if len(platforms) > 3 {
				platforms = append(platforms[:3], "...")
			}
			parts = append(parts, fmt.Sprintf("[%s]", strings.Join(platforms, ", ")))
		}
	}

	return "  " + strings.Join(parts, " ")
}

// Compile-time interface compliance checks.
var _ Plugin = (*RetroPlugin)(nil)
