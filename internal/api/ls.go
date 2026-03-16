package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) ls(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	folder := r.URL.Query().Get("path")
	if folder == "" {
		folder = "/"
	}
	folder = models.NormalizePath(folder)

	recursive := r.URL.Query().Get("recursive") == "true"

	// Build prefix with trailing slash for filtering (root "/" stays as "/").
	prefix := folder
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	logger := logutil.FromCtx(r.Context())

	// Pass prefix (with trailing slash) to the DB so that "/docs" doesn't match "/docs-other".
	// For root ("/"), starts_with(path, "/") correctly matches all documents.
	// Exclude folders — they are listed separately via ListChildFolders / ListFolders.
	isNotFolder := false
	docs, err := s.app.DBClient().ListFileMetas(r.Context(), db.ListFilesFilter{
		VaultID:  vaultID,
		Folder:   &prefix,
		IsFolder: &isNotFolder,
		Limit:    10000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		logger.Error("ls documents", "vault_id", vaultID, "path", folder, "error", err)
		return
	}

	var entries []models.FileEntry

	if recursive {
		// Recursive: return all folders and documents under this path.
		// ListFolders(nil) returns all vault folders; filter to those under the prefix.
		allFolders, err := s.app.VaultService().ListFolders(r.Context(), vaultID, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list folders")
			logger.Error("ls folders", "vault_id", vaultID, "path", folder, "error", err)
			return
		}
		for _, f := range allFolders {
			if strings.HasPrefix(f.Path, prefix) {
				entries = append(entries, models.FileEntry{
					Name:  path.Base(f.Path),
					Path:  f.Path,
					IsDir: true,
				})
			}
		}
		for _, d := range docs {
			entries = append(entries, models.FileEntry{
				Name:  path.Base(d.Path),
				Path:  d.Path,
				Title: d.Title,
				Size:  d.Size,
			})
		}
	} else {
		// Non-recursive: only direct children
		childFolders, err := s.app.DBClient().ListChildFolders(r.Context(), vaultID, folder)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list folders")
			logger.Error("ls child folders", "vault_id", vaultID, "path", folder, "error", err)
			return
		}
		for _, f := range childFolders {
			entries = append(entries, models.FileEntry{
				Name:  path.Base(f.Path),
				Path:  f.Path,
				IsDir: true,
			})
		}

		// Direct child documents (path after prefix has no further "/")
		for _, d := range docs {
			rel := strings.TrimPrefix(d.Path, prefix)
			if rel != d.Path && !strings.Contains(rel, "/") {
				entries = append(entries, models.FileEntry{
					Name:  path.Base(d.Path),
					Path:  d.Path,
					Title: d.Title,
					Size:  d.Size,
				})
			}
		}
	}

	if entries == nil {
		entries = []models.FileEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}
