package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
)

// CreateDocumentTool implements tool.InvokableTool for creating new documents.
type CreateDocumentTool struct {
	db         *db.Client
	docService *file.Service
}

func (t *CreateDocumentTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolCreateDocument,
		Desc: "Create a new document in the knowledge base. The content should be markdown. Fails if a document already exists at the given path.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Document path (e.g. /guides/new-guide.md)",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Full markdown content for the document",
				Required: true,
			},
		}),
	}, nil
}

func (t *CreateDocumentTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	args, err := parseInput[struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}](argumentsInJSON, ToolCreateDocument)
	if err != nil {
		return "", err
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if args.Content == "" {
		return "", fmt.Errorf("content is required")
	}

	existing, err := t.db.GetFileByPath(ctx, o.VaultID, args.Path)
	if err != nil {
		return "", fmt.Errorf("check existing document: %w", err)
	}
	if existing != nil {
		return "", &ToolError{Message: fmt.Sprintf("document already exists at path: %s. Use edit_document to update it or choose a different path", args.Path)}
	}

	start := time.Now()
	doc, err := t.docService.Create(ctx, models.FileInput{
		VaultID: o.VaultID,
		Path:    args.Path,
		Content: args.Content,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("create document: %w", err)
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	})
	return fmt.Sprintf("Document created: %s (%s)", doc.Title, doc.Path), nil
}
