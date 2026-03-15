package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
)

// ListTasksTool implements tool.InvokableTool for listing tasks across documents.
type ListTasksTool struct {
	db *db.Client
}

func (t *ListTasksTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_tasks",
		Desc: "List tasks (markdown checkboxes) extracted from documents. Returns tasks grouped by document with status, labels, and due dates. Use list_labels to discover available labels for filtering.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"status": {
				Type: schema.String,
				Desc: "Filter by status: 'open' or 'done'",
			},
			"labels": {
				Type:     schema.Array,
				Desc:     "Filter by task labels (returns tasks matching any label)",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"due_before": {
				Type: schema.String,
				Desc: "Filter tasks due on or before this date (YYYY-MM-DD)",
			},
			"due_after": {
				Type: schema.String,
				Desc: "Filter tasks due on or after this date (YYYY-MM-DD)",
			},
			"folder": {
				Type: schema.String,
				Desc: "Filter by document folder path prefix (e.g. /daily/)",
			},
			"path": {
				Type: schema.String,
				Desc: "Filter by exact document path",
			},
			"limit": {
				Type: schema.Integer,
				Desc: "Max results (default 100)",
			},
			"offset": {
				Type: schema.Integer,
				Desc: "Pagination offset",
			},
		}),
	}, nil
}

func (t *ListTasksTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	input, err := parseInput[struct {
		Status    *string  `json:"status"`
		Labels    []string `json:"labels"`
		DueBefore *string  `json:"due_before"`
		DueAfter  *string  `json:"due_after"`
		Folder    *string  `json:"folder"`
		Path      *string  `json:"path"`
		Limit     *int     `json:"limit"`
		Offset    *int     `json:"offset"`
	}](argumentsInJSON, "list_tasks")
	if err != nil {
		return "", err
	}

	if input.Status != nil && *input.Status != models.TaskStatusOpen && *input.Status != models.TaskStatusDone {
		return "", fmt.Errorf("invalid status %q: must be 'open' or 'done'", *input.Status)
	}

	filter := db.TaskFilter{
		VaultID:   o.VaultID,
		Status:    input.Status,
		Labels:    input.Labels,
		DueBefore: input.DueBefore,
		DueAfter:  input.DueAfter,
		Folder:    input.Folder,
		DocPath:   input.Path,
	}
	if input.Limit != nil {
		filter.Limit = *input.Limit
	}
	if input.Offset != nil {
		filter.Offset = *input.Offset
	}

	start := time.Now()
	tasks, err := t.db.ListTasks(ctx, filter)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("list tasks: %w", err)
	}

	if len(tasks) == 0 {
		SetResultMeta(ctx, &ToolResultMeta{
			DurationMs:  durationMs,
			ResultCount: new(0),
		})
		return "No tasks found.", nil
	}

	// Group tasks by document path.
	type docGroup struct {
		path  string
		title string
		tasks []string
	}
	var groups []docGroup
	groupIdx := map[string]int{}

	for _, task := range tasks {
		key := task.DocPath
		idx, exists := groupIdx[key]
		if !exists {
			idx = len(groups)
			groupIdx[key] = idx
			groups = append(groups, docGroup{path: task.DocPath, title: task.DocTitle})
		}

		checkbox := "[ ]"
		if task.Status == models.TaskStatusDone {
			checkbox = "[x]"
		}

		var parts []string
		parts = append(parts, fmt.Sprintf("- %s %s", checkbox, task.Text))
		if len(task.Labels) > 0 {
			for _, l := range task.Labels {
				parts = append(parts, "#"+l)
			}
		}
		if task.DueDate != nil {
			parts = append(parts, "due:"+*task.DueDate)
		}

		id, err := models.RecordIDString(task.ID)
		if err != nil {
			return "", fmt.Errorf("corrupt task record ID: %w", err)
		}
		line := strings.Join(parts, " ") + fmt.Sprintf(" (id:%s)", id)
		groups[idx].tasks = append(groups[idx].tasks, line)
	}

	var sb strings.Builder
	for i, g := range groups {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "%s:\n", g.path)
		for _, line := range g.tasks {
			fmt.Fprintf(&sb, "  %s\n", line)
		}
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(tasks)),
	})
	return sb.String(), nil
}
