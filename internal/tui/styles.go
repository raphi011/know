package tui

import (
	lipgloss "charm.land/lipgloss/v2"

	"github.com/raphi011/know/internal/tui/pick"
)

var (
	// Colors — canonical definitions live in pick/styles.go and are re-aliased here.
	primaryColor   = pick.PrimaryColor
	secondaryColor = pick.SecondaryColor
	accentColor    = pick.AccentColor
	errorColor     = pick.ErrorColor
	mutedColor     = pick.MutedColor

	// Header style (startup banner)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
)

// Banner returns the styled startup banner for printing before bubbletea starts.
func Banner() string {
	return headerStyle.Render("know")
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

	// Diff styles
	diffAddStyle        = lipgloss.NewStyle().Foreground(accentColor)               // green for added lines
	diffDeleteStyle     = lipgloss.NewStyle().Foreground(errorColor)                // red for deleted lines
	diffHunkHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")) // blue for @@ headers
	diffGutterStyle     = lipgloss.NewStyle().Foreground(mutedColor)
	diffSeparatorStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	diffHeaderStyle     = lipgloss.NewStyle().Foreground(mutedColor).Bold(true)
	diffNavStyle        = lipgloss.NewStyle().Foreground(mutedColor)
)
