package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/raphi011/knowhow/internal/db"
)

// NoAuthMiddleware injects an admin AuthContext with access to all vaults,
// bypassing token validation entirely. Use only for local/Docker setups.
func NoAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac, err := Authenticate(r.Context(), nil, "", true)
		if err != nil {
			slog.Error("no-auth mode: unexpected error", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAuth(r.Context(), ac)))
	})
}

// Middleware validates Bearer tokens and injects AuthContext.
func Middleware(dbClient *db.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}
			rawToken := strings.TrimPrefix(header, "Bearer ")

			ac, err := Authenticate(r.Context(), dbClient, rawToken, false)
			if err != nil {
				slog.Warn("token validation failed", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithAuth(r.Context(), ac)))
		})
	}
}
