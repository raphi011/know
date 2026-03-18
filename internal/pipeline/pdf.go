package pipeline

import (
	"context"
	"log/slog"

	"github.com/raphi011/know/internal/models"
)

// PDFChunker splits PDFs into page-based chunks.
// Currently a no-op — binary data is stored in the blob store and not available
// on the File struct. Full implementation will read from blob store when needed.
type PDFChunker struct{}

func (p *PDFChunker) Chunk(_ context.Context, file *models.File, _ string, _ ChunkConfig) ([]ChunkResult, error) {
	if file != nil && file.Size > 0 {
		slog.Debug("pdf chunking skipped: blob store integration pending", "path", file.Path)
	}
	return nil, nil
}
