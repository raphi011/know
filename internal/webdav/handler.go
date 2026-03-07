package webdav

import (
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/net/webdav"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/vault"
)

// NewHandler creates an http.Handler that serves WebDAV for vault documents.
// The path prefix is stripped from incoming requests (e.g. "/dav/default/").
// Auth uses HTTP Basic Auth where the password is a knowhow API token.
func NewHandler(
	pathPrefix string,
	dbClient *db.Client,
	docService *document.Service,
	vaultSvc *vault.Service,
	noAuth bool,
) http.Handler {
	lockSystem := webdav.NewMemLS()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract vault name from URL: /dav/{vaultName}/...
		trimmed := strings.TrimPrefix(r.URL.Path, pathPrefix)
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "vault name required in path", http.StatusBadRequest)
			return
		}
		vaultName := parts[0]

		// Authenticate via HTTP Basic Auth (password = API token)
		var ac auth.AuthContext
		if noAuth {
			ac = auth.AuthContext{
				UserID:      "admin",
				VaultAccess: []string{auth.WildcardVaultAccess},
			}
		} else {
			_, password, ok := r.BasicAuth()
			if !ok || password == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="knowhow"`)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			hash := auth.HashToken(password)
			token, err := dbClient.GetTokenByHash(r.Context(), hash)
			if err != nil {
				slog.Warn("webdav: token lookup failed", "error", err)
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			if token == nil {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}

			vaultAccess := make([]string, 0, len(token.VaultAccess))
			for _, v := range token.VaultAccess {
				id, err := models.RecordIDString(v)
				if err != nil {
					slog.Warn("webdav: failed to extract vault access ID", "error", err)
					continue
				}
				vaultAccess = append(vaultAccess, id)
			}

			ac = auth.AuthContext{
				UserID:      "webdav",
				VaultAccess: vaultAccess,
			}
		}

		// Resolve vault
		v, err := vaultSvc.GetByName(r.Context(), vaultName)
		if err != nil {
			slog.Warn("webdav: vault lookup failed", "vault", vaultName, "error", err)
			http.Error(w, "vault not found", http.StatusNotFound)
			return
		}
		if v == nil {
			http.Error(w, "vault not found", http.StatusNotFound)
			return
		}

		vaultID, err := models.RecordIDString(v.ID)
		if err != nil {
			slog.Error("webdav: failed to extract vault ID", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Check vault access
		ctx := auth.WithAuth(r.Context(), ac)
		if err := auth.RequireVaultAccess(ctx, vaultID); err != nil {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}

		// Create per-request WebDAV handler with the resolved vault
		davFS := NewFS(vaultID, dbClient, docService, vaultSvc)
		davHandler := &webdav.Handler{
			FileSystem: davFS,
			LockSystem: lockSystem,
			Prefix:     pathPrefix + vaultName,
			Logger: func(r *http.Request, err error) {
				if err != nil {
					slog.Debug("webdav request",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err)
				}
			},
		}

		davHandler.ServeHTTP(w, r)
	})
}
