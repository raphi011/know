package browse

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/tui/pick"
)

// FilterBarConfig declares which filter keys a tab supports.
type FilterBarConfig struct {
	SupportedKeys []string
	Placeholder   string
	Hints         string // e.g. "status:open|done  label:<name>  due:today|week|overdue"
}

// FilterResult holds the parsed output of the filter input.
type FilterResult struct {
	Query   string
	Filters map[string][]string
}

// Filter returns the first value for a key, or empty string.
func (r FilterResult) Filter(key string) string {
	vals := r.Filters[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// FilterAll returns all values for a key.
func (r FilterResult) FilterAll(key string) []string {
	return r.Filters[key]
}

// FilterBar wraps textarea.Model and parses key:value filter tokens.
type FilterBar struct {
	input     textarea.Model
	config    FilterBarConfig
	result    FilterResult
	supported map[string]bool
}

// NewFilterBar creates a FilterBar with the given config.
func NewFilterBar(config FilterBarConfig) FilterBar {
	ta := textarea.New()
	ta.Placeholder = config.Placeholder
	ta.CharLimit = 256
	ta.Prompt = "/ "
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.MaxHeight = 10

	// Enter triggers search; shift+enter inserts newline.
	km := ta.KeyMap
	km.InsertNewline.SetKeys("shift+enter")
	ta.KeyMap = km

	styles := ta.Styles()
	styles.Cursor.Blink = false
	styles.Focused.Prompt = pick.PromptStyle
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(pick.MutedColor)
	// Clear default dark-mode styles that cause visual artifacts:
	// - CursorLine Background("0") paints the entire 1-line textarea black
	// - EndOfBuffer Foreground("0") can produce rendering artifacts on padding lines
	styles.Focused.Base = lipgloss.NewStyle()
	styles.Blurred.Base = lipgloss.NewStyle()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	styles.Focused.EndOfBuffer = lipgloss.NewStyle()
	styles.Blurred.EndOfBuffer = lipgloss.NewStyle()
	ta.SetStyles(styles)

	supported := make(map[string]bool, len(config.SupportedKeys))
	for _, k := range config.SupportedKeys {
		supported[k] = true
	}

	return FilterBar{
		input:     ta,
		config:    config,
		result:    parseFilterInput("", supported),
		supported: supported,
	}
}

// parseFilterInput splits input into query text and key:value filters.
func parseFilterInput(input string, supported map[string]bool) FilterResult {
	tokens := strings.Fields(input)
	filters := make(map[string][]string)
	var queryParts []string

	for _, token := range tokens {
		key, value, found := strings.Cut(token, ":")
		if found && value != "" && supported[key] {
			filters[key] = append(filters[key], value)
		} else {
			queryParts = append(queryParts, token)
		}
	}

	return FilterResult{
		Query:   strings.Join(queryParts, " "),
		Filters: filters,
	}
}

// Update handles messages and returns an updated FilterBar.
func (f FilterBar) Update(msg tea.Msg) (FilterBar, tea.Cmd) {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	f.result = parseFilterInput(f.input.Value(), f.supported)
	f.resizeInput()
	return f, cmd
}

var filterHintStyle = lipgloss.NewStyle().Foreground(pick.MutedColor)

// View renders the filter bar with optional filter hints underneath.
func (f FilterBar) View() string {
	// TrimRight removes trailing newlines from the textarea's viewport output
	// to prevent EndOfBuffer padding lines from leaking into the layout.
	s := strings.TrimRight(f.input.View(), "\n")
	if f.config.Hints != "" {
		s += "\n  " + filterHintStyle.Render(f.config.Hints)
	}
	return s
}

// SetWidth sets the width of the underlying text input.
// Subtracts 4 for the prompt ("/ ") and surrounding padding.
func (f *FilterBar) SetWidth(width int) {
	f.input.SetWidth(max(width-4, 1))
}

// HeightLines returns the number of lines the filter bar occupies.
func (f FilterBar) HeightLines() int {
	h := max(f.input.LineCount(), 1)
	if f.config.Hints != "" {
		h++
	}
	return h
}

// Result returns the current parsed filter result.
func (f FilterBar) Result() FilterResult {
	return f.result
}

// Focus focuses the text input.
func (f FilterBar) Focus() (FilterBar, tea.Cmd) {
	cmd := f.input.Focus()
	return f, cmd
}

// Blur removes focus from the text input.
func (f FilterBar) Blur() FilterBar {
	f.input.Blur()
	return f
}

// Focused returns whether the filter bar's text input is focused.
func (f FilterBar) Focused() bool {
	return f.input.Focused()
}

// Value returns the raw text input value.
func (f FilterBar) Value() string {
	return f.input.Value()
}

// SetValue sets the raw text input value, re-parses, and resizes the textarea.
// Note: value receiver — cannot call pointer-receiver resizeInput, so resize is inlined.
func (f FilterBar) SetValue(v string) FilterBar {
	f.input.SetValue(v)
	f.result = parseFilterInput(v, f.supported)
	f.input.SetHeight(min(max(f.input.LineCount(), 1), f.input.MaxHeight))
	return f
}

// resizeInput adjusts the textarea height to match its content.
func (f *FilterBar) resizeInput() {
	h := min(max(f.input.LineCount(), 1), f.input.MaxHeight)
	f.input.SetHeight(h)
}
