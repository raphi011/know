package pipeline

import (
	"context"
	"log/slog"

	"github.com/raphi011/know/internal/models"
)

// ImageChunker creates a single chunk from an image file.
// Currently a no-op — binary data is stored in the blob store and not available
// on the File struct. Full implementation will read from blob store when needed.
type ImageChunker struct{}

func (i *ImageChunker) Chunk(_ context.Context, file *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	if file != nil && file.Size > 0 {
		slog.Debug("image chunking skipped: blob store integration pending", "path", file.Path)
	}
	return nil, nil
}
