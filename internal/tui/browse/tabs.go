package browse

import (
	"fmt"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/tui/pick"
)

// Tab identifies a browse tab.
type Tab int

const (
	// TabSearch is the default tab for full-text search.
	TabSearch Tab = iota
	// TabAllFiles shows all vault documents with fuzzy filtering.
	TabAllFiles
	// TabLinks shows saved web clips.
	TabLinks
	// TabBookmarks shows user-bookmarked files and folders.
	TabBookmarks
	// TabTags shows all labels with file counts.
	TabTags

	tabCount // sentinel — must be last
)

var (
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(pick.PrimaryColor).
			Bold(true).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(pick.MutedColor).
				Padding(0, 1)
)

func renderTabs(active Tab, linkCount, bookmarkCount, tagCount int) string {
	tabs := []struct {
		tab   Tab
		label string
	}{
		{TabSearch, "Search"},
		{TabAllFiles, "All Files"},
		{TabLinks, fmt.Sprintf("Links (%d)", linkCount)},
		{TabBookmarks, fmt.Sprintf("Bookmarks (%d)", bookmarkCount)},
		{TabTags, fmt.Sprintf("Tags (%d)", tagCount)},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == active {
			parts = append(parts, activeTabStyle.Render(t.label))
		} else {
			parts = append(parts, inactiveTabStyle.Render(t.label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
