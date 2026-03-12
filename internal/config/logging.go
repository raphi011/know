package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
)

// SetupLogger creates a dual-output logger: text to stderr, JSON to file.
// Both handlers share the given LevelVar so level changes apply to both.
// Returns the logger, a cleanup function to close the file, and any error.
func SetupLogger(logFile string, levelVar *slog.LevelVar) (*slog.Logger, func() error, error) {
	// Stderr handler (text for readability)
	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: levelVar,
	})

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %s: %w", logFile, err)
	}

	// File handler (JSON for machine parsing)
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: levelVar,
	})

	// Fanout to both handlers
	logger := slog.New(slogmulti.Fanout(stderrHandler, fileHandler))

	cleanup := func() error {
		return file.Close()
	}

	return logger, cleanup, nil
}

// SetupLoggerWithWriters creates a logger with custom writers (for testing).
func SetupLoggerWithWriters(stderr, file io.Writer, levelVar *slog.LevelVar) *slog.Logger {
	stderrHandler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: levelVar})
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: levelVar})
	return slog.New(slogmulti.Fanout(stderrHandler, fileHandler))
}
