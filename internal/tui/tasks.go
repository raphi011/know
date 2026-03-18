package tui

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/raphi011/know/internal/apiclient"
)

const maxTaskListHeight = 20

// taskItem wraps a TaskResponse for the list component.
type taskItem struct {
	apiclient.TaskResponse
}

func (t taskItem) FilterValue() string { return t.Text }
func (t taskItem) Title() string       { return t.Text }
func (t taskItem) Description() string { return "" }

// toggleResultMsg is sent after a toggle API call completes.
type toggleResultMsg struct {
	index int
	resp  *apiclient.ToggleTaskResponse
	err   error
}

// refetchResultMsg is sent after refetching the full task list.
type refetchResultMsg struct {
	tasks []apiclient.TaskResponse
	err   error
}

// TaskModel is the top-level Bubbletea model for the task TUI.
type TaskModel struct {
	list     list.Model
	client   *apiclient.Client
	vaultID  string
	filter   apiclient.TaskFilter
	toggling bool
}

// NewTaskModel creates a new task TUI model from pre-fetched tasks.
func NewTaskModel(client *apiclient.Client, vaultID string, filter apiclient.TaskFilter, tasks []apiclient.TaskResponse) TaskModel {
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = taskItem{t}
	}

	delegate := newTaskDelegate(client)
	l := list.New(items, delegate, 0, 0)

	openCount := 0
	for _, t := range tasks {
		if t.Status == "open" {
			openCount++
		}
	}

	title := fmt.Sprintf("Tasks (%d)", len(tasks))
	if filter.Status == "" || filter.Status == "all" {
		title = fmt.Sprintf("Tasks (%d open, %d total)", openCount, len(tasks))
	}
	l.Title = title
	l.SetShowStatusBar(true)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.InfiniteScrolling = true

	// Disable q as quit key — only Esc and Ctrl+C quit.
	l.KeyMap.Quit.SetKeys("esc")

	// Add toggle key to help.
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter", " "),
				key.WithHelp("enter/space", "toggle"),
			),
		}
	}

	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)

	return TaskModel{
		list:    l,
		client:  client,
		vaultID: vaultID,
		filter:  filter,
	}
}

func (m TaskModel) Init() tea.Cmd {
	return nil
}

func (m TaskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := min(msg.Height, maxTaskListHeight)
		m.list.SetSize(msg.Width, h)
		return m, nil

	case tea.KeyPressMsg:
		// Don't handle toggle keys while filtering.
		if m.list.SettingFilter() {
			break
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
			if m.toggling {
				return m, nil
			}
			item, ok := m.list.SelectedItem().(taskItem)
			if !ok {
				return m, nil
			}
			m.toggling = true
			idx := m.list.GlobalIndex()
			return m, toggleTaskCmd(m.client, m.vaultID, item.ID, idx)
		}

	case toggleResultMsg:
		m.toggling = false
		if msg.err != nil {
			cmd := m.list.NewStatusMessage(errorMsgStyle.Render("Toggle failed: " + msg.err.Error()))
			return m, cmd
		}
		if msg.resp.ID != "" {
			// Task returned directly — update in-place.
			updated := taskItem{msg.resp.TaskResponse}
			cmd := m.list.SetItem(msg.index, updated)
			return m, cmd
		}
		// ID changed — refetch full list.
		return m, refetchTasksCmd(m.client, m.vaultID, m.filter)

	case refetchResultMsg:
		if msg.err != nil {
			cmd := m.list.NewStatusMessage(errorMsgStyle.Render("Refetch failed: " + msg.err.Error()))
			return m, cmd
		}
		items := make([]list.Item, len(msg.tasks))
		for i, t := range msg.tasks {
			items[i] = taskItem{t}
		}
		cmd := m.list.SetItems(items)
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m TaskModel) View() tea.View {
	return tea.NewView(m.list.View())
}

// toggleTaskCmd returns a command that toggles a task via the API.
func toggleTaskCmd(client *apiclient.Client, vaultName, taskID string, index int) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ToggleTask(context.Background(), vaultName, taskID)
		return toggleResultMsg{index: index, resp: resp, err: err}
	}
}

