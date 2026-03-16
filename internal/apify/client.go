// Package apify provides a client for the Apify REST API.
package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL = "https://api.apify.com/v2"

	// defaultTimeout is the maximum time to wait for a synchronous actor run.
	defaultTimeout = 120
)

// Client is a thin wrapper around the Apify REST API.
type Client struct {
	token      string
	httpClient *http.Client
}

// New creates an Apify client with the given API token.
func New(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: time.Duration(defaultTimeout+30) * time.Second,
		},
	}
}

// RunActorSync starts an actor run synchronously and returns the dataset items.
// It uses the Apify "run-sync-get-dataset-items" endpoint which blocks until the
// actor finishes (up to defaultTimeout seconds) and returns the dataset directly.
func (c *Client) RunActorSync(ctx context.Context, actorID string, input any) ([]json.RawMessage, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}

	url := fmt.Sprintf("%s/acts/%s/run-sync-get-dataset-items?timeout=%d&format=json",
		baseURL, actorID, defaultTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run actor: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("actor run timed out after %ds", defaultTimeout)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("actor not found: %s", actorID)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("apify API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var items []json.RawMessage
	if err := json.Unmarshal(respBody, &items); err != nil {
		return nil, fmt.Errorf("unmarshal dataset items: %w", err)
	}

	return items, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
