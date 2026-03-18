package api

import (
	"net/http"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/webclip"
)

type fetchRequest struct {
	URL     string  `json:"url"`
	VaultID string  `json:"vault_id"`
	Path    *string `json:"path,omitempty"`
}

type fetchResponse struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	VaultID string `json:"vault_id"`
}

func (s *Server) fetchWebpage(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[fetchRequest](w, r, 1<<16) // 64KB max
	if !ok {
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	// Resolve vault.
	vaultID := req.VaultID
	if vaultID == "" {
		ac, err := auth.FromContext(ctx)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if len(ac.Vaults) > 0 {
			vaultID = ac.Vaults[0].VaultID
		}
	}
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault_id is required")
		return
	}

	jinaClient := s.app.JinaClient()

	// Load vault settings for web clip path.
	v, err := s.app.DBClient().GetVault(ctx, vaultID)
	if err != nil {
		logger.Warn("fetch: load vault failed", "vault", vaultID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load vault")
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "vault not found")
		return
	}
	settings := v.Defaults()

	result, err := webclip.FetchAndSave(ctx, jinaClient, s.app.FileService(), vaultID, req.URL, req.Path, settings)
	if err != nil {
		logger.Warn("fetch: fetch and save failed", "url", req.URL, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch and save page")
		return
	}

	writeJSON(w, http.StatusOK, fetchResponse{
		Path:    result.Path,
		Title:   result.Title,
		VaultID: vaultID,
	})
}
