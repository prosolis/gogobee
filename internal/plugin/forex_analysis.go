package plugin

import (
	"fmt"
	"math"
	"strings"
)

// ForexSignal holds the computed analysis for a currency pair.
type ForexSignal struct {
	Currency   string
	Rate       float64
	Avg30      float64
	Avg90      float64
	High52w    float64
	Low52w     float64
	Percentile float64 // 0-100: where current rate sits in 52w range
	Score      int     // 1-10
	Label      string  // "Excellent", "Good", "Fair", "Poor"
	Emoji      string  // color indicator
}

// fxComputeSignal computes the full signal for a currency at the given rate.
func (p *ForexPlugin) fxComputeSignal(currency string, currentRate float64) (*ForexSignal, error) {
	// Get last 260 trading days (~52 weeks) of data
	records, err := fxGetRatesByLimit(currency, 260)
	if err != nil || len(records) < 10 {
		return nil, fmt.Errorf("insufficient data (%d records)", len(records))
	}

	// Append today's live rate for analysis
	rates := make([]float64, len(records)+1)
	for i, r := range records {
		rates[i] = r.Rate
	}
	rates[len(records)] = currentRate

	n := len(rates)

	// 30-day and 90-day moving averages (trading days, not calendar)
	avg30 := fxAvg(rates, 30)
	avg90 := fxAvg(rates, 90)

	// 52-week high/low
	high52w, low52w := rates[0], rates[0]
	for _, r := range rates {
		if r > high52w {
			high52w = r
		}
		if r < low52w {
			low52w = r
		}
	}

	// Percentile in 52w range
	var percentile float64
	if high52w != low52w {
		percentile = (currentRate - low52w) / (high52w - low52w) * 100
	} else {
		percentile = 50
	}

	// Score: weighted combination of percentile position and deviation from averages
	// Higher = USD is stronger = better time to convert
	devScore := fxDeviationScore(currentRate, avg30, avg90)
	rawScore := percentile/100*10*0.6 + devScore*0.4
	score := int(math.Round(rawScore))
	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}

	label, emoji := fxScoreLabel(score)

	_ = n // rates slice used above

	return &ForexSignal{
		Currency:   currency,
		Rate:       currentRate,
		Avg30:      avg30,
		Avg90:      avg90,
		High52w:    high52w,
		Low52w:     low52w,
		Percentile: percentile,
		Score:      score,
		Label:      label,
		Emoji:      emoji,
	}, nil
}

// fxAvg computes the average of the last n values in a slice.
func fxAvg(rates []float64, n int) float64 {
	if len(rates) < n {
		n = len(rates)
	}
	if n == 0 {
		return 0
	}
	sum := 0.0
	start := len(rates) - n
	for i := start; i < len(rates); i++ {
		sum += rates[i]
	}
	return sum / float64(n)
}

// fxDeviationScore computes how far above/below the moving averages the current rate is.
// +5% above average = 10, -5% below = 0, linear interpolation between.
func fxDeviationScore(current, avg30, avg90 float64) float64 {
	avgAvg := (avg30 + avg90) / 2
	if avgAvg == 0 {
		return 5
	}
	dev := (current - avgAvg) / avgAvg * 100 // percent deviation
	// Map [-5%, +5%] to [0, 10]
	score := (dev + 5) / 10 * 10
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}
	return score
}

func fxScoreLabel(score int) (string, string) {
	switch {
	case score >= 8:
		return "Excellent", "🟢"
	case score >= 6:
		return "Good", "🟡"
	case score >= 4:
		return "Fair", "🟠"
	default:
		return "Poor", "🔴"
	}
}

// ── Formatting ──────────────────────────────────────────────────────────────

// FormatQuick returns a one-line summary.
func (s *ForexSignal) FormatQuick() string {
	meta := fxMeta[s.Currency]
	return fmt.Sprintf("%s **%s** 1 USD = **%s** %s · 30d avg: %s · USD strength: %d/10 %s %s",
		meta.Emoji, s.Currency, fxFormatRate(s.Currency, s.Rate), s.Currency,
		fxFormatRate(s.Currency, s.Avg30),
		s.Score, s.Emoji, s.Label)
}

// FormatReport returns a detailed analysis block.
func (s *ForexSignal) FormatReport() string {
	meta := fxMeta[s.Currency]
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s **%s — %s**\n", meta.Emoji, s.Currency, meta.Label))
	sb.WriteString(fmt.Sprintf("**Current:** 1 USD = **%s %s**\n\n", fxFormatRate(s.Currency, s.Rate), s.Currency))

	// Moving averages
	sb.WriteString(fmt.Sprintf("  30-day avg: %s", fxFormatRate(s.Currency, s.Avg30)))
	if s.Rate > s.Avg30 {
		sb.WriteString(fmt.Sprintf(" _(+%.1f%% above)_", (s.Rate-s.Avg30)/s.Avg30*100))
	} else if s.Rate < s.Avg30 {
		sb.WriteString(fmt.Sprintf(" _(%.1f%% below)_", (s.Rate-s.Avg30)/s.Avg30*100))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  90-day avg: %s", fxFormatRate(s.Currency, s.Avg90)))
	if s.Rate > s.Avg90 {
		sb.WriteString(fmt.Sprintf(" _(+%.1f%% above)_", (s.Rate-s.Avg90)/s.Avg90*100))
	} else if s.Rate < s.Avg90 {
		sb.WriteString(fmt.Sprintf(" _(%.1f%% below)_", (s.Rate-s.Avg90)/s.Avg90*100))
	}
	sb.WriteString("\n\n")

	// 52-week range bar
	sb.WriteString(fmt.Sprintf("  52-week range: %s — %s\n",
		fxFormatRate(s.Currency, s.Low52w), fxFormatRate(s.Currency, s.High52w)))
	sb.WriteString(fmt.Sprintf("  %s\n", fxRangeBar(s.Percentile)))
	sb.WriteString(fmt.Sprintf("  Position: %.0f%%\n\n", s.Percentile))

	// Score
	sb.WriteString(fmt.Sprintf("  **USD Strength: %d/10** %s %s\n", s.Score, s.Emoji, s.Label))
	sb.WriteString("  _Higher = USD buys more — good time to convert USD._")

	return sb.String()
}

// fxRangeBar renders a text-based position indicator.
func fxRangeBar(percentile float64) string {
	width := 20
	pos := int(percentile / 100 * float64(width))
	if pos < 0 {
		pos = 0
	}
	if pos >= width {
		pos = width - 1
	}

	bar := make([]byte, width)
	for i := range bar {
		bar[i] = '-'
	}
	bar[pos] = '|'
	return "[" + string(bar) + "]"
}
