package parser

import (
	"strings"
	"testing"
)

// parseSectionsHelper is a test helper that parses markdown and returns sections.
func parseSectionsHelper(t *testing.T, content string) []Section {
	t.Helper()
	return ParseMarkdown(content).Sections
}

func TestParseSections_BasicHeadings(t *testing.T) {
	content := "# Title\n\nIntro paragraph.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."

	sections := parseSectionsHelper(t, content)

	// Should have: preamble (empty), h1 Title, h2 Section A, h2 Section B
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

func TestParseSections_StartLineNumbers(t *testing.T) {
	content := "# Title\n\nIntro.\n\n## Second\n\nBody.\n\n### Third\n\nMore."
	// Line numbers (1-indexed):
	// 1: # Title
	// 5: ## Second
	// 9: ### Third

	sections := parseSectionsHelper(t, content)

	want := map[string]int{
		"Title":  1,
		"Second": 5,
		"Third":  9,
	}
	for _, s := range sections {
		if expected, ok := want[s.Heading]; ok {
			if s.Start != expected {
				t.Errorf("section %q: expected Start=%d, got %d", s.Heading, expected, s.Start)
			}
			delete(want, s.Heading)
		}
	}
	for heading := range want {
		t.Errorf("section %q not found in parsed sections", heading)
	}
}

func TestParseSections_CodeFenceWithHash(t *testing.T) {
	// This is the critical bug fix: # inside code fences should NOT create sections
	content := "## Setup\n\nInstructions here.\n\n```bash\n# This is a comment\necho hello\n# Another comment\n```\n\n## Next\n\nMore content."

	sections := parseSectionsHelper(t, content)

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

	sections := parseSectionsHelper(t, content)

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

	sections := parseSectionsHelper(t, content)

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

	sections := parseSectionsHelper(t, content)

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

	sections := parseSectionsHelper(t, content)

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

	sections := parseSectionsHelper(t, content)

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
	sections := parseSectionsHelper(t, "")

	// Empty doc produces a preamble with no content
	for _, s := range sections {
		if strings.TrimSpace(s.Content) != "" {
			t.Errorf("empty document should produce no content, got %q", s.Content)
		}
	}
}

func TestParseSections_HeadingPathHierarchy(t *testing.T) {
	content := "# A\n\nContent A.\n\n## B\n\nContent B.\n\n### C\n\nContent C.\n\n## D\n\nContent D."

	sections := parseSectionsHelper(t, content)

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

func TestParseSections_EmptyHeading(t *testing.T) {
	// Goldmark produces headings with zero lines for "# \n" — must not panic
	content := "# \n\n## Real Section\n\nContent here."

	sections := parseSectionsHelper(t, content)

	var found bool
	for _, s := range sections {
		if s.Heading == "Real Section" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Real Section' to be parsed after empty heading")
	}
}

func TestParseSections_FormattedHeading(t *testing.T) {
	content := "## Install `brew` and **setup**\n\nContent here."

	sections := parseSectionsHelper(t, content)

	for _, s := range sections {
		if s.Level == 2 {
			if !strings.Contains(s.Heading, "brew") || !strings.Contains(s.Heading, "setup") {
				t.Errorf("heading with inline formatting should preserve text, got %q", s.Heading)
			}
			return
		}
	}
	t.Error("expected h2 section")
}

func TestParseSections_IndentedCodeBlock(t *testing.T) {
	// Indented code blocks use 4-space indent (no fences)
	content := "## Example\n\n    func main() {\n        println(\"hello\")\n    }\n\nSome text after."

	sections := parseSectionsHelper(t, content)

	var found bool
	for _, s := range sections {
		if s.Heading == "Example" && strings.Contains(s.Content, "func main()") {
			found = true
		}
	}
	if !found {
		t.Error("indented code block content should be captured in section")
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
			doc := ParseMarkdown(tt.content)
			if doc.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", doc.Title, tt.wantTitle)
			}
		})
	}
}

// AST correctness tests: constructs inside code blocks must NOT be extracted.

