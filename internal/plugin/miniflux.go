package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// MinifluxPlugin polls a self-hosted Miniflux RSS instance for new feed entries
// and routes them to configured Matrix rooms.
type MinifluxPlugin struct {
	Base
	httpClient *http.Client

	mu              sync.Mutex
	enabled         bool
	baseURL         string
	apiKey          string
	pollInterval    int // minutes
	defaultRoom     string
	maxPerPoll      int
	feedFailCounts  map[int64]int // per-feed consecutive failure counter
	pollingDisabled bool          // set true on 401
}

func NewMinifluxPlugin(client *mautrix.Client) *MinifluxPlugin {
	return &MinifluxPlugin{
		Base: NewBase(client),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *MinifluxPlugin) Name() string { return "miniflux" }

func (p *MinifluxPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "rss", Description: "Miniflux RSS feed routing", Usage: "!rss feeds · !rss subscribe <feed_id> [#room] · !rss unsubscribe <feed_id> · !rss subscriptions · !rss latest <feed_id> · !rss pause <feed_id> · !rss resume <feed_id> · !rss status", Category: "Automation"},
	}
}

func (p *MinifluxPlugin) Init() error {
	p.baseURL = strings.TrimRight(os.Getenv("MINIFLUX_URL"), "/")
	p.apiKey = os.Getenv("MINIFLUX_API_KEY")

	if p.baseURL == "" || p.apiKey == "" {
		slog.Info("miniflux: disabled (MINIFLUX_URL or MINIFLUX_API_KEY not set)")
		return nil
	}

	p.pollInterval = 15
	if v := os.Getenv("MINIFLUX_POLL_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.pollInterval = n
		}
	}

	p.defaultRoom = os.Getenv("MINIFLUX_DEFAULT_ROOM")
	if p.defaultRoom == "" {
		p.defaultRoom = os.Getenv("BROADCAST_ROOMS")
		if idx := strings.Index(p.defaultRoom, ","); idx > 0 {
			p.defaultRoom = p.defaultRoom[:idx]
		}
	}
	p.defaultRoom = strings.TrimSpace(p.defaultRoom)

	p.maxPerPoll = 5
	if v := os.Getenv("MINIFLUX_MAX_PER_POLL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.maxPerPoll = n
		}
	}

	p.feedFailCounts = make(map[int64]int)
	p.enabled = true
	slog.Info("miniflux: initialized", "url", p.baseURL, "poll_interval", p.pollInterval, "max_per_poll", p.maxPerPoll)
	return nil
}

func (p *MinifluxPlugin) OnReaction(_ ReactionContext) error { return nil }

// PollInterval returns the configured polling interval in minutes.
func (p *MinifluxPlugin) PollInterval() int { return p.pollInterval }

func (p *MinifluxPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "rss") {
		return nil
	}

	if !p.enabled {
		return p.SendReply(ctx.RoomID, ctx.EventID, "RSS feed routing is disabled (MINIFLUX_URL not configured).")
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "rss"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!rss feeds` · `!rss subscribe <feed_id> [#room]` · `!rss unsubscribe <feed_id>` · `!rss subscriptions` · `!rss latest <feed_id>` · `!rss pause <feed_id>` · `!rss resume <feed_id>` · `!rss status`")
	}

	parts := strings.Fields(args)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "feeds":
		go func() {
			if err := p.cmdFeeds(ctx); err != nil {
				slog.Error("miniflux: feeds error", "err", err)
			}
		}()
		return nil
	case "subscribe":
		if !p.IsAdmin(ctx.Sender) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can manage subscriptions.")
		}
		return p.cmdSubscribe(ctx, parts[1:])
	case "unsubscribe":
		if !p.IsAdmin(ctx.Sender) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can manage subscriptions.")
		}
		return p.cmdUnsubscribe(ctx, parts[1:])
	case "subscriptions":
		return p.cmdSubscriptions(ctx)
	case "latest":
		go func() {
			if err := p.cmdLatest(ctx, parts[1:]); err != nil {
				slog.Error("miniflux: latest error", "err", err)
			}
		}()
		return nil
	case "pause":
		if !p.IsAdmin(ctx.Sender) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can manage subscriptions.")
		}
		return p.cmdPause(ctx, parts[1:])
	case "resume":
		if !p.IsAdmin(ctx.Sender) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can manage subscriptions.")
		}
		return p.cmdResume(ctx, parts[1:])
	case "status":
		if !p.IsAdmin(ctx.Sender) {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Only admins can view polling status.")
		}
		return p.cmdStatus(ctx)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unknown subcommand `%s`. Try `!rss` for usage.", parts[0]))
	}
}

