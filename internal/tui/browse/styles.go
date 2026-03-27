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

	// Audio player styles
	playerStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")).
				Bold(true)

	playerPausedStyle = lipgloss.NewStyle().
				Foreground(pick.MutedColor).
				Bold(true)

	playerHotkeyKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")).
				Bold(true)

	playerHotkeyStyle = lipgloss.NewStyle().
				Foreground(pick.MutedColor)

	separatorStyle = lipgloss.NewStyle().
			Foreground(pick.MutedColor)

	// Hotkey rendering for tab footers.
	hotkeyKeyStyle = lipgloss.NewStyle().
			Foreground(pick.SecondaryColor).
			Bold(true)

	hotkeyDescStyle = lipgloss.NewStyle().
			Foreground(pick.MutedColor)
)
