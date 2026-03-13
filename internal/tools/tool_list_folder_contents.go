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
	"github.com/raphi011/knowhow/internal/pathutil"
)

// ListFolderContentsTool implements tool.InvokableTool for listing folder contents.
type ListFolderContentsTool struct {
	db *db.Client
}

func (t *ListFolderContentsTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_folder_contents",
		Desc: "List documents and subfolders in a specific folder. Returns immediate children only.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"folder": {
				Type:     schema.String,
				Desc:     "Folder path (e.g. /guides/)",
				Required: true,
			},
		}),
	}, nil
}

func (t *ListFolderContentsTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	var input struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse list_folder_contents input: %w", err)
	}
	if input.Folder == "" {
		return "", fmt.Errorf("folder is required")
	}
	folder := pathutil.NormalizeFolderPath(input.Folder)

	start := time.Now()

	docs, err := t.db.ListDocuments(ctx, db.ListDocumentsFilter{
		VaultID: o.VaultID,
		Folder:  &folder,
	})
	if err != nil {
		return "", fmt.Errorf("list folder contents: %w", err)
	}
	allFolders, err := t.db.ListFolders(ctx, o.VaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("list folder subfolders: %w", err)
	}

	var sb strings.Builder
	count := 0

	for _, f := range allFolders {
		if pathutil.IsImmediateChildFolder(folder, f.Path) {
			fmt.Fprintf(&sb, "📁 %s/\n", f.Name)
			count++
		}
	}

	for _, d := range docs {
		if !pathutil.IsImmediateChild(folder, d.Path) {
			continue
		}
		labels := ""
		if len(d.Labels) > 0 {
			labels = " [" + strings.Join(d.Labels, ", ") + "]"
		}
		fmt.Fprintf(&sb, "📄 %s — %s%s\n", d.Path, d.Title, labels)
		count++
	}

	result := sb.String()
	if result == "" {
		result = fmt.Sprintf("No contents found in folder %s", folder)
	}
	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(count),
	})
	return result, nil
}
