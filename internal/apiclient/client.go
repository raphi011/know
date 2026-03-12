// Package apiclient provides a lightweight REST API client for the Knowhow server.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/raphi011/knowhow/internal/models"
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

// BulkFile represents a single file to upload via the bulk endpoint.
type BulkFile struct {
	Path string    // vault path (used as the multipart form name)
	Data io.Reader // file content
}

// BulkMeta holds shared metadata for a bulk upload request.
type BulkMeta struct {
	VaultID string `json:"vaultId"`
	Source  string `json:"source"`
	Force   bool   `json:"force"`
	DryRun  bool   `json:"dryRun"`
}

// BulkResult is a per-file result from the bulk upload endpoint.
type BulkResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// BulkUpload sends multiple files (documents and assets) to the bulk upload endpoint.
// The meta part is sent first as JSON, then each file as a separate multipart part
// where the form name is the vault path.
//
// A nil error means the HTTP request succeeded, but individual files may still have
// failed — callers must check each BulkResult.Status for "error" entries.
func (c *Client) BulkUpload(ctx context.Context, meta BulkMeta, files []BulkFile) ([]BulkResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Write meta part as JSON
	metaPart, err := writer.CreateFormField("meta")
	if err != nil {
		return nil, fmt.Errorf("create meta part: %w", err)
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return nil, fmt.Errorf("encode meta: %w", err)
	}

	// Write each file as a part with the vault path as form name
	for _, f := range files {
		part, err := writer.CreateFormFile(f.Path, f.Path)
		if err != nil {
			return nil, fmt.Errorf("create file part %s: %w", f.Path, err)
		}
		if _, err := io.Copy(part, f.Data); err != nil {
			return nil, fmt.Errorf("copy file data %s: %w", f.Path, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/bulk", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	var resp struct {
		Results []BulkResult `json:"results"`
	}
	if err := c.handleResponse(req, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
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

// Document is the JSON representation of a document returned by the REST API.
type Document struct {
	ID          string  `json:"id"`
	VaultID     string  `json:"vaultId"`
	Path        string  `json:"path"`
	Title       string  `json:"title"`
	Content     string  `json:"content"`
	Source      string  `json:"source"`
	ContentHash *string `json:"contentHash,omitempty"`
}

// GetDocument fetches a document by vault and path.
func (c *Client) GetDocument(ctx context.Context, vaultID, path string) (*Document, error) {
	q := url.Values{"vault": {vaultID}, "path": {path}}
	var doc Document
	if err := c.Get(ctx, "/api/documents?"+q.Encode(), &doc); err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	return &doc, nil
}

// DeleteResult is the response from the delete documents endpoint.
type DeleteResult struct {
	Deleted []string `json:"deleted"`
	Count   int      `json:"count"`
	DryRun  bool     `json:"dryRun"`
}

// DeleteDocuments deletes a document or folder (with recursive flag) from a vault.
func (c *Client) DeleteDocuments(ctx context.Context, vaultID, path string, recursive, dryRun bool) (*DeleteResult, error) {
	q := url.Values{"vault": {vaultID}, "path": {path}}
	if recursive {
		q.Set("recursive", "true")
	}
	if dryRun {
		q.Set("dry-run", "true")
	}
	var result DeleteResult
	if err := c.do(ctx, http.MethodDelete, "/api/documents?"+q.Encode(), nil, &result); err != nil {
		return nil, fmt.Errorf("delete documents: %w", err)
	}
	return &result, nil
}

// ListFiles lists files and folders at the given path in a vault.
func (c *Client) ListFiles(ctx context.Context, vaultID, path string, recursive bool) ([]models.FileEntry, error) {
	q := url.Values{"vault": {vaultID}, "path": {path}}
	if recursive {
		q.Set("recursive", "true")
	}
	var entries []models.FileEntry
	if err := c.Get(ctx, "/api/ls?"+q.Encode(), &entries); err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return entries, nil
}

// ListLabels returns all distinct labels in the given vault.
func (c *Client) ListLabels(ctx context.Context, vaultID string) ([]string, error) {
	var labels []string
	if err := c.Get(ctx, "/api/labels?vault="+url.QueryEscape(vaultID), &labels); err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	return labels, nil
}

// ListLabelsWithCounts returns labels with their document counts for the given vault.
func (c *Client) ListLabelsWithCounts(ctx context.Context, vaultID string) ([]models.LabelCount, error) {
	var counts []models.LabelCount
	if err := c.Get(ctx, "/api/labels?vault="+url.QueryEscape(vaultID)+"&counts=true", &counts); err != nil {
		return nil, fmt.Errorf("list labels with counts: %w", err)
	}
	return counts, nil
}

// VaultInfo holds comprehensive stats about a vault.
type VaultInfo struct {
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	DocumentCount      int `json:"documentCount"`
	UnprocessedDocs    int `json:"unprocessedDocs"`
	ChunkTotal         int `json:"chunkTotal"`
	ChunkWithEmbedding int `json:"chunkWithEmbedding"`
	ChunkPending       int `json:"chunkPending"`

	LabelCount int                `json:"labelCount"`
	TopLabels  []models.LabelStat `json:"topLabels"`

	Members []models.MemberStat `json:"members"`

	AssetCount     int   `json:"assetCount"`
	AssetTotalSize int64 `json:"assetTotalSize"`

	WikiLinkTotal  int `json:"wikiLinkTotal"`
	WikiLinkBroken int `json:"wikiLinkBroken"`

	TemplateCount     int   `json:"templateCount"`
	VersionCount      int   `json:"versionCount"`
	ConversationCount int   `json:"conversationCount"`
	TokenInput        int64 `json:"tokenInput"`
	TokenOutput       int64 `json:"tokenOutput"`
}

// GetVaultInfo fetches comprehensive stats about a vault.
func (c *Client) GetVaultInfo(ctx context.Context, vaultName string) (*VaultInfo, error) {
	var info VaultInfo
	if err := c.Get(ctx, "/api/vaults/"+url.PathEscape(vaultName)+"/info", &info); err != nil {
		return nil, fmt.Errorf("get vault info: %w", err)
	}
	return &info, nil
}

// DownloadBackup downloads a vault backup archive to the given output path.
// Returns the number of bytes written.
func (c *Client) DownloadBackup(ctx context.Context, vaultID, outputPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/backup?vault="+url.QueryEscape(vaultID), nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// Use a client with no timeout — large vaults may take a while.
	// Context controls cancellation instead.
	noTimeoutClient := &http.Client{}
	resp, err := noTimeoutClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return 0, fmt.Errorf("HTTP %d (failed to read error body: %w)", resp.StatusCode, readErr)
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
		}
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("create output file: %w", err)
	}

	n, copyErr := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		if removeErr := os.Remove(outputPath); removeErr != nil {
			slog.Warn("failed to clean up partial backup file", "path", outputPath, "error", removeErr)
		}
		return 0, fmt.Errorf("write output file: %w", copyErr)
	}

	return n, nil
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
