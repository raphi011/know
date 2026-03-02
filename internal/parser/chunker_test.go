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
		{Path: "HasContent", Content: strings.Repeat("x", 900)},
		{Path: "AnotherEmpty", Content: ""},
	}

	config := DefaultChunkConfig()
	chunks := chunkBySections(sections, config)

	if len(chunks) != 1 {
		t.Errorf("chunkBySections() got %d chunks, want 1", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
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

func TestChunkMarkdown_LongContentWithEmptySections(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("# Decision Log\n\n")
	for i := 1; i <= 200; i++ {
		sb.WriteString("## Decision " + strings.Repeat("X", 20) + "\n\n")
	}
	sb.WriteString("## Decision with content\n\n")
	sb.WriteString(strings.Repeat("This decision has actual meaningful content. ", 20) + "\n\n")

	content := sb.String()
	if len(content) < DefaultChunkConfig().Threshold {
		t.Fatalf("test content too short: %d chars, need >%d", len(content), DefaultChunkConfig().Threshold)
	}

	doc, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	chunks := ChunkMarkdown(doc, DefaultChunkConfig())

	for i, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk.Content)
		if trimmed == "" {
			t.Errorf("chunk[%d] is empty", i)
		}
	}

	if len(chunks) == 0 {
		t.Error("expected at least one chunk from section with content")
	}
}

func TestChunkBySections_HierarchicalMerge(t *testing.T) {
	// Small subsections should merge into parent heading's chunk
	config := DefaultChunkConfig()
	parentContent := strings.Repeat("Parent section content. ", 30) // ~720 chars
	childContent := "Small child section."                          // 20 chars, below MinSize

	sections := []Section{
		{Level: 2, Path: "## Parent", Content: parentContent},
		{Level: 3, Path: "## Parent > ### Child", Content: childContent},
	}

	chunks := chunkBySections(sections, config)

	// Child should have been merged into parent
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk (merged), got %d", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
		return
	}

	if !strings.Contains(chunks[0].Content, childContent) {
		t.Error("parent chunk should contain merged child content")
	}
	if chunks[0].HeadingPath != "## Parent" {
		t.Errorf("merged chunk should keep parent path, got %q", chunks[0].HeadingPath)
	}
}

func TestChunkBySections_HierarchicalMerge_OverflowCreatesNewChunk(t *testing.T) {
	config := DefaultChunkConfig()
	// Parent already near max
	parentContent := strings.Repeat("x", config.MaxSize-100)
	childContent := strings.Repeat("y", 200) // Would exceed max if merged

	sections := []Section{
		{Level: 2, Path: "## Parent", Content: parentContent},
		{Level: 3, Path: "## Parent > ### Child", Content: childContent},
	}

	chunks := chunkBySections(sections, config)

	// Child should be standalone since parent is too full
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (overflow), got %d", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
	}
}

func TestChunkBySections_CodeBlockAtomic(t *testing.T) {
	config := DefaultChunkConfig()
	// Create a code-block section that's between MinSize and MaxSize
	codeContent := "```go\n" + strings.Repeat("fmt.Println(\"hello\")\n", 80) + "```"

	sections := []Section{
		{Level: 2, Path: "## Example", Content: codeContent, CodeBlock: true},
	}

	chunks := chunkBySections(sections, config)

	if len(chunks) != 1 {
		t.Errorf("code block section should produce 1 atomic chunk, got %d", len(chunks))
		return
	}
	if !strings.Contains(chunks[0].Content, "fmt.Println") {
		t.Error("code block chunk should contain the code")
	}
}

func TestChunkBySections_CodeBlockSmallMergesIntoParent(t *testing.T) {
	config := DefaultChunkConfig()
	parentContent := strings.Repeat("Parent context. ", 30) // ~480 chars
	codeContent := "```\nsmall code\n```"                    // tiny

	sections := []Section{
		{Level: 2, Path: "## Setup", Content: parentContent},
		{Level: 3, Path: "## Setup > ### Code", Content: codeContent, CodeBlock: true},
	}

	chunks := chunkBySections(sections, config)

	// Small code block should merge into parent
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk (merged code block), got %d", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
		return
	}

	if !strings.Contains(chunks[0].Content, "small code") {
		t.Error("merged chunk should contain the code block")
	}
}

func TestDefaultChunkConfig_Values(t *testing.T) {
	config := DefaultChunkConfig()

	if config.Threshold != 6000 {
		t.Errorf("Threshold = %d, want 6000", config.Threshold)
	}
	if config.TargetSize != 3000 {
		t.Errorf("TargetSize = %d, want 3000", config.TargetSize)
	}
	if config.MaxSize != 4000 {
		t.Errorf("MaxSize = %d, want 4000", config.MaxSize)
	}
	if config.MinSize != 800 {
		t.Errorf("MinSize = %d, want 800", config.MinSize)
	}
}
