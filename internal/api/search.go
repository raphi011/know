package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/search"
)

func (s *Server) searchDocuments(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	query := r.URL.Query().Get("query")

	if vaultID == "" || query == "" {
		writeError(w, http.StatusBadRequest, "vault and query parameters required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var labels []string
	if l := r.URL.Query().Get("labels"); l != "" {
		labels = strings.Split(l, ",")
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	results, err := s.app.SearchService().Search(ctx, search.SearchInput{
		VaultID: vaultID,
		Query:   query,
		Labels:  labels,
		Limit:   limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		logger.Error("search documents", "vault_id", vaultID, "error", err)
		return
	}

	resp := make([]SearchResultResponse, len(results))
	for i, r := range results {
		chunks := make([]ChunkMatchResponse, len(r.MatchedChunks))
		for j, c := range r.MatchedChunks {
			chunks[j] = ChunkMatchResponse{
				Snippet:     c.Snippet,
				HeadingPath: c.HeadingPath,
				Position:    c.Position,
				Score:       c.Score,
			}
		}
		resp[i] = SearchResultResponse{
			Path:          r.Path,
			Title:         r.Title,
			Score:         r.Score,
			MatchedChunks: chunks,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
