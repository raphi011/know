package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type SearchFilter struct {
	VaultID string
	Labels  []string
	DocType *string
	Folder  *string
	Limit   int
}

type ChunkWithScore struct {
	models.Chunk
	Score     float64  `json:"score"`
	DocPath   string   `json:"doc_path"`
	DocTitle  string   `json:"doc_title"`
	DocLabels []string `json:"doc_labels"`
	DocType   *string  `json:"doc_type"`
}

// buildChunkFilterConditions returns WHERE conditions and query variables
// for filtering chunks by their parent document's vault, labels, doc_type and folder.
func buildChunkFilterConditions(filter SearchFilter) ([]string, map[string]any) {
	var conditions []string
	vars := map[string]any{
		"vault_id": bareID("vault", filter.VaultID),
	}

	conditions = append(conditions, `document.vault = type::record("vault", $vault_id)`)

	if len(filter.Labels) > 0 {
		conditions = append(conditions, `document.labels CONTAINSANY $labels`)
		vars["labels"] = filter.Labels
	}
	if filter.DocType != nil {
		conditions = append(conditions, `document.doc_type = $doc_type`)
		vars["doc_type"] = *filter.DocType
	}
	if filter.Folder != nil {
		conditions = append(conditions, `string::starts_with(document.path, $folder)`)
		vars["folder"] = *filter.Folder
	}

	return conditions, vars
}

// BM25ChunkSearch performs fulltext search on chunk content via BM25,
// filtering by the parent document's vault, labels, doc_type and folder.
func (c *Client) BM25ChunkSearch(ctx context.Context, query string, filter SearchFilter) ([]ChunkWithScore, error) {
	defer c.logOp(ctx, "search.bm25", time.Now())
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	conditions, vars := buildChunkFilterConditions(filter)
	vars["query"] = query

	sql := fmt.Sprintf(`
		SELECT *,
			search::score(1) AS score,
			document.path AS doc_path,
			document.title AS doc_title,
			document.labels AS doc_labels,
			document.doc_type AS doc_type
		FROM chunk
		WHERE %s
			AND content @1@ $query
		ORDER BY score DESC
		LIMIT %d
	`, strings.Join(conditions, " AND "), limit)

	results, err := surrealdb.Query[[]ChunkWithScore](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("bm25 chunk search: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// HybridSearch performs hybrid BM25+vector search using SurrealDB's search::rrf()
// to fuse both result sets in a single query. Returns chunks ranked by RRF score
// with parent document metadata included via record link traversal.
func (c *Client) HybridSearch(ctx context.Context, query string, embedding []float32, filter SearchFilter) ([]ChunkWithScore, error) {
	defer c.logOp(ctx, "search.hybrid", time.Now())
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	conditions, vars := buildChunkFilterConditions(filter)
	vars["query"] = query
	vars["embedding"] = embedding

	whereClause := strings.Join(conditions, " AND ")

	// LET variables don't work with search::rrf() (return None) — subqueries must be inlined.
	// ORDER BY is omitted: BM25 @1@ returns results in relevance order, and <|K,EF|> returns
	// K nearest neighbors in similarity order. search::rrf() uses these orderings for rank fusion.
	sql := fmt.Sprintf(`
		SELECT *,
			document.path AS doc_path,
			document.title AS doc_title,
			document.labels AS doc_labels,
			document.doc_type AS doc_type
		FROM search::rrf([
			(SELECT * FROM chunk
			 WHERE %s
			   AND content @1@ $query
			 LIMIT %d),
			(SELECT * FROM chunk
			 WHERE %s
			   AND embedding <|%d,40|> $embedding
			 LIMIT %d)
		], %d, 60)
	`, whereClause, limit, whereClause, limit, limit, limit)

	results, err := surrealdb.Query[[]ChunkWithScore](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// GetDocumentsByIDs fetches multiple documents by ID in a single query.
func (c *Client) GetDocumentsByIDs(ctx context.Context, ids []string) ([]models.Document, error) {
	defer c.logOp(ctx, "search.get_docs_by_ids", time.Now())
	if len(ids) == 0 {
		return nil, nil
	}

	sql := `SELECT * FROM document WHERE id INSIDE $ids`
	recordIDs := make([]surrealmodels.RecordID, len(ids))
	for i, id := range ids {
		recordIDs[i] = newRecordID("document", bareID("document", id))
	}

	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"ids": recordIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("get documents by ids: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}
