package logging

import (
	"log/slog"
	"os"
)

// NewLogger creates a structured JSON logger for production use.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
