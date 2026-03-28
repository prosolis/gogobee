package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
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

// ── Plugin ───────────────────────────────────────────────────────────────────

// MarketPlugin provides daily market index snapshots with Ollama-generated
// summaries, market status by exchange hours, and historical trend reports.
type MarketPlugin struct {
	Base
	httpClient  *http.Client
	finnhubKey  string
	mu          sync.Mutex
	cooldowns   map[id.RoomID]time.Time
	enabled     bool
	broadcastOn bool
}

func NewMarketPlugin(client *mautrix.Client) *MarketPlugin {
	enabled := os.Getenv("FEATURE_MARKET") == "true"
	broadcast := os.Getenv("MARKET_BROADCAST_SUMMARY") == "true"
	return &MarketPlugin{
		Base:        NewBase(client),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		finnhubKey:  os.Getenv("FINNHUB_API_KEY"),
		cooldowns:   make(map[id.RoomID]time.Time),
		enabled:     enabled,
		broadcastOn: broadcast,
	}
}

func (p *MarketPlugin) Name() string { return "market" }

func (p *MarketPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "howsthemarket", Description: "Daily market snapshot with commentary", Usage: "!howsthemarket", Category: "Finance"},
		{Name: "marketstatus", Description: "Which exchanges are open right now", Usage: "!marketstatus", Category: "Finance"},
		{Name: "marketreport", Description: "Historical market trend reports", Usage: "!marketreport week|month|year|vix|compare <index> <days>", Category: "Finance"},
	}
}

func (p *MarketPlugin) Init() error { return nil }

func (p *MarketPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *MarketPlugin) OnMessage(ctx MessageContext) error {
	if !p.enabled {
		return nil
	}
	if p.IsCommand(ctx.Body, "howsthemarket") {
		return p.handleSnapshot(ctx)
	}
	if p.IsCommand(ctx.Body, "marketstatus") {
		return p.handleStatus(ctx)
	}
	if p.IsCommand(ctx.Body, "marketreport") {
		return p.handleReport(ctx)
	}
	return nil
}

// ── Index Configuration ──────────────────────────────────────────────────────

type marketIndex struct {
	Symbol      string
	DisplayName string
	FallbackETF string
	ShortName   string
}

var marketIndices = []marketIndex{
	{Symbol: "^GSPC", DisplayName: "S&P 500", FallbackETF: "SPY", ShortName: "SP500"},
	{Symbol: "^IXIC", DisplayName: "NASDAQ", FallbackETF: "QQQ", ShortName: "NASDAQ"},
	{Symbol: "^DJI", DisplayName: "Dow Jones", FallbackETF: "DIA", ShortName: "DOW"},
	{Symbol: "^FTSE", DisplayName: "FTSE 100", FallbackETF: "EWU", ShortName: "FTSE"},
	{Symbol: "^N225", DisplayName: "Nikkei 225", FallbackETF: "EWJ", ShortName: "NIKKEI"},
	{Symbol: "PSI20.NX", DisplayName: "PSI 20", FallbackETF: "PGAL", ShortName: "PSI20"},
	{Symbol: "^VIX", DisplayName: "VIX", FallbackETF: "VIXY", ShortName: "VIX"},
}

// marketShortNames maps case-insensitive short names to indices for the compare command.
var marketShortNames map[string]*marketIndex

func init() {
	marketShortNames = make(map[string]*marketIndex, len(marketIndices))
	for i := range marketIndices {
		marketShortNames[strings.ToUpper(marketIndices[i].ShortName)] = &marketIndices[i]
	}
}

// ── Data Types ───────────────────────────────────────────────────────────────

type marketSnapshot struct {
	Symbol      string
	DisplayName string
	Price       *float64 // nil if unavailable
	PrevClose   *float64
	ChangePct   *float64
	Source      string
}

// ── Cooldown ─────────────────────────────────────────────────────────────────

const marketCooldown = 30 * time.Minute

func (p *MarketPlugin) checkCooldown(roomID id.RoomID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	last, ok := p.cooldowns[roomID]
	if ok && time.Since(last) < marketCooldown {
		return false
	}
	p.cooldowns[roomID] = time.Now()
	return true
}

