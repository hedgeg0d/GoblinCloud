// Package logging turns the logging config into the process-wide slog logger.
package logging

import (
	"io"
	"log/slog"
	"os"

	"goblin_cloud/internal/config"
)

// Setup builds a logger from cfg and installs it as the slog default. Every
// package that calls slog.Default() (HTTP, FTP, CLI) picks it up from here.
func Setup(cfg config.Log) {
	slog.SetDefault(slog.New(buildHandler(os.Stderr, cfg)))
}

// buildHandler constructs the slog handler for the given output and config. It
// is separate from Setup so tests can capture output into a buffer.
func buildHandler(w io.Writer, cfg config.Log) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
	if cfg.Format == "json" {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// parseLevel maps a config level name to a slog.Level, defaulting to Info.
func parseLevel(name string) slog.Level {
	switch name {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
