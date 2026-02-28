package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// DiffLineType classifies a line in a diff hunk.
type DiffLineType string

const (
	DiffContext DiffLineType = "context"
	DiffAdd     DiffLineType = "add"
	DiffDelete  DiffLineType = "delete"
)

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	Type      DiffLineType `json:"type"`
	Content   string       `json:"content"`
	OldLineNo *int         `json:"old_line_no,omitempty"`
	NewLineNo *int         `json:"new_line_no,omitempty"`
}

// Hunk represents a contiguous group of changes in a unified diff.
type Hunk struct {
	Index    int        `json:"index"`
	OldStart int        `json:"old_start"`
	OldLines int        `json:"old_lines"`
	NewStart int        `json:"new_start"`
	NewLines int        `json:"new_lines"`
	Lines    []DiffLine `json:"lines"`
}

// Header returns the unified diff header for this hunk, e.g. "@@ -10,5 +10,8 @@".
func (h Hunk) Header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
}

// DiffStats summarizes the changes across all hunks.
type DiffStats struct {
	Additions  int `json:"additions"`
	Deletions  int `json:"deletions"`
	HunksCount int `json:"hunks_count"`
}

// splitLines splits text into lines, preserving the behavior expected by difflib.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	// Remove trailing empty string from SplitAfter if input ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ComputeHunks computes unified diff hunks between original and proposed text.
// contextLines controls how many unchanged lines surround each change group.
func ComputeHunks(original, proposed string, contextLines int) ([]Hunk, error) {
	origLines := splitLines(original)
	propLines := splitLines(proposed)

	matcher := difflib.NewMatcher(origLines, propLines)
	groups := matcher.GetGroupedOpCodes(contextLines)

	var hunks []Hunk
	for i, group := range groups {
		if len(group) == 0 {
			continue
		}

		// Determine hunk boundaries from the first and last opcode in the group
		first := group[0]
		last := group[len(group)-1]

		oldStart := first.I1 + 1 // 1-based
		oldEnd := last.I2
		newStart := first.J1 + 1
		newEnd := last.J2

		hunk := Hunk{
			Index:    i,
			OldStart: oldStart,
			OldLines: oldEnd - first.I1,
			NewStart: newStart,
			NewLines: newEnd - first.J1,
		}

		oldLineNo := first.I1 + 1
		newLineNo := first.J1 + 1

		for _, code := range group {
			switch code.Tag {
			case 'e': // equal
				for j := code.I1; j < code.I2; j++ {
					oln := oldLineNo
					nln := newLineNo
					hunk.Lines = append(hunk.Lines, DiffLine{
						Type:      DiffContext,
						Content:   origLines[j],
						OldLineNo: &oln,
						NewLineNo: &nln,
					})
					oldLineNo++
					newLineNo++
				}
			case 'r': // replace: show deletes then adds
				for j := code.I1; j < code.I2; j++ {
					oln := oldLineNo
					hunk.Lines = append(hunk.Lines, DiffLine{
						Type:      DiffDelete,
						Content:   origLines[j],
						OldLineNo: &oln,
					})
					oldLineNo++
				}
				for j := code.J1; j < code.J2; j++ {
					nln := newLineNo
					hunk.Lines = append(hunk.Lines, DiffLine{
						Type:      DiffAdd,
						Content:   propLines[j],
						NewLineNo: &nln,
					})
					newLineNo++
				}
			case 'd': // delete
				for j := code.I1; j < code.I2; j++ {
					oln := oldLineNo
					hunk.Lines = append(hunk.Lines, DiffLine{
						Type:      DiffDelete,
						Content:   origLines[j],
						OldLineNo: &oln,
					})
					oldLineNo++
				}
			case 'i': // insert
				for j := code.J1; j < code.J2; j++ {
					nln := newLineNo
					hunk.Lines = append(hunk.Lines, DiffLine{
						Type:      DiffAdd,
						Content:   propLines[j],
						NewLineNo: &nln,
					})
					newLineNo++
				}
			}
		}

		hunks = append(hunks, hunk)
	}

	return hunks, nil
}

// ApplyHunks applies only the accepted hunks to the original text.
// acceptedIndexes contains the hunk Index values to accept.
// Returns the merged result.
func ApplyHunks(original string, hunks []Hunk, acceptedIndexes []int) (string, error) {
	if len(hunks) == 0 {
		return original, nil
	}

	// Build set of accepted hunk indexes
	accepted := make(map[int]bool, len(acceptedIndexes))
	for _, idx := range acceptedIndexes {
		if idx < 0 || idx >= len(hunks) {
			return "", fmt.Errorf("invalid hunk index: %d (have %d hunks)", idx, len(hunks))
		}
		accepted[idx] = true
	}

	origLines := splitLines(original)

	// Sort hunks by OldStart to process in order
	sorted := make([]Hunk, len(hunks))
	copy(sorted, hunks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OldStart < sorted[j].OldStart
	})

	var result []string
	origPos := 0 // 0-based index into origLines

	for _, hunk := range sorted {
		hunkStart := hunk.OldStart - 1 // convert to 0-based

		// Copy unchanged lines before this hunk
		if hunkStart > origPos {
			result = append(result, origLines[origPos:hunkStart]...)
		}

		if accepted[hunk.Index] {
			// Apply this hunk: emit context and added lines, skip deleted lines
			for _, line := range hunk.Lines {
				switch line.Type {
				case DiffContext:
					result = append(result, line.Content)
				case DiffAdd:
					result = append(result, line.Content)
				case DiffDelete:
					// skip
				}
			}
		} else {
			// Reject this hunk: emit context and deleted lines (original), skip added lines
			for _, line := range hunk.Lines {
				switch line.Type {
				case DiffContext:
					result = append(result, line.Content)
				case DiffDelete:
					result = append(result, line.Content)
				case DiffAdd:
					// skip
				}
			}
		}

		origPos = hunkStart + hunk.OldLines
	}

	// Copy remaining lines after last hunk
	if origPos < len(origLines) {
		result = append(result, origLines[origPos:]...)
	}

	return strings.Join(result, ""), nil
}

// ComputeStats returns summary statistics for a set of hunks.
func ComputeStats(hunks []Hunk) DiffStats {
	stats := DiffStats{HunksCount: len(hunks)}
	for _, h := range hunks {
		for _, line := range h.Lines {
			switch line.Type {
			case DiffAdd:
				stats.Additions++
			case DiffDelete:
				stats.Deletions++
			}
		}
	}
	return stats
}