// ── Yahoo Finance Fetcher ────────────────────────────────────────────────────
//
// Yahoo Finance's unofficial API has historically been stable but is not
// guaranteed. If the endpoint format changes, the Finnhub ETF fallback
// handles continuity until the primary is updated.

func (p *MarketPlugin) fetchYahoo(symbol string) (*marketSnapshot, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo HTTP %d", resp.StatusCode)
	}

	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice     float64 `json:"regularMarketPrice"`
					PreviousClose          float64 `json:"previousClose"`
					RegularMarketChangePct float64 `json:"regularMarketChangePercent"`
				} `json:"meta"`
			} `json:"result"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("yahoo decode: %w", err)
	}
	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Code)
	}
	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo: no results")
	}

	meta := result.Chart.Result[0].Meta
	price := meta.RegularMarketPrice
	prevClose := meta.PreviousClose
	changePct := meta.RegularMarketChangePct

	return &marketSnapshot{
		Price:     &price,
		PrevClose: &prevClose,
		ChangePct: &changePct,
		Source:    "yahoo",
	}, nil
}

// ── Finnhub ETF Fallback ─────────────────────────────────────────────────────

func (p *MarketPlugin) fetchFinnhub(etfSymbol string) (*marketSnapshot, error) {
	if p.finnhubKey == "" {
		return nil, fmt.Errorf("no FINNHUB_API_KEY")
	}

	url := fmt.Sprintf("https://finnhub.io/api/v1/quote?symbol=%s&token=%s", etfSymbol, p.finnhubKey)
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("finnhub request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub HTTP %d", resp.StatusCode)
	}

	var q struct {
		Current   float64 `json:"c"`
		PrevClose float64 `json:"pc"`
		ChangePct float64 `json:"dp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&q); err != nil {
		return nil, fmt.Errorf("finnhub decode: %w", err)
	}
	if q.Current == 0 {
		return nil, fmt.Errorf("finnhub: zero price for %s", etfSymbol)
	}

	return &marketSnapshot{
		Price:     &q.Current,
		PrevClose: &q.PrevClose,
		ChangePct: &q.ChangePct,
		Source:    "finnhub_etf",
	}, nil
}

// ── Daily Pull ───────────────────────────────────────────────────────────────

