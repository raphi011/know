package search

import (
	"strings"
	"testing"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func rid(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}

func doc(id, title string) models.Document {
	return models.Document{
		ID:    rid("document", id),
		Title: title,
		Path:  "docs/" + id + ".md",
	}
}

func docWithScore(id, title string, score float64) db.DocumentWithScore {
	return db.DocumentWithScore{
		Document: doc(id, title),
		Score:    score,
	}
}

func chunkWithScore(docID, content string, score float64) db.ChunkWithScore {
	return db.ChunkWithScore{
		Chunk: models.Chunk{
			ID:       rid("chunk", docID+"-0"),
			Document: rid("document", docID),
			Content:  content,
			Position: 0,
		},
		Score: score,
	}
}

func TestRRFFusion_BM25Only(t *testing.T) {
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
		docWithScore("b", "Doc B", 3.0),
	}

	results := rrfFusion(bm25, nil, nil, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Doc A should rank higher (rank 1 in BM25)
	if results[0].Title != "Doc A" {
		t.Errorf("expected Doc A first, got %q", results[0].Title)
	}
	if results[0].DocumentID != "a" {
		t.Errorf("expected DocumentID 'a', got %q", results[0].DocumentID)
	}
}

func TestRRFFusion_ChunkPromotion(t *testing.T) {
	// Chunks promote their parent documents even without BM25 hits
	chunks := []db.ChunkWithScore{
		chunkWithScore("x", "Doc X content", 0.95),
		chunkWithScore("y", "Doc Y content", 0.80),
	}
	chunkDocs := map[string]models.Document{
		"x": doc("x", "Doc X"),
		"y": doc("y", "Doc Y"),
	}

	results := rrfFusion(nil, chunks, chunkDocs, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Doc X" {
		t.Errorf("expected Doc X first, got %q", results[0].Title)
	}
	if len(results[0].MatchedChunks) != 1 {
		t.Fatalf("expected 1 matched chunk on Doc X, got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_HybridBoost(t *testing.T) {
	// Doc A appears in both BM25 and chunks — should get boosted
	// Doc B only in BM25, Doc C only via chunk
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
		docWithScore("b", "Doc B", 3.0),
	}
	chunks := []db.ChunkWithScore{
		chunkWithScore("c", "Doc C chunk", 0.95),
		chunkWithScore("a", "Doc A chunk", 0.90),
	}
	chunkDocs := map[string]models.Document{
		"c": doc("c", "Doc C"),
	}

	results := rrfFusion(bm25, chunks, chunkDocs, 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Doc A should be first — it appears in both lists and gets boosted
	if results[0].Title != "Doc A" {
		t.Errorf("expected Doc A first (hybrid boost), got %q", results[0].Title)
	}
	// Doc A's score should be higher than any single-source doc
	if results[0].Score <= results[1].Score {
		t.Errorf("hybrid doc should have higher score than single-source docs")
	}
}

func TestRRFFusion_ChunkAttachment(t *testing.T) {
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}
	chunks := []db.ChunkWithScore{
		chunkWithScore("a", "chunk content from A", 0.9),
	}

	results := rrfFusion(bm25, chunks, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 1 {
		t.Fatalf("expected 1 matched chunk, got %d", len(results[0].MatchedChunks))
	}
	if results[0].MatchedChunks[0].Snippet != "chunk content from A" {
		t.Errorf("expected chunk snippet, got %q", results[0].MatchedChunks[0].Snippet)
	}
}

func TestRRFFusion_ChunkOrphan(t *testing.T) {
	// Chunk references a doc not in BM25 and not in chunkDocs — should be ignored
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}
	chunks := []db.ChunkWithScore{
		chunkWithScore("unknown", "orphan chunk", 0.9),
	}

	results := rrfFusion(bm25, chunks, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 0 {
		t.Errorf("orphan chunks should not be attached, got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_Limit(t *testing.T) {
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
		docWithScore("b", "Doc B", 4.0),
		docWithScore("c", "Doc C", 3.0),
		docWithScore("d", "Doc D", 2.0),
		docWithScore("e", "Doc E", 1.0),
	}

	results := rrfFusion(bm25, nil, nil, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

func TestRRFFusion_Empty(t *testing.T) {
	results := rrfFusion(nil, nil, nil, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty inputs, got %d", len(results))
	}
}

func TestRRFFusion_LightweightFields(t *testing.T) {
	bm25 := []db.DocumentWithScore{
		{
			Document: models.Document{
				ID:     rid("document", "abc"),
				Title:  "My Doc",
				Path:   "notes/my-doc.md",
				Labels: []string{"go", "test"},
			},
			Score: 5.0,
		},
	}

	results := rrfFusion(bm25, nil, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.DocumentID != "abc" {
		t.Errorf("expected DocumentID 'abc', got %q", r.DocumentID)
	}
	if r.Path != "notes/my-doc.md" {
		t.Errorf("expected path 'notes/my-doc.md', got %q", r.Path)
	}
	if r.Title != "My Doc" {
		t.Errorf("expected title 'My Doc', got %q", r.Title)
	}
	if len(r.Labels) != 2 || r.Labels[0] != "go" {
		t.Errorf("expected labels [go, test], got %v", r.Labels)
	}
}

func TestTruncateSnippet(t *testing.T) {
	short := "hello world"
	if got := truncateSnippet(short, 200); got != short {
		t.Errorf("short string should not be truncated, got %q", got)
	}

	// Build a long string
	var long strings.Builder
	for range 50 {
		long.WriteString("word ")
	}
	got := truncateSnippet(long.String(), 50)
	if len(got) > 55 { // 50 + ellipsis + tolerance
		t.Errorf("expected truncated string, got length %d", len(got))
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("expected ellipsis suffix, got %q", got[len(got)-5:])
	}
}

func TestRRFFusion_SnippetTruncation(t *testing.T) {
	// Build chunk content longer than maxSnippetLen
	var longContent strings.Builder
	for range 100 {
		longContent.WriteString("word ")
	}

	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}
	chunks := []db.ChunkWithScore{
		chunkWithScore("a", longContent.String(), 0.9),
	}

	results := rrfFusion(bm25, chunks, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	snippet := results[0].MatchedChunks[0].Snippet
	if len(snippet) > maxSnippetLen+5 { // +5 for ellipsis char
		t.Errorf("snippet should be truncated, got length %d", len(snippet))
	}
}

func TestRRFFusion_NilMatchedChunks(t *testing.T) {
	// BM25-only results should have non-nil empty MatchedChunks
	bm25 := []db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}

	results := rrfFusion(bm25, nil, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MatchedChunks == nil {
		t.Error("MatchedChunks should be empty slice, not nil")
	}
	if len(results[0].MatchedChunks) != 0 {
		t.Errorf("expected 0 matched chunks, got %d", len(results[0].MatchedChunks))
	}
}
