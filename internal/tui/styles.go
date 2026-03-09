package tui

import lipgloss "charm.land/lipgloss/v2"

var (
	// Colors
	primaryColor   = lipgloss.Color("#7C3AED") // violet
	secondaryColor = lipgloss.Color("#6B7280") // gray
	accentColor    = lipgloss.Color("#10B981") // green
	errorColor     = lipgloss.Color("#EF4444") // red
	mutedColor     = lipgloss.Color("#9CA3AF") // light gray
	borderColor    = lipgloss.Color("#374151") // dark gray

	// Pane styles
	activePaneBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor)

	inactivePaneBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor)

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Padding(0, 1)

	// Status bar
	statusStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1)

	// Chat styles
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D1D5DB"))

	roleLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1, 0, 0)

	userRoleStyle = roleLabelStyle.Foreground(accentColor)

	assistantRoleStyle = roleLabelStyle.Foreground(primaryColor)

	toolRoleStyle = roleLabelStyle.Foreground(secondaryColor)

	// Input style
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	// Help style
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Approval styles
	approvalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#F59E0B")).
				Padding(0, 1)

	approveKeyStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	rejectKeyStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Error message style
	errorMsgStyle = lipgloss.NewStyle().
			Foreground(errorColor)
)
