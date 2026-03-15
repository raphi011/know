package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
)

// ToggleTaskTool implements tool.InvokableTool for toggling a task's status.
type ToggleTaskTool struct {
	docService *file.Service
}

func (t *ToggleTaskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "toggle_task",
		Desc: "Toggle a task's status between open and done. This modifies the source markdown document (changes `- [ ]` to `- [x]` or vice versa). Use list_tasks to find task IDs.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id": {
				Type:     schema.String,
				Desc:     "The task ID to toggle (from list_tasks output)",
				Required: true,
			},
		}),
	}, nil
}

func (t *ToggleTaskTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	input, err := parseInput[struct {
		TaskID string `json:"task_id"`
	}](argumentsInJSON, "toggle_task")
	if err != nil {
		return "", err
	}
	if input.TaskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	start := time.Now()
	updated, err := t.docService.ToggleTask(ctx, input.TaskID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("toggle task: %w", err)
	}

	SetResultMeta(ctx, &ToolResultMeta{DurationMs: durationMs})

	// updated is never nil here — ToggleTask returns an error if the task
	// disappears after toggle.

	status := "open"
	if updated.Status == models.TaskStatusDone {
		status = "completed"
	}
	return fmt.Sprintf("Task marked as %s: %s", status, updated.Text), nil
}
