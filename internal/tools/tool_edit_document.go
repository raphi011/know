package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/models"
)

// EditDocumentTool implements tool.InvokableTool for editing a document's full content.
type EditDocumentTool struct {
	db         *db.Client
	docService *document.Service
}

func (t *EditDocumentTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "edit_document",
		Desc: "Edit an existing document by replacing its full content. Read the document first to get the current content, then modify and pass the complete new content.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Document path of the existing document",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Complete new markdown content (replaces existing content entirely)",
				Required: true,
			},
			"expected_hash": {
				Type: schema.String,
				Desc: "Content hash from get_document for optimistic concurrency check",
			},
		}),
	}, nil
}

func (t *EditDocumentTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	args, err := parseInput[struct {
		Path         string  `json:"path"`
		Content      string  `json:"content"`
		ExpectedHash *string `json:"expected_hash"`
	}](argumentsInJSON, "edit_document")
	if err != nil {
		return "", err
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if args.Content == "" {
		return "", fmt.Errorf("content is required")
	}

	existing, err := t.db.GetDocumentByPath(ctx, o.VaultID, args.Path)
	if err != nil {
		return "", fmt.Errorf("check document: %w", err)
	}
	if existing == nil {
		return "", &ToolError{Message: fmt.Sprintf("document not found: %s. Use search_documents to find it or list_folder_contents to browse", args.Path)}
	}
	if err := checkContentHash(args.ExpectedHash, existing.ContentHash); err != nil {
		return "", err
	}

	start := time.Now()
	doc, err := t.docService.Create(ctx, models.DocumentInput{
		VaultID: o.VaultID,
		Path:    args.Path,
		Content: args.Content,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("edit document: %w", err)
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	})
	return fmt.Sprintf("Document updated: %s (%s)", doc.Title, doc.Path), nil
}
