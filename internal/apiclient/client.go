// Package apiclient provides a lightweight REST API client for the Knowhow server.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client is a REST API client for the Knowhow server.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New creates a new REST API client. The baseURL should be the server root
// (e.g. "http://localhost:4001"), not a specific endpoint.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs a GET request and decodes the JSON response into target.
func (c *Client) Get(ctx context.Context, path string, target any) error {
	return c.do(ctx, http.MethodGet, path, nil, target)
}

// Post performs a POST request with a JSON body and decodes the response into target.
func (c *Client) Post(ctx context.Context, path string, body, target any) error {
	return c.do(ctx, http.MethodPost, path, body, target)
}

// Patch performs a PATCH request with a JSON body and decodes the response into target.
func (c *Client) Patch(ctx context.Context, path string, body, target any) error {
	return c.do(ctx, http.MethodPatch, path, body, target)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// PostMultipart performs a multipart/form-data POST request.
// fields are sent as form fields, fileData is sent as a file upload with the given fileField name and fileName.
func (c *Client) PostMultipart(ctx context.Context, path string, fields map[string]string, fileField, fileName string, fileData io.Reader, target any) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return fmt.Errorf("write field %s: %w", k, err)
		}
	}

	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, fileData); err != nil {
		return fmt.Errorf("copy file data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.handleResponse(req, target)
}

func (c *Client) do(ctx context.Context, method, path string, body, target any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.handleResponse(req, target)
}

// handleResponse executes the request and processes the response.
func (c *Client) handleResponse(req *http.Request, target any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if target != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, target); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}
