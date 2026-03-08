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
		var rawToken string
		if !noAuth {
			_, password, ok := r.BasicAuth()
			if !ok || password == "" {
				w.Header().Set("WWW-Authenticate", `Basic realm="knowhow"`)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			rawToken = password
		}

		ac, err := auth.Authenticate(r.Context(), dbClient, rawToken, noAuth)
		if err != nil {
			slog.Warn("webdav: auth failed", "error", err)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// Resolve vault + check access
		vaultID, err := auth.ResolveVault(r.Context(), ac, vaultSvc, vaultName)
		if err != nil {
			slog.Warn("webdav: vault resolution failed", "vault", vaultName, "error", err)
			http.Error(w, "vault not found or access denied", http.StatusForbidden)
			return
		}

		// Reject write operations for read-only users
		if isWriteMethod(r.Method) {
			if err := auth.CheckVaultRole(ac, vaultID, models.RoleWrite); err != nil {
				http.Error(w, "read-only access", http.StatusForbidden)
				return
			}
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

// isWriteMethod returns true for HTTP methods that modify resources.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK":
		return true
	}
	return false
}
