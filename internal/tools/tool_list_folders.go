package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/pathutil"
)

// ListFoldersTool implements tool.InvokableTool for listing vault folders.
type ListFoldersTool struct {
	db *db.Client
}

func (t *ListFoldersTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_folders",
		Desc: "List the folder structure of the knowledge base. Optionally filter to immediate children of a parent folder.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"parent": {
				Type: schema.String,
				Desc: "Parent folder path to list children of (e.g. /guides/). Lists all folders if omitted.",
			},
		}),
	}, nil
}

func (t *ListFoldersTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	var input struct {
		Parent *string `json:"parent"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse list_folders input: %w", err)
	}

	start := time.Now()
	folders, err := t.db.ListFolders(ctx, o.VaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("list folders: %w", err)
	}

	if input.Parent != nil {
		parent := pathutil.NormalizeFolderPath(*input.Parent)
		var filtered []models.Folder
		for _, f := range folders {
			if pathutil.IsImmediateChildFolder(parent, f.Path) {
				filtered = append(filtered, f)
			}
		}
		folders = filtered
	}

	var sb strings.Builder
	for _, f := range folders {
		fmt.Fprintf(&sb, "%s (%s)\n", f.Path, f.Name)
	}
	result := sb.String()
	if result == "" {
		result = "No folders found."
	}
	setResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(folders)),
	})
	return result, nil
}
