// Package pick provides a standalone fuzzy file picker TUI.
package pick

import lipgloss "charm.land/lipgloss/v2"

var (
	PrimaryColor   = lipgloss.Color("#7C3AED")
	SecondaryColor = lipgloss.Color("#6B7280")
	AccentColor    = lipgloss.Color("#10B981")
	ErrorColor     = lipgloss.Color("#EF4444")
	MutedColor     = lipgloss.Color("#9CA3AF")
	MatchColor     = lipgloss.Color("#F59E0B")

	PromptStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	CountStyle = lipgloss.NewStyle().
			Foreground(MutedColor)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	NormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	MatchStyle = lipgloss.NewStyle().
			Foreground(MatchColor).
			Bold(true)

	TitleStyle = lipgloss.NewStyle().
			Foreground(MutedColor)
)
