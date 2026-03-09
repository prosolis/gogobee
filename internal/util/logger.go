package util

import (
	"log/slog"
	"os"
)

// InitLogger sets up a structured slog logger writing to stderr.
func InitLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
