package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
)

// MiddlewareConfig holds configuration for the auth middleware.
type MiddlewareConfig struct {
	TrustXForwardedFor  bool
	ResourceMetadataURL string // If set, 401 responses include WWW-Authenticate with resource_metadata (RFC 9728)
}

// NoAuthMiddleware injects an admin AuthContext with access to all vaults,
// bypassing token validation entirely. Use only for local/Docker setups.
func NoAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac, err := Authenticate(r.Context(), nil, "", true)
		if err != nil {
			slog.Error("no-auth mode: unexpected error", "error", err)
			httputil.WriteProblem(w, http.StatusInternalServerError, "internal error")
			return
		}
		ctx := WithAuth(r.Context(), ac)
		logger := logutil.FromCtx(ctx).With("user_id", ac.UserID)
		ctx = logutil.WithLogger(ctx, logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Middleware validates Bearer tokens and injects AuthContext.
func Middleware(dbClient *db.Client, m *metrics.Metrics, mwCfg MiddlewareConfig) func(http.Handler) http.Handler {
	// Build the WWW-Authenticate header value once (RFC 9728).
	wwwAuth := "Bearer"
	if mwCfg.ResourceMetadataURL != "" {
		wwwAuth = fmt.Sprintf(`Bearer resource_metadata=%q`, mwCfg.ResourceMetadataURL)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			ip := httputil.ClientIP(r, mwCfg.TrustXForwardedFor)
			if !strings.HasPrefix(header, "Bearer ") {
				event := AuditFailure
				AuditLog(r.Context(), event,
					slog.String("reason", "missing_header"),
					slog.String("ip", ip),
				)
				m.RecordAuthEvent(string(event), string(event.Result()))
				w.Header().Set("WWW-Authenticate", wwwAuth)
				httputil.WriteProblem(w, http.StatusUnauthorized, "missing authorization header")
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
					slog.String("ip", ip),
				)
				m.RecordAuthEvent(string(event), string(event.Result()))
				w.Header().Set("WWW-Authenticate", wwwAuth)
				httputil.WriteProblem(w, http.StatusUnauthorized, "invalid token")
				return
			}

			event := AuditSuccess
			AuditLog(r.Context(), event,
				slog.String("user_id", ac.UserID),
				slog.String("ip", ip),
				slog.String("provider", string(ac.Provider)),
				slog.String("token_name", ac.TokenName),
			)
			m.RecordAuthEvent(string(event), string(event.Result()))

			ctx := WithAuth(r.Context(), ac)
			logger := logutil.FromCtx(ctx).With("user_id", ac.UserID, "token_name", ac.TokenName)
			ctx = logutil.WithLogger(ctx, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