// ── Miniflux API Types ──────────────────────────────────────────────────────

// errMinifluxUnauthorized is returned on 401 to allow reliable type checking.
var errMinifluxUnauthorized = fmt.Errorf("miniflux: 401 unauthorized")

type minifluxFeed struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	SiteURL  string `json:"site_url"`
	Category struct {
		Title string `json:"title"`
	} `json:"category"`
}

type minifluxEntry struct {
	ID          int64  `json:"id"`
	FeedID      int64  `json:"feed_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Content     string `json:"content"`
	Author      string `json:"author"`
	PublishedAt string `json:"published_at"`
	Feed        struct {
		Title string `json:"title"`
	} `json:"feed"`
}

type minifluxFeedsResponse []minifluxFeed

type minifluxEntriesResponse struct {
	Total   int             `json:"total"`
	Entries []minifluxEntry `json:"entries"`
}

// ── API Helpers ─────────────────────────────────────────────────────────────

func (p *MinifluxPlugin) minifluxGet(endpoint string, result interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	url := p.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("miniflux API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return errMinifluxUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("miniflux API returned %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("miniflux decode error: %w", err)
	}
	return nil
}

func (p *MinifluxPlugin) fetchFeeds() ([]minifluxFeed, error) {
	var feeds minifluxFeedsResponse
	if err := p.minifluxGet("/v1/feeds", &feeds); err != nil {
		return nil, err
	}
	return feeds, nil
}

func (p *MinifluxPlugin) fetchEntries(feedID int64, limit int) ([]minifluxEntry, error) {
	var resp minifluxEntriesResponse
	endpoint := fmt.Sprintf("/v1/feeds/%d/entries?status=unread&limit=%d&order=published_at&direction=asc", feedID, limit)
	if err := p.minifluxGet(endpoint, &resp); err != nil {
		return nil, err
	}
	return resp.Entries, nil
}

// ── Command Handlers ────────────────────────────────────────────────────────

func (p *MinifluxPlugin) cmdFeeds(ctx MessageContext) error {
	feeds, err := p.fetchFeeds()
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not fetch feeds: %v", err))
	}
	if len(feeds) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No feeds found in Miniflux.")
	}

	var sb strings.Builder
	sb.WriteString("**Miniflux Feeds**\n\n")
	for _, f := range feeds {
		cat := f.Category.Title
		if cat == "" {
			cat = "Uncategorized"
		}
		sb.WriteString(fmt.Sprintf("**%d** — %s _%s_\n", f.ID, f.Title, cat))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MinifluxPlugin) cmdSubscribe(ctx MessageContext, args []string) error {
	if len(args) < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!rss subscribe <feed_id> [#room]`")
	}

	feedID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid feed ID `%s`.", args[0]))
	}

	roomID := string(ctx.RoomID)
	if len(args) >= 2 {
		roomID = args[1]
	}

	now := time.Now().Unix()
	d := db.Get()
	_, err = d.Exec(
		`INSERT INTO miniflux_subscriptions (feed_id, room_id, paused, created_at) VALUES (?, ?, 0, ?)
		 ON CONFLICT(feed_id, room_id) DO UPDATE SET paused = 0`,
		feedID, roomID, now,
	)
	if err != nil {
		slog.Error("miniflux: subscribe insert", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to save subscription.")
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Subscribed feed **%d** to room `%s`.", feedID, roomID))
}

func (p *MinifluxPlugin) cmdUnsubscribe(ctx MessageContext, args []string) error {
	if len(args) < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!rss unsubscribe <feed_id>`")
	}

	feedID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid feed ID `%s`.", args[0]))
	}

	res, err := db.Get().Exec(
		`DELETE FROM miniflux_subscriptions WHERE feed_id = ? AND room_id = ?`,
		feedID, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("miniflux: unsubscribe delete", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove subscription.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No subscription found for feed **%d** in this room.", feedID))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unsubscribed feed **%d** from this room.", feedID))
}

func (p *MinifluxPlugin) cmdSubscriptions(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT feed_id, room_id, paused, created_at FROM miniflux_subscriptions WHERE room_id = ?`,
		string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("miniflux: query subscriptions", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to query subscriptions.")
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("**RSS Subscriptions for this room**\n\n")
	count := 0
	for rows.Next() {
		var feedID int64
		var roomID string
		var paused int
		var createdAt int64
		if err := rows.Scan(&feedID, &roomID, &paused, &createdAt); err != nil {
			continue
		}
		status := "active"
		if paused == 1 {
			status = "paused"
		}
		t := time.Unix(createdAt, 0).UTC().Format("Jan 2, 2006")
		sb.WriteString(fmt.Sprintf("Feed **%d** — %s _(since %s)_\n", feedID, status, t))
		count++
	}
	if count == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No RSS subscriptions for this room. Use `!rss subscribe <feed_id>` to add one.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MinifluxPlugin) cmdLatest(ctx MessageContext, args []string) error {
	if len(args) < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!rss latest <feed_id>`")
	}

	feedID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid feed ID `%s`.", args[0]))
	}

	entries, err := p.fetchEntries(feedID, 1)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not fetch entries: %v", err))
	}
	if len(entries) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No unread entries for this feed.")
	}

	msg := formatEntry(entries[0])
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *MinifluxPlugin) cmdPause(ctx MessageContext, args []string) error {
	if len(args) < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!rss pause <feed_id>`")
	}

	feedID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid feed ID `%s`.", args[0]))
	}

	res, err := db.Get().Exec(
		`UPDATE miniflux_subscriptions SET paused = 1 WHERE feed_id = ? AND room_id = ?`,
		feedID, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("miniflux: pause update", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to pause subscription.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No subscription found for feed **%d** in this room.", feedID))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Paused feed **%d** in this room.", feedID))
}

