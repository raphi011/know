package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/backup"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)

	bareVault := models.BareID("vault", vaultID)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="know-backup-%s.tar.gz"`, bareVault))

	if err := backup.Export(ctx, s.app.DBClient(), s.app.BlobStore(), vaultID, w); err != nil {
		logutil.FromCtx(ctx).Error("backup export failed", "vault", vaultID, "error", err)
		// Headers already sent — can't write error response.
		return
	}
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	authCtx, err := auth.FromContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Limit request body to 1 GB.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	start := time.Now()
	if err := backup.Restore(ctx, s.app.DBClient(), s.app.BlobStore(), s.app.VaultService(), authCtx.UserID, r.Body); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restore failed: %v", err))
		logger.Error("backup restore failed", "error", err)
		return
	}

	logger.Info("backup restore complete", "duration", time.Since(start))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
