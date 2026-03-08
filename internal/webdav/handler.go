package webdav

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

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
	// Per-vault lock systems to isolate WebDAV locks across vaults.
	var lockSystems sync.Map // vaultID → webdav.LockSystem

	getLockSystem := func(vaultID string) webdav.LockSystem {
		if ls, ok := lockSystems.Load(vaultID); ok {
			return ls.(webdav.LockSystem)
		}
		ls, _ := lockSystems.LoadOrStore(vaultID, webdav.NewMemLS())
		return ls.(webdav.LockSystem)
	}

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

			info, err := auth.ValidateToken(r.Context(), dbClient, password)
			if err != nil {
				slog.Warn("webdav: token validation failed", "error", err)
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}

			ac = auth.AuthContext{
				UserID:      info.UserID,
				VaultAccess: info.VaultAccess,
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
			LockSystem: getLockSystem(vaultID),
			Prefix:     pathPrefix + vaultName,
			Logger: func(r *http.Request, err error) {
				if err == nil {
					return
				}
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
					slog.Debug("webdav request",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err)
				} else {
					slog.Warn("webdav request failed",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err)
				}
			},
		}

		davHandler.ServeHTTP(w, r)
	})
}
