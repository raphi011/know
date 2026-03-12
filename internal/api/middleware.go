package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/raphi011/knowhow/internal/logutil"
)

// RequestLogMiddleware generates a request_id, enriches the context logger,
// and logs request completion at Debug level with timing.
func RequestLogMiddleware(next http.Handler) http.Handler {
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

		logger.Debug("http request", "status", rec.statusCode, "duration_ms", time.Since(start).Milliseconds())
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
