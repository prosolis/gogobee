//go:build training

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"gogobee/internal/plugin"
)

func main() {
	iterations := flag.Int("iterations", 5_000_000, "Number of training iterations")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of parallel workers")
	output := flag.String("output", "data/policy.gob", "Output policy file path")
	resume := flag.String("resume", "", "Resume from checkpoint file")
	validate := flag.Bool("validate", false, "Run validation instead of training")
	checkpointEvery := flag.Int("checkpoint-every", 500_000, "Save checkpoint every N iterations")
	seed := flag.Int64("seed", 42, "Random seed for reproducibility")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if *validate {
		runValidation(*output)
		return
	}

	runTraining(*iterations, *workers, *output, *resume, *checkpointEvery, *seed)
}

func runTraining(iterations, workers int, output, resumePath string, checkpointEvery int, seed int64) {
	var data *plugin.CFRData

	if resumePath != "" {
		slog.Info("Resuming from checkpoint", "path", resumePath)
		var err error
		data, err = plugin.LoadCFRData(resumePath)
		if err != nil {
			slog.Error("Failed to load checkpoint", "err", err)
			os.Exit(1)
		}
		slog.Info("Checkpoint loaded",
			"existing_iterations", data.Meta.Iterations,
			"nodes", len(data.Regrets))
	} else {
		data = &plugin.CFRData{
			Regrets:  make(plugin.RegretTable),
			Strategy: make(plugin.RegretTable),
			Meta: plugin.CFRTrainingMeta{
				Seed: seed,
			},
		}
	}

	slog.Info("Starting CFR training",
		"iterations", iterations,
		"workers", workers,
		"output", output)

	start := time.Now()

	progress := &plugin.TrainProgress{
		Total:     iterations,
		StartTime: time.Now(),
	}

	if workers <= 1 {
		// Single-threaded training.
		plugin.TrainCFR(data, iterations, 100_000, "", progress)
		data.Meta.Iterations += iterations
	} else {
		// Parallel training with shared regret table.
		var mu sync.Mutex
		iterPerWorker := iterations / workers
		remainder := iterations % workers

		var wg sync.WaitGroup
		totalCompleted := 0

		for w := 0; w < workers; w++ {
			iters := iterPerWorker
			if w < remainder {
				iters++
			}

			wg.Add(1)
			go func(workerID, workerIters int) {
				defer wg.Done()

				// Each worker trains on its own local data, then merges.
				localData := &plugin.CFRData{
					Regrets:  make(plugin.RegretTable),
					Strategy: make(plugin.RegretTable),
				}

				progressEvery := 100_000 / workers
				if progressEvery < 10_000 {
					progressEvery = 10_000
				}

				label := fmt.Sprintf("worker-%d", workerID)
				plugin.TrainCFR(localData, workerIters, progressEvery, label, progress)

				// Merge local data into shared data.
				mu.Lock()
				for key, regrets := range localData.Regrets {
					existing := data.Regrets[key]
					for i := range regrets {
						existing[i] += regrets[i]
					}
					data.Regrets[key] = existing
				}
				for key, strat := range localData.Strategy {
					existing := data.Strategy[key]
					for i := range strat {
						existing[i] += strat[i]
					}
					data.Strategy[key] = existing
				}
				totalCompleted += workerIters
				mu.Unlock()

				slog.Info("Worker completed", "worker", workerID, "iterations", workerIters)
			}(w, iters)
		}

		// Checkpoint saver (runs in background).
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(30 * time.Second) // checkpoint every 30s
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					mu.Lock()
					nodeCount := len(data.Regrets)
					if nodeCount == 0 {
						mu.Unlock()
						continue
					}
					// Deep copy maps to avoid concurrent read during gob encoding.
					checkpoint := &plugin.CFRData{
						Regrets:  make(plugin.RegretTable, nodeCount),
						Strategy: make(plugin.RegretTable, len(data.Strategy)),
						Meta:     data.Meta,
					}
					for k, v := range data.Regrets {
						checkpoint.Regrets[k] = v
					}
					for k, v := range data.Strategy {
						checkpoint.Strategy[k] = v
					}
					checkpoint.Meta.Iterations += totalCompleted
					mu.Unlock()

					checkpointPath := output + ".checkpoint"
					if err := plugin.SaveCFRData(checkpointPath, checkpoint); err != nil {
						slog.Warn("Checkpoint save failed", "err", err)
					} else {
						slog.Info("Checkpoint saved", "path", checkpointPath, "nodes", len(checkpoint.Regrets))
					}
				case <-done:
					return
				}
			}
		}()

		wg.Wait()
		close(done)
		data.Meta.Iterations += iterations
	}

	elapsed := time.Since(start)
	data.Meta.Date = time.Now().Format("2006-01-02 15:04:05")

	slog.Info("Training complete",
		"iterations", data.Meta.Iterations,
		"nodes", len(data.Regrets),
		"elapsed", elapsed.Round(time.Second))

	// Ensure output directory exists.
	if dir := outputDir(output); dir != "" {
		os.MkdirAll(dir, 0o755)
	}

	if err := plugin.SaveCFRData(output, data); err != nil {
		slog.Error("Failed to save policy", "err", err)
		os.Exit(1)
	}

	slog.Info("Policy saved", "path", output, "nodes", len(data.Strategy))
}

func runValidation(policyPath string) {
	slog.Info("Loading policy for validation", "path", policyPath)

	policy, err := plugin.LoadPolicy(policyPath)
	if err != nil {
		slog.Error("Failed to load policy", "err", err)
		os.Exit(1)
	}

	slog.Info("Running validation (50,000 hands)...")
	winRate, vpip, aggFactor := plugin.ValidatePolicy(policy, 50_000)

	fmt.Println()
	fmt.Println("=== Validation Results ===")
	fmt.Printf("Win Rate:    %.1f%%\n", winRate*100)
	fmt.Printf("VPIP:        %.1f%%\n", vpip*100)
	fmt.Printf("Agg Factor:  %.2f\n", aggFactor)
	fmt.Println()

	if winRate > 0.55 {
		fmt.Println("✅ Policy exceeds 55% win rate threshold.")
		os.Exit(0)
	} else if winRate > 0.50 {
		fmt.Println("⚠️ Policy above 50% but below 55% threshold. Consider more training iterations.")
		os.Exit(0)
	} else {
		fmt.Println("❌ Policy below 50% win rate. More training needed.")
		os.Exit(1)
	}
}

func outputDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}
