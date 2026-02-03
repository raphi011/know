package config

import (
	"io"
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
)

// SetupLogger creates a dual-output logger: text to stderr, JSON to file.
// Returns the logger and a cleanup function to close the file.
func SetupLogger(logFile string, level slog.Level) (*slog.Logger, func() error) {
	// Stderr handler (text for readability)
	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	// Try to open log file
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fall back to stderr-only if file fails
		slog.Error("failed to open log file, using stderr only", "error", err, "file", logFile)
		return slog.New(stderrHandler), func() error { return nil }
	}

	// File handler (JSON for machine parsing)
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: level,
	})

	// Fanout to both handlers
	logger := slog.New(slogmulti.Fanout(stderrHandler, fileHandler))

	cleanup := func() error {
		return file.Close()
	}

	return logger, cleanup
}

// SetupLoggerWithWriters creates a logger with custom writers (for testing).
func SetupLoggerWithWriters(stderr, file io.Writer, level slog.Level) *slog.Logger {
	stderrHandler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level})
	return slog.New(slogmulti.Fanout(stderrHandler, fileHandler))
}
