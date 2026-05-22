package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── Crypto support (CoinGecko, keyless) ──────────────────────────────────────
//
// CoinGecko's public simple/price endpoint needs no API key and no usage tier,
// matching Frankfurter's keyless model. Crypto is wired into the *conversion*
// path only (`!fx 100 USD to BTC`). The rate/report/alert analysis stays
// fiat-only on purpose: a daily-snapshot "buy signal" or 52-week range is
// meaningless for 24/7 crypto markets.

type fxCryptoInfo struct {
	CoinGeckoID string
	Emoji       string
	Label       string
}

// fxCryptoMeta maps a ticker to its CoinGecko coin ID and display metadata.
// Add a row here to support another asset.
var fxCryptoMeta = map[string]fxCryptoInfo{
	"BTC":  {"bitcoin", "₿", "Bitcoin"},
	"ETH":  {"ethereum", "Ξ", "Ethereum"},
	"SOL":  {"solana", "◎", "Solana"},
	"XRP":  {"ripple", "✕", "XRP"},
	"DOGE": {"dogecoin", "🐕", "Dogecoin"},
	"ADA":  {"cardano", "₳", "Cardano"},
	"LTC":  {"litecoin", "Ł", "Litecoin"},
}

// fxIsCrypto reports whether sym is a supported crypto asset.
func fxIsCrypto(sym string) bool {
	_, ok := fxCryptoMeta[strings.ToUpper(sym)]
	return ok
}

// fxIsConvertible reports whether a symbol can appear in a conversion —
// USD, a tracked fiat currency, or a supported crypto asset.
func fxIsConvertible(cur string) bool {
	return fxIsSupported(cur) || fxIsCrypto(cur)
}

const coinGeckoBaseURL = "https://api.coingecko.com/api/v3"

// Short-TTL price cache to stay well under CoinGecko's public rate limit and
// keep repeated conversions snappy.
const cgCacheTTL = 60 * time.Second

var (
	cgMu    sync.Mutex
	cgCache = map[string]cgCacheEntry{}
)

type cgCacheEntry struct {
	priceUSD float64
	at       time.Time
}

// cgSpotUSD returns the USD spot price for one unit of the given crypto symbol.
func (p *ForexPlugin) cgSpotUSD(sym string) (float64, error) {
	sym = strings.ToUpper(sym)
	info, ok := fxCryptoMeta[sym]
	if !ok {
		return 0, fmt.Errorf("unsupported crypto %s", sym)
	}

	cgMu.Lock()
	if e, ok := cgCache[sym]; ok && time.Since(e.at) < cgCacheTTL {
		cgMu.Unlock()
		return e.priceUSD, nil
	}
	cgMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=usd", coinGeckoBaseURL, info.CoinGeckoID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("coingecko API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("coingecko API returned %d", resp.StatusCode)
	}

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("coingecko decode error: %w", err)
	}
	entry, ok := data[info.CoinGeckoID]
	if !ok {
		return 0, fmt.Errorf("no price for %s", sym)
	}
	price, ok := entry["usd"]
	if !ok || price <= 0 {
		return 0, fmt.Errorf("invalid price for %s", sym)
	}

	cgMu.Lock()
	cgCache[sym] = cgCacheEntry{priceUSD: price, at: time.Now()}
	cgMu.Unlock()

	return price, nil
}
