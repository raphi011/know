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
	"github.com/raphi011/know/internal/jina"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/webclip"
)

// FetchWebpageTool fetches a web page as markdown via Jina Reader.
type FetchWebpageTool struct {
	jina    *jina.Client
	db      *db.Client
	fileSvc *file.Service
	model   *llm.Model
}

func (t *FetchWebpageTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolFetchWebpage,
		Desc: "Fetch a web page and convert it to markdown. Set save=true to persist the page to the vault's web clip folder with proper frontmatter. Without save, returns the markdown content only.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "URL of the web page to fetch",
				Required: true,
			},
			"save": {
				Type: schema.Boolean,
				Desc: "Save the fetched page to the vault (default: false)",
			},
			"path": {
				Type: schema.String,
				Desc: "Custom vault path for saving (optional, only used when save=true). Defaults to the vault's web clip folder.",
			},
			"clean": {
				Type: schema.Boolean,
				Desc: "Clean up markdown formatting with LLM (default: false). Fixes broken headings, removes boilerplate, and improves readability without changing content.",
			},
		}),
	}, nil
}

func (t *FetchWebpageTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	args, err := parseInput[struct {
		URL   string  `json:"url"`
		Save  bool    `json:"save"`
		Path  *string `json:"path"`
		Clean bool    `json:"clean"`
	}](argumentsInJSON, ToolFetchWebpage)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(args.URL) == "" {
		return "", &ToolError{Message: "url is required"}
	}

	var model *llm.Model
	if args.Clean {
		if t.model == nil {
			return "", &ToolError{Message: "clean option requires an LLM provider to be configured"}
		}
		model = t.model
	}

	if !args.Save {
		start := time.Now()
		result, err := webclip.Fetch(ctx, t.jina, args.URL, model)
		if err != nil {
			return "", fmt.Errorf("fetch webpage: %w", err)
		}

		durationMs := time.Since(start).Milliseconds()
		SetResultMeta(ctx, &ToolResultMeta{DurationMs: durationMs})

		return webclip.FormatAsMarkdown(result), nil
	}

	// Save mode — persist to vault.
	v, err := t.db.GetVault(ctx, o.VaultID)
	if err != nil {
		return "", fmt.Errorf("fetch webpage: load vault: %w", err)
	}
	if v == nil {
		return "", &ToolError{Message: "vault not found"}
	}
	settings := v.Defaults()

	start := time.Now()
	result, err := webclip.FetchAndSave(ctx, t.jina, t.fileSvc, o.VaultID, args.URL, args.Path, settings, model)
	if err != nil {
		return "", fmt.Errorf("fetch webpage: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:   durationMs,
		DocumentPath: &result.Path,
	})

	return fmt.Sprintf("Fetched and saved: %s → %s", result.Title, result.Path), nil
}
