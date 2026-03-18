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
	"github.com/raphi011/know/internal/webclip"
)

// FetchWebpageTool fetches a web page as markdown via Jina Reader.
type FetchWebpageTool struct {
	jina    *jina.Client
	db      *db.Client
	fileSvc *file.Service
}

func (t *FetchWebpageTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "fetch_webpage",
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
		}),
	}, nil
}

func (t *FetchWebpageTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	args, err := parseInput[struct {
		URL  string  `json:"url"`
		Save bool    `json:"save"`
		Path *string `json:"path"`
	}](argumentsInJSON, "fetch_webpage")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(args.URL) == "" {
		return "", &ToolError{Message: "url is required"}
	}

	if !args.Save {
		start := time.Now()
		result, err := webclip.Fetch(ctx, t.jina, args.URL)
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
	result, err := webclip.FetchAndSave(ctx, t.jina, t.fileSvc, o.VaultID, args.URL, args.Path, settings)
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
