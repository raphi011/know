package browse

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/apiclient"
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
		switch msg.String() {
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				t.ensureCursorVisible()
			}
		case "down", "j":
			if t.cursor < len(t.filtered)-1 {
				t.cursor++
				t.ensureCursorVisible()
			}
		}
	}
	return t, nil
}

func (t tasksModel) View() string {
	var sb strings.Builder

	if !t.loaded {
		sb.WriteString("  Loading tasks...")
		return sb.String()
	}

	if t.statusErr != "" {
		sb.WriteString(errStyle.Render(t.statusErr))
		sb.WriteString("\n")
	}

	sb.WriteString(t.filterBar.View())
	sb.WriteString("\n")

	countStr := fmt.Sprintf("%d tasks", len(t.filtered))
	if len(t.filtered) != len(t.allTasks) {
		countStr = fmt.Sprintf("%d of %d tasks", len(t.filtered), len(t.allTasks))
	}
	sb.WriteString(countStr)
	sb.WriteString("\n")

	if len(t.filtered) == 0 {
		sb.WriteString("\n  No tasks found.")
		return sb.String()
	}

	visible := t.visibleRows()
	end := min(t.offset+visible, len(t.filtered))
	for i := t.offset; i < end; i++ {
		task := t.filtered[i]
		checkbox := "[ ]"
		if task.Status == "done" {
			checkbox = "[x]"
		}
		line := fmt.Sprintf("%s %s  %s", checkbox, task.Text, task.DocumentPath)
		if i == t.cursor {
			sb.WriteString("> " + line)
		} else {
			sb.WriteString("  " + line)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n  enter: open document  x: toggle")

	return sb.String()
}