func TestParseMarkdown_CheckboxInsideCodeBlock(t *testing.T) {
	content := "# Tasks\n\n- [ ] real task\n\n```markdown\n- [ ] fake task in code\n- [x] another fake\n```\n\n- [x] done task"

	doc := ParseMarkdown(content)

	if len(doc.Tasks) != 2 {
		t.Errorf("expected 2 tasks (ignoring code block), got %d", len(doc.Tasks))
		for _, task := range doc.Tasks {
			t.Logf("  task: %q status=%s line=%d", task.Text, task.Status, task.LineNumber)
		}
		return
	}
	if doc.Tasks[0].Text != "real task" {
		t.Errorf("first task should be 'real task', got %q", doc.Tasks[0].Text)
	}
	if doc.Tasks[1].Text != "done task" {
		t.Errorf("second task should be 'done task', got %q", doc.Tasks[1].Text)
	}
}

func TestParseMarkdown_WikiLinkInsideCodeBlock(t *testing.T) {
	content := "See [[real link]] for details.\n\n```\n[[fake link in code]]\n```\n\nAlso [[another link]]."

	doc := ParseMarkdown(content)

	if len(doc.WikiLinks) != 2 {
		t.Errorf("expected 2 wiki-links (ignoring code block), got %d: %v", len(doc.WikiLinks), doc.WikiLinks)
		return
	}
	if doc.WikiLinks[0] != "real link" {
		t.Errorf("first link should be 'real link', got %q", doc.WikiLinks[0])
	}
	if doc.WikiLinks[1] != "another link" {
		t.Errorf("second link should be 'another link', got %q", doc.WikiLinks[1])
	}
}

func TestParseMarkdown_HeadingInsideCodeBlock(t *testing.T) {
	content := "## Real Section\n\nContent.\n\n```bash\n# This is a bash comment\n## Not a heading\n```\n\n## Another Real Section\n\nMore content."

	doc := ParseMarkdown(content)

	var headings []string
	for _, s := range doc.Sections {
		if s.Level > 0 {
			headings = append(headings, s.Heading)
		}
	}

	if len(headings) != 2 {
		t.Errorf("expected 2 headings, got %d: %v", len(headings), headings)
		return
	}
	if headings[0] != "Real Section" || headings[1] != "Another Real Section" {
		t.Errorf("expected [Real Section, Another Real Section], got %v", headings)
	}
}

func TestParseMarkdown_LabelInsideCodeBlock(t *testing.T) {
	content := "This has #realtag in text.\n\n```\n#faketag in code\n```\n\nAlso #anothertag here."

	doc := ParseMarkdown(content)

	for _, label := range doc.InlineLabels {
		if label == "faketag" {
			t.Error("#faketag inside code block should not be extracted")
		}
	}

	found := map[string]bool{}
	for _, l := range doc.InlineLabels {
		found[l] = true
	}
	if !found["realtag"] {
		t.Error("expected #realtag to be extracted")
	}
	if !found["anothertag"] {
		t.Error("expected #anothertag to be extracted")
	}
}

func TestParseMarkdown_MentionInsideCodeBlock(t *testing.T) {
	content := "Hello @realuser, check this.\n\n```\n@fakeuser in code\n```"

	doc := ParseMarkdown(content)

	for _, m := range doc.Mentions {
		if m == "fakeuser" {
			t.Error("@fakeuser inside code block should not be extracted")
		}
	}
	found := false
	for _, m := range doc.Mentions {
		if m == "realuser" {
			found = true
		}
	}
	if !found {
		t.Error("expected @realuser to be extracted")
	}
}

func TestParseMarkdown_QueryBlockViaAST(t *testing.T) {
	content := "# Doc\n\n```know\nFROM /daily\nWHERE labels CONTAIN \"work\"\n```\n\nSome text.\n\n```python\nprint('hello')\n```"

	doc := ParseMarkdown(content)

	if len(doc.QueryBlocks) != 1 {
		t.Errorf("expected 1 query block, got %d", len(doc.QueryBlocks))
		return
	}
	if doc.QueryBlocks[0].Folder == nil || *doc.QueryBlocks[0].Folder != "/daily" {
		t.Error("query block should have FROM /daily")
	}
}

func TestParseMarkdown_WikiLinkWithAlias(t *testing.T) {
	content := "See [[target page|display text]] here."

	doc := ParseMarkdown(content)

	if len(doc.WikiLinks) != 1 {
		t.Errorf("expected 1 wiki-link, got %d: %v", len(doc.WikiLinks), doc.WikiLinks)
		return
	}
	// The target should be just the page part, not the alias
	if doc.WikiLinks[0] != "target page" {
		t.Errorf("wiki-link target should be 'target page', got %q", doc.WikiLinks[0])
	}
}

