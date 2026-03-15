# `know task` Interactive Command — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `know task` CLI command that opens an interactive Bubbletea v2 TUI for browsing and toggling tasks with label/status/date/path filtering.

**Architecture:** CLI command fetches tasks via REST API, launches a Bubbletea program with a `bubbles/v2/list` component. Custom item delegate renders checkboxes with labels and due dates. Toggle sends `POST /api/tasks/{id}/toggle` and updates the list in-place.

**Tech Stack:** Go, Cobra, Bubbletea v2, Bubbles v2 list, Lipgloss v2, REST API

**Spec:** `docs/superpowers/specs/2026-03-15-task-command-design.md`

---

## Chunk 1: API Client + CLI Command + TUI

### Task 1: Add apiclient task methods

**Files:**
- Create: `internal/apiclient/tasks.go`

- [ ] **Step 1: Create `internal/apiclient/tasks.go`**

```go
package apiclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// TaskFilter holds query parameters for listing tasks.
type TaskFilter struct {
	Status    string // "open", "done", or "" (all)
	Labels    []string
	DueBefore string // YYYY-MM-DD
	DueAfter  string // YYYY-MM-DD
	Path      string // exact doc path or folder prefix (ending with /)
}

// TaskResponse is the JSON representation of a task from the REST API.
type TaskResponse struct {
	ID            string   `json:"id"`
	DocumentPath  string   `json:"documentPath,omitempty"`
	DocumentTitle string   `json:"documentTitle,omitempty"`
	Status        string   `json:"status"`
	Text          string   `json:"text"`
	Labels        []string `json:"labels"`
	DueDate       *string  `json:"dueDate,omitempty"`
	HeadingPath   *string  `json:"headingPath,omitempty"`
	LineNumber    int      `json:"lineNumber"`
}

// TaskListResponse is the JSON envelope for listing tasks.
type TaskListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

// ToggleTaskResponse holds the response from toggling a task.
// If ID is non-empty, the task was returned directly.
// If Message is non-empty, the task ID changed during re-ingestion.
type ToggleTaskResponse struct {
	TaskResponse
	Message string `json:"message,omitempty"`
}

// ListTasks fetches tasks matching the given filter.
func (c *Client) ListTasks(ctx context.Context, vaultID string, filter TaskFilter) (*TaskListResponse, error) {
	q := url.Values{"vault": {vaultID}, "limit": {"1000"}}

	if filter.Status != "" && filter.Status != "all" {
		q.Set("status", filter.Status)
	}
	if len(filter.Labels) > 0 {
		q.Set("labels", strings.Join(filter.Labels, ","))
	}
	if filter.DueBefore != "" {
		q.Set("due_before", filter.DueBefore)
	}
	if filter.DueAfter != "" {
		q.Set("due_after", filter.DueAfter)
	}
	if filter.Path != "" {
		if strings.HasSuffix(filter.Path, "/") {
			q.Set("folder", filter.Path)
		} else {
			q.Set("path", filter.Path)
		}
	}

	var resp TaskListResponse
	if err := c.Get(ctx, "/api/tasks?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return &resp, nil
}

// ToggleTask toggles a task's status (open↔done).
func (c *Client) ToggleTask(ctx context.Context, taskID string) (*ToggleTaskResponse, error) {
	var resp ToggleTaskResponse
	if err := c.Post(ctx, "/api/tasks/"+url.PathEscape(taskID)+"/toggle", nil, &resp); err != nil {
		return nil, fmt.Errorf("toggle task: %w", err)
	}
	return &resp, nil
}
```

- [ ] **Step 2: Build to verify compilation**

Run: `just build`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add internal/apiclient/tasks.go
git commit -m "feat(apiclient): add ListTasks and ToggleTask methods"
```

---

### Task 2: Create the task TUI model

**Files:**
- Create: `internal/tui/tasks.go`

The TUI model wraps `bubbles/v2/list` with a custom item type and delegate. Key behaviors:
- Custom rendering: checkbox + text + labels + due date
- `enter`/`space` toggles via API, updates in-place
- `q` quit binding disabled (only Esc/Ctrl+C quit)
- Status message on toggle error

- [ ] **Step 1: Create `internal/tui/tasks.go`**

```go
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/raphi011/know/internal/apiclient"
)

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
	list    list.Model
	client  *apiclient.Client
	vaultID string
	filter  apiclient.TaskFilter
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
		m.list.SetSize(msg.Width, msg.Height)
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
			return m, toggleTaskCmd(m.client, item.ID, idx)
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
func toggleTaskCmd(client *apiclient.Client, taskID string, index int) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ToggleTask(context.Background(), taskID)
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

	// Checkbox
	checkbox := "☐ "
	checkboxStyle := lipgloss.NewStyle().Foreground(mutedColor)
	if t.Status == "done" {
		checkbox = "☑ "
		checkboxStyle = lipgloss.NewStyle().Foreground(accentColor)
	}

	// Task text
	textStyle := lipgloss.NewStyle()
	if t.Status == "done" {
		textStyle = textStyle.Foreground(mutedColor).Strikethrough(true)
	}

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

	// Build line
	line := checkboxStyle.Render(checkbox) + textStyle.Render(t.Text)
	if len(labelParts) > 0 {
		line += "  " + strings.Join(labelParts, " ")
	}
	if duePart != "" {
		line += "  " + duePart
	}

	// Cursor / selection
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
		line = lipgloss.NewStyle().Bold(true).Render(line)
	}

	fmt.Fprint(w, cursor+line)
}

