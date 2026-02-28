package review

import (
	"strconv"
	"testing"
)

func TestComputeHunks_NoChanges(t *testing.T) {
	text := "line 1\nline 2\nline 3\n"
	hunks, err := ComputeHunks(text, text, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 0 {
		t.Fatalf("expected 0 hunks, got %d", len(hunks))
	}
}

func TestComputeHunks_SimpleChange(t *testing.T) {
	original := "line 1\nline 2\nline 3\n"
	proposed := "line 1\nline TWO\nline 3\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	h := hunks[0]
	if h.Index != 0 {
		t.Errorf("expected index 0, got %d", h.Index)
	}

	// Should have context + delete + add + context
	var adds, deletes, ctx int
	for _, line := range h.Lines {
		switch line.Type {
		case DiffAdd:
			adds++
		case DiffDelete:
			deletes++
		case DiffContext:
			ctx++
		}
	}
	if adds != 1 || deletes != 1 {
		t.Errorf("expected 1 add + 1 delete, got %d adds + %d deletes", adds, deletes)
	}
	if ctx != 2 { // line 1 and line 3 as context
		t.Errorf("expected 2 context lines, got %d", ctx)
	}
}

func TestComputeHunks_MultipleHunks(t *testing.T) {
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, lineN(i))
	}
	original := joinLines(lines)

	modified := make([]string, len(lines))
	copy(modified, lines)
	modified[1] = "CHANGED 2\n"  // change line 2
	modified[17] = "CHANGED 18\n" // change line 18
	proposed := joinLines(modified)

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) < 2 {
		t.Fatalf("expected at least 2 hunks (changes far apart), got %d", len(hunks))
	}
}

func TestComputeHunks_AddedLines(t *testing.T) {
	original := "line 1\nline 2\n"
	proposed := "line 1\nline 2\nline 3\nline 4\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	stats := ComputeStats(hunks)
	if stats.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", stats.Additions)
	}
	if stats.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", stats.Deletions)
	}
}

func TestComputeHunks_DeletedLines(t *testing.T) {
	original := "line 1\nline 2\nline 3\nline 4\n"
	proposed := "line 1\nline 4\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	stats := ComputeStats(hunks)
	if stats.Additions != 0 {
		t.Errorf("expected 0 additions, got %d", stats.Additions)
	}
	if stats.Deletions != 2 {
		t.Errorf("expected 2 deletions, got %d", stats.Deletions)
	}
}

func TestHunkHeader(t *testing.T) {
	h := Hunk{OldStart: 10, OldLines: 5, NewStart: 10, NewLines: 8}
	expected := "@@ -10,5 +10,8 @@"
	if h.Header() != expected {
		t.Errorf("expected %q, got %q", expected, h.Header())
	}
}

func TestApplyHunks_AcceptAll(t *testing.T) {
	original := "line 1\nline 2\nline 3\n"
	proposed := "line 1\nline TWO\nline 3\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}
}

func TestApplyHunks_RejectAll(t *testing.T) {
	original := "line 1\nline 2\nline 3\n"
	proposed := "line 1\nline TWO\nline 3\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	result, err := ApplyHunks(original, hunks, nil)
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != original {
		t.Errorf("expected original %q, got %q", original, result)
	}
}

func TestApplyHunks_PartialAccept(t *testing.T) {
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, lineN(i))
	}
	original := joinLines(lines)

	modified := make([]string, len(lines))
	copy(modified, lines)
	modified[1] = "CHANGED 2\n"
	modified[17] = "CHANGED 18\n"
	proposed := joinLines(modified)

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}
	if len(hunks) < 2 {
		t.Fatalf("expected at least 2 hunks, got %d", len(hunks))
	}

	// Accept only the first hunk (line 2 change), reject second (line 18 change)
	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}

	// Result should have CHANGED 2 but original line 18
	expected := make([]string, len(lines))
	copy(expected, lines)
	expected[1] = "CHANGED 2\n"
	expectedStr := joinLines(expected)

	if result != expectedStr {
		t.Errorf("partial accept mismatch.\nexpected: %q\ngot:      %q", expectedStr, result)
	}
}

func TestApplyHunks_InvalidIndex(t *testing.T) {
	original := "line 1\nline 2\n"
	proposed := "line 1\nline TWO\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	_, err = ApplyHunks(original, hunks, []int{5})
	if err == nil {
		t.Fatal("expected error for invalid hunk index")
	}
}

func TestApplyHunks_NoHunks(t *testing.T) {
	original := "line 1\nline 2\n"
	result, err := ApplyHunks(original, nil, nil)
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != original {
		t.Errorf("expected original, got %q", result)
	}
}

