package api

import (
	"net/http"
	"path"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pathutil"
)

type updateFolderRequest struct {
	Path    string `json:"path"`
	NoEmbed *bool  `json:"no_embed"`
}

type updateFolderResponse struct {
	StrippedChunks int `json:"strippedChunks"`
}

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	logger := logutil.FromCtx(ctx)

	folders, err := s.app.DBClient().ListFolders(ctx, vaultID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list folders")
		logger.Error("list folders", "vault_id", vaultID, "error", err)
		return
	}

	parent := r.URL.Query().Get("parent")
	if parent != "" {
		parent = pathutil.NormalizeFolderPath(parent)
		var filtered []models.File
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
			Path:    f.Path,
			Name:    path.Base(f.Path),
			NoEmbed: f.NoEmbed,
		}
	}

	writeJSON(w, http.StatusOK, httputil.NewListResponse(resp, len(resp)))
}

func (s *Server) updateFolder(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[updateFolderRequest](w, r, 1<<16)
	if !ok {
		return
	}

	if req.Path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path is required")
		return
	}

	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(ctx)
	db := s.app.DBClient()

	req.Path = models.NormalizePath(req.Path)

	folder, err := db.GetFolderByPath(ctx, vaultID, req.Path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get folder")
		logger.Error("update folder: get folder", "vault_id", vaultID, "path", req.Path, "error", err)
		return
	}
	if folder == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "folder not found")
		return
	}

	var stripped int

	if req.NoEmbed != nil {
		if err := db.SetFolderNoEmbed(ctx, vaultID, req.Path, *req.NoEmbed); err != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, "failed to update folder")
			logger.Error("update folder: set no_embed", "vault_id", vaultID, "path", req.Path, "error", err)
			return
		}
		if *req.NoEmbed {
			n, err := db.StripEmbeddingsByFolder(ctx, vaultID, req.Path)
			if err != nil {
				httputil.WriteProblem(w, http.StatusInternalServerError, "failed to strip embeddings")
				logger.Error("update folder: strip embeddings", "vault_id", vaultID, "path", req.Path, "error", err)
				return
			}
			stripped = n
		}
	}

	writeJSON(w, http.StatusOK, updateFolderResponse{StrippedChunks: stripped})
}
