package api

import (
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
)

func (s *Server) listLabels(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())

	logger := logutil.FromCtx(r.Context())

	if r.URL.Query().Get("counts") == "true" {
		counts, err := s.app.DBClient().ListLabelsWithCounts(r.Context(), vaultID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list labels")
			logger.Error("list labels with counts", "vault_id", vaultID, "error", err)
			return
		}
		writeJSON(w, http.StatusOK, counts)
		return
	}

	labels, err := s.app.DBClient().ListLabels(r.Context(), vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list labels")
		logger.Error("list labels", "vault_id", vaultID, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, labels)
}
