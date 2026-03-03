package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// gqlClient is a minimal GraphQL HTTP client.
type gqlClient struct {
	url   string
	token string
	http  *http.Client
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

func newGQLClient(inst Instance) *gqlClient {
	url := strings.TrimSuffix(inst.URL, "/")
	if !strings.HasSuffix(url, "/query") {
		url += "/query"
	}
	return &gqlClient{url: url, token: inst.Token, http: &http.Client{}}
}

func (c *gqlClient) do(query string, vars map[string]any, target any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp gqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("graphql: %s", gqlResp.Errors[0].Message)
	}

	if target != nil {
		if err := json.Unmarshal(gqlResp.Data, target); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}
