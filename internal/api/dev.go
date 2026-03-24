package api

import (
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) resetDB(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	if err := auth.RequireSystemAdmin(ctx); err != nil {
		logger.Warn("dev access denied", "error", err)
		httputil.WriteProblem(w, http.StatusForbidden, "system admin required")
		return
	}

	dbClient := s.app.DBClient()

	snap, err := dbClient.SnapshotIdentity(ctx)
	if err != nil {
		logger.Error("snapshot identity", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to snapshot identity data")
		return
	}

	logger.Info("identity snapshot complete",
		"users", len(snap.Users),
		"vaults", len(snap.Vaults),
		"vault_members", len(snap.VaultMembers),
		"tokens", len(snap.Tokens),
	)

	embedDim := s.app.Config().EmbedDimension
	if err := dbClient.ResetSchema(ctx, embedDim); err != nil {
		logger.Error("reset schema", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to reset schema")
		return
	}

	if err := dbClient.RestoreIdentity(ctx, snap); err != nil {
		logger.Error("restore identity", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to restore identity data")
		return
	}

	writeJSON(w, http.StatusOK, models.DevResetDBResponse{
		Preserved: models.PreservedCounts{
			Users:        len(snap.Users),
			Vaults:       len(snap.Vaults),
			VaultMembers: len(snap.VaultMembers),
			Tokens:       len(snap.Tokens),
		},
	})
}
