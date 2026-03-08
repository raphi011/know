package parser

import (
	"strings"
	"testing"
)

func TestSectionOutline_Basic(t *testing.T) {
	doc, err := ParseMarkdown("Preamble.\n\n# Title\n\nIntro.\n\n## Setup\n\nSetup content.\n\n## Usage\n\nUsage content.\n\n## Setup\n\nSecond setup.")
	if err != nil {
		t.Fatal(err)
	}

	infos := SectionOutline(doc)

	if len(infos) != 5 {
		t.Fatalf("expected 5 sections, got %d", len(infos))
	}

	// Preamble
	if infos[0].Level != 0 || infos[0].Position != 0 {
		t.Errorf("preamble: level=%d pos=%d", infos[0].Level, infos[0].Position)
	}

	// First "Setup" should have position 0
	if infos[2].Heading != "Setup" || infos[2].Position != 0 {
		t.Errorf("first Setup: heading=%q pos=%d", infos[2].Heading, infos[2].Position)
	}

	// Second "Setup" should have position 1
	if infos[4].Heading != "Setup" || infos[4].Position != 1 {
		t.Errorf("second Setup: heading=%q pos=%d", infos[4].Heading, infos[4].Position)
	}
}

func TestApplySectionEdit_ReplacePreamble(t *testing.T) {
	content := "Old preamble.\n\n## Section A\n\nContent A."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "", Position: 0},
		Content:   "New preamble content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "New preamble content.") {
		t.Error("expected new preamble content")
	}
	if strings.Contains(result, "Old preamble.") {
		t.Error("old preamble should be gone")
	}
	if !strings.Contains(result, "## Section A") {
		t.Error("Section A heading should be preserved")
	}
	if !strings.Contains(result, "Content A.") {
		t.Error("Section A content should be preserved")
	}
}

func TestApplySectionEdit_ReplaceSection(t *testing.T) {
	content := "# Doc\n\nIntro.\n\n## Setup\n\nOld setup instructions.\n\n## Usage\n\nUsage info."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup"},
		Content:   "New setup instructions.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Setup") {
		t.Error("heading should be preserved")
	}
	if !strings.Contains(result, "New setup instructions.") {
		t.Error("new content should be present")
	}
	if strings.Contains(result, "Old setup instructions.") {
		t.Error("old content should be gone")
	}
	if !strings.Contains(result, "## Usage") {
		t.Error("Usage section should be preserved")
	}
	if !strings.Contains(result, "Usage info.") {
		t.Error("Usage content should be preserved")
	}
}

func TestApplySectionEdit_ReplaceDuplicateHeading(t *testing.T) {
	content := "## Setup\n\nFirst setup.\n\n## Other\n\nOther content.\n\n## Setup\n\nSecond setup."

	// Replace the second "Setup" (position=1)
	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup", Position: 1},
		Content:   "Replaced second setup.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "First setup.") {
		t.Error("first Setup content should be preserved")
	}
	if !strings.Contains(result, "Replaced second setup.") {
		t.Error("second Setup should have new content")
	}
	if strings.Contains(result, "Second setup.") {
		t.Error("old second Setup content should be gone")
	}
}

func TestApplySectionEdit_FrontmatterPreserved(t *testing.T) {
	content := "---\ntitle: My Doc\nlabels:\n  - test\n---\n# Title\n\nContent.\n\n## Section\n\nOld."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Section"},
		Content:   "New section content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result, "---\ntitle: My Doc\n") {
		t.Errorf("frontmatter should be preserved at start, got: %s", result[:min(60, len(result))])
	}
	if !strings.Contains(result, "labels:") {
		t.Error("frontmatter labels should be preserved")
	}
	if !strings.Contains(result, "New section content.") {
		t.Error("new content should be present")
	}
}

func TestApplySectionEdit_Delete(t *testing.T) {
	content := "## Keep\n\nKeep this.\n\n## Delete Me\n\nGone content.\n\n## Also Keep\n\nAlso keep this."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpDelete,
		Target:    &SectionAddress{Heading: "Delete Me"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "Delete Me") {
		t.Error("deleted section heading should be gone")
	}
	if strings.Contains(result, "Gone content.") {
		t.Error("deleted section content should be gone")
	}
	if !strings.Contains(result, "## Keep") {
		t.Error("Keep section should remain")
	}
	if !strings.Contains(result, "## Also Keep") {
		t.Error("Also Keep section should remain")
	}
}

