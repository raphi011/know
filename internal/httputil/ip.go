package httputil

import (
	"net/http"
	"strings"
)

// ClientIP extracts the client IP from an HTTP request.
// When trustXFF is true, the first entry in X-Forwarded-For is preferred
// (for reverse proxy setups). Otherwise, RemoteAddr is always used.
func ClientIP(r *http.Request, trustXFF bool) string {
	if trustXFF {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// XFF is "client, proxy1, proxy2" — take the first (client) IP.
			if before, _, ok := strings.Cut(xff, ","); ok {
				return strings.TrimSpace(before)
			}
			return xff
		}
	}
	return r.RemoteAddr
}
