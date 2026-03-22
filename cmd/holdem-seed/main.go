//go:build training

package main

import (
	"fmt"
	"log/slog"
	"os"

	"gogobee/internal/plugin"
)

// Creates a minimal seed policy so the bot can start without a trained policy.
// Run: go run -tags training ./cmd/holdem-seed/
func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	data := &plugin.CFRData{
		Regrets:  make(plugin.RegretTable),
		Strategy: make(plugin.RegretTable),
		Meta: plugin.CFRTrainingMeta{
			Iterations: 0,
			Seed:       42,
			Date:       "seed-policy",
		},
	}

	os.MkdirAll("data", 0o755)
	if err := plugin.SaveCFRData("data/policy.gob", data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("Seed policy created", "path", "data/policy.gob")
	slog.Info("Train a real policy with: go run -tags training ./cmd/holdem-train/ --iterations 1000000 --workers 8")
}