func TestApplySectionEdit_DeleteIsShallow(t *testing.T) {
	content := "## Parent\n\nParent content.\n\n### Child\n\nChild content.\n\n## Sibling\n\nSibling content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpDelete,
		Target:    &SectionAddress{Heading: "Parent"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Shallow delete: only Parent's own content is removed, Child stays
	if strings.Contains(result, "Parent content.") {
		t.Error("parent content should be gone")
	}
	if !strings.Contains(result, "### Child") {
		t.Error("child section should remain (shallow delete)")
	}
	if !strings.Contains(result, "Child content.") {
		t.Error("child content should remain")
	}
}

func TestApplySectionEdit_Append(t *testing.T) {
	content := "## Existing\n\nExisting content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation:  OpAppend,
		Content:    "New section body.",
		NewHeading: "Appended",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Existing") {
		t.Error("existing section should remain")
	}
	if !strings.Contains(result, "## Appended") {
		t.Error("appended heading should be present")
	}
	if !strings.Contains(result, "New section body.") {
		t.Error("appended content should be present")
	}

	// Appended section should come after existing
	existingIdx := strings.Index(result, "## Existing")
	appendedIdx := strings.Index(result, "## Appended")
	if appendedIdx <= existingIdx {
		t.Error("appended section should come after existing section")
	}
}

func TestApplySectionEdit_InsertAfter(t *testing.T) {
	content := "## First\n\nFirst content.\n\n## Third\n\nThird content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation:  OpInsertAfter,
		Target:     &SectionAddress{Heading: "First"},
		Content:    "Second content.",
		NewHeading: "Second",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstIdx := strings.Index(result, "## First")
	secondIdx := strings.Index(result, "## Second")
	thirdIdx := strings.Index(result, "## Third")

	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Fatalf("all sections should be present, got:\n%s", result)
	}

	if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
		t.Error("sections should be in order: First, Second, Third")
	}
}

func TestApplySectionEdit_InsertBefore(t *testing.T) {
	content := "## First\n\nFirst content.\n\n## Third\n\nThird content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation:  OpInsertBefore,
		Target:     &SectionAddress{Heading: "Third"},
		Content:    "Second content.",
		NewHeading: "Second",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstIdx := strings.Index(result, "## First")
	secondIdx := strings.Index(result, "## Second")
	thirdIdx := strings.Index(result, "## Third")

	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Fatalf("all sections should be present, got:\n%s", result)
	}

	if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
		t.Error("sections should be in order: First, Second, Third")
	}
}

func TestApplySectionEdit_AppendWithFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\n---\n## Existing\n\nContent."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation:  OpAppend,
		Content:    "New body.",
		NewHeading: "New Section",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result, "---\ntitle: Test\n---\n") {
		t.Error("frontmatter should be preserved")
	}
	if !strings.Contains(result, "## New Section") {
		t.Error("new section should be present")
	}
}

func TestApplySectionEdit_OnlyPreambleDocument(t *testing.T) {
	content := "Just plain text.\n\nAnother paragraph."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: ""},
		Content:   "Replaced content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Replaced content.") {
		t.Error("preamble should be replaced")
	}
	if strings.Contains(result, "Just plain text.") {
		t.Error("old content should be gone")
	}
}

func TestApplySectionEdit_ErrorSectionNotFound(t *testing.T) {
	content := "## Existing\n\nContent."

	_, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Nonexistent"},
		Content:   "whatever",
	})
	if err == nil {
		t.Error("expected error for nonexistent section")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %s", err)
	}
}

func TestApplySectionEdit_ErrorPositionOutOfRange(t *testing.T) {
	content := "## Setup\n\nContent."

	_, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup", Position: 5},
		Content:   "whatever",
	})
	if err == nil {
		t.Error("expected error for out-of-range position")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error should mention out of range, got: %s", err)
	}
}

