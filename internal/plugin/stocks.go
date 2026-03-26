package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
)

// finnhubQuote is the response from the Finnhub quote endpoint.
type finnhubQuote struct {
	Current    float64 `json:"c"`  // Current price
	Change     float64 `json:"d"`  // Change
	ChangePct  float64 `json:"dp"` // Percent change
	High       float64 `json:"h"`  // High price of the day
	Low        float64 `json:"l"`  // Low price of the day
	Open       float64 `json:"o"`  // Open price of the day
	PrevClose  float64 `json:"pc"` // Previous close price
	Timestamp  int64   `json:"t"`  // Timestamp
}

// finnhubProfile is the response from the Finnhub company profile endpoint.
type finnhubProfile struct {
	Name          string  `json:"name"`
	Ticker        string  `json:"ticker"`
	Exchange      string  `json:"exchange"`
	Industry      string  `json:"finnhubIndustry"`
	MarketCap     float64 `json:"marketCapitalization"` // in millions
	Currency      string  `json:"currency"`
}

// stockCacheEntry holds cached stock data.
type stockCacheEntry struct {
	Quote   finnhubQuote   `json:"quote"`
	Profile finnhubProfile `json:"profile"`
}

// StocksPlugin provides stock market quotes via Finnhub.
type StocksPlugin struct {
	Base
	apiKey     string
	httpClient *http.Client
}

// NewStocksPlugin creates a new StocksPlugin.
func NewStocksPlugin(client *mautrix.Client) *StocksPlugin {
	return &StocksPlugin{
		Base:   NewBase(client),
		apiKey: os.Getenv("FINNHUB_API_KEY"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *StocksPlugin) Name() string { return "stocks" }

func (p *StocksPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "stock", Description: "Get stock quote(s)", Usage: "!stock <ticker> [ticker2...]", Category: "Entertainment"},
		{Name: "stockwatch", Description: "Manage your stock watchlist", Usage: "!stockwatch add|list|remove <ticker>", Category: "Entertainment"},
	}
}

func (p *StocksPlugin) Init() error { return nil }

func (p *StocksPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *StocksPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "stock") {
		go func() {
			if err := p.handleStock(ctx); err != nil {
				slog.Error("stocks: handler error", "err", err)
			}
		}()
		return nil
	}
	if p.IsCommand(ctx.Body, "stockwatch") {
		return p.handleStockwatch(ctx)
	}
	return nil
}

func (p *StocksPlugin) handleStock(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "stock")
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !stock <ticker> [ticker2...]")
	}

	if p.apiKey == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Stock lookups are not configured (missing API key).")
	}

	tickers := strings.Fields(strings.ToUpper(args))
	if len(tickers) > 5 {
		tickers = tickers[:5]
	}

	var sb strings.Builder
	for i, ticker := range tickers {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		entry, err := p.fetchStock(ticker)
		if err != nil {
			slog.Error("stocks: fetch failed", "ticker", ticker, "err", err)
			sb.WriteString(fmt.Sprintf("%s: Failed to fetch data", ticker))
			continue
		}
		sb.WriteString(p.formatStock(ticker, entry))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *StocksPlugin) fetchStock(ticker string) (*stockCacheEntry, error) {
	d := db.Get()

	// Check cache (60s TTL)
	var cached string
	var cachedAt int64
	err := d.QueryRow(
		`SELECT data, cached_at FROM stocks_cache WHERE ticker = ?`, ticker,
	).Scan(&cached, &cachedAt)
	if err == nil && time.Now().Unix()-cachedAt < 60 {
		var entry stockCacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return &entry, nil
		}
	}

	// Fetch quote
	quote, err := p.fetchFinnhubQuote(ticker)
	if err != nil {
		return nil, fmt.Errorf("fetch quote: %w", err)
	}

	// Fetch profile
	profile, err := p.fetchFinnhubProfile(ticker)
	if err != nil {
		slog.Warn("stocks: profile fetch failed, continuing without it", "ticker", ticker, "err", err)
		profile = &finnhubProfile{}
	}

	entry := &stockCacheEntry{
		Quote:   *quote,
		Profile: *profile,
	}

	// Update cache
	data, _ := json.Marshal(entry)
	db.Exec("stocks: cache write",
		`INSERT INTO stocks_cache (ticker, data, cached_at) VALUES (?, ?, ?)
		 ON CONFLICT(ticker) DO UPDATE SET data = ?, cached_at = ?`,
		ticker, string(data), time.Now().Unix(), string(data), time.Now().Unix(),
	)

	return entry, nil
}

