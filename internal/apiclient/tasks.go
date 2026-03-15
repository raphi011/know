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
