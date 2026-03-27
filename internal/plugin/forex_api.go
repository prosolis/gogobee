package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ── Currency Configuration ──────────────────────────────────────────────────

var fxTrackedCurrencies = []string{"EUR", "JPY", "CAD"}

type fxCurrencyMeta struct {
	Emoji string
	Label string
}

var fxMeta = map[string]fxCurrencyMeta{
	"EUR": {Emoji: "🇪🇺", Label: "Euro"},
	"JPY": {Emoji: "🇯🇵", Label: "Japanese Yen"},
	"CAD": {Emoji: "🇨🇦", Label: "Canadian Dollar"},
}

func fxIsTracked(cur string) bool {
	for _, c := range fxTrackedCurrencies {
		if c == cur {
			return true
		}
	}
	return false
}

// fxFormatRate formats a rate: JPY uses 2 decimal places, others use 4.
func fxFormatRate(currency string, rate float64) string {
	if currency == "JPY" {
		return fmt.Sprintf("%.2f", rate)
	}
	return fmt.Sprintf("%.4f", rate)
}

// ── Frankfurter v2 API ──────────────────────────────────────────────────────
//
// Frankfurter v2 (api.frankfurter.dev) tracks rates from 20 central banks,
// covering 150 currencies. No API key required, no usage limits.

const frankfurterBaseURL = "https://api.frankfurter.dev/v2"

// frankfurterV2Rate is one entry in the v2 response array.
type frankfurterV2Rate struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

// fxFetchCurrent fetches live rates from Frankfurter v2 with a context timeout
// so user commands don't block the dispatch pipeline if the API is slow.
func (p *ForexPlugin) fxFetchCurrent(currencies []string) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/rates?base=USD&quotes=%s", frankfurterBaseURL, strings.Join(currencies, ","))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frankfurter API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frankfurter API returned %d", resp.StatusCode)
	}

	var data []frankfurterV2Rate
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("frankfurter decode error: %w", err)
	}

	rates := make(map[string]float64, len(data))
	for _, r := range data {
		rates[r.Quote] = r.Rate
	}
	return rates, nil
}

// fxFetchRange fetches historical rates for a date range from Frankfurter v2.
func (p *ForexPlugin) fxFetchRange(from, to time.Time, currencies []string) ([]frankfurterV2Rate, error) {
	url := fmt.Sprintf("%s/rates?base=USD&quotes=%s&from=%s&to=%s",
		frankfurterBaseURL, strings.Join(currencies, ","),
		from.Format("2006-01-02"), to.Format("2006-01-02"))

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frankfurter API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frankfurter API returned %d", resp.StatusCode)
	}

	var data []frankfurterV2Rate
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("frankfurter decode error: %w", err)
	}

	return data, nil
}

// fxLiveRates fetches current rates, falling back to the most recent stored rate.
func (p *ForexPlugin) fxLiveRates(currencies []string) (map[string]float64, error) {
	rates, err := p.fxFetchCurrent(currencies)
	if err != nil {
		slog.Warn("forex: live fetch failed, using stored fallback", "err", err)
		rates = make(map[string]float64)
		for _, cur := range currencies {
			if r, ok := fxLatestRate(cur); ok {
				rates[cur] = r
			}
		}
		if len(rates) == 0 {
			return nil, fmt.Errorf("could not fetch rates and no stored data available")
		}
	}
	return rates, nil
}

// backfill fetches the trailing year of rates and saves them.
// Safe to call repeatedly — uses INSERT OR IGNORE.
func (p *ForexPlugin) backfill() {
	end := time.Now().UTC()
	start := end.AddDate(-1, 0, 0)

	slog.Info("forex: starting backfill", "from", start.Format("2006-01-02"), "to", end.Format("2006-01-02"))

	data, err := p.fxFetchRange(start, end, fxTrackedCurrencies)
	if err != nil {
		slog.Error("forex: backfill failed", "err", err)
		return
	}

	for _, r := range data {
		fxSaveRate(r.Quote, r.Date, r.Rate)
	}

	slog.Info("forex: backfill complete", "records", len(data))
}
