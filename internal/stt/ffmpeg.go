package stt

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// MaxWhisperFileSize is the maximum file size the OpenAI Whisper API accepts (25MB).
const MaxWhisperFileSize = 25 * 1024 * 1024

// SplitPart represents a segment of audio split from a larger file.
type SplitPart struct {
	Data       []byte  // audio segment bytes
	OffsetSecs float64 // start time offset in the original file
}

// SplitForTranscription splits audio data into parts smaller than maxBytes using ffmpeg.
// It writes data to a temp file, probes the duration, then splits into segments.
// Returns audio byte slices for sequential Whisper API calls along with their offset times.
// Returns an error if ffmpeg is not available.
func SplitForTranscription(ctx context.Context, data []byte, mimeType string, maxBytes int) ([]SplitPart, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: install ffmpeg for large file support")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: install ffmpeg for large file support")
	}

	ext := extForMIME(mimeType)

	tmpDir, err := os.MkdirTemp("", "stt-split-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input"+ext)
	if err := os.WriteFile(inputPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("write temp file: %w", err)
	}

	duration, err := probeDuration(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("probe duration: %w", err)
	}

	segmentDuration := calcSegmentDuration(duration, len(data), maxBytes)
	numSegments := int(math.Ceil(duration / segmentDuration))

	parts := make([]SplitPart, 0, numSegments)

	for i := range numSegments {
		start := float64(i) * segmentDuration
		outPath := filepath.Join(tmpDir, fmt.Sprintf("part_%03d%s", i, ext))

		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-v", "error",
			"-y",
			"-i", inputPath,
			"-ss", formatSeconds(start),
			"-t", formatSeconds(segmentDuration),
			"-c", "copy",
			outPath,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg split segment %d: %w: %s", i, err, output)
		}

		segData, err := os.ReadFile(outPath)
		if err != nil {
			return nil, fmt.Errorf("read segment %d: %w", i, err)
		}

		parts = append(parts, SplitPart{
			Data:       segData,
			OffsetSecs: start,
		})
	}

	return parts, nil
}

// probeDuration uses ffprobe to get the duration of an audio file in seconds.
func probeDuration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("run ffprobe: %w", err)
	}

	s := strings.TrimSpace(string(out))
	duration, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}

	return duration, nil
}

// calcSegmentDuration estimates how long each segment should be so that each
// part stays under maxBytes, assuming roughly constant bitrate.
func calcSegmentDuration(totalDuration float64, totalBytes, maxBytes int) float64 {
	if totalBytes <= maxBytes {
		return totalDuration
	}
	return totalDuration * float64(maxBytes) / float64(totalBytes)
}

// formatSeconds formats a float64 seconds value for ffmpeg's -ss/-t flags.
func formatSeconds(s float64) string {
	return strconv.FormatFloat(s, 'f', 3, 64)
}