// isOverdue checks if a YYYY-MM-DD date is before today.
func isOverdue(dateStr string) bool {
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	today := time.Now().Truncate(24 * time.Hour)
	return d.Before(today)
}
```

- [ ] **Step 2: Build to verify compilation**

Run: `just build`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add internal/tui/tasks.go
git commit -m "feat(tui): add task list model with toggle support"
```

---

### Task 3: Create the CLI command

**Files:**
- Create: `cmd/know/cmd_task.go`
- Modify: `cmd/know/main.go:87` (add `rootCmd.AddCommand(taskCmd)`)

- [ ] **Step 1: Create `cmd/know/cmd_task.go`**

```go
package main

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/tui"
	"github.com/spf13/cobra"
)

var (
	taskAPI       *apiFlags
	taskVaultID   *string
	taskLabels    string
	taskStatus    string
	taskDueBefore string
	taskDueAfter  string
	taskPath      string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Browse and toggle tasks interactively",
	Long: `Open an interactive TUI for browsing and toggling tasks.

Tasks are extracted from markdown checkboxes (- [ ] / - [x]) in your documents.
Use flags to filter which tasks are shown.

Keybindings:
  enter/space  Toggle task (check/uncheck)
  /            Filter tasks
  esc          Quit
  j/k, arrows  Navigate

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)

Examples:
  know task
  know task --labels work,urgent
  know task --status all
  know task --path /daily/
  know task --due-before 2026-03-20`,
	RunE: runTask,
}

func init() {
	taskAPI = addAPIFlags(taskCmd)
	taskVaultID = addVaultFlag(taskCmd, taskAPI)
	taskCmd.Flags().StringVar(&taskLabels, "labels", "", "filter by labels (comma-separated)")
	taskCmd.Flags().StringVar(&taskStatus, "status", "open", "filter by status: open, done, all")
	taskCmd.Flags().StringVar(&taskDueBefore, "due-before", "", "only tasks due on or before this date (YYYY-MM-DD)")
	taskCmd.Flags().StringVar(&taskDueAfter, "due-after", "", "only tasks due on or after this date (YYYY-MM-DD)")
	taskCmd.Flags().StringVar(&taskPath, "path", "", "filter by document path or folder (path ending with / matches folder prefix)")

	if err := taskCmd.RegisterFlagCompletionFunc("labels", completeLabelNames(taskAPI, taskVaultID)); err != nil {
		panic(fmt.Sprintf("register labels completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("status", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"open", "done", "all"}, noFileComp
	}); err != nil {
		panic(fmt.Sprintf("register status completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("due-before", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register due-before completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("due-after", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register due-after completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("path", completeVaultPaths(taskAPI, taskVaultID, pathFilterAll)); err != nil {
		panic(fmt.Sprintf("register path completion: %v", err))
	}
}

func runTask(_ *cobra.Command, _ []string) error {
	client := taskAPI.newClient()
	ctx := context.Background()

	filter := apiclient.TaskFilter{
		Status:    taskStatus,
		DueBefore: taskDueBefore,
		DueAfter:  taskDueAfter,
		Path:      taskPath,
	}
	if taskLabels != "" {
		filter.Labels = strings.Split(taskLabels, ",")
	}

	resp, err := client.ListTasks(ctx, *taskVaultID, filter)
	if err != nil {
		return fmt.Errorf("task: %w", err)
	}

	model := tui.NewTaskModel(client, *taskVaultID, filter, resp.Tasks)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("task: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Check if `completeLabelNames` helper exists**

Search for `completeLabelNames` in `cmd/know/completions.go`. If it doesn't exist, add it:

```go
// completeLabelNames returns a completion function that lists label names from the REST API.
func completeLabelNames(af *apiFlags, vaultFlag *string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		if vaultFlag == nil || *vaultFlag == "" {
			return nil, noFileComp
		}
		client := af.newClient()
		labels, err := client.ListLabels(context.Background(), *vaultFlag)
		if err != nil {
			cobra.CompDebugln(fmt.Sprintf("failed to list labels: %v", err), true)
			return nil, noFileComp
		}
		return labels, noFileComp
	}
}
```

- [ ] **Step 3: Register command in `cmd/know/main.go`**

Add `rootCmd.AddCommand(taskCmd)` in the `main()` function, after the existing `AddCommand` calls.

- [ ] **Step 4: Build to verify compilation**

Run: `just build`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add cmd/know/cmd_task.go cmd/know/completions.go cmd/know/main.go
git commit -m "feat: add know task command with interactive TUI"
```

---

### Task 4: Manual testing and polish

- [ ] **Step 1: Start the dev server**

Run: `just dev`

- [ ] **Step 2: Test basic usage**

Run: `just run task` (or `./bin/know task --vault default`)
Expected: TUI opens showing open tasks. Navigate with j/k, toggle with enter/space, quit with Esc.

- [ ] **Step 3: Test with filters**

```bash
just run task -- --labels work
just run task -- --status all
just run task -- --status done
just run task -- --path /daily/
```

- [ ] **Step 4: Test empty state**

```bash
just run task -- --labels nonexistent
```
Expected: TUI shows "No items" empty state.

- [ ] **Step 5: Test filter mode**

Press `/` in the TUI, type a search term, press Enter. Verify fuzzy filtering works.

- [ ] **Step 6: Run tests**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 7: Update docs**

Add usage examples to `docs/feature-tasks.md` documenting the new `know task` command.

- [ ] **Step 8: Commit docs**

```bash
git add docs/feature-tasks.md
git commit -m "docs: add know task command examples to feature-tasks.md"
```