// DailyPull fetches end-of-day data for all indices. Called from cron at 23:00 UTC.
func (p *MarketPlugin) DailyPull() {
	if !p.enabled {
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	if db.JobCompleted("market_pull", today) {
		return
	}

	start := time.Now()
	d := db.Get()
	var fetched, failed int

	for _, idx := range marketIndices {
		snap, err := p.fetchYahoo(idx.Symbol)
		if err != nil {
			slog.Warn("market: yahoo failed, trying finnhub", "symbol", idx.Symbol, "err", err)
			snap, err = p.fetchFinnhub(idx.FallbackETF)
			if err != nil {
				slog.Error("market: both sources failed", "symbol", idx.Symbol, "etf", idx.FallbackETF, "err", err)
				// Store NULL row
				_, _ = d.Exec(`INSERT INTO market_snapshots (snapshot_date, symbol, display_name, price, prev_close, change_pct, source, pulled_at)
					VALUES (?, ?, ?, NULL, NULL, NULL, 'unavailable', ?)
					ON CONFLICT(snapshot_date, symbol) DO UPDATE SET price=NULL, prev_close=NULL, change_pct=NULL, source='unavailable', pulled_at=?`,
					today, idx.Symbol, idx.DisplayName, time.Now().Unix(), time.Now().Unix())
				failed++
				continue
			}
		}

		_, err = d.Exec(`INSERT INTO market_snapshots (snapshot_date, symbol, display_name, price, prev_close, change_pct, source, pulled_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(snapshot_date, symbol) DO UPDATE SET price=?, prev_close=?, change_pct=?, source=?, pulled_at=?`,
			today, idx.Symbol, idx.DisplayName, snap.Price, snap.PrevClose, snap.ChangePct, snap.Source, time.Now().Unix(),
			snap.Price, snap.PrevClose, snap.ChangePct, snap.Source, time.Now().Unix())
		if err != nil {
			slog.Error("market: db insert failed", "symbol", idx.Symbol, "err", err)
			failed++
			continue
		}
		fetched++
	}

	// Generate Ollama summary
	summary := p.generateDailySummary(today)
	if summary != "" {
		_, _ = d.Exec(`INSERT INTO market_daily_summary (snapshot_date, summary, generated_at) VALUES (?, ?, ?)
			ON CONFLICT(snapshot_date) DO UPDATE SET summary=?, generated_at=?`,
			today, summary, time.Now().Unix(), summary, time.Now().Unix())
	}

	db.MarkJobCompleted("market_pull", today)
	slog.Info("market: daily pull complete", "fetched", fetched, "failed", failed, "duration", time.Since(start))

	// Optional broadcast
	if p.broadcastOn {
		gr := gamesRoom()
		if gr != "" {
			text := p.formatSnapshot(today)
			if text != "" {
				_ = p.SendMessage(id.RoomID(gr), text)
			}
		}
	}
}

// ── Ollama Summary Generation ────────────────────────────────────────────────

const marketSystemPrompt = `You are GogoBee, a sardonic but genuinely helpful financial commentator for a retro gaming and anime community chat room.
You summarize global market conditions in 2-3 sentences.
Be dry, witty, and informative. Reference the VIX when describing overall sentiment.
Do not use em dashes. Do not use exclamation marks. Do not offer financial advice.
If markets are closed or data is stale, note it briefly and move on.`

func (p *MarketPlugin) generateDailySummary(date string) string {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host == "" || model == "" {
		return ""
	}

	snapshots := p.loadSnapshots(date)
	if len(snapshots) == 0 {
		return ""
	}

	var prompt strings.Builder
	var notableMovers []string
	var unavailable []string

	for _, s := range snapshots {
		if s.ChangePct != nil && math.Abs(*s.ChangePct) > 2.0 {
			notableMovers = append(notableMovers, fmt.Sprintf("%s %+.2f%%", s.DisplayName, *s.ChangePct))
		}
	}

	if len(notableMovers) > 0 {
		prompt.WriteString(fmt.Sprintf("Notable moves today: %s\n\n", strings.Join(notableMovers, ", ")))
	}

	prompt.WriteString(fmt.Sprintf("Here is today's end-of-day market data (%s, all prices in local currency):\n\n", date))
	for _, s := range snapshots {
		if s.Price != nil && s.ChangePct != nil {
			prompt.WriteString(fmt.Sprintf("%s: %.2f (%+.2f%%)\n", s.DisplayName, *s.Price, *s.ChangePct))
		} else {
			unavailable = append(unavailable, s.DisplayName)
		}
	}
	if len(unavailable) > 0 {
		prompt.WriteString(fmt.Sprintf("\nUnavailable: %s\n", strings.Join(unavailable, ", ")))
	}
	prompt.WriteString("\nWrite a 2-3 sentence summary.")

	result, err := p.callOllamaChat(host, model, marketSystemPrompt, prompt.String())
	if err != nil {
		slog.Error("market: ollama summary failed", "err", err)
		return ""
	}
	return result
}

func (p *MarketPlugin) generateReportSummary(snapsByDate map[string][]marketSnapshot, dateRange string) string {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host == "" || model == "" {
		return ""
	}

	// Build per-index price series
	// Collect sorted dates
	var dates []string
	for d := range snapsByDate {
		dates = append(dates, d)
	}
	// Sort dates
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[j] < dates[i] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}

	indexPrices := make(map[string][]string)
	for _, date := range dates {
		for _, s := range snapsByDate[date] {
			if s.Price != nil {
				indexPrices[s.DisplayName] = append(indexPrices[s.DisplayName], fmt.Sprintf("%.2f", *s.Price))
			}
		}
	}

	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("Here is historical data for the following indices (%s):\n\n", dateRange))
	for name, prices := range indexPrices {
		prompt.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(prices, ", ")))
	}
	prompt.WriteString("\nDescribe the trend in 2-3 sentences. Note any significant moves or divergences between indices.\nBe sardonic but accurate. Reference the VIX trajectory when relevant.")

	result, err := p.callOllamaChat(host, model, marketSystemPrompt, prompt.String())
	if err != nil {
		slog.Warn("market: ollama report summary failed", "err", err)
		return ""
	}
	return result
}