func TestParseMarkdown_FrontmatterViaGoldmarkMeta(t *testing.T) {
	content := "---\ntitle: Test Doc\nlabels:\n  - work\n  - urgent\ntype: note\n---\n\n# Content\n\nBody text."

	doc := ParseMarkdown(content)

	if doc.Title != "Test Doc" {
		t.Errorf("title should be 'Test Doc', got %q", doc.Title)
	}
	if doc.GetFrontmatterString("type") != "note" {
		t.Errorf("type should be 'note', got %q", doc.GetFrontmatterString("type"))
	}
	labels := doc.GetFrontmatterStringSlice("labels")
	if len(labels) != 2 || labels[0] != "work" || labels[1] != "urgent" {
		t.Errorf("labels should be [work, urgent], got %v", labels)
	}
}

func TestParseMarkdown_UnclosedFrontmatter(t *testing.T) {
	// Content starts with --- but has no closing --- delimiter.
	// ContentAfterFrontmatter strips the opening "---\n" to prevent
	// the frontmatter parser from consuming the entire document as a YAML block.
	content := "---\ntitle: Broken\n\n# Real Heading\n\n- [ ] a task"

	doc := ParseMarkdown(content)

	// Frontmatter should be empty (unclosed block can't be parsed)
	if len(doc.Frontmatter) != 0 {
		t.Errorf("expected empty frontmatter for unclosed block, got %v", doc.Frontmatter)
	}
	// Heading and tasks should still be extracted (graceful degradation)
	if doc.Title != "Real Heading" {
		t.Errorf("expected title 'Real Heading', got %q", doc.Title)
	}
	if len(doc.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(doc.Tasks))
	}
}

func TestParseMarkdown_ClosedFrontmatterPreservesStructure(t *testing.T) {
	// Contrast with unclosed: properly closed frontmatter should work fine
	content := "---\ntitle: Works\n---\n\n# Real Heading\n\n- [ ] a task"

	doc := ParseMarkdown(content)

	if doc.Title != "Works" {
		t.Errorf("expected title 'Works', got %q", doc.Title)
	}
	if len(doc.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(doc.Tasks))
	}
}

func TestParseMarkdown_QueryBlockAtDocumentStart(t *testing.T) {
	// Fenced code block at byte 0 — tests findFenceStart edge case.
	content := "```know\nFROM /daily\n```\n\nSome text."

	doc := ParseMarkdown(content)

	if len(doc.QueryBlocks) != 1 {
		t.Fatalf("expected 1 query block, got %d", len(doc.QueryBlocks))
	}
	if doc.QueryBlocks[0].Index != 0 {
		t.Errorf("expected query block index 0, got %d", doc.QueryBlocks[0].Index)
	}
}

func TestParseMarkdown_MultipleQueryBlocks(t *testing.T) {
	content := "# Doc\n\n```know\nFROM /daily\n```\n\nMiddle text.\n\n```know\nFROM /work\nWHERE labels CONTAIN \"urgent\"\n```"

	doc := ParseMarkdown(content)

	if len(doc.QueryBlocks) != 2 {
		t.Fatalf("expected 2 query blocks, got %d", len(doc.QueryBlocks))
	}
	if doc.QueryBlocks[0].Folder == nil || *doc.QueryBlocks[0].Folder != "/daily" {
		t.Error("first query block should have FROM /daily")
	}
	if doc.QueryBlocks[1].Folder == nil || *doc.QueryBlocks[1].Folder != "/work" {
		t.Error("second query block should have FROM /work")
	}
	if doc.QueryBlocks[0].Index >= doc.QueryBlocks[1].Index {
		t.Errorf("first block index (%d) should be less than second (%d)",
			doc.QueryBlocks[0].Index, doc.QueryBlocks[1].Index)
	}
}

func TestParseMarkdown_MultipleMentionsAndLabels(t *testing.T) {
	content := "Hello @alice and @bob, check #frontend #backend. @alice again."

	doc := ParseMarkdown(content)

	if len(doc.Mentions) != 2 {
		t.Errorf("expected 2 unique mentions, got %d: %v", len(doc.Mentions), doc.Mentions)
	}
	if len(doc.InlineLabels) != 2 {
		t.Errorf("expected 2 unique labels, got %d: %v", len(doc.InlineLabels), doc.InlineLabels)
	}
}

