package search

import (
	"fmt"
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

func chunkWithScorePos(docID, content string, position int, score float64) db.ChunkWithScore {
	return db.ChunkWithScore{
		Chunk: models.Chunk{
			ID:       rid("chunk", fmt.Sprintf("%s-%d", docID, position)),
			Document: rid("document", docID),
			Content:  content,
			Position: position,
		},
		Score: score,
	}
}

func chunkWithHeading(docID, content string, heading string, score float64) db.ChunkWithScore {
	cs := chunkWithScore(docID, content, score)
	cs.Chunk.HeadingPath = &heading
	return cs
}

// docMap builds a map of document ID → Document for use in tests.
// Panics on invalid record IDs (test-only helper; IDs are always constructed via rid()).
func docMap(docs ...models.Document) map[string]models.Document {
	m := make(map[string]models.Document, len(docs))
	for _, d := range docs {
		id, err := models.RecordIDString(d.ID)
		if err != nil {
			panic(fmt.Sprintf("docMap: invalid record ID: %v", err))
		}
		m[id] = d
	}
	return m
}

// =============================================================================
// RRF FUSION TESTS
// =============================================================================

func TestRRFFusion_BM25Only(t *testing.T) {
	bm25 := []db.ChunkWithScore{
		chunkWithScore("a", "Doc A content", 5.0),
		chunkWithScore("b", "Doc B content", 3.0),
	}
	dm := docMap(doc("a", "Doc A"), doc("b", "Doc B"))

	results := rrfFusion(bm25, nil, dm, 10)
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
	// BM25 chunks now carry matched chunks
	if len(results[0].MatchedChunks) != 1 {
		t.Errorf("expected 1 matched chunk on Doc A, got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_ChunkPromotion(t *testing.T) {
	// Vector chunks promote their parent documents even without BM25 hits
	chunks := []db.ChunkWithScore{
		chunkWithScore("x", "Doc X content", 0.95),
		chunkWithScore("y", "Doc Y content", 0.80),
	}
	dm := docMap(doc("x", "Doc X"), doc("y", "Doc Y"))

	results := rrfFusion(nil, chunks, dm, 10)
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
	// Doc A has different chunks in BM25 and vector — both should appear, doc gets score boost
	// Doc B only in BM25, Doc C only via vector
	bm25 := []db.ChunkWithScore{
		chunkWithScorePos("a", "Doc A bm25", 0, 5.0),
		chunkWithScore("b", "Doc B bm25", 3.0),
	}
	vector := []db.ChunkWithScore{
		chunkWithScore("c", "Doc C vector", 0.95),
		chunkWithScorePos("a", "Doc A vector", 1, 0.90),
	}
	dm := docMap(doc("a", "Doc A"), doc("b", "Doc B"), doc("c", "Doc C"))

	results := rrfFusion(bm25, vector, dm, 10)
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
	// Doc A should have 2 matched chunks (different chunks from BM25 and vector)
	if len(results[0].MatchedChunks) != 2 {
		t.Errorf("expected 2 matched chunks on Doc A, got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_ChunkAttachment(t *testing.T) {
	// Different chunks from same doc in BM25 and vector
	bm25 := []db.ChunkWithScore{
		chunkWithScorePos("a", "bm25 chunk from A", 0, 5.0),
	}
	vector := []db.ChunkWithScore{
		chunkWithScorePos("a", "vector chunk from A", 1, 0.9),
	}
	dm := docMap(doc("a", "Doc A"))

	results := rrfFusion(bm25, vector, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 2 {
		t.Fatalf("expected 2 matched chunks (different positions), got %d", len(results[0].MatchedChunks))
	}
}

func TestRRFFusion_ChunkDedup(t *testing.T) {
	// Same chunk (same ID) appears in both BM25 and vector — should be deduped
	// but the document still gets RRF score boost from both
	sameChunk := chunkWithScore("a", "same content", 5.0)
	dm := docMap(doc("a", "Doc A"))

	results := rrfFusion(
		[]db.ChunkWithScore{sameChunk},
		[]db.ChunkWithScore{sameChunk},
		dm, 10,
	)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Only 1 matched chunk despite appearing in both lists
	if len(results[0].MatchedChunks) != 1 {
		t.Errorf("expected 1 matched chunk (deduped), got %d", len(results[0].MatchedChunks))
	}
	// But score should reflect both contributions (higher than single path)
	singleResults := rrfFusion([]db.ChunkWithScore{sameChunk}, nil, dm, 10)
	if results[0].Score <= singleResults[0].Score {
		t.Error("deduped chunk should still boost document RRF score from both paths")
	}
}

func TestRRFFusion_ChunkOrphan(t *testing.T) {
	// Chunk references a doc not in docMap — should be ignored
	bm25 := []db.ChunkWithScore{
		chunkWithScore("a", "Doc A content", 5.0),
	}
	vector := []db.ChunkWithScore{
		chunkWithScore("unknown", "orphan chunk", 0.9),
	}
	dm := docMap(doc("a", "Doc A"))

	results := rrfFusion(bm25, vector, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestRRFFusion_Limit(t *testing.T) {
	bm25 := []db.ChunkWithScore{
		chunkWithScore("a", "Doc A", 5.0),
		chunkWithScore("b", "Doc B", 4.0),
		chunkWithScore("c", "Doc C", 3.0),
		chunkWithScore("d", "Doc D", 2.0),
		chunkWithScore("e", "Doc E", 1.0),
	}
	dm := docMap(doc("a", "Doc A"), doc("b", "Doc B"), doc("c", "Doc C"), doc("d", "Doc D"), doc("e", "Doc E"))

	results := rrfFusion(bm25, nil, dm, 3)
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
	bm25 := []db.ChunkWithScore{
		{
			Chunk: models.Chunk{
				ID:       rid("chunk", "abc-0"),
				Document: rid("document", "abc"),
				Content:  "chunk content",
				Position: 0,
			},
			Score: 5.0,
		},
	}
	dm := docMap(models.Document{
		ID:     rid("document", "abc"),
		Title:  "My Doc",
		Path:   "notes/my-doc.md",
		Labels: []string{"go", "test"},
	})

	results := rrfFusion(bm25, nil, dm, 10)
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
	if len([]rune(got)) > 55 { // 50 + ellipsis + tolerance
		t.Errorf("expected truncated string, got rune length %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestTruncateSnippet_MultiByte(t *testing.T) {
	// Verify truncation doesn't break multi-byte characters
	// 10 German words with umlauts = more bytes than runes
	input := strings.Repeat("übung ", 50) // 300 runes, ~400 bytes
	got := truncateSnippet(input, 50)

	// Must be valid UTF-8
	for i, r := range got {
		if r == '\uFFFD' {
			t.Errorf("invalid UTF-8 at byte %d after truncation", i)
			break
		}
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestTruncateSnippet_NoSpaces(t *testing.T) {
	// Long string with no spaces should hard-cut at maxLen
	input := strings.Repeat("a", 300)
	got := truncateSnippet(input, 50)
	runes := []rune(got)
	// 50 chars + ellipsis = 51
	if len(runes) != 51 {
		t.Errorf("expected 51 runes (50 + ellipsis), got %d", len(runes))
	}
}

func TestRRFFusion_SnippetTruncation(t *testing.T) {
	// Build chunk content longer than maxSnippetLen
	var longContent strings.Builder
	for range 100 {
		longContent.WriteString("word ")
	}

	bm25 := []db.ChunkWithScore{
		chunkWithScore("a", longContent.String(), 5.0),
	}
	dm := docMap(doc("a", "Doc A"))

	results := rrfFusion(bm25, nil, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	snippet := results[0].MatchedChunks[0].Snippet
	if len([]rune(snippet)) > maxSnippetLen+5 { // +5 for ellipsis char
		t.Errorf("snippet should be truncated, got rune length %d", len([]rune(snippet)))
	}
}

func TestRRFFusion_HeadingPathPreserved(t *testing.T) {
	heading := "## Setup > ### Install"
	bm25 := []db.ChunkWithScore{
		chunkWithHeading("a", "Install instructions", heading, 5.0),
	}
	dm := docMap(doc("a", "Doc A"))

	results := rrfFusion(bm25, nil, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].MatchedChunks) != 1 {
		t.Fatalf("expected 1 matched chunk, got %d", len(results[0].MatchedChunks))
	}
	if results[0].MatchedChunks[0].HeadingPath == nil {
		t.Fatal("expected heading path, got nil")
	}
	if *results[0].MatchedChunks[0].HeadingPath != heading {
		t.Errorf("expected heading %q, got %q", heading, *results[0].MatchedChunks[0].HeadingPath)
	}
}

// =============================================================================
// AGGREGATE CHUNKS TESTS (BM25-only path)
// =============================================================================

func TestAggregateChunks_MultiDocRanking(t *testing.T) {
	// Two docs with different scores — higher score doc should rank first
	chunks := []db.ChunkWithScore{
		chunkWithScore("b", "Doc B content", 3.0),
		chunkWithScore("a", "Doc A content", 5.0),
	}
	dm := docMap(doc("a", "Doc A"), doc("b", "Doc B"))

	results := aggregateChunks(chunks, dm, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].DocumentID != "a" {
		t.Errorf("expected Doc A first (higher score), got %q", results[0].DocumentID)
	}
	if results[0].Score != 5.0 {
		t.Errorf("expected score 5.0, got %f", results[0].Score)
	}
	if results[1].DocumentID != "b" {
		t.Errorf("expected Doc B second, got %q", results[1].DocumentID)
	}
}

func TestAggregateChunks_MultiChunkPerDoc(t *testing.T) {
	// Multiple chunks from same document — score should be max, both chunks attached
	chunks := []db.ChunkWithScore{
		chunkWithScorePos("a", "chunk 1", 0, 3.0),
		chunkWithScorePos("a", "chunk 2", 1, 7.0),
		chunkWithScorePos("a", "chunk 3", 2, 1.0),
	}
	dm := docMap(doc("a", "Doc A"))

	results := aggregateChunks(chunks, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 7.0 {
		t.Errorf("expected max score 7.0, got %f", results[0].Score)
	}
	if len(results[0].MatchedChunks) != 3 {
		t.Errorf("expected 3 matched chunks, got %d", len(results[0].MatchedChunks))
	}
}

func TestAggregateChunks_Limit(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunkWithScore("a", "Doc A", 5.0),
		chunkWithScore("b", "Doc B", 4.0),
		chunkWithScore("c", "Doc C", 3.0),
	}
	dm := docMap(doc("a", "Doc A"), doc("b", "Doc B"), doc("c", "Doc C"))

	results := aggregateChunks(chunks, dm, 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (limit), got %d", len(results))
	}
	if results[0].DocumentID != "a" {
		t.Errorf("expected Doc A first, got %q", results[0].DocumentID)
	}
	if results[1].DocumentID != "b" {
		t.Errorf("expected Doc B second, got %q", results[1].DocumentID)
	}
}

func TestAggregateChunks_OrphanChunk(t *testing.T) {
	// Chunk references a doc not in docMap — should be skipped
	chunks := []db.ChunkWithScore{
		chunkWithScore("a", "Doc A content", 5.0),
		chunkWithScore("orphan", "orphan content", 9.0),
	}
	dm := docMap(doc("a", "Doc A"))

	results := aggregateChunks(chunks, dm, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (orphan skipped), got %d", len(results))
	}
	if results[0].DocumentID != "a" {
		t.Errorf("expected Doc A, got %q", results[0].DocumentID)
	}
}

func TestAggregateChunks_Empty(t *testing.T) {
	results := aggregateChunks(nil, nil, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil chunks, got %d", len(results))
	}
}

func TestAggregateChunks_StableOrder(t *testing.T) {
	// Two docs with equal scores — should preserve encounter order
	chunks := []db.ChunkWithScore{
		chunkWithScore("first", "first doc", 5.0),
		chunkWithScore("second", "second doc", 5.0),
	}
	dm := docMap(
		models.Document{ID: rid("document", "first"), Title: "First", Path: "first.md"},
		models.Document{ID: rid("document", "second"), Title: "Second", Path: "second.md"},
	)

	results := aggregateChunks(chunks, dm, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].DocumentID != "first" {
		t.Errorf("expected 'first' to maintain encounter order, got %q", results[0].DocumentID)
	}
}
