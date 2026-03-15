package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/surrealdb/surrealdb.go"

	"github.com/raphi011/know/internal/models"
)

// TaskFilter controls which tasks are returned by ListTasks.
type TaskFilter struct {
	VaultID   string
	Status    *models.TaskStatus // "open" or "done"
	Labels    []string           // CONTAINSANY
	DueBefore *string            // inclusive upper bound (YYYY-MM-DD)
	DueAfter  *string            // inclusive lower bound (YYYY-MM-DD)
	Folder    *string            // document path prefix
	DocPath   *string            // exact document path
	Limit     int
	Offset    int
}

// TaskUpdate contains the mutable fields for updating an existing task.
type TaskUpdate struct {
	Status      models.TaskStatus
	RawLine     string
	Text        string
	Labels      []string
	DueDate     *string
	LineNumber  int
	HeadingPath *string
}

// CreateTask inserts a new task record.
func (c *Client) CreateTask(ctx context.Context, input models.TaskInput) (*models.Task, error) {
	defer c.logOp(ctx, "task.create", time.Now())

	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	sql := `CREATE task SET
		document = type::record("document", $doc_id),
		vault = type::record("vault", $vault_id),
		status = $status,
		raw_line = $raw_line,
		text = $text,
		labels = $labels,
		due_date = $due_date,
		line_number = $line_number,
		heading_path = $heading_path,
		content_hash = $content_hash`

	results, err := surrealdb.Query[[]models.Task](ctx, c.DB(), sql, map[string]any{
		"doc_id":       bareID("document", input.DocumentID),
		"vault_id":     bareID("vault", input.VaultID),
		"status":       string(input.Status),
		"raw_line":     input.RawLine,
		"text":         input.Text,
		"labels":       labels,
		"due_date":     optionalString(input.DueDate),
		"line_number":  input.LineNumber,
		"heading_path": optionalString(input.HeadingPath),
		"content_hash": input.ContentHash,
	})
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return firstResult(results, "create task")
}

// UpdateTask updates a task's mutable fields (status, line position, metadata).
func (c *Client) UpdateTask(ctx context.Context, id string, update TaskUpdate) error {
	defer c.logOp(ctx, "task.update", time.Now())

	labels := update.Labels
	if labels == nil {
		labels = []string{}
	}

	sql := `UPDATE type::record("task", $id) SET
		status = $status,
		raw_line = $raw_line,
		text = $text,
		labels = $labels,
		due_date = $due_date,
		line_number = $line_number,
		heading_path = $heading_path`

	results, err := surrealdb.Query[[]models.Task](ctx, c.DB(), sql, map[string]any{
		"id":           bareID("task", id),
		"status":       string(update.Status),
		"raw_line":     update.RawLine,
		"text":         update.Text,
		"labels":       labels,
		"due_date":     optionalString(update.DueDate),
		"line_number":  update.LineNumber,
		"heading_path": optionalString(update.HeadingPath),
	})
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	if len(allResults(results)) == 0 {
		return fmt.Errorf("update task: not found (id: %s)", id)
	}
	return nil
}

// DeleteTask removes a single task by ID.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	defer c.logOp(ctx, "task.delete", time.Now())

	sql := `DELETE type::record("task", $id)`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": bareID("task", id)})
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// DeleteTasksByDocument removes all tasks belonging to a document.
func (c *Client) DeleteTasksByDocument(ctx context.Context, docID string) error {
	defer c.logOp(ctx, "task.delete_by_document", time.Now())

	sql := `DELETE FROM task WHERE document = type::record("document", $doc_id)`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"doc_id": bareID("document", docID)})
	if err != nil {
		return fmt.Errorf("delete tasks by document: %w", err)
	}
	return nil
}

// GetTaskByID fetches a single task. Returns nil if not found.
func (c *Client) GetTaskByID(ctx context.Context, id string) (*models.Task, error) {
	defer c.logOp(ctx, "task.get", time.Now())

	sql := `SELECT * FROM type::record("task", $id)`
	results, err := surrealdb.Query[[]models.Task](ctx, c.DB(), sql, map[string]any{"id": bareID("task", id)})
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return firstResultOpt(results), nil
}

// GetTasksByDocument returns all tasks for a document, ordered by line number.
func (c *Client) GetTasksByDocument(ctx context.Context, docID string) ([]models.Task, error) {
	defer c.logOp(ctx, "task.get_by_document", time.Now())

	sql := `SELECT * FROM task WHERE document = type::record("document", $doc_id) ORDER BY line_number ASC`
	results, err := surrealdb.Query[[]models.Task](ctx, c.DB(), sql, map[string]any{"doc_id": bareID("document", docID)})
	if err != nil {
		return nil, fmt.Errorf("get tasks by document: %w", err)
	}
	return allResults(results), nil
}

// ListTasks returns tasks matching the filter, with denormalized document info.
func (c *Client) ListTasks(ctx context.Context, filter TaskFilter) ([]models.TaskWithDoc, error) {
	defer c.logOp(ctx, "task.list", time.Now())

	conditions, vars := buildTaskFilter(filter)

	limit := 100
	if filter.Limit > 0 && filter.Limit <= 999 {
		limit = filter.Limit
	}

	where := strings.Join(conditions, " AND ")
	vars["limit"] = limit
	vars["start"] = filter.Offset
	sql := fmt.Sprintf(`SELECT *, document.path AS doc_path, document.title AS doc_title
		FROM task WHERE %s
		ORDER BY due_date IS NONE ASC, due_date ASC, line_number ASC
		LIMIT $limit START $start`, where)

	results, err := surrealdb.Query[[]models.TaskWithDoc](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return allResults(results), nil
}

// CountTasks returns the number of tasks matching the filter.
func (c *Client) CountTasks(ctx context.Context, filter TaskFilter) (int, error) {
	defer c.logOp(ctx, "task.count", time.Now())

	conditions, vars := buildTaskFilter(filter)
	where := strings.Join(conditions, " AND ")
	sql := fmt.Sprintf(`SELECT count() AS total FROM task WHERE %s GROUP ALL`, where)

	type countResult struct {
		Total int `json:"total"`
	}
	results, err := surrealdb.Query[[]countResult](ctx, c.DB(), sql, vars)
	if err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}
	r := firstResultOpt(results)
	if r == nil {
		return 0, nil
	}
	return r.Total, nil
}

func buildTaskFilter(filter TaskFilter) ([]string, map[string]any) {
	var conditions []string
	vars := map[string]any{
		"vault_id": bareID("vault", filter.VaultID),
	}

	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if filter.Status != nil {
		conditions = append(conditions, `status = $status`)
		vars["status"] = string(*filter.Status)
	}
	if len(filter.Labels) > 0 {
		conditions = append(conditions, `labels CONTAINSANY $labels`)
		vars["labels"] = filter.Labels
	}
	if filter.DueBefore != nil {
		conditions = append(conditions, `due_date != NONE AND due_date <= $due_before`)
		vars["due_before"] = *filter.DueBefore
	}
	if filter.DueAfter != nil {
		conditions = append(conditions, `due_date != NONE AND due_date >= $due_after`)
		vars["due_after"] = *filter.DueAfter
	}
	if filter.Folder != nil {
		conditions = append(conditions, `string::starts_with(document.path, $folder)`)
		vars["folder"] = *filter.Folder
	}
	if filter.DocPath != nil {
		conditions = append(conditions, `document.path = $doc_path`)
		vars["doc_path"] = *filter.DocPath
	}

	return conditions, vars
}