func TestApplySectionEdit_ErrorInvalidOperation(t *testing.T) {
	_, err := ApplySectionEdit("content", SectionEdit{
		Operation: "invalid_op",
		Target:    &SectionAddress{Heading: "X"},
	})
	if err == nil {
		t.Error("expected error for invalid operation")
	}
}

func TestApplySectionEdit_ErrorMissingTarget(t *testing.T) {
	_, err := ApplySectionEdit("content", SectionEdit{
		Operation: OpReplace,
		Target:    nil,
		Content:   "x",
	})
	if err == nil {
		t.Error("expected error for missing target on replace")
	}
}

func TestApplySectionEdit_ErrorInvalidLevel(t *testing.T) {
	_, err := ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpInsertAfter,
		Target:     &SectionAddress{Heading: "X"},
		NewHeading: "Y",
		NewLevel:   0,
		Content:    "x",
	})
	if err == nil {
		t.Error("expected error for level 0")
	}

	_, err = ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpAppend,
		NewHeading: "Y",
		NewLevel:   7,
		Content:    "x",
	})
	if err == nil {
		t.Error("expected error for level 7")
	}
}

func TestApplySectionEdit_ReplacePreservesHeadingFormatting(t *testing.T) {
	content := "## Setup Instructions\n\nOld content.\n\n## Next\n\nNext content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup Instructions"},
		Content:   "New content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The original heading line should be preserved exactly
	if !strings.Contains(result, "## Setup Instructions") {
		t.Error("original heading should be preserved exactly")
	}
}

func TestApplySectionEdit_DeleteLastSection(t *testing.T) {
	content := "## First\n\nFirst content.\n\n## Last\n\nLast content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpDelete,
		Target:    &SectionAddress{Heading: "Last"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## First") {
		t.Error("First section should remain")
	}
	if strings.Contains(result, "## Last") {
		t.Error("Last section should be gone")
	}
}

func TestApplySectionEdit_ReplaceWithEmptyContent(t *testing.T) {
	content := "## Section\n\nOld content.\n\n## Next\n\nNext content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Section"},
		Content:   "",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Section") {
		t.Error("heading should be preserved even with empty content")
	}
	if strings.Contains(result, "Old content.") {
		t.Error("old content should be gone")
	}
	if !strings.Contains(result, "## Next") {
		t.Error("next section should remain")
	}
}

func TestApplySectionEdit_HeadingsInsideCodeFence(t *testing.T) {
	content := "## Real Section\n\nSome text.\n\n```markdown\n## Fake Heading\n\nThis is inside a code fence.\n```\n\n## Another Real\n\nMore text."

	// The ## inside the code fence is content of "Real Section", not a separate section.
	// Goldmark's AST correctly ignores headings inside fenced code blocks.
	// Verify by checking the section outline only has the real headings.
	doc, err := ParseMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}

	outline := SectionOutline(doc)
	headings := make([]string, len(outline))
	for i, info := range outline {
		headings[i] = info.Heading
	}

	// Should only have: preamble (empty heading), "Real Section", "Another Real"
	// "Fake Heading" inside the code fence must NOT appear as a section.
	for _, info := range outline {
		if info.Heading == "Fake Heading" {
			t.Error("heading inside code fence should not be parsed as a section")
		}
	}

	// Replacing "Another Real" should preserve the code fence in "Real Section"
	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Another Real"},
		Content:   "Replaced.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## Fake Heading") {
		t.Error("code fence content should be preserved when replacing a different section")
	}
	if !strings.Contains(result, "Replaced.") {
		t.Error("replacement content should be present")
	}
	if !strings.Contains(result, "## Real Section") {
		t.Error("Real Section should be preserved")
	}
}

func TestApplySectionEdit_FrontmatterContainsSameTextAsContent(t *testing.T) {
	// Frontmatter contains text that matches the start of the content body
	content := "---\ntitle: \"## Setup\"\ndesc: \"Setup content.\"\n---\n## Setup\n\nSetup content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup"},
		Content:   "New content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result, "---\ntitle: \"## Setup\"\n") {
		t.Errorf("frontmatter should be preserved exactly, got: %s", result[:min(60, len(result))])
	}
	if !strings.Contains(result, "New content.") {
		t.Error("new content should be present")
	}
}

