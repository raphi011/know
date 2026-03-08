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
