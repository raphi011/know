package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
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
		ctx := WithAuth(r.Context(), ac)
		logger := logutil.FromCtx(ctx).With("user_id", ac.UserID)
		ctx = logutil.WithLogger(ctx, logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Middleware validates Bearer tokens and injects AuthContext.
func Middleware(dbClient *db.Client, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				event := AuditFailure
				AuditLog(r.Context(), event,
					slog.String("reason", "missing_header"),
					slog.String("ip", clientIP(r)),
				)
				m.RecordAuthEvent(string(event), string(event.Result()))
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}
			rawToken := strings.TrimPrefix(header, "Bearer ")

			ac, err := Authenticate(r.Context(), dbClient, rawToken, false)
			if err != nil {
				event := AuditFailure
				reason := "invalid_token"
				if errors.Is(err, ErrTokenExpired) {
					event = AuditExpired
					reason = "token_expired"
				}
				AuditLog(r.Context(), event,
					slog.String("reason", reason),
					slog.String("ip", clientIP(r)),
				)
				m.RecordAuthEvent(string(event), string(event.Result()))
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			event := AuditSuccess
			AuditLog(r.Context(), event,
				slog.String("user_id", ac.UserID),
				slog.String("ip", clientIP(r)),
				slog.String("provider", string(ac.Provider)),
			)
			m.RecordAuthEvent(string(event), string(event.Result()))

			ctx := WithAuth(r.Context(), ac)
			logger := logutil.FromCtx(ctx).With("user_id", ac.UserID)
			ctx = logutil.WithLogger(ctx, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
