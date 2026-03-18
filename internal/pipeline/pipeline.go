package pipeline

import (
	"context"
	"strings"

	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

// ChunkConfig controls chunking behavior across all file types.
type ChunkConfig struct {
	// Text chunking thresholds (used by markdown, PDF text extraction)
	Threshold  int // only chunk if content exceeds this (default 6000)
	TargetSize int // ideal chunk size in chars (default 3000)
	MaxSize    int // hard limit per chunk in chars (default 4000)

	// Audio chunking
	AudioSegmentSeconds int // max audio segment duration (default 60, max 80 for Gemini)
}

// DefaultChunkConfig returns sensible defaults.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		Threshold:           6000,
		TargetSize:          3000,
		MaxSize:             4000,
		AudioSegmentSeconds: 60,
	}
}

// ToParserConfig converts to the existing parser.ChunkConfig.
func (c ChunkConfig) ToParserConfig() parser.ChunkConfig {
	return parser.ChunkConfig{
		Threshold:  c.Threshold,
		TargetSize: c.TargetSize,
		MaxSize:    c.MaxSize,
	}
}

// ChunkResult represents a single chunk produced by a Chunker.
type ChunkResult struct {
	Text      string // extracted/transcribed text (for BM25 + text embedding)
	Data      []byte // optional binary payload (for multimodal embedding)
	MimeType  string // "text/plain" for text chunks, original MIME for multimodal
	Position  int
	SourceLoc string // location within source file
}

// Chunker splits a file into chunks for embedding and search.
// content is the text content loaded from the blob store (empty for binary files).
type Chunker interface {
	Chunk(ctx context.Context, file *models.File, content string, config ChunkConfig) ([]ChunkResult, error)
}

// Registry maps MIME types to their processing pipeline components.
type Registry struct {
	chunkers       map[string]Chunker
	textExtractors []llm.TextExtractor
}

// NewRegistry creates a registry with built-in chunkers.
func NewRegistry() *Registry {
	r := &Registry{
		chunkers: make(map[string]Chunker),
	}
	r.RegisterChunker(models.MimeMarkdown, &MarkdownChunker{})
	r.RegisterChunker("application/pdf", &PDFChunker{})
	r.RegisterChunker("audio/", &AudioChunker{})
	r.RegisterChunker("image/", &ImageChunker{})
	return r
}

// RegisterChunker registers a chunker for an exact MIME type or prefix (e.g. "audio/").
func (r *Registry) RegisterChunker(mimeType string, c Chunker) {
	r.chunkers[mimeType] = c
}

// RegisterTextExtractor adds a text extractor to the registry.
func (r *Registry) RegisterTextExtractor(e llm.TextExtractor) {
	r.textExtractors = append(r.textExtractors, e)
}

// ChunkerFor returns the chunker for a MIME type.
// Tries exact match first, then prefix match (e.g. "audio/" matches "audio/mpeg").
// Returns nil for unknown types (no chunking).
func (r *Registry) ChunkerFor(mimeType string) Chunker {
	if c, ok := r.chunkers[mimeType]; ok {
		return c
	}
	prefix := strings.SplitN(mimeType, "/", 2)[0] + "/"
	if c, ok := r.chunkers[prefix]; ok {
		return c
	}
	return nil
}

// TextExtractorFor returns the first text extractor that supports the given MIME type.
// Returns nil if no extractor is available (graceful degradation).
func (r *Registry) TextExtractorFor(mimeType string) llm.TextExtractor {
	for _, e := range r.textExtractors {
		if e.SupportsMIME(mimeType) {
			return e
		}
	}
	return nil
}
