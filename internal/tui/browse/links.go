package browse

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/browser"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/pick"
	"github.com/sahilm/fuzzy"
)

// linkSource implements fuzzy.Source for matching against link title + source URL.
type linkSource []models.FileEntry

func (s linkSource) String(i int) string {
	e := s[i]
	parts := e.Title
	if e.Source != "" {
		parts += " " + e.Source
	}
	if parts == "" {
		parts = e.Path
	}
	return parts
}

func (s linkSource) Len() int { return len(s) }

// linksLoadedMsg is sent when the links list has been fetched from the API.
type linksLoadedMsg struct {
	links []models.FileEntry
	err   error
}

// linkArchivedMsg is sent after a label patch completes.
type linkArchivedMsg struct {
	path   string
	labels []string
	err    error
}

// linkDeletedMsg is sent after a delete completes.
type linkDeletedMsg struct {
	path string
	err  error
}

type linksModel struct {
	allLinks []models.FileEntry // all web clips
	filtered []models.FileEntry // filtered by view (inbox/archived)
	matches  []fuzzy.Match
	cursor   int
	offset   int
	width    int
	height   int

	filterBar FilterBar
	loaded    bool
	statusErr string
	statusOK  string

	confirmDelete bool // true when waiting for y/n confirmation

	client  *apiclient.Client
	vaultID string
}

func newLinksModel(client *apiclient.Client, vaultID string) linksModel {
	return linksModel{
		filterBar: NewFilterBar(FilterBarConfig{
			SupportedKeys: []string{"label", "host", "status"},
			Placeholder:   "Filter links...",
			Hints:         "status:inbox|archived  host:<domain>  label:<name>",
		}),
		client:  client,
		vaultID: vaultID,
	}
}

func (l linksModel) loadLinks() tea.Cmd {
	client := l.client
	vaultID := l.vaultID
	return func() tea.Msg {
		links, err := client.ListFilesByLabels(context.Background(), vaultID, []string{"web-clip"})
		if err != nil {
			return linksLoadedMsg{err: fmt.Errorf("load links: %w", err)}
		}
		return linksLoadedMsg{links: links}
	}
}

