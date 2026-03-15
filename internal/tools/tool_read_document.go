package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/parser"
)

// ReadDocumentTool implements tool.InvokableTool for reading a document by path.
type ReadDocumentTool struct {
	db *db.Client
}

func (t *ReadDocumentTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_document",
		Desc: "Read the full content of a specific document by its path. Set sections=true to include a section outline for use with edit_document_section.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "The document path (e.g. /folder/document-name)",
				Required: true,
			},
			"sections": {
				Type: schema.Boolean,
				Desc: "Include section outline for targeted editing",
			},
		}),
	}, nil
}

func (t *ReadDocumentTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	input, err := parseInput[struct {
		Path     string `json:"path"`
		Sections bool   `json:"sections"`
	}](argumentsInJSON, "read_document")
	if err != nil {
		return "", err
	}

	start := time.Now()
	doc, err := t.db.GetDocumentByPath(ctx, o.VaultID, input.Path)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("read document: %w", err)
	}
	if doc == nil {
		SetResultMeta(ctx, &ToolResultMeta{DurationMs: durationMs})
		return "", &ToolError{Message: fmt.Sprintf("document not found: %s. Use search_documents to find it or list_folder_contents to browse", input.Path)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", doc.Title)

	if doc.ContentHash != nil {
		fmt.Fprintf(&sb, "Content-Hash: %s\n\n", *doc.ContentHash)
	}

	if input.Sections {
		parsed := parser.ParseMarkdown(doc.ContentBody)
		if len(parsed.Sections) > 0 {
			outline := parser.SectionOutline(parsed)
			sb.WriteString("## Sections\n")
			sb.WriteString("| # | Heading | Pos |\n")
			sb.WriteString("|---|---------|-----|\n")
			for _, info := range outline {
				heading := info.Heading
				if heading == "" {
					heading = "(preamble)"
				}
				fmt.Fprintf(&sb, "| %d | %s | %d |\n", info.Index, heading, info.Position)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(doc.ContentBody)

	contentLen := len(doc.ContentBody)
	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
		ContentLength: &contentLen,
	})
	return sb.String(), nil
}
