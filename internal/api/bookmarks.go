package api

import (
	"net/http"
	"path"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

type bookmarkRequest struct {
	Path string `json:"path"`
}

func (s *Server) listBookmarks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := ac.UserID
	logger := logutil.FromCtx(ctx)

	files, err := s.app.DBClient().ListBookmarks(ctx, userID, vaultID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list bookmarks")
		logger.Error("list bookmarks", "vault_id", vaultID, "user_id", userID, "error", err)
		return
	}

	entries := make([]models.FileEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, models.FileEntry{
			Name:  path.Base(f.Path),
			Path:  f.Path,
			Title: f.Title,
			IsDir: f.IsFolder,
			Size:  f.Size,
		})
	}

	writeJSON(w, http.StatusOK, httputil.NewListResponse(entries, len(entries)))
}

func (s *Server) addBookmark(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := ac.UserID
	logger := logutil.FromCtx(ctx)

	req, ok := decodeBody[bookmarkRequest](w, r, 1024)
	if !ok {
		return
	}
	if req.Path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path is required")
		return
	}

	// Resolve path to file ID
	file, err := s.app.DBClient().GetFileByPath(ctx, vaultID, req.Path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up file")
		logger.Error("add bookmark: get file", "vault_id", vaultID, "user_id", userID, "path", req.Path, "error", err)
		return
	}
	if file == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "file not found")
		return
	}
	fileID, err := models.RecordIDString(file.ID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid file ID")
		logger.Error("add bookmark: extract file ID", "error", err)
		return
	}

	if err := s.app.DBClient().CreateBookmark(ctx, userID, fileID, vaultID); err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create bookmark")
		logger.Error("add bookmark", "vault_id", vaultID, "user_id", userID, "path", req.Path, "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeBookmark(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID := ac.UserID
	logger := logutil.FromCtx(ctx)

	req, ok := decodeBody[bookmarkRequest](w, r, 1024)
	if !ok {
		return
	}
	if req.Path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path is required")
		return
	}

	// Resolve path to file ID
	file, err := s.app.DBClient().GetFileByPath(ctx, vaultID, req.Path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up file")
		logger.Error("remove bookmark: get file", "vault_id", vaultID, "user_id", userID, "path", req.Path, "error", err)
		return
	}
	if file == nil {
		// File doesn't exist — bookmark can't exist either, treat as success
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fileID, err := models.RecordIDString(file.ID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid file ID")
		logger.Error("remove bookmark: extract file ID", "error", err)
		return
	}

	if err := s.app.DBClient().DeleteBookmark(ctx, userID, fileID); err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to remove bookmark")
		logger.Error("remove bookmark", "vault_id", vaultID, "user_id", userID, "path", req.Path, "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