func (l *linksModel) applyFilter() {
	r := l.filterBar.Result()
	l.filtered = nil

	// status filter: default to inbox if no status specified
	statusFilter := r.Filter("status")

	for _, link := range l.allLinks {
		isArchived := slices.Contains(link.Labels, "archived")

		// Status filter
		switch statusFilter {
		case "archived":
			if !isArchived {
				continue
			}
		case "inbox":
			if isArchived {
				continue
			}
		case "":
			// Default: show inbox
			if isArchived {
				continue
			}
		}

		// Host filter
		if host := r.Filter("host"); host != "" {
			if !strings.Contains(link.Source, host) {
				continue
			}
		}

		// Label filter
		if labels := r.FilterAll("label"); len(labels) > 0 {
			matched := true
			for _, label := range labels {
				if !slices.Contains(link.Labels, label) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}

		l.filtered = append(l.filtered, link)
	}
	l.refilter()
}

func (l *linksModel) refilter() {
	query := l.filterBar.Result().Query
	if query == "" {
		l.matches = make([]fuzzy.Match, len(l.filtered))
		for i := range l.filtered {
			l.matches[i] = fuzzy.Match{Index: i}
		}
	} else {
		l.matches = fuzzy.FindFrom(query, linkSource(l.filtered))
	}

	if l.cursor >= len(l.matches) {
		l.cursor = max(0, len(l.matches)-1)
	}
	l.ensureCursorVisible()
}

func (l *linksModel) ensureCursorVisible() {
	visible := l.visibleRows()
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+visible {
		l.offset = l.cursor - visible + 1
	}
}

func (l linksModel) visibleRows() int {
	return max(l.height-l.filterBar.HeightLines()-4, 1) // filterbar(+hints) + count + gap + footer(blank+hotkeys)
}

func (l linksModel) selectedEntry() *models.FileEntry {
	if len(l.matches) == 0 || l.cursor >= len(l.matches) {
		return nil
	}
	return &l.filtered[l.matches[l.cursor].Index]
}

func (l linksModel) Update(msg tea.Msg) (linksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case linksLoadedMsg:
		l.loaded = true
		if msg.err != nil {
			l.statusErr = msg.err.Error()
			return l, nil
		}
		l.allLinks = msg.links
		l.applyFilter()
		return l, nil

	case linkArchivedMsg:
		if msg.err != nil {
			l.statusErr = fmt.Sprintf("Archive failed: %v", msg.err)
			return l, nil
		}
		// Update the local entry's labels
		for i := range l.allLinks {
			if l.allLinks[i].Path == msg.path {
				l.allLinks[i].Labels = msg.labels
				break
			}
		}
		l.statusOK = "Updated"
		l.applyFilter()
		return l, nil

	case linkDeletedMsg:
		if msg.err != nil {
			l.statusErr = fmt.Sprintf("Delete failed: %v", msg.err)
			return l, nil
		}
		l.allLinks = slices.DeleteFunc(l.allLinks, func(e models.FileEntry) bool {
			return e.Path == msg.path
		})
		l.statusOK = "Deleted"
		l.applyFilter()
		return l, nil

	case tea.KeyPressMsg:
		l.statusErr = ""
		l.statusOK = ""

		// Delete confirmation mode takes priority over everything.
		if l.confirmDelete {
			switch msg.String() {
			case "y":
				l.confirmDelete = false
				entry := l.selectedEntry()
				if entry == nil {
					return l, nil
				}
				return l, l.deleteLink(entry.Path)
			default:
				l.confirmDelete = false
				return l, nil
			}
		}

		if l.filterBar.Focused() {
			// Input mode: filter bar gets all keys except navigation out.
			switch msg.String() {
			case "down":
				l.filterBar = l.filterBar.Blur()
				return l, nil
			case "enter":
				return l, nil
			case "esc":
				if l.filterBar.Value() != "" {
					l.filterBar = l.filterBar.SetValue("")
					l.applyFilter()
					return l, nil
				}
				return l, tea.Quit
			}
			// All other keys → filter bar.
			prev := l.filterBar.Value()
			var cmd tea.Cmd
			l.filterBar, cmd = l.filterBar.Update(msg)
			if l.filterBar.Value() != prev {
				l.applyFilter()
			}
			return l, cmd
		}

		// List mode: hotkeys work here.
		switch msg.String() {
		case "/":
			var cmd tea.Cmd
			l.filterBar, cmd = l.filterBar.Focus()
			return l, cmd
		case "enter":
			entry := l.selectedEntry()
			if entry != nil {
				return l, func() tea.Msg {
					return fileSelectedMsg{path: entry.Path}
				}
			}
			return l, nil
		case "o":
			entry := l.selectedEntry()
			if entry != nil && entry.Source != "" {
				if err := browser.Open(entry.Source); err != nil {
					l.statusErr = fmt.Sprintf("Could not open browser: %v", err)
				}
			}
			return l, nil
		case "a":
			entry := l.selectedEntry()
			if entry == nil {
				return l, nil
			}
			return l, l.toggleArchive(entry)
		case "d":
			if l.selectedEntry() != nil {
				l.confirmDelete = true
			}
			return l, nil
		case "v":
			current := l.filterBar.Value()
			if strings.Contains(current, "status:archived") {
				l.filterBar = l.filterBar.SetValue(strings.TrimSpace(strings.Replace(current, "status:archived", "", 1)))
			} else {
				if current == "" {
					l.filterBar = l.filterBar.SetValue("status:archived")
				} else {
					l.filterBar = l.filterBar.SetValue(current + " status:archived")
				}
			}
			l.applyFilter()
			return l, nil
		case "esc":
			return l, tea.Quit
		case "up":
			if l.cursor > 0 {
				l.cursor--
				l.ensureCursorVisible()
			} else {
				var cmd tea.Cmd
				l.filterBar, cmd = l.filterBar.Focus()
				return l, cmd
			}
			return l, nil
		case "down":
			if l.cursor < len(l.matches)-1 {
				l.cursor++
				l.ensureCursorVisible()
			}
			return l, nil
		case "pgup":
			l.cursor = max(l.cursor-l.visibleRows(), 0)
			l.ensureCursorVisible()
			return l, nil
		case "pgdown":
			l.cursor = min(l.cursor+l.visibleRows(), max(len(l.matches)-1, 0))
			l.ensureCursorVisible()
			return l, nil
		}
	}

	return l, nil
}