// callOllamaChat calls the Ollama /api/chat endpoint with a system and user message.
// Uses the types already defined in holdem_tips.go (same package).
func (p *MarketPlugin) callOllamaChat(host, model, systemPrompt, userPrompt string) (string, error) {
	req := ollamaChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(host, "/") + "/api/chat"
	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	text := ollamaResp.Message.Content
	// Strip <think>...</think> blocks (reasoning models)
	if i := strings.Index(text, "<think>"); i != -1 {
		if j := strings.Index(text, "</think>"); j != -1 {
			text = text[:i] + text[j+len("</think>"):]
		}
	}

	return strings.TrimSpace(text), nil
}

// ── DB Helpers ───────────────────────────────────────────────────────────────

func (p *MarketPlugin) loadSnapshots(date string) []marketSnapshot {
	d := db.Get()
	rows, err := d.Query(
		`SELECT symbol, display_name, price, prev_close, change_pct, source
		 FROM market_snapshots WHERE snapshot_date = ? ORDER BY ROWID`, date)
	if err != nil {
		slog.Error("market: load snapshots", "err", err)
		return nil
	}
	defer rows.Close()

	var results []marketSnapshot
	for rows.Next() {
		var s marketSnapshot
		var price, prevClose, changePct *float64
		if err := rows.Scan(&s.Symbol, &s.DisplayName, &price, &prevClose, &changePct, &s.Source); err != nil {
			continue
		}
		s.Price = price
		s.PrevClose = prevClose
		s.ChangePct = changePct
		results = append(results, s)
	}
	return results
}

func (p *MarketPlugin) loadSummary(date string) string {
	d := db.Get()
	var summary *string
	err := d.QueryRow(`SELECT summary FROM market_daily_summary WHERE snapshot_date = ?`, date).Scan(&summary)
	if err != nil || summary == nil {
		return ""
	}
	return *summary
}

// loadSnapshotRange returns snapshots grouped by date for a date range.
func (p *MarketPlugin) loadSnapshotRange(startDate, endDate string) map[string][]marketSnapshot {
	d := db.Get()
	rows, err := d.Query(
		`SELECT snapshot_date, symbol, display_name, price, prev_close, change_pct, source
		 FROM market_snapshots WHERE snapshot_date >= ? AND snapshot_date <= ?
		 ORDER BY snapshot_date, ROWID`, startDate, endDate)
	if err != nil {
		slog.Error("market: load range", "err", err)
		return nil
	}
	defer rows.Close()

	result := make(map[string][]marketSnapshot)
	for rows.Next() {
		var date string
		var s marketSnapshot
		var price, prevClose, changePct *float64
		if err := rows.Scan(&date, &s.Symbol, &s.DisplayName, &price, &prevClose, &changePct, &s.Source); err != nil {
			continue
		}
		s.Price = price
		s.PrevClose = prevClose
		s.ChangePct = changePct
		result[date] = append(result[date], s)
	}
	return result
}

// mostRecentSnapshotDate returns the most recent date with data, or empty string.
func (p *MarketPlugin) mostRecentSnapshotDate() string {
	d := db.Get()
	var date string
	err := d.QueryRow(`SELECT snapshot_date FROM market_snapshots ORDER BY snapshot_date DESC LIMIT 1`).Scan(&date)
	if err != nil {
		return ""
	}
	return date
}

// ── Command: !howsthemarket ──────────────────────────────────────────────────

func (p *MarketPlugin) handleSnapshot(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) && !p.checkCooldown(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Market data was just posted. Try again in a few minutes.")
	}

	text := p.formatSnapshot("")
	if text == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No market data available yet. Data pulls at 23:00 UTC.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, text)
}

