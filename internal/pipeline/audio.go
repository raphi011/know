package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/stt"
)

// AudioChunker is a no-op chunker for audio files.
// Audio chunking happens after transcription via GroupSegments, not during initial processing.
type AudioChunker struct{}

func (a *AudioChunker) Chunk(_ context.Context, _ *models.File, _ ChunkConfig) ([]ChunkResult, error) {
	return nil, nil
}

// GroupSegments groups STT segments into time-window chunks.
// windowSeconds controls the maximum duration of each chunk (e.g., 60 seconds).
// Returns ChunkResults with Text (concatenated segment text), SourceLoc (time range), and Position.
func GroupSegments(segments []stt.Segment, windowSeconds int) []ChunkResult {
	if len(segments) == 0 {
		return nil
	}
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	var results []ChunkResult
	windowStart := segments[0].Start
	var texts []string
	var windowEnd float64

	for _, seg := range segments {
		// If adding this segment would exceed the window, flush the current group.
		if len(texts) > 0 && seg.Start-windowStart >= float64(windowSeconds) {
			results = append(results, ChunkResult{
				Text:      strings.TrimSpace(strings.Join(texts, " ")),
				SourceLoc: fmt.Sprintf("%s-%s", formatTime(windowStart), formatTime(windowEnd)),
				Position:  len(results),
				MimeType:  "text/plain",
			})
			texts = texts[:0]
			windowStart = seg.Start
		}
		texts = append(texts, seg.Text)
		windowEnd = seg.End
	}

	// Flush remaining segments.
	if len(texts) > 0 {
		results = append(results, ChunkResult{
			Text:      strings.TrimSpace(strings.Join(texts, " ")),
			SourceLoc: fmt.Sprintf("%s-%s", formatTime(windowStart), formatTime(windowEnd)),
			Position:  len(results),
			MimeType:  "text/plain",
		})
	}

	return results
}

// formatTime formats seconds as M:SS (e.g., 0 → "0:00", 65.5 → "1:05", 3661 → "61:01").
func formatTime(seconds float64) string {
	totalSeconds := int(seconds)
	minutes := totalSeconds / 60
	secs := totalSeconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
