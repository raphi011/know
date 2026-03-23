// Package apiclient provides a lightweight REST API client for the Know server.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"

	"github.com/raphi011/know/internal/models"
)

// HTTPError is returned when the server responds with a 4xx/5xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// Client is a REST API client for the Know server.
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

// vaultPath constructs a vault-scoped API path: /api/v1/vaults/{name}{resource}.
func vaultPath(vaultName, resource string) string {
	return "/api/v1/vaults/" + url.PathEscape(vaultName) + resource
}

// Get performs a GET request and decodes the JSON response into target.
func (c *Client) Get(ctx context.Context, path string, target any) error {
	return c.do(ctx, http.MethodGet, path, nil, target)
}

// Post performs a POST request with a JSON body and decodes the response into target.
func (c *Client) Post(ctx context.Context, path string, body, target any) error {
	return c.do(ctx, http.MethodPost, path, body, target)
}

// Put performs a PUT request with a JSON body and decodes the response into target.
func (c *Client) Put(ctx context.Context, path string, body, target any) error {
	return c.do(ctx, http.MethodPut, path, body, target)
}

// Patch performs a PATCH request with a JSON body and decodes the response into target.
func (c *Client) Patch(ctx context.Context, path string, body, target any) error {
	return c.do(ctx, http.MethodPatch, path, body, target)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// DeleteWithBody performs a DELETE request with a JSON body.
func (c *Client) DeleteWithBody(ctx context.Context, path string, body any) error {
	return c.do(ctx, http.MethodDelete, path, body, nil)
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
	Force  bool `json:"force"`
	DryRun bool `json:"dryRun"`
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
func (c *Client) BulkUpload(ctx context.Context, vaultName string, meta BulkMeta, files []BulkFile) ([]BulkResult, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+vaultPath(vaultName, "/documents/bulk"), &buf)
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

// --- Import (two-phase) ---

// Import result status constants (must match server-side values in api/import.go).
const (
	ImportStatusCreated = "created"
	ImportStatusUpdated = "updated"
	ImportStatusSkipped = "skipped"
	ImportStatusError   = "error"
)

// Import result reason constants.
const (
	ImportReasonHashDiffers = "hash_differs"
)

// ImportManifestFile is a single file entry for the import manifest.
type ImportManifestFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// ImportManifestRequest is the request body for POST /api/v1/vaults/{name}/import/manifest.
type ImportManifestRequest struct {
	Force  bool                 `json:"force"`
	DryRun bool                 `json:"dryRun"`
	Files  []ImportManifestFile `json:"files"`
}

// ImportManifestResponse is the response from POST /api/import/manifest.
type ImportManifestResponse struct {
	Needed  []string       `json:"needed"`
	Results []ImportResult `json:"results"`
}

// ImportResult is a per-file result from the import endpoints.
type ImportResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ImportUploadResponse is the response from POST /api/import/upload.
type ImportUploadResponse struct {
	Results []ImportResult `json:"results"`
	Error   string         `json:"error,omitempty"`
}

// ImportManifest sends a file manifest to the server and returns which files need uploading.
func (c *Client) ImportManifest(ctx context.Context, vaultName string, req ImportManifestRequest) (*ImportManifestResponse, error) {
	var resp ImportManifestResponse
	if err := c.Post(ctx, vaultPath(vaultName, "/import/manifest"), req, &resp); err != nil {
		return nil, fmt.Errorf("import manifest: %w", err)
	}
	return &resp, nil
}

// ImportFile represents a single file to upload in the import upload phase.
type ImportFile struct {
	Path string    // vault path (used as the multipart form name)
	Hash string    // SHA256 hash (sent in meta for server-side verification)
	Data io.Reader // file content (streamed from disk)
}

// ImportUpload sends only the needed files to the server's import upload endpoint.
// The meta part includes per-file hashes for server-side verification.
func (c *Client) ImportUpload(ctx context.Context, vaultName string, files []ImportFile) (*ImportUploadResponse, error) {
	// Build the multipart request with pipe to avoid buffering all files in memory.
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write multipart parts in a goroutine so the pipe streams to the HTTP request.
	errCh := make(chan error, 1)
	go func() {
		var writeErr error
		defer func() {
			// CloseWithError propagates the actual error to the read side.
			// On success writeErr is nil, which is equivalent to Close().
			pw.CloseWithError(writeErr)
		}()

		// Write meta part with vault ID and per-file hashes.
		metaPart, err := writer.CreateFormField("meta")
		if err != nil {
			writeErr = fmt.Errorf("create meta part: %w", err)
			errCh <- writeErr
			return
		}
		hashes := make(map[string]string, len(files))
		for _, f := range files {
			hashes[f.Path] = f.Hash
		}
		meta := struct {
			Hashes map[string]string `json:"hashes"`
		}{Hashes: hashes}
		if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
			writeErr = fmt.Errorf("encode meta: %w", err)
			errCh <- writeErr
			return
		}

		// Write each file as a multipart part.
		for _, f := range files {
			part, err := writer.CreateFormFile(f.Path, f.Path)
			if err != nil {
				writeErr = fmt.Errorf("create file part %s: %w", f.Path, err)
				errCh <- writeErr
				return
			}
			if _, err := io.Copy(part, f.Data); err != nil {
				writeErr = fmt.Errorf("copy file data %s: %w", f.Path, err)
				errCh <- writeErr
				return
			}
		}

		if err := writer.Close(); err != nil {
			writeErr = fmt.Errorf("close multipart writer: %w", err)
			errCh <- writeErr
			return
		}
		errCh <- nil
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+vaultPath(vaultName, "/import/upload"), pr)
	if err != nil {
		pr.Close()
		<-errCh // drain goroutine to prevent leak
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// Use a client without timeout — large imports can take much longer than 30s.
	// Context cancellation still handles user-initiated aborts.
	noTimeoutClient := &http.Client{}
	resp2, err := noTimeoutClient.Do(req)
	if err != nil {
		if writeErr := <-errCh; writeErr != nil {
			return nil, fmt.Errorf("%w (write side: %v)", err, writeErr)
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp2.Body.Close()

	respBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		<-errCh
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp2.StatusCode >= 400 {
		<-errCh
		var pd httputil.ProblemDetail
		if json.Unmarshal(respBody, &pd) == nil && pd.Detail != "" {
			return nil, &HTTPError{StatusCode: resp2.StatusCode, Message: pd.Detail}
		}
		return nil, &HTTPError{StatusCode: resp2.StatusCode, Message: string(respBody)}
	}

	var resp ImportUploadResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &resp); err != nil {
			<-errCh
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
	}

	// Check for write errors from the goroutine.
	if writeErr := <-errCh; writeErr != nil {
		return nil, writeErr
	}

	return &resp, nil
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

// AdminUser is the JSON representation of a user from the admin API.
type AdminUser struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Email         *string `json:"email,omitempty"`
	IsSystemAdmin bool    `json:"is_system_admin"`
	OIDCProvider  *string `json:"oidc_provider,omitempty"`
}

// AdminCreateUserResponse is the response from POST /api/v1/admin/users.
type AdminCreateUserResponse struct {
	User struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Email *string `json:"email,omitempty"`
	} `json:"user"`
	Vault struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"vault"`
}

// AdminListUsers returns all users (system admin only).
func (c *Client) AdminListUsers(ctx context.Context) ([]AdminUser, error) {
	var resp httputil.ListResponse[AdminUser]
	if err := c.Get(ctx, "/api/v1/admin/users", &resp); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return resp.Items, nil
}

// AdminCreateUser creates a new user with a private vault (system admin only).
func (c *Client) AdminCreateUser(ctx context.Context, name, email string) (*AdminCreateUserResponse, error) {
	body := map[string]string{"name": name, "email": email}
	var resp AdminCreateUserResponse
	if err := c.Post(ctx, "/api/v1/admin/users", body, &resp); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &resp, nil
}

// DeviceFlowStartResponse is the response from POST /auth/device/start.
type DeviceFlowStartResponse struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	DeviceCode      string `json:"device_code"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceFlowStart initiates the device authorization flow.
// This is an unauthenticated endpoint.
func (c *Client) DeviceFlowStart(ctx context.Context) (*DeviceFlowStartResponse, error) {
	var resp DeviceFlowStartResponse
	if err := c.Post(ctx, "/auth/device/start", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeviceFlowPoll polls for device flow completion.
// Returns the token string when approved, or an error if still pending/expired.
func (c *Client) DeviceFlowPoll(ctx context.Context, deviceCode string) (string, error) {
	body := map[string]string{"device_code": deviceCode}
	var resp map[string]string
	if err := c.Post(ctx, "/auth/device/poll", body, &resp); err != nil {
		return "", err
	}
	return resp["token"], nil
}

// MoveResult is the response from the move endpoint.
type MoveResult struct {
	Type   string   `json:"type"`
	Moved  []string `json:"moved"`
	Count  int      `json:"count"`
	DryRun bool     `json:"dryRun"`
}

// Move moves a document or folder to a new path within the same vault.
func (c *Client) Move(ctx context.Context, vaultName, source, destination string, dryRun bool) (*MoveResult, error) {
	body := map[string]any{
		"source":      source,
		"destination": destination,
		"dryRun":      dryRun,
	}
	var result MoveResult
	if err := c.Post(ctx, vaultPath(vaultName, "/documents/move"), body, &result); err != nil {
		return nil, fmt.Errorf("move: %w", err)
	}
	return &result, nil
}

// Vault is the JSON representation of a vault from the REST API.
type Vault struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Remote      *string `json:"remote,omitempty"`
}

// ListVaults returns all accessible vaults.
func (c *Client) ListVaults(ctx context.Context) ([]Vault, error) {
	var resp httputil.ListResponse[Vault]
	if err := c.Get(ctx, "/api/v1/vaults", &resp); err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	return resp.Items, nil
}

// SearchResult is the JSON representation of a search result from the REST API.
type SearchResult struct {
	Path          string       `json:"path"`
	Title         string       `json:"title"`
	Score         float64      `json:"score"`
	MatchedChunks []ChunkMatch `json:"matchedChunks"`
}

// ChunkMatch is a matched chunk within a search result.
type ChunkMatch struct {
	Snippet     string  `json:"snippet"`
	HeadingPath *string `json:"headingPath,omitempty"`
	Position    int     `json:"position"`
	Score       float64 `json:"score"`
}

// SearchDocuments searches documents on the remote server.
// When bm25Only is true, only BM25 keyword search is used (no vector search).
func (c *Client) SearchDocuments(ctx context.Context, vaultName, query string, limit int, bm25Only bool) ([]SearchResult, error) {
	q := url.Values{"query": {query}}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if bm25Only {
		q.Set("bm25_only", "true")
	}
	var resp httputil.ListResponse[SearchResult]
	if err := c.Get(ctx, vaultPath(vaultName, "/search")+"?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	return resp.Items, nil
}

// Folder is the JSON representation of a folder from the REST API.
type Folder struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// ListFolders lists folders in a vault, optionally under a parent path.
func (c *Client) ListFolders(ctx context.Context, vaultName string, parent *string) ([]Folder, error) {
	q := url.Values{}
	if parent != nil {
		q.Set("parent", *parent)
	}
	path := vaultPath(vaultName, "/folders")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var resp httputil.ListResponse[Folder]
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return resp.Items, nil
}

// CreateDocumentRequest is the body for creating a document via REST API.
type CreateDocumentRequest struct {
	VaultName string `json:"-"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// CreateDocument creates a new document on the remote server.
func (c *Client) CreateDocument(ctx context.Context, req CreateDocumentRequest) (*Document, error) {
	var doc Document
	if err := c.Post(ctx, vaultPath(req.VaultName, "/documents"), req, &doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}
	return &doc, nil
}

// EditDocumentRequest is the body for editing a document via REST API.
// Uses the same endpoint as create (upsert semantics).
type EditDocumentRequest = CreateDocumentRequest

// EditDocument updates an existing document on the remote server.
func (c *Client) EditDocument(ctx context.Context, req EditDocumentRequest) (*Document, error) {
	var doc Document
	if err := c.Post(ctx, vaultPath(req.VaultName, "/documents"), req, &doc); err != nil {
		return nil, fmt.Errorf("edit document: %w", err)
	}
	return &doc, nil
}

// Version is the JSON representation of a document version from the REST API.
type Version struct {
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListVersions returns version history for a document.
func (c *Client) ListVersions(ctx context.Context, vaultName, path string, limit int) ([]Version, error) {
	q := url.Values{"path": {path}}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	var resp httputil.ListResponse[Version]
	if err := c.Get(ctx, vaultPath(vaultName, "/versions")+"?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return resp.Items, nil
}

// Document is the JSON representation of a document returned by the REST API.
type Document struct {
	ID       string  `json:"id"`
	VaultID  string  `json:"vaultId"`
	Path     string  `json:"path"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Hash     *string `json:"hash,omitempty"`
	MimeType string  `json:"mimeType,omitempty"`
}

// GetDocument fetches a document by vault and path.
func (c *Client) GetDocument(ctx context.Context, vaultName, path string) (*Document, error) {
	q := url.Values{"path": {path}}
	var doc Document
	if err := c.Get(ctx, vaultPath(vaultName, "/documents")+"?"+q.Encode(), &doc); err != nil {
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
func (c *Client) DeleteDocuments(ctx context.Context, vaultName, path string, recursive, dryRun bool) (*DeleteResult, error) {
	q := url.Values{"path": {path}}
	if recursive {
		q.Set("recursive", "true")
	}
	if dryRun {
		q.Set("dry-run", "true")
	}
	var result DeleteResult
	if err := c.do(ctx, http.MethodDelete, vaultPath(vaultName, "/documents")+"?"+q.Encode(), nil, &result); err != nil {
		return nil, fmt.Errorf("delete documents: %w", err)
	}
	return &result, nil
}

// ListFiles lists files and folders at the given path in a vault.
func (c *Client) ListFiles(ctx context.Context, vaultName, path string, recursive bool) ([]models.FileEntry, error) {
	q := url.Values{"path": {path}}
	if recursive {
		q.Set("recursive", "true")
	}
	var resp httputil.ListResponse[models.FileEntry]
	if err := c.Get(ctx, vaultPath(vaultName, "/documents/ls")+"?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return resp.Items, nil
}

// ListFilesByLabels lists files with specific labels across the vault.
func (c *Client) ListFilesByLabels(ctx context.Context, vaultName string, labels []string) ([]models.FileEntry, error) {
	q := url.Values{
		"path":      {"/"},
		"recursive": {"true"},
		"labels":    {strings.Join(labels, ",")},
	}
	var resp httputil.ListResponse[models.FileEntry]
	if err := c.Get(ctx, vaultPath(vaultName, "/documents/ls")+"?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("list files by labels: %w", err)
	}
	return resp.Items, nil
}

// PatchLabelsRequest is the request body for patching document labels.
type PatchLabelsRequest struct {
	Path   string   `json:"path"`
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// PatchDocumentLabels adds or removes labels on a document.
func (c *Client) PatchDocumentLabels(ctx context.Context, vaultName string, req PatchLabelsRequest) error {
	return c.Patch(ctx, vaultPath(vaultName, "/documents/labels"), req, nil)
}

// ListLabels returns all distinct labels in the given vault.
func (c *Client) ListLabels(ctx context.Context, vaultName string) ([]string, error) {
	var resp httputil.ListResponse[string]
	if err := c.Get(ctx, vaultPath(vaultName, "/labels"), &resp); err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	return resp.Items, nil
}

// ListLabelsWithCounts returns labels with their document counts for the given vault.
func (c *Client) ListLabelsWithCounts(ctx context.Context, vaultName string) ([]models.LabelCount, error) {
	var resp httputil.ListResponse[models.LabelCount]
	if err := c.Get(ctx, vaultPath(vaultName, "/labels")+"?counts=true", &resp); err != nil {
		return nil, fmt.Errorf("list labels with counts: %w", err)
	}
	return resp.Items, nil
}

// ListBookmarks returns bookmarked files for the given vault.
func (c *Client) ListBookmarks(ctx context.Context, vaultName string) ([]models.FileEntry, error) {
	var resp httputil.ListResponse[models.FileEntry]
	if err := c.Get(ctx, vaultPath(vaultName, "/bookmarks"), &resp); err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	return resp.Items, nil
}

// AddBookmark pins a file/folder by path.
func (c *Client) AddBookmark(ctx context.Context, vaultName, filePath string) error {
	return c.Put(ctx, vaultPath(vaultName, "/bookmarks"), map[string]string{"path": filePath}, nil)
}

// RemoveBookmark unpins a file/folder by path.
func (c *Client) RemoveBookmark(ctx context.Context, vaultName, filePath string) error {
	return c.DeleteWithBody(ctx, vaultPath(vaultName, "/bookmarks"), map[string]string{"path": filePath})
}

// GetAssetReader downloads a binary asset and returns the response body as an io.ReadCloser.
// The caller must close the reader when done.
func (c *Client) GetAssetReader(ctx context.Context, vaultName, path string) (io.ReadCloser, error) {
	q := url.Values{"path": {path}}
	reqURL := c.baseURL + vaultPath(vaultName, "/assets") + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("get asset: HTTP %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// JobStatusResponse is the response from GET /api/jobs.
type JobStatusResponse struct {
	Stats        models.JobStats            `json:"stats"`
	Durations    []models.JobTypeDuration   `json:"durations"`
	Active       []models.PipelineJobDetail `json:"active"`
	RecentFailed []models.PipelineJobDetail `json:"recent_failed"`
}

// GetJobStatus fetches pipeline job stats from the server.
func (c *Client) GetJobStatus(ctx context.Context, since string) (*JobStatusResponse, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	path := "/api/v1/jobs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var resp JobStatusResponse
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("get job status: %w", err)
	}
	return &resp, nil
}

// ReprocessRequest is the request body for POST /api/v1/jobs/reprocess.
type ReprocessRequest struct {
	Vault string `json:"vault,omitempty"`
}

// ReprocessResponse is the response from POST /api/v1/jobs/reprocess.
type ReprocessResponse struct {
	JobsCancelled int `json:"jobs_cancelled"`
	HashesCleared int `json:"hashes_cleared"`
	JobsEnqueued  int `json:"jobs_enqueued"`
}

// Reprocess triggers a full reprocess of all files, optionally filtered by vault.
func (c *Client) Reprocess(ctx context.Context, req ReprocessRequest) (*ReprocessResponse, error) {
	var resp ReprocessResponse
	if err := c.Post(ctx, "/api/v1/jobs/reprocess", req, &resp); err != nil {
		return nil, fmt.Errorf("reprocess: %w", err)
	}
	return &resp, nil
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

	VersionCount      int   `json:"versionCount"`
	ConversationCount int   `json:"conversationCount"`
	TokenInput        int64 `json:"tokenInput"`
	TokenOutput       int64 `json:"tokenOutput"`
}

// GetVaultInfo fetches comprehensive stats about a vault.
func (c *Client) GetVaultInfo(ctx context.Context, vaultName string) (*VaultInfo, error) {
	var info VaultInfo
	if err := c.Get(ctx, vaultPath(vaultName, ""), &info); err != nil {
		return nil, fmt.Errorf("get vault info: %w", err)
	}
	return &info, nil
}

// GetVaultSettings fetches vault settings with defaults applied.
func (c *Client) GetVaultSettings(ctx context.Context, vaultName string) (*models.VaultSettings, error) {
	var settings models.VaultSettings
	if err := c.Get(ctx, vaultPath(vaultName, "/settings"), &settings); err != nil {
		return nil, fmt.Errorf("get vault settings: %w", err)
	}
	return &settings, nil
}

// UpdateVaultSettings updates vault settings (partial: only non-zero fields are applied).
func (c *Client) UpdateVaultSettings(ctx context.Context, vaultName string, patch models.VaultSettings) (*models.VaultSettings, error) {
	var settings models.VaultSettings
	if err := c.Patch(ctx, vaultPath(vaultName, "/settings"), patch, &settings); err != nil {
		return nil, fmt.Errorf("update vault settings: %w", err)
	}
	return &settings, nil
}

// FetchWebpageRequest is the body for the fetch webpage endpoint.
type FetchWebpageRequest struct {
	URL       string  `json:"url"`
	VaultName string  `json:"-"`
	Path      *string `json:"path,omitempty"`
	Clean     bool    `json:"clean,omitempty"`
}

// FetchWebpageResponse is the response from the fetch webpage endpoint.
type FetchWebpageResponse struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// FetchWebpage fetches a web page via the server and saves it to the vault.
func (c *Client) FetchWebpage(ctx context.Context, req FetchWebpageRequest) (*FetchWebpageResponse, error) {
	var resp FetchWebpageResponse
	if err := c.Post(ctx, vaultPath(req.VaultName, "/documents/clip"), req, &resp); err != nil {
		return nil, fmt.Errorf("fetch webpage: %w", err)
	}
	return &resp, nil
}

// DownloadExport downloads a vault export archive to the given output path.
// Returns the number of bytes written.
func (c *Client) DownloadExport(ctx context.Context, vaultName, outputPath string) (int64, error) {
	return c.downloadToFile(ctx, vaultPath(vaultName, "/export"), outputPath)
}

// DownloadBackup downloads a manifest-based backup archive to the given output path.
// Returns the number of bytes written.
func (c *Client) DownloadBackup(ctx context.Context, vaultName, outputPath string) (int64, error) {
	return c.downloadToFile(ctx, vaultPath(vaultName, "/backup"), outputPath)
}

// RestoreBackup uploads a backup archive to the server for restoration.
func (c *Client) RestoreBackup(ctx context.Context, archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/backup/restore", f)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/gzip")

	// Use a client without timeout — restore can take a long time for large
	// vaults and the context handles cancellation.
	noTimeoutClient := &http.Client{}
	resp, err := noTimeoutClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var pd httputil.ProblemDetail
		if json.Unmarshal(body, &pd) == nil && pd.Detail != "" {
			return &HTTPError{StatusCode: resp.StatusCode, Message: pd.Detail}
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
	}
	return nil
}

// DownloadExportEPUB downloads an EPUB export to the given output path.
// Returns the number of bytes written.
func (c *Client) DownloadExportEPUB(ctx context.Context, vaultName, path, title, author, outputPath string) (int64, error) {
	params := url.Values{"path": {path}}
	if title != "" {
		params.Set("title", title)
	}
	if author != "" {
		params.Set("author", author)
	}
	return c.downloadToFile(ctx, vaultPath(vaultName, "/export/epub")+"?"+params.Encode(), outputPath)
}

// downloadToFile performs a GET request and streams the response body to a file.
// Uses no HTTP timeout — context controls cancellation for large downloads.
func (c *Client) downloadToFile(ctx context.Context, apiPath, outputPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+apiPath, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// No timeout — large exports may take a while. Context controls cancellation.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return 0, fmt.Errorf("HTTP %d (failed to read error body: %w)", resp.StatusCode, readErr)
		}
		var pd httputil.ProblemDetail
		if json.Unmarshal(body, &pd) == nil && pd.Detail != "" {
			return 0, &HTTPError{StatusCode: resp.StatusCode, Message: pd.Detail}
		}
		return 0, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
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
			logutil.FromCtx(ctx).Warn("failed to clean up partial file", "path", outputPath, "error", removeErr)
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
		var pd httputil.ProblemDetail
		if json.Unmarshal(respBody, &pd) == nil && pd.Detail != "" {
			return &HTTPError{StatusCode: resp.StatusCode, Message: pd.Detail}
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if target != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, target); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}