func TestComputeStats(t *testing.T) {
	hunks := []Hunk{
		{
			Lines: []DiffLine{
				{Type: DiffContext},
				{Type: DiffAdd},
				{Type: DiffAdd},
				{Type: DiffDelete},
			},
		},
		{
			Lines: []DiffLine{
				{Type: DiffAdd},
				{Type: DiffDelete},
				{Type: DiffDelete},
			},
		},
	}

	stats := ComputeStats(hunks)
	if stats.Additions != 3 {
		t.Errorf("expected 3 additions, got %d", stats.Additions)
	}
	if stats.Deletions != 3 {
		t.Errorf("expected 3 deletions, got %d", stats.Deletions)
	}
	if stats.HunksCount != 2 {
		t.Errorf("expected 2 hunks, got %d", stats.HunksCount)
	}
}

func TestApplyHunks_InsertionOnly(t *testing.T) {
	original := "line 1\nline 2\nline 3\n"
	proposed := "line 1\nline 2\nnew line A\nnew line B\nline 3\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	// Accept the insertion
	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}

	// Reject the insertion
	result, err = ApplyHunks(original, hunks, nil)
	if err != nil {
		t.Fatalf("ApplyHunks reject: %v", err)
	}
	if result != original {
		t.Errorf("expected original %q, got %q", original, result)
	}
}

func TestApplyHunks_DeletionOnly(t *testing.T) {
	original := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	proposed := "line 1\nline 5\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	// Accept the deletion
	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}

	// Reject the deletion
	result, err = ApplyHunks(original, hunks, nil)
	if err != nil {
		t.Fatalf("ApplyHunks reject: %v", err)
	}
	if result != original {
		t.Errorf("expected original %q, got %q", original, result)
	}
}

func TestApplyHunks_MixedInsertionAndDeletion(t *testing.T) {
	// Two hunks far apart: first is insertion-only, second is deletion-only
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, lineN(i))
	}
	original := joinLines(lines)

	// Insert after line 2, delete lines 17-18
	modified := make([]string, 0, 20)
	modified = append(modified, lines[:2]...)
	modified = append(modified, "INSERTED\n")
	modified = append(modified, lines[2:16]...)
	modified = append(modified, lines[18:]...)
	proposed := joinLines(modified)

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}
	if len(hunks) < 2 {
		t.Fatalf("expected at least 2 hunks, got %d", len(hunks))
	}

	// Accept only the insertion hunk (first)
	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	// Should have the inserted line but keep the deleted lines
	expected := make([]string, 0, 21)
	expected = append(expected, lines[:2]...)
	expected = append(expected, "INSERTED\n")
	expected = append(expected, lines[2:]...)
	if result != joinLines(expected) {
		t.Errorf("partial accept mismatch.\nexpected: %q\ngot:      %q", joinLines(expected), result)
	}
}

func TestApplyHunks_EmptyOriginal(t *testing.T) {
	original := ""
	proposed := "new line 1\nnew line 2\n"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}
}

func TestApplyHunks_EmptyProposed(t *testing.T) {
	original := "line 1\nline 2\n"
	proposed := ""

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}
}

func TestSplitLines_NoTrailingNewline(t *testing.T) {
	lines := splitLines("line 1\nline 2")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line 1\n" {
		t.Errorf("expected %q, got %q", "line 1\n", lines[0])
	}
	if lines[1] != "line 2" {
		t.Errorf("expected %q, got %q", "line 2", lines[1])
	}
}

func TestSplitLines_Empty(t *testing.T) {
	lines := splitLines("")
	if lines != nil {
		t.Errorf("expected nil, got %v", lines)
	}
}

func TestApplyHunks_NoTrailingNewline(t *testing.T) {
	original := "line 1\nline 2\nline 3"
	proposed := "line 1\nchanged\nline 3"

	hunks, err := ComputeHunks(original, proposed, 3)
	if err != nil {
		t.Fatalf("ComputeHunks: %v", err)
	}

	result, err := ApplyHunks(original, hunks, []int{0})
	if err != nil {
		t.Fatalf("ApplyHunks: %v", err)
	}
	if result != proposed {
		t.Errorf("expected %q, got %q", proposed, result)
	}
}

// Helpers

func lineN(n int) string {
	return "line " + strconv.Itoa(n) + "\n"
}

func joinLines(lines []string) string {
	var b []byte
	for _, l := range lines {
		b = append(b, l...)
	}
	return string(b)
}
