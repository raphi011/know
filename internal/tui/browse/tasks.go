package browse

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/tui/pick"
)

// tasksLoadedMsg is sent when the task list has been fetched.
type tasksLoadedMsg struct {
	tasks []apiclient.TaskResponse
	err   error
}

// taskToggledMsg is sent after a task toggle completes.
type taskToggledMsg struct {
	resp *apiclient.ToggleTaskResponse
	err  error
}

type tasksModel struct {
	allTasks  []apiclient.TaskResponse
	filtered  []apiclient.TaskResponse
	cursor    int
	offset    int
	width     int
	height    int
	loaded    bool
	toggling  bool
	grouped   bool
	statusErr string
	statusOK  string
	filterBar FilterBar
	client    *apiclient.Client
	vaultID   string
}

func newTasksModel(client *apiclient.Client, vaultID string) tasksModel {
	return tasksModel{
		filterBar: NewFilterBar(FilterBarConfig{
			SupportedKeys: []string{"status", "label", "due", "from"},
			Placeholder:   "Filter tasks... (status:open label:go due:overdue)",
		}),
		client:  client,
		vaultID: vaultID,
	}
}

// loadTasks returns a tea.Cmd that fetches tasks from the API.
func (t tasksModel) loadTasks() tea.Cmd {
	client := t.client
	vaultID := t.vaultID
	filter := t.buildFilter()
	return func() tea.Msg {
		resp, err := client.ListTasks(context.Background(), vaultID, filter)
		if err != nil {
			return tasksLoadedMsg{err: err}
		}
		return tasksLoadedMsg{tasks: resp.Items}
	}
}

// buildFilter maps the current FilterBar result to an apiclient.TaskFilter.
func (t tasksModel) buildFilter() apiclient.TaskFilter {
	r := t.filterBar.Result()
	f := apiclient.TaskFilter{}
	if v := r.Filter("status"); v != "" {
		f.Status = v
	}
	if v := r.Filter("from"); v != "" {
		f.Path = v
	}
	if v := r.Filter("due"); v != "" {
		now := time.Now()
		today := now.Format("2006-01-02")
		switch v {
		case "today":
			f.DueAfter = today
			f.DueBefore = now.AddDate(0, 0, 1).Format("2006-01-02")
		case "week":
			f.DueAfter = today
			f.DueBefore = now.AddDate(0, 0, 7).Format("2006-01-02")
		case "overdue":
			f.DueBefore = today
		}
	}
	f.Labels = r.FilterAll("label")
	return f
}

// applyFilter applies fuzzy text matching and re-sorts filtered tasks.
func (t *tasksModel) applyFilter() {
	query := strings.ToLower(t.filterBar.Result().Query)
	t.filtered = nil
	for _, task := range t.allTasks {
		if query == "" ||
			strings.Contains(strings.ToLower(task.Text), query) ||
			strings.Contains(strings.ToLower(task.DocumentPath), query) {
			t.filtered = append(t.filtered, task)
		}
	}
	t.sortTasks()
	t.cursor = min(t.cursor, max(len(t.filtered)-1, 0))
	t.ensureCursorVisible()
}

// sortTasks sorts filtered tasks: open before done, overdue first, then by due
// date ascending, no-due last, then by document path.
func (t *tasksModel) sortTasks() {
	slices.SortFunc(t.filtered, func(a, b apiclient.TaskResponse) int {
		// Open before done
		if a.Status != b.Status {
			if a.Status == "open" {
				return -1
			}
			return 1
		}
		// Both have due dates: sort ascending
		if a.DueDate != nil && b.DueDate != nil {
			if *a.DueDate != *b.DueDate {
				return strings.Compare(*a.DueDate, *b.DueDate)
			}
		}
		// Has due date before no due date
		if a.DueDate != nil && b.DueDate == nil {
			return -1
		}
		if a.DueDate == nil && b.DueDate != nil {
			return 1
		}
		// By document path
		return strings.Compare(a.DocumentPath, b.DocumentPath)
	})
}

