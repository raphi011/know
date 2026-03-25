package browse

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/tui/pick"
)

// searchResultsMsg is sent when a search API call completes.
type searchResultsMsg struct {
	query   string   // the query that produced these results
	labels  []string // the labels that produced these results
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
	searched    bool // true after at least one search has completed for current input
	statusErr   string
	debounceSeq int
	lastQuery   string // query that produced current results

	client  *apiclient.Client
	vaultID string
}

var snippetStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

func newSearchModel(client *apiclient.Client, vaultID string) searchModel {
	return searchModel{
		filterBar: NewFilterBar(FilterBarConfig{
			SupportedKeys: []string{"label"},
			Placeholder:   "Search documents...",
		}),
		client:  client,
		vaultID: vaultID,
	}
}

func (s searchModel) doSearch() tea.Cmd {
	r := s.filterBar.Result()
	query := r.Query
	labels := r.FilterAll("label")
	client := s.client
	vaultID := s.vaultID

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if query != "" {
			results, err := client.SearchDocuments(ctx, vaultID, query, labels, 20, true)
			return searchResultsMsg{query: query, labels: labels, results: results, err: err}
		}

		// Label-only: use list endpoint (no query string to score against).
		files, err := client.ListFilesByLabels(ctx, vaultID, labels)
		if err != nil {
			return searchResultsMsg{labels: labels, err: err}
		}
		results := make([]apiclient.SearchResult, len(files))
		for i, f := range files {
			results[i] = apiclient.SearchResult{Path: f.Path, Title: f.Title}
		}
		return searchResultsMsg{labels: labels, results: results}
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
	// filterbar + count line (1) + footer (2) = overhead
	// Each result takes 2 lines (path, title).
	overhead := s.filterBar.HeightLines() + 3
	return max((s.height-overhead)/2, 1)
}

// hasSearchInput returns true if the filter bar has a query or any filters.
func (s searchModel) hasSearchInput() bool {
	r := s.filterBar.Result()
	return r.Query != "" || len(r.Filters) > 0
}

func (s searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultsMsg:
		s.searching = false
		// Discard stale responses from superseded queries.
		r := s.filterBar.Result()
		if msg.query != r.Query || !slices.Equal(msg.labels, r.FilterAll("label")) {
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
		s.searched = true
		s.cursor = 0
		s.offset = 0
		if len(s.results) == 0 {
			var cmd tea.Cmd
			s.filterBar, cmd = s.filterBar.Focus()
			return s, cmd
		}
		return s, nil

	case searchDebounceMsg:
		if msg.seq != s.debounceSeq {
			return s, nil // stale tick
		}
		if !s.hasSearchInput() {
			return s, nil
		}
		s.searching = true
		return s, s.doSearch()

	case tea.KeyPressMsg:
		s.statusErr = ""

		if s.filterBar.Focused() {
			// Input mode: filter bar gets all keys except navigation out.
			switch msg.String() {
			case "down":
				if len(s.results) > 0 {
					s.filterBar = s.filterBar.Blur()
				}
				return s, nil
			case "enter":
				// If we have results, select the first one.
				if len(s.results) > 0 && s.cursor < len(s.results) {
					path := s.results[s.cursor].Path
					return s, func() tea.Msg {
						return fileSelectedMsg{path: path}
					}
				}
				// Otherwise trigger immediate search.
				if s.hasSearchInput() {
					s.debounceSeq++
					s.searching = true
					return s, s.doSearch()
				}
				return s, nil
			case "esc":
				if s.filterBar.Value() != "" {
					s.filterBar = s.filterBar.SetValue("")
					s.results = nil
					s.lastQuery = ""
					s.searching = false
					s.searched = false
					return s, nil
				}
				return s, tea.Quit
			}
			// All other keys → filter bar.
			prev := s.filterBar.Value()
			var cmd tea.Cmd
			s.filterBar, cmd = s.filterBar.Update(msg)
			if s.filterBar.Value() != prev {
				s.searched = false
				if !s.hasSearchInput() {
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

		// List mode: hotkeys work here.
		switch msg.String() {
		case "/":
			var cmd tea.Cmd
			s.filterBar, cmd = s.filterBar.Focus()
			return s, cmd
		case "enter":
			if len(s.results) > 0 && s.cursor < len(s.results) {
				path := s.results[s.cursor].Path
				return s, func() tea.Msg {
					return fileSelectedMsg{path: path}
				}
			}
			return s, nil
		case "esc":
			return s, tea.Quit
		case "up":
			if s.cursor > 0 {
				s.cursor--
				s.ensureCursorVisible()
			} else {
				var cmd tea.Cmd
				s.filterBar, cmd = s.filterBar.Focus()
				return s, cmd
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

	return s, nil
}

func (s searchModel) View() string {
	var b strings.Builder

	b.WriteString(s.filterBar.View())
	b.WriteString("\n")

	// Count / status line — only show "No results" after a search has completed
	// for the current input, not during the debounce period.
	if !s.hasSearchInput() {
		b.WriteString(pick.CountStyle.Render("  Type to search documents"))
	} else if len(s.results) > 0 {
		b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d results", len(s.results))))
	} else if s.searched {
		b.WriteString(pick.CountStyle.Render("  No results"))
	}
	b.WriteString("\n")

	visible := s.visibleRows()
	end := min(s.offset+visible, len(s.results))

	listFocused := !s.filterBar.Focused()
	for i := s.offset; i < end; i++ {
		result := s.results[i]

		prefix := "  "
		selected := i == s.cursor && listFocused
		if selected {
			prefix = pick.SelectedStyle.Render("> ")
		} else if i == s.cursor {
			prefix = pick.CursorDimStyle.Render("> ")
		}

		// Line 1: path
		path := result.Path
		if selected {
			path = pick.SelectedStyle.Render(path)
		}
		b.WriteString(prefix + path + "\n")

		// Line 2: title
		if result.Title != "" {
			b.WriteString("  " + snippetStyle.Render(result.Title) + "\n")
		} else {
			b.WriteString("\n")
		}
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
		b.WriteString("\n")
		b.WriteString(pick.CountStyle.Render("  label:<name>"))
	}

	return b.String()
}
