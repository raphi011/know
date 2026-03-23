package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/search"
)

func (s *Server) searchDocuments(w http.ResponseWriter, r *http.Request) {
	vaultID := auth.MustVaultIDFromCtx(r.Context())
	query := r.URL.Query().Get("query")

	if query == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "query parameter required")
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

	bm25Only := r.URL.Query().Get("bm25_only") == "true"

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	results, err := s.app.SearchService().Search(ctx, search.SearchInput{
		VaultID:  vaultID,
		Query:    query,
		Labels:   labels,
		Limit:    limit,
		BM25Only: bm25Only,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "search failed")
		logger.Error("search documents", "vault_id", vaultID, "error", err)
		return
	}

	resp := make([]SearchResultResponse, len(results))
	for i, res := range results {
		chunks := make([]ChunkMatchResponse, len(res.MatchedChunks))
		for j, c := range res.MatchedChunks {
			chunks[j] = ChunkMatchResponse{
				Snippet:     c.Snippet,
				HeadingPath: c.HeadingPath,
				Position:    c.Position,
				Score:       c.Score,
			}
		}
		resp[i] = SearchResultResponse{
			DocumentID:    res.DocumentID,
			Path:          res.Path,
			Title:         res.Title,
			Labels:        nonNilLabels(res.Labels),
			DocType:       res.DocType,
			Score:         res.Score,
			MatchedChunks: chunks,
		}
	}

	writeJSON(w, http.StatusOK, httputil.NewListResponse(resp, len(resp)))
}
