package browse

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
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

type linksView int

const (
	linksViewInbox linksView = iota
	linksViewArchived
)

type linksModel struct {
	allLinks []models.FileEntry // all web clips
	filtered []models.FileEntry // filtered by view (inbox/archived)
	matches  []fuzzy.Match
	cursor   int
	offset   int
	width    int
	height   int

	input     textinput.Model
	view      linksView
	loaded    bool
	statusErr string
	statusOK  string

	confirmDelete bool // true when waiting for y/n confirmation

	client  *apiclient.Client
	vaultID string
}

func newLinksModel(client *apiclient.Client, vaultID string) linksModel {
	ti := textinput.New()
	ti.Placeholder = "Search links..."
	ti.CharLimit = 256
	ti.Prompt = "/ "

	styles := ti.Styles()
	styles.Cursor.Blink = false
	styles.Focused.Prompt = pick.PromptStyle
	ti.SetStyles(styles)

	return linksModel{
		input:   ti,
		client:  client,
		vaultID: vaultID,
		view:    linksViewInbox,
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
	l.filtered = nil
	for _, link := range l.allLinks {
		isArchived := slices.Contains(link.Labels, "archived")
		if l.view == linksViewInbox && !isArchived {
			l.filtered = append(l.filtered, link)
		} else if l.view == linksViewArchived && isArchived {
			l.filtered = append(l.filtered, link)
		}
	}
	l.refilter()
}

func (l *linksModel) refilter() {
	query := l.input.Value()
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
	// tab bar (1) + search hint (1) + count line (1) + footer (1) = 4 lines overhead
	return max(l.height-4, 1)
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

		// Delete confirmation mode
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

		switch msg.String() {
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
			if l.view == linksViewInbox {
				l.view = linksViewArchived
			} else {
				l.view = linksViewInbox
			}
			l.cursor = 0
			l.offset = 0
			l.applyFilter()
			return l, nil
		case "esc":
			return l, tea.Quit
		case "up":
			if l.cursor > 0 {
				l.cursor--
				l.ensureCursorVisible()
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

	// Delegate remaining keys to text input for search.
	prev := l.input.Value()
	var cmd tea.Cmd
	l.input, cmd = l.input.Update(msg)
	if l.input.Value() != prev {
		l.refilter()
	}
	return l, cmd
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

	// Search input
	b.WriteString(l.input.View())
	b.WriteString("\n")

	// View indicator + count
	viewLabel := "Inbox"
	if l.view == linksViewArchived {
		viewLabel = "Archived"
	}
	b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %s — %d/%d links", viewLabel, len(l.matches), len(l.filtered))))
	b.WriteString("\n")

	visible := l.visibleRows()
	end := min(l.offset+visible, len(l.matches))

	for i := l.offset; i < end; i++ {
		m := l.matches[i]
		entry := l.filtered[m.Index]

		prefix := "  "
		style := pick.NormalStyle
		if i == l.cursor {
			prefix = "> "
			style = pick.SelectedStyle
		}

		// Title or path
		title := entry.Title
		if title == "" {
			title = entry.Path
		}
		line := style.Render(title)

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

	// Status messages
	if l.confirmDelete {
		b.WriteString(errStyle.Render("  Delete this link? y/n"))
	} else if l.statusErr != "" {
		b.WriteString(errStyle.Render("  " + l.statusErr))
	} else if l.statusOK != "" {
		b.WriteString(pick.CountStyle.Render("  " + l.statusOK))
	} else {
		b.WriteString(pick.CountStyle.Render("  enter: view  o: open  a: archive  d: delete  v: inbox/archived  esc: quit"))
	}

	return b.String()
}
