package tui

import lipgloss "charm.land/lipgloss/v2"

var (
	// Colors
	primaryColor   = lipgloss.Color("#7C3AED") // violet
	secondaryColor = lipgloss.Color("#6B7280") // gray
	accentColor    = lipgloss.Color("#10B981") // green
	errorColor     = lipgloss.Color("#EF4444") // red
	mutedColor     = lipgloss.Color("#9CA3AF") // light gray

	// Header style (startup banner)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
)

// Banner returns the styled startup banner for printing before bubbletea starts.
func Banner() string {
	return headerStyle.Render("knowhow")
}

var (

	// Status bar
	statusStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Chat styles
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			PaddingLeft(2)

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

	// Status bar detail style (below prompt)
	statusBarDetailStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Error message style
	errorMsgStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// Attachment indicator style (shown below user messages)
	attachmentStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2)

	// File list styles (drag-and-drop attachments above input)
	fileListItemStyle     = lipgloss.NewStyle().Foreground(mutedColor).PaddingLeft(2)
	fileListSelectedStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).PaddingLeft(2)
)
