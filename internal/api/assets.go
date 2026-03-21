package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	logger := logutil.FromCtx(r.Context())

	// 32 MB max memory for multipart parsing
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		logger.Warn("parse multipart form", "error", err)
		httputil.WriteProblem(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.FormValue("path")
	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path field is required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	if !models.IsImageFile(path) {
		httputil.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("unsupported image format: %s", path))
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		logger.Warn("form file extraction failed", "error", err)
		httputil.WriteProblem(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to read file")
		logger.Error("read upload file", "error", err)
		return
	}

	asset, err := s.app.FileService().Create(r.Context(), models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Data:    data,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to upload asset")
		logger.Error("upload asset", "error", err)
		return
	}

	vID := models.BareID("vault", vaultID)
	writeJSON(w, http.StatusCreated, AssetMeta{
		VaultID:   vID,
		Path:      asset.Path,
		MimeType:  asset.MimeType,
		Size:      asset.Size,
		Hash:      asset.Hash,
		UpdatedAt: asset.UpdatedAt,
	})
}

func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.URL.Query().Get("path")

	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	asset, err := s.app.FileService().Get(r.Context(), vaultID, path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get asset")
		logutil.FromCtx(r.Context()).Error("get asset", "error", err)
		return
	}
	if asset == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "asset not found")
		return
	}

	if asset.Hash == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "asset has no content")
		return
	}

	rc, err := s.app.BlobStore().Get(r.Context(), *asset.Hash)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to read asset data")
		logutil.FromCtx(r.Context()).Error("get asset blob", "hash", *asset.Hash, "error", err)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to read asset data")
		logutil.FromCtx(r.Context()).Error("read asset blob", "error", err)
		return
	}
	w.Header().Set("Content-Type", asset.MimeType)
	w.Header().Set("ETag", `"`+*asset.Hash+`"`)
	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
	http.ServeContent(w, r, asset.Path, asset.UpdatedAt, bytes.NewReader(data))
}

func (s *Server) getAssetMeta(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.URL.Query().Get("path")

	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	meta, err := s.app.FileService().GetMeta(r.Context(), vaultID, path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get asset metadata")
		logutil.FromCtx(r.Context()).Error("get asset meta", "error", err)
		return
	}
	if meta == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "asset not found")
		return
	}

	vID := models.BareID("vault", vaultID)
	writeJSON(w, http.StatusOK, AssetMeta{
		VaultID:   vID,
		Path:      meta.Path,
		MimeType:  meta.MimeType,
		Size:      meta.Size,
		Hash:      meta.Hash,
		UpdatedAt: meta.UpdatedAt,
	})
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.URL.Query().Get("path")

	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(r.Context())

	// Check existence first so we return 404 instead of silent 204
	meta, err := s.app.FileService().GetMeta(r.Context(), vaultID, path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to check asset")
		logger.Error("delete asset: check existence", "error", err)
		return
	}
	if meta == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "asset not found")
		return
	}

	if err := s.app.FileService().Delete(r.Context(), vaultID, path); err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to delete asset")
		logger.Error("delete asset", "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
