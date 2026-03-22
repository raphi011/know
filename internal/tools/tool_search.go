package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/search"
)

// SearchTool implements tool.InvokableTool for knowledge base search.
type SearchTool struct {
	search *search.Service
}

func (t *SearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolSearch,
		Desc: "Search documents using full-text and semantic search. Returns titles, paths, scores, and matching snippets.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "Search query text", Required: true},
		}),
	}, nil
}

func (t *SearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	o := getToolOptions(opts...)

	input, err := parseInput[struct {
		Query string `json:"query"`
	}](argumentsInJSON, ToolSearch)
	if err != nil {
		return "", err
	}

	start := time.Now()
	results, err := t.search.Search(ctx, search.SearchInput{
		VaultID:     o.VaultID,
		Query:       input.Query,
		Limit:       20,
		FullContent: true,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	var sb strings.Builder
	var matchedDocs []ToolDocRef
	totalChunks := 0
	for _, r := range results {
		fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
		matchedDocs = append(matchedDocs, ToolDocRef{Title: r.Title, Path: r.Path, Score: r.Score})
		totalChunks += len(r.MatchedChunks)
		for _, ch := range r.MatchedChunks {
			sb.WriteString(ch.Snippet)
			sb.WriteString("\n\n")
		}
	}

	result := sb.String()
	if result == "" {
		result = "No results found."
	}

	SetResultMeta(ctx, &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(results)),
		ChunkCount:  new(totalChunks),
		MatchedDocs: matchedDocs,
	})
	return result, nil
}
