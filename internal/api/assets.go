package api

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
)

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	// 32 MB max memory for multipart parsing
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		slog.Warn("parse multipart form", "error", err)
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	vaultID := r.FormValue("vault")
	path := r.FormValue("path")
	if vaultID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "vault and path fields are required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if !models.IsImageFile(path) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported image format: %s", path))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		slog.Warn("form file extraction failed", "error", err)
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		slog.Error("read upload file", "error", err)
		return
	}

	asset, err := s.app.AssetService().Create(r.Context(), models.AssetInput{
		VaultID: vaultID,
		Path:    path,
		Data:    data,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload asset")
		slog.Error("upload asset", "error", err)
		return
	}

	vID := models.BareID("vault", vaultID)
	writeJSON(w, http.StatusCreated, AssetMeta{
		VaultID:     vID,
		Path:        asset.Path,
		MimeType:    asset.MimeType,
		Size:        asset.Size,
		ContentHash: asset.ContentHash,
		CreatedAt:   asset.CreatedAt,
		UpdatedAt:   asset.UpdatedAt,
	})
}

func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	path := r.URL.Query().Get("path")

	if vaultID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "vault and path query parameters required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	asset, err := s.app.AssetService().Get(r.Context(), vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get asset")
		slog.Error("get asset", "error", err)
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}

	w.Header().Set("Content-Type", asset.MimeType)
	w.Header().Set("ETag", `"`+asset.ContentHash+`"`)
	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	http.ServeContent(w, r, asset.Path, asset.UpdatedAt, bytes.NewReader(asset.Data))
}

func (s *Server) getAssetMeta(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	path := r.URL.Query().Get("path")

	if vaultID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "vault and path query parameters required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	meta, err := s.app.AssetService().GetMeta(r.Context(), vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get asset metadata")
		slog.Error("get asset meta", "error", err)
		return
	}
	if meta == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}

	vID := models.BareID("vault", vaultID)
	writeJSON(w, http.StatusOK, AssetMeta{
		VaultID:     vID,
		Path:        meta.Path,
		MimeType:    meta.MimeType,
		Size:        meta.Size,
		ContentHash: meta.ContentHash,
		CreatedAt:   meta.CreatedAt,
		UpdatedAt:   meta.UpdatedAt,
	})
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	path := r.URL.Query().Get("path")

	if vaultID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "vault and path query parameters required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Check existence first so we return 404 instead of silent 204
	meta, err := s.app.AssetService().GetMeta(r.Context(), vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check asset")
		slog.Error("delete asset: check existence", "error", err)
		return
	}
	if meta == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}

	if err := s.app.AssetService().Delete(r.Context(), vaultID, path); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete asset")
		slog.Error("delete asset", "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
