package pipeline

import (
	"context"

	"github.com/raphi011/know/internal/models"
)

// ImageChunker creates a single chunk from an image file.
// The chunk stores the image bytes for multimodal embedding.
// Text is populated later by an optional image-to-text extractor.
type ImageChunker struct{}

func (i *ImageChunker) Chunk(_ context.Context, file *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	if len(file.Data) == 0 {
		return nil, nil
	}

	return []ChunkResult{{
		Data:     file.Data,
		MimeType: file.MimeType,
		Position: 0,
	}}, nil
}
