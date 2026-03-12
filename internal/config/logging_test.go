package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSetupLoggerWithWriters_dynamicLevel(t *testing.T) {
	var stderr, file bytes.Buffer

	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelWarn)

	logger := SetupLoggerWithWriters(&stderr, &file, levelVar)

	// Debug should be suppressed at Warn level
	logger.Debug("should be suppressed")
	if stderr.Len() > 0 {
		t.Error("debug message should be suppressed at warn level")
	}

	// Warn should appear
	logger.Warn("visible warning")
	if !strings.Contains(stderr.String(), "visible warning") {
		t.Errorf("expected warning in stderr, got: %s", stderr.String())
	}
	if !strings.Contains(file.String(), "visible warning") {
		t.Errorf("expected warning in file, got: %s", file.String())
	}

	// Switch to Debug level
	stderr.Reset()
	file.Reset()
	levelVar.Set(slog.LevelDebug)

	logger.Debug("now visible")
	if !strings.Contains(stderr.String(), "now visible") {
		t.Errorf("expected debug message in stderr after level change, got: %s", stderr.String())
	}
	if !strings.Contains(file.String(), "now visible") {
		t.Errorf("expected debug message in file after level change, got: %s", file.String())
	}
}
