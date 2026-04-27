// Package logger wraps log/slog with consistent defaults for the
// wisp-ops-center processes. Logs are structured key=value pairs by
// default; LOG_FORMAT=json switches to JSON output.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Logger is the shared structured logger used across the project.
type Logger = slog.Logger

// New returns a new slog.Logger named after the given component.
func New(component string) *Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler).With("component", component)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
