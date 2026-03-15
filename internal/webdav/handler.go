package webdav

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/webdav"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// vaultMap is a lazily-initialized, concurrent-safe per-vault store.
type vaultMap[T any] struct {
	m     sync.Map
	newFn func() T
}

func newVaultMap[T any](newFn func() T) *vaultMap[T] {
	return &vaultMap[T]{newFn: newFn}
}

func (vm *vaultMap[T]) Get(vaultID string) T {
	if v, ok := vm.m.Load(vaultID); ok {
		return v.(T)
	}
	v, _ := vm.m.LoadOrStore(vaultID, vm.newFn())
	return v.(T)
}

func (vm *vaultMap[T]) Range(fn func(vaultID string, v T) bool) {
	vm.m.Range(func(key, value any) bool {
		return fn(key.(string), value.(T))
	})
}

// NewHandler creates an http.Handler that serves WebDAV for vault documents.
// The path prefix is stripped from incoming requests (e.g. "/dav/default/").
// Auth uses HTTP Basic Auth where the password is a know API token.
// maxPutBytes limits the size of PUT request bodies (0 = no limit).
func NewHandler(
	ctx context.Context,
	pathPrefix string,
	dbClient *db.Client,
	fileSvc *file.Service,
	vaultSvc *vault.Service,
	noAuth bool,
	maxPutBytes int64,
) http.Handler {
	// Per-vault lock systems to isolate WebDAV locks across vaults.
	lockSystems := newVaultMap(func() webdav.LockSystem { return webdav.NewMemLS() })

	// Per-vault pending sets track files claimed by Finder's two-phase PUT
	// but not yet written with real content. Prevents ghost empty documents.
	pendingSets := newVaultMap(func() *pendingSet { return newPendingSet() })

	// Background goroutine sweeps expired pending entries every 30s.
	// Stops when ctx is cancelled (server shutdown).
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pendingSets.Range(func(_ string, ps *pendingSet) bool {
					ps.Sweep(60 * time.Second)
					return true
				})
			}
		}
	}()

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

		// Fast-path: short-circuit OS metadata files (._*, .DS_Store) and unsupported
		// file types (.pdf, .txt, .docx, etc.) before auth. These files are never stored,
		// so we return canned responses to prevent macOS Finder from aborting batch copies.
		filePath := ""
		if len(parts) > 1 {
			filePath = "/" + parts[1]
		}
		unsupported := isUnsupportedFile(filePath)
		if filePath != "" && (isOSMetadataFile(filePath) || unsupported) {
			switch r.Method {
			case "PROPFIND", http.MethodGet, http.MethodHead:
				http.Error(w, "not found", http.StatusNotFound)
			case http.MethodPut:
				// Drain body so connection stays clean for keep-alive.
				// Error is harmless: body is discarded and connection may just close.
				_, _ = io.Copy(io.Discard, r.Body)
				if unsupported {
					slog.Info("webdav: discarded unsupported file", "path", filePath)
				}
				w.WriteHeader(http.StatusCreated)
			case "LOCK":
				writeFakeLockResponse(w, filePath)
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
				w.Header().Set("WWW-Authenticate", `Basic realm="know"`)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			rawToken = password
		}

		ac, err := auth.Authenticate(r.Context(), dbClient, rawToken, noAuth)
		if err != nil {
			logutil.FromCtx(r.Context()).Warn("webdav: auth failed", "error", err)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// Resolve vault + check access
		vaultID, err := auth.ResolveVault(r.Context(), ac, vaultSvc, vaultName)
		if err != nil {
			logutil.FromCtx(r.Context()).Warn("webdav: vault resolution failed", "vault", vaultName, "error", err)
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
		davFS := NewFS(vaultID, dbClient, fileSvc, vaultSvc, pendingSets.Get(vaultID))
		reqLogger := logutil.FromCtx(r.Context())
		davHandler := &webdav.Handler{
			FileSystem: davFS,
			LockSystem: lockSystems.Get(vaultID),
			Prefix:     pathPrefix + vaultName,
			Logger: func(r *http.Request, err error) {
				if err == nil {
					return
				}
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
					reqLogger.Debug("webdav request",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err)
				} else {
					reqLogger.Warn("webdav request failed",
						"method", r.Method,
						"path", r.URL.Path,
						"error", err)
				}
			},
		}

		davStart := time.Now()
		wrec := &singleWriteResponseWriter{ResponseWriter: w}
		davHandler.ServeHTTP(wrec, r)
		reqLogger.Debug("webdav request",
			"method", r.Method, "vault", vaultName, "path", filePath,
			"status", wrec.statusCode, "duration_ms", time.Since(davStart).Milliseconds())
	})
}

// singleWriteResponseWriter wraps an http.ResponseWriter to suppress duplicate
// WriteHeader calls. The x/net/webdav library internally calls WriteHeader twice
// in some error paths (known issue), which spams server logs with
// "superfluous response.WriteHeader call" warnings.
type singleWriteResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
	statusCode  int
}

func (w *singleWriteResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		if code != w.statusCode {
			slog.Debug("webdav: suppressed duplicate WriteHeader", "first", w.statusCode, "suppressed", code)
		}
		return
	}
	w.wroteHeader = true
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *singleWriteResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusOK
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter so http.Flusher and other
// interfaces can be discovered via ResponseController.
func (w *singleWriteResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// isWriteMethod returns true for HTTP methods that modify resources.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK":
		return true
	}
	return false
}

// writeFakeLockResponse writes a valid 200 LOCK response with a fake lock token
// for files that are silently discarded (OS metadata, unsupported file types).
// This prevents macOS Finder from aborting the entire copy operation.
func writeFakeLockResponse(w http.ResponseWriter, filePath string) {
	token := "opaquelocktoken:" + uuid.NewString()
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<D:prop xmlns:D="DAV:">
  <D:lockdiscovery>
    <D:activelock>
      <D:locktype><D:write/></D:locktype>
      <D:lockscope><D:exclusive/></D:lockscope>
      <D:depth>0</D:depth>
      <D:owner><D:href>finder</D:href></D:owner>
      <D:timeout>Second-60</D:timeout>
      <D:locktoken><D:href>%s</D:href></D:locktoken>
      <D:lockroot><D:href>%s</D:href></D:lockroot>
    </D:activelock>
  </D:lockdiscovery>
</D:prop>`, token, html.EscapeString(filePath))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Lock-Token", "<"+token+">")
	w.WriteHeader(http.StatusOK)
	// Write error is harmless: response is best-effort for discarded files.
	_, _ = io.WriteString(w, body)
}
