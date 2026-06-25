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
	"gogobee/internal/safehttp"

	"github.com/PuerkitoBio/goquery"
	"maunium.net/go/mautrix"
)

var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// URLsPlugin detects URLs in messages and previews og:title/og:description.
type URLsPlugin struct {
	Base
	enabled     bool
	ignoreUsers map[string]struct{}
	httpClient  *http.Client
}

// NewURLsPlugin creates a new URL preview plugin.
func NewURLsPlugin(client *mautrix.Client) *URLsPlugin {
	enabled := os.Getenv("FEATURE_URL_PREVIEW") != ""
	ignore := make(map[string]struct{})
	if raw := os.Getenv("URL_PREVIEW_IGNORE_USERS"); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			if u = strings.TrimSpace(u); u != "" {
				ignore[u] = struct{}{}
			}
		}
	}
	return &URLsPlugin{
		Base:        NewBase(client),
		enabled:     enabled,
		ignoreUsers: ignore,
		// Route page scrapes through safehttp so a posted link can't steer the
		// fetch at loopback / RFC1918 / cloud-metadata IPs (re-checked on every
		// redirect), and can't OOM the parser by streaming an unbounded body.
		httpClient: safehttp.NewClient(8 * time.Second),
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

	// Skip ignored users (e.g. news bots)
	if _, ok := p.ignoreUsers[string(ctx.Sender)]; ok {
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

	safeGo("url-preview", func() { p.previewURL(ctx, urls[0]) })
	return nil
}

func (p *URLsPlugin) previewURL(ctx MessageContext, targetURL string) {
	title, desc, image, err := p.fetchPreview(targetURL)
	if err != nil {
		slog.Debug("urls: fetch preview failed", "url", targetURL, "err", err)
		return
	}

	if title == "" && desc == "" && image == "" {
		return
	}

	// Post the thumbnail first (best-effort), so it renders above the text.
	if image != "" && validateImageURL(image) {
		if err := p.SendImageFromURL(ctx.RoomID, image); err != nil {
			slog.Debug("urls: thumbnail post failed", "url", targetURL, "image", image, "err", err)
		}
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

// fetchPreview retrieves og:title, og:description and og:image, checking cache first.
func (p *URLsPlugin) fetchPreview(rawURL string) (title, desc, image string, err error) {
	d := db.Get()
	now := time.Now().UTC().Unix()
	cacheTTL := int64(24 * 60 * 60)

	// Check cache
	var cachedAt int64
	err = d.QueryRow(
		`SELECT title, description, image_url, cached_at FROM url_cache WHERE url = ?`, rawURL,
	).Scan(&title, &desc, &image, &cachedAt)
	if err == nil && now-cachedAt < cacheTTL {
		return title, desc, image, nil
	}

	// Fetch from web
	title, desc, image, err = p.scrapeOG(rawURL)
	if err != nil {
		return "", "", "", err
	}

	// Cache the result
	db.Exec("urls: cache write",
		`INSERT INTO url_cache (url, title, description, image_url, cached_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET title = ?, description = ?, image_url = ?, cached_at = ?`,
		rawURL, title, desc, image, now, title, desc, image, now,
	)

	return title, desc, image, nil
}

// scrapeOG fetches a URL and extracts og:title, og:description and og:image.
func (p *URLsPlugin) scrapeOG(rawURL string) (string, string, string, error) {
	if err := safehttp.ValidateURL(rawURL); err != nil {
		return "", "", "", err
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "GogoBee Bot/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	// Cap the parsed body at 2 MiB — og: tags live in <head>, near the top.
	doc, err := goquery.NewDocumentFromReader(safehttp.LimitedBody(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", "", fmt.Errorf("parse HTML: %w", err)
	}

	title := ""
	desc := ""
	image := ""

	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		prop, _ := s.Attr("property")
		content, _ := s.Attr("content")
		switch prop {
		case "og:title":
			title = content
		case "og:description":
			desc = content
		case "og:image:secure_url", "og:image:url", "og:image":
			if image == "" && strings.TrimSpace(content) != "" {
				image = content
			}
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

	if image != "" {
		image = normalizeImageURL(resolveURL(rawURL, strings.TrimSpace(image)))
	}

	return strings.TrimSpace(title), strings.TrimSpace(desc), image, nil
}
