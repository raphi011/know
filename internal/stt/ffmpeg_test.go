package stt

import (
	"context"
	"encoding/binary"
	"os/exec"
	"testing"
	"time"
)

func TestCalcSegmentDuration(t *testing.T) {
	tests := []struct {
		name          string
		totalDuration float64
		totalBytes    int
		maxBytes      int
		want          float64
	}{
		{
			name:          "file already under limit",
			totalDuration: 60.0,
			totalBytes:    10 * 1024 * 1024,
			maxBytes:      25 * 1024 * 1024,
			want:          60.0,
		},
		{
			name:          "file exactly at limit",
			totalDuration: 60.0,
			totalBytes:    25 * 1024 * 1024,
			maxBytes:      25 * 1024 * 1024,
			want:          60.0,
		},
		{
			name:          "file double the limit",
			totalDuration: 300.0,
			totalBytes:    50 * 1024 * 1024,
			maxBytes:      25 * 1024 * 1024,
			want:          150.0,
		},
		{
			name:          "file triple the limit",
			totalDuration: 300.0,
			totalBytes:    75 * 1024 * 1024,
			maxBytes:      25 * 1024 * 1024,
			want:          100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcSegmentDuration(tt.totalDuration, tt.totalBytes, tt.maxBytes)
			if got != tt.want {
				t.Errorf("calcSegmentDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatSeconds(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0.000"},
		{1.5, "1.500"},
		{150.123, "150.123"},
	}

	for _, tt := range tests {
		got := formatSeconds(tt.input)
		if got != tt.want {
			t.Errorf("formatSeconds(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func ffmpegAvailable() bool {
	_, err1 := exec.LookPath("ffmpeg")
	_, err2 := exec.LookPath("ffprobe")
	return err1 == nil && err2 == nil
}

// generateWAV creates a minimal WAV file with silence of the given duration.
// 8000 Hz sample rate, mono, 16-bit PCM.
func generateWAV(duration time.Duration) []byte {
	sampleRate := 8000
	numSamples := int(duration.Seconds()) * sampleRate
	dataSize := numSamples * 2 // 16-bit = 2 bytes per sample

	buf := make([]byte, 44+dataSize)

	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")

	// fmt sub-chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // sub-chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(buf[22:24], 1)  // mono
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], 2)                    // block align
	binary.LittleEndian.PutUint16(buf[34:36], 16)                   // bits per sample

	// data sub-chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	// samples are all zeros (silence)

	return buf
}

func TestSplitForTranscription_SmallWAV(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}

	// Generate a 4-second WAV file (~64KB at 8kHz mono 16-bit)
	wav := generateWAV(4 * time.Second)

	// Split with a very small maxBytes to force multiple parts
	// Each second of 8kHz 16-bit mono = 16000 bytes
	// 4 seconds = ~64000 bytes of audio data + 44 header
	// Set max to ~20000 to force splitting
	parts, err := SplitForTranscription(context.Background(), wav, "audio/wav", 20000)
	if err != nil {
		t.Fatalf("SplitForTranscription() error: %v", err)
	}

	if len(parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %d", len(parts))
	}

	// First part should have offset 0
	if parts[0].OffsetSecs != 0 {
		t.Errorf("first part offset = %v, want 0", parts[0].OffsetSecs)
	}

	// Each part should have non-empty data
	for i, p := range parts {
		if len(p.Data) == 0 {
			t.Errorf("part %d has empty data", i)
		}
	}

	// Offsets should be monotonically increasing
	for i := 1; i < len(parts); i++ {
		if parts[i].OffsetSecs <= parts[i-1].OffsetSecs {
			t.Errorf("part %d offset (%v) not greater than part %d offset (%v)",
				i, parts[i].OffsetSecs, i-1, parts[i-1].OffsetSecs)
		}
	}
}

func TestSplitForTranscription_SinglePart(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}

	// Generate a small WAV that fits within the limit
	wav := generateWAV(1 * time.Second)

	parts, err := SplitForTranscription(context.Background(), wav, "audio/wav", MaxWhisperFileSize)
	if err != nil {
		t.Fatalf("SplitForTranscription() error: %v", err)
	}

	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	if parts[0].OffsetSecs != 0 {
		t.Errorf("offset = %v, want 0", parts[0].OffsetSecs)
	}
}