// formatSnapshot builds the snapshot text. If date is empty, uses the most recent available.
func (p *MarketPlugin) formatSnapshot(date string) string {
	today := time.Now().UTC().Format("2006-01-02")
	showingStale := false

	if date == "" {
		date = today
	}

	snapshots := p.loadSnapshots(date)
	if len(snapshots) == 0 {
		// Try most recent
		date = p.mostRecentSnapshotDate()
		if date == "" {
			return ""
		}
		snapshots = p.loadSnapshots(date)
		if len(snapshots) == 0 {
			return ""
		}
		showingStale = true
	}

	// Check if today's data exists but we were given today
	if date == today && len(snapshots) == 0 {
		date = p.mostRecentSnapshotDate()
		snapshots = p.loadSnapshots(date)
		showingStale = true
	}

	if date != today {
		showingStale = true
	}

	summary := p.loadSummary(date)

	var sb strings.Builder

	if showingStale {
		sb.WriteString(fmt.Sprintf("_Showing end-of-day data from %s. Today's data pulls at 23:00 UTC._\n\n", date))
	}

	if summary != "" {
		sb.WriteString(fmt.Sprintf("**Market Snapshot** — %s (end of day)\n\n", date))
		sb.WriteString(summary)
		sb.WriteString("\n\n")
		// Also show the numbers
		for _, s := range snapshots {
			if s.Price != nil && s.ChangePct != nil {
				emoji := marketChangeEmoji(*s.ChangePct)
				sb.WriteString(fmt.Sprintf("%s %-10s %s  %s\n",
					emoji, s.DisplayName, formatPrice(*s.Price), formatChangePct(*s.ChangePct)))
			} else {
				sb.WriteString(fmt.Sprintf("   %-10s  —  unavailable\n", s.DisplayName))
			}
		}
	} else {
		// Numbers-only fallback
		sb.WriteString(fmt.Sprintf("**Market Snapshot** — %s (end of day)\n\n", date))
		for _, s := range snapshots {
			if s.Price != nil && s.ChangePct != nil {
				emoji := marketChangeEmoji(*s.ChangePct)
				sb.WriteString(fmt.Sprintf("%s %-10s  %s  %s\n",
					emoji, s.DisplayName, formatPrice(*s.Price), formatChangePct(*s.ChangePct)))
			} else {
				sb.WriteString(fmt.Sprintf("   %-10s  —  unavailable\n", s.DisplayName))
			}
		}
	}

	return sb.String()
}

func marketChangeEmoji(pct float64) string {
	if math.Abs(pct) < 0.10 {
		return "➡️"
	}
	if pct > 0 {
		return "📈"
	}
	return "📉"
}

func formatPrice(price float64) string {
	s := fmt.Sprintf("%.2f", price)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	neg := ""
	if strings.HasPrefix(intPart, "-") {
		neg = "-"
		intPart = intPart[1:]
	}
	if len(intPart) > 3 {
		var buf strings.Builder
		for i, c := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				buf.WriteByte(',')
			}
			buf.WriteRune(c)
		}
		intPart = buf.String()
	}
	return neg + intPart + "." + parts[1]
}

func formatChangePct(pct float64) string {
	return fmt.Sprintf("%+.2f%%", pct)
}

// ── Command: !marketstatus ───────────────────────────────────────────────────

type exchangeSchedule struct {
	Name      string
	Covers    string
	Timezone  string // IANA timezone
	OpenHour  int
	OpenMin   int
	CloseHour int
	CloseMin  int
	// Optional lunch break (TSE)
	HasLunch       bool
	LunchStartHour int
	LunchStartMin  int
	LunchEndHour   int
	LunchEndMin    int
}

var exchanges = []exchangeSchedule{
	{
		Name: "NYSE / NASDAQ", Covers: "S&P 500, NASDAQ, DOW",
		Timezone: "America/New_York",
		OpenHour: 9, OpenMin: 30, CloseHour: 16, CloseMin: 0,
	},
	{
		Name: "LSE", Covers: "FTSE 100",
		Timezone: "Europe/London",
		OpenHour: 8, OpenMin: 0, CloseHour: 16, CloseMin: 30,
	},
	{
		Name: "Euronext", Covers: "PSI 20",
		Timezone: "Europe/Lisbon",
		OpenHour: 8, OpenMin: 0, CloseHour: 16, CloseMin: 30,
	},
	{
		Name: "TSE", Covers: "Nikkei 225",
		Timezone: "Asia/Tokyo",
		OpenHour: 9, OpenMin: 0, CloseHour: 15, CloseMin: 0,
		HasLunch: true, LunchStartHour: 11, LunchStartMin: 30, LunchEndHour: 12, LunchEndMin: 30,
	},
}