func (p *MinifluxPlugin) cmdResume(ctx MessageContext, args []string) error {
	if len(args) < 1 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!rss resume <feed_id>`")
	}

	feedID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid feed ID `%s`.", args[0]))
	}

	res, err := db.Get().Exec(
		`UPDATE miniflux_subscriptions SET paused = 0 WHERE feed_id = ? AND room_id = ?`,
		feedID, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("miniflux: resume update", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to resume subscription.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No subscription found for feed **%d** in this room.", feedID))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Resumed feed **%d** in this room.", feedID))
}

func (p *MinifluxPlugin) cmdStatus(ctx MessageContext) error {
	p.mu.Lock()
	disabled := p.pollingDisabled
	totalFailing := 0
	for _, v := range p.feedFailCounts {
		if v > 0 {
			totalFailing++
		}
	}
	p.mu.Unlock()

	status := "active"
	if disabled {
		status = "DISABLED (401 unauthorized)"
	} else if totalFailing > 0 {
		status = fmt.Sprintf("degraded (%d feeds with errors)", totalFailing)
	}

	// Count subscriptions
	var total, active, paused int
	d := db.Get()
	_ = d.QueryRow(`SELECT COUNT(*) FROM miniflux_subscriptions`).Scan(&total)
	_ = d.QueryRow(`SELECT COUNT(*) FROM miniflux_subscriptions WHERE paused = 0`).Scan(&active)
	_ = d.QueryRow(`SELECT COUNT(*) FROM miniflux_subscriptions WHERE paused = 1`).Scan(&paused)

	var seenCount int
	_ = d.QueryRow(`SELECT COUNT(*) FROM miniflux_seen`).Scan(&seenCount)

	msg := fmt.Sprintf("**RSS Polling Status**\n\n"+
		"Status: **%s**\n"+
		"Poll interval: %d minutes\n"+
		"Max per poll: %d\n"+
		"Subscriptions: %d total (%d active, %d paused)\n"+
		"Seen entries tracked: %d",
		status, p.pollInterval, p.maxPerPoll, total, active, paused, seenCount)

	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// ── Entry Formatting ────────────────────────────────────────────────────────

func formatEntry(e minifluxEntry) string {
	feedName := e.Feed.Title
	if feedName == "" {
		feedName = "RSS"
	}

	summary := stripHTMLTags(e.Content)
	summary = truncateAtWordBoundary(summary, 280)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📰 %s\n\n", feedName))
	sb.WriteString(fmt.Sprintf("**%s**\n", e.Title))
	sb.WriteString(e.URL + "\n\n")
	if summary != "" {
		sb.WriteString(fmt.Sprintf("> %s\n\n", summary))
	}

	// Build attribution line
	var attr []string
	if e.Author != "" {
		attr = append(attr, e.Author)
	}
	if e.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, e.PublishedAt); err == nil {
			attr = append(attr, timeAgo(t))
		}
	}
	if len(attr) > 0 {
		sb.WriteString("— " + strings.Join(attr, " · "))
	}

	return sb.String()
}

// truncateAtWordBoundary truncates s to at most maxLen characters, breaking at
// a word boundary, and appends an ellipsis if truncated.
func truncateAtWordBoundary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find last space before maxLen
	truncated := s[:maxLen]
	if idx := strings.LastIndex(truncated, " "); idx > 0 {
		truncated = truncated[:idx]
	}
	return truncated + "…"
}

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// ── Polling (called by scheduler) ───────────────────────────────────────────

// MinifluxPoll runs one poll cycle: fetches new entries for all active
// subscriptions and posts them to the configured rooms. Safe to call from
// the scheduler goroutine.
func MinifluxPoll(client *mautrix.Client) {
	p := findMinifluxPlugin(client)
	if p == nil {
		return
	}
	p.poll()
}

// minifluxPluginInstance caches the singleton so the scheduler can find it.
var (
	minifluxPluginInstance   *MinifluxPlugin
	minifluxPluginInstanceMu sync.Mutex
)

// RegisterMinifluxPlugin stores the plugin instance for scheduler access.
func RegisterMinifluxPlugin(p *MinifluxPlugin) {
	minifluxPluginInstanceMu.Lock()
	minifluxPluginInstance = p
	minifluxPluginInstanceMu.Unlock()
}

func findMinifluxPlugin(_ *mautrix.Client) *MinifluxPlugin {
	minifluxPluginInstanceMu.Lock()
	defer minifluxPluginInstanceMu.Unlock()
	return minifluxPluginInstance
}

func (p *MinifluxPlugin) poll() {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	if p.pollingDisabled {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	// Get all active (non-paused) subscriptions
	d := db.Get()
	rows, err := d.Query(`SELECT DISTINCT feed_id, room_id FROM miniflux_subscriptions WHERE paused = 0`)
	if err != nil {
		slog.Error("miniflux: poll query subscriptions", "err", err)
		return
	}

	type sub struct {
		feedID int64
		roomID string
	}
	var subs []sub
	for rows.Next() {
		var s sub
		if err := rows.Scan(&s.feedID, &s.roomID); err != nil {
			continue
		}
		subs = append(subs, s)
	}
	rows.Close() // close explicitly before HTTP work

	if len(subs) == 0 {
		return
	}

	// Group subscriptions by feed ID
	feedRooms := make(map[int64][]string)
	for _, s := range subs {
		feedRooms[s.feedID] = append(feedRooms[s.feedID], s.roomID)
	}

	for feedID, rooms := range feedRooms {
		entries, err := p.fetchEntries(feedID, p.maxPerPoll)
		if err != nil {
			p.handlePollError(feedID, err)
			slog.Error("miniflux: poll fetch entries", "feed_id", feedID, "err", err)
			continue
		}

		// Reset consecutive failures for this feed on success
		p.mu.Lock()
		delete(p.feedFailCounts, feedID)
		p.mu.Unlock()

		for _, entry := range entries {
			// Check if already seen
			if p.isEntrySeen(feedID, entry.ID) {
				continue
			}

			// Check if this is a first-time subscription and entry is older than 24h
			if p.isEntryTooOld(entry) {
				p.markEntrySeen(feedID, entry.ID)
				continue
			}

			// Post to all subscribed rooms
			msg := formatEntry(entry)
			for _, roomID := range rooms {
				if err := p.SendMessage(id.RoomID(roomID), msg); err != nil {
					slog.Error("miniflux: post entry", "feed_id", feedID, "entry_id", entry.ID, "room", roomID, "err", err)
				}
			}

			p.markEntrySeen(feedID, entry.ID)
		}
	}
}

func (p *MinifluxPlugin) handlePollError(feedID int64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// On 401, disable polling immediately and alert admins
	if errors.Is(err, errMinifluxUnauthorized) {
		p.pollingDisabled = true
		slog.Error("miniflux: 401 unauthorized, disabling all polling")
		go p.alertAdmins("RSS polling has been **disabled** due to a 401 Unauthorized response from Miniflux. Check MINIFLUX_API_KEY.")
		return
	}

	p.feedFailCounts[feedID]++
	if p.feedFailCounts[feedID] == 5 {
		slog.Error("miniflux: 5 consecutive poll failures for feed", "feed_id", feedID)
		go p.alertAdmins(fmt.Sprintf("RSS feed **%d** has failed **5 consecutive polls**. Latest error: %v", feedID, err))
	}
}

func (p *MinifluxPlugin) alertAdmins(msg string) {
	admins := os.Getenv("ADMIN_USERS")
	if admins == "" {
		return
	}
	for _, a := range strings.Split(admins, ",") {
		userID := id.UserID(strings.TrimSpace(a))
		if userID == "" {
			continue
		}
		if err := p.SendDM(userID, msg); err != nil {
			slog.Error("miniflux: failed to DM admin", "admin", userID, "err", err)
		}
	}
}

func (p *MinifluxPlugin) isEntrySeen(feedID, entryID int64) bool {
	var count int
	err := db.Get().QueryRow(
		`SELECT COUNT(*) FROM miniflux_seen WHERE feed_id = ? AND entry_id = ?`,
		feedID, entryID,
	).Scan(&count)
	return err == nil && count > 0
}

func (p *MinifluxPlugin) markEntrySeen(feedID, entryID int64) {
	db.Exec("miniflux mark seen",
		`INSERT OR IGNORE INTO miniflux_seen (feed_id, entry_id, seen_at) VALUES (?, ?, ?)`,
		feedID, entryID, time.Now().Unix(),
	)
}

// isEntryTooOld returns true if the entry was published more than 24h ago.
// Used to skip old entries on first subscription.
func (p *MinifluxPlugin) isEntryTooOld(entry minifluxEntry) bool {
	if entry.PublishedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, entry.PublishedAt)
	if err != nil {
		return false
	}
	return time.Since(t) > 24*time.Hour
}

// ── Maintenance ─────────────────────────────────────────────────────────────

// MinifluxPurgeSeen removes miniflux_seen entries older than 7 days.
// Intended to be called from the maintenance scheduler.
func MinifluxPurgeSeen() {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	res, err := db.Get().Exec(`DELETE FROM miniflux_seen WHERE seen_at < ?`, cutoff)
	if err != nil {
		slog.Error("miniflux: purge seen", "err", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		slog.Info("miniflux: purged seen entries", "rows", n)
	}
}
