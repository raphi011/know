package browse

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/record"
)

// backToFinderMsg signals the browser to switch back to the finder.
type backToFinderMsg struct{}

type viewerModel struct {
	viewport    viewport.Model
	audioPlayer *audioPlayerModel
	path        string
	sourceLine  int // 1-based source line for editor +LINE (0 = none)
	width       int
	height      int
	bookmarked  bool
	statusMsg   string
}

func newViewer(path, renderedContent string, width, height int) viewerModel {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(max(height-2, 1)))
	vp.SetContent(renderedContent)

	return viewerModel{
		viewport: vp,
		path:     path,
		width:    width,
		height:   height,
	}
}

func newAudioViewer(path string, player *record.Player, transcript string, width, height int) viewerModel {
	ap := newAudioPlayer(player, transcript, width, height)
	return viewerModel{
		audioPlayer: &ap,
		path:        path,
		width:       width,
		height:      height,
	}
}

func (v viewerModel) Update(msg tea.Msg) (viewerModel, tea.Cmd) {
	if v.audioPlayer != nil {
		var cmd tea.Cmd
		*v.audioPlayer, cmd = v.audioPlayer.Update(msg)
		return v, cmd
	}

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

	if v.audioPlayer != nil {
		b.WriteString(v.audioPlayer.View())
	} else {
		// Viewport content
		b.WriteString(v.viewport.View())
		b.WriteString("\n")

		// Footer with scroll position and keybinds
		pct := v.viewport.ScrollPercent()
		bookmarkHint := "b: bookmark"
		if v.bookmarked {
			bookmarkHint = "b: unbookmark"
		}
		editHint := "  e: edit"
		if v.audioPlayer != nil || !models.IsTextFile(v.path) {
			editHint = ""
		}
		parts := fmt.Sprintf("%.0f%%%s  %s  esc: back  q: quit", pct*100, editHint, bookmarkHint)
		if v.statusMsg != "" {
			parts = v.statusMsg + "  —  " + parts
		}
		footer := footerBarStyle.Render("  " + parts)
		b.WriteString(footer)
	}

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