func (p *MarketPlugin) handleStatus(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) && !p.checkCooldown(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Market status was just posted. Try again in a few minutes.")
	}

	now := time.Now().UTC()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Market Status** — %s UTC\n\n", now.Format("15:04")))

	for _, ex := range exchanges {
		loc, err := time.LoadLocation(ex.Timezone)
		if err != nil {
			sb.WriteString(fmt.Sprintf("%-15s  — timezone error\n", ex.Name))
			continue
		}

		localNow := now.In(loc)
		weekday := localNow.Weekday()

		// Weekend check
		if weekday == time.Saturday || weekday == time.Sunday {
			nextOpen := nextWeekdayOpen(localNow, ex)
			sb.WriteString(fmt.Sprintf("%-15s  🔴 Closed (weekend, opens %s)\n", ex.Name, marketFormatDuration(nextOpen.Sub(now))))
			continue
		}

		openTime := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), ex.OpenHour, ex.OpenMin, 0, 0, loc)
		closeTime := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), ex.CloseHour, ex.CloseMin, 0, 0, loc)

		if localNow.Before(openTime) {
			dur := openTime.Sub(now)
			sb.WriteString(fmt.Sprintf("%-15s  🔴 Closed (opens in %s)\n", ex.Name, marketFormatDuration(dur)))
		} else if localNow.After(closeTime) {
			nextOpen := nextWeekdayOpen(localNow.AddDate(0, 0, 1), ex)
			sb.WriteString(fmt.Sprintf("%-15s  🔴 Closed (opens in %s)\n", ex.Name, marketFormatDuration(nextOpen.Sub(now))))
		} else if ex.HasLunch {
			lunchStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), ex.LunchStartHour, ex.LunchStartMin, 0, 0, loc)
			lunchEnd := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), ex.LunchEndHour, ex.LunchEndMin, 0, 0, loc)
			if localNow.After(lunchStart) && localNow.Before(lunchEnd) {
				sb.WriteString(fmt.Sprintf("%-15s  🟡 Lunch break (resumes in %s)\n", ex.Name, marketFormatDuration(lunchEnd.Sub(now))))
			} else {
				sb.WriteString(fmt.Sprintf("%-15s  🟢 Open\n", ex.Name))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%-15s  🟢 Open\n", ex.Name))
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func nextWeekdayOpen(from time.Time, ex exchangeSchedule) time.Time {
	loc, err := time.LoadLocation(ex.Timezone)
	if err != nil {
		loc = time.UTC
	}
	t := time.Date(from.Year(), from.Month(), from.Day(), ex.OpenHour, ex.OpenMin, 0, 0, loc)
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func marketFormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// ── Command: !marketreport ───────────────────────────────────────────────────

func (p *MarketPlugin) handleReport(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "marketreport"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!marketreport week|month|year|vix|compare <index> <days>`")
	}

	if !p.IsAdmin(ctx.Sender) && !p.checkCooldown(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Market data was just posted. Try again in a few minutes.")
	}

	parts := strings.Fields(args)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "week":
		return p.handleTrendReport(ctx, 7)
	case "month":
		return p.handleTrendReport(ctx, 30)
	case "year":
		return p.handleYearReport(ctx)
	case "vix":
		return p.handleVixReport(ctx)
	case "compare":
		return p.handleCompare(ctx, parts[1:])
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Unknown report type. Try: `week`, `month`, `year`, `vix`, or `compare <index> <days>`.")
	}
}

func (p *MarketPlugin) handleTrendReport(ctx MessageContext, days int) error {
	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -days).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	snapsByDate := p.loadSnapshotRange(startDate, endDate)
	if len(snapsByDate) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No historical data available for that range.")
	}

	dateRange := fmt.Sprintf("%s to %s", startDate, endDate)
	summary := p.generateReportSummary(snapsByDate, dateRange)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Market Report** — Last %d days\n\n", days))

	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}

	// Show first and last data points for each index
	sb.WriteString(p.formatTrendTable(snapsByDate))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MarketPlugin) handleYearReport(ctx MessageContext) error {
	now := time.Now().UTC()
	startDate := now.AddDate(-1, 0, 0).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	snapsByDate := p.loadSnapshotRange(startDate, endDate)
	if len(snapsByDate) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No historical data available.")
	}

	dateRange := fmt.Sprintf("%s to %s", startDate, endDate)
	summary := p.generateReportSummary(snapsByDate, dateRange)

	var sb strings.Builder
	sb.WriteString("**Market Report** — 365 days\n\n")

	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}

	// High, low, current per index
	indexStats := make(map[string]struct{ High, Low, Current float64 })
	for _, snaps := range snapsByDate {
		for _, s := range snaps {
			if s.Price == nil {
				continue
			}
			stats, ok := indexStats[s.DisplayName]
			if !ok {
				stats = struct{ High, Low, Current float64 }{*s.Price, *s.Price, *s.Price}
			}
			if *s.Price > stats.High {
				stats.High = *s.Price
			}
			if *s.Price < stats.Low {
				stats.Low = *s.Price
			}
			stats.Current = *s.Price // last one wins (data is ordered by date)
			indexStats[s.DisplayName] = stats
		}
	}

	for _, idx := range marketIndices {
		if stats, ok := indexStats[idx.DisplayName]; ok {
			sb.WriteString(fmt.Sprintf("%-10s  High: %s  Low: %s  Current: %s\n",
				idx.DisplayName, formatPrice(stats.High), formatPrice(stats.Low), formatPrice(stats.Current)))
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MarketPlugin) handleVixReport(ctx MessageContext) error {
	now := time.Now().UTC()
	startDate := now.AddDate(-1, 0, 0).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	d := db.Get()
	rows, err := d.Query(
		`SELECT snapshot_date, price, change_pct FROM market_snapshots
		 WHERE symbol = '^VIX' AND snapshot_date >= ? AND snapshot_date <= ? AND price IS NOT NULL
		 ORDER BY snapshot_date`, startDate, endDate)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load VIX data.")
	}
	defer rows.Close()

	type vixEntry struct {
		Date      string
		Price     float64
		ChangePct float64
	}
	var entries []vixEntry
	for rows.Next() {
		var e vixEntry
		if err := rows.Scan(&e.Date, &e.Price, &e.ChangePct); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No VIX data available.")
	}

	// Generate VIX-specific summary
	var prices []string
	for _, e := range entries {
		prices = append(prices, fmt.Sprintf("%.2f", e.Price))
	}

	var summary string
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host != "" && model != "" {
		prompt := fmt.Sprintf("VIX (fear index) data over %d days (%s to %s):\n%s\n\nDescribe the fear/greed trajectory in 2-3 sentences. Be sardonic but accurate.",
			len(entries), entries[0].Date, entries[len(entries)-1].Date, strings.Join(prices, ", "))
		summary, _ = p.callOllamaChat(host, model, marketSystemPrompt, prompt)
	}

	var sb strings.Builder
	sb.WriteString("**VIX Report** — Fear & Greed\n\n")

	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}

	current := entries[len(entries)-1]
	high, low := entries[0].Price, entries[0].Price
	for _, e := range entries {
		if e.Price > high {
			high = e.Price
		}
		if e.Price < low {
			low = e.Price
		}
	}

	sb.WriteString(fmt.Sprintf("Current: %.2f  |  52w High: %.2f  |  52w Low: %.2f\n", current.Price, high, low))

	// Sentiment label
	label := "Moderate"
	if current.Price < 15 {
		label = "Complacent"
	} else if current.Price < 20 {
		label = "Calm"
	} else if current.Price < 25 {
		label = "Elevated"
	} else if current.Price < 30 {
		label = "High Fear"
	} else {
		label = "Extreme Fear"
	}
	sb.WriteString(fmt.Sprintf("Sentiment: %s", label))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *MarketPlugin) handleCompare(ctx MessageContext, args []string) error {
	if len(args) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!marketreport compare <index> <days>`\nIndices: SP500, NASDAQ, DOW, FTSE, NIKKEI, PSI20, VIX")
	}

	idx, ok := marketShortNames[strings.ToUpper(args[0])]
	if !ok {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Unknown index '%s'. Valid: SP500, NASDAQ, DOW, FTSE, NIKKEI, PSI20, VIX", args[0]))
	}

	days, err := strconv.Atoi(args[1])
	if err != nil || days < 1 || days > 365 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Days must be between 1 and 365.")
	}

	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -days).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	d := db.Get()
	rows, err := d.Query(
		`SELECT snapshot_date, price, change_pct FROM market_snapshots
		 WHERE symbol = ? AND snapshot_date >= ? AND snapshot_date <= ? AND price IS NOT NULL
		 ORDER BY snapshot_date`, idx.Symbol, startDate, endDate)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load data.")
	}
	defer rows.Close()

	type entry struct {
		Date      string
		Price     float64
		ChangePct float64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Date, &e.Price, &e.ChangePct); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("No data for %s in that range.", idx.DisplayName))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s** — %d day report\n\n", idx.DisplayName, days))

	// Generate LLM summary
	var prices []string
	for _, e := range entries {
		prices = append(prices, fmt.Sprintf("%.2f", e.Price))
	}
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host != "" && model != "" {
		prompt := fmt.Sprintf("%s over %d days (%s to %s):\n%s\n\nDescribe the trend in 2-3 sentences. Be sardonic but accurate.",
			idx.DisplayName, len(entries), entries[0].Date, entries[len(entries)-1].Date, strings.Join(prices, ", "))
		if summary, err := p.callOllamaChat(host, model, marketSystemPrompt, prompt); err == nil && summary != "" {
			sb.WriteString(summary)
			sb.WriteString("\n\n")
		}
	}

	first := entries[0]
	last := entries[len(entries)-1]
	var totalChange float64
	if first.Price != 0 {
		totalChange = ((last.Price - first.Price) / first.Price) * 100
	}

	high, low := first.Price, first.Price
	for _, e := range entries {
		if e.Price > high {
			high = e.Price
		}
		if e.Price < low {
			low = e.Price
		}
	}

	sb.WriteString(fmt.Sprintf("Start: %s (%s)  →  Current: %s (%s)\n",
		formatPrice(first.Price), first.Date, formatPrice(last.Price), last.Date))
	sb.WriteString(fmt.Sprintf("Change: %+.2f%%  |  High: %s  |  Low: %s\n",
		totalChange, formatPrice(high), formatPrice(low)))
	sb.WriteString(fmt.Sprintf("Data points: %d", len(entries)))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// ── Trend Table Helper ───────────────────────────────────────────────────────

