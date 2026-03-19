package browse

import (
	"fmt"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/tui/pick"
)

// Tab identifies a browse tab.
type Tab int

const (
	// TabAllFiles is the default tab showing all vault documents.
	TabAllFiles Tab = iota
	// TabLinks shows saved web clips.
	TabLinks
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

func renderTabs(active Tab, linkCount int) string {
	allFiles := "All Files"
	links := fmt.Sprintf("Links (%d)", linkCount)

	if active == TabAllFiles {
		return activeTabStyle.Render(allFiles) + " " + inactiveTabStyle.Render(links)
	}
	return inactiveTabStyle.Render(allFiles) + " " + activeTabStyle.Render(links)
}
