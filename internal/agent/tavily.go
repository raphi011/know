package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// tavilyClient wraps the Tavily search API.
type tavilyClient struct {
	apiKey     string
	httpClient *http.Client
}

func newTavilyClient(apiKey string) *tavilyClient {
	return &tavilyClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type tavilyRequest struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
	IncludeAnswer bool `json:"include_answer"`
}

type tavilyResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// Search performs a web search via Tavily and returns formatted markdown results alongside the parsed result entries.
func (c *tavilyClient) Search(ctx context.Context, query string) (string, []tavilyResult, error) {
	reqBody := tavilyRequest{
		APIKey:        c.apiKey,
		Query:         query,
		SearchDepth:   "basic",
		MaxResults:    5,
		IncludeAnswer: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return "", nil, fmt.Errorf("tavily API returned %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return "", nil, fmt.Errorf("tavily API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tavilyResp tavilyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tavilyResp); err != nil {
		return "", nil, fmt.Errorf("decode response: %w", err)
	}

	var sb strings.Builder
	if tavilyResp.Answer != "" {
		fmt.Fprintf(&sb, "**Summary:** %s\n\n", tavilyResp.Answer)
	}
	for _, r := range tavilyResp.Results {
		fmt.Fprintf(&sb, "### [%s](%s)\n%s\n\n", r.Title, r.URL, r.Content)
	}

	result := sb.String()
	if result == "" {
		return "No web results found.", tavilyResp.Results, nil
	}
	return result, tavilyResp.Results, nil
}
