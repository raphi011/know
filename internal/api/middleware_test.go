package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raphi011/know/internal/logutil"
)

func TestRequestLogMiddleware_setsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	handler := RequestLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify logger is in context with request_id
		l := logutil.FromCtx(r.Context())
		l.Info("inner handler")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "request_id=") {
		t.Errorf("expected request_id in log output, got: %s", output)
	}
	if !strings.Contains(output, "http request") {
		t.Errorf("expected 'http request' log line, got: %s", output)
	}
	if !strings.Contains(output, "duration_ms=") {
		t.Errorf("expected duration_ms in log output, got: %s", output)
	}
}

func TestRequestLogMiddleware_capturesStatusCode(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	handler := RequestLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "status=404") {
		t.Errorf("expected status=404 in log output, got: %s", output)
	}
}
