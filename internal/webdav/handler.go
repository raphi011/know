package webdav

import (
	"errors"
	"io"
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
// maxPutBytes limits the size of PUT request bodies (0 = no limit).
func NewHandler(
	pathPrefix string,
	dbClient *db.Client,
	docService *document.Service,
	vaultSvc *vault.Service,
	noAuth bool,
	maxPutBytes int64,
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
		// Always advertise WebDAV compliance so clients (e.g. macOS Finder)
		// recognise this as a WebDAV endpoint before authenticating.
		w.Header().Set("DAV", "1, 2")

		// Extract vault name from URL: /dav/{vaultName}/...
		trimmed := strings.TrimPrefix(r.URL.Path, pathPrefix)
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "vault name required in path", http.StatusBadRequest)
			return
		}
		vaultName := parts[0]

		// Fast-path: short-circuit OS metadata files (._*, .DS_Store) before auth.
		// The majority of Finder's requests are OS metadata files that never touch real data.
		filePath := ""
		if len(parts) > 1 {
			filePath = "/" + parts[1]
		}
		if filePath != "" && isOSMetadataFile(filePath) {
			switch r.Method {
			case "PROPFIND", http.MethodGet, http.MethodHead:
				http.Error(w, "not found", http.StatusNotFound)
			case http.MethodPut:
				// Drain body so connection stays clean for keep-alive.
				// Error is harmless: body is discarded and connection may just close.
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusCreated)
			case "LOCK":
				// Reject lock on metadata files — returning 423 tells clients
				// locking is not available, which they handle gracefully.
				w.WriteHeader(http.StatusLocked)
			case "UNLOCK", http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}

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

		// Reject PROPFIND Depth:infinity — unbounded recursion is a DoS vector (RFC 4918 §9.1).
		if r.Method == "PROPFIND" && r.Header.Get("Depth") == "infinity" {
			http.Error(w, "Depth: infinity not supported", http.StatusForbidden)
			return
		}

		// Limit PUT body size to prevent memory exhaustion (write-on-close buffers in memory).
		// The ContentLength check provides a clean 413 when the header is present.
		// MaxBytesReader is a fallback for chunked transfers without Content-Length;
		// in that case the x/net/webdav library surfaces a 500 because it cannot map
		// http.MaxBytesError to a WebDAV status code.
		if r.Method == http.MethodPut && maxPutBytes > 0 {
			if r.ContentLength > maxPutBytes {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxPutBytes)
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
