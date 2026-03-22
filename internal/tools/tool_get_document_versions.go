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

// GetDocumentVersionsTool implements tool.InvokableTool for listing document versions.
type GetDocumentVersionsTool struct {
	db *db.Client
}

func (t *GetDocumentVersionsTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolGetDocumentVersions,
		Desc: "Get version history for a document by path. Returns previous versions with timestamps, sources, and titles.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Document path",
				Required: true,
			},
			"limit": {
				Type: schema.Integer,
				Desc: "Max versions to return (default 20)",
			},
		}),
	}, nil
}

func (t *GetDocumentVersionsTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	input, err := parseInput[struct {
		Path  string `json:"path"`
		Limit *int   `json:"limit"`
	}](argumentsInJSON, ToolGetDocumentVersions)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return "", &ToolError{Message: "limit must be a positive integer (e.g. 5, 10, 20)"}
	}
	if input.Limit != nil && *input.Limit > 100 {
		return "", &ToolError{Message: "limit must be at most 100"}
	}

	limit := 20
	if input.Limit != nil {
		limit = *input.Limit
	}

	doc, err := t.db.GetFileByPath(ctx, o.VaultID, input.Path)
	if err != nil {
		return "", fmt.Errorf("get document for versions: %w", err)
	}
	if doc == nil {
		return "", &ToolError{Message: fmt.Sprintf("document not found: %s. Use search_documents to find it or list_folder_contents to browse", input.Path)}
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return "", fmt.Errorf("extract document ID: %w", err)
	}

	start := time.Now()
	versions, err := t.db.ListVersions(ctx, docID, limit, 0)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("list versions: %w", err)
	}

	// Only query total count if we hit the limit (there may be more)
	totalCount := len(versions)
	if len(versions) == limit {
		totalCount, err = t.db.CountVersions(ctx, docID)
		if err != nil {
			return "", fmt.Errorf("count versions: %w", err)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Document: %s (%s)\n", input.Path, doc.Title)
	fmt.Fprintf(&sb, "Total versions: %d\n\n", totalCount)

	if len(versions) == 0 {
		sb.WriteString("No previous versions.\n")
	} else {
		for _, v := range versions {
			fmt.Fprintf(&sb, "### Version %d\n", v.Version)
			fmt.Fprintf(&sb, "- Title: %s\n", v.Title)
			fmt.Fprintf(&sb, "- Created: %s\n", v.CreatedAt.Format(time.RFC3339))
			fmt.Fprintf(&sb, "- Hash: %s\n\n", v.Hash)
		}
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(totalCount),
	})
	return sb.String(), nil
}
