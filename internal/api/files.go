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

	doc, err := s.app.DBClient().GetFileByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get document")
		logger.Error("get document", "error", err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	content, err := s.app.FileService().ReadFileContent(ctx, doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read document content")
		logger.Error("read document content", "error", err)
		return
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		logger.Warn("failed to extract file ID for wiki links", "error", err)
		writeJSON(w, http.StatusOK, documentFromModel(doc, content))
		return
	}

	// Enhance content: resolve wiki-links and execute query blocks.
	if renderSvc := s.app.RenderService(); renderSvc != nil {
		enhanced, err := renderSvc.Enhance(ctx, vaultID, fileID, content)
		if err != nil {
			logger.Warn("render enhance failed, serving raw content", "error", err)
		} else {
			content = enhanced
		}
	}

	wikiLinks, err := s.app.DBClient().GetWikiLinksWithTargetInfo(ctx, fileID)
	if err != nil {
		logger.Warn("failed to get wiki links", "error", err)
	}

	resp := documentFromModel(doc, content)
	if len(wikiLinks) > 0 {
		resp.WikiLinks = make([]WikiLinkInfo, len(wikiLinks))
		for i, wl := range wikiLinks {
			resp.WikiLinks[i] = WikiLinkInfo{
				RawTarget: wl.RawTarget,
				Path:      wl.Path,
				Title:     wl.Title,
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
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

	doc, err := s.app.FileService().Create(r.Context(), models.FileInput{
		VaultID: body.VaultID,
		Path:    body.Path,
		Content: body.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create/update document")
		logger.Error("upsert document", "error", err)
		return
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc, body.Content))
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

	// Check if path exists (documents and folders are in the same table)
	doc, err := s.app.DBClient().GetFileByPath(ctx, vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check file")
		logger.Error("delete documents: get file", "error", err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Single file (not a folder) — delete directly
	if !doc.IsFolder {
		if !dryRun {
			if err := s.app.FileService().Delete(ctx, vaultID, path); err != nil {
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

	if !recursive {
		writeError(w, http.StatusBadRequest, "path is a directory, use recursive=true")
		return
	}

	// List folder contents for response (non-folder files only)
	prefix := path + "/"
	isNotFolder := false
	metas, err := s.app.DBClient().ListFileMetas(ctx, db.ListFilesFilter{
		VaultID:  vaultID,
		Folder:   &prefix,
		IsFolder: &isNotFolder,
		Limit:    10000,
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

type moveRequest struct {
	VaultID     string `json:"vaultId"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	DryRun      bool   `json:"dryRun"`
}

type moveResponse struct {
	Type   string   `json:"type"` // "document" or "folder"
	Moved  []string `json:"moved"`
	Count  int      `json:"count"`
	DryRun bool     `json:"dryRun"`
}

func (s *Server) move(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[moveRequest](w, r, 1*1024*1024)
	if !ok {
		return
	}
	if body.VaultID == "" || body.Source == "" || body.Destination == "" {
		writeError(w, http.StatusBadRequest, "vaultId, source, and destination are required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	source := models.NormalizePath(body.Source)
	destination := models.NormalizePath(body.Destination)

	if source == destination {
		writeError(w, http.StatusBadRequest, "source and destination must be different")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	// Single lookup — documents and folders are in the same table
	sourceFile, err := s.app.DBClient().GetFileByPath(ctx, body.VaultID, source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to check source: %s", source))
		logger.Error("move: get source", "source", source, "error", err)
		return
	}
	if sourceFile == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("source not found: %s", source))
		return
	}

	// Check for destination conflict (same table — covers both docs and folders)
	existing, err := s.app.DBClient().GetFileByPath(ctx, body.VaultID, destination)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to check destination: %s", destination))
		logger.Error("move: check destination", "destination", destination, "error", err)
		return
	}
	if existing != nil {
		if sourceFile.IsFolder != existing.IsFolder {
			if sourceFile.IsFolder {
				writeError(w, http.StatusConflict, fmt.Sprintf("cannot move folder to existing document path: %s", destination))
			} else {
				writeError(w, http.StatusConflict, fmt.Sprintf("cannot move document to existing folder path: %s", destination))
			}
		} else {
			writeError(w, http.StatusConflict, fmt.Sprintf("destination already exists: %s", destination))
		}
		return
	}

	// Document move
	if !sourceFile.IsFolder {
		if !body.DryRun {
			if _, err := s.app.FileService().Move(ctx, body.VaultID, source, destination); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to move document: %s", source))
				logger.Error("move document", "source", source, "destination", destination, "error", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, moveResponse{
			Type:   "document",
			Moved:  []string{source},
			Count:  1,
			DryRun: body.DryRun,
		})
		return
	}

	// Folder move — list contents for response
	prefix := source + "/"
	isNotFolder := false
	metas, err := s.app.DBClient().ListFileMetas(ctx, db.ListFilesFilter{
		VaultID:  body.VaultID,
		Folder:   &prefix,
		IsFolder: &isNotFolder,
		Limit:    10000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list folder contents: %s", source))
		logger.Error("move: list metas", "source", source, "error", err)
		return
	}

	moved := make([]string, len(metas))
	for i, m := range metas {
		moved[i] = m.Path
	}

	if !body.DryRun {
		if err := s.app.VaultService().MoveFolder(ctx, body.VaultID, source, destination); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to move folder: %s", source))
			logger.Error("move folder", "source", source, "destination", destination, "error", err)
			return
		}
	}

	writeJSON(w, http.StatusOK, moveResponse{
		Type:   "folder",
		Moved:  moved,
		Count:  len(moved),
		DryRun: body.DryRun,
	})
}

func documentFromModel(d *models.File, content string) Document {
	id, err := models.RecordIDString(d.ID)
	if err != nil {
		slog.Warn("unexpected file ID format", "path", d.Path, "error", err)
		id = fmt.Sprintf("%v", d.ID.ID)
	}
	vaultID, err := models.RecordIDString(d.Vault)
	if err != nil {
		slog.Warn("unexpected file vault ID format", "path", d.Path, "error", err)
		vaultID = fmt.Sprintf("%v", d.Vault.ID)
	}
	return Document{
		ID:          id,
		VaultID:     vaultID,
		Path:        d.Path,
		Title:       d.Title,
		Content:     content,
		Labels:      nonNilLabels(d.Labels),
		DocType:     d.DocType,
		ContentHash: d.ContentHash,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}
