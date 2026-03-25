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
	allItems  []models.FileEntry
	filtered  []models.FileEntry
	cursor    int
	offset    int
	width     int
	height    int
	loaded    bool
	statusErr string
	filterBar FilterBar
	client    *apiclient.Client
	vaultID   string
}

func newBookmarksModel(client *apiclient.Client, vaultID string) bookmarksModel {
	return bookmarksModel{
		filterBar: NewFilterBar(FilterBarConfig{
			SupportedKeys: []string{"label"},
			Placeholder:   "Filter bookmarks...",
			Hints:         "label:<name>",
		}),
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

// applyFilter filters allItems by label filter and fuzzy text match on title/path,
// storing results in filtered.
func (b *bookmarksModel) applyFilter() {
	result := b.filterBar.Result()
	labelFilter := result.Filter("label")
	query := strings.ToLower(result.Query)

	filtered := make([]models.FileEntry, 0, len(b.allItems))
	for _, item := range b.allItems {
		// Label filter: skip items whose path doesn't contain the label token.
		if labelFilter != "" && !strings.Contains(strings.ToLower(item.Path), strings.ToLower(labelFilter)) {
			continue
		}
		// Text match on title or path.
		if query != "" {
			titleLower := strings.ToLower(item.Title)
			pathLower := strings.ToLower(item.Path)
			if !strings.Contains(titleLower, query) && !strings.Contains(pathLower, query) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	b.filtered = filtered
	b.cursor = 0
	b.offset = 0
}

func (b bookmarksModel) Update(msg tea.Msg) (bookmarksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case bookmarksLoadedMsg:
		b.loaded = true
		if msg.err != nil {
			b.statusErr = fmt.Sprintf("Failed to load bookmarks: %v", msg.err)
			return b, nil
		}
		b.allItems = msg.bookmarks
		b.statusErr = ""
		b.applyFilter()
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
			return b, nil
		case "down", "j":
			if b.cursor < len(b.filtered)-1 {
				b.cursor++
				visible := b.visibleRows()
				if b.cursor >= b.offset+visible {
					b.offset = b.cursor - visible + 1
				}
			}
			return b, nil
		case "enter":
			if len(b.filtered) > 0 && b.cursor < len(b.filtered) {
				return b, func() tea.Msg {
					return fileSelectedMsg{path: b.filtered[b.cursor].Path}
				}
			}
			return b, nil
		case "d":
			// Remove bookmark
			if len(b.filtered) > 0 && b.cursor < len(b.filtered) {
				return b, b.toggleBookmark(b.filtered[b.cursor].Path, false)
			}
			return b, nil
		case "esc":
			return b, tea.Quit
		}
	}

	// Delegate remaining messages to filterBar.
	prev := b.filterBar.Value()
	var cmd tea.Cmd
	b.filterBar, cmd = b.filterBar.Update(msg)
	if b.filterBar.Value() != prev {
		b.applyFilter()
	}
	return b, cmd
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
	return max(b.height-b.filterBar.HeightLines()-3, 1) // filterbar + header + status + padding
}

func (b bookmarksModel) View() string {
	var sb strings.Builder

	sb.WriteString(b.filterBar.View())
	sb.WriteString("\n")

	if !b.loaded {
		sb.WriteString("  Loading bookmarks...")
		return sb.String()
	}

	if b.statusErr != "" {
		sb.WriteString(errStyle.Render(b.statusErr))
		sb.WriteString("\n")
	}

	// Header
	countStr := fmt.Sprintf("%d bookmarks", len(b.filtered))
	if len(b.filtered) != len(b.allItems) {
		countStr = fmt.Sprintf("%d of %d bookmarks", len(b.filtered), len(b.allItems))
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(pick.MutedColor).Render(countStr))
	sb.WriteString("\n")

	if len(b.filtered) == 0 && len(b.allItems) == 0 {
		sb.WriteString("\n  No bookmarks yet. Use ")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("b"))
		sb.WriteString(" while viewing a document to bookmark it.")
		return sb.String()
	}

	if len(b.filtered) == 0 {
		sb.WriteString("\n  No bookmarks match the filter.")
		return sb.String()
	}

	visible := b.visibleRows()
	end := min(b.offset+visible, len(b.filtered))
	for i := b.offset; i < end; i++ {
		item := b.filtered[i]
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
