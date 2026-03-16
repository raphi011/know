package pipeline

import (
	"testing"

	"github.com/raphi011/know/internal/stt"
)

func TestGroupSegments_Empty(t *testing.T) {
	result := GroupSegments(nil, 60)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}

	result = GroupSegments([]stt.Segment{}, 60)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestGroupSegments_SingleSegment(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 5, Text: "Hello world"},
	}

	results := GroupSegments(segments, 60)

	if len(results) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(results))
	}
	if results[0].Text != "Hello world" {
		t.Errorf("text = %q, want %q", results[0].Text, "Hello world")
	}
	if results[0].SourceLoc != "0:00-0:05" {
		t.Errorf("source_loc = %q, want %q", results[0].SourceLoc, "0:00-0:05")
	}
	if results[0].Position != 0 {
		t.Errorf("position = %d, want 0", results[0].Position)
	}
	if results[0].MimeType != "text/plain" {
		t.Errorf("mime_type = %q, want %q", results[0].MimeType, "text/plain")
	}
	if results[0].Data != nil {
		t.Errorf("data should be nil")
	}
}

func TestGroupSegments_MultipleSegmentsOneWindow(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 10, Text: "First"},
		{Start: 10, End: 20, Text: "Second"},
		{Start: 20, End: 30, Text: "Third"},
	}

	results := GroupSegments(segments, 60)

	if len(results) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(results))
	}
	if results[0].Text != "First Second Third" {
		t.Errorf("text = %q, want %q", results[0].Text, "First Second Third")
	}
	if results[0].SourceLoc != "0:00-0:30" {
		t.Errorf("source_loc = %q, want %q", results[0].SourceLoc, "0:00-0:30")
	}
}

func TestGroupSegments_MultipleWindows(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 25, Text: "First"},
		{Start: 25, End: 50, Text: "Second"},
		{Start: 55, End: 70, Text: "Third"},
		{Start: 70, End: 90, Text: "Fourth"},
		{Start: 120, End: 130, Text: "Fifth"},
	}

	results := GroupSegments(segments, 60)

	if len(results) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(results))
	}

	// First window: starts at 0, includes segments until one starts >= 60s from window start
	// 0, 25, 55 are all < 60 from start 0. 70 >= 60 → new window.
	if results[0].Text != "First Second Third" {
		t.Errorf("chunk 0 text = %q, want %q", results[0].Text, "First Second Third")
	}
	if results[0].SourceLoc != "0:00-1:10" {
		t.Errorf("chunk 0 source_loc = %q, want %q", results[0].SourceLoc, "0:00-1:10")
	}
	if results[0].Position != 0 {
		t.Errorf("chunk 0 position = %d, want 0", results[0].Position)
	}

	// Second window: starts at 70, includes 70 and 120 (120-70=50 < 60)
	if results[1].Text != "Fourth Fifth" {
		t.Errorf("chunk 1 text = %q, want %q", results[1].Text, "Fourth Fifth")
	}
	if results[1].SourceLoc != "1:10-2:10" {
		t.Errorf("chunk 1 source_loc = %q, want %q", results[1].SourceLoc, "1:10-2:10")
	}
	if results[1].Position != 1 {
		t.Errorf("chunk 1 position = %d, want 1", results[1].Position)
	}
}

func TestGroupSegments_DefaultWindow(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 30, Text: "First"},
		{Start: 30, End: 55, Text: "Second"},
		{Start: 61, End: 80, Text: "Third"},
	}

	// windowSeconds=0 should default to 60
	results := GroupSegments(segments, 0)

	if len(results) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(results))
	}
	if results[0].Text != "First Second" {
		t.Errorf("chunk 0 text = %q, want %q", results[0].Text, "First Second")
	}
	if results[1].Text != "Third" {
		t.Errorf("chunk 1 text = %q, want %q", results[1].Text, "Third")
	}

	// Negative value should also default to 60
	results2 := GroupSegments(segments, -5)
	if len(results2) != 2 {
		t.Fatalf("expected 2 chunks with negative window, got %d", len(results2))
	}
}

func TestGroupSegments_TimeFormatting(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 5, Text: "A"},
		{Start: 3661, End: 3720, Text: "B"},
	}

	// Use a very small window so each segment is its own chunk
	results := GroupSegments(segments, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(results))
	}

	if results[0].SourceLoc != "0:00-0:05" {
		t.Errorf("chunk 0 source_loc = %q, want %q", results[0].SourceLoc, "0:00-0:05")
	}
	// 3661s = 61:01, 3720s = 62:00
	if results[1].SourceLoc != "61:01-62:00" {
		t.Errorf("chunk 1 source_loc = %q, want %q", results[1].SourceLoc, "61:01-62:00")
	}
}

func TestGroupSegments_TextTrimming(t *testing.T) {
	segments := []stt.Segment{
		{Start: 0, End: 5, Text: " Hello "},
		{Start: 5, End: 10, Text: " World "},
	}

	results := GroupSegments(segments, 60)

	if len(results) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(results))
	}
	if results[0].Text != "Hello   World" {
		t.Errorf("text = %q, want %q", results[0].Text, "Hello   World")
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{59, "0:59"},
		{60, "1:00"},
		{65.5, "1:05"},
		{3661, "61:01"},
		{930, "15:30"},
	}

	for _, tt := range tests {
		got := formatTime(tt.seconds)
		if got != tt.want {
			t.Errorf("formatTime(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}
