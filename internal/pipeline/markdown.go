package pipeline

import (
	"context"

	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

// MarkdownChunker wraps the existing parser.ChunkMarkdown for markdown files.
type MarkdownChunker struct{}

func (m *MarkdownChunker) Chunk(_ context.Context, _ *models.File, content string, config ChunkConfig) ([]ChunkResult, error) {
	parsed := parser.ParseMarkdown(content)
	results := parser.ChunkMarkdown(parsed, config.ToParserConfig())

	chunks := make([]ChunkResult, len(results))
	for i, r := range results {
		chunks[i] = ChunkResult{
			Text:      r.Content,
			MimeType:  "text/plain",
			Position:  r.Position,
			SourceLoc: r.HeadingPath,
		}
	}
	return chunks, nil
}