func (p *MarketPlugin) formatTrendTable(snapsByDate map[string][]marketSnapshot) string {
	// Get first and last dates
	var dates []string
	for d := range snapsByDate {
		dates = append(dates, d)
	}
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[j] < dates[i] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}
	if len(dates) < 2 {
		return ""
	}

	firstDate := dates[0]
	lastDate := dates[len(dates)-1]
	first := snapsByDate[firstDate]
	last := snapsByDate[lastDate]

	// Map by symbol
	firstMap := make(map[string]*marketSnapshot)
	lastMap := make(map[string]*marketSnapshot)
	for i := range first {
		firstMap[first[i].Symbol] = &first[i]
	}
	for i := range last {
		lastMap[last[i].Symbol] = &last[i]
	}

	var sb strings.Builder
	for _, idx := range marketIndices {
		f, okF := firstMap[idx.Symbol]
		l, okL := lastMap[idx.Symbol]
		if !okF || !okL || f.Price == nil || l.Price == nil || *f.Price == 0 {
			continue
		}
		change := ((*l.Price - *f.Price) / *f.Price) * 100
		emoji := marketChangeEmoji(change)
		sb.WriteString(fmt.Sprintf("%s %-10s  %s → %s  (%+.2f%%)\n",
			emoji, idx.DisplayName, formatPrice(*f.Price), formatPrice(*l.Price), change))
	}

	return sb.String()
}
