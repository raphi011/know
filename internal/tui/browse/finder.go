package browse

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/sahilm/fuzzy"
)

// fileSelectedMsg is sent when the user presses Enter on a file.
type fileSelectedMsg struct {
	path string
}

// fileSource implements fuzzy.Source for matching against file path + title.
type fileSource []models.FileEntry

func (s fileSource) String(i int) string {
	e := s[i]
	if e.Title != "" {
		return e.Path + " " + e.Title
	}
	return e.Path
}

func (s fileSource) Len() int { return len(s) }

type finderModel struct {
	input     textinput.Model
	allFiles  []models.FileEntry
	matches   []fuzzy.Match
	cursor    int
	offset    int // scroll offset for visible window
	statusErr string
	width     int
	height    int
}

func newFinder(files []models.FileEntry) finderModel {
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 256
	ti.Prompt = promptStyle.Render("/ ")

	styles := ti.Styles()
	styles.Cursor.Blink = false
	ti.SetStyles(styles)

	f := finderModel{
		input:    ti,
		allFiles: files,
	}
	f.refilter()
	return f
}

func (f finderModel) Init() tea.Cmd {
	return f.input.Focus()
}

func (f finderModel) Update(msg tea.Msg) (finderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(f.matches) > 0 && f.cursor < len(f.matches) {
				idx := f.matches[f.cursor].Index
				return f, func() tea.Msg {
					return fileSelectedMsg{path: f.allFiles[idx].Path}
				}
			}
			return f, nil
		case "up":
			if f.cursor > 0 {
				f.cursor--
				f.ensureCursorVisible()
			}
			return f, nil
		case "down":
			if f.cursor < len(f.matches)-1 {
				f.cursor++
				f.ensureCursorVisible()
			}
			return f, nil
		}
	}

	// Delegate to text input
	var cmd tea.Cmd
	prev := f.input.Value()
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != prev {
		f.statusErr = ""
		f.refilter()
	}
	return f, cmd
}

func (f finderModel) View() string {
	var b strings.Builder

	// Input line
	b.WriteString(f.input.View())
	b.WriteString("\n")

	// Result count
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d/%d files", len(f.matches), len(f.allFiles))))
	b.WriteString("\n")

	// File list
	visible := f.visibleRows()
	end := min(f.offset+visible, len(f.matches))

	for i := f.offset; i < end; i++ {
		m := f.matches[i]
		entry := f.allFiles[m.Index]

		prefix := "  "
		style := normalStyle
		if i == f.cursor {
			prefix = "> "
			style = selectedStyle
		}

		// Render path with highlighted match characters
		line := renderHighlighted(entry.Path, m.MatchedIndexes, style)

		// Append title if present and different from path
		if entry.Title != "" {
			line += " " + titleStyle.Render(entry.Title)
		}

		b.WriteString(prefix + line + "\n")
	}

	// Pad remaining space
	rendered := end - f.offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n")
	}

	// Error line (if any)
	if f.statusErr != "" {
		b.WriteString(errStyle.Render("  " + f.statusErr))
		b.WriteString("\n")
	}

	// Help line
	b.WriteString(countStyle.Render("  enter: open  esc: quit"))

	return b.String()
}

// visibleRows returns how many file rows fit in the current terminal height.
// Reserves 4 lines: input, count, help, and a blank line.
func (f finderModel) visibleRows() int {
	rows := max(f.height-4, 1)
	return rows
}

func (f *finderModel) refilter() {
	query := f.input.Value()
	if query == "" {
		// Show all files when query is empty
		f.matches = make([]fuzzy.Match, len(f.allFiles))
		for i := range f.allFiles {
			f.matches[i] = fuzzy.Match{Index: i}
		}
	} else {
		f.matches = fuzzy.FindFrom(query, fileSource(f.allFiles))
	}

	// Clamp cursor
	if f.cursor >= len(f.matches) {
		f.cursor = max(0, len(f.matches)-1)
	}
	f.ensureCursorVisible()
}

func (f *finderModel) ensureCursorVisible() {
	visible := f.visibleRows()
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if f.cursor >= f.offset+visible {
		f.offset = f.cursor - visible + 1
	}
}

// renderHighlighted renders a string with matched character indices highlighted.
func renderHighlighted(s string, matchedIndexes []int, baseStyle lipgloss.Style) string {
	if len(matchedIndexes) == 0 {
		return baseStyle.Render(s)
	}

	matchSet := make(map[int]bool, len(matchedIndexes))
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	var b strings.Builder
	for i, ch := range s {
		c := string(ch)
		if matchSet[i] {
			b.WriteString(matchStyle.Render(c))
		} else {
			b.WriteString(baseStyle.Render(c))
		}
	}
	return b.String()
}
