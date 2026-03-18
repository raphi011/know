package api

import (
	"net/http"
	"strconv"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) externalLinkStats(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())

	stats, err := s.app.DBClient().GetExternalLinkStats(r.Context(), vaultID)
	if err != nil {
		logutil.FromCtx(r.Context()).Error("get external link stats", "vault_id", vaultID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get external link stats")
		return
	}

	if stats == nil {
		stats = []db.ExternalLinkHostStats{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"stats": stats})
}

func (s *Server) listExternalLinks(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())

	filter := db.ExternalLinkFilter{
		VaultID: vaultID,
	}

	if h := r.URL.Query().Get("hostname"); h != "" {
		filter.Hostname = &h
	}
	if f := r.URL.Query().Get("file"); f != "" {
		filter.FileID = &f
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			filter.Limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			filter.Offset = v
		}
	}

	logger := logutil.FromCtx(r.Context())

	links, err := s.app.DBClient().ListExternalLinks(r.Context(), filter)
	if err != nil {
		logger.Error("list external links", "vault_id", vaultID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list external links")
		return
	}

	total, err := s.app.DBClient().CountExternalLinks(r.Context(), filter)
	if err != nil {
		logger.Error("count external links", "vault_id", vaultID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to count external links")
		return
	}

	if links == nil {
		links = []models.ExternalLink{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"links": links,
		"total": total,
	})
}
