package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/agent"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/server"
)

// Server holds the REST API handlers.
type Server struct {
	app *server.App
}

// NewServer creates a new REST API server.
func NewServer(app *server.App) *Server {
	return &Server{app: app}
}

// Register wires all /api/v1/ routes onto the given mux.
// authMw validates the Bearer token and injects AuthContext.
// agentRunner is optional — if nil, agent routes are not registered.
func (s *Server) Register(mux *http.ServeMux, authMw func(http.Handler) http.Handler, agentRunner *agent.Runner) {
	// v aliases for vault-scoped and global routes.
	// vs wraps handler with auth + vault scope (resolves {vault} name → ID in context).
	// g wraps handler with auth only (no vault scope).
	vs := func(handler http.HandlerFunc) http.Handler {
		return authMw(s.vaultScope(handler))
	}
	g := func(handler http.HandlerFunc) http.Handler {
		return authMw(handler)
	}

	// --- Vaults (list is global, rest is vault-scoped) ---
	mux.Handle("GET /api/v1/vaults", g(s.listVaults))
	mux.Handle("GET /api/v1/vaults/{vault}", vs(s.getVaultInfo))
	mux.Handle("GET /api/v1/vaults/{vault}/settings", vs(s.getVaultSettings))
	mux.Handle("PATCH /api/v1/vaults/{vault}/settings", vs(s.updateVaultSettings))

	// --- Documents ---
	mux.Handle("GET /api/v1/vaults/{vault}/documents", vs(s.getDocument))
	mux.Handle("POST /api/v1/vaults/{vault}/documents", vs(s.upsertDocument))
	mux.Handle("DELETE /api/v1/vaults/{vault}/documents", vs(s.deleteDocuments))
	mux.Handle("GET /api/v1/vaults/{vault}/documents/ls", vs(s.ls))
	mux.Handle("POST /api/v1/vaults/{vault}/documents/move", vs(s.move))
	mux.Handle("POST /api/v1/vaults/{vault}/documents/bulk", vs(s.bulkUpload))
	mux.Handle("POST /api/v1/vaults/{vault}/documents/clip", vs(s.fetchWebpage))

	// --- Import (two-phase) ---
	mux.Handle("POST /api/v1/vaults/{vault}/import/manifest", vs(s.importManifest))
	mux.Handle("POST /api/v1/vaults/{vault}/import/upload", vs(s.importUpload))

	// --- Export ---
	mux.Handle("GET /api/v1/vaults/{vault}/export", vs(s.export))
	mux.Handle("GET /api/v1/vaults/{vault}/export/epub", vs(s.exportEPUB))

	// --- Backup / Restore ---
	mux.Handle("GET /api/v1/vaults/{vault}/backup", vs(s.exportBackup))
	mux.Handle("POST /api/v1/backup/restore", g(s.restoreBackup)) // global: creates vault from archive

	// --- Search ---
	mux.Handle("GET /api/v1/vaults/{vault}/search", vs(s.searchDocuments))

	// --- Folders ---
	mux.Handle("GET /api/v1/vaults/{vault}/folders", vs(s.listFolders))
	mux.Handle("PATCH /api/v1/vaults/{vault}/folders", vs(s.updateFolder))

	// --- Versions ---
	mux.Handle("GET /api/v1/vaults/{vault}/versions", vs(s.listVersions))

	// --- Labels ---
	mux.Handle("GET /api/v1/vaults/{vault}/labels", vs(s.listLabels))

	// --- Tasks ---
	mux.Handle("GET /api/v1/vaults/{vault}/tasks", vs(s.listTasks))
	mux.Handle("POST /api/v1/vaults/{vault}/tasks/{id}/toggle", vs(s.toggleTask))

	// --- Assets ---
	mux.Handle("POST /api/v1/vaults/{vault}/assets", vs(s.uploadAsset))
	mux.Handle("GET /api/v1/vaults/{vault}/assets", vs(s.getAsset))
	mux.Handle("GET /api/v1/vaults/{vault}/assets/meta", vs(s.getAssetMeta))
	mux.Handle("DELETE /api/v1/vaults/{vault}/assets", vs(s.deleteAsset))

	// --- External Links ---
	mux.Handle("GET /api/v1/vaults/{vault}/external-links", vs(s.listExternalLinks))
	mux.Handle("GET /api/v1/vaults/{vault}/external-links/stats", vs(s.externalLinkStats))

	// --- Changes (incremental sync) ---
	mux.Handle("GET /api/v1/vaults/{vault}/changes", vs(s.getChanges))

	// --- SSE (document change stream, vault-scoped) ---
	// Registered in cmd_serve.go since it depends on the event bus.

	// --- Tokens ---
	mux.Handle("GET /api/v1/tokens", g(s.listTokens))
	mux.Handle("POST /api/v1/tokens", g(s.createToken))
	mux.Handle("DELETE /api/v1/tokens/{id}", g(s.deleteToken))
	mux.Handle("POST /api/v1/tokens/{id}/rotate", g(s.rotateToken))

	// --- Conversations (global, identified by ID) ---
	mux.Handle("GET /api/v1/conversations", g(s.listConversations))
	mux.Handle("POST /api/v1/conversations", g(s.createConversation))
	mux.Handle("GET /api/v1/conversations/{id}", g(s.getConversation))
	mux.Handle("PATCH /api/v1/conversations/{id}", g(s.renameConversation))
	mux.Handle("DELETE /api/v1/conversations/{id}", g(s.deleteConversation))

	// --- Agent ---
	if agentRunner != nil {
		mux.Handle("POST /api/v1/vaults/{vault}/agent/chat", vs(agentRunner.HandleChat()))
		mux.Handle("GET /api/v1/agent/events/{id}", g(agentRunner.HandleEvents()))
		mux.Handle("POST /api/v1/agent/cancel/{id}", g(agentRunner.HandleCancel()))
		mux.Handle("POST /api/v1/agent/approval", g(agentRunner.HandleApproval()))
	}

	// --- Admin (system admin only) ---
	mux.Handle("GET /api/v1/admin/users", g(s.listUsers))
	mux.Handle("POST /api/v1/admin/users", g(s.createUser))

	// --- Remotes (federation, system admin only) ---
	mux.Handle("GET /api/v1/remotes", g(s.listRemotes))
	mux.Handle("POST /api/v1/remotes", g(s.addRemote))
	mux.Handle("DELETE /api/v1/remotes/{name}", g(s.removeRemote))

	// --- Jobs (pipeline status) ---
	mux.Handle("GET /api/v1/jobs", g(s.getJobStatus))

	// --- Config ---
	mux.Handle("GET /api/v1/config", g(s.getConfig))
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to encode JSON response", "error", err)
	}
}

// decodeBody reads and JSON-decodes the request body with a size limit.
// Returns the decoded value and true on success, or writes an error response and returns nil, false.
func decodeBody[T any](w http.ResponseWriter, r *http.Request, maxBytes int64) (*T, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httputil.WriteProblem(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			logutil.FromCtx(r.Context()).Debug("invalid request body", "error", err)
			httputil.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		}
		return nil, false
	}
	return &v, true
}
