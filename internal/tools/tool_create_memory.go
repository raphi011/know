package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/models"
)

// CreateMemoryTool implements tool.InvokableTool for creating memories.
type CreateMemoryTool struct {
	db         *db.Client
	docService *file.Service
}

func (t *CreateMemoryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "create_memory",
		Desc: "Create a memory, optionally scoped to a project. For project memories, use a stable identifier (git remote URL or repo folder name). For global memories (e.g. Go patterns, Docker tips), omit project and add descriptive labels. Call list_labels first to reuse existing labels.",
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
				Desc: "Project identifier (git remote URL or repo folder name). Omit for global memories.",
			},
			"labels": {
				Type: schema.Array,
				Desc: "Labels for categorization (e.g. golang, docker). Call list_labels to discover existing labels.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
		}),
	}, nil
}

func (t *CreateMemoryTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	input, err := parseInput[struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Project string   `json:"project"`
		Labels  []string `json:"labels"`
	}](argumentsInJSON, "create_memory")
	if err != nil {
		return "", err
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
	doc, err := t.docService.Create(ctx, models.FileInput{
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