// refetchTasksCmd returns a command that refetches all tasks.
func refetchTasksCmd(client *apiclient.Client, vaultID string, filter apiclient.TaskFilter) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListTasks(context.Background(), vaultID, filter)
		if err != nil {
			return refetchResultMsg{err: err}
		}
		return refetchResultMsg{tasks: resp.Tasks}
	}
}

// taskDelegate is a custom item delegate for rendering tasks.
type taskDelegate struct {
	client *apiclient.Client
}

func newTaskDelegate(client *apiclient.Client) taskDelegate {
	return taskDelegate{client: client}
}

func (d taskDelegate) Height() int  { return 1 }
func (d taskDelegate) Spacing() int { return 0 }

func (d taskDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d taskDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	t, ok := item.(taskItem)
	if !ok {
		return
	}

	selected := index == m.Index()

	// Checkbox
	checkbox := "☐ "
	checkboxStyle := lipgloss.NewStyle().Foreground(mutedColor)
	if t.Status == "done" {
		checkbox = "☑ "
		checkboxStyle = lipgloss.NewStyle().Foreground(accentColor)
	}

	// Task text with search highlighting
	textStyle := lipgloss.NewStyle()
	if t.Status == "done" {
		textStyle = textStyle.Foreground(mutedColor).Strikethrough(true)
	}
	highlightStyle := textStyle.UnsetForeground().Foreground(primaryColor).Bold(true)
	taskText := highlightRunes(t.Text, m.MatchesForItem(index), textStyle, highlightStyle)

	// Labels
	var labelParts []string
	labelStyle := lipgloss.NewStyle().Foreground(secondaryColor)
	for _, l := range t.Labels {
		labelParts = append(labelParts, labelStyle.Render("#"+l))
	}

	// Due date
	var duePart string
	if t.DueDate != nil {
		dueStyle := lipgloss.NewStyle().Foreground(mutedColor)
		if isOverdue(*t.DueDate) {
			dueStyle = dueStyle.Foreground(errorColor)
		}
		duePart = dueStyle.Render("due:" + *t.DueDate)
	}

	// Doc path (last folder + filename, dimmed)
	docPart := shortDocPath(t.DocumentPath)
	docStyle := lipgloss.NewStyle().Foreground(mutedColor)

	// Build line
	line := checkboxStyle.Render(checkbox) + taskText
	if len(labelParts) > 0 {
		line += "  " + strings.Join(labelParts, " ")
	}
	if duePart != "" {
		line += "  " + duePart
	}
	if docPart != "" {
		line += "  " + docStyle.Render(docPart)
	}

	// Cursor / selection
	cursor := "  "
	if selected {
		cursor = "> "
		line = lipgloss.NewStyle().Bold(true).Render(line)
	}

	fmt.Fprint(w, cursor+line)
}

// highlightRunes renders text with matched rune positions highlighted.
func highlightRunes(text string, matches []int, normal, highlight lipgloss.Style) string {
	if len(matches) == 0 {
		return normal.Render(text)
	}

	matchSet := make(map[int]bool, len(matches))
	for _, m := range matches {
		matchSet[m] = true
	}

	var sb strings.Builder
	for i, r := range text {
		s := string(r)
		if matchSet[i] {
			sb.WriteString(highlight.Render(s))
		} else {
			sb.WriteString(normal.Render(s))
		}
	}
	return sb.String()
}

// shortDocPath returns the last folder + filename from a document path.
// e.g. "/02 Notes/Programming/Next.js Best Practices.md" → "Programming/Next.js Best Practices.md"
func shortDocPath(docPath string) string {
	if docPath == "" {
		return ""
	}
	dir, file := path.Split(docPath)
	dir = strings.TrimSuffix(dir, "/")
	parent := path.Base(dir)
	if parent == "." || parent == "/" {
		return file
	}
	return parent + "/" + file
}

// isOverdue checks if a YYYY-MM-DD date is before today.
// Parse errors return false — dates are validated upstream by the parser regex.
func isOverdue(dateStr string) bool {
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	today := time.Now().Truncate(24 * time.Hour)
	return d.Before(today)
}
