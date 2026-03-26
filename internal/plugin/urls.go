package plugin

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gogobee/internal/db"

	"github.com/PuerkitoBio/goquery"
	"maunium.net/go/mautrix"
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// URLsPlugin detects URLs in messages and previews og:title/og:description.
type URLsPlugin struct {
	Base
	enabled    bool
	httpClient *http.Client
}

// NewURLsPlugin creates a new URL preview plugin.
func NewURLsPlugin(client *mautrix.Client) *URLsPlugin {
	enabled := os.Getenv("FEATURE_URL_PREVIEW") != ""
	return &URLsPlugin{
		Base:    NewBase(client),
		enabled: enabled,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (p *URLsPlugin) Name() string { return "urls" }

func (p *URLsPlugin) Commands() []CommandDef {
	return nil // No commands — purely passive
}

func (p *URLsPlugin) Init() error { return nil }

func (p *URLsPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *URLsPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}

	// Skip command messages
	if ctx.IsCommand {
		return nil
	}

	allURLs := urlRe.FindAllString(ctx.Body, -1)
	// Filter out Matrix internal links (user mentions, room links, etc.)
	var urls []string
	for _, u := range allURLs {
		if !strings.Contains(u, "matrix.to/") {
			urls = append(urls, u)
		}
	}

	// Only preview if there is exactly one URL; skip multi-URL messages
	if len(urls) != 1 {
		return nil
	}

	go p.previewURL(ctx, urls[0])
	return nil
}

func (p *URLsPlugin) previewURL(ctx MessageContext, targetURL string) {
	title, desc, err := p.fetchPreview(targetURL)
	if err != nil {
		slog.Debug("urls: fetch preview failed", "url", targetURL, "err", err)
		return
	}

	if title == "" && desc == "" {
		return
	}

	var preview strings.Builder
	if title != "" {
		preview.WriteString(fmt.Sprintf("Title: %s", title))
	}
	if desc != "" {
		if preview.Len() > 0 {
			preview.WriteString("\n")
		}
		// Truncate long descriptions
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		preview.WriteString(fmt.Sprintf("Description: %s", desc))
	}

	if err := p.SendReply(ctx.RoomID, ctx.EventID, preview.String()); err != nil {
		slog.Error("urls: send preview", "err", err)
	}
}

// fetchPreview retrieves og:title and og:description, checking cache first.
func (p *URLsPlugin) fetchPreview(rawURL string) (string, string, error) {
	d := db.Get()
	now := time.Now().UTC().Unix()
	cacheTTL := int64(24 * 60 * 60)

	// Check cache
	var title, desc string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT title, description, cached_at FROM url_cache WHERE url = ?`, rawURL,
	).Scan(&title, &desc, &cachedAt)
	if err == nil && now-cachedAt < cacheTTL {
		return title, desc, nil
	}

	// Fetch from web
	title, desc, err = p.scrapeOG(rawURL)
	if err != nil {
		return "", "", err
	}

	// Cache the result
	db.Exec("urls: cache write",
		`INSERT INTO url_cache (url, title, description, cached_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET title = ?, description = ?, cached_at = ?`,
		rawURL, title, desc, now, title, desc, now,
	)

	return title, desc, nil
}

// scrapeOG fetches a URL and extracts og:title and og:description.
func (p *URLsPlugin) scrapeOG(rawURL string) (string, string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "GogoBee Bot/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("parse HTML: %w", err)
	}

	title := ""
	desc := ""

	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		prop, _ := s.Attr("property")
		content, _ := s.Attr("content")
		switch prop {
		case "og:title":
			title = content
		case "og:description":
			desc = content
		}
	})

	// Fallback to <title> tag if no og:title
	if title == "" {
		title = doc.Find("title").First().Text()
	}

	// Fallback to meta description
	if desc == "" {
		doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
			name, _ := s.Attr("name")
			if strings.EqualFold(name, "description") {
				content, _ := s.Attr("content")
				desc = content
			}
		})
	}

	return strings.TrimSpace(title), strings.TrimSpace(desc), nil
}
