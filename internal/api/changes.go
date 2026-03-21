package api

import (
	"net/http"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) getChanges(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "since query parameter required (RFC3339)")
		return
	}

	since, err := time.Parse(time.RFC3339Nano, sinceStr)
	if err != nil {
		httputil.WriteProblem(w, http.StatusBadRequest, "invalid since timestamp: expected RFC3339 format")
		return
	}

	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	logger := logutil.FromCtx(ctx)

	// Capture sync token before queries to avoid missing concurrent changes.
	syncToken := time.Now().UTC()

	// Fetch files updated since the given timestamp.
	notFolder := false
	files, err := s.app.DBClient().ListFiles(ctx, db.ListFilesFilter{
		VaultID:      vaultID,
		IsFolder:     &notFolder,
		UpdatedSince: &since,
		OrderBy:      db.OrderByUpdatedAtAsc,
		Limit:        10000,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list changed files")
		logger.Error("get changes: list files", "vault_id", vaultID, "since", since, "error", err)
		return
	}

	updated := make([]FileChange, 0, len(files))
	for _, f := range files {
		fileID, idErr := models.RecordIDString(f.ID)
		if idErr != nil {
			logger.Error("get changes: extract file ID", "vault_id", vaultID, "path", f.Path, "error", idErr)
			httputil.WriteProblem(w, http.StatusInternalServerError, "internal error processing changed files")
			return
		}
		updated = append(updated, FileChange{
			FileID:    fileID,
			Path:      f.Path,
			Hash:      f.Hash,
			UpdatedAt: f.UpdatedAt,
		})
	}

	// Fetch tombstones (deleted files) since the given timestamp.
	tombstones, err := s.app.DBClient().ListTombstonesSince(ctx, vaultID, since)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list deleted files")
		logger.Error("get changes: list tombstones", "vault_id", vaultID, "since", since, "error", err)
		return
	}

	deleted := make([]FileChange, 0, len(tombstones))
	for _, t := range tombstones {
		deleted = append(deleted, FileChange{
			FileID:    t.FileID,
			Path:      t.Path,
			UpdatedAt: t.DeletedAt,
		})
	}

	const maxResults = 10000
	truncated := len(files) >= maxResults || len(tombstones) >= maxResults

	writeJSON(w, http.StatusOK, ChangesResponse{
		Updated:   updated,
		Deleted:   deleted,
		SyncToken: syncToken.Format(time.RFC3339Nano),
		Truncated: truncated,
	})
}
