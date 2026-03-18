// Package jina provides a client for the Jina Reader API (r.jina.ai)
// which converts web pages to clean markdown.
package jina

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://r.jina.ai"

// Client is a thin wrapper around the Jina Reader API.
type Client struct {
	apiKey     string // optional — enables higher rate limits
	httpClient *http.Client
}

// New creates a Jina Reader client. apiKey is optional (empty = free tier).
func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ReadResult holds the converted markdown and metadata from a fetched page.
type ReadResult struct {
	Title       string
	Description string
	URL         string // canonical/final URL
	Markdown    string
}

// response is the JSON structure returned by Jina Reader.
type response struct {
	Code int          `json:"code"`
	Data responseData `json:"data"`
}

type responseData struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Content     string `json:"content"`
}

// Read fetches a URL via Jina Reader and returns the markdown content with metadata.
func (c *Client) Read(ctx context.Context, targetURL string) (*ReadResult, error) {
	reqURL := baseURL + "/" + targetURL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseSize = 10 << 20 // 10 MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jina API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if r.Code != 0 && r.Code != 200 {
		return nil, fmt.Errorf("jina returned error code %d for %s", r.Code, targetURL)
	}

	if r.Data.Content == "" {
		return nil, fmt.Errorf("empty content returned for %s", targetURL)
	}

	return &ReadResult{
		Title:       r.Data.Title,
		Description: r.Data.Description,
		URL:         r.Data.URL,
		Markdown:    r.Data.Content,
	}, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
