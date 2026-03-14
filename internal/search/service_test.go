package search

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func rid(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}

func chunk(docID, content string, score float64) db.ChunkWithScore {
	return db.ChunkWithScore{
		Chunk: models.Chunk{
			ID:       rid("chunk", docID+"-0"),
			Document: rid("document", docID),
			Content:  content,
			Position: 0,
		},
		Score:    score,
		DocPath:  "docs/" + docID + ".md",
		DocTitle: "Doc " + docID,
	}
}

func chunkPos(docID, content string, position int, score float64) db.ChunkWithScore {
	return db.ChunkWithScore{
		Chunk: models.Chunk{
			ID:       rid("chunk", fmt.Sprintf("%s-%d", docID, position)),
			Document: rid("document", docID),
			Content:  content,
			Position: position,
		},
		Score:    score,
		DocPath:  "docs/" + docID + ".md",
		DocTitle: "Doc " + docID,
	}
}

func chunkWithLabels(docID, content string, score float64, labels []string) db.ChunkWithScore {
	ch := chunk(docID, content, score)
	ch.DocLabels = labels
	return ch
}

func chunkWithHeading(docID, content string, heading string, score float64) db.ChunkWithScore {
	ch := chunk(docID, content, score)
	ch.Chunk.HeadingPath = &heading
	return ch
}

func chunkWithDocType(docID, content string, score float64, docType string) db.ChunkWithScore {
	ch := chunk(docID, content, score)
	ch.DocType = &docType
	return ch
}

// =============================================================================
// ASSEMBLE RESULTS TESTS
// =============================================================================

func TestAssembleResults_SingleDoc(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunk("a", "Doc A content", 5.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].DocumentID != "a" {
		t.Errorf("expected DocumentID 'a', got %q", results[0].DocumentID)
	}
	if results[0].Path != "docs/a.md" {
		t.Errorf("expected path 'docs/a.md', got %q", results[0].Path)
	}
	if results[0].Title != "Doc a" {
		t.Errorf("expected title 'Doc a', got %q", results[0].Title)
	}
	if len(results[0].MatchedChunks) != 1 {
		t.Errorf("expected 1 matched chunk, got %d", len(results[0].MatchedChunks))
	}
	if results[0].Degraded {
		t.Error("expected Degraded=false")
	}
}

func TestAssembleResults_MultiDocRanking(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunk("b", "Doc B content", 3.0),
		chunk("a", "Doc A content", 5.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Doc A should rank higher (higher score)
	if results[0].DocumentID != "a" {
		t.Errorf("expected Doc A first (higher score), got %q", results[0].DocumentID)
	}
	if results[0].Score != 5.0 {
		t.Errorf("expected score 5.0, got %f", results[0].Score)
	}
}

func TestAssembleResults_MultiChunkPerDoc(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunkPos("a", "chunk 1", 0, 3.0),
		chunkPos("a", "chunk 2", 1, 7.0),
		chunkPos("a", "chunk 3", 2, 1.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Score should be sum of chunk scores
	if results[0].Score != 11.0 {
		t.Errorf("expected sum score 11.0, got %f", results[0].Score)
	}
	if len(results[0].MatchedChunks) != 3 {
		t.Errorf("expected 3 matched chunks, got %d", len(results[0].MatchedChunks))
	}
}

func TestAssembleResults_Limit(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunk("a", "Doc A", 5.0),
		chunk("b", "Doc B", 4.0),
		chunk("c", "Doc C", 3.0),
		chunk("d", "Doc D", 2.0),
		chunk("e", "Doc E", 1.0),
	}

	results := assembleResults(context.Background(), chunks, 3, false, false)
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

func TestAssembleResults_Empty(t *testing.T) {
	results := assembleResults(context.Background(), nil, 10, false, false)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil chunks, got %d", len(results))
	}
}

func TestAssembleResults_StableOrder(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunk("first", "first doc", 5.0),
		chunk("second", "second doc", 5.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].DocumentID != "first" {
		t.Errorf("expected 'first' to maintain encounter order, got %q", results[0].DocumentID)
	}
}

func TestAssembleResults_HeadingPathPreserved(t *testing.T) {
	heading := "## Setup > ### Install"
	chunks := []db.ChunkWithScore{
		chunkWithHeading("a", "Install instructions", heading, 5.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MatchedChunks[0].HeadingPath == nil {
		t.Fatal("expected heading path, got nil")
	}
	if *results[0].MatchedChunks[0].HeadingPath != heading {
		t.Errorf("expected heading %q, got %q", heading, *results[0].MatchedChunks[0].HeadingPath)
	}
}

func TestAssembleResults_Labels(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunkWithLabels("a", "content", 5.0, []string{"go", "test"}),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Labels) != 2 || results[0].Labels[0] != "go" {
		t.Errorf("expected labels [go, test], got %v", results[0].Labels)
	}
}

func TestAssembleResults_NilLabelsNormalized(t *testing.T) {
	ch := chunk("a", "content", 5.0)
	ch.DocLabels = nil

	results := assembleResults(context.Background(), []db.ChunkWithScore{ch}, 10, false, false)
	if results[0].Labels == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestAssembleResults_DocType(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunkWithDocType("a", "content", 5.0, "note"),
	}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if results[0].DocType == nil || *results[0].DocType != "note" {
		t.Errorf("expected doc_type 'note', got %v", results[0].DocType)
	}
}

func TestAssembleResults_Degraded(t *testing.T) {
	chunks := []db.ChunkWithScore{
		chunk("a", "content", 5.0),
		chunk("b", "content", 3.0),
	}

	results := assembleResults(context.Background(), chunks, 10, false, true)
	for i, r := range results {
		if !r.Degraded {
			t.Errorf("result %d: expected Degraded=true", i)
		}
	}
}

// =============================================================================
// FULL CONTENT TESTS
// =============================================================================

func TestChunkToMatch_FullContent(t *testing.T) {
	longContent := strings.Repeat("word ", 100) // 500 chars
	ch := chunk("a", longContent, 5.0)

	truncated := chunkToMatch(ch, false)
	if truncated.Snippet == longContent {
		t.Error("expected snippet to be truncated when fullContent=false")
	}
	if len([]rune(truncated.Snippet)) > maxSnippetLen+5 {
		t.Errorf("expected truncated snippet, got rune length %d", len([]rune(truncated.Snippet)))
	}

	full := chunkToMatch(ch, true)
	if full.Snippet != longContent {
		t.Errorf("expected full content, got truncated snippet")
	}
}

func TestAssembleResults_FullContent(t *testing.T) {
	longContent := strings.Repeat("word ", 100)
	chunks := []db.ChunkWithScore{chunk("a", longContent, 5.0)}

	results := assembleResults(context.Background(), chunks, 10, true, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MatchedChunks[0].Snippet != longContent {
		t.Error("expected full content in snippet when fullContent=true")
	}
}

func TestAssembleResults_SnippetTruncation(t *testing.T) {
	var longContent strings.Builder
	for range 100 {
		longContent.WriteString("word ")
	}

	chunks := []db.ChunkWithScore{chunk("a", longContent.String(), 5.0)}

	results := assembleResults(context.Background(), chunks, 10, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	snippet := results[0].MatchedChunks[0].Snippet
	if len([]rune(snippet)) > maxSnippetLen+5 {
		t.Errorf("snippet should be truncated, got rune length %d", len([]rune(snippet)))
	}
}

// =============================================================================
// TRUNCATE SNIPPET TESTS
// =============================================================================

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
