package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
)

// LogFormat represents a validated log output format.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// ParseLogFormat validates a format string. Returns an error for unknown values.
func ParseLogFormat(s string) (LogFormat, error) {
	switch s {
	case "text", "":
		return LogFormatText, nil
	case "json":
		return LogFormatJSON, nil
	default:
		return "", fmt.Errorf("unknown log format %q (valid: text, json)", s)
	}
}

// newHandler creates a slog handler for the given writer and format.
func newHandler(w io.Writer, levelVar *slog.LevelVar, format LogFormat) slog.Handler {
	opts := &slog.HandlerOptions{Level: levelVar}
	if format == LogFormatJSON {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// SetupLogger creates a logger that respects the given format.
//
//   - No log file: single handler to stderr in the chosen format.
//   - Log file set: fanout to stderr (always text) + file (chosen format).
//
// The LevelVar is shared so level changes apply to all handlers.
// Returns the logger, a cleanup function to close the file, and any error.
func SetupLogger(logFile string, levelVar *slog.LevelVar, format LogFormat) (*slog.Logger, func() error, error) {
	noop := func() error { return nil }

	if logFile == "" {
		return slog.New(newHandler(os.Stderr, levelVar, format)), noop, nil
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, noop, fmt.Errorf("open log file %s: %w", logFile, err)
	}

	cleanup := func() error { return file.Close() }

	// Stderr always uses text for readability; file uses chosen format.
	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: levelVar})
	fileHandler := newHandler(file, levelVar, format)

	return slog.New(slogmulti.Fanout(stderrHandler, fileHandler)), cleanup, nil
}

// SetupLoggerWithWriters creates a logger with custom writers (for testing).
func SetupLoggerWithWriters(stderr, file io.Writer, levelVar *slog.LevelVar) *slog.Logger {
	stderrHandler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: levelVar})
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: levelVar})
	return slog.New(slogmulti.Fanout(stderrHandler, fileHandler))
}
