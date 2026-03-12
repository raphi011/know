package logutil

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

func TestFromCtx_fallsBackToDefault(t *testing.T) {
	ctx := context.Background()
	got := FromCtx(ctx)
	if got != slog.Default() {
		t.Error("expected slog.Default() when no logger in context")
	}
}

func TestWithLogger_roundTrips(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx := WithLogger(context.Background(), logger)
	got := FromCtx(ctx)

	if got != logger {
		t.Error("expected stored logger to be returned")
	}
}

func TestFromCtx_nilLoggerFallsBackToDefault(t *testing.T) {
	ctx := WithLogger(context.Background(), nil)
	got := FromCtx(ctx)
	if got != slog.Default() {
		t.Error("expected slog.Default() when nil logger stored in context")
	}
}

func TestFromCtx_propagatesAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	enriched := logger.With("request_id", "abc-123")

	ctx := WithLogger(context.Background(), enriched)
	FromCtx(ctx).Debug("test message", "extra", "val")

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("request_id=abc-123")) {
		t.Errorf("expected request_id in output, got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("extra=val")) {
		t.Errorf("expected extra in output, got: %s", output)
	}
}
