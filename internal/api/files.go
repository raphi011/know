package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	path := r.URL.Query().Get("path")

	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	doc, err := s.app.DBClient().GetFileByPath(ctx, vaultID, path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get document")
		logger.Error("get document", "error", err)
		return
	}
	if doc == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "document not found")
		return
	}

	content, err := s.app.FileService().ReadFileContent(ctx, doc)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to read document content")
		logger.Error("read document content", "error", err)
		return
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		logger.Warn("failed to extract file ID for wiki links", "error", err)
		writeJSON(w, http.StatusOK, documentFromModel(doc, content))
		return
	}

	// Enhance content and get resolved wiki-links in one pass.
	if renderSvc := s.app.RenderService(); renderSvc != nil {
		enhanced, wikiLinks, enhErr := renderSvc.Enhance(ctx, vaultID, fileID, content)
		if enhErr != nil {
			logger.Warn("render enhance failed, serving raw content", "error", enhErr)
		} else {
			content = enhanced
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
			return
		}
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc, content))
}

type upsertDocumentRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) upsertDocument(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[upsertDocumentRequest](w, r, 5*1024*1024) // 5 MB
	if !ok {
		return
	}
	if body.Path == "" {
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

	doc, err := s.app.FileService().Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    body.Path,
		Content: body.Content,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create/update document")
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
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	path := r.URL.Query().Get("path")

	if path == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	path = models.NormalizePath(path)
	recursive := r.URL.Query().Get("recursive") == "true"
	dryRun := r.URL.Query().Get("dry-run") == "true"

	logger := logutil.FromCtx(ctx)

	// Check if path exists (documents and folders are in the same table)
	doc, err := s.app.DBClient().GetFileByPath(ctx, vaultID, path)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to check file")
		logger.Error("delete documents: get file", "error", err)
		return
	}
	if doc == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "not found")
		return
	}

	// Single file (not a folder) — delete directly
	if !doc.IsFolder {
		if !dryRun {
			if err := s.app.FileService().Delete(ctx, vaultID, path); err != nil {
				httputil.WriteProblem(w, http.StatusInternalServerError, "failed to delete document")
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
		httputil.WriteProblem(w, http.StatusBadRequest, "path is a directory, use recursive=true")
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
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list folder contents")
		logger.Error("delete documents: list metas", "error", err)
		return
	}

	deleted := make([]string, len(metas))
	for i, m := range metas {
		deleted[i] = m.Path
	}

	if !dryRun {
		if err := s.app.VaultService().DeleteFolder(ctx, vaultID, path); err != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, "failed to delete folder")
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
	if body.Source == "" || body.Destination == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "source and destination are required")
		return
	}

	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	source := models.NormalizePath(body.Source)
	destination := models.NormalizePath(body.Destination)

	if source == destination {
		httputil.WriteProblem(w, http.StatusBadRequest, "source and destination must be different")
		return
	}

	logger := logutil.FromCtx(ctx)

	// Single lookup — documents and folders are in the same table
	sourceFile, err := s.app.DBClient().GetFileByPath(ctx, vaultID, source)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, fmt.Sprintf("failed to check source: %s", source))
		logger.Error("move: get source", "source", source, "error", err)
		return
	}
	if sourceFile == nil {
		httputil.WriteProblem(w, http.StatusNotFound, fmt.Sprintf("source not found: %s", source))
		return
	}

	// Check for destination conflict (same table — covers both docs and folders)
	existing, err := s.app.DBClient().GetFileByPath(ctx, vaultID, destination)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, fmt.Sprintf("failed to check destination: %s", destination))
		logger.Error("move: check destination", "destination", destination, "error", err)
		return
	}
	if existing != nil {
		if sourceFile.IsFolder != existing.IsFolder {
			if sourceFile.IsFolder {
				httputil.WriteProblem(w, http.StatusConflict, fmt.Sprintf("cannot move folder to existing document path: %s", destination))
			} else {
				httputil.WriteProblem(w, http.StatusConflict, fmt.Sprintf("cannot move document to existing folder path: %s", destination))
			}
		} else {
			httputil.WriteProblem(w, http.StatusConflict, fmt.Sprintf("destination already exists: %s", destination))
		}
		return
	}

	// Document move
	if !sourceFile.IsFolder {
		if !body.DryRun {
			if _, err := s.app.FileService().Move(ctx, vaultID, source, destination); err != nil {
				httputil.WriteProblem(w, http.StatusInternalServerError, fmt.Sprintf("failed to move document: %s", source))
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
		VaultID:  vaultID,
		Folder:   &prefix,
		IsFolder: &isNotFolder,
		Limit:    10000,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, fmt.Sprintf("failed to list folder contents: %s", source))
		logger.Error("move: list metas", "source", source, "error", err)
		return
	}

	moved := make([]string, len(metas))
	for i, m := range metas {
		moved[i] = m.Path
	}

	if !body.DryRun {
		if err := s.app.VaultService().MoveFolder(ctx, vaultID, source, destination); err != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, fmt.Sprintf("failed to move folder: %s", source))
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
		ID:        id,
		VaultID:   vaultID,
		Path:      d.Path,
		Title:     d.Title,
		Content:   content,
		Labels:    nonNilLabels(d.Labels),
		DocType:   d.DocType,
		Hash:      d.Hash,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}
