package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
)

// ListLabelsTool implements tool.InvokableTool for listing vault labels.
type ListLabelsTool struct {
	db *db.Client
}

func (t *ListLabelsTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "list_labels",
		Desc:        "List all labels/categories used across documents in the knowledge base",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *ListLabelsTool) InvokableRun(ctx context.Context, _ string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	start := time.Now()
	labels, err := t.db.ListLabels(ctx, o.VaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("list labels: %w", err)
	}

	result := "No labels found."
	if len(labels) > 0 {
		result = strings.Join(labels, ", ")
	}
	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(labels)),
	})
	return result, nil
}
