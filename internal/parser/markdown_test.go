package parser

import (
	"strings"
	"testing"
)

func TestParseSections_BasicHeadings(t *testing.T) {
	content := "# Title\n\nIntro paragraph.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."

	sections := parseSections(content)

	// Should have: preamble (empty), h1 Title, h2 Section A, h2 Section B
	// Preamble is empty so might be included with empty content
	var nonEmpty []Section
	for _, s := range sections {
		if strings.TrimSpace(s.Content) != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}

	if len(nonEmpty) < 3 {
		t.Errorf("expected at least 3 non-empty sections, got %d", len(nonEmpty))
		for i, s := range sections {
			t.Errorf("  section[%d]: level=%d heading=%q content=%q", i, s.Level, s.Heading, s.Content)
		}
	}
}

func TestParseSections_CodeFenceWithHash(t *testing.T) {
	// This is the critical bug fix: # inside code fences should NOT create sections
	content := "## Setup\n\nInstructions here.\n\n```bash\n# This is a comment\necho hello\n# Another comment\n```\n\n## Next\n\nMore content."

	sections := parseSections(content)

	var headings []string
	for _, s := range sections {
		if s.Level > 0 {
			headings = append(headings, s.Heading)
		}
	}

	// Should only have "Setup" and "Next" - NOT "This is a comment" or "Another comment"
	if len(headings) != 2 {
		t.Errorf("expected 2 headings, got %d: %v", len(headings), headings)
		return
	}
	if headings[0] != "Setup" || headings[1] != "Next" {
		t.Errorf("expected [Setup, Next], got %v", headings)
	}

	// The code block should be in the Setup section's content
	for _, s := range sections {
		if s.Heading == "Setup" {
			if !strings.Contains(s.Content, "# This is a comment") {
				t.Error("Setup section should contain the code block with # comments")
			}
			if !strings.Contains(s.Content, "```bash") {
				t.Error("Setup section should contain the fenced code block markers")
			}
			break
		}
	}
}

func TestParseSections_PreHeadingContent(t *testing.T) {
	content := "This is a preamble paragraph.\n\nAnother preamble line.\n\n## First Section\n\nSection content."

	sections := parseSections(content)

	if len(sections) < 1 {
		t.Fatal("expected at least 1 section")
	}

	// First section should be the preamble (Level=0)
	if sections[0].Level != 0 {
		t.Errorf("first section should be preamble (Level=0), got Level=%d", sections[0].Level)
	}
	if !strings.Contains(sections[0].Content, "preamble paragraph") {
		t.Errorf("preamble section should contain pre-heading content, got %q", sections[0].Content)
	}
}

func TestParseSections_NestedHeadings(t *testing.T) {
	content := "# Top\n\n## Sub1\n\nContent 1.\n\n### SubSub\n\nDeep content.\n\n## Sub2\n\nContent 2."

	sections := parseSections(content)

	// Find the subsub section and check its path
	var paths []string
	for _, s := range sections {
		if s.Level > 0 {
			paths = append(paths, s.Path)
		}
	}

	// Should have hierarchical paths
	found := false
	for _, p := range paths {
		if strings.Contains(p, "### SubSub") && strings.Contains(p, "## Sub1") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected nested path containing '## Sub1 > ### SubSub', got paths: %v", paths)
	}

	// Sub2 should NOT contain SubSub in its path (it pops back up)
	for _, s := range sections {
		if s.Heading == "Sub2" {
			if strings.Contains(s.Path, "SubSub") {
				t.Errorf("Sub2 path should not contain SubSub, got %q", s.Path)
			}
			break
		}
	}
}

func TestParseSections_DocumentWithoutHeadings(t *testing.T) {
	content := "Just a plain document with no headings.\n\nMultiple paragraphs here.\n\nAnd another one."

	sections := parseSections(content)

	// Should have a preamble section with all content
	if len(sections) != 1 {
		t.Errorf("expected 1 preamble section, got %d", len(sections))
		for i, s := range sections {
			t.Errorf("  section[%d]: level=%d content=%q", i, s.Level, s.Content)
		}
		return
	}

	if sections[0].Level != 0 {
		t.Errorf("expected preamble (Level=0), got Level=%d", sections[0].Level)
	}
	if !strings.Contains(sections[0].Content, "plain document") {
		t.Error("preamble should contain all content")
	}
}

func TestParseSections_CodeBlockMarkedCorrectly(t *testing.T) {
	// A section dominated by a code block should have CodeBlock=true
	content := "## Code Example\n\n```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"

	sections := parseSections(content)

	var codeSection *Section
	for _, s := range sections {
		if s.Heading == "Code Example" {
			codeSection = &s
			break
		}
	}

	if codeSection == nil {
		t.Fatal("expected to find 'Code Example' section")
	}
	if !codeSection.CodeBlock {
		t.Error("section dominated by code block should have CodeBlock=true")
	}
}

func TestParseSections_MixedContentNotCodeBlock(t *testing.T) {
	// A section with mostly prose and a small code snippet should NOT be CodeBlock
	content := "## Explanation\n\n" +
		strings.Repeat("This is a long explanation about how things work. ", 10) +
		"\n\n```\nsmall snippet\n```"

	sections := parseSections(content)

	var section *Section
	for _, s := range sections {
		if s.Heading == "Explanation" {
			section = &s
			break
		}
	}

	if section == nil {
		t.Fatal("expected to find 'Explanation' section")
	}
	if section.CodeBlock {
		t.Error("mixed section with mostly prose should have CodeBlock=false")
	}
}

func TestParseSections_EmptyDocument(t *testing.T) {
	sections := parseSections("")

	// Empty doc produces a preamble with no content
	for _, s := range sections {
		if strings.TrimSpace(s.Content) != "" {
			t.Errorf("empty document should produce no content, got %q", s.Content)
		}
	}
}

func TestParseSections_HeadingPathHierarchy(t *testing.T) {
	content := "# A\n\nContent A.\n\n## B\n\nContent B.\n\n### C\n\nContent C.\n\n## D\n\nContent D."

	sections := parseSections(content)

	pathMap := make(map[string]string)
	for _, s := range sections {
		if s.Level > 0 {
			pathMap[s.Heading] = s.Path
		}
	}

	// C should be under A > B
	if p, ok := pathMap["C"]; ok {
		if !strings.Contains(p, "# A") || !strings.Contains(p, "## B") || !strings.Contains(p, "### C") {
			t.Errorf("C path should contain A > B > C hierarchy, got %q", p)
		}
	} else {
		t.Error("expected section C")
	}

	// D should be under A only (B/C popped off)
	if p, ok := pathMap["D"]; ok {
		if strings.Contains(p, "## B") || strings.Contains(p, "### C") {
			t.Errorf("D path should not contain B or C, got %q", p)
		}
		if !strings.Contains(p, "# A") {
			t.Errorf("D path should contain A, got %q", p)
		}
	} else {
		t.Error("expected section D")
	}
}

func TestParseMarkdown_TitleExtraction(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantTitle string
	}{
		{
			name:      "title from frontmatter",
			content:   "---\ntitle: My Title\n---\n# Heading\n\nContent.",
			wantTitle: "My Title",
		},
		{
			name:      "title from h1",
			content:   "# My Heading\n\nContent.",
			wantTitle: "My Heading",
		},
		{
			name:      "no title",
			content:   "Just content.",
			wantTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseMarkdown(tt.content)
			if err != nil {
				t.Fatalf("ParseMarkdown() error = %v", err)
			}
			if doc.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", doc.Title, tt.wantTitle)
			}
		})
	}
}
