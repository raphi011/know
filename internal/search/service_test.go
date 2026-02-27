package search

import (
	"testing"

	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func rid(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}

func doc(id, title string) models.Document {
	return models.Document{
		ID:    rid("document", id),
		Title: title,
	}
}

func docWithScore(id, title string, score float64) v2db.DocumentWithScore {
	return v2db.DocumentWithScore{
		Document: doc(id, title),
		Score:    score,
	}
}

func chunkWithScore(docID, content string, score float64) v2db.ChunkWithScore {
	return v2db.ChunkWithScore{
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
	bm25 := []v2db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
		docWithScore("b", "Doc B", 3.0),
	}

	results := rrfFusion(bm25, nil, nil, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Doc A should rank higher (rank 1 in BM25)
	if results[0].Document.Title != "Doc A" {
		t.Errorf("expected Doc A first, got %q", results[0].Document.Title)
	}
}

func TestRRFFusion_VectorOnly(t *testing.T) {
	vector := []v2db.DocumentWithScore{
		docWithScore("x", "Doc X", 0.95),
		docWithScore("y", "Doc Y", 0.80),
	}

	results := rrfFusion(nil, vector, nil, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.Title != "Doc X" {
		t.Errorf("expected Doc X first, got %q", results[0].Document.Title)
	}
}

func TestRRFFusion_HybridBoost(t *testing.T) {
	// Doc A appears in both, Doc B only in BM25, Doc C only in vector
	bm25 := []v2db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
		docWithScore("b", "Doc B", 3.0),
	}
	vector := []v2db.DocumentWithScore{
		docWithScore("c", "Doc C", 0.95),
		docWithScore("a", "Doc A", 0.90),
	}

	results := rrfFusion(bm25, vector, nil, 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Doc A should be first — it appears in both lists and gets boosted
	if results[0].Document.Title != "Doc A" {
		t.Errorf("expected Doc A first (hybrid boost), got %q", results[0].Document.Title)
	}
	// Doc A's score should be higher than any single-source doc
	if results[0].Score <= results[1].Score {
		t.Errorf("hybrid doc should have higher score than single-source docs")
	}
}

func TestRRFFusion_ChunkAttachment(t *testing.T) {
	bm25 := []v2db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}
	chunks := []v2db.ChunkWithScore{
		chunkWithScore("a", "chunk content from A", 0.9),
	}

	results := rrfFusion(bm25, nil, chunks, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 1 {
		t.Fatalf("expected 1 matched chunk, got %d", len(results[0].MatchedChunks))
	}
	if results[0].MatchedChunks[0].Content != "chunk content from A" {
		t.Errorf("expected chunk content, got %q", results[0].MatchedChunks[0].Content)
	}
}

func TestRRFFusion_ChunkOrphan(t *testing.T) {
	// Chunk references a doc that isn't in BM25 or vector results — should be ignored
	bm25 := []v2db.DocumentWithScore{
		docWithScore("a", "Doc A", 5.0),
	}
	chunks := []v2db.ChunkWithScore{
		chunkWithScore("unknown", "orphan chunk", 0.9),
	}

	results := rrfFusion(bm25, nil, chunks, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 0 {
		t.Errorf("orphan chunks should not be attached, got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_Limit(t *testing.T) {
	bm25 := []v2db.DocumentWithScore{
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
