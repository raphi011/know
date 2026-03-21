package browse

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/pick"
)

// bookmarksLoadedMsg is sent when the bookmarks list has been fetched.
type bookmarksLoadedMsg struct {
	bookmarks []models.FileEntry
	err       error
}

// bookmarkToggledMsg is sent after a bookmark add/remove completes.
type bookmarkToggledMsg struct {
	path  string
	added bool
	err   error
}

type bookmarksModel struct {
	items     []models.FileEntry
	cursor    int
	offset    int
	width     int
	height    int
	loaded    bool
	statusErr string
	client    *apiclient.Client
	vaultID   string
}

func newBookmarksModel(client *apiclient.Client, vaultID string) bookmarksModel {
	return bookmarksModel{
		client:  client,
		vaultID: vaultID,
	}
}

func (b bookmarksModel) loadBookmarks() tea.Cmd {
	client := b.client
	vaultID := b.vaultID
	return func() tea.Msg {
		items, err := client.ListBookmarks(context.Background(), vaultID)
		if err != nil {
			return bookmarksLoadedMsg{err: err}
		}
		return bookmarksLoadedMsg{bookmarks: items}
	}
}

func (b bookmarksModel) Update(msg tea.Msg) (bookmarksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case bookmarksLoadedMsg:
		b.loaded = true
		if msg.err != nil {
			b.statusErr = fmt.Sprintf("Failed to load bookmarks: %v", msg.err)
			return b, nil
		}
		b.items = msg.bookmarks
		b.statusErr = ""
		b.cursor = 0
		b.offset = 0
		return b, nil

	case bookmarkToggledMsg:
		if msg.err != nil {
			b.statusErr = fmt.Sprintf("Failed to toggle bookmark: %v", msg.err)
			return b, nil
		}
		b.statusErr = ""
		// Reload bookmarks to reflect changes
		return b, b.loadBookmarks()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if b.cursor > 0 {
				b.cursor--
				if b.cursor < b.offset {
					b.offset = b.cursor
				}
			}
		case "down", "j":
			if b.cursor < len(b.items)-1 {
				b.cursor++
				visible := b.visibleRows()
				if b.cursor >= b.offset+visible {
					b.offset = b.cursor - visible + 1
				}
			}
		case "enter":
			if len(b.items) > 0 && b.cursor < len(b.items) {
				return b, func() tea.Msg {
					return fileSelectedMsg{path: b.items[b.cursor].Path}
				}
			}
		case "d":
			// Remove bookmark
			if len(b.items) > 0 && b.cursor < len(b.items) {
				return b, b.toggleBookmark(b.items[b.cursor].Path, false)
			}
		}
	}
	return b, nil
}

func (b bookmarksModel) toggleBookmark(path string, add bool) tea.Cmd {
	client := b.client
	vaultID := b.vaultID
	return func() tea.Msg {
		var err error
		if add {
			err = client.AddBookmark(context.Background(), vaultID, path)
		} else {
			err = client.RemoveBookmark(context.Background(), vaultID, path)
		}
		return bookmarkToggledMsg{path: path, added: add, err: err}
	}
}

func (b bookmarksModel) visibleRows() int {
	return max(b.height-3, 1) // header + status + padding
}

func (b bookmarksModel) View() string {
	var sb strings.Builder

	if !b.loaded {
		sb.WriteString("  Loading bookmarks...")
		return sb.String()
	}

	if b.statusErr != "" {
		sb.WriteString(errStyle.Render(b.statusErr))
		sb.WriteString("\n")
	}

	// Header
	countStr := fmt.Sprintf("%d bookmarks", len(b.items))
	sb.WriteString(lipgloss.NewStyle().Foreground(pick.MutedColor).Render(countStr))
	sb.WriteString("\n")

	if len(b.items) == 0 {
		sb.WriteString("\n  No bookmarks yet. Use ")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("b"))
		sb.WriteString(" while viewing a document to bookmark it.")
		return sb.String()
	}

	visible := b.visibleRows()
	end := min(b.offset+visible, len(b.items))
	for i := b.offset; i < end; i++ {
		item := b.items[i]
		line := item.Path
		if item.Title != "" {
			line = item.Title + "  " + lipgloss.NewStyle().Foreground(pick.MutedColor).Render(item.Path)
		}

		if i == b.cursor {
			sb.WriteString(lipgloss.NewStyle().Foreground(pick.PrimaryColor).Bold(true).Render("> " + line))
		} else {
			sb.WriteString("  " + line)
		}
		sb.WriteString("\n")
	}

	// Footer keybinds
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(pick.MutedColor).Render("  enter: open  d: remove bookmark"))

	return sb.String()
}
