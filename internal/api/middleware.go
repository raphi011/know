package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
)

// RequestLogMiddleware generates a request_id, enriches the context logger,
// logs request completion at Debug level with timing, and records HTTP metrics.
// If m is nil, metrics recording is skipped.
func RequestLogMiddleware(m *metrics.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := uuid.NewString()

		logger := slog.Default().With(
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)
		ctx := logutil.WithLogger(r.Context(), logger)

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		duration := time.Since(start)
		logger.Debug("http request", "status", rec.statusCode, "duration_ms", duration.Milliseconds())

		if m != nil {
			// Use r.Pattern (Go 1.22+) for the route pattern to avoid high
			// cardinality from path parameters like /conversations/{id}.
			pattern := r.Pattern
			if pattern == "" {
				pattern = r.URL.Path
			}
			m.RecordHTTPRequest(r.Method, pattern, rec.statusCode, duration)
		}
	})
}

// SecurityHeadersMiddleware sets standard security response headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying writer.
// This is required for SSE streaming — direct type assertions like
// w.(http.Flusher) don't use Unwrap(), so the wrapper must implement
// the interface explicitly.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter so other optional
// interfaces (e.g. http.Hijacker) can be discovered via ResponseController.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
