package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
)

// EditDocumentSectionTool implements tool.InvokableTool for section-level document editing.
type EditDocumentSectionTool struct {
	db         *db.Client
	docService *document.Service
}

func (t *EditDocumentSectionTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "edit_document_section",
		Desc: "Edit a specific section of a document by heading, without sending the full content. Use read_document with sections=true to see available sections. Supports replace, insert_after, insert_before, delete, and append operations.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Document path",
				Required: true,
			},
			"operation": {
				Type:     schema.String,
				Desc:     "One of: replace, insert_after, insert_before, delete, append",
				Required: true,
			},
			"heading": {
				Type: schema.String,
				Desc: "Target section heading (empty string for preamble, omit for append)",
			},
			"position": {
				Type: schema.Integer,
				Desc: "Disambiguation index for duplicate headings (default 0)",
			},
			"content": {
				Type: schema.String,
				Desc: "New section body (required for replace, insert, append)",
			},
			"new_heading": {
				Type: schema.String,
				Desc: "Heading text for insert/append operations",
			},
			"new_level": {
				Type: schema.Integer,
				Desc: "Heading level 1-6 for insert/append operations",
			},
			"expected_hash": {
				Type: schema.String,
				Desc: "Content hash from get_document for optimistic concurrency check",
			},
		}),
	}, nil
}

func (t *EditDocumentSectionTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	var args struct {
		Path         string  `json:"path"`
		Operation    string  `json:"operation"`
		Heading      *string `json:"heading"`
		Position     *int    `json:"position"`
		Content      *string `json:"content"`
		NewHeading   *string `json:"new_heading"`
		NewLevel     *int    `json:"new_level"`
		ExpectedHash *string `json:"expected_hash"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("parse edit_document_section input: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if args.Operation == "" {
		return "", fmt.Errorf("operation is required")
	}
	// Validate operation early before DB lookup
	switch parser.SectionOperation(args.Operation) {
	case parser.OpReplace, parser.OpInsertAfter, parser.OpInsertBefore, parser.OpDelete, parser.OpAppend:
		// valid
	default:
		return "", &ToolError{Message: fmt.Sprintf("unknown operation: %s", args.Operation)}
	}

	existing, err := t.db.GetDocumentByPath(ctx, o.VaultID, args.Path)
	if err != nil {
		return "", fmt.Errorf("check document: %w", err)
	}
	if existing == nil {
		return "", &ToolError{Message: fmt.Sprintf("document not found: %s", args.Path)}
	}
	if err := checkContentHash(args.ExpectedHash, existing.ContentHash); err != nil {
		return "", err
	}

	// Build the section edit
	edit := parser.BuildSectionEdit(parser.SectionEditArgs{
		Operation:  args.Operation,
		Heading:    args.Heading,
		Position:   args.Position,
		Content:    args.Content,
		NewHeading: args.NewHeading,
		NewLevel:   args.NewLevel,
	})

	// Apply the section edit to the existing content
	newContent, err := parser.ApplySectionEdit(existing.Content, edit)
	if err != nil {
		return "", &ToolError{Message: fmt.Sprintf("apply section edit: %s", err)}
	}

	start := time.Now()
	doc, err := t.docService.Create(ctx, models.DocumentInput{
		VaultID: o.VaultID,
		Path:    args.Path,
		Content: newContent,
		Source:  models.SourceAIGenerated,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("edit document section: %w", err)
	}

	opDesc := string(args.Operation)
	headingDesc := ""
	if args.Heading != nil {
		headingDesc = *args.Heading
		if headingDesc == "" {
			headingDesc = "(preamble)"
		}
	}
	if args.NewHeading != nil && (args.Operation == "insert_after" || args.Operation == "insert_before" || args.Operation == "append") {
		headingDesc = *args.NewHeading
	}

	setResultMeta(ctx, &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	})
	return fmt.Sprintf("Section %s: %q in %s (%s)", opDesc, headingDesc, doc.Title, doc.Path), nil
}
