package api

import (
	"net/http"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/webclip"
)

type fetchRequest struct {
	URL  string  `json:"url"`
	Path *string `json:"path,omitempty"`
}

type fetchResponse struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func (s *Server) fetchWebpage(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[fetchRequest](w, r, 1<<16) // 64KB max
	if !ok {
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "url is required")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	vaultID := auth.MustVaultIDFromCtx(ctx)

	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	jinaClient := s.app.JinaClient()

	// Load vault settings for web clip path.
	v, err := s.app.DBClient().GetVault(ctx, vaultID)
	if err != nil {
		logger.Warn("fetch: load vault failed", "vault", vaultID, "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to load vault")
		return
	}
	if v == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "vault not found")
		return
	}
	settings := v.Defaults()

	result, err := webclip.FetchAndSave(ctx, jinaClient, s.app.FileService(), vaultID, req.URL, req.Path, settings)
	if err != nil {
		logger.Warn("fetch: fetch and save failed", "url", req.URL, "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to fetch and save page")
		return
	}

	writeJSON(w, http.StatusOK, fetchResponse{
		Path:  result.Path,
		Title: result.Title,
	})
}