func (l linksModel) toggleArchive(entry *models.FileEntry) tea.Cmd {
	client := l.client
	vaultID := l.vaultID
	path := entry.Path
	isArchived := slices.Contains(entry.Labels, "archived")

	return func() tea.Msg {
		var req apiclient.PatchLabelsRequest
		req.Path = path
		if isArchived {
			req.Remove = []string{"archived"}
		} else {
			req.Add = []string{"archived"}
		}
		if err := client.PatchDocumentLabels(context.Background(), vaultID, req); err != nil {
			return linkArchivedMsg{path: path, err: err}
		}

		// Compute resulting labels locally
		labels := make([]string, len(entry.Labels))
		copy(labels, entry.Labels)
		if isArchived {
			labels = slices.DeleteFunc(labels, func(l string) bool { return l == "archived" })
		} else {
			labels = append(labels, "archived")
		}
		return linkArchivedMsg{path: path, labels: labels}
	}
}

func (l linksModel) deleteLink(path string) tea.Cmd {
	client := l.client
	vaultID := l.vaultID
	return func() tea.Msg {
		_, err := client.DeleteDocuments(context.Background(), vaultID, path, false, false)
		if err != nil {
			return linkDeletedMsg{path: path, err: err}
		}
		return linkDeletedMsg{path: path}
	}
}

var (
	sourceStyle   = lipgloss.NewStyle().Foreground(pick.MutedColor)
	archivedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
)

func (l linksModel) View() string {
	var b strings.Builder

	if !l.loaded {
		b.WriteString("  Loading links...")
		return b.String()
	}

	// Filter bar
	b.WriteString(l.filterBar.View())
	b.WriteString("\n")

	// Count line
	b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d/%d links", len(l.matches), len(l.filtered))))
	b.WriteString("\n\n")

	listFocused := !l.filterBar.Focused()
	visible := l.visibleRows()
	end := min(l.offset+visible, len(l.matches))

	for i := l.offset; i < end; i++ {
		m := l.matches[i]
		entry := l.filtered[m.Index]

		selected := i == l.cursor && listFocused
		prefix := pick.CursorPrefix(i == l.cursor, listFocused)

		// Title or path
		title := entry.Title
		if title == "" {
			title = entry.Path
		}
		if selected {
			title = pick.SelectedStyle.Render(title)
		}
		line := title

		// Source URL (dimmed)
		if entry.Source != "" {
			line += " " + sourceStyle.Render(entry.Source)
		}

		// Archived badge
		if slices.Contains(entry.Labels, "archived") {
			line += " " + archivedStyle.Render("[archived]")
		}

		b.WriteString(prefix + line + "\n")
	}

	// Pad remaining space
	rendered := end - l.offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n")
	}

	// Footer: status + hotkeys
	statusOK := l.statusOK
	statusErr := l.statusErr
	if l.confirmDelete {
		statusErr = "Delete this link? y/n"
	}
	b.WriteString(renderFooterStatus(statusErr, statusOK, []hotkey{
		{"enter", "view"},
		{"o", "open"},
		{"a", "archive"},
		{"d", "delete"},
		{"v", "toggle archived"},
		{"esc", "quit"},
	}))

	return b.String()
}
