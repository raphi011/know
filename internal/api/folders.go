package api

import (
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pathutil"
)

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	folders, err := s.app.DBClient().ListFolders(ctx, vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list folders")
		logger.Error("list folders", "vault_id", vaultID, "error", err)
		return
	}

	parent := r.URL.Query().Get("parent")
	if parent != "" {
		parent = pathutil.NormalizeFolderPath(parent)
		var filtered []models.Folder
		for _, f := range folders {
			if pathutil.IsImmediateChildFolder(parent, f.Path) {
				filtered = append(filtered, f)
			}
		}
		folders = filtered
	}

	resp := make([]FolderResponse, len(folders))
	for i, f := range folders {
		resp[i] = FolderResponse{
			Path: f.Path,
			Name: f.Name,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