func (t tasksModel) visibleRows() int {
	return max(t.height-3, 1) // filterbar + count + footer
}

func (t *tasksModel) ensureCursorVisible() {
	visible := t.visibleRows()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+visible {
		t.offset = t.cursor - visible + 1
	}
}

func (t tasksModel) selectedTask() *apiclient.TaskResponse {
	if len(t.filtered) == 0 || t.cursor >= len(t.filtered) {
		return nil
	}
	return &t.filtered[t.cursor]
}

func (t tasksModel) toggleTask(taskID string) tea.Cmd {
	client := t.client
	vaultID := t.vaultID
	return func() tea.Msg {
		resp, err := client.ToggleTask(context.Background(), vaultID, taskID)
		return taskToggledMsg{resp: resp, err: err}
	}
}

func (t tasksModel) Update(msg tea.Msg) (tasksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tasksLoadedMsg:
		t.loaded = true
		if msg.err != nil {
			t.statusErr = fmt.Sprintf("Failed to load tasks: %v", msg.err)
			return t, nil
		}
		t.allTasks = msg.tasks
		t.statusErr = ""
		t.cursor = 0
		t.offset = 0
		t.applyFilter()
		return t, nil

	case taskToggledMsg:
		t.toggling = false
		if msg.err != nil {
			t.statusErr = fmt.Sprintf("Failed to toggle task: %v", msg.err)
			return t, nil
		}
		t.statusErr = ""
		t.statusOK = "Task updated"
		return t, t.loadTasks()

	case tea.KeyPressMsg:
		t.statusErr = ""
		t.statusOK = ""

		switch msg.String() {
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				t.ensureCursorVisible()
			}
			return t, nil
		case "down", "j":
			if t.cursor < len(t.filtered)-1 {
				t.cursor++
				t.ensureCursorVisible()
			}
			return t, nil
		case "pgup":
			t.cursor = max(t.cursor-t.visibleRows(), 0)
			t.ensureCursorVisible()
			return t, nil
		case "pgdown":
			t.cursor = min(t.cursor+t.visibleRows(), max(len(t.filtered)-1, 0))
			t.ensureCursorVisible()
			return t, nil
		case "space":
			if t.toggling {
				return t, nil
			}
			task := t.selectedTask()
			if task == nil {
				return t, nil
			}
			t.toggling = true
			return t, t.toggleTask(task.ID)
		case "enter":
			task := t.selectedTask()
			if task == nil {
				return t, nil
			}
			return t, func() tea.Msg {
				return fileSelectedMsg{path: task.DocumentPath}
			}
		case "g":
			t.grouped = !t.grouped
			return t, nil
		}

		// Delegate remaining keys to filterBar.
		prevFilter := t.buildFilter()
		var cmd tea.Cmd
		t.filterBar, cmd = t.filterBar.Update(msg)
		newFilter := t.buildFilter()

		// If structured filters changed, re-fetch from API.
		if !filtersEqual(prevFilter, newFilter) {
			return t, t.loadTasks()
		}
		// If only query text changed, apply locally.
		t.applyFilter()
		return t, cmd
	}

	return t, nil
}

// filtersEqual compares two TaskFilter structs for equality (ignores query text).
func filtersEqual(a, b apiclient.TaskFilter) bool {
	if a.Status != b.Status || a.Path != b.Path || a.DueAfter != b.DueAfter || a.DueBefore != b.DueBefore {
		return false
	}
	if len(a.Labels) != len(b.Labels) {
		return false
	}
	for i := range a.Labels {
		if a.Labels[i] != b.Labels[i] {
			return false
		}
	}
	return true
}

// isOverdue reports whether a due date string (YYYY-MM-DD) is before today.
func isOverdue(dueDate string) bool {
	t, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return false
	}
	return t.Before(time.Now().Truncate(24 * time.Hour))
}