func TestApplySectionEdit_ContentWithCodeFencesInBody(t *testing.T) {
	content := "## Config\n\nHere is a config:\n\n```yaml\nkey: value\n```\n\n## Next\n\nMore."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Config"},
		Content:   "Updated:\n\n```json\n{\"a\": 1}\n```",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "```json") {
		t.Error("new code fence should be present")
	}
	if strings.Contains(result, "```yaml") {
		t.Error("old code fence should be gone")
	}
	if !strings.Contains(result, "## Next") {
		t.Error("Next section should be preserved")
	}
}

func TestApplySectionEdit_InsertBeforeFirstSection(t *testing.T) {
	content := "## Only Section\n\nContent."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation:  OpInsertBefore,
		Target:     &SectionAddress{Heading: "Only Section"},
		Content:    "Preamble-like content.",
		NewHeading: "Intro",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	introIdx := strings.Index(result, "## Intro")
	onlyIdx := strings.Index(result, "## Only Section")

	if introIdx < 0 || onlyIdx < 0 {
		t.Fatalf("both sections should be present, got:\n%s", result)
	}
	if introIdx >= onlyIdx {
		t.Error("Intro should come before Only Section")
	}
}

func TestApplySectionEdit_DeletePreamble(t *testing.T) {
	content := "This is preamble text.\n\nMore preamble.\n\n## Section A\n\nContent A."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpDelete,
		Target:    &SectionAddress{Heading: ""},
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "This is preamble text.") {
		t.Error("preamble should be deleted")
	}
	if strings.Contains(result, "More preamble.") {
		t.Error("preamble content should be deleted")
	}
	if !strings.Contains(result, "## Section A") {
		t.Error("Section A should be preserved")
	}
	if !strings.Contains(result, "Content A.") {
		t.Error("Section A content should be preserved")
	}
}

func TestApplySectionEdit_AdjacentHeadingsNoContent(t *testing.T) {
	content := "## First\n## Second\n## Third\n\nOnly third has content."

	// Replace the middle section (which has no content body)
	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Second"},
		Content:   "Now has content.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## First") {
		t.Error("First heading should be preserved")
	}
	if !strings.Contains(result, "## Second") {
		t.Error("Second heading should be preserved")
	}
	if !strings.Contains(result, "Now has content.") {
		t.Error("replacement content should be present")
	}
	if !strings.Contains(result, "## Third") {
		t.Error("Third heading should be preserved")
	}
	if !strings.Contains(result, "Only third has content.") {
		t.Error("Third section content should be preserved")
	}
}

func TestApplySectionEdit_AppendToEmptyDocument(t *testing.T) {
	result, err := ApplySectionEdit("", SectionEdit{
		Operation:  OpAppend,
		Content:    "First content.",
		NewHeading: "First",
		NewLevel:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "## First") {
		t.Error("heading should be present")
	}
	if !strings.Contains(result, "First content.") {
		t.Error("content should be present")
	}
}

func TestApplySectionEdit_ErrorMissingNewHeading(t *testing.T) {
	_, err := ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpInsertAfter,
		Target:     &SectionAddress{Heading: "X"},
		NewHeading: "",
		NewLevel:   2,
		Content:    "body",
	})
	if err == nil {
		t.Error("expected error for empty new_heading on insert")
	}

	_, err = ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpAppend,
		NewHeading: "",
		NewLevel:   2,
		Content:    "body",
	})
	if err == nil {
		t.Error("expected error for empty new_heading on append")
	}
}

func TestApplySectionEdit_ReplaceIncludesChildSections(t *testing.T) {
	content := "## Setup\n\nIntro text.\n\n### Prerequisites\n\nInstall Go.\n\n### Configuration\n\nConfig steps.\n\n## Usage\n\nUsage info."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup"},
		Content:   "New setup content with everything included.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The replacement should include all child sections
	if !strings.Contains(result, "## Setup") {
		t.Error("heading should be preserved")
	}
	if !strings.Contains(result, "New setup content with everything included.") {
		t.Error("new content should be present")
	}
	if strings.Contains(result, "Prerequisites") {
		t.Error("child section Prerequisites should be replaced (deep replace)")
	}
	if strings.Contains(result, "Install Go.") {
		t.Error("child section content should be replaced")
	}
	if strings.Contains(result, "Configuration") {
		t.Error("child section Configuration should be replaced (deep replace)")
	}
	if strings.Contains(result, "Config steps.") {
		t.Error("child section content should be replaced")
	}
	// Sibling section should be preserved
	if !strings.Contains(result, "## Usage") {
		t.Error("sibling Usage section should be preserved")
	}
	if !strings.Contains(result, "Usage info.") {
		t.Error("Usage content should be preserved")
	}
}

