package api

import (
	"net/http"
	"strconv"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.URL.Query().Get("path")

	if path == "" {
		writeError(w, http.StatusBadRequest, "path parameter required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	doc, err := s.app.DBClient().GetFileByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get document")
		logger.Error("list versions: get document", "vault_id", vaultID, "path", path, "error", err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid document ID")
		logger.Error("list versions: extract doc ID", "path", path, "error", err)
		return
	}

	versions, err := s.app.DBClient().ListVersions(ctx, docID, limit, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list versions")
		logger.Error("list versions", "doc_id", docID, "error", err)
		return
	}

	resp := make([]VersionResponse, len(versions))
	for i, v := range versions {
		resp[i] = VersionResponse{
			Version:     v.Version,
			Title:       v.Title,
			ContentHash: v.ContentHash,
			CreatedAt:   v.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
