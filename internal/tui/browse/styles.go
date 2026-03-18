package browse

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/tui/pick"
)

var (
	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	// Viewer styles
	headerBarStyle = lipgloss.NewStyle().
			Background(pick.PrimaryColor).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	footerBarStyle = lipgloss.NewStyle().
			Foreground(pick.MutedColor)
)
