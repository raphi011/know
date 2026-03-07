package web

import (
	"log/slog"
	"net/http"
	"strings"
)

// CSRFMiddleware protects state-changing requests (POST, PUT, DELETE, PATCH)
// by validating the Origin or Referer header. This is a simple same-origin
// check that works with HTMX (which always sends these headers).
//
// Requests without both Origin and Referer are rejected. This prevents
// cross-site form submissions from malicious pages.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			if isSameOrigin(r, origin) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("web: CSRF origin mismatch", "origin", origin, "host", r.Host)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		referer := r.Header.Get("Referer")
		if referer != "" {
			if isSameOriginReferer(r, referer) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("web: CSRF referer mismatch", "referer", referer, "host", r.Host)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// No Origin or Referer — reject
		slog.Warn("web: CSRF missing Origin and Referer", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

// isSameOrigin checks if the Origin header matches the request's host.
func isSameOrigin(r *http.Request, origin string) bool {
	// Origin is scheme://host[:port]
	// Extract the host portion after "://"
	parts := strings.SplitN(origin, "://", 2)
	if len(parts) != 2 {
		return false
	}
	originHost := parts[1]
	return originHost == r.Host
}

// isSameOriginReferer checks if the Referer URL's host matches the request's host.
func isSameOriginReferer(r *http.Request, referer string) bool {
	// Referer is a full URL: scheme://host[:port]/path
	parts := strings.SplitN(referer, "://", 2)
	if len(parts) != 2 {
		return false
	}
	// Extract host from host[:port]/path
	hostAndPath := parts[1]
	host := strings.SplitN(hostAndPath, "/", 2)[0]
	return host == r.Host
}
