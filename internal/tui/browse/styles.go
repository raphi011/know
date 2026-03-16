package browse

import lipgloss "charm.land/lipgloss/v2"

var (
	// Colors — reuse the violet/gray palette from the agent TUI.
	primaryColor = lipgloss.Color("#7C3AED")
	mutedColor   = lipgloss.Color("#9CA3AF")
	matchColor   = lipgloss.Color("#F59E0B") // amber for fuzzy match highlights

	// Finder styles
	promptStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	countStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	selectedStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	matchStyle = lipgloss.NewStyle().
			Foreground(matchColor).
			Bold(true)

	titleStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	// Viewer styles
	headerBarStyle = lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	footerBarStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
)
