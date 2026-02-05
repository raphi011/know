package parser

import (
	"strings"
	"testing"
)

func TestChunkMarkdown_EmptyContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantLen  int
		wantZero bool // expect zero chunks
	}{
		{
			name:     "completely empty",
			content:  "",
			wantZero: true,
		},
		{
			name:     "whitespace only",
			content:  "   \n\n\t  ",
			wantZero: true,
		},
		{
			// Short content below threshold - raw markdown returned as-is
			// Headings-only short content is passed through (not chunked)
			name:    "heading only no content - below threshold",
			content: "# Title\n\n## Section",
			wantLen: 1, // Short content passed as single chunk
		},
		{
			name:    "heading with content",
			content: "# Title\n\nSome actual content here.",
			wantLen: 1,
		},
		{
			name:    "mixed empty and content sections",
			content: "# Empty\n\n## Also Empty\n\n## Has Content\n\nThis section has content.",
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseMarkdown(tt.content)
			if err != nil {
				t.Fatalf("ParseMarkdown() error = %v", err)
			}

			chunks := ChunkMarkdown(doc, DefaultChunkConfig())

			if tt.wantZero {
				if len(chunks) != 0 {
					t.Errorf("ChunkMarkdown() got %d chunks, want 0", len(chunks))
					for i, c := range chunks {
						t.Errorf("  chunk[%d]: %q", i, c.Content)
					}
				}
				return
			}

			if len(chunks) != tt.wantLen {
				t.Errorf("ChunkMarkdown() got %d chunks, want %d", len(chunks), tt.wantLen)
			}

			// Verify no empty chunks
			for i, chunk := range chunks {
				if strings.TrimSpace(chunk.Content) == "" {
					t.Errorf("chunk[%d] is empty", i)
				}
			}
		})
	}
}

func TestChunkBySections_SkipsEmptySections(t *testing.T) {
	sections := []Section{
		{Path: "Empty", Content: ""},
		{Path: "Whitespace", Content: "   \n\t  "},
		{Path: "HasContent", Content: "This has actual content that is meaningful."},
		{Path: "AnotherEmpty", Content: ""},
	}

	config := DefaultChunkConfig()
	// Lower min size for test
	config.MinSize = 10

	chunks := chunkBySections(sections, config)

	if len(chunks) != 1 {
		t.Errorf("chunkBySections() got %d chunks, want 1", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q content=%q", i, c.HeadingPath, c.Content)
		}
		return
	}

	if chunks[0].HeadingPath != "HasContent" {
		t.Errorf("chunk[0].HeadingPath = %q, want 'HasContent'", chunks[0].HeadingPath)
	}
}

func TestChunkBySections_AllEmpty(t *testing.T) {
	sections := []Section{
		{Path: "Empty1", Content: ""},
		{Path: "Empty2", Content: "   "},
		{Path: "Empty3", Content: "\n\n"},
	}

	chunks := chunkBySections(sections, DefaultChunkConfig())

	if len(chunks) != 0 {
		t.Errorf("chunkBySections() with all empty sections got %d chunks, want 0", len(chunks))
	}
}

// TestChunkMarkdown_LongContentWithEmptySections simulates the actual bug scenario:
// a long document that exceeds threshold but has sections with empty content.
func TestChunkMarkdown_LongContentWithEmptySections(t *testing.T) {
	// Create content that exceeds threshold (1500 chars) but sections have empty content
	// This would happen with a document full of headings but sparse content
	var sb strings.Builder
	sb.WriteString("# Decision Log\n\n")
	// Add many empty sections to exceed threshold
	for i := 1; i <= 50; i++ {
		sb.WriteString("## Decision " + strings.Repeat("X", 20) + "\n\n")
	}
	// Add one section with actual content
	sb.WriteString("## Decision with content\n\n")
	sb.WriteString("This decision has actual meaningful content that should be chunked.\n\n")

	content := sb.String()
	if len(content) < 1500 {
		t.Fatalf("test content too short: %d chars, need >1500", len(content))
	}

	doc, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	chunks := ChunkMarkdown(doc, DefaultChunkConfig())

	// Should only have chunks from the section with content
	for i, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk.Content)
		if trimmed == "" {
			t.Errorf("chunk[%d] is empty (text_len=0 would fail embedding)", i)
		}
	}

	if len(chunks) == 0 {
		t.Error("expected at least one chunk from section with content")
	}
}

func TestApplyOverlap_SemanticBoundaries(t *testing.T) {
	tests := []struct {
		name           string
		chunks         []ChunkResult
		overlap        int
		wantContains   []string // strings that should appear in second chunk
		wantNotPrefix  []string // strings that should NOT be at the start of second chunk
	}{
		{
			name: "prefers sentence boundary over word boundary",
			chunks: []ChunkResult{
				{Content: "First chunk with some content. This is the last sentence.", Position: 0},
				{Content: "Second chunk content here.", Position: 1},
			},
			overlap:       40,
			wantContains:  []string{"This is the last sentence."},
			wantNotPrefix: []string{"sentence."}, // should not cut mid-sentence
		},
		{
			name: "handles exclamation marks",
			chunks: []ChunkResult{
				{Content: "Something important! Remember this part.", Position: 0},
				{Content: "Next section.", Position: 1},
			},
			overlap:      30,
			wantContains: []string{"Remember this part."},
		},
		{
			name: "handles question marks",
			chunks: []ChunkResult{
				{Content: "What is the answer? The answer is here.", Position: 0},
				{Content: "More content.", Position: 1},
			},
			overlap:      30,
			wantContains: []string{"The answer is here."},
		},
		{
			name: "falls back to word boundary when no sentence boundary",
			chunks: []ChunkResult{
				{Content: "No sentence endings here, just words and more words", Position: 0},
				{Content: "Second chunk.", Position: 1},
			},
			overlap:       20,
			wantNotPrefix: []string{"rds"}, // should not cut mid-word
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyOverlap(tt.chunks, tt.overlap)

			if len(result) < 2 {
				t.Fatalf("expected at least 2 chunks, got %d", len(result))
			}

			secondChunk := result[1].Content

			for _, want := range tt.wantContains {
				if !strings.Contains(secondChunk, want) {
					t.Errorf("second chunk should contain %q\ngot: %q", want, secondChunk)
				}
			}

			for _, notWant := range tt.wantNotPrefix {
				if strings.HasPrefix(secondChunk, notWant) {
					t.Errorf("second chunk should not start with %q\ngot: %q", notWant, secondChunk)
				}
			}
		})
	}
}

func TestApplyOverlap_EdgeCases(t *testing.T) {
	// Empty chunks
	result := applyOverlap([]ChunkResult{}, 100)
	if len(result) != 0 {
		t.Error("empty input should return empty output")
	}

	// Single chunk
	single := []ChunkResult{{Content: "Only one chunk.", Position: 0}}
	result = applyOverlap(single, 100)
	if len(result) != 1 || result[0].Content != "Only one chunk." {
		t.Error("single chunk should be unchanged")
	}

	// Zero overlap
	two := []ChunkResult{
		{Content: "First chunk.", Position: 0},
		{Content: "Second chunk.", Position: 1},
	}
	result = applyOverlap(two, 0)
	if result[1].Content != "Second chunk." {
		t.Errorf("zero overlap should not modify chunks, got %q", result[1].Content)
	}
}
