package browse

import (
	"context"
	"fmt"
	"log/slog"
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
			Placeholder:   "Filter tasks...",
			Hints:         "status:open|done  label:<name>  due:today|week|overdue  from:<path>",
		}),
		client:  client,
		vaultID: vaultID,
	}
}

// loadTasks returns a tea.Cmd that fetches tasks from the API.
func (t tasksModel) loadTasks() tea.Cmd {
	client := t.client
	vaultID := t.vaultID
	filter, _ := t.buildFilter()
	return func() tea.Msg {
		resp, err := client.ListTasks(context.Background(), vaultID, filter)
		if err != nil {
			return tasksLoadedMsg{err: err}
		}
		return tasksLoadedMsg{tasks: resp.Items}
	}
}

// buildFilter maps the current FilterBar result to an apiclient.TaskFilter.
// Returns an error message if an unrecognized filter value is used.
func (t tasksModel) buildFilter() (apiclient.TaskFilter, string) {
	r := t.filterBar.Result()
	f := apiclient.TaskFilter{}
	var filterErr string
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
		default:
			filterErr = fmt.Sprintf("Unknown due:%s (use today, week, or overdue)", v)
		}
	}
	f.Labels = r.FilterAll("label")
	return f, filterErr
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
	return max(t.height-t.filterBar.HeightLines()-2, 1) // filterbar + count + footer
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
		case "esc":
			return t, tea.Quit
		}

		// Delegate remaining keys to filterBar.
		prevFilter, _ := t.buildFilter()
		var cmd tea.Cmd
		t.filterBar, cmd = t.filterBar.Update(msg)
		newFilter, filterErr := t.buildFilter()

		// Show filter validation errors.
		t.statusErr = filterErr

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
// NOTE: update this function when adding new fields to TaskFilter.
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
		slog.Debug("invalid due date format", "due_date", dueDate, "error", err)
		return false
	}
	return t.Before(time.Now().Truncate(24 * time.Hour))
}

// isDueSoon reports whether a due date string (YYYY-MM-DD) is within 7 days.
func isDueSoon(dueDate string) bool {
	t, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		slog.Debug("invalid due date format", "due_date", dueDate, "error", err)
		return false
	}
	return t.Before(time.Now().AddDate(0, 0, 7).Truncate(24 * time.Hour))
}

// renderTaskRow renders checkbox + text + labels + due date for a single task.
func renderTaskRow(task *apiclient.TaskResponse, selected bool) string {
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
	} else if selected {
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

	return checkbox + taskText + labels.String() + due
}

var (
	taskMutedStyle     = lipgloss.NewStyle().Foreground(pick.MutedColor)
	taskAccentStyle    = lipgloss.NewStyle().Foreground(pick.AccentColor)
	taskSecondaryStyle = lipgloss.NewStyle().Foreground(pick.SecondaryColor)
	taskOverdueStyle   = lipgloss.NewStyle().Foreground(pick.ErrorColor)
	taskDueSoonStyle   = lipgloss.NewStyle().Foreground(pick.MatchColor)
	taskDoneStyle      = lipgloss.NewStyle().Foreground(pick.MutedColor).Strikethrough(true)
)

// taskRow represents a single visual row in the grouped tasks view.
// When isHeader is true, only docPath is valid (task is nil).
// When isHeader is false, only task and taskIdx are valid (docPath is unused).
type taskRow struct {
	isHeader bool
	docPath  string
	task     *apiclient.TaskResponse
	taskIdx  int
}

// buildGroupedRows builds a flat list of rows (headers + tasks) for the grouped view.
func (t tasksModel) buildGroupedRows() []taskRow {
	var rows []taskRow
	var lastDoc string
	for i, task := range t.filtered {
		if task.DocumentPath != lastDoc {
			rows = append(rows, taskRow{isHeader: true, docPath: task.DocumentPath})
			lastDoc = task.DocumentPath
		}
		rows = append(rows, taskRow{task: &t.filtered[i], taskIdx: i})
	}
	return rows
}

// viewGrouped renders the grouped-by-document task list into sb.
func (t tasksModel) viewGrouped(sb *strings.Builder, visible int) {
	rows := t.buildGroupedRows()

	// Find which visual row index corresponds to the cursor task.
	cursorRowIdx := -1
	for i, row := range rows {
		if !row.isHeader && row.taskIdx == t.cursor {
			cursorRowIdx = i
			break
		}
	}

	// Compute offset for grouped rows: we want cursorRowIdx to be visible.
	// We reuse t.offset as a row offset for the grouped view. Since ensureCursorVisible
	// works on t.cursor (task index), we compute a row-level offset here.
	rowOffset := t.offset
	// Clamp rowOffset so cursorRowIdx is visible.
	if cursorRowIdx >= 0 {
		if cursorRowIdx < rowOffset {
			rowOffset = cursorRowIdx
		}
		if cursorRowIdx >= rowOffset+visible {
			rowOffset = cursorRowIdx - visible + 1
		}
	}
	if rowOffset < 0 {
		rowOffset = 0
	}

	end := min(rowOffset+visible, len(rows))
	rendered := 0
	for i := rowOffset; i < end; i++ {
		row := rows[i]
		if row.isHeader {
			sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(row.docPath))
		} else {
			selected := row.taskIdx == t.cursor
			prefix := "    "
			if selected {
				prefix = "  > "
			}
			sb.WriteString(prefix + renderTaskRow(row.task, selected))
		}
		sb.WriteString("\n")
		rendered++
	}

	// Pad remaining space
	for i := rendered; i < visible; i++ {
		sb.WriteString("\n")
	}
}

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

	groupHint := "g: group"
	if t.grouped {
		groupHint = "g: ungroup"
	}
	footer := pick.CountStyle.Render("  space: toggle  enter: open  " + groupHint + "  esc: quit")

	if len(t.filtered) == 0 {
		sb.WriteString("\n  No tasks found.")
		sb.WriteString("\n")
		sb.WriteString(footer)
		return sb.String()
	}

	visible := t.visibleRows()

	if t.grouped {
		t.viewGrouped(&sb, visible)
	} else {
		end := min(t.offset+visible, len(t.filtered))
		for i := t.offset; i < end; i++ {
			task := t.filtered[i]
			selected := i == t.cursor

			cursor := "  "
			if selected {
				cursor = "> "
			}

			// Doc path (filename only)
			docName := ""
			if task.DocumentPath != "" {
				docName = " " + taskMutedStyle.Render(path.Base(task.DocumentPath))
			}

			sb.WriteString(cursor + renderTaskRow(&task, selected) + docName)
			sb.WriteString("\n")
		}

		// Pad remaining space
		rendered := end - t.offset
		for i := rendered; i < visible; i++ {
			sb.WriteString("\n")
		}
	}

	sb.WriteString(footer)

	return sb.String()
}