// isDueSoon reports whether a due date string (YYYY-MM-DD) is within 7 days.
func isDueSoon(dueDate string) bool {
	t, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return false
	}
	return t.Before(time.Now().AddDate(0, 0, 7).Truncate(24 * time.Hour))
}

var (
	taskMutedStyle     = lipgloss.NewStyle().Foreground(pick.MutedColor)
	taskAccentStyle    = lipgloss.NewStyle().Foreground(pick.AccentColor)
	taskSecondaryStyle = lipgloss.NewStyle().Foreground(pick.SecondaryColor)
	taskOverdueStyle   = lipgloss.NewStyle().Foreground(pick.ErrorColor)
	taskDueSoonStyle   = lipgloss.NewStyle().Foreground(pick.MatchColor)
	taskDoneStyle      = lipgloss.NewStyle().Foreground(pick.MutedColor).Strikethrough(true)
)

func (t tasksModel) View() string {
	var sb strings.Builder

	if !t.loaded {
		sb.WriteString("  Loading tasks...")
		return sb.String()
	}

	if t.statusErr != "" {
		sb.WriteString(errStyle.Render("  " + t.statusErr))
		sb.WriteString("\n")
	} else if t.statusOK != "" {
		sb.WriteString(pick.CountStyle.Render("  " + t.statusOK))
		sb.WriteString("\n")
	}

	sb.WriteString(t.filterBar.View())
	sb.WriteString("\n")

	countStr := fmt.Sprintf("%d tasks", len(t.filtered))
	if len(t.filtered) != len(t.allTasks) {
		countStr = fmt.Sprintf("%d of %d tasks", len(t.filtered), len(t.allTasks))
	}
	sb.WriteString(pick.CountStyle.Render("  " + countStr))
	sb.WriteString("\n")

	if len(t.filtered) == 0 {
		sb.WriteString("\n  No tasks found.")
		sb.WriteString("\n")
		sb.WriteString(pick.CountStyle.Render("  space: toggle  enter: open  g: group  esc: quit"))
		return sb.String()
	}

	visible := t.visibleRows()
	end := min(t.offset+visible, len(t.filtered))
	for i := t.offset; i < end; i++ {
		task := t.filtered[i]

		// Cursor
		cursor := "  "
		if i == t.cursor {
			cursor = "> "
		}

		// Checkbox
		var checkbox string
		if task.Status == "done" {
			checkbox = taskAccentStyle.Render("☑ ")
		} else {
			checkbox = taskMutedStyle.Render("☐ ")
		}

		// Task text
		var taskText string
		if task.Status == "done" {
			taskText = taskDoneStyle.Render(task.Text)
		} else if i == t.cursor {
			taskText = pick.SelectedStyle.Render(task.Text)
		} else {
			taskText = pick.NormalStyle.Render(task.Text)
		}

		// Labels
		var labels strings.Builder
		for _, label := range task.Labels {
			labels.WriteString(" " + taskSecondaryStyle.Render("#"+label))
		}

		// Due date
		var due string
		if task.DueDate != nil && *task.DueDate != "" {
			dueStr := " due:" + *task.DueDate
			if isOverdue(*task.DueDate) {
				due = taskOverdueStyle.Render(dueStr)
			} else if isDueSoon(*task.DueDate) {
				due = taskDueSoonStyle.Render(dueStr)
			} else {
				due = taskMutedStyle.Render(dueStr)
			}
		}

		// Doc path (filename only)
		docName := ""
		if task.DocumentPath != "" {
			docName = " " + taskMutedStyle.Render(path.Base(task.DocumentPath))
		}

		line := cursor + checkbox + taskText + labels.String() + due + docName
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Pad remaining space
	rendered := end - t.offset
	for i := rendered; i < visible; i++ {
		sb.WriteString("\n")
	}

	sb.WriteString(pick.CountStyle.Render("  space: toggle  enter: open  g: group  esc: quit"))

	return sb.String()
}
