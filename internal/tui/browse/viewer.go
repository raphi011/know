package browse

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// backToFinderMsg signals the browser to switch back to the finder.
type backToFinderMsg struct{}

type viewerModel struct {
	viewport   viewport.Model
	path       string
	width      int
	bookmarked bool
}

func newViewer(path, renderedContent string, width, height int) viewerModel {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(max(height-2, 1)))
	vp.SetContent(renderedContent)

	return viewerModel{
		viewport: vp,
		path:     path,
		width:    width,
	}
}

func (v viewerModel) Update(msg tea.Msg) (viewerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return v, func() tea.Msg { return backToFinderMsg{} }
		case "q":
			return v, tea.Quit
		}
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

func (v viewerModel) View() string {
	var b strings.Builder

	// Header bar with file path
	header := headerBarStyle.Render(truncate(v.path, v.width-2))
	b.WriteString(header)
	b.WriteString("\n")

	// Viewport content
	b.WriteString(v.viewport.View())
	b.WriteString("\n")

	// Footer with scroll position and keybinds
	pct := v.viewport.ScrollPercent()
	bookmarkHint := "b: bookmark"
	if v.bookmarked {
		bookmarkHint = "b: unbookmark"
	}
	footer := footerBarStyle.Render(fmt.Sprintf("  %.0f%%  %s  esc: back  q: quit", pct*100, bookmarkHint))
	b.WriteString(footer)

	return b.String()
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
