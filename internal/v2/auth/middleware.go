package auth

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
)

// Middleware validates Bearer tokens and injects AuthContext.
func Middleware(dbClient *v2db.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}
			rawToken := strings.TrimPrefix(header, "Bearer ")
			hash := HashToken(rawToken)

			token, err := dbClient.GetTokenByHash(r.Context(), hash)
			if err != nil {
				slog.Warn("token lookup failed", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			if token == nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			// Check expiry
			if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
				http.Error(w, "token expired", http.StatusUnauthorized)
				return
			}

			// Update last used (fire and forget)
			tokenID, err := models.RecordIDString(token.ID)
			if err == nil {
				go func() {
					if err := dbClient.UpdateTokenLastUsed(r.Context(), tokenID); err != nil {
						slog.Warn("failed to update token last_used", "error", err)
					}
				}()
			}

			// Extract vault access IDs
			vaultAccess := make([]string, 0, len(token.VaultAccess))
			for _, v := range token.VaultAccess {
				if id, err := models.RecordIDString(v); err == nil {
					vaultAccess = append(vaultAccess, id)
				}
			}

			// Extract user ID
			userID, err := models.RecordIDString(token.User)
			if err != nil {
				http.Error(w, "invalid token user", http.StatusInternalServerError)
				return
			}

			ac := AuthContext{
				UserID:      userID,
				VaultAccess: vaultAccess,
			}
			ctx := WithAuth(r.Context(), ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
