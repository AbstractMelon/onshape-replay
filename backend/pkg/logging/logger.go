package logging

import (
	"log/slog"
	"os"
)

// New returns a structured logger writing JSON to stdout.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// NewText returns a structured logger writing human-readable text to stdout.
func NewText() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}
