package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
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

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	doc, err := s.app.DBClient().GetDocumentByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get document")
		logger.Error("get document", "error", err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc))
}

type upsertDocumentRequest struct {
	VaultID string `json:"vaultId"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) upsertDocument(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[upsertDocumentRequest](w, r, 5*1024*1024) // 5 MB
	if !ok {
		return
	}
	if body.VaultID == "" || body.Path == "" {
		writeError(w, http.StatusBadRequest, "vaultId and path are required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(r.Context())

	doc, err := s.app.DocumentService().Create(r.Context(), models.DocumentInput{
		VaultID: body.VaultID,
		Path:    body.Path,
		Content: body.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create/update document")
		logger.Error("upsert document", "error", err)
		return
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc))
}

type deleteDocumentsResponse struct {
	Deleted []string `json:"deleted"`
	Count   int      `json:"count"`
	DryRun  bool     `json:"dryRun"`
}

func (s *Server) deleteDocuments(w http.ResponseWriter, r *http.Request) {
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

	path = models.NormalizePath(path)
	recursive := r.URL.Query().Get("recursive") == "true"
	dryRun := r.URL.Query().Get("dry-run") == "true"

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	// Check if path is a document
	doc, err := s.app.DBClient().GetDocumentByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check document")
		logger.Error("delete documents: get document", "error", err)
		return
	}
	if doc != nil {
		if !dryRun {
			if err := s.app.DocumentService().Delete(ctx, vaultID, path); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to delete document")
				logger.Error("delete document", "path", path, "error", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, deleteDocumentsResponse{
			Deleted: []string{path},
			Count:   1,
			DryRun:  dryRun,
		})
		return
	}

	// Check if path is a folder
	folder, err := s.app.DBClient().GetFolderByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check folder")
		logger.Error("delete documents: get folder", "error", err)
		return
	}
	if folder == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if !recursive {
		writeError(w, http.StatusBadRequest, "path is a directory, use recursive=true")
		return
	}

	// List folder contents for response
	prefix := path + "/"
	metas, err := s.app.DBClient().ListDocumentMetas(ctx, db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  &prefix,
		Limit:   10000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list folder contents")
		logger.Error("delete documents: list metas", "error", err)
		return
	}

	deleted := make([]string, len(metas))
	for i, m := range metas {
		deleted[i] = m.Path
	}

	if !dryRun {
		if err := s.app.VaultService().DeleteFolder(ctx, vaultID, path); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete folder")
			logger.Error("delete folder", "path", path, "error", err)
			return
		}
	}

	writeJSON(w, http.StatusOK, deleteDocumentsResponse{
		Deleted: deleted,
		Count:   len(deleted),
		DryRun:  dryRun,
	})
}

func documentFromModel(d *models.Document) Document {
	id, err := models.RecordIDString(d.ID)
	if err != nil {
		slog.Warn("unexpected document ID format", "path", d.Path, "error", err)
		id = fmt.Sprintf("%v", d.ID.ID)
	}
	vaultID, err := models.RecordIDString(d.Vault)
	if err != nil {
		slog.Warn("unexpected document vault ID format", "path", d.Path, "error", err)
		vaultID = fmt.Sprintf("%v", d.Vault.ID)
	}
	return Document{
		ID:          id,
		VaultID:     vaultID,
		Path:        d.Path,
		Title:       d.Title,
		Content:     d.Content,
		ContentHash: d.ContentHash,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}
