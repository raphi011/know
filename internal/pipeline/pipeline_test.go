package pipeline

import (
	"context"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestChunkerFor_ExactMatch(t *testing.T) {
	r := NewRegistry()

	c := r.ChunkerFor(models.MimeMarkdown)
	if c == nil {
		t.Fatal("expected MarkdownChunker for text/markdown")
	}
	if _, ok := c.(*MarkdownChunker); !ok {
		t.Fatalf("expected *MarkdownChunker, got %T", c)
	}

	c = r.ChunkerFor("application/pdf")
	if c == nil {
		t.Fatal("expected PDFChunker for application/pdf")
	}
}

func TestChunkerFor_PrefixMatch(t *testing.T) {
	r := NewRegistry()

	c := r.ChunkerFor("audio/mpeg")
	if c == nil {
		t.Fatal("expected AudioChunker for audio/mpeg via prefix match")
	}
	if _, ok := c.(*AudioChunker); !ok {
		t.Fatalf("expected *AudioChunker, got %T", c)
	}

	c = r.ChunkerFor("image/png")
	if c == nil {
		t.Fatal("expected ImageChunker for image/png via prefix match")
	}
	if _, ok := c.(*ImageChunker); !ok {
		t.Fatalf("expected *ImageChunker, got %T", c)
	}
}

func TestChunkerFor_UnknownReturnsNil(t *testing.T) {
	r := NewRegistry()

	c := r.ChunkerFor("application/octet-stream")
	if c != nil {
		t.Fatalf("expected nil for unknown MIME type, got %T", c)
	}

	c = r.ChunkerFor("")
	if c != nil {
		t.Fatalf("expected nil for empty MIME type, got %T", c)
	}
}

func TestTextExtractorFor_NoExtractors(t *testing.T) {
	r := NewRegistry()

	e := r.TextExtractorFor("application/pdf")
	if e != nil {
		t.Fatalf("expected nil when no extractors registered, got %T", e)
	}
}

func TestMarkdownChunker_Chunk(t *testing.T) {
	chunker := &MarkdownChunker{}
	file := &models.File{
		Content: "# Hello\n\nSome content here.",
	}

	chunks, err := chunker.Chunk(context.Background(), file, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].MimeType != "text/plain" {
		t.Fatalf("expected text/plain MIME type, got %q", chunks[0].MimeType)
	}
}

func TestImageChunker_NoOp(t *testing.T) {
	chunker := &ImageChunker{}
	file := &models.File{
		MimeType: "image/png",
	}

	chunks, err := chunker.Chunk(context.Background(), file, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Fatalf("expected nil chunks (no-op), got %d", len(chunks))
	}
}
