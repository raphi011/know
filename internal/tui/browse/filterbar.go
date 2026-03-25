package browse

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/tui/pick"
)

// FilterBarConfig declares which filter keys a tab supports.
type FilterBarConfig struct {
	SupportedKeys []string
	Placeholder   string
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

// FilterBar wraps textinput.Model and parses key:value filter tokens.
type FilterBar struct {
	input     textinput.Model
	config    FilterBarConfig
	result    FilterResult
	supported map[string]bool
}

// NewFilterBar creates a FilterBar with the given config.
func NewFilterBar(config FilterBarConfig) FilterBar {
	ti := textinput.New()
	ti.Placeholder = config.Placeholder
	ti.CharLimit = 256
	ti.Prompt = "/ "

	styles := ti.Styles()
	styles.Cursor.Blink = false
	styles.Focused.Prompt = pick.PromptStyle
	ti.SetStyles(styles)

	supported := make(map[string]bool, len(config.SupportedKeys))
	for _, k := range config.SupportedKeys {
		supported[k] = true
	}

	return FilterBar{
		input:     ti,
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
	return f, cmd
}

// View renders the filter bar.
func (f FilterBar) View() string {
	return f.input.View()
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

// Value returns the raw text input value.
func (f FilterBar) Value() string {
	return f.input.Value()
}

// SetValue sets the raw text input value and re-parses.
func (f FilterBar) SetValue(v string) FilterBar {
	f.input.SetValue(v)
	f.result = parseFilterInput(v, f.supported)
	return f
}
