package api

import (
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
)

func (s *Server) listLabels(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if r.URL.Query().Get("counts") == "true" {
		counts, err := s.app.DBClient().ListLabelsWithCounts(r.Context(), vaultID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list labels")
			slog.Error("list labels with counts", "vault_id", vaultID, "error", err)
			return
		}
		writeJSON(w, http.StatusOK, counts)
		return
	}

	labels, err := s.app.DBClient().ListLabels(r.Context(), vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list labels")
		slog.Error("list labels", "vault_id", vaultID, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, labels)
}
