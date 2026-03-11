package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/server"
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

	// Documents
	mux.Handle("GET /api/documents", authMw(http.HandlerFunc(s.getDocument)))
	mux.Handle("POST /api/documents", authMw(http.HandlerFunc(s.upsertDocument)))

	// Assets
	mux.Handle("POST /api/assets", authMw(http.HandlerFunc(s.uploadAsset)))
	mux.Handle("GET /api/assets", authMw(http.HandlerFunc(s.getAsset)))
	mux.Handle("GET /api/assets/meta", authMw(http.HandlerFunc(s.getAssetMeta)))
	mux.Handle("DELETE /api/assets", authMw(http.HandlerFunc(s.deleteAsset)))

	// Bulk upload
	mux.Handle("POST /api/bulk", authMw(http.HandlerFunc(s.bulkUpload)))

	// Labels
	mux.Handle("GET /api/labels", authMw(http.HandlerFunc(s.listLabels)))

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
