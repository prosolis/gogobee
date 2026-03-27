package plugin

import (
	"math/rand/v2"
	"testing"
)

func BenchmarkComputeSignalFromRates(b *testing.B) {
	// Simulate 260 trading days of rates around 150 (JPY-like)
	rates := make([]float64, 261)
	for i := range rates {
		rates[i] = 145 + rand.Float64()*10
	}
	currentRate := rates[len(rates)-1]

	b.ResetTimer()
	for b.Loop() {
		fxComputeSignalFromRates(rates, currentRate)
	}
}

func BenchmarkComputeSignalFromRates_CrossPairSize(b *testing.B) {
	// Simulate cross-pair: 260 days of EUR/JPY-like rates
	baseRates := make([]float64, 260)
	quoteRates := make([]float64, 260)
	for i := range baseRates {
		baseRates[i] = 0.90 + rand.Float64()*0.10  // EUR/USD ~0.90-1.00
		quoteRates[i] = 145 + rand.Float64()*10     // JPY/USD ~145-155
	}

	// Compute cross-rates (simulating the join)
	crossRates := make([]float64, len(baseRates)+1)
	for i := range baseRates {
		crossRates[i] = quoteRates[i] / baseRates[i]
	}
	crossRates[len(baseRates)] = quoteRates[len(quoteRates)-1] / baseRates[len(baseRates)-1]
	currentRate := crossRates[len(crossRates)-1]

	b.ResetTimer()
	for b.Loop() {
		fxComputeSignalFromRates(crossRates, currentRate)
	}
}
