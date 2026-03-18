package api

import (
	"net/http"
	"path"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pathutil"
)

type updateFolderRequest struct {
	Vault   string `json:"vault"`
	Path    string `json:"path"`
	NoEmbed *bool  `json:"no_embed"`
}

type updateFolderResponse struct {
	StrippedChunks int `json:"stripped_chunks"`
}

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

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) updateFolder(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[updateFolderRequest](w, r, 1<<16)
	if !ok {
		return
	}

	if req.Vault == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "vault and path are required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), req.Vault, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	db := s.app.DBClient()

	req.Path = pathutil.NormalizeFolderPath(req.Path)

	folder, err := db.GetFolderByPath(ctx, req.Vault, req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get folder")
		logger.Error("update folder: get folder", "error", err)
		return
	}
	if folder == nil {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}

	var stripped int

	if req.NoEmbed != nil {
		if err := db.SetFolderNoEmbed(ctx, req.Vault, req.Path, *req.NoEmbed); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update folder")
			logger.Error("update folder: set no_embed", "error", err)
			return
		}
		if *req.NoEmbed {
			n, err := db.StripEmbeddingsByFolder(ctx, req.Vault, req.Path)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to strip embeddings")
				logger.Error("update folder: strip embeddings", "error", err)
				return
			}
			stripped = n
		}
	}

	writeJSON(w, http.StatusOK, updateFolderResponse{StrippedChunks: stripped})
}