func TestApplySectionEdit_ReplaceDeepNestedChildren(t *testing.T) {
	content := "# Top\n\nTop content.\n\n## Mid\n\nMid content.\n\n### Sub\n\nSub content.\n\n# Another Top\n\nAnother content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Mid"},
		Content:   "Replaced mid.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "Sub content.") {
		t.Error("nested child ### Sub should be replaced with parent ## Mid")
	}
	if !strings.Contains(result, "Replaced mid.") {
		t.Error("new content should be present")
	}
	if !strings.Contains(result, "# Another Top") {
		t.Error("sibling # Another Top should be preserved")
	}
}

func TestApplySectionEdit_ErrorMissingContentForInsert(t *testing.T) {
	_, err := ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpInsertAfter,
		Target:     &SectionAddress{Heading: "X"},
		NewHeading: "Y",
		NewLevel:   2,
		Content:    "",
	})
	if err == nil {
		t.Error("expected error for empty content on insert_after")
	}

	_, err = ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpInsertBefore,
		Target:     &SectionAddress{Heading: "X"},
		NewHeading: "Y",
		NewLevel:   2,
		Content:    "",
	})
	if err == nil {
		t.Error("expected error for empty content on insert_before")
	}
}

func TestApplySectionEdit_ErrorMissingContentForAppend(t *testing.T) {
	_, err := ApplySectionEdit("## X\n\nContent.", SectionEdit{
		Operation:  OpAppend,
		NewHeading: "Y",
		NewLevel:   2,
		Content:    "",
	})
	if err == nil {
		t.Error("expected error for empty content on append")
	}
}

func TestApplySectionEdit_RoundTrip(t *testing.T) {
	// Verify structural integrity: parse → edit → parse should produce valid doc
	content := "## Setup\n\nSetup content.\n\n## Usage\n\nUsage content."

	result, err := ApplySectionEdit(content, SectionEdit{
		Operation: OpReplace,
		Target:    &SectionAddress{Heading: "Setup"},
		Content:   "New setup.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Re-parse should succeed and have the right sections
	doc, err := ParseMarkdown(result)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}

	outline := SectionOutline(doc)
	headings := make([]string, 0, len(outline))
	for _, info := range outline {
		if info.Level > 0 {
			headings = append(headings, info.Heading)
		}
	}

	if len(headings) != 2 || headings[0] != "Setup" || headings[1] != "Usage" {
		t.Errorf("expected [Setup, Usage], got %v", headings)
	}
}

func TestBuildSectionEdit(t *testing.T) {
	heading := "Setup"
	pos := 1
	content := "body text"
	newHeading := "New"
	newLevel := 3

	edit := BuildSectionEdit(SectionEditArgs{
		Operation:  "replace",
		Heading:    &heading,
		Position:   &pos,
		Content:    &content,
		NewHeading: &newHeading,
		NewLevel:   &newLevel,
	})

	if edit.Operation != OpReplace {
		t.Errorf("expected replace, got %s", edit.Operation)
	}
	if edit.Target == nil || edit.Target.Heading != "Setup" || edit.Target.Position != 1 {
		t.Error("target should be set from heading/position")
	}
	if edit.Content != "body text" {
		t.Error("content should be set")
	}
	if edit.NewHeading != "New" || edit.NewLevel != 3 {
		t.Error("new heading/level should be set")
	}
}

func TestBuildSectionEdit_NilFields(t *testing.T) {
	edit := BuildSectionEdit(SectionEditArgs{
		Operation: "append",
	})

	if edit.Target != nil {
		t.Error("target should be nil when heading is nil")
	}
	if edit.Content != "" {
		t.Error("content should be empty when nil")
	}
	if edit.NewHeading != "" || edit.NewLevel != 0 {
		t.Error("new heading/level should be zero values")
	}
}
