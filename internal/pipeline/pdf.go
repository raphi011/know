package pipeline

import (
	"context"

	"github.com/raphi011/know/internal/models"
)

// PDFChunker splits PDFs into page-based chunks.
// Currently a no-op — binary data is stored in the blob store and not available
// on the File struct. Full implementation will read from blob store when needed.
type PDFChunker struct{}

func (p *PDFChunker) Chunk(_ context.Context, _ *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	return nil, nil
}
