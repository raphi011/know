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
		{Path: "HasContent", Content: strings.Repeat("x", 100)},
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

func TestChunkBySections_EachHeadingGetsOwnChunk(t *testing.T) {
	// Small subsections should each get their own chunk (no merging)
	config := DefaultChunkConfig()
	parentContent := "Parent section content."
	childContent := "Small child section."

	sections := []Section{
		{Level: 2, Path: "## Parent", Content: parentContent},
		{Level: 3, Path: "## Parent > ### Child", Content: childContent},
	}

	chunks := chunkBySections(sections, config)

	// Each section gets its own chunk — no merging
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (one per heading), got %d", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
		return
	}

	if chunks[0].HeadingPath != "## Parent" {
		t.Errorf("chunk[0].HeadingPath = %q, want '## Parent'", chunks[0].HeadingPath)
	}
	if chunks[1].HeadingPath != "## Parent > ### Child" {
		t.Errorf("chunk[1].HeadingPath = %q, want '## Parent > ### Child'", chunks[1].HeadingPath)
	}
	if chunks[0].Content != parentContent {
		t.Errorf("chunk[0] should contain only parent content")
	}
	if chunks[1].Content != childContent {
		t.Errorf("chunk[1] should contain only child content")
	}
}

func TestChunkBySections_SmallSectionsStaySeparate(t *testing.T) {
	config := DefaultChunkConfig()
	// Two small top-level sections should each get their own chunk
	smallContent := "Short section content."

	sections := []Section{
		{Level: 2, Path: "## Alpha", Content: smallContent},
		{Level: 2, Path: "## Beta", Content: smallContent},
	}

	chunks := chunkBySections(sections, config)

	if len(chunks) != 2 {
		t.Errorf("expected 2 separate chunks, got %d", len(chunks))
		for i, c := range chunks {
			t.Logf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
	}
}

func TestChunkBySections_CodeBlockAtomic(t *testing.T) {
	config := DefaultChunkConfig()
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

func TestChunkBySections_SmallCodeBlockGetsOwnChunk(t *testing.T) {
	config := DefaultChunkConfig()
	parentContent := strings.Repeat("Parent context. ", 30) // ~480 chars
	codeContent := "```\nsmall code\n```"                   // tiny

	sections := []Section{
		{Level: 2, Path: "## Setup", Content: parentContent},
		{Level: 3, Path: "## Setup > ### Code", Content: codeContent, CodeBlock: true},
	}

	chunks := chunkBySections(sections, config)

	// Each section gets its own chunk — no merging
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (no merging), got %d", len(chunks))
		for i, c := range chunks {
			t.Errorf("  chunk[%d] path=%q len=%d", i, c.HeadingPath, len(c.Content))
		}
		return
	}

	if chunks[0].HeadingPath != "## Setup" {
		t.Errorf("chunk[0] should be parent, got %q", chunks[0].HeadingPath)
	}
	if chunks[1].HeadingPath != "## Setup > ### Code" {
		t.Errorf("chunk[1] should be code child, got %q", chunks[1].HeadingPath)
	}
}

func TestChunkBySections_CodeBlockExceedsHardLimit(t *testing.T) {
	config := DefaultChunkConfig()
	largeCode := strings.Repeat("This is paragraph one with enough content to be meaningful.\n\n", 200)

	sections := []Section{
		{Level: 2, Path: "## HugeCode", Content: largeCode, CodeBlock: true},
	}

	if len(strings.TrimSpace(largeCode)) <= maxAtomicCodeBlockSize {
		t.Fatalf("test content too small: %d chars, need >%d", len(largeCode), maxAtomicCodeBlockSize)
	}

	chunks := chunkBySections(sections, config)

	if len(chunks) < 2 {
		t.Errorf("code block >%d chars with paragraph breaks should be split, got %d chunks",
			maxAtomicCodeBlockSize, len(chunks))
	}
}

func TestChunkBySections_LargeSectionSplitPreservesHeadingPath(t *testing.T) {
	config := DefaultChunkConfig()
	bigContent := strings.Repeat("This is a long paragraph with enough words. ", 200)

	sections := []Section{
		{Level: 2, Path: "## BigSection", Content: bigContent},
	}

	chunks := chunkBySections(sections, config)

	if len(chunks) < 2 {
		t.Fatalf("expected section to be split into multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.HeadingPath != "## BigSection" {
			t.Errorf("chunk[%d].HeadingPath = %q, want '## BigSection'", i, c.HeadingPath)
		}
	}
}

func TestChunkMarkdown_FallbackToParagraphs(t *testing.T) {
	config := DefaultChunkConfig()
	doc := &MarkdownDoc{
		Content:  strings.Repeat("Paragraph content here. ", 300),
		Sections: nil,
	}

	chunks := ChunkMarkdown(doc, config)

	if len(chunks) < 2 {
		t.Errorf("expected paragraph-based splitting, got %d chunks", len(chunks))
	}
}

func TestChunkMarkdown_BelowThresholdAboveMaxSize(t *testing.T) {
	config := ChunkConfig{
		Threshold:  6000,
		TargetSize: 3000,
		MaxSize:    4000,
	}

	// ~5000 chars: below Threshold (6000) but above MaxSize (4000)
	content := strings.Repeat("This is a paragraph with enough content to be meaningful for testing. ", 70)
	if len(content) <= config.MaxSize || len(content) >= config.Threshold {
		t.Fatalf("test content %d chars, need >%d and <%d", len(content), config.MaxSize, config.Threshold)
	}

	doc, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	chunks := ChunkMarkdown(doc, config)

	if len(chunks) < 2 {
		t.Errorf("content of %d chars (>MaxSize %d) should produce multiple chunks, got %d",
			len(content), config.MaxSize, len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk.Content) > config.MaxSize {
			t.Errorf("chunk[%d] length %d exceeds MaxSize %d", i, len(chunk.Content), config.MaxSize)
		}
	}
}

func TestChunkMarkdown_BelowThresholdAboveMaxSize_WithSections(t *testing.T) {
	config := ChunkConfig{
		Threshold:  6000,
		TargetSize: 3000,
		MaxSize:    4000,
	}

	content := "# Title\n\n## Section A\n\n" +
		strings.Repeat("Content for section A. ", 100) + "\n\n" +
		"## Section B\n\n" +
		strings.Repeat("Content for section B. ", 100)

	if len(content) <= config.MaxSize || len(content) >= config.Threshold {
		t.Fatalf("test content %d chars, need >%d and <%d", len(content), config.MaxSize, config.Threshold)
	}

	doc, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}

	chunks := ChunkMarkdown(doc, config)

	if len(chunks) < 2 {
		t.Errorf("sectioned content of %d chars should produce multiple chunks, got %d",
			len(content), len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk.Content) > config.MaxSize {
			t.Errorf("chunk[%d] length %d exceeds MaxSize %d", i, len(chunk.Content), config.MaxSize)
		}
	}
}

func TestChunkConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ChunkConfig
		wantErr bool
	}{
		{
			name:    "valid defaults",
			config:  DefaultChunkConfig(),
			wantErr: false,
		},
		{
			name:    "zero values",
			config:  ChunkConfig{},
			wantErr: true,
		},
		{
			name:    "TargetSize >= MaxSize",
			config:  ChunkConfig{TargetSize: 4000, MaxSize: 4000, Threshold: 6000},
			wantErr: true,
		},
		{
			name:    "MaxSize > Threshold is allowed",
			config:  ChunkConfig{TargetSize: 3000, MaxSize: 7000, Threshold: 6000},
			wantErr: false,
		},
		{
			name:    "negative value",
			config:  ChunkConfig{TargetSize: -1, MaxSize: 4000, Threshold: 6000},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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
}
