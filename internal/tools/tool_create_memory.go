package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/models"
)

// CreateMemoryTool implements tool.InvokableTool for creating memories.
type CreateMemoryTool struct {
	db         *db.Client
	docService *document.Service
}

func (t *CreateMemoryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "create_memory",
		Desc: "Create a memory, optionally scoped to a project",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"title": {
				Type:     schema.String,
				Desc:     "Memory title (used for filename)",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "Memory content (markdown)",
				Required: true,
			},
			"project": {
				Type: schema.String,
				Desc: "Optional project identifier",
			},
			"labels": {
				Type: schema.Array,
				Desc: "Additional labels for categorization",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
		}),
	}, nil
}

func (t *CreateMemoryTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	var input struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Project string   `json:"project"`
		Labels  []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse create_memory input: %w", err)
	}
	if strings.TrimSpace(input.Title) == "" {
		return "", fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return "", fmt.Errorf("content is required")
	}

	// Load vault settings for memory path config
	vault, err := t.db.GetVault(ctx, o.VaultID)
	if err != nil {
		return "", fmt.Errorf("create memory: load vault: %w", err)
	}
	if vault == nil {
		return "", fmt.Errorf("create memory: vault not found: %s", o.VaultID)
	}
	settings := vault.MemoryDefaults()

	path, fullContent := memory.BuildMemoryDocument(input.Project, input.Title, input.Content, input.Labels, settings)

	start := time.Now()
	doc, err := t.docService.Create(ctx, models.DocumentInput{
		VaultID: o.VaultID,
		Path:    path,
		Content: fullContent,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("create memory: %w", err)
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	})
	return fmt.Sprintf("Memory created at %s", doc.Path), nil
}
