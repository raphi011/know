package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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

// Register wires all /api/ routes onto the given mux.
func (s *Server) Register(mux *http.ServeMux, authMw func(http.Handler) http.Handler) {
	// Conversations
	mux.Handle("GET /api/conversations", authMw(http.HandlerFunc(s.listConversations)))
	mux.Handle("GET /api/conversations/{id}", authMw(http.HandlerFunc(s.getConversation)))
	mux.Handle("POST /api/conversations", authMw(http.HandlerFunc(s.createConversation)))
	mux.Handle("DELETE /api/conversations/{id}", authMw(http.HandlerFunc(s.deleteConversation)))
	mux.Handle("PATCH /api/conversations/{id}", authMw(http.HandlerFunc(s.renameConversation)))

	// Vaults
	mux.Handle("GET /api/vaults", authMw(http.HandlerFunc(s.listVaults)))
	mux.Handle("GET /api/vaults/{name}/info", authMw(http.HandlerFunc(s.getVaultInfo)))
	mux.Handle("GET /api/vaults/{name}/settings", authMw(http.HandlerFunc(s.getVaultSettings)))
	mux.Handle("PATCH /api/vaults/{name}/settings", authMw(http.HandlerFunc(s.updateVaultSettings)))

	// Documents
	mux.Handle("GET /api/ls", authMw(http.HandlerFunc(s.ls)))
	mux.Handle("GET /api/documents", authMw(http.HandlerFunc(s.getDocument)))
	mux.Handle("POST /api/documents", authMw(http.HandlerFunc(s.upsertDocument)))
	mux.Handle("DELETE /api/documents", authMw(http.HandlerFunc(s.deleteDocuments)))

	// Move (document or folder)
	mux.Handle("POST /api/move", authMw(http.HandlerFunc(s.move)))

	// Assets
	mux.Handle("POST /api/assets", authMw(http.HandlerFunc(s.uploadAsset)))
	mux.Handle("GET /api/assets", authMw(http.HandlerFunc(s.getAsset)))
	mux.Handle("GET /api/assets/meta", authMw(http.HandlerFunc(s.getAssetMeta)))
	mux.Handle("DELETE /api/assets", authMw(http.HandlerFunc(s.deleteAsset)))

	// Import (two-phase: manifest + upload)
	mux.Handle("POST /api/import/manifest", authMw(http.HandlerFunc(s.importManifest)))
	mux.Handle("POST /api/import/upload", authMw(http.HandlerFunc(s.importUpload)))

	// Bulk upload (used by integration tests; CLI uses import/* endpoints)
	mux.Handle("POST /api/bulk", authMw(http.HandlerFunc(s.bulkUpload)))

	// Search
	mux.Handle("GET /api/search", authMw(http.HandlerFunc(s.searchDocuments)))

	// Folders
	mux.Handle("GET /api/folders", authMw(http.HandlerFunc(s.listFolders)))
	mux.Handle("PATCH /api/folders", authMw(http.HandlerFunc(s.updateFolder)))

	// Versions
	mux.Handle("GET /api/versions", authMw(http.HandlerFunc(s.listVersions)))

	// Labels
	mux.Handle("GET /api/labels", authMw(http.HandlerFunc(s.listLabels)))

	// Tasks
	mux.Handle("GET /api/tasks", authMw(http.HandlerFunc(s.listTasks)))
	mux.Handle("POST /api/tasks/{id}/toggle", authMw(http.HandlerFunc(s.toggleTask)))

	// Remotes (federation)
	mux.Handle("GET /api/remotes", authMw(http.HandlerFunc(s.listRemotes)))
	mux.Handle("POST /api/remotes", authMw(http.HandlerFunc(s.addRemote)))
	mux.Handle("DELETE /api/remotes/{name}", authMw(http.HandlerFunc(s.removeRemote)))

	// External links
	mux.Handle("GET /api/external-links/stats", authMw(http.HandlerFunc(s.externalLinkStats)))
	mux.Handle("GET /api/external-links", authMw(http.HandlerFunc(s.listExternalLinks)))

	// Export
	mux.Handle("GET /api/export", authMw(http.HandlerFunc(s.export)))
	mux.Handle("GET /api/export/epub", authMw(http.HandlerFunc(s.exportEPUB)))

	// Jobs (pipeline status)
	mux.Handle("GET /api/jobs", authMw(http.HandlerFunc(s.getJobStatus)))

	// Web clipping
	mux.Handle("POST /api/fetch", authMw(http.HandlerFunc(s.fetchWebpage)))

	// Config
	mux.Handle("GET /api/config", authMw(http.HandlerFunc(s.getConfig)))
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("failed to encode JSON response", "error", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeBody reads and JSON-decodes the request body with a size limit.
// Returns the decoded value and true on success, or writes an error response and returns nil, false.
func decodeBody[T any](w http.ResponseWriter, r *http.Request, maxBytes int64) (*T, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			logutil.FromCtx(r.Context()).Debug("invalid request body", "error", err)
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return nil, false
	}
	return &v, true
}