func (p *StocksPlugin) fetchFinnhubQuote(ticker string) (*finnhubQuote, error) {
	url := fmt.Sprintf("https://finnhub.io/api/v1/quote?symbol=%s&token=%s", ticker, p.apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub quote returned status %d", resp.StatusCode)
	}

	var quote finnhubQuote
	if err := json.NewDecoder(resp.Body).Decode(&quote); err != nil {
		return nil, err
	}
	return &quote, nil
}

func (p *StocksPlugin) fetchFinnhubProfile(ticker string) (*finnhubProfile, error) {
	url := fmt.Sprintf("https://finnhub.io/api/v1/stock/profile2?symbol=%s&token=%s", ticker, p.apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub profile returned status %d", resp.StatusCode)
	}

	var profile finnhubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (p *StocksPlugin) formatStock(ticker string, entry *stockCacheEntry) string {
	q := entry.Quote
	pr := entry.Profile

	changeSign := ""
	if q.Change > 0 {
		changeSign = "+"
	}

	var sb strings.Builder
	if pr.Name != "" {
		sb.WriteString(fmt.Sprintf("%s (%s)", pr.Name, ticker))
	} else {
		sb.WriteString(ticker)
	}

	sb.WriteString(fmt.Sprintf("\nPrice: $%.2f", q.Current))
	sb.WriteString(fmt.Sprintf("\nChange: %s%.2f (%s%.2f%%)", changeSign, q.Change, changeSign, q.ChangePct))
	sb.WriteString(fmt.Sprintf("\nDay Range: $%.2f - $%.2f", q.Low, q.High))

	if pr.MarketCap > 0 {
		sb.WriteString(fmt.Sprintf("\nMarket Cap: %s", formatMarketCap(pr.MarketCap)))
	}
	if pr.Exchange != "" {
		sb.WriteString(fmt.Sprintf("\nExchange: %s", pr.Exchange))
	}

	return sb.String()
}

func formatMarketCap(capMillions float64) string {
	switch {
	case capMillions >= 1_000_000:
		return fmt.Sprintf("$%.2fT", capMillions/1_000_000)
	case capMillions >= 1_000:
		return fmt.Sprintf("$%.2fB", capMillions/1_000)
	default:
		return fmt.Sprintf("$%.2fM", capMillions)
	}
}

func (p *StocksPlugin) handleStockwatch(ctx MessageContext) error {
	args := p.GetArgs(ctx.Body, "stockwatch")
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !stockwatch add|list|remove <ticker>")
	}

	sub := strings.ToLower(parts[0])
	switch sub {
	case "add":
		if len(parts) < 2 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !stockwatch add <ticker>")
		}
		return p.watchlistAdd(ctx, strings.ToUpper(parts[1]))
	case "remove":
		if len(parts) < 2 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !stockwatch remove <ticker>")
		}
		return p.watchlistRemove(ctx, strings.ToUpper(parts[1]))
	case "list":
		return p.watchlistList(ctx)
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: !stockwatch add|list|remove <ticker>")
	}
}

func (p *StocksPlugin) watchlistAdd(ctx MessageContext, ticker string) error {
	d := db.Get()
	_, err := d.Exec(
		`INSERT INTO stock_watchlist (user_id, ticker, room_id) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, ticker) DO NOTHING`,
		string(ctx.Sender), ticker, string(ctx.RoomID),
	)
	if err != nil {
		slog.Error("stocks: watchlist add", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to add to watchlist.")
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Added %s to your watchlist.", ticker))
}

func (p *StocksPlugin) watchlistRemove(ctx MessageContext, ticker string) error {
	d := db.Get()
	res, err := d.Exec(
		`DELETE FROM stock_watchlist WHERE user_id = ? AND ticker = ?`,
		string(ctx.Sender), ticker,
	)
	if err != nil {
		slog.Error("stocks: watchlist remove", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to remove from watchlist.")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s is not in your watchlist.", ticker))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Removed %s from your watchlist.", ticker))
}

func (p *StocksPlugin) watchlistList(ctx MessageContext) error {
	d := db.Get()
	rows, err := d.Query(
		`SELECT ticker FROM stock_watchlist WHERE user_id = ? ORDER BY ticker`,
		string(ctx.Sender),
	)
	if err != nil {
		slog.Error("stocks: watchlist list", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load watchlist.")
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			continue
		}
		tickers = append(tickers, t)
	}

	if len(tickers) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Your stock watchlist is empty. Use !stockwatch add <ticker> to add stocks.")
	}

	var sb strings.Builder
	sb.WriteString("Your Stock Watchlist:\n")
	for _, t := range tickers {
		sb.WriteString(fmt.Sprintf("  - %s\n", t))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}
