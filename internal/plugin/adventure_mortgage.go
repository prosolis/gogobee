package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"gogobee/internal/db"
)

// ── FRED API Integration ───────────────────────────────────────────────────
//
// Pulls the 5/1 ARM rate weekly from FRED (series: MORTGAGE5US).
// Requires FRED_API_KEY environment variable.
// Falls back to a hardcoded default if the API is unavailable.

const fredSeries = "MORTGAGE5US"
const defaultMortgageRate = 6.5
const currentRateCacheKey = "mortgage_current_rate"

type fredResponse struct {
	Observations []struct {
		Date  string `json:"date"`
		Value string `json:"value"`
	} `json:"observations"`
}

// fetchFREDRate pulls the latest 5/1 ARM rate from the FRED API.
func fetchFREDRate() (float64, error) {
	apiKey := os.Getenv("FRED_API_KEY")
	if apiKey == "" {
		return 0, fmt.Errorf("FRED_API_KEY not set")
	}

	url := fmt.Sprintf(
		"https://api.stlouisfed.org/fred/series/observations?series_id=%s&api_key=%s&file_type=json&sort_order=desc&limit=1",
		fredSeries, apiKey,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("FRED API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("FRED API status %d", resp.StatusCode)
	}

	var data fredResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("FRED JSON decode: %w", err)
	}

	if len(data.Observations) == 0 {
		return 0, fmt.Errorf("FRED: no observations returned")
	}

	val := data.Observations[0].Value
	if val == "." {
		return 0, fmt.Errorf("FRED: value is placeholder")
	}

	rate, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("FRED: parse rate %q: %w", val, err)
	}

	// Clamp to sane range
	if rate < 1.0 {
		rate = 1.0
	}
	if rate > 15.0 {
		rate = 15.0
	}

	return rate, nil
}

// getCurrentMortgageRate returns the cached rate, or fetches fresh if stale (daily).
func getCurrentMortgageRate() float64 {
	// Check DB cache (24h TTL)
	if val := db.CacheGet(currentRateCacheKey, 24*3600); val != "" {
		if rate, err := strconv.ParseFloat(val, 64); err == nil {
			return rate
		}
	}

	rate, err := fetchFREDRate()
	if err != nil {
		slog.Warn("mortgage: FRED fetch failed, using default", "err", err)
		return defaultMortgageRate
	}

	db.CacheSet(currentRateCacheKey, strconv.FormatFloat(rate, 'f', 2, 64))
	return rate
}

// ── Weekly Mortgage Ticker ─────────────────────────────────────────────────

func (p *AdventurePlugin) mortgageTicker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		// Run on Mondays only (FRED updates Thursdays, we process Mondays)
		if now.Weekday() != time.Monday {
			continue
		}

		_, week := now.ISOWeek()
		dateKey := fmt.Sprintf("%d-W%02d", now.Year(), week)
		jobName := "mortgage_weekly"
		if db.JobCompleted(jobName, dateKey) {
			continue
		}

		slog.Info("mortgage: running weekly payment processing")

		// Fetch fresh rate and compare with previous
		newRate := getCurrentMortgageRate()
		oldRate := p.getLastKnownRate()

		if oldRate > 0 && oldRate != newRate {
			p.sendMortgageRateChangeDMs(oldRate, newRate)
		}
		p.setLastKnownRate(newRate)

		// Process payments
		p.processMortgagePayments()

		db.MarkJobCompleted(jobName, dateKey)
	}
}

const lastKnownRateCacheKey = "mortgage_last_known_rate"

func (p *AdventurePlugin) getLastKnownRate() float64 {
	val := db.CacheGet(lastKnownRateCacheKey, 365*24*3600) // 1 year TTL
	if val == "" {
		return 0
	}
	rate, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return rate
}

func (p *AdventurePlugin) setLastKnownRate(rate float64) {
	db.CacheSet(lastKnownRateCacheKey, strconv.FormatFloat(rate, 'f', 2, 64))
}
