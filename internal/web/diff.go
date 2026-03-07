package web

import (
	"strings"

	"github.com/raphi011/knowhow/internal/web/templates/partials"
)

// computeDiff produces a simple line-by-line diff between old and new content.
// Uses a basic LCS-style approach for readable output.
func computeDiff(oldContent, newContent string) []partials.DiffLine {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Build LCS table
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	var lines []partials.DiffLine
	i, j := m, n
	var stack []partials.DiffLine

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			stack = append(stack, partials.DiffLine{Type: "context", Content: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			stack = append(stack, partials.DiffLine{Type: "add", Content: newLines[j-1]})
			j--
		} else {
			stack = append(stack, partials.DiffLine{Type: "remove", Content: oldLines[i-1]})
			i--
		}
	}

	// Reverse
	for k := len(stack) - 1; k >= 0; k-- {
		lines = append(lines, stack[k])
	}

	return lines
}
