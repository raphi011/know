package pick

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/sahilm/fuzzy"
)

// FileSource implements fuzzy.Source for matching against file path + title.
type FileSource []models.FileEntry

func (s FileSource) String(i int) string {
	e := s[i]
	if e.Title != "" {
		return e.Path + " " + e.Title
	}
	return e.Path
}

func (s FileSource) Len() int { return len(s) }

// Model is a bubbletea model for fuzzy-picking a file.
type Model struct {
	Input    textinput.Model
	AllFiles []models.FileEntry
	Matches  []fuzzy.Match
	Cursor   int
	Offset   int
	Width    int
	Height   int
	Selected string // set on Enter; empty if cancelled
}

func NewModel(files []models.FileEntry) Model {
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 256
	ti.Prompt = "/ "

	styles := ti.Styles()
	styles.Cursor.Blink = false
	styles.Focused.Prompt = PromptStyle
	ti.SetStyles(styles)

	m := Model{
		Input:    ti,
		AllFiles: files,
	}
	m.Input.Focus()
	m.Refilter()
	return m
}

func (m Model) Init() tea.Cmd {
	// Input is already focused from NewModel; re-call to get the focus cmd.
	return m.Input.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(m.Matches) > 0 && m.Cursor < len(m.Matches) {
				idx := m.Matches[m.Cursor].Index
				m.Selected = m.AllFiles[idx].Path
			}
			return m, tea.Quit
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "up":
			if m.Cursor > 0 {
				m.Cursor--
				m.EnsureCursorVisible()
			}
			return m, nil
		case "down":
			if m.Cursor < len(m.Matches)-1 {
				m.Cursor++
				m.EnsureCursorVisible()
			}
			return m, nil
		case "pgup":
			m.Cursor = max(m.Cursor-m.VisibleRows(), 0)
			m.EnsureCursorVisible()
			return m, nil
		case "pgdown":
			m.Cursor = min(m.Cursor+m.VisibleRows(), max(len(m.Matches)-1, 0))
			m.EnsureCursorVisible()
			return m, nil
		}
	}

	// Delegate to text input.
	var cmd tea.Cmd
	prev := m.Input.Value()
	m.Input, cmd = m.Input.Update(msg)
	if m.Input.Value() != prev {
		m.Refilter()
	}
	return m, cmd
}

func (m Model) View() tea.View {
	var b strings.Builder

	b.WriteString(m.Input.View())
	b.WriteString("\n")

	b.WriteString(CountStyle.Render(fmt.Sprintf("  %d/%d files", len(m.Matches), len(m.AllFiles))))
	b.WriteString("\n")

	visible := m.VisibleRows()
	end := min(m.Offset+visible, len(m.Matches))

	for i := m.Offset; i < end; i++ {
		match := m.Matches[i]
		entry := m.AllFiles[match.Index]

		prefix := "  "
		style := NormalStyle
		if i == m.Cursor {
			prefix = "> "
			style = SelectedStyle
		}

		line := RenderHighlighted(entry.Path, match.MatchedIndexes, style)
		if entry.Title != "" {
			line += " " + TitleStyle.Render(entry.Title)
		}

		b.WriteString(prefix + line + "\n")
	}

	// Pad remaining space.
	rendered := end - m.Offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n")
	}

	b.WriteString(CountStyle.Render("  enter: select  esc: cancel"))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// SetSize updates the picker dimensions and propagates width to the text input.
func (m *Model) SetSize(width, height int) {
	m.Width = width
	m.Height = height
	m.Input.SetWidth(width - len(m.Input.Prompt))
}

func (m Model) VisibleRows() int {
	return max(m.Height-5, 1) // input + count + gap + footer(blank+hotkeys)
}

func (m *Model) Refilter() {
	query := m.Input.Value()
	if query == "" {
		m.Matches = make([]fuzzy.Match, len(m.AllFiles))
		for i := range m.AllFiles {
			m.Matches[i] = fuzzy.Match{Index: i}
		}
	} else {
		m.Matches = fuzzy.FindFrom(query, FileSource(m.AllFiles))
	}

	if m.Cursor >= len(m.Matches) {
		m.Cursor = max(0, len(m.Matches)-1)
	}
	m.EnsureCursorVisible()
}

func (m *Model) EnsureCursorVisible() {
	visible := m.VisibleRows()
	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	}
	if m.Cursor >= m.Offset+visible {
		m.Offset = m.Cursor - visible + 1
	}
}

// RenderHighlighted renders a string with matched character indices highlighted.
func RenderHighlighted(s string, matchedIndexes []int, baseStyle lipgloss.Style) string {
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
			b.WriteString(MatchStyle.Render(c))
		} else {
			b.WriteString(baseStyle.Render(c))
		}
	}
	return b.String()
}

// Run launches the fuzzy picker TUI and returns the selected file path.
// Returns an empty string if the user cancelled (Escape/Ctrl+C).
func Run(files []models.FileEntry, opts ...tea.ProgramOption) (string, error) {
	m := NewModel(files)
	p := tea.NewProgram(m, opts...)
	final, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("pick: %w", err)
	}
	result, ok := final.(Model)
	if !ok {
		return "", fmt.Errorf("pick: unexpected model type %T", final)
	}
	return result.Selected, nil
}
