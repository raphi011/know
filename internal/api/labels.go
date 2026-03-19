package api

import (
	"net/http"
	"slices"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listLabels(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())

	logger := logutil.FromCtx(r.Context())

	if r.URL.Query().Get("counts") == "true" {
		counts, err := s.app.DBClient().ListLabelsWithCounts(r.Context(), vaultID)
		if err != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list labels")
			logger.Error("list labels with counts", "vault_id", vaultID, "error", err)
			return
		}
		writeJSON(w, http.StatusOK, httputil.NewListResponse(counts, len(counts)))
		return
	}

	labels, err := s.app.DBClient().ListLabels(r.Context(), vaultID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list labels")
		logger.Error("list labels", "vault_id", vaultID, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, httputil.NewListResponse(labels, len(labels)))
}

type patchLabelsRequest struct {
	Path   string   `json:"path"`
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

func (s *Server) patchLabels(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	logger := logutil.FromCtx(r.Context())

	req, ok := decodeBody[patchLabelsRequest](w, r, 1<<16)
	if !ok {
		return
	}

	if req.Path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path is required")
		return
	}

	req.Path = models.NormalizePath(req.Path)

	file, err := s.app.DBClient().GetFileByPath(r.Context(), vaultID, req.Path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up document")
		logger.Error("patch labels: get file", "path", req.Path, "error", err)
		return
	}
	if file == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "document not found")
		return
	}

	fileID, err := models.RecordIDString(file.ID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid document record")
		logger.Error("patch labels: record ID", "path", req.Path, "error", err)
		return
	}

	// Compute new labels
	labels := file.Labels
	for _, l := range req.Add {
		if !slices.Contains(labels, l) {
			labels = append(labels, l)
		}
	}
	labels = slices.DeleteFunc(labels, func(l string) bool {
		return slices.Contains(req.Remove, l)
	})

	if err := s.app.DBClient().UpdateFileLabels(r.Context(), fileID, labels); err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to update labels")
		logger.Error("patch labels", "path", req.Path, "error", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"labels": labels})
}
