package stt

import "context"

// Segment represents a timestamped portion of a transcript.
type Segment struct {
	Start float64 // seconds
	End   float64 // seconds
	Text  string
}

// Result holds the full transcript and its timestamped segments.
type Result struct {
	Text     string    // full transcript
	Segments []Segment // timestamped segments
}

// Transcriber converts audio bytes into a text transcript.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte, mimeType string) (*Result, error)
}
