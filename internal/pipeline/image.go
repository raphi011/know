package pipeline

import (
	"context"

	"github.com/raphi011/know/internal/models"
)

// ImageChunker creates a single chunk from an image file.
// Currently a no-op — binary data is stored in the blob store and not available
// on the File struct. Full implementation will read from blob store when needed.
type ImageChunker struct{}

func (i *ImageChunker) Chunk(_ context.Context, _ *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	return nil, nil
}