func TestParseMarkdown_InvalidQueryBlockSyntax(t *testing.T) {
	content := "```know\nGARBAGE LINE\n```"

	doc := ParseMarkdown(content)

	if len(doc.QueryBlocks) != 1 {
		t.Fatalf("expected 1 query block, got %d", len(doc.QueryBlocks))
	}
	if doc.QueryBlocks[0].Error == "" {
		t.Error("expected error for invalid query block syntax")
	}
}

func TestParseMarkdown_OnlyFrontmatter(t *testing.T) {
	content := "---\ntitle: Empty\n---\n"

	doc := ParseMarkdown(content)

	if doc.Title != "Empty" {
		t.Errorf("expected title 'Empty', got %q", doc.Title)
	}
	if len(doc.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(doc.Tasks))
	}
}

func TestParseMarkdown_ExternalLinks_MarkdownLink(t *testing.T) {
	content := "Check [GitHub](https://github.com/repo) and [Docs](https://docs.example.com/guide)."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 2 {
		t.Fatalf("expected 2 external links, got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
	if doc.ExternalLinks[0].URL != "https://github.com/repo" {
		t.Errorf("first link URL = %q, want https://github.com/repo", doc.ExternalLinks[0].URL)
	}
	if doc.ExternalLinks[0].LinkText != "GitHub" {
		t.Errorf("first link text = %q, want GitHub", doc.ExternalLinks[0].LinkText)
	}
	if doc.ExternalLinks[1].URL != "https://docs.example.com/guide" {
		t.Errorf("second link URL = %q, want https://docs.example.com/guide", doc.ExternalLinks[1].URL)
	}
}

func TestParseMarkdown_ExternalLinks_BareURL(t *testing.T) {
	content := "Visit https://example.com/page for more info."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 1 {
		t.Fatalf("expected 1 external link, got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
	if doc.ExternalLinks[0].URL != "https://example.com/page" {
		t.Errorf("link URL = %q, want https://example.com/page", doc.ExternalLinks[0].URL)
	}
	if doc.ExternalLinks[0].LinkText != "" {
		t.Errorf("bare URL should have empty link text, got %q", doc.ExternalLinks[0].LinkText)
	}
}

func TestParseMarkdown_ExternalLinks_InsideCodeBlock(t *testing.T) {
	content := "See [real](https://real.com) here.\n\n```\n[fake](https://fake.com)\nhttps://also-fake.com\n```\n\nEnd."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 1 {
		t.Fatalf("expected 1 external link (code block excluded), got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
	if doc.ExternalLinks[0].URL != "https://real.com" {
		t.Errorf("link URL = %q, want https://real.com", doc.ExternalLinks[0].URL)
	}
}

func TestParseMarkdown_ExternalLinks_RelativeIgnored(t *testing.T) {
	content := "See [local](/docs/foo) and [external](https://example.com)."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 1 {
		t.Fatalf("expected 1 external link (relative excluded), got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
	if doc.ExternalLinks[0].URL != "https://example.com" {
		t.Errorf("link URL = %q, want https://example.com", doc.ExternalLinks[0].URL)
	}
}

func TestParseMarkdown_ExternalLinks_Dedup(t *testing.T) {
	content := "Link [first](https://example.com) and [second](https://example.com) again."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 1 {
		t.Fatalf("expected 1 external link after dedup, got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
	if doc.ExternalLinks[0].LinkText != "first" {
		t.Errorf("dedup should keep first occurrence text, got %q", doc.ExternalLinks[0].LinkText)
	}
}

func TestParseMarkdown_ExternalLinks_HttpAndHttps(t *testing.T) {
	content := "Both [http](http://example.com) and [https](https://example.com/secure)."

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 2 {
		t.Fatalf("expected 2 external links, got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
}

func TestParseMarkdown_ExternalLinks_MailtoIgnored(t *testing.T) {
	content := "Email <user@example.com> and visit [site](https://example.com)."

	doc := ParseMarkdown(content)

	for _, l := range doc.ExternalLinks {
		if strings.Contains(l.URL, "mailto") || strings.Contains(l.URL, "@") {
			t.Errorf("mailto should not be extracted, got %q", l.URL)
		}
	}
	// Should have at least the https link
	found := false
	for _, l := range doc.ExternalLinks {
		if l.URL == "https://example.com" {
			found = true
		}
	}
	if !found {
		t.Error("expected https://example.com in external links")
	}
}

func TestParseMarkdown_ExternalLinks_InListItems(t *testing.T) {
	content := "# Resources\n\n- [Go docs](https://go.dev)\n- [Rust docs](https://doc.rust-lang.org)\n"

	doc := ParseMarkdown(content)

	if len(doc.ExternalLinks) != 2 {
		t.Fatalf("expected 2 external links from list items, got %d: %v", len(doc.ExternalLinks), doc.ExternalLinks)
	}
}

func TestParseMarkdown_ExternalLinks_Empty(t *testing.T) {
	content := "No links here, just text."

	doc := ParseMarkdown(content)

	if doc.ExternalLinks != nil {
		t.Errorf("expected nil external links, got %v", doc.ExternalLinks)
	}
}

func TestParseMarkdown_ListContentInSections(t *testing.T) {
	content := `# Title

## Section One

- First item
- Second item
- Third item

## Section Two

Some paragraph text.

- Item A
- Item B
`

	doc := ParseMarkdown(content)

	// Find sections by path instead of brittle indices.
	findSection := func(substr string) *Section {
		for i := range doc.Sections {
			if strings.Contains(doc.Sections[i].Path, substr) {
				return &doc.Sections[i]
			}
		}
		return nil
	}

	// Section with only list items must have non-empty content.
	sec1 := findSection("Section One")
	if sec1 == nil {
		t.Fatal("Section One not found")
	}
	if sec1.Content == "" {
		t.Error("section with list items has empty Content")
	}
	if !strings.Contains(sec1.Content, "First item") {
		t.Errorf("section content missing list item, got %q", sec1.Content)
	}
	if !strings.Contains(sec1.Content, "Third item") {
		t.Errorf("section content missing last list item, got %q", sec1.Content)
	}

	// Section with paragraph + list must include both.
	sec2 := findSection("Section Two")
	if sec2 == nil {
		t.Fatal("Section Two not found")
	}
	if !strings.Contains(sec2.Content, "paragraph text") {
		t.Errorf("section content missing paragraph, got %q", sec2.Content)
	}
	if !strings.Contains(sec2.Content, "Item A") {
		t.Errorf("section content missing list item, got %q", sec2.Content)
	}
}

func TestParseMarkdown_NestedListContentInSections(t *testing.T) {
	content := `## Config

- Option A
  - Sub-option 1
  - Sub-option 2
- Option B
`

	doc := ParseMarkdown(content)

	// Find the Config section.
	var configSection *Section
	for i := range doc.Sections {
		if strings.Contains(doc.Sections[i].Path, "Config") {
			configSection = &doc.Sections[i]
			break
		}
	}
	if configSection == nil {
		t.Fatal("Config section not found")
	}
	if configSection.Content == "" {
		t.Fatal("Config section has empty Content")
	}
	if !strings.Contains(configSection.Content, "Option A") {
		t.Errorf("missing top-level list item, got %q", configSection.Content)
	}
	if !strings.Contains(configSection.Content, "Sub-option 1") {
		t.Errorf("missing nested list item, got %q", configSection.Content)
	}
}

func TestParseMarkdown_BlockquoteContentInSections(t *testing.T) {
	content := `## Quote Section

> Some quoted text
> spanning multiple lines

## After Quote

Regular paragraph.
`

	doc := ParseMarkdown(content)

	var quoteSection *Section
	for i := range doc.Sections {
		if strings.Contains(doc.Sections[i].Path, "Quote Section") {
			quoteSection = &doc.Sections[i]
			break
		}
	}
	if quoteSection == nil {
		t.Fatal("Quote Section not found")
	}
	if quoteSection.Content == "" {
		t.Fatal("blockquote section has empty Content")
	}
	if !strings.Contains(quoteSection.Content, "Some quoted text") {
		t.Errorf("missing blockquote text, got %q", quoteSection.Content)
	}
	if !strings.Contains(quoteSection.Content, "spanning multiple lines") {
		t.Errorf("missing second blockquote line, got %q", quoteSection.Content)
	}
}
