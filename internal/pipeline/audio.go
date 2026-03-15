package pipeline

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/models"
)

// AudioChunker splits audio files into time-based segments.
// Currently a stub — full implementation requires an audio processing library.
type AudioChunker struct{}

func (a *AudioChunker) Chunk(_ context.Context, file *models.File, config ChunkConfig) ([]ChunkResult, error) {
	if len(file.Data) == 0 {
		return nil, nil
	}

	segmentSecs := config.AudioSegmentSeconds
	if segmentSecs <= 0 {
		segmentSecs = 60
	}

	// TODO: implement audio splitting into segments of segmentSecs duration.
	// For now, create a single chunk with the full audio data.
	return []ChunkResult{{
		Data:      file.Data,
		MimeType:  file.MimeType,
		Position:  0,
		SourceLoc: fmt.Sprintf("0:00-%d:00", segmentSecs),
	}}, nil
}
