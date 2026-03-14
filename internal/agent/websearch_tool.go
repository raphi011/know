package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/tools"
)

// WebSearchTool is an agent-only InvokableTool that wraps the Tavily web search API.
type WebSearchTool struct {
	tavily *tavilyClient
}

func (t *WebSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the web for information not found in the knowledge base. Only call this after the user explicitly asks to search the web.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The web search query",
				Required: true,
			},
		}),
	}, nil
}

func (t *WebSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse web_search input: %w", err)
	}

	start := time.Now()
	result, webResults, err := t.tavily.Search(ctx, input.Query)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("web search: %w", err)
	}

	var webSources []tools.ToolWebRef
	for _, r := range webResults {
		webSources = append(webSources, tools.ToolWebRef{Title: r.Title, URL: r.URL})
	}

	meta := &tools.ToolResultMeta{
		DurationMs:     durationMs,
		WebResultCount: new(len(webResults)),
		WebSources:     webSources,
	}
	tools.SetResultMeta(ctx, meta)

	return result, nil
}
