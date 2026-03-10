package webdav

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// NewDebugLogger creates a logger that writes WebDAV request details to a file.
// Returns nil if path is empty. The caller should close the returned file.
func NewDebugLogger(path string) (*log.Logger, *os.File, error) {
	if path == "" {
		return nil, nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open dav debug log: %w", err)
	}
	return log.New(f, "", 0), f, nil
}

// DebugLogMiddleware wraps an http.Handler and logs every request with method,
// path, key headers, response status, and duration to the given logger.
func DebugLogMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Capture interesting WebDAV headers
		headers := []string{}
		for _, h := range []string{"Depth", "Destination", "Overwrite", "If", "Lock-Token", "Content-Type", "Content-Length"} {
			if v := r.Header.Get(h); v != "" {
				headers = append(headers, h+"="+v)
			}
		}
		headerStr := ""
		if len(headers) > 0 {
			headerStr = " [" + strings.Join(headers, ", ") + "]"
		}

		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)

		logger.Printf("%s %s %s%s → %d (%s)\n",
			time.Now().Format("15:04:05.000"),
			r.Method,
			r.URL.Path,
			headerStr,
			rec.status,
			time.Since(start).Round(time.Millisecond),
		)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}
