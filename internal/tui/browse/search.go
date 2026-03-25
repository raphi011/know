package browse

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/tui/pick"
)

// searchResultsMsg is sent when a search API call completes.
type searchResultsMsg struct {
	query   string // the query that produced these results
	results []apiclient.SearchResult
	err     error
}

// searchDebounceMsg fires after the debounce delay; seq must match current
// debounceSeq to trigger a search (stale ticks are discarded).
type searchDebounceMsg struct {
	seq int
}

type searchModel struct {
	results []apiclient.SearchResult
	cursor  int
	offset  int
	width   int
	height  int

	filterBar   FilterBar
	searching   bool
	statusErr   string
	debounceSeq int
	lastQuery   string // query that produced current results

	client  *apiclient.Client
	vaultID string
}

var (
	snippetStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	headingPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
)

func newSearchModel(client *apiclient.Client, vaultID string) searchModel {
	return searchModel{
		filterBar: NewFilterBar(FilterBarConfig{
			SupportedKeys: []string{"label"},
			Placeholder:   "Search documents...",
			Hints:         "label:<name>",
		}),
		client:  client,
		vaultID: vaultID,
	}
}

func (s searchModel) doSearch() tea.Cmd {
	client := s.client
	vaultID := s.vaultID
	query := s.filterBar.Result().Query
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results, err := client.SearchDocuments(ctx, vaultID, query, 20, true)
		return searchResultsMsg{query: query, results: results, err: err}
	}
}

func (s *searchModel) ensureCursorVisible() {
	visible := s.visibleRows()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+visible {
		s.offset = s.cursor - visible + 1
	}
}

func (s searchModel) visibleRows() int {
	// filterbar + count line (1) + footer (1) = overhead
	// Each result takes 2 lines (path+title, snippet).
	overhead := s.filterBar.HeightLines() + 2
	return max((s.height-overhead)/2, 1)
}

func (s searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultsMsg:
		s.searching = false
		// Discard stale responses from superseded queries.
		if msg.query != s.filterBar.Result().Query {
			return s, nil
		}
		if msg.err != nil {
			s.statusErr = fmt.Sprintf("Search failed: %v", msg.err)
			s.results = nil
			return s, nil
		}
		s.statusErr = ""
		s.results = msg.results
		s.lastQuery = msg.query
		s.cursor = 0
		s.offset = 0
		return s, nil

	case searchDebounceMsg:
		if msg.seq != s.debounceSeq {
			return s, nil // stale tick
		}
		query := s.filterBar.Result().Query
		if query == "" {
			return s, nil
		}
		s.searching = true
		return s, s.doSearch()

	case tea.KeyPressMsg:
		s.statusErr = ""

		switch msg.String() {
		case "enter":
			// If we have results and a valid cursor, select the file.
			if len(s.results) > 0 && s.cursor < len(s.results) {
				path := s.results[s.cursor].Path
				return s, func() tea.Msg {
					return fileSelectedMsg{path: path}
				}
			}
			// Otherwise trigger immediate search if we have a query.
			if s.filterBar.Result().Query != "" {
				s.debounceSeq++ // cancel pending debounce
				s.searching = true
				return s, s.doSearch()
			}
			return s, nil
		case "esc":
			return s, tea.Quit
		case "up":
			if s.cursor > 0 {
				s.cursor--
				s.ensureCursorVisible()
			}
			return s, nil
		case "down":
			if s.cursor < len(s.results)-1 {
				s.cursor++
				s.ensureCursorVisible()
			}
			return s, nil
		case "pgup":
			s.cursor = max(s.cursor-s.visibleRows(), 0)
			s.ensureCursorVisible()
			return s, nil
		case "pgdown":
			s.cursor = min(s.cursor+s.visibleRows(), max(len(s.results)-1, 0))
			s.ensureCursorVisible()
			return s, nil
		}
	}

	// Delegate remaining keys to filterBar.
	prev := s.filterBar.Value()
	var cmd tea.Cmd
	s.filterBar, cmd = s.filterBar.Update(msg)

	if s.filterBar.Value() != prev {
		// Input changed — clear results if query is now empty, otherwise debounce.
		if s.filterBar.Result().Query == "" {
			s.results = nil
			s.lastQuery = ""
			s.searching = false
			return s, cmd
		}
		s.debounceSeq++
		seq := s.debounceSeq
		debounceCmd := tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
			return searchDebounceMsg{seq: seq}
		})
		return s, tea.Batch(cmd, debounceCmd)
	}

	return s, cmd
}

func (s searchModel) View() string {
	var b strings.Builder

	b.WriteString(s.filterBar.View())
	b.WriteString("\n")

	// Count / status line
	if s.searching {
		b.WriteString(pick.CountStyle.Render("  Searching..."))
	} else if s.filterBar.Result().Query == "" {
		b.WriteString(pick.CountStyle.Render("  Type to search documents"))
	} else if len(s.results) == 0 {
		b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  No results for %q", s.lastQuery)))
	} else {
		b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d results", len(s.results))))
	}
	b.WriteString("\n")

	visible := s.visibleRows()
	end := min(s.offset+visible, len(s.results))

	for i := s.offset; i < end; i++ {
		result := s.results[i]

		prefix := "  "
		style := pick.NormalStyle
		if i == s.cursor {
			prefix = "> "
			style = pick.SelectedStyle
		}

		// Line 1: path + title
		line := style.Render(result.Path)
		if result.Title != "" {
			line += "  " + pick.TitleStyle.Render(result.Title)
		}
		b.WriteString(prefix + line + "\n")

		// Line 2: heading path + snippet (from top matched chunk)
		snippetLine := "  "
		if len(result.MatchedChunks) > 0 {
			chunk := result.MatchedChunks[0]
			if chunk.HeadingPath != nil && *chunk.HeadingPath != "" {
				snippetLine += headingPathStyle.Render("## "+*chunk.HeadingPath) + " — "
			}
			snippet := truncateSnippet(chunk.Snippet, s.width-len(snippetLine)-4)
			snippetLine += snippetStyle.Render(snippet)
		}
		b.WriteString(snippetLine + "\n")
	}

	// Pad remaining space (2 lines per result slot).
	rendered := end - s.offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n\n")
	}

	// Footer
	if s.statusErr != "" {
		b.WriteString(errStyle.Render("  " + s.statusErr))
	} else {
		b.WriteString(pick.CountStyle.Render("  enter: open  esc: quit"))
	}

	return b.String()
}

// truncateSnippet shortens a snippet to fit within maxLen runes, adding "…" if truncated.
func truncateSnippet(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 40
	}
	// Replace newlines with spaces for single-line display.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
