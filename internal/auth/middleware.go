package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/db"
)

// NoAuthMiddleware injects an admin AuthContext with access to all vaults,
// bypassing token validation entirely. Use only for local/Docker setups.
func NoAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac := AuthContext{
			UserID:      "admin",
			VaultAccess: []string{WildcardVaultAccess},
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

			info, err := ValidateToken(r.Context(), dbClient, rawToken)
			if err != nil {
				slog.Warn("token validation failed", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			// Update last used (fire and forget)
			if info.TokenID != "" {
				go func() {
					bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := dbClient.UpdateTokenLastUsed(bgCtx, info.TokenID); err != nil {
						slog.Warn("failed to update token last_used", "token_id", info.TokenID, "error", err)
					}
				}()
			}

			ac := AuthContext{
				UserID:      info.UserID,
				VaultAccess: info.VaultAccess,
			}
			ctx := WithAuth(r.Context(), ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
