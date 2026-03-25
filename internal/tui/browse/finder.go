package browse

import (
	"fmt"
	"strings"

	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/pick"
)

// fileSelectedMsg is sent when the user presses Enter on a file.
type fileSelectedMsg struct {
	path string
}

type finderModel struct {
	picker    pick.Model
	allFiles  []models.FileEntry // original unfiltered list
	supported map[string]bool
	statusErr string
}

func newFinder(files []models.FileEntry) finderModel {
	return finderModel{
		picker:   pick.NewModel(files),
		allFiles: files,
		supported: map[string]bool{
			"label": true,
			"from":  true,
		},
	}
}

func (f finderModel) Init() tea.Cmd {
	return f.picker.Init()
}

func (f finderModel) Update(msg tea.Msg) (finderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(f.picker.Matches) > 0 && f.picker.Cursor < len(f.picker.Matches) {
				idx := f.picker.Matches[f.picker.Cursor].Index
				return f, func() tea.Msg {
					return fileSelectedMsg{path: f.picker.AllFiles[idx].Path}
				}
			}
			return f, nil
		case "esc":
			return f, tea.Quit
		}
	}

	// Delegate to picker for text input, up/down, resize.
	var cmd tea.Cmd
	prev := f.picker.Input.Value()
	updated, cmd := f.picker.Update(msg)
	f.picker = updated.(pick.Model)
	if f.picker.Input.Value() != prev {
		f.statusErr = ""
		f.applyFilters()
	}
	return f, cmd
}

// applyFilters pre-filters allFiles based on structured filter tokens in the
// picker input, then updates picker.AllFiles and re-runs fuzzy matching.
func (f *finderModel) applyFilters() {
	result := parseFilterInput(f.picker.Input.Value(), f.supported)

	if len(result.Filters) == 0 {
		f.picker.AllFiles = f.allFiles
		f.picker.Refilter()
		return
	}

	var filtered []models.FileEntry
	for _, file := range f.allFiles {
		if f.matchesFilters(file, result) {
			filtered = append(filtered, file)
		}
	}
	f.picker.AllFiles = filtered
	f.picker.Refilter()
}

// matchesFilters returns true if a file satisfies all structured filters.
func (f *finderModel) matchesFilters(file models.FileEntry, result FilterResult) bool {
	// from: filter by path prefix
	if from := result.Filter("from"); from != "" {
		if !strings.HasPrefix(file.Path, from) {
			return false
		}
	}

	// label: filter by label — all specified labels must be present
	if labels := result.FilterAll("label"); len(labels) > 0 {
		for _, label := range labels {
			if !slices.Contains(file.Labels, label) {
				return false
			}
		}
	}

	return true
}

func (f finderModel) View() string {
	var b strings.Builder

	b.WriteString(f.picker.Input.View())
	b.WriteString("\n")

	b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d/%d files", len(f.picker.Matches), len(f.picker.AllFiles))))
	b.WriteString("\n")

	visible := f.picker.VisibleRows()
	end := min(f.picker.Offset+visible, len(f.picker.Matches))

	for i := f.picker.Offset; i < end; i++ {
		m := f.picker.Matches[i]
		entry := f.picker.AllFiles[m.Index]

		prefix := "  "
		style := pick.NormalStyle
		if i == f.picker.Cursor {
			prefix = "> "
			style = pick.SelectedStyle
		}

		line := pick.RenderHighlighted(entry.Path, m.MatchedIndexes, style)
		if entry.Title != "" {
			line += " " + pick.TitleStyle.Render(entry.Title)
		}

		b.WriteString(prefix + line + "\n")
	}

	// Pad remaining space.
	rendered := end - f.picker.Offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n")
	}

	if f.statusErr != "" {
		b.WriteString(errStyle.Render("  " + f.statusErr))
		b.WriteString("\n")
	}

	b.WriteString(pick.CountStyle.Render("  enter: open  esc: quit"))

	return b.String()
}
